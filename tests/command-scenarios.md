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
