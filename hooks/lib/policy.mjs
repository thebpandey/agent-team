import { execFile } from "node:child_process";
import { realpath } from "node:fs/promises";
import path from "node:path";
import { promisify } from "node:util";
import { analyzeChangedFiles } from "./analyzers.mjs";
import {
  identityFor,
  loadCanonicalState,
  loadOperationMappingInventory,
  validateOperationMappings,
} from "./canonical-state.mjs";
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

/** Select the safe native fallback when canonical/runtime evidence cannot be read. */
export function unavailableDecision(event, mappings = {}) {
  const operation = classifyOperation(event, mappings);
  const critical = ["file_change", "integration", "release", "database_destructive", "completion"].includes(operation.kind);
  return critical ? deny("Agent-Team evidence is unavailable for this potentially critical operation.") : decision({
    messages: ["Agent-Team advisory checks are unavailable for this event."],
    capabilities: { policy: "unavailable" },
  });
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

async function remoteRevision(cwd, remote, ref) {
  if (!remote || !ref) throw new Error("Remote evidence fields are missing.");
  const output = await gitValue(cwd, ["ls-remote", "--exit-code", remote, ref]);
  const matches = output.split(/\r?\n/).filter(Boolean).map((line) => line.split(/\s+/)).filter(([, name]) => name === ref);
  if (matches.length !== 1 || !/^[0-9a-f]{40,64}$/i.test(matches[0][0])) throw new Error("Remote evidence is ambiguous.");
  return matches[0][0];
}

async function gitEvidence(project, state) {
  const head = await gitValue(project.worktreeRoot, ["rev-parse", "HEAD"]);
  const base = await gitValue(project.worktreeRoot, ["rev-parse", state.baseRef]);
  const delta = await gitValue(project.worktreeRoot, ["status", "--porcelain", "--untracked-files=no"]);
  const [baseRemote, remote] = await Promise.all([
    remoteRevision(project.worktreeRoot, state.remoteName, state.baseRemoteRef),
    remoteRevision(project.worktreeRoot, state.remoteName, state.remoteRef),
  ]);
  return { head, base, baseRemote, remote, clean: delta === "" };
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
  if (evidence.baseRemote !== gate.baseRevision) return deny("The integration base does not match the current remote base.");
  if (evidence.remote !== gate.remoteRevision) return deny("The integration remote revision does not match current Git evidence.");
  if (!evidence.clean) return deny("The current integration delta is not clean.");
  return undefined;
}

function sameIds(left, right) {
  return Array.isArray(left) && Array.isArray(right)
    && left.length === right.length
    && [...left].sort().every((value, index) => value === [...right].sort()[index]);
}

function releaseRecord(record, revision, taskIds) {
  return record?.status === "passed" && record.revision === revision && sameIds(record.taskIds, taskIds);
}

async function releaseGate(event, project, canonical, operation, now) {
  const gate = canonical.state.release ?? {};
  if (event.sessionId !== gate.ownerSessionId || event.sessionId !== canonical.registry.integrationOwner) return deny("The registered release owner must run this operation.");
  if (!gate.authorized) return deny("Release authorization is missing.");
  if (!gate.target) return deny("The release target is unknown.");
  const authorization = gate.authorization ?? {};
  if (!authorization.source || !authorization.scope || !authorization.grantedAt) return deny("Release authorization provenance is missing.");
  if (authorization.target !== gate.target || authorization.process !== gate.process || operation.process !== gate.process) {
    return deny("Release target or process authorization does not match this operation.");
  }
  if (!gate.batchId || authorization.scope !== gate.batchId || gate.batch?.id !== gate.batchId || !sameIds(gate.batch?.taskIds, gate.taskIds)) {
    return deny("Release batch membership is missing or inconsistent.");
  }
  if (!Array.isArray(gate.taskIds) || !gate.taskIds.length || gate.taskIds.some((id) => !canonical.tasks.some((task) => task.id === id))) {
    return deny("Release task IDs do not match the canonical tracker.");
  }
  if ((gate.runMode === "auto_deploy" && gate.autoDeploy !== true)
    || (gate.runMode === "manual" && gate.autoDeploy !== false)
    || !["auto_deploy", "manual"].includes(gate.runMode)) return deny("Release run mode is missing or inconsistent.");
  if (canonical.state.run?.paused || gate.projectPaused) return deny("The project run is paused.");
  if (gate.hold) return deny("A release hold is active.");
  if (!fresh(gate.evidenceAt, now)) return deny("Release evidence is stale or unavailable.");
  if (!gate.artifact?.id || gate.artifact.revision !== gate.expectedRevision || !sameIds(gate.artifact.taskIds, gate.taskIds)) {
    return deny("Release artifact evidence does not match the exact revision and task IDs.");
  }
  if (!releaseRecord(gate.integration, gate.expectedRevision, gate.taskIds)) return deny("Required integration evidence is missing or stale.");
  if (!releaseRecord(gate.verification, gate.expectedRevision, gate.taskIds)) return deny("Required release verification is missing or stale.");
  if (gate.preview?.required
    ? gate.preview.status !== "passed" || gate.preview.revision !== gate.expectedRevision
    : !["not_required", "passed"].includes(gate.preview?.status)) return deny("Required preview evidence is missing or stale.");
  if (gate.delta?.status !== "clean" || gate.delta.revision !== gate.expectedRevision || !sameIds(gate.delta.taskIds, gate.taskIds)) {
    return deny("The deployment delta is not limited to the recorded release scope.");
  }
  if (gate.recovery?.status !== "verified" || !gate.recovery.artifactId || !gate.recovery.action) return deny("Release recovery evidence is missing.");
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
    if (gate.dryRun?.capability === "supported") {
      if (!fresh(gate.dryRun.verifiedAt, now)) return deny("Database dry-run evidence is stale or unavailable.");
    } else if (gate.dryRun?.capability === "unsupported") {
      if (!gate.dryRun.reason || !gate.dryRun.alternative?.kind || !fresh(gate.dryRun.alternative.verifiedAt, now)) {
        return deny("Database alternative safety evidence is stale or unavailable for an operation without dry-run support.");
      }
    } else {
      return deny("Database dry-run capability is unknown.");
    }
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
  if (["partial", "blocked", "deferred"].includes(operation.outcome)) return undefined;
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
  for (const field of ["deployment", "cleanup"]) {
    if (!gate.scope?.[field]) continue;
    const record = gate[field];
    if (record?.status !== "passed" || record.taskId !== taskId || record.revision !== head) {
      return deny(`Completion ${field} evidence is missing or stale for this task and revision.`);
    }
  }
  return undefined;
}

/** Apply deterministic gates first, then return bounded advice for changed content. */
export async function evaluatePolicy(event, project, { now = new Date() } = {}) {
  if (!project.active) return decision({ capabilities: { activation: "inactive" } });
  let inventory = {};
  try {
    inventory = await loadOperationMappingInventory(project);
  } catch {
    // A bad inventory cannot expand the set of operations that fail closed.
  }
  let canonical;
  let operation;
  try {
    canonical = await loadCanonicalState(project);
    operation = classifyOperation(event, validateOperationMappings(canonical.state.operationMappings ?? inventory));
  } catch {
    return unavailableDecision(event, inventory);
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
  if (operation.kind === "release") return (await releaseGate(event, project, canonical, operation, now)) ?? decision();
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
    if (["PostToolUse", "PostToolBatch"].includes(event.event)) {
      const lint = await runLintChecks(project.worktreeRoot, policyEvent.operation.files.map(({ path: file }) => file));
      capabilities.lint = lint;
      if (lint.status === "failed" || lint.status === "timeout") messages.push(`Changed-file lint ${lint.status}.`);
      else if (lint.status === "skipped" && lint.reason === "missing_executable") messages.push("Changed-file lint skipped because no installed executable was found.");
    }
    return decision({ messages, context: { findings }, capabilities });
  }
  return decision();
}
