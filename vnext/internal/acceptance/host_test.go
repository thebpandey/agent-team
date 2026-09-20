package acceptance_test

import (
	"errors"
	"testing"

	"github.com/thebpandey/agent-team/vnext/internal/cli"
	"github.com/thebpandey/agent-team/vnext/internal/contracts"
	"github.com/thebpandey/agent-team/vnext/internal/core"
	"github.com/thebpandey/agent-team/vnext/internal/resources"
)

func TestPlannedCapacityReviewerReservation(t *testing.T) {
	cases := []struct {
		name string
		caps contracts.HostCapabilities
		want error
	}{
		{"zero", contracts.HostCapabilities{}, core.ErrCapacity},
		{"one sequential", contracts.HostCapabilities{ConfiguredSlots: 1, UsableSlots: 1, DeveloperSlots: 1, ReviewerSlots: 1}, nil},
		{"two reserved", contracts.HostCapabilities{ConfiguredSlots: 2, UsableSlots: 2, DeveloperSlots: 1, ReviewerSlots: 1}, nil},
		{"missing reviewer", contracts.HostCapabilities{ConfiguredSlots: 2, UsableSlots: 2, DeveloperSlots: 2}, core.ErrCapacity},
		{"too many reviewers", contracts.HostCapabilities{ConfiguredSlots: 2, UsableSlots: 2, DeveloperSlots: 1, ReviewerSlots: 2}, core.ErrCapacity},
		{"unknown serial", contracts.HostCapabilities{Unknown: true, DeveloperSlots: 1, ReviewerSlots: 1}, nil},
		{"unknown parallel", contracts.HostCapabilities{Unknown: true, DeveloperSlots: 2, ReviewerSlots: 1}, core.ErrCapacity},
	}
	for _, tc := range cases {
		if err := resources.ValidatePlannedAdmission(tc.caps); !errors.Is(err, tc.want) {
			t.Fatalf("%s: %v", tc.name, err)
		}
	}
}

func TestHostAcceptanceMarksLifecycleScopeUnresolved(t *testing.T) {
	for _, args := range [][]string{{"pause"}, {"stop", "--run", "RUN-1"}, {"cancel", "--task", "TASK-1"}, {"resume", "--team", "TEAM-1"}} {
		action, err := cli.Parse(args)
		if err != nil || !action.ScopeRequired {
			t.Fatalf("args=%v action=%+v err=%v", args, action, err)
		}
	}
}
