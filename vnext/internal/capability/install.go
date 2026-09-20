package capability

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/thebpandey/agent-team/vnext/internal/core"
	"github.com/thebpandey/agent-team/vnext/internal/tracker"
)

const maxInstallArgv = 64

// installTimeout is a test seam; production stays aligned with ProbeAll.
var installTimeout = probeTimeout

// buildInstallPlan is intentionally package-private: Task 21's concrete
// adapters provide fixed specs here, while the public entry point stays
// fail-closed until such an adapter exists.
func buildInstallPlan(spec adapterSpec, probe Probe, consent Consent) (InstallPlan, error) {
	if err := validateConsent(probe, consent); err != nil {
		return InstallPlan{}, err
	}
	project, err := filepath.Abs(spec.project)
	if err != nil {
		return InstallPlan{}, fmt.Errorf("project root: %w", err)
	}
	if spec.name != consent.Name || spec.mode != consent.Mode || spec.packageName != consent.InstallerPackage || consent.Source != spec.source+"@"+spec.version || spec.version == "" || (probe.Available && probe.Version != spec.version) {
		return InstallPlan{}, fmt.Errorf("consent does not match adapter")
	}
	if info, err := os.Stat(project); err != nil || !info.IsDir() {
		return InstallPlan{}, fmt.Errorf("invalid project root")
	}
	if !validDigest(spec.sourceDigest) || !validDigest(spec.stageDigest) || !validRelative(spec.stage) || !validRelative(spec.destination) || spec.stage == spec.destination || !validArgv(spec.installArgv) {
		return InstallPlan{}, fmt.Errorf("invalid adapter specification")
	}
	stagePath := filepath.Join(project, spec.stage)
	if !reflect.DeepEqual(spec.probeArgv, []string{stagePath, "--version"}) {
		return InstallPlan{}, fmt.Errorf("invalid staged probe")
	}
	if _, err := sourceBytes(spec.sourceArtifact, spec.sourceDigest); err != nil {
		return InstallPlan{}, fmt.Errorf("invalid verified source: %w", err)
	}
	spec.project = project
	spec.installArgv = append([]string(nil), spec.installArgv...)
	spec.probeArgv = append([]string(nil), spec.probeArgv...)
	return InstallPlan{state: &planState{spec: spec}}, nil
}

// Install writes only an adapter-owned stage and publishes it atomically to an
// absent versioned destination. It never replaces user content or rolls back.
func Install(ctx context.Context, runner NativeRunner, plan InstallPlan) (Probe, error) {
	if ctx == nil || runner == nil || plan.state == nil {
		return Probe{}, fmt.Errorf("install requires context, runner, and plan")
	}
	state := plan.state
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.used {
		return Probe{}, fmt.Errorf("install plan already used")
	}
	state.used = true

	root, err := os.OpenRoot(state.spec.project)
	if err != nil {
		return Probe{}, fmt.Errorf("unsafe project root: %w", err)
	}
	defer root.Close()
	if err := absent(root, state.spec.destination); err != nil {
		return Probe{}, err
	}
	if err := absent(root, state.spec.stage); err != nil {
		return Probe{}, err
	}
	// Read and hash the source immediately before its fixed argv is executed.
	if _, err := sourceBytes(state.spec.sourceArtifact, state.spec.sourceDigest); err != nil {
		return Probe{}, fmt.Errorf("verified source changed: %w", err)
	}
	if result := runInstall(ctx, runner, state.spec.installArgv); failed(result) {
		return Probe{}, failStage(root, state.spec.stage, "install action failed: "+commandReason(result))
	}
	if err := verifyStaged(root, state.spec.stage, state.spec.stageDigest); err != nil {
		return Probe{}, failStage(root, state.spec.stage, "staged artifact verification failed: "+err.Error())
	}
	result := runInstall(ctx, runner, state.spec.probeArgv)
	if failed(result) || len(result.Stdout) > probeOutputLimit || strings.TrimSpace(string(result.Stdout)) != state.spec.version {
		reason := "post-install probe failed"
		if failed(result) {
			reason += ": " + commandReason(result)
		}
		return Probe{}, failStage(root, state.spec.stage, reason)
	}
	if err := verifyStaged(root, state.spec.stage, state.spec.stageDigest); err != nil {
		return Probe{}, failStage(root, state.spec.stage, "staged artifact changed during probe: "+err.Error())
	}
	if err := absent(root, state.spec.destination); err != nil {
		return Probe{}, failStage(root, state.spec.stage, "destination changed before publish: "+err.Error())
	}
	if err := root.MkdirAll(filepath.Dir(state.spec.destination), 0o700); err != nil {
		return Probe{}, failStage(root, state.spec.stage, "destination directory: "+err.Error())
	}
	if err := absent(root, state.spec.destination); err != nil {
		return Probe{}, failStage(root, state.spec.stage, "destination changed before publish: "+err.Error())
	}
	// Link publishes only if destination is still absent; unlike Rename, it
	// cannot replace a file created between the checks above.
	if err := root.Link(state.spec.stage, state.spec.destination); err != nil {
		_ = failStage(root, state.spec.stage, "publish did not complete")
		return Probe{}, fmt.Errorf("unsafe publish ambiguity: %w", err)
	}
	if err := root.Remove(state.spec.stage); err != nil {
		return Probe{}, fmt.Errorf("unsafe publish cleanup ambiguity: %w", err)
	}
	state.published = true
	return Probe{Name: state.spec.name, Mode: state.spec.mode, Path: filepath.Join(state.spec.project, state.spec.destination), Version: state.spec.version, Digest: "sha256:" + state.spec.stageDigest, Available: true, Healthy: true}, nil
}

// Rollback only removes the exact, fresh destination published by this plan.
// It never invokes an adapter or uses the runner.
func Rollback(ctx context.Context, _ NativeRunner, plan InstallPlan) error {
	if ctx == nil || plan.state == nil {
		return fmt.Errorf("rollback requires context and plan")
	}
	state := plan.state
	state.mu.Lock()
	defer state.mu.Unlock()
	if !state.published || state.removed {
		return nil
	}
	root, err := os.OpenRoot(state.spec.project)
	if err != nil {
		return fmt.Errorf("unsafe project root: %w", err)
	}
	defer root.Close()
	if err := verifyStaged(root, state.spec.destination, state.spec.stageDigest); err != nil {
		return fmt.Errorf("unsafe rollback identity: %w", err)
	}
	if err := root.Remove(state.spec.destination); err != nil {
		return fmt.Errorf("unsafe rollback removal: %w", err)
	}
	if _, err := root.Lstat(state.spec.destination); !os.IsNotExist(err) {
		if err == nil {
			return fmt.Errorf("unsafe rollback removal: destination remains")
		}
		return fmt.Errorf("unsafe rollback identity: %w", err)
	}
	state.removed = true
	return nil
}

func sourceBytes(path, want string) ([]byte, error) {
	if path == "" {
		return nil, fmt.Errorf("source artifact unavailable")
	}
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() > skillContentLimit {
		return nil, fmt.Errorf("source artifact is not a bounded regular file")
	}
	data, err := os.ReadFile(path)
	if err != nil || digestWithoutPrefix(data) != want {
		return nil, fmt.Errorf("source digest mismatch")
	}
	return data, nil
}

func verifyStaged(root *os.Root, path, want string) error {
	info, err := root.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() > skillContentLimit {
		return fmt.Errorf("not a bounded regular file")
	}
	f, err := root.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, skillContentLimit+1))
	if err != nil || len(data) > skillContentLimit || digestWithoutPrefix(data) != want {
		return fmt.Errorf("stage digest mismatch")
	}
	return nil
}

func failStage(root *os.Root, stage, reason string) error {
	if err := root.Remove(stage); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("unsafe stage cleanup after %s: %w", reason, err)
	}
	if _, err := root.Lstat(stage); !os.IsNotExist(err) {
		if err == nil {
			return fmt.Errorf("unsafe stage cleanup after %s: stage remains", reason)
		}
		return fmt.Errorf("unsafe stage identity after %s: %w", reason, err)
	}
	return fmt.Errorf("%s", reason)
}

func absent(root *os.Root, path string) error {
	if _, err := root.Lstat(path); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return fmt.Errorf("unsafe managed path %q: %w", path, err)
	}
	return fmt.Errorf("managed destination already exists: %q", path)
}

func validArgv(argv []string) bool {
	if len(argv) == 0 || len(argv) > maxInstallArgv || argv[0] == "" || shellLauncher(argv[0]) {
		return false
	}
	var bytes int64
	for _, arg := range argv {
		if strings.IndexByte(arg, 0) >= 0 {
			return false
		}
		bytes += int64(len(arg))
		if bytes > core.DefaultConfig().Storage.ArgumentBytes {
			return false
		}
	}
	return true
}

func shellLauncher(argv0 string) bool {
	name := strings.ToLower(strings.ReplaceAll(argv0, "\\", "/"))
	if slash := strings.LastIndexByte(name, '/'); slash >= 0 {
		name = name[slash+1:]
	}
	_, prohibited := shellBasenames[name]
	return prohibited
}

// shellBasenames is a closed denial set. Concrete Task21 adapters must use
// code-reviewed exact launchers; this is deliberately not a public allowlist.
var shellBasenames = map[string]struct{}{
	"sh": {}, "sh.exe": {}, "bash": {}, "bash.exe": {}, "dash": {}, "dash.exe": {},
	"zsh": {}, "zsh.exe": {}, "fish": {}, "fish.exe": {}, "ash": {}, "ash.exe": {},
	"ksh": {}, "ksh.exe": {}, "mksh": {}, "mksh.exe": {}, "csh": {}, "csh.exe": {},
	"tcsh": {}, "tcsh.exe": {}, "yash": {}, "yash.exe": {}, "cmd": {}, "cmd.exe": {},
	"command.com": {}, "powershell": {}, "powershell.exe": {}, "pwsh": {}, "pwsh.exe": {},
}

func runInstall(ctx context.Context, runner NativeRunner, argv []string) tracker.CommandResult {
	callCtx, cancel := context.WithTimeout(ctx, installTimeout)
	defer cancel()
	result := runner.Run(callCtx, append([]string(nil), argv...), []string{})
	if errors.Is(callCtx.Err(), context.DeadlineExceeded) {
		result.Exit = -1
		result.TimedOut = true
		result.Transport = nil
	}
	return result
}

func validRelative(path string) bool {
	return path != "" && !filepath.IsAbs(path) && filepath.VolumeName(path) == "" && filepath.Clean(path) == path && path != "." && path != ".." && !strings.HasPrefix(path, ".."+string(filepath.Separator))
}

func validDigest(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, c := range value {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}

func digestWithoutPrefix(data []byte) string {
	return strings.TrimPrefix(sha256Digest(data), "sha256:")
}

func failed(result tracker.CommandResult) bool {
	return result.TimedOut || result.Transport != nil || result.Exit != 0
}

func commandReason(result tracker.CommandResult) string {
	switch {
	case result.TimedOut:
		return "command timed out"
	case result.Transport != nil:
		return "command transport failed"
	case result.Exit != 0:
		return fmt.Sprintf("command exited %d", result.Exit)
	default:
		return "command failed"
	}
}
