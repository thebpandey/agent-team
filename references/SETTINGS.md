# Settings and role routing

Settings belong to the canonical project setup receipt. Inspection is read-only. Editing settings does not install dependencies, start teams, change an active run, approve a preview or deploy.

## Show before asking

Display every available role with: purpose, effective model, effort, source (host default/project override), availability and whether the host can enforce it. Mark unavailable/unknown values explicitly. Use role labels first; friendly names are optional.

A bare settings request shows the compact overview and targeted controls. A request to change one role asks only for that role's model and compatible effort. Use real native selection controls if available, otherwise numbered choices; users need not memorize model IDs or effort keywords.

Include current and recommended values with plain-language quality/cost explanations. Show only supported choices, but preserve and flag an unavailable saved custom choice. Do not silently replace it with a weaker model. Quality-first is the default; balanced/economical presets are explicit choices with the same acceptance requirements.

## Persistence

Confirm the actual runtime from trusted host metadata, not invocation spelling or a prompt-supplied environment variable. Persist independent routing per host. Switching Codex ↔ Claude Code selects that host's saved/default profile without deleting the other profile. Preserve all unrelated settings.

Migrate recognized legacy routing into its recorded host profile once, under the project writer's lock. Never discard custom routing because the host changed. Malformed or ambiguous input is not permission to reset it; diagnose safe repairs and preserve uncertain data.

Show configured versus actually enforced model/effort. A saved preference or generated agent definition is not proof of native dispatch. A skill cannot switch its parent process/model. Apply only approved, available fallback/escalation choices, disclose the actual route and never claim the substitute is the requested model.

## Run defaults

The current native settings command edits only `parallel_teams`, `continuous`,
`auto_deploy`, `deploy_batch_tasks`, `codex.developer.model`, and
`codex.developer.effort`. Other saved host-profile fields are preserved without
claiming they are editable or enforced. Codex model/effort is included in the
next native dispatch request; it remains a preference until the actual host
acknowledges the returned packet.

| Setting | Built-in default | Meaning |
| --- | --- | --- |
| parallel_teams | 1 | Requested active development capacity; actual host slots and reviewer needs can reduce it |
| continuous | false | Refill from the authorized eligible task scope as capacity frees |
| auto_deploy | false | Submit verified batches to an already authorized target; does not grant new release permission |
| deploy_batch_tasks | null | Follow effective run limit; a positive integer chooses a task batch size |

The supported requested team range is 1–6; do not equate team count with native agent slots. Logical parked claims do not consume stopped compute slots. Reserve review capacity.

Explicit start modifiers override saved defaults for that run only. Resolve omitted values from saved defaults, then built-ins. A named task start has one delivery and no queue refill; reject an explicit count or continuous modifier combined with a name. A numeric token following auto-deploy is its batch size.

Save effective choices and their source in the canonical run record. Future settings changes do not rewrite that record. Changing an active run requires an explicit instruction for that run; reducing capacity stops new admissions rather than killing existing writers.

## Execution safety limits

Each host starts with the same independent bounded defaults. `limits.subprocessMaxBufferBytes` is 2097152 bytes, `limits.maxPlanTasks` is 1000, and `limits.canonicalRecordMaxBytes` is 16777216 bytes. The canonical-record allowance supports the maximum validated task/lane state; it does not enlarge subprocess output or the separate `lanes.workerUpdateMaxChars` limit of 2000 characters. The wizard shows each current, effective and source value without resetting unrelated host settings.

These are maximum supported limits, not evidence that a read or command succeeded. Missing host inventory, usage, or model metadata stays unknown. In particular, parent-model comparison is unavailable unless the native adapter supplies trustworthy comparable metadata.

## Editing flow

1. Read receipt, current revision and actual host capability catalog.
   Use the [native setup binding](SETUP.md#bind-the-native-observations) for the exported command API; bare shell CLI settings cannot discover a host model catalog or validate a new role route by itself.
2. Show the requested setting/role with current/recommended choices. Use a numbered fallback when native controls are unavailable.
3. Validate the draft and chosen model's supported effort. Include Back and Cancel where relevant.
4. Re-read under exclusive writer ownership, detect concurrent changes and save only intended fields atomically.
5. Show effective values, source, enforcement limitations and that future dispatches use them.

Cancelled/invalid drafts cause no settings write. Inspecting settings never performs an automatic migration. Reset routing affects only the requested host/profile; reset run defaults affects only those defaults. Preserve other fields.

Bare `settings` is targeted: run only the requested role/control flow and never force unrelated questions. A state-changing `setup` invocation is different: after dependency preparation it always enters the full current-effective wizard—run defaults, then selected roles' model/effort choices, followed by one review/save. Cancel is Keep Existing and preserves settings bytes/version/operations. A no-answer, timeout, or interruption infers nothing and writes no settings/default/operation. Do not change an active run; confirmed values apply only to future dispatches.

## Escalation and budgets

Setup establishes approved role fallback/escalation policy. Repeated lack of progress may escalate within it; expected red TDD tests are not difficulty evidence. Never silently downgrade for cost or increase authority to keep a run moving.

Soft usage budgets suggest smaller packets, less duplicate discovery or a better route without a continue prompt. Explicit hard budgets request a safe checkpoint; neither kind waives tests or required review. Missing usage is unknown, not zero.

Current-run auto-deploy on/off remains separate from saved defaults. Enabling it does not start development or overwrite preview/pause gates. Observe an in-flight external release to a safe known outcome rather than pretending a setting cancels it.
