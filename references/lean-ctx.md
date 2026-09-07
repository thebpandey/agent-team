# LeanCTX integration

LeanCTX is a runtime context tool for compact source discovery, shell-output compression, and lossless recovery of archived output. It is supplemental to Agent-Team. Agent-Team's canonical `CONTEXT.md`, active task tracker, `.agent-team/TEAMS.md`, `MISTAKES.md`, setup receipt, and resumption records remain authoritative.

Do not install RTK, Headroom, or another automatic context compressor around LeanCTX.

## Safe architecture

Use LeanCTX's documented hybrid integration: its Model Context Protocol (MCP) server plus its host hook. This contract was verified against release v3.10.1 and the current upstream default branch; inspect the selected release again before installation. Do not use `wrap`, `onboard`, `setup`, `harden`, or proxy commands for Agent-Team. They have a wider mutation or compression scope than this integration needs.

Resolve LeanCTX's effective global configuration path with `lean-ctx config path`, merge the profile below, validate it, and then initialize each host separately:

```bash
lean-ctx init --agent codex --mode hybrid
lean-ctx init --agent claude --mode hybrid
```

The current Codex initializer registers MCP in `~/.codex/config.toml`, hooks in `~/.codex/hooks.json`, LeanCTX-owned guidance in `~/.codex/instructions.md` and `~/.codex/LEAN-CTX.md`, and the skill at `~/.codex/skills/lean-ctx/SKILL.md`. The current Claude initializer registers MCP plus an `autoApprove` list in `~/.claude.json`, hooks plus `permissions.allow` entries for LeanCTX tools in `~/.claude/settings.json`, and the skill at `~/.claude/skills/lean-ctx/SKILL.md`. Its current approval lists cover the full LeanCTX catalog, not only the selected profile, so treat both files as security-sensitive changes and compare every added permission during review. With `rules_injection = "dedicated"`, neither host needs a LeanCTX block in the user's shared `AGENTS.md` or `CLAUDE.md`; SessionStart provides the compact pointer.

The initializer must merge its MCP entry, instructions, skill, hooks, `autoApprove`, and `permissions.allow` entries into existing user configuration. It must not replace Agent-Team hook arrays, settings objects, permissions, skills, or Claude role definitions. Abort on an unparseable target instead of allowing a default object to replace it. Before any user-level change, inspect the selected upstream release, make a separate timestamped backup of every target, record the source and version, and obtain the user's approval. Upstream also writes a `.bak` beside changed files, but that is not the only backup. Run `lean-ctx config validate`, `lean-ctx doctor`, and `lean-ctx doctor integrations` after initialization. Compare the full Claude approval additions with the selected profile; record that non-advertised tools remain approved by the host but prohibited by this Agent-Team contract. Remove only LeanCTX-owned blocks and entries during rollback; do not remove Agent-Team entries.

LeanCTX currently keys persistent state by project. That does not prove isolation by Git worktree, Agent-Team team ID, and agent/session identity. Use compact reads, search, shell compression, and recoverable local context, but disable LeanCTX task/handoff/coordination and persistent knowledge features for Agent-Team runs until upstream provides and verification proves all four isolation dimensions.

Merge these values into the exact path printed by `lean-ctx config path`; preserve unrelated keys and user changes:

```toml
shell_activation = "agents-only"
shell_hook_mode = "rewrite"
shell_security = "enforce"
shell_allow_writes = false
structure_first = true
read_redirect = "auto"
read_dedup = "auto"
cache_policy = "safe"
delta_explicit = false
tool_profile = "standard"
prefer_native_editor = true
shadow_mode = false
prompt_reinject = "off"
rules_injection = "dedicated"
rules_scope = "global"
proxy_enabled = false
compression_level = "off"
terse_agent = "off"
response_verbosity = "full"
savings_footer = "never"
recovery_hints = "minimal"
tee_mode = "always"
auto_capture = false
enable_wakeup_ctx = false
minimal_overhead = true
behavior_nudges = "off"
bypass_hints = "off"
buddy_enabled = false
journal_enabled = false
default_tool_categories = ["core"]
disabled_tools = [
  "ctx_call",
  "ctx_execute",
  "ctx_session",
  "ctx_knowledge",
  "ctx_agent",
  "ctx_share",
  "ctx_task",
  "ctx_handoff",
  "ctx_workflow",
  "ctx_checkpoint",
  "ctx_compress_memory",
  "shell",
]

[archive]
enabled = true
ephemeral = true

[context]
proactive_expansion = false

[autonomy]
enabled = false

[cross_agent]
auto_extract = false
cross_agent_sync = false

[boundary_policy]
cross_project_import = false
cross_project_search = false
universal_gotchas_enabled = false

[decision_loop]
enabled = false

[graph]
traversal_edges = false

[providers]
enabled = false

[provenance]
enabled = false

[skillify]
enabled = false

[solution]
enabled = false
inject_in_compose = false
inject_in_instructions = false
inject_in_subagents = false
track_decisions = false
track_loc = false

[summaries]
enabled = false

[telemetry]
enabled = false

[updates]
auto_update = false
```

This profile keeps compact discovery, `ctx_shell`, shell-hook shaping, and `ctx_expand`, while removing the broader `ctx_execute` and `shell` execution aliases, the listed LeanCTX memory and coordination tools, and the universal `ctx_call` gateway from the advertised surface. `disabled_tools` controls visibility; it is not a server-side authorization boundary, so the Agent-Team contract also prohibits direct or nested use of those tools. The remaining settings disable decision tracking, edit tools, output-style steering, native-tool denial, and request proxying. `shadow_mode = false` is a code-quality guard: native read/search tools stay available for exact recovery and diagnosis. `tee_mode = "always"` keeps original shell output recoverable for the archive retention period. Do not add a project `.lean-ctx.toml` for Agent-Team. If one already exists, inspect it and leave security-sensitive overrides inactive until the user explicitly trusts the workspace.

## Conservative use

1. Start discovery with `ctx_overview`, `ctx_search`, file maps, signatures, or targeted-line reads. If the host hook is confirmed active, use normal shell commands. Otherwise use `ctx_shell`, or `lean-ctx -c` only where upstream documents that explicit wrapper. Agent-Team's hook normalizes the documented `ctx_shell.command`, `lean-ctx -c`, `lean-ctx exec`, and `lean-ctx raw` paths so its existing critical-operation gates still inspect the inner command. Do not use another LeanCTX execution alias or hide commands in an unsupported nested form.
2. Before editing, retrieve the exact relevant implementation plus nearby callers, interfaces, types, and tests. Never edit from only a map, summary, signature list, compressed diff, or memory entry.
3. Use full/raw source and uncompressed diagnostics for failing tests, production incidents, deployment, migrations, database/schema work, authentication, authorization, payments, security, destructive operations, and unclear errors.
4. When compact context is insufficient, use `ctx_read` with `mode="full"` and `fresh=true`, or the host's native reader. Recover archived tool output with `ctx_expand`. At the CLI, use `lean-ctx raw "command"`, `lean-ctx -c --raw "command"`, or `LEAN_CTX_RAW=1` for one command. Use `LEAN_CTX_DISABLED=1` to bypass LeanCTX for a diagnostic shell run. Do not rely on `ctx_retrieve`; the standard profile does not advertise it.
5. Report meaningful LeanCTX use in the normal progress update and handoff. Examples are the search or compact read that found the source, the full/raw recovery used before an edit, and the compressed command whose original output remains recoverable. Do not create a LeanCTX ledger.

## Status, receipt, and fallback

Resolve LeanCTX's installed skill name and exact `SKILL.md` path or resource identifier. Each Project Orchestrator, Team Orchestrator, developer, reviewer, tester, replacement, and resumed fresh session must load that complete skill in its own context before task work. The dispatch includes the path, binary/version, MCP state, shell-wrapper state, memory-policy state, and health result.

Use one of these receipt statuses: `loaded`, `missing`, `unreadable`, `disabled`, or `not applicable`. `loaded` requires a successful Skill invocation or a complete read through end of file. A present directory, prompt mention, parent receipt, MCP registration, or shell hook does not prove the skill loaded. `not applicable` is for a host where LeanCTX does not support the requested integration, not for ordinary non-UI work.

If LeanCTX is absent, unreadable, disabled, or unhealthy, report the exact status to the Project Orchestrator. Continue with Agent-Team's existing source-reading, `CONTEXT.md`, tracker, team, mistakes, and recovery procedures unless the user made LeanCTX a hard gate. Never report LeanCTX as active or in use without evidence. LeanCTX failure does not change the existing Beads-versus-local tracker selection rule.

## Verification

After an approved user-level setup, verify all of the following without exposing secrets:

- `lean-ctx --version`, executable path, `lean-ctx config path`, effective profile, installed skill path, and `lean-ctx doctor integrations`.
- Codex MCP registration, Codex initialization instructions, and coexistence of every pre-existing Agent-Team hook group.
- Claude Code MCP registration, Claude initialization instructions and skill, its exact `autoApprove` and `permissions.allow` additions, and coexistence of every Agent-Team hook group and customized role definition.
- A compact source read/search followed by exact full-source recovery.
- One verbose shell command compressed through the active wrapper followed by original-output expansion.
- A Project Orchestrator, Team Orchestrator, developer, and reviewer dispatch that each returns its own LeanCTX receipt and meaningful-use evidence.

Record unavailable checks as unavailable. An installed binary, a healthy MCP entry, and a loaded skill are separate facts.
