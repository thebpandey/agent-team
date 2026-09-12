import { constants } from "node:fs";
import { mkdir, open, readFile, rename, rm, writeFile } from "node:fs/promises";
import path from "node:path";
import { randomUUID } from "node:crypto";
import { withDirectoryLock } from "./lock.mjs";
import { readTracker } from "./tracker.mjs";
import { assertNoOwnerRecoveryJournal, repairOwnerRecovery } from "./owner-recovery.mjs";

const criticalMappingKinds = new Set(["file_change", "integration", "release", "database_destructive", "completion"]);

async function text(file) {
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
    if (error.code === "ENOENT") return "";
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
  await repairOwnerRecovery(project, options);
  const [teamsText, taskResult, stateText, setupText, ownerHistoryText] = await Promise.all([
    text(project.paths.teams),
    options.includeTasks === false ? null : loadCanonicalTracker(project, options),
    text(project.paths.state),
    text(project.paths.setup),
    project.paths.ownerHistory ? text(project.paths.ownerHistory) : "",
  ]);
  try { await assertNoOwnerRecoveryJournal(project); }
  catch (error) {
    if (error.message !== "owner_recovery_in_progress" || options.ownerRecoveryRetry === false) throw error;
    await repairOwnerRecovery(project, options);
    return loadCanonicalState(project, { ...options, ownerRecoveryRetry: false });
  }
  let state = {};
  if (stateText) state = JSON.parse(stateText);
  const setup = setupText ? JSON.parse(setupText) : {};
  let ownerHistory;
  if (ownerHistoryText) {
    ownerHistory = JSON.parse(ownerHistoryText);
    const epoch = state.ownership?.epoch;
    if (ownerHistory.schemaVersion !== 1 || !Number.isSafeInteger(ownerHistory.version) || ownerHistory.version < 1
      || !Array.isArray(ownerHistory.entries) || !Number.isSafeInteger(epoch) || epoch < 1
      || setup.ownership?.epoch !== epoch || ownerHistory.ownership?.epoch !== epoch
      || JSON.stringify(setup.ownership?.current) !== JSON.stringify(state.ownership?.current)
      || state.ownership?.current?.sessionId !== label(teamsText, "Project owner")
      || state.ownership?.current?.host !== label(teamsText, "Project owner host")) {
      throw new Error("owner_generation_mismatch");
    }
  } else if (state.ownership !== undefined || setup.ownership !== undefined) {
    throw new Error("owner_history_missing");
  }
  return {
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
  return { tracker: result.tracker, tasks };
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
