# Quiescent v8 to v9 cutover

v9 replaces the host skill with a portable skill-only package. It does not migrate project state. The v8 manifest defines uninstall scope: v8 uninstall has no host selector and removes every manifest-listed entry. Cut over in a foreground session only after all listed hosts and their work are reconciled.

## Stop conditions

Inspect the complete v8 manifest first and list every host and managed path it covers. Do not cut over while any worker on any listed host is active, possibly active, or has an uncertain result. Finish or stop each observable worker through its native host, then reconcile its Beads task, Git branch/worktree, and dirty files. If any state or identity is uncertain, stop here and preserve it until the original host evidence resolves it. Do not relaunch it under v9.

Inspect the target host and project first. Preserve `.beads`, Git history and worktrees, and the entire project `.agent-team` tree. v9 leaves old project controller records in place and ignores them, except for its own `SESSION.md` breadcrumb and dashboard. Unknown or unclassified hook entries are a cutover STOP; do not clean or rewrite hooks here. v9 installs no hooks.

## Back up and install

Defaults are `~/.agents/skills/agent-team` for Codex and `~/.claude/skills/agent-team` for Claude; use the actual configured homes when set. Before any v8 uninstall, establish a complete, verified recovery route for every manifest-listed entrypoint and shared binary, contract, and manifest: back up each exact path and record hashes, or retain a checksum-verified v8 package and its reinstall procedure. If any path is unknown or either route is incomplete, stop before uninstall. Keep recovery material until v9 is verified. The v9 installer also backs up an existing target skill root and prints its path.

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

These examples cover skill roots only. For every other path in the v8 manifest, preserve its exact relative path under a separate recovery directory (for example, `cp -a -- SOURCE RECOVERY/PATH` on Unix or `Copy-Item -LiteralPath SOURCE -Destination RECOVERY\PATH -Recurse` on Windows), including the shared binary, contracts, and manifest. Record a source-to-backup path and SHA-256 for every entry before uninstall.

If the v8 managed manifest is present and the complete recovery route is verified, run `agent-teamctl uninstall --json` exactly once, before installing v9. This removes every manifest-listed host entry, including entries for a host you do not plan to retain. Verify the command result and confirm only expected managed files were removed while unowned files and hooks remain intact. If output or retained state is unknown or differs from the manifest, stop and restore the complete v8 installation. Never run v8 uninstall after v9 installation. Leave hooks untouched; any unknown or unclassified entry keeps cutover stopped.

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

Install v9 for every host intended to remain configured. If the v8 manifest covered both Codex and Claude, install both with `both` (and provide both explicit homes if needed); do not leave a former manifest-listed host unintentionally uninstalled. Defaults honor `CODEX_HOME` / `CLAUDE_HOME`, otherwise they use the standard homes above. The shell installer also accepts `--codex-home PATH` and `--claude-home PATH`; PowerShell accepts `-CodexHome PATH` and `-ClaudeHome PATH`.

Open the selected host on a Git project and ask `$agent-team status` (Codex) or `/agent-team status` (Claude). Confirm the v9 skill is loaded and Beads status is readable. Status is read-only; if `.beads` is absent, explicitly approve setup before asking for `$agent-team setup` or `/agent-team setup`.

## Rollback

If v9 installation fails after v8 uninstall, restore the complete v8 installation across every host and path in the manifest using the verified route: restore each exact backed-up entrypoint and shared file, including the manifest, or reinstall from the checksum-verified v8 package using its retained procedure. Do not run v8 uninstall again. If a v9 installer reports a backup path, retain and inspect it before manual recovery.

If v9 installed but fails verification, remove only the exact v9 `skills/agent-team` roots installed in this cutover, then restore the complete old installation across every manifest-listed host and path using the same verified route. On Windows use `Remove-Item -LiteralPath` and `Copy-Item -LiteralPath`; on Unix use `rm -r --` and `cp -a` with explicit paths. Never target a host home, `skills/`, or another parent directory. Preserve the v9 roots and installer output until recovery is confirmed. Do not run v8 uninstall after installing v9.

Rollback restores the complete v8 installation across all hosts and paths listed in the original manifest. It must not alter project Beads, Git, `.agent-team` data, Project Kickoff, or unrelated hooks. If any target is ambiguous or contains unexpected data, stop and preserve both copies.
