import { loadCanonicalState } from "./canonical-state.mjs";
import { resolveProject } from "./project.mjs";
import { createEventBudget } from "./budget.mjs";

const completed = new Set(["completed", "complete", "done", "closed"]);
const excluded = new Map([
  ["cancelled", "cancelled"],
  ["canceled", "cancelled"],
  ["approved_deferred", "deferred"],
  ["approved deferred", "deferred"],
]);
const knownStatuses = new Set(["open", "todo", "ready", "in_progress", "active", "working", "parked", "paused", "deferred", "blocked", "completed", "complete", "done", "closed", ...excluded.keys()]);

function value(row, ...names) {
  for (const name of names) {
    const found = row?.[name];
    if (found !== undefined && found !== null && String(found).trim()) return String(found).trim();
  }
  return "";
}

function normalizeStatus(status) {
  return String(status || "unknown").trim().toLowerCase().replace(/[ -]+/g, "_") || "unknown";
}

function dependencies(row) {
  const raw = value(row, "dependencies", "depends on", "depends_on");
  return !raw || /^(none|n\/a|-)$/i.test(raw) ? [] : raw.split(/\s*,\s*/).filter(Boolean);
}

function deepFreeze(value) {
  if (!value || typeof value !== "object" || Object.isFrozen(value)) return value;
  for (const entry of Object.values(value)) deepFreeze(entry);
  return Object.freeze(value);
}

function normalizeTasks(rows) {
  const tasks = (Array.isArray(rows) ? rows : []).map((row) => {
    const status = normalizeStatus(value(row, "status"));
    const parentId = value(row, "parent", "parent id", "parent_id");
    return {
      id: value(row, "id") || "unknown",
      label: value(row, "label", "requirement / acceptance", "requirement", "title") || "Unknown task",
      status,
      priority: value(row, "priority") || "unknown",
      owner: value(row, "owner", "team", "team id") || "unassigned",
      dependencies: dependencies(row),
      parentId: parentId || null,
      nextAction: value(row, "next action", "next_action") || null,
      evidence: value(row, "revision / evidence", "evidence", "revision") || null,
      counted: false,
      duplicate: false,
    };
  });
  const ids = new Set(tasks.map((task) => task.id));
  const parentsWithChildren = new Set(tasks.filter((task) => task.parentId && ids.has(task.parentId)).map((task) => task.parentId));
  const admittedIds = new Set();
  for (const task of tasks) {
    task.duplicate = admittedIds.has(task.id);
    admittedIds.add(task.id);
    task.counted = !task.duplicate && !parentsWithChildren.has(task.id) && !excluded.has(task.status);
  }
  return tasks;
}

function freshnessFor(tracker) {
  const status = tracker?.status;
  if (status === "current") return { status: "current", source: tracker.path || tracker.id || "canonical tracker", observedAt: tracker.observedAt || null, fingerprint: tracker.fingerprint || null };
  if (status === "unavailable") return { status: "unavailable", source: tracker?.path || tracker?.id || "canonical tracker", observedAt: tracker?.observedAt || null, reason: tracker?.reason || "unavailable" };
  if (status === "stale") return { status: "stale", source: tracker?.path || tracker?.id || "canonical tracker", observedAt: tracker?.observedAt || null, reason: tracker?.reason || null };
  return { status: "unknown", source: tracker?.path || tracker?.id || "canonical tracker", observedAt: tracker?.observedAt || null, reason: tracker?.reason || "not_read" };
}

function progressFor(tasks, freshness) {
  if (freshness.status === "unavailable" || freshness.status === "unknown") {
    return { status: "unknown", total: null, completed: null, remaining: null, percentage: null, excluded: { cancelled: null, deferred: null } };
  }
  const actionable = tasks.filter((task) => task.counted);
  const counts = { cancelled: 0, deferred: 0 };
  for (const task of tasks) {
    const kind = excluded.get(task.status);
    if (kind) counts[kind] += 1;
  }
  if (!actionable.length) return { status: "not_applicable", total: 0, completed: 0, remaining: 0, percentage: null, excluded: counts };
  const complete = actionable.filter((task) => completed.has(task.status)).length;
  const hasUnknownHierarchy = tasks.some((task) => task.parentId && !tasks.some((candidate) => candidate.id === task.parentId));
  const hasUnknownStatus = tasks.some((task) => !knownStatuses.has(task.status));
  const hasDuplicates = tasks.some((task) => task.duplicate);
  return {
    status: hasUnknownHierarchy || hasUnknownStatus || hasDuplicates ? "provisional" : "exact",
    total: actionable.length,
    completed: complete,
    remaining: actionable.length - complete,
    percentage: Math.round((complete / actionable.length) * 100),
    excluded: counts,
  };
}

function runtimeFor(state, taskId) {
  const runtime = state?.taskRuntime?.[taskId];
  if (!runtime || typeof runtime !== "object") return null;
  return {
    compute: ["active", "parked", "paused"].includes(runtime.compute) ? runtime.compute : "unknown",
    explicitPause: runtime.explicitPause === true,
    checkpointPath: typeof runtime.checkpointPath === "string" ? runtime.checkpointPath : null,
    resumeWhen: typeof runtime.resumeWhen === "string" ? runtime.resumeWhen : null,
    evidencePointers: Array.isArray(runtime.evidencePointers) ? runtime.evidencePointers.filter((entry) => typeof entry === "string") : [],
  };
}

function capacityFor(value) {
  if (Number.isFinite(value) && value >= 0) return value;
  if (!value || typeof value !== "object") return null;
  if (Number.isFinite(value.available) && value.available >= 0) return value.available;
  if ([value.limit, value.active, value.reservedReview].every(Number.isFinite)) return Math.max(0, value.limit - value.active - value.reservedReview);
  return null;
}

function cleanupFor(state, taskId) {
  const cleanup = state?.cleanup?.[taskId];
  if (!cleanup || typeof cleanup !== "object") return null;
  return {
    revision: typeof cleanup.revision === "string" ? cleanup.revision : null,
    integrationRef: typeof cleanup.integrationRef === "string" ? cleanup.integrationRef : null,
    verification: typeof cleanup.verification?.status === "string" ? cleanup.verification.status : "unknown",
    evidencePaths: Array.isArray(cleanup.evidencePaths) ? cleanup.evidencePaths.filter((entry) => typeof entry === "string") : [],
    retain: cleanup.retain === true,
  };
}

/** Build one immutable, read-only status view from canonical task and team records. */
export function createStatusModel(project, canonical = {}, options = {}) {
  const freshness = freshnessFor(canonical.tracker);
  const state = canonical.state && typeof canonical.state === "object" ? canonical.state : {};
  const tasks = normalizeTasks(canonical.tasks).map((task) => ({ ...task, runtime: runtimeFor(state, task.id), cleanup: cleanupFor(state, task.id) }));
  const teams = (canonical.registry?.teams || []).map((team) => ({
    id: value(team, "team id", "id") || "unknown",
    name: value(team, "name") || "Unnamed team",
    role: value(team, "role") || "unknown",
    session: value(team, "session") || "unknown",
    status: normalizeStatus(value(team, "status")),
    model: value(team, "model") || "unknown",
    effort: value(team, "effort") || "unknown",
    assignments: value(team, "tasks") || "unknown",
    updatedAt: value(team, "last update", "updated at", "updated") || null,
  }));
  const unavailable = freshness.status === "unavailable" || freshness.status === "unknown";
  const execution = (task) => task.runtime?.compute && task.runtime.compute !== "unknown"
    ? task.runtime.compute
    : ["active", "in_progress", "working"].includes(task.status) ? "active" : ["parked", "paused"].includes(task.status) ? task.status : "unknown";
  const activity = unavailable ? { active: null, parked: null, paused: null, ready: null, capacity: null } : {
    active: tasks.filter((task) => execution(task) === "active").length,
    parked: tasks.filter((task) => execution(task) === "parked").length,
    paused: tasks.filter((task) => execution(task) === "paused").length,
    ready: tasks.filter((task) => task.status === "ready").length,
    capacity: capacityFor(options.capacity ?? state.capacity),
  };
  return deepFreeze({
    project: { id: canonical.registry?.projectId || project?.projectId || "unknown", root: project?.root || null },
    mode: options.mode === "live" ? "live" : "snapshot",
    freshness,
    progress: progressFor(tasks, freshness),
    activity,
    teams,
    tasks,
    run: {
      paused: typeof state.run?.paused === "boolean" ? state.run.paused : null,
      current: typeof state.run?.id === "string" ? state.run.id : typeof state.run?.current === "string" ? state.run.current : "unknown",
      blockers: Array.isArray(state.run?.blockers) ? state.run.blockers.filter((entry) => typeof entry === "string") : [],
      blockerStatus: Array.isArray(state.run?.blockers) ? "known" : "unknown",
      scope: Array.isArray(state.run?.taskIds)
        ? { status: "partial", taskIds: state.run.taskIds.filter((entry) => typeof entry === "string") }
        : state.run ? { status: "full_project", taskIds: [] } : { status: "unknown", taskIds: [] },
    },
    state: {
      integration: state.integration?.status ? { status: state.integration.status } : { status: "unknown" },
      release: state.release?.status ? { status: state.release.status } : { status: "unknown" },
    },
  });
}

/** Resolve and read canonical state only; this function intentionally has no write or process side effects. */
export async function readStatus(cwd, options = {}) {
  const resolve = options.resolveProject || resolveProject;
  const load = options.loadCanonicalState || loadCanonicalState;
  const base = options.budget ?? createEventBudget(options.deadlineMs ?? 1500);
  const signal = options.signal ? AbortSignal.any([base.signal, options.signal]) : base.signal;
  const budget = {
    ...base, signal,
    check() { signal.throwIfAborted(); base.check(); },
    timeout(cap) { this.check(); return base.timeout(cap); },
    async run(action) {
      this.check();
      let abort;
      try {
        return await Promise.race([base.run(action), new Promise((_, reject) => {
          abort = () => reject(signal.reason ?? new Error('Status read aborted.'));
          signal.addEventListener('abort', abort, { once: true });
        })]);
      } finally { signal.removeEventListener('abort', abort); }
    },
  };
  let project = typeof cwd === "object" && cwd ? cwd : null;
  try {
    project ??= await budget.run(() => resolve(cwd, { budget }));
    if (!project?.active) throw new Error("An active Agent-Team project is required for status.");
    return createStatusModel(project, await budget.run(() => load(project, { budget })), options);
  } catch (error) {
    if (!project?.active) throw error;
    const previous = options.lastGood;
    return createStatusModel(project, {
      tracker: { kind: previous?.freshness?.source === "TASKS.md" ? "tasks" : "unknown", id: previous?.freshness?.source || "canonical tracker", path: previous?.freshness?.source || "canonical tracker", status: "unavailable", reason: String(error.message || error) },
      registry: { projectId: previous?.project?.id || project.projectId, teams: previous?.teams || [] },
      tasks: previous?.tasks || [],
      state: previous?.state || {},
    }, options);
  } finally { if (!options.budget) base.close(); }
}
