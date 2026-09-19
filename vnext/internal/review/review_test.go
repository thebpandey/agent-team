package review

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/thebpandey/agent-team/vnext/internal/contracts"
	"github.com/thebpandey/agent-team/vnext/internal/core"
	"github.com/thebpandey/agent-team/vnext/internal/store"
	"github.com/thebpandey/agent-team/vnext/internal/tracker"
)

func TestReviewerPersistsBoundCandidateAndRejectsMutation(t *testing.T) {
	r := NewReviewer(nil, nil, store.New(t.TempDir(), core.StorageLimits{CanonicalBytes: 16 << 20}))
	in := Input{Task: core.Task{RecordEnvelope: core.RecordEnvelope{RunID: "run"}, ID: "task"}, Candidate: contracts.Candidate{Task: "task", Revision: "rev", Base: "base", Worktree: contracts.Worktree{Run: "run", Team: "team", Path: "/tmp/task", Base: "base"}}, CandidateDigest: "sha256:candidate"}
	got, err := r.Review(context.Background(), in)
	if err != nil || (got.Verdict != FIX && got.Verdict != CLEAN) || got.CandidateDigest != in.CandidateDigest || got.EvidencePointer == "" || got.ReviewerIdentity == "" {
		t.Fatalf("Review() = %#v, %v", got, err)
	}
	again, err := r.Review(context.Background(), in)
	if err != nil || !reflect.DeepEqual(again, got) {
		t.Fatalf("idempotent Review() = %#v, %v", again, err)
	}
	in.CandidateDigest = "sha256:mutated"
	if _, err := r.Review(context.Background(), in); !errors.Is(err, core.ErrRevision) {
		t.Fatalf("mutation error = %v", err)
	}
}

func TestReviewerRequiresIndependentVerifiedHostIdentity(t *testing.T) {
	adapter := &reviewAdapter{}
	r := NewReviewer(adapter, nil, store.New(t.TempDir(), core.StorageLimits{CanonicalBytes: 16 << 20}))
	in := Input{Task: core.Task{RecordEnvelope: core.RecordEnvelope{RunID: "run"}, ID: "task"}, Candidate: contracts.Candidate{Task: "task", Revision: "rev", Base: "base", Worktree: contracts.Worktree{Run: "run", Team: "team", Path: "/tmp/task", Base: "base"}}, Developer: contracts.WorkerHandle{Identity: "developer", Run: "run", Team: "team", Task: "task", CandidateRevision: "rev", PacketDigest: "sha256:candidate"}, CandidateDigest: "sha256:candidate"}
	got, err := r.Review(context.Background(), in)
	if err != nil || !adapter.started || got.ReviewerIdentity != "reviewer" {
		t.Fatalf("Review() = %#v, %v", got, err)
	}
	adapter.identity = "developer"
	in.CandidateDigest = "sha256:second"
	if _, err := r.Review(context.Background(), in); !errors.Is(err, core.ErrRevision) {
		t.Fatalf("non-independent reviewer error = %v", err)
	}
}

func TestReviewerCreatesBoundedFixFindings(t *testing.T) {
	runner := &checkRunner{result: tracker.CommandResult{Exit: 1, Stderr: []byte("failed")}}
	r := NewReviewer(nil, runner, store.New(t.TempDir(), core.StorageLimits{CanonicalBytes: 16 << 20}))
	in := Input{Task: core.Task{RecordEnvelope: core.RecordEnvelope{RunID: "run"}, ID: "task"}, CandidateDigest: "sha256:checks", Checks: []core.Check{{Name: "unit", Command: []string{"test", "arg"}}}}
	got, err := r.Review(context.Background(), in)
	if err != nil || got.Verdict != FIX || len(got.Findings) != 1 || len(runner.calls) != 1 {
		t.Fatalf("Review() = %#v, %v, calls %#v", got, err, runner.calls)
	}
}

func TestReviewerMinimalPlanInputAndVerdictValidation(t *testing.T) {
	r := NewReviewer(nil, nil, store.New(t.TempDir(), core.StorageLimits{CanonicalBytes: 16 << 20}))
	got, err := r.Review(context.Background(), Input{Task: core.Task{ID: "T"}, CandidateDigest: "sha256:a"})
	if err != nil || (got.Verdict != FIX && got.Verdict != CLEAN) {
		t.Fatalf("Review() = %#v, %v", got, err)
	}
	if !errors.Is(validateVerdict("other"), core.ErrTransition) {
		t.Fatal("invalid verdict accepted")
	}
}

type reviewAdapter struct {
	started  bool
	identity string
}

func (*reviewAdapter) Probe(context.Context) (contracts.HostCapabilities, error) {
	return contracts.HostCapabilities{}, nil
}
func (a *reviewAdapter) StartWorker(context.Context, contracts.WorkerRequest) (contracts.WorkerHandle, error) {
	return contracts.WorkerHandle{}, nil
}
func (a *reviewAdapter) StartReviewer(_ context.Context, req contracts.WorkerRequest, _ contracts.WorkerHandle) (contracts.WorkerHandle, error) {
	a.started = true
	return contracts.WorkerHandle{Identity: "reviewer", Reviewer: true, Run: req.Packet.RunID, Team: req.Packet.Team, Task: req.Packet.Task, CandidateRevision: req.Packet.SpecRevision, PacketDigest: req.Packet.QueueFingerprint}, nil
}
func (*reviewAdapter) Poll(context.Context, contracts.WorkerHandle) (string, error)   { return "", nil }
func (*reviewAdapter) Stop(context.Context, contracts.WorkerHandle, core.Scope) error { return nil }
func (a *reviewAdapter) ReadIdentity(context.Context, contracts.WorkerHandle) (string, error) {
	if a.identity != "" {
		return a.identity, nil
	}
	return "reviewer", nil
}

type checkRunner struct {
	result tracker.CommandResult
	calls  [][]string
}

func (r *checkRunner) Run(_ context.Context, name string, args ...string) tracker.CommandResult {
	r.calls = append(r.calls, append([]string{name}, args...))
	return r.result
}
