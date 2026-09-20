package handoff_test

import (
	"context"
	"errors"
	"fmt"
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

func TestDeriveIncludesResourcesWithoutReceiptAndKeepsInspect(t *testing.T) {
	s, r := twoTeamRun(t)
	writeReceipt(t, s, r, r.Teams[0], core.Paused, "resume")

	got, err := handoff.New(s).Derive(context.Background(), r.ID)
	if err != nil {
		t.Fatal(err)
	}
	wantResource := r.Teams[1].Resources[0]
	if !strings.Contains(string(got), wantResource) || !strings.Contains(string(got), "- Next action: inspect") {
		t.Fatalf("handoff=%q", got)
	}
}

func TestDeriveDoesNotResumeBlockedTeam(t *testing.T) {
	s, r := twoTeamRun(t)
	writeReceipt(t, s, r, r.Teams[0], core.Paused, "resume")
	writeReceipt(t, s, r, r.Teams[1], core.Blocked, "resume")

	got, err := handoff.New(s).Derive(context.Background(), r.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "- Next action: inspect") {
		t.Fatalf("handoff=%q", got)
	}
}

func TestDeriveRejectsInvalidExistingReceipt(t *testing.T) {
	s, r := twoTeamRun(t)
	team := r.Teams[0]
	receipt := knowledge.Receipt{RecordEnvelope: r.RecordEnvelope, Team: string(team.ID), Task: string(team.Queue[0]), Attempt: 1, State: core.Paused, NextAction: "resume"}
	receipt.Project = "different-project"
	if _, err := s.WriteJSON(".agent-team/receipts/"+string(team.ID)+".json", receipt, 1<<20); err != nil {
		t.Fatal(err)
	}

	if _, err := handoff.New(s).Derive(context.Background(), r.ID); !errors.Is(err, core.ErrRevision) {
		t.Fatalf("Derive() error = %v, want ErrRevision", err)
	}
}

func twoTeamRun(t *testing.T) (*store.Store, run.Run) {
	t.Helper()
	tasks := make([]core.Task, 9)
	for i := range tasks {
		id := fmt.Sprintf("T%02d", i+1)
		tasks[i] = core.Task{ID: core.TaskID(id), Objective: id, State: core.Ready, Criteria: []string{"done"}, WritablePaths: []string{"src/" + id}, Resources: []string{"resource-" + id}}
	}
	s := store.New(t.TempDir(), core.StorageLimits{CanonicalBytes: 16 << 20})
	r, err := run.CreateOneOff(context.Background(), t.TempDir(), run.OneOffFeature, "x", tasks)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Teams) != 2 {
		t.Fatalf("teams=%d, want 2", len(r.Teams))
	}
	if _, err := run.NewRepositories(s).Runs.Initialize(context.Background(), r); err != nil {
		t.Fatal(err)
	}
	return s, r
}

func writeReceipt(t *testing.T, s *store.Store, r run.Run, team run.TeamRecord, state core.TaskState, next string) {
	t.Helper()
	receipt := knowledge.Receipt{RecordEnvelope: r.RecordEnvelope, Team: string(team.ID), Task: string(team.Queue[0]), Attempt: 1, State: state, NextAction: next}
	if err := knowledge.WriteReceipt(context.Background(), s, receipt); err != nil {
		t.Fatal(err)
	}
}
