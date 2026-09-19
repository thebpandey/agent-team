package resources_test

import (
	"errors"
	"testing"

	"github.com/thebpandey/agent-team/vnext/internal/contracts"
	"github.com/thebpandey/agent-team/vnext/internal/core"
	"github.com/thebpandey/agent-team/vnext/internal/resources"
)

func TestCapacityMatrix(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name         string
		capacities   []resources.Capacity
		reservations []resources.Reservation
		wantErr      bool
	}{
		{
			name:       "zero",
			capacities: []resources.Capacity{{Name: "worker", Effective: 0}},
			wantErr:    true,
		},
		{
			name:         "zero over-reserved",
			capacities:   []resources.Capacity{{Name: "worker", Effective: 0}},
			reservations: []resources.Reservation{{Name: "worker", Count: 1}},
			wantErr:      true,
		},
		{
			name:         "one",
			capacities:   []resources.Capacity{{Name: "worker", Effective: 1}},
			reservations: []resources.Reservation{{Name: "worker", Count: 1}},
			wantErr:      false,
		},
		{
			name:         "many",
			capacities:   []resources.Capacity{{Name: "worker", Effective: 3}},
			reservations: []resources.Reservation{{Name: "worker", Count: 2}},
			wantErr:      false,
		},
		{
			name:         "reserved plus requested",
			capacities:   []resources.Capacity{{Name: "worker", Effective: 3, Reserved: 1}},
			reservations: []resources.Reservation{{Name: "worker", Count: 2}},
			wantErr:      false,
		},
		{
			name:         "reserved plus requested over limit",
			capacities:   []resources.Capacity{{Name: "worker", Effective: 3, Reserved: 1}},
			reservations: []resources.Reservation{{Name: "worker", Count: 3}},
			wantErr:      true,
		},
		{
			name: "observed mismatch uses effective",
			capacities: []resources.Capacity{{
				Name:       "worker",
				Configured: 4,
				Observed:   1,
				Effective:  1,
			}},
			reservations: []resources.Reservation{{Name: "worker", Count: 2}},
			wantErr:      true,
		},
		{
			name:         "unknown",
			capacities:   []resources.Capacity{{Name: "reviewer", Unknown: true}},
			reservations: []resources.Reservation{{Name: "reviewer", Count: 1}},
			wantErr:      true,
		},
		{
			name:         "disposable reservation still consumes capacity",
			capacities:   []resources.Capacity{{Name: "worker", Effective: 1}},
			reservations: []resources.Reservation{{Name: "worker", Count: 1, State: "disposable"}},
			wantErr:      false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := resources.Reserve(tc.capacities, tc.reservations); (err != nil) != tc.wantErr {
				t.Fatalf("Reserve() error = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}

func TestReserveRejectsMalformedInputs(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name         string
		capacities   []resources.Capacity
		reservations []resources.Reservation
	}{
		{name: "empty capacity name", capacities: []resources.Capacity{{Effective: 1}}},
		{name: "duplicate capacity name", capacities: []resources.Capacity{{Name: "worker", Effective: 1}, {Name: "worker", Effective: 1}}},
		{name: "negative effective", capacities: []resources.Capacity{{Name: "worker", Effective: -1}}},
		{name: "negative reserved", capacities: []resources.Capacity{{Name: "worker", Effective: 1, Reserved: -1}}},
		{name: "empty reservation name", capacities: []resources.Capacity{{Name: "worker", Effective: 1}}, reservations: []resources.Reservation{{Count: 1}}},
		{name: "unknown resource", capacities: []resources.Capacity{{Name: "worker", Effective: 1}}, reservations: []resources.Reservation{{Name: "reviewer", Count: 1}}},
		{name: "negative reservation", capacities: []resources.Capacity{{Name: "worker", Effective: 1}}, reservations: []resources.Reservation{{Name: "worker", Count: -1}}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := resources.Reserve(tc.capacities, tc.reservations); !errors.Is(err, core.ErrCapacity) {
				t.Fatalf("Reserve() error = %v, want errors.Is(..., core.ErrCapacity)", err)
			}
		})
	}
}

func TestLaterPhasePortValuesArePassive(t *testing.T) {
	_ = contracts.HostCapabilities{Host: "test", OS: "windows", ConfiguredSlots: 2, ObservedSlots: 1, UsableSlots: 1, DeveloperSlots: 1, ReviewerSlots: 0, Servers: 2, Browsers: 2}
	_ = contracts.WorkerRequest{Packet: core.AssignmentPacket{RecordEnvelope: core.RecordEnvelope{Schema: 1}}, WritablePaths: []string{"src"}}
	_ = contracts.WorkerHandle{Run: "run", Team: "team", Task: "task", Attempt: 1}
	_ = contracts.WorktreeSpec{Run: "run", Team: "team", Root: "/tmp/root", Base: "HEAD"}
	_ = contracts.Worktree{Run: "run", Team: "team", Path: "/tmp/worktree", Branch: "task"}
	_ = contracts.Candidate{Task: "task", Revision: "HEAD", Worktree: contracts.Worktree{Path: "/tmp/worktree"}}
	_ = contracts.GateInput{Run: "run", Task: "task", Candidate: contracts.Candidate{Task: "task"}}
	_ = contracts.GateResult{Revision: "HEAD", Result: "CLEAN"}
	_ = contracts.DeploymentBatch{Run: "run", TaskIDs: []core.TaskID{"task"}, Revisions: []string{"HEAD"}}
	_ = contracts.Operation{Provider: "provider", State: "submitted"}
	_ = contracts.Verification{State: "passed", Exit: 0}
}
