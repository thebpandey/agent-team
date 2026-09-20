# Agent-Team vNext user-experience design record

> Status: historical design record. The native v8 implementation now exists;
> use the [README](../README.md), [Getting Started](../GETTING_STARTED.md), and
> [8.0.0 readiness guide](releases/8.0.0-readiness.md) for current commands and
> release status. Proposed wording below is retained as design rationale and is
> not an operational contract.

Agent-Team vNext should feel like one small project command: it reads the
project's plan, gives bounded work to developers, obtains independent review,
integrates only clean work, and resumes from compact project records. Git and
the selected tracker remain authoritative. The core installs/manages no
lifecycle hooks or default/Agent-Team-managed MCP server, and adds no daemon,
background wakeup, lease, or second task database.

## Install once

The planned distribution is one self-contained Go binary for Windows, macOS,
and Linux. Installation and host enablement are one combined operation with
staged internal steps:

```text
agent-teamctl install --host codex|claude|both
```

This one interactive/flagged operation verifies the published checksum and
selects Codex, Claude Code, or both as native host integrations. It installs
only the self-contained binary, host integration contract, skill entrypoint,
and install manifest, recording exact Agent-Team ownership for those artifacts.

The operation uses each host's native delegation integration. It does not
install hooks, register MCP, edit unrelated host settings, or start a service.
The planned health check reports selected hosts, Git, binary version, and any
missing native capability. Unsupported host/OS combinations remain readable
but cannot claim execution support. It snapshots exact Agent-Team-owned
integration entries and retains a rollback path; it preserves all pre-existing
hooks, MCP servers, and host settings.

The native installer owns lifecycle operations:

```text
agent-teamctl update
agent-teamctl rollback --version <v>
agent-teamctl uninstall
```

These are installer actions, not skill actions.

All commands in this document are proposed vNext command vocabulary; none are
available until the Go core is implemented.

## First project setup

From a Git project, invoke the planned setup skill action:

```text
Agent-Team skill action: setup
```

Setup checks the canonical repository, branch, existing plan/tracker, and
optional Project Kickoff handoff. Before creating anything it shows the exact
artifacts and asks for confirmation.

Default project artifacts:

- `TASKS.md` — the default and only active task authority;
- `.agent-team/config.json` — project preferences and safe limits;
- `.agent-team/runs/`, `.agent-team/receipts/`, and evidence pointers — compact
  execution state;
- `BLOCKERS.md` — active blocker projection, when blockers exist;
- `AGENT_TEAM_RULES.md` — required project-specific rules and prohibitions in
  plan mode;
- `DECISIONS.md` — required canonical project decision history in plan mode.

Beads is opt-in. Selecting it makes Beads the sole active authority; Agent-
Team never maintains writable `TASKS.md` and Beads simultaneously. In plan
mode, setup creates or adopts the selected tracker plus `AGENT_TEAM_RULES.md`
and `DECISIONS.md` after approval. If no tracker exists, the user may instead
choose a one-off run, which does not require those plan artifacts.

Project Kickoff is optional. A validated handoff is read once, imported as
project facts, and recorded by digest, producer version, approved revision,
branch, tracker, task IDs, acceptance/checks/paths, and required capabilities.
Existing project decision documents are explicitly imported or migrated into
canonical `DECISIONS.md` with provenance; they are not silently treated as a
second decision authority. Kickoff creates no second plan, tracker, hook, or
memory system.

## Role and model setup

Setup asks for functional roles and available host models:

```text
Orchestrator: [select available capable model]
Developer:    [select suitable model]
Reviewer:     [select independent suitable model]
```

Model and project defaults are reviewed through the planned skill action:

```text
Agent-Team skill action: settings
```

The orchestrator plans, delegates, supervises, integrates, and deploys. It
does not implement feature code or replace independent review. A developer
writes only its assigned worktree and paths. A non-author reviewer returns
only `FIX` or `CLEAN`. `BLOCKED` is a task/run state, recorded when a required
check, resource, decision, or external result cannot proceed. A deterministic
gate checks revisions, evidence, required checks, and clean scope; it does not
make semantic AI judgments.

Routing may downgrade ordinary low-risk work to a cheaper available model only
after availability and task risk are known. High-risk, security, migration,
or release work does not downgrade merely because capacity is busy. A missing
requested model is reported as a routing constraint, never silently relabeled.

## Normal continuous execution

```text
Agent-Team skill action: start [--task TASK-41]
```

During the user-initiated foreground turn, the orchestrator:

1. reads the selected tracker and approved scope;
2. admits dependency-ready tasks with disjoint paths/resources;
3. assigns each team a serial list of up to eight tasks;
4. reserves a non-author reviewer capacity slot;
5. reuses the same team for its next list after accepted completion;
6. routes `FIX` findings back to the developer and repeats review;
7. runs the deterministic gate on the exact candidate revision;
8. integrates accepted work serially;
9. deploys only authorized completed batches.

The default project parallel-team limit is configurable within host capacity.
The orchestrator never creates another scheduler or agent hierarchy.

## Add work or execute it now

Use one explicit skill action and intent so an idea is not accidentally
dispatched:

```text
Agent-Team skill action: task add --queue
Objective: Add export filtering

Agent-Team skill action: task add --execute
Objective: Fix the broken export filter
```

These are proposed skill actions, not yet implemented. `--queue` records a compact
task in `TASKS.md` or Beads without dispatch. `--execute` records/deduplicates a
task, validates acceptance/scope/dependencies/resources, and admits it when a
reviewer slot is available. If intent, acceptance, or scope is materially
ambiguous, the host asks one concise clarification.

An exact active-task duplicate becomes a continuation, not a new task. Similar
work is linked as a duplicate candidate or follow-up; it is never silently
merged.

## One-off work without a plan

For one bounded feature when no tracker exists, use an immutable one-off
manifest. A read-only audit or review is a separate kind: it inspects a pinned
revision and produces evidence, but does not implement, integrate, or deploy.

```text
Agent-Team skill action: one-off feature
Objective: Fix export filtering

Agent-Team skill action: one-off audit (read-only)
Objective: Audit the auth boundary

Agent-Team skill action: one-off review (read-only)
Revision: <git-revision>
```

One-off mode is capped at two safe teams and one bounded objective. It creates
one immutable one-off manifest and evidence directory, not a second project
ledger. Feature work still uses a worktree, non-author review, deterministic
gate, and serial integration. Read-only audits/reviews produce only revision-
bound evidence and do not integrate.

Escalate to a real tracker when work expands to multiple independent tasks,
shared schema/migration changes, deployment batches, external mutations,
uncertain acceptance, or more than one team queue.

## Review, gate, integration, and deployment

The developer handoff contains task ID, acceptance, exclusive paths/worktree,
input/base revision, checks, selected capability paths/digests, evidence
destination, prohibitions, and next action. The reviewer receives the exact
candidate revision and changed-file fingerprint.

Only a revision-bound `CLEAN` can reach the gate. Integration changes create
a new candidate identity and require a new review and gate. After successful
integration, the top-level task enters the deployment queue if deployment is
enabled. Batches use the configured size, including a final nonempty smaller
batch. A failed or ambiguous deployment holds later deployment while coding
on unaffected work may continue.

## Servers, browsers, and dashboard

Development servers and browser sessions are on-demand capabilities, not
always-running services. The project limit is at most two Agent-Team-managed
servers and two browser sessions. Each is bound to one team, worktree,
revision, purpose, URL/port, and evidence. Reuse requires the same revision
and purpose. Known managed resources stop gracefully after evidence and known
consumers are finished; unknown processes are never killed by port/PID alone.

After successful implementation integration, the planned dashboard snapshot is
written to:

```text
<project>/.agent-team/dashboard/index.html
```

It is local-machine-only, ignored, derived from canonical state, and viewable
by opening the file directly. It is not shared across Codex/Claude hosts and
is not a tracker, deployment record, server, or visual-review evidence store.
A failed refresh preserves the last-good snapshot and does not block
integration.

## Blockers and decisions

`BLOCKERS.md` is a live human-readable projection of active root blockers. Its
source facts are the canonical tracker, run, receipt, and applicable decision
records. Resolved entries are removed from `BLOCKERS.md`; their evidence and
decision history remain durable elsewhere.

`DECISIONS.md` is required in plan mode and may be absent in one-off mode. Only
consequential user/product/security/authority decisions belong there. Routine
test, environment, or implementation failures stay in the task, receipt, and
evidence history. Existing project decision documentation is explicitly
imported or migrated into canonical `DECISIONS.md` with provenance; do not
silently adopt it as a second decision ledger.

`AGENT_TEAM_RULES.md` is required project guidance in plan mode and optional
in one-off mode: scope conventions, prohibitions, approval requirements, and
safe commands. Promoted rules reference canonical `DEC-*` decision IDs. It
cannot override the core role, revision, review, tracker, path, secret, or
cleanup rules.

## Pause, stop, cancel, and resume

```text
Agent-Team skill action: pause --run R-01 --team TEAM-02 --task TASK-41
Agent-Team skill action: stop --run R-01 --team TEAM-02 --task TASK-41
Agent-Team skill action: cancel --run R-01 --task TASK-41
Agent-Team skill action: resume --run R-01
Agent-Team skill action: status [--run R-01]
Agent-Team skill action: inspect --run R-01 --team TEAM-02 --task TASK-41
Agent-Team skill action: cleanup --run R-01 --team TEAM-02 --task TASK-41
Agent-Team skill action: deploy [--run R-01] [--batch-size N] [--target profile]
```

These are planned actions. `pause` stops new admission while preserving claims
and evidence. `stop` ends the current foreground turn without claiming work is
complete. `cancel` explicitly ends a task without reverting source. `resume`
reconciles tracker, Git, receipts, resources, blockers, and pending external
operations before continuing. `status` and `inspect` are read-only;
`reconcile` records a repair plan and does not dispatch, integrate, stop
processes, or deploy. Scoped pause/stop/cancel/resume/inspect/cleanup/deploy
actions use the canonical `--run`, `--team`, and `--task` flags; a run ID is
optional only when exactly one run is active. Deployment uses the core-derived
task/batch fingerprint and never accepts a caller-invented fingerprint.

## Generated run handoff and exact resume

Each active run has one bounded, derived Markdown handoff:

```text
<project>/.agent-team/handoffs/<run-id>.md
```

It is generated from the existing machine-readable receipts and checkpoints;
it is not a second authority and contains no per-team, per-task, or per-attempt
JSON handoffs. It summarizes the run ID, active teams/tasks, exact
candidate/integration revision, latest reviewer `FIX`/`CLEAN` verdict,
deterministic gate result, active server/browser/resource references, evidence
pointers, blockers, prohibitions, and next action.

Example Codex skill action:

```text
Agent-Team skill action: resume --run R-01
```

Example Claude Code skill action:

```text
Agent-Team skill action: resume --run R-01
```

The binary action is the same; the host adapter supplies the native delegation
context. A switch does not transfer a lease or require the old session to
return. Only one Codex/Claude harness is active for a project at a time.
Unknown worker liveness may serialize or block only the conflicting task or
resource-cleanup operation; it never blocks Codex↔Claude continuation, unrelated
tasks, or tracker resume.

## Optional tools and visual work

Optional tools remain off until selected for a task. Agent-Team has no
Agent-Team-managed or default MCP server. Serena is permitted only after
explicit read-only consent. LeanCTX is limited to compact discovery/output
shaping; Graphify is a local code-only structural aid; Playwright is selected
only when real browser evidence is needed. None becomes task authority,
memory, hooks, or an always-loaded schema.

Initially route at most one analytical accelerator per task. Select a second
only for a named unanswered question after the first result is insufficient.
Native source reads, search, Git, and project checks remain the fallback when
an accelerator is missing, declined, unhealthy, or unnecessary.

For visual work, select `impeccable` plus `ui-ux-pro-max`. `ui-styling` is an
optional additional capability for a stack/task-specific need or explicit user
selection; it never substitutes for either required visual skill. Skills are
loaded task-scoped by path/digest. The project's single design-system artifact
remains authoritative. A reviewer must inspect rendered screenshots at
representative sizes; text snapshots alone do not prove visual acceptance.

## Safe cleanup

After terminal integration has passed its exact review, gate, and revision
checks, automatic-safe cleanup is the default. Cleanup coordinates exact
managed-resource stops and manifest-owned disposable worktree/transient
removal, with evidence safely retained elsewhere. It must
preserve dirty/untracked/ignored user files, unknown processes, shared or
external databases, backups, remote branches, custom host settings, and
obsolete state whose ownership is uncertain. Cleanup failure becomes a bounded
cleanup blocker; it never triggers force deletion.

Update and uninstall own only exact Agent-Team-installed files and preserve
existing hooks, MCP servers, host settings, project trackers, evidence, and
optional tools. Updates retain a verified rollback version; uninstall removes
only unchanged files recorded as Agent-Team-owned.

## Known limitations

- This is not implemented yet; the current installed skill remains v7.
- Execution is foreground-only during a user-initiated host turn.
- No daemon, cron job, background wakeup, automatic parent replacement, or
  guaranteed work after the host exits.
- Codex and Claude must not run simultaneously on one project.
- Optional tool availability, host capacity, credentials, and project commands
  remain environment-dependent.
- Unknown liveness, dirty worktrees, ambiguous external operations, and missing
  required checks block only their affected task or cleanup operation rather
  than being guessed through. Unknown liveness never blocks Codex↔Claude
  continuation, unrelated tasks, or tracker resume.
