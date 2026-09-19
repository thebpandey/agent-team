# Agent-Team vNext Master Implementation Roadmap

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to execute each accepted phase task with independent review.

**Goal:** Deliver the clean-room Go vNext through six gated plans without importing v7 runtime behavior.

**Authority:** [vNext design spec](../specs/2026-09-18-agent-team-vnext-design.md) is authoritative. The current v7 `TASKS.md` is byte-for-byte historical provenance, never a vNext live tracker.

## Dependency DAG

```text
Phase 1 native core → Phase 2 host execution ┬→ Phase 3 optional capabilities/dashboard observer
                                               └→ Phase 4 authorized deployment/recovery
Phase 3 + Phase 4 → Phase 5 install/migration/cutover
Phase 1 + 2 + 3 + 4 + 5 → Phase 6 release readiness
```

Every phase follows: author → independent review → remediation → independent CLEAN → serial integration. No phase begins implementation from a non-CLEAN prerequisite.

## Global Operating Rules

- Clean-room code lives only under `vnext/`; never import/copy/execute v7 hooks, agents, manifests, leases, claims, or state.
- Initialize Beads only after user confirmation. First calculate and record the SHA-256 of the existing root `TASKS.md`; run exactly `bd init --prefix atv --skip-hooks --skip-agents --non-interactive --role maintainer`; then calculate the same SHA-256 and byte-compare the file. Any difference aborts setup. This is a deliberate tracker cutover: Beads is then the sole live tracker, while `TASKS.md` remains byte-for-byte v7 provenance. Do not initialize Beads for a one-off run. Enforce 1,000 non-archived tasks and warn at 900.
- Create one root implementation epic; capture its generated Beads ID in the run receipt and use it as the only parent. Create exactly 43 direct child tasks beneath it: 11 Phase 1, 8 Phase 2, 7 Phase 3, 7 Phase 4, 4 Phase 5, and 6 Phase 6. Every child title must begin `P<N>.<NN>`, carry `agent-team`, `implementation`, `vnext`, and `phase-<N>` labels, and cite its exact source plan/task in the description. Preserve generated issue IDs and audit history; vNext never relies on `bd --id`, even if a local Beads installation offers it. Link each later-phase child to the prior phase’s CLEAN gate dependency, rather than creating duplicate phase epics. No other issue is a live implementation task.
- Use at most four Terra/Luna subagents: at most three implementers, one independent reviewer; retain and refill teams only after CLEAN. One active orchestrator assigns disjoint writable paths/resources. Worktree product edits belong only to task worktrees; main is control and serial integration. Capacity is `usable=min(configuredSlots, observedSlots)` when observed and `1` when unknown: `0` refuses admission; `1` permits one developer followed by a separately-contexted non-author reviewer; `N>=2` reserves one reviewer token and admits at most `N-1` developers. No task enters implementation without that eventual reviewer reservation.
- Each admission batch contains 1–8 tasks. Integrate only revision-bound CLEAN plus deterministic gate evidence, one candidate at a time. A changed candidate requires new review/CLEAN/gate.
- A blocker pauses only its lane/dependencies; unrelated lanes continue. Pause/stop/cancel/checkpoint write receipt plus derived handoff. Resume reconciles tracker, Git, worktree, resources, and external operation facts; no lease/release/PID proof. Before a Codex↔Claude switch the user stops the old foreground run by workflow convention; the new harness immediately resumes from those facts and marks old attempts interrupted. Do not operate Codex and Claude live on the same project simultaneously. A possibly still-running old worker remains a recorded risk whose paths/resources are serialized, not a hard liveness/lease gate.
- Deployment, install, cutover, release, tag, and push require their phase’s explicit authorization and verification evidence. Rollback preserves unknown/user files and records evidence. Never force-push.

## Interface Ledger

| Owner | Stable interface boundary | Consumers |
| --- | --- | --- |
| Phase 1 | `core`, `store.Store`, project/tracker/run/receipt/knowledge/workflow/resources schemas; `contracts.HostAdapter`, `contracts.WorktreeManager`, and deferred `contracts.DeploymentExecutor` port | 2–6 |
| Phase 2 | Phase-1 aliases `contracts.HostAdapter`/`contracts.WorktreeManager`; `host.Capabilities`, packet dispatch, reviewer FIX/CLEAN, deterministic gate/integration, lifecycle and cleanup | 3–6 |
| Phase 3 | `capability` probe/router/install contracts, typed resource CAS/registries, dashboard `IntegrationObserver`, `Snapshot`, and `DashboardRefreshReceipt` derived from integration results | 6 |
| Phase 4 | `deploy.ProviderAdapter` (the concrete three-argument Submit/Query/Verify adapter, not Phase 1’s deferred `contracts.DeploymentExecutor`), `BoundExecutor`, deployment operation/receipt/evidence, recovery and `DashboardTrigger` | 6 |
| Phase 5 | `install.Layout`, `Release`, `InstallManifest`, `CASOutcome`, `Install`/`Update`/`Rollback`/`Uninstall`, migration provenance/canary records | 6 |
| Phase 6 | deterministic artifact/SBOM/evidence/package/canary/release gates and the non-force publication workflow | release only |

## Roadmap Tasks

### Task 1: Establish tracker and Phase 1

**Plan:** `docs/superpowers/plans/2026-09-18-agent-team-vnext-native-core.md`.

**Dependencies:** User confirmation for Beads initialization; no phase dependency.

**Beads:** Under the captured root epic, create exactly 11 direct Phase 1 children titled `P1.01`…`P1.11`, one for every current Phase 1 task section. Before creating any Beads record, require `setup` to inventory existing `.agent-team` artifacts and either confirm their reuse or refusal; offer the optional Project Kickoff handoff and validate it only when approved; and, if no selected tracker exists, display the exact new `TASKS.md` artifacts and require fresh-bootstrap confirmation. A refusal writes no tracker, config, receipt, rules, or Kickoff artifact.

**Execution:** Assign up to three disjoint Phase 1 packages; reserve reviewer capacity. Run `cd vnext && go test ./... -count=1 && go vet ./...`.

**Evidence/gate:** schema/store/tracker/run/receipt/knowledge/workflow/resource acceptance output; independent reviewer returns CLEAN.

**Integration:** One serial batch after every revision-bound CLEAN; proposed commit `feat(vnext): complete native core phase`.

### Task 2: Host execution

**Plan:** `docs/superpowers/plans/2026-09-19-agent-team-vnext-host-execution.md`.

**Dependencies:** Phase 1 CLEAN and compiled interface ledger.

**Beads:** Under the captured root epic, create exactly 8 direct Phase 2 children titled `P2.01`…`P2.08`, each linked to the Phase 1 CLEAN gate.

**Execution:** Admit only conflict-free worktree/host tasks; reviewer is non-author. Verify `cd vnext && go test ./... && go vet ./...` plus native CI evidence.

**Gate:** main-edit rejection, FIX/CLEAN loop, mutation re-review, deterministic gate, serial integration, scoped resume, resource stop evidence, and cleanup all pass independently.

**Integration:** Serial Phase 2 batch; proposed commit `feat(vnext): complete host execution phase`.

### Task 3: Optional capabilities and dashboard observer

**Plan:** `docs/superpowers/plans/2026-09-19-agent-team-vnext-optional-capabilities.md`.

**Dependencies:** Phase 1–2 CLEAN.

**Beads:** Under the captured root epic, create exactly 7 direct Phase 3 children titled `P3.01`…`P3.07`, each linked to the Phase 2 CLEAN gate.

**Execution/evidence:** Consent/fallback first; no optional capability gates core. Verify two server/two browser caps, ownership/CAS, visual skill policy, Playwright evidence, and Current/Stale/Unavailable dashboard observation/rendering. Phase 3 owns local snapshot refresh from typed integration results; it neither consumes nor depends on a deployment trigger. Run `cd vnext && go test ./... && go vet ./...`.

**Integration:** One serial batch; proposed commit `feat(vnext): complete optional capabilities phase`.

### Task 4: Authorized deployment and dashboard trigger

**Plan:** `docs/superpowers/plans/2026-09-19-agent-team-vnext-deployment.md`.

**Dependencies:** Phase 1–2 CLEAN. Phase 3 remains independent: its `dashboard.IntegrationObserver` refreshes from typed integration results. This phase separately emits best-effort `deploy.DashboardTrigger` for failed deployment state; it does not supply Phase 3’s refresh input.

**Beads:** Under the captured root epic, create exactly 7 direct Phase 4 children titled `P4.01`…`P4.07`, each linked to the Phase 2 CLEAN gate.

**Execution/evidence:** First pass the fake-provider contract suite. Only after explicit pre-authorization for a named non-production target may the executor submit, query, and live-verify; persist the authorization reference, immutable profile/batch fingerprint, provider operation ID, evidence pointer, result, and any unknown/blocked state. Test disabled default, authorization, idempotency, unknown-query-before-retry, blocker behavior, and the best-effort dashboard trigger. Run `cd vnext && go test ./... && go vet ./...`.

**Integration:** One serial batch; production remains uninvoked. Proposed commit `feat(vnext): complete authorized deployment phase`.

### Task 5: Install, migration, and cutover

**Plan:** `docs/superpowers/plans/2026-09-19-agent-team-vnext-install-migration.md`.

**Dependencies:** Phase 1–4 CLEAN.

**Beads:** Under the captured root epic, create exactly 4 direct Phase 5 children titled `P5.01`…`P5.04`, each linked to the Phase 3 and Phase 4 CLEAN gates.

**Execution/evidence:** Verify Go artifacts/checksums, `agent-teamctl install --host codex|claude|both`, `agent-teamctl update`, `agent-teamctl rollback`, and `agent-teamctl uninstall`; record per-file ownership/checksums plus backup/rollback/uninstall outcome evidence. Prove no hooks/MCP/settings mutation, byte preservation, provenance-only v7 inventory, project opt-in, and Codex/Claude canaries with the stop-before-switch convention. Run `cd vnext && go test ./... && go vet ./...` plus native matrix.

**Integration:** Serial batch; cut over one opted-in project only after canary and rollback evidence. Proposed commit `feat(vnext): complete reversible installation phase`.

### Task 6: Release readiness

**Plan:** `docs/superpowers/plans/2026-09-19-agent-team-vnext-release-readiness.md`.

**Dependencies:** Phases 1–5 CLEAN and all native CI artifacts.

**Beads:** Under the captured root epic, create exactly 6 direct Phase 6 children titled `P6.01`…`P6.06`, each linked to the Phase 1–5 CLEAN gates.

**Execution/evidence:** Verify docs/legacy labels, deterministic artifacts/checksums/SBOM availability, baseline/raw-pointer benchmark, semver/changelog, least-privilege matrix, canary/rollback, provider verification, cutover rehearsal, non-force tag/push/release.

**Integration:** Final serial batch; proposed commit `release(vnext): prepare verified vnext release`.

## Definition of Done

All six epics are CLEAN; all commands/evidence are revision-bound; native Linux/macOS/Windows CI passes; tracker has no active release blocker; v7 remains provenance only; canary/rollback and provider live verification have passed; docs match shipped behavior; release artifact checksums verify; authorized non-force publication completes.

## Execution Handoff

This roadmap is ready for independent review. It is not a CLEAN implementation claim.
