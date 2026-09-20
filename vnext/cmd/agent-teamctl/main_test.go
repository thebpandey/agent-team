package main

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"

	"github.com/thebpandey/agent-team/vnext/internal/cli"
	"github.com/thebpandey/agent-team/vnext/internal/core"
	"github.com/thebpandey/agent-team/vnext/internal/install"
	releasepkg "github.com/thebpandey/agent-team/vnext/internal/release"
	"github.com/thebpandey/agent-team/vnext/internal/store"
)

const testRevision = "0123456789abcdef0123456789abcdef01234567"

func TestRollbackRevisionDispatchWithJSONAndUniqueCompatibility(t *testing.T) {
	const nextRevision = "abcdef0123456789abcdef0123456789abcdef01"
	for _, test := range []struct {
		name      string
		ambiguous bool
		args      []string
	}{
		{name: "revision with JSON", ambiguous: true, args: []string{"rollback", "--version", "8.0.0", "--revision", testRevision, "--json"}},
		{name: "unique without JSON", args: []string{"rollback", "--version", "8.0.0"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			dataHome := filepath.Join(root, "data")
			t.Setenv("LOCALAPPDATA", dataHome)
			t.Setenv("APPDATA", filepath.Join(root, "roaming"))
			t.Setenv("XDG_DATA_HOME", dataHome)
			t.Setenv("CODEX_HOME", filepath.Join(root, "codex"))
			t.Setenv("CLAUDE_HOME", filepath.Join(root, "claude"))
			layout, err := install.ResolveLayout(runtime.GOOS, map[string]string{"LOCALAPPDATA": os.Getenv("LOCALAPPDATA"), "XDG_DATA_HOME": os.Getenv("XDG_DATA_HOME"), "CODEX_HOME": os.Getenv("CODEX_HOME"), "CLAUDE_HOME": os.Getenv("CLAUDE_HOME")})
			if err != nil {
				t.Fatal(err)
			}
			old := cliReleaseFixture(t, filepath.Join(root, "old"), "8.0.0", testRevision, "old")
			next := cliReleaseFixture(t, filepath.Join(root, "next"), "8.0.0", nextRevision, "next")
			var installed install.CASOutcome
			if test.ambiguous {
				legacy := cliReleaseFixture(t, filepath.Join(root, "legacy"), "8.0.0", "fedcba9876543210fedcba9876543210fedcba98", "legacy")
				old.Contract, old.Entrypoints = legacy.Contract, legacy.Entrypoints
				next.Contract, next.Entrypoints = legacy.Contract, legacy.Entrypoints
				installed, err = install.Install(context.Background(), layout, legacy, []install.Host{install.Codex, install.Claude}, 0)
				if err == nil {
					installed, err = install.Update(context.Background(), layout, old, installed.Manifest.Revision)
				}
			} else {
				installed, err = install.Install(context.Background(), layout, old, []install.Host{install.Codex, install.Claude}, 0)
			}
			if err != nil {
				t.Fatal(err)
			}
			updated, err := install.Update(context.Background(), layout, next, installed.Manifest.Revision)
			if err != nil {
				t.Fatal(err)
			}
			var output bytes.Buffer
			if code := cli.Run(context.Background(), test.args, core.Dependencies{Stdout: &output, Stderr: &output, Management: runManagement}); code != 0 {
				t.Fatalf("code=%d output=%q", code, output.String())
			}
			got, err := install.NewManifestStore(layout).Read(context.Background())
			if err != nil || got.ReleaseRevision != testRevision || got.Revision != updated.Manifest.Revision+1 {
				t.Fatalf("manifest=%+v err=%v output=%q", got, err, output.String())
			}
			for _, file := range got.Files {
				if file.Revision != testRevision {
					t.Fatalf("incoherent rollback file: %+v", file)
				}
			}
		})
	}
}

func cliReleaseFixture(t *testing.T, root, version, revision, marker string) install.Release {
	t.Helper()
	file := func(name string) install.ReleaseFile {
		path := filepath.Join(root, name)
		body := []byte(marker + " " + name)
		writeTestFile(t, path, string(body))
		return install.ReleaseFile{Path: path, SHA256: fmt.Sprintf("%x", sha256.Sum256(body)), Bytes: int64(len(body))}
	}
	return install.Release{Version: version, Revision: revision, Binary: file("agent-teamctl"), Contract: file("WORKER-CONTRACT"), Entrypoints: map[install.Host]install.ReleaseFile{install.Codex: file("codex.md"), install.Claude: file("claude.md")}}
}

type recoveryProof struct {
	dead    bool
	entered chan<- struct{}
	wait    <-chan struct{}
}

func (p recoveryProof) HolderDead(context.Context, store.MutationOwner) (bool, error) {
	if p.entered != nil {
		close(p.entered)
	}
	if p.wait != nil {
		<-p.wait
	}
	return p.dead, nil
}

func TestCleanupMutationLockRecoveryCLI(t *testing.T) {
	for _, test := range []struct {
		name  string
		alter func(store.MutationOwner) store.MutationOwner
		proof recoveryProof
		want  int
	}{
		{name: "proven dead", proof: recoveryProof{dead: true}, want: 0},
		{name: "live", proof: recoveryProof{}, want: 1},
		{name: "wrong owner", alter: func(owner store.MutationOwner) store.MutationOwner {
			owner.Token = "00000000000000000000000000000000"
			return owner
		}, proof: recoveryProof{dead: true}, want: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			guard, err := store.AcquireProjectMutation(context.Background(), root, "test", "cli-recovery")
			if err != nil {
				t.Fatal(err)
			}
			owner := guard.Owner()
			if test.alter != nil {
				owner = test.alter(owner)
			}
			args := recoveryCLIArgs(store.MutationLockPrimary, owner)
			var output bytes.Buffer
			code := cli.Run(context.Background(), args, core.Dependencies{Stdout: &output, Stderr: &output, Management: recoveryManagement(root, test.proof)})
			if code != test.want || (code == 0 && !bytes.Contains(output.Bytes(), []byte(`"recovered":true`))) {
				t.Fatalf("code=%d output=%q", code, output.String())
			}
		})
	}
}

func TestCleanupMutationLockRecoveryCLIRejectsCorruptAndIsSingleWinner(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, ".agent-team", "mutation.lock")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("corrupt"), 0o600); err != nil {
		t.Fatal(err)
	}
	owner := store.MutationOwner{Token: "0123456789abcdef0123456789abcdef", OperationID: "corrupt"}
	var output bytes.Buffer
	if code := cli.Run(context.Background(), recoveryCLIArgs(store.MutationLockPrimary, owner), core.Dependencies{Stdout: &output, Stderr: &output, Management: recoveryManagement(root, recoveryProof{dead: true})}); code != 1 {
		t.Fatalf("corrupt code=%d output=%q", code, output.String())
	}

	root = t.TempDir()
	guard, err := store.AcquireProjectMutation(context.Background(), root, "test", "concurrent-cli")
	if err != nil {
		t.Fatal(err)
	}
	args := recoveryCLIArgs(store.MutationLockPrimary, guard.Owner())
	codes := make(chan int, 2)
	var wait sync.WaitGroup
	wait.Add(2)
	for range 2 {
		go func() {
			defer wait.Done()
			codes <- cli.Run(context.Background(), args, core.Dependencies{Stdout: io.Discard, Stderr: io.Discard, Management: recoveryManagement(root, recoveryProof{dead: true})})
		}()
	}
	wait.Wait()
	close(codes)
	winners := 0
	for code := range codes {
		if code == 0 {
			winners++
		}
	}
	if winners != 1 {
		t.Fatalf("winners=%d", winners)
	}
	next, err := store.AcquireProjectMutation(context.Background(), root, "test", "after-cli-recovery")
	if err != nil {
		t.Fatal(err)
	}
	if err := next.Release(); err != nil {
		t.Fatal(err)
	}
}

func TestCleanupMutationLockRecoveryCLIRecoversClaimResidue(t *testing.T) {
	root := t.TempDir()
	primary, err := store.AcquireProjectMutation(context.Background(), root, "test", "claim-residue")
	if err != nil {
		t.Fatal(err)
	}
	entered, resume := make(chan struct{}), make(chan struct{})
	first := make(chan error, 1)
	go func() {
		_, err := store.RecoverProjectMutation(context.Background(), root, store.MutationRecoveryRequest{Target: store.MutationLockPrimary, Token: primary.Owner().Token, OperationID: primary.Owner().OperationID}, recoveryProof{dead: true, entered: entered, wait: resume})
		first <- err
	}()
	<-entered
	claim, err := store.MutationLockOwner(root, store.MutationLockRecovery)
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	code := cli.Run(context.Background(), recoveryCLIArgs(store.MutationLockRecovery, claim), core.Dependencies{Stdout: &output, Stderr: &output, Management: recoveryManagement(root, recoveryProof{dead: true})})
	if code != 0 {
		t.Fatalf("code=%d output=%q", code, output.String())
	}
	close(resume)
	if err := <-first; err != nil {
		t.Fatal(err)
	}
	next, err := store.AcquireProjectMutation(context.Background(), root, "test", "after-claim-residue")
	if err != nil {
		t.Fatal(err)
	}
	if err := next.Release(); err != nil {
		t.Fatal(err)
	}
}

func recoveryCLIArgs(target string, owner store.MutationOwner) []string {
	return []string{"cleanup", "--mutation-lock", target, "--owner-token", owner.Token, "--operation", owner.OperationID, "--confirm-dead", "--json"}
}

func recoveryManagement(root string, proof store.HolderLiveness) core.ManagementAction {
	return func(ctx context.Context, args []string, stdout, stderr io.Writer) int {
		owner, err := recoverMutationLock(ctx, root, args, proof)
		if err != nil {
			return managementError(args, stdout, stderr, err)
		}
		return managementResult(args, stdout, map[string]any{"ok": true, "recovered": true, "target": args[2], "operation": owner.OperationID, "owner_token": owner.Token})
	}
}

func TestLocalReleaseRequiresPackagedProvenance(t *testing.T) {
	root, executable := packagedDirectory(t)
	rel, err := localReleaseFrom(root, executable, "1.0.0")
	if err != nil || rel.Version != "1.0.0" || rel.Revision != testRevision {
		t.Fatal(rel, err)
	}
	for name, mutate := range map[string]func(string){
		"tampered source": func(root string) { writeTestFile(t, filepath.Join(root, "WORKER-CONTRACT"), "tampered") },
		"missing manifest": func(root string) {
			if err := os.Remove(filepath.Join(root, "RELEASE.json")); err != nil {
				t.Fatal(err)
			}
		},
		"missing checksums": func(root string) {
			if err := os.Remove(filepath.Join(root, "SHA256SUMS")); err != nil {
				t.Fatal(err)
			}
		},
		"extra path": func(root string) { writeTestFile(t, filepath.Join(root, "extra"), "unexpected") },
		"symlink source": func(root string) {
			path := filepath.Join(root, "WORKER-CONTRACT")
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(filepath.Join(root, "VERSION"), path); err != nil {
				t.Fatal(err)
			}
		},
		"wrong revision": func(root string) {
			replaceTestBytes(t, filepath.Join(root, "RELEASE.json"), []byte(testRevision), []byte("not-a-release-revision------------------"))
		},
	} {
		t.Run(name, func(t *testing.T) {
			copyRoot, executable := packagedDirectory(t)
			mutate(copyRoot)
			if _, err := localReleaseFrom(copyRoot, executable, "1.0.0"); err == nil {
				t.Fatal("invalid package accepted")
			}
		})
	}
	if _, err := localReleaseFrom(root, executable, "2.0.0"); err == nil {
		t.Fatal("wrong requested version accepted")
	}
}

func packagedDirectory(t *testing.T) (string, string) {
	t.Helper()
	source := t.TempDir()
	writeTestFile(t, filepath.Join(source, "agent-teamctl"), "binary")
	writeTestFile(t, filepath.Join(source, "WORKER-CONTRACT"), "contract")
	writeTestFile(t, filepath.Join(source, "codex", "SKILL.md"), "codex")
	writeTestFile(t, filepath.Join(source, "claude", "SKILL.md"), "claude")
	writeTestFile(t, filepath.Join(source, "VERSION"), "1.0.0\n")
	output := t.TempDir()
	if err := releasepkg.BuildReleasePackageFrom(source, output, "1.0.0", testRevision); err != nil {
		t.Fatal(err)
	}
	archive, err := zip.OpenReader(filepath.Join(output, "agent-teamctl-1.0.0.zip"))
	if err != nil {
		t.Fatal(err)
	}
	defer archive.Close()
	for _, member := range archive.File {
		reader, err := member.Open()
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(reader)
		_ = reader.Close()
		if err != nil {
			t.Fatal(err)
		}
		writeTestFile(t, filepath.Join(output, filepath.FromSlash(member.Name)), string(body))
	}
	return output, filepath.Join(output, "agent-teamctl")
}

func writeTestFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
}

func replaceTestBytes(t *testing.T, path string, old, replacement []byte) {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for index := 0; index+len(old) <= len(body); index++ {
		match := true
		for offset := range old {
			if body[index+offset] != old[offset] {
				match = false
				break
			}
		}
		if match {
			body = append(append(append([]byte(nil), body[:index]...), replacement...), body[index+len(old):]...)
			if err := os.WriteFile(path, body, 0o644); err != nil {
				t.Fatal(err)
			}
			return
		}
	}
	t.Fatal("test bytes not found")
}
