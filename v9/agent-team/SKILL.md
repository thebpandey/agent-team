---
name: agent-team
metadata:
  version: "9.0.0"
description: Use when coordinating an active Codex or Claude development session with Beads: inspect status, initialize Beads after explicit approval, or select a bounded page of ready work.
---

# Agent-Team v9

Use Beads for live task state and Git for source revisions. This is an active-session skill: it has no controller, daemon, hooks, second task database, or promise of work after the host session ends. Default to Ponytail's smallest-working-change discipline. Project Kickoff is an optional one-time input when present, never a setup or handoff gate.

## Safety boundary

Treat `status` as inspection. It must not initialize Beads, create project records, install tools, repair configuration, dispatch workers, or write a checkpoint.

Do not make a project ready through optional tooling. Beads and Git are the only prerequisites here. Serena, Graphify, browsers, UI tools, and other aids are task-specific and never readiness gates. Do not probe, install, or invoke LeanCTX.

Native dispatch is available only through the active host's own Agent tooling. It has no controller, synthetic acknowledgement, shell-worker substitute, or cross-session worker promise. Follow [native host routing](references/HOSTS.md) for exact Codex and Claude differences; follow the [worker contract](references/WORKER_RULES.md) for developer and reviewer scope.

Fixture checks can validate Beads command semantics, but only a live host can prove native dispatch. Treat a live Codex or Claude invocation as a host-specific acceptance canary; never infer it from documentation or fixture output.

## Inspect the project

First confirm that the requested directory is a Git worktree:

```sh
git rev-parse --is-inside-work-tree
```

Then inspect Beads at the Agent-Team/project/task-authority boundary: make no Agent-Team write, task or database mutation, or Git change.

### `status`

If `.beads` is present, run exactly:

```sh
bd --readonly status --json
```

Report its result without an Agent-Team write, task/database mutation, or Git change. On its first open of an initialized Beads 1.2.2 project, Beads/Dolt may create only `.beads/embeddeddolt/<project>/.dolt/temptf/dolt_embedded_metrics`; report it as bounded Beads-owned housekeeping, not project setup. If `.beads` is absent, report that Beads has not been initialized and that no change was made; do not run `bd init` and do not turn the status request into setup.

### First run: `setup` or `start`

If `.beads` is absent, ask this one explicit approval question and wait:

> Initialize Beads in this Git project? This will run `bd init --skip-hooks --skip-agents --non-interactive --init-if-missing` and create only Beads-managed files.

On refusal, cancellation, or no answer, make no write and report that the project remains uninitialized. Only after an affirmative answer, run exactly:

```sh
bd init --skip-hooks --skip-agents --non-interactive --init-if-missing
bd --readonly status --json
```

Do not create `TASKS.md`, ledgers, settings, checkpoints, hooks, or optional-tool state as part of first run. See [project records](references/STATE.md) only to interpret records that already exist.

### Ready work

For `start` in an initialized project, read one bounded page only:

```sh
bd ready --limit 20 --json
```

Give the orchestrator at most those 20 task rows and choose only disjoint work. Never load or paste a full tracker dump. Native dispatch claims the selected next task, one at a time, with `bd update ID --claim`; do not claim speculative or later tasks.

## Native team dispatch

After `start` reads its bounded ready page, choose only disjoint work. Default to at most two parallel teams. Give each team at most four ordered tasks and claim only the current task with `bd update ID --claim` immediately before native dispatch; never claim a later speculative task.

Prepare a distinct Git worktree for each active task, pass bounded task context and applicable rules, and retain the actual native handle in this active session. On bounded waits, inspect the host result; if progress is absent after the agreed interval, request status through the same handle, record a stale/hung concern, and continue unrelated lanes. Refill a completed retained team with at most four more ordered tasks. An explicit no-launch rejection creates no handle or completion; an ambiguous launch is task-local uncertainty, not permission to invent a replacement.

Use [native host routing](references/HOSTS.md) for host operations and [the worker contract](references/WORKER_RULES.md) for assignment limits.

### Launch uncertainty and temporary holds

Keep active-session observation separate from cross-session uncertainty. An explicit native rejection that expressly guarantees no worker was created returns the just-claimed issue to open and clears its stale claim with `bd update ID --status open --assignee ''`, then adds an evidence comment containing the actual rejection and retry condition. It creates no handle, completion, or replacement worker.

A timeout, lost response, missing identity, possible launch, or ambiguous delivery blocks only that issue with a Beads comment naming the evidence and next observation. Do not claim, relaunch, or fabricate a worker for it until the original host observation permits a specific action. A later session has no proof of an earlier handle: inspect Beads and Git, preserve the uncertainty, and never reconstruct an identity from a task name or start a substitute worker. Independent ready issues may proceed.

When a host gives a temporary NO-GO, block only that Beads issue while retaining its actual handle and claim. If the original live handle is actually observable, send the returned delta only to that same handle through the host's native follow-up operation and record the response. Restore `in_progress` only after the blocker resolves and that same handle is observed actually beginning work. Never infer `CLEAN` or close the issue from a NO-GO or follow-up. If the handle cannot be observed, keep only this issue blocked; do not turn the NO-GO into a new launch.

### Pause and resume

On pause, the orchestrator stops new claims and assignments, asks every observable active worker to checkpoint and stop through its native host operation, and records only the resulting observation. An unsupported or unobservable control is task-local uncertainty, not a fabricated acknowledgement. Write the short session breadcrumb described in [STATE.md](references/STATE.md); it is not a worker registry or task authority.

On resume, first inspect Beads and Git for every recorded active task/worktree. Preserve dirty or uncertain work and reconcile original observable handles before any dispatch. Never silently overwrite, reassign, or relaunch it. Resume only an existing assignment when the native host actually confirms that capability; otherwise retain the task-local block and continue unrelated ready work.

## Review, remediation, and integration

Before any integration, the developer supplies the task's full candidate revision and observed acceptance/test evidence. Dispatch a real independent reviewer: the reviewer must be a non-author native handle/identity and must inspect the actual candidate diff, task acceptance criteria, and the supplied evidence. A reviewer reports only this revision-bound form:

```text
TASK: <Beads ID>
REVISION: <full candidate Git revision>
REVIEWER: <independent native handle or identity>
CHECKS: <command — PASS|FAIL|not run, with relevant observed result>
FINDINGS: <specific findings, or none>
VERDICT: FIX|CLEAN
```

An empty `FINDINGS` field alone is not `CLEAN`. `CLEAN` requires observed review of the actual diff and sufficient observed checks/evidence for the exact revision. A missing, ambiguous, author-performed, or revision-mismatched review is not `CLEAN` and cannot reach integration or closure.

On `FIX`, the developer remediates in the same task worktree, produces a new tested revision, and the independent reviewer rechecks that new revision. Repeated unresolved findings, uncertain review, or a task-specific host failure block only that Beads issue with the evidence and next reconciliation action; dispatch and integration of independent ready lanes continue. Never close blocked or failed work.

Only the orchestrator may accept a valid `CLEAN`. It rechecks that the reported task, reviewer, revision, write scope, and relevant checks match the candidate; integrates that revision in the main worktree; commits the integration; then closes the exact issue with `bd close ID --reason ...` naming the CLEAN review and integrated revision. If any check or integration step fails, retain the task and evidence without closure. The orchestrator may make a tiny surgical main-worktree fix only when it documents why that exception is necessary and obtains separate independent review of the resulting revision.

After a successful integration, remove a task worktree and branch only when its worktree is clean and either its branch is merged into the integrated revision or recorded verification proves its exact task scope is patch-equivalent to that exact integrated revision (for example, an approved cherry-pick). Preserve and report dirty, unknown, unverified, failed, and ancestry-unmerged branches unless they have that recorded verification. After removing an exact verified clean worktree without force, try normal `git branch -d <exact-verified-branch>`; only if it refuses because that branch is ancestry-unmerged may the orchestrator use `git branch -D <exact-verified-branch>`. No generic force cleanup is permitted.

Deployment is opt-in: run it only after the project records an explicit completed-task batch command, approval, and verification rule. Do not discover or invent a deployment command. Keep at most two on-demand dev servers per project, never share one across projects, and record their task/project ownership; a server limit or failure is task-local and does not halt independent lanes.

## State and honest reporting

Use [STATE.md](references/STATE.md) for Beads commands, review/blocker records, and the ownership of optional project records. Preserve existing project data. State what was observed, which command ran, whether a write was approved, and any Beads error. Never claim native dispatch, review, integration, deployment, or cross-session persistence unless that capability was actually implemented and observed in the active host session.

At status and integration milestones, raise every unresolved `BLOCKERS.md` entry to the orchestrator. `BLOCKERS.md` contains questions only, not host uncertainty or task state: each entry has `ID`, `Task`, `Question`, `Recommendation`, `Impact`, and `Next prompt`. Before removing a resolved blocker, append its resolution to `DECISIONS.md` with a new stable `D-` ID, date, scope, decision, rationale, and supersession. Keep prior `D-` entries unchanged. Reuse existing `M-` IDs when referring to known mistakes; append rather than rewrite any mistake entry.
