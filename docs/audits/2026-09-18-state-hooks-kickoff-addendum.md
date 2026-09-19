# Agent-Team state, hooks, and Project Kickoff addendum

Audit date: 2026-09-18.  This is a read-only evidence summary; it does not
claim that historical receipts or documentation describe a currently live
agent.

## Current, declared, and stale records

| Surface | Current evidence | Assessment |
| --- | --- | --- |
| Git checkout | `main` at `7680da3714a3222b27e6c3f6f734fcfbfd9be1dc`; no tracked working-tree change. | Current source checkout. |
| User installations | `/home/server/.agents/skills/agent-team` and `/home/server/.claude/skills/agent-team` each record Agent-Team 7.3.1 at source revision `68b7c469…`. Their hook code and manifests are aligned. No Codex-specific copy was found under `/home/server/.codex/skills/agent-team`. | Current installed copies, but source revision predates this checkout's current documentation commit. |
| Setup receipt | Ignored `.agent-team/setup.json`, schema 1, setup version 6, selects root `TASKS.md`, has initialization `complete` and ownership epoch 3. | Local runtime state, last modified 2026-09-14. |
| State record | Ignored `.agent-team/state.json`, schema 1/state version 59, has a finite `agent-team-v7.3.0-release` run for AT-19/AT-20 and reports `progress_possible`; it has no claims, lanes, or pending operations. | No recorded active writer, but the retained run is stale or unreconciled. |
| Locks | `.agent-team/.locks` is empty. | No current local mutation lock. This does not prove that no unrelated host process exists. |
| Root tracker | [TASKS.md](../../TASKS.md#L1) calls itself the canonical tracker; AT-19 is `deployed` and AT-20 `verified` ([line 33](../../TASKS.md#L33), [line 34](../../TASKS.md#L34)). | Project documentation and selected tracker. It conflicts with the stale run's pending delivery IDs. |
| Local tracker | `.agent-team/TASKS.md` is an older 7.2.0/Project-Kickoff reliability list: REL-011 is in progress and REL-012–018 are blocked. | Stale alternate tracker; it conflicts with setup's root-tracker selection. |
| Team directory | `.agent-team/TEAMS.md` still marks the historical `TEAM-AT15-DOCS` row active. | Stale: root TASKS says AT-15 is verified. |
| Recovery memory | [CONTEXT.md](../../CONTEXT.md#L1) is 56.8 KB and mixes a current checkpoint with extensive historical records; [MISTAKES.md](../../MISTAKES.md#L1) is 2.1 KB with two focused lessons. | CONTEXT is a material context cost and duplicate control plane; MISTAKES is a useful durable lesson record. |
| Worktrees | `git worktree list` reports 39 worktrees, including one prunable external worktree and many historical task branches. | Retained resources, not proof of live teams. |

The state root also retains six checkpoints, 19 request envelopes, 184 evidence
files, advisory cache entries, ownership history, and operation mapping cache.
Those artifacts preserve provenance, but their coexistence with the root task
tracker, local task tracker, team table, and run record creates several
independent descriptions of ownership and progress.

## Hook implementation and behavior

The installed hooks are user-scope registrations; this audit found no project
hook registrations. Codex registers five event groups: `SessionStart`,
`PreToolUse`, `PreCompact`, `PostToolUse` for `Edit|Write|apply_patch`, and
`Interrupt` ([codex-hooks.json](../../hooks/codex-hooks.json#L2)). Claude
registers six: `SessionStart`, `PreToolUse` for Bash/Edit/Write/Skill/MCP,
`PreCompact`, `PostToolBatch`, `TaskCompleted`, and `UserPromptExpansion`
([claude-hooks.json](../../hooks/claude-hooks.json#L2)). Each invokes Node with
a three- or five-second timeout.

Both adapters run the same executable hook entry point. It imports event
normalization, policy, recovery, canonical-state, checkpoints, ownership,
telemetry, lint, and health modules
([agent-team-hook.mjs](../../hooks/agent-team-hook.mjs#L8-L20)). At session
start it can refresh the operation-mapping cache; at session start and prompt
submission it reads recovery; compaction, interruption, session end, and
Claude changed-file batches can write checkpoints
([agent-team-hook.mjs](../../hooks/agent-team-hook.mjs#L63-L66),
[lines 231-247](../../hooks/agent-team-hook.mjs#L231-L247)). It can append
activation logs and hook evidence ([lines 220-228](../../hooks/agent-team-hook.mjs#L220-L228)).

The policy is bounded rather than complete: its own documentation identifies
unmapped hosted tools, shell aliases, `write_stdin`, generated commands, and
host termination before hook execution as blind spots
([HOOKS.md](../../references/HOOKS.md#L47-L51)). Therefore lifecycle hooks add
per-event latency and state writes without establishing universal operation or
ownership enforcement.

### Project Kickoff user hooks

Claude user scope also registers three optional Project Kickoff handlers, but
this checkout is not activated for them: both `.project-kickoff/hooks.json`
and `.project-kickoff/context-hook.json` are absent. The live commands are
`load_context.py` on `SessionStart`, `guard_edits.py` on `PreToolUse` for
`Write|Edit`, and `check_checkpoint.py` on `PostToolUse` for `Write|Edit`
([installed settings](../../../../../.claude/settings.json#L127-L260)).

When enabled, `load_context.py` reads only the explicit marker, `CONTEXT.md`,
and `.project-kickoff/DISCOVERY.md`, then emits bounded checkpoint context;
`guard_edits.py` denies unapproved or shared-record edits; and
`check_checkpoint.py` emits advisory diagnostics for missing or conflicting
checkpoint fields. The scripts are read-only with respect to project data and
validate their guards and markers against the canonical Git root
([load_context.py](../../../../../.claude/skills/project-kickoff/scripts/load_context.py#L84),
[guard_edits.py](../../../../../.claude/skills/project-kickoff/scripts/guard_edits.py#L30),
[check_checkpoint.py](../../../../../.claude/skills/project-kickoff/scripts/check_checkpoint.py#L28)).

The Kickoff `SessionStart` and `PreToolUse` handlers share event groups with
Agent-Team, while its `PostToolUse` advisory is separate from Claude
Agent-Team's `PostToolBatch` checkpoint path. The Kickoff guard is listed before
Agent-Team's policy handler in the live `PreToolUse` group, so a Kickoff denial
can stop an edit before Agent-Team evaluates it ([Claude settings](../../../../../.claude/settings.json#L178-L199)).
The scripts use `/usr/bin/python3`, POSIX shell expansion, and POSIX file
operations; their own reference directs native Windows sessions to the
instruction workflow ([context-hook.md](../../../../../.claude/skills/project-kickoff/references/context-hook.md#L190-L198)).

## Project Kickoff handoff

Project Kickoff 0.5.0 is an optional planning dependency in the catalog
([dependency-catalog.mjs](../../hooks/lib/dependency-catalog.mjs#L155-L161)).
Its intended handoff produces `PLAN.md`, one selected tracker, and a validated
`AGENT_TEAM_HANDOFF.json`; Agent-Team 7.3.1 has a current consumption path.
This checkout has no `.project-kickoff/AGENT_TEAM_HANDOFF.json` or other
`.project-kickoff/` handoff directory, so the following is a contract audit,
not evidence that this project currently has a Kickoff handoff.

The 0.5.0 handoff is schema 1 and records `projectKickoff` (version, approval
ID, approved revision), `agentTeam` (tested version and initialization source),
project ID/root/branch/revision, tracker kind/path or absolute Beads executable,
and plan scope, acceptance, verification, branch, owned paths, external
actions, optional `requiredCapabilities`, and task IDs
([AGENT_TEAM_HANDOFF.json](/home/server/dev/skills/project-kickoff/assets/templates/AGENT_TEAM_HANDOFF.json#L1)).
The checker permits only Beads or `TASKS.md`/`.agent-team/TASKS.md`, requires
task IDs to exactly match the selected tracker, rejects dependency cycles and
runtime lane/claim/assignment/worker/capacity fields, and supports at most
1,000 task IDs in a 250 KiB handoff
([check_agent_team_handoff.py](/home/server/dev/skills/project-kickoff/scripts/check_agent_team_handoff.py#L16)).

Agent-Team 7.3.1 validates the producer/consumer version pair, path containment,
regular-file and digest identity, generation ancestry, and that the recorded
revision is still the canonical branch tip before initialization
([initialization.mjs](../../hooks/lib/initialization.mjs#L128-L158)). It then
consumes the handoff once into its initialization receipt with an operation ID
and timestamp ([initialization.mjs](../../hooks/lib/initialization.mjs#L455-L460)).
The current compatibility matrix is explicit and version-coupled: it includes
`0.5.0/7.3.0` and `0.5.0/7.3.1`, alongside historical pairs
([initialization.mjs](../../hooks/lib/initialization.mjs#L16-L18)).

The minimum facts a simplified vNext importer must preserve are: approved plan
revision and canonical branch; one tracker identity and location; stable task
IDs; acceptance criteria and verification checks; owned paths; and optional
required capabilities. It should preserve the single plan/single tracker
boundary while dropping producer-specific runtime state. Version negotiation
or a stable schema is a vNext design requirement; it is not implemented by
this audit or by the current package.

## Review chain and identity limits

The declared quality route is developer handoff, delegated mechanical verifier,
independent reviewer, repair, completion gate, then serial integration
([LANES.md](../../references/LANES.md#L47-L51)). The reviewer must differ from
the author, but the audited contracts do not require an explicit identity
inequality between verifier and reviewer. Current gates are separate stages:
the verifier runs before independent review, and the reviewer checks the exact
assignment/revision before the developer repairs findings
([TEAM.md](../../references/TEAM.md#L11-L17), [LANES.md](../../references/LANES.md#L47-L49)).
In a redesign, one non-author agent may perform both mechanical verification and
review to save tokens, provided it emits separate evidence for each stage and
the orchestrator still requires a `CLEAN` result before integration. That is a
design option for vNext, not current Agent-Team behavior.

## Windows and portability

Some executable discovery uses `.cmd`/`.exe` on Windows
([dependencies.mjs](../../hooks/lib/dependencies.mjs#L368-L376)), but the
pinned `uv` release asset selector only supports Linux and macOS
([dependencies.mjs](../../hooks/lib/dependencies.mjs#L971-L975)). The hook
registration strings use `$HOME` and Node rather than a PowerShell/CMD wrapper
([codex-hooks.json](../../hooks/codex-hooks.json#L8),
[claude-hooks.json](../../hooks/claude-hooks.json#L8)). Windows is consequently
partial discovery support, not a qualified end-to-end host path.

## Decisions this evidence supports

1. Establish one task authority: Beads or root `TASKS.md`; remove or archive
   the alternate `.agent-team/TASKS.md` only during a deliberate migration.
2. Treat host worker liveness as an observed runtime fact. Do not infer it from
   old team rows, worktrees, checkpoints, or retained run objects.
3. Keep Project Kickoff handoff optional and make it create one plan plus one
   task authority; do not retain it as another active ledger.
4. If independent review remains, require reviewer identity distinct from both
   author and verifier, or explicitly accept the weaker cost-saving route.
5. Make hooks optional and limited to a small checkpoint mechanism, or remove
   them. Do not use them as the primary coordination mechanism.
6. Define Windows support as either an implemented and tested host adapter or
   an explicit unsupported platform; partial binary suffix handling is not
   sufficient.
