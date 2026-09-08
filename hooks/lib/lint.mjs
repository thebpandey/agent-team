import { execFile } from "node:child_process";
import { access, mkdir, writeFile } from "node:fs/promises";
import { createHash } from "node:crypto";
import path from "node:path";
import { promisify } from "node:util";

const run = promisify(execFile);
const configs = ["eslint.config.js", "eslint.config.mjs", "eslint.config.cjs", ".eslintrc", ".eslintrc.json"];

async function present(file) {
  try {
    await access(file);
    return true;
  } catch {
    return false;
  }
}

function inside(root, target) {
  const relative = path.relative(root, target);
  return relative === "" || (!relative.startsWith("..") && !path.isAbsolute(relative));
}

async function configRoot(root, file) {
  let directory = path.dirname(path.resolve(root, file));
  while (inside(root, directory)) {
    for (const name of configs) if (await present(path.join(directory, name))) return directory;
    if (directory === root) break;
    directory = path.dirname(directory);
  }
  return root;
}

async function executable(root, start, name) {
  let directory = start;
  while (inside(root, directory)) {
    const candidate = path.join(directory, "node_modules", ".bin", name);
    if (await present(candidate)) return candidate;
    if (directory === root) break;
    directory = path.dirname(directory);
  }
  return undefined;
}

function eligible(file) {
  return /\.[cm]?[jt]sx?$/i.test(file) && !/(^|\/)(node_modules|vendor|dist|build|generated|coverage)(\/|$)|\.min\.[cm]?js$/i.test(file);
}

/** Run one installed linter process per config root; never install a missing tool. */
export async function runLintChecks(root, changedFiles, { timeoutMs = 3000, maxOutputBytes = 4096, executableName = "eslint", budget } = {}) {
  const files = [...new Set(changedFiles.map((file) => file.replaceAll("\\", "/")))].filter(eligible);
  const grouped = new Map();
  for (const file of files) {
    const directory = await configRoot(root, file);
    if (!grouped.has(directory)) grouped.set(directory, []);
    grouped.get(directory).push(path.relative(directory, path.resolve(root, file)).replaceAll("\\", "/"));
  }
  if (!grouped.size) return { status: "skipped", reason: "no_changed_files", batches: [] };

  const batches = [];
  for (const [cwd, targets] of grouped) {
    if (budget?.remaining() === 0) {
      batches.push({ cwd, files: targets, status: "timeout", reason: "event_deadline", output: "" });
      continue;
    }
    const binary = await executable(root, cwd, executableName);
    if (!binary) {
      batches.push({ cwd, files: targets, status: "skipped", reason: "missing_executable", output: "" });
      continue;
    }
    let batch;
    let fullOutput;
    try {
      const { stdout, stderr } = await run(binary, targets, { cwd, timeout: budget?.timeout(timeoutMs) ?? timeoutMs,
        maxBuffer: 1024 * 1024, encoding: "utf8", ...(budget ? { signal: budget.signal } : {}) });
      fullOutput = `${stdout}${stderr}`;
      batch = { cwd, files: targets, status: "passed" };
    } catch (error) {
      fullOutput = `${error.stdout ?? ""}${error.stderr ?? ""}`;
      batch = { cwd, files: targets, status: error.killed || error.signal || ["ETIMEDOUT", "ABORT_ERR", "EVENT_DEADLINE"].includes(error.code) ? "timeout" : "failed" };
      if (error.code === "ERR_CHILD_PROCESS_STDIO_MAXBUFFER") batch.reason = "output_limit";
    }
    batch.output = bounded(fullOutput, maxOutputBytes);
    if (fullOutput) {
      const hash = createHash("sha256").update(cwd).update(JSON.stringify(targets)).update(fullOutput).digest("hex");
      const logPath = path.join(root, ".agent-team", "logs", "lint", `${hash}.log`);
      try {
        await mkdir(path.dirname(logPath), { recursive: true, mode: 0o700 });
        await writeFile(logPath, fullOutput, { mode: 0o600 });
        batch.logPath = logPath;
      } catch {
        batch.logStatus = "unavailable";
      }
    }
    batches.push(batch);
  }
  const statuses = new Set(batches.map(({ status }) => status));
  const status = statuses.has("failed") ? "failed" : statuses.has("timeout") ? "timeout"
    : statuses.has("skipped") ? (statuses.size === 1 ? "skipped" : "incomplete") : "checked";
  return { status, ...(status === "skipped" ? { reason: "missing_executable" } : {}), batches,
    output: bounded(batches.map(({ output }) => output).filter(Boolean).join("\n"), maxOutputBytes) };
}

function bounded(source, bytes) {
  return Buffer.from(source).subarray(0, bytes).toString("utf8").replace(/\uFFFD$/, "");
}

export function lintMessages(lint, root) {
  if (lint.status === "checked" || lint.reason === "no_changed_files") return [];
  const messages = [`Changed-file lint ${lint.status}. Repair affected files and rerun required checks before completion.`];
  for (const batch of lint.batches.filter(({ status }) => status !== "passed").slice(0, 8)) {
    const files = batch.files.map((file) => path.relative(root, path.resolve(batch.cwd, file))).join(", ");
    messages.push(`${files}: ${batch.status}${batch.reason ? ` (${batch.reason})` : ""}${batch.output ? `\n${bounded(batch.output, 1500)}` : ""}${batch.logPath ? `\nFull lint log: ${batch.logPath}` : ""}`);
  }
  return [...new Set(messages)];
}
