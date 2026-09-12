import { constants } from "node:fs";
import { access, mkdir, open, readFile, readdir, rename, rm, writeFile } from "node:fs/promises";
import { createHash, randomUUID } from "node:crypto";
import path from "node:path";
import { operationMappingHealth } from "./canonical-state.mjs";
import { resolveProject } from "./project.mjs";
import { activationCapability, readActivationLogs } from "./telemetry.mjs";
import { withDirectoryLock } from "./lock.mjs";
import { fileMapDigest } from "./artifacts.mjs";

const supportedEvents = {
  codex: new Set(["SessionStart", "PreToolUse", "PostToolUse", "PreCompact", "PostCompact", "Interrupt", "Stop", "SessionEnd", "UserPromptSubmit"]),
  claude: new Set(["SessionStart", "PreToolUse", "PostToolUse", "PostToolBatch", "PreCompact", "PostCompact", "TaskCompleted", "UserPromptExpansion", "UserPromptSubmit", "Stop", "SessionEnd"]),
};

async function present(file) {
  try {
    await access(file);
    return true;
  } catch {
    return false;
  }
}

async function json(file) {
  try {
    return JSON.parse(await readFile(file, "utf8"));
  } catch {
    return {};
  }
}

function registered(config) {
  return Object.values(config.hooks ?? {}).flat().some((group) => group.hooks?.some(({ command = "" }) => command.includes("agent-team-hook.mjs")));
}

function eventHealth(runtime, config, records) {
  const events = new Set([...supportedEvents.codex, ...supportedEvents.claude, ...Object.keys(config.hooks ?? {})]);
  return Object.fromEntries([...events].map((event) => {
    const supported = supportedEvents[runtime].has(event);
    const record = records[`${runtime}:${event}`];
    return [event, {
      registered: registered({ hooks: { [event]: config.hooks?.[event] ?? [] } }), supported, supportSource: "packaged_adapter", nativeSupport: "unknown", trusted: "unknown",
      exercise: !supported ? "unsupported" : record?.status === "failed" ? "failed" : record?.status === "passed" ? "exercised" : "unobserved",
      ...(record ? { source: record.source ?? "recorded_transport", observedAt: record.observedAt } : {}),
    }];
  }));
}

async function observedFileMap(root, relative = "", output = {}) {
  for (const entry of (await readdir(path.join(root, relative), { withFileTypes: true })).sort((left, right) => left.name.localeCompare(right.name))) {
    const name = path.posix.join(relative, entry.name);
    if (entry.isDirectory()) await observedFileMap(root, name, output);
    else if (entry.isFile() && !entry.isSymbolicLink()) {
      const file = path.join(root, name);
      let handle;
      try {
        handle = await open(file, constants.O_RDONLY | constants.O_NOFOLLOW | constants.O_NONBLOCK);
        const before = await handle.stat({ bigint: true });
        const bytes = await handle.readFile();
        const after = await handle.stat({ bigint: true });
        if (!before.isFile() || !after.isFile() || bytes.length !== Number(after.size)
          || ["dev", "ino", "size", "mtimeNs", "ctimeNs"].some((key) => before[key] !== after[key])) throw new Error("installed file changed");
        output[name] = { sha256: createHash("sha256").update(bytes).digest("hex"), mode: Number(after.mode & 0o777n), size: bytes.length };
      } finally { await handle?.close(); }
    } else output[name] = null;
  }
  return output;
}

async function artifactHealth(root, runtime, installed, receipt) {
  const empty = { releaseTag: null, sourceRevision: null, archiveSha256: null, installedFileMapDigest: null };
  if (!installed) return { status: "not_installed", ...empty };
  const target = path.join(root, runtime === "codex" ? ".agents" : ".claude", "skills", "agent-team");
  const targetReceipts = receipt?.targets?.filter((entry) => entry.runtime === runtime && entry.path === target) ?? [];
  const targetReceipt = targetReceipts[0];
  if (targetReceipts.length !== 1) return { status: "missing_receipt", ...empty };
  if (receipt.schemaVersion !== 4 || !receipt.artifact) return { status: "unverified_legacy", ...empty };
  try {
    if (receipt.version !== receipt.artifact.version
      || !/^v\d+\.\d+\.\d+$/.test(receipt.artifact.releaseTag)
      || !/^[0-9a-f]{40}$/.test(receipt.artifact.sourceRevision)
      || !/^[0-9a-f]{64}$/.test(receipt.artifact.archiveSha256)
      || fileMapDigest(receipt.artifact.packageFileMap) !== receipt.artifact.packageContentDigest
      || fileMapDigest(receipt.artifact.archiveFileMap) !== receipt.artifact.archiveContentDigest) throw new Error("invalid receipt");
  } catch {
    return { status: "drifted", ...empty };
  }
  const record = receipt.installedFileMaps?.[runtime];
  if (!record) return { status: "drifted", releaseTag: receipt.artifact.releaseTag ?? null,
    sourceRevision: receipt.artifact.sourceRevision ?? null, archiveSha256: receipt.artifact.archiveSha256 ?? null,
    installedFileMapDigest: null };
  const recordedNames = Object.keys(record.files ?? {}).sort();
  const targetNames = Array.isArray(targetReceipt.files) ? [...targetReceipt.files].sort() : [];
  if (Object.keys(record).sort().join("\0") !== ["digest", "files", "target"].sort().join("\0")
    || record.target !== target
    || JSON.stringify(record.files) !== JSON.stringify(receipt.artifact.packageFileMap)
    || record.digest !== fileMapDigest(record.files)
    || JSON.stringify(targetNames) !== JSON.stringify(recordedNames)) {
    return { status: "drifted", releaseTag: receipt.artifact.releaseTag ?? null,
      sourceRevision: receipt.artifact.sourceRevision ?? null, archiveSha256: receipt.artifact.archiveSha256 ?? null,
      installedFileMapDigest: null };
  }
  let current = false;
  try {
    const observed = await observedFileMap(target);
    current = Object.values(observed).every(Boolean) && fileMapDigest(observed) === record.digest
      && fileMapDigest(observed) === receipt.artifact.packageContentDigest;
  } catch { current = false; }
  return {
    status: current ? "current" : "drifted",
    releaseTag: receipt.artifact.releaseTag ?? null,
    sourceRevision: receipt.artifact.sourceRevision ?? null,
    archiveSha256: receipt.artifact.archiveSha256 ?? null,
    installedFileMapDigest: record.digest ?? null,
  };
}

/** Record execution of one hook transport. This is not evidence of native trust or every event. */
export async function recordHookEvidence(home, input, { budget, now = new Date() } = {}) {
  if (!supportedEvents[input.runtime]?.has(input.event) || !["passed", "failed"].includes(input.status) || !input.eventId) return { status: "unavailable", reason: "invalid_event_evidence" };
  const directory = path.join(home, ".agent-team-hooks");
  const file = path.join(directory, "hook-events.json");
  const bounded = (action) => budget ? budget.run(action) : action();
  try {
    budget?.check();
    await bounded(() => mkdir(directory, { recursive: true, mode: 0o700 }));
    return await withDirectoryLock(path.join(directory, ".hook-evidence.lock"), { eventId: input.eventId, pid: process.pid }, async () => {
      const records = await bounded(() => json(file));
      const key = `${input.runtime}:${input.event}`;
      if (records[key]?.eventId === input.eventId) return { status: "duplicate" };
      records[key] = { status: input.status, eventId: String(input.eventId).slice(0, 128), sessionId: String(input.sessionId ?? "unknown").slice(0, 128), observedAt: now.toISOString(), source: input.source === "packaged_entrypoint" ? "packaged_entrypoint" : "recorded_transport" };
      const temporary = `${file}.${randomUUID()}.tmp`;
      try {
        budget?.check();
        await writeFile(temporary, JSON.stringify(records), { mode: 0o600, ...(budget ? { signal: budget.signal } : {}) });
        budget?.check();
        await rename(temporary, file);
      } finally { await rm(temporary, { force: true }); }
      return { status: "applied", path: file };
    }, { budget });
  } catch { return { status: "unavailable", reason: "evidence_write_unavailable" }; }
}

/** Only a receipted package may attribute observations to an installation. */
export async function resolveHookEvidenceRoot(packageRoot, runtime) {
  if (!['codex', 'claude'].includes(runtime)) return null;
  const root = path.resolve(packageRoot, '../../..');
  const expected = path.join(root, runtime === 'codex' ? '.agents' : '.claude', 'skills', 'agent-team');
  if (path.resolve(packageRoot) !== expected) return null;
  const receipt = await json(path.join(root, '.agent-team-hooks/install.json'));
  const legacyTarget = receipt?.targets?.some((entry) => entry?.runtime === runtime && entry?.path === expected);
  if (receipt.schemaVersion !== 4) return legacyTarget ? root : null;
  const installed = await present(path.join(expected, "SKILL.md"));
  return (await artifactHealth(root, runtime, installed, receipt)).status === "current" ? root : null;
}

/** Report each installation dimension separately. Trust stays unknown without native evidence. */
export async function getHealth({ home, projectPath, scope = 'user' }) {
  if (!['user', 'project'].includes(scope)) throw new Error('Health scope must be user or project.');
  if (scope === 'project' && !projectPath) throw new Error('Project-scope health requires a project path.');
  const project = projectPath ? await resolveProject(projectPath) : null;
  const root = scope === 'project' ? project.root : home;
  const events = await json(path.join(root, ".agent-team-hooks", "hook-events.json"));
  const logs = await readActivationLogs(path.join(root, ".agent-team-hooks", "logs"));
  const codexConfig = await json(path.join(root, ".codex", "hooks.json"));
  const claudeConfig = await json(path.join(root, ".claude", scope === 'project' ? "settings.local.json" : "settings.json"));
  const receipt = await json(path.join(root, ".agent-team-hooks", "install.json"));
  const codexInstalled = await present(path.join(root, ".agents", "skills", "agent-team", "SKILL.md"));
  const claudeInstalled = await present(path.join(root, ".claude", "skills", "agent-team", "SKILL.md"));
  const health = {
    status: "completed",
    installation: { scope, root },
    runtimes: {
      codex: {
        installed: codexInstalled,
        artifact: await artifactHealth(root, "codex", codexInstalled, receipt),
        registered: registered(codexConfig),
        trusted: "unknown",
        activation: activationCapability("codex"),
        exercised: logs.records.some(({ runtime }) => runtime === "codex"),
        events: eventHealth("codex", codexConfig, events),
      },
      claude: {
        installed: claudeInstalled,
        artifact: await artifactHealth(root, "claude", claudeInstalled, receipt),
        registered: registered(claudeConfig),
        trusted: "unknown",
        activation: activationCapability("claude"),
        exercised: logs.records.some(({ runtime }) => runtime === "claude"),
        events: eventHealth("claude", claudeConfig, events),
      },
    },
    legacyCodexCopy: await present(path.join(root, ".codex", "skills", "agent-team", "SKILL.md")),
  };
  if (projectPath) {
    health.operationMappings = project.active
      ? await operationMappingHealth(project)
      : { status: "inactive", fallbackProtection: "unavailable" };
  }
  return health;
}
