# Agent-Team vNext Phase 1 Go Native Core Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the hook-free, cross-platform Go Phase 1 control plane for bounded project state, task admission, recovery records, and deferred-action validation.

**Architecture:** A clean-room Go module under `vnext` exposes `agent-teamctl`. Standard-library packages own schemas, canonical Git identity, bounded atomic persistence, TASKS/Beads adapters, immutable plan/one-off runs, revision-checked admission, receipts/knowledge, scoped workflow state, and abstract capacity. Phase 1 declares exact later-phase ports but never spawns workers or mutates product code.

**Tech Stack:** Go standard library, UTF-8 JSON/Markdown, `os/exec` argument arrays, Git, `go test`, `go vet`; module `github.com/thebpandey/agent-team/vnext`.

**Spec:** `docs/superpowers/specs/2026-09-18-agent-team-vnext-design.md`

## Global Constraints

- Clean room: do not import, copy, invoke, register, or modify `hooks/`, `references/`, v7 manifests/installers/state, or hook runtime code.
- No hooks, Node.js, npm, Python, third-party Go packages, daemons, MCP registration, settings/plugin mutation, shell-profile mutation, environment mutation, or credential mutation.
- Use `TASKS.md` by default or explicitly selected Beads; one-off runs use an immutable manifest and no tracker. Git/worktree identity is authoritative.
- Schema version is `1`; malformed/unknown/conflicting records block and preserve last-good state.
- Support Windows, macOS, and Linux without `/proc`, `flock`, bash, HOME, POSIX signals, symlink assumptions, executable-bit assumptions, or case-sensitive behavior.
- Capacity is 1,000 non-archived tasks: warn at 900 and reject creation/admission above 1,000. Admission batches contain 1–8 tasks. One-off runs allow two teams only when paths/resources are disjoint.
- Limits are tracker 2 MiB, canonical records 16 MiB, handoff 256 KiB with warning at 192 KiB, argument file 1 MiB, argument staging 10 MiB/attempt, command log 1 MiB, attempt log 10 MiB, and evidence 100 MiB/attempt. No automatic deletion.
- Phase 1 defines abstract resource/capacity records and later-phase ports only. It does not spawn hosts, create worktrees, review, gate, integrate, deploy, run browsers/servers, render dashboards, install, migrate, or install optional tools.
- Execution is foreground-only during a user-initiated host turn. There are no daemons, leases, cross-harness locks, transfer protocols, PID gates, or telemetry streams.

## File Map

| Path | Responsibility |
| --- | --- |
| `vnext/go.mod`, `vnext/cmd/agent-teamctl/main.go` | Go module and executable entrypoint. |
| `vnext/internal/core/*.go` | IDs, states, scopes, errors, config, limits, task/run/resource records, packets. |
| `vnext/internal/cli/*.go` | Canonical `cli.Run` parser, action validation, and bounded output. |
| `vnext/internal/testkit/*.go`, `vnext/testdata/**` | Deterministic Git, filesystem, Beads, record, and assertion fixtures. |
| `vnext/internal/project/*.go` | Canonical project/Git identity, portable paths, setup and Kickoff validation. |
| `vnext/internal/store/*.go` | Bounded atomic JSON/Markdown persistence and last-good behavior. |
| `vnext/internal/tracker/*.go` | Tracker snapshot/revision interface, TASKS.md, Beads, bounded command runner. |
| `vnext/internal/run/*.go` | Plan and immutable one-off manifests. |
| `vnext/internal/admission/*.go` | Same-revision atomic admission, refill history, idempotency, and capacity. |
| `vnext/internal/knowledge/*.go` | Receipts, evidence, decisions, blockers, rules, packets, and handoff projection. |
| `vnext/internal/workflow/*.go`, `vnext/internal/resources/*.go` | Typed scope/events, pause/stop/cancel/resume/checkpoint, and abstract reservations. |
| `vnext/internal/contracts/*.go` | Complete later-phase port interfaces and value types; no operational implementations. |
| `.github/workflows/vnext-native.yml` | Native Windows/macOS/Linux test and cross-build acceptance. |

### Task 1: Go module, shared domain, CLI envelope, and testkit

**Files:** Create `vnext/go.mod`, `vnext/cmd/agent-teamctl/main.go`, `vnext/internal/core/{errors.go,types.go,config.go}`, `vnext/internal/cli/{app.go,parser.go,app_test.go}`, and `vnext/internal/testkit/{fs.go,git.go,assert.go}`.

**Interfaces produced:**

**Package:** `vnext/internal/core` (`package core`).

```go
type TaskID string
type RunID string
type TeamID string
type TaskState string

const (
    Ready TaskState = "ready"
    Idle TaskState = "idle"
    Working TaskState = "working"
    Implementing TaskState = "implementing"
    Reviewing TaskState = "reviewing"
    Fix TaskState = "fix"
    Clean TaskState = "clean"
    Gated TaskState = "gated"
    Integrated TaskState = "integrated"
    Paused TaskState = "paused"
    Blocked TaskState = "blocked"
    Interrupted TaskState = "interrupted"
    Cancelled TaskState = "cancelled"
    Archived TaskState = "archived"
)

type ScopeKind string
const (
    ScopeProject ScopeKind = "project"
    ScopeRun ScopeKind = "run"
    ScopeTeam ScopeKind = "team"
    ScopeTask ScopeKind = "task"
)
type Scope struct { Kind ScopeKind; ID string }
type Check struct { Name string; Command []string }
type SkillRef struct { Name,Path,Digest,Purpose string; Required bool }
type AcceleratorStatus string
const ( AcceleratorAvailable AcceleratorStatus = "available"; AcceleratorUnavailable AcceleratorStatus = "unavailable"; AcceleratorFailed AcceleratorStatus = "failed" )
type AcceleratorRef struct { Name,Path,Digest string; Status AcceleratorStatus; Reason string }
type Task struct {
    RecordEnvelope; ID TaskID; Objective string; State TaskState
    Dependencies []TaskID; Criteria []string; Checks []Check
    WritablePaths []string; Resources []string; EvidencePointers []string
    Archived bool
}
type TrackerPage struct { TrackerRevision uint64; TotalNonArchived int; Cursor string; Tasks []Task }
type ResourceSnapshot struct { Servers, Browsers, External []string }
type RunRecord struct { RecordEnvelope; ID RunID; SpecRevision string; TrackerKind string; TrackerRevision uint64; CanonicalRevision string; State TaskState }
type RecordEnvelope struct { Schema int; Project string; RunID RunID; WrittenAt string; Revision uint64 }
type KickoffHandoff struct { ApprovedPlanRevision string; Branch string; TrackerKind, TrackerRef string; TrackerRevision uint64; TaskIDs []TaskID; Acceptance []string; Checks []Check; WritablePaths, Resources, Capabilities []string }
type AssignmentPacket struct { RecordEnvelope; SpecRevision string; Task TaskID; Team TeamID; QueueFingerprint string; Owner,Worktree,Base string; Criteria []string; Scope []string; Checks []Check; Capabilities []string; Skills []SkillRef; Resources ResourceSnapshot; Accelerators []AcceleratorRef; Review,Gate,NextAction string; ReceiptPath string }
type Dependencies struct { ProjectRoot string; Stdout, Stderr io.Writer; Confirmations map[string]bool }
type ErrorCode string
type ErrorEnvelope struct { Code ErrorCode; Message string; Details map[string]string }
type Config struct { Schema int; Runtime RuntimeConfig; Tracker TrackerConfig; Limits Limits; Storage StorageLimits }
type RuntimeConfig struct { Kind string; WorkerContractDigest string }
type TrackerConfig struct { Kind string; Path string }
type Limits struct { ParallelTeams, TeamQueueMax, ProjectTaskCapacity, DevServers, BrowserSessions int }
type StorageLimits struct { TrackerBytes, CanonicalBytes, HandoffWarnBytes, HandoffHardBytes, ArgumentBytes, StagingBytes int64; TrackerWarnPercent int }

var ErrPath, ErrGit, ErrLimit, ErrCapacity, ErrBatch, ErrRevision, ErrTransition, ErrSettings, ErrPhase error
func DefaultConfig() Config
func ApplySettings(Config, map[string]string) (Config, error)
```

The declarations through `StorageLimits` belong to `internal/core`; every durable record in Tasks 5–8 embeds `RecordEnvelope` and uses its project/run/schema/revision fields. The only CLI entrypoint is `internal/cli.Run(context.Context, []string, core.Dependencies) int`. `Dependencies` contains project root, `io.Writer` output/error writers, and confirmation decisions. `internal/testkit` defines `GitRepo(*testing.T) string`, `Fixture(*testing.T,string) string`, `SnapshotTree(*testing.T,string) map[string]string`, and `RequireNoWrites(*testing.T,string,string)`.

- [ ] **Step 1: Write failing tests:**
```go
func TestCoreAndCLIContracts(t *testing.T) {
    var _ core.TaskID = "TASK-1"
    c := core.DefaultConfig()
    if c.Storage.TrackerWarnPercent != 90 || c.Limits.ProjectTaskCapacity != 1000 { t.Fatal(c) }
    if _, err := core.ApplySettings(c, map[string]string{"runtime.kind":"node"}); !errors.Is(err, core.ErrSettings) { t.Fatal(err) }
    var out bytes.Buffer
    deps := core.Dependencies{ProjectRoot:t.TempDir(), Stdout:&out, Stderr:io.Discard}
    if cli.Run(context.Background(), []string{"version", "--json"}, deps) != 0 || !bytes.Contains(out.Bytes(), []byte(`"schema":1`)) { t.Fatal(out.String()) }
    first := core.RecordEnvelope{Schema:1, Project:"p", RunID:"R-1", WrittenAt:"2026-09-19T00:00:00Z", Revision:1}
    encoded, _ := json.Marshal(first); var round core.RecordEnvelope; if json.Unmarshal(encoded, &round) != nil || round != first { t.Fatal(round) }
}
```
- [ ] **Step 2: Run:** `cd vnext && go test ./internal/core ./internal/cli`; **Expected:** FAIL because the packages do not exist.
- [ ] **Step 3: Implement** the exact types, concrete sentinel errors, defaults, `Dependencies`, parser, and `cli.Run`; serialize `RecordEnvelope` on every durable record, reject schema/project/run mismatches, require strictly increasing revisions for new writes, and keep output bounded and settings restricted to project config.
- [ ] **Step 4: Run:** `cd vnext && gofmt -w cmd/agent-teamctl/main.go internal/core/errors.go internal/core/types.go internal/core/config.go internal/cli/app.go internal/cli/parser.go internal/cli/app_test.go internal/testkit/fs.go internal/testkit/git.go internal/testkit/assert.go && go test ./internal/core ./internal/cli ./internal/testkit`; **Expected:** PASS.
- [ ] **Step 5: Commit:** `git add vnext && git commit -m "feat(vnext): add Go domain and canonical CLI"`.

### Task 2: Cross-platform project paths, Git identity, and setup/Kickoff confirmation

**Files:** Create `vnext/internal/project/{project.go,path.go,git.go,setup.go,project_test.go}`.

**Interfaces produced:**

**Package:** `vnext/internal/project` (`package project`).

```go
type Project struct { Root, TopLevel, CommonDir, Head string; Dirty, Detached bool; Readable, Writable bool; FreeBytes int64 }
type ArtifactMode string
const ( ExistingArtifact ArtifactMode = "existing"; GeneratedArtifact ArtifactMode = "generated" )
type RunMode string
const ( PlanMode RunMode = "plan"; OneOffMode RunMode = "one-off" )
type Confirmation string
const ( Approved Confirmation = "approved"; Refused Confirmation = "refused" )
type ArtifactDecision struct { Path string; Mode ArtifactMode; Confirmation Confirmation }
type KickoffDecision struct { Path, Digest string; Confirmation Confirmation }
type SetupInput struct { Root string; Mode RunMode; Artifacts []ArtifactDecision; Kickoff *KickoffDecision }
type SetupResult struct { Project Project; Config core.Config; ArtifactDigests map[string]string; Handoff core.KickoffHandoff; ConfigRevision uint64; ReceiptPath string }
type SetupService interface { Initialize(context.Context, SetupInput) (SetupResult, error); Validate(context.Context, SetupInput) (SetupResult, error) }
func NewSetupService(*store.Store) SetupService
func Contain(root, candidate string) (string, error)
func ValidateSegment(string) error
func Discover(context.Context, string) (Project, error)
func ValidateSetup(context.Context, SetupInput) (SetupResult, error)
```

- [ ] **Step 1: Write failing tests:**
```go
func TestProjectAndSetupDecisions(t *testing.T) {
    root := testkit.GitRepo(t)
    if _, err := project.Contain(root, filepath.Join(root, "..", "outside")); !errors.Is(err, core.ErrPath) { t.Fatal(err) }
    if err := project.ValidateSegment("CON"); !errors.Is(err, core.ErrPath) { t.Fatal(err) }
    _, err := project.ValidateSetup(context.Background(), project.SetupInput{Root:root, Mode:project.PlanMode, Artifacts:[]project.ArtifactDecision{{Path:"TASKS.md", Mode:project.ExistingArtifact, Confirmation:project.Refused}}})
    if !errors.Is(err, core.ErrSettings) { t.Fatal(err) }
    approved := project.SetupInput{Root:root, Mode:project.PlanMode, Artifacts:[]project.ArtifactDecision{{Path:"TASKS.md", Mode:project.ExistingArtifact, Confirmation:project.Approved}}, Kickoff:&project.KickoffDecision{Path:"docs/kickoff.md", Digest:"sha256:x", Confirmation:project.Approved}}
    if _, err := project.ValidateSetup(context.Background(), approved); err != nil { t.Fatal("approved setup", err) }
    approved.Kickoff.Confirmation = project.Refused
    if _, err := project.ValidateSetup(context.Background(), approved); !errors.Is(err, core.ErrSettings) { t.Fatal("refused kickoff", err) }
    svc := project.NewSetupService(store.New(root, core.StorageLimits{CanonicalBytes:16<<20})); if _, err := svc.Initialize(context.Background(), approved); err != nil { t.Fatal(err) }; if _, err := svc.Initialize(context.Background(), approved); err != nil { t.Fatal("idempotent initialize", err) }
}
```
- [ ] **Step 2: Run:** `cd vnext && go test ./internal/project`; **Expected:** FAIL because project/setup symbols do not exist.
- [ ] **Step 3: Implement** canonical path/Git discovery, explicit typed confirmation decisions without prompting, artifact digest validation, and one-time Kickoff fact import without hooks or second ledgers. `SetupService.Initialize` atomically bootstraps `.agent-team/config.json` and the initial receipt using `RecordEnvelope`; `Validate` reads and compares those records without writing. Populate `SetupResult.Handoff` with approved plan revision, branch, tracker kind/ref/revision, task IDs, acceptance, checks, writable paths, resources, and capabilities. Refusal writes neither config nor receipt; repeated Initialize with the same digest/revision is idempotent and conflicting initialization returns `core.ErrRevision`.
- [ ] **Step 4: Run:** `cd vnext && gofmt -w internal/project/project.go internal/project/path.go internal/project/git.go internal/project/setup.go internal/project/project_test.go && go test ./internal/project -count=1`; **Expected:** PASS.
- [ ] **Step 5: Commit:** `git add vnext/internal/project && git commit -m "feat(vnext): add portable identity and setup validation"`.

### Task 3: Bounded atomic JSON/Markdown store

**Files:** Create `vnext/internal/store/{store.go,replace_unix.go,replace_windows.go,replace_other.go,store_test.go}`.

**Interfaces produced:**

**Package:** `vnext/internal/store` (`package store`).

```go
type Store struct { Root string; Limits core.StorageLimits }
type AtomicResult struct { Bytes int64; SHA256 string }
func New(string, core.StorageLimits) *Store
func (*Store) ReadJSON(relative string, maxBytes int64, destination any) error
func (*Store) WriteJSON(relative string, value any, maxBytes int64) (AtomicResult, error)
func (*Store) WriteMarkdown(relative string, value []byte, maxBytes int64) (AtomicResult, error)
```

- [ ] **Step 1: Write failing tests:**
```go
func TestStoreBoundsAndLastGood(t *testing.T) {
    s := store.New(t.TempDir(), core.StorageLimits{CanonicalBytes: 32})
    if _, err := s.WriteJSON("record.json", strings.Repeat("x", 33), 32); !errors.Is(err, core.ErrLimit) { t.Fatal(err) }
    first, err := s.WriteJSON("record.json", map[string]string{"state":"good"}, 32)
    if err != nil || first.SHA256 == "" { t.Fatal(err, first) }
    if _, err := s.WriteJSON("record.json", strings.Repeat("y", 33), 32); !errors.Is(err, core.ErrLimit) { t.Fatal(err) }
    var got map[string]string
    if err := s.ReadJSON("record.json", 32, &got); err != nil || got["state"] != "good" { t.Fatal(err, got) }
    if _, err := s.WriteMarkdown("utf8.md", []byte("✓"), 8); err != nil { t.Fatal(err) }
}
```
The same test package adds explicit platform cases: probe same-directory replacement before the first canonical write; hold a destination open on Windows and inject a sharing/rename failure; verify OS-specific destination replacement after the source temp file is flushed, closed, and hashed; and perform two serialized read-after-write checksum comparisons on a network-backed temporary root. Each case must preserve the last-good destination, retain a locked transient path, and return a typed error rather than delete or truncate either file.
- [ ] **Step 2: Run:** `cd vnext && go test ./internal/store`; **Expected:** FAIL because storage is absent.
- [ ] **Step 3: Implement** bounded JSON/Markdown reads and writes with restrictive temporary files where supported. Probe whether same-directory atomic replacement is available before canonical writes. Flush, close, size-check, and hash the temp file; use Unix rename semantics in `replace_unix.go`, Windows destination replacement after all handles close in `replace_windows.go`, and a bounded sharing-violation retry with backoff on both create/replace/cleanup. If the probe reports a network or otherwise non-atomic filesystem, serialize foreground writes and verify a read-after-write checksum instead of claiming atomicity. On exhaustion preserve the locked transient and last-good destination, return a typed error, and never delete unknown files.
- [ ] **Step 4: Run:** `cd vnext && gofmt -w internal/store/store.go internal/store/replace_unix.go internal/store/replace_windows.go internal/store/replace_other.go internal/store/store_test.go && go test ./internal/store -count=1`; **Expected:** PASS on native platform jobs.
- [ ] **Step 5: Commit:** `git add vnext/internal/store && git commit -m "feat(vnext): add bounded atomic store"`.

### Task 4: Tracker snapshot/revision interface, TASKS.md, Beads, and bounded command runner

**Files:** Create `vnext/internal/tracker/{tracker.go,tasksmd.go,beads.go,command.go,tracker_test.go}` and `vnext/testdata/tasks/{ready.md,malformed.md}`.

**Interfaces produced:**

**Package:** `vnext/internal/tracker` (`package tracker`).

```go
type CommandResult struct { Stdout, Stderr []byte; Exit int; TimedOut bool; Transport error }
type CommandRunner interface { Run(context.Context, string, ...string) CommandResult }
type FakeRunner struct { Result CommandResult }
func NewFakeRunner(CommandResult) CommandRunner
type Tracker interface {
    Page(ctx context.Context, cursor string, limit int) (core.TrackerPage, error)
    Get(ctx context.Context, id core.TaskID, expectedTrackerRevision uint64) (core.Task, error)
    Refresh(ctx context.Context, expectedTrackerRevision uint64) (core.TrackerPage, error)
    Create(ctx context.Context, task core.Task, expectedTrackerRevision uint64) (core.Task, error)
    Archive(ctx context.Context, id core.TaskID, reason string, expectedTrackerRevision uint64) error
}
func NewTasksMD(string, *store.Store) Tracker
func NewBeads(CommandRunner) Tracker
```

- [ ] **Step 1: Write failing table-driven tests:**
```go
func TestTrackerFailuresAndSnapshot(t *testing.T) {
    for _, result := range []tracker.CommandResult{{TimedOut:true}, {Exit:7}, {Transport:errors.New("missing")}, {Stdout:[]byte("{")}} {
        tr := tracker.NewBeads(tracker.NewFakeRunner(result))
        if _, err := tr.Page(context.Background(), "", 8); err == nil { t.Fatalf("result %#v unexpectedly succeeded", result) }
    }
    tr := tracker.NewTasksMD(testkit.Fixture(t, "tasks/ready.md"), store.New(t.TempDir(), core.StorageLimits{TrackerBytes:2<<20}))
    page, err := tr.Page(context.Background(), "", 8)
    if err != nil || page.TrackerRevision == 0 || page.TotalNonArchived < 900 || page.Cursor == "" { t.Fatal(err, page) }
    task, err := tr.Get(context.Background(), page.Tasks[0].ID, page.TrackerRevision)
    if err != nil || len(task.Dependencies) == 0 || len(task.Criteria) == 0 || len(task.Checks) == 0 || len(task.WritablePaths) == 0 || len(task.Resources) == 0 || len(task.EvidencePointers) == 0 { t.Fatal(err, task) }
}
```
The table also invokes the resolver with each executable name (`bd`, `bd.exe`, `bd.cmd`) and fixtures at 999/1000/1001 non-archived tasks; expected results are deterministic pages, a warning at 900, and typed capacity errors above 1,000.
- [ ] **Step 2: Run:** `cd vnext && go test ./internal/tracker`; **Expected:** FAIL because adapters and runner are absent.
- [ ] **Step 3: Implement** bounded stream capture, structured command results, tracker snapshots carrying `TrackerRevision`, expected-revision checks on `Get/Refresh/Create/Archive`, and sole-authority TASKS/Beads selection.
- [ ] **Step 4: Run:** `cd vnext && gofmt -w internal/tracker/tracker.go internal/tracker/tasksmd.go internal/tracker/beads.go internal/tracker/command.go internal/tracker/tracker_test.go && go test ./internal/tracker -count=1`; **Expected:** PASS.
- [ ] **Step 5: Commit:** `git add vnext/internal/tracker vnext/testdata/tasks && git commit -m "feat(vnext): add revisioned tracker adapters"`.

### Task 5: Plan and immutable one-off manifests

**Files:** Create `vnext/internal/run/{run.go,manifest.go,run_test.go}`.

**Interfaces produced:**

**Package:** `vnext/internal/run` (`package run`).

```go
type OneOffKind string
const ( Feature OneOffKind = "feature"; Audit OneOffKind = "audit"; Review OneOffKind = "review" )
type TeamRecord struct { core.RecordEnvelope; ID core.TeamID; Queue []core.TaskID; QueueFingerprint string; State core.TaskState; Paths,Resources []string }
type Run struct { core.RecordEnvelope; SpecRevision string; ID core.RunID; Root,TrackerKind,Objective string; Tasks []core.Task; Teams []TeamRecord; TrackerRevision uint64; ManifestDigest string; State core.TaskState }
type AdmissionBatch struct { core.RecordEnvelope; BatchID,Fingerprint string; Tasks []core.TaskID; Team core.TeamID; Sequence uint64; TrackerRevision uint64; Paths,Resources []string }
type RunRepository interface { Initialize(context.Context, Run) (Run, error); Read(context.Context, core.RunID) (Run, error); CompareAndSwap(context.Context, core.RunID, uint64, Run) (Run, error) }
type TeamRepository interface { Initialize(context.Context, TeamRecord) (TeamRecord, error); Read(context.Context, core.TeamID) (TeamRecord, error); CompareAndSwap(context.Context, core.TeamID, uint64, TeamRecord) (TeamRecord, error) }
type Repositories struct { Runs RunRepository; Teams TeamRepository }
func NewRepositories(*store.Store) Repositories
func CreatePlan(context.Context, string, tracker.Tracker) (Run, error)
func CreateOneOff(context.Context, string, OneOffKind, string, []core.Task) (Run, error)
func ValidateConflict(AdmissionBatch, AdmissionBatch) bool
```

- [ ] **Step 1: Write failing table-driven tests:**
```go
func TestRunKindsAndConflicts(t *testing.T) {
    tr := tracker.NewTasksMD(testkit.Fixture(t, "tasks/ready.md"), store.New(t.TempDir(), core.StorageLimits{TrackerBytes:2<<20}))
    plan, err := run.CreatePlan(context.Background(), t.TempDir(), tr)
    if err != nil || plan.TrackerKind == "" || plan.ManifestDigest == "" { t.Fatal(err, plan) }
    one, err := run.CreateOneOff(context.Background(), t.TempDir(), run.Audit, "inspect config", []core.Task{{ID:"AUDIT-1", Objective:"inspect config", State:core.Ready}})
    if err != nil || one.TrackerKind != "none" || one.Tasks[0].WritablePaths != nil { t.Fatal(err, one) }
    a := run.AdmissionBatch{Tasks:[]core.TaskID{"T1"}, Paths:[]string{"src/a"}, Resources:[]string{"server:1"}}
    b := run.AdmissionBatch{Tasks:[]core.TaskID{"T2"}, Paths:[]string{"src/a"}, Resources:[]string{"server:1"}}
    if !run.ValidateConflict(a, b) { t.Fatal("overlap was admitted") }
}
```
```go
func TestRunRepositoryCAS(t *testing.T) { repo := run.NewRepositories(store.New(t.TempDir(), core.StorageLimits{CanonicalBytes:16<<20})); saved, _ := repo.Runs.Initialize(context.Background(), run.Run{RecordEnvelope:core.RecordEnvelope{Schema:1, Project:"p", RunID:"R-1", WrittenAt:"2026-09-19T00:00:00Z", Revision:1}, ID:"R-1"}); if _, err := repo.Runs.CompareAndSwap(context.Background(), saved.ID, 0, saved); !errors.Is(err, core.ErrRevision) { t.Fatal(err) }; if _, err := repo.Runs.CompareAndSwap(context.Background(), saved.ID, saved.Revision, saved); err != nil { t.Fatal(err) } }
```
The fixture repeats creation with reordered inputs and compares `ManifestDigest`; a feature with one bounded objective and disjoint paths/resources must pass, while overlapping teams serialize and audit/review writes are rejected. Repository tests use canonical `.agent-team/runs/<run-id>.json` and `.agent-team/teams/<team-id>.json` paths, assert Initialize is idempotent only for the same envelope, reject stale `CompareAndSwap` revisions, and verify successful writes advance `RecordEnvelope.Revision` monotonically with atomic last-good replacement.
- [ ] **Step 2: Run:** `cd vnext && go test ./internal/run`; **Expected:** FAIL because manifests are absent.
- [ ] **Step 3: Implement** schema-1 run files under `.agent-team/runs`, selected tracker authority, immutable one-off fields, bounded team queues, path/resource conflict detection, and no second ledger.
- [ ] **Step 4: Run:** `cd vnext && gofmt -w internal/run/run.go internal/run/manifest.go internal/run/run_test.go && go test ./internal/run -count=1`; **Expected:** PASS.
- [ ] **Step 5: Commit:** `git add vnext/internal/run && git commit -m "feat(vnext): add plan and one-off manifests"`.

### Task 6: Revision-checked atomic admission, refill history, idempotency, and capacity

**Files:** Create `vnext/internal/admission/{admission.go,admission_test.go}` and modify `vnext/internal/testkit/{run.go,store.go}`.

**Interfaces produced:**

**Package:** `vnext/internal/admission` (`package admission`). The modified `vnext/internal/testkit` (`package testkit`) implements the fixture declarations below without importing the admission package.

```go
type OutcomeKind string
const ( Created OutcomeKind = "created"; Duplicate OutcomeKind = "duplicate"; Stale OutcomeKind = "stale" )
type AdmissionOutcome struct { Kind OutcomeKind; Run core.RunRecord; TeamRevision uint64; Fingerprint string; Warning string }
func AppendAdmission(ctx context.Context, st *store.Store, tr tracker.Tracker, runID core.RunID, expectedRunRevision uint64, teamID core.TeamID, expectedTeamRevision uint64, expectedTrackerRevision uint64, expectedTaskRevisions map[core.TaskID]uint64, batch run.AdmissionBatch) (AdmissionOutcome, error)
```

**Test fixture package:** `vnext/internal/testkit` (`package testkit`).

```go
type AdmissionFixture struct { Store *store.Store; Tracker tracker.Tracker; Run core.RunID; RunRevision uint64; Team core.TeamID; TeamRevision,TrackerRevision uint64; TaskRevisions map[core.TaskID]uint64 }
func NewAdmissionFixture(*testing.T) AdmissionFixture
func (AdmissionFixture) Batch(int) run.AdmissionBatch
```

- [ ] **Step 1: Write failing tests:**
```go
func TestAdmissionOutcomes(t *testing.T) {
    f := testkit.NewAdmissionFixture(t)
    for _, n := range []int{0, 1, 8, 9} {
        batch := f.Batch(n)
        out, err := admission.AppendAdmission(context.Background(), f.Store, f.Tracker, f.Run, f.RunRevision, f.Team, f.TeamRevision, f.TrackerRevision, f.TaskRevisions, batch)
        if n == 1 && (err != nil || out.Kind != admission.Created) { t.Fatal(n, err, out) }
        if n == 0 || n == 9 { if err == nil || !errors.Is(err, core.ErrBatch) { t.Fatal(n, err) } }
    }
    duplicate, err := admission.AppendAdmission(context.Background(), f.Store, f.Tracker, f.Run, f.RunRevision, f.Team, f.TeamRevision, f.TrackerRevision, f.TaskRevisions, f.Batch(1))
    if err != nil || duplicate.Kind != admission.Duplicate { t.Fatal(err, duplicate) }
    f.RunRevision++
    stale, err := admission.AppendAdmission(context.Background(), f.Store, f.Tracker, f.Run, f.RunRevision, f.Team, f.TeamRevision, f.TrackerRevision, f.TaskRevisions, f.Batch(1))
    if err != nil || stale.Kind != admission.Stale { t.Fatal(err, stale) }
}
```
`testkit.NewAdmissionFixture` and its `Batch` method create revisions 900/999/1000/1001 and disjoint/overlapping path-resource cases; each boundary asserts the typed outcome and that the store has no partial append.
- [ ] **Step 2: Run:** `cd vnext && go test ./internal/admission`; **Expected:** FAIL because admission is absent.
- [ ] **Step 3: Implement** one transaction that validates all revisions and path/resource preconditions against one tracker snapshot, appends `admissions/<team>/<batch>.json`, publishes team/run revisions atomically, and preserves prior state on interruption.
- [ ] **Step 4: Run:** `cd vnext && gofmt -w internal/admission/admission.go internal/admission/admission_test.go internal/testkit/run.go internal/testkit/store.go && go test ./internal/admission -count=1`; **Expected:** PASS.
- [ ] **Step 5: Commit:** `git add vnext/internal/admission vnext/internal/testkit && git commit -m "feat(vnext): add idempotent revision-checked admission"`.

### Task 7: Bounded receipts, evidence, decisions, blockers, rules, packets, and handoff

**Files:** Create `vnext/internal/knowledge/{receipt.go,evidence.go,decision.go,blocker.go,rules.go,packet.go,handoff.go,knowledge_test.go}`.

**Interfaces produced:**

**Package:** `vnext/internal/knowledge` (`package knowledge`).

```go
type Owner struct { Team core.TeamID; Harness,Status string }
type ReviewFact struct { Reviewer,Revision,Verdict,Evidence string }
type GateFact struct { Revision,Result,Evidence string }
type Receipt struct { core.RecordEnvelope; SpecRevision string; Team core.TeamID; QueueFingerprint string; Task core.TaskID; Attempt int; State core.TaskState; Worktree,Dirty,Base,Candidate,Canonical,Integration string; Owner Owner; Scope []string; Checks []core.Check; Review ReviewFact; Gate GateFact; Resources core.ResourceSnapshot; Accelerators []core.AcceleratorRef; Next,Updated string }
type Evidence struct { core.RecordEnvelope; SpecRevision string; Team core.TeamID; Task core.TaskID; Attempt int; Owner,Worktree,QueueFingerprint string; Command []string; Exit int; TimedOut bool; OutputPointer,InputFingerprint,ReviewerFinding,Resolution string }
type Decision struct { core.RecordEnvelope; SpecRevision string; ID,Date,Scope,Decision,Rationale,Source string; Owner string; Alternatives,EvidencePointers []string }
type Blocker struct { core.RecordEnvelope; SpecRevision string; ID,Severity,RequestedChoice,SafeDefault,EvidencePointer,State,Owner string; Affected []string }
type Rule struct { core.RecordEnvelope; SpecRevision string; ID,Name,Content,DecisionID,Owner,Supersedes string }
type HandoffSnapshot struct { Run core.RunRecord; Teams []Receipt; Decisions []Decision; Blockers []Blocker; Resources core.ResourceSnapshot }
type BlockerStore interface { Create(context.Context,Blocker) (Blocker,error); Update(context.Context,Blocker) (Blocker,error); Resolve(context.Context,string,string) error }
type ProjectionWriter interface { WriteHandoff(context.Context,HandoffSnapshot) ([]byte,error); RegenerateBlockers(context.Context,[]Decision,[]Receipt,core.ResourceSnapshot) ([]Blocker,error) }
func NewBlockerStore(*store.Store) BlockerStore
func NewProjectionWriter(*store.Store) ProjectionWriter
func WriteReceipt(context.Context,*store.Store,Receipt) error
func WriteEvidence(context.Context,*store.Store,Evidence) error
func AppendDecision(context.Context,*store.Store,Decision) (string,error)
func PromoteRule(context.Context,*store.Store,Rule) error
func DeriveHandoff(HandoffSnapshot) ([]byte,error)
```

- [ ] **Step 1: Write failing tests:**
```go
func TestKnowledgeRecordsAndHandoff(t *testing.T) {
    s := store.New(t.TempDir(), core.StorageLimits{CanonicalBytes:16<<20, HandoffHardBytes:256<<10})
    r := knowledge.Receipt{RecordEnvelope:core.RecordEnvelope{Schema:1, Project:"p", RunID:"RUN-1", WrittenAt:"2026-09-19T00:00:00Z", Revision:4}, Team:"TEAM-1", Task:"TASK-1", State:core.Clean, Gate:knowledge.GateFact{Result:"CLEAN"}, Next:"integrate", Resources:core.ResourceSnapshot{Servers:[]string{"server:1"}}, Accelerators:[]core.AcceleratorRef{{Name:"serena", Status:core.AcceleratorAvailable, Digest:"sha256:x"}}}
    if err := knowledge.WriteReceipt(context.Background(), s, r); err != nil { t.Fatal(err) }
    if err := knowledge.WriteEvidence(context.Background(), s, knowledge.Evidence{RecordEnvelope:core.RecordEnvelope{Schema:1, Project:"p", RunID:"RUN-1", WrittenAt:"2026-09-19T00:00:00Z", Revision:1}, Task:"TASK-1", Attempt:1, Exit:0, InputFingerprint:"abc"}); err != nil { t.Fatal(err) }
    d1, err := knowledge.AppendDecision(context.Background(), s, knowledge.Decision{Scope:"run", Decision:"approve", Rationale:"safe"})
    if err != nil || !strings.HasPrefix(d1, "DEC-") { t.Fatal(err, d1) }
    d2, err := knowledge.AppendDecision(context.Background(), s, knowledge.Decision{Scope:"run", Decision:"refuse", Rationale:"unsafe"})
    if err != nil || d1 == d2 { t.Fatal(err, d1, d2) }
    handoff, err := knowledge.DeriveHandoff(knowledge.HandoffSnapshot{Run:core.RunRecord{RecordEnvelope:core.RecordEnvelope{Schema:1, Project:"p", RunID:"RUN-1", WrittenAt:"2026-09-19T00:00:00Z", Revision:4}, ID:"RUN-1"}, Teams:[]knowledge.Receipt{r}, Decisions:[]knowledge.Decision{{ID:d1}}, Blockers:[]knowledge.Blocker{{ID:"BLK-1", State:"open"}}, Resources:r.Resources})
    if err != nil || len(handoff) == 0 { t.Fatal(err) }
}
```
The table adds oversized command/output pointers and FIX/open-blocker variants; expected behavior is a bounded typed error, no partial file, and a handoff containing the revision, gate, resources, decisions, blockers, and next action. Tests call `BlockerStore.Create`, `Update`, and `Resolve`, then assert resolution removes the blocker from `BLOCKERS.md`; they call `ProjectionWriter.WriteHandoff` and `RegenerateBlockers` with decision, receipt, and resource inputs and assert canonical paths `.agent-team/handoffs/<run>.md` and `BLOCKERS.md`, bounded atomic writes, and monotonic envelope revisions.
```go
func TestBlockerResolveAndProjection(t *testing.T) { s := store.New(t.TempDir(), core.StorageLimits{CanonicalBytes:16<<20, HandoffHardBytes:256<<10}); bs := knowledge.NewBlockerStore(s); b, err := bs.Create(context.Background(), knowledge.Blocker{RecordEnvelope:core.RecordEnvelope{Schema:1, Project:"p", RunID:"R-1", WrittenAt:"2026-09-19T00:00:00Z", Revision:1}, ID:"BLK-1", State:"open"}); if err != nil { t.Fatal(err) }; if err := bs.Resolve(context.Background(), b.ID, "DEC-1"); err != nil { t.Fatal(err) }; pw := knowledge.NewProjectionWriter(s); if _, err := pw.RegenerateBlockers(context.Background(), nil, nil, core.ResourceSnapshot{}); err != nil { t.Fatal(err) } }
```
- [ ] **Step 2: Run:** `cd vnext && go test ./internal/knowledge`; **Expected:** FAIL because record schemas are absent.
- [ ] **Step 3: Implement** `.agent-team/receipts/<team>.json`, `.agent-team/evidence/<task>/<attempt>/`, canonical `DECISIONS.md`, derived `BLOCKERS.md`, promoted `AGENT_TEAM_RULES.md`, immutable packet digests, and one bounded `.agent-team/handoffs/<run>.md` projection.
- [ ] **Step 4: Run:** `cd vnext && gofmt -w internal/knowledge/receipt.go internal/knowledge/evidence.go internal/knowledge/decision.go internal/knowledge/blocker.go internal/knowledge/rules.go internal/knowledge/packet.go internal/knowledge/handoff.go internal/knowledge/knowledge_test.go && go test ./internal/knowledge -count=1`; **Expected:** PASS.
- [ ] **Step 5: Commit:** `git add vnext/internal/knowledge && git commit -m "feat(vnext): add bounded recovery knowledge"`.

### Task 8: Unified scoped workflow and durable checkpoint state

**Files:** Create `vnext/internal/workflow/{workflow.go,workflow_test.go}`.

**Interfaces produced:**

**Package:** `vnext/internal/workflow` (`package workflow`).

```go
type EventKind string
const ( Pause EventKind = "pause"; Stop EventKind = "stop"; Cancel EventKind = "cancel"; Resume EventKind = "resume"; Checkpoint EventKind = "checkpoint" )
type Event struct { Run core.RunID; Scope core.Scope; Kind EventKind; From,To core.TaskState; Reason,CheckpointDigest string; Confirmed bool; AdmissionHeld,RefillHeld bool }
func Transition(core.TaskState, Event) (core.TaskState,error)
func Checkpoint(context.Context,*store.Store,core.RunID,core.Scope,string) error
```

- [ ] **Step 1: Write table-driven failing tests:**
```go
func TestScopedTransitions(t *testing.T) {
    cases := []struct{ name string; state core.TaskState; event workflow.Event; want core.TaskState; wantErr bool }{
        {"task pause", core.Implementing, workflow.Event{Scope:core.Scope{Kind:core.ScopeTask, ID:"T1"}, Kind:workflow.Pause, AdmissionHeld:true, RefillHeld:true}, core.Paused, false},
        {"blocked resume", core.Blocked, workflow.Event{Scope:core.Scope{Kind:core.ScopeTeam, ID:"TEAM-1"}, Kind:workflow.Resume}, core.Ready, false},
        {"invalid", core.Clean, workflow.Event{Kind:workflow.Resume}, "", true},
    }
    for _, tc := range cases { t.Run(tc.name, func(t *testing.T) { got, err := workflow.Transition(tc.state, tc.event); if (err != nil) != tc.wantErr || !tc.wantErr && got != tc.want { t.Fatal(got, err) } }) }
}
```
The table includes project pause with `Confirmed:true`, stop/cancel/checkpoint events, interrupted preservation, and an unrelated task event; the checkpoint case asserts an on-disk digest and both admission/refill holds.
- [ ] **Step 2: Run:** `cd vnext && go test ./internal/workflow`; **Expected:** FAIL because typed events/transitions are absent.
- [ ] **Step 3: Implement** transitions using the single `core.TaskState` type, durable checkpoint receipt updates, scope validation, and no lease/PID/host-liveness dependency.
- [ ] **Step 4: Run:** `cd vnext && gofmt -w internal/workflow/workflow.go internal/workflow/workflow_test.go && go test ./internal/workflow -count=1`; **Expected:** PASS.
- [ ] **Step 5: Commit:** `git add vnext/internal/workflow && git commit -m "feat(vnext): add scoped workflow state"`.

### Task 9: Abstract capacity/resources and complete later-phase ports

**Files:** Create `vnext/internal/resources/{resources.go,resources_test.go}` and `vnext/internal/contracts/{ports.go,types.go}`.

**Interfaces produced:**

**Packages:** `vnext/internal/resources` (`package resources`) for capacity values and `vnext/internal/contracts` (`package contracts`) for later-phase ports.

```go
type Capacity struct { Name string; Configured,Observed,Effective,Reserved int; Unknown bool }
type Reservation struct { Run core.RunID; Team core.TeamID; Name string; Count int; State string }
func Reserve([]Capacity,[]Reservation) error
type HostCapabilities struct { Host,OS string; ConfiguredSlots,ObservedSlots,UsableSlots,DeveloperSlots,ReviewerSlots int; Models []string; Servers,Browsers int; Unknown bool }
type WorkerRequest struct { Packet core.AssignmentPacket; Worktree WorktreeSpec; WritablePaths []string; Reviewer bool; Model string }
type WorkerHandle struct { Host,Identity,PacketDigest,CandidateRevision string; Run core.RunID; Team core.TeamID; Task core.TaskID; Attempt int; Sequence uint64; Reviewer bool }
type WorktreeSpec struct { Run core.RunID; Team core.TeamID; Root,Base string; WritablePaths []string }
type Worktree struct { Run core.RunID; Path,Branch,Base,Candidate,Canonical string; Team core.TeamID; Dirty bool }
type Candidate struct { Task core.TaskID; Revision,Base string; Worktree Worktree }
type GateInput struct { Run core.RunID; Task core.TaskID; Candidate Candidate; TrackerRevision,ReceiptRevision uint64; RequiredCheckFingerprints []string; ScopeFingerprint string; WorktreeDirty bool; ReceiptDigest,ReviewDigest string }
type GateResult struct { Revision,Result,Evidence,CleanEvidence string; CheckFingerprints []string }
type DeploymentBatch struct { Run core.RunID; TaskIDs []core.TaskID; Revisions []string; TargetProfile,AuthorizationRef string; ExecutorCommand,VerificationCommand []string; Fingerprint,IdempotencyKey string }
type Operation struct { Provider,ProviderID,IdempotencyKey,State,ExternalState string; Unknown bool }
type Verification struct { State,OutputPointer,InputFingerprint string; Command []string; Exit int }
type HostAdapter interface { Probe(context.Context)(HostCapabilities,error); StartWorker(context.Context,WorkerRequest)(WorkerHandle,error); StartReviewer(context.Context,WorkerRequest,WorkerHandle)(WorkerHandle,error); Poll(context.Context,WorkerHandle)(string,error); Stop(context.Context,WorkerHandle,core.Scope) error; ReadIdentity(context.Context,WorkerHandle)(string,error) }
type WorktreeManager interface { Create(context.Context,WorktreeSpec)(Worktree,error); Inspect(context.Context,Worktree)(Worktree,error); Integrate(context.Context,Candidate)(Candidate,error); Cleanup(context.Context,core.TeamID) error }
type CompletionGate interface { Check(context.Context,GateInput)(GateResult,error) }
type DeploymentExecutor interface { Submit(context.Context,DeploymentBatch)(Operation,error); Query(context.Context,Operation)(Operation,error); Verify(context.Context,Operation)(Verification,error) }
```

- [ ] **Step 1: Write table-driven failing tests:**
```go
func TestCapacityMatrix(t *testing.T) {
    cases := []struct{ name string; caps []resources.Capacity; reservations []resources.Reservation; wantErr bool }{
        {"zero", []resources.Capacity{{Name:"worker", Effective:0}}, nil, true},
        {"one", []resources.Capacity{{Name:"worker", Effective:1}}, []resources.Reservation{{Name:"worker", Count:1}}, false},
        {"many", []resources.Capacity{{Name:"worker", Effective:3}}, []resources.Reservation{{Name:"worker", Count:2}}, false},
        {"unknown", []resources.Capacity{{Name:"reviewer", Unknown:true}}, []resources.Reservation{{Name:"reviewer", Count:1}}, true},
    }
    for _, tc := range cases { t.Run(tc.name, func(t *testing.T) { if got := resources.Reserve(tc.caps, tc.reservations); (got != nil) != tc.wantErr { t.Fatal(got) } }) }
    _ = contracts.HostCapabilities{Host:"test", OS:"windows", ConfiguredSlots:2, ObservedSlots:1, UsableSlots:1, DeveloperSlots:1, ReviewerSlots:0, Servers:2, Browsers:2}
    _ = contracts.WorkerRequest{Packet:core.AssignmentPacket{RecordEnvelope:core.RecordEnvelope{Schema:1}}, WritablePaths:[]string{"src"}}
}
```
The table adds effective `N`, observed-versus-configured mismatch, reserved reviewer capacity, and disposable-worker cases; constructing every declared port value must not start a process or mutate a worktree.
- [ ] **Step 2: Run:** `cd vnext && go test ./internal/resources ./internal/contracts`; **Expected:** FAIL because resources and ports are absent.
- [ ] **Step 3: Implement** pure reservations and the port/value types declared above; operational methods are declarations only and cannot create processes, worktrees, or deployments.
- [ ] **Step 4: Run:** `cd vnext && gofmt -w internal/resources/resources.go internal/resources/resources_test.go internal/contracts/ports.go internal/contracts/types.go && go test ./internal/resources ./internal/contracts`; **Expected:** PASS.
- [ ] **Step 5: Commit:** `git add vnext/internal/resources vnext/internal/contracts && git commit -m "feat(vnext): define capacity and later-phase ports"`.

### Task 10: Canonical CLI actions and deferred operational outcomes

**Files:** Modify `vnext/internal/cli/{app.go,parser.go,app_test.go}` and `vnext/cmd/agent-teamctl/main.go`.

**Interfaces produced:**

**Package:** `vnext/internal/cli` (`package cli`).

```go
type Action struct { Name string; Args []string; JSON bool }
func Parse([]string) (Action,error)
```

Task 10 calls the single `internal/cli.Run(context.Context, []string, core.Dependencies) int` produced by Task 1; it does not declare a second runner.

- [ ] **Step 1: Write table-driven failing tests:**
```go
func TestCanonicalActions(t *testing.T) {
    accepted := [][]string{{"setup"}, {"settings"}, {"status"}, {"start"}, {"task","add","--queue"}, {"task","add","--execute"}, {"one-off","feature","--objective","inspect feature"}, {"one-off","audit","--objective","inspect config"}, {"one-off","review","--objective","review change"}, {"pause","--scope","team:TEAM-1"}, {"inspect"}, {"cleanup"}, {"deploy"}}
    rejected := [][]string{{"review"}, {"gate"}, {"integrate"}, {"reconcile"}, {"checkpoint"}, {"list"}, {"archive"}, {"plan"}}
    for _, args := range accepted { if _, err := cli.Parse(args); err != nil { t.Fatalf("accepted %v: %v", args, err) } }
    for _, args := range rejected { if _, err := cli.Parse(args); !errors.Is(err, core.ErrPhase) { t.Fatalf("rejected %v: %v", args, err) } }
}
```
- [ ] **Step 2: Run:** `cd vnext && go test ./internal/cli`; **Expected:** FAIL because canonical parsing and deferred outcomes are absent.
- [ ] **Step 3: Implement** argument-only parsing, canonical JSON/text output, run disambiguation, and `core.ErrPhase` for deferred operational actions without mutating hosts, legacy files, product code, processes, or credentials.
- [ ] **Step 4: Run:** `cd vnext && gofmt -w cmd/agent-teamctl/main.go internal/cli/app.go internal/cli/parser.go internal/cli/app_test.go && go test ./internal/cli -count=1`; **Expected:** PASS.
- [ ] **Step 5: Commit:** `git add vnext/cmd/agent-teamctl vnext/internal/cli && git commit -m "feat(vnext): add canonical deferred actions"`.

### Task 11: Native CI and complete Phase 1 acceptance

**Files:** Create `.github/workflows/vnext-native.yml` and `vnext/internal/acceptance/acceptance_test.go`.

- [ ] **Step 1: Write failing acceptance tests:**
```go
func TestPhase1Acceptance(t *testing.T) {
    root := testkit.GitRepo(t)
    before := testkit.SnapshotTree(t, root)
    if code := cli.Run(context.Background(), []string{"setup", "--refuse-kickoff", "--json"}, core.Dependencies{ProjectRoot:root, Stdout:io.Discard, Stderr:io.Discard}); code == 0 { t.Fatal("refused setup succeeded") }
    if after := testkit.SnapshotTree(t, root); !reflect.DeepEqual(before, after) { t.Fatal("refused setup wrote files") }
    approvedRoot := testkit.GitRepo(t)
    approvedDeps := core.Dependencies{ProjectRoot:approvedRoot, Stdout:io.Discard, Stderr:io.Discard, Confirmations:map[string]bool{"kickoff":true}}
    if code := cli.Run(context.Background(), []string{"setup", "--approve-kickoff", "--json"}, approvedDeps); code != 0 { t.Fatal("approved setup failed", code) }
    deps := approvedDeps
    cases := []struct { name string; args []string; wantZero bool }{
        {"settings", []string{"settings"}, true},
        {"status", []string{"status", "--json"}, true},
        {"start-deferred", []string{"start"}, false},
        {"task-queue", []string{"task", "add", "--queue"}, true},
        {"task-execute-deferred", []string{"task", "add", "--execute"}, false},
        {"one-off-feature", []string{"one-off", "feature", "--objective", "inspect feature"}, true},
        {"one-off-audit", []string{"one-off", "audit", "--objective", "inspect config"}, true},
        {"one-off-review", []string{"one-off", "review", "--objective", "review change"}, true},
        {"pause-team", []string{"pause", "--scope", "team:TEAM-1"}, true},
        {"inspect", []string{"inspect"}, true},
        {"cleanup-deferred", []string{"cleanup"}, false},
        {"deploy-deferred", []string{"deploy"}, false},
    }
    for _, tc := range cases { t.Run(tc.name, func(t *testing.T) { code := cli.Run(context.Background(), tc.args, deps); if (code == 0) != tc.wantZero { t.Fatalf("args=%v code=%d wantZero=%v", tc.args, code, tc.wantZero) } }) }
}
```
The table uses the temporary Git root fixture and separate TASKS/Beads fixtures for the task and tracker cases; additional subtests assert schema-1 envelope precedence, tracker counts 900/1000/1001, one-off read-only safety, duplicate/stale admission outcomes, bounded handoff, scoped checkpoint, capacity refusal, `core.ErrPhase` for deferred actions, and no path outside `root`.
- [ ] **Step 2: Run:** `cd vnext && go test ./internal/acceptance`; **Expected:** FAIL until all Phase 1 packages and wiring exist.
- [ ] **Step 3: Implement** only test orchestration and platform workflow; the acceptance test invokes `gofmt` with `exec.CommandContext(ctx, "gofmt", "-l", ".")` and fails when stdout is nonempty, so no shell-specific check is required.
- [ ] **Step 4: Run:** native Ubuntu/macOS/Windows workflow steps set `working-directory: vnext` and execute `go test ./...` then `go vet ./...`; the acceptance test performs the argument-array `gofmt` check. Cross-build matrix steps set `GOOS`/`GOARCH` through runner-native environment fields and execute `go build ./cmd/agent-teamctl` for `linux/amd64`, `darwin/arm64`, and `windows/amd64`.
- [ ] **Step 5: Commit:** `git add .github/workflows/vnext-native.yml vnext/internal/acceptance && git commit -m "test(vnext): add Phase 1 native acceptance"`.

## Spec Coverage and Self-Review

- Tasks 1–4 cover Go runtime, shared schemas, canonical paths/Git, bounded storage, setup/Kickoff confirmation, and tracker/command contracts.
- Tasks 5–6 cover plan/one-off manifests, trackerless read-only kinds, bounded queues, same-revision admissions, capacity limits, and idempotent refill history.
- Tasks 7–8 cover complete bounded receipts/evidence/decisions/blockers/rules/packets/handoff and unified scoped control/checkpoints.
- Task 9 defines all Phase 2+ value types and ports without operational behavior; Task 10 implements only the public Phase 1 action boundary; Task 11 verifies native acceptance.
- Every symbol used in a later task is defined in an earlier task or that task’s Interfaces block. Every test requirement is represented by a concrete test table or snippet and command; no vague or placeholder instruction remains.
- No task imports or executes v7 runtime code, hooks, Node/Python, optional tools, host configuration, or deferred operational implementations.

## Execution Handoff

Plan complete and saved to `docs/superpowers/plans/2026-09-18-agent-team-vnext-native-core.md`. Two execution options:

1. **Subagent-Driven (recommended)** — dispatch a fresh subagent per task and review between tasks.
2. **Inline Execution** — execute tasks in this session using executing-plans with checkpoints.

Which approach?
