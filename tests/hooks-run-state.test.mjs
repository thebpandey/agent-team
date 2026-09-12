import assert from "node:assert/strict";
import { execFileSync } from "node:child_process";
import { mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import test from "node:test";

import { loadCanonicalState } from "../hooks/lib/canonical-state.mjs";
import { resolveProject } from "../hooks/lib/project.mjs";
import {
  classifyRun, effectiveRunFingerprint, readRunDecision, reconcileRun,
  selectReleaseBatch, startRun, validateEffectiveRun,
} from "../hooks/lib/run-state.mjs";
import { policyFixture } from "./hook-test-helpers.mjs";

const temporary = [];
test.afterEach(async () => Promise.all(temporary.splice(0).map((item) => rm(item, { recursive: true, force: true }))));

const sources = (source = "explicit_run") => Object.fromEntries(["mode", "taskIds", "teamLimit", "autoDeploy", "batchSize"].map((key) => [key, source]));
const task = (id, status = "ready", extra = {}) => ({ id, owner: "none", status, dependencies: [], dependencyEvidence: "current", parentId: null,
  taskType: "task", isEpic: false, isSubtask: false, isTopLevelDelivery: true, ...extra });
const run = (extra = {}) => ({ id: "run-1", ownerSessionId: "owner-session", ownerHost: "codex", ownershipEpoch: 1, mode: "finite",
  taskIds: ["AT-001"], teamLimit: 2, autoDeploy: true, batchSize: 2, source: "explicit_run", settingSources: sources(), paused: false,
  operationalVersion: 1, blockers: [], pendingDeliveryIds: [], deployedTaskIds: [], terminalClassification: "progress_possible", ...extra });
const canonical = (runValue, tasks, extra = {}) => ({ state: { run: runValue, ownership: { epoch: 1 }, integration: { ownerSessionId: "owner-session", ownerHost: "codex", ownershipEpoch: 1 },
  release: { ownerSessionId: "owner-session", ownerHost: "codex", ownershipEpoch: 1,
    authorization: { ownerSessionId: "owner-session", ownerHost: "codex", ownershipEpoch: 1 } } }, registry: { projectOwner: "owner-session", projectOwnerHost: "codex", ownershipEpoch: 1 },
  tracker: { status: "current", fingerprint: "f".repeat(64) }, tasks, deliveryEvidence: {}, git: { headRevision: "a".repeat(40) }, ...extra });

test("effective run validation is closed complete and fingerprint stable", () => {
  const valid = run();
  assert.equal(validateEffectiveRun(valid, [task("AT-001")]), undefined);
  assert.match(effectiveRunFingerprint(valid), /^[a-f0-9]{64}$/);
  assert.equal(effectiveRunFingerprint(valid), effectiveRunFingerprint(structuredClone(valid)));
  for (const invalid of [
    { ...valid, extra: true }, { ...valid, ownerHost: "claude" }, { ...valid, ownershipEpoch: 0 }, { ...valid, taskIds: [] },
    { ...valid, teamLimit: 65 }, { ...valid, batchSize: 0 }, { ...valid, blockers: [{ taskId: "AT-001", reason: "" }] },
    { ...valid, pendingDeliveryIds: ["AT-001"], deployedTaskIds: ["AT-001"] },
  ]) assert.equal(validateEffectiveRun(invalid, [task("AT-001")]), "invalid_effective_run");
  assert.notEqual(effectiveRunFingerprint(valid), effectiveRunFingerprint({ ...valid, taskIds: ["AT-001", "AT-002"] }));
});

test("tracker hierarchy excludes epics subtasks and unknown parents", () => {
  const tasks = [task("EPIC", "ready", { taskType: "epic", isEpic: true, isTopLevelDelivery: false }),
    task("CHILD", "ready", { parentId: "EPIC", isSubtask: true, isTopLevelDelivery: false }), task("TOP")];
  const value = run({ taskIds: tasks.map(({ id }) => id) });
  assert.equal(validateEffectiveRun(value, tasks), undefined);
  assert.deepEqual(classifyRun(canonical(value, tasks)).eligibleTaskIds, ["TOP"]);
  assert.equal(classifyRun(canonical(value, [task("ORPHAN", "ready", { parentId: "MISSING", isSubtask: true, isTopLevelDelivery: false })],
    { state: { ...canonical(value, tasks).state, run: run({ taskIds: ["ORPHAN"] }) } })).kind, "unknown");
});

test("classification recomputes every disjoint run state from canonical facts", () => {
  assert.equal(classifyRun(canonical(run({ paused: true }), [task("AT-001")])).kind, "paused");
  assert.equal(classifyRun(canonical(run(), [task("AT-001")])).kind, "progress_possible");
  assert.equal(classifyRun(canonical(run(), [task("AT-001", "done")])).kind, "unreconciled_completion");
  assert.equal(classifyRun(canonical(run({ deployedTaskIds: ["AT-001"] }), [task("AT-001", "done")])).kind, "finite_exhausted");
  assert.equal(classifyRun(canonical(run({ mode: "continuous", deployedTaskIds: ["AT-001"] }), [task("AT-001", "closed")])).kind, "continuous_scope_exhausted");
  assert.equal(classifyRun(canonical(run({ blockers: [{ taskId: "AT-001", reason: "Needs authority." }] }), [task("AT-001", "blocked")])).kind, "blocked_tail");
  assert.equal(classifyRun(canonical({ ...run(), extra: true }, [task("AT-001")])).kind, "unknown");
});

test("qualified writer liveness never aliases equal sessions across hosts", () => {
  const active = task("AT-001", "in_progress", { owner: "TEAM-1" });
  assert.equal(classifyRun(canonical(run(), [active]), { writerLiveness: { "AT-001": { host: "claude-code", sessionId: "same", status: "unknown" } } }).kind, "blocked_tail");
  assert.equal(classifyRun(canonical(run(), [active]), { writerLiveness: { "AT-001": { host: "codex", sessionId: "same", status: "active" } } }).kind, "progress_possible");
  assert.equal(classifyRun(canonical(run(), [task("AT-001", "ready", { owner: "TEAM-1" })])).kind, "blocked_tail");
});

async function fixture() {
  const root = await mkdtemp(path.join(os.tmpdir(), "agent-team-run-state-"));
  temporary.push(root, `${root}-feature`, `${root}-remote`);
  const value = await policyFixture(root, { qualifiedOwnership: true });
  const project = await resolveProject(root);
  await writeFile(project.paths.tasks, "| ID | Requirement / acceptance | Owner | Depends on | Status | Revision / evidence | Next action | Type | Parent ID |\n| --- | --- | --- | --- | --- | --- | --- | --- | --- |\n| AT-001 | Delivery | none | none | ready | none | Claim. | task | |\n");
  const current = await loadCanonicalState(project);
  return { ...value, project, current, nativeIdentity: { host: "codex", sessionId: "owner-session", observed: true, cwd: root, ownershipEpoch: 1 } };
}

const proposed = (source = "explicit_run") => ({ id: "run-1", mode: "finite", taskIds: ["AT-001"], teamLimit: 2, autoDeploy: false,
  batchSize: 2, source, settingSources: sources(source) });

test("start persists immutable effective choices and authenticated owner generation", async () => {
  const value = await fixture();
  const request = { operationId: "run-start-1", expectedTrackerFingerprint: value.current.tracker.fingerprint, reason: "Start the approved run.", run: proposed() };
  const result = await startRun(value.project, request, { actorSessionId: "owner-session", expectedVersion: value.current.state.stateVersion ?? 0, nativeIdentity: value.nativeIdentity });
  assert.equal(result.status, "applied");
  assert.deepEqual({ host: result.result.run.ownerHost, session: result.result.run.ownerSessionId, epoch: result.result.run.ownershipEpoch }, { host: "codex", session: "owner-session", epoch: 1 });
  assert.equal((await startRun(value.project, request, { actorSessionId: "owner-session", expectedVersion: value.current.state.stateVersion ?? 0, nativeIdentity: value.nativeIdentity })).status, "duplicate");
});

test("start and reconcile reject stale authority fingerprints and scope without writes", async () => {
  const value = await fixture();
  const before = await readFile(value.project.paths.state);
  const start = { operationId: "stale-tracker", expectedTrackerFingerprint: "c".repeat(64), reason: "Start the approved run.", run: proposed() };
  assert.equal((await startRun(value.project, start, { actorSessionId: "owner-session", expectedVersion: 0, nativeIdentity: value.nativeIdentity })).reason, "stale_tracker");
  assert.deepEqual(await readFile(value.project.paths.state), before);
});

test("legacy reconciliation fills provenance once and preserves runtime facts", async () => {
  const value = await fixture();
  const legacy = { mode: "finite", taskIds: ["AT-001"], paused: false, blockers: [], pendingDeliveryIds: [], deployedTaskIds: [] };
  await writeFile(value.project.paths.state, JSON.stringify({ ...value.current.state, run: legacy }, null, 2));
  const current = await loadCanonicalState(value.project);
  const request = { operationId: "legacy-reconcile", expectedTrackerFingerprint: current.tracker.fingerprint, expectedRunFingerprint: effectiveRunFingerprint(legacy),
    authoritativeSource: "compatibility_migration", reason: "Populate explicit legacy run choices.", affectedTaskIds: ["AT-001"], run: proposed("compatibility_migration") };
  const result = await reconcileRun(value.project, request, { actorSessionId: "owner-session", expectedVersion: current.state.stateVersion ?? 0, nativeIdentity: value.nativeIdentity });
  assert.equal(result.status, "applied");
  assert.equal(result.result.deploymentHeld, true);
  assert.deepEqual(result.result.previousRun, legacy);
  assert.equal(result.result.run.ownerHost, "codex");
});

test("ordinary reconciliation preserves historical run provenance", async () => {
  const value = await fixture();
  const existing = run({ autoDeploy: false });
  await writeFile(value.project.paths.state, JSON.stringify({ ...value.current.state, run: existing }, null, 2));
  const current = await loadCanonicalState(value.project);
  const request = { operationId: "ordinary-reconcile", expectedTrackerFingerprint: current.tracker.fingerprint, expectedRunFingerprint: effectiveRunFingerprint(existing),
    authoritativeSource: "explicit_run", reason: "Confirm explicit effective choices.", affectedTaskIds: ["AT-001"], run: proposed() };
  const result = await reconcileRun(value.project, request, { actorSessionId: "owner-session", expectedVersion: current.state.stateVersion ?? 0, nativeIdentity: value.nativeIdentity });
  assert.equal(result.status, "applied");
  assert.deepEqual([result.result.run.ownerHost, result.result.run.ownerSessionId, result.result.run.ownershipEpoch], ["codex", "owner-session", 1]);
});

function joinedEvidence(id, revision = "a".repeat(40)) {
  const actor = { ownerHost: "codex", ownerSessionId: "owner-session", ownershipEpoch: 1 };
  return { taskId: id, sourceRevision: revision, revision, integratedRevision: revision, completion: { status: "passed" }, integration: { status: "passed", ...actor },
    review: { status: "passed" }, checks: [{ name: "unit", status: "passed" }], preview: { status: "not_required", required: false, revision, ...actor },
    target: { status: "authorized", revision, ...actor }, recovery: { status: "ready", revision, ...actor } };
}

test("delivery evidence joins exact passed lineage and current generation", async () => {
  const value = await fixture();
  const revision = execFileSync("git", ["rev-parse", "HEAD"], { cwd: value.root, encoding: "utf8" }).trim();
  const actor = { ownerHost: "codex", ownerSessionId: "owner-session", ownershipEpoch: 1 };
  const receipt = (status, extra = {}) => ({ taskId: "AT-001", status, ...extra });
  const state = { ...value.current.state, release: { ...value.current.state.release,
    authorization: { ...value.current.state.release.authorization, ...actor } }, deliveryReceipts: {
    completion: { "AT-001": receipt("passed", { sourceRevision: revision }) }, review: { "AT-001": receipt("passed", { revision }) },
    checks: { "AT-001": receipt("passed", { revision, results: [{ name: "unit", status: "passed" }] }) },
    integration: { "AT-001": receipt("passed", { sourceRevision: revision, boundaryRevision: revision, ...actor }) },
    preview: { "AT-001": receipt("not_required", { revision, required: false, ...actor }) }, target: { "AT-001": receipt("authorized", { revision, target: "origin/main", ...actor }) },
    recovery: { "AT-001": receipt("ready", { revision, artifact: "tag", action: "rollback", ...actor }) },
  } };
  await writeFile(value.project.paths.state, JSON.stringify(state, null, 2));
  const loaded = await loadCanonicalState(value.project);
  assert.equal(loaded.deliveryEvidence["AT-001"].integratedRevision, revision);
  state.deliveryReceipts.integration["AT-001"].status = "failed";
  await writeFile(value.project.paths.state, JSON.stringify(state, null, 2));
  assert.equal((await loadCanonicalState(value.project)).deliveryEvidence["AT-001"], undefined);
});

test("release batches are oldest first full or terminally underfilled", () => {
  const tasks = [task("AT-001", "done"), task("AT-002", "done")];
  const evidence = { "AT-001": joinedEvidence("AT-001"), "AT-002": joinedEvidence("AT-002") };
  const full = run({ taskIds: ["AT-001", "AT-002"], pendingDeliveryIds: ["AT-002", "AT-001"], batchSize: 2 });
  assert.deepEqual(selectReleaseBatch(canonical(full, tasks, { deliveryEvidence: evidence }), { kind: "progress_possible", eligibleTaskIds: [], blockedTaskIds: [] }), ["AT-002", "AT-001"]);
  const one = run({ pendingDeliveryIds: ["AT-001"], batchSize: 2 });
  const view = canonical(one, [task("AT-001", "done")], { deliveryEvidence: { "AT-001": evidence["AT-001"] } });
  assert.deepEqual(selectReleaseBatch(view, { kind: "finite_exhausted", eligibleTaskIds: [], blockedTaskIds: [] }), ["AT-001"]);
  assert.deepEqual(selectReleaseBatch(view, { kind: "progress_possible", eligibleTaskIds: ["AT-002"], blockedTaskIds: [] }), []);
  const stale = structuredClone(view);
  stale.deliveryEvidence["AT-001"].target.ownershipEpoch = 0;
  assert.deepEqual(selectReleaseBatch(stale, { kind: "finite_exhausted", eligibleTaskIds: [], blockedTaskIds: [] }), []);
});

test("run decision is a pure held projection of one canonical snapshot", () => {
  const input = canonical(run({ autoDeploy: false }), [task("AT-001")]);
  const before = structuredClone(input);
  const decision = readRunDecision(input);
  assert.equal(decision.status, "available");
  assert.equal(decision.deploymentHeld, true);
  assert.deepEqual(decision.holdReasons, ["auto_deploy_disabled"]);
  assert.deepEqual(input, before);
});
