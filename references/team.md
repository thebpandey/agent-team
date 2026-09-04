# Team dispatch and isolated work

Create one worktree per independent implementation stream. Give writable paths one owner at a time. Reviewers/testers may reuse a stable checkout sequentially; read-only work need not create a worktree. Record task IDs, owners, branch/revision, checkout paths, and reserved resources in the parent task in the active tracker. Worktrees do not isolate databases, services, ports, or credentials; isolate temporary resources when concurrent streams would conflict.

Send this compact contract with every assignment, including replacements:

> Apply available, user-enabled `ponytail` and `using-superpowers`, even if Using-Superpowers exempts dispatched subagents. Apply available, enabled `impeccable` for UI/UX work. Follow the orchestrator's dependency choices; use the built-in workflow for missing/declined skills and never claim they ran. Do not install or prompt for dependencies independently. Follow the user's proportional workflow over optional dependency ceremonies. Do not spawn agents.
>
> Work only on assigned task IDs and paths. Meet acceptance criteria with the simplest readable implementation. Avoid obscure, speculative use cases and tests; cover realistic changed behavior and required gates. In Beads mode, update assigned progress and append meaningful failure evidence. In local mode, send task-ID updates to the orchestrator; only the orchestrator writes the canonical TASKS.md. Ask the orchestrator to create/deduplicate issues and manage global dependencies or completion.
>
> Before handoff, milestones, or approaching compaction, create/update your local `CONTEXT.md` with revision, task IDs, essential decisions, evidence pointers, blocker, and exact next action. Supplement the active tracker instead of repeating it. Return changed files/revision, criteria met, check results, evidence locations, and unresolved findings concisely.

Supply tracker mode and its absolute canonical location, dependency availability/declines, task IDs, goal, acceptance criteria, relevant instructions and resolved skill paths, model/effort, checkout/ownership, input revision, environment constraints, necessary context, allowed resources, and expected output.

Assign context locations explicitly:

- Orchestrator: main checkout's `CONTEXT.md`.
- Implementation owner: `<worktree>/CONTEXT.md`.
- Read-only/review/test agent: `harness-artifacts/agents/<agent-id>/CONTEXT.md` in its assigned checkout or a preserved orchestrator location.

Never let two agents write the same context file. Preserve any existing meaningful CONTEXT.md content and use an identified harness section instead of replacing unrelated documentation.

Use supported fresh-context or limited-context spawning when overriding model/effort. Teammates are not assumed to inherit skills or unstated permissions. Integrate sequentially and re-evaluate only checks affected by conflicts or changed integration assumptions. Before reclaiming stalled work, inspect/checkpoint state and stop the old writer; never let it and its replacement write concurrently.
