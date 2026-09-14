import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import { mkdtemp, mkdir, readFile, rm, symlink, writeFile } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import test from "node:test";

const temporary = [];
const recordSetupReceipt = async (receipt) => ({ status: "committed", receiptDigest: receipt.digest });
test.afterEach(async () => Promise.all(temporary.splice(0).map((entry) => rm(entry, { recursive: true, force: true }))));

async function fixture(prefix) {
  const root = await mkdtemp(path.join(os.tmpdir(), prefix));
  temporary.push(root);
  return root;
}

const visible = [
  { kind: "mcp", id: "browser", enabled: true, visible: true },
  { kind: "mcp", id: "tracker", enabled: true, visible: true },
  { kind: "plugin", id: "unused@market", enabled: true, visible: true },
  { kind: "plugin", id: "hidden@market", enabled: true, visible: false },
  { kind: "mcp", id: "already-off", enabled: false, visible: true },
];
const approved = (proposal, selected) => ({ action: "apply", reviewed: true, proposalId: proposal.proposalId, selected });

test("candidate proposal is pure and offers only visible enabled capabilities unused by the selected profile", async () => {
  const { proposeContextReduction } = await import("../hooks/lib/context-shrink.mjs");
  const root = await fixture("agent-team-context-pure-");
  await writeFile(path.join(root, "sentinel"), "unchanged\n");

  const proposal = proposeContextReduction({ host: "codex", selectedProfile: { required: ["mcp:tracker"] }, visible });

  assert.deepEqual(proposal.candidates.map(({ key }) => ({ key })), [
    { key: "mcp:browser" },
    { key: "plugin:unused@market" },
  ]);
  assert.equal(await readFile(path.join(root, "sentinel"), "utf8"), "unchanged\n");
});

test("no answer and cancel perform zero filesystem writes", async () => {
  const { applyContextReduction, proposeContextReduction } = await import("../hooks/lib/context-shrink.mjs");
  const projectRoot = await fixture("agent-team-context-cancel-");
  const proposal = proposeContextReduction({ host: "codex", selectedProfile: { required: [] }, visible });
  let receiptCalls = 0;

  for (const action of ["no_answer", "cancel"]) {
    const result = await applyContextReduction({ projectRoot, proposal, decision: { action },
      recordSetupReceipt: async () => { receiptCalls += 1; } });
    assert.equal(result.status, action);
  }
  assert.equal(receiptCalls, 0);
  assert.deepEqual(await import("node:fs/promises").then(({ readdir }) => readdir(projectRoot)), []);
});

test("context inspection is read-only when setup paths are absent", async () => {
  const { inspectContextReduction } = await import("../hooks/lib/context-shrink.mjs");
  const projectRoot = await fixture("agent-team-context-inspect-");
  assert.equal((await inspectContextReduction({ projectRoot })).status, "not_applied");
  assert.deepEqual(await import("node:fs/promises").then(({ readdir }) => readdir(projectRoot)), []);
});

test("Codex applies only a reviewed selection and preserves existing project configuration", async () => {
  const { applyContextReduction, proposeContextReduction } = await import("../hooks/lib/context-shrink.mjs");
  const projectRoot = await fixture("agent-team-context-codex-");
  await mkdir(path.join(projectRoot, ".codex"));
  await writeFile(path.join(projectRoot, ".codex", "config.toml"), "model = \"keep\"\n\n[mcp_servers.tracker]\nenabled = true\n");
  const proposal = proposeContextReduction({ host: "codex", selectedProfile: { required: ["mcp:tracker"] }, visible });

  await assert.rejects(applyContextReduction({ projectRoot, proposal,
    decision: { ...approved(proposal, ["mcp:browser"]), reviewed: false } }), /reviewed approval/);
  const result = await applyContextReduction({ projectRoot, proposal,
    decision: approved(proposal, ["mcp:browser", "plugin:unused@market"]), recordSetupReceipt });
  const config = await readFile(path.join(projectRoot, ".codex", "config.toml"), "utf8");

  assert.equal(result.status, "applied");
  assert.match(config, /model = "keep"/);
  assert.match(config, /\[mcp_servers\."browser"\]\nenabled = false/);
  assert.match(config, /\[plugins\."unused@market"\]\nenabled = false/);
  assert.match(config, /\[mcp_servers\.tracker\]\nenabled = true/);
});

test("Claude stores plugin disable in project settings and MCP disable in an explicitly supplied fixture home", async () => {
  const { applyContextReduction, proposeContextReduction } = await import("../hooks/lib/context-shrink.mjs");
  const projectRoot = await fixture("agent-team-context-claude-project-");
  const home = await fixture("agent-team-context-claude-home-");
  await mkdir(path.join(projectRoot, ".claude"));
  await writeFile(path.join(projectRoot, ".claude", "settings.json"), JSON.stringify({ custom: true,
    enabledPlugins: { "keep@market": true } }, null, 2));
  await writeFile(path.join(home, ".claude.json"), JSON.stringify({ custom: "keep", projects: {
    [projectRoot]: { custom: true, disabledMcpServers: ["previous"] },
  } }, null, 2));
  const proposal = proposeContextReduction({ host: "claude-code", selectedProfile: { required: ["mcp:tracker"] }, visible });

  const result = await applyContextReduction({ projectRoot, home, proposal,
    decision: approved(proposal, ["mcp:browser", "plugin:unused@market"]), recordSetupReceipt });
  const settings = JSON.parse(await readFile(path.join(projectRoot, ".claude", "settings.json"), "utf8"));
  const user = JSON.parse(await readFile(path.join(home, ".claude.json"), "utf8"));

  assert.equal(result.status, "applied");
  assert.deepEqual(settings, { custom: true, enabledPlugins: { "keep@market": true, "unused@market": false } });
  assert.deepEqual(user.projects[projectRoot], { custom: true, disabledMcpServers: ["previous", "browser"] });
  assert.equal(user.custom, "keep");
});

test("revert restores owned values while preserving unrelated subsequent edits", async () => {
  const { applyContextReduction, proposeContextReduction, revertContextReduction } = await import("../hooks/lib/context-shrink.mjs");
  const projectRoot = await fixture("agent-team-context-revert-");
  const home = await fixture("agent-team-context-revert-home-");
  const proposal = proposeContextReduction({ host: "claude-code", selectedProfile: { required: [] }, visible });
  const applied = await applyContextReduction({ projectRoot, home, proposal,
    decision: approved(proposal, ["mcp:browser", "plugin:unused@market"]), recordSetupReceipt });
  const settingsPath = path.join(projectRoot, ".claude", "settings.json");
  const settings = JSON.parse(await readFile(settingsPath, "utf8"));
  settings.unrelated = "later";
  await writeFile(settingsPath, `${JSON.stringify(settings, null, 2)}\n`);

  const result = await revertContextReduction({ projectRoot, home, host: "claude-code",
    expectedReceiptDigest: applied.setupReceipt.digest, recordSetupReceipt });
  const reverted = JSON.parse(await readFile(settingsPath, "utf8"));
  const user = JSON.parse(await readFile(path.join(home, ".claude.json"), "utf8"));

  assert.equal(result.status, "reverted");
  assert.deepEqual(reverted, { enabledPlugins: {}, unrelated: "later" });
  assert.deepEqual(user.projects[projectRoot].disabledMcpServers, []);
});

test("revert detects an owned-value conflict before changing any configuration", async () => {
  const { applyContextReduction, proposeContextReduction, revertContextReduction } = await import("../hooks/lib/context-shrink.mjs");
  const projectRoot = await fixture("agent-team-context-conflict-");
  const proposal = proposeContextReduction({ host: "codex", selectedProfile: { required: [] }, visible });
  const applied = await applyContextReduction({ projectRoot, proposal,
    decision: approved(proposal, ["mcp:browser", "plugin:unused@market"]), recordSetupReceipt });
  const configPath = path.join(projectRoot, ".codex", "config.toml");
  const changed = (await readFile(configPath, "utf8")).replace("enabled = false", "enabled = true");
  await writeFile(configPath, changed);

  const result = await revertContextReduction({ projectRoot, host: "codex",
    expectedReceiptDigest: applied.setupReceipt.digest, recordSetupReceipt });

  assert.equal(result.status, "conflict");
  assert.equal(result.conflicts[0].key, "mcp:browser");
  assert.equal(await readFile(configPath, "utf8"), changed);
});

test("context apply requires canonical receipt recording before a state change", async () => {
  const { applyContextReduction, proposeContextReduction } = await import("../hooks/lib/context-shrink.mjs");
  const projectRoot = await fixture("agent-team-context-canonical-receipt-");
  const proposal = proposeContextReduction({ host: "codex", selectedProfile: { required: [] }, visible });

  await assert.rejects(applyContextReduction({ projectRoot, proposal,
    decision: approved(proposal, ["mcp:browser"]) }), /recordSetupReceipt is required/);
  await assert.rejects(readFile(path.join(projectRoot, ".codex", "config.toml")), { code: "ENOENT" });
});

test("context apply rejects a mutated proposal with its original proposal identity", async () => {
  const { applyContextReduction, proposeContextReduction } = await import("../hooks/lib/context-shrink.mjs");
  const projectRoot = await fixture("agent-team-context-proposal-");
  const proposal = proposeContextReduction({ host: "codex", selectedProfile: { required: [] }, visible });
  proposal.candidates[0] = { ...proposal.candidates[0], kind: "plugin", id: "fabricated" };

  await assert.rejects(applyContextReduction({ projectRoot, proposal,
    decision: approved(proposal, ["mcp:browser"]), recordSetupReceipt }), /proposal.*invalid|identity/i);
  await assert.rejects(readFile(path.join(projectRoot, ".codex", "config.toml")), { code: "ENOENT" });
});

test("context approval is bound to the exact offered proposal identity", async () => {
  const { applyContextReduction, proposeContextReduction } = await import("../hooks/lib/context-shrink.mjs");
  const projectRoot = await fixture("agent-team-context-approval-binding-");
  const offered = proposeContextReduction({ host: "codex", selectedProfile: { required: [] }, visible });
  const substituted = proposeContextReduction({ host: "codex", selectedProfile: { required: [] }, visible: [
    { kind: "plugin", id: "fabricated", enabled: true, visible: true },
  ] });

  await assert.rejects(applyContextReduction({ projectRoot, proposal: substituted,
    decision: { ...approved(substituted, ["plugin:fabricated"]), proposalId: offered.proposalId }, recordSetupReceipt }),
  /proposal identity/);
  assert.deepEqual(await import("node:fs/promises").then(({ readdir }) => readdir(projectRoot)), []);
});

test("context apply rejects symlink ancestors without writing outside the project", async () => {
  const { applyContextReduction, proposeContextReduction } = await import("../hooks/lib/context-shrink.mjs");
  const projectRoot = await fixture("agent-team-context-link-");
  const outside = await fixture("agent-team-context-outside-");
  await symlink(outside, path.join(projectRoot, ".codex"), "dir");
  const proposal = proposeContextReduction({ host: "codex", selectedProfile: { required: [] }, visible });

  await assert.rejects(applyContextReduction({ projectRoot, proposal,
    decision: approved(proposal, ["mcp:browser"]), recordSetupReceipt }), /unsafe.*ancestor|symlink/i);
  await assert.rejects(readFile(path.join(outside, "config.toml")), { code: "ENOENT" });
});

test("context apply preserves a preexisting pending journal", async () => {
  const { applyContextReduction, inspectContextReduction, proposeContextReduction } = await import("../hooks/lib/context-shrink.mjs");
  const projectRoot = await fixture("agent-team-context-pending-");
  await mkdir(path.join(projectRoot, ".agent-team"));
  const journal = Buffer.from("pending transaction\n");
  await writeFile(path.join(projectRoot, ".agent-team", "context-reduction-transaction.json"), journal);
  const proposal = proposeContextReduction({ host: "codex", selectedProfile: { required: [] }, visible });

  await assert.rejects(applyContextReduction({ projectRoot, proposal,
    decision: approved(proposal, ["mcp:browser"]), recordSetupReceipt }), /recovery_required/);
  assert.deepEqual(await readFile(path.join(projectRoot, ".agent-team", "context-reduction-transaction.json")), journal);
  assert.equal((await inspectContextReduction({ projectRoot })).pendingTransactionDigest,
    createHash("sha256").update(journal).digest("hex"));
});

test("context inspection rejects a malformed local receipt", async () => {
  const { inspectContextReduction } = await import("../hooks/lib/context-shrink.mjs");
  const projectRoot = await fixture("agent-team-context-invalid-receipt-");
  await mkdir(path.join(projectRoot, ".agent-team"));
  await writeFile(path.join(projectRoot, ".agent-team", "context-reduction.json"), "{}\n");
  const result = await inspectContextReduction({ projectRoot });
  assert.deepEqual({ status: result.status, reason: result.reason }, { status: "conflict", reason: "receipt_invalid" });
});

test("revert rejects a tampered receipt path and digest without touching its target", async () => {
  const { applyContextReduction, proposeContextReduction, revertContextReduction } = await import("../hooks/lib/context-shrink.mjs");
  const projectRoot = await fixture("agent-team-context-receipt-tamper-");
  const outside = path.join(await fixture("agent-team-context-receipt-outside-"), "config.toml");
  const proposal = proposeContextReduction({ host: "codex", selectedProfile: { required: [] }, visible });
  const applied = await applyContextReduction({ projectRoot, proposal,
    decision: approved(proposal, ["mcp:browser"]), recordSetupReceipt });
  const receiptPath = path.join(projectRoot, ".agent-team", "context-reduction.json");
  const receipt = JSON.parse(await readFile(receiptPath, "utf8"));
  receipt.changes[0].configPath = outside;
  await writeFile(outside, '[mcp_servers."browser"]\nenabled = false\n');
  await writeFile(receiptPath, `${JSON.stringify(receipt, null, 2)}\n`);

  const result = await revertContextReduction({ projectRoot, host: "codex",
    expectedReceiptDigest: applied.setupReceipt.digest, recordSetupReceipt });

  assert.equal(result.status, "conflict");
  assert.match(result.reason, /receipt/);
  assert.equal(await readFile(outside, "utf8"), '[mcp_servers."browser"]\nenabled = false\n');
});

test("context rollback preserves a concurrent edit and leaves recovery evidence", async () => {
  const { applyContextReduction, inspectContextReduction, proposeContextReduction } = await import("../hooks/lib/context-shrink.mjs");
  const projectRoot = await fixture("agent-team-context-cas-");
  const configPath = path.join(projectRoot, ".codex", "config.toml");
  const proposal = proposeContextReduction({ host: "codex", selectedProfile: { required: [] }, visible });

  await assert.rejects(applyContextReduction({ projectRoot, proposal,
    decision: approved(proposal, ["mcp:browser"]), recordSetupReceipt: async (receipt) => {
      await writeFile(configPath, "concurrent operator edit\n");
      return { status: "not_committed", receiptDigest: receipt.digest };
    } }), /recovery_conflict/);

  assert.equal(await readFile(configPath, "utf8"), "concurrent operator edit\n");
  assert.equal((await inspectContextReduction({ projectRoot })).status, "recovery_required");
});

test("context recovery requires the exact journal and rolls back verified postimages", async () => {
  const { applyContextReduction, proposeContextReduction, recoverContextReductionTransaction } = await import("../hooks/lib/context-shrink.mjs");
  const projectRoot = await fixture("agent-team-context-recover-");
  const configPath = path.join(projectRoot, ".codex", "config.toml");
  const proposal = proposeContextReduction({ host: "codex", selectedProfile: { required: [] }, visible });
  await assert.rejects(applyContextReduction({ projectRoot, proposal,
    decision: approved(proposal, ["mcp:browser"]), recordSetupReceipt: async (receipt) => {
      await writeFile(configPath, "concurrent edit\n");
      return { status: "not_committed", receiptDigest: receipt.digest };
    } }), /recovery_conflict/);
  await writeFile(configPath, '[mcp_servers."browser"]\nenabled = false\n');
  const journalPath = path.join(projectRoot, ".agent-team", "context-reduction-transaction.json");
  const journal = await readFile(journalPath);
  const journalDigest = createHash("sha256").update(journal).digest("hex");

  await assert.rejects(recoverContextReductionTransaction({ projectRoot, host: "codex",
    expectedJournalDigest: "0".repeat(64), recordSetupReceipt }), /journal.*digest/i);
  const receipts = [];
  await assert.rejects(recoverContextReductionTransaction({ projectRoot, host: "codex",
    expectedJournalDigest: journalDigest,
    recordSetupReceipt: async (receipt) => { receipts.push(receipt); throw new Error("commit response lost"); } }),
  /canonical_outcome_unknown/);
  const retryJournal = await readFile(journalPath);
  const recovered = await recoverContextReductionTransaction({ projectRoot, host: "codex",
    expectedJournalDigest: createHash("sha256").update(retryJournal).digest("hex"),
    recordSetupReceipt: async (receipt) => { receipts.push(receipt); return { status: "committed", receiptDigest: receipt.digest }; } });
  assert.equal(recovered.status, "rolled_back");
  assert.equal(receipts[0].kind, "context-reduction-recovery");
  assert.equal(receipts[1].digest, receipts[0].digest);
  await assert.rejects(readFile(configPath), { code: "ENOENT" });
  await assert.rejects(readFile(journalPath), { code: "ENOENT" });
});

test("context recovery finalizes a committed rollback against restored postimages", async () => {
  const { applyContextReduction, proposeContextReduction, recoverContextReductionTransaction } = await import("../hooks/lib/context-shrink.mjs");
  const projectRoot = await fixture("agent-team-context-recovery-finalize-");
  const configPath = path.join(projectRoot, ".codex", "config.toml");
  const journalPath = path.join(projectRoot, ".agent-team", "context-reduction-transaction.json");
  const proposal = proposeContextReduction({ host: "codex", selectedProfile: { required: [] }, visible });
  await assert.rejects(applyContextReduction({ projectRoot, proposal,
    decision: approved(proposal, ["mcp:browser"]), recordSetupReceipt: async (receipt) => {
      await writeFile(configPath, "concurrent edit\n");
      return { status: "not_committed", receiptDigest: receipt.digest };
    } }), /recovery_conflict/);
  await writeFile(configPath, '[mcp_servers."browser"]\nenabled = false\n');
  const firstJournal = await readFile(journalPath);
  await assert.rejects(recoverContextReductionTransaction({ projectRoot, host: "codex",
    expectedJournalDigest: createHash("sha256").update(firstJournal).digest("hex"),
    recordSetupReceipt: async () => { throw new Error("recovery commit response lost"); },
  }), /canonical_outcome_unknown/);
  const persisted = JSON.parse(await readFile(journalPath, "utf8"));
  assert.equal(persisted.status, "recovery_local_applied");
  persisted.status = "recovery_canonical_committed";
  await writeFile(journalPath, `${JSON.stringify(persisted, null, 2)}\n`);
  const committedJournal = await readFile(journalPath);

  const finalized = await recoverContextReductionTransaction({ projectRoot, host: "codex",
    expectedJournalDigest: createHash("sha256").update(committedJournal).digest("hex"),
    recordSetupReceipt: async () => { throw new Error("canonical callback must not repeat"); },
  });

  assert.equal(finalized.status, "finalized");
  assert.equal(finalized.setupReceipt.kind, "context-reduction-recovery");
  await assert.rejects(readFile(configPath), { code: "ENOENT" });
  await assert.rejects(readFile(journalPath), { code: "ENOENT" });
});

test("Codex rejects ambiguous TOML and exactly restores a supported enabled assignment", async () => {
  const { applyContextReduction, proposeContextReduction, revertContextReduction } = await import("../hooks/lib/context-shrink.mjs");
  const projectRoot = await fixture("agent-team-context-toml-");
  await mkdir(path.join(projectRoot, ".codex"));
  const configPath = path.join(projectRoot, ".codex", "config.toml");
  const ambiguous = '[mcp_servers."browser"]\nenabled = true\nenabled = false\n';
  await writeFile(configPath, ambiguous);
  const proposal = proposeContextReduction({ host: "codex", selectedProfile: { required: [] }, visible });
  await assert.rejects(applyContextReduction({ projectRoot, proposal,
    decision: approved(proposal, ["mcp:browser"]), recordSetupReceipt }), /ambiguous.*TOML/i);
  assert.equal(await readFile(configPath, "utf8"), ambiguous);

  const supported = '[mcp_servers."browser"]\n  enabled = true # operator comment\n';
  await writeFile(configPath, supported);
  const applied = await applyContextReduction({ projectRoot, proposal,
    decision: approved(proposal, ["mcp:browser"]), recordSetupReceipt });
  const reverted = await revertContextReduction({ projectRoot, host: "codex",
    expectedReceiptDigest: applied.setupReceipt.digest, recordSetupReceipt });
  assert.equal(reverted.status, "reverted");
  assert.equal(await readFile(configPath, "utf8"), supported);
});

test("context apply rolls back only when canonical recording proves no commit", async () => {
  const { applyContextReduction, proposeContextReduction } = await import("../hooks/lib/context-shrink.mjs");
  const projectRoot = await fixture("agent-team-context-receipt-failure-");
  const proposal = proposeContextReduction({ host: "codex", selectedProfile: { required: [] }, visible });

  await assert.rejects(applyContextReduction({ projectRoot, proposal,
    decision: approved(proposal, ["mcp:browser"]),
    recordSetupReceipt: async (receipt) => ({ status: "not_committed", receiptDigest: receipt.digest }) }), /not committed/);

  await assert.rejects(readFile(path.join(projectRoot, ".codex", "config.toml")), { code: "ENOENT" });
  await assert.rejects(readFile(path.join(projectRoot, ".agent-team", "context-reduction.json")), { code: "ENOENT" });
  await assert.rejects(readFile(path.join(projectRoot, ".agent-team", "context-reduction-transaction.json")), { code: "ENOENT" });
});

test("context apply preserves postimages and journal when canonical outcome is unknown", async () => {
  const { applyContextReduction, inspectContextReduction, proposeContextReduction } = await import("../hooks/lib/context-shrink.mjs");
  const projectRoot = await fixture("agent-team-context-unknown-commit-");
  const proposal = proposeContextReduction({ host: "codex", selectedProfile: { required: [] }, visible });
  await assert.rejects(applyContextReduction({ projectRoot, proposal,
    decision: approved(proposal, ["mcp:browser"]), recordSetupReceipt: async () => { throw new Error("commit response lost"); } }),
  /canonical_outcome_unknown/);
  assert.match(await readFile(path.join(projectRoot, ".codex", "config.toml"), "utf8"), /enabled = false/);
  const health = await inspectContextReduction({ projectRoot, host: "codex" });
  assert.equal(health.status, "recovery_required");
  assert.match(health.pendingTransactionDigest, /^[a-f0-9]{64}$/);
});

test("context apply never rolls back after a proven commit when finalization conflicts", async () => {
  const { applyContextReduction, proposeContextReduction } = await import("../hooks/lib/context-shrink.mjs");
  const projectRoot = await fixture("agent-team-context-finalize-");
  const journalPath = path.join(projectRoot, ".agent-team", "context-reduction-transaction.json");
  const concurrent = Buffer.from("concurrent journal edit\n");
  const proposal = proposeContextReduction({ host: "codex", selectedProfile: { required: [] }, visible });

  await assert.rejects(applyContextReduction({ projectRoot, proposal,
    decision: approved(proposal, ["mcp:browser"]), recordSetupReceipt: async (receipt) => {
      await writeFile(journalPath, concurrent);
      return { status: "committed", receiptDigest: receipt.digest };
    } }), /recovery_required/);

  assert.match(await readFile(path.join(projectRoot, ".codex", "config.toml"), "utf8"), /enabled = false/);
  assert.deepEqual(await readFile(journalPath), concurrent);
});
