---
name: agent-team
description: Use when a Codex request involves Agent-Team setup, status, or starting coordinated development work.
metadata:
  version: "8.0.7"
---

# Agent-Team for Codex

Route `setup`, `status`, and `start` through the installed native `agent-teamctl` contract. Keep task authority in Beads and preserve native fallbacks.

For the status banner, use this installed native entrypoint's `metadata.version` or the matching native binary version. The latest-repository root entrypoint uses the same native version; historical v7.3.1 documents are not banner authority.
