package acceptance_test

import (
	"errors"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/thebpandey/agent-team/vnext/internal/cli"
	"github.com/thebpandey/agent-team/vnext/internal/contracts"
	"github.com/thebpandey/agent-team/vnext/internal/core"
	"github.com/thebpandey/agent-team/vnext/internal/project"
	"github.com/thebpandey/agent-team/vnext/internal/resources"
	"github.com/thebpandey/agent-team/vnext/internal/testkit"
)

func TestPlannedCapacityReviewerReservation(t *testing.T) {
	cases := []struct {
		name  string
		caps  contracts.HostCapabilities
		teams int
		want  error
	}{
		{"zero", contracts.HostCapabilities{}, 1, core.ErrCapacity},
		{"one sequential", contracts.HostCapabilities{ConfiguredSlots: 1, UsableSlots: 1, DeveloperSlots: 1, ReviewerSlots: 1}, 1, nil},
		{"two reserved", contracts.HostCapabilities{ConfiguredSlots: 2, UsableSlots: 2, DeveloperSlots: 1, ReviewerSlots: 1}, 2, nil},
		{"missing reviewer", contracts.HostCapabilities{ConfiguredSlots: 2, UsableSlots: 2, DeveloperSlots: 2}, 2, core.ErrCapacity},
		{"too many reviewers", contracts.HostCapabilities{ConfiguredSlots: 2, UsableSlots: 2, DeveloperSlots: 1, ReviewerSlots: 2}, 2, core.ErrCapacity},
		{"unknown serial", contracts.HostCapabilities{Unknown: true, DeveloperSlots: 1, ReviewerSlots: 1}, 1, nil},
		{"unknown parallel", contracts.HostCapabilities{Unknown: true, DeveloperSlots: 2, ReviewerSlots: 1}, 2, core.ErrCapacity},
		{"configured three", contracts.HostCapabilities{UsableSlots: 3, DeveloperSlots: 1, ReviewerSlots: 1}, 3, core.ErrCapacity},
	}
	for _, tc := range cases {
		if err := resources.ValidatePlannedAdmission(core.Limits{ParallelTeams: 2}, tc.caps, tc.teams); !errors.Is(err, tc.want) {
			t.Fatalf("%s: %v", tc.name, err)
		}
	}
}

func TestPlannedCapacityRejectsOversizedOneOff(t *testing.T) {
	caps := contracts.HostCapabilities{UsableSlots: 2, DeveloperSlots: 1, ReviewerSlots: 1}
	if err := resources.ValidatePlannedAdmission(core.Limits{ParallelTeams: 2}, caps, 3); !errors.Is(err, core.ErrCapacity) {
		t.Fatal(err)
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

func TestNativePathAndSharingCases(t *testing.T) {
	switch runtime.GOOS {
	case "windows":
		if err := testkit.WindowsSharingReplacement(); err != nil {
			t.Fatal(err)
		}
	case "linux", "darwin":
		if err := testkit.PosixContainmentAndAtomicRename(); err != nil {
			t.Fatal(err)
		}
		root := t.TempDir()
		if _, err := project.Contain(root, filepath.Join(root, "..", "outside")); !errors.Is(err, core.ErrPath) {
			t.Fatal(err)
		}
	default:
		t.Skip("unsupported native OS")
	}
}
