# Pro session actions

Treat these as instructions handled by the skill, not new native CLI commands. Use `$agent-team` in Codex and `/agent-team` in Claude Code. Accept natural-language equivalents. Interpret the action before setup, model checks, spawning, or implementation. Read only the references needed for that action.

| Action | Behavior |
| --- | --- |
| `start [name] [with-preview]` | Without a name, claim the next ready unassigned task and create exactly one team. With a name, resolve its feature scope from the request or existing tasks. Follow [project coordination](projects.md). |
| `status [name-or-ID or all]` | Run the read-only [status procedure](status.md). Never enter setup, recovery, or development from this action. |
| `pause [name-or-ID]` | Checkpoint and pause only the selected team's work using [recovery](recovery.md). |
| `resume [name-or-ID or all]` | Without a target, recover all unfinished teams in the current project. A name/ID targets one team. Inspect actual and saved state using recovery. |
| `approve <name-or-ID>` | Record the user's approval of the current review version for integration using [preview approval](preview.md). |
| `setup` | Follow the existing dependency setup procedure. |

Names use a short readable form such as `email-preferences`. Resolve exact name or stable team ID within the current project. Reserve action words and `all`; never interpret user text as a shell command or path. No fuzzy selection for actions that change state. Ask one question for an ambiguous project, feature scope, or target; do not invent requirements from a name alone.

For status and pause, a session assigned to one team defaults to that team. Unqualified `status` from a project session shows the project overview. Unqualified `pause` from an ambiguous project session asks which team. Unqualified `resume`, even from a feature session, means all unfinished teams in the resolved current project; `resume <name-or-ID>` targets one. `status all` means all teams in the current project, not every repository.

On `start`, check for an existing matching team and task ownership before creating anything. A duplicate request identifies the existing team; it does not spawn another writer or silently resume a paused team. `with-preview` is a persistent integration gate, not just a request to start a server. Adding that requirement later is allowed before integration; never clear it because a later invocation omits the flag. Tests or standing deployment authority cannot bypass it.

A normal development invocation checks for unfinished work in its resolved scope. Use recovery when it continues that work; create a new named team only for a distinct task. A second invocation in the same conversation does not by itself start an independent session. Report the actual execution mode.

Keep status separate from action. A status question during active development briefly reports recorded progress and then returns to the current task without waiting for new instructions. A standalone status request in another session ends after the report; it must not start work. A pause request supersedes continued development for its target. Honor later user corrections and current host permissions.

These lifecycle, multi-team, and preview features belong to Pro. They are not a background service, a transcript restore system, or a promise that the host will keep executing after closure.

## Automatic start

Accept plain-language forms such as “agent team start” and “agent team resume.” Without a name, use the existing canonical Beads list, or its configured local-file fallback. Read complete eligible records and dependency/claim state using installed-version commands. Select one ready, unclaimed, non-deferred actionable task by the tracker's recorded priority and order; break otherwise equal ties by stable task ID. Do not select blocked tasks or a summary epic merely because it appears first.

Under project registration/claim ownership, recheck eligibility, preserve its existing task ID, allocate one unique team number, and derive a readable name from the task title. Use a team-ID suffix if the name collides. If a new task genuinely needs an ID, let Beads assign its native ID or allocate one through the local tracker's single writer. Never replace an existing Beads ID or create duplicate tasks just to match a naming convention. Record the mapping before starting the team.

If a competing team claimed the task, inspect the next eligible item instead of duplicating the claim. If no task is ready, report why and create no team. Missing task scope, a missing list, or unavailable exclusive claim capability is a concrete blocker, not permission to invent work. `start with-preview` applies the same automatic selection with the user approval gate. One start request creates at most one feature team; it does not authorize an endless queue-draining loop. That team may use its bounded developer/reviewer assignments.

`start` checks existing claims but does not resume other teams. If unfinished teams exist, identify them when useful. If no new work is ready, suggest `resume` without executing it. Keep new-work creation separate from recovery.
