# Agent-Team Cross-Runtime Hooks Design

## Scope and authority

Implement the fifteen approved requirements in the user specification for the current Agent-Team root edition. The root `SKILL.md` is the supported Codex and Claude Code edition. `legacy/claude-v3/` stays archived. Hooks add deterministic collection, advisory warnings, and narrow enforcement. The skill and human/independent review remain responsible for semantic judgment.

## Architecture

Use Node.js 24 standard-library modules only. A single executable entry point reads one event JSON object from standard input. Thin Codex and Claude Code adapters normalize their native payloads into a shared event object, including every file in a Codex patch and Claude edit/write operations. A shared policy core resolves the canonical Git project and Agent-Team setup receipt, reads canonical state, evaluates deterministic prerequisites, creates atomic checkpoints/logs, batches lint work, and returns structured findings. Output adapters convert the shared decision into each runtime's documented JSON/exit contract.

The package contains declarative hook files for each runtime. The installer merges only Agent-Team-owned hook groups into user configuration, preserves unrelated settings, creates timestamped backups, prevents duplicate registration, installs one authoritative Codex skill under `~/.agents/skills/agent-team`, and installs the Claude copy under `~/.claude/skills/agent-team`. A legacy `~/.codex/skills/agent-team` copy is backed up and removed from discovery instead of being retained as a second active copy.

## Activation and state

Agent-Team policy activates only when canonical project state identifies an Agent-Team project/session. Canonical paths come from `git rev-parse --git-common-dir`, worktree metadata, and `.agent-team/setup.json`; a nested current directory is valid. Registered identity in `.agent-team/TEAMS.md` and the canonical task tracker outranks caller-supplied role text.

Use the smallest backward-compatible machine-readable state in `.agent-team/state.json` when deterministic gates need fields that Markdown cannot safely express. It stores pointers and provenance, not a second task ledger. Shared mutations use short atomic lock directories with owner/session metadata and atomic rename. A timestamp never proves that an owner is orphaned.

Activation logs contain only timestamp, runtime, skill identity, session identity, event kind, project/team identifiers, and non-sensitive correlation IDs. Claude Code logs reliable `Skill` tool and direct slash-command expansion events with deduplication. If Codex exposes no reliable skill-activation event, health reports activation logging as unsupported/degraded rather than inferring it from `SKILL.md` reads. Logs use user-only permissions, append locking, bounded rotation, and no prompts, arguments, file contents, credentials, or raw SQL.

## Policy behavior

- Startup/recovery is advisory and bounded. It distinguishes current, stale, and unavailable evidence. It never resumes or mutates work.
- Ownership, recognized integration/release operations, recognized destructive shared/production database operations, and explicit task-completion transitions enforce deterministic prerequisites. Missing evidence blocks with an actionable agent-facing reason. Existing scoped authorization proceeds without a new prompt.
- Test, migration, lint, and recurring-cost checks are advisory, changed-content focused, bounded, and deduplicated. Missing tools and unavailable checks are skips, never passes.
- Compaction/interruption checkpoints are automatic, atomic, idempotent, factual, and small. They preserve agent-authored decision/next-action notes and do not promise coverage after abrupt termination.
- Provider/MCP tools are matched through explicit configurable operation mappings. Shell detection tokenizes common command forms, including `git -C <repo> push`; it is documented as partial coverage, not a universal security boundary.
- Integration re-resolves the canonical repository and refreshes base/remote evidence immediately before the operation. Release remains separate from integration and checks whether updating remote main is itself a deployment trigger.

## Package and artifacts

`hooks/manifest.json` defines supported source files, adapters, registration targets, current versus legacy inclusion, and policy parity IDs. The package validator checks required files, frontmatter/version parity, local links, executable scripts, hook registration targets, platform coverage, and manifest inclusion. Its regression fixtures introduce deletions, version drift, broken links, missing adapters, and parity errors.

Archive validation is conditional. When given generated ZIPs, it rejects missing, stale, unexpected, duplicate, absolute, or path-escaping entries and compares both platform payloads with the manifest and source revision. Source-only installation reports that no archive was supplied instead of inventing one. Reproducible packaging uses normalized member ordering and timestamps where the archive tool permits it.

## Failure and trust behavior

Advisory hook failures are visible and non-blocking. Enforcement parser/runtime failures fail closed only for a recognized in-scope critical operation; ordinary and read-only operations continue. Codex uses supported `deny` or exit-2 behavior and never emits `permissionDecision: "ask"`. Claude Code uses its native exit/JSON rules. Installers never fabricate Codex trust. Health separates installed, registered, trusted, supported, and exercised. Any native `/hooks` trust step remains a named user action.

## Verification map

| Requirements | Required evidence |
| --- | --- |
| 1, 8 | Nested-repository recovery fixtures; current/stale/unavailable labels; no mutation; bounded Git/GitHub probes |
| 2 | Main/worktree, role/identity, shared/team-local, symlink, shell/provider, multi-file add/edit/delete/move tests; concurrent lock test |
| 3 | Authorized integration/release passes without prompt; wrong owner/revision/base, stale evidence, preview, pause/hold, delta, recovery, deployment-trigger failures block |
| 4 | Shared/live destructive test data and privileged client warnings; fixture/disposable and `Map.delete` false-positive tests; assertion-quality and dedupe tests |
| 5 | SQL layout/database-type, guards, environment, disposable execution, catalog verification, unenforced invariant, unavailable-check and dedupe tests |
| 6 | Recognized SQL client, provider/MCP, app-script entry, authorization, inventory, recovery, dry-run, cascade, fixture target, redaction and blind-spot tests |
| 7 | Batch/dedupe, monorepo config, changed files, exclusions, timeout, missing executable, bounded output, and no-install tests |
| 9 | Atomic/idempotent checkpoint tests, repeated events, note preservation, secret/data redaction, and runtime limitation documentation |
| 10 | Explicit completion event pass/block fixtures plus ordinary Stop/status/question/interruption non-trigger tests |
| 11 | Added/materially changed schedules warn; unrelated existing schedule edit does not; unknown live pricing stays unknown |
| 12, 13 | Reliable Claude activation paths, Codex unsupported health, rotation, permissions, malformed records, concurrent writers, audit/mistake cross-reference and limitations |
| 14 | Validator success and intentional missing file/version/link/adapter/registration/manifest/parity defect failures; CI deletion coverage |
| 15 | Both-platform archive success plus stale, omitted, unexpected, duplicate and path-escape failures; source-only N/A result |
| Both runtimes/install | Payload normalization/output contract fixtures, live safe runtime checks where supported, rerun idempotency, unrelated config preservation, backup, rollback, duplicate prevention, health dimensions |

## Documentation and release

Update `SKILL.md`, platform references, README, CHANGELOG, and semantic version for the compatible hook capability release. Document event mappings, modes, supported tool paths, blind spots, dependencies, tests, installation paths, health/audit/rollback commands, required trust steps, upstream reference revision/license, and differences from the prior instruction-only workflow. Create a private draft PR only; do not merge or publish a release.
