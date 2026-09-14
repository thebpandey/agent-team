import { createHash } from "node:crypto";
import { execFile } from "node:child_process";
import { constants } from "node:fs";
import { open, realpath } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import { promisify } from "node:util";

import { loadCanonicalState, loadCanonicalTracker } from "./canonical-state.mjs";
import {
  laneCollectionFingerprint,
  laneCleanupBlocker,
  laneFingerprint,
  laneOwnsTask,
  readLaneCollection,
  readLaneBrief,
  readLanePacket,
  summarizeLanes,
  validateLaneBrief,
  validateLaneCollection,
  validateLaneFactReferences,
  validateLaneHandover,
  validateLanePacket,
} from "./lane-schema.mjs";
import { effectiveRunFingerprint, validateEffectiveRun } from "./run-state.mjs";
import { mutateOperationalState, transitionTaskInState } from "./task-transitions.mjs";

export {
  laneCollectionFingerprint,
  laneCleanupBlocker,
  laneFingerprint,
  laneOwnsTask,
  readLaneCollection,
  readLaneBrief,
  readLanePacket,
  summarizeLanes,
  validateLaneBrief,
  validateLaneCollection,
  validateLaneFactReferences,
  validateLaneHandover,
  validateLanePacket,
};

const run = promisify(execFile);
const emptyLanes = () => ({ schemaVersion: 1, records: [] });
const stable = (value) => JSON.stringify(value && typeof value === "object"
  ? Array.isArray(value) ? value.map((entry) => JSON.parse(stable(entry)))
    : Object.fromEntries(Object.keys(value).sort().map((key) => [key, JSON.parse(stable(value[key]))])) : value);
const exactKeys = (value, keys) => value && typeof value === "object" && !Array.isArray(value)
  && Object.keys(value).sort().join("\0") === [...keys].sort().join("\0");
const hex = (value, length) => typeof value === "string" && new RegExp(`^[a-f0-9]{${length}}$`).test(value);
const conflict = (reason) => ({ status: "conflict", reason });
const reviewerRoles = new Set(["reviewer", "visual_reviewer"]);
const trackerOptions = (options, state) => ({ ...options,
  ...(options.nativeIdentity?.host ? { host: options.nativeIdentity.host === "claude" ? "claude-code" : options.nativeIdentity.host } : {}),
  ...(state?.run?.executionSettings ? { executionSettings: state.run.executionSettings } : {}),
});

async function sealedBytes(file, maximum = 1024 * 1024) {
  let handle;
  try {
    handle = await open(file, constants.O_RDONLY | constants.O_NOFOLLOW | constants.O_NONBLOCK);
    const stat = await handle.stat();
    if (!stat.isFile() || stat.size > maximum) throw new Error("unsafe_lane_evidence");
    const bytes = Buffer.alloc(stat.size);
    let offset = 0;
    while (offset < bytes.length) {
      const { bytesRead } = await handle.read(bytes, offset, bytes.length - offset, offset);
      if (!bytesRead) break;
      offset += bytesRead;
    }
    if (offset !== bytes.length) throw new Error("lane_evidence_changed");
    return bytes;
  } finally { await handle?.close(); }
}

function validOwnedPath(value) {
  const prefix = typeof value === "string" && value.endsWith("/**") ? value.slice(0, -3) : value;
  return typeof prefix === "string" && prefix.length > 0 && !path.isAbsolute(prefix)
    && path.posix.normalize(prefix) === prefix && !prefix.split("/").includes("..") && !/[\r\n\0]/.test(prefix);
}

function overlaps(first, second) {
  if (first === "*" || second === "*") return true;
  const left = first.endsWith("/**") ? first.slice(0, -3) : first;
  const right = second.endsWith("/**") ? second.slice(0, -3) : second;
  return left === right || first.endsWith("/**") && right.startsWith(`${left}/`) || second.endsWith("/**") && left.startsWith(`${right}/`);
}

function requestProblem(request) {
  const keys = ["schemaVersion", "operationId", "expectedTrackerFingerprint", "expectedRunFingerprint", "expectedRevision", "expectedLanesFingerprint", "lane", "reason"];
  const laneKeys = ["id", "teamId", "role", "model", "effort", "queue", "worker", "worktree", "branch", "brief", "ownershipEvidence", "factSheets"];
  if (!exactKeys(request, keys) || request.schemaVersion !== 1 || !exactKeys(request.lane, laneKeys) || !/^[\w.:-]{1,128}$/.test(request.operationId ?? "")
    || ![request.expectedTrackerFingerprint, request.expectedRunFingerprint, request.expectedLanesFingerprint].every((value) => hex(value, 64))
    || !hex(request.expectedRevision, 40) || typeof request.reason !== "string" || request.reason.trim() !== request.reason
    || !request.reason || Buffer.byteLength(request.reason) > 4096) return "invalid_request";
  const candidate = { schemaVersion: 1, ...structuredClone(request.lane), status: "prepared", currentTaskId: null, rotationCount: 0,
    handover: null, assignments: [], results: [] };
  return validateLaneCollection({ schemaVersion: 1,
    records: [reviewerRoles.has(candidate.role) ? { ...candidate, role: "developer" } : candidate] });
}

async function gitValue(cwd, ...args) {
  return (await run("git", ["-C", cwd, ...args], { encoding: "utf8", timeout: 1500, maxBuffer: 16384 })).stdout.trim();
}

async function validatePreparedWorktree(project, lane, state, expectedRevision) {
  let worktree;
  try { worktree = await realpath(lane.worktree); } catch { return "lane_worktree_unavailable"; }
  if (worktree !== lane.worktree || worktree === project.root) return "lane_worktree_invalid";
  try {
    const [top, commonRaw, branch, head, base] = await Promise.all([
      gitValue(worktree, "rev-parse", "--show-toplevel"),
      gitValue(worktree, "rev-parse", "--git-common-dir"),
      gitValue(worktree, "branch", "--show-current"),
      gitValue(worktree, "rev-parse", "HEAD"),
      gitValue(project.root, "rev-parse", state.integration?.baseRef ?? "main"),
    ]);
    const common = await realpath(path.isAbsolute(commonRaw) ? commonRaw : path.resolve(worktree, commonRaw));
    if (await realpath(top) !== worktree || common !== project.commonDirectory || branch !== lane.branch
      || head !== expectedRevision || base !== expectedRevision) return "lane_worktree_invalid";
  } catch { return "lane_worktree_unavailable"; }
  return undefined;
}

async function validateLaneFiles(project, lane, team, expectedRevision, briefMaxWords, otherTeams) {
  const ownedPaths = String(team["owned paths"] ?? "").split(/\s*,\s*/).filter(Boolean);
  if (!ownedPaths.length || ownedPaths.some((entry) => !validOwnedPath(entry))) return "owned_paths_invalid";
  for (const other of otherTeams) {
    const otherPaths = String(other["owned paths"] ?? "").split(/\s*,\s*/).filter(Boolean);
    if (ownedPaths.some((left) => otherPaths.some((right) => overlaps(left, right)))) return "owned_paths_overlap";
  }
  let brief; let evidenceBytes; let evidence;
  try {
    brief = await sealedBytes(path.join(project.root, lane.brief.path));
    evidenceBytes = await sealedBytes(path.join(project.root, lane.ownershipEvidence.path));
    evidence = JSON.parse(evidenceBytes);
  } catch { return "lane_evidence_unavailable"; }
  if (createHash("sha256").update(brief).digest("hex") !== lane.brief.sha256
    || validateLaneBrief(brief.toString("utf8"), { maxWords: briefMaxWords })) return "lane_brief_invalid";
  const briefRecord = readLaneBrief(brief.toString("utf8"), { maxWords: briefMaxWords });
  if (briefRecord.contextRevision !== expectedRevision) return "lane_brief_context_stale";
  const skillRoots = await Promise.all([path.join(os.homedir(), ".agents", "skills"), path.join(os.homedir(), ".codex", "skills")]
    .map((root) => realpath(root).catch(() => null)));
  for (const [kind, entry] of [...briefRecord.requiredSkills.map((value) => ["skill", value]),
    ...briefRecord.applicableInstructions.map((value) => ["instruction", value])]) {
    let target; let resolved;
    try {
      if (path.isAbsolute(entry.path)) {
        if (kind !== "skill") return "lane_instruction_unsafe";
        target = path.normalize(entry.path);
        resolved = await realpath(target);
        if (!skillRoots.some((root) => root && (resolved === root || resolved.startsWith(`${root}${path.sep}`)))) return "lane_instruction_unsafe";
      } else {
        if (path.posix.normalize(entry.path) !== entry.path || entry.path.split("/").includes("..")) return "lane_instruction_unsafe";
        target = path.resolve(project.root, entry.path);
        resolved = await realpath(target);
        if (resolved !== target || !resolved.startsWith(`${project.root}${path.sep}`)) return "lane_instruction_unsafe";
      }
      if (kind === "skill" && path.basename(resolved) !== "SKILL.md") return "lane_instruction_unsafe";
      const bytes = await sealedBytes(resolved, 1024 * 1024);
      if (createHash("sha256").update(bytes).digest("hex") !== entry.sha256) return "lane_instruction_changed";
    } catch { return "lane_instruction_unavailable"; }
  }
  if (createHash("sha256").update(evidenceBytes).digest("hex") !== lane.ownershipEvidence.sha256
    || !exactKeys(evidence, ["schemaVersion", "status", "source", "revision", "ownedPaths", "pathSetHash", "conflicts", "observedAt"])
    || evidence.schemaVersion !== 1 || evidence.status !== "resolved" || evidence.source !== "graphify"
    || evidence.revision !== expectedRevision || stable(evidence.ownedPaths) !== stable(ownedPaths)
    || evidence.pathSetHash !== createHash("sha256").update(JSON.stringify(ownedPaths)).digest("hex")
    || evidence.pathSetHash !== lane.ownershipEvidence.pathSetHash || evidence.revision !== lane.ownershipEvidence.revision
    || !Array.isArray(evidence.conflicts) || evidence.conflicts.length !== 0
    || typeof evidence.observedAt !== "string" || !Number.isFinite(Date.parse(evidence.observedAt))) return "ownership_evidence_unresolved";
  for (const sheet of lane.factSheets) {
    let bytes; let record;
    try {
      bytes = await sealedBytes(path.join(project.root, sheet.path), 128 * 1024);
      record = JSON.parse(bytes);
    } catch { return "fact_sheet_unavailable"; }
    if (createHash("sha256").update(bytes).digest("hex") !== sheet.sha256
      || !exactKeys(record, ["schemaVersion", "id", "ownerLaneId", "facts"])
      || record.schemaVersion !== 1 || record.id !== sheet.id || record.ownerLaneId !== lane.id
      || stable(record.facts) !== stable(sheet.facts)) return "fact_sheet_invalid";
  }
  return undefined;
}

/** Bind an already prepared worktree and registered team without claiming any queued tracker task. */
export async function createLane(project, request, options = {}) {
  const problem = requestProblem(request);
  if (problem) return conflict(problem);
  const effectiveRequest = { ...request, actorSessionId: options.actorSessionId, expectedVersion: options.expectedVersion };
  return mutateOperationalState(project, effectiveRequest, async (state, canonical) => {
    const lanes = state.lanes ?? emptyLanes();
    if (validateLaneCollection(lanes)) return conflict("invalid_lanes");
    if (laneCollectionFingerprint(lanes) !== request.expectedLanesFingerprint) return conflict("stale_lanes");
    if (!state.run?.executionSettings || state.run.executionSettings.lanes?.enabled !== true) return conflict("run_settings_reconcile_required");
    const tracker = await loadCanonicalTracker(project, trackerOptions(options, state));
    if (tracker.tracker.status !== "current") return { status: "unavailable", reason: "tracker_unavailable" };
    if (tracker.tracker.fingerprint !== request.expectedTrackerFingerprint) return conflict("stale_tracker");
    if (effectiveRunFingerprint(state.run) !== request.expectedRunFingerprint || validateEffectiveRun(state.run, tracker.tasks)) return conflict("stale_run");
    const revision = await gitValue(project.root, "rev-parse", "HEAD");
    if (revision !== request.expectedRevision) return conflict("stale_revision");
    const reviewLane = reviewerRoles.has(request.lane.role);
    if (lanes.records.some(({ id }) => id === request.lane.id)) return conflict("lane_identity_conflict");
    const sameKind = lanes.records.filter((lane) => reviewerRoles.has(lane.role) === reviewLane);
    if (sameKind.some(({ queue }) => queue.some((id) => request.lane.queue.includes(id)))) return conflict("lane_identity_conflict");
    if (reviewLane && (lanes.records.some((lane) => reviewerRoles.has(lane.role) && lane.status !== "closed")
      || request.lane.queue.some((taskId) => !lanes.records.some((lane) => !reviewerRoles.has(lane.role) && lane.queue.includes(taskId))))) {
      return conflict("review_lane_source_required");
    }
    const open = lanes.records.filter(({ status, role }) => status !== "closed" && !reviewerRoles.has(role)).length;
    if (!reviewLane && open >= state.run.teamLimit) return conflict("lane_capacity_reserved");
    if (request.lane.queue.some((id) => !state.run.taskIds.includes(id) || !tracker.tasks.some((task) => task.id === id))) return conflict("outside_scope");
    const team = canonical.registry.teams.find((entry) => entry["team id"] === request.lane.teamId);
    if (!team || team.session !== request.lane.worker.sessionId || team.branch !== request.lane.branch
      || stable(String(team.tasks ?? "").split(/\s*,\s*/).filter(Boolean)) !== stable(request.lane.queue)) return conflict("registered_team_required");
    let registeredWorktree;
    try { registeredWorktree = await realpath(path.resolve(project.root, team.worktree)); } catch { return conflict("registered_team_required"); }
    if (registeredWorktree !== request.lane.worktree) return conflict("registered_team_required");
    const worktreeProblem = await validatePreparedWorktree(project, request.lane, state, request.expectedRevision);
    if (worktreeProblem) return conflict(worktreeProblem);
    const fileProblem = await validateLaneFiles(project, request.lane, team, request.expectedRevision,
      state.run.executionSettings.lanes.briefMaxWords, canonical.registry.teams.filter((entry) => entry !== team));
    if (fileProblem) return conflict(fileProblem);
    const lane = { schemaVersion: 1, ...structuredClone(request.lane), status: "prepared", currentTaskId: null, rotationCount: 0,
      handover: null, assignments: [], results: [] };
    return { state: { ...state, lanes: { schemaVersion: 1, records: [...lanes.records, lane] } },
      result: { lane: structuredClone(lane), laneFingerprint: laneFingerprint(lane), trackerFingerprint: tracker.tracker.fingerprint,
        runFingerprint: effectiveRunFingerprint(state.run), preparedOnly: true } };
  }, options);
}

function prepareRequestProblem(request) {
  const keys = ["schemaVersion", "phase", "operationId", "laneId", "assignmentId", "attempt", "expectedTrackerFingerprint", "expectedRunFingerprint",
    "expectedRevision", "expectedLaneFingerprint", "packet", "factSheets", "reason"];
  if (!(exactKeys(request, keys) || exactKeys(request, [...keys, "sourceAssignment"]))
    || request.schemaVersion !== 1 || request.phase !== "prepare"
    || ![request.operationId, request.laneId, request.assignmentId].every((value) => typeof value === "string" && /^[\w.:-]{1,128}$/.test(value))
    || !Number.isSafeInteger(request.attempt) || request.attempt < 1
    || ![request.expectedTrackerFingerprint, request.expectedRunFingerprint, request.expectedLaneFingerprint].every((value) => hex(value, 64))
    || !hex(request.expectedRevision, 40) || !exactKeys(request.packet, ["path", "sha256"]) || !hex(request.packet.sha256, 64)
    || !Array.isArray(request.factSheets)
    || typeof request.reason !== "string" || request.reason.trim() !== request.reason || !request.reason
    || Buffer.byteLength(request.reason) > 4096) return "invalid_request";
  return undefined;
}

function validSourceAssignment(value) {
  return exactKeys(value, ["laneId", "assignmentId", "packetSha256", "revision"])
    && typeof value.laneId === "string" && /^[a-z0-9][a-z0-9._-]{0,63}$/.test(value.laneId)
    && typeof value.assignmentId === "string" && /^[\w.:-]{1,128}$/.test(value.assignmentId)
    && hex(value.packetSha256, 64) && /^[a-f0-9]{40,64}$/.test(value.revision ?? "");
}

function boundedReason(request) {
  return typeof request.reason === "string" && request.reason.trim() === request.reason && request.reason
    && Buffer.byteLength(request.reason) <= 4096;
}

function validWorkerBinding(value) {
  return exactKeys(value, ["host", "sessionId", "generation"]) && ["codex", "claude-code"].includes(value.host)
    && typeof value.sessionId === "string" && /^[\w.:-]{1,128}$/.test(value.sessionId)
    && Number.isSafeInteger(value.generation) && value.generation > 0;
}

function dispatchRequestProblem(request) {
  if (!exactKeys(request, ["schemaVersion", "phase", "operationId", "laneId", "assignmentId", "expectedLaneFingerprint",
    "expectedRevision", "expectedPacketSha256", "worker", "observation", "reason"])
    || request.schemaVersion !== 1 || request.phase !== "dispatch"
    || ![request.operationId, request.laneId, request.assignmentId].every((value) => typeof value === "string" && /^[\w.:-]{1,128}$/.test(value))
    || !hex(request.expectedLaneFingerprint, 64) || !hex(request.expectedRevision, 40) || !hex(request.expectedPacketSha256, 64)
    || !validWorkerBinding(request.worker) || !exactKeys(request.observation, ["status", "source", "eventId", "observedAt"])
    || !Number.isFinite(Date.parse(request.observation.observedAt ?? ""))
    || !(request.observation.status === "observed" && request.observation.source === request.worker.host
      && typeof request.observation.eventId === "string" && /^[\w.:-]{1,128}$/.test(request.observation.eventId)
      || request.observation.status === "unknown" && request.observation.source === "unavailable" && request.observation.eventId === null)
    || !boundedReason(request)) return "invalid_request";
  return undefined;
}

function resultRequestProblem(request) {
  if (!exactKeys(request, ["schemaVersion", "phase", "operationId", "laneId", "assignmentId", "expectedLaneFingerprint",
    "expectedPacketSha256", "result", "reason"])
    || request.schemaVersion !== 1 || request.phase !== "result"
    || ![request.operationId, request.laneId, request.assignmentId].every((value) => typeof value === "string" && /^[\w.:-]{1,128}$/.test(value))
    || !hex(request.expectedLaneFingerprint, 64) || !hex(request.expectedPacketSha256, 64)
    || !exactKeys(request.result, ["kind", "status", "revision", "worker", "evidence"])
    || !["worker", "verification", "independent_review", "integration", "unresolved"].includes(request.result.kind)
    || !["passed", "failed", "unresolved"].includes(request.result.status) || !hex(request.result.revision, 40)
    || !validWorkerBinding(request.result.worker) || !exactKeys(request.result.evidence, ["path", "sha256"])
    || typeof request.result.evidence.path !== "string" || !hex(request.result.evidence.sha256, 64) || !boundedReason(request)) return "invalid_request";
  if (request.result.kind === "unresolved" ? request.result.status !== "unresolved" : request.result.status === "unresolved") return "invalid_request";
  return undefined;
}

function bindRequestProblem(request) {
  if (!exactKeys(request, ["schemaVersion", "phase", "operationId", "laneId", "expectedLaneFingerprint", "expectedRevision",
    "expectedHandoverSha256", "replacementWorker", "reason"])
    || request.schemaVersion !== 1 || request.phase !== "bind"
    || ![request.operationId, request.laneId].every((value) => typeof value === "string" && /^[\w.:-]{1,128}$/.test(value))
    || !hex(request.expectedLaneFingerprint, 64) || !hex(request.expectedRevision, 40) || !hex(request.expectedHandoverSha256, 64)
    || !validWorkerBinding(request.replacementWorker) || !boundedReason(request)) return "invalid_request";
  return undefined;
}

function exactPacketPath(lane, taskId, attempt) {
  return `.agent-team/lanes/${lane.id}/packets/${taskId}-${attempt}.md`;
}

async function readPreparedPacket(project, request, lane, taskId, trackerFingerprint, priorDecisions) {
  const expected = { assignmentId: request.assignmentId, taskId, attempt: request.attempt, revision: request.expectedRevision,
    trackerFingerprint, laneFingerprint: request.expectedLaneFingerprint, briefSha256: lane.brief.sha256 };
  if (request.packet.path !== exactPacketPath(lane, taskId, request.attempt)) return { problem: "invalid_request" };
  let bytes;
  try { bytes = await sealedBytes(path.join(project.root, request.packet.path), 128 * 1024); }
  catch { return { problem: "lane_packet_unavailable" }; }
  if (createHash("sha256").update(bytes).digest("hex") !== request.packet.sha256) return { problem: "lane_packet_changed" };
  try { return { packet: readLanePacket(bytes.toString("utf8"), { expected, priorDecisions }) }; }
  catch (error) { return { problem: error.message }; }
}

async function validateBoundFacts(project, lane, references, staleDays, now) {
  const referenceProblem = validateLaneFactReferences(lane, references);
  if (referenceProblem) return referenceProblem;
  if (!Number.isSafeInteger(staleDays) || staleDays < 1) return "fact_sheet_settings_invalid";
  const observedAt = new Date(now()).getTime();
  if (!Number.isFinite(observedAt)) return "fact_sheet_time_invalid";
  for (const reference of references) {
    const sheet = lane.factSheets.find(({ id }) => id === reference.id);
    let sheetBytes; let citationBytes;
    try {
      [sheetBytes, citationBytes] = await Promise.all([
        sealedBytes(path.join(project.root, sheet.path), 128 * 1024),
        sealedBytes(path.join(project.root, reference.citation.path), 128 * 1024),
      ]);
    } catch { return "fact_evidence_unavailable"; }
    if (createHash("sha256").update(sheetBytes).digest("hex") !== reference.sha256) return "fact_sheet_changed";
    if (createHash("sha256").update(citationBytes).digest("hex") !== reference.citation.sha256) return "fact_citation_changed";
    for (const factId of reference.factIds) {
      const checkedOn = new Date(sheet.facts.find(({ id }) => id === factId).checkedOn).getTime();
      const age = observedAt - checkedOn;
      if (age < 0) return "fact_sheet_time_invalid";
      if (age > staleDays * 24 * 60 * 60 * 1000) return "fact_sheet_stale";
    }
  }
  return undefined;
}

/** Claim the next tracker task and persist the lane assignment in the same recoverable state intent. */
async function prepareLane(project, request, options = {}) {
  const problem = prepareRequestProblem(request);
  if (problem) return conflict(problem);
  const effectiveRequest = { ...request, actorSessionId: options.actorSessionId, expectedVersion: options.expectedVersion };
  return mutateOperationalState(project, effectiveRequest, async (state, canonical, { persistIntent }) => {
    const lanes = state.lanes;
    if (validateLaneCollection(lanes)) return conflict("invalid_lanes");
    let lane = lanes.records.find(({ id }) => id === request.laneId);
    if (!lane) return conflict("lane_not_found");
    const reviewLane = reviewerRoles.has(lane.role);
    if (reviewLane !== (request.sourceAssignment !== undefined) || reviewLane && !validSourceAssignment(request.sourceAssignment)) {
      return conflict(reviewLane ? "review_source_assignment_required" : "invalid_request");
    }
    if (lane.status === "closed") return conflict("lane_closed");
    if (lane.status === "rotation_required") return conflict("lane_rotation_required");
    const pending = state.pendingOperations?.[request.operationId];
    let assignment = lane.assignments.find(({ id }) => id === request.assignmentId);
    let taskId;
    let tracker;
    if (pending) {
      if (pending.kind !== "lane_tracker_transition" || pending.laneId !== lane.id || pending.assignmentId !== request.assignmentId || !assignment) {
        return conflict("lane_intent_mismatch");
      }
      taskId = assignment.taskId;
      tracker = await loadCanonicalTracker(project, trackerOptions(options, state));
    } else {
      if (laneFingerprint(lane) !== request.expectedLaneFingerprint) return conflict("stale_lane");
      if (!state.run?.executionSettings || effectiveRunFingerprint(state.run) !== request.expectedRunFingerprint) return conflict("stale_run");
      tracker = await loadCanonicalTracker(project, trackerOptions(options, state));
      if (tracker.tracker.status !== "current") return { status: "unavailable", reason: "tracker_unavailable" };
      if (tracker.tracker.fingerprint !== request.expectedTrackerFingerprint) return conflict("stale_tracker");
      if (validateEffectiveRun(state.run, tracker.tasks)) return conflict("stale_run");
      const revision = await gitValue(project.root, "rev-parse", "HEAD");
      if (revision !== request.expectedRevision) return conflict("stale_revision");
      if (reviewLane) {
        const sourceLane = lanes.records.find(({ id }) => id === request.sourceAssignment.laneId);
        const sourceAssignment = sourceLane?.assignments.find(({ id }) => id === request.sourceAssignment.assignmentId);
        const sourceResults = sourceLane?.results.filter(({ assignmentId }) => assignmentId === sourceAssignment?.id) ?? [];
        const sourceWorker = sourceResults.find(({ kind, status, revision: resultRevision }) => kind === "worker" && status === "passed"
          && resultRevision === request.sourceAssignment.revision);
        const verification = sourceResults.find(({ kind, status, revision: resultRevision }) => kind === "verification" && status === "passed"
          && resultRevision === request.sourceAssignment.revision);
        if (!sourceLane || reviewerRoles.has(sourceLane.role) || !sourceAssignment || sourceAssignment.taskId !== lane.queue.find((taskId) =>
          !lane.results.some(({ kind, status, taskId: reviewedTask }) => kind === "independent_review" && status === "passed" && reviewedTask === taskId))
          || sourceAssignment.packet.sha256 !== request.sourceAssignment.packetSha256 || !sourceWorker || !verification
          || sourceResults.some(({ kind }) => kind === "independent_review")) return conflict("review_source_assignment_unready");
        taskId = sourceAssignment.taskId;
        const expectedAttempt = lane.assignments.filter((entry) => entry.taskId === taskId).length + 1;
        if (request.attempt !== expectedAttempt || lane.currentTaskId !== null && lane.currentTaskId !== taskId) return conflict("stale_attempt");
        const packetResult = await readPreparedPacket(project, request, lane, taskId, tracker.tracker.fingerprint,
          lane.assignments.flatMap(({ decisions }) => decisions));
        if (packetResult.problem) return conflict(packetResult.problem);
        if (packetResult.packet.decisions.length || request.factSheets.length) return conflict("review_packet_scope_invalid");
        const createdAt = (options.now ?? (() => new Date().toISOString()))();
        assignment = { id: request.assignmentId, taskId, attempt: request.attempt, packet: structuredClone(request.packet),
          briefSha256: lane.brief.sha256, revision: request.expectedRevision, worker: structuredClone(lane.worker), decisions: [], factSheets: [],
          sourceAssignment: structuredClone(request.sourceAssignment), status: "prepared", createdAt, dispatchedAt: null, dispatch: null };
        lane = { ...lane, status: "active", currentTaskId: taskId, assignments: [...lane.assignments, assignment] };
        state.lanes = { ...lanes, records: lanes.records.map((entry) => entry.id === lane.id ? lane : entry) };
        if (validateLaneCollection(state.lanes)) return conflict("invalid_lanes");
        return { state, result: { laneId: lane.id, assignmentId: assignment.id, sourceAssignment: structuredClone(request.sourceAssignment),
          packet: structuredClone(assignment.packet), preparedOnly: true, claimedTrackerTask: false } };
      }
      let retrying = false;
      if (lane.currentTaskId !== null) {
        const accepted = (await loadCanonicalState(project, trackerOptions(options, state))).deliveryEvidence[lane.currentTaskId];
        const integrated = accepted?.integratedRevision && lane.results.some((result) => result.taskId === lane.currentTaskId
          && result.kind === "integration" && result.status === "passed" && result.revision === accepted.sourceRevision);
        if (!integrated) {
          const prior = lane.assignments.filter(({ taskId: assignedTask }) => assignedTask === lane.currentTaskId).at(-1);
          const failed = prior && lane.results.some((result) => result.assignmentId === prior.id && result.kind === "worker" && result.status === "failed");
          const uncertain = prior && lane.results.some((result) => result.assignmentId === prior.id && result.kind === "unresolved"
            && result.status === "unresolved") && lane.worker && lane.worker.generation > prior.worker.generation;
          if (!failed && !uncertain) return conflict("prior_task_not_integrated");
          taskId = lane.currentTaskId;
          retrying = true;
        }
      }
      taskId ??= lane.queue.find((id) => !lane.assignments.some((entry) => entry.taskId === id));
      if (!taskId) return conflict("lane_queue_exhausted");
      const expectedAttempt = lane.assignments.filter((entry) => entry.taskId === taskId).length + 1;
      if (request.attempt !== expectedAttempt) return conflict("stale_attempt");
      const packetResult = await readPreparedPacket(project, request, lane, taskId, tracker.tracker.fingerprint,
        lane.assignments.flatMap(({ decisions }) => decisions));
      if (packetResult.problem) return conflict(packetResult.problem);
      const factProblem = await validateBoundFacts(project, lane, request.factSheets,
        state.run.executionSettings.lanes.factSheetStaleDays, options.now ?? (() => new Date().toISOString()));
      if (factProblem) return conflict(factProblem);
      const createdAt = (options.now ?? (() => new Date().toISOString()))();
      assignment = { id: request.assignmentId, taskId, attempt: request.attempt, packet: structuredClone(request.packet),
        briefSha256: lane.brief.sha256, revision: request.expectedRevision, worker: structuredClone(lane.worker),
        decisions: packetResult.packet.decisions, factSheets: structuredClone(request.factSheets), status: "prepared", createdAt, dispatchedAt: null, dispatch: null };
      lane = { ...lane, status: "active", currentTaskId: taskId, assignments: [...lane.assignments, assignment] };
      state.lanes = { ...lanes, records: lanes.records.map((entry) => entry.id === lane.id ? lane : entry) };
      if (validateLaneCollection(state.lanes)) return conflict("invalid_lanes");
      if (retrying) return { state, result: { laneId: lane.id, assignmentId: assignment.id,
        packet: structuredClone(assignment.packet), preparedOnly: true, claimedTrackerTask: false } };
    }
    const transition = { operationId: request.operationId, actorSessionId: options.actorSessionId, expectedVersion: options.expectedVersion,
      taskId, action: "claim", expectedFingerprint: request.expectedTrackerFingerprint, expectedOwner: "none", owner: lane.teamId };
    const changed = await transitionTaskInState(project, transition, { state, canonical,
      persistIntent: (intent) => persistIntent({ ...intent, kind: "lane_tracker_transition", laneId: lane.id,
        assignmentId: assignment.id, packet: structuredClone(assignment.packet) }),
      options: { ...trackerOptions(options, state), persistTrackerIntent: true } });
    if (changed?.status && changed.status !== "applied") return changed;
    return { state: changed.state, result: { ...changed.result, laneId: lane.id, assignmentId: assignment.id,
      packet: structuredClone(assignment.packet), preparedOnly: true } };
  }, options);
}

function replaceLane(state, lane) {
  return { ...state, lanes: { ...state.lanes, records: state.lanes.records.map((entry) => entry.id === lane.id ? lane : entry) } };
}

async function recordLaneDispatch(project, request, options) {
  const problem = dispatchRequestProblem(request);
  if (problem) return conflict(problem);
  const effectiveRequest = { ...request, actorSessionId: options.actorSessionId, expectedVersion: options.expectedVersion };
  return mutateOperationalState(project, effectiveRequest, async (state, canonical) => {
    if (validateLaneCollection(state.lanes)) return conflict("invalid_lanes");
    const lane = state.lanes.records.find(({ id }) => id === request.laneId);
    if (!lane) return conflict("lane_not_found");
    if (laneFingerprint(lane) !== request.expectedLaneFingerprint) return conflict("stale_lane");
    const assignment = lane.assignments.find(({ id }) => id === request.assignmentId);
    if (!assignment || assignment.taskId !== lane.currentTaskId) return conflict("assignment_identity_mismatch");
    if (assignment.status !== "prepared") return conflict("dispatch_out_of_order");
    if (assignment.revision !== request.expectedRevision || assignment.packet.sha256 !== request.expectedPacketSha256) return conflict("assignment_binding_mismatch");
    if (stable(assignment.worker) !== stable(request.worker) || stable(lane.worker) !== stable(request.worker)) return conflict("worker_identity_mismatch");
    const team = canonical.registry.teams.find((entry) => entry["team id"] === lane.teamId);
    if (!team || team.session !== request.worker.sessionId || team.branch !== lane.branch) return conflict("registered_team_required");
    const factProblem = await validateBoundFacts(project, lane, assignment.factSheets,
      state.run.executionSettings?.lanes?.factSheetStaleDays, options.now ?? (() => new Date().toISOString()));
    if (factProblem) return conflict(factProblem);
    const changed = { ...assignment, status: "dispatched", dispatchedAt: request.observation.observedAt,
      dispatch: structuredClone(request.observation) };
    const nextLane = { ...lane, assignments: lane.assignments.map((entry) => entry.id === assignment.id ? changed : entry) };
    const nextState = replaceLane(state, nextLane);
    if (validateLaneCollection(nextState.lanes)) return conflict("invalid_lanes");
    return { state: nextState, result: { laneId: lane.id, assignmentId: assignment.id, dispatch: structuredClone(changed.dispatch) } };
  }, options);
}

async function bindLaneWorker(project, request, options) {
  const problem = bindRequestProblem(request);
  if (problem) return conflict(problem);
  const effectiveRequest = { ...request, actorSessionId: options.actorSessionId, expectedVersion: options.expectedVersion };
  return mutateOperationalState(project, effectiveRequest, async (state, canonical) => {
    if (validateLaneCollection(state.lanes)) return conflict("invalid_lanes");
    const lane = state.lanes.records.find(({ id }) => id === request.laneId);
    if (!lane) return conflict("lane_not_found");
    if (laneFingerprint(lane) !== request.expectedLaneFingerprint) return conflict("stale_lane");
    if (lane.status !== "rotation_required" || lane.worker !== null || !lane.handover) return conflict("lane_binding_not_required");
    if (lane.handover.sha256 !== request.expectedHandoverSha256 || lane.handover.revision !== request.expectedRevision) {
      return conflict("handover_binding_mismatch");
    }
    if (request.replacementWorker.generation !== lane.rotationCount + 1) return conflict("worker_generation_mismatch");
    const team = canonical.registry.teams.find((entry) => entry["team id"] === lane.teamId);
    if (!team || team.session !== request.replacementWorker.sessionId || team.branch !== lane.branch) return conflict("registered_team_required");
    const head = await gitValue(lane.worktree, "rev-parse", "HEAD").catch(() => null);
    if (head !== request.expectedRevision) return conflict("stale_revision");
    const changed = { ...lane, worker: structuredClone(request.replacementWorker), status: lane.currentTaskId === null ? "prepared" : "active" };
    const nextState = replaceLane(state, changed);
    if (validateLaneCollection(nextState.lanes)) return conflict("invalid_lanes");
    return { state: nextState, result: { laneId: lane.id, replacementWorker: structuredClone(request.replacementWorker),
      currentTaskId: lane.currentTaskId, claimedTrackerTask: false } };
  }, options);
}

function priorPassed(results, kind, revision) {
  return results.some((entry) => entry.kind === kind && entry.status === "passed" && entry.revision === revision);
}

function nonempty(value, maximum = 4096) {
  return typeof value === "string" && value.trim() === value && value.length > 0 && Buffer.byteLength(value) <= maximum;
}

function validVerificationEvidence(value, request, assignment) {
  return exactKeys(value, ["schemaVersion", "assignmentId", "taskId", "revision", "verifier", "source", "checks", "status"])
    && value.schemaVersion === 1 && value.assignmentId === assignment.id && value.taskId === assignment.taskId
    && value.revision === request.result.revision && stable(value.verifier) === stable(request.result.worker)
    && exactKeys(value.source, ["revision"]) && value.source.revision === request.result.revision
    && Array.isArray(value.checks) && value.checks.length > 0
    && value.checks.every((check) => exactKeys(check, ["name", "status"]) && nonempty(check.name, 256) && ["passed", "failed"].includes(check.status))
    && value.status === request.result.status;
}

function validReviewEvidence(value, request, assignment, results, sourceBinding) {
  const sourceResults = sourceBinding?.results ?? results;
  const sourceAssignment = sourceBinding?.assignment ?? assignment;
  const verification = sourceResults.find((entry) => entry.kind === "verification" && entry.status === "passed" && entry.revision === request.result.revision);
  const keys = ["schemaVersion", "assignmentId", "taskId", "revision", "reviewer", "source", "diff", "verifierEvidence",
    "rubric", "challenges", "testsExercised", "status"];
  return exactKeys(value, sourceBinding ? [...keys, "sourceAssignment"] : keys)
    && value.schemaVersion === 1 && value.assignmentId === assignment.id && value.taskId === assignment.taskId
    && value.revision === request.result.revision && stable(value.reviewer) === stable(request.result.worker)
    && (!sourceBinding || stable(value.sourceAssignment) === stable(assignment.sourceAssignment))
    && exactKeys(value.source, ["revision", "readFirst"]) && value.source.revision === request.result.revision && value.source.readFirst === true
    && exactKeys(value.diff, ["baseRevision", "sourceRevision"]) && value.diff.baseRevision === sourceAssignment.revision
    && value.diff.sourceRevision === request.result.revision && stable(value.verifierEvidence) === stable(verification?.evidence)
    && Array.isArray(value.rubric) && value.rubric.length > 0 && value.rubric.every((entry) => nonempty(entry, 256))
    && Array.isArray(value.challenges) && value.challenges.length > 0
    && value.challenges.every((entry) => exactKeys(entry, ["scenario", "testEvidence"])
      && nonempty(entry.scenario, 2048) && nonempty(entry.testEvidence, 2048))
    && value.testsExercised === true && value.status === request.result.status;
}

function rotationRequestProblem(request) {
  if (!exactKeys(request, ["schemaVersion", "operationId", "laneId", "expectedLaneFingerprint", "expectedRevision", "trigger",
    "outgoingWorker", "replacementWorker", "handover", "transferEvidence", "reason"])
    || request.schemaVersion !== 1 || ![request.operationId, request.laneId].every((value) => typeof value === "string" && /^[\w.:-]{1,128}$/.test(value))
    || !hex(request.expectedLaneFingerprint, 64) || !hex(request.expectedRevision, 40) || !["threshold", "pressure"].includes(request.trigger)
    || !validWorkerBinding(request.outgoingWorker) || !(request.replacementWorker === null || validWorkerBinding(request.replacementWorker))
    || request.replacementWorker !== null && request.replacementWorker.generation !== request.outgoingWorker.generation + 1
    || !exactKeys(request.handover, ["path", "sha256", "revision", "sequence"]) || !hex(request.handover.sha256, 64)
    || request.handover.revision !== request.expectedRevision || !Number.isSafeInteger(request.handover.sequence) || request.handover.sequence < 1
    || request.handover.path !== `.agent-team/lanes/${request.laneId}/HANDOVER-${String(request.handover.sequence).padStart(3, "0")}.md`
    || !(request.transferEvidence?.kind === "explicit_transfer"
      && exactKeys(request.transferEvidence, ["kind", "path", "sha256"]) && typeof request.transferEvidence.path === "string"
      && hex(request.transferEvidence.sha256, 64)
      || request.transferEvidence?.kind === "stopped_writer" && request.replacementWorker === null
      && exactKeys(request.transferEvidence, ["kind", "observationId"]) && typeof request.transferEvidence.observationId === "string"
      && /^[\w.:-]{1,128}$/.test(request.transferEvidence.observationId)) || !boundedReason(request)) return "invalid_request";
  return undefined;
}

/** Rotate an open lane only from sealed handover and explicit owner-authorized transfer evidence. */
export async function rotateLane(project, request, options = {}) {
  const problem = rotationRequestProblem(request);
  if (problem) return conflict(problem);
  const effectiveRequest = { ...request, actorSessionId: options.actorSessionId, expectedVersion: options.expectedVersion };
  return mutateOperationalState(project, effectiveRequest, async (state, canonical) => {
    if (validateLaneCollection(state.lanes)) return conflict("invalid_lanes");
    const lane = state.lanes.records.find(({ id }) => id === request.laneId);
    if (!lane) return conflict("lane_not_found");
    if (lane.status === "closed") return conflict("lane_closed");
    if (laneFingerprint(lane) !== request.expectedLaneFingerprint) return conflict("stale_lane");
    if (stable(lane.worker) !== stable(request.outgoingWorker)) return conflict("worker_identity_mismatch");
    if (request.handover.sequence !== lane.rotationCount + 1) return conflict("stale_rotation_sequence");
    if (request.trigger === "pressure" && state.run?.executionSettings?.lanes?.rotation?.onPressure !== true) return conflict("rotation_not_due");
    if (request.trigger === "threshold") {
      const limit = state.run?.executionSettings?.lanes?.rotation?.tasks;
      const completed = new Set(lane.assignments.filter((assignment) => assignment.worker.generation === lane.worker.generation
        && lane.results.some((result) => result.assignmentId === assignment.id && result.kind === "integration" && result.status === "passed"))
        .map(({ taskId }) => taskId)).size;
      if (!Number.isSafeInteger(limit) || completed < limit) return conflict("rotation_not_due");
    }
    const team = canonical.registry.teams.find((entry) => entry["team id"] === lane.teamId);
    if (!team || team.branch !== lane.branch || request.replacementWorker !== null && team.session !== request.replacementWorker.sessionId) {
      return conflict("registered_team_required");
    }
    const head = await gitValue(lane.worktree, "rev-parse", "HEAD").catch(() => null);
    if (head !== request.expectedRevision) return conflict("stale_revision");
    let handover;
    try {
      handover = await sealedBytes(path.join(project.root, request.handover.path), 128 * 1024);
    } catch { return conflict("lane_rotation_evidence_unavailable"); }
    if (createHash("sha256").update(handover).digest("hex") !== request.handover.sha256
      || validateLaneHandover(handover.toString("utf8"), { expected: { laneId: lane.id, sequence: request.handover.sequence,
        revision: request.expectedRevision } })) return conflict("lane_handover_invalid");
    if (request.transferEvidence.kind === "explicit_transfer") {
      let authorizationBytes; let authorization;
      try {
        authorizationBytes = await sealedBytes(path.join(project.root, request.transferEvidence.path), 128 * 1024);
        authorization = JSON.parse(authorizationBytes);
      } catch { return conflict("lane_rotation_evidence_unavailable"); }
      const expectedActor = { host: canonical.registry.projectOwnerHost, sessionId: canonical.registry.projectOwner,
        ownershipEpoch: canonical.registry.ownershipEpoch };
      const expectedAuthorization = { schemaVersion: 1, kind: "explicit_transfer", laneId: lane.id,
        outgoingWorker: request.outgoingWorker, replacementWorker: request.replacementWorker, revision: request.expectedRevision,
        handoverSha256: request.handover.sha256, authorizedBy: expectedActor, reason: request.reason };
      if (createHash("sha256").update(authorizationBytes).digest("hex") !== request.transferEvidence.sha256
        || !exactKeys(authorization, [...Object.keys(expectedAuthorization), "observedAt"])
        || stable(Object.fromEntries(Object.keys(expectedAuthorization).map((key) => [key, authorization[key]]))) !== stable(expectedAuthorization)
        || !Number.isFinite(Date.parse(authorization.observedAt ?? ""))) return conflict("lane_transfer_evidence_invalid");
    } else {
      if (typeof options.inspectLaneSession !== "function") return conflict("writer_liveness_unknown");
      let observation;
      try { observation = await options.inspectLaneSession(structuredClone(request.outgoingWorker)); }
      catch { return conflict("writer_liveness_unknown"); }
      if (!exactKeys(observation, ["status", "host", "sessionId", "generation", "observationId", "source", "observedAt"])
        || observation.status !== "stopped" || observation.host !== request.outgoingWorker.host
        || observation.sessionId !== request.outgoingWorker.sessionId || observation.generation !== request.outgoingWorker.generation
        || observation.observationId !== request.transferEvidence.observationId || typeof observation.source !== "string" || !observation.source
        || !Number.isFinite(Date.parse(observation.observedAt ?? ""))) return conflict("writer_liveness_unknown");
    }
    const rotated = { ...lane, worker: request.replacementWorker === null ? null : structuredClone(request.replacementWorker),
      rotationCount: lane.rotationCount + 1, handover: structuredClone(request.handover),
      status: request.replacementWorker === null ? "rotation_required" : lane.currentTaskId === null ? "prepared" : "active" };
    const nextState = replaceLane(state, rotated);
    if (validateLaneCollection(nextState.lanes)) return conflict("invalid_lanes");
    return { state: nextState, result: { laneId: lane.id, outgoingWorker: structuredClone(request.outgoingWorker),
      replacementWorker: structuredClone(request.replacementWorker), handover: structuredClone(request.handover), currentTaskId: lane.currentTaskId } };
  }, options);
}

function closeRequestProblem(request) {
  if (!exactKeys(request, ["schemaVersion", "operationId", "laneId", "expectedLaneFingerprint", "expectedTrackerFingerprint",
    "expectedRunFingerprint", "expectedRevision", "writerRelease", "reason"])
    || request.schemaVersion !== 1 || ![request.operationId, request.laneId].every((value) => typeof value === "string" && /^[\w.:-]{1,128}$/.test(value))
    || ![request.expectedLaneFingerprint, request.expectedTrackerFingerprint, request.expectedRunFingerprint].every((value) => hex(value, 64))
    || !hex(request.expectedRevision, 40)
    || !(request.writerRelease === null || exactKeys(request.writerRelease, ["kind", "path", "sha256"])
      && request.writerRelease.kind === "explicit_release" && typeof request.writerRelease.path === "string" && hex(request.writerRelease.sha256, 64))
    || !boundedReason(request)) return "invalid_request";
  return undefined;
}

/** Close a fully evidenced lane record without deleting its retained worktree or history. */
export async function closeLane(project, request, options = {}) {
  const problem = closeRequestProblem(request);
  if (problem) return conflict(problem);
  const effectiveRequest = { ...request, actorSessionId: options.actorSessionId, expectedVersion: options.expectedVersion };
  return mutateOperationalState(project, effectiveRequest, async (state) => {
    if (validateLaneCollection(state.lanes)) return conflict("invalid_lanes");
    const lane = state.lanes.records.find(({ id }) => id === request.laneId);
    if (!lane) return conflict("lane_not_found");
    if (lane.status === "closed") return conflict("lane_closed");
    if (laneFingerprint(lane) !== request.expectedLaneFingerprint) return conflict("stale_lane");
    if (lane.worker === null && request.writerRelease !== null || lane.worker !== null && request.writerRelease === null) {
      return conflict("lane_writer_not_released");
    }
    if (!state.run?.executionSettings || effectiveRunFingerprint(state.run) !== request.expectedRunFingerprint) return conflict("stale_run");
    const canonical = await loadCanonicalState(project, trackerOptions(options, state));
    if (canonical.tracker.status !== "current") return { status: "unavailable", reason: "tracker_unavailable" };
    if (canonical.tracker.fingerprint !== request.expectedTrackerFingerprint) return conflict("stale_tracker");
    if (canonical.git.headRevision !== request.expectedRevision) return conflict("stale_revision");
    if (request.writerRelease) {
      let bytes; let release;
      try { bytes = await sealedBytes(path.join(project.root, request.writerRelease.path), 128 * 1024); release = JSON.parse(bytes); }
      catch { return conflict("lane_writer_release_unavailable"); }
      const expected = { schemaVersion: 1, kind: "explicit_release", laneId: lane.id, worker: lane.worker,
        revision: request.expectedRevision, authorizedBy: { host: canonical.registry.projectOwnerHost,
          sessionId: canonical.registry.projectOwner, ownershipEpoch: canonical.registry.ownershipEpoch }, reason: request.reason };
      if (createHash("sha256").update(bytes).digest("hex") !== request.writerRelease.sha256
        || !exactKeys(release, [...Object.keys(expected), "observedAt"])
        || stable(Object.fromEntries(Object.keys(expected).map((key) => [key, release[key]]))) !== stable(expected)
        || !Number.isFinite(Date.parse(release.observedAt ?? ""))) return conflict("lane_writer_release_invalid");
    }
    const closed = { ...lane, status: "closed", currentTaskId: null, worker: null };
    const candidate = { ...canonical, state: replaceLane(state, closed) };
    const blocker = laneCleanupBlocker(candidate, { taskId: lane.queue[0], worktree: lane.worktree });
    if (blocker) return conflict(blocker);
    return { state: candidate.state, result: { laneId: lane.id, closed: true, retainedWorktree: lane.worktree,
      retainedAssignments: lane.assignments.length, retainedResults: lane.results.length } };
  }, options);
}

async function recordLaneResult(project, request, options) {
  const problem = resultRequestProblem(request);
  if (problem) return conflict(problem);
  const effectiveRequest = { ...request, actorSessionId: options.actorSessionId, expectedVersion: options.expectedVersion };
  return mutateOperationalState(project, effectiveRequest, async (state, canonical) => {
    if (validateLaneCollection(state.lanes)) return conflict("invalid_lanes");
    const lane = state.lanes.records.find(({ id }) => id === request.laneId);
    if (!lane) return conflict("lane_not_found");
    if (laneFingerprint(lane) !== request.expectedLaneFingerprint) return conflict("stale_lane");
    const assignment = lane.assignments.find(({ id }) => id === request.assignmentId);
    if (!assignment || assignment.taskId !== lane.currentTaskId) return conflict("assignment_identity_mismatch");
    if (request.result.kind === "independent_review" && !["reviewer", "visual_reviewer"].includes(lane.role)) {
      return conflict("review_lane_required");
    }
    const reviewLane = reviewerRoles.has(lane.role);
    if (reviewLane && !["independent_review", "unresolved"].includes(request.result.kind)) return conflict("review_lane_result_invalid");
    if (reviewLane) {
      const team = canonical.registry.teams.find((entry) => entry["team id"] === lane.teamId);
      if (!team || team.session !== lane.worker?.sessionId || team.branch !== lane.branch) return conflict("registered_team_required");
    }
    if (assignment.status === "prepared") return conflict("result_before_dispatch");
    if (assignment.packet.sha256 !== request.expectedPacketSha256) return conflict("assignment_binding_mismatch");
    if (lane.results.some((entry) => entry.assignmentId === assignment.id && entry.kind === request.result.kind)) return conflict("result_already_recorded");
    const results = lane.results.filter((entry) => entry.assignmentId === assignment.id);
    let sourceBinding;
    if (reviewLane && request.result.kind === "independent_review") {
      const sourceLane = state.lanes.records.find(({ id }) => id === assignment.sourceAssignment.laneId);
      const sourceAssignment = sourceLane?.assignments.find(({ id }) => id === assignment.sourceAssignment.assignmentId);
      const sourceResults = sourceLane?.results.filter(({ assignmentId }) => assignmentId === sourceAssignment?.id) ?? [];
      if (!sourceLane || !sourceAssignment || sourceAssignment.packet.sha256 !== assignment.sourceAssignment.packetSha256
        || !sourceResults.some(({ kind, status, revision }) => kind === "worker" && status === "passed" && revision === request.result.revision)
        || !sourceResults.some(({ kind, status, revision }) => kind === "verification" && status === "passed" && revision === request.result.revision)
        || sourceResults.some(({ kind }) => kind === "independent_review")) return conflict("review_source_assignment_unready");
      if (assignment.status !== "dispatched" || assignment.dispatch?.status !== "observed"
        || stable(request.result.worker) !== stable(assignment.worker) || stable(lane.worker) !== stable(assignment.worker)) {
        return conflict("review_dispatch_unobserved");
      }
      sourceBinding = { lane: sourceLane, assignment: sourceAssignment, results: sourceResults };
    } else if (["worker", "unresolved"].includes(request.result.kind)) {
      if (assignment.status !== "dispatched" || results.some((entry) => ["worker", "unresolved"].includes(entry.kind))) return conflict("result_out_of_order");
      if (stable(request.result.worker) !== stable(assignment.worker)) return conflict("worker_identity_mismatch");
      if (request.result.kind === "worker" && assignment.dispatch?.status !== "observed") return conflict("dispatch_unobserved");
    } else {
      const workerResult = results.find((entry) => entry.kind === "worker" && entry.status === "passed");
      if (assignment.status !== "resolved" || !workerResult || request.result.revision !== workerResult.revision) return conflict("result_out_of_order");
      if (request.result.kind === "verification" && stable(request.result.worker) === stable(assignment.worker)) return conflict("verification_not_independent");
      if (request.result.kind === "independent_review" && (!priorPassed(results, "verification", request.result.revision)
        || stable(request.result.worker) === stable(assignment.worker))) return conflict("review_not_independent");
      if (request.result.kind === "integration") {
        const delivery = await loadCanonicalState(project, trackerOptions(options, state));
        if (!priorPassed(results, "independent_review", request.result.revision)
          || delivery.deliveryEvidence?.[assignment.taskId]?.sourceRevision !== request.result.revision) return conflict("integration_evidence_mismatch");
      }
    }
    const prefix = `.agent-team/lanes/${lane.id}/evidence/`;
    if (!request.result.evidence.path.startsWith(prefix) || path.posix.normalize(request.result.evidence.path) !== request.result.evidence.path
      || request.result.evidence.path.split("/").includes("..")) return conflict("invalid_request");
    let evidence;
    try { evidence = await sealedBytes(path.join(project.root, request.result.evidence.path)); }
    catch { return conflict("lane_result_evidence_unavailable"); }
    if (createHash("sha256").update(evidence).digest("hex") !== request.result.evidence.sha256) return conflict("lane_result_evidence_changed");
    if (["verification", "independent_review"].includes(request.result.kind)) {
      let parsed;
      try { parsed = JSON.parse(evidence); } catch { return conflict(`${request.result.kind}_evidence_invalid`); }
      if (request.result.kind === "verification" ? !validVerificationEvidence(parsed, request, assignment)
        : !validReviewEvidence(parsed, request, assignment, results, sourceBinding)) return conflict(`${request.result.kind}_evidence_invalid`);
    }
    if (["worker", "unresolved"].includes(request.result.kind)) {
      const head = await gitValue(lane.worktree, "rev-parse", "HEAD").catch(() => null);
      if (head !== request.result.revision) return conflict("result_revision_mismatch");
    }
    const result = { assignmentId: assignment.id, taskId: assignment.taskId, ...structuredClone(request.result),
      recordedAt: (options.now ?? (() => new Date().toISOString()))() };
    const changed = ["worker", "independent_review"].includes(result.kind) ? { ...assignment, status: "resolved" } : assignment;
    const nextResults = [...lane.results, result];
    const rotationLimit = state.run?.executionSettings?.lanes?.rotation?.tasks;
    const completedByCurrentWorker = result.kind === "integration" && result.status === "passed" && lane.worker
      ? new Set(lane.assignments.filter((entry) => entry.worker.generation === lane.worker.generation
        && nextResults.some((outcome) => outcome.assignmentId === entry.id && outcome.kind === "integration" && outcome.status === "passed"))
        .map(({ taskId }) => taskId)).size : 0;
    const nextLane = { ...lane, assignments: lane.assignments.map((entry) => entry.id === assignment.id ? changed : entry),
      results: nextResults, ...(reviewLane && result.kind === "independent_review" ? { currentTaskId: null, status: "prepared" } : {}),
      ...(Number.isSafeInteger(rotationLimit) && completedByCurrentWorker >= rotationLimit
        ? { status: "rotation_required" } : {}) };
    let nextState = replaceLane(state, nextLane);
    if (sourceBinding && result.kind === "independent_review" && result.status === "passed") {
      const linked = { ...result, assignmentId: sourceBinding.assignment.id };
      const changedSource = { ...sourceBinding.lane, results: [...sourceBinding.lane.results, linked] };
      nextState = replaceLane(nextState, changedSource);
    }
    if (validateLaneCollection(nextState.lanes)) return conflict("invalid_lanes");
    return { state: nextState, result: { laneId: lane.id, assignmentId: assignment.id, result: structuredClone(result), authorityGranted: false } };
  }, options);
}

/** Advance one lifecycle phase without conflating a prepared packet with an observed dispatch. */
export async function advanceLane(project, request, options = {}) {
  if (request?.phase === "prepare") return prepareLane(project, request, options);
  if (request?.phase === "bind") return bindLaneWorker(project, request, options);
  if (request?.phase === "dispatch") return recordLaneDispatch(project, request, options);
  if (request?.phase === "result") return recordLaneResult(project, request, options);
  return conflict("invalid_request");
}
