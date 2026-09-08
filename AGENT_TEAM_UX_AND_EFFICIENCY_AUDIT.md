# Agent-Team: Codex UX, autonomy, and efficiency audit

Date: 2026-09-08

Status: all 17 core decisions and all 20 dependency decisions finalized and consolidated on 2026-09-08. The optional web dashboard and context-continuity designs are approved. Implementation is not authorized by this report.

Reading order: section 10 is the final core decision record; section 11 is the final dependency policy and 20-item ledger; section 12 specifies the approved optional dashboard; section 13 lists remaining implementation evidence and authority boundaries. Section 9 summarizes the review outcome.

Sections 1–8 are preserved verbatim as the original audit snapshot, including historical proposals and test results. They are not the current implementation instructions where later decisions disagree. In particular, RTK comparisons, Backlog.md migration/pilots, and adoption of GSD, Ralph, or beads_rust are superseded and excluded; beads_viewer is now selected for optional dashboard integration, subject to licensing clearance. Project Kickoff remains optional, not an Agent-Team prerequisite.

These are approved future design decisions, not claims that the changes have been implemented, dependencies installed, licenses cleared, or new native-host/efficiency tests passed. The user authorized this documentation consolidation only. No code, guides, runtime configuration, hooks, trackers, or installations are changed by this update.

## 1. Executive recommendation

Keep Agent-Team's orchestrator-led, parallel implementation model. Make its execution contract smaller and more deterministic before adding another orchestration framework.

The strongest existing features are isolated worktrees, one integration/release owner, revision-bound verification, truthful recovery, and one authoritative task tracker. These are valuable accuracy controls. The largest opportunities are removing repeated workflow ceremony, making the Project Kickoff handoff authoritative, and ensuring the actual hook implementation supports the workflow the instructions promise.

Three correctness findings deserve attention before UX polish:

1. The hook runtime reads only `.agent-team/TASKS.md`, despite the documented support for Beads and an existing user-selected task file.
2. Ordinary lint failures can disappear between the lint runner and the host-visible hook response.
3. Runtime-specific distribution archives omit a file that the bundled installer still requires.

There is also a configuration-preservation defect: an unrelated hook sharing a group with Agent-Team can be removed with that group.

For efficiency, the best first changes are role-specific instruction loading, a short setup path, compact status output, and eliminating duplicate planning after Kickoff. Do not equate more agents, more skills, or more compressed output with better results. Measure tokens and elapsed time **per correctly accepted task**, including repairs.

Recommended direction: **Project Kickoff → validated handoff → one orchestrator → bounded worker/reviewer pool → verified integration → next ready task.** Deployment remains a separate, explicitly scoped policy.

## 2. Scope, evidence, and limitations

### Reviewed baseline

| Component | Baseline |
| --- | --- |
| Agent-Team | v6.5.0, commit `6feb76a0a2df42fcbbd42c5bc3df0d36396bee0e` |
| Project Kickoff | v0.3.0, sibling checkout at commit `89e6228611b1dd726c6aa0363e7206d0c72fa16d` |
| Review coverage | Root instructions; all 25 references; host routing; current role assets and package layout; hook entry points and all library modules; automated test/scenario coverage; installation, state, recovery, integration, deployment, cleanup; Kickoff setup/handoff contract |
| External research | Current official Codex documentation and primary GitHub repositories, licenses, and public repository metadata |

The two existing, untracked getting-started guides were preserved. No skill behavior, dependencies, settings, hooks, task state, branches, or production systems were changed for this report.

### Checks actually run

| Check | Result |
| --- | --- |
| `node --test tests/hooks-*.test.mjs` | **135 passed; 0 failed; 0 skipped**, approximately 3.69 seconds reported by the test runner |
| `node hooks/agent-team-cli.mjs check-package` | **Passed**, no reported errors |
| Instruction-size inventory | Root `SKILL.md` plus references: **30,604 words**; root alone: **2,290 words** |
| Installed four-skill startup inventory | Ponytail 1,079; Using-Superpowers 485; Impeccable 1,533; LeanCTX 454: **3,551 words** combined |
| Repository metadata | Stars, license metadata, archive state, and push dates queried on the audit date |

These are source and contract findings, not a production incident investigation. The full test suite passes but does not establish that every documented workflow works in a real Codex session. Native Codex version/TUI probing was blocked by the active shell allowlist; additional scratch-fixture preparation was also blocked. Those restrictions were respected. No interactive Codex/Claude end-to-end run or alternative-tool benchmark was performed.

Word counts are **not tokenizer measurements or billable-token totals**. The 30,604-word figure is the whole reference library, not a claim that every run loads it all. Installed dependency contents can differ across users. Cost and speed benefits below are hypotheses unless explicitly described as observed source behavior.

### Priority and evidence labels

- **P1 — next release:** likely breaks an intended path, loses important feedback, or materially obstructs the principal autonomy/efficiency objective.
- **P2 — following iteration:** significant usability, reliability, or resource-efficiency improvement.
- **P3 — optional experiment:** replacement or expansion that needs evidence before adoption.
- **Confirmed:** directly established by source behavior or conflicting instructions; not necessarily reproduced in a live host.
- **Risk/proposal:** a plausible failure mode or design improvement requiring scenario/benchmark validation.

No P0 production incident or universal security bypass is claimed.

## 3. Prioritized findings

| ID | Priority | Finding | Evidence type | Relative effort |
| --- | --- | --- | --- | --- |
| F01 | P1 | Hooks do not consume the selected canonical tracker | Confirmed | Medium |
| F02 | P1 | Lint failures and useful diagnostics can be lost | Confirmed | Small |
| F03 | P1 | Distribution archives and installer disagree | Confirmed | Medium |
| F04 | P1 | Mixed hook groups can lose unrelated hooks | Confirmed | Small–medium |
| F05 | P1 | Kickoff handoff is not one enforced compatibility contract | Confirmed conflicts | Medium |
| F06 | P1 | Every role pays for all four dependency skills and repeated receipts | Confirmed overhead; savings unmeasured | Small–medium |
| F07 | P1 | Critical scheduling transitions are mostly instruction-driven | Confirmed architecture; reliability risk | Medium–large |
| F08 | P1 | Setup/settings require too many decisions for repeat users | Confirmed | Small–medium |
| F09 | P2 | Blocked/approval-waiting teams retain scarce slots | Confirmed | Medium |
| F10 | P2 | Cleanup depends on production deployment | Confirmed | Medium |
| F11 | P2 | Terminal output emphasizes ceremony over actionable state | Confirmed design | Small |
| F12 | P2 | Health and recovery output do not communicate the evidence users need | Confirmed | Small–medium |
| F13 | P2 | Hook work is bounded locally, but not by one overall event budget | Confirmed structure; latency unmeasured | Medium |
| F14 | P2 | Model routing needs explicit capability profiles and host-specific persistence | Confirmed constraints; proposal | Medium |
| F15 | P2 | There is no end-to-end efficiency budget or outcome benchmark | Confirmed gap | Medium |
| F16 | P2 | Tests need user-journey and multi-agent contract coverage | Confirmed coverage gap | Medium |

Effort is comparative, not a delivery estimate.

### F01 — P1: honor the actual tracker in the hook runtime

**Evidence.** [State instructions](references/state.md) permit Beads or one existing designated local tracker. However, [project.mjs:72](hooks/lib/project.mjs#L72) fixes the runtime path to `.agent-team/TASKS.md`. [canonical-state.mjs:44](hooks/lib/canonical-state.mjs#L44) reads Markdown tables from that path; there is no Beads task reader in this flow. [policy.mjs:169](hooks/lib/policy.mjs#L169) checks release IDs against those rows, and [policy.mjs:229](hooks/lib/policy.mjs#L229) does the same for completion. Recovery also checks that fixed file.

**Impact.** A valid Beads project, or Kickoff project using root `TASKS.md`, can appear to have no canonical tasks to the hooks. A recognized completion/release can then be denied. Conversely, closing a Beads task through an unmapped shell command is not the documented Markdown completion transition. Creating a second Markdown task ledger to satisfy the hook would violate the single-tracker design.

**Recommendation.** Resolve a versioned tracker adapter from the setup receipt. Support the selected absolute Markdown path and the selected Beads executable/backend. Read current task IDs, ownership, and state through that adapter. Map supported completion operations explicitly. If a bounded read snapshot is necessary, make it derived, revision/freshness-bound, and non-authoritative; never require humans or models to maintain two ledgers.

- **Pros:** fixes the core Beads path; preserves Kickoff decisions; reduces confusing repair loops.
- **Cons:** adapters need compatibility tests, bounded timeouts, and an explicit unavailable-state policy.
- **Acceptance:** Beads, root `TASKS.md`, and `.agent-team/TASKS.md` each pass the same ownership/completion/release scenarios without creating another tracker. Missing/stale evidence remains unavailable, never a pass.

### F02 — P1: propagate lint failures through to the agent

**Evidence.** [lint.mjs:72](hooks/lib/lint.mjs#L72) returns top-level `status: "checked"` after a normal run, including when individual batches have `status: "failed"`. [policy.mjs:303](hooks/lib/policy.mjs#L303) only announces top-level `failed` or `timeout`. [output.mjs](hooks/lib/output.mjs) serializes messages, not the lint results stored in `capabilities`.

**Impact.** A normal nonzero linter exit can produce no host-visible lint failure message. Even a timeout lacks useful bounded diagnostics. Agents may rerun checks unnecessarily or miss an advisory that was supposed to support accuracy. This does not prove final verification is bypassed; the independent required checks still matter.

**Recommendation.** Aggregate failed batch outcomes correctly and include a short diagnostic containing affected files, failure category, and a bounded error excerpt or recoverable log pointer. Preserve already-completed batch results if a later config lacks an executable. Keep advisory lint separate from final acceptance gates.

- **Pros:** immediate actionable feedback; fewer duplicate tool calls; better accuracy.
- **Cons:** diagnostic output needs redaction, deduplication, and size limits.
- **Acceptance:** a fake linter exiting nonzero is visible in actual adapter stdout for both hosts; mixed pass/fail/skipped batches stay distinguishable; no unbounded logs.

### F03 — P1: make distribution and installation one coherent product path

**Evidence.** [manifest.json](hooks/manifest.json) excludes `hooks/claude-hooks.json` from the Codex ZIP and the opposite adapter from the Claude ZIP. Both remain in `manifest.files`. [artifacts.mjs:114](hooks/lib/artifacts.mjs#L114) applies those exclusions, while [install.mjs:151](hooks/lib/install.mjs#L151) hashes every manifest file before installing both runtimes. An extracted runtime ZIP therefore lacks an input the installer requires. Package validation also assumes source-checkout support files not shipped in these archives.

The source installer always targets both hosts and user scope. It has no runtime/scope selection, despite broader documentation discussing project-scoped installation. The CLI's permissive flag reader can accept an unknown flag without making it effective. This is especially confusing when a user expects a Codex-only install.

**Recommendation.** Choose one distribution contract: a complete universal package with an explicit target selector, or genuinely self-contained runtime packages with runtime-specific manifests and installation validation. Add a preview of exact targets; reject unsupported flags and missing values. Distinguish “skill plus lifecycle hooks” from a native marketplace plugin unless a corresponding host plugin package actually exists.

- **Pros:** a novice can follow one tested install route; fewer scope surprises; recoverable updates.
- **Cons:** artifact metadata and installation receipts need migration handling.
- **Acceptance:** build each ZIP, extract into an isolated fixture, install its intended host, verify health, reinstall, then uninstall. A Codex-only choice must not create Claude configuration. Test source-clone and extracted-package paths separately.

### F04 — P1: preserve unrelated hooks inside mixed groups

**Evidence.** [install.mjs:35](hooks/lib/install.mjs#L35) considers a whole hook group owned when any command contains `agent-team-hook.mjs`. Both merge and uninstall filter out the whole group. A group containing one Agent-Team handler and one unrelated handler loses both before replacement/removal.

**Recommendation.** Identify owned handlers at the handler level, preferably with exact managed identities/receipt data. Preserve siblings, matcher properties, and unrelated ordering. Detect customized managed handlers as conflicts. Do not treat a filename substring as ownership of an entire group.

- **Pros:** fulfills the documented additive-install guarantee; protects LeanCTX, Kickoff, and user hooks.
- **Cons:** mixed-group reconciliation is slightly more involved than replacing a group.
- **Acceptance:** install/update/uninstall preserve an unrelated handler inside the same group, as well as unrelated groups. Include customized commands and repeated installation cases.

### F05 — P1: define a shared Kickoff-to-Agent-Team contract

**Evidence.** Kickoff preserves the user-selected tracker and an existing canonical integration branch; its [handoff procedure](https://github.com/thebpandey/project-kickoff/blob/89e6228611b1dd726c6aa0363e7206d0c72fa16d/references/handoff.md) requires stable plan-to-tracker mappings and current approvals. Agent-Team [setup](references/setup.md) selects local tracking if any of Beads, Ponytail, Using-Superpowers, or Impeccable is unavailable. Its instructions repeatedly prescribe `main`, and its [recovery/state rules](references/state.md) allow reconstructing local tracking when Beads is unavailable. Kickoff instead requires a user choice before fallback activation.

The coupling is unnecessary: declining a UI design skill does not make Beads unusable. Root/local tracker reuse is already permitted in Agent-Team's state guide; the gap is inconsistent selection rules and runtime support, not an absolute prohibition on root `TASKS.md`.

**Recommendation.** Validate and adopt the Kickoff receipt once: schema/version, approved document revisions, canonical branch, selected tracker/backend/path, plan-ID mapping, first ready tasks, installed capabilities, and authority boundaries. Do not reinstall, reseed, rename a branch, switch trackers, or reopen approved design questions without a material reason. A changed requirement should reopen only the affected decision.

Beads currently documents an embedded single-writer default, an external-server mode for concurrent writers, and atomic claiming. Select the actual backend deliberately; the presence of `bd` alone does not establish safe multi-writer operation. Initialization can also add agent instructions/hooks, so inspect its effects before composing it with other installers. [Beads README](https://github.com/gastownhall/beads#readme)

- **Pros:** setup becomes reusable; no duplicate planning; safer existing-project support.
- **Cons:** both owned skills need a shared compatibility fixture and version policy.
- **Acceptance:** start from Kickoff's approved Beads project with Impeccable declined and a canonical branch named `develop`. Preserve all three choices and start an eligible implementation task without a new general approval. A tracker outage must not silently migrate it.

### F06 — P1: replace universal skill loading with role-specific loading

**Evidence.** [dependencies.md:67](references/dependencies.md#L67) requires every orchestrator, developer, reviewer, tester, and replacement to load all four enabled skill entry files. Impeccable is loaded even for non-UI work. The dispatch contract repeats this requirement, and the per-run confirmation requires loading and use evidence for each agent/skill pair, including retained agents on later runs.

The four installed entry files alone contain 3,551 words. Eight fresh agent contexts would consume **28,408 words of repeated dependency-entry reads**, before Agent-Team contracts, required companion references, task code, or tool schemas. This is a workload illustration, not a measured eight-agent run or a bill estimate.

Using-Superpowers can introduce further workflow requirements. Agent-Team already says not to create duplicate plans or review rounds, but it also overrides the dependency's teammate entry-skill exemption. The precedence for an already-approved Kickoff plan needs to be explicit, not rediscovered by each worker.

**Recommendation.** Keep a small common execution contract, then load only applicable role/task skills. Use planning skills for unresolved planning, Impeccable for UI work, and exact-source/recovery instructions for the chosen context tool. Supply bounded task context instead of full-history forks. In a retained context, reuse a current receipt; emit a short exception only when something changed or failed. A fresh context must still actually load its applicable instructions—do not pretend a parent's read transfers knowledge.

- **Pros:** a large reducible source of repeated input; fewer conflicting instructions and startup messages.
- **Cons:** routing must reliably identify UI/security/domain needs; removing universal loading changes current policy and needs explicit agreement.
- **Acceptance:** backend-only workers never load UI guidance; approved-plan execution does not reopen brainstorming; replacements load required policies; accuracy is non-inferior in the benchmark in section 7.

### F07 — P1: give the orchestrator small deterministic state operations

**Evidence.** [runs](references/runs.md), [projects](references/projects.md), and [state](references/state.md) contain detailed scheduling rules. The actual [CLI](hooks/agent-team-cli.mjs) provides install, health, audit, and package/artifact commands—not task admission, claim, transition, next-task selection, or run resumption. Hooks validate some operations but do not implement the scheduler. The command scenario table explicitly says it is not proof of host concurrency.

**Risk.** The model must repeatedly interpret and coordinate claims, batch boundaries, approvals, and occupancy. A compact prompt alone will not make those transitions atomic. Unknown writer liveness and orphaned locks can also create recurring manual recovery work.

**Recommendation.** Add the smallest deterministic operations around the existing tracker: validate handoff, resolve command, list eligible tasks, claim, record a transition, inspect next action, and reconcile run state. Use supported tracker atomic operations and a single-writer fallback. Require expected-state/version checks and idempotency keys for consequential transitions. Return compact structured results; let the model handle engineering judgment, not recounting rows.

Do not build a second scheduler database, a broad event platform, or a background service by default. Ownership recovery should use host/session identity and verified stopped writers or explicit handoff; elapsed time alone must not authorize takeover.

- **Pros:** fewer model/tool turns; reproducible behavior; stronger crash/race handling.
- **Cons:** this is the largest core change and needs adapter migration tests.
- **Acceptance:** duplicate starts cannot claim the same task; interrupted integration does not repeat a completed operation; a stale owner cannot overwrite a newer transition; unknown liveness blocks only conflicting writes.

### F08 — P1: introduce a short setup path and targeted settings changes

**Evidence.** [setup](references/setup.md) requires displaying all optional design tools, even irrelevant ones, and separate installation decisions. [settings.md:35](references/settings.md#L35) requires the full wizard on every explicit setup/settings invocation, even with supplied values. There are four run-setting stages, then a model and effort stage for every role, plus final Save. That is at least **4 + 2R + 1 decision points**, excluding dependencies, custom values, Back, and retries. Adapter tables also group some roles that the settings instructions describe as independently configurable.

**Recommendation.** After Kickoff, show a readiness summary and only unresolved prerequisites. Offer “Use recommended settings,” “Change a setting,” and “Advanced” where supported. Keep the full wizard as an explicit guided path. Group a reviewed installation plan for approval while clearly identifying each item/scope; skipping an item remains possible. This is a proposed policy change, not permission to bypass the current per-item approval rule.

Preflight should distinguish installed binaries, host login, Git identity, GitHub authentication, repository access, write permission when needed, hook registration, trust, and actual exercise. Authentication stays in the user's host/browser flow; never request tokens in chat. Install only relevant runtimes: Node 24 for Agent-Team hooks, Python only for selected Python-based skills, and the project's package/browser tools when needed.

- **Pros:** much faster onboarding and settings edits; fewer “yes” turns after an approved setup.
- **Cons:** defaults and grouped approval need clear scope; inexperienced users may prefer the retained guided wizard.
- **Acceptance:** ready-project setup makes no installations or settings changes and asks no repeat questions. Changing continuous mode does not require editing every model. An unknown target or new privilege still asks once for that specific missing authority.

### F09 — P2: separate parked tasks from active compute slots

**Evidence.** [runs](references/runs.md) and [preview](references/preview.md) make blocked, paused, and approval-waiting teams retain slots until integration. A run can have ready independent work but no admission capacity because all logical team slots are held.

**Recommendation.** Track logical task ownership separately from active worker capacity. Allow a safely checkpointed externally blocked task to be parked; preserve its claim, approval gate, and resources, but release stopped compute capacity. Keep explicit user pauses as holds, not opportunities to restart that task. Reserve room for review/integration so workers cannot consume all practical capacity.

- **Pros:** independent work continues without a human unblocking every stalled task.
- **Cons:** more lifecycle states; parked work must have a fair resumption policy and resource limits.
- **Acceptance:** task A awaiting credentials or preview approval does not prevent independent task B from running after A's writers safely stop. A is neither duplicated nor auto-approved.

### F10 — P2: separate implementation cleanup from deployment cleanup

**Evidence.** [release](references/release.md) and [preview](references/preview.md) require cleanup after production verification. Auto-deploy defaults to off. Thus implementation-only runs can retain completed worktrees, dependency folders, and preview processes indefinitely under the current lifecycle. Kickoff's [cleanup contract](https://github.com/thebpandey/project-kickoff/blob/89e6228611b1dd726c6aa0363e7206d0c72fa16d/references/handoff.md#cleanup-after-verified-integration) explicitly separates cleanup from production deployment.

**Recommendation.** Define integration-complete cleanup and release-complete cleanup separately. After verified integration, stopped writers, clean checkout checks, and durable evidence preservation, remove eligible disposable worktrees/resources under a saved retention policy. Keep immutable release evidence/artifacts and previews still awaiting approval. Do not make deployment a prerequisite for reclaiming all development resources.

- **Pros:** bounded disk/process growth during long continuous runs; consistent Kickoff handoff.
- **Cons:** later release debugging depends on good artifacts and recovery pointers.
- **Acceptance:** a no-deploy run cleans eligible completed worktrees without deploying; dirty, active, user-owned, and evidence-owning worktrees remain protected.

### F11 — P2: make the terminal experience state-first

**Evidence.** [wordmark](references/wordmark.md) requires a multi-line banner for help/start/resume/settings/setup/status. [output](references/output.md) frames each message and teammate handoff with 74-character borders. [status](references/status.md) uses character names without role labels, while other output rules require identity/role information. On narrow terminals this wraps, and frequent decoration displaces the next useful action.

**Recommendation.** Default to compact output: run ID, scope, phase, task counts, active/parked capacity, blockers, next automatic action, and whether the user must act. Show role labels beside optional character names. Put branding in help/about or the first setup summary. Use ASCII-safe text and semantic labels; do not rely on color or Unicode glyphs alone. Use native supported pickers, never claim to add keybindings or a live TUI widget through Markdown.

Keep updates event-based: task started, meaningful milestone, review finding, integration, changed blocker, and completion. During a long quiet operation, give a bounded heartbeat appropriate to host expectations, without repeating the whole roster or polling just to generate updates.

- **Pros:** less output, faster scanning, better accessibility and copyability.
- **Cons:** less visual branding; detailed views must remain easy to request.
- **Acceptance:** representative output is readable at 60/80/120 columns, without color, and with screen-reader-friendly text. Progress percentages distinguish completed implementation from production delivery and disclose unknown/stale source data.

### F12 — P2: make health and recovery evidence honest and useful

**Evidence.** [health.mjs](hooks/lib/health.mjs) considers registration true when any matching handler exists. It derives `exercised` from activation logs, but [telemetry.mjs](hooks/lib/telemetry.mjs) only creates reliable activation records for Claude. Codex can therefore keep showing `exercised: false` despite actual hook execution. A Claude skill activation also does not prove every policy event was exercised.

[agent-team-hook.mjs:101](hooks/agent-team-hook.mjs#L101) builds a detailed recovery snapshot in `decision.context.recovery`; [output.mjs](hooks/lib/output.mjs) emits only messages. The host gets a freshness sentence, not the useful snapshot. Recovery selects the most recent checkpoint across the project, which should not be confused with the current session's checkpoint.

**Recommendation.** Separate per-event registration, hook execution receipt, activation capability, user trust, and functional policy checks. Render a small recovery summary with the relevant task, revision, pending operation, and next-action pointer. Label session-specific versus project-wide evidence. Keep unsupported/unknown distinct from false/failed.

- **Pros:** fewer reinstall/retrust loops; recovery becomes actionable.
- **Cons:** minimal execution receipts require schema/retention/privacy rules.
- **Acceptance:** removing one required event reports partial registration; exercised Codex hooks can be recognized without inventing activation telemetry; a new session receives a correctly attributed recovery pointer.

### F13 — P2: budget hook work across the whole event

**Evidence.** Every registered pre-tool event enters [project resolution](hooks/lib/project.mjs) and policy loading; active events load canonical state again for identity. Recovery performs Git and GitHub probes and scans checkpoints. Lint runs one process per config root, sequentially, each with its own timeout. Several individually bounded operations can exceed the outer three/five-second hook declaration budget. The current recovery snapshot probes `gh pr status` even though it only retains the probe's status, not PR details.

**Recommendation.** Measure event latency, add one overall deadline, and avoid work irrelevant to the event. Share the canonical read within an invocation. Debounce/deduplicate advisory lint by changed content where safe. Cache only advisory discovery with explicit invalidation; **never cache away fresh ownership, revision, authorization, or release checks**. Emit actionable advice once per changed condition.

- **Pros:** less cumulative latency and warning noise; fewer hook timeouts.
- **Cons:** invalidation and deadline handling need care; persistent daemon caching is probably unnecessary.
- **Acceptance:** record p50/p95 and worst-case budgets on a monorepo, slow remote, and missing-tool fixture. Critical checks fail safely when evidence is unavailable; advisory exhaustion reports a bounded limitation and does not loop.

### F14 — P2: make model routing predictable and portable

**Evidence.** The [Codex adapter](references/platform-codex.md) requires Astra for the parent and reports a blocker if it cannot confirm the model. This is a deliberate quality policy, but can stop all implementation where the model or metadata is unavailable. Settings reset role routing on a recognized host switch, so switching back can lose previously customized routing.

**Recommendation.** Retain the current quality profile as an option. Add explicitly selected capability profiles, with a suitable authorized fallback chain, bounded escalation, and separate per-host saved overrides. Route by task risk and observed capability, not price guesses. Report unavailable model controls honestly; do not silently downgrade or claim the skill changed its parent model.

Native Codex custom agents can carry model, effort, and scoped configuration. Prefer evaluating those native role definitions over repeatedly spelling out every setting in dispatch prompts, with runtime capability detection and project/user scope clearly shown. Native agents are not a guarantee that every Codex surface exposes identical controls. [Official subagent documentation](https://learn.chatgpt.com/docs/agent-configuration/subagents)

- **Pros:** fewer avoidable routing blockers; stable settings across hosts; potentially lower cost for routine work.
- **Cons:** cheaper profiles may increase repairs; model/version churn requires testing.
- **Acceptance:** switching Codex → Claude → Codex restores the respective profiles. Missing models follow only the user's selected policy. Quality comparisons include repair cost and independent review.

### F15 — P2: measure the whole run, not just compression

**Evidence.** [telemetry](hooks/lib/telemetry.mjs) explicitly reports activation correlations, not effectiveness. Run settings cover team count, continuous mode, deployment, and role routing—not input/output budgets, wasted retries, context growth, or time-to-accepted-task. The existing bounded-failure and evidence-reuse rules are good foundations, but not measured controls.

**Recommendation.** Record per-run accepted tasks, elapsed time, available input/cached-input/output token usage, tool calls, first-pass acceptance, repair rounds, and blocked time. Add soft budgets and stagnation detection with a specific next action. Attribute setup, orchestration, implementation, review, and tool-output costs separately when data permits. Unknown usage remains unknown.

An autonomy preset should continue approved implementation without routine permission questions; retry a transient failure only within a bounded policy; attempt safe in-scope repairs; park external blockers; and stop for genuinely new authority, a user pause, exhausted authorized scope, or an agreed budget boundary. Do not let a budget silently weaken acceptance criteria.

- **Pros:** optimization targets real waste; predictable long runs.
- **Cons:** host usage visibility varies; hard limits can interrupt useful work.
- **Acceptance:** each benchmark reports total cost/time through acceptance, not just initial code output. An unchanged failed approach is not retried indefinitely; unrelated safe work continues.

### F16 — P2: test the delivered experience, not just individual rules

**Evidence.** The current suite usefully exercises hooks, ownership, recovery, installation, and artifacts. [command-scenarios.md](tests/command-scenarios.md) describes interpretation scenarios but explicitly is not a passing host-concurrency test. Current CI runs the unit suite and source-package validation. Passing artifact-content checks does not test installing the extracted archive; a policy helper result does not prove its diagnostics reach the host.

**Recommendation.** Keep the existing suite and add targeted cross-boundary tests listed in section 8. Use scripted host fixtures first, then a small real Codex transcript evaluation. Store sanitized fixtures and structured expected decisions, not brittle exact prose or private reasoning. Do not fix tests by preserving mandatory banners or unnecessary prompts as product requirements.

- **Pros:** catches the seams where this audit found defects; supports safe simplification.
- **Cons:** real model tests cost money/time and are not perfectly deterministic.
- **Acceptance:** deterministic tests cover state/adapter/package behavior; repeated live samples evaluate autonomy, UX, and accuracy separately. Never label simulated concurrency as a verified native run.

## 4. Proposed Codex user experience

The following is a **design proposal**, not a list of newly implemented commands.

### After Project Kickoff

```text
Agent-Team ready · project shop · tracker Beads
Approved plan: PLAN.md @ <revision> · branch: develop
Mode: continuous implementation · deploy: off
Capacity: 2 active workers; independent review queued as needed
Prerequisites: ready · hooks: registered, trust requires your check
Next: restart Codex if required, inspect /hooks, then start.
```

Show only real evidence. If the tracker is unverified or trust is unknown, use that wording. A project can be “configured” without being “ready for unattended execution.”

### Starting an approved plan

An existing supported invocation is:

```text
$agent-team start 2 continuous no-auto-deploy
```

After the proposed improvements, a brief effective-policy summary should be informational, not another approval request when choices are already authorized:

```text
RUN-014 · implementing approved PLAN.md
2 requested teams · continuous · no deployment
Starting TASK-012 and TASK-015; TASK-013 depends on TASK-012.
I will continue eligible work and report only decisions needing you.
```

Actual scheduling must fit available host capacity; two teams does not necessarily mean two simultaneous full developer/reviewer pairs.

### During execution

```text
RUN-014 · implementation 8/12 integrated
Active 2 · parked 1 · ready 1 · deployment off
TASK-012 integrated; TASK-016 started.
TASK-015 parked: missing test-service access. Other work continues.
Action needed: none for the active tasks.
```

Use one focused blocker card when user action is genuinely required:

```text
TASK-015 needs you: authenticate the test service in its browser flow.
Verified: local tests pass; live integration is untested.
Impact: TASK-015 only. TASK-016 continues.
After access is restored: resume TASK-015; no setup repeat needed.
```

### Command and status discoverability

| Surface | Meaning |
| --- | --- |
| `$agent-team help` | Skill actions and a few common examples; more detail on request |
| `$agent-team status` | Project/task evidence and next steps; remain read-only |
| `$agent-team settings` | Proposed summary and targeted-edit entry point; full guided wizard remains optional |
| `/agent` or `/subagents` | Native Codex thread navigation where supported |
| `/status` | Native session/context/rate-limit information, not project task completion |
| `/hooks` | Native hook inspection/trust management; installation does not grant trust |

Native commands above are documented by Codex. Availability should still be checked against the actual client version. Do not conflate skill actions with installed native slash commands. [Official developer commands](https://learn.chatgpt.com/docs/developer-commands?surface=cli)

Keep four distinct endings visible: implementation complete; release pending by policy; externally blocked; fully delivered. With deployment disabled, do not repeatedly ask to deploy after each integrated task. Report release readiness once at the appropriate boundary unless the user explicitly asks.

## 5. Recommended dependency policy

| Component | Proposed policy | Reason / trade-off |
| --- | --- | --- |
| Project Kickoff | Keep as planning/setup authority | Reuse its approved artifacts; reopen only changed decisions |
| Agent-Team | Keep as sole execution orchestrator | Avoid conflicting schedulers and multiple task authorities |
| Beads | Keep when selected and its adapter/backend are verified | Dependency graph and claiming fit parallel work; do not couple it to UI skills |
| Local task file | Keep as explicit lightweight alternative | Low installation cost; serialize writes; no silent migration |
| Ponytail | Retain selectively or encode a short compatible implementation checklist | Useful simplicity pressure; must not remove required behavior/checks |
| Using-Superpowers | Make task/stage-specific rather than mandatory for every worker | Preserve useful engineering practices; avoid repeating approved planning |
| Impeccable | UI roles/tasks only | Good design support; no reason for backend workers to load it |
| LeanCTX | Keep provisionally; benchmark an appropriately small profile | Exact recovery is useful; broader tool surface/setup policy can add overhead |
| Optional UI catalog | On-demand discovery, not default setup inventory | Less decision and context load; users can still browse advanced choices |
| Independent review | Keep for substantive implementation | Reduce ceremony around review, not the independent accuracy check itself |

Relevant existing sources: [Ponytail](https://github.com/DietrichGebert/ponytail), [Superpowers](https://github.com/obra/superpowers), [Impeccable](https://github.com/pbakaus/impeccable), [LeanCTX](https://github.com/yvgude/lean-ctx), [Project Kickoff](https://github.com/thebpandey/project-kickoff).

## 6. GitHub alternatives: shortlist and recommendation

Popularity is a discovery signal, not a quality score. Counts below are GitHub stars retrieved on 2026-09-08, not endorsements or performance measurements. Push dates do not prove release quality. Open-source software also does not make the model/API usage free.

| Candidate | Stars / license / latest push observed | Potential role | Pros | Cons and recommendation |
| --- | --- | --- | --- | --- |
| [Beads](https://github.com/gastownhall/beads) | 26,978 · MIT · Sep 8 | Existing tracker | Ready queue, atomic claim, dependency graph | Backend/version integration matters. **Keep first; fix Agent-Team's adapter before migrating.** |
| [Backlog.md](https://github.com/MrLesk/Backlog.md) | 6,674 · MIT · Sep 3 | Optional tracker replacement | Human-readable tasks, terminal/web views, structured JSON and optional MCP | Requires deliberate mapping and multi-writer validation; not a parallel second ledger. **Best tracker alternative to pilot if Beads setup remains too costly.** |
| [RTK](https://github.com/rtk-ai/rtk) | 79,503 · Apache-2.0 · Sep 8 | Alternative output-compression layer | Small binary, command-aware reduction, raw passthrough and usage estimates | Reduced output can omit useful context. Codex integration is documented as instructions, unlike Claude's automatic rewriting. **Benchmark instead of LeanCTX, never stack blindly.** |
| [LeanCTX](https://github.com/yvgude/lean-ctx) | 3,737 · Apache-2.0 · Sep 8 | Current context layer | Structured discovery and recoverable output already integrated | Tool/configuration surface and policy interaction must justify themselves. **Keep provisionally; compare against native tools and RTK.** |
| [Superpowers](https://github.com/obra/superpowers) | 283,233 · MIT · Sep 8 | Selective engineering skills | Broad adoption; useful planning/testing/review guidance | Universal loading can repeat process already owned by Kickoff/Agent-Team. **Select relevant parts; do not add another orchestration owner.** |
| [GSD Core](https://github.com/open-gsd/gsd-core) | 9,244 · MIT · Sep 8 | Alternative workflow framework / design reference | Supports Codex; fresh-context execution and parallel phase waves | Overlaps both Kickoff and Agent-Team, with its own lifecycle/artifacts. **Borrow patterns; evaluate only as a deliberate replacement, not another dependency.** |
| [Ralph](https://github.com/snarktank/ralph) | 21,741 · MIT · Feb 2 | External bounded iteration pattern | Small fresh-context loop with persistent progress and iteration cap | Reviewed implementation documents Amp/Claude, not a drop-in Codex parallel team scheduler; adds another PRD/task format. **Pattern reference only for this project.** |
| [beads_rust](https://github.com/Dicklesworthstone/beads_rust) / [beads_viewer](https://github.com/Dicklesworthstone/beads_viewer) | 1,087 / 1,678 · API license `NOASSERTION` · Sep 8 | Initially screened as tracker/viewer candidates | Names and popularity make them discoverable candidates | Current license files contain an OpenAI/Anthropic restriction rider. **Exclude from the default recommendation; not unqualified MIT substitutes.** No installation or benchmark performed. |

Important source checks:

- The highly starred [original GSD repository](https://github.com/gsd-build/get-shit-done) had 64,575 stars but is archived; its README points to Open GSD. Do not recommend the archived URL as the active development home.
- RTK explicitly distinguishes reduced shell output from reduced total bills, and describes its Codex integration as `AGENTS.md`/`RTK.md` instructions. Its published savings are not evidence of Agent-Team run savings. [RTK README](https://github.com/rtk-ai/rtk#readme)
- Backlog.md supports CLI-first agent usage and optional MCP. A smaller connector footprint is worth testing, but its tracker migration must preserve task IDs, dependency edges, approvals, and history. [Backlog.md README](https://github.com/MrLesk/Backlog.md#readme)
- The screened Beads alternatives' actual licenses, rather than their names or GitHub badges, control the recommendation: [beads_rust license](https://github.com/Dicklesworthstone/beads_rust/blob/main/LICENSE), [beads_viewer license](https://github.com/Dicklesworthstone/beads_viewer/blob/main/LICENSE). Treat applicability as a license-review question, not an assumption of unrestricted reuse.

No replacement is recommended for immediate installation. The highest-confidence first experiment is **native tools versus LeanCTX versus RTK**, one configuration at a time, after reducing Agent-Team's own repeated instruction overhead. The tracker pilot is second; a wholesale orchestration replacement is last.

## 7. Efficiency plan with accuracy guardrails

### Optimize in this order

1. Remove unnecessary questions and duplicate planning. This reduces wall time without reducing engineering checks.
2. Reduce mandatory context per role; keep the orchestrator focused on decisions, ownership, and evidence.
3. Use bounded task packets and compact results. Carry task/requirement IDs, allowed paths, dependencies, checks, relevant lessons, and recovery pointers. Do not copy full logs or full project documents to every worker.
4. Reuse unchanged evidence using source/dependency/configuration/environment identity. Rerun affected checks after relevant changes. Keep the integrated-version check.
5. Right-size concurrency. Parallelize genuinely independent work; pool/reuse appropriate agents; do not spawn ceremonial roles or nested supervisors by default. Reviewers need not remain idle throughout implementation.
6. Measure compressor and model-profile choices after the workflow baseline is smaller.

Reusing an agent can save startup work but preserve irrelevant context. Use retained contexts for coherent task sequences; use fresh bounded contexts when roles or domains change or accumulated context becomes harmful. No universal “always reuse” or “always restart” rule is efficient.

### Benchmark design

Run the same approved tasks and initial snapshots across a baseline and one changed configuration at a time. Use several repetitions; report median and spread, not the best run.

Include a small backend fix, multi-file feature, UI feature, independent parallel tasks, shared-file conflict, failing test repair, external-auth blocker, paused/resumed run, and an integration-only/no-deploy run.

| Metric | Why it matters |
| --- | --- |
| Total available input/cached-input/output tokens through acceptance | Captures repeated context and repair cost; report unsupported usage fields as unknown |
| Time to first useful action; time to verified integration | Exposes setup/orchestration delays and actual delivery speed |
| First-pass independent acceptance; escaped seeded defects | Protects code accuracy |
| Repair rounds and repeated unchanged failures | Measures wasted execution |
| User questions after approved setup | Measures autonomy; distinguish necessary authority questions from routine confirmations |
| Tool calls, hook latency, and output bytes | Identifies tool/schema/probe overhead; bytes are not tokens |
| Active versus parked/idle agent time | Shows scheduling efficiency |
| Residual worktrees/processes/disk usage | Shows resource cost across continuous runs |
| Unauthorized actions, duplicate claims, stale approvals | Must remain zero in the test scenarios |

Promotion rule: choose a candidate only when verified accuracy and safety are maintained and total cost/time improves meaningfully for representative tasks. Set numerical targets after establishing the baseline; no unsupported “50% cheaper” promise.

A persistent process outside the interactive session is a separate product choice. The current [run contract](references/runs.md) correctly says it is not a daemon. Do not promise work continues after Codex exits. If that becomes a requirement, evaluate a bounded external runner with checkpoint/reconciliation and explicit permissions separately—not an unconditional stop-hook loop.

## 8. Verification gaps to close before claiming unattended readiness

| Scenario | Required outcome | Findings covered |
| --- | --- | --- |
| Extracted Codex/Claude release package installation | Intended host installs, health works, reinstall is idempotent, uninstall preserves unrelated state | F03–F04 |
| Beads and two local tracker locations | Same gates, one authority, correct IDs, no shadow ledger | F01, F05 |
| Failing linter and multiple config roots | Failure reaches host stdout; useful bounded diagnostics; passed results preserved | F02, F13 |
| Mixed hook group + Kickoff/LeanCTX configuration | Unrelated handlers and custom definitions survive | F04 |
| Kickoff approved plan, non-main branch, UI skill declined | No replanning, forced migration, or branch rename | F05–F06 |
| Second start during active run | No second owner or duplicate task admission | F07 |
| Reviewer needed when worker capacity is full | Work reaches review without spawning beyond limits or starving review | F09 |
| All original teams externally blocked, other tasks ready | Safely parked tasks preserve ownership; independent work progresses | F09 |
| Crash after provider success before tracker update | Reconcile evidence; no duplicate release | F07, F12 |
| Explicit pause during an in-flight operation | Stop admission first; report unknown/still-running accurately | F07 |
| Continuous no-deploy run | Integrate authorized work, no repeated deploy prompts, bounded resource retention | F10–F11 |
| Partial registration and exercised Codex hook | Missing event is visible; unsupported activation is not failed execution | F12 |
| Host switch and unavailable model | Preserve per-host settings; honor explicitly selected fallback policy | F14 |
| Narrow/no-color terminal and screen-reader transcript | Actionable status, no misleading progress or wrapping-dependent meaning | F11 |
| Reduced-context/compressed-output run with seeded defects | Same acceptance criteria; missing diagnostics trigger exact/raw recovery | F06, F15 |

Add these at the relevant seam rather than building a separate general-purpose testing framework. Package fixtures and adapter stdout tests should come before expensive live model evaluations.

## 9. Review outcome and decision precedence

The one-at-a-time review is complete for the 17 core decisions and 20 dependency candidates. The first 16 core decisions were saved at the user's earlier checkpoint; this user-authorized consolidation adds F17, the final dependency dispositions, and the dashboard design. Later specific decisions below control over original proposals in sections 1–8.

| Decision | Recommendation | Settled implementation contract |
| --- | --- | --- |
| 1. Correctness fixes | Address F01–F04 first | Tracker adapter scope, packaging contract, lint output contract, hook ownership identity |
| 2. Execution readiness | Reuse Kickoff when used; also support existing and standalone projects | F05: one readiness contract, branch/tracker preservation, selective invalidation |
| 3. Dependency policy | Role-specific loading and concise exception reporting | F06 and section 11: selected role/task profiles replace universal four-skill ceremony |
| 4. Setup and terminal output | Short default path, full wizard available, compact status | F08/F11 and section 11: automatic first-run preparation, genuine trust/auth boundaries, readable model/effort menus |
| 5. Autonomy model | One owner, deterministic transitions, safe parking | Claim/recovery semantics, worker/reviewer capacity, retry and authority boundaries |
| 6. Cleanup and deployment | Separate integration completion from release completion | Retention policy and evidence needed before worktree/process removal |
| 7. Efficiency measurements | Establish baseline before selecting replacements | Representative tasks, usage visibility, accuracy and cost/time acceptance thresholds |
| 8. Selected dependencies | Prepare the accepted profiles; retain Beads and Markdown | Section 11: narrowed LeanCTX versus native tools; no RTK or replacement-tracker pilot |
| 9. Optional web dashboard | Snapshot-first status webpage; lightweight live mode optional | D20 and section 12: teams, progress, all tasks, graphs, freshness, licensing, read-only operation |
| 10. Context continuity | Durable records, bounded contexts, verified handoff | F17: keep native compaction as fallback; measure complete recovery cost and accuracy |

The decisions authorize the described product direction, not implementation. A future implementation request must establish its scope; it does not need to reopen settled choices. New material decisions remain separate. The automatic-remediation contract applies during authorized setup/development, not to this read-only review or documentation update.

## 10. Agreed decisions — implementation on hold

Consolidation authorized by the user on 2026-09-08: save all 17 finalized core decisions and all 20 dependency decisions, including the approved dashboard and context-continuity designs. F01–F16 retain the earlier checkpoint's substance; F17 records the additional approved P1 finding. The final dependency choices are in section 11.

No implementation has started. Do not implement these decisions, install dependencies, or change runtime settings/hooks without a subsequent implementation authorization. Recording this report neither starts an autonomous development run nor grants permission to publish, migrate, deploy, or commit changes.

### F01 — Finalized: honor the selected canonical tracker

Retain Beads and Markdown support. Add a compatibility layer that resolves the actual canonical tracker selected by the user or during Project Kickoff and uses it consistently for ownership, completion, release, and recovery checks. Support the selected Markdown location, including root `TASKS.md` and `.agent-team/TASKS.md`. Report unavailable or stale evidence honestly; never silently migrate the tracker or create a second authoritative task ledger.

Acceptance: all supported tracker choices pass equivalent checks without duplicating tasks or weakening verification.

### F02 — Finalized: actionable lint feedback with automatic remediation

Correctly aggregate passed, failed, skipped, and timed-out checks. Report affected files and bounded, actionable diagnostics; preserve completed results when another check cannot run. Deduplicate repeated warnings and keep longer diagnostics recoverable separately.

Automatic checks remain advisory while code is being edited. If lint is a required acceptance check, unresolved failures prevent completion through final verification. A failed check blocks task completion, not continued repair work.

During authorized development, the orchestrator automatically assigns findings to the responsible developer, coordinates repairs, reruns affected checks, and continues after verification. Do not ask the user whether to fix ordinary lint errors, test failures, or review findings, or whether to continue authorized work. If repair stalls, diagnose, change approach, or escalate to a suitable agent within bounded retry limits; continue independent safe work.

This automatic-remediation requirement applies across all 17 core decisions and the dependency decisions. Ask the user only for genuinely missing authority, credentials/access, or material product decisions. Classify pre-existing unrelated problems rather than silently expanding scope. This agreement does not itself authorize implementing the audit findings.

Acceptance: both host adapters expose failed lint with actionable, bounded diagnostics. During authorized development, ordinary findings trigger assignment, repair, and rechecking without a new user instruction; required checks still gate completion.

### F03 — Finalized: one complete package, selected installation targets

Use one complete distribution package. Default the installation target to the host in use, Codex or Claude Code; install for both only when explicitly requested. Clearly distinguish supported project-only and user-wide scope. Preserve unrelated settings and customized files, reject unsupported or ineffective options, and avoid loading unused host instructions into agent context.

The agent performs the approved installation automatically. Required host trust, authentication, and elevated permissions remain separately user-controlled. Accept the small download-size increase in exchange for one consistent package contract.

Acceptance: test the extracted-package installation, verification, update, reinstall, and uninstall journey. Only the selected host and scope may be configured.

### F04 — Finalized: preserve unrelated hooks and customized handlers

Manage ownership at the individual handler level, using exact managed identities and installation receipts rather than treating an entire mixed group as owned. Preserve unrelated handlers, group properties, and ordering during installation, update, and uninstall.

Automatically update clearly owned, unchanged handlers. Preserve ambiguous or customized handlers and explain the specific conflict. Continue unaffected work; ask for a decision only when proceeding would require overwriting customization or otherwise exceeding existing authority.

Acceptance: repeated install/update/uninstall preserves unrelated siblings and groups. Customized or ambiguously owned commands are not silently replaced or deleted.

### F05 — Finalized: Kickoff-aware, independently functional execution readiness

Agent-Team must work independently of Project Kickoff and any separate planning/tracking skill. Use one shared execution-readiness contract, with three supported entry paths:

1. **Project Kickoff was used:** validate and reuse its approved handoff, plan, canonical branch, tracker, capabilities, and authority boundaries. Do not repeat settled planning or setup decisions.
2. **An existing plan or tracker exists without Kickoff:** adopt the usable project state, validate readiness, and resolve only the gaps needed for safe execution.
3. **A standalone request has neither:** inspect the project and create a proportionate task breakdown and setup sufficient for the requested work. Do not impose Kickoff documents, stage approvals, or a separate planning workflow.

Readiness covers scope, acceptance conditions, actionable tasks/dependencies, the integration branch, verification commands, necessary capabilities, and authorization boundaries. Reuse valid choices and ask only about genuine unresolved essentials. A material change reopens only the affected decision.

Beads is supported but not required for every project; a selected Markdown tracker remains a valid standalone path. Missing UI or engineering skills must not silently switch the tracker. Never silently migrate a tracker because its backend is temporarily unavailable.

Acceptance: all three entry paths reach an eligible task without duplicate planning, mandatory Kickoff installation, or a second tracker. A valid Kickoff project using `develop`, Beads, and no UI skill preserves those choices.

### F06 — Finalized: prepared skill catalog with selective instruction loading

Use a small common contract for safety, scope, ownership, verification, and recovery. Load additional instructions according to the agent's role and current task: UI guidance for UI work, planning guidance for unresolved planning, and relevant engineering/context guidance where needed. Preserve substantive independent review and required tests.

During approved setup, install and functionally verify the complete skill/tool catalog required by every enabled role, including access from the relevant host sessions and worktrees. This means preparing the enabled catalog, not installing every skill in the ecosystem or approving all candidates in section 11. Agents must be able to select prepared capabilities without rediscovering or reinstalling them during each task.

Keep installation readiness separate from context loading. Reuse applicable, still-valid instructions in retained contexts; fresh or replacement agents must actually read their applicable instructions. Supply bounded task packets rather than full-history forks. Use compact internal receipts and report changed, missing, or failed requirements instead of repeating a full skill-use ceremony.

Acceptance: enabled roles can access their prepared dependencies; backend-only workers do not load UI instructions; approved-plan execution does not reopen brainstorming. Fresh agents receive the required contract, and reduced loading does not weaken acceptance checks.

### F07 — Finalized: small deterministic helpers for task bookkeeping

Add tested helpers to validate readiness, identify eligible tasks, atomically claim work, enforce ownership/state transitions, identify the next permitted action, and reconcile interruptions. Use the existing canonical tracker as the authority, with supported atomic operations or a safe single-writer path. Do not introduce a second scheduler database or a background daemon by default.

Make consequential operations idempotent and check expected state/version before applying transitions. The orchestrator retains engineering judgment, delegation, repair strategy, and escalation; helpers handle repeatable bookkeeping rather than becoming another orchestrator.

Unknown writer liveness must not authorize ownership takeover. Recover through verified stopped writers or explicit handoff; prevent conflicting writes while independent work continues.

Acceptance: duplicate starts cannot claim the same task, stale owners cannot overwrite newer state, and interrupted integration can resume without repeating completed side effects.

### F08 — Finalized: short setup, targeted settings, readable model selection

Provide a short first-run setup with a recommended configuration and an explicit, inspected, grouped installation plan. The later dependency decision refines this policy: automatically install missing mandatory dependencies and their prerequisites on first run, with concise progress and functional verification, without a separate approval question for each plugin. Prepare default-selected components according to section 11; optional additions remain opt-in. Advanced customization must disclose any missing mandatory capability rather than calling an incomplete setup fully ready. For configured projects, present only missing, changed, or broken requirements instead of reopening settled questions.

Support changing one setting directly; keep a full guided wizard available on request. Automatically execute and verify approved installations. Required authentication, hook trust, administrator permissions, purchases, and other new authority remain separate. Preserve declined options and unrelated/customized configuration.

Make role routing easy to inspect and change without memorizing model identifiers or effort keywords:

- Show every configured role, its model, effort, availability, and whether each value is a default or override.
- Let the user select a role, then choose from readable model and effort menus with current/recommended markers and short explanations.
- Use native selection controls where the host supports them, with a numbered text fallback otherwise.
- Show a change summary before saving and state when the change takes effect.
- Distinguish requested/configured routing from routing the current runtime can actually enforce.

Separate saved defaults from active-run settings. Do not silently alter an active run when saving future defaults.

Acceptance: a user can inspect all roles and change one role's model/effort without knowing exact keywords or repeating full setup. Unsupported routing is reported honestly; declined dependencies and active-run choices remain intact.

### F09 — Finalized: park blocked tasks without retaining active compute

Separate logical task ownership from active agent capacity. After bounded automatic remediation, a task awaiting an external prerequisite may be safely checkpointed and parked: stop its writers, preserve its claim, worktree, evidence, and applicable approval gates, and free active capacity for ready independent work.

Resume when the prerequisite is restored within existing authority. An explicit user pause stays paused; a project-wide pause stops new admissions. Parking does not imply preview approval or permission to bypass a version-bound gate.

Reserve practical review capacity and apply fairness/resource bounds so ready tasks and reviews do not starve. Count actual host limits rather than assuming a logical team always maps to a fixed number of available agents.

Acceptance: one externally blocked task does not occupy compute indefinitely or stall unrelated ready tasks. Resume preserves ownership and evidence; concurrent writers and implicit unpausing are prevented.

### F10 — Finalized: development cleanup independent of deployment

Allow automatic cleanup of eligible development resources after work is safely committed, integrated, and verified, without requiring production deployment. Before cleanup, verify stopped writers, inspect tracked/untracked/ignored content, preserve durable evidence outside the disposable checkout, and honor pending preview or explicit retention needs.

Keep release-resource cleanup separate and retain the evidence needed for rollback. Never deploy merely to make cleanup eligible.

Preserve dirty, unknown, user-owned, or active work. Report ambiguous cleanup cases without blocking unrelated development or weakening the preservation checks.

Acceptance: an authorized continuous run with deployment disabled can integrate work and reclaim eligible resources. Worktrees containing unpreserved material or active writers are not removed.

### F11 — Finalized: compact, state-first terminal communication

Lead with actionable state: run/task progress, active/parked/ready work, deployment mode, next action, and whether the user needs to do anything. Distinguish integrated work, release-pending work, blocked work, and delivered results.

Send concise event-driven updates and useful heartbeats for long operations without busy polling. Use role labels by default; character names are optional. Make task, agent, model, and blocker details available on request rather than printing everything repeatedly.

Limit branding to first setup/help instead of repeating wordmarks and fixed-width decorative borders throughout execution. Support narrow terminals, no-color output, and accessible transcripts.

Status is read-only. Use real host capabilities and selection controls; do not imply a native dashboard, slash command, daemon, or background execution capability that is not implemented and supported.

Acceptance: compact output makes progress and required user action clear without relying on color or wide layouts. A status request does not mutate project or runtime state.

### F12 — Finalized: evidence-based health and attributed recovery

Report installation, per-event hook registration, trust evidence, host support, and actual per-event exercise separately. Unknown trust stays unknown. Unsupported events, unobserved events, and failed events are distinct states; one successful event must not imply that all hooks work.

Provide compact recovery context attributed to the correct task/session: checkpoint, revision, pending operation, uncertainty, and next safe action. Do not present an unrelated project-wide checkpoint as the current session's proven state.

During authorized setup or development, automatically repair in-scope configuration problems where ownership and authority are clear. Preserve customization. Health/status inspection itself remains read-only and must not silently install, rewrite, or repair anything.

Acceptance: partial hook registration is visible, an unsupported activation event is not labeled a failure, and recovery output exposes the relevant pending operation and uncertainty without inventing evidence.

### F13 — Finalized: one overall hook-event budget and selective checks

Give each hook event an overall deadline. Read shared project/state inputs once where practical, run only relevant checks, remove unused network probes, deduplicate unchanged advisory output, and keep host-visible responses compact.

Measure current latency before choosing numerical targets. Cache only suitable advisory/discovery information with explicit invalidation. Required ownership, authorization, revision, and release evidence must meet their freshness requirements; performance optimization cannot turn stale or unavailable evidence into a pass.

An advisory timeout does not stop safe work. When required evidence is unavailable, hold only the affected consequential action, automatically diagnose within scope, and continue independent work.

Acceptance: event processing obeys an overall budget and avoids repeated unnecessary probes. Timeouts preserve truthful state and do not bypass required checks or stall unrelated work.

### F14 — Finalized: persistent per-host routing and approved escalation

Keep the current quality-focused default. Offer optional balanced and economical profiles with clear tradeoffs. Preserve separate configurations for each host and restore them when switching hosts rather than discarding previous role choices.

Let setup approve suitable fallback and escalation chains. During execution, automatically use those approved paths when persistent difficulty warrants it; an expected first failing test is not itself a reason to escalate. Do not silently downgrade quality or select an unapproved route.

Show requested routing separately from actual runtime capability. Parent-model changes may require a host action or user action; do not claim a change was enforced when it was not.

Retain independent review and required checks in every profile. Compare total cost through accepted, accurate completion, including repairs, rather than selecting by nominal per-call price alone.

Acceptance: host switching preserves prior settings; unavailable routes follow the approved policy; unsupported enforcement is visible. Lower-cost profiles must be evaluated against the same acceptance and safety requirements.

### F15 — Finalized: end-to-end metrics and explicit budget behavior

Measure available input, cached-input, and output tokens across the orchestrator, workers, reviewers, and repairs. Track time to first useful action and verified integration, first-pass acceptance, retries, user questions, idle/parked capacity, hook latency, and resource retention. Mark unavailable data as unknown and estimates as estimates.

Use soft budgets by default: adapt context, scheduling, and approved routing to improve efficiency without routine permission-to-continue prompts. Hard budgets apply only when explicitly set; checkpoint safely at their boundary without weakening tests or silently increasing the budget.

Compare representative identical tasks from equivalent starting states with repeated runs. Assess accepted-code accuracy and safety alongside total tokens and wall time; output compression alone does not establish overall savings.

Acceptance: metrics include repair cost, distinguish observed from estimated values, and support fair comparisons. Budget handling preserves checks and authorization boundaries.

### F16 — Finalized: verify complete journeys and multi-agent boundaries

Keep the existing tests and add focused fixtures for the missing user journeys and contracts:

- Extracted-package install, verification, update/reinstall, and uninstall for selected host/scope.
- Kickoff handoff, adoption of an existing non-Kickoff project, and standalone setup.
- Equivalent behavior for Beads and supported Markdown tracker locations.
- Failed lint reaching actual host output and triggering automatic remediation.
- Duplicate claims, real capacity accounting, parking/resume, and interrupted-operation reconciliation.
- Safe no-deployment cleanup and preservation of user/unknown work.
- Compact terminal output, health/recovery distinctions, and readable role/model/effort changes.

Start with cheap deterministic checks, then perform bounded real Codex evaluations at the relevant boundaries. Label simulated behavior separately from native-host evidence. Do not equate passing fixtures with proof of actual host concurrency or user-interface support.

Automatically repair in-scope regressions during authorized implementation. Do not weaken tests to get a pass or ask whether ordinary regressions should be fixed.

Acceptance: the added scenarios demonstrate the agreed contracts, with explicit evidence limitations. Required review and verification remain intact while efficiency changes are measured.

### F17 — Finalized: durable context continuity and bounded context growth

Priority: **P1**. Make continuation depend on durable project evidence, not preservation of the complete conversation. Reduce unnecessary compaction, especially in worker sessions, while retaining native compaction as a safety net. Do not promise zero compactions in an indefinitely running host conversation.

**Confirmed source gap.** The [recovery contract](references/recovery.md#checkpoint-during-normal-work) already calls for regularly updated, normally sub-600-word CONTEXT.md files. The [checkpoint schema](hooks/lib/checkpoint.mjs#L6) permits nextAction and decisionNotes, but [checkpointFacts](hooks/agent-team-hook.mjs#L30) does not populate them: it collects Git, task, tracker, and pending-operation facts. Preserving an existing field is not evidence that current semantic decisions were captured. A deterministic hook cannot reconstruct an unwritten decision.

#### Durable records and selective recovery

| Information | Authority |
| --- | --- |
| Scope, acceptance, ownership, dependencies, findings, task status | The existing selected Beads or TASKS.md tracker |
| Approved design decisions, constraints, rejected alternatives and reasons | Existing project/design documents |
| Current attempt, evidence pointers, pending operations, exact next action | Existing agent checkpoint / CONTEXT.md |
| Code and detailed verification evidence | Actual files and revision-linked artifacts |

Record important facts as they arise, at meaningful progress and handoff boundaries, not only when context is almost full. Keep checkpoints small indexes into authoritative records; do not copy the project into every checkpoint. Preserve explicit prohibitions, approval boundaries, unresolved questions, relevant failed approaches, and the distinction between approved decisions, proposals, and assumptions. Recover from original records rather than repeated summaries of previous summaries. Retain required evidence outside disposable worktrees; no secrets or private reasoning in checkpoints.

Do not add a memory database, vector store, MCP server, duplicate task ledger, or transcript archive for this purpose. A small stable recovery instruction in host project instructions points to the records; do not load a growing backlog/history through that instruction file.

#### Bounded workers and a compact orchestrator

Give each worker its task/acceptance criteria, relevant constraints and decisions, owned files/worktree, dependencies, applicable role instructions, evidence pointers, and required handoff format. Avoid full-history forks as the routine dispatch mechanism.

Reuse a worker for coherent implementation/repair while its context is useful. Use a fresh worker for unrelated work or when insufficient headroom remains for the next substantial step. Rotate at safe, meaningful boundaries, not after every command or trivial task. Exact budgets depend on exposed host/model capacity and measured task needs; no universal percentage or task-size limit was approved.

The Project Orchestrator receives compact receipts: outcome, revision, evidence location, unresolved findings, and next required action. It retrieves detail when needed, rather than absorbing all searches, logs, screenshots, and worker conversations. Dashboard/status rendering reads records instead of repeatedly soliciting agent-generated reports.

#### Verified automatic handoff

1. Save current facts and the small checkpoint.
2. Reach a safe stopping point and resolve or identify outstanding operations.
3. Verify the previous writer stopped or explicitly transferred ownership.
4. Start the replacement with the bounded recovery packet.
5. Reconcile task identity, scope, worktree, current revision, evidence validity, and the next permitted action.
6. Continue the same authorized work automatically, without reopening setup or asking for general permission to continue.

Missing records trigger bounded recovery from tracker/files/evidence first. Ask only when material authority or a decision genuinely cannot be recovered. Never infer stopped ownership from a stale timestamp alone; independent tasks continue while conflicting writes remain held. Explicit user pauses and required gates survive every handoff.

A skill cannot assume it can replace its own parent conversation or run after the host closes. Fully automatic orchestrator rotation requires exposed host controls or a separately approved controller. This decision does **not** add a new orchestration framework or external runner.

#### Compaction-safe restoration and cost measurement

Keep native auto-compaction enabled as fallback. Use supported lifecycle events to check records before compaction and restore a small recovery index afterward; verify registration, trust, and behavior on the installed host. Do not launch another model to summarize the whole conversation from a hook. In Claude Code, SessionStart matching compact is the documented context-injection route; PostCompact is not an interchangeable route for adding model context. [Codex hooks](https://learn.chatgpt.com/docs/hooks), [Claude Code hooks](https://code.claude.com/docs/en/hooks#sessionstart)

A context limit and a prompt-cache miss are different problems. Caching does not shrink context occupancy. Claude Code's compaction request can reuse a warm cache; after expiry, it can process the history as uncached input. A shorter post-compaction context may save total cost despite reduced cache reuse. Fresh-worker initialization and rediscovery also cost tokens. Preserve useful stable instructions, and measure total input/output, cache usage where exposed, compaction count/duration, checkpoint/startup overhead, repeated investigation, and recovery errors. Do not infer savings from fewer compactions alone. [Claude Code caching](https://code.claude.com/docs/en/prompt-caching#compacting-the-conversation), [OpenAI caching](https://developers.openai.com/api/docs/guides/prompt-caching)

Codex documents model_auto_compact_token_limit; current Claude Code documentation describes /autocompact. Treat these as version-dependent advanced controls, not approved numerical defaults. Lowering thresholds can increase compaction frequency. Larger windows are useful for genuinely interconnected work, not a replacement for checkpoints; disabling auto-compaction or inflating a configured window is not the default strategy. Focused manual compaction or fresh context at a natural task boundary remains an available workaround. [Codex configuration](https://learn.chatgpt.com/docs/config-file/config-reference), [Claude Code context controls](https://code.claude.com/docs/en/context-window)

**Pros:** less dependence on lossy conversation summaries; smaller worker/parent contexts; fewer repeated investigations and routine resume questions.

**Cons:** useful decisions still require timely recording; replacement startup and recovery reads have a cost; hooks and context controls vary by host. No guarantee of perfectly restoring the old conversation or eliminating compaction.

**Acceptance:** a fresh agent given only the recovery packet and referenced records recovers the correct task, constraints, approved/rejected decisions, ownership, evidence, pending operation, and next action without old chat history. Cover real compaction restoration, stale/missing/conflicting checkpoints, an active old writer, and source changes invalidating tests. Preserve gates and independent progress. Compare accuracy and total cost/time, including repair and replacement overhead, before claiming savings.

## 11. Final dependency decisions

All 20 candidates below have been discussed and finalized. “Default,” “mandatory,” and “automatically install” describe the approved future product behavior, not installations performed during this review.

### Cross-cutting installation and loading policy

- **Mandatory baseline:** Serena and Microsoft Playwright CLI are prerequisites. Detect and automatically install missing mandatory components and required runtimes in dependency order on first run, then functionally verify them. Playwright remains a universal prerequisite by explicit user choice, not web-project-only.
- **Default-prepared catalog:** prepare the selected ast-grep CLI, narrowed LeanCTX, selective Superpowers, local Ponytail, Impeccable skill/detector, and React Best Practices profiles. Default preparation is not universal instruction loading or a requirement to invoke every tool on every task.
- **Optional:** Context7 and the dashboard/beads_viewer integration are opt-in. The Caveman subagent-tax diagnostic is optional maintainer tooling, not onboarding or ordinary runtime work.
- Reuse compatible working installations. Do not upgrade everything merely because a newer version exists. Pin reviewed, compatible package/components for the eventual release and verify access from relevant host sessions/worktrees. A skill package is not proof its companion executable or service is ready.
- Give a concise installation/progress summary without per-plugin confirmation for the mandatory baseline or already selected defaults. Respect genuine authentication, administrative permissions, hook trust, access policy, and customization boundaries. Do not request secrets in chat or claim installation grants hook trust.
- Prepare all capabilities needed by enabled roles, but load only the applicable instructions into each agent. Fresh agents read their own applicable instructions; retained agents reuse valid context. Keep tool definitions/profiles appropriately scoped where supported. Installation, discovery metadata, loaded instructions, and actual use are distinct.
- Automatically repair ordinary in-scope installation/runtime failures during authorized setup/development, using bounded diagnosis and permitted fallbacks. Do not call an unavailable prerequisite ready, bypass host restrictions, migrate a tracker, or stall unrelated feasible work because an auxiliary capability failed.
- Third-party guidance is subordinate to the approved scope, ownership, review, verification, and autonomy contract. Maintain reviewed compatibility profiles, not multiple competing workflow controllers. Package examples do not authorize installing every library they mention.
- Discarding a candidate means no Agent-Team installation, integration, migration, or benchmark for it under this decision set. It does not authorize removing a user's unrelated existing installation.

### Existing tracker retained: Beads, with Markdown alternative

Keep [main Beads](https://github.com/gastownhall/beads) and its selected backend; the user explicitly rejected replacing it with beads_rust. The reviewed main package uses an [MIT license](https://github.com/gastownhall/beads/blob/main/LICENSE); the restriction rider belongs to the separately maintained candidates D19/D20. Retain the user's existing Beads choice, or the selected canonical TASKS.md when Beads is not used.

Beads documents an embedded single-writer mode and an external-server mode for concurrent writers. Do not assume every project requires an external Dolt server, or that merely finding bd makes concurrent writes safe. Use version-appropriate supported reads/claims and the approved ownership path. JSONL exports used by viewers are derived interchange artifacts, not the task authority or a substitute for a backup. [Beads README](https://github.com/gastownhall/beads#readme)

Project Kickoff remains an optional upstream planning/setup skill. Reuse its approved handoff when present; otherwise use F05's existing-project or standalone path. [Project Kickoff](https://github.com/thebpandey/project-kickoff)

### Decision index

| ID | Candidate / repository | Final disposition |
| --- | --- | --- |
| D01 | [Context7](https://github.com/upstash/context7) | Optional; not a prerequisite |
| D02 | [Serena](https://github.com/oraios/serena) | Mandatory; first-run automatic installation |
| D03 | [Microsoft Playwright CLI](https://github.com/microsoft/playwright-cli) | Mandatory universal prerequisite; default browser tool |
| D04 | [Vercel Agent Browser](https://github.com/vercel-labs/agent-browser) | Skipped; no integration |
| D05 | [ast-grep CLI](https://github.com/ast-grep/ast-grep) / [companion skills](https://github.com/ast-grep/agent-skill) | CLI default-installed; companion bundle deferred, not approved |
| D06 | [RTK](https://github.com/rtk-ai/rtk) | Discarded entirely |
| D07 | [LeanCTX](https://github.com/yvgude/lean-ctx) | Default-installed, narrowed context/output profile |
| D08 | [Superpowers](https://github.com/obra/superpowers) | Default-prepared selective engineering toolkit |
| D09 | [Ponytail](https://github.com/DietrichGebert/ponytail) | Default-installed selective local skills; no MCP or blanket hooks |
| D10 | [Impeccable](https://github.com/pbakaus/impeccable) | Default-installed UI skill and local detector; batched checks |
| D11 | [Vercel React Best Practices](https://github.com/vercel-labs/agent-skills/tree/main/skills/react-best-practices) | Default-installed individual skill; React/Next.js context only |
| D12 | [Matt Pocock TDD](https://github.com/mattpocock/skills/blob/main/skills/engineering/tdd/SKILL.md) | Skipped; existing testing profile retained |
| D13 | [Matt diagnosing-bugs](https://github.com/mattpocock/skills/blob/main/skills/engineering/diagnosing-bugs/SKILL.md) | Package skipped; selected techniques added to existing debugging contract |
| D14 | [Matt code-review](https://github.com/mattpocock/skills/blob/main/skills/engineering/code-review/SKILL.md) | Package skipped; selected techniques added to existing review contract |
| D15 | [Caveman](https://github.com/JuliusBrussee/caveman) | No default integration; optional subagent-tax maintainer diagnostic only |
| D16 | [Backlog.md](https://github.com/MrLesk/Backlog.md) | Package skipped; backlog lives in existing Beads/TASKS.md |
| D17 | [GSD Core](https://github.com/open-gsd/gsd-core) | Discarded entirely |
| D18 | [Ralph](https://github.com/snarktank/ralph) | Discarded entirely |
| D19 | [beads_rust](https://github.com/Dicklesworthstone/beads_rust) | Discarded entirely; keep main Beads |
| D20 | [beads_viewer](https://github.com/Dicklesworthstone/beads_viewer) | Selected for optional dashboard integration, subject to licensing clearance |

### D01 — Finalized: Context7 optional, not a prerequisite

Offer a short description so users can make an informed choice:

> Context7 — optional documentation helper. Gives agents relevant library documentation and examples. Useful when working with unfamiliar frameworks or changing APIs. Uses an online service; Agent-Team works without it.

If selected, install and functionally verify the integration, reusing a working installation. Invoke it automatically for relevant documentation questions, not before every task. Match the actual dependency version, retrieve focused excerpts, and reuse still-valid results across workers.

Do not send secrets or private project content in queries. If unavailable, use official documentation or local package sources and continue feasible work; do not require the user to fix Context7 simply to continue.

**Pros:** focused documentation can improve accuracy on unfamiliar/changing APIs. **Cons:** online-service availability, rate limits/authentication, and query privacy; net run savings are unmeasured. Source: [Context7](https://github.com/upstash/context7).

### D02 — Finalized: Serena mandatory

Automatically install and functionally verify Serena, uv where required, and applicable free language-server support. Do not select paid JetBrains integration as the default. Installation readiness does not require invoking Serena for every task.

Expose a small relevant tool profile. Start with navigation-oriented integration; enable editing only after confirming owned-file/worktree boundaries and compatibility with host editing rules. One active project per instance requires appropriate isolation across worktrees/sessions. Preserve host-required editing mechanisms.

Automatically diagnose runtime failures and use safe native navigation fallbacks when appropriate; distinguish unsupported language support from a working capability. No repeated repair/continue questions for ordinary failures.

**Pros:** semantic navigation complements exact reads and structural search. **Cons:** language-server/runtime preparation, project-instance isolation, and editing compatibility require tests. Source: [Serena](https://github.com/oraios/serena).

### D03 — Finalized: Playwright CLI default browser and mandatory prerequisite

Use Microsoft's Playwright CLI as the default browser tool and a prerequisite for all projects, including non-web projects, by explicit user decision. Automatically prepare missing supported runtime/browser dependencies and verify operation during first-run setup.

Load browser instructions only for relevant roles/tasks. Honor a host-mandated browser interface where it takes precedence. Isolate browser sessions and clean up only owned resources. Keep repeatable project tests and actual visual inspection; browser automation is not proof that a screenshot was inspected. Required desktop/mobile viewports remain required, not replaced by a cheaper mobile-only check.

**Pros:** one default browser path reduces tool-choice overhead and supports functional/visual verification. **Cons:** universal installation adds download/runtime cost even for backend-only projects; host controls and browser dependencies vary. Source: [Microsoft Playwright CLI](https://github.com/microsoft/playwright-cli).

### D04 — Finalized: skip Vercel Agent Browser

Do not install or integrate Agent Browser as another browser path. Keep D03's default and respect host-required capabilities. Preserve unrelated user installations.

**Trade-off:** avoids duplicate browser tools and instructions; forgoes this alternative's distinct features. Source: [Agent Browser](https://github.com/vercel-labs/agent-browser).

### D05 — Finalized: ast-grep CLI default-installed; companion bundle deferred

Prepare the ast-grep CLI by default as a structural-search complement to Serena's semantic navigation. Supply concise original Agent-Team guidance.

The separately reviewed companion skill bundle is **not approved for bundling**: its licensing was not explicit in the reviewed tree. Deferral is not an instruction to install it later automatically.

Before broad rewrites, verify positive and negative pattern fixtures, constrain changes to owned files, inspect the diff, and run affected tests. Syntax matches do not prove symbol scope, types, control flow, or data flow.

**Pros:** precise structural searches and scoped transformations. **Cons:** patterns can match the wrong semantics and require validation; the companion skills' licensing remains unresolved. Sources: [CLI](https://github.com/ast-grep/ast-grep), [companion skills](https://github.com/ast-grep/agent-skill).

### D06 — Finalized: discard RTK entirely

No RTK installation, integration, evaluation, or benchmark. The earlier native-versus-LeanCTX-versus-RTK proposal is superseded. D07's comparison is narrowed LeanCTX versus native tools only.

**Trade-off:** avoids another compression layer and integration surface; gives up the proposed comparison, without claiming RTK is inherently ineffective. Source: [RTK](https://github.com/rtk-ai/rtk).

### D07 — Finalized: narrowed LeanCTX default-installed

Use compact reads/shell output with exact-source and diagnostic recovery. Do not repeat discovery when Serena or ast-grep already supplied the needed information.

Disable LeanCTX's separate coordination/shared-knowledge mechanisms, request proxy, and model/effort steering in the Agent-Team profile. Load applicable context guidance, not the entire catalog for every agent. Retained output archives have bounded retention and are not permanent project truth.

Recover original code or diagnostics whenever compression is insufficient; summaries are not verification evidence. Diagnose failures automatically and use permitted native tools when needed, never as a way around a security denial. Compare the narrowed profile against native tools on total accepted accuracy, time, and tokens, including recovery overhead.

**Pros:** smaller tool results with recoverability. **Cons:** recovery calls, tooling/policy interaction, and retention add costs; no net savings claim before measurement. Source: [LeanCTX](https://github.com/yvgude/lean-ctx).

### D08 — Finalized: selective Superpowers toolkit, not a second lifecycle

Default-prepare reviewed compatible engineering skills, not the entire unchanged plugin lifecycle. Pin a tested compatibility profile.

| Component | Approved use |
| --- | --- |
| Test-driven development and systematic debugging | Applicable implementation/repair work, with project-required coverage |
| Requesting/receiving code review | Existing independent review and automatic repair loop; no duplicate dispatch |
| Verification before completion | Actual required checks tied to the relevant revision and environment |
| Brainstorming and writing plans | Genuine unresolved planning gaps only; reuse approved Kickoff/existing plans |
| Subagent-driven development, executing plans, dispatching parallel agents | Useful techniques only; Agent-Team owns scheduling, capacity, and task state |
| Worktrees and finishing a development branch | Agent-Team retains ownership of integration, cleanup, and release lifecycle |
| Using-Superpowers entry behavior | Replace blanket loading with applicable task/role selection |
| Writing skills | Actual skill-authoring assignments only |

Reconcile human-stop and approval ceremonies with existing approved authority: ordinary in-scope findings automatically enter repair. Preserve genuine scope/access/approval gates. Do not delete valid or user-owned code merely because it predates a test, and do not waive required acceptance findings at a retry cap.

The reviewed subagent-driven workflow has its own serial implementation/ledger and bounded parking behavior; do not adopt that controller or treat parked residual defects as complete. Agent-Team remains the orchestrator for parallel eligible work.

**Pros:** established engineering guidance without recreating every practice. **Cons:** upstream instructions can conflict with the accepted autonomy/preservation contract; selective loading and compatibility need tests. Source: [Superpowers](https://github.com/obra/superpowers).

### D09 — Finalized: selective local Ponytail skills, not MCP

Prepare local Ponytail core and review skills by default for relevant implementers/reviewers. Help is on demand; whole-repository audit is for an explicitly authorized maintenance/audit task; debt harvesting is on demand and writes meaningful findings into the canonical tracker, not a new ledger. Exclude gain from the normal interface because the reviewed scoreboard did not match the newer benchmark material.

Do not enable Ponytail's MCP or blanket session/subagent/prompt hooks by default. Preserve user-owned existing integration. The reviewed MCP serves the same instruction text; transporting it through MCP is not intrinsically more token-efficient than selectively loading the local skill.

Required test coverage and explicit acceptance override minimal-check or no-framework defaults. Do not reduce scope, readability, security, accessibility, or maintainability merely to shorten code. No forced one-liners, unrelated cleanup, second reviewer workflow, or endless subjective rewrites. Valid in-scope findings are remediated through the existing loop.

**Pros:** focused simplicity checks with a small local instruction path. **Cons:** overlaps Superpowers; brevity rules need guardrails. Measure incremental benefit through accepted results, not a maintainer headline or line-count reduction alone. Source: [Ponytail](https://github.com/DietrichGebert/ponytail).

### D10 — Finalized: Impeccable skill and local detector, UI-scoped

Default-prepare the selected local skill and self-contained detector; verify the chosen installation route and its actual runtime prerequisites. Load instructions and initialize project design context for UI work only, not every backend agent. Prefer a supported prebuilt engine where appropriate; do not require users to source-build Rust/Bun tooling merely to obtain it.

Do not enable per-edit detector hooks by default. Use the supported no-hooks installation approach and batch targeted detection at meaningful checkpoints. Use the default Playwright browser path for applicable visual/functional checks; live interactive detector mode is optional.

Reuse approved product/design facts from Kickoff or standalone setup. Do not introduce another requirements authority, repeat the design interview, require image concept cards/asset agents for ordinary code work, or add a second team/controller. Image exploration is opt-in. Agent-Team owns agents, capacity, budget, review, and verification.

| Guidance / command family | Approved boundary |
| --- | --- |
| init, document, shape | Fill genuine design-context gaps; reuse settled facts |
| audit, critique, polish | Existing review/repair loop; a user-requested read-only audit stays read-only |
| harden, clarify, adapt, optimize, onboard | Task- or finding-driven |
| extract, distill, typeset, layout | Scoped improvements within the approved brief |
| bolder, quieter, colorize, animate, delight, overdrive | Only when supported by the approved design brief |
| craft | Do not build the integration around the deprecated alias |

Aesthetic warnings are not automatically defects. The approved brief, including deep-green/neon choices, prevails over generic stylistic preferences; use narrow evidence-backed exceptions without suppressing accessibility requirements. Operational detector failures and detected findings are distinct, and a clean heuristic scan does not prove the UI works.

**Pros:** design guidance plus local checks in the existing loop. **Cons:** overlapping reviews, asset-generation overhead, and stylistic false positives if the profile is not constrained. Detector operation without model/API calls does not establish total agent-run savings. Source: [Impeccable](https://github.com/pbakaus/impeccable).

### D11 — Finalized: React Best Practices default-prepared, React/Next.js-only

Install the individual skill and its rules, not the full Vercel collection. Load relevant entry guidance and individual rules for React/Next.js assignments, not every compiled rule into every agent.

Use the existing review/automatic-fix loop. No Vercel account, MCP, or deployment is required by this choice. Check framework version, runtime, and compiler behavior before applying a rule; Next.js-specific APIs do not apply to plain React automatically.

Cache examples need authorization, isolation, freshness, and invalidation checks. Mentions of SWR, better-all, or lru-cache do not authorize adding those packages. Prefer proportionate measured performance improvements over rewriting healthy code for speculative micro-optimizations. A rule's priority is not automatically a task-blocking severity.

**Pros:** targeted framework guidance. **Cons:** version-sensitive advice and potential over-optimization; application performance gains are not token-savings evidence. Source: [React Best Practices](https://github.com/vercel-labs/agent-skills/tree/main/skills/react-best-practices).

### D12 — Finalized: skip Matt Pocock TDD

Do not add this TDD package or its conditional codebase-design dependency. Retain the approved Superpowers testing profile and project acceptance requirements.

**Trade-off:** avoids duplicate testing instructions and design workflow; does not discard behavior-focused testing or other independently approved testing safeguards. Source: [Matt Pocock TDD](https://github.com/mattpocock/skills/blob/main/skills/engineering/tdd/SKILL.md).

### D13 — Finalized: debugging techniques without another skill package

Do not install diagnosing-bugs. Add these approved techniques to the existing debugging contract:

- Reuse a concrete reproduction command and its evidence.
- After a minimized fix, rerun the original failing scenario, not just a narrower substitute.
- Tag temporary instrumentation and remove only agent-owned instrumentation.
- Compare competing hypotheses when the cause is ambiguous; do not impose a hypothesis quota for an obvious failure.
- Keep required test coverage; absence of a convenient test seam is not a waiver.
- Permit useful evidence-led, read-only investigation before a perfect reproduction exists.
- Retain bounded attempts, strategy changes/escalation, and independent progress.

**Pros:** better reproducibility and confirmation of repairs. **Cons:** rigid reproduction/interview rituals or unbounded persistence would waste work, so they are not adopted. Source: [diagnosing-bugs](https://github.com/mattpocock/skills/blob/main/skills/engineering/diagnosing-bugs/SKILL.md).

### D14 — Finalized: review techniques within the existing review contract

Do not install the code-review package or its separate tracker/configuration workflow. Preserve distinct requirements and code-quality verdicts while using Agent-Team's risk- and capacity-controlled review.

Review an explicit stable revision or a captured work-in-progress state, including relevant staged, unstaged, and untracked changes. A base-to-HEAD commit comparison alone does not cover uncommitted work. Deduplicate and prioritize findings without losing which verdict they affect; route valid findings through automatic repair and rechecking.

Reuse existing approved acceptance criteria. A missing standalone specification file is not automatically missing requirements or a reason for another setup interview. Do not mandate two additional reviewers or another review scheduler for every task.

**Pros:** clearer acceptance versus quality results; complete review scope. **Cons:** redundant reviewers and tracker setup would increase cost, so those parts are excluded. Source: [code-review](https://github.com/mattpocock/skills/blob/main/skills/engineering/code-review/SKILL.md).

### D15 — Finalized: no default Caveman integration; optional diagnostic only

Keep concise, plain Agent-Team communication. Do not install a new Caveman response/review style or its runtime, proxy, MCP, browser, memory, or pixel-related companion stack. These add overlap with the approved context profile and are not approved runtime dependencies.

The MIT subagent-tax tool is allowed as an **optional maintainer diagnostic** during a deliberate optimization investigation, not a prerequisite, setup action, per-task check, or tool every agent runs. Its request-size measurements/estimates are not invoices or demonstrated savings. A minimal isolated Codex capture is not the real Agent-Team loadout; a Claude capture using real configuration can start hooks/MCP processes and leave transcripts, so it is not side-effect-free.

Keep captures private. Provider token-count uploads require explicit opt-in. Do not present unreviewed aggregate headlines or unavailable benchmark artifacts as measured Agent-Team benefits. Keep component licensing distinct: permission for the diagnostic does not approve the separately licensed runtime stack.

**Pros:** optional visibility into startup/context overhead. **Cons:** incomplete capture fidelity, estimates, sensitive data, and capture side effects; no validated net benefit for our workflow. Source: [Caveman and its diagnostic documentation](https://github.com/JuliusBrussee/caveman).

### D16 — Finalized: no Backlog.md; backlog stays in Beads or TASKS.md

Discard the dedicated Backlog.md package, MCP integration, tracker migration, and separate backlog.md file. The approved replacement is a disposition/view of records in the existing canonical tracker, not another dependency.

During authorized development, deduplicate and classify meaningful discoveries:

| Discovery | Canonical disposition and action |
| --- | --- |
| Required fix within approved scope | Link to affected work; assign, repair, and verify without asking |
| Approved work for later | Queue with priority/dependencies; a continuous run admits it when eligible and within authorized run scope |
| Genuine external blocker | Record attempts, impact, and resume condition; park only affected work and continue independent tasks |
| Out-of-scope improvement | Capture as proposed/deferred without interrupting; recording is not implementation authority |

Required acceptance failures cannot be relabeled optional backlog to claim completion. Keep the minimal useful record: ID/description, priority, origin, acceptance, disposition/blocker, and next action. Use supported Beads issue/state/label/relationship fields or the existing canonical TASKS.md table/details, with the orchestrator as the sole agent writer in Markdown mode.

A discovered-from or related link is not a blocking edge unless the dependency is real. Priority and backlog labels do not grant authority. Event-based updates and prerequisite changes drive reconsideration; do not poll or reload the entire backlog for every action. Show ready, active, externally blocked, and deferred/proposed work distinctly.

A temporary Beads outage triggers bounded recovery, not silent creation of a second Markdown ledger.

**Pros:** one authority, no additional setup, and autonomous handling of required findings. **Cons:** disposition/eligibility mappings must be consistent across tracker adapters. Sources: [candidate not adopted](https://github.com/MrLesk/Backlog.md), [retained Beads](https://github.com/gastownhall/beads).

### D17 — Finalized: discard GSD Core entirely

No GSD dependency, embedded engine, integration, evaluation, or additional design-technique adoption from this candidate review. Independently approved Agent-Team practices remain; do not relabel them as a new GSD integration.

The earlier proposal to borrow patterns or evaluate replacement is superseded. The [original GSD repository](https://github.com/gsd-build/get-shit-done) was archived at review and redirected to [GSD Core](https://github.com/open-gsd/gsd-core); neither is adopted.

**Trade-off:** avoids another lifecycle, state/artifact system, and coordination owner; forgoes its packaged workflow features.

### D18 — Finalized: discard Ralph entirely

No snarktank/ralph package, external loop/wrapper, PRD converter, extra progress/memory files, evaluation, or new technique adoption. Small tasks, durable checkpoints, and bounded recovery remain independently approved Agent-Team principles.

The reviewed Amp/Claude-oriented sequential loop and output-marker completion are not the required Codex parallel ownership and independent-verification contract. The earlier pattern-reference recommendation is superseded.

**Trade-off:** avoids a duplicate task format/controller and incompatible permission/completion assumptions; does not adopt its simple standalone iteration loop. Source: [Ralph](https://github.com/snarktank/ralph).

### D19 — Finalized: discard beads_rust; keep main Beads

No beads_rust dependency, br CLI, migration, companion skill, MCP, evaluation, or benchmark. The user explicitly confirmed: keep Beads, discard beads_rust. D20 does not reopen this decision; beads_viewer can consume exports from main Beads.

**Trade-off:** avoids migration and another backend/compatibility surface; no claim that the Rust alternative was benchmarked against our actual Beads configuration. Sources: [discarded beads_rust](https://github.com/Dicklesworthstone/beads_rust), [retained main Beads](https://github.com/gastownhall/beads).

### D20 — Finalized: optional beads_viewer dashboard integration

Select beads_viewer as the optional Beads-backed graph/export provider for an Agent-Team web dashboard. This supersedes the earlier exclusion recommendation. It is **not** a mandatory dependency for normal Agent-Team development, a replacement tracker, or another scheduler/claim authority.

When the user enables this integration, prepare and verify a compatible prebuilt bv binary if missing, subject to the licensing/release condition below. No br CLI, source-build toolchain, cloud account, or full web application build stack is required by the chosen design. Do not auto-install the unrelated ast-grep companion bundle or other deferred/discarded tools as transitive features.

The approved feature is a webpage version of Agent-Team status: project progress, current teams and their states, a dedicated all-tasks section, and dependency graphs. Use serverless snapshots by default and an optional lightweight live local mode. Section 12 is the detailed accepted design.

**Licensing condition:** the reviewed [license](https://github.com/Dicklesworthstone/beads_viewer/blob/main/LICENSE) is MIT with an OpenAI/Anthropic restriction rider, not unrestricted standard MIT. It names restricted parties and associated entities; do not infer either a blanket ban on every Codex user or clearance for our integration/distribution. Resolve applicability/necessary permission before shipping the integration, including exported/embedded assets. Optionality does not resolve the license. This product decision is not legal clearance or permission to publish.

**Pros:** existing interactive graph exports support a lightweight visual view. **Cons:** Agent-Team still needs its own team/status layer, fresh Beads exports, secure artifact handling, compatibility tests, and licensing clearance. Graph analytics are not verified task claims or proven token savings. Source: [beads_viewer](https://github.com/Dicklesworthstone/beads_viewer).

## 12. Approved optional web dashboard design

This is the D20 feature design, not an installed command or an existing webpage. Dashboard use is optional. Agent-Team remains the owner of the status presentation, and Beads/TASKS.md remains the task authority.

### User-facing sections

| Section | Required contents |
| --- | --- |
| Project overview | Overall task completion, completed/remaining work, blockers, current run, and separate integration/deployment state |
| Teams | Team assignment, agents/roles, model/effort, recorded execution state, last update |
| All tasks | Dedicated searchable/filterable list including completed, unassigned, blocked, and deferred work; priority, owner, dependencies, expandable details |
| Dependency graph | Interactive Beads task relationships through beads_viewer, with exploration/filtering |
| Freshness | Collection time, source, and explicit stale/unavailable information |

Reuse the terminal status counting and mapping contract. Count unique actionable tasks without double-counting parent/child or cross-team dependencies. Display excluded/deferred/cancelled work separately from the completion denominator, and distinguish project totals from current-run scope. Task counts do not measure effort/time. Unknown or incomplete data cannot support an exact completion percentage; zero included tasks is N/A, not 100%. A task marked in progress does not prove its agent process is alive. Full task completion does not imply production delivery. [Existing status contract](references/status.md)

### Snapshot-first architecture

Use a small Agent-Team-owned page in plain HTML, CSS, and JavaScript, generated through existing Node-based helper infrastructure. No React, Vite, Docker, application build server, cloud account, or separate dashboard database.

A small local output folder can hold the dashboard HTML and a beads_viewer-generated graph HTML displayed in the graph section. This is a practical serverless output, not a requirement to maintain a fork of beads_viewer's full dashboard or squeeze every artifact into one physical file.

The reviewed bv repository documents a standalone HTML graph export with embedded assets and a fuller static dashboard bundle, along with watch-export and a built-in local preview path. Agent-Team needs its own team/session/status layer regardless. Pin and verify a release that actually supports the selected behavior rather than assuming moving-main documentation proves an installed release does. [Graph export](https://github.com/Dicklesworthstone/beads_viewer#-interactive-graph-visualization---export-graph), [static dashboard export](https://github.com/Dicklesworthstone/beads_viewer#-static-site-export-shareable-dashboards), [preview implementation](https://github.com/Dicklesworthstone/beads_viewer/blob/main/pkg/export/preview.go)

| Mode | Refresh behavior | Running process |
| --- | --- | --- |
| Local HTML snapshot — default | Agent-Team regenerates after meaningful state changes; opening the file or Reload snapshot displays the latest saved version | None needed merely to view it |
| Live local dashboard — optional | Opening the page or Refresh status asks a small local helper to read current records; automatic updates may run while open | One lightweight local helper; no build/dev server |

An ordinary local HTML file cannot execute bd or bv. Its button reloads a saved snapshot; it cannot itself collect new tracker data. If work stops or outside changes have not been exported, that snapshot remains old and must be labeled accordingly. Genuine refresh-on-click without an active agent conversation requires the optional helper.

The local view needs no account/login. Remote access, authentication for sharing, cloud publishing, and hosting are separate product/authority choices, not included in this approval.

### Automatic update contract

1. The orchestrator records a meaningful task, ownership, blocker, verification, or completion transition in the authoritative records.
2. A deterministic helper refreshes dashboard data and refreshes the graph when necessary.
3. For Beads, obtain a current export from the actual selected tracker before bv renders it. Watching an unchanged old JSONL export does not observe later database changes.
4. Publish a complete consistent replacement snapshot; retain the previous usable snapshot if generation fails.
5. Show the new version on reload or deliver the update through the optional live mode.

The reviewed main-Beads export route is bd export -o .beads/issues.jsonl; eventual integration must use the selected backend/version safely and avoid overwriting user-owned artifacts. JSONL remains a derived view, never task authority. [Beads export requirements](https://github.com/Dicklesworthstone/beads_viewer#generating-the-jsonl-file-br-and-bd)

Dashboard generation/refresh does not call a model. Combine bursts of events, skip unchanged outputs, and avoid unnecessary graph analysis. Do not load full graph/triage reports into all agents or recompute them after every source edit. F15 measures actual overhead rather than assuming the graph tool saves tokens.

### Safety, standalone support, and acceptance

- Reuse one status-reading contract across terminal and web. Generated files are disposable views, not another tracker.
- The dashboard is read-only with respect to project execution: no task claims, resume, tracker repair, tests, integration, or deployment from a refresh. Opting into dashboard export permits its derived files; a plain status request still does not install/start a server or refresh checkpoints.
- Dashboard/export failure does not stop development. Preserve the last snapshot, show staleness, and use bounded in-scope recovery.
- Keep exported information local by default, sanitize untrusted task content, and omit unnecessary logs/history and sensitive data. Do not assume raw exports are already appropriate for public sharing.
- The optional live helper binds to loopback, serves only dashboard resources, and has request protections. It is not an arbitrary shell endpoint or a project execution daemon.
- Preserve TASKS.md support for teams, progress, and the all-tasks section. Do not introduce Beads just to enable those views; the bv-specific graph integration is for Beads-backed projects.
- Verify snapshot opening/reload, live refresh without agent turns, all-task completeness, progress parity with terminal status, graph/source freshness, safe failure behavior, malicious task text, and process/resource boundaries.
- Resolve D20 licensing before shipping the integration. Neither the review nor a successful export test establishes legal clearance.

**Pros:** visual project visibility without an application build stack; serverless snapshots remain usable after the agent exits.

**Cons:** serverless refresh can only show the latest generated artifact; truly current click-to-refresh needs the optional process. Exported task data needs privacy care, and bv cannot supply Agent-Team session state by itself.

## 13. Consolidation scope and remaining implementation evidence

### What is settled

F01–F17 and D01–D20 are finalized product decisions. F02's automatic remediation applies across the accepted workflow. D16 keeps backlog in the canonical tracker; F17 keeps context recovery in existing records; D20/section 12 adds a read-only optional dashboard. None creates a second task authority or orchestration owner.

Historical recommendations in sections 1–8 do not authorize discarded candidates, blanket instruction loading, mandatory Kickoff, repeated ordinary repair approvals, or a second backlog file. The final ledger controls. A future reader should use the decision index and load only the relevant detailed sections, not paste this entire audit into every worker's context.

### What still requires evidence, not another general approval ceremony

During a separately authorized implementation, establish compatible versions/platforms, installation receipts and prerequisite ordering, supported host controls, tracker semantics, and measured budgets. Verify the selected skill compatibility profiles against project acceptance rather than assuming installation equals correctness.

The baseline 135-test result in section 2 predates these unimplemented decisions. No new behavioral, native-host, concurrency, dashboard, compaction-recovery, or cost benchmark result is claimed by this consolidation. Preserve existing tests and add the approved boundary/journey tests in F16, F17, and section 12. Compare narrowed LeanCTX with native tools; do not revive RTK or discarded tracker/framework pilots.

D20's licensing clearance is a release condition. The ast-grep companion skill bundle remains deferred and unapproved; that does not prevent preparing the approved CLI. The optional subagent-tax diagnostic is not a runtime prerequisite.

### Authority and handoff

The user authorized updating this Markdown report only. No code, runtime settings, hooks, installations, migrations, tracker records, getting-started guides, commits, pushes, publication, or deployments are authorized by this documentation action.

The next stage is a user-authorized implementation scope based on these decisions. Until then, keep implementation on hold. If additional material questions arise, discuss them individually rather than silently changing settled choices or treating recorded backlog as execution authority.
