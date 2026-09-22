---
name: agent-team
description: Use when a Codex request involves Agent-Team setup, status, or starting coordinated development work.
metadata:
  version: "8.0.9"
---

# Agent-Team for Codex

Route `setup`, `status`, and `start` through the installed native `agent-teamctl` contract. Keep task authority in Beads and preserve native fallbacks.

`agent-teamctl start --json` only reserves a packet; it never starts a worker.
Call `collaboration.spawn_agent` only when the response has
`host_dispatch_required: true` and `already_admitted: false`. If it reports an
already admitted packet or `host_dispatch_required: false`, observe the named
team/packet and do not spawn or acknowledge a replacement worker. For a fresh
packet, use its bounded task payload and saved `profile` model/effort, then
acknowledge only the exact returned canonical task name using
`agent-teamctl start --action ack ...`; do not invent a handle or invoke an
unproven worker shell command.

Use this exact acknowledgement shape, replacing only returned JSON values:

```text
agent-teamctl start --action ack --team <team> --packet-digest <packet_digest> --host codex --identity <spawn_agent_task_name> --task <packet.task> --candidate <packet.specRevision> --json
```

Completion and retained reuse use the same exact handle fields:

```text
agent-teamctl start --action complete --team <team> --packet-digest <packet_digest> --host codex --identity <spawn_agent_task_name> --task <packet.task> --candidate <packet.specRevision> --json
agent-teamctl start --action clean --team <team> --reviewer <independent_reviewer_identity> --json
agent-teamctl start --action idle --team <team> --packet-digest <packet_digest> --host codex --identity <spawn_agent_task_name> --task <packet.task> --candidate <packet.specRevision> --json
agent-teamctl start --action next --team <team> --json
```

For a retained queue handoff, first record completion, a distinct independent
`clean` reviewer identity, and idle evidence through the native start actions.
When `start --action next --team <team> --json` returns
`host_followup_required`, call `collaboration.followup_task` with the same
acknowledged canonical handle and its fresh delta packet, then acknowledge the
new packet using that same host identity. Do not follow up merely because work
was appended: require a fresh native `host_followup_required: true` response
after that idle evidence. A terminal `next` returns
`host_followup_required: false`, consumes the final task, and requires no host
action. If a retained-intent retry returns the same packet with
`already_admitted: true` and `host_followup_required: false`, observe it only:
do not call `followup_task` again or issue a replacement acknowledgement; the
original exact acknowledgement remains the only valid continuation.

`start --run <run> --task <task> --json` normally only appends an explicit
already-snapshotted task. If it appends to a consumed idle retained team, it
instead returns a fresh `host_followup_required` packet; use
`collaboration.followup_task` with the returned retained handle, then
acknowledge that packet with the same identity.

For the status banner, use this installed native entrypoint's `metadata.version` or the matching native binary version. The latest-repository root entrypoint uses the same native version; historical v7.3.1 documents are not banner authority.
