import { access, mkdir, readFile, rename, rm, writeFile } from "node:fs/promises";
import { randomUUID } from "node:crypto";
import path from "node:path";
import { operationMappingHealth } from "./canonical-state.mjs";
import { resolveProject } from "./project.mjs";
import { activationCapability, readActivationLogs } from "./telemetry.mjs";
import { withDirectoryLock } from "./lock.mjs";

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
  return Array.isArray(receipt.targets) && receipt.targets.some((target) => target.runtime === runtime && target.path === expected) ? root : null;
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
  const health = {
    status: "completed",
    installation: { scope, root },
    runtimes: {
      codex: {
        installed: await present(path.join(root, ".agents", "skills", "agent-team", "SKILL.md")),
        registered: registered(codexConfig),
        trusted: "unknown",
        activation: activationCapability("codex"),
        exercised: logs.records.some(({ runtime }) => runtime === "codex"),
        events: eventHealth("codex", codexConfig, events),
      },
      claude: {
        installed: await present(path.join(root, ".claude", "skills", "agent-team", "SKILL.md")),
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
