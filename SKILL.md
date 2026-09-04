---
name: agent-team
description: Coordinate software and app implementation with GPT-6 Astra, adaptive developer teams, Beads tracking, compact agent memory, proportional verification, and authorized release recovery. Use for end-to-end development tasks requiring this harness, coordinated implementation streams, or continued work already tracked through this harness.
---

# Agent-Team

Deliver the requested working result with the least coordination, code, and verification needed to establish it. Preserve every requirement and report its disposition. Avoid speculative abstractions, obscure use cases, duplicate checks, and ceremonial review loops. Keep code readable; fewer lines alone are not evidence of better code.

## Start once, resume from evidence

First run a lightweight dependency check. On first use, explicit `setup`/`install dependencies`, a changed environment, or missing dependencies, follow [dependency setup](references/setup.md). Offer installation before declaring required dependencies blocked. Setup may run before missing skills or Beads are available; it must finish or report precise blockers before affected implementation starts. Reuse the recorded choice and verify availability on later runs.

1. Use `gpt-6-astra` with `high` reasoning as orchestrator. Increase effort only for a concrete difficult decision. A skill cannot switch the parent model. Check exposed runtime metadata; do not invent verification or silently substitute a different model. If Astra is unavailable, report the configuration blocker before implementation.
2. Load and apply `$ponytail` and `$using-superpowers` before task work. For any UI/UX work, also load `$impeccable`. Resolve actual installed skill names and read their instructions; typing a name is not proof of execution. Read [dependencies](references/dependencies.md) at first use or when resolution fails.
3. Read applicable project instructions, current change status, requested behavior, and relevant existing code. Preserve user changes. Reuse established build, test, design, and deployment conventions.
4. Require Beads as the authoritative task tracker. Discover the installed version and supported workflow; initialize through supported project setup. If unavailable after the setup opportunity, surface that blocker and continue only useful read-only discovery. Do not create a second Markdown task ledger.
5. Give every user requirement an ID, observable acceptance criteria, and a corresponding Beads task or explicit mapping within a task. Record dependencies, ownership, and the next actionable item. One task may cover a trivial change; use an epic and children for substantial work.

## Route work adaptively

| Assignment | Model and effort | Use |
| --- | --- | --- |
| Orchestration; trivial implementation | `gpt-6-astra`, high | Plan, coordinate, integrate, verify, release; handle small isolated work directly when delegation costs more |
| Standard development; independent review | `gpt-5.6-terra`, medium or high | Substantive implementation, investigation, focused review |
| Complex developer teammate | `gpt-5.6-sol`, high; xhigh if warranted | Deep reasoning, architecture, difficult debugging, intertwined changes; choose upfront when appropriate |
| Narrow routine work | `gpt-5.6-luna`, low or medium | Bounded discovery, straightforward edits, running defined checks, summarizing evidence |

Spawn only for a bounded task that improves delivery speed or verification quality. Default to one developer and one independent reviewer for substantive work. Add parallel developers only for independent implementation streams, and specialist testing only for concrete needs. Do not create judges, panels, or reviewers of reviewers.

Use exposed Codex agent controls, supported model/effort settings, and small task-specific context. Prefer fresh child context over copying conversation history. Runtime settings must enforce routing; this table cannot create unavailable models. Reuse a suitable idle agent when its context remains useful. Only Astra spawns teammates. Read [team dispatch](references/team.md) before the first delegation.

## Execute, verify, finish

1. Claim the next ready Beads task and implement the smallest complete solution. Follow existing architecture; prefer existing utilities, native capabilities, and installed dependencies.
2. Verify changed behavior, common failure paths, and affected integration points. For nonbehavioral wording/format changes, use a focused check. For substantive changes, have the developer test and one independent reviewer examine the relevant diff and evidence. Always independently review authorization, payments, schema, or destructive data changes. These are realistic concerns, not obscure cases.
3. Reuse valid results tied to the same relevant source, dependencies, configuration, and environment. After a fix, retest the finding and impacted behavior. Run wider suites only when required by the project or to resolve a concrete uncertainty. A stale result or unrun check is not a pass.
4. Integrate serially into the intended release revision. Astra verifies requirement coverage, integration checks, and release evidence before deployment; do not require another blanket review afterward. UI work follows [UI workflow](references/ui.md); when selecting specialized design resources, consult [optional UI routing](references/ui-optional.md).
5. Automatically deploy and recover within the user's standing authorization for the established target and release process, following [release and cleanup](references/release.md). Honor enforced permissions. Verification alone does not authorize an unknown destination or irreversible operation.
6. After every successful deployment, immediately update Beads with the deployed revision, deployment identity, live verification evidence, and remaining issues. Close only tasks whose acceptance criteria are satisfied. If tracking fails, report the deployed-but-unrecorded state and repair tracking without redeploying.
7. Remove eligible task worktrees only after Astra confirms their work is integrated, deployed, and verified. Reconcile every requirement ID against evidence. Finish with a compact requested/result/evidence table, deployment status, unresolved blockers, and cleanup status. Distinguish implemented, verified, deployed, deferred, and blocked work. Never imply complete delivery when required items are missing.

## Keep state small and current

Read [state and recovery](references/state.md) when setting up tasks, handling failures, or checkpointing. Beads owns task status and failure history. Project docs own durable architecture and design decisions. Each agent's local `CONTEXT.md` owns only resumption notes and pointers.

Update Beads at claim, meaningful progress, blocker, handoff, verification, and release transitions. Share concise user updates on changed outcomes, blockers, and next actions without narrating routine tool calls. Do not leave the user without a meaningful update for more than about a minute during active work.

Create or refresh `CONTEXT.md` before compaction whenever warning is available, at major milestones, and before handoff. Do not rely on an exact context-limit warning. Every teammate does the same in its assigned location. Resume by checking actual files, revision, ownership, and deployment state.

For meaningful failures, record one Beads issue per distinct cause and append evidence-driven attempts. After two attempts without useful progress, change strategy or escalate to Sol. If the changed approach still cannot progress, ask one focused question. Do not repeat identical attempts or relax acceptance criteria to manufacture success.

Apply the user's efficiency and team-wide skill requirements even when dependency skills suggest optional ceremonies or exempt subagents. Do not bypass higher-priority instructions, security controls, or mandatory project gates.
