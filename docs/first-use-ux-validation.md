# First-use repair candidate

Date: 2026-09-22. Agent-Team branch: `fix/first-use-ux`.
Project Kickoff companion: `feat/agent-team-v8-handoff`, commit `4c96f81`.

This is an unpublished source candidate based on v8.0.10. It includes Claude's
`ef0d5db` fix as cherry-pick `da3f5a7`. The shared installation and `ai-training`
were not modified. Existing v8.0.10 downloads do not contain these repairs.

## Causes and repairs

| Failure | Repair |
| --- | --- |
| Native skill installed below `agent-team-vnext/` | Both hosts receive `agent-team/SKILL.md`; updates reconcile verified historical/manual moves without replacing custom files. |
| Old Node hook failures | Native guidance requires no external hooks and resolves the installed controller even when it is absent from PATH. Unrelated hooks are not removed. |
| Bare `settings` error on a fresh project | Structured setup states name the missing inputs and next action. Approved setup creates only missing artifacts after validation. Ordinary setup does not require release cutover evidence. |
| `.beads` exceeds the canonical record limit | Fingerprint bounded metadata and the small Dolt manifest root hash, with a passive-export fallback; never walk the Dolt store. Keep the receipt digest a string and leave TASKS.md hashing unchanged. Check actual backend health before reporting the tracker prepared. |
| Markdown fallback cannot start | Root and designated Markdown trackers are supported through setup, admission, and persisted run validation. |
| Kickoff repeats setup or cannot hand off | Read approved 0.5.0 handoffs; prepare dependencies before binding setup; attach later handoffs without resetting settings or active packets. Default admission stays within approved task IDs. |
| First-run role settings absent | Store independent Codex/Claude preferences for orchestrator, developer/coder, reviewer, and visual reviewer. Saving accepted inherited defaults completes the first settings step. |
| Host switching rejects existing packets | Foreground replay returns the original packet and worker identity for observation. Exact launch acknowledgments and independent review checks remain enforced. |
| Dependency setup leaves manual gaps | One selected-bundle consent covers project-local pinned binaries and required uv/Python bootstrap. Beads can be declined in favor of TASKS.md. |
| Serena prompts then aborts | Pass explicit detected source languages to its pinned `project create` command. |
| Graphify refresh overwrites unrelated output or cannot retry | Extract into private staging, bind all generated files and source bytes, then publish verified output. Failed extraction preserves the previous graph. |

## Intended user flow

1. Invoke `/agent-team setup` in Claude or `$agent-team setup` in Codex.
2. Reuse the project's existing approved tracker/plan, or ask for the unresolved
   tracker choice. Offer the selected dependency bundle once, including its
   scoped prerequisites. Reuse previous matching consent.
3. Complete preparation and approve missing scaffold files. Import an approved
   Kickoff handoff without repeating its interview.
4. Show role models/effort and save accepted choices or inherited defaults.
5. Continue an original start request with a bounded packet and the host's real
   agent tool. A setup-only request ends with readiness.

The CLI is noninteractive; the installed skill conducts the short conversation
and follows structured `status`/`next_action` results. No answer is not approval.
Optional tools have explicit fallbacks. Required capabilities gate admission.

## Verification

Passed:

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
