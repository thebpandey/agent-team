package store

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/thebpandey/agent-team/vnext/internal/core"
)

func TestMutationGuardNamespaceSyncOrdering(t *testing.T) {
	for _, failAt := range []int32{1, 2, 3} {
		t.Run(fmt.Sprintf("sync-%d", failAt), func(t *testing.T) {
			root := t.TempDir()
			original := syncGuardNamespace
			var calls atomic.Int32
			syncGuardNamespace = func(path string) error {
				if calls.Add(1) == failAt {
					return errors.New("injected namespace sync failure")
				}
				return original(path)
			}
			_, err := AcquireProjectMutation(context.Background(), root, "test", "sync-order")
			syncGuardNamespace = original
			if err == nil {
				t.Fatal("sync failure accepted")
			}
			matches, globErr := filepath.Glob(filepath.Join(root, ".agent-team", "mutation.lock.candidate-*"))
			if globErr != nil || len(matches) != 0 {
				t.Fatalf("candidate residue = %v, %v", matches, globErr)
			}
			owner, ownerErr := MutationLockOwner(root, MutationLockPrimary)
			if failAt == 1 {
				if !errors.Is(ownerErr, core.ErrRevision) {
					t.Fatalf("canonical published before candidate sync: %#v, %v", owner, ownerErr)
				}
				return
			}
			if ownerErr != nil {
				t.Fatalf("complete fail-closed canonical missing: %v", ownerErr)
			}
			if _, err := RecoverProjectMutation(context.Background(), root, recoveryRequest(MutationLockPrimary, owner), livenessProof{dead: true}); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestNativeLivenessRejectsCurrentHolder(t *testing.T) {
	root := t.TempDir()
	guard, err := AcquireProjectMutation(context.Background(), root, "test", "native-live")
	if err != nil {
		t.Fatal(err)
	}
	dead, err := (NativeLiveness{}).HolderDead(context.Background(), guard.Owner())
	if err != nil || dead {
		t.Fatalf("dead=%v err=%v", dead, err)
	}
	if err := guard.Release(); err != nil {
		t.Fatal(err)
	}
}

func TestHolderLivenessRequiresCreationIdentityAndKnownState(t *testing.T) {
	live, exited := false, true
	for _, test := range []struct {
		name              string
		recorded, current string
		exited            *bool
		wantDead, wantErr bool
	}{
		{"live exact owner", "one", "one", &live, false, false},
		{"exited exact owner", "one", "one", &exited, true, false},
		{"reused pid", "one", "two", &live, true, false},
		{"unknown state", "one", "one", nil, false, true},
		{"missing identity", "", "one", &exited, false, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			dead, err := decideHolderDead(test.recorded, test.current, test.exited)
			if dead != test.wantDead || (err != nil) != test.wantErr {
				t.Fatalf("dead=%v err=%v", dead, err)
			}
		})
	}
}

func TestMutationOwnerReadRejectsSymlinkOversizeAndSwap(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*testing.T, string)
	}{
		{"symlink", func(t *testing.T, path string) {
			target := path + ".target"
			if err := os.Rename(path, target); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(target, path); err != nil {
				t.Skipf("symlink unavailable: %v", err)
			}
		}},
		{"oversize", func(t *testing.T, path string) {
			if err := os.WriteFile(path, make([]byte, (16<<10)+1), 0o600); err != nil {
				t.Fatal(err)
			}
		}},
		{"swap", func(t *testing.T, path string) {
			ownerReadHook = func(got string) {
				if got != path {
					return
				}
				ownerReadHook = nil
				if err := os.Rename(path, path+".opened"); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte("foreign"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			guard, err := AcquireProjectMutation(context.Background(), root, "test", "owner-read")
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(root, filepath.FromSlash(projectMutationLock))
			test.mutate(t, path)
			t.Cleanup(func() { ownerReadHook = nil })
			if err := guard.Release(); !errors.Is(err, core.ErrRevision) {
				t.Fatalf("unsafe owner read accepted: %v", err)
			}
			if _, err := os.Lstat(path); err != nil {
				t.Fatalf("replacement removed: %v", err)
			}
		})
	}
}

func TestMutationRecoveryRejectsOwnerSwap(t *testing.T) {
	root := t.TempDir()
	guard, err := AcquireProjectMutation(context.Background(), root, "test", "owner-recovery-swap")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, filepath.FromSlash(projectMutationLock))
	ownerReadHook = func(got string) {
		if got != path {
			return
		}
		ownerReadHook = nil
		if err := os.Rename(path, path+".opened"); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("foreign"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() { ownerReadHook = nil })
	if _, err := RecoverProjectMutation(context.Background(), root, recoveryRequest(MutationLockPrimary, guard.Owner()), livenessProof{dead: true}); !errors.Is(err, core.ErrRevision) {
		t.Fatalf("recovery accepted swapped owner: %v", err)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != "foreign" {
		t.Fatalf("foreign replacement changed: %q, %v", got, err)
	}
}

func TestMutationRecoveryRejectsSymlinkAndOversizeOwner(t *testing.T) {
	for _, kind := range []string{"symlink", "oversize"} {
		t.Run(kind, func(t *testing.T) {
			root := t.TempDir()
			guard, err := AcquireProjectMutation(context.Background(), root, "test", "unsafe-recovery")
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(root, filepath.FromSlash(projectMutationLock))
			if kind == "symlink" {
				target := path + ".target"
				if err := os.Rename(path, target); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(target, path); err != nil {
					t.Skipf("symlink unavailable: %v", err)
				}
			} else if err := os.WriteFile(path, make([]byte, (16<<10)+1), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := RecoverProjectMutation(context.Background(), root, recoveryRequest(MutationLockPrimary, guard.Owner()), livenessProof{dead: true}); !errors.Is(err, core.ErrRevision) {
				t.Fatalf("unsafe recovery owner accepted: %v", err)
			}
			if _, err := os.Lstat(path); err != nil {
				t.Fatalf("unsafe owner removed: %v", err)
			}
		})
	}
}

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
			_, err := RecoverProjectMutation(context.Background(), root, recoveryRequest(MutationLockPrimary, guard.Owner()), livenessProof{})
			if !errors.Is(err, core.ErrRevision) {
				t.Fatal(err)
			}
		},
		"wrong owner": func(t *testing.T, root string, guard *MutationGuard) {
			wrong := guard.Owner()
			wrong.Token = "00000000000000000000000000000000"
			_, err := RecoverProjectMutation(context.Background(), root, recoveryRequest(MutationLockPrimary, wrong), livenessProof{dead: true})
			if !errors.Is(err, core.ErrRevision) {
				t.Fatal(err)
			}
		},
		"unknown liveness": func(t *testing.T, root string, guard *MutationGuard) {
			_, err := RecoverProjectMutation(context.Background(), root, recoveryRequest(MutationLockPrimary, guard.Owner()), livenessProof{err: errors.New("unknown")})
			if !errors.Is(err, core.ErrRevision) {
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
	if err := os.MkdirAll(filepath.Dir(lock), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lock, []byte("not-json"), 0o600); err != nil {
		t.Fatal(err)
	}
	expected := MutationOwner{Token: "00000000000000000000000000000000", Scope: "test", OperationID: "op"}
	_, err := RecoverProjectMutation(context.Background(), root, recoveryRequest(MutationLockPrimary, expected), livenessProof{dead: true})
	if !errors.Is(err, core.ErrRevision) {
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
		_, err := RecoverProjectMutation(context.Background(), root, recoveryRequest(MutationLockPrimary, guard.Owner()), livenessProof{dead: true, entered: entered, wait: resume})
		results <- err
	}()
	<-entered
	if err := guard.Release(); !errors.Is(err, core.ErrRevision) {
		t.Fatalf("release raced recovery claim: %v", err)
	}
	go func() {
		_, err := RecoverProjectMutation(context.Background(), root, recoveryRequest(MutationLockPrimary, guard.Owner()), livenessProof{dead: true})
		results <- err
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
	path := filepath.Join(root, filepath.FromSlash(projectMutationLock))
	owner := guard.Owner()
	owner.Token = "00000000000000000000000000000000"
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := writeOwnerExclusive(path, owner); err != nil {
		t.Fatal(err)
	}
	if err := guard.Release(); !errors.Is(err, core.ErrRevision) {
		t.Fatalf("release error = %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("foreign lock removed: %v", err)
	}
}

func TestMutationGuardReleaseResumesExactTombstoneAndIsIdempotent(t *testing.T) {
	for _, crash := range []string{"owner-claimed"} {
		t.Run(crash, func(t *testing.T) {
			root := t.TempDir()
			guard, err := AcquireProjectMutation(context.Background(), root, "install", "resume-release")
			if err != nil {
				t.Fatal(err)
			}
			canonical := filepath.Join(root, filepath.FromSlash(projectMutationLock))
			claim := canonical + ".removed-" + guard.Owner().Token
			if err := os.Mkdir(claim, 0o700); err != nil {
				t.Fatal(err)
			}
			if crash == "owner-claimed" {
				if err := os.Rename(canonical, filepath.Join(claim, "owned")); err != nil {
					t.Fatal(err)
				}
			}
			if err := guard.Release(); err != nil {
				t.Fatal(err)
			}
			if err := guard.Release(); err != nil {
				t.Fatalf("repeated release = %v", err)
			}
			for _, path := range []string{canonical, claim} {
				if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("release residue %s: %v", path, err)
				}
			}
			next, err := AcquireProjectMutation(context.Background(), root, "install", "after-resume")
			if err != nil {
				t.Fatal(err)
			}
			if err := next.Release(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestMutationGuardReleaseRejectsUnauthenticatedClaimCollision(t *testing.T) {
	root := t.TempDir()
	guard, err := AcquireProjectMutation(context.Background(), root, "install", "claim-collision")
	if err != nil {
		t.Fatal(err)
	}
	canonical := filepath.Join(root, filepath.FromSlash(projectMutationLock))
	claim := canonical + ".removed-" + guard.Owner().Token
	ownerBefore, err := os.Lstat(canonical)
	if err != nil {
		t.Fatal(err)
	}
	rawBefore, err := os.ReadFile(canonical)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(claim, 0o700); err != nil {
		t.Fatal(err)
	}
	claimBefore, err := os.Lstat(claim)
	if err != nil {
		t.Fatal(err)
	}
	if err := guard.Release(); !errors.Is(err, core.ErrRevision) {
		t.Fatalf("claim collision release = %v", err)
	}
	ownerAfter, err := os.Lstat(canonical)
	if err != nil {
		t.Fatal(err)
	}
	rawAfter, err := os.ReadFile(canonical)
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(ownerBefore, ownerAfter) || string(rawBefore) != string(rawAfter) {
		t.Fatal("canonical owner changed on claim collision")
	}
	claimAfter, err := os.Lstat(claim)
	if err != nil || !os.SameFile(claimBefore, claimAfter) ||
		claimBefore.Mode() != claimAfter.Mode() || claimBefore.Size() != claimAfter.Size() ||
		!claimBefore.ModTime().Equal(claimAfter.ModTime()) {
		t.Fatalf("claim identity changed: %v", err)
	}
	if entries, err := os.ReadDir(claim); err != nil || len(entries) != 0 {
		t.Fatalf("claim changed: %v, %v", entries, err)
	}
}

func TestMutationGuardReleaseRejectsSourceReplacementBeforeClaim(t *testing.T) {
	root := t.TempDir()
	guard, err := AcquireProjectMutation(context.Background(), root, "install", "replacement-release")
	if err != nil {
		t.Fatal(err)
	}
	canonical := filepath.Join(root, filepath.FromSlash(projectMutationLock))
	original := canonical + ".original"
	claim := canonical + ".removed-" + guard.Owner().Token
	ownerRemoveClaimHook = func(got string) {
		if got != canonical {
			return
		}
		ownerRemoveClaimHook = nil
		if err := os.Rename(canonical, original); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(canonical, []byte("foreign"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() { ownerRemoveClaimHook = nil })
	if err := guard.Release(); !errors.Is(err, core.ErrRevision) {
		t.Fatalf("replacement release = %v", err)
	}
	if got, err := os.ReadFile(canonical); err != nil || string(got) != "foreign" {
		t.Fatalf("replacement = %q, %v", got, err)
	}
	if _, err := os.Stat(original); err != nil {
		t.Fatalf("original owner changed: %v", err)
	}
	if _, err := os.Lstat(claim); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("replacement claim retained: %v", err)
	}
}

func TestMutationGuardReleaseRejectsParentSwapBeforeClaim(t *testing.T) {
	root := t.TempDir()
	guard, err := AcquireProjectMutation(context.Background(), root, "install", "parent-swap-release")
	if err != nil {
		t.Fatal(err)
	}
	canonical := filepath.Join(root, filepath.FromSlash(projectMutationLock))
	agentTeam := filepath.Dir(canonical)
	original := agentTeam + ".original"
	external := t.TempDir()
	sentinel := filepath.Join(external, "sentinel")
	if err := os.WriteFile(sentinel, []byte("unchanged"), 0o600); err != nil {
		t.Fatal(err)
	}
	ownerRemoveClaimHook = func(got string) {
		if got != canonical {
			return
		}
		ownerRemoveClaimHook = nil
		if err := os.Rename(agentTeam, original); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(external, agentTeam); err != nil {
			t.Skipf("symlink unavailable: %v", err)
		}
	}
	t.Cleanup(func() { ownerRemoveClaimHook = nil })
	if err := guard.Release(); err == nil {
		t.Fatal("parent replacement accepted")
	}
	if got, err := os.ReadFile(sentinel); err != nil || string(got) != "unchanged" {
		t.Fatalf("external sentinel = %q, %v", got, err)
	}
	entries, err := os.ReadDir(external)
	if err != nil || len(entries) != 1 || entries[0].Name() != "sentinel" {
		t.Fatalf("external directory changed: %v, %v", entries, err)
	}
	if _, err := os.Stat(filepath.Join(original, "mutation.lock")); err != nil {
		t.Fatalf("owner moved through replacement: %v", err)
	}
}

func TestMutationGuardReleaseRetainsReplacementAfterPrivateClaim(t *testing.T) {
	root := t.TempDir()
	guard, err := AcquireProjectMutation(context.Background(), root, "install", "claimed-replacement-release")
	if err != nil {
		t.Fatal(err)
	}
	canonical := filepath.Join(root, filepath.FromSlash(projectMutationLock))
	claim := canonical + ".removed-" + guard.Owner().Token
	claimedRelative := filepath.ToSlash(projectMutationLock + ".removed-" + guard.Owner().Token + "/owned")
	ownedRemoveHook = func(opened *os.Root, owned ownedTemp) {
		if owned.name != claimedRelative {
			return
		}
		ownedRemoveHook = nil
		file, err := opened.OpenFile(claimedRelative, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := file.WriteString("foreign"); err != nil {
			_ = file.Close()
			t.Fatal(err)
		}
		if err := file.Close(); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() { ownedRemoveHook = nil })
	if err := guard.Release(); !errors.Is(err, core.ErrRevision) {
		t.Fatalf("claimed replacement release = %v", err)
	}
	if _, err := os.Lstat(canonical); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("canonical owner restored: %v", err)
	}
	if got, err := os.ReadFile(filepath.Join(claim, "owned")); err != nil || string(got) != "foreign" {
		t.Fatalf("claimed replacement = %q, %v", got, err)
	}
}

func TestMutationGuardReleaseRetainsTamperedTombstone(t *testing.T) {
	root := t.TempDir()
	guard, err := AcquireProjectMutation(context.Background(), root, "install", "tampered-tombstone")
	if err != nil {
		t.Fatal(err)
	}
	canonical := filepath.Join(root, filepath.FromSlash(projectMutationLock))
	claim := canonical + ".removed-" + guard.Owner().Token
	if err := os.Mkdir(claim, 0o700); err != nil {
		t.Fatal(err)
	}
	tombstone := filepath.Join(claim, "owned")
	if err := os.Rename(canonical, tombstone); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(tombstone, []byte("tampered"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := guard.Release(); !errors.Is(err, core.ErrRevision) {
		t.Fatalf("tampered tombstone release = %v", err)
	}
	if got, err := os.ReadFile(tombstone); err != nil || string(got) != "tampered" {
		t.Fatalf("tampered tombstone = %q, %v", got, err)
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

func TestMutationGuardPublishesOnlyCompleteOwner(t *testing.T) {
	root := t.TempDir()
	entered, resume := make(chan struct{}), make(chan struct{})
	ownerCandidateHook = func(relative string) {
		if relative == projectMutationLock {
			close(entered)
			<-resume
		}
	}
	t.Cleanup(func() { ownerCandidateHook = nil })
	done := make(chan error, 1)
	go func() {
		guard, err := AcquireProjectMutation(context.Background(), root, "test", "atomic")
		if err == nil {
			err = guard.Release()
		}
		done <- err
	}()
	<-entered
	path := filepath.Join(root, filepath.FromSlash(projectMutationLock))
	if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("partial canonical owner published: %v", err)
	}
	close(resume)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestCrashedRecoveryClaimCanBeRecovered(t *testing.T) {
	root := t.TempDir()
	if _, err := canonicalMutationRoot(root); err != nil {
		t.Fatal(err)
	}
	claim, err := acquireRecoveryClaim(root, "crashed-primary")
	if err != nil {
		t.Fatal(err)
	}
	got, err := RecoverProjectMutation(context.Background(), root, recoveryRequest(MutationLockRecovery, claim.Owner()), livenessProof{dead: true})
	if err != nil || got != claim.Owner() {
		t.Fatal(got, err)
	}
	guard, err := AcquireProjectMutation(context.Background(), root, "test", "after-claim-recovery")
	if err != nil {
		t.Fatal(err)
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
			_, err := RecoverProjectMutation(context.Background(), root, recoveryRequest(MutationLockPrimary, guard.Owner()), livenessProof{dead: true})
			errs <- err
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

func recoveryRequest(target string, owner MutationOwner) MutationRecoveryRequest {
	return MutationRecoveryRequest{Target: target, Token: owner.Token, OperationID: owner.OperationID}
}
