import assert from "node:assert/strict";
import { execFile, execFileSync } from "node:child_process";
import { access, mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import test from "node:test";
import { promisify } from "node:util";
import { runCommand } from "../hooks/agent-team-cli.mjs";

const run = promisify(execFile);
const cli = path.resolve(import.meta.dirname, "../hooks/agent-team-cli.mjs");
const temporary = [];
test.afterEach(async () => Promise.all(temporary.splice(0).map((root) => rm(root, { recursive: true, force: true }))));

async function fixture() {
  const root = await mkdtemp(path.join(os.tmpdir(), "agent team journey "));
  temporary.push(root);
  execFileSync("git", ["init", "-q", "-b", "main", root]);
  execFileSync("git", ["config", "user.name", "Journey Fixture"], { cwd: root });
  execFileSync("git", ["config", "user.email", "journey@example.test"], { cwd: root });
  await writeFile(path.join(root, "README.md"), "Fixture project.\n");
  execFileSync("git", ["add", "README.md"], { cwd: root });
  execFileSync("git", ["commit", "-qm", "fixture"], { cwd: root });
  return root;
}

async function invoke(command, root, ...args) {
  const result = await run(process.execPath, [cli, command, "--project", root, ...args], { encoding: "utf8", timeout: 5000 });
  return JSON.parse(result.stdout);
}

async function request(root, name, value) {
  const file = path.join(root, `${name}.json`);
  await writeFile(file, JSON.stringify(value));
  return file;
}

function planRequest() {
  return { schemaVersion: 1, actorSessionId: "project-owner", expectedVersion: 0, request: {
    projectId: "journey", operationId: "initialize-journey", source: "standalone",
    tracker: { kind: "markdown", path: "TASKS.md" },
    plan: { scope: "Implement the approved change", acceptance: ["Required checks pass"], branch: "main",
      verification: ["node --test"], authority: { ownedPaths: ["src/**", "tests/**"] },
      tasks: [{ id: "WORK-1", title: "Implement approved change", status: "ready" }] },
  } };
}

test("actual project CLI connects initialization, settings, claims, checkpoints and an opted-in snapshot", async () => {
  const root = await fixture();
  const initialization = await request(root, "initialization", planRequest());
  const shell = await invoke("project-initialize", root, "--request", initialization);
  assert.deepEqual({ status: shell.status, ready: shell.ready, reason: shell.reason },
    { status: "validated", ready: false, reason: "native_identity_required" });
  const nativeIdentity = { host: "codex", sessionId: "project-owner", observed: true, cwd: root };
  const created = await runCommand("project-initialize", { project: root, request: initialization }, { nativeIdentity });
  assert.equal(created.status, "applied");
  assert.equal(created.canonicalReady, true);
  assert.equal(created.ready, false, "canonical initialization is not native/capability readiness");
  assert.equal((await runCommand("project-initialize", { project: root, request: initialization }, { nativeIdentity })).status, "duplicate");
  const ownerIdentity = { ...nativeIdentity, ownershipEpoch: 1 };

  const selectors = ["--host", "codex", "--scope", "project"];
  const settings = await invoke("settings", root, ...selectors);
  assert.ok(settings.roles.length > 0);
  const readiness = await invoke("readiness", root, ...selectors);
  assert.equal(readiness.projectInitialization.required, false);
  assert.equal(readiness.readyForDispatch, false);
  assert.ok(readiness.missing.some(({ id }) => id === "capability:playwright-cli"));
  const dependencies = await invoke("dependencies", root, ...selectors);
  assert.equal(dependencies.host, "codex");

  const update = await request(root, "settings-update", { schemaVersion: 1, expectedVersion: 1, operationId: "run-defaults",
    writer: { id: "project-owner", role: "project_orchestrator" }, request: { change: { kind: "run", values: { continuous: true } } } });
  assert.equal((await runCommand("settings-update", { project: root, host: "codex", scope: "project", request: update }, { nativeIdentity: ownerIdentity })).status, "applied");
  const configure = await request(root, "dashboard-configure", { schemaVersion: 1, expectedVersion: 2, operationId: "snapshot-opt-in",
    writer: { id: "project-owner", role: "project_orchestrator" }, request: { dashboard: { snapshot: true, graph: { enabled: false, termsAcknowledged: false } } } });
  assert.equal((await runCommand("dashboard-configure", { project: root, host: "codex", scope: "project", request: configure }, { nativeIdentity: ownerIdentity })).status, "applied");
  await assert.rejects(access(path.join(root, ".agent-team/dashboard/index.html")), { code: "ENOENT" });

  const status = await invoke("status", root);
  assert.equal(status.versions.setup, 3);
  const claim = await request(root, "claim", { schemaVersion: 1, actorSessionId: "project-owner", expectedVersion: status.versions.operational,
    request: { operationId: "claim-WORK-1", taskId: "WORK-1", expectedFingerprint: status.freshness.fingerprint,
      expectedOwner: status.tasks[0].canonicalOwner, action: "claim", owner: "project-owner" } });
  const claimed = await runCommand("task-transition", { project: root, request: claim }, { nativeIdentity: ownerIdentity });
  assert.equal(claimed.status, "applied");
  assert.equal(claimed.dashboard.status, "published");
  const current = await invoke("status", root);
  assert.equal(current.tasks[0].status, "in_progress");
  // This fixture has no prior checkpoint; checkpoint and operational versions are independent.
  const checkpoint = await request(root, "checkpoint", { schemaVersion: 1, actorSessionId: "project-owner", expectedVersion: 0,
    request: { eventId: "journey-checkpoint", sessionId: "project-owner", taskIds: ["WORK-1"], worktree: root,
      revision: execFileSync("git", ["rev-parse", "HEAD"], { cwd: root, encoding: "utf8" }).trim(), nextAction: "Run the required verification." } });
  assert.equal((await runCommand("checkpoint", { project: root, request: checkpoint }, { nativeIdentity: ownerIdentity })).status, "applied");
  assert.match(await readFile(path.join(root, ".agent-team/dashboard/index.html"), "utf8"), /WORK-1/);
  assert.equal((await invoke("recovery", root, "--session", "project-owner")).nextAction, "Run the required verification.");
  assert.equal((await invoke("readiness", root, "--host", "codex", "--scope", "user")).readyForDispatch, false);
});

test("initialization CLI cannot replace the envelope actor with a body owner claim", async () => {
  const root = await fixture();
  const value = planRequest();
  value.request.ownerSessionId = "forged-owner";
  const file = await request(root, "initialization", value);
  const nativeIdentity = { host: "codex", sessionId: value.actorSessionId, observed: true, cwd: root };
  const result = await runCommand("project-initialize", { project: root, request: file }, { nativeIdentity });
  assert.equal(result.status, "conflict");
  assert.equal(result.reason, "invalid_request");
  await assert.rejects(access(path.join(root, ".agent-team")), { code: "ENOENT" });
});

test("native identity validation and shell validation are mutation-free", async () => {
  const cases = [
    ["shell validation", undefined, "native_identity_required"],
    ["unobserved native identity", { host: "codex", sessionId: "project-owner", observed: false }, "native_identity_required"],
    ["unsupported native host", { host: "other", sessionId: "project-owner", observed: true }, "native_host_unsupported"],
    ["mismatched native identity", { host: "codex", sessionId: "different-owner", observed: true }, "native_identity_mismatch"],
  ];
  for (const [name, identity, reason] of cases) {
    const root = await fixture();
    const file = await request(root, `initialization-${name.replaceAll(" ", "-")}`, planRequest());
    const result = await runCommand("project-initialize", { project: root, request: file }, identity ? { nativeIdentity: { ...identity, cwd: root } } : {});
    assert.deepEqual({ status: result.status, ready: result.ready, reason: result.reason }, { status: "validated", ready: false, reason }, name);
    await assert.rejects(access(path.join(root, ".agent-team")), { code: "ENOENT" });
    await assert.rejects(access(path.join(root, "TASKS.md")), { code: "ENOENT" });
  }
});

test("native identity rejects nested repository and linked worktree cwd before mutation", async () => {
  const root = await fixture();
  const linked = `${root}-linked`;
  temporary.push(linked);
  execFileSync("git", ["worktree", "add", "-q", "-b", "linked", linked], { cwd: root });
  const nested = path.join(root, "nested");
  execFileSync("git", ["init", "-q", "-b", "main", nested]);
  const envelope = planRequest();
  for (const [project, cwd] of [[root, nested], [root, linked], [linked, linked]]) {
    const file = await request(root, `identity-${Math.random().toString(16).slice(2)}`, envelope);
    const result = await runCommand("project-initialize", { project, request: file }, {
      nativeIdentity: { host: "codex", sessionId: envelope.actorSessionId, observed: true, cwd },
    });
    assert.equal(result.reason, "native_project_cwd_mismatch");
  }
  await assert.rejects(access(path.join(root, ".agent-team")), { code: "ENOENT" });
  await assert.rejects(access(path.join(root, "TASKS.md")), { code: "ENOENT" });
});

test("initialization CLI rejects a nonzero version for absent setup without publishing records", async () => {
  const root = await fixture();
  const value = planRequest();
  value.expectedVersion = 7;
  const file = await request(root, "stale-initialization", value);
  const nativeIdentity = { host: "codex", sessionId: value.actorSessionId, observed: true, cwd: root };
  const result = await runCommand("project-initialize", { project: root, request: file }, { nativeIdentity });
  assert.equal(result.status, "conflict");
  assert.equal(result.reason, "stale_setup_version");
  for (const name of ["TASKS.md", ".agent-team/setup.json", ".agent-team/state.json", ".agent-team/TEAMS.md", ".agent-team/.setup-initialization.json"]) {
    await assert.rejects(access(path.join(root, name)), { code: "ENOENT" });
  }
  value.expectedVersion = 0;
  await writeFile(file, JSON.stringify(value));
  assert.equal((await runCommand("project-initialize", { project: root, request: file }, { nativeIdentity })).status, "applied");
});

test("initialized Project Kickoff capabilities flow unchanged into readiness", async () => {
  const root = await fixture();
  const value = planRequest();
  value.request.plan.requiredCapabilities = ["graphify"];
  const file = await request(root, "project-kickoff-initialization", value);
  const shell = await invoke("project-initialize", root, "--request", file);
  assert.deepEqual({ status: shell.status, ready: shell.ready, reason: shell.reason },
    { status: "validated", ready: false, reason: "native_identity_required" });
  await assert.rejects(access(path.join(root, ".agent-team")), { code: "ENOENT" });
  await assert.rejects(access(path.join(root, "TASKS.md")), { code: "ENOENT" });

  const nativeIdentity = { host: "codex", sessionId: value.actorSessionId, observed: true, cwd: root };
  const initialized = await runCommand("project-initialize", { project: root, request: file }, { nativeIdentity });
  assert.equal(initialized.status, "applied");
  assert.deepEqual(initialized.setup.plan.requiredCapabilities, ["graphify"]);
  const setupPath = path.join(root, ".agent-team/setup.json");
  const before = await readFile(setupPath);
  const readiness = await invoke("readiness", root, "--host", "codex", "--scope", "project");
  assert.deepEqual(readiness.requiredCapabilities, ["serena", "playwright-cli", "graphify"]);
  assert.ok(readiness.missing.some(({ id }) => id === "capability:graphify"));
  assert.deepEqual(await readFile(setupPath), before);
});

const journeyChoices = { models: [
  { id: "quality", recommended: true, available: true, efforts: ["high"] },
  { id: "fast", available: true, efforts: ["low"] },
] };

async function initializedJourney() {
  const root = await fixture();
  const file = await request(root, "native-setup-initialization", planRequest());
  const nativeIdentity = { host: "codex", sessionId: "project-owner", observed: true, cwd: root };
  const initialized = await runCommand("project-initialize", { project: root, request: file }, { nativeIdentity });
  assert.equal(initialized.status, "applied");
  return { root, nativeIdentity: { ...nativeIdentity, ownershipEpoch: 1 }, setupPath: path.join(root, ".agent-team", "setup.json") };
}

test("native setup prepares dependencies saves settings and returns readiness summary", async () => {
  const { orchestrateSetup } = await import("../hooks/lib/setup-cli.mjs");
  const value = await initializedJourney();
  const events = [];
  const result = await orchestrateSetup({
    project: value.root, host: "codex", scope: "project",
    dependencies: { action: "prepare", expectedVersion: 1, operationId: "journey-dependencies", selections: { defaults: [] } },
    settings: { operationId: "journey-settings" },
  }, {
    nativeIdentity: value.nativeIdentity, nativeChoices: journeyChoices,
    createDependencyRunner: () => async ({ dependency, phase }) => {
      events.push(`dependency:${phase}`);
      return { status: "passed", version: dependency.version, evidence: "journey setup evidence" };
    },
    interactSettings: async () => {
      events.push("settings");
      return { kind: "save", draft: { runDefaults: { parallel_teams: 2 }, roles: { developer: { model: "fast", effort: "low" } } } };
    },
  });
  assert.ok(events.some((entry) => entry.startsWith("dependency:")));
  assert.equal(events.at(-1), "settings");
  assert.equal(result.settingsOutcome, "saved");
  assert.ok(result.readiness);
  assert.equal(result.settings.runDefaults.parallel_teams, 2);
  const setup = JSON.parse(await readFile(value.setupPath, "utf8"));
  assert.equal(setup.setupOperations.filter(({ id }) => id === "journey-settings").length, 1);
});

test("repeated native setup keeps existing settings after dependency preparation", async () => {
  const { orchestrateSetup } = await import("../hooks/lib/setup-cli.mjs");
  const value = await initializedJourney();
  let checkpoint;
  let interactions = 0;
  const result = await orchestrateSetup({
    project: value.root, host: "codex", scope: "project",
    dependencies: { action: "prepare", expectedVersion: 1, operationId: "journey-repeat-dependencies", selections: { defaults: [] } },
    settings: { operationId: "journey-repeat-settings" },
  }, {
    nativeIdentity: value.nativeIdentity, nativeChoices: journeyChoices,
    createDependencyRunner: () => async ({ dependency }) => ({ status: "passed", version: dependency.version, evidence: "journey repeat evidence" }),
    interactSettings: async () => {
      interactions += 1;
      checkpoint = await readFile(value.setupPath);
      return { kind: "keep_existing" };
    },
  });
  assert.equal(interactions, 1);
  assert.equal(result.settingsOutcome, "kept_existing");
  assert.deepEqual(await readFile(value.setupPath), checkpoint);
  assert.ok(JSON.parse(checkpoint).setupOperations.some(({ id }) => id === "journey-repeat-dependencies"));
});

test("read-only status bypasses setup orchestration", async () => {
  const value = await initializedJourney();
  const before = await readFile(value.setupPath);
  const result = await runCommand("status", { project: value.root }, {
    nativeIdentity: value.nativeIdentity,
    interactSettings: async () => { throw new Error("read-only status entered setup"); },
  });
  assert.equal(result.project.id, "journey");
  assert.deepEqual(await readFile(value.setupPath), before);
});
