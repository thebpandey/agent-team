// Package preparation inspects optional executables and performs explicitly
// approved, fixed dependency installs. Executable availability does not imply
// tracker readiness, MCP registration, or successful workload qualification.
package preparation

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
)

type Dependency struct {
	Name          string   `json:"name"`
	Status        string   `json:"status"`
	Available     bool     `json:"available"`
	Prepared      bool     `json:"prepared"`
	Path          string   `json:"path,omitempty"`
	Version       string   `json:"version,omitempty"`
	Source        string   `json:"source"`
	Scope         string   `json:"scope,omitempty"`
	Prerequisites []string `json:"prerequisites,omitempty"`
	Guidance      string   `json:"guidance"`
	Fallback      string   `json:"fallback"`
	Error         string   `json:"error,omitempty"`
}

// All returns the accepted catalog names. An empty selection inspects this list.
func All() []string { return []string{"beads", "serena", "graphify", "rg", "ast-grep", "lean-ctx"} }

// DefaultNames excludes Beads: selecting a task tracker is a caller decision.
func DefaultNames() []string { return []string{"serena", "graphify", "rg", "ast-grep", "lean-ctx"} }

func Inspect(ctx context.Context, root string, names []string) ([]Dependency, error) {
	return prepare(ctx, root, names, false, false, nativeRunner{})
}

func Install(ctx context.Context, root string, names []string, approved bool) ([]Dependency, error) {
	return prepare(ctx, root, names, true, approved, nativeRunner{})
}

type recipe struct {
	name, binary, source, manager, pkg, fallback string
	prerequisites                                []string
}

// Fixed package identities and direct dependency pins verified against upstream
// installation docs and the official release registries. See SOURCES.md.
var catalog = []recipe{
	{"beads", "bd", "https://github.com/gastownhall/beads", "release", "v1.3.0", "Select the native Markdown tracker explicitly if Beads is unavailable.", []string{"HTTPS access to official GitHub release assets"}},
	{"serena", "serena", "https://github.com/oraios/serena", "uv", "serena-agent==1.7.0", "Use native file reads, Git, and text search for navigation.", []string{"uv (reused or installed in this project after approval)", "Python 3.13 (reused or downloaded into this project after approval)"}},
	{"graphify", "graphify", "https://github.com/Graphify-Labs/graphify", "uv", "graphifyy==0.9.65", "Use native file reads and Git to inspect repository structure.", []string{"uv (reused or installed in this project after approval)", "Python 3.13 (reused or downloaded into this project after approval)"}},
	{"rg", "rg", "https://github.com/BurntSushi/ripgrep", "release", "15.2.0", "Use native file reads and Git search.", []string{"HTTPS access to official GitHub release assets"}},
	{"ast-grep", "ast-grep", "https://github.com/ast-grep/ast-grep", "release", "0.45.3", "Use native text search and verify edits with diffs and tests.", []string{"HTTPS access to official GitHub release assets"}},
	{"lean-ctx", "lean-ctx", "https://github.com/yvgude/lean-ctx", "release", "v3.10.2", "Use bounded native reads and shell output.", []string{"HTTPS access to official GitHub release assets"}},
}

type invocation struct {
	Path        string
	Args        []string
	Dir         string
	Env         map[string]string
	OutputLimit int
}
type runner interface {
	fetcher
	lookPath(string) (string, error)
	execute(context.Context, invocation) (string, error)
}

func prepare(ctx context.Context, root string, names []string, install, approved bool, run runner) ([]Dependency, error) {
	if ctx == nil {
		return nil, errors.New("nil preparation context")
	}
	if strings.TrimSpace(root) == "" {
		return nil, errors.New("project root is required")
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(root)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, errors.New("project root is not a directory")
	}
	if len(names) == 0 {
		names = All()
	}
	selected := make([]recipe, 0, len(names))
	seen := map[string]bool{}
	for _, name := range names {
		name = strings.ToLower(strings.TrimSpace(name))
		if name == "bd" {
			name = "beads"
		}
		if name == "leanctx" {
			name = "lean-ctx"
		}
		if name == "ripgrep" {
			name = "rg"
		}
		found := false
		for _, r := range catalog {
			if r.name == name {
				found = true
				if !seen[name] {
					selected = append(selected, r)
					seen[name] = true
				}
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("unknown dependency %q; choose %s", name, strings.Join(All(), ", "))
		}
	}
	out := make([]Dependency, 0, len(selected))
	for _, r := range selected {
		if err := ctx.Err(); err != nil {
			return out, err
		}
		d := inspect(ctx, root, r, run)
		if !install || d.Available || d.Status == "failed" {
			out = append(out, d)
			continue
		}
		if !approved {
			d.Status = "needs_consent"
			out = append(out, d)
			continue
		}
		if r.manager == "" {
			d.Status = "failed"
			d.Error = "No automatic install recipe is enabled for this dependency."
			out = append(out, d)
			continue
		}
		base := filepath.Join(root, ".agent-team", "dependencies")
		if err := ensureDirectory(root, filepath.Join(base, "bin")); err != nil {
			d.Status = "failed"
			d.Error = err.Error()
			out = append(out, d)
			continue
		}
		// A project lock prevents concurrent preparations from racing tool installers.
		lockPath := filepath.Join(base, ".install-lock")
		lock, err := os.OpenFile(lockPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			d.Status = "failed"
			d.Error = "Dependency preparation is locked; check " + lockPath + ": " + err.Error()
			out = append(out, d)
			continue
		}
		_ = lock.Close()
		callCtx, cancel := context.WithTimeout(ctx, 15*time.Minute)
		var installErr error
		if r.manager == "release" {
			installErr = installReleaseTool(callCtx, r.name, filepath.Join(base, "bin", executable(r.binary)), runtime.GOOS, runtime.GOARCH, run)
		} else {
			var manager string
			manager, installErr = resolveUV(callCtx, base, run)
			if installErr == nil {
				_, installErr = run.execute(callCtx, installInvocation(base, manager, r))
			}
		}
		cancel()
		_ = os.Remove(lockPath)
		if installErr != nil {
			d.Status = "failed"
			d.Error = "Installation failed: " + installErr.Error()
			out = append(out, d)
			continue
		}
		d = inspect(ctx, root, r, run)
		if !d.Available {
			d.Status = "failed"
			if d.Error == "" {
				d.Error = "Installer completed but the executable is unavailable."
			}
		} else {
			d.Status = "installed"
		}
		out = append(out, d)
	}
	return out, nil
}

// Called only after consent and while holding the installation lock. Reuse a
// project uv first, then PATH; bootstrap verified native bytes only if absent.
func resolveUV(ctx context.Context, base string, run runner) (string, error) {
	local := filepath.Join(base, "bin", executable("uv"))
	if _, err := os.Lstat(local); os.IsNotExist(err) {
		if existing, err := run.lookPath("uv"); err == nil {
			return existing, nil
		}
		if err := installReleaseTool(ctx, "uv", local, runtime.GOOS, runtime.GOARCH, run); err != nil {
			return "", fmt.Errorf("prepare project uv: %w", err)
		}
	} else if err != nil {
		return "", err
	}
	probeCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	version, err := run.execute(probeCtx, invocation{Path: local, Args: []string{"--version"}, Dir: base})
	if err != nil {
		return "", fmt.Errorf("project uv probe failed: %w", err)
	}
	if !strings.HasPrefix(strings.TrimSpace(version), "uv ") {
		return "", errors.New("project uv returned no recognizable version evidence")
	}
	return local, nil
}

func inspect(ctx context.Context, root string, r recipe, run runner) Dependency {
	d := Dependency{Name: r.name, Status: "missing", Source: r.source, Prerequisites: append([]string(nil), r.prerequisites...), Fallback: r.fallback}
	d.Guidance = "Install the CLI using " + r.source + " and rerun setup; " + r.fallback
	if r.manager != "" {
		d.Guidance = "Run agent-teamctl setup --install " + r.name + " --approve to prepare the pinned CLI in this project. Requires " + strings.Join(r.prerequisites, ", ") + ". " + r.fallback
	}
	local := filepath.Join(root, ".agent-team", "dependencies", "bin", executable(r.binary))
	if _, err := os.Lstat(local); err == nil {
		d.Path = local
		d.Scope = "project"
	} else if !os.IsNotExist(err) {
		d.Status = "failed"
		d.Error = err.Error()
		return d
	} else {
		path, err := run.lookPath(r.binary)
		if err != nil {
			return d
		}
		d.Path, err = filepath.Abs(path)
		if err != nil {
			d.Status = "failed"
			d.Error = err.Error()
			return d
		}
		d.Scope = "existing"
	}
	args := []string{"--version"}
	if r.name == "beads" {
		args = []string{"version"}
	}
	probeCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	version, err := run.execute(probeCtx, invocation{Path: d.Path, Args: args, Dir: root})
	if err != nil {
		d.Status = "failed"
		d.Error = "Executable probe failed: " + err.Error()
		d.Guidance = "Repair the existing executable at " + d.Path + " using " + r.source + "; then rerun setup. " + r.fallback
		return d
	}
	version = strings.TrimSpace(version)
	if version == "" {
		d.Status = "failed"
		d.Error = "Executable returned no version evidence."
		return d
	}
	// Limit status output even when a CLI prints a verbose build description.
	if len(version) > 512 {
		version = version[:512]
	}
	d.Version = version
	d.Available = true
	d.Status = "available"
	d.Guidance = "CLI available at " + d.Path + "; workload readiness and any host integration must be qualified separately."
	return d
}

func installInvocation(base, manager string, r recipe) invocation {
	call := invocation{Path: manager, Dir: base, Env: map[string]string{}}
	bin := filepath.Join(base, "bin")
	call.Args = []string{"tool", "install", "--no-config", "--python", "3.13", r.pkg}
	call.Env = map[string]string{
		"UV_TOOL_DIR": filepath.Join(base, "uv-tools"), "UV_TOOL_BIN_DIR": bin, "UV_CACHE_DIR": filepath.Join(base, "uv-cache"),
		"UV_PYTHON_DOWNLOADS": "automatic", "UV_PYTHON_INSTALL_DIR": filepath.Join(base, "python"), "UV_PYTHON_BIN_DIR": filepath.Join(base, "python-bin"),
		"UV_PYTHON_INSTALL_BIN": "false", "UV_PYTHON_INSTALL_REGISTRY": "false", "UV_NO_MODIFY_PATH": "1", "UV_NO_PROGRESS": "1",
	}
	return call
}

// Reject redirected directories before any writes. uv's own executable links
// remain valid; directory symlinks must never redirect package installation.
func ensureDirectory(root, target string) error {
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return err
	}
	p := root
	for _, part := range strings.Split(rel, string(os.PathSeparator)) {
		p = filepath.Join(p, part)
		info, err := os.Lstat(p)
		if os.IsNotExist(err) {
			if err = os.Mkdir(p, 0700); err != nil {
				return err
			}
			continue
		}
		if err != nil {
			return err
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("install directory must be an ordinary directory: %s", p)
		}
	}
	// Inspect all existing descendants, including cache/tool folders, so an
	// existing redirected subtree cannot send an approved install elsewhere.
	base := filepath.Dir(target)
	return filepath.WalkDir(base, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Type()&os.ModeSymlink != 0 {
			resolved, err := filepath.EvalSymlinks(path)
			if err != nil {
				return err
			}
			// uv virtual environments link to an existing Python interpreter.
			// Permit that specific file link without permitting redirected folders.
			name := filepath.Base(path)
			toolRel, relErr := filepath.Rel(filepath.Join(base, "uv-tools"), path)
			parts := strings.Split(toolRel, string(os.PathSeparator))
			if relErr == nil && len(parts) == 3 && parts[0] != ".." && parts[1] == "bin" && (name == "python" || name == "python3" || name == "python3.13") {
				if info, statErr := os.Stat(resolved); statErr == nil && info.Mode().IsRegular() {
					return nil
				}
			}
			r, err := filepath.Rel(base, resolved)
			if err != nil || r == ".." || strings.HasPrefix(r, ".."+string(os.PathSeparator)) {
				return fmt.Errorf("install subtree links outside project dependencies: %s", path)
			}
		}
		return nil
	})
}

func executable(name string) string {
	if runtime.GOOS == "windows" {
		return name + ".exe"
	}
	return name
}

type nativeRunner struct{ client *http.Client }

func (nativeRunner) lookPath(name string) (string, error) { return exec.LookPath(name) }
func (nativeRunner) execute(ctx context.Context, c invocation) (string, error) {
	cmd := exec.CommandContext(ctx, c.Path, c.Args...)
	cmd.Dir = c.Dir
	cmd.Env = os.Environ()
	for key, value := range c.Env {
		for i := len(cmd.Env) - 1; i >= 0; i-- {
			if strings.HasPrefix(cmd.Env[i], key+"=") {
				cmd.Env = append(cmd.Env[:i], cmd.Env[i+1:]...)
			}
		}
		cmd.Env = append(cmd.Env, key+"="+value)
	}
	output := &limitedOutput{limit: c.OutputLimit}
	cmd.Stdout = output
	cmd.Stderr = output
	cmd.WaitDelay = 2 * time.Second
	err := cmd.Run()
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	if err != nil {
		if diagnostic := commandDiagnostic(output.text(), cmd.Env); diagnostic != "" {
			return "", fmt.Errorf("%w: %s", err, diagnostic)
		}
		return "", err
	}
	if c.OutputLimit > 0 && output.truncated {
		return "", errors.New("command output exceeds preparation limit")
	}
	return output.text(), nil
}

var diagnosticCredentials = regexp.MustCompile(`(?i)(https?://)[^\s/@]+:[^\s/@]*@`)

// Preserve a bounded actionable error without echoing environment credentials.
func commandDiagnostic(output string, env []string) string {
	var secrets []string
	for _, entry := range env {
		name, value, ok := strings.Cut(entry, "=")
		name = strings.ToUpper(name)
		if ok && len(value) >= 4 && (strings.Contains(name, "TOKEN") || strings.Contains(name, "SECRET") || strings.Contains(name, "PASSWORD") || strings.Contains(name, "CREDENTIAL") || strings.Contains(name, "KEY")) {
			secrets = append(secrets, value)
		}
	}
	sort.Slice(secrets, func(i, j int) bool { return len(secrets[i]) > len(secrets[j]) })
	for _, secret := range secrets {
		output = strings.ReplaceAll(output, secret, "[redacted]")
	}
	output = diagnosticCredentials.ReplaceAllString(output, "${1}[redacted]@")
	output = strings.TrimSpace(output)
	if len(output) > 1024 {
		output = output[:1024] + "..."
	}
	return output
}

type limitedOutput struct {
	mu        sync.Mutex
	data      []byte
	limit     int
	truncated bool
}

func (w *limitedOutput) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	n := len(p)
	limit := w.limit
	if limit <= 0 {
		limit = 4096
	}
	remaining := limit - len(w.data)
	if len(p) > remaining {
		w.truncated = true
	}
	if remaining > 0 {
		if len(p) > remaining {
			p = p[:remaining]
		}
		w.data = append(w.data, p...)
	}
	return n, nil
}
func (w *limitedOutput) text() string { w.mu.Lock(); defer w.mu.Unlock(); return string(w.data) }
