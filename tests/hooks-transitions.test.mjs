import assert from "node:assert/strict";
import { spawn } from "node:child_process";
import { mkdir, mkdtemp, readFile, rename, rm, writeFile } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import test from "node:test";
import { fileURLToPath } from "node:url";
import { resolveProject } from "../hooks/lib/project.mjs";
import { loadCanonicalState } from "../hooks/lib/canonical-state.mjs";
import { writeCheckpoint } from "../hooks/lib/checkpoint.mjs";
import { createEventBudget } from "../hooks/lib/budget.mjs";
import { policyFixture } from "./hook-test-helpers.mjs";

const modulePath = new URL("../hooks/lib/task-transitions.mjs", import.meta.url);
if (process.argv[2] === "writer") {
  process.stdout.write("ready");
  setInterval(() => {}, 1000);
} else if (process.argv[2] === "claim-worker") {
  const { transitionTask } = await import(modulePath);
  const request = JSON.parse(process.argv[4]);
  const result = await transitionTask(await resolveProject(process.argv[3]), request);
  process.stdout.write(JSON.stringify(result));
} else {
  const temporary = [];
  test.afterEach(async () => Promise.all(temporary.splice(0).map((p) => rm(p, { recursive: true, force: true }))));
  async function fixture() {
    const root = await mkdtemp(path.join(os.tmpdir(), "agent-team-transitions-"));
    temporary.push(root, `${root}-feature`, `${root}-remote`);
    const value = await policyFixture(root);
    const project = await resolveProject(value.feature);
    await writeFile(project.paths.tasks, "| ID | Requirement / acceptance | Owner | Depends on | Status | Revision / evidence | Next action |\n| --- | --- | --- | --- | --- | --- | --- |\n| AT-001 | Build requested feature | none | none | ready | none | Claim. |\n| AT-002 | Independent work | none | none | ready | none | Claim. |\n| AT-003 | Dependent work | none | AT-001 | ready | none | Wait. |\n");
    return { ...value, project, canonical: await loadCanonicalState(project) };
  }
  async function api() {
    const module = await import(modulePath).catch(() => ({}));
    assert.equal(typeof module.transitionTask, "function", "canonical task transition implementation is required");
    return module;
  }
  function request(value, operationId, owner = "TEAM-001") {
    return { operationId, actorSessionId: "owner-session", expectedVersion: 0, taskId: "AT-001", expectedFingerprint: value.canonical.tracker.fingerprint, expectedOwner: "none", action: "claim", owner };
  }
  function child(root, request) {
    return new Promise((resolve, reject) => {
      const process_ = spawn(process.execPath, [fileURLToPath(import.meta.url), "claim-worker", root, JSON.stringify(request)], { stdio: ["ignore", "pipe", "pipe"] });
      let stdout = ""; let stderr = "";
      process_.stdout.on("data", (chunk) => { stdout += chunk; });
      process_.stderr.on("data", (chunk) => { stderr += chunk; });
      process_.on("error", reject);
      process_.on("close", (code) => code ? reject(new Error(stderr)) : resolve(JSON.parse(stdout)));
    });
  }

  test("two actual processes contend for one canonical claim and produce exactly one owner", async () => {
    await api();
    const value = await fixture();
    const results = await Promise.all([child(value.root, request(value, "claim-a")), child(value.root, request(value, "claim-b"))]);
    assert.deepEqual(results.map(({ status }) => status).sort(), ["applied", "conflict"]);
    const canonical = await loadCanonicalState(value.project);
    assert.equal(canonical.tasks[0].owner, "TEAM-001");
    assert.equal(canonical.tasks[0].status, "in_progress");
  });

  test("duplicate operation, stale version, stale owner and unknown writer are distinct", async () => {
    const { transitionTask } = await api();
    const value = await fixture();
    const first = await transitionTask(value.project, request(value, "claim-once"));
    const repeated = await transitionTask(value.project, request(value, "claim-once"));
    assert.equal(first.status, "applied");
    assert.equal(repeated.status, "duplicate");
    const stale = await transitionTask(value.project, request(value, "claim-stale"));
    assert.equal(stale.status, "conflict");
    const current = await loadCanonicalState(value.project);
    const wrongOwner = await transitionTask(value.project, { ...request(value, "stale-owner"), action: "pause", expectedVersion: current.state.stateVersion, expectedFingerprint: current.tracker.fingerprint });
    assert.equal(wrongOwner.reason, "stale_owner");
    const unknown = await transitionTask(value.project, { ...request(value, "unknown-actor"), actorSessionId: "unregistered-session" });
    assert.equal(unknown.status, "conflict");
    assert.equal(unknown.reason, "project_owner_required");
  });

  test("state mutation deadline returns uncertain and never starts a delayed rename stage", async () => {
    const { mutateOperationalState } = await api();
    const value = await fixture();
    const budget = createEventBudget(30);
    let renames = 0;
    let release;
    const held = new Promise((resolve) => { release = resolve; });
    try {
      const mutation = mutateOperationalState(value.project, { actorSessionId: "owner-session", operationId: "deadline-state", expectedVersion: 0 },
        (state) => ({ state, result: { recorded: true } }), { budget, filesystem: { writeFile: async (...args) => { await held; return writeFile(...args); }, rename: async (...args) => { renames += 1; return rename(...args); } } });
      const result = await Promise.race([mutation, new Promise((resolve) => setTimeout(() => resolve({ status: "test-timeout" }), 250))]);
      assert.equal(result.status, "unavailable");
      assert.equal(result.reason, "deadline");
    } finally { release(); budget.close(); }
    await new Promise((resolve) => setTimeout(resolve, 50));
    assert.equal(renames, 0);
  });

  test("eligibility respects dependencies, finite scope, project pause and reserved review capacity", async () => {
    const { taskEligibility } = await api();
    const value = await fixture();
    const options = { scopeTaskIds: ["AT-001", "AT-002", "AT-003"], capacity: { limit: 3, active: 1, reservedReview: 1 } };
    assert.deepEqual(taskEligibility(value.canonical, options).eligible.map(({ id }) => id), ["AT-001", "AT-002"]);
    assert.deepEqual(taskEligibility(value.canonical, { ...options, scopeTaskIds: ["AT-002"] }).eligible.map(({ id }) => id), ["AT-002"]);
    value.canonical.state.run = { paused: true };
    assert.deepEqual(taskEligibility(value.canonical, options).eligible, []);
    value.canonical.state.run.paused = false;
    assert.deepEqual(taskEligibility(value.canonical, { ...options, capacity: { limit: 2, active: 1, reservedReview: 1 } }).eligible, []);
  });

  test("parking requires a stopped matching writer, retains claim and gates, and frees independent admission", async () => {
    const { transitionTask, captureWriterIdentity, inspectWriterIdentity, taskEligibility } = await api();
    const value = await fixture();
    const worker = spawn(process.execPath, [fileURLToPath(import.meta.url), "writer"], { stdio: ["ignore", "pipe", "ignore"] });
    await new Promise((resolve) => worker.stdout.once("data", resolve));
    try {
      const writer = await captureWriterIdentity(worker.pid);
      assert.equal((await inspectWriterIdentity(writer)).status, "active");
      assert.equal((await inspectWriterIdentity({ ...writer, startTime: "reused" })).status, "unknown");
      await transitionTask(value.project, request(value, "claim-park"));
      const checkpoint = await writeCheckpoint(value.project, { eventId: "park-checkpoint", sessionId: "developer-session", taskIds: ["AT-001"], worktree: value.feature, revision: value.revision, nextAction: "Wait for prerequisite.json, then rerun tests." });
      let canonical = await loadCanonicalState(value.project);
      canonical.state.taskRuntime = { "AT-001": { compute: "active", writer, explicitPause: false } };
      await writeFile(value.project.paths.state, JSON.stringify(canonical.state));
      const park = { operationId: "park-one", actorSessionId: "owner-session", expectedVersion: canonical.state.stateVersion,
        taskId: "AT-001", expectedFingerprint: canonical.tracker.fingerprint, expectedOwner: "TEAM-001", action: "park", writer,
        checkpointPath: checkpoint.path, resumeWhen: "external-api-restored", repair: { attempts: 2, maxAttempts: 2 } };
      assert.equal((await transitionTask(value.project, park)).reason, "writer_not_stopped");
      const stopped = new Promise((resolve) => worker.once("exit", resolve)); worker.kill("SIGTERM"); await stopped;
      assert.equal((await inspectWriterIdentity(writer)).status, "stopped");
      const wrongCheckpoint = await writeCheckpoint(value.project, { eventId: "wrong-scope", sessionId: "other-session", taskIds: ["AT-001"], worktree: value.root, revision: value.revision, nextAction: "Wrong session." });
      assert.equal((await transitionTask(value.project, { ...park, checkpointPath: wrongCheckpoint.path })).reason, "checkpoint_identity_mismatch");
      assert.equal((await transitionTask(value.project, park)).status, "applied");
      canonical = await loadCanonicalState(value.project);
      assert.equal(canonical.tasks[0].owner, "TEAM-001");
      assert.equal(canonical.tasks[0].status, "parked");
      assert.equal(canonical.state.taskRuntime["AT-001"].compute, "parked");
      assert.deepEqual(canonical.state.release, value.state.release);
      assert.deepEqual(taskEligibility(canonical).eligible.map(({ id }) => id), ["AT-002"]);
      const evidencePath = path.join(value.root, ".agent-team/prerequisite.json");
      await writeFile(evidencePath, JSON.stringify({ taskId: "AT-001", condition: "external-api-restored", status: "passed", revision: value.revision }));
      const resume = { ...park, operationId: "resume-one", action: "resume", expectedVersion: canonical.state.stateVersion,
        expectedFingerprint: canonical.tracker.fingerprint, prerequisiteEvidence: evidencePath, writer: await captureWriterIdentity() };
      canonical.state.run = { paused: true };
      await writeFile(value.project.paths.state, JSON.stringify(canonical.state));
      assert.equal((await transitionTask(value.project, resume)).reason, "project_paused");
      canonical.state.run.paused = false;
      await writeFile(value.project.paths.state, JSON.stringify(canonical.state));
      const resumed = await transitionTask(value.project, resume);
      assert.equal(resumed.status, "applied");
      assert.equal((await loadCanonicalState(value.project)).state.taskRuntime["AT-001"].compute, "active");
    } finally { if (worker.exitCode === null && worker.signalCode === null) worker.kill("SIGTERM"); }
  });

  test("explicit task pause cannot be implicitly resumed or parked", async () => {
    const { transitionTask } = await api();
    const value = await fixture();
    await transitionTask(value.project, request(value, "claim-paused"));
    let canonical = await loadCanonicalState(value.project);
    const base = { actorSessionId: "owner-session", taskId: "AT-001", expectedOwner: "TEAM-001" };
    const pause = await transitionTask(value.project, { ...base, operationId: "pause", action: "pause", expectedVersion: canonical.state.stateVersion, expectedFingerprint: canonical.tracker.fingerprint });
    assert.equal(pause.status, "applied");
    canonical = await loadCanonicalState(value.project);
    for (const action of ["park", "resume"]) {
      const result = await transitionTask(value.project, { ...base, operationId: action, action, expectedVersion: canonical.state.stateVersion, expectedFingerprint: canonical.tracker.fingerprint });
      assert.equal(result.reason, "explicit_pause");
    }
    assert.equal((await loadCanonicalState(value.project)).tasks[0].status, "paused");
  });

  test("interrupted claim reconciles the canonical operation marker without repeating the tracker write", async () => {
    const { transitionTask } = await api();
    const value = await fixture();
    const wanted = request(value, "interrupted-claim");
    const failed = await transitionTask(value.project, wanted, { filesystem: { rename: async (from, to) => {
      if (to === value.project.paths.state) throw Object.assign(new Error("disk unavailable"), { code: "EIO" });
      return rename(from, to);
    } } });
    assert.equal(failed.status, "unavailable");
    const afterWrite = await readFile(value.project.paths.tasks, "utf8");
    assert.match(afterWrite, /TEAM-001.*in_progress/);
    let trackerWrites = 0;
    const recovered = await transitionTask(value.project, wanted, { filesystem: { rename: async (from, to) => {
      if (to === value.project.paths.tasks) trackerWrites += 1;
      return rename(from, to);
    } } });
    assert.equal(recovered.result.reconciled, true);
    assert.equal(trackerWrites, 0);
    assert.equal(await readFile(value.project.paths.tasks, "utf8"), afterWrite);
  });

  test("Beads backend failures preserve selected authority and never create Markdown claims", async () => {
    const { transitionTask } = await api();
    const value = await fixture();
    await writeFile(value.project.paths.setup, JSON.stringify({ skill: "agent-team", projectId: "project-1", tracker: { kind: "beads" } }));
    const project = await resolveProject(value.feature);
    const before = await readFile(path.join(value.root, ".agent-team/TASKS.md"), "utf8");
    const failed = await transitionTask(project, request(value, "beads-failed"), { runBeads: async () => { throw new Error("backend unavailable"); } });
    assert.equal(failed.status, "unavailable");
    assert.equal(await readFile(path.join(value.root, ".agent-team/TASKS.md"), "utf8"), before);
  });

  test("gate evidence writer binds exact canonical fingerprint and revision without granting authorization", async () => {
    const module = await api();
    assert.equal(typeof module.recordGateEvidence, "function");
    const value = await fixture();
    const evidencePath = path.join(value.root, ".agent-team/verification.json");
    await writeFile(evidencePath, JSON.stringify({ status: "passed", revision: value.revision, taskIds: ["AT-001"], checks: ["unit"] }));
    const wanted = { actorSessionId: "owner-session", operationId: "gate-evidence", expectedVersion: 0, expectedFingerprint: value.canonical.tracker.fingerprint,
      gate: "completion", taskIds: ["AT-001"], expectedRevision: value.revision, evidencePath };
    const result = await module.recordGateEvidence(value.project, wanted);
    assert.equal(result.status, "applied");
    const current = await loadCanonicalState(value.project);
    assert.equal(current.state.completion.trackerFingerprint, current.tracker.fingerprint);
    assert.equal(current.state.completion.authorized, value.state.completion.authorized);
    await writeFile(path.join(value.feature, "src/owned.js"), "dirty now\n");
    const changed = await module.recordGateEvidence(value.project, { ...wanted, operationId: "dirty-evidence", expectedVersion: current.state.stateVersion });
    assert.equal(changed.reason, "dirty_revision");
  });

  test("Beads atomic claim retains uncertain writes and reconciles its native operation note without retry", async () => {
    const { transitionTask } = await api();
    const value = await fixture();
    await writeFile(value.project.paths.setup, JSON.stringify({ skill: "agent-team", projectId: "project-1", tracker: { kind: "beads", executable: "/selected/bin/bd" } }));
    const project = await resolveProject(value.feature);
    const row = { id: "AT-001", title: "Native task", status: "open", assignee: "", dependency_count: 0, priority: 1 };
    let writes = 0;
    const runBeads = async (binary, args) => {
      assert.equal(binary, "/selected/bin/bd");
      if (args[0] === "list") return { stdout: JSON.stringify([row]) };
      assert.deepEqual(args.slice(0, 5), ["update", "AT-001", "--claim", "--actor", "TEAM-001"]);
      row.assignee = "TEAM-001"; row.status = "in_progress"; row.notes = args[args.indexOf("--append-notes") + 1]; writes += 1;
      throw Object.assign(new Error("connection lost after commit"), { killed: true });
    };
    const canonical = await loadCanonicalState(project, { runBeads });
    const wanted = { ...request(value, "native-claim"), expectedOwner: "", expectedFingerprint: canonical.tracker.fingerprint };
    const unknown = await transitionTask(project, wanted, { runBeads });
    assert.equal(unknown.status, "unavailable");
    assert.equal(unknown.reason, "tracker_write_uncertain");
    const state = JSON.parse(await readFile(project.paths.state, "utf8"));
    assert.equal(state.pendingOperations["native-claim"].phase, "uncertain");
    const recovered = await transitionTask(project, { ...wanted, expectedVersion: state.stateVersion }, { runBeads });
    assert.equal(recovered.result.reconciled, true);
    assert.equal(writes, 1);
  });
}
