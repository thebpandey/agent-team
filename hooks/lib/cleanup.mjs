import { execFile } from "node:child_process";
import { promisify } from "node:util";
import { access, readFile, realpath } from "node:fs/promises";
import path from "node:path";
import { loadCanonicalTracker } from "./canonical-state.mjs";
import { inspectWriterIdentity, mutateOperationalState } from "./task-transitions.mjs";

const run = promisify(execFile);
const retained = (reason) => ({ status: "conflict", reason });
const within = (parent, child) => child === parent || child.startsWith(`${parent}${path.sep}`);

/** Remove only a proven disposable development checkout; deployment is independent. */
export async function cleanupDevelopmentWorktree(project, request, options = {}) {
  const { budget } = options;
  const bounded = (action) => budget ? budget.run(action) : action();
  const git = async (cwd, args) => {
    budget?.check();
    return (options.runGit ?? run)("git", args, { cwd, encoding: "utf8", timeout: budget?.timeout(1500) ?? 1500,
      maxBuffer: 1024 * 1024, ...(budget ? { signal: budget.signal } : {}) });
  };
  return mutateOperationalState(project, request, async (state, canonical, { persistIntent }) => {
    const gate = state.cleanup?.[request.taskId];
    const target = path.resolve(request.worktree ?? project.root);
    if (!gate || gate.worktree !== target || gate.revision !== request.expectedRevision) return retained("retain_identity_mismatch");
    if (gate.resourceOwner !== "agent-team" || gate.retain) return retained("retain_resource_owner");
    if (gate.previewRequired) return retained("retain_required_preview");
    if (target === project.root || within(target, project.root)) return retained("retain_canonical_checkout");
    if (JSON.stringify(gate.writer) !== JSON.stringify(request.expectedWriter)
      || (await bounded(() => inspectWriterIdentity(gate.writer))).status !== "stopped") return retained("retain_writer_not_stopped");
    if (!canonical.registry.teams.some((team) => path.resolve(project.root, team.worktree ?? "") === target)) return retained("retain_unregistered_worktree");
    const pending = state.pendingOperations?.[request.operationId];
    const exists = await bounded(() => access(target).then(() => true, (error) => error.code === "ENOENT" ? false : Promise.reject(error)));
    if (!exists && pending?.kind === "worktree_cleanup" && pending.worktree === target && pending.revision === gate.revision) {
      const registered = (await git(project.root, ["worktree", "list", "--porcelain"])).stdout.includes(`worktree ${target}\n`);
      if (registered) return { status: "unavailable", reason: "cleanup_uncertain" };
      delete state.pendingOperations[request.operationId];
      state.cleanup[request.taskId] = { ...gate, removed: true };
      return { state, result: { taskId: request.taskId, worktree: target, removed: true, reconciled: true } };
    }
    if (!exists || pending) return { status: "unavailable", reason: "cleanup_uncertain" };
    if (await bounded(() => realpath(target)) !== target) return retained("retain_symlink_worktree");
    const registered = (await git(project.root, ["worktree", "list", "--porcelain"])).stdout.includes(`worktree ${target}\n`);
    if (!registered) return retained("retain_unregistered_worktree");
    const tracker = await loadCanonicalTracker(project, options);
    if (tracker.tracker.status !== "current") return { status: "unavailable", reason: "tracker_unavailable" };
    if (!tracker.tasks.some((task) => task.id === request.taskId && ["verified", "integrated", "deployed", "closed", "done"].includes(task.status))) return retained("retain_unverified_task");
    if (gate.verification?.status !== "passed" || gate.verification.revision !== gate.revision || !gate.integrationRef) return retained("retain_unverified_revision");
    if ((await git(target, ["rev-parse", "HEAD"])).stdout.trim() !== gate.revision) return retained("retain_changed_revision");
    try { await git(project.root, ["merge-base", "--is-ancestor", gate.revision, gate.integrationRef]); }
    catch { return retained("retain_not_integrated"); }
    if ((await git(target, ["status", "--porcelain", "--untracked-files=all", "--ignored"])).stdout.trim()) return retained("retain_dirty_worktree");
    if (!gate.evidencePaths?.length) return retained("retain_evidence_unavailable");
    for (const pointer of gate.evidencePaths) {
      const file = await bounded(() => realpath(pointer));
      if (within(target, file)) return retained("retain_evidence_inside_worktree");
      const evidence = JSON.parse(await bounded(() => readFile(file, "utf8")));
      if (evidence.taskId !== request.taskId || evidence.revision !== gate.revision || evidence.status !== "passed") return retained("retain_evidence_unverified");
    }
    await persistIntent({ phase: "uncertain", kind: "worktree_cleanup", taskId: request.taskId, worktree: target, revision: gate.revision });
    try { await git(project.root, ["worktree", "remove", "--", target]); }
    catch { return { status: "unavailable", reason: "cleanup_uncertain" }; }
    delete state.pendingOperations[request.operationId];
    state.cleanup[request.taskId] = { ...gate, removed: true };
    return { state, result: { taskId: request.taskId, worktree: target, removed: true } };
  }, options);
}
