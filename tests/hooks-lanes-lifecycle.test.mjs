import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import { execFileSync } from "node:child_process";
import { mkdir, mkdtemp, readFile, rename, rm, symlink, writeFile } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import test from "node:test";

import { runCommand } from "../hooks/agent-team-cli.mjs";
import { runNormalizedHook } from "../hooks/agent-team-hook.mjs";
import { loadCanonicalState } from "../hooks/lib/canonical-state.mjs";
import { advanceLane, closeLane, createLane, laneCollectionFingerprint, laneFingerprint, rotateLane } from "../hooks/lib/lanes.mjs";
import { classifyOperation } from "../hooks/lib/operation.mjs";
import { resolveProject } from "../hooks/lib/project.mjs";
import { effectiveRunFingerprint } from "../hooks/lib/run-state.mjs";
import { resolveExecutionSettings } from "../hooks/lib/settings.mjs";
import { evaluatePolicy } from "../hooks/lib/policy.mjs";
import { recordGateEvidence } from "../hooks/lib/task-transitions.mjs";
import { policyFixture } from "./hook-test-helpers.mjs";

const temporary = [];
test.afterEach(async () => Promise.all(temporary.splice(0).map((item) => rm(item, { recursive: true, force: true }))));
const digest = (source) => createHash("sha256").update(source).digest("hex");
const json = (value) => `${JSON.stringify(value, null, 2)}\n`;
const fixtureSkillPath = ".agent-team/fixture-skills/test-driven-development/SKILL.md";
const fixtureSkillSource = "# Test fixture skill\n\nExercise the lane lifecycle test protocol.\n";

async function fixture() {
  const root = await mkdtemp(path.join(os.tmpdir(), "agent-team-lane-lifecycle-"));
  temporary.push(root, `${root}-feature`, `${root}-remote`, `${root}-lane-build-a`);
  await policyFixture(root, { qualifiedOwnership: true });
  const laneWorktree = `${root}-lane-build-a`;
  execFileSync("git", ["worktree", "add", "-q", "-b", "lane/build-a", laneWorktree, "main"], { cwd: root });
  const revision = execFileSync("git", ["rev-parse", "HEAD"], { cwd: root, encoding: "utf8" }).trim();
  const readmeHash = digest(await readFile(path.join(root, "README.md")));
  await mkdir(path.dirname(path.join(root, fixtureSkillPath)), { recursive: true });
  await writeFile(path.join(root, fixtureSkillPath), fixtureSkillSource);
  const skillPath = fixtureSkillPath;
  const skillHash = digest(fixtureSkillSource);
  const briefSource = `# Build lane A

## Role / model / effort
developer / gpt-6-astra / high

## Queue
AT-101, AT-102

## Shared rules
Preserve tracker authority and exact revision evidence.

## Writable paths
hooks/lib/lane-fixture/**

## Required skills
${skillPath} ${skillHash}

## Applicable instructions
README.md ${readmeHash}

## Context revision
${revision}

## Evidence destination
.agent-team/lanes/build-a/evidence/

## Handoff format
State revision, checks, remaining risk, and one next action.
`;
  const laneRoot = path.join(root, ".agent-team", "lanes", "build-a");
  await mkdir(path.join(laneRoot, "evidence"), { recursive: true });
  await writeFile(path.join(laneRoot, "BRIEF.md"), briefSource);
  const ownedPaths = ["hooks/lib/lane-fixture/**"];
  const pathSetHash = digest(JSON.stringify(ownedPaths));
  const ownershipSource = json({ schemaVersion: 1, status: "resolved", source: "graphify", revision, ownedPaths, pathSetHash,
    conflicts: [], observedAt: "2026-09-14T13:00:00.000Z" });
  await writeFile(path.join(laneRoot, "evidence", "ownership.json"), ownershipSource);
  const project = await resolveProject(root);
  const teams = await readFile(project.paths.teams, "utf8");
  await writeFile(project.paths.teams, `${teams.trimEnd()}\n| TEAM-LANE-A | lane-build-a | lane-worker | ${laneWorktree} | lane/build-a | ${ownedPaths[0]} | AT-101, AT-102 | ready |\n`);
  await writeFile(project.paths.tasks, `# Agent-Team Tasks
| ID | Requirement / acceptance | Owner | Depends on | Status | Revision / evidence | Next action |
| --- | --- | --- | --- | --- | --- | --- |
| AT-101 | First lane task | none | none | ready | none | Claim. |
| AT-102 | Second lane task | none | AT-101 | ready | none | Wait. |
`);
  const initial = await loadCanonicalState(project);
  const state = structuredClone(initial.state);
  state.stateVersion = 0;
  state.run = {
    id: "run-lanes",
    ownerSessionId: "owner-session",
    ownerHost: "codex",
    ownershipEpoch: 1,
    mode: "finite",
    taskIds: ["AT-101", "AT-102"],
    teamLimit: 2,
    autoDeploy: false,
    batchSize: 2,
    source: "explicit_run",
    settingSources: Object.fromEntries(["mode", "taskIds", "teamLimit", "autoDeploy", "batchSize"].map((key) => [key, "explicit_run"])),
    executionSettings: resolveExecutionSettings(project.setup, "codex"),
    paused: false,
    operationalVersion: 1,
    blockers: [],
    pendingDeliveryIds: [],
    deployedTaskIds: [],
    terminalClassification: "progress_possible",
  };
  await writeFile(project.paths.state, json(state));
  const current = await loadCanonicalState(project);
  const lane = {
    id: "build-a",
    teamId: "TEAM-LANE-A",
    role: "developer",
    model: "gpt-6-astra",
    effort: "high",
    queue: ["AT-101", "AT-102"],
    worker: { host: "codex", sessionId: "lane-worker", generation: 1 },
    worktree: laneWorktree,
    branch: "lane/build-a",
    brief: { path: ".agent-team/lanes/build-a/BRIEF.md", sha256: digest(briefSource) },
    ownershipEvidence: { path: ".agent-team/lanes/build-a/evidence/ownership.json", sha256: digest(ownershipSource), revision, pathSetHash },
    factSheets: [],
  };
  const request = {
    schemaVersion: 1,
    operationId: "lane-create-build-a",
    expectedTrackerFingerprint: current.tracker.fingerprint,
    expectedRunFingerprint: effectiveRunFingerprint(current.state.run),
    expectedRevision: revision,
    expectedLanesFingerprint: laneCollectionFingerprint(),
    lane,
    reason: "Create the admitted retained build lane.",
  };
  const options = { actorSessionId: "owner-session", expectedVersion: 0,
    nativeIdentity: { host: "codex", sessionId: "owner-session", observed: true, cwd: root, ownershipEpoch: 1 } };
  return { project, request, options, laneRoot };
}

function packetSource({ assignmentId, taskId, attempt, revision, trackerFingerprint, laneFingerprint: fingerprint, briefSha256 }) {
  return `# Lane task packet
Assignment: ${assignmentId}
Task: ${taskId}
Attempt: ${attempt}
Revision: ${revision}
Tracker fingerprint: ${trackerFingerprint}
Lane fingerprint: ${fingerprint}
Brief fingerprint: ${briefSha256}

## Acceptance
Complete the admitted tracker task under the immutable brief and existing gates.

## Delta
This is the first assignment in the retained lane.

## New decisions
- DEC-AT-101-001 | new | none

## Revision pointers
Use the exact source, tracker, lane, and brief fingerprints above.
`;
}

async function nextRequest(value, operationId = "lane-next-at-101", assignmentId = "assignment-at-101-1") {
  const canonical = await loadCanonicalState(value.project);
  const lane = canonical.state.lanes.records[0];
  const expected = { assignmentId, taskId: "AT-101", attempt: 1,
    revision: canonical.git.headRevision, trackerFingerprint: canonical.tracker.fingerprint,
    laneFingerprint: laneFingerprint(lane), briefSha256: lane.brief.sha256 };
  const source = packetSource(expected);
  const packetPath = ".agent-team/lanes/build-a/packets/AT-101-1.md";
  await mkdir(path.join(value.laneRoot, "packets"), { recursive: true });
  await writeFile(path.join(value.project.root, packetPath), source);
  return {
    request: { operationId, laneId: "build-a", assignmentId: expected.assignmentId, attempt: 1,
      schemaVersion: 1, phase: "prepare",
      expectedTrackerFingerprint: canonical.tracker.fingerprint, expectedRunFingerprint: effectiveRunFingerprint(canonical.state.run),
      expectedRevision: canonical.git.headRevision, expectedLaneFingerprint: expected.laneFingerprint,
      packet: { path: packetPath, sha256: digest(source) }, factSheets: [], reason: "Claim and prepare the first admitted lane task packet." },
    options: { ...value.options, expectedVersion: canonical.state.stateVersion },
  };
}

async function registerReviewerLane(value, taskIds = ["AT-101"]) {
  const root = value.project.root;
  const laneId = "review-a";
  const laneWorktree = `${root}-lane-review-a`;
  temporary.push(laneWorktree);
  execFileSync("git", ["worktree", "add", "-q", "-b", `lane/${laneId}`, laneWorktree, "main"], { cwd: root });
  const canonical = await loadCanonicalState(value.project);
  const revision = canonical.git.headRevision;
  const readmeHash = digest(await readFile(path.join(root, "README.md")));
  const skillPath = fixtureSkillPath;
  const skillHash = digest(await readFile(path.join(root, skillPath)));
  const briefSource = `# Reserved review lane

## Role / model / effort
reviewer / gpt-6-astra / high

## Queue
${taskIds.join(", ")}

## Shared rules
Review source assignments without claiming author tracker tasks.

## Writable paths
.agent-team/lanes/${laneId}/**

## Required skills
${skillPath} ${skillHash}

## Applicable instructions
README.md ${readmeHash}

## Context revision
${revision}

## Evidence destination
.agent-team/lanes/${laneId}/evidence/

## Handoff format
State source assignment, revision, challenges, checks, and remaining risk.
`;
  const laneRoot = path.join(root, ".agent-team/lanes", laneId);
  await mkdir(path.join(laneRoot, "evidence"), { recursive: true });
  await writeFile(path.join(laneRoot, "BRIEF.md"), briefSource);
  const ownedPaths = [`.agent-team/lanes/${laneId}/**`];
  const pathSetHash = digest(JSON.stringify(ownedPaths));
  const ownershipSource = json({ schemaVersion: 1, status: "resolved", source: "graphify", revision, ownedPaths, pathSetHash,
    conflicts: [], observedAt: "2026-09-14T13:00:00.000Z" });
  await writeFile(path.join(laneRoot, "evidence/ownership.json"), ownershipSource);
  const teams = await readFile(value.project.paths.teams, "utf8");
  await writeFile(value.project.paths.teams, `${teams.trimEnd()}\n| TEAM-REVIEW-A | lane-review-a | reviewer-session | ${laneWorktree} | lane/${laneId} | ${ownedPaths[0]} | ${taskIds.join(", ")} | ready |\n`);
  const refreshed = await loadCanonicalState(value.project);
  const lane = { id: laneId, teamId: "TEAM-REVIEW-A", role: "reviewer", model: "gpt-6-astra", effort: "high", queue: taskIds,
    worker: { host: "codex", sessionId: "reviewer-session", generation: 1 }, worktree: laneWorktree, branch: `lane/${laneId}`,
    brief: { path: `.agent-team/lanes/${laneId}/BRIEF.md`, sha256: digest(briefSource) }, ownershipEvidence: {
      path: `.agent-team/lanes/${laneId}/evidence/ownership.json`, sha256: digest(ownershipSource), revision, pathSetHash }, factSheets: [] };
  return nativeLaneRequest(value, "lane-create", { schemaVersion: 1, operationId: `create-${laneId}`,
    expectedTrackerFingerprint: refreshed.tracker.fingerprint, expectedRunFingerprint: effectiveRunFingerprint(refreshed.state.run),
    expectedRevision: revision, expectedLanesFingerprint: laneCollectionFingerprint(refreshed.state.lanes), lane,
    reason: "Create one reserved independent review lane without consuming author lane capacity." });
}

test("lane-create binds prepared worktree brief path evidence and queue without claiming tracker tasks", async () => {
  assert.equal(typeof createLane, "function");
  const value = await fixture();
  const before = await readFile(value.project.paths.tasks);
  const applied = await createLane(value.project, value.request, value.options);
  assert.equal(applied.status, "applied");
  assert.equal((await createLane(value.project, value.request, value.options)).status, "duplicate");
  const after = await loadCanonicalState(value.project);
  assert.deepEqual(after.state.lanes.records[0], {
    schemaVersion: 1,
    ...value.request.lane,
    status: "prepared",
    currentTaskId: null,
    rotationCount: 0,
    handover: null,
    factSheets: [],
    assignments: [],
    results: [],
  });
  assert.deepEqual(await readFile(value.project.paths.tasks), before);
});

test("lane-create rejects stale or unavailable path evidence without state changes", async () => {
  const value = await fixture();
  const before = await readFile(value.project.paths.state);
  const stale = await createLane(value.project, { ...value.request, operationId: "lane-create-stale", expectedRevision: "f".repeat(40) }, value.options);
  assert.equal(stale.reason, "stale_revision");
  assert.deepEqual(await readFile(value.project.paths.state), before);

  const evidencePath = path.join(value.project.root, value.request.lane.ownershipEvidence.path);
  const evidence = JSON.parse(await readFile(evidencePath, "utf8"));
  evidence.status = "unavailable";
  const source = json(evidence);
  await writeFile(evidencePath, source);
  const unavailable = await createLane(value.project, { ...value.request, operationId: "lane-create-unavailable",
    lane: { ...value.request.lane, ownershipEvidence: { ...value.request.lane.ownershipEvidence, sha256: digest(source) } } }, value.options);
  assert.equal(unavailable.reason, "ownership_evidence_unresolved");
  assert.deepEqual(await readFile(value.project.paths.state), before);
});

test("lane-create authenticates BRIEF revision and every instruction or skill file", async () => {
  const value = await fixture();
  const before = await readFile(value.project.paths.state);
  const briefPath = path.join(value.project.root, value.request.lane.brief.path);
  const original = await readFile(briefPath, "utf8");
  const attempt = async (operationId, source) => {
    await writeFile(briefPath, source);
    return createLane(value.project, { ...value.request, operationId, lane: { ...value.request.lane,
      brief: { ...value.request.lane.brief, sha256: digest(source) } } }, value.options);
  };
  assert.equal((await attempt("lane-create-stale-brief-context",
    original.replace(value.request.expectedRevision, "e".repeat(40)))).reason, "lane_brief_context_stale");
  assert.equal((await attempt("lane-create-changed-instruction",
    original.replace(/README\.md [a-f0-9]{64}/, `README.md ${"f".repeat(64)}`))).reason, "lane_instruction_changed");
  assert.equal((await attempt("lane-create-missing-instruction",
    original.replace(/README\.md [a-f0-9]{64}/, `MISSING.md ${"f".repeat(64)}`))).reason, "lane_instruction_unavailable");
  await symlink("README.md", path.join(value.project.root, "LINKED.md"));
  const readmeHash = digest(await readFile(path.join(value.project.root, "README.md")));
  assert.equal((await attempt("lane-create-linked-instruction",
    original.replace(/README\.md [a-f0-9]{64}/, `LINKED.md ${readmeHash}`))).reason, "lane_instruction_unsafe");
  assert.deepEqual(await readFile(value.project.paths.state), before);
});

test("dependent dispatch binds cited fact sheets and rejects changed or stale facts without state changes", async () => {
  const value = await fixture();
  const factPath = ".agent-team/lanes/build-a/facts/api-contract.json";
  const factRecord = { schemaVersion: 1, id: "api-contract", ownerLaneId: "build-a",
    facts: [{ id: "api-version", checkedOn: "2026-09-14T12:00:00.000Z", sourceUrl: "https://example.test/api" }] };
  const factSource = json(factRecord);
  const citationPath = ".agent-team/lanes/build-a/evidence/citations/AT-101-api-contract.json";
  const citationSource = json({ schemaVersion: 1, taskId: "AT-101", factSheetId: "api-contract",
    factIds: ["api-version"], sourceUrl: "https://example.test/api" });
  await mkdir(path.dirname(path.join(value.project.root, factPath)), { recursive: true });
  await mkdir(path.dirname(path.join(value.project.root, citationPath)), { recursive: true });
  await writeFile(path.join(value.project.root, factPath), factSource);
  await writeFile(path.join(value.project.root, citationPath), citationSource);
  value.request.lane.factSheets = [{ id: "api-contract", ownerLaneId: "build-a", path: factPath,
    sha256: digest(factSource), facts: factRecord.facts }];
  assert.equal((await createLane(value.project, value.request, value.options)).status, "applied");
  const next = await nextRequest(value);
  next.request.factSheets = [{ id: "api-contract", sha256: digest(factSource), factIds: ["api-version"],
    citation: { path: citationPath, sha256: digest(citationSource) } }];
  next.options.now = () => "2026-09-15T12:00:00.000Z";
  assert.equal((await advanceLane(value.project, next.request, next.options)).status, "applied");
  let canonical = await loadCanonicalState(value.project);
  let lane = canonical.state.lanes.records[0];
  assert.deepEqual(lane.assignments[0].factSheets, next.request.factSheets);
  const dispatch = { schemaVersion: 1, phase: "dispatch", operationId: "dispatch-fact-bound", laneId: lane.id,
    assignmentId: lane.assignments[0].id, expectedLaneFingerprint: laneFingerprint(lane), expectedRevision: lane.assignments[0].revision,
    expectedPacketSha256: lane.assignments[0].packet.sha256, worker: lane.worker,
    observation: { status: "observed", source: "codex", eventId: "fact-dispatch", observedAt: "2026-09-15T12:01:00.000Z" },
    reason: "Dispatch only while exact cited facts remain current." };
  const before = await readFile(value.project.paths.state);
  await writeFile(path.join(value.project.root, citationPath), `${citationSource} `);
  assert.equal((await advanceLane(value.project, dispatch, { ...next.options, expectedVersion: canonical.state.stateVersion })).reason,
    "fact_citation_changed");
  assert.deepEqual(await readFile(value.project.paths.state), before);
  await writeFile(path.join(value.project.root, citationPath), citationSource);
  assert.equal((await advanceLane(value.project, dispatch, { ...next.options, expectedVersion: canonical.state.stateVersion,
    now: () => "2026-09-22T12:00:00.001Z" })).reason, "fact_sheet_stale");
  canonical = await loadCanonicalState(value.project); lane = canonical.state.lanes.records[0];
  assert.equal(lane.assignments[0].status, "prepared");
});

test("lane-next claims only the first eligible queue task and records a prepared packet, not a dispatch", async () => {
  assert.equal(typeof advanceLane, "function");
  const value = await fixture();
  assert.equal((await createLane(value.project, value.request, value.options)).status, "applied");
  const next = await nextRequest(value);
  const applied = await advanceLane(value.project, next.request, next.options);
  assert.equal(applied.status, "applied");
  assert.equal((await advanceLane(value.project, next.request, next.options)).status, "duplicate");
  const canonical = await loadCanonicalState(value.project);
  const task = canonical.tasks.find(({ id }) => id === "AT-101");
  const lane = canonical.state.lanes.records[0];
  assert.deepEqual({ owner: task.owner, status: task.status }, { owner: "TEAM-LANE-A", status: "in_progress" });
  assert.equal(lane.currentTaskId, "AT-101");
  assert.equal(lane.status, "active");
  assert.equal(lane.assignments.length, 1);
  assert.deepEqual({ status: lane.assignments[0].status, dispatchedAt: lane.assignments[0].dispatchedAt }, { status: "prepared", dispatchedAt: null });
  assert.deepEqual(lane.assignments[0].decisions, [{ id: "DEC-AT-101-001", kind: "new", supersedes: null }]);
  assert.equal(applied.result.preparedOnly, true);
});

test("lane-next accepts an opaque assignment id because the sealed packet binds its task", async () => {
  const value = await fixture();
  assert.equal((await createLane(value.project, value.request, value.options)).status, "applied");
  const next = await nextRequest(value, "lane-next-opaque-id", "dispatch-token-1");
  const applied = await advanceLane(value.project, next.request, next.options);
  assert.equal(applied.status, "applied");
  assert.equal((await loadCanonicalState(value.project)).state.lanes.records[0].assignments[0].id, "dispatch-token-1");
});

test("lane-next records observed dispatch and one immutable worker outcome without minting gate authority", async () => {
  const value = await fixture();
  assert.equal((await createLane(value.project, value.request, value.options)).status, "applied");
  const next = await nextRequest(value);
  assert.equal((await advanceLane(value.project, next.request, next.options)).status, "applied");
  let canonical = await loadCanonicalState(value.project);
  let lane = canonical.state.lanes.records[0];
  const dispatch = {
    schemaVersion: 1,
    phase: "dispatch",
    operationId: "lane-dispatch-at-101",
    laneId: lane.id,
    assignmentId: lane.assignments[0].id,
    expectedLaneFingerprint: laneFingerprint(lane),
    expectedRevision: lane.assignments[0].revision,
    expectedPacketSha256: lane.assignments[0].packet.sha256,
    worker: lane.worker,
    observation: { status: "observed", source: "codex", eventId: "host-dispatch-101", observedAt: "2026-09-14T13:05:00.000Z" },
    reason: "Record the coordinator-observed host dispatch receipt.",
  };
  const dispatched = await advanceLane(value.project, dispatch, { ...value.options, expectedVersion: canonical.state.stateVersion });
  assert.equal(dispatched.status, "applied");
  assert.equal((await advanceLane(value.project, dispatch, { ...value.options, expectedVersion: canonical.state.stateVersion })).status, "duplicate");
  canonical = await loadCanonicalState(value.project);
  lane = canonical.state.lanes.records[0];
  assert.deepEqual(lane.assignments[0].dispatch, dispatch.observation);
  assert.equal(lane.assignments[0].status, "dispatched");

  const evidencePath = ".agent-team/lanes/build-a/evidence/worker-at-101.json";
  const evidenceSource = json({ taskId: "AT-101", assignmentId: dispatch.assignmentId, status: "passed" });
  await writeFile(path.join(value.project.root, evidencePath), evidenceSource);
  const resultRequest = {
    schemaVersion: 1,
    phase: "result",
    operationId: "lane-result-at-101",
    laneId: lane.id,
    assignmentId: dispatch.assignmentId,
    expectedLaneFingerprint: laneFingerprint(lane),
    expectedPacketSha256: lane.assignments[0].packet.sha256,
    result: { kind: "worker", status: "passed", revision: execFileSync("git", ["rev-parse", "HEAD"], { cwd: lane.worktree, encoding: "utf8" }).trim(),
      worker: lane.worker, evidence: { path: evidencePath, sha256: digest(evidenceSource) } },
    reason: "Record the observed worker outcome without changing canonical completion gates.",
  };
  const resolved = await advanceLane(value.project, resultRequest, { ...value.options, expectedVersion: canonical.state.stateVersion,
    now: () => "2026-09-14T13:00:00.000Z" });
  assert.equal(resolved.status, "applied");
  canonical = await loadCanonicalState(value.project);
  lane = canonical.state.lanes.records[0];
  assert.equal(lane.assignments[0].status, "resolved");
  assert.deepEqual(lane.results[0], { assignmentId: dispatch.assignmentId, taskId: "AT-101", ...resultRequest.result,
    recordedAt: "2026-09-14T13:00:00.000Z" });
  assert.equal(canonical.deliveryEvidence["AT-101"], undefined);

  const gateRequest = (operationId, kind, worker, evidencePath, evidenceSource) => ({ schemaVersion: 1, phase: "result", operationId,
    laneId: lane.id, assignmentId: dispatch.assignmentId, expectedLaneFingerprint: laneFingerprint(lane),
    expectedPacketSha256: lane.assignments[0].packet.sha256,
    result: { kind, status: "passed", revision: resultRequest.result.revision, worker,
      evidence: { path: evidencePath, sha256: digest(evidenceSource) } }, reason: `Record ${kind} evidence in required order.` });
  const reviewWorker = { host: "codex", sessionId: "reviewer-session", generation: 1 };
  const verifierWorker = { host: "codex", sessionId: "verifier-session", generation: 1 };
  const verifyPath = ".agent-team/lanes/build-a/evidence/verify-at-101.json";
  const verifyRecord = { schemaVersion: 1, assignmentId: dispatch.assignmentId, taskId: "AT-101", revision: resultRequest.result.revision,
    verifier: verifierWorker, source: { revision: resultRequest.result.revision }, checks: [{ name: "fixture", status: "passed" }], status: "passed" };
  const verifySource = json(verifyRecord);
  const reviewRecord = { schemaVersion: 1, assignmentId: dispatch.assignmentId, taskId: "AT-101", revision: resultRequest.result.revision,
    reviewer: reviewWorker, source: { revision: resultRequest.result.revision, readFirst: true },
    diff: { baseRevision: lane.assignments[0].revision, sourceRevision: resultRequest.result.revision },
    verifierEvidence: { path: verifyPath, sha256: digest(verifySource) }, rubric: ["Correctness", "Regression safety"],
    challenges: [{ scenario: "A stale revision is supplied.", testEvidence: "The stale binding regression refuses it." }],
    testsExercised: true, status: "passed" };
  const reviewSource = json(reviewRecord);
  const reviewPath = ".agent-team/lanes/build-a/evidence/review-at-101.json";
  const earlyReview = gateRequest("lane-review-too-early", "independent_review", reviewWorker, reviewPath, reviewSource);
  assert.equal((await advanceLane(value.project, earlyReview, { ...value.options, expectedVersion: canonical.state.stateVersion })).reason,
    "review_lane_required");
  const sameVerifier = gateRequest("lane-verifier-same-author", "verification", lane.worker,
    ".agent-team/lanes/build-a/evidence/verify-at-101.json", json({ assignmentId: dispatch.assignmentId, kind: "verification" }));
  assert.equal((await advanceLane(value.project, sameVerifier, { ...value.options, expectedVersion: canonical.state.stateVersion })).reason,
    "verification_not_independent");
  await writeFile(path.join(value.project.root, verifyPath), verifySource);
  const verification = gateRequest("lane-verifier-at-101", "verification", verifierWorker, verifyPath, verifySource);
  assert.equal((await advanceLane(value.project, verification, { ...value.options, expectedVersion: canonical.state.stateVersion })).status, "applied");
  canonical = await loadCanonicalState(value.project); lane = canonical.state.lanes.records[0];
  const weakReviewSource = json({ ...reviewRecord, challenges: [] });
  await writeFile(path.join(value.project.root, reviewPath), weakReviewSource);
  const weakReview = gateRequest("lane-review-without-challenge", "independent_review", reviewWorker, reviewPath, weakReviewSource);
  weakReview.expectedLaneFingerprint = laneFingerprint(lane);
  assert.equal((await advanceLane(value.project, weakReview, { ...value.options, expectedVersion: canonical.state.stateVersion })).reason,
    "review_lane_required");
  await writeFile(path.join(value.project.root, reviewPath), reviewSource);
  const review = gateRequest("lane-review-at-101", "independent_review", reviewWorker, reviewPath, reviewSource);
  review.expectedLaneFingerprint = laneFingerprint(lane);
  assert.equal((await advanceLane(value.project, review, { ...value.options, expectedVersion: canonical.state.stateVersion })).reason,
    "review_lane_required");
  canonical = await registerReviewerLane(value);
  lane = canonical.state.lanes.records.find(({ id }) => id === "build-a");
  let reviewerLane = canonical.state.lanes.records.find(({ id }) => id === "review-a");
  const reviewAssignmentId = "review-assignment-at-101-1";
  const sourceAssignment = { laneId: lane.id, assignmentId: dispatch.assignmentId,
    packetSha256: lane.assignments[0].packet.sha256, revision: resultRequest.result.revision };
  const reviewPacketSource = packetSource({ assignmentId: reviewAssignmentId, taskId: "AT-101", attempt: 1,
    revision: canonical.git.headRevision, trackerFingerprint: canonical.tracker.fingerprint,
    laneFingerprint: laneFingerprint(reviewerLane), briefSha256: reviewerLane.brief.sha256 })
    .replace("- DEC-AT-101-001 | new | none", "None.");
  const reviewPacketPath = ".agent-team/lanes/review-a/packets/AT-101-1.md";
  await mkdir(path.dirname(path.join(value.project.root, reviewPacketPath)), { recursive: true });
  await writeFile(path.join(value.project.root, reviewPacketPath), reviewPacketSource);
  canonical = await nativeLaneRequest(value, "lane-next", { schemaVersion: 1, phase: "prepare", operationId: "prepare-review-at-101",
    laneId: reviewerLane.id, assignmentId: reviewAssignmentId, attempt: 1, expectedTrackerFingerprint: canonical.tracker.fingerprint,
    expectedRunFingerprint: effectiveRunFingerprint(canonical.state.run), expectedRevision: canonical.git.headRevision,
    expectedLaneFingerprint: laneFingerprint(reviewerLane), packet: { path: reviewPacketPath, sha256: digest(reviewPacketSource) },
    factSheets: [], sourceAssignment, reason: "Prepare an independent review bound to the passed author assignment." });
  reviewerLane = canonical.state.lanes.records.find(({ id }) => id === "review-a");
  canonical = await nativeLaneRequest(value, "lane-next", { schemaVersion: 1, phase: "dispatch", operationId: "dispatch-review-at-101",
    laneId: reviewerLane.id, assignmentId: reviewAssignmentId, expectedLaneFingerprint: laneFingerprint(reviewerLane),
    expectedRevision: reviewerLane.assignments[0].revision, expectedPacketSha256: reviewerLane.assignments[0].packet.sha256,
    worker: reviewerLane.worker, observation: { status: "observed", source: "codex", eventId: "review-dispatch-at-101",
      observedAt: "2026-09-14T13:10:00.000Z" }, reason: "Record actual reviewer dispatch." });
  reviewerLane = canonical.state.lanes.records.find(({ id }) => id === "review-a");
  const boundReviewPath = ".agent-team/lanes/review-a/evidence/review-at-101.json";
  const boundReviewSource = json({ ...reviewRecord, assignmentId: reviewAssignmentId, sourceAssignment });
  await writeFile(path.join(value.project.root, boundReviewPath), boundReviewSource);
  canonical = await nativeLaneRequest(value, "lane-next", { schemaVersion: 1, phase: "result", operationId: "result-review-at-101",
    laneId: reviewerLane.id, assignmentId: reviewAssignmentId, expectedLaneFingerprint: laneFingerprint(reviewerLane),
    expectedPacketSha256: reviewerLane.assignments[0].packet.sha256,
    result: { kind: "independent_review", status: "passed", revision: resultRequest.result.revision, worker: reviewerLane.worker,
      evidence: { path: boundReviewPath, sha256: digest(boundReviewSource) } }, reason: "Record source-first review from the registered reviewer lane." });
  lane = canonical.state.lanes.records.find(({ id }) => id === "build-a");
  reviewerLane = canonical.state.lanes.records.find(({ id }) => id === "review-a");
  assert.equal(reviewerLane.assignments[0].status, "resolved");
  assert.equal(reviewerLane.results[0].kind, "independent_review");
  assert.equal(lane.results.find(({ kind }) => kind === "independent_review").worker.sessionId, "reviewer-session");
  const integration = gateRequest("lane-integration-without-authority", "integration",
    { host: "codex", sessionId: "owner-session", generation: 1 }, ".agent-team/lanes/build-a/evidence/integration-at-101.json",
    json({ assignmentId: dispatch.assignmentId, kind: "integration" }));
  integration.expectedLaneFingerprint = laneFingerprint(lane);
  assert.equal((await advanceLane(value.project, integration, { ...value.options, expectedVersion: canonical.state.stateVersion })).reason,
    "integration_evidence_mismatch");
});

test("lane-next dispatch/result reject out-of-order foreign and stale bindings without state changes", async () => {
  const value = await fixture();
  assert.equal((await createLane(value.project, value.request, value.options)).status, "applied");
  const next = await nextRequest(value);
  assert.equal((await advanceLane(value.project, next.request, next.options)).status, "applied");
  const canonical = await loadCanonicalState(value.project);
  const lane = canonical.state.lanes.records[0];
  const before = await readFile(value.project.paths.state);
  const base = { schemaVersion: 1, phase: "dispatch", operationId: "bad-dispatch", laneId: lane.id,
    assignmentId: lane.assignments[0].id, expectedLaneFingerprint: laneFingerprint(lane), expectedRevision: lane.assignments[0].revision,
    expectedPacketSha256: lane.assignments[0].packet.sha256, worker: lane.worker,
    observation: { status: "observed", source: "codex", eventId: "dispatch-event", observedAt: "2026-09-14T13:05:00.000Z" }, reason: "Reject altered bindings." };
  assert.equal((await advanceLane(value.project, { ...base, worker: { ...lane.worker, sessionId: "other-worker" } },
    { ...value.options, expectedVersion: canonical.state.stateVersion })).reason, "worker_identity_mismatch");
  assert.equal((await advanceLane(value.project, { ...base, operationId: "stale-dispatch", expectedLaneFingerprint: "f".repeat(64) },
    { ...value.options, expectedVersion: canonical.state.stateVersion })).reason, "stale_lane");
  assert.deepEqual(await readFile(value.project.paths.state), before);
});

test("lane-next UNKNOWN dispatch outcome stays unresolved and cannot be hidden by later history", async () => {
  const value = await fixture();
  assert.equal((await createLane(value.project, value.request, value.options)).status, "applied");
  const next = await nextRequest(value);
  assert.equal((await advanceLane(value.project, next.request, next.options)).status, "applied");
  let canonical = await loadCanonicalState(value.project);
  let lane = canonical.state.lanes.records[0];
  const dispatch = { schemaVersion: 1, phase: "dispatch", operationId: "lane-dispatch-unknown", laneId: lane.id,
    assignmentId: lane.assignments[0].id, expectedLaneFingerprint: laneFingerprint(lane), expectedRevision: lane.assignments[0].revision,
    expectedPacketSha256: lane.assignments[0].packet.sha256, worker: lane.worker,
    observation: { status: "unknown", source: "unavailable", eventId: null, observedAt: "2026-09-14T13:05:00.000Z" },
    reason: "Preserve unavailable host observation as UNKNOWN." };
  assert.equal((await advanceLane(value.project, dispatch, { ...value.options, expectedVersion: canonical.state.stateVersion })).status, "applied");
  canonical = await loadCanonicalState(value.project);
  lane = canonical.state.lanes.records[0];
  const before = await readFile(value.project.paths.state);
  const falsePassPath = ".agent-team/lanes/build-a/evidence/false-pass-at-101.json";
  const falsePassSource = json({ taskId: "AT-101", assignmentId: dispatch.assignmentId, status: "passed" });
  await writeFile(path.join(value.project.root, falsePassPath), falsePassSource);
  const falsePass = { schemaVersion: 1, phase: "result", operationId: "lane-result-false-pass", laneId: lane.id,
    assignmentId: dispatch.assignmentId, expectedLaneFingerprint: laneFingerprint(lane), expectedPacketSha256: lane.assignments[0].packet.sha256,
    result: { kind: "worker", status: "passed", revision: lane.assignments[0].revision, worker: lane.worker,
      evidence: { path: falsePassPath, sha256: digest(falsePassSource) } }, reason: "An unobserved dispatch cannot become a passing result." };
  assert.equal((await advanceLane(value.project, falsePass, { ...value.options, expectedVersion: canonical.state.stateVersion })).reason,
    "dispatch_unobserved");
  assert.deepEqual(await readFile(value.project.paths.state), before);
  const evidencePath = ".agent-team/lanes/build-a/evidence/unresolved-at-101.json";
  const evidenceSource = json({ taskId: "AT-101", assignmentId: dispatch.assignmentId, status: "unresolved" });
  await writeFile(path.join(value.project.root, evidencePath), evidenceSource);
  const result = { schemaVersion: 1, phase: "result", operationId: "lane-result-unknown", laneId: lane.id,
    assignmentId: dispatch.assignmentId, expectedLaneFingerprint: laneFingerprint(lane), expectedPacketSha256: lane.assignments[0].packet.sha256,
    result: { kind: "unresolved", status: "unresolved", revision: lane.assignments[0].revision, worker: lane.worker,
      evidence: { path: evidencePath, sha256: digest(evidenceSource) } }, reason: "Record the required unresolved disposition." };
  assert.equal((await advanceLane(value.project, result, { ...value.options, expectedVersion: canonical.state.stateVersion })).status, "applied");
  lane = (await loadCanonicalState(value.project)).state.lanes.records[0];
  assert.equal(lane.assignments[0].status, "dispatched");
  assert.equal(lane.results[0].status, "unresolved");
});

test("lane-next retries a definite failed worker attempt without reclaiming the retained tracker task", async () => {
  const value = await fixture();
  assert.equal((await createLane(value.project, value.request, value.options)).status, "applied");
  const next = await nextRequest(value);
  assert.equal((await advanceLane(value.project, next.request, next.options)).status, "applied");
  let canonical = await loadCanonicalState(value.project);
  let lane = canonical.state.lanes.records[0];
  const assignment = lane.assignments[0];
  assert.equal((await advanceLane(value.project, { schemaVersion: 1, phase: "dispatch", operationId: "failed-dispatch-at-101",
    laneId: lane.id, assignmentId: assignment.id, expectedLaneFingerprint: laneFingerprint(lane), expectedRevision: assignment.revision,
    expectedPacketSha256: assignment.packet.sha256, worker: lane.worker,
    observation: { status: "observed", source: "codex", eventId: "failed-dispatch", observedAt: "2026-09-14T13:05:00.000Z" },
    reason: "Record the actual failed dispatch." }, { ...value.options, expectedVersion: canonical.state.stateVersion })).status, "applied");
  canonical = await loadCanonicalState(value.project); lane = canonical.state.lanes.records[0];
  const evidencePath = ".agent-team/lanes/build-a/evidence/failed-at-101.json";
  const evidenceSource = json({ assignmentId: assignment.id, taskId: assignment.taskId, status: "failed" });
  await writeFile(path.join(value.project.root, evidencePath), evidenceSource);
  assert.equal((await advanceLane(value.project, { schemaVersion: 1, phase: "result", operationId: "failed-result-at-101",
    laneId: lane.id, assignmentId: assignment.id, expectedLaneFingerprint: laneFingerprint(lane), expectedPacketSha256: assignment.packet.sha256,
    result: { kind: "worker", status: "failed", revision: assignment.revision, worker: lane.worker,
      evidence: { path: evidencePath, sha256: digest(evidenceSource) } }, reason: "Retain the definite failed outcome." },
  { ...value.options, expectedVersion: canonical.state.stateVersion })).status, "applied");
  canonical = await loadCanonicalState(value.project); lane = canonical.state.lanes.records[0];
  const trackerBefore = await readFile(value.project.paths.tasks);
  const retrySource = packetSource({ assignmentId: "assignment-at-101-2", taskId: "AT-101", attempt: 2,
    revision: canonical.git.headRevision, trackerFingerprint: canonical.tracker.fingerprint,
    laneFingerprint: laneFingerprint(lane), briefSha256: lane.brief.sha256 }).replace("- DEC-AT-101-001 | new | none", "None.");
  const retryPath = ".agent-team/lanes/build-a/packets/AT-101-2.md";
  await writeFile(path.join(value.project.root, retryPath), retrySource);
  const retried = await advanceLane(value.project, { schemaVersion: 1, phase: "prepare", operationId: "retry-failed-at-101",
    laneId: lane.id, assignmentId: "assignment-at-101-2", attempt: 2, expectedTrackerFingerprint: canonical.tracker.fingerprint,
    expectedRunFingerprint: effectiveRunFingerprint(canonical.state.run), expectedRevision: canonical.git.headRevision,
    expectedLaneFingerprint: laneFingerprint(lane), packet: { path: retryPath, sha256: digest(retrySource) }, factSheets: [],
    reason: "Retry the retained task after a definite failed attempt." }, { ...value.options, expectedVersion: canonical.state.stateVersion });
  assert.equal(retried.status, "applied", retried.reason);
  assert.equal(retried.result.claimedTrackerTask, false);
  assert.deepEqual(await readFile(value.project.paths.tasks), trackerBefore);
});

test("lane-rotate explicit transfer can invalidate the old identity before a replacement slot exists", async () => {
  const value = await fixture();
  assert.equal((await createLane(value.project, value.request, value.options)).status, "applied");
  const next = await nextRequest(value);
  assert.equal((await advanceLane(value.project, next.request, next.options)).status, "applied");
  const canonical = await loadCanonicalState(value.project);
  const lane = canonical.state.lanes.records[0];
  const sequence = 1;
  const handoverSource = `# Lane handover

## Voice
Continue as the retained implementation lane and report exact evidence.

## Gotchas
The tracker is the sole task authority.

## Open threads
Resume AT-101 without another tracker claim.

## Decisions
DEC-AT-101-001 remains in force.

## Exact revision
${canonical.git.headRevision}
`;
  const handover = { path: ".agent-team/lanes/build-a/HANDOVER-001.md", sha256: digest(handoverSource), revision: canonical.git.headRevision, sequence };
  await writeFile(path.join(value.project.root, handover.path), handoverSource);
  const authorization = { schemaVersion: 1, kind: "explicit_transfer", laneId: lane.id, outgoingWorker: lane.worker,
    replacementWorker: null, revision: canonical.git.headRevision, handoverSha256: handover.sha256,
    authorizedBy: { host: "codex", sessionId: "owner-session", ownershipEpoch: 1 },
    reason: "Rotate at explicit coordinator-observed context pressure.", observedAt: "2026-09-14T13:10:00.000Z" };
  const authorizationSource = json(authorization);
  const evidencePath = ".agent-team/lanes/build-a/evidence/rotation-001.json";
  await writeFile(path.join(value.project.root, evidencePath), authorizationSource);
  const request = { schemaVersion: 1, operationId: "lane-rotate-build-a-1", laneId: lane.id,
    expectedLaneFingerprint: laneFingerprint(lane), expectedRevision: canonical.git.headRevision, trigger: "pressure",
    outgoingWorker: lane.worker, replacementWorker: null, handover,
    transferEvidence: { kind: "explicit_transfer", path: evidencePath, sha256: digest(authorizationSource) },
    reason: authorization.reason };
  const requestPath = path.join(value.project.root, "lane-rotate.json");
  await writeFile(requestPath, json({ schemaVersion: 1, actorSessionId: "owner-session", expectedVersion: canonical.state.stateVersion, request }));
  const cli = path.resolve(import.meta.dirname, "../hooks/agent-team-cli.mjs");
  const hooked = await runNormalizedHook({ runtime: "codex", event: "PreToolUse", cwd: value.project.root, sessionId: "owner-session",
    eventId: "lane-rotate-native-event", operation: { kind: "shell", command: `node ${cli} lane-rotate --project ${value.project.root} --request ${requestPath}` } });
  assert.deepEqual(hooked.decision.mutations.find(({ kind }) => kind === "lane_rotate"), {
    kind: "lane_rotate", command: "lane-rotate", status: "applied",
  });
  assert.equal((await rotateLane(value.project, request, { ...value.options, expectedVersion: canonical.state.stateVersion })).status, "duplicate");
  const after = await loadCanonicalState(value.project);
  const rotated = after.state.lanes.records[0];
  assert.equal(rotated.worker, null);
  assert.equal(rotated.status, "rotation_required");
  assert.equal(rotated.rotationCount, 1);
  assert.deepEqual(rotated.handover, handover);
  assert.equal(rotated.currentTaskId, "AT-101");
  assert.deepEqual(rotated.assignments[0].worker, lane.worker);
  assert.equal(after.tasks.find(({ id }) => id === "AT-101").owner, "TEAM-LANE-A");
});

test("lane-rotate accepts authenticated stopped-writer evidence without prebinding replacement and lane-next bind resumes without reclaim", async () => {
  const value = await fixture();
  assert.equal((await createLane(value.project, value.request, value.options)).status, "applied");
  const next = await nextRequest(value);
  assert.equal((await advanceLane(value.project, next.request, next.options)).status, "applied");
  let canonical = await loadCanonicalState(value.project);
  let lane = canonical.state.lanes.records[0];
  const dispatch = { schemaVersion: 1, phase: "dispatch", operationId: "lane-dispatch-before-stopped", laneId: lane.id,
    assignmentId: lane.assignments[0].id, expectedLaneFingerprint: laneFingerprint(lane), expectedRevision: lane.assignments[0].revision,
    expectedPacketSha256: lane.assignments[0].packet.sha256, worker: lane.worker,
    observation: { status: "unknown", source: "unavailable", eventId: null, observedAt: "2026-09-14T13:05:00.000Z" },
    reason: "Preserve the uncertain first attempt before rotation." };
  assert.equal((await advanceLane(value.project, dispatch, { ...value.options, expectedVersion: canonical.state.stateVersion })).status, "applied");
  canonical = await loadCanonicalState(value.project); lane = canonical.state.lanes.records[0];
  const unresolvedPath = ".agent-team/lanes/build-a/evidence/unresolved-before-stopped.json";
  const unresolvedSource = json({ taskId: "AT-101", assignmentId: lane.assignments[0].id, status: "unresolved" });
  await writeFile(path.join(value.project.root, unresolvedPath), unresolvedSource);
  const unresolved = { schemaVersion: 1, phase: "result", operationId: "lane-unresolved-before-stopped", laneId: lane.id,
    assignmentId: lane.assignments[0].id, expectedLaneFingerprint: laneFingerprint(lane), expectedPacketSha256: lane.assignments[0].packet.sha256,
    result: { kind: "unresolved", status: "unresolved", revision: lane.assignments[0].revision, worker: lane.worker,
      evidence: { path: unresolvedPath, sha256: digest(unresolvedSource) } }, reason: "Record uncertainty before authenticated rotation." };
  assert.equal((await advanceLane(value.project, unresolved, { ...value.options, expectedVersion: canonical.state.stateVersion })).status, "applied");
  canonical = await loadCanonicalState(value.project); lane = canonical.state.lanes.records[0];
  const handoverSource = `# Lane handover

## Voice
Continue as the retained implementation lane and report exact evidence.

## Gotchas
The tracker is the sole task authority.

## Open threads
Resume AT-101 without another tracker claim.

## Decisions
DEC-AT-101-001 remains in force.

## Exact revision
${canonical.git.headRevision}
`;
  const handover = { path: ".agent-team/lanes/build-a/HANDOVER-001.md", sha256: digest(handoverSource), revision: canonical.git.headRevision, sequence: 1 };
  await writeFile(path.join(value.project.root, handover.path), handoverSource);
  const rotate = { schemaVersion: 1, operationId: "lane-rotate-stopped", laneId: lane.id,
    expectedLaneFingerprint: laneFingerprint(lane), expectedRevision: canonical.git.headRevision, trigger: "pressure",
    outgoingWorker: lane.worker, replacementWorker: null, handover,
    transferEvidence: { kind: "stopped_writer", observationId: "host-stop-observation-1" }, reason: "Rotate after authenticated stopped-writer observation." };
  const stopped = { status: "stopped", host: lane.worker.host, sessionId: lane.worker.sessionId, generation: lane.worker.generation,
    observationId: rotate.transferEvidence.observationId, source: "native_host", observedAt: "2026-09-14T13:10:00.000Z" };
  assert.equal((await rotateLane(value.project, rotate, { ...value.options, expectedVersion: canonical.state.stateVersion,
    inspectLaneSession: async () => stopped })).status, "applied");
  canonical = await loadCanonicalState(value.project);
  lane = canonical.state.lanes.records[0];
  assert.equal(lane.worker, null);
  assert.equal(lane.status, "rotation_required");
  assert.equal(lane.currentTaskId, "AT-101");
  const beforeTask = canonical.tasks.find(({ id }) => id === "AT-101");

  const replacementWorker = { host: "codex", sessionId: "replacement-worker", generation: 2 };
  const teams = await readFile(value.project.paths.teams, "utf8");
  await writeFile(value.project.paths.teams, teams.replace("| lane-worker |", "| replacement-worker |"));
  canonical = await loadCanonicalState(value.project);
  lane = canonical.state.lanes.records[0];
  const bind = { schemaVersion: 1, phase: "bind", operationId: "lane-bind-replacement", laneId: lane.id,
    expectedLaneFingerprint: laneFingerprint(lane), expectedRevision: canonical.git.headRevision,
    expectedHandoverSha256: lane.handover.sha256, replacementWorker, reason: "Bind the registered replacement without a new claim." };
  assert.equal((await advanceLane(value.project, bind, { ...value.options, expectedVersion: canonical.state.stateVersion })).status, "applied");
  const after = await loadCanonicalState(value.project);
  assert.deepEqual(after.state.lanes.records[0].worker, replacementWorker);
  assert.equal(after.state.lanes.records[0].currentTaskId, "AT-101");
  assert.deepEqual(after.state.lanes.records[0].assignments[0].worker, rotate.outgoingWorker);
  assert.deepEqual(after.tasks.find(({ id }) => id === "AT-101"), beforeTask);

  const reboundLane = after.state.lanes.records[0];
  const retrySource = packetSource({ assignmentId: "assignment-at-101-2", taskId: "AT-101", attempt: 2,
    revision: after.git.headRevision, trackerFingerprint: after.tracker.fingerprint,
    laneFingerprint: laneFingerprint(reboundLane), briefSha256: reboundLane.brief.sha256 })
    .replace("- DEC-AT-101-001 | new | none", "None.");
  const retryPath = ".agent-team/lanes/build-a/packets/AT-101-2.md";
  await writeFile(path.join(value.project.root, retryPath), retrySource);
  const retried = await advanceLane(value.project, { schemaVersion: 1, phase: "prepare", operationId: "lane-retry-at-101",
    laneId: reboundLane.id, assignmentId: "assignment-at-101-2", attempt: 2, expectedTrackerFingerprint: after.tracker.fingerprint,
    expectedRunFingerprint: effectiveRunFingerprint(after.state.run), expectedRevision: after.git.headRevision,
    expectedLaneFingerprint: laneFingerprint(reboundLane), packet: { path: retryPath, sha256: digest(retrySource) }, factSheets: [],
    reason: "Retry the retained claim only after the uncertain writer was stopped and replaced." },
  { ...value.options, expectedVersion: after.state.stateVersion });
  assert.equal(retried.status, "applied", retried.reason);
  const retryState = await loadCanonicalState(value.project);
  assert.equal(retryState.state.lanes.records[0].assignments.length, 2);
  assert.equal(retryState.state.lanes.records[0].assignments[1].attempt, 2);
  assert.equal(retryState.tasks.find(({ id }) => id === "AT-101").owner, "TEAM-LANE-A");
});

test("lane-close refuses an active writer and incomplete history without changing state", async () => {
  const value = await fixture();
  assert.equal((await createLane(value.project, value.request, value.options)).status, "applied");
  let canonical = await loadCanonicalState(value.project);
  let lane = canonical.state.lanes.records[0];
  const request = { schemaVersion: 1, operationId: "lane-close-build-a", laneId: lane.id, expectedLaneFingerprint: laneFingerprint(lane),
    expectedTrackerFingerprint: canonical.tracker.fingerprint, expectedRunFingerprint: effectiveRunFingerprint(canonical.state.run),
    expectedRevision: canonical.git.headRevision, writerRelease: null, reason: "Close only after the entire lane history is accepted." };
  const before = await readFile(value.project.paths.state);
  assert.equal((await closeLane(value.project, request, { ...value.options, expectedVersion: canonical.state.stateVersion })).reason, "lane_writer_not_released");
  assert.deepEqual(await readFile(value.project.paths.state), before);

  const changed = structuredClone(canonical.state);
  changed.lanes.records[0].worker = null;
  changed.lanes.records[0].status = "rotation_required";
  await writeFile(value.project.paths.state, json(changed));
  canonical = await loadCanonicalState(value.project);
  lane = canonical.state.lanes.records[0];
  const incomplete = { ...request, operationId: "lane-close-incomplete", expectedLaneFingerprint: laneFingerprint(lane) };
  assert.equal((await closeLane(value.project, incomplete, { ...value.options, expectedVersion: canonical.state.stateVersion })).reason,
    "lane_assignment_unresolved");
});

async function installAcceptedLaneHistory(value) {
  let canonical = await loadCanonicalState(value.project);
  await mkdir(path.join(value.project.root, ".agent-team/lanes/review-a/evidence"), { recursive: true });
  const revision = canonical.git.headRevision;
  const state = structuredClone(canonical.state);
  const lane = state.lanes.records[0];
  const actor = { ownerHost: "codex", ownerSessionId: "owner-session", ownershipEpoch: 1 };
  const completionPointers = {};
  const integrationPointer = { path: path.join(value.project.root, ".agent-team/integration.json"), fingerprint: "9".repeat(64), revision,
    taskIds: [...lane.queue], operationId: "integration-lane", observedAt: "2026-09-14T14:30:00.000Z" };
  const assignments = [];
  const results = [];
  const reviewAssignments = [];
  const reviewResults = [];
  for (const [index, taskId] of lane.queue.entries()) {
    const assignmentId = `accepted-${taskId.toLowerCase()}-1`;
    const assignment = { id: assignmentId, taskId, attempt: 1,
      packet: { path: `.agent-team/lanes/build-a/packets/${taskId}-1.md`, sha256: String(index + 2).repeat(64) },
      briefSha256: lane.brief.sha256, revision, worker: lane.worker, decisions: [], factSheets: [], status: "resolved",
      createdAt: "2026-09-14T13:00:00.000Z", dispatchedAt: "2026-09-14T13:01:00.000Z",
      dispatch: { status: "observed", source: "codex", eventId: `dispatch-${taskId}`, observedAt: "2026-09-14T13:01:00.000Z" } };
    assignments.push(assignment);
    for (const [kind, worker] of [["worker", lane.worker], ["verification", { host: "codex", sessionId: `verifier-${taskId}`, generation: 1 }],
      ["independent_review", { host: "codex", sessionId: "reviewer-session", generation: 1 }], ["integration", { host: "codex", sessionId: "owner-session", generation: 1 }]]) {
      const evidencePath = `.agent-team/lanes/${kind === "independent_review" ? "review-a" : "build-a"}/evidence/${taskId}-${kind}.json`;
      const source = json({ assignmentId, taskId, kind, status: "passed", revision });
      await writeFile(path.join(value.project.root, evidencePath), source);
      results.push({ assignmentId, taskId, kind, status: "passed", revision, worker,
        evidence: { path: evidencePath, sha256: digest(source) }, recordedAt: `2026-09-14T14:0${results.length}:00.000Z` });
    }
    const sourceReview = results.at(-2);
    const reviewAssignment = { id: `review-${assignmentId}`, taskId, attempt: 1,
      packet: { path: `.agent-team/lanes/review-a/packets/${taskId}-1.md`, sha256: String(index + 7).repeat(64) },
      briefSha256: "8".repeat(64), revision, worker: sourceReview.worker, decisions: [], factSheets: [],
      sourceAssignment: { laneId: lane.id, assignmentId, packetSha256: assignment.packet.sha256, revision }, status: "resolved",
      createdAt: "2026-09-14T13:30:00.000Z", dispatchedAt: "2026-09-14T13:31:00.000Z",
      dispatch: { status: "observed", source: "codex", eventId: `review-dispatch-${taskId}`, observedAt: "2026-09-14T13:31:00.000Z" } };
    reviewAssignments.push(reviewAssignment);
    reviewResults.push({ ...sourceReview, assignmentId: reviewAssignment.id });
    completionPointers[taskId] = { path: path.join(value.project.root, `.agent-team/completion-${taskId}.json`), fingerprint: String(index + 5).repeat(64),
      revision, taskIds: [taskId], operationId: `completion-${taskId}`, observedAt: "2026-09-14T14:00:00.000Z" };
  }
  Object.assign(lane, { status: "rotation_required", worker: null, currentTaskId: lane.queue.at(-1), assignments, results });
  state.lanes.records.push({ schemaVersion: 1, id: "review-a", teamId: "TEAM-REVIEW-A", status: "closed", role: "reviewer",
    model: "gpt-6-astra", effort: "high", queue: [...lane.queue], currentTaskId: null, worker: null,
    worktree: `${value.project.root}-lane-review-a`, branch: "lane/review-a",
    brief: { path: ".agent-team/lanes/review-a/BRIEF.md", sha256: "8".repeat(64) }, ownershipEvidence: {
      path: ".agent-team/lanes/review-a/evidence/ownership.json", sha256: "9".repeat(64), revision, pathSetHash: "a".repeat(64) },
    rotationCount: 0, handover: null, factSheets: [], assignments: reviewAssignments, results: reviewResults });
  state.completion = { ...state.completion, taskId: lane.queue.at(-1), recordedEvidenceByTask: completionPointers };
  state.integration = { ...state.integration, taskIds: [...lane.queue], recordedEvidenceByTask: Object.fromEntries(lane.queue.map((id) => [id, integrationPointer])) };
  state.release = { ...state.release, authorization: { ...state.release.authorization, ...actor } };
  const authority = { status: "authorized", source: "explicit-release-authorization", target: "origin/main", revision,
    taskIds: [...lane.queue], ...actor };
  state.deliveryReceipts = Object.fromEntries(["completion", "review", "checks", "integration", "preview", "target", "recovery"].map((key) => [key, {}]));
  for (const taskId of lane.queue) {
    const completion = completionPointers[taskId];
    state.deliveryReceipts.completion[taskId] = { taskId, status: "passed", sourceRevision: revision, evidence: completion };
    state.deliveryReceipts.review[taskId] = { taskId, status: "passed", revision, evidence: completion };
    state.deliveryReceipts.checks[taskId] = { taskId, revision, results: [{ name: "fixture", status: "passed" }], evidence: completion };
    state.deliveryReceipts.integration[taskId] = { taskId, status: "passed", sourceRevision: revision, boundaryRevision: revision,
      integratedRevision: revision, integrationOperationId: integrationPointer.operationId, ...actor, evidence: integrationPointer };
    state.deliveryReceipts.preview[taskId] = { taskId, revision, required: false, status: "not_required", ...actor, evidence: integrationPointer };
    state.deliveryReceipts.target[taskId] = { taskId, revision, status: "authorized", target: "origin/main", authority, ...actor, evidence: integrationPointer };
    state.deliveryReceipts.recovery[taskId] = { taskId, revision, status: "ready", artifact: "fixture-tag", action: "rollback", ...actor, evidence: integrationPointer };
  }
  await writeFile(value.project.paths.state, json(state));
  await writeFile(value.project.paths.tasks, `# Agent-Team Tasks
| ID | Requirement / acceptance | Owner | Depends on | Status | Revision / evidence | Next action |
| --- | --- | --- | --- | --- | --- | --- |
| AT-101 | First lane task | TEAM-LANE-A | none | done | ${revision} | Integrated. |
| AT-102 | Second lane task | TEAM-LANE-A | AT-101 | done | ${revision} | Integrated. |
`);
  const teams = await readFile(value.project.paths.teams, "utf8");
  await writeFile(value.project.paths.teams, `${teams.trimEnd()}\n| TEAM-REVIEW-A | lane-review-a | reviewer-session | ${value.project.root}-lane-review-a | lane/review-a | .agent-team/lanes/review-a/** | AT-101, AT-102 | ready |\n`);
  return loadCanonicalState(value.project);
}

test("lane-close accepts only fully ordered lane results joined to canonical integration and retains history", async () => {
  const value = await fixture();
  assert.equal((await createLane(value.project, value.request, value.options)).status, "applied");
  const canonical = await installAcceptedLaneHistory(value);
  const lane = canonical.state.lanes.records[0];
  assert.deepEqual(Object.keys(canonical.deliveryEvidence).sort(), ["AT-101", "AT-102"]);
  const request = { schemaVersion: 1, operationId: "lane-close-accepted", laneId: lane.id, expectedLaneFingerprint: laneFingerprint(lane),
    expectedTrackerFingerprint: canonical.tracker.fingerprint, expectedRunFingerprint: effectiveRunFingerprint(canonical.state.run),
    expectedRevision: canonical.git.headRevision, writerRelease: null, reason: "Close the fully accepted lane while retaining its evidence." };
  const requestPath = path.join(value.project.root, "lane-close.json");
  await writeFile(requestPath, json({ schemaVersion: 1, actorSessionId: "owner-session", expectedVersion: canonical.state.stateVersion, request }));
  const cli = path.resolve(import.meta.dirname, "../hooks/agent-team-cli.mjs");
  const hooked = await runNormalizedHook({ runtime: "codex", event: "PreToolUse", cwd: value.project.root, sessionId: "owner-session",
    eventId: "lane-close-native-event", operation: { kind: "shell", command: `node ${cli} lane-close --project ${value.project.root} --request ${requestPath}` } });
  assert.deepEqual(hooked.decision.mutations.find(({ kind }) => kind === "lane_close"), {
    kind: "lane_close", command: "lane-close", status: "applied",
  });
  assert.equal((await closeLane(value.project, request, { ...value.options, expectedVersion: canonical.state.stateVersion })).status, "duplicate");
  const closed = (await loadCanonicalState(value.project)).state.lanes.records[0];
  assert.equal(closed.status, "closed");
  assert.equal(closed.worker, null);
  assert.equal(closed.currentTaskId, null);
  assert.equal(closed.assignments.length, 2);
  assert.equal(closed.results.length, 8);
});

async function nativeLaneRequest(value, command, request, options = {}) {
  const canonical = await loadCanonicalState(value.project);
  const requestPath = path.join(value.project.root, ".agent-team/requests", `${request.operationId}.json`);
  await mkdir(path.dirname(requestPath), { recursive: true });
  await writeFile(requestPath, json({ schemaVersion: 1, actorSessionId: "owner-session", expectedVersion: canonical.state.stateVersion, request }));
  const cli = path.resolve(import.meta.dirname, "../hooks/agent-team-cli.mjs");
  const outcome = await runNormalizedHook({ runtime: "codex", event: "PreToolUse", cwd: value.project.root, sessionId: "owner-session",
    eventId: `${request.operationId}-event`, operation: { kind: "shell",
      command: `node ${cli} ${command} --project ${value.project.root} --request ${requestPath}` } }, options);
  assert.equal(outcome.decision.allow, true, outcome.decision.messages.join("\n"));
  assert.equal(outcome.decision.mutations.find(({ command: name }) => name === command)?.status, "applied");
  return loadCanonicalState(value.project);
}

async function runLaneTask(value, laneId, taskId, attempt) {
  let canonical = await loadCanonicalState(value.project);
  let lane = canonical.state.lanes.records.find(({ id }) => id === laneId);
  const assignmentId = `${laneId}-${taskId.toLowerCase()}-${attempt}`;
  const source = packetSource({ assignmentId, taskId, attempt, revision: canonical.git.headRevision,
    trackerFingerprint: canonical.tracker.fingerprint, laneFingerprint: laneFingerprint(lane), briefSha256: lane.brief.sha256 })
    .replace("DEC-AT-101-001", `DEC-${taskId}-001`);
  const packetPath = `.agent-team/lanes/${laneId}/packets/${taskId}-${attempt}.md`;
  await mkdir(path.dirname(path.join(value.project.root, packetPath)), { recursive: true });
  await writeFile(path.join(value.project.root, packetPath), source);
  canonical = await nativeLaneRequest(value, "lane-next", { schemaVersion: 1, phase: "prepare", operationId: `prepare-${assignmentId}`,
    laneId, assignmentId, attempt, expectedTrackerFingerprint: canonical.tracker.fingerprint,
    expectedRunFingerprint: effectiveRunFingerprint(canonical.state.run), expectedRevision: canonical.git.headRevision,
    expectedLaneFingerprint: laneFingerprint(lane), packet: { path: packetPath, sha256: digest(source) }, factSheets: [], reason: `Prepare ${taskId}.` });
  lane = canonical.state.lanes.records.find(({ id }) => id === laneId);
  const assignment = lane.assignments.find(({ id }) => id === assignmentId);
  canonical = await nativeLaneRequest(value, "lane-next", { schemaVersion: 1, phase: "dispatch", operationId: `dispatch-${assignmentId}`,
    laneId, assignmentId, expectedLaneFingerprint: laneFingerprint(lane), expectedRevision: assignment.revision,
    expectedPacketSha256: assignment.packet.sha256, worker: lane.worker,
    observation: { status: "observed", source: lane.worker.host, eventId: `host-${assignmentId}`, observedAt: "2026-09-14T15:00:00.000Z" },
    reason: `Record observed dispatch for ${taskId}.` });
  lane = canonical.state.lanes.records.find(({ id }) => id === laneId);
  const resultRevision = canonical.git.headRevision;
  const workerPath = `.agent-team/lanes/${laneId}/evidence/${assignmentId}-worker.json`;
  const workerSource = json({ assignmentId, taskId, status: "passed", revision: resultRevision });
  await writeFile(path.join(value.project.root, workerPath), workerSource);
  canonical = await nativeLaneRequest(value, "lane-next", { schemaVersion: 1, phase: "result", operationId: `worker-${assignmentId}`,
    laneId, assignmentId, expectedLaneFingerprint: laneFingerprint(lane), expectedPacketSha256: assignment.packet.sha256,
    result: { kind: "worker", status: "passed", revision: resultRevision, worker: lane.worker,
      evidence: { path: workerPath, sha256: digest(workerSource) } }, reason: `Record worker result for ${taskId}.` });
  lane = canonical.state.lanes.records.find(({ id }) => id === laneId);
  const verifier = { host: "codex", sessionId: `verifier-${taskId.toLowerCase()}`, generation: 1 };
  const verifyPath = `.agent-team/lanes/${laneId}/evidence/${assignmentId}-verification.json`;
  const verifySource = json({ schemaVersion: 1, assignmentId, taskId, revision: resultRevision, verifier,
    source: { revision: resultRevision }, checks: [{ name: "fixture", status: "passed" }], status: "passed" });
  await writeFile(path.join(value.project.root, verifyPath), verifySource);
  canonical = await nativeLaneRequest(value, "lane-next", { schemaVersion: 1, phase: "result", operationId: `verify-${assignmentId}`,
    laneId, assignmentId, expectedLaneFingerprint: laneFingerprint(lane), expectedPacketSha256: assignment.packet.sha256,
    result: { kind: "verification", status: "passed", revision: resultRevision, worker: verifier,
      evidence: { path: verifyPath, sha256: digest(verifySource) } }, reason: `Record verifier result for ${taskId}.` });
  lane = canonical.state.lanes.records.find(({ id }) => id === laneId);
  let reviewerLane = canonical.state.lanes.records.find(({ id }) => id === "review-a");
  const reviewAssignmentId = `review-${assignmentId}`;
  const sourceAssignment = { laneId, assignmentId, packetSha256: assignment.packet.sha256, revision: resultRevision };
  const reviewPacketSource = packetSource({ assignmentId: reviewAssignmentId, taskId, attempt: 1,
    revision: canonical.git.headRevision, trackerFingerprint: canonical.tracker.fingerprint,
    laneFingerprint: laneFingerprint(reviewerLane), briefSha256: reviewerLane.brief.sha256 }).replace(/- DEC-[^\n]+/, "None.");
  const reviewPacketPath = `.agent-team/lanes/review-a/packets/${taskId}-1.md`;
  await mkdir(path.dirname(path.join(value.project.root, reviewPacketPath)), { recursive: true });
  await writeFile(path.join(value.project.root, reviewPacketPath), reviewPacketSource);
  canonical = await nativeLaneRequest(value, "lane-next", { schemaVersion: 1, phase: "prepare", operationId: `prepare-${reviewAssignmentId}`,
    laneId: reviewerLane.id, assignmentId: reviewAssignmentId, attempt: 1, expectedTrackerFingerprint: canonical.tracker.fingerprint,
    expectedRunFingerprint: effectiveRunFingerprint(canonical.state.run), expectedRevision: canonical.git.headRevision,
    expectedLaneFingerprint: laneFingerprint(reviewerLane), packet: { path: reviewPacketPath, sha256: digest(reviewPacketSource) }, factSheets: [],
    sourceAssignment, reason: `Prepare independent review for ${taskId}.` });
  reviewerLane = canonical.state.lanes.records.find(({ id }) => id === "review-a");
  canonical = await nativeLaneRequest(value, "lane-next", { schemaVersion: 1, phase: "dispatch", operationId: `dispatch-${reviewAssignmentId}`,
    laneId: reviewerLane.id, assignmentId: reviewAssignmentId, expectedLaneFingerprint: laneFingerprint(reviewerLane),
    expectedRevision: reviewerLane.assignments.at(-1).revision, expectedPacketSha256: reviewerLane.assignments.at(-1).packet.sha256,
    worker: reviewerLane.worker, observation: { status: "observed", source: reviewerLane.worker.host,
      eventId: `host-${reviewAssignmentId}`, observedAt: "2026-09-14T15:01:00.000Z" }, reason: `Record observed reviewer dispatch for ${taskId}.` });
  reviewerLane = canonical.state.lanes.records.find(({ id }) => id === "review-a");
  const reviewer = reviewerLane.worker;
  const reviewPath = `.agent-team/lanes/review-a/evidence/${reviewAssignmentId}-review.json`;
  const reviewSource = json({ schemaVersion: 1, assignmentId: reviewAssignmentId, taskId, revision: resultRevision, reviewer,
    source: { revision: resultRevision, readFirst: true }, diff: { baseRevision: assignment.revision, sourceRevision: resultRevision },
    verifierEvidence: { path: verifyPath, sha256: digest(verifySource) }, rubric: ["Correctness", "Regression safety"],
    challenges: [{ scenario: "Stale task evidence is supplied.", testEvidence: "The exact revision check rejects it." }],
    testsExercised: true, status: "passed", sourceAssignment });
  await writeFile(path.join(value.project.root, reviewPath), reviewSource);
  canonical = await nativeLaneRequest(value, "lane-next", { schemaVersion: 1, phase: "result", operationId: `result-${reviewAssignmentId}`,
    laneId: reviewerLane.id, assignmentId: reviewAssignmentId, expectedLaneFingerprint: laneFingerprint(reviewerLane),
    expectedPacketSha256: reviewerLane.assignments.at(-1).packet.sha256,
    result: { kind: "independent_review", status: "passed", revision: resultRevision, worker: reviewer,
      evidence: { path: reviewPath, sha256: digest(reviewSource) } }, reason: `Record independent review for ${taskId}.` });

  const trackerSource = await readFile(value.project.paths.tasks, "utf8");
  const changedTracker = trackerSource.replace(new RegExp(`(^\\|\\s*${taskId}\\s*\\|[^\\n]*?\\|\\s*)in_progress(\\s*\\|)`, "m"), "$1done$2");
  assert.notEqual(changedTracker, trackerSource);
  const completionPath = path.join(value.project.root, `.agent-team/completion-${taskId}.json`);
  await writeFile(completionPath, json({ status: "passed", revision: resultRevision, taskIds: [taskId], requirementsReconciled: true,
    review: { status: "passed", revision: resultRevision, taskId }, checks: [{ name: "fixture", status: "passed", revision: resultRevision, taskId }] }));
  let gate = await recordGateEvidence(value.project, { actorSessionId: "owner-session", operationId: `completion-${assignmentId}`,
    expectedVersion: canonical.state.stateVersion, expectedFingerprint: canonical.tracker.fingerprint, gate: "completion", taskIds: [taskId],
    expectedRevision: resultRevision, evidencePath: completionPath }, { ...value.options, expectedVersion: canonical.state.stateVersion });
  assert.equal(gate.status, "applied", gate.reason);
  canonical = await loadCanonicalState(value.project);
  const trackerPolicy = await evaluatePolicy({ runtime: "codex", event: "PreToolUse", cwd: value.project.root, sessionId: "owner-session",
    operation: { kind: "file_change", files: [{ action: "edit", path: ".agent-team/TASKS.md",
      previousContent: trackerSource, changedContent: changedTracker }] } }, value.project, { canonical });
  assert.equal(trackerPolicy.allow, true, trackerPolicy.messages.join("\n"));
  await writeFile(value.project.paths.tasks, changedTracker);
  canonical = await loadCanonicalState(value.project);
  const integrationPath = path.join(value.project.root, `.agent-team/integration-${taskId}.json`);
  const actor = { ownerHost: "codex", ownerSessionId: "owner-session", ownershipEpoch: 1 };
  await writeFile(integrationPath, json({ status: "passed", revision: resultRevision, taskIds: [taskId], sourceRevisions: { [taskId]: resultRevision },
    remote: { name: "origin", baseRef: "refs/heads/main", revision: resultRevision, targetRef: "refs/heads/feature", targetRevision: resultRevision },
    authorization: { source: "accepted-lane-packet", scope: "integration", ownerSessionId: "owner-session", revision: resultRevision, taskIds: [taskId] },
    targetAuthorization: { status: "authorized", source: "explicit-release-authorization", target: "refs/heads/feature", revision: resultRevision,
      taskIds: [taskId], ...actor }, recovery: { status: "reconciled", revision: resultRevision, taskIds: [taskId],
      artifactId: `fixture-${taskId}`, action: "rollback" }, preview: { required: false }, remoteMainDeploys: false }));
  gate = await recordGateEvidence(value.project, { actorSessionId: "owner-session", operationId: `integration-${assignmentId}`,
    expectedVersion: canonical.state.stateVersion, expectedFingerprint: canonical.tracker.fingerprint, gate: "integration", taskIds: [taskId],
    expectedRevision: resultRevision, evidencePath: integrationPath }, { ...value.options, expectedVersion: canonical.state.stateVersion });
  assert.equal(gate.status, "applied", gate.reason);
  canonical = await loadCanonicalState(value.project); lane = canonical.state.lanes.records.find(({ id }) => id === laneId);
  assert.ok(canonical.deliveryEvidence[taskId], `canonical delivery evidence missing for ${taskId}`);
  const laneIntegrationPath = `.agent-team/lanes/${laneId}/evidence/${assignmentId}-integration.json`;
  const laneIntegrationSource = json({ assignmentId, taskId, status: "passed", revision: resultRevision,
    canonicalOperationId: `integration-${assignmentId}` });
  await writeFile(path.join(value.project.root, laneIntegrationPath), laneIntegrationSource);
  return nativeLaneRequest(value, "lane-next", { schemaVersion: 1, phase: "result", operationId: `lane-integration-${assignmentId}`,
    laneId, assignmentId, expectedLaneFingerprint: laneFingerprint(lane), expectedPacketSha256: assignment.packet.sha256,
    result: { kind: "integration", status: "passed", revision: resultRevision,
      worker: { host: "codex", sessionId: "owner-session", generation: 1 },
      evidence: { path: laneIntegrationPath, sha256: digest(laneIntegrationSource) } }, reason: `Bind canonical integration for ${taskId}.` });
}

test("two retained work lanes and one reserved reviewer lane process five tasks through native phases normal gates and closure", async () => {
  const value = await fixture();
  temporary.push(`${value.project.root}-lane-build-b`);
  const revision = execFileSync("git", ["rev-parse", "HEAD"], { cwd: value.project.root, encoding: "utf8" }).trim();
  const state = JSON.parse(await readFile(value.project.paths.state, "utf8"));
  state.run.taskIds = ["AT-101", "AT-102", "AT-103", "AT-201", "AT-202"];
  Object.assign(state.release.authorization, { ownerHost: "codex", ownershipEpoch: 1 });
  await writeFile(value.project.paths.state, json(state));
  await writeFile(value.project.paths.tasks, `# Agent-Team Tasks
| ID | Requirement / acceptance | Owner | Depends on | Status | Revision / evidence | Next action |
| --- | --- | --- | --- | --- | --- | --- |
| AT-101 | Lane A first | none | none | ready | none | Claim. |
| AT-102 | Lane A second | none | AT-101 | ready | none | Wait. |
| AT-103 | Lane A third | none | AT-102 | ready | none | Wait. |
| AT-201 | Lane B first | none | none | ready | none | Claim. |
| AT-202 | Lane B second | none | AT-201 | ready | none | Wait. |
`);
  let teams = await readFile(value.project.paths.teams, "utf8");
  teams = teams.replace("AT-101, AT-102 | ready |", "AT-101, AT-102, AT-103 | ready |");
  await writeFile(value.project.paths.teams, teams);
  let canonical = await loadCanonicalState(value.project);
  const laneA = { ...value.request.lane, queue: ["AT-101", "AT-102", "AT-103"] };
  await nativeLaneRequest(value, "lane-create", { ...value.request, operationId: "fixture-create-a", lane: laneA,
    expectedTrackerFingerprint: canonical.tracker.fingerprint, expectedRunFingerprint: effectiveRunFingerprint(canonical.state.run),
    expectedLanesFingerprint: laneCollectionFingerprint() });

  const worktreeB = `${value.project.root}-lane-build-b`;
  execFileSync("git", ["worktree", "add", "-q", "-b", "lane/build-b", worktreeB, "main"], { cwd: value.project.root });
  const readmeHash = digest(await readFile(path.join(value.project.root, "README.md")));
  const skillPath = fixtureSkillPath;
  const skillHash = digest(await readFile(path.join(value.project.root, skillPath)));
  const briefB = `# Build lane B

## Role / model / effort
developer / gpt-6-astra / high

## Queue
AT-201, AT-202

## Shared rules
Preserve tracker authority and exact revision evidence.

## Writable paths
hooks/lib/lane-fixture-b/**

## Required skills
${skillPath} ${skillHash}

## Applicable instructions
README.md ${readmeHash}

## Context revision
${revision}

## Evidence destination
.agent-team/lanes/build-b/evidence/

## Handoff format
State revision, checks, remaining risk, and one next action.
`;
  const laneRootB = path.join(value.project.root, ".agent-team/lanes/build-b");
  await mkdir(path.join(laneRootB, "evidence"), { recursive: true });
  await writeFile(path.join(laneRootB, "BRIEF.md"), briefB);
  const ownedB = ["hooks/lib/lane-fixture-b/**"];
  const pathSetHashB = digest(JSON.stringify(ownedB));
  const ownershipB = json({ schemaVersion: 1, status: "resolved", source: "graphify", revision, ownedPaths: ownedB,
    pathSetHash: pathSetHashB, conflicts: [], observedAt: "2026-09-14T13:00:00.000Z" });
  await writeFile(path.join(laneRootB, "evidence/ownership.json"), ownershipB);
  teams = await readFile(value.project.paths.teams, "utf8");
  await writeFile(value.project.paths.teams, `${teams.trimEnd()}\n| TEAM-LANE-B | lane-build-b | lane-worker-b | ${worktreeB} | lane/build-b | ${ownedB[0]} | AT-201, AT-202 | ready |\n`);
  canonical = await loadCanonicalState(value.project);
  const laneB = { id: "build-b", teamId: "TEAM-LANE-B", role: "developer", model: "gpt-6-astra", effort: "high",
    queue: ["AT-201", "AT-202"], worker: { host: "codex", sessionId: "lane-worker-b", generation: 1 }, worktree: worktreeB,
    branch: "lane/build-b", brief: { path: ".agent-team/lanes/build-b/BRIEF.md", sha256: digest(briefB) },
    ownershipEvidence: { path: ".agent-team/lanes/build-b/evidence/ownership.json", sha256: digest(ownershipB), revision, pathSetHash: pathSetHashB },
    factSheets: [] };
  await nativeLaneRequest(value, "lane-create", { schemaVersion: 1, operationId: "fixture-create-b",
    expectedTrackerFingerprint: canonical.tracker.fingerprint, expectedRunFingerprint: effectiveRunFingerprint(canonical.state.run), expectedRevision: revision,
    expectedLanesFingerprint: laneCollectionFingerprint(canonical.state.lanes), lane: laneB, reason: "Create second fixture lane." });

  await registerReviewerLane(value, ["AT-101", "AT-102", "AT-103", "AT-201", "AT-202"]);

  await runLaneTask(value, "build-a", "AT-101", 1);
  await runLaneTask(value, "build-a", "AT-102", 1);
  canonical = await loadCanonicalState(value.project);
  let currentA = canonical.state.lanes.records.find(({ id }) => id === "build-a");
  assert.equal(currentA.status, "rotation_required");
  const handoverSource = `# Lane handover

## Voice
Continue the exact retained lane protocol.

## Gotchas
Do not reclaim the current task.

## Open threads
Start AT-103 after replacement binding.

## Decisions
Existing packet decisions remain in force.

## Exact revision
${revision}
`;
  const handover = { path: ".agent-team/lanes/build-a/HANDOVER-001.md", sha256: digest(handoverSource), revision, sequence: 1 };
  await writeFile(path.join(value.project.root, handover.path), handoverSource);
  const outgoing = currentA.worker;
  await nativeLaneRequest(value, "lane-rotate", { schemaVersion: 1, operationId: "fixture-rotate-a", laneId: "build-a",
    expectedLaneFingerprint: laneFingerprint(currentA), expectedRevision: revision, trigger: "threshold", outgoingWorker: outgoing,
    replacementWorker: null, handover, transferEvidence: { kind: "stopped_writer", observationId: "fixture-stop-a" },
    reason: "Rotate at the snapshotted two-task threshold." }, { inspectLaneSession: async () => ({ status: "stopped", ...outgoing,
      observationId: "fixture-stop-a", source: "simulated_native_host", observedAt: "2026-09-14T16:00:00.000Z" }) });
  teams = await readFile(value.project.paths.teams, "utf8");
  await writeFile(value.project.paths.teams, teams.replace("| lane-worker |", "| lane-worker-a2 |"));
  canonical = await loadCanonicalState(value.project); currentA = canonical.state.lanes.records.find(({ id }) => id === "build-a");
  await nativeLaneRequest(value, "lane-next", { schemaVersion: 1, phase: "bind", operationId: "fixture-bind-a2", laneId: "build-a",
    expectedLaneFingerprint: laneFingerprint(currentA), expectedRevision: revision, expectedHandoverSha256: handover.sha256,
    replacementWorker: { host: "codex", sessionId: "lane-worker-a2", generation: 2 }, reason: "Bind replacement without reclaim." });
  await runLaneTask(value, "build-a", "AT-103", 1);
  await runLaneTask(value, "build-b", "AT-201", 1);
  await runLaneTask(value, "build-b", "AT-202", 1);

  for (const laneId of ["build-a", "build-b", "review-a"]) {
    canonical = await loadCanonicalState(value.project);
    const lane = canonical.state.lanes.records.find(({ id }) => id === laneId);
    const reason = `Release the final ${laneId} worker and close retained history.`;
    const release = { schemaVersion: 1, kind: "explicit_release", laneId, worker: lane.worker, revision,
      authorizedBy: { host: "codex", sessionId: "owner-session", ownershipEpoch: 1 }, reason,
      observedAt: "2026-09-14T17:00:00.000Z" };
    const releaseSource = json(release);
    const releasePath = `.agent-team/lanes/${laneId}/evidence/final-worker-release.json`;
    await writeFile(path.join(value.project.root, releasePath), releaseSource);
    await nativeLaneRequest(value, "lane-close", { schemaVersion: 1, operationId: `fixture-close-${laneId}`, laneId,
      expectedLaneFingerprint: laneFingerprint(lane), expectedTrackerFingerprint: canonical.tracker.fingerprint,
      expectedRunFingerprint: effectiveRunFingerprint(canonical.state.run), expectedRevision: revision,
      writerRelease: { kind: "explicit_release", path: releasePath, sha256: digest(releaseSource) }, reason });
  }
  canonical = await loadCanonicalState(value.project);
  assert.deepEqual(canonical.state.lanes.records.map(({ id, status, rotationCount, assignments }) => ({ id, status, rotationCount,
    tasks: assignments.map(({ taskId }) => taskId) })), [
    { id: "build-a", status: "closed", rotationCount: 1, tasks: ["AT-101", "AT-102", "AT-103"] },
    { id: "build-b", status: "closed", rotationCount: 0, tasks: ["AT-201", "AT-202"] },
    { id: "review-a", status: "closed", rotationCount: 0, tasks: ["AT-101", "AT-102", "AT-103", "AT-201", "AT-202"] },
  ]);
});

test("lane-next replays one interrupted tracker claim with its persisted assignment intent", async () => {
  const value = await fixture();
  assert.equal((await createLane(value.project, value.request, value.options)).status, "applied");
  const next = await nextRequest(value, "lane-next-interrupted");
  let renames = 0;
  const interrupted = await advanceLane(value.project, next.request, { ...next.options, filesystem: {
    async rename(source, destination) {
      renames += 1;
      if (renames === 3) throw new Error("injected_lane_receipt_failure");
      return rename(source, destination);
    },
  } });
  assert.equal(interrupted.status, "unavailable");
  const pending = await loadCanonicalState(value.project);
  assert.equal(pending.tasks.find(({ id }) => id === "AT-101").owner, "TEAM-LANE-A");
  assert.equal(pending.state.lanes.records[0].assignments.length, 1);
  assert.equal(pending.state.pendingOperations[next.request.operationId].phase, "uncertain");

  const replayed = await advanceLane(value.project, next.request, { ...next.options, expectedVersion: pending.state.stateVersion });
  assert.equal(replayed.status, "applied");
  assert.equal(replayed.result.reconciled, true);
  const after = await loadCanonicalState(value.project);
  assert.equal(after.state.lanes.records[0].assignments.length, 1);
  assert.equal(after.state.pendingOperations[next.request.operationId], undefined);
});

test("lane-next retries an interrupted preimage claim once when the tracker proves the write did not occur", async () => {
  const value = await fixture();
  assert.equal((await createLane(value.project, value.request, value.options)).status, "applied");
  const next = await nextRequest(value, "lane-next-preimage-interrupted");
  const trackerBefore = await readFile(value.project.paths.tasks);
  let renames = 0;
  const interrupted = await advanceLane(value.project, next.request, { ...next.options, filesystem: {
    async rename(source, destination) {
      renames += 1;
      if (renames === 2) throw new Error("injected_tracker_preimage_failure");
      return rename(source, destination);
    },
  } });
  assert.equal(interrupted.status, "unavailable");
  assert.deepEqual(await readFile(value.project.paths.tasks), trackerBefore);
  const pending = await loadCanonicalState(value.project);
  assert.equal(pending.state.pendingOperations[next.request.operationId].phase, "uncertain");
  assert.equal(pending.state.lanes.records[0].assignments.length, 1);

  const replayed = await advanceLane(value.project, next.request, { ...next.options, expectedVersion: pending.state.stateVersion });
  assert.equal(replayed.status, "applied", replayed.reason);
  const after = await loadCanonicalState(value.project);
  assert.equal(after.tasks.find(({ id }) => id === "AT-101").owner, "TEAM-LANE-A");
  assert.equal(after.state.lanes.records[0].assignments.length, 1);
  assert.equal(after.state.pendingOperations[next.request.operationId], undefined);
});

test("all lane commands parse exactly, bare CLI refuses identity, and native hook applies lane-create", async () => {
  const value = await fixture();
  const cli = path.resolve(import.meta.dirname, "../hooks/agent-team-cli.mjs");
  for (const commandName of ["lane-create", "lane-next", "lane-rotate", "lane-close"]) {
    const requestPath = path.join(value.project.root, `${commandName}.json`);
    const command = `node ${cli} ${commandName} --project ${value.project.root} --request ${requestPath}`;
    assert.deepEqual(classifyOperation({ operation: { kind: "shell", command } }), {
      kind: "agent_team_native_command", command: commandName, cliPath: cli, project: value.project.root, request: requestPath, valid: true,
    });
    assert.notEqual(classifyOperation({ operation: { kind: "shell", command: `${command} ; true` } }).kind, "agent_team_native_command");
    assert.deepEqual(await runCommand(commandName, { project: value.project.root, request: requestPath }, {
      nativeIdentity: value.options.nativeIdentity,
    }), { status: "conflict", reason: "native_hook_identity_required" });
  }

  const requestPath = path.join(value.project.root, "lane-create.json");
  await writeFile(requestPath, json({ schemaVersion: 1, actorSessionId: "owner-session", expectedVersion: 0, request: value.request }));
  const shell = `node ${cli} lane-create --project ${value.project.root} --request ${requestPath}`;
  const hooked = await runNormalizedHook({ runtime: "codex", event: "PreToolUse", cwd: value.project.root,
    sessionId: "owner-session", eventId: "lane-create-native-event", operation: { kind: "shell", command: shell } });
  assert.equal(hooked.decision.allow, true);
  assert.deepEqual(hooked.decision.mutations.find(({ kind }) => kind === "lane_create"), {
    kind: "lane_create", command: "lane-create", status: "applied",
  });
  assert.equal((await loadCanonicalState(value.project)).state.lanes.records.length, 1);
  const next = await nextRequest(value, "lane-next-native");
  const nextPath = path.join(value.project.root, "lane-next.json");
  await writeFile(nextPath, json({ schemaVersion: 1, actorSessionId: "owner-session", expectedVersion: next.options.expectedVersion, request: next.request }));
  const nextHooked = await runNormalizedHook({ runtime: "codex", event: "PreToolUse", cwd: value.project.root,
    sessionId: "owner-session", eventId: "lane-next-native-event",
    operation: { kind: "shell", command: `node ${cli} lane-next --project ${value.project.root} --request ${nextPath}` } });
  assert.deepEqual(nextHooked.decision.mutations.find(({ kind }) => kind === "lane_next"), {
    kind: "lane_next", command: "lane-next", status: "applied",
  });
  assert.equal((await loadCanonicalState(value.project)).state.lanes.records[0].assignments.length, 1);
});
