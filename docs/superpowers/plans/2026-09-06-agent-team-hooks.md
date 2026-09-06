# Agent-Team Cross-Runtime Hooks Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build, verify, install, and submit the approved lifecycle hook system for current Codex and Claude Code.

**Architecture:** A dependency-free Node.js shared policy core receives normalized events from thin runtime adapters. Declarative runtime hook files and an idempotent installer preserve unrelated configuration, while validators and an on-demand audit share the same manifest and state rules.

**Tech Stack:** Node.js 24 standard library, `node:test`, JSON/TOML-aware safe merge code, Git/GitHub CLI probes, GitHub Actions.

**Spec:** `docs/superpowers/specs/2026-09-06-agent-team-hooks-design.md`

## Global Constraints

- Implement all fifteen approved requirements for both current runtimes; keep `legacy/claude-v3/` archived.
- Use no new runtime dependency and never download/install tools from a hook.
- Hooks supplement reasoning and review; evidence-record presence does not prove correctness.
- Preserve scoped authorization; add no blanket user approval prompt and no Codex `permissionDecision: "ask"`.
- Store no prompts, credentials, customer data, connection strings, or raw sensitive SQL in telemetry/checkpoints.
- Keep critical enforcement narrow and document unsupported tool paths.
- Use test-first red/green cycles and safe disposable fixtures only.

---

### Task 1: Shared event model, project resolution, and recovery/checkpoints

**Files:** Create `hooks/agent-team-hook.mjs`, focused modules under `hooks/lib/`, Codex/Claude hook declarations, and `tests/hooks-*.test.mjs` fixtures.

**Interfaces:** Consume native stdin JSON plus `--runtime` and `--event`. Produce `{mode, allow, messages, context, capabilities, mutations}` for output adapters.

- [ ] Write failing tests for both payload shapes, Codex multi-file patches, nested/symlink project resolution, advisory recovery freshness, and atomic/idempotent checkpoints.
- [ ] Run focused tests and confirm failures are caused by missing production modules.
- [ ] Implement the smallest shared event/project/recovery/checkpoint modules and thin output adapters.
- [ ] Run focused tests until green and commit the task.

### Task 2: Deterministic enforcement and advisory analyzers

**Files:** Add focused policy/analyzer modules and extend hook fixtures/tests.

**Interfaces:** Consume normalized operation, canonical Agent-Team identity/state, changed content, and explicit provider mappings. Produce advisory findings or enforcement denials without approval prompts.

- [ ] Write failing tests covering requirements 2–7, 10, and 11, including authorized-pass, stale/missing-block, false positives, dedupe, batching, timeouts, and unsupported-path labels.
- [ ] Run the tests and confirm expected failures.
- [ ] Implement ownership, integration/release, destructive-database, and explicit-completion gates plus test/migration/lint/cost advisories.
- [ ] Run focused and aggregate tests until green and commit the task.

### Task 3: Activation telemetry, effectiveness audit, health, installer, and validators

**Files:** Add telemetry/audit/health/install/rollback/package/artifact command modules, `hooks/manifest.json`, regression fixtures, and `.github/workflows/check-package.yml`.

**Interfaces:** `node hooks/agent-team-cli.mjs <health|audit|install|uninstall|check-package|check-artifacts>` with structured status and nonzero exits for validation failures.

- [ ] Write failing tests for logging, rotation, permissions, concurrency, malformed data, audit limits, Codex degraded activation, safe config merge/backups/reruns/duplicate prevention, validators, and archive defects.
- [ ] Run focused tests and confirm expected failures.
- [ ] Implement the command surface, manifest, installer/rollback, validators, and CI workflow without adding dependencies.
- [ ] Run focused and aggregate tests until green and commit the task.

### Task 4: Documentation, version, package, live fixtures, and installation

**Files:** Modify `SKILL.md`, `README.md`, `CHANGELOG.md`, platform references, setup/package docs, and any generated package manifest data.

**Interfaces:** Document every requirement's shared policy, runtime mapping, mode, paths, blind spots, failure behavior, dependencies, tests, and installation location.

- [ ] Add documentation consistency tests before changing the documents and confirm they fail.
- [ ] Update documentation and semantic version, record upstream inspiration revision/license, and keep current/legacy packaging boundaries explicit.
- [ ] Run full unit, validator, intentionally broken regression, and safe installed-runtime fixture checks.
- [ ] Commit, obtain independent whole-branch review, repair findings through focused tests, and rerun full verification.
- [ ] Build and verify artifacts, create backups, install both present runtimes, preserve unrelated configuration, and report trust/exercise status accurately.
- [ ] Push `codex/agent-team-hooks`, create a private draft PR, and leave `main` unchanged.
