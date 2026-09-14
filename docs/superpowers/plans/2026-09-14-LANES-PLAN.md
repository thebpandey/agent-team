# Native lanes implementation plan

> Use Agent-Team delegated implementation and independent verification with retained workers for related work. User approved proceeding without additional design checkpoints.

**Goal:** Deliver Pro 7.3.0 with durable lanes, bounded coordination, uppercase canonical Markdown, and no stale-session write lock.

**Architecture:** Extend existing canonical state and versioned native workflow routes. Keep task status in the selected tracker and reuse existing evidence, recovery, and cleanup machinery. The later explicit user instruction replaces session UUID ownership locking with project-scoped coordination continuity.

**Tech stack:** Node.js 24 standard library, node:test, Git worktrees, existing Codex/Claude adapters.

**Spec:** ../specs/2026-09-14-LANES-DESIGN.md

## Global constraints

- No Lite/archive edits, remote publication, fabricated native identity, or evidence loss.
- Keep path ownership, tracker single writer, review independence, completion/integration/release checks, exact revisions, clean-main, and retention.
- All new canonical Markdown filenames uppercase; SKILL.md at most 108 lines.
- No em dashes, emoji, or preambles in authored files.
- Write behavior tests first, record expected RED, implement minimal changes, record GREEN.
- Effective per-host defaults and limits are those in the approved spec.
- Runtime simulations stay in tests. Full logs stay in evidence files, compact receipts in parent context.

## Task 1: Session continuity repair

Files: hooks/lib/policy.mjs, hooks/lib/owner-recovery.mjs, relevant initialization/workflow adapters, tests/hooks-policy.test.mjs, tests/hooks-owner-recovery.test.mjs, and session-continuity tests.

Interface: replace permanent historical session locking with explicit project-scoped continuity; preserve existing registered worker path restrictions and consequential gate validity. Provide a genuine auditable owner transfer rather than rewriting historical evidence or pretending a user instruction is a native bootstrap token.

- [ ] Write regression: a fresh session editing unclaimed project documentation is not denied solely for a different session UUID; a conflicting registered worker remains restricted.
- [ ] Write transfer tests for preserving history, retaining claims/pending operations, and invalidating old gate authority without rewriting evidence provenance.
- [ ] Run node --test tests/hooks-session-continuity.test.mjs and confirm expected missing-behavior failures.
- [ ] Implement the narrow session continuity repair and supported transfer path; no new ability to approve a release through prose or synthetic identity.
- [ ] Run focused policy/ownership suites. Independently review exact changes before applying transfer to the real project.

## Task 2: Host settings and operational limits

Files: hooks/lib/settings.mjs, hooks/lib/tracker.mjs, hooks/lib/initialization.mjs, settings/setup request validators and their tests. Coordinate subprocess-limit call sites in task-transitions/recovery with Task 3.

Interface: export resolveExecutionSettings(setup, host) returning lanes/supervision/limits objects with validated defaults; settings wizard and mutations use the same validator. Defaults are snapshotted for runs.

- [ ] Test literal defaults, heartbeat 59 rejection and 60 acceptance, host profile independence, malformed values, cancellation, and a 501-to-1000 task initialization.
- [ ] Run focused settings tests to observe RED.
- [ ] Add shared defaults/validation and merge host-local execution changes without overwriting role routing or other hosts.
- [ ] Replace tracker/initialization hard caps with configured limits, including fresh-project initialization and downstream read compatibility.
- [ ] Run settings, initialization, tracker, and setup CLI tests; record exact counts.

## Task 3: Lane lifecycle and enforcement

Files: new hooks/lib/lanes.mjs and lane validators; hooks/lib/run-state.mjs, canonical-state.mjs, task-transitions.mjs, workflow-cli.mjs, operation.mjs, policy.mjs, recovery.mjs, cleanup.mjs, dashboard.mjs, status.mjs; hooks/agent-team-cli.mjs and agent-team-hook.mjs; tests/hooks-lanes*.test.mjs.

Interface: lane-create/next/rotate/close use closed request schemaVersion 1 envelopes and native command parsing. Validate state.lanes records and stable fingerprints. Reuse tracker claim transaction under the state lock; do not nest a state lock or separately persist an unbound queue advance. Refuse stale lane/tracker/revision, unknown liveness, unresolved prior dispatch, or unintegrated prior tasks.

- [ ] Test brief required fields/cap, lane schema/fingerprints, disjoint ownership, fact-sheet expiry, capacity reservation, immutable packet/result bindings.
- [ ] Test all four commands accepted through test-only native adapter and rejected without native identity or on chained/stale requests.
- [ ] Test interrupted claim write reconciliation, duplicate operation replay, rotation/rebind/current-task resume, and unknown liveness remaining occupied.
- [ ] Test verifier-first gate ordering and update final-content cap across write/edit/append routes.
- [ ] Test lane-close and cleanup rejection for any outstanding/unintegrated task or retained evidence/resource.
- [ ] Run node --test tests/hooks-lanes*.test.mjs for RED, implement, then GREEN.
- [ ] Extend canonical projections/recovery/status/dashboard with lane queue/current task/rotations/slots, preserving existing task behavior.
- [ ] Run related native route, policy, transitions, recovery, cleanup, dashboard, and status suites.

## Task 4: Context reduction and shipped helpers

Files: assets/helpers; hooks/lib/helpers.mjs and context-shrink.mjs; focused tests; setup/workflow CLI and health integration coordinated after Tasks 2/3.

Interface: pure candidate proposal; explicit reviewed selection writes host-supported project configuration and setup receipts; revert detects conflicts. helpers CLI copies files with recorded hashes and preserves customized targets. Helpers validate actual result evidence, fresh request bindings, and dotenv child isolation.

- [ ] Verify official current host controls for project MCP/plugin disable and retained-worker continuation before defining configuration writes.
- [ ] Test no-answer/cancel zero writes, selected disable preservation, revert/conflict behavior on fixture configs for supported hosts.
- [ ] Test helper self-checks, safely quoted paths, failed/unrun checks not becoming pass, child-only env, installation idempotency and customized-file drift.
- [ ] Observe RED; implement focused modules; run GREEN and relevant setup/health/CLI regression suites.

## Task 5: Canonical documentation and package migration

Files: SKILL.md, references/*.md, assets/claude-agents/*.md, new assets/templates/lanes/*.md, GETTING_STARTED.md, README.md, code path consumers, hooks/manifest.json, package validator/docs tests.

Interface: uppercase current Pro Markdown basenames while keeping native agent IDs and lowercase directory names. Previously persisted lowercase project evidence paths remain valid. references/LANES.md is the single detailed protocol owner.

- [ ] Add package validation for uppercase canonical basenames, required lane assets, resolving cross-links, and 108-line SKILL ceiling.
- [ ] Rename with updated links/consumers/manifests; do not rename archived content or user project docs.
- [ ] Write lane/packet/handover templates and full lane protocol; update host-specific conduct, heartbeat/model guidance, phase recovery, verifier-first review, context reduction, and helpers.
- [ ] Record the five upstream-inspired improvements without duplicate task authorities or unsupported enforcement claims.
- [ ] Run package/docs/installer tests and inspect native role filename compatibility.

## Task 6: End-to-end fixture, review, version, and handoff

Files: lane fixture/test under tests or assets, CHANGELOG.md, SKILL.md metadata and hooks/manifest.json version plus other current-package version consumers.

- [ ] Two-lane five-task dry run exercises three tasks on one lane, one rotation, every normal gate, both closes, and exact revision events.
- [ ] Run helper self-checks, node --test tests/hooks-*.test.mjs, and node hooks/agent-team-cli.mjs check-package --source . with full logs retained and exact counts reported.
- [ ] Delegate independent review of the complete diff against the approved spec. Repair findings, rerun affected tests, re-review.
- [ ] Bump version to 7.3.0 and changelog only after features pass; validate all version consumers and package checks.
- [ ] Build sealed artifacts in a temporary output, install to fixture home, and run health --project <fixture> --home <fixture-home> with no drift. Native trust/support remain separately reported, not fabricated.
- [ ] Provide exact validation commands/results and retained worktree location. No publication or destructive replacement of live installations without applicable authority.

## Coordination and current state

Implementation worktree: /home/server/dev/skills/agent-team/.worktrees/lanes-730, branch feat/lanes-730, base 04fd2be6e0cf5f1c712bae13cbf3feb719ab5dc0. Design and plan are authored in canonical docs; implementation stays in the isolated worktree.

Task 1 precedes live coordination-record transfer. Tasks 2 and 4 have independent new-module/test surfaces. Task 3 consumes Task 2 settings; serialize shared CLI/policy/canonical-state edits. Task 5 follows code changes so global path rewrites do not race workers. Task 6 validates the combined result. Do not claim tasks complete from another task's report.

Ruling: the user explicitly disabled the blocking PreToolUse handler and authorized removing stale session ownership blocking. Keep this exception scoped to session continuity, preserving other stated gates. Do not forge historical native approval or stopped-worker evidence.
