import assert from "node:assert/strict";
import { execFileSync } from "node:child_process";
import { mkdtemp, mkdir, readFile, readdir, rm, writeFile } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import test from "node:test";
import { resolveProject } from "../hooks/lib/project.mjs";
import { inspectRecovery } from "../hooks/lib/recovery.mjs";
import { writeCheckpoint } from "../hooks/lib/checkpoint.mjs";
import { withDirectoryLock } from "../hooks/lib/lock.mjs";
import { createEventBudget } from "../hooks/lib/budget.mjs";
import { policyFixture } from "./hook-test-helpers.mjs";

const temporary = [];
test.afterEach(async () => Promise.all(temporary.splice(0).map((p) => rm(p, { recursive: true, force: true }))));
async function fixture() {
  const root = await mkdtemp(path.join(os.tmpdir(), "agent-team-recovery-"));
  temporary.push(root, `${root}-feature`, `${root}-remote`);
  const value = await policyFixture(root);
  return { ...value, project: await resolveProject(value.feature) };
}

test("recovery selects the requested task/session/worktree despite a newer unrelated checkpoint", async () => {
  const value = await fixture();
  await writeCheckpoint(value.project, { eventId: "wanted", sessionId: "developer-session", taskIds: ["AT-001"], worktree: value.feature, revision: value.revision, nextAction: "Run the owned unit tests.",
    sourcePointers: [{ kind: "decision", path: "DESIGN.md", section: "Storage choice" }],
    pendingOperations: [{ operationId: "release-1", status: "unknown", action: "reconcile" }] }, { now: new Date("2026-09-08T10:00:00Z") });
  await writeCheckpoint(value.project, { eventId: "other", sessionId: "other-session", taskIds: ["AT-999"], worktree: value.root, nextAction: "Do unrelated work." }, { now: new Date("2026-09-08T10:01:00Z") });
  const result = await inspectRecovery(value.project, { sessionId: "developer-session", taskId: "AT-001", now: new Date("2026-09-08T10:02:00Z") });
  assert.equal(result.sessionId, "developer-session");
  assert.equal(result.nextAction, "Run the owned unit tests.");
  assert.equal(result.pendingOperations[0].status, "unknown");
  assert.deepEqual(result.sourcePointers, [{ kind: "decision", path: "DESIGN.md", section: "Storage choice" }]);
  assert.equal(result.revision, value.revision);
});

test("a requested task cannot inherit an unrelated checkpoint or previous task's authored notes", async () => {
  const value = await fixture();
  await writeCheckpoint(value.project, { eventId: "first", sessionId: "developer-session", taskIds: ["AT-001"], nextAction: "Original task only." });
  const missing = await inspectRecovery(value.project, { sessionId: "developer-session", taskId: "AT-002" });
  assert.equal(missing.status, "unavailable");
  assert.equal(missing.nextAction, undefined);
  const changed = await writeCheckpoint(value.project, { eventId: "second", sessionId: "developer-session", taskIds: ["AT-002"] });
  assert.equal(changed.checkpoint.nextAction, undefined);
});

test("checkpoint expected version rejects stale writers and preserves authored source pointers", async () => {
  const value = await fixture();
  const first = await writeCheckpoint(value.project, { eventId: "one", sessionId: "developer-session", taskIds: ["AT-001"], nextAction: "Inspect DESIGN.md before editing.", sourcePointers: [{ kind: "constraint", path: "DESIGN.md", section: "Never write production" }] });
  const updated = await writeCheckpoint(value.project, { eventId: "two", sessionId: "developer-session", taskIds: ["AT-001"] }, { expectedVersion: first.version });
  const stale = await writeCheckpoint(value.project, { eventId: "three", sessionId: "developer-session", taskIds: ["AT-001"], nextAction: "Stale overwrite." }, { expectedVersion: first.version });
  assert.equal(updated.status, "applied");
  assert.equal(stale.status, "conflict");
  assert.equal(JSON.parse(await readFile(first.path, "utf8")).nextAction, "Inspect DESIGN.md before editing.");
  assert.equal(updated.checkpoint.sourcePointers[0].kind, "constraint");
});

test("original source edits invalidate recovery evidence without rewriting authored decisions", async () => {
  const value = await fixture();
  const original = path.join(value.root, "DESIGN.md");
  await writeFile(original, "Approved: local records only. Rejected: transcript database.\n");
  const checkpoint = await writeCheckpoint(value.project, { eventId: "semantic", sessionId: "developer-session", taskIds: ["AT-001"],
    worktree: value.feature, revision: value.revision, evidenceRevision: value.revision, scope: { taskIds: ["AT-001"] },
    nextAction: "Implement the approved design constraints.", sourcePointers: [{ kind: "decision", path: original }] });
  const before = await inspectRecovery(value.project, { sessionId: "developer-session", taskId: "AT-001" });
  assert.equal(before.sourceEvidence[0].status, "current");
  assert.equal(before.task.owner, "TEAM-001");
  await writeFile(original, "Approved: changed requirement.\n");
  await writeCheckpoint(value.project, { eventId: "automatic", sessionId: "developer-session", taskIds: ["AT-001"], worktree: value.feature, revision: value.revision });
  const after = await inspectRecovery(value.project, { sessionId: "developer-session", taskId: "AT-001" });
  assert.equal(after.status, "stale");
  assert.equal(after.evidenceStatus, "stale");
  assert.equal(after.sourceEvidence[0].status, "stale");
  assert.equal(after.nextAction, checkpoint.checkpoint.nextAction);
});

test("semantic checkpoint rejects wrong actor and oversized packet before overwriting", async () => {
  const value = await fixture();
  const input = { eventId: "authored", sessionId: "developer-session", taskIds: ["AT-001"], nextAction: "Run owned tests." };
  assert.equal((await writeCheckpoint(value.project, input, { actorSessionId: "other-session", expectedVersion: 0 })).reason, "checkpoint_owner_required");
  assert.equal((await writeCheckpoint(value.project, { ...input, decisionNotes: "x".repeat(33000) }, { actorSessionId: "developer-session", expectedVersion: 0 })).reason, "checkpoint_too_large");
});

test("automatic checkpoint HEAD refresh cannot move verification authored at an older revision", async () => {
  const value = await fixture();
  await writeCheckpoint(value.project, { eventId: "verified-at-a", sessionId: "developer-session", taskIds: ["AT-001"], worktree: value.feature,
    revision: value.revision, evidenceRevision: value.revision, nextAction: "Use original verification only for revision A." });
  await writeFile(path.join(value.feature, "src/owned.js"), "export const updated = true;\n");
  execFileSync("git", ["add", "src/owned.js"], { cwd: value.feature });
  execFileSync("git", ["commit", "-qm", "new revision"], { cwd: value.feature });
  const next = execFileSync("git", ["rev-parse", "HEAD"], { cwd: value.feature, encoding: "utf8" }).trim();
  await writeCheckpoint(value.project, { eventId: "refresh-at-b", sessionId: "developer-session", taskIds: ["AT-001"], worktree: value.feature, revision: next });
  const result = await inspectRecovery(value.project, { sessionId: "developer-session", taskId: "AT-001", includeGit: true });
  assert.equal(result.evidenceRevision, value.revision);
  assert.equal(result.revision, next);
  assert.equal(result.evidenceStatus, "stale");
  assert.equal(result.status, "stale");
});

test("checkpoint receipts reject changed semantics and older replay without restoring obsolete next actions", async () => {
  const value = await fixture();
  const first = { eventId: "first-semantic", sessionId: "developer-session", taskIds: ["AT-001"], nextAction: "First action." };
  const second = { ...first, eventId: "second-semantic", nextAction: "Current action." };
  await writeCheckpoint(value.project, first);
  const latest = await writeCheckpoint(value.project, second);
  const changed = await writeCheckpoint(value.project, { ...second, nextAction: "Changed operation identity." });
  assert.equal(changed.status, "conflict");
  assert.equal(changed.reason, "operation_identity_reused");
  const replay = await writeCheckpoint(value.project, first);
  assert.equal(replay.status, "duplicate");
  const stored = JSON.parse(await readFile(latest.path, "utf8"));
  assert.equal(stored.nextAction, "Current action.");
  assert.equal(stored.version, latest.version);
});

test("bounded checkpoint index archives immutable receipts and retains old replay protection beyond 128 events", async () => {
  const value = await fixture();
  let latest;
  for (let index = 0; index < 128; index += 1) latest = await writeCheckpoint(value.project, { eventId: `event-${index}`, sessionId: "developer-session", taskIds: ["AT-001"], nextAction: `Action ${index}.` });
  const full = await writeCheckpoint(value.project, { eventId: "event-overflow", sessionId: "developer-session", taskIds: ["AT-001"], nextAction: "Continue without discarding old receipts." });
  assert.equal(full.status, "applied");
  const replay = await writeCheckpoint(value.project, { eventId: "event-0", sessionId: "developer-session", taskIds: ["AT-001"], nextAction: "Action 0." });
  assert.equal(replay.status, "duplicate");
  assert.equal(JSON.parse(await readFile(latest.path, "utf8")).nextAction, "Continue without discarding old receipts.");
  const changedReplay = await writeCheckpoint(value.project, { eventId: "event-0", sessionId: "developer-session", taskIds: ["AT-001"], nextAction: "Changed past operation." });
  assert.equal(changedReplay.reason, "operation_identity_reused");
  assert.ok(Object.keys(JSON.parse(await readFile(latest.path, "utf8")).operationReceipts).length <= 128);
  assert.ok((await readdir(path.join(value.project.paths.checkpoints, ".receipts"), { recursive: true })).some((entry) => entry.endsWith(".json")));
  assert.ok(Buffer.byteLength(await readFile(latest.path, "utf8")) <= 32768);
});

test("deadline while waiting for a lock never runs its mutation after the lock is released", async () => {
  const value = await fixture();
  const lock = path.join(value.project.paths.locks, "held.lock");
  await mkdir(lock, { recursive: true });
  const budget = createEventBudget(20);
  let mutated = false;
  try {
    await assert.rejects(withDirectoryLock(lock, { operationId: "waiter" }, async () => { mutated = true; }, { budget, timeoutMs: 500 }), /deadline|abort/i);
  } finally { budget.close(); }
  await rm(lock, { recursive: true });
  await new Promise((resolve) => setTimeout(resolve, 40));
  assert.equal(mutated, false);
  assert.deepEqual(await readdir(value.project.paths.locks), []);
});
