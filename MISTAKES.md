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
