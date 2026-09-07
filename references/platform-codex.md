# Codex adapter

Use only in a Codex runtime. Invoke `$agent-team`; resolve dependency skills with `$name` or the host's supported skill selector. Keep ChatGPT-managed installation separate from CLI installation.

Confirm Codex from trusted runtime/tool metadata before loading this adapter. Reconcile the saved harness through [settings](settings.md): when the saved harness is `claude-code`, record Codex, remove project role routing (`role_routing`), and use the Codex defaults below while preserving `run_defaults`. If it is unknown or malformed, use the settings repair path and preserve routing until the user selects that repair. Do not infer Codex from `$agent-team` text alone.

| Role | Model | Effort |
| --- | --- | --- |
| Project/team orchestrator; trivial work in feature worktree | `gpt-6-astra` | high |
| Standard developer; code reviewer | `gpt-5.6-terra` | medium or high |
| Pro visual reviewer | `gpt-5.6-terra` | high |
| Complex developer | `gpt-5.6-sol` | high; xhigh for a concrete need |
| Routine developer | `gpt-5.6-luna` | low or medium |

Verify the parent is Astra using exposed runtime metadata. If unavailable or mismatched, report the configuration blocker before implementation; do not silently substitute. Raise effort only for a concrete difficult decision.

Use exposed Codex spawning, messaging, resume, and stop tools, not assumed shell commands. Pass supported model/effort settings explicitly. Where controls match `spawn_agent`, use `fork_turns="none"` or supported limited context for model overrides; send the compact dispatch contract and relevant evidence. Adapt to the actual schema rather than assuming identical APIs on every surface. Reuse an appropriate idle agent instead of duplicating it. Only the orchestrator dispatches.

Before task work or dispatch, both orchestrators and every child complete [skill startup](dependencies.md#skill-startup-for-every-agent). Send the procedure, [LeanCTX contract](lean-ctx.md), and all four resolved skill paths in each spawn/replacement assignment. When no Skill tool is exposed, complete file/resource reads load the instructions; `$name` in a message alone does not. Check each agent's skill receipt before accepting its work.

For approved user-scope LeanCTX setup, merge the shared conservative profile first, then run `lean-ctx init --agent codex --mode hybrid`. It registers MCP in `~/.codex/config.toml`, hooks in `~/.codex/hooks.json`, LeanCTX-owned guidance in `~/.codex/instructions.md` and `~/.codex/LEAN-CTX.md`, and the skill at `~/.codex/skills/lean-ctx/SKILL.md`. With dedicated rules it does not add a shared `AGENTS.md` block. Preserve existing MCP servers, hook groups, and Agent-Team configuration. The LeanCTX shell hook can be unavailable inside a Codex sandbox; confirm it before relying on normal wrapped commands and use `ctx_shell` when it is not active. Keep `shadow_mode = false`, the standard LeanCTX tool profile plus disabled coordination/memory tools, and Agent-Team's existing worktree, tracker, context, and recovery rules.

Apply shared worktree ownership, CONTEXT.md, tracking, verification, release, and cleanup rules unchanged. A missing teammate model is a reported routing constraint: reassign only to an available suitable model under the user's policy, never label a substitute as the requested model.

## Lifecycle hooks

The current edition can register the shared [lifecycle hook system](hooks.md) in `~/.codex/hooks.json`. The authoritative Codex skill path is `~/.agents/skills/agent-team`. Codex has no reliable local skill-activation event, so health reports that dimension as unsupported. Do not infer activation from reading `SKILL.md`. Use the native `/hooks` trust flow when Codex requires it; the installer never fabricates trust.

LeanCTX initialization and Agent-Team hook installation are separate additive operations. Snapshot both configurations first. After either operation, verify every prior hook group and MCP server still exists and run both health checks. Neither installer may replace the other's arrays or settings objects.

## Named teams and lifecycle actions

Use shared [runs](runs.md), [settings](settings.md), and [help](help.md) for counted starts (1–6 teams), continuous refill, project defaults, and task-based auto-deploy. The project owner alone schedules and releases. Measure actual host capacity for complete developer/reviewer assignments; do not map six teams to six tool slots or change model tiers to fit. Preserve the shared-orchestrator disclosure when full independent sessions are unavailable. Display the [wordmark](wordmark.md) once for user help, start, resume, settings, setup, and status commands; no shell banner tool or background scheduler is required.

Use shared [session actions](actions.md), [project coordination](projects.md), [recovery](recovery.md), [status](status.md), and [preview approval](preview.md). These are skill instructions, not added native CLI subcommands. Both project and full team orchestrators use this adapter’s orchestrator tier. A second invocation is not automatically a new full session. Use supported independent sessions, or disclose one parent coordinating named groups when separate orchestrators cannot be created. Do not enable forbidden nested spawning. Keep status read-only and leave other sessions running. Main is for coordination; all feature changes use feature worktrees and serial integration uses the project-owned integration worktree.
