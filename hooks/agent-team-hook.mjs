#!/usr/bin/env node
import process from "node:process";
import os from "node:os";
import path from "node:path";
import { fileURLToPath, pathToFileURL } from 'node:url';
import { randomUUID } from 'node:crypto';
import { normalizeEvent } from "./lib/event.mjs";
import { identityFor, syncOperationMappingInventory } from "./lib/canonical-state.mjs";
import { resolveProject } from "./lib/project.mjs";
import { inspectRecovery } from "./lib/recovery.mjs";
import { writeCheckpoint } from "./lib/checkpoint.mjs";
import { adaptOutput, adaptTransport } from "./lib/output.mjs";
import { evaluatePolicy, unavailableDecision } from "./lib/policy.mjs";
import { activationRecordFor, appendActivationLog } from "./lib/telemetry.mjs";
import { createEventBudget } from "./lib/budget.mjs";
import { lintMessages } from "./lib/lint.mjs";
import { recordHookEvidence, resolveHookEvidenceRoot } from './lib/health.mjs';

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

async function checkpointFacts(event, project, options) {
  const snapshot = await inspectRecovery(project, { ...options, includeProbes: false, includeGit: true, sessionId: event.sessionId });
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

function shouldRefreshOperationMappings(event, project) {
  if (event.event === "SessionStart") return true;
  if (!["PostToolUse", "PostToolBatch"].includes(event.event) || event.operation.kind !== "file_change") return false;
  return event.operation.files.some((file) => path.resolve(event.cwd, file.path) === project.paths.state);
}

/** Run one normalized event through shared policy and bounded factual mutations. */
export async function runNormalizedHook(event, { timeoutMs = 5000, runBeads, evidencePackageRoot } = {}) {
  const budget = createEventBudget(timeoutMs);
  try {
    const { evidenceRoot, ...result } = await runEvent(event, budget, runBeads, evidencePackageRoot);
    if (evidenceRoot) {
      try {
        const evidence = await budget.run(() => recordHookEvidence(evidenceRoot, {
          runtime: event.runtime, event: event.event, status: 'passed', eventId: event.eventId || randomUUID(),
          sessionId: event.sessionId, source: 'packaged_entrypoint',
        }, { budget }));
        result.decision.mutations.push({ kind: 'hook_transport_evidence', status: evidence.status });
      } catch {
        result.decision.capabilities.hookTransportEvidence = 'unavailable';
        result.decision.messages.push('Agent-Team hook transport evidence is unavailable; the policy decision is unchanged.');
      }
      result.output = adaptOutput(event.runtime, event.event, result.decision);
    }
    return result;
  } catch (error) {
    const decision = unavailableDecision(event);
    decision.messages.push(error.code === "EVENT_DEADLINE" ? "Agent-Team event deadline exceeded; required evidence remains unavailable." : `Agent-Team hook unavailable: ${error.message}`);
    return { decision, output: adaptOutput(event.runtime, event.event, decision) };
  } finally {
    budget.close();
  }
}

async function runEvent(event, budget, runBeads, evidencePackageRoot) {
  const project = await budget.run(() => resolveProject(event.cwd, { budget }));
  const progress = {};
  let decision;
  try {
    decision = await budget.run(() => evaluatePolicy(event, project, { budget, runBeads, progress }));
  } catch {
    decision = unavailableDecision({ ...event, operation: progress.operation ?? event.operation });
    if (progress.lint) {
      decision.capabilities.lint = structuredClone(progress.lint);
      decision.messages.push(...lintMessages(decision.capabilities.lint, project.worktreeRoot));
    }
  }
  const canonical = progress.canonical;
  decision.context.active = project.active;
  decision.context.projectId = project.projectId;

  if (project.active && decision.allow && shouldRefreshOperationMappings(event, project)) {
    try {
      const result = await budget.run(() => syncOperationMappingInventory(project, { canonical, budget }));
      decision.mutations.push({ kind: "operation_mapping_cache", changed: result.changed });
      decision.messages.push(`Agent-Team mapping cache is ${result.changed ? "updated" : "current"}.`);
    } catch {
      decision.messages.push("Agent-Team mapping cache refresh is unavailable; the previous cache, if any, is unchanged.");
      decision.capabilities.operationMappingRefresh = "unavailable";
    }
  }

  let identity = { role: "unregistered" };
  if (project.active) {
    try {
      identity = canonical ? identityFor(canonical.registry, event.runtime, event.sessionId) : { role: "unknown" };
    } catch {
      identity = { role: "unknown" };
    }
  }
  // Optional installation discovery follows required policy evaluation. Its
  // deadline must never replace a resolved mapped-operation denial.
  let evidenceRoot = null;
  let evidenceDiscoveryUnavailable = false;
  if (evidencePackageRoot) {
    try { evidenceRoot = await budget.run(() => resolveHookEvidenceRoot(evidencePackageRoot, event.runtime)); }
    catch {
      evidenceDiscoveryUnavailable = true;
      decision.capabilities.hookTransportEvidence = 'unavailable';
      decision.messages.push('Agent-Team hook installation evidence is unavailable; the policy decision is unchanged.');
    }
  }
  const activation = activationRecordFor(event, project, identity);
  if (activation && !evidenceDiscoveryUnavailable) {
    try {
      const result = await budget.run(() => appendActivationLog(path.join(evidenceRoot ?? os.homedir(), ".agent-team-hooks", "logs"), activation, { budget }));
      decision.mutations.push({ kind: "activation_log", recorded: result.recorded });
    } catch {
      decision.messages.push("Agent-Team activation logging is unavailable for this event.");
      decision.capabilities.activationLogging = "unavailable";
    }
  }

  if (project.active && ["SessionStart", "UserPromptSubmit"].includes(event.event)) {
    try {
      const recovery = await budget.run(() => inspectRecovery(project, { includeProbes: false, includeGit: true, sessionId: event.sessionId, canonical, budget }));
      decision.context.recovery = recovery;
      decision.messages.push(`Agent-Team recovery evidence: ${recovery.status}.`);
    } catch {
      decision.messages.push("Agent-Team recovery evidence is unavailable within the event deadline.");
      decision.capabilities.recovery = "unavailable";
    }
  }
  const checkpointEvent = ["PreCompact", "PostCompact", "Interrupt", "SessionEnd"].includes(event.event)
    || (event.runtime === "claude" && event.event === "PostToolBatch" && event.operation.kind === "file_change");
  if (project.active && checkpointEvent) {
    try {
      const facts = await budget.run(() => checkpointFacts(event, project, { canonical, budget }));
      const result = await budget.run(() => writeCheckpoint(project, facts, { budget }));
      decision.mutations.push({ kind: "checkpoint", created: result.created, status: result.status });
      if (result.status === 'conflict' || result.status === 'unavailable') {
        decision.messages.push(`Agent-Team checkpoint ${result.status}: ${result.reason ?? 'evidence not persisted'}. Existing evidence is unchanged.`);
        decision.capabilities.checkpoint = result.status;
      }
    } catch {
      decision.messages.push("Agent-Team checkpoint is unavailable for this event.");
      decision.capabilities.checkpoint = "unavailable";
    }
  }
  const changedCanonicalRecord = ['PostToolUse', 'PostToolBatch'].includes(event.event)
    && event.operation.kind === 'file_change'
    && event.operation.files.some((file) => [project.paths?.tasks, project.paths?.state, project.paths?.teams]
      .includes(path.resolve(event.cwd, file.path)));
  if (project.active && project.setup.dashboard?.snapshot === true
    && (changedCanonicalRecord || decision.mutations.some((entry) => entry.kind === 'checkpoint' && entry.created))) {
    try {
      const { refreshConfiguredDashboard } = await budget.run(() => import('./lib/workflow-cli.mjs'));
      const result = await budget.run(() => refreshConfiguredDashboard(project, { budget }));
      decision.mutations.push({ kind: 'dashboard_snapshot', status: result.status });
    } catch {
      decision.capabilities.dashboardSnapshot = 'unavailable';
      decision.messages.push('Agent-Team dashboard refresh is unavailable; task authority and policy decision are unchanged.');
    }
  }
  return { decision, output: adaptOutput(event.runtime, event.event, decision), evidenceRoot };
}

/** Normalize one native payload before running the shared entrypoint. */
export async function runHook(runtime, eventName, payload) {
  return runNormalizedHook(normalizeEvent(runtime, eventName, payload));
}

if (process.argv[1] && import.meta.url === pathToFileURL(path.resolve(process.argv[1])).href) {
  let event;
  try {
    const runtime = argument("runtime");
    const eventName = argument("event");
    const payload = await stdin();
    event = normalizeEvent(runtime, eventName, payload);
    const { decision } = await runNormalizedHook(event, { evidencePackageRoot: path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..') });
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
