# Codex adapter

Use only in a Codex runtime. Invoke `$agent-team`; resolve dependency skills with `$name` or the host's supported skill selector. Keep ChatGPT-managed installation separate from CLI installation.

Confirm Codex from trusted runtime/tool metadata. Select its saved role profile using [settings](settings.md), preserving Claude Code routing and all run defaults. A host switch must not delete custom model/effort choices. Diagnose malformed/ambiguous legacy routing without silently resetting it. Do not infer the host from `$agent-team` text alone.

| Role | Model | Effort |
| --- | --- | --- |
| Project/team orchestrator; trivial work in feature worktree | `gpt-6-astra` | high |
| Standard developer; code reviewer | `gpt-5.6-terra` | medium or high |
| Pro visual reviewer | `gpt-5.6-terra` | high |
| Complex developer | `gpt-5.6-sol` | high; xhigh for a concrete need |
| Routine developer | `gpt-5.6-luna` | low or medium |

These are quality-first routing defaults, not proof of account availability. Show actual parent and child model/effort from exposed controls; a skill cannot change its parent. Use the user's approved available routing/fallback, preserving quality and reporting configured versus enforceable settings. Do not stall unrelated work solely because metadata is unavailable or silently substitute a weaker model.

For Codex CLI sessions, launch each developer with `-C` set to its registered feature worktree. A shell tool's `workdir` and absolute edit paths do not change the native session's working directory; the ownership guard checks that session location as well as affected paths. Arrange canonical shared-tool and skill reads through the already-authorized narrow scope before dispatch. If access is denied, report it and stop the affected operation.

Use exposed Codex spawning, messaging, resume, and stop tools, not assumed shell commands. Pass supported model/effort settings explicitly. Where controls match `spawn_agent`, use `fork_turns="none"` or supported limited context for model overrides; send the compact dispatch contract and relevant evidence. Adapt to the actual schema rather than assuming identical APIs on every surface. Reuse an appropriate idle agent instead of duplicating it. Only the orchestrator dispatches.

Feed observed catalog and fresh-worker results into the [native setup binding](setup.md#bind-the-native-observations) before claiming scoped readiness or saving a role change. A generated driver transfers inspected facts; it does not grant native trust or substitute for the actual dispatch.

Before work, each fresh child completes [selective skill startup](dependencies.md#skill-startup-for-every-agent). Send only the applicable instruction paths and capability evidence. With no Skill tool, complete file/resource reads load instructions; a name in a message or parent receipt does not. Reuse valid retained context and report exceptions compactly.

For LeanCTX, use only the [narrowed profile](lean-ctx.md) and selected-scope adapter. Do not automatically invoke broad initializers or configure the other host. Preserve existing MCP servers, hooks, instructions, permissions and Agent-Team configuration. Verify the actual available access path: a shell hook may be unavailable in a sandbox, while a permitted `ctx_shell` can remain usable. Keep native exact recovery, disable coordination/memory/model steering, and never bypass a denied command through another wrapper.

Apply shared worktree ownership, CONTEXT.md, tracking, verification, release, and cleanup rules unchanged. A missing teammate model is a reported routing constraint: reassign only to an available suitable model under the user's policy, never label a substitute as the requested model.

## Lifecycle hooks

The current edition can register the shared [lifecycle hook system](hooks.md) in `~/.codex/hooks.json` for user scope or `<project>/.codex/hooks.json` for project scope. The corresponding skill is under `~/.agents/skills/agent-team` or `<project>/.agents/skills/agent-team`; use the selected installation's receipt and pass its exact applicable instruction paths to workers. Codex has no reliable local skill-activation event, so health reports that dimension as unsupported. Do not infer activation from reading `SKILL.md`. Use the native `/hooks` trust flow when Codex requires it; the installer never fabricates trust.

Before dispatch from a linked worktree, inspect the actual native hook sources, trust, and resolved commands there. A project installation in the primary checkout does not prove that a linked native session loads it. Codex runs matching hooks from multiple sources; project hooks do not replace user hooks. A different globally installed Agent-Team version can therefore still affect the session. Report a scope/version conflict and resolve the selected installation within existing authorization before dispatch; do not disable protection or update another scope implicitly. See [Codex hook discovery](https://learn.chatgpt.com/docs/hooks).

LeanCTX initialization and Agent-Team hook installation are separate additive operations. Snapshot both configurations first. After either operation, verify every prior hook group and MCP server still exists and run both health checks. Neither installer may replace the other's arrays or settings objects.

## Named teams and lifecycle actions

Use shared [runs](runs.md), [settings](settings.md), and [help](help.md) for counted starts (1–6 teams), continuous refill, project defaults, and task-based auto-deploy. The project owner alone schedules and releases. Measure actual host capacity for complete developer/reviewer assignments; do not map six teams to six tool slots or change model tiers to fit. Preserve the shared-orchestrator disclosure when full independent sessions are unavailable. Use compact role-first output for ordinary actions; reserve optional branding for first setup/help. No banner tool or background scheduler is required.

Use shared [session actions](actions.md), [project coordination](projects.md), [recovery](recovery.md), [status](status.md), and [preview approval](preview.md). These are skill instructions, not added native CLI subcommands. Both project and full team orchestrators use this adapter’s orchestrator tier. A second invocation is not automatically a new full session. Use supported independent sessions, or disclose one parent coordinating named groups when separate orchestrators cannot be created. Do not enable forbidden nested spawning. Keep status read-only and leave other sessions running. Main is for coordination; all feature changes use feature worktrees and serial integration uses the project-owned integration worktree.
