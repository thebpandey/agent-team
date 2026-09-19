# Agent-Team vNext Optional Capabilities Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add explicit-consent, cross-platform optional capability adapters, bounded managed server/browser resources, and a local last-good dashboard without making any tool, MCP, hook, or skill a core dependency.

**Architecture:** Extend the Phase 1 Go module with capability detection, consent/install planning, task-scoped routing, resource registries, and an atomic dashboard renderer. Adapters execute only verified argument arrays, record short outcome metrics in existing receipts, and always have a native fallback. The router starts with one analytical accelerator per task and permits a second only for a named unanswered question.

**Tech Stack:** Go standard library, existing `vnext/internal/{core,store,knowledge,contracts}` packages, native process invocation, UTF-8 JSON/Markdown/HTML, `go test`, `go vet`; no Node.js, hooks, daemons, third-party Go packages, or automatic MCP/skill registration.

**Spec:** `docs/superpowers/specs/2026-09-18-agent-team-vnext-design.md`

## Global Constraints

- Optional capability use is explicit, task-scoped, revision-bound, non-authoritative, and nonblocking; native Git/search/project checks remain the fallback.
- A fresh session reads the common `using-superpowers` contract once; each task selects only the skills it needs, reads each selected skill completely, records path/digest, and invalidates reuse when path/digest changes. Role-specific calls use only task-scoped skill references. Visual work requires the Impeccable + UI-UX-Pro-Max pair; browser evidence requires a named session and managed resource record.
- Setup presents one consent table showing detected version/path, installer package and source, files/settings changed, rollback path, and post-install probe. Decline, unsupported, unhealthy, or failed installation falls back without host mutation.
- LeanCTX is CLI-only compression of selected discovery/build/test output; it never owns memory, tasks, handoffs, receipts, coordination, or authority.
- Serena is an explicit read-only MCP choice with an allowlist of navigation/search operations; no edit, command, memory, project-switch, handoff, or arbitrary-tool operation is exposed.
- Graphify is code-only and per-worktree/revision (`extract --code-only --no-viz`); no graph sharing, visualization, semantic/document extraction, MCP, hooks, memory, or automatic rebuild.
- Visual work always receives Impeccable plus `ui-ux-pro-max`; `ui-styling` is an optional third task-scoped capability and never a substitute.
- At most two Agent-Team-managed servers and two browser sessions exist per project. Unknown or user-owned resources are retained and reported, never killed.
- Dashboard is a local-only atomic snapshot at `<project>/.agent-team/dashboard/index.html`; refresh occurs once after successful integration and preserves the last-good snapshot on failure.
- All adapters use Go argument arrays and platform-native process/filesystem APIs. Tests cover Windows, macOS, and Linux; no `/proc`, `flock`, `bash`, `$HOME`, POSIX signals, symlink assumptions, or executable-bit assumptions.
- No secrets, transcripts, API keys, telemetry, raw large output, or private source are written to receipts.

## File Map

| Path | Responsibility |
| --- | --- |
| `vnext/internal/capability/{types.go,probe.go,consent.go,install.go,router.go}` | Capability schema, detection, explicit consent/install plan, routing, fallback, and accelerator budgets. |
| `vnext/internal/capability/*_test.go`, `vnext/testdata/capabilities/**` | Deterministic probes, fake runners, installer rollback, forbidden-operation, and routing fixtures. |
| `vnext/internal/resources/{registry.go,server.go,browser.go}` | Project-scoped managed server/browser records, caps, ownership, unknown retention, and lifecycle. |
| `vnext/internal/resources/*_test.go` | Two-resource caps, collision serialization, stale/unknown ownership, and Windows lifecycle tests. |
| `vnext/internal/dashboard/{render.go,template.go,render_test.go}` | Local dashboard model, bounded HTML rendering, atomic last-good publication, and failure status. |
| `vnext/internal/bench/{bench.go,bench_test.go}` | 20–30 fixture benchmark harness comparing native and selected optional routes. |
| `vnext/internal/acceptance/optional_test.go` | Cross-platform acceptance of consent, fallbacks, resources, visual routing, and dashboard refresh. |
| `vnext/internal/testkit/{native.go,optional.go}` | Concrete deterministic runners, stores, renderers, and Git fixtures shared by this plan's tests. |

### Task 1: Capability contracts, probes, consent, and bounded installation

**Files:** Create `vnext/internal/capability/{types.go,probe.go,consent.go,install.go,capability_test.go}` and modify `vnext/internal/core/{types.go,config.go}`.

**Interfaces: Consumes** Phase 1 `core`, `store`, `tracker`, and `contracts`; **Produces** capability probes, consent/install records, and the Task 1 testkit helpers:

```go
type Name string
const ( Native Name = "native"; UsingSuperpowers Name = "using-superpowers"; LeanCTX Name = "leanctx"; Serena Name = "serena"; Graphify Name = "graphify"; Playwright Name = "playwright"; Impeccable Name = "impeccable"; UIUXProMax Name = "ui-ux-pro-max"; UIStyling Name = "ui-styling" )
type Mode string
const ( CLI Mode = "cli"; ReadOnlyMCP Mode = "readonly-mcp"; SkillContent Mode = "skill-content" )
type Probe struct { Name Name; Mode Mode; Path,Version,Digest string; Available,Healthy bool; Reason string }
type Consent struct { Name Name; Enabled bool; Mode Mode; InstallerPackage,Source,Rollback string }
type OwnedFile struct { Path,Role,SHA256 string }
type RollbackEntry struct { Path,SHA256 string }
type InstallPlan struct { Name Name; Package,Source,VerifiedVersion,Scope string; OwnedFiles []OwnedFile; SettingsChanged []string; Rollback []RollbackEntry; Command []string; Explicit bool }
type NativeResult struct { Stdout,Stderr []byte; Exit int; TimedOut bool; Transport error }
type NativeRunner interface { Run(context.Context,[]string,[]string) NativeResult }
type OutputStore interface { Write(context.Context,string,[]byte,int64) (string,error) }
type TokenSource interface { Read(context.Context,string) (int,error) }
// vnext/internal/testkit provides concrete FakeNativeRunner (records argv/env and returns NativeResult{Exit:0}),
// FakeOutputStore (bounded in-memory pointers), FakeTokenSource (map-backed counts), FakeResultWriter,
// FakeRenderer, GitRepo (canonical temporary Git repo), and FakeTrackerRunner using Phase 1 tracker.CommandResult.
func ProbeAll(context.Context,NativeRunner,[]Name) ([]Probe,error)
func BuildInstallPlan(Probe,Consent) (InstallPlan,error)
func Install(context.Context,NativeRunner,InstallPlan) (Probe,error)
func Rollback(context.Context,NativeRunner,InstallPlan) error
```

**Interfaces: Consumes** Phase 1 `tracker.CommandResult` only through `FakeTrackerRunner`; **Produces** `NativeResult`, `NativeRunner`, capability records, and concrete testkit helpers consumed by Tasks 2–6.

```go
// vnext/internal/testkit/optional.go
package testkit

import (
  "context"
  "os/exec"
  "testing"
  "github.com/thebpandey/agent-team/vnext/internal/capability"
  "github.com/thebpandey/agent-team/vnext/internal/dashboard"
  "github.com/thebpandey/agent-team/vnext/internal/tracker"
)
type NativeFake struct { Calls [][]string; Result capability.NativeResult }
func (f *NativeFake) Run(_ context.Context, argv, env []string) capability.NativeResult { f.Calls = append(f.Calls, append([]string{}, argv...)); return f.Result }
func FakeNativeRunner() capability.NativeRunner { return &NativeFake{Result: capability.NativeResult{Exit: 0}} }
type OutputFake struct { Writes map[string][]byte }
func (f *OutputFake) Write(_ context.Context, name string, data []byte, _ int64) (string,error) { if f.Writes == nil { f.Writes = map[string][]byte{} }; f.Writes[name] = append([]byte{}, data...); return "memory:"+name,nil }
func FakeOutputStore() capability.OutputStore { return &OutputFake{} }
type TokenFake struct { Counts map[string]int }
func (f TokenFake) Read(_ context.Context, key string) (int,error) { return f.Counts[key],nil }
func FakeTokenSource() capability.TokenSource { return TokenFake{Counts: map[string]int{}} }
type ReceiptFake struct { Receipts []capability.DashboardRefreshReceipt }
func (f *ReceiptFake) WriteRefreshReceipt(_ context.Context, r capability.DashboardRefreshReceipt) error { f.Receipts = append(f.Receipts, r); return nil }
func (f *ReceiptFake) Count() int { return len(f.Receipts) }
func FakeReceiptWriter() *ReceiptFake { return &ReceiptFake{} }
type RendererFake struct { Published []dashboard.Snapshot; Last dashboard.DashboardStatus; RenderErr,PublishErr error }
func (f *RendererFake) Render(_ context.Context, s dashboard.Snapshot) ([]byte,error) { if f.RenderErr != nil { return nil, f.RenderErr }; return []byte("<html>"),nil }
func (f *RendererFake) Publish(_ context.Context, s dashboard.Snapshot) error { if f.PublishErr != nil { return f.PublishErr }; f.Published = append(f.Published, s); f.Last = s.Status; return nil }
func (f *RendererFake) PublishCount() int { return len(f.Published) }
func (f *RendererFake) Status() dashboard.DashboardStatus { return f.Last }
func FakeRenderer() *RendererFake { return &RendererFake{} }
type ResultFake struct { Results []capability.Result }
func (f *ResultFake) WriteCapabilityResult(_ context.Context, r capability.Result) error { f.Results = append(f.Results, r); return nil }
func FakeResultWriter() capability.ResultWriter { return &ResultFake{} }
func FakeTrackerRunner() tracker.CommandRunner { return tracker.NewFakeRunner(tracker.CommandResult{Exit: 0}) }
func GitRepo(t *testing.T) string { t.Helper(); root := t.TempDir(); cmd := exec.Command("git", "init", "--quiet", root); if out, err := cmd.CombinedOutput(); err != nil { t.Fatalf("git init: %v: %s", err, out) }; return root }
```

- [ ] **Step 1: Write failing tests:**
```go
package capability_test

import (
  "context"
  "testing"
  capability "github.com/thebpandey/agent-team/vnext/internal/capability"
  "github.com/thebpandey/agent-team/vnext/internal/testkit"
)

func TestConsentAndRollback(t *testing.T) {
  runner := testkit.FakeNativeRunner()
  p := capability.InstallPlan{Name:capability.Serena, Source:"verified", VerifiedVersion:"1", OwnedFiles:[]capability.OwnedFile{{Path:"serena.json",Role:"config",SHA256:"sha256:x"}}, Rollback:[]capability.RollbackEntry{{Path:"serena.json",SHA256:"sha256:y"}}, Explicit:true}
  if _, err := capability.Install(context.Background(), runner, p); err != nil { t.Fatal(err) }
  if err := capability.Rollback(context.Background(), runner, p); err != nil { t.Fatal(err) }
}
func TestDeclinedAndUnsafePlansDoNotRun(t *testing.T) {
  if _, err := capability.BuildInstallPlan(capability.Probe{Name:capability.Serena,Available:false}, capability.Consent{Name:capability.Serena,Enabled:false}); err == nil { t.Fatal("declined plan accepted") }
  if _, err := capability.BuildInstallPlan(capability.Probe{Name:capability.Serena,Available:true,Healthy:true,Version:"1"}, capability.Consent{Name:capability.Serena,Enabled:true,Source:"unverified"}); err == nil { t.Fatal("unverified plan accepted") }
}
```
- [ ] **Step 2: Run:** `cd vnext && go test ./internal/capability -run 'TestProbe|TestConsent|TestInstall'`; **Expected:** FAIL because contracts are absent.
- [ ] **Step 3: Implement** typed capability/config records, `NativeRunner`, bounded `OutputStore`, and explicit install plans. Verify source/version before execution and every changed file against the rollback manifest afterward. Never invoke a shell, vendor host installer, or operation that mutates hooks, MCP, settings, plugins, skills, environment, or credentials.
- [ ] **Step 4: Run:** `cd vnext && gofmt -w internal/core internal/capability && go test ./internal/core ./internal/capability -count=1`; **Expected:** PASS.
- [ ] **Step 5: Commit:** `git add vnext/internal/core vnext/internal/capability && git commit -m "feat(vnext): add consented capability contracts"`.

### Task 2: Narrow accelerator adapters and one-at-a-time router

**Files:** Create `vnext/internal/capability/{leanctx.go,serena.go,graphify.go,playwright.go,router.go,router_test.go}`.

**Interfaces: Consumes** Task 1 capability probes, native runner, and bounded stores; **Produces**:

```go
type Question struct { Task core.TaskID; Worktree,Revision string; Prompt string; SecondQuestion string; Files []string; Skills []core.SkillRef }
type Result struct { Name Name; Version,Worktree,Revision string; Skills []core.SkillRef; Used bool; Fallback bool; Summary,RawOutputPointer string; DurationMillis int64; TokensBefore,TokensAfter int }
type Route struct { Primary Name; Secondary *Name; Reason string; NativeFallback []string }
type ResultWriter interface { WriteCapabilityResult(context.Context,Result) error }
type Router interface { Select(Question,[]Probe) (Route,error); Execute(context.Context,Route,Question) (Result,error) }
type SerenaOperation string
const ( SerenaFindDefinition SerenaOperation = "find-definition"; SerenaFindReferences SerenaOperation = "find-references"; SerenaFindCallers SerenaOperation = "find-callers"; SerenaSearchSymbols SerenaOperation = "search-symbols" )
type SerenaRequest struct { Operation SerenaOperation; Worktree,Revision,Path,Symbol,Query string; ReadOnly bool }
type SerenaResponse struct { Operation SerenaOperation; Result, EvidencePointer string }
type BrowserResource struct { ID,Session,State,Ownership string }
type PlaywrightQuestion struct { Question; SessionName,ResourceID,TargetURL,Viewport string }
type PlaywrightRoute struct { SessionName,ResourceID,TargetURL,Viewport string; EvidencePointer string }
func AllowSerena(Consent,SerenaRequest) error
func RunLeanCTX(context.Context,NativeRunner,OutputStore,TokenSource,Question) (Result,error)
func RunGraphify(context.Context,NativeRunner,OutputStore,Question) (Result,error)
func NewRouter(NativeRunner,OutputStore,TokenSource,ResultWriter) Router
func RoutePlaywright(PlaywrightQuestion,[]BrowserResource) (PlaywrightRoute,error)
```

**Interfaces: Consumes** Task 1 `NativeRunner`, `OutputStore`, `TokenSource`, and `ResultWriter`; **Produces** `Question`, `Route`, `Result`, the Serena allowlist, and named-session Playwright routes.

- [ ] **Step 1: Write failing tests:**
```go
package capability_test

import (
  "context"
  "testing"
  capability "github.com/thebpandey/agent-team/vnext/internal/capability"
  "github.com/thebpandey/agent-team/vnext/internal/testkit"
)

func TestRouterAndPlaywright(t *testing.T) {
  q := capability.PlaywrightQuestion{Question:capability.Question{Task:"T-1",Prompt:"capture UI"},SessionName:"ui-1",ResourceID:"browser-1",TargetURL:"http://127.0.0.1:3000",Viewport:"1280x800"}
  got, err := capability.RoutePlaywright(q, []capability.BrowserResource{{ID:"browser-1",Session:"ui-1",State:"ready",Ownership:"managed"}})
  if err != nil || got.ResourceID != "browser-1" { t.Fatal(got, err) }
  router := capability.NewRouter(testkit.FakeNativeRunner(), testkit.FakeOutputStore(), testkit.FakeTokenSource(), testkit.FakeResultWriter())
  route, err := router.Select(capability.Question{Task:"T-1",Prompt:"find callers",SecondQuestion:"which tests cover this?"}, []capability.Probe{{Name:capability.Serena,Available:true,Healthy:true}})
  if err != nil || route.Primary != capability.Serena || route.Secondary == nil { t.Fatal(route, err) }
}
func TestAdaptersAndForbiddenRoutes(t *testing.T) {
  q := capability.Question{Task:"T-1",Worktree:"wt",Revision:"git-1",Prompt:"inspect"}
  if _, err := capability.RunLeanCTX(context.Background(), testkit.FakeNativeRunner(), testkit.FakeOutputStore(), testkit.FakeTokenSource(), q); err != nil { t.Fatal(err) }
  if _, err := capability.RunGraphify(context.Background(), testkit.FakeNativeRunner(), testkit.FakeOutputStore(), q); err != nil { t.Fatal(err) }
  consent := capability.Consent{Name:capability.Serena,Enabled:true,Mode:capability.ReadOnlyMCP}
  if err := capability.AllowSerena(consent, capability.SerenaRequest{Operation:capability.SerenaFindDefinition,ReadOnly:false}); err == nil { t.Fatal("non-read-only Serena request accepted") }
  if _, err := capability.RoutePlaywright(capability.PlaywrightQuestion{Question:q,SessionName:"ui-1",ResourceID:"browser-1",TargetURL:"http://127.0.0.1:3000",Viewport:"1280x800"}, nil); err == nil { t.Fatal("unowned browser route accepted") }
  router := capability.NewRouter(testkit.FakeNativeRunner(), testkit.FakeOutputStore(), testkit.FakeTokenSource(), testkit.FakeResultWriter())
  route, err := router.Select(capability.Question{Task:"T-1",Prompt:"find callers",SecondQuestion:""}, []capability.Probe{{Name:capability.Serena,Available:true,Healthy:true}}); if err != nil || route.Secondary != nil { t.Fatal("unnamed second question was admitted", route, err) }
}
```
- [ ] **Step 2: Run:** `cd vnext && go test ./internal/capability -run 'TestRouter|TestForbidden'`; **Expected:** FAIL because adapters are absent.
- [ ] **Step 3: Implement** adapters with exact argument arrays: LeanCTX compression only with raw evidence pointer and `TokenSource`; Serena requests restricted to the four declared read-only operations and explicit consent; Graphify `extract --code-only --no-viz` plus bounded `affected/path/explain`; Playwright routes only a named session/resource/URL/viewport and writes evidence through `OutputStore`; and receipt outcome metrics through `ResultWriter`. Every `Question` carries the task-scoped `core.SkillRef` list, including one `using-superpowers` reference and only selected role/task skills; every `Result` records the selected path/digest references. `AllowSerena` rejects edit, command, memory, project-switch, handoff, and arbitrary-tool requests, including `ReadOnly:false`. Do not expose those operations through the router.
- [ ] **Step 4: Run:** `cd vnext && gofmt -w internal/capability && go test ./internal/capability -count=1`; **Expected:** PASS.
- [ ] **Step 5: Commit:** `git add vnext/internal/capability && git commit -m "feat(vnext): add bounded optional accelerator routing"`.

### Task 3: Visual skill routing and rendered-evidence packet preparation

**Files:** Create `vnext/internal/capability/visual.go` and `visual_test.go`; modify `vnext/internal/core/types.go` and `vnext/internal/run/manifest.go`.

**Interfaces: Consumes** Task 1 `Probe` and Phase 1 `core.SkillRef`; **Produces**:

```go
type VisualRequest struct { Task core.TaskID; RequiredEvidence bool; Stack string; DesignAuthority string; RequestedSkills []Name }
func VisualSkills(VisualRequest,[]Probe) ([]core.SkillRef,error)
func BrowserEvidenceRequired(VisualRequest) bool
```

**Interfaces: Consumes** Task 1 `Probe` and Phase 1 `core.SkillRef`; **Produces** validated visual skill references and browser-evidence requirements for later packets.

- [ ] **Step 1: Write failing tests:**
```go
package capability_test

import (
  "testing"
  capability "github.com/thebpandey/agent-team/vnext/internal/capability"
)

func TestVisualPair(t *testing.T) { got, err := capability.VisualSkills(capability.VisualRequest{Task:"T-1",RequiredEvidence:true,DesignAuthority:"sha256:d"}, nil); if err != nil || len(got) < 2 || got[0].Name != string(capability.Impeccable) || got[1].Name != string(capability.UIUXProMax) { t.Fatal(got, err) } }
func TestNonVisualAndDigestInvalidation(t *testing.T) { got, err := capability.VisualSkills(capability.VisualRequest{Task:"T-1",RequiredEvidence:false}, nil); if err != nil || len(got) != 0 { t.Fatal(got, err) }; if capability.BrowserEvidenceRequired(capability.VisualRequest{Task:"T-1",RequiredEvidence:false}) { t.Fatal("browser evidence required without criteria") } }
```
- [ ] **Step 2: Run:** `cd vnext && go test ./internal/capability -run 'TestVisual'`; **Expected:** FAIL.
- [ ] **Step 3: Implement** packet skill references and digest validation without loading skill content into core context. Preserve project design authority and record rendered evidence pointers only.
- [ ] **Step 4: Run:** `cd vnext && gofmt -w internal/core internal/run internal/capability && go test ./internal/core ./internal/run ./internal/capability -count=1`; **Expected:** PASS.
- [ ] **Step 5: Commit:** `git add vnext/internal/core vnext/internal/run vnext/internal/capability && git commit -m "feat(vnext): route visual skills and evidence"`.

### Task 4: Managed development-server and browser registries

**Files:** Create `vnext/internal/resources/{registry.go,server.go,browser.go,registry_test.go}`; modify `vnext/internal/core/types.go` and `vnext/internal/knowledge/receipt.go`.

**Interfaces: Consumes** Phase 1 `store`, `tracker`, `core.RecordEnvelope`, and Task 1 testkit; **Produces**:

```go
type Ownership string
const ( Managed Ownership = "managed"; Unknown Ownership = "unknown"; UserOwned Ownership = "user-owned" )
type ResourceOwner struct { Run core.RunID; Team core.TeamID; Task core.TaskID; Worktree,Revision string }
type ServerRecord struct { core.RecordEnvelope; ID,Command,URL,Target,Purpose string; Port int; Owner ResourceOwner; State string; Ownership Ownership; StartedAt,TraceEvidence,ExternalRef,PIDDiagnostic string }
type BrowserRecord struct { core.RecordEnvelope; ID,Session,URL,Target,Purpose string; Owner ResourceOwner; State string; Ownership Ownership; StartedAt,TraceEvidence,ExternalRef string }
type ReserveOutcome struct { ID string; ExpectedRevision,Revision uint64; CollisionKey,State,TraceEvidence,ExternalRef string }
type ReleaseOutcome struct { ID string; ExpectedRevision,Revision uint64; Released bool; State,TraceEvidence,ExternalRef,ProtectedReason string }
type Registry interface { List(context.Context,string) ([]ServerRecord,[]BrowserRecord,error); ReserveServer(context.Context,ServerRecord,uint64) (ReserveOutcome,error); ReserveBrowser(context.Context,BrowserRecord,uint64) (ReserveOutcome,error); MarkStarted(context.Context,string,uint64,string,string) (ReserveOutcome,error); Release(context.Context,string,uint64) (ReleaseOutcome,error); StopManaged(context.Context,string,uint64) (ReleaseOutcome,error); MarkUnknown(context.Context,string,uint64,string) (ReleaseOutcome,error) }
func NewRegistry(*store.Store,tracker.CommandRunner) Registry
```

**Interfaces: Consumes** Phase 1 `store.Store`, `tracker.CommandRunner`, `core.RecordEnvelope`, and Task 1 testkit; **Produces** resource records and every lifecycle operation with an explicit expected revision, evidence pointer, and external reference.

- [ ] **Step 1: Write failing tests:**
```go
package resources_test

import (
  "context"
  "errors"
  "fmt"
  "testing"
  "github.com/thebpandey/agent-team/vnext/internal/core"
  resources "github.com/thebpandey/agent-team/vnext/internal/resources"
  "github.com/thebpandey/agent-team/vnext/internal/store"
  "github.com/thebpandey/agent-team/vnext/internal/tracker"
  "github.com/thebpandey/agent-team/vnext/internal/testkit"
)

func TestRegistryCapsCASAndOwnership(t *testing.T) { r := resources.NewRegistry(store.New(t.TempDir(), core.StorageLimits{CanonicalBytes:16<<20}), testkit.FakeTrackerRunner()); owner := resources.ResourceOwner{Run:"R-1",Team:"TEAM-1",Task:"TASK-1",Worktree:"wt",Revision:"git-1"}; a, err := r.ReserveServer(context.Background(), resources.ServerRecord{ID:"s1",Port:3000,URL:"http://localhost:3000",Target:"staging",Purpose:"ui",Owner:owner,Ownership:resources.Managed}, 0); if err != nil || a.ExpectedRevision != 0 || a.Revision == 0 { t.Fatal(a, err) }; if _, err := r.ReserveServer(context.Background(), resources.ServerRecord{ID:"s2",Port:3000,URL:"http://localhost:3000",Target:"staging",Purpose:"ui",Owner:owner,Ownership:resources.Managed}, a.Revision); !errors.Is(err, core.ErrCapacity) { t.Fatal(err) }; if _, err := r.Release(context.Background(), "s1", 0); !errors.Is(err, core.ErrRevision) { t.Fatal(err) }; if _, err := r.MarkUnknown(context.Background(), "s1", a.Revision, "timeout"); err != nil { t.Fatal(err) } }
func TestRegistryServerAndBrowserCaps(t *testing.T) { r := resources.NewRegistry(store.New(t.TempDir(), core.StorageLimits{CanonicalBytes:16<<20}), testkit.FakeTrackerRunner()); owner := resources.ResourceOwner{Run:"R",Team:"TEAM",Task:"T",Worktree:"wt",Revision:"rev"}; for i := 0; i < 2; i++ { if _, err := r.ReserveServer(context.Background(), resources.ServerRecord{ID:fmt.Sprintf("s%d",i),Port:3000+i,Target:"dev",Purpose:"ui",Owner:owner,Ownership:resources.Managed}, 0); err != nil { t.Fatal(err) } }; if _, err := r.ReserveServer(context.Background(), resources.ServerRecord{ID:"s3",Port:3003,Target:"dev",Purpose:"ui",Owner:owner,Ownership:resources.Managed}, 0); !errors.Is(err, core.ErrCapacity) { t.Fatal(err) }; for i := 0; i < 2; i++ { if _, err := r.ReserveBrowser(context.Background(), resources.BrowserRecord{ID:fmt.Sprintf("b%d",i),Session:fmt.Sprintf("session-%d",i),Target:"dev",Purpose:"ui",Owner:owner,Ownership:resources.Managed}, 0); err != nil { t.Fatal(err) } }; if _, err := r.ReserveBrowser(context.Background(), resources.BrowserRecord{ID:"b3",Session:"session-3",Target:"dev",Purpose:"ui",Owner:owner,Ownership:resources.Managed}, 0); !errors.Is(err, core.ErrCapacity) { t.Fatal(err) } }
func TestRegistryCASLifecycle(t *testing.T) { r := resources.NewRegistry(store.New(t.TempDir(), core.StorageLimits{CanonicalBytes:16<<20}), testkit.FakeTrackerRunner()); owner := resources.ResourceOwner{Run:"R",Team:"TEAM",Task:"T",Worktree:"wt",Revision:"rev"}; a, err := r.ReserveServer(context.Background(), resources.ServerRecord{ID:"s1",Port:3100,Target:"dev",Purpose:"ui",Owner:owner,Ownership:resources.Managed}, 0); if err != nil { t.Fatal(err) }; started, err := r.MarkStarted(context.Background(), "s1", a.Revision, "trace:s1", "external:s1"); if err != nil || started.Revision <= a.Revision { t.Fatal(started, err) }; unknown, err := r.MarkUnknown(context.Background(), "s1", started.Revision, "stop-timeout"); if err != nil || unknown.State != "unknown" { t.Fatal(unknown, err) } }
type stopFailureRunner struct{}
func (stopFailureRunner) Run(context.Context, string, ...string) tracker.CommandResult { return tracker.CommandResult{Transport:errors.New("stop unavailable")} }
func TestFailedStopPreservesRecord(t *testing.T) { r := resources.NewRegistry(store.New(t.TempDir(), core.StorageLimits{CanonicalBytes:16<<20}), stopFailureRunner{}); owner := resources.ResourceOwner{Run:"R",Team:"TEAM",Task:"T",Worktree:"wt",Revision:"rev"}; a, err := r.ReserveServer(context.Background(), resources.ServerRecord{ID:"s1",Port:3200,Target:"dev",Purpose:"ui",Owner:owner,Ownership:resources.Managed}, 0); if err != nil { t.Fatal(err) }; if _, err := r.StopManaged(context.Background(), "s1", a.Revision); err == nil { t.Fatal("failed stop accepted") }; servers, _, err := r.List(context.Background(), "R"); if err != nil || len(servers) != 1 { t.Fatal(servers, err) } }
```
- [ ] **Step 2: Run:** `cd vnext && go test ./internal/resources -run 'TestRegistry|TestResource'`; **Expected:** FAIL.
- [ ] **Step 3: Implement** typed run/team/task/worktree/revision ownership, URL/target/purpose collision keys, lifecycle/start/trace evidence, external references, expected-revision compare-and-swap reserve/release outcomes, two-server/two-browser caps, stale terminal cleanup, and native process invocation with bounded stop attempts. `PIDDiagnostic` is informational and non-authoritative; never infer ownership or cleanup permission from it. Never use `/proc` or POSIX signals.
- [ ] **Step 4: Run:** `cd vnext && gofmt -w internal/resources internal/core internal/knowledge && go test ./internal/resources ./internal/knowledge -count=1`; **Expected:** PASS.
- [ ] **Step 5: Commit:** `git add vnext/internal/resources vnext/internal/core vnext/internal/knowledge && git commit -m "feat(vnext): add bounded managed resource registry"`.

### Task 5: Local atomic dashboard renderer

**Files:** Create `vnext/internal/dashboard/{render.go,template.go,render_test.go}`.

**Interfaces: Consumes** Phase 1 `store`, `core.ResourceSnapshot`, and typed integration evidence; **Produces**:

```go
type TaskSummary struct { ID,State,Next string }
type TeamSummary struct { ID,State,QueueFingerprint string; Tasks []string }
type DashboardStatus string
const ( Current DashboardStatus = "current"; Stale DashboardStatus = "stale"; Unavailable DashboardStatus = "unavailable" )
type Snapshot struct { Schema int; Project,RunID,CanonicalRevision,LastIntegration string; Status DashboardStatus; Tasks []TaskSummary; Teams []TeamSummary; Resources core.ResourceSnapshot; Evidence []string; GeneratedAt string }
type IntegrationResult struct { Success bool; RunID,TaskID,CanonicalRevision,Error string; EvidencePointers []string; Revision uint64 }
type DashboardRefreshReceipt struct { core.RecordEnvelope; RunID,TaskID,CanonicalRevision string; Status DashboardStatus; Success bool; Error string; EvidencePointers []string }
type Renderer interface { Render(context.Context,Snapshot) ([]byte,error); Publish(context.Context,Snapshot) error }
type ReceiptWriter interface { WriteRefreshReceipt(context.Context,DashboardRefreshReceipt) error }
type IntegrationObserver interface { AfterIntegration(context.Context,IntegrationResult) error }
func NewRenderer(*store.Store) Renderer
func NewIntegrationObserver(Renderer,ReceiptWriter) IntegrationObserver
```

**Interfaces: Consumes** Phase 1 `store.Store`, `core.ResourceSnapshot`, and typed integration evidence; **Produces** a local last-good `Snapshot` and one durable `DashboardRefreshReceipt` for both successful and failed refreshes.

- [ ] **Step 1: Write failing tests:**
```go
package dashboard_test

import (
  "context"
  "errors"
  "testing"
  dashboard "github.com/thebpandey/agent-team/vnext/internal/dashboard"
  "github.com/thebpandey/agent-team/vnext/internal/testkit"
)

func TestDashboardObserverPublishesOnce(t *testing.T) { fake, receipts := testkit.FakeRenderer(), testkit.FakeReceiptWriter(); obs := dashboard.NewIntegrationObserver(fake, receipts); if err := obs.AfterIntegration(context.Background(), dashboard.IntegrationResult{Success:true,RunID:"R",TaskID:"T",CanonicalRevision:"git-1",Revision:2}); err != nil { t.Fatal(err) }; if fake.PublishCount() != 1 || receipts.Count() != 1 { t.Fatal(fake.PublishCount(), receipts.Count()) }; if err := obs.AfterIntegration(context.Background(), dashboard.IntegrationResult{Success:false,RunID:"R",TaskID:"T",Error:"conflict",Revision:3}); err != nil { t.Fatal(err) }; if fake.PublishCount() != 1 || receipts.Count() != 2 || fake.Status() != dashboard.Current || receipts.Receipts[1].Status != dashboard.Stale || receipts.Receipts[1].Success { t.Fatal(fake.PublishCount(), receipts.Count(), fake.Status(), receipts.Receipts) } }
func TestRendererFailureIsUnavailableAndLastGoodIsRetained(t *testing.T) { fake, receipts := testkit.FakeRenderer(), testkit.FakeReceiptWriter(); obs := dashboard.NewIntegrationObserver(fake, receipts); if err := obs.AfterIntegration(context.Background(), dashboard.IntegrationResult{Success:true,RunID:"R",TaskID:"T",CanonicalRevision:"git-1",Revision:1}); err != nil { t.Fatal(err) }; failing := testkit.FakeRenderer(); failing.PublishErr = errors.New("rename failed"); failingObs := dashboard.NewIntegrationObserver(failing, receipts); if err := failingObs.AfterIntegration(context.Background(), dashboard.IntegrationResult{Success:true,RunID:"R",TaskID:"T",CanonicalRevision:"git-2",Revision:2}); err != nil { t.Fatal(err) }; if failing.PublishCount() != 0 || receipts.Count() != 2 || receipts.Receipts[1].Status != dashboard.Unavailable || receipts.Receipts[1].Success { t.Fatal(failing.PublishCount(), receipts.Receipts) } }
```
- [ ] **Step 2: Run:** `cd vnext && go test ./internal/dashboard`; **Expected:** FAIL.
- [ ] **Step 3: Implement** `NewIntegrationObserver` against typed `IntegrationResult` and `ReceiptWriter`. A successful integration attempts one publish and writes one `Current` receipt; an integration error writes one `Stale` receipt and does not call the renderer, preserving the last-good snapshot; a renderer/render/atomic-replace error writes one `Unavailable` receipt with `Success:false` and preserves the last-good bytes. Use escaped static HTML with no external assets, local-only path containment, bounded lists, and atomic last-good publication. Refresh failure never blocks integration.
- [ ] **Step 4: Run:** `cd vnext && gofmt -w internal/dashboard && go test ./internal/dashboard -count=1`; **Expected:** PASS.
- [ ] **Step 5: Commit:** `git add vnext/internal/dashboard && git commit -m "feat(vnext): add local last-good dashboard"`.

### Task 6: Native fallbacks and optional-capability acceptance matrix

**Files:** Create `vnext/internal/acceptance/optional_test.go`, `.github/workflows/vnext-optional.yml`, and `vnext/testdata/capabilities/{healthy.json,missing.json,unsafe.json}`.

**Interfaces: Consumes** `capability.NewRouter`/`Probe`/`Question` from Task 2 (`internal/capability/router.go`), `resources.NewRegistry`/`ServerRecord`/`ResourceOwner` from Task 4 (`internal/resources/registry.go`), `dashboard.IntegrationResult`/`NewIntegrationObserver` from Task 5 (`internal/dashboard/render.go`), and `testkit.Fake*` from Task 1 (`internal/testkit/optional.go`). **Produces** native Linux/macOS/Windows acceptance coverage, explicit fallback evidence, and the CI matrix workflow.

- [ ] **Step 1: Write failing tests:**
```go
package acceptance

import ( "context"; "errors"; "testing"; "github.com/thebpandey/agent-team/vnext/internal/capability"; "github.com/thebpandey/agent-team/vnext/internal/core"; "github.com/thebpandey/agent-team/vnext/internal/dashboard"; "github.com/thebpandey/agent-team/vnext/internal/resources"; "github.com/thebpandey/agent-team/vnext/internal/store"; "github.com/thebpandey/agent-team/vnext/internal/testkit" )
// resources.NewRegistry, ServerRecord, ResourceOwner, ReserveServer, and Release are
// produced by Task 4; dashboard.IntegrationResult/NewIntegrationObserver are produced
// by Task 5; all Fake* helpers are produced by Task 1's testkit block.
func TestOptionalAcceptance(t *testing.T) {
  for _, tc := range []struct{name string; probe capability.Probe; fallback bool}{{"missing",capability.Probe{Available:false},true},{"unhealthy",capability.Probe{Available:true,Healthy:false},true},{"healthy",capability.Probe{Name:capability.Serena,Available:true,Healthy:true},false}} {
    t.Run(tc.name, func(t *testing.T) { r:=capability.NewRouter(testkit.FakeNativeRunner(),testkit.FakeOutputStore(),testkit.FakeTokenSource(),testkit.FakeResultWriter()); got,err:=r.Select(capability.Question{Task:"T-1",Prompt:"inspect"},[]capability.Probe{tc.probe}); if err!=nil || got.Primary=="" || got.NativeFallback==nil { t.Fatal(got,err) }; if tc.fallback && got.Primary!=capability.Native { t.Fatal("optional route did not fall back to native",got) }; if !tc.fallback && got.Primary!=capability.Serena { t.Fatal("healthy optional route not selected",got) } })
  }
  registry:=resources.NewRegistry(store.New(t.TempDir(),core.StorageLimits{CanonicalBytes:16<<20}),testkit.FakeTrackerRunner()); owner:=resources.ResourceOwner{Run:"R",Team:"TEAM",Task:"T",Worktree:"wt",Revision:"rev"}; first,err:=registry.ReserveServer(context.Background(),resources.ServerRecord{ID:"s1",Port:3000,Target:"dev",Purpose:"ui",Owner:owner,Ownership:resources.Managed},0); if err!=nil || first.Revision==0 { t.Fatal(first,err) }; if _,err=registry.ReserveServer(context.Background(),resources.ServerRecord{ID:"s2",Port:3000,Target:"dev",Purpose:"ui",Owner:owner,Ownership:resources.Managed},first.Revision); !errors.Is(err,core.ErrCapacity) { t.Fatal(err) }; if _,err=registry.Release(context.Background(),"s1",0); !errors.Is(err,core.ErrRevision) { t.Fatal(err) }
  renderer,receipts:=testkit.FakeRenderer(),testkit.FakeReceiptWriter(); observer:=dashboard.NewIntegrationObserver(renderer,receipts); if err:=observer.AfterIntegration(context.Background(),dashboard.IntegrationResult{Success:true,RunID:"R",TaskID:"T",CanonicalRevision:"rev",Revision:1}); err!=nil { t.Fatal(err) }; if err:=observer.AfterIntegration(context.Background(),dashboard.IntegrationResult{Success:false,RunID:"R",TaskID:"T",Error:"refresh",Revision:2}); err!=nil || receipts.Count()!=2 || renderer.Status()!=dashboard.Current || receipts.Receipts[1].Status!=dashboard.Stale { t.Fatal(err,receipts.Count(),renderer.Status(),receipts.Receipts) }
}
```
- [ ] **Step 2: Run:** `cd vnext && go test ./internal/acceptance -run 'TestOptional'`; **Expected:** FAIL until all adapters and registries are wired.
- [ ] **Step 3: Implement** test orchestration with fake runners and store snapshots; use no network, vendor installer, shell, browser process, or host configuration in unit tests.
- [ ] **Step 4: Add native CI** by creating `.github/workflows/vnext-optional.yml` with this complete workflow:
```yaml
name: vnext-optional
on: [push, pull_request]
jobs:
  optional-ubuntu:
    runs-on: ubuntu-latest
    defaults: { run: { working-directory: vnext } }
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with: { go-version: '1.23' }
      - run: go test ./...
      - run: go vet ./...
      - run: go test ./internal/acceptance -run TestOptional
  optional-macos:
    runs-on: macos-latest
    defaults: { run: { working-directory: vnext } }
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with: { go-version: '1.23' }
      - run: go test ./...
      - run: go vet ./...
      - run: go test ./internal/acceptance -run TestOptional
  optional-windows:
    runs-on: windows-latest
    defaults: { run: { working-directory: vnext } }
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with: { go-version: '1.23' }
      - run: go test ./...
      - run: go vet ./...
      - run: go test ./internal/acceptance -run TestOptional
```
- [ ] **Step 4b: Native matrix commands:** Ubuntu, macOS, and Windows each run from `vnext` with their native shell:
  `go test ./...`, `go vet ./...`, and `go test ./internal/acceptance -run TestOptional`; the Windows job must use PowerShell-compatible command invocation and no POSIX path syntax.
- [ ] **Step 5: Run:** `cd vnext && go test ./... && go vet ./...`; **Expected:** PASS.
- [ ] **Step 6: Commit:** `git add .github/workflows/vnext-optional.yml vnext/internal/acceptance vnext/testdata/capabilities && git commit -m "test(vnext): verify optional capability fallbacks"`.

### Task 7: 20–30 fixture token/time/quality benchmark

**Files:** Create `vnext/internal/bench/{bench.go,fixtures.go,bench_test.go}`.

**Interfaces: Consumes** Task 2 `capability.Router`, Task 1 bounded output/token contracts, and deterministic fixture inputs; **Produces**:

```go
type Fixture struct { Name,Kind,InputPath,ExpectedFallback string; Complexity int }
type Measurement struct { Fixture,Route,Mode,TokenSource,RawOutputPointer string; DurationMillis int64; InputBytes,OutputBytes int; TokensBefore,TokensAfter int; QualityScore int; Fallback bool }
type BenchmarkSummary struct { Fixtures int; Native,Optional []Measurement; MeanTokenDelta,MeanDurationDelta float64; QualityRegressions int }
func Fixtures() []Fixture
func Run(context.Context,[]Fixture,capability.NativeRunner,capability.Router,capability.TokenSource) ([]Measurement,error)
func Compare([]Measurement) BenchmarkSummary
```

- [ ] **Step 1: Write failing tests:** require `len(bench.Fixtures()) == 24`, unique names, deterministic ordering, the eight declared kinds (`discovery`, `test-output`, `semantic`, `refactor`, `blast-radius`, `parallel`, `ui`, `recovery`), native and selected-route measurements, nonempty `RawOutputPointer` values for captured output, and no secret/transcript capture.
- The failing test uses the following expected fixture table (the implementation is added only after Step 2):
```go
func Fixtures() []Fixture {
    rows := []struct{ name, kind string }{
        {"discovery-small", "discovery"}, {"discovery-large", "discovery"}, {"discovery-generated", "discovery"},
        {"test-output-short", "test-output"}, {"test-output-verbose", "test-output"}, {"test-output-failure", "test-output"},
        {"semantic-definition", "semantic"}, {"semantic-references", "semantic"}, {"semantic-callers", "semantic"}, {"semantic-symbol-search", "semantic"},
        {"refactor-imports", "refactor"}, {"refactor-api", "refactor"}, {"refactor-migration", "refactor"},
        {"blast-radius-small", "blast-radius"}, {"blast-radius-large", "blast-radius"}, {"parallel-boundary", "parallel"},
        {"ui-layout", "ui"}, {"ui-responsive", "ui"}, {"ui-accessibility", "ui"}, {"ui-interaction", "ui"},
        {"recovery-malformed", "recovery"}, {"recovery-stale", "recovery"}, {"recovery-conflict", "recovery"}, {"recovery-capacity", "recovery"},
    }
    out := make([]Fixture, 0, len(rows))
    for i, row := range rows { out = append(out, Fixture{Name: row.name, Kind: row.kind, InputPath: "generated:" + row.name, ExpectedFallback: "native", Complexity: i + 1}) }
    return out
}
```
- [ ] **Step 1b: Add the complete benchmark test:**
```go
package bench_test

import (
  "context"
  "strings"
  "testing"
  "github.com/thebpandey/agent-team/vnext/internal/bench"
  "github.com/thebpandey/agent-team/vnext/internal/capability"
  "github.com/thebpandey/agent-team/vnext/internal/testkit"
)

type benchmarkRouter struct{}
func (benchmarkRouter) Select(q capability.Question, probes []capability.Probe) (capability.Route,error) {
  return capability.Route{Primary: capability.Serena, NativeFallback: []string{"rg", "git"}}, nil
}
func (benchmarkRouter) Execute(_ context.Context, _ capability.Route, q capability.Question) (capability.Result,error) {
  return capability.Result{Name: capability.Serena, Worktree:q.Worktree, Revision:q.Revision, Used:true, Summary:"bounded", RawOutputPointer:"memory:optional/"+string(q.Task), DurationMillis:1, TokensBefore:100, TokensAfter:60}, nil
}

func TestFixturesAndMeasurements(t *testing.T) {
  fs := bench.Fixtures()
  if len(fs) != 24 { t.Fatalf("fixtures=%d", len(fs)) }
  wantKinds := map[string]bool{"discovery":true,"test-output":true,"semantic":true,"refactor":true,"blast-radius":true,"parallel":true,"ui":true,"recovery":true}
  seen := map[string]bool{}
  for _, f := range fs { if seen[f.Name] || !wantKinds[f.Kind] || f.InputPath == "" { t.Fatalf("bad fixture: %+v", f) }; seen[f.Name] = true }
  native := testkit.FakeNativeRunner()
  got, err := bench.Run(context.Background(), fs, native, benchmarkRouter{}, testkit.FakeTokenSource())
  if err != nil || len(got) != 48 { t.Fatalf("measurements=%d want=48 err=%v", len(got), err) }
  counts := map[string]int{}; pointers := map[string]map[string]bool{}; routes := map[string]map[string]bool{}
  for _, m := range got { if m.RawOutputPointer == "" || m.InputBytes <= 0 || m.OutputBytes <= 0 || m.TokensBefore < m.TokensAfter { t.Fatalf("incomplete measurement: %+v", m) }; if strings.Contains(m.RawOutputPointer, "secret") { t.Fatal("secret pointer") }; if pointers[m.Fixture] == nil { pointers[m.Fixture] = map[string]bool{}; routes[m.Fixture] = map[string]bool{} }; if pointers[m.Fixture][m.RawOutputPointer] { t.Fatalf("duplicate raw pointer: %s", m.RawOutputPointer) }; pointers[m.Fixture][m.RawOutputPointer] = true; routes[m.Fixture][m.Route] = true; counts[m.Fixture+":"+m.Mode]++; if m.Mode == "native" && (m.Route != "native" || m.TokenSource != "unknown") { t.Fatal("native baseline mislabeled") }; if m.Mode == "optional" && (m.Route == "native" || m.TokenSource != "counter") { t.Fatal("optional route mislabeled") } }
  for _, f := range fs { if counts[f.Name+":native"] != 1 || counts[f.Name+":optional"] != 1 || len(pointers[f.Name]) != 2 || len(routes[f.Name]) != 2 { t.Fatalf("fixture %s lacks distinct baseline/optional pair", f.Name) } }
  summary := bench.Compare(got)
  if summary.Fixtures != 24 || summary.QualityRegressions != 0 { t.Fatalf("summary=%+v", summary) }
}
```
- [ ] **Step 2: Run:** `cd vnext && go test ./internal/bench`; **Expected:** FAIL because benchmark types and runner are absent.
- [ ] **Step 3: Implement** the fixture generator shown above in `vnext/internal/bench/fixtures.go`, then implement fixture loading, monotonic timing, a forced native `Mode:"native"` measurement plus one routed optional `Mode:"optional"` measurement per fixture, byte/token accounting through the supplied `TokenSource`, native `TokenSource:"unknown"` and optional `TokenSource:"counter"` labels, distinct raw-output pointers, quality checks, and summary thresholds.
- [ ] **Step 4: Run:** `cd vnext && go test ./internal/bench -count=1`; **Expected:** PASS with a report under test output only.
- [ ] **Step 5: Commit:** `git add vnext/internal/bench && git commit -m "test(vnext): benchmark optional capability routing"`.

## Spec Coverage and Self-Review

- Tasks 1–3 cover detection, consent, rollback, one-accelerator routing, explicit Serena read-only mode, LeanCTX compression-only behavior, Graphify code-only behavior, Playwright named sessions, and mandatory visual skill routing.
- Task 4 covers two-server/two-browser limits, collision serialization, ownership, unknown retention, resource receipts, and cross-platform stop behavior.
- Task 5 covers local-only atomic dashboard generation after integration with stale last-good preservation.
- Tasks 6–7 cover native fallback, privacy, Windows/macOS/Linux acceptance, and 20–30 fixture cost/quality measurement.
- No task imports v7 code, hooks, Node/Python, vendor host installers, automatic MCP/skill registration, or undefined external service APIs.
- Each task has concrete files, interfaces, tests, commands, expected results, and a commit.

## Execution Handoff

Plan complete and saved to `docs/superpowers/plans/2026-09-19-agent-team-vnext-optional-capabilities.md`. Execute it only after Phase 1 core interfaces and the Phase 2 host/resource contracts are available. Use subagent-driven development or executing-plans, with a review after each task.
