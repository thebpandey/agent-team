import { execFile } from "node:child_process";
import { realpath } from "node:fs/promises";
import path from "node:path";
import { promisify } from "node:util";
import { analyzeChangedFiles } from "./analyzers.mjs";
import { identityFor, loadCanonicalState } from "./canonical-state.mjs";
import { runLintChecks } from "./lint.mjs";
import { classifyOperation } from "./operation.mjs";
import { resolveProject } from "./project.mjs";

const run = promisify(execFile);

function decision({ allow = true, mode = "advisory", messages = [], context = {}, capabilities = {}, mutations = [] } = {}) {
  return { mode, allow, messages, context, capabilities, mutations };
}

function deny(message, context = {}) {
  return decision({ allow: false, mode: "enforce", messages: [message], context });
}

function fresh(value, now, maximumAgeMs = 5 * 60_000) {
  const timestamp = Date.parse(value);
  return Number.isFinite(timestamp) && now.getTime() - timestamp >= 0 && now.getTime() - timestamp <= maximumAgeMs;
}

async function canonicalTarget(base, name) {
  const target = path.isAbsolute(name) ? name : path.resolve(base, name);
  const missing = [];
  let candidate = target;
  while (true) {
    try {
      return path.join(await realpath(candidate), ...missing.reverse());
    } catch (error) {
      if (error.code !== "ENOENT" || path.dirname(candidate) === candidate) throw error;
      missing.push(path.basename(candidate));
      candidate = path.dirname(candidate);
    }
  }
}

function inside(root, target) {
  const relative = path.relative(root, target);
  return relative === "" || (!relative.startsWith("..") && !path.isAbsolute(relative));
}

function owns(patterns, relative) {
  return patterns.some((pattern) => pattern === "*" || pattern === relative
    || (pattern.endsWith("/**") && (relative === pattern.slice(0, -3) || relative.startsWith(pattern.slice(0, -2)))));
}

async function ownership(event, project, canonical, identity) {
  if (event.event !== "PreToolUse" || event.operation.kind !== "file_change") return undefined;
  if (identity.role === "unknown") return deny("Registered Agent-Team ownership is missing for this session.");
  if (identity.role === "project_owner") return undefined;
  const registeredWorktree = await realpath(identity.team.worktree);
  if (project.worktreeRoot !== registeredWorktree) return deny("The registered team does not own this worktree.");
  const patterns = identity.team["owned paths"].split(/\s*,\s*/).filter(Boolean);

  for (const file of event.operation.files) {
    for (const changedPath of [file.previousPath, file.path].filter(Boolean)) {
      if (changedPath === "MISTAKES.md" || changedPath.startsWith(".agent-team/")) {
        return deny(`Only the project owner can change the shared path ${changedPath}.`);
      }
      const target = await canonicalTarget(event.cwd, changedPath);
      if (!inside(registeredWorktree, target)) return deny(`The changed path ${changedPath} resolves outside the registered worktree.`);
      const relative = path.relative(registeredWorktree, target).replaceAll("\\", "/");
      if (relative === "CONTEXT.md") continue;
      if (!owns(patterns, relative)) return deny(`The registered team does not own ${changedPath}.`);
    }
  }
  return undefined;
}

async function gitValue(cwd, args) {
  const { stdout } = await run("git", args, { cwd, encoding: "utf8", timeout: 1500, maxBuffer: 16 * 1024 });
  return stdout.trim();
}

async function gitEvidence(project, state) {
  const head = await gitValue(project.worktreeRoot, ["rev-parse", "HEAD"]);
  const base = await gitValue(project.worktreeRoot, ["rev-parse", state.baseRef]);
  const remote = await gitValue(project.worktreeRoot, ["rev-parse", state.remoteRef]);
  const delta = await gitValue(project.worktreeRoot, ["status", "--porcelain", "--untracked-files=no"]);
  return { head, base, remote, clean: delta === "" };
}

async function integrationGate(event, project, canonical, operation, now) {
  const gate = canonical.state.integration ?? {};
  if (event.sessionId !== canonical.registry.integrationOwner || event.sessionId !== gate.ownerSessionId) return deny("The registered integration owner must run this operation.");
  if (!gate.authorized) return deny("Integration authorization is missing.");
  if (!fresh(gate.evidenceAt, now)) return deny("Integration evidence is stale or unavailable.");
  if (gate.paused) return deny("Integration is paused.");
  if (gate.hold) return deny("An integration hold is active.");
  if (!gate.recoveryReconciled) return deny("Integration recovery evidence is not reconciled.");
  if (!gate.deltaClean) return deny("The recorded integration delta is not clean.");
  if (gate.preview?.required && gate.preview.approvedRevision !== gate.expectedRevision) return deny("Preview approval does not match the integration revision.");
  if (gate.updatesRemoteMain && gate.remoteMainDeploys && !canonical.state.release?.autoDeploy) {
    return deny("Updating remote main is a deployment trigger, but automatic deployment is off.");
  }

  const targetProject = operation.repository ? await resolveProject(path.resolve(event.cwd, operation.repository)) : project;
  if (!targetProject.active || targetProject.commonDirectory !== project.commonDirectory) return deny("The integration repository does not match the canonical project.");
  let evidence;
  try {
    evidence = await gitEvidence(targetProject, gate);
  } catch {
    return deny("Current integration Git evidence is unavailable.");
  }
  if (evidence.head !== gate.expectedRevision) return deny("The integration revision does not match current HEAD.");
  if (evidence.base !== gate.baseRevision) return deny("The integration base does not match current Git evidence.");
  if (evidence.remote !== gate.remoteRevision) return deny("The integration remote revision does not match current Git evidence.");
  if (!evidence.clean) return deny("The current integration delta is not clean.");
  return undefined;
}

async function releaseGate(event, project, canonical, now) {
  const gate = canonical.state.release ?? {};
  if (event.sessionId !== gate.ownerSessionId || event.sessionId !== canonical.registry.integrationOwner) return deny("The registered release owner must run this operation.");
  if (!gate.authorized) return deny("Release authorization is missing.");
  if (!gate.target) return deny("The release target is unknown.");
  if (!gate.recoveryReady) return deny("Release recovery evidence is missing.");
  if (gate.hold) return deny("A release hold is active.");
  if (!fresh(gate.evidenceAt, now)) return deny("Release evidence is stale or unavailable.");
  let head;
  try {
    head = await gitValue(project.worktreeRoot, ["rev-parse", "HEAD"]);
  } catch {
    return deny("Current release Git evidence is unavailable.");
  }
  if (head !== gate.expectedRevision) return deny("The release revision does not match current HEAD.");
  return undefined;
}

function databaseGate(event, canonical, operation, now) {
  const gate = canonical.state.database ?? {};
  if (operation.parserFailed) return deny("The recognized destructive database operation could not be parsed safely.");
  if (event.sessionId !== gate.ownerSessionId) return deny("The recorded database operation owner must run this operation.");
  if (!gate.authorized) return deny("Destructive database authorization is missing.");
  if (!gate.environment) return deny("The database target environment is unknown.");
  if (gate.environment === "production" && !gate.productionApproved) return deny("Production database approval is missing.");
  if (!gate.fixtureTarget) {
    if (!fresh(gate.inventoryAt, now)) return deny("Database inventory evidence is stale or unavailable.");
    if (!fresh(gate.recovery?.verifiedAt, now)) return deny("Database recovery evidence is stale or unavailable.");
    if (!fresh(gate.dryRunAt, now)) return deny("Database dry-run evidence is stale or unavailable.");
  }
  if (operation.cascade && !gate.allowCascade) return deny("Cascade authorization is missing for this database operation.");
  return undefined;
}

async function completionGate(event, project, canonical, operation) {
  const identity = identityFor(canonical.registry, event.sessionId);
  if (identity.role === "unknown") return deny("Registered task ownership is missing for completion.");
  const gate = canonical.state.completion ?? {};
  const taskId = operation.taskId ?? gate.taskId;
  const task = canonical.tasks.find((entry) => entry.id === taskId);
  if (!task || (identity.role === "team" && task.owner !== identity.team["team id"])) return deny("The canonical task owner does not match this completion.");
  let head;
  try {
    head = await gitValue(project.worktreeRoot, ["rev-parse", "HEAD"]);
  } catch {
    return deny("Current completion Git evidence is unavailable.");
  }
  if (gate.evidenceRevision !== head) return deny("Completion evidence does not match the current revision.");
  if (!gate.requirementsReconciled) return deny("Completion requirements are not reconciled.");
  if (gate.review?.status !== "passed" || gate.review.revision !== head) return deny("Completion review evidence is missing or stale.");
  if (!gate.checks?.length || gate.checks.some((check) => check.status !== "passed" || check.revision !== head)) {
    return deny("Completion check evidence is missing or stale.");
  }
  return undefined;
}

/** Apply deterministic gates first, then return bounded advice for changed content. */
export async function evaluatePolicy(event, project, { now = new Date() } = {}) {
  if (!project.active) return decision({ capabilities: { activation: "inactive" } });
  let canonical;
  let operation;
  try {
    canonical = await loadCanonicalState(project);
    operation = classifyOperation(event, canonical.state.operationMappings);
  } catch {
    const recognized = ["integration", "release", "database_destructive", "completion"].includes(classifyOperation(event).kind);
    return recognized ? deny("Canonical Agent-Team state is unavailable for this critical operation.") : decision({
      messages: ["Agent-Team advisory checks are unavailable because canonical state could not be read."],
      capabilities: { policy: "unavailable" },
    });
  }

  const identity = identityFor(canonical.registry, event.sessionId);
  const policyEvent = operation.kind === "file_change" ? { ...event, operation } : event;
  if (operation.kind === "file_change" && operation.parserFailed) return deny("The recognized file operation could not be resolved to a path.");
  let ownershipDecision;
  try {
    ownershipDecision = await ownership(policyEvent, project, canonical, identity);
  } catch {
    if (event.event === "PreToolUse" && policyEvent.operation.kind === "file_change") {
      return deny("Canonical ownership evidence is unavailable for this file operation.");
    }
    return decision({ messages: ["Agent-Team ownership advice is unavailable for this event."], capabilities: { ownership: "unavailable" } });
  }
  if (ownershipDecision) return ownershipDecision;
  if (operation.kind === "integration") return (await integrationGate(event, project, canonical, operation, now)) ?? decision();
  if (operation.kind === "release") return (await releaseGate(event, project, canonical, now)) ?? decision();
  if (operation.kind === "database_destructive") return databaseGate(event, canonical, operation, now) ?? decision();
  if (operation.kind === "completion") return (await completionGate(event, project, canonical, operation)) ?? decision();
  if (operation.kind === "database_blind_spot") return decision({
    messages: ["This database-like shell path is not mapped, so enforcement coverage is unavailable."],
    capabilities: { database: "unsupported_path" },
  });

  if (policyEvent.operation.kind === "file_change") {
    const findings = analyzeChangedFiles(policyEvent.operation.files, canonical.state.advisories ?? {});
    const messages = findings.map(({ message, path: file }) => `${file}: ${message}`);
    const capabilities = {};
    if (event.event === "PostToolUse") {
      const lint = await runLintChecks(project.worktreeRoot, policyEvent.operation.files.map(({ path: file }) => file));
      capabilities.lint = lint;
      if (lint.status === "failed" || lint.status === "timeout") messages.push(`Changed-file lint ${lint.status}.`);
      else if (lint.status === "skipped" && lint.reason === "missing_executable") messages.push("Changed-file lint skipped because no installed executable was found.");
    }
    return decision({ messages, context: { findings }, capabilities });
  }
  return decision();
}
