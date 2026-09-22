---
name: agent-team
description: Use when a Codex request involves Agent-Team setup, status, or starting coordinated development work.
metadata:
  version: "8.0.10"
---

# Agent-Team for Codex

Use the native controller from the canonical Git project. Keep the selected
Beads or `TASKS.md` tracker authoritative. Native v8 requires no external hooks.

## Resolve the controller

Try `command -v agent-teamctl` (PowerShell: `Get-Command agent-teamctl`). If absent,
read the install manifest's `files` entry with `role: binary` and use its absolute
`path`. Find `install-manifest.json` under `$XDG_DATA_HOME/agent-team` when set;
otherwise Linux uses `${XDG_CONFIG_HOME:-$HOME/.config}/agent-team`, macOS uses
`$HOME/Library/Application Support/agent-team`, and Windows uses
`%LOCALAPPDATA%\AgentTeam`. The adjacent `bin/agent-teamctl` (`.exe` on Windows)
is the fallback. Do not assume the installed binary is on PATH or rewrite shell
configuration. Below, `agent-teamctl` means that resolved executable. If none
exists, report the missing native installation and its recovery step.

## First use and settings

For `setup`, or `start` without ready setup, run
`agent-teamctl setup --host codex --json`. Reuse existing tracker, governance,
settings, and Project Kickoff facts; do not repeat the Kickoff interview.
Follow `needs_input` and `next_action` rather than inventing readiness:

| `next_action` | Continue with |
| --- | --- |
| `choose_tracker` | Ask which tracker to use only if existing authority does not resolve it: `tasks-md` or `beads`. |
| `approve_artifacts` | Describe the missing files; after consent run `setup --tracker <choice> --approve --host codex --json`. This creates only missing governance/tracker files. |
| `approve_kickoff` | Summarize the discovered nested Project Kickoff 0.5.0 handoff, then use `setup --approve-kickoff --approve --host codex --json` after approval. Add `--kickoff <path>` only for an explicit handoff path. |
| `initialize_beads` | Obtain approval for the selected Beads initialization and continue the approved setup; preserve existing Beads data. |
| `approve_dependencies` | Confirm the selected bundle once, then rerun its install command with `--approve`. |
| `resolve_dependencies` | Read each dependency result; repair failed preparation or obtain an explicit fallback choice. |
| `prepare_dependencies` | Prepare the handoff's missing required capabilities before dispatch. |
| `project_kickoff` | Continue approved Kickoff planning and tracker seeding, then import its handoff. |
| `settings` | Show role choices and save accepted preferences, including `inherit`; inspection alone does not complete this step. |
| `start` | Continue the original start request; a setup-only request ends with readiness. |
| `inspect_tracker` | Inspect the selected tracker's task readiness; do not invent ready work. |

Without a handoff, approved setup can create a minimal scaffold. Carry earlier
approval forward; do not re-ask for already approved artifacts or dependencies.
Offer one bundle: selected Beads plus Serena, Graphify, rg, ast-grep, and lean-ctx.
Explain the selected missing dependencies and ask one installation question.
For approved selections, execute
`agent-teamctl setup --install beads,serena,graphify,rg,ast-grep,lean-ctx --approve --host codex --json`,
using only the chosen names, then resume setup. Installs are project scoped and
add no external hooks. Report failures and available native fallbacks; an
optional dependency's absence does not by itself authorize broader changes.
Include required scoped installer/Python preparation in that consent; never
rewrite global PATH or registry. A cancelled/refused Kickoff stops this flow;
do not resume setup or dispatch after `--refuse-kickoff` is rejected/cancelled.
For unfinished planning, add `--prepare-only` (and selected `--tracker beads`)
to the approved install command, then complete approved Kickoff
planning before import. This prepares dependencies without initializing setup.
The handoff is `.project-kickoff/AGENT_TEAM_HANDOFF.json`; an approved late
attachment preserves settings and active packets. Follow next_action even
when status is initialized.

Inspect `agent-teamctl settings --json`. Save individual preferences with
`agent-teamctl settings codex.developer.model=<model-or-inherit> --json` and
`agent-teamctl settings codex.developer.effort=<effort-or-inherit> --json`.
Both `codex` and `claude` support `orchestrator`, `developer` (`coder` alias),
`reviewer`, and `visual_reviewer`. Settings apply to future dispatch; they do
not change the current parent model. Pass the returned profile to supported
host tool fields and report actual runtime constraints without claiming an
unsupported model or effort was enforced. Once ready, continue the user's
original start request without another generic confirmation.

If the user accepts inherited defaults, actually save
`agent-teamctl settings codex.developer.model=inherit --json`. This advances
settings revision beyond zero; merely inspecting or verbally accepting does
not. Cancel or no answer saves nothing. Then resume setup/start as requested.

`agent-teamctl status --json` reads real persisted state without setup,
installation, or dispatch. Show this skill's `metadata.version` or the matching
native binary version; historical Node v7 documents are not banner authority.

## Native dispatch

`agent-teamctl start --host codex --json` only reserves a packet; it never starts a worker.
Call `collaboration.spawn_agent` only when the response has
`host_dispatch_required: true` and `already_admitted: false`. If it reports an
already admitted packet or `host_dispatch_required: false`, observe the named
team/packet and do not spawn or acknowledge a replacement worker. For a fresh
packet, use its bounded task payload and saved `profile` model/effort, then
acknowledge only the exact returned canonical task name using
`agent-teamctl start --action ack ...`; do not invent a handle or invoke an
unproven worker shell command.

Route the assignment's role to a supported native agent type: developer work
uses the worker capability, and review uses an independent reviewer capable of
that assignment. If the host has no suitable role capability, report it. When
passing explicitly configured model/effort overrides to `spawn_agent`, use
`fork_turns: "none"` or a supported positive count; omitted/`"all"` forks do not
accept overrides in this runtime. Include the bounded packet and instructions
explicitly when the child receives no inherited context.

Observe a live handle owned by another host; do not replace or duplicate it.
Changing the foreground host transfers no worker identity or ownership. If a
required host tool is unavailable, report the dispatch blocker and keep the
packet unacknowledged. Give each worker its exact scope, worktree, acceptance
checks, and applicable instructions; preserve other agents' edits.

On replay, honor `actual_host` (the packet owner) and `observation_required`;
retain that original host's developer profile and worker identity. A missing
ack after a possible launch is uncertain: observe the original host rather
than treating the missing ack as permission to spawn again.

Use this exact acknowledgement shape, replacing only returned JSON values:

```text
agent-teamctl start --action ack --team <team> --packet-digest <packet_digest> --host codex --identity <spawn_agent_task_name> --task <packet.task> --candidate <packet.specRevision> --json
```

Completion and retained reuse use the same exact handle fields:

```text
agent-teamctl start --action complete --team <team> --packet-digest <packet_digest> --host codex --identity <spawn_agent_task_name> --task <packet.task> --candidate <packet.specRevision> --json
agent-teamctl start --action clean --team <team> --reviewer <independent_reviewer_identity> --json
agent-teamctl start --action idle --team <team> --packet-digest <packet_digest> --host codex --identity <spawn_agent_task_name> --task <packet.task> --candidate <packet.specRevision> --json
agent-teamctl start --action next --team <team> --json
```

For a retained queue handoff, require actual completed work, an actual
independent CLEAN review result, and observed host idle state. Then record
completion, the distinct `clean` reviewer identity, and idle evidence through
the native start actions; calling those actions cannot manufacture evidence.
When `start --action next --team <team> --json` returns
`host_followup_required`, call `collaboration.followup_task` with the same
acknowledged canonical handle and its fresh delta packet, then acknowledge the
new packet using that same host identity. Do not follow up merely because work
was appended: require a fresh native `host_followup_required: true` response
after that idle evidence. A terminal `next` returns
`host_followup_required: false`, consumes the final task, and requires no host
action. If a retained-intent retry returns the same packet with
`already_admitted: true` and `host_followup_required: false`, observe it only:
do not call `followup_task` again or issue a replacement acknowledgement; the
original exact acknowledgement remains the only valid continuation.

`start --run <run> --task <task> --json` normally only appends an explicit
already-snapshotted task. If it appends to a consumed idle retained team, it
instead returns a fresh `host_followup_required` packet; use
`collaboration.followup_task` with the returned retained handle, then
acknowledge that packet with the same identity.
