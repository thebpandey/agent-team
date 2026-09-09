import assert from "node:assert/strict";
import { execFile, execFileSync } from "node:child_process";
import { access, mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import test from "node:test";
import { promisify } from "node:util";

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
  const created = await invoke("project-initialize", root, "--request", initialization);
  assert.equal(created.status, "applied");
  assert.equal(created.canonicalReady, true);
  assert.equal(created.ready, false, "canonical initialization is not native/capability readiness");
  assert.equal((await invoke("project-initialize", root, "--request", initialization)).status, "duplicate");

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
  assert.equal((await invoke("settings-update", root, ...selectors, "--request", update)).status, "applied");
  const configure = await request(root, "dashboard-configure", { schemaVersion: 1, expectedVersion: 2, operationId: "snapshot-opt-in",
    writer: { id: "project-owner", role: "project_orchestrator" }, request: { dashboard: { snapshot: true, graph: { enabled: false, termsAcknowledged: false } } } });
  assert.equal((await invoke("dashboard-configure", root, ...selectors, "--request", configure)).status, "applied");
  await assert.rejects(access(path.join(root, ".agent-team/dashboard/index.html")), { code: "ENOENT" });

  const status = await invoke("status", root);
  assert.equal(status.versions.setup, 3);
  const claim = await request(root, "claim", { schemaVersion: 1, actorSessionId: "project-owner", expectedVersion: status.versions.operational,
    request: { operationId: "claim-WORK-1", taskId: "WORK-1", expectedFingerprint: status.freshness.fingerprint,
      expectedOwner: status.tasks[0].canonicalOwner, action: "claim", owner: "project-owner" } });
  const claimed = await invoke("task-transition", root, "--request", claim);
  assert.equal(claimed.status, "applied");
  assert.equal(claimed.dashboard.status, "published");
  const current = await invoke("status", root);
  assert.equal(current.tasks[0].status, "in_progress");
  // This fixture has no prior checkpoint; checkpoint and operational versions are independent.
  const checkpoint = await request(root, "checkpoint", { schemaVersion: 1, actorSessionId: "project-owner", expectedVersion: 0,
    request: { eventId: "journey-checkpoint", sessionId: "project-owner", taskIds: ["WORK-1"], worktree: root,
      revision: execFileSync("git", ["rev-parse", "HEAD"], { cwd: root, encoding: "utf8" }).trim(), nextAction: "Run the required verification." } });
  assert.equal((await invoke("checkpoint", root, "--request", checkpoint)).status, "applied");
  assert.match(await readFile(path.join(root, ".agent-team/dashboard/index.html"), "utf8"), /WORK-1/);
  assert.equal((await invoke("recovery", root, "--session", "project-owner")).nextAction, "Run the required verification.");
  assert.equal((await invoke("readiness", root, "--host", "codex", "--scope", "user")).readyForDispatch, false);
});

test("initialization CLI cannot replace the envelope actor with a body owner claim", async () => {
  const root = await fixture();
  const value = planRequest();
  value.request.ownerSessionId = "forged-owner";
  const file = await request(root, "initialization", value);
  const result = await invoke("project-initialize", root, "--request", file);
  assert.equal(result.status, "applied");
  const teams = await readFile(path.join(root, ".agent-team/TEAMS.md"), "utf8");
  assert.match(teams, /Project owner: project-owner/);
  assert.doesNotMatch(teams, /forged-owner/);
});

test("initialization CLI rejects a nonzero version for absent setup without publishing records", async () => {
  const root = await fixture();
  const value = planRequest();
  value.expectedVersion = 7;
  const file = await request(root, "stale-initialization", value);
  const result = await invoke("project-initialize", root, "--request", file);
  assert.equal(result.status, "conflict");
  assert.equal(result.reason, "stale_setup_version");
  for (const name of ["TASKS.md", ".agent-team/setup.json", ".agent-team/state.json", ".agent-team/TEAMS.md", ".agent-team/.setup-initialization.json"]) {
    await assert.rejects(access(path.join(root, name)), { code: "ENOENT" });
  }
  value.expectedVersion = 0;
  await writeFile(file, JSON.stringify(value));
  assert.equal((await invoke("project-initialize", root, "--request", file)).status, "applied");
});
