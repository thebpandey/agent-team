import { execFile } from "node:child_process";
import { createHash } from "node:crypto";
import { constants } from "node:fs";
import { open } from "node:fs/promises";
import path from "node:path";
import { promisify } from "node:util";
import { resolveExecutionSettings, validateExecutionSettings } from "./settings.mjs";

const run = promisify(execFile);
const BEADS_READ_TIMEOUT_MS = 5000;

function executionSettings(project, host, snapshot) {
  if (snapshot !== undefined) {
    const configured = validateExecutionSettings(snapshot, "tracker execution settings snapshot");
    return resolveExecutionSettings({ settings: { hosts: { codex: { execution: configured } } } }, "codex");
  }
  if (host !== undefined) return resolveExecutionSettings(project.setup ?? {}, host);
  return resolveExecutionSettings({}, "codex");
}

async function readBoundedRegularFile(file, maximum, budget) {
  let handle;
  try {
    handle = await open(file, constants.O_RDONLY | constants.O_NOFOLLOW | constants.O_NONBLOCK);
    const before = await handle.stat();
    if (!before.isFile() || !Number.isSafeInteger(before.size) || before.size > maximum) {
      throw Object.assign(new Error("unsafe tracker file"), { code: "INVALID_RESPONSE" });
    }
    const bytes = Buffer.alloc(before.size);
    let offset = 0;
    while (offset < bytes.length) {
      budget?.check();
      const { bytesRead } = await handle.read(bytes, offset, bytes.length - offset, offset);
      if (!bytesRead) break;
      offset += bytesRead;
    }
    const extra = await handle.read(Buffer.alloc(1), 0, 1, offset);
    const after = await handle.stat();
    if (offset !== bytes.length || extra.bytesRead || after.size !== before.size || after.dev !== before.dev || after.ino !== before.ino
      || after.mtimeMs !== before.mtimeMs || after.ctimeMs !== before.ctimeMs) {
      throw Object.assign(new Error("tracker changed during read"), { code: "INVALID_RESPONSE" });
    }
    return bytes.toString("utf8");
  } catch (error) {
    if (["ELOOP", "EISDIR", "ENXIO"].includes(error.code)) error.code = "INVALID_RESPONSE";
    throw error;
  } finally { await handle?.close(); }
}

/** Pin routing to selected project metadata, while preserving authentication/runtime settings. */
export function beadsEnvironment(project, environment = process.env) {
  const env = { ...environment };
  const routing = ["DB", "DOLT_DATA_DIR", "DOLT_DATABASE", "DOLT_SERVER_DATABASE", "DOLT_HOST", "DOLT_PORT", "DOLT_SERVER_HOST",
    "DOLT_SERVER_PORT", "DOLT_SERVER_SOCKET", "DOLT_SHARED_SERVER", "DOLT_SERVER_MODE", "SHARED_SERVER_DIR",
    "ROUTING_MODE", "ROUTING_DEFAULT", "ROUTING_MAINTAINER", "ROUTING_CONTRIBUTOR"];
  for (const key of routing) for (const prefix of ["BEADS_", "BD_"]) delete env[`${prefix}${key}`];
  for (const key of ["GT_DOLT_DATA", "GIT_DIR", "GIT_WORK_TREE", "GIT_COMMON_DIR", "GIT_INDEX_FILE"]) delete env[key];
  env.BEADS_DIR = project.tracker?.path ?? resolveTracker(project.root, project.setup?.tracker).path;
  env.PWD = project.root;
  return env;
}

/** Selection is explicit; only absent legacy selectors use the historical path. */
export function resolveTracker(root, selection) {
  const selected = selection === undefined ? { kind: "markdown", path: ".agent-team/TASKS.md" } : selection;
  if (selected?.kind === "beads") {
    if (selected.executable !== undefined && (typeof selected.executable !== "string" || !path.isAbsolute(selected.executable) || selected.executable.includes("\0"))) {
      return { kind: "unknown", id: `invalid:${root}`, path: null, reason: "invalid_selection" };
    }
    const location = path.join(root, ".beads");
    return { kind: "beads", id: `beads:${location}`, path: location, executable: selected.executable ?? "bd", executableSource: selected.executable ? "selected" : "legacy_path" };
  }
  if (selected?.kind === "markdown" && ["TASKS.md", ".agent-team/TASKS.md"].includes(selected.path)) {
    const location = path.join(root, selected.path);
    return { kind: "markdown", id: `markdown:${location}`, path: location };
  }
  return { kind: "unknown", id: `invalid:${root}`, path: null, reason: "invalid_selection" };
}

export function trackerFingerprint(tracker, source) {
  return createHash("sha256").update(tracker.id).update("\0").update(source).digest("hex");
}

/** Read only the selected authority. Never synthesize a passing fallback ledger. */
export async function readTracker(project, { runBeads = run, budget, environment, host, executionSettings: settingsSnapshot } = {}) {
  const selected = project.tracker ?? resolveTracker(project.root, project.setup?.tracker);
  const tracker = { ...selected, status: "unavailable", fingerprint: null, observedAt: new Date().toISOString() };
  if (selected.reason) return { tracker, tasks: [], source: "" };
  try {
    const maxBuffer = executionSettings(project, host, settingsSnapshot).limits.subprocessMaxBufferBytes;
    let source;
    let tasks;
    if (selected.kind === "beads") {
      const result = await runBeads(selected.executable ?? "bd", ["list", "--all", "--limit", "0", "--json", "--readonly"], {
        cwd: project.root, encoding: "utf8", timeout: budget?.timeout(BEADS_READ_TIMEOUT_MS) ?? BEADS_READ_TIMEOUT_MS,
        maxBuffer, ...(budget ? { signal: budget.signal } : {}),
        // A host/worktree environment must not redirect the selected project authority.
        env: beadsEnvironment(project, environment),
      });
      source = result.stdout;
      const rows = JSON.parse(source);
      if (!Array.isArray(rows) || rows.some((row) => !row || typeof row.id !== "string" || !row.id
        || typeof row.status !== "string" || !row.status || typeof row.title !== "string"
        || (row.assignee !== undefined && typeof row.assignee !== "string")
        || (row.parent !== undefined && typeof row.parent !== "string")
        || (row.issue_type !== undefined && typeof row.issue_type !== "string"))
        || new Set(rows.map(({ id }) => id)).size !== rows.length) throw Object.assign(new Error("Invalid Beads rows"), { code: "INVALID_RESPONSE" });
      tasks = rows.map((row) => ({ id: row.id, owner: row.assignee ?? "", status: row.status,
        "requirement / acceptance": row.acceptance_criteria ?? row.description ?? row.title,
        "revision / evidence": row.notes ?? "", "next action": "", updatedAt: row.updated_at,
        ...(Number.isInteger(row.priority) ? { priority: row.priority } : {}),
        ...(typeof row.parent === "string" && row.parent ? { parent: row.parent, parentId: row.parent } : {}),
        ...(typeof row.issue_type === "string" ? { taskType: row.issue_type } : {}),
        dependencies: Array.isArray(row.dependencies) ? row.dependencies
          .filter((dep) => dep?.type === "blocks" && typeof dep.depends_on_id === "string")
          .map((dep) => dep.depends_on_id) : [],
        dependencyEvidence: (row.dependency_count === 0 || Array.isArray(row.dependencies)
          && row.dependencies.every((dep) => typeof dep?.depends_on_id === "string" && ["blocks", "parent-child", "related"].includes(dep.type))) ? "current" : "unavailable",
      }));
    } else {
      source = await readBoundedRegularFile(selected.path, maxBuffer, budget);
    }
    return { tracker: { ...tracker, status: "current", fingerprint: trackerFingerprint(selected, source) }, source, tasks };
  } catch (error) {
    const reason = error.killed || error.signal || ["ETIMEDOUT", "ABORT_ERR", "EVENT_DEADLINE"].includes(error.code) ? "timeout"
      : error.code === "ENOENT" ? (selected.kind === "beads" ? "missing_executable" : "missing_tracker")
        : error instanceof SyntaxError || error.code === "INVALID_RESPONSE" ? "invalid_response" : "backend_failed";
    return { tracker: { ...tracker, reason }, source: "", tasks: [] };
  }
}
