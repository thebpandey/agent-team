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
