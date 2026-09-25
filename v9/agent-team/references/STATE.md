# Project state

## Authorities

| Concern | Authority | Task 1 handling |
| --- | --- | --- |
| Tasks, dependencies, acceptance, and task status | Beads | Read a bounded ready page; do not mirror it. |
| Source and integrated revisions | Git | Inspect the current worktree; do not create a worktree or commit on first run. |
| Decisions | `DECISIONS.md`, when present | Read only relevant IDs; Task 1 does not create it. |
| Confirmed mistakes | `MISTAKES.md`, when present | Read only relevant IDs; Task 1 does not create it. |
| Unresolved questions | `BLOCKERS.md`, when present | Read only relevant entries; Task 1 does not create it. |
| Session handoff | `.agent-team/SESSION.md`, when present | A future active-session revision maintains it; Task 1 does not create it. |
| Project Kickoff handoff | Optional one-time input, when present | It may inform Beads import; it never gates setup, dispatch, review, or integration. |
| Deployment batch authorization | Project decision/approval record, when present | Later orchestration requires an explicit command, approval, completed-task scope, and verification rule. |

`TASKS.md` and Project Kickoff handoffs are one-time inputs to Beads, not live trackers. Do not import either during Task 1 first run.

## Beads operations

Use the project directory as the command working directory.

| Intent | Command | Write rule |
| --- | --- | --- |
| Inspect readiness/status | `bd --readonly status --json` | No Agent-Team write, task/database mutation, or Git change; a cold Beads 1.2.2 open may create only `.beads/embeddeddolt/<project>/.dolt/temptf/dolt_embedded_metrics`. |
| Read candidates | `bd ready --limit 20 --json` | Read-only; pass no more than 20 rows onward. |
| Initialize an approved project | `bd init --skip-hooks --skip-agents --non-interactive --init-if-missing` | Only after the explicit first-run approval. |
| Claim the active selected task | `bd update ID --claim` | Reserved for the later native-dispatch loop; claim only the next task actually starting. |
| Park repeated unresolved review work | `bd update ID --status blocked` | Orchestrator only; add the matching evidence/next-action comment below and leave unrelated ready work alone. |
| Record a task-local blocker | `bd comments add ID "<revision, observed result, reconciliation action>"` | Orchestrator only; use for repeated unresolved findings, uncertain host/review, or failed integration. |
| Close integrated CLEAN work | `bd close ID --reason "CLEAN <reviewed revision>; integrated <revision>"` | Orchestrator only, after valid exact-revision CLEAN, scope/check revalidation, integration, and integration commit. Never use `--force` to bypass a failed gate. |

If Beads is absent during `status`, say so and stop. If it is absent during `setup` or `start`, ask once for initialization approval. A declined, cancelled, or unanswered approval leaves the project unchanged.

## First-run mutation budget

The approved Task 1 mutation is the `bd init` command above and Beads-created files or Beads-owned additions to an existing `.gitignore`. A cold readonly status may additionally create only its exact Beads/Dolt metrics path listed above; it does not authorize an Agent-Team file, a task/database mutation, a Git change, or any other project file. Optional-tool availability never changes this budget or Beads readiness.

## Records used by later revisions

When they exist, keep records narrow: decisions carry stable IDs, date, scope, decision, rationale, and supersession; mistakes carry stable IDs, failure mode, cause, remedy, and evidence; blockers contain unresolved questions only. Record a resolution in `DECISIONS.md` before removing a resolved blocker. A future session checkpoint is a breadcrumb, not worker or task authority; Beads and Git remain authoritative.

## Review, blocker, cleanup, and deployment evidence

Attach each review report to its Beads task and exact full revision. A `FIX` remains task-local: retain the developer worktree for same-worktree remediation and require a new independent report for the new revision. On repeated unresolved findings, uncertain host/reviewer state, or failed integration, use the blocked update and comment above to record the revision, observed result, prior attempts, and next reconciliation action. Do not block, close, or rewrite unrelated tasks.

Only the orchestrator integrates, makes the integration commit, and closes a task. Its closure reason must identify the reviewed revision and integrated revision. It may remove a task worktree and delete its branch only after recording that the worktree is clean and either its branch is merged or verified patch-equivalence identifies the exact task scope and exact integrated revision (for example, an approved cherry-pick); retain dirty, unverified, unmerged, unknown, or failed worktrees/branches without force removal.

Deployment has no default command. Before running one, the project must have a recorded explicit completed-task batch command, approval, and verification rule; retain its command output and result with the batch evidence. Track each on-demand dev server's project and task ownership, permit at most two per project, never share a server across projects, and treat a cap or server failure as a task-local blocker.
