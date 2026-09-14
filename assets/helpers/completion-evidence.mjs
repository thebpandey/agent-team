import { mkdirSync, readFileSync, writeFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const RESULTS = new Set(["passed", "failed", "unrun", "skipped"]);

function required(value, label) {
  if (typeof value !== "string" || !value) throw new Error(`${label} is required`);
  return value;
}

function validateResult(result, kind, taskId, revision) {
  if (!result || typeof result !== "object" || Array.isArray(result)) throw new Error(`${kind} must be a result object`);
  if (!RESULTS.has(result.status)) throw new Error(`${kind}.status must be an actual result`);
  if (result.taskId !== taskId || result.revision !== revision) throw new Error(`${kind} must bind the task and revision`);
  return structuredClone(result);
}

export function buildCompletionEvidence(input) {
  if (!input || typeof input !== "object" || Array.isArray(input)) throw new Error("evidence input must be an object");
  const taskId = required(input.taskId, "taskId");
  const revision = required(input.revision, "revision");
  if (!/^[0-9a-f]{40,64}$/i.test(revision)) throw new Error("revision must be a Git object ID");
  if (input.requirementsReconciled !== true) throw new Error("requirementsReconciled must be true");
  const review = validateResult(input.review, "review", taskId, revision);
  required(review.reviewer, "review.reviewer");
  if (!Array.isArray(input.checks) || input.checks.length === 0) throw new Error("at least one check result object is required");
  const checks = input.checks.map((result, index) => {
    const checked = validateResult(result, `checks[${index}]`, taskId, revision);
    required(checked.name, `checks[${index}].name`);
    return checked;
  });
  const passed = review.status === "passed" && checks.every(({ status }) => status === "passed");
  return { status: passed ? "passed" : "failed", revision, taskIds: [taskId], requirementsReconciled: true, review, checks };
}

function selfCheck() {
  const revision = "a".repeat(40);
  const evidence = buildCompletionEvidence({
    taskId: "TASK-1", revision, requirementsReconciled: true,
    review: { status: "passed", revision, taskId: "TASK-1", reviewer: "reviewer" },
    checks: [{ name: "unit", status: "failed", revision, taskId: "TASK-1" }],
  });
  if (evidence.status !== "failed" || evidence.checks[0].status !== "failed") throw new Error("self-check failed");
  let rejected = false;
  try { buildCompletionEvidence({ taskId: "TASK-1", revision, requirementsReconciled: true, review: evidence.review, checks: ["unit"] }); }
  catch { rejected = true; }
  if (!rejected) throw new Error("self-check accepted a check name without a result");
  process.stdout.write("self-check ok\n");
}

function main(argv) {
  if (argv[0] === "--self-check") return selfCheck();
  const [inputPath, outputPath] = argv;
  if (!inputPath || !outputPath) throw new Error("usage: completion-evidence.mjs <results.json> <evidence.json>");
  const input = JSON.parse(readFileSync(inputPath, "utf8"));
  const evidence = buildCompletionEvidence(input);
  mkdirSync(path.dirname(path.resolve(outputPath)), { recursive: true });
  writeFileSync(outputPath, `${JSON.stringify(evidence, null, 2)}\n`, { mode: 0o600 });
  process.stdout.write(`${JSON.stringify({ status: "written", path: path.resolve(outputPath), evidenceStatus: evidence.status })}\n`);
}

if (process.argv[1] && fileURLToPath(import.meta.url) === path.resolve(process.argv[1])) {
  try { main(process.argv.slice(2)); }
  catch (error) { process.stderr.write(`${error.message}\n`); process.exitCode = 2; }
}
