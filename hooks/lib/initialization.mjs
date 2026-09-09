import { execFile } from "node:child_process";
import { createHash, randomUUID } from "node:crypto";
import { link, lstat, mkdir, readFile, readdir, rename, rm, writeFile } from "node:fs/promises";
import path from "node:path";
import { promisify } from "node:util";
import { createEventBudget } from "./budget.mjs";
import { loadCanonicalState, loadCanonicalTracker, validateOperationMappings } from "./canonical-state.mjs";
import { withDirectoryLock } from "./lock.mjs";
import { resolveProject } from "./project.mjs";
import { captureWriterIdentity, inspectWriterIdentity, taskEligibility } from "./task-transitions.mjs";
import { resolveTracker, trackerFingerprint as fingerprintTracker } from "./tracker.mjs";

const run = promisify(execFile);
const hash = (source) => createHash("sha256").update(source).digest("hex");
const stable = (value) => JSON.stringify(value && typeof value === "object"
  ? Array.isArray(value) ? value.map((entry) => JSON.parse(stable(entry))) : Object.fromEntries(Object.keys(value).sort().map((key) => [key, JSON.parse(stable(value[key]))])) : value);
const json = (value) => `${JSON.stringify(value, null, 2)}\n`;
const validId = (value) => typeof value === "string" && /^[\w.:-]{1,128}$/.test(value) && !["none", "unknown", "unassigned", "-"].includes(value.toLowerCase());
const strings = (value) => Array.isArray(value) && value.length > 0 && value.length <= 100 && value.every((item) => typeof item === "string" && item.trim() && item.length <= 4096);
const decision = (status, reason, extra = {}) => ({ status, ready: false, reason, ...extra });
const relative = (value) => typeof value === "string" && value && value.length <= 256 && !path.isAbsolute(value) && !value.split(/[\\/]/).includes("..") && !/[\r\n\0|]/.test(value);
const trackerSelection = (tracker) => tracker?.kind === "beads"
  ? { kind: "beads", executable: tracker.executable ?? "bd" }
  : { kind: tracker?.kind, path: tracker?.path };
function completeState(state, { taskIds, integrationOwner, branch }) {
  const booleanFields = (record, fields) => record && fields.every((field) => typeof record[field] === "boolean");
  if (state?.schemaVersion !== 1 || !Number.isSafeInteger(state.stateVersion) || state.stateVersion < 0
    || !["finite", "continuous"].includes(state.run?.mode) || typeof state.run.paused !== "boolean"
    || !Array.isArray(state.run.taskIds) || new Set(state.run.taskIds).size !== state.run.taskIds.length
    || state.run.taskIds.some((id) => !validId(id) || !taskIds.includes(id))
    || (state.stateVersion === 0 && state.run.taskIds.length !== taskIds.length)
    || !booleanFields(state.integration, ["authorized", "paused", "hold"])
    || !validId(integrationOwner) || state.integration.ownerSessionId !== integrationOwner || state.integration.baseRef !== branch
    || !booleanFields(state.release, ["authorized", "autoDeploy", "hold"]) || state.release.ownerSessionId !== integrationOwner
    || !booleanFields(state.completion, ["requirementsReconciled"]) || !Array.isArray(state.completion.checks)
    || state.completion.checks.some((check) => !check || typeof check.name !== "string" || !check.name.trim()
      || !["pending", "passed", "failed", "skipped", "blocked", "unavailable"].includes(check.status)
      || (check.status === "passed" && (typeof check.revision !== "string" || !check.revision)))) return false;
  if (!Object.hasOwn(state, "operationMappings")) return false;
  try { validateOperationMappings(state.operationMappings); return true; } catch { return false; }
}

function registryIdentity(source) {
  const field = (label) => {
    const matches = source?.match(new RegExp(`^${label}:[ \\t]*([^\\r\\n]+)$`, "gm")) ?? [];
    return matches.length === 1 ? matches[0].slice(label.length + 1).trim() : undefined;
  };
  return { projectId: field("Project"), projectOwner: field("Project owner"), integrationOwner: field("Integration owner") };
}

function validateRequest(request) {
  if (!request || Buffer.byteLength(JSON.stringify(request)) > 32768) return "invalid_request";
  if (![request.projectId, request.ownerSessionId, request.operationId].every(validId)) return "explicit_setup_identity_required";
  if (!["standalone", "existing"].includes(request.source)) return "explicit_source_required";
  if (!request.tracker || !["markdown", "beads"].includes(request.tracker.kind)) return "explicit_tracker_required";
  const plan = request.plan;
  if (!plan || typeof plan.scope !== "string" || !plan.scope.trim() || plan.scope.length > 4096 || !strings(plan.acceptance)
    || !strings(plan.verification) || typeof plan.branch !== "string" || !plan.branch || plan.branch.length > 256
    || !plan.authority || !strings(plan.authority.ownedPaths) || !plan.authority.ownedPaths.every(relative)) return "required_plan_facts_missing";
  if (request.source === "standalone" && (!Array.isArray(plan.tasks) || !plan.tasks.length)) return "actionable_tasks_missing";
  if (plan.tasks !== undefined && (!Array.isArray(plan.tasks) || plan.tasks.length > 100 || new Set(plan.tasks.map((task) => task?.id)).size !== plan.tasks.length
    || plan.tasks.some((task) => !validId(task?.id)))) return "invalid_task_identity";
  return undefined;
}

/** Structural readiness only; neither a receipt nor a caller identity proves native host trust. */
export function initializationRecordProblem(setup, canonical, { projectRoot, validateTracker = false } = {}) {
  const receipt = setup?.initialization;
  const ids = setup?.plan?.taskIds;
  const owner = canonical?.registry?.projectOwner;
  if (setup?.schemaVersion !== 1 || setup.skill !== "agent-team" || !Number.isSafeInteger(setup.version) || setup.version < 1
    || receipt?.status !== "complete" || !validId(receipt.operationId) || !/^[a-f0-9]{64}$/.test(receipt.signature ?? "")
    || !["standalone", "existing"].includes(receipt.source) || !Array.isArray(ids) || new Set(ids).size !== ids.length
    || validateRequest({ projectId: setup.projectId, ownerSessionId: owner, operationId: receipt.operationId,
      source: receipt.source, tracker: setup.tracker, plan: { ...setup.plan, tasks: ids?.map((id) => ({ id })) } })) return "invalid_initialization_receipt";
  if (canonical.registry.projectId !== setup.projectId) return "existing_owner_conflict";
  if (typeof projectRoot !== "string" || !path.isAbsolute(projectRoot)) return "invalid_tracker_selection";
  const selected = resolveTracker(projectRoot, setup.tracker);
  if (selected.reason || !canonical.tracker || canonical.tracker.reason === "invalid_selection"
    || Object.entries(selected).some(([key, value]) => canonical.tracker[key] !== value)) return "invalid_tracker_selection";
  if (!receipt.trackerSelection || stable(receipt.trackerSelection) !== stable(trackerSelection(setup.tracker))) return "initialization_tracker_changed";
  if (!completeState(canonical.state, { taskIds: ids, integrationOwner: canonical.registry.integrationOwner, branch: setup.plan.branch })) return "required_state_facts_missing";
  if (validateTracker) {
    if (canonical.tracker?.status !== "current") return "tracker_unavailable";
    const actual = canonical.tasks.map(({ id }) => id);
    if (ids.length !== actual.length || ids.some((id) => !actual.includes(id))) return "existing_task_identity_conflict";
  }
  return undefined;
}

function taskTable(tasks) {
  const ids = tasks.map(({ id }) => id);
  if (tasks.some((task) => typeof task.title !== "string" || !task.title.trim() || /[\r\n|]/.test(task.title)
    || !["ready", "blocked", "todo"].includes(task.status) || (task.dependencies ?? []).some((id) => !ids.includes(id)))) throw new Error("invalid_standalone_tasks");
  return "# Agent-Team Tasks\n\n| ID | Requirement / acceptance | Owner | Depends on | Status | Revision / evidence | Next action |\n| --- | --- | --- | --- | --- | --- | --- |\n"
    + tasks.map((task) => `| ${task.id} | ${[task.title, ...(task.acceptance ?? [])].join("; ").replace(/[\r\n|]/g, " ")} | none | ${(task.dependencies ?? []).join(", ") || "none"} | ${task.status} | none | Follow the approved plan. |\n`).join("");
}

/** Explicit setup only: canonical owner, approved plan, no implicit hook/prompt authorization. */
export async function initializeProject(projectPath, request, options = {}) {
  const invalid = validateRequest(request);
  if (invalid) return decision("conflict", invalid);
  const budget = options.budget ?? createEventBudget(5000);
  const bounded = (action) => budget.run(action);
  const io = { link, rename, ...options.filesystem };
  const git = async (cwd, args) => (await run("git", args, { cwd, encoding: "utf8", timeout: budget.timeout(1000), signal: budget.signal, maxBuffer: 16384 })).stdout.trim();
  const read = async (file) => {
    let stat;
    try { stat = await bounded(() => lstat(file)); } catch (error) { if (error.code === "ENOENT") return undefined; throw error; }
    if (!stat.isFile() || stat.isSymbolicLink() || stat.size > 1024 * 1024) throw new Error("unsafe_or_oversized_record");
    return bounded(() => readFile(file, { encoding: "utf8", signal: budget.signal }));
  };
  const directory = async (file) => {
    try { const stat = await bounded(() => lstat(file)); if (!stat.isDirectory() || stat.isSymbolicLink()) throw new Error("unsafe_record_directory"); }
    catch (error) {
      if (error.code !== "ENOENT") throw error;
      budget.check();
      try { await bounded(() => mkdir(file, { mode: 0o700 })); } catch (created) { if (created.code !== "EEXIST") throw created; }
      const stat = await bounded(() => lstat(file));
      if (!stat.isDirectory() || stat.isSymbolicLink()) throw new Error("unsafe_record_directory");
    }
  };
  const publish = async (file, source, replace = false) => {
    const temporary = `${file}.${process.pid}.${randomUUID()}.tmp`;
    try {
      budget.check();
      await bounded(() => writeFile(temporary, source, { flag: "wx", mode: 0o600, signal: budget.signal }));
      budget.check();
      await bounded(() => replace ? io.rename(temporary, file) : io.link(temporary, file));
    } finally { await rm(temporary, { force: true }); }
  };
  try {
    return await bounded(async () => {
      const found = await resolveProject(projectPath, { budget });
      if (!found.commonDirectory || !["setup_missing", "active"].includes(found.reason)) return decision("unavailable", found.reason);
      const root = found.root;
      const stateRoot = path.join(root, ".agent-team");
      await directory(stateRoot);
      const locks = path.join(stateRoot, ".locks");
      await directory(locks);
      const lockPath = path.join(locks, "setup.lock");
      const writer = await captureWriterIdentity().catch(() => undefined);
      // Serialize recovery separately; never infer orphanhood from a timestamp or PID alone.
      await withDirectoryLock(path.join(locks, "setup-recovery.lock"), { kind: "initialization_lock_recovery", pid: process.pid, writer }, async () => {
        for (const recoverPath of [lockPath, path.join(locks, "state.lock")]) {
        let stat;
        try { stat = await bounded(() => lstat(recoverPath)); } catch (error) { if (error.code === "ENOENT") continue; throw error; }
        if (!stat.isDirectory() || stat.isSymbolicLink()) throw new Error("unsafe_setup_lock");
        const ownerPath = path.join(recoverPath, "owner.json");
        const ownerSource = await read(ownerPath);
        if (!ownerSource) continue;
        const owner = JSON.parse(ownerSource);
        if (owner.kind !== "project_initialization" || (await bounded(() => inspectWriterIdentity(owner.writer))).status !== "stopped") continue;
        const entries = await bounded(() => readdir(recoverPath));
        if (entries.length !== 1 || entries[0] !== "owner.json" || await read(ownerPath) !== ownerSource) continue;
        budget.check();
        await rm(recoverPath, { recursive: true });
        }
      }, { budget });
      return withDirectoryLock(lockPath, { kind: "project_initialization", projectId: request.projectId, ownerSessionId: request.ownerSessionId, operationId: request.operationId, pid: process.pid, writer }, async () => {
        return withDirectoryLock(path.join(locks, "state.lock"), { kind: "project_initialization", operationId: request.operationId, pid: process.pid, writer }, async () => {
        const current = await resolveProject(projectPath, { budget });
        if (current.root !== root || current.commonDirectory !== found.commonDirectory) return decision("conflict", "project_identity_changed");
        const tracker = resolveTracker(root, request.tracker);
        if (tracker.reason) return decision("conflict", "invalid_tracker_selection");
        const paths = { stateRoot, setup: path.join(stateRoot, "setup.json"), teams: path.join(stateRoot, "TEAMS.md"), state: path.join(stateRoot, "state.json"),
          tasks: tracker.path, locks, operationMappings: path.join(stateRoot, "operation-mappings.json") };
        const project = { ...current, active: true, projectId: request.projectId, setup: { tracker: request.tracker }, tracker, paths };
        const { expectedVersion, ...semanticRequest } = request;
        const signature = hash(stable(semanticRequest));
        const setupSource = await read(paths.setup);
        const setup = setupSource ? JSON.parse(setupSource) : undefined;
        const teamsSource = await read(paths.teams);
        const identity = registryIdentity(teamsSource);
        const { projectOwner: recordedOwner, projectId: recordedProject } = identity;
        if ((setup || teamsSource !== undefined) && (!validId(recordedOwner) || recordedProject !== request.projectId)) return decision("conflict", "existing_owner_unavailable");
        if (recordedOwner && recordedOwner !== request.ownerSessionId) return decision("conflict", "existing_owner_conflict");
        if (setup && (setup.skill !== "agent-team" || setup.projectId !== request.projectId)) return decision("conflict", "existing_project_conflict");
        if (setup && stable(resolveTracker(root, setup.tracker)) !== stable(tracker)) return decision("conflict", "existing_tracker_conflict");
        const priorBranch = setup?.plan?.branch ?? setup?.branch;
        if (priorBranch && priorBranch !== request.plan.branch) return decision("conflict", "existing_branch_conflict");
        if (setup?.plan && ["scope", "acceptance", "verification", "authority"].some((field) => setup.plan[field] !== undefined && stable(setup.plan[field]) !== stable(request.plan[field]))) return decision("conflict", "existing_plan_conflict");
        await git(root, ["check-ref-format", "--branch", request.plan.branch]);
        try { await git(root, ["show-ref", "--verify", `refs/heads/${request.plan.branch}`]); }
        catch { if (await git(root, ["symbolic-ref", "--quiet", "--short", "HEAD"]) !== request.plan.branch) return decision("conflict", "integration_branch_unavailable"); }
        const finish = async (status) => {
          for (const file of [paths.setup, paths.teams, paths.state]) if (await read(file) === undefined) return decision("unavailable", "required_records_missing");
          const committed = JSON.parse(await read(paths.setup));
          const canonical = await loadCanonicalState({ ...project, setup: committed }, { ...options, budget });
          const problem = initializationRecordProblem(committed, canonical, { projectRoot: root, validateTracker: true });
          if (problem) return decision("unavailable", problem);
          if (canonical.tracker.status !== "current") return decision("unavailable", "tracker_unavailable", { tracker: canonical.tracker });
          const observedIdentity = registryIdentity(await read(paths.teams));
          if (observedIdentity.projectOwner !== request.ownerSessionId || observedIdentity.projectId !== request.projectId) return decision("conflict", "existing_owner_conflict");
          const taskIds = canonical.tasks.map(({ id }) => id);
          const eligible = taskEligibility(canonical, { scopeTaskIds: canonical.state.run.taskIds }).eligible;
          return { status, ready: eligible.length > 0, ...(eligible.length ? {} : { reason: "no_eligible_task" }), projectRoot: root,
            taskIds, tracker: canonical.tracker, version: committed.version, eligibleTaskIds: eligible.map(({ id }) => id) };
        };
        if (setup?.initialization) {
          if (setup.initialization.operationId !== request.operationId || setup.initialization.signature !== signature) return decision("conflict", "initialization_identity_conflict");
          return finish("duplicate");
        }
        if (setup && (!Number.isInteger(expectedVersion) || expectedVersion !== (setup.version ?? 0))) return decision("conflict", "stale_setup_version");
        const journalPath = path.join(stateRoot, ".setup-initialization.json");
        const journalSource = await read(journalPath);
        let journal = journalSource ? JSON.parse(journalSource) : undefined;
        if (journal && (journal.signature !== signature || journal.operationId !== request.operationId || journal.ownerSessionId !== request.ownerSessionId)) return decision("conflict", "initialization_in_progress");
        if (journal && journal.setupFingerprint !== (setupSource === undefined ? null : hash(setupSource))) return decision("conflict", "setup_changed_during_initialization");
        if (!journal && request.source === "standalone") {
          for (const candidate of [path.join(root, "TASKS.md"), path.join(stateRoot, "TASKS.md"), path.join(root, ".beads")]) {
            try { await bounded(() => lstat(candidate)); return decision("conflict", "existing_tracker_requires_adoption"); }
            catch (error) { if (error.code !== "ENOENT") throw error; }
          }
        }
        if (request.source === "standalone" && tracker.kind !== "markdown") return decision("unavailable", "tracker_initialization_required");
        const created = new Map();
        // Fingerprint the exact bytes whose authority was validated, not a later read.
        const preserved = new Map([[paths.teams, teamsSource], [paths.state, await read(paths.state)], [paths.operationMappings, await read(paths.operationMappings)]]);
        let tasks;
        let trackerFingerprint = null;
        if (request.source === "standalone") {
          created.set(paths.tasks, taskTable(request.plan.tasks));
          tasks = request.plan.tasks;
        } else {
          if (tracker.kind === "markdown") {
            const source = await read(paths.tasks);
            if (source === undefined) return decision("unavailable", "tracker_unavailable");
            preserved.set(paths.tasks, source);
          }
          const selected = await loadCanonicalTracker(project, { ...options, budget });
          if (selected.tracker.status !== "current") return decision("unavailable", "tracker_unavailable", { tracker: selected.tracker });
          trackerFingerprint = selected.tracker.fingerprint;
          if (tracker.kind === "markdown" && fingerprintTracker(tracker, preserved.get(paths.tasks)) !== trackerFingerprint) return decision("conflict", "tracker_changed_during_initialization");
          tasks = selected.tasks;
          if (request.plan.tasks && (request.plan.tasks.length !== tasks.length || request.plan.tasks.some(({ id }) => !tasks.some((task) => task.id === id)))) return decision("conflict", "existing_task_identity_conflict");
        }
        if (journal && journal.trackerFingerprint !== trackerFingerprint) return decision("conflict", "tracker_changed_during_initialization");
        const taskIds = tasks.map(({ id }) => id);
        const teams = `# Agent-Team teams\nProject: ${request.projectId}\nProject owner: ${request.ownerSessionId}\nIntegration owner: ${request.ownerSessionId}\n\n| Team ID | Name | Session | Worktree | Branch | Owned paths | Tasks | Status |\n| --- | --- | --- | --- | --- | --- | --- | --- |\n`;
        const initialState = { schemaVersion: 1, stateVersion: 0, run: { mode: "finite", taskIds, paused: false },
          integration: { ownerSessionId: request.ownerSessionId, authorized: false, baseRef: request.plan.branch, paused: false, hold: false },
          release: { ownerSessionId: request.ownerSessionId, authorized: false, autoDeploy: false, hold: true },
          completion: { requirementsReconciled: false, checks: [] }, operationMappings: { providers: {}, shell: [] } };
        for (const [file, source] of [[paths.teams, teams], [paths.state, json(initialState)]]) {
          if (journal?.createPaths.includes(file) || preserved.get(file) === undefined) created.set(file, source);
        }
        const state = JSON.parse(created.get(paths.state) ?? preserved.get(paths.state));
        const integrationOwner = created.has(paths.teams) ? request.ownerSessionId : identity.integrationOwner;
        if (!completeState(state, { taskIds, integrationOwner, branch: request.plan.branch })) return decision("unavailable", "required_state_facts_missing");
        const mappings = validateOperationMappings(state.operationMappings);
        if (journal?.createPaths.includes(paths.operationMappings) || preserved.get(paths.operationMappings) === undefined) created.set(paths.operationMappings,
          json({ schemaVersion: 1, kind: "agent-team-operation-mapping-cache", authoritative: false, projectId: request.projectId, sourcePath: ".agent-team/state.json", operationMappings: mappings }));
        const records = {};
        for (const file of [...new Set([...created.keys(), paths.teams, paths.state, paths.operationMappings, ...(tracker.kind === "markdown" ? [paths.tasks] : [])])]) {
          records[file] = hash(created.get(file) ?? preserved.get(file));
        }
        if (!journal) {
          journal = { schemaVersion: 1, kind: "project_initialization", projectId: request.projectId, ownerSessionId: request.ownerSessionId,
            operationId: request.operationId, signature, setupFingerprint: setupSource === undefined ? null : hash(setupSource), trackerFingerprint, createPaths: [...created.keys()], records };
          await publish(journalPath, json(journal));
        } else if (stable(journal.records) !== stable(records)) return decision("conflict", "initialization_records_changed");
        for (const [file, source] of created) {
          const existing = await read(file);
          if (existing !== undefined && hash(existing) !== hash(source)) return decision("conflict", "initialization_record_conflict");
          if (existing === undefined) await publish(file, source);
        }
        if (request.source === "existing") {
          const latest = await loadCanonicalTracker(project, { ...options, budget });
          if (latest.tracker.status !== "current") return decision("unavailable", "tracker_unavailable", { tracker: latest.tracker });
          if (latest.tracker.fingerprint !== trackerFingerprint) return decision("conflict", "tracker_changed_during_initialization");
        }
        for (const [file, fingerprint] of Object.entries(journal.records)) if (hash(await read(file) ?? "") !== fingerprint) return decision("conflict", "initialization_records_changed");
        if ((await read(paths.setup)) !== setupSource) return decision("conflict", "setup_changed_during_initialization");
        const { tasks: _tasks, ...plan } = request.plan;
        const completed = { ...setup, schemaVersion: 1, version: (setup?.version ?? 0) + 1, skill: "agent-team", projectId: request.projectId,
          tracker: request.tracker, plan: { ...setup?.plan, ...plan, taskIds }, initialization: { status: "complete", operationId: request.operationId, signature, source: request.source, trackerSelection: trackerSelection(request.tracker) } };
        await publish(paths.setup, json(completed), setupSource !== undefined);
        budget.check();
        await rm(journalPath, { force: true });
        return finish("applied");
        }, { budget });
      }, { budget });
    });
  } catch (error) { return decision("unavailable", error.code === "EVENT_DEADLINE" || budget.signal.aborted ? "deadline" : "initialization_unavailable"); }
  finally { if (!options.budget) budget.close(); }
}
