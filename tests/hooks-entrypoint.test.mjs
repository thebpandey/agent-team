import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import { createHash } from "node:crypto";
import { chmod, cp, mkdir, mkdtemp, readFile, readdir, rm, writeFile } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import test from "node:test";

import { readActivationLogs } from "../hooks/lib/telemetry.mjs";
import { policyFixture } from "./hook-test-helpers.mjs";
import { normalizeEvent } from '../hooks/lib/event.mjs';

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

async function inactiveFixture() {
  const root = await mkdtemp(path.join(os.tmpdir(), "agent-team-entry-inactive-"));
  const home = await mkdtemp(path.join(os.tmpdir(), "agent-team-entry-home-"));
  temporary.push(root, home);
  const initialized = spawnSync("git", ["init", "-q", "-b", "main", root]);
  assert.equal(initialized.status, 0);
  return { root, home };
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

test('checkpoint events without native IDs remain distinct and complete batch IDs cannot collide', async () => {
  const value = await fixture();
  const payload = { cwd: value.feature, session_id: 'developer-session' };
  assert.equal(invoke('codex', 'PreCompact', payload, value.home).status, 0);
  const file = path.join(value.root, '.agent-team/checkpoints/developer-session.json');
  const first = JSON.parse(await readFile(file, 'utf8'));
  assert.equal(invoke('codex', 'PreCompact', payload, value.home).status, 0);
  const second = JSON.parse(await readFile(file, 'utf8'));
  assert.notEqual(second.eventId, first.eventId);
  assert.equal(second.version, first.version + 1);
  const calls = Array.from({ length: 21 }, (_, index) => ({ tool_use_id: `tool-${index}` }));
  const a = normalizeEvent('claude', 'PostToolBatch', { ...payload, tool_calls: calls });
  const b = normalizeEvent('claude', 'PostToolBatch', { ...payload, tool_calls: [...calls.slice(0, 20), { tool_use_id: 'different' }] });
  assert.notEqual(a.eventId, b.eventId);
  assert.equal(a.eventId, normalizeEvent('claude', 'PostToolBatch', { ...payload, tool_calls: calls }).eventId);
  const partial = { ...payload, tool_calls: [{ tool_use_id: 'known' }, {}] };
  assert.notEqual(normalizeEvent('claude', 'PostToolBatch', partial).eventId, normalizeEvent('claude', 'PostToolBatch', partial).eventId);
});

test('actual copied hook under a spaced installation path executes and records only its scoped transport', async () => {
  const directory = await mkdtemp(path.join(os.tmpdir(), 'agent team hook consumer '));
  temporary.push(directory);
  const value = await policyFixture(path.join(directory, 'project'));
  const packageRoot = path.join(value.root, '.agents/skills/agent-team');
  await mkdir(packageRoot, { recursive: true });
  await cp(path.dirname(hook), path.join(packageRoot, 'hooks'), { recursive: true });
  await mkdir(path.join(value.root, '.agent-team-hooks'));
  await writeFile(path.join(value.root, '.agent-team-hooks/install.json'), JSON.stringify({ targets: [{ runtime: 'codex', path: packageRoot }] }));
  const isolatedHome = path.join(directory, 'home');
  await mkdir(isolatedHome);
  const result = spawnSync(process.execPath, [path.join(packageRoot, 'hooks/agent-team-hook.mjs'), '--runtime', 'codex', '--event', 'PreToolUse'], {
    input: JSON.stringify({ cwd: value.feature, session_id: 'owner-session', tool_name: 'exec_command', tool_input: { cmd: 'git status --short' } }),
    encoding: 'utf8', env: { ...process.env, HOME: isolatedHome },
  });
  assert.equal(result.status, 0, result.stderr);
  assert.ok(result.stdout.trim(), 'copied hook must execute its main function');
  const events = JSON.parse(await readFile(path.join(value.root, '.agent-team-hooks/hook-events.json'), 'utf8'));
  assert.equal(events['codex:PreToolUse'].source, 'packaged_entrypoint');
  await assert.rejects(readFile(path.join(isolatedHome, '.agent-team-hooks/hook-events.json')), { code: 'ENOENT' });
});

function mappingDigest(value) {
  return createHash("sha256").update(JSON.stringify(value)).digest("hex");
}

function invokeCli(command, options) {
  const args = [cli, command];
  for (const [name, value] of Object.entries(options)) args.push(`--${name}`, value);
  const result = spawnSync(process.execPath, args, { encoding: "utf8" });
  return { status: result.status, output: JSON.parse(result.stdout), stderr: result.stderr };
}

for (const runtime of ["codex", "claude"]) {
  test(`${runtime} mapped-provider tracker completion requires the project owner (synthetic host payload)`, async () => {
    const value = await fixture();
    const result = invoke(runtime, "PreToolUse", { cwd: value.feature, session_id: "developer-session",
      tool_name: "mcp__filesystem__write", tool_input: { path: path.join(value.root, ".agent-team/TASKS.md"), content: "| AT-001 | TEAM-001 | verified |" },
    }, value.home);
    assert.equal(result.status, 0);
    assert.equal(output(result).hookSpecificOutput.permissionDecision, "deny");
    assert.match(output(result).hookSpecificOutput.permissionDecisionReason, /project owner|shared path/i);
  });
  test(`${runtime} subprocess stdout exposes actionable lint failure while permitting repair (synthetic host payload)`, async () => {
    const value = await fixture();
    const bin = path.join(value.feature, "node_modules/.bin/eslint");
    await mkdir(path.dirname(bin), { recursive: true });
    await writeFile(bin, "#!/usr/bin/env node\nprocess.stdout.write('src/owned.js:4:7 no-undef missingName\\n' + 'detail '.repeat(10000)); process.exitCode=1;\n");
    await chmod(bin, 0o755);
    const result = invoke(runtime, "PostToolUse", {
      cwd: value.feature, session_id: "developer-session",
      tool_name: runtime === "codex" ? "apply_patch" : "Edit",
      tool_input: runtime === "codex"
        ? { command: "*** Begin Patch\n*** Update File: src/owned.js\n+missingName();\n*** End Patch" }
        : { file_path: "src/owned.js", old_string: "export {};", new_string: "missingName();" },
    }, value.home);
    assert.equal(result.status, 0);
    assert.match(result.stdout, /owned.js:4:7.*no-undef.*missingName/);
    assert.match(result.stdout, /lint failed/i);
    assert.match(result.stdout, /log/i);
    assert.ok(Buffer.byteLength(result.stdout) < 7000);
    assert.doesNotMatch(result.stdout, /permissionDecision.*deny/);
  });
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

test("LeanCTX shell paths retain Agent-Team critical-operation coverage", async () => {
  // This catches ctx_shell or the explicit CLI wrapper hiding an integration command from Agent-Team.
  const home = await mkdtemp(path.join(os.tmpdir(), "agent-team-entry-home-"));
  temporary.push(home);
  const cwd = path.join(home, "missing-project");
  const results = [
    invoke("claude", "PreToolUse", {
      cwd,
      session_id: "owner-session",
      tool_name: "mcp__lean-ctx__ctx_shell",
      tool_input: { command: "git push origin main" },
    }, home),
    invoke("codex", "PreToolUse", {
      cwd,
      session_id: "owner-session",
      tool_name: "exec_command",
      tool_input: { cmd: 'lean-ctx -c --raw "git push origin main"' },
    }, home),
    invoke("codex", "PreToolUse", {
      cwd,
      session_id: "owner-session",
      tool_name: "exec_command",
      tool_input: { cmd: 'lean-ctx raw "git push origin main"' },
    }, home),
    invoke("codex", "PreToolUse", {
      cwd,
      session_id: "owner-session",
      tool_name: "exec_command",
      tool_input: { cmd: 'lean-ctx.exe -c --raw "git push origin main"' },
    }, home),
  ];

  for (const result of results) {
    assert.equal(result.status, 0);
    assert.equal(output(result).hookSpecificOutput.permissionDecision, "deny");
    assert.match(output(result).hookSpecificOutput.permissionDecisionReason, /unavailable/i);
  }
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
      tool_name: "mcp__lean-ctx__ctx_shell",
      tool_input: { command: "git status --short" },
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

test("standalone CLI does not expose a mapping cache write command", async () => {
  // This test catches a status or setup command becoming a second cache-content input.
  const value = await fixture();
  const cache = path.join(value.root, ".agent-team", "operation-mappings.json");
  await rm(cache);
  const migration = invokeCli("migrate-mappings", {
    project: value.feature,
  });

  assert.equal(migration.status, 1);
  assert.equal(migration.output.status, "failed");
  assert.match(migration.output.error, /unknown command/i);
  await assert.rejects(readFile(cache), { code: "ENOENT" });
});

test("caller-controlled hook fields cannot inject or alter mapping cache content", async () => {
  // This test catches runtime, event, session, or payload fields becoming mapping input or authority.
  const value = await fixture();
  const cache = path.join(value.root, ".agent-team", "operation-mappings.json");
  const state = structuredClone(value.state);
  state.operationMappings.providers.mcp__canonical__purge = { kind: "database_destructive", sqlField: "query" };
  await writeFile(path.join(value.root, ".agent-team", "state.json"), JSON.stringify(state, null, 2));
  const forged = invoke("claude", "SessionStart", {
    cwd: value.feature,
    runtime: "codex",
    event: "PostToolUse",
    session_id: "forged-owner-session",
    event_id: "forged-cache-write",
    operationMappings: {
      providers: { mcp__injected__destroy: { kind: "database_destructive", sqlField: "query" } },
      shell: [],
    },
  }, value.home);

  assert.equal(forged.status, 0);
  const receipt = JSON.parse(await readFile(cache, "utf8"));
  assert.equal(mappingDigest(receipt.operationMappings), mappingDigest(state.operationMappings));
  assert.equal(receipt.operationMappings.providers.mcp__canonical__purge.kind, "database_destructive");
  assert.equal(receipt.operationMappings.providers.mcp__injected__destroy, undefined);
});

test("a non-owner lifecycle event reproduces the exact canonical mapping digest", async () => {
  // This test catches cache refresh being gated by an unsigned session ID or changing canonical mappings.
  const value = await fixture();
  const cache = path.join(value.root, ".agent-team", "operation-mappings.json");
  await rm(cache);
  const first = invoke("codex", "SessionStart", {
    cwd: value.feature,
    session_id: "developer-session",
    event_id: "team-start",
  }, value.home);
  const firstSource = await readFile(cache, "utf8");
  const second = invoke("claude", "SessionStart", {
    cwd: value.feature,
    session_id: "unknown-session",
    event_id: "unknown-start",
  }, value.home);

  assert.equal(first.status, 0);
  assert.match(output(first).hookSpecificOutput.additionalContext, /mapping cache.*updated/i);
  assert.equal(second.status, 0);
  assert.match(output(second).hookSpecificOutput.additionalContext, /mapping cache.*current/i);
  assert.equal(await readFile(cache, "utf8"), firstSource);
  const receipt = JSON.parse(await readFile(cache, "utf8"));
  assert.equal(receipt.schemaVersion, 1);
  assert.equal(receipt.kind, "agent-team-operation-mapping-cache");
  assert.equal(receipt.authoritative, false);
  assert.equal(receipt.projectId, "project-1");
  assert.equal(receipt.sourcePath, ".agent-team/state.json");
  assert.equal(mappingDigest(receipt.operationMappings), mappingDigest(value.operationMappings));
});

test("malformed canonical state cannot update an existing mapping cache", async () => {
  // This test catches unvalidated state or unsigned payload content replacing the last valid cache.
  const value = await fixture();
  const cache = path.join(value.root, ".agent-team", "operation-mappings.json");
  const before = await readFile(cache, "utf8");
  await writeFile(path.join(value.root, ".agent-team", "state.json"), "{}\n");
  const result = invoke("codex", "SessionStart", {
    cwd: value.feature,
    session_id: "forged-owner-session",
    event_id: "malformed-state-start",
    operationMappings: { providers: {}, shell: [] },
  }, value.home);

  assert.equal(result.status, 0);
  assert.equal(await readFile(cache, "utf8"), before);
  assert.match(output(result).hookSpecificOutput.additionalContext, /mapping cache refresh.*unavailable.*unchanged/i);
  assert.doesNotMatch(output(result).hookSpecificOutput.additionalContext, /project owner/i);
});

test("mapped provider and app operations deny after lifecycle cache refresh", async () => {
  // This test catches a lifecycle cache refresh that writes mappings the fallback cannot consume.
  const value = await fixture();
  const cache = path.join(value.root, ".agent-team", "operation-mappings.json");
  await rm(cache);
  const migration = invoke("claude", "SessionStart", {
    cwd: value.feature,
    session_id: "developer-session",
    event_id: "cache-refresh",
  }, value.home);
  assert.equal(migration.status, 0);
  assert.match(String(output(migration).hookSpecificOutput.additionalContext ?? ""), /mapping cache/i);
  assert.equal(JSON.parse(await readFile(cache, "utf8")).projectId, "project-1");
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

test("state-file lifecycle events update the cache while read-only tools do not create it", async () => {
  // This test catches automatic cache writes on status events or stale cache after a canonical state change.
  const value = await fixture();
  const cache = path.join(value.root, ".agent-team", "operation-mappings.json");
  await rm(cache);
  const status = invoke("codex", "PreToolUse", {
    cwd: value.root,
    session_id: "developer-session",
    tool_name: "exec_command",
    tool_input: { cmd: "git status --short" },
  }, value.home);
  await assert.rejects(readFile(cache), { code: "ENOENT" });
  const initialMigration = invoke("codex", "SessionStart", {
    cwd: value.feature,
    session_id: "developer-session",
    event_id: "team-start-before-update",
  }, value.home);
  assert.equal(initialMigration.status, 0);
  assert.match(String(output(initialMigration).hookSpecificOutput.additionalContext ?? ""), /mapping cache/i);

  const state = structuredClone(value.state);
  state.operationMappings.providers.mcp__records__purge = { kind: "database_destructive", sqlField: "query" };
  await writeFile(path.join(value.root, ".agent-team", "state.json"), JSON.stringify(state, null, 2));
  const refresh = invoke("codex", "PostToolUse", {
    cwd: value.root,
    session_id: "developer-session",
    tool_name: "apply_patch",
    tool_input: { command: "*** Begin Patch\n*** Update File: .agent-team/state.json\n+ mapping changed\n*** End Patch" },
  }, value.home);
  assert.equal(status.status, 0);
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
  assert.match(output(checkpoint).systemMessage, /checkpoint.*unavailable/i);
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

test("direct Claude slash activation logs outside initialized Agent-Team projects", async () => {
  // This test catches global slash activation logging being coupled to project-scoped policy state.
  const value = await inactiveFixture();
  const result = invoke("claude", "UserPromptExpansion", {
    cwd: value.root,
    session_id: "global-slash-session",
    event_id: "global-slash-event",
    command_name: "agent-team",
    prompt: "/agent-team help with private context",
  }, value.home);
  const { records } = await readActivationLogs(path.join(value.home, ".agent-team-hooks", "logs"));

  assert.equal(result.status, 0);
  assert.equal(output(result).hookSpecificOutput.permissionDecision, undefined);
  assert.equal(records.length, 1);
  assert.equal(records[0].runtime, "claude");
  assert.equal(records[0].eventKind, "UserPromptExpansion");
  assert.equal(records[0].identityKind, "unregistered");
  assert.equal(records[0].projectId, undefined);
  assert.equal(records[0].teamId, undefined);
  assert.equal(JSON.stringify(records[0]).includes("private context"), false);
});

test("Claude Skill activation logs outside initialized Agent-Team projects", async () => {
  // This test catches global model-invoked activation logging being coupled to project state.
  const value = await inactiveFixture();
  const result = invoke("claude", "PreToolUse", {
    cwd: value.root,
    session_id: "global-skill-session",
    tool_name: "Skill",
    tool_input: { skill: "agent-team", prompt: "private skill input" },
    tool_use_id: "global-skill-event",
  }, value.home);
  const { records } = await readActivationLogs(path.join(value.home, ".agent-team-hooks", "logs"));

  assert.equal(result.status, 0);
  assert.equal(output(result).hookSpecificOutput.permissionDecision, undefined);
  assert.equal(records.length, 1);
  assert.equal(records[0].runtime, "claude");
  assert.equal(records[0].eventKind, "PreToolUse");
  assert.equal(records[0].identityKind, "unregistered");
  assert.equal(records[0].projectId, undefined);
  assert.equal(records[0].teamId, undefined);
  assert.equal(JSON.stringify(records[0]).includes("private skill input"), false);
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
