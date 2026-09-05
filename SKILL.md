---
name: agent-team
description: Coordinate software and app implementation on Codex or Claude Code with adaptive developer teams, Beads or local-file tracking, compact agent memory, proportional verification, and authorized release recovery. Use for end-to-end development tasks requiring this harness, coordinated implementation streams, or continued work already tracked through this harness.
---

# Agent-Team

Deliver the requested working result with the least coordination, code, and verification needed to establish it. Preserve every requirement and report its disposition. Avoid speculative abstractions, obscure use cases, duplicate checks, and ceremonial review loops. Keep code readable; fewer lines alone are not evidence of better code.

Use ASD-STE100 principles when you explain tools, installation choices, progress, or results to the user. Use short, active sentences and consistent terms. Explain technical terms at first use. Keep official names and commands unchanged. Give each teammate the same communication rule.

## Edition boundary

This full package is the basis for Pro. Visual browser review, end-to-end acceptance tests, feature-code explanations, and shared mistakes memory are Pro-only. Exclude these procedures and their templates or dependency choices from Agent Team Lite. Lite must remain independent of external plugins, packages, and skills. Publishing a skill update requires authorization for its destination. An ordinary personal skill save does not authorize publication to another repository.

## Start once, resume from evidence

First run a lightweight dependency check. On first use, explicit `setup`/`install dependencies`, a changed environment, or missing dependencies, follow [dependency setup](references/setup.md). Present missing Beads, Ponytail, Using-Superpowers, and Impeccable as recommended dependencies and offer automatic installation with the user's chosen scope. If any is declined, unavailable, or fails to install, continue with the built-in workflow and local-file tracking described in [state and recovery](references/state.md). Reuse the recorded choice; do not repeatedly prompt for declined items. Missing these four tools must never block otherwise feasible work.

1. Identify the actual host from runtime/tool metadata, then load only its adapter: [Codex](references/platform-codex.md) or [Claude Code](references/platform-claude.md). Use that adapter's models, effort, invocation syntax, and delegation controls. A prompt mentioning another platform does not change the runtime. A skill cannot switch its parent model; verify exposed configuration and report a mismatch without pretending it was changed. Resolve an ambiguous host before model-specific dispatch.
2. Load and apply `ponytail` and `using-superpowers` when available and enabled by the user. For UI/UX work, also use `impeccable` when available and enabled. Otherwise use this harness's own simple-code, proportional-verification, and UI guidance without claiming to have run the missing skill. Resolve actual installed skill names and read their instructions; typing a name is not proof of execution. Read [dependencies](references/dependencies.md) at first use or when resolution fails.
3. Read applicable project instructions, current change status, requested behavior, and relevant existing code. Preserve user changes. Reuse established build, test, design, and deployment conventions.
4. Select one authoritative tracker: Beads when the preferred dependencies are usable and enabled, or `.agent-team/TASKS.md` in the main project checkout when any of the four is skipped or unusable. Honor an existing local-mode choice even if a dependency later appears. Use the state reference for ownership, safe switching, and recovery; never keep two live task ledgers.
5. Give every user requirement an ID, observable acceptance criteria, and a corresponding active-tracker task or explicit mapping within a task. Record dependencies, ownership, and the next actionable item. One task may cover a trivial change; use an epic and children for substantial work.
6. Create or read the shared project `MISTAKES.md` using [mistakes memory](references/mistakes-memory.md). Read relevant lessons before work and pass their IDs plus the canonical path to each teammate.

## Route work adaptively

Use the selected platform adapter's role map. Keep the orchestrator on its strongest configured model with high effort. Standard developers handle substantive work; the complex developer handles difficult reasoning or intertwined changes, including upfront assignments. Routine developers handle bounded implementation and checks. In Claude Code, use Opus at xhigh effort for Sol-level work, Opus at high effort for Terra-level development and review, and Sonnet at high effort only for Luna-level work; Haiku is restricted to simple text rewrites or paraphrasing, never coding, debugging, investigation, testing, review, or release decisions.

Spawn only for a bounded task that improves delivery speed or verification quality. Default to one developer and one independent reviewer for substantive work. Add parallel developers only for independent implementation streams, and specialist testing only for concrete needs. Do not create judges, panels, or reviewers of reviewers.

Use the selected host's exposed agent controls and small task-specific context. Runtime settings must enforce routing; prose cannot create unavailable models. Reuse a suitable idle agent when its context remains useful. Only the orchestrator spawns teammates. Read [team dispatch](references/team.md) before the first delegation.

## Execute, verify, finish

1. Claim the next ready task in the active tracker and implement the smallest complete solution. Follow existing architecture; prefer existing utilities, native capabilities, and installed dependencies. Add or update concise [feature-code explanations](references/code-explanations.md) beside the relevant code.
2. Verify changed behavior, common failure paths, and affected integration points. Apply [Pro acceptance tests](references/acceptance-tests.md) to the important changed user flows, including saved results where relevant. For nonbehavioral wording/format changes, use a focused check. For substantive changes, have the developer test and one independent reviewer examine the relevant diff and evidence. Always independently review authorization, payments, schema, or destructive data changes. These are realistic concerns, not obscure cases.
3. Reuse valid results tied to the same relevant source, dependencies, configuration, and environment. After a fix, retest the finding and impacted behavior. Run wider suites only when required by the project or to resolve a concrete uncertainty. A stale result or unrun check is not a pass.
4. Integrate serially into the intended release revision. The orchestrator verifies requirement coverage, integration checks, and release evidence before deployment; do not require another blanket review afterward. UI work follows [UI workflow](references/ui.md) and Pro [visual browser review](references/visual-review.md); when selecting specialized design resources, consult [optional UI routing](references/ui-optional.md).
5. Automatically deploy and recover within the user's standing authorization for the established target and release process, following [release and cleanup](references/release.md). Honor enforced permissions. Verification alone does not authorize an unknown destination or irreversible operation.
6. After every successful deployment, immediately update the active tracker (Beads when in use, otherwise TASKS.md) with the deployed revision, deployment identity, live verification evidence, and remaining issues. Close only tasks whose acceptance criteria are satisfied. If tracking fails, report the deployed-but-unrecorded state and repair tracking without redeploying.
7. Remove eligible task worktrees only after the orchestrator confirms their work is integrated, deployed, and verified. Reconcile every requirement ID against evidence. Finish with a compact requested/result/evidence table, deployment status, unresolved blockers, and cleanup status. Distinguish implemented, verified, deployed, deferred, and blocked work. Never imply complete delivery when required items are missing.

## Keep state small and current

Read [state and recovery](references/state.md) when setting up tasks, handling failures, or checkpointing. The active tracker owns task status and failure history. Project docs own durable architecture and design decisions. The shared `MISTAKES.md` holds confirmed mistakes and prevention lessons, linked to task evidence. Each agent's local `CONTEXT.md` owns only resumption notes and pointers.

Update the active tracker at claim, meaningful progress, blocker, handoff, verification, and release transitions. Share concise user updates on changed outcomes, blockers, and next actions without narrating routine tool calls. Do not leave the user without a meaningful update for more than about a minute during active work.

Create or refresh `CONTEXT.md` before compaction whenever warning is available, at major milestones, and before handoff. Do not rely on an exact context-limit warning. Every teammate does the same in its assigned location. Resume by checking actual files, revision, ownership, and deployment state.

For meaningful failures, record one issue in the active tracker per distinct cause and append evidence-driven attempts. Record confirmed agent mistakes through the shared mistakes procedure. After two attempts without useful progress, change strategy or escalate to the complex developer. If the changed approach still cannot progress, ask one focused question. Do not repeat identical attempts or relax acceptance criteria to manufacture success.

Apply the user's efficiency and team-wide use of available, enabled skills even when dependency skills suggest optional ceremonies or exempt subagents. Do not bypass higher-priority instructions, security controls, or mandatory project gates.
