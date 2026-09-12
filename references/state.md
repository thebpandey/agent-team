# Tasks, memory, and failures

## One authoritative task graph

Choose Beads or the selected root/designated TASKS.md as the only active project tracker, recording its identity and canonical location in the setup receipt and handoffs. Reuse Project Kickoff or existing project choices. Skill availability never selects or migrates the tracker. In Beads mode, discover installed-version help; do not assume storage backends, commands, or custom statuses. Use native states plus supported metadata for execution phase. Keep requirement-to-task/evidence mappings in the active tracker, never a second parallel checklist.

The project orchestrator creates/deduplicates tasks/issues, manages dependencies/ownership, integrates evidence, and closes/reopens tasks. In Beads mode teammates update only assigned progress and append evidence; in local mode the project orchestrator is the sole tracker writer and teammates send updates to the orchestrator. For Beads, confirm the backend supports concurrent writes before enabling them. If single-writer, serialize teammate-authored updates through the orchestrator; do not start competing database writers. Use supported persistence/sync at handoff and release.

Use [project coordination](projects.md) for stable team IDs, shared-record ownership, and one canonical tracker across worktrees. TEAMS.md holds identities and resource locations only. Team leads route updates to the project owner where direct writes are not supported.

Task evidence needs project/team IDs, requirement IDs, acceptance criteria, owner/dependencies, current phase, changed revision, relevant check/environment identity, result/evidence link, and next action. Link logs/screenshots instead of inserting them. Track unrelated discoveries separately without automatically expanding scope.

For counted or continuous work, include the [run record](runs.md) in this same tracker. Keep selected top-level delivery IDs distinct from child tasks and summary epics. Record effective choices and explicit overrides, run ownership/hold, membership, integration boundary per delivery, pending batch IDs, exact release artifacts, and provider outcomes. Derive occupied development slots from actual team/task evidence; integration frees a slot without claiming production success. Do not create a second live queue or count Git commits as completed tasks. Project defaults live only in `run_defaults` in the canonical setup receipt, as [settings](settings.md) specifies.

Keep immutable initialization (`initialTaskIds`, tracker fingerprint, consumed handoff and original scope) separate from versioned effective-run transitions. Supported workflow names are exactly `run-start`, `run-reconcile`, `run-scope-extend`, `completion-quarantine`, `completion-rebind`, `evidence-store-register`, `completion-history-reconcile`, and read-only `run-decision`. Scope extension is owner-only and additive. Completion, integration, and publication evidence remains task-keyed across current and historical records.

Project status provenance separately: `generatedBy` and `testedAgainst` describe producer compatibility, authenticated `loadedRuntime` describes the installed consumer actually loaded, `sourceCandidate` is only an observed non-authoritative checkout, and `readiness` is current evidence. Never promote a source candidate or prose into runtime or release authority. Report selected Beads with no Dolt remote as `local_only`; report auto-deploy with no exact target as `enabled_but_held` / `target_required`, without erasing independent `origin/main` publication authority.

## Local-file tracking and safe switching

Create `.agent-team/TASKS.md` in the main project checkout, or reuse an existing user-designated task file. Record its absolute path and do not create per-worktree copies. Preserve any existing content and task IDs. Keep the file outside disposable worktrees and out of product commits unless the project intentionally tracks it. The project orchestrator alone writes it; teammates report progress with task IDs, revision, evidence, and next action. Serialize updates, preserve concurrent user edits, and use atomic file replacement where supported.

Use this minimal structure, expanding only for actual work:

```markdown
# Agent-Team Tasks
Updated: <time>; writer: <orchestrator>; tracker: local
Objective: <requested outcome>

| ID | Requirement / acceptance | Owner | Depends on | Status | Revision / evidence | Next action |
| --- | --- | --- | --- | --- | --- | --- |
| AT-001 | <observable result> | <owner> | <IDs or none> | planned | <links> | <action> |

## Failures
<One ID per distinct meaningful issue, linked task IDs, attempts, result and resolution proof.>

## Releases and cleanup
<Deployment/recovery identity, revision, live evidence, retained/removed worktrees.>

## Run
<Run ID/owner/scope, effective choices and explicit overrides, scheduling/release holds,
admitted delivery IDs, integration boundaries, pending/in-flight batch pointers.>

## Reconciliation
<Every original requirement ID, outcome, evidence and any approved deferral.>
```

Use planned, in_progress, blocked, verified, deployed, or deferred status as appropriate. A non-deployed task can finish at verified if deployment is outside scope. Update after claims, meaningful progress, failures, handoffs, verification, each successful deployment/rollback, and cleanup. Keep release history and unresolved evidence references; remove redundant narration instead of appending a diary. CONTEXT.md remains short resumption memory, not a second task list.

If Beads becomes unavailable, preserve it as the authority. Diagnose and repair within scope; use timestamped evidence only as explicitly stale/unknown information, never as a writable fallback tracker. Continue independent work that does not require unavailable claims or gates. Do not repeat external actions to infer their outcome or manufacture a passing empty task list.

Do not switch back just because Beads is later installed. On explicit selection of Beads, pause tracker writes, snapshot the local file, transfer tasks/IDs or record an ID mapping, dependencies, failures, and evidence, and verify coverage before changing the active mode. Mark the old file as an archived snapshot with a pointer to Beads. If transfer fails, keep local mode active. Apply the same reconciliation when leaving an accessible Beads tracker. Never maintain two writable authorities or destroy original records.

## Small resumption checkpoints

Aim for a few hundred words per agent's CONTEXT.md, normally below 600. Replace stale notes rather than appending a diary:

```
Updated: <time>; project/team: <IDs>; agent/session: <owner/attempt>
Harness: <version and applicable instruction paths>
Location: <checkout/branch/revision>; uncommitted changes: <summary>
Tasks: <task IDs, tracker mode, and absolute canonical location>
Decisions: <essential facts; durable doc links>
Lessons: <absolute MISTAKES.md path; relevant IDs and revision>
Evidence: <result locations and matching revisions>
Pending: <operation intent/identity, preview and approval pointers, blocker or handoff>
Next: <one executable action>
```

Include authorization/target pointers for release resumption. Never store secrets, credentials, raw personal data, or private reasoning traces. Keep local checkpoints out of product commits unless intentionally tracked, and preserve them before checkout cleanup. Give read-only agents unique paths.

Checkpoint at milestones, before handoff, when context pressure is signaled, and before requested compaction. Exact warnings are not guaranteed. After interruption, follow [pause and recovery](recovery.md). Read the checkpoint, active tasks, current Git state, ownership, approval gates, and in-flight operation status. Old memory and task labels are not proof of success. Reconstruct missing memory from evidence rather than guessing or repeating deployment.

Store enduring conventions in existing project docs and use one canonical design-system source. Use the Pro [shared mistakes procedure](mistakes-memory.md) to record confirmed agent mistakes and prevention actions in the main checkout's `MISTAKES.md`. Keep task-specific attempts in the active tracker. Link each lesson to its issue; do not duplicate failure logs across memories.

## Evidence-driven failures

Create one issue per distinct meaningful failure, link affected work, and deduplicate by symptom/cause where known. Record category (implementation, verification, environment, deployment, permission, tracking), symptom/reproduction, revision/environment, evidence pointer, and owner. For each attempt append hypothesis, material change, result, learning, and next action. Close with resolution proof.

Do not issue-track routine typos, expected empty searches, or harmless transient commands. Append evidence to an existing failure instead of creating tickets per retry.

Retry only with new evidence or a changed approach. After two attempts without useful progress, reassess and apply an approved escalation or different strategy. If repair still cannot progress, safely park the affected task with its evidence and resume condition while independent authorized tasks continue. An ordinary engineering failure does not require a user fix/continue instruction. Ask only for a genuinely missing authority, access or material product decision. A transient failure may justify one bounded retry; inspect ambiguous side effects first. Never erase failing tests, waive required checks, or mark blocked work complete to advance.

## Findings and later work

Record all findings in the same Beads/TASKS authority, not backlog.md:

- Required in-scope repair: assign, fix and verify automatically.
- Approved later work: queue with priority/dependencies and execute when eligible within the run scope.
- External blocker: record impact, attempts and resume condition; retain claim/gates and stop its writer before parking compute.
- Out-of-scope improvement: record proposed/deferred, not authorized implementation.

Keep stable ID, description, priority, origin, acceptance, disposition and blocker/next action. A discovery relation becomes a blocking dependency only when work actually depends on it. A required check cannot be relabeled optional just to complete the task.

## Durable context ownership

Persist facts when they arise, not only when compaction is imminent. The tracker owns task state and acceptance. Project documents own approved decisions, constraints and rejected alternatives. Checkpoints own only the current attempt, pending operations, uncertainty, next action and links to those records. Evidence belongs to its actual revision/environment.

Use a compact recovery index, not a summary of previous summaries. Preserve prohibitions and distinguish proposals, approvals and unresolved decisions. Do not add a memory database, vector index, MCP memory service or full-transcript archive. Recover original diagnostics when needed; compressed output is not verification proof.

A fresh worker must verify task scope, actual files/revision, writer handoff, pending operations and required evidence from its packet and referenced records before continuing. Missing records trigger bounded reconstruction from original evidence; ask only when a material fact cannot be recovered. Keep native compaction enabled as fallback and never claim cache space expands the context window.

Reference: [Beads documentation](https://github.com/gastownhall/beads). Installed-version behavior controls implementation.

Use [status](status.md) for read-only counts and progress. A status request is not a tracking transition and must not trigger checkpoint writes or reconciliation. Use the canonical parent task for preview and approval state as specified in [preview approval](preview.md).
