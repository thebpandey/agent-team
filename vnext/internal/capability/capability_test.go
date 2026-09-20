package capability

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/thebpandey/agent-team/vnext/internal/core"
	"github.com/thebpandey/agent-team/vnext/internal/tracker"
)

type fakeRunner struct {
	results []tracker.CommandResult
	calls   [][]string
	envs    [][]string
	onRun   func([]string)
}

type timeoutRunner struct {
	blockOn int
	calls   int
	started chan struct{}
	install func()
}

func (r *timeoutRunner) Run(ctx context.Context, _ []string, _ []string) tracker.CommandResult {
	r.calls++
	if r.calls == 1 && r.install != nil {
		r.install()
	}
	if r.calls == r.blockOn {
		close(r.started)
		<-ctx.Done()
		return tracker.CommandResult{Transport: ctx.Err()}
	}
	if r.calls == 2 {
		return tracker.CommandResult{Stdout: []byte("1\n")}
	}
	return tracker.CommandResult{}
}

func (f *fakeRunner) Run(_ context.Context, argv, env []string) tracker.CommandResult {
	f.calls = append(f.calls, append([]string(nil), argv...))
	f.envs = append(f.envs, append([]string(nil), env...))
	if f.onRun != nil {
		f.onRun(argv)
	}
	if len(f.results) == 0 {
		return tracker.CommandResult{}
	}
	r := f.results[0]
	f.results = f.results[1:]
	return r
}

func digest(data []byte) string { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }

func TestProbeNativeAndRequestValidation(t *testing.T) {
	got, err := ProbeAll(context.Background(), nil, []Name{Native})
	if err != nil || len(got) != 1 || !got[0].Healthy {
		t.Fatalf("native probe = %#v, %v", got, err)
	}
	for _, names := range [][]Name{{Serena, Serena}, {Name("unknown")}} {
		if _, err := ProbeAll(context.Background(), nil, names); err == nil {
			t.Fatalf("accepted malformed request %#v", names)
		}
	}
	if _, err := ProbeAll(nil, nil, []Name{Native}); err == nil {
		t.Fatal("accepted nil context")
	}
}

func TestProbeAstGrepContract(t *testing.T) {
	got, err := ProbeAll(context.Background(), nil, []Name{AstGrep})
	if err != nil || len(got) != 1 || got[0].Name != AstGrep || got[0].Mode != CLI {
		t.Fatalf("ast-grep probe = %#v, %v", got, err)
	}
}

func TestProbeExecutableOutcomesAndDigest(t *testing.T) {
	dir := t.TempDir()
	name := string(Serena)
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	path := filepath.Join(dir, name)
	bytes := []byte("not executed by fake runner")
	if err := os.WriteFile(path, bytes, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	for _, tc := range []struct {
		name string
		run  *fakeRunner
		ok   bool
	}{
		{"nil", nil, false},
		{"nonzero", &fakeRunner{results: []tracker.CommandResult{{Exit: 1}}}, false},
		{"timeout", &fakeRunner{results: []tracker.CommandResult{{TimedOut: true}}}, false},
		{"transport", &fakeRunner{results: []tracker.CommandResult{{Transport: errors.New("gone")}}}, false},
		{"empty", &fakeRunner{results: []tracker.CommandResult{{Stdout: []byte(" \n")}}}, false},
		{"oversize", &fakeRunner{results: []tracker.CommandResult{{Stdout: []byte(strings.Repeat("x", probeOutputLimit+1))}}}, false},
		{"healthy", &fakeRunner{results: []tracker.CommandResult{{Stdout: []byte("1.2.3\n")}}}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var runner NativeRunner
			if tc.run != nil {
				runner = tc.run
			}
			got, err := ProbeAll(context.Background(), runner, []Name{Serena})
			if err != nil || len(got) != 1 || got[0].Healthy != tc.ok {
				t.Fatalf("probe = %#v, %v", got, err)
			}
			if tc.ok && (got[0].Path != path || got[0].Version != "1.2.3" || got[0].Digest != "sha256:"+digest(bytes)) {
				t.Fatalf("unverified executable probe: %#v", got[0])
			}
		})
	}
	missing, err := ProbeAll(context.Background(), &fakeRunner{}, []Name{Graphify})
	if err != nil || missing[0].Available {
		t.Fatalf("missing probe = %#v, %v", missing, err)
	}
}

func TestProbeRejectsExecutableChangedDuringVersionCheck(t *testing.T) {
	dir := t.TempDir()
	name := string(Serena)
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("before"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	runner := &fakeRunner{results: []tracker.CommandResult{{Stdout: []byte("1\n")}}, onRun: func([]string) {
		if err := os.WriteFile(path, []byte("after"), 0o755); err != nil {
			t.Fatal(err)
		}
	}}
	got, err := ProbeAll(context.Background(), runner, []Name{Serena})
	if err != nil || got[0].Healthy {
		t.Fatalf("accepted executable replacement: %#v, %v", got, err)
	}
}

func TestProbeSkillContentSizeRootAndDigest(t *testing.T) {
	root := t.TempDir()
	t.Setenv("AGENT_TEAM_SKILL_ROOT", root)
	path := filepath.Join(root, string(Impeccable), "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	data := []byte("skill")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := ProbeAll(context.Background(), nil, []Name{Impeccable})
	if err != nil || !got[0].Healthy || got[0].Path != path || got[0].Digest != "sha256:"+digest(data) {
		t.Fatalf("skill probe = %#v, %v", got, err)
	}
	if err := os.Truncate(path, skillContentLimit+1); err != nil {
		t.Fatal(err)
	}
	got, err = ProbeAll(context.Background(), nil, []Name{Impeccable})
	if err != nil || got[0].Healthy {
		t.Fatalf("oversized skill probe = %#v, %v", got, err)
	}
	t.Setenv("AGENT_TEAM_SKILL_ROOT", "")
	got, err = ProbeAll(context.Background(), nil, []Name{Impeccable})
	if err != nil || got[0].Healthy {
		t.Fatalf("rootless skill probe = %#v, %v", got, err)
	}
}

func TestBuildInstallPlanFailsClosedWithoutAdapter(t *testing.T) {
	p := Probe{Name: Serena, Mode: ReadOnlyMCP, Available: true, Healthy: true, Version: "1"}
	c := Consent{Name: Serena, Enabled: true, Mode: ReadOnlyMCP, InstallerPackage: "serena", Source: "registry.example/serena@1"}
	if _, err := BuildInstallPlan(p, c); err == nil || !strings.Contains(err.Error(), "installer unavailable") {
		t.Fatalf("BuildInstallPlan error = %v", err)
	}
	if _, err := BuildInstallPlan(p, Consent{Name: Serena, Mode: ReadOnlyMCP}); err == nil {
		t.Fatal("accepted declined consent")
	}
}

func TestInstallRejectsForgedZeroPlan(t *testing.T) {
	if _, err := Install(context.Background(), &fakeRunner{}, InstallPlan{}); err == nil {
		t.Fatal("accepted forged zero plan")
	}
}

func stagedPlan(t *testing.T) (InstallPlan, adapterSpec, string, *fakeRunner) {
	t.Helper()
	project := t.TempDir()
	source := filepath.Join(t.TempDir(), "release")
	if err := os.WriteFile(source, []byte("trusted source"), 0o600); err != nil {
		t.Fatal(err)
	}
	stage := filepath.Join(".agent-team", "stage", "serena")
	destination := filepath.Join(".agent-team", "tools", "serena-1")
	stageHost := filepath.Join(project, stage)
	spec := adapterSpec{name: Serena, mode: ReadOnlyMCP, packageName: "serena", source: "registry.example/serena", version: "1", project: project, sourceArtifact: source, sourceDigest: digest([]byte("trusted source")), stage: stage, destination: destination, stageDigest: digest([]byte("built artifact")), installArgv: []string{"installer", "--source", source, "--stage", stageHost}, probeArgv: []string{stageHost, "--version"}}
	p := Probe{Name: Serena, Mode: ReadOnlyMCP, Available: true, Healthy: true, Version: "1"}
	c := Consent{Name: Serena, Enabled: true, Mode: ReadOnlyMCP, InstallerPackage: "serena", Source: "registry.example/serena@1"}
	plan, err := buildInstallPlan(spec, p, c)
	if err != nil {
		t.Fatal(err)
	}
	runner := &fakeRunner{results: []tracker.CommandResult{{}, {Stdout: []byte("1\n")}}, onRun: func(argv []string) {
		if len(argv) > 0 && argv[0] == "installer" {
			if err := os.MkdirAll(filepath.Dir(stageHost), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(stageHost, []byte("built artifact"), 0o700); err != nil {
				t.Fatal(err)
			}
		}
	}}
	return plan, spec, source, runner
}

func TestInstallUsesCopiedExactArgvAndScrubbedEnvironment(t *testing.T) {
	plan, spec, _, runner := stagedPlan(t)
	spec.installArgv[0] = "caller-mutated"
	got, err := Install(context.Background(), runner, plan)
	if err != nil || !got.Healthy || got.Version != "1" {
		t.Fatalf("Install = %#v, %v", got, err)
	}
	if len(runner.calls) != 2 || strings.Join(runner.calls[0], "\x00") != strings.Join([]string{"installer", "--source", spec.sourceArtifact, "--stage", filepath.Join(spec.project, spec.stage)}, "\x00") || strings.Join(runner.calls[1], "\x00") != strings.Join([]string{filepath.Join(spec.project, spec.stage), "--version"}, "\x00") {
		t.Fatalf("argv = %#v", runner.calls)
	}
	for _, env := range runner.envs {
		if len(env) != 0 {
			t.Fatalf("unscrubbed environment: %#v", runner.envs)
		}
	}
	if _, err := os.Stat(filepath.Join(spec.project, spec.destination)); err != nil {
		t.Fatalf("published destination missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(spec.project, spec.stage)); !os.IsNotExist(err) {
		t.Fatalf("stage remains: %v", err)
	}
}

func TestInstallRejectsStageChangedDuringDirectProbe(t *testing.T) {
	plan, spec, _, runner := stagedPlan(t)
	runner.onRun = func(argv []string) {
		if argv[0] == "installer" {
			if err := os.MkdirAll(filepath.Dir(filepath.Join(spec.project, spec.stage)), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(spec.project, spec.stage), []byte("built artifact"), 0o700); err != nil {
				t.Fatal(err)
			}
			return
		}
		if err := os.WriteFile(filepath.Join(spec.project, spec.stage), []byte("replaced after probe"), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := Install(context.Background(), runner, plan); err == nil {
		t.Fatal("published artifact changed during direct probe")
	}
	if _, err := os.Stat(filepath.Join(spec.project, spec.destination)); !os.IsNotExist(err) {
		t.Fatalf("published changed stage: %v", err)
	}
}

func TestAdapterSpecRejectsShellLaunchers(t *testing.T) {
	for _, launcher := range []string{"sh", "sh.exe", "bash", "bash.exe", "dash", "dash.exe", "zsh", "zsh.exe", "fish", "fish.exe", "cmd", "cmd.exe", "command.com", "powershell", "powershell.exe", "pwsh", "pwsh.exe", "/bin/SH", `C:\\Windows\\System32\\CMD.EXE`} {
		t.Run(launcher, func(t *testing.T) {
			_, spec, _, _ := stagedPlan(t)
			spec.installArgv = []string{launcher}
			probe := Probe{Name: Serena, Mode: ReadOnlyMCP, Available: true, Healthy: true, Version: "1"}
			consent := Consent{Name: Serena, Enabled: true, Mode: ReadOnlyMCP, InstallerPackage: "serena", Source: "registry.example/serena@1"}
			if _, err := buildInstallPlan(spec, probe, consent); err == nil {
				t.Fatal("accepted shell launcher")
			}
		})
	}
}

func TestAdapterSpecBoundsArgv(t *testing.T) {
	probe := Probe{Name: Serena, Mode: ReadOnlyMCP, Available: true, Healthy: true, Version: "1"}
	consent := Consent{Name: Serena, Enabled: true, Mode: ReadOnlyMCP, InstallerPackage: "serena", Source: "registry.example/serena@1"}
	_, spec, _, _ := stagedPlan(t)
	spec.installArgv = make([]string, maxInstallArgv+1)
	for i := range spec.installArgv {
		spec.installArgv[i] = "tool"
	}
	if _, err := buildInstallPlan(spec, probe, consent); err == nil {
		t.Fatal("accepted too many argv entries")
	}
	_, spec, _, _ = stagedPlan(t)
	spec.installArgv = []string{strings.Repeat("x", int(core.DefaultConfig().Storage.ArgumentBytes)+1)}
	if _, err := buildInstallPlan(spec, probe, consent); err == nil {
		t.Fatal("accepted oversized argv")
	}
}

func TestInstallEnforcesDeadlineForEveryRunnerCall(t *testing.T) {
	old := installTimeout
	installTimeout = time.Millisecond
	defer func() { installTimeout = old }()
	for _, blockOn := range []int{1, 2} {
		t.Run(fmt.Sprintf("call-%d", blockOn), func(t *testing.T) {
			plan, spec, _, _ := stagedPlan(t)
			stage := filepath.Join(spec.project, spec.stage)
			runner := &timeoutRunner{blockOn: blockOn, started: make(chan struct{}), install: func() {
				if err := os.MkdirAll(filepath.Dir(stage), 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(stage, []byte("built artifact"), 0o700); err != nil {
					t.Fatal(err)
				}
			}}
			if _, err := Install(context.Background(), runner, plan); err == nil || !strings.Contains(err.Error(), "timed out") {
				t.Fatalf("deadline error = %v", err)
			}
			select {
			case <-runner.started:
			default:
				t.Fatal("runner never received bounded context")
			}
			if _, err := os.Stat(filepath.Join(spec.project, spec.destination)); !os.IsNotExist(err) {
				t.Fatalf("published timed-out install: %v", err)
			}
		})
	}
}

func TestInstallRejectsSourceTamperWithoutMutation(t *testing.T) {
	plan, spec, source, runner := stagedPlan(t)
	if err := os.WriteFile(source, []byte("tampered"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Install(context.Background(), runner, plan); err == nil {
		t.Fatal("accepted tampered source")
	}
	if len(runner.calls) != 0 {
		t.Fatalf("ran installer after source tamper: %#v", runner.calls)
	}
	if _, err := os.Stat(filepath.Join(spec.project, spec.destination)); !os.IsNotExist(err) {
		t.Fatalf("destination changed: %v", err)
	}
}

func TestInstallNeverOverwritesFreshDestination(t *testing.T) {
	plan, spec, _, runner := stagedPlan(t)
	final := filepath.Join(spec.project, spec.destination)
	if err := os.MkdirAll(filepath.Dir(final), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(final, []byte("user file"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Install(context.Background(), runner, plan); err == nil {
		t.Fatal("overwrote existing destination")
	}
	got, err := os.ReadFile(final)
	if err != nil || string(got) != "user file" || len(runner.calls) != 0 {
		t.Fatalf("destination = %q, calls = %#v, err = %v", got, runner.calls, err)
	}
}

func TestInstallCleansStageOnFailureAndReportsUnsafeAmbiguity(t *testing.T) {
	plan, spec, _, runner := stagedPlan(t)
	runner.results = []tracker.CommandResult{{Exit: 1}}
	if _, err := Install(context.Background(), runner, plan); err == nil {
		t.Fatal("accepted failing install")
	}
	if _, err := os.Stat(filepath.Join(spec.project, spec.stage)); !os.IsNotExist(err) {
		t.Fatalf("stage not cleaned: %v", err)
	}
	plan, spec, _, runner = stagedPlan(t)
	runner.results = []tracker.CommandResult{{Exit: 1}}
	runner.onRun = func(argv []string) {
		if argv[0] == "installer" {
			if err := os.MkdirAll(filepath.Join(spec.project, spec.stage), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(spec.project, spec.stage, "unknown"), []byte("x"), 0o600); err != nil {
				t.Fatal(err)
			}
		}
	}
	if _, err := Install(context.Background(), runner, plan); err == nil || !strings.Contains(err.Error(), "unsafe") {
		t.Fatalf("ambiguous removal error = %v", err)
	}
}

func TestInstallRootRejectsSymlinkParentEscape(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires privilege on Windows")
	}
	plan, spec, _, runner := stagedPlan(t)
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(spec.project, ".agent-team")); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}
	if _, err := Install(context.Background(), runner, plan); err == nil {
		t.Fatal("accepted symlink parent")
	}
	if entries, err := os.ReadDir(outside); err != nil || len(entries) != 0 {
		t.Fatalf("outside root mutated: %#v, %v", entries, err)
	}
}

func TestRollbackRemovesOnlyPublishedExactDestination(t *testing.T) {
	plan, spec, _, runner := stagedPlan(t)
	if _, err := Install(context.Background(), runner, plan); err != nil {
		t.Fatal(err)
	}
	if err := Rollback(context.Background(), &fakeRunner{}, plan); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(spec.project, spec.destination)); !os.IsNotExist(err) {
		t.Fatalf("exact published destination remains: %v", err)
	}
	if err := Rollback(context.Background(), nil, plan); err != nil {
		t.Fatalf("prior exact removal was not idempotent: %v", err)
	}
}

func TestRollbackRefusesForeignTamperedAndMissingDestination(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(t *testing.T, path string)
	}{
		{"foreign", func(t *testing.T, path string) {
			if err := os.WriteFile(path, []byte("foreign"), 0o600); err != nil {
				t.Fatal(err)
			}
		}},
		{"tampered", func(t *testing.T, path string) {
			if err := os.WriteFile(path, []byte("tampered"), 0o700); err != nil {
				t.Fatal(err)
			}
		}},
		{"missing", func(t *testing.T, path string) {
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			plan, spec, _, runner := stagedPlan(t)
			if _, err := Install(context.Background(), runner, plan); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(spec.project, spec.destination)
			tc.mutate(t, path)
			if err := Rollback(context.Background(), nil, plan); err == nil {
				t.Fatal("removed or accepted non-owned destination")
			}
			if tc.name != "missing" {
				if _, err := os.Stat(path); err != nil {
					t.Fatalf("foreign destination removed: %v", err)
				}
			}
		})
	}
	plan, _, _, _ := stagedPlan(t)
	if err := Rollback(context.Background(), nil, plan); err != nil {
		t.Fatalf("unpublished plan was not idempotent: %v", err)
	}
}
