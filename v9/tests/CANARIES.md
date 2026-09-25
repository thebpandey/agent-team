# v9 core canaries

Run these in disposable Git projects only. They are live behavior checks, not text-grep tests. Record the date, host, `bd version`, command output, exit code, and changed-path list with each run. The evidence below was captured with Beads `1.2.2 (6c124203e)` on 2026-09-25.

## RED baseline: installed v8 has extra admission gates

Create two throwaway Git repositories: `no-beads` with no `.beads`, and `empty-beads` initialized only with `bd init --skip-hooks --skip-agents --non-interactive --init-if-missing`. Never run this against a user project.

Observed installed skill: `/home/server/.agents/skills/agent-team/SKILL.md` identifies v8.0.15. Its setup instructions offer one selected dependency bundle: Beads, Serena, Graphify, `rg`, `ast-grep`, and LeanCTX; its native routing sends `setup` and `status` to `agent-teamctl`. In this environment the installed controller was unavailable:

```text
$ agent-teamctl status --json
/bin/bash: agent-teamctl: command not found
exit 127

$ agent-teamctl setup --json
/bin/bash: agent-teamctl: command not found
exit 127
```

The v8 skill consequently reports installation-needed rather than a Beads-only ready result. This is the failing baseline: no-Beads cannot become ready without approval, and v8 additionally depends on its controller/bundle path. No v9 package was installed under `/home/server/.agents/skills` at capture time. The command failures did not create `.beads` in `no-beads` or alter `empty-beads` beyond its deliberate Beads initialization.

## GREEN canary A: inspection does not mutate

1. Make `no-beads`, run `git init`, and capture `find . -mindepth 1 -maxdepth 3 -print | sort`.
2. Request v9 `status` in that project. It must report missing Beads without running `bd init`; recapture the file list and require it to match exactly.
3. In `empty-beads`, capture its file list and run `bd status --json`. Require exit 0, valid JSON, and an identical file list before and after.
4. Confirm no optional tool is installed, configured, or required by either status result.

Pass only if both inspections are write-free. A missing `.beads` directory is a status result, not approval to initialize it.

## GREEN canary B: approved first run is Beads-only and selection is bounded

1. In a fresh disposable Git repository with no `.beads`, request v9 `setup` or `start`. Decline the approval once; require no changed paths.
2. Repeat and explicitly approve. Require the only initialization command to be:

   ```sh
   bd init --skip-hooks --skip-agents --non-interactive --init-if-missing
   ```

3. Run `bd status --json`; require exit 0 and valid JSON. Compare paths with the pre-approval snapshot: every new path must have been created by Beads. With Beads 1.2.2 this includes `.beads/` and its root `.gitignore` (which says `# Beads / Dolt files (added by bd init)`); no other project file is allowed. In particular, no `.agent-team/`, `TASKS.md`, ledger, hook, setting, dashboard, or optional-tool file is allowed.
4. Create 21 independent ready Beads issues in this disposable project. Run:

   ```sh
   bd ready --limit 20 --json | jq 'length'
   ```

   Require `20`. Give the orchestrator only those rows. This Task 1 draft must not claim or dispatch any issue.

Pass only if initialization followed explicit approval, the mutation budget was Beads-only, optional-tool absence did not block readiness, and ready selection was capped at 20 rows.

## Actual Beads 1.2.2 command checks

The Task 1 author ran `bd version` and observed:

```text
bd version 1.2.2 (6c124203e: HEAD@6c124203e771)
```

In the disposable no-Beads project, `bd status --json` exited 1 with `Error: no beads database found` and a `bd init` hint; it did not initialize the project. In the initialized empty project, `bd status --json` returned a JSON object without changing the file list; 21 independently created ready issues yielded 20 JSON rows from `bd ready --limit 20 --json`. The approved `bd init` created Beads state plus the Beads-marked root `.gitignore`, and nothing else. Re-run the two GREEN canaries before calling a v9 release candidate verified.
