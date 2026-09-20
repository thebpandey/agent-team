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
	helperErr := error(nil)
	if result := run(ctx, runner, state.rollback); failed(result) {
		helperErr = fmt.Errorf("rollback action failed: %s", commandReason(result))
	}
	for _, backup := range state.backups {
		if err := noFollowParents(state.root, backup.path); err != nil {
			return err
		}
		if backup.exists {
			if err := replaceLeaf(backup.path, backup.bytes, os.FileMode(backup.mode)); err != nil {
				return err
			}
		} else if err := os.Remove(backup.path); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	if err := verifyBackups(state.backups); err != nil {
		return err
	}
	if helperErr != nil {
		return helperErr
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
		if err := noFollow(state.root, path); err != nil {
			return nil, err
		}
		info, err := os.Lstat(path)
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
		path := filepath.Join(state.root, owned.Path)
		if err := noFollow(state.root, path); err != nil {
			return err
		}
		data, err := readRegular(path)
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
		info, err := os.Lstat(backup.path)
		if err != nil || uint32(info.Mode().Perm()) != backup.mode {
			return fmt.Errorf("backup mode restoration verification failed")
		}
	}
	return nil
}

func readRegular(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > core.DefaultConfig().Storage.CanonicalBytes {
		return nil, fmt.Errorf("not a bounded regular file")
	}
	return os.ReadFile(path)
}

func validateState(state *planState) error {
	if state == nil || !known(state.name) || state.name == Native || !validSource(state.source) || state.version == "" || state.source.Version != state.version || state.binding != planBinding(state) {
		return fmt.Errorf("invalid consent-bound plan")
	}
	if len(state.install) == 0 || len(state.rollback) == 0 || len(state.probe) < 2 || state.probe[0] != filepath.Join(state.root, state.owned[0].Path) {
		return fmt.Errorf("invalid consent-bound actions")
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

func noFollow(root, path string) error {
	if err := noFollowParents(root, path); err != nil {
		return err
	}
	if info, err := os.Lstat(path); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("symlink managed path")
	}
	return nil
}

func noFollowParents(root, path string) error {
	if !within(root, path) {
		return fmt.Errorf("managed root escape")
	}
	for p := filepath.Dir(path); ; p = filepath.Dir(p) {
		if info, err := os.Lstat(p); err == nil && info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("symlink managed path")
		}
		if p == root {
			break
		}
		if p == filepath.Dir(p) {
			return fmt.Errorf("managed root escape")
		}
	}
	return nil
}

func replaceLeaf(path string, data []byte, mode os.FileMode) error {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".agent-team-restore-")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(mode); err == nil {
		_, err = tmp.Write(data)
	}
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return err
	}
	return os.Chmod(path, mode)
}
func planBinding(s *planState) string {
	parts := []string{s.source.Identity, s.source.Digest, s.source.Version, s.root}
	parts = append(parts, s.install...)
	parts = append(parts, s.probe...)
	parts = append(parts, s.rollback...)
	for _, f := range s.owned {
		parts = append(parts, f.Path, string(f.Role), f.SHA256)
	}
	return hash([]byte(strings.Join(parts, "\x00")))
}
