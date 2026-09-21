---
name: agent-team
description: Use when a Codex request involves Agent-Team setup, status, or starting coordinated development work.
metadata:
  version: "8.0.6"
---

# Agent-Team for Codex

Route `setup`, `status`, and `start` through the installed native `agent-teamctl` contract. Keep task authority in Beads and preserve native fallbacks.

For the status banner, use this installed native entrypoint's `metadata.version` or the matching native binary version. Do not use the separate legacy v7.3.1 repository-root skill version.
