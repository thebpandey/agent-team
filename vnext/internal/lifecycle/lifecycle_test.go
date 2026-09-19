package lifecycle_test

import (
	"context"
	"errors"
	"testing"

	"github.com/thebpandey/agent-team/vnext/internal/contracts"
	"github.com/thebpandey/agent-team/vnext/internal/core"
	"github.com/thebpandey/agent-team/vnext/internal/lifecycle"
	"github.com/thebpandey/agent-team/vnext/internal/run"
	"github.com/thebpandey/agent-team/vnext/internal/store"
	"github.com/thebpandey/agent-team/vnext/internal/supervise"
	"github.com/thebpandey/agent-team/vnext/internal/workflow"
)

type supervisor struct {
	events      []workflow.Event
	checkpoints int
	err         error
	failAt      int
}

func packetFor(manifest run.Run) core.AssignmentPacket {
	return core.AssignmentPacket{RecordEnvelope: core.RecordEnvelope{RunID: manifest.ID}, Team: manifest.Teams[0].ID, Task: manifest.Teams[0].Queue[0]}
}

func (s *supervisor) Start(context.Context, core.AssignmentPacket) (contracts.WorkerHandle, error) {
	return contracts.WorkerHandle{}, nil
}
func (s *supervisor) Turn(context.Context, contracts.WorkerHandle) (supervise.Observation, error) {
	return "", nil
}
func (s *supervisor) Emit(_ context.Context, event workflow.Event) error {
	s.events = append(s.events, event)
	if s.err != nil && (s.failAt == 0 || len(s.events) == s.failAt) {
		return s.err
	}
	return nil
}
func (s *supervisor) Checkpoint(context.Context, core.RunID, core.Scope, string) error {
	s.checkpoints++
	return s.err
}

func TestScopedResumeIsDurableAndIdempotent(t *testing.T) {
	s := store.New(t.TempDir(), core.StorageLimits{CanonicalBytes: 16 << 20})
	manifest, err := run.CreateOneOff(context.Background(), t.TempDir(), run.OneOffFeature, "x", []core.Task{{ID: "T1", Objective: "x", State: core.Ready, Criteria: []string{"done"}, WritablePaths: []string{"x"}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := run.NewRepositories(s).Runs.Initialize(context.Background(), manifest); err != nil {
		t.Fatal(err)
	}
	supervisor := &supervisor{}
	l := lifecycle.New(s, supervisor)
	scope := core.Scope{Kind: core.ScopeTask, ID: "T1"}
	if err := l.Pause(context.Background(), scope, "user"); err != nil {
		t.Fatal(err)
	}
	if err := l.Pause(context.Background(), scope, "user"); err != nil {
		t.Fatal(err)
	}
	if err := l.Resume(context.Background(), scope); err != nil {
		t.Fatal(err)
	}
	if err := l.Resume(context.Background(), scope); err != nil {
		t.Fatal(err)
	}
}

func TestTaskScopeWithoutUniqueCanonicalMembershipFailsClosed(t *testing.T) {
	l := lifecycle.New(store.New(t.TempDir(), core.StorageLimits{CanonicalBytes: 16 << 20}), &supervisor{})
	if err := l.Pause(context.Background(), core.Scope{Kind: core.ScopeTask, ID: "T1"}, "user"); !errors.Is(err, core.ErrTransition) {
		t.Fatal(err)
	}
}

func TestTaskScopeWithAmbiguousCanonicalMembershipFailsClosed(t *testing.T) {
	s := store.New(t.TempDir(), core.StorageLimits{CanonicalBytes: 16 << 20})
	for _, objective := range []string{"x", "y"} {
		manifest, err := run.CreateOneOff(context.Background(), t.TempDir(), run.OneOffFeature, objective, []core.Task{{ID: "T1", Objective: objective, State: core.Ready, Criteria: []string{"done"}, WritablePaths: []string{"x"}}})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := run.NewRepositories(s).Runs.Initialize(context.Background(), manifest); err != nil {
			t.Fatal(err)
		}
	}
	if err := lifecycle.New(s, &supervisor{}).Pause(context.Background(), core.Scope{Kind: core.ScopeTask, ID: "T1"}, "user"); !errors.Is(err, core.ErrTransition) {
		t.Fatal(err)
	}
}

type lookup struct{ team, task bool }

func (l lookup) TeamMember(context.Context, core.RunID, core.TeamID) (bool, error) {
	return l.team, nil
}
func (l lookup) TaskMember(context.Context, core.RunID, core.TaskID) (bool, error) {
	return l.task, nil
}

func TestResolveScopeRejectsAmbiguityAndNonMembers(t *testing.T) {
	active := []core.RunID{"R1"}
	if got, err := lifecycle.ResolveScope(context.Background(), nil, active, lookup{}); err != nil || got != (core.Scope{Kind: core.ScopeRun, ID: "R1"}) {
		t.Fatalf("scope=%+v err=%v", got, err)
	}
	for _, active := range [][]core.RunID{nil, {"R1", "R2"}} {
		if _, err := lifecycle.ResolveScope(context.Background(), nil, active, lookup{}); !errors.Is(err, core.ErrTransition) {
			t.Fatalf("active=%v err=%v", active, err)
		}
	}
	if _, err := lifecycle.ResolveScope(context.Background(), []string{"--team", "TEAM"}, active, lookup{}); !errors.Is(err, core.ErrTransition) {
		t.Fatal(err)
	}
	if _, err := lifecycle.ResolveScope(context.Background(), []string{"--task", "TASK"}, active, lookup{task: true}); err != nil {
		t.Fatal(err)
	}
}

func TestCheckpointAndCancelFailClosedOnConflict(t *testing.T) {
	s := store.New(t.TempDir(), core.StorageLimits{CanonicalBytes: 16 << 20})
	manifest, err := run.CreateOneOff(context.Background(), t.TempDir(), run.OneOffFeature, "x", []core.Task{{ID: "T1", Objective: "x", State: core.Ready, Criteria: []string{"done"}, WritablePaths: []string{"x"}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := run.NewRepositories(s).Runs.Initialize(context.Background(), manifest); err != nil {
		t.Fatal(err)
	}
	supervisor := &supervisor{}
	l := lifecycle.New(s, supervisor)
	scope := core.Scope{Kind: core.ScopeRun, ID: string(manifest.ID)}
	digest := "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	if err := l.Checkpoint(context.Background(), scope, digest); err != nil {
		t.Fatal(err)
	}
	if err := l.Checkpoint(context.Background(), scope, digest); err != nil {
		t.Fatal(err)
	}
	if err := l.Checkpoint(context.Background(), scope, "sha256:abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"); !errors.Is(err, core.ErrRevision) {
		t.Fatal(err)
	}
	if err := l.Cancel(context.Background(), scope, "user"); err != nil {
		t.Fatal(err)
	}
	if err := l.Resume(context.Background(), scope); !errors.Is(err, core.ErrTransition) {
		t.Fatal(err)
	}
	if err := l.Pause(context.Background(), core.Scope{Kind: core.ScopeTask, ID: "../bad"}, "user"); !errors.Is(err, core.ErrTransition) {
		t.Fatal(err)
	}
}

func TestRunBarrierEmitsAndBlocksAdmission(t *testing.T) {
	s := store.New(t.TempDir(), core.StorageLimits{CanonicalBytes: 16 << 20})
	manifest, err := run.CreateOneOff(context.Background(), t.TempDir(), run.OneOffFeature, "x", []core.Task{{ID: "T1", Objective: "x", State: core.Ready, Criteria: []string{"done"}, WritablePaths: []string{"x"}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := run.NewRepositories(s).Runs.Initialize(context.Background(), manifest); err != nil {
		t.Fatal(err)
	}
	sup := &supervisor{}
	l := lifecycle.New(s, sup)
	scope := core.Scope{Kind: core.ScopeRun, ID: string(manifest.ID)}
	if err := l.Pause(context.Background(), scope, "user"); err != nil {
		t.Fatal(err)
	}
	if len(sup.events) != 1 || sup.events[0].Kind != workflow.Pause || sup.events[0].Run != manifest.ID {
		t.Fatalf("events=%+v", sup.events)
	}
	if err := lifecycle.AdmissionAllowed(context.Background(), s, packetFor(manifest)); !errors.Is(err, core.ErrTransition) {
		t.Fatal(err)
	}
	if err := l.Resume(context.Background(), scope); err != nil {
		t.Fatal(err)
	}
	if err := lifecycle.AdmissionAllowed(context.Background(), s, packetFor(manifest)); err != nil {
		t.Fatal(err)
	}
}

func TestSupervisorFailureDoesNotPublishBarrier(t *testing.T) {
	s := store.New(t.TempDir(), core.StorageLimits{CanonicalBytes: 16 << 20})
	manifest, err := run.CreateOneOff(context.Background(), t.TempDir(), run.OneOffFeature, "x", []core.Task{{ID: "T1", Objective: "x", State: core.Ready, Criteria: []string{"done"}, WritablePaths: []string{"x"}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := run.NewRepositories(s).Runs.Initialize(context.Background(), manifest); err != nil {
		t.Fatal(err)
	}
	l := lifecycle.New(s, &supervisor{err: core.ErrCapacity})
	if err := l.Pause(context.Background(), core.Scope{Kind: core.ScopeRun, ID: string(manifest.ID)}, "user"); !errors.Is(err, core.ErrCapacity) {
		t.Fatal(err)
	}
	if err := lifecycle.AdmissionAllowed(context.Background(), s, packetFor(manifest)); err != nil {
		t.Fatal(err)
	}
}

func TestScopedAdmissionRejectsTeamTaskAndForgedPacket(t *testing.T) {
	s := store.New(t.TempDir(), core.StorageLimits{CanonicalBytes: 16 << 20})
	manifest, err := run.CreateOneOff(context.Background(), t.TempDir(), run.OneOffFeature, "x", []core.Task{{ID: "T1", Objective: "x", State: core.Ready, Criteria: []string{"done"}, WritablePaths: []string{"x"}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := run.NewRepositories(s).Runs.Initialize(context.Background(), manifest); err != nil {
		t.Fatal(err)
	}
	packet := packetFor(manifest)
	l := lifecycle.New(s, &supervisor{})
	if err := l.Pause(context.Background(), core.Scope{Kind: core.ScopeTeam, ID: string(packet.Team)}, "user"); err != nil {
		t.Fatal(err)
	}
	if err := lifecycle.AdmissionAllowed(context.Background(), s, packet); !errors.Is(err, core.ErrTransition) {
		t.Fatal(err)
	}
	if err := l.Resume(context.Background(), core.Scope{Kind: core.ScopeTeam, ID: string(packet.Team)}); err != nil {
		t.Fatal(err)
	}
	if err := l.Pause(context.Background(), core.Scope{Kind: core.ScopeTask, ID: string(packet.Task)}, "user"); err != nil {
		t.Fatal(err)
	}
	if err := lifecycle.AdmissionAllowed(context.Background(), s, packet); !errors.Is(err, core.ErrTransition) {
		t.Fatal(err)
	}
	packet.RunID = "forged"
	if err := lifecycle.AdmissionAllowed(context.Background(), s, packet); !errors.Is(err, core.ErrTransition) {
		t.Fatal(err)
	}
}

func TestProjectScopeResolvesAllCanonicalRuns(t *testing.T) {
	s := store.New(t.TempDir(), core.StorageLimits{CanonicalBytes: 16 << 20})
	projectRoot := t.TempDir()
	var packets []core.AssignmentPacket
	for _, objective := range []string{"x", "y"} {
		manifest, err := run.CreateOneOff(context.Background(), projectRoot, run.OneOffFeature, objective, []core.Task{{ID: core.TaskID("T" + objective), Objective: objective, State: core.Ready, Criteria: []string{"done"}, WritablePaths: []string{"x"}}})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := run.NewRepositories(s).Runs.Initialize(context.Background(), manifest); err != nil {
			t.Fatal(err)
		}
		packets = append(packets, packetFor(manifest))
	}
	sup := &supervisor{}
	if err := lifecycle.New(s, sup).Pause(context.Background(), core.Scope{Kind: core.ScopeProject, ID: projectRoot}, "user"); err != nil {
		t.Fatal(err)
	}
	if len(sup.events) != len(packets) {
		t.Fatalf("events=%+v", sup.events)
	}
	for _, packet := range packets {
		if err := lifecycle.AdmissionAllowed(context.Background(), s, packet); !errors.Is(err, core.ErrTransition) {
			t.Fatalf("run=%s err=%v", packet.RunID, err)
		}
	}
}

func TestRestrictiveProjectFailureKeepsEarlierBarrier(t *testing.T) {
	s := store.New(t.TempDir(), core.StorageLimits{CanonicalBytes: 16 << 20})
	projectRoot := t.TempDir()
	var packets []core.AssignmentPacket
	for _, objective := range []string{"x", "y"} {
		manifest, err := run.CreateOneOff(context.Background(), projectRoot, run.OneOffFeature, objective, []core.Task{{ID: core.TaskID("T" + objective), Objective: objective, State: core.Ready, Criteria: []string{"done"}, WritablePaths: []string{"x"}}})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := run.NewRepositories(s).Runs.Initialize(context.Background(), manifest); err != nil {
			t.Fatal(err)
		}
		packets = append(packets, packetFor(manifest))
	}
	sup := &supervisor{err: core.ErrCapacity, failAt: 2}
	if err := lifecycle.New(s, sup).Pause(context.Background(), core.Scope{Kind: core.ScopeProject, ID: projectRoot}, "user"); !errors.Is(err, core.ErrCapacity) {
		t.Fatal(err)
	}
	if len(sup.events) != 2 {
		t.Fatalf("events=%+v", sup.events)
	}
	for _, packet := range packets {
		err := lifecycle.AdmissionAllowed(context.Background(), s, packet)
		if packet.RunID == sup.events[0].Run && !errors.Is(err, core.ErrTransition) {
			t.Fatalf("first=%v", err)
		}
		if packet.RunID != sup.events[0].Run && err != nil {
			t.Fatalf("later=%v", err)
		}
	}
}
