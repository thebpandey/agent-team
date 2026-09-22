---
name: agent-team
metadata:
  version: "8.0.10"
description: Use when coordinating development in Codex or Claude Code, switching the same Agent-Team project between hosts or sessions, continuing tasks, or requesting setup, settings, start, status, pause, resume, or release.
---

# Agent-Team native v8

## Current native contract

This latest-repository root entrypoint is the current native v8 authority. Install only verified release bytes with `agent-teamctl install --host both --json`; use `codex` or `claude` instead of `both` for one host. Published native downloads support Linux amd64 and Windows amd64; macOS has source verification but no published native binary. Historical Node package v7.3.1 materials are historical context only and are not native authority. `rollback --version <version>` and `uninstall` act only on exact owned bytes.

Route `setup`, `status`, and `start` through the installed native `agent-teamctl` contract. The public actions are `setup`, `status`, `start`, `task add`, `one-off`, `pause`, `stop`, `cancel`, and `resume`. FIX/CLEAN is the internal independent-review state machine. Preserve the selected Beads or `TASKS.md` tracker; only an explicitly approved legacy conversion makes the former tracker provenance. `BLOCKERS.md` and `DECISIONS.md` are bounded projections. Native v8 needs no external hooks. Host switching is foreground-only and transfers no identity or lease.

Report the installed native binary version when available. If it differs from this repository entrypoint metadata, report that this repository version is available and require a checksum-verified native update; do not call the older controller updated. Without an installed controller, report the repository version as available and installation-needed.

Settings persist supported keys in a receipt-bound overlay; preserve setup and the selected tracker. Both hosts support model/effort preferences for orchestrator, developer (coder alias), reviewer, and visual_reviewer. Native start admission is not a launch. Follow the actual host's installed skill: Codex calls its collaboration tool; Claude calls its Agent tool. Acknowledge the exact returned handle. Retained reuse requires actual completion, independent CLEAN review, observed idle state, and the same handle; Claude must support same-agent resume. Observe foreign live handles without duplicating them. Each follow-up receives a fresh bounded packet. A standalone command or fictional worker shell command does not establish launch.

Replay preserves `actual_host` from the packet owner, that host's developer profile, and the original worker identity. Honor `observation_required`. Missing acknowledgement after a possible launch is uncertain; observe the original host instead of spawning a duplicate.

The dashboard is local-only. Preserve capacity caps and use native fallbacks whenever optional semantic, graph, compression, browser, or visual tooling is unavailable. Do not infer wider authority from a fallback. Release claims require the [benchmark report](https://github.com/thebpandey/agent-team/blob/main/docs/benchmarks/vnext-optional-8.0.0.md) and revision-bound release checks.

One project orchestrator owns scope, task admission, integration and release. Delegate bounded implementation and independent review through the actual host controls. Deliver verified work with small task-specific contexts; keep engineering judgment with the orchestrator and routine state checks in the bundled helpers.

The orchestrator is strictly an orchestrator. It plans with the user, decides everything that can be decided without the user, assigns tasks to teams intelligently, identifies which tasks are independent so they run in parallel across teams, and keeps every team under continuous active supervision. It does not search code, check features, review, visually inspect or verify work itself: before dispatching a planned task and after a team reports completion, it delegates those checks through the actual host's supported verifier controls. Use the user's saved reviewer or visual_reviewer model/effort profile; inherited choices follow observable host defaults. Report unsupported choices and use only approved fallbacks. Historical fixed model recommendations in [team dispatch](references/TEAM.md#orchestrator-conduct) or platform references do not override these preferences. A worker update or handoff is an event to act on immediately, never a reason to pause orchestration or end the turn while teams are active.

This complete package is Pro. Agent Team Lite is a separate dependency-free package; do not copy Pro-only browser/acceptance/preview procedures or dependencies into it. Publication always needs authority for its destination.

## Native routing before acting

Use a clear natural-language request, `$agent-team` in Codex, or `/agent-team` in Claude Code. The skill supplies operating guidance; the installed native contract performs the action. Do not route current work through the historical Node package or its hooks.

Resolve the executable with `command -v agent-teamctl` (`Get-Command` on Windows), then the install manifest's owned binary path. On Linux its default is `~/.config/agent-team/bin/agent-teamctl`; `XDG_DATA_HOME` overrides the base, otherwise `XDG_CONFIG_HOME` overrides `~/.config`. macOS uses `~/Library/Application Support/agent-team`; Windows uses `%LOCALAPPDATA%\AgentTeam`. Each data directory contains `install-manifest.json` and `bin`. Invoke the absolute path when PATH has no controller.

The remaining sections are current native operating guidance for actions, models, teams, review, recovery, and release. Historical Node materials do not supply an alternate current-action route.

| Request | Read and do |
| --- | --- |
| Help or version | Inspect the installed native entrypoint; no setup or mutations |
| Status | Read recorded facts only; then continue an already-authorized active run |
| Setup | Prepare selected capabilities and enter the current settings flow before readiness |
| Take over or switch host/session | Verify the same native Git project and continue without transferring identity |
| Settings | Show roles, model, and effort; edit only the requested setting |
| Start or feature request | Establish readiness, then dispatch bounded native work |
| Pause or resume | Preserve explicit pauses, claims, and pending operations |
| Release | Require the exact revision and existing authority |

Use [compact output](references/OUTPUT.md): state first, role labels, meaningful changes and user action only when needed. Show branding on first setup/help, not every progress update.

## Establish readiness once

Confirm the actual host from trusted runtime metadata; load only [Codex](references/PLATFORM-CODEX.md) or [Claude Code](references/PLATFORM-CLAUDE.md). Preserve independent per-host settings; a skill cannot change its parent model.

Run `setup --host <actual-host> --json` and follow its `next_action`; consult [native setup](references/SETUP.md). Reuse the nested Project Kickoff 0.5.0 handoff at `.project-kickoff/AGENT_TEAM_HANDOFF.json` without repeating its interview. Approve import with `setup --approve-kickoff --host <actual-host> --json`, including `--kickoff <path>` for an explicit path. A late handoff can attach to existing setup without resetting tracker, settings, or active packets. If planning is unfinished, prepare approved dependencies with `setup --install <selected-names> --approve --prepare-only --host <actual-host> --json`, then finish approved Kickoff planning and tracker seeding before import. Project Kickoff is optional; approved setup without a handoff can create a minimal scaffold.

Preserve the selected Beads or root/designated Markdown tracker across worktrees. Missing UI skills or a temporarily unavailable backend never select another tracker. Follow [state](references/STATE.md) and [projects](references/PROJECTS.md).

Offer selected Beads plus default Serena, Graphify, rg, ast-grep, and lean-ctx as one missing-dependency bundle. Include required scoped installer/Python preparation in that consent, carry prior approval forward, and execute the approved native `setup --install` operation. Do not alter global PATH or registry. Observations distinguish executable availability from project preparation and worker access. Required capabilities gate affected work; optional failures retain native fallbacks. Preserve tracker, trust, credentials, and customization. A rejected/cancelled `--refuse-kickoff` stops setup/dispatch; `inspect_tracker` means inspect task readiness rather than invent ready work.

When setup returns `next_action: settings`, show [supported settings](references/SETTINGS.md). Save accepted choices, including inherited defaults: `settings codex.developer.model=inherit --json` (use `claude` in Claude). Inspection alone leaves settings revision zero; an accepted save advances it. Cancel or no answer saves nothing. Settings affect future dispatch and cannot change the parent model. Resume the original start request when setup and required capabilities are ready.

## Dispatch a bounded task

Each assignment contains task IDs, acceptance, exclusive paths/worktree, input revision, relevant decisions and prohibitions, actual model/effort, required skill paths, evidence destination and next action. Fresh workers read their own applicable instructions; retained workers reuse still-valid context. Do not fork the full conversation or load every prepared skill into every agent.

Use the smallest useful team within actual host capacity, reserving review capacity. Default to a developer and independent reviewer. Before dispatch, derive the parallel set from the tracker: tasks with no unmet dependencies and disjoint writable paths/resources run at once on separate teams in separate worktrees; tasks that share an entrypoint, schema or resource serialize behind one writer. Use Graphify `affected` and `path` when available and useful; missing Graphify evidence never replaces or blocks deterministic path-overlap validation. Send any pre-dispatch code search, feature check or review to the delegated verifier rather than doing it in the orchestrator context. One writer handles each shared file or resource at a time. Only the orchestrator spawns; do not add another scheduler, database or agent hierarchy.

## Execute and repair continuously

Claim under the selected tracker's supported atomic or single-writer controls. Implement with relevant TDD/debugging guidance and inspect exact source. Independently review requirements and code quality, repair findings, recheck affected behavior and integrate the verified revision serially.

Ordinary lint, test and review failures trigger automatic in-scope repair—never “shall I fix this?” or “continue?”. After two attempts without useful progress, diagnose and change approach or use an approved escalation. Park a genuinely blocked task after a verified safe checkpoint and stopped writer; retain its claim, gates and worktree while freeing compute for independent ready work. A retry cap never waives required acceptance.

Accepted integration evidence queues completed top-level, nondeployed delivery tasks in integration order, including recovered completions already admitted to the run. Incomplete top-level integration fails closed; subtasks and epics never enter the deployment queue.

Continuous runs refill from authorized eligible tasks without new prompts. A proposed improvement is not authorized implementation; record it in Beads or the same TASKS.md, not backlog.md. Ask only for genuinely missing authority/access or a material product decision. Explicit project pause holds admission.

## Active host-turn supervision

While an authorized run has live work, use this one ordered loop:

1. Consume every new user message, worker update, completion, handoff, review verdict, and provider result.
2. Classify steered user input as a replacement, compatible addition, or status/question. Safely checkpoint and stop only work that a replacement conflicts with; reconcile an addition against dependencies, ownership, and conflicts before admission.
3. Answer a status/question briefly in commentary. A question is an interrupt, not a pause, cancel, ownership loss, or terminal condition.
4. Reconcile every live worker and completed handoff with canonical state.
5. Route exact revisions through repair or independent review, then serially integrate only accepted work.
6. Prove the previous writer stopped or explicitly handed off its worker slot before treating compute as free; unknown liveness remains occupied.
7. Recompute actual capacity, reserve reviewer capacity, and admit the next authorized eligible disjoint task. Only the project orchestrator refills.
8. While active workers remain in the current host turn, supervise with the effective `supervision.heartbeatSeconds`, default 600 and minimum 60. Use shorter host waits when required without treating each return as a heartbeat deadline. At the configured interval, emit exactly one compact heartbeat from already-known state and blocker facts, then reconcile and repeat. A lower interval consumes more orchestrator turns.

A scoped blocker is recorded and reported immediately while independent implementation, review, integration, publication, recovery, and refill continue. Repeat an unchanged blocker in the heartbeat without probing for novelty. A blocker is global only when no authorized safe work remains or one irreversible/security decision governs everything left. Before one final blocking question, boundedly inspect uncertain operations without retrying them and durably record active workers, occupied/free/unknown slots, blockers, pending decision, operation identity/evidence, pending tail, and next eligible dispatch.

The host turn ends only when authorized work is complete, the user explicitly pauses or cancels the applicable scope, a genuine global blocker needs that question, or the host interrupts. This loop creates no cron, daemon, timer, hosted monitor, nested scheduler, recurring job, or activity after a final response or host interruption. Its heartbeat uses ordinary host model/output usage; exact incremental cost is unknown.

## Verify, retain and recover

Tests prove the checked revision and environment only. Keep unavailable, failed, timed-out and unrun checks distinct. Final checks after a team finishes (acceptance re-run, review, visual review, feature verification) are delegated to the verifier route; the orchestrator reads the verifier's compact verdict and evidence pointer, routes findings back to the owning developer, and integrates only the accepted revision. The [lifecycle hooks](references/HOOKS.md) describe historical Node behavior, not a native prerequisite. Read-only status never installs, repairs, checkpoints, dispatches or starts servers.

Cleanup and deployment are separate. After verified integration, reclaim only safely stopped, clean task-owned resources with retained evidence; preserve dirty/untracked/ignored user files and required previews. Publish only to the authorized destination through required gates.

Persist decisions, exclusions and consequential operation facts when they arise. Tracker owns tasks; project docs own decisions; [recovery](references/RECOVERY.md) checkpoints point to current attempts, evidence and next action. A fresh host or session validates the same Git project, revision, writer liveness and pending operations, then continues; recorded coordinator identity is never a mutation gate. Keep native auto-compaction enabled as fallback; do not promise automatic parent replacement or execution after host exit.

Measure total accepted-work cost across orchestration, developers, review, repair and recovery. Label missing usage and estimates; cached input is a subset, not extra context capacity. Report implemented, verified, integrated and released states separately.
