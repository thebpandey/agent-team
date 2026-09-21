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

Prevention: Before publication, validate one documented downloadable artifact for each supported platform: executable format and architecture, exact contents, manifest, checksums, SBOM, and installation from that artifact on its matching CI runner. Source tests or cross-compilation alone do not prove artifact delivery or native execution.

## M-006: Check all supported skill discovery roots before native installation

Status: Active

Scope: Native Codex install/update and status version; v8.0.5 and later

Source: Beads atv-5sh.44 R8, user status report and installed-manifest inspection, 2026-09-21.

Mistake: A native v8.0.5 entrypoint was installed under `.agents/skills` while a v6.1.0 same-name skill remained active under `.codex/skills`. Codex could load the old skill and show its old banner version.

Cause: The installer validated only its selected destination and manifest-owned files. It did not inspect the other supported Codex discovery roots.

Correction: Native install/update detect conflicting discoverable entrypoints before target mutation and preserve unowned files. An operator can move a superseded legacy root to a recoverable location outside skill discovery, then retry. Native banner instructions use the installed native entrypoint version; the repository's separate legacy package version is not that authority.

Prevention: Test installation and update with stale same-name skills in each default discovery root and a custom host home. Require one authoritative current entrypoint, an exact-path conflict for unowned files, and no target mutation on conflict.
