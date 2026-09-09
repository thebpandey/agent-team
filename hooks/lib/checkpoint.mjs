import { mkdir, readFile, rename, rm, writeFile } from "node:fs/promises";
import path from "node:path";
import { createHash, randomUUID } from "node:crypto";
import { withDirectoryLock } from "./lock.mjs";
import { identityFor, loadCanonicalState } from "./canonical-state.mjs";

const fields = [
  "eventId", "sessionId", "eventKind", "projectId", "teamId", "skillRevision", "worktree", "branch",
  "revision", "trackerPath", "mistakesPath", "taskIds", "evidence", "nextAction", "decisionNotes", "pendingOperations",
  "sourcePointers", "uncertainty", "scope", "writer", "evidenceRevision", "resumeWhen",
];

function safeId(value) {
  return String(value || "unknown").replace(/[^a-zA-Z0-9_.-]/g, "_").slice(0, 120);
}

const fingerprint = (value) => createHash("sha256").update(value).digest("hex");
const operationSignature = (input) => fingerprint(JSON.stringify(Object.fromEntries(fields
  .filter((field) => input[field] !== undefined && input[field] !== "")
  .map((field) => [field, redact(input[field], field)]))));

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
  const sameScope = previous?.taskIds === undefined || JSON.stringify(input.taskIds ?? []) === JSON.stringify(previous.taskIds);
  const output = { schemaVersion: 1, version: (previous?.version ?? 0) + 1 };
  for (const field of fields) {
    const value = input[field];
    if (value !== undefined && value !== "") output[field] = redact(value, field);
    else if (sameScope && ["nextAction", "decisionNotes", "sourcePointers", "uncertainty", "scope", "pendingOperations", "resumeWhen", "writer", "evidence", "evidenceRevision"].includes(field) && previous?.[field]) output[field] = redact(previous[field], field);
  }
  output.updatedAt = now.toISOString();
  if (previous && previous.taskIds === undefined && (previous.nextAction || previous.decisionNotes)) {
    output.uncertainty = [...new Set([...(output.uncertainty ?? []), "Legacy authored notes lack task attribution."])];
  }
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
export async function writeCheckpoint(project, input, { now = new Date(), timeoutMs = 1000, budget, expectedVersion, actorSessionId } = {}) {
  if (!project.active) return { created: false, skipped: "inactive" };
  if (actorSessionId !== undefined && actorSessionId !== input.sessionId) return { status: "conflict", reason: "checkpoint_owner_required" };
  if (Buffer.byteLength(JSON.stringify(input)) > 32768) return { status: "conflict", reason: "checkpoint_too_large" };
  const bounded = (action) => budget ? budget.run(action) : action();
  if (actorSessionId !== undefined) {
    if (!Number.isInteger(expectedVersion) || !input.eventId) return { status: "conflict", reason: "checkpoint_version_and_operation_required" };
    const canonical = await loadCanonicalState(project, { includeTasks: false, budget });
    if (identityFor(canonical.registry, actorSessionId).role === "unknown") return { status: "conflict", reason: "checkpoint_owner_required" };
  }
  budget?.check();
  await bounded(() => mkdir(project.paths.checkpoints, { recursive: true, mode: 0o700 }));
  const file = path.join(project.paths.checkpoints, `${safeId(input.sessionId)}.json`);
  const lock = path.join(project.paths.locks, `checkpoint-${safeId(input.sessionId)}.lock`);

  return withDirectoryLock(lock, {
    pid: process.pid,
    sessionId: input.sessionId,
    acquiredAt: now.toISOString(),
  }, async () => {
    const previous = await bounded(() => existing(file));
    if (previous?.sessionId && previous.sessionId !== input.sessionId) return { status: "conflict", reason: "checkpoint_identity_collision" };
    const receipts = { ...previous?.operationReceipts };
    if (previous?.eventId && !receipts[fingerprint(String(previous.eventId))]) receipts[fingerprint(String(previous.eventId))] = { signature: null, version: previous.version ?? 0 };
    const receiptKey = input.eventId ? fingerprint(String(input.eventId)) : null;
    const receiptDirectory = path.join(project.paths.checkpoints, ".receipts", fingerprint(String(input.sessionId ?? "unknown")));
    const signature = operationSignature(input);
    const recorded = receiptKey && (receipts[receiptKey] ?? await bounded(() => existing(path.join(receiptDirectory, `${receiptKey}.json`))));
    if (recorded) return recorded.signature === signature
      ? { status: "duplicate", created: false, path: file, checkpoint: previous, version: previous.version, operationVersion: recorded.version }
      : { status: "conflict", reason: "operation_identity_reused", created: false, path: file, version: previous.version };
    if (expectedVersion !== undefined && expectedVersion !== (previous?.version ?? 0)) return { status: "conflict", reason: "stale_version", created: false, path: file, version: previous?.version ?? 0 };
    if (receiptKey && Object.keys(receipts).length >= 128) {
      // Archive only already-committed receipts before evicting the bounded read index.
      const [oldestKey, oldest] = Object.entries(receipts)[0];
      budget?.check();
      await bounded(() => mkdir(receiptDirectory, { recursive: true, mode: 0o700 }));
      const archive = path.join(receiptDirectory, `${oldestKey}.json`);
      try {
        budget?.check();
        await bounded(() => writeFile(archive, `${JSON.stringify(oldest)}\n`, { flag: "wx", mode: 0o600, ...(budget ? { signal: budget.signal } : {}) }));
      } catch (error) {
        if (error.code !== "EEXIST") throw error;
        if (JSON.stringify(await bounded(() => existing(archive))) !== JSON.stringify(oldest)) return { status: "conflict", reason: "checkpoint_receipt_conflict", created: false };
      }
      delete receipts[oldestKey];
    }
    const checkpoint = checkpointData(input, previous, now);
    checkpoint.operationReceipts = { ...receipts, ...(receiptKey ? { [receiptKey]: { signature, version: checkpoint.version } } : {}) };
    checkpoint.sourceEvidence = [];
    for (const pointer of checkpoint.sourcePointers ?? []) {
      if (input.sourcePointers === undefined) {
        checkpoint.sourceEvidence.push(previous?.sourceEvidence?.find((entry) => entry.path === pointer.path) ?? { path: pointer.path, fingerprint: null });
        continue;
      }
      // Hash original records, never infer or summarize unwritten decisions.
      try {
        const source = await bounded(() => readFile(path.resolve(project.root, pointer.path)));
        checkpoint.sourceEvidence.push({ path: pointer.path, fingerprint: createHash("sha256").update(source).digest("hex") });
      } catch { checkpoint.sourceEvidence.push({ path: pointer.path, fingerprint: null }); }
    }
    if (Buffer.byteLength(`${JSON.stringify(checkpoint, null, 2)}\n`) > 32768) return { status: "conflict", reason: "checkpoint_too_large" };
    const temporary = `${file}.${process.pid}.${randomUUID()}.tmp`;
    try {
      budget?.check();
      await writeFile(temporary, `${JSON.stringify(checkpoint, null, 2)}\n`, { mode: 0o600, ...(budget ? { signal: budget.signal } : {}) });
      budget?.check();
      await rename(temporary, file);
    } finally {
      await rm(temporary, { force: true });
    }
    return { status: "applied", created: true, path: file, checkpoint, version: checkpoint.version };
  }, { timeoutMs, budget });
}
