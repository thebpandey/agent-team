# Graphify provenance and paired skill release design

Date: 2026-09-10

## Objective

Repair Agent-Team's Graphify offline-readiness check and publish compatible patch releases of Agent-Team and Project Kickoff.

- Project Kickoff 0.4.1 declares `graphify` in `plan.requiredCapabilities` and leaves all capability preparation and execution to Agent-Team.
- Agent-Team 7.1.1 accepts deterministic AST-resolved Graphify edges while rejecting semantic-origin graph content.
- Both repositories update their current-version README surfaces, pass exact-revision release gates, push `origin/main`, and publish their normal tagged GitHub releases and assets.

## Proven defect

Graphify 0.9.57 uses two independent fields:

- `_origin` identifies the extraction tier: `ast` or `semantic`.
- `confidence` describes how directly Graphify resolved a relationship: `EXTRACTED` or `INFERRED`.

Agent-Team 7.1.0 rejects every `INFERRED` edge. That rule incorrectly rejects deterministic callback and cross-file relationships whose `_origin` is `ast`. The repair must enforce provenance, not confidence wording.

## Selected approach

Agent-Team will retain the existing Graphify 0.9.57 pin. Its isolated functional probe will continue to scrub backend credentials and run exactly:

```text
graphify extract . --code-only --no-viz
```

The probe will require AST provenance for the generated fixture graph and reject semantic or missing provenance. A realistic callback fixture will require at least one deterministic `INFERRED` edge with `_origin: "ast"`. Existing traversal and cleanup assertions remain.

The documentation will explain that AST-derived inferred relationships are structural leads, not behavioral evidence. It will also require a fresh per-worktree graph for offline evidence instead of adopting output that might contain a prior semantic layer.

Rejected alternatives:

- Upgrading to Graphify 0.9.58: it does not fix this contract and adds unrelated dependency changes.
- Removing the confidence check without a provenance replacement: it would lose the offline safety gate.
- Changing Project Kickoff to inspect or run Graphify: this would duplicate Agent-Team's capability ownership.

## Components and ownership

### Agent-Team 7.1.1

One core writer owns the Graphify probe and dependency tests. One documentation/release writer owns version metadata, Graphify guidance, README, changelog, site badge, and release-artifact expectations. These paths are disjoint, but their changes merge only after the core contract is fixed.

A separate stabilization writer is admitted only if the existing deadline-sensitive test failures reproduce. That work stays separate from the Graphify semantic correction and may not weaken deadlines merely to obtain a green run.

### Project Kickoff 0.4.1

The already-integrated source remains unchanged unless release verification finds a concrete defect. Its package declares `graphify`, validates and preserves optional required capabilities, and explicitly forbids Project Kickoff from installing, initializing, executing, registering, or evaluating Graphify.

## Data and readiness flow

1. Project Kickoff emits an approved handoff with `plan.requiredCapabilities: ["graphify"]`.
2. Agent-Team adopts the handoff and prepares its selected dependency profile.
3. The Graphify functional probe creates an isolated fixture and fresh `graphify-out/` directory.
4. The probe verifies the exact offline command, scrubbed backend environment, `_origin: "ast"` graph provenance, the expected AST-derived callback edge, traversal behavior, and cleanup.
5. Fresh-worker discovery remains a separate readiness requirement. The functional probe does not fabricate native discovery or host trust.

Semantic-origin or missing-provenance graph content fails preparation with a provenance-specific diagnostic. An AST-origin `INFERRED` relationship does not fail solely because its confidence is inferred.

## Verification gates

Agent-Team requires targeted dependency tests, the real pinned Graphify test, documentation and artifact tests, package checks, and the complete test suite on the exact candidate revision. The two previously observed deadline failures must be reproduced and repaired if real. A retry alone cannot establish release readiness.

Project Kickoff requires its complete suite against the final Agent-Team source, Skill Creator validation, package-manifest checks, archive integrity, link and JSON checks, and byte comparison between the archive and its release revision.

Independent review covers requirements and code quality for each exact feature revision and each combined integration revision.

## Release sequence and recovery

1. Verify and publish Project Kickoff 0.4.1 to `origin/main`.
2. Create annotated tag `v0.4.1`, publish the GitHub release with `project-kickoff-0.4.1.zip` and `SHA256SUMS`, then download and verify the assets and Pages version.
3. Update Agent-Team README links to the verified Project Kickoff release.
4. Verify and push Agent-Team 7.1.1 to `origin/main`.
5. Create and push annotated tag `v7.1.1`. The existing tag workflow owns Agent-Team release creation and assets.
6. Require a green tag workflow, verify the GitHub release and downloaded assets, and requalify Graphify through an authorized Agent-Team preparation and fresh worker.

Before each push, confirm the remote branch still matches the tested base. Never force-push. If a push races, incorporate the remote change and repeat affected verification. If a tag workflow or live verification fails, hold further publication, inspect actual remote state, and repair or use the repository's known-good release procedure without repeating an ambiguous external operation.

## Completion criteria

- Project Kickoff `origin/main` and release `v0.4.1` point to the verified Project Kickoff revision.
- Agent-Team `origin/main` and release `v7.1.1` point to the verified Agent-Team revision.
- Published assets match their checksums and source manifests.
- Agent-Team's exact release revision has green local gates and GitHub checks.
- A fresh Agent-Team Graphify preparation accepts AST-origin inferred edges, rejects semantic-origin content, and records honest worker readiness.
- No Project Kickoff runtime path invokes Graphify.
