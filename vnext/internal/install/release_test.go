package install_test

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/thebpandey/agent-team/vnext/internal/core"
	install "github.com/thebpandey/agent-team/vnext/internal/install"
)

func TestVerifyReleaseAndResolveNativeLayouts(t *testing.T) {
	root := t.TempDir()
	release := writeReleaseFixture(t, root)
	if err := install.VerifyRelease(release); err != nil {
		t.Fatal(err)
	}
	release.Binary.SHA256 = "wrong"
	if err := install.VerifyRelease(release); err == nil {
		t.Fatal("hash mismatch accepted")
	}
	linux, err := install.ResolveLayout("linux", map[string]string{"XDG_DATA_HOME": filepath.Join(root, "data"), "CODEX_HOME": filepath.Join(root, "codex"), "CLAUDE_HOME": filepath.Join(root, "claude")})
	if err != nil {
		t.Fatal(err)
	}
	darwin, err := install.ResolveLayout("darwin", map[string]string{"XDG_DATA_HOME": filepath.Join(root, "mac-data"), "CODEX_HOME": filepath.Join(root, "mac-codex"), "CLAUDE_HOME": filepath.Join(root, "mac-claude")})
	if err != nil {
		t.Fatal(err)
	}
	windows, err := install.ResolveLayout("windows", map[string]string{"LOCALAPPDATA": filepath.Join(root, "local")})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(windows.DataRoot, "AgentTeam") || linux.DataRoot == windows.DataRoot || darwin.DataRoot == linux.DataRoot {
		t.Fatal(linux, darwin, windows)
	}
	if err := install.ValidateLayout(linux); err != nil {
		t.Fatal(err)
	}
}

func TestManifestStoreCAS(t *testing.T) {
	layout, err := install.ResolveLayout("linux", map[string]string{"XDG_DATA_HOME": filepath.Join(t.TempDir(), "data"), "CODEX_HOME": filepath.Join(t.TempDir(), "codex"), "CLAUDE_HOME": filepath.Join(t.TempDir(), "claude")})
	if err != nil {
		t.Fatal(err)
	}
	store := install.NewManifestStore(layout)
	manifest := install.InstallManifest{Schema: 1, Version: "1.0.0", Hosts: []install.Host{install.Codex}}
	created, err := store.CompareAndSwap(context.Background(), 0, manifest)
	if err != nil || created.Kind != install.CASCreated || created.Manifest.Revision != 1 {
		t.Fatal(created, err)
	}
	duplicate, err := store.CompareAndSwap(context.Background(), 1, manifest)
	if err != nil || duplicate.Kind != install.CASDuplicate || !duplicate.Idempotent || duplicate.ObservedRevision != 1 {
		t.Fatal(duplicate, err)
	}
	stale, err := store.CompareAndSwap(context.Background(), 0, manifest)
	if !errors.Is(err, core.ErrRevision) || stale.Kind != install.CASStale {
		t.Fatal(stale, err)
	}
}
