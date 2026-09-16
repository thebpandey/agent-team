import { createHash } from "node:crypto";
import path from "node:path";
import { ROLE_DEFINITIONS } from "./dependency-profiles.mjs";

// Canonical lane data is pure so state readers do not depend on lifecycle writers.

const roles = new Set(ROLE_DEFINITIONS.map(({ id }) => id));
const reviewerRoles = new Set(["reviewer", "visual_reviewer"]);
const statuses = new Set(["prepared", "active", "rotation_required", "closed"]);
const efforts = new Set(["low", "medium", "high", "xhigh", "max", "ultra"]);
const laneKeys = ["schemaVersion", "id", "teamId", "status", "role", "model", "effort", "queue", "currentTaskId", "worker",
  "worktree", "branch", "brief", "ownershipEvidence", "rotationCount", "handover", "factSheets", "assignments", "results"];
const requiredBriefSections = ["role / model / effort", "queue", "shared rules", "writable paths", "required skills", "applicable instructions",
  "context revision", "evidence destination", "handoff format"];

const stable = (value) => JSON.stringify(value && typeof value === "object"
  ? Array.isArray(value) ? value.map((entry) => JSON.parse(stable(entry)))
    : Object.fromEntries(Object.keys(value).sort().map((key) => [key, JSON.parse(stable(value[key]))])) : value);
const exactKeys = (value, keys) => value && typeof value === "object" && !Array.isArray(value)
  && stable(Object.keys(value).sort()) === stable([...keys].sort());
const validId = (value) => typeof value === "string" && /^[A-Za-z0-9][\w.:-]{0,127}$/.test(value)
  && !value.includes("..") && !["none", "unknown", "unassigned"].includes(value.toLowerCase());
const validLaneId = (value) => typeof value === "string" && /^[a-z0-9][a-z0-9._-]{0,63}$/.test(value) && !value.includes("..");
const hex = (value) => typeof value === "string" && /^[a-f0-9]{64}$/.test(value);
const timestamp = (value) => typeof value === "string" && value.length <= 128 && Number.isFinite(Date.parse(value));
const unique = (values) => Array.isArray(values) && new Set(values).size === values.length;

function validWorker(value) {
  return value === null || exactKeys(value, ["host", "sessionId", "generation"])
    && ["codex", "claude-code"].includes(value.host) && validId(value.sessionId)
    && Number.isSafeInteger(value.generation) && value.generation > 0;
}

function validDispatch(value, worker) {
  return value === null || exactKeys(value, ["status", "source", "eventId", "observedAt"])
    && timestamp(value.observedAt)
    && (value.status === "observed" && value.source === worker?.host && validId(value.eventId)
      || value.status === "unknown" && value.source === "unavailable" && value.eventId === null);
}

function validHandover(value, laneId) {
  return value === null || exactKeys(value, ["path", "sha256", "revision", "sequence"])
    && value.path === `.agent-team/lanes/${laneId}/HANDOVER-${String(value.sequence).padStart(3, "0")}.md`
    && hex(value.sha256) && /^[a-f0-9]{40,64}$/.test(value.revision ?? "")
    && Number.isSafeInteger(value.sequence) && value.sequence > 0;
}

function validOwnershipEvidence(value, laneId) {
  return value === null || exactKeys(value, ["path", "sha256", "revision", "pathSetHash"])
    && value.path === `.agent-team/lanes/${laneId}/evidence/ownership.json`
    && hex(value.sha256) && /^[a-f0-9]{40,64}$/.test(value.revision ?? "") && hex(value.pathSetHash);
}

function validFactSheet(value, laneId) {
  return exactKeys(value, ["id", "ownerLaneId", "path", "sha256", "facts"])
    && validId(value.id) && value.ownerLaneId === laneId
    && value.path === `.agent-team/lanes/${laneId}/facts/${value.id}.json`
    && hex(value.sha256) && Array.isArray(value.facts) && value.facts.length > 0
    && unique(value.facts.map((fact) => fact?.id)) && value.facts.every((fact) => exactKeys(fact, ["id", "checkedOn", "sourceUrl"])
      && validId(fact.id) && timestamp(fact.checkedOn) && typeof fact.sourceUrl === "string"
      && /^https?:\/\/[^\s]+$/.test(fact.sourceUrl) && fact.sourceUrl.length <= 2048);
}

function validFactReference(value, lane) {
  const sheet = lane.factSheets.find(({ id }) => id === value?.id);
  return exactKeys(value, ["id", "sha256", "factIds", "citation"]) && sheet && value.sha256 === sheet.sha256
    && Array.isArray(value.factIds) && value.factIds.length > 0 && unique(value.factIds)
    && value.factIds.every((id) => sheet.facts.some((fact) => fact.id === id))
    && exactKeys(value.citation, ["path", "sha256"])
    && typeof value.citation.path === "string" && value.citation.path.startsWith(`.agent-team/lanes/${lane.id}/evidence/citations/`)
    && path.posix.normalize(value.citation.path) === value.citation.path && !value.citation.path.split("/").includes("..")
    && hex(value.citation.sha256);
}

export function validateLaneFactReferences(lane, value) {
  return validLane(lane) && Array.isArray(value) && unique(value.map((entry) => entry?.id))
    && value.every((entry) => validFactReference(entry, lane)) ? undefined : "invalid_fact_references";
}

function validSourceAssignment(value) {
  return exactKeys(value, ["laneId", "assignmentId", "packetSha256", "revision"])
    && validLaneId(value.laneId) && validId(value.assignmentId) && hex(value.packetSha256)
    && /^[a-f0-9]{40,64}$/.test(value.revision ?? "");
}

function validAssignment(value, lane) {
  const keys = ["id", "taskId", "attempt", "packet", "briefSha256", "revision", "worker", "decisions", "factSheets", "status", "createdAt", "dispatchedAt", "dispatch"];
  const reviewLane = reviewerRoles.has(lane.role);
  return exactKeys(value, reviewLane ? [...keys, "sourceAssignment"] : keys)
    && validId(value.id) && lane.queue.includes(value.taskId) && Number.isSafeInteger(value.attempt) && value.attempt > 0
    && exactKeys(value.packet, ["path", "sha256"])
    && value.packet.path === `.agent-team/lanes/${lane.id}/packets/${value.taskId}-${value.attempt}.md`
    && hex(value.packet.sha256) && value.briefSha256 === lane.brief.sha256 && /^[a-f0-9]{40,64}$/.test(value.revision ?? "")
    && validWorker(value.worker) && value.worker !== null && Array.isArray(value.decisions) && unique(value.decisions.map((entry) => entry?.id))
    && value.decisions.every((entry) => exactKeys(entry, ["id", "kind", "supersedes"]) && validId(entry.id)
      && ["new", "supersedes"].includes(entry.kind) && (entry.kind === "new" ? entry.supersedes === null : validId(entry.supersedes) && entry.supersedes !== entry.id))
    && Array.isArray(value.factSheets) && unique(value.factSheets.map((entry) => entry?.id))
    && value.factSheets.every((entry) => validFactReference(entry, lane))
    && (!reviewLane || validSourceAssignment(value.sourceAssignment))
    && ["prepared", "dispatched", "resolved"].includes(value.status)
    && timestamp(value.createdAt) && (value.dispatchedAt === null || timestamp(value.dispatchedAt))
    && validDispatch(value.dispatch, value.worker)
    && (value.status === "prepared" ? value.dispatchedAt === null && value.dispatch === null
      : value.dispatchedAt !== null && value.dispatch !== null && value.dispatchedAt === value.dispatch.observedAt);
}

function validResult(value, lane, assignments) {
  const assignment = assignments.find(({ id }) => id === value?.assignmentId);
  return exactKeys(value, ["assignmentId", "taskId", "kind", "status", "revision", "worker", "evidence", "recordedAt"])
    && assignment && value.taskId === assignment.taskId
    && ["worker", "verification", "independent_review", "integration", "unresolved"].includes(value.kind)
    && ["passed", "failed", "unresolved"].includes(value.status) && /^[a-f0-9]{40,64}$/.test(value.revision ?? "")
    && validWorker(value.worker) && value.worker !== null && exactKeys(value.evidence, ["path", "sha256"])
    && typeof value.evidence.path === "string"
    && (value.kind === "independent_review" && !reviewerRoles.has(lane.role)
      ? value.evidence.path.startsWith(".agent-team/lanes/") && value.evidence.path.includes("/evidence/")
      : value.evidence.path.startsWith(`.agent-team/lanes/${lane.id}/evidence/`))
    && !value.evidence.path.split("/").includes("..") && hex(value.evidence.sha256) && timestamp(value.recordedAt);
}

function validLaneHistory(lane) {
  const reviewLane = reviewerRoles.has(lane.role);
  const attempts = new Set();
  const decisions = new Set();
  const superseded = new Set();
  const attemptCounts = new Map();
  for (const assignment of lane.assignments) {
    const attemptKey = `${assignment.taskId}\0${assignment.attempt}`;
    const expectedAttempt = (attemptCounts.get(assignment.taskId) ?? 0) + 1;
    if (attempts.has(attemptKey) || assignment.attempt !== expectedAttempt) return false;
    attempts.add(attemptKey);
    attemptCounts.set(assignment.taskId, expectedAttempt);
    for (const decision of assignment.decisions) {
      if (decisions.has(decision.id)) return false;
      if (decision.kind === "supersedes" && (!decisions.has(decision.supersedes) || superseded.has(decision.supersedes))) return false;
      decisions.add(decision.id);
      if (decision.supersedes) superseded.add(decision.supersedes);
    }
  }
  const resultKinds = new Set();
  for (const result of lane.results) {
    const key = `${result.assignmentId}\0${result.kind}`;
    if (resultKinds.has(key)) return false;
    resultKinds.add(key);
    if (reviewLane && !["independent_review", "unresolved"].includes(result.kind)) return false;
    if (!reviewLane && result.kind === "independent_review" && !result.evidence.path.startsWith(".agent-team/lanes/")) return false;
    if (result.kind === "unresolved" ? result.status !== "unresolved"
      : result.kind === "worker" ? !["passed", "failed"].includes(result.status)
        : result.status === "unresolved") return false;
  }
  for (const assignment of lane.assignments) {
    const results = lane.results.map((entry, index) => ({ ...entry, index })).filter(({ assignmentId }) => assignmentId === assignment.id);
    const workerResult = results.find(({ kind }) => kind === "worker");
    const unresolved = results.find(({ kind }) => kind === "unresolved");
    const verification = results.find(({ kind }) => kind === "verification");
    const review = results.find(({ kind }) => kind === "independent_review");
    const integration = results.find(({ kind }) => kind === "integration");
    if (assignment.status === "prepared" && results.length) return false;
    if (assignment.status === "dispatched" && (workerResult || verification || review || integration)) return false;
    if (assignment.status === "resolved" && (assignment.dispatch?.status !== "observed"
      || (reviewLane ? !review || unresolved : !workerResult || unresolved))) return false;
    if (reviewLane && (workerResult || verification || integration || review && stable(review.worker) !== stable(assignment.worker))) return false;
    if (workerResult && stable(workerResult.worker) !== stable(assignment.worker)) return false;
    if (unresolved && stable(unresolved.worker) !== stable(assignment.worker)) return false;
    if (verification && (!workerResult || workerResult.status !== "passed" || verification.index < workerResult.index
      || stable(verification.worker) === stable(assignment.worker))) return false;
    if (!reviewLane && review && (!verification || verification.status !== "passed" || review.index < verification.index
      || stable(review.worker) === stable(assignment.worker))) return false;
    if (integration && (!review || review.status !== "passed" || integration.index < review.index)) return false;
    if (workerResult && results.some((entry) => entry.revision !== workerResult.revision)) return false;
  }
  return true;
}

function validLane(lane) {
  if (!exactKeys(lane, laneKeys) || lane.schemaVersion !== 1 || !validLaneId(lane.id) || !validId(lane.teamId)
    || !statuses.has(lane.status) || !roles.has(lane.role) || typeof lane.model !== "string" || !/^[A-Za-z0-9][\w./:-]{0,127}$/.test(lane.model)
    || !efforts.has(lane.effort) || !unique(lane.queue) || !lane.queue.length || lane.queue.some((id) => !validId(id))
    || !(lane.currentTaskId === null || lane.queue.includes(lane.currentTaskId)) || !validWorker(lane.worker)
    || typeof lane.worktree !== "string" || !path.isAbsolute(lane.worktree) || path.normalize(lane.worktree) !== lane.worktree
    || lane.branch !== `lane/${lane.id}` || !exactKeys(lane.brief, ["path", "sha256"])
    || lane.brief.path !== `.agent-team/lanes/${lane.id}/BRIEF.md` || !hex(lane.brief.sha256)
    || !validOwnershipEvidence(lane.ownershipEvidence, lane.id)
    || !Number.isSafeInteger(lane.rotationCount) || lane.rotationCount < 0 || !validHandover(lane.handover, lane.id)
    || !Array.isArray(lane.factSheets) || !Array.isArray(lane.assignments) || !Array.isArray(lane.results)
    || !unique(lane.factSheets.map((entry) => entry?.id)) || !lane.factSheets.every((entry) => validFactSheet(entry, lane.id))
    || !unique(lane.assignments.map((entry) => entry?.id)) || !lane.assignments.every((entry) => validAssignment(entry, lane))
    || !lane.results.every((entry) => validResult(entry, lane, lane.assignments)) || !validLaneHistory(lane)) return false;
  if (lane.status === "closed" && (lane.currentTaskId !== null || lane.worker !== null)) return false;
  if (lane.status === "active" && (lane.currentTaskId === null || lane.worker === null)) return false;
  return true;
}

export function validateLaneCollection(value) {
  if (!exactKeys(value, ["schemaVersion", "records"]) || value.schemaVersion !== 1 || !Array.isArray(value.records)
    || !unique(value.records.map((lane) => lane?.id)) || !unique(value.records.map((lane) => lane?.teamId))
    || !unique(value.records.map((lane) => lane?.worktree)) || !unique(value.records.map((lane) => lane?.branch))
    || !value.records.every(validLane)) return "invalid_lanes";
  const authorTasks = new Set();
  const reviewTasks = new Set();
  const factSheets = new Set();
  for (const lane of value.records) {
    for (const sheet of lane.factSheets) {
      if (factSheets.has(sheet.id)) return "invalid_lanes";
      factSheets.add(sheet.id);
    }
    const taskSet = reviewerRoles.has(lane.role) ? reviewTasks : authorTasks;
    for (const taskId of lane.queue) {
      if (taskSet.has(taskId)) return "invalid_lanes";
      taskSet.add(taskId);
    }
  }
  if ([...reviewTasks].some((taskId) => !authorTasks.has(taskId))) return "invalid_lanes";
  for (const reviewLane of value.records.filter((lane) => reviewerRoles.has(lane.role))) {
    for (const assignment of reviewLane.assignments) {
      const authorLane = value.records.find(({ id }) => id === assignment.sourceAssignment.laneId);
      const source = authorLane?.assignments.find(({ id }) => id === assignment.sourceAssignment.assignmentId);
      const sourceResults = authorLane?.results.filter(({ assignmentId }) => assignmentId === source?.id) ?? [];
      if (!source || reviewerRoles.has(authorLane.role) || source.taskId !== assignment.taskId
        || source.packet.sha256 !== assignment.sourceAssignment.packetSha256
        || !sourceResults.some(({ kind, status, revision }) => kind === "worker" && status === "passed" && revision === assignment.sourceAssignment.revision)
        || !sourceResults.some(({ kind, status, revision }) => kind === "verification" && status === "passed" && revision === assignment.sourceAssignment.revision)) {
        return "invalid_lanes";
      }
      const review = reviewLane.results.find(({ assignmentId, kind }) => assignmentId === assignment.id && kind === "independent_review");
      if (review?.status === "passed") {
        const linked = authorLane.results.find((entry) => entry.assignmentId === source.id && entry.kind === "independent_review"
          && entry.status === review.status && entry.revision === review.revision && stable(entry.worker) === stable(review.worker)
          && stable(entry.evidence) === stable(review.evidence));
        if (!linked) return "invalid_lanes";
      }
    }
  }
  for (const authorLane of value.records.filter((lane) => !reviewerRoles.has(lane.role))) {
    for (const review of authorLane.results.filter(({ kind }) => kind === "independent_review")) {
      const linked = value.records.filter((lane) => reviewerRoles.has(lane.role)).some((lane) => lane.assignments.some((assignment) =>
        assignment.sourceAssignment.laneId === authorLane.id && assignment.sourceAssignment.assignmentId === review.assignmentId
        && lane.results.some((entry) => entry.assignmentId === assignment.id && entry.kind === "independent_review"
          && entry.status === review.status && entry.revision === review.revision && stable(entry.worker) === stable(review.worker)
          && stable(entry.evidence) === stable(review.evidence))));
      if (!linked) return "invalid_lanes";
    }
  }
  return undefined;
}

export function readLaneCollection(state) {
  if (state?.lanes === undefined) return [];
  if (validateLaneCollection(state.lanes)) throw new Error("invalid_lanes");
  return structuredClone(state.lanes.records);
}

export function laneFingerprint(lane) {
  if (!validLane(lane)) throw new Error("invalid_lane");
  return createHash("sha256").update(stable(lane)).digest("hex");
}

export function laneCollectionFingerprint(value = { schemaVersion: 1, records: [] }) {
  if (validateLaneCollection(value)) throw new Error("invalid_lanes");
  return createHash("sha256").update(stable(value)).digest("hex");
}

export function laneOwnsTask(lane, taskId) {
  return validLane(lane) && validId(taskId) && lane.queue.includes(taskId);
}

export function laneCleanupBlocker(canonical, { taskId, worktree } = {}) {
  const records = readLaneCollection(canonical?.state ?? {});
  const matching = records.filter((entry) => entry.worktree === worktree);
  if (!matching.length) return null;
  if (matching.length !== 1) return "lane_identity_conflict";
  const [lane] = matching;
  if (!laneOwnsTask(lane, taskId)) return "lane_task_mismatch";
  if (lane.status !== "closed") return "lane_open";
  if (reviewerRoles.has(lane.role)) {
    for (const assignment of lane.assignments) {
      const review = lane.results.find((entry) => entry.assignmentId === assignment.id && entry.kind === "independent_review"
        && entry.status === "passed" && entry.revision === assignment.sourceAssignment.revision);
      const sourceLane = records.find(({ id }) => id === assignment.sourceAssignment.laneId);
      const sourceIntegrated = sourceLane?.results.some((entry) => entry.assignmentId === assignment.sourceAssignment.assignmentId
        && entry.kind === "integration" && entry.status === "passed" && entry.revision === assignment.sourceAssignment.revision);
      const accepted = canonical?.deliveryEvidence?.[assignment.taskId];
      if (assignment.status !== "resolved" || !review) return "lane_review_incomplete";
      if (!sourceIntegrated || accepted?.sourceRevision !== assignment.sourceAssignment.revision
        || !/^[a-f0-9]{40,64}$/.test(accepted.integratedRevision ?? "")) return "lane_tasks_not_integrated";
    }
    return lane.assignments.length === lane.queue.length ? null : "lane_review_incomplete";
  }
  if (lane.assignments.some((assignment) => assignment.status !== "resolved"
    || !lane.results.some((result) => result.assignmentId === assignment.id)
    || !lane.results.some((result) => result.assignmentId === assignment.id && ["worker", "unresolved"].includes(result.kind))
    || lane.results.some((result) => result.assignmentId === assignment.id && ["failed", "unresolved"].includes(result.status)))) {
    return "lane_assignment_unresolved";
  }
  for (const queuedTaskId of lane.queue) {
    const assignments = lane.assignments.filter((entry) => entry.taskId === queuedTaskId).sort((left, right) => left.attempt - right.attempt);
    const assignment = assignments.at(-1);
    if (!assignment || assignment.status !== "resolved") return "lane_assignment_unresolved";
    const results = lane.results.map((entry, index) => ({ ...entry, index })).filter((entry) => entry.assignmentId === assignment.id);
    const workerResult = results.find((entry) => entry.kind === "worker" && entry.status === "passed");
    if (!workerResult) return "lane_assignment_unresolved";
    const verification = results.find((entry) => entry.kind === "verification" && entry.status === "passed"
      && entry.revision === workerResult.revision && entry.index > workerResult.index);
    if (!verification) return "lane_verification_incomplete";
    const review = results.find((entry) => entry.kind === "independent_review" && entry.status === "passed"
      && entry.revision === workerResult.revision && entry.index > verification.index
      && stable(entry.worker) !== stable(assignment.worker));
    if (!review) return "lane_review_incomplete";
    const integration = results.find((entry) => entry.kind === "integration" && entry.status === "passed"
      && entry.revision === workerResult.revision && entry.index > review.index);
    const accepted = canonical?.deliveryEvidence?.[queuedTaskId];
    if (!integration || accepted?.sourceRevision !== workerResult.revision || !/^[a-f0-9]{40,64}$/.test(accepted.integratedRevision ?? "")) {
      return "lane_tasks_not_integrated";
    }
  }
  return null;
}

function laneLiveness(lane, observation) {
  if (lane.status === "closed") return "stopped";
  if (!lane.worker || !exactKeys(observation, ["host", "sessionId", "generation", "status"])
    || observation.host !== lane.worker.host || observation.sessionId !== lane.worker.sessionId
    || observation.generation !== lane.worker.generation || !["active", "stopped", "unknown"].includes(observation.status)) return "unknown";
  return observation.status;
}

export function summarizeLanes(canonical, { writerLiveness = {}, effectiveSettings, nativeCapacity } = {}) {
  const records = readLaneCollection(canonical?.state ?? {});
  const logicalLimit = canonical?.state?.run?.teamLimit ?? effectiveSettings?.runDefaults?.parallel_teams;
  const rotationTasks = effectiveSettings?.lanes?.rotation?.tasks;
  if (!Number.isSafeInteger(logicalLimit) || logicalLimit < 1 || !Number.isSafeInteger(rotationTasks) || rotationTasks < 1
    || nativeCapacity !== undefined && (!exactKeys(nativeCapacity, ["limit", "active", "reservedReview"])
      || ![nativeCapacity.limit, nativeCapacity.active, nativeCapacity.reservedReview].every((value) => Number.isSafeInteger(value) && value >= 0)
      || nativeCapacity.active > nativeCapacity.limit || nativeCapacity.reservedReview > nativeCapacity.limit)) throw new Error("invalid_lane_capacity");
  const rows = records.map((lane) => {
    const integrated = new Set(lane.results.filter((result) => result.kind === "integration" && result.status === "passed").map(({ taskId }) => taskId));
    const currentGeneration = lane.worker?.generation;
    const completedByCurrentWorker = new Set(lane.assignments.filter((assignment) => assignment.worker.generation === currentGeneration
      && integrated.has(assignment.taskId)).map(({ taskId }) => taskId)).size;
    return {
      id: lane.id,
      teamId: lane.teamId,
      status: lane.status,
      role: lane.role,
      currentTaskId: lane.currentTaskId,
      queue: [...lane.queue],
      remainingTaskIds: lane.queue.filter((taskId) => !integrated.has(taskId)),
      rotationCount: lane.rotationCount,
      rotationDue: lane.status === "rotation_required" || completedByCurrentWorker >= rotationTasks,
      worker: structuredClone(lane.worker),
      liveness: laneLiveness(lane, writerLiveness[lane.id]),
      worktree: lane.worktree,
      branch: lane.branch,
      brief: structuredClone(lane.brief),
      handover: structuredClone(lane.handover),
      laneFingerprint: laneFingerprint(lane),
    };
  });
  const open = rows.filter(({ status }) => status !== "closed");
  const openWork = open.filter(({ role }) => !reviewerRoles.has(role));
  return {
    rows,
    logical: { limit: logicalLimit, occupied: openWork.length, free: Math.max(0, logicalLimit - openWork.length) },
    native: nativeCapacity === undefined ? null : {
      limit: nativeCapacity.limit,
      occupied: nativeCapacity.active,
      reservedReview: nativeCapacity.reservedReview,
      unknown: open.filter(({ liveness }) => liveness === "unknown").length,
      free: Math.max(0, nativeCapacity.limit - nativeCapacity.active - nativeCapacity.reservedReview
        - open.filter(({ liveness }) => liveness === "unknown").length),
    },
  };
}

export function validateLaneBrief(source, { maxWords = 6000 } = {}) {
  if (typeof source !== "string" || !Number.isSafeInteger(maxWords) || maxWords < 1) return "invalid_lane_brief";
  const words = source.trim() ? source.trim().split(/\s+/).length : 0;
  if (words > maxWords) return "lane_brief_too_large";
  const sections = new Map();
  const matches = [...source.matchAll(/^##\s+(.+?)\s*$\n([\s\S]*?)(?=^##\s+|\s*$)/gmi)];
  for (const match of matches) {
    const heading = match[1].trim().toLowerCase();
    if (sections.has(heading)) return "invalid_lane_brief";
    sections.set(heading, match[2].trim());
  }
  if (!requiredBriefSections.every((heading) => sections.get(heading))) return "invalid_lane_brief";
  if (!["required skills", "applicable instructions"].every((heading) => {
    const lines = sections.get(heading).split(/\r?\n/).map((line) => line.trim().replace(/^-\s+/, "")).filter(Boolean);
    return lines.length > 0 && lines.every((line) => /^.+\s+[a-f0-9]{64}$/.test(line));
  })
    || !/^[a-f0-9]{40,64}$/.test(sections.get("context revision"))) return "invalid_lane_brief";
  return undefined;
}

export function readLaneBrief(source, options) {
  const problem = validateLaneBrief(source, options);
  if (problem) throw new Error(problem);
  const sections = new Map();
  for (const match of source.matchAll(/^##\s+(.+?)\s*$\n([\s\S]*?)(?=^##\s+|\s*$)/gmi)) sections.set(match[1].trim().toLowerCase(), match[2].trim());
  const files = (heading) => sections.get(heading).split(/\r?\n/).map((line) => line.trim().replace(/^-\s+/, "")).filter(Boolean).map((line) => {
    const match = line.match(/^(.+?)\s+([a-f0-9]{64})$/);
    return { path: match[1], sha256: match[2] };
  });
  return { contextRevision: sections.get("context revision"), requiredSkills: files("required skills"),
    applicableInstructions: files("applicable instructions") };
}

export function validateLaneHandover(source, { expected, maxWords = 500 } = {}) {
  if (typeof source !== "string" || !Number.isSafeInteger(maxWords) || maxWords < 1) return "invalid_lane_handover";
  const words = source.trim() ? source.trim().split(/\s+/).length : 0;
  if (words > maxWords) return "lane_handover_too_large";
  if (!exactKeys(expected, ["laneId", "sequence", "revision"]) || !validLaneId(expected.laneId)
    || !Number.isSafeInteger(expected.sequence) || expected.sequence < 1 || !/^[a-f0-9]{40,64}$/.test(expected.revision ?? "")) {
    return "invalid_lane_handover";
  }
  const sections = new Map();
  for (const match of source.matchAll(/^##\s+(.+?)\s*$\n([\s\S]*?)(?=^##\s+|\s*$)/gmi)) {
    const heading = match[1].trim().toLowerCase();
    if (sections.has(heading)) return "invalid_lane_handover";
    sections.set(heading, match[2].trim());
  }
  const required = ["voice", "gotchas", "open threads", "decisions", "exact revision"];
  if (sections.size !== required.length || !required.every((heading) => sections.get(heading))
    || sections.get("exact revision") !== expected.revision) return "invalid_lane_handover";
  return undefined;
}

const packetHeadings = ["acceptance", "delta", "new decisions", "revision pointers"];
const packetLabels = ["Assignment", "Task", "Attempt", "Revision", "Tracker fingerprint", "Lane fingerprint", "Brief fingerprint"];

function parseSections(source) {
  const sections = new Map();
  for (const match of source.matchAll(/^##\s+(.+?)\s*$\n([\s\S]*?)(?=^##\s+|\s*$)/gmi)) {
    const heading = match[1].trim().toLowerCase();
    if (sections.has(heading)) throw new Error("invalid_lane_packet");
    sections.set(heading, match[2].trim());
  }
  return sections;
}

function parseLabel(source, label) {
  const matches = [...source.matchAll(new RegExp(`^${label}:\\s*(.+)$`, "gmi"))];
  if (matches.length !== 1) throw new Error("invalid_lane_packet");
  return matches[0][1].trim();
}

export function readLanePacket(source, { expected, priorDecisions = [], maxWords = 400 } = {}) {
  if (typeof source !== "string" || !Number.isSafeInteger(maxWords) || maxWords < 1
    || source.trim().split(/\s+/).length > maxWords) throw new Error("lane_packet_too_large");
  if (!exactKeys(expected, ["assignmentId", "taskId", "attempt", "revision", "trackerFingerprint", "laneFingerprint", "briefSha256"])
    || !validId(expected.assignmentId) || !validId(expected.taskId) || !Number.isSafeInteger(expected.attempt) || expected.attempt < 1
    || !/^[a-f0-9]{40,64}$/.test(expected.revision ?? "")
    || ![expected.trackerFingerprint, expected.laneFingerprint, expected.briefSha256].every(hex)) throw new Error("invalid_lane_packet");
  const labels = Object.fromEntries(packetLabels.map((label) => [label, parseLabel(source, label)]));
  if (labels.Assignment !== expected.assignmentId || labels.Task !== expected.taskId || Number(labels.Attempt) !== expected.attempt
    || labels.Revision !== expected.revision || labels["Tracker fingerprint"] !== expected.trackerFingerprint
    || labels["Lane fingerprint"] !== expected.laneFingerprint || labels["Brief fingerprint"] !== expected.briefSha256) throw new Error("invalid_lane_packet");
  const sections = parseSections(source);
  if (sections.has("brief") || sections.size !== packetHeadings.length || !packetHeadings.every((heading) => sections.get(heading))) {
    throw new Error("invalid_lane_packet");
  }
  if (!Array.isArray(priorDecisions) || !priorDecisions.every((entry) => exactKeys(entry, ["id", "kind", "supersedes"])
    && validId(entry.id) && ["new", "supersedes"].includes(entry.kind))) throw new Error("invalid_lane_packet");
  const known = new Set(priorDecisions.map(({ id }) => id));
  const decisionSource = sections.get("new decisions");
  const decisions = [];
  for (const line of /^(?:none\.?)$/i.test(decisionSource) ? [] : decisionSource.split(/\r?\n/).map((entry) => entry.trim()).filter(Boolean)) {
    const match = line.match(/^-\s+([A-Za-z0-9][\w.:-]{0,127})\s*\|\s*(new|supersedes)\s*\|\s*([A-Za-z0-9][\w.:-]{0,127}|none)$/);
    if (!match) throw new Error("invalid_lane_packet");
    const decision = { id: match[1], kind: match[2], supersedes: match[3] === "none" ? null : match[3] };
    if (known.has(decision.id) || decision.kind === "new" && decision.supersedes !== null
      || decision.kind === "supersedes" && (!decision.supersedes || !known.has(decision.supersedes) || decision.supersedes === decision.id)) {
      throw new Error("decision_reversal");
    }
    known.add(decision.id);
    decisions.push(decision);
  }
  return { ...structuredClone(expected), decisions, wordCount: source.trim().split(/\s+/).length };
}

export function validateLanePacket(source, options) {
  try { readLanePacket(source, options); return undefined; }
  catch (error) { return ["lane_packet_too_large", "decision_reversal"].includes(error.message) ? error.message : "invalid_lane_packet"; }
}
