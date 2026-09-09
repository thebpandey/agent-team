import assert from "node:assert/strict";
import { mkdir, mkdtemp, readFile, readdir, rm, writeFile } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import test from "node:test";
import { getHealth } from "../hooks/lib/health.mjs";
import * as healthTools from "../hooks/lib/health.mjs";
import { policyFixture } from './hook-test-helpers.mjs';

const temporary = [];
test.afterEach(async () => Promise.all(temporary.splice(0).map((p) => rm(p, { recursive: true, force: true }))));

test("health distinguishes partial per-event registration, unsupported and unobserved execution without writes", async () => {
  const home = await mkdtemp(path.join(os.tmpdir(), "agent-team-event-health-"));
  temporary.push(home);
  await mkdir(path.join(home, ".codex"));
  await writeFile(path.join(home, ".codex/hooks.json"), JSON.stringify({ hooks: { PreToolUse: [{ hooks: [{ type: "command", command: "node /managed/agent-team-hook.mjs --runtime codex --event PreToolUse" }] }] } }));
  const before = await readdir(home, { recursive: true });
  const source = await readFile(path.join(home, ".codex/hooks.json"), "utf8");
  const health = await getHealth({ home });
  assert.deepEqual(health.runtimes.codex.events?.PreToolUse, { registered: true, supported: true, supportSource: "packaged_adapter", nativeSupport: "unknown", trusted: "unknown", exercise: "unobserved" });
  assert.deepEqual(health.runtimes.codex.events?.TaskCompleted, { registered: false, supported: false, supportSource: "packaged_adapter", nativeSupport: "unknown", trusted: "unknown", exercise: "unsupported" });
  assert.equal(health.runtimes.codex.events?.SessionStart.registered, false);
  assert.deepEqual(await readdir(home, { recursive: true }), before);
  assert.equal(await readFile(path.join(home, ".codex/hooks.json"), "utf8"), source);
});

test("one exercised or failed hook event never promotes other events or establishes trust", async () => {
  const home = await mkdtemp(path.join(os.tmpdir(), "agent-team-event-health-"));
  temporary.push(home);
  assert.equal(typeof healthTools.recordHookEvidence, "function");
  await healthTools.recordHookEvidence(home, { runtime: "claude", event: "PreToolUse", status: "passed", eventId: "first", sessionId: "s" });
  await healthTools.recordHookEvidence(home, { runtime: "claude", event: "PostToolBatch", status: "failed", eventId: "second", sessionId: "s" });
  const health = await getHealth({ home });
  assert.equal(health.runtimes.claude.events.PreToolUse.exercise, "exercised");
  assert.equal(health.runtimes.claude.events.PostToolBatch.exercise, "failed");
  assert.equal(health.runtimes.claude.events.SessionStart.exercise, "unobserved");
  assert.equal(health.runtimes.claude.events.PreToolUse.trusted, "unknown");
});

test('project-scope health reads that installation without promoting the user-scope host', async () => {
  const directory = await mkdtemp(path.join(os.tmpdir(), 'agent-team-scoped-health-'));
  temporary.push(directory);
  const fixture = await policyFixture(path.join(directory, 'project'));
  const home = path.join(directory, 'home');
  await mkdir(home);
  await mkdir(path.join(fixture.root, '.claude/skills/agent-team'), { recursive: true });
  await writeFile(path.join(fixture.root, '.claude/skills/agent-team/SKILL.md'), 'fixture');
  await writeFile(path.join(fixture.root, '.claude/settings.local.json'), JSON.stringify({ hooks: {
    PreToolUse: [{ hooks: [{ type: 'command', command: 'node /managed/agent-team-hook.mjs --runtime claude --event PreToolUse' }] }],
  } }));
  await healthTools.recordHookEvidence(fixture.root, { runtime: 'claude', event: 'PreToolUse', status: 'passed', eventId: 'project-only' });
  const health = await getHealth({ home, projectPath: fixture.feature, scope: 'project' });
  assert.equal(health.installation.scope, 'project');
  assert.equal(health.installation.root, fixture.root);
  assert.equal(health.runtimes.claude.installed, true);
  assert.equal(health.runtimes.claude.events.PreToolUse.exercise, 'exercised');
  assert.equal((await getHealth({ home })).runtimes.claude.installed, false);
  assert.equal((await getHealth({ home })).runtimes.claude.events.PreToolUse.exercise, 'unobserved');
});
