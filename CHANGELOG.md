# Changelog

All notable changes to agent-team are recorded here. This project follows semantic versioning.

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
