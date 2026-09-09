import { constants } from "node:fs";
import { open } from "node:fs/promises";
import path from "node:path";
import { fileURLToPath } from "node:url";

import { loadCanonicalState } from "./canonical-state.mjs";
import { writeCheckpoint } from "./checkpoint.mjs";
import { cleanupDevelopmentWorktree } from "./cleanup.mjs";
import { createLoopbackDashboard, createSnapshotPublisher } from "./dashboard.mjs";
import { resolveProject } from "./project.mjs";
import { inspectRecovery } from "./recovery.mjs";
import { readStatus } from "./status.mjs";
import { recordGateEvidence, taskEligibility, transitionTask } from "./task-transitions.mjs";
import { readUsageReport } from "./usage.mjs";

const requestLimit = 256 * 1024;
const actorPattern = /^[\w.:-]{1,128}$/;
const assetsDirectory = fileURLToPath(new URL("../../assets/dashboard/", import.meta.url));

export const workflowCommandFlags = Object.freeze({
  status: new Set(["project"]),
  usage: new Set(["project", "receipt", "soft-tokens", "hard-tokens"]),
  recovery: new Set(["project", "session", "task", "worktree", "stale-ms", "include-probes", "include-git"]),
  eligibility: new Set(["project"]),
  checkpoint: new Set(["project", "request"]),
  "task-transition": new Set(["project", "request"]),
  "gate-evidence": new Set(["project", "request"]),
  cleanup: new Set(["project", "request"]),
  "dashboard-snapshot": new Set(["project"]),
  "dashboard-start": new Set(["project", "port"]),
});

function record(value, label) {
  if (!value || typeof value !== "object" || Array.isArray(value)) throw new Error(`${label} must be an object.`);
  return value;
}

function requiredString(value, label) {
  if (typeof value !== "string" || !value) throw new Error(`${label} is required.`);
  return value;
}

function string(value, label) {
  if (typeof value !== "string") throw new Error(`${label} must be a string.`);
  return value;
}

function integer(value, label, { minimum = 0, maximum = Number.MAX_SAFE_INTEGER } = {}) {
  const parsed = typeof value === "number" ? value : Number(value);
  if (!Number.isSafeInteger(parsed) || parsed < minimum || parsed > maximum) throw new Error(`${label} must be an integer from ${minimum} to ${maximum}.`);
  return parsed;
}

function boolean(value, label) {
  if (value === undefined) return false;
  if (value === "true" || value === true) return true;
  if (value === "false" || value === false) return false;
  throw new Error(`${label} must be true or false.`);
}

function writer(value, label) {
  const identity = record(value, label);
  integer(identity.pid, `${label}.pid`, { minimum: 1 });
  for (const field of ["startTime", "bootId", "host"]) requiredString(identity[field], `${label}.${field}`);
  return identity;
}

/** Read one request envelope without following a FIFO or accepting an unbounded file. */
export async function readRequestEnvelope(file) {
  requiredString(file, "--request");
  let handle;
  try {
    handle = await open(path.resolve(file), constants.O_RDONLY | constants.O_NONBLOCK);
    const metadata = await handle.stat();
    if (!metadata.isFile() || metadata.size > requestLimit) throw new Error("Request must be a bounded regular JSON file.");
    const buffer = Buffer.alloc(requestLimit + 1);
    let length = 0;
    while (length < buffer.length) {
      const { bytesRead } = await handle.read(buffer, length, buffer.length - length, null);
      if (!bytesRead) break;
      length += bytesRead;
    }
    if (length > requestLimit) throw new Error("Request must be a bounded regular JSON file.");
    const envelope = record(JSON.parse(buffer.subarray(0, length).toString("utf8")), "Request envelope");
    if (envelope.schemaVersion !== 1) throw new Error("Request schemaVersion must be 1.");
    if (!actorPattern.test(envelope.actorSessionId ?? "")) throw new Error("Request actorSessionId is required and invalid.");
    integer(envelope.expectedVersion, "Request expectedVersion");
    record(envelope.request, "Request body");
    return envelope;
  } catch (error) {
    if (error instanceof SyntaxError) throw new Error("Request must contain valid JSON.");
    throw error;
  } finally {
    await handle?.close();
  }
}

async function activeProject(projectPath) {
  requiredString(projectPath, "--project");
  const project = await resolveProject(path.resolve(projectPath));
  if (!project.active) throw new Error(`Active Agent-Team project required: ${project.reason}.`);
  return project;
}

function validateMutation(command, envelope, project) {
  const request = { ...envelope.request, actorSessionId: envelope.actorSessionId, expectedVersion: envelope.expectedVersion };
  if (command === "checkpoint") {
    requiredString(request.eventId, "checkpoint request.eventId");
    requiredString(request.sessionId, "checkpoint request.sessionId");
  } else {
    requiredString(request.operationId, `${command} request.operationId`);
    if (!actorPattern.test(request.operationId)) throw new Error(`${command} request.operationId is invalid.`);
  }
  if (command === "task-transition") {
    requiredString(request.taskId, "task-transition request.taskId");
    requiredString(request.expectedFingerprint, "task-transition request.expectedFingerprint");
    string(request.expectedOwner, "task-transition request.expectedOwner");
    if (!new Set(["claim", "pause", "park", "resume"]).has(request.action)) throw new Error("task-transition request.action is invalid.");
    if (request.action === "claim") requiredString(request.owner, "task-transition request.owner");
    if (["park", "resume"].includes(request.action)) writer(request.writer, "task-transition request.writer");
    if (request.action === "park") {
      const repair = record(request.repair, "task-transition request.repair");
      integer(repair.attempts, "task-transition request.repair.attempts");
      integer(repair.maxAttempts, "task-transition request.repair.maxAttempts", { minimum: 1 });
      requiredString(request.resumeWhen, "task-transition request.resumeWhen");
      requiredString(request.checkpointPath, "task-transition request.checkpointPath");
    }
    for (const field of ["checkpointPath", "prerequisiteEvidence"]) if (request[field]) request[field] = path.resolve(project.root, request[field]);
  }
  if (command === "gate-evidence") {
    if (!new Set(["completion", "integration", "release"]).has(request.gate)) throw new Error("gate-evidence request.gate is invalid.");
    if (!Array.isArray(request.taskIds) || !request.taskIds.length || request.taskIds.some((id) => typeof id !== "string" || !id)) throw new Error("gate-evidence request.taskIds is required.");
    requiredString(request.expectedFingerprint, "gate-evidence request.expectedFingerprint");
    requiredString(request.expectedRevision, "gate-evidence request.expectedRevision");
    request.evidencePath = path.resolve(project.root, requiredString(request.evidencePath, "gate-evidence request.evidencePath"));
  }
  if (command === "cleanup") {
    requiredString(request.taskId, "cleanup request.taskId");
    requiredString(request.expectedRevision, "cleanup request.expectedRevision");
    writer(request.expectedWriter, "cleanup request.expectedWriter");
    request.worktree = path.resolve(project.root, requiredString(request.worktree, "cleanup request.worktree"));
  }
  if (request.worktree && command === "checkpoint") request.worktree = path.resolve(project.root, request.worktree);
  return request;
}

function snapshotDestination(project) {
  return path.join(project.root, ".agent-team", "dashboard", "index.html");
}

async function publishDashboard(project, { budget } = {}) {
  return createSnapshotPublisher({
    destination: snapshotDestination(project),
    derive: () => readStatus(project, { budget }),
  }).refresh();
}

/** Refresh only when the shared setup explicitly opts into automatic snapshots. */
export async function refreshConfiguredDashboard(project, options = {}) {
  if (project?.setup?.dashboard?.snapshot !== true) return { status: "skipped", reason: "not_configured" };
  return publishDashboard(project, options);
}

async function runDashboard(project, options, context) {
  const port = integer(options.port ?? 0, "--port", { maximum: 65535 });
  const dashboard = createLoopbackDashboard({
    assetsDirectory,
    port,
    readModel: ({ signal, deadlineMs }) => readStatus(project, { signal, deadlineMs }),
  });
  let stop;
  const stopped = new Promise((resolve) => {
    stop = () => resolve();
    process.once("SIGINT", stop);
    process.once("SIGTERM", stop);
  });
  let result;
  try {
    const listening = await dashboard.start();
    result = { status: "listening", url: `http://127.0.0.1:${listening.port}/`, port: listening.port };
    context.onStarted?.(result);
    await stopped;
  } finally {
    process.removeListener("SIGINT", stop);
    process.removeListener("SIGTERM", stop);
    if (result) await dashboard.stop();
  }
  return { ...result, status: "stopped" };
}

/** Route agent-facing workflow commands through the accepted state APIs. */
export async function runWorkflowCommand(command, options, context = {}) {
  const project = await activeProject(options.project);
  if (command === "status") return readStatus(project);
  if (command === "usage") return readUsageReport(project, {
    ...(options.receipt ? { receiptPath: path.resolve(options.receipt) } : {}),
    budgetPolicy: {
      ...(options["soft-tokens"] !== undefined ? { softTokens: integer(options["soft-tokens"], "--soft-tokens", { minimum: 1 }) } : {}),
      ...(options["hard-tokens"] !== undefined ? { hardTokens: integer(options["hard-tokens"], "--hard-tokens", { minimum: 1 }) } : {}),
    },
  });
  if (command === "recovery") return inspectRecovery(project, {
    ...(options.session ? { sessionId: options.session } : {}),
    ...(options.task ? { taskId: options.task } : {}),
    ...(options.worktree ? { worktree: path.resolve(project.root, options.worktree) } : {}),
    ...(options["stale-ms"] !== undefined ? { staleAfterMs: integer(options["stale-ms"], "--stale-ms") } : {}),
    includeProbes: boolean(options["include-probes"], "--include-probes"),
    includeGit: boolean(options["include-git"], "--include-git"),
  });
  if (command === "eligibility") {
    const canonical = await loadCanonicalState(project);
    return { status: "completed", ...taskEligibility(canonical, { scopeTaskIds: canonical.state.run?.taskIds, capacity: canonical.state.capacity }) };
  }
  if (["checkpoint", "task-transition", "gate-evidence", "cleanup"].includes(command)) {
    const envelope = await readRequestEnvelope(options.request);
    const request = validateMutation(command, envelope, project);
    if (command === "checkpoint") return writeCheckpoint(project, request, { actorSessionId: envelope.actorSessionId, expectedVersion: envelope.expectedVersion });
    if (command === "task-transition") return transitionTask(project, request);
    if (command === "gate-evidence") return recordGateEvidence(project, request);
    return cleanupDevelopmentWorktree(project, request);
  }
  if (command === "dashboard-snapshot") return publishDashboard(project);
  if (command === "dashboard-start") return runDashboard(project, options, context);
  throw new Error(`Unknown workflow command: ${command ?? "missing"}`);
}
