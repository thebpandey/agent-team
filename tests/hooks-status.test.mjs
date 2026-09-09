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
