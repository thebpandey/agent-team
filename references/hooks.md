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

Codex loads managed groups from `~/.codex/hooks.json`. Claude Code loads them from the `hooks` object in `~/.claude/settings.json`. Host events have different payloads. The adapters normalize them before shared policy runs. A Codex patch includes every add, edit, delete, and move. Claude `PostToolBatch` collects all `Edit` and `Write` calls once. It runs changed-file lint and saves a factual checkpoint without also registering a per-edit `PostToolUse` hook.

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
| 12. Activation telemetry | Automatic | Log only reliable Claude hook-visible activation paths with registered project/team or project-owner provenance. Report Codex activation as unsupported instead of inferring it from a file read. |
| 13. Effectiveness audit | On demand | Bounded-read logs, the canonical tracker, and `MISTAKES.md`. Report unavailable or malformed sources and correlate stable task, team, and lesson IDs. This factual correlation does not prove policy effectiveness. |
| 14. Package validation | CI and on demand | Check required files, version parity, local links, executable entry points, runtime registration, platform coverage, manifest inclusion, and policy IDs. |
| 15. Artifact validation | On demand | Compare both runtime ZIP files with the manifest, source bytes, and source revision. Reject missing, stale, unexpected, duplicate, absolute, and path-escaping members. Source-only checks report not applicable. |

## Policy state and limits

Policy activates only when Git metadata resolves a canonical project whose `.agent-team/setup.json` identifies Agent-Team. `.agent-team/TEAMS.md` and the canonical task tracker own identity and task status. `.agent-team/state.json` carries only machine-readable gate evidence and pointers that Markdown cannot safely express. It is not a second task ledger. `.agent-team/operation-mappings.json` is a separate schema-versioned inventory of explicit critical provider and shell mappings. This inventory remains readable when operational state is unavailable.

Shell recognition tokenizes documented command forms, including `git -C <repo> push` and unambiguous `mv` operands. Provider and Model Context Protocol (MCP) tools require an explicit operation mapping in operational state or the validated separate inventory. This explicit mapping coverage is not a universal security boundary. MCP means a configured external tool connection. Missing mappings, hosted tools that do not emit a hook, continued `write_stdin` input, shell aliases, generated commands, and a host process that is killed before its hook runs are blind spots.

Advisory parser or tool failures stay visible and do not block work. A statically recognized in-scope critical operation, or an operation identified by the validated separate mapping inventory, fails closed when its required state or parser result is unavailable. Ordinary shell commands and unmapped read-only provider calls continue with visible unavailable advice. Both `PreToolUse` adapters use the native structured `deny` result. Claude `TaskCompleted` uses exit code 2 and stderr because that event does not accept a JSON permission decision. Codex never emits `permissionDecision: "ask"`. Existing scoped authorization proceeds without a new prompt.

Checkpoints cannot guarantee a final write after abrupt termination. A timestamp never proves that a lock owner stopped. Activation logs store only time, runtime, skill/session/event identity, project/team identity, and a non-sensitive correlation ID. They do not store prompts, arguments, credentials, file contents, connection strings, customer data, or raw SQL. Logs use user-only permissions, append locking, deduplication, and bounded rotation.

## Install and operate

The installer puts the authoritative Codex skill at `~/.agents/skills/agent-team` and the Claude copy at `~/.claude/skills/agent-team`. It also manages the bundled current role definitions under `~/.claude/agents`. It installs missing roles and updates unchanged managed roles with backups. It reports a conflict and preserves a customized role. It does not touch `legacy/claude-v3`.

One user-level lock covers skill, role, configuration, backup, and receipt mutations. A failed transaction rolls back its own changes. The installer does not mark the source-at-target checkout for deletion. Uninstall removes a copied package only when its complete managed file set still matches the receipt. It preserves changed targets and reports conflicts. The installer also backs up and removes `~/.codex/skills/agent-team` from active discovery, merges only Agent-Team hook groups, and preserves unrelated settings. A second identical install does not duplicate groups or backups.

Run these commands from an inspected source checkout:

```bash
node hooks/agent-team-cli.mjs check-package
node hooks/agent-team-cli.mjs install
node hooks/agent-team-cli.mjs health
node hooks/agent-team-cli.mjs audit --tracker /path/to/.agent-team/TASKS.md --mistakes /path/to/MISTAKES.md
node hooks/agent-team-cli.mjs build-artifacts --output /safe/output
node hooks/agent-team-cli.mjs check-artifacts --archive /safe/codex.zip --archive /safe/claude.zip
node hooks/agent-team-cli.mjs uninstall
```

`uninstall` is the rollback command for the managed installation. It removes Agent-Team hook groups and unchanged files owned by its receipt. It restores saved prior skill copies. It preserves changed managed targets and reports them as conflicts. Installation does not grant trust. `health` reports installed, registered, trusted, supported, and exercised as separate values. It leaves `trusted` unknown until the host supplies evidence. Complete any native `/hooks` trust step yourself, then refresh the host if it requires a new session.

Run unit and regression tests with:

```bash
node --test tests/hooks-*.test.mjs
```

The archive builder uses normalized member order and timestamps. Archive validation accepts no archive as a source-only, not-applicable result. It does not invent an archive or claim that one passed.

## Upstream inspiration

The clean implementation was informed by `randommonicle/claude-skills` revision `6c4e5ad39220f50f1059f2cde77f046a61158f6a`, licensed under Apache-2.0. No upstream source was copied. The Agent-Team repository keeps its own license in [LICENSE](../LICENSE).
