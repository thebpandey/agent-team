package acceptance_test

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
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
	f := scriptedVertical{developer: developer, reviewer: "developer-host", orchestrator: orchestrator.New(nil, nil, developer, nil, nil, nil, nil)}
	if _, err := f.orchestrator.DispatchPlanned(context.Background(), core.Limits{ParallelTeams: 2}, contracts.HostCapabilities{UsableSlots: 2, DeveloperSlots: 1, ReviewerSlots: 1}, 3, core.AssignmentPacket{}, contracts.WorktreeSpec{}); !errors.Is(err, core.ErrCapacity) || developer.calls != 0 {
		t.Fatalf("rejected dispatch err=%v calls=%d", err, developer.calls)
	}
	plans := []scriptedPlan{
		{
			packet:   core.AssignmentPacket{RecordEnvelope: core.RecordEnvelope{RunID: "RUN-1"}, Team: "TEAM-1", Task: "TASK-1", SpecRevision: "rev-1", QueueFingerprint: "packet-1"},
			worktree: contracts.WorktreeSpec{Run: "RUN-1", Team: "TEAM-1", Root: "task-1", Base: "base-1", WritablePaths: []string{"src/one"}},
		},
		{
			packet:   core.AssignmentPacket{RecordEnvelope: core.RecordEnvelope{RunID: "RUN-2"}, Team: "TEAM-2", Task: "TASK-2", SpecRevision: "rev-2", QueueFingerprint: "packet-2"},
			worktree: contracts.WorktreeSpec{Run: "RUN-2", Team: "TEAM-2", Root: "task-2", Base: "base-2", WritablePaths: []string{"src/two"}},
		},
	}
	for _, plan := range plans {
		if err := f.execute(plan.packet, plan.worktree); err != nil {
			t.Fatal(err)
		}
	}
	want := []string{"developer", "FIX", "developer:repaired", "CLEAN", "gate", "integrate", "cleanup"}
	if len(f.events) != 14 {
		t.Fatal(f)
	}
	for i, event := range want {
		if f.events[i] != event || f.events[i+len(want)] != event {
			t.Fatal(f.events)
		}
	}
	if len(developer.packets) != len(plans) || len(developer.worktrees) != len(plans) {
		t.Fatalf("dispatches packets=%d worktrees=%d", len(developer.packets), len(developer.worktrees))
	}
	for i, plan := range plans {
		if assignmentIdentity(f.developer.host, "developer", plan.packet) == assignmentIdentity(f.reviewer, "reviewer", plan.packet) {
			t.Fatalf("dispatch %d reused the developer identity", i)
		}
		if !reflect.DeepEqual(developer.packets[i], plan.packet) || !reflect.DeepEqual(developer.worktrees[i], plan.worktree) {
			t.Fatalf("dispatch %d packet=%+v worktree=%+v", i, developer.packets[i], developer.worktrees[i])
		}
	}
}

type scriptedPlan struct {
	packet   core.AssignmentPacket
	worktree contracts.WorktreeSpec
}

type scriptedVertical struct {
	developer    *scriptedDeveloper
	reviewer     string
	orchestrator orchestrator.Orchestrator
	events       []string
}

func (s *scriptedVertical) execute(p core.AssignmentPacket, worktree contracts.WorktreeSpec) error {
	developerIdentity := assignmentIdentity(s.developer.host, "developer", p)
	reviewerIdentity := assignmentIdentity(s.reviewer, "reviewer", p)
	if developerIdentity == reviewerIdentity || p.Task == "" || p.SpecRevision == "" {
		return core.ErrTransition
	}
	s.developer.events = &s.events
	expectedRepaired := p.SpecRevision + ":repaired"
	reviewer := scriptedReviewer{identity: reviewerIdentity, author: developerIdentity, initial: p.SpecRevision, repaired: expectedRepaired, events: &s.events}
	gate := scriptedGate{expected: expectedRepaired, events: &s.events}
	integrator := scriptedIntegrator{expected: expectedRepaired, events: &s.events}
	cleaner := scriptedCleaner{expected: expectedRepaired, events: &s.events}
	handle, err := s.orchestrator.DispatchPlanned(context.Background(), core.Limits{ParallelTeams: 2}, contracts.HostCapabilities{UsableSlots: 2, DeveloperSlots: 1, ReviewerSlots: 1}, 2, p, worktree)
	if err != nil {
		return err
	}
	revision := handle.CandidateRevision
	if handle.Identity != developerIdentity || revision != p.SpecRevision {
		return core.ErrRevision
	}
	if reviewer.review(revision) != "FIX" {
		return core.ErrTransition
	}
	revision, err = s.developer.repair(revision)
	if err != nil {
		return err
	}
	if reviewer.review(revision) != "CLEAN" || reviewer.identity == handle.Identity {
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
	host      string
	events    *[]string
	calls     int
	packets   []core.AssignmentPacket
	worktrees []contracts.WorktreeSpec
}

func (d *scriptedDeveloper) Dispatch(_ context.Context, p core.AssignmentPacket, worktree contracts.WorktreeSpec) (contracts.WorkerHandle, error) {
	d.calls++
	d.packets = append(d.packets, p)
	d.worktrees = append(d.worktrees, worktree)
	*d.events = append(*d.events, "developer")
	return contracts.WorkerHandle{Host: d.host, Identity: assignmentIdentity(d.host, "developer", p), Run: p.RunID, Team: p.Team, Task: p.Task, CandidateRevision: p.SpecRevision, PacketDigest: p.QueueFingerprint}, nil
}

func (d *scriptedDeveloper) repair(revision string) (string, error) {
	if revision == "" {
		return "", core.ErrRevision
	}
	*d.events = append(*d.events, "developer:repaired")
	return revision + ":repaired", nil
}

type scriptedReviewer struct {
	identity string
	author   string
	initial  string
	repaired string
	events   *[]string
	calls    int
}

func (r *scriptedReviewer) review(revision string) string {
	if r.identity == "" || r.identity == r.author || (r.calls == 0 && revision != r.initial) || (r.calls == 1 && revision != r.repaired) || r.calls > 1 {
		return ""
	}
	r.calls++
	if r.calls == 1 {
		*r.events = append(*r.events, "FIX")
		return "FIX"
	}
	*r.events = append(*r.events, "CLEAN")
	return "CLEAN"
}

func TestScriptedCollaboratorsBindAuthorAndRepairedCandidate(t *testing.T) {
	events := []string{}
	reviewer := scriptedReviewer{identity: "reviewer", author: "developer", initial: "base", repaired: "base:repaired", events: &events}
	if reviewer.review("other-nonempty-candidate") != "" {
		t.Fatal("review accepted an arbitrary candidate")
	}
	reviewer.identity = "developer"
	if reviewer.review("base") != "" {
		t.Fatal("review accepted the author identity")
	}
	for _, check := range []struct {
		name string
		fn   func(string) error
	}{
		{"gate", scriptedGate{expected: "base:repaired", events: &events}.check},
		{"integrate", scriptedIntegrator{expected: "base:repaired", events: &events}.integrate},
		{"cleanup", scriptedCleaner{expected: "base:repaired", events: &events}.cleanup},
	} {
		if err := check.fn("other-nonempty-candidate"); !errors.Is(err, core.ErrRevision) {
			t.Fatalf("%s accepted an arbitrary candidate: %v", check.name, err)
		}
	}
}

type scriptedGate struct {
	expected string
	events   *[]string
}

func (g scriptedGate) check(revision string) error {
	if revision == "" || revision != g.expected {
		return core.ErrRevision
	}
	*g.events = append(*g.events, "gate")
	return nil
}

type scriptedIntegrator struct {
	expected string
	events   *[]string
}

func (i scriptedIntegrator) integrate(revision string) error {
	if revision == "" || revision != i.expected {
		return core.ErrRevision
	}
	*i.events = append(*i.events, "integrate")
	return nil
}

type scriptedCleaner struct {
	expected string
	events   *[]string
}

func (c scriptedCleaner) cleanup(revision string) error {
	if revision == "" || revision != c.expected {
		return core.ErrRevision
	}
	*c.events = append(*c.events, "cleanup")
	return nil
}

func assignmentIdentity(host, role string, packet core.AssignmentPacket) string {
	return host + "/" + role + "/" + string(packet.RunID) + "/" + string(packet.Team) + "/" + string(packet.Task) + "/" + packet.QueueFingerprint
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
