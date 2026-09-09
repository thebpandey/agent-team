# Agent-Team lifecycle hooks

Lifecycle hooks run small checks when Codex or Claude Code reports an event. They add deterministic checks and reminders to the Agent-Team workflow. They do not replace the skill, the task tracker, or independent review.

The hooks require Node.js 24 and use only the Node.js standard library. They do not install project dependencies or missing analysis tools. The current root package supports Codex and Claude Code. The files in `legacy/claude-v3` stay archived and do not enter current installations or archives.

## Runtime mapping

Both hosts run `hooks/agent-team-hook.mjs`. The `--runtime` and `--event` options select a thin adapter. The shared result has `mode`, `allow`, `messages`, `context`, `capabilities`, and `mutations` fields.

| Purpose | Codex | Claude Code |
| --- | --- | --- |
| Startup and recovery reminder | `SessionStart` | `SessionStart` |
| Tool policy | `PreToolUse` | `PreToolUse` |
| Changed-file lint | Bounded `PostToolUse` batches | One `PostToolBatch` result after parallel tools |
| Compaction checkpoint | `PreCompact` | `PreCompact` |
| Interruption checkpoint | `Interrupt` | No equivalent reliable final event |
| Explicit task completion | Supported mapped transition only | `TaskCompleted` |
| Reliable local skill activation | unsupported | `PreToolUse` for `Skill`; `UserPromptExpansion` for a direct slash command |

User-scope Codex configuration is `~/.codex/hooks.json`; Claude Code uses the `hooks` object in `~/.claude/settings.json`. Project-scope installation targets `.codex/hooks.json` or `.claude/settings.local.json` in the selected project. Registration in a file does not prove the installed host supports or exercised an event. Host events have different payloads. The adapters normalize them before shared policy runs. A Codex patch includes every add, edit, delete, and move. Claude `PostToolBatch` collects all `Edit` and `Write` calls once. It runs changed-file lint and saves a factual checkpoint without also registering a per-edit `PostToolUse` hook.

## Requirements 1–15

| Requirement | Mode | Shared behavior |
| --- | --- | --- |
| 1. Recovery reminder | Advisory | Return a bounded, read-only recovery snapshot with current Git, registered identity, tracker, handoff, pause/hold, and pending integration/release facts. Label evidence current, stale, or unavailable. Never resume work. |
| 2. Ownership | Enforce | Use registered identity and canonical paths. Check all files, both sides of moves, shared files, linked worktrees, and symlink targets. |
| 3. Integration and release | Enforce | Keep integration and release separate. Integration uses bounded current local Git and `git ls-remote` base/remote evidence. Release binds owner, authorization provenance, process, run/batch, task IDs, artifact/revision, integration, verification, preview, pause/hold, delta, and recovery records. |
| 4. Test data | Advisory | Use heuristic changed-content warnings for destructive test code, privileged clients, and weak cleanup assertions. This is not proof that data is shared, live, or safe. Exclude recognized disposable fixtures and `Map.delete`. |
| 5. Migrations | Advisory | Use heuristic changed-SQL checks for configured database type, guards, environment, disposable execution, catalog status, and visible invariants. This is not proof that a migration is correct. An unavailable check is not a pass. |
| 6. Destructive database work | Enforce | Recognize supported SQL clients and explicit provider or app-script mappings. Require scoped authorization, inventory, recovery, target, cascade evidence, and either a fresh dry run or a recorded fresh alternative when the explicit capability is unsupported. Messages omit raw SQL. |
| 7. Lint | Advisory | Batch changed source files by the nearest monorepo config. Exclude generated paths. Use only an installed executable, with a timeout and bounded output. |
| 8. Bounded evidence | Advisory | Git and external evidence probes have time and output limits. Missing tools or unavailable checks are reported, not passed. |
| 9. Checkpoints | Automatic | Write a small atomic and idempotent record for supported compaction and interruption events. Preserve agent-authored next actions and decisions. Omit native payload data and redact common secret fields. |
| 10. Completion | Enforce | Gate explicit final transitions against canonical task ownership, revision, checks, review, requirement evidence, and deployment/cleanup evidence only when each flag is in scope. Permit factual partial, blocked, and deferred outcomes. Ordinary `Stop`, status, questions, and interruption events do not complete a task. |
| 11. Recurring cost | Advisory | Compare normalized schedule declarations and warn when a schedule is added or materially changed. Retained declarations do not warn when unrelated whole-file content changes. Report unknown live pricing as unknown. |
| 12. Activation telemetry | Automatic | Log reliable Claude hook-visible activation paths in the receipted installation scope; legacy unreceipted execution keeps user-scope logging. Add project/team correlation only from active canonical state. Report Codex skill activation as unsupported instead of inferring it from a file read. |
| 13. Effectiveness audit | On demand | Bounded-read logs, the canonical tracker, and `MISTAKES.md`. Report unavailable or malformed sources and correlate stable task, team, and lesson IDs. This factual correlation does not prove policy effectiveness. |
| 14. Package validation | CI and on demand | Check required files, version parity, local links, executable entry points, runtime registration, platform coverage, manifest inclusion, and policy IDs. |
| 15. Artifact validation | On demand | Compare the one complete universal ZIP with the manifest, source bytes, and source revision. It contains both host adapters; installation selects the actual host and scope. Reject missing, stale, unexpected, duplicate, absolute, and path-escaping members. Source-only checks report not applicable. |

## Policy state and limits

Policy activates only when Git metadata resolves a canonical project whose `.agent-team/setup.json` identifies Agent-Team. `.agent-team/TEAMS.md` and the canonical task tracker own identity and task status. `.agent-team/state.json` carries machine-readable run, gate, writer, pending-operation and usage evidence. It is not a second task ledger. `.agent-team/operation-mappings.json` is a schema-validated, non-authoritative cache derived only from validated canonical state. The cache is not a task ledger. It contains a project identity, a source path, an explicit non-authoritative marker, and validated mappings only.

Shell recognition tokenizes documented command forms, including `git -C <repo> push` and unambiguous `mv` operands. Provider and Model Context Protocol (MCP) tools require an explicit operation mapping in operational state or the validated separate inventory. This explicit mapping coverage is not a universal security boundary. MCP means a configured external tool connection. Missing mappings, hosted tools that do not emit a hook, continued `write_stdin` input, shell aliases, generated commands, and a host process that is killed before its hook runs are blind spots.

Advisory parser or tool failures stay visible and do not block work. A statically recognized in-scope critical operation, or an operation identified by the validated separate mapping cache, fails closed when its required state or parser result is unavailable. A missing or invalid mapping cache makes mapped-operation fallback protection unavailable; health and unavailable advice state that fact. Ordinary shell commands and unmapped read-only provider calls continue with visible unavailable advice. Both `PreToolUse` adapters use the native structured `deny` result. Claude `TaskCompleted` uses exit code 2 and stderr because that event does not accept a JSON permission decision. Codex never emits `permissionDecision: "ask"`. Existing scoped authorization proceeds without a new prompt.

Hook JSON is unsigned. Its runtime, event, and session fields do not authenticate a caller. They cannot supply mapping content or authorization. Any session identity can trigger a `SessionStart` cache rebuild for an active project. A supported post-tool state-file event can also rebuild it. Both paths read and validate canonical state while holding the project lock, then use an atomic rename. Malformed canonical state cannot update the cache. The cache only preserves operation classification during a later state read failure and never grants authority. Read-only and status events do not write it.

Checkpoints cannot guarantee a final write after abrupt termination. A timestamp never proves that a lock owner stopped. Activation logs store only time, runtime, skill/session/event identity, project/team identity, and a non-sensitive correlation ID. They do not store prompts, arguments, credentials, file contents, connection strings, customer data, or raw SQL. Logs use user-only permissions, append locking, deduplication, and bounded rotation.

## Install and operate

For user scope, the installer puts the Codex skill at `~/.agents/skills/agent-team`, the Claude copy at `~/.claude/skills/agent-team`, and Claude role definitions under `~/.claude/agents`. For project scope, these directories are relative to the selected project root instead of home. Resolve the applicable skill and hook paths from that installation's receipt; do not substitute a different globally installed version. The installer adds missing roles and updates unchanged managed roles with backups. It reports a conflict and preserves a customized role. It does not touch `legacy/claude-v3`.

One scope-specific lock covers skill, role, configuration, backup, and receipt mutations. A failed transaction reconciles only its own changes. The installer does not mark an unowned source checkout for deletion. Uninstall removes a copied package only when its complete managed file set still matches the receipt. It preserves changed targets and reports conflicts. Legacy discovery copies are recoverably backed up under the installation receipt. Handler-level ownership, not a substring or whole-group claim, controls update/removal inside mixed hook groups. A second identical install does not duplicate handlers or backups.

LeanCTX hooks and MCP registration are unrelated configuration and must survive Agent-Team install, update, and uninstall. Neither integration replaces whole hook arrays or settings objects. Back up affected configuration and compare pre-existing handlers afterward. Do not run a broad LeanCTX initializer as an automatic repair. A healthy LeanCTX hook does not prove Agent-Team lifecycle-hook health, or vice versa. See [LeanCTX integration](lean-ctx.md).

Run these commands from an inspected source checkout:

```bash
node hooks/agent-team-cli.mjs check-package
node hooks/agent-team-cli.mjs install --host codex --scope user
node hooks/agent-team-cli.mjs health --project /path/to/project
node hooks/agent-team-cli.mjs audit --tracker /path/to/.agent-team/TASKS.md --mistakes /path/to/MISTAKES.md
node hooks/agent-team-cli.mjs build-artifacts --output /safe/output
node hooks/agent-team-cli.mjs check-artifacts --archive /safe/output/agent-team-7.0.0.zip
node hooks/agent-team-cli.mjs uninstall --host codex --scope user
```

Choose `--host claude-code` for Claude, or `--host both` only when explicitly wanted. Project scope also needs `--project /path/to/project`. `rollback` uses the same selected-host ownership removal contract as `uninstall`; it is not a version downgrade. Only receipted pre-install resources are candidates for restoration, not earlier managed update snapshots. Changed or unowned content is preserved and reported.

Installation does not grant trust. `health` reports installed, registered, trusted, supported, and exercised as separate values, with per-event provenance and native support unknown until evidenced. With `--project`, it also reports a current, stale, missing, invalid, or unavailable-state mapping cache. Cache health reports that the cache is non-authoritative, comes from validated canonical state, and is only for fallback classification. Complete any native `/hooks` trust step yourself, then reload the host if it requires a new session. Approving hooks does not prove each event ran.

For a project installation use `health --scope project --project /path/to/project`; the default health scope is user. Observed `packaged_entrypoint` transport evidence does not establish native trust, native support or skill activation. Anonymous lifecycle events receive distinct observation IDs; complete native batch IDs support replay detection without truncating later tool identities.

## Agent-facing project helpers

These are Node helpers from [the official Agent-Team package](https://github.com/thebpandey/agent-team), not native Codex/Claude terminal subcommands. Use them from the installed complete package. The user can keep using natural-language skill requests.

| Helper after `node hooks/agent-team-cli.mjs` | Contract |
| --- | --- |
| `project-initialize --project /path/to/project --request /path/to/request.json` | Create/adopt approved canonical records with exact owner, branch and complete tracker IDs. Reports canonical readiness separately from native/dependency readiness. |
| `settings`, `settings-wizard`, `settings-update` | Require project/host/project-scope selectors. Read overview or compatible role menus; mutations require the versioned setup envelope described in [setup](setup.md). |
| `dependencies`, `dependencies-prepare`, `readiness` | Require project/host and selected user/project capability scope. Installation is not proof of functional or fresh-worker access. |
| `dashboard-configure` | Save explicit snapshot/optional graph preferences under setup ownership/version checks; never starts a server or installs bv. |
| `status --project /path/to/project` | Read all tasks/teams, freshness, exact canonical owner values and observed setup/operational versions. No tests or writes. |
| `usage --project /path/to/project` | Read existing per-agent usage receipts. Missing data is unknown. Optional `--receipt /path/to/receipts.json` reads an explicit bounded regular file. |
| `recovery --project /path/to/project --session session-id` | Read the session's recovery packet. Add `--task task-id`, `--worktree /path` or `--include-git true` when relevant. Never resumes work. |
| `eligibility --project /path/to/project` | Read ready and held work under recorded scope/capacity; never claims or spawns a task. |
| `checkpoint --project /path/to/project --request /path/to/request.json` | Save the session's versioned authored packet. Its expected version is the checkpoint version, not the operational-state version. |
| `task-transition --project /path/to/project --request /path/to/request.json` | Claim, pause, park or resume through the canonical owner/version/identity checks. Does not terminate or spawn agents. |
| `gate-evidence --project /path/to/project --request /path/to/request.json` | Bind a passed evidence artifact to the current exact revision and task set. Does not run tests or grant authorization. |
| `cleanup --project /path/to/project --request /path/to/request.json` | Remove only the requested eligible verified worktree; uncertain or retained resources remain untouched. |
| `dashboard-snapshot --project /path/to/project` | Generate `.agent-team/dashboard/index.html` once. This does not silently enable future snapshots. |
| `dashboard-start --project /path/to/project --port 0` | Explicitly run the foreground loopback viewer. Reports its actual URL; Ctrl-C/SIGTERM stops this helper. A requested occupied port fails instead of moving silently. |

Mutation request files are schema-versioned JSON, at most 256 KiB, and must be regular files. For example, the orchestrator can construct a pause request from freshly observed status:

```json
{
  "schemaVersion": 1,
  "actorSessionId": "registered-project-owner-session",
  "expectedVersion": 4,
  "request": {
    "operationId": "pause-AT-001-unique-id",
    "taskId": "AT-001",
    "action": "pause",
    "expectedFingerprint": "exact-current-tracker-fingerprint",
    "expectedOwner": "TEAM-001"
  }
}
```

Replace every example value with observed facts. `status.versions.operational` and `status.freshness.fingerprint` supply transition preconditions; `task.canonicalOwner` preserves Beads' empty unassigned owner, unlike the human display label. Never guess a zero version after an unavailable read. Re-read on conflict; do not reuse an operation ID with different intent. JSON identity fields are not authentication and cannot override the registered canonical owner.

At actual worker dispatch, include its observed `writer` identity (`pid`, `startTime`, `bootId`, `host`) in the claim request. The exported `captureWriterIdentity(pid)` helper records those facts; pass the dispatched worker's PID, not the short-lived claim command's PID. Claim validates that process is active and records it under the existing state lock. Claims without a writer remain supported, but cannot establish stopped-writer evidence for parking or resumption. Never fill `state.json` with a synthetic writer to make a lifecycle check pass.

If a claim's tracker write succeeded before its state receipt failed, replay reconciles only the original signature-bound writer. An observed exited writer is restored as `stopped`, never active; unknown or reused process identity holds recovery. The claim remains assigned, and a stopped writer requires the normal park or explicit pause/resume procedure before a replacement can run.

After a requested pause, save the assigned session's checkpoint and establish that its recorded writer stopped. An explicit resume includes `explicitResume: true`, the new observed active writer, and `checkpointPath` when the paused runtime has no recorded checkpoint. The helper validates the checkpoint's session, task, worktree and current source evidence before continuing. A parked task retains its original checkpoint and prerequisite requirement; a request cannot replace them.

Versions are separate: initialization/settings/preparation use the setup version; task/gate/cleanup mutations use the operational version; checkpoint writes use that session's checkpoint version from recovery. Zero is valid only for an actually absent initial record, not an unreadable record. The [setup guide](setup.md#bundled-setup-helpers) documents its distinct envelope and trusted-host context boundary.

Ordinary conflict/unavailable outcomes are structured results, not completion. Inspect `status` and `reason` even if the process exits zero. A successful or replayed mutation can refresh an explicitly enabled snapshot without repeating its tracker effect. Supported checkpoint and canonical-file post-tool events also refresh opted-in snapshots under the existing event deadline. Dashboard failure never changes the policy decision or task authority; expired writes preserve the last usable file.

The original dashboard is server-free by default. Optional [beads_viewer](https://github.com/Dicklesworthstone/beads_viewer), by Jeffrey Emanuel, retains its [complete upstream license and rider](https://github.com/Dicklesworthstone/beads_viewer/blob/main/LICENSE); do not vendor or relicense its engine. Its graph is derived from fresh selected Beads data, not a second tracker.

Run unit and regression tests with:

```bash
node --test tests/hooks-*.test.mjs
```

The archive builder uses normalized member order and timestamps. Archive validation accepts no archive as a source-only, not-applicable result. It does not invent an archive or claim that one passed.

## Upstream inspiration

The clean implementation was informed by `randommonicle/claude-skills` revision `6c4e5ad39220f50f1059f2cde77f046a61158f6a`, licensed under Apache-2.0. No upstream source was copied. The Agent-Team repository keeps its own license in [LICENSE](../LICENSE).
