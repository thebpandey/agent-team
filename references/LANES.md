# Lane protocol

This is the single detailed protocol for Agent-Team lanes. Other references describe their integration boundary and link here instead of defining another lane state machine. The canonical tracker remains the only task authority. A lane queue is immutable admitted membership and ordering, not a claim on every future task.

## Operating invariant

One lane has one retained worker, one registered team, one `lane/<lane-id>` branch, one worktree, and one ordered queue. Its writable paths are exact and disjoint from other live writers. The orchestrator alone creates lanes, advances their task claims, dispatches or continues native workers, rotates workers, integrates accepted revisions, and closes lanes.

Lane roles reuse the native role IDs `project_orchestrator`, `complex_developer`, `developer`, `routine_developer`, `reviewer`, and `visual_reviewer`. Lanes do not rename roles or introduce a second role catalog.

The durable lane record contains its identity, role, effective model and effort, full queue, current task, worker generation, worktree and branch, brief pointer and hash, ownership evidence, rotation count, handover pointer, fact sheets, and immutable assignment and result history. Progress is derived from tracker and accepted integration evidence. Do not add lane-local task status that can contradict the tracker.

Before lane creation, a delegated Graphify affected/path check records the owned paths, exact revision, path-set hash, and absence or approved resolution of overlaps. Missing or stale ownership evidence is unresolved. Resolve an overlap through one writer or an explicit per-task split before dispatch.

## Route contract

Every native mutation file is the closed envelope `{schemaVersion:1, actorSessionId, expectedVersion, request}` and requires the current native project owner. Each closed request has its own `schemaVersion:1`, `operationId`, `reason`, and phase-specific preconditions. CLI text does not create native identity. Commands are exact and unchained.

- `lane-create` binds an already prepared worktree and registered team. It records the full queue but claims no task. Its request binds tracker, run, revision, lanes-collection fingerprint, and the new lane. One open reserved reviewer lane may overlap author task IDs only when every queued task belongs to exactly one non-review lane; it is excluded from logical author occupancy.
- `lane-next` has exactly four phases: `prepare`, `dispatch`, `result`, and `bind`.
  - Ordinary `prepare` binds assignment ID and attempt, tracker, run, revision, lane fingerprint, packet path/hash, and fact-sheet citations. It claims only the first eligible unassigned queue task and seals the assignment and NEXT-TASK packet. Packet preparation is not dispatch proof.
  - Reserved reviewer `prepare` adds exact `sourceAssignment` lane, assignment, packet hash, and revision binding. It requires passed author worker and verification results at that revision, requires empty decisions and fact sheets, and performs no tracker claim.
  - `dispatch` preconditions are lane fingerprint, revision, and packet hash; the request also binds the assignment, worker, and observation. It records a coordinator-observed native host receipt or explicit `UNKNOWN` and never spawns a worker.
  - `result` preconditions are lane fingerprint and packet hash; the request binds the assignment and sealed result. Every actual dispatch receives a result or unresolved disposition. `UNKNOWN` or unresolved outcomes remain unresolved. A definite failed worker outcome settles that invocation and permits attempt plus one without reclaiming the retained tracker task. A failed review similarly settles the review attempt and may retry as attempt plus one, but grants no author review or gate authority. Only a passed review from the registered reviewer lane, with observed dispatch and source-first challenge evidence, can atomically link to the matching author history. A lane result never grants completion, integration, release, or tracker authority.
  - `bind` preconditions are lane fingerprint, revision, and handover hash; it attaches a pre-registered generation plus one replacement after stopped-writer rotation. It has no assignment ID and does not reclaim or advance the current task.
- `lane-rotate` binds lane fingerprint, revision, trigger, outgoing worker, nullable replacement, handover, and stopped-writer or transfer evidence. It preserves the current claim, worktree, queue, and history. Trustworthy stopped-writer evidence sets worker null and status `rotation_required`; owner-bound explicit transfer may prebind a registered replacement.
- `lane-close` binds lane, tracker, run, revision, and `writerRelease`. `writerRelease` is null only when the lane worker is already null. Otherwise an owner-authorized sealed `explicit_release` evidence record must match the lane, worker, revision, owner epoch, reason, and digest; successful close verifies it and clears the worker. An author lane requires exhausted queue progress, resolved history, verifier-first evidence, independent review, and accepted canonical integration for every queued task. A reviewer lane requires every queued review to pass and each source assignment to have accepted integration. Close retains the worktree and evidence for separate cleanup checks.

Interrupted `prepare` replays the same persisted assignment intent and tracker transition. Never mint a second attempt because a dispatch or response is ambiguous. Assignment history is append-only. Scope changes are new amendments. Decision changes use stable IDs: a reversal records a new decision with `supersedes`, returns to the orchestrator, and never edits the earlier decision.

## Files and bounded context

Canonical runtime lane documents use uppercase basenames under `.agent-team/lanes/<lane-id>/`. Preserve previously recorded lowercase paths during recovery.

| File | Contract |
| --- | --- |
| [BRIEF.md](../assets/templates/lanes/BRIEF.md) | Shared lane context: role/model/effort, full queue, shared rules, exact writable paths, required skill paths and SHA-256 hashes, applicable instruction paths and SHA-256 hashes, context revision, evidence destination, and handoff format. Default enforced cap: 6000 words. |
| [NEXT-TASK.md](../assets/templates/lanes/NEXT-TASK.md) | Immutable per-task delta at `packets/<TASK-ID>-<ATTEMPT>.md`. Target fewer than 400 tokens and never repeat the brief. The production parser enforces at most 400 whitespace-delimited words. |
| [HANDOVER.md](../assets/templates/lanes/HANDOVER.md) | Rotation context at `HANDOVER-001.md` and increasing sequences. Target fewer than 500 words; the production validator enforces at most 500 words. |

Use paths and hashes instead of content dumps. Full logs and result evidence remain in retained evidence storage outside disposable worktrees. A fresh worker reads the complete applicable project instructions and required skills. A retained worker may reuse context only while the brief hash, instruction hashes, relevant requirements, paths, and context revision still match.

Worker updates are separate receipts containing task, revision, evidence pointer, named checks, and one next action. The default maximum is 2000 characters. Supported file-edit and append hooks validate the resulting update size. A native message or opaque shell write that the host does not expose cannot be universally intercepted, so do not claim that every transport is enforced.

## Verification and integration

The quality route is developer handoff, delegated mechanical verifier, independent reviewer, repair, completion gate, and serial accepted integration. The delegated verifier runs before independent review. Mechanical failure returns to the developer before reviewer dispatch. Reviewer identity must differ from the assignment worker and a retained reviewer cannot review their own authored task.

Review consumes the exact assignment, source/diff, verifier evidence, rubric, and concrete failure scenarios. Developer narrative and prepared packets are not proof. Each task keeps its revision-bound worker result, verifier result, independent review, canonical gate evidence, and accepted integration. Only canonical accepted delivery evidence changes integration progress.

Keep each task claim and implementation commit task-specific and revision-bound. Integrate one accepted revision at a time. A lane advances only after the previous task has accepted completion and integration evidence. Alternating failed repairs use the existing two-attempt diagnostic rule; it does not waive acceptance, checks, review, or exact revision binding.

Shared fact sheets have one owner lane, content hash, consumer citations, and facts with `checkedOn` and `sourceUrl`. The default expiry is seven days. Changed requirements or sources can make a fact stale sooner. Consumers recheck stale facts instead of copying an unverified conclusion.

## Capacity and supervision

Open lanes occupy logical capacity between tasks. Native reviewer slots and logical `parallel_teams` are different quantities and must not be double counted. Unknown liveness never frees a slot. Rotation status alone never proves a writer stopped. Release compute only after authentic stopped-writer evidence or an explicit ownership transfer.

Normal wakeups are user input, worker messages, completions, handovers, reviews, and provider results. The configured heartbeat is an active-host-turn fallback, default 600 seconds with a minimum of 60. Use the shorter host wait limit when required, but a returned wait is not evidence of progress and does not justify a manufactured poll.

Do not use sleep-and-check loops. The heartbeat creates no process, daemon, timer, scheduled activity, hosted monitor, or work after the host turn ends. Lower intervals consume more orchestrator turns and do not inherently produce progress.

## Settings, context, and models

Effective settings are independent per host and snapshotted on a qualified run:

- `lanes.enabled=true`
- `lanes.rotation.tasks=2`
- `lanes.rotation.onPressure=true`
- `lanes.factSheetStaleDays=7`
- `lanes.workerUpdateMaxChars=2000`
- `lanes.briefMaxWords=6000`
- `supervision.heartbeatSeconds=600`, minimum 60
- `limits.subprocessMaxBufferBytes=2097152`
- `limits.maxPlanTasks=1000`
- `limits.canonicalRecordMaxBytes=16777216`

Rotate after two completed tasks by default or earlier under context pressure, using a safe mid-task checkpoint when necessary. Settings changes preserve unrelated roles, run defaults, custom fields, and the other host. A future settings edit does not rewrite an existing run snapshot.

Model routing remains quality-first and host-aware. A skill cannot change its parent model. For Claude Code, recommend an available Opus execution session when the actual catalog confirms it and preserve independent review. For Codex, use an explicitly detected catalog model and supported effort. Planning may justify a different available tier. Recommend a model above Opus only when the actual host catalog provides a genuinely comparable detected model; otherwise the comparison remains unknown. Never invent cross-vendor ordering or silently substitute a weaker reviewer.

Context reduction is owner-approved only. Visibility does not prove a dependency is unused. Present the exact selected project settings, never auto-disable tools, store reversible receipts, and refuse drift on revert. Managed helpers install with hashes and self-checks; they prepare evidence and fresh requests but grant no authority. Read large records through bounded single-record reads.

## Host continuity and recovery

For Codex, sending a message to an active retained agent and assigning follow-up work to an idle retained agent are distinct host controls. Use the control that matches observed state. For Claude Code, use retained-worker messaging or resume only when the current host exposes and verifies that capability. Otherwise rotate or use a fresh worker with the complete applicable instructions and exact recovery pointers.

Use one execution session per planning, build, and release phase, joined by compact recovery-index handoffs. Reattach only when native worker generation, brief hash, and current revision context all match. A stopped observation may authorize rotation. Missing, mismatched, or unknown observation remains occupied and requires exact stop or transfer evidence.

Historical session UUID locking is not a prerequisite for editing unclaimed project files. Project registration, audit history, registered-worker path restrictions, tracker authority, native workflow identity, and revision-bound gates still apply. User-directed coordinator maintenance records liveness as not asserted; it is not proof the old worker died, native authentication, task admission, integration approval, or release authority.

Recovery reconciles pending operations before new dispatch. Cleanup is separate from integration and release: it first requires a closed integrated lane, then preserves all task-level stopped-writer, exact-revision, dirty/untracked/ignored-file, preview, retained-evidence, and no-force checks. Explicit maintenance never weakens those gates.

Token savings are estimates unless measured by `usage.mjs`. Reuse valid retained context, brief hashes, packets, fact sheets, and recovery pointers without inventing a savings percentage.
