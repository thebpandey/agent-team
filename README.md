# Agent-Team

Agent-Team v9 is a portable skill for active Codex and Claude sessions. It coordinates bounded work using Beads for task state and Git for revisions. It installs no controller, daemon, runtime binary, or hooks; workers do not persist across host sessions.

## Release status

v9 is a release candidate; no public v9 bundle is published yet. Native host canaries remain a release gate, so installer CI is not proof of host acceptance. No native host acceptance is claimed. These install steps apply once an approved, checksum-verified bundle is available; see [canary status](v9/tests/CANARIES.md).

## Install

Download `agent-team-skill-9.0.0.zip` and its `SHA256SUMS` from the approved release. Verify the checksum before extracting. The portable bundle contains the same skill for both hosts.

```sh
sha256sum -c SHA256SUMS
unzip agent-team-skill-9.0.0.zip -d agent-team-v9
cd agent-team-v9
./install.sh codex   # or claude or both
```

On Windows PowerShell, compare `Get-FileHash .\agent-team-skill-9.0.0.zip -Algorithm SHA256` with its entry in `SHA256SUMS`, then extract and run:

```powershell
Expand-Archive .\agent-team-skill-9.0.0.zip .\agent-team-v9
Set-Location .\agent-team-v9
.\install.ps1 -TargetHost codex   # or claude or both
```

Defaults are `~/.agents/skills/agent-team` for Codex and `~/.claude/skills/agent-team` for Claude. Custom homes are supported. The installer prints the installed root and any recoverable backup. See [the v9 install guide](v9/README.md) and [v8 cutover and rollback guide](v9/CUTOVER.md).

## First use

Open a Git project and ask `$agent-team status` in Codex or `/agent-team status` in Claude. Status never initializes Beads or creates Agent-Team task/project state; Beads may perform internal housekeeping on first read. If `.beads` is absent, ask for `setup`; it requests explicit approval before running `bd init --skip-hooks --skip-agents --non-interactive --init-if-missing`. Declining leaves the project unchanged.

After Beads is ready, use `$agent-team start` in Codex or `/agent-team start` in Claude for ready work, or request a bounded one-off directly. For example: “Use Agent-Team for a one-off audit of the login flow; report findings without changing files.” One-off work becomes a Beads task and follows the same review and integration rules.

An ordinary Git-and-Beads project needs no Project Kickoff, `TASKS.md`, or optional tools. Import a Project Kickoff handoff or task list only when you ask for that one-time adoption. v9 uses Ponytail by default: prefer the smallest complete change. At first use, choose only model and effort options the host actually offers; unavailable or undecided roles remain `inherit`, and choices can be overridden per task.

Serena, Graphify, Playwright, and visual aids are optional and task-triggered. Use them only when they help that task; native tools remain the fallback. Agent-Team does not probe, install, or invoke LeanCTX.

Ask `$agent-team pause` in Codex or `/agent-team pause` in Claude to stop new assignments and request observable workers to checkpoint and stop. Use `$agent-team resume` or `/agent-team resume` to inspect Beads and Git and preserve dirty or uncertain work; v9 does not promise cross-session worker recovery.

After accepted integration and Beads closure, `.agent-team/dashboard/index.html` may be refreshed as a local static snapshot. It is not a service or task authority; refresh errors do not change accepted work.

## Legacy releases

The latest published release remains [v8.0.15](https://github.com/thebpandey/agent-team/releases/tag/v8.0.15); see its [readiness record](docs/releases/8.0.15-readiness.md) and [recovery guide](references/RECOVERY.md). For a v8-to-v9 transition, follow [CUTOVER.md](v9/CUTOVER.md) and do not run v8 uninstall after v9 is installed. [v7.3.1](https://github.com/thebpandey/agent-team/releases/tag/v7.3.1) is the historical Node hook package.
