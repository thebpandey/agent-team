import { chmod, mkdir, readFile, readdir, rename, rm, stat, writeFile } from "node:fs/promises";
import path from "node:path";
import { withDirectoryLock } from "./lock.mjs";

const activationFields = ["runtime", "skill", "sessionId", "eventKind", "projectId", "teamId", "correlationId"];

function activationRecord(input, now) {
  const output = { timestamp: now.toISOString() };
  for (const field of activationFields) if (["string", "number", "boolean"].includes(typeof input[field])) output[field] = input[field];
  return output;
}

function logNames(maxFiles) {
  return [...Array(maxFiles).keys()].slice(1).reverse().map((index) => `activation.jsonl.${index}`).concat("activation.jsonl");
}

async function rotate(directory, maxFiles) {
  await rm(path.join(directory, `activation.jsonl.${maxFiles}`), { force: true });
  for (let index = maxFiles - 1; index >= 1; index -= 1) {
    try {
      await rename(path.join(directory, `activation.jsonl.${index}`), path.join(directory, `activation.jsonl.${index + 1}`));
    } catch (error) {
      if (error.code !== "ENOENT") throw error;
    }
  }
  try {
    await rename(path.join(directory, "activation.jsonl"), path.join(directory, "activation.jsonl.1"));
  } catch (error) {
    if (error.code !== "ENOENT") throw error;
  }
}

/** Append one allowlisted activation event under a lock, with bounded rotation and dedupe. */
export async function appendActivationLog(directory, input, { now = new Date(), maxBytes = 256 * 1024, maxFiles = 3 } = {}) {
  await mkdir(directory, { recursive: true, mode: 0o700 });
  await chmod(directory, 0o700);
  const file = path.join(directory, "activation.jsonl");
  return withDirectoryLock(path.join(directory, ".activation.lock"), {
    pid: process.pid,
    sessionId: input.sessionId,
    acquiredAt: now.toISOString(),
  }, async () => {
    const existing = await readActivationLogs(directory, { maxRecords: 2000, maxFiles });
    if (input.correlationId && existing.records.some(({ correlationId }) => correlationId === input.correlationId)) {
      return { recorded: false, path: file };
    }
    const line = `${JSON.stringify(activationRecord(input, now))}\n`;
    let size = 0;
    try {
      size = (await stat(file)).size;
    } catch (error) {
      if (error.code !== "ENOENT") throw error;
    }
    if (size > 0 && size + Buffer.byteLength(line) > maxBytes) await rotate(directory, maxFiles);
    await writeFile(file, line, { flag: "a", mode: 0o600 });
    await chmod(file, 0o600);
    return { recorded: true, path: file };
  });
}

export async function readActivationLogs(directory, { maxRecords = 1000, maxFiles = 3 } = {}) {
  const records = [];
  let malformed = 0;
  let names;
  try {
    names = new Set(await readdir(directory));
  } catch (error) {
    if (error.code === "ENOENT") return { records, malformed, truncated: false };
    throw error;
  }
  for (const name of logNames(maxFiles).filter((entry) => names.has(entry))) {
    const source = await readFile(path.join(directory, name), "utf8");
    for (const line of source.split(/\r?\n/).filter(Boolean)) {
      try {
        const record = JSON.parse(line);
        if (record && typeof record === "object") records.push(record);
        else malformed += 1;
      } catch {
        malformed += 1;
      }
    }
  }
  const truncated = records.length > maxRecords;
  return { records: records.slice(-maxRecords), malformed, truncated };
}

export function activationCapability(runtime) {
  if (runtime === "claude") return {
    status: "supported",
    paths: ["PreToolUse:Skill", "UserPromptExpansion"],
    limitation: "OpenTelemetry skill_activated events are outside the local hook log.",
  };
  return {
    status: "unsupported",
    paths: [],
    limitation: "Codex exposes no reliable local skill-activation hook. SKILL.md reads are not activation evidence.",
  };
}

export function activationRecordFor(event, project) {
  const skill = event.operation.kind === "skill" ? event.operation.skill.toLowerCase().replace(/^\//, "") : "";
  if (event.runtime !== "claude" || skill !== "agent-team" || !["PreToolUse", "UserPromptExpansion"].includes(event.event)) return undefined;
  return {
    runtime: event.runtime,
    skill: "agent-team",
    sessionId: event.sessionId,
    eventKind: event.event,
    projectId: project.projectId,
    correlationId: event.eventId || `${event.sessionId}:${event.event}`,
  };
}

/** Summarize bounded activation evidence and point to, but do not duplicate, project records. */
export async function auditEffectiveness({ logDirectory, trackerPath, mistakesPath, maxRecords = 1000 }) {
  const logs = await readActivationLogs(logDirectory, { maxRecords });
  const byRuntime = Object.create(null);
  for (const record of logs.records) byRuntime[record.runtime] = (byRuntime[record.runtime] ?? 0) + 1;
  return {
    status: "completed",
    recordsProcessed: logs.records.length,
    malformed: logs.malformed,
    truncated: logs.truncated,
    activations: byRuntime,
    coverage: { codex: activationCapability("codex"), claude: activationCapability("claude") },
    references: { tracker: trackerPath, mistakes: mistakesPath },
    limitation: "Activation counts show hook-visible use. They do not prove task quality or policy effectiveness.",
  };
}
