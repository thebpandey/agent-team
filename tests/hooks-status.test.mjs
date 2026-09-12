import assert from "node:assert/strict";
import test from "node:test";

import { createStatusModel, readStatus } from "../hooks/lib/status.mjs";

const project = { projectId: "project-1", root: "/project", active: true };

test('status exposes observed mutation versions and exact empty ownership without turning unknown into zero', () => {
  const source = canonical();
  source.state.stateVersion = 4;
  const model = createStatusModel({ ...project, setup: { version: 7 } }, source);
  assert.deepEqual(model.versions, { operational: 4, setup: 7 });
  const unassigned = model.tasks.find(({ id }) => id === 'READY');
  assert.equal(unassigned.owner, 'unassigned');
  assert.equal(unassigned.canonicalOwner, '');
  assert.deepEqual(createStatusModel(project, canonical({ tracker: { status: 'unavailable' } })).versions, { operational: null, setup: null });
});

test('readStatus forwards one abortable deadline through project and canonical reads', async () => {
  const controller = new AbortController();
  let seen;
  const result = await readStatus('/project', {
    signal: controller.signal, deadlineMs: 20,
    resolveProject: async (_cwd, { budget } = {}) => { assert.ok(budget, 'project lookup needs the shared budget'); seen = budget; return project; },
    loadCanonicalState: async (_project, { budget } = {}) => {
      assert.equal(budget, seen);
      return new Promise((_resolve, reject) => budget.signal.addEventListener('abort', () => reject(new Error('read cancelled')), { once: true }));
    },
  });
  assert.equal(result.freshness.status, 'unavailable');
  assert.equal(seen.signal.aborted, true);
});

function canonical(overrides = {}) {
  return {
    tracker: { kind: "tasks", id: "TASKS.md", path: "TASKS.md", status: "current", fingerprint: "abc", observedAt: "2026-09-08T12:00:00.000Z" },
    registry: {
      projectId: "project-1",
      projectOwner: "owner",
      integrationOwner: "integrator",
      teams: [{ "team id": "T1", name: "Platform", session: "agent-a", status: "in_progress", model: "gpt", effort: "high", tasks: "PARENT, CHILD", "last update": "2026-09-08T12:00:00.000Z" }],
    },
    state: {
      integration: { status: "passed" }, release: { status: "pending" },
      taskRuntime: { PARKED: { compute: "parked", explicitPause: false, checkpointPath: ".agent-team/checkpoints/parked.json", resumeWhen: "READY", writer: { host: "local", pid: 44 } } },
      run: { paused: true, id: "run-1", taskIds: ["PARENT", "CHILD"] },
    },
    tasks: [
      { id: "PARENT", "requirement / acceptance": "Parent", owner: "T1", "depends on": "none", status: "in_progress", priority: "P1", "next action": "Delegate child" },
      { id: "CHILD", "requirement / acceptance": "Child", owner: "T1", "depends on": "PARENT", status: "completed", priority: "P1", parent: "PARENT" },
      { id: "READY", "requirement / acceptance": "Ready", owner: "", "depends on": "none", status: "ready", priority: "P2" },
      { id: "PARKED", "requirement / acceptance": "Parked", owner: "T1", "depends on": "READY", status: "parked", priority: "P2" },
      { id: "CANCEL", "requirement / acceptance": "Cancelled", owner: "T1", "depends on": "none", status: "cancelled", priority: "P3" },
      { id: "DEFER", "requirement / acceptance": "Deferred", owner: "T1", "depends on": "none", status: "approved_deferred", priority: "P3" },
    ],
    ...overrides,
  };
}

function effectiveCanonical(overrides = {}) {
  const task = { id: "AT-001", title: "Release", status: "completed", owner: "TEAM-001", parentId: null,
    taskType: "task", isTopLevelDelivery: true, dependencies: [] };
  const run = { id: "run-1", ownerSessionId: "owner", ownerHost: "codex", ownershipEpoch: 3, mode: "finite", taskIds: ["AT-001"],
    teamLimit: 2, autoDeploy: false, batchSize: 2, source: "explicit_run",
    settingSources: Object.fromEntries(["mode", "taskIds", "teamLimit", "autoDeploy", "batchSize"].map((key) => [key, "explicit_run"])),
    paused: false, operationalVersion: 4, blockers: [], pendingDeliveryIds: [], deployedTaskIds: [], terminalClassification: "finite_exhausted" };
  return {
    tracker: { kind: "tasks", id: "TASKS.md", path: "TASKS.md", status: "current", fingerprint: "f".repeat(64) },
    registry: { projectId: "project-1", projectOwner: "owner", projectOwnerHost: "codex", integrationOwner: "owner", integrationOwnerHost: "codex", ownershipEpoch: 3, teams: [] },
    state: { stateVersion: 4, ownership: { epoch: 3 }, run,
      integration: { ownerSessionId: "owner", ownerHost: "codex", ownershipEpoch: 3, remoteName: "origin", remoteRef: "refs/heads/main", remoteMainDeploys: false },
      release: { ownerSessionId: "owner", ownerHost: "codex", ownershipEpoch: 3, target: "vercel", process: "vercel", hold: true } },
    tasks: [task], deliveryEvidence: { "AT-001": { taskId: "AT-001", target: { taskId: "AT-001", status: "authorized", target: "refs/heads/main",
      ownerSessionId: "owner", ownerHost: "codex", ownershipEpoch: 3,
      authority: { status: "authorized", target: "refs/heads/main", taskIds: ["AT-001"], ownerSessionId: "owner", ownerHost: "codex", ownershipEpoch: 3 } } } },
    git: { headRevision: "a".repeat(40) },
    ...overrides,
  };
}

test("status exposes immutable effective run target and migration provenance", () => {
  const source = effectiveCanonical();
  const model = createStatusModel({ ...project, setup: { initialization: { handoff: { generatedBy: { skill: "project-kickoff" }, testedAgainst: { skill: "agent-team" } } } } }, source);
  assert.equal(model.stateVersion, 4);
  assert.deepEqual(model.project, { id: "project-1", root: "/project", ownerSessionId: "owner", ownerHost: "codex", ownershipEpoch: 3 });
  assert.equal(model.run.status, "available");
  assert.match(model.run.fingerprint, /^[a-f0-9]{64}$/);
  assert.deepEqual(model.run.taskIds, ["AT-001"]);
  assert.equal(model.targets["origin/main"].status, "ready");
  assert.equal(model.targets.vercel.status, "held");
  assert.deepEqual(model.provenance.generatedBy, { skill: "project-kickoff" });
  assert.throws(() => { model.run.taskIds.push("AT-002"); }, TypeError);
});

test("status separates loaded runtime source candidate handoff and readiness", () => {
  const model = createStatusModel(project, effectiveCanonical(), {
    authenticatedLoadedRuntime: { status: "current", version: "7.2.0" },
    observedSourceCandidate: { status: "current", version: "7.2.1", authoritative: false },
    currentReadinessEvidence: { status: "passed", worker: "fresh" },
  });
  assert.deepEqual(model.provenance.loadedRuntime, { status: "current", version: "7.2.0" });
  assert.deepEqual(model.provenance.sourceCandidate, { status: "current", version: "7.2.1", authoritative: false });
  assert.deepEqual(model.provenance.readiness, { status: "passed", worker: "fresh" });
  assert.notDeepEqual(model.provenance.loadedRuntime, model.provenance.sourceCandidate);
});

test("status reports local-only and target-required without globalizing holds", () => {
  const source = effectiveCanonical();
  source.state.run.autoDeploy = true;
  source.tracker = { kind: "beads", id: "beads:/project/.beads", path: "/project/.beads", status: "current", fingerprint: "f".repeat(64) };
  delete source.state.release.target;
  const model = createStatusModel({ ...project, setup: { tracker: { kind: "beads", executable: "/bin/bd" } }, tracker: { kind: "beads", path: "/project/.beads" } }, source);
  assert.deepEqual(model.targets.deployment, { kind: "deployment", status: "enabled_but_held", reason: "target_required", taskIds: ["AT-001"] });
  assert.equal(model.targets.database.status, "local_only");
  assert.equal(model.targets["origin/main"].status, "ready");
  assert.equal(model.targets.vercel, undefined);
  assert.equal(model.targets.dns, undefined);
});

test("status target readiness requires exact observed authority and database disposition", () => {
  const remoteOnly = effectiveCanonical();
  remoteOnly.deliveryEvidence = {};
  const remote = createStatusModel(project, remoteOnly);
  assert.notEqual(remote.targets["origin/main"].status, "ready");
  assert.deepEqual(remote.targets["origin/main"].taskIds, []);

  const production = effectiveCanonical();
  production.state.database = { authorized: false, environment: "production", productionApproved: false,
    inventoryAt: "2026-09-08T12:00:00.000Z", recovery: { verifiedAt: "2026-09-08T12:00:00.000Z", kind: "backup" },
    dryRun: { capability: "supported", verifiedAt: "2026-09-08T12:00:00.000Z" } };
  const held = createStatusModel(project, production);
  assert.equal(held.targets["database:production"].status, "held");

  const local = effectiveCanonical();
  local.tracker = { kind: "beads", id: "beads:/project/.beads", path: "/project/.beads", status: "current", fingerprint: "f".repeat(64) };
  assert.equal(createStatusModel({ ...project, setup: { tracker: { kind: "beads", executable: "/bin/bd" } },
    tracker: { kind: "beads", path: "/project/.beads" } }, local).targets.database.status, "local_only");
});

test("status keeps task publication authority independent from an incomplete aggregate release target", () => {
  const incomplete = effectiveCanonical();
  incomplete.state.release = { ...incomplete.state.release, authorized: true, hold: false };
  assert.notEqual(createStatusModel(project, incomplete).targets.vercel.status, "ready");

  const collision = effectiveCanonical();
  collision.state.release = { ...collision.state.release, target: "origin/main", process: "vercel", hold: true };
  const targets = createStatusModel(project, collision).targets;
  assert.equal(targets["origin/main"].status, "ready");
  assert.deepEqual(targets["release:origin/main:vercel"], {
    kind: "release", process: "vercel", status: "held", reason: "release_hold", taskIds: [],
  });
});

test("status derives the local Beads store from selected tracker facts and keeps application database authority separate", () => {
  const source = effectiveCanonical({ tracker: { kind: "beads", id: "beads:/project/.beads", path: "/project/.beads", status: "current", fingerprint: "f".repeat(64) } });
  const selected = { ...project, setup: { tracker: { kind: "beads", executable: "/bin/bd" } }, tracker: { kind: "beads", path: "/project/.beads" } };
  assert.deepEqual(createStatusModel(selected, source).targets.database, {
    kind: "tracker_database", status: "local_only", reason: null, taskIds: [],
  });

  source.state.database = { ownerSessionId: "owner", ownerHost: "codex", ownershipEpoch: 3, authorized: false,
    environment: "production", productionApproved: false, inventoryAt: "2026-09-08T12:00:00.000Z",
    recovery: { verifiedAt: "2026-09-08T12:00:00.000Z", kind: "backup" },
    dryRun: { capability: "supported", verifiedAt: "2026-09-08T12:00:00.000Z" } };
  const targets = createStatusModel(selected, source).targets;
  assert.equal(targets.database.status, "local_only");
  assert.deepEqual(targets["database:production"], {
    kind: "database", environment: "production", authorized: false, productionApproved: false, status: "held",
    reason: "database_authority_required", taskIds: [], inventoryAt: "2026-09-08T12:00:00.000Z",
    recovery: { verifiedAt: "2026-09-08T12:00:00.000Z", kind: "backup" },
    dryRun: { capability: "supported", verifiedAt: "2026-09-08T12:00:00.000Z" },
  });
});

test("status exposes the same qualified run occupancy and decision facts as recovery", () => {
  const source = effectiveCanonical();
  source.tasks = [
    { id: "AT-001", title: "Owned", owner: "TEAM-001", status: "in_progress", parentId: null, taskType: "task", isTopLevelDelivery: true, dependencies: [] },
    { id: "AT-002", title: "Ready", owner: "", status: "ready", parentId: null, taskType: "task", isTopLevelDelivery: true, dependencies: [] },
  ];
  source.state.run = { ...source.state.run, taskIds: ["AT-001", "AT-002"], teamLimit: 2, batchSize: 2 };
  source.state.taskRuntime = { "AT-001": { compute: "active", writer: { host: "local", sessionId: "worker" } } };
  source.state.pendingDecisions = [{ id: "target", taskIds: ["AT-002"] }];
  source.state.pendingOperations = { uncertain: { taskId: "AT-001", status: "unknown" } };
  const model = createStatusModel(project, source);
  assert.deepEqual(model.workers.unknown.map(({ taskId }) => taskId), ["AT-001"]);
  assert.deepEqual(model.slots, { teamLimit: 2, occupied: 1, reviewReservation: 1, safelyFree: 0, unknownOccupancy: 1 });
  assert.deepEqual(model.run.eligibleTaskIds, ["AT-002"]);
  assert.deepEqual(model.run.blockedTaskIds, ["AT-001"]);
  assert.equal(model.run.deploymentHeld, true);
  assert.deepEqual(model.run.holdReasons, ["auto_deploy_disabled"]);
  assert.deepEqual(model.pendingDecisions, [{ id: "target", taskIds: ["AT-002"] }]);
  assert.deepEqual(model.uncertainOperations, [{ operationId: "uncertain", taskId: "AT-001", status: "unknown" }]);
  assert.deepEqual(model.tail, { classification: "progress_possible", selectedTaskIds: [] });
  assert.deepEqual(model.nextAction, { kind: "dispatch_or_refill", taskIds: ["AT-002"] });
});

test("status never invents an unobserved operational version", () => {
  const source = effectiveCanonical();
  delete source.state.stateVersion;
  delete source.state.run.operationalVersion;
  assert.equal(createStatusModel(project, source).stateVersion, null);
  assert.equal(createStatusModel(project, source).versions.operational, null);
});

test("status last-good fallback never presents stale authority as current", async () => {
  const lastGood = createStatusModel(project, effectiveCanonical());
  let optionsSeen;
  const model = await readStatus(project, { lastGood, loadCanonicalState: async (_project, options) => { optionsSeen = options; throw new Error("generation mismatch"); } });
  assert.equal(optionsSeen.readOnly, true);
  assert.equal(model.tracker.fingerprint, null);
  assert.equal(model.run.fingerprint, null);
  assert.deepEqual(model.run.selectedBatchTaskIds, []);
  assert.equal(model.project.ownerSessionId, null);
  assert.deepEqual(model.targets, {});
});

test("status model keeps every task visible while counting only unique actionable leaves", () => {
  const model = createStatusModel(project, canonical());

  assert.equal(model.tasks.length, 6);
  assert.deepEqual(model.progress, {
    status: "exact",
    total: 3,
    completed: 1,
    remaining: 2,
    percentage: 33,
    excluded: { cancelled: 1, deferred: 1 },
  });
  assert.deepEqual(model.activity, { active: 1, parked: 1, paused: 0, ready: 1, capacity: null });
  assert.equal(model.tasks.find((task) => task.id === "CHILD").counted, true);
  assert.equal(model.tasks.find((task) => task.id === "PARENT").counted, false);
  assert.equal(model.tasks.find((task) => task.id === "PARENT").nextAction, "Delegate child");
  assert.deepEqual(model.tasks.find((task) => task.id === "PARKED").runtime, {
    compute: "parked", explicitPause: false, checkpointPath: ".agent-team/checkpoints/parked.json", resumeWhen: "READY", evidencePointers: [],
  });
  assert.equal(model.run.paused, true);
  assert.equal(model.run.current, "run-1");
  assert.deepEqual(model.run.scope, { status: "partial", taskIds: ["PARENT", "CHILD"] });
  assert.equal(model.teams[0].updatedAt, "2026-09-08T12:00:00.000Z");
});

test("status model de-duplicates IDs, retains unapproved deferred work, and marks unknown status provisional", () => {
  const model = createStatusModel(project, canonical({ tasks: [
    { id: "PARENT", status: "in_progress" },
    { id: "CHILD", status: "completed", parent: "PARENT" },
    { id: "CHILD", status: "completed", parent: "PARENT" },
    { id: "LATER", status: "deferred" },
    { id: "MYSTERY", status: "vendor_waiting" },
    { id: "APPROVED", status: "approved_deferred" },
  ] }));
  assert.equal(model.tasks.length, 6);
  assert.equal(model.progress.total, 3);
  assert.equal(model.progress.completed, 1);
  assert.equal(model.progress.excluded.deferred, 1);
  assert.equal(model.progress.status, "provisional");
  assert.equal(model.tasks.filter((task) => task.id === "CHILD" && task.counted).length, 1);
  assert.equal(model.tasks.find((task) => task.id === "LATER").counted, true);
});

test("status model uses supplied capacity rather than inferring it from unassigned work", () => {
  const model = createStatusModel(project, canonical(), { capacity: { limit: 4, active: 2, reservedReview: 1 } });
  assert.equal(model.activity.capacity, 1);
});

test("status model marks a missing hierarchy reference as provisional", () => {
  const model = createStatusModel(project, canonical({ tasks: [{ id: "ORPHAN", status: "completed", parent: "MISSING" }] }));
  assert.equal(model.progress.status, "provisional");
  assert.equal(model.progress.percentage, 100);
});

test("status model represents unavailable and unknown sources without claiming empty progress", () => {
  const unavailable = createStatusModel(project, canonical({ tracker: { kind: "tasks", id: "TASKS.md", path: "TASKS.md", status: "unavailable", reason: "read_failed" }, tasks: [] }));
  const unknown = createStatusModel(project, canonical({ tracker: { kind: "tasks", id: "TASKS.md", path: "TASKS.md", status: "not_read" }, tasks: [] }));

  assert.equal(unavailable.freshness.status, "unavailable");
  assert.equal(unavailable.progress.status, "unknown");
  assert.equal(unavailable.progress.percentage, null);
  assert.deepEqual(unavailable.activity, { active: null, parked: null, paused: null, ready: null, capacity: null });
  assert.equal(unknown.freshness.status, "unknown");
  assert.equal(unknown.progress.status, "unknown");
});

test("status model returns N/A for a known zero-task tracker and is immutable", () => {
  const model = createStatusModel(project, canonical({ tasks: [] }));

  assert.equal(model.progress.status, "not_applicable");
  assert.equal(model.progress.percentage, null);
  assert.throws(() => { model.tasks.push({ id: "nope" }); }, TypeError);
});

test("readStatus delegates project resolution and canonical loading without mutation", async () => {
  const source = canonical();
  const result = await readStatus("/project", {
    resolveProject: async (cwd) => ({ ...project, cwd }),
    loadCanonicalState: async () => source,
  });

  assert.equal(result.project.id, "project-1");
  assert.equal(result.tasks[0].id, "PARENT");
  assert.equal(source.tasks.length, 6);
});

test("readStatus converts a canonical read failure into an unavailable model retaining supplied last-good rows", async () => {
  const lastGood = createStatusModel(project, canonical());
  const result = await readStatus("/project", {
    resolveProject: async () => project,
    loadCanonicalState: async () => { throw new Error("tracker backend timeout"); },
    lastGood,
  });
  assert.equal(result.freshness.status, "unavailable");
  assert.match(result.freshness.reason, /timeout/);
  assert.equal(result.progress.status, "unknown");
  assert.equal(result.tasks.length, 6);
});
