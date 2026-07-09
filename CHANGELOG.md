# Changelog

All notable changes to agent-team are recorded here. This project follows semantic versioning.

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
