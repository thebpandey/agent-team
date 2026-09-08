# Agent-Team Approved Decisions Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use subagent-driven-development task review and test-driven-development. Agent-Team owns coordination; TASKS.md is the only task ledger. The approved parallel-worktree workflow overrides a dependency skill's serial-only dispatch or separate-ledger instructions.

**Goal:** Implement all finalized Agent-Team decisions, verify the actual user journeys, update the user documentation, and publish the verified result to the authorized GitHub repository.

**Architecture:** Extend the existing Node helpers and host adapters. The selected tracker remains authoritative; checkpoints reference it, status/dashboard derive from it, and only the existing orchestrator schedules engineering work. Keep instruction loading conditional on role and task.

**Tech Stack:** Node built-ins, node:test, Git worktrees, existing Markdown/JSON interfaces, native Codex and Claude Code adapters, approved third-party tools, plain HTML/CSS/JavaScript.

**Spec:** [Finalized audit decisions](../../../AGENT_TEAM_UX_AND_EFFICIENCY_AUDIT.md#9-review-outcome-and-decision-precedence), sections 9–13, plus the user's 2026-09-08 implementation/publishing authorization and referenced-third-party bv choice.

## Global constraints

- "Retain Beads and Markdown support."
- "Use one complete distribution package."
- "Do not ask the user whether to fix ordinary lint errors, test failures, or review findings, or whether to continue authorized work."
- "Beads is supported but not required for every project; a selected Markdown tracker remains a valid standalone path."
- "Never silently migrate a tracker because its backend is temporarily unavailable."
- "Preserve substantive independent review and required tests."
- Prepared capabilities do not mean every skill is loaded into every agent.
- Ordinary findings trigger bounded repair, not a completion waiver. Scope, credentials, security, explicit pauses and preview/release authority remain binding.
- Serena and Microsoft Playwright CLI are mandatory prerequisites; selected default and optional profiles follow D01–D20 exactly. Do not install discarded packages or the deferred ast-grep companion bundle.
- Keep the installed development harness stable. Test candidate installations in isolated roots; do not overwrite personal hooks, skills, credentials, or MCP configuration to test setup.
- Keep native auto-compaction enabled as fallback. Do not claim automatic parent replacement or continuation after host exit without actual host support.
- Referenced beads_viewer remains Jeffrey Emanuel's third-party component under its complete MIT-with-OpenAI/Anthropic-rider terms. Link the official repository and license, do not vendor or white-label its engine, and do not assert universal license eligibility.
- Use one canonical TASKS.md for this implementation. Plans define work, evidence reports prove work, CONTEXT.md is a recovery index; none is a shadow task ledger.
- Root controls shared integration and GitHub writes. Push origin/main only after required acceptance gates pass. No force push, unrelated staging, credential disclosure, or automatic external deployment target creation.

## Ownership and shared contracts

The root orchestrator owns this plan, TASKS.md, main/integration history, shared interface decisions, top-level skill/instruction changes, README/guides, and final acceptance. Worker ownership is exclusive and temporary; a later task acquires files only after the former writer has stopped and its changes have been integrated.

| Producer / consumer | Contract and integration rule |
| --- | --- |
| AT-01 → AT-03/05/06 | Preserve resolveProject(cwd) and loadCanonicalState(project). Extend the result with explicit selected tracker identity, current/unavailable/invalid state, and a revision/fingerprint; normalized tasks retain IDs, status, owner, priority and dependencies. Never infer an empty authoritative list from a failed read. |
| AT-01 → AT-03/06 | Hook checks expose passed/failed/skipped/timeout results, affected files and bounded diagnostics; long diagnostics are recoverable. One event budget caps all probes. Final gates and edit advice remain distinct. |
| AT-02 → AT-04/07 | installPackage({sourceRoot, home, host, scope, projectRoot}) and matching uninstall preserve selected targets and return managed receipts. Hosts: codex, claude-code, both. Scopes: user, project. Ambiguous host is resolved from trusted host metadata or explicit options, never guessed from a prompt. |
| AT-03 → AT-05/06 | Logical claims, stopped/active/unknown writers, parked/paused compute, pending operations and evidence are distinct fields. Reuse existing state/checkpoint files, not another database. |
| AT-04 → AT-05/06 | Settings hold per-host role defaults/overrides, availability/enforcement evidence, selected catalog and capability receipts. Read-only inspection does not write or install. Future settings do not rewrite active runs. |
| AT-05 → AT-07/08 | One status read model supplies terminal and dashboard counts. Snapshot generation is explicit or event-driven; live refresh is opt-in, loopback-only and never an execution controller. |
| AT-06 → AT-07/08 | Skill/recovery packets identify actual source records and verification. Metrics distinguish observed usage from estimates/unknowns. No unmeasured savings claims. |

Collision review: AT-01 owns hook/policy wiring while AT-02 owns CLI/manifest/install wiring. Both may read but must not change the other's files. New focused test files avoid concurrent changes to shared test helpers. AT-03 and AT-04 inherit their respective streams after integration. Root integrates later CLI registrations and global documentation serially. The tasks' acceptance tests below match these same contracts; no task may replace the canonical tracker or declare unsupported host behavior implemented.

## Execution and test method

For each task, write a regression/acceptance test before production changes, run it and preserve the relevant expected failure, implement the narrow change, then rerun the focused tests. Use realistic subprocess/fixture boundaries; an assertion on the code's own expected-entry list is not independent artifact verification. Preserve existing valid coverage and replace obsolete decision assertions with the newly approved behavior.

Each task has one independent review with separate requirements and code-quality verdicts. Workers report exact revision/diff, RED/GREEN commands and results, affected files and remaining concerns. Root handles cross-task wiring, then reruns affected checks. Required findings remain open until fixed and verified; a retry cap does not turn them into optional improvements.

## Task AT-01: Canonical tracker and actionable hook findings

**Files:** Modify hooks/lib/project.mjs, canonical-state.mjs, operation.mjs, lint.mjs, policy.mjs, output.mjs, hooks/agent-team-hook.mjs. Create focused tracker and budget modules only where they remove duplication. Add tests/hooks-tracker.test.mjs and tests/hooks-budget.test.mjs; update affected lint/entrypoint tests. Do not edit installer, CLI, manifest, global docs or shared fixture helpers.

**Interfaces:** Consume existing setup/project/host event structures. Produce the canonical-state and hook-result contracts in the table above. Preserve existing exported APIs where possible; communicate a necessary signature change before another task uses it.

- [ ] Add equivalent Beads/root TASKS/.agent-team TASKS ownership, completion and release cases, including linked worktrees and backend failure.
- [ ] Add subprocess cases where a failed batch, followed by a missing executable or timeout, still appears in both host outputs with completed batches preserved.
- [ ] Run `node --test tests/hooks-tracker.test.mjs tests/hooks-lint.test.mjs tests/hooks-entrypoint.test.mjs` and record the expected pre-fix failures.
- [ ] Implement explicit tracker selection, normalized reads and source freshness. Run supported bd JSON operations with bounded subprocesses; use fixtures for exhaustive failure cases and keep real-bd verification for AT-07.
- [ ] Aggregate result states without dropping prior results; pass bounded actionable text through the actual adapters; preserve recoverable detailed output. Apply one event deadline and reuse the event snapshot without reusing stale evidence at consequential actions.
- [ ] Run focused and affected policy/foundation tests. Commit only owned files and return the exact revision for review.

Minimum observable assertion pattern (use the real exported entrypoint and controlled host payload):

```js
assert.equal(result.decision, 'allow'); // advisory edit continues to repair
assert.match(result.messages.join('\n'), /src\/broken/);
assert.match(result.messages.join('\n'), /failed|error/i);
assert.ok(result.capabilities.lint.batches.some(batch => batch.exitCode === 0));
```

The final required-check fixture must separately deny completion of that same failing revision.

## Task AT-02: Universal package and exact installation ownership

**Files:** Own hooks/lib/install.mjs, artifacts.mjs, package-validator.mjs, hooks/manifest.json, hooks/agent-team-cli.mjs, both hook registration JSON files, tests/hooks-install-health.test.mjs, hooks-artifacts.test.mjs, hooks-package.test.mjs, hooks-cli.test.mjs, and both .github/workflows/*.yml files. Do not edit health.mjs, runtime/policy or shared test helpers.

**Interfaces:** Produce selected-host/scope installer calls above, a complete archive, and distinct source-versus-installed-package validation. Keep Node built-ins and current public behavior unless the approved decisions change it.

- [ ] Add a test extracting the real built ZIP independently, then installing and validating it outside the source checkout. Assert both adapters and executable permissions exist.
- [ ] Add host/scope selection, unknown/ineffective flags, mixed handler siblings, custom owned commands, repeat/update/uninstall and rollback tests.
- [ ] Run `node --test tests/hooks-artifacts.test.mjs tests/hooks-install-health.test.mjs tests/hooks-cli.test.mjs` and preserve expected failure evidence.
- [ ] Implement one complete distribution, consumer-valid installed checks, handler-level receipts and selective safe configuration. Preserve unrelated group metadata/order, handlers, customized resources and other host/scope files.
- [ ] Update nonpublishing CI/package steps for the complete artifact; do not run release publication while implementing.
- [ ] Verify focused tests and all affected package tests. Commit owned files and return revision for review.

The extracted test must inspect consumer results independently, for example:

```js
assert.equal(installed.status, 'passed');
assert.deepEqual(await readFile(unselectedHostConfig), beforeUnselectedHost);
assert.equal((await stat(extractedEntrypoint)).mode & 0o111, 0o111);
```

Use an actual ZIP extractor, not the builder's expectedEntries function, to establish extraction behavior.

## Task AT-03: Claims, parking, recovery, continuity and cleanup

**Files:** Own hooks/lib/lock.mjs, recovery.mjs, checkpoint.mjs, health.mjs, new focused task-transitions.mjs and cleanup.mjs if needed, and focused tests/hooks-transitions.test.mjs, hooks-recovery.test.mjs, hooks-health.test.mjs, hooks-cleanup.test.mjs. Coordinate hook/CLI registrations through root.

**Interfaces:** Consume AT-01 canonical identity/freshness. Transition inputs identify expected owner/version, operation identity, requested action and evidence. Results distinguish applied, duplicate, conflict and unavailable. Recovery identifies project/task/session/worktree/revision and refers to authoritative records.

- [ ] First test simultaneous claims, stale expected versions, unknown writer liveness, explicit pauses, parked claim retention and an interrupted operation that must not be repeated.
- [ ] First test recovery selecting the correct session even when another task has a newer checkpoint, evidence invalidation after a source change, and per-event unsupported/unobserved/failed health states.
- [ ] First test integrated verified work with deployment disabled is cleanup-eligible only after stopped writers and dirty/untracked/ignored/user/preview checks.
- [ ] Implement minimal idempotent transitions using supported tracker atomics or its verified single-writer model. Markdown writes remain serialized under one owner.
- [ ] Extend existing checkpoints with authored next action/decision pointers and pending-operation uncertainty; verify stopped-writer/handoff evidence before fresh-context continuation. Preserve gates and explicit pause intent.
- [ ] Run the focused tests and affected integration tests, record evidence outside disposable checkouts, and commit owned files for review.

Acceptance includes two competing claim attempts yielding exactly one owner, no claim theft from an unknown writer, and no deletion of a fixture worktree containing an ignored or untracked user file. Hooks must not manufacture semantic decisions from raw event metadata.

## Task AT-04: Readiness, prepared dependencies and role settings

**Files:** Own new focused hooks/lib/settings.mjs, readiness.mjs and dependencies.mjs plus dependency catalog/profile data, and their dedicated tests. Acquire install.mjs from AT-02 only when needed after its writer stops. Root owns CLI wiring and skill reference text.

**Interfaces:** Consume selected host/scope installer and project setup data. Produce saved per-host role routing, future-run defaults, capability receipts with configured versus enforced values, and catalog preparations with selected/optional/unsupported/error states.

- [ ] Add tests for Kickoff handoff using develop/Beads, existing-plan adoption, and standalone task readiness without a Kickoff dependency or redundant planning.
- [ ] Add tests for one-role edits, native/numbered choices, cancel-without-write, custom settings preservation, concurrent changes and switching hosts without losing previous routing.
- [ ] Add tests for mandatory/default/optional catalog selection and excluded packages, prerequisite order, missing-runtime handling, compatible-install reuse, truthful functional receipts and role-specific instruction selection.
- [ ] Implement first-use preparation of Serena/Playwright and selected defaults using verified official installation paths and explicit component versions/profiles. Keep optional Context7/dashboard disabled until selected, and retain auth/trust/admin boundaries.
- [ ] Implement targeted settings with full wizard available, capability-aware role menus, approved fallback/escalation and immutable active-run routing unless the user explicitly changes it.
- [ ] Run focused tests, preserve actual install probe evidence and unresolved external prerequisites, and commit owned files for review.

Observable contracts include a settings inspection leaving every file byte-identical, a host round-trip restoring prior overrides, and a missing unrelated UI capability leaving the Beads selection unchanged.

## Task AT-05: Compact status and optional web dashboard

**Files:** Own hooks/lib/status.mjs and dashboard.mjs, dedicated dashboard HTML/CSS/JS assets, and tests/hooks-status.test.mjs and hooks-dashboard.test.mjs. Coordinate entrypoint/manifest additions through root.

**Interfaces:** Consume canonical task/team records and saved model/effort metadata. Produce one read model with counts, scope, freshness, active/parked/ready states, and explicit unknowns. Render terminal and dashboard from this same model.

- [ ] Test all-task visibility, parent/child deduplication, cancelled/deferred exclusions, zero-task N/A, stale/unknown source and recorded-versus-live distinctions.
- [ ] Test explicit atomic snapshots, unchanged-input skipping, last-good snapshot retention after refresh failure, escaped malicious task text, and TASKS-only operation.
- [ ] Implement plain local HTML with teams, overall progress, searchable/filterable complete tasks, freshness and dependency graph section. A file reload reads the latest saved snapshot, not a live tracker.
- [ ] Implement optional loopback-only read/refresh helper with fixed resource routes, request/origin/path guards, bounded subprocesses and no arbitrary shell or task execution API.
- [ ] Integrate selected external bv through its documented export/robot interface after fresh bd JSONL export; preserve upstream attribution and license identity. No binary/source/assets vendoring or replacement scheduler. Test the interface with explicit fixtures; real bv evidence is separate.
- [ ] Verify keyboard, responsive and actual browser behavior with Playwright, plus helper security and cleanup. Commit owned files for review.

Read-only status must never install dependencies, run recovery, write checkpoints, start agents or launch a server. Dashboard enablement/refresh may write only its own derived artifacts; it must not mark tasks complete.

## Task AT-06: Skill instructions, context efficiency and metrics

**Files:** Root owns SKILL.md, references/*.md, assets/claude-agents/*.md and agents/openai.yaml. A delegated worker may own hooks/lib/telemetry.mjs and focused metrics/context helpers/tests under explicit assignment. Do not edit user-wide installed skills.

**Interfaces:** Consume the verified helper behavior and approved catalog. Instructions explain the real native controls and helpers; they must not invent a command, menu, enforced model or auto-parent replacement.

- [ ] Run baseline consuming-agent scenarios for mandatory four-skill loading, blocked-slot retention, host-routing resets, lost pending operations and repair permission prompts. Record observed behavior, distinguishing simulated instruction tests from native runs.
- [ ] Replace universal ceremony with a small common contract and conditional role/task references. Preserve complete reading of applicable instructions by each fresh worker and substantive review/TDD.
- [ ] Document autonomous bounded repair, parked compute/retained claims, cleanup without deployment, three readiness paths, attribution, selective catalog loading and exact exclusions.
- [ ] Implement observed all-role usage/repair/time metrics, with estimates and unknowns labeled, and no new proxy or transcript database. Keep soft budgets advisory and hard budgets checkpointing without waiving acceptance.
- [ ] Verify fresh-context packets recover constraints, approved/rejected decisions, tracker authority, pending operations, writer safety and next action from source records only. Keep native compaction as fallback.
- [ ] Repeat instruction scenarios with the candidate; fix observed gaps. Run targeted runtime checks, inspect reference links and commit for independent review.

## Task AT-07: Cross-host integration and release qualification

**Files:** Own tests and nonpublished evidence under harness-artifacts/, approved command-scenario fixtures, and release-qualification documentation. Root serializes any production fixes through the responsible worker.

- [ ] Run the full integrated node:test suite, check-package and nonpublishing artifact build/check steps on the exact candidate.
- [ ] Build twice and compare complete archive checksums; independently extract. Execute source/ZIP × Codex/Claude/both × project/user scope (12 core cases), with install/reinstall/update/uninstall and preserved unselected settings.
- [ ] In isolated environments, run useful operations with the selected real dependencies and record versions, licensing links and discovery from relevant workers/worktrees.
- [ ] Run actual Codex and Claude setup, reload/trust, all three project-entry paths, developer/reviewer execution, automatic repair, park/continue/resume, role setting changes, cleanup without deployment and fresh-context recovery. Exercise actual compaction restoration separately from fabricated hook payloads.
- [ ] Verify dashboard browser flows, all-task count parity, freshness, live refresh without an agent turn, hostile task text and restricted helper routes.
- [ ] Run repeated matched representative baseline/candidate tasks, including review/repairs/startup/recovery; record accepted outcomes, available token usage, elapsed time and unknown metrics. Compare native versus narrowed LeanCTX only; no discarded-tool benchmark.
- [ ] Obtain a whole-candidate independent review and reconcile every F/D decision against evidence. Required unavailable checks remain explicitly unverified and prevent a successful-release claim.

## Task AT-08: User docs and authorized GitHub delivery

**Files:** README.md, GETTING_STARTED.md, Getting_Started_with_Agent-Team.html, CHANGELOG.md, version metadata and lightweight original diagram assets if useful. Root owns final Git operations.

- [ ] Re-read the official project-kickoff repository and preserve its installation/setup/invocation flow before Agent-Team in the standalone HTML guide.
- [ ] Update README and both guides to verified installation prompts, Git/GitHub CLI authentication, prerequisite order, automatic versus user-controlled steps, reload/hook trust, settings, execution, recovery and dashboard usage.
- [ ] Add readable original infographics/flowcharts for entry paths, orchestration/repair and dashboard data flow. Keep the HTML standalone, deep-green/emerald/neon accents, white text, accessible collapsible descriptions/instructions/reference examples and full upstream links.
- [ ] Verify documentation commands against the candidate, links and rendered layouts. Preserve third-party attribution and separate licenses; do not include private test transcripts or secrets in the package or commit.
- [ ] Review exact staged changes, rerun affected checks, commit the verified candidate, fetch/check origin/main and integrate concurrent upstream changes safely if any.
- [ ] Push the verified main update to the configured thebpandey/agent-team origin without force; observe GitHub checks and fix in-scope failures. Publish only the repository's existing authorized distribution workflow after its tests pass, with exact revision/checksum evidence.
- [ ] Verify the remote revision and distribution, record remaining limitations honestly, then clean only stopped, integrated, verified task-owned disposable resources while preserving evidence.

## Delivery interpretation

The destination supplied is a GitHub skill repository, not an application hosting target. Deliver the repository and its existing GitHub distribution artifacts; do not create an unrelated Vercel/Pages/cloud deployment, new public dashboard, or service. Existing release workflow details and actual access are verified before publication.
