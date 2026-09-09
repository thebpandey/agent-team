import { loadCanonicalState } from "./canonical-state.mjs";
import { resolveProject } from "./project.mjs";

const completed = new Set(["completed", "complete", "done", "closed"]);
const excluded = new Map([
  ["cancelled", "cancelled"],
  ["canceled", "cancelled"],
  ["approved_deferred", "deferred"],
  ["approved deferred", "deferred"],
  ["deferred", "deferred"],
]);

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
      counted: false,
    };
  });
  const ids = new Set(tasks.map((task) => task.id));
  for (const task of tasks) task.counted = (!task.parentId || !ids.has(task.parentId)) && !excluded.has(task.status);
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
  return {
    status: hasUnknownHierarchy ? "provisional" : "exact",
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

/** Build one immutable, read-only status view from canonical task and team records. */
export function createStatusModel(project, canonical = {}, options = {}) {
  const freshness = freshnessFor(canonical.tracker);
  const state = canonical.state && typeof canonical.state === "object" ? canonical.state : {};
  const tasks = normalizeTasks(canonical.tasks).map((task) => ({ ...task, runtime: runtimeFor(state, task.id) }));
  const teams = (canonical.registry?.teams || []).map((team) => ({
    id: value(team, "team id", "id") || "unknown",
    name: value(team, "name") || "Unnamed team",
    role: value(team, "role") || "unknown",
    session: value(team, "session") || "unknown",
    status: normalizeStatus(value(team, "status")),
    model: value(team, "model") || "unknown",
    effort: value(team, "effort") || "unknown",
  }));
  const activity = {
    active: tasks.filter((task) => (task.runtime?.compute || task.status) === "active" || ["in_progress", "working"].includes(task.status)).length,
    parked: tasks.filter((task) => task.status === "parked" || task.runtime?.compute === "parked").length,
    paused: tasks.filter((task) => task.status === "paused" || task.runtime?.compute === "paused").length,
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
    run: { paused: typeof state.run?.paused === "boolean" ? state.run.paused : null },
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
  const project = typeof cwd === "object" && cwd ? cwd : await resolve(cwd);
  if (!project?.active) throw new Error("An active Agent-Team project is required for status.");
  return createStatusModel(project, await load(project), options);
}
