# Agent-Team message format

Frame each user-facing Agent-Team response and each individual team update or handoff with this exact top and bottom border:

```text
============================================================================

TEAM-001 / lesson-progress / Developer

Implemented the assigned change. Focused checks passed.
Next: independent review.

============================================================================
```

Use the actual stable team ID, readable feature name, and sender role. For project-wide output use `AGENT-TEAM / Project Orchestrator`. Before team identity exists, use the project label; never invent a team ID to fill the header. Keep messages concise and use the existing required result/evidence fields for their purpose.

The example is fenced to show spacing. In normal Markdown output, place each border on its own line with a blank line between the border and the message body. This prevents the bottom border from turning the preceding text into a Markdown heading. Keep links, tables, and ordinary content as Markdown inside the frame. In a plain-text host, use the same spacing. Do not wrap the whole response in a code fence; only the [wordmark](wordmark.md) needs one.

Apply this format to progress updates, questions/pickers expressed as text, help, settings, setup, status, recovery notices, and completion reports. Use one frame for a project summary table; do not frame each table row. If relaying individual team messages, give each message its own frame and identity. Do not add nested frames inside a project report. Native selection widgets and tool calls keep the host's required format; frame their accompanying text rather than altering their payload.

For a user-invoked help, start, resume, settings, setup, or status, put the wordmark inside the first frame, before its identity and body. Place the creator credit directly below the wordmark, adding the GitHub source beside it for help and setup as specified in [wordmark](wordmark.md). Show it once per invocation, not once per progress update, team, continuous refill, or batch. Team messages use their identity header and borders without the project wordmark. Formatting alone never triggers a state change, agent request, or shell command.
