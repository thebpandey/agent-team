package gate_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/thebpandey/agent-team/vnext/internal/contracts"
	"github.com/thebpandey/agent-team/vnext/internal/core"
	"github.com/thebpandey/agent-team/vnext/internal/gate"
	"github.com/thebpandey/agent-team/vnext/internal/knowledge"
	"github.com/thebpandey/agent-team/vnext/internal/store"
)

func TestGateRejectsDirtyAndPersistsDeterministicCleanEvidence(t *testing.T) {
	state := store.New(t.TempDir(), core.StorageLimits{CanonicalBytes: 16 << 20})
	g := gate.NewGateWithAuthority(nil, state, testAuthority{})
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
	g := gate.NewGateWithAuthority(nil, store.New(t.TempDir(), core.StorageLimits{CanonicalBytes: 16 << 20}), testAuthority{})
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
	g := gate.NewGateWithAuthority(nil, store.New(t.TempDir(), core.StorageLimits{CanonicalBytes: 16 << 20}), testAuthority{})
	if _, err := g.Check(ctx, validInput()); !errors.Is(err, core.ErrTransition) {
		t.Fatalf("interrupted Check() error = %v, want ErrTransition", err)
	}
}

func TestDefaultGateRejectsCallerOnlyEvidence(t *testing.T) {
	g := gate.NewGate(nil, store.New(t.TempDir(), core.StorageLimits{CanonicalBytes: 16 << 20}))
	if _, err := g.Check(context.Background(), validInput()); !errors.Is(err, core.ErrRevision) {
		t.Fatalf("Check without durable receipt/review/check evidence = %v, want ErrRevision", err)
	}
}

func TestDefaultGateValidatesDurableReceiptAndPassedChecks(t *testing.T) {
	state := store.New(t.TempDir(), core.StorageLimits{CanonicalBytes: 16 << 20})
	input := durableInput(t, state)
	if _, err := gate.NewGate(nil, state).Check(context.Background(), input); err != nil {
		t.Fatalf("durable gate error = %v", err)
	}
	var receipt knowledge.Receipt
	if err := state.ReadJSON(".agent-team/receipts/TEAM.json", 64<<10, &receipt); err != nil {
		t.Fatal(err)
	}
	receipt.Head = "tampered"
	if _, err := state.WriteJSON(".agent-team/receipts/TEAM.json", receipt, 64<<10); err != nil {
		t.Fatal(err)
	}
	if _, err := gate.NewGate(nil, state).Check(context.Background(), input); !errors.Is(err, core.ErrRevision) {
		t.Fatalf("tampered receipt error = %v, want ErrRevision", err)
	}
}

func durableInput(t *testing.T, state *store.Store) contracts.GateInput {
	t.Helper()
	envelope := core.RecordEnvelope{Schema: 1, Project: "project", RunID: "RUN", WrittenAt: "2026-09-19T00:00:00Z", Revision: 1}
	evidence := knowledge.Evidence{RecordEnvelope: envelope, Task: "TASK", Attempt: 1, Exit: 0, InputFingerprint: "check-a"}
	if err := knowledge.WriteEvidence(context.Background(), state, evidence); err != nil {
		t.Fatal(err)
	}
	receipt := knowledge.Receipt{RecordEnvelope: envelope, Team: "TEAM", Task: "TASK", Attempt: 1, State: core.Clean, Base: "base", Head: "candidate", Review: "review-digest", EvidencePointers: []string{".agent-team/evidence/TASK/1/evidence.json"}, NextAction: "integrate"}
	if err := knowledge.WriteReceipt(context.Background(), state, receipt); err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(receipt)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(encoded)
	input := validInput()
	input.RequiredCheckFingerprints = []string{"check-a"}
	input.ReceiptDigest = "sha256:" + hex.EncodeToString(sum[:])
	return input
}

type testAuthority struct{}

func (testAuthority) Validate(context.Context, contracts.GateInput) (core.RecordEnvelope, error) {
	return core.RecordEnvelope{Schema: 1, Project: "project", RunID: "RUN", Revision: 1, WrittenAt: time.Now().UTC().Format(time.RFC3339)}, nil
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
