import { execFile } from "node:child_process";
import { mkdir, readFile, realpath, writeFile } from "node:fs/promises";
import { createHash } from "node:crypto";
import path from "node:path";
import { promisify } from "node:util";
import { analyzeChangedFiles } from "./analyzers.mjs";
import {
  identityFor,
  loadCanonicalState,
  loadCanonicalTracker,
  loadOperationMappingInventory,
  validateOperationMappings,
} from "./canonical-state.mjs";
import { runLintChecks, lintMessages } from "./lint.mjs";
import { classifyOperation } from "./operation.mjs";
import { resolveProject } from "./project.mjs";
import { readRunDecision, validateEffectiveRun } from "./run-state.mjs";

const run = promisify(execFile);

function decision({ allow = true, mode = "advisory", messages = [], context = {}, capabilities = {}, mutations = [] } = {}) {
  return { mode, allow, messages, context, capabilities, mutations };
}

function deny(message, context = {}) {
  return decision({ allow: false, mode: "enforce", messages: [message], context });
}

/** Select the safe native fallback when canonical/runtime evidence cannot be read. */
export function unavailableDecision(event, mappings = {}, { inventoryStatus = "unavailable" } = {}) {
  const operation = classifyOperation(event, mappings);
  const critical = !["PostToolUse", "PostToolBatch"].includes(event.event)
    && ["file_change", "integration", "release", "database_destructive", "completion", "agent_team_native_command"].includes(operation.kind);
  const cache = ["missing", "invalid"].includes(inventoryStatus)
    ? ` Agent-Team mapping cache is ${inventoryStatus}; mapped critical protection is unavailable.`
    : "";
  return critical ? deny(`Agent-Team evidence is unavailable for this potentially critical operation.${cache}`) : decision({
    messages: [`Agent-Team advisory checks are unavailable for this event.${cache}`],
    capabilities: { policy: "unavailable", operationMappings: inventoryStatus },
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
  if (typeof identity.team.worktree !== "string" || !identity.team.worktree.trim()) return deny("The registered team worktree is missing.");
  const registeredWorktree = await realpath(path.resolve(project.root, identity.team.worktree));
  if (project.worktreeRoot !== registeredWorktree) return deny("The registered team does not own this worktree.");
  const patterns = identity.team["owned paths"].split(/\s*,\s*/).filter(Boolean);

  for (const file of event.operation.files) {
    for (const changedPath of [file.previousPath, file.path].filter(Boolean)) {
      if (changedPath === "MISTAKES.md" || changedPath.startsWith(".agent-team/")) {
        return deny(`Only the project owner can change the shared path ${changedPath}.`);
      }
      const target = await canonicalTarget(event.cwd, changedPath);
      if (target === project.tracker?.path) return deny("Only the project owner can change the selected canonical tracker.");
      if (!inside(registeredWorktree, target)) return deny(`The changed path ${changedPath} resolves outside the registered worktree.`);
      const relative = path.relative(registeredWorktree, target).replaceAll("\\", "/");
      if (relative === "CONTEXT.md") continue;
      if (!owns(patterns, relative)) return deny(`The registered team does not own ${changedPath}.`);
    }
  }
  return undefined;
}

async function gitValue(cwd, args, budget) {
  const { stdout } = await run("git", args, { cwd, encoding: "utf8", timeout: budget?.timeout(1500) ?? 1500,
    ...(budget ? { signal: budget.signal } : {}), maxBuffer: 16 * 1024 });
  return stdout.trim();
}

async function remoteRevision(cwd, remote, ref, budget) {
  if (!remote || !ref) throw new Error("Remote evidence fields are missing.");
  const output = await gitValue(cwd, ["ls-remote", "--exit-code", remote, ref], budget);
  const matches = output.split(/\r?\n/).filter(Boolean).map((line) => line.split(/\s+/)).filter(([, name]) => name === ref);
  if (matches.length !== 1 || !/^[0-9a-f]{40,64}$/i.test(matches[0][0])) throw new Error("Remote evidence is ambiguous.");
  return matches[0][0];
}

async function remoteTarget(cwd, remote, ref, budget) {
  try { return { status: "present", revision: await remoteRevision(cwd, remote, ref, budget) }; }
  catch (error) { if (error?.code === 2) return { status: "absent" }; throw error; }
}

async function gitEvidence(project, state, budget) {
  const head = await gitValue(project.worktreeRoot, ["rev-parse", "HEAD"], budget);
  const base = await gitValue(project.worktreeRoot, ["rev-parse", state.baseRef], budget);
  const delta = await gitValue(project.worktreeRoot, ["status", "--porcelain", "--untracked-files=no"], budget);
  const [baseRemote, target] = await Promise.all([
    remoteRevision(project.worktreeRoot, state.remoteName, state.baseRemoteRef, budget),
    remoteTarget(project.worktreeRoot, state.remoteName, state.remoteRef, budget),
  ]);
  return { head, base, baseRemote, target, clean: delta === "" };
}

async function isAncestor(cwd, ancestor, descendant, budget) {
  try {
    await run("git", ["merge-base", "--is-ancestor", ancestor, descendant], { cwd, encoding: "utf8", timeout: budget?.timeout(1500) ?? 1500,
      ...(budget ? { signal: budget.signal } : {}), maxBuffer: 16 * 1024 });
    return true;
  } catch (error) {
    if (error?.code === 1) return false;
    throw error;
  }
}

function integrationAuthorization(gate, now) {
  const authorization = gate.authorization;
  return authorization && typeof authorization === "object" && typeof authorization.source === "string" && authorization.source.trim()
    && authorization.source.length <= 256 && authorization.scope === "integration" && authorization.ownerSessionId === gate.ownerSessionId
    && authorization.revision === gate.expectedRevision && sameIds(authorization.taskIds, gate.taskIds) && fresh(authorization.observedAt, now);
}

function currentOwner(canonical, event, gate = {}) {
  const identity = identityFor(canonical.registry, event.runtime, event.sessionId);
  if (identity.role !== "project_owner" || event.sessionId !== gate.ownerSessionId) return false;
  if (!canonical.registry.projectOwnerHost) return true;
  return gate.ownerHost === canonical.registry.projectOwnerHost
    && gate.ownershipEpoch === canonical.registry.ownershipEpoch;
}

function configuredRemoteBaseRef(baseRef) {
  return typeof baseRef === "string" && /^[A-Za-z0-9][\w./-]*$/.test(baseRef)
    && !baseRef.startsWith("refs/") && !/[./]$|\.\.|\/\//.test(baseRef) ? `refs/heads/${baseRef}` : undefined;
}

function nonForcePushMatches(operation, gate) {
  return operation.method !== "push" || (operation.push?.valid === true && operation.push.remote === gate.remoteName
    && operation.push.targetRef === gate.remoteRef);
}

async function annotatedTagObject(cwd, sourceRef, revision, budget) {
  try {
    const object = await gitValue(cwd, ["rev-parse", "--verify", `${sourceRef}^{object}`], budget);
    const [type, peeled] = await Promise.all([
      gitValue(cwd, ["cat-file", "-t", object], budget),
      gitValue(cwd, ["rev-parse", `${object}^{}`], budget),
    ]);
    return type === "tag" && peeled === revision ? object : undefined;
  } catch {
    return undefined;
  }
}

async function integrationGate(event, project, canonical, operation, now, budget) {
  const gate = canonical.state.integration ?? {};
  if (event.sessionId !== canonical.registry.integrationOwner || !currentOwner(canonical, event, gate)) return deny("The registered integration owner must run this operation.");
  if (!gate.authorized) return deny("Integration authorization is missing.");
  if (!integrationAuthorization(gate, now)) return deny("Integration authorization provenance is missing or mismatched.");
  if (gate.baseRemoteRef !== configuredRemoteBaseRef(gate.baseRef)) return deny("The integration remote base does not match the configured integration branch.");
  if (!fresh(gate.evidenceAt, now)) return deny("Integration evidence is stale or unavailable.");
  if (gate.paused) return deny("Integration is paused.");
  if (gate.hold) return deny("An integration hold is active.");
  if (!gate.recoveryReconciled) return deny("Integration recovery evidence is not reconciled.");
  if (!gate.deltaClean) return deny("The recorded integration delta is not clean.");
  if (gate.preview?.required && gate.preview.approvedRevision !== gate.expectedRevision) return deny("Preview approval does not match the integration revision.");
  if (gate.updatesRemoteMain && gate.remoteMainDeploys) {
    const release = canonical.state.release ?? {};
    const authorization = release.authorization ?? {};
    if (!release.autoDeploy) return deny("Updating remote main is a deployment trigger, but automatic deployment is off.");
    if (!releaseAuthorityReady(canonical, { now, process: release.process })) {
      return deny("The deployment-triggering integration does not match the exact frozen selected task IDs.");
    }
    if (release.authorized !== true || release.runMode !== "auto_deploy" || release.ownerSessionId !== canonical.registry.integrationOwner
      || release.expectedRevision !== gate.expectedRevision || release.trackerFingerprint !== canonical.tracker.fingerprint
      || !fresh(release.evidenceAt, now) || release.remoteMainDeploys !== true || release.hold || release.projectPaused || canonical.state.run?.paused
      || typeof authorization.source !== "string" || !authorization.source.trim() || authorization.source.length > 256
      || authorization.target !== release.target || authorization.process !== release.process || !authorization.grantedAt
      || authorization.scope !== release.batchId || authorization.ownerSessionId !== release.ownerSessionId
      || authorization.revision !== release.expectedRevision || !sameIds(authorization.taskIds, release.taskIds)
      || !fresh(authorization.observedAt, now) || release.batch?.id !== release.batchId
      || !sameIds(release.batch?.taskIds, release.taskIds) || !releaseRun(release.run, release.runMode, release.taskIds)
      || !releaseArtifact(release.artifact, release.expectedRevision, release.taskIds)
      || !releaseRecord(release.integration, release.expectedRevision, release.taskIds) || release.integration.remoteMainDeploys !== true
      || !releaseRecord(release.verification, release.expectedRevision, release.taskIds)
      || release.delta?.status !== "clean" || release.delta.revision !== release.expectedRevision || !sameIds(release.delta.taskIds, release.taskIds)
      || release.recovery?.status !== "verified" || !release.recovery.artifactId || !release.recovery.action) {
      return deny("The automatic deployment authority does not match this exact release batch.");
    }
  }

  const targetProject = operation.repository ? await resolveProject(path.resolve(event.cwd, operation.repository), { budget }) : project;
  if (!targetProject.active || targetProject.commonDirectory !== project.commonDirectory) return deny("The integration repository does not match the canonical project.");
  let evidence;
  try {
    evidence = await gitEvidence(targetProject, gate, budget);
  } catch {
    return deny("Current integration Git evidence is unavailable.");
  }
  if (evidence.head !== gate.expectedRevision) return deny("The integration revision does not match current HEAD.");
  if (evidence.base !== gate.expectedRevision) return deny("The configured integration branch does not resolve to the exact integration revision.");
  if (evidence.baseRemote !== gate.baseRevision) return deny("The integration base does not match the current remote base.");
  if (gate.targetAbsent ? evidence.target.status !== "absent" : evidence.target.revision !== gate.remoteRevision) return deny("The integration remote revision does not match current Git evidence.");
  if (!evidence.clean) return deny("The current integration delta is not clean.");
  try { if (!await isAncestor(targetProject.worktreeRoot, gate.baseRevision, gate.expectedRevision, budget)) return deny("The observed remote base is not an ancestor of the exact integration revision."); }
  catch { return deny("Integration ancestry evidence is unavailable."); }
  if (!nonForcePushMatches(operation, gate)) return deny("Integration push must be the evidenced non-force advance.");
  let tagObject;
  if (operation.push?.sourceRef) {
    tagObject = await annotatedTagObject(targetProject.worktreeRoot, operation.push.sourceRef, gate.expectedRevision, budget);
    if (!tagObject) return deny("Integration tag push must preserve an annotated local tag for the exact integration revision.");
  }
  try {
    const target = await remoteTarget(targetProject.worktreeRoot, gate.remoteName, gate.remoteRef, budget);
    if (gate.targetAbsent ? target.status !== "absent" : target.revision !== gate.remoteRevision) return deny("The integration target changed after evidence was recorded.");
  } catch { return deny("Current integration target evidence is unavailable."); }
  if (tagObject) {
    try {
      const current = await gitValue(targetProject.worktreeRoot, ["rev-parse", "--verify", `${operation.push.sourceRef}^{object}`], budget);
      if (current !== tagObject) return deny("The validated local integration tag changed before the push boundary.");
    } catch { return deny("Current local integration tag evidence is unavailable."); }
  }
  return undefined;
}

function sameIds(left, right) {
  return Array.isArray(left) && Array.isArray(right)
    && left.length === right.length
    && [...left].sort().every((value, index) => value === [...right].sort()[index]);
}

function consequentialScope(canonical, operation) {
  const run = canonical.state?.run;
  const problem = validateEffectiveRun(run, canonical.tasks ?? []);
  if (problem) return `The ${operation.kind} effective run is unavailable (${problem}).`;
  const ids = operation.kind === "completion" ? [operation.taskId ?? canonical.state.completion?.taskId]
    : canonical.state?.[operation.kind]?.taskIds;
  if (!Array.isArray(ids) || !ids.length || new Set(ids).size !== ids.length || ids.some((id) => !run.taskIds.includes(id))) {
    return `The ${operation.kind} operation is outside the effective run scope.`;
  }
  const tasks = new Map((canonical.tasks ?? []).map((task) => [task.id, task]));
  if (ids.some((id) => { const task = tasks.get(id); return !task || task.isTopLevelDelivery === false || task.parentId !== null && task.parentId !== undefined; })) {
    return `The ${operation.kind} operation is outside the admitted top-level scope.`;
  }
  if ((canonical.state.quarantinedEvidence ?? []).some((entry) => ids.includes(entry?.taskId) && !entry?.reboundByOperationId)) {
    return "The consequential operation is quarantined pending explicit evidence rebind.";
  }
  return null;
}

function exactFrozenRelease(canonical) {
  const gate = canonical.state.release ?? {};
  const selected = readRunDecision(canonical).selectedBatchTaskIds;
  if (!selected.length || !sameIds(gate.taskIds, selected) || !sameIds(gate.recordedEvidence?.selectedTaskIds, selected)
    || !sameIds(gate.recordedEvidence?.taskIds, selected)) return false;
  for (const name of ["batch", "authorization", "artifact", "integration", "verification", "delta"]) {
    if (!sameIds(gate[name]?.taskIds, selected)) return false;
  }
  return selected.every((id) => canonical.deliveryEvidence?.[id]?.taskId === id);
}

function boundedString(value, maximum = 256) {
  return typeof value === "string" && value.trim() === value && value.length > 0 && value.length <= maximum;
}

function releaseRecord(record, revision, taskIds) {
  return record?.status === "passed" && record.revision === revision && sameIds(record.taskIds, taskIds);
}

function releaseRun(record, mode, taskIds) {
  return record && typeof record === "object" && boundedString(record.id, 128)
    && record.mode === mode && sameIds(record.taskIds, taskIds) && record.paused === false;
}

function releaseArtifact(record, revision, taskIds) {
  return record && typeof record === "object" && boundedString(record.id, 4096)
    && record.revision === revision && sameIds(record.taskIds, taskIds) && /^[0-9a-f]{64}$/i.test(record.sha256)
    && (record.path === undefined || boundedString(record.path, 4096))
    && (record.bytes === undefined || Number.isSafeInteger(record.bytes) && record.bytes > 0)
    && (record.checksumPath === undefined || boundedString(record.checksumPath, 4096))
    && (record.checksumBytes === undefined || Number.isSafeInteger(record.checksumBytes) && record.checksumBytes > 0)
    && (record.checksumSha256 === undefined || /^[0-9a-f]{64}$/i.test(record.checksumSha256))
    && (record.checksumEntry === undefined || boundedString(record.checksumEntry, 4096));
}

/** Pure aggregate check shared by policy and read-only recovery; performs no Git/provider probes. */
export function releaseAuthorityReady(canonical, { now = new Date(), process } = {}) {
  const gate = canonical.state?.release ?? {};
  const registry = canonical.registry ?? {};
  const authorization = gate.authorization ?? {};
  return [registry.projectOwnerHost, gate.ownerHost].every((host) => ["codex", "claude-code"].includes(host))
    && Number.isSafeInteger(registry.ownershipEpoch) && registry.ownershipEpoch > 0
    && gate.ownerHost === registry.projectOwnerHost && gate.ownerSessionId === registry.projectOwner
    && gate.ownerSessionId === registry.integrationOwner && gate.ownershipEpoch === registry.ownershipEpoch
    && gate.authorized === true && boundedString(gate.target, 4096) && boundedString(gate.process, 256)
    && (process === undefined || process === gate.process) && gate.trackerFingerprint === canonical.tracker?.fingerprint
    && canonical.git?.headRevision === gate.expectedRevision
    && typeof authorization.source === "string" && authorization.source.trim() && authorization.source.length <= 256
    && authorization.scope === gate.batchId && authorization.ownerSessionId === gate.ownerSessionId
    && authorization.target === gate.target && authorization.process === gate.process && authorization.grantedAt
    && authorization.revision === gate.expectedRevision && sameIds(authorization.taskIds, gate.taskIds)
    && fresh(authorization.observedAt, now) && fresh(gate.evidenceAt, now)
    && gate.batch?.id === gate.batchId && sameIds(gate.batch?.taskIds, gate.taskIds)
    && ((gate.runMode === "auto_deploy" && gate.autoDeploy === true) || (gate.runMode === "manual" && gate.autoDeploy === false))
    && releaseRun(gate.run, gate.runMode, gate.taskIds) && gate.projectPaused !== true && canonical.state?.run?.paused !== true && gate.hold !== true
    && releaseArtifact(gate.artifact, gate.expectedRevision, gate.taskIds)
    && releaseRecord(gate.integration, gate.expectedRevision, gate.taskIds)
    && typeof gate.remoteMainDeploys === "boolean" && gate.remoteMainDeploys === gate.integration.remoteMainDeploys
    && releaseRecord(gate.verification, gate.expectedRevision, gate.taskIds)
    && (gate.preview?.required ? gate.preview.status === "passed" && gate.preview.revision === gate.expectedRevision
      : ["not_required", "passed"].includes(gate.preview?.status))
    && gate.delta?.status === "clean" && gate.delta.revision === gate.expectedRevision && sameIds(gate.delta.taskIds, gate.taskIds)
    && gate.recovery?.status === "verified" && Boolean(gate.recovery.artifactId) && Boolean(gate.recovery.action)
    && exactFrozenRelease(canonical);
}

async function releaseGate(event, project, canonical, operation, now, budget) {
  const gate = canonical.state.release ?? {};
  if (!releaseAuthorityReady(canonical, { now, process: operation.process })) return deny("Release evidence does not match the exact frozen selected task IDs, ordinary gates, and current owner generation.");
  if (!currentOwner(canonical, event, gate) || event.sessionId !== canonical.registry.integrationOwner) return deny("The registered release owner must run this operation.");
  if (!gate.authorized) return deny("Release authorization is missing.");
  if (!gate.target) return deny("The release target is unknown.");
  const authorization = gate.authorization ?? {};
  if (typeof authorization.source !== "string" || !authorization.source.trim() || authorization.source.length > 256
    || !authorization.scope || !authorization.grantedAt || authorization.ownerSessionId !== gate.ownerSessionId
    || authorization.revision !== gate.expectedRevision || !sameIds(authorization.taskIds, gate.taskIds)
    || !fresh(authorization.observedAt, now)) return deny("Release authorization provenance is missing or mismatched.");
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
  if (!releaseRun(gate.run, gate.runMode, gate.taskIds)) return deny("Release run evidence is missing or inconsistent.");
  if (canonical.state.run?.paused || gate.projectPaused) return deny("The project run is paused.");
  if (gate.hold) return deny("A release hold is active.");
  if (!fresh(gate.evidenceAt, now)) return deny("Release evidence is stale or unavailable.");
  if (!releaseArtifact(gate.artifact, gate.expectedRevision, gate.taskIds)) {
    return deny("Release artifact identity, digest, revision, or task evidence is missing or inconsistent.");
  }
  if (!releaseRecord(gate.integration, gate.expectedRevision, gate.taskIds)) return deny("Required integration evidence is missing or stale.");
  if (typeof gate.remoteMainDeploys !== "boolean" || gate.remoteMainDeploys !== gate.integration.remoteMainDeploys) {
    return deny("The remote-main deployment binding is missing or inconsistent.");
  }
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
    head = await gitValue(project.worktreeRoot, ["rev-parse", "HEAD"], budget);
  } catch {
    return deny("Current release Git evidence is unavailable.");
  }
  if (head !== gate.expectedRevision) return deny("The release revision does not match current HEAD.");
  return undefined;
}

function databaseGate(event, canonical, operation, now) {
  const gate = canonical.state.database ?? {};
  if (operation.parserFailed) return deny("The recognized destructive database operation could not be parsed safely.");
  if (!currentOwner(canonical, event, gate)) return deny("The recorded database operation owner must run this operation.");
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

async function completionGate(event, project, canonical, operation, budget) {
  if (operation.parserFailed) return deny("Completion requires one explicit canonical task ID and an unambiguous command.");
  const identity = identityFor(canonical.registry, event.runtime, event.sessionId);
  if (identity.role === "unknown") return deny("Registered task ownership is missing for completion.");
  const gate = canonical.state.completion ?? {};
  const taskId = operation.taskId ?? gate.taskId;
  const task = canonical.tasks.find((entry) => entry.id === taskId);
  if (!task || (identity.role === "team" && task.owner !== identity.team["team id"])) return deny("The canonical task owner does not match this completion.");
  if (["partial", "blocked", "deferred"].includes(operation.outcome)) return undefined;
  let head;
  try {
    head = await gitValue(project.worktreeRoot, ["rev-parse", "HEAD"], budget);
  } catch {
    return deny("Current completion Git evidence is unavailable.");
  }
  if (gate.evidenceRevision !== head) return deny("Completion evidence does not match the current revision.");
  try {
    if (await gitValue(project.worktreeRoot, ["status", "--porcelain", "--untracked-files=no"], budget)) {
      return deny("Tracked content changed after the completion revision; rerun required checks on the committed delta.");
    }
  } catch {
    return deny("Current completion delta evidence is unavailable.");
  }
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

async function deduplicateAdvice(event, project, messages, advisories, budget) {
  // A disposable last-output digest suppresses repeated advice only; it never caches gate evidence.
  const key = createHash("sha256").update(`${event.runtime}:${event.sessionId}:${project.worktreeRoot}`).digest("hex");
  const file = path.join(project.paths.stateRoot, "cache", "advisories", `${key}.json`);
  const fingerprint = createHash("sha256").update(JSON.stringify({ messages, files: event.operation.files, advisories })).digest("hex");
  const bounded = (action) => budget ? budget.run(action) : action();
  try {
    const previous = JSON.parse(await bounded(() => readFile(file, { encoding: "utf8", ...(budget ? { signal: budget.signal } : {}) })));
    if (previous.fingerprint === fingerprint) return [];
  } catch { /* Missing or invalid cache means show advice again. */ }
  if (budget?.remaining() === 0) return messages;
  try {
    await bounded(() => mkdir(path.dirname(file), { recursive: true, mode: 0o700 }));
    await bounded(() => writeFile(file, JSON.stringify({ fingerprint }), { mode: 0o600, ...(budget ? { signal: budget.signal } : {}) }));
  } catch { /* Advisory cache failure cannot affect policy. */ }
  return messages;
}

/** Apply deterministic gates first, then return bounded advice for changed content. */
export async function evaluatePolicy(event, project, { now = new Date(), canonical: suppliedCanonical, budget, runBeads, progress = {} } = {}) {
  if (!project.active) return decision({ capabilities: { activation: "inactive" } });
  const bounded = (action) => budget ? budget.run(action) : action();
  let inventory = {};
  let inventoryStatus = "current";
  try {
    inventory = await bounded(() => loadOperationMappingInventory(project));
  } catch (error) {
    // A bad inventory cannot expand the set of operations that fail closed.
    inventoryStatus = error.code === "ENOENT" ? "missing" : "invalid";
  }
  // Preserve validated fallback classification before any later read can time out.
  progress.operation = classifyOperation(event, inventory, { tracker: project.tracker });
  let canonical;
  let operation;
  try {
    canonical = suppliedCanonical ?? await bounded(() => loadCanonicalState(project, { includeTasks: false, includeDeliveryEvidence: false, budget }));
    progress.canonical = canonical;
    operation = classifyOperation(event, validateOperationMappings(canonical.state.operationMappings ?? inventory), { tracker: project.tracker });
    progress.operation = operation;
  } catch {
    return unavailableDecision(event, inventory, { inventoryStatus });
  }

  const identity = identityFor(canonical.registry, event.runtime, event.sessionId);
  if (["completion", "release", "integration"].includes(operation.kind)) {
    if (canonical.tracker?.status === "not_read") Object.assign(canonical, await loadCanonicalTracker(project, { budget, runBeads }));
    if (canonical.tracker?.status !== "current") return deny(`The selected canonical tracker is unavailable (${canonical.tracker?.reason ?? "not_read"}).`);
    const scopeProblem = consequentialScope(canonical, operation);
    if (scopeProblem) return deny(scopeProblem);
    const recordedFingerprint = canonical.state[operation.kind]?.trackerFingerprint;
    if (recordedFingerprint && recordedFingerprint !== canonical.tracker.fingerprint) return deny("The selected tracker changed; recorded gate evidence is stale.");
  }
  if (!suppliedCanonical && ["completion", "release", "integration"].includes(operation.kind)) {
    try { canonical = await bounded(() => loadCanonicalState(project, { budget })); }
    catch { return unavailableDecision({ ...event, operation }, inventory, { inventoryStatus }); }
    progress.canonical = canonical;
    const scopeProblem = consequentialScope(canonical, operation);
    if (scopeProblem) return deny(scopeProblem);
    const recordedFingerprint = canonical.state[operation.kind]?.trackerFingerprint;
    if (recordedFingerprint && recordedFingerprint !== canonical.tracker.fingerprint) return deny("The selected tracker changed; recorded gate evidence is stale.");
  }
  const policyEvent = operation.files ? { ...event, operation: { ...operation, kind: "file_change" } } : event;
  if (operation.kind === "file_change" && operation.parserFailed) return deny("The recognized file operation could not be resolved to a path.");
  let ownershipDecision;
  try {
    ownershipDecision = await bounded(() => ownership(policyEvent, project, canonical, identity));
  } catch {
    return unavailableDecision({ ...event, operation }, inventory, { inventoryStatus });
  }
  if (ownershipDecision) return ownershipDecision;
  if (operation.kind === "integration") return (await integrationGate(event, project, canonical, operation, now, budget)) ?? decision();
  if (operation.kind === "release") return (await releaseGate(event, project, canonical, operation, now, budget)) ?? decision();
  if (operation.kind === "database_destructive") return databaseGate(event, canonical, operation, now) ?? decision();
  if (operation.kind === "completion") return (await completionGate(event, project, canonical, operation, budget)) ?? decision();
  if (operation.kind === "database_blind_spot") return decision({
    messages: ["This database-like shell path is not mapped, so enforcement coverage is unavailable."],
    capabilities: { database: "unsupported_path" },
  });

  if (policyEvent.operation.kind === "file_change") {
    const findings = analyzeChangedFiles(policyEvent.operation.files, canonical.state.advisories ?? {});
    const messages = findings.map(({ message, path: file }) => `${file}: ${message}`);
    const capabilities = {};
    if (["PostToolUse", "PostToolBatch"].includes(event.event)) {
      const lint = await runLintChecks(project.worktreeRoot, policyEvent.operation.files.map(({ path: file }) => file), { budget, progress });
      capabilities.lint = lint;
      messages.push(...lintMessages(lint, project.worktreeRoot));
    }
    const visibleMessages = ["PostToolUse", "PostToolBatch"].includes(event.event)
      ? await deduplicateAdvice(policyEvent, project, messages, canonical.state.advisories, budget) : messages;
    return decision({ messages: visibleMessages, context: { findings }, capabilities });
  }
  return decision();
}
