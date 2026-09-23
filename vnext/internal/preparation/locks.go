package preparation

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/thebpandey/agent-team/vnext/internal/store"
)

func preparationLockRoot(root string) string {
	return filepath.Join(root, ".agent-team", "dependencies", "coordination")
}

// Preparation has its own store guard so callers may already hold the project
// setup/settings guard. All dependency installation and initialization shares
// this guard. Legacy empty .install-lock/.graphify-lock markers are preserved;
// they contain no owner identity and cannot authorize recovery or future locks.
func acquirePreparationLock(ctx context.Context, root, operation string) (*store.MutationGuard, error) {
	return acquirePreparationLockWithLiveness(ctx, root, operation, store.NativeLiveness{})
}

func acquirePreparationLockWithLiveness(ctx context.Context, root, operation string, proof store.HolderLiveness) (*store.MutationGuard, error) {
	if ctx == nil {
		return nil, errors.New("nil preparation lock context")
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	lockRoot := preparationLockRoot(root)
	if err := ensureDirectory(root, filepath.Join(lockRoot, ".agent-team")); err != nil {
		return nil, err
	}
	for {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("waiting for another dependency preparation: %w", err)
		}
		// An abandoned recovery claim must be recovered before its primary.
		busy, err := recoverPreparationOwner(ctx, lockRoot, store.MutationLockRecovery, proof)
		if err != nil {
			return nil, err
		}
		if !busy {
			busy, err = recoverPreparationOwner(ctx, lockRoot, store.MutationLockPrimary, proof)
			if err != nil {
				return nil, err
			}
		}
		if !busy {
			// Another process may win after the inspection. Retry with liveness
			// checks instead of waiting forever behind an owner that later exits.
			attempt, stop := context.WithTimeout(ctx, 100*time.Millisecond)
			guard, acquireErr := store.AcquireProjectMutation(attempt, lockRoot, "preparation", operation)
			stop()
			if acquireErr == nil {
				return guard, nil
			}
			if !errors.Is(acquireErr, context.DeadlineExceeded) && !errors.Is(acquireErr, context.Canceled) {
				return nil, acquireErr
			}
		}
		timer := time.NewTimer(25 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, fmt.Errorf("waiting for another dependency preparation: %w", ctx.Err())
		case <-timer.C:
		}
	}
}

func recoverPreparationOwner(ctx context.Context, root, target string, proof store.HolderLiveness) (bool, error) {
	filename := "mutation.lock"
	if target == store.MutationLockRecovery {
		filename = "mutation.recovery"
	}
	path := filepath.Join(root, ".agent-team", filename)
	if _, err := os.Lstat(path); errors.Is(err, os.ErrNotExist) {
		return false, nil
	} else if err != nil {
		return false, err
	}
	owner, err := store.MutationLockOwner(root, target)
	if err != nil {
		// Release may have completed between inspection and reading.
		if _, statErr := os.Lstat(path); errors.Is(statErr, os.ErrNotExist) {
			return false, nil
		}
		return false, fmt.Errorf("unrecognized preparation coordination record at %s; preserving it: %w", path, err)
	}
	dead, err := proof.HolderDead(ctx, owner)
	if err != nil {
		return false, fmt.Errorf("cannot verify preparation owner at %s; preserving it: %w", path, err)
	}
	if !dead {
		return true, nil
	}
	_, err = store.RecoverProjectMutation(ctx, root, store.MutationRecoveryRequest{Target: target, Token: owner.Token, OperationID: owner.OperationID}, proof)
	if err != nil {
		// A competing recovery or replacement is resolved by a fresh exact
		// owner read on the next iteration, never by removing a guessed PID.
		return true, nil
	}
	return false, nil
}
