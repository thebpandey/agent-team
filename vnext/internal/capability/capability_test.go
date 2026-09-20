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
	install func([]string)
}

func (r *timeoutRunner) Run(ctx context.Context, argv []string, _ []string) tracker.CommandResult {
	r.calls++
	if r.calls == 1 && r.install != nil {
		r.install(argv)
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

func TestInstallExternalRunnerNeverReceivesManagedProjectPath(t *testing.T) {
	plan, spec, _, runner := stagedPlan(t)
	if _, err := Install(context.Background(), runner, plan); err != nil {
		t.Fatal(err)
	}
	for _, argv := range runner.calls {
		for _, arg := range argv {
			if strings.Contains(arg, spec.project) || strings.Contains(strings.ReplaceAll(arg, "\\", "/"), "/.agent-team/") {
				t.Fatalf("runner received managed project path %q in %#v", arg, argv)
			}
		}
	}
}

func TestInstallRejectsExternalOutputChangedDuringProbe(t *testing.T) {
	plan, spec, _, runner := stagedPlan(t)
	runner.onRun = func(argv []string) {
		if argv[0] == "installer" {
			if err := os.WriteFile(argv[len(argv)-1], []byte("built artifact"), 0o700); err != nil {
				t.Fatal(err)
			}
			return
		}
		if err := os.WriteFile(argv[0], []byte("changed after probe"), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := Install(context.Background(), runner, plan); err == nil {
		t.Fatal("accepted external output mutation")
	}
	if _, err := os.Stat(filepath.Join(spec.project, spec.destination)); !os.IsNotExist(err) {
		t.Fatalf("published mutated output: %v", err)
	}
}

func TestInstallRejectsPostPublishDestinationSwap(t *testing.T) {
	plan, spec, _, runner := stagedPlan(t)
	outside := filepath.Join(t.TempDir(), "outside")
	if err := os.WriteFile(outside, []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}
	old := publishHook
	publishHook = func(path string) {
		if err := os.Remove(path); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(outside, path); err != nil {
			t.Fatal(err)
		}
	}
	defer func() { publishHook = old }()
	if _, err := Install(context.Background(), runner, plan); err == nil {
		t.Fatal("accepted post-publish swap")
	}
	got, err := os.ReadFile(outside)
	if err != nil || string(got) != "outside" {
		t.Fatalf("outside changed: %q, %v", got, err)
	}
	_ = spec
}

func TestInstallRejectsProjectParentSwapWithoutOutsideMutation(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires privilege on Windows")
	}
	plan, spec, _, runner := stagedPlan(t)
	outside := t.TempDir()
	runner.onRun = func(argv []string) {
		if argv[0] != "installer" {
			return
		}
		if err := os.Symlink(outside, filepath.Join(spec.project, ".agent-team")); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(argv[len(argv)-1], []byte("built artifact"), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := Install(context.Background(), runner, plan); err == nil {
		t.Fatal("accepted project parent swap")
	}
	if entries, err := os.ReadDir(outside); err != nil || len(entries) != 0 {
		t.Fatalf("outside changed: %#v, %v", entries, err)
	}
}

func TestInstallRejectsPostPublishTamper(t *testing.T) {
	plan, spec, _, runner := stagedPlan(t)
	old := publishHook
	publishHook = func(path string) {
		if err := os.WriteFile(path, []byte("tampered"), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	defer func() { publishHook = old }()
	if _, err := Install(context.Background(), runner, plan); err == nil {
		t.Fatal("accepted post-publish tamper")
	}
	got, err := os.ReadFile(filepath.Join(spec.project, spec.destination))
	if err != nil || string(got) != "tampered" {
		t.Fatalf("unexpected tamper cleanup: %q, %v", got, err)
	}
}

func stagedPlan(t *testing.T) (InstallPlan, adapterSpec, string, *fakeRunner) {
	t.Helper()
	project := t.TempDir()
	source := filepath.Join(t.TempDir(), "release")
	if err := os.WriteFile(source, []byte("trusted source"), 0o600); err != nil {
		t.Fatal(err)
	}
	stage := "output"
	destination := filepath.Join(".agent-team", "tools", "serena-1")
	spec := adapterSpec{name: Serena, mode: ReadOnlyMCP, packageName: "serena", source: "registry.example/serena", version: "1", project: project, sourceArtifact: source, sourceDigest: digest([]byte("trusted source")), stage: stage, destination: destination, stageDigest: digest([]byte("built artifact")), installArgv: []string{"installer", "--source", source, "--output", externalOutputTag}, probeArgv: []string{externalOutputTag, "--version"}}
	p := Probe{Name: Serena, Mode: ReadOnlyMCP, Available: true, Healthy: true, Version: "1"}
	c := Consent{Name: Serena, Enabled: true, Mode: ReadOnlyMCP, InstallerPackage: "serena", Source: "registry.example/serena@1"}
	plan, err := buildInstallPlan(spec, p, c)
	if err != nil {
		t.Fatal(err)
	}
	runner := &fakeRunner{results: []tracker.CommandResult{{}, {Stdout: []byte("1\n")}}, onRun: func(argv []string) {
		if len(argv) > 0 && argv[0] == "installer" {
			if err := os.WriteFile(argv[len(argv)-1], []byte("built artifact"), 0o700); err != nil {
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
	if len(runner.calls) != 2 || strings.Join(runner.calls[0][:4], "\x00") != strings.Join([]string{"installer", "--source", spec.sourceArtifact, "--output"}, "\x00") || strings.Join(runner.calls[1], "\x00") != strings.Join([]string{runner.calls[0][4], "--version"}, "\x00") {
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
	if _, err := os.Stat(runner.calls[0][4]); !os.IsNotExist(err) {
		t.Fatalf("external output remains: %v", err)
	}
}

func TestInstallRejectsStageChangedDuringDirectProbe(t *testing.T) {
	plan, spec, _, runner := stagedPlan(t)
	runner.onRun = func(argv []string) {
		if argv[0] == "installer" {
			if err := os.WriteFile(argv[len(argv)-1], []byte("built artifact"), 0o700); err != nil {
				t.Fatal(err)
			}
			return
		}
		if err := os.WriteFile(argv[0], []byte("replaced after probe"), 0o700); err != nil {
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
	for _, launcher := range []string{"sh", "sh.exe", "bash", "bash.exe", "dash", "dash.exe", "zsh", "zsh.exe", "fish", "fish.exe", "ash", "ash.exe", "ksh", "ksh.exe", "mksh", "mksh.exe", "csh", "csh.exe", "tcsh", "tcsh.exe", "yash", "yash.exe", "cmd", "cmd.exe", "command.com", "powershell", "powershell.exe", "pwsh", "pwsh.exe", "/bin/SH", `/usr/local/bin/MKSH.EXE`, `C:\\Windows\\System32\\CMD.EXE`} {
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
	_, spec, _, _ = stagedPlan(t)
	spec.installArgv = []string{"installer", "./.agent-team/output", externalOutputTag}
	if _, err := buildInstallPlan(spec, probe, consent); err == nil {
		t.Fatal("accepted managed relative argv")
	}
}

func TestInstallEnforcesDeadlineForEveryRunnerCall(t *testing.T) {
	old := installTimeout
	installTimeout = time.Millisecond
	defer func() { installTimeout = old }()
	for _, blockOn := range []int{1, 2} {
		t.Run(fmt.Sprintf("call-%d", blockOn), func(t *testing.T) {
			plan, spec, _, _ := stagedPlan(t)
			runner := &timeoutRunner{blockOn: blockOn, started: make(chan struct{}), install: func(argv []string) {
				if err := os.WriteFile(argv[len(argv)-1], []byte("built artifact"), 0o700); err != nil {
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

func TestInstallCleansExternalOutputOnFailure(t *testing.T) {
	plan, _, _, runner := stagedPlan(t)
	runner.results = []tracker.CommandResult{{Exit: 1}}
	var output string
	runner.onRun = func(argv []string) {
		output = argv[len(argv)-1]
		if err := os.WriteFile(output, []byte("partial"), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := Install(context.Background(), runner, plan); err == nil {
		t.Fatal("accepted failing install")
	}
	if _, err := os.Stat(output); !os.IsNotExist(err) {
		t.Fatalf("external output remains: %v", err)
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
	if err := Rollback(context.Background(), &fakeRunner{}, plan); err == nil || !strings.Contains(err.Error(), "mutation guard unavailable") {
		t.Fatalf("Rollback() = %v, want explicit fail-closed error", err)
	}
	if got, err := os.ReadFile(filepath.Join(spec.project, spec.destination)); err != nil || string(got) != "built artifact" {
		t.Fatalf("fail-closed rollback changed destination: %q, %v", got, err)
	}
}

func TestRollbackRetainsDestinationAcrossDeterministicInterleaving(t *testing.T) {
	plan, spec, _, runner := stagedPlan(t)
	if _, err := Install(context.Background(), runner, plan); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(spec.project, spec.destination)
	interleaved := make(chan struct{})
	old := removeHook
	removeHook = func(string) { close(interleaved) }
	defer func() { removeHook = old }()
	if err := Rollback(context.Background(), nil, plan); err == nil {
		t.Fatal("rollback without a project mutation guard succeeded")
	}
	select {
	case <-interleaved:
	default:
		t.Fatal("identity-to-removal interleaving seam was not reached")
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != "built artifact" {
		t.Fatalf("interleaved rollback changed destination: %q, %v", got, err)
	}
}

func TestContainsManagedPathIsCaseInsensitiveAndComponentWise(t *testing.T) {
	project := `C:\\Users\\Dev\\Project`
	for _, argv := range [][]string{
		{`tool`, `c:\\users\\dev\\PROJECT\\out`},
		{`tool`, `D:\\Temp\\.AGENT-TEAM\\out`},
	} {
		if !containsManagedPath(argv, project) {
			t.Fatalf("managed path not detected: %#v", argv)
		}
	}
	for _, argv := range [][]string{
		{`tool`, `C:\\Users\\Dev\\Projection\\out`},
		{`tool`, `D:\\Temp\\.agent-team-backup\\out`},
	} {
		if containsManagedPath(argv, project) {
			t.Fatalf("non-component path rejected: %#v", argv)
		}
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
