# Agent-Team implementation tasks

Canonical tracker: this file. Project owner and sole tracker writer: root orchestrator.
Scope: finalized F01–F17 and D01–D20 in AGENT_TEAM_UX_AND_EFFICIENCY_AUDIT.md, sections 9–13.
Authorization: 2026-09-08 user approved local implementation/testing, README and HTML updates, commits, push to origin/main, and GitHub distribution at thebpandey/agent-team after successful verification.
Integration branch: main. Baseline: 6feb76a0a2df42fcbbd42c5bc3df0d36396bee0e.
Plan: docs/superpowers/plans/2026-09-08-agent-team-approved-decisions.md.
No task is complete until implementation and applicable independent review/verification pass. Unavailable native checks remain unverified, not passed.

| ID | Task | Status | Priority | Owner | Dependencies | Acceptance / evidence |
| --- | --- | --- | --- | --- | --- | --- |
| AT-01 | Canonical tracker and actionable hook findings | integrated | P1 | root | none | 322bf89 repairs scoped outer-deadline regression; root exact-diff review and integrated 79/79 focused tests pass; manifest/full-suite/real-host gates remain open |
| AT-02 | Universal package and exact installation ownership | in_progress | P1 | installation_implementation | none | F03/F04: independently extracted install/reinstall/update/uninstall, selected host/scope, mixed/custom hooks preserved |
| AT-03 | Claims, parking, recovery, continuity and cleanup | in_progress | P1 | runtime_implementation | AT-01 | F07/F09/F10/F12/F17: RED tests/API preparation complete, production begun on 5822c45; prioritize any scoped review fixes |
| AT-04 | Readiness, prepared dependencies and role settings | pending | P1 | unassigned | AT-02 | F05/F06/F08/F14, D01–D19: three entry paths, prepared catalog, persistent host routing, exact exclusions |
| AT-05 | Compact status and optional web dashboard | in_progress | P2 | dashboard_implementation | AT-01, AT-03, AT-04 | Independent pure status/render/security work active; final role/writer bindings await producer APIs; real browser/bv gates separate |
| AT-06 | Skill instructions, context efficiency and usage evidence | in_progress | P1 | root | AT-01, AT-03, AT-04 | Instruction baseline/candidate scenarios recorded; usage aggregation 4/4 initial tests pass; native metrics/integration still open |
| AT-07 | Cross-host integration and release qualification | pending | P1 | unassigned | AT-01, AT-02, AT-03, AT-04, AT-05, AT-06 | F16 and all cross-cutting criteria: full suite, extracted matrix, real dependencies/hosts, recovery, browser and matched measurements |
| AT-08 | User documentation and GitHub delivery | pending | P1 | root | AT-07 | README/HTML/Markdown guide and diagrams reflect verified behavior; attribution; reviewed clean commit pushed and remote verified |

## Evidence and execution notes

- 2026-09-08 baseline: node --test tests/hooks-*.test.mjs — 135 passed, 0 failed, 0 skipped; 3680 ms. node hooks/agent-team-cli.mjs check-package — passed.
- Initial worktree contained three untracked user-owned documents: audit report, GETTING_STARTED.md, Getting_Started_with_Agent-Team.html. Preserve and include only in their authorized documentation scope.
- Referenced third-party bv integration is approved. Keep its upstream identity, Jeffrey Emanuel attribution, and MIT-with-OpenAI/Anthropic-rider notice. Do not vendor, relicense, or represent unrestricted permission; applicable upstream terms still control use and redistribution.
- Implementation and review findings are automatically assigned and repaired within scope. Only genuine access/authority/material decisions require user input. Continue independent tasks when one verification is unavailable.

## Findings

AT-01 review at 1c680a7: changes required; runtime owner is automatically repairing the following within its accepted scope. These must pass focused re-review before acceptance.

| Finding | Priority | Owner | Required verification |
| --- | --- | --- | --- |
| Deadline during ownership can fail open on consequential operations | P1 | runtime_implementation | After-read expiration and mapped destructive operations deny; advisory repair remains allowed |
| Mapped tracker completion loses affected-file ownership checks | P1 | runtime_implementation | Native/provider parity denies non-project owner tracker writes |
| Multi-row Markdown terminal transitions gate only first ID or none | P1 | runtime_implementation | Gate every changed task or reject ambiguity; existing terminal row cannot mask new transition |
| Malformed selected task table can look like current empty tracker | P2 | runtime_implementation | Invalid separator/blank ID unavailable; actual valid empty table remains distinguishable |
| Unbounded filesystem waits can exceed hook deadline | P2 | runtime_implementation | Bound discovery/log/cache waits, preserve completed lint batches and truthful persistence state |

Review evidence: immutable a2c2c1f..1c680a7 inspected by /root/runtime_review. Static exact-head findings; node -e reproduction was policy-blocked and not rerouted. Manifest integration and real-host/Beads gates remain open separately.

Follow-up: runtime_review resolved the five originals at 5822c45 and found one outer-deadline mapping-loss regression. Fixed at 322bf89 with a real FIFO-read regression. Root independently inspected 5822c45..322bf89, confirmed fallback classification precedes canonical read and shared state reuse, and reran integrated budget/policy/entrypoint/usage: 79/79 passed (4018 ms). No native-host claim follows from those fixtures.

AT-07 external verification constraint: native `codex` and `claude` launch commands are denied by the current command policy. Do not retry, wrap, change allowlists, or mark native journeys passed. Preserve this as an open publication gate while independent implementation and safe tests continue.

## Current ownership

- Integration: .worktrees/integration on codex/approved-decisions; root only.
- AT-01: .worktrees/runtime on codex/runtime-foundations; agent /root/runtime_implementation, gpt-6-astra/high. Baseline a2c2c1f.
- AT-02: .worktrees/installation on codex/installation-foundations; agent /root/installation_implementation, gpt-5.6-sol/high. Baseline a2c2c1f.
- AT-05: .worktrees/dashboard on codex/status-dashboard; agent /root/dashboard_implementation, gpt-5.6-terra/high. Baseline a2c2c1f. Own new status/dashboard modules/assets/tests only.
- Briefs and evidence: harness-artifacts/at-01-brief.md and at-02-brief.md; task reports at corresponding *-report.md paths. These are private implementation evidence, not a tracker.
