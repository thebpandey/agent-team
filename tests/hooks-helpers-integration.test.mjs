import assert from "node:assert/strict";
import { execFile } from "node:child_process";
import { mkdtemp, mkdir, readFile, rm, writeFile } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import { promisify } from "node:util";
import test from "node:test";

import { runCommand } from "../hooks/agent-team-cli.mjs";
import { applyContextReduction, revertContextReduction } from "../hooks/lib/context-shrink.mjs";
import { installHelpers } from "../hooks/lib/helpers.mjs";
import { initializeProject } from "../hooks/lib/initialization.mjs";
import { orchestrateSetup } from "../hooks/lib/setup-cli.mjs";

const temporary = [];
const exec = promisify(execFile);
test.afterEach(async () => Promise.all(temporary.splice(0).map((entry) => rm(entry, { recursive: true, force: true }))));

async function fixture() {
  const root = await mkdtemp(path.join(os.tmpdir(), "agent-team-helper-integration-"));
  temporary.push(root);
  await import("node:child_process").then(({ execFileSync }) => execFileSync("git", ["init", "--quiet", "-b", "main", root]));
  const initialized = await initializeProject(root, {
    projectId: "helpers-project", operationId: "initialize-helpers", source: "standalone",
    tracker: { kind: "markdown", path: ".agent-team/TASKS.md" },
    plan: { scope: "Integrate helpers", acceptance: ["Routes pass"], branch: "main", verification: ["node --test"],
      authority: { ownedPaths: ["hooks/**"] }, tasks: [{ id: "AT-001", title: "Integrate helpers", status: "todo", dependencies: [] }] },
  }, { actorSessionId: "owner", nativeIdentity: { host: "codex", sessionId: "owner", observed: true, cwd: root } });
  assert.equal(initialized.status, "applied", initialized.reason);
  const setupPath = path.join(root, ".agent-team", "setup.json");
  const nativeIdentity = { host: "codex", sessionId: "owner", observed: true, cwd: root, ownershipEpoch: 1 };
  const options = { project: root, host: "codex", scope: "project" };
  return { root, setupPath, nativeIdentity, options };
}

async function envelope(value, request, { operationId, expectedVersion } = {}) {
  const setup = JSON.parse(await readFile(value.setupPath, "utf8"));
  const file = path.join(value.root, `${operationId ?? "helper-operation"}.json`);
  await writeFile(file, JSON.stringify({ schemaVersion: 1, expectedVersion: expectedVersion ?? setup.version ?? 0,
    operationId: operationId ?? "helper-operation", writer: { id: "owner", role: "project_orchestrator" }, request }));
  return file;
}

const inventory = {
  schemaVersion: 1, host: "codex", observed: true, profile: { id: "quality", required: ["mcp:tracker"] },
  capabilities: [
    { kind: "mcp", id: "tracker", enabled: true, visible: true, label: "Tracker", source: "native" },
    { kind: "mcp", id: "browser", enabled: true, visible: true, label: "Browser", source: "native" },
    { kind: "plugin", id: "hidden", enabled: true, visible: false, label: "Hidden", source: "native" },
  ],
};

test("helpers route installs through the canonical setup transaction and health reports linkage", async () => {
  const value = await fixture();
  const before = await readFile(value.setupPath);
  const inspection = await runCommand("helpers", value.options);
  assert.equal(inspection.status, "drift");
  assert.deepEqual(await readFile(value.setupPath), before);

  const request = await envelope(value, {}, { operationId: "install-helpers" });
  const refused = await runCommand("helpers-install", { ...value.options, request });
  assert.deepEqual(refused, { status: "conflict", reason: "native_project_context_required" });
  await assert.rejects(readFile(path.join(value.root, "scripts", "agent-team", "with-env.mjs")), { code: "ENOENT" });

  const installed = await runCommand("helpers-install", { ...value.options, request }, { nativeIdentity: value.nativeIdentity });
  assert.equal(installed.status, "installed");
  const setup = JSON.parse(await readFile(value.setupPath, "utf8"));
  const local = JSON.parse(await readFile(path.join(value.root, ".agent-team", "helpers.json"), "utf8"));
  assert.deepEqual(setup.helpers, local);
  const health = await runCommand("health", { project: value.root, scope: "project", home: value.root });
  assert.equal(health.helpers.status, "current");
  assert.equal(health.helpers.canonicalReceipt, "linked");
  assert.equal(health.helpers.pendingTransactionDigest, null);
});

test("executable public CLI inspects helpers but cannot invent mutation authority or capability visibility", async () => {
  const value = await fixture();
  const cli = path.resolve(import.meta.dirname, "../hooks/agent-team-cli.mjs");
  const base = ["--project", value.root, "--host", "codex", "--scope", "project"];
  const inspected = JSON.parse((await exec(process.execPath, [cli, "helpers", ...base])).stdout);
  assert.equal(inspected.status, "drift");
  const unknown = JSON.parse((await exec(process.execPath, [cli, "context-reduction", ...base])).stdout);
  assert.equal(unknown.status, "visibility_unknown");
  assert.equal(unknown.proposal, null);
  const request = await envelope(value, {}, { operationId: "public-helper-install" });
  const refused = JSON.parse((await exec(process.execPath, [cli, "helpers-install", ...base, "--request", request])).stdout);
  assert.deepEqual(refused, { status: "conflict", reason: "native_project_context_required" });
  await assert.rejects(readFile(path.join(value.root, "scripts", "agent-team", "with-env.mjs")), { code: "ENOENT" });
});

test("context routes require observed host visibility and bind apply and revert receipts to setup", async () => {
  const value = await fixture();
  const before = await readFile(value.setupPath);
  const unknown = await runCommand("context-reduction", value.options, { nativeIdentity: value.nativeIdentity });
  assert.equal(unknown.status, "visibility_unknown");
  assert.deepEqual(await readFile(value.setupPath), before);

  const offered = await runCommand("context-reduction", value.options, {
    nativeIdentity: value.nativeIdentity, capabilityInventory: inventory,
  });
  assert.deepEqual(offered.proposal.candidates.map(({ key }) => key), ["mcp:browser", "mcp:tracker"]);

  for (const action of ["no_answer", "cancel"]) {
    const noWriteRequest = await envelope(value, { decision: { action } }, { operationId: `context-${action}` });
    const noWrite = await runCommand("context-reduction-apply", { ...value.options, request: noWriteRequest }, {
      nativeIdentity: value.nativeIdentity,
    });
    assert.deepEqual(noWrite, { status: action, changed: false });
    assert.deepEqual(await readFile(value.setupPath), before);
    await assert.rejects(readFile(path.join(value.root, ".codex", "config.toml")), { code: "ENOENT" });
  }

  const applyRequest = await envelope(value, { proposal: offered.proposal,
    decision: { action: "apply", reviewed: true, proposalId: offered.proposal.proposalId, selected: ["mcp:browser"] } },
  { operationId: "context-apply" });
  const applied = await runCommand("context-reduction-apply", { ...value.options, request: applyRequest }, {
    nativeIdentity: value.nativeIdentity, capabilityInventory: inventory,
  });
  assert.equal(applied.status, "applied");
  let setup = JSON.parse(await readFile(value.setupPath, "utf8"));
  assert.deepEqual(setup.contextReduction, applied.setupReceipt);
  assert.match(await readFile(path.join(value.root, ".codex", "config.toml"), "utf8"), /enabled = false/);

  const health = await runCommand("health", { project: value.root, scope: "project", home: value.root });
  assert.equal(health.contextReduction.status, "applied");
  assert.equal(health.contextReduction.canonicalReceipt, "linked");

  const revertRequest = await envelope(value, { expectedReceiptDigest: setup.contextReduction.digest }, { operationId: "context-revert" });
  const reverted = await runCommand("context-reduction-revert", { ...value.options, request: revertRequest }, {
    nativeIdentity: value.nativeIdentity,
  });
  assert.equal(reverted.status, "reverted");
  setup = JSON.parse(await readFile(value.setupPath, "utf8"));
  assert.equal(setup.contextReduction.kind, "context-reduction-revert");
  assert.doesNotMatch(await readFile(path.join(value.root, ".codex", "config.toml"), "utf8"), /enabled = false/);
});

test("health and mutation routes preserve an unknown helper journal with its exact digest", async () => {
  const value = await fixture();
  const journalPath = path.join(value.root, ".agent-team", "helper-transaction.json");
  await mkdir(path.dirname(journalPath), { recursive: true });
  await writeFile(journalPath, "unknown transaction\n");
  const request = await envelope(value, {}, { operationId: "blocked-helper-install" });

  await assert.rejects(runCommand("helpers-install", { ...value.options, request }, { nativeIdentity: value.nativeIdentity }), /recovery_required/);
  const health = await runCommand("health", { project: value.root, scope: "project", home: value.root });
  assert.equal(health.helpers.status, "recovery_required");
  assert.match(health.helpers.pendingTransactionDigest, /^[a-f0-9]{64}$/);
  assert.equal(await readFile(journalPath, "utf8"), "unknown transaction\n");
});

test("a stale canonical setup version rolls back helper postimages without losing setup", async () => {
  const value = await fixture();
  const request = await envelope(value, {}, { operationId: "stale-helper-install" });
  const setup = JSON.parse(await readFile(value.setupPath, "utf8"));
  setup.version = (setup.version ?? 0) + 1;
  await writeFile(value.setupPath, `${JSON.stringify(setup, null, 2)}\n`);
  const before = await readFile(value.setupPath);

  await assert.rejects(runCommand("helpers-install", { ...value.options, request }, { nativeIdentity: value.nativeIdentity }),
    /not committed/);

  assert.deepEqual(await readFile(value.setupPath), before);
  await assert.rejects(readFile(path.join(value.root, "scripts", "agent-team", "with-env.mjs")), { code: "ENOENT" });
  await assert.rejects(readFile(path.join(value.root, ".agent-team", "helpers.json")), { code: "ENOENT" });
  await assert.rejects(readFile(path.join(value.root, ".agent-team", "helper-transaction.json")), { code: "ENOENT" });
});

test("helper recovery reconciles an unknown callback outcome through the canonical writer", async () => {
  const value = await fixture();
  await assert.rejects(installHelpers({ projectRoot: value.root, recordSetupReceipt: async () => {
    throw new Error("commit response lost");
  } }), /canonical_outcome_unknown/);
  const pending = await runCommand("health", { project: value.root, scope: "project", home: value.root });
  assert.equal(pending.helpers.status, "recovery_required");
  assert.equal(pending.helpers.canonicalReceipt, "missing");
  const request = await envelope(value, { expectedJournalDigest: pending.helpers.pendingTransactionDigest },
    { operationId: "recover-helper-install" });

  const recovered = await runCommand("helpers-recover", { ...value.options, request }, { nativeIdentity: value.nativeIdentity });

  assert.equal(recovered.status, "finalized");
  const setup = JSON.parse(await readFile(value.setupPath, "utf8"));
  assert.equal(setup.helpers.kind, "agent-team-helpers");
  await assert.rejects(readFile(path.join(value.root, ".agent-team", "helper-transaction.json")), { code: "ENOENT" });
  const health = await runCommand("health", { project: value.root, scope: "project", home: value.root });
  assert.equal(health.helpers.canonicalReceipt, "linked");
});

test("health never links a helper recovery receipt to a malformed restored receipt", async () => {
  const value = await fixture();
  const target = path.join(value.root, "scripts", "agent-team", "with-env.mjs");
  await assert.rejects(installHelpers({ projectRoot: value.root, recordSetupReceipt: async (receipt) => {
    await writeFile(target, "concurrent edit\n");
    return { status: "not_committed", receiptDigest: receipt.digest };
  } }), /recovery_conflict/);
  await writeFile(target, await readFile(path.resolve(import.meta.dirname, "../assets/helpers/with-env.mjs")));
  const pending = await runCommand("health", { project: value.root, scope: "project", home: value.root });
  const request = await envelope(value, { expectedJournalDigest: pending.helpers.pendingTransactionDigest },
    { operationId: "recover-helper-rollback" });
  const recovered = await runCommand("helpers-recover", { ...value.options, request }, { nativeIdentity: value.nativeIdentity });
  assert.equal(recovered.setupReceipt.kind, "agent-team-helpers-recovery");
  await writeFile(path.join(value.root, ".agent-team", "helpers.json"), "{}\n");

  const health = await runCommand("health", { project: value.root, scope: "project", home: value.root });

  assert.equal(health.helpers.receiptStatus, "invalid");
  assert.equal(health.helpers.canonicalReceipt, "mismatch");
});

test("context apply rejects stale observed visibility before writing configuration", async () => {
  const value = await fixture();
  const offered = await runCommand("context-reduction", value.options, {
    nativeIdentity: value.nativeIdentity, capabilityInventory: inventory,
  });
  const request = await envelope(value, { proposal: offered.proposal,
    decision: { action: "apply", reviewed: true, proposalId: offered.proposal.proposalId, selected: ["mcp:browser"] } },
  { operationId: "stale-context-offer" });
  const changedInventory = structuredClone(inventory);
  changedInventory.capabilities[1].enabled = false;
  const before = await readFile(value.setupPath);

  const result = await runCommand("context-reduction-apply", { ...value.options, request }, {
    nativeIdentity: value.nativeIdentity, capabilityInventory: changedInventory,
  });

  assert.deepEqual(result, { status: "conflict", reason: "proposal_stale", changed: false });
  assert.deepEqual(await readFile(value.setupPath), before);
  await assert.rejects(readFile(path.join(value.root, ".codex", "config.toml")), { code: "ENOENT" });
});

test("a stale canonical setup version rolls back context postimages without losing setup", async () => {
  const value = await fixture();
  const offered = await runCommand("context-reduction", value.options, {
    nativeIdentity: value.nativeIdentity, capabilityInventory: inventory,
  });
  const request = await envelope(value, { proposal: offered.proposal,
    decision: { action: "apply", reviewed: true, proposalId: offered.proposal.proposalId, selected: ["mcp:browser"] } },
  { operationId: "stale-context-apply" });
  const setup = JSON.parse(await readFile(value.setupPath, "utf8"));
  setup.version = (setup.version ?? 0) + 1;
  await writeFile(value.setupPath, `${JSON.stringify(setup, null, 2)}\n`);
  const before = await readFile(value.setupPath);

  await assert.rejects(runCommand("context-reduction-apply", { ...value.options, request }, {
    nativeIdentity: value.nativeIdentity, capabilityInventory: inventory,
  }), /not committed/);

  assert.deepEqual(await readFile(value.setupPath), before);
  await assert.rejects(readFile(path.join(value.root, ".codex", "config.toml")), { code: "ENOENT" });
  await assert.rejects(readFile(path.join(value.root, ".agent-team", "context-reduction.json")), { code: "ENOENT" });
  await assert.rejects(readFile(path.join(value.root, ".agent-team", "context-reduction-transaction.json")), { code: "ENOENT" });
});

test("context recovery reconciles an unknown callback outcome through the canonical writer", async () => {
  const value = await fixture();
  const offered = await runCommand("context-reduction", value.options, {
    nativeIdentity: value.nativeIdentity, capabilityInventory: inventory,
  });
  await assert.rejects(applyContextReduction({ projectRoot: value.root, proposal: offered.proposal,
    decision: { action: "apply", reviewed: true, proposalId: offered.proposal.proposalId, selected: ["mcp:browser"] },
    recordSetupReceipt: async () => { throw new Error("commit response lost"); },
  }), /canonical_outcome_unknown/);
  const pending = await runCommand("health", { project: value.root, scope: "project", home: value.root });
  assert.equal(pending.contextReduction.status, "recovery_required");
  assert.equal(pending.contextReduction.canonicalReceipt, "missing");
  assert.match(pending.contextReduction.pendingTransactionDigest, /^[a-f0-9]{64}$/);
  const request = await envelope(value, { expectedJournalDigest: pending.contextReduction.pendingTransactionDigest },
    { operationId: "recover-context-apply" });

  const recovered = await runCommand("context-reduction-recover", { ...value.options, request }, {
    nativeIdentity: value.nativeIdentity,
  });

  assert.equal(recovered.status, "finalized");
  const setup = JSON.parse(await readFile(value.setupPath, "utf8"));
  assert.equal(setup.contextReduction.kind, "context-reduction");
  await assert.rejects(readFile(path.join(value.root, ".agent-team", "context-reduction-transaction.json")), { code: "ENOENT" });
  const health = await runCommand("health", { project: value.root, scope: "project", home: value.root });
  assert.equal(health.contextReduction.canonicalReceipt, "linked");
});

test("revert recovery binds the restored apply receipt and permits a later safe revert", async () => {
  const value = await fixture();
  const offered = await runCommand("context-reduction", value.options, {
    nativeIdentity: value.nativeIdentity, capabilityInventory: inventory,
  });
  const applyRequest = await envelope(value, { proposal: offered.proposal,
    decision: { action: "apply", reviewed: true, proposalId: offered.proposal.proposalId, selected: ["mcp:browser"] } },
  { operationId: "recovery-fixture-apply" });
  const applied = await runCommand("context-reduction-apply", { ...value.options, request: applyRequest }, {
    nativeIdentity: value.nativeIdentity, capabilityInventory: inventory,
  });
  await assert.rejects(revertContextReduction({ projectRoot: value.root, host: "codex",
    expectedReceiptDigest: applied.setupReceipt.digest,
    recordSetupReceipt: async () => { throw new Error("revert commit response lost"); },
  }), /canonical_outcome_unknown/);
  const pending = await runCommand("health", { project: value.root, scope: "project", home: value.root });
  const staleRequest = await envelope(value, { expectedJournalDigest: pending.contextReduction.pendingTransactionDigest },
    { operationId: "recovery-revert-stale" });
  const setup = JSON.parse(await readFile(value.setupPath, "utf8"));
  setup.version += 1;
  await writeFile(value.setupPath, `${JSON.stringify(setup, null, 2)}\n`);
  await assert.rejects(runCommand("context-reduction-recover", { ...value.options, request: staleRequest }, {
    nativeIdentity: value.nativeIdentity,
  }), /canonical recovery receipt was not committed/);
  const rolledBack = await runCommand("health", { project: value.root, scope: "project", home: value.root });
  const recoverRequest = await envelope(value, { expectedJournalDigest: rolledBack.contextReduction.pendingTransactionDigest },
    { operationId: "recovery-revert-finalize" });
  const recovered = await runCommand("context-reduction-recover", { ...value.options, request: recoverRequest }, {
    nativeIdentity: value.nativeIdentity,
  });
  assert.equal(recovered.status, "rolled_back");
  assert.equal(recovered.setupReceipt.restoredReceiptDigest, applied.setupReceipt.digest);
  const linked = await runCommand("health", { project: value.root, scope: "project", home: value.root });
  assert.equal(linked.contextReduction.canonicalReceipt, "linked");
  assert.equal(linked.contextReduction.receipt.digest, applied.setupReceipt.digest);
  const revertRequest = await envelope(value, { expectedReceiptDigest: applied.setupReceipt.digest },
    { operationId: "recovery-revert-retry" });

  const reverted = await runCommand("context-reduction-revert", { ...value.options, request: revertRequest }, {
    nativeIdentity: value.nativeIdentity,
  });

  assert.equal(reverted.status, "reverted");
  assert.doesNotMatch(await readFile(path.join(value.root, ".codex", "config.toml"), "utf8"), /enabled = false/);
});

test("health reports a canonical context receipt mismatch without changing either receipt", async () => {
  const value = await fixture();
  const offered = await runCommand("context-reduction", value.options, {
    nativeIdentity: value.nativeIdentity, capabilityInventory: inventory,
  });
  const request = await envelope(value, { proposal: offered.proposal,
    decision: { action: "apply", reviewed: true, proposalId: offered.proposal.proposalId, selected: ["mcp:browser"] } },
  { operationId: "context-health-mismatch" });
  await runCommand("context-reduction-apply", { ...value.options, request }, {
    nativeIdentity: value.nativeIdentity, capabilityInventory: inventory,
  });
  const setup = JSON.parse(await readFile(value.setupPath, "utf8"));
  setup.contextReduction = { ...setup.contextReduction, digest: "0".repeat(64) };
  await writeFile(value.setupPath, `${JSON.stringify(setup, null, 2)}\n`);
  const beforeSetup = await readFile(value.setupPath);
  const receiptPath = path.join(value.root, ".agent-team", "context-reduction.json");
  const beforeReceipt = await readFile(receiptPath);

  const health = await runCommand("health", { project: value.root, scope: "project", home: value.root });

  assert.equal(health.contextReduction.status, "applied");
  assert.equal(health.contextReduction.canonicalReceipt, "mismatch");
  assert.deepEqual(await readFile(value.setupPath), beforeSetup);
  assert.deepEqual(await readFile(receiptPath), beforeReceipt);
});

test("context mutation refuses when the canonical applied receipt has lost its local counterpart", async () => {
  const value = await fixture();
  const offered = await runCommand("context-reduction", value.options, {
    nativeIdentity: value.nativeIdentity, capabilityInventory: inventory,
  });
  const applyRequest = await envelope(value, { proposal: offered.proposal,
    decision: { action: "apply", reviewed: true, proposalId: offered.proposal.proposalId, selected: ["mcp:browser"] } },
  { operationId: "missing-local-apply" });
  const applied = await runCommand("context-reduction-apply", { ...value.options, request: applyRequest }, {
    nativeIdentity: value.nativeIdentity, capabilityInventory: inventory,
  });
  await rm(path.join(value.root, ".agent-team", "context-reduction.json"));
  const beforeSetup = await readFile(value.setupPath);
  const beforeConfig = await readFile(path.join(value.root, ".codex", "config.toml"));
  const repeatRequest = await envelope(value, { proposal: offered.proposal,
    decision: { action: "apply", reviewed: true, proposalId: offered.proposal.proposalId, selected: ["mcp:browser"] } },
  { operationId: "missing-local-repeat" });
  const repeat = await runCommand("context-reduction-apply", { ...value.options, request: repeatRequest }, {
    nativeIdentity: value.nativeIdentity, capabilityInventory: inventory,
  });
  assert.deepEqual(repeat, { status: "conflict", reason: "canonical_receipt_mismatch", changed: false });
  const revertRequest = await envelope(value, { expectedReceiptDigest: applied.setupReceipt.digest },
    { operationId: "missing-local-revert" });
  const reverted = await runCommand("context-reduction-revert", { ...value.options, request: revertRequest }, {
    nativeIdentity: value.nativeIdentity,
  });
  assert.deepEqual(reverted, { status: "conflict", reason: "canonical_receipt_mismatch", changed: false });
  assert.deepEqual(await readFile(value.setupPath), beforeSetup);
  assert.deepEqual(await readFile(path.join(value.root, ".codex", "config.toml")), beforeConfig);
});

test("settings wizard and setup start advise only from observed host model inventory", async () => {
  const value = await fixture();
  const unobserved = await runCommand("settings-wizard", value.options, {
    nativeChoices: { models: [{ id: "assumed", available: true, recommended: true, efforts: ["high"] }] },
  });
  assert.deepEqual(unobserved.modelRouting, {
    status: "unknown", host: "codex", reason: "native_model_inventory_unobserved",
  });
  const hostless = await runCommand("settings-wizard", value.options, {
    nativeChoices: { observed: true, models: [{ id: "unbound", available: true }] },
  });
  assert.deepEqual(hostless.modelRouting, {
    status: "unknown", host: "codex", reason: "native_model_inventory_unobserved",
  });
  const nativeChoices = { observed: true, host: "codex", models: [
    { id: "gpt-available", available: true, efforts: ["high"] },
    { id: "gpt-hidden", available: "unknown", efforts: ["high"], recommended: true },
  ] };
  const wizard = await runCommand("settings-wizard", value.options, { nativeChoices });
  assert.deepEqual(wizard.modelRouting, {
    status: "observed", host: "codex", guidance: "explicit_available_model_selection",
    availableModels: ["gpt-available"],
  });
  let interactiveAdvisory;
  const started = await orchestrateSetup({
    project: value.root, host: "codex", scope: "project", dependencies: { action: "inspect" },
    settings: { operationId: "setup-model-advisory" },
  }, {
    nativeIdentity: value.nativeIdentity, nativeChoices,
    interactSettings: async ({ wizard: current }) => {
      interactiveAdvisory = current.modelRouting;
      return { kind: "keep_existing" };
    },
  });
  assert.deepEqual(interactiveAdvisory, wizard.modelRouting);
  assert.deepEqual(started.modelRouting, wizard.modelRouting);
});

test("owner-reported context offer filters canonical dependencies and requires visibility acknowledgement", async () => {
  const value = await fixture();
  const setup = JSON.parse(await readFile(value.setupPath, "utf8"));
  setup.dependencies = { hosts: { codex: { scope: "project", selected: ["tracker"], receipts: [] } } };
  await writeFile(value.setupPath, `${JSON.stringify(setup, null, 2)}\n`);
  const report = { schemaVersion: 1, host: "codex", capabilities: [
    { kind: "mcp", id: "tracker", enabled: true, visible: true, label: "Tracker" },
    { kind: "mcp", id: "browser", enabled: true, visible: true, label: "Browser" },
  ] };
  const offerRequest = await envelope(value, { ownerReportedCapabilities: report }, { operationId: "manual-context-offer" });
  const offered = await runCommand("context-reduction", { ...value.options, request: offerRequest });
  assert.equal(offered.status, "offered_unverified");
  assert.equal(offered.visibility, "unknown");
  assert.deepEqual(offered.proposal.required, ["mcp:tracker"]);
  assert.deepEqual(offered.proposal.candidates.map(({ key, source }) => ({ key, source })), [
    { key: "mcp:browser", source: "owner_reported_unverified" },
  ]);
  const before = await readFile(value.setupPath);
  const unacknowledged = await envelope(value, { proposal: offered.proposal, ownerReportedCapabilities: report,
    decision: { action: "apply", reviewed: true, proposalId: offered.proposal.proposalId, selected: ["mcp:browser"] } },
  { operationId: "manual-context-no-ack" });
  const refused = await runCommand("context-reduction-apply", { ...value.options, request: unacknowledged }, {
    nativeIdentity: value.nativeIdentity,
  });
  assert.deepEqual(refused, { status: "conflict", reason: "visibility_acknowledgement_required", changed: false });
  assert.deepEqual(await readFile(value.setupPath), before);
  const approved = await envelope(value, { proposal: offered.proposal, ownerReportedCapabilities: report,
    decision: { action: "apply", reviewed: true, visibilityAcknowledged: true,
      proposalId: offered.proposal.proposalId, selected: ["mcp:browser"] } },
  { operationId: "manual-context-apply" });
  const applied = await runCommand("context-reduction-apply", { ...value.options, request: approved }, {
    nativeIdentity: value.nativeIdentity,
  });
  assert.equal(applied.status, "applied");
  assert.equal(applied.setupReceipt.changes[0].source, "owner_reported_unverified");
});

test("Claude guidance recommends only Opus variants observed as available", async () => {
  const value = await fixture();
  const nativeChoices = { observed: true, host: "claude-code", models: [
    { id: "claude-sonnet-current", label: "Claude Sonnet", available: true, efforts: ["high"] },
    { id: "claude-opus-current", label: "Claude Opus", available: true, efforts: ["high"] },
    { id: "claude-opus-hidden", label: "Claude Opus Hidden", available: false, efforts: ["high"] },
  ] };

  const wizard = await runCommand("settings-wizard", { ...value.options, host: "claude-code" }, { nativeChoices });

  assert.deepEqual(wizard.modelRouting, {
    status: "observed", host: "claude-code", guidance: "prefer_opus_for_execution",
    availableModels: ["claude-sonnet-current", "claude-opus-current"], recommendedModels: ["claude-opus-current"],
  });
});
