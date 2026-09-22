# Native first-use setup

Setup inspects and reuses the canonical Git project, prepares approved missing
artifacts and dependencies, and records readiness without launching workers.
Native v8 requires no external hooks. Historical Node hook registration and
trust flows are not part of this setup contract.

## Inspect and reuse

Determine the actual host from runtime metadata. Resolve the installed native
controller as described in the host skill; it need not be on PATH. Run from the
Git project:

```text
agent-teamctl setup --host <codex-or-claude> --json
```

Reuse the selected Beads or `TASKS.md` tracker, existing governance, role
preferences, and approved Project Kickoff facts. Setup does not migrate an
existing tracker merely because another tool is available. Changing established
tracker authority needs an explicit data migration.

Read the structured response even when setup exits with `needs_input`. Follow
`next_action`, including when `status` is already `initialized`:

| Next action | Required continuation |
| --- | --- |
| `choose_tracker` | Resolve the tracker choice: `tasks-md` or `beads`. |
| `approve_artifacts` | Describe missing files and obtain approval for their creation. |
| `approve_kickoff` | Confirm using the discovered or explicitly selected handoff. |
| `initialize_beads` | Approve initialization of the selected Beads tracker and resume setup. |
| `approve_dependencies` | Approve the selected install bundle once; carry earlier consent forward. |
| `resolve_dependencies` | Read dependency results; repair preparation or agree on an available fallback. |
| `prepare_dependencies` | Prepare the approved handoff's missing required capabilities. |
| `project_kickoff` | Continue approved planning and tracker seeding, then import the handoff. |
| `settings` | Inspect role choices and save accepted settings, even when the choice is `inherit`. |
| `start` | Continue a requested start through the actual host tools; a setup-only request ends with readiness. |
| `inspect_tracker` | Inspect the selected tracker's readiness and missing task criteria; do not invent ready work. |

An explicit `--refuse-kickoff` returns rejected (CLI exit 2) or cancelled from
the defensive management path, without setup writes. Respect that refusal;
do not continue initialization or dispatch from this flow.

After consent, create only missing governance/tracker files with
`setup --tracker <tasks-md-or-beads> --approve --host <host> --json`.
`DECISIONS.md` and `AGENT_TEAM_RULES.md` are reused when present. The controller
does not overwrite an existing plan with its minimal scaffold. `BLOCKERS.md`
and `DECISIONS.md` remain projections rather than alternative task stores.

## Planning before setup

When planning is still in progress, prepare dependencies without initializing
the project setup receipt:

```text
agent-teamctl setup --install beads,serena,graphify --tracker beads --approve --prepare-only --host <host> --json
```

Use only the approved names and selected tracker. `--prepare-only` returns
dependency observations and a next action; it does not generate governance or
attach a handoff. Continue approved Project Kickoff planning and seed its
selected tracker before importing the resulting handoff. Dependency preparation
does not prove that planning or task acceptance is complete.

The current discovery path is `.project-kickoff/AGENT_TEAM_HANDOFF.json`.
The loader supports nested Project Kickoff 0.5.1 and historical 0.5.0 handoffs. Reuse
scope, checks, task identities, and tracker facts instead of interviewing the
user again. Import after approval:

```text
agent-teamctl setup --approve-kickoff --host <host> --json
agent-teamctl setup --kickoff <path> --approve-kickoff --host <host> --json
```

The second form selects an explicit handoff. A late handoff may attach to an
existing setup without rewriting its setup receipt, settings, tracker contents,
or active run packets. It must agree with existing tracker authority. Preserve
an error or conflicting handoff for diagnosis; do not reset setup to force it.
Without any handoff, approved setup can create a minimal scaffold. Project
Kickoff remains optional.

## Dependency consent and preparation

Inspect before installing and offer the selected missing bundle once. Native
installation recipes cover selected Beads plus Serena, Graphify, rg, ast-grep,
and LeanCTX. Include their required scoped installer/Python prerequisites in
the bundle consent; do not change global PATH or registry. An executable is not automatically prepared
for this project or available to a fresh worker.

Run the approved selection with
`setup --install <comma-separated-names> --approve --host <host> --json`.
Include `--tracker beads` when preparing the selected Beads tracker. Installs
use project scope and add no external hooks. Preserve compatible tools and
unrelated host settings. Report missing package managers, runtimes, access, or
customized-file conflicts specifically; do not infer permission to change them.

Optional tools retain bounded native fallbacks. An approved handoff's required
capabilities still gate affected work. Never claim executable discovery proves
MCP registration, worker access, a successful workload, or readiness of the
selected tracker. Reuse successful preparation rather than reinstalling it.

## Save accepted settings

Read `settings --json` and present the requested host/roles using
[supported settings](SETTINGS.md). Both hosts support orchestrator, developer
(coder alias), reviewer, and visual_reviewer model/effort preferences.
Setup asks for settings while their saved revision is zero.

For every role the user wants inherited, save both model and effort after consent:
`settings codex.<role>.model=inherit codex.<role>.effort=inherit --json` in Codex or
`settings claude.<role>.model=inherit claude.<role>.effort=inherit --json` in Claude.
Repeat for all requested roles, or bundle assignments in one save. Preserve other
explicit choices; saving developer alone does not make reviewer/visual reviewer
inherit instead of their defaults. The revision advances
even when the effective choice remains inherited. Inspection alone cannot
complete this step. Cancel, no answer, or interruption saves nothing.

## Bind the native observations

The controller's JSON reports project and dependency facts. The foreground
host supplies actual runtime identity, tool availability, worker IDs, and
supported model/effort fields. Record unavailable or unobserved facts honestly;
do not replace native observations with passing fixtures or edit receipts.

After readiness, `start --host <host> --json` reserves a packet. Follow the
installed host skill for the real Agent/collaboration call and exact returned
handle acknowledgement. `status --json` is read-only and does not prepare,
install, repair, or dispatch. Switching foreground hosts transfers no live
worker ownership, and an unknown writer remains occupied.
