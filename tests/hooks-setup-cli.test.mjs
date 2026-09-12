import assert from "node:assert/strict";
import { execFile } from "node:child_process";
import { chmod, mkdir, mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import { pathToFileURL } from "node:url";
import { promisify } from "node:util";
import test from "node:test";
import { initializeProject } from "../hooks/lib/initialization.mjs";

const exec = promisify(execFile);
const modulePath = path.resolve(import.meta.dirname, "../hooks/lib/setup-cli.mjs");
const nativeChoices = { enforceable: false, models: [
  { id: "quality", label: "Quality", efforts: ["high"], recommended: true, available: true },
  { id: "fast", label: "Fast", efforts: ["low"], available: true },
] };

async function fixture(t, overrides = {}, { trackerKind = "markdown", projectId = "project-1" } = {}) {
  const root = await mkdtemp(path.join(os.tmpdir(), "agent team setup cli "));
  t.after(() => rm(root, { recursive: true, force: true }));
  await exec("git", ["init", "--quiet", "-b", "develop", root]);
  const trackerExecutable = trackerKind === "beads" ? path.join(root, "selected-bd") : undefined;
  if (trackerExecutable) {
    await mkdir(path.join(root, ".beads"));
    await writeFile(trackerExecutable, "bounded Beads fixture\n");
  }
  const tasks = [
    { id: "T-1", title: "Complete prerequisite", status: "todo", dependencies: [] },
    { id: "T-2", title: "Repair parser", status: "blocked", dependencies: ["T-1"] },
  ];
  const initialized = await initializeProject(root, {
    projectId, operationId: "initialize-fixture", source: trackerExecutable ? "existing" : "standalone",
    tracker: trackerExecutable ? { kind: "beads", executable: trackerExecutable } : { kind: "markdown", path: ".agent-team/TASKS.md" },
    plan: {
      scope: "Repair parser", acceptance: ["Regression passes"], branch: "develop", verification: ["node --test"],
      authority: { ownedPaths: ["src/**"] },
      tasks,
    },
  }, { actorSessionId: "owner", nativeIdentity: { host: "codex", sessionId: "owner", observed: true, cwd: root }, ...(trackerExecutable ? { runBeads: async () => ({ stdout: JSON.stringify([
    { id: "T-1", title: tasks[0].title, status: "closed", assignee: "", dependency_count: 0 },
    { id: "T-2", title: tasks[1].title, status: "open", assignee: "", dependency_count: 1,
      dependencies: [{ type: "blocks", depends_on_id: "T-1" }] },
  ]) }) } : {}) });
  assert.equal(initialized.status, "applied", initialized.reason);
  const setupPath = path.join(root, ".agent-team", "setup.json");
  const initializedSetup = JSON.parse(await readFile(setupPath, "utf8"));
  const setup = {
    ...initializedSetup, version: 3,
    settings: { custom: "keep", hosts: { "claude-code": { roles: { developer: { model: "custom", effort: "high" } } } } },
    dashboard: { custom: "keep", graph: { custom: "keep", enabled: false, termsAcknowledged: false } },
    dependencies: { hosts: { codex: { scope: "project", receipts: [
      { id: "serena", functional: "passed", availableToWorker: "passed", status: "ready" },
      { id: "playwright-cli", functional: "passed", availableToWorker: "passed", status: "ready" },
    ] } } },
    ...overrides,
  };
  await writeFile(setupPath, `${JSON.stringify(setup, null, 2)}\n`);
  const tasksPath = path.join(root, ".agent-team", "TASKS.md");
  const statePath = path.join(root, ".agent-team", "state.json");
  if (!trackerExecutable) await writeFile(tasksPath, "| ID | Owner | Status | Depends on |\n| --- | --- | --- | --- |\n| T-1 | - | closed | - |\n| T-2 | - | ready | T-1 |\n");
  const options = { project: root, host: "codex", scope: "project" };
  const consumer = path.join(root, "consumer.mjs");
  await writeFile(consumer, `import { runSetupCommand } from ${JSON.stringify(pathToFileURL(modulePath).href)};\ntry { console.log(JSON.stringify(await runSetupCommand(process.argv[2], JSON.parse(process.argv[3])))); } catch (error) { console.log(JSON.stringify({ status: "failed", error: error.message })); process.exitCode = 1; }\n`);
  const invoke = async (command, extra = {}) => JSON.parse((await exec(process.execPath, [consumer, command, JSON.stringify({ ...options, ...extra })])).stdout);
  const nativeIdentity = { host: "codex", sessionId: "owner", observed: true, cwd: root, ownershipEpoch: 1 };
  return { root, setupPath, statePath, tasksPath, trackerExecutable, setup, options, invoke, nativeIdentity };
}

async function envelope(value, request, top = {}) {
  const file = path.join(value.root, `request-${Math.random().toString(16).slice(2)}.json`);
  await writeFile(file, JSON.stringify({ schemaVersion: 1, expectedVersion: 3, operationId: "operation-1", writer: { id: "owner", role: "project_orchestrator" }, request, ...top }));
  return file;
}

async function legacyBrainVaultFixture(t) {
  const value = await fixture(t, {}, { trackerKind: "beads", projectId: "brainvault-system" });
  await writeFile(value.trackerExecutable, `#!${process.execPath}\nprocess.stdout.write(JSON.stringify([\n  { id: "brainvault-system-6zf", title: "Historical delivery", status: "closed", assignee: "", dependency_count: 0 },\n  { id: "T-1", title: "Complete prerequisite", status: "closed", assignee: "", dependency_count: 0 },\n  { id: "T-2", title: "Repair parser", status: "open", assignee: "", dependency_count: 0 }\n]));\n`);
  await chmod(value.trackerExecutable, 0o755);
  const setup = JSON.parse(await readFile(value.setupPath, "utf8"));
  delete setup.initialization.initialTaskIds;
  delete setup.initialization.trackerFingerprint;
  delete setup.initialization.trackerSelection.root;
  delete setup.tracker.root;
  delete setup.ownership;
  await writeFile(value.setupPath, `${JSON.stringify(setup, null, 2)}\n`);
  const state = JSON.parse(await readFile(value.statePath, "utf8"));
  delete state.ownership;
  for (const gate of [state.integration, state.release]) { delete gate.ownerHost; delete gate.ownershipEpoch; }
  await writeFile(value.statePath, `${JSON.stringify(state, null, 2)}\n`);
  const teamsPath = path.join(value.root, ".agent-team", "TEAMS.md");
  await writeFile(teamsPath, (await readFile(teamsPath, "utf8")).replace(/^Project owner host:.*\n/m, "").replace(/^Integration owner host:.*\n/m, ""));
  await rm(path.join(value.root, ".agent-team", "owner-history.json"));
  return { ...value, legacyInitialization: JSON.stringify(setup.initialization) };
}

test("Node consumer inspects settings/dependencies without writes or invented native choices", async (t) => {
  const value = await fixture(t);
  const before = await readFile(value.setupPath, "utf8");
  const overview = await value.invoke("settings");
  assert.equal(overview.roles.length, 6);
  assert.equal(overview.roles[0].enforceable, "unknown");
  const menu = await value.invoke("settings", { role: "developer" });
  assert.deepEqual(menu.models, []);
  assert.equal((await value.invoke("dependencies")).receipts.length, 2);
  assert.equal(await readFile(value.setupPath, "utf8"), before);
});

test("menus use programmatic native facts and refresh the selected model's efforts", async (t) => {
  const { runSetupCommand } = await import(modulePath);
  const value = await fixture(t);
  const request = await envelope(value, { draft: { roles: { developer: { model: "fast" } } } });
  const wizard = await runSetupCommand("settings-wizard", { ...value.options, request }, { nativeChoices });
  const efforts = wizard.steps.find(({ kind, role }) => kind === "role-effort" && role === "developer");
  assert.deepEqual(efforts.choices.filter(({ id }) => !["keep_existing", "back", "cancel"].includes(id)).map(({ id }) => id), ["low"]);
});

test("dependency inspection reports scope mismatch without presenting another scope's ready counts", async (t) => {
  const value = await fixture(t);
  for (const recordedScope of ["user", "project"]) {
    value.setup.dependencies.hosts.codex.scope = recordedScope;
    await writeFile(value.setupPath, JSON.stringify(value.setup));
    const before = await readFile(value.setupPath, "utf8");
    const scope = recordedScope === "user" ? "project" : "user";
    assert.deepEqual(await value.invoke("dependencies", { scope }), {
      status: "unavailable", reason: "dependency_scope_mismatch", host: "codex", scope, recordedScope,
    });
    assert.equal((await value.invoke("dependencies", { scope: recordedScope })).receipts.length, 2);
    assert.equal(await readFile(value.setupPath, "utf8"), before);
  }
});

test("settings changes use canonical owner/version and semantic operation identity", async (t) => {
  const { runSetupCommand } = await import(modulePath);
  const value = await fixture(t);
  const request = await envelope(value, { change: { kind: "role", role: "developer", model: "quality", effort: "high" } });
  const applied = await runSetupCommand("settings-update", { ...value.options, request }, { nativeChoices, nativeIdentity: value.nativeIdentity });
  assert.equal(applied.status, "applied");
  assert.equal(applied.version, 4);
  assert.deepEqual(applied.setup.settings.hosts["claude-code"], value.setup.settings.hosts["claude-code"]);
  assert.equal(applied.setup.settings.custom, "keep");
  assert.equal((await runSetupCommand("settings-update", { ...value.options, request }, { nativeChoices, nativeIdentity: value.nativeIdentity })).status, "duplicate");
  const reused = await envelope(value, { change: { kind: "run", values: { continuous: true } } });
  assert.equal((await runSetupCommand("settings-update", { ...value.options, request: reused }, { nativeChoices, nativeIdentity: value.nativeIdentity })).reason, "operation_id_reused");
  const stale = await envelope(value, { change: { kind: "run", values: { continuous: true } } }, { operationId: "operation-2" });
  assert.equal((await runSetupCommand("settings-update", { ...value.options, request: stale }, { nativeChoices, nativeIdentity: value.nativeIdentity })).reason, "version_changed");
  const teamsPath = path.join(value.root, ".agent-team", "TEAMS.md");
  await writeFile(teamsPath, (await readFile(teamsPath, "utf8")).replace("Project owner: owner", "Project owner: replacement"));
  assert.equal((await runSetupCommand("settings-update", { ...value.options, request: stale }, { nativeChoices, nativeIdentity: value.nativeIdentity })).reason, "project_owner_required");
});

test("settings remain editable while the selected Beads tracker is unavailable", async (t) => {
  const { runSetupCommand } = await import(modulePath);
  const value = await fixture(t, {}, { trackerKind: "beads" });
  await rm(value.trackerExecutable);
  const request = await envelope(value, { change: { kind: "run", values: { continuous: true } } }, { operationId: "settings-during-outage" });

  const result = await runSetupCommand("settings-update", { ...value.options, request }, { nativeIdentity: value.nativeIdentity });

  assert.equal(result.status, "applied");
  assert.equal(result.setup.settings.runDefaults.continuous, true);
});

test("settings reject a tracker selection rewritten after initialization", async (t) => {
  const { runSetupCommand } = await import(modulePath);
  const value = await fixture(t);
  await writeFile(value.setupPath, JSON.stringify({ ...value.setup, tracker: { kind: "beads", executable: "/missing/selected-bd" } }));
  const request = await envelope(value, { change: { kind: "run", values: { continuous: true } } }, { operationId: "rewritten-tracker" });

  const result = await runSetupCommand("settings-update", { ...value.options, request }, { nativeIdentity: value.nativeIdentity });

  assert.deepEqual(result, { status: "conflict", reason: "project_owner_required" });
  assert.equal(JSON.parse(await readFile(value.setupPath, "utf8")).settings.runDefaults?.continuous, undefined);
});

test("Node consumer cannot mutate qualified settings or submit native capability claims", async (t) => {
  const value = await fixture(t);
  const request = await envelope(value, { change: { kind: "run", values: { continuous: true } } });
  assert.deepEqual(await value.invoke("settings-update", { request }), { status: "conflict", reason: "project_owner_required" });
  const forged = await envelope(value, { change: { kind: "role", role: "developer", model: "made-up", effort: "high" }, nativeChoices }, { expectedVersion: 4, operationId: "forged" });
  assert.deepEqual(await value.invoke("settings-update", { request: forged }), { status: "conflict", reason: "project_owner_required" });
});

test("canonical readiness uses actual tasks and tracker and requires initialization identity", async (t) => {
  const value = await fixture(t);
  const ready = await value.invoke("readiness");
  assert.equal(ready.branch, "develop");
  assert.equal(ready.eligibleTask.id, "T-2");
  assert.deepEqual(ready.eligibleTask.dependencies, ["T-1"]);
  assert.equal(ready.readyForDispatch, true);
  await writeFile(value.tasksPath, "| ID | Owner | Status |\n| --- | --- | --- |\n| T-2 | TEAM-1 | ready |\n");
  assert.equal((await value.invoke("readiness")).readyForDispatch, false);
  await writeFile(path.join(value.root, ".agent-team", "TEAMS.md"), "Project: another-project\nProject owner: owner\n");
  const held = await value.invoke("readiness");
  assert.equal(held.readyForDispatch, false);
  assert.equal(held.projectInitialization.required, true);
});

test("inactive approved handoff preserves choices but cannot manufacture initialization or capability evidence", async (t) => {
  const value = await fixture(t);
  await rm(value.setupPath);
  const request = await envelope(value, { kickoff: { status: "approved", handoff: {
    ...value.setup.plan, tracker: { kind: "beads", status: "current" }, tasks: [{ id: "K-1", status: "ready", dependencies: [] }],
  } } });
  const result = await value.invoke("readiness", { request });
  assert.equal(result.path, "kickoff");
  assert.equal(result.branch, "develop");
  assert.equal(result.tracker.kind, "beads");
  assert.equal(result.readyForDispatch, false);
  assert.equal(result.projectInitialization.required, true);
  assert.ok(result.missing.some(({ id }) => id === "capability:serena"));
  await assert.rejects(readFile(value.setupPath), { code: "ENOENT" });
});

test("dependency preparation fixes selected managed paths and keeps worker discovery unverified", async (t) => {
  const { runSetupCommand } = await import(modulePath);
  const value = await fixture(t);
  const request = await envelope(value, { selections: { defaults: [] } });
  let observedPaths;
  const createRunner = ({ paths }) => {
    observedPaths = paths;
    return async ({ dependency, phase }) => phase === "worker"
      ? { status: "unverified", evidence: "No fresh worker" }
      : { status: "passed", version: dependency.version, evidence: "Fixture functional evidence" };
  };
  const result = await runSetupCommand("dependencies-prepare", { ...value.options, request }, { createDependencyRunner: createRunner, nativeIdentity: value.nativeIdentity });
  assert.equal(result.status, "incomplete");
  assert.equal(observedPaths.toolRoot, path.join(value.root, ".agent-team", "tools"));
  assert.equal(observedPaths.skillRoot, path.join(value.root, ".agents", "skills"));
  assert.ok(result.receipts.every(({ availableToWorker }) => availableToWorker === "unknown"));
  const home = path.join(value.root, "isolated-home");
  const second = await envelope(value, { selections: { defaults: [] } }, { expectedVersion: 4, operationId: "user-prepare" });
  await runSetupCommand("dependencies-prepare", { ...value.options, scope: "user", home, request: second }, { createDependencyRunner: createRunner, nativeIdentity: value.nativeIdentity });
  assert.equal(observedPaths.toolRoot, path.join(home, ".agent-team", "tools"));
  assert.equal(observedPaths.skillRoot, path.join(home, ".agents", "skills"));
});

test("user-scope dependency receipts can satisfy project readiness", async (t) => {
  const { runSetupCommand } = await import(modulePath);
  const value = await fixture(t);
  const request = await envelope(value, { selections: { defaults: [] } }, { operationId: "user-ready" });
  const createRunner = () => async ({ dependency }) => ({ status: "passed", version: dependency.version, evidence: "Bounded fixture evidence" });

  assert.equal((await runSetupCommand("dependencies-prepare", { ...value.options, scope: "user", home: path.join(value.root, "home"), request }, { createDependencyRunner: createRunner, nativeIdentity: value.nativeIdentity })).status, "ready");
  const ready = await runSetupCommand("readiness", { ...value.options, scope: "user" });
  assert.equal(ready.readyForDispatch, true);
  assert.equal(ready.eligibleTask.id, "T-2");
});

test("canonical readiness holds tasks outside the active run and when capacity is reserved", async (t) => {
  const { runSetupCommand } = await import(modulePath);
  const value = await fixture(t);
  const state = JSON.parse(await readFile(value.statePath, "utf8"));
  state.stateVersion = 1;
  state.run.taskIds = ["T-1"];
  await writeFile(value.statePath, JSON.stringify(state));
  const scoped = await runSetupCommand("readiness", value.options);
  assert.equal(scoped.readyForDispatch, false);
  assert.equal(scoped.eligibleTask, null);

  state.stateVersion = 2;
  state.run.taskIds = ["T-1", "T-2"];
  state.capacity = { limit: 1, active: 1, reservedReview: 0 };
  await writeFile(value.statePath, JSON.stringify(state));
  const full = await runSetupCommand("readiness", value.options);
  assert.equal(full.readyForDispatch, false);
  assert.equal(full.eligibleTask, null);
});

test("dashboard configuration requires Beads and explicit graph terms and preserves custom fields", async (t) => {
  const { runSetupCommand } = await import(modulePath);
  const markdown = await fixture(t);
  const config = { snapshot: true, graph: { enabled: true, termsAcknowledged: true, executable: "/opt/bv" } };
  const markdownRequest = await envelope(markdown, { dashboard: config });
  await assert.rejects(runSetupCommand("dashboard-configure", { ...markdown.options, request: markdownRequest }, { nativeIdentity: markdown.nativeIdentity }), /Beads/);
  const value = await fixture(t, {}, { trackerKind: "beads" });
  await rm(value.trackerExecutable);
  const request = await envelope(value, { dashboard: config });
  const denied = await envelope(value, { dashboard: { ...config, graph: { enabled: true, termsAcknowledged: false } } });
  await assert.rejects(runSetupCommand("dashboard-configure", { ...value.options, request: denied }, { nativeIdentity: value.nativeIdentity }), /acknowledg/i);
  const result = await runSetupCommand("dashboard-configure", { ...value.options, request }, { nativeIdentity: value.nativeIdentity });
  assert.equal(result.status, "applied");
  assert.equal(result.setup.dashboard.custom, "keep");
  assert.equal(result.setup.dashboard.graph.custom, "keep");
  assert.equal(result.setup.dashboard.graph.executable, "/opt/bv");
});

test("malformed or ineffective input fails before mutation and request reads are bounded regular JSON", async (t) => {
  const { runSetupCommand } = await import(modulePath);
  const value = await fixture(t);
  const before = await readFile(value.setupPath, "utf8");
  for (const extra of [{ host: "both" }, { scope: "global" }, { toolRoot: "/outside" }, { home: "/outside" }, { nativeChoices }]) {
    await assert.rejects(runSetupCommand("settings", { ...value.options, ...extra }));
  }
  await assert.rejects(runSetupCommand("unknown", value.options), /Unknown/);
  await assert.rejects(runSetupCommand("settings-update", { ...value.options, request: value.root }), /regular/);
  const huge = path.join(value.root, "huge.json");
  await writeFile(huge, " ".repeat(256 * 1024 + 1));
  await assert.rejects(runSetupCommand("settings-update", { ...value.options, request: huge }), /bounded/);
  const injected = await envelope(value, { selections: {}, paths: { toolRoot: "/outside" } });
  await assert.rejects(runSetupCommand("dependencies-prepare", { ...value.options, request: injected }, { nativeIdentity: value.nativeIdentity }), /paths/);
  const invalidVersion = await envelope(value, { change: { kind: "run", values: { continuous: true } } }, { expectedVersion: "3" });
  await assert.rejects(runSetupCommand("settings-update", { ...value.options, request: invalidVersion }), /expectedVersion/);
  assert.equal(await readFile(value.setupPath, "utf8"), before);
});
test('readiness requires a completed initialization receipt', async (t) => {
  const f = await fixture(t);
  const setupPath = path.join(f.root, '.agent-team/setup.json');
  const setup = JSON.parse(await readFile(setupPath, 'utf8'));
  delete setup.initialization;
  await writeFile(setupPath, JSON.stringify(setup));
  const { runSetupCommand } = await import(modulePath);
  const result = await runSetupCommand('readiness', { project: f.root, host: 'codex', scope: 'project' });
  assert.equal(result.readyForDispatch, false);
  assert.equal(result.projectInitialization.required, true);
});

test("BrainVault-shaped pre-7.2 initialization remains readable without fabricating provenance", async (t) => {
  const value = await legacyBrainVaultFixture(t);
  const paths = [value.setupPath, value.statePath, path.join(value.root, ".agent-team", "TEAMS.md")];
  const before = await Promise.all(paths.map((file) => readFile(file, "utf8")));

  const readiness = await value.invoke("readiness");
  const settings = await value.invoke("settings");
  const { runSetupCommand } = await import(modulePath);

  assert.equal(readiness.projectInitialization.required, false, readiness.projectInitialization.reason);
  assert.equal(readiness.projectInitialization.projectOwner, "owner");
  assert.equal(settings.host, "codex");
  assert.deepEqual(await Promise.all(paths.map((file) => readFile(file, "utf8"))), before);

  const intruderRequest = await envelope(value, { change: { kind: "run", values: { continuous: true } } }, {
    operationId: "legacy-intruder-write", writer: { id: "other-owner", role: "project_orchestrator" },
  });
  const intruderResult = await runSetupCommand("settings-update", { ...value.options, request: intruderRequest });
  assert.equal(intruderResult.status, "conflict");
  assert.equal(intruderResult.reason, "project_owner_required");
  assert.deepEqual(await Promise.all(paths.map((file) => readFile(file, "utf8"))), before);

  const settingsRequest = await envelope(value, { change: { kind: "run", values: { continuous: true } } }, { operationId: "legacy-settings-write" });
  const settingsResult = await runSetupCommand("settings-update", { ...value.options, request: settingsRequest });
  assert.equal(settingsResult.status, "applied");
  assert.equal(JSON.stringify(settingsResult.setup.initialization), value.legacyInitialization);

  const dependencyRequest = await envelope(value, { selections: { defaults: [] } }, { expectedVersion: 4, operationId: "legacy-dependency-write" });
  const createRunner = () => async ({ dependency }) => ({ status: "passed", version: dependency.version, evidence: "Legacy fixture evidence" });
  const dependencyResult = await runSetupCommand("dependencies-prepare", { ...value.options, request: dependencyRequest }, { createDependencyRunner: createRunner });
  assert.equal(dependencyResult.status, "ready");
  assert.equal(JSON.stringify(dependencyResult.setup.initialization), value.legacyInitialization);

  const finalSetup = JSON.parse(await readFile(value.setupPath, "utf8"));
  assert.equal(JSON.stringify(finalSetup.initialization), value.legacyInitialization);
  assert.equal(Object.hasOwn(JSON.parse(await readFile(value.setupPath, "utf8")).initialization, "initialTaskIds"), false);
  assert.equal(Object.hasOwn(JSON.parse(await readFile(value.setupPath, "utf8")).initialization, "trackerFingerprint"), false);
  assert.equal(Object.hasOwn(JSON.parse(await readFile(value.setupPath, "utf8")).initialization.trackerSelection, "root"), false);
  assert.deepEqual(await Promise.all(paths.slice(1).map((file) => readFile(file, "utf8"))), before.slice(1));
});

test("setup mutations reject tampered legacy initialization receipts without writes", async (t) => {
  for (const command of ["settings-update", "dependencies-prepare"]) await t.test(command, async () => {
    const value = await legacyBrainVaultFixture(t);
    const setup = JSON.parse(await readFile(value.setupPath, "utf8"));
    setup.initialization.initialTaskIds = setup.plan.taskIds;
    await writeFile(value.setupPath, `${JSON.stringify(setup, null, 2)}\n`);
    const paths = [value.setupPath, value.statePath, path.join(value.root, ".agent-team", "TEAMS.md")];
    const before = await Promise.all(paths.map((file) => readFile(file, "utf8")));
    const request = command === "settings-update"
      ? { change: { kind: "run", values: { continuous: true } } }
      : { selections: { defaults: [] } };
    const requestPath = await envelope(value, request, { operationId: `tampered-${command}` });
    const { runSetupCommand } = await import(modulePath);
    const result = await runSetupCommand(command, { ...value.options, request: requestPath }, {
      createDependencyRunner: () => async () => { throw new Error("tampered receipt reached dependency runner"); },
    });
    assert.equal(result.status, "conflict");
    assert.equal(result.reason, "project_owner_required");
    assert.deepEqual(await Promise.all(paths.map((file) => readFile(file, "utf8"))), before);
  });
});

test("readiness rejects a receipt whose required initialization state is missing", async (t) => {
  const { runSetupCommand } = await import(modulePath);
  const value = await fixture(t);
  await rm(value.statePath);
  const result = await runSetupCommand("readiness", value.options);
  assert.equal(result.readyForDispatch, false);
  assert.equal(result.projectInitialization.required, true);
});

test("setup mutations recheck the initialization record after waiting for the setup lock", async (t) => {
  const { runSetupCommand } = await import(modulePath);
  const value = await fixture(t);
  const request = await envelope(value, { change: { kind: "run", values: { continuous: true } } }, { operationId: "stale-setup" });
  const lock = path.join(value.root, ".agent-team", ".locks", "setup.lock");
  await mkdir(lock, { recursive: true });
  const pending = runSetupCommand("settings-update", { ...value.options, request }, { nativeIdentity: value.nativeIdentity });
  await new Promise((resolve) => setTimeout(resolve, 250));
  const setup = JSON.parse(await readFile(value.setupPath, "utf8"));
  delete setup.initialization;
  await writeFile(value.setupPath, JSON.stringify(setup));
  await rm(lock, { recursive: true });

  const result = await pending;
  assert.deepEqual(result, { status: "conflict", reason: "project_owner_required" });
  assert.equal(JSON.parse(await readFile(value.setupPath, "utf8")).settings.runDefaults?.continuous, undefined);
});

test("setup mutations cannot switch to a different project while waiting for the setup lock", async (t) => {
  const { runSetupCommand } = await import(modulePath);
  const original = await fixture(t);
  const replacement = await fixture(t, {}, { projectId: "project-2" });
  const request = await envelope(original, { change: { kind: "run", values: { continuous: true } } }, { operationId: "project-switch" });
  const lock = path.join(original.root, ".agent-team", ".locks", "setup.lock");
  await mkdir(lock, { recursive: true });
  const pending = runSetupCommand("settings-update", { ...original.options, request }, { nativeIdentity: original.nativeIdentity });
  await new Promise((resolve) => setTimeout(resolve, 250));
  for (const name of ["setup.json", "TEAMS.md", "state.json", "operation-mappings.json"]) {
    await writeFile(path.join(original.root, ".agent-team", name), await readFile(path.join(replacement.root, ".agent-team", name)));
  }
  await rm(lock, { recursive: true });

  const result = await pending;

  assert.deepEqual(result, { status: "conflict", reason: "project_owner_required" });
  assert.equal(JSON.parse(await readFile(original.setupPath, "utf8")).settings.runDefaults?.continuous, undefined);
});

function orchestrationInput(value, overrides = {}) {
  return {
    project: value.root,
    host: "codex",
    scope: "project",
    dependencies: { action: "inspect" },
    settings: { operationId: "orchestrated-settings-1" },
    ...overrides,
  };
}

test("state-changing setup enters dependencies settings readiness and summary in order", async (t) => {
  const { orchestrateSetup } = await import(modulePath);
  const value = await fixture(t);
  const events = [];
  const result = await orchestrateSetup(orchestrationInput(value, { dependencies: {
    action: "prepare", expectedVersion: 3, operationId: "orchestrated-dependencies-1", selections: { defaults: [] },
  } }), {
    nativeIdentity: value.nativeIdentity, nativeChoices,
    createDependencyRunner: () => async ({ dependency, phase }) => {
      events.push(`dependencies:${phase}:${dependency.id}`);
      return { status: "passed", version: dependency.version, evidence: "bounded setup orchestration fixture" };
    },
    interactSettings: async ({ overview, wizard }) => {
      events.push("settings");
      assert.equal(overview.host, "codex");
      assert.equal(wizard.steps.at(-1).kind, "review");
      return { kind: "save", draft: { runDefaults: { continuous: true }, roles: { developer: { model: "fast", effort: "low" } } } };
    },
  });
  events.push("summary");
  assert.ok(events.findIndex((entry) => entry.startsWith("dependencies:")) < events.indexOf("settings"));
  assert.equal(events.at(-1), "summary");
  assert.equal(result.settingsOutcome, "saved");
  assert.equal(result.settings.runDefaults.continuous, true);
  assert.deepEqual(result.nativeIdentity, { role: "project_owner", host: "codex", sessionId: "owner", ownershipEpoch: 1 });
  const setup = JSON.parse(await readFile(value.setupPath, "utf8"));
  assert.equal(setup.version, 5);
  assert.equal(setup.setupOperations.filter(({ id }) => id === "orchestrated-settings-1").length, 1);
});

test("repeated setup always enters current-effective settings", async (t) => {
  const { orchestrateSetup } = await import(modulePath);
  const value = await fixture(t);
  let interactions = 0;
  const context = {
    nativeIdentity: value.nativeIdentity, nativeChoices,
    interactSettings: async ({ overview }) => {
      interactions += 1;
      assert.equal(overview.runDefaults.parallel_teams, interactions === 1 ? undefined : 4);
      return interactions === 1
        ? { kind: "save", draft: { runDefaults: { parallel_teams: 4 } } }
        : { kind: "keep_existing" };
    },
  };
  assert.equal((await orchestrateSetup(orchestrationInput(value), context)).settingsOutcome, "saved");
  assert.equal((await orchestrateSetup(orchestrationInput(value, { settings: { operationId: "orchestrated-settings-2" } }), context)).settingsOutcome, "kept_existing");
  assert.equal(interactions, 2);
});

test("non-consent outcomes preserve post-dependency bytes and continue to summary", async (t) => {
  const { orchestrateSetup } = await import(modulePath);
  const cases = [
    ["keep_existing", { kind: "keep_existing" }], ["cancel", { kind: "cancel" }], ["back", { kind: "back" }],
    ["undefined", undefined], ["no_answer", { kind: "no_answer" }], ["timeout", { kind: "timeout" }],
    ["returned-interrupted", { kind: "interrupted" }], ["thrown-interrupted", "throw"],
  ];
  for (const [label, outcome] of cases) await t.test(label, async () => {
    const value = await fixture(t);
    let checkpoint;
    let interactions = 0;
    const result = await orchestrateSetup(orchestrationInput(value, { dependencies: {
      action: "prepare", expectedVersion: 3, operationId: `dependencies-${label}`, selections: { defaults: [] },
    }, settings: { operationId: `settings-${label}` } }), {
      nativeIdentity: value.nativeIdentity, nativeChoices,
      createDependencyRunner: () => async ({ dependency }) => ({ status: "passed", version: dependency.version, evidence: "non-consent fixture" }),
      interactSettings: async () => {
        interactions += 1;
        checkpoint = await readFile(value.setupPath);
        if (outcome === "throw") { const error = new Error("host interrupted"); error.code = "SETUP_INTERACTION_INTERRUPTED"; throw error; }
        return outcome;
      },
    });
    assert.equal(interactions, 1);
    assert.equal(result.settingsOutcome, "kept_existing");
    assert.ok(result.readiness);
    assert.deepEqual(await readFile(value.setupPath), checkpoint);
    const setup = JSON.parse(checkpoint);
    assert.ok(setup.dependencies.hosts.codex.receipts.length >= 2);
    assert.equal(setup.setupOperations.some(({ id }) => id === `settings-${label}`), false);
  });
});

test("native setup authority cannot come from flags requests or caller epochs", async (t) => {
  const { orchestrateSetup } = await import(modulePath);
  const cases = [
    ["missing", undefined],
    ["unobserved", { host: "codex", sessionId: "owner", observed: false, cwd: null }],
    ["wrong host", { host: "claude-code", sessionId: "owner", observed: true }],
    ["wrong session", { host: "codex", sessionId: "other", observed: true }],
    ["wrong cwd", { host: "codex", sessionId: "owner", observed: true, cwd: os.tmpdir() }],
  ];
  for (const [label, native] of cases) {
    const value = await fixture(t);
    let interacted = false;
    const result = await orchestrateSetup(orchestrationInput(value), {
      nativeIdentity: native && { ...native, cwd: native.cwd ?? value.root }, nativeChoices,
      interactSettings: async () => { interacted = true; return { kind: "keep_existing" }; },
    });
    assert.deepEqual(result, { status: "conflict", reason: "project_owner_required" }, label);
    assert.equal(interacted, false);
  }
  const value = await fixture(t);
  await assert.rejects(orchestrateSetup({ ...orchestrationInput(value), writer: { id: "owner" }, ownershipEpoch: 1 }, {
    nativeIdentity: value.nativeIdentity, interactSettings: async () => ({ kind: "keep_existing" }),
  }), /Unsupported setup input field/);
  assert.deepEqual(await orchestrateSetup(orchestrationInput(value), {}), { status: "conflict", reason: "project_owner_required" });
});

test("legacy unqualified ownership cannot authorize native setup across host cwd or missing epoch", async (t) => {
  const { orchestrateSetup } = await import(modulePath);
  const cases = [
    ["same host and cwd without qualified epoch", (value) => ({ host: "codex", sessionId: "owner", observed: true, cwd: value.root })],
    ["cross host", (value) => ({ host: "claude-code", sessionId: "owner", observed: true, cwd: value.root })],
    ["wrong cwd", () => ({ host: "codex", sessionId: "owner", observed: true, cwd: os.tmpdir() })],
  ];
  for (const [label, identityValue] of cases) {
    const value = await legacyBrainVaultFixture(t);
    const before = await readFile(value.setupPath);
    let interacted = false;
    const result = await orchestrateSetup(orchestrationInput(value), {
      nativeIdentity: identityValue(value), nativeChoices,
      interactSettings: async () => { interacted = true; return { kind: "keep_existing" }; },
    });
    assert.deepEqual(result, { status: "conflict", reason: "project_owner_required" }, label);
    assert.equal(interacted, false);
    assert.deepEqual(await readFile(value.setupPath), before);
  }
});

test("dependency inspection reports selected scope mismatch and still enters settings summary", async (t) => {
  const { orchestrateSetup } = await import(modulePath);
  const value = await fixture(t);
  let interacted = false;
  const result = await orchestrateSetup(orchestrationInput(value, { scope: "user" }), {
    nativeIdentity: value.nativeIdentity, nativeChoices,
    interactSettings: async () => { interacted = true; return { kind: "keep_existing" }; },
  });
  assert.equal(interacted, true);
  assert.deepEqual(result.dependencies, {
    status: "unavailable", reason: "dependency_scope_mismatch", host: "codex",
    scope: "user", recordedScope: "project",
  });
  assert.equal(result.settingsOutcome, "kept_existing");
  assert.ok(result.readiness);
});

test("setup summary is pure and projects only qualified identity", async () => {
  const { buildSetupSummary } = await import(modulePath);
  const input = {
    project: { projectId: "pure", root: "/project", secret: "omit" }, dependencies: { status: "ready", nested: { value: 1 } },
    readiness: { readyForDispatch: true, eligibleTask: { id: "T-1" } }, settings: { host: "codex", runDefaults: { parallel_teams: 2 } },
    settingsOutcome: "kept_existing", nativeIdentity: { role: "project_owner", host: "codex", sessionId: "owner", ownershipEpoch: 4, raw: "omit" },
  };
  const before = structuredClone(input);
  const result = buildSetupSummary(input);
  assert.deepEqual(input, before);
  assert.deepEqual(result.project, { id: "pure", root: "/project" });
  assert.deepEqual(result.nativeIdentity, { role: "project_owner", host: "codex", sessionId: "owner", ownershipEpoch: 4 });
  input.dependencies.nested.value = 2;
  assert.equal(result.dependencies.nested.value, 1);
  assert.equal(result.status, "ready");
});

test("read-only actions and shell setup bypass orchestration without writes", async (t) => {
  const { runSetupCommand, setupCommandFlags } = await import(modulePath);
  const value = await fixture(t);
  const before = await readFile(value.setupPath);
  assert.equal(Object.hasOwn(setupCommandFlags, "setup"), false);
  await assert.rejects(runSetupCommand("setup", value.options, { interactSettings: async () => { throw new Error("wizard entered"); } }), /Unknown setup command/);
  await value.invoke("settings");
  await value.invoke("readiness");
  const cli = path.resolve(import.meta.dirname, "../hooks/agent-team-cli.mjs");
  await exec(process.execPath, [cli, "status", "--project", value.root]);
  await exec(process.execPath, [cli, "health", "--project", value.root]);
  await assert.rejects(exec(process.execPath, [cli, "setup", "--project", value.root]),
    (error) => /Unknown command: setup/.test(error.stdout));
  assert.deepEqual(await readFile(value.setupPath), before);
});

test("unknown settings interaction and arbitrary errors are not relabeled as consent", async (t) => {
  const { orchestrateSetup } = await import(modulePath);
  for (const [label, interactSettings, message] of [
    ["unknown", async () => ({ kind: "default" }), /Unknown settings interaction outcome/],
    ["arbitrary", async () => { throw new Error("programmer failure"); }, /programmer failure/],
  ]) {
    const value = await fixture(t);
    const before = await readFile(value.setupPath);
    await assert.rejects(orchestrateSetup(orchestrationInput(value, { settings: { operationId: `unknown-${label}` } }), {
      nativeIdentity: value.nativeIdentity, nativeChoices, interactSettings,
    }), message);
    assert.deepEqual(await readFile(value.setupPath), before);
  }
});
