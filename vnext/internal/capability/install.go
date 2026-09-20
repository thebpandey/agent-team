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

const (
	maxInstallArgv    = 64
	externalOutputTag = "{agent-team-output}"
)

// installTimeout is a test seam; production stays aligned with ProbeAll.
var installTimeout = probeTimeout

// publishHook is test-only synchronization for post-write identity checks.
var publishHook func(string)

// buildInstallPlan is package-private: Task 21 adapters provide fixed specs,
// while the public entry point remains fail-closed until then.
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
	if !validDigest(spec.sourceDigest) || !validDigest(spec.stageDigest) || !validRelative(spec.stage) || !validRelative(spec.destination) || spec.stage == spec.destination || !validArgv(spec.installArgv) || containsManagedPath(spec.installArgv, project) {
		return InstallPlan{}, fmt.Errorf("invalid adapter specification")
	}
	if outputTags(spec.installArgv) != 1 || !reflect.DeepEqual(spec.probeArgv, []string{externalOutputTag, "--version"}) {
		return InstallPlan{}, fmt.Errorf("invalid external output probe")
	}
	if _, err := sourceBytes(spec.sourceArtifact, spec.sourceDigest); err != nil {
		return InstallPlan{}, fmt.Errorf("invalid verified source: %w", err)
	}
	spec.project = project
	spec.installArgv = append([]string(nil), spec.installArgv...)
	spec.probeArgv = append([]string(nil), spec.probeArgv...)
	return InstallPlan{state: &planState{spec: spec}}, nil
}

// Install gives the adapter only a private external output path. It validates
// those bytes before copying them exclusively into an absent project path.
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

	projectRoot, err := os.OpenRoot(state.spec.project)
	if err != nil {
		return Probe{}, fmt.Errorf("unsafe project root: %w", err)
	}
	defer projectRoot.Close()
	if err := absent(projectRoot, state.spec.destination); err != nil {
		return Probe{}, err
	}
	if _, err := sourceBytes(state.spec.sourceArtifact, state.spec.sourceDigest); err != nil {
		return Probe{}, fmt.Errorf("verified source changed: %w", err)
	}

	externalDir, err := os.MkdirTemp("", "agent-team-capability-")
	if err != nil {
		return Probe{}, err
	}
	defer os.RemoveAll(externalDir)
	externalRoot, err := os.OpenRoot(externalDir)
	if err != nil {
		return Probe{}, fmt.Errorf("external output root: %w", err)
	}
	defer externalRoot.Close()
	externalPath := filepath.Join(externalDir, state.spec.stage)
	installArgv := replaceOutput(state.spec.installArgv, externalPath)
	if result := runInstall(ctx, runner, installArgv); failed(result) {
		return Probe{}, fmt.Errorf("install action failed: %s", commandReason(result))
	}
	if _, err := readVerified(externalRoot, state.spec.stage, state.spec.stageDigest); err != nil {
		return Probe{}, fmt.Errorf("external artifact verification failed: %w", err)
	}
	probeArgv := replaceOutput(state.spec.probeArgv, externalPath)
	result := runInstall(ctx, runner, probeArgv)
	if failed(result) || len(result.Stdout) > probeOutputLimit || strings.TrimSpace(string(result.Stdout)) != state.spec.version {
		if failed(result) {
			return Probe{}, fmt.Errorf("post-install probe failed: %s", commandReason(result))
		}
		return Probe{}, fmt.Errorf("post-install probe failed")
	}
	bytes, err := readVerified(externalRoot, state.spec.stage, state.spec.stageDigest)
	if err != nil {
		return Probe{}, fmt.Errorf("external artifact changed during probe: %w", err)
	}
	if err := publish(projectRoot, state.spec, bytes); err != nil {
		return Probe{}, err
	}
	state.published = true
	return Probe{Name: state.spec.name, Mode: state.spec.mode, Path: filepath.Join(state.spec.project, state.spec.destination), Version: state.spec.version, Digest: "sha256:" + state.spec.stageDigest, Available: true, Healthy: true}, nil
}

func publish(root *os.Root, spec adapterSpec, bytes []byte) error {
	if err := absent(root, spec.destination); err != nil {
		return err
	}
	if err := root.MkdirAll(filepath.Dir(spec.destination), 0o700); err != nil {
		return fmt.Errorf("destination directory: %w", err)
	}
	f, err := root.OpenFile(spec.destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o700)
	if err != nil {
		return fmt.Errorf("unsafe exclusive destination create: %w", err)
	}
	_, writeErr := f.Write(bytes)
	closeErr := f.Close()
	if writeErr != nil || closeErr != nil {
		return removeCreated(root, spec.destination, "", true, fmt.Errorf("destination write failed: %w", firstErr(writeErr, closeErr)))
	}
	if publishHook != nil {
		publishHook(filepath.Join(spec.project, spec.destination))
	}
	if _, err := readVerified(root, spec.destination, spec.stageDigest); err != nil {
		return removeCreated(root, spec.destination, spec.stageDigest, false, fmt.Errorf("unsafe post-publish identity: %w", err))
	}
	return nil
}

func firstErr(a, b error) error {
	if a != nil {
		return a
	}
	return b
}

func removeCreated(root *os.Root, path, want string, partial bool, cause error) error {
	if !partial {
		if _, err := readVerified(root, path, want); err != nil {
			return cause
		}
	}
	if err := root.Remove(path); err != nil {
		return fmt.Errorf("unsafe destination cleanup after %v: %w", cause, err)
	}
	if _, err := root.Lstat(path); !os.IsNotExist(err) {
		return fmt.Errorf("unsafe destination cleanup after %v", cause)
	}
	return cause
}

// Rollback removes only the exact fresh destination previously published by
// this plan. It never invokes the runner or an adapter helper.
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
	if _, err := readVerified(root, state.spec.destination, state.spec.stageDigest); err != nil {
		return fmt.Errorf("unsafe rollback identity: %w", err)
	}
	if err := root.Remove(state.spec.destination); err != nil {
		return fmt.Errorf("unsafe rollback removal: %w", err)
	}
	if _, err := root.Lstat(state.spec.destination); !os.IsNotExist(err) {
		return fmt.Errorf("unsafe rollback removal")
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

func readVerified(root *os.Root, path, want string) ([]byte, error) {
	info, err := root.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() > skillContentLimit {
		return nil, fmt.Errorf("not a bounded regular file")
	}
	f, err := root.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, skillContentLimit+1))
	if err != nil || len(data) > skillContentLimit || (want != "" && digestWithoutPrefix(data) != want) {
		return nil, fmt.Errorf("artifact digest mismatch")
	}
	return data, nil
}

func absent(root *os.Root, path string) error {
	if _, err := root.Lstat(path); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return fmt.Errorf("unsafe managed path %q: %w", path, err)
	}
	return fmt.Errorf("managed destination already exists: %q", path)
}

func replaceOutput(argv []string, output string) []string {
	copy := append([]string(nil), argv...)
	for i := range copy {
		if copy[i] == externalOutputTag {
			copy[i] = output
		}
	}
	return copy
}

func outputTags(argv []string) int {
	n := 0
	for _, arg := range argv {
		if arg == externalOutputTag {
			n++
		}
	}
	return n
}

func containsManagedPath(argv []string, project string) bool {
	for _, arg := range argv {
		normalized := strings.ReplaceAll(arg, "\\", "/")
		if strings.Contains(arg, project) {
			return true
		}
		for _, part := range strings.Split(normalized, "/") {
			if part == ".agent-team" {
				return true
			}
		}
	}
	return false
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
	"sh": {}, "sh.exe": {}, "bash": {}, "bash.exe": {}, "dash": {}, "dash.exe": {}, "zsh": {}, "zsh.exe": {}, "fish": {}, "fish.exe": {}, "ash": {}, "ash.exe": {}, "ksh": {}, "ksh.exe": {}, "mksh": {}, "mksh.exe": {}, "csh": {}, "csh.exe": {}, "tcsh": {}, "tcsh.exe": {}, "yash": {}, "yash.exe": {}, "cmd": {}, "cmd.exe": {}, "command.com": {}, "powershell": {}, "powershell.exe": {}, "pwsh": {}, "pwsh.exe": {},
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
