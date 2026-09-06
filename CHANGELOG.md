# Changelog

## 6.2.0 - 2026-09-06

- Add dependency-free lifecycle hooks for current Codex and Claude Code with shared normalization, canonical project resolution, ownership and completion gates, bounded advisories, and atomic recovery checkpoints.
- Add narrow integration, release, and destructive database enforcement. Preserve scoped authorization and report unsupported tool paths without using an approval prompt.
- Add Claude activation logging, bounded effectiveness audit, explicit Codex activation limits, health dimensions, safe install/rollback, package validation, reproducible archives, and defect regression tests.
- Harden critical error transport, remote/release/completion/database gates, shell moves, recovery/checkpoint facts, telemetry provenance, audit correlation, transactional install/uninstall, Claude role management, and Claude `PostToolBatch` checks.
- Keep `legacy/claude-v3` archived. Record the upstream inspiration revision and Apache-2.0 license without copying upstream source.
- Clarify that a full feature request can create a deduplicated canonical task, while `start` selects already-defined tracker work.

## 6.1.0 - 2026-09-06

- Add installed version metadata and display it below the wordmark, with guidance for comparing copies across machines.
- Include project settings in explicit setup and add creator attribution and the GitHub source.
- Show the wordmark for help and standardize message borders at 74 characters.

- Add counted starts with a maximum of six occupied development teams and continuous refill after each verified integration into main.
- Preserve blocked, paused, interrupted, and preview-waiting teams in the slot count; project-wide pause also holds refill and automatic releases.
- Add task-based auto-deploy batches, exact integration boundaries, final smaller batches, and deployment-failure holds with evidence-based recovery.
- Add project-only settings with run-only command overrides and confirmation when auto-deploy is inherited from saved settings.
- Add help with all commands and combined examples; preserve named feature scope and existing version-specific preview gates.
- Display a solid block-letter AGENT-TEAM wordmark for start, resume, settings, setup, and status; recognize `auto-agent start` as a plain-language alias.
- Frame individual messages with equals-sign borders and team identity or a Project Orchestrator header.
- Update platform adapters, native role instructions, workflow diagrams, and recovery/status records without changing model policies or the proprietary license.

## 6.0.2 - 2026-09-05

- Require a team picker for bare pause and resume, with named teams and an explicit All option.
- Keep explicit all and named/team-ID commands direct, without another scope prompt.
- Keep work unchanged until selection; preserve safe recovery of clearly labeled interrupted teams.

## 6.0.1 - 2026-09-05

- Make unqualified pause and pause all safely pause every team in the current project, even from a feature session.
- Preserve named/team-ID pause for one team. Report partial pauses and pending operations without claiming a complete stop.
- Hold new assignments, integration, and release for paused teams; preserve worktrees, progress, previews, and approval evidence.

## 6.0.0 - 2026-09-05

- Add skill actions for automatic start, named start, read-only status, scoped pause, project-wide or named resume, and version-specific approval.
- Make unqualified start select one ready unassigned task and assign a team number/name without replacing existing task IDs. Keep start separate from resume.
- Add stable project/team identity, one shared record owner, safe cross-session handoffs, and duplicate-writer prevention.
- Reserve main for initialization/planning; require feature worktrees and one serial integration worktree.
- Add a persistent with-preview gate, dedicated development previews, and explicit user approval before integration.
- Add task-count percentages with deduplication, scope, exclusions, freshness, and separate approval/production state.
- Recover unfinished work by its actual stage, including paused teams and incomplete release cleanup, without replaying successful external actions.
- Require preview/process shutdown and eligible worktree/disk cleanup after production verification.
- Update both platform adapters, all native Claude role templates, README examples, and rendered workflow diagrams.
- Keep the current package name and license. New procedures remain Pro-only and excluded from the planned Lite edition.

## 5.3.0 - 2026-09-05

- Add Pro visual browser review with actual screenshot inspection and an optional visual tester.
- Add focused end-to-end acceptance tests for changed user flows and saved results.
- Require concise, plain-English explanations beside feature code and useful updates to existing feature guides.
- Add one shared uppercase `MISTAKES.md` with an orchestrator writer, relevant lessons for all teammates, and linked task evidence.
- Connect these procedures to Codex dispatch, all Claude role templates, context checkpoints, and release checks.
- Keep checks within the existing review and repair loop; exclude the new Pro procedures from the planned Lite edition.
- Publish the completed full package to this repository at the owner's request. Keep its current name, visibility, and MIT license unchanged.

## 5.2.1 — 2026-09-04

- Replace README Mermaid blocks with styled, rendered flowchart images for consistent display.
- Include SVG graphics, PNG previews, and editable Mermaid source for both workflows.
- Update the README summary to name Codex and Claude Code and explain the review-remediate loop.


## 5.2.0 — 2026-09-04

- Explain every recommended and optional tool in simple English using ASD-STE100 principles.
- Rewrite setup and UI tool guides; define technical terms and preserve official commands and sources.
- Add README diagrams for setup/development and authorized release/recovery.
- Apply the same language rule to the lead agent and teammates. Preserve model routing and task controls.


## 5.1.0 — 2026-09-04

- Route Sol-level Claude work to Opus 5 at xhigh effort and Terra-level development/review to Opus 5 at high effort.
- Reserve Sonnet 5 at high effort for Luna-level tasks; add a separate native routine agent definition.
- Refresh existing managed role definitions on upgrade without overwriting user customizations; verify effective effort and report unavailable tiers.
- Keep Fable orchestration, Codex routing, and the Haiku text-only restriction unchanged.


## 5.0.0 — 2026-09-04

- Support Codex and Claude Code through one shared skill and separate runtime adapters.
- Preserve Astra/Terra/Sol/Luna routing on Codex; add Fable 5.1 orchestration, Opus 5 complex development, and Sonnet 5 standard/review/routine work on Claude.
- Reserve Haiku 4.5 strictly for simple text rewrites/paraphrasing; all Luna-equivalent work routes to Sonnet.
- Bundle native Claude agent templates and document installation/discovery without overwriting existing definitions.
- Preserve optional dependency setup, local tracking fallback, agent memory, proportional checks, authorized recovery, and verified worktree cleanup across hosts.


All notable changes to agent-team are recorded here. This project follows semantic versioning.

## [4.1.0] - 2026-09-04

### Changed
- Make Beads, Ponytail, Using-Superpowers, and Impeccable recommended dependencies with opt-in automatic installation.
- Continue after declined/unavailable dependencies using one local TASKS.md tracker, available skills, and built-in guidance.
- Add single-writer task updates, failure/release/reconciliation records, remembered declines, and safe tracker migration.

## [4.0.0] - 2026-09-04

### Changed
- Repurpose Agent-Team as a Codex skill with GPT-6 Astra orchestration, Terra/Luna teammates, and Sol for complex development.
- Replace adversary/judge panels with adaptive implementation and proportional independent review.
- Use Beads for tasks/failures, compact per-agent CONTEXT.md checkpoints, and verified deployment/recovery/cleanup.
- Add first-run dependency inventory, user-selected installation profiles, verified setup receipts, and project-specific optional UI resources.
- Preserve the Claude v3 instructions under legacy/claude-v3; the root SKILL.md is now the supported entrypoint.

## [3.1.0] - 2026-07-14

### Added
- First-class support for OpenAI's official codex-plugin-cc. Stage 0 detection order is now plugin -> MCP -> CLI -> ALL-CLAUDE, with `/codex:setup` as the auth check.
- HYBRID-PLUGIN mechanics: T1 delegation targets the plugin's `codex:codex-rescue` subagent via the Task tool (--model gpt-5.6-sol --effort high); the Adversary runs `/codex:adversarial-review --background` with per-ticket focus text.
- Background job tracking: tickets carry a CODEX_TASK id; the orchestrator polls /codex:status and /codex:result instead of heartbeats for Codex-lane work, never reaps a running job, and uses /codex:cancel as the kill switch.
- The plugin's Stop-hook review gate documented as opt-in for high-risk phases only, with the usage-drain warning.

### Changed
- The hand-rolled `codex exec` wrapper is now the HYBRID-MCP/CLI fallback rather than the primary Codex path.

## [3.0.0] - 2026-07-14

### Changed (breaking: generator becomes operator, Claude Code only)
- The skill no longer generates prompts for other surfaces. Invoked inside Claude Code, the session itself orchestrates end to end: decomposition, phase tagging, crew definition, routing, gated execution, verification, merge, report. Claude Chat, ChatGPT, and Codex-as-runtime support removed; `portable/` deleted.
- Codex is now a detected worker engine. Stage 0 probes for it every run; HYBRID topology puts GPT-5.6 Sol on Tier 1 and the Adversary, with Opus reviewing Codex-built code (cross-family review). ALL-CLAUDE fallback noted honestly as weaker for review.
- Model roster refreshed to verified July 2026 engines: Fable/Opus 4.8/Sonnet 4.6/Haiku 4.5 and GPT-5.6 Sol/Terra/Luna (GA 2026-07-09) plus GPT-5.4-mini. Haiku readmitted as Tier 4.
- Orchestrator judgment/delegation split codified; micro-fix exception added (10 lines max, one file, non-high-risk, logged to /goals/microfix-log.md, sampled by the Adversary).
- Adversary gating is risk-tiered: mandatory for high-risk and Tier 1 tickets pre and post execution, 1-in-5 sampling for routine work, per-tier rubric, veto power.
- Persistent protocol infrastructure: /specs (Agents.md, architecture.md, design.md), /goals tickets with a STATUS state machine and atomic claims, heartbeats with a 20-minute reaper, worktree isolation, verifier-never-author, 2-fail escalation and 3-fail halt, per-ticket token budgets with an 80% session checkpoint. Crew agent files are now persistent (created by init), reversing v2's per-run auto-clean; scratch (./.agent-team-tmp/) still auto-cleans.
- references/templates.md replaced by references/protocol.md (tickets, state machine, scaffolds). New /architect planning-only command. New Adversary persona: Terry Benedict.

## [2.0.0] - 2026-07-09

### Changed (breaking architecture)
- Stage A now defines the crew and the implementation plan (phases, dependencies, difficulty tags) but assigns no agents. Stage B is the orchestrator's job: Danny Ocean reads each phase and routes it to the specialist whose tier fits the phase's severity, complexity, and length. Delegation intelligence moved out of Stage A, as intended.
- Each crew member has a locked model and locked effort (a capability tier), instead of effort derived per task. The orchestrator routes phases to the fitting tier.
- The orchestrator is pinned to Opus 4.8 (Claude Code) or GPT-5.5 (Codex) at high or xhigh effort.
- Effort is now set as a real field on both runtimes: Claude Code `effort:` (Opus or Sonnet only, not Haiku); Codex `model_reasoning_effort`. Prior belief that Codex lacked per-agent effort was incorrect. Effort band is medium to high.

### Added
- Agent files created by a run are auto-deleted after the report is displayed, alongside the temp directory. Only run-created files; pre-existing agents are never touched.
- Roster in personas.md now lists the locked model and effort per character for both runtimes.

## [1.3.0] - 2026-07-09

### Changed
- Two-stage output. The generated prompt now has Stage A (define each agent as a real named agent with a bound model and tool scope) and Stage B (run the phases and fan in). Earlier versions only labeled agents inline in a spawn call, so names and per-agent models did not stick. This is the fix for that.
- Agent names and ASCII prefixes are always on, independent of heist mode.
- Effort binds to a real field: Claude Code `model:`, Codex `model:` plus `model_reasoning_effort:`. No longer a prose hint.
- Heist mode is on by default. Agents write logs and report sections in character, each report section closed by a plain-language summary in parentheses. Next steps stay plain in both modes. Plain mode ("plain mode" / "heist off") drops the voice and keeps the names as labels.

### Added
- Codex agent definitions via `.codex/agents/<name>.toml` (name, description, developer_instructions, model, model_reasoning_effort), alongside Claude Code `.claude/agents/agent-team/<name>.md`.
- Overwrite guard: never overwrite an existing same-name agent; create under a `-at` suffix instead.

## [1.2.0] - 2026-07-09

### Added
- Temp-file lifecycle: large agent outputs go to a run-scoped temp directory and return as a summary plus file path, preventing parent context overflow on fan-in. After the consolidated report is displayed, the temp directory is auto-deleted, scoped to that directory only, never pre-existing files or real deliverables.
- Portable prompt (`portable/agent-team-prompt.md`): a single self-contained block for surfaces that do not load skills, so agent-team can be invoked in Claude Chat and ChatGPT (including as a Custom GPT system prompt), in addition to Claude Code and Codex.
- README table documenting all four surfaces and the generate-versus-execute distinction.

### Changed
- Stop conditions now carve out an explicit exception for the run's own temp directory so cleanup does not require a human pause.
- Fan-in step reads temp files and folds needed content into the plain report before cleanup, so nothing needed is lost when the temp directory is deleted.

## [1.1.0] - 2026-07-09

### Added
- Optional heist mode: named Ocean's-Eleven crew personas assigned by sub-task type, with ASCII log prefixes for themed terminal output. Personas color log lines only; the final report stays plain high-school language.
- `references/personas.md`: crew roster, task-type mapping, ASCII prefixes, assignment and fallback rules, and the raw config.
- Teardown instruction: each agent cleans up what it started (servers, containers, temp files, background jobs) before returning.
- Large-output rule: agents write big results to a file and return a summary plus the file path, preventing orchestrator context overflow and session crashes.
- Codex support: fan-out described generically so the generated prompt maps to Codex parallel workers as well as the Claude Code Task tool.

### Changed
- Report section headers use the agent's ASCII prefix.
- Clarified that sub-agents self-terminate on return, so prompts never instruct them to "close." A separate Claude Code process leak is a runtime bug, not a prompt concern.

## [1.0.0] - 2026-07-07

### Added
- Initial release.
- Phase separation engine: decompose a task, map dependencies, tag every phase PARALLEL or SEQUENTIAL, and show run order.
- Agent count logic: use the user's number and flag mismatches, or derive a count capped at 7 concurrent with overflow into labeled waves.
- Effort ladder (Low, Medium, High), defaulting to High, mapping to model preference, verification depth, and turn budget.
- Consolidated fan-in report schema written for a high-school reader: per agent (job, what happened, findings, concerns, failures, successes), then overall summary and next steps.
- Output discipline baked into every generated prompt: no restatement, no narration, no logs, no over-explaining.
- Stop conditions and per-agent scope locks in every generated prompt.
- Cost warning line for large or High-effort fan-outs.
- Claude Code slash command (`/agent-team`).
- References file with the master prompt template and four worked examples (independent fan-out, dependency trap, user count smaller than unit count, waves over the concurrent cap).
- Fully standalone: no runtime dependency on any other skill.
