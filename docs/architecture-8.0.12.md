# Agent-Team 8.0.12: bounded installer journal repair

The 8.0.11 managed upgrade exceeded the installer's 32 MiB JSON journal budget.
The installed 8.0.10 executable was 7,198,764 bytes and the published 8.0.11 Linux
executable was 13,665,509 bytes. Update records both the old target preimage and
its rollback backup, plus the new executable. Those payloads alone require about
37.4 MB after JSON base64 encoding. A later upgrade between two 14 MiB binaries
requires 56 MiB before metadata.

The repair preserves the schema-1 journal and its exact-owner, digest, rollback,
interruption and recovery checks. Individual file reads remain bounded at 32 MiB;
the complete encoded journal is bounded separately at 64 MiB. Metadata and every
payload are still charged before installation mutations, and serialization is
bounded again before publishing the durable journal. The aggregate limit can
reject a combination of otherwise valid individual files. It is not a promise
that every combination of 32 MiB files fits.

Only installer journal reads and writes use `store.NewInstallJournal`, which
reuses rooted atomic persistence with a fixed 64 MiB ceiling. Ordinary
`store.New` records retain their 32 MiB hard limit, including when callers request
a larger limit. Individual-file reads, deletion, legacy receipts and snapshots
also retain the existing 32 MiB bound.

This limit applies to temporary installer transaction payloads. It does not alter
Beads database storage or relax tracker readiness, evidence or ownership checks.
External payload references would introduce additional recovery and cleanup
states; retaining the established journal format keeps this repair scoped.

Regression tests cover the measured 8.0.10-to-8.0.11 sizes and two 14 MiB releases
through update, rollback, reupdate and uninstall. Oversized files and aggregate
payloads still fail before mutation. The release gate now seeds a prior binary
from the final archive's actual executable bytes, with an inert content change,
then verifies update, rollback, reupdate, idempotent retry and uninstall while
preserving unrelated settings. That gate uses the archive supplied to publication
so future executable growth cannot be hidden by tiny fixture binaries.

## Verification boundary

The repair was implemented and reviewed in parallel with version/docs and
handoff compatibility work. Independent review checked that the 64 MiB
allowance is limited to installer journals and that ordinary records retain
their existing ceiling. Local regression and packaged-executable canary checks
passed; [PR #11](https://github.com/thebpandey/agent-team/pull/11) then passed
all 30 CI checks before merge.

These results qualify the code changes, not a user's installation. The
[release record](releases/8.0.12-readiness.md) tracks the exact release revision
and the subsequent download, managed-update, and installed smoke evidence.
That managed update completed at manifest revision 6 with all four owned hashes
verified. Installed-controller setup and replay checks passed for both hosts;
they do not establish a live Claude UI wizard run.
