import assert from "node:assert/strict";
import { spawn } from "node:child_process";
import { access, mkdir, mkdtemp, readFile, rename, rm, writeFile } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import test from "node:test";
import { resolveProject } from "../hooks/lib/project.mjs";
import { loadCanonicalState } from "../hooks/lib/canonical-state.mjs";
import { captureWriterIdentity } from "../hooks/lib/task-transitions.mjs";
import { policyFixture } from "./hook-test-helpers.mjs";

const temporary = [];
test.afterEach(async () => Promise.all(temporary.splice(0).map((p) => rm(p, { recursive: true, force: true }))));
const activeRun = (taskIds = ["AT-001"]) => ({ id: "cleanup-run", ownerSessionId: "owner-session", ownerHost: "codex", ownershipEpoch: 1,
  mode: "finite", taskIds, teamLimit: 1, autoDeploy: false, batchSize: 1, source: "explicit_run",
  settingSources: Object.fromEntries(["mode", "taskIds", "teamLimit", "autoDeploy", "batchSize"].map((key) => [key, "explicit_run"])),
  paused: false, operationalVersion: 0, blockers: [], pendingDeliveryIds: [], deployedTaskIds: [], terminalClassification: "progress_possible" });
async function fixture() {
  const root = await mkdtemp(path.join(os.tmpdir(), "agent-team-cleanup-"));
  temporary.push(root, `${root}-feature`, `${root}-remote`);
  const value = await policyFixture(root);
  const project = await resolveProject(root);
  const child = spawn(process.execPath, [path.join(import.meta.dirname, "hooks-transitions.test.mjs"), "writer"], { stdio: ["ignore", "pipe", "ignore"] });
  await new Promise((resolve) => child.stdout.once("data", resolve));
  const writer = await captureWriterIdentity(child.pid);
  const stopped = new Promise((resolve) => child.once("exit", resolve)); child.kill("SIGTERM"); await stopped;
  const evidencePath = path.join(root, ".agent-team/evidence/verified.json");
  await mkdir(path.dirname(evidencePath), { recursive: true });
  await writeFile(evidencePath, JSON.stringify({ status: "passed", revision: value.revision, taskId: "AT-001" }));
  await writeFile(project.paths.tasks, (await readFile(project.paths.tasks, "utf8")).replace("in_progress", "verified"));
  value.state.cleanup = { "AT-001": { worktree: value.feature, revision: value.revision, integrationRef: "main", verification: { status: "passed", revision: value.revision },
    writer, taskOwner: "TEAM-001", evidencePaths: [evidencePath], resourceOwner: "agent-team", previewRequired: false, retain: false } };
  value.state.taskRuntime = { "AT-001": { compute: "parked", writer, worktree: value.feature } };
  value.state.release.autoDeploy = false;
  value.state.run = activeRun();
  await writeFile(project.paths.state, JSON.stringify(value.state));
  return { ...value, project, writer, evidencePath, request: { actorSessionId: "owner-session", operationId: "cleanup-one", expectedVersion: 0, taskId: "AT-001", expectedRevision: value.revision, worktree: value.feature, expectedWriter: writer } };
}
async function cleanup(...args) {
  const module = await import("../hooks/lib/cleanup.mjs").catch(() => ({}));
  assert.equal(typeof module.cleanupDevelopmentWorktree, "function", "development cleanup implementation is required");
  return module.cleanupDevelopmentWorktree(...args);
}

test("verified integrated clean worktree is removed normally with deployment disabled and evidence retained", async () => {
  const value = await fixture();
  const result = await cleanup(value.project, value.request);
  assert.equal(result.status, "applied");
  await assert.rejects(access(value.feature), { code: "ENOENT" });
  await access(value.evidencePath);
  await access(value.project.paths.tasks);
  assert.equal((await loadCanonicalState(value.project)).state.release.autoDeploy, false);
  assert.equal((await cleanup(value.project, value.request)).status, "duplicate");
});

test("cleanup reconciles removal after a lost state receipt without another destructive command", async () => {
  const value = await fixture();
  let stateWrites = 0;
  const result = await cleanup(value.project, value.request, { filesystem: { rename: async (from, to) => {
    if (to === value.project.paths.state && ++stateWrites === 2) throw new Error("receipt unavailable");
    return rename(from, to);
  } } });
  assert.equal(result.status, "unavailable");
  await assert.rejects(access(value.feature), { code: "ENOENT" });
  const state = (await loadCanonicalState(value.project)).state;
  assert.equal(state.pendingOperations["cleanup-one"].phase, "uncertain");
  const recovered = await cleanup(value.project, { ...value.request, expectedVersion: state.stateVersion });
  assert.equal(recovered.status, "applied");
  assert.equal(recovered.result.reconciled, true);
});

test("outside-scope cleanup rejects before every probe and removal", async () => {
  const value = await fixture();
  const state = JSON.parse(await readFile(value.project.paths.state, "utf8"));
  state.run = activeRun(["AT-OTHER"]);
  await writeFile(value.project.paths.state, JSON.stringify(state));
  let gitProbes = 0;
  const before = await readFile(value.project.paths.state);
  const result = await cleanup(value.project, value.request, { runGit: async () => { gitProbes += 1; throw new Error("must not probe"); } });
  assert.deepEqual(result, { status: "conflict", reason: "outside_scope" });
  assert.equal(gitProbes, 0);
  assert.deepEqual(await readFile(value.project.paths.state), before);
  await access(value.feature);
});

test("cleanup rejects a missing or malformed admitted run before probes", async () => {
  for (const run of [undefined, { taskIds: ["AT-001"], paused: false }]) {
    const value = await fixture();
    const state = JSON.parse(await readFile(value.project.paths.state, "utf8"));
    if (run === undefined) delete state.run; else state.run = run;
    await writeFile(value.project.paths.state, JSON.stringify(state));
    let gitProbes = 0;
    const before = await readFile(value.project.paths.state);
    const result = await cleanup(value.project, value.request, { runGit: async () => { gitProbes += 1; throw new Error("must not probe"); } });
    assert.deepEqual(result, { status: "conflict", reason: "outside_scope" });
    assert.equal(gitProbes, 0);
    assert.deepEqual(await readFile(value.project.paths.state), before);
    await access(value.feature);
  }
});

test("in-scope cleanup still uses one locked tracker snapshot", async () => {
  const value = await fixture();
  const state = JSON.parse(await readFile(value.project.paths.state, "utf8"));
  state.run = activeRun();
  await writeFile(value.project.paths.state, JSON.stringify(state));
  assert.equal((await cleanup(value.project, value.request)).status, "applied");
  await assert.rejects(access(value.feature), { code: "ENOENT" });
});

for (const kind of ["tracked", "untracked", "ignored", "user_owned", "unknown_writer", "different_pid_namespace", "missing_pid_namespace", "preview"]) {
  test(`cleanup retains ${kind} resources`, async () => {
    const value = await fixture();
    if (kind === "tracked") await writeFile(path.join(value.feature, "src/owned.js"), "user change\n");
    if (kind === "untracked") await writeFile(path.join(value.feature, "USER.txt"), "user content\n");
    if (kind === "ignored") {
      await mkdir(path.join(value.feature, ".agent-team"));
      await writeFile(path.join(value.feature, ".agent-team/USER.txt"), "ignored content\n");
    }
    if (kind === "user_owned") value.state.cleanup["AT-001"].resourceOwner = "user";
    if (kind === "unknown_writer") value.state.cleanup["AT-001"].writer = { pid: process.pid };
    if (kind === "different_pid_namespace") value.writer.pidNamespace = "pid:[0]";
    if (kind === "missing_pid_namespace") delete value.writer.pidNamespace;
    if (kind === "preview") value.state.cleanup["AT-001"].previewRequired = true;
    await writeFile(value.project.paths.state, JSON.stringify(value.state));
    const result = await cleanup(value.project, value.request);
    assert.equal(result.status, "conflict");
    assert.match(result.reason, /retain|dirty|writer|preview|owner/);
    await access(value.feature);
  });
}

for (const kind of ["replacement_writer", "changed_owner", "other_task", "other_runtime", "missing_runtime"]) {
  test(`cleanup retains checkout when current ${kind} no longer matches the old cleanup assignment`, async () => {
    const value = await fixture();
    if (kind === "replacement_writer") value.state.taskRuntime["AT-001"].writer = await captureWriterIdentity();
    if (kind === "changed_owner") await writeFile(value.project.paths.tasks, (await readFile(value.project.paths.tasks, "utf8")).replace("TEAM-001", "TEAM-NEW"));
    if (kind === "other_task") await writeFile(value.project.paths.teams, (await readFile(value.project.paths.teams, "utf8")).replace("| AT-001 |", "| AT-001, AT-OTHER |"));
    if (kind === "other_runtime") value.state.taskRuntime["AT-OTHER"] = { worktree: value.feature, compute: "active", writer: await captureWriterIdentity() };
    if (kind === "missing_runtime") delete value.state.taskRuntime;
    await writeFile(value.project.paths.state, JSON.stringify(value.state));
    const result = await cleanup(value.project, value.request);
    assert.equal(result.status, "conflict");
    await access(value.feature);
    await access(value.evidencePath);
  });
}
