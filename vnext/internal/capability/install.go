package capability

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/thebpandey/agent-team/vnext/internal/tracker"
)

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
	if result := runner.Run(ctx, append([]string(nil), state.spec.installArgv...), []string{}); failed(result) {
		return Probe{}, failStage(root, state.spec.stage, "install action failed: "+commandReason(result))
	}
	if err := verifyStaged(root, state.spec.stage, state.spec.stageDigest); err != nil {
		return Probe{}, failStage(root, state.spec.stage, "staged artifact verification failed: "+err.Error())
	}
	result := runner.Run(ctx, append([]string(nil), state.spec.probeArgv...), []string{})
	if failed(result) || len(result.Stdout) > probeOutputLimit || strings.TrimSpace(string(result.Stdout)) != state.spec.version {
		reason := "post-install probe failed"
		if failed(result) {
			reason += ": " + commandReason(result)
		}
		return Probe{}, failStage(root, state.spec.stage, reason)
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
	return Probe{Name: state.spec.name, Mode: state.spec.mode, Path: filepath.Join(state.spec.project, state.spec.destination), Version: state.spec.version, Digest: "sha256:" + state.spec.stageDigest, Available: true, Healthy: true}, nil
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
	if len(argv) == 0 || argv[0] == "" {
		return false
	}
	for _, arg := range argv {
		if strings.IndexByte(arg, 0) >= 0 {
			return false
		}
	}
	return true
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
