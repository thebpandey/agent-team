# Codex handoff: finish the Agent-Team 7.1.0 open items

Date: 2026-09-10. Repository: `/home/server/dev/skills/agent-team` (the Agent-Team skill source, Node 24 ESM, `node --test`). Integration branch: `main`. Canonical tracker: root `TASKS.md`. Previous orchestrator: a Claude Code session (hook session id `6833fc57-f363-4505-b761-31867573dd93`), now ended.

Use `$agent-team` in this Codex session and read `SKILL.md` plus `references/platform-codex.md`, `references/team.md`, `references/recovery.md`, `references/hooks.md` and `references/release.md` before acting. The 7.1.0 rules apply to you: you orchestrate, developers implement, and a `gpt-5.6-sol` verifier at `medium` effort performs every pre-dispatch check and every post-completion verification. Do not implement or verify in the orchestrator context.

## What changed (already committed on main)

- `e04a84a` feat: pure orchestrator, delegated verifier route and Graphify (v7.1.0). 24 files, +376/-44.
- `4890038` docs: record AT-09..AT-12 evidence and propose AT-13 completion-gate writer.
- Both sit on top of the user's own `35bebd3` (gitignores `.agent-team/`, `.serena/`, `.playwright-cli/`).

Substance of the change set:

1. **AT-09 LeanCTX profile** (`references/lean-ctx.md`): `shell_security = "warn"` and `shell_allowlist_extra = ["cmp"]` with rationale. Also applied to the user's live config `~/.config/lean-ctx/config.toml` (backup `config.toml.bak-2026-09-10`); heredocs, `node -e`, `python3 -c` and `cmp` now run with a logged warning. Codex sandboxes may not see this hook at all; do not touch the config again.
2. **AT-10 delegated verifier route**: `references/platform-claude.md` (section "Delegated verification": Codex plugin companion `task --model gpt-5.6-sol --effort medium`, fallback `claude-opus-5` high), `references/platform-codex.md` (role table row: verifier `gpt-5.6-sol` medium), `SKILL.md`, `references/team.md` ("Orchestrator conduct"), `references/visual-review.md`, `references/acceptance-tests.md`, `references/release.md`, `references/recovery.md`, `references/projects.md`, `references/ui.md`, README.
3. **AT-11 orchestrator conduct**: same files. Key sentences: the orchestrator plans with the user, decides within authority, derives the parallel set (no unmet dependencies, disjoint writable paths), supervises continuously, never pauses on worker messages, never edits or verifies itself. Routine helper state checks (setup receipts, readiness, status, recovery packets, cleanup eligibility) are orchestration; verification of work always goes to the verifier.
4. **AT-12 Graphify**: `hooks/lib/dependency-catalog.mjs` entry `graphify` (default, PyPI `graphifyy` 0.9.57, uv-tool, python 3.12, prerequisite `uv`, `functionalCheck: "code-graph-traversal"`); `hooks/lib/dependencies.mjs` (`graphifyFunctional` with exact-label id resolution, EXTRACTED-only edges, three fixture edges, full path chain, budget threading; `TOOL_IDS`; `selectInstructions` adds `graphify` for developer, complex_developer, reviewer and project_orchestrator on non-text tasks); tests in `tests/hooks-dependencies.test.mjs` (fake executable with INFERRED and missing-edge negatives) and `tests/hooks-dependencies-real.test.mjs`; new `references/graphify.md`; catalog rows in `references/dependencies.md`, README, GETTING_STARTED; manifest entry. Profile: `graphify extract . --code-only --no-viz` per worktree, `graphify update .` to refresh, never `graphify install`/`hook`/`claude`/`codex` installers, no semantic backends, MCP (`graphify-mcp`) only on explicit selection.
5. **Version 7.1.0**: `hooks/manifest.json`, `SKILL.md` metadata, README banner and version paragraph, CHANGELOG entry, `tests/hooks-artifacts.test.mjs` pins. The manifest also gained the two `assets/guide/agent-team-banner-ultrawide.*` files that `a46a272` had added without listing.

Verification recorded for `e04a84a`: `node --test tests/hooks-*.test.mjs` 502 tests, 490 pass, 0 fail, 12 opt-in skips; `AGENT_TEAM_REAL_DEPS=1 AGENT_TEAM_REAL_SERENA=1 AGENT_TEAM_REAL_LEAN_CTX=/home/server/.local/bin/lean-ctx AGENT_TEAM_REAL_BD=/home/server/.local/bin/bd node --test tests/hooks-dependencies-real.test.mjs` 13 pass, 0 fail, 0 skip; `node hooks/agent-team-cli.mjs check-package --source .` passed; docs contracts 16/16. Independent review: five `gpt-5.6-sol` medium passes through the Codex plugin companion, 17 findings repaired, final verdict ACCEPT. Note that `AGENT_TEAM_REAL_BD` and `AGENT_TEAM_REAL_LEAN_CTX` take executable paths, not `1`.

Host state on this machine: the user-scope Claude Code skill copy at `~/.claude/skills/agent-team` is 7.1.0 (installed with no conflicts). Managed tools under `~/.agent-team/tools/bin`: uv 0.12.10, serena 1.7.0, playwright-cli 0.1.19, sg/ast-grep 0.45.3, impeccable 4.1.0, graphify 0.9.57. Serena is registered as a user-scope Claude MCP server pinned to this project path. The Claude-host setup receipt (`.agent-team/setup.json`, version 6) shows 7 of 10 catalog items ready; lean-ctx, superpowers and ponytail are `cannot_use` by design (existing user copies preserved). No Codex-host dependency receipts exist for this project yet.

## Pending items, in order

### 0. Take ownership before writing

`.agent-team/` is local, git-ignored state. `TEAMS.md` there names the ended Claude session as project and integration owner, and the Agent-Team hooks deny file changes to any unregistered session. Do this first:

1. Run `$agent-team status` (read-only) and then `$agent-team resume`, which enters `references/recovery.md`. The recorded writer identity belongs to a short-lived CLI process that has exited, so the stopped-writer check should succeed and allow the ownership handoff.
2. If ownership cannot be established that way, delete the local `.agent-team/` directory (it holds no shared authority) and re-initialize with `node hooks/agent-team-cli.mjs project-initialize --project /home/server/dev/skills/agent-team --request /absolute/initialization.json`, using YOUR Codex hook session id as `actorSessionId` and this request body:

```json
{
  "schemaVersion": 1,
  "actorSessionId": "<your codex hook session id>",
  "expectedVersion": 0,
  "request": {
    "projectId": "agent-team",
    "operationId": "init-agent-team-<date>-<session>",
    "source": "existing",
    "tracker": { "kind": "markdown", "path": "TASKS.md" },
    "plan": {
      "scope": "Agent-Team skill package: TASKS.md AT-01..AT-13.",
      "acceptance": [
        "node --test tests/hooks-*.test.mjs passes with zero failed, cancelled or skipped tests",
        "node hooks/agent-team-cli.mjs check-package --source . reports passed",
        "Delegated verifier ACCEPT before a task is marked verified"
      ],
      "branch": "main",
      "verification": [
        "node --test tests/hooks-*.test.mjs",
        "node hooks/agent-team-cli.mjs check-package --source ."
      ],
      "authority": { "ownedPaths": ["SKILL.md", "README.md", "CHANGELOG.md", "GETTING_STARTED.md", "TASKS.md", "references", "hooks", "assets", "agents", "tests", "docs", ".github"] }
    }
  }
}
```

3. Prepare the Codex host: `node hooks/agent-team-cli.mjs dependencies-prepare --project <root> --host codex --scope user --request <envelope>` with empty selections, then bind fresh-worker discovery through the exported `runCommand` driver described in `references/setup.md` ("Bind the native observations"), then `readiness --host codex --scope user`. Reuse the tools already under `~/.agent-team/tools`; the runner reuses compatible installations.

### 1. AT-13: completion gate has no supported evidence writer (authorized by this handoff)

Problem: `hooks/lib/policy.mjs` `completionGate` denies marking a TASKS.md row `verified` unless `state.completion` carries `evidenceRevision === HEAD`, `taskId`, `requirementsReconciled === true`, `review { status: "passed", revision }` and `checks[] { name, status: "passed", revision }`. The only writer, `recordGateEvidence` in `hooks/lib/task-transitions.mjs` (`gate-evidence` CLI), stores `recordedEvidence` and nothing the gate reads. Initialization seeds `{ requirementsReconciled: false, checks: [] }`. Result: under an active project no task can ever reach `verified` without hand-editing `state.json`, which `references/hooks.md` forbids.

Deliver: a supported mapping from a `gate-evidence` completion artifact to those fields (for example, the evidence JSON carries `review`, `checks` and `requirementsReconciled`, validated for the exact revision and task ids, and `recordGateEvidence` writes them under `state.completion` alongside `recordedEvidence`), a regression test in `tests/hooks-transitions.test.mjs` and `tests/hooks-policy.test.mjs` proving a row can be flipped only after such evidence for HEAD, and a short documentation update in `references/hooks.md`. Keep the "one task id per completion" rule and the clean-tree rule. Route the implementation to a developer and the review to the verifier.

Then, for AT-09, AT-10, AT-11, AT-12 and AT-13, record gate evidence at HEAD and flip each row to `verified` one row per commit (the classifier accepts exactly one newly verified id per change and the gate requires a clean tracked tree).

### 2. Push and tag the release

Authority: TASKS.md line 5 records the user's approval for push to `origin/main` and GitHub distribution after successful verification, and the user's 2026-09-10 handoff request. Before pushing, rerun the full suite and the real opt-in file exactly as above and `check-package`. Then `git push origin main`, tag `v7.1.0` on the verified revision and push the tag; the release workflow (`.github/workflows/release.yml`) builds and checks the archive. Afterwards download the published ZIP and validate it with `check-artifacts --revision <commit> --archive <zip>`, as AT-08 did for 7.0.1. Record release identity and evidence in TASKS.md.

### 3. Remove the last opt-in skip

`tests/hooks-playwright-companion.test.mjs` skips "actual integrity-pinned npm companion copies completely without executing package code" unless `AGENT_TEAM_TEST_PINNED_PLAYWRIGHT_PACKAGE` points at a pinned package. Inspect the test to learn the expected form (a local `npm pack` tarball of `@playwright/cli@0.1.19` is the likely answer), produce it under a temp directory, run the file with the variable set, and record the result. The 7.0.x release baseline was zero skips; 7.1.0 should match.

### 4. Refresh the user-facing guide for 7.1.0

`agent-team-guide-v7.0.2.html`, `Getting_Started_with_Agent-Team.html`, `index.html` and the README hero links still document the 7.0.2 workflow (the README banner already says so explicitly). Produce a 7.1.0 guide (delegated verifier route, pure orchestrator conduct, Graphify next to Serena, LeanCTX warn profile), update the manifest `rootFiles`/`files` lists and the `tests/hooks-docs.test.mjs` expectations if they reference guide filenames, keep the 7.0.2 guide reachable as history, and re-run `check-package`.

### 5. Small follow-ups

- Add `graphify-out/` to this repository's `.gitignore` (developer task under project authority, per `references/graphify.md`).
- Serena's user-scope MCP registration is pinned to `--project /home/server/dev/skills/agent-team`; if the user works on other repositories from Claude Code, switch it to `--project-from-cwd` or a per-project registration. This is a user decision; ask once.
- Local `.serena/` in this checkout is Serena's project config, now git-ignored; leave it.

## Rules for this session

- One writer per file; developers work in feature worktrees under `.worktrees/`, integration is serial into `main`.
- Every review and every final check goes to `gpt-5.6-sol` medium. Findings return to the owning developer for automatic in-scope repair; do not ask the user to authorize routine repair.
- Never claim a check that did not run. Report implemented, verified, integrated and released as separate states.
- Ask the user only for genuinely missing authority: nothing above needs it except the Serena registration choice.
