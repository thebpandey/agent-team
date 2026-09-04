# Deployment, recovery, and cleanup

## Standing authorization

Use the active user's standing authorization for automatic deployment and safe rollback when it covers this project and established target/process. Do not ask again per task. If no applicable deployment authorization exists, prepare a concrete verified release and request the missing authorization once; installing or invoking this public skill does not itself grant it. Preserve authorization in project/task context so resumption does not ask again. Loading the skill does not identify a new destination or grant destructive data authority. If the target is unclear, finish concrete reviewable implementation first, then ask one focused question. Honor enforced approval gates and access controls.

Before release, Astra verifies:

1. Requirements and required reviews/checks pass for the exact integrated source and resulting artifact. Identify target environment and release identity. Exclude unrelated uncommitted changes from the artifact.
2. The established deployment process, live health/smoke signals, and a bounded observation window appropriate to the project are known. If absent, establish a small realistic check rather than an elaborate monitoring exercise.
3. A known-good release artifact/revision and supported recovery action are identified. Confirm compatibility with schema, configuration, and external state. For first deployments without a prior release, define safe disabling/removal of the failed new release. Resolve irreversible migration recovery separately before release.

Prefer native deployment health checks and automatic recovery where available. This skill is not a background monitor and cannot guarantee recovery after runtime termination.

## Release once, inspect uncertain outcomes

Deploy the verified artifact, capture deployment identity, and confirm live health plus a representative changed user path on the intended destination. Mark success only when provider state and required live evidence agree. A build result is not production verification.

If deployment fails before live state changes, preserve the existing release and diagnose. For ambiguous status, inspect deployment/traffic state before retrying. If live checks fail, restore the known-good compatible artifact or approved first-release recovery, then verify recovery. Do not revert schema/data automatically without explicit authority and a known-safe procedure. If recovery fails or would be destructive/unknown, stop further promotions, preserve evidence, report the incident, and request the specific missing decision. Never enter a deploy/rollback loop.

After every successful release, including a verified rollback, record outcome, release identity, revision, verification/recovery evidence in Beads. Reopen affected feature tasks after rollback; recovery does not mean the feature was delivered. If tracking fails, retain a local checkpoint and repair the record without redeploying.

## Remove completed worktrees

Astra verifies each non-main task worktree's required changes are integrated into the successfully deployed and live-verified revision. Stop its writers/task processes, preserve evidence and resumption notes outside the checkout, inspect tracked/untracked files, then remove the clean disposable checkout through normal Git worktree removal. Verify removal and update Beads.

Preserve the main checkout regardless of directory name, unrelated user worktrees, active work, unintegrated changes, failed/rolled-back feature work, and unarchived evidence. Do not force-delete, reset away changes, or delete branches as a substitute for checking. Remove all eligible completed task worktrees, including explicitly adopted ones. Report retained trees with concrete reasons. Free only task-owned resources.
