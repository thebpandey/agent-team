---
description: Generate a paste-ready multi-agent delegation prompt (Claude Code or Codex) from a rough task
argument-hint: [the task, optionally "N agents", "low|medium|high effort", "heist mode"]
---

Act as a delegation architect. Take the task below and output ONE paste-ready
prompt that fans the work out across async sub-agents in clearly separated phases,
then fans the results back into one plain-language report. Target Claude Code (Task
tool) or Codex (parallel worker calls); only the orchestrator fans out.

TASK: $ARGUMENTS

Follow this contract exactly.

1. Decompose the task into work units. Map dependencies: if unit B needs unit A's
   output (a shared type, schema, interface, or result counts), B waits for A.
2. Group independent units into PARALLEL phases. Chain dependent units into later
   SEQUENTIAL phases. Tag every phase PARALLEL or SEQUENTIAL.
3. Agent count: if the task names a number, use exactly that and flag any mismatch
   with the natural unit count. If no number, derive one, one agent per independent
   unit, capped at 7 concurrent, overflow into labeled waves.
4. Effort per agent: use the level named in the task, else default High. High means
   do it, self-check, re-derive the key result a second way, then cross-check against
   the other agents. Medium means do it and check once. Low means one pass. Prefer a
   lighter model for Low and a stronger model for High where model selection exists.
5. Never assume nested sub-agents. Only the orchestrator fans out.
6. Give each agent a scope lock (the exact files or dirs it may touch, nothing else).
7. Add stop conditions: pause and ask before deleting files, adding dependencies, or
   changing the database or schema.
8. Demand fan-in: after the last phase, produce ONE consolidated report, written for
   a high-school reader, with per-agent (agent and job, what happened, findings,
   concerns, failures, successes) then an overall summary and next steps.
9. Add output discipline to the generated prompt: no task restatement, no narration,
   no tool logs, no explanation unless it changes a decision or a next step. Print
   only phase markers and the final report.
10. If fan-out is large or effort is High across many agents, add a one-line cost
    warning to the generated prompt.
11. Add teardown: each agent cleans up anything it started (servers, containers,
    temp files, background jobs) before returning. Do not tell agents to "close"
    themselves; they self-terminate on return.
12. Add a large-output rule: if a result is big, the agent writes it to a file under
    the run's temp directory and returns a summary plus the file path, so returns
    cannot overflow the orchestrator.
13. Add cleanup: after the consolidated report is written and displayed, delete the
    run's temp directory only. Never auto-delete pre-existing files or real
    deliverables. Do not tell agents to "close" themselves; they self-terminate.
14. Heist mode (only if the task says "heist mode" or "personas on"): assign a named
    crew member per sub-task type (security = Livingston, DevOps = Basher, frontend =
    Frank, concurrency = Malloys, optimization = Yen, legacy = Saul, data = Linus,
    code review = Rusty, budget = Reuben, orchestrator = Danny). Inject the name and
    an ASCII prefix like "[Basher] ->" into each agent's prompt, and prefix its log
    lines with it. ASCII only, no emoji. Personas color log lines only; the final
    report stays plain high-school language.

Output: the single prompt block, then one plan line stating phases, which are
parallel vs sequential, agent count, and effort. Do not explain your reasoning
unless asked.
