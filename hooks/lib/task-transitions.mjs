import { createHash, randomUUID } from "node:crypto";
import { execFile } from "node:child_process";
import { promisify } from "node:util";
import { constants } from "node:fs";
import { lstat, open, readFile, readlink, realpath, rename, rm, writeFile } from "node:fs/promises";
import path from "node:path";
import os from "node:os";
import { loadCanonicalState, loadCanonicalTracker } from "./canonical-state.mjs";
import { withDirectoryLock } from "./lock.mjs";
import { inspectCheckpointEvidence } from "./recovery.mjs";
import { beadsEnvironment } from "./tracker.mjs";
import { assertNoOwnerRecoveryJournal, repairOwnerRecovery, validateNativeOwnerAuthority } from "./owner-recovery.mjs";

const digest = (value) => createHash("sha256").update(typeof value === "string" ? value : JSON.stringify(value)).digest("hex");
const operationSignature = ({ expectedVersion, expectedFingerprint, ...operation }) => digest(operation);
const run = promisify(execFile);
const conflict = (reason) => ({ status: "conflict", reason });
const finished = new Set(["verified", "integrated", "deployed", "closed", "done", "complete", "completed", "cancelled", "canceled"]);
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
const exactKeys = (value, keys) => value && typeof value === "object" && !Array.isArray(value)
  && Object.keys(value).sort().join("\0") === [...keys].sort().join("\0");
const validId = (value) => typeof value === "string" && /^[\w.:-]{1,128}$/.test(value)
  && !["none", "unknown", "unassigned", "-"].includes(value.toLowerCase());
const hex = (value, size) => typeof value === "string" && new RegExp(`^[a-f0-9]{${size}}$`).test(value);
const stable = (value) => JSON.stringify(value && typeof value === "object"
  ? Array.isArray(value) ? value.map((entry) => JSON.parse(stable(entry)))
    : Object.fromEntries(Object.keys(value).sort().map((key) => [key, JSON.parse(stable(value[key]))])) : value);
const stateFingerprint = (value) => digest(stable(value));
const runKeys = ["id", "ownerSessionId", "ownerHost", "ownershipEpoch", "mode", "taskIds", "teamLimit", "autoDeploy", "batchSize", "source",
  "settingSources", "paused", "operationalVersion", "blockers", "pendingDeliveryIds", "deployedTaskIds", "terminalClassification"];
const runSources = new Set(["explicit_run", "saved_default", "compatibility_migration"]);
const terminalKinds = new Set(["unknown", "paused", "unreconciled_completion", "progress_possible", "finite_exhausted", "continuous_scope_exhausted", "blocked_tail"]);
export function requireAdmittedTaskIds(state, taskIds) {
  const run = state?.run;
  const unique = (values) => Array.isArray(values) && new Set(values).size === values.length;
  const structurallyValid = exactKeys(run, runKeys) && validId(run.id) && validId(run.ownerSessionId)
    && ["codex", "claude-code"].includes(run.ownerHost) && Number.isSafeInteger(run.ownershipEpoch) && run.ownershipEpoch > 0
    && ["finite", "continuous"].includes(run.mode) && unique(run.taskIds) && run.taskIds.length > 0 && run.taskIds.every(validId)
    && Number.isSafeInteger(run.teamLimit) && run.teamLimit > 0 && run.teamLimit <= 64 && typeof run.autoDeploy === "boolean"
    && Number.isSafeInteger(run.batchSize) && run.batchSize > 0 && run.batchSize <= 64 && runSources.has(run.source)
    && exactKeys(run.settingSources, ["mode", "taskIds", "teamLimit", "autoDeploy", "batchSize"])
    && Object.values(run.settingSources).every((source) => runSources.has(source)) && typeof run.paused === "boolean"
    && Number.isSafeInteger(run.operationalVersion) && run.operationalVersion >= 0
    && unique(run.blockers) && unique(run.pendingDeliveryIds) && unique(run.deployedTaskIds)
    && new Set(run.blockers.map((blocker) => blocker?.taskId)).size === run.blockers.length
    && run.blockers.every((blocker) => exactKeys(blocker, ["taskId", "reason"]) && run.taskIds.includes(blocker.taskId)
      && typeof blocker.reason === "string" && blocker.reason.trim() === blocker.reason && blocker.reason.length > 0 && Buffer.byteLength(blocker.reason) <= 4096)
    && [...run.pendingDeliveryIds, ...run.deployedTaskIds].every((id) => run.taskIds.includes(id))
    && run.deployedTaskIds.every((id) => !run.pendingDeliveryIds.includes(id)) && terminalKinds.has(run.terminalClassification);
  return structurallyValid && Array.isArray(taskIds) && taskIds.length > 0 && taskIds.every((id) => run.taskIds.includes(id))
    ? undefined : "outside_scope";
}
const admittedProblem = requireAdmittedTaskIds;
const relativePath = (value) => typeof value === "string" && value.length > 0 && Buffer.byteLength(value) <= 4096
  && !path.isAbsolute(value) && path.normalize(value) === value && !value.split(/[\\/]/).includes("..") && !/[\r\n\0]/.test(value);

async function readSealedHandle(handle, { maximum = 256 * 1024, expectedUid, writableMask = 0 } = {}) {
  const before = await handle.stat();
  if (!before.isFile() || before.size > maximum || expectedUid !== undefined && before.uid !== expectedUid
    || (before.mode & writableMask) !== 0) throw new Error("unsafe_evidence_file");
  const bytes = Buffer.alloc(before.size);
  let offset = 0;
  while (offset < bytes.length) {
    const { bytesRead } = await handle.read(bytes, offset, bytes.length - offset, offset);
    if (!bytesRead) break;
    offset += bytesRead;
  }
  const after = await handle.stat();
  if (offset !== bytes.length || before.dev !== after.dev || before.ino !== after.ino || before.size !== after.size
    || before.mtimeMs !== after.mtimeMs) throw new Error("evidence_changed_during_read");
  return { bytes, parsed: JSON.parse(bytes.toString("utf8")), sha256: createHash("sha256").update(bytes).digest("hex"), stat: after };
}

async function readSealed(file, { maximum = 256 * 1024, expectedUid } = {}) {
  let handle;
  try {
    handle = await open(file, constants.O_RDONLY | constants.O_NOFOLLOW | constants.O_NONBLOCK);
    return await readSealedHandle(handle, { maximum, expectedUid });
  } finally { await handle?.close(); }
}

function bindTaskDeliveryReceipts(state, gate, taskIds, revision, evidence, recordedEvidence) {
  const categories = ["completion", "review", "checks", "integration", "preview", "target", "recovery"];
  state.deliveryReceipts ??= Object.fromEntries(categories.map((category) => [category, {}]));
  for (const category of categories) state.deliveryReceipts[category] ??= {};
  const put = (category, taskId, value) => { state.deliveryReceipts[category][taskId] = { taskId, ...value }; };
  for (const taskId of taskIds) {
    if (gate === "completion") {
      put("completion", taskId, { status: "passed", sourceRevision: revision, evidence: recordedEvidence });
      put("review", taskId, { status: "passed", revision, evidence: recordedEvidence });
      put("checks", taskId, { revision, results: evidence.checks.map(({ name, status }) => ({ name, status })), evidence: recordedEvidence });
    } else if (gate === "integration") {
      const sourceRevision = evidence.sourceRevisions?.[taskId];
      if (!hex(sourceRevision, 40) || state.deliveryReceipts.completion?.[taskId]?.sourceRevision !== sourceRevision) throw new Error("integration_source_revision_mismatch");
      const integrationActor = { ownerHost: state.integration.ownerHost, ownerSessionId: state.integration.ownerSessionId, ownershipEpoch: state.integration.ownershipEpoch };
      const releaseActor = { ownerHost: state.release.ownerHost, ownerSessionId: state.release.ownerSessionId, ownershipEpoch: state.release.ownershipEpoch };
      put("integration", taskId, { status: "passed", sourceRevision, boundaryRevision: revision, integratedRevision: revision,
        integrationOperationId: recordedEvidence.operationId, ...integrationActor, evidence: recordedEvidence });
      put("preview", taskId, { revision, required: evidence.preview.required, status: evidence.preview.required ? "passed" : "not_required",
        ...integrationActor, evidence: recordedEvidence });
      put("target", taskId, { revision, status: "authorized", target: evidence.targetAuthorization.target,
        authority: structuredClone(evidence.targetAuthorization), ...releaseActor, evidence: recordedEvidence });
      put("recovery", taskId, { revision, status: "ready", artifact: evidence.recovery.artifactId, action: evidence.recovery.action,
        ...releaseActor, evidence: recordedEvidence });
    }
  }
}

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
    if (!await validateNativeOwnerAuthority(project, options.nativeIdentity, project.setup?.ownership, request.actorSessionId)) {
      return conflict("project_owner_required");
    }
    await repairOwnerRecovery(project, { budget });
    const execute = () => withDirectoryLock(path.join(project.paths.locks, "state.lock"), {
      operationId: request.operationId, actorSessionId: request.actorSessionId, pid: process.pid,
    }, async () => {
      await assertNoOwnerRecoveryJournal(project);
      const canonical = await loadCanonicalState(project, { includeTasks: false, includeDeliveryEvidence: false, budget });
      const owner = canonical.registry.projectOwner;
      const actorHost = options.nativeIdentity?.host === "claude" ? "claude-code" : options.nativeIdentity?.host;
      if (typeof owner !== "string" || !/^[\w.:-]{1,128}$/.test(owner) || ["none", "unknown", "unassigned", "-"].includes(owner.toLowerCase())
        || typeof request.actorSessionId !== "string" || !/^[\w.:-]{1,128}$/.test(request.actorSessionId)
        || request.actorSessionId !== owner || canonical.registry.projectOwnerHost && (actorHost !== canonical.registry.projectOwnerHost
          || options.nativeIdentity?.observed !== true || options.nativeIdentity?.sessionId !== owner
          || options.nativeIdentity?.ownershipEpoch !== canonical.registry.ownershipEpoch)
        || !canonical.registry.projectId || canonical.registry.projectId !== project.projectId) return conflict("project_owner_required");
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
        state.pendingOperations = { ...state.pendingOperations, [request.operationId]: { ...intent, signature, ownerSessionId: request.actorSessionId,
          ...(canonical.registry.projectOwnerHost ? { ownerHost: actorHost, ownershipEpoch: canonical.registry.ownershipEpoch } : {}) } };
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
    if (admittedProblem(state, request.taskIds)) return conflict("outside_scope");
    const canonical = await loadCanonicalTracker(project, options);
    if (canonical.tracker.status !== "current") return { status: "unavailable", reason: "tracker_unavailable" };
    if (canonical.tracker.fingerprint !== request.expectedFingerprint) return conflict("stale_tracker");
    if (!request.taskIds?.length || request.taskIds.some((id) => !canonical.tasks.some((task) => task.id === id))) return conflict("task_identity_mismatch");
    const git = (args) => run("git", args, { cwd: project.worktreeRoot, encoding: "utf8", timeout: budget?.timeout(1500) ?? 1500,
      maxBuffer: 1024 * 1024, ...(budget ? { signal: budget.signal } : {}) });
    const revision = (await git(["rev-parse", "HEAD"])).stdout.trim();
    if (revision !== request.expectedRevision) return conflict("stale_revision");
    if ((await git(["status", "--porcelain", "--untracked-files=all"])).stdout.trim()) return conflict("dirty_revision");
    const sealed = await bounded(() => readSealed(request.evidencePath));
    const source = sealed.bytes;
    const evidence = sealed.parsed;
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
      if (validId(state.run?.id) && state.ownership) {
        const authority = evidence.targetAuthorization;
        const authorityKeys = ["status", "source", "target", "revision", "taskIds", "ownerHost", "ownerSessionId", "ownershipEpoch"];
        const expectedTaskIds = [...request.taskIds].sort();
        if (!evidence.sourceRevisions || !exactKeys(evidence.sourceRevisions, request.taskIds)
          || !exactKeys(authority, authorityKeys) || authority.status !== "authorized" || !boundedString(authority.source)
          || authority.target !== remote.targetRef || authority.revision !== revision || stable(authority.taskIds) !== stable(expectedTaskIds)
          || authority.ownerHost !== state.release?.ownerHost || authority.ownerSessionId !== state.release?.ownerSessionId
          || authority.ownershipEpoch !== state.release?.ownershipEpoch || !boundedString(evidence.recovery.artifactId, 4096)
          || !boundedString(evidence.recovery.action, 4096)) return conflict("integration_delivery_evidence_mismatch");
      }
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
      const ownerHost = state.release?.ownerHost;
      const ownershipEpoch = state.release?.ownershipEpoch;
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
        || canonicalState.registry.projectOwnerHost && (ownerHost !== canonicalState.registry.projectOwnerHost
          || ownershipEpoch !== canonicalState.registry.ownershipEpoch)
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
        ...(canonicalState.registry.projectOwnerHost ? { ownerHost, ownershipEpoch } : {}),
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
    const observedAt = new Date().toISOString();
    const recordedEvidence = { path: request.evidencePath, fingerprint: createHash("sha256").update(source).digest("hex"), revision,
      taskIds: [...request.taskIds], operationId: request.operationId, observedAt };
    if (["completion", "integration"].includes(request.gate) && validId(state.run?.id) && state.ownership) {
      bindTaskDeliveryReceipts(state, request.gate, request.taskIds, revision, evidence, recordedEvidence);
    }
    state[request.gate] = { ...state[request.gate], trackerFingerprint: canonical.tracker.fingerprint, recordedEvidence };
    return { state, result: { gate: request.gate, trackerFingerprint: canonical.tracker.fingerprint, revision } };
  }, options);
}

function mutationRequest(request, options) {
  return { ...request, actorSessionId: options.actorSessionId, expectedVersion: options.expectedVersion };
}

function currentActor(state) {
  return { ownerHost: state.ownership?.current?.host, ownerSessionId: state.ownership?.current?.sessionId, ownershipEpoch: state.ownership?.epoch };
}

function completionIdentity(value) {
  return value && { path: value.path, fingerprint: value.fingerprint, revision: value.revision, taskIds: value.taskIds };
}

function completionEvidenceValid(evidence, taskId, revision) {
  return evidence?.status === "passed" && evidence.revision === revision && sameIds(evidence.taskIds ?? [evidence.taskId], [taskId])
    && evidence.requirementsReconciled === true && evidence.review?.status === "passed"
    && (evidence.review.revision === undefined || evidence.review.revision === revision)
    && Array.isArray(evidence.checks) && evidence.checks.length > 0
    && evidence.checks.every((check) => boundedString(check?.name) && check.status === "passed"
      && (check.revision === undefined || check.revision === revision));
}

async function gitObservation(project, options = {}) {
  const git = options.runGit ?? ((args) => run("git", args, { cwd: project.root, encoding: "utf8", timeout: options.budget?.timeout(1500) ?? 1500,
    maxBuffer: 1024 * 1024, ...(options.budget ? { signal: options.budget.signal } : {}) }));
  const output = (value) => typeof value === "string" ? value : value.stdout;
  const revision = output(await git(["rev-parse", "HEAD"])).trim();
  const clean = !output(await git(["status", "--porcelain", "--untracked-files=all"])).trim();
  return { revision, clean, git };
}

async function ancestor(git, left, right) {
  try { await git(["merge-base", "--is-ancestor", left, right]); return true; }
  catch { return false; }
}

export async function quarantineCompletion(project, request, options = {}) {
  if (!exactKeys(request, ["operationId", "taskId", "reason", "expectedEvidence"]) || !validId(request?.operationId) || !validId(request?.taskId)
    || request.reason !== "task_outside_admitted_run_scope" || !exactKeys(request.expectedEvidence, ["path", "fingerprint", "revision", "taskIds"])
    || !boundedString(request.expectedEvidence.path, 4096) || !hex(request.expectedEvidence.fingerprint, 64)
    || !hex(request.expectedEvidence.revision, 40) || !sameIds(request.expectedEvidence.taskIds, [request.taskId])) return conflict("invalid_request");
  return mutateOperationalState(project, mutationRequest(request, options), async (state) => {
    if (state.run?.paused) return conflict("paused");
    if (state.run?.taskIds?.includes(request.taskId)) return conflict("quarantine_scope_mismatch");
    if (state.completion?.taskId !== request.taskId
      || stable(completionIdentity(state.completion.recordedEvidence)) !== stable(request.expectedEvidence)) return conflict("completion_evidence_changed");
    const tracker = await loadCanonicalTracker(project, options);
    if (tracker.tracker.status !== "current") return { status: "unavailable", reason: "tracker_unavailable" };
    if (!tracker.tasks.some((task) => task.id === request.taskId)) return conflict("quarantine_scope_mismatch");
    const sealed = await readSealed(request.expectedEvidence.path);
    if (sealed.sha256 !== request.expectedEvidence.fingerprint || sealed.parsed.revision !== request.expectedEvidence.revision
      || !sameIds(sealed.parsed.taskIds ?? [sealed.parsed.taskId], [request.taskId])) return conflict("completion_evidence_changed");
    const quarantineId = `completion:${request.taskId}:${request.expectedEvidence.fingerprint}`;
    const { recordedEvidence: _removed, ...completion } = state.completion;
    if (state.deliveryReceipts?.completion) delete state.deliveryReceipts.completion[request.taskId];
    const quarantinedAt = (options.now ?? (() => new Date().toISOString()))();
    const record = { quarantineId, taskId: request.taskId, evidence: structuredClone(request.expectedEvidence),
      historicalOwnerSessionId: state.completion.ownerSessionId ?? null, historicalOperationId: state.completion.operationId ?? state.completion.recordedEvidence.operationId ?? null,
      authenticatedActor: currentActor(state), reason: request.reason, quarantinedAt };
    return { state: { ...state, completion, quarantinedEvidence: [...(state.quarantinedEvidence ?? []), record] },
      result: { quarantineId, taskId: request.taskId, reason: request.reason, evidence: request.expectedEvidence, activeCompletionCleared: true } };
  }, options);
}

export async function rebindCompletion(project, request, options = {}) {
  if (!exactKeys(request, ["operationId", "taskId", "quarantineId", "expectedTrackerFingerprint", "evidencePath", "expectedEvidenceFingerprint",
    "expectedSourceRevision", "expectedBoundaryRevision"]) || ![request?.operationId, request?.taskId, request?.quarantineId].every(validId)
    || !hex(request.expectedTrackerFingerprint, 64) || !boundedString(request.evidencePath, 4096) || !hex(request.expectedEvidenceFingerprint, 64)
    || !hex(request.expectedSourceRevision, 40) || !hex(request.expectedBoundaryRevision, 40)) return conflict("invalid_request");
  return mutateOperationalState(project, mutationRequest(request, options), async (state) => {
    if (admittedProblem(state, [request.taskId])) return conflict("outside_scope");
    if (state.run?.paused) return conflict("paused");
    const tracker = await loadCanonicalTracker(project, options);
    if (tracker.tracker.status !== "current") return { status: "unavailable", reason: "tracker_unavailable" };
    if (tracker.tracker.fingerprint !== request.expectedTrackerFingerprint) return conflict("stale_tracker");
    if (!tracker.tasks.some((task) => task.id === request.taskId)) return conflict("task_identity_mismatch");
    const quarantined = state.quarantinedEvidence?.find((entry) => entry.quarantineId === request.quarantineId);
    const wanted = { path: request.evidencePath, fingerprint: request.expectedEvidenceFingerprint, revision: request.expectedSourceRevision, taskIds: [request.taskId] };
    if (!quarantined || quarantined.reboundByOperationId || stable(quarantined.evidence) !== stable(wanted)) return conflict("invalid_quarantine_identity");
    const sealed = await readSealed(request.evidencePath);
    if (sealed.sha256 !== request.expectedEvidenceFingerprint || !completionEvidenceValid(sealed.parsed, request.taskId, request.expectedSourceRevision)) return conflict("completion_evidence_changed");
    const integration = state.deliveryReceipts?.integration?.[request.taskId];
    if (integration?.status !== "passed" || integration.sourceRevision !== request.expectedSourceRevision
      || integration.boundaryRevision !== request.expectedBoundaryRevision || integration.integratedRevision !== request.expectedBoundaryRevision) return conflict("integration_evidence_mismatch");
    const repository = await gitObservation(project, options);
    if (!repository.clean || !await ancestor(repository.git, request.expectedSourceRevision, request.expectedBoundaryRevision)
      || !await ancestor(repository.git, request.expectedBoundaryRevision, repository.revision)) return conflict("invalid_completion_lineage");
    const observedAt = (options.now ?? (() => new Date().toISOString()))();
    const pointer = { path: request.evidencePath, fingerprint: request.expectedEvidenceFingerprint, revision: request.expectedSourceRevision,
      taskIds: [request.taskId], operationId: request.operationId, observedAt };
    state.deliveryReceipts ??= {};
    state.deliveryReceipts.completion = { ...(state.deliveryReceipts.completion ?? {}), [request.taskId]: { taskId: request.taskId, status: "passed",
      sourceRevision: request.expectedSourceRevision, evidence: pointer } };
    state.deliveryReceipts.review = { ...(state.deliveryReceipts.review ?? {}), [request.taskId]: { taskId: request.taskId, status: "passed",
      revision: request.expectedSourceRevision, evidence: pointer } };
    state.deliveryReceipts.checks = { ...(state.deliveryReceipts.checks ?? {}), [request.taskId]: { taskId: request.taskId, revision: request.expectedSourceRevision,
      results: sealed.parsed.checks.map(({ name, status }) => ({ name, status })), evidence: pointer } };
    const completion = state.completion?.recordedEvidence && state.completion.taskId !== request.taskId ? state.completion
      : { ...state.completion, taskId: request.taskId, evidenceRevision: request.expectedSourceRevision, requirementsReconciled: true,
        review: { status: "passed", revision: request.expectedSourceRevision, taskId: request.taskId },
        checks: sealed.parsed.checks.map(({ name, status }) => ({ name, status, revision: request.expectedSourceRevision, taskId: request.taskId })), recordedEvidence: pointer };
    state.quarantinedEvidence = state.quarantinedEvidence.map((entry) => entry.quarantineId === request.quarantineId
      ? { ...entry, reboundByOperationId: request.operationId, reboundAt: observedAt } : entry);
    return { state: { ...state, completion }, result: { taskId: request.taskId, quarantineId: request.quarantineId,
      sourceRevision: request.expectedSourceRevision, boundaryRevision: request.expectedBoundaryRevision, currentRevision: repository.revision,
      evidenceFingerprint: request.expectedEvidenceFingerprint, trackerFingerprint: tracker.tracker.fingerprint, rebound: true } };
  }, options);
}

async function inspectDirectoryNoFollow(directory) {
  const resolved = await realpath(directory);
  if (resolved !== directory) throw new Error("evidence_store_not_canonical");
  const root = path.parse(directory).root;
  let current = root;
  for (const part of path.relative(root, directory).split(path.sep).filter(Boolean)) {
    current = path.join(current, part);
    const metadata = await lstat(current);
    if (!metadata.isDirectory() || metadata.isSymbolicLink()) throw new Error("unsafe_evidence_store");
  }
  return lstat(directory);
}

export async function registerEvidenceStore(project, request, options = {}) {
  if (!exactKeys(request, ["operationId", "storeId", "realpath", "expectedTeamsFingerprint", "declaration", "projectId", "expectedOwnershipEpoch"])
    || ![request?.operationId, request?.storeId, request?.projectId].every(validId) || !path.isAbsolute(request?.realpath ?? "")
    || path.normalize(request.realpath) !== request.realpath || !hex(request.expectedTeamsFingerprint, 64) || !boundedString(request.declaration, 4096)
    || !Number.isSafeInteger(request.expectedOwnershipEpoch) || request.expectedOwnershipEpoch < 1) return conflict("invalid_request");
  return mutateOperationalState(project, mutationRequest(request, options), async (state, canonical) => {
    const teams = canonical.sources?.teams;
    if (request.projectId !== canonical.registry.projectId || request.expectedOwnershipEpoch !== canonical.registry.ownershipEpoch) return conflict("stale_owner_generation");
    if (typeof teams !== "string" || digest(teams) !== request.expectedTeamsFingerprint) return conflict("stale_teams");
    const declaration = `Evidence root: ${request.realpath}/<team>/.`;
    if (request.declaration !== declaration || teams.split(/\r?\n/).filter((line) => line === declaration).length !== 1) return conflict("evidence_store_not_declared");
    let metadata;
    try { metadata = await inspectDirectoryNoFollow(request.realpath); } catch { return conflict("unsafe_evidence_store"); }
    if (typeof process.getuid === "function" && metadata.uid !== process.getuid()) return conflict("evidence_store_owner_mismatch");
    if ((metadata.mode & 0o002) !== 0) return conflict("unsafe_evidence_store_mode");
    if (state.evidenceStores?.[request.storeId]) return conflict("evidence_store_already_registered");
    const registeredAt = (options.now ?? (() => new Date().toISOString()))();
    const store = { storeId: request.storeId, realpath: request.realpath, projectId: request.projectId, uid: metadata.uid, gid: metadata.gid,
      mode: metadata.mode & 0o7777, dev: metadata.dev, ino: metadata.ino, teamsFingerprint: request.expectedTeamsFingerprint,
      declaration: request.declaration, registeredBy: currentActor(state), registeredAt };
    return { state: { ...state, evidenceStores: { ...(state.evidenceStores ?? {}), [request.storeId]: store } }, result: { store: structuredClone(store) } };
  }, options);
}

const withinPath = (root, target) => target === root || target.startsWith(`${root}${path.sep}`);

async function readRegisteredEvidence(store, relative, expectedSha256, options = {}) {
  if (!relativePath(relative)) throw new Error("unsafe_evidence_path");
  const openPath = options.filesystem?.open ?? open;
  const resolvePath = options.filesystem?.realpath ?? realpath;
  const handles = [];
  try {
    const directoryFlags = constants.O_RDONLY | constants.O_DIRECTORY | constants.O_NOFOLLOW | constants.O_NONBLOCK;
    let directory = await openPath(store.realpath, directoryFlags); handles.push(directory);
    const rootMetadata = await directory.stat();
    if (!rootMetadata.isDirectory() || ["uid", "gid", "dev", "ino"].some((field) => rootMetadata[field] !== store[field])
      || (rootMetadata.mode & 0o7777) !== store.mode) throw new Error("evidence_store_changed");
    const parts = relative.split(/[\\/]/);
    for (const part of parts.slice(0, -1)) {
      directory = await openPath(`/proc/self/fd/${directory.fd}/${part}`, directoryFlags); handles.push(directory);
      const metadata = await directory.stat();
      if (!metadata.isDirectory() || metadata.uid !== store.uid) throw new Error("unsafe_evidence_path");
    }
    const evidenceHandle = await openPath(`/proc/self/fd/${directory.fd}/${parts.at(-1)}`,
      constants.O_RDONLY | constants.O_NOFOLLOW | constants.O_NONBLOCK); handles.push(evidenceHandle);
    const sealed = await readSealedHandle(evidenceHandle, { expectedUid: store.uid, writableMask: 0o022 });
    const resolvedRoot = await resolvePath(`/proc/self/fd/${handles[0].fd}`);
    const resolvedFile = await resolvePath(`/proc/self/fd/${evidenceHandle.fd}`);
    if (resolvedRoot !== store.realpath || !withinPath(resolvedRoot, resolvedFile)) {
      throw new Error("evidence_store_changed");
    }
    for (const handle of handles.slice(1, -1)) {
      if (!withinPath(resolvedRoot, await resolvePath(`/proc/self/fd/${handle.fd}`))) throw new Error("evidence_store_changed");
    }
    if (sealed.sha256 !== expectedSha256) throw new Error("evidence_changed_during_read");
    return sealed;
  } finally {
    await Promise.allSettled(handles.reverse().map((handle) => handle.close()));
  }
}

function activeCompletionProjection(state) {
  if (!state.completion?.taskId || !state.completion?.operationId || !state.completion?.resultFingerprint) return null;
  return { taskId: state.completion.taskId, operationId: state.completion.operationId, resultFingerprint: state.completion.resultFingerprint };
}

function strictAddition(state, tracker, taskId) {
  if (!state.run || state.run.taskIds.includes(taskId)) return "scope_extension_not_strict_addition";
  const task = tracker.tasks.find((entry) => entry.id === taskId);
  if (!task || task.isEpic || task.isSubtask || task.hierarchyUnknown) return "scope_extension_not_strict_addition";
  const admitted = new Set([...state.run.taskIds, taskId]);
  if (split(task.dependencies ?? task["depends on"]).some((id) => !admitted.has(id)
    && !finished.has(String(tracker.tasks.find((entry) => entry.id === id)?.status).toLowerCase()))) return "unresolved_scope_dependency";
  return undefined;
}

export async function reconcileCompletionHistory(project, request, options = {}) {
  const keys = ["operationId", "taskId", "intent", "completionOperationId", "completionResultFingerprint", "evidenceStoreId", "completionRelativePath",
    "completionSha256", "sourceRevision", "integrationOperationId", "integrationRelativePath", "integrationSha256", "boundaryRevision",
    "expectedTrackerFingerprint", "expectedStateFingerprint", "expectedRunFingerprint", "expectedTeamsFingerprint", "expectedOwnershipEpoch",
    "expectedActiveCompletion", "publicationTarget", "observedRemoteRevision"];
  if (!exactKeys(request, keys) || ![request?.operationId, request?.taskId, request?.completionOperationId, request?.integrationOperationId,
    request?.evidenceStoreId].every(validId) || !["admit_and_record", "record_existing_scope", "decline_admission"].includes(request.intent)
    || ![request.completionResultFingerprint, request.completionSha256, request.integrationSha256, request.expectedTrackerFingerprint,
      request.expectedStateFingerprint, request.expectedRunFingerprint, request.expectedTeamsFingerprint].every((value) => hex(value, 64))
    || ![request.sourceRevision, request.boundaryRevision, request.observedRemoteRevision].every((value) => hex(value, 40))
    || !relativePath(request.completionRelativePath) || !relativePath(request.integrationRelativePath)
    || !Number.isSafeInteger(request.expectedOwnershipEpoch) || request.expectedOwnershipEpoch < 1
    || request.publicationTarget !== "origin:refs/heads/main"
    || request.expectedActiveCompletion !== null && (!exactKeys(request.expectedActiveCompletion, ["taskId", "operationId", "resultFingerprint"])
      || !validId(request.expectedActiveCompletion.taskId) || !validId(request.expectedActiveCompletion.operationId) || !hex(request.expectedActiveCompletion.resultFingerprint, 64))) {
    return conflict("invalid_request");
  }
  return mutateOperationalState(project, mutationRequest(request, options), async (state, canonical) => {
    if (request.intent === "record_existing_scope" && admittedProblem(state, [request.taskId])) return conflict("outside_scope");
    if (state.run?.paused) return conflict("paused");
    if (stateFingerprint(canonical.state) !== request.expectedStateFingerprint) return conflict("stale_state");
    const runFingerprint = state.run ? stateFingerprint(state.run) : null;
    if (runFingerprint !== request.expectedRunFingerprint) return conflict("stale_run");
    if (digest(canonical.sources?.teams ?? "") !== request.expectedTeamsFingerprint) return conflict("stale_teams");
    if (canonical.registry.ownershipEpoch !== request.expectedOwnershipEpoch) return conflict("stale_owner_generation");
    if (stable(activeCompletionProjection(state)) !== stable(request.expectedActiveCompletion)) return conflict("active_completion_changed");
    if (Object.entries(state.pendingOperations ?? {}).some(([id, entry]) => id !== request.operationId
      && ["uncertain", "pending", "unknown"].includes(entry.phase ?? entry.status))) return { status: "unavailable", reason: "pending_operation_unresolved" };
    const tracker = await loadCanonicalTracker(project, options);
    if (tracker.tracker.status !== "current") return { status: "unavailable", reason: "tracker_unavailable" };
    if (tracker.tracker.fingerprint !== request.expectedTrackerFingerprint) return conflict("stale_tracker");
    if (request.intent === "admit_and_record") {
      const problem = strictAddition(state, tracker, request.taskId); if (problem) return conflict(problem);
    } else if (request.intent !== "decline_admission" && !tracker.tasks.some((task) => task.id === request.taskId)) return conflict("task_identity_mismatch");
    const observedAt = (options.now ?? (() => new Date().toISOString()))();
    if (request.intent === "decline_admission") {
      const history = { operationId: request.operationId, taskId: request.taskId, intent: request.intent, authenticatedActor: currentActor(state), observedAt };
      return { state: { ...state, completionHistory: [...(state.completionHistory ?? []), history] }, result: history };
    }
    const store = state.evidenceStores?.[request.evidenceStoreId];
    if (!store) return conflict("unregistered_evidence_store");
    let completionSealed; let integrationSealed;
    try {
      completionSealed = await readRegisteredEvidence(store, request.completionRelativePath, request.completionSha256, options);
      integrationSealed = request.integrationRelativePath === request.completionRelativePath
        ? completionSealed : await readRegisteredEvidence(store, request.integrationRelativePath, request.integrationSha256, options);
      if (integrationSealed.sha256 !== request.integrationSha256) throw new Error("evidence_changed_during_read");
    } catch { return conflict("historical_evidence_changed"); }
    const completionEvidence = completionSealed.parsed;
    const integrationEvidence = integrationSealed.parsed;
    if (!completionEvidenceValid(completionEvidence, request.taskId, request.sourceRevision)
      || completionEvidence.operationId !== request.completionOperationId || completionEvidence.resultFingerprint !== request.completionResultFingerprint
      || integrationEvidence?.status !== "passed" || integrationEvidence.revision !== request.boundaryRevision
      || integrationEvidence.operationId !== request.integrationOperationId
      || !sameIds(integrationEvidence.taskIds, [request.taskId]) && !integrationEvidence.taskIds?.includes(request.taskId)
      || integrationEvidence.sourceRevisions?.[request.taskId] !== request.sourceRevision) return conflict("historical_evidence_mismatch");
    const task = tracker.tasks.find((entry) => entry.id === request.taskId);
    if (!task || !finished.has(String(task.status).toLowerCase())) return conflict("historical_task_not_complete");
    const repository = await gitObservation(project, options);
    if (!repository.clean || !await ancestor(repository.git, request.sourceRevision, request.boundaryRevision)
      || !await ancestor(repository.git, request.boundaryRevision, repository.revision)) return conflict("invalid_completion_lineage");
    if (!options.observePublicationTarget) return { status: "unavailable", reason: "publication_observation_unavailable" };
    const separator = request.publicationTarget.indexOf(":");
    const publicationRemote = request.publicationTarget.slice(0, separator);
    const publicationRef = request.publicationTarget.slice(separator + 1);
    if (!validId(publicationRemote) || !ref(publicationRef)) return conflict("invalid_publication_target");
    const publication = await options.observePublicationTarget({ project, target: request.publicationTarget });
    const remoteRevision = typeof publication === "string" ? publication : publication?.revision;
    if (publication?.target !== undefined && publication.target !== request.publicationTarget
      || remoteRevision !== request.observedRemoteRevision || !await ancestor(repository.git, request.boundaryRevision, remoteRevision)) return conflict("publication_observation_changed");
    const actor = currentActor(state);
    const integrationActor = { ownerHost: state.integration?.ownerHost, ownerSessionId: state.integration?.ownerSessionId,
      ownershipEpoch: state.integration?.ownershipEpoch };
    const releaseActor = { ownerHost: state.release?.ownerHost, ownerSessionId: state.release?.ownerSessionId,
      ownershipEpoch: state.release?.ownershipEpoch };
    const targetAuthority = integrationEvidence.targetAuthorization;
    if (!exactKeys(targetAuthority, ["status", "source", "target", "revision", "taskIds", "ownerHost", "ownerSessionId", "ownershipEpoch"])
      || targetAuthority.status !== "authorized" || targetAuthority.revision !== request.boundaryRevision
      || targetAuthority.target !== publicationRef || !boundedString(targetAuthority.source)
      || !sameIds(targetAuthority.taskIds, integrationEvidence.taskIds) || !targetAuthority.taskIds.includes(request.taskId)
      || targetAuthority.ownerHost !== releaseActor.ownerHost || targetAuthority.ownerSessionId !== releaseActor.ownerSessionId
      || targetAuthority.ownershipEpoch !== releaseActor.ownershipEpoch || !exactKeys(integrationEvidence.preview, ["required"])
      || typeof integrationEvidence.preview.required !== "boolean"
      || !exactKeys(integrationEvidence.recovery, ["artifactId", "action"])
      || !boundedString(integrationEvidence.recovery.artifactId, 4096) || !boundedString(integrationEvidence.recovery.action, 4096)) {
      return conflict("historical_evidence_mismatch");
    }
    const completionPointer = { evidenceStoreId: request.evidenceStoreId, relativePath: request.completionRelativePath, sha256: request.completionSha256,
      operationId: request.completionOperationId, observedAt };
    const integrationPointer = { evidenceStoreId: request.evidenceStoreId, relativePath: request.integrationRelativePath, sha256: request.integrationSha256,
      operationId: request.integrationOperationId, observedAt };
    const previousRunFingerprint = stateFingerprint(state.run);
    if (request.intent === "admit_and_record") state.run = { ...state.run, taskIds: [...state.run.taskIds, request.taskId] };
    state.deliveryReceipts ??= Object.fromEntries(["completion", "review", "checks", "integration", "preview", "target", "recovery"].map((category) => [category, {}]));
    for (const category of ["completion", "review", "checks", "integration", "preview", "target", "recovery"]) state.deliveryReceipts[category] ??= {};
    state.deliveryReceipts.completion[request.taskId] = { taskId: request.taskId, status: "passed", sourceRevision: request.sourceRevision,
      completionOperationId: request.completionOperationId, completionResultFingerprint: request.completionResultFingerprint, evidence: completionPointer };
    state.deliveryReceipts.review[request.taskId] = { taskId: request.taskId, status: "passed", revision: request.sourceRevision, evidence: completionPointer };
    state.deliveryReceipts.checks[request.taskId] = { taskId: request.taskId, revision: request.sourceRevision,
      results: completionEvidence.checks.map(({ name, status }) => ({ name, status })), evidence: completionPointer };
    state.deliveryReceipts.integration[request.taskId] = { taskId: request.taskId, status: "passed", sourceRevision: request.sourceRevision,
      boundaryRevision: request.boundaryRevision, integratedRevision: request.boundaryRevision, integrationOperationId: request.integrationOperationId,
      ...integrationActor, evidence: integrationPointer };
    state.deliveryReceipts.preview[request.taskId] = { taskId: request.taskId, revision: request.boundaryRevision, required: integrationEvidence.preview.required,
      status: integrationEvidence.preview.required ? "passed" : "not_required", ...integrationActor, evidence: integrationPointer };
    state.deliveryReceipts.target[request.taskId] = { taskId: request.taskId, revision: request.boundaryRevision, status: "authorized",
      target: targetAuthority.target, authority: structuredClone(targetAuthority), ...releaseActor, evidence: integrationPointer };
    state.deliveryReceipts.recovery[request.taskId] = { taskId: request.taskId, revision: request.boundaryRevision, status: "ready",
      artifact: integrationEvidence.recovery.artifactId, action: integrationEvidence.recovery.action, ...releaseActor, evidence: integrationPointer };
    state.run.deployedTaskIds = [...new Set([...(state.run.deployedTaskIds ?? []), request.taskId])];
    state.run.pendingDeliveryIds = (state.run.pendingDeliveryIds ?? []).filter((id) => id !== request.taskId);
    const history = { operationId: request.operationId, taskId: request.taskId, intent: request.intent, completionOperationId: request.completionOperationId,
      completionResultFingerprint: request.completionResultFingerprint, integrationOperationId: request.integrationOperationId,
      completionEvidence: completionPointer, integrationEvidence: integrationPointer, sourceRevision: request.sourceRevision,
      boundaryRevision: request.boundaryRevision, currentRevision: repository.revision, publication: { target: request.publicationTarget, revision: remoteRevision, status: "published" },
      authenticatedActor: actor, activeCompletion: structuredClone(request.expectedActiveCompletion), previousRunFingerprint, runFingerprint: stateFingerprint(state.run), previousTaskIds: request.intent === "admit_and_record"
        ? state.run.taskIds.slice(0, -1) : [...state.run.taskIds], taskIds: [...state.run.taskIds], resultingStateVersion: (canonical.state.stateVersion ?? 0) + 1, observedAt };
    state.completionHistory = [...(state.completionHistory ?? []), history];
    return { state, result: { taskId: request.taskId, intent: request.intent, sourceRevision: request.sourceRevision,
      boundaryRevision: request.boundaryRevision, currentRevision: repository.revision, published: true } };
  }, options);
}
