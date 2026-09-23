# Native setup and managed-update validation

## 8.0.12 repair checks

[PR #11](https://github.com/thebpandey/agent-team/pull/11) passed all 30 CI
checks and merged as `1ba08db17f38c82268d6da525dfeca05c008b5b3`. Local tests,
the canary using packaged executable bytes, and independent review passed.
The review covered measured executable sizes, per-file and aggregate rejection,
ordinary record limits, rollback, retry, and preservation of unrelated settings.

The [8.0.12 release record](releases/8.0.12-readiness.md) records the successful
release workflow, verified download, and completed managed update to manifest
revision 6. All four owned hashes matched, and both hosts had the top-level
skill without a nested duplicate.

The installed controller passed fresh TASKS.md setup, saved Sol/Luna and Opus
model normalization, byte-preserving replay, and both-host empty-start routing.
A 64 MiB fake Dolt store kept its Beads receipt stable and preserved leftover
TASKS.md. Evidence: `/tmp/agent-team-installed-smoke.ygh5gbnr/evidence.json`.
These controller checks do not claim a live Claude UI wizard test or universal
model availability. The separately published Project Kickoff 0.5.1 archive was
verified after download; all 34 installed files matched on both hosts.

## 8.0.11 first-use evidence

The [published release record](releases/8.0.11-readiness.md) links the final
revision and platform workflow. The historical local results below retain
their original scope; publication does not turn fixtures into live host tests. A later managed update
failed with `revision: lifecycle journal budget exceeded`; the
[8.0.12 repair](releases/8.0.12-readiness.md) addresses that separate defect.

## LeanCTX controller passthrough

A separately authorized local LeanCTX repair used the supported
`lean-ctx allow agent-teamctl` allowlist extension and preserved existing guard
settings. `excluded_commands` includes `agent-teamctl` and the actual installed
absolute binary path; the literal prefix matcher requires a simple command.
`lean-ctx raw` supports unusual compound invocations without disabling security
checks. These are optional local integration settings, not Agent-Team hooks.

Observed checks preserved controller version/status output, invalid-command
stdout/stderr and exit status, and a 23,023-byte JSON fixture. An unrelated
disallowed command still returned 126 under test-only enforcement. Global
warn/write settings were unchanged. Configuration was backed up before the
local repair; this does not claim a change on other users' machines.

## Original source validation

Date: 2026-09-22. Agent-Team branch: `fix/first-use-ux`.
Project Kickoff companion: `feat/agent-team-v8-handoff`, version 0.5.1,
commit `f4df06104d0614b5a5a27853fbeffe49f69328f2`. Earlier smoke evidence below
used companion commit `4c96f81`; it is not a new run of 0.5.1.

The development checks below were recorded before the [8.0.11 release](https://github.com/thebpandey/agent-team/releases/tag/v8.0.11), from source developed on v8.0.10. They include Claude's
`ef0d5db` fix as cherry-pick `da3f5a7`. The shared native controller/skill installation and `ai-training`
were not modified during source validation. The separately authorized global Claude
hook repair below changed only its six orphan hook handlers. Existing v8.0.10 downloads do not contain these repairs.

See [architecture changes](architecture-8.0.11.md) for host adapters, shared
admission, persistent intent and exact acknowledgements, deferred preparation,
ownership recovery, and executable-bound dependency provenance. Local results
below do not replace [revision-bound publication gates](releases/8.0.11-readiness.md).

## Causes and repairs

| Failure | Repair |
| --- | --- |
| Native skill installed below `agent-team-vnext/` | Both hosts receive `agent-team/SKILL.md`; updates reconcile verified historical/manual moves without replacing custom files. |
| Old Node hook failures | Native guidance requires no external hooks and resolves the installed controller even when it is absent from PATH. Unrelated hooks are not removed. |
| Bare `settings` error on a fresh project | Structured setup states name the missing inputs and next action. Approved setup creates only missing artifacts after validation. Ordinary setup does not require release cutover evidence. |
| `.beads` exceeds the canonical record limit | Fingerprint bounded metadata and the small Dolt manifest root hash, with a passive-export fallback; never walk the Dolt store. Keep the receipt digest a string and leave TASKS.md hashing unchanged. Check actual backend health before reporting the tracker prepared. |
| Markdown fallback cannot start | Root and designated Markdown trackers are supported through setup, admission, and persisted run validation. |
| Kickoff repeats setup or cannot hand off | Read approved 0.5.0/0.5.1 handoffs; prepare dependencies before binding setup; attach later handoffs without resetting settings or active packets. Default admission stays within approved task IDs. |
| First-run role settings absent | Store independent Codex/Claude preferences for orchestrator, developer/coder, reviewer, and visual reviewer. Saving accepted inherited defaults completes the first settings step. |
| Saved model choices become stale | Normalize only documented exact old IDs; preserve custom choices, effort, explicit inheritance, and extensions. Inspection stays read-only; an explicit settings save persists updates. Availability is reported separately. |
| Host switching rejects existing packets | Foreground replay returns the original packet and worker identity for observation. Exact launch acknowledgments and independent review checks remain enforced. |
| Dependency setup leaves manual gaps | One selected-bundle consent covers project-local pinned binaries and required uv/Python bootstrap. Beads can be declined in favor of TASKS.md. |
| Serena prompts then aborts | Pass explicit detected source languages to its pinned `project create` command. |
| Graphify refresh overwrites unrelated output or cannot retry | Extract into private staging, bind all generated files and source bytes, then publish verified output. Failed extraction preserves the previous graph. |
| Empty or README-only projects cannot finish planning | Report project analysis as deferred until real source and a commit exist; tool installation alone does not imply project readiness. |
| Interrupted tool installation leaves partial executables | Fully write and sync private staging before exclusive publication; retries preserve existing files, symlinks and concurrent destinations. |
| Markdown tasks lose execution details | Preserve Kickoff/custom table cells and prose while adding optional JSON columns for bounded task facts. Incomplete drafts stay blocked. |
| Beads closed history exhausts admission capacity | Read bounded active tasks and only referenced completed prerequisites; unrelated closed history does not consume the admission view. |
| Task and one-off commands report intent as success | Route queue/execute and bounded one-off requests through native handlers; reserved packets still require actual host dispatch and exact acknowledgement. Audit/review remain read-only. |
| Pause/resume loses actual worker state | Persist scoped admission controls, expose pending handles and held scopes, and require exact actual-host observations. A cleared admission hold does not prove worker resumption. |
| YAML validation adds a dependency absent from SBOM | Read linked Go module metadata from the same executable bytes that receive release checksums; verify manifest/SBOM correspondence and reject substituted provenance. |

## Intended user flow

1. Invoke `/agent-team setup` in Claude or `$agent-team setup` in Codex.
2. Reuse the project's existing approved tracker/plan, or ask for the unresolved
   tracker choice. Offer the selected dependency bundle once, including its
   scoped prerequisites. Reuse previous matching consent.
3. Install approved dependencies; defer source-dependent preparation when needed
   while planning continues. Approve missing scaffold files and import an approved
   Kickoff handoff without repeating its interview. Retry deferred preparation
   after source and a Git commit exist.
4. Show role models/effort and save accepted choices or inherited defaults.
5. Continue an original start request with a bounded packet and the host's real
   agent tool. A setup-only request ends with readiness.

The CLI is noninteractive; the installed skill conducts the short conversation
and follows structured `status`/`next_action` results. No answer is not approval.
Optional tools have explicit fallbacks. Required capabilities gate admission.

## Current local gate pass

Environment: Linux amd64, Go 1.27.1, Node 24.16.0. These checks use the
uncommitted 8.0.11 candidate worktree on base
`b376fdc3033b6fbebbbcc528540def9e0da894c0`, after implementation owners froze
their changes. They are local evidence, not a publication-ready commit identity.

| Gate | Result |
| --- | --- |
| `go test ./... -count=1` in `vnext` | Passed all packages. |
| `go vet ./...` in `vnext` | Passed. |
| `node --test tests/hooks-docs.test.mjs` | Passed 35 tests after the final numbered-picker edits, including batched inheritance and preservation of explicit choices. |
| `node --test tests/hooks-artifacts.test.mjs` | Passed all 26 tests in 110.6 seconds, including extracted-package consumer lifecycles. This run precedes the final numbered-picker prose; packaging code, contract, schema and paths are unchanged. Exact-commit artifacts remain a CI gate. |
| `go test -race ./internal/project ./cmd/agent-teamctl -run 'TestRole\|TestCurrentRole\|TestOneOffRole' -count=1` | Passed the changed role/default/migration and one-off profile cases. |
| Native host guidance pressure review | Passed six scenarios for each host: task execution, read-only one-off audit, pause, held resume, deferred preparation, and tracker/run next actions. |
| Numbered model wizard pressure review | Passed after resolving three ambiguities: Keep preserves inherited preference, the native picker is offered only when available, and retained effort is checked against a newly selected model. Stable mappings, cancellation without writes, and proven Claude alias equivalence were also checked. |

The first full Go attempt exposed stale shallow CLI expectations, a migration
fixture that assumed the machine's default branch was `master`, and a Kickoff
producer test running before its decoder edit was complete. Owners repaired the
tests/decoder and froze their files before the passing full rerun. The CLI
boundary now asserts refusal without a native handler and exact handler
forwarding; the migration fixture explicitly chooses its branch.

Two earlier artifact attempts hit the environment wrapper's 120-second limit
without a reported assertion failure. They are incomplete attempts, not passing
checks; the complete separate artifact run above supplies the recorded result.

Coverage includes real-built executable module provenance and SBOM tampering,
owned install migration, complete Markdown task facts, bounded Beads active and
prerequisite views, task and one-off dispatch, exact lifecycle observations,
read-only status, interrupted executable publication, deferred empty-project
preparation, Kickoff 0.5.0/0.5.1 compatibility, and role defaults/migration.

The companion owner also reported 73 passing tests with one skip and a verified
34-file package at the companion revision named above. That source/package
check does not repeat the earlier disposable upstream smoke or establish a
published companion release.

## Earlier repair-pass evidence

The earlier repair pass recorded the following checks. These are retained
historical results, not a claim that later edits were covered by the same run:

- `go test ./...` and `go vet ./...` in `vnext`.
- Race tests for preparation, project, tracker, run, start, installation, and CLI.
- Node documentation/artifact suite: 60 tests, no failures.
- Project Kickoff Python suite: 73 tests, one skip, no failures.
- Whitespace checks in both repositories.

Automated checks cover fresh setup, refusal and replay without user-file
rewrites, large Beads stores, both-host settings, designated Markdown trackers,
approved task scope, actual CLI admission, preserved packet identity, dependency
consent, archive validation, preparation failures, and installer migration.

The disposable upstream smoke used an isolated HOME/XDG environment and project
at `/tmp/agent-team-upstream-smoke.p2dQed`. It installed Beads 1.3.0, Serena 1.7.0,
Graphify 0.9.65, ast-grep 0.45.3, LeanCTX 3.10.2, and project-local uv/Python;
existing system ripgrep was reused. Beads initialized embedded Dolt, and
Graphify generated a real four-node/two-link Python/JavaScript graph. No active
Git hooks were installed. Beads appended its normal `.gitignore` exclusions.

That smoke exposed Serena's interactive language prompt. After repairing the
recipe, the exact explicit-language command succeeded against the downloaded
Serena package and generated `.serena/project.yml`. The exact readonly Beads
health command also succeeded with `summary.total_issues: 0`.

## Limits

- Host-agent dispatch is verified with fixtures and exact handle contracts;
  this does not claim a new Claude UI session was launched or provider models
  were exercised. Saved model preferences cannot switch the current parent
  model or create unsupported host capabilities.
- Serena preparation creates project configuration. It does not register an
  MCP server or certify every language server. Graphify uses offline code-only
  extraction; it does not claim semantic model analysis.
- Live workers remain attached to their original host. Switching foreground
  sessions does not transfer them or duplicate uncertain launches.
- Tests and local packaging are not publication. Deploying this candidate to
  the shared installation is a separate step, coordinated with ongoing Claude
  work. No release readiness evidence was fabricated.

## Global Claude hook repair

The live global settings still referenced the missing script
`~/.claude/skills/agent-team/hooks/agent-team-hook.mjs`. Six handlers matched the
legacy receipt exactly. On 2026-09-22, a scoped settings repair removed those six
handlers (18 to 12) and preserved all unrelated settings, plugins, and handlers.
Empty event groups were removed only when they contained no remaining handlers.
A guarded atomic replacement preserved the owner/group and mode `0600`.

Exact original backup:
`/home/server/.local/state/agent-team/config-backups/claude-orphan-hooks-JjSDX4/settings.json.original`.
Original SHA-256: `3a4a1eabf24681ad81ad708d1f17da33241394aeae863acf07696bddf0c11635`.
Repaired SHA-256: `e62b4a19aeb0cede5cebb824882f7bf9ba99a7e75ba8edfaafd6e77cedc2be9b`.

The old uninstall also processes skill directories and Claude agent definitions;
the settings-only repair was narrower. No project files, skill files, or legacy
receipts were modified by this repair. This verifies the settings change, not a
newly opened Claude session.

## Confirmed no-launch recovery guidance

The native skills now allow one bounded retry in the original uninterrupted
foreground attempt when the actual host response expressly guarantees no worker
was created or no follow-up was delivered. The packet and original owner remain
unchanged; current reservation, hold and native capability checks still apply.
An approved profile correction does not create a new reservation or authorize a
replacement worker. This is a skill exception, not a new controller API.

A fresh-context baseline review found that both original skills blocked the
proven no-worker case, while correctly rejecting timeout retries. A separate
fresh-context pressure review reached these decisions for both updated skills:

| Evidence and state | Permitted action |
| --- | --- |
| Express no-worker guarantee, same foreground, approved supported profile, unchanged unacknowledged packet, no holds | One retry of the original native spawn |
| Express no-delivery guarantee and unchanged retained handle, with the same checks | One retry of the original follow-up |
| Timeout, missing handle or caller inference | Observe the original host; no retry |
| New session or missing original evidence | Exception invalid; report recovery blocked and observe |
| Applicable pause/control hold | Preserve the hold; no retry |

That review also identified a general replay-profile wording ambiguity; both
skills now explicitly permit only the approved profile correction inside this
exception. The new documentation regression failed against the old text. All
36 `tests/hooks-docs.test.mjs` checks and the focused native release
Doc/Skill/Version/Historical checks passed after the change. No Go/controller
implementation changed. These are guidance and fixture checks: no actual host
model rejection, recovered spawn, or recovered follow-up was exercised. No
cross-session recovery capability is claimed.

Windows CI exposed a real abandoned-owner recovery failure after a child exited
and its final process handle closed. The Windows liveness probe now distinguishes
an absent process from an inaccessible or unverifiable process, after validating
the recorded PID and creation identity. Windows boundary tests cover invalid
identities/PIDs, foreign hosts and ambiguous API errors; the existing real-child
primary/recovery-claim tests remain enabled. Linux store/preparation tests and
Windows cross-compilation are local checks; native Windows CI must verify the
process-object disappearance path.
