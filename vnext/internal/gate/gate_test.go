package gate_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/thebpandey/agent-team/vnext/internal/contracts"
	"github.com/thebpandey/agent-team/vnext/internal/core"
	"github.com/thebpandey/agent-team/vnext/internal/gate"
	"github.com/thebpandey/agent-team/vnext/internal/store"
)

func TestGateRejectsDirtyAndPersistsDeterministicCleanEvidence(t *testing.T) {
	state := store.New(t.TempDir(), core.StorageLimits{CanonicalBytes: 16 << 20})
	g := gate.NewGate(nil, state)
	dirty := validInput()
	dirty.WorktreeDirty = true
	if _, err := g.Check(context.Background(), dirty); !errors.Is(err, core.ErrTransition) {
		t.Fatalf("dirty candidate error = %v, want ErrTransition", err)
	}

	first, err := g.Check(context.Background(), validInput())
	if err != nil || first.Result != "CLEAN" || first.Revision != "candidate" || first.CleanEvidence != "review-digest" || first.Evidence == "" {
		t.Fatalf("first gate = %+v, %v", first, err)
	}
	if want := []string{"check-a", "check-b"}; !reflect.DeepEqual(first.CheckFingerprints, want) {
		t.Fatalf("check fingerprints = %#v, want %#v", first.CheckFingerprints, want)
	}
	second, err := g.Check(context.Background(), validInput())
	if err != nil || !reflect.DeepEqual(second, first) {
		t.Fatalf("idempotent gate = %+v, %v; want %+v", second, err, first)
	}
}

func TestGateRejectsUnboundOrMutatedInputs(t *testing.T) {
	g := gate.NewGate(nil, store.New(t.TempDir(), core.StorageLimits{CanonicalBytes: 16 << 20}))
	for _, mutate := range []struct {
		name string
		edit func(*contracts.GateInput)
		want error
	}{
		{"missing receipt", func(in *contracts.GateInput) { in.ReceiptDigest = "" }, core.ErrTransition},
		{"missing review", func(in *contracts.GateInput) { in.ReviewDigest = "" }, core.ErrTransition},
		{"zero receipt revision", func(in *contracts.GateInput) { in.ReceiptRevision = 0 }, core.ErrRevision},
		{"wrong task", func(in *contracts.GateInput) { in.Candidate.Task = "OTHER" }, core.ErrRevision},
		{"wrong run", func(in *contracts.GateInput) { in.Candidate.Worktree.Run = "OTHER" }, core.ErrRevision},
		{"wrong base", func(in *contracts.GateInput) { in.Candidate.Worktree.Base = "other-base" }, core.ErrRevision},
		{"mutated candidate", func(in *contracts.GateInput) { in.Candidate.Worktree.Candidate = "other-candidate" }, core.ErrRevision},
		{"duplicate check", func(in *contracts.GateInput) { in.RequiredCheckFingerprints = []string{"same", "same"} }, core.ErrRevision},
	} {
		t.Run(mutate.name, func(t *testing.T) {
			input := validInput()
			mutate.edit(&input)
			if _, err := g.Check(context.Background(), input); !errors.Is(err, mutate.want) {
				t.Fatalf("Check() error = %v, want %v", err, mutate.want)
			}
		})
	}
}

func TestGateRejectsInterruptedContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	g := gate.NewGate(nil, store.New(t.TempDir(), core.StorageLimits{CanonicalBytes: 16 << 20}))
	if _, err := g.Check(ctx, validInput()); !errors.Is(err, core.ErrTransition) {
		t.Fatalf("interrupted Check() error = %v, want ErrTransition", err)
	}
}

func validInput() contracts.GateInput {
	return contracts.GateInput{
		Run:             "RUN",
		Task:            "TASK",
		TrackerRevision: 1,
		ReceiptRevision: 1,
		RequiredCheckFingerprints: []string{
			"check-b", "check-a",
		},
		ScopeFingerprint: "scope-digest",
		ReceiptDigest:    "receipt-digest",
		ReviewDigest:     "review-digest",
		Candidate: contracts.Candidate{
			Task:     "TASK",
			Revision: "candidate",
			Base:     "base",
			Worktree: contracts.Worktree{Run: "RUN", Team: "TEAM", Path: "/tmp/task", Branch: "agent-team/RUN/TEAM", Base: "base", Candidate: "candidate"},
		},
	}
}
