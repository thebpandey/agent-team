# Agent-Team v9 Core Skill Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a portable, Beads-only Agent-Team skill that directly orchestrates native Codex or Claude agents during an active session.

**Architecture:** One host-neutral skill routes to short host and worker references. Beads holds tasks; Git holds code; `DECISIONS.md`, `MISTAKES.md`, `BLOCKERS.md`, and `.agent-team/SESSION.md` hold only their named human/operational knowledge. No controller reserves or acknowledges workers.

**Tech Stack:** Markdown skill instructions, Git, Beads CLI (test against 1.2.2), native Codex/Claude agent tools. No runtime Go, Node, Python, hooks, or LeanCTX.

**Spec:** `docs/superpowers/specs/2026-09-24-agent-team-skill-first-design.md`

## Global Constraints

- Use a new `v9/` source tree; do not reuse v8 controller or v7 hook code.
- Beads is the only live task tracker; `TASKS.md` and Project Kickoff are one-time inputs.
- Team lists contain at most four ordered tasks, claimed one at a time; one-off work uses at most two parallel teams.
- Default to Ponytail's smallest-working-change discipline. Do not probe, install, or invoke LeanCTX.
- Serena, Graphify, Playwright/browser, Impeccable, UI-styling, and UI-UX-Pro-Max are optional, task-triggered aids, never admission gates.
- A real independent reviewer inspects code and evidence. A distinct name alone is not CLEAN evidence.
- Limit on-demand dev servers to two per project; never share one across projects. A blocker or uncertain host action stops only its affected task lane.
- Preserve all unrelated changes, including existing `.beads/interactions.jsonl` and `.gitignore` edits in the main checkout.
- Each implementation task uses a clean isolated worktree, genuine RED→GREEN for executable behavior, a focused independent review, and one commit.

## File map and interfaces

| File | Responsibility |
| --- | --- |
| `v9/agent-team/SKILL.md` | Host-neutral entrypoint: setup, bounded Beads selection, orchestration loop, pause/resume, and references. |
| `v9/agent-team/references/HOSTS.md` | Exact native Codex/Claude dispatch, observation, follow-up, and stop differences. |
| `v9/agent-team/references/WORKER_RULES.md` | Developer/reviewer authority, Ponytail rules, scoped reports, FIX/CLEAN schema. |
| `v9/agent-team/references/STATE.md` | Beads commands, one-time imports, ledgers, generated `.agent-team/SETTINGS.md`, and session checkpoint format. |
| `v9/agent-team/assets/dashboard.html` | Small offline local status snapshot template; no runtime fetch or server. |
| `v9/tests/CANARIES.md` | Reproducible live-host scenarios and observable pass/fail criteria; not text-grep tests. |

The installed skill is the exact `v9/agent-team/` tree. `SKILL.md` names only the relevant reference for a requested action. A worker receives task facts, its worktree/scope, applicable project rules, and relevant `DECISIONS.md`/`MISTAKES.md` entry IDs; never full tracker or ledger dumps.

---

### Task 1: Portable entrypoint and Beads-only first run

**Files:** Create `v9/agent-team/SKILL.md`, `v9/agent-team/references/STATE.md`, `v9/tests/CANARIES.md`.

**Interfaces:** Consumes `bd --readonly status --json`, `bd ready --limit 20 --json`, `bd update ID --claim`, and `bd init --skip-hooks --skip-agents --non-interactive --init-if-missing`. Produces a skill that either reports Beads readiness or asks for one explicit first-run approval; it never creates `.beads` on an inspection-only request.

- [ ] **Step 1: Record a failing baseline.** In `v9/tests/CANARIES.md`, specify two disposable Git projects: one with no `.beads`, one with an initialized empty `.beads`. Invoke the installed 8.x skill's setup/status behavior in each and record the actual extra setup gates as the baseline, without changing either user's real project.
- [ ] **Step 2: Verify the baseline gap.** The no-Beads project must not become ready without approval; the 8.x flow still offers its optional bundle. Record tool output and confirm no v9 package exists yet.
- [ ] **Step 3: Write the minimal v9 entrypoint.** Set skill metadata version `9.0.0`. Its first-run branch is exactly: inspect Git/Beads → if Beads absent, ask once → only after approval run the initialization command above → verify `bd --readonly status --json`. For ready projects, read `bd ready --limit 20 --json`, not a full tracker dump. Status must not change tasks, Git, or Agent-Team records; Beads' own first-call Dolt metrics artifact is a documented exception. Task 2 adds host-specific routing; Task 1 routes project records to `references/STATE.md` without linking the not-yet-created `HOSTS.md`.

```text
status: bd --readonly status --json; report only, with no Agent-Team/setup/task writes
start: bd ready --limit 20 --json; select disjoint work
missing .beads: request approval; only then bd init --skip-hooks --skip-agents --non-interactive --init-if-missing
```
- [ ] **Step 4: Run the fixture portions of both canaries.** Use the installed managed v8 executable by its verified path, not `PATH` alone. Compare full before/after file content (including an existing `.gitignore`); inspection may create only the observed Beads-owned Dolt metrics artifact, while no task, Git, Agent-Team, or other project file changes. `bd init` preserves prior content while creating only its own files. Verify the bounded `bd ready` response contains at most 20 rows. A live v9 Skill invocation is deferred until a native host can load the draft; record that as unverified here and run it at the core acceptance handoff, never substitute command/text searches for that host canary.
- [ ] **Step 5: Commit** only the three files with `feat(v9): add Beads-first skill entrypoint` after independent review.

### Task 2: Native team dispatch and default Ponytail contract

**Files:** Create `v9/agent-team/references/HOSTS.md` and `v9/agent-team/references/WORKER_RULES.md`; modify `v9/agent-team/SKILL.md` and `v9/tests/CANARIES.md`.

**Interfaces:** Consumes a claimed Beads task and Git worktree, then the active host's native spawn/follow-up/observation tools. Produces an actual returned worker handle in the current session, never a synthetic ack. Default concurrent-team cap is two until Task 5 adds a user setting. Task lists are at most four ordered tasks, with only the current task claimed.

- [ ] **Step 1: Add a RED live canary.** Create two independent Beads tasks in a disposable repo and ask the v9 draft to launch two workers. Before this task, the entrypoint cannot perform native dispatch or keep a same-session team; record that missing behavior.
- [ ] **Step 2: Define the worker report and authority.** `WORKER_RULES.md` must forbid main-worktree feature edits, other-team writes, self-review, integration, deployment, fabricated evidence, and unapproved scope expansion. It must require the smallest working change, deletion of unnecessary abstractions, targeted tests, and a report containing Beads ID, branch/worktree, changed files, revision, tests, blockers, and relevant decision/mistake IDs. If the Ponytail skill exists, invoke it; the embedded rules still apply if absent.

Read `AGENT_TEAM_RULES.md` when present. Direct user instructions outrank it; it cannot widen the packaged worker contract. Send only rules relevant to the assignment, not the whole project history.

```text
TASK: atv-demo-1
WORKTREE: .worktrees/atv-demo-1
REVISION: 0123456789abcdef0123456789abcdef01234567
CHANGED: src/example.go, src/example_test.go
CHECKS: go test ./... — PASS
BLOCKERS: none
REFERENCES: M-018, D-004
```
- [ ] **Step 3: Implement host routing.** `HOSTS.md` names Codex `spawn_agent`/`followup_task`/`list_agents` and Claude's supported Agent/resume facility without pretending they share one API. Codex `list_agents` observes the current root tree, not all historical sessions; absence there is not proof of no worker. The entrypoint chooses disjoint worktrees, supplies bounded context, retains actual handles in the active session, and refills a completed team with at most four more ordered tasks. On each bounded host wait, inspect completion/status; after an agreed task interval without progress, request a status report, record a stale/hung concern, and continue other lanes. If a host capability is absent, report it; do not invent a shell worker or silently kill a stale one.
- [ ] **Step 4: Run the GREEN canary.** Observe two actual handles and worktrees; verify no shared writable path, no unclaimed later task marked in progress, and a retained same-host team receiving its next task. An explicit spawn rejection must leave no fictitious handle or completion. Delay one worker beyond its agreed progress interval; verify the orchestrator requests status and the other lane still advances.
- [ ] **Step 5: Commit** the four changed files with `feat(v9): dispatch bounded native teams` after independent review.

### Task 3: Real independent review, integration, and task-local blockers

**Files:** Modify `v9/agent-team/SKILL.md`, `v9/agent-team/references/WORKER_RULES.md`, `v9/agent-team/references/STATE.md`, and `v9/tests/CANARIES.md`.

**Interfaces:** Consumes developer revision and test evidence. Produces a non-author FIX or CLEAN report tied to Beads ID and revision; only CLEAN may reach orchestrator integration/commit and `bd close ID --reason ...`.

- [ ] **Step 1: Add RED canaries.** In a disposable project, create one task with a deliberately failing acceptance check and one independent task. Confirm the draft skill does not yet enforce reviewer FIX→remediation→CLEAN or let the unaffected task proceed after the first blocks.
- [ ] **Step 2: Define the review report.** Require `task`, `revision`, `reviewer`, `checks` with observed results, `findings`, and final `FIX` or `CLEAN`; a reviewer must inspect the actual diff and cannot be the author. Empty findings alone are not CLEAN.

```text
TASK: atv-demo-1
REVISION: 0123456789abcdef0123456789abcdef01234567
REVIEWER: independent native agent handle
CHECKS: go test ./... — PASS
FINDINGS: none
VERDICT: CLEAN
```
- [ ] **Step 3: Implement the loop.** Developer repairs FIX findings in the same worktree; reviewer rechecks the new revision. Repeated unresolved findings are recorded against only that Beads issue (`bd update ID --status blocked` plus a comment) while other lanes continue. For CLEAN, orchestrator checks revision/scope and relevant tests, integrates and commits, then closes Beads. It may directly perform only a documented tiny surgical main-worktree fix with separate review.
- [ ] **Step 4: Verify GREEN.** Failed work never closes; the independent task can integrate; a corrected revision obtains real CLEAN, is committed and closed. Confirm clean merged worktrees/branches are removed while dirty or unmerged ones remain. A deployment runs only if an explicit per-project batch command and approval are recorded. Keep at most two on-demand dev servers.
- [ ] **Step 5: Commit** with `feat(v9): require real review before integration` after independent review.

### Task 4: Pause/resume, decisions, mistakes, and uncertainty

**Files:** Modify `v9/agent-team/SKILL.md`, `v9/agent-team/references/STATE.md`, `v9/agent-team/references/HOSTS.md`, and `v9/tests/CANARIES.md`.

**Interfaces:** Consumes current Beads/Git/native observations. Produces `.agent-team/SESSION.md` as a short operational pointer, `BLOCKERS.md` with unresolved user questions only, and append-only `DECISIONS.md`/`MISTAKES.md` entries with stable IDs. Beads remains task authority.

- [ ] **Step 1: Add RED canaries.** Simulate an explicit no-worker spawn rejection, an ambiguous timeout, a temporary NO-GO, a graceful pause, and a later same-host session. Record that the draft has no complete recovery and ledger protocol.
- [ ] **Step 2: Define exact outcomes.** Explicit no-launch returns its claimed Beads issue to open with a comment. Ambiguous launch and temporary NO-GO mark only that issue blocked; no replacement worker starts until observation permits it. Same available handle gets native follow-up. `BLOCKERS.md` entries contain ID, task, question, recommendation, impact, and next prompt; the orchestrator raises unresolved entries at status and integration milestones. Resolutions move to `DECISIONS.md` and are removed from `BLOCKERS.md`. Reuse existing `M-` mistake IDs and add stable `D-` decision IDs without rewriting earlier entries.
- [ ] **Step 3: Add orchestrator-owned checkpointing.** On pause, stop new assignments and ask workers to checkpoint/stop. `.agent-team/SESSION.md` records host/time; active Beads IDs; worktree/branch/revision and uncommitted-work summary; evidence pointers; last observed handles and uncertainty; pending operations/approvals; blockers; and the next action. It contains no full transcript or duplicate task definition. Resume checks Beads and Git before dispatch.

```markdown
# Agent-Team session handoff
Host: codex
Beads task: atv-demo-1 (blocked; native handle uncertain)
Worktree: .worktrees/atv-demo-1; branch: work/atv-demo-1
Git: uncommitted src/example.go; inspect before any reassignment
Evidence: Beads comment on atv-demo-1; test output in worktree
Pending approval: none
Next action: inspect original host handle; continue unrelated bd ready work
```
- [ ] **Step 4: Verify GREEN.** The ambiguous task remains blocked through a session boundary while another ready task proceeds; NO-GO resumes with the same observable handle; user resolution removes its blocker and adds a decision; pause prevents new claims; resume finds dirty work and does not silently overwrite or relaunch it.
- [ ] **Step 5: Commit** with `feat(v9): keep bounded session and decision context` after independent review.

### Task 5: One-time import, model choices, optional aids, and dashboard

**Files:** Modify `v9/agent-team/SKILL.md`, `v9/agent-team/references/STATE.md`, `v9/tests/CANARIES.md`; create `v9/agent-team/assets/dashboard.html`.

**Interfaces:** Consumes approved Project Kickoff handoff or `TASKS.md` only when present, and actual host model metadata. Produces Beads tasks once, saved per-host role preferences, and a local `.agent-team/dashboard/index.html` snapshot after integration; none becomes a second live tracker.

- [ ] **Step 1: Add RED canaries.** Fixture cases: existing Beads task IDs must not duplicate on second import; one-off audit creates one Beads task; missing optional tools cannot block an ordinary code task; a dashboard refresh error cannot reopen accepted work.
- [ ] **Step 2: Implement import and role choices.** Ask before importing. A Project Kickoff handoff's `plan.tasks` contains IDs only: when its selected tracker is Beads, verify those IDs and adopt that database without importing; when it selects `TASKS.md`, read the actual Markdown task rows. Convert those rows once to temporary Beads JSONL, preserving stable task IDs, objective/title, acceptance, dependencies, status, and a source pointer; keep non-task run history in the read-only original file. Run `bd import --dry-run --json tasks-import.jsonl`, compare the proposed IDs/count/dependencies to the source, then run `bd import --json tasks-import.jsonl` after approval. Reject unrepresentable dependencies before mutation. Re-import must not overwrite newer Beads edits. Offer actual available model/effort choices once per host for orchestrator, developer, reviewer, and visual work; save preferences in a small project setting, allow later overrides and cheaper per-task routing.

The project setting is `.agent-team/SETTINGS.md`: default `max_teams: 2`, `dev_server_limit: 2`, deployment disabled until an approved command/batch rule is recorded, and per-host role model/effort choices initially `inherit`. A user's explicit choices replace those defaults. The setting is a preference, never an alternate task store or a reason to claim an unavailable model works.

```text
handoff + Beads tracker: verify plan.tasks IDs using bd show; no conversion
handoff + TASKS.md tracker: task rows → temporary tasks-import.jsonl → bd import --dry-run --json → inspect → bd import --json
plain TASKS.md: use the same one-time JSONL path; archive original as provenance
```
- [ ] **Step 3: Implement optional routing.** Never mention LeanCTX in runtime prompts. Use Serena for a relevant code-navigation/edit task, Graphify for a relevant graph question, Playwright for browser verification, and available visual skills for visual work, only when useful and installed. Missing aids use native code/search/browser fallback or report the task-specific gap without making them setup gates.

Honor installed host skill instructions such as using-superpowers when they apply, without installing or bundling them as Agent-Team requirements.
- [ ] **Step 4: Implement non-blocking dashboard.** A lowest-cost available agent updates the offline HTML snapshot from Beads status after a successful integration, using the packaged template. The README in the distribution plan names its local path. Dashboard failure is reported but cannot change Beads task closure.
- [ ] **Step 5: Verify GREEN and commit.** Re-import is idempotent; 1,000 Beads tasks remain on disk while only a bounded ready page enters the model prompt; all four canaries pass. Commit with `feat(v9): add optional imports and status snapshot` after independent review.

Agent-Team must pass its standalone Beads and one-off canaries with Project Kickoff absent. For the separately tracked, optional Project Kickoff bridge (`atv-uns.11`), run a cross-project interoperability canary after this task: a v9-targeted handoff with Beads task IDs is adopted without duplicate import; a Markdown handoff is offered for explicit one-time import; missing optional aids do not block either path. This bridge does not gate Agent-Team's standalone release. Preserve the current v8 Project Kickoff route, and do not describe the bridge as runtime-qualified or replace the installed Project Kickoff 0.5.2 before its canary is observed.

## Core acceptance handoff

Task 5 completion is not release approval. Run `v9/tests/CANARIES.md` on actual Codex and Claude sessions, record host/tool versions and observable results, and hand any failed scenario back to its owning task. The distribution plan packages only this reviewed core and does not claim an OS/host combination verified without its live canary.
