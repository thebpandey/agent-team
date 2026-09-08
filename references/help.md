# Agent-Team command help

## Agent roles

Show each available role, its purpose and effective model/effort source. Optional friendly names may follow: Morpheus (orchestrator), Neo (complex developer), Trinity (developer), Tank (routine developer), Agent Smith (reviewer), and The Oracle (visual reviewer). Names are labels, not extra mandatory agents.

For help, show installed version and creator/source credit; the [wordmark](wordmark.md) is optional when space permits. Use the current host's prefix: `$agent-team` in Codex or `/agent-team` in Claude Code. Plain-language “agent-team …” is also accepted. These are skill instructions, not installed shell commands. Help does not run setup, model checks, agents, task writes or release actions. During active work, answer briefly and continue; a standalone help request ends after the answer.

| Command after the prefix | What it does | Example |
| --- | --- | --- |
| `help` | Show commands and examples. | `$agent-team help` |
| `settings` | Show role/model/effort defaults and targeted choices; full wizard is optional. | `$agent-team settings` |
| `setup` | Prepare missing mandatory and selected default capabilities; show grouped recommended defaults. | `$agent-team setup` |
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
| `pause <name-or-ID>` | Pause only that team; retain its claim and release compute only after a safe stopped-writer checkpoint. | `$agent-team pause TEAM-002` |
| `pause all` | Pause the project run and teams, including continuous refill. | `$agent-team pause all` |
| `pause and deploy` | Pause all agents, then commit and deploy verified finished work that is integrated and not yet deployed. | `$agent-team pause and deploy` |
| `resume` | Offer paused/interrupted teams or a stopped Blocked run and All; wait for selection. | `$agent-team resume` |
| `resume <name-or-ID>` | Recover only that team; do not clear a project-wide hold. | `$agent-team resume TEAM-002` |
| `resume all` | Recover the project run and eligible teams from evidence. | `$agent-team resume all` |
| `approve <name-or-ID>` | Approve only that team's submitted preview version. | `$agent-team approve TEAM-002` |

Explain these rules below the table:

- Explicit command choices apply only to the run. `settings` saves future project defaults. A start uses saved auto-deploy without another confirmation; explicit `auto-deploy` or `no-auto-deploy` overrides it for that run.
- N limits active development capacity. Safely stopped parked/paused tasks retain claims without occupying compute; unknown writers are not free capacity. Host limits and reviewer needs can reduce actual concurrency. Named starts never expand into continuous queue work.
- B counts completed top-level tasks, not commits or subtasks. `start 3 auto-deploy` deploys three tasks together; `start 3 auto-deploy 1` deploys each integrated task. Continuous mode refills independently of batching.
- A final smaller batch deploys when the run's work ends or only blocked work remains, if auto-deploy is enabled and release gates pass. Explicit pause and a deployment-failure hold prevent that flush.
- `pause and deploy` is the explicit exception to the normal pause flush rule: it pauses first, then releases only verified finished, integrated, approved, not-yet-deployed work through the normal gated process. It keeps the project paused afterward.
- Preview approval, verified integration, established target authority, and production checks still apply. The skill does not keep running after the host stops.

Exact rules: [actions](actions.md), [settings](settings.md), [runs](runs.md), [release](release.md).

`auto-agent start` accepts the same options as plain-language `agent-team start`. Ordinary start/resume/settings/status use compact output without branding. Ask for “change the reviewer model” to edit only that role, or “show the full settings wizard” for all choices.
