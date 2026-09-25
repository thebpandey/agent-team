# Agent-Team v9 Distribution and Cutover Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Install, verify, and publish the reviewed skill-only Agent-Team v9 without carrying the v8 controller into user projects.

**Architecture:** Package the exact portable `v9/agent-team/` skill once for both hosts and all OSes. Tiny installation scripts make recoverable backups and copy that tree into native skill directories; no background updater or runtime binary is installed. Quiescent v8 projects keep Beads and archive old controller records.

**Tech Stack:** POSIX shell and PowerShell for installation only, GitHub Actions for packaging checks, GitHub Release checksum set, checked-in GitHub Pages `index.html`.

**Spec:** `docs/superpowers/specs/2026-09-24-agent-team-skill-first-design.md`

## Global Constraints

- Begin only after all core-plan tasks and independent reviews are CLEAN. This plan does not authorize release before live-host canaries.
- Install one platform-neutral skill tree; no native Go binary, Node hooks, LeanCTX integration, or OS-specific skill contents.
- Default homes are `~/.agents/skills/agent-team` for Codex and `~/.claude/skills/agent-team` for Claude, with explicit custom-home options. Never overwrite an existing skill root without first moving the whole root to a recoverable backup.
- Do not run v8 managed uninstall after v9 is installed. Quiescent cutover may run the supported v8 `agent-teamctl uninstall --json` **before** v9 installation; it must not touch project Beads, Git, or `.agent-team` data.
- Preserve user-owned dirty files and all uncertain active work. Exact old Agent-Team hook references may be removed only after a backup; unrelated hooks remain.
- A release is stable only for OS/host combinations with an actual native canary. Do not infer Windows or Claude success from Linux unit tests.

## File map and interfaces

| File | Responsibility |
| --- | --- |
| `v9/install.sh` | POSIX install/update with exact target and recoverable backup. |
| `v9/install.ps1` | Equivalent Windows PowerShell install/update. |
| `v9/tests/install_unix.sh` | Disposable-directory tests for new install, update, backup, collision, and restoration. |
| `v9/tests/install_windows.ps1` | Equivalent native PowerShell tests on Windows. |
| `v9/CUTOVER.md` | Quiescent v8-to-v9 host and project procedure, including exact-hook detection and rollback. |
| `v9/README.md` | First-use/install/update/rollback guide and dashboard location. |
| `.github/workflows/v9-release.yml` | Test/package workflow for one checksum-verified skill archive. |
| `docs/releases/9.0.0-readiness.md` | Evidence table: exact revision, Beads version, OS/host canaries, install and cutover results. |
| Root `README.md`, `CHANGELOG.md`, `index.html` | Public version and proven features, changed only after release gates. |

The installer copies only the reviewed `v9/agent-team/` tree. The skill does not need the installer at runtime. The one release archive contains the skill, both installers, the cutover guide, and checksums.

---

### Task 1: POSIX recoverable skill installation

**Files:** Create `v9/install.sh` and `v9/tests/install_unix.sh`.

**Interfaces:** Input is `codex|claude|both` plus optional `--codex-home` and `--claude-home` paths. Defaults honor existing `CODEX_HOME`/`CLAUDE_HOME` if set, otherwise the current user's `.agents`/`.claude` homes. Output is the exact installed skill root, backup path when replaced, and a nonzero exit on any incomplete copy. It never edits project files.

- [ ] **Step 1: Write RED tests.** In `install_unix.sh`, use `mktemp -d` and explicit home-path arguments to test a new Codex install, both-host install, second update with an unknown user file, and a failed copy. Assert that unknown bytes are in the printed backup and the prior root is restored on failure. Run `bash v9/tests/install_unix.sh`; expected failure is missing `v9/install.sh`.

```bash
test_root=$(mktemp -d)
bash v9/install.sh codex --codex-home "$test_root/codex"
test -f "$test_root/codex/skills/agent-team/SKILL.md"
```
- [ ] **Step 2: Implement the smallest installer.** Resolve the script's own directory, validate `codex|claude|both`, map default homes without changing `HOME`/`CODEX_HOME`/`CLAUDE_HOME`, reject symlink target roots, move any existing root to a unique sibling backup, copy `agent-team/` to a fresh root, compare copied files to source, and restore on failure. Do not merge unknown files into the new root. The target mapping is:

```text
codex  -> <codex-home>/skills/agent-team
claude -> <claude-home>/skills/agent-team
```

- [ ] **Step 3: Run GREEN tests.** `bash v9/tests/install_unix.sh` passes; `bash -n v9/install.sh` passes. Check the test also catches a deliberately removed source file rather than merely testing its own fixture.
- [ ] **Step 4: Independently review** path resolution, backup/restore, shell quoting, and unknown-file preservation.
- [ ] **Step 5: Commit** only these two files with `feat(v9): install portable skill on Unix`.

### Task 2: Windows PowerShell installation parity

**Files:** Create `v9/install.ps1` and `v9/tests/install_windows.ps1`.

**Interfaces:** Same installed tree and backup semantics as Task 1. PowerShell parameters are `-TargetHost codex|claude|both`, `-CodexHome`, and `-ClaudeHome`; defaults honor existing `CODEX_HOME`/`CLAUDE_HOME` if set, otherwise use the current user's `.agents` and `.claude` homes. Users never have to set those variables for a normal install.

- [ ] **Step 1: Write RED native Windows tests.** Use `Join-Path $env:TEMP` with a unique leaf, pass explicit home parameters, and assert the new install, update backup, unknown-file preservation, symlink/reparse refusal, and restoration after simulated failure. Run `pwsh -NoProfile -File v9/tests/install_windows.ps1`; expected failure is the missing installer.

```powershell
$testRoot = Join-Path $env:TEMP ([guid]::NewGuid().ToString('N'))
& ./v9/install.ps1 -TargetHost codex -CodexHome (Join-Path $testRoot 'codex')
if (-not (Test-Path (Join-Path $testRoot 'codex/skills/agent-team/SKILL.md'))) { throw 'skill missing' }
```
- [ ] **Step 2: Implement the minimal script.** Use `Join-Path`, `Move-Item`, `Copy-Item`, and `Get-FileHash` on exact files. Refuse unresolved/reparse target paths. Backup the whole prior root before copy, restore it if copy or hash verification fails, and print the installed and backup paths. Do not modify the PowerShell profile, PATH, registry, or unrelated Claude/Codex settings.
- [ ] **Step 3: Run GREEN tests** on Windows PowerShell; compare the installed file list and SHA-256 values to the same source tree used by the POSIX test.
- [ ] **Step 4: Independently review** Windows path handling, ACL/reparse behavior, rollback, and error exits.
- [ ] **Step 5: Commit** only these two files with `feat(v9): install portable skill on Windows`.

### Task 3: Quiescent v8 cutover and first-use guide

**Files:** Create `v9/CUTOVER.md` and `v9/README.md`; update `v9/tests/CANARIES.md` from the core plan.

**Interfaces:** Consumes v8 status, native host observations, Beads, and Git. Produces a documented cutover or a precise refusal with no project-state mutation. It never imports live v8 reservations.

- [ ] **Step 1: Add a RED cutover canary.** A disposable v8 project with a live/uncertain worker must be refused. An idle project with existing Beads must remain readable after host package replacement. Record expected preserved hashes for `.beads` and old `.agent-team` files.
- [ ] **Step 2: Write the exact procedure.** Observe and finish/stop all v8 native workers; reconcile dirty worktrees; back up current host skill roots and any exact Agent-Team hook configuration; run installed v8 `agent-teamctl uninstall --json` before v9 installation if the managed manifest exists; install v9; verify Beads status and the new skill version. Leave old project `.agent-team` controller records read-only; new skill ignores them except its own `SESSION.md` and dashboard. Unknown hooks or active workers stop cutover rather than triggering broad cleanup.
- [ ] **Step 3: Document rollback and first use.** Restore exact backed-up skill roots if v9 install fails; do not call v8 uninstall afterward. Show `$agent-team` and `/agent-team` setup/status/start/pause/resume, the optional Project Kickoff import, model selection, one-off requests, and `.agent-team/dashboard/index.html` as a local snapshot.
- [ ] **Step 4: Run GREEN canary.** Refusal leaves the live fixture untouched; idle fixture retains Beads and Git hashes, has v9 skill on selected hosts, and can read `bd status --json`. Review docs for paths that work on Windows and Linux.
- [ ] **Step 5: Commit** these documentation/canary files with `docs(v9): define recoverable v8 cutover`.

### Task 4: One portable checksum-verified release artifact

**Files:** Create `.github/workflows/v9-release.yml` and `docs/releases/9.0.0-readiness.md`; modify `v9/README.md`.

**Interfaces:** Consumes the exact reviewed Git revision, core and installer test results, and live canary evidence. Produces `agent-team-skill-9.0.0.zip` plus `SHA256SUMS`; it never includes v8 binary/hook files.

- [ ] **Step 1: Add a RED package inspection.** Package the current tree locally and assert the archive contains only `agent-team/`, `install.sh`, `install.ps1`, `CUTOVER.md`, and `README.md`. The check must fail if a v8 `agent-teamctl` binary or `hooks/` file is injected.

```text
Allowed archive roots: agent-team/, install.sh, install.ps1, CUTOVER.md, README.md
Forbidden archive paths: hooks/, agent-teamctl, vnext/, .beads/, .agent-team/
```
- [ ] **Step 2: Implement the workflow.** On a manual release request, run Unix installer tests on Linux, PowerShell installer tests on Windows, skill source checks, and the actual Beads fixture on a pinned tested CLI. Archive the exact source tree once and publish its SHA-256. Do not attach a macOS/Windows-specific binary or run the v8 release workflow.
- [ ] **Step 3: Run GREEN workflow on a candidate commit.** Record the exact SHA, artifact file list, checksums, real Beads version, and each runner result in `9.0.0-readiness.md`. The release remains a candidate while live-host canaries are missing.
- [ ] **Step 4: Independently review** workflow permissions, tag/ref selection, archive contents, checksum verification, and release refusal on any failed gate.
- [ ] **Step 5: Commit** workflow/readiness changes with `build(v9): package one portable skill release`.

### Task 5: Live-host acceptance and public release

**Files:** Update `v9/tests/CANARIES.md`, `docs/releases/9.0.0-readiness.md`, root `README.md`, `CHANGELOG.md`, and `index.html` only after the results are known.

**Interfaces:** Consumes completed core, package and canaries. Produces a truthful v9.0.0 GitHub Release and Pages/README update only for verified behavior; no code is changed in this task.

- [ ] **Step 1: Run actual native canaries.** On Codex and Claude, verify worker launch, four-task refill, one-off two-team limit, true independent review, remediation, integration, blocker lane continuation, explicit rejection, ambiguous result isolation, NO-GO follow-up, pause/resume, and quiescent host switch. Test Windows and Linux separately and record unavailable combinations as unverified, not passed.
- [ ] **Step 2: Run the scale/resource canary.** In a disposable real Beads 1.2.2 project with 1,000 tasks, verify a bounded ready query and no prompt dump; observe the two-server cap and dashboard snapshot after integration. Keep the fixture outside user projects.
- [ ] **Step 3: Verify the downloaded candidate archive.** Check its published SHA-256, install it on the claimed platforms, run status/one-off and backup/rollback smoke, and record exact release revision and output in `9.0.0-readiness.md`. A failure returns to its owning core/distribution task; do not label it CLEAN by changing docs.
- [ ] **Step 4: Publish only after gates pass.** Tag the exact candidate revision, publish the one archive/checksum set, then update README, changelog, and Pages with the release link, install guide, dashboard location, supported hosts, and limitations. Verify the public page displays the actual published version.
- [ ] **Step 5: Commit and close** the release-documentation change and corresponding Beads work only after publication and observed installation. Preserve older release artifacts and v8 project archives.

## Distribution acceptance handoff

The release is not complete when CI is green; an actual downloaded package must install and run in both native hosts on every OS claimed. Report any missing Windows or Claude live canary explicitly, and do not replace the user's working v8 installation until the quiescent cutover checklist and rollback path are verified.
