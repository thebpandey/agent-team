import { execFile } from "node:child_process";
import { readFile, readdir } from "node:fs/promises";
import path from "node:path";
import { promisify } from "node:util";
import { createHash } from "node:crypto";
import { identityFor, loadCanonicalState, loadCanonicalTracker } from "./canonical-state.mjs";

const run = promisify(execFile);

async function checkpointSources(project, checkpoint, budget) {
  const result = [];
  for (const pointer of checkpoint.sourcePointers ?? []) {
    const recorded = checkpoint.sourceEvidence?.find((entry) => entry.path === pointer.path)?.fingerprint;
    try {
      const read = () => readFile(path.resolve(project.root, pointer.path));
      const source = await (budget ? budget.run(read) : read());
      const fingerprint = createHash("sha256").update(source).digest("hex");
      result.push({ ...pointer, status: !recorded ? "unavailable" : recorded === fingerprint ? "current" : "stale" });
    } catch { result.push({ ...pointer, status: "unavailable" }); }
  }
  return result;
}

/** Current original records and revision binding, also enforced by consequential resume. */
export async function inspectCheckpointEvidence(project, checkpoint, { budget, probe = runBoundedProbe } = {}) {
  const sourceEvidence = await checkpointSources(project, checkpoint, budget);
  const [head, dirty] = await Promise.all([
    probe("git", ["rev-parse", "HEAD"], { cwd: checkpoint.worktree, budget, timeoutMs: 500, maxOutputBytes: 256 }),
    probe("git", ["status", "--porcelain", "--untracked-files=all"], { cwd: checkpoint.worktree, budget, timeoutMs: 500, maxOutputBytes: 2048 }),
  ]);
  const unavailable = !checkpoint.evidenceRevision || head.status !== "available" || dirty.status !== "available"
    || sourceEvidence.some(({ status }) => status === "unavailable");
  const stale = sourceEvidence.some(({ status }) => status === "stale") || head.output.trim() !== checkpoint.evidenceRevision
    || head.output.trim() !== checkpoint.revision || dirty.output.trim() !== "";
  return { status: unavailable ? "unavailable" : stale ? "stale" : "current", evidenceRevision: checkpoint.evidenceRevision, sourceEvidence };
}

export async function runBoundedProbe(executable, args, { cwd, timeoutMs = 1000, maxOutputBytes = 4096, budget } = {}) {
  try {
    const { stdout, stderr } = await run(executable, args, { cwd, timeout: budget?.timeout(timeoutMs) ?? timeoutMs,
      ...(budget ? { signal: budget.signal } : {}), maxBuffer: 1024 * 1024, encoding: "utf8" });
    return { status: "available", output: `${stdout}${stderr}`.slice(0, maxOutputBytes) };
  } catch (error) {
    const output = `${error.stdout ?? ""}${error.stderr ?? ""}`.slice(0, maxOutputBytes);
    if (error.killed || error.signal || ["ETIMEDOUT", "ABORT_ERR", "EVENT_DEADLINE"].includes(error.code)) return { status: "timeout", output };
    if (error.code === "ENOENT") return { status: "unavailable", output: "" };
    return { status: "failed", output };
  }
}

function evidence(probeResult, transform = (value) => value) {
  if (probeResult.status !== "available") return { status: "unavailable" };
  const value = transform(probeResult.output.trim());
  return value ? { status: "current", value } : { status: "unavailable" };
}

async function handoffInventory(root, limit = 20, budget) {
  const items = [];
  const queue = [{ directory: root, relative: "" }];
  try {
    while (queue.length && items.length <= limit) {
      const { directory, relative } = queue.shift();
      for (const entry of await (budget ? budget.run(() => readdir(directory, { withFileTypes: true })) : readdir(directory, { withFileTypes: true }))) {
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

async function factualSnapshot(project, sessionId, probe, { includeProbes, budget, canonical: suppliedCanonical }) {
  const boundedProbe = (executable, args, options) => probe(executable, args, { ...options, budget });
  const [branchProbe, revisionProbe, dirtyProbe, githubProbe] = await Promise.all([
    boundedProbe("git", ["branch", "--show-current"], { cwd: project.worktreeRoot, timeoutMs: 500, maxOutputBytes: 256 }),
    boundedProbe("git", ["rev-parse", "HEAD"], { cwd: project.worktreeRoot, timeoutMs: 500, maxOutputBytes: 256 }),
    boundedProbe("git", ["status", "--porcelain", "--untracked-files=no"], { cwd: project.worktreeRoot, timeoutMs: 500, maxOutputBytes: 2048 }),
    includeProbes ? boundedProbe("gh", ["pr", "status"], { cwd: project.worktreeRoot, timeoutMs: 1000, maxOutputBytes: 512 }) : { status: "not_requested" },
  ]);
  const dirtyLines = dirtyProbe.status === "available" ? dirtyProbe.output.split(/\r?\n/).filter(Boolean) : [];
  let canonical;
  try {
    canonical = suppliedCanonical ?? await loadCanonicalState(project, { budget });
    if (canonical.tracker?.status === "not_read") Object.assign(canonical, await loadCanonicalTracker(project, { budget }));
  } catch {
    canonical = undefined;
  }
  const identity = canonical ? identityFor(canonical.registry, sessionId) : { role: "unknown" };
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
    tracker: canonical?.tracker ?? { status: "unavailable", path: project.paths.tasks },
    handoffs: await handoffInventory(project.paths.handoffs, 20, budget),
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
  includeGit = false,
  sessionId = "unknown",
  taskId,
  worktree = project.worktreeRoot,
  probe = runBoundedProbe,
  budget,
  canonical,
} = {}) {
  if (!project.active) return { status: "unavailable", reason: project.reason };
  const bounded = (action) => budget ? budget.run(action) : action();
  const finish = async (snapshot) => {
    if (!includeProbes && !includeGit) return snapshot;
    const facts = await factualSnapshot(project, sessionId, probe, { includeProbes, budget, canonical });
    const binding = snapshot.evidenceRevision ?? snapshot.revision;
    const stale = binding && (facts.git.revision.value !== binding || facts.git.dirty.entries.length > 0);
    return { ...snapshot, ...facts, ...(stale ? { status: "stale", evidenceStatus: "stale" } : {}) };
  };
  let names;
  try {
    names = await bounded(() => readdir(project.paths.checkpoints));
  } catch (error) {
    if (error.code === "ENOENT") return finish({ status: "unavailable", reason: "checkpoint_missing" });
    return finish({ status: "unavailable", reason: "checkpoint_unreadable" });
  }

  const records = [];
  for (const name of names.filter((entry) => entry.endsWith(".json")).slice(0, 100)) {
    try {
      const source = await bounded(() => readFile(path.join(project.paths.checkpoints, name), "utf8"));
      if (Buffer.byteLength(source) > 32768) continue;
      const record = JSON.parse(source);
      const recordSession = record.sessionId ?? name.replace(/\.json$/, "");
      if (sessionId !== "unknown" && recordSession !== sessionId) continue;
      if (taskId && !record.taskIds?.includes(taskId)) continue;
      if (record.worktree && path.resolve(record.worktree) !== path.resolve(worktree)) continue;
      const timestamp = Date.parse(record.updatedAt);
      if (Number.isFinite(timestamp)) records.push({ ...record, sessionId: recordSession, path: path.join(project.paths.checkpoints, name), timestamp });
    } catch {
      // A malformed record is unavailable evidence, not proof that work is current.
    }
  }
  records.sort((left, right) => right.timestamp - left.timestamp);
  const latest = records[0];
  if (!latest) return finish({ status: "unavailable", reason: "checkpoint_invalid" });
  const sourceEvidence = await checkpointSources(project, latest, budget);
  let task;
  let currentPending = [];
  if (taskId) {
    try {
      const current = await loadCanonicalState(project, { budget });
      if (current.tracker.status === "current") task = current.tasks.find(({ id }) => id === taskId);
      currentPending = Object.entries(current.state.pendingOperations ?? {}).filter(([, entry]) => !entry.taskId || entry.taskId === taskId)
        .map(([operationId, entry]) => ({ operationId, ...entry }));
    } catch { /* A missing authority is not reconstructed from hook metadata. */ }
  }
  const sourceChanged = sourceEvidence.some(({ status }) => status === "stale")
    || latest.evidenceRevision && latest.revision !== latest.evidenceRevision;
  return finish({
    status: !sourceChanged && now.getTime() - latest.timestamp <= staleAfterMs ? "current" : "stale",
    evidenceStatus: sourceChanged ? "stale" : sourceEvidence.some(({ status }) => status === "unavailable") ? "unavailable" : "current",
    updatedAt: latest.updatedAt,
    sessionId: latest.sessionId,
    path: latest.path,
    version: latest.version,
    revision: latest.revision,
    evidenceRevision: latest.evidenceRevision,
    taskIds: latest.taskIds ?? [],
    nextAction: latest.nextAction,
    sourcePointers: latest.sourcePointers ?? [],
    sourceEvidence,
    task,
    pendingOperations: [...(latest.pendingOperations ?? []), ...currentPending].slice(0, 20),
    uncertainty: latest.uncertainty ?? [],
    scope: latest.scope,
    restoreIndex: { tracker: project.paths.tasks, checkpoint: latest.path, sources: latest.sourcePointers ?? [] },
  });
}
