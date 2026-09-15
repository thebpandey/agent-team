import assert from "node:assert/strict";
import { mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import test from "node:test";

import { loadCanonicalState } from "../hooks/lib/canonical-state.mjs";
import { resolveProject } from "../hooks/lib/project.mjs";
import { mutateOperationalState } from "../hooks/lib/task-transitions.mjs";
import { policyFixture } from "./hook-test-helpers.mjs";

const temporary = [];
test.afterEach(async () => Promise.all(temporary.splice(0).map((item) => rm(item, { recursive: true, force: true }))));

test("canonical state reads legacy lane absence but rejects malformed present lanes", async () => {
  const root = await mkdtemp(path.join(os.tmpdir(), "agent-team-lanes-canonical-"));
  temporary.push(root, `${root}-feature`, `${root}-remote`);
  await policyFixture(root, { qualifiedOwnership: true });
  const project = await resolveProject(root);
  assert.deepEqual((await loadCanonicalState(project)).state.lanes, undefined);

  const state = JSON.parse(await readFile(project.paths.state, "utf8"));
  state.lanes = { schemaVersion: 1, records: [{ forged: true }] };
  await writeFile(project.paths.state, `${JSON.stringify(state, null, 2)}\n`);
  await assert.rejects(loadCanonicalState(project), /invalid_lanes/);
});

test("canonical state reads and mutates a bounded 1000-task retained history but honors an explicit smaller host cap", async () => {
  const root = await mkdtemp(path.join(os.tmpdir(), "agent-team-lanes-large-canonical-"));
  temporary.push(root, `${root}-feature`, `${root}-remote`);
  await policyFixture(root, { qualifiedOwnership: true });
  const project = await resolveProject(root);
  const state = JSON.parse(await readFile(project.paths.state, "utf8"));
  const worker = { host: "codex", sessionId: "lane-worker", generation: 1 };
  const verifier = { host: "codex", sessionId: "lane-verifier", generation: 1 };
  const reviewer = { host: "codex", sessionId: "lane-reviewer", generation: 1 };
  const taskIds = Array.from({ length: 1000 }, (_, index) => `AT-${String(index + 1).padStart(4, "0")}`);
  const assignments = taskIds.map((taskId) => ({ id: `assignment-${taskId}`, taskId, attempt: 1,
    packet: { path: `.agent-team/lanes/large/packets/${taskId}-1.md`, sha256: "a".repeat(64) }, briefSha256: "b".repeat(64),
    revision: "c".repeat(40), worker, decisions: [], factSheets: [], status: "resolved", createdAt: "2026-09-14T12:00:00.000Z",
    dispatchedAt: "2026-09-14T12:01:00.000Z", dispatch: { status: "observed", source: "codex",
      eventId: `dispatch-${taskId}`, observedAt: "2026-09-14T12:01:00.000Z" } }));
  const results = assignments.flatMap((assignment) => [["worker", worker], ["verification", verifier], ["independent_review", reviewer],
    ["integration", { host: "codex", sessionId: "owner-session", generation: 1 }]].map(([kind, resultWorker], index) => ({
      assignmentId: assignment.id, taskId: assignment.taskId, kind, status: "passed", revision: assignment.revision, worker: resultWorker,
      evidence: { path: `.agent-team/lanes/${kind === "independent_review" ? "large-review" : "large"}/evidence/${assignment.taskId}-${kind}.json`,
        sha256: String(index + 1).repeat(64) },
      recordedAt: `2026-09-14T12:0${index + 2}:00.000Z`,
    })));
  const reviewAssignments = assignments.map((assignment) => ({ id: `review-${assignment.id}`, taskId: assignment.taskId, attempt: 1,
    packet: { path: `.agent-team/lanes/large-review/packets/${assignment.taskId}-1.md`, sha256: "6".repeat(64) },
    briefSha256: "7".repeat(64), revision: assignment.revision, worker: reviewer, decisions: [], factSheets: [], sourceAssignment: {
      laneId: "large", assignmentId: assignment.id, packetSha256: assignment.packet.sha256, revision: assignment.revision }, status: "resolved",
    createdAt: "2026-09-14T12:00:00.000Z", dispatchedAt: "2026-09-14T12:01:00.000Z",
    dispatch: { status: "observed", source: "codex", eventId: `review-${assignment.taskId}`,
      observedAt: "2026-09-14T12:01:00.000Z" } }));
  const reviewResults = reviewAssignments.map((assignment) => {
    const linked = results.find((entry) => entry.assignmentId === assignment.sourceAssignment.assignmentId && entry.kind === "independent_review");
    return { ...linked, assignmentId: assignment.id };
  });
  state.lanes = { schemaVersion: 1, records: [{ schemaVersion: 1, id: "large", teamId: "TEAM-LARGE", status: "closed", role: "developer",
    model: "gpt-6-astra", effort: "high", queue: taskIds, currentTaskId: null, worker: null, worktree: `${root}-lane-large`, branch: "lane/large",
    brief: { path: ".agent-team/lanes/large/BRIEF.md", sha256: "b".repeat(64) }, ownershipEvidence: {
      path: ".agent-team/lanes/large/evidence/ownership.json", sha256: "d".repeat(64), revision: "c".repeat(40), pathSetHash: "e".repeat(64) },
    rotationCount: 0, handover: null, factSheets: [], assignments, results },
  { schemaVersion: 1, id: "large-review", teamId: "TEAM-LARGE-REVIEW", status: "closed", role: "reviewer",
    model: "gpt-6-astra", effort: "high", queue: taskIds, currentTaskId: null, worker: null, worktree: `${root}-lane-large-review`,
    branch: "lane/large-review", brief: { path: ".agent-team/lanes/large-review/BRIEF.md", sha256: "7".repeat(64) },
    ownershipEvidence: { path: ".agent-team/lanes/large-review/evidence/ownership.json", sha256: "8".repeat(64),
      revision: "c".repeat(40), pathSetHash: "9".repeat(64) }, rotationCount: 0, handover: null, factSheets: [],
    assignments: reviewAssignments, results: reviewResults }] };
  const source = `${JSON.stringify(state, null, 2)}\n`;
  assert.ok(Buffer.byteLength(source) > 1024 * 1024);
  await writeFile(project.paths.state, source);
  const loaded = await loadCanonicalState(project);
  assert.equal(loaded.state.lanes.records[0].queue.length, 1000);
  const mutation = await mutateOperationalState(project, { operationId: "large-state-mutation", actorSessionId: "owner-session",
    expectedVersion: loaded.state.stateVersion ?? 0 }, async (next) => ({ state: next, result: { retained: next.lanes.records[0].queue.length } }), {
    nativeIdentity: { host: "codex", sessionId: "owner-session", observed: true, cwd: root, ownershipEpoch: 1 },
  });
  assert.equal(mutation.status, "applied", mutation.reason);
  assert.equal(mutation.result.retained, 1000);

  const setup = JSON.parse(await readFile(project.paths.setup, "utf8"));
  setup.settings = { hosts: { codex: { execution: { limits: { canonicalRecordMaxBytes: 1024 * 1024 } } } } };
  await writeFile(project.paths.setup, `${JSON.stringify(setup, null, 2)}\n`);
  await assert.rejects(loadCanonicalState(project, { host: "codex" }), /unsafe_canonical_record/);
});
