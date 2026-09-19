# Agent-Team vNext Install and Migration Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship a checksum-verified, reversible native installation lifecycle and an explicit per-project v7-to-vNext Codex/Claude canary without mutating hooks, MCP, settings, credentials, or unrelated files.

**Architecture:** The install package owns one versioned manifest and one atomic CAS writer. It installs only a release binary, immutable worker contract, and explicitly selected host skill entrypoints. The migration package records redacted v7 provenance, runs independent Codex and Claude canaries against current tracker/Git/receipt facts, and can roll back only vNext-owned files.

**Tech Stack:** Go standard library, SHA-256, UTF-8 JSON, `os.Rename` same-directory replacement, bounded Windows retry, `go test`, and `go vet`; module `github.com/thebpandey/agent-team/vnext`.

**Spec:** `docs/superpowers/specs/2026-09-18-agent-team-vnext-design.md`

## Global Constraints

- Host selection is explicit: `install --host codex|claude|both`; no implicit host discovery or global host mutation.
- Never write hooks, MCP registrations, global host/model settings, shell profiles, credentials, environment variables, or v7 files.
- Install/update/rollback/uninstall touch only the user-data root, the manifest-owned binary/contract, and selected manifest-owned host skill entrypoints.
- A changed or unknown file is retained and reported; uninstall never deletes by path alone.
- v7 files are hashed/redacted provenance only. Claims, leases, workers, runs, checkpoints, hooks, and ownership are never imported as vNext authority.
- Cutover is per project, explicitly approved, side-by-side, and reversible. Codex and Claude have separate typed identities; switching hosts transfers no lease, process, or worker.
- Native Linux, macOS, and Windows are required. No bash, `HOME`, symlink, or executable-bit assumption is allowed.

## File Structure

| Path | Responsibility |
| --- | --- |
| `vnext/internal/install/types.go` | Authoritative host, layout, release, owned-file, manifest, CAS, and lifecycle contracts. |
| `vnext/internal/install/layout.go` | Deterministic platform paths and containment checks. |
| `vnext/internal/install/release.go` | Release fixture validation and SHA-256 verification. |
| `vnext/internal/install/install.go` | Atomic install/update/rollback/uninstall and manifest CAS. |
| `vnext/internal/install/*_test.go` | Native-path, ownership, lifecycle, lock, and preservation tests. |
| `vnext/internal/migrate/types.go` | Provenance, observation, canary, and migration-store contracts. |
| `vnext/internal/migrate/inventory.go` | Bounded v7 inventory and redacted digest. |
| `vnext/internal/migrate/cutover.go` | Explicit two-host canary, switch, and rollback. |
| `vnext/internal/migrate/*_test.go` | Codex/Claude side-by-side and v7 non-import tests. |
| `vnext/internal/cli/app.go`, `parser.go` | Existing canonical `cli.Run` management dispatch. |
| `vnext/internal/core/types.go` | One management callback field on existing `core.Dependencies`. |
| `vnext/cmd/agent-teamctl/main.go` | Native binary wiring; no second CLI runner. |
| `.github/workflows/vnext-install.yml` | Complete native test, vet, and cross-build matrix. |

### Task 1: Define the authoritative manifest contract and verified platform layout

**Files:**

- Create: `vnext/internal/install/types.go`
- Create: `vnext/internal/install/layout.go`
- Create: `vnext/internal/install/release.go`
- Test: `vnext/internal/install/release_test.go`
- Test: `vnext/internal/install/testutil_test.go`

**Interfaces:**

`types.go` is the only owner of these names. Later tasks must import them and must not redeclare them:

```go
package install

import (
    "context"
)

type Host string
const ( Codex Host = "codex"; Claude Host = "claude" )
type FileRole string
const ( BinaryRole FileRole = "binary"; ContractRole FileRole = "worker-contract"; EntrypointRole FileRole = "skill-entrypoint" )
type ReleaseFile struct { Path, SHA256 string; Bytes int64 }
type Release struct { Version string; Binary, Contract ReleaseFile; Entrypoints map[Host]ReleaseFile }
type OwnedFile struct { Role FileRole; Host Host; Path, SHA256, Version string; Bytes int64 }
type Backup struct { Role FileRole; Host Host; Path, SHA256, Version string; Bytes int64 }
type InstallManifest struct { Schema int; Revision uint64; Version string; Hosts []Host; Files []OwnedFile; Backups []Backup }
type CASKind string
const ( CASCreated CASKind = "created"; CASUpdated CASKind = "updated"; CASDuplicate CASKind = "duplicate"; CASStale CASKind = "stale" )
type CASOutcome struct { Kind CASKind; Manifest InstallManifest; ExpectedRevision, ObservedRevision uint64; Idempotent bool; Retained []string }
type Layout struct { DataRoot, BinaryPath, ContractPath, ManifestPath string; SkillRoots map[Host]string }
type ManifestStore struct { Path string }
func NewManifestStore(Layout) *ManifestStore
func (*ManifestStore) Read(context.Context) (InstallManifest, error)
func (*ManifestStore) CompareAndSwap(context.Context, uint64, InstallManifest) (CASOutcome, error)
func ResolveLayout(string, map[string]string) (Layout, error)
func ValidateLayout(Layout) error
func VerifyRelease(Release) error
func sha256File(string) (string, int64, error)
```

The CAS contract is exact: an absent manifest accepts expected revision `0`; an existing manifest accepts only its exact revision; stale writes return `CASStale` and `core.ErrRevision` without changing bytes; an identical version/file-hash set returns `CASDuplicate` with `Idempotent:true`; every successful write increments `Revision` exactly once and atomically replaces the prior manifest.

- [ ] **Step 1: Write the failing layout and release tests.** Create the one reusable fixture writer below in `testutil_test.go`; it writes each release input once, computes its bytes and digest, and never uses a shell or executable bit.

```go
package install_test

import (
    "crypto/sha256"
    "fmt"
    "os"
    "path/filepath"
    "testing"
    install "github.com/thebpandey/agent-team/vnext/internal/install"
)

func writeReleaseFixture(t *testing.T, root string) install.Release {
    t.Helper()
    files := map[string]string{"agent-teamctl.bin":"vnext-binary", "WORKER-CONTRACT":"contract-v1", "SKILL.md":"entrypoint-v1"}
    paths := make(map[string]string, len(files))
    for name, contents := range files { path := filepath.Join(root, name); if err := os.WriteFile(path, []byte(contents), 0600); err != nil { t.Fatal(err) }; paths[name] = path }
    file := func(name string) install.ReleaseFile { b, err := os.ReadFile(paths[name]); if err != nil { t.Fatal(err) }; return install.ReleaseFile{Path:paths[name], SHA256:fmt.Sprintf("%x", sha256.Sum256(b)), Bytes:int64(len(b))} }
    return install.Release{Version:"1.0.0", Binary:file("agent-teamctl.bin"), Contract:file("WORKER-CONTRACT"), Entrypoints:map[install.Host]install.ReleaseFile{install.Codex:file("SKILL.md"), install.Claude:file("SKILL.md")}}
}
```

```go
package install_test

import (
    "path/filepath"
    "strings"
    "testing"
    install "github.com/thebpandey/agent-team/vnext/internal/install"
)

func TestVerifyReleaseAndResolveNativeLayouts(t *testing.T) {
    root := t.TempDir(); release := writeReleaseFixture(t, root)
    if err := install.VerifyRelease(release); err != nil { t.Fatal(err) }
    release.Binary.SHA256 = "wrong"; if err := install.VerifyRelease(release); err == nil { t.Fatal("hash mismatch accepted") }
    linux, err := install.ResolveLayout("linux", map[string]string{"XDG_DATA_HOME":filepath.Join(root,"data"), "CODEX_HOME":filepath.Join(root,"codex"), "CLAUDE_HOME":filepath.Join(root,"claude")}); if err != nil { t.Fatal(err) }
    darwin, err := install.ResolveLayout("darwin", map[string]string{"XDG_DATA_HOME":filepath.Join(root,"mac-data"), "CODEX_HOME":filepath.Join(root,"mac-codex"), "CLAUDE_HOME":filepath.Join(root,"mac-claude")}); if err != nil { t.Fatal(err) }
    windows, err := install.ResolveLayout("windows", map[string]string{"LOCALAPPDATA":filepath.Join(root,"local")}); if err != nil { t.Fatal(err) }
    if !strings.Contains(windows.DataRoot, "AgentTeam") || linux.DataRoot == windows.DataRoot || darwin.DataRoot == linux.DataRoot { t.Fatal(linux, darwin, windows) }
    if err := install.ValidateLayout(linux); err != nil { t.Fatal(err) }
}
```

- [ ] **Step 2: Run the focused tests and verify failure.** Run `cd vnext && go test ./internal/install -run 'TestVerifyReleaseAndResolveNativeLayouts' -count=1`. Expected: FAIL with missing `install` package symbols.
- [ ] **Step 3: Implement the contract.** `ResolveLayout` uses explicit `XDG_DATA_HOME` on Unix and `LOCALAPPDATA` on Windows, with `os.UserConfigDir` only as a process-local fallback. It derives `DataRoot`, `BinaryPath`, `ContractPath`, `ManifestPath`, and separate Codex/Claude skill roots; it rejects empty roots, relative paths, and a skill path outside its root. `VerifyRelease` requires version, regular files, exact byte counts, lowercase SHA-256 equality, both required artifact roles, and selected host entrypoints. `ManifestStore` uses bounded JSON, same-directory temp files, `Sync`, close, rename, and revision CAS.
- [ ] **Step 4: Run formatting and native tests.** Run `cd vnext && gofmt -w internal/install/types.go internal/install/layout.go internal/install/release.go internal/install/release_test.go internal/install/testutil_test.go && go test ./internal/install -run 'TestVerifyReleaseAndResolveNativeLayouts' -count=1`. Expected: PASS on Linux, macOS, and Windows.
- [ ] **Step 5: Commit.** Run `git add vnext/internal/install/types.go vnext/internal/install/layout.go vnext/internal/install/release.go vnext/internal/install/release_test.go vnext/internal/install/testutil_test.go && git commit -m "feat(vnext): define verified install manifest contract"`.

### Task 2: Implement install, update, rollback, and ownership-safe uninstall

**Files:**

- Create: `vnext/internal/install/install.go`
- Test: `vnext/internal/install/install_test.go`
- Modify: `vnext/internal/install/types.go` only for a newly required named implementation helper

**Interfaces:**

```go
package install

import (
    "context"
    "io/fs"
)

func Install(context.Context, Layout, Release, []Host, uint64) (CASOutcome, error)
func Update(context.Context, Layout, Release, uint64) (CASOutcome, error)
func Rollback(context.Context, Layout, string, uint64) (CASOutcome, error)
func Uninstall(context.Context, Layout, uint64) ([]string, CASOutcome, error)
func AtomicReplace(Layout, string, []byte, fs.FileMode) error
```

`Install` validates selected hosts and release hashes, creates only absent target files, records every installed `OwnedFile`, and stores prior bytes as `Backup` only when the manifest explicitly owned the prior version. `Update` replaces a current file only when its on-disk digest equals the manifest digest; otherwise it retains the changed file and returns its path in `CASOutcome.Retained`. `Rollback` restores the named backup only when the current file still matches the manifest version. `Uninstall` removes only files whose current digest matches the manifest digest and returns retained paths for changed or unknown files. All four operations use `ManifestStore.CompareAndSwap` with the caller's expected revision.

- [ ] **Step 1: Write failing lifecycle and preservation tests.** Reuse `writeReleaseFixture` from Task 1 and verify binary, contract, both selected entrypoints, changed owned content, unrelated files, stale CAS, and exact backup restoration.

```go
package install_test

import (
    "bytes"
    "crypto/sha256"
    "context"
    "fmt"
    "errors"
    "os"
    "path/filepath"
    "runtime"
    "testing"
    install "github.com/thebpandey/agent-team/vnext/internal/install"
    "github.com/thebpandey/agent-team/vnext/internal/core"
)

func TestInstallUpdateRollbackUninstallPreserveChangedFiles(t *testing.T) {
    root := t.TempDir(); release := writeReleaseFixture(t, root)
    layout, err := install.ResolveLayout("linux", map[string]string{"XDG_DATA_HOME":filepath.Join(root,"data"), "CODEX_HOME":filepath.Join(root,"codex"), "CLAUDE_HOME":filepath.Join(root,"claude")}); if err != nil { t.Fatal(err) }
    unrelated := filepath.Join(root, "unrelated.txt"); if err := os.WriteFile(unrelated, []byte("keep"), 0600); err != nil { t.Fatal(err) }
    first, err := install.Install(context.Background(), layout, release, []install.Host{install.Codex, install.Claude}, 0); if err != nil { t.Fatal(err) }
    oldBinary, _ := os.ReadFile(release.Binary.Path); oldContract, _ := os.ReadFile(release.Contract.Path); oldEntrypoint, _ := os.ReadFile(release.Entrypoints[install.Codex].Path)
    changed := filepath.Join(layout.SkillRoots[install.Codex], "agent-team-vnext", "SKILL.md"); if err := os.WriteFile(changed, []byte("user change"), 0600); err != nil { t.Fatal(err) }
    release2Root := filepath.Join(root, "release-1.1.0"); if err := os.MkdirAll(release2Root, 0700); err != nil { t.Fatal(err) }; release2 := writeReleaseFixture(t, release2Root); release2.Version = "1.1.0"
    rewriteReleaseFile(t, &release2.Binary, "vnext-binary-v1.1.0"); rewriteReleaseFile(t, &release2.Contract, "contract-v1.1.0")
    for host, file := range release2.Entrypoints { rewriteReleaseFile(t, &file, "entrypoint-v1.1.0"); release2.Entrypoints[host] = file }
    next, err := install.Update(context.Background(), layout, release2, first.Manifest.Revision); if err != nil { t.Fatal(err) }; if len(next.Retained) != 1 || len(next.Manifest.Backups) == 0 { t.Fatal(next.Retained, next.Manifest.Backups) }
    backupChecked := false; for _, backup := range next.Manifest.Backups { if backup.Role == install.BinaryRole { got, readErr := os.ReadFile(backup.Path); if readErr != nil || !bytes.Equal(got, oldBinary) { t.Fatalf("binary backup=%q err=%v", got, readErr) }; backupChecked = true } }; if !backupChecked { t.Fatal("binary backup missing") }
    rolledBack, err := install.Rollback(context.Background(), layout, release.Version, next.Manifest.Revision); if err != nil { t.Fatal(err) }; if rolledBack.Manifest.Revision != next.Manifest.Revision+1 { t.Fatalf("rollback revision=%d want=%d",rolledBack.Manifest.Revision,next.Manifest.Revision+1) }
    if got, _ := os.ReadFile(layout.BinaryPath); !bytes.Equal(got, oldBinary) { t.Fatalf("binary rollback=%q", got) }; if got, _ := os.ReadFile(layout.ContractPath); !bytes.Equal(got, oldContract) { t.Fatalf("contract rollback=%q", got) }; claudePath := filepath.Join(layout.SkillRoots[install.Claude], "agent-team-vnext", "SKILL.md"); if got, _ := os.ReadFile(claudePath); !bytes.Equal(got, oldEntrypoint) { t.Fatalf("entrypoint rollback=%q", got) }
    retained, _, err := install.Uninstall(context.Background(), layout, rolledBack.Manifest.Revision); if err != nil { t.Fatal(err) }; if len(retained) == 0 { t.Fatal("changed file was deleted") }
    if got, _ := os.ReadFile(changed); string(got) != "user change" { t.Fatalf("changed=%q", got) }
    for _, path := range []string{layout.BinaryPath, layout.ContractPath, claudePath} { if _, statErr := os.Stat(path); !errors.Is(statErr, os.ErrNotExist) { t.Fatalf("owned file remains path=%s err=%v", path, statErr) } }
    if got, _ := os.ReadFile(unrelated); string(got) != "keep" { t.Fatalf("unrelated=%q", got) }
}
func rewriteReleaseFile(t *testing.T, file *install.ReleaseFile, contents string) { t.Helper(); if err := os.WriteFile(file.Path, []byte(contents), 0600); err != nil { t.Fatal(err) }; b, err := os.ReadFile(file.Path); if err != nil { t.Fatal(err) }; file.SHA256 = fmt.Sprintf("%x", sha256.Sum256(b)); file.Bytes = int64(len(b)) }
func TestManifestCASRejectsStaleWriter(t *testing.T) {
    root := t.TempDir(); release := writeReleaseFixture(t, root); layout, _ := install.ResolveLayout("linux", map[string]string{"XDG_DATA_HOME":filepath.Join(root,"data"), "CODEX_HOME":filepath.Join(root,"codex"), "CLAUDE_HOME":filepath.Join(root,"claude")})
    first, err := install.Install(context.Background(), layout, release, []install.Host{install.Codex}, 0); if err != nil { t.Fatal(err) }
    _, err = install.Update(context.Background(), layout, release, 0); if !errors.Is(err, core.ErrRevision) { t.Fatal(err) }; if first.Manifest.Revision != 1 { t.Fatal(first) }
}
func TestPOSIXAtomic(t *testing.T) {
    if runtime.GOOS == "windows" { t.Skip("POSIX atomic mode is covered by the Windows-specific test") }
    root := t.TempDir(); layout,err:=install.ResolveLayout("linux",map[string]string{"XDG_DATA_HOME":root,"CODEX_HOME":filepath.Join(root,"codex"),"CLAUDE_HOME":filepath.Join(root,"claude")}); if err!=nil { t.Fatal(err) }; target := filepath.Join(layout.DataRoot, "owned.bin")
    if err := install.AtomicReplace(layout,target, []byte("atomic"), 0600); err != nil { t.Fatal(err) }
    if got, _ := os.ReadFile(target); string(got) != "atomic" { t.Fatal(string(got)) }
    escape:=filepath.Join(filepath.Dir(root),"outside-owned.bin"); if err:=install.AtomicReplace(layout,escape,[]byte("escape"),0600); !errors.Is(err,core.ErrPath) { t.Fatal("root escape accepted",err) }
    if runtime.GOOS != "windows" { if mode := mustFileMode(t, target).Perm(); mode != 0600 { t.Fatalf("mode=%o", mode) } }
}
func TestWindowsSharing(t *testing.T) {
    if runtime.GOOS != "windows" { t.Skip("Windows sharing-lock branch") }
    root := t.TempDir(); layout, err := install.ResolveLayout("windows", map[string]string{"LOCALAPPDATA":root, "CODEX_HOME":filepath.Join(root,"codex"), "CLAUDE_HOME":filepath.Join(root,"claude")}); if err != nil { t.Fatal(err) }; target := filepath.Join(layout.DataRoot, "locked.bin")
    if err := os.WriteFile(target, []byte("old"), 0600); err != nil { t.Fatal(err) }; lock, err := os.Open(target); if err != nil { t.Fatal(err) }; defer lock.Close()
    replaceErr := install.AtomicReplace(layout, target, []byte("new"), 0600); got, readErr := os.ReadFile(target); if readErr != nil { t.Fatal(readErr) }; if replaceErr != nil && !bytes.Equal(got, []byte("old")) { t.Fatalf("failed replacement changed target=%q", got) }; if replaceErr == nil && !bytes.Equal(got, []byte("new")) { t.Fatalf("successful replacement=%q", got) }
}
func TestOwnership(t *testing.T) {
    root := t.TempDir(); release := writeReleaseFixture(t, root); layout, err := install.ResolveLayout("linux", map[string]string{"XDG_DATA_HOME":filepath.Join(root,"data"), "CODEX_HOME":filepath.Join(root,"codex"), "CLAUDE_HOME":filepath.Join(root,"claude")}); if err != nil { t.Fatal(err) }
    manifest, err := install.Install(context.Background(), layout, release, []install.Host{install.Codex}, 0); if err != nil { t.Fatal(err) }; changed := filepath.Join(layout.SkillRoots[install.Codex], "agent-team-vnext", "SKILL.md"); if err := os.WriteFile(changed, []byte("user-owned"), 0600); err != nil { t.Fatal(err) }
    retained, _, err := install.Uninstall(context.Background(), layout, manifest.Manifest.Revision); if err != nil || len(retained) != 1 || retained[0] != changed { t.Fatal(retained, err) }; if _, err := os.Stat(changed); err != nil { t.Fatal("changed file was deleted", err) }; if _, err := os.Stat(layout.BinaryPath); !errors.Is(err, os.ErrNotExist) { t.Fatalf("unchanged binary remains err=%v", err) }
}
func mustFileMode(t *testing.T, path string) os.FileMode { t.Helper(); info, err := os.Stat(path); if err != nil { t.Fatal(err) }; return info.Mode() }
```

- [ ] **Step 2: Run the failing lifecycle test.** Run `cd vnext && go test ./internal/install -run 'TestInstallUpdateRollbackUninstallPreserveChangedFiles|TestManifestCASRejectsStaleWriter' -count=1`. Expected: FAIL because lifecycle functions are absent.
- [ ] **Step 3: Implement the lifecycle.** `AtomicReplace(layout Layout, target string, data []byte, mode fs.FileMode)` creates a random temp file beside the destination, validates target containment under `layout.DataRoot`/owned roots, writes exact bytes, calls `Sync`, closes, applies mode only on non-Windows, renames, and removes the temp on failure. On Windows it retries sharing/access-denied rename failures three times with bounded 25/50/100 ms backoff; after the final failure it returns the error and leaves the prior file untouched. Path containment is checked before every write/delete. No operation follows symlinks or accepts a path outside `Layout` roots.
- [ ] **Step 4: Run lifecycle, portability, and race checks.** Run `cd vnext && gofmt -w internal/install/install.go internal/install/install_test.go && go test ./internal/install -count=1 -race`; expected PASS. Run `cd vnext && go test ./internal/install -run 'TestWindowsSharing|TestPOSIXAtomic|TestOwnership' -count=1`; expected PASS or an OS-gated skip only where the host cannot exercise that branch; these names are defined above, so no zero-match command can falsely pass.
- [ ] **Step 5: Commit.** Run `git add vnext/internal/install/install.go vnext/internal/install/install_test.go && git commit -m "feat(vnext): add reversible ownership-safe installation"`.

### Task 3: Record v7 provenance and run side-by-side Codex/Claude canaries

**Files:**

- Create: `vnext/internal/migrate/types.go`
- Create: `vnext/internal/migrate/inventory.go`
- Create: `vnext/internal/migrate/cutover.go`
- Test: `vnext/internal/migrate/migrate_test.go`

**Interfaces:**

```go
package migrate

import (
    "context"
    install "github.com/thebpandey/agent-team/vnext/internal/install"
    "github.com/thebpandey/agent-team/vnext/internal/core"
)

type V7File struct { Path, SHA256 string; Bytes int64 }
type Inventory struct { Project string; Files []V7File; Digest, BackupPath string }
type Observation struct { Host install.Host; Identity, Revision, TrackerDigest, ReceiptDigest string; State string }
type CanaryArtifact struct { Path, SHA256 string }
type CanaryRecord struct { core.RecordEnvelope; ID, Project, V7InventoryDigest string; Codex, Claude Observation; State, RollbackEvidence string; Owned []CanaryArtifact; Retained []string; Idempotent bool }
type CanaryRunner interface { Observe(context.Context, string, install.Host) (Observation, error) }
type Store struct { Root string }
func InventoryV7(context.Context, string, string) (Inventory, error)
func NewStore(string) *Store
func (*Store) CompareAndSwap(context.Context, uint64, CanaryRecord) (CanaryRecord, error)
func BeginCanary(context.Context, *Store, string, install.Host, bool, CanaryRunner) (CanaryRecord, error)
func ResumeCanary(context.Context, *Store, string, install.Host, uint64, string, CanaryRunner) (CanaryRecord, error)
func RollbackCanary(context.Context, *Store, string) (CanaryRecord, error)
```

The migration store writes `.agent-team/migration/v7-inventory.json` and `.agent-team/migration/canary-<id>.json` with schema `1`, monotonic revisions, bounded JSON, and same-directory atomic replacement. `InventoryV7` reads only an allowlisted project-local v7 metadata set, records path/size/hash, redacts content, and never copies a lease, owner, worker, credential, hook, or MCP value. `BeginCanary` requires `approved:true`, records one requested host, and does not alter v7. `ResumeCanary` accepts the caller's expected revision and the current `V7InventoryDigest`, rejects stale or mismatched snapshots, and returns `Idempotent:true` without incrementing for an identical repeated observation; it observes current tracker/Git/receipt facts independently for the selected host. Switching Codex to Claude creates no transfer record and uses no cross-harness lease. A project is cut over only after both typed host observations succeed. `RollbackCanary` verifies each `Owned` artifact hash, removes unchanged vNext-owned artifacts, retains changed/unknown paths in `Retained`, and persists terminal `State` plus `RollbackEvidence` while retaining v7 bytes.

- [ ] **Step 1: Write failing provenance and canary tests.** Use a fake runner with distinct identities and a v7 fixture containing a lease-like field; assert only its digest/path metadata is persisted, stale and mismatched inventory resumes are rejected, duplicate resumes are idempotent, and rollback removes unchanged owned canary artifacts while retaining changed paths and durable rollback evidence.

```go
package migrate_test

import (
    "bytes"
    "context"
    "crypto/sha256"
    "encoding/json"
    "errors"
    "fmt"
    "os"
    "path/filepath"
    "strings"
    "testing"
    install "github.com/thebpandey/agent-team/vnext/internal/install"
    migrate "github.com/thebpandey/agent-team/vnext/internal/migrate"
    "github.com/thebpandey/agent-team/vnext/internal/core"
)

type fakeRunner struct{}
func (fakeRunner) Observe(_ context.Context, project string, host install.Host) (migrate.Observation, error) { return migrate.Observation{Host:host, Identity:string(host)+"-identity", Revision:"git-1", TrackerDigest:"tracker-1", ReceiptDigest:project+"-receipt", State:"ready"}, nil }
func digestFile(t *testing.T, path string) string { t.Helper(); b, err := os.ReadFile(path); if err != nil { t.Fatal(err) }; return fmt.Sprintf("%x", sha256.Sum256(b)) }

func TestInventoryIsRedactedAndBothHostsCanaryWithoutTransfer(t *testing.T) {
    project := t.TempDir(); legacy := filepath.Join(project, "v7-state.json"); if err := os.WriteFile(legacy, []byte(`{"lease":"secret","owner":"v7"}`), 0600); err != nil { t.Fatal(err) }
    inv, err := migrate.InventoryV7(context.Background(), project, legacy); if err != nil || inv.Digest == "" || len(inv.Files) != 1 { t.Fatal(inv, err) }
    store := migrate.NewStore(project); codex, err := migrate.BeginCanary(context.Background(), store, project, install.Codex, true, fakeRunner{}); if err != nil { t.Fatal(err) }
    codexResumed, err := migrate.ResumeCanary(context.Background(), store, codex.ID, install.Codex, codex.Revision, codex.V7InventoryDigest, fakeRunner{}); if err != nil { t.Fatal(err) }
    duplicate, err := migrate.ResumeCanary(context.Background(), store, codex.ID, install.Codex, codexResumed.Revision, codex.V7InventoryDigest, fakeRunner{}); if err != nil || !duplicate.Idempotent || duplicate.Revision != codexResumed.Revision { t.Fatal(duplicate,err) }
    claude, err := migrate.ResumeCanary(context.Background(), store, codex.ID, install.Claude, duplicate.Revision, codex.V7InventoryDigest, fakeRunner{}); if err != nil { t.Fatal(err) }
    if claude.Codex.Identity == claude.Claude.Identity || claude.Codex.Host == claude.Claude.Host { t.Fatal(claude) }
    raw, err := os.ReadFile(filepath.Join(project, ".agent-team", "migration", "canary-"+codex.ID+".json")); if err != nil { t.Fatal(err) }
    if strings.Contains(string(raw), "secret") || strings.Contains(string(raw), "lease") { t.Fatal("v7 claim copied") }
    var record migrate.CanaryRecord; if json.Unmarshal(raw, &record) != nil || record.State != "ready" { t.Fatal(string(raw)) }
    if got, _ := os.ReadFile(legacy); string(got) != `{"lease":"secret","owner":"v7"}` { t.Fatal("legacy changed") }
}
func TestDeclinedCanaryWritesNothing(t *testing.T) {
    project := t.TempDir(); _, err := migrate.BeginCanary(context.Background(), migrate.NewStore(project), project, install.Codex, false, fakeRunner{}); if err == nil { t.Fatal("declined canary accepted") }
}
type failedRunner struct{}
func (failedRunner) Observe(context.Context, string, install.Host) (migrate.Observation, error) { return migrate.Observation{}, errors.New("host observation failed") }
func TestFailedObservationAndRollbackPreserveLegacy(t *testing.T) {
    project := t.TempDir(); legacy := filepath.Join(project, "v7-state.json"); original := []byte(`{"lease":"secret","owner":"v7"}`); if err := os.WriteFile(legacy, original, 0600); err != nil { t.Fatal(err) }
    store := migrate.NewStore(project); if _, err := migrate.BeginCanary(context.Background(), store, project, install.Codex, true, failedRunner{}); err == nil { t.Fatal("failed observation accepted") }
    record, err := migrate.BeginCanary(context.Background(), store, project, install.Codex, true, fakeRunner{}); if err != nil { t.Fatal(err) }
    cleanPath := filepath.Join(project, ".agent-team", "migration", "clean-owned.txt"); changedPath := filepath.Join(project, ".agent-team", "migration", "changed-owned.txt"); if err := os.WriteFile(cleanPath, []byte("owned"), 0600); err != nil { t.Fatal(err) }; if err := os.WriteFile(changedPath, []byte("owned"), 0600); err != nil { t.Fatal(err) }
    record.Owned = []migrate.CanaryArtifact{{Path:cleanPath, SHA256:digestFile(t, cleanPath)}, {Path:changedPath, SHA256:digestFile(t, changedPath)}}; record, err = store.CompareAndSwap(context.Background(), record.Revision, record); if err != nil { t.Fatal(err) }; if err := os.WriteFile(changedPath, []byte("user change"), 0600); err != nil { t.Fatal(err) }
    rolledBack, err := migrate.RollbackCanary(context.Background(), store, record.ID); if err != nil { t.Fatal(err) }; if rolledBack.State != "rolled_back" || rolledBack.RollbackEvidence == "" || len(rolledBack.Retained) != 1 || rolledBack.Retained[0] != changedPath { t.Fatal(rolledBack) }; if _, err := os.Stat(cleanPath); !errors.Is(err, os.ErrNotExist) { t.Fatalf("unchanged canary artifact remains err=%v", err) }; if got, _ := os.ReadFile(changedPath); string(got) != "user change" { t.Fatalf("changed canary artifact=%q", got) }
    raw, err := os.ReadFile(filepath.Join(project, ".agent-team", "migration", "canary-"+record.ID+".json")); if err != nil { t.Fatal(err) }; var persisted migrate.CanaryRecord; if err := json.Unmarshal(raw, &persisted); err != nil || persisted.State != "rolled_back" || persisted.RollbackEvidence == "" { t.Fatalf("rollback evidence not durable: %s err=%v", raw, err) }
    if got, _ := os.ReadFile(legacy); !bytes.Equal(got, original) { t.Fatal("rollback changed v7 bytes") }
}
func TestCanaryResumeRejectsStaleOrMismatchedSnapshot(t *testing.T) { project:=t.TempDir(); store:=migrate.NewStore(project); record,err:=migrate.BeginCanary(context.Background(),store,project,install.Codex,true,fakeRunner{}); if err!=nil { t.Fatal(err) }; if _,err:=migrate.ResumeCanary(context.Background(),store,record.ID,install.Codex,record.Revision,"wrong-inventory-digest",fakeRunner{}); err==nil { t.Fatal("mismatched inventory accepted") }; current,err:=migrate.ResumeCanary(context.Background(),store,record.ID,install.Codex,record.Revision,record.V7InventoryDigest,fakeRunner{}); if err!=nil { t.Fatal(err) }; if _,err:=migrate.ResumeCanary(context.Background(),store,record.ID,install.Claude,record.Revision,record.V7InventoryDigest,fakeRunner{}); !errors.Is(err,core.ErrRevision) { t.Fatal("stale resume accepted",err) }; if current.Revision<=record.Revision { t.Fatal(current) } }
```

- [ ] **Step 2: Run the failing migration tests.** Run `cd vnext && go test ./internal/migrate -run 'TestInventoryIsRedactedAndBothHostsCanaryWithoutTransfer|TestDeclinedCanaryWritesNothing|TestFailedObservationAndRollbackPreserveLegacy|TestCanaryResumeRejectsStaleOrMismatchedSnapshot' -count=1`. Expected: FAIL because migration types and storage are absent.
- [ ] **Step 3: Implement inventory and canary transitions.** Validate project containment and file size before hashing; write only redacted records. Require `BeginCanary` approval, require a matching inventory digest on resume, reject unknown host/state/revision, and make duplicate resume idempotent. `ResumeCanary` updates only the selected `Observation`; it never copies the other host's identity or operation. `RollbackCanary` verifies the canary-owned digest before removal and returns retained paths when the user changed a canary file.
- [ ] **Step 4: Run migration and race tests.** Run `cd vnext && gofmt -w internal/migrate/types.go internal/migrate/inventory.go internal/migrate/cutover.go internal/migrate/migrate_test.go && go test ./internal/migrate -run 'Test(InventoryIsRedactedAndBothHostsCanaryWithoutTransfer|DeclinedCanaryWritesNothing|FailedObservationAndRollbackPreserveLegacy|CanaryResumeRejectsStaleOrMismatchedSnapshot)' -count=1 -race`; expected PASS, including declined approval, stale revision, mismatched inventory snapshot, duplicate resume, failed observation, Codex/Claude identity separation, owned-artifact cleanup/retention, rollback evidence, and v7 preservation.
- [ ] **Step 5: Commit.** Run `git add vnext/internal/migrate && git commit -m "feat(vnext): add redacted dual-host canary migration"`.

### Task 4: Expose the exact management CLI and native release gate

**Files:**

- Modify: `vnext/internal/core/types.go` (`Dependencies` adds only `Management`)
- Modify: `vnext/internal/cli/parser.go`
- Modify: `vnext/internal/cli/app.go`
- Modify: `vnext/cmd/agent-teamctl/main.go`
- Test: `vnext/internal/cli/install_test.go`
- Create: `.github/workflows/vnext-install.yml`

**Interfaces:**

The only runner remains the Phase 1 signature `func Run(context.Context, []string, core.Dependencies) int`. Add this callback without importing `cli` from `core`:

```go
package core

import (
    "context"
    "errors"
    "io"
)

type ManagementAction func(context.Context, []string, io.Writer, io.Writer) int
var ErrOutputLimit = errors.New("management output limit exceeded")
const DefaultOutputLimit = 8192
type Dependencies struct { ProjectRoot string; Stdout, Stderr io.Writer; OutputLimit int; Confirmations map[string]bool; Management ManagementAction }
type BoundedOutput struct { Limit int; Data []byte; Overflow bool }
func (b *BoundedOutput) Write(p []byte) (int,error) { limit:=b.Limit; if limit<=0 { limit=DefaultOutputLimit }; if len(b.Data)+len(p)>limit { b.Overflow=true; return 0,ErrOutputLimit }; b.Data=append(b.Data,p...); return len(p),nil }
```

The CLI accepts exactly `install --host codex|claude|both [--json]`, `update --version <version> [--json]`, `rollback --version <version> [--json]`, and `uninstall [--json]`. It rejects unknown flags, missing values, `--host` on update/rollback, and any unrecognized management action. The callback receives original action arguments and uses `deps.Stdout`/`deps.Stderr`; there is no writer-parameter runner and no second parser entrypoint.

- [ ] **Step 1: Write the failing exact-CLI tests.** The tests call the Phase 1 runner signature directly and assert every valid install/update/rollback/uninstall form, exact callback argument preservation, malformed argument rejection, bounded JSON, and no callback for invalid commands.

```go
package cli_test

import (
    "bytes"
    "context"
    "encoding/json"
    "io"
    "reflect"
    "strings"
    "testing"
    "github.com/thebpandey/agent-team/vnext/internal/cli"
    "github.com/thebpandey/agent-team/vnext/internal/core"
)

func TestManagementUsesCanonicalRun(t *testing.T) {
    var out, errOut bytes.Buffer; calls := 0
    deps := core.Dependencies{Stdout:&out, Stderr:&errOut, Management:func(_ context.Context, args []string, stdout, _ io.Writer) int { calls++; if len(args) != 4 || args[0] != "install" || args[2] != "both" { t.Fatal(args) }; _, _ = stdout.Write([]byte(`{"ok":true}`)); return 0 }}
    if code := cli.Run(context.Background(), []string{"install", "--host", "both", "--json"}, deps); code != 0 || calls != 1 || !bytes.Contains(out.Bytes(), []byte(`"ok":true`)) { t.Fatal(code, calls, out.String()) }
}
func TestManagementRejectsInvalidShape(t *testing.T) {
    deps := core.Dependencies{Stdout:&bytes.Buffer{}, Stderr:&bytes.Buffer{}, Management:func(context.Context, []string, io.Writer, io.Writer) int { t.Fatal("callback called"); return 0 }}
    if code := cli.Run(context.Background(), []string{"install", "--host", "mars"}, deps); code == 0 { t.Fatal("invalid host accepted") }
    if code := cli.Run(context.Background(), []string{"update", "--host", "both"}, deps); code == 0 { t.Fatal("invalid update shape accepted") }
}
func TestManagementActionsAndJSONBound(t *testing.T) {
    calls:=0; var seen [][]string; var out,errOut bytes.Buffer; deps:=core.Dependencies{Stdout:&out,Stderr:&errOut,OutputLimit:64,Management:func(_ context.Context,args []string,stdout, _ io.Writer) int { calls++; seen=append(seen,append([]string(nil),args...)); if len(args)>0 && args[len(args)-1]=="--json" { _,_=stdout.Write([]byte(`{"ok":true}`)) }; return 0 }}
    valid := [][]string{{"install","--host","codex"},{"install","--host","claude","--json"},{"update","--version","1.2.3","--json"},{"rollback","--version","8.0.0","--json"},{"uninstall","--json"}}
    for _,args:=range valid { if code:=cli.Run(context.Background(),args,deps); code!=0 { t.Fatal(args,code) } }
    for _,args:=range [][]string{{"install"},{"install","--host"},{"update"},{"rollback","--version"},{"unknown"},{"uninstall","--bad"}} { if code:=cli.Run(context.Background(),args,deps); code==0 { t.Fatal("malformed command accepted",args) } }
    if calls!=len(valid) || !reflect.DeepEqual(seen,valid) { t.Fatal(calls,seen) }
    out.Reset(); deps.Management=func(_ context.Context, _ []string, stdout, _ io.Writer) int { _,_=stdout.Write([]byte(`{"ok":true,"padding":"`+strings.Repeat("x",128)+`"}`)); return 0 }; if code:=cli.Run(context.Background(),[]string{"uninstall","--json"},deps); code==0 || out.Len()>64 || !json.Valid(out.Bytes()) || string(out.Bytes())!="{\"ok\":false,\"error\":\"output_limit\"}" { t.Fatalf("bounded output code=%d bytes=%d json=%q",code,out.Len(),out.Bytes()) }
}
```

- [ ] **Step 2: Run the failing CLI tests.** Run `cd vnext && go test ./internal/cli -run 'TestManagement(UsesCanonicalRun|RejectsInvalidShape|ActionsAndJSONBound)' -count=1`. Expected: FAIL until parser and callback dispatch exist.
- [ ] **Step 3: Implement parser, wiring, and workflow.** Keep `cli.Run` as the sole dispatch function; parse into typed local management arguments, return stable nonzero errors for malformed input, and invoke the callback with `core.BoundedOutput` buffers. The default limit is `core.DefaultOutputLimit`; `Dependencies.OutputLimit` may lower it. If a callback exceeds the limit, discard the partial buffer, return a stable nonzero code, and emit exactly `{"ok":false,"error":"output_limit"}` through `deps.Stdout` (which must itself be bounded), preserving valid JSON and deterministic truncation/error behavior. `main.go` constructs `core.Dependencies` and wires the install package through the callback; it never mutates hooks, MCP, settings, or environment. Add the complete workflow below.

```yaml
name: vnext-install
on:
  push:
  pull_request:
jobs:
  native:
    strategy:
      fail-fast: false
      matrix:
        os: [ubuntu-latest, macos-latest, windows-latest]
    runs-on: ${{ matrix.os }}
    defaults:
      run:
        working-directory: vnext
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.23'
      - run: go test ./... -count=1
      - run: go vet ./...
      - run: go test ./internal/install ./internal/migrate ./internal/cli -run 'Test(Install|Manifest|Inventory|Canary|Management)' -count=1
  cross-build:
    runs-on: ubuntu-latest
    strategy:
      matrix:
        goos: [linux, darwin, windows]
        goarch: [amd64]
    defaults:
      run:
        working-directory: vnext
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.23'
      - run: go build ./cmd/agent-teamctl
        env:
          GOOS: ${{ matrix.goos }}
          GOARCH: ${{ matrix.goarch }}
```

- [ ] **Step 4: Run the complete release gate.** From the repository root run `gofmt -w vnext/internal/core/types.go vnext/internal/cli/parser.go vnext/internal/cli/app.go vnext/internal/cli/install_test.go vnext/cmd/agent-teamctl/main.go`, then run `cd vnext && go test ./... -count=1`, `go vet ./...`, and `go test ./internal/install ./internal/migrate ./internal/cli -run 'Test(Install|Manifest|Inventory|Canary|Management)' -count=1`. Expected: PASS. Run `git diff --check` and `rg -n 'hook|MCP|os\.Setenv|exec\.Command\(.*-c|os\.Remove(All)?\(' vnext/internal/install vnext/internal/migrate`; expected no shell invocation or mutation outside manifest-owned paths.
- [ ] **Step 5: Commit.** Run `git add vnext/internal/core/types.go vnext/internal/cli vnext/cmd/agent-teamctl .github/workflows/vnext-install.yml && git commit -m "feat(vnext): expose native install and migration lifecycle"`.

## Spec Coverage and Self-Review

- Task 1 defines every release, manifest, owned-file, backup, layout, and typed CAS symbol before any later task uses it; the fixture writes binary/contract/entrypoint bytes once and verifies all hashes.
- Task 2 proves install, update with changed release bytes, exact backup contents/restoration, uninstall deletion of unchanged owned files, expected-revision rejection, same-directory atomicity, Windows sharing-lock handling, permissions, path containment, modified-file retention, and unrelated-file preservation.
- Task 3 proves provenance-only v7 inventory, explicit approval, independent Codex/Claude identities, immediate host switching without transfer, failed observation, stale/mismatched/duplicate resume, owned-artifact cleanup/retention, durable rollback evidence, and rollback of only vNext-owned artifacts.
- Task 4 uses the exact Phase 1 `cli.Run(context.Context, []string, core.Dependencies) int`, provides a complete native matrix and cross-build, and forbids a second runner or host mutation.
- No task depends on a hook, daemon, lease, runtime migration, shell, `HOME`, symlink, executable bit, or provider credential.

## Execution Handoff

Plan is ready for independent review at `docs/superpowers/plans/2026-09-19-agent-team-vnext-install-migration.md`. Execute only after Phase 1–4 contracts compile; do not claim installation or migration success from fixture tests alone.
