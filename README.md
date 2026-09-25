# Agent-Team

Agent-Team v9 is a Beads and Git skill for coordinating bounded work in an active Codex or Claude session.

Choose your OS bundle from the [v9.0.0 release](https://github.com/thebpandey/agent-team/releases/tag/v9.0.0), verify its matching SHA-256 sidecar, and run the included installer. Project Kickoff v0.6.0 is optional.

After installing, open a Git project and ask `$agent-team status` then `$agent-team start` in Codex, or `/agent-team status` then `/agent-team start` in Claude.

## Install v9.0.0

Choose the ZIP for your operating system and download its matching `.sha256` file from the [v9.0.0 release](https://github.com/thebpandey/agent-team/releases/tag/v9.0.0). Each ZIP contains the same skill and only the matching installer; `any` means it has no CPU-specific executable.

| Operating system | ZIP | Checksum |
| --- | --- | --- |
| Linux | [agent-team-skill-9.0.0-linux-any.zip](https://github.com/thebpandey/agent-team/releases/download/v9.0.0/agent-team-skill-9.0.0-linux-any.zip) | [SHA-256](https://github.com/thebpandey/agent-team/releases/download/v9.0.0/agent-team-skill-9.0.0-linux-any.zip.sha256) |
| macOS | [agent-team-skill-9.0.0-macos-any.zip](https://github.com/thebpandey/agent-team/releases/download/v9.0.0/agent-team-skill-9.0.0-macos-any.zip) | [SHA-256](https://github.com/thebpandey/agent-team/releases/download/v9.0.0/agent-team-skill-9.0.0-macos-any.zip.sha256) |
| Windows | [agent-team-skill-9.0.0-windows-any.zip](https://github.com/thebpandey/agent-team/releases/download/v9.0.0/agent-team-skill-9.0.0-windows-any.zip) | [SHA-256](https://github.com/thebpandey/agent-team/releases/download/v9.0.0/agent-team-skill-9.0.0-windows-any.zip.sha256) |

On Linux, verify and install the extracted bundle:

```sh
sha256sum -c agent-team-skill-9.0.0-linux-any.zip.sha256
unzip agent-team-skill-9.0.0-linux-any.zip
./install.sh both  # or codex or claude
```

On macOS, verify with `shasum -a 256 -c agent-team-skill-9.0.0-macos-any.zip.sha256`, extract that ZIP, then run `./install.sh both` from the extracted folder (or use `codex` or `claude`).

On Windows, verify the ZIP against the digest in its `.sha256` file, extract it, then run in PowerShell:

```powershell
$expected = (Get-Content .\agent-team-skill-9.0.0-windows-any.zip.sha256).Split()[0]
if ((Get-FileHash .\agent-team-skill-9.0.0-windows-any.zip -Algorithm SHA256).Hash -ne $expected) { throw "Checksum mismatch" }
Expand-Archive .\agent-team-skill-9.0.0-windows-any.zip .\agent-team-v9
Set-Location .\agent-team-v9
.\install.ps1 -TargetHost both  # or codex or claude
```

The default skill locations are `~/.agents/skills/agent-team` for Codex and `~/.claude/skills/agent-team` for Claude. See the [v9 install guide](v9/README.md) and [v8 cutover guide](v9/CUTOVER.md) before replacing an existing install.

## First use

In a Git project, ask `$agent-team status` in Codex or `/agent-team status` in Claude. If Beads is not initialized, ask for `setup`; it requests your approval before initializing Beads. Then use `$agent-team start` or `/agent-team start` to choose ready work. A one-off request can also become a bounded Beads task.

Project Kickoff is optional. Use it only when you want help planning a project or explicitly adopting a handoff; an ordinary Git-and-Beads project is ready without it. See the separate [Project Kickoff v0.6.0 release](https://github.com/thebpandey/project-kickoff/releases/tag/v0.6.0).

Use `pause` to stop new assignments and ask observable workers to checkpoint. On a later `resume`, Agent-Team inspects Beads and Git and preserves dirty or uncertain work. Work does not persist as a running team after the host session ends. After accepted integration and Beads closure, the `.agent-team/dashboard/index.html` file can be refreshed as a local status snapshot; it is not a service or task authority.

The Linux Codex worker/reviewer canary passed. Claude status was observed; Claude worker dispatch has not been verified. Windows and macOS worker canaries have not been run. Installer checks are not host worker acceptance.

## Historical releases

[v8.0.15](https://github.com/thebpandey/agent-team/releases/tag/v8.0.15) is the previous controller release; use its [readiness record](docs/releases/8.0.15-readiness.md) and [recovery guide](references/RECOVERY.md) for historical context. For a v8-to-v9 transition, follow [CUTOVER.md](v9/CUTOVER.md). [v7.3.1](https://github.com/thebpandey/agent-team/releases/tag/v7.3.1) is the historical Node hook package.
