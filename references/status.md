# Pro non-disruptive status

Status and reports show role labels and stable task/team IDs first. Friendly names are optional. Show configured model/effort when useful, with recorded versus actually enforced values distinguished.

A status request reads recorded state. It does not change execution. Run this path before setup, dependency/model checks, recovery, or dispatch. Use the [action](actions.md) target rules. The project overview includes all project tasks and names unassigned work; a team view includes only its mapped tasks.

Use compact state-first output without a wordmark or fixed-width border. Start with active/parked/ready work, actual capacity, overall progress, release state and any necessary user action.

Read the canonical team directory, active tracker, and already available evidence through a supported read-only query. In Beads, discover installed help only if needed and use a read-only listing of the complete requested scope, including completed tasks and all pages. Never run a command that initializes, migrates, syncs, claims, or repairs the store merely to report status. If read access is unavailable, use an existing timestamped snapshot or report unavailable data; do not create a fallback tracker during status.

Do not interrupt, message, poll for fresh replies from, or wait on agents. Do not start tests, servers, installs, recovery, cleanup, integration, or deployment. Do not write task records or refresh checkpoints. Do not acquire a lock that blocks writers. Use a coherent tracker snapshot where available; if records change during reading, label the report approximate and keep it bounded rather than repeatedly polling.

When status steers an active development session, answer briefly and continue that session's existing task without waiting for another message. A standalone status-only session returns the report and ends. Subagents continue where the host supports background execution. The orchestrator may briefly use a turn to answer; do not promise host-independent simultaneous execution or claim that stopped sessions are running.

## Count once and state the scope

Count unique actionable task IDs in the requested scope. Exclude summary epics/parents whose children are counted; include a parent only when it has distinct actionable work not represented by children. Deduplicate tasks shared as dependencies across teams. The owner team's row counts the task; dependent teams link it without counting it again. Project totals use the union of IDs, not a blind sum of overlapping views.

Exclude cancelled tasks and explicitly approved deferrals from the denominator; show their counts separately. Unapproved deferrals remain unfinished. Report the scope, source, read time, source update time when known, and any exclusions. Show changes in scope when a previous snapshot is available without creating one just for status.

Map native tracker states plus recorded phase/evidence to these disjoint categories. Do not assume a particular Beads version's status names:

| Category | Meaning |
| --- | --- |
| Completed | Acceptance criteria satisfied and completion recorded; required approval is satisfied if part of those criteria. A cancelled/waived/duplicate closure is not completed work. |
| In progress | Assigned implementation or verification is underway, according to the recorded state. This is not proof the agent process is currently alive. |
| Not started | Work has not begun and is not blocked. |
| Blocked | Unfinished work waiting on a dependency, access, required approval, or an intentional pause. Show the reason. |
| Unknown | Status cannot be mapped or confirmed from the available record. Keep this visible. |

Total = Completed + In progress + Not started + Blocked + Unknown.
Remaining = Total - Completed; it includes active, blocked, and unknown tasks.
Task completion percentage = 100 * Completed / Total, rounded to a whole percent for display. For zero included tasks show N/A, not 100%. If the task list is incomplete or its completion states are unknown, show the known counts and mark the percentage provisional or unavailable rather than presenting an exact result.

Required preview approval, integration, and release work must be represented in task criteria or their own actionable tasks when in scope. Do not claim a feature is delivered because code tasks are closed. Report approval and production state separately even when task completion reaches 100%. Counts measure tasks, not elapsed time or effort; new tasks can lower the percentage.

## Compact report

For one team, show its ID/name, recorded execution phase, total, completed, in progress, not started, blocked, remaining, and task completion percentage in a table. Add Unknown when nonzero. For `status all`, use one row per team plus an unassigned row where needed and a deduplicated project total. Also show preview URL and last-known availability, approval state, deployment state, blockers, and the next milestone when known. Link large evidence instead of loading it.

When a run is recorded, also show its ID/state, occupied development slots versus limit, continuous on/off, effective auto-deploy mode and batch size, number of integrated top-level tasks awaiting deployment, in-flight batch, and any scheduling/release hold. Distinguish these top-level batch counts from the actionable-task completion percentage above. Read effective run values; do not substitute newly saved defaults. Unknown run data stays unknown. Status never fills an empty slot, flushes a batch, asks for inherited auto-deploy confirmation, or changes settings.

Example with complete current data:

| Team | Total | Completed | In progress | Not started | Blocked | Remaining | Complete |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| TEAM-001 / lesson-progress | 10 | 6 | 2 | 1 | 1 | 4 | 60% |
| TEAM-002 / email-preferences | 8 | 2 | 3 | 3 | 0 | 6 | 25% |
| Project total | 18 | 8 | 5 | 4 | 1 | 10 | 44% |

Label cached data with its age. Note unconsumed team handoffs without silently turning them into completed tasks. Show “recorded in progress; session activity unverified” when only task state is known. The status action must not repair inconsistencies it discovers.

## Optional webpage

The dashboard derives teams, all tasks and overall progress from this same read model and canonical tracker. Opening or reloading local HTML displays its latest saved snapshot; it cannot itself query bd or bv. Event-driven generation updates the snapshot after meaningful task transitions without a model call.

Optional live mode uses one loopback-only helper for current read/refresh, not a build/dev server or execution controller. Opening/refreshing may regenerate only its own derived artifacts, never claim/resume tasks, repair code, run tests or deploy. Enablement is a separate explicit dashboard action, not a side effect of status.

Keep all-task search/filter, dependency graphs, source and freshness visible. A stale/failed refresh keeps the last good snapshot clearly labeled. For Beads, refresh the canonical bd export before the separately attributed optional bv provider reads it. TASKS mode retains complete progress/task/team views without forcing Beads installation. See the setup catalog for bv's upstream identity and license terms.
