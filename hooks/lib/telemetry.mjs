import { chmod, mkdir, open, readFile, readdir, rename, rm, stat, writeFile } from "node:fs/promises";
import path from "node:path";
import { withDirectoryLock } from "./lock.mjs";

const activationFields = ["runtime", "skill", "sessionId", "eventKind", "projectId", "teamId", "identityKind", "correlationId"];

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

export function activationRecordFor(event, project, identity = { role: "unregistered" }) {
  const skill = event.operation.kind === "skill" ? event.operation.skill.toLowerCase().replace(/^\//, "") : "";
  if (event.runtime !== "claude" || skill !== "agent-team" || !["PreToolUse", "UserPromptExpansion"].includes(event.event)) return undefined;
  return {
    runtime: event.runtime,
    skill: "agent-team",
    sessionId: event.sessionId,
    eventKind: event.event,
    projectId: project.projectId,
    identityKind: identity.role === "unknown" ? "unregistered" : identity.role,
    ...(identity.role === "team" ? { teamId: identity.team["team id"] } : {}),
    correlationId: event.eventId || `${event.sessionId}:${event.event}`,
  };
}

async function boundedSource(file, maxBytes = 64 * 1024) {
  if (!file) return { status: "unavailable", reason: "path_missing", text: "", truncated: false };
  let handle;
  try {
    handle = await open(file, "r");
    const buffer = Buffer.alloc(maxBytes + 1);
    const { bytesRead } = await handle.read(buffer, 0, buffer.length, 0);
    return { status: "current", text: buffer.subarray(0, Math.min(bytesRead, maxBytes)).toString("utf8"), truncated: bytesRead > maxBytes };
  } catch (error) {
    return { status: "unavailable", reason: error.code ?? "read_failed", text: "", truncated: false };
  } finally {
    await handle?.close();
  }
}

function trackerSource(source) {
  const lines = source.text.split(/\r?\n/);
  const header = lines.findIndex((line) => /^\s*\|/.test(line) && /\bID\b/i.test(line) && /\bOwner\b/i.test(line));
  if (source.status !== "current") return { status: source.status, reason: source.reason, taskIds: [], rows: [], truncated: source.truncated };
  if (header === -1) return { status: "malformed", taskIds: [], rows: [], truncated: source.truncated };
  const headings = lines[header].trim().replace(/^\||\|$/g, "").split("|").map((value) => value.trim().toLowerCase());
  const idIndex = headings.indexOf("id");
  const ownerIndex = headings.indexOf("owner");
  const rows = [];
  for (const line of lines.slice(header + 2)) {
    if (!/^\s*\|/.test(line)) break;
    const cells = line.trim().replace(/^\||\|$/g, "").split("|").map((value) => value.trim());
    if (cells[idIndex]) rows.push({ id: cells[idIndex], owner: cells[ownerIndex] ?? "" });
  }
  return { status: rows.length ? "current" : "malformed", taskIds: rows.map(({ id }) => id), rows, truncated: source.truncated };
}

function mistakesSource(source, taskIds) {
  if (source.status !== "current") return { status: source.status, reason: source.reason, lessonIds: [], taskIds: [], truncated: source.truncated };
  if (!/^#\s+Agent-Team Mistakes\b/im.test(source.text)) return { status: "malformed", lessonIds: [], taskIds: [], truncated: source.truncated };
  const lessonIds = [...source.text.matchAll(/^##\s+(M-\d+)\b/gim)].map((match) => match[1]);
  const referencedTaskIds = taskIds.filter((id) => source.text.includes(id));
  return { status: "current", lessonIds, taskIds: referencedTaskIds, truncated: source.truncated };
}

/** Correlate bounded stable identifiers without claiming policy effectiveness. */
export async function auditEffectiveness({ logDirectory, trackerPath, mistakesPath, maxRecords = 1000 }) {
  const logs = await readActivationLogs(logDirectory, { maxRecords });
  const byRuntime = Object.create(null);
  for (const record of logs.records) byRuntime[record.runtime] = (byRuntime[record.runtime] ?? 0) + 1;
  const tracker = trackerSource(await boundedSource(trackerPath));
  const mistakes = mistakesSource(await boundedSource(mistakesPath), tracker.taskIds);
  const activatedTeams = new Set(logs.records.map(({ teamId }) => teamId).filter(Boolean));
  const activatedTaskIds = tracker.rows.filter(({ owner }) => activatedTeams.has(owner)).map(({ id }) => id);
  return {
    status: "completed",
    recordsProcessed: logs.records.length,
    malformed: logs.malformed,
    truncated: logs.truncated,
    activations: byRuntime,
    coverage: { codex: activationCapability("codex"), claude: activationCapability("claude") },
    references: { tracker: trackerPath, mistakes: mistakesPath },
    sources: {
      tracker: { status: tracker.status, taskIds: tracker.taskIds, truncated: tracker.truncated, ...(tracker.reason ? { reason: tracker.reason } : {}) },
      mistakes: { status: mistakes.status, lessonIds: mistakes.lessonIds, taskIds: mistakes.taskIds, truncated: mistakes.truncated, ...(mistakes.reason ? { reason: mistakes.reason } : {}) },
    },
    correlations: { activatedTaskIds, mistakeTaskIds: mistakes.taskIds },
    limitation: "This is a factual identifier correlation. Activation counts do not prove task quality or policy effectiveness.",
  };
}
