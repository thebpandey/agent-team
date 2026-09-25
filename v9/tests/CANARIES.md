# v9 core canaries

## Distribution cutover evidence

The cutover canary is defined for disposable projects only. Read the v8 manifest and model every host/path it lists; current v8 uninstall is global and has no host selector. Its active/uncertain-worker case must stop before any cutover mutation when any listed host has uncertain work. Its idle case backs up every listed skill root and shared manifest-owned file, verifies preservation of unowned project state, uninstalls once, installs v9 for all intended hosts, checks the selected skill roots, and reads `bd --readonly status --json`. Never run this canary against a live user project or host skill root.

If a verified v8 package and real managed uninstaller are unavailable, a disposable manual install/rollback smoke may exercise dual-host v9 installation and project-state preservation only. Label it partial/manual: do not call it a v8 managed-uninstall cutover pass.

Evidence status: candidate CI passed the POSIX and PowerShell installer checks; native Windows CI run `36106466674` passed at revision `ea749d9`, closing installer test Bead `atv-uns.7`. This is installer fixture evidence only, not a v8 managed-uninstall result. After the separately authorized Linux force-removal cutover and LeanCTX removal, a fresh Codex disposable task completed the native developer → independent CLEAN reviewer → integration → Beads closure path at `3340f586991ab629b090c7af9a2c31dc7de16866`. Fresh Codex and Claude status commands returned Beads data without setup or dispatch; Claude native worker acceptance is still unverified. No Windows or managed-v8-uninstall cutover pass is asserted here; the destructive live route has no rollback claim. See [the release ledger](../../docs/releases/9.0.0-readiness.md).

Run these in disposable Git projects only. They are fixture behavior checks, not text-grep tests. Record the date, host, `bd version`, command output, exit code, and content-aware before/after file inventory with each run. The evidence below was captured with Beads `1.2.2 (6c124203e)` on 2026-09-25.

## RED fixture: Task 1 could not form a native team

This fixture is intentionally recorded before the Task 2 routing contract. In a disposable Git repository with initialized Beads, create two independent ready issues with non-overlapping file scopes, then ask a live host that has loaded the Task 1 draft to start both as one-off work. The Task 1 entrypoint has no native dispatch or same-session team procedure, so the expected RED observation is: no native handles are returned or retained, no worker is launched, and neither issue is claimed merely because the request was made.

```sh
bd create --title 'canary alpha' --description 'Scope: alpha.txt; acceptance: create alpha.txt' --json
bd create --title 'canary beta' --description 'Scope: beta.txt; acceptance: create beta.txt' --json
```

Record the actual issue IDs, host/tool version, prompt, returned result, and Beads status. Do not substitute a shell process, a made-up acknowledgement, or a documentation search for a native launch. This RED case is a fixture/contract record only: no live v9 host session was available to this Task 2 author, so it does not assert an observed host result.

## GREEN acceptance fixture: bounded native teams

Run this only in a disposable Git repository with initialized Beads and a live host that has loaded this v9 draft. Create `alpha`, `beta`, and `gamma` as independent ready tasks with non-overlapping writable paths. Keep `gamma` unclaimed at first. Prepare separate worktrees and branches for `alpha` and `beta`, then ask the active host to run two teams. The host must use its own native mechanism; no CLI controller, shell background process, or synthetic acknowledgement is an acceptable substitute.

For **Codex**, retain the actual result of `spawn_agent` for each team, inspect the current session with `list_agents`, and send `alpha`'s next task to the same retained agent with `followup_task` after `alpha` completes. For **Claude**, retain the actual Agent/session identity from its supported Agent facility and use that same identity's supported resume/follow-up facility. Do not describe either host as having the other's API.

Pass only when all observed facts below are recorded:

- Two actual native handles/Agent identities and two distinct task worktrees were returned or visibly shown in the same live session; no writable path is shared.
- Only `alpha` and `beta` are claimed/in progress. `gamma` remains ready until the retained `alpha` team is given it; each team has no more than four ordered tasks and only its current task is claimed.
- The retained `alpha` handle/identity receives `gamma`; the new task is not given to a replacement worker solely to simulate team reuse.
- Delaying `alpha` beyond the agreed progress interval causes a status request to its same native handle/identity and a stale/hung concern to be recorded, while the independent `beta` lane continues.
- In a separate explicit host rejection that guarantees no worker was created, no handle, completion, or in-progress task is recorded. The rejected task is accurately returned to ready with the observed failure noted.

Record host and tool versions, task IDs, returned handles/identities (redact sensitive data), worktree paths, all Beads transitions, the status-request transcript/result, and whether the fixture passed. A timeout, lost response, missing handle, or ambiguous launch is a task-local uncertain result, not GREEN. This acceptance fixture is defined but unexecuted here: its live Codex run belongs to the root orchestrator, and Claude requires an actual Claude session.

## RED fixture: Task 2 has no review-remediation loop

Run this against the pre-Task-3 draft in a disposable initialized-Beads Git repository. Create two independent tasks with disjoint worktrees: `review-blocked`, whose committed acceptance command deliberately exits nonzero, and `independent-clean`, whose committed acceptance command exits zero. Dispatch both through a live host. Give `review-blocked` to a developer, capture its candidate revision and failing acceptance result, then request a review. Continue `independent-clean` through its own developer/reviewer path.

The expected RED observation is that the Task 2 draft has no required revision-bound non-author `FIX`/`CLEAN` report, no required same-worktree remediation and re-review, and no integration/closure gate. Record that absence as the defect; do not fabricate a review or close either task to make the fixture appear complete. Also record whether the independent lane is able to proceed while `review-blocked` remains incomplete. This is a live-host fixture: a documentation search or a shell substitute does not run it.

## RED fixture: Task 4 has no bounded recovery or ledger protocol

This is a draft-gap fixture, intentionally unexecuted. In a disposable initialized-Beads Git repository with a live host that has loaded the pre-Task-4 v9 draft, simulate: an explicit no-worker spawn rejection; an ambiguous timeout after dispatch; a temporary NO-GO from an observable retained handle; an explicit graceful pause; and a later same-host session with dirty work.

Record that the pre-Task-4 draft has no complete recovery and ledger protocol: it does not require confirmed no-launch requeue evidence with stale-assignee clearing, task-local ambiguous blocking without replacement, same-handle NO-GO blocking/follow-up/resumption evidence, a bounded pause/resume breadcrumb, or resolution movement from `BLOCKERS.md` to stable `DECISIONS.md` entries.

## GREEN acceptance fixture: bounded recovery, pause, and ledgers

Run only in a disposable initialized-Beads Git repository with a real live host loaded with the complete v9 draft. This canary is defined here and has not been run by this change.

1. Claim `no-launch`, cause an explicit native rejection that guarantees no worker was created, and retain the host response. Require `bd update ID --status open --assignee ''`, then require `no-launch` is ready and unassigned with the response and retry condition in its Beads comment; require no handle, completion, or replacement worker.
2. Claim `ambiguous`, induce a timeout or lost response after a possible launch, and start independent `ready`. Require only `ambiguous` blocked with evidence and next observation; through a later same-host session, require `ready` can proceed while `ambiguous` is neither relaunched nor assigned a fabricated identity.
3. Give an observable retained handle a temporary NO-GO. Require only its Beads task becomes blocked while the actual handle and claim remain retained. Send the returned follow-up delta only to that same native handle; after the blocker resolves, require an observed start of work by that same handle before restoring `in_progress`. Require no inferred CLEAN or closure. If the handle is not observable, require the task stays blocked and no substitute worker starts.
4. Pause with an observable active worker and dirty worktree. Require no new claim or assignment, a native checkpoint/stop request, and a short `.agent-team/SESSION.md` containing host/time, active Beads IDs, worktree/branch/revision plus dirty summary, evidence, last handles/uncertainty, pending operations/approvals, blockers, and next action. Require no transcript or duplicate task definition.
5. Resume in a later same-host session. Require Beads and Git inspection before dispatch, preservation of dirty work, and no silent overwrite, reassignment, or relaunch. Resolve one user question: require its `BLOCKERS.md` entry has `ID`, task, question, recommendation, impact, and next prompt; at a status or integration milestone it is raised; its resolution is appended under a new stable `D-` ID in `DECISIONS.md` before removal from `BLOCKERS.md`. Require earlier `D-` and referenced `M-` entries unchanged.

Record host/tool versions, exact Beads transitions/comments, actual returned identities and control observations, Git/worktree state, ledger diffs, and command results. A fixture that cannot observe a live native handle records `not run` or `blocked`; it must not claim these outcomes.

## GREEN acceptance fixture: independent review and safe integration

Run this only in a disposable initialized-Beads Git repository with a live host that has loaded the complete v9 draft. Create `review-blocked` and `independent-clean` with disjoint worktrees and committed acceptance commands. Make the first command fail deliberately; make the second pass. Record every native developer and reviewer handle/identity, Beads ID, candidate revision, command output, reviewer report, integration commit, worktree/branch state, and Beads transition. Never substitute a shell worker or a made-up review report.

Pass only if all observed facts below are recorded:

- `review-blocked` receives a real non-author report for its exact candidate revision with all six required fields and `VERDICT: FIX`; its failing acceptance result is a finding, it is not integrated or closed, and repeated unresolved work is marked blocked with a Beads comment containing the revision and next reconciliation action.
- `independent-clean` continues despite that block. Its reviewer actually inspects its diff and evidence, reports its exact candidate revision and `CLEAN`, and is not its author. The orchestrator rechecks scope/relevant checks, integrates and commits that exact revision, then runs `bd close ID --reason ...`; capture the close reason.
- Repair `review-blocked` in the same worktree, produce a new revision, and obtain another real non-author review for that new revision. Only after its exact-revision `CLEAN`, orchestrator revalidation, integration, and integration commit may it close.
- After each successful integration, clean merged task worktrees and branches are removed without force. Exercise a clean task branch integrated by cherry-pick too: after recorded verified patch-equivalence identifies its exact task scope and exact integrated revision, remove the clean worktree without force, show `git branch -d <exact-verified-branch>` refusing only for ancestry, then use only `git branch -D <exact-verified-branch>`. Preserve and report every dirty, unknown, unverified, failed, or ancestry-unmerged branch without that verification.
- No deployment runs without a project-recorded explicit completed-task batch command, approval, and verification rule. When all three are recorded, run only that command and retain its output/result. At most two on-demand dev servers run for this project; record their task ownership and do not share either with another project.

This live-host acceptance fixture is defined but unexecuted in this Task 3 worktree. Do not call it GREEN from these Markdown edits, a fixture setup, or a documentation check; a root orchestrator must run and retain the actual host evidence.

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

## RED fixture: Task 5 import and optional paths are not yet proved

This is a disposable Beads 1.2.2 fixture, not a live-host claim. It must start from an initialized Git-and-Beads project and an actual `TASKS.md` task row containing a source ID, title/objective, acceptance, status, valid dependency IDs, and a source pointer. Convert only the task rows into export-compatible `tasks-import.jsonl`; retain non-task source history unchanged. Require these RED observations from a pre-Task-5 draft:

- A second import can duplicate a source task because it mistakes a source ID for a Beads ID, or rewrite a newer local Beads edit.
- A bounded one-off audit cannot become exactly one Beads task without a redundant approval or a handoff.
- Absence of Serena, Graphify, Playwright, or visual tooling blocks an ordinary code task.
- A local dashboard refresh error reopens or changes already accepted work.

## GREEN fixture: Task 5 imports are guarded and status snapshots are bounded

Run only in a disposable initialized Beads 1.2.2 Git repository. This proves CLI/data behavior and documented skill routing; it does not prove native host dispatch, available model metadata, or an installed optional aid.

1. Build a temporary `tasks-import.jsonl` from a Markdown fixture with non-Beads source IDs such as `legacy-a` and `legacy-b`; `legacy-b` depends on `legacy-a`. Create no `id` field from either source ID. Instead preserve each source ID in Beads-supported provenance: `external_ref: "TASKS.md#legacy-a"`, a stable `source_system`, and metadata if useful. Preserve title/objective, acceptance criteria, and status. Before mutation, reject a fixture whose dependency has no corresponding source row or has no Beads representation. Run:

   ```sh
   bd import --dry-run --json tasks-import.jsonl
   ```

   Compare source provenance and candidate row count to the source, and display the source-dependency mapping plan before approval. Only after a separate approval run `bd import --json tasks-import.jsonl`. Resolve the generated native Beads IDs with `bd list --metadata-field agent_team_source_id=<source-id> --json`, verifying each matching `external_ref` and `source_system`; map `legacy-b -> legacy-a` through those native IDs and add/verify that native dependency with `bd dep add`. On re-import, find both source rows by provenance and omit them and their dependency edge from the candidate. Require exactly one native issue per source ID, no issue whose native ID is `legacy-a` or `legacy-b`, and the mapped dependency exactly once.

2. Update the imported issue locally, retaining a strictly newer `updated_at`, then re-run the original JSONL import without `--allow-stale`. Require `stale_skipped_ids` to name that ID and the newer local fields to remain unchanged. The migration flow must also omit existing IDs before it invokes import, so the CLI guard is defense in depth rather than permission to update.

3. With no optional aids installed, request one ordinary code task and require it to remain ready/dispatchable through native search/edit fallback. Make a bounded one-off audit request and require it to create exactly one Beads task without another approval prompt. Neither path may inspect, require, or wait for Project Kickoff or a handoff.

4. Put 1,000 issues in Beads, obtain aggregate status without supplying issue rows to a model, and run `bd ready --limit 20 --json`. Require the refresh context to contain no more than those 20 ready rows. For a nonempty page, require its 1–20 rows in the table body, the table visible, and empty copy hidden; for an empty page, require the table hidden and empty copy visible. After an accepted integration and its normal closure, force the local `.agent-team/dashboard/index.html` refresh to fail; require the already closed Beads task to remain closed and report the dashboard error separately.

Pass only if all four checks hold. Record `bd version`, dry-run/import JSON results, source-to-proposal comparison, issue count/IDs, newer-edit evidence, the exact bounded ready-page size, optional-aid availability, dashboard failure result, and any actual host/model metadata separately. Do not label a live-host canary GREEN unless it was actually run on that host.

## Actual Beads 1.2.2 Task 5 fixture observation

Observed in a disposable initialized fixture on `bd version 1.2.2 (6c124203e: HEAD@6c124203e771)`: `bd import --dry-run --json tasks-import.jsonl` accepted an export-schema JSONL and reported a dry run; `bd import --json tasks-import.jsonl` kept the stable issue count at one on re-import. After a local `bd update` produced a newer `updated_at`, importing the original JSONL reported that ID in `stale_skipped_ids` and retained the local edit. A second provenance fixture imported `legacy-a` and `legacy-b` without `id`, generated nonmatching native IDs, preserved `external_ref`, `source_system`, and source-ID metadata, located one native issue per source ID through `bd list --metadata-field`, and added the mapped native dependency with `bd dep add`. A bulk import left 1,070 issues on disk and `bd ready --limit 20 --json` returned 20 rows. This is CLI evidence only: it does not prove what a model receives, and the GREEN fixture and every live-host assertion above remain unrun.
