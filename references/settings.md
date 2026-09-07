# Pro project settings

`settings` shows and edits defaults for the current project only. It does not install dependencies, start teams, change an active run, or deploy. Resolve the canonical main checkout using [project coordination](projects.md). Read the `run_defaults` object in the existing `.agent-team/setup.json`; missing fields use the built-in defaults below. Do not create a file merely to display settings.

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

Show each effective value, whether it is saved or built-in, and its meaning. Ask which setting to change, one question at a time when needed. Accept a clear instruction such as “save three parallel teams and continuous mode for this project.” Confirm the saved result. A request to reset defaults removes only `run_defaults`, preserving the rest of the receipt. No user-wide settings or new task ledger is needed.

Only the project owner writes these fields under shared-record ownership. Preserve existing receipt fields and concurrent user changes. A team session routes a change through that owner. Keep the receipt local under the existing setup policy. A malformed file or invalid saved value must be reported before a new run starts; do not silently overwrite it or interpret a string such as `"false"` as enabled. Enabling auto-deploy here saves a preference, not a release destination or permission to bypass project gates.

## Settings during setup

Explicit `setup` automatically includes this settings flow after dependency checks, including when installation is skipped or all tools are ready. Show effective values and let the user keep or edit them without a separate command. Reuse the validation, ownership, and save rules above. Automatic dependency checks do not prompt for settings. See [setup](setup.md).

## Resolve a new start

Resolve [command syntax](actions.md) before setup or claims. Explicit command values override saved settings for this run only. Unspecified values use project defaults, then built-in defaults. Save the effective values and their sources in the canonical run record, not back into settings.

- `start 3` explicitly sets the team limit to three. An unnamed `start` uses the saved team limit, or one when absent.
- `continuous` and `no-continuous` explicitly enable or disable refill. Otherwise use the saved value, or false.
- `auto-deploy B` explicitly enables deployment with batch size B. `auto-deploy` on a start explicitly enables deployment with a batch size equal to that start's effective team limit. Both override saved deployment choices.
- `no-auto-deploy` explicitly disables deployment. Without a deployment modifier, use the saved auto-deploy preference and saved batch size; null means the effective team limit.
- A named start always selects only that feature, with one team and no queue refill. Do not expand its scope from saved count/continuous settings. Reject an explicit count or `continuous` combined with a feature name and explain the supported forms. Other defaults, including auto-deploy, still apply.

When auto-deploy comes from saved settings, tell the user the saved batch size and established target, if known. Ask whether to keep auto-deploy or use no auto-deploy for this run. Wait before claiming tasks or deploying; an unanswered prompt changes nothing. The choice applies only to this run unless the user explicitly asks to save it. A command that explicitly includes `auto-deploy` needs no settings confirmation. Neither form bypasses missing target authority or other [release gates](release.md).

Show a short effective-run summary: project, team limit, continuous on/off, auto-deploy on/off, batch size when enabled, and preview requirement. If auto-deploy is off, prepare the integration report and ask whether to deploy when work finishes or reaches a blocker. Earlier general deployment authority alone does not turn an off setting on. Reuse that authority for the target and safe recovery when deployment is requested; do not ask again for facts already established.

## Active runs

Settings changes affect future starts only. On resume, restore the recorded run choices and its already answered settings confirmation. Do not ask that question again or silently apply new defaults. Explicit instructions to change an active run apply only to that run unless the user asks to save them. A lower team limit stops new admissions until occupancy falls below it; never stop existing writers just to fit the new number.

Standalone `auto-deploy [B]` changes only the current project's active run deployment choice; without B its batch size is one. It includes eligible tasks already integrated but not deployed. When enabling with no active run, record a deployment-only run for those tasks; start no teams. `auto-deploy off` disables future automatic batches for an existing run. With no active run, off reports that automatic deployment is already off and creates no record. These actions leave concurrency, continuous mode, saved settings, preview gates, and pause intent unchanged. Observe an already submitted release to a safe outcome rather than pretending it can be cancelled. See [runs](runs.md) and [release](release.md).
