package orchestrator_test

import (
	"context"
	"errors"
	"testing"

	"github.com/thebpandey/agent-team/vnext/internal/contracts"
	"github.com/thebpandey/agent-team/vnext/internal/core"
	"github.com/thebpandey/agent-team/vnext/internal/orchestrator"
	"github.com/thebpandey/agent-team/vnext/internal/store"
)

type recordingDispatcher struct{ calls int }

func (d *recordingDispatcher) Dispatch(context.Context, core.AssignmentPacket, contracts.WorktreeSpec) (contracts.WorkerHandle, error) {
	d.calls++
	return contracts.WorkerHandle{Identity: "worker"}, nil
}

func TestForegroundControlFailsClosedWithoutCanonicalRun(t *testing.T) {
	o := orchestrator.New(store.New(t.TempDir(), core.StorageLimits{CanonicalBytes: 16 << 20}), nil, nil, nil, nil, nil, nil)
	if err := o.Start(context.Background(), "R1"); !errors.Is(err, core.ErrRevision) {
		t.Fatal(err)
	}
	if err := o.Execute(context.Background(), "../bad"); !errors.Is(err, core.ErrRevision) {
		t.Fatal(err)
	}
}

func TestDispatchPlannedRejectsBeforeWorkerStart(t *testing.T) {
	dispatcher := &recordingDispatcher{}
	o := orchestrator.New(nil, nil, dispatcher, nil, nil, nil, nil)
	caps := contracts.HostCapabilities{UsableSlots: 2, DeveloperSlots: 1, ReviewerSlots: 1}
	_, err := o.DispatchPlanned(context.Background(), core.Limits{ParallelTeams: 2}, caps, 3, core.AssignmentPacket{}, contracts.WorktreeSpec{})
	if !errors.Is(err, core.ErrCapacity) || dispatcher.calls != 0 {
		t.Fatalf("err=%v calls=%d", err, dispatcher.calls)
	}
	if _, err := o.DispatchPlanned(context.Background(), core.Limits{ParallelTeams: 2}, caps, 2, core.AssignmentPacket{}, contracts.WorktreeSpec{}); err != nil || dispatcher.calls != 1 {
		t.Fatalf("err=%v calls=%d", err, dispatcher.calls)
	}
}
