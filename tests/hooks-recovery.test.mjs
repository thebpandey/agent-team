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

function recoveryCanonical(overrides = {}) {
  const tasks = [{ id: "AT-001", status: "ready", owner: "none", parentId: null, taskType: "task", isTopLevelDelivery: true, dependencies: [] }];
  const run = { id: "recovery-run", ownerSessionId: "owner-session", ownerHost: "codex", ownershipEpoch: 1, mode: "finite", taskIds: ["AT-001"],
    teamLimit: 2, autoDeploy: true, batchSize: 2, source: "explicit_run",
    settingSources: Object.fromEntries(["mode", "taskIds", "teamLimit", "autoDeploy", "batchSize"].map((key) => [key, "explicit_run"])),
    paused: false, operationalVersion: 0, blockers: [], pendingDeliveryIds: [], deployedTaskIds: [], terminalClassification: "progress_possible" };
  return { tracker: { status: "current", fingerprint: "f".repeat(64) }, registry: { projectId: "project-1", projectOwner: "owner-session", projectOwnerHost: "codex", integrationOwner: "owner-session", ownershipEpoch: 1, teams: [] },
    state: { run, ownership: { epoch: 1 }, integration: { ownerSessionId: "owner-session", ownerHost: "codex", ownershipEpoch: 1 },
      release: { ownerSessionId: "owner-session", ownerHost: "codex", ownershipEpoch: 1 }, taskRuntime: {} }, tasks, deliveryEvidence: {}, git: { headRevision: "a".repeat(40) }, ...overrides };
}

test("recovery projects workers slots blockers decisions and pending tail without writes", async () => {
  const value = await fixture();
  const canonical = recoveryCanonical();
  canonical.state.run.blockers = [{ taskId: "AT-001", reason: "approval required" }];
  canonical.state.taskRuntime["AT-001"] = { compute: "unknown", writer: { host: "codex", sessionId: "worker-1" }, resumeWhen: "approval" };
  canonical.state.pendingOperations = { uncertain: { taskId: "AT-001", phase: "uncertain" } };
  const before = await readFile(value.project.paths.state);
  const result = await inspectRecovery(value.project, { canonical });
  assert.equal(result.workers.unknown.length, 1);
  assert.equal(result.slots.unknownOccupancy, 1);
  assert.equal(result.slots.safelyFree, 0);
  assert.deepEqual(result.blockers, [{ taskId: "AT-001", reason: "approval required" }]);
  assert.equal(result.pendingOperations[0].operationId, "uncertain");
  assert.equal(result.nextAction.kind, "reconcile_writer_liveness");
  assert.deepEqual(await readFile(value.project.paths.state), before);
});

test("assigned scoped work without qualified stopped runtime is unknown occupied capacity", async () => {
  const value = await fixture();
  const canonical = recoveryCanonical();
  canonical.tasks[0].owner = "TEAM-001";
  canonical.tasks[0].status = "in_progress";
  const result = await inspectRecovery(value.project, { canonical });
  assert.deepEqual(result.workers.unknown.map(({ taskId }) => taskId), ["AT-001"]);
  assert.equal(result.slots.unknownOccupancy, 1);
  assert.equal(result.slots.safelyFree, 0);
  assert.equal(result.run.classification, "blocked_tail");
  assert.equal(result.nextAction.kind, "reconcile_writer_liveness");
});

function manualReleaseCanonical() {
  const canonical = recoveryCanonical();
  const revision = "a".repeat(40);
  const actor = { ownerHost: "codex", ownerSessionId: "owner-session", ownershipEpoch: 1 };
  canonical.tasks[0].status = "completed";
  Object.assign(canonical.state.run, { autoDeploy: false, batchSize: 2, pendingDeliveryIds: ["AT-001"], terminalClassification: "finite_exhausted" });
  const authority = { status: "authorized", source: "fixture", target: "origin/main", revision, taskIds: ["AT-001"], ...actor };
  canonical.state.integration = { taskIds: ["AT-001"], ...actor };
  canonical.state.release = { ...actor, authorized: true, target: "origin/main", process: "gh-release", runMode: "manual", autoDeploy: false,
    expectedRevision: revision, trackerFingerprint: canonical.tracker.fingerprint, evidenceAt: "2026-09-06T12:00:00.000Z", taskIds: ["AT-001"],
    batchId: "batch-1", batch: { id: "batch-1", taskIds: ["AT-001"] }, hold: false, projectPaused: false, remoteMainDeploys: false,
    recordedEvidence: { taskIds: ["AT-001"], selectedTaskIds: ["AT-001"] },
    authorization: { source: "fixture", target: "origin/main", process: "gh-release", scope: "batch-1", grantedAt: "2026-09-06", observedAt: "2026-09-06T12:00:00.000Z", revision, taskIds: ["AT-001"], ...actor },
    run: { id: "batch-run", mode: "manual", taskIds: ["AT-001"], paused: false },
    artifact: { id: "artifact", revision, taskIds: ["AT-001"], sha256: "b".repeat(64) },
    integration: { status: "passed", revision, taskIds: ["AT-001"], remoteMainDeploys: false },
    verification: { status: "passed", revision, taskIds: ["AT-001"] }, preview: { required: false, status: "not_required", revision },
    delta: { status: "clean", revision, taskIds: ["AT-001"] }, recovery: { status: "verified", artifactId: "known", action: "restore" } };
  canonical.deliveryEvidence["AT-001"] = { taskId: "AT-001", sourceRevision: revision, revision, integratedRevision: revision,
    completion: { taskId: "AT-001", status: "passed", sourceRevision: revision },
    integration: { taskId: "AT-001", status: "passed", sourceRevision: revision, boundaryRevision: revision, ...actor },
    review: { taskId: "AT-001", status: "passed", revision }, checks: [{ name: "unit", status: "passed" }],
    preview: { taskId: "AT-001", status: "not_required", required: false, revision, ...actor },
    target: { taskId: "AT-001", status: "authorized", target: "origin/main", revision, authority, ...actor },
    recovery: { taskId: "AT-001", status: "ready", artifact: "known", action: "restore", revision, ...actor } };
  return canonical;
}

test("manual release availability requires every exact policy gate", async () => {
  const value = await fixture();
  const exact = manualReleaseCanonical();
  assert.equal((await inspectRecovery(value.project, { canonical: exact, now: new Date("2026-09-06T12:01:00Z") })).nextAction.kind, "manual_release_available");
  for (const mutate of [
    (value) => { value.state.release.preview.status = "pending"; },
    (value) => { value.state.release.authorization.taskIds = ["AT-002"]; },
    (value) => { value.state.release.evidenceAt = "2026-09-06T11:00:00.000Z"; },
    (value) => { value.state.release.process = "npm"; },
  ]) {
    const changed = structuredClone(exact); mutate(changed);
    assert.equal((await inspectRecovery(value.project, { canonical: changed, now: new Date("2026-09-06T12:01:00Z") })).nextAction.kind, "resolve_target_decision");
  }
});

test("recovery no-false-stop table chooses one deterministic next action", async () => {
  const value = await fixture();
  const cases = [
    ["no run", { ...recoveryCanonical(), state: {} }, "start_or_reconcile_run"],
    ["eligible", recoveryCanonical(), "dispatch_or_refill"],
    ["active", recoveryCanonical(), "continue_or_supervise"],
    ["unknown", recoveryCanonical(), "reconcile_writer_liveness"],
  ];
  cases[2][1].tasks[0].status = "in_progress";
  cases[2][1].state.taskRuntime["AT-001"] = { compute: "active", writer: { host: "codex", sessionId: "worker" } };
  cases[3][1].state.taskRuntime["AT-001"] = { compute: "unknown", writer: { host: "codex", sessionId: "worker" } };
  for (const [name, canonical, expected] of cases) {
    const result = await inspectRecovery(value.project, { canonical });
    assert.equal(result.nextAction.kind, expected, name);
  }
});

test("recovery preserves checkpoint facts without letting them replace canonical run state", async () => {
  const value = await fixture();
  await writeCheckpoint(value.project, { eventId: "stale-scope", sessionId: "developer-session", taskIds: ["AT-999"], nextAction: "obsolete" });
  const result = await inspectRecovery(value.project, { canonical: recoveryCanonical(), sessionId: "developer-session" });
  assert.deepEqual(result.checkpoint.taskIds, ["AT-999"]);
  assert.deepEqual(result.run.taskIds, ["AT-001"]);
  assert.notEqual(result.nextAction.kind, "complete");
});

test("recovery journal and generation mismatch stay unavailable and byte identical", async () => {
  const value = await fixture();
  const before = await readFile(value.project.paths.state);
  let reads = 0;
  const result = await inspectRecovery(value.project, { loadCanonicalState: async (_project, options) => {
    reads += 1; assert.equal(options.readOnly, true); throw new Error("owner_generation_mismatch");
  } });
  assert.equal(reads, 1);
  assert.deepEqual(result, { status: "unavailable", reason: "owner_generation_mismatch", recurring: { frequency: "none", scheduledExecutions: 0 } });
  assert.deepEqual(await readFile(value.project.paths.state), before);
});

test("recovery selects the requested task/session/worktree despite a newer unrelated checkpoint", async () => {
  const value = await fixture();
  await writeCheckpoint(value.project, { eventId: "wanted", sessionId: "developer-session", taskIds: ["AT-001"], worktree: value.feature, revision: value.revision, nextAction: "Run the owned unit tests.",
    sourcePointers: [{ kind: "decision", path: "DESIGN.md", section: "Storage choice" }],
    pendingOperations: [{ operationId: "release-1", status: "unknown", action: "reconcile" }] }, { now: new Date("2026-09-08T10:00:00Z") });
  await writeCheckpoint(value.project, { eventId: "other", sessionId: "other-session", taskIds: ["AT-999"], worktree: value.root, nextAction: "Do unrelated work." }, { now: new Date("2026-09-08T10:01:00Z") });
  const result = await inspectRecovery(value.project, { sessionId: "developer-session", taskId: "AT-001", now: new Date("2026-09-08T10:02:00Z") });
  assert.equal(result.sessionId, "developer-session");
  assert.equal(result.checkpoint.nextAction, "Run the owned unit tests.");
  assert.equal(result.pendingOperations[0].status, "unknown");
  assert.deepEqual(result.sourcePointers, [{ kind: "decision", path: "DESIGN.md", section: "Storage choice" }]);
  assert.equal(result.revision, value.revision);
});

test("a requested task cannot inherit an unrelated checkpoint or previous task's authored notes", async () => {
  const value = await fixture();
  await writeCheckpoint(value.project, { eventId: "first", sessionId: "developer-session", taskIds: ["AT-001"], nextAction: "Original task only." });
  const missing = await inspectRecovery(value.project, { sessionId: "developer-session", taskId: "AT-002" });
  assert.equal(missing.status, "unavailable");
  assert.equal(missing.checkpoint.nextAction, undefined);
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
  assert.equal(after.checkpoint.nextAction, checkpoint.checkpoint.nextAction);
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

test("interrupted archive publication leaves no partial immutable receipt and later events keep replay protection", async () => {
  const value = await fixture();
  let latest;
  for (let index = 0; index < 128; index += 1) latest = await writeCheckpoint(value.project, { eventId: `publication-${index}`, sessionId: "developer-session", taskIds: ["AT-001"], nextAction: `Action ${index}.` });
  const before = await readFile(latest.path, "utf8");
  const wanted = { eventId: "publication-next", sessionId: "developer-session", taskIds: ["AT-001"], nextAction: "Continue after interruption." };
  await assert.rejects(writeCheckpoint(value.project, wanted, { receiptFilesystem: { writeFile: async (file, _source, options) => {
    await writeFile(file, '{"signature":', options);
    throw Object.assign(new Error("interrupted archive write"), { code: "ABORT_ERR" });
  } } }), /interrupted archive/);
  assert.equal(await readFile(latest.path, "utf8"), before);
  const receiptRoot = path.join(value.project.paths.checkpoints, ".receipts");
  assert.equal((await readdir(receiptRoot, { recursive: true })).some((name) => name.endsWith(".json") || name.endsWith(".tmp")), false);
  assert.equal((await writeCheckpoint(value.project, wanted)).status, "applied");
  const relative = (await readdir(receiptRoot, { recursive: true })).find((name) => name.endsWith(".json"));
  const archive = path.join(receiptRoot, relative);
  const immutable = await readFile(archive, "utf8");
  // Simulate publication succeeding before an interrupted hot-index replacement.
  await writeFile(latest.path, before);
  assert.equal((await writeCheckpoint(value.project, wanted)).status, "applied");
  assert.equal(await readFile(archive, "utf8"), immutable);
  assert.equal((await writeCheckpoint(value.project, { eventId: "publication-0", sessionId: "developer-session", taskIds: ["AT-001"], nextAction: "Action 0." })).status, "duplicate");
  assert.equal((await writeCheckpoint(value.project, { eventId: "publication-0", sessionId: "developer-session", taskIds: ["AT-001"], nextAction: "Changed replay." })).reason, "operation_identity_reused");
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
