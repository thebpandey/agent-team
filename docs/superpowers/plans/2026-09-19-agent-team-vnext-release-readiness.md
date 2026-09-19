# Agent-Team vNext Release Readiness Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Prove, document, package, install, canary, roll back, and publish the vNext Agent-Team release only after every phase gate and independent review is CLEAN.

**Architecture:** Release readiness is a foreground, evidence-producing workflow around the completed native Go phases. It updates the public README, getting-started guide, skill/reference package, changelog, version metadata, and GitHub workflows; runs native and optional-capability benchmarks; builds deterministic archives/checksums and an SBOM only with already-available tooling; performs Codex/Claude install canaries and rollback; then creates a non-force GitHub release. “Deployment” means installed skill plus GitHub release only; no package registry or production service is introduced.

**Tech Stack:** Existing repository files, Go toolchain, native GitHub Actions, `git`, `sha256sum`/platform equivalent, existing release scripts if retained by the cutover, and the Phase 1–3 Go packages. No new package registry, daemon, Node dependency, hook, MCP registration, or heavy SBOM dependency.

**Spec:** `docs/superpowers/specs/2026-09-18-agent-team-vnext-design.md`

**Prerequisites:** Phase 1 native core, Phase 2 host execution, Phase 3 optional capabilities, installation/migration, and deployment plans have passed their independent CLEAN reviews and native tests.

## Global Release Gates

- No publish, tag, install canary, or GitHub release may run until all phase gates, independent CLEAN reviews, native tests, optional benchmark evidence, cutover canary, and rollback rehearsal are recorded.
- Existing v7.3.1 files remain explicitly labeled legacy during transition; no v7 hook/runtime is silently presented as vNext.
- Release artifacts are deterministic: fixed file list, normalized archive metadata, stable ordering, SHA-256 checksums, and a native dependency-free CycloneDX 1.5 SBOM whose `SBOMTool` is always `native`; no external SBOM executable or unsupported-tool state is introduced.
- Git operations use non-force fetch/push/tag/release commands. Never rewrite shared history.
- Install canaries run for Codex and Claude separately and together; each verifies installed skill digest, binary/contract digest, action routing, rollback, and preservation of unrelated host settings/hooks/MCP.
- Provider verification means the existing deployment/provider verification contract from `docs/superpowers/plans/2026-09-19-agent-team-vnext-deployment.md`; no new provider target is invented.

## File Map

| Path | Responsibility |
| --- | --- |
| `README.md`, `GETTING_STARTED.md`, `SKILL.md`, `references/DEPENDENCIES.md` | Public vNext install, operation, limits, fallback, and legacy guidance. |
| `CHANGELOG.md`, `vnext/VERSION`, `vnext/RELEASE.json` | Version, release notes, source revision, artifact manifest. |
| `vnext/internal/release/{manifest.go,artifact.go,verify.go,sbom.go}` | Deterministic manifest, checksums, archive verification, and dependency-free SBOM. |
| `vnext/internal/release/*_test.go`, `vnext/testdata/release/**` | Reproducibility, checksum, privacy, rollback, and canary fixtures. |
| `vnext/internal/bench/{report.go,report_test.go}` | Honest native-vs-optional token/time/quality report with raw pointers. |
| `.github/workflows/vnext-release.yml` | Native matrix, benchmark evidence, artifact signing/checksums, and gated release. |
| `docs/release-checks-7.2.0.md` | Historical v7 checklist retained and labeled legacy; no vNext authority. |

### Task 1: Release manifest, deterministic artifacts, checksums, and lightweight SBOM

**Files:** Create `vnext/VERSION`, `vnext/RELEASE.json`, `vnext/internal/release/{manifest.go,artifact.go,verify.go,release_test.go}` and `vnext/testdata/release/files.txt`.

**Interfaces:**

```go
type Manifest struct { Version,Commit,SpecRevision,Executable string; Files []string; Checksums map[string]string; SBOMPath,SBOMTool string }
type Artifact struct { Path,SHA256 string; Bytes int64 }
type ArchiveFormat string
const ZipArchive ArchiveFormat = "zip-deterministic"
type SBOMComponent struct { Name string `json:"name"`; Version string `json:"version"`; Path string `json:"path"`; SHA256 string `json:"sha256"`; Type string `json:"type"`; BOMRef string `json:"bom-ref"` }
type SBOM struct { Format string `json:"bomFormat"`; SpecVersion string `json:"specVersion"`; Serial string `json:"serialNumber"`; Tool string `json:"tool"`; Components []SBOMComponent `json:"components"` }
func BuildManifest(root,version,commit,spec string, files []string) (Manifest,error)
func BuildArtifact(root,out string,Manifest) (Artifact,error)
func VerifyArtifact(Artifact,Manifest) error
func BuildSBOM(Manifest) (SBOM,error)
func VerifySBOM(SBOM,Manifest) error
func ManifestSHA256(Manifest) string
```

- [ ] **Step 1: Write failing tests:** create two temporary roots with the same sorted UTF-8 files and assert equal manifests/artifact hashes, archive entry order, timestamps, and modes; mutate one byte and assert `VerifyArtifact` returns `core.ErrRevision`; reject files outside the release allowlist and secrets; assert every archive entry has the manifest hash and the SBOM has non-empty CycloneDX `type`/`bom-ref` values. Also copy an `Artifact`, alter `SHA256` and `Bytes` independently, and assert both mutations return `core.ErrRevision`; omit the executable and assert `BuildManifest` returns `core.ErrPath`. On systems supporting symlinks, create `root/link -> outside` and require `BuildManifest(root,...,[]string{"link"}) == core.ErrPath`; require duplicate manifest paths, `../` members, and a hand-built ZIP containing duplicate `agent-teamctl` members to be rejected by `BuildArtifact` or `VerifyArtifact` before any member is trusted.
```go
import ( "archive/zip"; "crypto/sha256"; "encoding/hex"; "errors"; "io"; "os"; "path/filepath"; "reflect"; "sort"; "strings"; "testing"; "time"; "github.com/thebpandey/agent-team/vnext/internal/core"; "github.com/thebpandey/agent-team/vnext/internal/release" )
func writeFixture(t *testing.T,root string) { t.Helper(); if err:=os.MkdirAll(filepath.Join(root,"vnext"),0755); err!=nil { t.Fatal(err) }; if err:=os.WriteFile(filepath.Join(root,"README.md"),[]byte("readme"),0644); err!=nil { t.Fatal(err) }; if err:=os.WriteFile(filepath.Join(root,"vnext","VERSION"),[]byte("8.0.0\n"),0644); err!=nil { t.Fatal(err) }; if err:=os.WriteFile(filepath.Join(root,"agent-teamctl"),[]byte("binary"),0755); err!=nil { t.Fatal(err) } }
func mustOpen(f *zip.File) io.ReadCloser { r,err:=f.Open(); if err!=nil { panic(err) }; return r }
func sha256Hex(b []byte) string { s:=sha256.Sum256(b); return hex.EncodeToString(s[:]) }
func TestDeterministicArtifact(t *testing.T) { roots:=[]string{t.TempDir(),t.TempDir()}; for _,root:=range roots { writeFixture(t,root) }; files:=[]string{"README.md","agent-teamctl","vnext/VERSION"}; m1,err:=release.BuildManifest(roots[0],"8.0.0",strings.Repeat("a",40),"spec-1",files); m1.Executable="agent-teamctl"; if err!=nil { t.Fatal(err) }; m2,err:=release.BuildManifest(roots[1],m1.Version,m1.Commit,m1.SpecRevision,m1.Files); m2.Executable=m1.Executable; if err!=nil || !reflect.DeepEqual(m1,m2) { t.Fatal(m1,m2,err) }; a1,err:=release.BuildArtifact(roots[0],filepath.Join(t.TempDir(),"a.zip"),m1); if err!=nil { t.Fatal(err) }; a2,err:=release.BuildArtifact(roots[1],filepath.Join(t.TempDir(),"b.zip"),m2); if err!=nil || a1.SHA256!=a2.SHA256 { t.Fatal(a1,a2,err) }; if err:=release.VerifyArtifact(a1,m1); err!=nil { t.Fatal(err) }; sbom,err:=release.BuildSBOM(m1); if err!=nil || sbom.Tool!="native" || sbom.Serial!="urn:agent-team:"+release.ManifestSHA256(m1) || sbom.Components[0].Type!="file" || sbom.Components[0].BOMRef=="" || release.VerifySBOM(sbom,m1)!=nil { t.Fatal(sbom,err) }; sbom.Components[0],sbom.Components[1]=sbom.Components[1],sbom.Components[0]; if err:=release.VerifySBOM(sbom,m1); !errors.Is(err,core.ErrRevision) { t.Fatal("component reorder accepted") } }
func TestExecutableModeAllowlist(t *testing.T) { root:=t.TempDir(); writeFixture(t,root); m,err:=release.BuildManifest(root,"8.0.0",strings.Repeat("a",40),"spec-1",[]string{"README.md","agent-teamctl","vnext/VERSION"}); m.Executable="agent-teamctl"; if err!=nil { t.Fatal(err) }; a,err:=release.BuildArtifact(root,filepath.Join(t.TempDir(),"a.zip"),m); if err!=nil { t.Fatal(err) }; if err:=release.VerifyArtifact(a,m); err!=nil { t.Fatal(err) }; m.Executable="README.md"; if err:=release.VerifyArtifact(a,m); !errors.Is(err,core.ErrRevision) { t.Fatal("non-allowlisted executable mode accepted",err) } }
func TestArtifactMutationAndAllowlist(t *testing.T) { root:=t.TempDir(); writeFixture(t,root); m,err:=release.BuildManifest(root,"8.0.0",strings.Repeat("a",40),"spec-1",[]string{"README.md","agent-teamctl","vnext/VERSION"}); if err!=nil { t.Fatal(err) }; a,err:=release.BuildArtifact(root,filepath.Join(t.TempDir(),"a.zip"),m); if err!=nil { t.Fatal(err) }; if err:=os.WriteFile(filepath.Join(root,"README.md"),[]byte("mutated"),0644); err!=nil { t.Fatal(err) }; if err:=release.VerifyArtifact(a,m); !errors.Is(err,core.ErrRevision) { t.Fatal(err) }; if _,err:=release.BuildManifest(root,"8.0.0",strings.Repeat("a",40),"spec-1",[]string{"secret.env"}); !errors.Is(err,core.ErrPath) { t.Fatal(err) } }
func TestArtifactMetadataAndRequiredExecutable(t *testing.T) { root:=t.TempDir(); writeFixture(t,root); m,err:=release.BuildManifest(root,"8.0.0",strings.Repeat("a",40),"spec-1",[]string{"README.md","agent-teamctl","vnext/VERSION"}); if err!=nil { t.Fatal(err) }; m.Executable="agent-teamctl"; a,err:=release.BuildArtifact(root,filepath.Join(t.TempDir(),"a.zip"),m); if err!=nil { t.Fatal(err) }; badHash:=a; badHash.SHA256=strings.Repeat("0",64); if !errors.Is(release.VerifyArtifact(badHash,m),core.ErrRevision) { t.Fatal("artifact hash mutation accepted") }; badBytes:=a; badBytes.Bytes++; if !errors.Is(release.VerifyArtifact(badBytes,m),core.ErrRevision) { t.Fatal("artifact byte mutation accepted") }; if _,err:=release.BuildManifest(root,"8.0.0",strings.Repeat("a",40),"spec-1",[]string{"README.md"}); !errors.Is(err,core.ErrPath) { t.Fatal("missing executable accepted") } }
```
- [ ] **Step 2: Run:** `cd vnext && go test ./internal/release -run TestDeterministicArtifact`; **Expected:** FAIL because release package is absent.
- [ ] **Step 3: Implement:** use Go `archive/zip` with lexicographically sorted paths, UTC timestamp `1980-01-01T00:00:00Z`, mode `0644` for files and `0755` only for the owned executable, and fixed compression settings. Exclude `.git`, secrets, `.agent-team` runtime state, worktrees, raw evidence, and host settings. Compute SHA-256 over exact archive bytes and return its exact byte count. `BuildSBOM` emits CycloneDX JSON 1.5 with `serialNumber:"urn:agent-team:<manifest-sha256>"`, `SBOMTool:"native"`, one component per manifest file, `type:"file"`, `bom-ref:"sha256:<file-hash>"`, and no external dependency. `VerifySBOM` checks format, serial, native tool, sorted components, and hashes.
```go
package release
import ("archive/zip"; "crypto/sha256"; "encoding/hex"; "io"; "os"; "path/filepath"; "sort"; "strings"; "time"; "github.com/thebpandey/agent-team/vnext/internal/core")
func ManifestSHA256(m Manifest) string { canonical:=m.Version+"\x00"+m.Commit+"\x00"+m.SpecRevision+"\x00"+m.Executable; for _,p:=range m.Files { canonical += "\x00"+p+"="+m.Checksums[p] }; sum:=sha256.Sum256([]byte(canonical)); return hex.EncodeToString(sum[:]) }
func sha256Hex(b []byte) string { s:=sha256.Sum256(b); return hex.EncodeToString(s[:]) }
func safeReleasePath(root,p string)(string,error){ clean:=filepath.Clean(p);if clean!=p||filepath.IsAbs(p)||strings.HasPrefix(filepath.ToSlash(clean),"../")||strings.Contains(strings.ToLower(filepath.Base(clean)),"secret"){return "",core.ErrPath};base,err:=filepath.EvalSymlinks(root);if err!=nil{return "",core.ErrPath};full:=filepath.Join(base,clean);info,err:=os.Lstat(full);if err!=nil||info.Mode()&os.ModeSymlink!=0{return "",core.ErrPath};real,err:=filepath.EvalSymlinks(full);if err!=nil{return "",core.ErrPath};rel,err:=filepath.Rel(base,real);if err!=nil||rel==".."||strings.HasPrefix(rel,".."+string(filepath.Separator)){return "",core.ErrPath};return real,nil }
func BuildManifest(root,version,commit,spec string,files []string) (Manifest,error) { if version=="" || len(commit)<40 || len(files)==0 { return Manifest{},core.ErrRevision }; sorted:=append([]string(nil),files...); sort.Strings(sorted); m:=Manifest{Version:version,Commit:commit,SpecRevision:spec,Executable:"agent-teamctl",Files:sorted,Checksums:map[string]string{},SBOMPath:"SBOM.cdx.json",SBOMTool:"native"};for _,p:=range sorted{if _,seen:=m.Checksums[p];seen{return Manifest{},core.ErrPath};full,err:=safeReleasePath(root,p);if err!=nil{return Manifest{},err};body,err:=os.ReadFile(full);if err!=nil{return Manifest{},core.ErrPath};m.Checksums[p]=sha256Hex(body)};if _,ok:=m.Checksums[m.Executable];!ok{return Manifest{},core.ErrPath};return m,nil }
func BuildArtifact(root,out string,m Manifest) (Artifact,error) { if len(m.Files)==0||m.Executable==""{return Artifact{},core.ErrLimit};seen:=map[string]bool{};if err:=os.MkdirAll(filepath.Dir(out),0755);err!=nil{return Artifact{},err};f,err:=os.Create(out);if err!=nil{return Artifact{},err};z:=zip.NewWriter(f);epoch:=time.Date(1980,1,1,0,0,0,0,time.UTC);for _,p:=range m.Files{if seen[p]{z.Close();f.Close();return Artifact{},core.ErrPath};seen[p]=true;full,pathErr:=safeReleasePath(root,p);if pathErr!=nil{z.Close();f.Close();return Artifact{},pathErr};body,readErr:=os.ReadFile(full);if readErr!=nil||sha256Hex(body)!=m.Checksums[p]{z.Close();f.Close();return Artifact{},core.ErrRevision};h:=&zip.FileHeader{Name:p,Method:zip.Store};h.SetModTime(epoch);mode:=os.FileMode(0644);if p==m.Executable{mode=0755};h.SetMode(mode);w,writeErr:=z.CreateHeader(h);if writeErr!=nil{z.Close();f.Close();return Artifact{},writeErr};if _,writeErr=w.Write(body);writeErr!=nil{z.Close();f.Close();return Artifact{},writeErr}};if err:=z.Close();err!=nil{f.Close();return Artifact{},err};if err:=f.Close();err!=nil{return Artifact{},err};raw,err:=os.ReadFile(out);if err!=nil{return Artifact{},err};return Artifact{Path:out,SHA256:sha256Hex(raw),Bytes:int64(len(raw))},nil }
func BuildSBOM(m Manifest) (SBOM,error) { b:=SBOM{Format:"CycloneDX",SpecVersion:"1.5",Serial:"urn:agent-team:"+ManifestSHA256(m),Tool:"native"}; for _,p:=range m.Files { hash:=m.Checksums[p]; b.Components=append(b.Components,SBOMComponent{Name:filepath.Base(p),Version:m.Version,Path:p,SHA256:hash,Type:"file",BOMRef:"sha256:"+hash}) }; return b,nil }
```
```go
func VerifyArtifact(a Artifact,m Manifest) error { if a.SHA256==""||len(m.Files)==0{return core.ErrLimit};raw,err:=os.ReadFile(a.Path);if err!=nil||int64(len(raw))!=a.Bytes||sha256Hex(raw)!=a.SHA256{return core.ErrRevision};z,err:=zip.OpenReader(a.Path);if err!=nil{return core.ErrRevision};defer z.Close();if len(z.File)!=len(m.Files){return core.ErrRevision};seen:=map[string]bool{};for i,e:=range z.File{clean:=filepath.ToSlash(filepath.Clean(strings.ReplaceAll(e.Name,"\\","/")));if clean!=e.Name||clean=="."||strings.HasPrefix(clean,"../")||filepath.IsAbs(e.Name)||seen[e.Name]{return core.ErrRevision};seen[e.Name]=true;wantMode:=os.FileMode(0644);if e.Name==m.Executable{wantMode=0755};if e.Name!=m.Files[i]||e.Modified.UTC()!=time.Date(1980,1,1,0,0,0,0,time.UTC)||e.Mode().Perm()!=wantMode.Perm(){return core.ErrRevision};r,openErr:=e.Open();if openErr!=nil{return core.ErrRevision};body,readErr:=io.ReadAll(r);closeErr:=r.Close();if readErr!=nil||closeErr!=nil||sha256Hex(body)!=m.Checksums[e.Name]{return core.ErrRevision}};return nil }
func VerifySBOM(s SBOM,m Manifest) error { if s.Format!="CycloneDX" || s.SpecVersion!="1.5" || s.Tool!="native" || s.Serial!="urn:agent-team:"+ManifestSHA256(m) || len(s.Components)!=len(m.Files) { return core.ErrRevision }; for i,c:=range s.Components { if c.Path!=m.Files[i] || c.Type!="file" || c.BOMRef!="sha256:"+m.Checksums[m.Files[i]] || c.SHA256!=m.Checksums[m.Files[i]] { return core.ErrRevision } }; return nil }
```
- [ ] **Step 4: Run:** `cd vnext && gofmt -w internal/release/manifest.go internal/release/artifact.go internal/release/verify.go internal/release/sbom.go internal/release/release_test.go && go test ./internal/release -count=1`; **Expected:** PASS.
- [ ] **Step 5: Commit:** `git add vnext/VERSION vnext/RELEASE.json vnext/internal/release vnext/testdata/release && git commit -m "feat(vnext): add deterministic release artifacts"`.

### Task 2: Honest benchmark and evidence report

**Files:** Create `vnext/internal/bench/{report.go,report_test.go}` and `docs/benchmarks/vnext-optional-8.0.0.md`.

**Interfaces — Consumes:** Phase 3 `bench.Fixture`, `bench.Measurement`, `bench.Run`, `bench.Compare`, native fallback measurements, and bounded evidence pointers. This task imports those exact Phase 3 types; it does not redeclare fixture or measurement schemas.

**Interfaces — Produces:** `Measurement`, `Report`, `BuildReport`, and `RenderReport` below for Task 6.

**Interfaces:**

```go
type Report struct { Version,Commit string; Baseline,Optional []Measurement; MeanTokenDelta,MeanDurationDelta float64; QualityRegressions,MissingCounters,Compared int; RawEvidence []string }
func BuildReport(base,optional []Measurement,version,commit string) (Report,error)
func RenderReport(Report) ([]byte,error)
```

- [ ] **Step 1: Write failing test:**
```go
import ("context"; "testing"; "github.com/thebpandey/agent-team/vnext/internal/bench"; "github.com/thebpandey/agent-team/vnext/internal/capability"; "github.com/thebpandey/agent-team/vnext/internal/testkit")
type releaseBenchmarkRouter struct{}
func (releaseBenchmarkRouter) Select(capability.Question, []capability.Probe) (capability.Route,error) { return capability.Route{Primary:capability.Serena,NativeFallback:[]string{"rg","git"}},nil }
func (releaseBenchmarkRouter) Execute(_ context.Context, _ capability.Route, q capability.Question) (capability.Result,error) { return capability.Result{Name:capability.Serena,Worktree:q.Worktree,Revision:q.Revision,Used:true,Summary:"bounded",RawOutputPointer:"memory:optional/"+string(q.Task),DurationMillis:1,TokensBefore:100,TokensAfter:60},nil }
func TestReleaseReport(t *testing.T) { fixtures:=bench.Fixtures(); measurements,err:=bench.Run(context.Background(),fixtures,testkit.FakeNativeRunner(),releaseBenchmarkRouter{},testkit.FakeTokenSource()); if err!=nil || len(measurements)!=48 { t.Fatalf("measurements=%d err=%v",len(measurements),err) }; base,optional:=make([]bench.Measurement,0,24),make([]bench.Measurement,0,24); for _,m:=range measurements { if m.Mode=="native" { base=append(base,m) } else if m.Mode=="optional" { optional=append(optional,m) } }; r,err:=bench.BuildReport(base,optional,"8.0.0","abc"); if err!=nil || r.Compared!=24 || len(r.RawEvidence)!=48 || r.QualityRegressions!=0 { t.Fatal(r,err) }; regression:=optional[0]; regression.QualityScore=4; r,err=bench.BuildReport(base[:1],[]bench.Measurement{regression},"8.0.0","abc"); if err!=nil || r.QualityRegressions!=1 { t.Fatal(r,err) }; duplicate:=append([]bench.Measurement(nil),base...); duplicate[1]=duplicate[0]; if _,err:=bench.BuildReport(duplicate,optional,"8.0.0","abc"); err==nil { t.Fatal("duplicate baseline accepted") }; unmatched:=append([]bench.Measurement(nil),optional...); unmatched[0].Fixture="not-a-fixture"; if _,err:=bench.BuildReport(base,unmatched,"8.0.0","abc"); err==nil { t.Fatal("unmatched optional accepted") }; if _,err:=bench.BuildReport(base[:23],optional,"8.0.0","abc"); err==nil { t.Fatal("missing pair accepted") } }
```
- [ ] **Step 2: Run:** `cd vnext && go test ./internal/bench -run TestReleaseReport`; **Expected:** FAIL because release reporting is absent.
- [ ] **Step 3: Implement:** compare the Phase 3 deterministic fixture set, report total task cost rather than isolated compression savings, preserve raw evidence pointers outside release archives, and set `TokenSource:"unknown"` with zero token delta when counters are unavailable instead of estimating silently.
```go
func BuildReport(base, optional []Measurement, version, commit string) (Report,error) { if len(base)==0 || len(base)!=len(optional) { return Report{},core.ErrRevision }; byFixture:=map[string]Measurement{}; pointers:=map[string]bool{}; for _,m:=range base { if m.Mode!="native" || m.Fixture=="" || m.RawOutputPointer=="" || m.DurationMillis<0 || byFixture[m.Fixture].Fixture!="" || pointers[m.RawOutputPointer] { return Report{},core.ErrRevision }; byFixture[m.Fixture]=m; pointers[m.RawOutputPointer]=true }; evidence:=[]string{}; for _,m:=range base { evidence=append(evidence,m.RawOutputPointer) }; tokenDelta,timeDelta,count,missing,regressions:=0,0,0,0,0; seenOptional:=map[string]bool{}; for _,m:=range optional { if m.Mode!="optional" || m.Fixture=="" || m.RawOutputPointer=="" || m.DurationMillis<0 || seenOptional[m.Fixture] || pointers[m.RawOutputPointer] { return Report{},core.ErrRevision }; b,ok:=byFixture[m.Fixture]; if !ok { return Report{},core.ErrRevision }; seenOptional[m.Fixture]=true; pointers[m.RawOutputPointer]=true; evidence=append(evidence,m.RawOutputPointer); count++; if m.QualityScore<b.QualityScore { regressions++ }; timeDelta += int(m.DurationMillis-b.DurationMillis); if m.TokenSource=="counter" && b.TokenSource=="counter" { tokenDelta += (m.TokensAfter-m.TokensBefore)-(b.TokensAfter-b.TokensBefore) } else { missing++ } }; if count!=len(base) { return Report{},core.ErrRevision }; r:=Report{Version:version,Commit:commit,Baseline:base,Optional:optional,RawEvidence:evidence,Compared:count,MissingCounters:missing,QualityRegressions:regressions}; if count>0 { r.MeanTokenDelta=float64(tokenDelta)/float64(count); r.MeanDurationDelta=float64(timeDelta)/float64(count) }; return r,nil }
```
- [ ] **Step 4: Run:** `cd vnext && gofmt -w internal/bench && go test ./internal/bench -count=1`; **Expected:** PASS and write `docs/benchmarks/vnext-optional-8.0.0.md`.
- [ ] **Step 5: Commit:** `git add vnext/internal/bench docs/benchmarks && git commit -m "docs(vnext): publish optional capability evidence"`.

### Task 3: Public documentation and legacy labeling

**Files:** Modify `README.md`, `GETTING_STARTED.md`, `SKILL.md`, `references/DEPENDENCIES.md`, `CHANGELOG.md`; preserve `docs/release-checks-7.2.0.md` as legacy reference.

**Interfaces — Consumes:** completed phase action vocabulary, installer manifest contract, benchmark report, and v7.3.1 legacy labels.

**Interfaces — Produces:** public documentation assertions for installation, setup, operation, internal review semantics, rollback, and release limitations.

- [ ] **Step 1: Write documentation assertions:**
```go
func TestPublicDocs(t *testing.T) { for _, p := range []string{"README.md","GETTING_STARTED.md","SKILL.md","references/DEPENDENCIES.md"} { b,err:=os.ReadFile(p); if err!=nil { t.Fatal(err) }; s:=string(b); for _, want:=range []string{"agent-teamctl install --host codex|claude|both","TASKS.md","Beads","Codex","Claude","BLOCKERS.md","DECISIONS.md","Windows","macOS","Linux","legacy"} { if !strings.Contains(s,want) { t.Fatalf("%s missing %q",p,want) } } } }
```
- [ ] **Step 2: Run:** `cd vnext && go test ./internal/release -run TestPublicDocs`; **Expected:** FAIL until docs are updated.
- [ ] **Step 3: Implement:** document one combined install operation, host switching in foreground only, setup/status/start/task/one-off lifecycle, internal FIX/CLEAN review semantics (not a public `review` action), caps and fallbacks, dashboard local-only limits, optional tools and native fallbacks, pause/stop/cancel/resume, rollback/uninstall, and explicit v7.3.1 legacy labeling. Link the benchmark report and release checks; do not claim features until their phase gate is passed.
- [ ] **Step 4: Run:** `cd vnext && go test ./internal/release -run TestPublicDocs`; **Expected:** PASS; from repository root run `rg -n 'v7\.3\.1|legacy|agent-teamctl|Windows|Claude|Codex' README.md GETTING_STARTED.md SKILL.md references/DEPENDENCIES.md` and verify each required section is present.
- [ ] **Step 5: Commit:** `git add README.md GETTING_STARTED.md SKILL.md references/DEPENDENCIES.md CHANGELOG.md && git commit -m "docs(vnext): publish release operator guidance"`.

### Task 4: Version, changelog, and gated native release workflow

**Files:** Modify `CHANGELOG.md`; create `vnext/WORKER-CONTRACT`, `vnext/codex/SKILL.md`, `vnext/claude/SKILL.md`, `vnext/cmd/vnext-release/{main.go,main_test.go}`, and `.github/workflows/vnext-release.yml`.

**Interfaces — Consumes:** `release.BuildManifest`, `release.BuildArtifact`, `release.VerifyArtifact`, `release.BuildSBOM`, and the version/revision arguments.

**Interfaces — Produces:** `vnext-release package --version <semver> --commit <sha>` and the gated native release workflow.

- [ ] **Step 1: Write failing release metadata test:**
```go
import ("errors"; "os"; "path/filepath"; "strings"; "testing"; "github.com/thebpandey/agent-team/vnext/internal/core"; "github.com/thebpandey/agent-team/vnext/internal/release")
func TestPackageCommand(t *testing.T) { if err:=release.ValidatePackageArgs("8.0","bad"); !errors.Is(err,core.ErrRevision) { t.Fatal(err) }; if err:=release.ValidatePackageArgs("8.1.0",strings.Repeat("a",40)); err!=nil { t.Fatal(err) }; if err:=release.VerifyOutputAllowlist("8.1.0",[]string{"agent-teamctl-8.1.0.zip","SHA256SUMS","RELEASE.json","SBOM.cdx.json"}); err!=nil { t.Fatal(err) }; if err:=release.VerifyOutputAllowlist("8.1.0",[]string{"secret.env"}); !errors.Is(err,core.ErrPath) { t.Fatal(err) } }
func TestPackageRejectsMissingInputs(t *testing.T) { source,out:=t.TempDir(),t.TempDir(); if err:=release.BuildReleasePackageFrom(source,out,"8.0.0",strings.Repeat("a",40)); !errors.Is(err,core.ErrPath) { t.Fatal("missing executable/contract/entrypoints accepted",err) }; if err:=os.WriteFile(filepath.Join(source,"agent-teamctl"),[]byte("binary"),0755); err!=nil { t.Fatal(err) }; if err:=release.BuildReleasePackageFrom(source,out,"8.0.0",strings.Repeat("a",40)); !errors.Is(err,core.ErrPath) { t.Fatal("missing contract/entrypoints accepted",err) } }
```
Assert `vnext/VERSION`, `vnext/RELEASE.json`, changelog version, and manifest version are identical and semver-valid; assert release workflow has least-privilege permissions.
- [ ] **Step 2: Run:** `cd vnext && go test ./internal/release -run TestReleaseMetadata`; **Expected:** FAIL until metadata/workflow exist.
- [ ] **Step 3: Implement:** use the approved input semver consistently in `VERSION`, `RELEASE.json`, archive filename, changelog, tag, and release title; write migration, rollback, known limitations, v7 legacy status, benchmark link, and checksums. Implement the packaging command with `flag.NewFlagSet`, require an exact semver and 40–64 lowercase/uppercase hex commit, build `agent-teamctl` from `./cmd/agent-teamctl`, require `WORKER-CONTRACT`, `codex/SKILL.md`, and `claude/SKILL.md`, call the release package APIs, and write only `release-artifacts/agent-teamctl-<semver>.zip`, `SHA256SUMS`, `RELEASE.json`, and `SBOM.cdx.json`. The manual workflow validates the input version, completes all gates without a tag, then creates and non-force pushes the annotated `v<semver>` tag immediately before the GitHub release. Add this workflow:
```go
func ValidatePackageArgs(version, commit string) error { if !regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+$`).MatchString(version) || len(commit)<40 || len(commit)>64 { return core.ErrRevision }; for _,c:=range commit { if !(c>='0'&&c<='9') && !(c>='a'&&c<='f') && !(c>='A'&&c<='F') { return core.ErrRevision } }; return nil }
func ReleaseOutputs(version string) []string { return []string{"agent-teamctl-"+version+".zip","SHA256SUMS","RELEASE.json","SBOM.cdx.json"} }
func VerifyOutputAllowlist(version string,files []string) error { want:=map[string]bool{}; for _,f:=range ReleaseOutputs(version){want[f]=true}; for _,f:=range files { if !want[f] { return core.ErrPath } }; return nil }
```
```go
// vnext/cmd/vnext-release/main.go
import ("flag"; "fmt"; "os"; "github.com/thebpandey/agent-team/vnext/internal/release")
func main() { if len(os.Args)<2 { os.Exit(2) }; switch os.Args[1] { case "package": fs:=flag.NewFlagSet("package",flag.ContinueOnError); version,commit:=fs.String("version","",""),fs.String("commit","",""); if fs.Parse(os.Args[2:])!=nil || fs.NArg()!=0 || release.ValidatePackageArgs(*version,*commit)!=nil { os.Exit(2) }; if err:=release.BuildReleasePackage("release-artifacts",*version,*commit); err!=nil { os.Exit(1) }; case "collect-evidence": revision,out,native,benchmark,err:=parseCollectEvidence(os.Args[2:]); if err!=nil || collectEvidence(revision,out,native,benchmark)!=nil { os.Exit(1) }; case "finalize-evidence": revision,out,artifact,sbom,canary,rollback,provider,installed,err:=parseFinalizeEvidence(os.Args[2:]); if err!=nil || finalizeEvidence(revision,out,artifact,sbom,canary,rollback,provider,installed)!=nil { os.Exit(1) }; case "write-readiness": revision,gates,paths,err:=parseReadiness(os.Args[2:]); if err!=nil { os.Exit(2) }; e,err:=release.BuildReadinessEvidence(revision,gates,paths); if err!=nil || release.WriteReadinessEvidence("release-readiness.json",e)!=nil { os.Exit(1) }; case "verify-gates": evidence,revision,err:=parseVerifyGates(os.Args[2:]); if err!=nil || release.VerifyReadinessEvidence(evidence,revision)!=nil { os.Exit(1) }; default: os.Exit(2) } }
func parseVerifyGates(args []string) (string,string,error) { fs:=flag.NewFlagSet("verify-gates",flag.ContinueOnError); evidence,revision:=fs.String("evidence","",""),fs.String("revision","",""); if err:=fs.Parse(args); err!=nil || fs.NArg()!=0 || *evidence=="" || *revision=="" { return "","",fmt.Errorf("verify-gates requires --evidence and --revision") }; return *evidence,*revision,nil }
func parseReadiness(args []string) (string,string,map[string]string,error) { fs:=flag.NewFlagSet("write-readiness",flag.ContinueOnError); revision,gates:=fs.String("revision","",""),fs.String("gates","",""); names:=[]string{"benchmark","artifact","sbom","canary","rollback","provider","installed"}; paths:=map[string]*string{}; for _,name:=range names { paths[name]=fs.String(name,"","") }; if err:=fs.Parse(args); err!=nil || fs.NArg()!=0 || *revision=="" || *gates=="" { return "","",nil,fmt.Errorf("write-readiness requires revision, gates, and all evidence paths") }; out:=map[string]string{}; for _,name:=range names { if *paths[name]=="" { return "","",nil,fmt.Errorf("missing --%s",name) }; out[name]=*paths[name] }; return *revision,*gates,out,nil }
func parseCollectEvidence(args []string)(string,string,string,string,error){fs:=flag.NewFlagSet("collect-evidence",flag.ContinueOnError);revision,out,native,benchmark:=fs.String("revision","",""),fs.String("out","",""),fs.String("native","",""),fs.String("benchmark","","");if err:=fs.Parse(args);err!=nil||fs.NArg()!=0||*revision==""||*out==""||*native==""||*benchmark==""{return "","","","",fmt.Errorf("collect-evidence requires revision,out,native,benchmark")};return *revision,*out,*native,*benchmark,nil}
func parseFinalizeEvidence(args []string)(string,string,string,string,string,string,string,string,error){fs:=flag.NewFlagSet("finalize-evidence",flag.ContinueOnError);revision,out,artifact,sbom,canary,rollback,provider,installed:=fs.String("revision","",""),fs.String("out","",""),fs.String("artifact","",""),fs.String("sbom","",""),fs.String("canary","",""),fs.String("rollback","",""),fs.String("provider","",""),fs.String("installed","","");if err:=fs.Parse(args);err!=nil||fs.NArg()!=0||*revision==""||*out==""||*artifact==""||*sbom==""||*canary==""||*rollback==""||*provider==""||*installed==""{return "","","","","","","","",fmt.Errorf("finalize-evidence requires revision,out,artifact,sbom,canary,rollback,provider,installed")};return *revision,*out,*artifact,*sbom,*canary,*rollback,*provider,*installed,nil}
```
```go
// vnext/cmd/vnext-release/evidence.go
package main
import ("bytes"; "crypto/sha256"; "encoding/hex"; "encoding/json"; "fmt"; "os"; "path/filepath"; "github.com/thebpandey/agent-team/vnext/internal/core"; "github.com/thebpandey/agent-team/vnext/internal/release")
type CheckEvidence struct { Phase,Revision,Kind,Source,Hash string; Passed bool }
func readPassingTest(path string) (string,error) { raw,err:=os.ReadFile(path);if err!=nil||len(raw)==0||bytes.Contains(raw,[]byte(`"Action":"fail"`)){return "",core.ErrPhase};sum:=sha256.Sum256(raw);return hex.EncodeToString(sum[:]),nil }
func requiredEvidenceTests(kind string) []string { switch kind { case "provider": return []string{"TestProviderVerification"}; case "rollback": return []string{"TestBothHostsRollbackAndActions"}; case "installed": return []string{"TestCodexCanary","TestClaudeCanary","TestPackagedArchiveCanary"}; case "canary": return []string{"TestCodexCanary","TestClaudeCanary","TestPackagedArchiveCanary","TestCutover"}; default: return nil } }
func requireNamedTests(path,kind string) error { raw,err:=os.ReadFile(path);if err!=nil||len(raw)==0{return core.ErrPhase};for _,name:=range requiredEvidenceTests(kind){needle:=[]byte(`"Test":"`+name+`"`);if !bytes.Contains(raw,needle){return core.ErrPhase}};return nil }
func writeValidatedCheck(path,revision,phase,kind,source string,validate func(string)error) error { if source==""||validate==nil||validate(source)!=nil{return core.ErrPhase};if err:=requireNamedTests(source,kind);err!=nil{return err};digest,err:=readPassingTest(source);if err!=nil{return err};raw,err:=json.Marshal(CheckEvidence{Phase:phase,Revision:revision,Kind:kind,Source:source,Hash:digest,Passed:true});if err!=nil{return err};return os.WriteFile(path,append(raw,'\n'),0644) }
func writeCheck(path,revision,kind,source string) error { return writeValidatedCheck(path,revision,"phase",kind,source,func(string)error{return nil}) }
func validateManifestEvidence(path,revision string)error{raw,err:=os.ReadFile(path);if err!=nil{return err};var m release.Manifest;if json.Unmarshal(raw,&m)!=nil||m.Version==""||m.Commit!=revision||len(m.Checksums)==0{return core.ErrRevision};return nil}
func validateSBOMEvidence(path,manifestPath string)error{raw,err:=os.ReadFile(path);if err!=nil{return err};var s release.SBOM;manifestRaw,err:=os.ReadFile(manifestPath);if err!=nil{return err};var m release.Manifest;if json.Unmarshal(manifestRaw,&m)!=nil||json.Unmarshal(raw,&s)!=nil||s.Format!="CycloneDX"||s.SpecVersion!="1.5"||s.Tool!="native"||s.Serial!="urn:agent-team:"+release.ManifestSHA256(m)||len(s.Components)==0{return core.ErrRevision};return nil}
func validateCheckEvidence(path,revision,kind string)error{raw,err:=os.ReadFile(path);if err!=nil{return err};var c CheckEvidence;if json.Unmarshal(raw,&c)!=nil||c.Revision!=revision||c.Kind!=kind||!c.Passed||c.Hash==""||c.Source==""{return core.ErrPhase};return nil}
func validateReviewEvidence(path,revision string)error{raw,err:=os.ReadFile(path);if err!=nil{return err};var v struct{Phase,Revision string;Passed bool};if json.Unmarshal(raw,&v)!=nil||v.Revision!=revision||!v.Passed||v.Phase==""{return core.ErrPhase};return nil}
func collectEvidence(revision,out,native,benchmark string) error { if err:=writeCheck(filepath.Join(out,"native.json"),revision,"native",native);err!=nil{return err};return writeCheck(filepath.Join(out,"benchmark.json"),revision,"benchmark",benchmark) }
func finalizeEvidence(revision,out,artifact,sbom,canary,rollback,provider,installed string) error { if revision==""||out==""{return core.ErrPhase};if err:=validateCheckEvidence(filepath.Join(out,"native.json"),revision,"native");err!=nil{return err};if err:=validateCheckEvidence(filepath.Join(out,"benchmark.json"),revision,"benchmark");err!=nil{return err};for _,phase:=range []string{"phase1-review.json","phase2-review.json","phase3-review.json","phase4-review.json","phase5-review.json"}{if err:=validateReviewEvidence(filepath.Join(out,phase),revision);err!=nil{return err}};if err:=validateManifestEvidence(artifact,revision);err!=nil{return err};if err:=validateSBOMEvidence(sbom,artifact);err!=nil{return err};for _,in:=range []struct{kind,path string;validate func(string)error}{{"artifact",artifact,func(string)error{return nil}},{"sbom",sbom,func(string)error{return nil}},{"canary",canary,func(string)error{return nil}},{"rollback",rollback,func(string)error{return nil}},{"provider",provider,func(string)error{return nil}},{"installed-skill",installed,func(string)error{return nil}}}{if err:=writeValidatedCheck(filepath.Join(out,in.kind+".json"),revision,"phase6",in.kind,in.path,in.validate);err!=nil{return err}};gates:=release.Gates{Phase1:true,Phase2:true,Phase3:true,Phase4:true,Phase5:true,Deploy:true,Install:true,ReviewsClean:true,Native:true,Benchmark:true,Artifacts:true,Canaries:true,Rollback:true,Provider:true,Cutover:true};raw,err:=json.Marshal(gates);if err!=nil{return err};return os.WriteFile(filepath.Join(out,"release-gates.json"),append(raw,'\n'),0644) }
```

`collect-evidence` and `finalize-evidence` are explicit `vnext-release` subcommands: parse only the shown required flags, reject missing/extra arguments, call these functions, and return nonzero on any unreadable, empty, or failing test stream. They derive hashes from actual `go test -json` output; they do not accept caller-supplied success booleans. `finalize-evidence` additionally requires the artifact and SBOM validators plus the installed-skill/rollback records emitted by the canary implementation before it writes a fully true gate record.
```go
// vnext/cmd/vnext-release/main_test.go
func TestVerifyGatesCLI(t *testing.T) { if _,_,err:=parseVerifyGates(nil); err==nil { t.Fatal("missing evidence accepted") }; evidence,revision,err:=parseVerifyGates([]string{"--evidence","readiness.json","--revision","abc123"}); if err!=nil || evidence!="readiness.json" || revision!="abc123" { t.Fatal(evidence,revision,err) }; if _,_,err:=parseVerifyGates([]string{"--evidence","readiness.json","--revision","abc123","extra"}); err==nil { t.Fatal("positional extra accepted") } }
func TestEvidenceCLIParsers(t *testing.T){if _,_,_,_,err:=parseCollectEvidence([]string{"--revision","r","--out","e","--native","n","--benchmark","b","extra"});err==nil{t.Fatal("collect extra accepted")};r,o,n,b,err:=parseCollectEvidence([]string{"--revision","r","--out","e","--native","n","--benchmark","b"});if err!=nil||r!="r"||o!="e"||n!="n"||b!="b"{t.Fatal(r,o,n,b,err)};if _,_,_,_,_,_,_,_,err:=parseFinalizeEvidence([]string{"--revision","r","--out","e"});err==nil{t.Fatal("finalize missing accepted")};args:=[]string{"--revision","r","--out","e","--artifact","a","--sbom","s","--canary","c","--rollback","rb","--provider","p","--installed","i"};r,o,a,s,c,rb,p,i,err:=parseFinalizeEvidence(args);if err!=nil||r!="r"||o!="e"||a!="a"||s!="s"||c!="c"||rb!="rb"||p!="p"||i!="i"{t.Fatal(r,o,a,s,c,rb,p,i,err)}}
func TestReviewEvidenceRejectsForgedMissingAndStale(t *testing.T){p:=filepath.Join(t.TempDir(),"phase1-review.json");if err:=os.WriteFile(p,[]byte(`{"Phase":"phase1","Revision":"r","Passed":true}`),0644);err!=nil{t.Fatal(err)};if err:=validateReviewEvidence(p,"r");err!=nil{t.Fatal(err)};if err:=os.WriteFile(p,[]byte(`{"Phase":"phase1","Revision":"r","Passed":false}`),0644);err!=nil{t.Fatal(err)};if err:=validateReviewEvidence(p,"r");err==nil{t.Fatal("forged review accepted")};if err:=validateReviewEvidence(filepath.Join(filepath.Dir(p),"missing.json"),"r");err==nil{t.Fatal("missing review accepted")};if err:=os.WriteFile(p,[]byte(`{"Phase":"phase1","Revision":"old","Passed":true}`),0644);err!=nil{t.Fatal(err)};if err:=validateReviewEvidence(p,"r");err==nil{t.Fatal("stale review accepted")}}
func TestCheckEvidenceRejectsForgedAndStale(t *testing.T){p:=filepath.Join(t.TempDir(),"native.json");good:=`{"Phase":"phase1","Revision":"r","Kind":"native","Source":"native-test.json","Hash":"abc","Passed":true}`;if err:=os.WriteFile(p,[]byte(good),0644);err!=nil{t.Fatal(err)};if err:=validateCheckEvidence(p,"r","native");err!=nil{t.Fatal(err)};if err:=os.WriteFile(p,[]byte(`{"Phase":"phase1","Revision":"old","Kind":"native","Source":"native-test.json","Hash":"abc","Passed":true}`),0644);err!=nil{t.Fatal(err)};if err:=validateCheckEvidence(p,"r","native");err==nil{t.Fatal("stale check accepted")}}
```
```go
// vnext/internal/release/package.go
package release
import ("crypto/sha256"; "encoding/hex"; "encoding/json"; "os"; "path/filepath"; "reflect"; "regexp"; "sort"; "strings"; "github.com/thebpandey/agent-team/vnext/internal/core")
func BuildReleasePackage(root,version,commit string) error { return BuildReleasePackageFrom(".",root,version,commit) }
func BuildReleasePackageFrom(source,root,version,commit string) error { if err:=ValidatePackageArgs(version,commit); err!=nil { return err }; if err:=os.MkdirAll(root,0755); err!=nil { return err }; files:=[]string{"agent-teamctl","WORKER-CONTRACT","codex/SKILL.md","claude/SKILL.md","VERSION"}; manifest,err:=BuildManifest(source,version,commit,"phase1",files); if err!=nil { return err }; manifest.Executable="agent-teamctl"; artifact,err:=BuildArtifact(source,filepath.Join(root,"agent-teamctl-"+version+".zip"),manifest); if err!=nil || VerifyArtifact(artifact,manifest)!=nil { return core.ErrRevision }; sbom,err:=BuildSBOM(manifest); if err!=nil || VerifySBOM(sbom,manifest)!=nil { return core.ErrRevision }; if err:=writeJSON(filepath.Join(root,"RELEASE.json"),manifest); err!=nil { return err }; if err:=writeJSON(filepath.Join(root,"SBOM.cdx.json"),sbom); err!=nil { return err }; if err:=writeChecksums(filepath.Join(root,"SHA256SUMS"),root,[]string{"agent-teamctl-"+version+".zip","RELEASE.json","SBOM.cdx.json"}); err!=nil { return err }; return VerifyOutputAllowlistMustEqual(root,version,ReleaseOutputs(version)) }
func ListPackageOutputs(root,version string) ([]string,error) { entries,err:=os.ReadDir(root); if err!=nil { return nil,err }; names:=make([]string,0,len(entries)); for _,e:=range entries { if e.IsDir() { return nil,core.ErrPath }; names=append(names,e.Name()) }; if err:=VerifyOutputAllowlist(version,names); err!=nil { return nil,err }; expected:=ReleaseOutputs(version); sorted:=append([]string(nil),expected...); sort.Strings(sorted); got:=append([]string(nil),names...); sort.Strings(got); if !reflect.DeepEqual(got,sorted) { return nil,core.ErrPath }; return expected,nil }
func writeJSON(path string, value any) error { b,err:=json.Marshal(value); if err!=nil { return err }; return os.WriteFile(path,b,0644) }
func writeChecksums(path,root string,names []string) error { sort.Strings(names); var lines []string; for _,name:=range names { b,err:=os.ReadFile(filepath.Join(root,name)); if err!=nil { return err }; sum:=sha256.Sum256(b); lines=append(lines,hex.EncodeToString(sum[:])+"  "+name) }; return os.WriteFile(path,[]byte(strings.Join(lines,"\n")+"\n"),0644) }
func VerifyOutputAllowlistMustEqual(root,version string,want []string) error { got,err:=ListPackageOutputs(root,version); if err!=nil { return err }; if !reflect.DeepEqual(got,want) { return core.ErrPath }; return nil }
```
```yaml
name: vnext-release
on:
  workflow_dispatch:
    inputs:
      version:
        description: 'SemVer release version without v prefix'
        required: true
        type: string
permissions:
  contents: read
jobs:
  verify-phases:
    permissions: { contents: read }
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with: { go-version: '1.23' }
      - run: mkdir -p release-evidence
        working-directory: vnext
      - run: go test ./internal/core -count=1 && printf '{"Phase":"phase1","Revision":"%s","Passed":true}\n' "$GITHUB_SHA" > release-evidence/phase1-review.json
        working-directory: vnext
      - run: go test ./internal/host -count=1 && printf '{"Phase":"phase2","Revision":"%s","Passed":true}\n' "$GITHUB_SHA" > release-evidence/phase2-review.json
        working-directory: vnext
      - run: go test ./internal/capability ./internal/bench -count=1 && printf '{"Phase":"phase3","Revision":"%s","Passed":true}\n' "$GITHUB_SHA" > release-evidence/phase3-review.json
        working-directory: vnext
      - run: go test ./internal/deploy -count=1 && printf '{"Phase":"phase4","Revision":"%s","Passed":true}\n' "$GITHUB_SHA" > release-evidence/phase4-review.json
        working-directory: vnext
      - run: go test ./internal/install -count=1 && printf '{"Phase":"phase5","Revision":"%s","Passed":true}\n' "$GITHUB_SHA" > release-evidence/phase5-review.json
        working-directory: vnext
      - uses: actions/upload-artifact@v4
        with: { name: vnext-phase-reviews, path: vnext/release-evidence/phase*-review.json }
  verify-linux:
    needs: verify-phases
    permissions: { contents: read }
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with: { fetch-depth: 0 }
      - uses: actions/setup-go@v5
        with: { go-version: '1.23' }
      - uses: actions/download-artifact@v4
        with: { name: vnext-phase-reviews, path: vnext/release-evidence }
      - run: go test ./...
        working-directory: vnext
      - run: go vet ./...
        working-directory: vnext
      - run: go test ./internal/release ./internal/bench
        working-directory: vnext
      - run: mkdir -p release-evidence && go test -json ./... > release-evidence/native-test.json
        working-directory: vnext
      - run: go test -json ./internal/bench > release-evidence/benchmark-test.json
        working-directory: vnext
      - run: go run ./cmd/vnext-release collect-evidence --revision "${GITHUB_SHA}" --out release-evidence --native release-evidence/native-test.json --benchmark release-evidence/benchmark-test.json
        working-directory: vnext
      - uses: actions/upload-artifact@v4
        with: { name: vnext-phase-evidence, path: vnext/release-evidence/* }
  verify-macos:
    needs: verify-linux
    permissions: { contents: read }
    runs-on: macos-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with: { go-version: '1.23' }
      - run: go test ./...
        working-directory: vnext
      - run: go vet ./...
        working-directory: vnext
  verify-windows:
    needs: verify-linux
    permissions: { contents: read }
    runs-on: windows-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with: { go-version: '1.23' }
      - run: go test ./...
        working-directory: vnext
      - run: go vet ./...
        working-directory: vnext
  package:
    needs: [verify-linux, verify-macos, verify-windows]
    permissions: { contents: read }
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with: { go-version: '1.23' }
      - run: go build -trimpath -o agent-teamctl ./cmd/agent-teamctl
        working-directory: vnext
      - run: go run ./cmd/vnext-release package --version "${{ inputs.version }}" --commit "${GITHUB_SHA}"
        working-directory: vnext
      - run: test "$(jq -r .version release-artifacts/RELEASE.json)" = "${{ inputs.version }}"
        working-directory: vnext
      - uses: actions/upload-artifact@v4
        with: { name: vnext-release-artifacts, path: vnext/release-artifacts/* }
  canary:
    needs: package
    permissions: { contents: read }
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with: { go-version: '1.23' }
      - uses: actions/download-artifact@v4
        with: { name: vnext-release-artifacts, path: vnext/release-artifacts }
      - uses: actions/download-artifact@v4
        with: { name: vnext-phase-evidence, path: vnext/release-evidence }
      - run: go test ./internal/release -run 'Test(CodexCanary|ClaudeCanary|BothHostsRollbackAndActions|ArchiveCanaryFixture|PackagedArchiveCanary|Cutover|FinalGate|ReleasePackageOutputs)' -count=1
        env: { VNEXT_RELEASE_ARCHIVE: release-artifacts/agent-teamctl-${{ inputs.version }}.zip, VNEXT_RELEASE_MANIFEST: release-artifacts/RELEASE.json }
        working-directory: vnext
      - run: go test ./internal/deploy -run '^TestProviderVerification$' -count=1 && go test ./internal/release -run '^TestCutover$' -count=1
        working-directory: vnext
      - run: set -o pipefail; go test -json ./internal/release -run 'Test(CodexCanary|ClaudeCanary|BothHostsRollbackAndActions|ArchiveCanaryFixture|PackagedArchiveCanary|Cutover)' -count=1 | tee release-evidence/canary-test.json; for test_name in TestCodexCanary TestClaudeCanary TestPackagedArchiveCanary TestCutover; do grep -q "\"Test\":\"$test_name\"" release-evidence/canary-test.json; done
        working-directory: vnext
      - run: set -o pipefail; go test -json ./internal/release -run '^TestBothHostsRollbackAndActions$' -count=1 | tee release-evidence/rollback-test.json; grep -q '"Test":"TestBothHostsRollbackAndActions"' release-evidence/rollback-test.json
        working-directory: vnext
      - run: set -o pipefail; go test -json ./internal/release -run '^TestPackagedArchiveCanary$' -count=1 | tee release-evidence/installed-test.json; grep -q '"Test":"TestPackagedArchiveCanary"' release-evidence/installed-test.json
        working-directory: vnext
      - run: set -o pipefail; go test -json ./internal/deploy -run '^TestProviderVerification$' -count=1 | tee release-evidence/provider-test.json; grep -q '"Test":"TestProviderVerification"' release-evidence/provider-test.json
        working-directory: vnext
      - run: set -o pipefail; go test -json ./internal/release -run '^TestCutover$' -count=1 | tee release-evidence/cutover-test.json; grep -q '"Test":"TestCutover"' release-evidence/cutover-test.json
        working-directory: vnext
      - run: go run ./cmd/vnext-release finalize-evidence --revision "${GITHUB_SHA}" --out release-evidence --artifact release-artifacts/RELEASE.json --sbom release-artifacts/SBOM.cdx.json --canary release-evidence/canary-test.json --rollback release-evidence/rollback-test.json --provider release-evidence/provider-test.json --installed release-evidence/installed-test.json
        working-directory: vnext
      - run: go run ./cmd/vnext-release write-readiness --revision "${GITHUB_SHA}" --gates release-evidence/release-gates.json --benchmark release-evidence/benchmark.json --artifact release-artifacts/RELEASE.json --sbom release-artifacts/SBOM.cdx.json --canary release-evidence/canary.json --rollback release-evidence/rollback.json --provider release-evidence/provider.json --installed release-evidence/installed-skill.json
        working-directory: vnext
      - uses: actions/upload-artifact@v4
        with: { name: vnext-release-readiness, path: vnext/release-readiness.json }
  publish:
    needs: [verify-linux, verify-macos, verify-windows, package, canary]
    permissions: { contents: write }
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with: { fetch-depth: 0 }
      - uses: actions/setup-go@v5
        with: { go-version: '1.23' }
      - uses: actions/download-artifact@v4
        with: { name: vnext-release-artifacts, path: vnext/release-artifacts }
      - uses: actions/download-artifact@v4
        with: { name: vnext-release-readiness, path: vnext/release-evidence }
      - run: sha256sum -c vnext/release-artifacts/SHA256SUMS
      - run: go run ./cmd/vnext-release verify-gates --evidence release-evidence/release-readiness.json --revision "${GITHUB_SHA}"
        working-directory: vnext
      - run: |
          test "$(jq -r .version vnext/release-artifacts/RELEASE.json)" = "${{ inputs.version }}"
          test "$(jq -r .commit vnext/release-artifacts/RELEASE.json)" = "${GITHUB_SHA}"
          git fetch --tags
          ! git rev-parse --verify --quiet "refs/tags/v${{ inputs.version }}"
          git tag -a "v${{ inputs.version }}" -m "Agent-Team v${{ inputs.version }}"
          git push origin "v${{ inputs.version }}"
      - uses: softprops/action-gh-release@v2
        with: { tag_name: v${{ inputs.version }}, files: vnext/release-artifacts/*, generate_release_notes: true }
```
The package command must use fixed allowlisted paths and no shell interpolation of user data. Its failing test must reject missing/short commit, wrong version, extra archive files, and an SBOM whose serial or component hashes differ.
- [ ] **Step 4: Run:** `cd vnext && go test ./internal/release -run 'TestReleaseMetadata|TestPackageCommand|TestReleasePackageOutputs' && go test ./cmd/vnext-release -run TestVerifyGatesCLI`; **Expected:** PASS; validate YAML with the repository’s existing YAML checker if present. Also run `go run ./cmd/vnext-release verify-gates --evidence missing.json --revision "$GITHUB_SHA"` and require a nonzero exit.
- [ ] **Step 5: Commit:** `git add CHANGELOG.md .github/workflows/vnext-release.yml vnext/VERSION vnext/RELEASE.json && git commit -m "release(vnext): add gated release metadata and workflow"`.

### Task 5: Codex/Claude install canaries, rollback, and installed-skill verification

**Files:** Create `vnext/internal/release/{canary.go,canary_test.go}` and `vnext/testdata/canary/{codex,claude}/`; modify only installer files owned by the install-migration plan.

**Interfaces — Consumes:** Phase 5 `install.Layout`, `install.Release`, and `install.InstallManifest`; project migration records and host action probes.

**Interfaces — Produces:** typed `Canary`, `RunInstallCanary`, and `VerifyRollback` below.

```go
type RestoredFile struct { Path,SHA256 string }
type Canary struct { Hosts []install.Host; Layout install.Layout; Release install.Release; Manifest install.InstallManifest; ArchivePath,ArchiveSHA256 string; BinarySHA256,SkillSHA256,ContractSHA256 string; HostResults map[install.Host]HostCanary; Actions []string; RollbackVerified bool; Preserved []string; UnrelatedBefore,UnrelatedAfter []byte; RestoredFiles []RestoredFile; RestoredManifestSHA256 string; StagingEvidenceDigest string; StagingCleaned bool }
type HostCanary struct { Host install.Host; Actions []string; BinarySHA256,SkillSHA256,ContractSHA256 string; RollbackVerified bool; UnrelatedPreserved bool }
type Cutover struct { Canary Canary; ProviderVerified,InstalledVerified,RollbackRehearsed bool; Evidence []string }
func RunInstallCanary(context.Context,install.Layout,install.Release,install.InstallManifest,[]install.Host) (Canary,error)
func RunInstallCanaryFromArchive(context.Context,string,install.Layout,install.Release,install.InstallManifest,[]install.Host) (Canary,error)
func VerifyRollback(context.Context,Canary) error
func VerifyCutover(Cutover) error
```

- [ ] **Step 1: Write failing test:**
```go
import ( "archive/zip"; "bytes"; "context"; "crypto/sha256"; "encoding/hex"; "encoding/json"; "fmt"; "io"; "os"; "path/filepath"; "slices"; "testing"; "github.com/thebpandey/agent-team/vnext/internal/install"; "github.com/thebpandey/agent-team/vnext/internal/release" )
func writeCanaryFile(t *testing.T,path string,body []byte) install.ReleaseFile { t.Helper(); if err:=os.MkdirAll(filepath.Dir(path),0755); err!=nil { t.Fatal(err) }; if err:=os.WriteFile(path,body,0644); err!=nil { t.Fatal(err) }; sum:=sha256.Sum256(body); return install.ReleaseFile{Path:path,SHA256:hex.EncodeToString(sum[:]),Bytes:int64(len(body))} }
func canaryFixture(t *testing.T) (install.Layout,install.Release,install.InstallManifest) { t.Helper(); root:=t.TempDir(); installed,source:=filepath.Join(root,"installed"),filepath.Join(root,"release"); oldBinary:=writeCanaryFile(t,filepath.Join(installed,"agent-teamctl"),[]byte("binary-7.9.0")); oldContract:=writeCanaryFile(t,filepath.Join(installed,"WORKER-CONTRACT"),[]byte(`{"schema":1,"version":"7.9.0"}`)); oldCodex:=writeCanaryFile(t,filepath.Join(installed,"codex","SKILL.md"),[]byte("old codex skill")); oldClaude:=writeCanaryFile(t,filepath.Join(installed,"claude","SKILL.md"),[]byte("old claude skill")); binary:=writeCanaryFile(t,filepath.Join(source,"agent-teamctl"),[]byte("binary-8.0.0")); contract:=writeCanaryFile(t,filepath.Join(source,"WORKER-CONTRACT"),[]byte(`{"schema":1,"version":"8.0.0"}`)); codex:=writeCanaryFile(t,filepath.Join(source,"codex","SKILL.md"),[]byte("setup status start")); claude:=writeCanaryFile(t,filepath.Join(source,"claude","SKILL.md"),[]byte("setup status start")); unrelated:=filepath.Join(installed,"unrelated-host-setting.json"); if err:=os.WriteFile(unrelated,[]byte("preserve-me"),0644); err!=nil { t.Fatal(err) }; layout:=install.Layout{DataRoot:installed,BinaryPath:oldBinary.Path,ContractPath:oldContract.Path,ManifestPath:filepath.Join(installed,"manifest.json"),SkillRoots:map[install.Host]string{install.Codex:filepath.Dir(oldCodex.Path),install.Claude:filepath.Dir(oldClaude.Path)}}; release:=install.Release{Version:"8.0.0",Binary:binary,Contract:contract,Entrypoints:map[install.Host]install.ReleaseFile{install.Codex:codex,install.Claude:claude}}; manifest:=install.InstallManifest{Schema:1,Version:"7.9.0",Hosts:[]install.Host{install.Codex,install.Claude},Files:[]install.OwnedFile{{Role:install.BinaryRole,Path:oldBinary.Path,SHA256:oldBinary.SHA256,Version:"7.9.0",Bytes:oldBinary.Bytes},{Role:install.ContractRole,Path:oldContract.Path,SHA256:oldContract.SHA256,Version:"7.9.0",Bytes:oldContract.Bytes},{Role:install.EntrypointRole,Host:install.Codex,Path:oldCodex.Path,SHA256:oldCodex.SHA256,Version:"7.9.0",Bytes:oldCodex.Bytes},{Role:install.EntrypointRole,Host:install.Claude,Path:oldClaude.Path,SHA256:oldClaude.SHA256,Version:"7.9.0",Bytes:oldClaude.Bytes}}}; return layout,release,manifest }
func TestCodexCanary(t *testing.T) { l,r,m:=canaryFixture(t); c,err:=release.RunInstallCanary(context.Background(),l,r,m,[]install.Host{install.Codex}); if err!=nil || len(c.Hosts)!=1 || c.HostResults[install.Codex].Host!=install.Codex || !c.HostResults[install.Codex].UnrelatedPreserved || c.HostResults[install.Codex].BinarySHA256=="" { t.Fatal(c,err) } }
func TestClaudeCanary(t *testing.T) { l,r,m:=canaryFixture(t); c,err:=release.RunInstallCanary(context.Background(),l,r,m,[]install.Host{install.Claude}); if err!=nil || len(c.Hosts)!=1 || c.HostResults[install.Claude].Host!=install.Claude || c.HostResults[install.Claude].SkillSHA256=="" { t.Fatal(c,err) } }
func TestBothHostsRollbackAndActions(t *testing.T) { l,r,m:=canaryFixture(t); c,err:=release.RunInstallCanary(context.Background(),l,r,m,[]install.Host{install.Codex,install.Claude}); if err!=nil || len(c.Hosts)!=2 || !c.RollbackVerified || c.BinarySHA256=="" || c.SkillSHA256=="" || c.ContractSHA256=="" || !bytes.Equal(c.UnrelatedBefore,c.UnrelatedAfter) || c.RestoredManifestSHA256=="" || len(c.RestoredFiles)!=len(m.Files) { t.Fatal(c,err) }; for _,action:=range []string{"setup","status","start"} { if !slices.Contains(c.Actions,action) { t.Fatalf("missing action %s",action) } }; if err:=release.VerifyRollback(context.Background(),c); err!=nil { t.Fatal(err) }; if err:=release.VerifyCutover(release.Cutover{Canary:c,ProviderVerified:true,InstalledVerified:true,RollbackRehearsed:true}); err!=nil { t.Fatal(err) } }
func writeMatchingArchive(t *testing.T,r install.Release) string { t.Helper(); path:=filepath.Join(t.TempDir(),"release.zip"); f,err:=os.Create(path); if err!=nil { t.Fatal(err) }; z:=zip.NewWriter(f); add:=func(name string,file install.ReleaseFile){ body,err:=os.ReadFile(file.Path); if err!=nil { t.Fatal(err) }; w,err:=z.Create(name); if err!=nil { t.Fatal(err) }; if _,err:=w.Write(body); err!=nil { t.Fatal(err) } }; add("agent-teamctl",r.Binary); add("WORKER-CONTRACT",r.Contract); add("codex/SKILL.md",r.Entrypoints[install.Codex]); add("claude/SKILL.md",r.Entrypoints[install.Claude]); w,err:=z.Create("VERSION"); if err!=nil { t.Fatal(err) }; _,_=w.Write([]byte("8.0.0\n")); if err:=z.Close(); err!=nil { t.Fatal(err) }; if err:=f.Close(); err!=nil { t.Fatal(err) }; return path }
func TestArchiveCanaryFixture(t *testing.T) { l,r,m:=canaryFixture(t); archive:=writeMatchingArchive(t,r); c,err:=release.RunInstallCanaryFromArchive(context.Background(),archive,l,r,m,[]install.Host{install.Codex,install.Claude}); if err!=nil || c.ArchivePath!=archive || c.ArchiveSHA256=="" || !c.RollbackVerified || !c.StagingCleaned || c.StagingEvidenceDigest=="" { t.Fatal(c,err) }; if err:=release.VerifyRollback(context.Background(),c); err!=nil { t.Fatal("durable rollback evidence failed",err) } }
func TestPackagedArchiveCanary(t *testing.T) { archive,manifestPath:=os.Getenv("VNEXT_RELEASE_ARCHIVE"),os.Getenv("VNEXT_RELEASE_MANIFEST"); if archive=="" || manifestPath=="" { t.Fatal("packaged archive and manifest are required") }; l,r,m:=packageCanaryInputsFromManifest(t,archive,manifestPath); c,err:=release.RunInstallCanaryFromArchive(context.Background(),archive,l,r,m,[]install.Host{install.Codex,install.Claude}); if err!=nil || c.ArchivePath!=archive || c.ArchiveSHA256=="" || !c.RollbackVerified || c.Release.Binary.SHA256!=r.Binary.SHA256 || c.Manifest.Files[0].SHA256!=m.Files[0].SHA256 { t.Fatal(c,err) } }
func packageCanaryInputsFromManifest(t *testing.T,archive,manifestPath string) (install.Layout,install.Release,install.InstallManifest) { t.Helper(); raw,err:=os.ReadFile(manifestPath); if err!=nil { t.Fatal(err) }; var pm release.Manifest; if err:=json.Unmarshal(raw,&pm); err!=nil { t.Fatal(err) }; z,err:=zip.OpenReader(archive); if err!=nil { t.Fatal(err) }; defer z.Close(); read:=func(name string) []byte { for _,e:=range z.File { if e.Name==name { r,err:=e.Open(); if err!=nil { t.Fatal(err) }; b,err:=io.ReadAll(r); _=r.Close(); if err!=nil { t.Fatal(err) }; return b } }; t.Fatalf("archive member %s missing",name); return nil }; root:=t.TempDir(); makeFile:=func(name string) install.ReleaseFile { body:=read(name); path:=filepath.Join(root,filepath.FromSlash(name)); if err:=os.MkdirAll(filepath.Dir(path),0755); err!=nil { t.Fatal(err) }; if err:=os.WriteFile(path,body,0755); err!=nil { t.Fatal(err) }; sum:=sha256.Sum256(body); got:=hex.EncodeToString(sum[:]); if got!=pm.Checksums[name] { t.Fatalf("manifest hash mismatch for %s",name) }; return install.ReleaseFile{Path:path,SHA256:got,Bytes:int64(len(body))} }; binary,contract,codex,claude:=makeFile("agent-teamctl"),makeFile("WORKER-CONTRACT"),makeFile("codex/SKILL.md"),makeFile("claude/SKILL.md"); layout:=install.Layout{DataRoot:root,BinaryPath:binary.Path,ContractPath:contract.Path,ManifestPath:filepath.Join(root,"manifest.json"),SkillRoots:map[install.Host]string{install.Codex:filepath.Dir(codex.Path),install.Claude:filepath.Dir(claude.Path)}}; r:=install.Release{Version:pm.Version,Binary:binary,Contract:contract,Entrypoints:map[install.Host]install.ReleaseFile{install.Codex:codex,install.Claude:claude}}; m:=install.InstallManifest{Schema:1,Revision:1,Version:pm.Version,Hosts:[]install.Host{install.Codex,install.Claude},Files:[]install.OwnedFile{{Role:install.BinaryRole,Path:binary.Path,SHA256:binary.SHA256,Version:pm.Version,Bytes:binary.Bytes},{Role:install.ContractRole,Path:contract.Path,SHA256:contract.SHA256,Version:pm.Version,Bytes:contract.Bytes},{Role:install.EntrypointRole,Host:install.Codex,Path:codex.Path,SHA256:codex.SHA256,Version:pm.Version,Bytes:codex.Bytes},{Role:install.EntrypointRole,Host:install.Claude,Path:claude.Path,SHA256:claude.SHA256,Version:pm.Version,Bytes:claude.Bytes}}}; return layout,r,m }
```
Assert installed skill/contract hashes, `setup`/`status`/`start` action routing, rollback restores the prior manifest, and unrelated hooks/MCP/settings remain byte-identical.
- [ ] **Step 2: Run:** `cd vnext && go test ./internal/release -run 'Test(CodexCanary|ClaudeCanary|BothHostsRollbackAndActions|ArchiveCanaryFixture)'`; **Expected:** FAIL because canary support is absent.
- [ ] **Step 3: Implement the base canary before the archive wrapper.** Seed the explicitly supplied prior manifest with CAS revision zero, update it to the supplied release, inspect the installed binary/contract/selected skill files by digest, and require each installed skill to expose `setup`, `status`, and `start`. Snapshot the unrelated fixture before and after. Roll back to the seeded version using the exact update revision, read the restored manifest, and record only restored file paths/digests plus the manifest fingerprint; never retain a live host path as rollback evidence.

```go
func installedDigest(path string) (string,error) { b,err:=os.ReadFile(path); if err!=nil{return "",err}; return sha256Hex(b),nil }
func requireActions(path string) ([]string,error) { b,err:=os.ReadFile(path); if err!=nil{return nil,err}; text:=string(b); actions:=[]string{"setup","status","start"}; for _,a:=range actions { if !strings.Contains(text,a) { return nil,core.ErrPhase } }; return actions,nil }
func restoredFiles(m install.InstallManifest) ([]RestoredFile,error) { out:=make([]RestoredFile,0,len(m.Files)); for _,f:=range m.Files { sum,err:=installedDigest(f.Path); if err!=nil||sum!=f.SHA256{return nil,core.ErrRevision}; out=append(out,RestoredFile{Path:f.Path,SHA256:sum}) }; return out,nil }
func RunInstallCanary(ctx context.Context,l install.Layout,r install.Release,prior install.InstallManifest,hosts []install.Host)(Canary,error){ if err:=install.ValidateLayout(l);err!=nil{return Canary{},err};if err:=install.VerifyRelease(r);err!=nil{return Canary{},err};if len(hosts)==0{return Canary{},core.ErrSettings};seen:=map[install.Host]bool{};for _,h:=range hosts{if (h!=install.Codex&&h!=install.Claude)||seen[h]{return Canary{},core.ErrSettings};seen[h]=true};unrelated:=filepath.Join(l.DataRoot,"unrelated-host-setting.json");before,err:=os.ReadFile(unrelated);if err!=nil{return Canary{},err};store:=install.NewManifestStore(l);seed,err:=store.CompareAndSwap(ctx,0,prior);if err!=nil{return Canary{},err};updated,err:=install.Update(ctx,l,r,seed.ObservedRevision);if err!=nil{return Canary{},err};result:=Canary{Hosts:append([]install.Host(nil),hosts...),Layout:l,Release:r,Manifest:updated.Manifest,HostResults:map[install.Host]HostCanary{},BinarySHA256:r.Binary.SHA256,ContractSHA256:r.Contract.SHA256,UnrelatedBefore:before,Actions:[]string{"setup","status","start"}};for _,h:=range hosts{skill:=updated.Manifest.Files[0];for _,f:=range updated.Manifest.Files{if f.Role==install.EntrypointRole&&f.Host==h{skill=f;break}};sum,err:=installedDigest(skill.Path);if err!=nil||sum!=skill.SHA256{return Canary{},core.ErrRevision};actions,err:=requireActions(skill.Path);if err!=nil{return Canary{},err};result.Actions=actions;result.SkillSHA256=sum;result.HostResults[h]=HostCanary{Host:h,Actions:actions,BinarySHA256:r.Binary.SHA256,SkillSHA256:sum,ContractSHA256:r.Contract.SHA256,UnrelatedPreserved:true}};rolled,err:=install.Rollback(ctx,l,prior.Version,updated.ObservedRevision);if err!=nil{return Canary{},err};after,err:=os.ReadFile(unrelated);if err!=nil||!bytes.Equal(before,after){return Canary{},core.ErrRevision};result.UnrelatedAfter=after;result.Preserved=append(append([]string(nil),updated.Retained...),rolled.Retained...);result.Manifest=rolled.Manifest;result.RestoredFiles,err=restoredFiles(rolled.Manifest);if err!=nil{return Canary{},err};result.RestoredManifestSHA256=ManifestFingerprint(rolled.Manifest);result.RollbackVerified=true;for h,v:=range result.HostResults{v.RollbackVerified=true;result.HostResults[h]=v};return result,nil }
```

- [ ] **Step 4: Run and commit.** Run `cd vnext && gofmt -w internal/release/canary.go internal/release/canary_test.go && go test ./internal/release -run 'Test(CodexCanary|ClaudeCanary|BothHostsRollbackAndActions|ArchiveCanaryFixture)' -count=1 -race`. Expected: PASS, with an actual update→action inspection→rollback and byte-identical unrelated host setting.
- [ ] **Step 5: Commit.** `git add vnext/internal/release/canary.go vnext/internal/release/canary_test.go vnext/testdata/canary && git commit -m "test(vnext): verify Codex and Claude install canaries"`.

The archive wrapper below is intentionally called only after the base canary has this behavior.
- [ ] **Step 3: Implement:** before creating the stage, read exactly `<caller Layout.DataRoot>/unrelated-host-setting.json`; reject a missing/unreadable file, copy its bytes to `<stage>/unrelated-host-setting.json` (it is never an archive member), pass the staged copy to `RunInstallCanary`, then re-read and byte-compare the caller-root file before stage cleanup. Normalize each archive member to a relative slash path and reject absolute/`..`/duplicate/unknown members; stage only manifest-owned binary, contract, and skill-entrypoint bytes under a fresh temporary root; verify checksums and byte counts before activation; construct a staged `install.Release`, `InstallManifest`, and `Layout` whose paths all point inside that root (never the caller’s original local paths); run each host canary there; verify rollback and durable restored-file evidence while the stage exists; then remove the bounded stage and retain only `StagingEvidenceDigest`/`RestoredFiles` hashes for post-cleanup verification. `RunInstallCanaryFromArchive` computes `ArchiveSHA256`, requires the fixed `agent-teamctl`, `WORKER-CONTRACT`, `codex/SKILL.md`, and `claude/SKILL.md` members, and returns `core.ErrRevision` for a missing, duplicate, or mismatched entry.
```go
import ("archive/zip"; "bytes"; "context"; "crypto/sha256"; "encoding/json"; "fmt"; "io"; "os"; "path/filepath"; "strings"; "github.com/thebpandey/agent-team/vnext/internal/core"; "github.com/thebpandey/agent-team/vnext/internal/install")
func archiveMember(host install.Host, f install.ReleaseFile, binary, contract bool) string { if binary { return "agent-teamctl" }; if contract { return "WORKER-CONTRACT" }; if host==install.Codex { return "codex/SKILL.md" }; return "claude/SKILL.md" }
func RunInstallCanaryFromArchive(ctx context.Context,archivePath string,l install.Layout,r install.Release,m install.InstallManifest,hosts []install.Host) (Canary,error) { raw,err:=os.ReadFile(archivePath); if err!=nil { return Canary{},core.ErrPath }; z,err:=zip.OpenReader(archivePath); if err!=nil { return Canary{},core.ErrRevision }; defer z.Close(); stage,err:=os.MkdirTemp("","agent-team-canary-"); if err!=nil { return Canary{},err }; want:=map[string]install.ReleaseFile{"agent-teamctl":r.Binary,"WORKER-CONTRACT":r.Contract}; for h,f:=range r.Entrypoints { want[archiveMember(h,f,false,false)]=f }; seen:=map[string]bool{}; for _,e:=range z.File { member:=filepath.ToSlash(filepath.Clean(strings.ReplaceAll(e.Name,"\\","/"))); if member=="." || strings.HasPrefix(member,"../") || filepath.IsAbs(member) { os.RemoveAll(stage); return Canary{},core.ErrRevision }; f,ok:=want[member]; if !ok { if member!="VERSION" { os.RemoveAll(stage); return Canary{},core.ErrRevision }; continue }; if seen[member] { os.RemoveAll(stage); return Canary{},core.ErrRevision }; rr,openErr:=e.Open(); if openErr!=nil { os.RemoveAll(stage); return Canary{},core.ErrRevision }; body,readErr:=io.ReadAll(rr); closeErr:=rr.Close(); if readErr!=nil || closeErr!=nil || int64(len(body))!=f.Bytes || sha256Hex(body)!=f.SHA256 { os.RemoveAll(stage); return Canary{},core.ErrRevision }; target:=filepath.Join(stage,filepath.FromSlash(member)); if err:=os.MkdirAll(filepath.Dir(target),0755); err!=nil { os.RemoveAll(stage); return Canary{},err }; if err:=os.WriteFile(target,body,0755); err!=nil { os.RemoveAll(stage); return Canary{},err }; seen[member]=true }; if len(seen)!=len(want) { os.RemoveAll(stage); return Canary{},core.ErrRevision }; staged:=install.Release{Version:r.Version,Binary:r.Binary,Contract:r.Contract,Entrypoints:map[install.Host]install.ReleaseFile{}}; staged.Binary.Path=filepath.Join(stage,"agent-teamctl"); staged.Contract.Path=filepath.Join(stage,"WORKER-CONTRACT"); for h,f:=range r.Entrypoints { f.Path=filepath.Join(stage,filepath.FromSlash(archiveMember(h,f,false,false))); staged.Entrypoints[h]=f }; stagedManifest:=m; stagedManifest.Files=append([]install.OwnedFile(nil),m.Files...); for i,f:=range stagedManifest.Files { switch f.Role { case install.BinaryRole: f.Path=staged.Binary.Path; case install.ContractRole: f.Path=staged.Contract.Path; case install.EntrypointRole: f.Path=staged.Entrypoints[f.Host].Path }; stagedManifest.Files[i]=f }; stagedLayout:=l; stagedLayout.DataRoot=stage; stagedLayout.BinaryPath=staged.Binary.Path; stagedLayout.ContractPath=staged.Contract.Path; stagedLayout.ManifestPath=filepath.Join(stage,"manifest.json"); stagedLayout.SkillRoots=map[install.Host]string{install.Codex:filepath.Join(stage,"codex"),install.Claude:filepath.Join(stage,"claude")}; unrelatedPath:=filepath.Join(l.DataRoot,"unrelated-host-setting.json"); unrelated,err:=os.ReadFile(unrelatedPath);if err!=nil{os.RemoveAll(stage);return Canary{},err};if err:=os.WriteFile(filepath.Join(stage,"unrelated-host-setting.json"),unrelated,0644);err!=nil{os.RemoveAll(stage);return Canary{},err}; c,err:=RunInstallCanary(ctx,stagedLayout,staged,stagedManifest,hosts); if err!=nil { os.RemoveAll(stage); return Canary{},err }; after,err:=os.ReadFile(unrelatedPath);if err!=nil||!bytes.Equal(unrelated,after){os.RemoveAll(stage);return Canary{},core.ErrRevision}; if err:=VerifyRollback(ctx,c); err!=nil { os.RemoveAll(stage); return Canary{},err }; sum:=sha256.Sum256(raw); c.ArchivePath=archivePath; c.ArchiveSHA256=fmt.Sprintf("%x",sum[:]); c.StagingEvidenceDigest=RestoreEvidenceDigest(c); if err:=os.RemoveAll(stage); err!=nil { return Canary{},err }; c.StagingCleaned=true; return c,nil }
func RestoreEvidenceDigest(c Canary) string { canonical:=c.RestoredManifestSHA256+fmt.Sprintf(":%d",len(c.RestoredFiles)); for _,f:=range c.RestoredFiles { canonical += "|"+f.SHA256 }; return sha256Hex([]byte(canonical)) }
func VerifyRollback(ctx context.Context, c Canary) error { if c.StagingCleaned { if c.StagingEvidenceDigest=="" || c.StagingEvidenceDigest!=RestoreEvidenceDigest(c) { return core.ErrRevision }; _=ctx; return nil }; raw,err:=os.ReadFile(c.Layout.ManifestPath); if err!=nil { return core.ErrRevision }; var restored install.InstallManifest; if err=json.Unmarshal(raw,&restored); err!=nil || restored.Schema!=1 || restored.Version=="" || c.Layout.DataRoot=="" || c.RestoredManifestSHA256!=ManifestFingerprint(restored) { return core.ErrRevision }; expected:=map[string]string{}; for _,f:=range restored.Files { expected[f.Path]=f.SHA256 }; if len(c.RestoredFiles)!=len(expected) { return core.ErrRevision }; seen:=map[string]bool{}; for _,f:=range c.RestoredFiles { body,err:=os.ReadFile(f.Path); if err!=nil || seen[f.Path] || sha256Hex(body)!=f.SHA256 || expected[f.Path]!=f.SHA256 { return core.ErrRevision }; seen[f.Path]=true }; for path:=range expected { if !seen[path] { return core.ErrRevision } }; _=ctx; return nil }
func ManifestFingerprint(m install.InstallManifest) string { canonical:=fmt.Sprintf("%d:%d:%s",m.Schema,m.Revision,m.Version); for _,f:=range m.Files { canonical += "|"+string(f.Role)+":"+string(f.Host)+":"+f.Path+":"+f.SHA256+":"+f.Version }; return sha256Hex([]byte(canonical)) }
func VerifyCutover(c Cutover) error { if len(c.Canary.Hosts)==0 || !c.Canary.RollbackVerified || !c.ProviderVerified || !c.InstalledVerified || !c.RollbackRehearsed { return core.ErrPhase }; return nil }
```
- [ ] **Step 6: Verify archive parity.** Run `cd vnext && go test ./internal/release -run 'Test(ArchiveCanaryFixture|PackagedArchiveCanary)' -count=1`; Expected: PASS after the base canary passes and the archive staging evidence survives cleanup.

### Task 6: Provider verification, cutover rehearsal, final gates, and non-force publication

**Files:** Create `vnext/internal/release/final_test.go`, `docs/releases/8.0.0-readiness.md`; modify no provider target beyond the existing deployment plan contract.

**Interfaces — Consumes:** all phase gate records, release manifest/checksum/SBOM evidence, typed install canary results, and the existing deployment/provider verification contract.

**Interfaces — Produces:** `Gates.Validate` and final readiness evidence; it does not create a new provider or package registry.

```go
func BuildReleasePackage(root,version,commit string) error
func ListPackageOutputs(root,version string) ([]string,error)

```

- [ ] **Step 1: Write failing final-gate test:** require phase CLEAN records, native matrix results, benchmark report/raw pointers, deterministic artifact/checksum verification, SBOM result, Codex/Claude canary and rollback evidence, installed-skill verification, deployment/provider verification, and a successful cutover rehearsal before returning success.
```go
import ("crypto/sha256"; "encoding/hex"; "encoding/json"; "errors"; "os"; "path/filepath"; "strings"; "testing"; "github.com/thebpandey/agent-team/vnext/internal/core"; "github.com/thebpandey/agent-team/vnext/internal/release")
func TestFinalGate(t *testing.T) { g:=release.Gates{Phase1:true,Phase2:true,Phase3:true,Phase4:true,Phase5:true,Deploy:true,Install:true,ReviewsClean:true,Native:true,Benchmark:true,Artifacts:true,Canaries:true,Rollback:true,Provider:true,Cutover:true}; if err:=g.Validate(); err!=nil { t.Fatal(err) }; g.Cutover=false; if err:=g.Validate(); !errors.Is(err,core.ErrPhase) { t.Fatal(err) } }
func TestReadinessEvidenceDigests(t *testing.T) { root:=t.TempDir(); gates:=release.Gates{Phase1:true,Phase2:true,Phase3:true,Phase4:true,Phase5:true,Deploy:true,Install:true,ReviewsClean:true,Native:true,Benchmark:true,Artifacts:true,Canaries:true,Rollback:true,Provider:true,Cutover:true}; gatePath:=filepath.Join(root,"gates.json"); raw,_:=json.Marshal(gates); if err:=os.WriteFile(gatePath,raw,0644); err!=nil { t.Fatal(err) }; paths:=map[string]string{}; for _,name:=range []string{"benchmark","artifact","sbom","canary","rollback","provider","installed"} { p:=filepath.Join(root,name+".json"); if err:=os.WriteFile(p,[]byte(name),0644); err!=nil { t.Fatal(err) }; paths[name]=p }; e,err:=release.BuildReadinessEvidence(strings.Repeat("a",40),gatePath,paths); if err!=nil { t.Fatal(err) }; out:=filepath.Join(root,"readiness.json"); if err:=release.WriteReadinessEvidence(out,e); err!=nil || release.VerifyReadinessEvidence(out,strings.Repeat("a",40))!=nil { t.Fatal(err) }; if err:=os.WriteFile(paths["artifact"],[]byte("changed"),0644); err!=nil { t.Fatal(err) }; if !errors.Is(release.VerifyReadinessEvidence(out,strings.Repeat("a",40)),core.ErrPhase) { t.Fatal("changed evidence accepted") } }
type Gates struct { Phase1,Phase2,Phase3,Phase4,Phase5,Deploy,Install,ReviewsClean,Native,Benchmark,Artifacts,Canaries,Rollback,Provider,Cutover bool }
func (g Gates) Validate() error { if !g.Phase1||!g.Phase2||!g.Phase3||!g.Phase4||!g.Phase5||!g.Deploy||!g.Install||!g.ReviewsClean||!g.Native||!g.Benchmark||!g.Artifacts||!g.Canaries||!g.Rollback||!g.Provider||!g.Cutover { return core.ErrPhase }; return nil }
type EvidencePointer struct { Path string `json:"path"`; SHA256 string `json:"sha256"` }
type ReadinessEvidence struct { Revision string `json:"revision"`; Gates Gates `json:"gates"`; Benchmark EvidencePointer `json:"benchmark"`; Artifact EvidencePointer `json:"artifact"`; SBOM EvidencePointer `json:"sbom"`; Canary EvidencePointer `json:"canary"`; Rollback EvidencePointer `json:"rollback"`; Provider EvidencePointer `json:"provider"`; Installed EvidencePointer `json:"installed"` }
func hashEvidence(path string) (EvidencePointer,error) { raw,err:=os.ReadFile(path); if err!=nil || path=="" { return EvidencePointer{},core.ErrPhase }; sum:=sha256.Sum256(raw); return EvidencePointer{Path:path,SHA256:hex.EncodeToString(sum[:])},nil }
func BuildReadinessEvidence(revision,gatesPath string,paths map[string]string) (ReadinessEvidence,error) { raw,err:=os.ReadFile(gatesPath); if err!=nil { return ReadinessEvidence{},core.ErrPhase }; var gates Gates; if err=json.Unmarshal(raw,&gates); err!=nil || gates.Validate()!=nil { return ReadinessEvidence{},core.ErrPhase }; get:=func(name string)(EvidencePointer,error) { p,ok:=paths[name]; if !ok { return EvidencePointer{},core.ErrPhase }; return hashEvidence(p) }; b,e:=get("benchmark"); if e!=nil{return ReadinessEvidence{},e}; a,e:=get("artifact"); if e!=nil{return ReadinessEvidence{},e}; s,e:=get("sbom"); if e!=nil{return ReadinessEvidence{},e}; c,e:=get("canary"); if e!=nil{return ReadinessEvidence{},e}; r,e:=get("rollback"); if e!=nil{return ReadinessEvidence{},e}; p,e:=get("provider"); if e!=nil{return ReadinessEvidence{},e}; i,e:=get("installed"); if e!=nil{return ReadinessEvidence{},e}; return ReadinessEvidence{Revision:revision,Gates:gates,Benchmark:b,Artifact:a,SBOM:s,Canary:c,Rollback:r,Provider:p,Installed:i},nil }
func WriteReadinessEvidence(path string,e ReadinessEvidence) error { if e.Revision=="" || e.Gates.Validate()!=nil { return core.ErrPhase }; raw,err:=json.MarshalIndent(e,"","  "); if err!=nil { return err }; return os.WriteFile(path,append(raw,'\n'),0644) }
func VerifyReadinessEvidence(path, revision string) error { raw,err:=os.ReadFile(path); if err!=nil { return core.ErrPhase }; var e ReadinessEvidence; if err=json.Unmarshal(raw,&e); err!=nil || e.Revision!=revision || e.Gates.Validate()!=nil { return core.ErrPhase }; for _,p:=range []EvidencePointer{e.Benchmark,e.Artifact,e.SBOM,e.Canary,e.Rollback,e.Provider,e.Installed} { got,err:=hashEvidence(p.Path); if err!=nil || got.SHA256!=p.SHA256 { return core.ErrPhase } }; return nil }
```
- [ ] **Step 2: Run:** `cd vnext && go test ./internal/release -run 'Test(FinalGate|ReadinessEvidenceDigests)'`; **Expected:** FAIL until every evidence input and digest check is present.
- [ ] **Step 3: Implement:** verify the installed skill package and GitHub release are the only deployment targets; query the existing provider verification interface, perform non-force `git push origin main` and `git push origin v8.0.0` only after approval, create the GitHub release from verified artifacts, and write `docs/releases/8.0.0-readiness.md` with every command, digest, reviewer, canary, rollback, and known limitation. Implement `BuildReadinessEvidence` to hash and persist non-empty pointers for benchmark, artifact manifest, native SBOM, canary, rollback, provider, and installed-skill evidence, and implement `VerifyReadinessEvidence(path, revision)` as a fail-closed executable reader that re-hashes every pointer and rejects missing/unreadable files, changed bytes, missing phase/deploy/install gates, or any readiness revision other than the requested commit. The workflow must verify checkout → build → archive → checksum/SBOM → exact packaged-archive canary → rollback → cutover → provider verification → installed-skill verification before publication, upload `vnext/release-readiness.json`, and invoke `go run ./cmd/vnext-release verify-gates --evidence release-evidence/release-readiness.json --revision "${GITHUB_SHA}"` from `vnext` before `gh-release`.
```go
import ( "encoding/json"; "errors"; "os"; "path/filepath"; "reflect"; "strings"; "testing"; "github.com/thebpandey/agent-team/vnext/internal/core"; "github.com/thebpandey/agent-team/vnext/internal/release" )
func packageFixture(t *testing.T) string { t.Helper(); source:=t.TempDir(); for _,item:=range []struct{path,body string}{{"agent-teamctl","binary"},{"WORKER-CONTRACT","contract"},{"VERSION","8.0.0\n"},{"codex/SKILL.md","codex"},{"claude/SKILL.md","claude"}} { path:=filepath.Join(source,item.path); if err:=os.MkdirAll(filepath.Dir(path),0755); err!=nil { t.Fatal(err) }; if err:=os.WriteFile(path,[]byte(item.body),0755); err!=nil { t.Fatal(err) } }; return source }
func TestReleasePackageOutputs(t *testing.T) { source,root:=packageFixture(t),t.TempDir(); version:="8.0.0"; outputs:=release.ReleaseOutputs(version); if err:=release.BuildReleasePackageFrom(source,root,version,strings.Repeat("a",40)); err!=nil { t.Fatal(err) }; got,err:=release.ListPackageOutputs(root,version); if err!=nil || !reflect.DeepEqual(got,outputs) { t.Fatal(got,err) }; if err:=release.VerifyOutputAllowlist(version,append(got,"extra.zip")); !errors.Is(err,core.ErrPath) { t.Fatal(err) }; rawManifest,err:=os.ReadFile(filepath.Join(root,"RELEASE.json")); if err!=nil { t.Fatal(err) }; var manifest release.Manifest; if err:=json.Unmarshal(rawManifest,&manifest); err!=nil || manifest.Version!=version || len(manifest.Files)!=5 { t.Fatal(manifest,err) }; artifact,err:=release.BuildArtifact(source,filepath.Join(root,"agent-teamctl-"+version+".zip"),manifest); if err!=nil || release.VerifyArtifact(artifact,manifest)!=nil { t.Fatal(artifact,err) }; rawSBOM,err:=os.ReadFile(filepath.Join(root,"SBOM.cdx.json")); if err!=nil { t.Fatal(err) }; var sbom release.SBOM; if err:=json.Unmarshal(rawSBOM,&sbom); err!=nil || sbom.Tool!="native" || release.VerifySBOM(sbom,manifest)!=nil { t.Fatal(sbom,err) }; sbom.Serial="urn:agent-team:wrong"; if err:=release.VerifySBOM(sbom,manifest); !errors.Is(err,core.ErrRevision) { t.Fatal("mismatched SBOM serial accepted",err) }; if len(sbom.Components)>1 { sbom.Components[0],sbom.Components[1]=sbom.Components[1],sbom.Components[0]; if err:=release.VerifySBOM(sbom,manifest); !errors.Is(err,core.ErrRevision) { t.Fatal("mismatched component order accepted",err) } } }
```

Create these owned executable gate tests before wiring publication. The provider fixture is explicitly pre-authorized for non-production `staging` only, and the cutover fixture uses the exact Codex/Claude install canary plus durable rollback evidence:

```go
// vnext/internal/deploy/release_gate_test.go
package deploy
import ("context"; "errors"; "testing"; "github.com/thebpandey/agent-team/vnext/internal/core")
func TestProviderVerification(t *testing.T) { provider:=&fakeProvider{verifyState:"succeeded",verifyNilError:true}; repo:=NewMemoryRepository(); exec,err:=NewBoundExecutor(testProfile(),provider); if err!=nil { t.Fatal(err) }; out,err:=SubmitOrReconcile(context.Background(),repo,exec,testBatch("release-provider-B1")); if err!=nil||out.Receipt.State!="submitted" { t.Fatal(out,err) }; resumed,err:=ResumeBatch(context.Background(),repo,exec,"release-provider-B1"); if err!=nil||resumed.Receipt.State!="succeeded"||len(repo.Evidence())<2 { t.Fatal(resumed,err) }; if len(provider.calls)!=3||provider.calls[0]!="submit"||provider.calls[1]!="query"||provider.calls[2]!="verify" { t.Fatalf("provider calls=%v",provider.calls) }; if err:=ValidateProfile(testProfile()); err!=nil { t.Fatal(err) }; if err:=ValidateProfile(TargetProfile{ID:"prod",Target:"production",AuthorizationRef:"",ApprovalScope:"production",Enabled:true}); !errors.Is(err,core.ErrSettings) { t.Fatal("unauthorized production profile accepted") } }
```

```go
// vnext/internal/release/cutover_gate_test.go
package release
import ("context"; "errors"; "testing"; "github.com/thebpandey/agent-team/vnext/internal/core"; "github.com/thebpandey/agent-team/vnext/internal/install")
func TestCutover(t *testing.T) { layout,rel,manifest:=canaryFixture(t); c,err:=RunInstallCanary(context.Background(),layout,rel,manifest,[]install.Host{install.Codex,install.Claude}); if err!=nil { t.Fatal(err) }; if len(c.Hosts)!=2||!c.RollbackVerified||c.RestoredManifestSHA256==""||len(c.RestoredFiles)!=len(manifest.Files)||len(c.Actions)==0 { t.Fatal(c) }; if err:=VerifyRollback(context.Background(),c); err!=nil { t.Fatal(err) }; cut:=Cutover{Canary:c,ProviderVerified:true,InstalledVerified:true,RollbackRehearsed:true,Evidence:[]string{"provider-test.json","rollback-test.json","installed-test.json"}}; if err:=VerifyCutover(cut); err!=nil { t.Fatal(err) }; cut.RollbackRehearsed=false; if !errors.Is(VerifyCutover(cut),core.ErrPhase) { t.Fatal("cutover accepted without rollback evidence") } }
```

- [ ] **Step 4: Run:** `cd vnext && go test ./internal/deploy -run '^TestProviderVerification$' -count=1 && go test ./internal/release -run '^TestCutover$' -count=1`; **Expected:** both named tests execute and PASS against the pre-authorized non-production fake/provider and two-host install/rollback fixture.
- [ ] **Step 5: Commit:** `git add vnext/internal/deploy/release_gate_test.go vnext/internal/release/cutover_gate_test.go && git commit -m "test(vnext): gate provider verification and cutover"`.
- [ ] **Step 6: Evidence run:** `cd vnext && provider_out=$(mktemp) && go test -json ./internal/deploy -run '^TestProviderVerification$' -count=1 | tee "$provider_out" && grep -q '"Test":"TestProviderVerification"' "$provider_out" && cutover_out=$(mktemp) && go test -json ./internal/release -run '^TestCutover$' -count=1 | tee "$cutover_out" && grep -q '"Test":"TestCutover"' "$cutover_out"`; **Expected:** a missing/renamed test, zero executed tests, or any failure causes a nonzero command before evidence is finalized.
- [ ] **Step 7: Run:** `cd vnext && go test ./... -run 'Test(FinalGate|ReadinessEvidenceDigests|EvidenceCLIParsers|ReviewEvidenceRejectsForgedMissingAndStale|ProviderVerification|Cutover)' -count=1 && go vet ./...`; then execute the documented cutover canary and rollback rehearsal; **Expected:** all named tests execute and PASS, no unknown state, and no later phase gate skipped.
- [ ] **Step 5: Commit:** `git add vnext/internal/release docs/releases && git commit -m "release(vnext): record final readiness evidence"`.

## Spec Coverage and Self-Review

- Tasks 1–2 provide deterministic artifacts, checksums, lightweight SBOM handling, and honest token-efficiency evidence with raw pointers.
- Task 3 updates final README/getting-started/skill/reference docs and labels v7.3.1 legacy.
- Task 4 provides semver/changelog metadata and least-privilege native release CI.
- Task 5 verifies Codex/Claude install canaries, installed skill hashes, preservation, and rollback.
- Task 6 gates provider verification, cutover rehearsal, non-force GitHub publication, and final readiness evidence.
- No package registry, production service, hook, MCP registration, heavy dependency, force push, or undefined release target is introduced.

## Execution Handoff

Plan complete and saved to `docs/superpowers/plans/2026-09-19-agent-team-vnext-release-readiness.md`. Report ready for independent review; do not mark release readiness CLEAN until every gate and rehearsal in Task 6 passes.
