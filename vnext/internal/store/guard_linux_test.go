//go:build linux

package store

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/thebpandey/agent-team/vnext/internal/core"
)

func TestMutationOwnerFIFOFailsClosedWithoutBlocking(t *testing.T) {
	root := t.TempDir()
	guard, err := AcquireProjectMutation(context.Background(), root, "test", "fifo-owner")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, filepath.FromSlash(projectMutationLock))
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(path, 0o600); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- guard.Release() }()
	select {
	case err := <-done:
		if !errors.Is(err, core.ErrRevision) {
			t.Fatalf("FIFO owner error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("FIFO owner read blocked")
	}
	done = make(chan error, 1)
	go func() {
		_, err := RecoverProjectMutation(context.Background(), root, recoveryRequest(MutationLockPrimary, guard.Owner()), livenessProof{dead: true})
		done <- err
	}()
	select {
	case err := <-done:
		if !errors.Is(err, core.ErrRevision) {
			t.Fatalf("FIFO recovery error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("FIFO recovery read blocked")
	}
}
