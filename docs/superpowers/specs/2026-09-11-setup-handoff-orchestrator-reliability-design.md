# Setup, Handoff, and Orchestrator Reliability Design

Date: 2026-09-11  
Repositories: Agent-Team and Project Kickoff  
Target project for migration verification: `/home/server/dev/brainvault-system`

## Problem

BrainVault exposed several distinct issues that were presented as one setup failure:

- Claude Code loads Project Kickoff 0.4.0 while Codex loads 0.4.1. Both hosts load Agent-Team 7.1.0, although newer source exists only in local repositories.
- Historical Project Kickoff 0.4.0 provenance correctly says its handoff was tested against Agent-Team 7.0.2, but status text makes that look like the active runtime.
- A tracked handoff cannot permanently equal the Git tip that contains it. Committing the handoff advances the tip and makes the exact-tip rule self-invalidating.
- Agent-Team accepted completion evidence for a tracker task outside the initialized run scope, while claim and re-initialization correctly rejected that task.
- Setup intentionally treated settings as a separate opt-in wizard, contrary to the desired setup experience.
- Compatible existing LeanCTX, Superpowers, and Ponytail installations were marked unusable only because Agent-Team did not own their provenance sidecars.
- LeanCTX and Graphify are independent, but combined reporting implied that LeanCTX blocked Graphify. Agent-Team 7.1.0 also rejected valid AST-origin Graphify relationships because it treated `confidence: INFERRED` as semantic provenance.
- The ast-grep package's deprecated `sg` alias collides with `/usr/bin/sg` and does not match the executable workers naturally verify.
- An orchestrator can answer a user question and then stop supervising otherwise active work.

The absence of a Beads Dolt remote and `auto_deploy` without a configured release target are expected fail-closed states. This design does not weaken them.

## Release identities

- Agent-Team becomes 7.2.0 because setup behavior, dependency reuse, run-scope admission, and orchestration semantics change compatibly but materially.
- Project Kickoff remains 0.4.1 because that version has not been published and the handoff correction is backward-compatible with existing approved artifacts.
- Agent-Team 7.2.0 publishes first. Its optional Project Kickoff documentation uses a stable `releases/latest` link rather than a future tag.
- Project Kickoff 0.4.1 then qualifies new handoffs against the released Agent-Team 7.2.0 and publishes second.
- Both Codex and Claude Code user installations are updated only from the verified release artifacts, with receipts and checksums. Updating a source checkout never claims that a host installation changed.
- An installation receipt binds the installed package to its exact release tag and URL, source revision, downloaded archive SHA-256, installed file digests, host and scope, transaction and recovery identity, and installation time. Agent-Team consumes the archive's `.agent-team-source.json`; Project Kickoff provides an equivalent transactional artifact installer instead of relying on an unmanaged directory copy.

## Design decisions

### 1. Settings runs on every setup invocation

Every state-changing `agent team setup` invocation enters the settings wizard after canonical project and dependency inspection. The wizard begins with the current effective settings and offers a first-class Cancel/Keep Existing action.

Cancel means:

- preserve every existing role, model, effort, fallback, run, dashboard, and deployment setting byte-for-byte;
- do not create a settings mutation or increment the settings version;
- do not roll back valid dependency preparation that completed earlier in the same setup invocation;
- continue to the setup/readiness summary with `settingsOutcome: "kept_existing"`.

Confirmed changes remain versioned atomic settings mutations. A timeout, missing answer, or host interruption is not consent to defaults and leaves settings unchanged. Read-only `status`, `health`, `help`, and `version` actions never launch the wizard.

### 2. Native runtime identity owns initialization and recovery

Initialization must bind the project owner to the actual native host session, not merely to a syntactically plausible string in a request file.

The native command context supplies a normalized identity:

```json
{
  "host": "codex | claude-code",
  "sessionId": "actual native session identifier",
  "observed": true
}
```

`project-initialize` requires the request actor to equal that observed identity. A mismatch fails before any tracker, setup, state, or team mutation. A plain shell invocation without current native identity can validate a request but cannot establish ownership. Existing initialized projects retain their valid current owner; migrations do not replace an owner merely because an earlier identifier looked unusual.

The setup summary echoes the host and derived owner before mutation. Regex checks continue to reject placeholders, but they are not described as authentication.

Native project identity also includes the host session's actual working directory. Agent-Team hooks resolve the project from the native event `cwd`; a shell tool's subprocess `workdir` and a CLI `--project` argument do not change that session location. Migration acceptance therefore runs in fresh native sessions rooted in BrainVault: Codex launches with `-C /home/server/dev/brainvault-system`, and Claude Code launches after changing to that directory. Linked-worktree and nested-repository sessions keep their own identity and cannot borrow the canonical checkout's authority. The installed `runCommand` native context carries the observed session and worker facts; request JSON cannot assert them.

Add a separate closed `project-owner-recover` transition for an initialized project whose historical native owner has stopped. It is exposed only through the installed Codex or Claude Code adapter. The trusted in-memory context, not the request or environment, supplies the replacement host/session/CWD/writer identity, a host-issued user approval bound to the project and old/new identities, and an authoritative old-session liveness observation. The request contains only bounded operation identity, expected versions/fingerprints/owner generation, project identity, and a human reason; it cannot assert an actor, replacement owner, host, approval, writer, or liveness result.

Ownership is runtime-qualified as `(host, sessionId)` and fenced by an increasing ownership generation. New initialization records generation 1 and a durable writer identity. Recovery preserves historical authorship and appends immutable history rather than rewriting earlier operation owners. Codex and Claude Code sessions with the same textual session ID remain distinct, and a cross-host transfer preserves both hosts' independent settings profiles.

Recovery requires positive proof that the recorded owner stopped. An active observation returns `owner_active`; missing legacy writer identity, unavailable registry data, PID reuse, namespace mismatch, or any other uncertain result returns `owner_liveness_unknown`. Both outcomes leave every canonical byte unchanged. A stale checkpoint, absent lock, old timestamp, or fresh session is never liveness proof. BrainVault's observed Claude Code owner is currently active, so live acceptance must not recover ownership, stop its teams, or mutate its setup/state/TEAMS records; it proves the successful stopped-owner path with a BrainVault-shaped fixture instead.

The recovery publishes one ownership generation across `.agent-team/setup.json`, `.agent-team/state.json`, `.agent-team/TEAMS.md`, and `.agent-team/owner-history.json`. A write-ahead `.agent-team/.owner-recovery.json` journal records exact preimages/postimages, versions, operation signature, and same-directory temporary files. Canonical readers treat the journal as a transaction barrier and deterministically repair or refuse it; they never expose a mixed generation. Recovery acquires `owner-recovery.lock`, `setup.lock`, then `state.lock`, revalidates every fact under lock, increments setup/state/ownership versions once, changes only current owner fields, and preserves initialization, settings, dependencies, team rows, scope, task runtime, pending operations, and gate evidence. Exact replay returns the original receipt; a changed replay, divergent byte, or stale fingerprint fails closed.

Recovery grants coordination ownership only. It does not resume a run, replace task writers, alter scope, authorize integration/release/deployment, or launch settings. Cancel at native recovery confirmation writes nothing. A later setup settings Cancel preserves settings and does not roll back a separately committed owner recovery.

### 3. Initialization snapshots and live run scope are separate

The initialization task snapshot is immutable provenance for the adopted plan. Once initialization completes, readiness does not require the forever-changing live tracker to remain byte-identical to that snapshot.

Current work is controlled by `state.run.taskIds`. Add an owner-only, versioned scope-extension transition that:

- admits existing canonical tracker task IDs only;
- validates dependencies and duplicate identity;
- records the previous and new run scope, reason, owner, operation ID, and tracker fingerprint;
- does not claim, complete, or deploy a task;
- rejects paused projects, stale state, identity mismatch, missing tracker rows, and scope removal disguised as extension.

Completion, integration, release, and cleanup evidence must name only tasks in the current admitted run scope. Rejection occurs before evidence is receipted or state is mutated. Owner authorization is not equivalent to scope admission.

Historical completion migration is snapshot-driven and task-keyed. It discovers all tracker deliveries represented by the historical integration evidence but missing from current run/delivery history, then reconciles each against fresh fingerprints and explicit scope disposition. In the current BrainVault snapshot those deliveries are `brainvault-system-6zf` and `brainvault-system-0o4`; `brainvault-system-n8s` is the unrelated active completion singleton and must remain byte-identical. No hand editing, deletion, re-initialization, destructive singleton clearing, or hard-coded assumption that `6zf` is still active is permitted.

The general transitions use named CLI commands with `--project` and a bounded regular `--request` JSON file of at most 256 KiB:

| Command | Request body | Atomic result |
| --- | --- | --- |
| `completion-quarantine` | `taskId`, `reason`, and exact current evidence identity | Retain the original receipt and evidence pointers in quarantine history and remove them from active completion consideration. |
| `run-scope-extend` | nonempty `taskIds`, `reason`, and current tracker fingerprint | Add only existing canonical tracker IDs to live run scope without changing initialization provenance. |
| `completion-rebind` | `taskId`, quarantined receipt identity, exact evidence revision and fingerprints | Revalidate the already-reviewed evidence against current admitted scope and bind it once. |
| `run-reconcile` | complete proposed effective run values, authoritative source, reason, and affected task IDs | Replace contradictory or incomplete active run values without silently widening scope or enabling deployment. |
| `evidence-store-register` | declared store identity, TEAMS declaration fingerprint, exact root attributes, and owner generation | Register one bounded external evidence root without accepting arbitrary evidence-file paths. |
| `completion-history-reconcile` | task identity, expected canonical/owner/singleton fingerprints, registered-store relative evidence identities, source/integration revisions, operation IDs, scope disposition, and publication target | Import or repair immutable task-keyed completion/integration/publication history while preserving the active singleton and prior receipts. |

Each command uses the common mutation envelope: `schemaVersion`, expected canonical state version, unique operation ID, and request body. Authority comes from the matching observed native project-owner identity, not an actor string supplied by the file. Unknown fields, FIFOs, oversized input, stale versions or fingerprints, replay with a different signature, paused state, owner mismatch, missing tracker rows, scope removal, and substituted evidence fail before mutation. Exact replay returns the original receipt. A crash between transitions resumes from the last atomic version; it never loses the quarantined record or half-binds completion.

`completion-history-reconcile` is the compatibility path for historical receipts, including records no longer held in `state.completion`. It preserves an unrelated current singleton exactly, appends immutable migration history, adds scope only when explicitly requested and dependency-valid, and records an already-published task once without queuing a duplicate batch. It is blocked by an uncertain same-record writer or the unresolved `qac` transition, but unrelated inspection remains available. A registered external evidence store is addressed only by store ID plus normalized relative path. Registration binds the exact TEAMS declaration, real root, project, owner UID, owner generation, and fingerprint. Every component is revalidated without symlinks; opened evidence must be a bounded regular file beneath that root with the expected digest, and containment is checked again after open. Callers can never nominate an arbitrary absolute evidence file.

A present legacy run object remains active even if it lacks `id` and other 7.2 fields. `run-reconcile` assigns a request-bound compatibility ID and requires explicit `mode`, scope, team limit, auto-deploy, batch size, and per-field provenance. It preserves the existing scope unless a separate append-only transition is authorized and never substitutes mutable setup defaults. A genuinely absent run returns `run_not_active`.

Completion history has two revision roles. Each task's completion, review, and task-local checks bind its `sourceRevision`; batch integration, target, recovery, and publication evidence bind a common `integrationRevision` (also called the integration boundary). Every task source must resolve and be an ancestor of that boundary, and the boundary must be an ancestor of the freshly observed repository head. Equality is allowed but never required. One batch can therefore contain different task source revisions while retaining one exact boundary; later integration commits do not rewrite task evidence. Release authorization binds the exact task set and boundary and rechecks the live branch/target before execution.

### 4. Handoffs record a generation baseline, not their own containing commit

Project Kickoff keeps the current JSON shape for compatibility. `project.revision` becomes the exact generation/integration baseline observed when the handoff is produced.

The checker requires that revision to:

- resolve to a commit in the canonical repository;
- be an ancestor of the named current integration branch;
- remain compatible with `projectKickoff.approvedRevision` ancestry;
- pass all existing root, branch, tracker, size, identity, and actionability checks.

At `--emit-request`, the checker re-reads the actual current branch and tracker, validates that the stored baseline is still an ancestor, and binds the current tip into the Agent-Team initialization request. It never asks a committed file to predict its own commit hash.

A consumed handoff is immutable historical input. Agent-Team records its digest, producer version, compatibility baseline, and consumption operation. Later legitimate tracker additions use run-scope extension; they do not require rewriting or revalidating the consumed handoff. Project context supersedes stale “refresh and start” instructions once the initialization receipt is complete.

Compatibility is explicit:

- Project Kickoff 0.3.1 or 0.4.0 with Agent-Team tested version 7.0.2 is accepted through the documented legacy migration path.
- Existing Project Kickoff 0.4.1 candidates with Agent-Team tested version 7.1.0 remain compatible.
- Newly generated Project Kickoff 0.4.1 handoffs record Agent-Team tested version 7.2.0.
- Unknown or crossed producer/consumer pairs fail with the loaded checker version and required migration.

Historical BrainVault fields remain 0.4.0 and 7.0.2. They are labeled producer provenance, never rewritten to impersonate a newer generator.

### 5. Referenced skills are reused, never copied

Agent-Team distinguishes compatibility from lifecycle ownership.

For referenced skill dependencies such as LeanCTX, Superpowers, Ponytail, Impeccable, and React guidance:

- discover the host-selected project/user installation;
- validate required files, pinned version or revision, and relevant content hashes in place;
- allow preserved unrelated files when required pinned files remain exact and no conflicting entrypoint exists;
- record compatible unowned packages as `reused_unowned`;
- run functional and fresh-worker qualification against the selected existing path;
- never write an ownership sidecar, overwrite, delete, roll back, or copy a duplicate merely to manufacture provenance;
- report missing or incompatible packages as `manual_action` or `cannot_use` with the exact mismatch.

Only Agent-Team-managed installations receive ownership receipts and become eligible for managed update/removal. Executable dependencies such as Graphify, ast-grep, Serena, and Playwright retain their reviewed scoped installers.

### 6. LeanCTX and Graphify report independently

Dependency results are emitted per capability with explicit prerequisite edges. Reporting may say one capability is unavailable, but it may say that it blocks another only when the catalog declares that dependency.

LeanCTX does not gate Graphify. A LeanCTX provenance or functional failure cannot change Graphify's receipt. Graphify's code-only probe remains writable in an isolated worktree and validates `_origin`, accepting deterministic AST-origin `INFERRED` relationships while rejecting semantic or missing provenance.

The narrowed LeanCTX profile becomes usable when its existing installation passes compatibility plus functional/fresh-worker checks. Agent-Team does not run a broad LeanCTX initializer, create a second tracker, enable persistent coordination memory, or overwrite hooks.

### 7. `ast-grep` is the canonical executable

The catalog and worker-discovery contract use the absolute managed `ast-grep` executable. Bare `sg` is never accepted from `PATH`.

For backward compatibility, an `sg` sibling may normalize to the canonical executable only when both resolve inside the same pinned package root and package identity is verified. `/usr/bin/sg` and other unrelated aliases fail with an actionable collision diagnostic.

### 8. Orchestrator liveness is an explicit invariant

Within an active host turn, the project orchestrator runs this event loop:

1. consume user and worker messages;
2. classify new user input as replacement, addition, or status/question;
3. answer a status/question in commentary without ending the run;
4. reconcile every live worker and completed handoff;
5. route findings to the owning developer, integrate accepted work serially, and refill every eligible development slot while reserving review capacity;
6. wait for at most 60 seconds while work remains active, then emit a concise state/blocker heartbeat and repeat.

A blocker parks only the affected task. Independent eligible tasks continue. A user question is an interrupt, not a terminal state.

The orchestrator ends its turn only when all authorized work is complete, the user explicitly pauses/cancels, a security-sensitive or irreversible action needs authority, a truly global blocker leaves no safe work, or the host interrupts the turn. Recovery records active workers, slots, blockers, pending user decisions, operation uncertainty, and the next eligible dispatch.

No skill can guarantee periodic messages after a final response or host termination. If a global blocker needs the user and no work remains, the orchestrator asks one final blocking question and resumes from durable state on the next message.

The 60-second heartbeat is a new recurring schedule relative to Agent-Team 7.1.x. It exists only while an authorized orchestration turn has active workers; it creates no cron job, daemon, hosted monitor, or activity after the turn ends. Each heartbeat consumes the host's ordinary model and output-token usage. Exact incremental cost is unknown because account pricing and cache behavior are not exposed here; implementations must keep the message to one compact state line and must not create extra verifier/developer calls solely to populate a heartbeat.

### 9. Batch exhaustion and active run state are evidence-based

A configured deployment batch size is a preferred full-batch threshold, not a minimum release cardinality. The orchestrator never asks for a one-time exception merely because the final eligible set contains fewer than the configured batch size.

The orchestrator submits a smaller final batch only after proving one of these terminal conditions from canonical state:

- the admitted finite task set is exhausted;
- a continuous run's explicitly scoped task list is exhausted; or
- every remaining scoped task is blocked, no safe independent work is eligible, and the smaller batch is otherwise release-ready.

Open, in-progress, unclaimed-but-eligible, or unreconciled scoped tasks mean the run is not exhausted. The orchestrator continues supervision, claims/refills eligible work, or performs the supported scope/ownership reconciliation. It does not reinterpret “no agent currently running” as “no tasks remain.”

At run start, Agent-Team copies the effective run choices into active operational state with their provenance:

```json
{
  "mode": "finite | continuous",
  "taskIds": ["canonical delivery IDs"],
  "teamLimit": 1,
  "autoDeploy": false,
  "batchSize": 1,
  "source": "explicit_run | saved_default | compatibility_migration"
}
```

Active state cannot omit these values and later infer them again from mutable setup defaults. If setup, tracker, recovery, and active state disagree, deployment is held and the owner performs a versioned reconciliation. Independent safe development continues. The reconciliation records the prior values, chosen authoritative source, affected task IDs, owner, operation ID, and new operational version; it never silently enables deployment or widens scope.

Release selection counts unique eligible top-level delivery IDs, not commits, subagents, tests, subtasks, or narrative summaries. Every candidate delivery must carry exact integrated revision, review/check evidence, preview state when applicable, and deployment-target/recovery authority. Unlinked claims such as test counts in a status message are not release evidence.

Database migrations, Vercel, DNS, credentials, and other external gates block only their affected tasks or release batch unless the dependency graph proves they are global. While such a decision is pending, the orchestrator reports it immediately, continues unrelated teams, refills freed slots, and includes the unchanged blocker in its bounded heartbeat. A final blocking question is permitted only when no authorized independent work remains.

### 10. Status language separates history, runtime, readiness, and authority

Status output uses four distinct labels:

- `generatedBy` / `testedAgainst`: immutable historical artifact provenance;
- `loadedRuntime`: the actual host-selected installed skill path and version;
- `sourceCandidate`: a local repository version that is not yet installed;
- `readiness`: current functional and fresh-worker evidence.

Beads with no Dolt remote reports `local_only`, not failure. `auto_deploy` without a target reports `enabled_but_held: target_required`, never release-ready.

Deployment authority is target-scoped. BrainVault's existing reversible code-publication target is `origin/main`; its recorded identity and authority are preserved independently from Vercel, database migration, DNS, credentials, and production-check targets. A missing or held Vercel/database target cannot erase the `origin/main` record or block unrelated eligible work. Conversely, authority to push `origin/main` never authorizes a production deployment or irreversible migration.

### 11. Release artifacts install transactionally

Publishing and installing are separate verified operations. Both packages install from a freshly downloaded release archive whose checksum matches the published `SHA256SUMS`; a moving source checkout, matching version text, or locally built candidate is not installation evidence.

The installer opens the freshly downloaded archive once, verifies its named `SHA256SUMS` entry and embedded release metadata, rejects duplicate/unsafe/symlink/special entries, and derives an immutable file digest/mode/size map from those verified archive bytes. Under the install lock it materializes those bytes into a private, same-filesystem staging tree with exclusive regular-file creation, canonical modes, and no extras. It then re-reads and seals that staging snapshot against the archive-derived map before any target mutation. A moving source checkout is never an installation input, and reopening a replaceable archive path after verification is not authority.

All package files, Claude role files, and hook declarations for the selected `codex`, `claude-code`, or explicit `both` host and `user` or project scope come only from that one sealed staging snapshot. The installer revalidates staging before each target swap and rehashes every installed target afterward; archive, staged-tree, and installed-tree digests must agree. It snapshots existing owned targets, rolls back a partial multi-host failure, and retains staging until targets, configuration, and the receipt commit. Customized, unowned, and ambiguous resources are preserved and reported rather than overwritten. Reinstall of the same artifact is idempotent; downgrade requires explicit compatible authority. Optional Project Kickoff project hooks remain inactive unless separately approved.

Agent-Team extends its managed receipt with the artifact provenance fields defined under release identities plus the checksum-file digest, archive-derived package-content digest and file map, and per-host installed-tree digest/file map. It verifies `.agent-team-source.json` from the same archive bytes before staging. Project Kickoff adds the same narrow receipt-producing update and rollback contract for its complete package. A temporary archive, checksum file, or staging path may disappear after installation without losing release provenance. Rollback and uninstall use installed file digests and ownership records and never touch unrelated configuration or reused-unowned dependency skills.

### 12. Narrow implementation ownership

The authority-sensitive implementation boundary is explicit:

- `hooks/lib/owner-recovery.mjs` owns `validateOwnerRecoveryEnvelope`, `validateNativeRecoveryContext`, `recoverProjectOwner`, and `repairOwnerRecovery`; `hooks/lib/project.mjs` owns the owner-history/journal paths; `hooks/lib/canonical-state.mjs` enforces the journal barrier and cross-record ownership generation.
- `hooks/lib/initialization.mjs` creates generation-1 ownership; `hooks/lib/workflow-cli.mjs` and `hooks/agent-team-cli.mjs` expose `project-owner-recover` only when the native adapter supplies trusted context. `hooks/lib/settings.mjs`, `hooks/lib/task-transitions.mjs`, and `hooks/lib/policy.mjs` compare runtime-qualified identity and refuse mixed/in-progress ownership generations.
- `hooks/lib/task-transitions.mjs` owns legacy-run reconciliation and task-keyed completion-history mutation; `hooks/lib/canonical-state.mjs` owns delivery-evidence joins; `hooks/lib/policy.mjs` owns Git ancestry and live integration/release boundary enforcement. The evidence-store registry/path opener is shared by these transitions but accepts only a registered store ID and relative descendant.
- `hooks/lib/artifacts.mjs::verifyReleaseArtifact` owns the one-read archive authority object; `hooks/lib/install.mjs::installPackage` owns private archive-derived staging, target transactions, rehashing, receipt schema migration, and rollback. No downstream copy helper accepts the moving source checkout for release installation.
- Focused tests are `tests/hooks-owner-recovery.test.mjs`, `tests/hooks-initialization.test.mjs`, `tests/hooks-project-journey.test.mjs`, `tests/hooks-canonical-state.test.mjs`, `tests/hooks-transitions.test.mjs`, `tests/hooks-policy.test.mjs`, `tests/hooks-artifacts.test.mjs`, `tests/hooks-install-health.test.mjs`, and `tests/hooks-package.test.mjs`. BrainVault live-state acceptance is conditional and read-only when `owner_active`; stopped-owner, legacy-run, `6zf`+`0o4`/`n8s`, evidence confinement, crash/replay, ancestry, and archive-tamper behavior use immutable fixtures.

This is the minimum repair contract. It adds no polling service or automatic takeover, does not stop an owner to make recovery pass, and does not broaden setup Cancel into mutation authority.

## Alternatives rejected

### Trust any existing path

This avoids copies but could execute edited or incomplete skills. Content and structural compatibility must be verified before reuse.

### Continue requiring Agent-Team sidecars

This preserves simple ownership but incorrectly treats matching user-managed skills as unusable and pressures setup to create duplicate discovery paths.

### Keep exact handoff-tip equality and require manual refresh

This is cryptographically self-referential for a tracked handoff and leaves the artifact stale immediately after commit.

### Launch settings only when missing

This does not satisfy the requested predictable setup experience. Every setup invocation must offer settings, while Cancel preserves existing values.

### Add a background scheduler for orchestration updates

The host already owns turn lifecycle and worker controls. A second scheduler cannot guarantee execution after host exit and would create competing orchestration state.

## Verification

### Project Kickoff

- A handoff generated at commit A and committed at descendant B passes.
- A revision from an unrelated branch fails.
- Invalid approved-revision ancestry still fails.
- The legacy/current/new compatibility matrix passes only supported pairs.
- `--emit-request` binds the observed current tip while retaining the stored generation baseline.
- A consumed handoff remains historical after a legitimate run-scope extension.
- Real Agent-Team 7.2.0 initialization consumes the new 0.4.1 handoff.
- The pre-design `a296ce8` / `43b6b85ede01bbc63b0a85bad25a5e017b228b3f7789fae060e8c8ebc5b73aab` candidate is explicitly invalidated. The final 0.4.1 archive is rebuilt twice from the new exact revision after compatibility changes, has identical checksums, and receives fresh revision-bound release evidence.

### Agent-Team setup and state

- Every state-changing setup invocation reaches settings.
- Cancel creates no settings mutation and preserves exact existing settings.
- Confirmed changes remain atomic and versioned.
- Native identity mismatch fails before any mutation.
- `project-owner-recover` rejects shell/request/environment identity, wrong native CWD/project, absent or mismatched host approval, active owner with `owner_active`, and unknown liveness without changing setup, state, TEAMS, or owner history.
- A positively stopped historical owner plus fresh native approval publishes exactly one runtime-qualified ownership generation across setup/state/TEAMS/history; crash injection at each rename is journal-repaired, exact replay is duplicate, changed replay conflicts, and Codex/Claude identical session strings do not alias.
- Owner recovery neither resumes nor stops teams and remains separate from settings; recovery Cancel is byte-identical, and settings Cancel after recovery does not undo it.
- Completion for a tracker task outside `state.run.taskIds` fails without evidence/state changes.
- Scope extension admits only current tracker tasks and preserves immutable initialization provenance.
- A legacy present run without `id` receives a request-bound compatibility ID and explicit effective settings; an absent run returns `run_not_active`, and setup defaults are never inferred.
- Generic history reconciliation imports all snapshot-discovered historical deliveries, preserves an unrelated active singleton byte-for-byte, and confines evidence reads to registered store IDs and relative descendants.
- The current BrainVault setup-v11/state-v24 fixture reconciles both `6zf` and `0o4`, preserves the `n8s` singleton, records both as already published to `origin/main` once, and leaves the unresolved `qac` record blocked until its writer/operation is reconciled. Stale singleton/fingerprints, path escape/symlink/type/owner/digest drift, missing tracker rows, unresolved dependencies, and changed replay leave canonical bytes unchanged.
- Task completion/review/check evidence at two different source revisions can join one later integration boundary when both ancestry checks pass; unrelated sources, missing commits/tasks, mixed boundaries, and revision-role mismatches fail before mutation.
- Fresh native BrainVault sessions establish project identity; a subprocess `workdir`, CLI `--project`, linked worktree, or nested repository alone cannot satisfy native-CWD acceptance on either host.
- While BrainVault's observed owner remains active, live migration acceptance performs read-only snapshot checks plus the stopped-owner fixture and makes no takeover or canonical mutation; it never stops the active teams to manufacture eligibility.

### Dependencies

- Exact pinned skills without sidecars qualify as `reused_unowned`.
- Exact required files plus unrelated preserved files qualify without modification.
- Edited, incomplete, or conflicting installations remain unusable.
- Rollback/uninstall never touches reused unowned paths.
- LeanCTX failure cannot change Graphify readiness.
- Graphify 0.9.57 real code-only callback evidence passes with AST-origin `INFERRED` provenance.
- Canonical `ast-grep` works; `/usr/bin/sg` and unrelated aliases fail.

### Orchestrator behavior

Pressure scenarios verify that a user status question during active work receives an answer and the run continues; completed workers trigger review; freed slots refill; task-scoped blockers do not stop other work; unchanged active work emits bounded heartbeats; and a global blocker produces a durable recovery record before the final question.

Batch/run scenarios verify that a finite exhausted run automatically submits one eligible delivery against a larger configured batch size; a non-exhausted run with one completed delivery keeps working; a continuous scoped run flushes only at scoped exhaustion; all-blocked exhaustion permits the final smaller batch only when its release gates pass; contradictory setup/active settings hold deployment but preserve independent work; effective run settings remain fixed after setup defaults change; top-level delivery IDs are counted once; and unlinked test-count prose cannot satisfy release evidence.

Artifact-install scenarios cover tampered/missing metadata and checksums, duplicate/unsafe entries, archive replacement or deletion after verification, source-checkout mutation, mutation during or after staging verification, installed-byte drift, deletion of temporary inputs after success, exact reinstall, incompatible downgrade, customized-target conflicts, partial `both`-host failure and rollback, and uninstall. A dedicated tamper regression changes bytes after archive validation but before target copy and proves the installer either consumes the already verified archive-derived sealed snapshot or fails and rolls back before a receipt; it can never install the replacement bytes. Receipts for both packages retain exact release/source/archive, staged-content, and per-target installed identity. A two-target BrainVault scenario proves a Vercel or database hold does not erase or block an authorized `origin/main` code publication, and that the code-push authority cannot cross into production.

Skill-instruction changes follow writing-skills RED/GREEN pressure testing. Runtime changes follow code TDD, complete suite verification, independent review, exact artifact checks, and fresh host installation/readiness verification.

## Migration and release

1. Implement and independently verify Agent-Team 7.2.0 in isolated worktrees.
2. Publish Agent-Team 7.2.0 first and verify tag, workflow, release assets, checksums, Pages, and transactional receipt-backed installations on both hosts.
3. Invalidate the existing Project Kickoff `a296ce8` candidate and its `43b6...` archive, implement the compatibility correction, prove a real released Agent-Team 7.2.0 consumes the new handoff, and build a fresh deterministic 0.4.1 candidate with new exact-revision evidence.
4. Publish Project Kickoff 0.4.1 and verify both host installations and historical compatibility behavior.
5. Re-read BrainVault setup/state/TEAMS, native owner liveness, tracker, singleton, evidence-store declaration, Git ancestry, and publication target immediately before migration. If the current owner is active—as in the approved snapshot—perform no takeover or live state mutation, do not stop its teams, and prove owner recovery plus history reconciliation with BrainVault-shaped fixtures. Only after a naturally quiescent owner is positively proven stopped and the unresolved `qac` writer/operation is reconciled may a fresh native Codex or Claude Code session recover ownership with explicit native approval. Then register the declared evidence store, reconcile every snapshot-discovered missing historical delivery (`6zf` and `0o4` in the current snapshot) while preserving the observed active singleton (`n8s` currently), populate the legacy run from explicit owner decisions, qualify reused skills, rerun Graphify and ast-grep workers, and verify settings-cancel behavior. Every request uses freshly observed versions/fingerprints; no fixed singleton, version, task-only sequence, or HEAD equality is authoritative.
6. Preserve Beads local-only state. Preserve the existing `origin/main` code-publication target separately; hold only unconfigured or unauthorized Vercel, database, DNS, credential, and production-check targets until the user supplies their required decisions.

No force-push, tag replacement, duplicate ambiguous release operation, direct runtime-state edit, copied dependency skill, or historical provenance rewrite is allowed.
