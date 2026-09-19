// Package command runs foreground commands without a shell.
package command

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"sync"

	"github.com/thebpandey/agent-team/vnext/internal/tracker"
)

const defaultLimit = 1 << 20

// Runner captures each command stream independently up to Limit bytes.
type Runner struct {
	Limit int
}

// NewRunner returns a foreground runner with an independent cap per stream.
func NewRunner(limit int) Runner { return Runner{Limit: limit} }

// Run executes name and args directly, preserving command exit failures apart
// from transport failures. A deadline expiry is reported as TimedOut.
func (r Runner) Run(ctx context.Context, name string, args ...string) tracker.CommandResult {
	if name == "" {
		return tracker.CommandResult{Exit: -1, Transport: errors.New("empty executable")}
	}
	if ctx == nil {
		return tracker.CommandResult{Exit: -1, Transport: errors.New("nil context")}
	}
	if err := ctx.Err(); err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return tracker.CommandResult{Exit: -1, TimedOut: true}
		}
		return tracker.CommandResult{Exit: -1, Transport: err}
	}

	limit := r.Limit
	if limit < 1 {
		limit = defaultLimit
	}
	command := exec.CommandContext(ctx, name, args...)
	stdout := &boundedWriter{limit: limit}
	stderr := &boundedWriter{limit: limit}
	command.Stdout = stdout
	command.Stderr = stderr
	err := command.Run()
	result := tracker.CommandResult{Stdout: stdout.Bytes(), Stderr: stderr.Bytes()}
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		result.Exit = -1
		result.TimedOut = true
		return result
	}
	if err := ctx.Err(); err != nil {
		result.Exit = -1
		result.Transport = err
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
	result.Exit = -1
	result.Transport = fmt.Errorf("command %q: %w", name, err)
	return result
}

type boundedWriter struct {
	mu    sync.Mutex
	buf   bytes.Buffer
	limit int
}

func (w *boundedWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	written := len(p)
	remaining := w.limit - w.buf.Len()
	if remaining > 0 {
		if len(p) > remaining {
			p = p[:remaining]
		}
		_, _ = w.buf.Write(p)
	}
	return written, nil
}

func (w *boundedWriter) Bytes() []byte {
	w.mu.Lock()
	defer w.mu.Unlock()
	return append([]byte(nil), w.buf.Bytes()...)
}
