# Lane recovery and projections implementation brief

## Scope

Implement the Task 3 recovery, cleanup, status, and dashboard surfaces after agreeing interfaces with the lane-engine owner. Read the approved design, lane-engine brief, and complete applicable instructions. Own hooks/lib/recovery.mjs, cleanup.mjs, status.mjs, dashboard.mjs and their focused tests. Do not edit shared schema, lane lifecycle, CLI, setup, or policy files without explicit handoff. No commits, spawns, live records, or installed configuration changes.

## Recovery and capacity

Treat missing legacy lane state as no lanes; reject malformed present records through the shared validator. Project durable lane ID, role/team, ordered remaining queue, current task, worker identity, rotation count, worktree, branch, brief hash, and last handover pointer. Actual retention requires native identity and observed liveness. Never infer stopped from host exit, elapsed time, missing local process, or task completion. Unknown liveness stays occupied.

Count one retained lane once, not once per historic task assignment. A retained idle worker still occupies its lane slot. Keep logical parallel_teams distinct from actual native worker capacity and reserved reviewer capacity. Existing non-lane task accounting must retain its conservative behavior. Read-only status/recovery projection never mutates task, lane, or resource state.

Resume provides bounded exact pointers for reattachment when the actual host exposes a live retained identity. Otherwise request the existing rotation path with authentic stop or transfer evidence; do not silently create a new worker or mark unknown capacity free. Surface pending operations and unresolved dispatch outcomes before admitting another task.

## Cleanup

One worktree belongs to one lane for its whole queue. Cleanup requires every queued and historical lane task to have accepted integration evidence for its exact task revision, exhausted queue, no unresolved assignment or current task, authentic stopped writer or ownership transfer, and existing resource/preview checks. Lane closure does not itself delete anything. Task cleanup cannot bypass lane eligibility and remove its shared worktree early.

Preserve all existing clean-main, branch safety, dirty/untracked/ignored-file preservation, evidence retention outside disposable worktrees, and no-force deletion rules. Shared lane task assignments are permitted only under explicit matching canonical lane identity, not merely equal worktree strings.

## Settings and tests

Use resolveExecutionSettings(setup, host) for subprocessMaxBufferBytes at recovery call sites and bounded output projections. Do not use a larger output cap as permission to copy full tool dumps to the parent.

Write tests first for lane single-counting, idle and unknown occupancy, mixed legacy and lane records, read-only projections, exact reattach/rotation pointers, early task cleanup refusal, unintegrated historical task refusal, safe completed lane eligibility, and unchanged legacy behavior. Coordinate fixtures and schema fields with lane-engine owner rather than inventing a second lane representation.

Record RED and GREEN commands with exact counts in /tmp/AGENT-TEAM-LANE-PROJECTIONS-REPORT.md. Include owned-file snapshot hashes and unresolved integration items. No em dashes, emoji, or preambles in authored files.
