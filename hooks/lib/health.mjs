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
const RELEASE_REPOSITORY = "https://github.com/thebpandey/agent-team";
const ARTIFACT_KEYS = ["archiveContentDigest", "archiveFileMap", "archiveName", "archiveSha256", "archiveUrl", "checksumFileName", "checksumFileSha256", "checksumUrl", "name", "packageContentDigest", "packageFileMap", "releaseTag", "releaseUrl", "repository", "sourceRevision", "updateUrl", "version"].sort();
const FILE_MAP_RECORD_KEYS = ["mode", "sha256", "size"];

function orderedFileMap(value) {
  if (!value || typeof value !== "object" || Array.isArray(value)) return null;
  const entries = Object.entries(value).sort(([left], [right]) => Buffer.from(left).compare(Buffer.from(right)));
  if (!entries.length || entries.some(([name, record]) => !name || name.startsWith("/") || name.includes("\\")
    || name.split("/").some((part) => !part || part === "." || part === "..") || !record || typeof record !== "object"
    || Array.isArray(record) || Object.keys(record).sort().join("\0") !== FILE_MAP_RECORD_KEYS.join("\0")
    || !/^[0-9a-f]{64}$/.test(record.sha256) || ![0o644, 0o755].includes(record.mode)
    || !Number.isSafeInteger(record.size) || record.size < 0)) return null;
  return Object.fromEntries(entries);
}

function expectedArchiveFileMap(artifact) {
  const packageFileMap = orderedFileMap(artifact.packageFileMap);
  if (!packageFileMap) return null;
  const metadata = {
    name: artifact.name,
    version: artifact.version,
    hosts: ["codex", "claude-code"],
    repository: artifact.repository,
    releaseTag: artifact.releaseTag,
    releaseUrl: artifact.releaseUrl,
    updateUrl: artifact.updateUrl,
    sourceRevision: artifact.sourceRevision,
    packageFileMap,
    packageContentDigest: artifact.packageContentDigest,
  };
  const bytes = Buffer.from(`${JSON.stringify(metadata, null, 2)}\n`);
  return orderedFileMap({ ...packageFileMap, ".agent-team-source.json": {
    sha256: createHash("sha256").update(bytes).digest("hex"), mode: 0o644, size: bytes.length,
  } });
}

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

async function observedPackage(root) {
  const files = {}, contents = [];
  async function walk(relative = "") {
    for (const entry of (await readdir(path.join(root, relative), { withFileTypes: true })).sort((left, right) => left.name.localeCompare(right.name))) {
      const name = path.posix.join(relative, entry.name);
      if (entry.isDirectory()) await walk(name);
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
          files[name] = { sha256: createHash("sha256").update(bytes).digest("hex"), mode: Number(after.mode & 0o777n), size: bytes.length };
          contents.push([name, bytes]);
        } finally { await handle?.close(); }
      } else throw new Error("installed file is not regular");
    }
  }
  await walk();
  const digest = createHash("sha256");
  for (const [name, bytes] of contents.sort(([left], [right]) => Buffer.from(left).compare(Buffer.from(right)))) digest.update(name).update(bytes);
  return { files, digest: digest.digest("hex") };
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
    const artifact = receipt.artifact;
    const releaseTag = `v${artifact.version}`;
    const archiveName = `agent-team-${artifact.version}.zip`;
    const releaseAssetRoot = `${RELEASE_REPOSITORY}/releases/download/${releaseTag}`;
    const expectedArchiveMap = expectedArchiveFileMap(artifact);
    if (Object.keys(artifact).sort().join("\0") !== ARTIFACT_KEYS.join("\0")
      || receipt.version !== artifact.version
      || !/^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)$/.test(artifact.version)
      || artifact.name !== "agent-team"
      || artifact.repository !== RELEASE_REPOSITORY
      || artifact.releaseTag !== releaseTag
      || artifact.archiveName !== archiveName
      || artifact.releaseUrl !== `${RELEASE_REPOSITORY}/releases/tag/${releaseTag}`
      || artifact.updateUrl !== `${RELEASE_REPOSITORY}/releases/latest`
      || artifact.archiveUrl !== `${releaseAssetRoot}/${archiveName}`
      || artifact.checksumFileName !== "SHA256SUMS"
      || artifact.checksumUrl !== `${releaseAssetRoot}/SHA256SUMS`
      || !/^[0-9a-f]{40}$/.test(receipt.artifact.sourceRevision)
      || !/^[0-9a-f]{64}$/.test(receipt.artifact.archiveSha256)
      || !/^[0-9a-f]{64}$/.test(receipt.artifact.checksumFileSha256)
      || !expectedArchiveMap
      || JSON.stringify(orderedFileMap(artifact.archiveFileMap)) !== JSON.stringify(expectedArchiveMap)
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
    || targetReceipt.mode !== "copied"
    || !/^[0-9a-f]{64}$/.test(targetReceipt.digest)
    || new Set(targetNames).size !== targetNames.length
    || JSON.stringify(targetNames) !== JSON.stringify(recordedNames)) {
    return { status: "drifted", releaseTag: receipt.artifact.releaseTag ?? null,
      sourceRevision: receipt.artifact.sourceRevision ?? null, archiveSha256: receipt.artifact.archiveSha256 ?? null,
      installedFileMapDigest: null };
  }
  let current = false;
  try {
    const observed = await observedPackage(target);
    current = fileMapDigest(observed.files) === record.digest
      && fileMapDigest(observed.files) === receipt.artifact.packageContentDigest
      && observed.digest === targetReceipt.digest;
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
