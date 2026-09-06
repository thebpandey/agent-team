import { mkdir, readFile, rename, writeFile } from "node:fs/promises";
import path from "node:path";
import { randomUUID } from "node:crypto";
import { withDirectoryLock } from "./lock.mjs";

const fields = [
  "eventId", "sessionId", "eventKind", "projectId", "teamId", "skillRevision", "worktree", "branch",
  "revision", "trackerPath", "mistakesPath", "taskIds", "evidence", "nextAction", "decisionNotes", "pendingOperations",
];

function safeId(value) {
  return String(value || "unknown").replace(/[^a-zA-Z0-9_.-]/g, "_").slice(0, 120);
}

function redact(value, key = "") {
  if (/password|secret|token|credential|connection|string|raw.?sql|prompt|argument|content/i.test(key)) return "[REDACTED]";
  if (Array.isArray(value)) return value.map((entry) => redact(entry));
  if (value && typeof value === "object") {
    return Object.fromEntries(Object.entries(value).map(([name, entry]) => [name, redact(entry, name)]));
  }
  if (typeof value === "string") {
    return value
      .replace(/\b(?:sk|ghp|github_pat|xox[baprs])-[a-zA-Z0-9_-]+\b/g, "[REDACTED]")
      .replace(/\bBearer\s+[^\s]+/gi, "Bearer [REDACTED]")
      .replace(/:\/\/[^/@\s]+:[^/@\s]+@/g, "://[REDACTED]@");
  }
  return value;
}

function checkpointData(input, previous, now) {
  const output = { schemaVersion: 1 };
  for (const field of fields) {
    const value = input[field];
    if (value !== undefined && value !== "") output[field] = redact(value, field);
    else if ((field === "nextAction" || field === "decisionNotes") && previous?.[field]) output[field] = redact(previous[field], field);
  }
  output.updatedAt = now.toISOString();
  return output;
}

async function existing(file) {
  try {
    return JSON.parse(await readFile(file, "utf8"));
  } catch (error) {
    if (error.code === "ENOENT") return undefined;
    throw error;
  }
}

/** Save a small factual recovery record without storing the native hook payload. */
export async function writeCheckpoint(project, input, { now = new Date(), timeoutMs = 1000 } = {}) {
  if (!project.active) return { created: false, skipped: "inactive" };
  await mkdir(project.paths.checkpoints, { recursive: true, mode: 0o700 });
  const file = path.join(project.paths.checkpoints, `${safeId(input.sessionId)}.json`);
  const lock = path.join(project.paths.locks, `checkpoint-${safeId(input.sessionId)}.lock`);

  return withDirectoryLock(lock, {
    pid: process.pid,
    sessionId: input.sessionId,
    acquiredAt: now.toISOString(),
  }, async () => {
    const previous = await existing(file);
    if (input.eventId && previous?.eventId === input.eventId) return { created: false, path: file, checkpoint: previous };
    const checkpoint = checkpointData(input, previous, now);
    const temporary = `${file}.${process.pid}.${randomUUID()}.tmp`;
    await writeFile(temporary, `${JSON.stringify(checkpoint, null, 2)}\n`, { mode: 0o600 });
    await rename(temporary, file);
    return { created: true, path: file, checkpoint };
  }, { timeoutMs });
}
