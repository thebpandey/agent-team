import { constants } from "node:fs";
import { mkdir, open, readFile, rename, rm, writeFile } from "node:fs/promises";
import path from "node:path";
import { randomUUID } from "node:crypto";
import { execFile } from "node:child_process";
import { promisify } from "node:util";
import { withDirectoryLock } from "./lock.mjs";
import { readTracker } from "./tracker.mjs";
import { assertNoOwnerRecoveryJournal, repairOwnerRecovery, validateOwnerHistory, validateQualifiedOwnership } from "./owner-recovery.mjs";

const criticalMappingKinds = new Set(["file_change", "integration", "release", "database_destructive", "completion"]);
const exec = promisify(execFile);
const revisionPattern = /^[a-f0-9]{40}$/;

async function text(file, { missing = "" } = {}) {
  let handle;
  try {
    handle = await open(file, constants.O_RDONLY | constants.O_NOFOLLOW | constants.O_NONBLOCK);
    const stat = await handle.stat();
    if (!stat.isFile() || stat.size > 1024 * 1024) throw new Error("unsafe_canonical_record");
    const bytes = Buffer.alloc(stat.size);
    let offset = 0;
    while (offset < bytes.length) {
      const { bytesRead } = await handle.read(bytes, offset, bytes.length - offset, offset);
      if (!bytesRead) break;
      offset += bytesRead;
    }
    if (offset !== bytes.length) throw new Error("canonical_record_changed");
    return bytes.toString("utf8");
  } catch (error) {
    if (error.code === "ENOENT") return missing;
    throw error;
  } finally { await handle?.close(); }
}

function label(source, name) {
  return source.match(new RegExp(`^${name}:\\s*(.+)$`, "im"))?.[1].trim();
}

function tables(source) {
  const lines = source.split(/\r?\n/);
  const output = [];
  for (let index = 0; index < lines.length - 1; index += 1) {
    if (!/^\s*\|/.test(lines[index]) || !/^\s*\|(?:\s*:?-+:?\s*\|)+\s*$/.test(lines[index + 1])) continue;
    const headers = cells(lines[index]).map((entry) => entry.toLowerCase());
    const rows = [];
    index += 2;
    while (index < lines.length && /^\s*\|/.test(lines[index])) {
      const values = cells(lines[index]);
      rows.push(Object.fromEntries(headers.map((header, position) => [header, values[position] ?? ""])));
      index += 1;
    }
    output.push({ headers, rows });
  }
  return output;
}

function cells(line) {
  return line.trim().replace(/^\||\|$/g, "").split("|").map((entry) => entry.trim());
}

/** Read identities and task status from their canonical records; state.json only carries gate evidence. */
export async function loadCanonicalState(project, options = {}) {
  if (options.readOnly) await assertNoOwnerRecoveryJournal(project);
  else await repairOwnerRecovery(project, options);
  const [teamsText, taskResult, stateText, setupText, ownerHistoryText] = await Promise.all([
    text(project.paths.teams),
    options.includeTasks === false ? null : loadCanonicalTracker(project, options),
    text(project.paths.state),
    text(project.paths.setup),
    project.paths.ownerHistory ? text(project.paths.ownerHistory, { missing: null }) : null,
  ]);
  try { await assertNoOwnerRecoveryJournal(project); }
  catch (error) {
    if (options.readOnly || error.message !== "owner_recovery_in_progress" || options.ownerRecoveryRetry === false) throw error;
    await repairOwnerRecovery(project, options);
    return loadCanonicalState(project, { ...options, ownerRecoveryRetry: false });
  }
  let state = {};
  if (stateText) state = JSON.parse(stateText);
  const setup = setupText ? JSON.parse(setupText) : {};
  let ownerHistory;
  if (ownerHistoryText !== null) {
    try { ownerHistory = JSON.parse(ownerHistoryText); }
    catch { throw new Error("owner_history_invalid"); }
    const epoch = state.ownership?.epoch;
    if (!validateQualifiedOwnership(state.ownership) || !validateQualifiedOwnership(setup.ownership)
      || !validateOwnerHistory(ownerHistory, state.ownership) || !Number.isSafeInteger(epoch) || epoch < 1
      || setup.ownership?.epoch !== epoch || ownerHistory.ownership?.epoch !== epoch
      || JSON.stringify(setup.ownership?.current) !== JSON.stringify(state.ownership?.current)
      || state.ownership?.current?.sessionId !== label(teamsText, "Project owner")
      || state.ownership?.current?.host !== label(teamsText, "Project owner host")
      || state.integration?.ownerSessionId !== label(teamsText, "Integration owner")
      || state.integration?.ownerHost !== label(teamsText, "Integration owner host")
      || state.integration?.ownershipEpoch !== epoch || state.release?.ownerSessionId !== label(teamsText, "Integration owner")
      || state.release?.ownerHost !== label(teamsText, "Integration owner host") || state.release?.ownershipEpoch !== epoch) {
      throw new Error("owner_generation_mismatch");
    }
  } else if (state.ownership !== undefined || setup.ownership !== undefined || label(teamsText, "Project owner host") !== undefined
    || label(teamsText, "Integration owner host") !== undefined || state.integration?.ownerHost !== undefined
    || state.integration?.ownershipEpoch !== undefined || state.release?.ownerHost !== undefined || state.release?.ownershipEpoch !== undefined) {
    throw new Error("owner_history_missing");
  }
  const canonical = {
    state,
    setup,
    ownerHistory,
    registry: {
      projectId: label(teamsText, "Project"),
      projectOwner: label(teamsText, "Project owner"),
      projectOwnerHost: label(teamsText, "Project owner host"),
      integrationOwner: label(teamsText, "Integration owner"),
      integrationOwnerHost: label(teamsText, "Integration owner host"),
      ownershipEpoch: state.ownership?.epoch,
      teams: tables(teamsText).flatMap(({ rows }) => rows).filter((row) => row["team id"]),
    },
    tasks: taskResult?.tasks ?? [],
    tracker: taskResult?.tracker ?? { ...project.tracker, status: "not_read", fingerprint: null },
  };
  const headRevision = await loadHeadRevision(project, options.budget);
  canonical.git = { headRevision };
  canonical.deliveryEvidence = await loadDeliveryEvidence(project, state, canonical, options.budget);
  return canonical;
}

export async function loadCanonicalTracker(project, options = {}) {
  const result = await readTracker(project, options);
  const taskTables = tables(result.source).filter(({ headers }) => headers.includes("id"));
  const tasks = result.tasks ?? taskTables.flatMap(({ rows }) => rows);
  if (result.tracker.kind === "markdown" && result.tracker.status === "current") {
    if (!taskTables.length || taskTables.some(({ headers }) => !headers.includes("owner") || !headers.includes("status") || new Set(headers).size !== headers.length)
      || tasks.some((row) => !row.id || !row.owner || !row.status) || new Set(tasks.map(({ id }) => id)).size !== tasks.length) {
      return { tracker: { ...result.tracker, status: "unavailable", reason: "invalid_response", fingerprint: null }, tasks: [] };
    }
  }
  if (result.tracker.status !== "current") return { tracker: result.tracker, tasks };
  const ids = new Set(tasks.map(({ id }) => id));
  const normalized = tasks.map((task) => {
    const parentValue = ["parentId", "parent", "parent id", "parent_id"].find((key) => task[key] !== undefined);
    const typeValue = ["taskType", "type", "issue type", "issue_type"].find((key) => task[key] !== undefined);
    const rawParent = parentValue ? task[parentValue] : "";
    const rawType = typeValue ? task[typeValue] : "unknown";
    if (typeof rawParent !== "string" || typeof rawType !== "string") throw new Error("invalid_tracker_hierarchy");
    const parentId = rawParent.trim() || null;
    const lowered = rawType.trim().toLowerCase();
    const taskType = ["epic", "task"].includes(lowered) ? lowered : "unknown";
    const hierarchyUnknown = parentId !== null && !ids.has(parentId);
    return { ...task, parentId, taskType, isEpic: taskType === "epic", isSubtask: parentId !== null,
      isTopLevelDelivery: taskType !== "epic" && parentId === null, ...(hierarchyUnknown ? { hierarchyUnknown: true } : {}) };
  });
  return { tracker: result.tracker, tasks: normalized };
}

async function loadHeadRevision(project, budget) {
  try {
    const result = await exec("git", ["-C", project.root, "rev-parse", "HEAD"], {
      encoding: "utf8", timeout: budget?.timeout(1500) ?? 1500, maxBuffer: 16384, ...(budget ? { signal: budget.signal } : {}),
    });
    const revision = result.stdout.trim();
    return revisionPattern.test(revision) ? revision : null;
  } catch { return null; }
}

function ownerTriple(value) {
  return value && ["codex", "claude-code"].includes(value.ownerHost) && typeof value.ownerSessionId === "string"
    && Number.isSafeInteger(value.ownershipEpoch) && value.ownershipEpoch > 0
    ? `${value.ownerHost}\0${value.ownerSessionId}\0${value.ownershipEpoch}` : null;
}

function exactObjectKeys(value, keys) {
  return value && typeof value === "object" && !Array.isArray(value)
    && Object.keys(value).sort().join("\0") === [...keys].sort().join("\0");
}

function exactTaskIds(value, expected) {
  return Array.isArray(value) && value.length > 0 && new Set(value).size === value.length
    && value.every((taskId) => typeof taskId === "string" && taskId.length > 0 && Buffer.byteLength(taskId) <= 128)
    && JSON.stringify(value) === JSON.stringify(expected);
}

function targetAuthorityValid(value, { taskId, boundaryRevision, target, releaseOwner, integrationTaskIds }) {
  return exactObjectKeys(value, ["status", "source", "target", "revision", "taskIds", "ownerHost", "ownerSessionId", "ownershipEpoch"])
    && value.status === "authorized"
    && typeof value.source === "string" && value.source.trim() === value.source && value.source.length > 0 && Buffer.byteLength(value.source) <= 256
    && value.target === target && value.revision === boundaryRevision && exactTaskIds(value.taskIds, integrationTaskIds) && value.taskIds.includes(taskId)
    && ownerTriple(value) === releaseOwner;
}

async function loadDeliveryEvidence(project, state, canonical, budget) {
  const categories = ["completion", "integration", "review", "checks", "preview", "target", "recovery"];
  const receipts = state.deliveryReceipts;
  if (!revisionPattern.test(canonical.git?.headRevision ?? "") || !receipts || typeof receipts !== "object" || Array.isArray(receipts)) return {};
  const ids = new Set(categories.flatMap((category) => Object.keys(receipts[category] ?? {})));
  const ancestry = new Map();
  const isAncestor = async (ancestor, descendant) => {
    const key = `${ancestor}\0${descendant}`;
    if (ancestry.has(key)) return ancestry.get(key);
    let accepted = false;
    try {
      await exec("git", ["-C", project.root, "merge-base", "--is-ancestor", ancestor, descendant], {
        timeout: budget?.timeout(1500) ?? 1500, maxBuffer: 16384, ...(budget ? { signal: budget.signal } : {}),
      });
      accepted = true;
    } catch (error) {
      if (error.code !== 1) accepted = false;
    }
    ancestry.set(key, accepted);
    return accepted;
  };
  const integrationOwner = ownerTriple(state.integration);
  const releaseOwner = ownerTriple(state.release);
  const nestedReleaseOwner = ownerTriple(state.release?.authorization);
  const output = {};
  for (const taskId of ids) {
    const part = Object.fromEntries(categories.map((category) => [category, receipts[category]?.[taskId]]));
    if (Object.values(part).some((value) => !value || typeof value !== "object" || value.taskId !== taskId)) continue;
    const sourceRevision = part.completion.sourceRevision;
    const boundaryRevision = part.integration.boundaryRevision;
    if (!revisionPattern.test(sourceRevision) || !revisionPattern.test(boundaryRevision)
      || part.completion.status !== "passed" || part.integration.status !== "passed"
      || part.review.status !== "passed" || part.review.revision !== sourceRevision
      || part.checks.revision !== sourceRevision || !Array.isArray(part.checks.results) || !part.checks.results.length
      || part.checks.results.some((check) => !check || check.status !== "passed")
      || part.integration.sourceRevision !== sourceRevision || ownerTriple(part.integration) !== integrationOwner
      || !["passed", "not_required"].includes(part.preview.status) || part.preview.revision !== boundaryRevision
      || typeof part.preview.required !== "boolean" || (part.preview.required && part.preview.status !== "passed")
      || (!part.preview.required && part.preview.status !== "not_required") || ownerTriple(part.preview) !== integrationOwner
      || !releaseOwner || nestedReleaseOwner !== releaseOwner
      || part.target.status !== "authorized" || part.target.revision !== boundaryRevision || ownerTriple(part.target) !== releaseOwner
      || !targetAuthorityValid(part.target.authority, { taskId, boundaryRevision, target: part.target.target, releaseOwner,
        integrationTaskIds: state.integration?.taskIds })
      || part.recovery.status !== "ready" || part.recovery.revision !== boundaryRevision || ownerTriple(part.recovery) !== releaseOwner
      || typeof part.target.target !== "string" || !part.target.target || Buffer.byteLength(part.target.target) > 4096
      || typeof part.recovery.artifact !== "string" || !part.recovery.artifact || Buffer.byteLength(part.recovery.artifact) > 4096
      || typeof part.recovery.action !== "string" || !part.recovery.action || Buffer.byteLength(part.recovery.action) > 4096
      || !await isAncestor(sourceRevision, boundaryRevision) || !await isAncestor(boundaryRevision, canonical.git.headRevision)) continue;
    output[taskId] = { taskId, sourceRevision, revision: boundaryRevision, integratedRevision: boundaryRevision,
      completion: structuredClone(part.completion), integration: structuredClone(part.integration), review: structuredClone(part.review),
      checks: structuredClone(part.checks.results), preview: structuredClone(part.preview), target: structuredClone(part.target), recovery: structuredClone(part.recovery) };
  }
  return output;
}

function mapping(value, label) {
  if (!value || typeof value !== "object" || Array.isArray(value) || !criticalMappingKinds.has(value.kind)) {
    throw new Error(`${label} is not a recognized critical operation mapping.`);
  }
  for (const field of ["pathField", "contentField", "sqlField", "process"]) {
    if (value[field] !== undefined && (typeof value[field] !== "string" || !value[field])) {
      throw new Error(`${label}.${field} must be a non-empty string.`);
    }
  }
  if (value.action !== undefined && !["add", "add_or_edit", "edit", "delete", "move"].includes(value.action)) {
    throw new Error(`${label}.action is invalid.`);
  }
  return { ...value };
}

/** Validate the small mapping surface before it can affect unavailable-state enforcement. */
export function validateOperationMappings(value = {}) {
  if (!value || typeof value !== "object" || Array.isArray(value)) throw new Error("Operation mappings must be an object.");
  const providers = value.providers ?? {};
  const shell = value.shell ?? [];
  if (!providers || typeof providers !== "object" || Array.isArray(providers)) throw new Error("Provider mappings must be an object.");
  if (!Array.isArray(shell)) throw new Error("Shell mappings must be an array.");
  return {
    providers: Object.fromEntries(Object.entries(providers).map(([tool, value_]) => {
      if (!tool || tool.length > 256) throw new Error("A provider mapping tool name is invalid.");
      return [tool, mapping(value_, `Provider mapping ${tool}`)];
    })),
    shell: shell.map((value_, index) => {
      const checked = mapping(value_, `Shell mapping ${index}`);
      if (typeof checked.prefix !== "string" || !checked.prefix.trim() || checked.prefix.length > 512) {
        throw new Error(`Shell mapping ${index}.prefix is invalid.`);
      }
      return checked;
    }),
  };
}

function canonicalOperationMappings(state) {
  if (state?.schemaVersion !== 1 || !Object.hasOwn(state, "operationMappings")) {
    throw new Error("Canonical operation mapping state is invalid.");
  }
  return validateOperationMappings(state.operationMappings);
}

/** Read the independent mapping inventory used when operational state cannot be parsed. */
export async function loadOperationMappingInventory(project) {
  const source = await readFile(project.paths.operationMappings, "utf8");
  if (Buffer.byteLength(source) > 256 * 1024) throw new Error("Operation mapping inventory exceeds 256 KiB.");
  const inventory = JSON.parse(source);
  if (inventory?.schemaVersion !== 1) throw new Error("Operation mapping inventory schema is unsupported.");
  if (inventory.kind !== "agent-team-operation-mapping-cache"
    || inventory.authoritative !== false
    || inventory.projectId !== project.projectId
    || inventory.sourcePath !== ".agent-team/state.json") throw new Error("Operation mapping inventory identity is invalid.");
  return validateOperationMappings(inventory.operationMappings);
}

function sameMappings(left, right) {
  return JSON.stringify(left) === JSON.stringify(right);
}

async function atomicWrite(file, source, budget) {
  budget?.check();
  await mkdir(path.dirname(file), { recursive: true, mode: 0o700 });
  const temporary = `${file}.${process.pid}.${randomUUID()}.tmp`;
  try {
    budget?.check();
    await writeFile(temporary, source, { encoding: "utf8", mode: 0o600, flag: "wx", ...(budget ? { signal: budget.signal } : {}) });
    budget?.check();
    await rename(temporary, file);
  } finally {
    await rm(temporary, { force: true });
  }
}

/** Rebuild the non-authoritative mapping cache only from validated canonical state. */
export async function syncOperationMappingInventory(project, { canonical: suppliedCanonical, budget } = {}) {
  if (!project.active) throw new Error("An active Agent-Team project is required.");
  return withDirectoryLock(path.join(project.paths.locks, "operation-mappings.lock"), {
    kind: "operation_mapping_cache",
    projectId: project.projectId,
  }, async () => {
    const canonical = suppliedCanonical ?? await loadCanonicalState(project, { includeTasks: false });
    const operationMappings = canonicalOperationMappings(canonical.state);
    const receipt = {
      schemaVersion: 1,
      kind: "agent-team-operation-mapping-cache",
      authoritative: false,
      projectId: project.projectId,
      sourcePath: ".agent-team/state.json",
      operationMappings,
    };
    const source = `${JSON.stringify(receipt, null, 2)}\n`;
    let previous = "";
    try {
      previous = await readFile(project.paths.operationMappings, "utf8");
    } catch (error) {
      if (error.code !== "ENOENT") throw error;
    }
    if (previous === source) return { status: "completed", changed: false };
    budget?.check();
    await atomicWrite(project.paths.operationMappings, source, budget);
    return { status: "completed", changed: true };
  }, { budget });
}

const operationMappingThreatModel = {
  authoritative: false,
  source: "validated_canonical_state",
  purpose: "classification_fallback",
};

/** Report whether the fallback cache exists, validates, and matches healthy operational state. */
export async function operationMappingHealth(project) {
  let cached;
  try {
    cached = await loadOperationMappingInventory(project);
  } catch (error) {
    return {
      ...operationMappingThreatModel,
      status: error.code === "ENOENT" ? "missing" : "invalid",
      fallbackProtection: "unavailable",
    };
  }
  try {
    const canonical = await loadCanonicalState(project);
    const current = canonicalOperationMappings(canonical.state);
    return sameMappings(cached, current)
      ? { ...operationMappingThreatModel, status: "current", fallbackProtection: "available" }
      : { ...operationMappingThreatModel, status: "stale", fallbackProtection: "stale" };
  } catch {
    return { ...operationMappingThreatModel, status: "available", fallbackProtection: "available" };
  }
}

export function identityFor(registry, host, sessionId) {
  if (sessionId === undefined) { sessionId = host; host = undefined; }
  const normalizedHost = host === "claude" ? "claude-code" : host;
  if (sessionId === registry.projectOwner && (!registry.projectOwnerHost || normalizedHost === registry.projectOwnerHost)) {
    return { role: "project_owner", host: registry.projectOwnerHost ?? normalizedHost, sessionId, ownershipEpoch: registry.ownershipEpoch };
  }
  const team = registry.teams.find((entry) => entry.session.split(/\s*,\s*/).includes(sessionId));
  return team ? { role: "team", host: normalizedHost, sessionId, team } : { role: "unknown", host: normalizedHost, sessionId };
}
