# Agent-Team

Created by [thebpandey](https://github.com/thebpandey). Agent-Team coordinates bounded development in Codex and Claude Code with isolated writers, independent review, verified integration, durable recovery, and explicit release authority.

The Node-based **v7.3.1** package described in older sections is legacy during the vNext transition. For release history, see the [changelog](CHANGELOG.md); for the compact operator walkthrough, see [Getting Started](GETTING_STARTED.md).

## vNext native transition

Install one checksum-verified native package and select the host explicitly: `agent-teamctl install --host codex|claude|both`. The same package supports Codex and Claude on Windows, macOS, and Linux; switching hosts is a foreground action and transfers no lease or worker identity. `rollback --version <version>` selects a unique release backup; add `--revision <commit>` when that version has multiple revisions. Rollback and `uninstall` touch only exact manifest-owned bytes and retain changed or unknown files.

The public lifecycle is `setup`, `status`, `start`, `task add`, and `one-off`, plus scoped `pause`, `stop`, `cancel`, and `resume`. FIX/CLEAN is an internal independent-review loop, not a public `review` command. Beads is the live tracker after an approved cutover; a pre-cutover `TASKS.md` is legacy provenance only. `BLOCKERS.md` and `DECISIONS.md` are bounded human-readable projections, never competing task stores.

Existing v7 projects and hosts use the explicit native transaction `agent-teamctl cutover --request /absolute/path.json`. Project requests bind the closed Beads export, exact Git revision, independently authored CLEAN review, test/readiness digests, remote observation, user authorization cause, and recovery disposition. The first project pass remains held; a separate `reconcile` request must match the recorded cause and receipt digest. Host requests are `host-cutover`, `host-status`, or `host-rollback`; they require the exact schema-4 v7 install receipt and preserve a durable rollback receipt. Unknown, modified, or ambiguous v7 files and handlers are retained and fail closed.

The dashboard is local-only and read-only. Capacity caps remain enforced. Optional Serena, Graphify, LeanCTX, browser, and visual tools have bounded native fallbacks; absence never widens authority. See the [honest benchmark report](docs/benchmarks/vnext-optional-8.0.0.md) and the revision-bound release checks before treating a candidate as published.

![Agent-Team overview: one orchestrator coordinates bounded implementation, independent review, integration, and release.](assets/guide/agent-team-essence-16x9.webp)

## What Agent-Team does

Agent-Team keeps one project orchestrator responsible for scope, task admission, coordination, integration, and release. Developers work on bounded assignments in separate worktrees. Independent reviewers verify exact revisions. Ordinary in-scope failures return to the owning developer for repair; they do not become repeated permission prompts.

One selected Beads tracker or one root/designated `TASKS.md` owns task state. Checkpoints, dashboards, setup receipts, plugins, and LeanCTX never become competing task stores. Unknown readiness, liveness, trust, or model availability stays unknown.

```mermaid
flowchart LR
  A["Approved scope and tracker"] --> B["Setup and readiness"]
  B --> C["Admit safe tasks"]
  C --> D["Isolated developers"]
  D --> E["Independent review"]
  E -->|repair| D
  E -->|accepted| F["Serial integration"]
  F --> G["Evidence and task state"]
  G -->|eligible work| C
  G --> H["Authorized release"]
```

## Quick start

### 1. Install the package

Ask the active host to install the complete package from the official repository. Choose the actual host and scope explicitly; installing for both hosts is never inferred.

```text
Install Agent-Team 7.3.1 from https://github.com/thebpandey/agent-team.
Use the managed installer for this host and user scope unless I explicitly choose project scope.
Inspect the source, license, existing installation, and affected hook configuration first.
Preserve custom files, role definitions, hooks, MCP servers, and unrelated settings.
Verify the installation receipt and tell me which reload or native trust step remains.
```

| Scope | Codex | Claude Code |
| --- | --- | --- |
| User | `~/.agents/skills/agent-team` | `~/.claude/skills/agent-team` |
| Project | `.agents/skills/agent-team` | `.claude/skills/agent-team` |

Project hooks use `.codex/hooks.json` or `.claude/settings.local.json`; user hooks use `~/.codex/hooks.json` or `~/.claude/settings.json`. Restart or reload the selected host after installation and review its native hook-trust UI. A skill cannot grant native trust to itself.

### 2. Run setup

Invoke `$agent-team setup` in Codex or `/agent-team setup` in Claude Code. Setup:

1. resolves the canonical project and selected tracker;
2. reuses an approved Project Kickoff handoff, an existing plan, or a bounded standalone task;
3. prepares selected companion capabilities;
4. reports installed, detected, functional, and fresh-worker observations separately;
5. shows the current-effective settings wizard; and
6. reports dispatch readiness without starting development.

Project Kickoff is optional. If you use it, install its current supported package from the [latest release discovery page](https://github.com/thebpandey/project-kickoff/releases/latest). Compatibility is an exact recorded handoff contract, not a guess based on version ordering.

### 3. Start work

```text
$agent-team start
$agent-team start 2
$agent-team start 2 continuous
$agent-team start 2 continuous auto-deploy 8
$agent-team start email-preferences with-preview
```

Use `/agent-team ...` for the same actions in Claude Code. A named start selects an already-defined tracker item. A full natural-language feature request may create one deduplicated canonical task only after scope and acceptance are sufficient.

## Switch between Codex and Claude Code

Host and session switching is deliberately uneventful:

```text
$agent-team setup
/agent-team setup
```

Open the same Git checkout in the other native host or a new session and continue. Re-running setup is safe and preserves the tracker, task IDs, settings, active run, claims, checkpoints, pending operations, and evidence. No release command, request file, epoch migration, owner recovery, or prior-session cooperation is required. Recorded host/session/epoch fields are audit provenance only.

The current session must still be genuine runtime metadata for Codex or Claude Code and must resolve to the same Git project. This prevents request JSON from impersonating a host; it is not a coordinator lock. Existing writers with unknown liveness remain occupied so a new session cannot create concurrent writes accidentally.

`takeover` remains a compatibility alias for older instructions but performs no ownership transfer. Legacy owner history or a stale recovery journal does not block ordinary setup, task, run, checkpoint, integration, or release operations. Model routing is independent: if a requested model is unavailable, Agent-Team reports the actual host/account constraint without rewriting role choices.

See [actions](references/ACTIONS.md), [project continuity](references/PROJECTS.md), [recovery](references/RECOVERY.md), and the [hook contract](references/HOOKS.md).

## Commands

| Action | Result |
| --- | --- |
| `help` | Show the current command card without setup or mutation. |
| `setup` | Prepare capabilities, review settings, and report readiness. |
| `takeover` | Compatibility alias: verify this native session is in the same project, then continue. |
| `settings` | Show or change targeted future-run defaults. |
| `start [N] [continuous]` | Admit existing eligible tasks within safe capacity. |
| `start <name> [with-preview]` | Start one resolved tracker item. |
| `auto-deploy [B or off]` | Change release batching for the active run only. |
| `status [name-or-ID or all]` | Read recorded state without checks or mutation. |
| `pause [name-or-ID or all]` | Checkpoint and hold selected work without discarding it. |
| `resume [name-or-ID or all]` | Recover selected work from durable evidence. |
| `pause and deploy` | Pause first, then release only eligible verified work. |
| `approve <name-or-ID>` | Approve the exact submitted preview revision. |

Settings persist for future runs. Explicit start modifiers affect the current run. Team count is development capacity, not raw agent slots; reviewer capacity and host limits may reduce concurrency. Deployment batch size counts completed top-level tasks, not commits or subtasks.

## Readiness and bundled capabilities

A path or version string is not readiness. Setup records each stage independently:

| Stage | Meaning |
| --- | --- |
| Installed | Expected files or executable are present. |
| Detected | The selected host can resolve the exact capability. |
| Functional | A useful bounded operation passed. |
| Available to worker | A fresh assigned worker used the actual scoped path successfully. |

Serena and Microsoft Playwright CLI are selected defaults, not universal dispatch gates. Other selected defaults include ast-grep, Graphify, LeanCTX, focused Superpowers procedures, Ponytail, Impeccable, and React Best Practices where applicable. Beads is prepared only when it is the selected tracker. Context7 and the optional external graph remain opt-in. Only capabilities explicitly listed by the active plan in `requiredCapabilities` can block that plan.

Serena can pass its direct MCP symbol probe while a just-created worker still needs a host reload to inherit the tool; that observation is reported as `unknown`, not treated as global failure. Browser qualification runs the exact selected `playwright-cli`; a bare `import("playwright")` inside a Node REPL is a different adapter and does not qualify or disqualify the CLI. Graphify evidence is optional: deterministic writable-path overlap checks remain authoritative when no graph evidence exists.

LeanCTX qualification uses process-local state and a target inside the assigned worktree. It does not widen the read jail with temporary roots. Impeccable uses an exact pinned guidance tree and bounded companion-file checks. Dependency preparation preserves compatible unowned installations and refuses to overwrite customized or ambiguous files.

Fresh workers load the complete applicable skill instructions and only the references needed for their assignment. A parent receipt is not proof that a child read them.

## Coordination and recovery

![Agent-Team execution flow across the host, worktrees, evidence, and release gates.](assets/guide/agent-team-harness-flow-3x4.webp)

The canonical project keeps stable team IDs, exact worktree/branch assignments, task claims, checkpoints, and evidence pointers. Feature writers never share a writable path. Feature changes occur in worktrees and verified revisions integrate serially.

Continuous mode refills only proven-free capacity with eligible in-scope tasks. Unknown writers remain occupied. A scoped blocker parks only affected work while safe independent work continues. Status and help are read-only and do not silently resume a run.

Accepted integration evidence queues completed top-level, nondeployed deliveries in integration order, including recovered completions already scoped into the run. An incomplete top-level delivery fails closed; subtasks and epics never enter the deployment queue.

## Release safety

Integration, publication, and deployment are separate facts. A release requires the exact authorized target, revision, task set, checks, review, preview disposition when applicable, recovery evidence, and clean delta. Recorded session provenance does not make release authority session-exclusive. Forced, chained, stale, held, mismatched, or multi-ref pushes remain denied.

Agent-Team 7.3.1 permits an explicitly authorized manual release record to cover its named deployment-triggering non-force `origin` push to `refs/heads/main` only when every ordinary integration and release gate matches.

## Official archive installation

Download `agent-team-7.3.1.zip` and the matching one-entry `SHA256SUMS` from the [Agent-Team v7.3.1 release](https://github.com/thebpandey/agent-team/releases/tag/v7.3.1), or discover the current package through [releases/latest](https://github.com/thebpandey/agent-team/releases/latest).

```bash
(cd /absolute/download && sha256sum -c SHA256SUMS)
node /absolute/extracted/agent-team/hooks/agent-team-cli.mjs install \
  --archive /absolute/download/agent-team-7.3.1.zip \
  --checksums /absolute/download/SHA256SUMS \
  --host codex \
  --scope user
```

Use `--host claude-code` or an explicitly authorized `--host both`. Project scope also requires `--scope project --project /absolute/project/root`. On macOS, use `shasum -a 256` on the same downloaded archive/checksum pair.

Automatic archive installation is qualified on Linux and WSL only when `/proc/self/fd` descriptor-root traversal works. Otherwise it returns `unsupported_platform`, `changed: false` before mutation. An identical reinstall changes nothing. A differing present target returns `update_requires_manual_replacement`, `changed: false`; replacement requires authorized quiescence and a rollback-backed move before a fresh absent-target installation.

The sealed `.agent-team-source.json` and schema-4 receipt bind archive identity, checksums, source revision, complete file maps, modes, sizes, host, scope, targets, roles, handlers, conflicts, backups, and recovery. Installation still does not prove native trust, event execution, dependency functionality, or worker discovery.

## Source and verification

The official source is [thebpandey/agent-team](https://github.com/thebpandey/agent-team). Release identity is [v7.3.1](https://github.com/thebpandey/agent-team/releases/tag/v7.3.1). Licensed packages should remain outside application commits.

From a source checkout:

```bash
node --test tests/hooks-*.test.mjs
node hooks/agent-team-cli.mjs check-package --source .
node hooks/agent-team-cli.mjs build-artifacts --source . --revision <full-commit-id> --output ../agent-team-artifacts
node hooks/agent-team-cli.mjs check-artifacts --revision <full-commit-id> --archive ../agent-team-artifacts/agent-team-7.3.1.zip
sha256sum ../agent-team-artifacts/agent-team-7.3.1.zip
```

Portable tests cover package contracts and shipped Node consumers. Native registration, account/model access, trust, provider behavior, and real browser operations remain separately reported host facts; they block only work that explicitly requires them.
