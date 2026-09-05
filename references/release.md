# Deployment, recovery, and cleanup

## Integration and preview gates

The project orchestrator owns integration and production release under [project coordination](projects.md). Feature teams hand off branches and evidence; they do not publish independently. Before integrating a feature, inspect its canonical [preview gate](preview.md). A required missing, stale, or unknown approval blocks that feature even when tests pass and standing deployment authority exists. Verify the combined version in the integration worktree and update main through the established process. Preserve exclusive integration/release ownership across the operation.

## Standing authorization

Use the active user's standing authorization for automatic deployment and safe rollback when it covers this project and established target/process. Do not ask again per task. If no applicable deployment authorization exists, prepare a concrete verified release and request the missing authorization once; installing or invoking this public skill does not itself grant it. Preserve authorization in project/task context so resumption does not ask again. Loading the skill does not identify a new destination or grant destructive data authority. If the target is unclear, finish concrete reviewable implementation first, then ask one focused question. Honor enforced approval gates and access controls.

Before release, the orchestrator verifies feature-code explanations and relevant MISTAKES.md updates within the existing review. Acceptance evidence must cover the important changed user flows on the integrated version. Blocked or simulated checks must not be reported as real end-to-end success. Then verify:

1. Requirements and required reviews/checks pass for the exact integrated source and resulting artifact. Identify target environment and release identity. Exclude unrelated uncommitted changes from the artifact.
2. The established deployment process, live health/smoke signals, and a bounded observation window appropriate to the project are known. If absent, establish a small realistic check rather than an elaborate monitoring exercise.
3. A known-good release artifact/revision and supported recovery action are identified. Confirm compatibility with schema, configuration, and external state. For first deployments without a prior release, define safe disabling/removal of the failed new release. Resolve irreversible migration recovery separately before release.

Prefer native deployment health checks and automatic recovery where available. This skill is not a background monitor and cannot guarantee recovery after runtime termination.

## Release once, inspect uncertain outcomes

Deploy the verified artifact, capture deployment identity, and confirm live health plus a representative changed user path on the intended destination. Mark success only when provider state and required live evidence agree. A build result is not production verification.

If deployment fails before live state changes, preserve the existing release and diagnose. For ambiguous status, inspect deployment/traffic state before retrying. If live checks fail, restore the known-good compatible artifact or approved first-release recovery, then verify recovery. Do not revert schema/data automatically without explicit authority and a known-safe procedure. If recovery fails or would be destructive/unknown, stop further promotions, preserve evidence, report the incident, and request the specific missing decision. Never enter a deploy/rollback loop.

After every successful release, including a verified rollback, record outcome, release identity, revision, verification/recovery evidence in the active tracker: Beads when in use, otherwise the canonical TASKS.md. Reopen affected feature tasks after rollback; recovery does not mean the feature was delivered. If tracking fails, retain a local checkpoint and repair the record without redeploying.

## Required cleanup after production verification

After each successful production verification, the project orchestrator must perform cleanup in the same release workflow, including on resume after an interrupted release. Do not leave cleanup as an optional suggestion. First repair the release record if needed; no redeployment is required.

The orchestrator verifies each non-main task worktree's required changes are integrated into the successfully deployed and live-verified revision. Stop its writers/task processes, preserve evidence and resumption notes outside the checkout, inspect tracked/untracked files, then remove the clean disposable checkout through normal Git worktree removal. Verify removal and update the active tracker. Never remove the checkout containing the canonical local tracker or its referenced unarchived evidence.

Preserve the main checkout regardless of directory name, unrelated user worktrees, active work, unintegrated changes, failed/rolled-back feature work, and unarchived evidence. Do not force-delete, reset away changes, or delete branches as a substitute for checking. Remove all eligible completed task worktrees, including explicitly adopted ones. Report retained trees with concrete reasons. Free only task-owned resources.

Inventory all feature and child worktrees created or adopted for the deployed team, plus its preview servers, task processes, logs, temporary files, dependency directories, and build output. Verify process identity and stop only task-owned processes before removal. Preserve required screenshots, reports, CONTEXT.md recovery notes, and unresolved MISTAKES.md handoffs outside disposable worktrees. Inspect untracked and ignored files as well as tracked files; classify secrets and user files for preservation, not publication. Remove known disposable task output through safe path-scoped operations, then use normal Git worktree removal. Do not use force deletion to bypass unknown or uncommitted work.

Removing an eligible worktree also recovers its local dependency and build directories. Do not clear shared package caches, shared databases, global installations, or another team's resources to inflate reclaimed space. Remove a clean idle integration worktree when no queued/unfinished integration needs it; otherwise reuse that one worktree and record the retention reason. Do not create a new integration worktree per feature.

Confirm each intended path and process is gone and the Git worktree list is correct. Record team ID, release identity, removed resources, retained resources with reasons, and reclaimed disk space when supported by a practical before/after measurement. Do not invent a disk-space figure. Mark Cleanup complete only when all eligible resources are removed; open one bounded cleanup issue for failures, preserve the production-success record, and retry only with new evidence. Future resume includes pending cleanup. Never delete unfinished feature work merely to reduce the number of worktrees.
