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
    writer, evidencePaths: [evidencePath], resourceOwner: "agent-team", previewRequired: false, retain: false } };
  value.state.release.autoDeploy = false;
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

for (const kind of ["tracked", "untracked", "ignored", "user_owned", "unknown_writer", "preview"]) {
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
    if (kind === "preview") value.state.cleanup["AT-001"].previewRequired = true;
    await writeFile(value.project.paths.state, JSON.stringify(value.state));
    const result = await cleanup(value.project, value.request);
    assert.equal(result.status, "conflict");
    assert.match(result.reason, /retain|dirty|writer|preview|owner/);
    await access(value.feature);
  });
}
