# Agent-Team implementation tasks

Canonical tracker: this file. Project owner and sole tracker writer: root orchestrator.
Scope: finalized F01–F17 and D01–D20 in AGENT_TEAM_UX_AND_EFFICIENCY_AUDIT.md, sections 9–13.
Authorization: 2026-09-08 user approved local implementation/testing, README and HTML updates, commits, push to origin/main, and GitHub distribution at thebpandey/agent-team after successful verification.
Integration branch: main. Baseline: 6feb76a0a2df42fcbbd42c5bc3df0d36396bee0e.
Plan: docs/superpowers/plans/2026-09-08-agent-team-approved-decisions.md.
No task is complete until implementation and applicable independent review/verification pass. Unavailable native checks remain unverified, not passed.

| ID | Task | Status | Priority | Owner | Dependencies | Acceptance / evidence |
| --- | --- | --- | --- | --- | --- | --- |
| AT-01 | Canonical tracker and actionable hook findings | ready | P1 | unassigned | none | F01/F02/F13: tracker parity, preserved check batches, actual adapter diagnostics, bounded deadlines |
| AT-02 | Universal package and exact installation ownership | ready | P1 | unassigned | none | F03/F04: independently extracted install/reinstall/update/uninstall, selected host/scope, mixed/custom hooks preserved |
| AT-03 | Claims, parking, recovery, continuity and cleanup | pending | P1 | unassigned | AT-01 | F07/F09/F10/F12/F17: no duplicate writers, correct attributed recovery, retained authority, cleanup without deployment |
| AT-04 | Readiness, prepared dependencies and role settings | pending | P1 | unassigned | AT-02 | F05/F06/F08/F14, D01–D19: three entry paths, prepared catalog, persistent host routing, exact exclusions |
| AT-05 | Compact status and optional web dashboard | pending | P2 | unassigned | AT-01, AT-03, AT-04 | F11, D20, section 12: all tasks and teams, count parity, safe snapshots/live refresh, referenced bv attribution |
| AT-06 | Skill instructions, context efficiency and usage evidence | pending | P1 | root | AT-01, AT-03, AT-04 | F06/F14/F15/F17: selective loading, actual routing, compact recoverable handoffs, measured/unknown usage |
| AT-07 | Cross-host integration and release qualification | pending | P1 | unassigned | AT-01, AT-02, AT-03, AT-04, AT-05, AT-06 | F16 and all cross-cutting criteria: full suite, extracted matrix, real dependencies/hosts, recovery, browser and matched measurements |
| AT-08 | User documentation and GitHub delivery | pending | P1 | root | AT-07 | README/HTML/Markdown guide and diagrams reflect verified behavior; attribution; reviewed clean commit pushed and remote verified |

## Evidence and execution notes

- 2026-09-08 baseline: node --test tests/hooks-*.test.mjs — 135 passed, 0 failed, 0 skipped; 3680 ms. node hooks/agent-team-cli.mjs check-package — passed.
- Initial worktree contained three untracked user-owned documents: audit report, GETTING_STARTED.md, Getting_Started_with_Agent-Team.html. Preserve and include only in their authorized documentation scope.
- Referenced third-party bv integration is approved. Keep its upstream identity, Jeffrey Emanuel attribution, and MIT-with-OpenAI/Anthropic-rider notice. Do not vendor, relicense, or represent unrestricted permission; applicable upstream terms still control use and redistribution.
- Implementation and review findings are automatically assigned and repaired within scope. Only genuine access/authority/material decisions require user input. Continue independent tasks when one verification is unavailable.

## Findings

None from the fresh baseline. Historical findings are covered by the task mappings above, not duplicated here.
