package acceptance_test

import (
	"context"
	"errors"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/thebpandey/agent-team/vnext/internal/cli"
	"github.com/thebpandey/agent-team/vnext/internal/contracts"
	"github.com/thebpandey/agent-team/vnext/internal/core"
	"github.com/thebpandey/agent-team/vnext/internal/orchestrator"
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
	developer := &scriptedDeveloper{host: "developer-host"}
	f := scriptedVertical{developer: developer, reviewer: "reviewer-host", orchestrator: orchestrator.New(nil, nil, developer, nil, nil, nil, nil)}
	if _, err := f.orchestrator.DispatchPlanned(context.Background(), core.Limits{ParallelTeams: 2}, contracts.HostCapabilities{UsableSlots: 2, DeveloperSlots: 1, ReviewerSlots: 1}, 3, core.AssignmentPacket{}, contracts.WorktreeSpec{}); !errors.Is(err, core.ErrCapacity) || developer.calls != 0 {
		t.Fatalf("rejected dispatch err=%v calls=%d", err, developer.calls)
	}
	for _, p := range []core.AssignmentPacket{{Task: "TASK-1", SpecRevision: "rev-1"}, {Task: "TASK-2", SpecRevision: "rev-2"}} {
		if err := f.execute(p); err != nil {
			t.Fatal(err)
		}
	}
	want := []string{"developer", "FIX", "developer:repaired", "CLEAN", "gate", "integrate", "cleanup"}
	if f.developer.host == f.reviewer || len(f.events) != 14 {
		t.Fatal(f)
	}
	for i, event := range want {
		if f.events[i] != event || f.events[i+len(want)] != event {
			t.Fatal(f.events)
		}
	}
}

type scriptedVertical struct {
	developer    *scriptedDeveloper
	reviewer     string
	orchestrator orchestrator.Orchestrator
	events       []string
}

func (s *scriptedVertical) execute(p core.AssignmentPacket) error {
	if s.developer.host == s.reviewer || p.Task == "" || p.SpecRevision == "" {
		return core.ErrTransition
	}
	s.developer.events = &s.events
	reviewer := scriptedReviewer{host: s.reviewer, events: &s.events}
	gate := scriptedGate{events: &s.events}
	integrator := scriptedIntegrator{events: &s.events}
	cleaner := scriptedCleaner{events: &s.events}
	worktree := contracts.WorktreeSpec{Run: "RUN", Team: "TEAM", Root: "task", Base: "base", WritablePaths: []string{"src"}}
	handle, err := s.orchestrator.DispatchPlanned(context.Background(), core.Limits{ParallelTeams: 2}, contracts.HostCapabilities{UsableSlots: 2, DeveloperSlots: 1, ReviewerSlots: 1}, 2, p, worktree)
	if err != nil {
		return err
	}
	revision := handle.CandidateRevision
	if reviewer.review(revision) != "FIX" {
		return core.ErrTransition
	}
	revision = s.developer.repair(revision)
	if reviewer.review(revision) != "CLEAN" || reviewer.host == s.developer.host {
		return core.ErrTransition
	}
	if err := gate.check(revision); err != nil {
		return err
	}
	if err := integrator.integrate(revision); err != nil {
		return err
	}
	return cleaner.cleanup(revision)
}

type scriptedDeveloper struct {
	host   string
	events *[]string
	calls  int
}

func (d *scriptedDeveloper) Dispatch(_ context.Context, p core.AssignmentPacket, _ contracts.WorktreeSpec) (contracts.WorkerHandle, error) {
	d.calls++
	*d.events = append(*d.events, "developer")
	return contracts.WorkerHandle{Host: d.host, Identity: "developer", Run: p.RunID, Team: p.Team, Task: p.Task, CandidateRevision: p.SpecRevision, PacketDigest: p.QueueFingerprint}, nil
}

func (d *scriptedDeveloper) repair(revision string) string {
	*d.events = append(*d.events, "developer:repaired")
	return revision + ":repaired"
}

type scriptedReviewer struct {
	host   string
	events *[]string
	calls  int
}

func (r *scriptedReviewer) review(string) string {
	r.calls++
	if r.calls == 1 {
		*r.events = append(*r.events, "FIX")
		return "FIX"
	}
	*r.events = append(*r.events, "CLEAN")
	return "CLEAN"
}

type scriptedGate struct{ events *[]string }

func (g scriptedGate) check(revision string) error {
	if revision == "" {
		return core.ErrRevision
	}
	*g.events = append(*g.events, "gate")
	return nil
}

type scriptedIntegrator struct{ events *[]string }

func (i scriptedIntegrator) integrate(revision string) error {
	if revision == "" {
		return core.ErrRevision
	}
	*i.events = append(*i.events, "integrate")
	return nil
}

type scriptedCleaner struct{ events *[]string }

func (c scriptedCleaner) cleanup(revision string) error {
	if revision == "" {
		return core.ErrRevision
	}
	*c.events = append(*c.events, "cleanup")
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
