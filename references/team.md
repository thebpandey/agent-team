# Team dispatch and isolated work

Create one worktree per independent implementation stream. Give writable paths one owner at a time. Reviewers/testers may reuse a stable checkout sequentially; read-only work need not create a worktree. Record task IDs, owners, branch/revision, checkout paths, and reserved resources in the parent Beads task. Worktrees do not isolate databases, services, ports, or credentials; isolate temporary resources when concurrent streams would conflict.

Send this compact contract with every assignment, including replacements:

> Apply `$ponytail` and `$using-superpowers` before work, even if Using-Superpowers exempts dispatched subagents. Apply `$impeccable` for UI/UX work. Resolve/read installed skills and report missing mandatory dependencies instead of claiming they ran. Follow the user's proportional workflow over optional dependency ceremonies. Do not spawn agents.
>
> Work only on assigned Beads IDs and paths. Meet acceptance criteria with the simplest readable implementation. Avoid obscure, speculative use cases and tests; cover realistic changed behavior and required gates. Update assigned Beads progress and append meaningful failure evidence. Ask Astra to create/deduplicate issues and manage global dependencies or completion.
>
> Before handoff, milestones, or approaching compaction, create/update your local `CONTEXT.md` with revision, task IDs, essential decisions, evidence pointers, blocker, and exact next action. Supplement Beads instead of repeating it. Return changed files/revision, criteria met, check results, evidence locations, and unresolved findings concisely.

Supply task IDs, goal, acceptance criteria, relevant instructions and resolved skill paths, model/effort, checkout/ownership, input revision, environment constraints, necessary context, allowed resources, and expected output.

Assign context locations explicitly:

- Astra: main checkout's `CONTEXT.md`.
- Implementation owner: `<worktree>/CONTEXT.md`.
- Read-only/review/test agent: `harness-artifacts/agents/<agent-id>/CONTEXT.md` in its assigned checkout or a preserved orchestrator location.

Never let two agents write the same context file. Preserve any existing meaningful CONTEXT.md content and use an identified harness section instead of replacing unrelated documentation.

Use supported fresh-context or limited-context spawning when overriding model/effort. Teammates are not assumed to inherit skills or unstated permissions. Integrate sequentially and re-evaluate only checks affected by conflicts or changed integration assumptions. Before reclaiming stalled work, inspect/checkpoint state and stop the old writer; never let it and its replacement write concurrently.
