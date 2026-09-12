import assert from "node:assert/strict";
import { execFileSync, spawn } from "node:child_process";
import { createHash } from "node:crypto";
import { mkdir, mkdtemp, readFile, rename, rm, writeFile } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import test from "node:test";
import { fileURLToPath } from "node:url";
import { resolveProject } from "../hooks/lib/project.mjs";
import { loadCanonicalState } from "../hooks/lib/canonical-state.mjs";
import { writeCheckpoint } from "../hooks/lib/checkpoint.mjs";
import { createEventBudget } from "../hooks/lib/budget.mjs";
import { evaluatePolicy } from "../hooks/lib/policy.mjs";
import { hookEvent, policyFixture } from "./hook-test-helpers.mjs";

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
  async function fixture({ qualifiedOwnership = false } = {}) {
    const root = await mkdtemp(path.join(os.tmpdir(), "agent-team-transitions-"));
    temporary.push(root, `${root}-feature`, `${root}-remote`);
    const value = await policyFixture(root, { qualifiedOwnership });
    const project = await resolveProject(qualifiedOwnership ? root : value.feature);
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

  test("integration gate evidence maps only exact owner-authorized remote-base provenance", async (context) => {
    const invalid = [
      ["missing provenance", (evidence) => { delete evidence.authorization; }],
      ["malformed provenance", (evidence) => { evidence.authorization = { source: "" }; }],
      ["wrong owner", (evidence) => { evidence.authorization.ownerSessionId = "developer-session"; }],
      ["wrong revision", (evidence) => { evidence.authorization.revision = "deadbeef"; }],
      ["wrong task set", (evidence) => { evidence.authorization.taskIds = ["AT-002"]; }],
      ["substituted base ref", (evidence) => { evidence.remote.baseRef = "refs/heads/feature"; }],
      ["missing recovery", (evidence) => { delete evidence.recovery; }],
      ["missing deployment disposition", (evidence) => { delete evidence.remoteMainDeploys; }],
      ["failed artifact", (evidence) => { evidence.status = "failed"; }],
    ];
    for (const [name, invalidate] of invalid) await context.test(name, async () => {
      const { recordGateEvidence } = await api();
      const value = await fixture();
      const evidencePath = path.join(value.root, `.agent-team/${name.replaceAll(" ", "-")}.json`);
      const evidence = {
        status: "passed", revision: value.revision, taskIds: ["AT-001"],
        remote: { name: "origin", baseRef: "refs/heads/main", revision: value.revision, targetRef: "refs/heads/feature", targetRevision: value.revision },
        authorization: { source: "accepted-packet", scope: "integration", ownerSessionId: "owner-session", revision: value.revision, taskIds: ["AT-001"] },
        recovery: { status: "reconciled", revision: value.revision, taskIds: ["AT-001"] }, preview: { required: false }, remoteMainDeploys: false,
      };
      invalidate(evidence);
      await writeFile(evidencePath, JSON.stringify(evidence));
      const before = structuredClone((await loadCanonicalState(value.project)).state.integration);
      const result = await recordGateEvidence(value.project, { actorSessionId: "owner-session", operationId: `integration-${name.replaceAll(" ", "-")}`,
        expectedVersion: 0, expectedFingerprint: value.canonical.tracker.fingerprint, gate: "integration", taskIds: ["AT-001"], expectedRevision: value.revision, evidencePath });
      assert.equal(result.status, "conflict");
      assert.deepEqual((await loadCanonicalState(value.project)).state.integration, before);
    });

    const { recordGateEvidence } = await api();
    const value = await fixture();
    const evidencePath = path.join(value.root, ".agent-team/integration.json");
    await writeFile(evidencePath, JSON.stringify({
      status: "passed", revision: value.revision, taskIds: ["AT-001"],
      remote: { name: "origin", baseRef: "refs/heads/main", revision: value.revision, targetRef: "refs/heads/feature", targetRevision: value.revision },
      authorization: { source: "accepted-packet", scope: "integration", ownerSessionId: "owner-session", revision: value.revision, taskIds: ["AT-001"] },
      recovery: { status: "reconciled", revision: value.revision, taskIds: ["AT-001"] }, preview: { required: false }, remoteMainDeploys: false,
    }));
    const before = structuredClone((await loadCanonicalState(value.project)).state.integration);
    const result = await recordGateEvidence(value.project, { actorSessionId: "owner-session", operationId: "integration-evidence",
      expectedVersion: 0, expectedFingerprint: value.canonical.tracker.fingerprint, gate: "integration", taskIds: ["AT-001"], expectedRevision: value.revision, evidencePath });
    const integration = (await loadCanonicalState(value.project)).state.integration;
    assert.equal(result.status, "applied");
    assert.equal(integration.baseRef, before.baseRef);
    assert.equal(integration.ownerSessionId, before.ownerSessionId);
    assert.equal(integration.paused, before.paused);
    assert.equal(integration.hold, before.hold);
    assert.equal(integration.baseRevision, value.revision);
    assert.equal(integration.deltaClean, true);
    assert.equal(integration.recoveryReconciled, true);
    assert.equal(integration.updatesRemoteMain, false);
    assert.deepEqual({ ...integration.authorization, observedAt: undefined }, { source: "accepted-packet", scope: "integration", ownerSessionId: "owner-session", revision: value.revision, taskIds: ["AT-001"], observedAt: undefined });
    assert.equal(typeof integration.authorization.observedAt, "string");
  });

  test("integration evidence rejects an untracked non-ignored worktree file", async () => {
    const { recordGateEvidence } = await api();
    const value = await fixture();
    const evidencePath = path.join(value.root, ".agent-team/untracked-evidence.json");
    await writeFile(evidencePath, JSON.stringify({ status: "passed", revision: value.revision, taskIds: ["AT-001"],
      remote: { name: "origin", baseRef: "refs/heads/main", revision: value.revision, targetRef: "refs/heads/feature", targetRevision: value.revision },
      authorization: { source: "accepted-packet", scope: "integration", ownerSessionId: "owner-session", revision: value.revision, taskIds: ["AT-001"] },
      recovery: { status: "reconciled", revision: value.revision, taskIds: ["AT-001"] }, preview: { required: false }, remoteMainDeploys: false }));
    await writeFile(path.join(value.feature, "untracked-source.js"), "export {};\n");
    const result = await recordGateEvidence(value.project, { actorSessionId: "owner-session", operationId: "untracked-evidence", expectedVersion: 0,
      expectedFingerprint: value.canonical.tracker.fingerprint, gate: "integration", taskIds: ["AT-001"], expectedRevision: value.revision, evidencePath });
    assert.deepEqual(result, { status: "conflict", reason: "dirty_revision" });
  });

  test("integration evidence records an explicitly absent exact tag target", async () => {
    const { recordGateEvidence } = await api();
    const value = await fixture();
    const evidencePath = path.join(value.root, ".agent-team/tag-evidence.json");
    await writeFile(evidencePath, JSON.stringify({ status: "passed", revision: value.revision, taskIds: ["AT-001"],
      remote: { name: "origin", baseRef: "refs/heads/main", revision: value.revision, targetRef: "refs/tags/v7.1.0", targetAbsent: true },
      authorization: { source: "accepted-packet", scope: "integration", ownerSessionId: "owner-session", revision: value.revision, taskIds: ["AT-001"] },
      recovery: { status: "reconciled", revision: value.revision, taskIds: ["AT-001"] }, preview: { required: false }, remoteMainDeploys: false }));
    const result = await recordGateEvidence(value.project, { actorSessionId: "owner-session", operationId: "tag-evidence", expectedVersion: 0,
      expectedFingerprint: value.canonical.tracker.fingerprint, gate: "integration", taskIds: ["AT-001"], expectedRevision: value.revision, evidencePath });
    const integration = (await loadCanonicalState(value.project)).state.integration;
    assert.equal(result.status, "applied");
    assert.equal(integration.remoteRef, "refs/tags/v7.1.0");
    assert.equal(integration.targetAbsent, true);
    assert.equal(Object.hasOwn(integration, "remoteRevision"), false);
  });

  test("release gate evidence maps one owner-authorized batch from the initialized release hold", async () => {
    // This test catches release evidence that is receipted without establishing the policy fields it validated.
    const { recordGateEvidence } = await api();
    const value = await fixture({ qualifiedOwnership: true });
    const seeded = structuredClone((await loadCanonicalState(value.project)).state);
    seeded.release = { ownerSessionId: "owner-session", ownerHost: "codex", ownershipEpoch: 1, authorized: false, autoDeploy: false, hold: true };
    const integrationEvidencePath = path.join(value.root, ".agent-team/integration.json");
    Object.assign(seeded.integration, { taskIds: ["AT-001"], remoteRef: "refs/heads/main", remoteMainDeploys: true,
      recordedEvidence: { path: integrationEvidencePath, fingerprint: "c".repeat(64), revision: value.revision, taskIds: ["AT-001"] } });
    await writeFile(value.project.paths.state, JSON.stringify(seeded));
    const evidencePath = path.join(value.root, ".agent-team/release.json");
    const sha256 = "a".repeat(64);
    const checksumSha256 = "b".repeat(64);
    const evidence = {
      status: "passed", revision: value.revision, taskIds: ["AT-001"], ownerSessionId: "owner-session", authorized: true,
      expectedRevision: value.revision, target: "github:example/project:v1.0.0", process: "gh-release",
      authorization: { source: "explicit user authorization", target: "github:example/project:v1.0.0", process: "gh-release",
        scope: "batch-1", ownerSessionId: "owner-session", grantedAt: "2026-09-10" },
      run: { id: "release-1", mode: "auto_deploy", taskIds: ["AT-001"], paused: false },
      runMode: "auto_deploy", batchId: "batch-1", batch: { id: "batch-1", taskIds: ["AT-001"] },
      artifact: { id: `artifact-1:${sha256}`, revision: value.revision, taskIds: ["AT-001"], path: "/tmp/artifact-1.zip",
        bytes: 42, sha256, checksumPath: "/tmp/SHA256SUMS", checksumBytes: 80, checksumSha256,
        checksumEntry: `${sha256}  artifact-1.zip` },
      integration: { status: "passed", revision: value.revision, taskIds: ["AT-001"], recordedTaskIds: ["AT-001"],
        evidencePath: integrationEvidencePath, remoteName: "origin", baseRef: "refs/heads/main", baseRevision: value.revision,
        targetRef: "refs/heads/main", targetRevision: value.revision, remoteMainDeploys: true },
      verification: { status: "passed", revision: value.revision, taskIds: ["AT-001"] },
      preview: { required: false, status: "not_required", revision: value.revision },
      delta: { status: "clean", revision: value.revision, taskIds: ["AT-001"] },
      recovery: { status: "verified", artifactId: "git:known-good", action: "restore the known-good revision" },
      autoDeploy: true, projectPaused: false, hold: false,
    };
    await writeFile(evidencePath, JSON.stringify(evidence));

    const result = await recordGateEvidence(value.project, { actorSessionId: "owner-session", operationId: "release-evidence", expectedVersion: 0,
      expectedFingerprint: value.canonical.tracker.fingerprint, gate: "release", taskIds: ["AT-001"], expectedRevision: value.revision, evidencePath },
    { nativeIdentity: { host: "codex", sessionId: "owner-session", observed: true, cwd: value.root, ownershipEpoch: 1 } });
    const release = (await loadCanonicalState(value.project)).state.release;

    assert.equal(result.status, "applied");
    assert.equal(release.ownerSessionId, "owner-session");
    assert.equal(release.ownerHost, "codex");
    assert.equal(release.ownershipEpoch, 1);
    assert.equal(release.authorized, true);
    assert.equal(release.expectedRevision, value.revision);
    assert.equal(release.target, evidence.target);
    assert.equal(release.process, "gh-release");
    assert.equal(release.runMode, "auto_deploy");
    assert.equal(release.autoDeploy, true);
    assert.equal(release.projectPaused, false);
    assert.equal(release.hold, false);
    assert.equal(release.remoteMainDeploys, true);
    assert.deepEqual(release.taskIds, ["AT-001"]);
    assert.deepEqual(release.batch, evidence.batch);
    assert.deepEqual(release.artifact, evidence.artifact);
    assert.deepEqual(release.integration, { ...evidence.integration, evidenceFingerprint: "c".repeat(64) });
    assert.deepEqual(release.verification, evidence.verification);
    assert.deepEqual(release.preview, evidence.preview);
    assert.deepEqual(release.delta, evidence.delta);
    assert.deepEqual(release.recovery, evidence.recovery);
    assert.equal(release.authorization.ownerSessionId, "owner-session");
    assert.equal(release.authorization.revision, value.revision);
    assert.deepEqual(release.authorization.taskIds, ["AT-001"]);
    assert.equal(typeof release.evidenceAt, "string");
    assert.equal(release.trackerFingerprint, value.canonical.tracker.fingerprint);
    assert.equal(release.recordedEvidence.revision, value.revision);
    const policy = await evaluatePolicy(hookEvent(value, { sessionId: "owner-session", cwd: value.root,
      operation: { kind: "shell", command: "gh release create v1.0.0" } }), value.project, { now: new Date() });
    assert.equal(policy.allow, true, JSON.stringify(policy));
    for (const [field, replacement] of [["ownerHost", "claude-code"], ["ownershipEpoch", 2]]) {
      const changed = JSON.parse(await readFile(value.project.paths.state, "utf8"));
      changed.release[field] = replacement;
      await writeFile(value.project.paths.state, JSON.stringify(changed));
      const denied = await evaluatePolicy(hookEvent(value, { sessionId: "owner-session", cwd: value.root,
        operation: { kind: "shell", command: "gh release create v1.0.0" } }), value.project, { now: new Date() });
      assert.equal(denied.allow, false, field);
      changed.release[field] = release[field];
      await writeFile(value.project.paths.state, JSON.stringify(changed));
    }
  });

  test("release gate evidence rejects incomplete, mismatched, paused, held, and fabricated authority without changing state", async (context) => {
    const invalid = [
      ["missing authorization source", (evidence) => { delete evidence.authorization.source; }],
      ["blank authorization source", (evidence) => { evidence.authorization.source = " "; }],
      ["oversized authorization source", (evidence) => { evidence.authorization.source = "x".repeat(257); }],
      ["wrong owner", (evidence) => { evidence.ownerSessionId = "developer-session"; }],
      ["wrong authorization owner", (evidence) => { evidence.authorization.ownerSessionId = "developer-session"; }],
      ["wrong target", (evidence) => { evidence.authorization.target = "github:example/other:v1"; }],
      ["wrong process", (evidence) => { evidence.authorization.process = "npm"; }],
      ["wrong scope", (evidence) => { evidence.authorization.scope = "batch-2"; }],
      ["fabricated authorization", (evidence) => { evidence.authorized = false; }],
      ["wrong run mode pair", (evidence) => { evidence.autoDeploy = false; }],
      ["paused run", (evidence) => { evidence.run.paused = true; }],
      ["missing batch", (evidence) => { delete evidence.batch; }],
      ["wrong batch tasks", (evidence) => { evidence.batch.taskIds = ["AT-002"]; }],
      ["stale expected revision", (evidence) => { evidence.expectedRevision = "deadbeef"; }],
      ["missing artifact checksum", (evidence) => { delete evidence.artifact.sha256; }],
      ["stale artifact", (evidence) => { evidence.artifact.revision = "deadbeef"; }],
      ["stale integration", (evidence) => { evidence.integration.revision = "deadbeef"; }],
      ["stale verification", (evidence) => { evidence.verification.revision = "deadbeef"; }],
      ["stale preview", (evidence) => { evidence.preview.revision = "deadbeef"; }],
      ["stale delta", (evidence) => { evidence.delta.revision = "deadbeef"; }],
      ["malformed remote delta base", (evidence) => { evidence.delta.remoteBaseRevision = "fabricated"; }],
      ["missing remote deployment fact", (evidence) => { delete evidence.integration.remoteMainDeploys; }],
      ["wrong integration remote", (evidence) => { evidence.integration.remoteName = "backup"; }],
      ["wrong integration base ref", (evidence) => { evidence.integration.baseRef = "refs/heads/other"; }],
      ["wrong integration base revision", (evidence) => { evidence.integration.baseRevision = "b".repeat(40); }],
      ["wrong integration target", (evidence) => { evidence.integration.targetRef = "refs/tags/v9.9.9"; }],
      ["fabricated integration evidence path", (evidence) => { evidence.integration.evidencePath = "/tmp/fabricated-integration.json"; }],
      ["missing recovery", (evidence) => { delete evidence.recovery; }],
      ["paused project", (evidence) => { evidence.projectPaused = true; }],
      ["active hold", (evidence) => { evidence.hold = true; }],
    ];
    for (const [name, invalidate] of invalid) await context.test(name, async () => {
      const { recordGateEvidence } = await api();
      const value = await fixture();
      const seeded = structuredClone((await loadCanonicalState(value.project)).state);
      seeded.release = { ownerSessionId: "owner-session", authorized: false, autoDeploy: false, hold: true };
      const integrationEvidencePath = path.join(value.root, ".agent-team/integration.json");
      Object.assign(seeded.integration, { taskIds: ["AT-001"], remoteRef: "refs/heads/main", remoteMainDeploys: true,
        recordedEvidence: { path: integrationEvidencePath, fingerprint: "c".repeat(64), revision: value.revision, taskIds: ["AT-001"] } });
      await writeFile(value.project.paths.state, JSON.stringify(seeded));
      const evidence = { status: "passed", revision: value.revision, taskIds: ["AT-001"], ownerSessionId: "owner-session", authorized: true,
        expectedRevision: value.revision, target: "github:example/project:v1.0.0", process: "gh-release",
        authorization: { source: "explicit user authorization", target: "github:example/project:v1.0.0", process: "gh-release",
          scope: "batch-1", ownerSessionId: "owner-session", grantedAt: "2026-09-10" },
        run: { id: "release-1", mode: "auto_deploy", taskIds: ["AT-001"], paused: false }, runMode: "auto_deploy", autoDeploy: true,
        batchId: "batch-1", batch: { id: "batch-1", taskIds: ["AT-001"] },
        artifact: { id: "artifact-1", revision: value.revision, taskIds: ["AT-001"], sha256: "a".repeat(64) },
        integration: { status: "passed", revision: value.revision, taskIds: ["AT-001"], recordedTaskIds: ["AT-001"],
          evidencePath: integrationEvidencePath, remoteName: "origin", baseRef: "refs/heads/main", baseRevision: value.revision,
          targetRef: "refs/heads/main", targetRevision: value.revision, remoteMainDeploys: true },
        verification: { status: "passed", revision: value.revision, taskIds: ["AT-001"] },
        preview: { required: false, status: "not_required", revision: value.revision },
        delta: { status: "clean", revision: value.revision, taskIds: ["AT-001"] },
        recovery: { status: "verified", artifactId: "git:known-good", action: "restore the known-good revision" },
        projectPaused: false, hold: false };
      invalidate(evidence);
      const evidencePath = path.join(value.root, `.agent-team/release-${name.replaceAll(" ", "-")}.json`);
      await writeFile(evidencePath, JSON.stringify(evidence));
      const before = structuredClone((await loadCanonicalState(value.project)).state.release);
      const result = await recordGateEvidence(value.project, { actorSessionId: "owner-session", operationId: `release-${name.replaceAll(" ", "-")}`,
        expectedVersion: 0, expectedFingerprint: value.canonical.tracker.fingerprint, gate: "release", taskIds: ["AT-001"],
        expectedRevision: value.revision, evidencePath });
      assert.deepEqual(result, { status: "conflict", reason: "release_evidence_mismatch" }, name);
      assert.deepEqual((await loadCanonicalState(value.project)).state.release, before, name);
    });
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

  test("run mutations reuse qualified operational serialization and preserve unrelated state", async () => {
    const { startRun } = await import("../hooks/lib/run-state.mjs");
    const value = await fixture({ qualifiedOwnership: true });
    const untouched = structuredClone(value.canonical.state.database);
    const settingSources = Object.fromEntries(["mode", "taskIds", "teamLimit", "autoDeploy", "batchSize"].map((key) => [key, "explicit_run"]));
    const request = { operationId: "transition-run-start", expectedTrackerFingerprint: value.canonical.tracker.fingerprint, reason: "Start qualified fixture run.",
      run: { id: "transition-run", mode: "finite", taskIds: ["AT-001"], teamLimit: 1, autoDeploy: false, batchSize: 1, source: "explicit_run", settingSources } };
    const options = { actorSessionId: "owner-session", expectedVersion: value.canonical.state.stateVersion ?? 0,
      nativeIdentity: { host: "codex", sessionId: "owner-session", observed: true, cwd: value.root, ownershipEpoch: 1 } };
    const applied = await startRun(value.project, request, options);
    assert.equal(applied.status, "applied");
    assert.deepEqual((await loadCanonicalState(value.project)).state.database, untouched);
    assert.equal((await startRun(value.project, request, options)).status, "duplicate");
  });

  test("future task-keyed delivery receipts remain inert until exact joined", async () => {
    const value = await fixture({ qualifiedOwnership: true });
    value.canonical.state.deliveryReceipts = { completion: { "AT-001": { taskId: "AT-001", status: "passed", sourceRevision: value.revision } } };
    await writeFile(value.project.paths.state, JSON.stringify(value.canonical.state, null, 2));
    assert.deepEqual((await loadCanonicalState(value.project)).deliveryEvidence, {});
  });

  const nativeOptions = (value, expectedVersion = 0) => ({ actorSessionId: "owner-session", expectedVersion,
    nativeIdentity: { host: "codex", sessionId: "owner-session", observed: true, cwd: value.root, ownershipEpoch: 1 } });
  const stableJson = (value) => JSON.stringify(value && typeof value === "object" ? Array.isArray(value)
    ? value.map((entry) => JSON.parse(stableJson(entry)))
    : Object.fromEntries(Object.keys(value).sort().map((key) => [key, JSON.parse(stableJson(value[key]))])) : value);
  const fingerprint = (value) => createHash("sha256").update(stableJson(value)).digest("hex");

  test("completion and integration bind seven task-keyed receipts from one evidence read", async () => {
    const { recordGateEvidence } = await api();
    const value = await fixture({ qualifiedOwnership: true });
    value.canonical.state.run = { id: "delivery-run", ownerSessionId: "owner-session", ownerHost: "codex", ownershipEpoch: 1,
      mode: "finite", taskIds: ["AT-001"], teamLimit: 1, autoDeploy: true, batchSize: 1, source: "explicit_run",
      settingSources: Object.fromEntries(["mode", "taskIds", "teamLimit", "autoDeploy", "batchSize"].map((key) => [key, "explicit_run"])),
      paused: false, operationalVersion: 0, blockers: [], pendingDeliveryIds: ["AT-001"], deployedTaskIds: [], terminalClassification: "progress_possible" };
    await writeFile(value.project.paths.state, JSON.stringify(value.canonical.state, null, 2));
    const completionPath = path.join(value.root, ".agent-team/completion-rel004.json");
    await writeFile(completionPath, JSON.stringify({ status: "passed", revision: value.revision, taskIds: ["AT-001"], requirementsReconciled: true,
      review: { status: "passed", revision: value.revision, taskId: "AT-001" }, checks: [{ name: "unit", status: "passed", revision: value.revision, taskId: "AT-001" }] }));
    const completion = await recordGateEvidence(value.project, { actorSessionId: "owner-session", operationId: "rel004-completion", expectedVersion: 0,
      expectedFingerprint: value.canonical.tracker.fingerprint, gate: "completion", taskIds: ["AT-001"], expectedRevision: value.revision, evidencePath: completionPath }, nativeOptions(value));
    assert.equal(completion.status, "applied");
    const integrationPath = path.join(value.root, ".agent-team/integration-rel004.json");
    const actor = { ownerHost: "codex", ownerSessionId: "owner-session", ownershipEpoch: 1 };
    await writeFile(integrationPath, JSON.stringify({ status: "passed", revision: value.revision, taskIds: ["AT-001"], sourceRevisions: { "AT-001": value.revision },
      remote: { name: "origin", baseRef: "refs/heads/main", revision: value.revision, targetRef: "refs/heads/feature", targetRevision: value.revision },
      authorization: { source: "accepted-packet", scope: "integration", ownerSessionId: "owner-session", revision: value.revision, taskIds: ["AT-001"] },
      targetAuthorization: { status: "authorized", source: "explicit-release-authorization", target: "refs/heads/feature", revision: value.revision, taskIds: ["AT-001"], ...actor },
      recovery: { status: "reconciled", revision: value.revision, taskIds: ["AT-001"], artifactId: "git:known-good", action: "rollback" },
      preview: { required: false }, remoteMainDeploys: false }));
    const current = await loadCanonicalState(value.project);
    const integration = await recordGateEvidence(value.project, { actorSessionId: "owner-session", operationId: "rel004-integration", expectedVersion: current.state.stateVersion,
      expectedFingerprint: current.tracker.fingerprint, gate: "integration", taskIds: ["AT-001"], expectedRevision: value.revision, evidencePath: integrationPath }, nativeOptions(value, current.state.stateVersion));
    assert.equal(integration.status, "applied");
    const state = (await loadCanonicalState(value.project)).state;
    assert.deepEqual(Object.keys(state.deliveryReceipts).sort(), ["checks", "completion", "integration", "preview", "recovery", "review", "target"]);
    assert.equal(state.deliveryReceipts.recovery["AT-001"].artifact, "git:known-good");
    assert.equal(state.deliveryReceipts.integration["AT-001"].ownerHost, "codex");
    assert.deepEqual(state.deliveryReceipts.target["AT-001"].authority.taskIds, ["AT-001"]);
    assert.deepEqual(state.deliveryReceipts.completion["AT-001"].evidence, state.completion.recordedEvidence);
    assert.deepEqual(state.deliveryReceipts.integration["AT-001"].evidence, state.integration.recordedEvidence);
  });

  test("completion quarantine preserves history and clears only matching active evidence", async () => {
    const { quarantineCompletion } = await api();
    assert.equal(typeof quarantineCompletion, "function");
    const value = await fixture({ qualifiedOwnership: true });
    const evidencePath = path.join(value.root, ".agent-team/outside-completion.json");
    const bytes = Buffer.from(`${JSON.stringify({ taskId: "AT-002", taskIds: ["AT-002"], revision: value.revision, status: "passed" })}\n`);
    await writeFile(evidencePath, bytes);
    const pointer = { path: evidencePath, fingerprint: createHash("sha256").update(bytes).digest("hex"), revision: value.revision, taskIds: ["AT-002"] };
    value.canonical.state.run = { taskIds: ["AT-001"], paused: false };
    value.canonical.state.completion = { ...value.canonical.state.completion, taskId: "AT-002", recordedEvidence: pointer };
    await writeFile(value.project.paths.state, JSON.stringify(value.canonical.state, null, 2));
    const result = await quarantineCompletion(value.project, { operationId: "quarantine-at-002", taskId: "AT-002", reason: "task_outside_admitted_run_scope", expectedEvidence: pointer }, nativeOptions(value));
    assert.equal(result.status, "applied");
    const state = (await loadCanonicalState(value.project)).state;
    assert.equal(state.quarantinedEvidence.length, 1);
    assert.equal(Object.hasOwn(state.completion, "recordedEvidence"), false);
  });

  test("completion rebind accepts source boundary head ancestry without equality", async () => {
    const { rebindCompletion } = await api();
    assert.equal(typeof rebindCompletion, "function");
    const value = await fixture({ qualifiedOwnership: true });
    const sourceRevision = value.revision;
    execFileSync("git", ["commit", "--allow-empty", "-qm", "integration boundary"], { cwd: value.root });
    const boundaryRevision = execFileSync("git", ["rev-parse", "HEAD"], { cwd: value.root, encoding: "utf8" }).trim();
    execFileSync("git", ["commit", "--allow-empty", "-qm", "later head"], { cwd: value.root });
    const evidencePath = path.join(value.root, ".agent-team/rebind-completion.json");
    const evidenceBytes = Buffer.from(`${JSON.stringify({ status: "passed", revision: sourceRevision, taskId: "AT-001", requirementsReconciled: true,
      review: { status: "passed", revision: sourceRevision }, checks: [{ name: "unit", status: "passed", revision: sourceRevision }] })}\n`);
    await writeFile(evidencePath, evidenceBytes);
    const evidenceFingerprint = createHash("sha256").update(evidenceBytes).digest("hex");
    const quarantineId = `completion:AT-001:${evidenceFingerprint}`;
    value.canonical.state.run = { taskIds: ["AT-001"], paused: false };
    value.canonical.state.quarantinedEvidence = [{ quarantineId, taskId: "AT-001", evidence: { path: evidencePath, fingerprint: evidenceFingerprint,
      revision: sourceRevision, taskIds: ["AT-001"] } }];
    value.canonical.state.deliveryReceipts = { integration: { "AT-001": { taskId: "AT-001", status: "passed", sourceRevision,
      boundaryRevision, integratedRevision: boundaryRevision } } };
    await writeFile(value.project.paths.state, JSON.stringify(value.canonical.state, null, 2));
    const tracker = await loadCanonicalState(value.project);
    const result = await rebindCompletion(value.project, { operationId: "rebind-at-001", taskId: "AT-001", quarantineId,
      expectedTrackerFingerprint: tracker.tracker.fingerprint, evidencePath, expectedEvidenceFingerprint: evidenceFingerprint,
      expectedSourceRevision: sourceRevision, expectedBoundaryRevision: boundaryRevision }, nativeOptions(value));
    assert.equal(result.status, "applied", JSON.stringify(result));
    const state = (await loadCanonicalState(value.project)).state;
    assert.equal(state.quarantinedEvidence[0].reboundByOperationId, "rebind-at-001");
    assert.equal(state.deliveryReceipts.completion["AT-001"].sourceRevision, sourceRevision);
    assert.notEqual(result.result.currentRevision, boundaryRevision);
  });

  test("registered evidence store confines one-open historical reads", async () => {
    const { registerEvidenceStore } = await api();
    assert.equal(typeof registerEvidenceStore, "function");
    const value = await fixture({ qualifiedOwnership: true });
    const store = await mkdtemp(path.join(os.tmpdir(), "agent-team-evidence-store-")); temporary.push(store);
    const teams = `${await readFile(value.project.paths.teams, "utf8")}Evidence root: ${store}/<team>/.\n`;
    await writeFile(value.project.paths.teams, teams);
    const result = await registerEvidenceStore(value.project, { operationId: "register-store", storeId: "fixture-store", realpath: store,
      expectedTeamsFingerprint: createHash("sha256").update(teams).digest("hex"), declaration: `Evidence root: ${store}/<team>/.`,
      projectId: "project-1", expectedOwnershipEpoch: 1 }, nativeOptions(value));
    assert.equal(result.status, "applied");
    assert.equal((await loadCanonicalState(value.project)).state.evidenceStores["fixture-store"].realpath, store);
  });

  test("history reconciliation imports two sources at one boundary and preserves singleton", async () => {
    const { reconcileCompletionHistory, registerEvidenceStore } = await api();
    assert.equal(typeof reconcileCompletionHistory, "function");
    const value = await fixture({ qualifiedOwnership: true });
    const sourceA = value.revision;
    execFileSync("git", ["commit", "--allow-empty", "-qm", "second source"], { cwd: value.root });
    const sourceB = execFileSync("git", ["rev-parse", "HEAD"], { cwd: value.root, encoding: "utf8" }).trim();
    execFileSync("git", ["commit", "--allow-empty", "-qm", "shared boundary"], { cwd: value.root });
    const boundaryRevision = execFileSync("git", ["rev-parse", "HEAD"], { cwd: value.root, encoding: "utf8" }).trim();
    execFileSync("git", ["commit", "--allow-empty", "-qm", "later main"], { cwd: value.root });
    const headRevision = execFileSync("git", ["rev-parse", "HEAD"], { cwd: value.root, encoding: "utf8" }).trim();
    await writeFile(value.project.paths.tasks, "| ID | Requirement / acceptance | Owner | Depends on | Status | Revision / evidence | Next action | Type | Parent ID |\n| --- | --- | --- | --- | --- | --- | --- | --- | --- |\n| AT-001 | First | none | none | done | source | Reconcile. | task | |\n| AT-002 | Second | none | none | done | source | Reconcile. | task | |\n");
    const store = await mkdtemp(path.join(os.tmpdir(), "agent-team-history-store-")); temporary.push(store);
    const teams = `${await readFile(value.project.paths.teams, "utf8")}Evidence root: ${store}/<team>/.\n`;
    await writeFile(value.project.paths.teams, teams);
    const registered = await registerEvidenceStore(value.project, { operationId: "register-history-store", storeId: "history-store", realpath: store,
      expectedTeamsFingerprint: createHash("sha256").update(teams).digest("hex"), declaration: `Evidence root: ${store}/<team>/.`, projectId: "project-1",
      expectedOwnershipEpoch: 1 }, nativeOptions(value));
    assert.equal(registered.status, "applied");
    const actor = { ownerHost: "codex", ownerSessionId: "owner-session", ownershipEpoch: 1 };
    const authority = { status: "authorized", source: "migration-approval", target: "origin/main", revision: boundaryRevision,
      taskIds: ["AT-001", "AT-002"], ...actor };
    const integrationEvidence = { status: "passed", operationId: "historical-integration", revision: boundaryRevision, taskIds: ["AT-001", "AT-002"],
      sourceRevisions: { "AT-001": sourceA, "AT-002": sourceB }, preview: { required: false }, targetAuthorization: authority,
      recovery: { artifactId: "git:known-good", action: "rollback" } };
    const integrationBytes = Buffer.from(`${JSON.stringify(integrationEvidence)}\n`);
    await writeFile(path.join(store, "integration.json"), integrationBytes);
    let state = (await loadCanonicalState(value.project)).state;
    state.run = { id: "history-run", ownerSessionId: "owner-session", ownerHost: "codex", ownershipEpoch: 1, mode: "finite", taskIds: ["AT-001", "AT-002"],
      teamLimit: 2, autoDeploy: true, batchSize: 2, source: "compatibility_migration", settingSources: Object.fromEntries(["mode", "taskIds", "teamLimit", "autoDeploy", "batchSize"].map((key) => [key, "compatibility_migration"])),
      paused: false, operationalVersion: state.stateVersion, blockers: [], pendingDeliveryIds: [], deployedTaskIds: [], terminalClassification: "unreconciled_completion" };
    state.integration.taskIds = ["AT-001", "AT-002"];
    state.release.authorization = { ...state.release.authorization, ...actor };
    state.completion = { ...state.completion, taskId: "AT-009", operationId: "unrelated-completion", resultFingerprint: "9".repeat(64) };
    await writeFile(value.project.paths.state, JSON.stringify(state, null, 2));
    const singleton = structuredClone(state.completion);
    for (const [index, [taskId, sourceRevision]] of [["AT-001", sourceA], ["AT-002", sourceB]].entries()) {
      const resultFingerprint = String(index + 1).repeat(64);
      const completionOperationId = `historical-completion-${index + 1}`;
      const completion = { status: "passed", operationId: completionOperationId, resultFingerprint, revision: sourceRevision, taskId,
        requirementsReconciled: true, review: { status: "passed", revision: sourceRevision }, checks: [{ name: "unit", status: "passed", revision: sourceRevision }] };
      const completionBytes = Buffer.from(`${JSON.stringify(completion)}\n`);
      const completionRelativePath = `completion-${index + 1}.json`;
      await writeFile(path.join(store, completionRelativePath), completionBytes);
      const current = await loadCanonicalState(value.project);
      const request = { operationId: `history-${index + 1}`, taskId, intent: "record_existing_scope", completionOperationId,
        completionResultFingerprint: resultFingerprint, evidenceStoreId: "history-store", completionRelativePath,
        completionSha256: createHash("sha256").update(completionBytes).digest("hex"), sourceRevision,
        integrationOperationId: "historical-integration", integrationRelativePath: "integration.json",
        integrationSha256: createHash("sha256").update(integrationBytes).digest("hex"), boundaryRevision,
        expectedTrackerFingerprint: current.tracker.fingerprint, expectedStateFingerprint: fingerprint(current.state),
        expectedRunFingerprint: fingerprint(current.state.run), expectedTeamsFingerprint: createHash("sha256").update(teams).digest("hex"),
        expectedOwnershipEpoch: 1, expectedActiveCompletion: { taskId: "AT-009", operationId: "unrelated-completion", resultFingerprint: "9".repeat(64) },
        publicationTarget: "origin:refs/heads/main", observedRemoteRevision: headRevision };
      const imported = await reconcileCompletionHistory(value.project, request, { ...nativeOptions(value, current.state.stateVersion),
        observePublicationTarget: async () => ({ revision: headRevision }) });
      assert.equal(imported.status, "applied", JSON.stringify(imported));
    }
    const imported = await loadCanonicalState(value.project);
    assert.deepEqual(imported.state.completion, singleton);
    assert.equal(imported.state.completionHistory.length, 2);
    assert.deepEqual(imported.state.completionHistory[0].activeCompletion,
      { taskId: "AT-009", operationId: "unrelated-completion", resultFingerprint: "9".repeat(64) });
    assert.deepEqual([imported.state.deliveryReceipts.integration["AT-001"].ownerHost,
      imported.state.deliveryReceipts.target["AT-001"].ownerHost], ["codex", "codex"]);
    assert.deepEqual(imported.state.run.deployedTaskIds, ["AT-001", "AT-002"]);
    assert.equal(imported.deliveryEvidence["AT-001"].sourceRevision, sourceA);
    assert.equal(imported.deliveryEvidence["AT-002"].sourceRevision, sourceB);
  });

  test("history reconciliation drift and uncertain operation are byte identical", async () => {
    const { reconcileCompletionHistory } = await api();
    assert.equal(typeof reconcileCompletionHistory, "function");
    const value = await fixture({ qualifiedOwnership: true });
    value.canonical.state.run = { taskIds: ["AT-001"], paused: false };
    value.canonical.state.pendingOperations = { uncertain: { phase: "uncertain", taskId: "AT-001" } };
    await writeFile(value.project.paths.state, JSON.stringify(value.canonical.state, null, 2));
    const before = await readFile(value.project.paths.state);
    const invalid = await reconcileCompletionHistory(value.project, { operationId: "history-invalid" }, nativeOptions(value));
    assert.equal(invalid.reason, "invalid_request");
    assert.deepEqual(await readFile(value.project.paths.state), before);
  });
}
