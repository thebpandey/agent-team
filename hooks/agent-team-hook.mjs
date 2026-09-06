#!/usr/bin/env node
import process from "node:process";
import os from "node:os";
import path from "node:path";
import { normalizeEvent } from "./lib/event.mjs";
import { resolveProject } from "./lib/project.mjs";
import { inspectRecovery } from "./lib/recovery.mjs";
import { writeCheckpoint } from "./lib/checkpoint.mjs";
import { adaptOutput } from "./lib/output.mjs";
import { evaluatePolicy } from "./lib/policy.mjs";
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

/** Run the shared hook policy for one host event and emit only native JSON. */
export async function runHook(runtime, eventName, payload) {
  const event = normalizeEvent(runtime, eventName, payload);
  const project = await resolveProject(event.cwd);
  const decision = await evaluatePolicy(event, project);
  decision.context.active = project.active;
  decision.context.projectId = project.projectId;

  const activation = activationRecordFor(event, project);
  if (activation) {
    try {
      const result = await appendActivationLog(path.join(os.homedir(), ".agent-team-hooks", "logs"), activation);
      decision.mutations.push({ kind: "activation_log", recorded: result.recorded });
    } catch {
      decision.messages.push("Agent-Team activation logging is unavailable for this event.");
      decision.capabilities.activationLogging = "unavailable";
    }
  }

  if (project.active && ["SessionStart", "UserPromptSubmit"].includes(eventName)) {
    const recovery = await inspectRecovery(project, { includeProbes: true });
    decision.context.recovery = recovery;
    decision.messages.push(`Agent-Team recovery evidence: ${recovery.status}.`);
  }
  if (project.active && ["PreCompact", "PostCompact", "Interrupt", "SessionEnd"].includes(eventName)) {
    const result = await writeCheckpoint(project, {
      eventId: event.eventId || `${eventName}:${event.sessionId}`,
      sessionId: event.sessionId,
      eventKind: eventName,
      projectId: project.projectId,
      worktree: project.worktreeRoot,
      trackerPath: project.paths.tasks,
    });
    decision.mutations.push({ kind: "checkpoint", created: result.created });
  }
  return { decision, output: adaptOutput(runtime, eventName, decision) };
}

if (import.meta.url === `file://${process.argv[1]}`) {
  try {
    const runtime = argument("runtime");
    const eventName = argument("event");
    const payload = await stdin();
    const { output } = await runHook(runtime, eventName, payload);
    process.stdout.write(`${JSON.stringify(output)}\n`);
  } catch (error) {
    process.stderr.write(`Agent-Team hook unavailable: ${error.message}\n`);
  }
}
