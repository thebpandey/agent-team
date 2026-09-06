import { execFile } from "node:child_process";
import { readFile, readdir } from "node:fs/promises";
import path from "node:path";
import { promisify } from "node:util";

const run = promisify(execFile);

export async function runBoundedProbe(executable, args, { cwd, timeoutMs = 1000, maxOutputBytes = 4096 } = {}) {
  try {
    const { stdout, stderr } = await run(executable, args, { cwd, timeout: timeoutMs, maxBuffer: 1024 * 1024, encoding: "utf8" });
    return { status: "available", output: `${stdout}${stderr}`.slice(0, maxOutputBytes) };
  } catch (error) {
    const output = `${error.stdout ?? ""}${error.stderr ?? ""}`.slice(0, maxOutputBytes);
    if (error.killed || error.signal || error.code === "ETIMEDOUT") return { status: "timeout", output };
    if (error.code === "ENOENT") return { status: "unavailable", output: "" };
    return { status: "failed", output };
  }
}

/** Report recovery evidence freshness. This advisory never resumes or edits work. */
export async function inspectRecovery(project, {
  now = new Date(),
  staleAfterMs = 15 * 60_000,
  includeProbes = false,
  probe = runBoundedProbe,
} = {}) {
  if (!project.active) return { status: "unavailable", reason: project.reason };
  const finish = async (snapshot) => {
    if (!includeProbes) return snapshot;
    const [git, github] = await Promise.all([
      probe("git", ["status", "--porcelain", "--untracked-files=no"], { cwd: project.worktreeRoot, timeoutMs: 500, maxOutputBytes: 512 }),
      probe("gh", ["pr", "status"], { cwd: project.worktreeRoot, timeoutMs: 1000, maxOutputBytes: 512 }),
    ]);
    return { ...snapshot, probes: { git: git.status, github: github.status } };
  };
  let names;
  try {
    names = await readdir(project.paths.checkpoints);
  } catch (error) {
    if (error.code === "ENOENT") return finish({ status: "unavailable", reason: "checkpoint_missing" });
    return finish({ status: "unavailable", reason: "checkpoint_unreadable" });
  }

  const records = [];
  for (const name of names.filter((entry) => entry.endsWith(".json")).slice(0, 100)) {
    try {
      const record = JSON.parse(await readFile(path.join(project.paths.checkpoints, name), "utf8"));
      const timestamp = Date.parse(record.updatedAt);
      if (Number.isFinite(timestamp)) records.push({ ...record, timestamp });
    } catch {
      // A malformed record is unavailable evidence, not proof that work is current.
    }
  }
  records.sort((left, right) => right.timestamp - left.timestamp);
  const latest = records[0];
  if (!latest) return finish({ status: "unavailable", reason: "checkpoint_invalid" });
  return finish({
    status: now.getTime() - latest.timestamp <= staleAfterMs ? "current" : "stale",
    updatedAt: latest.updatedAt,
    sessionId: latest.sessionId,
    nextAction: latest.nextAction,
  });
}
