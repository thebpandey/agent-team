# Changelog

All notable changes to agent-team are recorded here. This project follows semantic versioning.

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
