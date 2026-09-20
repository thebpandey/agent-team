package capability

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/thebpandey/agent-team/vnext/internal/core"
	"github.com/thebpandey/agent-team/vnext/internal/tracker"
)

// Install captures the exact prior files before invoking the consent-bound
// action. Any action or verification failure immediately restores that state.
func Install(ctx context.Context, runner tracker.CommandRunner, plan InstallPlan) (Probe, error) {
	if ctx == nil || runner == nil || plan.state == nil {
		return Probe{}, fmt.Errorf("install requires context, runner, and plan")
	}
	state := plan.state
	state.mu.Lock()
	defer state.mu.Unlock()
	if err := validateState(state); err != nil {
		return Probe{}, err
	}
	if len(state.backups) != 0 {
		return Probe{}, fmt.Errorf("install plan already used")
	}
	backups, err := capture(state)
	if err != nil {
		return Probe{}, err
	}
	state.backups = backups
	if result := run(ctx, runner, state.install); failed(result) {
		return Probe{}, failAndRestore(runner, state, "install action failed")
	}
	if err := verifyOwned(state); err != nil {
		return Probe{}, failAndRestore(runner, state, "installed files failed verification")
	}
	result := run(ctx, runner, state.probe)
	if failed(result) || boundedText(result.Stdout) != state.version {
		return Probe{}, failAndRestore(runner, state, "post-install probe failed")
	}
	return Probe{Name: state.name, Mode: modeFor(state.name), Path: state.probe[0], Source: state.source, Version: state.version, Available: true, Healthy: true}, nil
}

// Rollback executes the consent-bound action, restores captured bytes, and
// verifies that every managed path exactly matches its pre-install state.
func Rollback(ctx context.Context, runner tracker.CommandRunner, plan InstallPlan) error {
	if ctx == nil || runner == nil || plan.state == nil {
		return fmt.Errorf("rollback requires context, runner, and plan")
	}
	state := plan.state
	state.mu.Lock()
	defer state.mu.Unlock()
	if err := validateState(state); err != nil {
		return err
	}
	return rollbackLocked(ctx, runner, state)
}

func failAndRestore(runner tracker.CommandRunner, state *planState, reason string) error {
	ctx, cancel := context.WithTimeout(context.Background(), operationTimeout)
	defer cancel()
	if err := rollbackLocked(ctx, runner, state); err != nil {
		return fmt.Errorf("%s; rollback failed: %w", reason, err)
	}
	return fmt.Errorf("%s; rollback restored prior state", reason)
}

func rollbackLocked(ctx context.Context, runner tracker.CommandRunner, state *planState) error {
	if len(state.backups) != len(state.owned) {
		return fmt.Errorf("rollback has no verified backup")
	}
	if result := run(ctx, runner, state.rollback); failed(result) {
		return fmt.Errorf("rollback action failed: %s", commandReason(result))
	}
	for _, backup := range state.backups {
		if backup.exists {
			if err := os.MkdirAll(filepath.Dir(backup.path), 0o700); err != nil {
				return err
			}
			if err := os.WriteFile(backup.path, backup.bytes, os.FileMode(backup.mode)); err != nil {
				return err
			}
		} else if err := os.Remove(backup.path); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	if err := verifyBackups(state.backups); err != nil {
		return err
	}
	return nil
}

func capture(state *planState) ([]backup, error) {
	backups := make([]backup, 0, len(state.owned))
	for _, owned := range state.owned {
		path := filepath.Join(state.root, owned.Path)
		if filepath.Dir(path) == "" || !within(state.root, path) {
			return nil, fmt.Errorf("unsafe owned path")
		}
		info, err := os.Stat(path)
		if os.IsNotExist(err) {
			backups = append(backups, backup{path: path})
			continue
		}
		if err != nil || !info.Mode().IsRegular() || info.Size() > core.DefaultConfig().Storage.CanonicalBytes {
			return nil, fmt.Errorf("cannot capture prior state")
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		backups = append(backups, backup{path: path, exists: true, bytes: data, hash: hash(data), mode: uint32(info.Mode().Perm())})
	}
	return backups, nil
}

func verifyOwned(state *planState) error {
	for _, owned := range state.owned {
		data, err := readRegular(filepath.Join(state.root, owned.Path))
		if err != nil || hash(data) != owned.SHA256 {
			return fmt.Errorf("owned file verification failed")
		}
	}
	return nil
}

func verifyBackups(backups []backup) error {
	for _, backup := range backups {
		data, err := readRegular(backup.path)
		if !backup.exists {
			if os.IsNotExist(err) {
				continue
			}
			return fmt.Errorf("absent backup was not restored")
		}
		if err != nil || hash(data) != backup.hash {
			return fmt.Errorf("backup restoration verification failed")
		}
	}
	return nil
}

func readRegular(path string) ([]byte, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > core.DefaultConfig().Storage.CanonicalBytes {
		return nil, fmt.Errorf("not a bounded regular file")
	}
	return os.ReadFile(path)
}

func validateState(state *planState) error {
	if state == nil || !known(state.name) || state.name == Native || !verified(state.source) || state.version == "" || packageVersion(state.pkg) != state.version {
		return fmt.Errorf("invalid consent-bound plan")
	}
	if err := validateAction("install", state.install, state.root, state.pkg, state.version); err != nil {
		return err
	}
	if err := validateAction("probe", state.probe, state.root, state.pkg, state.version); err != nil {
		return err
	}
	if err := validateAction("rollback", state.rollback, state.root, state.pkg, state.version); err != nil {
		return err
	}
	for _, file := range state.owned {
		if !validOwned(file) {
			return fmt.Errorf("invalid consent-bound owned file")
		}
	}
	return nil
}

func run(ctx context.Context, runner tracker.CommandRunner, argv []string) NativeResult {
	callCtx, cancel := context.WithTimeout(ctx, operationTimeout)
	defer cancel()
	return runner.Run(callCtx, argv[0], argv[1:]...)
}

func failed(result NativeResult) bool {
	return result.Transport != nil || result.TimedOut || result.Exit != 0
}

func hash(data []byte) string {
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func within(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}

func unsafePortablePath(path string) bool {
	if path == "" || strings.HasPrefix(path, "/") || strings.HasPrefix(path, `\\`) || strings.Contains(path, "../") || strings.Contains(path, `..\\`) || strings.Contains(path, ".git") || strings.Contains(path, "hooks") {
		return true
	}
	return len(path) > 2 && path[1] == ':' && ((path[0] >= 'a' && path[0] <= 'z') || (path[0] >= 'A' && path[0] <= 'Z')) && (path[2] == '/' || path[2] == '\\')
}
