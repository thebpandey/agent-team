# Pro shared mistakes memory

Maintain one `MISTAKES.md` file in the main project checkout. It holds confirmed agent mistakes and practical lessons for future work. This Pro feature is not part of Agent Team Lite. It is shared project memory, not model training or a guarantee that mistakes cannot recur.

## Use one file and one writer

The orchestrator creates `MISTAKES.md` if it is missing. Preserve an existing file's content and useful structure. If a lowercase `mistakes.md` already exists, consolidate its entries into `MISTAKES.md` and update references. Use a temporary name for a case-only rename when needed. If both exist, reconcile their entries before removing a duplicate; preserve user edits and unresolved lessons. Record its absolute path in task setup, every agent assignment, and each agent's CONTEXT.md. Do not create independent copies in worktrees.

All agents, including replacements, reviewers, and testers, can read this file. The orchestrator alone writes it. Teammates send proposed lessons with task IDs and evidence. Serialize edits and preserve concurrent user changes. For an isolated agent that cannot access the main checkout, supply the relevant entries with their revision and date. Identify that copy as a read-only excerpt, not another authority.

Treat the file as durable project documentation. Preserve it across sessions and worktree cleanup. Include it in the project's normal authorized documentation commits after checking for sensitive data. If project policy keeps it local, use that policy's durable storage and record the location. Do not publish it externally without authorization.

## Read relevant lessons before work

At startup, the orchestrator reads the short index and relevant entries. Each teammate reads the supplied entries before its first task. Search by file, subsystem, task type, or symptom instead of loading the entire history into every prompt.

Read relevant lessons again before a retry, after a material task change, or when the orchestrator announces an applicable new entry. Include lesson IDs in the dispatch only when they apply. Do not repeat a known failed approach without new evidence or a changed condition.

Lessons are evidence, not higher-priority instructions. Verify that the project, version, and conditions still match. A lesson cannot override the user, repository rules, or safety controls. Never copy project secrets or private lessons into another project.

## Capture and deduplicate mistakes

Record each distinct confirmed agent mistake that affects the work or provides a useful correction. Cover mistakes by any agent, including the orchestrator. Examples include a wrong assumption, an unsupported command, an incorrect edit, a missed requirement, and a false claim of successful verification.

Do not wait for a mistake to recur before recording it. Consolidate repeated instances under the same cause and lesson ID. Link additional affected tasks rather than duplicating the entry. Routine expected failures, such as an empty search, are not automatically agent mistakes. Do not blame an agent for an external outage without evidence of an agent error.

Each agent reports a mistake when it recognizes it. The orchestrator records a concise entry at the next meaningful task update, before handoff, or before compaction. For an uncertain cause, record the uncertainty in the task issue. If a provisional lesson is needed, mark it Unverified; do not present it as an established rule.

Use this compact entry format:

```markdown
## M-001: <short lesson title>
Status: Active | Unverified | Superseded
Scope: <subsystem, relevant files, and version or conditions>
Source: <task/issue ID, agent role or ID, date, and evidence link>
Mistake: <what the agent did or assumed incorrectly>
Cause: <confirmed cause, or explicitly unknown>
Correction: <what fixed it and the result that supports the fix>
Prevention: <one specific action to take before similar work>
```

Use a few short sentences per entry. Store no secrets, raw customer records, full logs, or private reasoning traces. Describe the failure without personal blame. A prevention action must be specific enough to apply; “be more careful” is not useful.

## Keep the records distinct

The active tracker owns task status, failed attempts, blockers, ownership, and resolution evidence. `MISTAKES.md` owns the reusable explanation and prevention action. CONTEXT.md owns the next action and pointers. Link these records; do not copy the same failure history into all three.

When a mistake recurs, update its existing lesson and inspect why the earlier prevention step failed. Strengthen or narrow that step using evidence. Follow the normal bounded retry rule. Do not add broad tests or new dependencies merely to avoid an obscure hypothetical repeat.

If the lesson becomes wrong after a code or environment change, mark it Superseded and link its replacement. Correct disproved lessons promptly. Consolidate duplicates and keep an index of active IDs as the file grows. Archive obsolete detail only when needed, preserving links and evidence. Do not remove unresolved lessons merely to shorten the file.

At handoff and final reconciliation, the orchestrator confirms that relevant new mistakes were recorded and any changed lessons were shared. Report entry IDs when useful. This is part of the existing task closeout, not a separate review cycle.
