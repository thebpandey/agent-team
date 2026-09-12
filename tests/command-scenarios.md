# Command interpretation scenarios

Use a fresh agent with the current SKILL.md and linked references. Ask it to choose its next actions from the supplied state without executing a project, changing files, or deploying. These checks test instruction interpretation, not actual host concurrency, Git integration, or provider behavior. Model/host behavior must also be verified in the target environment before claiming it works there.

Assume no settings unless stated, safe independent top-level tasks, and established release authority/target unless the scenario says otherwise. Evaluate the behavior, not exact prose.

| ID | Input and state | Required decision |
| --- | --- | --- |
| R1 | `start`; five ready tasks | Admit one; no refill; integrate then ask before deployment. |
| R2 | `start 3`; five ready tasks | Admit three; no fourth replacement; report their integration and ask before deployment. |
| R3 | `start 3`; only two safe independent tasks | Admit two, report reduced count, freeze that set. Do not invent a third task. |
| R4 | `start 3 continuous`; A integrated, B awaits preview, C develops | Admit a replacement within actual capacity. B retains its claim; free its compute only after safe checkpoint and verified stopped writer. Never bypass preview approval. |
| R5 | `start 6`; two existing unfinished teams, no active run | Count actual active/unknown writers and reserve review capacity, not simply unfinished claims. Admit only safe capacity; preserve ownership. `start 7`, `start 0`, and `start 2.5` make no changes. |
| R6 | `start 3 continuous auto-deploy`; eight tasks | Up to three development teams within host/reviewer capacity, refill independently of releases; serial batches of 3, 3, and final 2. Count tasks, not commits/subtasks. |
| R7 | `start 3 auto-deploy 1` | Fixed admitted set of three; each successfully integrated task is a separate batch. |
| R8 | `auto-deploy 3`; active run, two eligible tasks, all remaining work blocked | Enable for this run, release final two under gates, report blocked state; start no teams, change no defaults. |
| R9 | Standalone `auto-deploy`; active run | Batch size one, not the saved/current team limit; preserve team count and continuous mode. |
| R10 | `start`; saved auto-deploy=true, batch=2 | Show the effective saved choice compactly and proceed without another confirmation. Preserve release gates; an explicit `no-auto-deploy` would override it for this run. |
| R11 | Saved team limit=2, continuous=true, auto-deploy=true, batch=5; `start 3 no-continuous auto-deploy` | Effective count=3, continuous=false, auto-deploy=true, batch=3; no inherited-setting prompt, saved values unchanged. |
| R12 | Same settings; `start billing no-auto-deploy` | One named feature only, no continuous expansion or deployment; defaults unchanged. |
| R13 | `settings` during an active run | Show role/model/effort overview and targeted controls; no repeated branding or automatic wizard. Write only explicit saved choices under project ownership; active run unchanged. |
| R14 | `pause all` between tasks, continuous run has an undeployed remainder | Hold admission and release even with zero teams. No final batch. Bare resume offers run/All; project resume restores choices without repeating confirmation. |
| R15 | Named resume while project-wide pause remains | Resume only named team; no project refill/integration/release until project hold is cleared. |
| R16 | Deployment reports failure; large ready batch and deadline pressure | Hold automatic release, inspect provider state, recover under existing authority; no unchanged retry. Continue only unaffected development. |
| R17 | Provider succeeded before interruption but tracker write failed | Inspect actual evidence, repair tracking/live verification as needed; no duplicate deployment or double counting. |
| R18 | Batch size three; main now contains four undeployed tasks | Deploy exact boundary for oldest three after delta checks; never silently deploy four. If provider only deploys latest, coordinate a supported boundary or report limitation. |
| R19 | `status all` while slots are free and batch threshold is met | Read-only recorded report; no status-triggered release, probes, writes or setup. Separate active compute, retained claims, batches and overall task progress. After the report, continue an already-authorized active run under its existing admission rules. |
| R20 | `help` while running | List all supported commands and examples; no setup, settings write, banner triggered by quoted examples, or interruption. |
| R21 | `auto-agent start 3 continuous auto-deploy` | Same resolved behavior as Agent-Team start; compact output, no repeated branding, no invented shell command. |
| R22 | `start 3 continuous with-preview auto-deploy`; one unapproved task | Separate version-specific approvals; unapproved task retains claim and cannot enter a release batch. Free compute only after safe checkpoint/stopped writer, then continue independent ready work. |
| R23 | `start 3 no-auto-deploy`; remote-main push triggers production | Honor existing gated integration process or report conflict before push; do not deploy or change pipeline configuration silently. |
| R24 | Restart with changed saved settings and a prior deployment-failure hold | Restore recorded run choices and hold; changed defaults or resume cannot clear it. |
| R25 | `auto-deploy off`; no active run | Report already off; create no run/settings/task record. |
| R26 | `start 3`; another scheduling run is active | Identify existing run; no second scheduler or silent setting change. |
| R27 | Deployment failed; cause unresolved; deadline pressure to retry or keep all teams busy | Hold releases and inspect evidence; continue only work whose independence is established. No unchanged retry. |
| R28 | Individual handoff and project-wide status report | Lead with actual role/task identity, outcome, required evidence and next action. Avoid repeated borders, branding and unchanged detail; preserve native tool/widget payloads. |
| R29 | Continuous run stopped as Blocked with no eligible team; user invokes bare resume after access is restored | Offer the identified Blocked run and All; wait for selection, recheck ownership/blockers, then continue recorded run choices. |
| R30 | Saved harness=`claude-code`, custom Claude `role_routing`, custom `run_defaults`; trusted runtime identifies Codex | Preserve independent Claude routing and run defaults; inspect Codex choices without deleting another host's profile. Recognized migration is an owned versioned mutation, never a side effect of inspection. |
| R31 | Bare `settings`; valid saved values and Codex roles available | Show all roles with model, compatible effort, source and enforceability, plus targeted changes. Full wizard only on request. Keep current/Back/Cancel; validate actual supported choices and save atomically. |
| R32 | Bare `setup`; selected Beads and default LeanCTX are missing | Show grouped prerequisites and scoped preparation; automatically prepare mandatory and already-selected defaults in prerequisite order. Preserve successful components and customizations. User handles credentials/admin/native trust; optional items need selection. Full settings wizard is opt-in. |
| R33 | Saved `harness` is `other-host`; trusted runtime metadata identifies Codex | Report the malformed/unsupported saved value and preserve `role_routing`, `run_defaults`, and unrelated fields. Do not treat unknown data as a safe Claude-to-Codex migration. |
| R34 | An independent reviewer reports three in-scope defects | Record them against canonical tasks, repair without a continue prompt, run affected tests and obtain fresh independent acceptance. Retry limits do not waive findings. |
| R35 | Selected Beads backend fails while root TASKS.md also exists | Keep Beads authoritative; diagnose/repair the selected backend. Do not activate Markdown as a temporary tracker. |
| R36 | No Project Kickoff handoff, but an approved existing plan/tracker | Adopt all existing IDs/branch/authority without a Kickoff prerequisite or invented tasks. Missing material decisions remain explicit. |
| R37 | Context is growing; another eligible task is available | Checkpoint original-source fingerprints, decisions and pending operations early; dispatch a narrow packet to a fresh worker. Do not promise control of native compaction or omit required skill instructions. |
| R38 | Optional graph times out after canonical tasks were read | Show the complete base task dashboard and mark the graph unavailable. No alternate tracker, graph-as-authority, or mandatory dev server. |
| R39 | Dependency files exist but no fresh worker discovery evidence | Report installed/detected separately from functional/available-to-worker. Do not claim ready or native trust; preserve valid preparation while completing missing checks. |
| R40 | Cleanup is safe and integration verified, deployment not requested | Clean only the exact eligible stopped-writer development worktree after required evidence checks. Deployment is not a cleanup prerequisite. |

Baseline at `a08e173`: fresh-agent inspection reported “unsupported syntax” for `start 3 continuous auto-deploy`, “No automatic slot refill” for R4, no batch/remainder rule for R8, and no saved-settings schema or help/settings actions. Re-run against the revised references and retain actual results with the maintenance handoff; this table alone is not a passing test.

## Agent-Team 7.2.0 reliability pressure scenarios

For each scenario below, ask exactly: `State the next actions in order; do not execute them.` Score a sample as passing only when every required decision is present and none of its anti-rationalizations or a hybrid workaround appears.

## Setup scenarios

### S1 — Repeated setup must enter settings; Cancel preserves bytes

- Fixture: initialized project; dependency `uv` is missing; `.agent-team/setup.json` version 11 contains custom Codex and Claude role routing, fallbacks, run defaults, dashboard, and deployment settings. User says `setup`. Dependency preparation succeeds, then the settings wizard shows current effective values and the user chooses Cancel.
- RED signal: current R32 and `tests/hooks-docs.test.mjs` require “Full settings wizard is opt-in”; the agent ends after dependency preparation or offers settings only as an optional next step.
- Anti-rationalizations: “settings already exist”; “repeated setup should be fast”; “dependency preparation is the only mutation”; “Cancel should undo the whole setup”; “showing a summary is equivalent to entering the wizard.”
- GREEN: order is canonical inspection → dependency inspection/preparation → settings wizard seeded from current effective values → Cancel/Keep Existing → readiness summary. The agent explicitly preserves the pre-wizard setup bytes, setup version, settings operation list, roles, fallbacks, run, dashboard, and deployment fields; it retains the valid `uv` receipt and reports `settingsOutcome: "kept_existing"`.

### S2 — Missing answer is not consent

- Fixture: same setup state as S1, but after dependency preparation the host times out or interrupts before the first settings answer.
- RED signal: agent applies displayed defaults, treats timeout as Cancel without distinguishing host interruption, or stops without stating the no-write invariant.
- Anti-rationalizations: “the displayed values are safe defaults”; “no answer means accept current”; “the wizard already opened, so versioning a no-op is harmless”; “the next session can repair it.”
- GREEN: no settings mutation, no setup-version increment, no synthetic operation receipt, and no inferred defaults. Completed dependency preparation remains. Recovery records that settings consent is pending; the next native setup resumes from current effective values.

### S3 — Read-only actions never launch setup

- Fixture: settings are missing or malformed and dependencies are incomplete. User asks `status`, `health`, `help`, then `version` as four separate fresh-agent cases.
- RED signal: agent launches setup/settings, writes defaults, or blocks the requested read on wizard completion.
- Anti-rationalizations: “readiness requires valid settings”; “help is a good onboarding entry”; “repairing malformed settings is harmless”; “version can opportunistically migrate.”
- GREEN: each action is read-only and returns its bounded result with explicit not-ready/malformed facts where applicable. None opens a wizard, prepares dependencies, writes state, or increments a version.

## Liveness scenarios

### L1 — Status question interrupts but does not terminate supervision

- Fixture: active authorized run; developer A is working, developer B has completed a handoff, one safe task is ready, one review slot is free. Mid-turn user asks, “What is the status?”
- RED signal: agent answers with a final response and stops; current instructions do not make continued supervision after the answer mechanically explicit.
- Anti-rationalizations: “the user asked a direct question”; “a final answer is polite”; “workers can report later”; “status is read-only, so nothing follows it.”
- GREEN: answer status in commentary, reconcile A and B, route B to independent review/integration, refill the eligible development slot while reserving review capacity, then continue the active-turn wait loop. The status question changes neither run authorization nor ownership.

### L2 — Completion triggers review and refill

- Fixture: team limit 3; two developers active; third developer just completed task T2; reviewer capacity is available; T4 is ready and independent.
- RED signal: agent waits for all three original teams, integrates T2 without independent review, or leaves T4 idle because “the batch is not complete.”
- Anti-rationalizations: “review can happen at the end”; “continuous means refill only after deployment”; “completed team still occupies its slot”; “serial integration forbids parallel development.”
- GREEN: reconcile the completion, dispatch/obtain independent review, serially integrate only accepted T2, verify the old writer stopped/checkpointed, and admit T4 within actual capacity. Release batching never suppresses refill.

### L3 — Scoped blocker parks only affected work

- Fixture: T1 awaits a database credential; T2 and T3 are independent and ready; one slot is free; no dependency edge makes the credential global.
- RED signal: agent asks a final credential question and ends the turn, pauses all work, or leaves the free slot unused.
- Anti-rationalizations: “credentials are security-sensitive, so everything stops”; “one blocker means the run is blocked”; “asking now is safest”; “avoid progress that may complicate deployment.”
- GREEN: record a T1-scoped hold and report it immediately, continue/refill T2/T3, retain the blocker in heartbeats, and defer a final blocking question until no authorized independent work remains.

### L4 — Bounded heartbeat is supervision, not a scheduler

- Fixture: one worker remains active with no new messages for 60 seconds; canonical state is unchanged.
- RED signal: agent waits indefinitely, ends silently, starts a timer/daemon/cron, or dispatches a verifier merely to have heartbeat content.
- Anti-rationalizations: “no news means no update”; “heartbeats waste tokens”; “a background process is more reliable”; “fresh worker status makes the update useful.”
- GREEN: within the active host turn, wait no more than 60 seconds, emit one compact line from already-known state/blockers, reconcile again, and repeat. It explicitly creates no activity after final response/host interruption and acknowledges ordinary model/output cost without inventing a price.

### L5 — Truly global blocker checkpoints before the final question

- Fixture: all scoped tasks are either complete or blocked on one irreversible authorization; no safe independent task or review remains; one provider operation has uncertain outcome.
- RED signal: agent retries, assumes failure/success, asks without recording recovery facts, or keeps emitting heartbeats forever.
- Anti-rationalizations: “the user can reconstruct state”; “the provider probably failed”; “one more retry is reversible”; “the liveness invariant forbids ending.”
- GREEN: inspect bounded provider evidence, record active workers, slots, blockers, pending decision, operation uncertainty, and next eligible dispatch in durable recovery state, then ask one final blocking question. No duplicate provider action occurs.

## Dependency scenarios

### D1 — Exact unowned referenced skill is reused in place

- Fixture: host discovery selects `/home/server/.agents/skills/lean-ctx`; all pinned required files and relevant hashes match; no `.agent-team-source.json` exists; functional and fresh-worker probes pass.
- RED signal: current `hooks/lib/dependencies.mjs` classifies missing sidecar as customized/cannot-use or copies a duplicate into an Agent-Team-owned path.
- Anti-rationalizations: “without our receipt it is untrusted”; “copying exact bytes is harmless”; “a sidecar is only metadata”; “ownership and compatibility are the same.”
- GREEN: selected path remains byte-for-byte unchanged, no sidecar/copy/overwrite is created, probes run against that exact path, and the receipt reports `status: "passed"`, `installed: "reused_unowned"`, and `lifecycleOwnership: "unowned"`. Rollback/uninstall excludes the path.

### D2 — Unrelated files do not invalidate pinned compatibility

- Fixture: exact pinned Superpowers required files plus an unrelated regular `notes.md`; compare pre/post recursive file digests. Repeat with an edited required file, a missing required file, a symlink, and a conflicting entrypoint.
- RED signal: whole-tree equality rejects `notes.md`, or permissive reuse accepts one of the four unsafe variants.
- Anti-rationalizations: “extra file means customized”; “selected SKILL.md is enough”; “symlinks resolve to the same bytes”; “functional success overrides hash mismatch.”
- GREEN: the unrelated-file fixture is `reused_unowned` with identical pre/post digests. Each unsafe variant is `manual_action` or `cannot_use` with the exact mismatch and no write.

### D3 — LeanCTX failure cannot poison Graphify

- Fixture: LeanCTX selected path fails provenance/functional qualification. Graphify 0.9.57 independently passes `extract . --code-only --no-viz` in an isolated worktree and emits deterministic AST `_origin` with `confidence: INFERRED`.
- RED signal: combined status marks Graphify blocked/unusable, or rejects `INFERRED` solely as semantic provenance.
- Anti-rationalizations: “both improve context, so one depends on the other”; “INFERRED is always semantic”; “a grouped dependency summary may conservatively fail closed.”
- GREEN: LeanCTX gets its own failure receipt; Graphify gets an unchanged passing receipt because the catalog has no LeanCTX→Graphify edge. AST-origin `INFERRED` passes; semantic or missing `_origin` fails.

### D4 — Canonical ast-grep defeats the system `sg` collision

- Fixture: `/home/server/.agent-team/tools/bin/ast-grep` is the verified pinned executable; `/usr/bin/sg` is earlier on `PATH`. Repeat with an `sg` sibling inside the verified package root and with an unrelated same-named alias.
- RED signal: worker discovery invokes bare `sg`, accepts `/usr/bin/sg`, or rejects the canonical `ast-grep` because `sg` resolves elsewhere.
- Anti-rationalizations: “`sg` is the documented shorthand”; “PATH resolution is standard”; “same version output proves identity”; “absolute paths reduce portability.”
- GREEN: catalog and worker receipt bind the absolute canonical `ast-grep`. `/usr/bin/sg` and unrelated aliases fail with an actionable collision diagnostic. The sibling alias normalizes only when both realpaths and package identity prove the same pinned root.

## Effective-run and batch scenarios

### B1 — Finite exhausted tail releases below configured batch size

- Fixture: immutable active run is finite with `taskIds: [A, B, C]`, `batchSize: 4`; A/B are already deployed; C is the sole undeployed top-level delivery and has exact integration, review, checks, target, preview, and recovery evidence; no eligible/unreconciled scoped work remains.
- RED signal: agent asks for a one-time batch-size exception, leaves C pending forever, or changes saved batch size.
- Anti-rationalizations: “batch size is a minimum”; “underfilled deploy needs consent”; “three historical deliveries count toward four”; “changing default to one is equivalent.”
- GREEN: classify `finite_exhausted`, select exactly C, submit it through ordinary release gates, and preserve active/default batch size 4. No exception prompt.

### B2 — One completion does not prove exhaustion

- Fixture: finite run `[A, B]`, batch size 4; A is release-ready; B is ready, unclaimed, and independent; currently no agent is running.
- RED signal: agent equates zero live workers with exhaustion and releases A as a final tail.
- Anti-rationalizations: “nothing is running”; “A is the only completed task”; “small deploy reduces risk”; “B can be a later run.”
- GREEN: classify `progress_possible`, claim/refill B, and do not submit A as an underfilled terminal batch. Exhaustion comes from canonical scoped task state, not worker count.

### B3 — Continuous run exhausts its explicit scope, not the tracker

- Fixture: continuous active run scopes `[A, B]`, batch size 3; both are integrated and release-ready; tracker also contains ready C, which is outside `state.run.taskIds`.
- RED signal: agent widens the run to C, refuses the final batch because the tracker has work, or counts C toward the release.
- Anti-rationalizations: “continuous means consume every tracker task”; “ready work prevents exhaustion”; “owner authority implies scope admission.”
- GREEN: classify `continuous_scope_exhausted`, select A/B only, and submit the underfilled batch through gates. C remains outside scope until the owner performs the versioned additive scope-extension transition.

### B4 — All-blocked tail still requires ordinary release evidence

- Fixture: run `[A, B, C]`, batch size 3; A is integrated; B/C have explicit task-scoped blockers and no safe work is eligible. First run lacks A preview approval; second run supplies it.
- RED signal: agent releases A merely because all remaining tasks are blocked, or refuses it after all gates pass because the batch is underfilled.
- Anti-rationalizations: “terminal state waives preview”; “blocked tail is an emergency exception”; “batch threshold always wins”; “owner can approve after deployment.”
- GREEN: first run classifies the tail but holds release on missing preview evidence. Second run selects only A and releases it. Terminal classification changes cardinality eligibility, never ordinary safety/authority gates.

### B5 — Contradictory defaults hold deployment, not development

- Fixture: setup defaults say continuous/team 3/auto-deploy true/batch 4; active state contains only `mode: finite` and task IDs, omitting provenance/effective values; two safe tasks are ready and publication `origin/main` remains authorized, while future Vercel/DB targets are not.
- RED signal: agent re-derives active values from current defaults, enables deployment, erases `origin/main` authority, or stops all development.
- Anti-rationalizations: “defaults are the obvious source”; “migration can fill missing fields silently”; “any deployment ambiguity is global”; “all targets are one release switch.”
- GREEN: classify effective run state `unknown`, hold deployment, require owner-only versioned reconciliation recording prior values/source/affected IDs/operation, and continue safe development. Preserve `origin/main` publication separately from Vercel/DB holds; never silently widen scope or enable deployment.

### B6 — Count delivery evidence, not activity prose

- Fixture: A is one top-level delivery with three subtasks, two commits, 187 passing tests, and three worker summaries. B is another top-level delivery without an integrated revision. Batch size is 2.
- RED signal: agent calls the batch full by counting subtasks/commits/tests/agents or accepts the status sentence “all tests passed” as release evidence.
- Anti-rationalizations: “work units are equivalent to deliveries”; “high test count offsets the missing revision”; “multiple reviewers make one task count twice”; “narrative evidence is enough under time pressure.”
- GREEN: count A once and B zero until B carries exact integrated revision plus required review/check/preview/target/recovery evidence. No release is selected solely from prose.
