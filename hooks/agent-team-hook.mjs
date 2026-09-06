#!/usr/bin/env node
import process from "node:process";
import os from "node:os";
import path from "node:path";
import { normalizeEvent } from "./lib/event.mjs";
import { identityFor, loadCanonicalState, syncOperationMappingInventory } from "./lib/canonical-state.mjs";
import { resolveProject } from "./lib/project.mjs";
import { inspectRecovery } from "./lib/recovery.mjs";
import { writeCheckpoint } from "./lib/checkpoint.mjs";
import { adaptOutput, adaptTransport } from "./lib/output.mjs";
import { evaluatePolicy, unavailableDecision } from "./lib/policy.mjs";
import { activationRecordFor, appendActivationLog } from "./lib/telemetry.mjs";

function argument(name) {
  const index = process.argv.indexOf(`--${name}`);
  return index === -1 ? undefined : process.argv[index + 1];
}

async function stdin() {
  const chunks = [];
  let size = 0;
  for await (const chunk of process.stdin) {
    size += chunk.length;
    if (size > 1024 * 1024) throw new Error("Hook input exceeds 1 MiB.");
    chunks.push(chunk);
  }
  return JSON.parse(Buffer.concat(chunks).toString("utf8") || "{}");
}

async function checkpointFacts(event, project) {
  const snapshot = await inspectRecovery(project, { includeProbes: true, sessionId: event.sessionId });
  return {
    eventId: event.eventId || `${event.event}:${event.sessionId}`,
    sessionId: event.sessionId,
    eventKind: event.event,
    projectId: project.projectId,
    worktree: project.worktreeRoot,
    trackerPath: project.paths.tasks,
    mistakesPath: path.join(project.root, "MISTAKES.md"),
    ...(snapshot.git?.branch?.status === "current" ? { branch: snapshot.git.branch.value } : {}),
    ...(snapshot.git?.revision?.status === "current" ? { revision: snapshot.git.revision.value } : {}),
    ...(snapshot.identity?.teamId ? { teamId: snapshot.identity.teamId } : {}),
    taskIds: snapshot.identity?.taskIds ?? [],
    evidence: {
      git: snapshot.probes?.git === "available" ? "current" : "unavailable",
      tracker: snapshot.tracker?.status ?? "unavailable",
      recovery: snapshot.status,
    },
    pendingOperations: Object.entries(snapshot.operations ?? {})
      .filter(([, value]) => Object.keys(value).length)
      .map(([kind, value]) => ({ kind, ...value })),
  };
}

function changedOperationalMappings(event, project) {
  if (event.event === "SessionStart") return true;
  if (!["PostToolUse", "PostToolBatch"].includes(event.event) || event.operation.kind !== "file_change") return false;
  return event.operation.files.some((file) => path.resolve(event.cwd, file.path) === project.paths.state);
}

/** Run one normalized event through shared policy and bounded factual mutations. */
export async function runNormalizedHook(event) {
  const project = await resolveProject(event.cwd);
  const decision = await evaluatePolicy(event, project);
  decision.context.active = project.active;
  decision.context.projectId = project.projectId;

  if (project.active && decision.allow && changedOperationalMappings(event, project)) {
    try {
      const result = await syncOperationMappingInventory(project, event);
      decision.mutations.push({ kind: "operation_mapping_cache", changed: result.changed });
      decision.messages.push(`Agent-Team mapping cache is ${result.changed ? "updated" : "current"}.`);
    } catch {
      decision.messages.push("Agent-Team mapping cache refresh is unavailable. Only the canonical project owner can refresh healthy state mappings.");
      decision.capabilities.operationMappings = "unavailable";
    }
  }

  let activation;
  if (project.active) {
    try {
      const canonical = await loadCanonicalState(project);
      activation = activationRecordFor(event, project, identityFor(canonical.registry, event.sessionId));
    } catch {
      activation = activationRecordFor(event, project, { role: "unknown" });
    }
  }
  if (activation) {
    try {
      const result = await appendActivationLog(path.join(os.homedir(), ".agent-team-hooks", "logs"), activation);
      decision.mutations.push({ kind: "activation_log", recorded: result.recorded });
    } catch {
      decision.messages.push("Agent-Team activation logging is unavailable for this event.");
      decision.capabilities.activationLogging = "unavailable";
    }
  }

  if (project.active && ["SessionStart", "UserPromptSubmit"].includes(event.event)) {
    const recovery = await inspectRecovery(project, { includeProbes: true, sessionId: event.sessionId });
    decision.context.recovery = recovery;
    decision.messages.push(`Agent-Team recovery evidence: ${recovery.status}.`);
  }
  const checkpointEvent = ["PreCompact", "PostCompact", "Interrupt", "SessionEnd"].includes(event.event)
    || (event.runtime === "claude" && event.event === "PostToolBatch" && event.operation.kind === "file_change");
  if (project.active && checkpointEvent) {
    try {
      const result = await writeCheckpoint(project, await checkpointFacts(event, project));
      decision.mutations.push({ kind: "checkpoint", created: result.created });
    } catch {
      decision.messages.push("Agent-Team checkpoint is unavailable for this event.");
      decision.capabilities.checkpoint = "unavailable";
    }
  }
  return { decision, output: adaptOutput(event.runtime, event.event, decision) };
}

/** Normalize one native payload before running the shared entrypoint. */
export async function runHook(runtime, eventName, payload) {
  return runNormalizedHook(normalizeEvent(runtime, eventName, payload));
}

if (import.meta.url === `file://${process.argv[1]}`) {
  let event;
  try {
    const runtime = argument("runtime");
    const eventName = argument("event");
    const payload = await stdin();
    event = normalizeEvent(runtime, eventName, payload);
    const { decision } = await runNormalizedHook(event);
    const transport = adaptTransport(event.runtime, event.event, decision);
    if (transport.stdout) process.stdout.write(transport.stdout);
    if (transport.stderr) process.stderr.write(transport.stderr);
    process.exitCode = transport.exitCode;
  } catch (error) {
    if (!event) {
      process.stderr.write(`Agent-Team hook unavailable: ${error.message}\n`);
      process.exitCode = 1;
    } else {
      const decision = unavailableDecision(event);
      decision.messages.push(`Agent-Team hook unavailable: ${error.message}`);
      const transport = adaptTransport(event.runtime, event.event, decision);
      if (transport.stdout) process.stdout.write(transport.stdout);
      if (transport.stderr) process.stderr.write(transport.stderr);
      process.exitCode = transport.exitCode;
    }
  }
}
