# Agent-Team v9

Agent-Team v9 is a host skill for active Codex or Claude sessions. Beads holds live task state and Git holds revisions. v9 installs no controller, daemon, runtime binary, or hooks.

## Install

Unpack the package and run one installer for the intended host. Existing skill roots are moved to recoverable sibling backups; the installer prints each installed root and backup path.

```sh
./install.sh codex       # Linux, macOS, or other POSIX shell
./install.sh claude
./install.sh both        # only when both are intended
```

```powershell
.\install.ps1 -TargetHost codex   # Windows PowerShell
.\install.ps1 -TargetHost claude
.\install.ps1 -TargetHost both
```

Default skill roots are `~/.agents/skills/agent-team` for Codex and `~/.claude/skills/agent-team` for Claude. Existing `CODEX_HOME` or `CLAUDE_HOME` values are honored. Set a custom home with `--codex-home PATH` / `--claude-home PATH` on POSIX, or `-CodexHome PATH` / `-ClaudeHome PATH` in PowerShell. Keep the printed backup until the install is verified. See [CUTOVER.md](CUTOVER.md) before replacing a v8 installation.

## First use

In a Git project with Beads initialized, ask the host `$agent-team status` in Codex or `/agent-team status` in Claude. To begin work, use `$agent-team start` or `/agent-team start`. An ordinary Git-and-Beads project needs no Project Kickoff installation, handoff, `TASKS.md`, or optional aid. If Beads is not initialized, setup asks for approval before creating Beads-managed state.

Ask for bounded one-off work directly, for example: “Use Agent-Team for a one-off audit of the login flow; report findings and do not change files.” A bounded one-off request becomes one Beads task and uses the same review and integration rules as other work.

You can ask `$agent-team pause` or `/agent-team pause` to stop new assignments and request observable workers to checkpoint and stop. On a later `$agent-team resume` or `/agent-team resume`, v9 inspects Beads and Git first and preserves dirty or uncertain work. A session breadcrumb is not proof that a past worker is still available.

At first use, v9 can ask for orchestrator, developer, reviewer, and visual model/effort preferences. Choose only models and effort levels the host actually offers; leave a role as `inherit` when undecided or unavailable. Preferences may be changed for an individual task. The optional Project Kickoff import is offered only when you explicitly request adoption of a handoff or task list.

After accepted integration and normal Beads closure, v9 may refresh `.agent-team/dashboard/index.html`. It is a local static snapshot, not a service or task authority. Refresh errors are reported separately and do not change accepted Beads work.

## Evidence

Run installer and cutover canaries only with disposable fixtures. `v9/tests/CANARIES.md` records the defined cases and distinguishes fixture checks from live-host observations. Linux tests do not establish Windows or Claude success; do not infer native canary results from source inspection.
