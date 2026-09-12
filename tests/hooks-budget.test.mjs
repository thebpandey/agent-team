import assert from "node:assert/strict";
import { execFileSync } from "node:child_process";
import { chmod, mkdir, mkdtemp, open, readdir, rm, writeFile } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import test from "node:test";
import { runNormalizedHook } from "../hooks/agent-team-hook.mjs";
import { resolveProject } from "../hooks/lib/project.mjs";
import { evaluatePolicy } from "../hooks/lib/policy.mjs";
import { loadCanonicalState } from "../hooks/lib/canonical-state.mjs";
import { withDirectoryLock } from "../hooks/lib/lock.mjs";
import { createEventBudget } from "../hooks/lib/budget.mjs";
import { appendActivationLog, readActivationLogs } from "../hooks/lib/telemetry.mjs";
import { hookEvent, policyFixture } from "./hook-test-helpers.mjs";

const temporary = [];
test.afterEach(async () => Promise.all(temporary.splice(0).map((p) => rm(p, { recursive: true, force: true }))));
async function fixture() {
  const root = await mkdtemp(path.join(os.tmpdir(), "agent-team-budget-"));
  temporary.push(root, `${root}-feature`, `${root}-remote`);
  return policyFixture(root);
}

test("one event deadline bounds successive lint roots and preserves completed lint evidence", async () => {
  const value = await fixture();
  for (const name of ["fast", "slow", "later"]) {
    const directory = path.join(value.feature, "src", name);
    await mkdir(path.join(directory, "node_modules/.bin"), { recursive: true });
    await writeFile(path.join(directory, "eslint.config.js"), "export default [];\n");
    const bin = path.join(directory, "node_modules/.bin/eslint");
    await writeFile(bin, `#!/usr/bin/env node\n${name === "fast" ? "process.stdout.write('ok');" : "setTimeout(() => {}, 1800);"}\n`);
    await chmod(bin, 0o755);
  }
  const start = performance.now();
  const { decision } = await runNormalizedHook(hookEvent(value, { event: "PostToolUse", operation: { kind: "file_change", files: ["fast", "slow", "later"].map((name) => ({ path: `src/${name}/index.js`, changedContent: "export {};" })) } }), { timeoutMs: 250 });
  const elapsed = performance.now() - start;
  assert.ok(elapsed < 700, `event took ${elapsed}ms`);
  assert.equal(decision.allow, true);
  assert.equal(decision.capabilities.lint.status, "timeout");
  assert.deepEqual(decision.capabilities.lint.batches.map(({ status }) => status), ["passed", "timeout", "timeout"]);
});

test("expired event deadline holds consequential completion and leaves advisory work available", async () => {
  const value = await fixture();
  const completion = await runNormalizedHook(hookEvent(value, { operation: { kind: "completion", taskId: "AT-001" } }), { timeoutMs: 0 });
  assert.equal(completion.decision.allow, false);
  assert.match(completion.decision.messages.join(" "), /deadline|unavailable/i);
  const ordinary = await runNormalizedHook(hookEvent(value, { operation: { kind: "ordinary" } }), { timeoutMs: 0 });
  assert.equal(ordinary.decision.allow, true);
});

test("ordinary advisory events do not query Beads", async () => {
  const value = await fixture();
  await writeFile(path.join(value.root, ".agent-team/setup.json"), JSON.stringify({ skill: "agent-team", projectId: "project-1", tracker: { kind: "beads" } }));
  let calls = 0;
  const result = await evaluatePolicy(hookEvent(value, { operation: { kind: "ordinary" } }), await resolveProject(value.feature), {
    runBeads: async () => { calls += 1; throw new Error("unexpected tracker probe"); },
  });
  assert.equal(result.allow, true);
  assert.equal(calls, 0);
});

test("same-revision lint evidence cannot complete changed tracked content, but repair remains allowed", async () => {
  const value = await fixture();
  value.state.completion.checks = [{ name: "lint", status: "passed", revision: value.revision }];
  await writeFile(path.join(value.root, ".agent-team/state.json"), JSON.stringify(value.state));
  await writeFile(path.join(value.feature, "src/owned.js"), "missingName();\n");
  const result = await runNormalizedHook(hookEvent(value, { operation: { kind: "completion", taskId: "AT-001" } }));
  assert.equal(result.decision.allow, false);
  assert.match(result.decision.messages.join(" "), /changed|dirty|delta/i);
  const repair = await runNormalizedHook(hookEvent(value, { operation: { kind: "file_change", files: [{ path: "src/owned.js", changedContent: "export {};" }] } }));
  assert.equal(repair.decision.allow, true);
});

test("unchanged advisory output is deduplicated per session and refreshed after changed input", async () => {
  const value = await fixture();
  const event = hookEvent(value, { event: "PostToolUse", operation: { kind: "file_change", files: [{ path: "src/owned.js", changedContent: "missingName();" }] } });
  const first = await runNormalizedHook(event);
  const second = await runNormalizedHook(event);
  assert.ok(first.decision.messages.length > 0);
  assert.equal(second.decision.messages.length, 0);
  event.operation.files[0].changedContent = "anotherName();";
  const changed = await runNormalizedHook(event);
  assert.ok(changed.decision.messages.length > 0);
  event.sessionId = "other-session";
  assert.ok((await runNormalizedHook(event)).decision.messages.length > 0);
});

test("deadline during ownership phase cannot allow classified consequential commands", async () => {
  const value = await fixture();
  const project = await resolveProject(value.feature);
  const canonical = await loadCanonicalState(project);
  for (const operation of [
    { kind: "completion", taskId: "AT-001" },
    { kind: "shell", command: "git push origin feature" },
    { kind: "shell", command: "npm publish" },
    { kind: "provider", tool: "mcp__database__execute", input: { query: "DROP TABLE records" } },
  ]) {
    let phases = 0;
    const budget = { run: async (action) => {
      if (++phases === 2) throw Object.assign(new Error("deadline"), { code: "EVENT_DEADLINE" });
      return action();
    } };
    const result = await evaluatePolicy(hookEvent(value, { sessionId: "owner-session", operation }), project, { canonical, budget });
    assert.equal(result.allow, false, JSON.stringify(operation));
    assert.match(result.messages.join(" "), /unavailable/i);
  }
});

test("deadline while waiting for a lock never runs its mutation after the lock is released", async () => {
  const value = await fixture();
  const project = await resolveProject(value.feature);
  const lock = path.join(project.paths.locks, "held.lock");
  await mkdir(lock, { recursive: true });
  const budget = createEventBudget(20);
  let mutated = false;
  try {
    await assert.rejects(withDirectoryLock(lock, { operationId: "waiter" }, async () => { mutated = true; }, { budget, timeoutMs: 500 }), /deadline|abort/i);
  } finally { budget.close(); }
  await rm(lock, { recursive: true });
  await new Promise((resolve) => setTimeout(resolve, 40));
  assert.equal(mutated, false);
  assert.deepEqual(await readdir(project.paths.locks), []);
});

test("outer event deadline bounds an unresponsive tracker read after canonical discovery", async () => {
  const value = await fixture();
  await writeFile(path.join(value.root, ".agent-team/setup.json"), JSON.stringify({ skill: "agent-team", projectId: "project-1", tracker: { kind: "beads" } }));
  const start = performance.now();
  const result = await runNormalizedHook(hookEvent(value, { operation: { kind: "completion", taskId: "AT-001" } }), {
    timeoutMs: 40,
    runBeads: async () => { await new Promise((resolve) => setTimeout(resolve, 200)); return { stdout: "[]" }; },
  });
  assert.ok(performance.now() - start < 150);
  assert.equal(result.decision.allow, false);
});

test("activation logging cannot append after its lock waiter exceeds the shared deadline", async () => {
  const value = await fixture();
  const directory = path.join(value.root, "activation-test");
  const lock = path.join(directory, ".activation.lock");
  await mkdir(lock, { recursive: true });
  const budget = createEventBudget(20);
  try {
    await assert.rejects(appendActivationLog(directory, { runtime: "claude", sessionId: "session", correlationId: "event" }, { budget }), /deadline|abort/i);
  } finally { budget.close(); }
  await rm(lock, { recursive: true });
  await new Promise((resolve) => setTimeout(resolve, 40));
  assert.deepEqual((await readActivationLogs(directory)).records, []);
});

test("outer deadline preserves mapped destructive classification when canonical state read stalls after inventory", async () => {
  const value = await fixture();
  const statePath = path.join(value.root, ".agent-team/state.json");
  await rm(statePath);
  execFileSync("mkfifo", [statePath]);
  const fifo = await open(statePath, "r+");
  try {
    const releaseRead = new Promise((resolve, reject) => setTimeout(() => {
      writeFile(statePath, JSON.stringify(value.state)).then(resolve, reject);
    }, 150));
    const result = await runNormalizedHook(hookEvent(value, { sessionId: "owner-session", operation: {
      kind: "provider", tool: "mcp__database__execute", input: { query: "DROP TABLE records" },
    } }), { timeoutMs: 50 });
    await releaseRead;
    assert.equal(result.decision.allow, false);
    assert.match(result.decision.messages.join(" "), /unavailable/i);
  } finally {
    await fifo.close();
  }
});
