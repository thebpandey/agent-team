import { createHash } from "node:crypto";
import { loadCanonicalState, loadCanonicalTracker } from "./canonical-state.mjs";
import { validateNativeOwnerAuthority } from "./owner-recovery.mjs";
import { mutateOperationalState } from "./task-transitions.mjs";

const validId = (value) => typeof value === "string" && /^[\w.:-]{1,128}$/.test(value)
  && !["none", "unknown", "unassigned", "-"].includes(value.toLowerCase());
const stable = (value) => JSON.stringify(value && typeof value === "object"
  ? Array.isArray(value) ? value.map((entry) => JSON.parse(stable(entry)))
    : Object.fromEntries(Object.keys(value).sort().map((key) => [key, JSON.parse(stable(value[key]))])) : value);
const exactKeys = (value, keys) => value && typeof value === "object" && !Array.isArray(value)
  && stable(Object.keys(value).sort()) === stable([...keys].sort());
const sources = new Set(["explicit_run", "saved_default", "compatibility_migration"]);
const runKeys = ["id", "ownerSessionId", "ownerHost", "ownershipEpoch", "mode", "taskIds", "teamLimit", "autoDeploy", "batchSize", "source",
  "settingSources", "paused", "operationalVersion", "blockers", "pendingDeliveryIds", "deployedTaskIds", "terminalClassification"];
const proposalKeys = ["id", "mode", "taskIds", "teamLimit", "autoDeploy", "batchSize", "source", "settingSources"];
const sourceKeys = ["mode", "taskIds", "teamLimit", "autoDeploy", "batchSize"];
const terminalKinds = new Set(["unknown", "paused", "unreconciled_completion", "progress_possible", "finite_exhausted", "continuous_scope_exhausted", "blocked_tail"]);
const completed = new Set(["verified", "integrated", "deployed", "closed", "done", "completed", "complete"]);
const cancelled = new Set(["cancelled", "canceled"]);
const actionable = new Set(["ready", "open", "todo", "pending"]);
const active = new Set(["in_progress", "in-progress", "active", "working"]);
const blocked = new Set(["blocked", "parked", "waiting"]);
const unclaimed = new Set(["", "none", "unassigned", "-"]);
const conflict = (reason) => ({ status: "conflict", reason });
const unique = (items) => Array.isArray(items) && new Set(items).size === items.length;
const dependencyEvidenceUnavailable = (task) => ["unavailable", "unknown"].includes(String(task?.dependencyEvidence ?? "").toLowerCase());

export function effectiveRunFingerprint(run) {
  return createHash("sha256").update(stable(run)).digest("hex");
}

function topLevel(task) { return task?.isTopLevelDelivery === true || task?.parentId === null && task?.taskType !== "epic" && task?.isEpic !== true; }
function dependencies(task) { return Array.isArray(task?.dependencies) ? task.dependencies : String(task?.["depends on"] ?? "").split(/\s*,\s*/).filter((id) => id && !["none", "-"].includes(id)); }

export function validateEffectiveRun(run, canonicalTasks) {
  if (!exactKeys(run, runKeys) || !Array.isArray(canonicalTasks) || !validId(run.id) || !validId(run.ownerSessionId)
    || !["codex", "claude-code"].includes(run.ownerHost) || !Number.isSafeInteger(run.ownershipEpoch) || run.ownershipEpoch < 1
    || !["finite", "continuous"].includes(run.mode) || !unique(run.taskIds) || !run.taskIds.length || run.taskIds.some((id) => !validId(id))
    || !Number.isSafeInteger(run.teamLimit) || run.teamLimit < 1 || run.teamLimit > 64 || typeof run.autoDeploy !== "boolean"
    || !Number.isSafeInteger(run.batchSize) || run.batchSize < 1 || run.batchSize > 64 || !sources.has(run.source)
    || !exactKeys(run.settingSources, sourceKeys) || Object.values(run.settingSources).some((source) => !sources.has(source))
    || typeof run.paused !== "boolean" || !Number.isSafeInteger(run.operationalVersion) || run.operationalVersion < 0
    || !unique(run.blockers) || !unique(run.pendingDeliveryIds) || !unique(run.deployedTaskIds) || !terminalKinds.has(run.terminalClassification)) return "invalid_effective_run";
  const taskById = new Map(canonicalTasks.map((task) => [task.id, task]));
  if (run.taskIds.some((id) => !taskById.has(id))) return "invalid_effective_run";
  const blockerIds = new Set();
  for (const item of run.blockers) {
    if (!exactKeys(item, ["taskId", "reason"]) || !run.taskIds.includes(item.taskId) || blockerIds.has(item.taskId)
      || typeof item.reason !== "string" || !item.reason.trim() || item.reason.trim() !== item.reason || Buffer.byteLength(item.reason) > 4096) return "invalid_effective_run";
    blockerIds.add(item.taskId);
  }
  const pending = new Set(run.pendingDeliveryIds);
  if ([...run.pendingDeliveryIds, ...run.deployedTaskIds].some((id) => !run.taskIds.includes(id) || !topLevel(taskById.get(id)))
    || run.deployedTaskIds.some((id) => pending.has(id))) return "invalid_effective_run";
  const scope = new Set(run.taskIds);
  for (const id of run.taskIds) {
    const task = taskById.get(id);
    if (task.hierarchyUnknown) return "invalid_effective_run";
    if (dependencyEvidenceUnavailable(task)) return "unresolved_scope_dependency";
    for (const dependency of dependencies(task)) {
      const predecessor = taskById.get(dependency);
      if (!scope.has(dependency) && (!predecessor || !completed.has(String(predecessor.status).toLowerCase()) && !cancelled.has(String(predecessor.status).toLowerCase()))) return "unresolved_scope_dependency";
    }
  }
  return undefined;
}

function proposalProblem(run, tasks, { migration = false } = {}) {
  const allowed = migration ? sources : new Set(["explicit_run", "saved_default"]);
  if (!exactKeys(run, proposalKeys) || !validId(run?.id) || !["finite", "continuous"].includes(run?.mode) || !unique(run?.taskIds) || !run.taskIds.length
    || !Number.isSafeInteger(run.teamLimit) || run.teamLimit < 1 || run.teamLimit > 64 || typeof run.autoDeploy !== "boolean"
    || !Number.isSafeInteger(run.batchSize) || run.batchSize < 1 || run.batchSize > 64 || !allowed.has(run.source)
    || !exactKeys(run.settingSources, sourceKeys) || Object.values(run.settingSources).some((source) => !allowed.has(source))) return "invalid_effective_run";
  const synthetic = { ...run, ownerSessionId: "synthetic", ownerHost: "codex", ownershipEpoch: 1, paused: false, operationalVersion: 0,
    blockers: [], pendingDeliveryIds: [], deployedTaskIds: [], terminalClassification: "progress_possible" };
  return validateEffectiveRun(synthetic, tasks);
}

async function authenticatedActor(project, options) {
  if (!validId(options.actorSessionId) || options.nativeIdentity?.observed !== true
    || !["codex", "claude-code"].includes(options.nativeIdentity?.host === "claude" ? "claude-code" : options.nativeIdentity?.host)
    || options.nativeIdentity?.sessionId !== options.actorSessionId
    || !await validateNativeOwnerAuthority(project, options.nativeIdentity, project.setup?.ownership, options.actorSessionId)) return null;
  let canonical;
  try { canonical = await loadCanonicalState(project, { includeTasks: false, budget: options.budget }); }
  catch { return null; }
  const epoch = canonical.registry.ownershipEpoch;
  const nativeIdentity = { ...options.nativeIdentity, ownershipEpoch: epoch };
  if (!await validateNativeOwnerAuthority(project, nativeIdentity, canonical.state.ownership, options.actorSessionId)
    || canonical.registry.projectOwner !== options.actorSessionId || canonical.registry.projectOwnerHost !== nativeIdentity.host) return null;
  return { ownerHost: nativeIdentity.host, ownerSessionId: options.actorSessionId, ownershipEpoch: epoch, nativeIdentity };
}

function validReason(value) { return typeof value === "string" && value.trim() === value && value.length > 0 && Buffer.byteLength(value) <= 4096; }

export async function startRun(project, request, options = {}) {
  if (!exactKeys(request, ["operationId", "expectedTrackerFingerprint", "reason", "run"]) || !validId(request?.operationId)
    || !/^[a-f0-9]{64}$/.test(request?.expectedTrackerFingerprint ?? "") || !validReason(request?.reason)) return conflict("invalid_request");
  const actor = await authenticatedActor(project, options);
  if (!actor) return conflict("project_owner_required");
  const effectiveRequest = { ...request, actorSessionId: options.actorSessionId, expectedVersion: options.expectedVersion,
    authenticatedActor: { ownerHost: actor.ownerHost, ownerSessionId: actor.ownerSessionId, ownershipEpoch: actor.ownershipEpoch } };
  return mutateOperationalState(project, effectiveRequest, async (state, canonical) => {
    if (state.run !== undefined) return conflict("run_already_active");
    const current = await loadCanonicalTracker(project, { budget: options.budget });
    if (current.tracker.status !== "current") return conflict("tracker_unavailable");
    if (current.tracker.fingerprint !== request.expectedTrackerFingerprint) return conflict("stale_tracker");
    if (canonical.registry.projectOwnerHost !== actor.ownerHost || canonical.registry.projectOwner !== actor.ownerSessionId || canonical.registry.ownershipEpoch !== actor.ownershipEpoch) return conflict("project_owner_required");
    const problem = proposalProblem(request.run, current.tasks);
    if (problem) return conflict(problem);
    const run = { ...structuredClone(request.run), ownerSessionId: actor.ownerSessionId, ownerHost: actor.ownerHost, ownershipEpoch: actor.ownershipEpoch,
      paused: false, operationalVersion: (canonical.state.stateVersion ?? 0) + 1, blockers: [], pendingDeliveryIds: [], deployedTaskIds: [], terminalClassification: "progress_possible" };
    return { state: { ...state, run }, result: { run: structuredClone(run), trackerFingerprint: current.tracker.fingerprint, authenticatedActor: effectiveRequest.authenticatedActor } };
  }, { ...options, nativeIdentity: actor.nativeIdentity });
}

export async function reconcileRun(project, request, options = {}) {
  if (!exactKeys(request, ["operationId", "expectedTrackerFingerprint", "expectedRunFingerprint", "authoritativeSource", "reason", "affectedTaskIds", "run"])
    || !validId(request?.operationId) || ![request?.expectedTrackerFingerprint, request?.expectedRunFingerprint].every((value) => /^[a-f0-9]{64}$/.test(value ?? ""))
    || !validReason(request?.reason) || !unique(request?.affectedTaskIds) || !request.affectedTaskIds.length || request.affectedTaskIds.some((id) => !validId(id))) return conflict("invalid_request");
  const actor = await authenticatedActor(project, options);
  if (!actor) return conflict("project_owner_required");
  const effectiveRequest = { ...request, actorSessionId: options.actorSessionId, expectedVersion: options.expectedVersion,
    authenticatedActor: { ownerHost: actor.ownerHost, ownerSessionId: actor.ownerSessionId, ownershipEpoch: actor.ownershipEpoch } };
  return mutateOperationalState(project, effectiveRequest, async (state, canonical) => {
    const previousRun = structuredClone(state.run);
    if (!previousRun) return conflict("run_not_active");
    if (previousRun.paused !== undefined && typeof previousRun.paused !== "boolean") return conflict("invalid_effective_run");
    if (previousRun.paused) return conflict("paused");
    const current = await loadCanonicalTracker(project, { budget: options.budget });
    if (current.tracker.status !== "current") return conflict("tracker_unavailable");
    if (current.tracker.fingerprint !== request.expectedTrackerFingerprint) return conflict("stale_tracker");
    if (effectiveRunFingerprint(previousRun) !== request.expectedRunFingerprint) return conflict("stale_run");
    if (canonical.registry.projectOwnerHost !== actor.ownerHost || canonical.registry.projectOwner !== actor.ownerSessionId || canonical.registry.ownershipEpoch !== actor.ownershipEpoch) return conflict("project_owner_required");
    const qualified = !validateEffectiveRun(previousRun, current.tasks);
    const legacy = !validId(previousRun.id) || !validId(previousRun.ownerSessionId) || !["codex", "claude-code"].includes(previousRun.ownerHost)
      || !Number.isSafeInteger(previousRun.ownershipEpoch) || previousRun.ownershipEpoch < 1;
    if (!qualified && !legacy) return conflict("invalid_effective_run");
    if (!sources.has(request.authoritativeSource) || request.run?.source !== request.authoritativeSource) return conflict("invalid_authoritative_source");
    if (legacy && request.authoritativeSource !== "compatibility_migration") return conflict("legacy_run_requires_compatibility_migration");
    if (!unique(request.affectedTaskIds) || request.affectedTaskIds.some((id) => !previousRun.taskIds?.includes(id))
      || stable(request.run?.taskIds) !== stable(previousRun.taskIds) || qualified && request.run.id !== previousRun.id) return conflict("scope_change_requires_extension");
    if (request.authoritativeSource === "compatibility_migration" && previousRun.autoDeploy !== true && request.run.autoDeploy === true) return conflict("migration_cannot_enable_deployment");
    const problem = proposalProblem(request.run, current.tasks, { migration: true });
    if (problem) return conflict(problem);
    const provenance = qualified ? { ownerSessionId: previousRun.ownerSessionId, ownerHost: previousRun.ownerHost, ownershipEpoch: previousRun.ownershipEpoch }
      : { ownerSessionId: actor.ownerSessionId, ownerHost: actor.ownerHost, ownershipEpoch: actor.ownershipEpoch };
    const run = { ...structuredClone(request.run), ...provenance, paused: previousRun.paused ?? false, operationalVersion: (canonical.state.stateVersion ?? 0) + 1,
      blockers: structuredClone(previousRun.blockers ?? []), pendingDeliveryIds: [...(previousRun.pendingDeliveryIds ?? [])], deployedTaskIds: [...(previousRun.deployedTaskIds ?? [])],
      terminalClassification: previousRun.terminalClassification ?? "unknown" };
    const constructedProblem = validateEffectiveRun(run, current.tasks);
    if (constructedProblem) return conflict(constructedProblem);
    const result = { previousRun, run: structuredClone(run), authenticatedActor: effectiveRequest.authenticatedActor, authoritativeSource: request.authoritativeSource,
      reason: request.reason, affectedTaskIds: [...request.affectedTaskIds], trackerFingerprint: current.tracker.fingerprint,
      deploymentHeld: request.authoritativeSource === "compatibility_migration" || run.autoDeploy === false };
    return { state: { ...state, run }, result };
  }, { ...options, nativeIdentity: actor.nativeIdentity });
}

function safeEligible(tasks, run) {
  const taskById = new Map(tasks.map((task) => [task.id, task]));
  const explicitlyBlocked = new Set((run?.blockers ?? []).map(({ taskId }) => taskId));
  return tasks.filter((task) => run?.taskIds?.includes(task.id) && topLevel(task) && actionable.has(String(task.status).toLowerCase())
    && !dependencyEvidenceUnavailable(task) && !explicitlyBlocked.has(task.id) && unclaimed.has(task.owner ?? "")
    && dependencies(task).every((id) => completed.has(String(taskById.get(id)?.status).toLowerCase()) || cancelled.has(String(taskById.get(id)?.status).toLowerCase()))).map(({ id }) => id);
}

export function classifyRun(canonical, { writerLiveness = {} } = {}) {
  const run = canonical?.state?.run;
  const eligibleTaskIds = Array.isArray(canonical?.tasks) ? safeEligible(canonical.tasks, run) : [];
  const ids = new Set((canonical?.tasks ?? []).map(({ id }) => id));
  if (canonical?.tracker?.status !== "current" || !canonical?.registry || canonical.tasks?.some((task) => task.hierarchyUnknown || task.parentId !== null && !ids.has(task.parentId))
    || validateEffectiveRun(run, canonical?.tasks ?? [])) return { kind: "unknown", eligibleTaskIds, blockedTaskIds: [] };
  if (run.paused) return { kind: "paused", eligibleTaskIds: [], blockedTaskIds: [] };
  const taskById = new Map(canonical.tasks.map((task) => [task.id, task]));
  const scoped = run.taskIds.map((id) => taskById.get(id)).filter(topLevel);
  const unreconciled = scoped.filter((task) => completed.has(String(task.status).toLowerCase()) && !run.pendingDeliveryIds.includes(task.id) && !run.deployedTaskIds.includes(task.id));
  const runBlocked = new Set(run.blockers.map(({ taskId }) => taskId));
  const blockedTaskIds = [];
  let canProgress = eligibleTaskIds.length > 0;
  const unfinished = scoped.filter((task) => !completed.has(String(task.status).toLowerCase()) && !cancelled.has(String(task.status).toLowerCase()));
  for (const task of unfinished) {
    const status = String(task.status).toLowerCase();
    const live = writerLiveness[task.id];
    const assigned = !unclaimed.has(task.owner ?? "");
    const dependenciesHeld = dependencies(task).some((id) => !completed.has(String(taskById.get(id)?.status).toLowerCase()) && !cancelled.has(String(taskById.get(id)?.status).toLowerCase()));
    if (assigned && live?.status === "active" && ["codex", "claude-code"].includes(live.host) && validId(live.sessionId)) canProgress = true;
    else if (blocked.has(status) || runBlocked.has(task.id) || dependenciesHeld || assigned && (!live || live.status !== "active")) blockedTaskIds.push(task.id);
    else if (!actionable.has(status)) canProgress = true;
  }
  if (unreconciled.length) return { kind: "unreconciled_completion", eligibleTaskIds, blockedTaskIds: [...new Set(blockedTaskIds)] };
  if (!unfinished.length) return { kind: run.mode === "finite" ? "finite_exhausted" : "continuous_scope_exhausted", eligibleTaskIds: [], blockedTaskIds: [] };
  const uniqueBlocked = [...new Set(blockedTaskIds)];
  return { kind: canProgress ? "progress_possible" : uniqueBlocked.length === unfinished.length ? "blocked_tail" : "progress_possible", eligibleTaskIds, blockedTaskIds: uniqueBlocked };
}

function sameOwner(value, expected) {
  return value?.ownerHost === expected?.ownerHost && value?.ownerSessionId === expected?.ownerSessionId
    && value?.ownershipEpoch === expected?.ownershipEpoch && ["codex", "claude-code"].includes(value?.ownerHost)
    && validId(value?.ownerSessionId) && Number.isSafeInteger(value?.ownershipEpoch) && value.ownershipEpoch > 0;
}

const joinedEvidenceKeys = ["taskId", "sourceRevision", "revision", "integratedRevision", "completion", "integration", "review", "checks", "preview", "target", "recovery"];
const fullRevision = (value) => typeof value === "string" && /^[a-f0-9]{40}$/.test(value);
const boundedAuthority = (value) => typeof value === "string" && value.trim() === value && value.length > 0 && Buffer.byteLength(value) <= 4096;

function targetAuthorityReady(value, { id, boundary, target, releaseOwner }) {
  return exactKeys(value, ["source", "target", "revision", "taskIds", "ownerHost", "ownerSessionId", "ownershipEpoch"])
    && typeof value.source === "string" && value.source.trim() === value.source && value.source.length > 0 && Buffer.byteLength(value.source) <= 256
    && value.target === target && value.revision === boundary && Array.isArray(value.taskIds) && value.taskIds.length === 1 && value.taskIds[0] === id
    && sameOwner(value, releaseOwner);
}

function evidenceReady(evidence, id, canonical) {
  const integrationOwner = canonical.state?.integration;
  const releaseOwner = canonical.state?.release;
  const source = evidence?.sourceRevision;
  const boundary = evidence?.revision;
  return exactKeys(evidence, joinedEvidenceKeys) && evidence.taskId === id && fullRevision(source) && fullRevision(boundary)
    && evidence.integratedRevision === boundary
    && evidence.completion?.taskId === id && evidence.completion?.status === "passed" && evidence.completion?.sourceRevision === source
    && evidence.integration?.taskId === id && evidence.integration?.status === "passed" && evidence.integration?.sourceRevision === source
    && evidence.integration?.boundaryRevision === boundary
    && evidence.review?.taskId === id && evidence.review?.status === "passed" && evidence.review?.revision === source
    && Array.isArray(evidence.checks) && evidence.checks.length > 0
    && evidence.checks.every((check) => check && check.status === "passed" && boundedAuthority(check.name))
    && evidence.preview?.taskId === id && evidence.preview?.revision === boundary && typeof evidence.preview?.required === "boolean"
    && (evidence.preview.required ? evidence.preview.status === "passed" : evidence.preview.status === "not_required")
    && evidence.target?.taskId === id && evidence.target?.revision === boundary && boundedAuthority(evidence.target?.target)
    && targetAuthorityReady(evidence.target?.authority, { id, boundary, target: evidence.target.target, releaseOwner })
    && evidence.recovery?.taskId === id && evidence.recovery?.revision === boundary
    && boundedAuthority(evidence.recovery?.artifact) && boundedAuthority(evidence.recovery?.action)
    && evidence.target?.status === "authorized" && evidence.recovery?.status === "ready"
    && sameOwner(evidence.integration, integrationOwner) && sameOwner(evidence.preview, integrationOwner)
    && sameOwner(evidence.target, releaseOwner) && sameOwner(evidence.recovery, releaseOwner)
    && sameOwner(releaseOwner?.authorization, releaseOwner);
}

export function selectReleaseBatch(canonical, classification) {
  const run = canonical?.state?.run;
  if (validateEffectiveRun(run, canonical?.tasks ?? []) || ["unknown", "paused", "unreconciled_completion"].includes(classification?.kind)) return [];
  const tasks = new Map(canonical.tasks.map((task) => [task.id, task]));
  const seen = new Set();
  const ready = run.pendingDeliveryIds.filter((id) => !seen.has(id) && seen.add(id) && !run.deployedTaskIds.includes(id) && run.taskIds.includes(id)
    && completed.has(String(tasks.get(id)?.status).toLowerCase()) && topLevel(tasks.get(id)) && evidenceReady(canonical.deliveryEvidence?.[id], id, canonical));
  if (ready.length >= run.batchSize) return ready.slice(0, run.batchSize);
  return ready.length && ["finite_exhausted", "continuous_scope_exhausted", "blocked_tail"].includes(classification.kind) ? ready : [];
}

export function readRunDecision(canonical, { writerLiveness = {} } = {}) {
  const run = canonical?.state?.run;
  const classification = classifyRun(canonical, { writerLiveness });
  const valid = !validateEffectiveRun(run, canonical?.tasks ?? []);
  const currentOwner = canonical?.registry?.projectOwnerHost && canonical?.registry?.projectOwner && canonical?.registry?.ownershipEpoch
    ? { ownerHost: canonical.registry.projectOwnerHost, ownerSessionId: canonical.registry.projectOwner, ownershipEpoch: canonical.registry.ownershipEpoch } : null;
  const runProvenance = valid ? { ownerHost: run.ownerHost, ownerSessionId: run.ownerSessionId, ownershipEpoch: run.ownershipEpoch,
    historical: !currentOwner || run.ownerHost !== currentOwner.ownerHost || run.ownerSessionId !== currentOwner.ownerSessionId || run.ownershipEpoch !== currentOwner.ownershipEpoch } : null;
  const holdReasons = [];
  const selectedBatchTaskIds = valid ? selectReleaseBatch(canonical, classification) : [];
  if (!valid || classification.kind === "unknown") holdReasons.push("effective_run_unavailable");
  if (valid && !run.autoDeploy) holdReasons.push("auto_deploy_disabled");
  if (runProvenance?.historical && selectedBatchTaskIds.length === 0) holdReasons.push("historical_run_provenance");
  if (valid && run.autoDeploy && selectedBatchTaskIds.length === 0) holdReasons.push("batch_not_ready");
  return { status: valid ? "available" : "unavailable", effectiveRun: run ? structuredClone(run) : null,
    effectiveRunFingerprint: valid ? effectiveRunFingerprint(run) : null, classification, selectedBatchTaskIds,
    deploymentHeld: holdReasons.length > 0, holdReasons, runProvenance, currentOwner, gitHeadRevision: canonical?.git?.headRevision ?? null };
}
