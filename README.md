# agent-team

A Claude skill that turns a rough task into one paste-ready prompt for Claude Code or Codex. That prompt fans the work out across async sub-agents in clearly separated phases, then fans the results back into one plain-language report.

It answers a simple question every time: what can run at the same time, and what has to wait?

The prompt has two stages. Stage A defines a crew of named specialists, each with a locked model and effort, and lays out the implementation plan. Stage B is the orchestrator's job: Danny Ocean, pinned to Opus 4.8 or GPT-5.5, reads each phase and routes it to the specialist whose tier fits its difficulty, then fans in one report. Agents are always named. Heist mode is on by default: each agent gets an Ocean's-Eleven persona that colors its logs and report, closed by a plain summary in parentheses. Turn it off with plain mode.

## What it does

Give it a task. It:

1. Splits the task into work units.
2. Works out the dependencies. If one unit needs another unit's output, the second waits.
3. Groups independent units into **parallel** phases and chains dependent units into **sequential** phases. Every phase is tagged so the run order is obvious.
4. Builds an implementation plan: phases tagged parallel or sequential, dependencies, and a difficulty tag per phase.
5. Defines a crew of specialists, each with a locked model and effort (Stage A), but does not pre-assign them to phases.
6. Lets the orchestrator route each phase to the fitting specialist at runtime (Stage B), based on the phase's severity, complexity, and length.
7. Keeps the runtime quiet: no narration, no logs, no over-explaining. Each report section ends with a plain summary in parentheses.

## How the two stages map to the runtimes

The name describes the shape of the work, not a product feature. It does not use Claude Code's experimental Agent Teams feature.

- Stage A writes real named agent files. Claude Code: `.claude/agents/agent-team/<name>.md` with `model` and `effort`. Codex: `.codex/agents/<name>.toml` with `model` and `model_reasoning_effort`. Both runtimes bind model and effort as real fields, so names and tiers actually stick. Earlier versions only labeled agents inline, which is why names did not hold.
- Effort is real on both, and it needs Opus or Sonnet; Haiku has no effort. The crew runs in a medium-to-high effort band.
- Stage A also builds the plan but assigns no agents. Stage B is where the orchestrator routes each phase to the specialist whose tier fits, which is the intelligence you want at runtime, not baked in.
- The orchestrator (Danny) runs on the session model: Opus 4.8 in Claude Code, GPT-5.5 in Codex, at high or xhigh effort. Set it when you start the session.
- Only the orchestrator fans out; agents cannot nest (Claude Code cannot; Codex `agents.max_depth` defaults to 1).
- Large outputs go to a run temp directory and come back as a summary plus file path, so many returns at once cannot overflow the orchestrator. After the report is displayed, the run deletes its temp directory and the agent files it created. Only those, never your existing files or agents.

## Crew tiers (locked model and effort)

Each specialist has a fixed capability profile. The orchestrator routes each phase to the one whose tier fits the difficulty. Effort stays in the medium-to-high band; no Haiku, because Haiku has no effort.

| Crew member | Specialty | Claude Code | Codex |
|-------------|-----------|-------------|-------|
| Danny Ocean | orchestrator, routing | opus (4.8) / xhigh | gpt-5.5 / high |
| Livingston Dell | security | opus / high | flagship / high |
| Rusty Ryan | code audit | opus / high | flagship / high |
| Saul Bloom | legacy | opus / medium | flagship / medium |
| The Malloy Twins | concurrency | opus / high | flagship / high |
| Amazing Yen | optimization | sonnet / high | coding model / high |
| Frank Catton | frontend | sonnet / medium | coding model / medium |
| Basher Tarr | devops | sonnet / medium | coding model / medium |
| Linus Caldwell | data extraction | sonnet / medium | coding model / medium |
| Reuben Tishkoff | budget | sonnet / medium | coding model / medium |

Confirm current model IDs against the runtime docs. Aliases (opus, sonnet) are safer than pinned IDs, except Danny, who is pinned to Opus 4.8 / GPT-5.5.

## Where you can invoke it

The skill is a prompt generator. Generating the delegation prompt works in any chat. Running it (actually spawning parallel agents) needs an agentic runtime.

| Surface | How | Generates prompt | Runs agents |
|---------|-----|:---:|:---:|
| Claude Code | Skill or `/agent-team` command | Yes | Yes |
| Codex | Paste the portable prompt, or the command contract | Yes | Yes |
| Claude Chat (Claude.ai) | Upload as a skill, or paste `portable/agent-team-prompt.md` | Yes | No |
| ChatGPT | Paste `portable/agent-team-prompt.md`, or set it as a Custom GPT system prompt | Yes | No |

So in Claude Chat or ChatGPT you use agent-team to write the delegation prompt, then paste that prompt into Claude Code or Codex to execute it. The portable prompt in `portable/agent-team-prompt.md` is a single self-contained block for the surfaces that do not load skills.

## Install

Three paths. Pick one.

### 1. Claude.ai upload

Zip the folder and upload it as a skill in the Claude.ai skills panel.

### 2. Claude Code (skill), Windows PowerShell

```powershell
git clone https://github.com/thebpandey/agent-team "$env:USERPROFILE\.claude\skills\agent-team"
```

Mac or Linux:

```bash
git clone https://github.com/thebpandey/agent-team ~/.claude/skills/agent-team
```

### 3. Claude Code (slash command)

Copy the command file so `/agent-team` works in any project.

Windows PowerShell:

```powershell
Copy-Item ".\claude-code\agent-team.md" "$env:USERPROFILE\.claude\commands\agent-team.md"
```

Mac or Linux:

```bash
cp ./claude-code/agent-team.md ~/.claude/commands/agent-team.md
```

## Usage

Invoke it explicitly. It never auto-triggers.

```
/agent-team audit my codebase across frontend, api, db, and auth
/agent-team build the user feature: types, model, routes, tests, docs
/agent-team research these 5 competitors, use 3 agents, high effort
```

Or in chat:

```
delegate this across agents: [your task]
multi-agent prompt for [your task]
```

You get back a single prompt block plus a one-line plan. Paste the block into Claude Code and run it.

## Standalone

This skill has no dependency on any other skill and makes no runtime reference to one. It ships everything it needs.

## License

MIT. See [LICENSE](LICENSE).
