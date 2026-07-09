# agent-team: Templates and Worked Examples

Load this only when you need the exact prompt wording, the full report schema, or an example to copy. Do not paste this file's headers into your output.

---

## Master Prompt Template

This is the shape of the prompt you emit. Replace every bracketed placeholder. Delete any section that does not apply, but never delete the phase plan, the report schema, the suppression rules, or the stop conditions.

```
You are the orchestrator for a multi-agent run. Your job: [ONE SENTENCE GOAL].
Spawn parallel workers using your runtime's sub-agent mechanism (Claude Code:
the Task tool; Codex: parallel worker calls). Only you, the orchestrator, fan out.
Workers cannot spawn their own workers.

GOAL
[The single end deliverable this whole run must produce.]

PHASE PLAN
Run the phases in the order below. Inside a PARALLEL phase, spawn all listed
agents at once using the Task tool and let them run concurrently. Wait for every
agent in a phase to finish before starting the next phase.

Phase 1 [SEQUENTIAL]  (run only if a shared piece must exist first)
  - Agent A: [unit]. Scope: [files or dirs it may touch]. Effort: [level].

Phase 2 [PARALLEL]  (these do not depend on each other)
  - Agent B: [unit]. Scope: [...]. Effort: [level].
  - Agent C: [unit]. Scope: [...]. Effort: [level].
  - Agent D: [unit]. Scope: [...]. Effort: [level].

Phase 3 [SEQUENTIAL]  (depends on Phase 2 output)
  - Agent E: [unit]. Scope: [...]. Effort: [level].

EFFORT
[High] means: do the work, check it yourself, then re-derive the key result a
second way, then compare your findings against what the other agents returned.
[Medium] means: do the work and check it once.
[Low] means: do the work in a single pass.
Prefer a lighter model for Low-effort agents and a stronger model for High-effort
agents where model selection is available.

SCOPE LOCK
Each agent may only touch the files and directories listed in its line. It must
not read, edit, or create anything outside that scope.

STOP CONDITIONS
Pause and ask the operator before: deleting any file, adding any dependency, or
changing the database or any schema. Do not take these actions on your own. The one
exception is the run's own temp directory, which you delete automatically in the
cleanup step below.

TEARDOWN
Before an agent returns, it cleans up anything it started: dev servers, containers,
temp files, background jobs. Do not leave processes running.

LARGE OUTPUT
If an agent's result is large, it writes the full output to a file under the run's
temp directory (for example ./.agent-team-tmp/) and returns a short summary plus
the file path. Do not paste large output back into the run, or the combined
returns can overflow me and crash the session.

FAN-IN
After the last phase, read any temp files, fold their needed content into ONE
consolidated report using the format below, and display it.

CLEANUP
After the report is displayed, delete the run's temp directory (./.agent-team-tmp/).
Delete only that directory. Do not touch pre-existing files or the real
deliverables. If a delete fails, note it in one line rather than stopping.

REPORT FORMAT (write for a high-school reader: short sentences, plain words)
For each agent:
  - Agent and job: one line.
  - What happened: two or three plain sentences.
  - Findings: the useful things it learned or made.
  - Concerns: anything risky or unclear.
  - Failures: what it could not do, and why.
  - Successes: what it finished and checked.
Then once:
  - Overall summary: three to five sentences tying it together.
  - Next steps: the immediate actions to take.

OUTPUT DISCIPLINE
Do not restate this task. Do not narrate what you are about to do. Do not print
tool logs or step-by-step commentary. Do not explain anything unless the
explanation changes a decision I must make now or a next step I must take. Print
only a short marker at the start of each phase, and the final report.

[COST NOTE: include only when fan-out is large or effort is High across many
agents:]  This run spawns [N] agents at [effort]. That multiplies token and
credit use and may hit rate limits. Reduce agent count or effort if that is a
concern.
```

Under the block, add one plan line, for example:
`Plan: 3 phases. Phase 1 sequential (1 agent), Phase 2 parallel (3 agents), Phase 3 sequential (1 agent). Effort High. Total 5 agents.`

---

## Effort Ladder (full detail)

| Effort | Model preference | What the agent does | Turn budget |
|--------|------------------|---------------------|-------------|
| Low | haiku or inherit | One pass. No self-check. | Tight |
| Medium | sonnet | One pass, then one self-check. | Moderate |
| High (default) | sonnet or opus | One pass, self-check, re-derive the key result a second way, cross-check against other agents' returns. | Generous |

Confirm current valid model IDs against Claude Code docs before hardcoding a
model string. The mapping is stable; the exact IDs change over time.

---

## Report Schema (full detail)

The consolidated report is the only substantial thing the run prints. Keep every
line readable by a high-school student. Define any needed technical word in one
short clause right where it appears.

Right (plain, scannable):
```
Agent B: check the login code
What happened: It read the three login files and traced how a user signs in.
Findings: The password check works, but the "stay signed in" setting is never saved.
Concerns: A user who ticks "stay signed in" will still get logged out. Worth a fix.
Failures: None.
Successes: Confirmed the main login path works end to end.
```

Wrong (jargon dump, no structure):
```
Agent B performed a static analysis of the authn module and surfaced a latent
defect in session persistence whereby the remember-me boolean is not marshaled
to the token store, resulting in premature session invalidation.
```

---

## Worked Example 1: Independent fan-out (all parallel)

User: "Audit my codebase across four areas and tell me what's wrong."

There are no dependencies between the four areas, so one PARALLEL phase.

```
You are the orchestrator for a multi-agent run in Claude Code. Your job: audit
this codebase and report what is wrong.

GOAL
One report covering the health of four areas: frontend, API, database, auth.

PHASE PLAN
Phase 1 [PARALLEL]
  - Agent 1: audit the frontend. Scope: /src/components, /src/pages. Effort: High.
  - Agent 2: audit the API. Scope: /src/api. Effort: High.
  - Agent 3: audit the database layer. Scope: /src/db. Effort: High.
  - Agent 4: audit auth. Scope: /src/auth. Effort: High.

[effort, scope lock, stop conditions, fan-in, report format, output discipline
as in the master template]
```

Plan line: `Plan: 1 phase, parallel, 4 agents. Effort High.`

---

## Worked Example 2: The dependency trap (sequential then parallel)

User: "Build the user feature: types, model, API routes, tests, and docs."

Types are a shared contract. The model, routes, tests, and docs all consume them.
So types run first, alone, then the rest fan out.

```
GOAL
A working user feature: shared types, model, API routes, tests, docs.

PHASE PLAN
Phase 1 [SEQUENTIAL]  (shared contract, must exist first)
  - Agent 1: define the user types and interfaces. Scope: /src/types/user.ts. Effort: High.

Phase 2 [PARALLEL]  (all consume the types from Phase 1)
  - Agent 2: build the user model. Scope: /src/models/user.ts. Effort: Medium.
  - Agent 3: build the API routes. Scope: /src/api/user. Effort: Medium.
  - Agent 4: write tests. Scope: /tests/user. Effort: Medium.
  - Agent 5: write docs. Scope: /docs/user.md. Effort: Low.
```

Plan line: `Plan: 2 phases. Phase 1 sequential (1 agent), Phase 2 parallel (4 agents). Total 5 agents.`

Why it matters: if types run in parallel with the model and routes, each agent
invents its own version of the user shape and they conflict. The 30-second
sequential step prevents a long cleanup.

---

## Worked Example 3: User number smaller than the unit count

User: "Use 3 agents to research these 5 competitors."

Five units, three agents. Group so they fit, and flag it.

```
PHASE PLAN
Phase 1 [PARALLEL]
  - Agent 1: research Competitor A and Competitor B. Scope: web research only. Effort: High.
  - Agent 2: research Competitor C and Competitor D. Scope: web research only. Effort: High.
  - Agent 3: research Competitor E. Scope: web research only. Effort: High.
```

Plan line: `Plan: 1 parallel phase. 5 competitors grouped into your 3 agents (A+B, C+D, E). Effort High.`

---

## Worked Example 4: More units than the concurrent cap (waves)

User: "Explore all 12 modules in parallel."

Twelve units, cap of 7. Two waves.

```
PHASE PLAN
Phase 1 [PARALLEL] Wave 1
  - Agents 1 to 7: one module each. Scope: the module's directory. Effort: Low.
Phase 1 [PARALLEL] Wave 2  (start after Wave 1 returns)
  - Agents 8 to 12: one module each. Scope: the module's directory. Effort: Low.
```

Plan line: `Plan: 1 parallel phase in 2 waves (7 then 5). 12 agents total. Effort Low (exploration).`

---

## Worked Example 5: Heist mode with crew personas (ASCII prefixes)

User: "Audit the app: security, frontend, and deploy config. Heist mode on."

Three independent units map to Livingston (security), Frank (frontend), Basher
(DevOps). Heist mode colors the log lines only; the report stays plain.

```
You are the orchestrator (Danny Ocean) for a multi-agent run. Your job: audit the
app across security, frontend, and deploy config. Only you fan out.

PHASE PLAN
Phase 1 [PARALLEL]
  - Livingston Dell (security). Prefix log lines with "[Livingston] ->".
    Scope: /src, /config. Effort: High.
  - Frank Catton (frontend). Prefix log lines with "[Frank] ->".
    Scope: /src/components, /src/styles. Effort: Medium.
  - Basher Tarr (DevOps). Prefix log lines with "[Basher] ->".
    Scope: /docker, /.github/workflows. Effort: Medium.

CREW (heist mode ON)
Each agent may write its prefixed log lines in character and add one short
in-character quip at the top of its report section. Findings, concerns, failures,
and successes stay plain high-school language. If a quirk would hide information,
drop it.

[teardown, large-output, stop conditions, fan-in, report format, output
discipline as in the master template]

REPORT FORMAT
Section headers use the ASCII prefix, for example "[Basher] ->". Everything under
them is plain language.
```

Plan line: `Plan: 1 parallel phase, 3 crew agents (Livingston, Frank, Basher). Heist mode on, report plain. Effort High/Medium.`

Log line, heist on: `[Basher] -> Blimey, the Dockerfile runs as root. Flagging it.`
Same finding in the report, always plain: `Findings: The Docker image runs as root, which is a security risk. Switch to a non-root user.`
