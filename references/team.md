# Team dispatch and isolated work

The project orchestrator alone admits tasks, assigns workers, integrates revisions and publishes. Use the host's real agent controls and actual concurrency limit. A named group sharing one parent is not an independent full session. Developers, reviewers and testers do not spawn children.

Use one developer and one independent reviewer for substantive work. Add developers only for independent writable work and preserve reviewer capacity. Reuse appropriate idle workers for coherent repair; a new context starts at a meaningful safe boundary, not an arbitrary turn count.

## Assignment packet

Supply only the task's necessary context:

- Stable project/task/team and attempt IDs; canonical tracker and integration branch.
- Observable acceptance, approved scope, constraints, rejected alternatives and genuine approval gates.
- Exact input revision, exclusive writable paths, worktree and resource ownership.
- Actual host, role, effective model/effort and approved escalation/fallback.
- Applicable skill/reference paths and prepared capability evidence; read [selective startup](dependencies.md#skill-startup-for-every-agent).
- Existing check commands, relevant lessons and evidence destination.
- Pending operation or uncertainty, if any, and one next action.

Do not copy the full conversation, entire audit or unrelated skills. A fresh worker reads complete applicable instructions in its own context. A parent receipt is not a child read. Discovery, instruction loading, functional capability and actual task use are different facts. Report exceptions once; do not repeat a four-skill matrix every turn.

## Ownership

Follow [projects](projects.md). Use a separate worktree for independent implementation; one editor per shared entrypoint/schema/manifest. Isolation does not isolate ports, databases or credentials. Allocate separate external resources only when needed and authorized.

The selected tracker is the only task authority. In Markdown mode only the project owner writes it; in Beads mode use the verified supported atomic/single-writer contract. Workers send concise task-ID/revision/evidence updates. CONTEXT.md and handoff files are recovery pointers, not another queue.

Reviewers inspect a stable exact revision or complete WIP snapshot including staged, unstaged and untracked intended changes. Review requirements and quality in one existing review loop. Return deduplicated findings with severity, location, impact and verification. Implementation-owner self-review is not independent review.

## Repair and handoff

Assign ordinary findings back automatically. Diagnose repeated failure, change strategy or apply an approved escalation; do not ask the user to authorize routine repair again. Stop conflicting writes and park external blockers with a recovery condition while independent work continues. Never mark failed acceptance complete to free a slot.

Before rotating a worker, save its authored decisions and pending operation facts, retain evidence outside disposable worktrees, and prove the previous writer stopped or transferred ownership. Unknown liveness is not takeover permission. See [recovery](recovery.md).

Return outcome, exact commit/revision, acceptance and check evidence, unresolved findings and next action. Link detail rather than dumping logs or images into parent context. Only the orchestrator integrates and closes the task after affected combined checks; deployment is a separate state.

Each worker owns only its assigned context path. Read-only reviewers use a unique evidence/context path; they do not rewrite shared records. Preserve pre-existing meaningful content.
