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

func TestResolveLayoutUsesPerUserHostDefaultsAndIndependentOverrides(t *testing.T) {
	root := t.TempDir()
	for _, test := range []struct {
		name   string
		goos   string
		env    map[string]string
		codex  string
		claude string
	}{
		{name: "linux", goos: "linux", env: map[string]string{"HOME": root, "XDG_DATA_HOME": filepath.Join(root, "data")}, codex: filepath.Join(root, ".agents"), claude: filepath.Join(root, ".claude")},
		{name: "darwin", goos: "darwin", env: map[string]string{"HOME": root, "XDG_DATA_HOME": filepath.Join(root, "data")}, codex: filepath.Join(root, ".agents"), claude: filepath.Join(root, ".claude")},
		{name: "windows", goos: "windows", env: map[string]string{"USERPROFILE": root, "LOCALAPPDATA": filepath.Join(root, "local")}, codex: filepath.Join(root, ".agents"), claude: filepath.Join(root, ".claude")},
		{name: "codex override only", goos: "linux", env: map[string]string{"HOME": root, "XDG_DATA_HOME": filepath.Join(root, "data"), "CODEX_HOME": filepath.Join(root, "custom-codex")}, codex: filepath.Join(root, "custom-codex"), claude: filepath.Join(root, ".claude")},
		{name: "claude override only", goos: "linux", env: map[string]string{"HOME": root, "XDG_DATA_HOME": filepath.Join(root, "data"), "CLAUDE_HOME": filepath.Join(root, "custom-claude")}, codex: filepath.Join(root, ".agents"), claude: filepath.Join(root, "custom-claude")},
	} {
		t.Run(test.name, func(t *testing.T) {
			layout, err := install.ResolveLayout(test.goos, test.env)
			if err != nil {
				t.Fatal(err)
			}
			if got := layout.SkillRoots[install.Codex]; got != filepath.Join(test.codex, "skills", "agent-team") {
				t.Fatalf("codex skill root = %q", got)
			}
			if got := layout.SkillRoots[install.Claude]; got != filepath.Join(test.claude, "skills", "agent-team") {
				t.Fatalf("claude skill root = %q", got)
			}
			if layout.ConfigPaths[install.Codex] != filepath.Join(test.codex, "hooks.json") || layout.ConfigPaths[install.Claude] != filepath.Join(test.claude, "settings.json") {
				t.Fatalf("config paths = %#v", layout.ConfigPaths)
			}
		})
	}
}

func TestResolveInstalledLayoutUsesManifestRootsAndRejectsConflict(t *testing.T) {
	root := t.TempDir()
	customCodex := filepath.Join(root, "custom-codex")
	customClaude := filepath.Join(root, "custom-claude")
	manifest := install.InstallManifest{
		Schema: 1,
		Hosts:  []install.Host{install.Codex, install.Claude},
		HostHomes: map[install.Host]string{
			install.Codex:  customCodex,
			install.Claude: customClaude,
		},
		Files: []install.OwnedFile{
			{Role: install.EntrypointRole, Host: install.Codex, Path: filepath.Join(customCodex, "skills", "agent-team", "agent-team-vnext", "SKILL.md")},
			{Role: install.EntrypointRole, Host: install.Claude, Path: filepath.Join(customClaude, "skills", "agent-team", "agent-team-vnext", "SKILL.md")},
		},
	}
	env := map[string]string{"HOME": filepath.Join(root, "default"), "XDG_DATA_HOME": filepath.Join(root, "data")}
	layout, err := install.ResolveInstalledLayout("linux", env, manifest)
	if err != nil {
		t.Fatal(err)
	}
	if layout.SkillRoots[install.Codex] != filepath.Join(customCodex, "skills", "agent-team") || layout.ConfigPaths[install.Claude] != filepath.Join(customClaude, "settings.json") {
		t.Fatalf("persisted layout = %#v", layout)
	}
	env["CODEX_HOME"] = filepath.Join(root, "other")
	if _, err := install.ResolveInstalledLayout("linux", env, manifest); !errors.Is(err, core.ErrRevision) {
		t.Fatalf("conflicting override = %v", err)
	}
}

func TestResolveInstalledLayoutInfersOnlyExactLegacyEntrypoints(t *testing.T) {
	root := t.TempDir()
	env := map[string]string{"HOME": filepath.Join(root, "default"), "XDG_DATA_HOME": filepath.Join(root, "data")}
	codexHome, claudeHome := filepath.Join(root, "codex"), filepath.Join(root, "claude")
	manifest := install.InstallManifest{Schema: 1, Hosts: []install.Host{install.Codex, install.Claude}, Files: []install.OwnedFile{
		{Role: install.EntrypointRole, Host: install.Codex, Path: filepath.Join(codexHome, "skills", "agent-team", "agent-team-vnext", "SKILL.md")},
		{Role: install.EntrypointRole, Host: install.Claude, Path: filepath.Join(claudeHome, "skills", "agent-team", "SKILL.md")},
	}}
	layout, err := install.ResolveInstalledLayout("linux", env, manifest)
	if err != nil || layout.SkillRoots[install.Codex] != filepath.Join(codexHome, "skills", "agent-team") || layout.SkillRoots[install.Claude] != filepath.Join(claudeHome, "skills", "agent-team") {
		t.Fatalf("legacy layout = %#v, %v", layout, err)
	}

	malformed := manifest
	malformed.Files = append([]install.OwnedFile(nil), manifest.Files...)
	malformed.Files[0].Path = filepath.Join(root, "codex", "SKILL.md")
	if _, err := install.ResolveInstalledLayout("linux", env, malformed); !errors.Is(err, core.ErrRevision) {
		t.Fatalf("malformed legacy path = %v", err)
	}
	ambiguous := manifest
	ambiguous.Files = append(append([]install.OwnedFile(nil), manifest.Files...), manifest.Files[0])
	if _, err := install.ResolveInstalledLayout("linux", env, ambiguous); !errors.Is(err, core.ErrRevision) {
		t.Fatalf("duplicate legacy entrypoint = %v", err)
	}
	crossHost := manifest
	crossHost.Files = append([]install.OwnedFile(nil), manifest.Files...)
	crossHost.Files[1].Path = filepath.Join(codexHome, "skills", "agent-team", "SKILL.md")
	if _, err := install.ResolveInstalledLayout("linux", env, crossHost); !errors.Is(err, core.ErrRevision) {
		t.Fatalf("cross-host legacy roots = %v", err)
	}
	crossHost.HostHomes = map[install.Host]string{install.Codex: codexHome, install.Claude: claudeHome}
	if _, err := install.ResolveInstalledLayout("linux", env, crossHost); !errors.Is(err, core.ErrRevision) {
		t.Fatalf("persisted roots accepted mismatched entrypoint = %v", err)
	}
}

func TestManifestStoreCAS(t *testing.T) {
	layout, err := install.ResolveLayout("linux", map[string]string{"XDG_DATA_HOME": filepath.Join(t.TempDir(), "data"), "CODEX_HOME": filepath.Join(t.TempDir(), "codex"), "CLAUDE_HOME": filepath.Join(t.TempDir(), "claude")})
	if err != nil {
		t.Fatal(err)
	}
	store := install.NewManifestStore(layout)
	manifest := install.InstallManifest{Schema: 1, Version: "1.0.0", ReleaseRevision: "0123456789abcdef0123456789abcdef01234567", Hosts: []install.Host{install.Codex}}
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
