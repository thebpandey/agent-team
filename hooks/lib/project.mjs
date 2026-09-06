import { execFile } from "node:child_process";
import { access, readFile, realpath } from "node:fs/promises";
import path from "node:path";
import { promisify } from "node:util";

const run = promisify(execFile);

async function git(cwd, ...args) {
  const { stdout } = await run("git", args, { cwd, encoding: "utf8", timeout: 1500, maxBuffer: 16 * 1024 });
  return stdout.trim();
}

async function readable(file) {
  try {
    await access(file);
    return true;
  } catch {
    return false;
  }
}

/** Resolve shared state through Git metadata, including nested paths and linked worktrees. */
export async function resolveProject(cwd) {
  const resolvedCwd = await realpath(cwd);
  let worktreeRoot;
  let commonDirectory;
  try {
    worktreeRoot = await realpath(await git(resolvedCwd, "rev-parse", "--show-toplevel"));
    const value = await git(resolvedCwd, "rev-parse", "--git-common-dir");
    commonDirectory = await realpath(path.isAbsolute(value) ? value : path.resolve(resolvedCwd, value));
  } catch {
    return { active: false, reason: "not_git", root: resolvedCwd, cwd: resolvedCwd };
  }

  const commonRoot = path.basename(commonDirectory) === ".git" ? path.dirname(commonDirectory) : worktreeRoot;
  const candidates = [...new Set([commonRoot, worktreeRoot])];
  const root = (await Promise.all(candidates.map(async (candidate) => ({
    candidate,
    present: await readable(path.join(candidate, ".agent-team", "setup.json")),
  })))).find(({ present }) => present)?.candidate ?? commonRoot;
  const stateRoot = path.join(root, ".agent-team");
  const setupPath = path.join(stateRoot, "setup.json");
  let setup;
  try {
    setup = JSON.parse(await readFile(setupPath, "utf8"));
  } catch (error) {
    return {
      active: false,
      reason: error.code === "ENOENT" ? "setup_missing" : "setup_invalid",
      root,
      worktreeRoot,
      commonDirectory,
      cwd: resolvedCwd,
    };
  }

  const active = setup.skill === "agent-team" && typeof setup.projectId === "string" && setup.projectId.length > 0;
  return {
    active,
    reason: active ? "active" : "setup_unrecognized",
    root,
    worktreeRoot,
    commonDirectory,
    cwd: resolvedCwd,
    projectId: setup.projectId,
    setup,
    paths: {
      stateRoot,
      setup: setupPath,
      state: path.join(stateRoot, "state.json"),
      operationMappings: path.join(stateRoot, "operation-mappings.json"),
      tasks: path.join(stateRoot, "TASKS.md"),
      teams: path.join(stateRoot, "TEAMS.md"),
      checkpoints: path.join(stateRoot, "checkpoints"),
      handoffs: path.join(stateRoot, "handoffs"),
      locks: path.join(stateRoot, ".locks"),
    },
  };
}
