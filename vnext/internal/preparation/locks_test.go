package preparation

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/thebpandey/agent-team/vnext/internal/store"
)

func TestPreparationLockRetainsLegacyMarkersAndDoesNotNestProjectGuard(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, ".agent-team/dependencies/.install-lock", "")
	writeFixture(t, root, ".agent-team/dependencies/prepared/.graphify-lock", "")
	project, err := store.AcquireProjectMutation(context.Background(), root, "setup", "fixture")
	if err != nil {
		t.Fatal(err)
	}
	defer project.Release()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	guard, err := acquirePreparationLock(ctx, root, "install:serena")
	if err != nil {
		t.Fatal(err)
	}
	if err := guard.Release(); err != nil {
		t.Fatal(err)
	}
	for _, rel := range []string{".agent-team/dependencies/.install-lock", ".agent-team/dependencies/prepared/.graphify-lock"} {
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil || len(data) != 0 {
			t.Fatalf("legacy marker changed: %s %v", rel, err)
		}
	}
}

func TestPreparationLockKeepsLiveOwnerAndAllowsRetryAfterRelease(t *testing.T) {
	root := t.TempDir()
	guard, err := acquirePreparationLock(context.Background(), root, "install:serena")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer cancel()
	if _, err := acquirePreparationLock(ctx, root, "initialize:graphify"); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("live lock accepted: %v", err)
	}
	owner, err := store.MutationLockOwner(preparationLockRoot(root), store.MutationLockPrimary)
	if err != nil || owner != guard.Owner() {
		t.Fatalf("live owner changed: %#v %v", owner, err)
	}
	if err := guard.Release(); err != nil {
		t.Fatal(err)
	}
	second, err := acquirePreparationLock(context.Background(), root, "initialize:graphify")
	if err != nil {
		t.Fatal(err)
	}
	if err := second.Release(); err != nil {
		t.Fatal(err)
	}
}

// A real child process leaves the durable owner behind on exit. Native liveness
// must prove that exact process exited; tests never invent or kill arbitrary PIDs.
func TestPreparationLockAbandonedChild(t *testing.T) {
	root := os.Getenv("AGENT_TEAM_PREPARATION_LOCK_TEST_ROOT")
	if root == "" {
		return
	}
	if _, err := acquirePreparationLock(context.Background(), root, "abandoned-fixture"); err != nil {
		t.Fatal(err)
	}
}

func TestPreparationLockRecoversExitedOwnerAndRecoveryClaim(t *testing.T) {
	for _, recovery := range []bool{false, true} {
		t.Run(map[bool]string{false: "primary", true: "recovery-and-primary"}[recovery], func(t *testing.T) {
			root := t.TempDir()
			cmd := exec.Command(os.Args[0], "-test.run=^TestPreparationLockAbandonedChild$")
			cmd.Env = append(os.Environ(), "AGENT_TEAM_PREPARATION_LOCK_TEST_ROOT="+root)
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("child failed: %s %v", out, err)
			}
			lockRoot := preparationLockRoot(root)
			old, err := store.MutationLockOwner(lockRoot, store.MutationLockPrimary)
			if err != nil {
				t.Fatal(err)
			}
			if recovery {
				raw, _ := json.Marshal(old)
				if err := os.WriteFile(filepath.Join(lockRoot, ".agent-team", "mutation.recovery"), raw, 0600); err != nil {
					t.Fatal(err)
				}
			}
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			guard, err := acquirePreparationLock(ctx, root, "retry")
			if err != nil {
				t.Fatal(err)
			}
			if guard.Owner().Token == old.Token {
				t.Fatal("reused dead owner")
			}
			if err := guard.Release(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestPreparationLockPreservesUnknownOwner(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(preparationLockRoot(root), ".agent-team", "mutation.lock")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("custom record"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := acquirePreparationLock(context.Background(), root, "retry"); err == nil {
		t.Fatal("accepted unknown owner")
	}
	data, _ := os.ReadFile(path)
	if string(data) != "custom record" {
		t.Fatal("removed unknown owner")
	}
}
