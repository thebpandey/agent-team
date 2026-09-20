package resources_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/thebpandey/agent-team/vnext/internal/core"
	"github.com/thebpandey/agent-team/vnext/internal/resources"
	"github.com/thebpandey/agent-team/vnext/internal/store"
	"github.com/thebpandey/agent-team/vnext/internal/tracker"
)

func newRegistry(t *testing.T, root string, runner tracker.CommandRunner) resources.Registry {
	t.Helper()
	if runner == nil {
		runner = tracker.NewFakeRunner(tracker.CommandResult{})
	}
	return resources.NewRegistry(store.New(root, core.StorageLimits{CanonicalBytes: 16 << 20}), runner)
}

func owner() resources.ResourceOwner {
	return resources.ResourceOwner{Run: "R-1", Team: "TEAM-1", Task: "TASK-1", Worktree: "wt", Revision: "git-1"}
}

func server(id string, port int, url string) resources.ServerRecord {
	return resources.ServerRecord{ID: id, Port: port, URL: url, Target: "dev", Purpose: "ui", Owner: owner(), Ownership: resources.Managed}
}

func browser(id, session, url string) resources.BrowserRecord {
	return resources.BrowserRecord{ID: id, Session: session, URL: url, Target: "dev", Purpose: "ui", Owner: owner(), Ownership: resources.Managed}
}

func TestRegistryCapsCASAndOwnership(t *testing.T) {
	r := newRegistry(t, t.TempDir(), nil)
	first, err := r.ReserveServer(context.Background(), server("S-1", 3000, "http://localhost:3000"), 0)
	if err != nil || first.ExpectedRevision != 0 || first.Revision == 0 {
		t.Fatalf("first reserve = %#v, %v", first, err)
	}
	if _, err := r.ReserveServer(context.Background(), server("S-2", 3000, "http://other:3000"), first.Revision); !errors.Is(err, core.ErrCapacity) {
		t.Fatalf("port collision = %v, want ErrCapacity", err)
	}
	if _, err := r.Release(context.Background(), "S-1", 0); !errors.Is(err, core.ErrRevision) {
		t.Fatalf("stale release = %v, want ErrRevision", err)
	}
	if _, err := r.MarkUnknown(context.Background(), "S-1", first.Revision, "timeout"); err != nil {
		t.Fatalf("mark unknown: %v", err)
	}
}

func TestRegistryServerAndBrowserCapsAndIndependentCollisions(t *testing.T) {
	r := newRegistry(t, t.TempDir(), nil)
	var revision uint64
	for i := 0; i < 2; i++ {
		out, err := r.ReserveServer(context.Background(), server(fmt.Sprintf("S-%d", i), 3000+i, fmt.Sprintf("http://localhost:%d", 3000+i)), revision)
		if err != nil {
			t.Fatal(err)
		}
		revision = out.Revision
	}
	if _, err := r.ReserveServer(context.Background(), server("S-3", 3010, "http://localhost:3010"), revision); !errors.Is(err, core.ErrCapacity) {
		t.Fatalf("third server = %v, want ErrCapacity", err)
	}

	r = newRegistry(t, t.TempDir(), nil)
	if _, err := r.ReserveServer(context.Background(), server("S-1", 3000, "http://localhost:3000"), 0); err != nil {
		t.Fatal(err)
	}
	if _, err := r.ReserveServer(context.Background(), server("S-2", 3100, "http://localhost:3000"), 0); !errors.Is(err, core.ErrCapacity) {
		t.Fatalf("URL collision = %v, want ErrCapacity", err)
	}
	r = newRegistry(t, t.TempDir(), nil)
	if _, err := r.ReserveBrowser(context.Background(), browser("B-1", "session-1", "http://localhost:4000"), 0); err != nil {
		t.Fatal(err)
	}
	if _, err := r.ReserveBrowser(context.Background(), browser("B-2", "session-1", "http://localhost:4100"), 0); !errors.Is(err, core.ErrCapacity) {
		t.Fatalf("session collision = %v, want ErrCapacity", err)
	}
	if _, err := r.ReserveBrowser(context.Background(), browser("B-3", "session-3", "http://localhost:4000"), 0); !errors.Is(err, core.ErrCapacity) {
		t.Fatalf("browser URL collision = %v, want ErrCapacity", err)
	}

	r = newRegistry(t, t.TempDir(), nil)
	revision = 0
	for i := 0; i < 2; i++ {
		out, err := r.ReserveBrowser(context.Background(), browser(fmt.Sprintf("B-%d", i), fmt.Sprintf("session-%d", i), fmt.Sprintf("http://localhost:%d", 4000+i)), revision)
		if err != nil {
			t.Fatal(err)
		}
		revision = out.Revision
	}
	if _, err := r.ReserveBrowser(context.Background(), browser("B-3", "session-3", "http://localhost:4003"), revision); !errors.Is(err, core.ErrCapacity) {
		t.Fatalf("third browser = %v, want ErrCapacity", err)
	}
}

func TestRegistryReusesSameWorktreeRevisionAndPurpose(t *testing.T) {
	r := newRegistry(t, t.TempDir(), nil)
	first, err := r.ReserveServer(context.Background(), server("S-1", 3000, "http://localhost:3000"), 0)
	if err != nil {
		t.Fatal(err)
	}
	reused, err := r.ReserveServer(context.Background(), server("S-1", 3000, "http://localhost:3000"), first.Revision)
	if err != nil {
		t.Fatal(err)
	}
	if reused.ID != first.ID || reused.Revision != first.Revision {
		t.Fatalf("reuse = %#v, want existing %#v", reused, first)
	}
	servers, _, err := r.List(context.Background(), "R-1")
	if err != nil || len(servers) != 1 {
		t.Fatalf("servers = %#v, %v", servers, err)
	}
}

func TestUnknownOccupiesSlotAndUserOwnedIsProtected(t *testing.T) {
	r := newRegistry(t, t.TempDir(), nil)
	unknown := server("S-unknown", 3000, "http://localhost:3000")
	unknown.Ownership = resources.Unknown
	if _, err := r.ReserveServer(context.Background(), unknown, 0); err != nil {
		t.Fatal(err)
	}
	second, err := r.ReserveServer(context.Background(), server("S-1", 3100, "http://localhost:3100"), 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.ReserveServer(context.Background(), server("S-2", 3200, "http://localhost:3200"), second.Revision); !errors.Is(err, core.ErrCapacity) {
		t.Fatalf("unknown slot was not counted: %v", err)
	}

	r = newRegistry(t, t.TempDir(), nil)
	user := server("S-user", 3000, "http://localhost:3000")
	user.Ownership = resources.UserOwned
	user.PIDDiagnostic = "1234"
	reserved, err := r.ReserveServer(context.Background(), user, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.StopManaged(context.Background(), "S-user", reserved.Revision); !errors.Is(err, core.ErrRevision) {
		t.Fatalf("user-owned stop = %v, want ErrRevision", err)
	}
	if _, err := r.ReserveServer(context.Background(), server("S-1", 3100, "http://localhost:3100"), reserved.Revision); err != nil {
		t.Fatalf("user-owned resource consumed managed capacity: %v", err)
	}
}

type recordingRunner struct {
	result tracker.CommandResult
	mu     sync.Mutex
	calls  int
}

func (r *recordingRunner) Run(context.Context, string, ...string) tracker.CommandResult {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls++
	return r.result
}

func (r *recordingRunner) Stop(context.Context, resources.StopRequest) tracker.CommandResult {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls++
	return r.result
}

func TestRegistryStopsOnlyExactManagedOwnership(t *testing.T) {
	runner := &recordingRunner{}
	r := newRegistry(t, t.TempDir(), runner)
	reserved, err := r.ReserveServer(context.Background(), server("S-1", 3000, "http://localhost:3000"), 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.MarkStarted(context.Background(), "S-1", reserved.Revision, "trace:S-1", "process:S-1"); err != nil {
		t.Fatal(err)
	}
	servers, _, err := r.List(context.Background(), "R-1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.StopManaged(context.Background(), "S-1", servers[0].Revision); err != nil {
		t.Fatal(err)
	}
	if runner.calls != 1 {
		t.Fatalf("stop calls = %d, want 1", runner.calls)
	}
}

func TestRegistryCannotReleaseStartedResourceWithoutStopEvidence(t *testing.T) {
	r := newRegistry(t, t.TempDir(), nil)
	reserved, err := r.ReserveServer(context.Background(), server("S-1", 3000, "http://localhost:3000"), 0)
	if err != nil {
		t.Fatal(err)
	}
	started, err := r.MarkStarted(context.Background(), "S-1", reserved.Revision, "trace:S-1", "process:S-1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.Release(context.Background(), "S-1", started.Revision); !errors.Is(err, core.ErrTransition) {
		t.Fatalf("released started resource: %v", err)
	}
	servers, _, err := r.List(context.Background(), "R-1")
	if err != nil || len(servers) != 1 || servers[0].State != "started" {
		t.Fatalf("started record = %#v, %v", servers, err)
	}
}

func TestRegistryRejectsCrossKindAndMalformedIDs(t *testing.T) {
	r := newRegistry(t, t.TempDir(), nil)
	bad := server("server-1", 3000, "http://localhost:3000")
	if _, err := r.ReserveServer(context.Background(), bad, 0); !errors.Is(err, core.ErrPath) {
		t.Fatalf("malformed server ID: %v", err)
	}
	if _, err := r.ReserveServer(context.Background(), server("S-shared", 3000, "http://localhost:3000"), 0); err != nil {
		t.Fatal(err)
	}
	if _, err := r.ReserveBrowser(context.Background(), browser("S-shared", "session-1", "http://localhost:4000"), 0); !errors.Is(err, core.ErrPath) {
		t.Fatalf("wrong browser kind: %v", err)
	}
}

func TestRegistryExactRetryConvergesAcrossSeparateStores(t *testing.T) {
	root := t.TempDir()
	left, right := newRegistry(t, root, nil), newRegistry(t, root, nil)
	first, err := left.ReserveServer(context.Background(), server("S-1", 3000, "http://localhost:3000"), 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := right.ReserveServer(context.Background(), server("S-1", 3000, "http://localhost:3000"), first.ExpectedRevision); err != nil {
		t.Fatalf("exact stale retry did not converge: %v", err)
	}
}

func TestRegistryCleansTerminalSlotOnlyAfterLifecycleEvidence(t *testing.T) {
	r := newRegistry(t, t.TempDir(), &recordingRunner{})
	reserved, err := r.ReserveServer(context.Background(), server("S-1", 3000, "http://localhost:3000"), 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.MarkStarted(context.Background(), "S-1", reserved.Revision, "trace:S-1", "process:S-1"); err != nil {
		t.Fatal(err)
	}
	servers, _, _ := r.List(context.Background(), "R-1")
	if _, err := r.StopManaged(context.Background(), "S-1", servers[0].Revision); err != nil {
		t.Fatal(err)
	}
	if _, err := r.ReserveServer(context.Background(), server("S-2", 3100, "http://localhost:3100"), 4); err != nil {
		t.Fatalf("terminal slot cleanup: %v", err)
	}
}

func TestRegistrySeparateStoresConvergeAfterRace(t *testing.T) {
	root := t.TempDir()
	left := newRegistry(t, root, nil)
	right := newRegistry(t, root, nil)
	start := make(chan struct{})
	results := make(chan error, 2)
	go func() {
		<-start
		_, err := left.ReserveServer(context.Background(), server("S-1", 3000, "http://localhost:3000"), 0)
		results <- err
	}()
	go func() {
		<-start
		_, err := right.ReserveServer(context.Background(), server("S-1", 3000, "http://localhost:3000"), 0)
		results <- err
	}()
	close(start)
	for range 2 {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}
	servers, _, err := left.List(context.Background(), "R-1")
	if err != nil || len(servers) != 1 {
		t.Fatalf("durable registry after race = %#v, %v", servers, err)
	}
}
