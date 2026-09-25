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

`TASKS.md` and Project Kickoff handoffs are one-time inputs to Beads, not live trackers. Do not import either during Task 1 first run.

## Beads operations

Use the project directory as the command working directory.

| Intent | Command | Write rule |
| --- | --- | --- |
| Inspect readiness/status | `bd status --json` | Read-only; never initialize as a side effect. |
| Read candidates | `bd ready --limit 20 --json` | Read-only; pass no more than 20 rows onward. |
| Initialize an approved project | `bd init --skip-hooks --skip-agents --non-interactive --init-if-missing` | Only after the explicit first-run approval. |
| Claim the active selected task | `bd update ID --claim` | Reserved for the later native-dispatch loop; claim only the next task actually starting. |

If Beads is absent during `status`, say so and stop. If it is absent during `setup` or `start`, ask once for initialization approval. A declined, cancelled, or unanswered approval leaves the project unchanged.

## First-run mutation budget

The approved Task 1 mutation is the `bd init` command above and files created by Beads itself. Do not create any other project file, including `.agent-team/`, settings, ledgers, hooks, a dashboard snapshot, or a task-import artifact. Optional-tool availability never changes this budget or Beads readiness.

## Records used by later revisions

When they exist, keep records narrow: decisions carry stable IDs, date, scope, decision, rationale, and supersession; mistakes carry stable IDs, failure mode, cause, remedy, and evidence; blockers contain unresolved questions only. Record a resolution in `DECISIONS.md` before removing a resolved blocker. A future session checkpoint is a breadcrumb, not worker or task authority; Beads and Git remain authoritative.
