import assert from "node:assert/strict";
import { execFile, execFileSync, spawn } from "node:child_process";
import { access, mkdir, mkdtemp, readFile, rm, symlink, writeFile } from "node:fs/promises";
import http from "node:http";
import os from "node:os";
import path from "node:path";
import test from "node:test";
import { promisify } from "node:util";

import { loadCanonicalState } from "../hooks/lib/canonical-state.mjs";
import { initializeProject, initializationRecordProblem } from "../hooks/lib/initialization.mjs";
import { resolveProject } from "../hooks/lib/project.mjs";
import { captureWriterIdentity, recordGateEvidence } from "../hooks/lib/task-transitions.mjs";
import { evaluatePolicy } from "../hooks/lib/policy.mjs";
import { hookEvent, policyFixture } from "./hook-test-helpers.mjs";
import { runWorkflowCommand } from "../hooks/lib/workflow-cli.mjs";
import { effectiveRunFingerprint } from "../hooks/lib/run-state.mjs";

const run = promisify(execFile);
const cli = path.resolve(import.meta.dirname, "../hooks/agent-team-cli.mjs");
const boundedBeads = path.resolve(import.meta.dirname, "fixtures/bounded-beads-cli.mjs");
const dashboardBd = path.resolve(import.meta.dirname, "fixtures/bounded-dashboard-bd.mjs");
const dashboardBv = path.resolve(import.meta.dirname, "fixtures/bounded-dashboard-bv.mjs");
const temporary = [];

test.afterEach(async () => Promise.all(temporary.splice(0).map((target) => rm(target, { force: true, recursive: true }))));

async function fixture() {
  const root = await mkdtemp(path.join(os.tmpdir(), "agent-team-workflow-cli-"));
  temporary.push(root, `${root}-feature`, `${root}-remote`);
  const value = await policyFixture(root);
  const requests = path.join(root, ".agent-team", "requests");
  await mkdir(requests, { recursive: true });
  return { ...value, requests, project: await resolveProject(value.feature) };
}

async function admitRun(project, taskIds = ["AT-001"]) {
  const canonical = await loadCanonicalState(project);
  canonical.state.run = { id: "workflow-run", ownerSessionId: "owner-session", ownerHost: "codex", ownershipEpoch: 1, mode: "finite", taskIds,
    teamLimit: 1, autoDeploy: false, batchSize: 1, source: "explicit_run",
    settingSources: Object.fromEntries(["mode", "taskIds", "teamLimit", "autoDeploy", "batchSize"].map((key) => [key, "explicit_run"])),
    paused: false, operationalVersion: canonical.state.stateVersion ?? 0, blockers: [], pendingDeliveryIds: [], deployedTaskIds: [], terminalClassification: "progress_possible" };
  await writeFile(project.paths.state, JSON.stringify(canonical.state, null, 2));
  return loadCanonicalState(project);
}

async function requestFile(value, name, envelope) {
  const file = path.join(value.requests, `${name}.json`);
  await writeFile(file, `${JSON.stringify(envelope, null, 2)}\n`);
  return file;
}

async function invoke(command, ...args) {
  const { stdout } = await run(process.execPath, [cli, command, ...args], { encoding: "utf8", timeout: 5000 });
  return JSON.parse(stdout);
}

async function invokeWithEnvironment(environment, command, ...args) {
  const { stdout } = await run(process.execPath, [cli, command, ...args], { encoding: "utf8", timeout: 5000, env: { ...process.env, ...environment } });
  return JSON.parse(stdout);
}

async function configureDashboardGraph(value, graph) {
  const setupPath = path.join(value.root, ".agent-team", "setup.json");
  const setup = JSON.parse(await readFile(setupPath, "utf8"));
  setup.tracker = { kind: "beads", executable: dashboardBd };
  setup.dashboard = { ...(setup.dashboard || {}), graph };
  await writeFile(setupPath, JSON.stringify(setup));
  return setupPath;
}

async function invokeFailure(command, ...args) {
  try {
    await run(process.execPath, [cli, command, ...args], { encoding: "utf8", timeout: 2000 });
  } catch (error) {
    return JSON.parse(error.stdout);
  }
  assert.fail("CLI command unexpectedly succeeded");
}

function envelope(actorSessionId, expectedVersion, request) {
  return { schemaVersion: 1, actorSessionId, expectedVersion, request };
}

test("real CLI exposes read-only status, usage, recovery, and eligibility without creating a dashboard", async () => {
  const value = await fixture();
  const checkpoint = await requestFile(value, "checkpoint-read", envelope("developer-session", 0, {
    eventId: "checkpoint-read",
    sessionId: "developer-session",
    taskIds: ["AT-001"],
    worktree: value.feature,
    revision: value.revision,
    nextAction: "Continue the assigned task.",
  }));
  assert.equal((await invoke("checkpoint", "--project", value.feature, "--request", checkpoint)).status, "applied");

  const status = await invoke("status", "--project", value.feature);
  const usage = await invoke("usage", "--project", value.feature);
  const recovery = await invoke("recovery", "--project", value.feature, "--session", "developer-session", "--task", "AT-001", "--worktree", value.feature);
  const eligibility = await invoke("eligibility", "--project", value.feature);

  assert.equal(status.project.id, "project-1");
  assert.equal(usage.status, "completed");
  assert.equal(usage.source.kind, "operational_state");
  assert.equal(recovery.nextAction, "Continue the assigned task.");
  assert.deepEqual(eligibility.held.map(({ id }) => id), ["AT-001"]);
  await assert.rejects(access(path.join(value.root, ".agent-team", "dashboard", "index.html")), { code: "ENOENT" });
});

test("real CLI executes through a linked skill directory", async () => {
  const value = await fixture();
  const linkedSkill = path.join(value.feature, ".claude");
  await symlink(path.dirname(cli), linkedSkill, "dir");
  const { stdout } = await run(process.execPath, [path.join(linkedSkill, "agent-team-cli.mjs"), "status", "--project", value.root], { encoding: "utf8", timeout: 5000 });
  assert.equal(JSON.parse(stdout).project.id, "project-1");
});

test("run command routing is exact native-qualified and shell mutation safe", async () => {
  const root = await mkdtemp(path.join(os.tmpdir(), "agent-team-workflow-run-"));
  temporary.push(root, `${root}-feature`, `${root}-remote`);
  await policyFixture(root, { qualifiedOwnership: true });
  const project = await resolveProject(root);
  const canonical = await loadCanonicalState(project);
  const settingSources = Object.fromEntries(["mode", "taskIds", "teamLimit", "autoDeploy", "batchSize"].map((key) => [key, "explicit_run"]));
  const body = { operationId: "workflow-run-start", expectedTrackerFingerprint: canonical.tracker.fingerprint, reason: "Start workflow fixture run.",
    run: { id: "workflow-run", mode: "finite", taskIds: ["AT-001"], teamLimit: 1, autoDeploy: false, batchSize: 1, source: "explicit_run", settingSources } };
  const request = await requestFile({ requests: path.join(root, ".agent-team") }, "workflow-run-start", envelope("owner-session", canonical.state.stateVersion ?? 0, body));
  const before = await readFile(project.paths.state);
  assert.equal((await invoke("run-start", "--project", root, "--request", request)).reason, "project_owner_required");
  assert.deepEqual(await readFile(project.paths.state), before);
  const applied = await runWorkflowCommand("run-start", { project: root, request }, { nativeIdentity: { host: "codex", sessionId: "owner-session", observed: true, cwd: root, ownershipEpoch: 1 } });
  assert.equal(applied.status, "applied");
  const reconcileBody = { operationId: "workflow-run-reconcile", expectedTrackerFingerprint: canonical.tracker.fingerprint,
    expectedRunFingerprint: effectiveRunFingerprint(applied.result.run), authoritativeSource: "explicit_run", reason: "Confirm workflow fixture run.",
    affectedTaskIds: ["AT-001"], run: body.run };
  const reconcileRequest = await requestFile({ requests: path.join(root, ".agent-team") }, "workflow-run-reconcile",
    envelope("owner-session", applied.version, reconcileBody));
  assert.equal((await runWorkflowCommand("run-reconcile", { project: root, request: reconcileRequest }, { nativeIdentity: {
    host: "codex", sessionId: "owner-session", observed: true, cwd: root, ownershipEpoch: 1,
  } })).status, "applied");
  await assert.rejects(runWorkflowCommand("run-begin", { project: root, request }), /Unknown workflow command/);
});

test("scoped migration routes are exact closed and native qualified", async () => {
  const root = await mkdtemp(path.join(os.tmpdir(), "agent-team-workflow-scope-"));
  temporary.push(root, `${root}-feature`, `${root}-remote`);
  await policyFixture(root, { qualifiedOwnership: true });
  const project = await resolveProject(root);
  const state = (await loadCanonicalState(project)).state;
  state.run = { id: "scope-run", ownerSessionId: "owner-session", ownerHost: "codex", ownershipEpoch: 1, mode: "finite", taskIds: ["AT-001"],
    teamLimit: 1, autoDeploy: false, batchSize: 1, source: "explicit_run", settingSources: Object.fromEntries(["mode", "taskIds", "teamLimit", "autoDeploy", "batchSize"].map((key) => [key, "explicit_run"])),
    paused: false, operationalVersion: 0, blockers: [], pendingDeliveryIds: [], deployedTaskIds: [], terminalClassification: "progress_possible" };
  await writeFile(project.paths.state, JSON.stringify(state, null, 2));
  await writeFile(project.paths.tasks, `${await readFile(project.paths.tasks, "utf8")}| AT-002 | Later | none | AT-001 | ready | none | Claim. | task | |\n`);
  const current = await loadCanonicalState(project);
  const body = { operationId: "workflow-scope-extension", expectedTrackerFingerprint: current.tracker.fingerprint, taskIds: ["AT-002"], reason: "Admit later work." };
  const file = await requestFile({ requests: path.join(root, ".agent-team") }, "workflow-scope-extension", envelope("owner-session", current.state.stateVersion ?? 0, body));
  const before = await readFile(project.paths.state);
  assert.equal((await invoke("run-scope-extend", "--project", root, "--request", file)).reason, "project_owner_required");
  assert.deepEqual(await readFile(project.paths.state), before);
  assert.equal((await runWorkflowCommand("run-scope-extend", { project: root, request: file }, { nativeIdentity: {
    host: "codex", sessionId: "owner-session", observed: true, cwd: root, ownershipEpoch: 1,
  } })).status, "applied");
  const malformed = await requestFile({ requests: path.join(root, ".agent-team") }, "workflow-scope-extra", envelope("owner-session", 1, { ...body, operationId: "extra", extra: true }));
  assert.equal((await runWorkflowCommand("run-scope-extend", { project: root, request: malformed }, { nativeIdentity: {
    host: "codex", sessionId: "owner-session", observed: true, cwd: root, ownershipEpoch: 1,
  } })).reason, "invalid_request");
});

test("obsolete evidence root route is unknown before project access", async () => {
  await assert.rejects(runWorkflowCommand("evidence-root-register", { project: path.join(os.tmpdir(), "does-not-exist"), request: "missing" }), /Unknown workflow command/);
});

test("run decision is shell-readable and byte identical", async () => {
  const root = await mkdtemp(path.join(os.tmpdir(), "agent-team-workflow-decision-"));
  temporary.push(root, `${root}-feature`, `${root}-remote`);
  await policyFixture(root, { qualifiedOwnership: true });
  const project = await resolveProject(root);
  const before = await Promise.all([project.paths.state, project.paths.setup, project.paths.teams, project.paths.ownerHistory, project.paths.tasks].map((file) => readFile(file)));
  const decision = await invoke("run-decision", "--project", root);
  assert.equal(decision.status, "unavailable");
  const after = await Promise.all([project.paths.state, project.paths.setup, project.paths.teams, project.paths.ownerHistory, project.paths.tasks].map((file) => readFile(file)));
  assert.deepEqual(after, before);
});

test("real CLI recovery probes the selected linked worktree instead of canonical main", async () => {
  const value = await fixture();
  execFileSync("git", ["commit", "--allow-empty", "-qm", "linked revision"], { cwd: value.feature });
  const revision = execFileSync("git", ["rev-parse", "HEAD"], { cwd: value.feature, encoding: "utf8" }).trim();
  assert.notEqual(revision, value.revision);
  const checkpoint = await requestFile(value, "linked-recovery", envelope("developer-session", 0, {
    eventId: "linked-recovery", sessionId: "developer-session", taskIds: ["AT-001"],
    worktree: value.feature, revision, evidenceRevision: revision, nextAction: "Review the linked revision.",
  }));
  assert.equal((await invoke("checkpoint", "--project", value.root, "--request", checkpoint)).status, "applied");
  const recover = () => invoke("recovery", "--project", value.root, "--session", "developer-session", "--task", "AT-001",
    "--worktree", path.relative(value.root, value.feature), "--include-git", "true");
  const result = await recover();
  assert.equal(result.worktree, value.feature);
  assert.equal(result.git.branch.value, "feature");
  assert.equal(result.git.revision.value, revision);
  assert.equal(result.status, "current");
  assert.equal(result.evidenceStatus, "current");
  const recorded = JSON.parse(await readFile(result.path, "utf8"));
  recorded.worktree = path.relative(value.root, value.feature);
  await writeFile(result.path, JSON.stringify(recorded));
  assert.equal((await recover()).evidenceStatus, "current");
  await writeFile(path.join(value.feature, "src", "owned.js"), "// changed after checkpoint\n");
  assert.equal((await recover()).evidenceStatus, "stale");
  const unmatched = await invoke("recovery", "--project", value.root, "--session", "developer-session", "--task", "AT-001",
    "--worktree", value.remote, "--include-git", "true");
  assert.equal(unmatched.status, "unavailable");
  assert.equal(unmatched.worktree, value.root);
  assert.equal(unmatched.git.revision.value, value.revision);
});

test("real CLI forwards versioned transition, gate-evidence, and cleanup requests to canonical APIs", async () => {
  const value = await fixture();
  const canonical = await admitRun(value.project);
  const transition = await requestFile(value, "pause", envelope("owner-session", 0, {
    operationId: "cli-pause",
    taskId: "AT-001",
    expectedFingerprint: canonical.tracker.fingerprint,
    expectedOwner: "TEAM-001",
    action: "pause",
  }));
  const paused = await invoke("task-transition", "--project", value.feature, "--request", transition);
  assert.equal(paused.status, "applied");
  assert.equal((await invoke("task-transition", "--project", value.feature, "--request", transition)).status, "duplicate");

  const afterPause = await loadCanonicalState(value.project);
  const evidencePath = path.join(value.root, ".agent-team", "evidence", "cli-gate.json");
  await mkdir(path.dirname(evidencePath), { recursive: true });
  await writeFile(evidencePath, JSON.stringify({
    status: "passed", revision: value.revision, taskIds: ["AT-001"], requirementsReconciled: true,
    review: { status: "passed", revision: value.revision, taskId: "AT-001" },
    checks: [{ name: "workflow-cli", status: "passed", revision: value.revision, taskId: "AT-001" }],
  }));
  const gate = await requestFile(value, "gate", envelope("owner-session", afterPause.state.stateVersion, {
    operationId: "cli-gate",
    gate: "completion",
    taskIds: ["AT-001"],
    expectedFingerprint: afterPause.tracker.fingerprint,
    expectedRevision: value.revision,
    evidencePath,
  }));
  assert.equal((await invoke("gate-evidence", "--project", value.feature, "--request", gate)).status, "applied");

  const current = await loadCanonicalState(value.project);
  const cleanup = await requestFile(value, "cleanup", envelope("owner-session", current.state.stateVersion, {
    operationId: "cli-cleanup",
    taskId: "AT-001",
    expectedRevision: value.revision,
    worktree: value.feature,
    expectedWriter: { pid: 99999999, startTime: "1", bootId: "not-current", host: os.hostname() },
  }));
  const retained = await invoke("cleanup", "--project", value.feature, "--request", cleanup);
  assert.deepEqual(retained, { status: "conflict", reason: "retain_identity_mismatch" });
  await access(value.feature);
});

test("real CLI persists integration evidence that policy can consume", async () => {
  const value = await fixture();
  const canonical = await admitRun(value.project);
  const evidencePath = path.join(value.root, ".agent-team", "evidence", "cli-integration.json");
  await mkdir(path.dirname(evidencePath), { recursive: true });
  await writeFile(evidencePath, JSON.stringify({
    status: "passed", revision: value.revision, taskIds: ["AT-001"],
    remote: { name: "origin", baseRef: "refs/heads/main", revision: value.revision, targetRef: "refs/heads/feature", targetRevision: value.revision },
    authorization: { source: "accepted-packet", scope: "integration", ownerSessionId: "owner-session", revision: value.revision, taskIds: ["AT-001"] },
    recovery: { status: "reconciled", revision: value.revision, taskIds: ["AT-001"] }, preview: { required: false }, remoteMainDeploys: false,
  }));
  const gate = await requestFile(value, "integration-gate", envelope("owner-session", canonical.state.stateVersion ?? 0, {
    operationId: "cli-integration-gate", gate: "integration", taskIds: ["AT-001"], expectedFingerprint: canonical.tracker.fingerprint,
    expectedRevision: value.revision, evidencePath,
  }));
  assert.equal((await invoke("gate-evidence", "--project", value.feature, "--request", gate)).status, "applied");
  const integration = (await loadCanonicalState(value.project)).state.integration;
  assert.equal(integration.authorization.source, "accepted-packet");
  assert.equal(integration.baseRef, "main");
  const consumed = await evaluatePolicy(hookEvent(value, { sessionId: "owner-session", operation: { kind: "shell", command: `git -C ${value.feature} push origin HEAD:feature` } }), value.project);
  assert.equal(consumed.allow, true);
});

test("real CLI maps a fresh initialized integration record and a raw autoDeploy flag cannot bypass the main deployment gate", async () => {
  const value = await fixture();
  const state = structuredClone((await admitRun(value.project)).state);
  state.integration = { ownerSessionId: "owner-session", authorized: false, baseRef: "main", paused: false, hold: false };
  state.release.autoDeploy = false;
  await writeFile(value.project.paths.state, JSON.stringify(state));
  const canonical = await loadCanonicalState(value.project);
  const evidencePath = path.join(value.root, ".agent-team", "evidence", "fresh-main.json");
  await mkdir(path.dirname(evidencePath), { recursive: true });
  await writeFile(evidencePath, JSON.stringify({
    status: "passed", revision: value.revision, taskIds: ["AT-001"],
    remote: { name: "origin", baseRef: "refs/heads/main", revision: value.revision, targetRef: "refs/heads/main", targetRevision: value.revision },
    authorization: { source: "accepted-packet", scope: "integration", ownerSessionId: "owner-session", revision: value.revision, taskIds: ["AT-001"] },
    recovery: { status: "reconciled", revision: value.revision, taskIds: ["AT-001"] }, preview: { required: false }, remoteMainDeploys: true,
  }));
  const request = await requestFile(value, "fresh-main", envelope("owner-session", canonical.state.stateVersion ?? 0, {
    operationId: "fresh-main", gate: "integration", taskIds: ["AT-001"], expectedFingerprint: canonical.tracker.fingerprint,
    expectedRevision: value.revision, evidencePath,
  }));
  assert.equal((await invoke("gate-evidence", "--project", value.feature, "--request", request)).status, "applied");
  const blocked = await evaluatePolicy(hookEvent(value, { sessionId: "owner-session", operation: { kind: "shell", command: `git -C ${value.feature} push origin HEAD:main` } }), value.project);
  assert.equal(blocked.allow, false);
  const mapped = (await loadCanonicalState(value.project)).state;
  mapped.release.autoDeploy = true;
  await writeFile(value.project.paths.state, JSON.stringify(mapped));
  const stillBlocked = await evaluatePolicy(hookEvent(value, { sessionId: "owner-session", operation: { kind: "shell", command: `git -C ${value.feature} push origin HEAD:main` } }), value.project);
  assert.equal(stillBlocked.allow, false);
});

test("real CLI maps one release batch before allowing its deployment-triggering main push", async () => {
  // This test catches a successful release evidence receipt that leaves the main deployment trigger disabled.
  const value = await fixture();
  const seeded = structuredClone((await admitRun(value.project)).state);
  seeded.integration = { ownerSessionId: "owner-session", authorized: false, baseRef: "main", paused: false, hold: false };
  seeded.release = { ownerSessionId: "owner-session", authorized: false, autoDeploy: false, hold: true };
  await writeFile(value.project.paths.state, JSON.stringify(seeded));
  const canonical = await loadCanonicalState(value.project);
  const integrationPath = path.join(value.root, ".agent-team", "evidence", "main-integration.json");
  await mkdir(path.dirname(integrationPath), { recursive: true });
  await writeFile(integrationPath, JSON.stringify({
    status: "passed", revision: value.revision, taskIds: ["AT-001"],
    remote: { name: "origin", baseRef: "refs/heads/main", revision: value.revision, targetRef: "refs/heads/main", targetRevision: value.revision },
    authorization: { source: "explicit-main-authorization", scope: "integration", ownerSessionId: "owner-session", revision: value.revision, taskIds: ["AT-001"] },
    recovery: { status: "reconciled", revision: value.revision, taskIds: ["AT-001"] }, preview: { required: false }, remoteMainDeploys: true,
  }));
  const integrationRequest = await requestFile(value, "main-integration", envelope("owner-session", canonical.state.stateVersion ?? 0, {
    operationId: "main-integration", gate: "integration", taskIds: ["AT-001"], expectedFingerprint: canonical.tracker.fingerprint,
    expectedRevision: value.revision, evidencePath: integrationPath,
  }));
  assert.equal((await invoke("gate-evidence", "--project", value.feature, "--request", integrationRequest)).status, "applied");
  const pushEvent = hookEvent(value, { sessionId: "owner-session", operation: { kind: "shell", command: `git -C ${value.feature} push origin HEAD:main` } });
  const beforeRelease = await evaluatePolicy(pushEvent, value.project);
  assert.equal(beforeRelease.allow, false);
  assert.match(beforeRelease.messages.join("\n"), /automatic deployment is off/);

  const afterIntegration = await loadCanonicalState(value.project);
  const releasePath = path.join(value.root, ".agent-team", "evidence", "release.json");
  const releaseEvidence = {
    status: "passed", revision: value.revision, taskIds: ["AT-001"], ownerSessionId: "owner-session", authorized: true,
    expectedRevision: value.revision, target: "github:example/project:v1.0.0", process: "gh-release",
    authorization: { source: "explicit one-time user authorization", target: "github:example/project:v1.0.0", process: "gh-release",
      scope: "batch-1", ownerSessionId: "owner-session", grantedAt: "2026-09-10" },
    run: { id: "release-1", mode: "auto_deploy", taskIds: ["AT-001"], paused: false }, runMode: "auto_deploy", autoDeploy: true,
    batchId: "batch-1", batch: { id: "batch-1", taskIds: ["AT-001"] },
    artifact: { id: "artifact-1", revision: value.revision, taskIds: ["AT-001"], sha256: "a".repeat(64) },
    integration: { status: "passed", revision: value.revision, taskIds: ["AT-001"], recordedTaskIds: ["AT-001"],
      evidencePath: integrationPath, remoteName: "origin", baseRef: "refs/heads/main", baseRevision: value.revision,
      targetRef: "refs/heads/main", targetRevision: value.revision, remoteMainDeploys: true },
    verification: { status: "passed", revision: value.revision, taskIds: ["AT-001"] },
    preview: { required: false, status: "not_required", revision: value.revision },
    delta: { status: "clean", revision: value.revision, taskIds: ["AT-001"] },
    recovery: { status: "verified", artifactId: "git:known-good", action: "restore the known-good revision" },
    projectPaused: false, hold: false,
  };
  const releaseBefore = await readFile(value.project.paths.state, "utf8");
  const invalidIntegrationBindings = [
    ["remote", (evidence) => { evidence.integration.remoteName = "backup"; }],
    ["base-ref", (evidence) => { evidence.integration.baseRef = "refs/heads/other"; }],
    ["base-revision", (evidence) => { evidence.integration.baseRevision = "b".repeat(40); }],
    ["target", (evidence) => { evidence.integration.targetRef = "refs/tags/v9.9.9"; }],
    ["evidence-path", (evidence) => { evidence.integration.evidencePath = "/tmp/fabricated-integration.json"; }],
  ];
  for (const [name, invalidate] of invalidIntegrationBindings) {
    const invalid = structuredClone(releaseEvidence);
    invalidate(invalid);
    const invalidPath = path.join(value.root, ".agent-team", "evidence", `release-${name}.json`);
    await writeFile(invalidPath, JSON.stringify(invalid));
    const invalidRequest = await requestFile(value, `release-${name}`, envelope("owner-session", afterIntegration.state.stateVersion, {
      operationId: `release-${name}`, gate: "release", taskIds: ["AT-001"], expectedFingerprint: afterIntegration.tracker.fingerprint,
      expectedRevision: value.revision, evidencePath: invalidPath,
    }));
    assert.deepEqual(await invoke("gate-evidence", "--project", value.feature, "--request", invalidRequest),
      { status: "conflict", reason: "release_evidence_mismatch" }, name);
    assert.equal(await readFile(value.project.paths.state, "utf8"), releaseBefore, name);
  }
  await writeFile(releasePath, JSON.stringify(releaseEvidence));
  const releaseRequest = await requestFile(value, "release", envelope("owner-session", afterIntegration.state.stateVersion, {
    operationId: "release", gate: "release", taskIds: ["AT-001"], expectedFingerprint: afterIntegration.tracker.fingerprint,
    expectedRevision: value.revision, evidencePath: releasePath,
  }));
  assert.equal((await invoke("gate-evidence", "--project", value.feature, "--request", releaseRequest)).status, "applied");

  assert.equal((await evaluatePolicy(pushEvent, value.project)).allow, true);
  const exactProcess = await evaluatePolicy(hookEvent(value, { sessionId: "owner-session", operation: { kind: "shell", command: "gh release create v1.0.0" } }), value.project);
  const wrongProcess = await evaluatePolicy(hookEvent(value, { sessionId: "owner-session", operation: { kind: "shell", command: "npm publish" } }), value.project);
  assert.equal(exactProcess.allow, true);
  assert.equal(wrongProcess.allow, false);
  const release = (await loadCanonicalState(value.project)).state.release;
  assert.equal(release.runMode, "auto_deploy");
  assert.equal(release.autoDeploy, true);
  assert.equal(release.remoteMainDeploys, true);
  assert.deepEqual(release.taskIds, ["AT-001"]);

  execFileSync("git", ["push", "-q", "origin", "HEAD:refs/heads/main"], { cwd: value.feature });
  const beforeTag = await loadCanonicalState(value.project);
  const tagPath = path.join(value.root, ".agent-team", "evidence", "tag-integration.json");
  await writeFile(tagPath, JSON.stringify({
    status: "passed", revision: value.revision, taskIds: ["AT-001"],
    remote: { name: "origin", baseRef: "refs/heads/main", revision: value.revision, targetRef: "refs/tags/v1.0.0", targetAbsent: true },
    authorization: { source: "explicit-tag-authorization", scope: "integration", ownerSessionId: "owner-session", revision: value.revision, taskIds: ["AT-001"] },
    recovery: { status: "reconciled", revision: value.revision, taskIds: ["AT-001"] }, preview: { required: false }, remoteMainDeploys: false,
  }));
  const tagRequest = await requestFile(value, "tag-integration", envelope("owner-session", beforeTag.state.stateVersion, {
    operationId: "tag-integration", gate: "integration", taskIds: ["AT-001"], expectedFingerprint: beforeTag.tracker.fingerprint,
    expectedRevision: value.revision, evidencePath: tagPath,
  }));
  assert.equal((await invoke("gate-evidence", "--project", value.feature, "--request", tagRequest)).status, "applied");
  const tagCommand = `git -C ${value.feature} push origin HEAD:refs/tags/v1.0.0`;
  assert.equal((await evaluatePolicy(hookEvent(value, { sessionId: "owner-session", operation: { kind: "shell", command: tagCommand } }), value.project)).allow, true);
  for (const command of [
    `git -C ${value.feature} push backup HEAD:refs/tags/v1.0.0`,
    `git -C ${value.feature} push origin HEAD:refs/tags/v1.0.1`,
    `git -C ${value.feature} push origin HEAD:refs/tags/v1.0.0 HEAD:refs/tags/v1.0.1`,
  ]) assert.equal((await evaluatePolicy(hookEvent(value, { sessionId: "owner-session", operation: { kind: "shell", command } }), value.project)).allow, false, command);
  execFileSync("git", ["push", "-q", "origin", "HEAD:refs/tags/v1.0.0"], { cwd: value.feature });
  assert.equal((await evaluatePolicy(hookEvent(value, { sessionId: "owner-session", operation: { kind: "shell", command: tagCommand } }), value.project)).allow, false);
});

test("integration evidence keeps an actual initialized project structurally ready", async () => {
  const root = await mkdtemp(path.join(os.tmpdir(), "agent-team-fresh-integration-"));
  const remote = `${root}-remote`;
  temporary.push(root, remote);
  execFileSync("git", ["init", "-q", "-b", "main", root]);
  execFileSync("git", ["config", "user.name", "Gate Test"], { cwd: root });
  execFileSync("git", ["config", "user.email", "gate@example.test"], { cwd: root });
  await writeFile(path.join(root, "README.md"), "fixture\n");
  await writeFile(path.join(root, ".gitignore"), ".agent-team/\n");
  execFileSync("git", ["add", "."], { cwd: root });
  execFileSync("git", ["commit", "-qm", "fixture"], { cwd: root });
  execFileSync("git", ["init", "-q", "--bare", remote]);
  execFileSync("git", ["remote", "add", "origin", remote], { cwd: root });
  execFileSync("git", ["push", "-q", "-u", "origin", "main"], { cwd: root });
  const request = { projectId: "fresh-project", operationId: "initialize-fresh", source: "standalone",
    tracker: { kind: "markdown", path: ".agent-team/TASKS.md" }, plan: { scope: "Gate test", acceptance: ["Gate passes"], branch: "main", verification: ["node --test"],
      authority: { ownedPaths: ["tests/**"] }, tasks: [{ id: "AT-001", title: "Gate", status: "ready", dependencies: [] }] } };
  assert.equal((await initializeProject(root, request, { actorSessionId: "owner-session",
    nativeIdentity: { host: "codex", sessionId: "owner-session", observed: true, cwd: root, ownershipEpoch: 1 } })).status, "applied");
  const project = await resolveProject(root);
  const canonical = await admitRun(project);
  const revision = execFileSync("git", ["rev-parse", "HEAD"], { cwd: root, encoding: "utf8" }).trim();
  const completionPath = path.join(root, ".agent-team", "completion.json");
  await writeFile(completionPath, JSON.stringify({ status: "passed", revision, taskIds: ["AT-001"], requirementsReconciled: true,
    review: { status: "passed", revision, taskId: "AT-001" }, checks: [{ name: "unit", status: "passed", revision, taskId: "AT-001" }] }));
  const completion = await recordGateEvidence(project, { actorSessionId: "owner-session", operationId: "fresh-completion", expectedVersion: canonical.state.stateVersion,
    expectedFingerprint: canonical.tracker.fingerprint, gate: "completion", taskIds: ["AT-001"], expectedRevision: revision, evidencePath: completionPath },
  { nativeIdentity: { host: "codex", sessionId: "owner-session", observed: true, cwd: root, ownershipEpoch: 1 } });
  assert.equal(completion.status, "applied", JSON.stringify(completion));
  const afterCompletion = await loadCanonicalState(project);
  const evidencePath = path.join(root, ".agent-team", "integration.json");
  await writeFile(evidencePath, JSON.stringify({ status: "passed", revision, taskIds: ["AT-001"],
    sourceRevisions: { "AT-001": revision },
    remote: { name: "origin", baseRef: "refs/heads/main", revision, targetRef: "refs/heads/main", targetRevision: revision },
    authorization: { source: "accepted-packet", scope: "integration", ownerSessionId: "owner-session", revision, taskIds: ["AT-001"] },
    targetAuthorization: { status: "authorized", source: "accepted-packet", target: "refs/heads/main", revision, taskIds: ["AT-001"],
      ownerHost: "codex", ownerSessionId: "owner-session", ownershipEpoch: 1 },
    recovery: { status: "reconciled", revision, taskIds: ["AT-001"], artifactId: "git:known-good", action: "rollback" },
    preview: { required: false }, remoteMainDeploys: false }));
  const recorded = await recordGateEvidence(project, { actorSessionId: "owner-session", operationId: "fresh-integration", expectedVersion: afterCompletion.state.stateVersion,
    expectedFingerprint: afterCompletion.tracker.fingerprint, gate: "integration", taskIds: ["AT-001"], expectedRevision: revision, evidencePath },
  { nativeIdentity: { host: "codex", sessionId: "owner-session", observed: true, cwd: root, ownershipEpoch: 1 } });
  assert.equal(recorded.status, "applied", JSON.stringify(recorded));
  const mapped = await loadCanonicalState(project);
  assert.equal(initializationRecordProblem(project.setup, mapped, { projectRoot: root, validateTracker: true }), undefined);
});

test("real CLI claims a canonical Beads task whose unassigned owner is the empty string", async () => {
  const value = await fixture();
  const setupPath = path.join(value.root, ".agent-team", "setup.json");
  const setup = JSON.parse(await readFile(setupPath, "utf8"));
  setup.tracker = { kind: "beads", executable: boundedBeads };
  await writeFile(setupPath, JSON.stringify(setup));
  const teamsPath = path.join(value.root, ".agent-team", "TEAMS.md");
  await writeFile(teamsPath, (await readFile(teamsPath, "utf8")).replaceAll("AT-001", "AT-BEADS"));
  const project = await resolveProject(value.feature);
  const canonical = await loadCanonicalState(project);
  assert.equal(canonical.tasks[0].owner, "");
  const claim = await requestFile(value, "beads-claim", envelope("owner-session", 0, {
    operationId: "beads-empty-owner-claim",
    taskId: "AT-BEADS",
    expectedFingerprint: canonical.tracker.fingerprint,
    expectedOwner: "",
    action: "claim",
    owner: "TEAM-001",
  }));

  const result = await invoke("task-transition", "--project", value.feature, "--request", claim);

  assert.equal(result.status, "applied");
  assert.equal((await loadCanonicalState(project)).tasks[0].owner, "TEAM-001");
});

test("real CLI records an observed claim writer and rejects malformed writer input", async () => {
  const value = await fixture();
  await writeFile(value.project.paths.tasks, (await readFile(value.project.paths.tasks, "utf8")).replace("| TEAM-001 | none | in_progress |", "| none | none | ready |"));
  const canonical = await loadCanonicalState(value.project);
  const writer = await captureWriterIdentity();
  const body = { operationId: "cli-claim-writer", taskId: "AT-001", action: "claim", owner: "TEAM-001",
    expectedOwner: "none", expectedFingerprint: canonical.tracker.fingerprint, writer };
  const malformed = await requestFile(value, "malformed-claim-writer", envelope("owner-session", 0, { ...body, writer: {} }));
  assert.match((await invokeFailure("task-transition", "--project", value.feature, "--request", malformed)).error, /request.writer/);
  const claim = await requestFile(value, "claim-writer", envelope("owner-session", 0, body));
  assert.equal((await invoke("task-transition", "--project", value.feature, "--request", claim)).status, "applied");
  const current = await loadCanonicalState(value.project);
  assert.deepEqual(current.state.taskRuntime["AT-001"].writer, writer);
  assert.equal(current.tasks[0].owner, "TEAM-001");
});

test("real CLI checkpoint and explicit resume accept a relative registered worktree but reject another checkout", async () => {
  const value = await fixture();
  const teams = await readFile(value.project.paths.teams, "utf8");
  await writeFile(value.project.paths.teams, teams.replace(value.feature, path.relative(value.root, value.feature)));
  const child = spawn(process.execPath, [path.join(import.meta.dirname, "hooks-transitions.test.mjs"), "writer"], { stdio: ["ignore", "pipe", "ignore"] });
  await new Promise((resolve) => child.stdout.once("data", resolve));
  try {
    const previousWriter = await captureWriterIdentity(child.pid);
    const canonical = await loadCanonicalState(value.project);
    await writeFile(value.project.paths.state, JSON.stringify({ ...canonical.state, taskRuntime: { "AT-001": { writer: previousWriter, compute: "active" } } }));
    const pause = await requestFile(value, "relative-pause", envelope("owner-session", 0, {
      operationId: "relative-pause", taskId: "AT-001", action: "pause", expectedOwner: "TEAM-001", expectedFingerprint: canonical.tracker.fingerprint,
    }));
    assert.equal((await invoke("task-transition", "--project", value.feature, "--request", pause)).status, "applied");
    const stopped = new Promise((resolve) => child.once("exit", resolve));
    child.kill("SIGTERM");
    await stopped;
    const current = await loadCanonicalState(value.project);
    const writer = await captureWriterIdentity();
    for (const [index, worktree] of [value.root, path.relative(value.root, value.feature)].entries()) {
      const checkpointRequest = await requestFile(value, `relative-checkpoint-${index}`, envelope("developer-session", index, {
        eventId: `relative-checkpoint-${index}`, sessionId: "developer-session", taskIds: ["AT-001"], worktree,
        revision: value.revision, evidenceRevision: value.revision, nextAction: "Resume the assigned implementation.",
      }));
      const checkpoint = await invoke("checkpoint", "--project", value.feature, "--request", checkpointRequest);
      assert.equal(checkpoint.status, "applied");
      const resumeRequest = await requestFile(value, `relative-resume-${index}`, envelope("owner-session", current.state.stateVersion, {
        operationId: `relative-resume-${index}`, taskId: "AT-001", action: "resume", explicitResume: true,
        expectedOwner: "TEAM-001", expectedFingerprint: current.tracker.fingerprint, writer, checkpointPath: checkpoint.path,
      }));
      const resumed = await invoke("task-transition", "--project", value.feature, "--request", resumeRequest);
      if (index === 0) assert.equal(resumed.reason, "checkpoint_identity_mismatch");
      else assert.equal(resumed.status, "applied", JSON.stringify(resumed));
    }
    const after = await loadCanonicalState(value.project);
    assert.equal(after.tasks[0].status, "in_progress");
    assert.deepEqual(after.state.taskRuntime["AT-001"].writer, writer);
  } finally {
    if (child.exitCode === null && child.signalCode === null) child.kill("SIGTERM");
  }
});

test("request JSON must be a bounded regular file with valid schema, actor, version, and command fields", async () => {
  const value = await fixture();
  const missingActor = await requestFile(value, "missing-actor", { schemaVersion: 1, expectedVersion: 0, request: {} });
  assert.match((await invokeFailure("checkpoint", "--project", value.feature, "--request", missingActor)).error, /actorSessionId/);
  const wrongSchema = await requestFile(value, "wrong-schema", envelope("owner-session", 0, {}));
  await writeFile(wrongSchema, JSON.stringify({ ...JSON.parse(await readFile(wrongSchema, "utf8")), schemaVersion: 2 }));
  assert.match((await invokeFailure("checkpoint", "--project", value.feature, "--request", wrongSchema)).error, /schemaVersion/);
  const wrongVersion = await requestFile(value, "wrong-version", envelope("owner-session", "latest", {}));
  assert.match((await invokeFailure("checkpoint", "--project", value.feature, "--request", wrongVersion)).error, /expectedVersion/);
  const missingEvent = await requestFile(value, "missing-event", envelope("owner-session", 0, { sessionId: "owner-session" }));
  assert.match((await invokeFailure("checkpoint", "--project", value.feature, "--request", missingEvent)).error, /eventId/);
  assert.match((await invokeFailure("task-transition", "--project", value.feature, "--request", missingActor)).error, /actorSessionId/);
  const canonical = await loadCanonicalState(value.project);
  const missingOwner = await requestFile(value, "missing-owner", envelope("owner-session", 0, {
    operationId: "missing-owner",
    taskId: "AT-001",
    expectedFingerprint: canonical.tracker.fingerprint,
    expectedOwner: "TEAM-001",
    action: "claim",
  }));
  assert.match((await invokeFailure("task-transition", "--project", value.feature, "--request", missingOwner)).error, /request.owner/);
  const invalidWriter = await requestFile(value, "invalid-writer", envelope("owner-session", 0, {
    operationId: "invalid-writer",
    taskId: "AT-001",
    expectedRevision: value.revision,
    worktree: value.feature,
    expectedWriter: {},
  }));
  assert.match((await invokeFailure("cleanup", "--project", value.feature, "--request", invalidWriter)).error, /expectedWriter/);
  assert.match((await invokeFailure("status", "--project", value.feature, "--unknown", "value")).error, /Unsupported flag/);

  const oversized = path.join(value.requests, "oversized.json");
  await writeFile(oversized, JSON.stringify(envelope("owner-session", 0, { eventId: "large", sessionId: "owner-session", padding: "x".repeat(256 * 1024) })));
  assert.match((await invokeFailure("checkpoint", "--project", value.feature, "--request", oversized)).error, /bounded regular JSON file/);

  const fifo = path.join(value.requests, "request.fifo");
  execFileSync("mkfifo", [fifo]);
  const started = Date.now();
  assert.match((await invokeFailure("checkpoint", "--project", value.feature, "--request", fifo)).error, /regular JSON file/);
  assert.ok(Date.now() - started < 1500, "FIFO validation must not wait for a writer");
});

test("health forwards explicit project scope", async () => {
  const value = await fixture();
  const result = await invoke("health", "--home", value.root, "--project", value.feature, "--scope", "project");
  assert.equal(result.installation.scope, "project");
  assert.equal(result.installation.root, value.root);
});

test("dashboard snapshot uses the fixed path and configured refresh remains opt-in", async () => {
  const value = await fixture();
  const destination = path.join(value.root, ".agent-team", "dashboard", "index.html");
  const result = await invoke("dashboard-snapshot", "--project", value.feature);
  assert.equal(result.destination, destination);
  assert.match(await readFile(destination, "utf8"), /LOCAL STATUS SNAPSHOT/);

  const module = await import("../hooks/lib/workflow-cli.mjs").catch(() => ({}));
  assert.equal(typeof module.refreshConfiguredDashboard, "function", "configured dashboard refresh helper is required");
  const unconfigured = await fixture();
  assert.deepEqual(await module.refreshConfiguredDashboard(unconfigured.project), { status: "skipped", reason: "not_configured" });
  await assert.rejects(access(path.join(unconfigured.root, ".agent-team", "dashboard", "index.html")), { code: "ENOENT" });
  const setupPath = path.join(unconfigured.root, ".agent-team", "setup.json");
  const setup = JSON.parse(await readFile(setupPath, "utf8"));
  setup.dashboard = { snapshot: true };
  await writeFile(setupPath, JSON.stringify(setup));
  const configured = await module.refreshConfiguredDashboard(await resolveProject(unconfigured.feature));
  assert.equal(configured.status, "published");
});

test("actual dashboard snapshot CLI derives the explicitly configured Beads graph with attribution", async () => {
  const value = await fixture();
  const log = path.join(value.root, "bounded-bv.log");
  await configureDashboardGraph(value, { enabled: true, termsAcknowledged: true, executable: dashboardBv });

  const result = await invokeWithEnvironment({ AGENT_TEAM_BOUNDED_GRAPH_LOG: log }, "dashboard-snapshot", "--project", value.feature);
  const html = await readFile(result.destination, "utf8");

  assert.equal(result.status, "published");
  assert.match(html, /AT-GRAPH-A/);
  assert.match(html, /AT-GRAPH-A --blocks--&gt; AT-GRAPH-B/);
  assert.match(html, /beads_viewer by Jeffrey Emanuel/);
  assert.match(html, /"attribution":\{"repository":/);
  assert.match(await readFile(log, "utf8"), /--robot-graph/);
});

test("dashboard CLI never invokes bv when graph selection or terms acknowledgement is absent", async () => {
  for (const graph of [
    { enabled: false, termsAcknowledged: true, executable: dashboardBv },
    { enabled: true, termsAcknowledged: false, executable: dashboardBv },
    { enabled: true, termsAcknowledged: true, executable: "relative-bv" },
  ]) {
    const value = await fixture();
    const log = path.join(value.root, "bounded-bv.log");
    await configureDashboardGraph(value, graph);
    const result = await invokeWithEnvironment({ AGENT_TEAM_BOUNDED_GRAPH_LOG: log }, "dashboard-snapshot", "--project", value.feature);
    const html = await readFile(result.destination, "utf8");
    assert.match(html, /AT-GRAPH-A/);
    assert.doesNotMatch(html, /<svg /);
    await assert.rejects(access(log), { code: "ENOENT" });
  }
});

test("dashboard CLI preserves the complete task view when optional graph rendering fails", async () => {
  const value = await fixture();
  await configureDashboardGraph(value, { enabled: true, termsAcknowledged: true, executable: dashboardBv });

  const result = await invokeWithEnvironment({ AGENT_TEAM_BOUNDED_GRAPH_MODE: "failed" }, "dashboard-snapshot", "--project", value.feature);
  const html = await readFile(result.destination, "utf8");

  assert.equal(result.status, "published");
  assert.match(html, /Graph source task/);
  assert.match(html, /Graph dependent task/);
  assert.doesNotMatch(html, /<svg /);
  assert.match(html, /"graph":\{"status":"unavailable"/);
});

test("dashboard snapshot publishes the base task view before an optional graph deadline", async () => {
  const value = await fixture();
  const log = path.join(value.root, "bounded-bv.log");
  await configureDashboardGraph(value, { enabled: true, termsAcknowledged: true, executable: dashboardBv });

  const result = await invokeWithEnvironment({ AGENT_TEAM_BOUNDED_GRAPH_MODE: "slow", AGENT_TEAM_BOUNDED_GRAPH_LOG: log }, "dashboard-snapshot", "--project", value.feature);
  const html = await readFile(result.destination, "utf8");

  assert.equal(result.status, "published");
  assert.match(html, /Graph source task/);
  assert.match(html, /Graph dependent task/);
  assert.doesNotMatch(html, /<svg /);
  assert.match(await readFile(log, "utf8"), /--help/);
  await new Promise((resolve) => setTimeout(resolve, 220));
  assert.doesNotMatch(await readFile(log, "utf8"), /--help:completed/);
});

test("live dashboard re-resolves graph configuration for each open or refresh", async () => {
  const value = await fixture();
  const setupPath = await configureDashboardGraph(value, { enabled: false, termsAcknowledged: false, executable: dashboardBv });
  const log = path.join(value.root, "bounded-bv.log");
  const child = spawn(process.execPath, [cli, "dashboard-start", "--project", value.feature, "--port", "0"], {
    stdio: ["ignore", "pipe", "pipe"], env: { ...process.env, AGENT_TEAM_BOUNDED_GRAPH_LOG: log },
  });
  let stdout = "";
  let stderr = "";
  child.stdout.on("data", (chunk) => { stdout += chunk; });
  child.stderr.on("data", (chunk) => { stderr += chunk; });
  const listening = await new Promise((resolve, reject) => {
    const timeout = setTimeout(() => reject(new Error(`dashboard did not start: ${stderr}`)), 3000);
    child.stdout.on("data", () => {
      const line = stdout.split("\n").find(Boolean);
      if (line) { clearTimeout(timeout); resolve(JSON.parse(line)); }
    });
  });
  const get = () => new Promise((resolve, reject) => {
    http.get(listening.url, (response) => {
      let source = "";
      response.setEncoding("utf8");
      response.on("data", (chunk) => { source += chunk; });
      response.on("end", () => resolve(source));
    }).on("error", reject);
  });
  try {
    const setup = JSON.parse(await readFile(setupPath, "utf8"));
    setup.dashboard.graph = { enabled: true, termsAcknowledged: true, executable: dashboardBv };
    await writeFile(setupPath, JSON.stringify(setup));
    assert.match(await get(), /AT-GRAPH-A --blocks--&gt; AT-GRAPH-B/);
    const before = await readFile(log, "utf8");
    setup.dashboard.graph.termsAcknowledged = false;
    await writeFile(setupPath, JSON.stringify(setup));
    const refreshed = await get();
    assert.doesNotMatch(refreshed, /<svg /);
    assert.equal(await readFile(log, "utf8"), before);
  } finally {
    child.kill("SIGTERM");
    await new Promise((resolve) => child.once("exit", resolve));
  }
});

test('a successful CLI transition refreshes an opted-in snapshot without a separate regeneration command', async () => {
  const value = await fixture();
  const setupPath = path.join(value.root, '.agent-team/setup.json');
  const setup = JSON.parse(await readFile(setupPath, 'utf8'));
  setup.dashboard = { snapshot: true };
  await writeFile(setupPath, JSON.stringify(setup));
  const canonical = await loadCanonicalState(value.project);
  const request = await requestFile(value, 'snapshot-pause', envelope('owner-session', 0, {
    operationId: 'snapshot-pause', taskId: 'AT-001', action: 'pause',
    expectedFingerprint: canonical.tracker.fingerprint, expectedOwner: 'TEAM-001',
  }));
  const result = await invoke('task-transition', '--project', value.feature, '--request', request);
  assert.equal(result.status, 'applied');
  assert.equal(result.dashboard.status, 'published');
  const html = await readFile(path.join(value.root, '.agent-team/dashboard/index.html'), 'utf8');
  assert.match(html, /"status":"paused"/);
  assert.equal((await invoke('task-transition', '--project', value.feature, '--request', request)).dashboard.status, 'unchanged');
});

test("dashboard start reports its loopback URL and stops cleanly on SIGINT or SIGTERM", async (t) => {
  for (const signal of ["SIGINT", "SIGTERM"]) {
    await t.test(signal, async () => {
      const value = await fixture();
      const child = spawn(process.execPath, [cli, "dashboard-start", "--project", value.feature, "--port", "0"], { stdio: ["ignore", "pipe", "pipe"] });
      let stdout = "";
      let stderr = "";
      child.stdout.on("data", (chunk) => { stdout += chunk; });
      child.stderr.on("data", (chunk) => { stderr += chunk; });
      const listening = await new Promise((resolve, reject) => {
        const timeout = setTimeout(() => reject(new Error(`dashboard did not start: ${stderr}`)), 3000);
        child.stdout.on("data", () => {
          const line = stdout.split("\n").find(Boolean);
          if (line) { clearTimeout(timeout); resolve(JSON.parse(line)); }
        });
      });
      assert.equal(listening.status, "listening");
      assert.match(listening.url, /^http:\/\/127\.0\.0\.1:\d+\/$/);
      const body = await new Promise((resolve, reject) => {
        http.get(listening.url, (response) => {
          let source = "";
          response.setEncoding("utf8");
          response.on("data", (chunk) => { source += chunk; });
          response.on("end", () => resolve(source));
        }).on("error", reject);
      });
      assert.match(body, /LOCAL LIVE STATUS/);
      child.kill(signal);
      const code = await new Promise((resolve) => child.once("exit", resolve));
      assert.equal(code, 0, stderr);
      const records = stdout.trim().split("\n").map(JSON.parse);
      assert.equal(records.at(-1).status, "stopped");
      assert.equal(records.at(-1).url, listening.url);
    });
  }
});

test("dashboard start binds the requested nonzero port and fails when that port is occupied", async () => {
  const value = await fixture();
  const reservation = http.createServer();
  await new Promise((resolve, reject) => reservation.once("error", reject).listen(0, "127.0.0.1", resolve));
  const port = reservation.address().port;
  await new Promise((resolve, reject) => reservation.close((error) => error ? reject(error) : resolve()));

  const child = spawn(process.execPath, [cli, "dashboard-start", "--project", value.feature, "--port", String(port)], { stdio: ["ignore", "pipe", "pipe"] });
  let stdout = "";
  child.stdout.on("data", (chunk) => { stdout += chunk; });
  const exited = new Promise((resolve) => child.once("exit", resolve));
  const listening = await new Promise((resolve, reject) => {
    const timeout = setTimeout(() => reject(new Error("dashboard did not bind requested port")), 3000);
    child.stdout.on("data", () => {
      const line = stdout.split("\n").find(Boolean);
      if (line) { clearTimeout(timeout); resolve(JSON.parse(line)); }
    });
  });
  try {
    assert.equal(listening.port, port);
  } finally {
    child.kill("SIGTERM");
    await exited;
  }

  const occupied = http.createServer();
  await new Promise((resolve, reject) => occupied.once("error", reject).listen(port, "127.0.0.1", resolve));
  try {
    const failure = await invokeFailure("dashboard-start", "--project", value.feature, "--port", String(port));
    assert.equal(failure.status, "failed");
    assert.match(failure.error, /EADDRINUSE|address already in use/i);
  } finally {
    await new Promise((resolve, reject) => occupied.close((error) => error ? reject(error) : resolve()));
  }
});
