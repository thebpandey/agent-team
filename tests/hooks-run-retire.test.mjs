import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import { execFileSync } from "node:child_process";
import { mkdir, mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import test from "node:test";

import { runNormalizedHook } from "../hooks/agent-team-hook.mjs";
import { runCommand } from "../hooks/agent-team-cli.mjs";
import { normalizeEvent } from "../hooks/lib/event.mjs";
import { classifyOperation } from "../hooks/lib/operation.mjs";
import { loadCanonicalState } from "../hooks/lib/canonical-state.mjs";
import { releaseAuthorityReady } from "../hooks/lib/policy.mjs";
import { evaluatePolicy } from "../hooks/lib/policy.mjs";
import { resolveProject } from "../hooks/lib/project.mjs";
import * as runState from "../hooks/lib/run-state.mjs";
import { recordGateEvidence } from "../hooks/lib/task-transitions.mjs";
import { policyFixture } from "./hook-test-helpers.mjs";

const cli = path.resolve(import.meta.dirname, "../hooks/agent-team-cli.mjs");
const temporary = [];
test.afterEach(async () => Promise.all(temporary.splice(0).map((target) => rm(target, { recursive: true, force: true }))));

const stable = (value) => JSON.stringify(value && typeof value === "object"
  ? Array.isArray(value) ? value.map((entry) => JSON.parse(stable(entry)))
    : Object.fromEntries(Object.keys(value).sort().map((key) => [key, JSON.parse(stable(value[key]))])) : value);
const fingerprint = (value) => createHash("sha256").update(stable(value)).digest("hex");
const settingSources = () => Object.fromEntries(["mode", "taskIds", "teamLimit", "autoDeploy", "batchSize"].map((key) => [key, "explicit_run"]));

function historicalRun() {
  return {
    id: "historical-run", ownerSessionId: "historical-owner", ownerHost: "codex", ownershipEpoch: 1,
    mode: "finite", taskIds: ["AT-001"], teamLimit: 1, autoDeploy: false, batchSize: 1,
    source: "explicit_run", settingSources: settingSources(), paused: false, operationalVersion: 0,
    blockers: [], pendingDeliveryIds: ["AT-001"], deployedTaskIds: [], terminalClassification: "unknown",
  };
}

async function fixture() {
  const root = await mkdtemp(path.join(os.tmpdir(), "agent-team-run-retire-"));
  temporary.push(root, `${root}-feature`, `${root}-remote`);
  const value = await policyFixture(root, { qualifiedOwnership: true });
  const project = await resolveProject(root);
  const current = await loadCanonicalState(project);
  const state = {
    ...current.state,
    stateVersion: current.state.stateVersion ?? 0,
    run: historicalRun(),
    deliveryReceipts: { completion: { "AT-001": { status: "passed", evidence: "retained" } } },
    retainedEvidence: { source: "historical", revision: value.revision },
  };
  await writeFile(project.paths.state, `${JSON.stringify(state, null, 2)}\n`);
  return {
    ...value,
    project,
    nativeIdentity: { host: "codex", sessionId: "owner-session", observed: true, cwd: root, ownershipEpoch: 1 },
  };
}

async function retirementRequest(value, overrides = {}) {
  const canonical = await loadCanonicalState(value.project);
  return {
    operationId: "retire-historical-run",
    expectedStateFingerprint: fingerprint(canonical.state),
    expectedRunFingerprint: fingerprint(canonical.state.run),
    expectedTrackerFingerprint: canonical.tracker.fingerprint,
    expectedRevision: canonical.git.headRevision,
    reason: "The user explicitly requested a clean release-maintenance successor run.",
    authorization: { source: "explicit_user_instruction", scope: "release_maintenance" },
    successorIntent: { kind: "maintenance_release", runId: "release-7.3.0", taskId: "AT-REL", target: "github:example/agent-team:v7.3.0" },
    ...overrides,
  };
}

function nativeOptions(value, expectedVersion) {
  return { actorSessionId: "owner-session", expectedVersion, nativeIdentity: value.nativeIdentity,
    now: () => "2026-09-14T20:30:00.000Z" };
}

async function startRetiredSuccessor(value) {
  const before = await loadCanonicalState(value.project);
  const request = await retirementRequest(value);
  assert.equal((await runState.retireRun(value.project, request, nativeOptions(value, before.state.stateVersion))).status, "applied");
  await writeFile(value.project.paths.tasks, `${await readFile(value.project.paths.tasks, "utf8")}| AT-REL | Publish 7.3.0 | none | none | ready | none | Run release maintenance. |\n`);
  const current = await loadCanonicalState(value.project);
  const start = { operationId: "start-release-maintenance", expectedTrackerFingerprint: current.tracker.fingerprint,
    reason: "Start the exact release-maintenance successor.", run: { id: "release-7.3.0", mode: "finite", taskIds: ["AT-REL"],
      teamLimit: 1, autoDeploy: false, batchSize: 1, source: "explicit_run", settingSources: settingSources() } };
  assert.equal((await runState.startRun(value.project, start, nativeOptions(value, current.state.stateVersion))).status, "applied");
  return loadCanonicalState(value.project);
}

function integrationEvidence(value) {
  return {
    status: "passed", revision: value.revision, taskIds: ["AT-REL"],
    remote: { name: "origin", baseRef: "refs/heads/main", revision: value.revision,
      targetRef: "refs/heads/main", targetRevision: value.revision },
    authorization: { source: "accepted-release-packet", scope: "integration", ownerSessionId: "owner-session",
      revision: value.revision, taskIds: ["AT-REL"] },
    recovery: { status: "reconciled", revision: value.revision, taskIds: ["AT-REL"], artifactId: "git:known-good", action: "restore tag" },
    sourceRevisions: { "AT-REL": value.revision },
    targetAuthorization: { status: "authorized", source: "explicit release target", target: "refs/heads/main", revision: value.revision,
      taskIds: ["AT-REL"], ownerHost: "codex", ownerSessionId: "owner-session", ownershipEpoch: 1 },
    preview: { required: false }, remoteMainDeploys: false,
  };
}

async function recordSuccessorGates(value) {
  let current = await loadCanonicalState(value.project);
  const completionPath = path.join(value.root, ".agent-team", "successor-completion.json");
  await writeFile(completionPath, JSON.stringify({ status: "passed", revision: value.revision, taskIds: ["AT-REL"], requirementsReconciled: true,
    review: { status: "passed", revision: value.revision, taskId: "AT-REL" },
    checks: [{ name: "focused", status: "passed", revision: value.revision, taskId: "AT-REL" }] }));
  assert.equal((await recordGateEvidence(value.project, { actorSessionId: "owner-session", operationId: "successor-completion",
    expectedVersion: current.state.stateVersion, expectedFingerprint: current.tracker.fingerprint, gate: "completion",
    taskIds: ["AT-REL"], expectedRevision: value.revision, evidencePath: completionPath }, { nativeIdentity: value.nativeIdentity })).status, "applied");
  await writeFile(value.project.paths.tasks, (await readFile(value.project.paths.tasks, "utf8"))
    .replace("| AT-REL | Publish 7.3.0 | none | none | ready |", "| AT-REL | Publish 7.3.0 | none | none | verified |"));
  current = await loadCanonicalState(value.project);
  const integrationPath = path.join(value.root, ".agent-team", "successor-integration.json");
  await writeFile(integrationPath, JSON.stringify(integrationEvidence(value)));
  return recordGateEvidence(value.project, { actorSessionId: "owner-session", operationId: "successor-integration",
    expectedVersion: current.state.stateVersion, expectedFingerprint: current.tracker.fingerprint, gate: "integration",
    taskIds: ["AT-REL"], expectedRevision: value.revision, evidencePath: integrationPath }, { nativeIdentity: value.nativeIdentity });
}

async function recordSuccessorRelease(value) {
  const current = await loadCanonicalState(value.project);
  const integration = current.state.integration;
  const evidencePath = path.join(value.root, ".agent-team", "successor-release.json");
  const artifactSha256 = "a".repeat(64);
  const evidence = {
    status: "passed", revision: value.revision, taskIds: ["AT-REL"], selectedTaskIds: ["AT-REL"],
    ownerSessionId: "owner-session", authorized: true, expectedRevision: value.revision,
    target: "github:example/agent-team:v7.3.0", process: "gh-release",
    authorization: { source: "explicit user authorization", target: "github:example/agent-team:v7.3.0", process: "gh-release",
      scope: "release-successor", ownerSessionId: "owner-session", grantedAt: "2026-09-14T20:30:00.000Z" },
    run: { id: "release-7.3.0", mode: "manual", taskIds: ["AT-REL"], paused: false },
    runMode: "manual", batchId: "release-successor", batch: { id: "release-successor", taskIds: ["AT-REL"] },
    artifact: { id: `agent-team-7.3.0:${artifactSha256}`, revision: value.revision, taskIds: ["AT-REL"], sha256: artifactSha256 },
    integration: { status: "passed", revision: value.revision, taskIds: ["AT-REL"], recordedTaskIds: ["AT-REL"],
      evidencePath: integration.recordedEvidence.path, remoteName: integration.remoteName, baseRef: integration.baseRemoteRef,
      baseRevision: integration.baseRevision, targetRef: integration.remoteRef, targetRevision: value.revision, remoteMainDeploys: false },
    verification: { status: "passed", revision: value.revision, taskIds: ["AT-REL"] },
    preview: { required: false, status: "not_required", revision: value.revision },
    delta: { status: "clean", revision: value.revision, taskIds: ["AT-REL"] },
    recovery: { status: "verified", artifactId: "git:known-good", action: "restore tag" },
    autoDeploy: false, projectPaused: false, hold: false,
  };
  await writeFile(evidencePath, JSON.stringify(evidence));
  return recordGateEvidence(value.project, { actorSessionId: "owner-session", operationId: "successor-release",
    expectedVersion: current.state.stateVersion, expectedFingerprint: current.tracker.fingerprint, gate: "release",
    taskIds: ["AT-REL"], expectedRevision: value.revision, evidencePath }, { nativeIdentity: value.nativeIdentity });
}

test("retirement archives the historical run and holds authority without changing tracker, claims, or evidence", async () => {
  assert.equal(typeof runState.retireRun, "function", "run-state must expose the authenticated retirement transition");
  const value = await fixture();
  const before = await loadCanonicalState(value.project);
  const stateBytes = await readFile(value.project.paths.state);
  const trackerBytes = await readFile(value.project.paths.tasks);
  const teamsBytes = await readFile(value.project.paths.teams);
  const request = await retirementRequest(value);
  const result = await runState.retireRun(value.project, request, nativeOptions(value, before.state.stateVersion));
  assert.equal(result.status, "applied");
  const after = await loadCanonicalState(value.project);
  assert.equal(Object.hasOwn(after.state, "run"), false);
  assert.deepEqual(after.state.runRetirements[0].run, before.state.run);
  assert.deepEqual(after.state.runRetirements[0].priorAuthority,
    { integration: before.state.integration, release: before.state.release });
  assert.deepEqual(after.state.deliveryReceipts, before.state.deliveryReceipts);
  assert.deepEqual(after.state.retainedEvidence, before.state.retainedEvidence);
  assert.equal(after.state.integration.authorized, false);
  assert.equal(after.state.integration.hold, true);
  assert.equal(after.state.release.authorized, false);
  assert.equal(after.state.release.hold, true);
  assert.equal(releaseAuthorityReady(after, { process: "npm" }), false);
  assert.deepEqual(after.state.runStartConstraint, {
    schemaVersion: 1, status: "pending", retirementOperationId: request.operationId,
    runId: "release-7.3.0", taskId: "AT-REL", target: "github:example/agent-team:v7.3.0",
  });
  assert.deepEqual(await readFile(value.project.paths.tasks), trackerBytes);
  assert.deepEqual(await readFile(value.project.paths.teams), teamsBytes);
  assert.notDeepEqual(await readFile(value.project.paths.state), stateBytes);
  const duplicate = await runState.retireRun(value.project, request, nativeOptions(value, before.state.stateVersion));
  assert.equal(duplicate.status, "duplicate");
  assert.deepEqual(duplicate.result, result.result);
});

test("retirement refuses missing native identity and every stale or pending canonical input without writes", async () => {
  assert.equal(typeof runState.retireRun, "function", "run-state must expose the authenticated retirement transition");
  const cases = [
    ["native", (request, options) => [request, { ...options, nativeIdentity: undefined }], "native_project_context_required"],
    ["version", (request, options) => [request, { ...options, expectedVersion: options.expectedVersion + 1 }], "stale_version"],
    ["state", (request, options) => [{ ...request, expectedStateFingerprint: "a".repeat(64) }, options], "stale_state"],
    ["run", (request, options) => [{ ...request, expectedRunFingerprint: "b".repeat(64) }, options], "stale_run"],
    ["tracker", (request, options) => [{ ...request, expectedTrackerFingerprint: "c".repeat(64) }, options], "stale_tracker"],
    ["revision", (request, options) => [{ ...request, expectedRevision: "d".repeat(40) }, options], "stale_revision"],
  ];
  for (const [label, alter, reason] of cases) {
    const value = await fixture();
    const canonical = await loadCanonicalState(value.project);
    const request = await retirementRequest(value);
    const before = await readFile(value.project.paths.state);
    const [changedRequest, changedOptions] = alter(request, nativeOptions(value, canonical.state.stateVersion));
    const result = await runState.retireRun(value.project, changedRequest, changedOptions);
    assert.equal(result.reason, reason, label);
    assert.deepEqual(await readFile(value.project.paths.state), before, label);
  }

  const pending = await fixture();
  const pendingState = JSON.parse(await readFile(pending.project.paths.state, "utf8"));
  pendingState.pendingOperations = { external: { phase: "uncertain", kind: "publication", target: "origin" } };
  await writeFile(pending.project.paths.state, `${JSON.stringify(pendingState, null, 2)}\n`);
  const request = await retirementRequest(pending);
  const before = await readFile(pending.project.paths.state);
  const result = await runState.retireRun(pending.project, request, nativeOptions(pending, pendingState.stateVersion));
  assert.deepEqual(result, { status: "unavailable", reason: "pending_operation_unresolved" });
  assert.deepEqual(await readFile(pending.project.paths.state), before);

  const currentOwner = await fixture();
  const currentOwnerState = JSON.parse(await readFile(currentOwner.project.paths.state, "utf8"));
  Object.assign(currentOwnerState.run, { ownerSessionId: "owner-session", ownerHost: "codex", ownershipEpoch: 1 });
  await writeFile(currentOwner.project.paths.state, `${JSON.stringify(currentOwnerState, null, 2)}\n`);
  const currentOwnerRequest = await retirementRequest(currentOwner);
  const currentOwnerBefore = await readFile(currentOwner.project.paths.state);
  assert.equal((await runState.retireRun(currentOwner.project, currentOwnerRequest,
    nativeOptions(currentOwner, currentOwnerState.stateVersion))).reason, "run_not_historical");
  assert.deepEqual(await readFile(currentOwner.project.paths.state), currentOwnerBefore);
});

test("only the exact pending maintenance successor can start and it still requires native project identity", async () => {
  assert.equal(typeof runState.retireRun, "function", "run-state must expose the authenticated retirement transition");
  const value = await fixture();
  const before = await loadCanonicalState(value.project);
  const request = await retirementRequest(value);
  const retired = await runState.retireRun(value.project, request, nativeOptions(value, before.state.stateVersion));
  assert.equal(retired.status, "applied");

  await writeFile(value.project.paths.tasks, `${await readFile(value.project.paths.tasks, "utf8")}| AT-REL | Publish 7.3.0 | none | none | ready | none | Run release maintenance. |\n`);
  const current = await loadCanonicalState(value.project);
  const proposed = { id: "release-7.3.0", mode: "finite", taskIds: ["AT-REL"], teamLimit: 1, autoDeploy: false,
    batchSize: 1, source: "explicit_run", settingSources: settingSources() };
  const start = { operationId: "start-release-maintenance", expectedTrackerFingerprint: current.tracker.fingerprint,
    reason: "Start only the explicitly authorized release-maintenance task.", run: proposed };
  const options = nativeOptions(value, current.state.stateVersion);
  const beforeStart = await readFile(value.project.paths.state);
  assert.equal((await runState.startRun(value.project, start, { ...options, nativeIdentity: undefined })).reason, "native_project_context_required");
  assert.deepEqual(await readFile(value.project.paths.state), beforeStart);
  assert.equal((await runState.startRun(value.project, { ...start, operationId: "wrong-successor", run: { ...proposed, taskIds: ["AT-001"] } }, options)).reason,
    "run_successor_mismatch");
  assert.deepEqual(await readFile(value.project.paths.state), beforeStart);
  const started = await runState.startRun(value.project, start, options);
  assert.equal(started.status, "applied");
  const after = await loadCanonicalState(value.project);
  assert.equal(after.state.run.id, "release-7.3.0");
  assert.deepEqual(after.state.run.taskIds, ["AT-REL"]);
  assert.equal(after.state.runStartConstraint.status, "started");
  assert.equal(after.state.runStartConstraint.startOperationId, "start-release-maintenance");
  assert.equal(after.state.integration.authorized, false);
  assert.equal(after.state.release.authorized, false);
  assert.deepEqual(after.state.runRetirements[0].run, historicalRun());
});

test("run-retire is an exact non-chained native hook route and the public CLI cannot mint identity", async () => {
  assert.equal(typeof runState.retireRun, "function", "run-state must expose the authenticated retirement transition");
  const value = await fixture();
  const canonical = await loadCanonicalState(value.project);
  const body = await retirementRequest(value);
  const requestPath = path.join(value.root, ".agent-team", "requests", "retire.json");
  await mkdir(path.dirname(requestPath), { recursive: true });
  await writeFile(requestPath, `${JSON.stringify({ schemaVersion: 1, actorSessionId: "owner-session",
    expectedVersion: canonical.state.stateVersion, request: body }, null, 2)}\n`);
  const invocation = `node ${cli} run-retire --project ${value.root} --request ${requestPath}`;
  assert.deepEqual(classifyOperation({ operation: { kind: "shell", command: invocation } }), {
    kind: "agent_team_native_command", command: "run-retire", cliPath: cli,
    project: value.root, request: requestPath, valid: true,
  });
  assert.notEqual(classifyOperation({ operation: { kind: "shell", command: `${invocation} ; true` } }).kind, "agent_team_native_command");
  assert.deepEqual(await runCommand("run-retire", { project: value.root, request: requestPath }, { nativeIdentity: value.nativeIdentity }),
    { status: "conflict", reason: "native_hook_identity_required" });
  const result = await runNormalizedHook(normalizeEvent("codex", "PreToolUse", { cwd: value.root,
    session_id: "owner-session", event_id: "native-run-retire", tool_name: "exec_command", tool_input: { cmd: invocation } }));
  assert.equal(result.decision.allow, true, JSON.stringify(result.decision));
  assert.equal(result.decision.mutations.some((entry) => entry.kind === "run_retirement" && entry.command === "run-retire"
    && entry.status === "applied"), true, JSON.stringify(result.decision));
  assert.equal(Object.hasOwn((await loadCanonicalState(value.project)).state, "run"), false);

  await writeFile(value.project.paths.tasks, `${await readFile(value.project.paths.tasks, "utf8")}| AT-REL | Publish 7.3.0 | none | none | ready | none | Run release maintenance. |\n`);
  const retired = await loadCanonicalState(value.project);
  const startPath = path.join(value.root, ".agent-team", "requests", "start-successor.json");
  const startBody = { operationId: "native-start-successor", expectedTrackerFingerprint: retired.tracker.fingerprint,
    reason: "Start the exact release-maintenance successor.", run: { id: "release-7.3.0", mode: "finite", taskIds: ["AT-REL"],
      teamLimit: 1, autoDeploy: false, batchSize: 1, source: "explicit_run", settingSources: settingSources() } };
  await writeFile(startPath, `${JSON.stringify({ schemaVersion: 1, actorSessionId: "owner-session",
    expectedVersion: retired.state.stateVersion, request: startBody }, null, 2)}\n`);
  const startInvocation = `node ${cli} run-start --project ${value.root} --request ${startPath}`;
  assert.equal(classifyOperation({ operation: { kind: "shell", command: startInvocation } }).command, "run-start");
  assert.deepEqual(await runCommand("run-start", { project: value.root, request: startPath }, { nativeIdentity: value.nativeIdentity }),
    { status: "conflict", reason: "native_hook_identity_required" });
  const started = await runNormalizedHook(normalizeEvent("codex", "PreToolUse", { cwd: value.root,
    session_id: "owner-session", event_id: "native-run-start", tool_name: "exec_command", tool_input: { cmd: startInvocation } }));
  assert.equal(started.decision.allow, true, JSON.stringify(started.decision));
  assert.equal(started.decision.mutations.some((entry) => entry.kind === "run_start" && entry.command === "run-start"
    && entry.status === "applied"), true, JSON.stringify(started.decision));
  assert.equal((await loadCanonicalState(value.project)).state.run.id, "release-7.3.0");
});

test("fresh successor evidence clears only its retirement integration hold and policy permits the exact integration", async () => {
  const value = await fixture();
  const started = await startRetiredSuccessor(value);
  assert.equal(started.state.integration.hold, true);
  assert.equal(started.state.integration.runRetirementHold.status, "pending_fresh_evidence");
  assert.equal((await recordSuccessorGates(value)).status, "applied");
  const current = await loadCanonicalState(value.project);
  assert.equal(current.state.integration.hold, false);
  assert.equal(current.state.integration.runRetirementHold.status, "cleared");
  assert.equal(current.state.integration.runRetirementHold.clearedByOperationId, "successor-integration");
  assert.equal(current.state.release.hold, true);
  assert.equal(current.state.release.runRetirementHold.status, "pending_fresh_evidence");
  assert.deepEqual(current.state.runRetirements[0].run, historicalRun());
  const decision = await evaluatePolicy({ runtime: "codex", event: "PreToolUse", cwd: value.root, sessionId: "owner-session", eventId: "successor-push",
    operation: { kind: "shell", command: `git -C ${value.root} push origin HEAD:main` } }, value.project, { now: new Date() });
  assert.equal(decision.allow, true, JSON.stringify(decision));
  assert.equal((await recordSuccessorRelease(value)).status, "applied");
  const released = await loadCanonicalState(value.project);
  assert.equal(released.state.release.hold, false);
  assert.equal(released.state.release.runRetirementHold.status, "cleared");
  assert.equal(released.state.release.runRetirementHold.clearedByOperationId, "successor-release");
  assert.deepEqual(released.state.runRetirements[0], current.state.runRetirements[0]);
});

test("fresh evidence cannot clear an unrelated or stale retirement hold", async () => {
  const value = await fixture();
  await startRetiredSuccessor(value);
  const state = JSON.parse(await readFile(value.project.paths.state, "utf8"));
  state.integration.runRetirementHold = { ...state.integration.runRetirementHold, retirementOperationId: "older-retirement" };
  state.release.runRetirementHold = { ...state.release.runRetirementHold, retirementOperationId: "older-retirement" };
  await writeFile(value.project.paths.state, `${JSON.stringify(state, null, 2)}\n`);
  assert.equal((await recordSuccessorGates(value)).status, "applied");
  const current = await loadCanonicalState(value.project);
  assert.equal(current.state.integration.authorized, true);
  assert.equal(current.state.integration.hold, true);
  assert.equal(current.state.integration.runRetirementHold.retirementOperationId, "older-retirement");
  const decision = await evaluatePolicy({ runtime: "codex", event: "PreToolUse", cwd: value.root, sessionId: "owner-session", eventId: "stale-hold-push",
    operation: { kind: "shell", command: `git -C ${value.root} push origin HEAD:main` } }, value.project, { now: new Date() });
  assert.equal(decision.allow, false);
  assert.equal((await recordSuccessorRelease(value)).status, "applied");
  const released = await loadCanonicalState(value.project);
  assert.equal(released.state.release.authorized, true);
  assert.equal(released.state.release.hold, true);
  assert.equal(released.state.release.runRetirementHold.retirementOperationId, "older-retirement");
  const releaseDecision = await evaluatePolicy({ runtime: "codex", event: "PreToolUse", cwd: value.root, sessionId: "owner-session",
    eventId: "stale-hold-release", operation: { kind: "shell", command: "gh release create v7.3.0" } }, value.project, { now: new Date() });
  assert.equal(releaseDecision.allow, false);
});
