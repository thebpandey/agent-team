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

Orchestration is continuous while any team is active. Each worker update, handoff, verifier verdict or completion notification is handled as soon as it arrives: record the fact, decide the next action (repair, verify, integrate, refill, park) and dispatch it, then return to supervising the other teams. Do not pause the run, wait for a user prompt or end the turn because a message was sent to or received from a worker; stop only for an explicit pause, a genuine authority/access gap or a material product decision.

## Finite and continuous scope

A finite start fixes the admitted delivery set after initial selection and finishes that set without adding replacements. A named start selects one existing task and rejects count/continuous modifiers. A natural-language feature request may create a deduplicated canonical task once its scope and acceptance are sufficient.

Continuous mode refills available capacity from the authorized task-list scope after verified integration or safe parking. Do not wait for unrelated teams or a deployment batch. Retry ordinary findings automatically with diagnosis and approved escalation; a failed check blocks completion, not repair.

When no eligible work exists but an active task can unblock it, keep progressing that work. When only genuine blockers remain, checkpoint/park them and report the exact external action or resume condition; do not ask for a generic continue instruction. Never invent work or waive acceptance to keep slots occupied.

Automatically resume a parked external blocker only when its prerequisite is demonstrably restored and existing run scope/authority permits it. Explicit user pauses still need explicit resume. Use stable ordering/fairness so a repeatedly failing task cannot starve other eligible work.

## Durable run records

Keep run identity/owner, authorized scope, effective defaults/overrides, membership, active/parked state, holds, pending operations and integration/release evidence in existing canonical records. Setup holds defaults, not current task status. Checkpoints carry pointers, not another queue.

Record claims before dispatch and verified integration before completion. Exact delivery IDs and revisions define release batches; commits, retries and subtasks are not batch counts. Retain in-flight operation identity and uncertainty across interruption.

A second start identifies an existing active run; it does not create a competing consumer or silently resume explicitly paused work. A clear active-run change is run-only unless the user asks to save defaults. Future saved settings do not alter an existing run.

## Pause, recovery and release

Project pause holds admission, integration and automatic release before checkpoint requests, including between tasks. Observe already-submitted side effects safely; do not start another batch. A named pause affects only its task and keeps its claim/gates. Other authorized work may use verified freed capacity.

Bare pause/resume retains the explicit target picker in [actions](actions.md). A named resume does not clear a project-wide hold. Resume validates ownership, current files, evidence and pending operations before continuation; see [recovery](recovery.md).

Deployment and cleanup are separate from task admission. With deployment off, finish the authorized integrated result without requiring a deployment decision. When automatic deployment is enabled, use [release](release.md) gates and exact task batches, including eligible final smaller batches when the run ends. A release failure holds affected releases, not unrelated safe development. Never report integrated-only work as production-verified.
