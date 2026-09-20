package store

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/thebpandey/agent-team/vnext/internal/core"
)

type livenessProof struct {
	dead    bool
	err     error
	entered chan<- struct{}
	wait    <-chan struct{}
}

func (p livenessProof) HolderDead(context.Context, MutationOwner) (bool, error) {
	if p.entered != nil {
		close(p.entered)
	}
	if p.wait != nil {
		<-p.wait
	}
	return p.dead, p.err
}

func TestProjectMutationGuardSerializesCanonicalAliasesAndTimesOut(t *testing.T) {
	root := t.TempDir()
	alias := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(root, alias); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	first, err := AcquireProjectMutation(context.Background(), root, "test", "first")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := AcquireProjectMutation(ctx, alias, "test", "waiter"); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("waiter error = %v", err)
	}
	if err := first.Release(); err != nil {
		t.Fatal(err)
	}
}

func TestMutationGuardRecoverySafety(t *testing.T) {
	for name, test := range map[string]func(*testing.T, string, *MutationGuard){
		"live holder": func(t *testing.T, root string, guard *MutationGuard) {
			if err := RecoverProjectMutation(context.Background(), root, guard.Owner(), livenessProof{}); !errors.Is(err, core.ErrRevision) {
				t.Fatal(err)
			}
		},
		"wrong owner": func(t *testing.T, root string, guard *MutationGuard) {
			wrong := guard.Owner()
			wrong.Token = "00000000000000000000000000000000"
			if err := RecoverProjectMutation(context.Background(), root, wrong, livenessProof{dead: true}); !errors.Is(err, core.ErrRevision) {
				t.Fatal(err)
			}
		},
		"unknown liveness": func(t *testing.T, root string, guard *MutationGuard) {
			if err := RecoverProjectMutation(context.Background(), root, guard.Owner(), livenessProof{err: errors.New("unknown")}); !errors.Is(err, core.ErrRevision) {
				t.Fatal(err)
			}
		},
	} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			guard, err := AcquireProjectMutation(context.Background(), root, "install", "op-1")
			if err != nil {
				t.Fatal(err)
			}
			test(t, root, guard)
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
			defer cancel()
			if _, err := AcquireProjectMutation(ctx, root, "test", "blocked"); !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("residue removed: %v", err)
			}
		})
	}
}

func TestMutationGuardRejectsCorruptCrashResidue(t *testing.T) {
	root := t.TempDir()
	lock := filepath.Join(root, filepath.FromSlash(projectMutationLock))
	if err := os.MkdirAll(lock, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(lock, mutationOwnerFile), []byte("not-json"), 0o600); err != nil {
		t.Fatal(err)
	}
	expected := MutationOwner{Token: "00000000000000000000000000000000", Scope: "test", OperationID: "op"}
	if err := RecoverProjectMutation(context.Background(), root, expected, livenessProof{dead: true}); !errors.Is(err, core.ErrRevision) {
		t.Fatal(err)
	}
	if _, err := os.Stat(lock); err != nil {
		t.Fatalf("corrupt residue removed: %v", err)
	}
}

func TestMutationGuardDeadRecoveryIsSingleWinnerAndReacquires(t *testing.T) {
	root := t.TempDir()
	guard, err := AcquireProjectMutation(context.Background(), root, "install", "op-1")
	if err != nil {
		t.Fatal(err)
	}
	resume := make(chan struct{})
	entered := make(chan struct{})
	results := make(chan error, 2)
	go func() {
		results <- RecoverProjectMutation(context.Background(), root, guard.Owner(), livenessProof{dead: true, entered: entered, wait: resume})
	}()
	<-entered
	go func() {
		results <- RecoverProjectMutation(context.Background(), root, guard.Owner(), livenessProof{dead: true})
	}()
	second := <-results
	if !errors.Is(second, core.ErrRevision) {
		t.Fatalf("concurrent recovery = %v", second)
	}
	close(resume)
	if err := <-results; err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(mutationRecoveryLock))); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("recovery claim retained: %v", err)
	}
	next, err := AcquireProjectMutation(context.Background(), root, "install", "op-2")
	if err != nil {
		t.Fatal(err)
	}
	if err := next.Release(); err != nil {
		t.Fatal(err)
	}
}

func TestMutationGuardReleaseRequiresExactOwner(t *testing.T) {
	root := t.TempDir()
	guard, err := AcquireProjectMutation(context.Background(), root, "install", "op-1")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, filepath.FromSlash(projectMutationLock), mutationOwnerFile)
	owner := guard.Owner()
	owner.Token = "00000000000000000000000000000000"
	if err := writeOwner(path, owner); err != nil {
		t.Fatal(err)
	}
	if err := guard.Release(); !errors.Is(err, core.ErrRevision) {
		t.Fatalf("release error = %v", err)
	}
	if _, err := os.Stat(filepath.Dir(path)); err != nil {
		t.Fatalf("foreign lock removed: %v", err)
	}
}

func TestMutationGuardOwnerRecordIsDurable(t *testing.T) {
	root := t.TempDir()
	guard, err := AcquireProjectMutation(context.Background(), root, "deploy", "batch-1")
	if err != nil {
		t.Fatal(err)
	}
	got, err := readMutationOwner(root)
	if err != nil || got != guard.Owner() {
		t.Fatalf("owner = %#v, %v", got, err)
	}
	if got.Token == "" || got.PID <= 0 || got.Host == "" || got.ProcessStart == "" || got.AcquiredAt == "" || got.HeartbeatAt == "" {
		t.Fatal(got)
	}
	if err := guard.Release(); err != nil {
		t.Fatal(err)
	}
}

func TestConcurrentDeadRecoveryUsesClaim(t *testing.T) {
	root := t.TempDir()
	guard, err := AcquireProjectMutation(context.Background(), root, "test", "crash")
	if err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	var wait sync.WaitGroup
	wait.Add(2)
	errs := make(chan error, 2)
	for range 2 {
		go func() {
			defer wait.Done()
			<-start
			errs <- RecoverProjectMutation(context.Background(), root, guard.Owner(), livenessProof{dead: true})
		}()
	}
	close(start)
	wait.Wait()
	close(errs)
	success, stale := 0, 0
	for err := range errs {
		if err == nil {
			success++
		} else if errors.Is(err, core.ErrRevision) {
			stale++
		}
	}
	if success != 1 || stale != 1 {
		t.Fatalf("success=%d stale=%d", success, stale)
	}
}
