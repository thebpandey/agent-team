package gate_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"reflect"
	"sort"
	"testing"
	"time"

	"github.com/thebpandey/agent-team/vnext/internal/contracts"
	"github.com/thebpandey/agent-team/vnext/internal/core"
	"github.com/thebpandey/agent-team/vnext/internal/gate"
	"github.com/thebpandey/agent-team/vnext/internal/knowledge"
	"github.com/thebpandey/agent-team/vnext/internal/run"
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

func TestDefaultGateRejectsCallerDownscopedCanonicalChecks(t *testing.T) {
	state := store.New(t.TempDir(), core.StorageLimits{CanonicalBytes: 16 << 20})
	input := canonicalDurableInput(t, state, []core.Check{{Name: "first", Command: []string{"first"}}, {Name: "second", Command: []string{"second"}}}, []string{"first"})
	if _, err := gate.NewGate(nil, state).Check(context.Background(), input); !errors.Is(err, core.ErrRevision) {
		t.Fatalf("downscoped canonical checks error = %v, want ErrRevision", err)
	}
}

func canonicalDurableInput(t *testing.T, state *store.Store, checks []core.Check, evidenceChecks []string) contracts.GateInput {
	t.Helper()
	ctx := context.Background()
	manifest, err := run.CreateOneOff(ctx, t.TempDir(), run.Feature, "objective", []core.Task{{ID: "TASK", Objective: "objective", State: core.Ready, Criteria: []string{"criterion"}, Checks: checks, WritablePaths: []string{"vnext"}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := run.NewRepositories(state).Runs.Initialize(ctx, manifest); err != nil {
		t.Fatal(err)
	}
	if evidenceChecks == nil {
		evidenceChecks = testCheckFingerprints(checks)
	}
	envelope := core.RecordEnvelope{Schema: 1, Project: manifest.Project, RunID: manifest.ID, WrittenAt: "2026-09-19T00:00:00Z", Revision: 1}
	pointers := make([]string, 0, len(evidenceChecks))
	for attempt, name := range evidenceChecks {
		evidence := knowledge.Evidence{RecordEnvelope: envelope, Task: "TASK", Attempt: attempt + 1, Exit: 0, InputFingerprint: name}
		if err := knowledge.WriteEvidence(ctx, state, evidence); err != nil {
			t.Fatal(err)
		}
		pointers = append(pointers, ".agent-team/evidence/TASK/"+string(rune('1'+attempt))+"/evidence.json")
	}
	receipt := knowledge.Receipt{RecordEnvelope: envelope, Team: "TEAM", Task: "TASK", Attempt: 1, State: core.Clean, Base: "base", Head: "candidate", Review: "review-digest", EvidencePointers: pointers, NextAction: "integrate"}
	if err := knowledge.WriteReceipt(ctx, state, receipt); err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(receipt)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(encoded)
	return contracts.GateInput{Run: manifest.ID, Task: "TASK", Candidate: contracts.Candidate{Task: "TASK", Revision: "candidate", Base: "base", Worktree: contracts.Worktree{Run: manifest.ID, Team: "TEAM", Path: "/tmp/task", Branch: "branch", Base: "base", Candidate: "candidate"}}, TrackerRevision: manifest.TrackerRevision, ReceiptRevision: 1, RequiredCheckFingerprints: evidenceChecks, ScopeFingerprint: "scope", ReceiptDigest: "sha256:" + hex.EncodeToString(sum[:]), ReviewDigest: "review-digest"}
}

func durableInput(t *testing.T, state *store.Store) contracts.GateInput {
	t.Helper()
	return canonicalDurableInput(t, state, []core.Check{{Name: "check", Command: []string{"check"}}}, nil)
}

func testCheckFingerprints(checks []core.Check) []string {
	result := make([]string, len(checks))
	for index, check := range checks {
		encoded, _ := json.Marshal(check)
		sum := sha256.Sum256(encoded)
		result[index] = "sha256:" + hex.EncodeToString(sum[:])
	}
	sort.Strings(result)
	return result
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
