import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import { mkdtemp, readFile, symlink, writeFile } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import test from "node:test";
import { runCommand } from "../hooks/agent-team-cli.mjs";
import { runNormalizedHook } from "../hooks/agent-team-hook.mjs";
import { loadCanonicalState } from "../hooks/lib/canonical-state.mjs";
import { classifyOperation } from "../hooks/lib/operation.mjs";
import * as ownerRecovery from "../hooks/lib/owner-recovery.mjs";
import { evaluatePolicy } from "../hooks/lib/policy.mjs";
import { resolveProject } from "../hooks/lib/project.mjs";
import { hookEvent, policyFixture } from "./hook-test-helpers.mjs";

const digest = async (file) => createHash("sha256").update(await readFile(file)).digest("hex");
const stable = (value) => JSON.stringify(value && typeof value === "object"
  ? Array.isArray(value) ? value.map((entry) => JSON.parse(stable(entry)))
    : Object.fromEntries(Object.keys(value).sort().map((key) => [key, JSON.parse(stable(value[key]))])) : value);

async function transferFixture() {
  const root = await mkdtemp(path.join(os.tmpdir(), "agent-team-continuity-transfer-"));
  const value = await policyFixture(root, { qualifiedOwnership: true });
  const project = await resolveProject(root);
  const state = JSON.parse(await readFile(project.paths.state, "utf8"));
  const setup = JSON.parse(await readFile(project.paths.setup, "utf8"));
  state.stateVersion = 7;
  state.pendingOperations = { integration: { operationId: "pending-integration", status: "unknown" } };
  state.integration.recordedEvidence = { path: "/evidence/integration.json", fingerprint: "b".repeat(64) };
  setup.version = 3;
  await writeFile(project.paths.state, `${JSON.stringify(state, null, 2)}\n`);
  await writeFile(project.paths.setup, `${JSON.stringify(setup, null, 2)}\n`);
  return { value, project };
}

async function transferEnvelope(project, {
  operationId = "transfer-coordinator-1",
  oldOwner = { host: "codex", sessionId: "owner-session" },
  newOwner = { host: "codex", sessionId: "01a0a0d3-206e-7010-bbef-41c369bdd10d" },
  grantedAt = "2026-09-14T12:00:00.000Z",
} = {}) {
  const canonical = await loadCanonicalState(project, { includeDeliveryEvidence: false });
  return { schemaVersion: 1, request: {
    operationId,
    projectId: project.projectId,
    expectedProjectRoot: project.root,
    expectedOwner: oldOwner,
    newOwner,
    expectedOwnershipEpoch: canonical.state.ownership.epoch,
    expectedSetupVersion: canonical.setup.version,
    expectedStateVersion: canonical.state.stateVersion,
    expectedStateFingerprint: await digest(project.paths.state),
    expectedTeamsFingerprint: await digest(project.paths.teams),
    expectedSetupFingerprint: await digest(project.paths.setup),
    expectedOwnerHistoryFingerprint: await digest(project.paths.ownerHistory),
    expectedTracker: { kind: canonical.tracker.kind, path: canonical.tracker.path, fingerprint: canonical.tracker.fingerprint },
    authorization: {
      kind: "user-directed-maintenance",
      status: "approved",
      scope: "coordinator_continuity_transfer",
      source: "explicit user instruction in the active project session",
      approvalId: `approval-${operationId}`,
      grantedAt,
      projectId: project.projectId,
      oldOwner,
      newOwner,
    },
    reason: "Remove historical coordinator session locking without asserting prior-session liveness",
  } };
}

test("fresh canonical sessions may edit unclaimed files without crossing registered ownership", async () => {
  // This catches restoring a historical owner UUID as a permanent ordinary-write lock.
  const root = await mkdtemp(path.join(os.tmpdir(), "agent-team-session-continuity-"));
  const value = await policyFixture(root, { qualifiedOwnership: true });
  const project = await resolveProject(root);
  const event = (changedPath) => hookEvent(value, {
    cwd: root,
    sessionId: "fresh-coordinator-session",
    operation: { kind: "file_change", files: [{ action: "edit", path: changedPath, changedContent: "updated\n" }] },
  });

  const documentation = await evaluatePolicy(event("README.md"), project);
  const claimed = await evaluatePolicy(event("src/owned.js"), project);
  const tracker = await evaluatePolicy(event(".agent-team/TASKS.md"), project);
  const stateAlias = await evaluatePolicy(event("./.agent-team/state.json"), project);
  const absoluteEvidence = await evaluatePolicy(event(path.join(project.paths.stateRoot, "evidence", "forged.json")), project);

  assert.equal(documentation.allow, true);
  for (const result of [claimed, tracker, stateAlias, absoluteEvidence]) {
    assert.equal(result.allow, false);
    assert.equal(result.mode, "enforce");
  }
});

test("fresh-session continuity cannot write canonical Git metadata through paths or recognized shell tools", async () => {
  const root = await mkdtemp(path.join(os.tmpdir(), "agent-team-session-git-metadata-"));
  const value = await policyFixture(root, { qualifiedOwnership: true });
  const project = await resolveProject(root);
  await symlink(project.commonDirectory, path.join(root, "git-metadata-alias"), "dir");
  const base = { runtime: "codex", event: "PreToolUse", cwd: root, sessionId: "fresh-coordinator-session", eventId: "git-metadata-event" };
  const operations = [
    { kind: "file_change", files: [{ action: "edit", path: ".git/config", changedContent: "forged\n" }] },
    { kind: "file_change", files: [{ action: "delete", path: ".git", changedContent: "" }] },
    { kind: "file_change", files: [{ action: "edit", path: path.join(project.commonDirectory, "refs/heads/main"), changedContent: "forged\n" }] },
    { kind: "file_change", files: [{ action: "edit", path: "git-metadata-alias/config", changedContent: "forged\n" }] },
    { kind: "shell", command: "rm -f .git/config" },
    { kind: "shell", command: "mv README.md .git/config" },
    { kind: "shell", command: "cp README.md .git/config" },
  ];
  for (const operation of operations) {
    const result = await evaluatePolicy({ ...base, operation }, project);
    assert.equal(result.allow, false, operation.command ?? operation.files[0].path);
    assert.equal(result.mode, "enforce");
  }
});

test("user-directed continuity transfer preserves work and provenance while invalidating old gates", async () => {
  // This catches transfer code that rewrites evidence, drops claims, or carries old consequential authority forward.
  assert.equal(typeof ownerRecovery.transferProjectCoordinator, "function");
  const { project } = await transferFixture();
  const envelope = await transferEnvelope(project);
  const before = await loadCanonicalState(project, { includeDeliveryEvidence: false });
  const preserved = {
    teams: structuredClone(before.registry.teams),
    tasks: structuredClone(before.tasks),
    pendingOperations: structuredClone(before.state.pendingOperations),
    integrationAuthorization: structuredClone(before.state.integration.authorization),
    integrationEvidence: structuredClone(before.state.integration.recordedEvidence),
    releaseAuthorization: structuredClone(before.state.release.authorization),
    completion: structuredClone(before.state.completion),
  };

  const result = await ownerRecovery.transferProjectCoordinator(project, envelope, {
    now: () => "2026-09-14T12:01:00.000Z",
  });
  const after = await loadCanonicalState(project, { includeDeliveryEvidence: false });

  assert.equal(result.status, "applied");
  assert.deepEqual({
    owner: after.registry.projectOwner,
    host: after.registry.projectOwnerHost,
    epoch: after.registry.ownershipEpoch,
  }, {
    owner: "01a0a0d3-206e-7010-bbef-41c369bdd10d",
    host: "codex",
    epoch: 2,
  });
  assert.deepEqual(after.registry.teams, preserved.teams);
  assert.deepEqual(after.tasks, preserved.tasks);
  assert.deepEqual(after.state.pendingOperations, preserved.pendingOperations);
  assert.deepEqual(after.state.integration.authorization, preserved.integrationAuthorization);
  assert.deepEqual(after.state.integration.recordedEvidence, preserved.integrationEvidence);
  assert.deepEqual(after.state.release.authorization, preserved.releaseAuthorization);
  assert.deepEqual(after.state.completion, preserved.completion);
  for (const gate of [after.state.integration, after.state.release]) {
    assert.equal(gate.authorized, false);
    assert.equal(gate.hold, true);
    assert.equal(gate.ownerSessionId, "01a0a0d3-206e-7010-bbef-41c369bdd10d");
    assert.equal(gate.ownerHost, "codex");
    assert.equal(gate.ownershipEpoch, 2);
  }
  assert.equal(after.ownerHistory.entries.length, 1);
  const entry = after.ownerHistory.entries[0];
  assert.equal(entry.kind, "coordinator_continuity");
  assert.equal(entry.livenessEvidence.status, "not_asserted");
  assert.deepEqual(entry.authorization, envelope.request.authorization);
  assert.deepEqual(entry.oldOwner, envelope.request.expectedOwner);
  assert.deepEqual(entry.newOwner, envelope.request.newOwner);
  assert.equal(entry.writer.kind, "user-directed-maintenance");
});

test("exact CLI maintenance route works without claiming native identity", async () => {
  // This catches making the explicitly authorized repair depend on a disabled PreToolUse identity route.
  const { project } = await transferFixture();
  const envelope = await transferEnvelope(project, { operationId: "transfer-through-cli" });
  const requestPath = path.join(project.root, "coordinator-transfer.json");
  await writeFile(requestPath, `${JSON.stringify(envelope, null, 2)}\n`);
  const cli = path.resolve(import.meta.dirname, "../hooks/agent-team-cli.mjs");
  const command = `node ${cli} coordinator-continuity-transfer --project ${project.root} --request ${requestPath}`;

  assert.deepEqual(classifyOperation({ operation: { kind: "shell", command } }), {
    kind: "agent_team_native_command",
    command: "coordinator-continuity-transfer",
    cliPath: cli,
    project: project.root,
    request: requestPath,
    valid: true,
  });
  const applied = await runCommand("coordinator-continuity-transfer", { project: project.root, request: requestPath });
  const duplicate = await runCommand("coordinator-continuity-transfer", { project: project.root, request: requestPath });
  assert.equal(applied.status, "applied");
  assert.equal(duplicate.status, "duplicate");
  assert.equal((await loadCanonicalState(project, { includeDeliveryEvidence: false })).registry.projectOwner,
    envelope.request.newOwner.sessionId);

  for (const changed of [
    `${command} ; true`,
    `node ${cli} coordinator-continuity-transfer --request ${requestPath} --project ${project.root}`,
  ]) assert.notEqual(classifyOperation({ operation: { kind: "shell", command: changed } }).kind, "agent_team_native_command");
});

test("enabled native hook and direct CLI replay one exact maintenance transfer", async () => {
  // This catches double application when the native hook performs the transaction before the shell command runs.
  const { project } = await transferFixture();
  const envelope = await transferEnvelope(project, { operationId: "transfer-through-hook" });
  const requestPath = path.join(project.root, "hook-coordinator-transfer.json");
  await writeFile(requestPath, `${JSON.stringify(envelope, null, 2)}\n`);
  const cli = path.resolve(import.meta.dirname, "../hooks/agent-team-cli.mjs");
  const command = `node ${cli} coordinator-continuity-transfer --project ${project.root} --request ${requestPath}`;

  const hooked = await runNormalizedHook({ runtime: "codex", event: "PreToolUse", cwd: project.root,
    sessionId: "fresh-coordinator-session", eventId: "continuity-event", operation: { kind: "shell", command } });
  assert.equal(hooked.decision.allow, true);
  assert.deepEqual(hooked.decision.mutations.find(({ kind }) => kind === "coordinator_continuity_transfer"), {
    kind: "coordinator_continuity_transfer",
    command: "coordinator-continuity-transfer",
    status: "applied",
  });
  assert.equal((await runCommand("coordinator-continuity-transfer", { project: project.root, request: requestPath })).status, "duplicate");
});

test("successive continuity transfers append history without rewriting its prefix", async () => {
  // This catches treating continuity as owner replacement rather than append-only provenance.
  const { project } = await transferFixture();
  const first = await transferEnvelope(project, { operationId: "continuity-history-1" });
  assert.equal((await ownerRecovery.transferProjectCoordinator(project, first, { now: () => "2026-09-14T12:01:00.000Z" })).status, "applied");
  const prefix = structuredClone((await loadCanonicalState(project, { includeDeliveryEvidence: false })).ownerHistory.entries);
  const second = await transferEnvelope(project, {
    operationId: "continuity-history-2",
    oldOwner: first.request.newOwner,
    newOwner: { host: "claude-code", sessionId: "future-authorized-session" },
    grantedAt: "2026-09-15T12:00:00.000Z",
  });

  assert.equal((await ownerRecovery.transferProjectCoordinator(project, second, { now: () => "2026-09-15T12:01:00.000Z" })).status, "applied");
  const after = await loadCanonicalState(project, { includeDeliveryEvidence: false });
  assert.deepEqual(after.ownerHistory.entries.slice(0, prefix.length), prefix);
  assert.equal(after.ownerHistory.entries[1].livenessEvidence.status, "not_asserted");
  assert.equal(after.registry.projectOwner, "future-authorized-session");
  assert.equal(after.registry.ownershipEpoch, 3);
});

test("duplicate continuity replay validates canonical history before returning its receipt", async () => {
  const { project } = await transferFixture();
  const envelope = await transferEnvelope(project, { operationId: "continuity-forged-replay" });
  const history = JSON.parse(await readFile(project.paths.ownerHistory, "utf8"));
  history.entries.push({ operationId: envelope.request.operationId, signature: ownerRecovery.sha256(stable(envelope.request)), receipt: { forged: true } });
  await writeFile(project.paths.ownerHistory, `${JSON.stringify(history, null, 2)}\n`);
  const files = [project.paths.teams, project.paths.state, project.paths.setup, project.paths.ownerHistory];
  const before = await Promise.all(files.map((file) => readFile(file)));

  const result = await ownerRecovery.transferProjectCoordinator(project, envelope);

  assert.equal(result.status, "conflict");
  assert.notEqual(result.reason, undefined);
  assert.deepEqual(await Promise.all(files.map((file) => readFile(file))), before);
  await assert.rejects(readFile(project.paths.ownerRecoveryJournal), { code: "ENOENT" });
});

test("continuity journal rolls forward after an interrupted record replacement", async () => {
  // This catches exposing a mixed owner generation after a crash between canonical record renames.
  const { project } = await transferFixture();
  const envelope = await transferEnvelope(project, { operationId: "continuity-crash" });
  await assert.rejects(ownerRecovery.transferProjectCoordinator(project, envelope, {
    now: () => "2026-09-14T12:01:00.000Z",
    failAfterRename: 2,
  }), /injected_owner_recovery_crash_2/);

  const repaired = await loadCanonicalState(project, { includeDeliveryEvidence: false });
  assert.equal(repaired.registry.projectOwner, envelope.request.newOwner.sessionId);
  assert.equal(repaired.ownerHistory.entries[0].livenessEvidence.status, "not_asserted");
  await assert.rejects(readFile(project.paths.ownerRecoveryJournal), { code: "ENOENT" });
});

test("malformed owner state and maintenance authority fail without transaction writes", async () => {
  // This catches exact fingerprints being mistaken for proof that malformed canonical records are safe to rotate.
  const { project } = await transferFixture();
  const malformedAuthority = await transferEnvelope(project, { operationId: "malformed-authority" });
  malformedAuthority.request.authorization.kind = "host-bootstrap";
  assert.equal(ownerRecovery.validateCoordinatorContinuityEnvelope(malformedAuthority), "invalid_request");
  const assertedNative = await transferEnvelope(project, { operationId: "asserted-native" });
  assertedNative.request.nativeIdentity = { observed: true, sessionId: assertedNative.request.newOwner.sessionId };
  assert.equal(ownerRecovery.validateCoordinatorContinuityEnvelope(assertedNative), "invalid_request");

  const envelope = await transferEnvelope(project, { operationId: "malformed-state" });
  const state = JSON.parse(await readFile(project.paths.state, "utf8"));
  state.integration = null;
  await writeFile(project.paths.state, `${JSON.stringify(state, null, 2)}\n`);
  envelope.request.expectedStateFingerprint = await digest(project.paths.state);
  const files = [project.paths.teams, project.paths.state, project.paths.setup, project.paths.ownerHistory];
  const before = await Promise.all(files.map((file) => readFile(file)));
  const result = await ownerRecovery.transferProjectCoordinator(project, envelope);
  const after = await Promise.all(files.map((file) => readFile(file)));

  assert.deepEqual(result, { status: "conflict", reason: "owner_records_invalid" });
  assert.deepEqual(after, before);
  await assert.rejects(readFile(project.paths.ownerRecoveryJournal), { code: "ENOENT" });
});

test("ordinary continuity never satisfies consequential owner gates", async () => {
  // This catches widening unclaimed-file continuity into integration, release, database, or completion authority.
  const root = await mkdtemp(path.join(os.tmpdir(), "agent-team-continuity-gates-"));
  const value = await policyFixture(root, { qualifiedOwnership: true });
  const project = await resolveProject(root);
  const base = { runtime: "codex", event: "PreToolUse", cwd: root, sessionId: "fresh-coordinator-session", eventId: "gate-event" };
  const operations = [
    { kind: "shell", command: `git push origin HEAD:feature` },
    { kind: "shell", command: "npm publish" },
    { kind: "provider", tool: "mcp__database__execute", input: { query: "TRUNCATE TABLE widgets" } },
    { kind: "completion", taskId: "AT-001", outcome: "verified" },
  ];
  for (const operation of operations) {
    const result = await evaluatePolicy({ ...base, operation }, project);
    assert.equal(result.allow, false, operation.kind === "shell" ? operation.command : operation.kind);
    assert.equal(result.mode, "enforce");
  }
});
