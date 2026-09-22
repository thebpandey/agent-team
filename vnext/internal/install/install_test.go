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
	"strings"
	"testing"

	"github.com/thebpandey/agent-team/vnext/internal/core"
	install "github.com/thebpandey/agent-team/vnext/internal/install"
)

func TestMain(m *testing.M) {
	home, err := os.MkdirTemp("", "agent-team-install-test-home-")
	if err != nil {
		panic(err)
	}
	previousHome, hadHome := os.LookupEnv("HOME")
	previousUserProfile, hadUserProfile := os.LookupEnv("USERPROFILE")
	if err := os.Setenv("HOME", home); err != nil {
		panic(err)
	}
	if err := os.Setenv("USERPROFILE", home); err != nil {
		panic(err)
	}
	code := m.Run()
	if hadHome {
		_ = os.Setenv("HOME", previousHome)
	} else {
		_ = os.Unsetenv("HOME")
	}
	if hadUserProfile {
		_ = os.Setenv("USERPROFILE", previousUserProfile)
	} else {
		_ = os.Unsetenv("USERPROFILE")
	}
	_ = os.RemoveAll(home)
	os.Exit(code)
}

func requireLayoutHomesWithin(t *testing.T, root string, layout install.Layout) {
	t.Helper()
	for host, skillRoot := range layout.SkillRoots {
		home := filepath.Dir(filepath.Dir(skillRoot))
		relative, err := filepath.Rel(root, home)
		if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			t.Fatalf("%s host home escapes test root: %q", host, home)
		}
	}
}

func TestInstallRejectsUnownedCodexDiscoverySkills(t *testing.T) {
	for _, scenario := range []struct {
		name       string
		customHome bool
		path       func(root string, layout install.Layout) string
	}{
		{name: "legacy codex root", path: func(root string, _ install.Layout) string {
			return filepath.Join(root, ".codex", "skills", "agent-team", "SKILL.md")
		}},
		{name: "selected agents root", path: func(_ string, layout install.Layout) string {
			return filepath.Join(layout.SkillRoots[install.Codex], "SKILL.md")
		}},
		{name: "default root despite custom home", customHome: true, path: func(root string, _ install.Layout) string {
			return filepath.Join(root, ".agents", "skills", "agent-team", "SKILL.md")
		}},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			root := t.TempDir()
			env := map[string]string{"HOME": root, "XDG_DATA_HOME": filepath.Join(root, "data")}
			if scenario.customHome {
				env["CODEX_HOME"] = filepath.Join(root, "custom-agents")
			}
			layout, err := install.ResolveLayout("linux", env)
			if err != nil {
				t.Fatal(err)
			}
			requireLayoutHomesWithin(t, root, layout)
			stale := scenario.path(root, layout)
			if err := os.MkdirAll(filepath.Dir(stale), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(stale, []byte("stale agent-team"), 0o600); err != nil {
				t.Fatal(err)
			}
			releaseRoot := filepath.Join(root, "release")
			if err := os.MkdirAll(releaseRoot, 0o755); err != nil {
				t.Fatal(err)
			}
			outcome, err := install.Install(context.Background(), layout, writeReleaseFixture(t, releaseRoot), []install.Host{install.Codex}, 0)
			if !errors.Is(err, core.ErrRevision) || len(outcome.Retained) != 1 || outcome.Retained[0] != stale || !strings.Contains(err.Error(), "recoverable backup") {
				t.Fatalf("outcome=%+v err=%v", outcome, err)
			}
			if _, statErr := os.Lstat(layout.BinaryPath); !errors.Is(statErr, os.ErrNotExist) {
				t.Fatalf("install mutated target: %v", statErr)
			}
		})
	}
}

func TestUpdateRejectsStaleCodexDiscoverySkillWithoutMutation(t *testing.T) {
	root := t.TempDir()
	layout, err := install.ResolveLayout("linux", map[string]string{"HOME": root, "XDG_DATA_HOME": filepath.Join(root, "data")})
	if err != nil {
		t.Fatal(err)
	}
	releaseRoot := filepath.Join(root, "release")
	if err := os.MkdirAll(releaseRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	first, err := install.Install(context.Background(), layout, writeReleaseFixture(t, releaseRoot), []install.Host{install.Codex}, 0)
	if err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(layout.BinaryPath)
	if err != nil {
		t.Fatal(err)
	}
	stale := filepath.Join(root, ".codex", "skills", "agent-team", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(stale), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stale, []byte("legacy"), 0o600); err != nil {
		t.Fatal(err)
	}
	next := writeReleaseFixture(t, releaseRoot)
	next.Version, next.Revision = "1.0.1", "abcdef0123456789abcdef0123456789abcdef01"
	outcome, err := install.Update(context.Background(), layout, next, first.Manifest.Revision)
	if !errors.Is(err, core.ErrRevision) || len(outcome.Retained) != 1 || outcome.Retained[0] != stale {
		t.Fatalf("outcome=%+v err=%v", outcome, err)
	}
	after, err := os.ReadFile(layout.BinaryPath)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("binary changed=%q err=%v", after, err)
	}
}

func TestInstallRejectsSymlinkedCodexDiscoverySkill(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires privileges on Windows")
	}
	root := t.TempDir()
	layout, err := install.ResolveLayout("linux", map[string]string{"HOME": root, "XDG_DATA_HOME": filepath.Join(root, "data")})
	if err != nil {
		t.Fatal(err)
	}
	stale := filepath.Join(root, ".codex", "skills", "agent-team", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(stale), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "outside"), stale); err != nil {
		t.Fatal(err)
	}
	releaseRoot := filepath.Join(root, "release")
	if err := os.MkdirAll(releaseRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	outcome, err := install.Install(context.Background(), layout, writeReleaseFixture(t, releaseRoot), []install.Host{install.Codex}, 0)
	if !errors.Is(err, core.ErrRevision) || len(outcome.Retained) != 1 || outcome.Retained[0] != stale {
		t.Fatalf("outcome=%+v err=%v", outcome, err)
	}
}

func TestInstallRejectsFallbackAndNestedCodexDiscoverySkills(t *testing.T) {
	for _, scenario := range []struct {
		name       string
		fallback   bool
		customHome bool
		path       func(root string, layout install.Layout) string
	}{
		{name: "effective fallback home", fallback: true, path: func(root string, _ install.Layout) string {
			return filepath.Join(root, ".codex", "skills", "agent-team", "SKILL.md")
		}},
		{name: "default nested root with custom configured home", customHome: true, path: func(root string, _ install.Layout) string {
			return filepath.Join(root, ".agents", "skills", "agent-team", "agent-team-vnext", "SKILL.md")
		}},
		{name: "selected nested root", path: func(_ string, layout install.Layout) string {
			return filepath.Join(layout.SkillRoots[install.Codex], "agent-team-vnext", "SKILL.md")
		}},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			root := t.TempDir()
			if scenario.fallback {
				t.Setenv("HOME", root)
				t.Setenv("USERPROFILE", root)
			}
			env := map[string]string{"XDG_DATA_HOME": filepath.Join(root, "data")}
			if !scenario.fallback {
				env["HOME"] = root
			}
			if scenario.customHome {
				env["CODEX_HOME"] = filepath.Join(root, "custom-agents")
			}
			layout, err := install.ResolveLayout("linux", env)
			if err != nil {
				t.Fatal(err)
			}
			requireLayoutHomesWithin(t, root, layout)
			stale := scenario.path(root, layout)
			if err := os.MkdirAll(filepath.Dir(stale), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(stale, []byte("stale nested agent-team"), 0o600); err != nil {
				t.Fatal(err)
			}
			releaseRoot := filepath.Join(root, "release")
			if err := os.MkdirAll(releaseRoot, 0o755); err != nil {
				t.Fatal(err)
			}
			outcome, err := install.Install(context.Background(), layout, writeReleaseFixture(t, releaseRoot), []install.Host{install.Codex}, 0)
			if !errors.Is(err, core.ErrRevision) || len(outcome.Retained) != 1 || outcome.Retained[0] != stale {
				t.Fatalf("outcome=%+v err=%v", outcome, err)
			}
			if _, statErr := os.Lstat(layout.BinaryPath); !errors.Is(statErr, os.ErrNotExist) {
				t.Fatalf("install mutated target: %v", statErr)
			}
		})
	}
}

func TestUpdateAllowsManifestOwnedCodexNestedSkill(t *testing.T) {
	root := t.TempDir()
	layout, err := install.ResolveLayout("linux", map[string]string{"HOME": root, "XDG_DATA_HOME": filepath.Join(root, "data")})
	if err != nil {
		t.Fatal(err)
	}
	releaseRoot := filepath.Join(root, "release")
	if err := os.MkdirAll(releaseRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	first, err := install.Install(context.Background(), layout, writeReleaseFixture(t, releaseRoot), []install.Host{install.Codex}, 0)
	if err != nil {
		t.Fatal(err)
	}
	next := writeReleaseFixture(t, releaseRoot)
	next.Version, next.Revision = "1.0.1", "abcdef0123456789abcdef0123456789abcdef01"
	if _, err := install.Update(context.Background(), layout, next, first.Manifest.Revision); err != nil {
		t.Fatal(err)
	}
}

func TestFreshInstallUsesPerUserHostDefaults(t *testing.T) {
	root := t.TempDir()
	layout, err := install.ResolveLayout("linux", map[string]string{"HOME": root, "XDG_DATA_HOME": filepath.Join(root, "data")})
	if err != nil {
		t.Fatal(err)
	}
	releaseRoot := filepath.Join(root, "release")
	if err := os.MkdirAll(releaseRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	outcome, err := install.Install(context.Background(), layout, writeReleaseFixture(t, releaseRoot), []install.Host{install.Codex, install.Claude}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if outcome.Manifest.HostHomes[install.Codex] != filepath.Join(root, ".agents") || outcome.Manifest.HostHomes[install.Claude] != filepath.Join(root, ".claude") {
		t.Fatalf("manifest host homes = %#v", outcome.Manifest.HostHomes)
	}
	for host, home := range outcome.Manifest.HostHomes {
		if _, err := os.Lstat(filepath.Join(home, "skills", "agent-team", "SKILL.md")); err != nil {
			t.Fatalf("%s default entrypoint: %v", host, err)
		}
	}
}

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
	if first.Manifest.Version != release.Version || first.Manifest.ReleaseRevision != release.Revision {
		t.Fatalf("manifest provenance = %q %q", first.Manifest.Version, first.Manifest.ReleaseRevision)
	}
	oldBinary, _ := os.ReadFile(release.Binary.Path)
	oldContract, _ := os.ReadFile(release.Contract.Path)
	oldEntrypoint, _ := os.ReadFile(release.Entrypoints[install.Codex].Path)
	changed := filepath.Join(layout.SkillRoots[install.Codex], "SKILL.md")
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
	claudePath := filepath.Join(layout.SkillRoots[install.Claude], "SKILL.md")
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

func TestLegacyManifestCustomHomesMigrateOnEnvFreeLifecycle(t *testing.T) {
	root := t.TempDir()
	dataHome := filepath.Join(root, "data")
	customCodex, customClaude := filepath.Join(root, "custom-codex"), filepath.Join(root, "custom-claude")
	installLayout, err := install.ResolveLayout("linux", map[string]string{
		"HOME": root, "XDG_DATA_HOME": dataHome, "CODEX_HOME": customCodex, "CLAUDE_HOME": customClaude,
	})
	if err != nil {
		t.Fatal(err)
	}
	firstReleaseRoot := filepath.Join(root, "release-1")
	if err := os.MkdirAll(firstReleaseRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	firstRelease := writeReleaseFixture(t, firstReleaseRoot)
	first, err := install.Install(context.Background(), installLayout, firstRelease, []install.Host{install.Codex, install.Claude}, 0)
	if err != nil {
		t.Fatal(err)
	}
	legacy := first.Manifest
	legacy.HostHomes = nil
	legacyWrite, err := install.NewManifestStore(installLayout).CompareAndSwap(context.Background(), first.Manifest.Revision, legacy)
	if err != nil {
		t.Fatal(err)
	}
	envFree := map[string]string{"HOME": filepath.Join(root, "default-home"), "XDG_DATA_HOME": dataHome}
	lifecycleLayout, err := install.ResolveInstalledLayout("linux", envFree, legacyWrite.Manifest)
	if err != nil {
		t.Fatal(err)
	}
	if lifecycleLayout.SkillRoots[install.Codex] != filepath.Join(customCodex, "skills", "agent-team") || lifecycleLayout.SkillRoots[install.Claude] != filepath.Join(customClaude, "skills", "agent-team") {
		t.Fatalf("inferred custom roots = %#v", lifecycleLayout.SkillRoots)
	}

	nextReleaseRoot := filepath.Join(root, "release-2")
	if err := os.MkdirAll(nextReleaseRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	nextRelease := writeReleaseFixture(t, nextReleaseRoot)
	nextRelease.Version = "1.1.0"
	nextRelease.Revision = "abcdef0123456789abcdef0123456789abcdef01"
	rewriteReleaseFile(t, &nextRelease.Binary, "binary 1.1.0")
	updated, err := install.Update(context.Background(), lifecycleLayout, nextRelease, legacyWrite.Manifest.Revision)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Manifest.HostHomes[install.Codex] != customCodex || updated.Manifest.HostHomes[install.Claude] != customClaude {
		t.Fatalf("migrated host homes = %#v", updated.Manifest.HostHomes)
	}
	lifecycleLayout, err = install.ResolveInstalledLayout("linux", envFree, updated.Manifest)
	if err != nil {
		t.Fatal(err)
	}
	rolledBack, err := install.RollbackRelease(context.Background(), lifecycleLayout, firstRelease.Version, firstRelease.Revision, updated.Manifest.Revision)
	if err != nil {
		t.Fatal(err)
	}
	lifecycleLayout, err = install.ResolveInstalledLayout("linux", envFree, rolledBack.Manifest)
	if err != nil {
		t.Fatal(err)
	}
	if retained, _, err := install.Uninstall(context.Background(), lifecycleLayout, rolledBack.Manifest.Revision); err != nil || len(retained) != 0 {
		t.Fatalf("env-free uninstall retained=%v err=%v", retained, err)
	}
	for _, path := range []string{filepath.Join(customCodex, "skills", "agent-team", "agent-team-vnext", "SKILL.md"), filepath.Join(customClaude, "skills", "agent-team", "agent-team-vnext", "SKILL.md")} {
		if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("managed custom entrypoint remains %s: %v", path, err)
		}
	}
}

func TestUpdateRejectsLayoutThatConflictsWithPersistedHostHomes(t *testing.T) {
	root := t.TempDir()
	dataHome := filepath.Join(root, "data")
	customCodex, customClaude := filepath.Join(root, "custom-codex"), filepath.Join(root, "custom-claude")
	custom, err := install.ResolveLayout("linux", map[string]string{"HOME": root, "XDG_DATA_HOME": dataHome, "CODEX_HOME": customCodex, "CLAUDE_HOME": customClaude})
	if err != nil {
		t.Fatal(err)
	}
	oldRoot, nextRoot := filepath.Join(root, "old"), filepath.Join(root, "next")
	if err := os.MkdirAll(oldRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(nextRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	old := writeReleaseFixture(t, oldRoot)
	installed, err := install.Install(context.Background(), custom, old, []install.Host{install.Codex, install.Claude}, 0)
	if err != nil {
		t.Fatal(err)
	}
	next := writeReleaseFixture(t, nextRoot)
	next.Version = "1.1.0"
	next.Revision = "abcdef0123456789abcdef0123456789abcdef01"
	wrong, err := install.ResolveLayout("linux", map[string]string{"HOME": filepath.Join(root, "default"), "XDG_DATA_HOME": dataHome})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := install.Update(context.Background(), wrong, next, installed.Manifest.Revision); !errors.Is(err, core.ErrRevision) {
		t.Fatalf("conflicting lifecycle layout = %v", err)
	}
	unchanged, err := install.NewManifestStore(custom).Read(context.Background())
	if err != nil || unchanged.Revision != installed.Manifest.Revision || unchanged.Version != old.Version {
		t.Fatalf("manifest changed = %+v, %v", unchanged, err)
	}
	for _, path := range wrong.SkillRoots {
		if _, err := os.Lstat(filepath.Join(path, "agent-team-vnext", "SKILL.md")); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("relocated entrypoint exists under %s: %v", path, err)
		}
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
	changed := filepath.Join(layout.SkillRoots[install.Codex], "SKILL.md")
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
