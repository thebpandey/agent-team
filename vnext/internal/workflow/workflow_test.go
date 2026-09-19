package workflow

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/thebpandey/agent-team/vnext/internal/core"
	"github.com/thebpandey/agent-team/vnext/internal/knowledge"
	"github.com/thebpandey/agent-team/vnext/internal/store"
)

func lifecycleEvent(kind EventKind, scope core.Scope, state core.TaskState) Event {
	return Event{
		Run:           "RUN-1",
		Scope:         scope,
		Kind:          kind,
		From:          state,
		Reason:        "operator requested",
		AdmissionHeld: true,
		RefillHeld:    true,
		Confirmed:     scope.Kind != core.ScopeProject,
	}
}

const testDigest = "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
const otherTestDigest = "sha256:fedcba9876543210fedcba9876543210fedcba9876543210fedcba9876543210"

func TestTransitionTable(t *testing.T) {
	scopeTask := core.Scope{Kind: core.ScopeTask, ID: "TASK-1"}
	scopeProject := core.Scope{Kind: core.ScopeProject, ID: "PROJECT-1"}
	tests := []struct {
		name    string
		state   core.TaskState
		event   Event
		want    core.TaskState
		wantErr error
	}{
		{name: "task pause", state: core.Implementing, event: lifecycleEvent(Pause, scopeTask, core.Implementing), want: core.Paused},
		{name: "team stop preserves interruption", state: core.Reviewing, event: lifecycleEvent(Stop, core.Scope{Kind: core.ScopeTeam, ID: "TEAM-1"}, core.Reviewing), want: core.Interrupted},
		{name: "cancel is terminal and explicit", state: core.Working, event: func() Event { e := lifecycleEvent(Cancel, scopeTask, core.Working); e.Confirmed = true; return e }(), want: core.Cancelled},
		{name: "blocked resume", state: core.Blocked, event: lifecycleEvent(Resume, core.Scope{Kind: core.ScopeTeam, ID: "TEAM-1"}, core.Blocked), want: core.Ready},
		{name: "paused resume", state: core.Paused, event: lifecycleEvent(Resume, scopeTask, core.Paused), want: core.Ready},
		{name: "interrupted resume", state: core.Interrupted, event: lifecycleEvent(Resume, scopeTask, core.Interrupted), want: core.Ready},
		{name: "checkpoint preserves interrupted", state: core.Interrupted, event: func() Event {
			e := lifecycleEvent(EventCheckpoint, scopeTask, core.Interrupted)
			e.CheckpointDigest = testDigest
			e.Revision = 1
			return e
		}(), want: core.Interrupted},
		{name: "project pause requires confirmation", state: core.Working, event: func() Event { e := lifecycleEvent(Pause, scopeProject, core.Working); e.Confirmed = true; return e }(), want: core.Paused},
		{name: "pause requires admission hold", state: core.Working, event: func() Event { e := lifecycleEvent(Pause, scopeTask, core.Working); e.AdmissionHeld = false; return e }(), wantErr: core.ErrTransition},
		{name: "pause requires refill hold", state: core.Working, event: func() Event { e := lifecycleEvent(Pause, scopeTask, core.Working); e.RefillHeld = false; return e }(), wantErr: core.ErrTransition},
		{name: "stop rejects blank reason", state: core.Working, event: func() Event { e := lifecycleEvent(Stop, scopeTask, core.Working); e.Reason = "  "; return e }(), wantErr: core.ErrTransition},
		{name: "project pause requires confirmation", state: core.Working, event: lifecycleEvent(Pause, scopeProject, core.Working), wantErr: core.ErrTransition},
		{name: "resume clean rejected", state: core.Clean, event: lifecycleEvent(Resume, scopeTask, core.Clean), wantErr: core.ErrTransition},
		{name: "resume integrated rejected", state: core.Integrated, event: lifecycleEvent(Resume, scopeTask, core.Integrated), wantErr: core.ErrTransition},
		{name: "resume cancelled rejected", state: core.Cancelled, event: lifecycleEvent(Resume, scopeTask, core.Cancelled), wantErr: core.ErrTransition},
		{name: "from mismatch rejected", state: core.Working, event: func() Event { e := lifecycleEvent(Pause, scopeTask, core.Ready); return e }(), wantErr: core.ErrTransition},
		{name: "unknown scope rejected", state: core.Working, event: lifecycleEvent(Pause, core.Scope{Kind: "bogus", ID: "x"}, core.Working), wantErr: core.ErrTransition},
		{name: "unsafe scope rejected", state: core.Working, event: lifecycleEvent(Pause, core.Scope{Kind: core.ScopeTask, ID: "../escape"}, core.Working), wantErr: core.ErrTransition},
		{name: "unrelated run rejected", state: core.Working, event: func() Event {
			e := lifecycleEvent(Pause, core.Scope{Kind: core.ScopeRun, ID: "OTHER"}, core.Working)
			return e
		}(), wantErr: core.ErrTransition},
		{name: "unknown event rejected", state: core.Working, event: func() Event { e := lifecycleEvent(EventKind("bogus"), scopeTask, core.Working); return e }(), wantErr: core.ErrTransition},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Transition(tc.state, tc.event)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("error = %v, want %v", err, tc.wantErr)
				}
				return
			}
			if err != nil || got != tc.want {
				t.Fatalf("got (%s, %v), want (%s, nil)", got, err, tc.want)
			}
		})
	}
}

func TestTransitionRejectsInvalidCheckpointCombinations(t *testing.T) {
	base := lifecycleEvent(EventCheckpoint, core.Scope{Kind: core.ScopeTask, ID: "TASK-1"}, core.Working)
	for _, tc := range []struct {
		name   string
		mutate func(*Event)
	}{
		{name: "missing digest", mutate: func(e *Event) { e.CheckpointDigest = "" }},
		{name: "missing revision", mutate: func(e *Event) { e.Revision = 0; e.CheckpointDigest = testDigest }},
		{name: "invalid target", mutate: func(e *Event) { e.To = core.Paused; e.CheckpointDigest = testDigest; e.Revision = 1 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := base
			tc.mutate(&e)
			if _, err := Transition(core.Working, e); !errors.Is(err, core.ErrTransition) {
				t.Fatalf("error = %v, want ErrTransition", err)
			}
		})
	}
}

func TestCheckpointAtomicIdempotentAndConflictingDigest(t *testing.T) {
	root := t.TempDir()
	s := store.New(root, core.StorageLimits{CanonicalBytes: 16 << 20})
	scope := core.Scope{Kind: core.ScopeTask, ID: "TASK-1"}
	ctx := context.Background()
	if err := Checkpoint(ctx, s, "RUN-1", scope, testDigest); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, ".agent-team", "checkpoints", "RUN-1", "task-TASK-1.json")
	first, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := Checkpoint(ctx, s, "RUN-1", scope, testDigest); err != nil {
		t.Fatal(err)
	}
	second, err := os.ReadFile(path)
	if err != nil || !reflect.DeepEqual(first, second) {
		t.Fatalf("repeat changed checkpoint: %v", err)
	}
	if err := Checkpoint(ctx, s, "RUN-1", scope, otherTestDigest); !errors.Is(err, core.ErrRevision) {
		t.Fatalf("conflicting digest error = %v, want ErrRevision", err)
	}
}

func TestCheckpointContextCancellationDoesNotWrite(t *testing.T) {
	s := store.New(t.TempDir(), core.StorageLimits{CanonicalBytes: 16 << 20})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := Checkpoint(ctx, s, "RUN-1", core.Scope{Kind: core.ScopeRun, ID: "RUN-1"}, testDigest); !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
	if _, err := os.Stat(filepath.Join(s.Root, ".agent-team")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("cancelled checkpoint wrote state: %v", err)
	}
}

func TestCheckpointUpdatesExistingApplicableReceiptOnly(t *testing.T) {
	s := store.New(t.TempDir(), core.StorageLimits{CanonicalBytes: 16 << 20})
	receipt := knowledge.Receipt{
		RecordEnvelope: core.RecordEnvelope{Schema: 1, Project: "project", RunID: "RUN-1", WrittenAt: time.Now().UTC().Format(time.RFC3339Nano), Revision: 1},
		Team:           "TEAM-1", Task: "TASK-1", Attempt: 1, State: core.Implementing, NextAction: "continue",
	}
	if err := knowledge.WriteReceipt(context.Background(), s, receipt); err != nil {
		t.Fatal(err)
	}
	if err := Checkpoint(context.Background(), s, "RUN-1", core.Scope{Kind: core.ScopeTeam, ID: "TEAM-1"}, testDigest); err != nil {
		t.Fatal(err)
	}
	var got knowledge.Receipt
	if err := s.ReadJSON(".agent-team/receipts/TEAM-1.json", 16<<20, &got); err != nil {
		t.Fatal(err)
	}
	if got.Revision != 2 || len(got.EvidencePointers) != 1 {
		t.Fatalf("receipt = %#v, want revision 2 and one checkpoint pointer", got)
	}
	if err := Checkpoint(context.Background(), s, "RUN-1", core.Scope{Kind: core.ScopeTask, ID: "TASK-2"}, otherTestDigest); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(s.Root, ".agent-team", "receipts", "TASK-2.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("checkpoint fabricated receipt: %v", err)
	}
}

func TestCheckpointConcurrentIdenticalWriters(t *testing.T) {
	s := store.New(t.TempDir(), core.StorageLimits{CanonicalBytes: 16 << 20})
	scope := core.Scope{Kind: core.ScopeRun, ID: "RUN-1"}
	var wg sync.WaitGroup
	errs := make(chan error, 16)
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- Checkpoint(context.Background(), s, "RUN-1", scope, testDigest)
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestCheckpointRejectsUnsafeSegments(t *testing.T) {
	s := store.New(t.TempDir(), core.StorageLimits{CanonicalBytes: 16 << 20})
	for _, tc := range []struct {
		name  string
		run   core.RunID
		scope core.Scope
	}{
		{name: "run traversal", run: "../RUN", scope: core.Scope{Kind: core.ScopeRun, ID: "RUN"}},
		{name: "scope traversal", run: "RUN", scope: core.Scope{Kind: core.ScopeTask, ID: "../TASK"}},
		{name: "empty scope", run: "RUN", scope: core.Scope{Kind: core.ScopeTask}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := Checkpoint(context.Background(), s, tc.run, tc.scope, testDigest); !errors.Is(err, core.ErrPath) {
				t.Fatalf("error = %v, want ErrPath", err)
			}
		})
	}
}
