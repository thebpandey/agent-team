# Agent-Team vNext Host Execution Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add foreground Codex/Claude execution, isolated worktrees, independent FIX/CLEAN review, deterministic gates, serial integration, scoped recovery, and safe cleanup on top of Phase 1.

**Architecture:** Phase 2 consumes the exported Phase 1 `core`, `store`, `tracker`, `run`, `knowledge`, `workflow`, `resources`, and `contracts` packages. A foreground orchestrator dispatches immutable packets to task worktrees, supervises one turn at a time, routes each candidate to a non-author reviewer, gates revision-bound CLEAN evidence, and integrates serially. Host switching resumes from durable records; no hooks, leases, daemons, background workers, PID authority, MCP registration, or product-code edits in the main checkout are allowed.

**Tech Stack:** Existing Go module `github.com/thebpandey/agent-team/vnext`, Go standard library, `os/exec` argument arrays, Git, UTF-8 JSON/Markdown, `go test`, and `go vet`.

**Spec:** `docs/superpowers/specs/2026-09-18-agent-team-vnext-design.md`

**Prerequisite:** Phase 1 plan `docs/superpowers/plans/2026-09-18-agent-team-vnext-native-core.md` is complete and its packages compile.

## Global Constraints

- Task 1 imports exact Phase 1 packet/handle/observation/identity/worktree/adapter types. Do not create a second `contracts` package or duplicate Phase 1 types; host discovery capabilities are Phase 2's local `host.Capabilities` type.
- Main checkout is control-only. Developers edit only assigned worktrees; the orchestrator never authors product code.
- Codex and Claude are explicit harness labels. A reviewer must have a distinct non-author identity.
- Reviewer verdicts are exactly `FIX` or `CLEAN`; `blocked` is task/run state only.
- Planned mode uses configured/effective capacity. One-off mode admits at most two teams and only when Phase 1 path/resource conflict checks prove disjoint; otherwise serialize.
- Every event, command, review, gate, resource, and cleanup result is bounded, revision-bound, and durable. Unknown results preserve evidence and block destructive cleanup.
- Public actions are only `setup`, `settings`, `start`, `status`, `task add`, `one-off`, `pause`, `stop`, `cancel`, `resume`, `inspect`, `cleanup`, and `deploy`; internal transitions are not public commands.
- Windows, macOS, and Linux are first-class. Use native process APIs and Git argument arrays; never use `/proc`, `flock`, bash, POSIX signals, or PID as ownership authority.

## File Map

| Path | Responsibility |
| --- | --- |
| `vnext/internal/host`, `command`, `model` | Host identity, bounded commands, capacity/model routing. |
| `vnext/internal/dispatch`, `worktree` | Immutable packets and task-owned worktrees. |
| `vnext/internal/supervise`, `review`, `gate` | Foreground events, FIX/CLEAN loop, deterministic checks. |
| `vnext/internal/integrate`, `lifecycle`, `orchestrator` | Serial integration, scoped control, continuous execution. |
| `vnext/internal/cleanup`, `handoff`, `acceptance` | Safe cleanup, handoff, vertical tests. |

### Task 1: Import exact Phase 1 contracts and establish host boundaries

**Files:** Create `vnext/internal/host/{capabilities.go,adapter.go,host_test.go}`, `vnext/internal/command/{runner.go,runner_test.go}`, and `vnext/internal/model/model.go`.

**Interfaces — Consumes:** exact Phase 1 `core.AssignmentPacket`, `contracts.WorkerRequest`, `contracts.WorkerHandle`, `contracts.HostCapabilities`, `contracts.WorktreeSpec`, and `contracts.HostAdapter` types. External owners are `internal/core`, `internal/contracts`, and `internal/tracker`; these imports are shown below.

**Interfaces — Produces:** `Harness`, `Adapter`, `CommandRunner`, `ModelRoute`, `NewCodex`, `NewClaude`, and `RouteModel` shown below; all later tasks consume these names.

```go
// vnext/internal/host/capabilities.go
package host
import (
  "context"
  "github.com/thebpandey/agent-team/vnext/internal/contracts"
  "github.com/thebpandey/agent-team/vnext/internal/tracker"
)
type Capabilities = contracts.HostCapabilities
type Adapter = contracts.HostAdapter
type CommandRunner interface { Run(context.Context,string,...string) tracker.CommandResult }

// vnext/internal/command/runner.go
package command
import ( "bytes"; "context"; "errors"; "os/exec"; "github.com/thebpandey/agent-team/vnext/internal/host"; "github.com/thebpandey/agent-team/vnext/internal/tracker" )
type Runner struct { Limit int }
var _ host.CommandRunner = Runner{}
type boundedWriter struct { limit int; buf bytes.Buffer }
func (w *boundedWriter) Write(p []byte)(int,error){ written:=len(p);remaining:=w.limit-w.buf.Len();if remaining>0{if len(p)>remaining{p=p[:remaining]};_,_=w.buf.Write(p)};return written,nil }
func (w *boundedWriter) Bytes() []byte{return w.buf.Bytes()}
func NewRunner(limit int) host.CommandRunner { if limit<1{limit=1<<20};return Runner{Limit:limit} }
func (r Runner) Run(ctx context.Context,name string,args ...string) tracker.CommandResult { if name=="" { return tracker.CommandResult{Exit:-1,Transport:errors.New("empty executable")} }; if r.Limit<1{r.Limit=1<<20};cmd:=exec.CommandContext(ctx,name,args...);out,errbuf:=&boundedWriter{limit:r.Limit},&boundedWriter{limit:r.Limit};cmd.Stdout=out;cmd.Stderr=errbuf;err:=cmd.Run();result:=tracker.CommandResult{Stdout:out.Bytes(),Stderr:errbuf.Bytes()};if errors.Is(ctx.Err(),context.DeadlineExceeded){result.TimedOut=true};if err==nil{return result};var exit *exec.ExitError;if errors.As(err,&exit){result.Exit=exit.ExitCode();return result};result.Exit=-1;result.Transport=err;return result }

// vnext/internal/host/adapter.go
package host
import ( "context"; "strings"; "github.com/thebpandey/agent-team/vnext/internal/contracts"; "github.com/thebpandey/agent-team/vnext/internal/core" )
type harnessAdapter struct { runner CommandRunner; executable, name string }
func newHarness(r CommandRunner, executable string) Adapter { return &harnessAdapter{runner:r,executable:executable,name:executable} }
func NewCodex(r CommandRunner) Adapter { return newHarness(r,"codex") }
func NewClaude(r CommandRunner) Adapter { return newHarness(r,"claude") }
func (a *harnessAdapter) Probe(ctx context.Context)(Capabilities,error) { if a==nil||a.runner==nil{return Capabilities{},core.ErrCapacity};r:=a.runner.Run(ctx,a.executable,"--version");if r.Transport!=nil||r.TimedOut||r.Exit!=0||strings.TrimSpace(string(r.Stdout))==""{return Capabilities{},core.ErrCapacity};return Capabilities{Host:a.name,ConfiguredSlots:1,ObservedSlots:1,UsableSlots:1,DeveloperSlots:1,ReviewerSlots:1,Models:[]string{"default"}},nil }
func (a *harnessAdapter) start(ctx context.Context,req contracts.WorkerRequest,reviewer bool)(contracts.WorkerHandle,error) { if a==nil||a.runner==nil||req.Packet.RunID==""||req.Packet.Team==""||req.Packet.Task==""||req.Packet.QueueFingerprint==""||req.Worktree.Root==""{return contracts.WorkerHandle{},core.ErrPath};role:="worker";if reviewer{role="review"};r:=a.runner.Run(ctx,a.executable,role,"--run",string(req.Packet.RunID),"--team",string(req.Packet.Team),"--task",string(req.Packet.Task),"--worktree",req.Worktree.Root);if r.Transport!=nil||r.TimedOut||r.Exit!=0{return contracts.WorkerHandle{},core.ErrCapacity};return contracts.WorkerHandle{Host:a.name,Identity:a.name+":"+role,PacketDigest:req.Packet.QueueFingerprint,CandidateRevision:req.Packet.SpecRevision,Run:req.Packet.RunID,Team:req.Packet.Team,Task:req.Packet.Task,Reviewer:reviewer},nil }
func (a *harnessAdapter) StartWorker(ctx context.Context,req contracts.WorkerRequest)(contracts.WorkerHandle,error){return a.start(ctx,req,false)}
func (a *harnessAdapter) StartReviewer(ctx context.Context,req contracts.WorkerRequest,author contracts.WorkerHandle)(contracts.WorkerHandle,error){if author.Identity==a.name+":review"{return contracts.WorkerHandle{},core.ErrRevision};return a.start(ctx,req,true)}
func (a *harnessAdapter) Poll(ctx context.Context,h contracts.WorkerHandle)(string,error){if h.Identity==""||a==nil||a.runner==nil{return "",core.ErrPath};r:=a.runner.Run(ctx,a.executable,"poll","--identity",h.Identity);if r.Transport!=nil||r.TimedOut||r.Exit!=0{return "",core.ErrCapacity};return string(r.Stdout),nil}
func (a *harnessAdapter) Stop(ctx context.Context,h contracts.WorkerHandle,_ core.Scope) error {if h.Identity==""||a==nil||a.runner==nil{return core.ErrPath};r:=a.runner.Run(ctx,a.executable,"stop","--identity",h.Identity);if r.Transport!=nil||r.TimedOut||r.Exit!=0{return core.ErrCapacity};return nil}
func (a *harnessAdapter) ReadIdentity(_ context.Context,h contracts.WorkerHandle)(string,error){if h.Identity==""{return "",core.ErrPath};return h.Identity,nil}
var _ contracts.HostAdapter = (*harnessAdapter)(nil)

// vnext/internal/model/model.go
package model
import (
  "github.com/thebpandey/agent-team/vnext/internal/contracts"
  "github.com/thebpandey/agent-team/vnext/internal/core"
  "github.com/thebpandey/agent-team/vnext/internal/host"
)
type Harness string
const ( Codex Harness = "codex"; Claude Harness = "claude" )
type Adapter = host.Adapter
type CommandRunner = host.CommandRunner
type ModelRoute struct { Harness Harness; Requested,Resolved,Effort string; Reviewer bool }
func RouteModel(host.Capabilities,Harness,string,bool)(ModelRoute,error)
```

- [ ] **Step 1: Write failing test:**
```go
package host_test
import ( "context"; "strings"; "testing"; "github.com/thebpandey/agent-team/vnext/internal/command"; "github.com/thebpandey/agent-team/vnext/internal/contracts"; "github.com/thebpandey/agent-team/vnext/internal/core"; "github.com/thebpandey/agent-team/vnext/internal/host"; "github.com/thebpandey/agent-team/vnext/internal/model"; "github.com/thebpandey/agent-team/vnext/internal/tracker" )
type fakeRunner struct { result tracker.CommandResult }
type recordingRunner struct { result tracker.CommandResult; calls [][]string }
func (r *recordingRunner) Run(_ context.Context,name string,args ...string) tracker.CommandResult { r.calls=append(r.calls,append([]string{name},args...)); return r.result }
func (f fakeRunner) Run(_ context.Context, name string, args ...string) tracker.CommandResult { _=name; _=strings.Join(args," "); return f.result }
func TestHostIdentity(t *testing.T) { r:=fakeRunner{result:tracker.CommandResult{Stdout:[]byte("codex 1.0")}}; if _,err:=host.NewCodex(r).Probe(context.Background()); err!=nil { t.Fatal(err) } }
func TestCodexClaudeConcreteAdapters(t *testing.T) { for _,tc:=range []struct{name string;new func(host.CommandRunner) host.Adapter}{{"codex",host.NewCodex},{"claude",host.NewClaude}} { r:=&recordingRunner{result:tracker.CommandResult{Stdout:[]byte("ok")}}; a:=tc.new(r); p:=core.AssignmentPacket{RecordEnvelope:core.RecordEnvelope{RunID:"R"},Team:"TEAM",Task:"TASK",QueueFingerprint:"q",SpecRevision:"1"}; req:=contracts.WorkerRequest{Packet:p,Worktree:contracts.WorktreeSpec{Run:"R",Team:"TEAM",Root:"/tmp/task"}}; h,err:=a.StartWorker(context.Background(),req);if err!=nil||h.Identity!=tc.name+":worker"{t.Fatal(tc.name,h,err)};rh,err:=a.StartReviewer(context.Background(),req,h);if err!=nil||rh.Identity!=tc.name+":review"||rh.Identity==h.Identity{t.Fatal(tc.name,rh,err)};if len(r.calls)<2||r.calls[0][0]!=tc.name||r.calls[0][1]!="worker"||r.calls[1][1]!="review"{t.Fatal(r.calls)}} }
func TestRouteModel(t *testing.T) { got,err:=model.RouteModel(host.Capabilities{Models:[]string{"small"}},model.Codex,"small",true); if err!=nil || !got.Reviewer { t.Fatal(got,err) } }
func TestNativeRunnerBoundedAndAdapterCompatible(t *testing.T) { r:=command.NewRunner(8); var _ host.CommandRunner=r; got:=r.Run(context.Background(),"go","version"); if got.Transport!=nil || len(got.Stdout)>8 || len(got.Stderr)>8 { t.Fatal(got) }; missing:=r.Run(context.Background(),"agent-team-command-that-does-not-exist"); if missing.Transport==nil || missing.Exit!=-1 { t.Fatal(missing) } }
func TestNativeRunnerStreamsNoisyChildWithinLimit(t *testing.T){ r:=command.NewRunner(16);got:=r.Run(context.Background(),"go","help","build");if got.Transport!=nil||len(got.Stdout)>16||len(got.Stderr)>16||len(got.Stdout)+len(got.Stderr)==0{t.Fatal(got)} }
```
- [ ] **Step 2: Run:** `cd vnext && go test ./internal/host ./internal/command ./internal/model`; **Expected:** FAIL because adapters and native runner are absent.
- [ ] **Step 3: Implement:** use exact executable argument arrays, typed unavailable/identity errors, packet/worktree-only inputs, and distinct reviewer identity.
```go
func RouteModel(c host.Capabilities, h Harness, requested string, reviewer bool) (ModelRoute,error) { for _, m := range c.Models { if m == requested { return ModelRoute{Harness:h,Requested:requested,Resolved:m,Reviewer:reviewer},nil } }; return ModelRoute{}, core.ErrCapacity }
```
- [ ] **Step 4: Run:** `cd vnext && gofmt -w internal/host/capabilities.go internal/host/adapter.go internal/host/host_test.go internal/command/runner.go internal/command/runner_test.go internal/model/model.go && go test ./internal/host ./internal/command ./internal/model -count=1`; **Expected:** PASS.
- [ ] **Step 5: Commit:** `git add vnext/internal/host vnext/internal/command vnext/internal/model && git commit -m "feat(vnext): establish native host boundaries"`.

### Task 2: Immutable packet dispatch and task-owned worktrees

**Files:** Create `vnext/internal/dispatch/{dispatch.go,dispatch_test.go}` and `vnext/internal/worktree/{worktree.go,worktree_test.go}`.

**Interfaces — Consumes:** `core.AssignmentPacket`, `core.TeamID`, `core.ErrPath`, `contracts.WorktreeSpec`, `contracts.Worktree`, and `contracts.WorkerHandle` from Phase 1.

**Imports/owners:** `core.*` comes from `internal/core`; `contracts.*`, including the inherited `WorktreeManager`, comes from `internal/contracts`; `ValidatePacket` is local to `internal/dispatch`.

**Interfaces — Produces:** `Dispatcher`, the concrete `Manager` implementation of the exact inherited Phase 1 `contracts.WorktreeManager` plus local `ExactWorktreeRemover`, and `ValidatePacket` shown below for Tasks 3, 6, and 7. `Manager` is the only Phase 2 component that creates, inspects, integrates, or removes task worktrees; it uses Git argument arrays through Task 1's bounded native runner.

```go
type Dispatcher interface { Dispatch(context.Context,core.AssignmentPacket,contracts.WorktreeSpec)(contracts.WorkerHandle,error) }
func NewDispatcher(contracts.HostAdapter) Dispatcher
type WorktreeManager = contracts.WorktreeManager
type ExactWorktreeRemover interface { RemoveExact(context.Context,contracts.Worktree) error }
type WorktreeIdentity struct { core.RecordEnvelope; Team core.TeamID; Path,Base,Branch string; Removed bool }
type Manager struct { repoRoot,project string; state *store.Store; runner host.CommandRunner; records map[string]contracts.Worktree }
func NewManager(repoRoot, project string, state *store.Store, runner host.CommandRunner) *Manager
func ValidatePacket(core.AssignmentPacket,contracts.WorktreeSpec) error
```

- [ ] **Step 1: Write failing test:**
```go
import ( "context"; "errors"; "os"; "path/filepath"; "reflect"; "runtime"; "strings"; "testing"; "github.com/thebpandey/agent-team/vnext/internal/contracts"; "github.com/thebpandey/agent-team/vnext/internal/core"; "github.com/thebpandey/agent-team/vnext/internal/dispatch"; "github.com/thebpandey/agent-team/vnext/internal/store"; "github.com/thebpandey/agent-team/vnext/internal/testkit"; "github.com/thebpandey/agent-team/vnext/internal/tracker"; "github.com/thebpandey/agent-team/vnext/internal/worktree" )
func TestMainCheckoutRejected(t *testing.T) { p:=core.AssignmentPacket{RecordEnvelope:core.RecordEnvelope{Schema:1,Project:"p",RunID:"R",Revision:1},Task:"T",Team:"TEAM",Base:"abc",Scope:[]string{"src"}}; if err:=dispatch.ValidatePacket(p,contracts.WorktreeSpec{Run:"R",Team:"TEAM",Root:".",Base:"abc",WritablePaths:[]string{"."}}); !errors.Is(err,core.ErrPath) { t.Fatal(err) } }
func TestPacketContainment(t *testing.T) { p:=core.AssignmentPacket{RecordEnvelope:core.RecordEnvelope{RunID:"R"},Team:"TEAM",Base:"abc"}; root:=t.TempDir(); good:=filepath.Join(root,"src"); if err:=dispatch.ValidatePacket(p,contracts.WorktreeSpec{Run:"R",Team:"TEAM",Root:root,Base:"abc",WritablePaths:[]string{good}}); err!=nil { t.Fatal(err) }; for _,escape:=range []string{filepath.Join(root,"..","outside"),filepath.Join(root,"src","..","..","outside")} { if err:=dispatch.ValidatePacket(p,contracts.WorktreeSpec{Run:"R",Team:"TEAM",Root:root,Base:"abc",WritablePaths:[]string{escape}}); !errors.Is(err,core.ErrPath) { t.Fatal(escape,err) } } }
func TestProspectiveRootValidatesBeforeGit(t *testing.T) { root:=filepath.Join(t.TempDir(),"new-worktree"); p:=core.AssignmentPacket{RecordEnvelope:core.RecordEnvelope{RunID:"R"},Team:"TEAM",Base:"abc"}; if err:=dispatch.ValidatePacket(p,contracts.WorktreeSpec{Run:"R",Team:"TEAM",Root:root,Base:"abc",WritablePaths:[]string{"../../outside"}}); !errors.Is(err,core.ErrPath){t.Fatal(err)} }
func TestManagerRejectsProspectiveWritableEscapeBeforeGit(t *testing.T){repo:=testkit.GitRepo(t);r:=&gitRunner{};m:=worktree.NewManager(repo,"project",store.New(repo,core.StorageLimits{CanonicalBytes:16<<20}),r);root:=filepath.Join(repo,".agent-team","worktrees","new");_,err:=m.Create(context.Background(),contracts.WorktreeSpec{Run:"R",Team:"A",Root:root,Base:"base",WritablePaths:[]string{"../../outside"}});if !errors.Is(err,core.ErrPath)||len(r.calls)!=0{t.Fatal(err,r.calls)}}
type dispatchFakeAdapter struct { called bool }
func (f *dispatchFakeAdapter) Probe(context.Context)(contracts.HostCapabilities,error){return contracts.HostCapabilities{},nil}
func (f *dispatchFakeAdapter) StartWorker(_ context.Context,r contracts.WorkerRequest)(contracts.WorkerHandle,error){f.called=true;return contracts.WorkerHandle{Run:r.Packet.RunID,Team:r.Packet.Team,Task:r.Packet.Task,Identity:"dev",PacketDigest:r.Packet.QueueFingerprint},nil}
func (f *dispatchFakeAdapter) StartReviewer(context.Context,contracts.WorkerRequest,contracts.WorkerHandle)(contracts.WorkerHandle,error){return contracts.WorkerHandle{},nil}
func (f *dispatchFakeAdapter) Poll(context.Context,contracts.WorkerHandle)(string,error){return "",nil}
func (f *dispatchFakeAdapter) Stop(context.Context,contracts.WorkerHandle,core.Scope) error{return nil}
func (f *dispatchFakeAdapter) ReadIdentity(context.Context,contracts.WorkerHandle)(string,error){return "dev",nil}
func TestDispatcherDispatchesValidatedPacket(t *testing.T){f:=&dispatchFakeAdapter{};d:=dispatch.NewDispatcher(f);p:=core.AssignmentPacket{RecordEnvelope:core.RecordEnvelope{RunID:"R"},Task:"T",Team:"TEAM",Base:"abc",QueueFingerprint:"q"};w:=contracts.WorktreeSpec{Run:"R",Team:"TEAM",Root:filepath.Join(t.TempDir(),"task"),Base:"abc",WritablePaths:[]string{"src"}};h,err:=d.Dispatch(context.Background(),p,w);if err!=nil||!f.called||h.Identity!="dev"{t.Fatal(h,err)};p.Team="OTHER";if _,err=d.Dispatch(context.Background(),p,w);!errors.Is(err,core.ErrPath){t.Fatal(err)};p.Team="TEAM";p.RunID="OTHER";if _,err=d.Dispatch(context.Background(),p,w);!errors.Is(err,core.ErrPath){t.Fatal(err)}}
func TestPacketSymlinkContainment(t *testing.T) { p:=core.AssignmentPacket{RecordEnvelope:core.RecordEnvelope{RunID:"R"},Team:"TEAM",Base:"abc"}; root,outside:=t.TempDir(),t.TempDir(); link:=filepath.Join(root,"escape"); if err:=os.Symlink(outside,link); err!=nil { if runtime.GOOS=="windows" { t.Skip("symlink privilege unavailable") }; t.Fatal(err) }; if err:=dispatch.ValidatePacket(p,contracts.WorktreeSpec{Run:"R",Team:"TEAM",Root:root,Base:"abc",WritablePaths:[]string{filepath.Join(link,"owned.go")}}); !errors.Is(err,core.ErrPath) { t.Fatal(err) } }
type gitRunner struct { calls [][]string; result tracker.CommandResult }
func (r *gitRunner) Run(_ context.Context,name string,args ...string) tracker.CommandResult { r.calls=append(r.calls,append([]string{name},args...)); return r.result }
func TestManagerGitLifecycleAndResumeIdentity(t *testing.T) { repo:=testkit.GitRepo(t); state:=store.New(repo,core.StorageLimits{CanonicalBytes:16<<20}); r:=&gitRunner{}; m:=worktree.NewManager(repo,"project",state,r); var _ contracts.WorktreeManager=m; root:=filepath.Join(repo,".agent-team","worktrees","R-A"); spec:=contracts.WorktreeSpec{Run:"R",Team:"A",Root:root,Base:"base",WritablePaths:[]string{"src"}}; got,err:=m.Create(context.Background(),spec); if err!=nil||got.Path!=root||got.Run!="R"||got.Team!="A" { t.Fatal(got,err) }; if want:=[]string{"git","-C",repo,"worktree","add","-b","agent-team/R/A",root,"base"}; !reflect.DeepEqual(r.calls[0],want){t.Fatal(r.calls)}; if _,err=m.Inspect(context.Background(),got);err!=nil{t.Fatal(err)}; resumed:=worktree.NewManager(repo,"project",state,r);if _,err=resumed.Inspect(context.Background(),got);err!=nil{t.Fatal("fresh manager could not resume",err)}; forged:=got;forged.Team="B";if _,err=resumed.Inspect(context.Background(),forged);!errors.Is(err,core.ErrPath){t.Fatal(err)}; if _,err=m.Integrate(context.Background(),contracts.Candidate{Task:"T",Revision:"candidate",Base:"base",Worktree:got});err!=nil{t.Fatal(err)};if err=m.Cleanup(context.Background(),"A");err!=nil{t.Fatal(err)};fresh:=worktree.NewManager(repo,"project",state,r);if _,err=fresh.Inspect(context.Background(),got);!errors.Is(err,core.ErrPath){t.Fatal("tombstoned identity resumed",err)}; if len(r.calls)!=4||!reflect.DeepEqual(r.calls[1],[]string{"git","-C",repo,"merge","--no-ff","--no-edit","candidate"})||!reflect.DeepEqual(r.calls[2],[]string{"git","-C",repo,"worktree","remove",root})||!reflect.DeepEqual(r.calls[3],[]string{"git","-C",repo,"branch","-d","agent-team/R/A"}){t.Fatal(r.calls)} }
func TestManagerRejectsOutsideRoot(t *testing.T){repo:=testkit.GitRepo(t);r:=&gitRunner{};m:=worktree.NewManager(repo,"project",store.New(repo,core.StorageLimits{CanonicalBytes:16<<20}),r);outside:=filepath.Join(t.TempDir(),"outside");_,err:=m.Create(context.Background(),contracts.WorktreeSpec{Run:"R",Team:"A",Root:outside,Base:"base",WritablePaths:[]string{"src"}});if !errors.Is(err,core.ErrPath)||len(r.calls)!=0{t.Fatal(err,r.calls)};if _,statErr:=os.Stat(outside);!errors.Is(statErr,os.ErrNotExist){t.Fatal("outside destination changed",statErr)} }
```
- [ ] **Step 2: Run:** `cd vnext && go test ./internal/dispatch ./internal/worktree`; **Expected:** FAIL because validation is absent.
- [ ] **Step 3: Implement:** hash the complete packet, reject main-checkout paths/scope escapes/base mismatch/mutable reuse, and implement `Manager` as the sole `contracts.WorktreeManager`: `Create` records a run/team/base/path/branch identity only after `git worktree add -b`; `Inspect` returns only that exact identity; `Integrate` merges the exact checked candidate; and `Cleanup` removes the exact clean worktree before deleting its branch. Every Git call uses the bounded Task 1 runner and a fixed argument array.
```go
// vnext/internal/dispatch/dispatch.go
package dispatch
import ( "context"; "path/filepath"; "strings"; "github.com/thebpandey/agent-team/vnext/internal/contracts"; "github.com/thebpandey/agent-team/vnext/internal/core" )
type Dispatcher interface { Dispatch(context.Context,core.AssignmentPacket,contracts.WorktreeSpec)(contracts.WorkerHandle,error) }
type dispatcher struct { adapter contracts.HostAdapter }
func NewDispatcher(a contracts.HostAdapter) Dispatcher { return &dispatcher{adapter:a} }
func (d *dispatcher) Dispatch(ctx context.Context,p core.AssignmentPacket,w contracts.WorktreeSpec)(contracts.WorkerHandle,error) { if d==nil||d.adapter==nil{return contracts.WorkerHandle{},core.ErrCapacity};if err:=ValidatePacket(p,w);err!=nil{return contracts.WorkerHandle{},err};h,err:=d.adapter.StartWorker(ctx,contracts.WorkerRequest{Packet:p,Worktree:w,WritablePaths:append([]string(nil),w.WritablePaths...),Reviewer:false});if err!=nil{return contracts.WorkerHandle{},err};if h.Run!=p.RunID||h.Team!=p.Team||h.Task!=p.Task||h.PacketDigest!=p.QueueFingerprint{return contracts.WorkerHandle{},core.ErrRevision};return h,nil }
func ValidatePacket(p core.AssignmentPacket,w contracts.WorktreeSpec) error { if w.Run!=p.RunID||w.Team!=p.Team||w.Base!=p.Base||len(w.WritablePaths)==0||filepath.Clean(w.Root)=="."{return core.ErrPath};root,err:=canonicalProspective(w.Root);if err!=nil{return core.ErrPath};for _,raw:=range w.WritablePaths{candidate:=raw;if !filepath.IsAbs(candidate){candidate=filepath.Join(root,raw)};abs,err:=canonicalCandidate(candidate);if err!=nil{return core.ErrPath};rel,err:=filepath.Rel(root,abs);if err!=nil||rel=="."||rel==".."||strings.HasPrefix(rel,".."+string(filepath.Separator)){return core.ErrPath}};return nil }
func canonicalProspective(path string)(string,error){abs,err:=filepath.Abs(filepath.Clean(path));if err!=nil{return "",err};if real,err:=filepath.EvalSymlinks(abs);err==nil{return real,nil};return canonicalCandidate(abs)}
func canonicalCandidate(path string)(string,error){abs,err:=filepath.Abs(filepath.Clean(path));if err!=nil{return "",err};for probe:=abs;;probe=filepath.Dir(probe){if real,err:=filepath.EvalSymlinks(probe);err==nil{suffix,err:=filepath.Rel(probe,abs);if err!=nil{return "",err};return filepath.Join(real,suffix),nil};if parent:=filepath.Dir(probe);parent==probe{return "",core.ErrPath}}}
```
```go
    package worktree
    import ( "context"; "path/filepath"; "strings"; "time"; "github.com/thebpandey/agent-team/vnext/internal/contracts"; "github.com/thebpandey/agent-team/vnext/internal/core"; dispatch "github.com/thebpandey/agent-team/vnext/internal/dispatch"; "github.com/thebpandey/agent-team/vnext/internal/host"; "github.com/thebpandey/agent-team/vnext/internal/store" )
    func worktreeKey(run core.RunID, team core.TeamID) string { return string(run)+"\x00"+string(team) }
    func worktreeIdentityPath(team core.TeamID) string { return filepath.ToSlash(filepath.Join(".agent-team","runtime","worktrees",string(team)+".json")) }
    func NewManager(repoRoot,project string,state *store.Store,runner host.CommandRunner) *Manager { return &Manager{repoRoot:repoRoot,project:project,state:state,runner:runner,records:map[string]contracts.Worktree{}} }
    func (m *Manager) git(ctx context.Context,args ...string) error { if m.runner==nil||m.repoRoot==""{return core.ErrPath}; r:=m.runner.Run(ctx,"git",append([]string{"-C",m.repoRoot},args...)...);if r.Transport!=nil||r.TimedOut||r.Exit!=0{return core.ErrGit};return nil }
    func (m *Manager) save(ctx context.Context,w contracts.Worktree) error { if m.state==nil||m.project==""{return core.ErrPath};_,err:=m.state.WriteJSON(worktreeIdentityPath(w.Team),WorktreeIdentity{RecordEnvelope:core.RecordEnvelope{Schema:1,Project:m.project,RunID:w.Run,WrittenAt:time.Now().UTC().Format(time.RFC3339Nano),Revision:1},Team:w.Team,Path:w.Path,Base:w.Base,Branch:w.Branch},64<<10);return err }
    func (m *Manager) tombstone(w contracts.Worktree) error {if m.state==nil||m.project==""{return core.ErrPath};_,err:=m.state.WriteJSON(worktreeIdentityPath(w.Team),WorktreeIdentity{RecordEnvelope:core.RecordEnvelope{Schema:1,Project:m.project,RunID:w.Run,WrittenAt:time.Now().UTC().Format(time.RFC3339Nano),Revision:2},Team:w.Team,Removed:true},64<<10);return err}
    func (m *Manager) load(run core.RunID,team core.TeamID)(contracts.Worktree,error){if m.state==nil{return contracts.Worktree{},core.ErrPath};var saved WorktreeIdentity;if err:=m.state.ReadJSON(worktreeIdentityPath(team),64<<10,&saved);err!=nil||saved.Schema!=1||saved.Project!=m.project||saved.RunID!=run||saved.Team!=team||saved.Removed||saved.Path==""||saved.Base==""||saved.Branch==""{return contracts.Worktree{},core.ErrPath};return contracts.Worktree{Run:run,Team:team,Path:saved.Path,Base:saved.Base,Branch:saved.Branch},nil}
func (m *Manager) Create(ctx context.Context,s contracts.WorktreeSpec)(contracts.Worktree,error){ if m==nil||m.runner==nil||m.state==nil||m.project==""||s.Run==""||s.Team==""||s.Root==""||s.Base==""||len(s.WritablePaths)==0{return contracts.Worktree{},core.ErrPath};if err:=dispatch.ValidatePacket(core.AssignmentPacket{RecordEnvelope:core.RecordEnvelope{RunID:s.Run},Team:s.Team,Base:s.Base},s);err!=nil{return contracts.Worktree{},err};repo,err:=canonicalExisting(m.repoRoot);if err!=nil{return contracts.Worktree{},core.ErrPath};candidate,err:=canonicalCandidate(s.Root);if err!=nil{return contracts.Worktree{},core.ErrPath};rel,err:=filepath.Rel(repo,candidate);if err!=nil||rel=="."||rel==".."||strings.HasPrefix(rel,".."+string(filepath.Separator)){return contracts.Worktree{},core.ErrPath};key:=worktreeKey(s.Run,s.Team);if _,ok:=m.records[key];ok{return contracts.Worktree{},core.ErrTransition};branch:="agent-team/"+string(s.Run)+"/"+string(s.Team);if err:=m.git(ctx,"worktree","add","-b",branch,candidate,s.Base);err!=nil{return contracts.Worktree{},err};w:=contracts.Worktree{Run:s.Run,Team:s.Team,Path:candidate,Branch:branch,Base:s.Base};if err:=m.save(ctx,w);err!=nil{return contracts.Worktree{},err};m.records[key]=w;return w,nil }
    func (m *Manager) Inspect(_ context.Context,w contracts.Worktree)(contracts.Worktree,error){ expected,ok:=m.records[worktreeKey(w.Run,w.Team)];if !ok{var err error;expected,err=m.load(w.Run,w.Team);if err!=nil{return contracts.Worktree{},err};m.records[worktreeKey(w.Run,w.Team)]=expected};if w.Path!=expected.Path||w.Base!=expected.Base||w.Branch!=expected.Branch{return contracts.Worktree{},core.ErrPath};return expected,nil }
    func (m *Manager) Integrate(ctx context.Context,c contracts.Candidate)(contracts.Candidate,error){ if _,err:=m.Inspect(ctx,c.Worktree);err!=nil||c.Revision==""||c.Base!=c.Worktree.Base{return contracts.Candidate{},core.ErrPath};if err:=m.git(ctx,"merge","--no-ff","--no-edit",c.Revision);err!=nil{return contracts.Candidate{},err};return c,nil }
func (m *Manager) RemoveExact(ctx context.Context,w contracts.Worktree) error { expected,err:=m.Inspect(ctx,w);if err!=nil{return err};if expected.Path!=w.Path||expected.Branch!=w.Branch||expected.Team!=w.Team||expected.Run!=w.Run{return core.ErrPath};if err:=m.git(ctx,"worktree","remove",expected.Path);err!=nil{return err};if err:=m.git(ctx,"branch","-d",expected.Branch);err!=nil{return err};if err:=m.tombstone(expected);err!=nil{return err};delete(m.records,worktreeKey(expected.Run,expected.Team));return nil }
func (m *Manager) Cleanup(ctx context.Context,team core.TeamID) error { for _,w:=range append([]contracts.Worktree(nil),m.records...){if w.Team==team{if err:=m.RemoveExact(ctx,w);err!=nil{return err}}};return nil }
var _ ExactWorktreeRemover = (*Manager)(nil)
    var _ contracts.WorktreeManager = (*Manager)(nil)
func canonicalExisting(path string)(string,error){ abs,err:=filepath.Abs(filepath.Clean(path)); if err!=nil{return "",err}; return filepath.EvalSymlinks(abs) }
func canonicalCandidate(path string)(string,error){ abs,err:=filepath.Abs(filepath.Clean(path)); if err!=nil{return "",err}; for probe:=abs;;probe=filepath.Dir(probe) { if real,err:=filepath.EvalSymlinks(probe);err==nil { suffix,err:=filepath.Rel(probe,abs);if err!=nil{return "",err};return filepath.Join(real,suffix),nil }; if parent:=filepath.Dir(probe);parent==probe{return "",core.ErrPath} } }
```
- [ ] **Step 4: Run:** `cd vnext && gofmt -w internal/dispatch/dispatch.go internal/dispatch/dispatch_test.go internal/worktree/worktree.go internal/worktree/worktree_test.go && go test ./internal/dispatch ./internal/worktree -count=1`; **Expected:** PASS.
- [ ] **Step 5: Commit:** `git add vnext/internal/dispatch vnext/internal/worktree && git commit -m "feat(vnext): enforce immutable packets and worktrees"`.

### Task 3: Foreground supervision and durable events

**Files:** Create `vnext/internal/supervise/{supervise.go,supervise_test.go}`.

**Interfaces — Consumes:** `host.Adapter`, `host.CommandRunner`, `store.Store`, `core.AssignmentPacket`, `core.RunID`, `core.Scope`, `contracts.WorkerHandle`, and `workflow.Event` from Phase 1/Tasks 1–2.

**Imports/owners:** `host.Adapter` and `host.CommandRunner` are Task 1 `internal/host`; `store.Store`, `core.*`, `contracts.*`, and `workflow.Event` are Phase 1 packages under `internal/store`, `internal/core`, `internal/contracts`, and `internal/workflow`.

**Interfaces — Produces:** `Observation`, `Supervisor`, `NewSupervisor`, and durable foreground events shown below for Tasks 4 and 6.

```go
type Observation = string
type Supervisor interface { Start(context.Context,core.AssignmentPacket)(contracts.WorkerHandle,error); Turn(context.Context,contracts.WorkerHandle)(Observation,error); Emit(context.Context,workflow.Event) error; Checkpoint(context.Context,core.RunID,core.Scope,string) error }
func NewSupervisor(*store.Store,host.Adapter,host.CommandRunner) Supervisor
func InterruptedEvent(contracts.WorkerHandle) workflow.Event
func InterruptedState() core.TaskState
```

- [ ] **Step 1: Write failing test:**
```go
import ( "context"; "errors"; "testing"; "github.com/thebpandey/agent-team/vnext/internal/contracts"; "github.com/thebpandey/agent-team/vnext/internal/core"; "github.com/thebpandey/agent-team/vnext/internal/store"; "github.com/thebpandey/agent-team/vnext/internal/supervise"; "github.com/thebpandey/agent-team/vnext/internal/workflow" )
func TestCancelledTurn(t *testing.T) { s:=supervise.NewSupervisor(store.New(t.TempDir(),core.StorageLimits{CanonicalBytes:16<<20}),nil,nil); ctx,cancel:=context.WithCancel(context.Background()); cancel(); _,err:=s.Turn(ctx,contracts.WorkerHandle{Run:"R",Team:"TEAM",Task:"T"}); if !errors.Is(err,core.ErrTransition) { t.Fatal(err) } }
func TestInterruptedRepresentation(t *testing.T){ h:=contracts.WorkerHandle{Run:"R",Task:"T"};event:=supervise.InterruptedEvent(h);if event.Kind!=workflow.Checkpoint||event.Run!="R"||event.Scope.Kind!=core.ScopeTask||event.Scope.ID!="T"{t.Fatal(event)};if got:=supervise.InterruptedState();got!=core.Interrupted{t.Fatal(got)} }
```
- [ ] **Step 2: Run:** `cd vnext && go test ./internal/supervise`; **Expected:** FAIL because supervision is absent.
- [ ] **Step 3: Implement:** write ordered foreground events/checkpoint digests, preserve output/argument evidence on cancellation, persist the valid `workflow.Checkpoint` event with its checkpoint digest, and transition the affected task record to the exact Phase 1 `core.Interrupted` state. Do not invent an `Interrupted` event kind; add no daemon/lease/PID gate/background goroutine.
```go
func InterruptedEvent(h contracts.WorkerHandle) workflow.Event { return workflow.Event{Run:h.Run,Scope:core.Scope{Kind:core.ScopeTask,ID:h.Task},Kind:workflow.Checkpoint} }
func InterruptedState() core.TaskState { return core.Interrupted }
```
- [ ] **Step 4: Run:** `cd vnext && gofmt -w internal/supervise/supervise.go internal/supervise/supervise_test.go && go test ./internal/supervise -count=1`; **Expected:** PASS.
- [ ] **Step 5: Commit:** `git add vnext/internal/supervise && git commit -m "feat(vnext): add foreground supervision"`.

### Task 4: Independent reviewer, FIX/CLEAN, and repair invalidation

**Files:** Create `vnext/internal/review/{review.go,review_test.go}`.

**Interfaces — Consumes:** `core.Task`, `core.RecordEnvelope`, `contracts.Candidate`, `contracts.WorkerHandle`, `core.Check`, `host.Adapter`, `host.CommandRunner`, and `store.Store`; reviewer identity is the Phase 1 `string` returned by `HostAdapter.ReadIdentity`.

**Imports/owners:** `core.*`, `contracts.*`, and `store.Store` are Phase 1 `internal/core`, `internal/contracts`, and `internal/store`; `host.Adapter` and `host.CommandRunner` are Task 1 outputs.

**Interfaces — Produces:** `Verdict`, `Input`, `Result`, `Reviewer`, and `NewReviewer` shown below for Tasks 5, 6, and 8.

```go
type Verdict string
const ( FIX Verdict = "FIX"; CLEAN Verdict = "CLEAN" )
type Input struct { Task core.Task; Candidate contracts.Candidate; Developer contracts.WorkerHandle; Checks []core.Check; CandidateDigest string }
type Result struct { core.RecordEnvelope; Verdict Verdict; Findings []string; EvidencePointer string; ReviewerIdentity string; CandidateDigest string }
type Reviewer interface { Review(context.Context,Input)(Result,error) }
func NewReviewer(host.Adapter,host.CommandRunner,*store.Store) Reviewer
```

- [ ] **Step 1: Write failing test:**
```go
import ( "context"; "errors"; "testing"; "github.com/thebpandey/agent-team/vnext/internal/contracts"; "github.com/thebpandey/agent-team/vnext/internal/core"; "github.com/thebpandey/agent-team/vnext/internal/review"; "github.com/thebpandey/agent-team/vnext/internal/store" )
func TestReviewerAndMutation(t *testing.T) { r:=review.NewReviewer(nil,nil,store.New(t.TempDir(),core.StorageLimits{CanonicalBytes:16<<20})); in:=review.Input{Task:core.Task{ID:"T"},Developer:contracts.WorkerHandle{},CandidateDigest:"sha256:a"}; got,err:=r.Review(context.Background(),in); if err!=nil || (got.Verdict!=review.FIX && got.Verdict!=review.CLEAN) { t.Fatal(got,err) }; in.CandidateDigest="sha256:b"; if _,err:=r.Review(context.Background(),in); !errors.Is(err,core.ErrRevision) { t.Fatal(err) } }
```
- [ ] **Step 2: Run:** `cd vnext && go test ./internal/review`; **Expected:** FAIL because review is absent.
- [ ] **Step 3: Implement:** reserve a non-author reviewer; accept only FIX/CLEAN; persist findings; create bounded repair requests; bind review/gate to candidate/base digests. Candidate mutation invalidates both and requires fresh review and gate; blocked is task/run state only.
```go
func validateVerdict(v Verdict) error { if v != FIX && v != CLEAN { return core.ErrTransition }; return nil }
```
- [ ] **Step 4: Run:** `cd vnext && gofmt -w internal/review/review.go internal/review/review_test.go && go test ./internal/review -count=1`; **Expected:** PASS.
- [ ] **Step 5: Commit:** `git add vnext/internal/review && git commit -m "feat(vnext): add independent FIX-CLEAN review"`.

### Task 5: Deterministic gate and serial integration

**Files:** Create `vnext/internal/gate/{gate.go,gate_test.go}` and `vnext/internal/integrate/{integrate.go,integrate_test.go}`.

**Interfaces — Consumes:** Phase 1 `contracts.GateInput`, `contracts.GateResult`, `contracts.Candidate`, `project.Project`, `store.Store`, and Task 2's exact `contracts.WorktreeManager`.

**Imports/owners:** `contracts.*`, `project.Project`, and `store.Store` are Phase 1 `internal/contracts`, `internal/project`, and `internal/store`; Task 2 only aliases the inherited `contracts.WorktreeManager`.

**Interfaces — Produces:** `Gate`, `Integrator`, `Integration`, `NewGate`, and `NewIntegrator` shown below for Tasks 6–8.

```go
type Gate interface { Check(context.Context,contracts.GateInput)(contracts.GateResult,error) }
type Integrator interface { Integrate(context.Context,contracts.Candidate,contracts.GateResult)(Integration,error) }
type Integration struct { core.RecordEnvelope; Task core.TaskID; Base,Candidate,Commit string; Order int; EvidencePointer string }
func NewGate(host.CommandRunner,*store.Store) Gate
func NewIntegrator(project.Project,*store.Store,contracts.WorktreeManager) Integrator
```

- [ ] **Step 1: Write failing test:**
```go
import ( "context"; "reflect"; "testing"; "github.com/thebpandey/agent-team/vnext/internal/contracts"; "github.com/thebpandey/agent-team/vnext/internal/core"; "github.com/thebpandey/agent-team/vnext/internal/gate"; "github.com/thebpandey/agent-team/vnext/internal/integrate"; "github.com/thebpandey/agent-team/vnext/internal/store" )
type integrationRecorder struct { order []string }
func (r *integrationRecorder) IntegrateTwo(ids []string) error { r.order=append(r.order,ids...); return nil }
func (r *integrationRecorder) Order() []string { return r.order }
func TestGateRejectsDirtyAndSerializes(t *testing.T) { g:=gate.NewGate(nil,store.New(t.TempDir(),core.StorageLimits{CanonicalBytes:16<<20})); if _,err:=g.Check(context.Background(),contracts.GateInput{Candidate:contracts.Candidate{Revision:"new"},WorktreeDirty:true}); err==nil { t.Fatal("dirty candidate passed") }; rec:=&integrationRecorder{}; if err:=rec.IntegrateTwo([]string{"TASK-1","TASK-2"}); err!=nil || !reflect.DeepEqual(rec.Order(),[]string{"TASK-1","TASK-2"}) { t.Fatal(rec.Order(),err) } }
```
- [ ] **Step 2: Run:** `cd vnext && go test ./internal/gate ./internal/integrate`; **Expected:** FAIL because gate/integration are absent.
- [ ] **Step 3: Implement:** run checks deterministically, bind gate to candidate/base/review digests, reject dirty/wrong-base/mutated candidates, integrate one at a time by task ID, and preserve conflict worktrees/evidence.
```go
func rejectGate(in contracts.GateInput) error { if in.WorktreeDirty || in.ReceiptDigest=="" || in.ReviewDigest=="" { return core.ErrTransition }; return nil }
```
- [ ] **Step 4: Run:** `cd vnext && gofmt -w internal/gate/gate.go internal/gate/gate_test.go internal/integrate/integrate.go internal/integrate/integrate_test.go && go test ./internal/gate ./internal/integrate -count=1`; **Expected:** PASS.
- [ ] **Step 5: Commit:** `git add vnext/internal/gate vnext/internal/integrate && git commit -m "feat(vnext): add deterministic gate and serial integration"`.

### Task 6: Scoped lifecycle, checkpoint, handoff, and cross-host resume

**Files:** Create `vnext/internal/lifecycle/{lifecycle.go,lifecycle_test.go}`, `vnext/internal/handoff/{handoff.go,handoff_test.go}`, and `vnext/internal/orchestrator/{orchestrator.go,orchestrator_test.go}`.

**Interfaces — Consumes:** Phase 1 `workflow.Transition`, `workflow.Checkpoint`, `store.Store`, `tracker.Tracker`, and Task 2–5 dispatcher/supervisor/reviewer/gate/integrator interfaces.

**Imports/owners:** `workflow.*`, `store.Store`, and `tracker.Tracker` are Phase 1 `internal/workflow`, `internal/store`, and `internal/tracker`; dispatcher/supervisor/reviewer/gate/integrator are Task 2–5 packages.

**Interfaces — Produces:** `Lifecycle`, `Orchestrator`, `Handoff`, `NewLifecycle`, and `NewOrchestrator` shown below for Task 8.

```go
import ( "context"; "strings"; "github.com/thebpandey/agent-team/vnext/internal/core"; "github.com/thebpandey/agent-team/vnext/internal/dispatch"; "github.com/thebpandey/agent-team/vnext/internal/gate"; "github.com/thebpandey/agent-team/vnext/internal/integrate"; "github.com/thebpandey/agent-team/vnext/internal/review"; "github.com/thebpandey/agent-team/vnext/internal/store"; "github.com/thebpandey/agent-team/vnext/internal/supervise"; "github.com/thebpandey/agent-team/vnext/internal/tracker"; "github.com/thebpandey/agent-team/vnext/internal/workflow" )
type Lifecycle interface { Pause(context.Context,core.Scope,string) error; Stop(context.Context,core.Scope,string) error; Cancel(context.Context,core.Scope,string) error; Resume(context.Context,core.Scope) error; Checkpoint(context.Context,core.Scope,string) error }
type Orchestrator interface { Start(context.Context,core.RunID) error; Execute(context.Context,core.RunID) error; Resume(context.Context,core.RunID) error }
type Handoff interface { Derive(context.Context,core.RunID)([]byte,error) }
type ParsedAction struct { Name string; Selector []string; ScopeRequired bool }
func NewLifecycle(*store.Store,supervise.Supervisor) Lifecycle
func NewOrchestrator(*store.Store,tracker.Tracker,dispatch.Dispatcher,supervise.Supervisor,review.Reviewer,gate.Gate,integrate.Integrator) Orchestrator
func Parse([]string) (ParsedAction,error)
type ScopeLookup interface { TeamMember(context.Context,core.RunID,core.TeamID)(bool,error); TaskMember(context.Context,core.RunID,core.TaskID)(bool,error) }
func ResolveScope(context.Context,[]string,[]core.RunID,ScopeLookup) (core.Scope,error)
func ExecuteLifecycle(context.Context,ParsedAction,[]core.RunID,ScopeLookup,Lifecycle) error
```

- [ ] **Step 1: Write failing test:**
```go
import ( "context"; "testing"; "github.com/thebpandey/agent-team/vnext/internal/core"; "github.com/thebpandey/agent-team/vnext/internal/lifecycle"; "github.com/thebpandey/agent-team/vnext/internal/store" )
func TestScopedResume(t *testing.T) { l:=lifecycle.New(store.New(t.TempDir(),core.StorageLimits{CanonicalBytes:16<<20}),nil); if err:=l.Pause(context.Background(),core.Scope{Kind:core.ScopeTask,ID:"T1"},"user"); err!=nil { t.Fatal(err) }; if err:=l.Resume(context.Background(),core.Scope{Kind:core.ScopeTask,ID:"T1"}); err!=nil { t.Fatal(err) } }
```
- [ ] **Step 2: Run:** `cd vnext && go test ./internal/lifecycle ./internal/handoff ./internal/orchestrator`; **Expected:** FAIL because lifecycle is absent.
- [ ] **Step 3: Implement:** delegate to Phase 1 `workflow.Transition`/`Checkpoint`, preserve independent lanes, derive one bounded handoff, and resume from records on Codex↔Claude switch without leases.
```go
func resumeEvent(scope core.Scope) workflow.Event { return workflow.Event{Scope:scope,Kind:workflow.Resume} }
```
- [ ] **Step 3a: Implement deterministic scope resolution:** accept an omitted selector only when `activeRuns` has exactly one run; accept `--run <id>` only when that exact run is active; accept `--team <id>` and `--task <id>` only when one active run can be resolved; accept project-wide control only as explicit `--project <id>`; return `core.ErrTransition` for ambiguity, inactive IDs, malformed flags, or blank values. Never infer a project-wide scope from a missing selector.
```go
func ResolveScope(ctx context.Context,args []string,active []core.RunID,lookup ScopeLookup) (core.Scope,error) { if len(args)==0 {if len(active)!=1{return core.Scope{},core.ErrTransition};return core.Scope{Kind:core.ScopeRun,ID:string(active[0])},nil};if len(args)!=2||strings.TrimSpace(args[1])==""{return core.Scope{},core.ErrTransition};switch args[0] {case "--project":return core.Scope{Kind:core.ScopeProject,ID:args[1]},nil;case "--run":for _,id:=range active{if string(id)==args[1]{return core.Scope{Kind:core.ScopeRun,ID:id},nil}};return core.Scope{},core.ErrTransition;case "--team":if len(active)!=1||lookup==nil{ return core.Scope{},core.ErrTransition };ok,err:=lookup.TeamMember(ctx,active[0],core.TeamID(args[1]));if err!=nil||!ok{return core.Scope{},core.ErrTransition};return core.Scope{Kind:core.ScopeTeam,ID:args[1]},nil;case "--task":if len(active)!=1||lookup==nil{return core.Scope{},core.ErrTransition};ok,err:=lookup.TaskMember(ctx,active[0],core.TaskID(args[1]));if err!=nil||!ok{return core.Scope{},core.ErrTransition};return core.Scope{Kind:core.ScopeTask,ID:args[1]},nil;default:return core.Scope{},core.ErrTransition} }
```
- [ ] **Step 3b: Write executable resolver/parser tests:**
```go
import ( "context"; "errors"; "testing"; "github.com/thebpandey/agent-team/vnext/internal/acceptance"; "github.com/thebpandey/agent-team/vnext/internal/core"; "github.com/thebpandey/agent-team/vnext/internal/lifecycle" )
type durableScopeLookup struct{}
func (durableScopeLookup) TeamMember(context.Context,core.RunID,core.TeamID)(bool,error){return true,nil}
func (durableScopeLookup) TaskMember(context.Context,core.RunID,core.TaskID)(bool,error){return true,nil}
func TestResolveScopeAndParser(t *testing.T) { lookup:=durableScopeLookup{};one:=[]core.RunID{"R1"};if got,err:=lifecycle.ResolveScope(context.Background(),nil,one,lookup);err!=nil||got.Kind!=core.ScopeRun||got.ID!="R1"{t.Fatal(got,err)};for _,active:=range [][]core.RunID{nil,{"R1","R2"}}{if _,err:=lifecycle.ResolveScope(context.Background(),nil,active,lookup);!errors.Is(err,core.ErrTransition){t.Fatal(active,err)}};if got,err:=lifecycle.ResolveScope(context.Background(),[]string{"--project","P1"},nil,nil);err!=nil||got.Kind!=core.ScopeProject{t.Fatal(got,err)};if _,err:=lifecycle.ResolveScope(context.Background(),[]string{"--run","R9"},one,lookup);!errors.Is(err,core.ErrTransition){t.Fatal(err)};if got,err:=lifecycle.ResolveScope(context.Background(),[]string{"--task","T1"},one,lookup);err!=nil||got.Kind!=core.ScopeTask{t.Fatal(got,err)};for _,name:=range []string{"pause","stop","cancel","resume"}{intent,err:=acceptance.Parse([]string{name});if err!=nil||!intent.ScopeRequired||intent.Name!=name{t.Fatal(name,intent,err)};if err:=lifecycle.ExecuteLifecycle(context.Background(),intent,nil,nil,nil);!errors.Is(err,core.ErrTransition){t.Fatal(name,err)};if err:=lifecycle.ExecuteLifecycle(context.Background(),intent,[]core.RunID{"R1","R2"},nil,nil);!errors.Is(err,core.ErrTransition){t.Fatal(name,err)}};for _,args:=range [][]string{{"pause","--project","P1"},{"stop","--run","R1"},{"cancel","--team","TEAM-1"},{"resume","--task","T1"},{"start","--task","T1"},{"status","--run","R1"},{"deploy"},{"deploy","--target","staging"},{"deploy","--batch-size","2","--run","R1"},{"deploy","--target","staging","--run","R1","--batch-size","2"}}{if _,err:=acceptance.Parse(args);err!=nil{t.Fatal(args,err)}};for _,args:=range [][]string{{"deploy","--run"},{"deploy","--run","R1","--run","R2"},{"deploy","--bogus","x"},{"deploy","--batch-size","0"},{"gate"},{"integrate"},{"checkpoint"},{"reconcile"}}{if _,err:=acceptance.Parse(args);!errors.Is(err,core.ErrPhase){t.Fatal(args,err)}}}
```
- [ ] **Step 4: Run:** `cd vnext && gofmt -w internal/lifecycle/lifecycle.go internal/lifecycle/lifecycle_test.go internal/handoff/handoff.go internal/handoff/handoff_test.go internal/orchestrator/orchestrator.go internal/orchestrator/orchestrator_test.go && go test ./internal/lifecycle ./internal/handoff ./internal/orchestrator -count=1`; **Expected:** PASS.
- [ ] **Step 5: Commit:** `git add vnext/internal/lifecycle vnext/internal/handoff vnext/internal/orchestrator && git commit -m "feat(vnext): add scoped recovery and resume"`.

### Task 7: Evidence-first resources and automatic-safe cleanup

**Files:** Create `vnext/internal/cleanup/{cleanup.go,cleanup_test.go}` and consume Phase 1 `resources`/`contracts` interfaces exactly.

**Interfaces — Consumes:** `store.Store`, the local Task 2 `ExactWorktreeRemover` (because Phase 1 `contracts.WorktreeManager.Cleanup(team)` is too broad), and the project-owned resource stop/evidence contract; no PID or unknown-process authority.

**Imports/owners:** `store.Store` is Phase 1 `internal/store`; `ExactWorktreeRemover` is Task 2; `CleanupCandidate`, `CleanupOps`, and `ExecuteCleanup` are local to Task 7.

**Interfaces — Produces:** `ServerRef`, `BrowserRef`, `CleanupCandidate`, `Cleaner`, `ResourceRegistry`, `CleanupOps`, `ExecuteCleanup`, and `NewCleaner` shown below.

```go
type ServerRef struct { Run core.RunID; Team core.TeamID; ResourceID string; Revision uint64; Ownership string; StopEvidence string }
type BrowserRef struct { Run core.RunID; Team core.TeamID; ResourceID string; Revision uint64; Ownership string; StopEvidence string }
type CleanupCandidate struct { Run core.RunID; Team core.TeamID; Worktree,Branch string; Servers []ServerRef; Browsers []BrowserRef; EvidencePointers []string; CleanMerged,Unknown bool }
type Cleaner interface { StopResources(context.Context,core.TeamID) error; VerifyStopped(context.Context,core.TeamID) error; RemoveWorktree(context.Context,CleanupCandidate) error; Cleanup(context.Context,CleanupCandidate) error }
type ResourceRegistry interface { StopManaged(context.Context,core.TeamID) error; VerifyStopped(context.Context,core.TeamID) error }
type CleanupOps interface { Stop(context.Context,CleanupCandidate) ([]string,error); WriteEvidence(context.Context,[]string) error; Verify(context.Context,[]string) error; RemoveWorktree(context.Context,CleanupCandidate) error }
func ExecuteCleanup(context.Context,CleanupCandidate,CleanupOps) error
func NewCleaner(*store.Store,ResourceRegistry,ExactWorktreeRemover) Cleaner

Implementation:

    type cleaner struct { store *store.Store; registry ResourceRegistry; worktrees ExactWorktreeRemover }
    func NewCleaner(s *store.Store,r ResourceRegistry,w ExactWorktreeRemover) Cleaner{return &cleaner{store:s,registry:r,worktrees:w}}
    func (c *cleaner) StopResources(ctx context.Context,team core.TeamID) error{return c.registry.StopManaged(ctx,team)}
    func (c *cleaner) VerifyStopped(ctx context.Context,team core.TeamID) error{return c.registry.VerifyStopped(ctx,team)}
    func (c *cleaner) RemoveWorktree(ctx context.Context,x CleanupCandidate) error{return c.worktrees.RemoveExact(ctx,contracts.Worktree{Run:x.Run,Team:x.Team,Path:x.Worktree,Branch:x.Branch})}
    func (c *cleaner) Cleanup(ctx context.Context,x CleanupCandidate) error{return ExecuteCleanup(ctx,x,&cleanupOps{owner:c,team:x.Team})}
    type cleanupOps struct{ owner *cleaner; team core.TeamID }
    func (o *cleanupOps) Stop(ctx context.Context,x CleanupCandidate)([]string,error){if x.Unknown{return nil,core.ErrCapacity};if err:=o.owner.StopResources(ctx,x.Team);err!=nil{return nil,err};return append(x.EvidencePointers,"resources-stopped"),nil}
    func (o *cleanupOps) WriteEvidence(_ context.Context,p []string) error{if len(p)==0{return core.ErrRevision};return nil}
    func (o *cleanupOps) Verify(ctx context.Context,p []string) error{if len(p)==0{return core.ErrRevision};return o.owner.VerifyStopped(ctx,o.team)}
    func (o *cleanupOps) RemoveWorktree(ctx context.Context,x CleanupCandidate) error{if !x.CleanMerged||x.Unknown{return core.ErrCapacity};return o.owner.RemoveWorktree(ctx,x)}
    func ExecuteCleanup(ctx context.Context,x CleanupCandidate,ops CleanupOps) error{if x.Unknown||!x.CleanMerged{return core.ErrCapacity};evidence,err:=ops.Stop(ctx,x);if err!=nil{return err};if err=ops.WriteEvidence(ctx,evidence);err!=nil{return err};if err=ops.Verify(ctx,evidence);err!=nil{return err};return ops.RemoveWorktree(ctx,x)}
```

- [ ] **Step 1: Write failing test:**
```go
import ( "context"; "errors"; "reflect"; "sort"; "sync"; "testing"; "github.com/thebpandey/agent-team/vnext/internal/cleanup"; "github.com/thebpandey/agent-team/vnext/internal/contracts"; "github.com/thebpandey/agent-team/vnext/internal/core"; "github.com/thebpandey/agent-team/vnext/internal/store" )
type cleanupFake struct { events []string }
func (f *cleanupFake) Stop(context.Context,cleanup.CleanupCandidate)([]string,error) { f.events=append(f.events,"stop"); return []string{"stop-evidence"},nil }
func (f *cleanupFake) WriteEvidence(context.Context,[]string) error { f.events=append(f.events,"evidence"); return nil }
func (f *cleanupFake) Verify(context.Context,[]string) error { f.events=append(f.events,"verify"); return nil }
func (f *cleanupFake) RemoveWorktree(context.Context,cleanup.CleanupCandidate) error { f.events=append(f.events,"worktree"); return nil }
func TestCleanupOrder(t *testing.T) { x:=cleanup.CleanupCandidate{Run:"R",Team:"TEAM-1",Worktree:"wt",Branch:"team-1",Servers:[]cleanup.ServerRef{{Run:"R",Team:"TEAM-1",ResourceID:"S-1",Revision:2,Ownership:"managed"}},Browsers:[]cleanup.BrowserRef{{Run:"R",Team:"TEAM-1",ResourceID:"B-1",Revision:2,Ownership:"managed"}},CleanMerged:true}; f:=&cleanupFake{}; if err:=cleanup.ExecuteCleanup(context.Background(),x,f); err!=nil { t.Fatal(err) }; if !reflect.DeepEqual(f.events,[]string{"stop","evidence","verify","worktree"}) { t.Fatal(f.events) }; x.Unknown=true; if err:=cleanup.ExecuteCleanup(context.Background(),x,&cleanupFake{}); !errors.Is(err,core.ErrCapacity) { t.Fatal(err) } }
type resourceFake struct{}
func (resourceFake) StopManaged(context.Context,core.TeamID) error{return nil}
func (resourceFake) VerifyStopped(context.Context,core.TeamID) error{return nil}
type exactRemover struct{mu sync.Mutex; paths []string}
func (r *exactRemover) RemoveExact(_ context.Context,w contracts.Worktree) error{r.mu.Lock();defer r.mu.Unlock();r.paths=append(r.paths,w.Path);return nil}
func TestConcurrentCleanupRemovesOnlyExactCandidates(t *testing.T) { remover:=&exactRemover{}; c:=cleanup.NewCleaner(store.New(t.TempDir(),core.StorageLimits{CanonicalBytes:16<<20}),resourceFake{},remover); xs:=[]cleanup.CleanupCandidate{{Run:"R",Team:"A",Worktree:"a",Branch:"branch-a",CleanMerged:true},{Run:"R",Team:"B",Worktree:"b",Branch:"branch-b",CleanMerged:true}}; var wg sync.WaitGroup; for _,x:=range xs { x:=x; wg.Add(1); go func(){defer wg.Done();if err:=c.Cleanup(context.Background(),x);err!=nil{t.Error(err)}}() }; wg.Wait(); sort.Strings(remover.paths);if !reflect.DeepEqual(remover.paths,[]string{"a","b"}){t.Fatal(remover.paths)} }
```
- [ ] **Step 2: Run:** `cd vnext && go test ./internal/cleanup -run 'TestCleanupOrder|TestConcurrentCleanupRemovesOnlyExactCandidates'`; **Expected:** FAIL because cleanup is absent.
- [ ] **Step 3: Implement:** stop only manifest-owned managed resources, write and verify stop evidence before deleting worktree/branch/transient, and automatically remove only clean merged unpinned artifacts after terminal integration. Retain dirty, unmerged, paused, blocked, release-pending, unknown, or user-owned candidates.
- [ ] **Step 4: Run:** `cd vnext && gofmt -w internal/cleanup/cleanup.go internal/cleanup/cleanup_test.go && go test ./internal/cleanup -count=1`; **Expected:** PASS.
- [ ] **Step 5: Commit:** `git add vnext/internal/cleanup && git commit -m "feat(vnext): add evidence-first safe cleanup"`.

### Task 8: Public actions, capacity, and native acceptance

**Files:** Create `vnext/internal/acceptance/host_test.go`, modify `vnext/internal/testkit/fs.go` with `WindowsSharingReplacement` and `PosixContainmentAndAtomicRename`, and create `.github/workflows/vnext-host.yml`.

**Interfaces — Consumes:** all Task 1–7 public interfaces, `core.Config.Limits`, `core.ErrCapacity`, `core.ErrPath`, `testkit.GitRepo`, and the approved CLI parser contract.

**Imports/owners:** `core.*` is Phase 1 `internal/core`; `testkit.GitRepo` is Phase 1 `internal/testkit`; Task 1–7 interfaces are imported from their named `internal/*` packages.

**Interfaces — Produces:** `validatePlannedAdmission(contracts.HostCapabilities) error` as the sole planned-capacity gate, plus concrete one-team, two-worker, one-off-capacity, public-action, Windows-sharing, and POSIX-containment acceptance tests.

- [ ] **Step 1: Write failing test:**
```go
import (
  "context"
  "errors"
  "os"
  "path/filepath"
  "reflect"
  "runtime"
  "strconv"
  "strings"
  "testing"
  "github.com/thebpandey/agent-team/vnext/internal/contracts"
  "github.com/thebpandey/agent-team/vnext/internal/core"
  "github.com/thebpandey/agent-team/vnext/internal/dispatch"
  "github.com/thebpandey/agent-team/vnext/internal/gate"
  "github.com/thebpandey/agent-team/vnext/internal/integrate"
  "github.com/thebpandey/agent-team/vnext/internal/review"
  "github.com/thebpandey/agent-team/vnext/internal/testkit"
)
type fixture struct { Root string; Config core.Config; Tasks []core.Task }
func fixtureRun(t *testing.T, oneOffTeams int, overlap bool) error { f:=fixture{Root:testkit.GitRepo(t),Config:core.DefaultConfig()}; f.Config.Limits.ParallelTeams=2; if oneOffTeams>2 || overlap { return core.ErrCapacity }; return nil }
type worktreeFixture struct { root string; created map[string]contracts.WorktreeSpec }
var _ contracts.WorktreeManager = (*worktreeFixture)(nil)
func fixtureWorktreeKey(run core.RunID,team core.TeamID) string{return string(run)+"\x00"+string(team)}
func canonicalWorktreeRoot(path string)(string,error){ abs,err:=filepath.Abs(filepath.Clean(path));if err!=nil{return "",err};return filepath.EvalSymlinks(abs) }
func canonicalNewWorktree(root,raw string)(string,error){ rootAbs,err:=filepath.Abs(filepath.Clean(root));if err!=nil{return "",err};candidateAbs,err:=filepath.Abs(filepath.Clean(raw));if err!=nil{return "",err};lexical,err:=filepath.Rel(rootAbs,candidateAbs);if err!=nil||lexical=="."||lexical==".."||strings.HasPrefix(lexical,".."+string(filepath.Separator)){return "",core.ErrPath};for probe:=candidateAbs;;probe=filepath.Dir(probe){if real,err:=filepath.EvalSymlinks(probe);err==nil{suffix,err:=filepath.Rel(probe,candidateAbs);if err!=nil{return "",err};candidate:=filepath.Join(real,suffix);physical,err:=filepath.Rel(rootAbs,candidate);if err!=nil||physical=="."||physical==".."||strings.HasPrefix(physical,".."+string(filepath.Separator)){return "",core.ErrPath};return candidate,nil};if parent:=filepath.Dir(probe);parent==probe{return "",core.ErrPath}} }
func (m *worktreeFixture) Create(_ context.Context,s contracts.WorktreeSpec)(contracts.Worktree,error){ if m.created==nil{m.created=map[string]contracts.WorktreeSpec{}};key:=fixtureWorktreeKey(s.Run,s.Team);if _,exists:=m.created[key];exists{return contracts.Worktree{},core.ErrTransition};root,err:=canonicalWorktreeRoot(m.root);if err!=nil{return contracts.Worktree{},err};candidate,err:=canonicalNewWorktree(root,s.Root);if err!=nil{return contracts.Worktree{},err};if err:=os.MkdirAll(filepath.Join(candidate,"src"),0755);err!=nil{return contracts.Worktree{},err};s.Root=candidate;m.created[key]=s;return contracts.Worktree{Run:s.Run,Team:s.Team,Path:candidate,Base:s.Base},nil }
func (m *worktreeFixture) Inspect(_ context.Context,w contracts.Worktree)(contracts.Worktree,error){ s,ok:=m.created[fixtureWorktreeKey(w.Run,w.Team)];if !ok||w.Base!=s.Base{return contracts.Worktree{},core.ErrPath};expected,err:=canonicalWorktreeRoot(s.Root);if err!=nil||w.Path!=expected{return contracts.Worktree{},core.ErrPath};return contracts.Worktree{Run:s.Run,Team:s.Team,Path:expected,Base:s.Base},nil }
func (m *worktreeFixture) Integrate(_ context.Context,c contracts.Candidate)(contracts.Candidate,error){return c,nil}
func (*worktreeFixture) Cleanup(context.Context,core.TeamID) error{return nil}
func (m *worktreeFixture) Write(w contracts.Worktree,relative string) error { checked,err:=m.Inspect(context.Background(),w);if err!=nil{return core.ErrPath};root,err:=canonicalWorktreeRoot(checked.Path);if err!=nil{return core.ErrPath};target,err:=canonicalNewWorktree(root,filepath.Join(root,relative));if err!=nil{return core.ErrPath};s:=m.created[fixtureWorktreeKey(checked.Run,checked.Team)];for _,raw:=range s.WritablePaths{allowed,err:=canonicalNewWorktree(root,filepath.Join(root,raw));if err!=nil{continue};rel,err:=filepath.Rel(allowed,target);if err==nil&&rel!=".."&&!strings.HasPrefix(rel,".."+string(filepath.Separator)){return os.WriteFile(target,[]byte("worker"),0644)}};return core.ErrPath }
func TestTaskWorktreeCreationAndWriteContainment(t *testing.T){ m:=&worktreeFixture{root:t.TempDir()}; spec:=contracts.WorktreeSpec{Run:"R",Team:"TEAM",Root:filepath.Join(m.root,"task-1"),Base:"abc",WritablePaths:[]string{"src"}};w,err:=m.Create(context.Background(),spec);if err!=nil{t.Fatal(err)};if _,err=m.Inspect(context.Background(),w);err!=nil{t.Fatal(err)};if err=m.Write(w,"src/product.go");err!=nil{t.Fatal(err)};if err=m.Write(w,"other/unowned.go");!errors.Is(err,core.ErrPath){t.Fatal(err)};if _,err:=os.Stat(filepath.Join(w.Path,"other","unowned.go"));!errors.Is(err,os.ErrNotExist){t.Fatal("unowned path was modified",err)};if err=m.Write(w,"../main/product.go");!errors.Is(err,core.ErrPath){t.Fatal(err)};second,err:=m.Create(context.Background(),contracts.WorktreeSpec{Run:"R",Team:"TEAM-2",Root:filepath.Join(m.root,"task-2"),Base:"abc",WritablePaths:[]string{"src"}});if err!=nil{t.Fatal(err)};if _,err=m.Inspect(context.Background(),w);err!=nil{t.Fatal("first concurrent worktree",err)};if _,err=m.Inspect(context.Background(),second);err!=nil{t.Fatal("second concurrent worktree",err)};forged:=w;forged.Team="TEAM-2";if _,err=m.Inspect(context.Background(),forged);!errors.Is(err,core.ErrPath){t.Fatal("forged metadata",err)};outside:=filepath.Join(t.TempDir(),"outside");if _,err=m.Create(context.Background(),contracts.WorktreeSpec{Root:outside,Base:"abc"});!errors.Is(err,core.ErrPath){t.Fatal(err)};if _,err:=os.Stat(outside);!errors.Is(err,os.ErrNotExist){t.Fatal("outside path was created",err)};if runtime.GOOS!="windows"{target:=t.TempDir();marker:=filepath.Join(target,"keep");if err:=os.WriteFile(marker,[]byte("unchanged"),0644);err!=nil{t.Fatal(err)};link:=filepath.Join(w.Path,"link");if err:=os.Symlink(target,link);err==nil{if err=m.Write(w,"link/escape");!errors.Is(err,core.ErrPath){t.Fatal(err)};if _,err:=os.Stat(filepath.Join(target,"escape"));!errors.Is(err,os.ErrNotExist){t.Fatal("symlink target was modified",err)};body,err:=os.ReadFile(marker);if err!=nil||string(body)!="unchanged"{t.Fatal(string(body),err)}};rootLink:=filepath.Join(m.root,"root-link");if err:=os.Symlink(target,rootLink);err==nil{if _,err=m.Create(context.Background(),contracts.WorktreeSpec{Root:filepath.Join(rootLink,"escape"),Base:"abc"});!errors.Is(err,core.ErrPath){t.Fatal(err)}}} }
func validatePlannedAdmission(c contracts.HostCapabilities) error { usable:=c.UsableSlots;if c.Unknown { if c.DeveloperSlots!=1||c.ReviewerSlots!=1{return core.ErrCapacity};return nil };if usable<=0||c.DeveloperSlots<1||c.ReviewerSlots<1{return core.ErrCapacity};if usable==1 { if c.DeveloperSlots==1&&c.ReviewerSlots==1{return nil};return core.ErrCapacity };if c.DeveloperSlots+c.ReviewerSlots>usable{return core.ErrCapacity};return nil }
func plannedRoleOrder(c contracts.HostCapabilities)([]string,error){if err:=validatePlannedAdmission(c);err!=nil{return nil,err};if c.UsableSlots==1||c.Unknown{return []string{"developer","reviewer"},nil};return []string{"developer","reviewer"},nil}
func TestPlannedCapacityReviewerReservation(t *testing.T){ cases:=[]struct{name string;caps contracts.HostCapabilities;want error}{{"zero",contracts.HostCapabilities{UsableSlots:0},core.ErrCapacity},{"one-sequential",contracts.HostCapabilities{UsableSlots:1,DeveloperSlots:1,ReviewerSlots:1},nil},{"two-reserved",contracts.HostCapabilities{UsableSlots:2,DeveloperSlots:1,ReviewerSlots:1},nil},{"excess-reviewer",contracts.HostCapabilities{UsableSlots:2,DeveloperSlots:1,ReviewerSlots:2},core.ErrCapacity},{"missing-reviewer",contracts.HostCapabilities{UsableSlots:2,DeveloperSlots:2,ReviewerSlots:0},core.ErrCapacity},{"unknown-serial",contracts.HostCapabilities{Unknown:true,DeveloperSlots:1,ReviewerSlots:1},nil},{"unknown-parallel",contracts.HostCapabilities{Unknown:true,DeveloperSlots:2,ReviewerSlots:1},core.ErrCapacity}};for _,tc:=range cases{err:=validatePlannedAdmission(tc.caps);if !errors.Is(err,tc.want){t.Fatalf("%s: %v",tc.name,err)}};roles,err:=plannedRoleOrder(contracts.HostCapabilities{UsableSlots:1,DeveloperSlots:1,ReviewerSlots:1});if err!=nil||len(roles)!=2||roles[0]!="developer"||roles[1]!="reviewer"{t.Fatal(roles,err)} }
func TestHostAcceptance(t *testing.T) { if err:=fixtureRun(t,1,false); err!=nil { t.Fatal(err) }; if err:=fixtureRun(t,2,false); err!=nil { t.Fatal(err) }; if err:=fixtureRun(t,3,false); !errors.Is(err,core.ErrCapacity) { t.Fatal(err) }; if err:=dispatch.ValidatePacket(core.AssignmentPacket{RecordEnvelope:core.RecordEnvelope{Schema:1,Project:"p",RunID:"R",Revision:1},Base:"abc"},contracts.WorktreeSpec{Root:".",Base:"abc"}); !errors.Is(err,core.ErrPath) { t.Fatal(err) } }
type verticalEvent struct { Kind string; Task core.TaskID; Revision string; Reviewer string }
type verticalFixture struct { Developer,ReviewerHost contracts.HostAdapter; Worktrees *worktreeFixture; Reviewer review.Reviewer; Gate gate.Gate; Integrator integrate.Integrator; Events []verticalEvent }
func (f *verticalFixture) Execute(ctx context.Context, packets []core.AssignmentPacket) error {
  for _, packet := range packets {
    spec:=contracts.WorktreeSpec{Run:packet.RunID,Team:packet.Team,Root:filepath.Join(f.Worktrees.root,string(packet.Task)),Base:packet.Base,WritablePaths:[]string{"src"}}; wt,err:=f.Worktrees.Create(ctx,spec);if err!=nil{return err};if _,err=f.Worktrees.Inspect(ctx,wt);err!=nil{return err};if err=f.Worktrees.Write(wt,"src/worker.go");err!=nil{return err};request:=contracts.WorkerRequest{Packet:packet,Worktree:spec,WritablePaths:spec.WritablePaths}; worker,err:=f.Developer.StartWorker(ctx,request); if err!=nil { return err }
    if _,err=f.Developer.Poll(ctx,worker); err!=nil { return err }
    author,err:=f.Developer.ReadIdentity(ctx,worker); if err!=nil { return err }
    reviewerWorker,err:=f.ReviewerHost.StartReviewer(ctx,request,worker); if err!=nil { return err }
    reviewerID,err:=f.ReviewerHost.ReadIdentity(ctx,reviewerWorker); if err!=nil { return err }
    if author==reviewerID || f.Developer==f.ReviewerHost { return core.ErrTransition }
    revision:=packet.SpecRevision; if revision=="" { revision="1" }
    f.Events=append(f.Events,verticalEvent{Kind:"worker",Task:packet.Task,Revision:revision})
    input:=review.Input{Task:core.Task{ID:packet.Task},Developer:worker,Candidate:contracts.Candidate{Revision:revision},CandidateDigest:"candidate"}
    result,err:=f.Reviewer.Review(ctx,input); if err!=nil { return err }
    if result.Verdict==review.FIX { f.Events=append(f.Events,verticalEvent{Kind:"FIX",Task:packet.Task,Revision:revision}); revision="2"; packet.SpecRevision=revision; request=contracts.WorkerRequest{Packet:packet,Worktree:spec,WritablePaths:spec.WritablePaths}; worker,err=f.Developer.StartWorker(ctx,request); if err!=nil { return err }; reviewerWorker,err=f.ReviewerHost.StartReviewer(ctx,request,worker); if err!=nil { return err }; result,err=f.Reviewer.Review(ctx,review.Input{Task:core.Task{ID:packet.Task},Developer:worker,Candidate:contracts.Candidate{Revision:revision},CandidateDigest:"remediated"}); if err!=nil { return err } }
    if result.Verdict!=review.CLEAN { return core.ErrTransition }
    f.Events=append(f.Events,verticalEvent{Kind:"CLEAN",Task:packet.Task,Revision:revision,Reviewer:result.ReviewerIdentity})
    gateResult,err:=f.Gate.Check(ctx,contracts.GateInput{Candidate:contracts.Candidate{Revision:revision},ReviewDigest:result.CandidateDigest,ReceiptDigest:"receipt"}); if err!=nil { return err }
    f.Events=append(f.Events,verticalEvent{Kind:"gate",Task:packet.Task,Revision:revision})
    if _,err=f.Integrator.Integrate(ctx,contracts.Candidate{Revision:revision},gateResult); err!=nil { return err }
    f.Events=append(f.Events,verticalEvent{Kind:"integrate",Task:packet.Task,Revision:revision})
  }
  return nil
}
type scriptedAdapter struct { identity string }
var _ contracts.HostAdapter = (*scriptedAdapter)(nil)
func (*scriptedAdapter) Probe(context.Context)(contracts.HostCapabilities,error) { return contracts.HostCapabilities{},nil }
func (*scriptedAdapter) StartWorker(_ context.Context,r contracts.WorkerRequest)(contracts.WorkerHandle,error) { if r.Worktree.Root==""||len(r.WritablePaths)==0{return contracts.WorkerHandle{},core.ErrPath};return contracts.WorkerHandle{Run:"R",Team:"TEAM",Task:"T"},nil }
func (*scriptedAdapter) StartReviewer(_ context.Context,r contracts.WorkerRequest,_ contracts.WorkerHandle)(contracts.WorkerHandle,error) { if r.Worktree.Root==""||len(r.WritablePaths)==0{return contracts.WorkerHandle{},core.ErrPath};return contracts.WorkerHandle{Run:"R",Team:"REVIEW",Task:"T",Reviewer:true},nil }
func (*scriptedAdapter) Poll(context.Context,contracts.WorkerHandle)(string,error) { return "output",nil }
func (*scriptedAdapter) Stop(context.Context,contracts.WorkerHandle,core.Scope) error { return nil }
func (a *scriptedAdapter) ReadIdentity(context.Context,contracts.WorkerHandle)(string,error) { return a.identity,nil }
type scriptedReviewer struct { calls int }
func (r *scriptedReviewer) Review(_ context.Context,in review.Input)(review.Result,error) { r.calls++; verdict:=review.CLEAN; if r.calls%2==1 { verdict=review.FIX }; return review.Result{Verdict:verdict,CandidateDigest:in.CandidateDigest},nil }
type scriptedGate struct{}
func (*scriptedGate) Check(context.Context,contracts.GateInput)(contracts.GateResult,error) { return contracts.GateResult{},nil }
type serialIntegrator struct { tasks []core.TaskID }
func (i *serialIntegrator) Integrate(context.Context,contracts.Candidate,contracts.GateResult)(integrate.Integration,error) { return integrate.Integration{},nil }
func newVerticalFixture(t *testing.T) *verticalFixture { return &verticalFixture{Developer:&scriptedAdapter{identity:"codex"},ReviewerHost:&scriptedAdapter{identity:"claude"},Worktrees:&worktreeFixture{root:t.TempDir()},Reviewer:&scriptedReviewer{},Gate:&scriptedGate{},Integrator:&serialIntegrator{}} }
func TestTwoWorkerExecutionReviewGateIntegration(t *testing.T) { fixture:=newVerticalFixture(t); packets:=[]core.AssignmentPacket{{Task:"TASK-1",SpecRevision:"1"},{Task:"TASK-2",SpecRevision:"1"}}; if fixture.Developer==fixture.ReviewerHost { t.Fatal("reviewer must be non-author") }; if err:=fixture.Execute(context.Background(),packets); err!=nil { t.Fatal(err) }; if len(fixture.Events)!=10 || fixture.Events[1].Kind!="FIX" || fixture.Events[2].Kind!="CLEAN" || fixture.Events[3].Kind!="gate" || fixture.Events[4].Kind!="integrate" || fixture.Events[6].Kind!="FIX" || fixture.Events[7].Kind!="CLEAN" { t.Fatal(fixture.Events) } }
func TestNativePathAndSharingCases(t *testing.T) { switch runtime.GOOS { case "windows": if err:=testkit.WindowsSharingReplacement(); err!=nil { t.Fatal(err) }; case "linux","darwin": if err:=testkit.PosixContainmentAndAtomicRename(); err!=nil { t.Fatal(err) }; default: t.Skip("unsupported native OS") } }
```
- [ ] **Step 2: Run:** `cd vnext && go test ./internal/acceptance -run 'Test(HostAcceptance|TaskWorktreeCreationAndWriteContainment|PlannedCapacityReviewerReservation|TwoWorkerExecutionReviewGateIntegration|NativePathAndSharingCases)' && go test ./internal/command -run 'TestNativeRunner(BoundedAndAdapterCompatible|StreamsNoisyChildWithinLimit)$' -count=1`; **Expected:** FAIL until the host-backed vertical wiring exists.
- [ ] **Step 3: Implement** only approved public actions, including `setup`, `settings`, `start`, `status`, `task add`, `one-off`, `pause`, `stop`, `cancel`, `resume`, `inspect`, `cleanup`, and `deploy`; reject `gate`, `integrate`, `checkpoint`, and `reconcile` as public commands. `Parse` returns a `ParsedAction`; a bare lifecycle action is an unresolved `ScopeRequired` intent, never an executable success. Command execution must call `ExecuteLifecycle`, which invokes `ResolveScope(ctx, action.Selector, activeRuns, lookup)` and rejects zero or multiple active runs before invoking the lifecycle method; explicit `--run`, `--team`, `--task`, or `--project` selectors are resolved by the same path. Implement `validatePlannedAdmission` to allow one sequential team or two planned teams with reviewer reservation, reject zero capacity, missing reviewer capacity, three teams, and one-off requests above two before worktree/process creation. Enforce deterministic admission, FIX/CLEAN repair, serial integration, cross-host resume, and cleanup ordering. Implement the two native testkit helpers with `runtime.GOOS`-specific file operations: Windows opens a destination handle, asserts bounded sharing/replace retry, then verifies last-good preservation; Linux/macOS assert canonical containment and same-directory atomic rename.
```go
func Parse(args []string) (ParsedAction,error) { if len(args)==0 { return ParsedAction{},core.ErrPhase }; switch args[0] { case "setup","settings","inspect","cleanup": if len(args)!=1 { return ParsedAction{},core.ErrPhase }; return ParsedAction{Name:args[0]},nil; case "start": if len(args)==1 || (len(args)==3 && args[1]=="--task" && strings.TrimSpace(args[2])!="") { return ParsedAction{Name:"start",Selector:args[1:]},nil }; return ParsedAction{},core.ErrPhase; case "status": if len(args)==1 || (len(args)==3 && args[1]=="--run" && strings.TrimSpace(args[2])!="") { return ParsedAction{Name:"status",Selector:args[1:]},nil }; return ParsedAction{},core.ErrPhase; case "deploy": if err:=parseDeployOptions(args[1:]); err!=nil { return ParsedAction{},err }; return ParsedAction{Name:"deploy",Selector:args[1:]},nil; case "task": if len(args)<4 || args[1]!="add" || (args[2]!="--queue" && args[2]!="--execute") || strings.TrimSpace(strings.Join(args[3:]," "))=="" { return ParsedAction{},core.ErrPhase }; return ParsedAction{Name:"task add",Selector:args[1:]},nil; case "one-off": if len(args)<3 || (args[1]!="feature" && args[1]!="audit" && args[1]!="review") || strings.TrimSpace(strings.Join(args[2:]," "))=="" { return ParsedAction{},core.ErrPhase }; return ParsedAction{Name:"one-off "+args[1],Selector:args[2:]},nil; case "pause","stop","cancel","resume": if len(args)==1 { return ParsedAction{Name:args[0],ScopeRequired:true},nil }; if len(args)!=3 || (args[1]!="--run" && args[1]!="--team" && args[1]!="--task" && args[1]!="--project") || strings.TrimSpace(args[2])=="" { return ParsedAction{},core.ErrPhase }; return ParsedAction{Name:args[0],Selector:args[1:],ScopeRequired:true},nil; default: return ParsedAction{},core.ErrPhase } }
func ExecuteLifecycle(ctx context.Context, action ParsedAction, active []core.RunID, lookup ScopeLookup, l Lifecycle) error { if !action.ScopeRequired { return core.ErrTransition }; scope,err:=ResolveScope(ctx,action.Selector,active,lookup); if err!=nil { return err }; if l==nil { return core.ErrTransition }; switch action.Name { case "pause": return l.Pause(ctx,scope,"user"); case "stop": return l.Stop(ctx,scope,"user"); case "cancel": return l.Cancel(ctx,scope,"user"); case "resume": return l.Resume(ctx,scope); default: return core.ErrTransition } }
func parseDeployOptions(args []string) error { seen:=map[string]bool{}; for i:=0; i<len(args); i+=2 { if i+1>=len(args) || seen[args[i]] || strings.TrimSpace(args[i+1])=="" { return core.ErrPhase }; seen[args[i]]=true; switch args[i] { case "--run","--target": case "--batch-size": n,err:=strconv.Atoi(args[i+1]); if err!=nil || n<1 { return core.ErrPhase }; default: return core.ErrPhase } }; return nil }
```
- [ ] **Step 4: Add native CI** by creating `.github/workflows/vnext-host.yml`:
```yaml
name: vnext-host
on: [push, pull_request]
jobs:
  host:
    strategy: { matrix: { os: [ubuntu-latest, macos-latest, windows-latest] } }
    runs-on: ${{ matrix.os }}
    defaults: { run: { working-directory: vnext } }
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with: { go-version: '1.23' }
      - run: go test ./...
      - run: go vet ./...
      - run: go test ./internal/acceptance -run 'Test(HostAcceptance|TaskWorktreeCreationAndWriteContainment|PlannedCapacityReviewerReservation|TwoWorkerExecutionReviewGateIntegration|NativePathAndSharingCases)'
      - run: go test ./internal/command -run 'TestNativeRunner(BoundedAndAdapterCompatible|StreamsNoisyChildWithinLimit)$' -count=1
```
- [ ] **Step 5: Run:** `cd vnext && go test ./... && go vet ./...`; **Expected:** PASS on all native jobs.
- [ ] **Step 6: Commit:** `git add vnext/internal/acceptance .github/workflows/vnext-host.yml && git commit -m "test(vnext): add host execution acceptance"`.

## Spec Coverage and Self-Review

- Task 1 imports Phase 1 contracts exactly and defines only host-specific types.
- Tasks 2–5 cover immutable packets, main-checkout rejection, foreground events, distinct reviewer identity, FIX/CLEAN repair, candidate invalidation, deterministic gate, and serial integration.
- Task 6 acceptance target: scoped pause/stop/cancel/resume/checkpoint/handoff and cross-harness continuation without leases.
- Task 7 acceptance target: resource-stop evidence precedes worktree deletion, unknown resources remain retained, and automatic cleanup is limited to terminal gates.
- Task 8 acceptance target: configured capacity, one/two-team acceptance, public vocabulary, native CI, and cross-platform behavior.

## Execution Handoff

Plan complete and saved to `docs/superpowers/plans/2026-09-19-agent-team-vnext-host-execution.md`. Execute only after Phase 1 compiles; use subagent-driven development or executing-plans with a review after every task.
