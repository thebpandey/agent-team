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

## Design decisions

### 1. Settings runs on every setup invocation

Every state-changing `agent team setup` invocation enters the settings wizard after canonical project and dependency inspection. The wizard begins with the current effective settings and offers a first-class Cancel/Keep Existing action.

Cancel means:

- preserve every existing role, model, effort, fallback, run, dashboard, and deployment setting byte-for-byte;
- do not create a settings mutation or increment the settings version;
- do not roll back valid dependency preparation that completed earlier in the same setup invocation;
- continue to the setup/readiness summary with `settingsOutcome: "kept_existing"`.

Confirmed changes remain versioned atomic settings mutations. A timeout, missing answer, or host interruption is not consent to defaults and leaves settings unchanged. Read-only `status`, `health`, `help`, and `version` actions never launch the wizard.

### 2. Native runtime identity owns initialization

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

### 3. Initialization snapshots and live run scope are separate

The initialization task snapshot is immutable provenance for the adopted plan. Once initialization completes, readiness does not require the forever-changing live tracker to remain byte-identical to that snapshot.

Current work is controlled by `state.run.taskIds`. Add an owner-only, versioned scope-extension transition that:

- admits existing canonical tracker task IDs only;
- validates dependencies and duplicate identity;
- records the previous and new run scope, reason, owner, operation ID, and tracker fingerprint;
- does not claim, complete, or deploy a task;
- rejects paused projects, stale state, identity mismatch, missing tracker rows, and scope removal disguised as extension.

Completion, integration, release, and cleanup evidence must name only tasks in the current admitted run scope. Rejection occurs before evidence is receipted or state is mutated. Owner authorization is not equivalent to scope admission.

BrainVault migration uses supported transitions: quarantine the invalid out-of-scope `brainvault-system-6zf` completion receipt, extend the run scope if the task is intended to remain part of the run, then rebind its already-reviewed completion evidence. No hand editing, deletion, or re-initialization is permitted.

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

### Agent-Team setup and state

- Every state-changing setup invocation reaches settings.
- Cancel creates no settings mutation and preserves exact existing settings.
- Confirmed changes remain atomic and versioned.
- Native identity mismatch fails before any mutation.
- Completion for a tracker task outside `state.run.taskIds` fails without evidence/state changes.
- Scope extension admits only current tracker tasks and preserves immutable initialization provenance.
- BrainVault migrates from the invalid `6zf` receipt without hand editing.

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

Skill-instruction changes follow writing-skills RED/GREEN pressure testing. Runtime changes follow code TDD, complete suite verification, independent review, exact artifact checks, and fresh host installation/readiness verification.

## Migration and release

1. Implement and independently verify Agent-Team 7.2.0 in isolated worktrees.
2. Publish Agent-Team 7.2.0 first and verify tag, workflow, release assets, checksums, Pages, and both host installations.
3. Implement and qualify Project Kickoff 0.4.1 against the released Agent-Team 7.2.0.
4. Publish Project Kickoff 0.4.1 and verify both host installations and historical compatibility behavior.
5. From a native session rooted in BrainVault, migrate the invalid completion receipt through supported state transitions, qualify reused skills, rerun Graphify and ast-grep workers, and verify settings-cancel behavior.
6. Preserve Beads local-only state and the deployment-target hold unless the user separately configures a Dolt remote or release destination.

No force-push, tag replacement, duplicate ambiguous release operation, direct runtime-state edit, copied dependency skill, or historical provenance rewrite is allowed.
