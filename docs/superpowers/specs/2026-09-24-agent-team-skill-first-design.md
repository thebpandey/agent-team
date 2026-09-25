# Agent-Team skill-first replacement design

**Status:** written design for user review, 2026-09-24 (America/Chicago). No implementation is authorized by this document alone.

This design supersedes the 2026-09-18 vNext controller design for the replacement release. Published 8.x releases remain historical and unchanged. The proposed replacement is v9.0.0 only after its acceptance gates pass.

## Purpose and honest boundary

Agent-Team helps one active Codex or Claude orchestrator implement a Beads plan accurately and with low token overhead. It assigns bounded work to native subagents, obtains an actual independent review, integrates accepted changes, and repeats until paused, blocked, or out of ready tasks. It does **not** run after the host session ends. A later session resumes from Beads, Git, and a short checkpoint.

The replacement is a skill package, not a Go/Node controller, daemon, hook suite, installer service, or second task database. Git and Beads are the only required external commands. It cannot guarantee exactly-once native worker creation after an ambiguous host response; it isolates uncertainty to the affected task and continues independent work.

## Sources of truth

| Concern | Authority | Other records |
| --- | --- | --- |
| Tasks, dependencies, acceptance, status | Beads | `TASKS.md` and Project Kickoff handoffs import once; neither stays live. |
| Source and integrated revisions | Git | A worktree is an isolated workspace, not a task database. |
| User and orchestrator decisions | `DECISIONS.md` | Beads issue comments link decision IDs without copying the full decision. |
| Recorded mistakes and their remedies | `MISTAKES.md` | Agents search relevant entries before acting. |
| Questions needing user input | `BLOCKERS.md` | Only unresolved items; resolution is recorded in `DECISIONS.md`, then removed here. |
| Last session's operational handoff | `.agent-team/SESSION.md` | Orchestrator-maintained breadcrumb, not ownership, task, or worker authority. It replaces v7's manually maintained per-agent `CONTEXT.md` role. |
| Project-specific worker policy | `AGENT_TEAM_RULES.md` when present | Cannot override direct user instructions or the packaged safety contract. |

`DECISIONS.md` entries use stable IDs, date, scope/task IDs, decision, rationale, and supersession link when changed. `MISTAKES.md` entries use stable IDs, failure mode, cause, remedy, and evidence. The orchestrator passes only task-relevant IDs or short excerpts to workers; every agent can search the full files on demand. This makes both references available without loading growing ledgers into every prompt. Agents propose ledger changes; the orchestrator records accepted decisions and confirmed mistakes. The orchestrator immediately removes resolved items from `BLOCKERS.md` after recording their resolution.

## Active-session execution loop

1. Read a bounded page of `bd ready` results and the applicable project rules. Do not load a 1,000-task database into the prompt. The orchestrator chooses disjoint work with clear acceptance and write scopes.
2. Assign up to the project-configured number of parallel teams. Each team receives at most **four ordered tasks** and claims only its next task when starting it. Teams are reused within the active session. A one-off request becomes a Beads task and may use up to two safe parallel teams.
3. Atomically claim the next Beads task, prepare its Git worktree, and invoke the host's native agent tool. Normally one developer and one independent reviewer suffice; add a specialist only for demonstrable task complexity. The orchestrator may choose a cheaper capable model per assignment.
4. The developer works only in its assigned worktree and approved scope. Its report names changed files, revision, checks, remaining uncertainty, and relevant decision/mistake IDs. It does not self-approve, integrate, deploy, or edit another team's work.
5. One non-author reviewer checks the actual diff, task criteria, relevant checks, and evidence. It returns precise FIX findings or a CLEAN report bound to task ID and revision. The developer remediates FIX findings and the same independent gate is repeated. Repeated inability to reach CLEAN becomes a task-local blocker, not an endless token loop.
6. The orchestrator performs a quick final check, integrates and commits CLEAN work, updates Beads, and takes the next ready task. Direct orchestrator code edits are exceptional: only a narrowly scoped change demonstrably cheaper than delegation, recorded with rationale and independently reviewed before integration.
7. Deploy only in the project's preapproved completed-task batches, using an explicit project-provided command and verification rule. No generic deployment discovery. Refresh the local HTML dashboard after successful integration; snapshot failure is reported but never blocks Beads progress.

The main worktree is for planning, decisions, status, integration, and reports. Feature implementation happens in task worktrees. The orchestrator coordinates write scopes within the session rather than persisting cross-harness file leases. Integrated clean worktrees and their merged branches are removed safely; dirty, unmerged, or unknown work is retained and reported. At most two on-demand dev servers run per project; worktrees may share an appropriate existing server, and servers are stopped on graceful session end.

## Pause, blockers, and uncertain native actions

Pause stops assignment immediately. The orchestrator asks active agents to reach a safe checkpoint or uses the host's actual stop facility when requested, then writes one `.agent-team/SESSION.md` from their reports: host and time; active Beads IDs; worktree, branch, revision and uncommitted-work summary; evidence and test-result pointers; last observed native handles and any uncertainty about them; pending operations and approvals; outstanding blockers; and the next explicit action. It records pointers rather than duplicating task definitions, decisions, or full transcripts. It does not mark unfinished work complete. Resume rechecks Beads, Git, worktrees, evidence, and host-visible handles; it follows up with the same worker when that handle is available.

An explicit native launch rejection that guarantees no worker was created returns the claimed task to ready, with a Beads comment noting the failure. A timeout, lost response, or possible launch is **uncertain**: mark only that task blocked, record the observation and required reconciliation in `BLOCKERS.md` if user input is needed, and continue other ready lanes. Never infer absence from an empty checkpoint or a new host session. A temporary worker NO-GO leaves its task incomplete and blocked; after resolution, follow up on the same native handle if available. If it is unavailable, inspect the prior host and worktree before any reassignment. No fabricated identity, CLEAN review, or completion is permitted.

On a question the orchestrator can resolve with high confidence inside approved scope, it records the decision and instructs the affected team. Otherwise it puts one concise, consistently formatted question with recommendation and impact in `BLOCKERS.md`, prompts the user at status milestones, and keeps unrelated lanes running. An unexpected session loss may leave a task uncertain; a later session must not silently re-launch it. This is a task-local limit of the host API, not a project-wide deadlock.

## Setup, models, and optional aids

One platform-neutral versioned skill package contains a short shared workflow and small Codex/Claude host adapters plus a worker contract. It has no runtime binary or hooks. Installation makes recoverable backups of exact prior skill roots; it does not modify unrelated host settings. README and first-run output identify the local dashboard path and its snapshot-only purpose. A missing Beads database or minimum project files triggers one clear approval question before creation. Project Kickoff is optional; an approved handoff or `TASKS.md` imports into Beads once. No Project Kickoff interview is repeated by Agent-Team.

On first host setup, offer actual available model/effort choices and recommendations for orchestration, development, review, and visual work; save preferences once with optional project overrides. A setting is a preference, not a promise that the host will honor an unavailable model. The orchestrator may downgrade a particular task to a cheaper capable model and records material exceptions.

Ponytail's simplest-working-change discipline is part of the default developer and reviewer contract; invoke the installed Ponytail skill when available. It is not an external runtime requirement. LeanCTX is absent from the replacement's installation, probes, prompts, and dispatch. Existing separate LeanCTX installations are left untouched. Serena, Graphify, Playwright/browser, Impeccable, UI-styling, and UI-UX-Pro-Max are optional task-triggered aids. Their absence cannot block ordinary work. Visual work uses the appropriate available design skills; browser verification uses Playwright when the task needs it. No accelerator bundle is selected by default.

## v8 cutover and release gates

An existing v8 project cuts over only after its native workers have finished or stopped and active worktrees are reconciled. The new skill adopts its Beads data, leaves old controller records as a read-only archive, and does not migrate live reservations or claim to repair legacy uncertain launches. Old Agent-Team hook registrations, if present, are identified and removed only as exact Agent-Team-owned entries with a recoverable backup. Projects with an uncertain old worker remain on a task-local reconciliation path rather than silently duplicating work.

The v9.0.0 candidate is not called stable until these observable canaries pass:

- A real Beads fixture with 1,000 tasks, dependencies, blocked/closed history, and bounded ready-task selection without putting all records in a model prompt.
- Native Codex and Claude sessions: assign, implement, independently review, remediate, integrate, commit, close, and refill a team of up to four ordered tasks.
- Explicit no-launch rejection, ambiguous launch isolation, temporary NO-GO and same-handle follow-up, unrelated-lane continuation, graceful pause/resume, and quiescent host switch.
- One-off work with up to two teams; Project Kickoff and `TASKS.md` one-time imports; optional-tool absence; two-server project cap; non-blocking dashboard refresh.
- Actual install, update/rollback or recoverable replacement, and live-host smoke on each OS/host combination advertised as verified. Windows and Linux results are stated separately. A combination lacking a live canary is labeled unverified, not green by inference from unit tests.

Release notes, GitHub README, and the public page describe only proven behavior. The old 8.x branch, records, and release artifacts remain recoverable until the replacement has passed its canaries and the user approves cutover.

## Explicitly retired from the replacement

The v8 Go controller's run/team/admission mirror records, synthetic `ack/complete/clean/idle/next` protocol, duplicate lifecycle controls, package-time optional-dependency bundle, v7 hooks, live `TASKS.md` authority, and managed binary updater are not ported. Their old code remains historical source during the transition, not a hidden dependency of the new skill.
