# Project state

## Authorities

| Concern | Authority | Task 1 handling |
| --- | --- | --- |
| Tasks, dependencies, acceptance, and task status | Beads | Read a bounded ready page; do not mirror it. |
| Source and integrated revisions | Git | Inspect the current worktree; do not create a worktree or commit on first run. |
| Decisions | `DECISIONS.md`, when present | Read relevant stable `D-` IDs; append resolutions without rewriting earlier entries. |
| Confirmed mistakes | `MISTAKES.md`, when present | Reuse relevant stable `M-` IDs; never rewrite earlier entries. |
| Unresolved questions | `BLOCKERS.md`, when present | Read only unresolved entries; raise them at status and integration milestones. |
| Session handoff | `.agent-team/SESSION.md`, when present | An active orchestrator refreshes this short breadcrumb; it is never task or worker authority. |
| Project Kickoff handoff | Optional one-time input, when explicitly supplied | A Beads-selected handoff is verified by ID and adopted; a `TASKS.md`-selected handoff may be explicitly imported once. It never gates setup, dispatch, review, or integration. |
| `.agent-team/SETTINGS.md` | Explicit user preferences only | Host-role model/effort preferences and limits; never tasks, task status, or readiness. |
| `.agent-team/dashboard/index.html` | Non-authoritative local snapshot | Best-effort post-integration display generated from Beads; never a tracker or closure gate. |
| Deployment batch authorization | Project decision/approval record, when present | Later orchestration requires an explicit command, approval, completed-task scope, and verification rule. |

`TASKS.md` and Project Kickoff handoffs are optional one-time inputs to Beads, not live trackers. Do not inspect, require, or wait for them on ordinary setup, status, or one-off work; do not import either during first run.

## Beads operations

Use the project directory as the command working directory.

| Intent | Command | Write rule |
| --- | --- | --- |
| Inspect readiness/status | `bd --readonly status --json` | No Agent-Team write, task/database mutation, or Git change; a cold Beads 1.2.2 open may create only `.beads/embeddeddolt/<project>/.dolt/temptf/dolt_embedded_metrics`. |
| Read candidates | `bd ready --limit 20 --json` | Read-only; pass no more than 20 rows onward. |
| Initialize an approved project | `bd init --skip-hooks --skip-agents --non-interactive --init-if-missing` | Only after the explicit first-run approval. |
| Claim the active selected task | `bd update ID --claim` | Reserved for the later native-dispatch loop; claim only the next task actually starting. |
| Return a guaranteed no-launch task | `bd update ID --status open --assignee ''` | Orchestrator only; clear the stale claim, then attach the actual rejection and retry condition in a Beads comment. |
| Park repeated unresolved review work | `bd update ID --status blocked` | Orchestrator only; add the matching evidence/next-action comment below and leave unrelated ready work alone. |
| Park a temporary NO-GO | `bd update ID --status blocked` | Orchestrator only; retain its actual handle and claim. Restore `in_progress` only after the same observable handle is observed beginning work; never infer CLEAN or closure. |
| Record a task-local blocker | `bd comments add ID "<revision, observed result, reconciliation action>"` | Orchestrator only; use for repeated unresolved findings, uncertain host/review, or failed integration. |
| Close integrated CLEAN work | `bd close ID --reason "CLEAN <reviewed revision>; integrated <revision>"` | Orchestrator only, after valid exact-revision CLEAN, scope/check revalidation, integration, and integration commit. Never use `--force` to bypass a failed gate. |
| Verify a Beads-selected handoff ID | `bd show ID --json` | Read-only; verify each supplied ID before adopting that existing database. |
| Preview an approved Markdown migration | `bd import --dry-run --json tasks-import.jsonl` | Temporary export-compatible JSONL only; compare proposed IDs/count/dependencies to source before a second approval. |
| Import an approved Markdown migration | `bd import --json tasks-import.jsonl` | One-time source adoption. Omit all existing IDs and never pass `--allow-stale`, preserving newer Beads edits. |

If Beads is absent during `status`, say so and stop. If it is absent during `setup` or `start`, ask once for initialization approval. A declined, cancelled, or unanswered approval leaves the project unchanged.

## One-time imports and settings

Ask before reading a handoff or `TASKS.md` for migration, and again after the dry-run comparison before the real import. For a Project Kickoff handoff with Beads selected, `plan.tasks` is an ID list: every ID must pass `bd show ID --json`; no JSONL is produced. For Markdown rows, create a temporary export-compatible JSONL with stable IDs, title/objective, acceptance criteria, status, dependency objects, and a source pointer. Reject unrepresentable dependencies before any Beads write. Preserve non-task run history in the original, read-only source.

Beads 1.2.2 imports are upserts and protect a local issue when its `updated_at` is newer, but this workflow additionally omits every existing ID from its candidate. Do not use `--allow-stale`; do not overwrite or duplicate an existing issue. After the actual import, retain source-pointer evidence and discard the temporary candidate. The original Markdown file may be archived as provenance only at the user's request; neither original nor archive becomes live state.

On an explicit preference save, `.agent-team/SETTINGS.md` starts with these values (and only actual host model/effort choices replace `inherit`):

```md
# Agent-Team settings

max_teams: 2
dev_server_limit: 2
deployment: disabled until an approved completed-task batch command, approval, and verification rule are recorded

host: <actual host>
orchestrator: inherit
developer: inherit
reviewer: inherit
visual: inherit
```

Enumerate models and effort values from the active host metadata, never from a presumed catalog. Preferences can be overridden later per role or with a cheaper available per-task route. The file neither stores tasks nor changes task readiness.

## Dashboard snapshot

The packaged `assets/dashboard.html` is copied to `.agent-team/dashboard/index.html` only after accepted integration has already closed the relevant Beads issue. Generate its aggregate status values from Beads without passing task rows to a model; its ready list is the same `bd ready --limit 20 --json` page, so 1,000 on-disk issues do not enter prompt context. A failure to copy or refresh the snapshot is reported separately and has no effect on integration evidence or Beads closure.

## First-run mutation budget

The approved Task 1 mutation is the `bd init` command above and Beads-created files or Beads-owned additions to an existing `.gitignore`. A cold readonly status may additionally create only its exact Beads/Dolt metrics path listed above; it does not authorize an Agent-Team file, a task/database mutation, a Git change, or any other project file. Optional-tool availability never changes this budget or Beads readiness.

## Records used by later revisions

When they exist, keep records narrow: decisions carry stable `D-` IDs, date, scope, decision, rationale, and supersession; mistakes carry stable `M-` IDs, failure mode, cause, remedy, and evidence; blockers contain unresolved questions only. A blocker entry is exactly `ID`, `Task`, `Question`, `Recommendation`, `Impact`, and `Next prompt`. Raise unresolved entries at status and integration milestones. Record a resolution in `DECISIONS.md` before removing a resolved blocker; append new entries and never rewrite an earlier `D-` or `M-` entry. A session checkpoint is a breadcrumb, not worker or task authority; Beads and Git remain authoritative.

## Session breadcrumb

On an explicit pause, write or refresh `.agent-team/SESSION.md` as a short operational pointer, not a transcript or duplicate task definition. Include host and time; active Beads IDs; each worktree, branch, revision, and uncommitted-work summary; evidence pointers; last observed native handles and uncertainty; pending operations or approvals; unresolved blockers; and the next action. For example:

```markdown
# Agent-Team session handoff
Host: codex; time: 2026-09-25T12:00:00-05:00
Beads task: atv-demo-1 (blocked; native handle uncertain)
Worktree: .worktrees/atv-demo-1; branch: work/atv-demo-1
Git: uncommitted src/example.go; inspect before any reassignment
Evidence: Beads comment on atv-demo-1; test output in worktree
Pending approval: none
Next action: inspect original host handle; continue unrelated bd ready work
```

On resume, inspect Beads and Git before dispatching. The breadcrumb cannot establish that a historic handle still exists; retain uncertainty until the active host actually observes it. A temporary NO-GO remains blocked even when that handle is observable; restore `in_progress` only after its same-handle follow-up actually begins work.

## Review, blocker, cleanup, and deployment evidence

Attach each review report to its Beads task and exact full revision. A `FIX` remains task-local: retain the developer worktree for same-worktree remediation and require a new independent report for the new revision. On repeated unresolved findings, uncertain host/reviewer state, or failed integration, use the blocked update and comment above to record the revision, observed result, prior attempts, and next reconciliation action. Do not block, close, or rewrite unrelated tasks.

Only the orchestrator integrates, makes the integration commit, and closes a task. Its closure reason must identify the reviewed revision and integrated revision. It may remove a task worktree and delete its branch only after recording that the worktree is clean and either its branch is merged or verified patch-equivalence identifies the exact task scope and exact integrated revision (for example, an approved cherry-pick). Retain dirty, unknown, unverified, failed, and ancestry-unmerged worktrees/branches unless they have that recorded verification. After removing an exact verified clean worktree without force, try normal `git branch -d <exact-verified-branch>`; only if it then refuses because that branch is ancestry-unmerged use `git branch -D <exact-verified-branch>`. Never force-remove any other branch or worktree.

Deployment has no default command. Before running one, the project must have a recorded explicit completed-task batch command, approval, and verification rule; retain its command output and result with the batch evidence. Track each on-demand dev server's project and task ownership, permit at most two per project, never share a server across projects, and treat a cap or server failure as a task-local blocker.
