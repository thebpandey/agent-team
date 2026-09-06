import { execFile } from "node:child_process";
import { access } from "node:fs/promises";
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
export async function runLintChecks(root, changedFiles, { timeoutMs = 3000, maxOutputBytes = 4096, executableName = "eslint" } = {}) {
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
    const binary = await executable(root, cwd, executableName);
    if (!binary) return { status: "skipped", reason: "missing_executable", batches: [] };
    try {
      const { stdout, stderr } = await run(binary, targets, { cwd, timeout: timeoutMs, maxBuffer: 1024 * 1024, encoding: "utf8" });
      batches.push({ cwd, files: targets, status: "passed", output: `${stdout}${stderr}`.slice(0, maxOutputBytes) });
    } catch (error) {
      const output = `${error.stdout ?? ""}${error.stderr ?? ""}`.slice(0, maxOutputBytes);
      if (error.killed || error.signal || error.code === "ETIMEDOUT") return { status: "timeout", batches, output };
      batches.push({ cwd, files: targets, status: "failed", output });
    }
  }
  return { status: "checked", batches, output: batches.map(({ output }) => output).join("\n").slice(0, maxOutputBytes) };
}
