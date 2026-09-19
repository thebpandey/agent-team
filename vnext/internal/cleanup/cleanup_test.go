package cleanup

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"sync"
	"testing"

	"github.com/thebpandey/agent-team/vnext/internal/contracts"
	"github.com/thebpandey/agent-team/vnext/internal/core"
	"github.com/thebpandey/agent-team/vnext/internal/store"
)

type cleanupFake struct {
	events []string
	err    error
}

func (f *cleanupFake) Stop(context.Context, CleanupCandidate) ([]string, error) {
	f.events = append(f.events, "stop")
	return []string{"stop-evidence"}, f.err
}
func (f *cleanupFake) WriteEvidence(context.Context, []string) error {
	f.events = append(f.events, "evidence")
	return f.err
}
func (f *cleanupFake) Verify(context.Context, []string) error {
	f.events = append(f.events, "verify")
	return f.err
}
func (f *cleanupFake) RemoveWorktree(context.Context, CleanupCandidate) error {
	f.events = append(f.events, "worktree")
	return f.err
}

func validCandidate(team core.TeamID) CleanupCandidate {
	return CleanupCandidate{
		Project: "p", Run: "R", Team: team, Task: "T", Worktree: "wt-" + string(team), Branch: "team-" + string(team), Base: "base", Revision: "rev", CleanMerged: true,
		Servers:  []ServerRef{{Run: "R", Team: team, ResourceID: "S-1", Revision: 2, Ownership: "managed"}},
		Browsers: []BrowserRef{{Run: "R", Team: team, ResourceID: "B-1", Revision: 2, Ownership: "managed"}},
	}
}

func TestCleanupRecordsAndVerifiesStopBeforeExactRemoval(t *testing.T) {
	fake := &cleanupFake{}
	if err := ExecuteCleanup(context.Background(), validCandidate("TEAM-1"), fake); err != nil {
		t.Fatal(err)
	}
	if want := []string{"stop", "evidence", "verify", "worktree"}; !reflect.DeepEqual(fake.events, want) {
		t.Fatalf("events = %#v, want %#v", fake.events, want)
	}
}

func TestCleanupFailsClosedBeforeStop(t *testing.T) {
	for _, candidate := range []CleanupCandidate{
		func() CleanupCandidate { c := validCandidate("TEAM-1"); c.Unknown = true; return c }(),
		func() CleanupCandidate { c := validCandidate("TEAM-1"); c.CleanMerged = false; return c }(),
		func() CleanupCandidate { c := validCandidate("TEAM-1"); c.Servers[0].Ownership = "unknown"; return c }(),
		func() CleanupCandidate { c := validCandidate("TEAM-1"); c.Browsers[0].Team = "OTHER"; return c }(),
	} {
		fake := &cleanupFake{}
		if err := ExecuteCleanup(context.Background(), candidate, fake); !errors.Is(err, core.ErrCapacity) {
			t.Fatalf("candidate %#v error = %v", candidate, err)
		}
		if len(fake.events) != 0 {
			t.Fatalf("unsafe candidate invoked ops: %#v", fake.events)
		}
	}
}

type authorityFake struct{}

func (authorityFake) Preflight(_ context.Context, x CleanupCandidate) (CleanupProof, error) {
	return CleanupProof{Project: x.Project, Run: x.Run, Team: x.Team, Task: x.Task, Worktree: x.Worktree, Branch: x.Branch, Base: x.Base, Revision: x.Revision, ReceiptRevision: 1, ReceiptDigest: "d", TerminalClean: true, Integrated: true, NoConsumers: true, Resources: []ResourceRef{{Project: x.Project, Kind: "server", Run: x.Run, Team: x.Team, Worktree: x.Worktree, Revision: x.Revision, ResourceID: "S", Generation: 1}}}, nil
}

type resourceFake struct{}

func (resourceFake) StopExact(_ context.Context, _ CleanupProof, r []ResourceRef) ([]StopReceipt, error) {
	return []StopReceipt{{Project: r[0].Project, Resource: r[0], Evidence: "e", Digest: "d"}}, nil
}
func (resourceFake) VerifyExact(context.Context, []ResourceRef, []StopReceipt) error { return nil }

type countingResources struct {
	stops, verifies int
	verifyErr       error
}

func (r *countingResources) StopExact(_ context.Context, _ CleanupProof, refs []ResourceRef) ([]StopReceipt, error) {
	r.stops++
	return []StopReceipt{{Project: refs[0].Project, Resource: refs[0], Evidence: "e", Digest: "d"}}, nil
}
func (r *countingResources) VerifyExact(context.Context, []ResourceRef, []StopReceipt) error {
	r.verifies++
	err := r.verifyErr
	r.verifyErr = nil
	return err
}

type badAuthority struct{}

func (badAuthority) Preflight(context.Context, CleanupCandidate) (CleanupProof, error) {
	return CleanupProof{}, nil
}

func TestCleanerFailsClosedOnMissingOrMismatchedAuthority(t *testing.T) {
	x := validCandidate("A")
	if err := NewCleaner(store.New(t.TempDir(), core.StorageLimits{}), "p", nil, &countingResources{}, &exactRemover{}).Cleanup(context.Background(), x); !errors.Is(err, core.ErrCapacity) {
		t.Fatal(err)
	}
	if err := NewCleaner(store.New(t.TempDir(), core.StorageLimits{}), "p", badAuthority{}, &countingResources{}, &exactRemover{}).Cleanup(context.Background(), x); !errors.Is(err, core.ErrRevision) {
		t.Fatal(err)
	}
}
func TestRetryAfterVerifyFailureDoesNotStopAgain(t *testing.T) {
	r := &countingResources{verifyErr: errors.New("temporary")}
	c := NewCleaner(store.New(t.TempDir(), core.StorageLimits{CanonicalBytes: 16 << 20}), "p", authorityFake{}, r, &exactRemover{})
	x := validCandidate("A")
	if err := c.Cleanup(context.Background(), x); err == nil {
		t.Fatal("first verify unexpectedly passed")
	}
	if err := c.Cleanup(context.Background(), x); err != nil {
		t.Fatal(err)
	}
	if r.stops != 1 {
		t.Fatalf("stops=%d", r.stops)
	}
}

func TestCleanersShareExactCandidateGuard(t *testing.T) {
	s := store.New(t.TempDir(), core.StorageLimits{CanonicalBytes: 16 << 20})
	r := &countingResources{}
	a, b := NewCleaner(s, "p", authorityFake{}, r, &exactRemover{}), NewCleaner(s, "p", authorityFake{}, r, &exactRemover{})
	var wg sync.WaitGroup
	for _, c := range []Cleaner{a, b} {
		wg.Add(1)
		go func(c Cleaner) {
			defer wg.Done()
			if err := c.Cleanup(context.Background(), validCandidate("A")); err != nil {
				t.Error(err)
			}
		}(c)
	}
	wg.Wait()
	if r.stops != 1 {
		t.Fatalf("stops=%d", r.stops)
	}
}

func TestCleanerAliasesIgnoreUnboundCandidateExtras(t *testing.T) {
	root := t.TempDir()
	alias := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(root, alias); err != nil {
		t.Skip(err)
	}
	r := &countingResources{}
	a, b := NewCleaner(store.New(root, core.StorageLimits{CanonicalBytes: 16 << 20}), "p", authorityFake{}, r, &exactRemover{}), NewCleaner(store.New(alias, core.StorageLimits{CanonicalBytes: 16 << 20}), "p", authorityFake{}, r, &exactRemover{})
	x, y := validCandidate("A"), validCandidate("A")
	x.EvidencePointers = []string{"one"}
	y.EvidencePointers = []string{"two"}
	var wg sync.WaitGroup
	for i, c := range []Cleaner{a, b} {
		candidate := []CleanupCandidate{x, y}[i]
		wg.Add(1)
		go func(c Cleaner, x CleanupCandidate) {
			defer wg.Done()
			if err := c.Cleanup(context.Background(), x); err != nil {
				t.Error(err)
			}
		}(c, candidate)
	}
	wg.Wait()
	if r.stops != 1 {
		t.Fatalf("stops=%d", r.stops)
	}
}

type retargetAuthority struct {
	alias, target string
	calls         int
}

func (a *retargetAuthority) Preflight(_ context.Context, x CleanupCandidate) (CleanupProof, error) {
	a.calls++
	if a.calls == 2 {
		if err := os.Remove(a.alias); err != nil {
			return CleanupProof{}, err
		}
		if err := os.Symlink(a.target, a.alias); err != nil {
			return CleanupProof{}, err
		}
	}
	return authorityFake{}.Preflight(context.Background(), x)
}
func TestCleanupPinsCanonicalStoreAfterAliasRetarget(t *testing.T) {
	original, other := t.TempDir(), t.TempDir()
	alias := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(original, alias); err != nil {
		t.Skip(err)
	}
	a := &retargetAuthority{alias: alias, target: other}
	r := &countingResources{}
	c := NewCleaner(store.New(alias, core.StorageLimits{CanonicalBytes: 16 << 20}), "p", a, r, &exactRemover{})
	if err := c.Cleanup(context.Background(), validCandidate("A")); err != nil {
		t.Fatal(err)
	}
	if r.stops != 1 {
		t.Fatal(r.stops)
	}
	if _, err := os.Stat(filepath.Join(original, ".agent-team", "cleanup", "R", "A")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(other, ".agent-team")); !os.IsNotExist(err) {
		t.Fatalf("retarget received state: %v", err)
	}
}

type exactRemover struct {
	mu    sync.Mutex
	paths []string
}

func (r *exactRemover) RemoveExact(_ context.Context, w contracts.Worktree) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.paths = append(r.paths, w.Path)
	return nil
}

func TestConcurrentCleanupRemovesOnlyExactCandidates(t *testing.T) {
	remover := &exactRemover{}
	cleaner := NewCleaner(store.New(t.TempDir(), core.StorageLimits{CanonicalBytes: 16 << 20}), "p", authorityFake{}, resourceFake{}, remover)
	candidates := []CleanupCandidate{validCandidate("A"), validCandidate("B")}
	var wait sync.WaitGroup
	for _, candidate := range candidates {
		candidate := candidate
		wait.Add(1)
		go func() {
			defer wait.Done()
			if err := cleaner.Cleanup(context.Background(), candidate); err != nil {
				t.Error(err)
			}
		}()
	}
	wait.Wait()
	sort.Strings(remover.paths)
	if want := []string{"wt-A", "wt-B"}; !reflect.DeepEqual(remover.paths, want) {
		t.Fatalf("removed %#v, want %#v", remover.paths, want)
	}
}
