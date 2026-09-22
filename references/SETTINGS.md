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

`inherit` removes that field's explicit override while preserving the explicit
role object, so that choice remains inherited instead of receiving a missing-role
default. Other fields, roles, hosts, and unknown preserved settings remain intact. A profile is included in the
appropriate future host dispatch; no setting switches the current parent
model or retroactively changes an active worker.

## Defaults and existing profiles

The orchestrator defaults to `inherit`. Missing developer, reviewer, and
visual-reviewer profiles default to `gpt-6-sol` for Codex and `claude-opus-5-5`
for Claude. Present explicit inheritance stays inherited.

The controller recognizes only these exact old model IDs for automatic updates:

| Saved ID | Effective replacement |
| --- | --- |
| `gpt-5.6-sol` | `gpt-6-sol` |
| `gpt-5.6-luna` | `gpt-6-luna` |
| `claude-opus-5` | `claude-opus-5-5` |

Inspection shows effective normalized choices without writing. The next explicit
settings save persists these recognized updates while retaining effort values,
custom model IDs, explicit inheritance, and unrelated JSON fields. These names
are preferences, not evidence of account availability: validate against current
native host capabilities and report unavailable or unknown models without
silently substituting a fallback.

## First-use acceptance must be saved

Setup returns `next_action: settings` while settings revision is zero. The native
Codex or Claude skill runs a numbered wizard for orchestrator, developer,
reviewer, and visual_reviewer, using the active host's actual available-model
metadata. Show each role's purpose, saved/effective values, and available
recommendation. The user selects a number; they never need to type a model ID.
The noninteractive controller stores the resolved preference strings.

Each model menu offers **1. Keep**, **2. Inherit**, then **3 onward: available
models**, with a stable displayed index to exact model ID mapping. Effort uses
its own numbered menu for the selected model's supported levels, plus Keep and
Inherit. Unknown effort support offers only Keep or Inherit. Never reinterpret
an answer using a reordered catalog. An invalid number reprompts only its role
or effort; keep other draft answers. If an option becomes unavailable, revisit
only that choice. Claude alias-only tools require current metadata proving the
alias resolves to the selected exact ID.
Keeping an effort after changing models still requires checking that the new
model supports it; revisit only that effort choice when it does not.

If a catalog is missing, explain it and offer Keep, Inherit, or the native model
picker only if actually available; refresh actual metadata after the picker. Do not invent a list from
the recommendations above. An unavailable saved choice can be preserved, but
does not authorize dispatch or a silent substitute. Orchestrator changes apply
to future orchestration, never the current parent model.

Selections authorize saving. Hold all answers in a draft, recap the resolved
choices, then run one batch settings command without another approval prompt.
For first-use Keep, persist the shown preference, representing empty values as
`inherit`. Inheritance stays inherited even when its runtime model or effort is
observable; do not pin that observed value. For existing Keep, omit that field from the write. If all
existing fields are kept, no write is needed. A verbal acceptance or a read-only inspection does
not advance the revision.

For every role the user wants inherited, actually run
`agent-teamctl settings codex.<role>.model=inherit codex.<role>.effort=inherit --json` in Codex, or
`agent-teamctl settings claude.<role>.model=inherit claude.<role>.effort=inherit --json` in Claude.
Bundle the accepted assignments for all requested roles in one explicit save.
If all roles were selected, include orchestrator, developer, reviewer, and
visual_reviewer; saving developer alone leaves the others' choices/defaults intact.
Preserve other explicit choices. A successful save advances
revision even when the effective preference remains inherited. Inspect the
result, then resume the original setup/start request. Carry prior approval
forward; do not repeatedly ask the same settings question.

Cancel, no answer, or interruption saves nothing, including earlier draft role
answers; do not advance setup or dispatch. An invalid draft must not be
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
