# Native lanes and session continuity

## Approved scope

The user approved all five lane design sections, five additions from SirRuggie/claude-code-orchestration-kit at 4416994bf730c8e9833984383daa58375c1ca6fd, uppercase canonical Markdown filenames, and autonomous implementation. The subsequent explicit instruction removes stale session registration as a blocking requirement and authorizes transferring this project's coordination record to the current session. This overrides the original session-owner restriction; it does not waive task ownership, exclusive paths, independent review, exact-revision evidence, clean-main, tracker single-writer, integration, release, or retention rules.

## Lane protocol

One lane is one retained worker, one registered team, one branch lane/<lane-id>, one worktree, and an ordered queue of admitted tracker tasks. The tracker remains the only task authority. Queue membership does not claim future tasks. Only the orchestrator spawns/continues agents. New runs default to lanes; unrelated one-off tasks may use fresh workers. Legacy runs remain readable.

Native lane-create, lane-next, lane-rotate, lane-close routes use closed versioned request envelopes and exact non-chained command parsing. Retain revision, fingerprint, idempotency, project scope, and tracker checks. CLI text cannot forge native execution identity. Current-session continuity must not depend on an inaccessible historical chat UUID.

state.lanes records lane id, team id, role/model/effort, ordered queue, current task, worker identity/generation, worktree/branch, brief path/hash, rotation count, handover pointer, fact sheets, and immutable assignment/result history. Lane state is durable, versioned, validated, and fingerprinted with canonical state. Open lanes reserve capacity between tasks; unknown liveness never frees a slot. Reserve independent reviewer capacity.

Lane-next claims only the next eligible task through the existing tracker route and durable interrupted-write reconciliation. The previous task must have accepted completion and integration evidence before advancement. One operation identity binds claim and queue advancement; ambiguous dispatch never triggers duplicate work. Rotation preserves current claims/worktree, invalidates outgoing worker identity only after stopped-writer or explicit transfer evidence, and supports replacement binding without advancing/reclaiming the current task.

## Files, context, and decisions

Canonical Pro Markdown basenames are uppercase; directories and JSON names remain stable. Root entry/release files stay at root; rules live in references; templates in assets/templates/lanes; role definitions in assets/claude-agents with native role IDs unchanged. Update all links, code consumers, manifests, and tests. Preserve recorded legacy lowercase project paths, user project documents, Lite, and archives.

Each lane reads .agent-team/lanes/<lane-id>/BRIEF.md, containing role/model/effort, queue, shared rules, exact writable paths, required skill paths, optional gold example, evidence destination, and handoff format. Validate required nonempty sections and a default 6000-word cap. Fresh workers read complete applicable project instructions and skills; retained workers invalidate context on relevant hash/version/path/requirement changes.

Save immutable next-task packets under packets/<TASK-ID>-<ATTEMPT>.md, targeting under 400 tokens. Include acceptance, delta, new decisions, revision pointers, never the repeated brief. Assignment records bind packet/brief hashes, revision, task attempt, and worker generation. Scope changes are explicit amendments, not edited history or expanded tracker authority.

Rotate after two completed tasks or pressure by default. HANDOVER-001.md and successors target under 500 words with voice, gotchas, open threads, decisions, exact revision. Mid-task pressure requires a safe checkpoint. Resume live workers through actual host controls; replace confirmed stopped workers; retain unknown occupancy. Compact recovery indexes support one session per planning/build/release phase and manual compaction focus guidance.

Project decision documents retain stable IDs, rationale, rejected alternatives, and reopening conditions. Packets carry relevant pointers/deltas. Decision reversals return to the orchestrator; alternating repairs trigger focused diagnosis under the existing two-attempt rule. No new ledger competes with the tracker.

## Verification and reporting

Developer handoff, delegated mechanical verifier, independent reviewer, repair, completion gate, serial integration remain mandatory. Mechanical failures return before reviewer dispatch. Reviewer reads exact assignment, source/diff, verifier evidence, rubric, and challenges concrete failure scenarios and whether tests exercise the change. Developer narrative is not proof. Preserve required checks; additional reruns target stale or insufficient evidence. A retained reviewer cannot review their own authored task.

Before dispatch, Graphify affected/path ownership evidence must identify overlapping writable paths. Resolve by one writer or approved per-task split and record the resolution. Missing evidence remains unresolved. Shared fact sheets have one owner, checkedOn/sourceUrl, content hash, consumer citations, and default seven-day expiry; changed requirements/sources may invalidate earlier.

Every dispatch receives a result or unresolved disposition bound to task/attempt/generation/assignment/revision. Prepared packets are not confirmed dispatch; missing reports are neither completion nor stopped-writer proof. Full evidence stays outside disposable worktrees. Worker updates are separate capped receipts: task, revision, evidence pointer, named checks, one next action. Default 2000 characters; supported hooks validate resulting edit/append size. Document unobservable-write/native-message limits.

## Settings and helpers

Independent per-host execution defaults: lanes.enabled=true; lanes.rotation.tasks=2; lanes.rotation.onPressure=true; lanes.factSheetStaleDays=7; lanes.workerUpdateMaxChars=2000; lanes.briefMaxWords=6000; supervision.heartbeatSeconds=600, minimum 60. Snapshot effective run settings. Safety limits: limits.subprocessMaxBufferBytes=2097152; limits.maxPlanTasks=1000. Ensure downstream readers and initialization settings do not defeat supported limits.

Wizard shows current/effective/source values without resetting unrelated settings. Setup offers unused visible MCP/plugin disable candidates based on selected profile, never disables without explicit yes. Store prior/installed values and paths in receipt; revert only owned changes with conflict detection. Verify actual host project-level support; do not substitute global disabling. Test fixtures, not live user settings.

Use notification-driven supervision with configured heartbeat; no polling/timers/daemons/hosted monitors/post-turn activity. Fetch known IDs singly, bound parent outputs, delegate large reads, preserve original evidence. Recommend Opus execution sessions on Claude and explicitly selected available models on Codex without invented cross-provider equivalence. Parent models cannot be changed by the skill. Planning may justify a higher tier.

Ship evidence writer, fresh gate-request rebinder, and dotenv child runner under assets/helpers, installed by helpers CLI into scripts/agent-team with hashes and conflict preservation. Each self-checks. Evidence writer accepts actual check/review results, never infers pass from names. Rebinder prints safely quoted exact native commands, no authority. Env runner never prints values or evaluates shell expressions. Health reports managed helper drift.

## Validation and release

Close lanes only when queue is empty and every task integrated. Lane-aware cleanup preserves every existing task-level safety check, including stopped writers, exact revisions, dirty/untracked/ignored files, required previews, retained evidence, and pending operations.

Behavioral tests first: defaults/validation/host isolation, context offers/approval/revert, helper self-checks/drift, lane schema/fingerprints/briefs, four routes/native identity/stale/chained/replay/interrupted claims, rotation/rebinding/unknown liveness, capacity/cleanup/update caps, verifier order, immutable packets, missing reports, uppercase paths/links and focused protocol duplication.

Two-lane five-task fixture: three lane-next calls on one lane, one rotation, close both, revision-bound events. Native simulation stays in tests with no production bypass. Run existing hook/CLI tests, package checks, helpers, fixture, fresh sealed-install health. Exact passed/failed/skipped counts; simulated host checks distinguished from live verification. SKILL.md cap 108 lines. Bump all package version fields and CHANGELOG to 7.3.0 after implementation/tests. No remote publication or automatic destructive installation replacement. Savings are estimates unless measured by usage.mjs.
