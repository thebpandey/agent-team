import assert from "node:assert/strict";
import { spawn } from "node:child_process";
import { createHash } from "node:crypto";
import { access, mkdir, mkdtemp, readFile, rename, rm, writeFile } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import test from "node:test";
import { resolveProject } from "../hooks/lib/project.mjs";
import { loadCanonicalState } from "../hooks/lib/canonical-state.mjs";
import { captureWriterIdentity } from "../hooks/lib/task-transitions.mjs";
import { policyFixture } from "./hook-test-helpers.mjs";

const temporary = [];
test.afterEach(async () => Promise.all(temporary.splice(0).map((p) => rm(p, { recursive: true, force: true }))));
const activeRun = (taskIds = ["AT-001"]) => ({ id: "cleanup-run", ownerSessionId: "owner-session", ownerHost: "codex", ownershipEpoch: 1,
  mode: "finite", taskIds, teamLimit: 1, autoDeploy: false, batchSize: 1, source: "explicit_run",
  settingSources: Object.fromEntries(["mode", "taskIds", "teamLimit", "autoDeploy", "batchSize"].map((key) => [key, "explicit_run"])),
  paused: false, operationalVersion: 0, blockers: [], pendingDeliveryIds: [], deployedTaskIds: [], terminalClassification: "progress_possible" });
async function fixture() {
  const root = await mkdtemp(path.join(os.tmpdir(), "agent-team-cleanup-"));
  temporary.push(root, `${root}-feature`, `${root}-remote`);
  const value = await policyFixture(root);
  const project = await resolveProject(root);
  const child = spawn(process.execPath, [path.join(import.meta.dirname, "hooks-transitions.test.mjs"), "writer"], { stdio: ["ignore", "pipe", "ignore"] });
  await new Promise((resolve) => child.stdout.once("data", resolve));
  const writer = await captureWriterIdentity(child.pid);
  const stopped = new Promise((resolve) => child.once("exit", resolve)); child.kill("SIGTERM"); await stopped;
  const evidencePath = path.join(root, ".agent-team/evidence/verified.json");
  await mkdir(path.dirname(evidencePath), { recursive: true });
  await writeFile(evidencePath, JSON.stringify({ status: "passed", revision: value.revision, taskId: "AT-001" }));
  await writeFile(project.paths.tasks, (await readFile(project.paths.tasks, "utf8")).replace("in_progress", "verified"));
  value.state.cleanup = { "AT-001": { worktree: value.feature, revision: value.revision, integrationRef: "main", verification: { status: "passed", revision: value.revision },
    writer, taskOwner: "TEAM-001", evidencePaths: [evidencePath], resourceOwner: "agent-team", previewRequired: false, retain: false } };
  value.state.taskRuntime = { "AT-001": { compute: "parked", writer, worktree: value.feature } };
  value.state.release.autoDeploy = false;
  value.state.run = activeRun();
  await writeFile(project.paths.state, JSON.stringify(value.state));
  return { ...value, project, writer, evidencePath, request: { actorSessionId: "owner-session", operationId: "cleanup-one", expectedVersion: 0, taskId: "AT-001", expectedRevision: value.revision, worktree: value.feature, expectedWriter: writer } };
}
async function cleanup(...args) {
  const module = await import("../hooks/lib/cleanup.mjs").catch(() => ({}));
  assert.equal(typeof module.cleanupDevelopmentWorktree, "function", "development cleanup implementation is required");
  return module.cleanupDevelopmentWorktree(...args);
}

const laneWorker = { host: "codex", sessionId: "lane-worker", generation: 1 };
const laneEvidenceWorker = { host: "codex", sessionId: "lane-verifier", generation: 1 };
const laneReviewer = { host: "claude-code", sessionId: "lane-reviewer", generation: 1 };

function laneAssignment(taskId, revision, attempt, briefSha256) {
  return {
    id: `assignment-${taskId}-${attempt}`, taskId, attempt,
    packet: { path: `.agent-team/lanes/build-a/packets/${taskId}-${attempt}.md`, sha256: String(attempt).repeat(64) },
    briefSha256, revision, worker: laneWorker, decisions: [], factSheets: [], status: "resolved",
    createdAt: "2026-09-14T10:00:00Z", dispatchedAt: "2026-09-14T10:01:00Z",
    dispatch: { status: "observed", source: "codex", eventId: `dispatch-${attempt}`, observedAt: "2026-09-14T10:01:00Z" },
  };
}

function laneResult(assignment, kind, workerIdentity, revision) {
  const digest = { worker: "2", verification: "3", independent_review: "4", integration: "5" }[kind];
  const evidenceLaneId = kind === "independent_review" ? "review-a" : "build-a";
  return {
    assignmentId: assignment.id, taskId: assignment.taskId, kind, status: "passed", revision, worker: workerIdentity,
    evidence: { path: `.agent-team/lanes/${evidenceLaneId}/evidence/${assignment.taskId}-${kind}.json`, sha256: digest.repeat(64) },
    recordedAt: "2026-09-14T10:02:00Z",
  };
}

async function installLane(value, { status = "closed", integrated = true } = {}) {
  await writeFile(value.project.paths.tasks, `${await readFile(value.project.paths.tasks, "utf8")}| AT-002 | Complete second lane task | TEAM-001 | none | verified | ${value.revision} | Done. |\n`);
  await writeFile(value.project.paths.teams, (await readFile(value.project.paths.teams, "utf8")).replace("| AT-001 | in_progress |", "| AT-001, AT-002 | in_progress |"));
  value.state.run = activeRun(["AT-001", "AT-002"]);
  const writeArtifact = async (relative, contents) => {
    const bytes = Buffer.from(contents);
    await mkdir(path.dirname(path.join(value.root, relative)), { recursive: true });
    await writeFile(path.join(value.root, relative), bytes);
    return createHash("sha256").update(bytes).digest("hex");
  };
  const brief = { path: ".agent-team/lanes/build-a/BRIEF.md" };
  brief.sha256 = await writeArtifact(brief.path, "immutable lane brief\n");
  const ownershipEvidence = { path: ".agent-team/lanes/build-a/evidence/ownership.json", revision: value.revision, pathSetHash: "f".repeat(64) };
  ownershipEvidence.sha256 = await writeArtifact(ownershipEvidence.path, JSON.stringify({ revision: value.revision, paths: ["src/"] }));
  const assignments = [laneAssignment("AT-001", value.revision, 1, brief.sha256), laneAssignment("AT-002", value.revision, 1, brief.sha256)];
  for (const assignment of assignments) assignment.packet.sha256 = await writeArtifact(assignment.packet.path, JSON.stringify({ assignment: assignment.id }));
  const results = assignments.flatMap((assignment) => [
    laneResult(assignment, "worker", laneWorker, value.revision),
    laneResult(assignment, "verification", laneEvidenceWorker, value.revision),
    laneResult(assignment, "independent_review", laneReviewer, value.revision),
    ...(integrated ? [laneResult(assignment, "integration", laneEvidenceWorker, value.revision)] : []),
  ]);
  for (const result of results) result.evidence.sha256 = await writeArtifact(result.evidence.path,
    JSON.stringify({ assignmentId: result.assignmentId, taskId: result.taskId, kind: result.kind, revision: result.revision }));
  const reviewBrief = { path: ".agent-team/lanes/review-a/BRIEF.md" };
  reviewBrief.sha256 = await writeArtifact(reviewBrief.path, "immutable review lane brief\n");
  const reviewOwnershipEvidence = { path: ".agent-team/lanes/review-a/evidence/ownership.json", revision: value.revision,
    pathSetHash: "e".repeat(64) };
  reviewOwnershipEvidence.sha256 = await writeArtifact(reviewOwnershipEvidence.path,
    JSON.stringify({ revision: value.revision, paths: [] }));
  const reviewAssignments = [];
  for (const source of assignments) {
    const assignment = {
      id: `review-${source.id}`, taskId: source.taskId, attempt: 1,
      packet: { path: `.agent-team/lanes/review-a/packets/${source.taskId}-1.md` },
      briefSha256: reviewBrief.sha256, revision: value.revision, worker: laneReviewer, decisions: [], factSheets: [], status: "resolved",
      createdAt: "2026-09-14T10:00:00Z", dispatchedAt: "2026-09-14T10:01:00Z",
      dispatch: { status: "observed", source: "claude-code", eventId: `review-dispatch-${source.taskId}`, observedAt: "2026-09-14T10:01:00Z" },
      sourceAssignment: { laneId: "build-a", assignmentId: source.id, packetSha256: source.packet.sha256, revision: value.revision },
    };
    assignment.packet.sha256 = await writeArtifact(assignment.packet.path, JSON.stringify({ assignment: assignment.id }));
    reviewAssignments.push(assignment);
  }
  const reviewResults = reviewAssignments.map((assignment, index) => ({
    ...structuredClone(results.find((result) => result.assignmentId === assignments[index].id && result.kind === "independent_review")),
    assignmentId: assignment.id,
  }));
  value.state.lanes = { schemaVersion: 1, records: [{
    schemaVersion: 1, id: "build-a", teamId: "TEAM-001", status, role: "developer", model: "gpt-6-astra", effort: "high",
    queue: ["AT-001", "AT-002"], currentTaskId: status === "closed" ? null : "AT-001", worker: status === "closed" ? null : laneWorker,
    worktree: value.feature, branch: "lane/build-a", brief, ownershipEvidence,
    rotationCount: 1, handover: null, factSheets: [], assignments, results,
  }, {
    schemaVersion: 1, id: "review-a", teamId: "TEAM-REVIEW", status: "closed", role: "reviewer", model: "gpt-5.6-sol", effort: "medium",
    queue: ["AT-001", "AT-002"], currentTaskId: null, worker: null,
    worktree: `${value.feature}-review`, branch: "lane/review-a", brief: reviewBrief, ownershipEvidence: reviewOwnershipEvidence,
    rotationCount: 0, handover: null, factSheets: [], assignments: reviewAssignments, results: reviewResults,
  }] };
  value.laneDeliveryEvidence = Object.fromEntries(assignments.map(({ taskId, revision }) => [taskId, {
    taskId, sourceRevision: revision, integratedRevision: revision,
  }]));
  await writeFile(value.project.paths.state, JSON.stringify(value.state));
}

test("verified integrated clean worktree is removed normally with deployment disabled and evidence retained", async () => {
  const value = await fixture();
  const result = await cleanup(value.project, value.request);
  assert.equal(result.status, "applied");
  await assert.rejects(access(value.feature), { code: "ENOENT" });
  await access(value.evidencePath);
  await access(value.project.paths.tasks);
  assert.equal((await loadCanonicalState(value.project)).state.release.autoDeploy, false);
  assert.equal((await cleanup(value.project, value.request)).status, "duplicate");
});

test("cleanup reconciles removal after a lost state receipt without another destructive command", async () => {
  const value = await fixture();
  let stateWrites = 0;
  const result = await cleanup(value.project, value.request, { filesystem: { rename: async (from, to) => {
    if (to === value.project.paths.state && ++stateWrites === 2) throw new Error("receipt unavailable");
    return rename(from, to);
  } } });
  assert.equal(result.status, "unavailable");
  await assert.rejects(access(value.feature), { code: "ENOENT" });
  const state = (await loadCanonicalState(value.project)).state;
  assert.equal(state.pendingOperations["cleanup-one"].phase, "uncertain");
  const recovered = await cleanup(value.project, { ...value.request, expectedVersion: state.stateVersion });
  assert.equal(recovered.status, "applied");
  assert.equal(recovered.result.reconciled, true);
});

test("outside-scope cleanup rejects before every probe and removal", async () => {
  const value = await fixture();
  const state = JSON.parse(await readFile(value.project.paths.state, "utf8"));
  state.run = activeRun(["AT-OTHER"]);
  await writeFile(value.project.paths.state, JSON.stringify(state));
  let gitProbes = 0;
  const before = await readFile(value.project.paths.state);
  const result = await cleanup(value.project, value.request, { runGit: async () => { gitProbes += 1; throw new Error("must not probe"); } });
  assert.deepEqual(result, { status: "conflict", reason: "outside_scope" });
  assert.equal(gitProbes, 0);
  assert.deepEqual(await readFile(value.project.paths.state), before);
  await access(value.feature);
});

test("cleanup rejects a missing or malformed admitted run before probes", async () => {
  for (const run of [undefined, { taskIds: ["AT-001"], paused: false }]) {
    const value = await fixture();
    const state = JSON.parse(await readFile(value.project.paths.state, "utf8"));
    if (run === undefined) delete state.run; else state.run = run;
    await writeFile(value.project.paths.state, JSON.stringify(state));
    let gitProbes = 0;
    const before = await readFile(value.project.paths.state);
    const result = await cleanup(value.project, value.request, { runGit: async () => { gitProbes += 1; throw new Error("must not probe"); } });
    assert.deepEqual(result, { status: "conflict", reason: "outside_scope" });
    assert.equal(gitProbes, 0);
    assert.deepEqual(await readFile(value.project.paths.state), before);
    await access(value.feature);
  }
});

test("in-scope cleanup still uses one locked tracker snapshot", async () => {
  const value = await fixture();
  const state = JSON.parse(await readFile(value.project.paths.state, "utf8"));
  state.run = activeRun();
  await writeFile(value.project.paths.state, JSON.stringify(state));
  assert.equal((await cleanup(value.project, value.request)).status, "applied");
  await assert.rejects(access(value.feature), { code: "ENOENT" });
});

test("lane worktree cleanup refuses an open queue before probes or removal", async () => {
  const value = await fixture();
  await installLane(value, { status: "active" });
  let probes = 0;
  const result = await cleanup(value.project, value.request, { runGit: async () => { probes += 1; throw new Error("must not probe"); } });
  assert.deepEqual(result, { status: "conflict", reason: "lane_open" });
  assert.equal(probes, 0);
  await access(value.feature);
});

test("lane worktree cleanup refuses an unintegrated historical task before probes", async () => {
  const value = await fixture();
  await installLane(value, { integrated: false });
  let probes = 0;
  const result = await cleanup(value.project, value.request, { runGit: async () => { probes += 1; throw new Error("must not probe"); } });
  assert.deepEqual(result, { status: "conflict", reason: "lane_tasks_not_integrated" });
  assert.equal(probes, 0);
  await access(value.feature);
});

test("fully closed integrated lane permits existing safe cleanup checks for its shared worktree", async () => {
  const value = await fixture();
  await installLane(value);
  const result = await cleanup(value.project, value.request, {
    loadCanonicalForCleanup: async () => ({ state: value.state, deliveryEvidence: value.laneDeliveryEvidence }),
  });
  assert.equal(result.status, "applied", result.reason);
  await assert.rejects(access(value.feature), { code: "ENOENT" });
  await access(value.evidencePath);
});

test("closed lane cleanup retains a worktree when immutable lane evidence changed", async () => {
  const value = await fixture();
  await installLane(value);
  const pointer = value.state.lanes.records[0].results[0].evidence.path;
  await writeFile(path.join(value.root, pointer), "changed evidence\n");
  let probes = 0;
  const result = await cleanup(value.project, value.request, {
    loadCanonicalForCleanup: async () => ({ state: value.state, deliveryEvidence: value.laneDeliveryEvidence }),
    runGit: async () => { probes += 1; throw new Error("must not probe"); },
  });
  assert.deepEqual(result, { status: "conflict", reason: "retain_lane_evidence_changed" });
  assert.equal(probes, 0);
  await access(value.feature);
});

for (const kind of ["tracked", "untracked", "ignored", "user_owned", "unknown_writer", "different_pid_namespace", "missing_pid_namespace", "preview"]) {
  test(`cleanup retains ${kind} resources`, async () => {
    const value = await fixture();
    if (kind === "tracked") await writeFile(path.join(value.feature, "src/owned.js"), "user change\n");
    if (kind === "untracked") await writeFile(path.join(value.feature, "USER.txt"), "user content\n");
    if (kind === "ignored") {
      await mkdir(path.join(value.feature, ".agent-team"));
      await writeFile(path.join(value.feature, ".agent-team/USER.txt"), "ignored content\n");
    }
    if (kind === "user_owned") value.state.cleanup["AT-001"].resourceOwner = "user";
    if (kind === "unknown_writer") value.state.cleanup["AT-001"].writer = { pid: process.pid };
    if (kind === "different_pid_namespace") value.writer.pidNamespace = "pid:[0]";
    if (kind === "missing_pid_namespace") delete value.writer.pidNamespace;
    if (kind === "preview") value.state.cleanup["AT-001"].previewRequired = true;
    await writeFile(value.project.paths.state, JSON.stringify(value.state));
    const result = await cleanup(value.project, value.request);
    assert.equal(result.status, "conflict");
    assert.match(result.reason, /retain|dirty|writer|preview|owner/);
    await access(value.feature);
  });
}

for (const kind of ["replacement_writer", "changed_owner", "other_task", "other_runtime", "missing_runtime"]) {
  test(`cleanup retains checkout when current ${kind} no longer matches the old cleanup assignment`, async () => {
    const value = await fixture();
    if (kind === "replacement_writer") value.state.taskRuntime["AT-001"].writer = await captureWriterIdentity();
    if (kind === "changed_owner") await writeFile(value.project.paths.tasks, (await readFile(value.project.paths.tasks, "utf8")).replace("TEAM-001", "TEAM-NEW"));
    if (kind === "other_task") await writeFile(value.project.paths.teams, (await readFile(value.project.paths.teams, "utf8")).replace("| AT-001 |", "| AT-001, AT-OTHER |"));
    if (kind === "other_runtime") value.state.taskRuntime["AT-OTHER"] = { worktree: value.feature, compute: "active", writer: await captureWriterIdentity() };
    if (kind === "missing_runtime") delete value.state.taskRuntime;
    await writeFile(value.project.paths.state, JSON.stringify(value.state));
    const result = await cleanup(value.project, value.request);
    assert.equal(result.status, "conflict");
    await access(value.feature);
    await access(value.evidencePath);
  });
}
