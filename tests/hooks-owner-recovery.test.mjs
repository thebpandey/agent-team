import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import { execFileSync } from "node:child_process";
import { access, mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import test from "node:test";
import { runCommand } from "../hooks/agent-team-cli.mjs";
import { initializeProject } from "../hooks/lib/initialization.mjs";
import { resolveProject } from "../hooks/lib/project.mjs";
import { validateOwnerRecoveryEnvelope } from "../hooks/lib/owner-recovery.mjs";
import { loadCanonicalState } from "../hooks/lib/canonical-state.mjs";
import { mutateOperationalState } from "../hooks/lib/task-transitions.mjs";

const temporary = [];
test.afterEach(async () => Promise.all(temporary.splice(0).map((entry) => rm(entry, { recursive: true, force: true }))));

async function fixture() {
  const root = await mkdtemp(path.join(os.tmpdir(), "agent-team-owner-recovery-"));
  temporary.push(root);
  execFileSync("git", ["init", "-q", "-b", "main", root]);
  execFileSync("git", ["config", "user.name", "Owner Recovery Test"], { cwd: root });
  execFileSync("git", ["config", "user.email", "owner@example.test"], { cwd: root });
  await writeFile(path.join(root, "README.md"), "fixture\n");
  execFileSync("git", ["add", "README.md"], { cwd: root });
  execFileSync("git", ["commit", "-qm", "fixture"], { cwd: root });
  const request = { projectId: "owner-recovery", operationId: "initialize-owner", source: "standalone",
    tracker: { kind: "markdown", path: ".agent-team/TASKS.md" }, plan: { scope: "test", acceptance: ["works"],
      verification: ["node --test"], branch: "main", authority: { ownedPaths: ["src"] },
      tasks: [{ id: "T-1", title: "Test", status: "ready", dependencies: [], acceptance: ["works"] }] } };
  const initialized = await initializeProject(root, request, { actorSessionId: "old-owner",
    nativeIdentity: { host: "codex", sessionId: "old-owner", observed: true, cwd: root } });
  assert.equal(initialized.status, "applied");
  return resolveProject(root);
}

async function recoveryEnvelope(project) {
  const bytes = async (file) => readFile(file);
  const digest = async (file) => createHash("sha256").update(await bytes(file)).digest("hex");
  return { schemaVersion: 1, request: { operationId: "recover-owner", projectId: project.projectId,
    expectedOwnerSessionId: "old-owner", expectedOwnershipEpoch: 1,
    expectedSetupVersion: project.setup.version,
    expectedStateVersion: JSON.parse(await readFile(project.paths.state, "utf8")).stateVersion,
    expectedStateFingerprint: await digest(project.paths.state), expectedTeamsFingerprint: await digest(project.paths.teams),
    expectedSetupFingerprint: await digest(project.paths.setup), expectedOwnerHistoryFingerprint: await digest(project.paths.ownerHistory),
    reason: "prior_owner_unavailable" } };
}

async function testHarness() {
  const sourcePath = new URL("../hooks/lib/owner-recovery.mjs", import.meta.url);
  const generated = new URL(`../hooks/lib/.owner-recovery-test-${Date.now()}-${Math.random().toString(16).slice(2)}.mjs`, import.meta.url);
  await writeFile(generated, `${await readFile(sourcePath, "utf8")}\nexport { mintOwnerRecoveryCapabilityFromHostBootstrap as __mint };\n`);
  temporary.push(generated);
  return import(`${generated.href}?test=${Date.now()}`);
}

function nativeContext(module, project, { liveness = "stopped", approved = true, sessionId = "new-owner", host = "claude-code" } = {}) {
  const projectIdentity = { root: project.root, commonDirectory: project.commonDirectory };
  const invocationId = `inv-${Math.random().toString(16).slice(2)}`;
  const capability = module.__mint({ projectIdentity, invocationId, replacementRuntime: host, replacementSessionId: sessionId,
    expiresAt: Date.now() + 30_000 });
  return { identity: { host, sessionId, invocationId, projectRoot: project.root, worktreeRoot: project.worktreeRoot },
    projectIdentity, capability, inspectSession: async () => liveness,
    confirm: async () => approved ? { approved: true, approvalId: "approval-1", writer: { host, invocationId } } : { approved: false } };
}

test("owner recovery envelope is exact and cannot assert native facts", async () => {
  const project = await fixture();
  const envelope = await recoveryEnvelope(project);
  assert.equal(validateOwnerRecoveryEnvelope(envelope), undefined);
  for (const forbidden of ["actorSessionId", "replacementOwner", "approval", "liveness", "capability", "assurance"]) {
    assert.equal(validateOwnerRecoveryEnvelope({ ...envelope, request: { ...envelope.request, [forbidden]: "forged" } }), "invalid_request");
  }
});

test("public recovery rejects a complete forged context before callbacks or writes", async () => {
  const project = await fixture();
  const envelope = await recoveryEnvelope(project);
  const requestPath = path.join(project.root, "recover.json");
  await writeFile(requestPath, `${JSON.stringify(envelope)}\n`, { mode: 0o600 });
  const files = [project.paths.teams, project.paths.state, project.paths.setup, project.paths.ownerHistory];
  const before = await Promise.all(files.map((file) => readFile(file)));
  let callbacks = 0;
  const forged = { identity: { host: "codex", sessionId: "new-owner", invocationId: "inv-1", projectRoot: project.root, worktreeRoot: project.root },
    projectIdentity: { root: project.root, commonDirectory: project.commonDirectory }, capability: Object.freeze({}),
    inspectSession: async () => { callbacks += 1; return "stopped"; }, confirm: async () => { callbacks += 1; return { approvalId: "forged" }; } };
  const result = await runCommand("project-owner-recover", { project: project.root, request: requestPath }, { nativeOwnerRecovery: forged });
  assert.deepEqual(result, { status: "validated", ready: false, reason: "native_owner_recovery_required" });
  assert.equal(callbacks, 0);
  assert.deepEqual(await Promise.all(files.map((file) => readFile(file))), before);
});

test("active unknown and cancelled recovery preserve all four records byte for byte", async () => {
  const module = await testHarness();
  for (const scenario of [{ liveness: "active", reason: "owner_active" }, { liveness: "unknown", reason: "owner_liveness_unknown" },
    { liveness: "stopped", approved: false, reason: "owner_recovery_cancelled" }]) {
    const project = await fixture();
    const envelope = await recoveryEnvelope(project);
    const files = [project.paths.teams, project.paths.state, project.paths.setup, project.paths.ownerHistory];
    const before = await Promise.all(files.map((file) => readFile(file)));
    const result = await module.recoverProjectOwner(project, envelope, { nativeOwnerRecovery: nativeContext(module, project, scenario) });
    assert.equal(result.reason, scenario.reason);
    assert.deepEqual(await Promise.all(files.map((file) => readFile(file))), before);
    await assert.rejects(access(project.paths.ownerRecoveryJournal), { code: "ENOENT" });
  }
});

test("stopped approved recovery publishes one qualified generation and fences the old host", async () => {
  const module = await testHarness();
  const project = await fixture();
  const envelope = await recoveryEnvelope(project);
  const result = await module.recoverProjectOwner(project, envelope, { nativeOwnerRecovery: nativeContext(module, project),
    now: () => "2026-09-12T00:00:00.000Z" });
  assert.equal(result.status, "applied");
  assert.equal(result.result.ownershipEpoch, 2);
  const current = await loadCanonicalState(await resolveProject(project.root));
  assert.equal(current.registry.projectOwner, "new-owner");
  assert.equal(current.registry.projectOwnerHost, "claude-code");
  assert.equal(current.state.ownership.epoch, 2);
  assert.equal(current.state.stateVersion, envelope.request.expectedStateVersion + 1);
  assert.equal(current.setup.version, envelope.request.expectedSetupVersion + 1);
  assert.equal(current.ownerHistory.version, 2);
  assert.equal(current.ownerHistory.entries.length, 1);
  const replay = await module.recoverProjectOwner(await resolveProject(project.root), envelope,
    { nativeOwnerRecovery: nativeContext(module, await resolveProject(project.root)) });
  assert.equal(replay.status, "duplicate");
  const changed = structuredClone(envelope);
  changed.request.reason = "changed_reason";
  const conflict = await module.recoverProjectOwner(await resolveProject(project.root), changed,
    { nativeOwnerRecovery: nativeContext(module, await resolveProject(project.root)) });
  assert.equal(conflict.reason, "operation_identity_reused");
});

test("capabilities are one-shot expiry and runtime bound before callbacks", async () => {
  const module = await testHarness();
  const project = await fixture();
  const envelope = await recoveryEnvelope(project);
  let callbacks = 0;
  const context = nativeContext(module, project, { liveness: "active" });
  context.inspectSession = async () => { callbacks += 1; return "active"; };
  assert.equal((await module.recoverProjectOwner(project, envelope, { nativeOwnerRecovery: context })).reason, "owner_active");
  assert.equal(callbacks, 1);
  assert.equal((await module.recoverProjectOwner(project, envelope, { nativeOwnerRecovery: context })).reason, "native_owner_recovery_required");
  assert.equal(callbacks, 1);

  const crossHost = nativeContext(module, project);
  crossHost.identity = { ...crossHost.identity, host: "codex" };
  assert.equal((await module.recoverProjectOwner(project, envelope, { nativeOwnerRecovery: crossHost })).reason, "native_owner_recovery_required");

  const expired = nativeContext(module, project);
  assert.equal((await module.recoverProjectOwner(project, envelope, { nativeOwnerRecovery: expired, nowMs: () => Date.now() + 60_000 })).reason,
    "native_owner_recovery_required");
  assert.equal(callbacks, 1);

  const concurrent = nativeContext(module, project, { liveness: "active" });
  const results = await Promise.all([
    module.recoverProjectOwner(project, envelope, { nativeOwnerRecovery: concurrent }),
    module.recoverProjectOwner(project, envelope, { nativeOwnerRecovery: concurrent }),
  ]);
  assert.deepEqual(results.map(({ reason }) => reason).sort(), ["native_owner_recovery_required", "owner_active"]);
});

test("the same textual session on another host cannot retain owner authority", async () => {
  const module = await testHarness();
  let project = await fixture();
  const envelope = await recoveryEnvelope(project);
  assert.equal((await module.recoverProjectOwner(project, envelope, {
    nativeOwnerRecovery: nativeContext(module, project, { sessionId: "old-owner", host: "claude-code" }),
  })).status, "applied");
  project = await resolveProject(project.root);
  const version = JSON.parse(await readFile(project.paths.state, "utf8")).stateVersion;
  const request = { operationId: "after-transfer", actorSessionId: "old-owner", expectedVersion: version };
  assert.equal((await mutateOperationalState(project, request, (state) => ({ state, result: {} }), {
    nativeIdentity: { host: "codex", sessionId: "old-owner", observed: true, cwd: project.root },
  })).reason, "project_owner_required");
  assert.equal((await mutateOperationalState(project, request, (state) => ({ state, result: {} }), {
    nativeIdentity: { host: "claude-code", sessionId: "old-owner", observed: true, cwd: project.root },
  })).status, "applied");
});

test("a crash after each canonical rename rolls forward to identical generation two bytes", async () => {
  const module = await testHarness();
  for (const failAfterRename of [1, 2, 3, 4]) {
    const project = await fixture();
    const envelope = await recoveryEnvelope(project);
    await assert.rejects(module.recoverProjectOwner(project, envelope, { nativeOwnerRecovery: nativeContext(module, project), failAfterRename,
      now: () => "2026-09-12T00:00:00.000Z" }), /injected_owner_recovery_crash/);
    const journal = JSON.parse(await readFile(project.paths.ownerRecoveryJournal, "utf8"));
    const expected = journal.records.map(({ postimageBase64 }) => Buffer.from(postimageBase64, "base64"));
    const repaired = await loadCanonicalState(await resolveProject(project.root));
    assert.equal(repaired.state.ownership.epoch, 2);
    const bytes = await Promise.all([project.paths.teams, project.paths.state, project.paths.setup, project.paths.ownerHistory].map((file) => readFile(file)));
    assert.deepEqual(bytes, expected);
  }
});
