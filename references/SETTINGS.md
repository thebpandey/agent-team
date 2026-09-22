# Native settings and role routing

`agent-teamctl settings --json` reads the project's saved settings overlay.
Inspection does not install dependencies, initialize setup, start workers,
approve previews, or deploy. Preserve unrelated keys and the other host's
preferences when saving a targeted change.

## Supported keys

| Key | Values / purpose |
| --- | --- |
| `parallel_teams` | Integer 1–6; requested development capacity remains bounded by actual host/reviewer capacity. |
| `continuous` | `true` or `false`; saves the future-run preference. |
| `auto_deploy` | `true` or `false`; grants no destination or release authority. |
| `deploy_batch_tasks` | Positive task count, or `null` to inherit. |
| `<host>.<role>.model` | A model identifier or `inherit`. |
| `<host>.<role>.effort` | An effort value or `inherit`. |

Hosts are `codex` and `claude`. Roles are `orchestrator`, `developer`, `reviewer`,
and `visual_reviewer`; `coder` is an alias for `developer`. Choose values that
the actual host supports. The controller validates a bounded preference string;
saving it does not prove account availability or native enforcement.

```text
agent-teamctl settings --json
agent-teamctl settings parallel_teams=1 --json
agent-teamctl settings codex.developer.model=inherit --json
agent-teamctl settings claude.reviewer.effort=inherit --json
```

`inherit` removes that field's explicit override. Other fields, roles, hosts,
and unknown preserved settings remain intact. A profile is included in the
appropriate future host dispatch; no setting switches the current parent
model or retroactively changes an active worker.

## First-use acceptance must be saved

Setup returns `next_action: settings` while settings revision is zero. Show the
effective role choices and obtain the user's selection or acceptance of
inherited host defaults. A verbal acceptance or a read-only inspection does
not advance the revision.

After inherited defaults are accepted, actually run
`agent-teamctl settings codex.developer.model=inherit --json` in Codex, or
`agent-teamctl settings claude.developer.model=inherit --json` in Claude. A successful save advances
revision even when the effective preference remains inherited. Inspect the
result, then resume the original setup/start request. Carry prior approval
forward; do not repeatedly ask the same settings question.

Cancel, no answer, or interruption saves nothing. An invalid draft must not be
silently replaced by defaults. A bare request to inspect settings requires no
wizard or write; a request to change one role asks only for its relevant choices.

## Native role routing

Show role purpose, configured model/effort, effective inherited values when
observable, and actual host support. Keep unknown availability and enforcement
explicit. Preserve and flag unavailable saved choices rather than silently
substituting another model. Host switching selects the other host's saved
profile without deleting the first profile or taking over its live workers.

Use the assignment's supported native agent type and saved role profile.
Independent review must remain independent of its author. Pass only model and
effort fields supported by the real host tool. Codex overrides need a compatible
`fork_turns` setting; Claude capabilities depend on the active Agent runtime.
Report the actual dispatched route and exact returned handle. A saved setting
or generated definition alone is not dispatch evidence.

## Persistence and authority

The controller re-reads settings under the project mutation lock and writes a
receipt-bound overlay atomically. It does not rewrite the selected tracker or
immutable setup receipt. An approved late Project Kickoff attachment preserves
the settings binding and active run packets. A malformed or conflicting binding
requires diagnosis, not a reset or inferred tracker migration.

Keep approved fallback/escalation choices separate from release authority.
Changing concurrency, continuous mode, or deployment preferences never waives
review, tests, explicit pauses, task scope, or destination approval. Missing
usage or parent-model metadata stays unknown.
