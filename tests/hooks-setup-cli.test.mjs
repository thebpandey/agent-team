import assert from "node:assert/strict";
import { execFile } from "node:child_process";
import { mkdir, mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import { pathToFileURL } from "node:url";
import { promisify } from "node:util";
import test from "node:test";

const exec = promisify(execFile);
const modulePath = path.resolve(import.meta.dirname, "../hooks/lib/setup-cli.mjs");
const nativeChoices = { enforceable: false, models: [
  { id: "quality", label: "Quality", efforts: ["high"], recommended: true, available: true },
  { id: "fast", label: "Fast", efforts: ["low"], available: true },
] };

async function fixture(t, overrides = {}) {
  overrides = { initialization: { status: 'complete' }, ...overrides };
  const root = await mkdtemp(path.join(os.tmpdir(), "agent team setup cli "));
  t.after(() => rm(root, { recursive: true, force: true }));
  await exec("git", ["init", "--quiet", root]);
  await mkdir(path.join(root, ".agent-team"));
  const setupPath = path.join(root, ".agent-team", "setup.json");
  const setup = {
    skill: "agent-team", projectId: "project-1", version: 3,
    tracker: { kind: "markdown", path: "TASKS.md" },
    plan: { scope: "Repair parser", acceptance: ["Regression passes"], branch: "develop", verification: ["node --test"], authority: { writes: ["src"], deployment: false } },
    settings: { custom: "keep", hosts: { "claude-code": { roles: { developer: { model: "custom", effort: "high" } } } } },
    dashboard: { custom: "keep", graph: { custom: "keep", enabled: false, termsAcknowledged: false } },
    dependencies: { hosts: { codex: { scope: "project", receipts: [
      { id: "serena", functional: "passed", availableToWorker: "passed", status: "ready" },
      { id: "playwright-cli", functional: "passed", availableToWorker: "passed", status: "ready" },
    ] } } },
    ...overrides,
  };
  await writeFile(setupPath, `${JSON.stringify(setup, null, 2)}\n`);
  await writeFile(path.join(root, ".agent-team", "TEAMS.md"), "Project: project-1\nProject owner: owner\n");
  await writeFile(path.join(root, "TASKS.md"), "| ID | Owner | Status | Depends on |\n| --- | --- | --- | --- |\n| T-1 | - | closed | - |\n| T-2 | - | ready | T-1 |\n");
  const options = { project: root, host: "codex", scope: "project" };
  const consumer = path.join(root, "consumer.mjs");
  await writeFile(consumer, `import { runSetupCommand } from ${JSON.stringify(pathToFileURL(modulePath).href)};\ntry { console.log(JSON.stringify(await runSetupCommand(process.argv[2], JSON.parse(process.argv[3])))); } catch (error) { console.log(JSON.stringify({ status: "failed", error: error.message })); process.exitCode = 1; }\n`);
  const invoke = async (command, extra = {}) => JSON.parse((await exec(process.execPath, [consumer, command, JSON.stringify({ ...options, ...extra })])).stdout);
  return { root, setupPath, setup, options, invoke };
}

async function envelope(value, request, top = {}) {
  const file = path.join(value.root, `request-${Math.random().toString(16).slice(2)}.json`);
  await writeFile(file, JSON.stringify({ schemaVersion: 1, expectedVersion: 3, operationId: "operation-1", writer: { id: "owner", role: "project_orchestrator" }, request, ...top }));
  return file;
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
  assert.deepEqual(efforts.choices.filter(({ id }) => !["back", "cancel"].includes(id)).map(({ id }) => id), ["low"]);
});

test("settings changes use canonical owner/version and semantic operation identity", async (t) => {
  const { runSetupCommand } = await import(modulePath);
  const value = await fixture(t);
  const request = await envelope(value, { change: { kind: "role", role: "developer", model: "quality", effort: "high" } });
  const applied = await runSetupCommand("settings-update", { ...value.options, request }, { nativeChoices });
  assert.equal(applied.status, "applied");
  assert.equal(applied.version, 4);
  assert.deepEqual(applied.setup.settings.hosts["claude-code"], value.setup.settings.hosts["claude-code"]);
  assert.equal(applied.setup.settings.custom, "keep");
  assert.equal((await runSetupCommand("settings-update", { ...value.options, request }, { nativeChoices })).status, "duplicate");
  const reused = await envelope(value, { change: { kind: "run", values: { continuous: true } } });
  assert.equal((await runSetupCommand("settings-update", { ...value.options, request: reused })).reason, "operation_id_reused");
  const stale = await envelope(value, { change: { kind: "run", values: { continuous: true } } }, { operationId: "operation-2" });
  assert.equal((await runSetupCommand("settings-update", { ...value.options, request: stale })).reason, "version_changed");
  await writeFile(path.join(value.root, ".agent-team", "TEAMS.md"), "Project: project-1\nProject owner: replacement\n");
  assert.equal((await runSetupCommand("settings-update", { ...value.options, request: stale })).reason, "project_owner_required");
});

test("Node consumer applies run defaults but cannot submit native capability claims", async (t) => {
  const value = await fixture(t);
  const request = await envelope(value, { change: { kind: "run", values: { continuous: true } } });
  assert.equal((await value.invoke("settings-update", { request })).status, "applied");
  const forged = await envelope(value, { change: { kind: "role", role: "developer", model: "made-up", effort: "high" }, nativeChoices }, { expectedVersion: 4, operationId: "forged" });
  await assert.rejects(value.invoke("settings-update", { request: forged }), (error) => /nativeChoices|Unsupported/.test(error.stdout));
});

test("canonical readiness uses actual tasks and tracker and requires initialization identity", async (t) => {
  const value = await fixture(t);
  const ready = await value.invoke("readiness");
  assert.equal(ready.branch, "develop");
  assert.equal(ready.eligibleTask.id, "T-2");
  assert.deepEqual(ready.eligibleTask.dependencies, ["T-1"]);
  assert.equal(ready.readyForDispatch, true);
  await writeFile(path.join(value.root, "TASKS.md"), "| ID | Owner | Status |\n| --- | --- | --- |\n| T-2 | TEAM-1 | ready |\n");
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
  const result = await runSetupCommand("dependencies-prepare", { ...value.options, request }, { createDependencyRunner: createRunner });
  assert.equal(result.status, "incomplete");
  assert.equal(observedPaths.toolRoot, path.join(value.root, ".agent-team", "tools"));
  assert.equal(observedPaths.skillRoot, path.join(value.root, ".agents", "skills"));
  assert.ok(result.receipts.every(({ availableToWorker }) => availableToWorker === "unknown"));
  const home = path.join(value.root, "isolated-home");
  const second = await envelope(value, { selections: { defaults: [] } }, { expectedVersion: 4, operationId: "user-prepare" });
  await runSetupCommand("dependencies-prepare", { ...value.options, scope: "user", home, request: second }, { createDependencyRunner: createRunner });
  assert.equal(observedPaths.toolRoot, path.join(home, ".agent-team", "tools"));
  assert.equal(observedPaths.skillRoot, path.join(home, ".agents", "skills"));
});

test("dashboard configuration requires Beads and explicit graph terms and preserves custom fields", async (t) => {
  const { runSetupCommand } = await import(modulePath);
  const value = await fixture(t);
  const config = { snapshot: true, graph: { enabled: true, termsAcknowledged: true, executable: "/opt/bv" } };
  const request = await envelope(value, { dashboard: config });
  await assert.rejects(runSetupCommand("dashboard-configure", { ...value.options, request }), /Beads/);
  await writeFile(value.setupPath, JSON.stringify({ ...value.setup, tracker: { kind: "beads", executable: "/missing/selected-bd" } }));
  const denied = await envelope(value, { dashboard: { ...config, graph: { enabled: true, termsAcknowledged: false } } });
  await assert.rejects(runSetupCommand("dashboard-configure", { ...value.options, request: denied }), /acknowledg/i);
  const result = await value.invoke("dashboard-configure", { request });
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
  await assert.rejects(runSetupCommand("dependencies-prepare", { ...value.options, request: injected }), /paths/);
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
