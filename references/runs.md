# Pro bounded and continuous runs

The project orchestrator owns one active project scheduling run. Use the existing canonical tracker for run state and task evidence, [settings](settings.md) for defaults, and [project coordination](projects.md) for exclusive claims and serial integration. A run is an instruction to the current host, not a daemon or a promise to keep working after the session closes.

## Select tasks and count slots

`start N` admits a fixed set of up to N safe, ready, unassigned top-level tasks. N is an integer from 1 to 6. A top-level task here is one independently deliverable feature or work item selected from the configured tracker, identified before dispatch. It may contain subtasks. Do not select a summary epic, count each child as another delivery, or renumber existing Beads IDs. Preserve existing task hierarchies; record the selected delivery ID and its child mapping.

Select by recorded priority/order, breaking ties by stable task ID. Check dependencies, overlapping writes, shared interfaces, services, and host capacity. Prefer independent tasks. Recheck eligibility and ownership under the project owner's exclusive claim before each admission. Start fewer teams when fewer tasks are safe; report the reason and actual count. Never invent filler work, weaken gates, or promise six independent sessions when the host cannot supply them.

Count every project team whose selected task has not been verified and successfully integrated into main against the limit, including existing teams outside this invocation. Implementation, review, waiting for integration, preview approval, paused, blocked, interrupted, and unknown ownership all occupy slots. A team frees its slot only after its selected task and required subtasks pass feature and combined checks, required preview approval is satisfied, and the exact result is integrated into main. A reviewer report, feature commit, partial child completion, or idle agent is not enough. Integrated tasks awaiting deployment/cleanup retain their records and resources but no longer occupy development slots. Never exceed six occupied development slots in this project; a lower configured or host-supported limit also applies.

Without continuous mode, freeze the admitted task IDs once initial selection completes. Finish that set without replacements or later filling unused places. Report verified/integrated tasks, blockers, deployment state, and cleanup retention. With auto-deploy off, ask whether to deploy the integrated result. `start` with no saved overrides preserves the one-task behavior.

## Refill in continuous mode

`start continuous` uses one slot unless settings specify another count. `start N continuous` keeps up to N occupied development slots. After each successful integration into main, immediately admit the next safe ready task into a free slot. Do not wait for the other teams or a deployment batch. Several newly free slots may admit several tasks, up to the limit. Only the project orchestrator admits new teams; feature members do not spawn replacements.

Record the configured task-list scope before starting. Continue through its authorized ready tasks and tasks that become ready as dependencies finish. Do not expand into another project, unrelated discoveries, deferred work, or newly invented requirements. Use supported completion events or bounded waits while admitted work can progress. Recheck the queue on meaningful progress, not in a busy polling loop.

If no safe task is currently ready but active work can unblock it, continue that work and reassess. If the scoped list is exhausted and admitted development is finished, finish the run after handling its pending releases. If all remaining work is blocked and none can progress, mark the run Blocked, stop automatic admission, handle any eligible final smaller batch, and report the blockers. Preview approval, paused teams, and unavailable ownership are blockers, never permission to free their slots. An explicit project pause suppresses even a final automatic batch. A later resume inspects actual state before continuing.

Integration frees capacity independently of deployment success. A release failure holds automatic deployment. Continue independent development only where the diagnosed failure does not affect its safety or shared integration assumptions; pause affected work. Follow the bounded recovery and retry rules in [release](release.md). At terminal completion, report task outcomes and deployed batches separately. Integrated-only work is not delivered when production is required.

## One owner and recoverable state

Keep these fields in a run section/record of the active tracker, using installed Beads capabilities or the canonical local TASKS.md. They are not a second task list:

| Record | Required content |
| --- | --- |
| Run identity | Run ID, project owner/session, source command, authorized task-list scope, fixed admitted set or continuous membership links |
| Effective choices | Team limit, continuous flag, auto-deploy flag, batch size, preview requirement, settings revision/source, and explicit run overrides |
| Skill confirmation | Each Team Orchestrator's current-run report covering itself and every teammate for Ponytail, Using-Superpowers, Impeccable, and LeanCTX; loading/use evidence pointers, exceptions, and Project Orchestrator acknowledgment. Follow [per-run confirmation](dependencies.md#per-run-confirmation-to-the-project-orchestrator). |
| Scheduling state | Running / Pause requested / Paused / Blocked / Finished; reason, occupancy derived from team/task evidence, next action |
| Deployment state | Enabled / Off / Held after failure; target/authority pointer, pending delivery IDs in integration order, release-boundary commits, in-flight batch ID/artifact/provider identity |
| Results | Integrated revisions, deployed task IDs and batch evidence, unresolved blockers, cleanup pointers |

Record task claims and membership before dispatch. Record verified integration and its release boundary before freeing a slot or selecting a deployment batch. Each batch records exact delivery IDs and its source revision before submission. Only successful live verification records those IDs as deployed; inspect provider evidence after interruption before retrying. Reuse evidence and membership on resume rather than counting commits or resetting counters.

A second start while a scheduling run is active identifies that run and does not create a competing queue consumer or silently replace its choices. A clear user instruction to change the active run is handled by its owner as a run-only override; ambiguous scope needs one question. Existing standalone teams keep their ownership and gates and count toward available capacity. A new run must preserve undeployed integrated task evidence; [release](release.md) determines whether it can be included in a batch.

## Pause and resume

`pause all` stops new admissions/refill, integration, and automatic batch submission for the project run before requesting team checkpoints. It applies even between tasks or when zero development teams remain but a run/release is active. All selected in the bare pause picker also holds that run and the offered team set. Preserve in-flight operations for safe observation and report partial pauses. A named pause holds only that team; its occupied slot cannot be replaced, while other slots may continue.

`resume all`, or All selected in the resume picker, restores the run after ownership and release evidence checks, including runs paused between tasks or stopped as Blocked. Recheck blockers; only resolved work proceeds. A named resume restores only that team and does not clear a project-wide scheduling/release hold. If there are no eligible teams but a paused/interrupted or stopped Blocked run exists, bare resume offers that run with its identity/state and All; bare pause similarly offers an active run with no eligible teams. Do not silently act on an empty picker. Status and help never resume or refill work. See [recovery](recovery.md).
