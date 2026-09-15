import { execFileSync } from "node:child_process";
import { readFileSync, writeFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

export function quoteShellArg(value) {
  const text = String(value);
  return /^[A-Za-z0-9_./:@%+=,-]+$/.test(text) ? text : `'${text.replaceAll("'", `'"'"'`)}'`;
}

function option(argv, name) {
  const index = argv.indexOf(name);
  return index < 0 ? undefined : argv[index + 1];
}

function selfCheck() {
  if (quoteShellArg("a b") !== "'a b'" || quoteShellArg("a'b") !== "'a'\"'\"'b'") throw new Error("self-check failed");
  process.stdout.write("self-check ok\n");
}

function validEvidence(evidence, taskId, revision) {
  return evidence && typeof evidence === "object" && !Array.isArray(evidence) && evidence.status === "passed"
    && evidence.revision === revision && Array.isArray(evidence.taskIds) && evidence.taskIds.length === 1
    && evidence.taskIds[0] === taskId && evidence.requirementsReconciled === true
    && evidence.review?.status === "passed" && evidence.review.taskId === taskId
    && evidence.review.revision === revision && typeof evidence.review.reviewer === "string" && evidence.review.reviewer.length > 0
    && Array.isArray(evidence.checks) && evidence.checks.length > 0
    && evidence.checks.every((check) => check && typeof check === "object" && check.status === "passed"
      && check.taskId === taskId && check.revision === revision && typeof check.name === "string" && check.name.length > 0);
}

function main(argv) {
  if (argv[0] === "--self-check") return selfCheck();
  const [taskId, evidencePath, outputPath] = argv;
  const projectRoot = option(argv, "--project");
  const cli = option(argv, "--cli");
  const sessionId = option(argv, "--session");
  if (!taskId || !evidencePath || !outputPath || !projectRoot || !cli || !sessionId) {
    throw new Error("usage: gate-rebind.mjs <task-id> <evidence.json> <out.json> --project <absolute> --cli <absolute> --session <id>");
  }
  if (![projectRoot, cli, evidencePath, outputPath].every(path.isAbsolute)) throw new Error("all paths must be absolute");
  const evidence = JSON.parse(readFileSync(evidencePath, "utf8"));
  const status = JSON.parse(execFileSync(process.execPath, [cli, "status", "--project", projectRoot], {
    cwd: projectRoot, encoding: "utf8", stdio: ["ignore", "pipe", "ignore"],
  }));
  const stateVersion = status.stateVersion ?? status.freshness?.stateVersion;
  const fingerprint = status.tracker?.fingerprint ?? status.freshness?.fingerprint;
  const revision = status.revision ?? status.freshness?.revision;
  if (!Number.isSafeInteger(stateVersion) || stateVersion < 0 || !/^[0-9a-f]{64}$/i.test(fingerprint)
    || !/^[0-9a-f]{40,64}$/i.test(revision)) throw new Error("status did not provide fresh state bindings");
  if (!validEvidence(evidence, taskId, revision)) throw new Error("evidence is not a passed result bound to the current task and revision");
  const request = {
    schemaVersion: 1, actorSessionId: sessionId, expectedVersion: stateVersion,
    request: {
      operationId: `gate-completion-${taskId}-${Date.now()}`, gate: "completion", taskIds: [taskId],
      expectedFingerprint: fingerprint, expectedRevision: revision, evidencePath,
    },
  };
  writeFileSync(outputPath, `${JSON.stringify(request, null, 2)}\n`, { mode: 0o600 });
  const command = [process.execPath, cli, "gate-evidence", "--project", projectRoot, "--request", outputPath].map(quoteShellArg).join(" ");
  process.stdout.write(`wrote ${outputPath}\nnext, exactly and unchained: ${command}\n`);
}

if (process.argv[1] && fileURLToPath(import.meta.url) === path.resolve(process.argv[1])) {
  try { main(process.argv.slice(2)); }
  catch (error) { process.stderr.write(`${error.message}\n`); process.exitCode = 2; }
}
