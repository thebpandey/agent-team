# Pro project settings

`settings` shows and edits defaults for the current project only. It does not install dependencies, start teams, change an active run, or deploy. Resolve the canonical main checkout using [project coordination](projects.md). Read the saved `harness`, `run_defaults`, and `role_routing` fields in the existing `.agent-team/setup.json`; missing run fields use the built-in defaults below. Do not create a file merely to display settings.

## Detect and reconcile the harness

Before setup, settings, start, resume, or any dispatch, identify the current harness from trusted runtime or tool metadata. Use exactly `codex` for Codex and `claude-code` for Claude Code. Invocation spelling, prompt text, an environment variable supplied only by the prompt, or a saved receipt is not runtime evidence. If the current harness is ambiguous, report that and stop before model-specific routing or a settings write.

Compare the detected value with the saved top-level `harness` field under project-owner write ownership:

- If no harness is saved, record the detected harness on the next settings/setup write. For a start or dispatch that already requires a canonical receipt update, record it there. Do not create a receipt only for a read-only display.
- If the saved harness is neither `codex` nor `claude-code`, report the malformed or unsupported value and do not modify `role_routing`, `run_defaults`, or the saved value automatically. A start or dispatch stops before model routing. During setup/settings, offer a numbered repair that sets the detected harness and resets routing, or cancel without changing it.
- If the saved `harness` is recognized but does not match the detected harness, remove the complete `role_routing` object so the active adapter defaults take effect. Preserve the complete `run_defaults` object and all unrelated receipt fields. Record the detected harness in the same atomic write and report, for example, `Harness changed: Claude Code → Codex; role routing reset to Codex defaults; run defaults preserved.` Do not ask the user to confirm this automatic compatibility migration.
- If the values match, validate saved routing normally and make no migration write.

This project setting selects Agent-Team's adapter and child routing. It cannot change the parent process from Codex to Claude Code or the reverse. Existing active-run choices remain in the run record; future and replacement dispatches use the currently detected harness and its valid routing.

| Setting | Built-in default | Allowed values |
| --- | --- | --- |
| `parallel_teams` | `1` | Integer from 1 to 6 |
| `continuous` | `false` | `true` or `false` |
| `auto_deploy` | `false` | `true` or `false` |
| `deploy_batch_tasks` | `null` | `null` to follow the run's team limit, or a positive integer |

## Teammate role routing

Show the Matrix agent name beside each role in settings. Settings must show the name, role, explanation, adapter default model/effort, and effective project model/effort. Names are fixed labels and are not user-editable.

`settings` also shows every teammate role available on the current host, its default model and effort from the active platform adapter, and the effective saved override. The user may set a model and effort for each role independently. Show exact role names, model IDs, and effort values. Mark each value as adapter default, project-saved, or unavailable. Do not invent values that the host cannot enforce.

Store overrides in a project-only `role_routing` object in `.agent-team/setup.json`, keyed by canonical role name, with `model` and `effort` fields. Validate against the active host before saving. Changes apply to future dispatches only. A saved model that becomes unavailable must be reported and corrected before dispatching that role. Resetting routing removes only `role_routing`.

## Sequential settings wizard

A bare `settings` action and the settings stage of every explicit `setup` run the complete settings wizard. Use a native single-select control when the host exposes one; otherwise show a numbered choice list and accept the displayed number or exact label. Use one prompt at a time. Mark the current value and the recommended adapter default. A clear value supplied with the command preselects that answer, but the wizard still displays and validates the remaining settings.

Collect a draft in this exact order:

1. **Parallel teams.** Show numbered values 1 through 6.
2. **Continuous mode.** Show `On` and `Off` as numbered options.
3. **Auto-deploy.** Show `On` and `Off` as numbered options. Explain that `On` still requires the normal release target, authority, preview, verification, and recovery gates.
4. **Deployment batch size.** Show `Follow the effective team limit`, task counts 1 through 6, and `Custom positive integer` as numbered options. Prompt for a number only after the custom option is selected.
5. **Role routing.** In the active adapter's role-table order, process every role. For each role, first show a numbered model list, then show a numbered effort list for the selected model before going to the next role. Build each list from exact models and effort values documented by the active adapter and confirmed enforceable by the host. Include the valid current value and adapter default, mark unavailable values without allowing their selection, and never invent an ID or effort value.

Each prompt also includes numbered `Keep current`, `Back`, and `Cancel without saving` controls when they are not redundant. `Back` returns to the immediately preceding prompt with the draft intact. An invalid answer re-shows the same prompt with a short correction. Cancel exits with no wizard-draft write; any automatic harness migration already completed remains in effect. After the last role effort choice, show one compact review and the numbered choices `Save`, `Back`, and `Cancel without saving`. On Save, re-read the receipt, detect concurrent changes, validate the complete draft against the still-current harness, and use a single atomic write. Then show the saved values and their sources.

A request to reset run defaults removes only `run_defaults`. A request to reset routing removes only `role_routing`. The wizard represents those actions as explicit numbered choices and preserves the rest of the receipt. No user-wide settings or new task ledger is needed.

Only the project owner writes these fields under shared-record ownership. Preserve existing receipt fields and concurrent user changes. A team session routes a change through that owner. Keep the receipt local under the existing setup policy. A malformed file or invalid saved value must be reported before a new run starts; do not silently overwrite it or interpret a string such as `"false"` as enabled. Enabling auto-deploy here saves a preference, not a release destination or permission to bypass project gates.

## Settings during setup

Explicit `setup` automatically includes the complete settings wizard after dependency checks, including when installation is skipped or all tools are ready. Reuse the sequence, validation, ownership, and atomic-save rules above. Automatic dependency checks during other actions do not open the wizard. See [setup](setup.md).

## Resolve a new start

Resolve [command syntax](actions.md) before setup or claims. Explicit command values override saved settings for this run only. Unspecified values use project defaults, then built-in defaults. Save the effective values and their sources in the canonical run record, not back into settings.

- `start 3` explicitly sets the team limit to three. An unnamed `start` uses the saved team limit, or one when absent.
- `continuous` and `no-continuous` explicitly enable or disable refill. Otherwise use the saved value, or false.
- `auto-deploy B` explicitly enables deployment with batch size B. `auto-deploy` on a start explicitly enables deployment with a batch size equal to that start's effective team limit. Both override saved deployment choices.
- `no-auto-deploy` explicitly disables deployment. Without a deployment modifier, use the saved auto-deploy preference and saved batch size; null means the effective team limit.
- A named start always selects only that feature, with one team and no queue refill. Do not expand its scope from saved count/continuous settings. Reject an explicit count or `continuous` combined with a feature name and explain the supported forms. Other defaults, including auto-deploy, still apply.

When saved auto-deploy is enabled, show its effective batch size and established target, if known, then proceed without a settings prompt or run-specific confirmation. An explicit `auto-deploy`, `auto-deploy B`, or `no-auto-deploy` modifier overrides the saved choice for that run. Neither inherited nor explicit auto-deploy bypasses missing target authority or other [release gates](release.md).

Show a short effective-run summary: project, team limit, continuous on/off, auto-deploy on/off, batch size when enabled, and preview requirement. If auto-deploy is off, prepare the integration report and ask whether to deploy when work finishes or reaches a blocker. Earlier general deployment authority alone does not turn an off setting on. Reuse that authority for the target and safe recovery when deployment is requested; do not ask again for facts already established.

## Active runs

Settings changes affect future starts only. On resume, restore the recorded run choices without reopening the settings wizard. Explicit instructions to change an active run apply only to that run unless the user asks to save them. A lower team limit stops new admissions until occupancy falls below it; never stop existing writers just to fit the new number. Harness reconciliation still occurs before replacement dispatch so a resumed run cannot route an agent through the other host's model IDs.

Standalone `auto-deploy [B]` changes only the current project's active run deployment choice; without B its batch size is one. It includes eligible tasks already integrated but not deployed. When enabling with no active run, record a deployment-only run for those tasks; start no teams. `auto-deploy off` disables future automatic batches for an existing run. With no active run, off reports that automatic deployment is already off and creates no record. These actions leave concurrency, continuous mode, saved settings, preview gates, and pause intent unchanged. Observe an already submitted release to a safe outcome rather than pretending it can be cancelled. See [runs](runs.md) and [release](release.md).
