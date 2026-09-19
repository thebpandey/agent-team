# Agent-Team vNext Authorized Deployment and Recovery Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a disabled-by-default, provider-neutral deployment service that submits only explicitly authorized non-production batches, recovers unknown operations without duplicate submission, persists evidence, and keeps coding lanes running when deployment is blocked.

**Architecture:** `internal/deploy` owns profile authorization, deterministic batch manifests, bounded native command execution, durable operation/receipt/evidence persistence, and recovery coordination. Provider adapters receive the exact Phase 1 `contracts.DeploymentBatch`; production authority is never inferred. The existing Phase 1 `cli.Run(context.Context, []string, core.Dependencies) int` remains the only CLI runner; Phase 4 adds one callback field to `core.Dependencies` and does not create another parser or runner.

**Tech Stack:** Go standard library, Phase 1 `core`, `store`, `knowledge`, and `contracts` packages; `os/exec` with argument arrays; bounded pipes; SHA-256; atomic Phase 1 store writes; fake provider tests only.

**Spec:** `docs/superpowers/specs/2026-09-18-agent-team-vnext-design.md`

## Global Constraints

- Deployment is disabled until a named target profile has `Confirmed`, a nonempty authorization reference/scope, valid command arrays, and an immutable derived digest.
- Only explicitly authorized non-production targets are accepted; production targets are rejected by validation and never appear in live tests.
- Every durable deployment value embeds `core.RecordEnvelope`; all CAS writes use Phase 1 `store.New(root, core.StorageLimits)` and bounded JSON.
- The only Phase 1 sentinel errors used are `core.ErrPath`, `core.ErrGit`, `core.ErrLimit`, `core.ErrCapacity`, `core.ErrBatch`, `core.ErrRevision`, `core.ErrTransition`, `core.ErrSettings`, and `core.ErrPhase`. Deployment-only `ErrNotFound`, `ErrAction`, `ErrProvider`, and `ErrOutputLimit` are declared locally.
- Fingerprints include run, target, profile digest, ordered task IDs, canonical revisions, per-task artifact digests, and sequence. Changed inputs produce new fingerprints and idempotency keys.
- Submit writes a pending operation before provider submission, then CAS-updates that operation with provider ID/state before writing receipt/evidence. Existing operations are read before any submit.
- Unknown or ambiguous operations query before verification and never submit again. Failed verification persists evidence and receipt, creates a blocker, holds later batches, and leaves `CodingMayContinue:true`.
- No automatic rollback, provider SDK, shell, hook, daemon, renderer, lease, production credential, or host mutation is introduced.

## File Structure

| Path | Responsibility |
| --- | --- |
| `vnext/internal/deploy/types.go` | Authoritative deployment types and repository/provider interfaces. |
| `vnext/internal/deploy/errors.go` | Deployment-local not-found/action/provider/output errors. |
| `vnext/internal/deploy/profile.go` | Profile digest, authorization decision, and store-backed CAS. |
| `vnext/internal/deploy/batch.go` | Eligibility, 1–100 batch sizing, fingerprint, and Phase 1 contract conversion. |
| `vnext/internal/deploy/runner.go` | Safe argv validation, owned argument files, bounded native process execution, and command provider. |
| `vnext/internal/deploy/service.go` | Idempotent submit, operation update, query-before-retry, verify, and durable evidence/receipt. |
| `vnext/internal/deploy/recovery.go` | Blocker, later-batch hold, dashboard best-effort notification, and explicit rollback authorization. |
| `vnext/internal/deploy/*_test.go` | Focused RED/GREEN tests and fake provider/repository fixtures. |
| `vnext/internal/cli/app.go`, `parser.go` | Existing canonical `cli.Run`; add deployment callback dispatch only. |
| `vnext/internal/core/types.go` | Existing `Dependencies` plus one deployment callback field; preserve all Phase 1 fields. |
| `vnext/internal/acceptance/deployment_test.go` | End-to-end fake-provider vertical acceptance using canonical `cli.Run`. |
| `.github/workflows/vnext-deployment.yml` | Native Linux/macOS/Windows test, vet, and acceptance matrix. |

### Task 1: Define profile, operation, receipt, evidence, recovery, and provider contracts

**Files:**

- Create: `vnext/internal/deploy/types.go`
- Create: `vnext/internal/deploy/errors.go`
- Test: `vnext/internal/deploy/types_test.go`

**Interfaces:** Consume the exact Phase 1 `core.RecordEnvelope`, `core.RunID`, `core.TaskID`, `core.StorageLimits`, `store.Store`, `knowledge.Blocker`, `contracts.DeploymentBatch`, `contracts.Operation`, and `contracts.Verification`. Produce:

```go
package deploy

import (
    "context"
    "github.com/thebpandey/agent-team/vnext/internal/contracts"
    "github.com/thebpandey/agent-team/vnext/internal/core"
    "github.com/thebpandey/agent-team/vnext/internal/knowledge"
)

type ProfileID string
type ProfileDecision struct { Enable bool; Target, AuthorizationRef, ApprovalScope string; ExecutorCommand, QueryCommand, VerificationCommand []string; DefaultBatchSize int; Confirmed bool }
type TargetProfile struct { core.RecordEnvelope; ID ProfileID; Target, AuthorizationRef, ApprovalScope, ProfileDigest string; ExecutorCommand, QueryCommand, VerificationCommand []string; DefaultBatchSize int; Enabled, Confirmed bool }
type ProfileWriteKind string
const ( ProfileCreated ProfileWriteKind = "created"; ProfileUpdated ProfileWriteKind = "updated"; ProfileDuplicate ProfileWriteKind = "duplicate" )
type ProfileWriteOutcome struct { Kind ProfileWriteKind; Profile TargetProfile; ExpectedRevision, ObservedRevision uint64; Idempotent bool }
type EligibleTask struct { ID core.TaskID; CanonicalRevision string; ArtifactDigests []string; Integrated, GatePassed, DependenciesComplete bool }
type BatchRequest struct { RunID core.RunID; Target string; Profile TargetProfile; Tasks []EligibleTask; BatchSize int }
type BatchManifest struct { core.RecordEnvelope; BatchID string; RunID core.RunID; Sequence int; Target, ProfileDigest, Fingerprint, IdempotencyKey string; TaskIDs []core.TaskID; Revisions []string; ArtifactDigests [][]string; Final bool }
type CommandInvocation struct { Executable string; Args, Env []string; ArgumentFile, OwnedRoot string; OutputLimit int64 }
type CommandResult struct { Stdout, Stderr []byte; Exit int; TimedOut, Started, OutputTruncated bool; Transport error }
type OperationRecord struct { core.RecordEnvelope; BatchID, Fingerprint, ProfileID, Target, State string; Operation contracts.Operation; Attempt int }
type DeploymentEvidence struct { core.RecordEnvelope; BatchID, ProfileID, Target, Fingerprint, State, Error, OutputPointer string; Attempt, Exit int; Command []string }
type DeploymentReceipt struct { core.RecordEnvelope; RunID core.RunID; BatchID, ProviderID, ProfileID, Target, Fingerprint, IdempotencyKey, State string; Verification contracts.Verification; EvidencePointers []string; CodingMayContinue bool }
type DeployOutcome struct { Receipt DeploymentReceipt; BlockerID string; CodingMayContinue bool }
type DashboardTriggerRequest struct { RunID core.RunID; BatchID, ReceiptPointer, Reason string }
type ProviderAdapter interface { Submit(context.Context, contracts.DeploymentBatch, string) (contracts.Operation, error); Query(context.Context, contracts.Operation, string) (contracts.Operation, error); Verify(context.Context, contracts.Operation, string) (contracts.Verification, error) }
type ProcessRunner interface { Run(context.Context, CommandInvocation) CommandResult }
type Repository interface { ReadOperation(context.Context, string) (OperationRecord, error); CompareAndSwapOperation(context.Context, uint64, OperationRecord) (OperationRecord, error); WriteEvidence(context.Context, DeploymentEvidence) (string, error); ReadReceipt(context.Context, string) (DeploymentReceipt, error); CompareAndSwapReceipt(context.Context, uint64, DeploymentReceipt) (DeploymentReceipt, error) }
type ProfileRepository interface { ApplyDecision(context.Context, ProfileID, ProfileDecision, uint64) (ProfileWriteOutcome, error); Get(context.Context, ProfileID) (TargetProfile, error) }
type BlockerWriter interface { Create(context.Context, knowledge.Blocker) (knowledge.Blocker, error) }
type HoldWriter interface { HoldLater(context.Context, core.RunID, string) error }
type DashboardTrigger interface { Trigger(context.Context, DashboardTriggerRequest) error }
type BoundExecutor struct { Profile TargetProfile; Provider ProviderAdapter }
type CommandProvider struct { Profile TargetProfile; Runner ProcessRunner; Root string; Limit int64 }
type RecoveryService struct { Blockers BlockerWriter; Holds HoldWriter; Dashboard DashboardTrigger }
```

`errors.go` declares `ErrNotFound`, `ErrAction`, `ErrProvider`, and `ErrOutputLimit` locally with `errors.New`; it does not add names to `core`.

- [ ] **Step 1: Write the failing contract test.** Create `types_test.go` with:

```go
package deploy

import (
    "encoding/json"
    "testing"
    "github.com/thebpandey/agent-team/vnext/internal/core"
)

func TestDeploymentContractsRoundTrip(t *testing.T) {
    p := TargetProfile{ID:"staging", Target:"staging", ExecutorCommand:[]string{"provider"}, QueryCommand:[]string{"provider-query"}, VerificationCommand:[]string{"provider-verify"}, DefaultBatchSize:3}
    raw, err := json.Marshal(p); if err != nil { t.Fatal(err) }
    var got TargetProfile; if err := json.Unmarshal(raw, &got); err != nil || got.ID != p.ID || got.DefaultBatchSize != 3 { t.Fatal(err, got) }
    var _ core.TaskID = core.TaskID("T-1")
}
```

- [ ] **Step 2: Run the failing test.** Run `cd vnext && go test ./internal/deploy -run TestDeploymentContractsRoundTrip -count=1`. Expected: FAIL because deployment types are absent.
- [ ] **Step 3: Implement the exact declarations.** Add package/import declarations, local errors, and JSON tags only where needed for bounded durable records. Use Phase 1 contract field names exactly; do not add a second runner or alter `contracts.Operation`/`contracts.Verification`.
- [ ] **Step 4: Run the contract test.** Run `cd vnext && gofmt -w internal/deploy/types.go internal/deploy/errors.go internal/deploy/types_test.go && go test ./internal/deploy -run TestDeploymentContractsRoundTrip -count=1`. Expected: PASS.
- [ ] **Step 5: Commit.** Run `git add vnext/internal/deploy/types.go vnext/internal/deploy/errors.go vnext/internal/deploy/types_test.go && git commit -m "feat(deploy): define deployment contracts"`.

### Task 2: Implement authorized profile CAS and deterministic batch planning

**Files:**

- Create: `vnext/internal/deploy/profile.go`
- Create: `vnext/internal/deploy/batch.go`
- Test: `vnext/internal/deploy/profile_test.go`
- Test: `vnext/internal/deploy/batch_test.go`

**Interfaces:** `NewProfileRepository(*store.Store) ProfileRepository`, `DeriveProfileDigest(TargetProfile) (string,error)`, `ValidateProfile(TargetProfile) error`, `PlanBatches(context.Context, BatchRequest) ([]BatchManifest,error)`, `DeriveBatchFingerprint(BatchManifest) (string,error)`, `DeriveBatchIdempotencyKey(core.RunID,int,string) string`, `ResolveBatchSize(TargetProfile,int) (int,error)`, and `(BatchManifest).ContractBatch(TargetProfile) (contracts.DeploymentBatch,error)`.

- [ ] **Step 1: Write failing profile and batch tests.** Use the exact Phase 1 store constructor and test all authorization/CAS and seven-task cases:

```go
package deploy

import (
    "context"
    "errors"
    "fmt"
    "testing"
    "github.com/thebpandey/agent-team/vnext/internal/core"
    "github.com/thebpandey/agent-team/vnext/internal/store"
)

func TestProfileCASAuthorizationDigestAndDuplicate(t *testing.T) {
    s := store.New(t.TempDir(), core.StorageLimits{CanonicalBytes:64*1024}); repo := NewProfileRepository(s)
    d := ProfileDecision{Enable:true, Target:"staging", AuthorizationRef:"approval-1", ApprovalScope:"staging", ExecutorCommand:[]string{"provider"}, QueryCommand:[]string{"provider-query"}, VerificationCommand:[]string{"provider-verify"}, DefaultBatchSize:3, Confirmed:true}
    first, err := repo.ApplyDecision(context.Background(), "staging", d, 0); if err != nil || first.Kind != ProfileCreated || first.Profile.ProfileDigest == "" { t.Fatal(first, err) }
    duplicate, err := repo.ApplyDecision(context.Background(), "staging", d, first.ObservedRevision); if err != nil || duplicate.Kind != ProfileDuplicate || !duplicate.Idempotent || duplicate.ObservedRevision != first.ObservedRevision { t.Fatal(duplicate, err) }
    if _, err := repo.ApplyDecision(context.Background(), "staging", d, 0); !errors.Is(err, core.ErrRevision) { t.Fatal("stale profile accepted", err) }
    changed := first.Profile; changed.ProfileDigest = "sha256:wrong"; if err := ValidateProfile(changed); !errors.Is(err, core.ErrSettings) { t.Fatal("changed digest accepted", err) }
}
func TestSevenTasksPlanThreeThreeOneAndArtifactFingerprint(t *testing.T) {
    p := TargetProfile{ID:"staging", Target:"staging", AuthorizationRef:"approval-1", ApprovalScope:"staging", ExecutorCommand:[]string{"provider"}, QueryCommand:[]string{"provider-query"}, VerificationCommand:[]string{"provider-verify"}, Enabled:true, DefaultBatchSize:3}; p.ProfileDigest, _ = DeriveProfileDigest(p)
    tasks := make([]EligibleTask, 7); for i := range tasks { tasks[i] = EligibleTask{ID:core.TaskID(fmt.Sprintf("T-%d", i)), CanonicalRevision:fmt.Sprintf("rev-%d", i), ArtifactDigests:[]string{fmt.Sprintf("artifact-%d", i)}, Integrated:true, GatePassed:true, DependenciesComplete:true} }
    batches, err := PlanBatches(context.Background(), BatchRequest{RunID:"R-1", Target:"staging", Profile:p, Tasks:tasks, BatchSize:3}); if err != nil || len(batches)!=3 || len(batches[0].TaskIDs)!=3 || len(batches[1].TaskIDs)!=3 || len(batches[2].TaskIDs)!=1 { t.Fatal(len(batches), err) }
    changed := batches[0]; changed.ArtifactDigests = append([][]string(nil), batches[0].ArtifactDigests...); changed.ArtifactDigests[0] = append([]string(nil), batches[0].ArtifactDigests[0]...); changed.ArtifactDigests[0][0] = "artifact-changed"; a, _ := DeriveBatchFingerprint(batches[0]); b, _ := DeriveBatchFingerprint(changed); if a == b { t.Fatal("artifact change reused fingerprint") }
    if batches[0].IdempotencyKey != DeriveBatchIdempotencyKey("R-1", 0, batches[0].Fingerprint) { t.Fatal("unstable idempotency key") }
    if n, _ := ResolveBatchSize(p, 0); n != 3 { t.Fatal(n) }; if _, err := ResolveBatchSize(p, 101); !errors.Is(err, core.ErrBatch) { t.Fatal(err) }
}
```

- [ ] **Step 2: Run the focused failures.** Run `cd vnext && go test ./internal/deploy -run 'TestProfileCASAuthorizationDigestAndDuplicate|TestSevenTasksPlanThreeThreeOneAndArtifactFingerprint' -count=1`. Expected: FAIL because profile CAS and batch planning are absent.
- [ ] **Step 3: Implement profile CAS.** Read `profiles/<id>.json` through the Phase 1 store, treat only local `ErrNotFound`/`fs.ErrNotExist` as an absent revision, reject stale expected revisions, set `p.ID` before deriving its digest, compare the complete requested decision for duplicate idempotency, and atomically write revision `expected+1`. `ValidateProfile` recomputes and compares the digest when enabled, rejects production targets, requires all three non-shell command arrays, and enforces batch size 1–100.

```go
func DeriveProfileDigest(p TargetProfile) (string,error) { raw,err:=json.Marshal(struct{ID ProfileID;Target,AuthorizationRef,ApprovalScope string;Exec,Query,Verify []string;Size int;Enabled,Confirmed bool}{p.ID,p.Target,p.AuthorizationRef,p.ApprovalScope,p.ExecutorCommand,p.QueryCommand,p.VerificationCommand,p.DefaultBatchSize,p.Enabled,p.Confirmed}); if err!=nil{return "",err}; sum:=sha256.Sum256(raw); return "sha256:"+hex.EncodeToString(sum[:]),nil }
func ValidateProfile(p TargetProfile) error { if p.Enabled&&(p.Target==""||strings.EqualFold(p.Target,"production")||p.AuthorizationRef==""||p.ApprovalScope==""){return core.ErrSettings}; if p.DefaultBatchSize<1||p.DefaultBatchSize>100{return core.ErrBatch}; for _,c:=range [][]string{p.ExecutorCommand,p.QueryCommand,p.VerificationCommand}{if err:=ValidateCommandTemplate(c);err!=nil{return err}}; if p.Enabled{d,err:=DeriveProfileDigest(p);if err!=nil||d!=p.ProfileDigest{return core.ErrSettings}}; return nil }
func DeriveBatchFingerprint(b BatchManifest)(string,error){ raw,err:=json.Marshal(struct{Run core.RunID;Target,Profile string;Tasks []core.TaskID;Revisions []string;Artifacts [][]string;Sequence int}{b.RunID,b.Target,b.ProfileDigest,b.TaskIDs,b.Revisions,b.ArtifactDigests,b.Sequence});if err!=nil{return "",err};sum:=sha256.Sum256(raw);return "sha256:"+hex.EncodeToString(sum[:]),nil }
```
- [ ] **Step 4: Implement profile repository and batch planning.** Validate run/target/profile authorization, reject empty tasks, duplicate IDs, missing canonical revisions, failed gates/dependencies, and size outside 1–100. Build batches in order, preserve each task’s artifact-digest slice, include all fingerprint inputs in canonical JSON, set `Final` only on the remainder batch, and convert to the exact Phase 1 `contracts.DeploymentBatch` fields. Add `MemoryProfileRepository` as the executable CAS oracle (the `store.Store` adapter must match it): lock, compare expected revision, derive the complete profile digest, return `ProfileDuplicate` for identical decisions, increment revision, and atomically save. Add `PlanBatches` and `(BatchManifest).ContractBatch` in the same package; the tests must exercise 7 tasks as 3,3,1 and mutate an artifact digest to prove fingerprint changes.
- [ ] **Step 4a: Add the executable CAS oracle.** The test package must include this implementation before the tests are run:

```go
type MemoryProfileRepository struct { mu sync.Mutex; revision uint64; profiles map[ProfileID]TargetProfile }
func NewMemoryProfileRepository()*MemoryProfileRepository{return &MemoryProfileRepository{profiles:map[ProfileID]TargetProfile{}}}
func(r *MemoryProfileRepository)ApplyDecision(_ context.Context,id ProfileID,d ProfileDecision,expected uint64)(ProfileWriteOutcome,error){r.mu.Lock();defer r.mu.Unlock();if expected!=r.revision{return ProfileWriteOutcome{},core.ErrRevision};p:=TargetProfile{ID:id,Target:d.Target,AuthorizationRef:d.AuthorizationRef,ApprovalScope:d.ApprovalScope,ExecutorCommand:d.ExecutorCommand,QueryCommand:d.QueryCommand,VerificationCommand:d.VerificationCommand,DefaultBatchSize:d.DefaultBatchSize,Enabled:d.Enable,Confirmed:d.Confirmed};var err error;p.ProfileDigest,err=DeriveProfileDigest(p);if err!=nil{return ProfileWriteOutcome{},err};if err=ValidateProfile(p);err!=nil{return ProfileWriteOutcome{},err};if old,ok:=r.profiles[id];ok&&reflect.DeepEqual(old,p){return ProfileWriteOutcome{Kind:ProfileDuplicate,Profile:old,ObservedRevision:r.revision,Idempotent:true},nil};r.revision++;p.Revision=r.revision;r.profiles[id]=p;kind:=ProfileCreated;if expected>0{kind=ProfileUpdated};return ProfileWriteOutcome{Kind:kind,Profile:p,ExpectedRevision:expected,ObservedRevision:r.revision},nil}
func DeriveBatchIdempotencyKey(run core.RunID,sequence int,fingerprint string)string{raw:=fmt.Sprintf("%s/%d/%s",run,sequence,fingerprint);sum:=sha256.Sum256([]byte(raw));return "deploy-v1:"+hex.EncodeToString(sum[:])}
func ResolveBatchSize(p TargetProfile,requested int)(int,error){size:=requested;if size==0{size=p.DefaultBatchSize};if size<1||size>100{return 0,core.ErrBatch};return size,nil}
func PlanBatches(_ context.Context,req BatchRequest)([]BatchManifest,error){if err:=ValidateProfile(req.Profile);err!=nil{return nil,err};size,err:=ResolveBatchSize(req.Profile,req.BatchSize);if err!=nil{return nil,err};if req.RunID==""||req.Target!=req.Profile.Target||len(req.Tasks)==0{return nil,core.ErrBatch};seen:=map[core.TaskID]bool{};for _,t:=range req.Tasks{if seen[t.ID]||t.CanonicalRevision==""||!t.Integrated||!t.GatePassed||!t.DependenciesComplete{return nil,core.ErrBatch};seen[t.ID]=true};out:=make([]BatchManifest,0);for start,seq:=0,0;start<len(req.Tasks);start,seq=start+size,seq+1{end:=start+size;if end>len(req.Tasks){end=len(req.Tasks)};m:=BatchManifest{BatchID:fmt.Sprintf("%s-batch-%d",req.RunID,seq+1),RunID:req.RunID,Sequence:seq,Target:req.Target,ProfileDigest:req.Profile.ProfileDigest,Final:end==len(req.Tasks)};for _,t:=range req.Tasks[start:end]{m.TaskIDs=append(m.TaskIDs,t.ID);m.Revisions=append(m.Revisions,t.CanonicalRevision);m.ArtifactDigests=append(m.ArtifactDigests,append([]string(nil),t.ArtifactDigests...))};m.Fingerprint,err=DeriveBatchFingerprint(m);if err!=nil{return nil,err};m.IdempotencyKey=DeriveBatchIdempotencyKey(m.RunID,m.Sequence,m.Fingerprint);out=append(out,m)};return out,nil}
func(m BatchManifest)ContractBatch(p TargetProfile)(contracts.DeploymentBatch,error){if err:=ValidateProfile(p);err!=nil{return contracts.DeploymentBatch{},err};return contracts.DeploymentBatch{Run:m.RunID,TaskIDs:m.TaskIDs,Revisions:m.Revisions,TargetProfile:string(p.ID),AuthorizationRef:p.AuthorizationRef,ExecutorCommand:p.ExecutorCommand,VerificationCommand:p.VerificationCommand,Fingerprint:m.Fingerprint,IdempotencyKey:m.IdempotencyKey},nil}
```
- [ ] **Step 5: Run profile/batch tests and commit.** Run `cd vnext && gofmt -w internal/deploy/profile.go internal/deploy/batch.go internal/deploy/profile_test.go internal/deploy/batch_test.go && go test ./internal/deploy -run 'TestProfile|TestSevenTasks' -count=1 && go vet ./internal/deploy`. Expected: PASS. Commit `feat(deploy): add authorized profiles and deterministic batches`.

### Task 3: Add safe bounded native execution and command-provider adapter

**Files:**

- Create: `vnext/internal/deploy/runner.go`
- Test: `vnext/internal/deploy/runner_test.go`

**Interfaces:** `ValidateCommandTemplate([]string) error`, `WriteOwnedArgumentFile(string,string,[]byte,int64) (string,error)`, `NativeRunner{Root string; Limit int64}`, `(NativeRunner).Run(context.Context,CommandInvocation) CommandResult`, and `CommandProvider{Profile TargetProfile; Runner ProcessRunner}` implementing the provider adapter calls.

- [ ] **Step 1: Write failing runner tests.** Use no shell and no network:

```go
package deploy

import (
    "bytes"
    "context"
    "errors"
    "io"
    "os/exec"
    "path/filepath"
    "os"
    "strings"
    "testing"
    "github.com/thebpandey/agent-team/vnext/internal/core"
)

func TestRunnerChild(t *testing.T) { if os.Getenv("AGENT_TEAM_RUNNER_CHILD") != "1" { return }; _, _ = os.Stdout.Write([]byte(strings.Repeat("x", 1<<20))); _, _ = os.Stderr.Write([]byte(strings.Repeat("e", 1<<20))); os.Exit(0) }
func TestCommandValidationAndStartFailure(t *testing.T) {
    for _, bad := range [][]string{{}, {"sh","-c","echo x"}, {"cmd.exe","/c","echo x"}, {"powershell.exe","-Command","echo x"}, {"provider","$(secret)"}, {"provider","${TOKEN}"}, {"provider","..\\escape"}} { if err := ValidateCommandTemplate(bad); err == nil { t.Fatal("accepted", bad) } }
    result := (NativeRunner{Root:t.TempDir(), Limit:8}).Run(context.Background(), CommandInvocation{Executable:""}); if result.Started || result.Exit != -1 || result.Transport == nil { t.Fatal(result) }
}
func TestOwnedArgumentFile(t *testing.T) {
    root := t.TempDir(); path, err := WriteOwnedArgumentFile(root, "args.json", []byte(`{"target":"staging"}`), 64); if err != nil { t.Fatal(err) }; if !strings.HasPrefix(path, root) { t.Fatal(path) }
    if _, err := WriteOwnedArgumentFile(root, filepath.Join("..","escape"), []byte("x"), 64); !errors.Is(err, core.ErrPath) { t.Fatal(err) }
}
func TestNativeRunnerBoundsOutput(t *testing.T) {
    result := (NativeRunner{Root:t.TempDir(), Limit:8}).Run(context.Background(), CommandInvocation{Executable:os.Args[0], Args:[]string{"-test.run=TestRunnerChild"}, Env:append(os.Environ(), "AGENT_TEAM_RUNNER_CHILD=1"), OutputLimit:8})
    if len(result.Stdout) > 8 || !result.OutputTruncated { t.Fatal(result) }
}
func TestNativeRunnerDrainsNoisyChild(t *testing.T) { result := (NativeRunner{Root:t.TempDir(),Limit:8}).Run(context.Background(),CommandInvocation{Executable:os.Args[0],Args:[]string{"-test.run=TestRunnerChild"},Env:append(os.Environ(),"AGENT_TEAM_RUNNER_CHILD=1"),OutputLimit:8}); if result.Exit!=0||len(result.Stderr)>8{t.Fatal(result)} }
```

- [ ] **Step 2: Run failures.** Run `cd vnext && go test ./internal/deploy -run 'TestCommandValidationAndStartFailure|TestOwnedArgumentFile|TestNativeRunnerBoundsOutput|TestNativeRunnerDrainsNoisyChild' -count=1`. Expected: FAIL because safe runner symbols are absent.
- [ ] **Step 3: Implement safe execution.** Reject shell executables by basename including `.exe`, shell switches (`-c`, `/c`, `--command`, `-Command`), interpolation (`$(`, `${`, backticks, `%NAME%`), NUL, and traversal. Semicolons remain one literal argv element because no shell parses `Args`. Write argument files only below `OwnedRoot` with bounded atomic replacement. Use `StdoutPipe`/`StderrPipe`, concurrent `io.Copy` through `io.LimitReader(limit+1)`, `Wait`, and bounded buffers; never buffer unbounded output. Set `cmd.Env` from a copied invocation list only; never call `os.Setenv`. Check `Start` before `ProcessState`, classify deadline cancellation, and return `Started:false, Exit:-1, Transport:err` on start failure.

```go
func ValidateCommandTemplate(c []string) error { if len(c)==0||c[0]==""{return core.ErrSettings};base:=strings.ToLower(filepath.Base(c[0]));base=strings.TrimSuffix(base,".exe");for _,name:=range []string{"sh","bash","zsh","fish","dash","cmd","powershell","pwsh","busybox"}{if base==name{return core.ErrSettings}};for _,a:=range c[1:]{if strings.ContainsRune(a,0)||strings.Contains(a,"$(")||strings.Contains(a,"${")||strings.Contains(a,"`")||strings.Contains(a,"%")||a=="-c"||a=="/c"||a=="--command"||a=="-Command"||strings.Contains(a,"..") {return core.ErrSettings}};return nil }
type boundedWriter struct { dst *bytes.Buffer; limit int64; n int64; truncated *bool }
func(w *boundedWriter)Write(p []byte)(int,error){w.n+=int64(len(p));if w.n-int64(len(p))<w.limit{keep:=int64(len(p));if keep>w.limit-(w.n-int64(len(p))){keep=w.limit-(w.n-int64(len(p)))};_,_=w.dst.Write(p[:keep])};if w.n>w.limit{*w.truncated=true};return len(p),nil}
func (r NativeRunner) Run(ctx context.Context,in CommandInvocation) CommandResult { if err:=ValidateCommandTemplate(append([]string{in.Executable},in.Args...));err!=nil{return CommandResult{Exit:-1,Transport:err}};cmd:=exec.CommandContext(ctx,in.Executable,in.Args...);cmd.Env=append([]string(nil),in.Env...);out,errOut:=bytes.NewBuffer(nil),bytes.NewBuffer(nil);stdoutErr:=make(chan error,2);stdout,err:=cmd.StdoutPipe();if err!=nil{return CommandResult{Exit:-1,Transport:err}};stderr,err:=cmd.StderrPipe();if err!=nil{return CommandResult{Exit:-1,Transport:err}};if err:=cmd.Start();err!=nil{return CommandResult{Exit:-1,Transport:err}};limit:=in.OutputLimit;if limit<=0||limit>r.Limit{limit=r.Limit};outTruncated,errTruncated:=false,false;go func(){_,e:=io.Copy(&boundedWriter{dst:out,limit:limit,truncated:&outTruncated},stdout);stdoutErr<-e}();go func(){_,e:=io.Copy(&boundedWriter{dst:errOut,limit:limit,truncated:&errTruncated},stderr);stdoutErr<-e}();waitErr:=cmd.Wait();<-stdoutErr;<-stdoutErr;result:=CommandResult{Stdout:out.Bytes(),Stderr:errOut.Bytes(),Started:true,OutputTruncated:outTruncated||errTruncated,Exit:0};if cmd.ProcessState!=nil{result.Exit=cmd.ProcessState.ExitCode()};if waitErr!=nil&&result.Exit==0{result.Transport=waitErr};if ctx.Err()==context.DeadlineExceeded{result.TimedOut=true};return result }
```
- [ ] **Step 4: Implement `CommandProvider`.** Build argv from the selected profile command plus the batch idempotency key and owned argument-file path; call `Runner.Run` for Submit, Query, and Verify. Return a contracts operation with provider ID/idempotency key, or a durable unknown/error result when transport/start fails. Do not invoke a shell. The production snippet must be executable (not pseudocode):

```go
package deploy
import ("context"; "crypto/sha256"; "encoding/hex"; "errors"; "os"; "strings"; "github.com/thebpandey/agent-team/vnext/internal/contracts"; "github.com/thebpandey/agent-team/vnext/internal/core")
func (p CommandProvider) invoke(ctx context.Context, command []string, batchID, key string) (CommandResult,error) { if err:=ValidateCommandTemplate(command);err!=nil{return CommandResult{},err};root,err:=os.MkdirTemp(p.Root,"deploy-");if err!=nil{return CommandResult{},err};defer os.RemoveAll(root);arg,err:=WriteOwnedArgumentFile(root,"request.json",[]byte(key),p.Limit);if err!=nil{return CommandResult{},err};r:=p.Runner.Run(ctx,CommandInvocation{Executable:command[0],Args:append(append([]string{},command[1:]...),"--batch-id",batchID,"--argument-file",arg),OwnedRoot:root,ArgumentFile:arg,OutputLimit:p.Limit});if !r.Started||r.Transport!=nil{return r,ErrProvider};if r.OutputTruncated{return r,ErrOutputLimit};return r,nil }
func (p CommandProvider) Submit(ctx context.Context,b contracts.DeploymentBatch,key string)(contracts.Operation,error){batchID:=b.IdempotencyKey;if batchID==""{return contracts.Operation{Provider:"native",State:"unknown",Unknown:true,IdempotencyKey:key},core.ErrBatch};r,err:=p.invoke(ctx,p.Profile.ExecutorCommand,batchID,key);if err!=nil||r.Exit!=0{if err==nil{err=ErrProvider};return contracts.Operation{Provider:"native",State:"unknown",Unknown:true,IdempotencyKey:key},err};sum:=sha256.Sum256(r.Stdout);return contracts.Operation{Provider:"native",ProviderID:hex.EncodeToString(sum[:]),State:"submitted",IdempotencyKey:key},nil}
func (p CommandProvider) Query(ctx context.Context,op contracts.Operation,batchID string)(contracts.Operation,error){r,err:=p.invoke(ctx,p.Profile.QueryCommand,batchID,op.IdempotencyKey);if err!=nil||r.Exit!=0{op.State="unknown";op.Unknown=true;if err==nil{err=ErrProvider};return op,err};op.State="succeeded";op.ExternalState=string(r.Stdout);return op,nil}
func (p CommandProvider) Verify(ctx context.Context,op contracts.Operation,batchID string)(contracts.Verification,error){r,err:=p.invoke(ctx,p.Profile.VerificationCommand,batchID,op.IdempotencyKey);v:=contracts.Verification{State:"succeeded",OutputPointer:"native:verify/"+batchID,InputFingerprint:op.IdempotencyKey,Exit:r.Exit};if err!=nil||r.Exit!=0||strings.TrimSpace(string(r.Stdout))=="failed"{v.State="failed";if err==nil{err=ErrProvider};return v,err};return v,nil}
type outputLimitedRunner struct{}
func(outputLimitedRunner)Run(context.Context,CommandInvocation)CommandResult{return CommandResult{Started:true,OutputTruncated:true}}
func TestCommandProviderOutputLimitIsUnknown(t *testing.T){p:=CommandProvider{Profile:testProfile(),Runner:outputLimitedRunner{},Root:t.TempDir(),Limit:8};op,err:=p.Submit(context.Background(),contracts.DeploymentBatch{Run:"R-1",IdempotencyKey:"batch-1"},"key-1");if !errors.Is(err,ErrOutputLimit)||!op.Unknown||op.State!="unknown"{t.Fatal(op,err)};q,err:=p.Query(context.Background(),contracts.Operation{ProviderID:"op",IdempotencyKey:"key-1"},"batch-1");if !errors.Is(err,ErrOutputLimit)||!q.Unknown{t.Fatal(q,err)};v,err:=p.Verify(context.Background(),contracts.Operation{ProviderID:"op",IdempotencyKey:"key-1"},"batch-1");if !errors.Is(err,ErrOutputLimit)||v.State!="failed"{t.Fatal(v,err)}}
```
The RED test must assert the runner receives the expected argv including the exact `contracts.DeploymentBatch.IdempotencyKey` as the stable batch ID, an argument file below `OwnedRoot`, output truncation is bounded, shell commands are rejected, nonzero query exits return `Unknown:true` and do not reach verify, and start/transport failure returns `Unknown:true` without a second submit.
- [ ] **Step 5: Run and commit.** Run `cd vnext && gofmt -w internal/deploy/runner.go internal/deploy/runner_test.go && go test ./internal/deploy -run 'TestCommandValidationAndStartFailure|TestOwnedArgumentFile|TestNativeRunnerBoundsOutput|TestNativeRunnerDrainsNoisyChild|TestCommandProviderOutputLimitIsUnknown' -count=1 -race`. Expected: PASS. Commit `feat(deploy): add bounded native command provider`.

### Task 4: Persist operations, evidence, receipts, and query-before-retry

**Files:**

- Create: `vnext/internal/deploy/service.go`
- Test: `vnext/internal/deploy/service_test.go`

**Interfaces:** `NewMemoryRepository() *MemoryRepository` implements `Repository`; `NewBoundExecutor(TargetProfile,ProviderAdapter) (*BoundExecutor,error)` validates the profile; `SubmitOrReconcile(context.Context,Repository,*BoundExecutor,BatchManifest) (DeployOutcome,error)` and `ResumeBatch(context.Context,Repository,*BoundExecutor,string) (DeployOutcome,error)` are the only service entrypoints.

- [ ] **Step 1: Write a complete fake-provider RED test.** The test defines a mutex-protected provider with `calls []string`, `unknown bool`, and `verifyState string`, then asserts provider ID/state persistence, one submit, query-before-verify, no second submit, and failed-verification evidence:

```go
package deploy

import (
    "context"
    "errors"
    "reflect"
    "sync"
    "testing"
    "github.com/thebpandey/agent-team/vnext/internal/contracts"
    "github.com/thebpandey/agent-team/vnext/internal/core"
)

type fakeProvider struct { mu sync.Mutex; calls []string; unknown, queryErr, verifyNilError bool; verifyState string }
func (p *fakeProvider) Submit(context.Context, contracts.DeploymentBatch, string) (contracts.Operation,error) { p.mu.Lock(); defer p.mu.Unlock(); p.calls=append(p.calls,"submit"); if p.unknown { return contracts.Operation{Provider:"fake",ProviderID:"op-1",State:"unknown",Unknown:true}, errors.New("transport") }; return contracts.Operation{Provider:"fake",ProviderID:"op-1",State:"submitted",IdempotencyKey:"key"},nil }
func (p *fakeProvider) Query(context.Context, contracts.Operation, string) (contracts.Operation,error) { p.mu.Lock(); defer p.mu.Unlock(); p.calls=append(p.calls,"query"); if p.queryErr{return contracts.Operation{Provider:"fake",ProviderID:"op-1",State:"unknown",Unknown:true,IdempotencyKey:"key"},errors.New("query unavailable")}; return contracts.Operation{Provider:"fake",ProviderID:"op-1",State:"succeeded",IdempotencyKey:"key"},nil }
func (p *fakeProvider) Verify(context.Context, contracts.Operation, string) (contracts.Verification,error) { p.mu.Lock(); defer p.mu.Unlock(); p.calls=append(p.calls,"verify"); v:=contracts.Verification{State:p.verifyState,OutputPointer:"memory:verify"}; if p.verifyState=="failed"&&!p.verifyNilError { return v, errors.New("verification failed") }; return v,nil }
func testProfile() TargetProfile { p:=TargetProfile{ID:"staging",Target:"staging",AuthorizationRef:"approval-1",ApprovalScope:"staging",ExecutorCommand:[]string{"provider"},QueryCommand:[]string{"provider-query"},VerificationCommand:[]string{"provider-verify"},DefaultBatchSize:1,Enabled:true}; p.ProfileDigest,_=DeriveProfileDigest(p); return p }
func testBatch(id string) BatchManifest { return BatchManifest{RunID:"R-1",BatchID:id,Target:"staging",ProfileDigest:"sha256:profile",Fingerprint:"sha256:"+id,IdempotencyKey:"deploy-v1:"+id,TaskIDs:[]core.TaskID{"T-1"},Revisions:[]string{"rev-1"},ArtifactDigests:[][]string{{"artifact-1"}}} }

func TestUnknownResumeAndFailedVerificationAreDurable(t *testing.T) {
    provider := &fakeProvider{unknown:true, verifyState:"succeeded"}; repo := NewMemoryRepository(); executor, err := NewBoundExecutor(testProfile(), provider); if err != nil { t.Fatal(err) }; batch := testBatch("B-1")
    first, err := SubmitOrReconcile(context.Background(), repo, executor, batch); if err == nil || first.Receipt.State != "unknown" || !reflect.DeepEqual(provider.calls, []string{"submit"}) { t.Fatal(first, err, provider.calls) }
    op, err := repo.ReadOperation(context.Background(), batch.BatchID); if err != nil || op.Operation.ProviderID != "op-1" || op.State != "unknown" { t.Fatal(op, err) }
    provider.unknown = false; resumed, err := ResumeBatch(context.Background(), repo, executor, batch.BatchID); if err != nil || resumed.Receipt.State != "succeeded" || !reflect.DeepEqual(provider.calls, []string{"submit","query","verify"}) { t.Fatal(resumed, err, provider.calls) }
    provider.verifyState = "failed"; provider.verifyNilError = true; failedBatch := testBatch("B-2"); if _, err := SubmitOrReconcile(context.Background(), repo, executor, failedBatch); err != nil { t.Fatal(err) }; failed, err := ResumeBatch(context.Background(), repo, executor, failedBatch.BatchID); if err != nil || failed.Receipt.State != "failed" { t.Fatal(failed, err) }; if len(repo.Evidence()) < 3 { t.Fatal("failed verification evidence missing") }
    queryFail := &fakeProvider{queryErr:true, verifyState:"succeeded"}; qrepo := NewMemoryRepository(); qexec, _ := NewBoundExecutor(testProfile(), queryFail); qb := testBatch("B-query"); _, _ = SubmitOrReconcile(context.Background(), qrepo, qexec, qb); qout, qerr := ResumeBatch(context.Background(), qrepo, qexec, qb.BatchID); if qerr == nil || qout.Receipt.State != "unknown" { t.Fatal(qout, qerr) }; persisted, _ := qrepo.ReadReceipt(context.Background(), qb.BatchID); if persisted.State != "unknown" || len(persisted.EvidencePointers) != 1 { t.Fatal(persisted) }
}
```

- [ ] **Step 2: Run failure.** Run `cd vnext && go test ./internal/deploy -run TestUnknownResumeAndFailedVerificationAreDurable -count=1`. Expected: FAIL because the durable service is absent.
- [ ] **Step 3: Implement `MemoryRepository` and production adapter.** Use Phase 1 `store.Store` JSON paths for production and a mutex-backed equivalent for tests. CAS operation/receipt revisions exactly; return local `ErrNotFound` for absent IDs. Every operation write includes provider ID, external state, fingerprint, target, profile, attempt, and state. Every evidence/receipt write checks and returns its error. The test repository must expose this complete behavior (the JSON adapter uses the same expected-revision checks):

```go
package deploy
import ("context"; "fmt"; "sync"; "github.com/thebpandey/agent-team/vnext/internal/core")
type MemoryRepository struct { mu sync.Mutex; operations map[string]OperationRecord; receipts map[string]DeploymentReceipt; evidence []DeploymentEvidence }
func NewMemoryRepository()*MemoryRepository{return &MemoryRepository{operations:map[string]OperationRecord{},receipts:map[string]DeploymentReceipt{}}}
func(r *MemoryRepository)ReadOperation(_ context.Context,id string)(OperationRecord,error){r.mu.Lock();defer r.mu.Unlock();v,ok:=r.operations[id];if !ok{return OperationRecord{},ErrNotFound};return v,nil}
func(r *MemoryRepository)CompareAndSwapOperation(_ context.Context,expected uint64,v OperationRecord)(OperationRecord,error){r.mu.Lock();defer r.mu.Unlock();old:=r.operations[v.BatchID];if old.Revision!=expected{return OperationRecord{},core.ErrRevision};v.Revision=expected+1;r.operations[v.BatchID]=v;return v,nil}
func(r *MemoryRepository)WriteEvidence(_ context.Context,v DeploymentEvidence)(string,error){r.mu.Lock();defer r.mu.Unlock();v.Revision=uint64(len(r.evidence)+1);r.evidence=append(r.evidence,v);return fmt.Sprintf("memory:evidence/%d",v.Revision),nil}
func(r *MemoryRepository)ReadReceipt(_ context.Context,id string)(DeploymentReceipt,error){r.mu.Lock();defer r.mu.Unlock();v,ok:=r.receipts[id];if !ok{return DeploymentReceipt{},ErrNotFound};return v,nil}
func(r *MemoryRepository)CompareAndSwapReceipt(_ context.Context,expected uint64,v DeploymentReceipt)(DeploymentReceipt,error){r.mu.Lock();defer r.mu.Unlock();old:=r.receipts[v.BatchID];if old.Revision!=expected{return DeploymentReceipt{},core.ErrRevision};v.Revision=expected+1;r.receipts[v.BatchID]=v;return v,nil}
func(r *MemoryRepository)Evidence()[]DeploymentEvidence{r.mu.Lock();defer r.mu.Unlock();return append([]DeploymentEvidence(nil),r.evidence...)}
```
The production profile implementation is not a stub: `NewProfileRepository(s *store.Store)` returns `FileProfileRepository{s}`, `Get` calls `s.ReadJSON("deploy/profiles/"+id+".json", limits.RecordMaxBytes, &p)`, and `ApplyDecision` reads the prior `RecordEnvelope.Revision`, compares it to `expected`, derives/validates the complete profile, sets `Revision=expected+1`, and calls `s.WriteJSON` once. A stale expected revision returns `core.ErrRevision`; a byte-for-byte identical decision returns `ProfileDuplicate` without writing. Add a test using a real temp `store.Store` for create, duplicate, stale CAS, and restart/read-back.

```go
import ("context"; "errors"; "io/fs"; "sync"; "github.com/thebpandey/agent-team/vnext/internal/core"; "github.com/thebpandey/agent-team/vnext/internal/store")
type FileProfileRepository struct { mu sync.Mutex; s *store.Store }
func NewProfileRepository(s *store.Store) ProfileRepository { return &FileProfileRepository{s:s} }
func (r *FileProfileRepository) Get(_ context.Context,id ProfileID)(TargetProfile,error){var p TargetProfile;if err:=r.s.ReadJSON("deploy/profiles/"+string(id)+".json",64<<10,&p);err!=nil{return p,err};return p,nil}
func (r *FileProfileRepository) ApplyDecision(ctx context.Context,id ProfileID,d ProfileDecision,expected uint64)(ProfileWriteOutcome,error){r.mu.Lock();defer r.mu.Unlock();old,readErr:=r.Get(ctx,id);if readErr!=nil&&!errors.Is(readErr,ErrNotFound)&&!errors.Is(readErr,fs.ErrNotExist){return ProfileWriteOutcome{},readErr};if readErr==nil&&old.Revision!=expected{return ProfileWriteOutcome{},core.ErrRevision};p:=TargetProfile{ID:id,Target:d.Target,AuthorizationRef:d.AuthorizationRef,ApprovalScope:d.ApprovalScope,ExecutorCommand:d.ExecutorCommand,QueryCommand:d.QueryCommand,VerificationCommand:d.VerificationCommand,DefaultBatchSize:d.DefaultBatchSize,Enabled:d.Enable,Confirmed:d.Confirmed};var err error;p.ProfileDigest,err=DeriveProfileDigest(p);if err!=nil{return ProfileWriteOutcome{},err};if err=ValidateProfile(p);err!=nil{return ProfileWriteOutcome{},err};if readErr==nil&&old.ProfileDigest==p.ProfileDigest{return ProfileWriteOutcome{Kind:ProfileDuplicate,Profile:old,ObservedRevision:old.Revision,Idempotent:true},nil};p.Revision=expected+1;if _,err=r.s.WriteJSON("deploy/profiles/"+string(id)+".json",p,64<<10);err!=nil{return ProfileWriteOutcome{},err};kind:=ProfileCreated;if expected>0{kind=ProfileUpdated};return ProfileWriteOutcome{Kind:kind,Profile:p,ExpectedRevision:expected,ObservedRevision:p.Revision},nil}
```
- [ ] **Step 4: Implement `SubmitOrReconcile`.** Read operation first. If an existing fingerprint/idempotency key matches, return the durable receipt or call `ResumeBatch`; if it differs, return `core.ErrRevision`. For a new batch, CAS-write pending operation, call `Submit` once, CAS-update the operation with returned provider ID/state/unknown flag, write evidence, and CAS-write the receipt. A transport/start failure returns unknown while preserving the durable operation and evidence.
- [ ] **Step 5: Implement `ResumeBatch`.** Read operation and receipt, call `Query` before any `Verify` for pending/submitted/unknown/ambiguous state, never call `Submit`, verify only a successful queried operation, persist query/verification evidence and receipt state, and return all write errors. A failed verification is terminal failed evidence but does not stop coding.

```go
import ("context"; "errors"; "fmt"; "github.com/thebpandey/agent-team/vnext/internal/contracts"; "github.com/thebpandey/agent-team/vnext/internal/core")
func SubmitOrReconcile(ctx context.Context,r Repository,e *BoundExecutor,b BatchManifest)(DeployOutcome,error){existing,err:=r.ReadOperation(ctx,b.BatchID);if err==nil{if existing.Fingerprint!=b.Fingerprint{return DeployOutcome{},core.ErrRevision};return ResumeBatch(ctx,r,e,b.BatchID)};if !errors.Is(err,ErrNotFound){return DeployOutcome{},err};pending:=OperationRecord{RecordEnvelope:core.RecordEnvelope{Schema:1,RunID:b.RunID,Revision:0},BatchID:b.BatchID,Fingerprint:b.Fingerprint,ProfileID:string(e.Profile.ID),Target:b.Target,State:"pending",Attempt:1};if _,err=r.CompareAndSwapOperation(ctx,0,pending);err!=nil{return DeployOutcome{},err};op,submitErr:=e.Provider.Submit(ctx,contracts.DeploymentBatch{Run:b.RunID,TaskIDs:b.TaskIDs,Revisions:b.Revisions,TargetProfile:string(e.Profile.ID),AuthorizationRef:e.Profile.AuthorizationRef,ExecutorCommand:e.Profile.ExecutorCommand,VerificationCommand:e.Profile.VerificationCommand,Fingerprint:b.Fingerprint,IdempotencyKey:b.IdempotencyKey},b.IdempotencyKey);state:="submitted";if submitErr!=nil||op.Unknown{state="unknown"};updated:=pending;updated.Revision=1;updated.Operation=op;updated.State=state;if _,err=r.CompareAndSwapOperation(ctx,1,updated);err!=nil{return DeployOutcome{},err};receipt:=DeploymentReceipt{RecordEnvelope:core.RecordEnvelope{Schema:1,RunID:b.RunID},RunID:b.RunID,BatchID:b.BatchID,ProviderID:op.ProviderID,ProfileID:string(e.Profile.ID),Target:b.Target,Fingerprint:b.Fingerprint,IdempotencyKey:b.IdempotencyKey,State:state,CodingMayContinue:true};ptr,err:=r.WriteEvidence(ctx,DeploymentEvidence{BatchID:b.BatchID,ProfileID:string(e.Profile.ID),Target:b.Target,Fingerprint:b.Fingerprint,State:state,Error:fmt.Sprint(submitErr)});if err!=nil{return DeployOutcome{Receipt:receipt,CodingMayContinue:true},err};receipt.EvidencePointers=[]string{ptr};if _,err=r.CompareAndSwapReceipt(ctx,0,receipt);err!=nil{return DeployOutcome{Receipt:receipt,CodingMayContinue:true},err};return DeployOutcome{Receipt:receipt,CodingMayContinue:true},submitErr}
func ResumeBatch(ctx context.Context,r Repository,e *BoundExecutor,id string)(DeployOutcome,error){op,err:=r.ReadOperation(ctx,id);if err!=nil{return DeployOutcome{},err};old,readErr:=r.ReadReceipt(ctx,id);if readErr!=nil&&!errors.Is(readErr,ErrNotFound){return DeployOutcome{},readErr};expected:=uint64(0);if readErr==nil{expected=old.Revision};queried,qerr:=e.Provider.Query(ctx,op.Operation,id);state:="unknown";if qerr==nil&&queried.State=="succeeded"{state="succeeded"};qptr,qwriteErr:=r.WriteEvidence(ctx,DeploymentEvidence{BatchID:op.BatchID,ProfileID:op.ProfileID,Target:op.Target,Fingerprint:op.Fingerprint,State:state,Error:fmt.Sprint(qerr),Attempt:op.Attempt});if qwriteErr!=nil{return DeployOutcome{},qwriteErr};op.Revision++;op.Operation=queried;op.State=state;if _,casErr:=r.CompareAndSwapOperation(ctx,op.Revision-1,op);casErr!=nil{return DeployOutcome{},casErr};receipt:=DeploymentReceipt{RecordEnvelope:core.RecordEnvelope{Schema:1,RunID:op.RunID},RunID:op.RunID,BatchID:op.BatchID,ProviderID:queried.ProviderID,ProfileID:op.ProfileID,Target:op.Target,Fingerprint:op.Fingerprint,State:state,EvidencePointers:[]string{qptr},CodingMayContinue:true};if qerr!=nil||state!="succeeded"{if _,casErr:=r.CompareAndSwapReceipt(ctx,expected,receipt);casErr!=nil{return DeployOutcome{Receipt:receipt,CodingMayContinue:true},casErr};return DeployOutcome{Receipt:receipt,CodingMayContinue:true},qerr};verification,verr:=e.Provider.Verify(ctx,queried,id);if verr!=nil||verification.State=="failed"{state="failed"};vptr,vwriteErr:=r.WriteEvidence(ctx,DeploymentEvidence{BatchID:op.BatchID,ProfileID:op.ProfileID,Target:op.Target,Fingerprint:op.Fingerprint,State:state,Error:fmt.Sprint(verr),OutputPointer:verification.OutputPointer,Attempt:op.Attempt});if vwriteErr!=nil{return DeployOutcome{},vwriteErr};receipt.State=state;receipt.Verification=verification;receipt.EvidencePointers=append(receipt.EvidencePointers,vptr);if _,casErr:=r.CompareAndSwapReceipt(ctx,expected,receipt);casErr!=nil{return DeployOutcome{Receipt:receipt,CodingMayContinue:true},casErr};return DeployOutcome{Receipt:receipt,CodingMayContinue:true},verr}
```
- [ ] **Step 6: Run and commit.** Run `cd vnext && gofmt -w internal/deploy/service.go internal/deploy/service_test.go && go test ./internal/deploy -run TestUnknownResumeAndFailedVerificationAreDurable -count=1 -race`. Expected: PASS. Commit `feat(deploy): persist operation recovery`.

### Task 5: Add blocker, hold, dashboard, and explicit rollback semantics

**Files:**

- Create: `vnext/internal/deploy/recovery.go`
- Test: `vnext/internal/deploy/recovery_test.go`

**Interfaces:** `RecordState(context.Context,DeploymentReceipt) (string,error)` creates one `knowledge.Blocker`, calls `HoldLater` for failed batches, sends a best-effort dashboard trigger without deleting receipts, and always reports coding may continue. `ValidateRollbackAuthorization(bool,string) error` is the only rollback decision gate; no rollback executor exists.

- [ ] **Step 1: Write complete recovery tests.** Define fake blocker/hold/dashboard writers in `recovery_test.go` and assert failure, empty evidence pointer, dashboard error, and authorization refusal:

```go
package deploy

import (
    "context"
    "errors"
    "testing"
    "github.com/thebpandey/agent-team/vnext/internal/core"
    "github.com/thebpandey/agent-team/vnext/internal/knowledge"
)

type fakeBlockers struct { items []knowledge.Blocker }
func (f *fakeBlockers) Create(_ context.Context, b knowledge.Blocker) (knowledge.Blocker,error) { b.ID="block-1"; f.items=append(f.items,b); return b,nil }
type fakeHolds struct { calls []string }
func (f *fakeHolds) HoldLater(_ context.Context, _ core.RunID, batch string) error { f.calls=append(f.calls,batch); return nil }
type fakeDashboard struct { err error }
func (f fakeDashboard) Trigger(context.Context, DashboardTriggerRequest) error { return f.err }

func TestRecoveryPersistsBlockerHoldAndSurvivesDashboardFailure(t *testing.T) {
    blockers, holds := &fakeBlockers{}, &fakeHolds{}; service := RecoveryService{Blockers:blockers, Holds:holds, Dashboard:fakeDashboard{err:errors.New("dashboard unavailable")}}
    id, err := service.RecordState(context.Background(), DeploymentReceipt{RunID:"R-1", BatchID:"B-1", State:"failed", CodingMayContinue:true}); if err != nil || id == "" || len(blockers.items)!=1 || len(holds.calls)!=1 || blockers.items[0].EvidencePointer != "" { t.Fatal(id,err,blockers,holds) }
    if err := ValidateRollbackAuthorization(false, ""); !errors.Is(err, core.ErrSettings) { t.Fatal(err) }
}
```

- [ ] **Step 2: Run failure.** Run `cd vnext && go test ./internal/deploy -run TestRecoveryPersistsBlockerHoldAndSurvivesDashboardFailure -count=1`. Expected: FAIL because recovery is absent.
- [ ] **Step 3: Implement recovery.** For `failed`, create a blocker with batch and evidence facts, call `HoldLater` and return its errors, then call dashboard best-effort while preserving the receipt. Do not panic when `EvidencePointers` is empty. For non-failed states return no blocker. Require both explicit confirmation and a nonempty authorization reference for any future rollback call.

```go
func (s RecoveryService) RecordState(ctx context.Context,r DeploymentReceipt)(string,error){if r.State!="failed"{return "",nil};b,err:=s.Blockers.Create(ctx,knowledge.Blocker{RecordEnvelope:core.RecordEnvelope{Schema:1,RunID:r.RunID},ID:"deployment-"+r.BatchID,Severity:"high",State:"open",Affected:[]string{r.BatchID},EvidencePointer:firstPointer(r.EvidencePointers)});if err!=nil{return "",err};if s.Holds!=nil{if err:=s.Holds.HoldLater(ctx,r.RunID,r.BatchID);err!=nil{return "",err}};if s.Dashboard!=nil{_ = s.Dashboard.Trigger(ctx,DashboardTriggerRequest{RunID:r.RunID,BatchID:r.BatchID,ReceiptPointer:firstPointer(r.EvidencePointers),Reason:"failed"})};return b.ID,nil}
func firstPointer(v []string)string{if len(v)==0{return ""};return v[0]}
func ValidateRollbackAuthorization(confirmed bool,ref string)error{if !confirmed||ref==""{return core.ErrSettings};return nil}
```
- [ ] **Step 4: Run and commit.** Run `cd vnext && gofmt -w internal/deploy/recovery.go internal/deploy/recovery_test.go && go test ./internal/deploy -run TestRecoveryPersistsBlockerHoldAndSurvivesDashboardFailure -count=1`. Expected: PASS. Commit `feat(deploy): hold failed batches and notify dashboard`.

### Task 6: Wire the canonical Phase 1 CLI and vertical acceptance

**Files:**

- Modify: `vnext/internal/core/types.go` by adding only `type DeploymentAction func(context.Context, []string, io.Writer, io.Writer) int` and `Deployment DeploymentAction` to existing `core.Dependencies`.
- Modify: `vnext/internal/cli/app.go` and `vnext/internal/cli/parser.go` to dispatch the canonical `deploy` lifecycle through the existing `cli.Run`; do not add another runner or a separate public `resume` action.
- Create: `vnext/internal/deploy/cli.go`
- Create: `vnext/internal/acceptance/deployment_test.go`

**Interfaces:** `ParseDeployArgs([]string) (DeployRequest,error)` accepts only the public `deploy` action. `--run` is optional when exactly one configured resumable run exists; omitted/ambiguous run selection is resolved by the canonical lifecycle resolver and otherwise returns `core.ErrSettings`. `--target`, `--profile`, and optional `--batch-size` use configured defaults, while `--resume` is a deploy lifecycle flag rather than a second action. Existing `cli.Run(context.Context, []string, core.Dependencies) int` invokes `deps.Deployment` and returns nonzero when the callback is nil.

The resolver is explicit and runs after parsing: `ResolveDeployRun(DeployRequest, []string) (DeployRequest,error)` fills `RunID` only when the candidate list has exactly one entry; zero or multiple candidates without `--run` return `core.ErrSettings`, and an explicitly supplied run not in the candidates returns `core.ErrSettings`. `ApplyDeployDefaults(DeployRequest,string,string) (DeployRequest,error)` fills omitted target/profile from configured defaults and rejects missing defaults or production. This keeps run selection deterministic without introducing a second public action.

- [ ] **Step 1: Write the canonical CLI RED test.** Call the exact Phase 1 runner and assert callback arguments plus rejection of legacy public actions:

```go
package acceptance

import (
    "bytes"
    "context"
    "errors"
    "fmt"
    "io"
    "reflect"
    "testing"
    "github.com/thebpandey/agent-team/vnext/internal/cli"
    "github.com/thebpandey/agent-team/vnext/internal/contracts"
    "github.com/thebpandey/agent-team/vnext/internal/core"
    "github.com/thebpandey/agent-team/vnext/internal/knowledge"
    deploy "github.com/thebpandey/agent-team/vnext/internal/deploy"
)

func TestCanonicalDeploymentCLI(t *testing.T) {
    calls := 0; var seen []string; deps := core.Dependencies{Stdout:&bytes.Buffer{}, Stderr:&bytes.Buffer{}, Deployment:func(_ context.Context,args []string,_,_ io.Writer) int { calls++; seen=append([]string(nil),args...); return 0 }}
    args := []string{"deploy","--run","R-1","--target","staging","--profile","staging","--batch-size","3"}; if code := cli.Run(context.Background(), args, deps); code != 0 || calls != 1 || !reflect.DeepEqual(seen,args) { t.Fatal(code,calls,seen) }
    if code := cli.Run(context.Background(), []string{"review"}, deps); code == 0 || calls != 1 { t.Fatal("legacy action accepted", code) }
    if _, err := deploy.ParseDeployArgs([]string{"deploy","--target","staging","--run","R-1","--profile","staging","--batch-size","0"}); !errors.Is(err, core.ErrBatch) { t.Fatal(err) }
    parsedFull, err := deploy.ParseDeployArgs([]string{"deploy","--run","R-1","--target","staging","--profile","staging","--batch-size","3"}); if err != nil || parsedFull.RunID != "R-1" || parsedFull.BatchSize != 3 { t.Fatal(parsedFull, err) }
    parsed, err := deploy.ParseDeployArgs([]string{"deploy","--resume"}); if err != nil { t.Fatal(err) }; if _, err := deploy.ResolveDeployRun(parsed, nil); !errors.Is(err, core.ErrSettings) { t.Fatal(err) }; if _, err := deploy.ResolveDeployRun(parsed, []string{"R-1","R-2"}); !errors.Is(err, core.ErrSettings) { t.Fatal(err) }
    one, err := deploy.ResolveDeployRun(parsed, []string{"R-1"}); if err != nil || one.RunID != "R-1" { t.Fatal(one, err) }; one, err = deploy.ApplyDeployDefaults(one, "staging", "staging"); if err != nil || one.Target != "staging" || one.ProfileID != "staging" { t.Fatal(one, err) }
    explicit := parsed; explicit.RunID="R-2"; if _, err := deploy.ResolveDeployRun(explicit, []string{"R-1"}); !errors.Is(err, core.ErrSettings) { t.Fatal(err) }
}
func TestDeploymentDefaultsAndRunResolution(t *testing.T){base,err:=deploy.ParseDeployArgs([]string{"deploy","--resume"});if err!=nil{t.Fatal(err)};if _,err=deploy.ResolveDeployRun(base,nil);!errors.Is(err,core.ErrSettings){t.Fatal(err)};if _,err=deploy.ResolveDeployRun(base,[]string{"R-1","R-2"});!errors.Is(err,core.ErrSettings){t.Fatal(err)};one,err:=deploy.ResolveDeployRun(base,[]string{"R-1"});if err!=nil||one.RunID!="R-1"{t.Fatal(one,err)};one,err=deploy.ApplyDeployDefaults(one,"staging","staging");if err!=nil||one.Target!="staging"||one.ProfileID!="staging"{t.Fatal(one,err)};if _,err=deploy.ApplyDeployDefaults(base,"production","staging");!errors.Is(err,core.ErrSettings){t.Fatal(err)};if _,err=deploy.ApplyDeployDefaults(base,"","staging");!errors.Is(err,core.ErrSettings){t.Fatal(err)};if _,err=deploy.ApplyDeployDefaults(base,"staging","");!errors.Is(err,core.ErrSettings){t.Fatal(err)}}
```

- [ ] **Step 2: Run failure.** Run `cd vnext && go test ./internal/acceptance -run TestCanonicalDeploymentCLI -count=1`. Expected: FAIL until the existing Phase 1 `cli.Run` dispatches the new callback and configured run/default resolvers are implemented.
- [ ] **Step 3: Implement argument parsing and callback wiring.** Preserve the existing Phase 1 action vocabulary and `cli.Run` signature. Reject `review`, `gate`, `integrate`, `reconcile`, shell-bearing values, duplicate flags, missing values, production targets, and batch sizes outside 1–100. Add no `CLI` struct, no second parser entrypoint, and no direct production authority. Run `go test ./internal/acceptance -run TestCanonicalDeploymentCLI -count=1` after implementing `ResolveDeployRun` and `ApplyDeployDefaults`; the test must cover zero, one, multiple, explicit, omitted-target, omitted-profile, configured-default, and production-default cases.

```go
type DeployRequest struct { Action string; RunID, Target, ProfileID string; BatchSize int; Resume bool }
func ParseDeployArgs(a []string)(DeployRequest,error){if len(a)==0||a[0]!="deploy"{return DeployRequest{},ErrAction};r:=DeployRequest{Action:"deploy"};seen:=map[string]bool{};for i:=1;i<len(a);{if a[i]=="--resume"{if seen[a[i]]{return DeployRequest{},core.ErrSettings};seen[a[i]]=true;r.Resume=true;i++;continue};if i+1>=len(a)||seen[a[i]]||strings.ContainsAny(a[i+1],";&|$`"){return DeployRequest{},core.ErrSettings};seen[a[i]]=true;switch a[i]{case "--run":r.RunID=a[i+1];case "--target":r.Target=a[i+1];case "--profile":r.ProfileID=a[i+1];case "--batch-size":n,err:=strconv.Atoi(a[i+1]);if err!=nil||n<1||n>100{return DeployRequest{},core.ErrBatch};r.BatchSize=n;default:return DeployRequest{},ErrAction};i+=2};if r.Target!=""&&strings.EqualFold(r.Target,"production"){return DeployRequest{},core.ErrSettings};return r,nil}
func ResolveDeployRun(r DeployRequest,candidates []string)(DeployRequest,error){if r.RunID!=""{for _,id:=range candidates{if id==r.RunID{return r,nil}};return DeployRequest{},core.ErrSettings};if len(candidates)!=1{return DeployRequest{},core.ErrSettings};r.RunID=candidates[0];return r,nil}
func ApplyDeployDefaults(r DeployRequest,target,profile string)(DeployRequest,error){if r.Target==""{r.Target=target};if r.ProfileID==""{r.ProfileID=profile};if r.Target==""||r.ProfileID==""||strings.EqualFold(r.Target,"production"){return DeployRequest{},core.ErrSettings};return r,nil}
```
- [ ] **Step 4: Add the vertical fake-provider test.** Append this complete test and fixture to `vnext/internal/acceptance/deployment_test.go`:

```go
type verticalProvider struct { calls []string; unknown bool }
func (p *verticalProvider) Submit(context.Context, contracts.DeploymentBatch, string) (contracts.Operation,error) { p.calls=append(p.calls,"submit"); if p.unknown { return contracts.Operation{Provider:"fake",ProviderID:"op-vertical",State:"unknown",Unknown:true}, errors.New("transport") }; return contracts.Operation{Provider:"fake",ProviderID:"op-vertical",State:"submitted"},nil }
func (p *verticalProvider) Query(context.Context, contracts.Operation, string) (contracts.Operation,error) { p.calls=append(p.calls,"query"); return contracts.Operation{Provider:"fake",ProviderID:"op-vertical",State:"succeeded"},nil }
func (p *verticalProvider) Verify(context.Context, contracts.Operation, string) (contracts.Verification,error) { p.calls=append(p.calls,"verify"); return contracts.Verification{State:"failed",OutputPointer:"memory:vertical"},errors.New("verification failed") }
type verticalBlockers struct { items []knowledge.Blocker }
func (b *verticalBlockers) Create(_ context.Context, v knowledge.Blocker) (knowledge.Blocker,error) { v.ID="vertical-blocker"; b.items=append(b.items,v); return v,nil }
type verticalHolds struct { calls []string }
func (h *verticalHolds) HoldLater(_ context.Context, _ core.RunID, batch string) error { h.calls=append(h.calls,batch); return nil }
type verticalDashboard struct{}
func (verticalDashboard) Trigger(context.Context, deploy.DashboardTriggerRequest) error { return errors.New("dashboard unavailable") }

func TestDeploymentVertical(t *testing.T) {
    profile := deploy.TargetProfile{ID:"staging",Target:"staging",AuthorizationRef:"approval-1",ApprovalScope:"staging",ExecutorCommand:[]string{"provider"},QueryCommand:[]string{"provider-query"},VerificationCommand:[]string{"provider-verify"},DefaultBatchSize:3,Enabled:true}; profile.ProfileDigest,_=deploy.DeriveProfileDigest(profile)
    tasks:=make([]deploy.EligibleTask,7); for i:=range tasks { tasks[i]=deploy.EligibleTask{ID:core.TaskID(fmt.Sprintf("T-%d",i)),CanonicalRevision:fmt.Sprintf("rev-%d",i),ArtifactDigests:[]string{fmt.Sprintf("artifact-%d",i)},Integrated:true,GatePassed:true,DependenciesComplete:true} }
    batches,err:=deploy.PlanBatches(context.Background(),deploy.BatchRequest{RunID:"R-vertical",Target:"staging",Profile:profile,Tasks:tasks,BatchSize:3}); if err!=nil||len(batches)!=3||len(batches[0].TaskIDs)!=3||len(batches[1].TaskIDs)!=3||len(batches[2].TaskIDs)!=1 { t.Fatal(len(batches),err) }
    provider:=&verticalProvider{unknown:true}; repo:=deploy.NewMemoryRepository(); executor,err:=deploy.NewBoundExecutor(profile,provider); if err!=nil { t.Fatal(err) }; first,err:=deploy.SubmitOrReconcile(context.Background(),repo,executor,batches[0]); if err==nil||first.Receipt.State!="unknown"||!first.CodingMayContinue { t.Fatal(first,err) }; resumed,err:=deploy.ResumeBatch(context.Background(),repo,executor,batches[0].BatchID); if err==nil||resumed.Receipt.State!="failed"||!resumed.CodingMayContinue||!reflect.DeepEqual(provider.calls,[]string{"submit","query","verify"}) { t.Fatal(resumed,err,provider.calls) }
    blockers,holds:=&verticalBlockers{},&verticalHolds{}; id,err:= (deploy.RecoveryService{Blockers:blockers,Holds:holds,Dashboard:verticalDashboard{}}).RecordState(context.Background(),resumed.Receipt); if err!=nil||id==""||len(blockers.items)!=1||len(holds.calls)!=1||len(repo.Evidence())<2 { t.Fatal(id,err,blockers,holds) }
}
```
- [ ] **Step 5: Run and commit.** Run `cd vnext && gofmt -w internal/core/types.go internal/cli/app.go internal/cli/parser.go internal/deploy/cli.go internal/acceptance/deployment_test.go && go test ./internal/acceptance -run 'TestCanonicalDeploymentCLI|TestDeploymentDefaultsAndRunResolution|TestDeploymentVertical' -count=1`. Expected: PASS. Commit `feat(deploy): wire guarded canonical deployment action`.

### Task 7: Native CI and final acceptance gate

**Files:**

- Create: `.github/workflows/vnext-deployment.yml`
- Test: `vnext/internal/deploy/acceptance_gate_test.go`

**Interfaces:** CI runs the exact package tests and native matrix; no live provider is contacted. The final gate consumes only fake-provider evidence and explicitly authorized staging fixtures.

- [ ] **Step 1: Write the final gate test.** Add `vnext/internal/deploy/acceptance_gate_test.go`:

```go
package deploy

import (
    "context"
    "fmt"
    "testing"
    "github.com/thebpandey/agent-team/vnext/internal/core"
)

func TestDeploymentGate(t *testing.T) {
    p:=testProfile(); tasks:=make([]EligibleTask,7); for i:=range tasks { tasks[i]=EligibleTask{ID:core.TaskID(fmt.Sprintf("T-%d",i)),CanonicalRevision:fmt.Sprintf("rev-%d",i),ArtifactDigests:[]string{fmt.Sprintf("artifact-%d",i)},Integrated:true,GatePassed:true,DependenciesComplete:true} }
    batches,err:=PlanBatches(context.Background(),BatchRequest{RunID:"R-gate",Target:"staging",Profile:p,Tasks:tasks,BatchSize:3}); if err!=nil||len(batches)!=3||len(batches[0].TaskIDs)!=3||len(batches[1].TaskIDs)!=3||len(batches[2].TaskIDs)!=1 { t.Fatal(len(batches),err) }
    for _,b:=range batches { if b.Fingerprint==""||b.IdempotencyKey==""||b.ProfileDigest==""||b.Target!="staging" { t.Fatal(b) } }
    failed:=(NativeRunner{Root:t.TempDir(),Limit:8}).Run(context.Background(),CommandInvocation{Executable:""}); if failed.Started||failed.Transport==nil||failed.Exit!=-1 { t.Fatal(failed) }
}
```
- [ ] **Step 2: Run the failing gate.** Run `cd vnext && go test ./internal/deploy ./internal/acceptance -run 'TestDeploymentGate|TestDeploymentVertical' -count=1`. Expected: FAIL until all prior tasks are integrated.
- [ ] **Step 3: Add complete CI.** Create:

```yaml
name: vnext-deployment
on: [push, pull_request]
jobs:
  native:
    strategy:
      fail-fast: false
      matrix: { os: [ubuntu-latest, macos-latest, windows-latest] }
    runs-on: ${{ matrix.os }}
    defaults: { run: { working-directory: vnext } }
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with: { go-version: '1.23' }
      - run: go test ./... -count=1
      - run: go test -race ./internal/deploy ./internal/acceptance -count=1
      - run: go vet ./...
      - run: git diff --check
```

- [ ] **Step 4: Run the release gate.** Run `cd vnext && gofmt -w internal/deploy/*.go internal/acceptance/deployment_test.go && go test ./... -count=1 && go test -race ./internal/deploy ./internal/acceptance -count=1 && go vet ./...`; expected PASS on Linux, macOS, and Windows. Run `rg -n 'os\.Setenv|exec\.Command\(.*-c|powershell|cmd\.exe|/bin/sh' vnext/internal/deploy vnext/internal/acceptance`; expected only rejection-test literals and no execution path.
- [ ] **Step 5: Commit.** Run `git add vnext/internal/deploy vnext/internal/core/types.go vnext/internal/cli/app.go vnext/internal/cli/parser.go vnext/internal/acceptance/deployment_test.go .github/workflows/vnext-deployment.yml && git commit -m "feat(deploy): complete authorized deployment recovery gate"`.

## Acceptance

All seven tasks are independently reviewable and CLEAN; profile authorization is revision-bound and idempotent; seven eligible tasks at size three become `3,3,1`; artifact changes alter fingerprints; command execution is argv-only and bounded; operation provider ID/state is durable; unknown/ambiguous recovery performs query then verify with zero duplicate submits; failed verification persists receipt/evidence, creates a blocker, holds later batches, and leaves coding runnable; dashboard failure preserves receipts; rollback requires explicit authorization; CI passes native Linux/macOS/Windows tests; no production target or host mutation is invoked.

Execute only after Phase 1 contracts compile and each task has an independent review/CLEAN gate.
