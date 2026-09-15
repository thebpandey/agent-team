import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import { execFile } from "node:child_process";
import { mkdtemp, mkdir, readFile, rm, stat, symlink, writeFile } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import { promisify } from "node:util";
import test from "node:test";
import { policyFixture } from "./hook-test-helpers.mjs";

const exec = promisify(execFile);
const sourceRoot = path.resolve(import.meta.dirname, "../assets/helpers");
const temporary = [];
const recordSetupReceipt = async (receipt) => ({ status: "committed", receiptDigest: receipt.digest });

test.afterEach(async () => Promise.all(temporary.splice(0).map((entry) => rm(entry, { recursive: true, force: true }))));

async function fixture(prefix) {
  const root = await mkdtemp(path.join(os.tmpdir(), prefix));
  temporary.push(root);
  return root;
}

test("every shipped helper passes its executable self-check", async () => {
  for (const name of ["completion-evidence.mjs", "gate-rebind.mjs", "with-env.mjs"]) {
    const result = await exec(process.execPath, [path.join(sourceRoot, name), "--self-check"]);
    assert.match(result.stdout, /^self-check ok\n$/);
    assert.equal(result.stderr, "");
  }
});

test("completion evidence preserves failed and unrun results and rejects names without results", async () => {
  const { buildCompletionEvidence } = await import("../assets/helpers/completion-evidence.mjs");
  const input = {
    taskId: "TASK-4",
    revision: "a".repeat(40),
    requirementsReconciled: true,
    review: { status: "passed", revision: "a".repeat(40), taskId: "TASK-4", reviewer: "independent-reviewer" },
    checks: [
      { name: "unit", status: "failed", revision: "a".repeat(40), taskId: "TASK-4" },
      { name: "browser", status: "unrun", revision: "a".repeat(40), taskId: "TASK-4" },
    ],
  };

  const evidence = buildCompletionEvidence(input);

  assert.equal(evidence.status, "failed");
  assert.deepEqual(evidence.checks.map(({ status }) => status), ["failed", "unrun"]);
  assert.throws(() => buildCompletionEvidence({ ...input, checks: ["unit"] }), /result object/);
});

test("completion evidence CLI requires an explicit task revision", async () => {
  const root = await fixture("agent-team-evidence-revision-");
  const inputPath = path.join(root, "results.json");
  const outputPath = path.join(root, "evidence.json");
  await writeFile(inputPath, JSON.stringify({ taskId: "TASK-4", requirementsReconciled: true,
    review: { status: "passed", taskId: "TASK-4", reviewer: "reviewer" },
    checks: [{ name: "unit", status: "passed", taskId: "TASK-4" }] }));

  await assert.rejects(exec(process.execPath, [path.join(sourceRoot, "completion-evidence.mjs"), inputPath, outputPath]),
    (error) => error.code === 2 && /revision is required/.test(error.stderr));
  await assert.rejects(readFile(outputPath), { code: "ENOENT" });
});

test("dotenv values stay literal in the child and never enter the parent environment", async () => {
  const root = await fixture("agent team helper env ");
  const child = path.join(root, "show-env.mjs");
  const variable = `AGENT_TEAM_CHILD_${process.pid}`;
  const literal = "$(printf injected);`printf also-injected`";
  await writeFile(path.join(root, ".env"), `${variable}='${literal}'\n`);
  await writeFile(child, `process.stdout.write(process.env.${variable} ?? "missing")\n`);

  const result = await exec(process.execPath, [path.join(sourceRoot, "with-env.mjs"), root, process.execPath, child]);

  assert.equal(result.stdout, literal);
  assert.equal(process.env[variable], undefined);
});

test("gate rebinder invokes argv without a shell and prints a safely quoted exact command", async () => {
  const root = await fixture("agent team gate path ");
  const cli = path.join(root, "fake cli.mjs");
  const evidence = path.join(root, "review evidence.json");
  const request = path.join(root, "gate request.json");
  await writeFile(cli, `process.stdout.write(JSON.stringify({stateVersion:3,revision:"${"a".repeat(40)}",freshness:{fingerprint:"${"f".repeat(64)}"}}))\n`);
  const revision = "a".repeat(40);
  await writeFile(evidence, JSON.stringify({ status: "passed", revision, taskIds: ["TASK-4"], requirementsReconciled: true,
    review: { status: "passed", revision, taskId: "TASK-4", reviewer: "reviewer" },
    checks: [{ name: "unit", status: "passed", revision, taskId: "TASK-4" }] }));

  const result = await exec(process.execPath, [path.join(sourceRoot, "gate-rebind.mjs"),
    "TASK-4", evidence, request, "--project", root, "--cli", cli, "--session", "session-1"]);
  const saved = JSON.parse(await readFile(request, "utf8"));

  assert.equal(saved.request.taskIds[0], "TASK-4");
  assert.equal(saved.request.evidencePath, evidence);
  assert.match(result.stdout, /next, exactly and unchained:/);
  assert.match(result.stdout, /'[^']*\/fake cli\.mjs'/);
  assert.match(result.stdout, /'[^']*\/gate request\.json'/);
});

test("gate rebinder rejects evidence that is malformed, failed, or bound to another revision", async () => {
  const root = await fixture("agent-team-gate-invalid-");
  const cli = path.join(root, "cli.mjs");
  const evidence = path.join(root, "evidence.json");
  const request = path.join(root, "request.json");
  await writeFile(cli, `process.stdout.write(JSON.stringify({stateVersion:3,revision:"${"a".repeat(40)}",freshness:{fingerprint:"${"f".repeat(64)}"}}))\n`);
  await writeFile(evidence, JSON.stringify({ status: "failed", revision: "b".repeat(40), taskIds: ["TASK-4"], checks: [] }));

  await assert.rejects(exec(process.execPath, [path.join(sourceRoot, "gate-rebind.mjs"), "TASK-4", evidence, request,
    "--project", root, "--cli", cli, "--session", "session-1"]), (error) => error.code === 2 && /evidence/.test(error.stderr));
  await assert.rejects(readFile(request), { code: "ENOENT" });
});

test("gate rebinder consumes the real status CLI response shape", async () => {
  const root = await fixture("agent-team-gate-real-status-");
  const value = await policyFixture(root, { qualifiedOwnership: true });
  await writeFile(path.join(root, ".agent-team", "state.json"), JSON.stringify({ ...value.state, stateVersion: 0 }, null, 2));
  const cli = path.resolve(import.meta.dirname, "../hooks/agent-team-cli.mjs");
  const evidence = path.join(root, "evidence.json");
  const request = path.join(root, "request.json");
  await writeFile(evidence, JSON.stringify({ status: "passed", revision: value.revision, taskIds: ["AT-001"], requirementsReconciled: true,
    review: { status: "passed", revision: value.revision, taskId: "AT-001", reviewer: "reviewer" },
    checks: [{ name: "unit", status: "passed", revision: value.revision, taskId: "AT-001" }] }));

  await exec(process.execPath, [path.join(sourceRoot, "gate-rebind.mjs"), "AT-001", evidence, request,
    "--project", root, "--cli", cli, "--session", "session-1"]);
  const saved = JSON.parse(await readFile(request, "utf8"));
  assert.equal(saved.request.expectedRevision, value.revision);
});

test("helper installation is idempotent and health reports customized-file drift without overwriting it", async () => {
  const { installHelpers, inspectHelpers } = await import("../hooks/lib/helpers.mjs");
  const projectRoot = await fixture("agent-team-helper-install-");

  const first = await installHelpers({ projectRoot, sourceRoot, recordSetupReceipt });
  const second = await installHelpers({ projectRoot, sourceRoot, recordSetupReceipt });
  const target = path.join(projectRoot, "scripts", "agent-team", "completion-evidence.mjs");
  const receipt = JSON.parse(await readFile(path.join(projectRoot, ".agent-team", "helpers.json"), "utf8"));

  assert.equal(first.status, "installed");
  assert.equal(second.status, "current");
  assert.equal(second.changed, false);
  assert.equal(receipt.files["completion-evidence.mjs"].sha256,
    createHash("sha256").update(await readFile(target)).digest("hex"));
  assert.equal((await stat(target)).mode & 0o111, 0o111);

  await writeFile(target, "operator customization\n");
  const update = await installHelpers({ projectRoot, sourceRoot, recordSetupReceipt });
  const health = await inspectHelpers({ projectRoot, sourceRoot });

  assert.equal(update.status, "conflict");
  assert.equal(update.conflicts[0].name, "completion-evidence.mjs");
  assert.equal(await readFile(target, "utf8"), "operator customization\n");
  assert.equal(health.status, "drift");
  assert.equal(health.files.find(({ name }) => name === "completion-evidence.mjs").status, "customized");
});

test("helper health inspection is read-only when installation paths are absent", async () => {
  const { inspectHelpers } = await import("../hooks/lib/helpers.mjs");
  const projectRoot = await fixture("agent-team-helper-inspect-");
  assert.equal((await inspectHelpers({ projectRoot, sourceRoot })).status, "drift");
  assert.deepEqual(await import("node:fs/promises").then(({ readdir }) => readdir(projectRoot)), []);
});

test("malformed helper receipts remain distinguishable from missing receipts", async () => {
  const { inspectHelpers } = await import("../hooks/lib/helpers.mjs");
  const projectRoot = await fixture("agent-team-helper-invalid-receipt-");
  await mkdir(path.join(projectRoot, ".agent-team"));
  await writeFile(path.join(projectRoot, ".agent-team", "helpers.json"), "{}\n");

  const inspection = await inspectHelpers({ projectRoot, sourceRoot });

  assert.equal(inspection.receipt, null);
  assert.equal(inspection.receiptStatus, "invalid");
});

test("a preexisting unowned helper is preserved while missing helpers are installed", async () => {
  const { installHelpers } = await import("../hooks/lib/helpers.mjs");
  const projectRoot = await fixture("agent-team-helper-custom-");
  const targetRoot = path.join(projectRoot, "scripts", "agent-team");
  await mkdir(targetRoot, { recursive: true });
  await writeFile(path.join(targetRoot, "with-env.mjs"), "custom helper\n");

  const result = await installHelpers({ projectRoot, sourceRoot, recordSetupReceipt });

  assert.equal(result.status, "conflict");
  assert.equal(await readFile(path.join(targetRoot, "with-env.mjs"), "utf8"), "custom helper\n");
  assert.match(await readFile(path.join(targetRoot, "gate-rebind.mjs"), "utf8"), /self-check/);
});

test("helper installation requires canonical receipt recording before a state change", async () => {
  const { installHelpers } = await import("../hooks/lib/helpers.mjs");
  const projectRoot = await fixture("agent-team-helper-canonical-receipt-");

  await assert.rejects(installHelpers({ projectRoot, sourceRoot }), /recordSetupReceipt is required/);
  await assert.rejects(readFile(path.join(projectRoot, "scripts", "agent-team", "with-env.mjs")), { code: "ENOENT" });
  assert.deepEqual(await import("node:fs/promises").then(({ readdir }) => readdir(projectRoot)), []);
});

test("helper installation rejects symlink ancestors without writing outside the project", async () => {
  const { installHelpers } = await import("../hooks/lib/helpers.mjs");
  const projectRoot = await fixture("agent-team-helper-link-");
  const outside = await fixture("agent-team-helper-outside-");
  await symlink(outside, path.join(projectRoot, "scripts"), "dir");

  await assert.rejects(installHelpers({ projectRoot, sourceRoot, recordSetupReceipt }), /unsafe.*ancestor|symlink/i);
  await assert.rejects(readFile(path.join(outside, "agent-team", "with-env.mjs")), { code: "ENOENT" });
});

test("helper installation preserves a preexisting pending journal", async () => {
  const { inspectHelpers, installHelpers } = await import("../hooks/lib/helpers.mjs");
  const projectRoot = await fixture("agent-team-helper-pending-");
  await mkdir(path.join(projectRoot, ".agent-team"));
  const journal = Buffer.from("pending transaction\n");
  await writeFile(path.join(projectRoot, ".agent-team", "helper-transaction.json"), journal);

  await assert.rejects(installHelpers({ projectRoot, sourceRoot, recordSetupReceipt }), /recovery_required/);
  assert.deepEqual(await readFile(path.join(projectRoot, ".agent-team", "helper-transaction.json")), journal);
  assert.equal((await inspectHelpers({ projectRoot, sourceRoot })).pendingTransactionDigest,
    createHash("sha256").update(journal).digest("hex"));
});

test("helper rollback preserves a concurrent operator edit and leaves recovery evidence", async () => {
  const { installHelpers, inspectHelpers } = await import("../hooks/lib/helpers.mjs");
  const projectRoot = await fixture("agent-team-helper-cas-");
  const target = path.join(projectRoot, "scripts", "agent-team", "with-env.mjs");

  await assert.rejects(installHelpers({ projectRoot, sourceRoot, recordSetupReceipt: async (receipt) => {
    await writeFile(target, "concurrent operator edit\n");
    return { status: "not_committed", receiptDigest: receipt.digest };
  } }), /recovery_conflict/);

  assert.equal(await readFile(target, "utf8"), "concurrent operator edit\n");
  assert.equal((await inspectHelpers({ projectRoot, sourceRoot })).status, "recovery_required");
});

test("helper recovery requires the exact journal and rolls back only verified postimages", async () => {
  const { installHelpers, recoverHelperTransaction } = await import("../hooks/lib/helpers.mjs");
  const projectRoot = await fixture("agent-team-helper-recover-");
  const target = path.join(projectRoot, "scripts", "agent-team", "with-env.mjs");
  await assert.rejects(installHelpers({ projectRoot, sourceRoot, recordSetupReceipt: async (receipt) => {
    await writeFile(target, "concurrent edit\n");
    return { status: "not_committed", receiptDigest: receipt.digest };
  } }), /recovery_conflict/);
  await writeFile(target, await readFile(path.join(sourceRoot, "with-env.mjs")));
  const journalPath = path.join(projectRoot, ".agent-team", "helper-transaction.json");
  const journal = await readFile(journalPath);
  const journalDigest = createHash("sha256").update(journal).digest("hex");

  await assert.rejects(recoverHelperTransaction({ projectRoot, expectedJournalDigest: "0".repeat(64),
    recordSetupReceipt }), /journal.*digest/i);
  const receipts = [];
  await assert.rejects(recoverHelperTransaction({ projectRoot, expectedJournalDigest: journalDigest,
    recordSetupReceipt: async (receipt) => { receipts.push(receipt); throw new Error("commit response lost"); } }),
  /canonical_outcome_unknown/);
  const retryJournal = await readFile(journalPath);
  const recovered = await recoverHelperTransaction({ projectRoot,
    expectedJournalDigest: createHash("sha256").update(retryJournal).digest("hex"),
    recordSetupReceipt: async (receipt) => { receipts.push(receipt); return { status: "committed", receiptDigest: receipt.digest }; } });
  assert.equal(recovered.status, "rolled_back");
  assert.equal(receipts[0].kind, "agent-team-helpers-recovery");
  assert.equal(receipts[1].digest, receipts[0].digest);
  await assert.rejects(readFile(target), { code: "ENOENT" });
  await assert.rejects(readFile(journalPath), { code: "ENOENT" });
});

test("helper recovery finalizes a committed rollback against restored postimages", async () => {
  const { installHelpers, recoverHelperTransaction } = await import("../hooks/lib/helpers.mjs");
  const projectRoot = await fixture("agent-team-helper-recovery-finalize-");
  const target = path.join(projectRoot, "scripts", "agent-team", "with-env.mjs");
  const journalPath = path.join(projectRoot, ".agent-team", "helper-transaction.json");
  await assert.rejects(installHelpers({ projectRoot, sourceRoot, recordSetupReceipt: async (receipt) => {
    await writeFile(target, "concurrent edit\n");
    return { status: "not_committed", receiptDigest: receipt.digest };
  } }), /recovery_conflict/);
  await writeFile(target, await readFile(path.join(sourceRoot, "with-env.mjs")));
  const firstJournal = await readFile(journalPath);
  await assert.rejects(recoverHelperTransaction({ projectRoot,
    expectedJournalDigest: createHash("sha256").update(firstJournal).digest("hex"),
    recordSetupReceipt: async () => { throw new Error("recovery commit response lost"); },
  }), /canonical_outcome_unknown/);
  const persisted = JSON.parse(await readFile(journalPath, "utf8"));
  assert.equal(persisted.status, "recovery_local_applied");
  persisted.status = "recovery_canonical_committed";
  await writeFile(journalPath, `${JSON.stringify(persisted, null, 2)}\n`);
  const committedJournal = await readFile(journalPath);

  const finalized = await recoverHelperTransaction({ projectRoot,
    expectedJournalDigest: createHash("sha256").update(committedJournal).digest("hex"),
    recordSetupReceipt: async () => { throw new Error("canonical callback must not repeat"); },
  });

  assert.equal(finalized.status, "finalized");
  assert.equal(finalized.setupReceipt.kind, "agent-team-helpers-recovery");
  await assert.rejects(readFile(target), { code: "ENOENT" });
  await assert.rejects(readFile(journalPath), { code: "ENOENT" });
});

test("helper installation rolls back only when canonical recording proves no commit", async () => {
  const { installHelpers } = await import("../hooks/lib/helpers.mjs");
  const projectRoot = await fixture("agent-team-helper-receipt-failure-");

  await assert.rejects(installHelpers({ projectRoot, sourceRoot,
    recordSetupReceipt: async (receipt) => ({ status: "not_committed", receiptDigest: receipt.digest }) }), /not committed/);

  await assert.rejects(readFile(path.join(projectRoot, "scripts", "agent-team", "with-env.mjs")), { code: "ENOENT" });
  await assert.rejects(readFile(path.join(projectRoot, ".agent-team", "helpers.json")), { code: "ENOENT" });
  await assert.rejects(readFile(path.join(projectRoot, ".agent-team", "helper-transaction.json")), { code: "ENOENT" });
});

test("helper installation preserves postimages and journal when canonical outcome is unknown", async () => {
  const { installHelpers, inspectHelpers } = await import("../hooks/lib/helpers.mjs");
  const projectRoot = await fixture("agent-team-helper-unknown-commit-");
  let committed;
  await assert.rejects(installHelpers({ projectRoot, sourceRoot, recordSetupReceipt: async (receipt) => {
    committed = receipt;
    throw new Error("commit response lost");
  } }), /canonical_outcome_unknown/);
  assert.ok(committed?.digest);
  assert.match(await readFile(path.join(projectRoot, "scripts", "agent-team", "with-env.mjs"), "utf8"), /self-check/);
  const health = await inspectHelpers({ projectRoot, sourceRoot });
  assert.equal(health.status, "recovery_required");
  assert.match(health.pendingTransactionDigest, /^[a-f0-9]{64}$/);
});

test("helper installation never rolls back after a proven commit when finalization conflicts", async () => {
  const { installHelpers } = await import("../hooks/lib/helpers.mjs");
  const projectRoot = await fixture("agent-team-helper-finalize-");
  const journalPath = path.join(projectRoot, ".agent-team", "helper-transaction.json");
  const concurrent = Buffer.from("concurrent journal edit\n");

  await assert.rejects(installHelpers({ projectRoot, sourceRoot, recordSetupReceipt: async (receipt) => {
    await writeFile(journalPath, concurrent);
    return { status: "committed", receiptDigest: receipt.digest };
  } }), /recovery_required/);

  assert.match(await readFile(path.join(projectRoot, "scripts", "agent-team", "with-env.mjs"), "utf8"), /self-check/);
  assert.deepEqual(await readFile(journalPath), concurrent);
});
