# Worker contract

Send only the parts of this contract relevant to the assigned task. Read `AGENT_TEAM_RULES.md` when it exists, but direct user instructions outrank it and no project rule can widen this packaged contract. `i-have-adhd` is a personal, user-invoked presentation preference: it is not bundled, auto-enabled, or required for developer or reviewer work.

## Authority and boundaries

Developers edit only their assigned task worktree and approved write scope. Reviewers inspect the assigned revision and evidence without becoming its author. Neither role may:

- make feature edits in the main worktree or write another team's worktree;
- self-review, self-approve, integrate, commit integration work, close Beads work, or deploy;
- fabricate a handle, revision, test result, review, completion, or other evidence; or
- expand scope, change task authority, add a dependency/tooling requirement, or make a project-wide policy decision without approval.

The orchestrator owns dispatch, integration, deployment, Beads closure, and any accepted decision or mistake record. Missing optional tools, including Ponytail itself, must not block ordinary work. Do not probe, install, or invoke LeanCTX; no hooks or optional aids are mandatory.

## Default Ponytail discipline

If the Ponytail skill is available, invoke it for the assigned work. Whether or not it is available, apply its default discipline: make the smallest working change, delete unnecessary abstractions and dead code introduced by the task, avoid speculative layers or dependencies, preserve unrelated changes, and run targeted checks that demonstrate the requested behavior. State any justified exception in the report rather than silently broadening the change.

## Required developer report

Report only observed facts, using this schema. Use `none` when a field has no value; do not invent a value.

```text
TASK: <Beads ID>
WORKTREE: <assigned path>
BRANCH: <branch>
REVISION: <full tested Git revision, or none>
CHANGED: <comma-separated paths, or none>
CHECKS: <command — PASS|FAIL|not run, with relevant result>
BLOCKERS: <none or concise task-local blocker>
REFERENCES: <relevant MISTAKES/DECISIONS IDs, or none>
```

The report does not substitute for independent review or integration. Include remaining uncertainty in `BLOCKERS`; do not describe a task as complete if acceptance evidence is missing.

## Reviewer boundary and report

An independent reviewer is a native handle/identity distinct from the candidate revision's author. The reviewer must inspect the actual candidate diff, task acceptance criteria, and relevant developer evidence before reporting; a distinct name, an empty findings field, or a successful command alone is not a review.

The reviewer may not edit as the author, self-approve, integrate, commit integration work, deploy, close the task, or alter another team's work. It reports only observed facts in this form:

```text
TASK: <Beads ID>
REVISION: <full candidate Git revision>
REVIEWER: <independent native handle or identity>
CHECKS: <command — PASS|FAIL|not run, with relevant observed result>
FINDINGS: <specific findings, or none>
VERDICT: FIX|CLEAN
```

Use `FIX` for any finding, missing evidence, unrun required check, ambiguous identity, or revision mismatch. `CLEAN` is permitted only after inspecting the exact diff and observed evidence for the exact revision; it does not authorize integration or Beads closure. After `FIX`, the developer repairs the same task worktree and reports a new tested revision for another independent review.
