# Mistakes and durable lessons

## M-001: Do not close a deadline-sensitive release failure with a green retry

Status: Active

Scope: Agent-Team hook deadline tests and release verification; 7.1.1 and later

Source: AT-15 Task 6a report and final whole-branch review, 2026-09-11

Mistake: An earlier full-suite deadline failure was classified as unrelated variance after an isolated pass and a green full rerun, without a causal contention probe.

Cause: Non-reproduction was treated as evidence that the concurrency-sensitive failure was resolved.

Correction: A bounded 16-way probe reproduced the same lint status race 9 times across 32 processes. The stabilization makes the log-stage test deterministic without changing production deadlines.

Prevention: Any deadline-sensitive failure in a required release suite remains open until the responsible phase is identified and tested under representative bounded load. An isolated pass or green retry is characterization only.

## M-002: Test consequential evidence writers through their policy consumer

Status: Active

Scope: Agent-Team operational gate-evidence writers and release policy; 7.1.1 and later

Source: AT-15 Task 6a ruling, review, and repairs `01a7956` and `7b237ba`, 2026-09-11

Mistake: The initial release evidence path could receipt evidence without mapping the release state consumed by policy. Its first repair did not bind every active integration identity and did not revalidate the mapped run and artifact digest.

Cause: Tests checked evidence acceptance and policy fields separately instead of proving the complete mutation-to-consumption boundary and independent corruption cases.

Correction: The release mapper now derives and validates exact integration identity, and policy revalidates run, batch, artifact digest, and repeated release bindings before consequential operations.

Prevention: For each consequential evidence writer, test invalid evidence without mutation, valid canonical mapping, exact policy consumption, and independent corruption of every authority and identity field.

## M-003: Recheck durable authority after cross-process races

Status: Active

Scope: File-backed coordination and revision-checked workflows; vNext and later

Source: Tasks 1–18 complexity audit, 2026-09-19; deterministic admission regression test

Mistake: Admission treated a revision change observed after its first commit lookup as stale, even when an identical concurrent process had since written the authoritative commit.

Cause: A process-local mutex was mistaken for complete coordination, and the stale branch did not recheck durable authority after the unlocked cross-process race window.

Correction: The stale branch now re-reads and validates the immutable commit, converges its projection, and classifies an exact retry as duplicate.

Prevention: For file-backed coordination used by multiple agent processes, test a deterministic interleaving across separate lock domains and recheck durable authority before classifying a raced request or mutating state.

## M-004: Search for existing primitives before adding local machinery

Status: Active

Scope: Agent-Team implementation and refactoring; vNext and later

Source: Tasks 1–18 complexity audit, 2026-09-19

Mistake: Completed code repeated path canonicalization, deep-copy routines, argument error handling, membership loops, unused collaborator state, and an unused digest helper.

Cause: Task-local implementations were added without a final repository-wide duplication and necessity pass.

Correction: Shared invariants now have one owner, standard-library operations replace handwritten loops, unused state and dead code are removed, and external APIs remain unchanged.

Prevention: Before handoff, search the affected repository for equivalent invariants and helpers; reuse the narrowest existing or standard-library primitive, remove unused state, and prove behavior with focused plus full tests.

## M-005: Validate the published artifact for every supported platform

Status: Active

Scope: Native release packaging and publication; v8.0.5 and later

Source: Beads atv-5sh.44, Windows host report and release inspection, 2026-09-21. The v8.0.5 release contains only the canonical Linux archive and its metadata.

Mistake: The release passed Windows source tests but shipped only a Linux ELF executable. A Windows user could not run the installer without WSL.

Cause: The package command built the release-runner host binary only. Release gates did not validate the supported-platform artifact matrix.

Correction: Task atv-5sh.44 adds a native Windows amd64 bundle and platform-specific package checks while preserving the Linux release contract. Its Beads record holds the exact revision and verification results.

Prevention: Before publication, validate one documented downloadable artifact for each supported platform: executable format and architecture, exact contents, manifest, checksums, SBOM, and installation from that artifact on its matching CI runner using the documented extraction layout. Source tests or cross-compilation alone do not prove artifact delivery or native execution.

## M-006: Check all supported skill discovery roots before native installation

Status: Active

Scope: Native Codex install/update and status version; v8.0.5 and later

Source: Beads atv-5sh.44 R8, user status report and installed-manifest inspection, 2026-09-21.

Mistake: A native v8.0.5 entrypoint was installed under `.agents/skills` while a v6.1.0 same-name skill remained active under `.codex/skills`. Codex could load the old skill and show its old banner version.

Cause: The installer validated only its selected destination and manifest-owned files. It did not inspect the other supported Codex discovery roots.

Correction: Native install/update detect conflicting discoverable entrypoints before target mutation and preserve unowned files. An operator can move a superseded legacy root to a recoverable location outside skill discovery, then retry. Native banner instructions use the installed native entrypoint version; the repository's separate legacy package version is not that authority.

Prevention: Test installation and update with stale top-level and nested native skills in each default discovery root, an OS-resolved fallback home, and a custom host home. Require one authoritative current entrypoint, an exact-path conflict for unowned files, and no target mutation on conflict.

## M-007: Test reproducibility through the public package command

Status: Active

Scope: Native release build identity and reproducible output; v8.0.5 and later

Source: Beads atv-5sh.44 R3, independent repeated-command reproductions, 2026-09-21.

Mistake: Archive-helper determinism was treated as package reproducibility. Repeating the real package command changed the Linux binary and archive at the same source revision.

Cause: Go's automatic VCS stamp changed from clean to dirty when the command created its own untracked output files.

Correction: Release builds disable the incidental VCS stamp and retain the explicit linker-bound release version and commit.

Prevention: Run the public package command twice from the same clean committed source, retain its generated files between runs, and compare every published asset byte for byte.

## M-008: Preserve generated output before normal worktree removal

Status: Active

Scope: Temporary review and release-test checkout cleanup

Source: Beads atv-5sh.44, independent reviewer cleanup report, 2026-09-21.

Mistake: A temporary detached review checkout was removed with `git worktree remove --force` because it contained two generated executables.

Cause: Disposable generated files were treated as sufficient reason to bypass normal worktree removal.

Correction: Pre-removal status showed only the two generated binaries, with no tracked changes; both release-output sets remained outside the checkout and were preserved in canonical task evidence. The tracked source remains at the reviewed commit.

Prevention: Inspect tracked, untracked, and ignored files; preserve required output outside the checkout, remove only confirmed disposable files, then use normal worktree removal without `--force`.

## M-009: Isolate the actual host's home lookup in platform tests

Status: Active
Scope: Cross-platform installer test fixtures
Source: Beads atv-5sh.44.2, Windows CI runs35661872390 and35661872297, 2026-09-21.
Mistake: A fallback-home fixture set HOME only and wrote fake skill bytes into the disposable Windows runner's default home.
Cause: `os.UserHomeDir` uses USERPROFILE on Windows, independent of a simulated layout's requested platform.
Correction: Isolate both HOME and USERPROFILE to the owned fixture before calling the real host fallback; confirm native Windows CI.
Prevention: Before a test can install files, assert that every resolved home and destination is inside its owned fixture on the actual running OS.

## M-010: Close binary-inspection handles before fixture cleanup

Status: Active
Scope: Windows PE inspection tests
Source: Beads atv-5sh.44.1, Windows CI runs35661872390 and35661872297, 2026-09-21.
Mistake: Two PE tests opened executable files without closing the returned handles, so Windows could not remove their temporary directories.
Cause: Linux permits unlinking open files and concealed the leaked handle during local checks.
Correction: Close every successful `debug/pe.Open` handle, including assertion-failure paths; confirm native Windows cleanup.
Prevention: Register cleanup immediately after every successful file or archive open, and run affected filesystem tests on each supported OS.

## M-011: Account for tracker audit-file side effects

Status: Active
Scope: Owner tracker operations with protected local files
Source: Beads atv-5sh.44 close operations, 2026-09-21.
Mistake: Closing two task blockers appended audit rows to a user-owned interactions file whose bytes had to remain unchanged.
Cause: The tracker command's local JSONL audit side effect was not included in the write-scope check.
Correction: Preserve the new task rows in task evidence, remove only those exact appended rows, and confirm the original file hash. Canonical Beads database records remain intact.
Prevention: Check protected-file hashes after every tracker mutation that can append local audit data; preserve task audit rows separately when the existing file is outside task scope.

## M-012: Align every current skill entrypoint with the native release

Status: Active
Scope: Repository-root and packaged native skill discovery
Source: Beads atv-5sh.45, Windows repository-root install report after v8.0.6, 2026-09-21.
Mistake: The v8.0.6 correction deliberately retained repository-root SKILL.md metadata7.3.1 while updating the native entrypoints. A current repository-root installation still reported the old version.
Cause: The release check treated the root skill as historical content even though users can install that current public entrypoint.
Correction: Bind repository-root metadata and active instructions to the current native release; keep old version references only as explicit historical migration context.
Prevention: Before publication, test every current discoverable entrypoint against the canonical release version and native routing, including a latest-repository install on each supported published platform. Do not exclude a public root entrypoint merely because its body contains legacy material.

## M-013: Compare metadata independently of checkout line endings

Status: Active
Scope: Cross-platform release and documentation assertions
Source: Beads atv-5sh.45.1, Windows CI35668129772/35668129822, 2026-09-21.
Mistake: New version-parity assertions rejected correct8.0.7 metadata on Windows because they matched literal LF line endings.
Cause: Local Linux checks did not exercise CRLF checkout text, and duplicate assertions embedded the line-ending assumption.
Correction: Use one line-ending-normalized metadata predicate and regress both LF and CRLF plus wrong-version rejection; require fresh Windows CI.
Prevention: For cross-platform source-text checks, separate semantic equality from newline representation and test both checkout forms before publication.

## M-014: Test assertions in their actual shell and data format

Status: Active
Scope: Windows PowerShell release verification
Source: Beads atv-5sh.45.2, failed release run35668956588, 2026-09-21.
Mistake: The Windows canary put literal Markdown backticks in a PowerShell double-quoted comparison string, so the shell changed the expected text and rejected valid native routing.
Cause: Static source assertions and review did not execute the comparison in its target shell before the release run.
Correction: Use literal quoting, decode structured command output before semantic checks, and verify the complete canary boundary before retrying publication.
Prevention: Exercise shell-sensitive literals and serialized native output in the target shell, including expected failures and Windows paths; a source-string presence check alone is not behavioral proof.

## M-015: Preserve supported legacy tracker authority in place

Status: Active
Scope: Native cutover from valid v7 projects
Source: Beads atv-5sh.46, v7 Beads preparation rejection, 2026-09-22.
Mistake: Legacy authority validation accepted only a markdown tracker and rejected valid projects that already used Beads.
Cause: The migration input guard represented one legacy tracker instead of the supported legacy authority boundary.
Correction: Accept supported legacy Beads authority without changing the canonical v8 setup or moving, converting, or rewriting the existing Beads tree.
Prevention: Cover each supported legacy tracker declaration at the authority boundary. For an in-place tracker transition, compare the complete tracker tree before and after preparation, cutover, and rollback; preserve the existing scope and authorization checks.

## M-016: Prove public actions reach their real effect

Status: Active
Scope: Native settings and start commands, packaged host handoff
Source: Beads atv-5sh.47, Windows settings/start no-op report, 2026-09-22.
Mistake: The CLI parsed settings and start but returned acceptance or deferral without persisting settings or providing an executable host-dispatch path.
Cause: Parser/status coverage was mistaken for end-to-end action coverage; the packaged host skill did not complete the missing dispatch boundary.
Correction: Connect bounded settings persistence and one-task admission to a truthful host handoff while preserving immutable setup receipts and legacy data.
Prevention: For each public mutating action, assert the real persisted state or observable host effect. Distinguish admission, host dispatch, and completion; never report a team launched from parser acceptance or a callback invocation alone.

## M-017: Bind test claims to completed commands and exact source

Status: Active
Scope: Native action correction verification
Source: Beads atv-5sh.47, CLI compatibility failure at b395e53, 2026-09-22.
Mistake: A developer reported the focused CLI suite as passing, but the committed parser rejected existing repeated start selectors and the same suite failed.
Cause: The reported result did not establish a successful final command exit for the exact committed source.
Correction: The owner reproduced the two selector failures with exit 1 and required compatibility repair before integration.
Prevention: Record the tested SHA, full command, and final exit status. Partial output, a passing subset, or results from earlier working-tree content cannot support an exact-SHA pass claim.
