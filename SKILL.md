---
name: agent-team
metadata:
  version: "7.1.1"
description: Use when coordinating development in Codex or Claude Code, continuing Agent-Team tasks, or requesting agent-team setup, settings, start, status, pause, resume, review, or release.
---

# Agent-Team

One project orchestrator owns scope, task admission, integration and release. Delegate bounded implementation and independent review through the actual host controls. Deliver verified work with small task-specific contexts; keep engineering judgment with the orchestrator and routine state checks in the bundled helpers.

The orchestrator is strictly an orchestrator. It plans with the user, decides everything that can be decided without the user, assigns tasks to teams intelligently, identifies which tasks are independent so they run in parallel across teams, and keeps every team under continuous active supervision. It does not search code, check features, review, visually inspect or verify work itself: before dispatching a planned task and after a team reports completion, it delegates those checks to the host's delegated verifier route described in [team dispatch](references/team.md#orchestrator-conduct) and each adapter ([Claude Code](references/platform-claude.md#delegated-verification), [Codex](references/platform-codex.md)): GPT-5.6-Sol at medium effort, with the Opus 5 fallback in Claude Code. A worker update or handoff is an event to act on immediately, never a reason to pause orchestration or end the turn while teams are active.

This complete package is Pro. Agent Team Lite is a separate dependency-free package; do not copy Pro-only browser/acceptance/preview procedures or dependencies into it. Publication always needs authority for its destination.

## Route before acting

Use `$agent-team` in Codex, `/agent-team` in Claude Code, or a clear natural-language request. These are skill actions, not invented native CLI commands. Read [actions](references/actions.md) and only the references for the requested action.

| Request | Read and do |
| --- | --- |
| Help or version | [Help](references/help.md); no setup or mutations |
| Status | [Status](references/status.md); recorded facts only; then continue an already-authorized active run |
| Setup | [Setup](references/setup.md); prepare selected capabilities, preserve choices and expose genuine manual steps |
| Settings | [Settings](references/settings.md); show roles/model/effort and edit the requested setting; full wizard is optional |
| Start or feature request | Establish readiness, then [runs](references/runs.md) and [team dispatch](references/team.md) |
| Pause or resume | [Recovery](references/recovery.md); preserve explicit pauses, claims and pending operations |
| Preview approval or release | [Preview](references/preview.md) and [release](references/release.md); exact revision and existing authority |

Use [compact output](references/output.md): state first, role labels, meaningful changes and user action only when needed. Show branding on first setup/help, not every progress update.

## Establish readiness once

Confirm the actual host from trusted runtime metadata; load only [Codex](references/platform-codex.md) or [Claude Code](references/platform-claude.md). Preserve independent per-host settings; a skill cannot change its parent model.

Reuse an approved Project Kickoff handoff when present. Otherwise adopt an existing plan/tracker or prepare a proportionate standalone breakdown. Agent-Team does not require Project Kickoff. Resolve scope, acceptance, dependencies, canonical integration branch, checks, capabilities and authority; reopen only affected unresolved decisions.

Preserve the selected Beads or root/designated Markdown tracker across worktrees. Missing UI skills or a temporarily unavailable backend never select another tracker. Follow [state](references/state.md) and [projects](references/projects.md).

First use automatically prepares missing mandatory Serena and Microsoft Playwright CLI plus selected defaults through [dependency profiles](references/dependencies.md). Optional tools require selection. Preparation is not permission to approve trust, obtain credentials, purchase access or overwrite customization.

## Dispatch a bounded task

Each assignment contains task IDs, acceptance, exclusive paths/worktree, input revision, relevant decisions and prohibitions, actual model/effort, required skill paths, evidence destination and next action. Fresh workers read their own applicable instructions; retained workers reuse still-valid context. Do not fork the full conversation or load every prepared skill into every agent.

Use the smallest useful team within actual host capacity, reserving review capacity. Default to a developer and independent reviewer. Before dispatch, derive the parallel set from the tracker: tasks with no unmet dependencies and disjoint writable paths/resources run at once on separate teams in separate worktrees; tasks that share an entrypoint, schema or resource serialize behind one writer. Have the delegated verifier confirm that ownership is actually disjoint with Graphify `affected` and `path` on the integration worktree graph before dispatch. Send any pre-dispatch code search, feature check or review to the delegated verifier rather than doing it in the orchestrator context. One writer owns each shared file or resource. Only the orchestrator spawns; do not add another scheduler, database or agent hierarchy.

## Execute and repair continuously

Claim under the selected tracker's supported atomic or single-writer controls. Implement with relevant TDD/debugging guidance and inspect exact source. Independently review requirements and code quality, repair findings, recheck affected behavior and integrate the verified revision serially.

Ordinary lint, test and review failures trigger automatic in-scope repair—never “shall I fix this?” or “continue?”. After two attempts without useful progress, diagnose and change approach or use an approved escalation. Park a genuinely blocked task after a verified safe checkpoint and stopped writer; retain its claim, gates and worktree while freeing compute for independent ready work. A retry cap never waives required acceptance.

Continuous runs refill from authorized eligible tasks without new prompts. A proposed improvement is not authorized implementation; record it in Beads or the same TASKS.md, not backlog.md. Ask only for genuinely missing authority/access or a material product decision. Explicit project pause holds admission.

## Verify, retain and recover

Tests prove the checked revision and environment only. Keep unavailable, failed, timed-out and unrun checks distinct. Final checks after a team finishes (acceptance re-run, review, visual review, feature verification) are delegated to the verifier route; the orchestrator reads the verifier's compact verdict and evidence pointer, routes findings back to the owning developer, and integrates only the accepted revision. The [lifecycle hooks](references/hooks.md) add bounded enforcement and evidence. Read-only status/health never install, repair, checkpoint, dispatch or start servers.

Cleanup and deployment are separate. After verified integration, reclaim only safely stopped, clean task-owned resources with retained evidence; preserve dirty/untracked/ignored user files and required previews. Publish only to the authorized destination through required gates.

Persist decisions, exclusions and consequential operation facts when they arise. Tracker owns tasks; project docs own decisions; [recovery](references/recovery.md) checkpoints point to current attempts, evidence and next action. Fresh-context handoff must validate ownership, revision and pending operations before continuing. Keep native auto-compaction enabled as fallback; do not promise automatic parent replacement or execution after host exit.

Measure total accepted-work cost across orchestration, developers, review, repair and recovery. Label missing usage and estimates; cached input is a subset, not extra context capacity. Report implemented, verified, integrated and released states separately.
