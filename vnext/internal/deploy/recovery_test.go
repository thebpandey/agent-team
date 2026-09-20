package deploy

import (
	"context"
	"errors"
	"testing"

	"github.com/thebpandey/agent-team/vnext/internal/core"
	"github.com/thebpandey/agent-team/vnext/internal/knowledge"
)

type fakeBlockers struct{ items []knowledge.Blocker }

func (f *fakeBlockers) Create(_ context.Context, blocker knowledge.Blocker) (knowledge.Blocker, error) {
	blocker.ID = "block-1"
	f.items = append(f.items, blocker)
	return blocker, nil
}

type fakeHolds struct{ calls []string }

func (f *fakeHolds) HoldLater(_ context.Context, _ core.RunID, batch string) error {
	f.calls = append(f.calls, batch)
	return nil
}

type fakeDashboard struct{ err error }

func (f fakeDashboard) Trigger(context.Context, DashboardTriggerRequest) error { return f.err }

func TestRecoveryPersistsBlockerHoldAndSurvivesDashboardFailure(t *testing.T) {
	blockers, holds := &fakeBlockers{}, &fakeHolds{}
	service := RecoveryService{Blockers: blockers, Holds: holds, Dashboard: fakeDashboard{err: errors.New("dashboard unavailable")}}
	id, err := service.RecordState(context.Background(), DeploymentReceipt{RunID: "R-1", BatchID: "B-1", State: "failed", CodingMayContinue: true})
	if err != nil || id == "" || len(blockers.items) != 1 || len(holds.calls) != 1 || blockers.items[0].EvidencePointer != "" {
		t.Fatal(id, err, blockers, holds)
	}
	if err := ValidateRollbackAuthorization(false, ""); !errors.Is(err, core.ErrSettings) {
		t.Fatal(err)
	}
}
