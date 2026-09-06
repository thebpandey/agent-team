import { mkdir, readFile, rename, rm, writeFile } from "node:fs/promises";
import path from "node:path";
import { randomUUID } from "node:crypto";
import { withDirectoryLock } from "./lock.mjs";

const criticalMappingKinds = new Set(["file_change", "integration", "release", "database_destructive", "completion"]);

async function text(file) {
  try {
    return await readFile(file, "utf8");
  } catch (error) {
    if (error.code === "ENOENT") return "";
    throw error;
  }
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
    output.push(rows);
  }
  return output;
}

function cells(line) {
  return line.trim().replace(/^\||\|$/g, "").split("|").map((entry) => entry.trim());
}

/** Read identities and task status from their canonical records; state.json only carries gate evidence. */
export async function loadCanonicalState(project) {
  const [teamsText, tasksText, stateText] = await Promise.all([
    text(project.paths.teams),
    text(project.paths.tasks),
    text(project.paths.state),
  ]);
  let state = {};
  if (stateText) state = JSON.parse(stateText);
  return {
    state,
    registry: {
      projectId: label(teamsText, "Project"),
      projectOwner: label(teamsText, "Project owner"),
      integrationOwner: label(teamsText, "Integration owner"),
      teams: tables(teamsText).flat().filter((row) => row["team id"]),
    },
    tasks: tables(tasksText).flat().filter((row) => row.id),
  };
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

/** Read the independent mapping inventory used when operational state cannot be parsed. */
export async function loadOperationMappingInventory(project) {
  const source = await readFile(project.paths.operationMappings, "utf8");
  if (Buffer.byteLength(source) > 256 * 1024) throw new Error("Operation mapping inventory exceeds 256 KiB.");
  const inventory = JSON.parse(source);
  if (inventory?.schemaVersion !== 1) throw new Error("Operation mapping inventory schema is unsupported.");
  if (inventory.kind !== "agent-team-operation-mapping-cache"
    || inventory.projectId !== project.projectId
    || inventory.sourcePath !== ".agent-team/state.json") throw new Error("Operation mapping inventory identity is invalid.");
  return validateOperationMappings(inventory.operationMappings);
}

function sameMappings(left, right) {
  return JSON.stringify(left) === JSON.stringify(right);
}

async function atomicWrite(file, source) {
  await mkdir(path.dirname(file), { recursive: true, mode: 0o700 });
  const temporary = `${file}.${process.pid}.${randomUUID()}.tmp`;
  try {
    await writeFile(temporary, source, { encoding: "utf8", mode: 0o600, flag: "wx" });
    await rename(temporary, file);
  } finally {
    await rm(temporary, { force: true });
  }
}

/** Write the validated mapping cache as a project-owner policy receipt under one canonical lock. */
export async function syncOperationMappingInventory(project, event) {
  if (!project.active) throw new Error("An active Agent-Team project is required.");
  if (!event || !["codex", "claude"].includes(event.runtime)
    || !["SessionStart", "PostToolUse", "PostToolBatch"].includes(event.event)) {
    throw new Error("A supported runtime hook event is required to update the operation mapping cache.");
  }
  return withDirectoryLock(path.join(project.paths.locks, "operation-mappings.lock"), {
    kind: "operation_mapping_cache",
    projectId: project.projectId,
    sessionId: event.sessionId,
  }, async () => {
    const canonical = await loadCanonicalState(project);
    if (identityFor(canonical.registry, event.sessionId).role !== "project_owner") {
      throw new Error("Only the canonical project owner can update the operation mapping cache.");
    }
    const operationMappings = validateOperationMappings(canonical.state.operationMappings);
    const receipt = {
      schemaVersion: 1,
      kind: "agent-team-operation-mapping-cache",
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
    await atomicWrite(project.paths.operationMappings, source);
    return { status: "completed", changed: true };
  });
}

/** Report whether the fallback cache exists, validates, and matches healthy operational state. */
export async function operationMappingHealth(project) {
  let cached;
  try {
    cached = await loadOperationMappingInventory(project);
  } catch (error) {
    return {
      status: error.code === "ENOENT" ? "missing" : "invalid",
      fallbackProtection: "unavailable",
    };
  }
  try {
    const canonical = await loadCanonicalState(project);
    const current = validateOperationMappings(canonical.state.operationMappings);
    return sameMappings(cached, current)
      ? { status: "current", fallbackProtection: "available" }
      : { status: "stale", fallbackProtection: "stale" };
  } catch {
    return { status: "available", fallbackProtection: "available" };
  }
}

export function identityFor(registry, sessionId) {
  if (sessionId === registry.projectOwner) return { role: "project_owner", sessionId };
  const team = registry.teams.find((entry) => entry.session.split(/\s*,\s*/).includes(sessionId));
  return team ? { role: "team", sessionId, team } : { role: "unknown", sessionId };
}
