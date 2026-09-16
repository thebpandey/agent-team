import assert from "node:assert/strict";
import { mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import { execFileSync, spawnSync } from "node:child_process";
import os from "node:os";
import path from "node:path";
import test from "node:test";

import { runCommand } from "../hooks/agent-team-cli.mjs";
import { runNormalizedHook } from "../hooks/agent-team-hook.mjs";
import { initializeProject } from "../hooks/lib/initialization.mjs";

const temporary = [];
test.afterEach(async () => Promise.all(temporary.splice(0).map((entry) => rm(entry, { recursive: true, force: true }))));

async function fixture(host = "codex") {
  const root = await mkdtemp(path.join(os.tmpdir(), "agent-team-setup-native-"));
  temporary.push(root);
  execFileSync("git", ["init", "--quiet", "-b", "main", root]);
  const initialized = await initializeProject(root, { projectId: "native-setup-project", operationId: "initialize-native-setup", source: "standalone",
    tracker: { kind: "markdown", path: ".agent-team/TASKS.md" }, plan: { scope: "Native setup bridge", acceptance: ["Routes pass"],
      branch: "main", verification: ["node --test"], authority: { ownedPaths: ["hooks/**"] },
      tasks: [{ id: "AT-001", title: "Bridge setup", status: "todo", dependencies: [] }] } },
  { actorSessionId: "owner", nativeIdentity: { host, sessionId: "owner", observed: true, cwd: root } });
  assert.equal(initialized.status, "applied");
  return { root, setup: path.join(root, ".agent-team/setup.json"), cli: path.resolve(import.meta.dirname, "../hooks/agent-team-cli.mjs") };
}

async function envelope(value, operationId, request) {
  const setup = JSON.parse(await readFile(value.setup, "utf8"));
  const file = path.join(value.root, `${operationId}.json`);
  await writeFile(file, `${JSON.stringify({ schemaVersion: 1, expectedVersion: setup.version, operationId,
    writer: { id: "owner", role: "project_orchestrator" }, request }, null, 2)}\n`);
  return file;
}

function shell(value, command, request, host = "codex") {
  return `node ${value.cli} ${command} --project ${value.root} --host ${host} --scope project --request ${request}`;
}

async function invoke(value, command, request, options = {}) {
  return runNormalizedHook({ runtime: "codex", event: "PreToolUse", cwd: value.root, sessionId: "owner",
    eventId: `${command}-event`, operation: { kind: "shell", command: shell(value, command, request) } }, options);
}

function invokeExecutable(value, command, { runtime = "codex", env, toolInput = {} } = {}) {
  const hook = path.resolve(import.meta.dirname, "../hooks/agent-team-hook.mjs");
  const result = spawnSync(process.execPath, [hook, "--runtime", runtime, "--event", "PreToolUse"], {
    input: JSON.stringify({ cwd: value.root, session_id: "owner", event_id: "executable-context-event",
      tool_name: "exec_command", tool_input: { cmd: command, ...toolInput } }), encoding: "utf8", env,
  });
  assert.equal(result.status, 0, result.stderr);
  return JSON.parse(result.stdout);
}

function invokeCli(value, command, request, host = "codex") {
  const result = spawnSync(process.execPath, [value.cli, command, "--project", value.root, "--host", host, "--scope", "project",
    "--request", request], { encoding: "utf8" });
  assert.equal(result.status, 0, result.stderr);
  return JSON.parse(result.stdout);
}

const inventory = { schemaVersion: 1, host: "codex", observed: true, profile: { id: "quality", required: ["mcp:tracker"] }, capabilities: [
  { kind: "mcp", id: "tracker", enabled: true, visible: true, label: "Tracker", source: "native" },
  { kind: "mcp", id: "browser", enabled: true, visible: true, label: "Browser", source: "native" },
] };

test("native helper install route accepts committed terminal status and links canonical receipt", async () => {
  const value = await fixture();
  const request = await envelope(value, "native-helper-install", {});
  assert.deepEqual(await runCommand("helpers-install", { project: value.root, host: "codex", scope: "project", request }),
    { status: "conflict", reason: "native_project_context_required" });
  const hooked = await invoke(value, "helpers-install", request);
  assert.equal(hooked.decision.allow, true);
  assert.deepEqual(hooked.decision.mutations.find(({ kind }) => kind === "helpers_install"), {
    kind: "helpers_install", command: "helpers-install", status: "installed",
  });
  const setup = JSON.parse(await readFile(value.setup, "utf8"));
  assert.equal(setup.helpers.kind, "agent-team-helpers");
});

test("native context apply and revert consume only trusted inventory and preserve setup receipts", async () => {
  const value = await fixture();
  const options = { project: value.root, host: "codex", scope: "project" };
  const offered = await runCommand("context-reduction", options, { capabilityInventory: inventory });
  const applyRequest = await envelope(value, "native-context-apply", { proposal: offered.proposal,
    decision: { action: "apply", reviewed: true, proposalId: offered.proposal.proposalId, selected: ["mcp:browser"] } });
  const applied = await invoke(value, "context-reduction-apply", applyRequest, { capabilityInventory: inventory });
  assert.equal(applied.decision.allow, true);
  assert.equal(applied.decision.mutations.find(({ kind }) => kind === "context_reduction_apply").status, "applied");
  let setup = JSON.parse(await readFile(value.setup, "utf8"));
  const revertRequest = await envelope(value, "native-context-revert", { expectedReceiptDigest: setup.contextReduction.digest });
  const reverted = await invoke(value, "context-reduction-revert", revertRequest);
  assert.equal(reverted.decision.allow, true);
  assert.equal(reverted.decision.mutations.find(({ kind }) => kind === "context_reduction_revert").status, "reverted");
  setup = JSON.parse(await readFile(value.setup, "utf8"));
  assert.equal(setup.contextReduction.kind, "context-reduction-revert");
});

test("native context cancel and no-answer are successful zero-write outcomes", async () => {
  const value = await fixture();
  const before = await readFile(value.setup);
  for (const action of ["cancel", "no_answer"]) {
    const request = await envelope(value, `native-context-${action}`, { proposal: null, decision: { action } });
    const outcome = await invoke(value, "context-reduction-apply", request);
    assert.equal(outcome.decision.allow, true, outcome.decision.messages.join("\n"));
    assert.equal(outcome.decision.mutations.find(({ kind }) => kind === "context_reduction_apply").status, action);
    assert.deepEqual(await readFile(value.setup), before);
  }
});

test("executable context flow uses acknowledged owner-reported visibility and rejects missing stale chained or extra input", async () => {
  const value = await fixture();
  const options = { project: value.root, host: "codex", scope: "project" };
  const offered = await runCommand("context-reduction", options, { capabilityInventory: inventory });
  const request = await envelope(value, "native-context-executable", { proposal: offered.proposal,
    decision: { action: "apply", reviewed: true, proposalId: offered.proposal.proposalId, selected: ["mcp:browser"] } });
  const before = await readFile(value.setup);
  const executable = invokeExecutable(value, shell(value, "context-reduction-apply", request));
  assert.equal(executable.hookSpecificOutput.permissionDecision, "deny");
  assert.match(executable.hookSpecificOutput.permissionDecisionReason, /visibility_unknown/);
  assert.deepEqual(await readFile(value.setup), before);

  const chained = invokeExecutable(value, `${shell(value, "context-reduction-apply", request)} && true`);
  assert.notEqual(chained.hookSpecificOutput.permissionDecision, "deny");
  assert.deepEqual(await readFile(value.setup), before);

  const changedInventory = structuredClone(inventory);
  changedInventory.capabilities[1].enabled = false;
  const stale = await invoke(value, "context-reduction-apply", request, { capabilityInventory: changedInventory });
  assert.equal(stale.decision.allow, false);
  assert.match(stale.decision.messages.join("\n"), /proposal_stale/);
  assert.deepEqual(await readFile(value.setup), before);

  const setup = JSON.parse(before);
  setup.dependencies = { hosts: { codex: { scope: "project", selected: ["tracker"], receipts: [] } } };
  await writeFile(value.setup, `${JSON.stringify(setup, null, 2)}\n`);
  const report = { schemaVersion: 1, host: "codex", capabilities: [
    { kind: "mcp", id: "tracker", enabled: true, visible: true, label: "Tracker" },
    { kind: "mcp", id: "browser", enabled: true, visible: true, label: "Browser" },
  ] };
  const offerRequest = await envelope(value, "manual-context-offer", { ownerReportedCapabilities: report });
  const manual = invokeCli(value, "context-reduction", offerRequest);
  assert.equal(manual.status, "offered_unverified");
  assert.equal(manual.visibility, "unknown");
  assert.deepEqual(manual.proposal.required, ["mcp:tracker"]);
  assert.equal(manual.proposal.candidates[0].source, "owner_reported_unverified");

  const decision = { action: "apply", reviewed: true, proposalId: manual.proposal.proposalId, selected: ["mcp:browser"] };
  const noAck = await envelope(value, "manual-context-no-ack", { proposal: manual.proposal, ownerReportedCapabilities: report, decision });
  const refused = invokeExecutable(value, shell(value, "context-reduction-apply", noAck));
  assert.equal(refused.hookSpecificOutput.permissionDecision, "deny");
  assert.match(refused.hookSpecificOutput.permissionDecisionReason, /visibility_acknowledgement_required/);

  const changedReport = structuredClone(report);
  changedReport.capabilities[1].enabled = false;
  const staleRequest = await envelope(value, "manual-context-stale", { proposal: manual.proposal, ownerReportedCapabilities: changedReport,
    decision: { ...decision, visibilityAcknowledged: true } });
  const staleExecutable = invokeExecutable(value, shell(value, "context-reduction-apply", staleRequest));
  assert.equal(staleExecutable.hookSpecificOutput.permissionDecision, "deny");
  assert.match(staleExecutable.hookSpecificOutput.permissionDecisionReason, /proposal_stale/);

  const extraRequest = await envelope(value, "manual-context-extra-home", { proposal: manual.proposal, ownerReportedCapabilities: report,
    decision: { ...decision, visibilityAcknowledged: true }, home: value.root });
  const extra = invokeExecutable(value, shell(value, "context-reduction-apply", extraRequest));
  assert.equal(extra.hookSpecificOutput.permissionDecision, "deny");
  assert.match(extra.hookSpecificOutput.permissionDecisionReason, /home|field/i);

  const applyRequest = await envelope(value, "manual-context-apply", { proposal: manual.proposal, ownerReportedCapabilities: report,
    decision: { ...decision, visibilityAcknowledged: true } });
  const applied = invokeExecutable(value, shell(value, "context-reduction-apply", applyRequest));
  assert.notEqual(applied.hookSpecificOutput.permissionDecision, "deny");
  let current = JSON.parse(await readFile(value.setup, "utf8"));
  assert.equal(current.contextReduction.changes[0].source, "owner_reported_unverified");
  const revertRequest = await envelope(value, "manual-context-revert", { expectedReceiptDigest: current.contextReduction.digest });
  const reverted = invokeExecutable(value, shell(value, "context-reduction-revert", revertRequest));
  assert.notEqual(reverted.hookSpecificOutput.permissionDecision, "deny");
  current = JSON.parse(await readFile(value.setup, "utf8"));
  assert.equal(current.contextReduction.kind, "context-reduction-revert");
});

test("Claude executable derives trusted context home from its process and ignores tool input home", async () => {
  const value = await fixture("claude-code");
  const trustedHome = await mkdtemp(path.join(os.tmpdir(), "agent-team-native-claude-home-"));
  const untrustedHome = await mkdtemp(path.join(os.tmpdir(), "agent-team-native-untrusted-home-"));
  temporary.push(trustedHome, untrustedHome);
  const setup = JSON.parse(await readFile(value.setup, "utf8"));
  setup.dependencies = { hosts: { "claude-code": { scope: "project", selected: ["tracker"], receipts: [] } } };
  await writeFile(value.setup, `${JSON.stringify(setup, null, 2)}\n`);
  const report = { schemaVersion: 1, host: "claude-code", capabilities: [
    { kind: "mcp", id: "tracker", enabled: true, visible: true, label: "Tracker" },
    { kind: "mcp", id: "browser", enabled: true, visible: true, label: "Browser" },
  ] };
  const offerRequest = await envelope(value, "claude-context-offer", { ownerReportedCapabilities: report });
  const offered = invokeCli(value, "context-reduction", offerRequest, "claude-code");
  const applyRequest = await envelope(value, "claude-context-apply", { proposal: offered.proposal,
    ownerReportedCapabilities: report, decision: { action: "apply", reviewed: true, visibilityAcknowledged: true,
      proposalId: offered.proposal.proposalId, selected: ["mcp:browser"] } });
  const environment = { ...process.env, HOME: trustedHome };
  const applied = invokeExecutable(value, shell(value, "context-reduction-apply", applyRequest, "claude-code"), {
    runtime: "claude", env: environment, toolInput: { HOME: untrustedHome },
  });
  assert.notEqual(applied.hookSpecificOutput.permissionDecision, "deny",
    applied.hookSpecificOutput.permissionDecisionReason);
  const nativeConfig = JSON.parse(await readFile(path.join(trustedHome, ".claude.json"), "utf8"));
  assert.deepEqual(nativeConfig.projects[value.root].disabledMcpServers, ["browser"]);
  await assert.rejects(readFile(path.join(untrustedHome, ".claude.json")), { code: "ENOENT" });

  const current = JSON.parse(await readFile(value.setup, "utf8"));
  const revertRequest = await envelope(value, "claude-context-revert", {
    expectedReceiptDigest: current.contextReduction.digest,
  });
  const reverted = invokeExecutable(value, shell(value, "context-reduction-revert", revertRequest, "claude-code"), {
    runtime: "claude", env: environment, toolInput: { HOME: untrustedHome },
  });
  assert.notEqual(reverted.hookSpecificOutput.permissionDecision, "deny",
    reverted.hookSpecificOutput.permissionDecisionReason);
  const restored = JSON.parse(await readFile(path.join(trustedHome, ".claude.json"), "utf8"));
  assert.deepEqual(restored.projects[value.root].disabledMcpServers, []);
});
