# First-run setup

Setup prepares the selected host/project without starting development or enabling deployment. Use [settings](settings.md) for saved defaults and [dependencies](dependencies.md) for the catalog. One short recommended path is the default; the full settings wizard remains available.

## Inspect and reuse

Resolve canonical Git root/worktree metadata and the existing setup receipt. Read approved Kickoff handoff if present; otherwise adopt usable project state or prepare the requested standalone scope. Preserve the chosen tracker and integration branch. A missing optional skill never changes either.

Confirm actual Codex/Claude Code runtime from trusted metadata. Select that host by default; install both only on explicit request. Show the chosen project/user scope and what will change. Preserve per-host model choices, custom files and unrelated hooks/MCP servers.

Group inspection into four short results: host/access; runtimes/tools; selected skills; project readiness. Report Ready, Missing, Manual action or Unavailable with a concrete reason. A path or version string alone is not functional readiness.

## Prerequisite order

1. The user installs/opens a supported Codex or Claude Code host and signs in to its own account. Agent-Team cannot create an account or buy model access.
2. Verify Git and the runtime required by Agent-Team's installed release. For GitHub installation/publishing verify GitHub CLI and authentication with `gh auth status`; if sign-in is needed, the user completes `gh auth login` in the trusted local flow. Never ask for a token in chat.
3. Inspect selected tool installation requirements. Prepare their free runtimes/package managers before tools: Node/npm for Node tools; Python/uv for Serena when required by the selected release. Prefer supported binaries/runtime management over unrelated source-build toolchains.
4. Install the complete Agent-Team package for the selected host/scope and preserve handler-level ownership. Verify the extracted installation's required files and entrypoints.
5. Automatically prepare missing mandatory Serena and Microsoft Playwright CLI, then selected default profiles. Install Playwright's required browser binaries and verify a real browser operation. Backend-only projects still prepare Playwright.
6. Prepare optional Context7 or dashboard/bv only if selected. Reuse working compatible dependencies instead of upgrading on every run.
7. Verify discovery and useful operations in the actual host and relevant worktrees. Record versions, sources, capability state and configuration effects without secrets.
8. Restart/reload the host session when installed skills/hooks are not yet visible. Review the installed definitions in the host's hook trust flow; Codex uses `/hooks`. A skill cannot fabricate trust. Claude settings changes may also require reload/review according to the installed host.
9. Show a compact role/model/effort/defaults summary and the command to start authorized development. A setup-only request stops after this summary.

If an operating-system prerequisite requires administrator approval, an auth challenge, a new purchase, or overwriting a customization, explain that specific step. Do not silently bypass it. Keep successful installations and continue independent safe preparation.

## Automatic preparation policy

First use and explicit setup automatically install missing mandatory and selected default catalog items at the established scope—no one-question-per-plugin ceremony. Explain each item's purpose in a short summary and link its official source. The optional choices remain user decisions.

Inspect and pin an official released version or reviewed revision before executing installation. Record checksums when provided and verify extraction/installed behavior. Do not pipe an uninspected moving-branch script into a shell. Install shared prerequisites and mutate a shared package/config store serially.

A prepared package must include its required companion files; a copied SKILL.md alone may not be enough. Use the host's actual discovery locations and installation support. Scoped installation must not silently change the other host or a wider scope. Customized/ambiguous resources are preserved and reported.

LeanCTX is a separate narrowed profile, not a proxy. Snapshot affected configurations, merge only owned entries, inspect security-sensitive permission additions, preserve unrelated hooks/roles and verify exact-output recovery. Do not invoke broad initializers blindly. See [LeanCTX](lean-ctx.md).

## Readiness and repair

The one execution-readiness contract covers scope/acceptance, actionable tasks/dependencies, canonical branch, checks, capabilities and authority. Reuse approved decisions; ask only about a missing material decision. Project Kickoff is not a prerequisite.

Keep Beads when selected. If its backend is unavailable, diagnose/repair within authority or report that capability unavailable; never activate a temporary Markdown tracker. A selected root or designated TASKS.md is fully supported.

Ordinary install/check failures trigger bounded diagnosis, repair and recheck. Preserve already passed results and usable installations. A required capability may remain unavailable while independent work progresses; do not claim setup fully ready or pass affected acceptance until it works.

Read-only status and health do not enter setup, write a receipt, repair a cache or install anything. During authorized setup/development, repair clearly owned malformed/missing artifacts; preserve uncertain user customization.

## Defaults and later changes

Recommended setup shows current/recommended values together and accepts a grouped choice. Change one setting or role directly when requested. Run the complete wizard only when explicitly chosen. Save validated changes atomically, preserve concurrent edits and apply them to future runs only.

Authentication, administrator permission and native hook trust are separate host/user actions, not implied by a saved installation preference. Reload does not prove hooks ran: distinguish installed, registered, trusted/unknown, supported and exercised states.

## Bundled setup helpers

The orchestrator uses the complete installed [Agent-Team package](https://github.com/thebpandey/agent-team), not invented native host subcommands. Users can simply say “Agent-Team setup.” These helpers do not spawn agents or replace native trust controls.

1. Adopt approved facts with `project-initialize --project /absolute/project --request /absolute/initialization.json`. The request uses the workflow envelope from [hooks](hooks.md): `schemaVersion: 1`, actual `actorSessionId`, expected setup version and a `request` body containing `projectId`, unique `operationId`, `source` (`standalone` or `existing`), selected `tracker`, and `plan`.
2. The plan contains `scope`, nonempty `acceptance`, existing integration `branch`, nonempty `verification`, `authority.ownedPaths`, and canonical task IDs. Standalone tasks also include title/status/dependencies. Existing adoption preserves the complete selected tracker; a subset is a conflict, not permission to hide tasks. Existing Beads must already be initialized through its supported setup; this helper never invents a database or a fallback tracker.
3. `canonicalReady` means canonical records and eligible tasks exist. The CLI still reports `ready: false` until a separate scoped readiness check. It never treats initialization as dependency or native-host certification.
4. Inspect dependencies, prepare selected components, verify real host discovery, then inspect readiness. Settings belong to the canonical project; dependency scope can be user or project. Match the readiness scope to the prepared capability scope.

From the installed package, agent-run examples are:

```bash
node hooks/agent-team-cli.mjs dependencies --project /absolute/project --host codex --scope user
node hooks/agent-team-cli.mjs dependencies-prepare --project /absolute/project --host codex --scope user --request /absolute/preparation.json
node hooks/agent-team-cli.mjs readiness --project /absolute/project --host codex --scope user
node hooks/agent-team-cli.mjs settings --project /absolute/project --host codex --scope project
node hooks/agent-team-cli.mjs settings --project /absolute/project --host codex --scope project --role developer
```

For Claude Code select `--host claude-code`. Do not install both hosts by inference. A dependency preparation request has this separate setup-mutation envelope:

```json
{
  "schemaVersion": 1,
  "expectedVersion": 1,
  "operationId": "prepare-selected-components-unique-id",
  "writer": { "id": "actual-project-owner-session", "role": "project_orchestrator" },
  "request": { "selections": {} }
}
```

Use the freshly observed setup version and actual registered owner; example values are not authority. Empty selections retain mandatory/current default choices and remembered declines. Optional selections are explicit. Safe user preparation uses managed user tool/skill locations; project preparation stays in the canonical project. A linked feature worktree never becomes another configuration authority.

Native model catalogs and fresh-worker discovery are supplied only by the actual host integration to `runSetupCommand`'s context (`nativeChoices`, `workerDiscovery`). JSON request files cannot assert them. A bare Node CLI without those facts reports unknown/unavailable and must not be described as a completed native journey. Registration instructions in a preparation receipt still require an owned native registration, reload where needed, and a real fresh-worker check.

`settings-update` takes `request.change`; `dashboard-configure` takes `request.dashboard` in the same versioned setup envelope. Neither starts development. For saved HTML use `{ "snapshot": true, "graph": { "enabled": false, "termsAcknowledged": false } }`. Enabling the external graph requires the selected Beads tracker and positive upstream-terms acknowledgement; an optional configured executable must be absolute. Start the live server only through a separate explicit `dashboard-start` request. All returned conflicts/unavailable states require inspection even when the process exit code is zero.
