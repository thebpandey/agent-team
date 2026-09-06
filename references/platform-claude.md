# Claude Code adapter

Use only in Claude Code. Invoke `/agent-team`; resolve dependencies through the actual Skill tool and installed names, including plugin namespaces. Keep the root skill in the main conversation; do not add `context: fork` or require Codex controls.

## Role map

These are workflow choices, not benchmark equivalence claims. Verified against Anthropic's model documentation on 2026-09-04.

| Role | Anthropic model ID | Effort |
| --- | --- | --- |
| Project/team orchestrator; trivial work in feature worktree | `claude-fable-5-1` | high |
| Complex developer: Sol-level work | `claude-opus-5` | xhigh |
| Standard developer: Terra-level work | `claude-opus-5` | high |
| Independent reviewer: Terra-level work | `claude-opus-5` | high |
| Pro visual reviewer | `claude-opus-5` | high |
| Routine developer: Luna-level work only | `claude-sonnet-5` | high |
| Optional text assistant: simple rewrite/paraphrase only | `claude-haiku-4-5-20251001` | omit effort override |

Use `xhigh` for the user's “extra effort” setting. Sonnet is reserved for Luna-level assignments and must not substitute for Terra- or Sol-level work. If a required model or effort cannot be enforced, report the constraint instead of silently lowering the tier.

Haiku is never a fallback for development, source discovery, debugging, tests, review, UI decisions, task planning, or releases. Do tiny text edits directly when delegation costs more. If Sonnet is unavailable, use a suitable available higher-tier developer or report the constraint; never downgrade that work to Haiku.

Fable is preferred for long, demanding orchestration; Opus handles standard development/review at high effort and complex development at xhigh effort. See [Fable](https://platform.claude.com/docs/en/models/fable-5-1/overview), [Opus](https://platform.claude.com/docs/en/models/opus-5/overview), [Sonnet](https://platform.claude.com/docs/en/models/sonnet-5/overview), and [Haiku](https://platform.claude.com/docs/en/models/haiku-4-5/overview).

## Configure the actual runtime

For the direct Anthropic provider, launch:

```bash
claude --model claude-fable-5-1 --effort high
```

Fable 5.1 needs Claude Code v2.1.255+. Confirm installed version and account/provider access. If Fable is unavailable or its usage-credit choice is declined, disclose the fallback to Opus 5 high; the user/runtime must actually select it. Never claim the skill changed its own parent. Do not buy access or bypass a consent prompt. Third-party provider identifiers and aliases differ; resolve supported equivalents before dispatch and record the actual model. See [model configuration](https://code.claude.com/docs/en/model-config).

For substantial UI work, the Pro-only `agent-team-visual-tester` definition is available. The existing reviewer owns smaller visual tasks. Apply the shared visual-review procedure to either assignment. Never include this template in Lite.

## Native dispatch

Bundled definitions are in `assets/claude-agents/` relative to the skill root. During setup, copy missing definitions to `.claude/agents/` in the project, or `~/.claude/agents/` for selected user scope. For updates, compare installed definitions with the bundled versions. Refresh unchanged harness-managed definitions within the established installation scope, preserving any user modifications; inspect conflicts before changing them. Do not leave old model/effort settings active while reporting the new routing installed. Record paths in the setup receipt. Verify registration; use a supported refresh or new session when required. Templates have explicit models, supported effort, and disabled child spawning. Do not preload unavailable dependencies.

Dispatch using Claude's native Agent tool and registered role names. Supply the shared team contract, resolved skill paths, ownership, tracker mode, and context location each time. Verify effective model/effort, including environment overrides or provider caps; never select a generic built-in agent that silently routes to Haiku. If definitions are not registered, use supported per-call configuration only when it can enforce the required role; otherwise report the dispatch blocker. See [subagent configuration](https://code.claude.com/docs/en/sub-agents).

Create persistent implementation worktrees under orchestrator ownership and pass their absolute paths. Do not rely on an ephemeral agent checkout surviving return until deployment. The shared release policy controls removal. Reviewers may use a stable checkout and write only their assigned context/evidence. Only full project/team orchestrators spawn their own members; no experimental agent-team mode or background monitor is required.

Install the root skill under the host's skill directory; the native role templates are supporting assets, not additional skills. See [Claude skill discovery](https://code.claude.com/docs/en/skills).

## Named teams and lifecycle actions

Use shared [runs](runs.md), [settings](settings.md), and [help](help.md) for counted starts (1–6 teams), continuous refill, project defaults, and task-based auto-deploy. The project owner alone schedules and releases. Measure actual host capacity for complete developer/reviewer assignments; do not map six teams to six tool slots or change model tiers to fit. Preserve the shared-orchestrator disclosure when full independent sessions are unavailable. Display the [wordmark](wordmark.md) once for user start, resume, settings, setup, and status commands; no shell banner tool or background scheduler is required.

Use shared [session actions](actions.md), [project coordination](projects.md), [recovery](recovery.md), [status](status.md), and [preview approval](preview.md). These are skill instructions, not added native CLI subcommands. Both project and full team orchestrators use this adapter’s orchestrator tier. A second invocation is not automatically a new full session. Use supported independent sessions, or disclose one parent coordinating named groups when separate orchestrators cannot be created. Do not enable forbidden nested spawning. Keep status read-only and leave other sessions running. Main is for coordination; all feature changes use feature worktrees and serial integration uses the project-owned integration worktree.
