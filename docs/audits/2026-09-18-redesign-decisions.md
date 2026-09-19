# Agent-Team vNext: approved redesign decisions

Status: design record only. No live hook, configuration, installation, or
runtime behavior changes are authorized by this document.

## Approved decisions

1. **Project Kickoff is optional.** Agent-Team may consume an approved Project
   Kickoff handoff when present, but does not require Project Kickoff to plan
   or execute work.
2. **First-run artifact check.** Agent-Team checks for required project
   artifacts and asks before first-run setup creates any missing artifact.
3. **No default Astra.** First-run setup presents model choices and concise
   recommendations for each functional role. It does not default the
   orchestrator or any other role to Astra.
4. **Flexible role routing.** Role assignments are functional rather than
   hard-bound to models. The orchestrator may select a cheaper suitable model
   for a particular task, subject to the task's complexity and required
   quality.
5. **Hook-free core.** The new Agent-Team core has no lifecycle hooks by
   default.
6. **Discard SessionStart.** The current Agent-Team `SessionStart` hook is
   specifically removed from vNext.
7. **Discard PreToolUse.** The current Agent-Team `PreToolUse` hook is
   specifically removed from vNext for both Codex and Claude.
8. **Discard PreCompact.** The current Agent-Team `PreCompact` hook is
   specifically removed from vNext for both Codex and Claude. A possible
   mitigation, still requiring design detail, is a small team-owned durable
   receipt updated after meaningful fact changes and read with the tracker and
   Git state on resumption.
9. **Discard Codex PostToolUse.** The current Codex Agent-Team `PostToolUse`
   handler for `Edit|Write|apply_patch` is specifically removed from vNext.
10. **Dashboard update after integration.** After each successful
    implementation-task integration, the orchestrator delegates a refresh of
    the ignored local HTML snapshot at
    `<project>/.agent-team/dashboard/index.html` to Luna or the lowest-cost
    suitable agent. A dashboard-only update does not recursively count as
    another implementation integration. The exact dashboard format and commit
    ordering remain to be designed.
11. **Dashboard visibility.** The future Agent-Team setup flow, settings
    surface, and GitHub README must clearly state the snapshot's location,
    purpose and status semantics, refresh timing, how to view it, and its
    local-only, cross-host limitation. The exact cross-host viewing action is
    still ambiguous: current material permits opening the local file or using
    an optional loopback helper.
12. **Discard Claude PostToolBatch.** The current Claude Agent-Team
    `PostToolBatch` hook is specifically removed from vNext.
13. **Two on-demand development servers per project.** Separate projects do
    not share Agent-Team-managed development servers. At most two active
    servers may exist per project. Each server is bound to one specific
    worktree and revision, allowing two distinct worktrees to preview
    concurrently. Reuse a compatible active server when it serves the same
    revision; further server-dependent checks queue while coding continues.
14. **Server evidence, lifecycle, and safety.** A worktree-specific preview or
    acceptance check cannot claim validation from a server serving another
    worktree. Record the served revision. Do not automatically switch a server
    back; stop it gracefully when it has no known consumers. An unverified or
    unknown process occupies its server slot, and Agent-Team must never kill a
    process solely by port or PID. The dashboard's static HTML snapshot is not
    a development server. Earlier host-wide, one-per-project, and automatic
    cross-project-sharing proposals are superseded and not adopted. The
    implementation of worktree/revision binding, consumer tracking, graceful
    stopping, queueing, and safe retargeting remains open; no helper is
    assumed.
15. **Discard Codex Interrupt.** The current Codex Agent-Team `Interrupt` hook
    is specifically removed from vNext.
16. **Durable per-team/task receipt.** Replace interruption checkpointing with
    a small, cross-platform, hook-free progress receipt per team/task. Update
    it immediately after consequential facts or decisions and before handoff
    or pause when possible. It links task IDs, current revision/worktree,
    evidence pointer, owner/server state, and pending operation/next action.
    On resumption, the orchestrator explicitly reconciles the tracker, Git
    revision, worktree changes, live worker/server ownership, and uncertain
    operations. Within the active harness, unknown liveness or ownership is not
    free capacity; it does not create a cross-harness transfer gate. The receipt
    does not duplicate the full context or tracker; it cannot restore truly
    unrecorded thoughts or guarantee work before an abrupt exit.
17. **Discard Claude TaskCompleted.** The current Claude Agent-Team
    `TaskCompleted` hook is specifically removed from vNext.
18. **Reviewer and explicit completion gate.** The existing non-author
    reviewer, with no extra fixed verifier, receives a minimal exact-revision
    task packet, runs or inspects real checks, and reviews semantic requirements
    and code. It returns `FIX` or revision-bound `CLEAN` evidence. The
    developer repairs `FIX` findings and the same reviewer rechecks the new
    revision. The orchestrator alone invokes one explicit deterministic
    cross-host completion gate, based on the existing `gate-evidence` concept
    and simplified later, to validate and record canonical evidence before
    serial integration. If integration changes the code, it rechecks the
    affected revision. This flow does not guess tests with AI or retain a
    duplicate hook path.
19. **Provisional model routing and benchmark.** Respect the user's setup
    choice and a per-task override, and verify actual availability. Proposed
    defaults are Codex Terra medium for ordinary work, Luna low only for
    validated trivial/low-risk work, and Sol high for high-risk work; Claude
    Sonnet 5 medium for ordinary work, Haiku only for validated trivial work,
    and Opus 5 high for high-risk work. Benchmark 20–30 historical diffs for
    misses, false positives, tokens, elapsed time, and repair cycles before
    locking these defaults.
20. **Discard Claude UserPromptExpansion.** The current Claude Agent-Team
    `UserPromptExpansion` hook is specifically removed from vNext. No
    replacement is needed for mere activation telemetry: native skill
    invocation continues to work. Record only meaningful run-start, resume,
    and decision facts in the durable per-team/task receipt, not a per-prompt
    activation log.
21. **Agent-Team hook disposition complete.** All current Agent-Team hooks are
    decided for vNext and discarded.
22. **Project Kickoff planning-only SessionStart.** Project Kickoff-owned
    Claude `SessionStart` `load_context.py` may remain optional only during an
    active Kickoff planning phase. Agent-Team vNext neither installs nor
    depends on it, and it is disabled at a validated handoff. It is currently
    inactive in this repository and POSIX-only; native Windows resumes use the
    instruction/read-on-resume fallback.
23. **Discard Project Kickoff PreToolUse.** Project Kickoff-owned Claude
    `PreToolUse` `guard_edits.py` is removed entirely in vNext. It enforces
    path and subagent edit restrictions, but does not itself own task claims or
    transfer ownership; that separate ownership mechanism remains under
    investigation. The portable replacement principle is explicit task and
    worktree writable-path assignment with a single-writer protocol, not
    another hook.
24. **Cross-harness switching and ownership.** Codex and Claude workers never
    run concurrently on one project. One active orchestrator controls parallel
    worker admission, assigns disjoint writable paths and resources, and
    serializes overlap. Switching harness or model continues immediately
    without coordinator transfer, writer lease, manual release, process-stop
    proof, or returning to an old session. Persist task assignment and progress
    in `TASKS.md` or Beads for recovery, not as cross-harness locks. On a
    switch, read the tracker and Git state and record prior-harness attempts as
    interrupted. By workflow convention the user stops the old run before
    switching; a still-running old worker remains a caveat to report, not a
    hard gate.
25. **Discard Project Kickoff PostToolUse.** Project Kickoff-owned Claude
    `PostToolUse` `check_checkpoint.py` is removed entirely in vNext. It is
    globally registered but currently inactive for this repository because the
    project has not opted in; it is advisory-only and POSIX-coupled. No
    replacement hook or command is added. During an optional Project Kickoff
    handoff, the receiving agent checks required structural fields once as
    normal handoff review. Projects without Project Kickoff use their tracker
    and task receipts.
26. **Adaptive optional accelerators.** The native core remains the source of
    truth and works without optional tools. First-run setup offers an explicit
    preferred, optional install prompt for LeanCTX, Serena, and Graphify. The
    orchestrator narrowly routes a task to at most one analytical accelerator
    initially, only when its question warrants it; LeanCTX is limited to
    output compression, while durable task state remains in the tracker and
    receipt. An absent, unhealthy, or declined optional tool never gates task
    assignment, review, integration, or deployment: the task uses native
    source, Git, and project-check fallbacks. When an accelerator is used, its
    compact, revision-bound result and applicable cost/quality metrics are
    recorded in the receipt. This is design only; it adds no hook, lease, or
    live tool configuration.
27. **Clean-room vNext rebuild.** Build and validate vNext as a separate,
    clean-room implementation rather than incrementally changing the legacy
    runtime. Migration, compatibility, backup, rollback, and legacy removal
    remain separate implementation decisions; this approval does not alter the
    live v7.3.1 installation or registrations.

## Evidence informing these decisions

- [State, hooks, and Project Kickoff addendum](2026-09-18-state-hooks-kickoff-addendum.md)
  distinguishes current runtime state from declared and stale records.
- [Hook audit](2026-09-18-state-hooks-kickoff-addendum.md#hook-implementation-and-behavior)
  records SessionStart's cache write, recovery read, latency cap, and limits.

These decisions are deliberately narrower than the full redesign. Tracker
selection, handoff shape, review loop, state format, deployment batching,
cross-host operation, and Windows support require later decisions.
