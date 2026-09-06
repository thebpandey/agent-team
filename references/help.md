# Agent-Team command help

For `help`, display the [AGENT-TEAM wordmark](wordmark.md) once inside the first message frame, with the installed skill version, then the creator credit and GitHub source below it, before the identity header and command table. Show the table below with the current host's prefix: `$agent-team` in Codex or `/agent-team` in Claude Code. Plain-language “agent-team …” is also accepted. These are skill instructions, not installed shell commands. Show help without setup, model checks, agents, task writes, or release actions. During active work, answer and continue that work. A standalone help request ends after the answer.

| Command after the prefix | What it does | Example |
| --- | --- | --- |
| `help` | Show commands and examples. | `$agent-team help` |
| `settings` | View or change defaults for this project only. | `$agent-team settings` |
| `setup` | Check dependencies, offer installation choices, and let the user keep or edit project defaults. | `$agent-team setup` |
| `start` | Select existing ready tracker work using project defaults; built-in default is one task. | `$agent-team start` |
| `start N` | Start up to N safe tasks; N is 1–6. Without continuous mode, finish only that set. | `$agent-team start 3` |
| `start [N] continuous` | Refill each slot after verified integration into main. | `$agent-team start 3 continuous` |
| `start <feature-name>` | Start only the already-defined tracker item with this resolved name. | `$agent-team start email-preferences` |
| `start [N or name] with-preview` | Require approval of each admitted feature's submitted version before integration. | `$agent-team start 3 with-preview` |
| `start [N] [continuous] auto-deploy [B]` | Run teams and deploy batches of B completed top-level tasks. Without B, use the effective team limit. | `$agent-team start 3 continuous auto-deploy` |
| `start … no-continuous` | Disable saved continuous mode for this run. | `$agent-team start 3 no-continuous` |
| `start … no-auto-deploy` | Finish integration and ask before deployment. | `$agent-team start 3 no-auto-deploy` |
| `auto-deploy [B]` | Enable current-run deployment batches, including eligible integrated tasks; do not start teams. Without B, deploy one task at a time. | `$agent-team auto-deploy 3` |
| `auto-deploy off` | Stop future automatic batches for the current run. | `$agent-team auto-deploy off` |
| `status [name-or-ID or all]` | Read recorded progress without checks or interruption. | `$agent-team status all` |
| `pause` | Offer eligible teams/run and All; wait for selection. | `$agent-team pause` |
| `pause <name-or-ID>` | Pause only that team; it retains its slot. | `$agent-team pause TEAM-002` |
| `pause all` | Pause the project run and teams, including continuous refill. | `$agent-team pause all` |
| `resume` | Offer paused/interrupted teams or a stopped Blocked run and All; wait for selection. | `$agent-team resume` |
| `resume <name-or-ID>` | Recover only that team; do not clear a project-wide hold. | `$agent-team resume TEAM-002` |
| `resume all` | Recover the project run and eligible teams from evidence. | `$agent-team resume all` |
| `approve <name-or-ID>` | Approve only that team's submitted preview version. | `$agent-team approve TEAM-002` |

Explain these rules below the table:

- Explicit command choices apply only to the run. `settings` saves future project defaults. If saved defaults enable auto-deploy, start asks whether to keep it or use no auto-deploy; explicit `auto-deploy` skips that settings question.
- N limits occupied development teams, including blocked/paused/approval-waiting teams. Host capacity and safe task independence can reduce the actual parallel count. Named starts never expand into continuous queue work.
- B counts completed top-level tasks, not commits or subtasks. `start 3 auto-deploy` deploys three tasks together; `start 3 auto-deploy 1` deploys each integrated task. Continuous mode refills independently of batching.
- A final smaller batch deploys when the run's work ends or only blocked work remains, if auto-deploy is enabled and release gates pass. Explicit pause and a deployment-failure hold prevent that flush.
- Preview approval, verified integration, established target authority, and production checks still apply. The skill does not keep running after the host stops.

Exact rules: [actions](actions.md), [settings](settings.md), [runs](runs.md), [release](release.md).

`auto-agent start` accepts the same options as the plain-language `agent-team start`. Help, start, resume, settings, setup, and status show the [AGENT-TEAM wordmark](wordmark.md) once per user invocation.
