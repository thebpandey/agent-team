# LeanCTX integration

LeanCTX is a runtime context tool for compact source discovery, shell-output compression, and lossless recovery of archived output. It is supplemental to Agent-Team. Agent-Team's canonical `CONTEXT.md`, active task tracker, `.agent-team/TEAMS.md`, `MISTAKES.md`, setup receipt, and resumption records remain authoritative.

Do not install RTK, Headroom, or another automatic context compressor around LeanCTX.

## Safe architecture

Prepare the pinned executable and complete applicable skill at the selected scope. Reuse a healthy compatible integration. MCP and a host hook are possible access paths, not proof of availability in a worker. Verify the actual path used by the assignment. Inspect the selected release again before installation.

An exact catalog-qualified existing skill without Agent-Team provenance is reused in place as `installed: "reused_unowned"`, `status: "passed"`, `lifecycleOwnership: "unowned"`. Keep its selected path byte-identical: no sidecar, copy, overwrite, rollback record, or uninstall authority. Qualification checks required files and the unique entrypoint; unrelated regular files are allowed, but required-file drift, symlinks/special entries, or ambiguous entrypoints fail closed without mutation. LeanCTX and Graphify have independent prerequisite catalog edges; neither one's failure changes the other's receipt.

The package helper downloads the catalog-pinned native release and requires its exact `SHA256SUMS` entry. It installs under the selected tool root's `lean-ctx-<version>/lean-ctx` (`lean-ctx.exe` on Windows), preserving existing npm shims and other executables. Use that exact path in the owned native registration or scoped worker environment. It also prepares the release's complete embedded host `SKILL.md` under the selected skill root, verifying its pinned Git blob and preserving custom files. The upstream skill's general setup commands remain subordinate to this narrowed profile. No npm lifecycle, process-stopping preinstall or moving-release onboarding runs during this preparation.

Do not invoke broad `init`, `wrap`, `onboard`, `setup`, `harden`, proxy, or full-catalog permission initializers from automatic setup. Register only the narrowed profile's owned entries in the selected host/scope, using a verified adapter. Never configure both hosts merely because both are installed. If that release cannot isolate a required change safely, report the exact manual/security decision and continue independent preparation; do not mark the integration ready.

Inspect the effective configuration with `lean-ctx config path`. Existing Codex/Claude MCP, hook, instruction, skill and role files are user resources. Claude `autoApprove` and `permissions.allow` are security-sensitive, separate from tool visibility. Do not copy an upstream full-catalog approval list or broaden permissions to make verification pass. With `rules_injection = "dedicated"`, keep guidance out of shared project `AGENTS.md`/`CLAUDE.md` unless an existing approved configuration requires it.

Before an authorized change, record the exact source/version and ownership, preserve the previous bytes in a separate recoverable backup, and merge only the intended entries. Fail on malformed or conflicting customization instead of replacing it with defaults. First-use authorization covers approved default preparation, not a new auth/admin/trust decision or an unrelated wider-scope overwrite. Run `lean-ctx config validate`, `lean-ctx doctor`, and applicable integration checks after configuration. Remove only unchanged entries established by the ownership receipt during rollback; preserve Agent-Team and unrelated entries.

LeanCTX currently keys persistent state by project. That does not prove isolation by Git worktree, Agent-Team team ID, and agent/session identity. Use compact reads, search, shell compression, and recoverable local context, but disable LeanCTX task/handoff/coordination and persistent knowledge features for Agent-Team runs until upstream provides and verification proves all four isolation dimensions.

The following is the compatibility profile, not an instruction to overwrite a global config. Apply only through the selected scope's verified adapter; inspect effective precedence and preserve unrelated keys and user changes:

For an owned process-local profile, set **both** `XDG_CONFIG_HOME` and `LEAN_CTX_CONFIG_DIR`, with the latter exactly `<XDG_CONFIG_HOME>/lean-ctx` containing the owned `config.toml`. Likewise pair `XDG_DATA_HOME`/`LEAN_CTX_DATA_DIR`, `XDG_STATE_HOME`/`LEAN_CTX_STATE_DIR` and `XDG_CACHE_HOME`/`LEAN_CTX_CACHE_DIR`; scope `XDG_RUNTIME_DIR` separately. Explicit LeanCTX pins override inherited routing, while matching XDG paths prevent MCP startup from treating an unrelated XDG config as stale. Neither set alone is sufficient. Pass all values through the actual child process and MCP server environments, then check that process's `lean-ctx config path`. A tool wrapper's advertised environment field is not evidence that it forwarded those values.

Before launching a new process-local MCP profile, inspect whether the real user's `~/.lean-ctx/config.toml` exists, without changing it. Pinned 3.10.1 MCP startup unconditionally checks that legacy config and may move or rename it even when a private config is already present. Matching XDG paths do not prevent that separate legacy migration. If it exists or the check is unavailable, hold the new MCP registration and report the exact manual isolation decision; do not change `HOME`, move user files, or claim that environment variables form a filesystem sandbox. A direct functional `read` check does not establish MCP startup safety. Preserve the existing config and applicable backups, verify actual startup behavior, and retain the narrowed tool contract.

```toml
shell_activation = "agents-only"
shell_hook_mode = "rewrite"
# warn: interpreter heredocs (python3 <<EOF) and inline code (node -e, python3 -c) are permitted; unlisted commands and dangerous patterns are logged, not blocked.
shell_security = "warn"
shell_allow_writes = false
# Additive; managed with `lean-ctx allow <cmd>`. cmp is required for byte-exact recovery checks.
shell_allowlist_extra = ["cmp"]
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

`shell_security = "warn"` is deliberate: LeanCTX has no separate switch for its interpreter-eval hard block, and Agent-Team workers legitimately run interpreter heredocs and `node -e`/`python3 -c` one-liners for fixtures and checks. In warn mode the allowlist and dangerous-pattern detector still log every hit, and Agent-Team's own PreToolUse hook remains the enforcement layer for critical operations. `cmp` is added through `shell_allowlist_extra` so byte-exact recovery comparisons are not blocked. A worker still may not alter the allowlist or security mode itself.

This profile keeps compact discovery, `ctx_shell`, shell-hook shaping, and `ctx_expand`, while removing the broader `ctx_execute` and `shell` execution aliases, the listed LeanCTX memory and coordination tools, and the universal `ctx_call` gateway from the advertised surface. `disabled_tools` controls visibility; it is not a server-side authorization boundary, so the Agent-Team contract also prohibits direct or nested use of those tools. The remaining settings disable decision tracking, edit tools, output-style steering, native-tool denial, and request proxying. `shadow_mode = false` is a code-quality guard: native read/search tools stay available for exact recovery and diagnosis. `tee_mode = "always"` keeps original shell output recoverable for the archive retention period. Do not add a project `.lean-ctx.toml` for Agent-Team. If one already exists, inspect it and leave security-sensitive overrides inactive until the user explicitly trusts the workspace.

## Conservative use

1. Start discovery with `ctx_overview`, `ctx_search`, file maps, signatures, or targeted-line reads. If the host hook is confirmed active, use normal shell commands. Otherwise use `ctx_shell`, or `lean-ctx -c` only where upstream documents that explicit wrapper. Agent-Team's hook normalizes the documented `ctx_shell.command`, `lean-ctx -c`, `lean-ctx exec`, and `lean-ctx raw` paths so its existing critical-operation gates still inspect the inner command. Do not use another LeanCTX execution alias or hide commands in an unsupported nested form.
2. Before editing, retrieve the exact relevant implementation plus nearby callers, interfaces, types, and tests. Never edit from only a map, summary, signature list, compressed diff, or memory entry.
3. Use full/raw source and uncompressed diagnostics for failing tests, production incidents, deployment, migrations, database/schema work, authentication, authorization, payments, security, destructive operations, and unclear errors.
4. When compact context is insufficient, recover the already-captured original with `ctx_read` full/fresh, `ctx_expand`, or the permitted native reader. Request uncompressed diagnostics through a supported path when the operation itself is permitted. A command denial is not permission to retry through raw mode, another wrapper, an interpreter, environment overrides or an altered allowlist. Do not rely on unadvertised retrieval tools.
5. Report meaningful LeanCTX use in the normal progress update and handoff. Examples are the search or compact read that found the source, the full/raw recovery used before an edit, and the compressed command whose original output remains recoverable. Do not create a LeanCTX ledger.

## Status, receipt, and fallback

Resolve LeanCTX's installed skill name and exact SKILL.md path for assignments that use its context tools. Each fresh worker using it reads its complete instructions and applicable references in its own context. A text-only assignment with no source/tool work need not load it. Retained workers reuse current reads; the packet points to prepared capability evidence rather than repeating all configuration on every turn.

Use loaded, missing, unreadable, disabled or not applicable honestly. Loaded requires a successful full instruction read/invocation in this context. A folder, mention, parent receipt, MCP registration or hook does not prove loading. Not applicable means this assignment does not use the capability, or the host does not support it; ordinary backend source work can still benefit from it.

If LeanCTX is absent, unreadable, disabled or unhealthy, report the exception to the orchestrator once. Diagnose within authority and use permitted native source tools when possible. Never bypass a denial or claim unavailable capabilities are active. LeanCTX does not select, migrate or replace the canonical tracker.

## Verification

After an approved user-level setup, verify all of the following without exposing secrets:

- `lean-ctx --version`, executable path, `lean-ctx config path`, effective profile, installed skill path, and `lean-ctx doctor integrations`.
- Codex MCP registration, Codex initialization instructions, and coexistence of every pre-existing Agent-Team hook group.
- Claude Code MCP registration, Claude initialization instructions and skill, its exact `autoApprove` and `permissions.allow` additions, and coexistence of every Agent-Team hook group and customized role definition.
- A compact source read/search followed by exact full-source recovery.
- One verbose shell command compressed through the active wrapper followed by original-output expansion.
- Relevant orchestrator/developer/reviewer assignments can access the selected tool profile and recover exact output; fresh users read their own applicable instructions. A role that does not need LeanCTX is not forced through a demonstration.

Record unavailable checks as unavailable. An installed binary, a healthy MCP entry, and a loaded skill are separate facts.
