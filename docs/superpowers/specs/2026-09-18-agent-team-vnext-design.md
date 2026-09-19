# Agent-Team vNext: clean-room architecture specification

**Status:** approved design baseline. This specifies a new implementation; it
does not authorize edits to, execution of, or dependency on the v7 runtime.

## 1. Purpose and boundaries

Agent-Team vNext continuously executes a project implementation plan with a
small host-neutral control plane. An orchestrator plans, admits bounded
parallel work, receives exact-revision review evidence, integrates serially,
and deploys authorized batches. Codex and Claude Code may be switched at any
safe point because durable truth is repository state, not a host session.

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

The tracker is the sole task authority. Git is the sole revision authority.
The receipt is a compact resumption and evidence index, never a second task
ledger or a duplicate transcript. `CONTEXT.md` and `MISTAKES.md`, when present,
are project knowledge inputs and not ownership or completion authorities.

### Components

| Component | Responsibility | Explicitly does not do |
| --- | --- | --- |
| Orchestrator | Plan/refine tasks, choose routing, admit teams, send packets, integrate and deploy | Write feature code or substitute its own review for CLEAN evidence |
| Tracker | Task IDs, dependencies, status, acceptance criteria, batch relation | Session/lease/process ownership |
| Team | A retained logical capacity with a serial queue of up to eight tasks | A nested orchestration system or permanent worker identity |
| Developer | Implement only assigned task and writable scope in its worktree | Self-approve or integrate |
| Reviewer | Inspect exact revision, execute applicable real checks, issue FIX/CLEAN | Author the reviewed change or invent missing tests |
| Gate | Deterministically validates receipt/evidence and Git preconditions | Guess semantic quality or call a model |
| Receipt | Minimal durable facts required to resume/reconcile one task | Full context, memory, locks, or telemetry |
| Resource registry | Records team-owned server/browser use and observed state | Kills unknown processes or arbitrates across projects |

### Authority and precedence

Every durable record has `schema`, `project`, `runId`, `writtenAt`, and a
monotonic `revision`. A reader rejects an unknown schema and treats a malformed
or conflicting record as blocked, never as an invitation to guess. Authority is
ordered as follows:

1. Git refs and the actual worktree index/dirty state establish source truth.
2. The selected tracker establishes task identity, dependency, acceptance
   criteria, and task lifecycle intent.
3. The run record establishes the admitted set, team assignment, refill batch,
   and authorized deployment batch.
4. The retained-team receipt establishes the latest observed attempt,
   candidate/canonical/integration revisions, evidence pointers, resource use,
   and next action for that team's queue.
5. Evidence establishes only the command/reviewer result it records; it cannot
   override a mismatched tracker, Git revision, or receipt.

On disagreement, Git/worktree dirtiness wins over every claimed revision;
tracker task identity wins over receipt task text; and a newer receipt revision
wins only after its `runId`, team, queue fingerprint, and Git references pass
validation. `CONTEXT.md`, `MISTAKES.md`, optional-tool output, dashboard, and
host session state are advisory inputs with no authority.

## 3. Lifecycle and public actions

### Setup and bootstrap

`setup` is an interactive, idempotent project bootstrap.

1. Detect the project root, Git availability, existing plan, tracker, and
   optional Project Kickoff handoff.
2. If required artifacts are missing, show the exact files to create and ask
   before creating them. Required baseline artifacts are the selected single
   tracker (root `TASKS.md` unless Beads is selected), `.agent-team/config.json`,
   and `.agent-team/receipts/`; `CONTEXT.md` and `MISTAKES.md` are offered but
   not required.
3. Default to `TASKS.md`. Offer Beads only as an explicit tracker selection;
   never maintain both as live authorities.
4. Run the model-role wizard and optional capability prompt described below.
5. Discover the project validation commands and record only approved commands
   in config. No command is inferred as a deployment command.

The public vocabulary is deliberately small: `setup`, `start`, `status`,
`assign`, `pause`, `resume`, `cancel`, `review`, `gate`, `integrate`,
`deploy`, `inspect`, and `reconcile`. Hosts may spell these commands
idiomatically, but must preserve these meanings and record names in receipts.
There are no legacy aliases, lifecycle hooks, background wakeups, automatic
prompt expansion, or hidden tool callbacks.

Task transition graph: `ready → implementing → reviewing → (fix →
implementing | clean → gated → integrated)`. `pause` moves any nonterminal
task to `paused` after writing the receipt; `resume` reconciles it then returns
it to its prior actionable state. `cancel` is explicit, records rationale and
leaves source untouched unless an already-authorized revert is separately run.
Any failed/unknown external operation moves its task to `blocked` or
`interrupted`, never back to ready automatically. `inspect` reads bounded
tracker/Git/receipt/resource facts only; `reconcile` compares those facts and
records a deterministic repair plan without dispatching, integrating, stopping
processes, or deploying.

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

Arguments and long commands are written as bounded argument files under the
assigned worktree rather than interpolated shell strings. The adapter probes
same-directory atomic replace before writing canonical records. On a filesystem
that cannot provide it (including a network filesystem), vNext uses a declared
single-writer foreground action and read-after-write checksum, serializing all
record writes; it does not emulate a lease. Windows transient sharing failures
use a small bounded retry with backoff and then preserve last-good state as
blocked. Detached, dirty, inaccessible, low-disk, or uncontained worktrees are
reported precisely and are not admitted until their applicable precondition is
resolved.

## 4. Configuration and durable records

All schemas are JSON or Markdown and use repository-relative paths plus Git
object IDs. They must be valid on Windows, macOS, and Linux.

### `.agent-team/config.json`

```json
{
  "schema": 1,
  "tracker": {"kind": "tasks-md", "path": "TASKS.md"},
  "models": {"orchestrator": "user-selected", "developer": "user-selected", "reviewer": "user-selected"},
  "limits": {"parallelTeams": 4, "teamQueueMax": 8, "projectTaskCapacity": 1000, "devServers": 2, "browserSessions": 2, "handoffWarnBytes": 196608, "handoffHardBytes": 262144, "commandLogBytes": 1048576, "attemptLogBytes": 10485760},
  "storage": {"trackerWarnPercent": 75, "trackerHardBytes": 2097152, "canonicalHardBytes": 16777216, "screenshotsPerAttempt": 20, "screenshotHardBytes": 10485760, "evidencePerAttemptBytes": 104857600, "activeEvidenceWarnBytes": 524288000},
  "checks": {"format": [], "type": [], "test": [], "build": [], "visual": []},
  "deployment": {"enabled": false, "batchSize": 1},
  "capabilities": {"leanctx": "optional", "serena": "optional", "graphify": "optional", "astGrep": "optional", "playwright": "optional"}
}
```

`parallelTeams` is chosen in setup, capped by the host-native delegation
capacity, and may be lowered per run. Configuration stores preferences and
validated commands, never API keys, session IDs, process IDs, raw logs, or
private prompts.

Limits are configurable downward only except where an explicit user change is
within these safe hard ceilings: tracker 2 MiB, all canonical vNext records
16 MiB, handoff 256 KiB (warn at 192 KiB), command log 1 MiB, attempt logs
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
smallest dependency-ready page needed to fill effective capacity, then
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

Host adapters translate only labels, availability, and native delegation calls:

| Host / OS | Supported vNext core | Model label treatment |
| --- | --- | --- |
| Codex on Windows, macOS, Linux | Yes after host-native delegation and Git acceptance tests | Resolve user-selected Codex label to a verified available model; record requested and resolved labels |
| Claude Code on Windows, macOS, Linux | Yes after host-native delegation and Git acceptance tests | Resolve user-selected Claude label likewise; never represent a Codex model as Claude-native |
| Any untested host/OS pair | No execution claim | `setup` reports unsupported; tracker/receipt remain readable |

The adapter has no persistent compatibility cache and does not alter host
configuration to make a label appear available.

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

Deployment is disabled unless setup records an authorized target and command.
Eligible completed tasks form ordered batches of configured size; the final
nonempty remainder is valid. Each deployment receipt contains task IDs,
canonical revision/artifact, target, idempotency key, provider operation ID,
and live verification result. Replaying an unknown operation first queries the
provider evidence using that key/operation ID; it never blindly deploys again.
A failed or ambiguous batch halts further deployment, preserves evidence, and
requires explicit repair/recovery evidence before continuation. Coding on
unaffected work may continue.

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

For visual work, the packet loads both Impeccable and `ui-ux-pro-max`; no
implicit `ui-styling` alternative is selected. The worker reads the complete
applicable skill material, records its digest/path in the packet, and may reuse
it only while that digest and task scope remain unchanged. A changed digest,
path, task scope, or design authority invalidates reuse. Project design rules
and explicit user direction take precedence over skill guidance, which takes
precedence over generic model taste. The worker inspects the existing design
system and implements within it. The reviewer obtains real browser/screenshot
evidence at representative small and large viewports when visual acceptance is
required; Playwright is selected only when that evidence needs browser
automation. An Impeccable detector or skill result is not browser proof.
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
    argument-file handling, atomic-replace probe, network-filesystem
    single-writer fallback, disk limits, and transient Windows sharing retries.
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

## 15. Phase-gated implementation plans

The umbrella design is delivered through exactly four separately accepted
implementation plans. A later phase cannot replace the acceptance evidence of
an earlier phase.

1. **Native core.** Implement schemas, tracker adapters, receipts, public
   actions/transitions, teams/refill transaction, admission, reviewer, clean-
   room gate, integration, deployment idempotency, resume, host adapters, and
   abstract resource keys/capacity accounting only. Gate: native-core portions
   of sections 3–7 and acceptance items 1–4, 9–10, 13–16 pass with no optional
   tool, concrete server/browser control, hook, or host mutation.
2. **Optional tools, UI, and browser.** Implement consented adapters and
   narrow capability router, concrete resource registry, browser/server controls,
   Impeccable + UI-UX-Pro-Max packet routing, and dashboard renderer. Gate:
   acceptance items 5–7, 11–12, 17–18 pass; missing/declined tools retain
   native-core operation.
3. **Migration, cutover, and uninstaller.** Implement redacted inventory,
   backup/restore, one-project cutover, conservative uninstaller, and rollback.
   Gate: acceptance item 8 passes on mixed legacy/custom settings without
   removing non-owned files or reviving a legacy run.
4. **Documentation and benchmarks.** Publish host setup/action vocabulary,
   migration guide, dashboard guide, privacy statement, and 20–30-diff routing
   benchmark report. Gate: docs match tested behavior and benchmark data is
   sufficient to retain or adjust provisional model/capability defaults.

## 16. Deliberate exclusions

vNext contains **no hooks**: no SessionStart, PreToolUse, PostToolUse,
PostToolBatch, PreCompact, Interrupt, TaskCompleted, UserPromptExpansion, or
Project Kickoff guard/checkpoint hooks. It contains no legacy v7 runtime code,
hook manifests, policy engine, event budgets, owner recovery, lanes, lock
files, context-shrinker, auto-install catalog, automatic MCP configuration, or
vendor hook installer. Those artifacts are migration inputs only and are not
linked, imported, copied, or executed by the clean-room implementation.

## Design sources

- [Approved redesign decisions](../../audits/2026-09-18-redesign-decisions.md)
- [Current-state audit](../../audits/2026-09-18-current-state-audit.md)
- [State/hooks/Kickoff addendum](../../audits/2026-09-18-state-hooks-kickoff-addendum.md)
