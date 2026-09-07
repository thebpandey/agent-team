# Pro session actions

Treat these as instructions handled by the skill, not new native CLI commands. Use `$agent-team` in Codex and `/agent-team` in Claude Code. Accept natural-language equivalents. Interpret the action before setup, model checks, spawning, or implementation. Read only the references needed for that action.

A full natural-language `$agent-team` or `/agent-team` feature request can create a new canonical Beads or `TASKS.md` task after scope is sufficient and duplicate work is excluded. It can then register and start the assigned team. `start <name>` selects an already-defined tracker item; it must not invent requirements from the name. Bare `start` selects existing ready work. Lifecycle hooks validate later ownership and completion, but the hooks do not create task records from arbitrary prompts. This rule adds no prompt-parsing hook or new command.

Accept `auto-agent start` with the same modifiers as a plain-language alias for `agent-team start`; it does not install or invoke a separate program. For resolved help, start, resume, settings, setup, or status actions, display the [AGENT-TEAM wordmark](wordmark.md) once before user-facing command output. Internal refills do not count as new user invocations.

| Action | Behavior |
| --- | --- |
| `start [N or name] [continuous] [with-preview] [auto-deploy [B]]` | Resolve project defaults and run-only overrides using [settings](settings.md). Saved auto-deploy does not prompt for confirmation; explicit modifiers can override it for this run. Select safe tasks and run bounded or continuous teams using [runs](runs.md). N is 1–6; B counts completed top-level tasks. A name selects one feature only. |
| `auto-deploy [B or off]` | Enable current-run deployment batches (default B=1), or stop future automatic batches with off. Include eligible integrated work, without starting teams or saving defaults. Follow [release](release.md). |
| `help` | Show the [command list and examples](help.md) without setup, checks, or mutations. |
| `settings` | Run the [current-project](settings.md) sequential numbered settings wizard; do not start work or change an active run. |
| `status [name-or-ID or all]` | Run the read-only [status procedure](status.md). Never enter setup, recovery, or development from this action. |
| `pause [name-or-ID or all]` | Without a target, show the in-progress team picker plus All and wait for selection. Explicit all acts on all eligible teams; a name/ID targets one. Follow [recovery](recovery.md). |
| `pause and deploy` | Pause all Agent-Team writers and project activity, then commit and deploy verified finished, integrated, approved, not-yet-deployed work through the normal release gates. |
| `resume [name-or-ID or all]` | Without a target, show the paused team picker plus All and wait for selection. Explicit all acts on all eligible teams; a name/ID targets one. Inspect actual and saved state using recovery. |
| `approve <name-or-ID>` | Record the user's approval of the current review version for integration using [preview approval](preview.md). |
| `setup` | Follow [dependency setup](setup.md), offer automatic installation for each missing dependency, then run the complete [settings wizard](settings.md) in the same flow. |

Names use a short readable form such as `email-preferences`. Resolve exact name or stable team ID within the current project. Reserve action words, modifier words, `all`, and numeric selectors; never interpret user text as a shell command or path. No fuzzy selection for actions that change state. Ask one question for an ambiguous project, feature scope, or target; do not invent requirements from a name alone.

Accept modifiers in any order after the optional count/name selector. A number immediately after `auto-deploy` is its batch size; a number in the start selector position is the team limit. Accept `no-continuous` and `no-auto-deploy` as explicit run-only overrides. Reject conflicting/duplicate modifiers, extra numbers, non-integer or out-of-range team limits, and non-positive/non-integer batch sizes before any claims or changes. Do not clamp `start 7` to six or treat it as a feature name. A named start permits preview/deployment modifiers but no explicit count or continuous mode. Consult [help](help.md) for combined examples.

For status, a session assigned to one team defaults to that team; a project session shows the project overview. Bare `pause` and `resume` always show the matching team picker, even from a feature session or when only one team is eligible. Do not infer that team or All from the current session. `pause all` and `resume all` skip the picker and apply their recovery scope directly. A supplied name/ID affects only that team, without an extra scope prompt when the target is unambiguous. Resolve an ambiguous project first. All means the current project, never every repository.

On `start`, check for an existing matching team and task ownership before creating anything. A duplicate request identifies the existing team; it does not spawn another writer or silently resume a paused team. `with-preview` is a persistent integration gate, not just a request to start a server. Adding that requirement later is allowed before integration; never clear it because a later invocation omits the flag. Tests or standing deployment authority cannot bypass it.

A normal development invocation checks for unfinished work in its resolved scope. Use recovery when it continues that work; create a new named team only for a distinct task. A second invocation in the same conversation does not by itself start an independent session. Report the actual execution mode.

Keep status separate from action. A status question during active development briefly reports recorded progress and then returns to the current task without waiting for new instructions. A standalone status request in another session ends after the report; it must not start work. A resolved pause action supersedes continued development only for its selected target, after any required picker response. Honor later user corrections and current host permissions.

These lifecycle, multi-team, and preview features belong to Pro. They are not a background service, a transcript restore system, or a promise that the host will keep executing after closure.

## Automatic start

Accept plain-language forms such as “agent team start” and “agent team resume.” Without a name, use the existing canonical Beads list, or its configured local-file fallback. Read complete eligible records and dependency/claim state using installed-version commands. Select one ready, unclaimed, non-deferred actionable task by the tracker's recorded priority and order; break otherwise equal ties by stable task ID. Do not select blocked tasks or a summary epic merely because it appears first.

Under project registration/claim ownership, recheck eligibility, preserve its existing task ID, allocate one unique team number, and derive a readable name from the task title. Use a team-ID suffix if the name collides. If a new task genuinely needs an ID, let Beads assign its native ID or allocate one through the local tracker's single writer. Never replace an existing Beads ID or create duplicate tasks just to match a naming convention. Record the mapping before starting the team.

If a competing team claimed the task, inspect the next eligible item instead of duplicating the claim. If no task is ready, report why and create no team. Missing task scope, a missing list, or unavailable exclusive claim capability is a concrete blocker, not permission to invent work. Apply this selection once per admitted top-level task under [run capacity and scope](runs.md). `with-preview` gates every admitted task, including continuous replacements. With built-in defaults one start creates at most one team; counts and continuous mode extend this only as resolved from the command and project settings. Each team may use its bounded developer/reviewer assignments.

`start` checks existing claims but does not resume other teams. If unfinished teams exist, identify them when useful. If no new work is ready, suggest `resume` without executing it. Keep new-work creation separate from recovery.

## Pick a pause or resume target

Read the canonical team directory and recorded phase before asking. For pause, list every in-progress team, including active implementation, review, integration, release, and pending pause activity. Exclude completed and already-paused teams. For resume, list every paused team. Also show interrupted/disconnected unfinished teams under a clearly labeled Interrupted group when present, so recovery after a crash remains available. Exclude already-running and fully delivered/cleaned teams. Unknown activity must be labeled; selection does not establish that an old writer stopped. Keep existing task IDs, names, gates, and recovery rules. Include an eligible project run when no team is eligible, as specified in [runs](runs.md), so pause/resume works between tasks or while only deployment remains.

Show each team's readable name, stable ID, and recorded state, plus an explicit All option. Describe All as all listed teams in this project, including the Interrupted group if present. Use the host's supported selection control. If its option limit cannot show every team, use supported paging or show the complete table and ask for a name, ID, or All. Do not hide eligible teams, install UI dependencies, or preselect All. If none are eligible, report that and do nothing; do not present an empty All action.

Wait for the user's selection before changing task state, messaging agents to pause, checkpointing, resuming, or taking project ownership. Opening or cancelling the picker has no effect on work; existing agents continue as the host permits. A dismissed or unanswered picker is not approval to act. Record the offered project and team IDs in the conversation so the answer cannot target another project accidentally.

Resolve the selection against the offered IDs and recheck each selected team's actual state through the normal pause/recovery procedure. All chosen in the picker means the offered team set and identified project run, not teams created after the prompt. Holding that run stops future admissions; any later-created team must be reported as outside the offered set, not silently paused. If a team has finished or is already in the requested state, report that without restarting it or choosing a substitute. Honor current permissions, ownership, and preview gates. Explicit `pause all` or `resume all` uses a fresh bounded eligible inventory and requires no selection prompt; named commands use exactly the named team, including a named interrupted team that needs recovery.
