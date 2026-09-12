# Bounded and continuous runs

One project orchestrator owns scheduling through the canonical tracker. A run is an instruction to the current host, not a daemon or a promise of execution after session closure. Reuse the canonical integration branch, including an approved develop branch; do not assume main.

## Eligibility and capacity

Resolve saved [settings](settings.md) and explicit run-only modifiers before claims. Built-ins start one task, with continuous mode and automatic deployment off. Requested team counts are 1–6; actual native slots, review capacity and independent writable work may reduce concurrency.

Read complete eligible records from the selected Beads or Markdown authority. Select authorized, actionable, ready, unclaimed work by priority/order and stable ID. Preserve native IDs/hierarchy. A summary epic is not another delivery when its children represent the work. A proposed or deferred improvement is not eligible merely because it was recorded.

Claim atomically using the backend's supported operation or verified single-writer control. Recheck dependencies, scope, owner/version and resource conflicts under that control. Use idempotency identities for repeated requests. Unknown tracker state or writer liveness cannot authorize a new claim or takeover.

Distinguish logical task ownership from compute occupancy:

- Active implementation/review/integration consumes its actual host resources.
- External blockers may be parked after saving evidence and proving the writer stopped; keep the claim, worktree, gates and resume condition while releasing compute.
- Explicitly paused work stays paused; freeing its stopped compute does not resume it.
- Unknown/disconnected activity is not proven free capacity.
- Integrated work retains release/evidence records but does not occupy developer capacity solely while waiting for deployment.

Reserve capacity for independent review. Reuse a shared reviewer when appropriate; do not spawn a full extra hierarchy per task. Only the project orchestrator admits replacements. Keep scheduling event-driven with bounded waits, not repeated full-queue polling.

Orchestration is continuous while any team is active. Follow the active host-turn loop below; do not create a second schedule.

1. Consume every new user message, worker update, completion, handoff, review verdict, and provider result.
2. Classify user input as replacement, compatible addition, or status/question. Checkpoint and stop only conflicting work for a replacement; reconcile an addition against dependencies, ownership, and conflicts.
3. Answer a status/question in commentary. It is an interrupt, not pause, cancel, ownership loss, or a terminal condition.
4. Reconcile every live worker and completed handoff against canonical state.
5. Route exact revisions to repair or independent review, and serially integrate only accepted work.
6. Prove a writer stopped or ownership transferred before compute becomes free. Unknown liveness is occupied and must not create a free slot.
7. Recompute actual capacity, reserve reviewer capacity, and admit the next authorized eligible disjoint task.
8. While the host turn has active workers, wait no more than 60 seconds. If nothing changed, emit one compact heartbeat from known state/blocker facts, reconcile, and repeat.

Thus commentary answer, reconcile, review/integrate/repair, refill, and bounded wait are one mandatory continuation. Completion does not wait for the original cohort, a release batch, deployment, or another `continue`. A scoped blocker is reported and retained while independent work continues; an unchanged blocker stays in the heartbeat without a novelty probe. No cron, daemon, timer, hosted monitor, nested scheduler, recurring job, or activity after the final response/host interruption implements this loop. Ordinary heartbeat model/output cost is real but its exact incremental amount is unknown.

## Finite and continuous scope

A finite start fixes the admitted delivery set after initial selection and finishes that set without adding replacements. A named start selects one existing task and rejects count/continuous modifiers. A natural-language feature request may create a deduplicated canonical task once its scope and acceptance are sufficient.

Continuous mode refills available capacity from the authorized task-list scope after verified integration or safe parking. Do not wait for unrelated teams or a deployment batch. Retry ordinary findings automatically with diagnosis and approved escalation; a failed check blocks completion, not repair.

When no eligible work exists but an active task can unblock it, keep progressing that work. When only genuine blockers remain, checkpoint/park them and report the exact external action or resume condition; do not ask for a generic continue instruction. Never invent work or waive acceptance to keep slots occupied.

Automatically resume a parked external blocker only when its prerequisite is demonstrably restored and existing run scope/authority permits it. Explicit user pauses still need explicit resume. Use stable ordering/fairness so a repeatedly failing task cannot starve other eligible work.

## Effective run decision

Consume the read-only `run-decision` result. Its classifications are exactly `unknown`, `paused`, `unreconciled_completion`, `progress_possible`, `finite_exhausted`, `continuous_scope_exhausted`, and `blocked_tail`. `progress_possible` means continue/refill. `unknown`, `paused`, and `unreconciled_completion` never select a release. Only the three terminal classifications permit a nonempty underfilled final batch, and cardinality never waives review, checks, preview, target, authority, recovery, or exact boundary evidence.

Count unique top-level delivery IDs, not commits, subtasks, checks, agents, or prose. Join task-keyed source, integration, review, checks, preview, target, authority, and recovery evidence for the exact current selected set. Work outside `run.taskIds` never widens a continuous run; extension is an owner-only additive versioned `run-scope-extend` transition.

## Durable run records

Keep run identity/owner, authorized scope, effective defaults/overrides, membership, active/parked state, holds, pending operations and integration/release evidence in existing canonical records. Setup holds defaults, not current task status. Checkpoints carry pointers, not another queue.

Record claims before dispatch and verified integration before completion. Exact delivery IDs and revisions define release batches; commits, retries and subtasks are not batch counts. Retain in-flight operation identity and uncertainty across interruption.

A second start identifies an existing active run; it does not create a competing consumer or silently resume explicitly paused work. A clear active-run change is run-only unless the user asks to save defaults. Future saved settings do not alter an existing run.

## Pause, recovery and release

Project pause holds admission, integration and automatic release before checkpoint requests, including between tasks. Observe already-submitted side effects safely; do not start another batch. A named pause affects only its task and keeps its claim/gates. Other authorized work may use verified freed capacity.

Bare pause/resume retains the explicit target picker in [actions](actions.md). A named resume does not clear a project-wide hold. Resume validates ownership, current files, evidence and pending operations before continuation; see [recovery](recovery.md).

Deployment and cleanup are separate from task admission. With deployment off, finish the authorized integrated result without requiring a deployment decision. When automatic deployment is enabled, use [release](release.md) gates and exact task batches, including eligible final smaller batches when the run ends. A release failure holds affected releases, not unrelated safe development. Never report integrated-only work as production-verified.
