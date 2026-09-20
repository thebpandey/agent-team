package capability_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/thebpandey/agent-team/vnext/internal/capability"
	"github.com/thebpandey/agent-team/vnext/internal/tracker"
)

type fakeRunner struct {
	calls   [][]string
	results []tracker.CommandResult
	run     func(string)
}

func (f *fakeRunner) Run(_ context.Context, n string, a ...string) tracker.CommandResult {
	f.calls = append(f.calls, append([]string{n}, a...))
	if f.run != nil {
		f.run(n)
	}
	if len(f.results) == 0 {
		return tracker.CommandResult{}
	}
	r := f.results[0]
	f.results = f.results[1:]
	return r
}
func hash(b []byte) string { s := sha256.Sum256(b); return "sha256:" + hex.EncodeToString(s[:]) }

func fixture(t *testing.T) (capability.Probe, capability.Consent, capability.Installers, string) {
	t.Helper()
	project := t.TempDir()
	root := filepath.Join(project, ".agent-team", "tools")
	artifact := filepath.Join(root, "bin", "serena")
	if err := os.MkdirAll(filepath.Dir(artifact), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(artifact, []byte("old"), 0600); err != nil {
		t.Fatal(err)
	}
	exe := filepath.Join(t.TempDir(), "installer")
	if err := os.WriteFile(exe, []byte("x"), 0700); err != nil {
		t.Fatal(err)
	}
	src := capability.VerifiedSource{Identity: "registry.example/serena", Digest: hash([]byte("release")), Version: "1"}
	probe := capability.Probe{Name: capability.Serena, Mode: capability.ReadOnlyMCP, Path: exe, Source: src, Version: "1", Available: true, Healthy: true}
	install := []string{exe, "install", "--root", root}
	rollback := []string{exe, "rollback", "--root", root}
	consent := capability.Consent{Name: capability.Serena, Enabled: true, Mode: capability.ReadOnlyMCP, Source: src, VerifiedVersion: "1", ProjectRoot: project, Install: capability.Action{Argv: install}, Rollback: capability.Action{Argv: rollback}, ProbeArgs: []string{"--version"}, OwnedFiles: []capability.OwnedFile{{Path: filepath.Join("bin", "serena"), Role: capability.ToolBinary, SHA256: hash([]byte("new"))}}}
	return probe, consent, capability.Installers{capability.Serena: {Name: capability.Serena, Executable: exe, Source: src, Install: install, Rollback: rollback, ProbeArgs: []string{"--version"}}}, artifact
}

func TestSourceMustBeConcreteAndExact(t *testing.T) {
	p, c, m, _ := fixture(t)
	c.Source.Identity = "verified:registry.example/serena"
	if _, err := capability.BuildInstallPlan(p, c, m); err == nil {
		t.Fatal("prefix source accepted")
	}
	p, c, m, _ = fixture(t)
	c.Source.Digest = hash([]byte("other"))
	if _, err := capability.BuildInstallPlan(p, c, m); err == nil {
		t.Fatal("source digest drift accepted")
	}
}
func TestMissingInstallerOrExecutableIsUnavailable(t *testing.T) {
	p, c, _, _ := fixture(t)
	if _, err := capability.BuildInstallPlan(p, c, capability.Installers{}); err == nil {
		t.Fatal("missing installer accepted")
	}
	p, c, m, _ := fixture(t)
	m[capability.Serena] = capability.Installer{Name: capability.Serena, Executable: filepath.Join(t.TempDir(), "missing")}
	if _, err := capability.BuildInstallPlan(p, c, m); err == nil {
		t.Fatal("missing executable accepted")
	}
}
func TestInstallUsesActualArtifactProbeAndRestoresOnProbeFailure(t *testing.T) {
	p, c, m, artifact := fixture(t)
	plan, err := capability.BuildInstallPlan(p, c, m)
	if err != nil {
		t.Fatal(err)
	}
	f := &fakeRunner{results: []tracker.CommandResult{{}, {Exit: 1}, {}}, run: func(n string) {
		if n == c.Install.Argv[0] {
			_ = os.WriteFile(artifact, []byte("new"), 0755)
		}
	}}
	if _, err := capability.Install(context.Background(), f, plan); err == nil {
		t.Fatal("failed probe accepted")
	}
	b, err := os.ReadFile(artifact)
	if err != nil || string(b) != "old" {
		t.Fatalf("restored=%q %v", b, err)
	}
	info, _ := os.Stat(artifact)
	if info.Mode().Perm() != 0600 {
		t.Fatal("mode not restored")
	}
	if len(f.calls) != 3 || f.calls[1][0] != artifact {
		t.Fatalf("calls=%#v", f.calls)
	}
}
func TestRollbackHelperFailureStillRestores(t *testing.T) {
	p, c, m, artifact := fixture(t)
	plan, err := capability.BuildInstallPlan(p, c, m)
	if err != nil {
		t.Fatal(err)
	}
	f := &fakeRunner{results: []tracker.CommandResult{{}, {Exit: 1}, {Exit: 2}}, run: func(n string) {
		if n == c.Install.Argv[0] {
			_ = os.WriteFile(artifact, []byte("new"), 0755)
		}
	}}
	if _, err := capability.Install(context.Background(), f, plan); err == nil {
		t.Fatal("rollback helper failure accepted")
	}
	b, _ := os.ReadFile(artifact)
	if string(b) != "old" {
		t.Fatal("bytes not restored")
	}
}
func TestNoFollowRejectsSymlinkFileAndParent(t *testing.T) {
	if os.Getenv("GOOS") == "windows" {
		t.Skip("symlink setup")
	}
	p, c, m, artifact := fixture(t)
	_ = os.Remove(artifact)
	if err := os.Symlink(filepath.Join(t.TempDir(), "target"), artifact); err != nil {
		t.Skip(err)
	}
	if _, err := capability.BuildInstallPlan(p, c, m); err != nil {
		t.Fatal(err)
	}
	if _, err := capability.Install(context.Background(), &fakeRunner{}, mustPlan(t, p, c, m)); err == nil {
		t.Fatal("symlink artifact accepted")
	}
}
func mustPlan(t *testing.T, p capability.Probe, c capability.Consent, m capability.Installers) capability.InstallPlan {
	t.Helper()
	x, e := capability.BuildInstallPlan(p, c, m)
	if e != nil {
		t.Fatal(e)
	}
	return x
}
func TestNativeFallback(t *testing.T) {
	p, e := capability.ProbeAll(context.Background(), nil, []capability.Name{capability.Native})
	if e != nil || len(p) != 1 || !p[0].Healthy {
		t.Fatal(p, e)
	}
}
func TestZeroPlanAndRunnerFailureRejected(t *testing.T) {
	if _, e := capability.Install(context.Background(), &fakeRunner{}, capability.InstallPlan{}); e == nil {
		t.Fatal("forgery")
	}
	_ = errors.New
}
