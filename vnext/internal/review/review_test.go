package review

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/thebpandey/agent-team/vnext/internal/contracts"
	"github.com/thebpandey/agent-team/vnext/internal/core"
	"github.com/thebpandey/agent-team/vnext/internal/store"
	"github.com/thebpandey/agent-team/vnext/internal/tracker"
)

func TestReviewerImmutableAttemptsAndFreshReplay(t *testing.T) {
	state := store.New(t.TempDir(), core.StorageLimits{CanonicalBytes: 16 << 20})
	adapter := &reviewAdapter{}
	r := NewReviewer(adapter, nil, state)
	first := validInput(t, "rev-1", "sha256:first")
	got, err := r.Review(context.Background(), first)
	if err != nil || got.Verdict != CLEAN {
		t.Fatalf("first Review() = %#v, %v", got, err)
	}
	again, err := NewReviewer(adapter, nil, state).Review(context.Background(), first)
	if err != nil || !reflect.DeepEqual(again, got) || adapter.startCount() != 1 {
		t.Fatalf("fresh replay = %#v, %v; starts=%d", again, err, adapter.startCount())
	}
	repaired := validInput(t, "rev-2", "sha256:repaired")
	repaired.Candidate.Worktree.Path, repaired.Candidate.Worktree.Canonical = first.Candidate.Worktree.Path, first.Candidate.Worktree.Canonical
	gotRepair, err := r.Review(context.Background(), repaired)
	if err != nil || gotRepair.EvidencePointer == got.EvidencePointer || adapter.startCount() != 2 {
		t.Fatalf("repaired Review() = %#v, %v; starts=%d", gotRepair, err, adapter.startCount())
	}
}

func TestReviewerConcurrentPublicationReturnsOneImmutableReceipt(t *testing.T) {
	state := store.New(t.TempDir(), core.StorageLimits{CanonicalBytes: 16 << 20})
	in := validInput(t, "rev", "sha256:candidate")
	var wg sync.WaitGroup
	results := make([]Result, 2)
	errs := make([]error, 2)
	for i := range results {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i], errs[i] = NewReviewer(&reviewAdapter{}, nil, state).Review(context.Background(), in)
		}(i)
	}
	wg.Wait()
	if errs[0] != nil || errs[1] != nil || !reflect.DeepEqual(results[0], results[1]) {
		t.Fatalf("concurrent Review() = %#v/%v, %#v/%v", results[0], errs[0], results[1], errs[1])
	}
}

func TestReviewerRejectsTamperedOrStaleReceipt(t *testing.T) {
	cases := []struct {
		name string
		edit func(*durableResult)
	}{
		{"findings", func(record *durableResult) { record.Findings = []string{"forged"} }},
		{"candidate", func(record *durableResult) { record.Provenance.Candidate.Base = "forged" }},
		{"developer", func(record *durableResult) { record.Provenance.Developer.Identity = "forged" }},
		{"envelope", func(record *durableResult) { record.Project = "forged" }},
		{"evidence", func(record *durableResult) { record.EvidencePointer = "forged" }},
		{"reviewer host", func(record *durableResult) { record.Reviewer.Host = "forged"; record.Digest = receiptDigest(*record) }},
		{"stale", func(record *durableResult) { record.WrittenAt = "not-a-time"; record.Digest = receiptDigest(*record) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			state := store.New(t.TempDir(), core.StorageLimits{CanonicalBytes: 16 << 20})
			in := validInput(t, "rev", "sha256:candidate")
			r := NewReviewer(&reviewAdapter{}, nil, state)
			got, err := r.Review(context.Background(), in)
			if err != nil {
				t.Fatal(err)
			}
			var record durableResult
			if err := state.ReadJSON(got.EvidencePointer, maxReceiptBytes, &record); err != nil {
				t.Fatal(err)
			}
			tc.edit(&record)
			if _, err := state.WriteJSON(got.EvidencePointer, record, maxReceiptBytes); err != nil {
				t.Fatal(err)
			}
			if _, err := r.Review(context.Background(), in); !errors.Is(err, core.ErrRevision) {
				t.Fatalf("tampered receipt error = %v", err)
			}
		})
	}
}

func TestReviewerRejectsIncompleteOrUnprovenProvenanceBeforeReservation(t *testing.T) {
	cases := []struct {
		name string
		edit func(*Input)
	}{
		{"developer", func(in *Input) { in.Developer.Identity = "" }},
		{"canonical", func(in *Input) { in.Candidate.Worktree.Canonical = "" }},
		{"dot path", func(in *Input) { in.Candidate.Worktree.Path = "." }},
		{"main checkout", func(in *Input) {
			wd, _ := os.Getwd()
			in.Candidate.Worktree.Path, in.Candidate.Worktree.Canonical = wd, wd
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			adapter := &reviewAdapter{}
			in := validInput(t, "rev", "sha256:candidate")
			tc.edit(&in)
			if _, err := NewReviewer(adapter, nil, store.New(t.TempDir(), core.StorageLimits{CanonicalBytes: 16 << 20})).Review(context.Background(), in); !errors.Is(err, core.ErrPath) || adapter.startCount() != 0 {
				t.Fatalf("Review() error=%v starts=%d", err, adapter.startCount())
			}
		})
	}
	if _, err := NewReviewer(nil, nil, store.New(t.TempDir(), core.StorageLimits{CanonicalBytes: 16 << 20})).Review(context.Background(), validInput(t, "rev", "sha256:nil")); !errors.Is(err, core.ErrCapacity) {
		t.Fatalf("nil host error = %v", err)
	}
}

func TestReviewerRejectsNonIndependentIdentityBeforeChecks(t *testing.T) {
	in := validInput(t, "rev", "sha256:candidate")
	adapter := &reviewAdapter{identity: "developer"}
	runner := &checkRunner{result: tracker.CommandResult{Exit: 1, Stderr: []byte("failed")}}
	if _, err := NewReviewer(adapter, runner, store.New(t.TempDir(), core.StorageLimits{CanonicalBytes: 16 << 20})).Review(context.Background(), in); !errors.Is(err, core.ErrRevision) || runner.callCount() != 0 {
		t.Fatalf("Review() error=%v checks=%d", err, runner.callCount())
	}
}

func TestReviewerBoundsChecksExecutionAndFindings(t *testing.T) {
	cases := []struct {
		name   string
		checks []core.Check
		runner *checkRunner
	}{
		{"count", makeChecks(maxChecks + 1), nil},
		{"arguments", []core.Check{{Name: "unit", Command: makeParts(maxCommandParts + 1)}}, nil},
		{"bytes", maxByteChecks(), nil},
		{"execution", []core.Check{{Name: "unit", Command: []string{"test"}}}, &checkRunner{result: tracker.CommandResult{Stdout: []byte(strings.Repeat("x", maxExecutionBytes+1))}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			adapter := &reviewAdapter{}
			in := validInput(t, "rev", "sha256:"+tc.name)
			in.Checks = tc.checks
			if _, err := NewReviewer(adapter, tc.runner, store.New(t.TempDir(), core.StorageLimits{CanonicalBytes: 16 << 20})).Review(context.Background(), in); !errors.Is(err, core.ErrLimit) {
				t.Fatalf("Review() error = %v", err)
			}
			if tc.name != "execution" && adapter.startCount() != 0 {
				t.Fatalf("reserved before input bounds: %d", adapter.startCount())
			}
		})
	}
}

func TestReviewerCreatesFixEvidence(t *testing.T) {
	in := validInput(t, "rev", "sha256:checks")
	in.Checks = []core.Check{{Name: "unit", Command: []string{"test", "arg"}}}
	runner := &checkRunner{result: tracker.CommandResult{Exit: 1, Stderr: []byte("failed")}}
	got, err := NewReviewer(&reviewAdapter{}, runner, store.New(t.TempDir(), core.StorageLimits{CanonicalBytes: 16 << 20})).Review(context.Background(), in)
	if err != nil || got.Verdict != FIX || len(got.Findings) != 1 || runner.callCount() != 1 {
		t.Fatalf("Review() = %#v, %v, checks=%d", got, err, runner.callCount())
	}
}

func validInput(t *testing.T, revision, digest string) Input {
	t.Helper()
	root := managedTaskRoot(t)
	return Input{Task: core.Task{RecordEnvelope: core.RecordEnvelope{Project: "project", RunID: "run"}, ID: "task"}, Candidate: contracts.Candidate{Task: "task", Revision: revision, Base: "base", Worktree: contracts.Worktree{Run: "run", Team: "team", Path: root, Canonical: root, Branch: "branch", Base: "base"}}, Developer: contracts.WorkerHandle{Host: "developer-host", Identity: "developer", Run: "run", Team: "team", Task: "task", CandidateRevision: revision, PacketDigest: digest}, CandidateDigest: digest}
}
func managedTaskRoot(t *testing.T) string {
	t.Helper()
	root, err := moduleRoot()
	if err != nil {
		t.Fatal(err)
	}
	managed := filepath.Join(root, ".agent-team", "worktrees")
	_, existed := os.Stat(managed)
	if err := os.MkdirAll(managed, 0o700); err != nil {
		t.Fatal(err)
	}
	path, err := os.MkdirTemp(managed, "review-test-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.RemoveAll(path)
		if os.IsNotExist(existed) {
			_ = os.RemoveAll(filepath.Join(root, ".agent-team"))
		}
	})
	return path
}
func makeChecks(n int) []core.Check {
	checks := make([]core.Check, n)
	for i := range checks {
		checks[i] = core.Check{Name: "unit", Command: []string{"test"}}
	}
	return checks
}
func makeParts(n int) []string {
	parts := make([]string, n)
	for i := range parts {
		parts[i] = "arg"
	}
	return parts
}
func maxByteChecks() []core.Check {
	checks := make([]core.Check, 17)
	for i := range checks {
		checks[i] = core.Check{Name: strings.Repeat("n", 1024), Command: []string{"test"}}
	}
	return checks
}

type reviewAdapter struct {
	mu       sync.Mutex
	starts   int
	identity string
}

func (*reviewAdapter) Probe(context.Context) (contracts.HostCapabilities, error) {
	return contracts.HostCapabilities{Host: "review-host"}, nil
}
func (*reviewAdapter) StartWorker(context.Context, contracts.WorkerRequest) (contracts.WorkerHandle, error) {
	return contracts.WorkerHandle{}, nil
}
func (a *reviewAdapter) StartReviewer(_ context.Context, req contracts.WorkerRequest, _ contracts.WorkerHandle) (contracts.WorkerHandle, error) {
	a.mu.Lock()
	a.starts++
	a.mu.Unlock()
	return contracts.WorkerHandle{Host: "review-host", Identity: "reviewer", Reviewer: true, Run: req.Packet.RunID, Team: req.Packet.Team, Task: req.Packet.Task, CandidateRevision: req.Packet.SpecRevision, PacketDigest: req.Packet.QueueFingerprint}, nil
}
func (*reviewAdapter) Poll(context.Context, contracts.WorkerHandle) (string, error)   { return "", nil }
func (*reviewAdapter) Stop(context.Context, contracts.WorkerHandle, core.Scope) error { return nil }
func (a *reviewAdapter) ReadIdentity(context.Context, contracts.WorkerHandle) (string, error) {
	if a.identity != "" {
		return a.identity, nil
	}
	return "reviewer", nil
}
func (a *reviewAdapter) startCount() int { a.mu.Lock(); defer a.mu.Unlock(); return a.starts }

type checkRunner struct {
	mu     sync.Mutex
	result tracker.CommandResult
	calls  int
}

func (r *checkRunner) Run(_ context.Context, _ string, _ ...string) tracker.CommandResult {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls++
	return r.result
}
func (r *checkRunner) callCount() int { r.mu.Lock(); defer r.mu.Unlock(); return r.calls }
