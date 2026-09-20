package install_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/thebpandey/agent-team/vnext/internal/core"
	install "github.com/thebpandey/agent-team/vnext/internal/install"
)

func TestInstallUpdateRollbackUninstallPreserveChangedFiles(t *testing.T) {
	root := t.TempDir()
	release := writeReleaseFixture(t, root)
	layout, err := install.ResolveLayout("linux", map[string]string{"XDG_DATA_HOME": filepath.Join(root, "data"), "CODEX_HOME": filepath.Join(root, "codex"), "CLAUDE_HOME": filepath.Join(root, "claude")})
	if err != nil {
		t.Fatal(err)
	}
	unrelated := filepath.Join(root, "unrelated.txt")
	if err := os.WriteFile(unrelated, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	first, err := install.Install(context.Background(), layout, release, []install.Host{install.Codex, install.Claude}, 0)
	if err != nil {
		t.Fatal(err)
	}
	oldBinary, _ := os.ReadFile(release.Binary.Path)
	oldContract, _ := os.ReadFile(release.Contract.Path)
	oldEntrypoint, _ := os.ReadFile(release.Entrypoints[install.Codex].Path)
	changed := filepath.Join(layout.SkillRoots[install.Codex], "agent-team-vnext", "SKILL.md")
	if err := os.WriteFile(changed, []byte("user change"), 0o600); err != nil {
		t.Fatal(err)
	}
	release2Root := filepath.Join(root, "release-1.1.0")
	if err := os.MkdirAll(release2Root, 0o700); err != nil {
		t.Fatal(err)
	}
	release2 := writeReleaseFixture(t, release2Root)
	release2.Version = "1.1.0"
	rewriteReleaseFile(t, &release2.Binary, "vnext-binary-v1.1.0")
	rewriteReleaseFile(t, &release2.Contract, "contract-v1.1.0")
	for host, file := range release2.Entrypoints {
		rewriteReleaseFile(t, &file, "entrypoint-v1.1.0")
		release2.Entrypoints[host] = file
	}
	next, err := install.Update(context.Background(), layout, release2, first.Manifest.Revision)
	if err != nil || len(next.Retained) != 1 || len(next.Manifest.Backups) == 0 {
		t.Fatal(next.Retained, next.Manifest.Backups, err)
	}
	backupChecked := false
	for _, backup := range next.Manifest.Backups {
		if backup.Role == install.BinaryRole {
			got, readErr := os.ReadFile(backup.Path)
			if readErr != nil || !bytes.Equal(got, oldBinary) {
				t.Fatalf("binary backup=%q err=%v", got, readErr)
			}
			backupChecked = true
		}
	}
	if !backupChecked {
		t.Fatal("binary backup missing")
	}
	rolledBack, err := install.Rollback(context.Background(), layout, release.Version, next.Manifest.Revision)
	if err != nil || rolledBack.Manifest.Revision != next.Manifest.Revision+1 {
		t.Fatal(rolledBack, err)
	}
	if got, _ := os.ReadFile(layout.BinaryPath); !bytes.Equal(got, oldBinary) {
		t.Fatalf("binary rollback=%q", got)
	}
	if got, _ := os.ReadFile(layout.ContractPath); !bytes.Equal(got, oldContract) {
		t.Fatalf("contract rollback=%q", got)
	}
	claudePath := filepath.Join(layout.SkillRoots[install.Claude], "agent-team-vnext", "SKILL.md")
	if got, _ := os.ReadFile(claudePath); !bytes.Equal(got, oldEntrypoint) {
		t.Fatalf("entrypoint rollback=%q", got)
	}
	retained, _, err := install.Uninstall(context.Background(), layout, rolledBack.Manifest.Revision)
	if err != nil || len(retained) == 0 {
		t.Fatal(retained, err)
	}
	if got, _ := os.ReadFile(changed); string(got) != "user change" {
		t.Fatalf("changed=%q", got)
	}
	for _, path := range []string{layout.BinaryPath, layout.ContractPath, claudePath} {
		if _, statErr := os.Stat(path); !errors.Is(statErr, os.ErrNotExist) {
			t.Fatalf("owned file remains path=%s err=%v", path, statErr)
		}
	}
	if got, _ := os.ReadFile(unrelated); string(got) != "keep" {
		t.Fatalf("unrelated=%q", got)
	}
}

func rewriteReleaseFile(t *testing.T, file *install.ReleaseFile, contents string) {
	t.Helper()
	if err := os.WriteFile(file.Path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(file.Path)
	file.SHA256 = fmt.Sprintf("%x", sha256.Sum256(data))
	file.Bytes = int64(len(data))
}

func TestManifestCASRejectsStaleWriter(t *testing.T) {
	root := t.TempDir()
	release := writeReleaseFixture(t, root)
	layout, _ := install.ResolveLayout("linux", map[string]string{"XDG_DATA_HOME": filepath.Join(root, "data"), "CODEX_HOME": filepath.Join(root, "codex"), "CLAUDE_HOME": filepath.Join(root, "claude")})
	first, err := install.Install(context.Background(), layout, release, []install.Host{install.Codex}, 0)
	if err != nil {
		t.Fatal(err)
	}
	_, err = install.Update(context.Background(), layout, release, 0)
	if !errors.Is(err, core.ErrRevision) || first.Manifest.Revision != 1 {
		t.Fatal(first, err)
	}
}

func TestPOSIXAtomic(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX atomic mode")
	}
	root := t.TempDir()
	layout, err := install.ResolveLayout("linux", map[string]string{"XDG_DATA_HOME": root, "CODEX_HOME": filepath.Join(root, "codex"), "CLAUDE_HOME": filepath.Join(root, "claude")})
	if err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(layout.DataRoot, "owned.bin")
	if err := install.AtomicReplace(layout, target, []byte("atomic"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(target); string(got) != "atomic" {
		t.Fatal(string(got))
	}
	escape := filepath.Join(filepath.Dir(root), "outside-owned.bin")
	if err := install.AtomicReplace(layout, escape, []byte("escape"), 0o600); !errors.Is(err, core.ErrPath) {
		t.Fatal("root escape accepted", err)
	}
}

func TestWindowsSharing(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows sharing-lock branch")
	}
	root := t.TempDir()
	layout, err := install.ResolveLayout("windows", map[string]string{"LOCALAPPDATA": root, "CODEX_HOME": filepath.Join(root, "codex"), "CLAUDE_HOME": filepath.Join(root, "claude")})
	if err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(layout.DataRoot, "locked.bin")
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	lock, err := os.Open(target)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	replaceErr := install.AtomicReplace(layout, target, []byte("new"), 0o600)
	got, readErr := os.ReadFile(target)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if replaceErr != nil && !bytes.Equal(got, []byte("old")) {
		t.Fatalf("failed replacement changed target=%q", got)
	}
	if replaceErr == nil && !bytes.Equal(got, []byte("new")) {
		t.Fatalf("successful replacement=%q", got)
	}
}

func TestOwnership(t *testing.T) {
	root := t.TempDir()
	release := writeReleaseFixture(t, root)
	layout, _ := install.ResolveLayout("linux", map[string]string{"XDG_DATA_HOME": filepath.Join(root, "data"), "CODEX_HOME": filepath.Join(root, "codex"), "CLAUDE_HOME": filepath.Join(root, "claude")})
	manifest, err := install.Install(context.Background(), layout, release, []install.Host{install.Codex}, 0)
	if err != nil {
		t.Fatal(err)
	}
	changed := filepath.Join(layout.SkillRoots[install.Codex], "agent-team-vnext", "SKILL.md")
	if err := os.WriteFile(changed, []byte("user-owned"), 0o600); err != nil {
		t.Fatal(err)
	}
	retained, _, err := install.Uninstall(context.Background(), layout, manifest.Manifest.Revision)
	if err != nil || len(retained) != 1 || retained[0] != changed {
		t.Fatal(retained, err)
	}
	if _, err := os.Stat(changed); err != nil {
		t.Fatal("changed file was deleted", err)
	}
}
