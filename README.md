# agent-team

A Claude Code skill that turns one plain task into an orchestrated multi-agent
run. You type the task. The session itself, acting as the orchestrator (FABLE,
persona Danny Ocean), breaks it into tickets, tags phases parallel or sequential,
routes each ticket to the right tier of worker, gates risky work through an
Adversary, executes in parallel where safe, verifies with independent evidence,
and hands you a short report.

v3 is Claude Code only. There is no prompt to copy anywhere. Earlier versions
generated prompts for Claude Chat, ChatGPT, and Codex; that distributed model is
gone. Codex now participates as a detected worker engine, not as a surface.

## How it works

One command, two modes.

`/agent-team init` scaffolds the protocol into your repo: `/specs/Agents.md` (the
canonical roles-and-invariants file, which Codex also reads natively),
`architecture.md` and `design.md` stubs, `/goals/` for tickets, and persistent
crew definitions under `.claude/agents/agent-team/`.

`/agent-team <task>` runs the lifecycle: Definition of Done, decomposition into
goal tickets with dependencies, phase tagging (independent work fans out in
parallel, dependent work waits), tier routing, adversarial gating on risky work,
worktree-isolated execution, independent verification, dependency-ordered merge,
short report.

`/architect <feature>` is planning-only: designs the lanes, drafts the tickets,
pre-clears the risky ones with the Adversary, executes nothing.

## Topology: it detects Codex

At the start of every run the orchestrator probes for Codex, preferring the
official OpenAI plugin (openai/codex-plugin-cc: `/plugin marketplace add
openai/codex-plugin-cc`, then `/plugin install codex@openai-codex`), which gives
delegation via the `codex:codex-rescue` subagent, a steerable
`/codex:adversarial-review`, and background job management (`/codex:status`,
`/codex:result`, `/codex:cancel`). Fallbacks: a codex MCP server, then the bare
CLI (`codex --version`). If any path is found, topology is HYBRID: Codex GPT-5.6 Sol takes the
hardest build work and the Adversary role, and Opus reviews Codex-built code
(different model families miss different things). If not found, ALL-CLAUDE: Opus
absorbs those roles, with an honest one-line note that same-family review is
weaker.

## Tiers

| Tier | Work | ALL-CLAUDE | HYBRID |
|---|---|---|---|
| Orchestrator | judgment: intent, architecture, tradeoffs, review, done | fable, else opus 4.8 | same (session) |
| Adversary | audits plans and risky submissions, veto power, writes no code | opus 4.8 | gpt-5.6-sol |
| T1 | complex implementation, deep debugging, security-sensitive | opus 4.8 | gpt-5.6-sol |
| T2 | schemas, data consistency, concurrency, reviewing cheaper agents | opus 4.8 | gpt-5.6-terra |
| T3 | scoped features, tests, local refactors | sonnet 4.6 | gpt-5.6-terra |
| T4 | discovery, summaries, checks, boilerplate | haiku 4.5 | gpt-5.6-luna / gpt-5.4-mini |

The orchestrator keeps judgment and delegates evidence. It touches `/src` only
under a logged micro-fix exception (10 lines or fewer, one file, never in a
high-risk area), and the Adversary samples that log.

High-risk areas (auth, billing, permissions, security, migrations, data loss,
shared state, caching, concurrency, cross-module behavior, public APIs,
user-visible workflows) always get the Adversary gate and top-tier build or
review. Routine tickets are sampled 1 in 5. Verification is always done by a
different agent than the author.

## The crew

Agents are always named with ASCII prefixes so parallel work reads clearly in the
terminal. Heist mode is on by default: sections read in character and each closes
with a plain summary in parentheses; Next Steps are always plain. Add "plain
mode" to drop the voice. Danny orchestrates; Terry Benedict is the Adversary; the
domain skins (Livingston security, Basher devops, Frank frontend, Malloys
concurrency, Yen perf, Saul legacy, Linus data, Rusty review, Reuben budget)
attach per ticket.

## Install (Claude Code)

Windows PowerShell:

```powershell
git clone https://github.com/thebpandey/agent-team "$env:USERPROFILE\.claude\skills\agent-team"
Copy-Item "$env:USERPROFILE\.claude\skills\agent-team\claude-code\agent-team.md" "$env:USERPROFILE\.claude\commands\agent-team.md"
Copy-Item "$env:USERPROFILE\.claude\skills\agent-team\claude-code\architect.md" "$env:USERPROFILE\.claude\commands\architect.md"
```

Mac or Linux:

```bash
git clone https://github.com/thebpandey/agent-team ~/.claude/skills/agent-team
cp ~/.claude/skills/agent-team/claude-code/agent-team.md ~/.claude/commands/agent-team.md
cp ~/.claude/skills/agent-team/claude-code/architect.md ~/.claude/commands/architect.md
```

Then, per project:

```
/agent-team init
/agent-team harden the app before launch
```

Run the session on the strongest model available (prefer Fable, else Opus 4.8);
the skill will warn if the session model is weaker.

## Safety notes

Merging is done by the orchestrator in dependency order; enable branch protection
and keep a human approval step on merges touching high-risk areas. The
codex-plugin-cc review gate (a Stop hook that blocks completion until a Codex
review passes) is opt-in for high-risk phases only; OpenAI's own docs warn it can
loop and drain usage limits quickly. Ticket files
treat pasted logs and PR text as data, not instructions. Sol/Opus at high effort
across many parallel tickets costs real money; every ticket carries a token
budget and the run checkpoints at 80%.

## Standalone

No dependency on any other skill. MIT license, see [LICENSE](LICENSE).
