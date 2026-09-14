# Compact Agent-Team output

Lead with current state and the useful outcome. Use plain language and stable task/team IDs. Show role labels first; optional display names may follow. Do not require a fixed-width frame, ASCII border or repeated wordmark.

[Lane](LANES.md) coordination uses a separate 2000-character worker update receipt with task, revision, evidence pointer, named checks, and one next action. Full logs, briefs, packets, handovers, and review evidence remain at their referenced paths; shortening a message never turns an unrun check into a pass.

A normal progress update contains: changed outcome, important evidence or blocker, and next action. Omit empty fields and routine tool narration. Keep updates event-driven, with a concise heartbeat when lengthy work has no visible transition; do not poll agents merely to manufacture an update.

Example:

> Developer · AT-012: implementation verified; independent review next.
> Reviewer · AT-009: two findings assigned for repair. Other work continues.

Status shows run state, active/parked/ready work, actual capacity, task progress, release state and any necessary user action. A setting view shows role, purpose, effective model/effort, source and enforceability. Use narrow stacked text when a table would overflow; color must not carry meaning alone. Respect no-color and reduced-motion preferences.

Use native selection controls only when exposed. Otherwise use readable numbered options; never invent a host widget. One targeted settings request does not open every wizard stage.

Show the [wordmark](WORDMARK.md), version and creator/source credit on first setup or requested help. Ordinary start, resume, settings, status, repair and handoff responses need no banner. Formatting never creates a tool action or state change.

Handoffs use a compact receipt: outcome; exact revision and owned changes; acceptance/check results with evidence paths; unresolved findings; next action. Keep full logs, screenshots and transcripts out of the parent context unless a specific finding requires them.
