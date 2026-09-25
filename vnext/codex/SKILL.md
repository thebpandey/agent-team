---
name: agent-team
description: Use for Agent-Team setup, status, tasks, one-off work, or scoped lifecycle actions in Codex.
metadata:
  version: "8.0.15"
---

# Agent-Team for Codex

Use the native controller from the canonical Git project. Keep Beads or `TASKS.md` authoritative. No external hooks are required.

## Resolve the controller

Try `command -v agent-teamctl` (PowerShell: `Get-Command agent-teamctl`). If absent, read the install manifest's
`files` entry with `role: binary` and use its absolute `path`. Find `install-manifest.json` under
`$XDG_DATA_HOME/agent-team` when set; otherwise Linux uses `${XDG_CONFIG_HOME:-$HOME/.config}/agent-team`, macOS uses
`$HOME/Library/Application Support/agent-team`, and Windows uses `%LOCALAPPDATA%\AgentTeam`. The adjacent
`bin/agent-teamctl` (`.exe` on Windows) is the fallback. Do not assume the installed binary is on PATH or rewrite shell
configuration. Below, `agent-teamctl` means that resolved executable. If none exists, report the missing native
installation and its recovery step.

## First use and settings

For `setup`, or `start` without ready setup, run `agent-teamctl setup --host codex --json`. Reuse existing tracker,
governance, settings, and Project Kickoff facts; do not repeat the Kickoff interview. Follow `needs_input` and
`next_action` rather than inventing readiness:

| `next_action` | Continue with |
| --- | --- |
| `choose_tracker` | Ask which tracker to use only if existing authority does not resolve it: `tasks-md` or `beads`. |
| `approve_artifacts` | Describe the missing files; after consent run `setup --tracker <choice> --approve --host codex --json`. This creates only missing governance/tracker files. |
| `approve_kickoff` | Summarize the discovered nested Project Kickoff 0.5.1 handoff (0.5.0 also supported), then use `setup --approve-kickoff --approve --host codex --json` after approval. Add `--kickoff <path>` only for an explicit handoff path. |
| `initialize_beads` | Obtain approval for the selected Beads initialization and continue the approved setup; preserve existing Beads data. |
| `approve_dependencies` | Confirm the selected bundle once, then rerun its install command with `--approve`. |
| `resolve_dependencies` | Read each dependency result; repair failed preparation or obtain an explicit fallback choice. |
| `prepare_dependencies` | Prepare the handoff's missing required capabilities before dispatch. |
| `project_kickoff` | Continue approved planning/source creation and commit, then import the handoff; an empty tracker can instead receive a bounded task. |
| `settings` | Show role choices and save accepted preferences, including `inherit`; inspection alone does not complete this step. |
| `start` | Continue the original start request; a setup-only request ends with readiness. |
| `inspect_tracker` | Inspect the selected tracker's task readiness; do not invent ready work. |
| `repair_tracker` | Preserve selected tracker data; diagnose its reported read failure and repair before admission. |
| `observe_run` | Read `status --json`, observe existing workers through their actual host, and continue that run without new setup or duplicate dispatch. |
| `resume` | Preserve the explicit hold until the user authorizes its scope's resume; follow scoped controls below. A start retry does not cancel a pause. |
| `observe_control` | Complete the pending actual host control and exact observation acknowledgements below; reconcile uncertain launches before repeating its request. |
| `provide_task_details` | Collect only missing approved fields; if `created: true`, update the existing blocked tracker draft before retrying. |

Without a handoff, approved setup can create a minimal scaffold. Carry earlier approval forward; do not re-ask for
already approved artifacts or dependencies. Offer one bundle: selected Beads plus Serena, Graphify, rg, ast-grep, and
lean-ctx. Explain the selected missing dependencies and ask one installation question. For approved selections, execute
`agent-teamctl setup --install beads,serena,graphify,rg,ast-grep,lean-ctx --approve --host codex --json`, using only
the chosen names, then resume setup. Installs are project scoped and add no external hooks. Report failures and
available native fallbacks; an optional dependency's absence does not by itself authorize broader changes. Include
required scoped installer/Python preparation in that consent; never rewrite global PATH or registry. A cancelled/refused
Kickoff stops this flow; do not resume setup or dispatch after `--refuse-kickoff` is rejected/cancelled. For unfinished
planning, add `--prepare-only` (and selected `--tracker beads`) to the approved install command, then complete approved
Kickoff planning before import. This installs tools without initializing setup. A `deferred` result awaits source files
and a Git commit; report analysis as deferred, continue planning, then retry preparation. Tool installation alone is not
project readiness. The handoff is `.project-kickoff/AGENT_TEAM_HANDOFF.json`; an approved late attachment preserves
settings and active packets. Follow next_action even when status is initialized.

## Numbered role settings wizard

When setup returns `next_action: settings`, or the user asks to change role settings, run
`agent-teamctl settings --json`. A bare inspection does not open this wizard. Configure the active host
`codex`; preserve the other host's profile. For first use, ask in order: `orchestrator`, `developer`
(`coder` alias), `reviewer`, `visual_reviewer`; a targeted change asks only for its requested roles.

Use the current Codex collaboration tool's advertised model IDs and supported efforts; include only models usable by that role's native agent capability.
The controller's defaults and normalized `modelUpdates` are saved preferences, not an available-model catalog.
For each role, show its purpose, saved/effective model and effort, and mark the recommendation only if available.
The orchestrator preference affects future orchestration; it does not switch the current parent model.

Present a numbered model menu and keep a stable `index -> exact model ID` mapping for that prompt:
1. Keep the shown saved/current preference (label an unavailable choice honestly).
2. Inherit the current native host model.
3 onward: one option per actually available model, displaying its name and exact ID; the user replies with its number.

Never ask the user to type a model ID. Decode numbers against the displayed mapping, not a newly sorted catalog.
After a model choice, show a numbered effort menu from that model's actual supported effort metadata, including
Keep and Inherit; if efforts are unknown, offer only Keep or Inherit and explain the host constraint.
Keeping effort after changing models requires checking compatibility with the new model; a valid menu number
does not make an unsupported effort valid. Revisit only that effort choice if unsupported.
An invalid or out-of-range answer must reprompt only that role or its effort, retaining other draft answers.
If availability changes, flag the affected selection and redisplay only that role before saving.

If the catalog is missing, say so: offer Keep, Inherit, or the native model picker only if actually available; never fabricate model options
from defaults or documentation. After the native model picker, re-read actual metadata and display the choices.
An unavailable saved choice may be kept without claiming readiness; require an available choice or an explicit
supported inheritance route before dispatch. Do not silently substitute another model.

Keep all answers in a draft. Cancel, no answer, or interruption saves nothing, even after earlier role selections;
do not advance setup or launch work. Selections authorize saving: after all requested model/effort answers, recap
the mapped choices and save them in one settings command with no extra approval prompt:
`agent-teamctl settings codex.<role>.model=<mapped-ID-or-inherit> codex.<role>.effort=<mapped-effort-or-inherit> ... --json`.
Expand the placeholders and include each accepted role; do not send numbers or literal ellipses to the controller.
For every role the user wants inherited, actually save both fields as `inherit` when both were selected.
For Keep on first use, persist the shown preference (empty values become `inherit`) so revision advances.
Inheritance stays `inherit`; never pin an observed runtime model or effort merely because its value is visible.
For existing settings, leave kept fields untouched. Preserve other explicit choices and unrelated settings.
If every choice keeps existing saved fields, skip the write. Read the saved result, confirm its actual values,
then resume the original setup/start request without another generic confirmation. Setup-only requests end at readiness.

`agent-teamctl status --json` reads real persisted state without setup, installation, or dispatch. Show this skill's
`metadata.version` or the matching native binary version; historical Node v7 documents are not banner authority.

## Native dispatch

`start`, `task add --execute`, and `one-off` packets use this same native dispatch protocol.
`agent-teamctl start --host codex --json` only reserves a packet; it never starts a worker. Call
`collaboration.spawn_agent` only when the response has `host_dispatch_required: true` and `already_admitted: false`.
The confirmed no-launch retry below is the only exception to this flag rule. Outside that exception, if the response
reports an already admitted packet or `host_dispatch_required: false`, observe the named team/packet and do not
spawn or acknowledge a replacement worker. For a fresh packet, use its bounded task payload and saved `profile`
model/effort, then acknowledge the exact returned canonical task name. Never invent a handle or a worker shell command.

Route the assignment's role to a supported native agent type: developer work uses the worker capability, and review uses
an independent reviewer capable of that assignment. If the host has no suitable role capability, report it. When passing
explicitly configured model/effort overrides to `spawn_agent`, use `fork_turns: "none"` or a supported positive count;
omitted/`"all"` forks do not accept overrides in this runtime. Include the bounded packet and instructions explicitly
when the child receives no inherited context.

Observe a live handle owned by another host; do not replace or duplicate it. Changing the foreground host transfers no
worker identity or ownership. If a required host tool is unavailable, report the dispatch blocker and keep the packet
unacknowledged. Give each worker its exact scope, worktree, acceptance checks, and applicable instructions; preserve
other agents' edits.

On replay, honor `actual_host` (the packet owner) and `observation_required`; preserve its worker identity.
Preserve its assignment profile except for an approved correction in the confirmed no-launch retry below.
A missing ack after a possible launch is uncertain: observe the original host rather
than treating the missing ack as permission to spawn again. Copy returned values for acknowledgement:

```text
agent-teamctl start --action ack --team <team> --packet-digest <packet_digest> --host codex --identity <spawn_agent_task_name> --task <packet.task> --candidate <packet.specRevision> --json
```

```text
agent-teamctl start --action complete --team <team> --packet-digest <packet_digest> --host codex --identity <spawn_agent_task_name> --task <packet.task> --candidate <packet.specRevision> --json
agent-teamctl start --action clean --team <team> --reviewer <independent_reviewer_identity> --json
agent-teamctl start --action idle --team <team> --packet-digest <packet_digest> --host codex --identity <spawn_agent_task_name> --task <packet.task> --candidate <packet.specRevision> --json
agent-teamctl start --action next --team <team> --json
```

Record actual completion, independent CLEAN review with a distinct reviewer, and observed host idle state before reuse.
For a fresh `host_followup_required: true` packet, call `collaboration.followup_task` on the same acknowledged handle
with its fresh delta, then acknowledge that packet with the same identity. Queue append alone never authorizes follow-up.
A terminal `next` consumes the final task with `host_followup_required: false` and requires no host action.
An `already_admitted: true` retained retry means observe only, except for the confirmed no-delivery retry below.

`start --run <run> --task <task> --json` normally only appends an explicit already-snapshotted task. If it appends to a
consumed idle retained team, it instead returns a fresh `host_followup_required` packet; use
`collaboration.followup_task` with the returned retained handle, then acknowledge that packet with the same identity.

### Confirmed no-launch retry

Use this exception only in the original uninterrupted foreground attempt when the actual native tool response expressly
guarantees **no worker was created**, or **no follow-up was delivered**. An unavailable-model label alone is insufficient.
Retain that actual response as evidence. Read the updated approved profile for the original host and assignment role;
verify it against current native model/capability metadata without substituting a fallback.

Re-read the reservation and controls: the exact packet, digest, owner, task, queue fingerprint, worktree and candidate revision
must be unchanged, still unacknowledged, with no applicable admission or control hold. Retained work also requires the
unchanged retained handle. If every check passes, make one bounded retry of the original native call with that packet,
owner/handle and approved profile, even when replay reports `host_dispatch_required: false`, `host_followup_required: false`
or `already_admitted: true`. Do not create another reservation, clear intent, transfer ownership or invent an acknowledgment.
Acknowledge only the actual successful result; another failure ends this retry.

Timeouts, generic errors, missing handles, lost responses or possible launch/delivery remain uncertain: observe the original
host without retrying. Cross-session requests or unavailable original evidence invalidate this exception; report recovery blocked
and continue observation, never claim recovered execution.

## Tasks, one-off work and scoped controls

Use approved task facts in one JSON object: `id` (optional), `objective`, `criteria` array, `checks` array of
`{name, command: [program, ...args]}`, and `writablePaths` array. Missing facts are `needs_input`, never permission to
invent scope. Each writable path is either one exact repository-relative path or a directory ending in `/**`; embedded
wildcards such as `src/*.ts` or `src/prefix*` are rejected before the task is saved.

```text
agent-teamctl task add --queue --from <task.json> --host codex --json
agent-teamctl task add --execute --from <task.json> --host codex --json
agent-teamctl one-off <feature|audit|review> --from <task.json> --host codex --json
```

Queue saves tracker work without launching. Execute reserves eligible work; follow settings/readiness responses or
dispatch its returned packet without another start. One-off uses a standalone assignment; audit/review need objective
and criteria, and `read_only: true` means no writes. Pass that restriction to the actual worker. Neither a reservation
nor `ok: true` establishes execution or completion.

For `pause|stop|cancel|resume`, use one explicit `--project <canonical-root>`, `--run <run>`, `--team <team>` or
`--task <task>` selector and `--json`. The controller records admission intent. For `pending_handles`, use actual host
control and observation on those exact identities; unsupported host control must report a blocker without a fabricated
acknowledgement. `unacknowledged_teams` require original-host launch reconciliation; reconcile, then repeat the control
request. On resume, `blocking_scopes` remain held; act only on returned pending handles and resume their existing
assignment without spawning replacements.

```text
agent-teamctl <pause|stop|cancel|resume> --action ack --control-id <control_id> --run <run> --team <team> --task <task> --host <handle.Host> --identity <handle.Identity> --packet-digest <handle.PacketDigest> --candidate <handle.CandidateRevision> --observation <observed-state> --json
```

Copy the returned handle fields; record `paused` for pause, `stopped` for stop/cancel, and `running` for resume only
after observing that actual state. An admission hold or cleared hold is not proof that a host worker stopped or resumed.
