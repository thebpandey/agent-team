import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import { createHash } from "node:crypto";
import { chmod, cp, mkdir, mkdtemp, readFile, readdir, rm, writeFile } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import test from "node:test";

import { readActivationLogs } from "../hooks/lib/telemetry.mjs";
import { policyFixture } from "./hook-test-helpers.mjs";
import { normalizeEvent } from '../hooks/lib/event.mjs';
import { runNormalizedHook } from '../hooks/agent-team-hook.mjs';
import { resolveProject } from '../hooks/lib/project.mjs';
import { readTracker } from '../hooks/lib/tracker.mjs';
import { runCommand } from '../hooks/agent-team-cli.mjs';
import { effectiveRunFingerprint } from '../hooks/lib/run-state.mjs';

const hook = path.resolve(import.meta.dirname, "..", "hooks", "agent-team-hook.mjs");
const cli = path.resolve(import.meta.dirname, "..", "hooks", "agent-team-cli.mjs");
const boundedBeads = path.resolve(import.meta.dirname, "fixtures", "bounded-beads-cli.mjs");
const temporary = [];

test.afterEach(async () => Promise.all(temporary.splice(0).map((item) => rm(item, { force: true, recursive: true }))));

async function fixture(options) {
  const root = await mkdtemp(path.join(os.tmpdir(), "agent-team-entry-"));
  const home = await mkdtemp(path.join(os.tmpdir(), "agent-team-entry-home-"));
  temporary.push(root, `${root}-feature`, `${root}-remote`, home);
  return { ...(await policyFixture(root, options)), home };
}

function effectiveRun(ownerHost) {
  return { id: "verification-run", ownerSessionId: "owner-session", ownerHost, ownershipEpoch: 1, mode: "finite", taskIds: ["AT-001"],
    teamLimit: 1, autoDeploy: true, batchSize: 1, source: "explicit_run",
    settingSources: Object.fromEntries(["mode", "taskIds", "teamLimit", "autoDeploy", "batchSize"].map((key) => [key, "explicit_run"])),
    paused: false, operationalVersion: 0, blockers: [], pendingDeliveryIds: [], deployedTaskIds: [], terminalClassification: "progress_possible" };
}

async function inactiveFixture() {
  const root = await mkdtemp(path.join(os.tmpdir(), "agent-team-entry-inactive-"));
  const home = await mkdtemp(path.join(os.tmpdir(), "agent-team-entry-home-"));
  temporary.push(root, home);
  const initialized = spawnSync("git", ["init", "-q", "-b", "main", root]);
  assert.equal(initialized.status, 0);
  return { root, home };
}

function invoke(runtime, event, payload, home) {
  const result = spawnSync(process.execPath, [hook, "--runtime", runtime, "--event", event], {
    input: JSON.stringify(payload),
    encoding: "utf8",
    env: { ...process.env, HOME: home },
  });
  return { status: result.status, stdout: result.stdout, stderr: result.stderr };
}

function output(result) {
  return JSON.parse(result.stdout);
}

async function legacyEntryFixture({ legacyOwner = "root", nativeOwner = "native-owner", continuity = false } = {}) {
  const value = await fixture();
  const setupPath = path.join(value.root, ".agent-team/setup.json");
  const statePath = path.join(value.root, ".agent-team/state.json");
  const teamsPath = path.join(value.root, ".agent-team/TEAMS.md");
  const setup = JSON.parse(await readFile(setupPath, "utf8"));
  const state = JSON.parse(await readFile(statePath, "utf8"));
  setup.version = 1; delete setup.ownership;
  state.stateVersion = 0; delete state.ownership;
  for (const gate of [state.integration, state.release]) {
    gate.ownerSessionId = legacyOwner; delete gate.ownerHost; delete gate.ownershipEpoch;
  }
  let teams = await readFile(teamsPath, "utf8");
  teams = teams.replace(/^Project owner:.*$/m, `Project owner: ${legacyOwner}`).replace(/^Integration owner:.*$/m, `Integration owner: ${legacyOwner}`)
    .replace(/^Project owner host:.*\n/m, "").replace(/^Integration owner host:.*\n/m, "");
  await writeFile(setupPath, `${JSON.stringify(setup, null, 2)}\n`);
  await writeFile(statePath, `${JSON.stringify(state, null, 2)}\n`);
  await writeFile(teamsPath, teams);
  const project = await resolveProject(value.root);
  const tracker = await readTracker(project);
  const digest = async (file) => createHash("sha256").update(await readFile(file)).digest("hex");
  const operationId = continuity ? "entry-migrate-legacy" : "entry-adopt-root";
  const authorization = continuity ? {
    kind: "user-directed-maintenance", status: "approved", scope: "legacy_coordinator_continuity_migration",
    source: "explicit user instruction to continue in the current native session", approvalId: "entry-continuity-approval",
    grantedAt: "2026-09-12T00:00:00.000Z", projectId: project.projectId, revision: value.revision,
    trackerFingerprint: tracker.tracker.fingerprint, expectedLegacyOwnerSessionId: legacyOwner,
    newOwner: { host: "codex", sessionId: nativeOwner },
  } : { status: "approved", scope: "legacy_owner_adoption",
    source: "approved-entry-test", approvalId: "entry-approval", grantedAt: "2026-09-12T00:00:00.000Z",
    projectId: project.projectId, revision: value.revision, trackerFingerprint: tracker.tracker.fingerprint, host: "codex" };
  const request = { schemaVersion: 1, request: {
    operationId, projectId: project.projectId, expectedLegacyOwnerSessionId: legacyOwner,
    expectedProjectRoot: project.root, expectedRevision: value.revision,
    expectedTracker: { kind: tracker.tracker.kind, path: tracker.tracker.path, fingerprint: tracker.tracker.fingerprint },
    expectedSetupVersion: 1, expectedStateVersion: 0, expectedTeamsFingerprint: await digest(project.paths.teams),
    expectedSetupFingerprint: await digest(project.paths.setup), expectedStateFingerprint: await digest(project.paths.state),
    expectedOwnerHistoryFingerprint: null, authorization,
    reason: continuity ? "continue from the legacy coordinator in this native session" : "adopt synthetic root",
  } };
  const requestPath = path.join(project.paths.stateRoot, "adopt.json");
  await writeFile(requestPath, `${JSON.stringify(request, null, 2)}\n`);
  return { ...value, project, request, requestPath };
}

async function ownerRunFixture({ scopeTask = false, host = "codex" } = {}) {
  const value = await fixture({ qualifiedOwnership: true });
  if (host !== "codex") {
    for (const file of ["setup.json", "state.json"]) {
      const target = path.join(value.root, ".agent-team", file);
      const record = JSON.parse(await readFile(target, "utf8"));
      await writeFile(target, `${JSON.stringify(record, (_key, entry) => entry === "codex" ? host : entry, 2)}\n`);
    }
    const teams = path.join(value.root, ".agent-team", "TEAMS.md");
    await writeFile(teams, (await readFile(teams, "utf8")).replaceAll("host: codex", `host: ${host}`));
  }
  const project = await resolveProject(value.root);
  if (scopeTask) await writeFile(project.paths.tasks,
    `${await readFile(project.paths.tasks, "utf8")}| AT-002 | Later delivery | none | AT-001 | ready | none | Claim. | task | |\n`);
  const state = JSON.parse(await readFile(project.paths.state, "utf8"));
  state.run = effectiveRun(host);
  await writeFile(project.paths.state, `${JSON.stringify(state, null, 2)}\n`);
  const tracker = await readTracker(project);
  const proposal = Object.fromEntries(["id", "mode", "taskIds", "teamLimit", "autoDeploy", "batchSize", "source", "settingSources"]
    .map((key) => [key, state.run[key]]));
  const body = scopeTask
    ? { operationId: "native-scope-extension", expectedTrackerFingerprint: tracker.tracker.fingerprint,
      taskIds: ["AT-002"], reason: "Admit the tracked dependent delivery." }
    : { operationId: "native-run-reconcile", expectedTrackerFingerprint: tracker.tracker.fingerprint,
      expectedRunFingerprint: effectiveRunFingerprint(state.run), authoritativeSource: "explicit_run",
      reason: "Confirm the effective native-owner run.", affectedTaskIds: ["AT-001"], run: proposal };
  const requestPath = path.join(project.paths.stateRoot, scopeTask ? "scope.json" : "reconcile.json");
  await writeFile(requestPath, `${JSON.stringify({ schemaVersion: 1, actorSessionId: "owner-session",
    expectedVersion: state.stateVersion ?? 0, request: body }, null, 2)}\n`);
  return { ...value, project, requestPath };
}

test("native PreToolUse performs exact legacy adoption while bare CLI only reports required native authority or replay", async () => {
  const value = await legacyEntryFixture();
  const bare = invokeCli("legacy-owner-adopt", { project: value.root, request: value.requestPath });
  assert.equal(bare.status, 0);
  assert.equal(bare.output.reason, "native_legacy_owner_adoption_required");
  const command = `node ${cli} legacy-owner-adopt --project ${value.root} --request ${value.requestPath}`;
  const event = normalizeEvent("codex", "PreToolUse", { cwd: value.root, session_id: "native-owner", event_id: "entry-invocation",
    tool_name: "exec_command", tool_input: { cmd: command } });
  const result = await runNormalizedHook(event);
  assert.equal(result.decision.allow, true);
  assert.equal(result.decision.mutations.some((entry) => entry.kind === "legacy_owner_adoption" && entry.status === "applied"), true,
    JSON.stringify(result.decision));
  const state = JSON.parse(await readFile(value.project.paths.state, "utf8"));
  assert.equal(state.ownership.current.sessionId, "native-owner");
  assert.equal(state.integration.authorized, false);
  assert.equal(state.release.authorized, false);
  const replay = invokeCli("legacy-owner-adopt", { project: value.root, request: value.requestPath });
  assert.equal(replay.output.status, "duplicate");
});

test("native PreToolUse performs an explicitly targeted legacy continuity migration", async () => {
  const legacyOwner = "69ebff15-55b3-4f58-b531-legacy-owner";
  const nativeOwner = "01a08d53-dc5d-7fe2-9c3a-371cde406965";
  const value = await legacyEntryFixture({ legacyOwner, nativeOwner, continuity: true });
  const bare = invokeCli("legacy-owner-adopt", { project: value.root, request: value.requestPath });
  assert.equal(bare.status, 0);
  assert.equal(bare.output.reason, "native_legacy_owner_adoption_required");
  const command = `node ${cli} legacy-owner-adopt --project ${value.root} --request ${value.requestPath}`;
  const result = await runNormalizedHook(normalizeEvent("codex", "PreToolUse", { cwd: value.root,
    session_id: nativeOwner, event_id: "entry-continuity-invocation", tool_name: "exec_command", tool_input: { cmd: command } }));
  assert.equal(result.decision.allow, true, JSON.stringify(result.decision));
  assert.equal(result.decision.mutations.some((entry) => entry.kind === "legacy_owner_adoption" && entry.status === "applied"), true,
    JSON.stringify(result.decision));
  const project = await resolveProject(value.root);
  const state = JSON.parse(await readFile(project.paths.state, "utf8"));
  assert.deepEqual({ host: state.ownership.current.host, sessionId: state.ownership.current.sessionId },
    { host: "codex", sessionId: nativeOwner });
  assert.equal(state.integration.hold, true);
  assert.equal(state.integration.authorized, false);
  assert.equal(state.release.hold, true);
  assert.equal(state.release.authorized, false);
  const receipt = JSON.parse(await readFile(path.join(project.paths.stateRoot, "legacy-owner-adoption.json"), "utf8"));
  assert.equal(receipt.kind, "legacy_coordinator_continuity");
  assert.equal(receipt.expectedLegacyOwnerSessionId, legacyOwner);
  assert.equal(receipt.nativeEvidence.sessionId, nativeOwner);
  assert.equal(receipt.livenessEvidence.status, "not_asserted");
});

test("native command adapter rejects copied CLI paths and chains without adopting", async () => {
  const value = await legacyEntryFixture();
  const copied = path.join(value.root, "agent-team-cli.mjs");
  await cp(cli, copied);
  for (const command of [
    `node ${copied} legacy-owner-adopt --project ${value.root} --request ${value.requestPath}`,
    `node ${cli} legacy-owner-adopt --project ${value.root} --request ${value.requestPath} ; true`,
  ]) {
    const event = normalizeEvent("codex", "PreToolUse", { cwd: value.root, session_id: "native-owner", event_id: "bad-invocation",
      tool_name: "exec_command", tool_input: { cmd: command } });
    const result = await runNormalizedHook(event);
    assert.equal(result.decision.mutations.some((entry) => entry.kind === "legacy_owner_adoption"), false, `${command}\n${JSON.stringify(result.decision)}`);
  }
  await assert.rejects(readFile(path.join(value.root, ".agent-team/legacy-owner-adoption.json")), { code: "ENOENT" });
});

test("native historical reconciliation commands report their exact mutation kinds", async () => {
  const value = await fixture({ qualifiedOwnership: true });
  for (const [command, kind] of [
    ["evidence-store-register", "evidence_store_registration"],
    ["completion-history-reconcile", "completion_history_reconciliation"],
  ]) {
    const request = path.join(value.root, ".agent-team", `${command}.json`);
    await writeFile(request, `${JSON.stringify({ schemaVersion: 1, actorSessionId: "owner-session", expectedVersion: 0, request: {} })}\n`);
    const invocation = `node ${cli} ${command} --project ${value.root} --request ${request}`;
    const result = await runNormalizedHook(normalizeEvent("codex", "PreToolUse", { cwd: value.root,
      session_id: "owner-session", event_id: `native-${command}`, tool_name: "exec_command", tool_input: { cmd: invocation } }));
    assert.equal(result.decision.allow, false, JSON.stringify(result.decision));
    assert.equal(result.decision.mutations.some((entry) => entry.kind === kind && entry.command === command
      && entry.status === "conflict" && entry.reason === "invalid_request"), true, JSON.stringify(result.decision));
  }
});

test("adopted owner can apply exact gate evidence through the same trusted native adapter", async () => {
  const value = await legacyEntryFixture();
  const adoptionCommand = `node ${cli} legacy-owner-adopt --project ${value.root} --request ${value.requestPath}`;
  const adopted = await runNormalizedHook(normalizeEvent("codex", "PreToolUse", { cwd: value.root, session_id: "native-owner",
    event_id: "adoption-before-gate", tool_name: "exec_command", tool_input: { cmd: adoptionCommand } }));
  assert.equal(adopted.decision.mutations.some((entry) => entry.kind === "legacy_owner_adoption" && entry.status === "applied"), true);
  const project = await resolveProject(value.root);
  const state = JSON.parse(await readFile(project.paths.state, "utf8"));
  state.run = effectiveRun("codex");
  state.run.ownerSessionId = "native-owner";
  await writeFile(project.paths.state, `${JSON.stringify(state, null, 2)}\n`);
  const tracker = await readTracker(project);
  const evidencePath = path.join(project.paths.stateRoot, "gate-evidence.json");
  await writeFile(evidencePath, `${JSON.stringify({ status: "passed", revision: value.revision, taskIds: ["AT-001"],
    requirementsReconciled: true, review: { status: "passed", revision: value.revision, taskId: "AT-001" },
    checks: [{ name: "unit", status: "passed", revision: value.revision, taskId: "AT-001" }] }, null, 2)}\n`, { mode: 0o600 });
  const gateRequestPath = path.join(project.paths.stateRoot, "gate.json");
  await writeFile(gateRequestPath, `${JSON.stringify({ schemaVersion: 1, actorSessionId: "native-owner", expectedVersion: state.stateVersion,
    request: { operationId: "native-gate-after-adoption", gate: "completion", taskIds: ["AT-001"],
      expectedFingerprint: tracker.tracker.fingerprint, expectedRevision: value.revision, evidencePath } }, null, 2)}\n`);
  const command = `node ${cli} gate-evidence --project ${value.root} --request ${gateRequestPath}`;
  const result = await runNormalizedHook(normalizeEvent("codex", "PreToolUse", { cwd: value.root, session_id: "native-owner",
    event_id: "gate-invocation", tool_name: "exec_command", tool_input: { cmd: command } }));
  assert.equal(result.decision.allow, true, JSON.stringify(result.decision));
  assert.equal(result.decision.mutations.some((entry) => entry.kind === "gate_evidence" && entry.status === "applied"), true,
    JSON.stringify(result.decision));
  const stored = JSON.parse(await readFile(project.paths.state, "utf8"));
  assert.equal(stored.completion.taskId, "AT-001");
  assert.equal(stored.completion.requirementsReconciled, true);
});

for (const command of ["run-reconcile", "run-scope-extend"]) {
  for (const [runtime, host] of [["codex", "codex"], ["claude", "claude-code"]]) {
    test(`${runtime} native PreToolUse derives owner identity for ${command}`, async () => {
      const value = await ownerRunFixture({ scopeTask: command === "run-scope-extend", host });
      const invocation = `node ${cli} ${command} --project ${value.root} --request ${value.requestPath}`;
      const result = await runNormalizedHook(normalizeEvent(runtime, "PreToolUse", { cwd: value.root,
        session_id: "owner-session", event_id: `${runtime}-${command}`, tool_name: "exec_command", tool_input: { cmd: invocation } }));
      assert.equal(result.decision.allow, true, JSON.stringify(result.decision));
      assert.equal(result.decision.mutations.some((entry) => entry.command === command && entry.status === "applied"), true,
        JSON.stringify(result.decision));
      const stored = JSON.parse(await readFile(value.project.paths.state, "utf8"));
      if (command === "run-reconcile") assert.equal(stored.run.operationalVersion, stored.stateVersion);
      else assert.deepEqual(stored.run.taskIds, ["AT-001", "AT-002"]);
    });
  }

  test(`exported runCommand rejects caller-supplied native identity for ${command}`, async () => {
    const value = await ownerRunFixture({ scopeTask: command === "run-scope-extend" });
    const before = await readFile(value.project.paths.state);
    const result = await runCommand(command, { project: value.root, request: value.requestPath }, { nativeIdentity: {
      host: "codex", sessionId: "owner-session", observed: true, cwd: value.root, ownershipEpoch: 1,
    } });
    assert.deepEqual(result, { status: "conflict", reason: "native_hook_identity_required" });
    assert.deepEqual(await readFile(value.project.paths.state), before);
  });
}

test('checkpoint events without native IDs remain distinct and complete batch IDs cannot collide', async () => {
  const value = await fixture();
  const payload = { cwd: value.feature, session_id: 'developer-session' };
  assert.equal(invoke('codex', 'PreCompact', payload, value.home).status, 0);
  const file = path.join(value.root, '.agent-team/checkpoints/developer-session.json');
  const first = JSON.parse(await readFile(file, 'utf8'));
  assert.equal(invoke('codex', 'PreCompact', payload, value.home).status, 0);
  const second = JSON.parse(await readFile(file, 'utf8'));
  assert.notEqual(second.eventId, first.eventId);
  assert.equal(second.version, first.version + 1);
  const calls = Array.from({ length: 21 }, (_, index) => ({ tool_use_id: `tool-${index}` }));
  const a = normalizeEvent('claude', 'PostToolBatch', { ...payload, tool_calls: calls });
  const b = normalizeEvent('claude', 'PostToolBatch', { ...payload, tool_calls: [...calls.slice(0, 20), { tool_use_id: 'different' }] });
  assert.notEqual(a.eventId, b.eventId);
  assert.equal(a.eventId, normalizeEvent('claude', 'PostToolBatch', { ...payload, tool_calls: calls }).eventId);
  const partial = { ...payload, tool_calls: [{ tool_use_id: 'known' }, {}] };
  assert.notEqual(normalizeEvent('claude', 'PostToolBatch', partial).eventId, normalizeEvent('claude', 'PostToolBatch', partial).eventId);
});

test('a factual checkpoint refreshes an explicitly enabled snapshot without changing task authority', async () => {
  const value = await fixture();
  const setupPath = path.join(value.root, '.agent-team/setup.json');
  const setup = JSON.parse(await readFile(setupPath, 'utf8'));
  setup.dashboard = { snapshot: true };
  await writeFile(setupPath, JSON.stringify(setup));
  const trackerPath = path.join(value.root, '.agent-team/TASKS.md');
  const before = await readFile(trackerPath, 'utf8');
  assert.equal(invoke('codex', 'PreCompact', { cwd: value.feature, session_id: 'developer-session' }, value.home).status, 0);
  assert.match(await readFile(path.join(value.root, '.agent-team/dashboard/index.html'), 'utf8'), /LOCAL STATUS SNAPSHOT/);
  assert.equal(await readFile(trackerPath, 'utf8'), before);
});

test('a canonical change refreshes v8 snapshots without the legacy dashboard flag', async () => {
  const value = await fixture();
  await writeFile(path.join(value.root, '.agent-team/setup.json'), JSON.stringify({
    schema: 1, authority: 'v8', project: value.root, receiptDigest: 'a'.repeat(64),
    receiptPath: '.agent-team/v8/authority.json', revision: value.revision,
    tracker: { kind: 'beads', executable: boundedBeads, parentId: 'AT-ROOT', taskIds: ['AT-001', 'AT-ROOT'], fingerprint: 'b'.repeat(64) },
  }));
  await writeFile(path.join(value.root, '.agent-team/state.json'), JSON.stringify({
    schema: 1, revision: value.revision,
    integration: { authorized: true, hold: false, receiptDigest: 'a'.repeat(64) },
    release: { authorized: true, hold: false, receiptDigest: 'a'.repeat(64) },
  }));
  const result = invoke('codex', 'PostToolUse', {
    cwd: value.root,
    session_id: 'developer-session',
    tool_name: 'apply_patch',
    tool_input: { command: '*** Begin Patch\n*** Update File: .agent-team/state.json\n+ observed change\n*** End Patch' },
  }, value.home);
  assert.equal(result.status, 0);
  assert.match(await readFile(path.join(value.root, '.agent-team/dashboard/index.html'), 'utf8'), /LOCAL STATUS SNAPSHOT/);
  assert.match(await readFile(path.join(value.root, '.agent-team/TEAMS.md'), 'utf8'), new RegExp(`Revision: ${value.revision}`));
});

for (const blockedAt of ['discovery', 'recording']) {
  test(`optional hook evidence ${blockedAt} cannot turn a mapped denial into allow`, async () => {
    const value = await fixture();
    const packageRoot = path.join(value.root, '.agents/skills/agent-team');
    const directory = path.join(value.root, '.agent-team-hooks');
    await mkdir(directory);
    const receipt = path.join(directory, 'install.json');
    let release;
    if (blockedAt === 'discovery') {
      assert.equal(spawnSync('mkfifo', [receipt]).status, 0);
      // Release the genuine FIFO reader even in the RED implementation.
      release = new Promise((resolve, reject) => setTimeout(() => writeFile(receipt, '{}').then(resolve, reject), 500));
    } else {
      await writeFile(receipt, JSON.stringify({ targets: [{ runtime: 'codex', path: packageRoot }] }));
      await mkdir(path.join(directory, '.hook-evidence.lock'));
    }
    try {
      const event = normalizeEvent('codex', 'PreToolUse', { cwd: value.feature, session_id: 'developer-session',
        tool_name: 'mcp__filesystem__write', tool_input: { path: path.join(value.root, '.agent-team/TASKS.md'), content: '| AT-001 | TEAM-001 | verified |' } });
      const result = await runNormalizedHook(event, { timeoutMs: 200, evidencePackageRoot: packageRoot });
      assert.equal(result.decision.allow, false, 'optional observation must never discard required enforcement');
    } finally { if (release) await release; }
  });
}

test('actual copied hook under a spaced installation path executes and records only its scoped transport', async () => {
  const directory = await mkdtemp(path.join(os.tmpdir(), 'agent team hook consumer '));
  temporary.push(directory);
  const value = await policyFixture(path.join(directory, 'project'));
  const packageRoot = path.join(value.root, '.agents/skills/agent-team');
  await mkdir(packageRoot, { recursive: true });
  await cp(path.dirname(hook), path.join(packageRoot, 'hooks'), { recursive: true });
  await mkdir(path.join(value.root, '.agent-team-hooks'));
  await writeFile(path.join(value.root, '.agent-team-hooks/install.json'), JSON.stringify({ targets: [{ runtime: 'codex', path: packageRoot }] }));
  const isolatedHome = path.join(directory, 'home');
  await mkdir(isolatedHome);
  const result = spawnSync(process.execPath, [path.join(packageRoot, 'hooks/agent-team-hook.mjs'), '--runtime', 'codex', '--event', 'PreToolUse'], {
    input: JSON.stringify({ cwd: value.feature, session_id: 'owner-session', tool_name: 'exec_command', tool_input: { cmd: 'git status --short' } }),
    encoding: 'utf8', env: { ...process.env, HOME: isolatedHome },
  });
  assert.equal(result.status, 0, result.stderr);
  assert.ok(result.stdout.trim(), 'copied hook must execute its main function');
  const events = JSON.parse(await readFile(path.join(value.root, '.agent-team-hooks/hook-events.json'), 'utf8'));
  assert.equal(events['codex:PreToolUse'].source, 'packaged_entrypoint');
  await assert.rejects(readFile(path.join(isolatedHome, '.agent-team-hooks/hook-events.json')), { code: 'ENOENT' });
});

function mappingDigest(value) {
  return createHash("sha256").update(JSON.stringify(value)).digest("hex");
}

function invokeCli(command, options) {
  const args = [cli, command];
  for (const [name, value] of Object.entries(options)) args.push(`--${name}`, value);
  const result = spawnSync(process.execPath, args, { encoding: "utf8" });
  return { status: result.status, output: JSON.parse(result.stdout), stderr: result.stderr };
}

for (const runtime of ["codex", "claude"]) {
  test(`${runtime} mapped-provider tracker completion cannot write outside its active checkout (synthetic host payload)`, async () => {
    const value = await fixture();
    value.state.run = effectiveRun(runtime === "claude" ? "claude-code" : "codex");
    await writeFile(path.join(value.root, ".agent-team/state.json"), JSON.stringify(value.state));
    const result = invoke(runtime, "PreToolUse", { cwd: value.feature, session_id: "developer-session",
      tool_name: "mcp__filesystem__write", tool_input: { path: path.join(value.root, ".agent-team/TASKS.md"), content: "| AT-001 | TEAM-001 | verified |" },
    }, value.home);
    assert.equal(result.status, 0);
    assert.equal(output(result).hookSpecificOutput.permissionDecision, "deny");
    assert.match(output(result).hookSpecificOutput.permissionDecisionReason, /outside the active project checkout/i);
  });
  test(`${runtime} subprocess stdout exposes actionable lint failure while permitting repair (synthetic host payload)`, async () => {
    const value = await fixture();
    const bin = path.join(value.feature, "node_modules/.bin/eslint");
    await mkdir(path.dirname(bin), { recursive: true });
    await writeFile(bin, "#!/usr/bin/env node\nprocess.stdout.write('src/owned.js:4:7 no-undef missingName\\n' + 'detail '.repeat(10000)); process.exitCode=1;\n");
    await chmod(bin, 0o755);
    const result = invoke(runtime, "PostToolUse", {
      cwd: value.feature, session_id: "developer-session",
      tool_name: runtime === "codex" ? "apply_patch" : "Edit",
      tool_input: runtime === "codex"
        ? { command: "*** Begin Patch\n*** Update File: src/owned.js\n+missingName();\n*** End Patch" }
        : { file_path: "src/owned.js", old_string: "export {};", new_string: "missingName();" },
    }, value.home);
    assert.equal(result.status, 0);
    assert.match(result.stdout, /owned.js:4:7.*no-undef.*missingName/);
    assert.match(result.stdout, /lint failed/i);
    assert.match(result.stdout, /log/i);
    assert.ok(Buffer.byteLength(result.stdout) < 7000);
    assert.doesNotMatch(result.stdout, /permissionDecision.*deny/);
  });
}

test("entrypoint emits native denial when project resolution fails for a critical command", async () => {
  // This test catches the top-level exception handler exiting successfully without a deny result.
  const home = await mkdtemp(path.join(os.tmpdir(), "agent-team-entry-home-"));
  temporary.push(home);
  const result = invoke("codex", "PreToolUse", {
    cwd: path.join(home, "missing-project"),
    session_id: "owner-session",
    tool_name: "exec_command",
    tool_input: { cmd: "git push origin main" },
  }, home);

  assert.equal(result.status, 0);
  assert.equal(output(result).hookSpecificOutput.permissionDecision, "deny");
  assert.match(output(result).hookSpecificOutput.permissionDecisionReason, /unavailable/i);
});

test("LeanCTX shell paths retain Agent-Team critical-operation coverage", async () => {
  // This catches ctx_shell or the explicit CLI wrapper hiding an integration command from Agent-Team.
  const home = await mkdtemp(path.join(os.tmpdir(), "agent-team-entry-home-"));
  temporary.push(home);
  const cwd = path.join(home, "missing-project");
  const results = [
    invoke("claude", "PreToolUse", {
      cwd,
      session_id: "owner-session",
      tool_name: "mcp__lean-ctx__ctx_shell",
      tool_input: { command: "git push origin main" },
    }, home),
    invoke("codex", "PreToolUse", {
      cwd,
      session_id: "owner-session",
      tool_name: "exec_command",
      tool_input: { cmd: 'lean-ctx -c --raw "git push origin main"' },
    }, home),
    invoke("codex", "PreToolUse", {
      cwd,
      session_id: "owner-session",
      tool_name: "exec_command",
      tool_input: { cmd: 'lean-ctx raw "git push origin main"' },
    }, home),
    invoke("codex", "PreToolUse", {
      cwd,
      session_id: "owner-session",
      tool_name: "exec_command",
      tool_input: { cmd: 'lean-ctx.exe -c --raw "git push origin main"' },
    }, home),
  ];

  for (const result of results) {
    assert.equal(result.status, 0);
    assert.equal(output(result).hookSpecificOutput.permissionDecision, "deny");
    assert.match(output(result).hookSpecificOutput.permissionDecisionReason, /unavailable/i);
  }
});

test("entrypoint uses the separate mapping inventory when operational state is malformed", async () => {
  // This test catches fallback classification that forgets separately readable critical mappings.
  const value = await fixture();
  await writeFile(path.join(value.root, ".agent-team", "state.json"), "{bad json}\n");
  const provider = invoke("claude", "PreToolUse", {
    cwd: value.feature,
    session_id: "owner-session",
    tool_name: "mcp__database__execute",
    tool_input: { query: "DROP TABLE records" },
  }, value.home);
  const app = invoke("codex", "PreToolUse", {
    cwd: value.feature,
    session_id: "owner-session",
    tool_name: "exec_command",
    tool_input: { cmd: "node scripts/reset-data.mjs" },
  }, value.home);

  assert.equal(output(provider).hookSpecificOutput.permissionDecision, "deny");
  assert.equal(output(app).hookSpecificOutput.permissionDecision, "deny");
});

test("entrypoint leaves ordinary and unmapped read-only operations available when state is malformed", async () => {
  // This test catches an unavailable-state fallback that treats every shell or provider call as critical.
  const value = await fixture();
  await writeFile(path.join(value.root, ".agent-team", "state.json"), "{bad json}\n");
  const events = [
    invoke("codex", "PreToolUse", {
      cwd: value.feature,
      session_id: "owner-session",
      tool_name: "exec_command",
      tool_input: { cmd: "git status --short" },
    }, value.home),
    invoke("codex", "PreToolUse", {
      cwd: value.feature,
      session_id: "owner-session",
      tool_name: "exec_command",
      tool_input: { cmd: "echo ready" },
    }, value.home),
    invoke("claude", "PreToolUse", {
      cwd: value.feature,
      session_id: "owner-session",
      tool_name: "mcp__lean-ctx__ctx_shell",
      tool_input: { command: "git status --short" },
    }, value.home),
    invoke("claude", "PreToolUse", {
      cwd: value.feature,
      session_id: "owner-session",
      tool_name: "mcp__catalog__list_records",
      tool_input: { limit: 5 },
    }, value.home),
  ];

  for (const result of events) {
    assert.equal(result.status, 0);
    assert.equal(output(result).hookSpecificOutput.permissionDecision, undefined);
    assert.match(output(result).hookSpecificOutput.additionalContext, /advisory checks.*unavailable/i);
  }
});

test("entrypoint reports a missing cache and denies only static critical operations when state is malformed", async () => {
  // This test catches a missing fallback cache being described as mapped-operation protection.
  const value = await fixture();
  await rm(path.join(value.root, ".agent-team", "operation-mappings.json"));
  await writeFile(path.join(value.root, ".agent-team", "state.json"), "{bad json}\n");
  const mapped = invoke("claude", "PreToolUse", {
    cwd: value.feature,
    session_id: "owner-session",
    tool_name: "mcp__database__execute",
    tool_input: { query: "DROP TABLE records" },
  }, value.home);
  const staticCritical = invoke("codex", "PreToolUse", {
    cwd: value.feature,
    session_id: "owner-session",
    tool_name: "exec_command",
    tool_input: { cmd: "git push origin feature" },
  }, value.home);

  assert.equal(output(mapped).hookSpecificOutput.permissionDecision, undefined);
  assert.match(output(mapped).hookSpecificOutput.additionalContext, /mapping cache.*missing.*protection.*unavailable/i);
  assert.equal(output(staticCritical).hookSpecificOutput.permissionDecision, "deny");
  assert.match(output(staticCritical).hookSpecificOutput.permissionDecisionReason, /mapping cache.*missing/i);
});

test("entrypoint reports an invalid cache without treating its mappings as protected", async () => {
  // This test catches malformed cache bytes being accepted as critical mapping evidence.
  const value = await fixture();
  await writeFile(path.join(value.root, ".agent-team", "operation-mappings.json"), "{bad json}\n");
  await writeFile(path.join(value.root, ".agent-team", "state.json"), "{bad json}\n");
  const mapped = invoke("codex", "PreToolUse", {
    cwd: value.feature,
    session_id: "owner-session",
    tool_name: "exec_command",
    tool_input: { cmd: "node scripts/reset-data.mjs" },
  }, value.home);

  assert.equal(output(mapped).hookSpecificOutput.permissionDecision, undefined);
  assert.match(output(mapped).hookSpecificOutput.additionalContext, /mapping cache.*invalid.*protection.*unavailable/i);
});

test("standalone CLI does not expose a mapping cache write command", async () => {
  // This test catches a status or setup command becoming a second cache-content input.
  const value = await fixture();
  const cache = path.join(value.root, ".agent-team", "operation-mappings.json");
  await rm(cache);
  const migration = invokeCli("migrate-mappings", {
    project: value.feature,
  });

  assert.equal(migration.status, 1);
  assert.equal(migration.output.status, "failed");
  assert.match(migration.output.error, /unknown command/i);
  await assert.rejects(readFile(cache), { code: "ENOENT" });
});

test("caller-controlled hook fields cannot inject or alter mapping cache content", async () => {
  // This test catches runtime, event, session, or payload fields becoming mapping input or authority.
  const value = await fixture();
  const cache = path.join(value.root, ".agent-team", "operation-mappings.json");
  const state = structuredClone(value.state);
  state.operationMappings.providers.mcp__canonical__purge = { kind: "database_destructive", sqlField: "query" };
  await writeFile(path.join(value.root, ".agent-team", "state.json"), JSON.stringify(state, null, 2));
  const forged = invoke("claude", "SessionStart", {
    cwd: value.feature,
    runtime: "codex",
    event: "PostToolUse",
    session_id: "forged-owner-session",
    event_id: "forged-cache-write",
    operationMappings: {
      providers: { mcp__injected__destroy: { kind: "database_destructive", sqlField: "query" } },
      shell: [],
    },
  }, value.home);

  assert.equal(forged.status, 0);
  const receipt = JSON.parse(await readFile(cache, "utf8"));
  assert.equal(mappingDigest(receipt.operationMappings), mappingDigest(state.operationMappings));
  assert.equal(receipt.operationMappings.providers.mcp__canonical__purge.kind, "database_destructive");
  assert.equal(receipt.operationMappings.providers.mcp__injected__destroy, undefined);
});

test("a non-owner lifecycle event reproduces the exact canonical mapping digest", async () => {
  // This test catches cache refresh being gated by an unsigned session ID or changing canonical mappings.
  const value = await fixture();
  const cache = path.join(value.root, ".agent-team", "operation-mappings.json");
  await rm(cache);
  const first = invoke("codex", "SessionStart", {
    cwd: value.feature,
    session_id: "developer-session",
    event_id: "team-start",
  }, value.home);
  const firstSource = await readFile(cache, "utf8");
  const second = invoke("claude", "SessionStart", {
    cwd: value.feature,
    session_id: "unknown-session",
    event_id: "unknown-start",
  }, value.home);

  assert.equal(first.status, 0);
  assert.match(output(first).hookSpecificOutput.additionalContext, /mapping cache.*updated/i);
  assert.equal(second.status, 0);
  assert.match(output(second).hookSpecificOutput.additionalContext, /mapping cache.*current/i);
  assert.equal(await readFile(cache, "utf8"), firstSource);
  const receipt = JSON.parse(await readFile(cache, "utf8"));
  assert.equal(receipt.schemaVersion, 1);
  assert.equal(receipt.kind, "agent-team-operation-mapping-cache");
  assert.equal(receipt.authoritative, false);
  assert.equal(receipt.projectId, "project-1");
  assert.equal(receipt.sourcePath, ".agent-team/state.json");
  assert.equal(mappingDigest(receipt.operationMappings), mappingDigest(value.operationMappings));
});

test("malformed canonical state cannot update an existing mapping cache", async () => {
  // This test catches unvalidated state or unsigned payload content replacing the last valid cache.
  const value = await fixture();
  const cache = path.join(value.root, ".agent-team", "operation-mappings.json");
  const before = await readFile(cache, "utf8");
  await writeFile(path.join(value.root, ".agent-team", "state.json"), "{}\n");
  const result = invoke("codex", "SessionStart", {
    cwd: value.feature,
    session_id: "forged-owner-session",
    event_id: "malformed-state-start",
    operationMappings: { providers: {}, shell: [] },
  }, value.home);

  assert.equal(result.status, 0);
  assert.equal(await readFile(cache, "utf8"), before);
  assert.match(output(result).hookSpecificOutput.additionalContext, /mapping cache refresh.*unavailable.*unchanged/i);
  assert.doesNotMatch(output(result).hookSpecificOutput.additionalContext, /project owner/i);
});

test("mapped provider and app operations deny after lifecycle cache refresh", async () => {
  // This test catches a lifecycle cache refresh that writes mappings the fallback cannot consume.
  const value = await fixture();
  const cache = path.join(value.root, ".agent-team", "operation-mappings.json");
  await rm(cache);
  const migration = invoke("claude", "SessionStart", {
    cwd: value.feature,
    session_id: "developer-session",
    event_id: "cache-refresh",
  }, value.home);
  assert.equal(migration.status, 0);
  assert.match(String(output(migration).hookSpecificOutput.additionalContext ?? ""), /mapping cache/i);
  assert.equal(JSON.parse(await readFile(cache, "utf8")).projectId, "project-1");
  await writeFile(path.join(value.root, ".agent-team", "state.json"), "{bad json}\n");
  const provider = invoke("claude", "PreToolUse", {
    cwd: value.feature,
    session_id: "owner-session",
    tool_name: "mcp__database__execute",
    tool_input: { query: "DROP TABLE records" },
  }, value.home);
  const app = invoke("codex", "PreToolUse", {
    cwd: value.feature,
    session_id: "owner-session",
    tool_name: "exec_command",
    tool_input: { cmd: "node scripts/reset-data.mjs" },
  }, value.home);
  assert.equal(output(provider).hookSpecificOutput.permissionDecision, "deny");
  assert.equal(output(app).hookSpecificOutput.permissionDecision, "deny");
});

test("state-file lifecycle events update the cache while read-only tools do not create it", async () => {
  // This test catches automatic cache writes on status events or stale cache after a canonical state change.
  const value = await fixture();
  const cache = path.join(value.root, ".agent-team", "operation-mappings.json");
  await rm(cache);
  const status = invoke("codex", "PreToolUse", {
    cwd: value.root,
    session_id: "developer-session",
    tool_name: "exec_command",
    tool_input: { cmd: "git status --short" },
  }, value.home);
  await assert.rejects(readFile(cache), { code: "ENOENT" });
  const initialMigration = invoke("codex", "SessionStart", {
    cwd: value.feature,
    session_id: "developer-session",
    event_id: "team-start-before-update",
  }, value.home);
  assert.equal(initialMigration.status, 0);
  assert.match(String(output(initialMigration).hookSpecificOutput.additionalContext ?? ""), /mapping cache/i);

  const state = structuredClone(value.state);
  state.operationMappings.providers.mcp__records__purge = { kind: "database_destructive", sqlField: "query" };
  await writeFile(path.join(value.root, ".agent-team", "state.json"), JSON.stringify(state, null, 2));
  const refresh = invoke("codex", "PostToolUse", {
    cwd: value.root,
    session_id: "developer-session",
    tool_name: "apply_patch",
    tool_input: { command: "*** Begin Patch\n*** Update File: .agent-team/state.json\n+ mapping changed\n*** End Patch" },
  }, value.home);
  assert.equal(status.status, 0);
  assert.equal(refresh.status, 0);
  assert.match(String(output(refresh).hookSpecificOutput.additionalContext ?? ""), /mapping cache/i);
  const receipt = JSON.parse(await readFile(cache, "utf8"));
  assert.equal(receipt.operationMappings.providers.mcp__records__purge.kind, "database_destructive");
  assert.equal((await readdir(path.join(value.root, ".agent-team", ".locks"))).includes("operation-mappings.lock"), false);
  assert.equal((await readdir(path.join(value.root, ".agent-team"))).some((name) => name.endsWith(".tmp")), false);
});

test("entrypoint keeps checkpoint and telemetry failures visible and non-blocking", async () => {
  // This test catches advisory mutation failures escaping to a silent successful process.
  const value = await fixture();
  await writeFile(path.join(value.root, ".agent-team", "checkpoints"), "not a directory\n");
  const checkpoint = invoke("codex", "PreCompact", {
    cwd: value.feature,
    session_id: "developer-session",
    event_id: "compact-failure",
  }, value.home);
  await mkdir(path.join(value.home, ".agent-team-hooks"), { recursive: true });
  await writeFile(path.join(value.home, ".agent-team-hooks", "logs"), "not a directory\n");
  const telemetry = invoke("claude", "PreToolUse", {
    cwd: value.feature,
    session_id: "developer-session",
    tool_name: "Skill",
    tool_input: { skill: "agent-team" },
    tool_use_id: "activation-failure",
  }, value.home);

  assert.equal(checkpoint.status, 0);
  assert.match(output(checkpoint).systemMessage, /checkpoint.*unavailable/i);
  assert.equal(telemetry.status, 0);
  assert.match(output(telemetry).hookSpecificOutput.additionalContext, /activation logging.*unavailable/i);
});

test("entrypoint checkpoints canonical Git, identity, task, evidence, and pending-operation facts", async () => {
  // This test catches an entrypoint that supplies only paths to the checkpoint writer.
  const value = await fixture();
  await mkdir(path.join(value.root, ".agent-team", "checkpoints"), { recursive: true });
  await writeFile(path.join(value.root, ".agent-team", "checkpoints", "developer-session.json"), JSON.stringify({
    eventId: "old-event",
    sessionId: "developer-session",
    nextAction: "Run the owner handoff.",
    decisionNotes: "Keep the recorded release boundary.",
    updatedAt: "2026-09-06T12:00:00.000Z",
  }));
  const result = invoke("codex", "PreCompact", {
    cwd: value.feature,
    session_id: "developer-session",
    event_id: "compact-facts",
  }, value.home);
  const stored = JSON.parse(await readFile(path.join(value.root, ".agent-team", "checkpoints", "developer-session.json"), "utf8"));

  assert.equal(result.status, 0);
  assert.equal(stored.branch, "feature");
  assert.equal(stored.revision, value.revision);
  assert.equal(stored.teamId, "TEAM-001");
  assert.deepEqual(stored.taskIds, ["AT-001"]);
  assert.equal(stored.evidence.git, "current");
  assert.equal(stored.pendingOperations.some(({ kind }) => kind === "integration"), true);
  assert.equal(stored.nextAction, "Run the owner handoff.");
  assert.equal(stored.decisionNotes, "Keep the recorded release boundary.");
});

test("entrypoint activation log records registered team provenance", async () => {
  // This test catches activation records built before canonical identity is loaded.
  const value = await fixture();
  const result = invoke("claude", "PreToolUse", {
    cwd: value.feature,
    session_id: "developer-session",
    tool_name: "Skill",
    tool_input: { skill: "agent-team" },
    tool_use_id: "activation-team",
  }, value.home);
  const record = JSON.parse((await readFile(path.join(value.home, ".agent-team-hooks", "logs", "activation.jsonl"), "utf8")).trim());

  assert.equal(result.status, 0);
  assert.equal(record.projectId, "project-1");
  assert.equal(record.teamId, "TEAM-001");
  assert.equal(record.identityKind, "team");
});

test("direct Claude slash activation logs outside initialized Agent-Team projects", async () => {
  // This test catches global slash activation logging being coupled to project-scoped policy state.
  const value = await inactiveFixture();
  const result = invoke("claude", "UserPromptExpansion", {
    cwd: value.root,
    session_id: "global-slash-session",
    event_id: "global-slash-event",
    command_name: "agent-team",
    prompt: "/agent-team help with private context",
  }, value.home);
  const { records } = await readActivationLogs(path.join(value.home, ".agent-team-hooks", "logs"));

  assert.equal(result.status, 0);
  assert.equal(output(result).hookSpecificOutput.permissionDecision, undefined);
  assert.equal(records.length, 1);
  assert.equal(records[0].runtime, "claude");
  assert.equal(records[0].eventKind, "UserPromptExpansion");
  assert.equal(records[0].identityKind, "unregistered");
  assert.equal(records[0].projectId, undefined);
  assert.equal(records[0].teamId, undefined);
  assert.equal(JSON.stringify(records[0]).includes("private context"), false);
});

test("Claude Skill activation logs outside initialized Agent-Team projects", async () => {
  // This test catches global model-invoked activation logging being coupled to project state.
  const value = await inactiveFixture();
  const result = invoke("claude", "PreToolUse", {
    cwd: value.root,
    session_id: "global-skill-session",
    tool_name: "Skill",
    tool_input: { skill: "agent-team", prompt: "private skill input" },
    tool_use_id: "global-skill-event",
  }, value.home);
  const { records } = await readActivationLogs(path.join(value.home, ".agent-team-hooks", "logs"));

  assert.equal(result.status, 0);
  assert.equal(output(result).hookSpecificOutput.permissionDecision, undefined);
  assert.equal(records.length, 1);
  assert.equal(records[0].runtime, "claude");
  assert.equal(records[0].eventKind, "PreToolUse");
  assert.equal(records[0].identityKind, "unregistered");
  assert.equal(records[0].projectId, undefined);
  assert.equal(records[0].teamId, undefined);
  assert.equal(JSON.stringify(records[0]).includes("private skill input"), false);
});

test("Claude PostToolBatch runs one changed-file check and writes one factual checkpoint", async () => {
  // This test checks the native batch path rather than a per-edit PostToolUse path.
  const value = await fixture();
  const result = invoke("claude", "PostToolBatch", {
    cwd: value.feature,
    session_id: "developer-session",
    tool_calls: [
      { tool_name: "Write", tool_input: { file_path: "src/one.js", content: "export {};" }, tool_use_id: "tool-one" },
      { tool_name: "Edit", tool_input: { file_path: "src/two.js", old_string: "old", new_string: "new" }, tool_use_id: "tool-two" },
    ],
  }, value.home);
  const checkpoint = JSON.parse(await readFile(path.join(value.root, ".agent-team", "checkpoints", "developer-session.json"), "utf8"));

  assert.equal(result.status, 0);
  assert.equal(checkpoint.eventId, "batch:tool-one,tool-two");
  assert.equal(checkpoint.eventKind, "PostToolBatch");
  assert.equal(checkpoint.revision, value.revision);
  assert.match(output(result).additionalContext, /lint skipped/i);
});
