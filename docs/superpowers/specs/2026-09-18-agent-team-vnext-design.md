# Agent-Team vNext: clean-room architecture specification

**Status:** approved design baseline. This specifies a new implementation; it
does not authorize edits to, execution of, or dependency on the v7 runtime.

## 1. Purpose and boundaries

Agent-Team vNext is a self-contained Go control plane for continuous plans and
one-off implementation runs. An orchestrator plans, admits bounded parallel
work, receives exact-revision review evidence, integrates serially, and invokes
only explicitly authorized idempotent deployments. Codex and Claude Code may be
switched at any safe point because durable truth is repository state, not a
host session.

### Goals

- High-confidence implementation with a developer → non-author reviewer →
  repair loop and one deterministic completion gate.
- Low context and tool overhead: short packets, one tracker, small receipts,
  task-specific instructions, and optional accelerators only when useful.
- Continuous, refillable teams: retain a completed team and give it another
  bounded list instead of repeatedly recreating orchestration context.
- Native Windows, macOS, and Linux behavior using portable files and host
  capabilities; no POSIX-only coordination assumption.
- Equivalent Codex/Claude operating model without a transfer protocol,
  cross-harness lock, or requirement to revive an old session.
- A single statically distributed Go binary; Git is the only core external
  requirement. Node.js and Python are not core runtime dependencies.

### Non-goals

- A daemon, autonomous scheduler, shared agent memory, telemetry product,
  dependency manager, MCP broker, IDE replacement, or generic project
  generator.
- Automatic permission escalation, process termination, port/PID cleanup,
  deployment-target discovery, or cross-project resource sharing.
- Reuse, port, invocation, or compatibility dependence on v7 hooks, lanes,
  locks, claim recovery, checkpoint journals, installer, dashboard runtime,
  dependency catalog, or Node hook code.

## 2. Core architecture

```text
User plan / optional Kickoff handoff
                 │
        Orchestrator (one active harness)
                 │ reads/writes
   TASKS.md (default) OR Beads (chosen once)
                 │
  Team record + compact task receipt + Git/worktrees
     ├─ Developer ──> non-author reviewer ──> FIX / CLEAN
     ├─ Developer repair <────────────────────────┘
     └─ orchestrator deterministic gate ──> serial integration ──> batch deploy
```

In `plan` mode the selected tracker is the sole task authority; in `one-off`
mode the immutable run manifest is that authority. Git is the sole revision
authority. The receipt is a compact resumption and evidence index, never a
second task ledger or a duplicate transcript. `CONTEXT.md` and `MISTAKES.md`,
when present, are project knowledge inputs and not ownership or completion
authorities.

### Components

| Component | Responsibility | Explicitly does not do |
| --- | --- | --- |
| Orchestrator | Plan/refine tasks, choose routing, admit teams, send packets, integrate and deploy | Write feature code or substitute its own review for CLEAN evidence |
| Tracker | Task IDs, dependencies, status, acceptance criteria, batch relation | Session/lease/process ownership |
| Team | A retained logical capacity with a serial queue of up to eight tasks | A nested orchestration system or permanent worker identity |
| Developer | Implement only assigned task and writable scope in its worktree | Self-approve or integrate |
| Reviewer | Inspect exact revision, execute applicable real checks, issue FIX/CLEAN | Author the reviewed change or invent missing tests |
| Gate | Deterministically validates receipt/evidence and Git preconditions | Guess semantic quality or call a model |
| Receipt | Minimal durable facts required to resume/reconcile its current team/task | Full context, memory, locks, or telemetry |
| Resource registry | Records team-owned server/browser use and observed state | Kills unknown processes or arbitrates across projects |

### Authority and precedence

Every durable record has `schema`, `project`, `runId`, `writtenAt`, and a
monotonic `revision`. A reader rejects an unknown schema and treats a malformed
or conflicting record as blocked, never as an invitation to guess. Authority is
ordered as follows:

1. Git refs and the actual worktree index/dirty state establish source truth.
2. Explicit user direction, `DECISIONS.md`, and project
   `AGENT_TEAM_RULES.md` establish applicable policy; a decision records its
   source and rules cannot override direct user direction.
3. The selected tracker in `plan` mode, or the immutable one-off manifest in
   `one-off` mode, establishes task identity, dependency, acceptance criteria,
   and task lifecycle intent.
4. The run record establishes the admitted set, team assignment, refill batch,
   and authorized deployment batch.
5. The retained-team receipt establishes the latest observed attempt,
   candidate/canonical/integration revisions, evidence pointers, resource use,
   and next action for that team's queue.
6. Evidence establishes only the command/reviewer result it records; it cannot
   override a mismatched tracker, Git revision, or receipt.

On disagreement, Git/worktree dirtiness wins over every claimed revision; the
tracker or one-off manifest task identity wins over receipt task text; and a
newer receipt revision wins only after its `runId`, team, queue fingerprint,
and Git references pass validation. `CONTEXT.md`, `MISTAKES.md`, optional-tool
output, dashboard, and host session state are advisory inputs with no authority.

### Run modes and control boundary

`plan` mode requires the selected authoritative tracker and supports refillable
retained teams. `one-off` mode requires no tracker: it writes a compact immutable
run manifest under `.agent-team/runs/<run-id>.json`, accepts one bounded request
with acceptance criteria and writable paths, and admits at most two safe,
independent teams. One-off tasks must be disjoint in paths/resources and share
no integration candidate; otherwise they run serially. A one-off manifest has
the same run, receipt, evidence, review, gate, and cleanup contract as plan
mode, but creates no `TASKS.md` or Beads record.

The canonical branch (`main` unless configuration explicitly chooses another
protected integration branch) is a control and serial-integration checkout only.
The orchestrator never authors product code there. Product edits occur only in
assigned worktrees through a delegated developer; the orchestrator may create
worktrees, route/review/integrate work, update derived artifacts, and execute
authorized deployment commands.

### Durable decisions and blockers

`DECISIONS.md` at project root is the canonical append-only consequential
decision log. Each entry has stable `DEC-<number>` ID, date, scope, decision,
rationale, alternatives considered, and evidence pointers. Receipts reference
decision IDs rather than duplicating decisions. `BLOCKERS.md` is a live derived
view of open entries: stable blocker ID, severity, affected task/team/run,
exact requested choice, safe default, evidence pointer, and state. It is
regenerated from tracker/run/receipt/decision facts and is never an authority.

A recurring, broadly applicable accepted decision is promoted by explicit
orchestrator action into project-root `AGENT_TEAM_RULES.md`; it retains its
`DEC-*` provenance. `AGENT_TEAM_RULES.md` governs project-specific execution
after explicit user direction, while the installed immutable `WORKER-CONTRACT`
governs common safety, scope, receipt, review, and evidence behavior. Assignment
packets are immutable and include digests of both documents; user direction and
project rules override generic worker guidance.

## 3. Lifecycle and public actions

### Setup and bootstrap

`setup` is an interactive, idempotent project bootstrap.

1. Detect the project root, Git availability, requested run mode, existing
   plan/tracker, optional Project Kickoff handoff, and installed immutable
   `WORKER-CONTRACT` digest.
2. In plan mode, if artifacts are missing, show the exact files to create and
   ask before creating them. Required plan-mode artifacts are the selected
   single tracker (root `TASKS.md` unless Beads is selected),
   `.agent-team/config.json`, `.agent-team/receipts/`, `DECISIONS.md`, and
   `AGENT_TEAM_RULES.md`. In one-off mode setup creates only the compact run
   manifest, receipt/evidence directories, and derived blocker view.
3. Default to `TASKS.md`. Offer Beads only as an explicit tracker selection;
   never maintain both as live authorities.
4. Run the model-role wizard and optional capability prompt described below.
5. Discover the project validation commands and record only approved commands
   in config. No command is inferred as a deployment command.

The public vocabulary has two deliberately separate namespaces. **Skill
actions** run in a project and never install or alter host configuration:

| Skill action | Exact form and meaning | Owning phase |
| --- | --- | --- |
| `setup` | `setup [--mode plan\|one-off]`; idempotently initializes/validates the selected project artifacts after any required confirmation | 1 |
| `settings` | `settings [key=value ...]`; views or changes project config after explicit confirmation; it cannot change host enablement, hooks, MCP, or model-provider settings | 1 |
| `status` | `status [--run <run-id>]`; read-only bounded state and derived blocker summary | 1 |
| `start` | `start [--run <run-id>] [--task <task-id>...]`; admits an existing plan-mode run after preflight, optionally limited to named ready tasks; it does not create tasks | 1/2 |
| `task add` | `task add --queue\|--execute <objective>`; creates one plan-mode task in the selected tracker, then queues it or starts foreground execution as selected, subject to the 1,000-task limit | 1/2 |
| `one-off` | `one-off feature\|audit\|review <objective>`; creates a trackerless immutable manifest. `feature` permits only packet-scoped product changes and uses the FIX/CLEAN review, gate, and integration loop; `audit` is read-only; `review` inspects an exact existing revision and returns only FIX/CLEAN. Read-only kinds cannot integrate/deploy. Each kind admits at most two independent teams and serializes overlap | 1/2 |
| `pause`, `stop`, `cancel`, `resume` | `pause\|stop\|cancel\|resume [--run <run-id>] [--team <team-id>] [--task <task-id>]`; scoped control semantics are defined below | 1/2 |
| `inspect` | `inspect [--run <run-id>] [--team <team-id>] [--task <task-id>]`; reads bounded tracker/manifest, Git, receipt, evidence, and resource facts without dispatch or mutation | 1 |
| `cleanup` | `cleanup [--run <run-id>] [--team <team-id>] [--task <task-id>]`; performs only safe manifest-owned terminal cleanup and reports protected candidates | 2/3 |
| `deploy` | `deploy [--run <run-id>] [--batch-size <N>] [--target <profile>]`; invokes only an already-authorized idempotent deployment executor. Omitted values use preconfigured defaults; the core derives the immutable batch fingerprint and idempotency key from exact task IDs, canonical revisions, and target | 4 |

`review`, `gate`, `integrate`, `reconcile`, and receipt checkpoints are internal
orchestrator transitions, not public action names: they occur only within the
above execution lifecycle and cannot be bypassed by a user-facing alias. Hosts
may spell the listed skill actions idiomatically, but must preserve their exact
parameters/meaning and record the canonical action name in the run record.
There are no legacy aliases, lifecycle hooks, background wakeups, automatic
prompt expansion, or hidden tool callbacks.

For every action above, `--run` is optional only if exactly one active run is
unambiguous; otherwise the core rejects the request and requires it. `--team`
and `--task` narrow an identified run and are rejected when they do not belong
to it. With neither narrow flag, a scoped control action applies to the run;
project-wide pause/stop requires explicit `--project` confirmation and is never
implied. `task add --queue` appends one task to a transactional admission batch
of at most eight; `task add --execute` performs that append then enters the same
foreground admission path as `start`.

Task transition graph: `ready → implementing → reviewing → (fix →
implementing | clean → gated → integrated)`. The only reviewer verdict values
are `FIX` and `CLEAN`; `CLEAN` is not completion until gate and serial
integration succeed. `pause` writes the existing team receipt and leaves its
scope `paused`. `stop` requests no new work from the selected run/scope and
records the observed stop result; an unconfirmed worker becomes `interrupted`,
not assumed stopped. `cancel` is explicit, records rationale, blocks new work,
and leaves source/evidence untouched unless an already-authorized revert is
separately run. `resume --run` reconciles its requested scope before it becomes
eligible again. Task/team pause leaves unrelated lanes eligible; run pause or
stop halts that run's admission/refill; project pause or stop halts all new
project admission. Any failed/unknown external operation moves its task to
`blocked` or `interrupted`, never back to ready automatically. A consequential
fact causes an internal atomic receipt checkpoint; the one derived run handoff
is regenerated from that receipt/checkpoint and never stores an independent
state ledger.

**Bootstrap management commands** are deliberately not skill actions:
`agent-teamctl install`, `agent-teamctl update`, `agent-teamctl rollback`, and
`agent-teamctl uninstall`. They are phase-5 distribution/cutover operations,
not project lifecycle calls; `install`/`update` are also needed before phase-1
skill actions can be invoked on a host.

### Bootstrap integrity and host-enablement boundary

One `agent-teamctl install --host codex|claude|both` operation handles the
explicit host selection. It downloads or accepts a static Go release, verifies
the published version and SHA-256 checksum before activation, stages it in the
platform user-data directory, verifies the immutable `WORKER-CONTRACT` digest,
then atomically installs a manifest listing exact paths, hashes, selected
host(s), and version.
The only host-visible files it may create are its own minimal static
`agent-team-vnext` skill entrypoint in the chosen host's user skill directory
and the matching manifest-owned contract reference. It may write only inside
the Agent-Team user-data root and those exact manifest-owned skill-entrypoint
paths.

Bootstrap never writes, patches, enables, disables, or registers host settings,
hooks, MCP servers, plugins, model settings, project files, shell profiles,
environment variables, credentials, or existing skills. It does not install a
daemon or an always-running service. If a host has no safe recognized user-skill
directory, installation leaves that host unenabled and documents native command
invocation; it does not guess a configuration path. `update` repeats staged
checksum/contract verification, atomically swaps only unchanged manifest-owned
files, and retains a versioned local backup plus prior manifest. `rollback
--version <version>` restores that exact verified backup atomically. `uninstall`
removes only exact hash-matching manifest-owned binary/contract/entrypoint
files; changed, unknown, or shared files are reported and retained. None of
these commands creates a hook, MCP registration, or settings mutation.

### Optional Project Kickoff

Project Kickoff is optional. On a validated handoff, the orchestrator reads the
plan and selected tracker reference once, checks structural fields, and copies
only missing project facts after user approval. It creates no active second
ledger. Kickoff's old SessionStart context loader may be used only during
Kickoff planning and is disabled at handoff; its PreToolUse and PostToolUse
hooks are not part of vNext. Native Windows uses ordinary read-on-resume.

### Preflight and portable filesystem contract

Before setup, start, resume, assignment, integration, or archive, the host
adapter performs a bounded preflight: canonical realpath and Git repository
identity; worktree branch/HEAD/detached state; index/working-tree dirtiness;
read/write accessibility; disk headroom for requested artifacts; and path
containment. It rejects path aliases caused by case folding, Unicode
normalization, Windows reserved names, or `.`/`..` ambiguity. Every assigned
writable path and artifact path must remain under the canonical worktree after
resolving symlinks (or Windows reparse points); a link/reparse traversal outside
is blocked. Core records require neither symlinks nor executable bits.

Long-command arguments are never interpolated into shell strings and never
written in an assigned product worktree. For each command, the Go core creates
an argument file in its per-run transient subtree
`<canonical-git-common-dir>/agent-team/runtime/<run-id>/evidence-tmp/<task-id>/<attempt>/<command-id>.args`.
The common Git directory is outside product diffs; the core verifies it is not
a worktree, is canonical/contained, and is writable before admission. This
subtree is owned solely by the current OS user running the Go core, has no
worker write authority, and holds only manifest-owned argument files and
unpromoted command-output staging files. It is not a tracker, receipt, or
evidence authority.

Each argument file is at most 1 MiB and all argument/staging files for one
attempt are at most 10 MiB (both configurable downward only). The core writes a
unique same-directory temporary file with restrictive owner permissions where
the platform supports them, flushes and closes it, validates its byte count and
content digest, then atomically renames it to the final argument-file name
before starting the child process. The child receives the absolute argument-file
path through a native argument array, not a shell. Command output is staged in
the same subtree and promoted only after its bounded evidence record is written
atomically; receipts retain only the final evidence pointer and digest.

After a command's result and promoted evidence are durably recorded, the core
removes only that exact manifest-owned transient file. Terminal safe cleanup
removes an otherwise-empty per-run subtree; an unresolved command, failed
promotion, or sharing lock preserves it for `inspect`/`resume` and marks the
operation blocked. The adapter probes same-directory atomic replace before
writing canonical records. On a filesystem that cannot provide it (including a
network filesystem), vNext uses a declared single-writer foreground action and
read-after-write checksum, serializing all record writes; it does not emulate a
lease. Windows create, rename, and cleanup sharing violations retry a bounded
number of times with backoff. On exhaustion, last-good canonical state and the
locked transient path are preserved, and the operation is blocked rather than
falling back to a product-worktree path. Detached, dirty, inaccessible,
low-disk, uncontained worktrees, or inaccessible common Git directories are
reported precisely and are not admitted until their applicable precondition is
resolved.

## 4. Configuration and durable records

All schemas are JSON or Markdown and use repository-relative paths plus Git
object IDs. They must be valid on Windows, macOS, and Linux.

### `.agent-team/config.json`

```json
{
  "schema": 1,
  "runtime": {"kind": "go-static", "workerContractDigest": "sha256:<...>"},
  "tracker": {"kind": "tasks-md", "path": "TASKS.md"},
  "models": {"orchestrator": "user-selected", "developer": "user-selected", "reviewer": "user-selected"},
  "limits": {"parallelTeams": 4, "teamQueueMax": 8, "projectTaskCapacity": 1000, "devServers": 2, "browserSessions": 2, "handoffWarnBytes": 196608, "handoffHardBytes": 262144, "argumentFileBytes": 1048576, "argumentStagingPerAttemptBytes": 10485760, "commandLogBytes": 1048576, "attemptLogBytes": 10485760},
  "storage": {"trackerWarnPercent": 90, "trackerHardBytes": 2097152, "canonicalHardBytes": 16777216, "screenshotsPerAttempt": 20, "screenshotHardBytes": 10485760, "evidencePerAttemptBytes": 104857600, "activeEvidenceWarnBytes": 524288000},
  "checks": {"format": [], "type": [], "test": [], "build": [], "visual": []},
  "deployment": {"enabled": false, "batchSize": 1, "executor": "explicit-command", "authorization": "user-approved"},
  "capabilities": {"leanctx": "optional", "serena": "optional", "graphify": "optional", "astGrep": "optional", "playwright": "optional"}
}
```

`parallelTeams` is chosen in setup, capped by the host-native delegation
capacity, and may be lowered per run. Configuration stores preferences and
validated commands, never API keys, session IDs, process IDs, raw logs, or
private prompts.

The installer supplies an immutable, checksum-verified `WORKER-CONTRACT` with
the static Go binary. The core installation has no Node.js or Python runtime
dependency and performs no host hook/MCP mutation. A project may add or amend
`AGENT_TEAM_RULES.md` only through explicit project decision records.

Limits are configurable downward only except where an explicit user change is
within these safe hard ceilings: tracker 2 MiB, all canonical vNext records
16 MiB, handoff 256 KiB (warn at 192 KiB), argument file 1 MiB, transient
argument/output staging 10 MiB per attempt, command log 1 MiB, attempt logs
10 MiB, 20 screenshots per task attempt at 10 MiB each, 100 MiB evidence per
attempt, and a 500 MiB active-project evidence warning. Receipts contain short
pointers and bounded fields, not payloads. Commands preserve full external logs
at their pointer while keeping bounded head/tail summaries in evidence. No cap
causes auto-delete: completed/deployed material is recoverably archived with
IDs/evidence preserved. A failed write preserves last-good canonical record,
marks the operation blocked, and reports the exact exceeded limit.

`projectTaskCapacity` is fixed at 1,000 for vNext's bounded tracker load and
admission window; it is distinct from `teamQueueMax` (one to eight tasks per
retained team batch). Both `TASKS.md` and Beads adapters count authoritative,
non-archived tasks. At 900 they emit a visible warning. At 1,000 they still
read, resume, review, integrate, deploy, and archive existing work, but reject
only a new task creation or admission that would exceed 1,000. Beads may store
more history; Agent-Team does not load/admit more than its 1,000-task window.
Completed/deployed history may be recoverably archived with stable task IDs,
status, revisions, and evidence pointers preserved. Nothing is silently
deleted or auto-archived, and in-flight, paused, blocked, or release-pending
tasks are never archival candidates.

Tracker adapters maintain an indexed, paginated ready-task window rather than
loading all 1,000 records into an orchestrator prompt. Selection reads the
smallest dependency-ready page needed to fill usable developer capacity, then
revalidates tracker revision, task status, scope/resource fingerprint, and
capacity immediately before admission. Imports preserve valid IDs/evidence and
quarantine malformed unrelated records in a recoverable adapter-specific
location when safe; a malformed record that participates in the requested
dependency path blocks that path instead of being ignored.

### Run and team records

`runs/<run-id>.json` contains the immutable plan revision, selected tracker,
admitted task IDs, model choices, current batch ID/fingerprint, and canonical
revision. A team record contains:

```json
{"schema":1,"runId":"R-01","revision":9,"teamId":"T-03","queue":["TASK-41","TASK-44"],"queueFingerprint":"sha256:<...>","state":"working|idle|paused|blocked|cancelled",
 "worktree":".worktrees/t-03","base":"<git-oid>","writablePaths":["src/payments/**"],
 "resourceRefs":{"servers":["S-1"],"browsers":["B-1"],"external":["db:test"]}}
```

A nonempty team queue is the active ordered set of one to eight tasks only. It
never contains completed tasks; an idle team has an empty active queue.
Admission history is separate append-only records at
`admissions/<team-id>/<batch-id>.json`, each with task IDs, prior/new queue
fingerprints, tracker revision, and outcome. A refill is an append-only
transaction: write a proposed batch `{batchId, previousRevision, teamId,
taskIds, queueFingerprint, admittedAt}`, verify all task/path/resource
preconditions against the same tracker revision, append it once, then publish
the new team/run revision atomically. Retrying the same `batchId` and
fingerprint is a no-op; any changed input is a new batch. The orchestrator never
silently changes an active task's writable paths.

### Per-retained-team receipt and evidence

`receipts/<team-id>.json` is the one bounded receipt for a retained team. It
contains active/current facts, resource reference arrays, and compact completed
summaries or archive pointers—not completed task payloads. Stable per-task
completion identities live at `evidence/<task-id>/<attempt>/completion.json`
and are referenced from tracker/admission history; refills cannot grow the
active queue or team receipt. The receipt is overwritten atomically after
consequential facts, before a voluntary handoff/pause, and after
review/gate/integration. It is small enough to read on every resume.

```json
{
  "schema": 1, "runId":"R-01", "revision":12, "team":"T-03", "queueFingerprint":"sha256:<...>",
  "task": "TASK-41", "attempt": 2, "state": "implementing|reviewing|fix|clean|gated|integrated|paused|blocked|cancelled|interrupted",
  "worktree": ".worktrees/t-03", "dirty":"clean|observed-dirty|unknown", "base": "<git-oid>", "candidate": "<git-oid>", "canonical":"<git-oid>|null", "integration":"<git-oid>|null",
  "owner": {"team": "T-03", "harness": "codex|claude", "status": "observed|unknown|stopped"},
  "scope": ["src/payments/**"], "checks": [{"name":"test:payments","result":"pass","evidence":"evidence/TASK-41/test.txt"}],
  "review": {"reviewer":"role-id", "revision":"<git-oid>", "verdict":"FIX|CLEAN", "evidence":"evidence/TASK-41/review.md"},
  "gate": {"schema":1,"revision":"<git-oid>","result":"pass|fail","evidence":"evidence/TASK-41/gate.json"},
  "resources": {"servers":["S-1"], "browsers":["B-1"], "external":["db:test"]},
  "accelerator": {"name":"none|leanctx|serena|graphify|ast-grep", "version":"optional", "result":"short pointer"},
  "next": "exact next action", "updated": "RFC3339"
}
```

Evidence files are revision-bound and contain command, exit result, concise
output pointer, input fingerprint, and reviewer finding/resolution. Raw large output remains in
the project/host artifact location named by the pointer; it is never injected
into every worker context.

`handoffs/<run-id>.md` is a derived run-level Markdown view, regenerated from
the run record, active team receipts, decision IDs, and blocker state. It lists
only current revision, team/task/attempt, CLEAN/FIX/gate state, resource refs,
open blockers, and next actions. It is bounded by the handoff limit, never
authoritative, and cannot contain worker transcripts or a second task ledger.

### Worktree, branch, and cleanup ownership

The Go core owns worktrees it creates under the configured project worktree
root and records canonical path, branch, base, candidate, and team ID. On a
successful terminal task, it automatically performs safe cleanup by default:
remove only its clean, merged, unpinned worktree and its derived transient
artifacts after completion evidence and serial integration are recorded. It
never removes a dirty, unmerged, blocked, paused, release-pending, user-owned,
or unknown worktree; these become explicit cleanup candidates.

Before it removes a managed worktree or its transient subtree, safe cleanup
separately asks the resource registry to gracefully stop each exact
manifest-owned server/browser reference bound to that team, worktree, and
revision, but only after the existing no-known-consumer and evidence gates pass.
It records confirmed stop evidence before filesystem removal. A failed or
unknown stop leaves that exact resource slot occupied and leaves cleanup
blocked/protected; it never kills by port/PID, stops another worktree's
resource, or treats resource cleanup as permission to remove an otherwise
protected worktree.

Cleanup boundaries are strict. Core removes only its manifest-owned transient
run/receipt/cache files, the exact per-run common-Git-dir argument/output
subtree described above, and merged managed worktrees. It never deletes
branches unless the branch is manifest-owned, merged, and not configured as a
backup reference. It never changes databases, schemas, stateful services,
backups, external storage, user-created files, or unknown processes. Database
and external-resource cleanup requires a separately authorized project command
with idempotency/evidence just like deployment.

## 5. Model and role routing

The setup wizard asks the user to select available providers/models and shows
recommendations, never defaulting to Astra. Roles are functional:

| Role/risk | Initial recommendation | Adaptive downgrade/escalation |
| --- | --- | --- |
| Orchestrator | User-selected capable model | Keep stable for a run; change only at an explicit resume boundary |
| Ordinary developer/reviewer | Codex Terra medium or Claude Sonnet 5 medium | Luna/Haiku only after demonstrated low-risk task success |
| High-risk/security/migration | Codex Sol high or Claude Opus 5 high | Never downgrade solely for queue pressure |
| Mechanical dashboard refresh | Luna or lowest-cost suitable model | Escalate only if the renderer/check fails |

Per-task override wins. Availability is verified before assignment; unavailable
choices become a routing constraint, not a silent model substitution. Defaults
are evaluated on 20–30 historical diffs for misses, false positives, token use,
elapsed time, and repair cycles before they are locked.

Host adapters translate model labels, availability, and native delegation calls.
Each implements `Probe()`, `StartWorker(packet)`, `StartReviewer(packet)`,
`Poll(handle)`, `Stop(handle, scope)`, and `ReadIdentity(handle)`. A worker
handle includes host, immutable packet digest, run/team/task/attempt, and
observed identity. The adapter starts a separate reviewer context and the core
rejects CLEAN when reviewer identity equals author identity. Adapters never
write product files directly, configure host hooks/MCPs, or create a hidden
cross-host ownership lock.

| Host / OS | Supported vNext core | Model label treatment |
| --- | --- | --- |
| Codex on Windows, macOS, Linux | Yes after host-native delegation and Git acceptance tests | Resolve user-selected Codex label to a verified available model; record requested and resolved labels |
| Claude Code on Windows, macOS, Linux | Yes after host-native delegation and Git acceptance tests | Resolve user-selected Claude label likewise; never represent a Codex model as Claude-native |
| Any untested host/OS pair | No execution claim | `setup` reports unsupported; tracker/receipt remain readable |

The adapter has no persistent compatibility cache and does not alter host
configuration to make a label appear available. The Go core's worktree manager
implements `Create(team, base)`, `Inspect(team)`, `Integrate(candidate)`, and
`Cleanup(team)` using Git argument arrays; host adapters receive absolute
assigned worktree paths and no authority to select another checkout.

## 6. Admission, parallelism, and resource safety

The orchestrator admits only tasks whose dependencies are complete and whose
writable paths/resources do not overlap. Disjoint paths may run in parallel.
Shared files, schema/migration surfaces, canonical integration branch, release
target, declared external resource, port, and any shared server/browser are
serialized. A task declares external resources and requested port(s) before
admission. A port collision blocks/queues the later task; it is never solved by
killing a process or picking an unrecorded port. If scope is uncertain, the task
waits or is split; uncertainty is not treated as free capacity.

Only one harness is active for a project at a time. The user stops an old
Codex/Claude run by workflow convention before switching. A switch immediately
continues with the new harness—no coordinator transfer, writer lease, manual
release, process-stop proof, or return to the old session. It reads bounded
tracker/Git/receipt/worktree/resource facts and marks prior attempts
`interrupted`. Pending external operations are reconciled by their idempotency
key/provider operation ID before retry. The known caveat of an actually
still-running old worker is recorded and its paths/resources are serialized;
it is not a PID/liveness gate that delays the switch.

### Effective capacity and disposable workers

`configuredSlots` is the user setting and `observedSlots` is host-native
concurrent-context capacity. `usableSlots = min(configuredSlots, observedSlots)`
when observed, otherwise `1`. For `usableSlots = 0`,
`developerSlots = reviewerSlots = 0` and dispatch is blocked. For
`usableSlots = 1`, `developerSlots = 1`, `reviewerSlots = 1` as sequential
tokens: the same physical slot runs a developer context and then a distinct
non-author reviewer context, never both concurrently. For `usableSlots = N ≥ 2`,
reserve `reviewerSlots = 1` and set `developerSlots = N - 1`; concurrent
developer admissions never exceed `developerSlots`, and the reserved review
token prevents a completed candidate from lacking eventual non-author review.
No task enters `implementing` until one eventual non-author review token is
reserved. The configured team maximum is never an entitlement to over-admit. A
same-harness duplicate
invocation warns and refuses new admission unless it is an explicit `resume`
for the recorded run; this is record validation, not a lock or lease.

Retained teams preserve queue and receipt identity, not a live worker. Workers
are disposable. Packets have explicit byte budgets: compact common contract,
task criteria/scope/checks, receipt pointer, and only digest-validated skills;
large facts stay in pointers. Compaction, eviction, or a lost worker returns to
the same bounded `resume` reconciliation. Every host message includes
`runId`, `teamId`, `taskId`, `attempt`, candidate revision, and monotonically
increasing sequence. A stale/duplicate message is a no-op after receipt
fingerprint validation.

“Continuous” means foreground execution during an active host turn: after a
completed task it may refill eligible capacity, but it neither creates a daemon
nor wakes/retries/displays progress outside a user-initiated active host turn.

Supervision is event-driven during that foreground turn: the core reacts to
worker completion, review result, gate result, declared resource transition,
or explicit user action. A blocker propagates only through dependent tasks and
shared resource/integration boundaries; disjoint lanes continue. A lane blocker
does not pause the project. Project pause is explicit user scope or a critical
shared authority, integration, release, or safety blocker. The orchestrator
records and delegates remediation but never directly edits product files.

### Development servers and browser sessions

At most two Agent-Team-managed development servers and two browser sessions are
active per project. No project shares either with another project. A server is
bound to exactly one worktree and Git revision; two different worktrees may
preview concurrently. Reuse only a server with the same served revision.
Further server-dependent checks queue while coding continues. There is no
automatic switch-back. Gracefully stop a known managed server when it has no
known consumer; an unknown/unverified process occupies a slot and is never
killed from port/PID alone. Each browser session is owned by one team and
recorded with worktree/revision, target URL, start evidence, and screenshot or
trace pointers. It is reusable only for the same revision and test purpose;
otherwise it queues. A known managed browser closes gracefully after evidence
is captured and no known consumer remains; unknown browser state occupies a
slot. Crash artifacts, traces, and video use the same per-attempt evidence cap;
their pointer and failure classification are written before a replacement is
requested. A host may expose fewer than two server or browser slots; effective
host capacity wins. A required browser/server check stays queued or blocked
when no compatible slot exists, while advisory work reports the limitation. A
static dashboard is not a development server.

The project resource registry is a versioned map with arrays
`servers: [ServerRecord]` and `browsers: [BrowserRecord]`, each capped at two;
records are keyed by stable `S-*`/`B-*` IDs and include team, worktree,
revision, target, lifecycle state, and evidence pointers. A retained-team
receipt stores only reference arrays, never a singular resource object or a
copy of registry payloads.

## 7. Execution, review, gate, integration, and deployment

An immutable task packet has `{schema, runId, task, team, base, criteria,
scope, checks, capabilities, skills, resources, receipt}`. Each `skills` entry
is `{name, path, digest, purpose}`. The reviewer receives a packet derived from
the developer packet plus exact candidate revision and changed-file fingerprint;
the gate receives only its independent schema fields and referenced evidence.

1. The developer receives a minimal packet: task criteria, worktree, writable
   paths, exact base revision, applicable checks, selected capabilities, and
   receipt path. It implements and records candidate revision/check evidence.
2. A non-author reviewer receives the candidate revision and same criteria. It
   runs/inspects real project checks and semantic requirements, then returns
   `FIX` with actionable findings or revision-bound `CLEAN` evidence. There is
   no extra fixed verifier.
3. On `FIX`, the developer repairs; the same reviewer rechecks the new exact
   revision. A reviewer may not review its own authored change.
4. Only the orchestrator invokes one independent clean-room deterministic gate.
   Its `gate.json` schema is `{schema, runId, task, candidate, base,
   trackerRevision, receiptRevision, requiredCheckFingerprints, cleanEvidence,
   worktreeDirty, scopeFingerprint, result, checkedAt}`. The gate validates
   those fields, task/revision match, successful required checks, CLEAN
   evidence, clean intended scope, and integration preconditions. It does not
   import v7 `gate-evidence`, infer/guess tests with AI, or accept an
   unversioned reviewer assertion.
5. The orchestrator serially integrates a passed candidate into the canonical
   branch and records the integration revision. If integration mutates the
   candidate (conflict resolution, formatting, generated source, or any changed
   tracked file), it creates a new candidate identity, requires the reviewer to
   return a new CLEAN for that identity, and reruns the gate before marking the
   task integrated. It then delegates one dashboard refresh. The dashboard
   refresh never recursively becomes an integration task.

Deployment is disabled unless setup records an authorized target profile,
immutable executor command/argument template, approval scope, live verification
command, and idempotency-key strategy. The Go `DeploymentExecutor` implements
`Submit(batch)`, `Query(operation)`, and `Verify(operation)` through argument
files and bounded command execution; it never infers a provider or target.
Eligible completed tasks form ordered batches of configured/default or explicit
`--batch-size`; the final nonempty remainder is valid. Before submit, the core
canonicalizes the ordered exact task IDs, their canonical revisions/artifacts,
and the selected target profile, then derives and records an immutable batch
fingerprint and idempotency key from those values. Each deployment receipt
contains those inputs, fingerprint, idempotency key, provider operation ID,
authorization reference, and live verification result. Replaying an unknown
operation first calls `Query`; it never blindly deploys again. A failed or
ambiguous batch halts further deployment, preserves evidence, and requires
explicit repair/recovery evidence before continuation. Coding on unaffected
work may continue.

## 8. Dashboard

After each successful implementation integration, a Luna or lowest-cost
suitable worker refreshes the ignored local snapshot
`<project>/.agent-team/dashboard/index.html`. Its schema is
`{schema, generatedAt, canonicalRevision, status, teams, resources,
integrations, evidenceLinks}` and its visible status is `current`, `stale`, or
`unavailable`. Rendering writes a temporary sibling then atomically replaces
the snapshot; on failure it preserves the last-good snapshot and marks/records
the refresh failure rather than presenting partial content as current. It shows
task status semantics, current canonical revision, last integration, current
team/resource summary, and evidence links without secrets. Setup/settings/README
state its location, local-only/cross-host limitation, refresh timing, and
opening method: open the file directly in a browser; an optional project-owned
loopback server is a separate explicit action. The snapshot is never a tracker,
deployment record, or server.

## 9. Optional capability router

Core work always uses native Git, targeted source reads/search, and project
checks. Setup presents a single consent table for optional LeanCTX, Serena,
Graphify, ast-grep, and Playwright; it detects usable path/version afresh and
installs only selected missing tools. Each installer adapter presents package,
scope, files/settings it will change, source/version, and rollback path, then
requires explicit consent; it verifies a direct command afterward. It neither
invokes vendor host installers nor registers MCP/hooks/skills automatically.
If adapter installation is declined, unsupported, or fails, native fallback is
used with no host mutation. A declined/missing/unhealthy tool is nonblocking
and uses the stated fallback. A receipt records tool,
version, worktree, revision, short result, and outcome metric only when used.

Initially route at most one analytical accelerator per task. Escalate to a
second only when the first leaves a named unanswered question.

| Capability | Narrow trigger and mode | Fallback |
| --- | --- | --- |
| LeanCTX | Large/verbose discovery, build/test output, or constrained recovery; output compression only | Targeted native reads, saved raw output, receipt |
| Serena | Supported-language semantic def/ref/caller/rename question in medium/high-risk unfamiliar scope | `rg`, source, language-native build/typecheck/test |
| Graphify | Refactor/migration or uncertain multi-module blast-radius/parallel-boundary question | Git diff, imports/search, affected project checks |
| ast-grep | Structural query/transformation across several candidate files where regex is unsafe | `rg`, manual narrow edits, compiler/tests |
| Playwright | UI acceptance requires real interaction, responsive/visual/accessibility evidence and project tooling is absent/inadequate | Existing project e2e/browser tooling; explicitly limited evidence if unavailable |

Graphify is base CLI code-only, per worktree/revision (`extract --code-only
--no-viz` plus bounded `affected`, `path`, or `explain`). No graph sharing,
reports/visualization, semantic/document extraction, backend/API key, MCP,
hook, memory, skill installer, or automatic rebuild is allowed. Serena MCP is
a separate explicit per-host user choice after local verification; it is never
a core requirement. Serena is read-only: its host integration exposes only an
allowlist of symbol/search/navigation operations and excludes edit, command,
memory, project-switch, handoff, and arbitrary-tool operations. Negative tests
must prove the excluded operations cannot be called. LeanCTX never supplies
shared memory, handoff, task, coordination, or authority state.

## 10. Instruction and visual-work routing

Use native host delegation. The only common packet instruction is a compact
safety/scope/receipt/review contract. `using-superpowers` routes an agent to
read only the task-relevant procedural skill: planning uses brainstorming and
writing-plans; isolated implementation uses worktrees/TDD/debugging as needed;
review uses requesting/receiving review and verification; completion uses
finishing-branch guidance. It is not a default bulk skill load.

For visual/design/UI work, the packet always loads Impeccable plus
`ui-ux-pro-max`; neither is optional nor substituted. `ui-styling` may be added
as a third, task-scoped capability only when it is explicitly requested by the
user or demonstrably applicable to the project's stack/task. It is never a
substitute for `ui-ux-pro-max` and is never broadly preloaded. The worker reads
the complete applicable skill material, records each selected digest/path in the
packet, and may reuse it only while that digest and task scope remain unchanged.
A changed digest, path, task scope, or design authority invalidates reuse.
Project design rules and explicit user direction take precedence over skill
guidance, which takes precedence over generic model taste. The worker inspects
the existing design system and implements within it. The reviewer obtains real
browser/screenshot evidence at representative small and large viewports when
visual acceptance is required; Playwright is selected only when that evidence
needs browser automation. An Impeccable detector or skill result is not browser
proof.
Required browser acceptance is declared in tracker criteria; otherwise visual
browser inspection is advisory and its absence is explicitly reported, never
silently treated as pass. vNext uses hook-free skill/instruction content only
and never runs vendor installers that add hooks.

## 11. Portability, privacy, and observability

All vNext coordination uses relative paths, UTF-8 JSON/Markdown, Git OIDs,
host-native process invocation, and ordinary file operations; it does not
depend on `/proc`, `flock`, `bash`, `$HOME`, POSIX signal semantics, symlinks,
or a Node runtime. The host adapter maps these actions to Codex and Claude
native delegation capabilities and reports an unsupported optional capability
instead of emulating it. Native Windows must be acceptance-tested, not inferred
from `.cmd` suffix handling.

No activation telemetry, per-prompt logs, model transcripts, API keys, or
private source are exported by core. Receipts retain only necessary project
facts. Optional external docs/services require separate user consent and may
not receive secrets/private code. Dashboard output is local and ignored. Tool
metrics are local aggregate task metrics (time, tokens if host exposes them,
repair count, result) and are used for routing evaluation, not surveillance.

## 12. Failure and resume semantics

After meaningful fact/decision and before voluntary pause/handoff, the owner
updates the receipt. On any resume the orchestrator reconciles tracker, Git
revision, worktree changes, candidate evidence, resource records, and observed
worker/server state. Unknown liveness/ownership is not assumed free within an
active harness. An incomplete/ambiguous operation becomes `interrupted` or
`blocked`; it is inspected before retry. No design can recover unrecorded
thoughts or guarantee an abrupt-exit write.

Invalid receipt/evidence, revision mismatch, dirty overlap, failed review,
failed gate, unavailable required project check, or ambiguous integration stops
that task at `FIX`/`blocked`; no completion is claimed. A missing optional
accelerator does not block. A failed server/browser check releases neither its
slot nor its evidence claim until observed cleanup or explicit user direction.

Every tool call and command has a declared timeout, retry budget, and attempt
identifier. Timeouts, process exit status, transport failure, and semantic
check result are separate fields; a zero exit cannot alone pass a requirement.
Retries are bounded and use backoff only for classified transient failures.
Full command output is retained at the evidence pointer, while the receipt/gate
uses bounded head/tail summaries and an exit/result field. Orphaned or unknown
server/browser/external resources occupy only their recorded slot, never all
project capacity; an unrelated resource remains usable when its declared
resource key does not collide.

CI, network, authentication, provider quota/429, and eventual-consistency
outcomes are explicit external-operation states. The first retry queries an
idempotency key/operation ID or provider status; it does not duplicate a
deployment, payment, migration, or remote mutation. Auth failure requires new
user authorization. Quota/429 waits only within the active foreground turn and
then blocks with retry-after evidence. Eventual consistency remains pending
until the bounded verification window expires, then becomes ambiguous/blocked
and requires `reconcile` before any retry.

## 13. Migration, cutover, rollback

vNext is built and tested in a clean room. Before cutover, an authorized
migration captures redacted hashes/backups of touched host/project settings,
lists active processes/worktrees/resources, snapshots tracker/Git state, and
classifies old `.agent-team` files as provenance. It does not import legacy
claims, leases, checkpoints, lanes, or ownership as vNext authority.

Cut over one project at a time after users stop old workers by convention.
Remove only demonstrably Agent-Team-owned legacy registrations; preserve
Project Kickoff entries under its policy and all unknown/custom third-party
entries. Install vNext separately, select one tracker, validate a fresh
Codex/Claude action with no Agent-Team hook callbacks, and run one end-to-end
task before general use. A conservative uninstaller deletes only exact,
manifest-owned unchanged entries; modified/unknown entries are reported. A
rollback restores exact backed-up configuration, never restarts stale runs or
deletes legacy worktrees/evidence.

The Codex `arg0` temp warning is a host-runtime maintenance issue, not a vNext
state path. vNext never touches `/home/.../.codex/tmp/arg0`; migration does not
use temp-directory cleanup as a cutover step.

## 14. Acceptance strategy

The implementation is accepted only after clean-room tests cover:

1. Fresh `TASKS.md` bootstrap, Beads opt-in, optional Kickoff handoff, and
   refusal to create missing artifacts without confirmation.
2. Retained/reused teams, 1–8 ordered tasks, path conflict serialization,
   queue refill, and parallel disjoint work.
3. Developer/reviewer FIX/CLEAN loops, exact-revision deterministic gate,
   serial integration, dashboard single refresh, idempotent deployment and
   ambiguous-provider recovery.
4. Interrupted task resume from tracker/Git/receipt; Codex↔Claude switch
   without lease/release/old-session continuation; unknown worker/resource
   handling.
5. Two server/two browser limits, same-revision reuse, cross-worktree preview
   separation, graceful known-process stop, and never-kill-unknown behavior.
6. Optional-tool decline/failure fallbacks; narrow Graphify/LeanCTX/Serena/
   ast-grep/Playwright triggers; no implicit MCP, hook, host-skill installer,
   memory, or analytics behavior.
7. Windows, macOS, and Linux host-adapter tests for Codex and Claude where
   supported, including JSON paths, worktrees, receipts, and dashboard viewing.
8. Migration backup/rollback/uninstaller tests proving mixed host settings,
   third-party hooks/MCPs, stale legacy state, and custom files are preserved.
9. Schema/precedence rejection tests for unknown versions, mismatched run/team/
   fingerprints, dirty worktrees, stale evidence, conflicting canonical/
   candidate/integration revisions, and invalid skill-digest reuse.
10. Pause/resume/cancel/inspect/reconcile tests, including pending external
    deployment-operation reconciliation without a lease, PID proof, or old
    harness return.
11. Browser ownership/capacity/cleanup tests, required-versus-advisory visual
    acceptance, server port/external-resource collision queueing, and dashboard
    atomic last-good/stale behavior.
12. Explicit installer-adapter consent, native fallback, Serena read-only
    allowlist, and negative tests for forbidden Serena operations and all
    implicit host mutation paths.
13. TASKS.md and Beads boundary tests at 999, 1,000, and 1,001 authoritative
    non-archived tasks; warning at 900; rejected over-cap creation/admission;
    recoverable archive preserving IDs/evidence; and resume/import behavior
    that retains in-flight, blocked, paused, and release-pending tasks.
14. Preflight tests for canonical Git identity, case/Unicode/reserved-name
    collisions, symlink/reparse escapes, detached/dirty/inaccessible worktrees,
    and inaccessible common Git directories. Argument-file tests prove that
    long commands never create files in an assigned product worktree; use only
    the bounded, manifest-owned per-run common-Git-dir transient subtree; use
    unique same-directory temp-write/flush/digest/atomic-rename handling; and
    preserve locked paths and last-good state after bounded Windows
    create/rename/cleanup sharing-lock retries. Also cover atomic-replace probe,
    network-filesystem single-writer fallback, and disk limits.
15. Configured/observed/effective capacity tests at zero, one, known N, and
    unknown capacity; reserved non-author reviewer context; duplicate same-host
    invocation; disposable-worker compaction/eviction; and stale duplicate
    message no-op semantics.
16. Bounded command/tool timeout/retry/exit/result/log tests; orphan resource
    per-slot behavior; CI/network/auth/429/eventual-consistency/idempotency
    reconciliation; and no background continuation outside an active host turn.
17. Size-cap tests for every canonical/receipt/handoff/log/evidence/screenshot/
    browser-crash trace/video limit, warning threshold, bounded summaries,
    recoverable archive/no auto-delete, and last-good write failure behavior.
18. Platform matrix tests on Windows, macOS, and Linux for both hosts, including
    paths, Unicode/case handling, reparse/symlink containment, host capacities,
    optional-tool unsupported fallback, and no POSIX/Node/executable-bit core
    dependency.
19. One-off runs with no tracker: immutable manifest, one/two disjoint team
    admission, serial fallback on overlap, review/gate/evidence, and automatic
    safe terminal cleanup without creating TASKS.md or Beads data.
20. Worktree manager and host delegation tests: main checkout rejects product
    authoring, worker packet containment/digests, Codex/Claude worker launch,
    distinct reviewer identity, worktree integration, and only manifest-owned
    clean merged worktree/branch/transient cleanup.
21. Decision artifact tests: append-only DECISIONS.md IDs, receipt references,
    derived BLOCKERS.md, explicit promotion to AGENT_TEAM_RULES.md, immutable
    WORKER-CONTRACT precedence, and bounded derived run handoff.
22. Scoped task/team/run/project pause, stop, cancel, checkpoint, resume, and
    blocker-propagation tests proving disjoint lanes continue while project
    critical shared blockers halt new admission.
23. Deployment executor tests: explicit authorization, common-Git-dir
    argument-file command isolation, idempotency key, Submit/Query/Verify,
    auth/429/eventual-consistency states, duplicate-prevention, live
    verification, and failed-batch hold.
24. Go distribution tests: checksum-verified static binary, installed immutable
    WORKER-CONTRACT, Git-only core operation, and no Node.js/Python runtime,
    hook, daemon, or host-settings mutation.
25. Public-contract tests: accept only the documented skill-action vocabulary
    and required run IDs/scopes; reject public `review`, `gate`, `integrate`,
    `reconcile`, and checkpoint bypasses; preserve only `FIX`/`CLEAN` reviewer
    verdicts; generate one bounded Markdown handoff from receipt/checkpoint
    facts without a second ledger; and prove install/update/rollback/uninstall
    verify checksums, use atomic manifest-owned files/backups, and never mutate
    hooks, MCP registrations, settings, plugins, credentials, or unrelated
    host skills.

## 15. Six phase-gated implementation plans

The umbrella design is delivered through exactly six separately accepted
implementation plans. A later phase cannot replace the acceptance evidence of
an earlier phase.

1. **Go core and authority.** Build the static Go binary, portable preflight,
   schemas/store, plan and one-off manifests, TASKS.md/Beads adapters, receipts,
   decisions/blockers/rules/handoff derivation, 1,000-task window, limits, and
   scoped actions. Gate: cross-platform native tests prove authority precedence,
   bounded writes, one-off two-team safety, pause/resume/cancel/checkpoint, and
   no Node/Python/hook dependency.
2. **Delegation, worktrees, and quality loop.** Build host adapters, immutable
   packets, reviewer-identity enforcement, Git worktree manager, admission,
   capacity reservation, FIX/CLEAN, clean-room gate, serial integration,
   event-driven foreground supervision, and safe automatic cleanup. Gate:
   Codex/Claude Linux/macOS/Windows adapters pass a real delegated developer
   and distinct reviewer journey without main product authoring.
3. **Optional capabilities, UI, browser, and dashboard observer.** Build consented
   optional capability adapters, concrete server/browser registry, mandatory
   Impeccable + UI-UX-Pro-Max visual packet routing, narrowly additional
   ui-styling where applicable, visual evidence, and atomic dashboard renderer.
   Gate: missing/declined tools retain core operation; required visual checks
   queue/block correctly; snapshot preserves last-good output.
4. **Authorized deployment and operational recovery.** Build the explicit
   idempotent deployment executor, provider query/verification, CI/auth/quota/
   eventual-consistency handling, resource/branch/backup/database cleanup
   boundaries, and dashboard refresh trigger contract. Gate: a fake provider
   proves submit/query/verify/retry/ambiguous hold with no duplicate mutation.
5. **Install, migration, cutover, and uninstaller.** Build checksum-verified
   Go distribution artifacts, immutable worker-contract installation, redacted
   v7 inventory, backup/restore, one-project cutover, conservative uninstaller,
   and rollback.
   Gate: mixed legacy/custom settings survive unchanged outside manifest-owned
   entries and no legacy run/worktree is revived.
6. **Release readiness.** Publish installer/worker-contract/setup/action/
   migration/dashboard/privacy guides, benchmark evidence, deterministic
   package/checksum/SBOM artifacts, Codex/Claude canaries and rollback,
   provider verification, and a non-force tagged publication. Gate: docs match
   tested Go behavior; benchmark, canary, rollback, provider, package, and
   revision-bound release evidence all verify before publication.

## 16. Deliberate exclusions

vNext contains **no hooks**: no SessionStart, PreToolUse, PostToolUse,
PostToolBatch, PreCompact, Interrupt, TaskCompleted, UserPromptExpansion, or
Project Kickoff guard/checkpoint hooks. It contains no legacy v7 runtime code,
hook manifests, policy engine, event budgets, owner recovery, lanes, lock
files, context-shrinker, auto-install catalog, automatic MCP configuration, or
vendor hook installer. Those artifacts are migration inputs only and are not
linked, imported, copied, or executed by the clean-room implementation.
All v7 documentation, tests, state, receipts, worktrees, hooks, and package
metadata are historical migration evidence only; none define vNext behavior.

## Design sources

- [Approved redesign decisions](../../audits/2026-09-18-redesign-decisions.md)
- [Current-state audit](../../audits/2026-09-18-current-state-audit.md)
- [State/hooks/Kickoff addendum](../../audits/2026-09-18-state-hooks-kickoff-addendum.md)
