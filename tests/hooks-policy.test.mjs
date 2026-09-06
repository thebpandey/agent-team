import assert from "node:assert/strict";
import { mkdtemp, rm, symlink } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import test from "node:test";

import { resolveProject } from "../hooks/lib/project.mjs";
import { evaluatePolicy } from "../hooks/lib/policy.mjs";
import { hookEvent, policyFixture, saveState } from "./hook-test-helpers.mjs";

const temporary = [];
test.afterEach(async () => Promise.all(temporary.splice(0).map((item) => rm(item, { force: true, recursive: true }))));

async function fixture() {
  const root = await mkdtemp(path.join(os.tmpdir(), "agent-team-policy-"));
  temporary.push(root, `${root}-feature`);
  const value = await policyFixture(root);
  return { ...value, project: await resolveProject(value.feature) };
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
    { dryRunAt: undefined },
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
