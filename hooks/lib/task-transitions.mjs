import { createHash, randomUUID } from "node:crypto";
import { execFile } from "node:child_process";
import { promisify } from "node:util";
import { readFile, readlink, realpath, rename, rm, writeFile } from "node:fs/promises";
import path from "node:path";
import os from "node:os";
import { loadCanonicalState, loadCanonicalTracker } from "./canonical-state.mjs";
import { withDirectoryLock } from "./lock.mjs";
import { inspectCheckpointEvidence } from "./recovery.mjs";
import { beadsEnvironment } from "./tracker.mjs";

const digest = (value) => createHash("sha256").update(typeof value === "string" ? value : JSON.stringify(value)).digest("hex");
const operationSignature = ({ expectedVersion, expectedFingerprint, ...operation }) => digest(operation);
const run = promisify(execFile);
const conflict = (reason) => ({ status: "conflict", reason });
const finished = new Set(["verified", "integrated", "deployed", "closed", "done"]);
const unclaimed = new Set(["", "none", "unassigned", "-"]);
const split = (value) => Array.isArray(value) ? value : String(value ?? "").split(/\s*,\s*/).filter((id) => id && !["none", "-"].includes(id));
const sameIds = (left, right) => Array.isArray(left) && Array.isArray(right) && left.length === right.length
  && [...left].sort().every((value, index) => value === [...right].sort()[index]);
const boundedString = (value, maximum = 256) => typeof value === "string" && value.trim() === value && value.length > 0 && value.length <= maximum;
const ref = (value) => typeof value === "string" && (/^refs\/heads\/[\w./-]+$/.test(value)
  || (/^refs\/tags\/[A-Za-z0-9][\w./-]*$/.test(value) && !/[./]$|\.\.|\/\//.test(value.slice("refs/tags/".length))));
const configuredRemoteBaseRef = (value) => typeof value === "string" && /^[A-Za-z0-9][\w./-]*$/.test(value)
  && !value.startsWith("refs/") && !/[./]$|\.\.|\/\//.test(value) ? `refs/heads/${value}` : undefined;
const provenance = (value, { ownerSessionId, revision, taskIds }) => value && typeof value === "object"
  && typeof value.source === "string" && value.source.trim() && value.source.length <= 256
  && value.scope === "integration" && value.ownerSessionId === ownerSessionId && value.revision === revision
  && Array.isArray(value.taskIds) && JSON.stringify([...value.taskIds].sort()) === JSON.stringify([...taskIds].sort());
const recovery = (value, { revision, taskIds }) => value && value.status === "reconciled" && value.revision === revision
  && Array.isArray(value.taskIds) && JSON.stringify([...value.taskIds].sort()) === JSON.stringify([...taskIds].sort());
const preview = (value, revision) => value && typeof value.required === "boolean"
  && (!value.required || value.approvedRevision === revision);

function pendingAffectsTask(entry, task, canonical, project) {
  const taskIds = [...(entry.taskId ? [entry.taskId] : []), ...(Array.isArray(entry.taskIds) ? entry.taskIds : [])];
  const resources = [entry.worktree, ...(Array.isArray(entry.resources) ? entry.resources : [])].filter((value) => typeof value === "string" && value);
  if (entry.scope === "global" || entry.global === true || taskIds.includes(task.id) || (!taskIds.length && !resources.length)) return true;
  const worktree = canonical.registry.teams.find((team) => team["team id"] === task.owner)?.worktree ?? project.worktreeRoot;
  for (const id of taskIds) {
    const other = canonical.tasks.find((candidate) => candidate.id === id);
    const assigned = canonical.registry.teams.find((team) => team["team id"] === other?.owner)?.worktree;
    if (assigned) resources.push(assigned);
  }
  return resources.some((resource) => {
    const target = path.resolve(project.root, resource);
    const current = path.resolve(project.root, worktree);
    return target === current || target.startsWith(`${current}${path.sep}`) || current.startsWith(`${target}${path.sep}`);
  });
}

/** Record the observer's PID namespace: a sandbox can hide a still-live host writer. */
export async function captureWriterIdentity(pid = process.pid) {
  const [stat, bootId, pidNamespace] = await Promise.all([readFile(`/proc/${pid}/stat`, "utf8"), readFile("/proc/sys/kernel/random/boot_id", "utf8"), readlink("/proc/self/ns/pid")]);
  const startTime = stat.slice(stat.lastIndexOf(")") + 2).split(/\s+/)[19];
  if (!startTime) throw new Error("Writer start identity is unavailable.");
  return { pid, startTime, bootId: bootId.trim(), host: os.hostname(), pidNamespace };
}

export async function inspectWriterIdentity(writer) {
  if (!writer || !Number.isInteger(writer.pid) || !writer.startTime || !writer.bootId || writer.host !== os.hostname()) return { status: "unknown", reason: "identity_unavailable" };
  const hidden = () => ({ status: "unknown", reason: "pid_visibility_unavailable" });
  if (typeof writer.pidNamespace !== "string" || !/^pid:\[\d+\]$/.test(writer.pidNamespace)) return hidden();
  try { if (await readlink("/proc/self/ns/pid") !== writer.pidNamespace) return hidden(); }
  catch { return hidden(); }
  let bootObserved = false;
  try {
    const bootId = (await readFile("/proc/sys/kernel/random/boot_id", "utf8")).trim();
    if (bootId !== writer.bootId) return { status: "unknown", reason: "boot_changed" };
    bootObserved = true;
    const current = await captureWriterIdentity(writer.pid);
    if (current.pidNamespace !== writer.pidNamespace) return hidden();
    return current.startTime === writer.startTime ? { status: "active" } : { status: "unknown", reason: "pid_reused" };
  } catch (error) {
    return bootObserved && error.code === "ENOENT" && error.path === `/proc/${writer.pid}/stat` ? { status: "stopped" } : { status: "unknown", reason: "probe_unavailable" };
  }
}

/** A pure admission view; it does not allocate compute or perform engineering scheduling. */
export function taskEligibility(canonical, { scopeTaskIds, capacity } = {}) {
  const held = [];
  const eligible = [];
  const globalReason = canonical.tracker?.status !== "current" ? "tracker_unavailable"
    : canonical.state.run?.paused ? "project_paused"
      : capacity && capacity.active >= capacity.limit - (capacity.reservedReview ?? 0) ? "capacity_reserved" : null;
  for (const task of canonical.tasks) {
    const dependencies = split(task["depends on"] ?? task.dependencies);
    const reason = globalReason ?? (scopeTaskIds && !scopeTaskIds.includes(task.id) ? "outside_scope"
      : !["ready", "open", "todo", "pending"].includes(task.status) ? "not_ready"
        : !unclaimed.has(task.owner ?? "") ? "already_claimed"
          : task.dependencyEvidence === "unavailable" ? "dependency_evidence_unavailable"
          : dependencies.some((id) => !canonical.tasks.some((other) => other.id === id && finished.has(other.status))) ? "dependency_unfinished" : null);
    if (reason) held.push({ id: task.id, reason }); else eligible.push(task);
  }
  eligible.sort((a, b) => Number(a.priority ?? 2) - Number(b.priority ?? 2) || a.id.localeCompare(b.id));
  return { eligible, held };
}

async function atomicWrite(file, source, { budget, filesystem = {} } = {}) {
  const io = { writeFile, rename, rm, ...filesystem };
  const temporary = `${file}.${process.pid}.${randomUUID()}.tmp`;
  try {
    budget?.check();
    await io.writeFile(temporary, source, { mode: 0o600, flag: "wx", ...(budget ? { signal: budget.signal } : {}) });
    budget?.check();
    await io.rename(temporary, file);
  } finally { await io.rm(temporary, { force: true }); }
}

/** Serialized operational evidence mutation. Task rows remain solely in the selected tracker. */
export async function mutateOperationalState(project, request, mutator, options = {}) {
  const { budget } = options;
  if (!project.active) return { status: "unavailable", reason: "inactive" };
  if (!/^[\w.:-]{1,128}$/.test(request.operationId ?? "")) return conflict("operation_identity_required");
  try {
    const execute = () => withDirectoryLock(path.join(project.paths.locks, "state.lock"), {
      operationId: request.operationId, actorSessionId: request.actorSessionId, pid: process.pid,
    }, async () => {
      const canonical = await loadCanonicalState(project, { includeTasks: false, budget });
      const owner = canonical.registry.projectOwner;
      if (typeof owner !== "string" || !/^[\w.:-]{1,128}$/.test(owner) || ["none", "unknown", "unassigned", "-"].includes(owner.toLowerCase())
        || typeof request.actorSessionId !== "string" || !/^[\w.:-]{1,128}$/.test(request.actorSessionId)
        || request.actorSessionId !== owner || !canonical.registry.projectId || canonical.registry.projectId !== project.projectId) return conflict("project_owner_required");
      const signature = operationSignature(request);
      const previous = canonical.state.operationReceipts?.[request.operationId];
      if (previous) return previous.signature === signature
        ? { status: "duplicate", version: canonical.state.stateVersion ?? 0, result: previous.result }
        : conflict("operation_identity_reused");
      const pending = canonical.state.pendingOperations?.[request.operationId];
      if (pending && pending.signature !== signature) return conflict("operation_identity_reused");
      if (!Number.isInteger(request.expectedVersion) || request.expectedVersion !== (canonical.state.stateVersion ?? 0)) return conflict("stale_version");
      const state = structuredClone(canonical.state);
      const persistIntent = async (intent) => {
        state.pendingOperations = { ...state.pendingOperations, [request.operationId]: { ...intent, signature, ownerSessionId: request.actorSessionId } };
        state.stateVersion = (state.stateVersion ?? 0) + 1;
        await atomicWrite(project.paths.state, `${JSON.stringify(state, null, 2)}\n`, options);
      };
      const mutation = await mutator(state, canonical, { persistIntent });
      if (mutation?.status && mutation.status !== "applied") return mutation;
      const next = mutation?.state ?? state;
      next.stateVersion = Math.max(canonical.state.stateVersion ?? 0, next.stateVersion ?? 0) + 1;
      next.operationReceipts = { ...next.operationReceipts, [request.operationId]: { signature, result: mutation?.result ?? {}, appliedAt: new Date().toISOString() } };
      await atomicWrite(project.paths.state, `${JSON.stringify(next, null, 2)}\n`, options);
      return { status: "applied", version: next.stateVersion, result: mutation?.result ?? {} };
    }, { budget });
    return await (budget ? budget.run(execute) : execute());
  } catch (error) {
    return { status: "unavailable", reason: error.code === "EVENT_DEADLINE" ? "deadline" : "state_unavailable" };
  }
}

function replaceTask(source, taskId, changes) {
  let columns;
  let changed = 0;
  const output = source.split(/\r?\n/).map((line) => {
    if (!/^\s*\|/.test(line)) return line;
    const values = line.trim().replace(/^\||\|$/g, "").split("|").map((value) => value.trim());
    if (values.map((value) => value.toLowerCase()).includes("id")) { columns = values.map((value) => value.toLowerCase()); return line; }
    if (!columns || values[columns.indexOf("id")] !== taskId) return line;
    for (const [name, value] of Object.entries(changes)) {
      const index = columns.indexOf(name);
      if (index < 0 || /[|\r\n]/.test(String(value))) throw new Error("Tracker mutation cannot preserve this table schema.");
      values[index] = value;
    }
    changed += 1;
    return `| ${values.join(" | ")} |`;
  }).join("\n");
  if (changed !== 1) throw new Error("Task row identity is ambiguous.");
  return output;
}

/** Claim or transition one task through the registered project owner. */
export async function transitionTask(project, request, options = {}) {
  const bounded = (action) => options.budget ? options.budget.run(action) : action();
  return mutateOperationalState(project, request, async (state, canonical, { persistIntent }) => {
    Object.assign(canonical, await loadCanonicalTracker(project, options));
    if (canonical.tracker.status !== "current") return { status: "unavailable", reason: "tracker_unavailable", tracker: canonical.tracker };
    const task = canonical.tasks.find(({ id }) => id === request.taskId);
    if (Object.entries(state.pendingOperations ?? {}).some(([id, entry]) => id !== request.operationId && entry.taskId === request.taskId
      && ["uncertain", "pending", "unknown"].includes(entry.phase ?? entry.status))) return { status: "unavailable", reason: "pending_task_operation" };
    const marker = `[agent-team-operation:${request.operationId}:${operationSignature(request)}]`;
    const reconciled = task?.["revision / evidence"]?.includes(marker);
    const pendingIntent = state.pendingOperations?.[request.operationId];
    if (reconciled && pendingIntent?.changes) {
      const status = canonical.tracker.kind === "beads" && ["paused", "parked"].includes(pendingIntent.changes.status) ? "deferred" : pendingIntent.changes.status;
      if (task.owner !== (pendingIntent.changes.owner ?? request.expectedOwner) || task.status !== status) return conflict("task_changed_after_interruption");
      if (pendingIntent.intendedRuntime) state.taskRuntime = { ...state.taskRuntime, [task.id]: pendingIntent.intendedRuntime };
      delete state.pendingOperations[request.operationId];
      return { state, result: { taskId: task.id, action: request.action, trackerFingerprint: canonical.tracker.fingerprint, reconciled: true } };
    }
    if (reconciled && request.action === "claim") {
      if (task.owner !== request.owner || task.status !== "in_progress") return conflict("claim_changed_after_interruption");
      if (request.writer !== undefined) {
        const runtime = state.taskRuntime?.[task.id] ?? {};
        if (runtime.writer && digest(runtime.writer) !== digest(request.writer)) return conflict("writer_already_registered");
        const observed = await bounded(() => inspectWriterIdentity(request.writer));
        if (!["active", "stopped"].includes(observed.status)) return conflict("new_writer_unverified");
        state.taskRuntime = { ...state.taskRuntime, [task.id]: { ...runtime, writer: request.writer, compute: observed.status, explicitPause: false } };
      }
      if (state.pendingOperations) delete state.pendingOperations[request.operationId];
      return { state, result: { taskId: task.id, action: request.action, trackerFingerprint: canonical.tracker.fingerprint, reconciled: true } };
    }
    if (!reconciled && request.expectedFingerprint !== canonical.tracker.fingerprint) return conflict("stale_tracker");
    if (!reconciled && state.pendingOperations?.[request.operationId]?.phase === "uncertain") return { status: "unavailable", reason: "tracker_write_uncertain" };
    if (!task || task.owner !== request.expectedOwner) return conflict("stale_owner");
    const runtime = state.taskRuntime?.[task.id] ?? {};
    const checkpointForTask = async (file) => {
      const resolved = await bounded(() => realpath(file));
      if (!resolved.startsWith(`${project.paths.checkpoints}${path.sep}`)) return conflict("checkpoint_identity_mismatch");
      const checkpoint = JSON.parse(await bounded(() => readFile(resolved, "utf8")));
      const team = canonical.registry.teams.find((entry) => entry["team id"] === task.owner);
      const expectedSession = team?.session ?? canonical.registry.projectOwner;
      const registeredWorktree = team ? team.worktree : project.root;
      if (typeof registeredWorktree !== "string" || !registeredWorktree.trim()
        || typeof checkpoint.worktree !== "string" || !checkpoint.worktree.trim()) return conflict("checkpoint_identity_mismatch");
      const expectedWorktree = path.resolve(project.root, registeredWorktree);
      const checkpointWorktree = path.resolve(project.root, checkpoint.worktree);
      if (checkpoint.sessionId !== expectedSession || checkpointWorktree !== expectedWorktree) return conflict("checkpoint_identity_mismatch");
      if (!checkpoint.taskIds?.includes(task.id) || !checkpoint.nextAction || !checkpoint.revision) return conflict("checkpoint_required");
      return { ...checkpoint, worktree: checkpointWorktree };
    };
    let changes;
    if (request.action === "claim") {
      const eligibility = taskEligibility(canonical, { scopeTaskIds: state.run?.taskIds, capacity: request.capacity });
      if (!eligibility.eligible.some(({ id }) => id === task.id)) return conflict(eligibility.held.find(({ id }) => id === task.id)?.reason ?? "not_ready");
      if (!canonical.registry.teams.some((team) => team["team id"] === request.owner) && request.owner !== canonical.registry.projectOwner) return conflict("registered_owner_required");
      if (request.writer !== undefined) {
        if (runtime.writer) return conflict("writer_already_registered");
        if ((await bounded(() => inspectWriterIdentity(request.writer))).status !== "active") return conflict("new_writer_unverified");
        state.taskRuntime = { ...state.taskRuntime, [task.id]: { ...runtime, writer: request.writer, compute: "active", explicitPause: false } };
      }
      changes = { owner: request.owner, status: "in_progress" };
    } else if (request.action === "pause") {
      changes = { status: "paused" };
      state.taskRuntime = { ...state.taskRuntime, [task.id]: { ...runtime, explicitPause: true, compute: "paused" } };
    } else if (["park", "resume"].includes(request.action)) {
      if ((runtime.explicitPause || task.status === "paused") && !(request.action === "resume" && request.explicitResume === true)) return conflict("explicit_pause");
      if (request.action === "park") {
        if (digest(request.writer ?? {}) !== digest(runtime.writer ?? {}) || (await bounded(() => inspectWriterIdentity(runtime.writer))).status !== "stopped") return conflict("writer_not_stopped");
        if (!request.repair?.maxAttempts || request.repair.attempts < request.repair.maxAttempts || !request.resumeWhen) return conflict("external_prerequisite_required");
        const checkpoint = await checkpointForTask(request.checkpointPath);
        if (checkpoint.status === "conflict") return checkpoint;
        changes = { status: "parked" };
        state.taskRuntime = { ...state.taskRuntime, [task.id]: { ...runtime, compute: "parked", checkpointPath: request.checkpointPath,
          resumeWhen: request.resumeWhen, repair: request.repair, explicitPause: false } };
      } else {
        if (state.run?.paused) return conflict("project_paused");
        if (request.capacity && request.capacity.active >= request.capacity.limit - (request.capacity.reservedReview ?? 0)) return conflict("capacity_reserved");
        if (state.run?.taskIds && !state.run.taskIds.includes(task.id)) return conflict("outside_scope");
        if (!(["parked", "paused"].includes(runtime.compute))) return conflict("task_not_parked");
        if ((await bounded(() => inspectWriterIdentity(runtime.writer))).status !== "stopped") return conflict("previous_writer_not_stopped");
        if ((await bounded(() => inspectWriterIdentity(request.writer))).status !== "active") return conflict("new_writer_unverified");
        const checkpointPath = runtime.compute === "paused" && request.explicitResume === true
          ? request.checkpointPath ?? runtime.checkpointPath : runtime.checkpointPath;
        if (typeof checkpointPath !== "string" || !checkpointPath) return conflict("checkpoint_required");
        const checkpoint = await checkpointForTask(checkpointPath);
        if (checkpoint.status === "conflict") return checkpoint;
        if ((await inspectCheckpointEvidence(project, checkpoint, options)).status !== "current") return conflict("stale_resume_evidence");
        const pending = [...(checkpoint.pendingOperations ?? []), ...Object.values(state.pendingOperations ?? {})];
        if (pending.some((entry) => ["unknown", "uncertain", "pending"].includes(entry.phase ?? entry.status)
          && pendingAffectsTask(entry, task, canonical, project))) return conflict("operation_reconciliation_required");
        if (runtime.resumeWhen) {
          if (!request.prerequisiteEvidence) return conflict("resume_evidence_required");
          const prerequisite = JSON.parse(await bounded(() => readFile(request.prerequisiteEvidence, "utf8")));
          if (prerequisite.status !== "passed" || prerequisite.taskId !== task.id || prerequisite.condition !== runtime.resumeWhen || prerequisite.revision !== checkpoint.revision) return conflict("resume_evidence_required");
        }
        changes = { status: "in_progress" };
        state.taskRuntime = { ...state.taskRuntime, [task.id]: { ...runtime, compute: "active", writer: request.writer, checkpointPath, explicitPause: false,
          ...(request.prerequisiteEvidence ? { prerequisiteEvidence: request.prerequisiteEvidence } : {}) } };
      }
    } else return conflict("unsupported_transition");
    if (reconciled) {
      const expectedStatus = canonical.tracker.kind === "beads" && ["parked", "paused"].includes(changes.status) ? "deferred" : changes.status;
      if (task.status !== expectedStatus) return conflict("task_changed_after_interruption");
      if (state.pendingOperations) delete state.pendingOperations[request.operationId];
      return { state, result: { taskId: task.id, action: request.action, trackerFingerprint: canonical.tracker.fingerprint, reconciled: true } };
    }
    if (canonical.tracker.kind === "beads") {
      if (request.action !== "claim" && (project.setup.tracker.writerMode !== "single_owner" || project.setup.tracker.writerSessionId !== canonical.registry.projectOwner)) {
        return { status: "unavailable", reason: "verified_single_writer_required" };
      }
      if (!reconciled) {
        const nativeStatus = ["parked", "paused"].includes(changes.status) ? "deferred" : changes.status;
        const args = request.action === "claim" ? ["update", task.id, "--claim", "--actor", request.owner]
          : ["update", task.id, "--status", nativeStatus, "--actor", request.actorSessionId];
        args.push("--append-notes", marker, "--json");
        await persistIntent({ phase: "uncertain", kind: "tracker_transition", taskId: task.id, action: request.action, changes,
          intendedRuntime: state.taskRuntime?.[task.id], worktree: canonical.registry.teams.find((team) => team["team id"] === task.owner)?.worktree });
        try {
          await (options.runBeads ?? run)(project.tracker.executable ?? "bd", args, { cwd: project.root, env: beadsEnvironment(project, options.environment),
            encoding: "utf8", timeout: options.budget?.timeout(1500) ?? 1500, maxBuffer: 1024 * 1024,
            ...(options.budget ? { signal: options.budget.signal } : {}) });
        } catch { return { status: "unavailable", reason: "tracker_write_uncertain" }; }
        const observed = await loadCanonicalTracker(project, options);
        const changedTask = observed.tasks.find(({ id }) => id === task.id);
        if (observed.tracker.status !== "current" || !changedTask?.["revision / evidence"]?.includes(marker)
          || changedTask.owner !== (changes.owner ?? task.owner) || changedTask.status !== nativeStatus) return { status: "unavailable", reason: "tracker_write_uncertain" };
        if (state.pendingOperations) delete state.pendingOperations[request.operationId];
        return { state, result: { taskId: task.id, action: request.action, trackerFingerprint: observed.tracker.fingerprint } };
      }
    }
    const source = await readFile(canonical.tracker.path, "utf8");
    const changed = replaceTask(source, task.id, { ...changes, "revision / evidence": `${task["revision / evidence"] ?? ""} ${marker}`.trim() });
    if (!reconciled) await atomicWrite(canonical.tracker.path, changed, options);
    return { state, result: { taskId: task.id, action: request.action, trackerFingerprint: (await loadCanonicalTracker(project, options)).tracker.fingerprint, ...(reconciled ? { reconciled: true } : {}) } };
  }, options);
}

/** Bind recorded verification artifacts to present Git/tracker facts; never grant a gate. */
export async function recordGateEvidence(project, request, options = {}) {
  const { budget } = options;
  const bounded = (action) => budget ? budget.run(action) : action();
  return mutateOperationalState(project, request, async (state, canonicalState) => {
    if (!["completion", "integration", "release"].includes(request.gate)) return conflict("unsupported_gate");
    const canonical = await loadCanonicalTracker(project, options);
    if (canonical.tracker.status !== "current") return { status: "unavailable", reason: "tracker_unavailable" };
    if (canonical.tracker.fingerprint !== request.expectedFingerprint) return conflict("stale_tracker");
    if (!request.taskIds?.length || request.taskIds.some((id) => !canonical.tasks.some((task) => task.id === id))) return conflict("task_identity_mismatch");
    const git = (args) => run("git", args, { cwd: project.worktreeRoot, encoding: "utf8", timeout: budget?.timeout(1500) ?? 1500,
      maxBuffer: 1024 * 1024, ...(budget ? { signal: budget.signal } : {}) });
    const revision = (await git(["rev-parse", "HEAD"])).stdout.trim();
    if (revision !== request.expectedRevision) return conflict("stale_revision");
    if ((await git(["status", "--porcelain", "--untracked-files=all"])).stdout.trim()) return conflict("dirty_revision");
    const source = await bounded(() => readFile(request.evidencePath, "utf8"));
    const evidence = JSON.parse(source);
    if (evidence.status !== "passed" || evidence.revision !== revision || !Array.isArray(evidence.taskIds)
      || JSON.stringify([...evidence.taskIds].sort()) !== JSON.stringify([...request.taskIds].sort())) return conflict("evidence_mismatch");
    if (request.gate === "completion") {
      if (request.taskIds.length !== 1) return conflict("completion_task_count");
      const [taskId] = request.taskIds;
      const review = evidence.review;
      const checks = evidence.checks;
      if (evidence.requirementsReconciled !== true || review?.status !== "passed" || review.revision !== revision || review.taskId !== taskId
        || !Array.isArray(checks) || !checks.length
        || checks.some((check) => typeof check?.name !== "string" || !check.name.trim() || check.name !== check.name.trim()
          || check.status !== "passed" || check.revision !== revision || check.taskId !== taskId)) {
        return conflict("completion_evidence_mismatch");
      }
      state.completion = { ...state.completion, taskId, evidenceRevision: revision, requirementsReconciled: true,
        review: { status: "passed", revision, taskId },
        checks: checks.map(({ name, status, revision: checkRevision, taskId: checkTaskId }) => ({ name, status, revision: checkRevision, taskId: checkTaskId })),
      };
    }
    if (request.gate === "integration") {
      const remote = evidence.remote;
      const ownerSessionId = state.integration?.ownerSessionId;
      const expectedBaseRef = configuredRemoteBaseRef(state.integration?.baseRef);
      if (!remote || typeof remote !== "object" || typeof remote.name !== "string" || !remote.name.trim() || remote.name.length > 128
        || remote.baseRef !== expectedBaseRef || !ref(remote.targetRef) || typeof remote.revision !== "string" || !/^[0-9a-f]{40,64}$/i.test(remote.revision)
        || (remote.targetAbsent === true ? remote.targetRevision !== undefined : typeof remote.targetRevision !== "string" || !/^[0-9a-f]{40,64}$/i.test(remote.targetRevision))
        || ownerSessionId !== canonicalState.registry.integrationOwner
        || !provenance(evidence.authorization, { ownerSessionId, revision, taskIds: request.taskIds })
        || !recovery(evidence.recovery, { revision, taskIds: request.taskIds }) || !preview(evidence.preview, revision)
        || typeof evidence.remoteMainDeploys !== "boolean") return conflict("integration_evidence_mismatch");
      const observedAt = new Date().toISOString();
      const authorization = { source: evidence.authorization.source, scope: "integration", ownerSessionId, revision,
        taskIds: [...request.taskIds].sort(), observedAt };
      state.integration = { ...state.integration, authorized: true, expectedRevision: revision, baseRevision: remote.revision,
        remoteName: remote.name, baseRemoteRef: remote.baseRef, remoteRef: remote.targetRef,
        ...(remote.targetAbsent === true ? { targetAbsent: true, remoteRevision: undefined } : { remoteRevision: remote.targetRevision, targetAbsent: false }),
        taskIds: [...request.taskIds].sort(), authorization, evidenceAt: observedAt, deltaClean: true, recoveryReconciled: true,
        updatesRemoteMain: remote.targetRef === "refs/heads/main", remoteMainDeploys: evidence.remoteMainDeploys,
        preview: evidence.preview.required ? { required: true, approvedRevision: revision } : { required: false } };
    }
    if (request.gate === "release") {
      const ownerSessionId = state.release?.ownerSessionId;
      const authorization = evidence.authorization;
      const runRecord = evidence.run;
      const batch = evidence.batch;
      const artifact = evidence.artifact;
      const integration = evidence.integration;
      const activeIntegration = state.integration;
      const integrationReceipt = activeIntegration?.recordedEvidence;
      const verification = evidence.verification;
      const releasePreview = evidence.preview;
      const delta = evidence.delta;
      const releaseRecovery = evidence.recovery;
      const runModeMatches = evidence.runMode === runRecord?.mode && ["auto_deploy", "manual"].includes(evidence.runMode)
        && ((evidence.runMode === "auto_deploy" && evidence.autoDeploy === true)
          || (evidence.runMode === "manual" && evidence.autoDeploy === false));
      const releaseRecord = (record, status) => record?.status === status && record.revision === revision
        && sameIds(record.taskIds, request.taskIds);
      const artifactMetadataValid = artifact && typeof artifact === "object" && boundedString(artifact.id, 4096)
        && artifact.revision === revision && sameIds(artifact.taskIds, request.taskIds)
        && (artifact.path === undefined || boundedString(artifact.path, 4096))
        && (artifact.bytes === undefined || Number.isSafeInteger(artifact.bytes) && artifact.bytes > 0)
        && /^[0-9a-f]{64}$/i.test(artifact.sha256)
        && (artifact.checksumPath === undefined || boundedString(artifact.checksumPath, 4096))
        && (artifact.checksumBytes === undefined || Number.isSafeInteger(artifact.checksumBytes) && artifact.checksumBytes > 0)
        && (artifact.checksumSha256 === undefined || /^[0-9a-f]{64}$/i.test(artifact.checksumSha256))
        && (artifact.checksumEntry === undefined || boundedString(artifact.checksumEntry, 4096));
      const integrationBindingValid = releaseRecord(integration, "passed")
        && sameIds(integration.recordedTaskIds, activeIntegration?.taskIds)
        && activeIntegration?.authorized === true && activeIntegration.expectedRevision === revision
        && integration.remoteName === activeIntegration.remoteName && integration.baseRef === activeIntegration.baseRemoteRef
        && integration.baseRevision === activeIntegration.baseRevision && integration.targetRef === activeIntegration.remoteRef
        && integration.targetRevision === revision && integration.evidencePath === integrationReceipt?.path
        && integrationReceipt?.revision === revision && sameIds(integrationReceipt?.taskIds, activeIntegration.taskIds)
        && /^[0-9a-f]{64}$/i.test(integrationReceipt?.fingerprint)
        && typeof integration.remoteMainDeploys === "boolean" && integration.remoteMainDeploys === activeIntegration.remoteMainDeploys
        && (integration.deploymentTarget === undefined || boundedString(integration.deploymentTarget, 4096));
      const deltaBindingValid = releaseRecord(delta, "clean")
        && (delta.remoteBaseRevision === undefined || /^[0-9a-f]{40,64}$/i.test(delta.remoteBaseRevision));
      if (ownerSessionId !== canonicalState.registry.integrationOwner || ownerSessionId !== request.actorSessionId
        || evidence.ownerSessionId !== ownerSessionId || evidence.authorized !== true || evidence.expectedRevision !== revision
        || !boundedString(evidence.target, 1024) || !boundedString(evidence.process, 128)
        || !authorization || typeof authorization !== "object" || !boundedString(authorization.source)
        || authorization.target !== evidence.target || authorization.process !== evidence.process
        || authorization.scope !== evidence.batchId || authorization.ownerSessionId !== ownerSessionId
        || !boundedString(authorization.grantedAt, 128) || !Number.isFinite(Date.parse(authorization.grantedAt))
        || !runRecord || typeof runRecord !== "object" || !boundedString(runRecord.id, 128) || !runModeMatches
        || !sameIds(runRecord.taskIds, request.taskIds) || runRecord.paused !== false
        || !boundedString(evidence.batchId, 128) || batch?.id !== evidence.batchId || !sameIds(batch.taskIds, request.taskIds)
        || !artifactMetadataValid || !integrationBindingValid || !releaseRecord(verification, "passed")
        || typeof releasePreview?.required !== "boolean" || releasePreview.revision !== revision
        || (releasePreview.required ? releasePreview.status !== "passed" : !["not_required", "passed"].includes(releasePreview.status))
        || !deltaBindingValid || releaseRecovery?.status !== "verified"
        || !boundedString(releaseRecovery.artifactId, 4096) || !boundedString(releaseRecovery.action, 4096)
        || evidence.projectPaused !== false || canonicalState.state.run?.paused === true || evidence.hold !== false) {
        return conflict("release_evidence_mismatch");
      }
      const observedAt = new Date().toISOString();
      const taskIds = [...request.taskIds].sort();
      const copy = (record, keys) => Object.fromEntries(keys.filter((key) => record[key] !== undefined).map((key) => [key, structuredClone(record[key])]));
      state.release = {
        ownerSessionId,
        authorized: true,
        expectedRevision: revision,
        evidenceAt: observedAt,
        target: evidence.target,
        process: evidence.process,
        authorization: { source: authorization.source, target: evidence.target, process: evidence.process, scope: evidence.batchId,
          ownerSessionId, grantedAt: authorization.grantedAt, revision, taskIds, observedAt },
        run: { id: runRecord.id, mode: evidence.runMode, taskIds, paused: false },
        runMode: evidence.runMode,
        batchId: evidence.batchId,
        taskIds,
        batch: { id: evidence.batchId, taskIds },
        artifact: { ...copy(artifact, ["id", "path", "bytes", "sha256", "checksumPath", "checksumBytes", "checksumSha256", "checksumEntry"]), revision, taskIds },
        integration: { ...copy(integration, ["status", "evidencePath", "remoteName", "baseRef", "baseRevision", "targetRef", "targetRevision", "remoteMainDeploys", "deploymentTarget"]),
          revision, taskIds, recordedTaskIds: [...integration.recordedTaskIds].sort(), evidenceFingerprint: integrationReceipt.fingerprint },
        verification: { status: "passed", revision, taskIds },
        preview: { required: releasePreview.required, status: releasePreview.status, revision },
        delta: { ...copy(delta, ["status", "remoteBaseRevision"]), revision, taskIds },
        recovery: { status: "verified", artifactId: releaseRecovery.artifactId, action: releaseRecovery.action },
        recoveryReady: true,
        remoteMainDeploys: integration.remoteMainDeploys,
        autoDeploy: evidence.autoDeploy,
        projectPaused: false,
        hold: false,
      };
    }
    const current = await loadCanonicalTracker(project, options);
    if (current.tracker.status !== "current" || current.tracker.fingerprint !== canonical.tracker.fingerprint) return conflict("stale_tracker");
    state[request.gate] = { ...state[request.gate], trackerFingerprint: current.tracker.fingerprint,
      recordedEvidence: { path: request.evidencePath, fingerprint: digest(source), revision, taskIds: request.taskIds, observedAt: new Date().toISOString() } };
    return { state, result: { gate: request.gate, trackerFingerprint: current.tracker.fingerprint, revision } };
  }, options);
}
