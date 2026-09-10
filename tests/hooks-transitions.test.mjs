import assert from "node:assert/strict";
import { execFileSync, spawn } from "node:child_process";
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

  test("claim rejects unknown writers and preserves an existing runtime assignment", async () => {
    const { transitionTask, captureWriterIdentity } = await api();
    const writer = await captureWriterIdentity();
    for (const proposed of [null, { ...writer, startTime: "not-the-active-process" }]) {
      const value = await fixture();
      const before = await readFile(value.project.paths.tasks, "utf8");
      const result = await transitionTask(value.project, { ...request(value, "bad-writer"), writer: proposed });
      assert.equal(result.reason, "new_writer_unverified");
      assert.equal(await readFile(value.project.paths.tasks, "utf8"), before);
      assert.equal((await loadCanonicalState(value.project)).state.taskRuntime, undefined);
    }
    const value = await fixture();
    const assigned = { writer, compute: "active" };
    await writeFile(value.project.paths.state, JSON.stringify({ ...value.canonical.state, taskRuntime: { "AT-001": assigned } }));
    const result = await transitionTask(value.project, { ...request(value, "existing-writer"), writer });
    assert.equal(result.reason, "writer_already_registered");
    assert.deepEqual((await loadCanonicalState(value.project)).state.taskRuntime["AT-001"], assigned);
    assert.equal((await loadCanonicalState(value.project)).tasks[0].owner, "none");
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

  test("missing, blank or malformed registry owner never grants an absent actor mutation authority", async () => {
    const { mutateOperationalState } = await api();
    for (const source of [null, "", "Project: project-1\nProject owner:   \n", "Project: project-1\nProject owner: not an identity\n"]) {
      const value = await fixture();
      if (source === null) await rm(value.project.paths.teams); else await writeFile(value.project.paths.teams, source);
      let invoked = false;
      const result = await mutateOperationalState(value.project, { operationId: "missing-authority", expectedVersion: 0 }, () => { invoked = true; return {}; });
      assert.equal(result.status, "conflict");
      assert.equal(result.reason, "project_owner_required");
      assert.equal(invoked, false);
    }
    const value = await fixture();
    await writeFile(value.project.paths.teams, "Project: project-1\nProject owner: none\n");
    let invoked = false;
    const placeholder = await mutateOperationalState(value.project, { operationId: "placeholder-authority", actorSessionId: "none", expectedVersion: 0 }, () => { invoked = true; return {}; });
    assert.equal(placeholder.reason, "project_owner_required");
    assert.equal(invoked, false);
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

  for (const visibility of ["different_namespace", "legacy_namespace_missing"]) {
    test(`an absent PID with ${visibility} is unknown, not a stopped writer`, async () => {
      const { captureWriterIdentity, inspectWriterIdentity } = await api();
      const worker = spawn(process.execPath, [fileURLToPath(import.meta.url), "writer"], { stdio: ["ignore", "pipe", "ignore"] });
      await new Promise((resolve) => worker.stdout.once("data", resolve));
      let writer;
      try { writer = await captureWriterIdentity(worker.pid); }
      finally { const stopped = new Promise((resolve) => worker.once("exit", resolve)); worker.kill("SIGTERM"); await stopped; }
      if (visibility === "different_namespace") writer.pidNamespace = "pid:[0]";
      else delete writer.pidNamespace;
      assert.deepEqual(await inspectWriterIdentity(writer), { status: "unknown", reason: "pid_visibility_unavailable" });
    });
  }

  test("parking requires a stopped matching writer, retains claim and gates, and frees independent admission", async () => {
    const { transitionTask, captureWriterIdentity, inspectWriterIdentity, taskEligibility } = await api();
    const value = await fixture();
    const worker = spawn(process.execPath, [fileURLToPath(import.meta.url), "writer"], { stdio: ["ignore", "pipe", "ignore"] });
    await new Promise((resolve) => worker.stdout.once("data", resolve));
    try {
      const writer = await captureWriterIdentity(worker.pid);
      assert.equal((await inspectWriterIdentity(writer)).status, "active");
      assert.equal((await inspectWriterIdentity({ ...writer, startTime: "reused" })).status, "unknown");
      const claimed = await transitionTask(value.project, { ...request(value, "claim-park"), writer });
      assert.equal(claimed.status, "applied");
      assert.deepEqual((await loadCanonicalState(value.project)).state.taskRuntime["AT-001"].writer, writer);
      const checkpoint = await writeCheckpoint(value.project, { eventId: "park-checkpoint", sessionId: "developer-session", taskIds: ["AT-001"], worktree: value.feature, revision: value.revision, evidenceRevision: value.revision, nextAction: "Wait for prerequisite.json, then rerun tests." });
      let canonical = await loadCanonicalState(value.project);
      const park = { operationId: "park-one", actorSessionId: "owner-session", expectedVersion: canonical.state.stateVersion,
        taskId: "AT-001", expectedFingerprint: canonical.tracker.fingerprint, expectedOwner: "TEAM-001", action: "park", writer,
        checkpointPath: checkpoint.path, resumeWhen: "external-api-restored", repair: { attempts: 2, maxAttempts: 2 } };
      assert.equal((await transitionTask(value.project, park)).reason, "writer_not_stopped");
      const stopped = new Promise((resolve) => worker.once("exit", resolve)); worker.kill("SIGTERM"); await stopped;
      assert.equal((await inspectWriterIdentity(writer)).status, "stopped");
      const savedState = await readFile(value.project.paths.state, "utf8");
      const hiddenWriter = { ...writer, pidNamespace: "pid:[0]" };
      const hiddenState = JSON.parse(savedState);
      hiddenState.taskRuntime["AT-001"].writer = hiddenWriter;
      await writeFile(value.project.paths.state, JSON.stringify(hiddenState));
      assert.equal((await transitionTask(value.project, { ...park, writer: hiddenWriter })).reason, "writer_not_stopped");
      await writeFile(value.project.paths.state, savedState);
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

  test("explicit pause resumes from the stopped writer's authored checkpoint without editing operational state", async () => {
    const { transitionTask, captureWriterIdentity } = await api();
    const value = await fixture();
    const worker = spawn(process.execPath, [fileURLToPath(import.meta.url), "writer"], { stdio: ["ignore", "pipe", "ignore"] });
    await new Promise((resolve) => worker.stdout.once("data", resolve));
    try {
      const writer = await captureWriterIdentity(worker.pid);
      assert.equal((await transitionTask(value.project, { ...request(value, "claim-explicit-resume"), writer })).status, "applied");
      const checkpoint = await writeCheckpoint(value.project, { eventId: "pause-resume-checkpoint", sessionId: "developer-session",
        taskIds: ["AT-001"], worktree: value.feature, revision: value.revision, evidenceRevision: value.revision, nextAction: "Resume the approved implementation." });
      let canonical = await loadCanonicalState(value.project);
      const base = { actorSessionId: "owner-session", taskId: "AT-001", expectedOwner: "TEAM-001" };
      assert.equal((await transitionTask(value.project, { ...base, action: "pause", operationId: "explicit-pause",
        expectedVersion: canonical.state.stateVersion, expectedFingerprint: canonical.tracker.fingerprint })).status, "applied");
      const stopped = new Promise((resolve) => worker.once("exit", resolve)); worker.kill("SIGTERM"); await stopped;
      canonical = await loadCanonicalState(value.project);
      const resume = { ...base, action: "resume", operationId: "explicit-resume", explicitResume: true,
        expectedVersion: canonical.state.stateVersion, expectedFingerprint: canonical.tracker.fingerprint,
        writer: await captureWriterIdentity() };
      assert.equal((await transitionTask(value.project, resume)).reason, "checkpoint_required");
      const resumed = await transitionTask(value.project, { ...resume, checkpointPath: checkpoint.path });
      assert.equal(resumed.status, "applied", JSON.stringify(resumed));
      assert.equal((await loadCanonicalState(value.project)).state.taskRuntime["AT-001"].compute, "active");
    } finally { if (worker.exitCode === null && worker.signalCode === null) worker.kill("SIGTERM"); }
  });

  async function parkedFixture() {
    const module = await api();
    const value = await fixture();
    const worker = spawn(process.execPath, [fileURLToPath(import.meta.url), "writer"], { stdio: ["ignore", "pipe", "ignore"] });
    await new Promise((resolve) => worker.stdout.once("data", resolve));
    const writer = await module.captureWriterIdentity(worker.pid);
    const stopped = new Promise((resolve) => worker.once("exit", resolve)); worker.kill("SIGTERM"); await stopped;
    await module.transitionTask(value.project, request(value, "claim-for-resume"));
    const sourcePath = path.join(value.root, "DESIGN.md");
    await writeFile(sourcePath, "Approved original requirement.\n");
    const checkpoint = await writeCheckpoint(value.project, { eventId: "resume-record", sessionId: "developer-session", taskIds: ["AT-001"],
      worktree: value.feature, revision: value.revision, evidenceRevision: value.revision, nextAction: "Continue after API recovery.", sourcePointers: [{ kind: "constraint", path: sourcePath }] });
    let canonical = await loadCanonicalState(value.project);
    canonical.state.taskRuntime = { "AT-001": { compute: "active", writer } };
    await writeFile(value.project.paths.state, JSON.stringify(canonical.state));
    const park = { actorSessionId: "owner-session", operationId: "park-for-resume", taskId: "AT-001", expectedOwner: "TEAM-001", expectedVersion: canonical.state.stateVersion,
      expectedFingerprint: canonical.tracker.fingerprint, action: "park", writer, checkpointPath: checkpoint.path, repair: { attempts: 1, maxAttempts: 1 }, resumeWhen: "api-restored" };
    assert.equal((await module.transitionTask(value.project, park)).status, "applied");
    canonical = await loadCanonicalState(value.project);
    const prerequisiteEvidence = path.join(value.root, ".agent-team/prerequisite.json");
    await writeFile(prerequisiteEvidence, JSON.stringify({ taskId: "AT-001", status: "passed", revision: value.revision, condition: "api-restored" }));
    return { ...value, sourcePath, checkpoint, canonical, resume: { ...park, action: "resume", operationId: "resume", expectedVersion: canonical.state.stateVersion,
      expectedFingerprint: canonical.tracker.fingerprint, prerequisiteEvidence, writer: await module.captureWriterIdentity() } };
  }

  for (const supplied of ["current", "wrong_session", "stale_evidence"]) {
    test(`explicit paused resume validates the supplied ${supplied} checkpoint after session replacement`, async () => {
      const { transitionTask } = await api();
      const value = await parkedFixture();
      assert.equal((await transitionTask(value.project, { ...value.resume, action: "pause", operationId: "pause-before-session-replacement" })).status, "applied");
      const teams = await readFile(value.project.paths.teams, "utf8");
      await writeFile(value.project.paths.teams, teams.replace("developer-session", "replacement-session"));
      const checkpoint = await writeCheckpoint(value.project, { eventId: `replacement-${supplied}`,
        sessionId: supplied === "wrong_session" ? "unregistered-session" : "replacement-session",
        taskIds: ["AT-001"], worktree: value.feature, revision: value.revision,
        evidenceRevision: supplied === "stale_evidence" ? "a".repeat(40) : value.revision,
        nextAction: "Resume approved work in the replacement session." });
      const canonical = await loadCanonicalState(value.project);
      const before = await readFile(value.project.paths.state, "utf8");
      const result = await transitionTask(value.project, { ...value.resume, explicitResume: true,
        expectedVersion: canonical.state.stateVersion, expectedFingerprint: canonical.tracker.fingerprint, checkpointPath: checkpoint.path });
      if (supplied === "current") {
        assert.equal(result.status, "applied", JSON.stringify(result));
        const runtime = (await loadCanonicalState(value.project)).state.taskRuntime["AT-001"];
        assert.equal(runtime.checkpointPath, checkpoint.path);
        assert.equal(runtime.compute, "active");
        assert.equal(runtime.explicitPause, false);
      } else {
        assert.equal(result.reason, supplied === "wrong_session" ? "checkpoint_identity_mismatch" : "stale_resume_evidence");
        assert.equal(await readFile(value.project.paths.state, "utf8"), before);
      }
    });
  }

  test("parked resume retains its stored checkpoint despite a supplied replacement", async () => {
    const { transitionTask } = await api();
    const value = await parkedFixture();
    const checkpoint = await writeCheckpoint(value.project, { eventId: "unregistered-replacement", sessionId: "other-session",
      taskIds: ["AT-001"], worktree: value.feature, revision: value.revision, evidenceRevision: value.revision, nextAction: "Wrong session." });
    assert.equal((await transitionTask(value.project, { ...value.resume, explicitResume: true, checkpointPath: checkpoint.path })).status, "applied");
    assert.equal((await loadCanonicalState(value.project)).state.taskRuntime["AT-001"].checkpointPath, value.checkpoint.path);
  });

  for (const change of ["original_source", "dirty_git", "new_head", "evidence_revision"]) {
    test(`resume holds ${change} evidence changes before tracker or runtime mutation`, async () => {
      const { transitionTask } = await api();
      const value = await parkedFixture();
      if (change === "original_source") await writeFile(value.sourcePath, "Changed original requirement.\n");
      if (["dirty_git", "new_head"].includes(change)) await writeFile(path.join(value.feature, "src/owned.js"), "export const changed = true;\n");
      if (change === "new_head") {
        execFileSync("git", ["add", "src/owned.js"], { cwd: value.feature });
        execFileSync("git", ["commit", "-qm", "changed head"], { cwd: value.feature });
      }
      if (change === "evidence_revision") {
        const checkpoint = JSON.parse(await readFile(value.checkpoint.path, "utf8"));
        checkpoint.evidenceRevision = "a".repeat(40);
        await writeFile(value.checkpoint.path, JSON.stringify(checkpoint));
      }
      const before = await readFile(value.project.paths.tasks, "utf8");
      const result = await transitionTask(value.project, value.resume);
      assert.equal(result.reason, "stale_resume_evidence");
      assert.equal(await readFile(value.project.paths.tasks, "utf8"), before);
      assert.equal((await loadCanonicalState(value.project)).state.taskRuntime["AT-001"].compute, "parked");
    });
  }

  for (const scope of ["independent_task", "shared_worktree", "same_task", "global_unknown"]) {
    test(`resume scopes ${scope} pending uncertainty without treating unrelated tasks as a project pause`, async () => {
      const { transitionTask } = await api();
      const value = await parkedFixture();
      const entry = { phase: "uncertain", kind: "external_operation" };
      if (scope === "independent_task") entry.taskId = "AT-002";
      if (scope === "shared_worktree") { entry.taskId = "AT-OTHER"; entry.worktree = value.feature; }
      if (scope === "same_task") entry.taskId = "AT-001";
      value.canonical.state.pendingOperations = { "pending-operation": entry };
      await writeFile(value.project.paths.state, JSON.stringify(value.canonical.state));
      const result = await transitionTask(value.project, value.resume);
      if (scope === "independent_task") assert.equal(result.status, "applied");
      else {
        assert.ok(["pending_task_operation", "operation_reconciliation_required"].includes(result.reason));
        assert.equal((await loadCanonicalState(value.project)).state.taskRuntime["AT-001"].compute, "parked");
      }
    });
  }

  test("interrupted claim reconciles the canonical operation marker without repeating the tracker write", async () => {
    const { transitionTask, captureWriterIdentity } = await api();
    const value = await fixture();
    const writer = await captureWriterIdentity();
    const wanted = { ...request(value, "interrupted-claim"), writer };
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
    assert.deepEqual((await loadCanonicalState(value.project)).state.taskRuntime["AT-001"].writer, writer);
  });

  test("interrupted claim restores an exited original writer as stopped, never active", async () => {
    const { transitionTask, captureWriterIdentity } = await api();
    const value = await fixture();
    const worker = spawn(process.execPath, [fileURLToPath(import.meta.url), "writer"], { stdio: ["ignore", "pipe", "ignore"] });
    await new Promise((resolve) => worker.stdout.once("data", resolve));
    try {
      const writer = await captureWriterIdentity(worker.pid);
      const wanted = { ...request(value, "interrupted-stopped-writer"), writer };
      const result = await transitionTask(value.project, wanted, { filesystem: { rename: async (from, to) => {
        if (to === value.project.paths.state) throw Object.assign(new Error("disk unavailable"), { code: "EIO" });
        return rename(from, to);
      } } });
      assert.equal(result.status, "unavailable");
      const stopped = new Promise((resolve) => worker.once("exit", resolve)); worker.kill("SIGTERM"); await stopped;
      const recovered = await transitionTask(value.project, wanted);
      assert.equal(recovered.status, "applied");
      assert.equal(recovered.result.reconciled, true);
      const canonical = await loadCanonicalState(value.project);
      assert.deepEqual(canonical.state.taskRuntime["AT-001"].writer, writer);
      assert.equal(canonical.state.taskRuntime["AT-001"].compute, "stopped");
      assert.equal(canonical.tasks[0].owner, "TEAM-001");
      const { readStatus } = await import("../hooks/lib/status.mjs");
      const status = await readStatus(value.project);
      assert.equal(status.tasks.find(({ id }) => id === "AT-001").runtime.compute, "stopped");
      assert.equal(status.activity.active, 0);
    } finally { if (worker.exitCode === null && worker.signalCode === null) worker.kill("SIGTERM"); }
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

  test("completion gate evidence maps one HEAD-bound task's reconciled review and checks", async () => {
    const module = await api();
    assert.equal(typeof module.recordGateEvidence, "function");
    const value = await fixture();
    const evidencePath = path.join(value.root, ".agent-team/verification.json");
    await writeFile(evidencePath, JSON.stringify({
      status: "passed", revision: value.revision, taskIds: ["AT-001"], requirementsReconciled: true,
      review: { status: "passed", revision: value.revision, taskId: "AT-001" },
      checks: [{ name: "unit", status: "passed", revision: value.revision, taskId: "AT-001" }],
    }));
    const wanted = { actorSessionId: "owner-session", operationId: "gate-evidence", expectedVersion: 0, expectedFingerprint: value.canonical.tracker.fingerprint,
      gate: "completion", taskIds: ["AT-001"], expectedRevision: value.revision, evidencePath };
    const result = await module.recordGateEvidence(value.project, wanted);
    assert.equal(result.status, "applied");
    const current = await loadCanonicalState(value.project);
    assert.equal(current.state.completion.trackerFingerprint, current.tracker.fingerprint);
    assert.equal(current.state.completion.taskId, "AT-001");
    assert.equal(current.state.completion.evidenceRevision, value.revision);
    assert.equal(current.state.completion.requirementsReconciled, true);
    assert.deepEqual(current.state.completion.review, { status: "passed", revision: value.revision, taskId: "AT-001" });
    assert.deepEqual(current.state.completion.checks, [{ name: "unit", status: "passed", revision: value.revision, taskId: "AT-001" }]);
    assert.equal(current.state.completion.authorized, value.state.completion.authorized);
    await writeFile(path.join(value.feature, "src/owned.js"), "dirty now\n");
    const changed = await module.recordGateEvidence(value.project, { ...wanted, operationId: "dirty-evidence", expectedVersion: current.state.stateVersion });
    assert.equal(changed.reason, "dirty_revision");
  });

  test("completion gate evidence rejects invalid reconciled review and check records without changing completion", async () => {
    const { recordGateEvidence } = await api();
    const artifact = (value) => ({ status: "passed", revision: value.revision, taskIds: ["AT-001"], requirementsReconciled: true,
      review: { status: "passed", revision: value.revision, taskId: "AT-001" },
      checks: [{ name: "unit", status: "passed", revision: value.revision, taskId: "AT-001" }] });
    const invalid = [
      ["multiple tasks", (evidence) => { evidence.taskIds = ["AT-001", "AT-002"]; }, ["AT-001", "AT-002"], "completion_task_count"],
      ["false reconciliation", (evidence) => { evidence.requirementsReconciled = false; }],
      ["failed review", (evidence) => { evidence.review.status = "failed"; }],
      ["stale review", (evidence) => { evidence.review.revision = "stale"; }],
      ["empty checks", (evidence) => { evidence.checks = []; }],
      ["failed check", (evidence) => { evidence.checks[0].status = "failed"; }],
      ["stale check", (evidence) => { evidence.checks[0].revision = "stale"; }],
      ["mismatched check", (evidence) => { evidence.checks[0].taskId = "AT-002"; }],
      ["blank check name", (evidence) => { evidence.checks[0].name = " "; }],
      ["untrimmed check name", (evidence) => { evidence.checks[0].name = " unit "; }],
      ["non-string check name", (evidence) => { evidence.checks[0].name = true; }],
    ];
    for (const [name, invalidate, taskIds = ["AT-001"], reason = "completion_evidence_mismatch"] of invalid) {
      const value = await fixture();
      const evidencePath = path.join(value.root, ".agent-team/verification.json");
      const evidence = artifact(value);
      invalidate(evidence);
      await writeFile(evidencePath, JSON.stringify(evidence));
      const before = structuredClone((await loadCanonicalState(value.project)).state.completion);
      const result = await recordGateEvidence(value.project, { actorSessionId: "owner-session", operationId: `bad-gate-${name.replaceAll(" ", "-")}`,
        expectedVersion: 0, expectedFingerprint: value.canonical.tracker.fingerprint, gate: "completion", taskIds,
        expectedRevision: value.revision, evidencePath });
      assert.equal(result.reason, reason, name);
      assert.deepEqual((await loadCanonicalState(value.project)).state.completion, before, name);
    }
  });

  test("Beads atomic claim retains uncertain writes and reconciles its native operation note without retry", async () => {
    const { transitionTask } = await api();
    const value = await fixture();
    await writeFile(value.project.paths.setup, JSON.stringify({ skill: "agent-team", projectId: "project-1", tracker: { kind: "beads", executable: "/selected/bin/bd" } }));
    const project = await resolveProject(value.feature);
    const row = { id: "AT-001", title: "Native task", status: "open", assignee: "", dependency_count: 0, priority: 1 };
    let writes = 0;
    const environment = { ...process.env, BEADS_DB: "/wrong/db", BD_DB: "/wrong/db2", BEADS_DOLT_SERVER_PORT: "7777", BEADS_DOLT_SERVER_PASSWORD: "fixture-auth" };
    const runBeads = async (binary, args, options) => {
      assert.equal(binary, "/selected/bin/bd");
      assert.equal(options.env.BEADS_DB, undefined);
      assert.equal(options.env.BD_DB, undefined);
      assert.equal(options.env.BEADS_DOLT_SERVER_PORT, undefined);
      assert.equal(options.env.BEADS_DOLT_SERVER_PASSWORD, "fixture-auth");
      if (args[0] === "list") return { stdout: JSON.stringify([row]) };
      assert.deepEqual(args.slice(0, 5), ["update", "AT-001", "--claim", "--actor", "TEAM-001"]);
      row.assignee = "TEAM-001"; row.status = "in_progress"; row.notes = args[args.indexOf("--append-notes") + 1]; writes += 1;
      throw Object.assign(new Error("connection lost after commit"), { killed: true });
    };
    const canonical = await loadCanonicalState(project, { runBeads, environment });
    const wanted = { ...request(value, "native-claim"), expectedOwner: "", expectedFingerprint: canonical.tracker.fingerprint };
    const unknown = await transitionTask(project, wanted, { runBeads, environment });
    assert.equal(unknown.status, "unavailable");
    assert.equal(unknown.reason, "tracker_write_uncertain");
    const state = JSON.parse(await readFile(project.paths.state, "utf8"));
    assert.equal(state.pendingOperations["native-claim"].phase, "uncertain");
    const recovered = await transitionTask(project, { ...wanted, expectedVersion: state.stateVersion }, { runBeads, environment });
    assert.equal(recovered.result.reconciled, true);
    assert.equal(writes, 1);
  });

  for (const interruption of ["reply", "receipt"]) {
    test(`interrupted Beads pause preserves explicit pause and holds competing task transition after lost ${interruption}`, async () => {
      const { transitionTask } = await api();
      const value = await fixture();
      await writeFile(value.project.paths.setup, JSON.stringify({ skill: "agent-team", projectId: "project-1", tracker: { kind: "beads", writerMode: "single_owner", writerSessionId: "owner-session" } }));
      const project = await resolveProject(value.feature);
      const row = { id: "AT-001", title: "Native task", status: "in_progress", assignee: "TEAM-001", dependency_count: 0 };
      let writes = 0; let stateWrites = 0;
      const runBeads = async (_binary, args) => {
        if (args[0] === "list") return { stdout: JSON.stringify([row]) };
        row.status = args[args.indexOf("--status") + 1]; row.notes = args[args.indexOf("--append-notes") + 1]; writes += 1;
        if (interruption === "reply") throw new Error("reply lost after native commit");
        return { stdout: JSON.stringify([row]) };
      };
      const canonical = await loadCanonicalState(project, { runBeads });
      const wanted = { actorSessionId: "owner-session", operationId: `pause-${interruption}`, expectedVersion: 0, taskId: "AT-001", expectedOwner: "TEAM-001", expectedFingerprint: canonical.tracker.fingerprint, action: "pause" };
      const result = await transitionTask(project, wanted, { runBeads, filesystem: { rename: async (from, to) => {
        if (to === project.paths.state && ++stateWrites === 2 && interruption === "receipt") throw new Error("receipt lost");
        return rename(from, to);
      } } });
      assert.equal(result.status, "unavailable");
      let current = await loadCanonicalState(project, { runBeads });
      assert.equal(current.state.taskRuntime["AT-001"].explicitPause, true);
      assert.equal(current.state.pendingOperations[wanted.operationId].action, "pause");
      const competing = await transitionTask(project, { ...wanted, operationId: "competing-park", action: "park", expectedVersion: current.state.stateVersion, expectedFingerprint: current.tracker.fingerprint }, { runBeads });
      assert.equal(competing.reason, "pending_task_operation");
      const reconciled = await transitionTask(project, { ...wanted, expectedVersion: current.state.stateVersion }, { runBeads });
      assert.equal(reconciled.result.reconciled, true);
      current = await loadCanonicalState(project, { runBeads });
      assert.equal(current.state.taskRuntime["AT-001"].explicitPause, true);
      assert.equal(writes, 1);
    });
  }
}
