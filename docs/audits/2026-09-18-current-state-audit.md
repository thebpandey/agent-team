# Agent-Team 7.3.1: current-state audit

Baseline: commit `7680da3714a3222b27e6c3f6f734fcfbfd9be1dc`, 18 September 2026. Three agents examined the repository read-only: a Terra agent audited the whole package; two Luna agents checked dependencies and portability/ownership independently. This document and the [architecture infographic](agent-team-current-state.architecture.html) are the only outputs added by this exercise. “Current” below means the checked source and documentation, not proof that every configured host event, model, or companion tool works on every machine.

## What exists today

The repository has 196 tracked files. The installed package is declared as version 7.3.1 in [SKILL.md](../../SKILL.md) and [hooks/manifest.json](../../hooks/manifest.json). The root skill is 84 lines, backed by 27 references (1,910 lines), 37 `hooks/lib` modules (14,813 lines), two Node entrypoints, two host hook manifests, six Claude role templates, helper scripts, a dashboard, and extensive tests. The source is an orchestration product with setup, package management, policy enforcement, state migration, recovery, and release machinery. It is much more than a dispatch prompt. [Package manifest](../../hooks/manifest.json) · [Code entrypoints](../../hooks/agent-team-cli.mjs) · [Hook entrypoint](../../hooks/agent-team-hook.mjs)

The nominal path is:

1. Install the skill and register host hooks; initialize project settings and prepare selected default dependencies. [Setup](../../references/SETUP.md) · [Dependencies](../../references/DEPENDENCIES.md)
2. Select one canonical tracker: Beads or a designated `TASKS.md`; record project identity, run defaults, teams, and evidence in `.agent-team`. [State](../../references/STATE.md#one-authoritative-task-graph) · [Project resolver](../../hooks/lib/project.mjs)
3. The orchestrator chooses ready, nonconflicting tasks and creates isolated worktrees/lanes; each lane carries an ordered task queue, packet, worker generation, and handover history. [Skill](../../SKILL.md#dispatch-a-bounded-task) · [Lanes](../../references/LANES.md)
4. A developer implements; a delegated mechanical verifier checks; an independent reviewer challenges the exact revision; findings return for repair. [Team](../../references/TEAM.md#repair-and-handoff) · [Lane quality route](../../references/LANES.md)
5. Versioned evidence gates completion; the orchestrator serially integrates accepted revisions, optionally runs preview/release gates, and deploys eligible task batches. [Runs](../../references/RUNS.md) · [Release](../../references/RELEASE.md)
6. Host events invoke hooks for policy checks, lint, checkpoints, recovery hints, telemetry, and completion evidence. [Hooks](../../references/HOOKS.md) · [Host manifests](../../hooks/manifest.json)

Today `start` accepts 1–6 teams, while lanes already support ordered retained-worker task queues. The queue validator does not impose the proposed eight-task cap; lane briefs can contain up to 6,000 words and task packets up to 400 words. This is a useful starting seam for the redesign, but its current record/phase protocol is elaborate. [Actions](../../references/ACTIONS.md) · [Lane schema](../../hooks/lib/lane-schema.mjs) · [Lane templates](../../references/LANES.md)

The infographic shows this main path and its state sidecars. It groups companion dependencies to stay readable; the complete named inventory is below.

For continuity, the resolver can load `.agent-team/setup.json`, `state.json`, `operation-mappings.json`, the selected tracker, `TEAMS.md`, checkpoints, handoffs, locks, and owner-recovery records. Lanes add brief, packet, handover, and result files. The separate project `CONTEXT.md` and `MISTAKES.md` supply resumption context and confirmed lessons. This persistence is real, but a new host must reconcile several records to know the next safe action. [Resolver](../../hooks/lib/project.mjs) · [State/memory](../../references/STATE.md) · [Lanes](../../references/LANES.md)

## Dependency inventory

The classifications matter: “default” means first-use preparation is selected by the current catalog, not that every task must use the tool. Only an explicitly listed `plan.requiredCapabilities` gates a task; the current skill says other failed defaults stay diagnostic. [Skill: readiness](../../SKILL.md#establish-readiness-once) · [Catalog](../../hooks/lib/dependency-catalog.mjs)

| Class | Current components | Evidence / role |
| --- | --- | --- |
| Core hosts and runtime | Codex or Claude Code; native spawn/message/wait/stop/model controls; Git; Node.js 24 and standard library | [Host adapters](../../references/PLATFORM-CODEX.md), [Claude](../../references/PLATFORM-CLAUDE.md); [hook runtime](../../references/HOOKS.md#agent-team-lifecycle-hooks) |
| Onboarding/conditional executables | `gh` for the documented GitHub setup; npm; browser; `tar`; Python 3.12/3.13 through `uv`; Beads `bd` only with Beads | [Getting started](../../GETTING_STARTED.md), [catalog](../../hooks/lib/dependency-catalog.mjs), [release extractors](../../hooks/lib/dependencies.mjs) |
| Prerequisite in catalog | `uv` 0.12.10, used by Serena and Graphify | [Catalog, lines 26–37](../../hooks/lib/dependency-catalog.mjs) |
| Default tools | Serena 1.7.0 with language server/MCP; Microsoft Playwright CLI 0.1.19 plus browser; ast-grep CLI 0.45.3; Graphify 0.9.57; LeanCTX 3.10.1 | [Catalog, lines 32–71](../../hooks/lib/dependency-catalog.mjs) |
| Default skill bundles | Superpowers (14 named skills); Ponytail (3 named skills); Impeccable 4.1.0 guidance and detector; React Best Practices | [Catalog, lines 73–140](../../hooks/lib/dependency-catalog.mjs) |
| Selected tracker | Beads 1.2.2 / `bd`, or no extra tool for Markdown `TASKS.md` | [Catalog, lines 142–147](../../hooks/lib/dependency-catalog.mjs) · [State](../../references/STATE.md) |
| Optional catalog entries | Context7 MCP 4.0.6; Project Kickoff; `beads_viewer` 0.24.1 dashboard provider; `subagent-tax` maintainer diagnostic | [Catalog, lines 148–168](../../hooks/lib/dependency-catalog.mjs) · [Dependencies](../../references/DEPENDENCIES.md) |
| Host/plugin route | Claude's preferred delegated verifier uses `gpt-5.6-sol` via the installed Codex plugin; `claude-opus-5` is documented as fallback. Codex uses native delegated agent controls. | [Claude adapter](../../references/PLATFORM-CLAUDE.md#role-map) · [Team](../../references/TEAM.md#orchestrator-conduct) |
| MCP surfaces | Serena MCP; optional Graphify stdio MCP and Context7 MCP; mapped arbitrary MCP operations in the hook policy | [Catalog](../../hooks/lib/dependency-catalog.mjs) · [Hook blind spots](../../references/HOOKS.md) |
| Optional UI choices in references | UI UX Pro Max, UI Skills, shadcn/ui, Magic UI, Motion, React Bits, Taste Skill, img2threejs, Awesome DESIGN.md, Bklit UI | [UI](../../references/UI.md) · [UI optional](../../references/UI-OPTIONAL.md) |
| Explicitly excluded catalog entries | Vercel Agent Browser, RTK, beads_rust, Backlog.md, GSD Core, Ralph, Caveman runtime, Matt Pocock TDD/diagnosing-bugs/code-review skills, ast-grep companion skill, Ponytail gain | [Catalog, lines 169–175](../../hooks/lib/dependency-catalog.mjs); not current runtime requirements |

The 14 selected Superpowers skill paths are `using-superpowers`, `test-driven-development`, `systematic-debugging`, `requesting-code-review`, `receiving-code-review`, `verification-before-completion`, `brainstorming`, `writing-plans`, `subagent-driven-development`, `executing-plans`, `dispatching-parallel-agents`, `using-git-worktrees`, `finishing-a-development-branch`, and `writing-skills`. Ponytail selects `ponytail`, `ponytail-review`, and `ponytail-help`. [Exact catalog paths](../../hooks/lib/dependency-catalog.mjs)

The current dispatch selector adds TDD, debugging, and Ponytail to developer roles; verification guidance to reviewer roles; Impeccable and Playwright to UI work; React rules to React work; brainstorming/plans to unresolved planning; and Graphify to most nontext developer, reviewer, and orchestrator assignments. This is task filtering, but the default installed surface and fresh-agent instruction reads remain broad. [Selector](../../hooks/lib/dependencies.mjs)

`ui-styling` is a requested option for a future design; it is not in the present catalog. `ui-ux-pro-max` appears as an optional UI choice, not a default install. Impeccable is presently a default. [Catalog](../../hooks/lib/dependency-catalog.mjs) · [UI](../../references/UI.md)

### Hook dependencies and events

Both adapters execute the shared Node hook entrypoint. Codex registers `SessionStart`, `PreToolUse`, `PostToolUse` for edits, `PreCompact`, and `Interrupt`. Claude registers `SessionStart`, `PreToolUse` for Bash/Edit/Write/Skill/MCP, `PostToolBatch`, `PreCompact`, `TaskCompleted`, and `UserPromptExpansion`. The host event surfaces are different; Codex has no reliable skill-activation event. [Codex registrations](../../hooks/codex-hooks.json) · [Claude registrations](../../hooks/claude-hooks.json) · [Hook comparison](../../references/HOOKS.md#runtime-mapping)

The manifest declares 15 cross-host policy areas. The implementation distributes them across `recovery`, `policy`, `analyzers`, `lint`, `checkpoint`, `telemetry`, `package-validator`, and `artifacts`, with supporting modules for budgets, operation mapping, locks, dependencies, lanes, tracker, run state, cleanup, installer, and dashboard. Some checks are advisory and some deny actions; the docs expressly acknowledge unmapped hosted tools, aliases, continued stdin input, and process death before a hook as blind spots. [Manifest policies](../../hooks/manifest.json) · [Hook requirements and limits](../../references/HOOKS.md#requirements-115)

## Findings that explain the reported pain

| Finding | Status | Evidence and consequence |
| --- | --- | --- |
| Native Windows is not supported end to end | Confirmed | Hook commands interpolate POSIX `$HOME`; artifact installation explicitly requires Linux/WSL `/proc/self/fd` and says there is no Windows fallback; `captureWriterIdentity()` reads Linux `/proc` files. `uv` and `beads_viewer` release selectors exclude Windows. Some npm and LeanCTX paths recognize Windows, so support is partial rather than absent everywhere. [Hooks](../../hooks/codex-hooks.json) · [Installer limitation](../../references/HOOKS.md) · [Writer identity](../../hooks/lib/task-transitions.mjs) · [Asset selectors](../../hooks/lib/dependencies.mjs) |
| Coordinator host switching is already allowed, but active writer transfer is harder | Confirmed / design limit | `validateNativeOwnerAuthority()` checks native session identity and the same canonical Git project/worktree, not historical coordinator identity. Docs say no takeover handshake. The separate writer-liveness rule treats unknown as occupied, and Linux process identity prevents native Windows from establishing that evidence. Native agent context itself cannot be resumed across different hosts from these files; the project can continue from tracker/checkpoint/evidence. [Continuity](../../references/ACTIONS.md#continue-in-another-host-or-session) · [Authority](../../hooks/lib/owner-recovery.mjs) · [Liveness](../../hooks/lib/task-transitions.mjs) |
| Task authority is conceptually single, operational state is spread across many records | Confirmed architecture; drift risk is an inference | Beads/`TASKS.md` owns tasks, while setup/state, TEAMS, lane queue/history, claim receipts, context, checkpoints, operation mapping, evidence, and release records also affect dispatch or recovery. Each layer has version/lock/reconciliation rules. This increases the number of facts a switching host must load and reconcile. [State](../../references/STATE.md) · [Lanes](../../references/LANES.md) · [Project resolver](../../hooks/lib/project.mjs) |
| A normal task traverses too many roles/gates for the proposed lightweight loop | Confirmed workflow; cost impact unmeasured | The orchestrator is forbidden from code search/review/verification and delegates predispatch and final checks. Lanes prescribe developer → delegated verifier → independent reviewer → repair → completion gate → serial integration. This can be valuable for high-risk work, but each step adds contexts and handoffs. [Skill](../../SKILL.md) · [Team](../../references/TEAM.md#orchestrator-conduct) · [Lanes](../../references/LANES.md) |
| First-use and per-agent instruction surfaces are large | Confirmed; token impact unmeasured | The catalog prepares many defaults, including 14 Superpowers skills. Fresh workers must read applicable complete skill instructions. There is no measured per-accepted-task token comparison in this audit, so “token intensive” is supported by surface size, not a quantified savings claim. [Dependencies](../../references/DEPENDENCIES.md) · [Selector](../../hooks/lib/dependencies.mjs) |
| Extra subsystems sit on the core path or alongside it | Confirmed | Package lifecycle, dependency qualification, policy hooks, telemetry, dashboard, LeanCTX context shrinking, Graphify graphs, and lane projections expand setup and recovery even when the implementation task is ordinary. [CLI](../../hooks/agent-team-cli.mjs) · [Catalog](../../hooks/lib/dependency-catalog.mjs) · [Manifest](../../hooks/manifest.json) |

The archived `legacy/claude-v3` tree contains another Claude-only orchestration model. It is not installed by the current package, so it is repository/history noise, not an active dependency. [Archive marker](../../legacy/claude-v3/ARCHIVED.md) · [Manifest legacy entry](../../hooks/manifest.json)

## Redesign session: decisions to make next

The following is a discussion frame, not an approved design or change to the skill:

1. Define one compact, host-neutral state contract: task ID, ordered team queue (at most eight tasks), assigned writer/reviewer, writable paths, current status, exact revision, check result, next action. Decide which fields belong in Beads/`TASKS.md` and which truly need a separate small file.
2. Define `CLEAN` as a concrete handoff: required task checks passed, independent review findings resolved, no ambiguous writer or Git state, and one short receipt. Decide whether low-risk tasks need a distinct reviewer or whether review intensity scales with risk.
3. Define host switching at a safe boundary: current native workers finish, stop, or checkpoint; the new host reads the same tracker/context/mistakes and resumes from a task receipt. Live agent context transfer should not be promised.
4. Set the team/reuse contract: configured parallel team limit, up to eight serial tasks per team, complexity-based worker count, one writer per overlapping path, and automatic next-list assignment to idle retained teams.
5. Decide the core dependency budget: likely native host agents, Git, one tracker, `CONTEXT.md`, and `MISTAKES.md`; visual work conditionally reads Impeccable plus `ui-styling` or `ui-ux-pro-max`. Decide whether any hook, installer, dashboard, Graphify, Serena, LeanCTX, Playwright, or bundled procedural skill survives as an explicitly optional add-on.
6. Specify the orchestrator's final review/commit and batch-deploy boundary, including what “commit” means when each worker already committed in a worktree and what authorization identifies a deployment destination.

No implementation, setup, hook registration, dependency installation, or deployment was performed for this audit. The infographic was generated as a documentation artifact with Archify; Archify is not a dependency of Agent-Team.
