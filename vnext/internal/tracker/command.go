package tracker

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"sync"
)

const maxCommandStreamBytes = 1 << 20

// CommandResult keeps bounded command output separate from process exit and
// transport failures. Exit is meaningful only when Transport is nil.
type CommandResult struct {
	Stdout    []byte
	Stderr    []byte
	Exit      int
	TimedOut  bool
	Transport error
}

// CommandRunner runs an executable and its arguments without involving a
// shell. Implementations must return output bounded to command-log capacity.
type CommandRunner interface {
	Run(context.Context, string, ...string) CommandResult
}

// FakeRunner is deterministic and is intended for adapter tests only.
type FakeRunner struct {
	Result CommandResult
}

func (f *FakeRunner) Run(context.Context, string, ...string) CommandResult {
	return CommandResult{
		Stdout:    append([]byte(nil), f.Result.Stdout...),
		Stderr:    append([]byte(nil), f.Result.Stderr...),
		Exit:      f.Result.Exit,
		TimedOut:  f.Result.TimedOut,
		Transport: f.Result.Transport,
	}
}

// NewFakeRunner returns a runner with one fixed result.
func NewFakeRunner(result CommandResult) CommandRunner { return &FakeRunner{Result: result} }

type commandRunner struct{}

// NewCommandRunner exposes the native bounded command runner for project
// adapters that must use the same no-shell execution contract as Beads.
func NewCommandRunner() CommandRunner { return commandRunner{} }

func (commandRunner) Run(ctx context.Context, name string, args ...string) CommandResult {
	resolved, err := resolveExecutable(name)
	if err != nil {
		return CommandResult{Transport: err}
	}
	cmd := exec.CommandContext(ctx, resolved, args...)
	stdout := &boundedCapture{limit: maxCommandStreamBytes}
	stderr := &boundedCapture{limit: maxCommandStreamBytes}
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	err = cmd.Run()
	result := CommandResult{Stdout: stdout.Bytes(), Stderr: stderr.Bytes()}
	if ctx.Err() != nil {
		result.TimedOut = true
		return result
	}
	if err == nil {
		return result
	}
	var exitError *exec.ExitError
	if errors.As(err, &exitError) {
		result.Exit = exitError.ExitCode()
		return result
	}
	result.Transport = err
	return result
}

// ResolveExecutable accepts the Beads executable spellings used by Unix and
// Windows installations. For bare bd it probes the supported Windows suffixes
// as a fallback, while preserving the OS's normal executable lookup rules.
func ResolveExecutable(name string) (string, error) { return resolveExecutable(name) }

func resolveExecutable(name string) (string, error) {
	if name == "" {
		return "", fmt.Errorf("empty executable")
	}
	candidates := []string{name}
	if filepath.Base(name) == "bd" {
		candidates = append(candidates, name+".exe", name+".cmd")
	}
	var last error
	for _, candidate := range candidates {
		path, err := exec.LookPath(candidate)
		if err == nil {
			return path, nil
		}
		last = err
	}
	return "", last
}

type boundedCapture struct {
	mu    sync.Mutex
	buf   bytes.Buffer
	limit int
}

func (b *boundedCapture) Write(value []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	written := len(value)
	remaining := b.limit - b.buf.Len()
	if remaining > 0 {
		if len(value) > remaining {
			value = value[:remaining]
		}
		_, _ = b.buf.Write(value)
	}
	return written, nil
}

func (b *boundedCapture) Bytes() []byte {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]byte(nil), b.buf.Bytes()...)
}
