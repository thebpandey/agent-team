# Tasks, memory, and failures

## One authoritative task graph

Choose Beads or the local TASKS.md as the only active project tracker, recording the mode and canonical location in the setup receipt and agent handoffs. Default to local mode when any of Beads, Ponytail, Using-Superpowers, or Impeccable is skipped/unusable. Continue using the other enabled skills. In Beads mode, discover installed-version help; do not assume storage backends, commands, or custom statuses. Use native states plus a short phase field/note for planned, implementing, verifying, ready to deploy, deployed, or blocked if needed. Keep requirement-to-task/evidence mappings in the active tracker, never a second parallel checklist.

The orchestrator creates/deduplicates tasks/issues, manages dependencies/ownership, integrates evidence, and closes/reopens tasks. In Beads mode teammates update only assigned progress and append evidence; in local mode the orchestrator is the sole tracker writer and teammates send updates to the orchestrator. For Beads, confirm the backend supports concurrent writes before enabling them. If single-writer, serialize teammate-authored updates through the orchestrator; do not start competing database writers. Use supported persistence/sync at handoff and release.

Task evidence needs only requirement IDs, acceptance criteria, owner/dependencies, current phase, changed revision, relevant check/environment identity, result/evidence link, and next action. Link logs/screenshots instead of inserting them. Track unrelated discoveries separately without automatically expanding scope.

## Local-file tracking and safe switching

Create `.agent-team/TASKS.md` in the main project checkout, or reuse an existing user-designated task file. Record its absolute path and do not create per-worktree copies. Preserve any existing content and task IDs. Keep the file outside disposable worktrees and out of product commits unless the project intentionally tracks it. The orchestrator alone writes it; teammates report progress with task IDs, revision, evidence, and next action. Serialize updates, preserve concurrent user edits, and use atomic file replacement where supported.

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

## Reconciliation
<Every original requirement ID, outcome, evidence and any approved deferral.>
```

Use planned, in_progress, blocked, verified, deployed, or deferred status as appropriate. A non-deployed task can finish at verified if deployment is outside scope. Update after claims, meaningful progress, failures, handoffs, verification, each successful deployment/rollback, and cleanup. Keep release history and unresolved evidence references; remove redundant narration instead of appending a diary. CONTEXT.md remains short resumption memory, not a second task list.

If an existing Beads tracker becomes unavailable, reconstruct local tasks from the latest accessible evidence and preserve original IDs. Mark unknown states explicitly, retain the original data, and do not repeat external actions to infer their outcome. Genuine uncertainty about an operation may require inspection; absence of these four dependencies alone does not block development.

Do not switch back just because Beads is later installed. On explicit selection of Beads, pause tracker writes, snapshot the local file, transfer tasks/IDs or record an ID mapping, dependencies, failures, and evidence, and verify coverage before changing the active mode. Mark the old file as an archived snapshot with a pointer to Beads. If transfer fails, keep local mode active. Apply the same reconciliation when leaving an accessible Beads tracker. Never maintain two writable authorities or destroy original records.

## Small resumption checkpoints

Aim for a few hundred words per agent's CONTEXT.md, normally below 600. Replace stale notes rather than appending a diary:

```
Updated: <time>; agent: <owner>
Location: <checkout/branch/revision>; uncommitted changes: <summary>
Tasks: <task IDs, tracker mode, and absolute canonical location>
Decisions: <essential facts; durable doc links>
Lessons: <absolute MISTAKES.md path; relevant IDs and revision>
Evidence: <result locations and matching revisions>
Pending: <running process/deployment identity, blocker or handoff>
Next: <one executable action>
```

Include authorization/target pointers for release resumption. Never store secrets, credentials, raw personal data, or private reasoning traces. Keep local checkpoints out of product commits unless intentionally tracked, and preserve them before checkout cleanup. Give read-only agents unique paths.

Checkpoint at milestones, before handoff, when context pressure is signaled, and before requested compaction. Exact warnings are not guaranteed. After interruption, read the checkpoint, active tasks, current Git state, and in-flight operation status. Old memory and task labels are not proof of success. Reconstruct missing memory from evidence rather than guessing or repeating deployment.

Store enduring conventions in existing project docs and use one canonical design-system source. Use the Pro [shared mistakes procedure](mistakes-memory.md) to record confirmed agent mistakes and prevention actions in the main checkout's `MISTAKES.md`. Keep task-specific attempts in the active tracker. Link each lesson to its issue; do not duplicate failure logs across memories.

## Evidence-driven failures

Create one issue per distinct meaningful failure, link affected work, and deduplicate by symptom/cause where known. Record category (implementation, verification, environment, deployment, permission, tracking), symptom/reproduction, revision/environment, evidence pointer, and owner. For each attempt append hypothesis, material change, result, learning, and next action. Close with resolution proof.

Do not issue-track routine typos, expected empty searches, or harmless transient commands. Append evidence to an existing failure instead of creating tickets per retry.

Retry only with new evidence or a changed approach. After two attempts without useful progress, the orchestrator reassesses and may assign the complex developer; after another unsuccessful approach, stop that loop and ask one focused question or report the external blocker. Continue independent unblocked work. A transient failure may justify one bounded retry; inspect ambiguous side effects first. Never erase failing tests, waive required checks, or mark blocked work complete to advance.

Reference: [Beads documentation](https://github.com/gastownhall/beads). Installed-version behavior controls implementation.
