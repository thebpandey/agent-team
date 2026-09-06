import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import { mkdir, mkdtemp, readFile, readdir, rm, writeFile } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import test from "node:test";

import { policyFixture } from "./hook-test-helpers.mjs";

const hook = path.resolve(import.meta.dirname, "..", "hooks", "agent-team-hook.mjs");
const cli = path.resolve(import.meta.dirname, "..", "hooks", "agent-team-cli.mjs");
const temporary = [];

test.afterEach(async () => Promise.all(temporary.splice(0).map((item) => rm(item, { force: true, recursive: true }))));

async function fixture() {
  const root = await mkdtemp(path.join(os.tmpdir(), "agent-team-entry-"));
  const home = await mkdtemp(path.join(os.tmpdir(), "agent-team-entry-home-"));
  temporary.push(root, `${root}-feature`, `${root}-remote`, home);
  return { ...(await policyFixture(root)), home };
}

function invoke(runtime, event, payload, home) {
  const result = spawnSync(process.execPath, [hook, "--runtime", runtime, "--event", event], {
    input: JSON.stringify(payload),
    encoding: "utf8",
    env: { ...process.env, HOME: home },
  });
  return { status: result.status, stdout: result.stdout, stderr: result.stderr };
}

function output(result) {
  return JSON.parse(result.stdout);
}

function invokeCli(command, options) {
  const args = [cli, command];
  for (const [name, value] of Object.entries(options)) args.push(`--${name}`, value);
  const result = spawnSync(process.execPath, args, { encoding: "utf8" });
  return { status: result.status, output: JSON.parse(result.stdout), stderr: result.stderr };
}

test("entrypoint emits native denial when project resolution fails for a critical command", async () => {
  // This test catches the top-level exception handler exiting successfully without a deny result.
  const home = await mkdtemp(path.join(os.tmpdir(), "agent-team-entry-home-"));
  temporary.push(home);
  const result = invoke("codex", "PreToolUse", {
    cwd: path.join(home, "missing-project"),
    session_id: "owner-session",
    tool_name: "exec_command",
    tool_input: { cmd: "git push origin main" },
  }, home);

  assert.equal(result.status, 0);
  assert.equal(output(result).hookSpecificOutput.permissionDecision, "deny");
  assert.match(output(result).hookSpecificOutput.permissionDecisionReason, /unavailable/i);
});

test("entrypoint uses the separate mapping inventory when operational state is malformed", async () => {
  // This test catches fallback classification that forgets separately readable critical mappings.
  const value = await fixture();
  await writeFile(path.join(value.root, ".agent-team", "state.json"), "{bad json}\n");
  const provider = invoke("claude", "PreToolUse", {
    cwd: value.feature,
    session_id: "owner-session",
    tool_name: "mcp__database__execute",
    tool_input: { query: "DROP TABLE records" },
  }, value.home);
  const app = invoke("codex", "PreToolUse", {
    cwd: value.feature,
    session_id: "owner-session",
    tool_name: "exec_command",
    tool_input: { cmd: "node scripts/reset-data.mjs" },
  }, value.home);

  assert.equal(output(provider).hookSpecificOutput.permissionDecision, "deny");
  assert.equal(output(app).hookSpecificOutput.permissionDecision, "deny");
});

test("entrypoint leaves ordinary and unmapped read-only operations available when state is malformed", async () => {
  // This test catches an unavailable-state fallback that treats every shell or provider call as critical.
  const value = await fixture();
  await writeFile(path.join(value.root, ".agent-team", "state.json"), "{bad json}\n");
  const events = [
    invoke("codex", "PreToolUse", {
      cwd: value.feature,
      session_id: "owner-session",
      tool_name: "exec_command",
      tool_input: { cmd: "git status --short" },
    }, value.home),
    invoke("codex", "PreToolUse", {
      cwd: value.feature,
      session_id: "owner-session",
      tool_name: "exec_command",
      tool_input: { cmd: "echo ready" },
    }, value.home),
    invoke("claude", "PreToolUse", {
      cwd: value.feature,
      session_id: "owner-session",
      tool_name: "mcp__catalog__list_records",
      tool_input: { limit: 5 },
    }, value.home),
  ];

  for (const result of events) {
    assert.equal(result.status, 0);
    assert.equal(output(result).hookSpecificOutput.permissionDecision, undefined);
    assert.match(output(result).hookSpecificOutput.additionalContext, /advisory checks.*unavailable/i);
  }
});

test("entrypoint reports a missing cache and denies only static critical operations when state is malformed", async () => {
  // This test catches a missing fallback cache being described as mapped-operation protection.
  const value = await fixture();
  await rm(path.join(value.root, ".agent-team", "operation-mappings.json"));
  await writeFile(path.join(value.root, ".agent-team", "state.json"), "{bad json}\n");
  const mapped = invoke("claude", "PreToolUse", {
    cwd: value.feature,
    session_id: "owner-session",
    tool_name: "mcp__database__execute",
    tool_input: { query: "DROP TABLE records" },
  }, value.home);
  const staticCritical = invoke("codex", "PreToolUse", {
    cwd: value.feature,
    session_id: "owner-session",
    tool_name: "exec_command",
    tool_input: { cmd: "git push origin feature" },
  }, value.home);

  assert.equal(output(mapped).hookSpecificOutput.permissionDecision, undefined);
  assert.match(output(mapped).hookSpecificOutput.additionalContext, /mapping cache.*missing.*protection.*unavailable/i);
  assert.equal(output(staticCritical).hookSpecificOutput.permissionDecision, "deny");
  assert.match(output(staticCritical).hookSpecificOutput.permissionDecisionReason, /mapping cache.*missing/i);
});

test("entrypoint reports an invalid cache without treating its mappings as protected", async () => {
  // This test catches malformed cache bytes being accepted as critical mapping evidence.
  const value = await fixture();
  await writeFile(path.join(value.root, ".agent-team", "operation-mappings.json"), "{bad json}\n");
  await writeFile(path.join(value.root, ".agent-team", "state.json"), "{bad json}\n");
  const mapped = invoke("codex", "PreToolUse", {
    cwd: value.feature,
    session_id: "owner-session",
    tool_name: "exec_command",
    tool_input: { cmd: "node scripts/reset-data.mjs" },
  }, value.home);

  assert.equal(output(mapped).hookSpecificOutput.permissionDecision, undefined);
  assert.match(output(mapped).hookSpecificOutput.additionalContext, /mapping cache.*invalid.*protection.*unavailable/i);
});

test("CLI migrates healthy state mappings and the entrypoint enforces them after state corruption", async () => {
  // This test catches a migration command that writes no usable fallback policy receipt.
  const value = await fixture();
  const cache = path.join(value.root, ".agent-team", "operation-mappings.json");
  await rm(cache);
  const migration = invokeCli("migrate-mappings", {
    project: value.feature,
    session: "owner-session",
  });
  assert.equal(migration.status, 0);
  assert.equal(migration.output.status, "completed");
  assert.equal(migration.output.changed, true);
  const receipt = JSON.parse(await readFile(cache, "utf8"));
  assert.equal(receipt.schemaVersion, 1);
  assert.equal(receipt.kind, "agent-team-operation-mapping-cache");
  assert.equal(receipt.projectId, "project-1");
  assert.equal(receipt.sourcePath, ".agent-team/state.json");
  assert.equal(receipt.operationMappings.providers.mcp__database__execute.kind, "database_destructive");
  await writeFile(path.join(value.root, ".agent-team", "state.json"), "{bad json}\n");
  const provider = invoke("claude", "PreToolUse", {
    cwd: value.feature,
    session_id: "owner-session",
    tool_name: "mcp__database__execute",
    tool_input: { query: "DROP TABLE records" },
  }, value.home);
  const app = invoke("codex", "PreToolUse", {
    cwd: value.feature,
    session_id: "owner-session",
    tool_name: "exec_command",
    tool_input: { cmd: "node scripts/reset-data.mjs" },
  }, value.home);
  assert.equal(output(provider).hookSpecificOutput.permissionDecision, "deny");
  assert.equal(output(app).hookSpecificOutput.permissionDecision, "deny");
});

test("only owner post-tool state changes update the cache while read-only tools do not create it", async () => {
  // This test catches automatic cache writes on status events, non-owner writes, or unlocked owner refresh.
  const value = await fixture();
  const cache = path.join(value.root, ".agent-team", "operation-mappings.json");
  await rm(cache);
  const status = invoke("codex", "PreToolUse", {
    cwd: value.root,
    session_id: "owner-session",
    tool_name: "exec_command",
    tool_input: { cmd: "git status --short" },
  }, value.home);
  await assert.rejects(readFile(cache), { code: "ENOENT" });
  assert.equal(invokeCli("migrate-mappings", { project: value.feature, session: "owner-session" }).status, 0);

  const state = structuredClone(value.state);
  state.operationMappings.providers.mcp__records__purge = { kind: "database_destructive", sqlField: "query" };
  await writeFile(path.join(value.root, ".agent-team", "state.json"), JSON.stringify(state, null, 2));
  const nonOwner = invoke("codex", "PostToolUse", {
    cwd: value.root,
    session_id: "developer-session",
    tool_name: "apply_patch",
    tool_input: { command: "*** Begin Patch\n*** Update File: .agent-team/state.json\n+ mapping changed\n*** End Patch" },
  }, value.home);
  const beforeOwner = JSON.parse(await readFile(cache, "utf8"));
  const refresh = invoke("codex", "PostToolUse", {
    cwd: value.root,
    session_id: "owner-session",
    tool_name: "apply_patch",
    tool_input: { command: "*** Begin Patch\n*** Update File: .agent-team/state.json\n+ mapping changed\n*** End Patch" },
  }, value.home);

  assert.equal(status.status, 0);
  assert.match(String(output(nonOwner).hookSpecificOutput.additionalContext ?? ""), /canonical project owner/i);
  assert.equal(beforeOwner.operationMappings.providers.mcp__records__purge, undefined);
  assert.equal(refresh.status, 0);
  assert.match(String(output(refresh).hookSpecificOutput.additionalContext ?? ""), /mapping cache/i);
  const receipt = JSON.parse(await readFile(cache, "utf8"));
  assert.equal(receipt.operationMappings.providers.mcp__records__purge.kind, "database_destructive");
  assert.equal((await readdir(path.join(value.root, ".agent-team", ".locks"))).includes("operation-mappings.lock"), false);
  assert.equal((await readdir(path.join(value.root, ".agent-team"))).some((name) => name.endsWith(".tmp")), false);
});

test("entrypoint keeps checkpoint and telemetry failures visible and non-blocking", async () => {
  // This test catches advisory mutation failures escaping to a silent successful process.
  const value = await fixture();
  await writeFile(path.join(value.root, ".agent-team", "checkpoints"), "not a directory\n");
  const checkpoint = invoke("codex", "PreCompact", {
    cwd: value.feature,
    session_id: "developer-session",
    event_id: "compact-failure",
  }, value.home);
  await mkdir(path.join(value.home, ".agent-team-hooks"), { recursive: true });
  await writeFile(path.join(value.home, ".agent-team-hooks", "logs"), "not a directory\n");
  const telemetry = invoke("claude", "PreToolUse", {
    cwd: value.feature,
    session_id: "developer-session",
    tool_name: "Skill",
    tool_input: { skill: "agent-team" },
    tool_use_id: "activation-failure",
  }, value.home);

  assert.equal(checkpoint.status, 0);
  assert.match(output(checkpoint).hookSpecificOutput.additionalContext, /checkpoint.*unavailable/i);
  assert.equal(telemetry.status, 0);
  assert.match(output(telemetry).hookSpecificOutput.additionalContext, /activation logging.*unavailable/i);
});

test("entrypoint checkpoints canonical Git, identity, task, evidence, and pending-operation facts", async () => {
  // This test catches an entrypoint that supplies only paths to the checkpoint writer.
  const value = await fixture();
  await mkdir(path.join(value.root, ".agent-team", "checkpoints"), { recursive: true });
  await writeFile(path.join(value.root, ".agent-team", "checkpoints", "developer-session.json"), JSON.stringify({
    eventId: "old-event",
    sessionId: "developer-session",
    nextAction: "Run the owner handoff.",
    decisionNotes: "Keep the recorded release boundary.",
    updatedAt: "2026-09-06T12:00:00.000Z",
  }));
  const result = invoke("codex", "PreCompact", {
    cwd: value.feature,
    session_id: "developer-session",
    event_id: "compact-facts",
  }, value.home);
  const stored = JSON.parse(await readFile(path.join(value.root, ".agent-team", "checkpoints", "developer-session.json"), "utf8"));

  assert.equal(result.status, 0);
  assert.equal(stored.branch, "feature");
  assert.equal(stored.revision, value.revision);
  assert.equal(stored.teamId, "TEAM-001");
  assert.deepEqual(stored.taskIds, ["AT-001"]);
  assert.equal(stored.evidence.git, "current");
  assert.equal(stored.pendingOperations.some(({ kind }) => kind === "integration"), true);
  assert.equal(stored.nextAction, "Run the owner handoff.");
  assert.equal(stored.decisionNotes, "Keep the recorded release boundary.");
});

test("entrypoint activation log records registered team provenance", async () => {
  // This test catches activation records built before canonical identity is loaded.
  const value = await fixture();
  const result = invoke("claude", "PreToolUse", {
    cwd: value.feature,
    session_id: "developer-session",
    tool_name: "Skill",
    tool_input: { skill: "agent-team" },
    tool_use_id: "activation-team",
  }, value.home);
  const record = JSON.parse((await readFile(path.join(value.home, ".agent-team-hooks", "logs", "activation.jsonl"), "utf8")).trim());

  assert.equal(result.status, 0);
  assert.equal(record.projectId, "project-1");
  assert.equal(record.teamId, "TEAM-001");
  assert.equal(record.identityKind, "team");
});

test("Claude PostToolBatch runs one changed-file check and writes one factual checkpoint", async () => {
  // This test checks the native batch path rather than a per-edit PostToolUse path.
  const value = await fixture();
  const result = invoke("claude", "PostToolBatch", {
    cwd: value.feature,
    session_id: "developer-session",
    tool_calls: [
      { tool_name: "Write", tool_input: { file_path: "src/one.js", content: "export {};" }, tool_use_id: "tool-one" },
      { tool_name: "Edit", tool_input: { file_path: "src/two.js", old_string: "old", new_string: "new" }, tool_use_id: "tool-two" },
    ],
  }, value.home);
  const checkpoint = JSON.parse(await readFile(path.join(value.root, ".agent-team", "checkpoints", "developer-session.json"), "utf8"));

  assert.equal(result.status, 0);
  assert.equal(checkpoint.eventId, "batch:tool-one,tool-two");
  assert.equal(checkpoint.eventKind, "PostToolBatch");
  assert.equal(checkpoint.revision, value.revision);
  assert.match(output(result).additionalContext, /lint skipped/i);
});
