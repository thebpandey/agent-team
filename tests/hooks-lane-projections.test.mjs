import assert from "node:assert/strict";
import test from "node:test";

import { renderDashboard } from "../hooks/lib/dashboard.mjs";
import { inspectCheckpointEvidence, projectRunState, runBoundedProbe } from "../hooks/lib/recovery.mjs";
import { createStatusModel } from "../hooks/lib/status.mjs";

const executionSettings = {
  lanes: { enabled: true, rotation: { tasks: 2, onPressure: true }, factSheetStaleDays: 7, workerUpdateMaxChars: 2000, briefMaxWords: 6000 },
  supervision: { heartbeatSeconds: 600 },
  limits: { subprocessMaxBufferBytes: 2 * 1024 * 1024, maxPlanTasks: 1000, canonicalRecordMaxBytes: 16 * 1024 * 1024 },
};

const worker = { host: "codex", sessionId: "worker-session", generation: 1 };

function lane(extra = {}) {
  return {
    schemaVersion: 1,
    id: "build-a",
    teamId: "TEAM-BUILD-A",
    status: "prepared",
    role: "developer",
    model: "gpt-6-astra",
    effort: "high",
    queue: ["AT-001", "AT-002", "AT-003"],
    currentTaskId: null,
    worker,
    worktree: "/project-lane-build-a",
    branch: "lane/build-a",
    brief: { path: ".agent-team/lanes/build-a/BRIEF.md", sha256: "a".repeat(64) },
    ownershipEvidence: { path: ".agent-team/lanes/build-a/evidence/ownership.json", sha256: "e".repeat(64), revision: "c".repeat(40), pathSetHash: "f".repeat(64) },
    rotationCount: 1,
    handover: { path: ".agent-team/lanes/build-a/HANDOVER-001.md", sha256: "b".repeat(64), revision: "c".repeat(40), sequence: 1 },
    factSheets: [],
    assignments: [],
    results: [],
    ...extra,
  };
}

function canonical(lanes) {
  const taskIds = ["AT-001", "AT-002", "AT-003", "AT-LEGACY"];
  return {
    tracker: { status: "current", fingerprint: "f".repeat(64) },
    registry: { projectId: "project-1", projectOwner: "owner", projectOwnerHost: "codex", ownershipEpoch: 1, teams: [] },
    tasks: taskIds.map((id) => ({ id, status: "in_progress", owner: id === "AT-LEGACY" ? "TEAM-LEGACY" : "TEAM-BUILD-A", dependencies: [] })),
    state: {
      stateVersion: 4,
      run: {
        id: "run-1", ownerSessionId: "owner", ownerHost: "codex", ownershipEpoch: 1, mode: "finite", taskIds,
        teamLimit: 3, autoDeploy: false, batchSize: 1, source: "explicit_run",
        settingSources: Object.fromEntries(["mode", "taskIds", "teamLimit", "autoDeploy", "batchSize"].map((key) => [key, "explicit_run"])),
        paused: false, operationalVersion: 4, blockers: [], pendingDeliveryIds: [], deployedTaskIds: [],
        terminalClassification: "progress_possible", executionSettings,
      },
      taskRuntime: {
        "AT-001": { compute: "active", writer: worker },
        "AT-002": { compute: "stopped", writer: worker },
        "AT-003": { compute: "unknown", writer: worker },
        "AT-LEGACY": { compute: "active", writer: { host: "codex", sessionId: "legacy-session" } },
      },
      ...(lanes === undefined ? {} : { lanes: { schemaVersion: 1, records: lanes } }),
    },
    deliveryEvidence: {},
    git: { headRevision: "c".repeat(40) },
  };
}

const projectionOptions = {
  writerLiveness: { "build-a": { ...worker, status: "active" } },
  retainedContext: { "build-a": { briefSha256: "a".repeat(64), revision: "c".repeat(40) } },
  effectiveSettings: executionSettings,
  nativeCapacity: { limit: 4, active: 2, reservedReview: 1 },
};

test("recovery counts a retained lane once and keeps its historical tasks out of legacy worker occupancy", () => {
  const source = canonical([lane()]);
  const before = structuredClone(source);
  const projected = projectRunState(source, undefined, new Date("2026-09-14T12:00:00Z"), projectionOptions);

  assert.equal(projected.lanes.length, 1);
  assert.deepEqual(projected.workers.active.map(({ taskId }) => taskId), ["AT-LEGACY"]);
  assert.deepEqual(projected.slots.logical, { limit: 3, occupied: 1, free: 2 });
  assert.deepEqual(projected.slots.native, { limit: 4, reservedReview: 1, occupied: 2, unknown: 0, free: 1 });
  assert.deepEqual(source, before);
});

test("recovery projects durable lane identity, ordered remaining work, and exact reattach pointers", () => {
  const projected = projectRunState(canonical([lane()]), undefined, new Date("2026-09-14T12:00:00Z"), projectionOptions);
  const row = projected.lanes[0];

  assert.deepEqual({
    id: row.id, teamId: row.teamId, role: row.role, queue: row.queue, currentTaskId: row.currentTaskId,
    worker: row.worker, rotationCount: row.rotationCount, worktree: row.worktree, branch: row.branch,
    brief: row.brief, handover: row.handover,
  }, {
    id: "build-a", teamId: "TEAM-BUILD-A", role: "developer", queue: ["AT-001", "AT-002", "AT-003"], currentTaskId: null,
    worker, rotationCount: 1, worktree: "/project-lane-build-a", branch: "lane/build-a",
    brief: { path: ".agent-team/lanes/build-a/BRIEF.md", sha256: "a".repeat(64) },
    handover: { path: ".agent-team/lanes/build-a/HANDOVER-001.md", sha256: "b".repeat(64), revision: "c".repeat(40), sequence: 1 },
  });
  assert.deepEqual(row.resume, {
    kind: "reattach", laneId: "build-a", host: "codex", sessionId: "worker-session", generation: 1,
    currentTaskId: null, worktree: "/project-lane-build-a", branch: "lane/build-a",
    briefSha256: "a".repeat(64), revision: "c".repeat(40),
  });
});

test("idle and unknown retained lanes remain occupied while mismatched context cannot authorize reattachment", () => {
  const activeIdle = projectRunState(canonical([lane()]), undefined, new Date(), projectionOptions);
  assert.equal(activeIdle.slots.logical.occupied, 1);
  assert.equal(activeIdle.lanes[0].liveness, "active");

  const unknown = projectRunState(canonical([lane({ status: "rotation_required" })]), undefined, new Date(), {
    ...projectionOptions,
    writerLiveness: {},
    retainedContext: { "build-a": { briefSha256: "d".repeat(64), revision: "c".repeat(40) } },
    nativeCapacity: { limit: 4, active: 2, reservedReview: 1 },
  });
  assert.equal(unknown.slots.logical.occupied, 1);
  assert.equal(unknown.slots.native.unknown, 1);
  assert.equal(unknown.lanes[0].liveness, "unknown");
  assert.deepEqual(unknown.lanes[0].resume, {
    kind: "context_refresh_or_rotation_evidence_required", laneId: "build-a", currentTaskId: null,
    worktree: "/project-lane-build-a", acceptedEvidence: ["stopped_writer", "ownership_transfer"],
  });
});

test("unresolved lane dispatch is surfaced before unrelated fresh admission", () => {
  const assignment = {
    id: "assignment-1", taskId: "AT-001", attempt: 1,
    packet: { path: ".agent-team/lanes/build-a/packets/AT-001-1.md", sha256: "9".repeat(64) },
    briefSha256: "a".repeat(64), revision: "c".repeat(40), worker,
    decisions: [], factSheets: [],
    status: "dispatched", createdAt: "2026-09-14T11:00:00Z", dispatchedAt: "2026-09-14T11:01:00Z",
    dispatch: { status: "observed", source: "codex", eventId: "dispatch-assignment-1", observedAt: "2026-09-14T11:01:00Z" },
  };
  const projected = projectRunState(canonical([lane({ status: "active", currentTaskId: "AT-001", assignments: [assignment] })]),
    undefined, new Date("2026-09-14T12:00:00Z"), projectionOptions);

  assert.deepEqual(projected.unresolvedDispatches, [{ laneId: "build-a", assignmentId: "assignment-1", taskId: "AT-001", worker }]);
  assert.deepEqual(projected.nextAction, { kind: "reconcile_lane_dispatch", taskIds: ["AT-001"] });
});

test("lane operations remain visible after the bounded unrelated operation projection", () => {
  const unrelated = Array.from({ length: 20 }, (_, index) => ({ operationId: `legacy-${index}`, taskId: "AT-LEGACY" }));
  const projected = projectRunState(canonical([lane()]), {
    pendingOperations: [...unrelated, { operationId: "lane-operation", laneId: "build-a", taskId: "AT-001" }],
  }, new Date("2026-09-14T12:00:00Z"), projectionOptions);

  assert.equal(projected.pendingOperations.length, 20);
  assert.deepEqual(projected.pendingOperations[0], { operationId: "lane-operation", laneId: "build-a", taskId: "AT-001" });
  assert.deepEqual(projected.nextAction, { kind: "reconcile_pending_operation", taskIds: ["AT-001"] });
});

test("a failed result cannot hide its still-dispatched lane assignment", () => {
  const assignment = {
    id: "assignment-failed", taskId: "AT-001", attempt: 1,
    packet: { path: ".agent-team/lanes/build-a/packets/AT-001-1.md", sha256: "9".repeat(64) },
    briefSha256: "a".repeat(64), revision: "c".repeat(40), worker, decisions: [], factSheets: [], status: "resolved",
    createdAt: "2026-09-14T11:00:00Z", dispatchedAt: "2026-09-14T11:01:00Z",
    dispatch: { status: "observed", source: "codex", eventId: "dispatch-assignment-failed", observedAt: "2026-09-14T11:01:00Z" },
  };
  const failed = {
    assignmentId: assignment.id, taskId: assignment.taskId, kind: "worker", status: "failed",
    revision: assignment.revision, worker,
    evidence: { path: ".agent-team/lanes/build-a/evidence/AT-001-worker.json", sha256: "8".repeat(64) },
    recordedAt: "2026-09-14T11:02:00Z",
  };
  const projected = projectRunState(canonical([lane({ status: "active", currentTaskId: "AT-001", assignments: [assignment], results: [failed] })]),
    undefined, new Date("2026-09-14T12:00:00Z"), projectionOptions);

  assert.deepEqual(projected.unresolvedDispatches, [{ laneId: "build-a", assignmentId: assignment.id, taskId: "AT-001", worker }]);
  assert.deepEqual(projected.nextAction, { kind: "reconcile_lane_dispatch", taskIds: ["AT-001"] });
});

test("zero native capacity fences fresh dispatch advice", () => {
  const source = canonical([lane()]);
  Object.assign(source.tasks.find(({ id }) => id === "AT-LEGACY"), { status: "ready", owner: "unassigned", parentId: null });
  delete source.state.taskRuntime["AT-LEGACY"];
  const projected = projectRunState(source, undefined, new Date("2026-09-14T12:00:00Z"), {
    ...projectionOptions,
    nativeCapacity: { limit: 2, active: 1, reservedReview: 1 },
  });

  assert.deepEqual(projected.slots.native, { limit: 2, reservedReview: 1, occupied: 1, unknown: 0, free: 0 });
  assert.deepEqual(projected.nextDispatch, { taskIds: [] });
  assert.deepEqual(projected.nextAction, { kind: "capacity_full", taskIds: ["AT-LEGACY"] });
});

test("mixed lane and legacy occupancy plus reviewer reserve fence logical dispatch capacity", () => {
  const source = canonical([lane()]);
  source.tasks.push({ id: "AT-ACTIVE", status: "in_progress", owner: "TEAM-LEGACY", dependencies: [], parentId: null });
  source.state.run.taskIds.push("AT-ACTIVE");
  source.state.taskRuntime["AT-ACTIVE"] = { compute: "active", writer: { host: "codex", sessionId: "active-legacy" } };
  Object.assign(source.tasks.find(({ id }) => id === "AT-LEGACY"), { status: "ready", owner: "unassigned", parentId: null });
  delete source.state.taskRuntime["AT-LEGACY"];
  const projected = projectRunState(source, undefined, new Date("2026-09-14T12:00:00Z"), {
    ...projectionOptions,
    nativeCapacity: { limit: 8, active: 1, reservedReview: 1 },
  });

  assert.equal(projected.slots.safelyFree, 0);
  assert.deepEqual(projected.nextDispatch, { taskIds: [] });
  assert.deepEqual(projected.nextAction, { kind: "capacity_full", taskIds: ["AT-LEGACY"] });
});

test("legacy-only full capacity cannot bypass conservative dispatch fencing", () => {
  const source = canonical(undefined);
  source.tasks.push({ id: "AT-FRESH", status: "ready", owner: "unassigned", dependencies: [], parentId: null });
  source.state.run.taskIds.push("AT-FRESH");
  const projected = projectRunState(source, undefined, new Date("2026-09-14T12:00:00Z"), {
    effectiveSettings: executionSettings,
    nativeCapacity: { limit: 8, active: 1, reservedReview: 1 },
  });

  assert.equal(projected.slots.safelyFree, 0);
  assert.deepEqual(projected.nextDispatch, { taskIds: [] });
  assert.deepEqual(projected.nextAction, { kind: "capacity_full", taskIds: ["AT-FRESH"] });
});

test("legacy state remains lane-empty while malformed present lane state fails closed", () => {
  const legacy = canonical(undefined);
  const projected = projectRunState(legacy, undefined, new Date(), { nativeCapacity: { limit: 4, active: 1, reservedReview: 1 } });
  assert.deepEqual(projected.lanes, []);
  assert.equal(projected.workers.active.length, 2);

  const malformed = canonical([]);
  malformed.state.lanes.extra = true;
  assert.throws(() => projectRunState(malformed, undefined, new Date(), projectionOptions), /invalid_lanes/);
});

test("status and dashboard expose bounded lane, rotation, and distinct capacity projections", () => {
  const source = canonical([lane({ status: "rotation_required" })]);
  const model = createStatusModel({ projectId: "project-1", root: "/project" }, source, projectionOptions);
  assert.equal(model.revision, "c".repeat(40));
  assert.equal(model.lanes[0].id, "build-a");
  assert.equal(model.lanes[0].rotationCount, 1);
  assert.deepEqual(model.slots.logical, { limit: 3, occupied: 1, free: 2 });
  assert.equal(model.slots.native.reservedReview, 1);

  const html = renderDashboard(model);
  assert.match(html, /Lane capacity/);
  assert.match(html, /build-a/);
  assert.match(html, /TEAM-BUILD-A/);
  assert.match(html, /rotation_required/);
  assert.match(html, /AT-001, AT-002, AT-003/);
  assert.match(html, /Native.*reserved review/i);
});

test("recovery subprocess settings raise collection capacity without expanding projected output", async () => {
  const output = await runBoundedProbe(process.execPath, ["-e", "process.stdout.write('x'.repeat(1200000))"], { maxOutputBytes: 32 });
  assert.equal(output.status, "available");
  assert.equal(output.output.length, 32);

  const observed = [];
  const revision = "d".repeat(40);
  const evidence = await inspectCheckpointEvidence({
    root: "/project",
    setup: { harness: "codex", settings: { hosts: { codex: { execution: { limits: { subprocessMaxBufferBytes: 3 * 1024 * 1024 } } } } } },
  }, { worktree: "/project-lane-build-a", revision, evidenceRevision: revision, sourcePointers: [] }, {
    probe: async (_command, args, options) => {
      observed.push(options.subprocessMaxBufferBytes);
      return { status: "available", output: args[0] === "rev-parse" ? revision : "" };
    },
  });
  assert.equal(evidence.status, "current");
  assert.deepEqual(observed, [3 * 1024 * 1024, 3 * 1024 * 1024]);
});
