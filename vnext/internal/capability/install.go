package capability

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/thebpandey/agent-team/vnext/internal/core"
)

// Install executes a pre-validated, explicitly consented argv and then a
// separate direct probe. It never invokes a shell or a host-vendor installer.
func Install(ctx context.Context, runner NativeRunner, plan InstallPlan) (Probe, error) {
	if ctx == nil || runner == nil {
		return Probe{}, fmt.Errorf("install requires context and runner")
	}
	if err := validatePlan(plan); err != nil {
		return Probe{}, err
	}
	if result := runBounded(ctx, runner, plan.Command); failed(result) {
		return Probe{}, fmt.Errorf("capability install failed: %s", commandReason(result))
	}
	result := runBounded(ctx, runner, plan.ProbeCommand)
	if failed(result) {
		return Probe{}, fmt.Errorf("capability post-install probe failed: %s", commandReason(result))
	}
	version := boundedText(result.Stdout)
	if version == "" {
		return Probe{}, fmt.Errorf("capability post-install probe returned no version")
	}
	if !strings.Contains(version, plan.VerifiedVersion) {
		return Probe{}, fmt.Errorf("capability post-install version does not match plan")
	}
	return Probe{Name: plan.Name, Mode: modeFor(plan.Name), Path: plan.ProbeCommand[0], Version: version, Available: true, Healthy: true}, nil
}

// Rollback runs only each plan-owned, explicit rollback argv. It fails before
// execution if a previous-state authority or a matching owned path is absent.
func Rollback(ctx context.Context, runner NativeRunner, plan InstallPlan) error {
	if ctx == nil || runner == nil {
		return fmt.Errorf("rollback requires context and runner")
	}
	if err := validatePlan(plan); err != nil {
		return err
	}
	for _, entry := range plan.Rollback {
		if len(entry.Command) == 0 {
			return fmt.Errorf("rollback for %q has no explicit argv", entry.Path)
		}
		if result := runBounded(ctx, runner, entry.Command); failed(result) {
			return fmt.Errorf("rollback for %q failed: %s", entry.Path, commandReason(result))
		}
	}
	return nil
}

func runBounded(ctx context.Context, runner NativeRunner, argv []string) NativeResult {
	callCtx, cancel := context.WithTimeout(ctx, operationTimeout)
	defer cancel()
	return runner.Run(callCtx, argv[0], argv[1:]...)
}

func failed(result NativeResult) bool {
	return result.Transport != nil || result.TimedOut || result.Exit != 0
}

func validatePlan(plan InstallPlan) error {
	if !known(plan.Name) || plan.Name == Native || !plan.Explicit || !trustedSource(plan.Source) || strings.TrimSpace(plan.Package) == "" || strings.TrimSpace(plan.VerifiedVersion) == "" || plan.Scope != "project" {
		return fmt.Errorf("unsafe install plan")
	}
	// Capability adapters never alter ambient host settings.
	if len(plan.SettingsChanged) != 0 {
		return fmt.Errorf("install plan changes settings")
	}
	if err := validateArgv(plan.Command); err != nil {
		return fmt.Errorf("install command: %w", err)
	}
	if plan.Command[0] != string(plan.Name)+"-install" {
		return fmt.Errorf("untrusted install command")
	}
	if err := validateArgv(plan.ProbeCommand); err != nil {
		return fmt.Errorf("probe command: %w", err)
	}
	if plan.ProbeCommand[0] != string(plan.Name) {
		return fmt.Errorf("untrusted probe command")
	}
	owned := make(map[string]OwnedFile, len(plan.OwnedFiles))
	for _, file := range plan.OwnedFiles {
		if !safePath(file.Path) || file.Role == "" || file.SHA256 == "" {
			return fmt.Errorf("unsafe owned file")
		}
		if _, exists := owned[file.Path]; exists {
			return fmt.Errorf("duplicate owned file %q", file.Path)
		}
		owned[file.Path] = file
	}
	if len(owned) == 0 || len(plan.Rollback) != len(owned) {
		return fmt.Errorf("incomplete rollback manifest")
	}
	seen := make(map[string]struct{}, len(plan.Rollback))
	for _, entry := range plan.Rollback {
		if !safePath(entry.Path) || entry.SHA256 == "" || (entry.Backup == "" && len(entry.Command) == 0) {
			return fmt.Errorf("rollback entry lacks prior-state authority")
		}
		if _, ok := owned[entry.Path]; !ok {
			return fmt.Errorf("rollback path %q is not owned", entry.Path)
		}
		if _, exists := seen[entry.Path]; exists {
			return fmt.Errorf("duplicate rollback entry %q", entry.Path)
		}
		if len(entry.Command) == 0 {
			return fmt.Errorf("rollback for %q has no executable argv", entry.Path)
		}
		if err := validateArgv(entry.Command); err != nil {
			return fmt.Errorf("rollback command: %w", err)
		}
		if entry.Command[0] != string(plan.Name)+"-rollback" {
			return fmt.Errorf("untrusted rollback command")
		}
		seen[entry.Path] = struct{}{}
	}
	return nil
}

func validateArgv(argv []string) error {
	if len(argv) == 0 || strings.TrimSpace(argv[0]) == "" || len(argv) > 32 || filepath.IsAbs(argv[0]) || filepath.Dir(argv[0]) != "." {
		return fmt.Errorf("invalid argv")
	}
	for _, arg := range argv {
		if arg == "" || strings.IndexByte(arg, 0) >= 0 || unsafePortablePath(arg) {
			return fmt.Errorf("invalid argument")
		}
	}
	switch strings.ToLower(filepath.Base(argv[0])) {
	case "sh", "bash", "zsh", "fish", "cmd", "cmd.exe", "powershell", "powershell.exe", "pwsh", "pwsh.exe":
		return fmt.Errorf("shell execution is forbidden")
	}
	var bytes int64
	for _, arg := range argv {
		bytes += int64(len(arg))
	}
	if bytes > core.DefaultConfig().Storage.ArgumentBytes {
		return fmt.Errorf("argv exceeds limit")
	}
	return nil
}

func safePath(path string) bool {
	if path == "" || filepath.IsAbs(path) || filepath.Clean(path) != path || unsafePortablePath(path) {
		return false
	}
	return path != "." && path != ".." && !strings.HasPrefix(path, ".."+string(filepath.Separator))
}

func unsafePortablePath(path string) bool {
	if strings.HasPrefix(path, "/") || strings.HasPrefix(path, `\\`) || strings.Contains(path, "../") || strings.Contains(path, `..\\`) {
		return true
	}
	return len(path) > 2 && path[1] == ':' && ((path[0] >= 'a' && path[0] <= 'z') || (path[0] >= 'A' && path[0] <= 'Z')) && (path[2] == '/' || path[2] == '\\')
}
