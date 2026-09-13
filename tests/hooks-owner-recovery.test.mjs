import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import { execFileSync } from "node:child_process";
import { access, mkdir, mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import test from "node:test";
import { runCommand } from "../hooks/agent-team-cli.mjs";
import { initializeProject } from "../hooks/lib/initialization.mjs";
import { resolveProject } from "../hooks/lib/project.mjs";
import { validateOwnerRecoveryEnvelope } from "../hooks/lib/owner-recovery.mjs";
import { repairOwnerRecovery } from "../hooks/lib/owner-recovery.mjs";
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
  const state = JSON.parse(await readFile(project.paths.state, "utf8"));
  return { schemaVersion: 1, request: { operationId: "recover-owner", projectId: project.projectId,
    expectedOwnerSessionId: state.ownership?.current?.sessionId ?? "old-owner", expectedOwnershipEpoch: state.ownership?.epoch ?? 1,
    expectedSetupVersion: project.setup.version,
    expectedStateVersion: state.stateVersion,
    expectedStateFingerprint: await digest(project.paths.state), expectedTeamsFingerprint: await digest(project.paths.teams),
    expectedSetupFingerprint: await digest(project.paths.setup), expectedOwnerHistoryFingerprint: await digest(project.paths.ownerHistory),
    reason: "prior_owner_unavailable" } };
}

const fourPaths = (project) => [project.paths.teams, project.paths.state, project.paths.setup, project.paths.ownerHistory];
const rawFour = (project) => Promise.all(fourPaths(project).map((file) => readFile(file)));
async function assertNoRecoveryJournal(project) {
  await assert.rejects(access(project.paths.ownerRecoveryJournal), { code: "ENOENT" });
}
function rewritePostimage(journal, name, mutate) {
  const record = journal.records.find((candidate) => candidate.name === name);
  const value = JSON.parse(Buffer.from(record.postimageBase64, "base64"));
  mutate(value);
  replacePostimage(journal, name, value);
}
function replacePostimage(journal, name, value) {
  const record = journal.records.find((candidate) => candidate.name === name);
  const bytes = Buffer.from(`${JSON.stringify(value, null, 2)}\n`);
  record.postimageBase64 = bytes.toString("base64");
  record.postSha256 = createHash("sha256").update(bytes).digest("hex");
}
const stableJson = (value) => JSON.stringify(value && typeof value === "object"
  ? Array.isArray(value) ? value.map((entry) => JSON.parse(stableJson(entry)))
    : Object.fromEntries(Object.keys(value).sort().map((key) => [key, JSON.parse(stableJson(value[key]))])) : value);
async function independentPostimages(project, envelope, { invocationId, now = "2026-09-12T00:00:00.000Z", host = "claude-code", sessionId = "new-owner" }) {
  const [teamsBytes, stateBytes, setupBytes, historyBytes] = await rawFour(project);
  const state = JSON.parse(stateBytes); const setup = JSON.parse(setupBytes); const history = JSON.parse(historyBytes);
  const oldOwner = { host: state.ownership.current.host, sessionId: state.ownership.current.sessionId };
  const newOwner = { host, sessionId }; const ownershipEpoch = state.ownership.epoch + 1;
  const writer = { kind: "host-bootstrap", host, invocationId, approvalId: "approval-1" };
  const receipt = { operationId: envelope.request.operationId, ownershipEpoch, oldOwner, newOwner, appliedAt: now };
  const signature = createHash("sha256").update(stableJson(envelope.request)).digest("hex");
  const ownership = { epoch: ownershipEpoch, current: { ...newOwner, since: now, operationId: envelope.request.operationId, writer } };
  const nextState = structuredClone(state);
  nextState.stateVersion += 1; nextState.ownership = ownership;
  for (const gate of [nextState.integration, nextState.release]) Object.assign(gate, { ownerSessionId: sessionId, ownerHost: host, ownershipEpoch });
  const nextSetup = { ...setup, version: setup.version + 1, ownership };
  const entry = { operationId: envelope.request.operationId, signature, receipt, epoch: ownershipEpoch - 1, oldOwner, newOwner,
    reason: envelope.request.reason, approvalId: "approval-1", writer, livenessEvidence: { status: "stopped", observedAt: now },
    priorStateFingerprint: envelope.request.expectedStateFingerprint, priorTeamsFingerprint: envelope.request.expectedTeamsFingerprint,
    priorSetupFingerprint: envelope.request.expectedSetupFingerprint, priorOwnerHistoryFingerprint: envelope.request.expectedOwnerHistoryFingerprint,
    appliedAt: now };
  const nextHistory = { schemaVersion: 1, version: history.version + 1, ownership: { epoch: ownershipEpoch }, entries: [...history.entries, entry] };
  let teams = teamsBytes.toString("utf8");
  for (const [label, value] of [["Project owner", sessionId], ["Project owner host", host], ["Integration owner", sessionId], ["Integration owner host", host]]) {
    teams = teams.replace(new RegExp(`^${label}:.*$`, "m"), `${label}: ${value}`);
  }
  const json = (value) => Buffer.from(`${JSON.stringify(value, null, 2)}\n`);
  return [Buffer.from(teams), json(nextState), json(nextSetup), json(nextHistory)];
}

async function testHarness() {
  const sourcePath = new URL("../hooks/lib/owner-recovery.mjs", import.meta.url);
  const generated = new URL(`../hooks/lib/.owner-recovery-test-${Date.now()}-${Math.random().toString(16).slice(2)}.mjs`, import.meta.url);
  await writeFile(generated, `${await readFile(sourcePath, "utf8")}\nexport { mintOwnerRecoveryCapabilityFromHostBootstrap as __mint, setOwnerRecoveryTestObserver as __observeIo };\n`);
  temporary.push(generated);
  return import(`${generated.href}?test=${Date.now()}`);
}

async function nativeContext(module, project, { liveness = "stopped", approved = true, sessionId = "new-owner", host = "claude-code",
  invocationId = `inv-${Math.random().toString(16).slice(2)}`, cwd = project.root, expectedOwnershipEpoch = 1 } = {}) {
  const capability = await module.__mint({ projectRoot: project.root, nativeCwd: cwd, invocationId, replacementRuntime: host,
    replacementSessionId: sessionId, expectedOwnershipEpoch, expiresAt: Date.now() + 30_000 });
  const stopped = { status: "stopped", observedAt: "2026-09-12T00:00:00.000Z" };
  return { identity: { host, sessionId, invocationId, projectRoot: project.root, worktreeRoot: project.root, cwd, ownershipEpoch: expectedOwnershipEpoch },
    capability, inspectSession: async () => typeof liveness === "string" ? { ...(liveness === "stopped" ? stopped : {}), status: liveness } : liveness,
    confirm: async () => approved ? { approved: true, approvalId: "approval-1" } : { approved: false } };
}

async function legacyFixture() {
  const project = await fixture();
  const state = JSON.parse(await readFile(project.paths.state, "utf8"));
  const setup = JSON.parse(await readFile(project.paths.setup, "utf8"));
  delete state.ownership;
  delete setup.ownership;
  state.integration.ownerSessionId = "root";
  state.release.ownerSessionId = "root";
  for (const gate of [state.integration, state.release]) {
    delete gate.ownerHost;
    delete gate.ownershipEpoch;
  }
  let teams = await readFile(project.paths.teams, "utf8");
  teams = teams.replace(/^Project owner:.*$/m, "Project owner: root")
    .replace(/^Integration owner:.*$/m, "Integration owner: root")
    .replace(/^Project owner host:.*\n/m, "")
    .replace(/^Integration owner host:.*\n/m, "");
  await writeFile(project.paths.state, `${JSON.stringify(state, null, 2)}\n`);
  await writeFile(project.paths.setup, `${JSON.stringify(setup, null, 2)}\n`);
  await writeFile(project.paths.teams, teams);
  await rm(project.paths.ownerHistory);
  return resolveProject(project.root);
}

async function adoptionEnvelope(project, overrides = {}) {
  const digest = async (file) => createHash("sha256").update(await readFile(file)).digest("hex");
  const state = JSON.parse(await readFile(project.paths.state, "utf8"));
  const revision = execFileSync("git", ["rev-parse", "HEAD"], { cwd: project.root, encoding: "utf8" }).trim();
  const tracker = await import("../hooks/lib/tracker.mjs").then(({ readTracker }) => readTracker(project));
  const setup = JSON.parse(await readFile(project.paths.setup, "utf8"));
  const request = {
    operationId: "adopt-legacy-root", projectId: project.projectId, expectedLegacyOwnerSessionId: "root",
    expectedProjectRoot: project.root, expectedRevision: revision,
    expectedTracker: { kind: tracker.tracker.kind, path: tracker.tracker.path, fingerprint: tracker.tracker.fingerprint },
    expectedSetupVersion: setup.version, expectedStateVersion: state.stateVersion,
    expectedTeamsFingerprint: await digest(project.paths.teams), expectedSetupFingerprint: await digest(project.paths.setup),
    expectedStateFingerprint: await digest(project.paths.state), expectedOwnerHistoryFingerprint: null,
    authorization: { status: "approved", scope: "legacy_owner_adoption", source: "user-approved-migration",
      approvalId: "approval-legacy-1", grantedAt: "2026-09-12T00:00:00.000Z", projectId: project.projectId,
      revision, trackerFingerprint: tracker.tracker.fingerprint, host: "codex" },
    reason: "migrate historical synthetic owner",
    ...overrides,
  };
  return { schemaVersion: 1, request };
}

test("legacy adoption envelope is closed and cannot supply actor writer or liveness", async () => {
  const module = await testHarness();
  const project = await legacyFixture();
  const envelope = await adoptionEnvelope(project);
  assert.equal(module.validateLegacyOwnerAdoptionEnvelope(envelope), undefined);
  for (const field of ["actorSessionId", "newOwner", "sessionId", "writer", "liveness", "capability"]) {
    assert.equal(module.validateLegacyOwnerAdoptionEnvelope({ ...envelope, request: { ...envelope.request, [field]: "forged" } }), "invalid_request");
  }
  assert.equal(module.validateLegacyOwnerAdoptionEnvelope({ ...envelope, request: { ...envelope.request,
    expectedLegacyOwnerSessionId: "somebody" } }), "invalid_request");
});

test("trusted native adoption creates epoch one, invalidates inherited gates, and replays exactly", async () => {
  const module = await testHarness();
  let project = await legacyFixture();
  const envelope = await adoptionEnvelope(project);
  const context = { nativeIdentity: { host: "codex", sessionId: "native-owner", observed: true, cwd: project.root,
    invocationId: "native-invocation-1" }, writer: await import("../hooks/lib/task-transitions.mjs").then(({ captureWriterIdentity }) => captureWriterIdentity()) };
  const before = JSON.parse(await readFile(project.paths.state, "utf8"));
  const applied = await module.adoptLegacyProjectOwner(project, envelope, context, { now: () => "2026-09-12T00:00:01.000Z" });
  assert.equal(applied.status, "applied");
  assert.equal(applied.result.ownershipEpoch, 1);
  project = await resolveProject(project.root);
  const canonical = await loadCanonicalState(project);
  assert.deepEqual({ owner: canonical.registry.projectOwner, host: canonical.registry.projectOwnerHost, epoch: canonical.registry.ownershipEpoch },
    { owner: "native-owner", host: "codex", epoch: 1 });
  assert.deepEqual(canonical.state.ownership, canonical.setup.ownership);
  assert.equal(canonical.ownerHistory.version, 1);
  assert.deepEqual(canonical.ownerHistory.entries, []);
  for (const gate of [canonical.state.integration, canonical.state.release]) {
    assert.equal(gate.ownerSessionId, "native-owner");
    assert.equal(gate.ownerHost, "codex");
    assert.equal(gate.ownershipEpoch, 1);
    assert.equal(gate.authorized, false);
    assert.equal(gate.hold, true);
  }
  assert.equal(canonical.state.stateVersion, before.stateVersion + 1);
  const receipt = JSON.parse(await readFile(path.join(project.paths.stateRoot, "legacy-owner-adoption.json"), "utf8"));
  assert.equal(receipt.authorization.approvalId, "approval-legacy-1");
  assert.equal(receipt.nativeEvidence.sessionId, "native-owner");
  assert.equal(receipt.prior.ownerHistoryFingerprint, null);
  assert.equal((await module.adoptLegacyProjectOwner(project, envelope, context)).status, "duplicate");
  const changed = structuredClone(envelope); changed.request.reason = "different";
  assert.equal((await module.adoptLegacyProjectOwner(project, changed, context)).reason, "operation_identity_reused");
  const second = await adoptionEnvelope(project, { operationId: "another-adoption" });
  assert.equal((await module.adoptLegacyProjectOwner(project, second, context)).reason, "legacy_owner_adoption_already_completed");
});

test("legacy adoption rejects dirty shape stale facts and untrusted invocation without writes", async () => {
  const module = await testHarness();
  for (const mutate of [
    async (project) => { const state = JSON.parse(await readFile(project.paths.state)); state.pendingOperations = { x: { phase: "uncertain" } }; await writeFile(project.paths.state, JSON.stringify(state)); },
    async (project) => writeFile(project.paths.ownerHistory, '{"schemaVersion":1,"version":1,"ownership":{"epoch":1},"entries":[]}\n'),
    async (project) => { const state = JSON.parse(await readFile(project.paths.state)); state.ownership = { epoch: 1 }; await writeFile(project.paths.state, JSON.stringify(state)); },
  ]) {
    const project = await legacyFixture();
    await mutate(project);
    const envelope = await adoptionEnvelope(await resolveProject(project.root));
    const before = await rawFour({ paths: { ...project.paths, ownerHistory: project.paths.ownerHistory } }).catch(() => []);
    const result = await module.adoptLegacyProjectOwner(await resolveProject(project.root), envelope,
      { nativeIdentity: { host: "codex", sessionId: "native-owner", observed: false, cwd: project.root, invocationId: "inv" }, writer: {} });
    assert.notEqual(result.status, "applied");
    if (before.length) assert.deepEqual(await rawFour(project), before);
  }
});

test("legacy adoption crash seams roll forward to one byte-identical epoch-one generation", async () => {
  const module = await testHarness();
  for (const seam of [1, 2, 3, 4, 5, "committed"]) {
    let project = await legacyFixture();
    const envelope = await adoptionEnvelope(project);
    const context = { nativeIdentity: { host: "codex", sessionId: "native-owner", observed: true, cwd: project.root,
      invocationId: "crash-invocation" } };
    await assert.rejects(module.adoptLegacyProjectOwner(project, envelope, context, {
      now: () => "2026-09-12T00:00:01.000Z", ...(seam === "committed" ? { failAfterCommitted: true } : { failAfterRename: seam }),
    }), /injected_legacy_owner_adoption_crash/);
    const journal = JSON.parse(await readFile(path.join(project.paths.stateRoot, ".legacy-owner-adoption.json"), "utf8"));
    const expected = journal.records.map((record) => Buffer.from(record.postimageBase64, "base64"));
    project = await resolveProject(project.root);
    const repaired = await module.adoptLegacyProjectOwner(project, envelope, context);
    assert.equal(repaired.status, "applied", String(seam));
    const bytes = await Promise.all([...fourPaths(project), path.join(project.paths.stateRoot, "legacy-owner-adoption.json")].map((file) => readFile(file)));
    assert.deepEqual(bytes, expected, String(seam));
    await assert.rejects(access(path.join(project.paths.stateRoot, ".legacy-owner-adoption.json")), { code: "ENOENT" });
  }
});

test("hash-consistent adoption journal cannot authorize gates or rewrite unrelated state", async () => {
  const module = await testHarness();
  const project = await legacyFixture();
  const envelope = await adoptionEnvelope(project);
  const context = { nativeIdentity: { host: "codex", sessionId: "native-owner", observed: true, cwd: project.root,
    invocationId: "tamper-invocation" } };
  await assert.rejects(module.adoptLegacyProjectOwner(project, envelope, context, {
    now: () => "2026-09-12T00:00:01.000Z", failAfterJournal: true,
  }), /injected_legacy_owner_adoption_crash/);
  const journalPath = path.join(project.paths.stateRoot, ".legacy-owner-adoption.json");
  const journal = JSON.parse(await readFile(journalPath, "utf8"));
  const stateRecord = journal.records.find(({ name }) => name === "state");
  const state = JSON.parse(Buffer.from(stateRecord.postimageBase64, "base64"));
  state.integration.authorized = true;
  state.integration.unrelatedInjected = "forged";
  const bytes = Buffer.from(`${JSON.stringify(state, null, 2)}\n`);
  stateRecord.postimageBase64 = bytes.toString("base64");
  stateRecord.postSha256 = createHash("sha256").update(bytes).digest("hex");
  await writeFile(journalPath, `${JSON.stringify(journal, null, 2)}\n`);
  const before = await Promise.all([project.paths.teams, project.paths.state, project.paths.setup].map((file) => readFile(file)));
  await assert.rejects(module.adoptLegacyProjectOwner(await resolveProject(project.root), envelope, context),
    /legacy_owner_adoption_manual_reconciliation_required/);
  assert.deepEqual(await Promise.all([project.paths.teams, project.paths.state, project.paths.setup].map((file) => readFile(file))), before);
});

test("canonical readers fail closed during a partial adoption generation", async () => {
  const module = await testHarness();
  const project = await legacyFixture();
  const envelope = await adoptionEnvelope(project);
  const context = { nativeIdentity: { host: "codex", sessionId: "native-owner", observed: true, cwd: project.root,
    invocationId: "reader-fence-invocation" } };
  await assert.rejects(module.adoptLegacyProjectOwner(project, envelope, context, { failAfterRename: 1 }),
    /injected_legacy_owner_adoption_crash/);
  await assert.rejects(loadCanonicalState(await resolveProject(project.root)), /legacy_owner_adoption_in_progress/);
});

test("legacy adoption requires a clean tracked main revision", async () => {
  const module = await testHarness();
  const project = await legacyFixture();
  const envelope = await adoptionEnvelope(project);
  await writeFile(path.join(project.root, "README.md"), "dirty tracked content\n");
  const result = await module.adoptLegacyProjectOwner(project, envelope, { nativeIdentity: {
    host: "codex", sessionId: "native-owner", observed: true, cwd: project.root, invocationId: "dirty-invocation",
  } });
  assert.equal(result.reason, "dirty_revision");
  await assert.rejects(access(path.join(project.paths.stateRoot, "legacy-owner-adoption.json")), { code: "ENOENT" });
});

test("simultaneous recovery and adoption journals require manual reconciliation without roll-forward", async () => {
  const module = await testHarness();
  const project = await legacyFixture();
  const envelope = await adoptionEnvelope(project);
  const context = { nativeIdentity: { host: "codex", sessionId: "native-owner", observed: true, cwd: project.root,
    invocationId: "ambiguous-journal-invocation" } };
  await assert.rejects(module.adoptLegacyProjectOwner(project, envelope, context, { failAfterJournal: true }),
    /injected_legacy_owner_adoption_crash/);
  await writeFile(project.paths.ownerRecoveryJournal, "{}\n");
  const before = await Promise.all([project.paths.teams, project.paths.state, project.paths.setup,
    path.join(project.paths.stateRoot, ".legacy-owner-adoption.json"), project.paths.ownerRecoveryJournal].map((file) => readFile(file)));
  await assert.rejects(module.adoptLegacyProjectOwner(await resolveProject(project.root), envelope, context),
    /legacy_owner_adoption_manual_reconciliation_required/);
  assert.deepEqual(await Promise.all([project.paths.teams, project.paths.state, project.paths.setup,
    path.join(project.paths.stateRoot, ".legacy-owner-adoption.json"), project.paths.ownerRecoveryJournal].map((file) => readFile(file))), before);
});

test("legacy adoption rejects every duplicate authority label before journal or receipt publication", async () => {
  const module = await testHarness();
  for (const [label, value] of [["Project", "owner-recovery"], ["Project owner", "root"],
    ["Integration owner", "root"], ["Project owner host", "codex"], ["Integration owner host", "codex"]]) {
    const project = await legacyFixture();
    await writeFile(project.paths.teams, `${await readFile(project.paths.teams, "utf8")}${label}: ${value}\n`);
    const envelope = await adoptionEnvelope(await resolveProject(project.root));
    const before = await Promise.all([project.paths.teams, project.paths.state, project.paths.setup].map((file) => readFile(file)));
    const result = await module.adoptLegacyProjectOwner(await resolveProject(project.root), envelope, { nativeIdentity: {
      host: "codex", sessionId: "native-owner", observed: true, cwd: project.root, invocationId: `duplicate-${label.replaceAll(" ", "-")}`,
    } });
    assert.equal(result.reason, "legacy_owner_shape_invalid", label);
    assert.deepEqual(await Promise.all([project.paths.teams, project.paths.state, project.paths.setup].map((file) => readFile(file))), before, label);
    await assert.rejects(access(path.join(project.paths.stateRoot, ".legacy-owner-adoption.json")), { code: "ENOENT" });
    await assert.rejects(access(path.join(project.paths.stateRoot, "legacy-owner-adoption.json")), { code: "ENOENT" });
  }
});

test("legacy adoption rejects empty authority-label occurrences at EOF without writes", async () => {
  const module = await testHarness();
  const labels = ["Project", "Project owner", "Integration owner", "Project owner host", "Integration owner host"];
  for (const label of labels) for (const suffix of ["", " \t "]) {
    const project = await legacyFixture();
    await writeFile(project.paths.teams, `${await readFile(project.paths.teams, "utf8")}${label}:${suffix}`);
    const envelope = await adoptionEnvelope(await resolveProject(project.root));
    const before = await Promise.all([project.paths.teams, project.paths.state, project.paths.setup].map((file) => readFile(file)));
    const result = await module.adoptLegacyProjectOwner(await resolveProject(project.root), envelope, { nativeIdentity: {
      host: "codex", sessionId: "native-owner", observed: true, cwd: project.root,
      invocationId: `empty-${label.replaceAll(" ", "-")}-${suffix.length}`,
    } });
    assert.equal(result.reason, "legacy_owner_shape_invalid", `${label}:${JSON.stringify(suffix)}`);
    assert.deepEqual(await Promise.all([project.paths.teams, project.paths.state, project.paths.setup].map((file) => readFile(file))),
      before, `${label}:${JSON.stringify(suffix)}`);
    await assert.rejects(access(path.join(project.paths.stateRoot, ".legacy-owner-adoption.json")), { code: "ENOENT" });
    await assert.rejects(access(path.join(project.paths.stateRoot, "legacy-owner-adoption.json")), { code: "ENOENT" });
  }
});

test("hash-consistent adoption journal rejects duplicated legacy authority labels", async () => {
  const module = await testHarness();
  const project = await legacyFixture();
  const envelope = await adoptionEnvelope(project);
  const context = { nativeIdentity: { host: "codex", sessionId: "native-owner", observed: true, cwd: project.root,
    invocationId: "duplicate-journal-invocation" } };
  await assert.rejects(module.adoptLegacyProjectOwner(project, envelope, context, { failAfterJournal: true,
    now: () => "2026-09-12T00:00:01.000Z" }), /injected_legacy_owner_adoption_crash/);
  const journalPath = path.join(project.paths.stateRoot, ".legacy-owner-adoption.json");
  const journal = JSON.parse(await readFile(journalPath, "utf8"));
  const teamsRecord = journal.records.find(({ name }) => name === "teams");
  const priorTeams = `${Buffer.from(teamsRecord.priorimageBase64, "base64")}Project owner: root\n`;
  const postTeams = priorTeams.replace(/^Project owner: root$/m, "Project owner: root\nProject owner host: codex")
    .replace(/^Integration owner: root$/m, "Integration owner: root\nIntegration owner host: codex")
    .replace(/^Project owner:.*$/m, "Project owner: native-owner")
    .replace(/^Integration owner:.*$/m, "Integration owner: native-owner");
  const priorBytes = Buffer.from(priorTeams); const postBytes = Buffer.from(postTeams);
  teamsRecord.priorimageBase64 = priorBytes.toString("base64"); teamsRecord.priorSha256 = createHash("sha256").update(priorBytes).digest("hex");
  teamsRecord.postimageBase64 = postBytes.toString("base64"); teamsRecord.postSha256 = createHash("sha256").update(postBytes).digest("hex");
  journal.receipt.prior.teamsFingerprint = teamsRecord.priorSha256;
  const receiptRecord = journal.records.find(({ name }) => name === "receipt");
  const receiptBytes = Buffer.from(`${JSON.stringify(journal.receipt, null, 2)}\n`);
  receiptRecord.postimageBase64 = receiptBytes.toString("base64"); receiptRecord.postSha256 = createHash("sha256").update(receiptBytes).digest("hex");
  await writeFile(journalPath, `${JSON.stringify(journal, null, 2)}\n`);
  const before = await Promise.all([project.paths.teams, project.paths.state, project.paths.setup, journalPath].map((file) => readFile(file)));
  await assert.rejects(module.adoptLegacyProjectOwner(await resolveProject(project.root), envelope, context),
    /legacy_owner_adoption_manual_reconciliation_required/);
  assert.deepEqual(await Promise.all([project.paths.teams, project.paths.state, project.paths.setup, journalPath].map((file) => readFile(file))), before);
});

test("hash-consistent adoption journals reject empty authority-label occurrences at EOF", async () => {
  const module = await testHarness();
  const labels = ["Project", "Project owner", "Integration owner", "Project owner host", "Integration owner host"];
  for (const label of labels) for (const suffix of ["", " \t "]) {
    const project = await legacyFixture();
    const envelope = await adoptionEnvelope(project);
    const context = { nativeIdentity: { host: "codex", sessionId: "native-owner", observed: true, cwd: project.root,
      invocationId: `empty-journal-${label.replaceAll(" ", "-")}-${suffix.length}` } };
    await assert.rejects(module.adoptLegacyProjectOwner(project, envelope, context, { failAfterJournal: true,
      now: () => "2026-09-12T00:00:01.000Z" }), /injected_legacy_owner_adoption_crash/);
    const journalPath = path.join(project.paths.stateRoot, ".legacy-owner-adoption.json");
    const journal = JSON.parse(await readFile(journalPath, "utf8"));
    const teamsRecord = journal.records.find(({ name }) => name === "teams");
    const fragment = `${label}:${suffix}`;
    const priorBytes = Buffer.concat([Buffer.from(teamsRecord.priorimageBase64, "base64"), Buffer.from(fragment)]);
    const postBytes = Buffer.concat([Buffer.from(teamsRecord.postimageBase64, "base64"), Buffer.from(fragment)]);
    teamsRecord.priorimageBase64 = priorBytes.toString("base64");
    teamsRecord.priorSha256 = createHash("sha256").update(priorBytes).digest("hex");
    teamsRecord.postimageBase64 = postBytes.toString("base64");
    teamsRecord.postSha256 = createHash("sha256").update(postBytes).digest("hex");
    journal.receipt.prior.teamsFingerprint = teamsRecord.priorSha256;
    const receiptRecord = journal.records.find(({ name }) => name === "receipt");
    const receiptBytes = Buffer.from(`${JSON.stringify(journal.receipt, null, 2)}\n`);
    receiptRecord.postimageBase64 = receiptBytes.toString("base64");
    receiptRecord.postSha256 = createHash("sha256").update(receiptBytes).digest("hex");
    await writeFile(journalPath, `${JSON.stringify(journal, null, 2)}\n`);
    const before = await Promise.all([project.paths.teams, project.paths.state, project.paths.setup, journalPath].map((file) => readFile(file)));
    await assert.rejects(module.adoptLegacyProjectOwner(await resolveProject(project.root), envelope, context),
      /legacy_owner_adoption_manual_reconciliation_required/, `${label}:${JSON.stringify(suffix)}`);
    assert.deepEqual(await Promise.all([project.paths.teams, project.paths.state, project.paths.setup, journalPath].map((file) => readFile(file))),
      before, `${label}:${JSON.stringify(suffix)}`);
  }
});

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
    const result = await module.recoverProjectOwner(project, envelope, { nativeOwnerRecovery: await nativeContext(module, project, scenario) });
    assert.equal(result.reason, scenario.reason);
    assert.deepEqual(await Promise.all(files.map((file) => readFile(file))), before);
    await assert.rejects(access(project.paths.ownerRecoveryJournal), { code: "ENOENT" });
  }
});

test("stopped approved recovery publishes one qualified generation and fences the old host", async () => {
  const module = await testHarness();
  const project = await fixture();
  const envelope = await recoveryEnvelope(project);
  const result = await module.recoverProjectOwner(project, envelope, { nativeOwnerRecovery: await nativeContext(module, project),
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
    { nativeOwnerRecovery: await nativeContext(module, await resolveProject(project.root)) });
  assert.equal(replay.status, "duplicate");
  const changed = structuredClone(envelope);
  changed.request.reason = "changed_reason";
  const conflict = await module.recoverProjectOwner(await resolveProject(project.root), changed,
    { nativeOwnerRecovery: await nativeContext(module, await resolveProject(project.root)) });
  assert.equal(conflict.reason, "operation_identity_reused");
});

test("capabilities are one-shot expiry and runtime bound before callbacks", async () => {
  const module = await testHarness();
  const project = await fixture();
  const envelope = await recoveryEnvelope(project);
  let callbacks = 0;
  const context = await nativeContext(module, project, { liveness: "active" });
  context.inspectSession = async () => { callbacks += 1; return { status: "active", observedAt: "2026-09-12T00:00:00.000Z" }; };
  assert.equal((await module.recoverProjectOwner(project, envelope, { nativeOwnerRecovery: context })).reason, "owner_active");
  assert.equal(callbacks, 1);
  assert.equal((await module.recoverProjectOwner(project, envelope, { nativeOwnerRecovery: context })).reason, "native_owner_recovery_required");
  assert.equal(callbacks, 1);

  const crossHost = await nativeContext(module, project);
  crossHost.identity = { ...crossHost.identity, host: "codex" };
  assert.equal((await module.recoverProjectOwner(project, envelope, { nativeOwnerRecovery: crossHost })).reason, "native_owner_recovery_required");

  const expired = await nativeContext(module, project);
  assert.equal((await module.recoverProjectOwner(project, envelope, { nativeOwnerRecovery: expired, nowMs: () => Date.now() + 60_000 })).reason,
    "native_owner_recovery_required");
  assert.equal(callbacks, 1);

  const concurrent = await nativeContext(module, project, { liveness: "active" });
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
    nativeOwnerRecovery: await nativeContext(module, project, { sessionId: "old-owner", host: "claude-code" }),
  })).status, "applied");
  project = await resolveProject(project.root);
  const version = JSON.parse(await readFile(project.paths.state, "utf8")).stateVersion;
  const request = { operationId: "after-transfer", actorSessionId: "old-owner", expectedVersion: version };
  assert.equal((await mutateOperationalState(project, request, (state) => ({ state, result: {} }), {
    nativeIdentity: { host: "codex", sessionId: "old-owner", observed: true, cwd: project.root, ownershipEpoch: 1 },
  })).reason, "project_owner_required");
  assert.equal((await mutateOperationalState(project, request, (state) => ({ state, result: {} }), {
    nativeIdentity: { host: "claude-code", sessionId: "old-owner", observed: true, cwd: project.root, ownershipEpoch: 2 },
  })).status, "applied");
  const { evaluatePolicy } = await import("../hooks/lib/policy.mjs");
  const operation = { kind: "file_change", files: [{ action: "edit", path: "README.md", changedContent: "fixture" }] };
  const oldPolicy = await evaluatePolicy({ runtime: "codex", event: "PreToolUse", cwd: project.root, sessionId: "old-owner", operation }, project);
  const newPolicy = await evaluatePolicy({ runtime: "claude", event: "PreToolUse", cwd: project.root, sessionId: "old-owner", operation }, project);
  assert.equal(oldPolicy.allow, false);
  assert.equal(newPolicy.allow, true);
  const { inspectRecovery } = await import("../hooks/lib/recovery.mjs");
  assert.equal((await inspectRecovery(project, { sessionId: "old-owner", host: "codex", includeGit: true })).identity.kind, "unknown");
  assert.equal((await inspectRecovery(project, { sessionId: "old-owner", host: "claude-code", includeGit: true })).identity.kind, "project_owner");
});

test("a crash after each canonical rename rolls forward to identical generation two bytes", async () => {
  const module = await testHarness();
  for (const failAfterRename of [1, 2, 3, 4]) {
    const project = await fixture();
    const envelope = await recoveryEnvelope(project);
    const invocationId = `crash-${failAfterRename}`;
    const expected = await independentPostimages(project, envelope, { invocationId });
    await assert.rejects(module.recoverProjectOwner(project, envelope, { nativeOwnerRecovery: await nativeContext(module, project, { invocationId }), failAfterRename,
      now: () => "2026-09-12T00:00:00.000Z" }), /injected_owner_recovery_crash/);
    const repaired = await loadCanonicalState(await resolveProject(project.root));
    assert.equal(repaired.state.ownership.epoch, 2);
    const bytes = await Promise.all([project.paths.teams, project.paths.state, project.paths.setup, project.paths.ownerHistory].map((file) => readFile(file)));
    assert.deepEqual(bytes, expected);
  }
});

test("recovery refuses the same qualified identity before callbacks and writes", async () => {
  const module = await testHarness();
  const project = await fixture();
  const envelope = await recoveryEnvelope(project);
  const before = await rawFour(project);
  let callbacks = 0;
  const context = await nativeContext(module, project, { host: "codex", sessionId: "old-owner" });
  context.inspectSession = async () => { callbacks += 1; return { status: "stopped", observedAt: "2026-09-12T00:00:00.000Z" }; };
  context.confirm = async () => { callbacks += 1; return { approved: true, approvalId: "should-not-run" }; };
  assert.deepEqual(await module.recoverProjectOwner(project, envelope, { nativeOwnerRecovery: context }),
    { status: "conflict", reason: "owner_identity_unchanged" });
  assert.equal(callbacks, 0);
  assert.deepEqual(await rawFour(project), before);
  await assertNoRecoveryJournal(project);
});

test("stale versions epoch owner project and every fingerprint preserve raw records", async () => {
  const module = await testHarness();
  const mutations = [
    ["setup-version", (request) => { request.expectedSetupVersion += 1; }],
    ["state-version", (request) => { request.expectedStateVersion += 1; }],
    ["epoch", (request) => { request.expectedOwnershipEpoch += 1; }],
    ["owner", (request) => { request.expectedOwnerSessionId = "other-owner"; }],
    ["project", (request) => { request.projectId = "other-project"; }],
    ...["expectedStateFingerprint", "expectedTeamsFingerprint", "expectedSetupFingerprint", "expectedOwnerHistoryFingerprint"]
      .map((field) => [field, (request) => { request[field] = "f".repeat(64); }]),
  ];
  for (const [name, change] of mutations) {
    const project = await fixture();
    const envelope = await recoveryEnvelope(project);
    change(envelope.request);
    const before = await rawFour(project);
    let callbacks = 0;
    const context = await nativeContext(module, project, { expectedOwnershipEpoch: envelope.request.expectedOwnershipEpoch });
    context.inspectSession = async () => { callbacks += 1; return { status: "stopped", observedAt: "2026-09-12T00:00:00.000Z" }; };
    context.confirm = async () => { callbacks += 1; return { approved: true, approvalId: "approval-1" }; };
    const result = await module.recoverProjectOwner(project, envelope, { nativeOwnerRecovery: context });
    assert.equal(result.status, "conflict", name);
    assert.equal(callbacks, 0, name);
    assert.deepEqual(await rawFour(project), before, name);
    await assertNoRecoveryJournal(project);
  }
});

test("missing qualified writer is liveness-unknown before callbacks", async () => {
  const module = await testHarness();
  const project = await fixture();
  for (const file of [project.paths.state, project.paths.setup]) {
    const record = JSON.parse(await readFile(file, "utf8"));
    delete record.ownership.current.writer;
    await writeFile(file, `${JSON.stringify(record, null, 2)}\n`);
  }
  const envelope = await recoveryEnvelope(project);
  const before = await rawFour(project);
  let callbacks = 0;
  const context = await nativeContext(module, project);
  context.inspectSession = async () => { callbacks += 1; return { status: "stopped", observedAt: "2026-09-12T00:00:00.000Z" }; };
  assert.deepEqual(await module.recoverProjectOwner(project, envelope, { nativeOwnerRecovery: context }),
    { status: "conflict", reason: "owner_liveness_unknown" });
  assert.equal(callbacks, 0);
  assert.deepEqual(await rawFour(project), before);
  await assertNoRecoveryJournal(project);
});

test("hash-consistent journal cannot substitute a valid but unauthenticated writer", async () => {
  const module = await testHarness();
  const project = await fixture();
  const envelope = await recoveryEnvelope(project);
  await assert.rejects(module.recoverProjectOwner(project, envelope, { nativeOwnerRecovery: await nativeContext(module, project),
    failAfterJournal: true, now: () => "2026-09-12T00:00:00.000Z" }), /prepared/);
  const before = await rawFour(project);
  const journalBefore = await readFile(project.paths.ownerRecoveryJournal);
  const journal = JSON.parse(journalBefore);
  for (const name of ["state", "setup"]) {
    const record = journal.records.find((candidate) => candidate.name === name);
    const value = JSON.parse(Buffer.from(record.postimageBase64, "base64"));
    value.ownership.current.writer.invocationId = "forged-invocation";
    const postimage = Buffer.from(`${JSON.stringify(value, null, 2)}\n`);
    record.postimageBase64 = postimage.toString("base64");
    record.postSha256 = createHash("sha256").update(postimage).digest("hex");
  }
  await writeFile(project.paths.ownerRecoveryJournal, `${JSON.stringify(journal, null, 2)}\n`);
  const forgedJournal = await readFile(project.paths.ownerRecoveryJournal);
  await assert.rejects(repairOwnerRecovery(project), /manual_reconciliation/);
  assert.deepEqual(await rawFour(project), before);
  assert.deepEqual(await readFile(project.paths.ownerRecoveryJournal), forgedJournal);
});

test("successful history binds the authenticated writer and preserves unrelated records", async () => {
  const module = await testHarness();
  const project = await fixture();
  const state = JSON.parse(await readFile(project.paths.state, "utf8"));
  state.pendingOperations = { pending: { phase: "uncertain", custom: "keep" } };
  state.integration.custom = { keep: true };
  state.release.custom = { keep: true };
  await writeFile(project.paths.state, `${JSON.stringify(state, null, 2)}\n`);
  const setup = JSON.parse(await readFile(project.paths.setup, "utf8"));
  setup.settings = { hosts: { codex: { custom: "keep" }, "claude-code": { custom: "keep" } } };
  await writeFile(project.paths.setup, `${JSON.stringify(setup, null, 2)}\n`);
  const envelope = await recoveryEnvelope(project);
  const context = await nativeContext(module, project, { invocationId: "authenticated-invocation" });
  assert.equal((await module.recoverProjectOwner(project, envelope, { nativeOwnerRecovery: context,
    now: () => "2026-09-12T00:00:00.000Z" })).status, "applied");
  const canonical = await loadCanonicalState(await resolveProject(project.root));
  assert.deepEqual(canonical.ownerHistory.entries[0].writer, canonical.state.ownership.current.writer);
  assert.deepEqual(canonical.state.pendingOperations, state.pendingOperations);
  assert.deepEqual(canonical.state.integration.custom, state.integration.custom);
  assert.deepEqual(canonical.state.release.custom, state.release.custom);
  assert.deepEqual(canonical.setup.settings, setup.settings);
});

test("journal structure phase receipt and semantic postimages are exact before the first rename", async () => {
  const cases = [
    ["top-level extra", (journal) => { journal.forged = true; }],
    ["record extra", (journal) => { journal.records[0].forged = true; }],
    ["phase mismatch", (journal) => { journal.phase = "renamed:1"; }],
    ["signature", (journal) => { journal.signature = "0".repeat(64); }],
    ["receipt", (journal) => { journal.receipt.newOwner.host = "codex"; }],
    ["state version", (journal) => rewritePostimage(journal, "state", (state) => { state.stateVersion += 1; })],
    ["state owner", (journal) => rewritePostimage(journal, "state", (state) => { state.ownership.current.sessionId = "forged-owner"; })],
    ["setup epoch", (journal) => rewritePostimage(journal, "setup", (setup) => { setup.ownership.epoch += 1; })],
    ["gate triple", (journal) => rewritePostimage(journal, "state", (state) => { state.release.ownershipEpoch += 1; })],
    ["history receipt", (journal) => rewritePostimage(journal, "owner-history", (history) => { history.entries.at(-1).approvalId = "forged"; })],
    ["null state", (journal) => replacePostimage(journal, "state", null)],
    ["scalar state", (journal) => replacePostimage(journal, "state", 1)],
    ["array state", (journal) => replacePostimage(journal, "state", [])],
    ["null setup", (journal) => replacePostimage(journal, "setup", null)],
    ["scalar setup", (journal) => replacePostimage(journal, "setup", "setup")],
    ["array setup", (journal) => replacePostimage(journal, "setup", [])],
    ["null history", (journal) => replacePostimage(journal, "owner-history", null)],
    ["scalar history", (journal) => replacePostimage(journal, "owner-history", 1)],
    ["array history", (journal) => replacePostimage(journal, "owner-history", [])],
    ["missing state ownership", (journal) => rewritePostimage(journal, "state", (state) => { delete state.ownership; })],
    ["array state integration", (journal) => rewritePostimage(journal, "state", (state) => { state.integration = []; })],
    ["null state release", (journal) => rewritePostimage(journal, "state", (state) => { state.release = null; })],
    ["missing setup ownership", (journal) => rewritePostimage(journal, "setup", (setup) => { delete setup.ownership; })],
    ["array history ownership", (journal) => rewritePostimage(journal, "owner-history", (history) => { history.ownership = []; })],
    ["null history entry", (journal) => rewritePostimage(journal, "owner-history", (history) => { history.entries[history.entries.length - 1] = null; })],
  ];
  const module = await testHarness();
  for (const [name, mutate] of cases) {
    const project = await fixture();
    const envelope = await recoveryEnvelope(project);
    await assert.rejects(module.recoverProjectOwner(project, envelope, { nativeOwnerRecovery: await nativeContext(module, project),
      failAfterJournal: true, now: () => "2026-09-12T00:00:00.000Z" }), /prepared/);
    const before = await rawFour(project);
    const journal = JSON.parse(await readFile(project.paths.ownerRecoveryJournal));
    mutate(journal);
    await writeFile(project.paths.ownerRecoveryJournal, `${JSON.stringify(journal, null, 2)}\n`);
    const journalBytes = await readFile(project.paths.ownerRecoveryJournal);
    await assert.rejects(repairOwnerRecovery(project), /manual_reconciliation/, name);
    assert.deepEqual(await rawFour(project), before, name);
    assert.deepEqual(await readFile(project.paths.ownerRecoveryJournal), journalBytes, name);
  }
});

test("qualified absent empty whitespace or malformed history refuses while exact legacy absence remains readable", async () => {
  for (const [mode, source, reason] of [
    ["absent", undefined, /owner_history_missing/],
    ["empty", "", /owner_history_invalid/],
    ["whitespace", " \n\t", /owner_history_invalid/],
    ["malformed", undefined, /owner_generation/],
  ]) {
    const project = await fixture();
    if (mode === "absent") await rm(project.paths.ownerHistory);
    else if (source !== undefined) await writeFile(project.paths.ownerHistory, source);
    else {
      const history = JSON.parse(await readFile(project.paths.ownerHistory));
      history.extra = true;
      await writeFile(project.paths.ownerHistory, JSON.stringify(history));
    }
    await assert.rejects(loadCanonicalState(await resolveProject(project.root)), reason);
  }
  const legacy = await fixture();
  const state = JSON.parse(await readFile(legacy.paths.state));
  delete state.ownership;
  for (const gate of [state.integration, state.release]) { delete gate.ownerHost; delete gate.ownershipEpoch; }
  await writeFile(legacy.paths.state, `${JSON.stringify(state, null, 2)}\n`);
  const setup = JSON.parse(await readFile(legacy.paths.setup));
  delete setup.ownership;
  await writeFile(legacy.paths.setup, `${JSON.stringify(setup, null, 2)}\n`);
  let teams = await readFile(legacy.paths.teams, "utf8");
  teams = teams.replace(/^Project owner host:.*\n/m, "").replace(/^Integration owner host:.*\n/m, "");
  await writeFile(legacy.paths.teams, teams);
  await rm(legacy.paths.ownerHistory);
  const canonical = await loadCanonicalState(await resolveProject(legacy.root));
  assert.equal(canonical.ownerHistory, undefined);
  assert.equal(canonical.registry.ownershipEpoch, undefined);
});

test("capability rejects cross invocation session project cwd and linked worktree before callbacks", async () => {
  const module = await testHarness();
  const project = await fixture();
  const other = await fixture();
  const linked = `${project.root}-linked`;
  temporary.push(linked);
  execFileSync("git", ["worktree", "add", "-q", "-b", "linked-test", linked], { cwd: project.root });
  const nested = path.join(project.root, "nested-repository");
  await mkdir(nested);
  execFileSync("git", ["init", "-q", "-b", "main", nested]);
  const cases = [
    ["invocation", async () => { const value = await nativeContext(module, project); value.identity.invocationId = "other-invocation"; return [project, value]; }],
    ["session", async () => { const value = await nativeContext(module, project); value.identity.sessionId = "other-session"; return [project, value]; }],
    ["project", async () => [other, await nativeContext(module, project)]],
    ["cwd", async () => { const value = await nativeContext(module, project); value.identity.cwd = other.root; return [project, value]; }],
    ["linked", async () => { const value = await nativeContext(module, project); value.identity.cwd = linked; return [project, value]; }],
    ["nested", async () => { const value = await nativeContext(module, project); value.identity.cwd = nested; return [project, value]; }],
    ["worktree field", async () => { const value = await nativeContext(module, project); value.identity.worktreeRoot = linked; return [project, value]; }],
    ["common directory", async () => { const value = await nativeContext(module, project); return [{ ...project, commonDirectory: other.commonDirectory }, value]; }],
  ];
  for (const [name, build] of cases) {
    const [target, context] = await build();
    const envelope = await recoveryEnvelope(target);
    const before = await rawFour(target);
    let callbacks = 0;
    context.inspectSession = async () => { callbacks += 1; return { status: "stopped", observedAt: "2026-09-12T00:00:00.000Z" }; };
    context.confirm = async () => { callbacks += 1; return { approved: true, approvalId: "approval-1" }; };
    assert.equal((await module.recoverProjectOwner(target, envelope, { nativeOwnerRecovery: context })).reason,
      "native_owner_recovery_required", name);
    assert.equal(callbacks, 0, name);
    assert.deepEqual(await rawFour(target), before, name);
    await assertNoRecoveryJournal(target);
  }
  await assert.rejects(nativeContext(module, await resolveProject(linked), { cwd: linked }), /invalid_owner_recovery_binding/);
});

test("prepared and committed crash boundaries repair once and third hashes preserve the journal", async () => {
  const module = await testHarness();
  for (const option of [{ failAfterJournal: true }, { failAfterCommitted: true }]) {
    const project = await fixture();
    const envelope = await recoveryEnvelope(project);
    await assert.rejects(module.recoverProjectOwner(project, envelope, { nativeOwnerRecovery: await nativeContext(module, project), ...option,
      now: () => "2026-09-12T00:00:00.000Z" }), /injected_owner_recovery_crash/);
    assert.equal((await repairOwnerRecovery(project)).status, "repaired");
    assert.equal((await loadCanonicalState(await resolveProject(project.root))).state.ownership.epoch, 2);
    await assertNoRecoveryJournal(project);
  }
  const divergenceBoundaries = [{ failAfterJournal: true }, ...[1, 2, 3, 4].map((failAfterRename) => ({ failAfterRename })), { failAfterCommitted: true }];
  for (const injection of divergenceBoundaries) {
    const project = await fixture();
    const envelope = await recoveryEnvelope(project);
    await assert.rejects(module.recoverProjectOwner(project, envelope, { nativeOwnerRecovery: await nativeContext(module, project), ...injection,
      now: () => "2026-09-12T00:00:00.000Z" }), /injected_owner_recovery_crash/);
    const parsedJournal = JSON.parse(await readFile(project.paths.ownerRecoveryJournal));
    const target = parsedJournal.records[Math.min(parsedJournal.nextRecord, 3)].path;
    await writeFile(target, "{\"third\":true}\n");
    const before = await rawFour(project);
    const journal = await readFile(project.paths.ownerRecoveryJournal);
    await assert.rejects(repairOwnerRecovery(project), /manual_reconciliation/);
    assert.deepEqual(await rawFour(project), before);
    assert.deepEqual(await readFile(project.paths.ownerRecoveryJournal), journal);
  }
});

test("epoch-one authority is fenced from operational checkpoint setup integration and release paths", async () => {
  const module = await testHarness();
  const original = await fixture();
  const envelope = await recoveryEnvelope(original);
  assert.equal((await module.recoverProjectOwner(original, envelope, { nativeOwnerRecovery: await nativeContext(module, original),
    now: () => "2026-09-12T00:00:00.000Z" })).status, "applied");
  const project = await resolveProject(original.root);
  const staleIdentity = { host: "claude-code", sessionId: "new-owner", observed: true, cwd: project.root, ownershipEpoch: 1 };
  const current = await loadCanonicalState(project);
  const before = await rawFour(project);
  let mutatorCalls = 0;
  const authorityCases = [
    ["stale", staleIdentity],
    ["absent", (({ ownershipEpoch: _, ...identity }) => identity)({ ...staleIdentity, ownershipEpoch: 2 })],
    ["zero", { ...staleIdentity, ownershipEpoch: 0 }],
    ["noninteger", { ...staleIdentity, ownershipEpoch: 2.5 }],
    ["host", { ...staleIdentity, ownershipEpoch: 2, host: "codex" }],
    ["session", { ...staleIdentity, ownershipEpoch: 2, sessionId: "other-owner" }],
    ["observed", { ...staleIdentity, ownershipEpoch: 2, observed: false }],
    ["cwd", { ...staleIdentity, ownershipEpoch: 2, cwd: path.dirname(project.root) }],
  ];
  for (const [name, identity] of authorityCases) {
    const operational = await mutateOperationalState(project, { operationId: `invalid-${name}`, actorSessionId: "new-owner",
      expectedVersion: current.state.stateVersion }, (state) => { mutatorCalls += 1; return { state, result: {} }; }, { nativeIdentity: identity });
    assert.equal(operational.reason, "project_owner_required", name);
  }
  assert.equal(mutatorCalls, 0);

  const { writeCheckpoint } = await import("../hooks/lib/checkpoint.mjs");
  const checkpoint = await writeCheckpoint(project, { eventId: "stale-checkpoint", sessionId: "new-owner", taskIds: ["T-1"], nextAction: "none" },
    { actorSessionId: "new-owner", expectedVersion: 0, nativeIdentity: staleIdentity });
  assert.equal(checkpoint.reason, "checkpoint_owner_required");

  const { runSetupCommand } = await import("../hooks/lib/setup-cli.mjs");
  const setupRequest = path.join(project.root, "stale-setup.json");
  await writeFile(setupRequest, JSON.stringify({ schemaVersion: 1, expectedVersion: current.setup.version, operationId: "stale-setup",
    writer: { id: "new-owner", role: "project_orchestrator" }, request: { change: { kind: "run", values: { continuous: true } } } }));
  assert.equal((await runSetupCommand("settings-update", { project: project.root, host: "claude-code", scope: "project", request: setupRequest },
    { nativeIdentity: staleIdentity })).reason, "project_owner_required");

  const { recordGateEvidence } = await import("../hooks/lib/task-transitions.mjs");
  for (const gate of ["integration", "release"]) {
    const result = await recordGateEvidence(project, { operationId: `stale-${gate}`, actorSessionId: "new-owner",
      expectedVersion: current.state.stateVersion, expectedFingerprint: current.tracker.fingerprint, gate, taskIds: ["T-1"],
      expectedRevision: "0".repeat(40), evidencePath: path.join(project.root, `missing-${gate}.json`) }, { nativeIdentity: staleIdentity });
    assert.equal(result.reason, "project_owner_required", gate);
  }
  assert.deepEqual(await rawFour(project), before);
  await assertNoRecoveryJournal(project);
});

test("journal publication fsyncs files before rename and directories after every rename and unlink", async () => {
  const module = await testHarness();
  const project = await fixture();
  const envelope = await recoveryEnvelope(project);
  const events = [];
  module.__observeIo((event) => events.push(event));
  assert.equal((await module.recoverProjectOwner(project, envelope, { nativeOwnerRecovery: await nativeContext(module, project),
    now: () => "2026-09-12T00:00:00.000Z" })).status, "applied");
  for (let index = 0; index < events.length; index += 1) {
    if (events[index].kind === "rename") {
      assert.deepEqual(events[index - 1], { kind: "file_fsync", file: events[index].file });
      assert.deepEqual(events[index + 1], { kind: "directory_fsync", directory: path.dirname(events[index].file), file: events[index].file });
    }
    if (events[index].kind === "unlink") {
      assert.deepEqual(events[index + 1], { kind: "directory_fsync", directory: path.dirname(events[index].file), file: events[index].file });
    }
  }
  assert.deepEqual(events.filter(({ kind, file }) => kind === "rename" && fourPaths(project).includes(file)).map(({ file }) => file), fourPaths(project));
});

test("before-journal and after-unlink crash seams preserve closed transaction outcomes", async () => {
  const module = await testHarness();
  const beforeProject = await fixture();
  const before = await rawFour(beforeProject);
  await assert.rejects(module.recoverProjectOwner(beforeProject, await recoveryEnvelope(beforeProject), {
    nativeOwnerRecovery: await nativeContext(module, beforeProject), failBeforeJournal: true,
  }), /before_prepared/);
  assert.deepEqual(await rawFour(beforeProject), before);
  await assertNoRecoveryJournal(beforeProject);

  const afterProject = await fixture();
  await assert.rejects(module.recoverProjectOwner(afterProject, await recoveryEnvelope(afterProject), {
    nativeOwnerRecovery: await nativeContext(module, afterProject), failAfterUnlink: true,
    now: () => "2026-09-12T00:00:00.000Z",
  }), /unlinked/);
  assert.equal((await loadCanonicalState(await resolveProject(afterProject.root))).state.ownership.epoch, 2);
  await assertNoRecoveryJournal(afterProject);
});

test("a committed hash-consistent malicious journal is validated before unlink", async () => {
  const module = await testHarness();
  const project = await fixture();
  await assert.rejects(module.recoverProjectOwner(project, await recoveryEnvelope(project), {
    nativeOwnerRecovery: await nativeContext(module, project), failAfterCommitted: true,
    now: () => "2026-09-12T00:00:00.000Z",
  }), /committed/);
  const journal = JSON.parse(await readFile(project.paths.ownerRecoveryJournal));
  for (const name of ["state", "setup"]) {
    rewritePostimage(journal, name, (value) => { value.ownership.current.writer.invocationId = "committed-forgery"; });
    const record = journal.records.find((candidate) => candidate.name === name);
    await writeFile(record.path, Buffer.from(record.postimageBase64, "base64"));
  }
  await writeFile(project.paths.ownerRecoveryJournal, `${JSON.stringify(journal, null, 2)}\n`);
  const before = await rawFour(project);
  const journalBytes = await readFile(project.paths.ownerRecoveryJournal);
  await assert.rejects(repairOwnerRecovery(project), /manual_reconciliation/);
  assert.deepEqual(await rawFour(project), before);
  assert.deepEqual(await readFile(project.paths.ownerRecoveryJournal), journalBytes);
});

test("canonical subdirectories validate while linked and nested Git identities do not", async () => {
  const module = await testHarness();
  const project = await fixture();
  const subdirectory = path.join(project.root, "src", "nested");
  await mkdir(subdirectory, { recursive: true });
  const result = await module.recoverProjectOwner(project, await recoveryEnvelope(project), {
    nativeOwnerRecovery: await nativeContext(module, project, { cwd: subdirectory, liveness: "active" }),
  });
  assert.deepEqual(result, { status: "conflict", reason: "owner_active" });
  await assertNoRecoveryJournal(project);
});

test("history entries reject extras broken chains duplicates and unauthenticated liveness", async () => {
  const module = await testHarness();
  const mutations = [
    ["extra", (history) => { history.entries[0].extra = true; }],
    ["old owner", (history) => { history.entries[0].oldOwner.sessionId = "other"; }],
    ["liveness", (history) => { history.entries[0].livenessEvidence.status = "unknown"; }],
    ["duplicate", (history) => { history.entries.push(structuredClone(history.entries[0])); history.version = 3; history.ownership.epoch = 3; }],
  ];
  for (const [name, mutate] of mutations) {
    const project = await fixture();
    await module.recoverProjectOwner(project, await recoveryEnvelope(project), { nativeOwnerRecovery: await nativeContext(module, project),
      now: () => "2026-09-12T00:00:00.000Z" });
    const history = JSON.parse(await readFile(project.paths.ownerHistory));
    mutate(history);
    await writeFile(project.paths.ownerHistory, JSON.stringify(history));
    await assert.rejects(loadCanonicalState(await resolveProject(project.root)), /owner_generation/, name);
  }
});

test("a second recovery appends one contiguous authenticated transition without rewriting the first", async () => {
  const module = await testHarness();
  let project = await fixture();
  await module.recoverProjectOwner(project, await recoveryEnvelope(project), { nativeOwnerRecovery: await nativeContext(module, project),
    now: () => "2026-09-12T00:00:00.000Z" });
  project = await resolveProject(project.root);
  const first = structuredClone(JSON.parse(await readFile(project.paths.ownerHistory)).entries[0]);
  const envelope = await recoveryEnvelope(project);
  envelope.request.operationId = "recover-owner-again";
  const context = await nativeContext(module, project, { host: "codex", sessionId: "third-owner", expectedOwnershipEpoch: 2,
    invocationId: "second-invocation" });
  assert.equal((await module.recoverProjectOwner(project, envelope, { nativeOwnerRecovery: context,
    now: () => "2026-09-12T00:01:00.000Z" })).status, "applied");
  const history = JSON.parse(await readFile(project.paths.ownerHistory));
  assert.equal(history.version, 3);
  assert.equal(history.ownership.epoch, 3);
  assert.deepEqual(history.entries[0], first);
  assert.equal(history.entries[1].epoch, 2);
  assert.deepEqual(history.entries[1].oldOwner, first.newOwner);
  assert.deepEqual(history.entries[1].newOwner, { host: "codex", sessionId: "third-owner" });
});
