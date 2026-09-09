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
| AT-02 | Universal package and exact installation ownership | integrated | P1 | root | none | 639a1fb final scoped review accepted; independent6/6 permitted crash/ownership tests pass, dead-claim node-e fixture static only; merged integration5e735f3; final extracted matrix remains |
| AT-03 | Claims, parking, recovery, continuity and cleanup | integrated | P1 | root | AT-01 | d312db4 resolves seven originals plus immutable archive interruption review; integrated at12114e2; root87/87 affected tests and actual selected bd qualification pass; CLI/producers/final native gates remain |
| AT-04 | Readiness, prepared dependencies and role settings | integrated | P1 | root | AT-02 | Final9ed9b115 independently accepted44/44; mergedebd8c7d; root57/57 docs/settings/readiness/dependencies; initializer0b47c15 and setupCLIdfe6e4f in review; native Playwright/fresh-worker discovery remains open |
| AT-05 | Compact status and optional web dashboard | integrated | P2 | root | AT-01, AT-03, AT-04 | be4575e final scoped review accepted27/27, actual final JSON static/mobile/live browser passed; merged5e735f3; CLI/manifest/automatic refresh wiring remains |
| AT-06 | Skill instructions, context efficiency and usage evidence | in_progress | P1 | root | AT-01, AT-03, AT-04 | d01efeb refreshed guides/selective instruction and docs assertions; usage7/7 incl FIFO safety, docs13/13; native metrics/CLI/producer integration still open |
| AT-07 | Cross-host integration and release qualification | in_progress | P1 | root | AT-01, AT-02, AT-03, AT-04, AT-05, AT-06 | CLI05b67c5 accepted/integrated; producer e8e6d7a root76/76; graph44c4459 review; initializer0b47c15 root50/50 after three P1 repairs, re-review; setupCLIdfe6e4f review; permitted wide299/299 with forbidden fixture excluded; final matrix/native gates remain |
| AT-08 | User documentation and GitHub delivery | in_progress | P1 | root | AT-07 | README rewritten with prompt-first onboarding/green Mermaid flows; docs13/13, HTML/Markdown local-preview guides updated; final API alignment/version/qualification/publication remain |

## Evidence and execution notes

- 2026-09-08 baseline: node --test tests/hooks-*.test.mjs — 135 passed, 0 failed, 0 skipped; 3680 ms. node hooks/agent-team-cli.mjs check-package — passed.
- Initial worktree contained three untracked user-owned documents: audit report, GETTING_STARTED.md, Getting_Started_with_Agent-Team.html. Preserve and include only in their authorized documentation scope.
- Referenced third-party bv integration is approved. Keep its upstream identity, Jeffrey Emanuel attribution, and MIT-with-OpenAI/Anthropic-rider notice. Do not vendor, relicense, or represent unrestricted permission; applicable upstream terms still control use and redistribution.
- Implementation and review findings are automatically assigned and repaired within scope. Only genuine access/authority/material decisions require user input. Continue independent tasks when one verification is unavailable.
- AT04 final fixes isolate production probe XDG directories and SERENA_HOME and strip inherited Beads/Git routing; contaminated selected-bd sentinel remains unchanged in reported real isolation tests. Earlier LeanCTX probe changed shared active boundary to /tmp, and earlier Serena probe may have touched default ~/.serena. No global reset/bypass; side effects disclosed, earlier non-isolated runs are not clean-isolation evidence.
- Initializer independent review found malformed state activation, validated-owner/tracker TOCTOU, and task-ID subset omission. Root reproduced negative/race cases and repaired in0b47c15;50/50 named tests pass including SIGKILL recovery. Independent re-review pending; source-denied worker stopped without edits or relayed contents.
- Runtime worker inadvertently ran an inherited node-e fixture in a broad suite. That broad run is excluded from qualification. Root permitted wide suite explicitly excludes `a dead stale-lock recovery claimant is reclaimed without displacing a new owner`; no denied-command retry or wrapper bypass is authorized.

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

AT-02 repair ledger: seven exact findings plus selector/partial-removal gaps in harness-artifacts/at-02-review.md. They remain required within AT-02, not deferred backlog: unowned resources, uninstall/update history, retired hooks, installed-source ownership, preexisting handler update, durable interruption recovery, actual extracted CLI matrix. Installer owner automatically repairs and returns for scoped review.

AT-03/AT-05 follow-ups: harness-artifacts/at-03-review.md and at-05-review-followup.md. These are required repair work in their existing canonical tasks, not new approval requests.

AT-07 real evidence: isolated selected bd 1.2.2 fixture passed canonical linked-worktree read, same-owner idempotent claim, conflicting claim rejection and fresh export. HTML guide passed actual isolated Chromium at 1440x1000, 375x812 and 812x375 with keyboard toggles and no external requests. Evidence in harness-artifacts/native-dependencies and guide-browser. Neither substitutes for native host or actual bv qualification.

## Current ownership

- Integration: .worktrees/integration on codex/approved-decisions; root only.
- AT-01: .worktrees/runtime on codex/runtime-foundations; agent /root/runtime_implementation, gpt-6-astra/high. Baseline a2c2c1f.
- AT-02: .worktrees/installation on codex/installation-foundations; agent /root/installation_implementation, gpt-5.6-sol/high. Baseline a2c2c1f.
- AT-05: .worktrees/dashboard clean idle be4575e, reviewed/merged. Graph wiring44c4459 in .worktrees/installation under lifecycle_review; setupCLIdfe6e4f in .worktrees/setup-cli under installation_implementation review. Initializer0b47c15 in .worktrees/initialization under readiness_implementation review. Root owns all integration wiring/manifests/docs; no concurrent writers.
- Briefs and evidence: harness-artifacts/at-01-brief.md and at-02-brief.md; task reports at corresponding *-report.md paths. These are private implementation evidence, not a tracker.
