package store

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/thebpandey/agent-team/vnext/internal/core"
)

const projectMutationLock = ".agent-team/mutation.lock"

// AcquireProjectMutation serializes filesystem and durable-state mutations for
// one canonical project. Callers must re-read their authority after acquiring.
func AcquireProjectMutation(ctx context.Context, root string) (func() error, error) {
	if ctx == nil || root == "" {
		return nil, core.ErrPath
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return nil, fmt.Errorf("%w: project root: %v", core.ErrPath, err)
	}
	canonical, err := filepath.EvalSymlinks(root)
	if err != nil {
		return nil, fmt.Errorf("%w: canonical project root: %v", core.ErrPath, err)
	}
	canonical, err = filepath.Abs(canonical)
	if err != nil {
		return nil, fmt.Errorf("%w: absolute project root: %v", core.ErrPath, err)
	}
	parent := filepath.Join(canonical, ".agent-team")
	if err := os.MkdirAll(parent, 0o700); err != nil {
		return nil, fmt.Errorf("%w: mutation guard parent: %v", core.ErrPath, err)
	}
	lock := filepath.Join(canonical, filepath.FromSlash(projectMutationLock))
	for {
		if err := os.Mkdir(lock, 0o700); err == nil {
			return func() error { return os.Remove(lock) }, nil
		} else if !errors.Is(err, os.ErrExist) {
			return nil, fmt.Errorf("%w: mutation guard: %v", core.ErrPath, err)
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(2 * time.Millisecond):
		}
	}
}
