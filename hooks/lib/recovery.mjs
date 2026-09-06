import { execFile } from "node:child_process";
import { access, readFile, readdir } from "node:fs/promises";
import path from "node:path";
import { promisify } from "node:util";
import { identityFor, loadCanonicalState } from "./canonical-state.mjs";

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

function evidence(probeResult, transform = (value) => value) {
  if (probeResult.status !== "available") return { status: "unavailable" };
  const value = transform(probeResult.output.trim());
  return value ? { status: "current", value } : { status: "unavailable" };
}

async function handoffInventory(root, limit = 20) {
  const items = [];
  const queue = [{ directory: root, relative: "" }];
  try {
    while (queue.length && items.length <= limit) {
      const { directory, relative } = queue.shift();
      for (const entry of await readdir(directory, { withFileTypes: true })) {
        const name = path.posix.join(relative, entry.name);
        if (entry.isDirectory()) queue.push({ directory: path.join(directory, entry.name), relative: name });
        else if (entry.isFile()) items.push(name);
        if (items.length > limit) break;
      }
    }
  } catch (error) {
    if (error.code === "ENOENT") return { status: "current", count: 0, items: [], truncated: false };
    return { status: "unavailable", count: 0, items: [], truncated: false };
  }
  return { status: "current", count: items.length, items: items.slice(0, limit), truncated: items.length > limit };
}

function operationPointers(state) {
  const integration = state.integration ?? {};
  const release = state.release ?? {};
  return {
    integration: {
      ...(integration.operationId ? { operationId: integration.operationId } : {}),
      ...(integration.expectedRevision ? { revision: integration.expectedRevision } : {}),
    },
    release: {
      ...(release.batchId ? { batchId: release.batchId } : {}),
      ...(release.artifact?.id ? { artifactId: release.artifact.id } : {}),
      ...(release.expectedRevision ? { revision: release.expectedRevision } : {}),
      ...(Array.isArray(release.taskIds) ? { taskIds: release.taskIds.slice(0, 20) } : {}),
    },
  };
}

async function factualSnapshot(project, sessionId, probe) {
  const [branchProbe, revisionProbe, dirtyProbe, githubProbe] = await Promise.all([
    probe("git", ["branch", "--show-current"], { cwd: project.worktreeRoot, timeoutMs: 500, maxOutputBytes: 256 }),
    probe("git", ["rev-parse", "HEAD"], { cwd: project.worktreeRoot, timeoutMs: 500, maxOutputBytes: 256 }),
    probe("git", ["status", "--porcelain", "--untracked-files=no"], { cwd: project.worktreeRoot, timeoutMs: 500, maxOutputBytes: 2048 }),
    probe("gh", ["pr", "status"], { cwd: project.worktreeRoot, timeoutMs: 1000, maxOutputBytes: 512 }),
  ]);
  const dirtyLines = dirtyProbe.status === "available" ? dirtyProbe.output.split(/\r?\n/).filter(Boolean) : [];
  let canonical;
  try {
    canonical = await loadCanonicalState(project);
  } catch {
    canonical = undefined;
  }
  const identity = canonical ? identityFor(canonical.registry, sessionId) : { role: "unknown" };
  let trackerStatus = "current";
  try {
    await access(project.paths.tasks);
  } catch {
    trackerStatus = "unavailable";
  }
  return {
    worktree: project.worktreeRoot,
    git: {
      branch: evidence(branchProbe),
      revision: evidence(revisionProbe, (value) => (/^[0-9a-f]{40,64}$/i.test(value) ? value : "")),
      dirty: {
        status: dirtyProbe.status === "available" ? "current" : "unavailable",
        entries: dirtyLines.slice(0, 20),
        truncated: dirtyLines.length > 20 || dirtyProbe.output.length >= 2048,
      },
    },
    identity: canonical ? {
      status: identity.role === "unknown" ? "unavailable" : "current",
      projectId: canonical.registry.projectId ?? project.projectId,
      sessionId,
      kind: identity.role,
      ...(identity.role === "team" ? {
        teamId: identity.team["team id"],
        taskIds: identity.team.tasks.split(/\s*,\s*/).filter(Boolean).slice(0, 20),
      } : { taskIds: [] }),
    } : { status: "unavailable", projectId: project.projectId, sessionId, kind: "unknown", taskIds: [] },
    tracker: { status: trackerStatus, path: project.paths.tasks },
    handoffs: await handoffInventory(project.paths.handoffs),
    controls: {
      projectPaused: canonical?.state.run?.paused === true,
      integrationPaused: canonical?.state.integration?.paused === true,
      integrationHold: canonical?.state.integration?.hold === true,
      releaseHold: canonical?.state.release?.hold === true,
    },
    operations: operationPointers(canonical?.state ?? {}),
    probes: {
      git: [branchProbe, revisionProbe, dirtyProbe].every(({ status }) => status === "available") ? "available" : "unavailable",
      github: githubProbe.status,
    },
  };
}

/** Report recovery evidence freshness. This advisory never resumes or edits work. */
export async function inspectRecovery(project, {
  now = new Date(),
  staleAfterMs = 15 * 60_000,
  includeProbes = false,
  sessionId = "unknown",
  probe = runBoundedProbe,
} = {}) {
  if (!project.active) return { status: "unavailable", reason: project.reason };
  const finish = async (snapshot) => {
    if (!includeProbes) return snapshot;
    return { ...snapshot, ...(await factualSnapshot(project, sessionId, probe)) };
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
