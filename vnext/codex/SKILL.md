---
name: agent-team
description: Use when a Codex request involves Agent-Team setup, status, or starting coordinated development work.
metadata:
  version: "8.0.8"
---

# Agent-Team for Codex

Route `setup`, `status`, and `start` through the installed native `agent-teamctl` contract. Keep task authority in Beads and preserve native fallbacks.

`agent-teamctl start --json` only reserves a packet and returns
`host_dispatch_required`; it never starts a worker. Read the returned packet,
then call the actual `collaboration.spawn_agent` tool with its bounded task
payload and saved `profile` model/effort. Acknowledge only the exact returned
canonical task name using `agent-teamctl start --action ack ...`; do not invent
a handle or invoke an unproven worker shell command.

For a retained queue handoff, first record completion, a distinct independent
`clean` reviewer identity, and idle evidence through the native start actions.
When `start --action next --team <team> --json` returns
`host_followup_required`, call `collaboration.followup_task` with the same
acknowledged canonical handle and its fresh delta packet, then acknowledge the
new packet using that same host identity. A packet reservation is not a launch.

For the status banner, use this installed native entrypoint's `metadata.version` or the matching native binary version. The latest-repository root entrypoint uses the same native version; historical v7.3.1 documents are not banner authority.
