---
name: agent-team
description: Use when a Claude Code request involves Agent-Team setup, status, or starting coordinated development work.
metadata:
  version: "8.0.10"
---

# Agent-Team for Claude

Use the native controller from the canonical Git project. Keep the selected
Beads or `TASKS.md` tracker authoritative. Native v8 requires no external hooks.

## Resolve the controller

Try `command -v agent-teamctl` (PowerShell: `Get-Command agent-teamctl`). If absent,
read the install manifest's `files` entry with `role: binary` and use its absolute
`path`. Find `install-manifest.json` under `$XDG_DATA_HOME/agent-team` when set;
otherwise Linux uses `${XDG_CONFIG_HOME:-$HOME/.config}/agent-team`, macOS uses
`$HOME/Library/Application Support/agent-team`, and Windows uses
`%LOCALAPPDATA%\AgentTeam`. The adjacent `bin/agent-teamctl` (`.exe` on Windows)
is the fallback. Do not assume the binary is on PATH or rewrite shell settings.
Below, `agent-teamctl` means that resolved executable. If none exists, report
the missing native installation and its recovery step.

## First use and settings

For `setup`, or `start` without ready setup, run
`agent-teamctl setup --host claude --json`. Reuse existing tracker, governance,
settings, and Project Kickoff facts; do not repeat the Kickoff interview.
Follow `needs_input` and `next_action` rather than inventing readiness:

| `next_action` | Continue with |
| --- | --- |
| `choose_tracker` | Ask which tracker to use only if existing authority does not resolve it: `tasks-md` or `beads`. |
| `approve_artifacts` | Describe the missing files; after consent run `setup --tracker <choice> --approve --host claude --json`. This creates only missing governance/tracker files. |
| `approve_kickoff` | Summarize the discovered nested Project Kickoff 0.5.0 handoff, then use `setup --approve-kickoff --approve --host claude --json` after approval. Add `--kickoff <path>` only for an explicit handoff path. |
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
`agent-teamctl setup --install beads,serena,graphify,rg,ast-grep,lean-ctx --approve --host claude --json`,
using only the chosen names, then resume setup. Installs are project scoped and
add no external hooks. Report failures and available native fallbacks; an
optional dependency's absence does not by itself authorize broader changes.

Include required scoped installer/Python preparation in that consent; never
rewrite global PATH or registry. A cancelled/refused Kickoff stops this flow;
do not resume setup or dispatch after `--refuse-kickoff` is rejected/cancelled.

When planning is unfinished, add `--prepare-only` to the approved install
command (and `--tracker beads` when selected), then complete approved Kickoff
planning before import. This prepares dependencies without initializing setup.
The discovered handoff is `.project-kickoff/AGENT_TEAM_HANDOFF.json`; an approved
late handoff can attach to existing setup without resetting settings or active
packets. Honor the next action even if `status` already says `initialized`.

Inspect `agent-teamctl settings --json`. Save individual preferences with
`agent-teamctl settings claude.developer.model=<model-or-inherit> --json` and
`agent-teamctl settings claude.developer.effort=<effort-or-inherit> --json`.
Both `codex` and `claude` support `orchestrator`, `developer` (`coder` alias),
`reviewer`, and `visual_reviewer`. Settings apply to future dispatch; they do
not change the current parent model. Pass the returned profile to supported
host tool fields and report actual runtime constraints without claiming an
unsupported model or effort was enforced. Once ready, continue the user's
original start request without another generic confirmation.

If the user accepts inherited defaults, actually save
`agent-teamctl settings claude.developer.model=inherit --json`. This advances
settings revision beyond zero; merely inspecting or verbally accepting does
not. Cancel or no answer saves nothing. Then resume setup/start as requested.

`agent-teamctl status --json` reads real persisted state without setup,
installation, or dispatch. Show this skill's `metadata.version` or the matching
native binary version; historical Node v7 documents are not banner authority.

## Native dispatch

Run `agent-teamctl start --host claude --json`. This reserves a bounded packet;
it does not launch a worker. Call the actual Claude `Agent` tool only when
`host_dispatch_required: true` and `already_admitted: false`. Use the returned
scope, worktree, acceptance checks, and saved `profile`; tell the worker to
preserve other agents' edits. Acknowledge the exact agent ID returned by that
tool. Never invent an ID, substitute a task label, or use a fictional CLI worker.
If the host cannot return an observable ID, report the blocker and leave the
packet unacknowledged. Report a launch only after the real tool call and ack.

Choose a supported Agent subagent type for the assignment's role and tools.
Development and independent review need their corresponding saved profiles;
do not route review back to the author. Report a missing role capability or an
unsupported model/effort field rather than claiming the host enforced it.

```text
agent-teamctl start --action ack --team <team> --packet-digest <packet_digest> --host claude --identity <returned_agent_id> --task <packet.task> --candidate <packet.specRevision> --json
agent-teamctl start --action complete --team <team> --packet-digest <packet_digest> --host claude --identity <returned_agent_id> --task <packet.task> --candidate <packet.specRevision> --json
agent-teamctl start --action clean --team <team> --reviewer <independent_reviewer_identity> --json
agent-teamctl start --action idle --team <team> --packet-digest <packet_digest> --host claude --identity <returned_agent_id> --task <packet.task> --candidate <packet.specRevision> --json
agent-teamctl start --action next --team <team> --json
```

Record actual completion, a distinct independent CLEAN reviewer, and observed
idle state before retained reuse. When `next` returns a fresh
`host_followup_required: true` packet, resume the same acknowledged agent using
Claude's supported Agent resume facility and its exact returned agent ID. Send
only the fresh bounded assignment, then acknowledge that packet with the same
identity. If this runtime lacks same-agent resume, report that limitation;
creating a replacement is not evidence of retained reuse.

`start --run <run> --task <task> --json` normally appends an explicit task from
the existing snapshot. Appending to a consumed idle team may instead return a
fresh `host_followup_required` packet; apply the same resume-and-ack rule.
Queue append alone never authorizes a follow-up. An `already_admitted: true`
retry or `host_followup_required: false` means observe only; do not resume or
acknowledge again. A terminal `next` consumes the last task and needs no host
action. Observe foreign live handles without duplicating them. Changing the
foreground host transfers no worker identity or ownership.

On replay, honor `actual_host` (the packet owner) and `observation_required`;
retain that original host's developer profile and worker identity. A missing
ack after a possible launch is uncertain: observe the original host rather
than treating the missing ack as permission to spawn again.
