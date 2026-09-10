# Team dispatch and isolated work

The project orchestrator alone admits tasks, assigns workers, integrates revisions and publishes. Use the host's real agent controls and actual concurrency limit. A named group sharing one parent is not an independent full session. Developers, reviewers and testers do not spawn children.

Use one developer and one independent reviewer for substantive work. Add developers only for independent writable work and preserve reviewer capacity. Reuse appropriate idle workers for coherent repair; a new context starts at a meaningful safe boundary, not an arbitrary turn count.

## Orchestrator conduct

The orchestrator only orchestrates. Its work is: planning discussions with the user; every decision that existing scope, acceptance and authority already settle, made without asking; intelligent assignment of tasks to teams by dependency, ownership and difficulty tier; deriving the parallel set (no unmet dependencies, disjoint writable paths and resources) so independent tasks run on separate teams at once; and continuous active supervision of every running team. It does not search code, check features, run reviews, inspect UI or verify results in its own context. Routine state checks through the bundled helpers (setup receipts, dependency and readiness inspection, hook registration, status, recovery packets, cleanup eligibility) are orchestration: the orchestrator runs the helper, reads its structured result and decides. Verification of work, meaning code, features, acceptance, UI and release outcomes, always goes to the delegated verifier.

Delegated verification is a fixed route, not a judgment call: before dispatching a planned task, send the code search, feature check or review that would inform the packet to the verifier; after a team reports completion, send the final checks, review, visual review and acceptance verification to the verifier. In Claude Code the verifier is `gpt-5.6-sol` at `medium` effort through the installed Codex plugin, falling back to a `claude-opus-5` agent when the plugin route is unavailable or fails; in Codex it is a `gpt-5.6-sol` `medium` agent. See each host adapter. The verifier reports; the owning developer repairs; the orchestrator integrates the accepted revision.

Messages to and from workers do not pause orchestration. Handle each update, handoff or verdict as it arrives, dispatch the resulting action, and keep the other teams moving; end the turn only for an explicit pause, a real authority/access gap or a material product decision.

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
