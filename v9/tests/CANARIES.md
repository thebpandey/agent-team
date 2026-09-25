# v9 core canaries

Run these in disposable Git projects only. They are fixture behavior checks, not text-grep tests. Record the date, host, `bd version`, command output, exit code, and content-aware before/after file inventory with each run. The evidence below was captured with Beads `1.2.2 (6c124203e)` on 2026-09-25.

## RED baseline: installed v8 has extra admission gates

Create two throwaway Git repositories: `no-beads` with no `.beads`, and `empty-beads` initialized only with `bd init --skip-hooks --skip-agents --non-interactive --init-if-missing`. In both, commit a pre-existing `.gitignore` containing `# user-owned ignore rule` and `*.local`; never run this against a user project.

Use the managed binary exactly: `/home/server/.config/agent-team/bin/agent-teamctl`. Its `--version` probe returned `phase rejected` with exit 2; the paired installed v8 skill metadata identifies 8.0.15. Its setup instructions offer one selected dependency bundle: Beads, Serena, Graphify, `rg`, `ast-grep`, and LeanCTX.

```text
CWD=/tmp/agent-team-v9-v8-managed.TeoTWG/no-beads
$ /home/server/.config/agent-team/bin/agent-teamctl status --host codex --json
{"schema":1,"action":"status","status":"rejected","message":"phase rejected"}
exit 2

$ /home/server/.config/agent-team/bin/agent-teamctl setup --host codex --json
status=needs_input; missing=TASKS.md or .beads, DECISIONS.md, AGENT_TEAM_RULES.md
exit 1; content-aware before/after diff=0

CWD=/tmp/agent-team-v9-v8-managed.TeoTWG/empty-beads
$ /home/server/.config/agent-team/bin/agent-teamctl status --host codex --json
{"schema":1,"action":"status","status":"rejected","message":"phase rejected"}
exit 2

$ /home/server/.config/agent-team/bin/agent-teamctl setup --host codex --json
status=needs_input; missing=DECISIONS.md, AGENT_TEAM_RULES.md
exit 1; content-aware before/after diff=0
```

The inventory hashes every regular file outside `.git` as `sha256 path`; both status/setup comparisons were byte-identical. In `empty-beads`, the pre-existing `.gitignore` bytes remained as a prefix after the deliberate Beads initialization, followed only by Beads' documented stanza. The v8 baseline is therefore real: no-Beads needs tracker choice/approval and v8 additionally asks for project records and offers its controller/bundle path. No v9 package was installed under `/home/server/.agents/skills` at capture time.

## GREEN canary A: inspection is authority-read-only

1. Make `no-beads`, run `git init`, add and commit the pre-existing `.gitignore` fixture, then capture a content-aware inventory: regular-file path plus SHA-256, excluding `.git/`.
2. Request v9 `status` in that project. It must report missing Beads without running `bd init`; recapture the inventory and require it to match exactly.
3. In `empty-beads`, snapshot immediately after `bd init`; the first inspection command must be `bd --readonly status --json`. Require exit 0 and valid JSON. Compare the full content-aware inventory: the only allowed new path is exactly `.beads/embeddeddolt/<project>/.dolt/temptf/dolt_embedded_metrics`, an empty Beads/Dolt housekeeping file. Require Git state unchanged across inspection and no changed task/database content; then repeat `bd --readonly status --json` and require an identical inventory.
4. Confirm no optional tool is installed, configured, or required by either status result.

Pass only if both inspections are read-only at the Agent-Team/project/task-authority boundary: no Agent-Team file, task/database mutation, or Git change; the listed cold-open metrics artifact is the sole bounded Beads-owned housekeeping exception. A missing `.beads` directory is a status result, not approval to initialize it.

## GREEN canary B: approved first run is Beads-only and selection is bounded

1. In a fresh disposable Git repository with no `.beads`, add and commit the pre-existing `.gitignore` fixture. Request v9 `setup` or `start`, decline the approval once, and require a byte-identical inventory.
2. Repeat and explicitly approve. Require the only initialization command to be:

   ```sh
   bd init --skip-hooks --skip-agents --non-interactive --init-if-missing
   ```

3. Run `bd --readonly status --json`; require exit 0 and valid JSON. Compare the complete content-aware inventory with the pre-approval snapshot. With Beads 1.2.2, only `.beads/` (including exactly the cold-open Beads/Dolt metrics artifact above) and the Beads-owned append to the existing root `.gitignore` are allowed; the original `.gitignore` content must remain byte-for-byte as its prefix. Its new stanza begins `# Beads / Dolt files (added by bd init)`. Require Git state unchanged across inspection and no task/database mutation. No other project file or content change is allowed: in particular, no `.agent-team/`, `TASKS.md`, ledger, hook, setting, dashboard, or optional-tool file. Snapshot again and repeat `bd --readonly status --json`; that inspection must be byte-stable.
4. Create 21 independent ready Beads issues in this disposable project. Run:

   ```sh
   bd ready --limit 20 --json | jq 'length'
   ```

   Require `20`. Give the orchestrator only those rows. This Task 1 draft must not claim or dispatch any issue.

Pass only if initialization followed explicit approval, the mutation budget was Beads-only, pre-existing ignore rules were preserved, optional-tool absence did not block readiness, and ready selection was capped at 20 rows.

## Scope of this evidence

This session cannot reload the uninstalled v9 draft into a live Codex or Claude host. These GREEN checks validate Task 1's Beads CLI semantics in fixtures; they do not claim a live v9 skill invocation, dispatch, or host activation. The required native Codex/Claude live-host invocation remains a deferred core-acceptance canary.

## Actual Beads 1.2.2 command checks

The Task 1 author ran `bd version` and observed:

```text
bd version 1.2.2 (6c124203e: HEAD@6c124203e771)
```

In the disposable no-Beads project, `bd --readonly status --json` exited 1 with `Error: no beads database found` and a `bd init` hint; its content-aware inventory did not change. In the initialized empty project, the first `bd --readonly status --json` returned a JSON object and created only the exact empty Beads/Dolt internal metrics path; a repeated readonly status left the full inventory unchanged. Twenty-one independently created ready issues yielded 20 JSON rows from `bd ready --limit 20 --json`. The approved first-run sequence created Beads state and appended its marked stanza to the pre-existing root `.gitignore`; it preserved the original ignore-rule bytes, task/database contents, and Git state. Re-run the two fixture canaries and the deferred live-host canary before calling a v9 release candidate verified.
