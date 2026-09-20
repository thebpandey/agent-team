package handoff_test

import (
	"context"
	"strings"
	"testing"

	"github.com/thebpandey/agent-team/vnext/internal/core"
	"github.com/thebpandey/agent-team/vnext/internal/handoff"
	"github.com/thebpandey/agent-team/vnext/internal/knowledge"
	"github.com/thebpandey/agent-team/vnext/internal/run"
	"github.com/thebpandey/agent-team/vnext/internal/store"
)

func TestDeriveReadsCanonicalRunAndWritesDeterministicHandoff(t *testing.T) {
	s := store.New(t.TempDir(), core.StorageLimits{CanonicalBytes: 16 << 20})
	r, err := run.CreateOneOff(context.Background(), t.TempDir(), run.OneOffFeature, "x", []core.Task{{ID: "T1", Objective: "x", State: core.Ready, Criteria: []string{"done"}, WritablePaths: []string{"x"}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := run.NewRepositories(s).Runs.Initialize(context.Background(), r); err != nil {
		t.Fatal(err)
	}
	receipt := knowledge.Receipt{RecordEnvelope: core.RecordEnvelope{Schema: 1, Project: r.Project, RunID: r.ID, WrittenAt: r.WrittenAt, Revision: 1}, Team: string(r.Teams[0].ID), Task: "T1", Attempt: 1, State: core.Paused, Review: "review-1", Gate: "gate-1", EvidencePointers: []string{"evidence/one"}, NextAction: "resume"}
	if err := knowledge.WriteReceipt(context.Background(), s, receipt); err != nil {
		t.Fatal(err)
	}
	got, err := handoff.New(s).Derive(context.Background(), r.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), string(r.ID)) || !strings.Contains(string(got), "resume") || !strings.Contains(string(got), "evidence/one") {
		t.Fatalf("handoff=%q", got)
	}
	var again []byte
	if again, err = handoff.New(s).Derive(context.Background(), r.ID); err != nil || string(got) != string(again) {
		t.Fatalf("first=%q second=%q err=%v", got, again, err)
	}
}
