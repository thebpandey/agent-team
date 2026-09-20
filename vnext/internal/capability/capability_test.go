package capability_test

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/thebpandey/agent-team/vnext/internal/capability"
	"github.com/thebpandey/agent-team/vnext/internal/tracker"
)

type nativeFake struct {
	calls   [][]string
	results []tracker.CommandResult
}

func (f *nativeFake) Run(_ context.Context, name string, args ...string) tracker.CommandResult {
	f.calls = append(f.calls, append([]string{name}, args...))
	if len(f.results) == 0 {
		return tracker.CommandResult{}
	}
	r := f.results[0]
	f.results = f.results[1:]
	return r
}

func TestProbeAllUsesKnownArgvAndNativeFallback(t *testing.T) {
	runner := &nativeFake{results: []tracker.CommandResult{{Stdout: []byte("serena 1.2.3\n")}}}
	got, err := capability.ProbeAll(context.Background(), runner, []capability.Name{capability.Native, capability.Serena})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || !got[0].Available || !got[0].Healthy || !got[1].Available || got[1].Version != "serena 1.2.3" {
		t.Fatalf("probes = %#v", got)
	}
	if want := [][]string{{"serena", "--version"}}; !reflect.DeepEqual(runner.calls, want) {
		t.Fatalf("argv = %#v, want %#v", runner.calls, want)
	}
}

func TestProbeAllMarksFailuresUnhealthyAndRejectsUnknown(t *testing.T) {
	runner := &nativeFake{results: []tracker.CommandResult{{Exit: 2, Stderr: []byte("not installed")}}}
	got, err := capability.ProbeAll(context.Background(), runner, []capability.Name{capability.AstGrep})
	if err != nil || len(got) != 1 || got[0].Available || got[0].Healthy || got[0].Reason == "" {
		t.Fatalf("probes = %#v, err = %v", got, err)
	}
	if _, err := capability.ProbeAll(context.Background(), runner, []capability.Name{"bogus"}); err == nil {
		t.Fatal("unknown capability accepted")
	}
}

func TestConsentRequiresHealthyVerifiedExplicitPlan(t *testing.T) {
	probe := capability.Probe{Name: capability.Serena, Mode: capability.ReadOnlyMCP, Available: true, Healthy: true, Version: "1.0"}
	consent := capability.Consent{Name: capability.Serena, Enabled: true, Mode: capability.ReadOnlyMCP, InstallerPackage: "serena@1.0", Source: "verified:example", Rollback: "backup:serena"}
	plan, err := capability.BuildInstallPlan(probe, consent)
	if err != nil || !plan.Explicit || plan.Package != consent.InstallerPackage || plan.VerifiedVersion != "1.0" {
		t.Fatalf("plan = %#v, err = %v", plan, err)
	}
	for _, bad := range []capability.Consent{
		{Name: capability.Serena, Enabled: false, Source: "verified:example"},
		{Name: capability.Serena, Enabled: true, Source: "unverified"},
		{Name: capability.Serena, Enabled: true, Source: "verified:example", InstallerPackage: "x"},
	} {
		if _, err := capability.BuildInstallPlan(probe, bad); err == nil {
			t.Fatalf("unsafe consent accepted: %#v", bad)
		}
	}
}

func TestConsentCanPlanAnExplicitVerifiedMissingCapability(t *testing.T) {
	plan, err := capability.BuildInstallPlan(
		capability.Probe{Name: capability.AstGrep, Mode: capability.CLI, Available: false},
		capability.Consent{Name: capability.AstGrep, Enabled: true, Mode: capability.CLI, InstallerPackage: "ast-grep@1.2.3", Source: "verified:example", Rollback: "backup:ast-grep"},
	)
	if err != nil || plan.VerifiedVersion != "1.2.3" || !plan.Explicit {
		t.Fatalf("plan = %#v, err = %v", plan, err)
	}
}

func validPlan() capability.InstallPlan {
	return capability.InstallPlan{
		Name:            capability.Serena,
		Package:         "serena@1",
		Source:          "verified:example",
		VerifiedVersion: "1",
		Scope:           "project",
		OwnedFiles:      []capability.OwnedFile{{Path: "config/serena.json", Role: "config", SHA256: "sha256:new"}},
		Rollback:        []capability.RollbackEntry{{Path: "config/serena.json", SHA256: "sha256:old", Backup: "backup:serena.json", Command: []string{"serena-rollback", "backup:serena.json"}}},
		Command:         []string{"serena-install", "--package", "serena@1"},
		ProbeCommand:    []string{"serena", "--version"},
		Explicit:        true,
	}
}

func TestInstallAndRollbackUseBoundedArgvOnly(t *testing.T) {
	runner := &nativeFake{results: []tracker.CommandResult{{}, {Stdout: []byte("1\n")}, {}}}
	plan := validPlan()
	probe, err := capability.Install(context.Background(), runner, plan)
	if err != nil || !probe.Available || !probe.Healthy || probe.Version != "1" {
		t.Fatalf("probe = %#v, err = %v", probe, err)
	}
	if err := capability.Rollback(context.Background(), runner, plan); err != nil {
		t.Fatal(err)
	}
	want := [][]string{{"serena-install", "--package", "serena@1"}, {"serena", "--version"}, {"serena-rollback", "backup:serena.json"}}
	if !reflect.DeepEqual(runner.calls, want) {
		t.Fatalf("argv = %#v, want %#v", runner.calls, want)
	}
}

func TestInstallFailsClosedBeforeRunningUnsafePlan(t *testing.T) {
	runner := &nativeFake{}
	cases := []capability.InstallPlan{
		func() capability.InstallPlan { p := validPlan(); p.Explicit = false; return p }(),
		func() capability.InstallPlan { p := validPlan(); p.Source = "unverified"; return p }(),
		func() capability.InstallPlan { p := validPlan(); p.Command = []string{"sh", "-c", "bad"}; return p }(),
		func() capability.InstallPlan {
			p := validPlan()
			p.Command = []string{"curl", "example.invalid"}
			return p
		}(),
		func() capability.InstallPlan { p := validPlan(); p.OwnedFiles[0].Path = "../escape"; return p }(),
		func() capability.InstallPlan {
			p := validPlan()
			p.Rollback[0].Backup, p.Rollback[0].Command = "", nil
			return p
		}(),
		func() capability.InstallPlan { p := validPlan(); p.Rollback[0].Command = nil; return p }(),
	}
	for _, plan := range cases {
		if _, err := capability.Install(context.Background(), runner, plan); err == nil {
			t.Fatalf("unsafe plan accepted: %#v", plan)
		}
	}
	if len(runner.calls) != 0 {
		t.Fatalf("unsafe plans ran: %#v", runner.calls)
	}
}

func TestInstallRejectsFailedOrTimedOutCommands(t *testing.T) {
	for _, result := range []tracker.CommandResult{{Exit: 1}, {TimedOut: true}, {Transport: errors.New("missing")}} {
		runner := &nativeFake{results: []tracker.CommandResult{result}}
		if _, err := capability.Install(context.Background(), runner, validPlan()); err == nil {
			t.Fatalf("result accepted: %#v", result)
		}
	}
}

func TestInstallRejectsCrossPlatformUnsafePaths(t *testing.T) {
	plan := validPlan()
	plan.OwnedFiles[0].Path = `C:\escape`
	plan.Rollback[0].Path = `C:\escape`
	runner := &nativeFake{results: []tracker.CommandResult{{}, {Stdout: []byte("1")}}}
	if _, err := capability.Install(context.Background(), runner, plan); err == nil {
		t.Fatal("Windows absolute path accepted")
	}
}

func TestInstallRejectsMismatchedPostInstallVersion(t *testing.T) {
	runner := &nativeFake{results: []tracker.CommandResult{{}, {Stdout: []byte("2")}}}
	if _, err := capability.Install(context.Background(), runner, validPlan()); err == nil {
		t.Fatal("mismatched installed version accepted")
	}
}

func TestInstallBoundsCallerWithoutDeadline(t *testing.T) {
	// The adapter always supplies a finite operation deadline to the runner.
	runner := deadlineFake{}
	if _, err := capability.Install(context.Background(), runner, validPlan()); err != nil {
		t.Fatal(err)
	}
}

type deadlineFake struct{}

func (deadlineFake) Run(ctx context.Context, _ string, _ ...string) tracker.CommandResult {
	if deadline, ok := ctx.Deadline(); !ok || time.Until(deadline) <= 0 {
		return tracker.CommandResult{Transport: errors.New("missing deadline")}
	}
	return tracker.CommandResult{Stdout: []byte("1")}
}
