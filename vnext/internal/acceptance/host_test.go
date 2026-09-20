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
	for _, tc := range []struct {
		caps  contracts.HostCapabilities
		teams int
		want  error
	}{
		{contracts.HostCapabilities{}, 1, core.ErrCapacity},
		{contracts.HostCapabilities{UsableSlots: 1, DeveloperSlots: 1, ReviewerSlots: 1}, 1, nil},
		{contracts.HostCapabilities{UsableSlots: 2, DeveloperSlots: 1, ReviewerSlots: 1}, 2, nil},
		{contracts.HostCapabilities{UsableSlots: 2, DeveloperSlots: 2}, 2, core.ErrCapacity},
		{contracts.HostCapabilities{Unknown: true, DeveloperSlots: 1, ReviewerSlots: 1}, 1, nil},
		{contracts.HostCapabilities{Unknown: true, DeveloperSlots: 2, ReviewerSlots: 1}, 2, core.ErrCapacity},
	} {
		if err := resources.ValidatePlannedAdmission(core.Limits{ParallelTeams: 2}, tc.caps, tc.teams); !errors.Is(err, tc.want) {
			t.Fatal(err)
		}
	}
}

func TestPlannedCapacityUsesConfiguredAvailability(t *testing.T) {
	caps := contracts.HostCapabilities{UsableSlots: 2, DeveloperSlots: 1, ReviewerSlots: 1}
	if err := resources.ValidatePlannedAdmission(core.DefaultConfig().Limits, caps, 2); err != nil {
		t.Fatal(err)
	}
	for _, limits := range []core.Limits{{}, {ParallelTeams: -1}} {
		if err := resources.ValidatePlannedAdmission(limits, caps, 1); !errors.Is(err, core.ErrCapacity) {
			t.Fatal(err)
		}
	}
	if err := resources.ValidatePlannedAdmission(core.Limits{ParallelTeams: 2}, caps, 3); !errors.Is(err, core.ErrCapacity) {
		t.Fatal(err)
	}
}

func TestHostAcceptanceMarksLifecycleScopeUnresolved(t *testing.T) {
	for _, args := range [][]string{{"pause"}, {"stop", "--run", "RUN-1"}, {"cancel", "--task", "TASK-1"}, {"resume", "--team", "TEAM-1"}} {
		action, err := cli.Parse(args)
		if err != nil || !action.ScopeRequired {
			t.Fatal(args, action, err)
		}
	}
}

func TestTwoWorkerExecutionReviewGateIntegration(t *testing.T) {
	f := scriptedVertical{developer: "developer-host", reviewer: "reviewer-host"}
	for _, p := range []core.AssignmentPacket{{Task: "TASK-1", SpecRevision: "rev-1"}, {Task: "TASK-2", SpecRevision: "rev-2"}} {
		if err := f.execute(p); err != nil {
			t.Fatal(err)
		}
	}
	want := []string{"developer", "FIX", "developer:repaired", "CLEAN", "gate", "integrate", "cleanup"}
	if f.developer == f.reviewer || len(f.events) != 14 {
		t.Fatal(f)
	}
	for i, event := range want {
		if f.events[i] != event || f.events[i+len(want)] != event {
			t.Fatal(f.events)
		}
	}
}

type scriptedVertical struct {
	developer, reviewer string
	events              []string
}

func (s *scriptedVertical) execute(p core.AssignmentPacket) error {
	if s.developer == s.reviewer || p.Task == "" || p.SpecRevision == "" {
		return core.ErrTransition
	}
	s.events = append(s.events, "developer", "FIX", "developer:repaired", "CLEAN", "gate", "integrate", "cleanup")
	return nil
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
