import assert from "node:assert/strict";
import { execFileSync } from "node:child_process";
import { chmod, mkdir, mkdtemp, readFile, rm, symlink, writeFile } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import test from "node:test";

import { classifyOperation } from "../hooks/lib/operation.mjs";
import { loadCanonicalState } from "../hooks/lib/canonical-state.mjs";
import { resolveProject } from "../hooks/lib/project.mjs";
import { evaluatePolicy } from "../hooks/lib/policy.mjs";
import { hookEvent, policyFixture, saveState } from "./hook-test-helpers.mjs";

const temporary = [];
test.afterEach(async () => Promise.all(temporary.splice(0).map((item) => rm(item, { force: true, recursive: true }))));

async function fixture() {
  const root = await mkdtemp(path.join(os.tmpdir(), "agent-team-policy-"));
  temporary.push(root, `${root}-feature`);
  const value = await policyFixture(root);
  temporary.push(value.remote);
  const state = structuredClone(value.state);
  state.integration.taskIds = ["AT-001"];
  state.integration.authorization = { source: "fixture", scope: "integration", ownerSessionId: "owner-session", revision: value.revision, taskIds: ["AT-001"], observedAt: "2026-09-06T12:00:00.000Z" };
  await saveState(value, state);
  return { ...value, state, project: await resolveProject(value.feature) };
}

test("canonical ownership allows assigned files and blocks unowned, shared, main, and symlink targets", async () => {
  // This test catches trust in caller role text or a path-prefix check before symlink resolution.
  const value = await fixture();
  const allowed = await evaluatePolicy(hookEvent(value, {
    operation: { kind: "file_change", files: [{ action: "edit", path: "src/owned.js", changedContent: "export const x = 1;" }] },
  }), value.project);
  const unowned = await evaluatePolicy({ ...hookEvent(value, {
    operation: { kind: "file_change", files: [{ action: "edit", path: "README.md", changedContent: "changed" }] },
  }), callerRole: "projectOwner" }, value.project);
  const shared = await evaluatePolicy(hookEvent(value, {
    operation: { kind: "file_change", files: [{ action: "edit", path: ".agent-team/TASKS.md", changedContent: "changed" }] },
  }), value.project);
  const mainProject = await resolveProject(value.root);
  const main = await evaluatePolicy(hookEvent(value, {
    cwd: value.root,
    operation: { kind: "file_change", files: [{ action: "edit", path: "README.md", changedContent: "changed" }] },
  }), mainProject);
  await symlink(path.join(value.root, "README.md"), path.join(value.feature, "src", "escape.md"));
  const escaped = await evaluatePolicy(hookEvent(value, {
    operation: { kind: "file_change", files: [{ action: "edit", path: "src/escape.md", changedContent: "changed" }] },
  }), value.project);

  assert.equal(allowed.allow, true);
  for (const result of [unowned, shared, main, escaped]) {
    assert.equal(result.allow, false);
    assert.equal(result.mode, "enforce");
  }
});

test("relative registered worktrees resolve from the canonical project without relaxing ownership", async () => {
  const value = await fixture();
  const teams = await readFile(value.project.paths.teams, "utf8");
  const relative = path.relative(value.root, value.feature);
  await writeFile(value.project.paths.teams, teams.replace(value.feature, relative));
  const operation = { kind: "file_change", files: [{ action: "edit", path: "src/owned.js", changedContent: "export {};" }] };
  assert.equal((await evaluatePolicy(hookEvent(value, { operation }), value.project)).allow, true);
  const main = await resolveProject(value.root);
  assert.equal((await evaluatePolicy(hookEvent(value, { cwd: value.root, operation }), main)).allow, false);
  assert.equal((await evaluatePolicy(hookEvent(value, { operation: { kind: "file_change", files: [{ action: "edit", path: "README.md" }] } }), value.project)).allow, false);
  await writeFile(value.project.paths.teams, teams.replace(value.feature, ""));
  assert.equal((await evaluatePolicy(hookEvent(value, { operation }), value.project)).allow, false);
});

test("one unowned file blocks a multi-file change", async () => {
  // This test catches enforcement that checks only the first file in a patch.
  const value = await fixture();
  const decision = await evaluatePolicy(hookEvent(value, {
    operation: { kind: "file_change", files: [
      { action: "edit", path: "src/owned.js", changedContent: "ok" },
      { action: "delete", path: "README.md", changedContent: "" },
    ] },
  }), value.project);

  assert.equal(decision.allow, false);
  assert.match(decision.messages.join("\n"), /README\.md/);
});

test("moving a file checks both its source and destination ownership", async () => {
  // This test catches a move that hides an unowned source behind an owned destination.
  const value = await fixture();
  const decision = await evaluatePolicy(hookEvent(value, {
    operation: { kind: "file_change", files: [{
      action: "move",
      previousPath: "README.md",
      path: "src/README.md",
      changedContent: "fixture",
    }] },
  }), value.project);

  assert.equal(decision.allow, false);
});

test("shell mv retains every source path and rejects ambiguous operand forms", async () => {
  // This test catches a shell move that checks only its destination path.
  const single = classifyOperation({ operation: { kind: "shell", command: "mv README.md src/README.md" } });
  const multiple = classifyOperation({ operation: { kind: "shell", command: "mv src/a.js README.md src/archive/" } });
  const separator = classifyOperation({ operation: { kind: "shell", command: "mv -- src/a.js src/b.js" } });
  const ambiguous = classifyOperation({ operation: { kind: "shell", command: "mv --target-directory=src README.md" } });

  assert.deepEqual(single.files, [{ action: "move", previousPath: "README.md", path: "src/README.md", changedContent: "" }]);
  assert.deepEqual(multiple.files.map(({ previousPath }) => previousPath), ["src/a.js", "README.md"]);
  assert.equal(separator.files[0].previousPath, "src/a.js");
  assert.equal(ambiguous.parserFailed, true);

  const value = await fixture();
  const unownedSource = await evaluatePolicy(hookEvent(value, { operation: { kind: "shell", command: "mv README.md src/README.md" } }), value.project);
  const unownedDestination = await evaluatePolicy(hookEvent(value, { operation: { kind: "shell", command: "mv src/owned.js README.md" } }), value.project);
  const allowed = await evaluatePolicy(hookEvent(value, { operation: { kind: "shell", command: "mv -- src/owned.js src/moved.js" } }), value.project);
  assert.equal(unownedSource.allow, false);
  assert.equal(unownedDestination.allow, false);
  assert.equal(allowed.allow, true);
});

test("adding a file under new owned directories resolves against the nearest existing parent", async () => {
  // This test catches ownership checks that require every new parent directory to exist.
  const value = await fixture();
  const decision = await evaluatePolicy(hookEvent(value, {
    operation: { kind: "file_change", files: [{
      action: "add",
      path: "src/new/nested/file.js",
      changedContent: "export {};",
    }] },
  }), value.project);

  assert.equal(decision.allow, true);
});

test("recognized shell and provider file operations use the same ownership gate", async () => {
  // This test catches file ownership enforcement that covers only Edit and apply_patch.
  const value = await fixture();
  const shellAllowed = await evaluatePolicy(hookEvent(value, {
    operation: { kind: "shell", command: "rm src/owned.js" },
  }), value.project);
  const shellDenied = await evaluatePolicy(hookEvent(value, {
    operation: { kind: "shell", command: "rm README.md" },
  }), value.project);
  const providerDenied = await evaluatePolicy(hookEvent(value, {
    operation: { kind: "provider", tool: "mcp__filesystem__write", input: { path: "README.md", content: "changed" } },
  }), value.project);
  const malformedProvider = await evaluatePolicy(hookEvent(value, {
    operation: { kind: "provider", tool: "mcp__filesystem__write", input: { content: "changed" } },
  }), value.project);

  assert.equal(shellAllowed.allow, true);
  assert.equal(shellDenied.allow, false);
  assert.equal(providerDenied.allow, false);
  assert.equal(malformedProvider.allow, false);
});

test("authorized integration and release operations pass without an approval prompt", async () => {
  // This test catches a gate that asks again after scoped authorization is recorded.
  const value = await fixture();
  const push = await evaluatePolicy(hookEvent(value, {
    sessionId: "owner-session",
    operation: { kind: "shell", command: `git -C ${value.feature} push origin HEAD:feature` },
  }), value.project, { now: new Date("2026-09-06T12:01:00.000Z") });
  const publish = await evaluatePolicy(hookEvent(value, {
    sessionId: "owner-session",
    operation: { kind: "shell", command: "npm publish" },
  }), value.project, { now: new Date("2026-09-06T12:01:00.000Z") });

  assert.equal(push.allow, true);
  assert.equal(publish.allow, true);
  assert.equal(JSON.stringify([push, publish]).includes('"ask"'), false);
});

test("recognized critical operations fail closed when runtime evidence is unavailable", async () => {
  // This test catches a runtime probe error escaping to the non-blocking hook fallback.
  const value = await fixture();
  const unavailableProject = { ...value.project, worktreeRoot: path.join(value.root, "missing-worktree") };
  const release = await evaluatePolicy(hookEvent(value, {
    sessionId: "owner-session",
    operation: { kind: "shell", command: "npm publish" },
  }), unavailableProject, { now: new Date("2026-09-06T12:01:00.000Z") });

  assert.equal(release.allow, false);
  assert.equal(release.mode, "enforce");
});

test("integration blocks wrong owner, revision, base, remote, stale evidence, gates, delta, recovery, and deployment triggers", async (context) => {
  // This table catches each deterministic prerequisite being accidentally skipped.
  const cases = [
    ["wrong owner", {}, { sessionId: "developer-session" }],
    ["wrong revision", { expectedRevision: "deadbeef" }],
    ["wrong base", { baseRevision: "deadbeef" }],
    ["wrong remote", { remoteRevision: "deadbeef" }],
    ["stale evidence", { evidenceAt: "2026-09-06T11:00:00.000Z" }],
    ["preview", { preview: { required: true, approvedRevision: "deadbeef" } }],
    ["pause", { paused: true }],
    ["hold", { hold: true }],
    ["delta", { deltaClean: false }],
    ["recovery", { recoveryReconciled: false }],
    ["deployment trigger", { updatesRemoteMain: true, remoteMainDeploys: true }],
  ];

  for (const [name, override, eventOverride = {}] of cases) {
    await context.test(name, async () => {
      const value = await fixture();
      const state = structuredClone(value.state);
      Object.assign(state.integration, override);
      if (name === "deployment trigger") state.release.autoDeploy = false;
      await saveState(value, state);
      const decision = await evaluatePolicy(hookEvent(value, {
        sessionId: "owner-session",
        operation: { kind: "shell", command: `git -C ${value.feature} push origin HEAD:feature` },
        ...eventOverride,
      }), value.project, { now: new Date("2026-09-06T12:01:00.000Z") });
      assert.equal(decision.allow, false);
      assert.ok(decision.messages.length > 0);
    });
  }
});

test("integration compares bounded current remote and base refs instead of stale local tracking refs", async () => {
  // This test catches remote validation that only resolves local refs.
  const value = await fixture();
  const tree = execFileSync("git", ["rev-parse", "HEAD^{tree}"], { cwd: value.feature, encoding: "utf8" }).trim();
  const remoteRevision = execFileSync("git", ["commit-tree", tree, "-p", "HEAD", "-m", "remote drift"], { cwd: value.feature, encoding: "utf8" }).trim();
  execFileSync("git", ["push", "-q", "origin", `${remoteRevision}:refs/heads/feature`], { cwd: value.feature });
  const drifted = await evaluatePolicy(hookEvent(value, {
    sessionId: "owner-session",
    operation: { kind: "shell", command: `git -C ${value.feature} push origin HEAD:feature` },
  }), value.project, { now: new Date("2026-09-06T12:01:00.000Z") });

  await rm(value.remote, { recursive: true, force: true });
  const unavailable = await evaluatePolicy(hookEvent(value, {
    sessionId: "owner-session",
    operation: { kind: "shell", command: `git -C ${value.feature} push origin HEAD:feature` },
  }), value.project, { now: new Date("2026-09-06T12:01:00.000Z") });

  assert.equal(drifted.allow, false);
  assert.match(drifted.messages.join("\n"), /remote/i);
  assert.equal(unavailable.allow, false);
  assert.match(unavailable.messages.join("\n"), /unavailable/i);
});

test("deployment-triggering main integration consumes only the current release batch authority", async (context) => {
  const invalid = [
    ["release revision", (state) => { state.release.expectedRevision = "deadbeef"; }],
    ["release tracker", (state) => { state.release.trackerFingerprint = "deadbeef"; }],
    ["release freshness", (state) => { state.release.evidenceAt = "2026-09-06T11:00:00.000Z"; }],
    ["release authorization scope", (state) => { state.release.authorization.scope = "another-batch"; }],
    ["release authorization target", (state) => { state.release.authorization.target = "another-target"; }],
    ["release deployment disposition", (state) => { state.release.remoteMainDeploys = false; }],
  ];
  for (const [name, invalidate] of invalid) await context.test(name, async () => {
    const value = await fixture();
    const canonical = await loadCanonicalState(value.project);
    const state = structuredClone(canonical.state);
    Object.assign(state.integration, {
      remoteRef: "refs/heads/main", remoteRevision: value.revision, updatesRemoteMain: true, remoteMainDeploys: true,
    });
    Object.assign(state.release, {
      trackerFingerprint: canonical.tracker.fingerprint, remoteMainDeploys: true,
    });
    state.release.integration.remoteMainDeploys = true;
    invalidate(state);
    await saveState(value, state);
    const result = await evaluatePolicy(hookEvent(value, {
      sessionId: "owner-session",
      operation: { kind: "shell", command: `git -C ${value.feature} push origin HEAD:main` },
    }), value.project, { now: new Date("2026-09-06T12:01:00.000Z") });
    assert.equal(result.allow, false, name);
  });
});

test("integration permits only an evidenced non-force remote-main advance", async () => {
  const value = await fixture();
  await writeFile(path.join(value.root, "advance.txt"), "advance\n");
  execFileSync("git", ["add", "advance.txt"], { cwd: value.root });
  execFileSync("git", ["commit", "-q", "-m", "advance main"], { cwd: value.root });
  const head = execFileSync("git", ["rev-parse", "HEAD"], { cwd: value.root, encoding: "utf8" }).trim();
  const remoteBase = execFileSync("git", ["ls-remote", "--exit-code", "origin", "refs/heads/main"], { cwd: value.root, encoding: "utf8" }).trim().split(/\s+/)[0];
  const state = structuredClone((await loadCanonicalState(await resolveProject(value.root))).state);
  Object.assign(state.integration, {
    expectedRevision: head, baseRevision: remoteBase, remoteRevision: remoteBase, remoteRef: "refs/heads/main",
    authorization: { source: "accepted-packet", scope: "integration", ownerSessionId: "owner-session", revision: head, taskIds: ["AT-001"], observedAt: "2026-09-06T12:00:00.000Z" },
  });
  await saveState(value, state);
  const project = await resolveProject(value.root);
  const operation = { kind: "shell", command: `git -C ${value.root} push origin HEAD:main` };
  const allowed = await evaluatePolicy(hookEvent(value, { cwd: value.root, sessionId: "owner-session", operation }), project, { now: new Date("2026-09-06T12:01:00.000Z") });
  assert.equal(allowed.allow, true);
  for (const [name, mutate] of [
    ["remote drift", (candidate) => { candidate.integration.remoteRevision = "deadbeef"; }],
    ["non ancestor", (candidate) => { candidate.integration.baseRevision = head; candidate.integration.expectedRevision = remoteBase; }],
    ["wrong head", (candidate) => { candidate.integration.expectedRevision = "deadbeef"; }],
    ["missing provenance", (candidate) => { delete candidate.integration.authorization; }],
  ]) {
    const candidate = structuredClone(state);
    mutate(candidate);
    await saveState(value, candidate);
    const denied = await evaluatePolicy(hookEvent(value, { cwd: value.root, sessionId: "owner-session", operation }), project, { now: new Date("2026-09-06T12:01:00.000Z") });
    assert.equal(denied.allow, false, name);
  }
  await saveState(value, state);
  const forced = await evaluatePolicy(hookEvent(value, { cwd: value.root, sessionId: "owner-session",
    operation: { kind: "shell", command: `git -C ${value.root} push --force origin HEAD:main` } }), project, { now: new Date("2026-09-06T12:01:00.000Z") });
  assert.equal(forced.allow, false, "force push");
  for (const command of [
    `git -C ${value.root} push origin +HEAD:main`,
    `git -C ${value.root} push --force-with-lease=refs/heads/main:${remoteBase} origin HEAD:main`,
    `git -C ${value.root} push backup HEAD:main`,
    `git -C ${value.root} push origin HEAD:main HEAD:other`,
    `git -C ${value.root} push origin HEAD:main ; git status`,
  ]) {
    const bypass = await evaluatePolicy(hookEvent(value, { cwd: value.root, sessionId: "owner-session", operation: { kind: "shell", command } }), project, { now: new Date("2026-09-06T12:01:00.000Z") });
    assert.equal(bypass.allow, false, command);
  }
  await writeFile(path.join(value.root, "advance.txt"), "dirty\n");
  const dirty = await evaluatePolicy(hookEvent(value, { cwd: value.root, sessionId: "owner-session", operation }), project, { now: new Date("2026-09-06T12:01:00.000Z") });
  assert.equal(dirty.allow, false, "dirty worktree");
});

test("PR integration retains its non-push evidence route", async () => {
  const value = await fixture();
  const result = await evaluatePolicy(hookEvent(value, { sessionId: "owner-session", operation: { kind: "shell", command: "gh pr merge 1 --merge" } }), value.project, { now: new Date("2026-09-06T12:01:00.000Z") });
  assert.equal(result.allow, true);
});

test("tag integration requires an absent exact remote target until creation", async () => {
  const value = await fixture();
  const state = structuredClone(value.state);
  state.integration.remoteRef = "refs/tags/v7.1.0";
  state.integration.targetAbsent = true;
  delete state.integration.remoteRevision;
  await saveState(value, state);
  const operation = { kind: "shell", command: `git -C ${value.feature} push origin HEAD:refs/tags/v7.1.0` };
  const allowed = await evaluatePolicy(hookEvent(value, { sessionId: "owner-session", operation }), value.project, { now: new Date("2026-09-06T12:01:00.000Z") });
  assert.equal(allowed.allow, true);
  for (const command of [`git -C ${value.feature} push backup HEAD:refs/tags/v7.1.0`, `git -C ${value.feature} push origin HEAD:refs/tags/v7.1.1`]) {
    const denied = await evaluatePolicy(hookEvent(value, { sessionId: "owner-session", operation: { kind: "shell", command } }), value.project, { now: new Date("2026-09-06T12:01:00.000Z") });
    assert.equal(denied.allow, false, command);
  }
  execFileSync("git", ["push", "-q", "origin", "HEAD:refs/tags/v7.1.0"], { cwd: value.feature });
  const drifted = await evaluatePolicy(hookEvent(value, { sessionId: "owner-session", operation }), value.project, { now: new Date("2026-09-06T12:01:00.000Z") });
  assert.equal(drifted.allow, false);
});

test("absent-tag integration rejects a substituted remote feature base after remote main diverges", async () => {
  const value = await fixture();
  const original = execFileSync("git", ["rev-parse", "HEAD"], { cwd: value.root, encoding: "utf8" }).trim();
  await writeFile(path.join(value.root, "local-main-advance.txt"), "local advance\n");
  execFileSync("git", ["add", "local-main-advance.txt"], { cwd: value.root });
  execFileSync("git", ["commit", "-q", "-m", "local main advance"], { cwd: value.root });
  const head = execFileSync("git", ["rev-parse", "HEAD"], { cwd: value.root, encoding: "utf8" }).trim();
  const tree = execFileSync("git", ["rev-parse", `${original}^{tree}`], { cwd: value.root, encoding: "utf8" }).trim();
  const divergentRemoteMain = execFileSync("git", ["commit-tree", tree, "-p", original, "-m", "remote main divergence"], { cwd: value.root, encoding: "utf8" }).trim();
  execFileSync("git", ["push", "-q", "origin", `${divergentRemoteMain}:refs/heads/main`], { cwd: value.root });

  const state = structuredClone(value.state);
  Object.assign(state.integration, {
    expectedRevision: head, baseRevision: original, baseRemoteRef: "refs/heads/feature",
    remoteRef: "refs/tags/v7.1.0", targetAbsent: true, remoteRevision: undefined,
    authorization: { source: "accepted-packet", scope: "integration", ownerSessionId: "owner-session", revision: head, taskIds: ["AT-001"], observedAt: "2026-09-06T12:00:00.000Z" },
  });
  await saveState(value, state);
  const project = await resolveProject(value.root);
  const decision = await evaluatePolicy(hookEvent(value, {
    cwd: value.root, sessionId: "owner-session",
    operation: { kind: "shell", command: `git -C ${value.root} push origin HEAD:refs/tags/v7.1.0` },
  }), project, { now: new Date("2026-09-06T12:01:00.000Z") });

  assert.equal(decision.allow, false, "a tampered canonical baseRemoteRef cannot substitute an unchanged feature for divergent remote main");
});

test("release gate binds authorization, run, batch, artifact, evidence, pause, delta, and recovery records", async (context) => {
  // This test catches a release decision based only on owner, target, and revision.
  const cases = [
    ["authorization provenance", (state) => { delete state.release.authorization.source; }],
    ["authorization owner", (state) => { state.release.authorization.ownerSessionId = "developer-session"; }],
    ["authorization revision", (state) => { state.release.authorization.revision = "deadbeef"; }],
    ["authorization task scope", (state) => { state.release.authorization.taskIds = ["AT-404"]; }],
    ["process scope", (state) => { state.release.authorization.process = "vercel"; }],
    ["run mode", (state) => { state.release.runMode = "auto_deploy"; state.release.autoDeploy = false; }],
    ["manual mode", (state) => { state.release.runMode = "manual"; state.release.autoDeploy = true; }],
    ["missing run record", (state) => { delete state.release.run; }],
    ["blank run identity", (state) => { state.release.run.id = ""; }],
    ["run record mode", (state) => { state.release.run.mode = "manual"; }],
    ["run record task scope", (state) => { state.release.run.taskIds = ["AT-404"]; }],
    ["paused run record", (state) => { state.release.run.paused = true; }],
    ["batch membership", (state) => { state.release.taskIds = ["AT-404"]; }],
    ["artifact revision", (state) => { state.release.artifact.revision = "deadbeef"; }],
    ["missing artifact SHA", (state) => { delete state.release.artifact.sha256; }],
    ["malformed artifact SHA", (state) => { state.release.artifact.sha256 = "invalid"; }],
    ["malformed checksum-file SHA", (state) => { state.release.artifact.checksumSha256 = "invalid"; }],
    ["integration record", (state) => { state.release.integration.status = "pending"; }],
    ["verification record", (state) => { state.release.verification.revision = "deadbeef"; }],
    ["preview record", (state) => { state.release.preview = { required: true, status: "pending", revision: state.release.expectedRevision }; }],
    ["project pause", (state) => { state.release.projectPaused = true; }],
    ["deployment delta", (state) => { state.release.delta.taskIds = []; }],
    ["remote main deployment binding", (state) => { state.release.remoteMainDeploys = true; }],
    ["known recovery", (state) => { delete state.release.recovery.action; }],
  ];

  for (const [name, mutate] of cases) {
    await context.test(name, async () => {
      const value = await fixture();
      const state = structuredClone(value.state);
      mutate(state);
      await saveState(value, state);
      const result = await evaluatePolicy(hookEvent(value, {
        sessionId: "owner-session",
        operation: { kind: "shell", command: "npm publish" },
      }), value.project, { now: new Date("2026-09-06T12:01:00.000Z") });
      assert.equal(result.allow, false);
    });
  }
});

test("recognized destructive database operations require authorization, inventory, recovery, dry run, and cascade evidence", async () => {
  // This test catches a destructive client command that proceeds on partial evidence.
  const value = await fixture();
  const operation = { kind: "shell", command: "psql --command 'DROP TABLE customers CASCADE'" };
  const allowed = await evaluatePolicy(hookEvent(value, { sessionId: "owner-session", operation }), value.project, {
    now: new Date("2026-09-06T12:01:00.000Z"),
  });
  assert.equal(allowed.allow, true);

  for (const change of [
    { authorized: false },
    { inventoryAt: undefined },
    { recovery: undefined },
    { dryRun: undefined },
    { allowCascade: false },
    { environment: "production", productionApproved: false },
  ]) {
    const state = structuredClone(value.state);
    Object.assign(state.database, change);
    await saveState(value, state);
    const denied = await evaluatePolicy(hookEvent(value, { sessionId: "owner-session", operation }), value.project, {
      now: new Date("2026-09-06T12:01:00.000Z"),
    });
    assert.equal(denied.allow, false);
    assert.equal(denied.messages.join("\n").includes("customers"), false);
  }
});

test("database gates accept an explicit unsupported dry-run capability only with fresh alternative evidence", async () => {
  // This test catches a database gate that treats every operation as transaction-dry-run capable.
  const value = await fixture();
  const state = structuredClone(value.state);
  state.database.dryRun = {
    capability: "unsupported",
    reason: "The provider has no transaction preview.",
    alternative: { kind: "disposable-clone", verifiedAt: "2026-09-06T12:00:00.000Z" },
  };
  await saveState(value, state);
  const allowed = await evaluatePolicy(hookEvent(value, {
    sessionId: "owner-session",
    operation: { kind: "shell", command: "psql -c 'DROP TABLE fixture CASCADE'" },
  }), value.project, { now: new Date("2026-09-06T12:01:00.000Z") });
  delete state.database.dryRun.alternative;
  await saveState(value, state);
  const blocked = await evaluatePolicy(hookEvent(value, {
    sessionId: "owner-session",
    operation: { kind: "shell", command: "psql -c 'DROP TABLE fixture CASCADE'" },
  }), value.project, { now: new Date("2026-09-06T12:01:00.000Z") });

  assert.equal(allowed.allow, true);
  assert.equal(blocked.allow, false);
});

test("explicit provider and app-script mappings gate destructive database paths and label blind spots", async () => {
  // This test catches broad provider-name guessing and silent treatment of unsupported scripts.
  const value = await fixture();
  const provider = await evaluatePolicy(hookEvent(value, {
    sessionId: "owner-session",
    operation: { kind: "provider", tool: "mcp__database__execute", input: { query: "TRUNCATE TABLE private_data" } },
  }), value.project, { now: new Date("2026-09-06T12:01:00.000Z") });
  const script = await evaluatePolicy(hookEvent(value, {
    sessionId: "owner-session",
    operation: { kind: "shell", command: "node scripts/reset-data.mjs --target staging" },
  }), value.project, { now: new Date("2026-09-06T12:01:00.000Z") });
  const blindSpot = await evaluatePolicy(hookEvent(value, {
    operation: { kind: "shell", command: "node scripts/unknown.mjs --drop-data" },
  }), value.project, { now: new Date("2026-09-06T12:01:00.000Z") });

  assert.equal(provider.allow, true);
  assert.equal(script.allow, true);
  assert.equal(blindSpot.allow, true);
  assert.equal(blindSpot.capabilities.database, "unsupported_path");
  assert.equal(JSON.stringify(provider).includes("private_data"), false);
});

test("explicit completion checks evidence while Stop and interruption events do not", async () => {
  // This test catches the old broad Stop gate and missing TaskCompleted evidence checks.
  const value = await fixture();
  const complete = await evaluatePolicy(hookEvent(value, {
    event: "TaskCompleted",
    operation: { kind: "completion", taskId: "AT-001" },
  }), value.project);
  const state = structuredClone(value.state);
  state.completion.checks = [];
  await saveState(value, state);
  const blocked = await evaluatePolicy(hookEvent(value, {
    event: "TaskCompleted",
    operation: { kind: "completion", taskId: "AT-001" },
  }), value.project);

  for (const event of ["Stop", "Interrupt", "UserPromptSubmit"]) {
    const ordinary = await evaluatePolicy(hookEvent(value, { event, operation: { kind: "lifecycle" } }), value.project);
    assert.equal(ordinary.allow, true);
  }
  assert.equal(complete.allow, true);
  assert.equal(blocked.allow, false);
});

test("completion checks deployment and cleanup only when scoped and permits honest non-final outcomes", async () => {
  // This test catches final-only evidence being applied to blocked or out-of-scope completion states.
  const value = await fixture();
  const baseline = structuredClone(value.state);
  baseline.completion.scope = { deployment: true, cleanup: true };
  await saveState(value, baseline);
  const missingScoped = await evaluatePolicy(hookEvent(value, {
    event: "TaskCompleted",
    operation: { kind: "completion", taskId: "AT-001", outcome: "verified" },
  }), value.project);

  baseline.completion.deployment = { status: "passed", taskId: "AT-001", revision: value.revision };
  baseline.completion.cleanup = { status: "passed", taskId: "AT-001", revision: value.revision };
  await saveState(value, baseline);
  const complete = await evaluatePolicy(hookEvent(value, {
    event: "TaskCompleted",
    operation: { kind: "completion", taskId: "AT-001", outcome: "verified" },
  }), value.project);

  baseline.completion.checks = [];
  await saveState(value, baseline);
  const blocked = await evaluatePolicy(hookEvent(value, {
    event: "TaskCompleted",
    operation: { kind: "completion", taskId: "AT-001", outcome: "blocked" },
  }), value.project);

  assert.equal(missingScoped.allow, false);
  assert.equal(complete.allow, true);
  assert.equal(blocked.allow, true);
});

test("Codex canonical tracker status transitions use the explicit completion gate", async () => {
  // This test catches Codex completion enforcement being limited to Claude TaskCompleted.
  const value = await fixture();
  const project = await resolveProject(value.root);
  const operation = { kind: "file_change", files: [{
    action: "edit",
    path: ".agent-team/TASKS.md",
    previousContent: "| AT-001 | Complete fixture work | TEAM-001 | none | in_progress | rev | Verify. |",
    changedContent: "| AT-001 | Complete fixture work | TEAM-001 | none | verified | rev | Done. |",
  }] };
  const initialState = structuredClone(value.state);
  delete initialState.completion.taskId;
  await saveState(value, initialState);
  const allowed = await evaluatePolicy(hookEvent(value, { cwd: value.root, sessionId: "owner-session", operation }), project);
  const state = structuredClone(initialState);
  state.completion.checks = [];
  await saveState(value, state);
  const blocked = await evaluatePolicy(hookEvent(value, { cwd: value.root, sessionId: "owner-session", operation }), project);

  assert.equal(allowed.allow, true);
  assert.equal(blocked.allow, false);
});

test("Codex permits a verified flip only after valid HEAD completion gate evidence", async () => {
  const value = await fixture();
  const project = await resolveProject(value.root);
  const operation = { kind: "file_change", files: [{
    action: "edit", path: ".agent-team/TASKS.md",
    previousContent: "| AT-001 | Complete fixture work | TEAM-001 | none | in_progress | rev | Verify. |",
    changedContent: "| AT-001 | Complete fixture work | TEAM-001 | none | verified | rev | Done. |",
  }] };
  const state = structuredClone(value.state);
  state.completion = { requirementsReconciled: false, checks: [] };
  await saveState(value, state);
  const beforeEvidence = await evaluatePolicy(hookEvent(value, { cwd: value.root, sessionId: "owner-session", operation }), project);
  const evidencePath = path.join(value.root, ".agent-team/verification.json");
  await writeFile(evidencePath, JSON.stringify({
    status: "passed", revision: value.revision, taskIds: ["AT-001"], requirementsReconciled: true,
    review: { status: "passed", revision: value.revision, taskId: "AT-001" },
    checks: [{ name: "focused", status: "passed", revision: value.revision, taskId: "AT-001" }],
  }));
  const { recordGateEvidence } = await import("../hooks/lib/task-transitions.mjs");
  const canonical = await loadCanonicalState(project);
  const recorded = await recordGateEvidence(project, { actorSessionId: "owner-session", operationId: "completion-evidence",
    expectedVersion: canonical.state.stateVersion ?? 0, expectedFingerprint: canonical.tracker.fingerprint, gate: "completion",
    taskIds: ["AT-001"], expectedRevision: value.revision, evidencePath });
  const afterEvidence = await evaluatePolicy(hookEvent(value, { cwd: value.root, sessionId: "owner-session", operation }), project);

  assert.equal(beforeEvidence.allow, false);
  assert.equal(recorded.status, "applied");
  assert.equal(afterEvidence.allow, true);
});

test("post-tool file events run one changed-file lint batch with an installed executable", async () => {
  // This test catches a lint module that is never connected to the hook policy.
  const value = await fixture();
  const binary = path.join(value.feature, "node_modules", ".bin", "eslint");
  await mkdir(path.dirname(binary), { recursive: true });
  await writeFile(path.join(value.feature, "eslint.config.js"), "export default [];\n");
  await writeFile(binary, "#!/usr/bin/env node\nprocess.stdout.write('checked');\n");
  await chmod(binary, 0o755);
  const result = await evaluatePolicy(hookEvent(value, {
    event: "PostToolUse",
    operation: { kind: "file_change", files: [
      { action: "edit", path: "src/owned.js", changedContent: "export {};" },
      { action: "edit", path: "src/owned.js", changedContent: "export {};" },
    ] },
  }), value.project);

  assert.equal(result.allow, true);
  assert.equal(result.capabilities.lint.status, "checked");
  assert.equal(result.capabilities.lint.batches.length, 1);
});
