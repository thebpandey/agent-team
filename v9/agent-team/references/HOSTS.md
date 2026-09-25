# Native host routing

Use this reference only after `start` has selected disjoint ready work. This is active-session orchestration, not a controller or a durable worker registry. Keep the actual returned host handles in the orchestrator's active session. Never create an acknowledgement, completion, identity, or status that the host did not return or visibly report.

## Shared dispatch contract

- Default to no more than two parallel teams. Each team has at most four ordered tasks and claims only its current task immediately before it starts. A one-off request first becomes a Beads task and follows the same two-team limit.
- Select disjoint write scopes and create a separate Git worktree for every active task. The main worktree is for planning, status, decisions, integration, and reports; it is not a feature workspace.
- Give the worker only the task facts and acceptance criteria, its worktree and permitted scope, applicable excerpts of `AGENT_TEAM_RULES.md` when present, and relevant decision/mistake IDs. Direct user instructions outrank project rules, but project rules cannot widen [the worker contract](WORKER_RULES.md).
- Retain the returned native handle, host, task ID, worktree, branch, and last observed result in the current host session. Beads and Git, rather than a controller record or checkpoint, remain task and revision authority.
- Wait only for a bounded agreed interval. Inspect the host's available status/completion result. If a worker shows no progress after that interval, request a status report through its existing native handle, record a stale/hung concern for that task, and continue independent lanes.
- Refill a completed retained team with at most four newly ordered tasks, again claiming only the next one. Do not replace an uncertain, rejected, or unavailable worker with a fabricated handle.
- If the required native capability is absent, report the task-local blocker. Do not launch a shell worker, silently kill a stale worker, or claim the task completed.

## Codex

Dispatch with the native `spawn_agent` tool and retain the actual returned agent ID/canonical task path. For a retained agent, send its next task or a progress request with `followup_task`; do not spawn a replacement simply to deliver follow-up work. Use `list_agents` to inspect the current root agent tree and its reported state.

`list_agents` observes the current root tree, not every historical session. An absent result there is not proof that a worker was never launched, has stopped, or has no changes. Treat a timeout, lost response, or ambiguous observation as uncertainty for that task. Inspect the original worktree and host-visible evidence before any later reassignment.

When the user requests interruption, use the native `interrupt_agent` operation on the retained actual identity and report its returned status. A request to interrupt, or an absence from `list_agents`, is not a stop acknowledgement. Codex does not use a Claude Agent/resume identifier.

## Claude

Dispatch with Claude's supported Agent creation facility and retain the actual returned Agent/session identity in the active Claude session. Send a next task, progress request, or resumed assignment only through that host's supported resume/follow-up facility for the same returned identity. Observe completion/status through the host-visible Agent result or status view, and use its actual stop/interrupt facility only when it is available and requested.

Do not rename a Claude Agent/session identity as a Codex task path or pretend that Claude exposes `spawn_agent`, `followup_task`, or `list_agents`. If Agent creation, resume, observation, or stop is not available in the current Claude session, report that capability gap for the affected task. Do not emulate it with a shell process, synthetic controller state, or a new unrelated Agent.

## Launch outcomes

An explicit host rejection that guarantees no worker was created leaves no handle and no fictitious completion. Return only that claimed task to ready with an accurate Beads comment. A timeout, missing response, or any possible launch is uncertain: retain the evidence, keep the task incomplete and task-locally blocked as appropriate, and continue other ready lanes. Never infer a clean result, a stopped worker, or no launch from missing session data.
