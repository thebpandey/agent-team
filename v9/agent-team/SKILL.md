---
name: agent-team
metadata:
  version: "9.0.0"
description: Use when coordinating an active Codex or Claude development session with Beads: inspect status, initialize Beads after explicit approval, or select a bounded page of ready work.
---

# Agent-Team v9

Use Beads for live task state and Git for source revisions. This is an active-session skill: it has no controller, daemon, hooks, second task database, or promise of work after the host session ends.

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

Use [native host routing](references/HOSTS.md) for host operations and [the worker contract](references/WORKER_RULES.md) for assignment limits. Do not promise independent review, integration, deployment, lifecycle control, or cross-session persistence before their later procedures and observed evidence exist.

## State and honest reporting

Use [STATE.md](references/STATE.md) for Beads commands, one-time inputs, and the ownership of optional project records. Preserve existing project data. State what was observed, which command ran, whether a write was approved, and any Beads error. Never claim native dispatch, review, integration, deployment, or cross-session persistence unless that capability was actually implemented and observed in the active host session.
