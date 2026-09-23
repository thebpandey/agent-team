# Claude Code adapter

Use only in Claude Code. Invoke `/agent-team`; resolve dependencies through the actual Skill tool and installed names, including plugin namespaces. Keep the root skill in the main conversation; do not add `context: fork` or require Codex controls.

Confirm Claude Code from trusted runtime/tool metadata. Select its saved role profile through [settings](SETTINGS.md), preserving Codex routing and all run defaults. A host switch does not delete model/effort preferences. Diagnose ambiguous or malformed legacy data without silently resetting custom values. Do not infer the host from `/agent-team` text alone.

When this Claude Code session continues a project last used by Codex or another session, apply [session continuity](ACTIONS.md#continue-in-another-host-or-session): verify the same Git project and continue. No prior-session release, takeover request, or owner migration is needed; model fallback remains a separate choice.

## Role map

These are optional tier recommendations, subject to actual host support. Saved profiles and [settings](SETTINGS.md) defaults govern native dispatch; these suggestions do not switch the current parent.

| Role | Anthropic model ID | Effort |
| --- | --- | --- |
| Project/team orchestrator: planning, assignment, decisions, supervision, integration and release only | `claude-fable-5-1` | high |
| Complex developer: Sol-level work | `claude-opus-5-5` | xhigh |
| Standard developer: Terra-level work | `claude-opus-5-5` | high |
| Independent reviewer: Terra-level work | `claude-opus-5-5` | high |
| Pro visual reviewer | `claude-opus-5-5` | high |
| Routine developer: Luna-level work only | `claude-sonnet-5` | high |
| Optional text assistant: simple rewrite/paraphrase only | `claude-haiku-4-5-20251001` | omit effort override |
| Delegated verifier: pre-dispatch code searches/feature checks and post-completion final checks, reviews and verification | `gpt-5.6-sol` through the installed Codex plugin | medium; fallback `claude-opus-5` high |

Use `xhigh` for the user's “extra effort” setting. Sonnet is reserved for Luna-level assignments and must not substitute for Terra- or Sol-level work. If a required model or effort cannot be enforced, report the constraint instead of silently lowering the tier.

Haiku is never a fallback for development, source discovery, debugging, tests, review, UI decisions, task planning, or releases. Even tiny text edits go to a developer or the text assistant; the orchestrator does not edit files itself. If Sonnet is unavailable, use a suitable available higher-tier developer or report the constraint; never downgrade that work to Haiku.

Fable is preferred for long, demanding orchestration; Opus handles standard development/review at high effort and complex development at xhigh effort. See [Fable](https://platform.claude.com/docs/en/models/fable-5-1/overview), [Opus](https://platform.claude.com/docs/en/models/opus-5/overview), [Sonnet](https://platform.claude.com/docs/en/models/sonnet-5/overview), and [Haiku](https://platform.claude.com/docs/en/models/haiku-4-5/overview).

## Configure the actual runtime

For the direct Anthropic provider, launch:

```bash
claude --model claude-fable-5-1 --effort high
```

Fable 5.1 needs Claude Code v2.1.255+. Confirm installed version and account/provider access. If Fable is unavailable or its usage-credit choice is declined, disclose the fallback to Opus 5 high; the user/runtime must actually select it. Never claim the skill changed its own parent. Do not buy access or bypass a consent prompt. Third-party provider identifiers and aliases differ; resolve supported equivalents before dispatch and record the actual model. See [model configuration](https://code.claude.com/docs/en/model-config).

## Delegated verification

The orchestrator never performs code searches, feature checks, reviews, visual reviews or final verification itself, whether before dispatching a team or after a team reports completion. Delegate that work to the verifier route:

1. Native route: use the saved Claude reviewer or visual reviewer profile through the actual Agent tool. The current missing-role default is `claude-opus-5-5`; validate availability instead of assuming the identifier is supported.
2. Optional cross-host route: use an explicitly approved Codex plugin route such as `gpt-6-sol` at `medium` effort only when installed and supported. Keep it read-only, verify the actual returned route, and return findings to the owning developer. Plugin presence alone does not authorize substitution for the saved profile.

If Agent accepts only aliases such as `opus`, resolve the saved full ID to an alias only when actual host model metadata proves it selects the same model. Otherwise report the affected blocker; never infer equivalence from the alias name or silently change models.

Access is proven by an actual invocation, not by the plugin's presence. The plugin's own result-handling rule to stop and ask before fixing does not apply inside an Agent-Team run: findings return to the owning developer for automatic in-scope repair, and the orchestrator integrates only after the verifier accepts the repaired revision. Verifier output is evidence linked from the task packet, never pasted wholesale into the orchestrator context.

For substantial UI work, the Pro-only `agent-team-visual-tester` definition is available. The existing reviewer owns smaller visual tasks. Apply the shared visual-review procedure to either assignment. Never include this template in Lite.

## Native dispatch

Bundled definitions are in `assets/claude-agents/` relative to the skill root. During setup, copy missing definitions to `.claude/agents/` in the project, or `~/.claude/agents/` for selected user scope. For updates, compare installed definitions with the bundled versions. Refresh unchanged harness-managed definitions within the established installation scope, preserving any user modifications; inspect conflicts before changing them. Do not leave old model/effort settings active while reporting the new routing installed. Record paths in the setup receipt. Verify registration; use a supported refresh or new session when required. Templates have explicit models, supported effort, and disabled child spawning. Do not preload unavailable dependencies.

Dispatch using Claude's native Agent tool and registered role names. Supply the compact team contract, applicable skill paths, ownership, tracker and context location. Each fresh child completes [selective startup](DEPENDENCIES.md#skill-startup-for-every-agent) through the actual Skill tool or complete applicable file reads. A parent's receipt or role registration is not a child read. Verify effective model/effort, including provider limits and substitutions; never silently route coding/review to Haiku. Use supported per-call settings or prepared role definitions only when enforceable, otherwise use the approved suitable fallback or report the affected limitation. See [subagent configuration](https://code.claude.com/docs/en/sub-agents).

Feed observed catalog and fresh-worker results into the [native setup binding](SETUP.md#bind-the-native-observations) before claiming scoped readiness or saving a role change. A generated driver transfers inspected facts; it does not grant native trust or substitute for the actual dispatch.

The executable hook cannot automatically enumerate the complete MCP/plugin inventory. A reviewed manual context-reduction report remains `owner_reported_unverified`, requires explicit unknown-visibility acknowledgement and native owner identity to apply, and cannot relabel a canonically selected dependency as unused. For apply, revert, and recovery, the native adapter derives the trusted Claude home from its own process context and ignores request-supplied home paths. Parent-model comparison stays unknown unless the adapter exposes trustworthy comparable metadata.

Use Graphify only through its CLI inside the worktree, as described in [the code-only integration](GRAPHIFY.md); never run `graphify install`, `graphify claude install` or its hook installers, which write host instruction files and PreToolUse hooks, and register `graphify-mcp` only on explicit selection.

For LeanCTX, use only the [narrowed profile](LEAN-CTX.md) and selected-scope adapter, not a broad initializer. Preserve MCP servers, permissions, hook handlers and customized roles. Do not import full-catalog `autoApprove` or `permissions.allow` lists. Tool visibility is not an authorization boundary. Keep native read-before-write and exact-output recovery; disable coordination/memory, model steering and proxying. Verify the actual worker access path without rewriting unrelated host settings.

Create persistent implementation worktrees under project-orchestrator ownership and pass their absolute paths. Retain required previews/evidence, but safe cleanup after verified integration does not wait for deployment. Reviewers use a stable checkout and write only assigned evidence. The project orchestrator owns native dispatch; logical teams do not introduce nested spawning, an experimental agent-team mode or a background monitor.

Install the root skill under the host's skill directory; the native role templates are supporting assets, not additional skills. See [Claude skill discovery](https://code.claude.com/docs/en/skills).

## Lifecycle hooks

The current edition can register the shared [lifecycle hook system](HOOKS.md) in the `hooks` object of `~/.claude/settings.json` for user scope or `<project>/.claude/settings.local.json` for project scope. The corresponding skill is under `~/.claude/skills/agent-team` or `<project>/.claude/skills/agent-team`; use the selected installation's receipt and exact applicable instruction paths. The managed installer also provisions unchanged current role definitions under that scope's `.claude/agents` and reports customized-file conflicts. Claude uses `PostToolBatch` once for changed-file lint and factual checkpoint work after parallel tool calls; it does not also register the per-edit hook. Local activation logging covers `PreToolUse` for model-invoked `Skill` use and `UserPromptExpansion` for a direct slash command. It does not replace the separate OpenTelemetry `skill_activated` signal.

LeanCTX and Agent-Team setup must merge their hook arrays, settings objects, and permissions additively. Back up and compare the files before and after each operation. Verify both health checks, MCP registration, LeanCTX's exact `autoApprove` and `permissions.allow` changes, every Agent-Team event group, and customized role definitions before reporting success.

## Named teams and lifecycle actions

Use shared [runs](RUNS.md), [settings](SETTINGS.md), and [help](HELP.md) for counted starts (1–6 teams), continuous refill, project defaults, and task-based auto-deploy. The active native project orchestrator schedules and releases through versioned locks and exact evidence. Measure actual host capacity for complete developer/reviewer assignments; do not map six teams to six tool slots or change model tiers to fit. Preserve the shared-orchestrator disclosure when full independent sessions are unavailable. Use compact role-first output for ordinary actions; reserve optional branding for first setup/help. No banner tool or background scheduler is required.

Use shared [session actions](ACTIONS.md), [project coordination](PROJECTS.md), [recovery](RECOVERY.md), [status](STATUS.md), and [preview approval](PREVIEW.md). These are skill instructions, not added native CLI subcommands. Both project and full team orchestrators use this adapter’s orchestrator tier. A second invocation is not automatically a new full session. Use supported independent sessions, or disclose one parent coordinating named groups when separate orchestrators cannot be created. Do not enable forbidden nested spawning. Keep status read-only and leave other sessions running. Main is for coordination; all feature changes use feature worktrees and serial integration uses the project-owned integration worktree.

Use Claude Code native Agent messaging/wait controls to implement the single [active host-turn loop](RUNS.md#eligibility-and-capacity). Reconcile actual worker state, treat unknown liveness as occupied, reserve reviewer capacity, and supervise with the effective `supervision.heartbeatSeconds`, default 600 and minimum 60. Use shorter host waits without treating each return as a heartbeat deadline. At the configured interval, emit one known-state heartbeat and reconcile again. A lower interval consumes more orchestrator turns. Do not launch the Codex verifier merely for heartbeat content. No cron, daemon, timer, hosted monitor, background/nested scheduler, or activity after final response/host interruption is permitted. Heartbeat model/output cost is ordinary usage and its exact incremental amount is unknown.
