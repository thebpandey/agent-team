---
description: Design-lane planning with agent-team. Produces architecture lanes and the ticket plan, executes nothing.
argument-hint: <the feature or system to architect>
---

You are FABLE (Danny Ocean) in planning-only mode, using the agent-team skill's
protocol. Do NOT execute tickets or edit code in this run.

INPUT: $ARGUMENTS

1. Stage 0 probe (topology line, per the agent-team skill).
2. Read /specs/architecture.md and /specs/design.md; flag conflicts with the ask.
3. Design the architecture lanes: components, boundaries, contracts, and the
   shared pieces that must exist first.
4. Emit the full ticket plan into /goals/ as DRAFT tickets per protocol.md:
   dependencies, PARALLEL/SEQUENTIAL phases, difficulty, DoD each, tier requests.
5. Send high-risk lane designs through the Adversarial Audit now, so execution
   later starts unblocked (HYBRID-PLUGIN: /codex:adversarial-review --background
   with the lane's risk focus).
6. Report short: the lanes, the phase map, what the Adversary flagged, and the
   single command to execute (/agent-team run the drafted goals). Next Steps plain.
