# Changelog

All notable changes to agent-team are recorded here. This project follows semantic versioning.

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
