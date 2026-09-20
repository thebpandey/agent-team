package capability_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/thebpandey/agent-team/vnext/internal/capability"
	"github.com/thebpandey/agent-team/vnext/internal/tracker"
)

type nativeFake struct {
	calls  [][]string
	result []tracker.CommandResult
	onRun  func(string, []string)
}

func (f *nativeFake) Run(_ context.Context, name string, args ...string) tracker.CommandResult {
	f.calls = append(f.calls, append([]string{name}, args...))
	if f.onRun != nil {
		f.onRun(name, args)
	}
	if len(f.result) == 0 {
		return tracker.CommandResult{}
	}
	r := f.result[0]
	f.result = f.result[1:]
	return r
}

func digest(data []byte) string {
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func TestNativeIsAlwaysFallback(t *testing.T) {
	got, err := capability.ProbeAll(context.Background(), &nativeFake{}, []capability.Name{capability.Native})
	if err != nil || len(got) != 1 || !got[0].Available || !got[0].Healthy {
		t.Fatalf("probe = %#v, err = %v", got, err)
	}
}

func TestExecutableProbeResolvesPathAndSkillProbeHashesContent(t *testing.T) {
	bin := t.TempDir()
	exe := filepath.Join(bin, "serena")
	if err := os.WriteFile(exe, []byte("ignored"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	runner := &nativeFake{result: []tracker.CommandResult{{Stdout: []byte("1.2.3\n")}}}
	got, err := capability.ProbeAll(context.Background(), runner, []capability.Name{capability.Serena})
	if err != nil || len(got) != 1 || got[0].Path != exe || got[0].Version != "1.2.3" || !reflect.DeepEqual(runner.calls, [][]string{{exe, "--version"}}) {
		t.Fatalf("probe = %#v calls=%#v err=%v", got, runner.calls, err)
	}

	skills := t.TempDir()
	skill := filepath.Join(skills, "impeccable", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(skill), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(skill, []byte("skill content"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AGENT_TEAM_SKILL_ROOT", skills)
	got, err = capability.ProbeAll(context.Background(), runner, []capability.Name{capability.Impeccable})
	if err != nil || len(got) != 1 || got[0].Path != skill || got[0].Digest == "" || !got[0].Available || len(runner.calls) != 1 {
		t.Fatalf("skill probe = %#v calls=%#v err=%v", got, runner.calls, err)
	}
}

func validConsent(t *testing.T) (capability.Probe, capability.Consent, string, string) {
	t.Helper()
	project := t.TempDir()
	rel := filepath.Join("bin", "serena")
	root := filepath.Join(project, ".agent-team", "tools")
	path := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	probe := capability.Probe{Name: capability.Serena, Mode: capability.ReadOnlyMCP, Path: "serena", Version: "1", Source: "verified:local", Available: true, Healthy: true}
	consent := capability.Consent{
		Name: capability.Serena, Enabled: true, Mode: capability.ReadOnlyMCP, Source: "verified:local", VerifiedVersion: "1", InstallerPackage: "serena@1", ProjectRoot: project,
		Install:    capability.Action{Argv: []string{"agent-team-capability-install", "--root", root, "--package", "serena@1", "--version", "1"}},
		Probe:      capability.Action{Argv: []string{"agent-team-capability-probe", "--root", root, "--package", "serena@1", "--version", "1"}},
		Rollback:   capability.Action{Argv: []string{"agent-team-capability-rollback", "--root", root, "--package", "serena@1", "--version", "1"}},
		OwnedFiles: []capability.OwnedFile{{Path: rel, Role: capability.ToolBinary, SHA256: digest([]byte("new"))}},
	}
	return probe, consent, path, root
}

func TestPlanIsImmutableAndBoundToConsent(t *testing.T) {
	probe, consent, path, _ := validConsent(t)
	plan, err := capability.BuildInstallPlan(probe, consent)
	if err != nil {
		t.Fatal(err)
	}
	consent.Install.Argv[0] = "curl"
	consent.OwnedFiles[0].Path = "../escape"
	consent.VerifiedVersion = "2"
	runner := &nativeFake{result: []tracker.CommandResult{{}, {Stdout: []byte("1")}}, onRun: func(name string, _ []string) {
		if name == "agent-team-capability-install" {
			_ = os.WriteFile(path, []byte("new"), 0o600)
		}
	}}
	if _, err := capability.Install(context.Background(), runner, plan); err != nil {
		t.Fatal(err)
	}
	if got := runner.calls[0][0]; got != "agent-team-capability-install" {
		t.Fatalf("post-consent mutation ran %q", got)
	}
}

func TestOpaquePlanRejectsPublicZeroValueForgery(t *testing.T) {
	if _, err := capability.Install(context.Background(), &nativeFake{}, capability.InstallPlan{}); err == nil {
		t.Fatal("forged zero-value plan accepted")
	}
}

func TestPlanRejectsSourceVersionAndActionDrift(t *testing.T) {
	probe, _, _, root := validConsent(t)
	for _, mutate := range []func(*capability.Consent){
		func(c *capability.Consent) { c.Source = "verified:other" },
		func(c *capability.Consent) { c.VerifiedVersion = "2" },
		func(c *capability.Consent) { c.Install.Argv = []string{"curl", "--root", root} },
		func(c *capability.Consent) { c.OwnedFiles[0].Role = "config" },
		func(c *capability.Consent) { c.OwnedFiles[0].SHA256 = "" },
		func(c *capability.Consent) { c.OwnedFiles[0].Path = filepath.Join("..", "hooks", "post-commit") },
	} {
		_, consent, _, _ := validConsent(t)
		mutate(&consent)
		if _, err := capability.BuildInstallPlan(probe, consent); err == nil {
			t.Fatal("drift accepted")
		}
	}
}

func TestPlanRejectsFilesystemRootAsProject(t *testing.T) {
	probe, consent, _, _ := validConsent(t)
	consent.ProjectRoot = string(filepath.Separator)
	root := filepath.Join(consent.ProjectRoot, ".agent-team", "tools")
	consent.Install.Argv[2], consent.Probe.Argv[2], consent.Rollback.Argv[2] = root, root, root
	if _, err := capability.BuildInstallPlan(probe, consent); err == nil {
		t.Fatal("filesystem root accepted as project")
	}
}

func TestInstallFailureRollsBackAndVerifiesPriorBytes(t *testing.T) {
	probe, consent, path, _ := validConsent(t)
	plan, err := capability.BuildInstallPlan(probe, consent)
	if err != nil {
		t.Fatal(err)
	}
	runner := &nativeFake{result: []tracker.CommandResult{{}, {Exit: 1}, {}}, onRun: func(name string, _ []string) {
		if name == "agent-team-capability-install" {
			_ = os.WriteFile(path, []byte("new"), 0o600)
		}
	}}
	if _, err := capability.Install(context.Background(), runner, plan); err == nil {
		t.Fatal("failed post-install probe accepted")
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != "old" {
		t.Fatalf("rollback bytes=%q err=%v", got, err)
	}
	if len(runner.calls) != 3 || runner.calls[2][0] != "agent-team-capability-rollback" {
		t.Fatalf("calls=%#v", runner.calls)
	}
}

func TestInstallRejectsWrongHashAndProtectedPathsBeforeMutation(t *testing.T) {
	probe, _, path, _ := validConsent(t)
	for _, mutate := range []func(*capability.Consent){
		func(c *capability.Consent) { c.OwnedFiles[0].SHA256 = "sha256:wrong" },
		func(c *capability.Consent) { c.OwnedFiles[0].Path = ".git/hooks/post-commit" },
		func(c *capability.Consent) { c.Install.Argv = append(c.Install.Argv, "--config", "../settings") },
		func(c *capability.Consent) { c.Install.Argv = append(c.Install.Argv, "--credentials", "secret") },
	} {
		_, consent, _, _ := validConsent(t)
		mutate(&consent)
		plan, err := capability.BuildInstallPlan(probe, consent)
		if err == nil {
			runner := &nativeFake{onRun: func(string, []string) { _ = os.WriteFile(path, []byte("new"), 0o600) }}
			_, err = capability.Install(context.Background(), runner, plan)
		}
		if err == nil {
			t.Fatal("unsafe authority accepted")
		}
	}
}

func TestRollbackRequiresCapturedVerifiedBackup(t *testing.T) {
	probe, consent, _, _ := validConsent(t)
	plan, err := capability.BuildInstallPlan(probe, consent)
	if err != nil {
		t.Fatal(err)
	}
	if err := capability.Rollback(context.Background(), &nativeFake{}, plan); err == nil {
		t.Fatal("uncaptured rollback accepted")
	}
}

func TestInstallRejectsRunnerFailure(t *testing.T) {
	probe, consent, _, _ := validConsent(t)
	plan, err := capability.BuildInstallPlan(probe, consent)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := capability.Install(context.Background(), &nativeFake{result: []tracker.CommandResult{{Transport: errors.New("offline")}}}, plan); err == nil {
		t.Fatal("runner failure accepted")
	}
}
