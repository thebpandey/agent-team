# Codex adapter

Use only in a Codex runtime. Invoke `$agent-team`; resolve dependency skills with `$name` or the host's supported skill selector. Keep ChatGPT-managed installation separate from CLI installation.

| Role | Model | Effort |
| --- | --- | --- |
| Orchestrator; trivial direct work | `gpt-6-astra` | high |
| Standard developer; independent reviewer | `gpt-5.6-terra` | medium or high |
| Complex developer | `gpt-5.6-sol` | high; xhigh for a concrete need |
| Routine developer | `gpt-5.6-luna` | low or medium |

Verify the parent is Astra using exposed runtime metadata. If unavailable or mismatched, report the configuration blocker before implementation; do not silently substitute. Raise effort only for a concrete difficult decision.

Use exposed Codex spawning, messaging, resume, and stop tools, not assumed shell commands. Pass supported model/effort settings explicitly. Where controls match `spawn_agent`, use `fork_turns="none"` or supported limited context for model overrides; send the compact dispatch contract and relevant evidence. Adapt to the actual schema rather than assuming identical APIs on every surface. Reuse an appropriate idle agent instead of duplicating it. Only the orchestrator dispatches.

Apply shared worktree ownership, CONTEXT.md, tracking, verification, release, and cleanup rules unchanged. A missing teammate model is a reported routing constraint: reassign only to an available suitable model under the user's policy, never label a substitute as the requested model.
