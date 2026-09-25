# Quiescent v8 to v9 cutover

v9 replaces the host skill with a portable skill-only package. It does not migrate project state. Cut over one host at a time, in a foreground session, after all v8 work is reconciled.

## Stop conditions

Do not cut over while any v8 worker is active, possibly active, or has an uncertain result. Finish or stop each observable worker through its native host, then reconcile its Beads task, Git branch/worktree, and dirty files. If its state or identity is uncertain, stop here and preserve it until the original host evidence resolves it. Do not relaunch it under v9.

Inspect the target host and project first. Preserve `.beads`, Git history and worktrees, and the entire project `.agent-team` tree. v9 leaves old project controller records in place and ignores them, except for its own `SESSION.md` breadcrumb and dashboard. Do not clean project data or remove hooks broadly. v9 installs no hooks.

## Back up and install

Back up the exact host skill root before any v8 uninstall. Defaults are `~/.agents/skills/agent-team` for Codex and `~/.claude/skills/agent-team` for Claude; use the actual configured home when `CODEX_HOME` or `CLAUDE_HOME` is set. Keep this backup until v9 is verified. The installers also move an existing target root to a unique sibling backup and print its path.

On Linux or macOS, copy only the exact root that will be replaced:

```sh
cp -a "${CODEX_HOME:-$HOME/.agents}/skills/agent-team" "${CODEX_HOME:-$HOME/.agents}/skills/agent-team.v8-backup"
```

For Claude on Linux or macOS:

```sh
cp -a "${CLAUDE_HOME:-$HOME/.claude}/skills/agent-team" "${CLAUDE_HOME:-$HOME/.claude}/skills/agent-team.claude-v8-backup"
```

On Windows, copy the exact root before proceeding:

```powershell
Copy-Item -LiteralPath "$env:USERPROFILE\.agents\skills\agent-team" -Destination "$env:USERPROFILE\.agents\skills\agent-team.v8-backup" -Recurse
```

For Claude on Windows, use `$env:USERPROFILE\.claude\skills\agent-team` and a sibling `agent-team.claude-v8-backup`. For custom homes, use their exact configured paths. Choose a backup name that does not already exist; never merge backups.

If the v8 managed manifest is present, run its supported `agent-teamctl uninstall --json` now, before installing v9. Do not run v8 uninstall after v9 installation. Leave hooks untouched unless a separately identified, exact Agent-Team hook entry must be removed; first back up that exact configuration. Never remove unrelated hooks or edit a whole hook file by pattern.

Run the v9 installer for the selected host. From the unpacked v9 package:

```sh
./install.sh codex
./install.sh claude
```

On Windows PowerShell:

```powershell
.\install.ps1 -TargetHost codex
.\install.ps1 -TargetHost claude
```

Use `both` only when both hosts are intentionally in scope. Defaults honor `CODEX_HOME` / `CLAUDE_HOME`, otherwise they use the standard homes above. The shell installer also accepts `--codex-home PATH` and `--claude-home PATH`; PowerShell accepts `-CodexHome PATH` and `-ClaudeHome PATH`.

Open the selected host on a Git project and ask `$agent-team status` (Codex) or `/agent-team status` (Claude). Confirm the v9 skill is loaded and Beads status is readable. Status is read-only; if `.beads` is absent, explicitly approve setup before asking for `$agent-team setup` or `/agent-team setup`.

## Rollback

If v9 installation fails, restore the exact pre-cutover skill root from the backup made above. Do not run v8 uninstall again. If the v9 installer reports a backup path, it restores the previous root itself on a failed copy; retain the path and inspect it before any manual recovery.

If v9 installed but fails verification, remove only the exact v9 `skills/agent-team` directory for the selected host, then copy the exact saved skill root back to that same path. On Windows use `Remove-Item -LiteralPath` and `Copy-Item -LiteralPath`; on Unix use `rm -r --` and `cp -a` with the explicit skill-root paths. Never target the host home, `skills/`, or another parent directory. Preserve the v9 root and installer output until recovery is confirmed. Do not run v8 uninstall after installing v9.

Rollback changes only the selected host skill root. It must not alter project Beads, Git, `.agent-team` data, Project Kickoff, or unrelated hooks. If any target is ambiguous or contains unexpected data, stop and preserve both copies.
