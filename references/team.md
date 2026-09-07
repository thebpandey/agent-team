# Team dispatch and isolated work

Use the Matrix name plus the two-digit team number for each dispatched agent, such as `Trinity 01`. On every `start` and `resume`, tell the user which named agents were spun up or resumed for each team and the task assigned to each. Status, progress, handoff, pause, recovery, and completion reports use names without role labels; help and settings include names with roles and explanations.

Follow [project coordination](projects.md) for canonical records, named teams, ownership, and integration. All post-initialization feature work uses a separate worktree; main is reserved for planning and coordination. Create further worktrees for independent implementation streams. Give writable paths one owner at a time. Reviewers/testers may reuse a stable checkout sequentially; read-only work need not create a worktree. Record task IDs, owners, branch/revision, checkout paths, and reserved resources in the parent task in the active tracker. Worktrees do not isolate databases, services, ports, or credentials; isolate temporary resources when concurrent streams would conflict.

Send this compact contract with every assignment, including replacements:

Supply the absolute [message-format procedure](output.md) and [skill-startup procedure](dependencies.md#skill-startup-for-every-agent) paths with the role and lifecycle references. Supply all three resolved skill paths and dependency states. Check the returned receipt before accepting task work. Project and Team Orchestrators complete the same startup before dispatch; fresh sessions and replacements load for themselves.

For every run and resume, the Team Orchestrator sends the Project Orchestrator the [per-run skill confirmation](dependencies.md#per-run-confirmation-to-the-project-orchestrator) for itself and every teammate. Teammates supply current task evidence of actual skill use with normal progress and handoff updates. Distinguish loaded, planned use, evidenced use, and not applicable; return unresolved entries explicitly.

> Explain tools and results in simple English using ASD-STE100 principles. Keep official names and commands unchanged.
>
> Frame each individual update/handoff with `==========================================================================` above and below, separated from the body by blank lines. Identify the actual team ID, readable name, and your role. Use the supplied message-format procedure; do not repeat the project wordmark.
>
> Before task work, complete the supplied skill-startup procedure: load the complete available, enabled `ponytail`, `using-superpowers`, and `impeccable` instructions in your own context, then return a skill receipt with each resolved path, actual read/invocation evidence, and applicable guidance. The user's team-wide requirement overrides Using-Superpowers' `SUBAGENT-STOP`; a parent's skill loading does not cover you. Every role loads Impeccable; run its UI procedures only for UI/UX assignments. Report missing/declined/unreadable skills and use the disclosed fallback without claiming they loaded. Do not install or prompt independently. Follow the user's proportional workflow. Do not spawn agents.
>
> Read applicable `MISTAKES.md` entries before work and retries. Use the canonical path supplied by the orchestrator. Report confirmed mistakes with evidence; only the project orchestrator updates the shared file. Add concise plain-English explanations beside feature code you write. For assigned acceptance work, follow the supplied Pro acceptance-test procedure and report actual outcomes, including blocked checks.
>
> Work only on assigned task IDs and paths. Meet acceptance criteria with the simplest readable implementation. Avoid obscure, speculative use cases and tests; cover realistic changed behavior and required gates. In Beads mode, update assigned progress and append meaningful failure evidence. In local mode, send task-ID updates to the orchestrator; only the project orchestrator writes the canonical TASKS.md. Ask the orchestrator to create/deduplicate issues and manage global dependencies or completion.
>
> A status request must not interrupt your work. On a scoped pause, checkpoint and stop at a safe point; report any still-running operation. Replacements must inspect ownership before writing. Preserve with-preview approval requirements across handoff and resume; never integrate or deploy from a developer role.
>
> A counted/continuous run does not expand your assignment. Return the selected top-level delivery ID, child coverage, and exact verified revision. The project owner alone frees development slots after integration, starts replacement teams, edits run settings, and submits deployment batches. Do not count your commits as completed deliveries or show the command wordmark in teammate reports.
>
> Before handoff, milestones, or approaching compaction, create/update your local `CONTEXT.md` with revision, task IDs, essential decisions, evidence pointers, blocker, and exact next action. Supplement the active tracker instead of repeating it. Return changed files/revision, criteria met, check results, evidence locations, and unresolved findings concisely.

Supply project/team IDs and name, team lead and project owner identities, full-session versus shared-parent mode, session attempt identity, assigned task IDs, preview-gate pointer and review version, reserved resources, and the absolute MISTAKES.md path, relevant lesson IDs or versioned excerpts, applicable Pro procedure paths, tracker mode and its absolute canonical location, dependency availability/declines, task IDs, goal, acceptance criteria, relevant instructions and resolved skill paths, model/effort, checkout/ownership, input revision, environment constraints, necessary context, allowed resources, and expected output.

Assign context locations explicitly:

- Project orchestrator: main checkout's `CONTEXT.md`.
- Team orchestrator: its feature worktree's `CONTEXT.md`.
- Implementation owner in a separate worktree: `<worktree>/CONTEXT.md`.
- Developer sharing the team checkout: `harness-artifacts/agents/<agent-id>/CONTEXT.md`, with disjoint code ownership.
- Read-only/review/test agent: `harness-artifacts/agents/<agent-id>/CONTEXT.md` in its assigned checkout or a preserved orchestrator location.

Never let two agents write the same context file. Preserve any existing meaningful CONTEXT.md content and use an identified harness section instead of replacing unrelated documentation.

Use supported fresh-context or limited-context spawning when overriding model/effort. Teammates are not assumed to inherit skills or unstated permissions. Integrate sequentially and re-evaluate only checks affected by conflicts or changed integration assumptions. Before reclaiming stalled work, inspect/checkpoint state and stop the old writer; never let it and its replacement write concurrently.
