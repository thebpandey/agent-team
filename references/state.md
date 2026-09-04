# Tasks, memory, and failures

## One authoritative task graph

Use Beads in every project. Discover installed-version help; do not assume storage backends, commands, or custom statuses. Use native states plus a short phase field/note for planned, implementing, verifying, ready to deploy, deployed, or blocked if needed. Keep requirement-to-task/evidence mappings in the parent task, not HARNESS_STATE.md or a separate task checklist.

Astra creates/deduplicates tasks/issues, manages dependencies/ownership, integrates evidence, and closes/reopens tasks. Teammates update only assigned progress and append evidence. Confirm the backend supports concurrent writes before enabling them. If single-writer, serialize teammate-authored updates through Astra; do not start competing database writers. Use supported persistence/sync at handoff and release.

Task evidence needs only requirement IDs, acceptance criteria, owner/dependencies, current phase, changed revision, relevant check/environment identity, result/evidence link, and next action. Link logs/screenshots instead of inserting them. Track unrelated discoveries separately without automatically expanding scope.

## Small resumption checkpoints

Aim for a few hundred words per agent's CONTEXT.md, normally below 600. Replace stale notes rather than appending a diary:

```
Updated: <time>; agent: <owner>
Location: <checkout/branch/revision>; uncommitted changes: <summary>
Tasks: <Beads IDs and canonical tracker location>
Decisions: <essential facts; durable doc links>
Evidence: <result locations and matching revisions>
Pending: <running process/deployment identity, blocker or handoff>
Next: <one executable action>
```

Include authorization/target pointers for release resumption. Never store secrets, credentials, raw personal data, or private reasoning traces. Keep local checkpoints out of product commits unless intentionally tracked, and preserve them before checkout cleanup. Give read-only agents unique paths.

Checkpoint at milestones, before handoff, when context pressure is signaled, and before requested compaction. Exact warnings are not guaranteed. After interruption, read the checkpoint, Beads tasks, current Git state, and in-flight operation status. Old memory and task labels are not proof of success. Reconstruct missing memory from evidence rather than guessing or repeating deployment.

Store enduring conventions in existing project docs and use one canonical design-system source. Promote solved failures into durable guidance only for actionable lessons likely to recur. Keep task-specific attempts in Beads; do not create a global failure diary or duplicate lessons across memories.

## Evidence-driven failures

Create one issue per distinct meaningful failure, link affected work, and deduplicate by symptom/cause where known. Record category (implementation, verification, environment, deployment, permission, tracking), symptom/reproduction, revision/environment, evidence pointer, and owner. For each attempt append hypothesis, material change, result, learning, and next action. Close with resolution proof.

Do not issue-track routine typos, expected empty searches, or harmless transient commands. Append evidence to an existing failure instead of creating tickets per retry.

Retry only with new evidence or a changed approach. After two attempts without useful progress, Astra reassesses and may assign Sol; after another unsuccessful approach, stop that loop and ask one focused question or report the external blocker. Continue independent unblocked work. A transient failure may justify one bounded retry; inspect ambiguous side effects first. Never erase failing tests, waive required checks, or mark blocked work complete to advance.

Reference: [Beads documentation](https://github.com/gastownhall/beads). Installed-version behavior controls implementation.
