import assert from "node:assert/strict";
import { execFileSync, spawn } from "node:child_process";
import { access, link, mkdir, mkdtemp, readFile, readdir, rm, writeFile } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import test from "node:test";
import { fileURLToPath } from "node:url";
import { resolveProject } from "../hooks/lib/project.mjs";
import { loadCanonicalState } from "../hooks/lib/canonical-state.mjs";

const modulePath = new URL("../hooks/lib/initialization.mjs", import.meta.url);
if (process.argv[2] === "initialize-worker") {
  const { initializeProject } = await import(modulePath);
  process.stdout.write(JSON.stringify(await initializeProject(process.argv[3], JSON.parse(process.argv[4]))));
} else if (process.argv[2] === "crash-worker") {
  const { initializeProject } = await import(modulePath);
  await initializeProject(process.argv[3], JSON.parse(process.argv[4]), { filesystem: { link: async (from, to) => {
    if (to.endsWith("state.json")) { process.stdout.write("held"); await new Promise(() => { setInterval(() => {}, 1000); }); }
    return link(from, to);
  } } });
} else {
  const temporary = [];
  test.afterEach(async () => Promise.all(temporary.splice(0).map((directory) => rm(directory, { recursive: true, force: true }))));
  async function fixture() {
    const root = await mkdtemp(path.join(os.tmpdir(), "agent-team-initialization-"));
    temporary.push(root, `${root}-feature`);
    execFileSync("git", ["init", "-q", "-b", "main", root]);
    execFileSync("git", ["config", "user.name", "Initialization Test"], { cwd: root });
    execFileSync("git", ["config", "user.email", "init@example.test"], { cwd: root });
    await writeFile(path.join(root, "README.md"), "Existing user content.\n");
    execFileSync("git", ["add", "README.md"], { cwd: root });
    execFileSync("git", ["commit", "-qm", "fixture"], { cwd: root });
    const feature = `${root}-feature`;
    execFileSync("git", ["worktree", "add", "-q", "-b", "feature", feature], { cwd: root });
    const request = { projectId: "project-init", ownerSessionId: "owner-session", operationId: "initialize-1", source: "standalone",
      tracker: { kind: "markdown", path: ".agent-team/TASKS.md" }, plan: { scope: "Implement the approved feature.", acceptance: ["Tests cover the feature."],
        branch: "main", verification: ["node --test tests/*.test.mjs"], authority: { ownedPaths: ["src/**", "tests/**"], externalActions: [] },
        tasks: [{ id: "AT-001", title: "Implement feature", status: "ready", dependencies: [], acceptance: ["Tests cover the feature."] }] } };
    return { root, feature, request };
  }
  async function initialize(...args) {
    const module = await import(modulePath).catch(() => ({}));
    assert.equal(typeof module.initializeProject, "function", "canonical initialization implementation is required");
    return module.initializeProject(...args);
  }
  function worker(root, request) {
    return new Promise((resolve, reject) => {
      const child = spawn(process.execPath, [fileURLToPath(import.meta.url), "initialize-worker", root, JSON.stringify(request)], { stdio: ["ignore", "pipe", "pipe"] });
      let stdout = ""; let stderr = "";
      child.stdout.on("data", (chunk) => { stdout += chunk; });
      child.stderr.on("data", (chunk) => { stderr += chunk; });
      child.once("error", reject);
      child.once("close", (code) => code ? reject(new Error(stderr)) : resolve(JSON.parse(stdout)));
    });
  }

  test("standalone initialization publishes canonical records from linked worktree and never grants verified gates", async () => {
    const value = await fixture();
    const result = await initialize(value.feature, value.request);
    assert.equal(result.status, "applied");
    assert.equal(result.ready, true);
    const main = await resolveProject(value.root);
    const linked = await resolveProject(value.feature);
    assert.equal(main.root, linked.root);
    assert.equal(main.setup.plan.branch, "main");
    const canonical = await loadCanonicalState(linked);
    assert.equal(canonical.registry.projectOwner, "owner-session");
    assert.deepEqual(canonical.tasks.map(({ id, owner, status }) => ({ id, owner, status })), [{ id: "AT-001", owner: "none", status: "ready" }]);
    assert.equal(canonical.state.release.authorized, false);
    assert.equal(canonical.state.release.autoDeploy, false);
    assert.equal(canonical.state.integration.authorized, false);
    assert.equal(canonical.state.completion.requirementsReconciled, false);
    assert.equal(execFileSync("git", ["branch", "--show-current"], { cwd: value.feature, encoding: "utf8" }).trim(), "feature");
    assert.equal(await readFile(path.join(value.root, "README.md"), "utf8"), "Existing user content.\n");
    await assert.rejects(access(path.join(value.feature, ".agent-team/setup.json")), { code: "ENOENT" });
  });

  test("standalone initialization accepts 101 tasks", async () => {
    const value = await fixture();
    value.request.plan.tasks = Array.from({ length: 101 }, (_, index) => ({
      id: `AT-${String(index + 1).padStart(3, "0")}`,
      title: `Implement approved task ${index + 1}`,
      status: "ready",
      dependencies: [],
      acceptance: ["Pass the task checks."],
    }));
    const result = await initialize(value.root, value.request);
    assert.equal(result.status, "applied");
    assert.equal((await loadCanonicalState(await resolveProject(value.root))).tasks.length, 101);
  });

  test("standalone initialization and its receipt support 500 tasks", async () => {
    const value = await fixture();
    value.request.plan.tasks = Array.from({ length: 500 }, (_, index) => ({
      id: `AT-${String(index + 1).padStart(3, "0")}`,
      title: `Implement approved task ${index + 1}`,
      status: "ready",
      dependencies: [],
      acceptance: ["Pass the task checks."],
    }));
    const result = await initialize(value.root, value.request);
    assert.equal(result.status, "applied");
    assert.equal(result.taskIds.length, 500);
    const project = await resolveProject(value.root);
    const canonical = await loadCanonicalState(project);
    const { initializationRecordProblem } = await import(modulePath);
    assert.equal(canonical.tasks.length, 500);
    assert.equal(initializationRecordProblem(project.setup, canonical, { projectRoot: project.root }), undefined);
  });

  test("standalone initialization rejects 501 tasks without publishing setup", async () => {
    const value = await fixture();
    value.request.plan.tasks = Array.from({ length: 501 }, (_, index) => ({
      id: `AT-${String(index + 1).padStart(3, "0")}`,
      title: `Implement approved task ${index + 1}`,
      status: "ready",
      dependencies: [],
      acceptance: ["Pass the task checks."],
    }));
    const result = await initialize(value.root, value.request);
    assert.equal(result.status, "conflict");
    assert.equal(result.reason, "invalid_task_identity");
    await assert.rejects(access(path.join(value.root, ".agent-team/setup.json")), { code: "ENOENT" });
  });

  test("larger task capacity still rejects duplicate task IDs", async () => {
    const value = await fixture();
    value.request.plan.tasks = [
      value.request.plan.tasks[0],
      { ...value.request.plan.tasks[0], title: "Duplicate task" },
    ];
    const result = await initialize(value.root, value.request);
    assert.equal(result.status, "conflict");
    assert.equal(result.reason, "invalid_task_identity");
  });

  test("initialization is idempotent and changed operation semantics or another owner cannot replace records", async () => {
    const value = await fixture();
    await initialize(value.root, value.request);
    const setup = path.join(value.root, ".agent-team/setup.json");
    const before = await readFile(setup, "utf8");
    assert.equal((await initialize(value.feature, value.request)).status, "duplicate");
    assert.equal((await initialize(value.root, { ...value.request, plan: { ...value.request.plan, scope: "Different scope." } })).status, "conflict");
    assert.equal((await initialize(value.root, { ...value.request, ownerSessionId: "other-owner", operationId: "other-init" })).status, "conflict");
    assert.equal(await readFile(setup, "utf8"), before);
  });

  test("two actual initializer processes cannot establish competing canonical owners", async () => {
    const value = await fixture();
    const module = await import(modulePath).catch(() => ({}));
    assert.equal(typeof module.initializeProject, "function");
    const results = await Promise.all([worker(value.root, value.request), worker(value.feature, { ...value.request, ownerSessionId: "other-owner", operationId: "other-init" })]);
    assert.deepEqual(results.map(({ status }) => status).sort(), ["applied", "conflict"]);
    assert.ok(["owner-session", "other-owner"].includes((await loadCanonicalState(await resolveProject(value.root))).registry.projectOwner));
  });

  test("existing plan adoption preserves root tracker IDs, branch and unrelated records without creating AT-001", async () => {
    const value = await fixture();
    const source = "# Existing approved plan\n| ID | Requirement / acceptance | Owner | Depends on | Status | Revision / evidence | Next action |\n| --- | --- | --- | --- | --- | --- | --- |\n| WK-77 | Existing scope | none | none | ready | none | Implement. |\n";
    await writeFile(path.join(value.root, "TASKS.md"), source);
    await mkdir(path.join(value.root, ".agent-team"));
    await writeFile(path.join(value.root, ".agent-team/user-notes.md"), "Keep these notes.\n");
    const request = { ...value.request, source: "existing", tracker: { kind: "markdown", path: "TASKS.md" }, plan: { ...value.request.plan, tasks: [{ id: "WK-77" }] } };
    const result = await initialize(value.feature, request);
    assert.equal(result.status, "applied");
    assert.deepEqual(result.taskIds, ["WK-77"]);
    assert.equal(await readFile(path.join(value.root, "TASKS.md"), "utf8"), source);
    await assert.rejects(access(path.join(value.root, ".agent-team/TASKS.md")), { code: "ENOENT" });
    assert.equal(await readFile(path.join(value.root, ".agent-team/user-notes.md"), "utf8"), "Keep these notes.\n");
  });

  test("interrupted record publication leaves setup inactive and only identical operation can resume", async () => {
    const value = await fixture();
    const result = await initialize(value.root, value.request, { filesystem: { link: async (from, to) => {
      if (to.endsWith("state.json")) throw new Error("interrupted record publication");
      return link(from, to);
    } } });
    assert.equal(result.status, "unavailable");
    assert.equal(result.ready, false);
    assert.equal((await resolveProject(value.root)).active, false);
    assert.equal((await initialize(value.root, { ...value.request, operationId: "different-operation" })).status, "conflict");
    const recovered = await initialize(value.feature, value.request);
    assert.equal(recovered.status, "applied");
    assert.equal(recovered.ready, true);
    assert.equal((await loadCanonicalState(await resolveProject(value.root))).tasks.length, 1);
  });

  test("existing tracker requires explicit adoption and unavailable selected Beads never falls back", async () => {
    const value = await fixture();
    await writeFile(path.join(value.root, "TASKS.md"), "Existing project work.\n");
    assert.equal((await initialize(value.root, value.request)).reason, "existing_tracker_requires_adoption");
    const selected = await initialize(value.root, { ...value.request, source: "existing", tracker: { kind: "beads", executable: "/selected/bin/bd" } }, { runBeads: async () => { throw new Error("backend unavailable"); } });
    assert.equal(selected.status, "unavailable");
    assert.equal(selected.ready, false);
    await assert.rejects(access(path.join(value.root, ".agent-team/TASKS.md")), { code: "ENOENT" });
  });

  test("missing plan facts, unsafe paths and active setup without valid owner remain unready", async () => {
    const value = await fixture();
    assert.equal((await initialize(value.root, { ...value.request, plan: { ...value.request.plan, verification: [] } })).ready, false);
    assert.equal((await initialize(value.root, { ...value.request, plan: { ...value.request.plan, authority: { ownedPaths: ["../outside/**"] } } })).ready, false);
    await mkdir(path.join(value.root, ".agent-team"), { recursive: true });
    await writeFile(path.join(value.root, ".agent-team/setup.json"), JSON.stringify({ skill: "agent-team", projectId: "project-init" }));
    const before = await readFile(path.join(value.root, ".agent-team/setup.json"), "utf8");
    const result = await initialize(value.root, { ...value.request, source: "existing", expectedVersion: 0 });
    assert.equal(result.status, "conflict");
    assert.equal(result.reason, "existing_owner_unavailable");
    assert.equal(await readFile(path.join(value.root, ".agent-team/setup.json"), "utf8"), before);
  });

  test("adoption preserves registered owner's existing approved plan and unrelated state fields", async () => {
    const value = await fixture();
    await initialize(value.root, value.request);
    const project = await resolveProject(value.root);
    const setup = JSON.parse(await readFile(project.paths.setup, "utf8"));
    delete setup.initialization;
    setup.unrelated = { preserve: true };
    await writeFile(project.paths.setup, JSON.stringify(setup));
    const state = JSON.parse(await readFile(project.paths.state, "utf8"));
    state.unrelated = { preserve: "exact" };
    await writeFile(project.paths.state, JSON.stringify(state));
    const originalState = await readFile(project.paths.state, "utf8");
    const request = { ...value.request, source: "existing", expectedVersion: setup.version, plan: { ...value.request.plan, scope: "Unrelated replacement scope." } };
    assert.equal((await initialize(value.feature, request)).reason, "existing_plan_conflict");
    request.plan = value.request.plan;
    assert.equal((await initialize(value.feature, request)).status, "applied");
    assert.deepEqual(JSON.parse(await readFile(project.paths.setup, "utf8")).unrelated, { preserve: true });
    assert.equal(await readFile(project.paths.state, "utf8"), originalState);
  });

  test("duplicate initialization cannot make a missing required state record ready", async () => {
    const value = await fixture();
    await initialize(value.root, value.request);
    const project = await resolveProject(value.root);
    await writeFile(project.paths.state, JSON.stringify({ schemaVersion: 1, operationMappings: { providers: {}, shell: [] } }));
    const result = await initialize(value.root, value.request);
    assert.equal(result.ready, false);
    assert.equal(result.reason, "required_state_facts_missing");
  });

  test("adoption rejects each malformed authority fact without publishing setup", async (t) => {
    const mutations = {
      stateVersion: (state) => { state.stateVersion = -1; },
      runMode: (state) => { state.run.mode = "anything"; },
      runIdentity: (state) => { state.run.taskIds = ["missing-task"]; },
      duplicateIdentity: (state) => { state.run.taskIds.push(state.run.taskIds[0]); },
      integrationOwner: (state) => { delete state.integration.ownerSessionId; },
      integrationHold: (state) => { delete state.integration.hold; },
      integrationPause: (state) => { delete state.integration.paused; },
      baseBranch: (state) => { state.integration.baseRef = "different-branch"; },
      releaseOwner: (state) => { delete state.release.ownerSessionId; },
      releaseHold: (state) => { delete state.release.hold; },
      completionChecks: (state) => { state.completion.checks = "passed"; },
      malformedCheck: (state) => { state.completion.checks = [null]; },
    };
    for (const [name, mutate] of Object.entries(mutations)) await t.test(name, async () => {
      const value = await fixture();
      await initialize(value.root, value.request);
      const project = await resolveProject(value.root);
      const state = JSON.parse(await readFile(project.paths.state, "utf8"));
      mutate(state);
      const source = JSON.stringify(state);
      await writeFile(project.paths.state, source);
      await rm(project.paths.setup);
      const result = await initialize(value.root, { ...value.request, source: "existing" });
      assert.equal(result.ready, false, name);
      assert.equal(result.reason, "required_state_facts_missing", name);
      await assert.rejects(access(project.paths.setup), { code: "ENOENT" });
      assert.equal(await readFile(project.paths.state, "utf8"), source);
    });
  });

  test("existing-plan adoption cannot silently omit canonical tracker task IDs", async () => {
    const value = await fixture();
    value.request.plan.tasks.push({ id: "AT-002", title: "Preserve remaining work", status: "ready" });
    await initialize(value.root, value.request);
    const project = await resolveProject(value.root);
    const source = await readFile(project.paths.tasks, "utf8");
    await rm(project.paths.setup);
    const result = await initialize(value.root, { ...value.request, source: "existing", plan: { ...value.request.plan, tasks: [{ id: "AT-001" }] } });
    assert.equal(result.status, "conflict");
    assert.equal(result.reason, "existing_task_identity_conflict");
    await assert.rejects(access(project.paths.setup), { code: "ENOENT" });
    assert.equal(await readFile(project.paths.tasks, "utf8"), source);
  });

  test("duplicate initialization rejects incomplete or inconsistent setup receipts", async (t) => {
    for (const field of ["status", "planIds", "version", "initialRunIds", "emptyStandalone"]) await t.test(field, async () => {
      const value = await fixture();
      value.request.plan.tasks.push({ id: "AT-002", title: "Remaining work", status: "ready" });
      await initialize(value.root, value.request);
      const project = await resolveProject(value.root);
      const setup = JSON.parse(await readFile(project.paths.setup, "utf8"));
      if (field === "status") setup.initialization.status = "pending";
      if (field === "planIds") setup.plan.taskIds = ["AT-001"];
      if (field === "version") setup.version = -1;
      if (field === "initialRunIds") {
        const state = JSON.parse(await readFile(project.paths.state, "utf8"));
        state.run.taskIds = ["AT-001"];
        await writeFile(project.paths.state, JSON.stringify(state));
      }
      if (field === "emptyStandalone") {
        setup.plan.taskIds = [];
        const state = JSON.parse(await readFile(project.paths.state, "utf8"));
        state.run.taskIds = [];
        await writeFile(project.paths.state, JSON.stringify(state));
        await writeFile(project.paths.tasks, (await readFile(project.paths.tasks, "utf8")).split("\n").filter((line) => !/^\| AT-00/.test(line)).join("\n"));
      }
      await writeFile(project.paths.setup, JSON.stringify(setup));
      const before = await readFile(project.paths.setup, "utf8");
      const result = await initialize(value.root, value.request);
      assert.equal(result.ready, false, JSON.stringify(result));
      assert.notEqual(result.status, "duplicate");
      assert.equal(await readFile(project.paths.setup, "utf8"), before);
    });
  });

  test("outage-tolerant receipt validation still rejects invalid or mismatched tracker selectors", async () => {
    const value = await fixture();
    await initialize(value.root, value.request);
    const project = await resolveProject(value.root);
    const canonical = await loadCanonicalState(project);
    const { initializationRecordProblem } = await import(modulePath);
    for (const tracker of [{ kind: "markdown", path: "elsewhere.md" }, { kind: "beads", executable: "relative/bd" }, { kind: "markdown", path: "TASKS.md" }]) {
      assert.equal(initializationRecordProblem({ ...project.setup, tracker }, canonical, { projectRoot: project.root }), "invalid_tracker_selection");
    }
    assert.equal(initializationRecordProblem(project.setup, { ...canonical, tracker: undefined }, { projectRoot: project.root }), "invalid_tracker_selection");
    assert.equal(initializationRecordProblem(project.setup, { ...canonical, tracker: { ...canonical.tracker, status: "unavailable" } }, { projectRoot: project.root }), undefined);
  });

  test("initialization receipt binds the tracker before outage-tolerant settings validation", async () => {
    const value = await fixture();
    await initialize(value.root, value.request);
    const project = await resolveProject(value.root);
    const { initializationRecordProblem } = await import(modulePath);
    const setup = { ...project.setup, tracker: { kind: "beads", executable: "/selected/unavailable-bd" } };
    await writeFile(project.paths.setup, JSON.stringify(setup));
    const fresh = await resolveProject(value.root);
    const canonical = await loadCanonicalState(fresh, { includeTasks: false });
    assert.equal(initializationRecordProblem(fresh.setup, canonical, { projectRoot: fresh.root }), "initialization_tracker_changed");
    assert.equal((await initialize(value.root, { ...value.request, tracker: setup.tracker })).status, "conflict");
  });

  test("duplicate initialization preserves a later finite run scope without losing the full approved plan", async () => {
    const value = await fixture();
    value.request.plan.tasks.push({ id: "AT-002", title: "Remaining work", status: "ready" });
    await initialize(value.root, value.request);
    const project = await resolveProject(value.root);
    const state = JSON.parse(await readFile(project.paths.state, "utf8"));
    state.stateVersion = 1;
    state.run.taskIds = ["AT-002"];
    await writeFile(project.paths.state, JSON.stringify(state));
    const result = await initialize(value.root, value.request);
    assert.equal(result.status, "duplicate");
    assert.deepEqual(result.taskIds, ["AT-001", "AT-002"]);
    assert.deepEqual(result.eligibleTaskIds, ["AT-002"]);
    assert.deepEqual(JSON.parse(await readFile(project.paths.state, "utf8")).run.taskIds, ["AT-002"]);
  });

  test("validated owner and tracker snapshots cannot change before setup publication", async (t) => {
    for (const change of ["owner", "tracker"]) await t.test(change, async () => {
      const value = await fixture();
      await initialize(value.root, value.request);
      const project = await resolveProject(value.root);
      await rm(project.paths.setup);
      await mkdir(path.join(value.root, ".beads"));
      const before = await readFile(project.paths.teams, "utf8");
      let reads = 0;
      const result = await initialize(value.root, { ...value.request, source: "existing", tracker: { kind: "beads", executable: "/selected/bin/bd" } }, {
        runBeads: async () => {
          reads += 1;
          if (change === "owner" && reads === 1) await writeFile(project.paths.teams, before.replace("Project owner: owner-session", "Project owner: other-owner"));
          return { stdout: JSON.stringify([{ id: "AT-001", title: change === "tracker" && reads > 1 ? "Changed after validation" : "Original task", status: "open", assignee: "", dependency_count: 0 }]) };
        },
      });
      assert.equal(result.status, "conflict", JSON.stringify(result));
      assert.equal(result.ready, false);
      await assert.rejects(access(project.paths.setup), { code: "ENOENT" });
      if (change === "owner") assert.match(await readFile(project.paths.teams, "utf8"), /Project owner: other-owner/);
    });
  });

  test("actual killed initializer is resumable only after verifying its stopped lock writer", async () => {
    const value = await fixture();
    const child = spawn(process.execPath, [fileURLToPath(import.meta.url), "crash-worker", value.root, JSON.stringify(value.request)], { stdio: ["ignore", "pipe", "pipe"] });
    try {
      await new Promise((resolve, reject) => { child.stdout.once("data", resolve); child.once("error", reject); });
      const ended = new Promise((resolve) => child.once("exit", resolve)); child.kill("SIGKILL"); await ended;
      assert.equal((await resolveProject(value.root)).active, false);
      const recovered = await initialize(value.feature, value.request);
      assert.equal(recovered.status, "applied");
      assert.equal(recovered.ready, true);
    } finally { if (child.exitCode === null && child.signalCode === null) child.kill("SIGKILL"); }
  });

  test("selected existing Beads adoption uses its exact executable and original IDs without Markdown", async () => {
    const value = await fixture();
    await mkdir(path.join(value.root, ".beads"));
    const request = { ...value.request, source: "existing", tracker: { kind: "beads", executable: "/selected/bin/bd" }, plan: { ...value.request.plan, tasks: [{ id: "native-X7" }] } };
    let reads = 0;
    const result = await initialize(value.feature, request, { runBeads: async (binary, args, options) => {
      assert.equal(binary, "/selected/bin/bd");
      assert.equal(args[0], "list");
      assert.equal(options.env.BEADS_DIR, path.join(value.root, ".beads"));
      reads += 1;
      return { stdout: JSON.stringify([{ id: "native-X7", title: "Existing native scope", status: "open", assignee: "", dependency_count: 0 }]) };
    } });
    assert.equal(result.status, "applied");
    assert.equal(result.ready, true);
    assert.deepEqual(result.taskIds, ["native-X7"]);
    assert.ok(reads >= 2);
    await assert.rejects(access(path.join(value.root, ".agent-team/TASKS.md")), { code: "ENOENT" });
    await assert.rejects(access(path.join(value.root, "TASKS.md")), { code: "ENOENT" });
  });

  test("tampered partially published task record is preserved and initialization remains inactive", async () => {
    const value = await fixture();
    await initialize(value.root, value.request, { filesystem: { link: async (from, to) => {
      if (to.endsWith("state.json")) throw new Error("publication interruption");
      return link(from, to);
    } } });
    const tracker = path.join(value.root, ".agent-team/TASKS.md");
    await writeFile(tracker, "User changed this record after interruption.\n");
    const result = await initialize(value.root, value.request);
    assert.equal(result.status, "conflict");
    assert.equal(await readFile(tracker, "utf8"), "User changed this record after interruption.\n");
    assert.equal((await resolveProject(value.root)).active, false);
  });
}
