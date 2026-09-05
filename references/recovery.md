# Pro pause and recovery

Restore the work from durable evidence. Do not promise restoration of the exact chat, unsaved edits, or original agents. Use existing tracker records, the team directory, and CONTEXT.md; do not add another task database or transcript archive.

## Checkpoint during normal work

Each agent refreshes its assigned CONTEXT.md at meaningful progress, before handoff or intentional pause, and before compaction when possible. Keep it normally below 600 words. Write updates atomically where supported and preserve the last complete checkpoint if an update fails. Store project and team IDs, attempt/session identity, skill revision and instruction paths, worktree/branch/base/current revision, saved uncommitted changes, tracker and MISTAKES.md pointers, task IDs, valid evidence, and the next action.

Include pointers to dependency choices, applicable authorization, preview gate and approved/review version, and pending operations. The tracker owns task and approval state; checkpoints carry pointers and essential recovery facts, not competing decisions. Record a consequential operation's intent and target before execution, then its provider/process identity and observed result immediately afterward. Examples are deployment, migration, external messages, integration, and resource removal. Never store credentials or private reasoning.

Use persistent project storage for code and checkpoints. Record the location outside disposable worktrees for shared records and required evidence. If storage is temporary, report that a lost workspace cannot be recovered from notes alone. Do not create commits, upload private data, or install shutdown hooks without existing authority. No checkpoint policy can guarantee notification of a crash.

## Pause a named team

1. Resolve the team and its current owner. Stop assigning new work to that team; leave unrelated teams running.
2. Request a checkpoint and safe pause from its active members through supported controls. Let an indivisible external operation reach an observable state when stopping it would be unsafe. Do not kill processes blindly.
3. Save existing edits, task progress, unfinished checks, ownership, preview identity, and pending operation IDs. Preserve worktrees and evidence. Pausing does not mean committing, merging, deployment, or deletion.
4. Record Pause requested until writers have acknowledged or their stopped state is verified. Only then record Paused. If some activity cannot be controlled, report it as still running or unknown; do not claim a complete pause.
5. Preserve a requested preview when its process can safely remain available; report whether the host can keep it alive. Do not confuse a preview server with a developer still modifying its files.

For a natural-language request to end the session, perform this bounded checkpoint procedure if the host allows a response. If the user closes the app immediately, recover from the last saved state later. Do not require a final shutdown hook.

## Resume scope and stage

An unqualified `resume` or `resume all` recovers all unfinished Agent-Team work in the current project, including intentionally paused teams. A named resume affects only that team. Take a bounded inventory of the team directory, active tasks, task-owned worktrees, pending handoffs, and incomplete release/cleanup records. Include work whose coding is finished but integration, deployment, live verification, or required cleanup remains. Exclude fully delivered and cleaned teams, cancelled work, approved deferrals, and unrelated user worktrees. Investigate recorded task worktrees missing from the directory without deleting or silently claiming unknown work.

Reconcile project ownership first, then inspect each eligible team. Reattach surviving writers; replace only confirmed stopped writers. Report already-running teams and let them continue. Launch eligible replacements within host capacity and queue the rest; do not overload the host with every unfinished task at once. One resume action is a recovery pass, not a new team per worktree or permission to repeatedly relaunch a blocked task. A single team can own several worktrees.

Continue from the actual stage: unfinished code returns to implementation; verified code waits for any preview approval then integration; merged work continues authorized deployment; already deployed work proceeds to live verification, tracking repair, and cleanup as needed. Never reimplement completed work or redeploy a verified release to satisfy a stale label. Approval gates, access constraints, and dependencies remain intact. Give one aggregate report with each team’s recovered stage, activity, blocker, and next action. Continue independent unblocked work without waiting for unrelated teams.

## Resume after a pause or interruption

1. Resolve the project and team from canonical records and actual worktree metadata. Ask one question only if the intended work is ambiguous. Never restart the plan from scratch because the conversation is new.
2. Read current project instructions, the installed harness/action rules and host adapter, dependency choices, relevant lessons, tasks, and agent checkpoints. Reapply role and skill rules to replacement agents. Report material instruction/version differences. Old notes cannot override current instructions or grant new access.
3. Inspect saved files and uncommitted changes, current branches, worktree ownership, process/session identities, preview state, evidence revisions, and in-flight provider operations through supported read access. Check whether original writers are alive. A lost connection, reused PID, or stale timestamp does not prove they stopped.
4. Reattach surviving sessions where supported. If another owner is still active, hand the request to it or report that the team is already running. Replace a writer only after confirming it stopped or completing an explicit ownership handoff. If that cannot be established, preserve the work and block conflicting writes while allowing independent tasks.
5. Reconcile interrupted operations from external evidence before retrying. If deployment succeeded, verify and record it without deploying again. If cleanup stopped midway, inspect each recorded resource and remove only what remains eligible. Inspect unresolved Git operations without resetting them. Unknown outcomes stay unknown until resolved.
6. Restore the preview gate, pending approval, reviewed version, and release authority. Resume never implies approval. Restart a dead preview only under its existing authorization after checking port and process ownership, then verify its served version. Do not call an old URL live without checking.
7. Reuse tests only when their relevant source, dependencies, configuration, and environment still match. Schedule a focused check for missing or stale evidence. Do not repeat a whole review or install declined dependencies.
8. Update ownership/session identity and the canonical tracker through its writer. Give a short report: recovered work, verified results, remaining uncertainty, surviving/replaced agents, preview/approval state, and next action. Continue the next safe authorized task without asking for general permission again. An approval gate or unresolved access/ownership constraint remains a real blocker.

A normal development invocation that finds interrupted work in its scope enters this procedure. A `status` invocation never enters it. Do not fabricate lost task criteria or silently migrate the tracker to make recovery appear complete.
