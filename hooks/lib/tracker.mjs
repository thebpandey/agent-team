import { execFile } from "node:child_process";
import { createHash } from "node:crypto";
import { readFile } from "node:fs/promises";
import path from "node:path";
import { promisify } from "node:util";

const run = promisify(execFile);
const BEADS_READ_TIMEOUT_MS = 5000;

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
export async function readTracker(project, { runBeads = run, budget, environment } = {}) {
  const selected = project.tracker ?? resolveTracker(project.root, project.setup?.tracker);
  const tracker = { ...selected, status: "unavailable", fingerprint: null, observedAt: new Date().toISOString() };
  if (selected.reason) return { tracker, tasks: [], source: "" };
  try {
    let source;
    let tasks;
    if (selected.kind === "beads") {
      const result = await runBeads(selected.executable ?? "bd", ["list", "--all", "--limit", "0", "--json", "--readonly"], {
        cwd: project.root, encoding: "utf8", timeout: budget?.timeout(BEADS_READ_TIMEOUT_MS) ?? BEADS_READ_TIMEOUT_MS,
        maxBuffer: 1024 * 1024, ...(budget ? { signal: budget.signal } : {}),
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
      source = await readFile(selected.path, { encoding: "utf8", ...(budget ? { signal: budget.signal } : {}) });
      if (Buffer.byteLength(source) > 1024 * 1024) throw Object.assign(new Error("Tracker exceeds 1 MiB"), { code: "INVALID_RESPONSE" });
    }
    return { tracker: { ...tracker, status: "current", fingerprint: trackerFingerprint(selected, source) }, source, tasks };
  } catch (error) {
    const reason = error.killed || error.signal || ["ETIMEDOUT", "ABORT_ERR", "EVENT_DEADLINE"].includes(error.code) ? "timeout"
      : error.code === "ENOENT" ? (selected.kind === "beads" ? "missing_executable" : "missing_tracker")
        : error instanceof SyntaxError || error.code === "INVALID_RESPONSE" ? "invalid_response" : "backend_failed";
    return { tracker: { ...tracker, reason }, source: "", tasks: [] };
  }
}
