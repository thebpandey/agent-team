package integrate_test

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/thebpandey/agent-team/vnext/internal/contracts"
	"github.com/thebpandey/agent-team/vnext/internal/core"
	"github.com/thebpandey/agent-team/vnext/internal/gate"
	"github.com/thebpandey/agent-team/vnext/internal/integrate"
	"github.com/thebpandey/agent-team/vnext/internal/project"
	"github.com/thebpandey/agent-team/vnext/internal/store"
)

func TestIntegratorUsesExactWorktreeProvenanceSeriallyAndIdempotently(t *testing.T) {
	manager := &recordingManager{}
	state := store.New(t.TempDir(), core.StorageLimits{CanonicalBytes: 16 << 20})
	integrator := integrate.NewIntegrator(project.Project{Root: "project", Head: "base", Readable: true, Writable: true}, state, manager)
	firstCandidate := validCandidate("TASK-1", "candidate-1")
	first, err := integrator.Integrate(context.Background(), firstCandidate, durableGate(t, state, firstCandidate))
	if err != nil || first.Order != 1 || first.Task != "TASK-1" || first.Candidate != "candidate-1" || first.EvidencePointer == "" {
		t.Fatalf("first integration = %+v, %v", first, err)
	}
	retry, err := integrator.Integrate(context.Background(), firstCandidate, durableGate(t, state, firstCandidate))
	if err != nil || !reflect.DeepEqual(retry, first) || manager.integrations != 1 {
		t.Fatalf("retry = %+v, %v; manager integrations = %d", retry, err, manager.integrations)
	}
	secondCandidate := validCandidate("TASK-2", "candidate-2")
	second, err := integrator.Integrate(context.Background(), secondCandidate, durableGate(t, state, secondCandidate))
	if err != nil || second.Order != 2 || manager.integrations != 2 {
		t.Fatalf("second integration = %+v, %v; manager integrations = %d", second, err, manager.integrations)
	}
	if !reflect.DeepEqual(manager.order, []string{"inspect:RUN", "integrate:TASK-1", "inspect:RUN", "inspect:RUN", "integrate:TASK-2", "inspect:RUN"}) {
		t.Fatalf("worktree call order = %#v", manager.order)
	}
}

func TestIntegratorFailsClosedBeforeMutatingWorktree(t *testing.T) {
	candidate := validCandidate("TASK", "candidate")
	for _, tc := range []struct {
		name       string
		project    project.Project
		candidate  contracts.Candidate
		gate       contracts.GateResult
		inspectOut contracts.Worktree
		want       error
	}{
		{"dirty project", project.Project{Root: "project", Head: "base", Dirty: true}, candidate, contracts.GateResult{}, contracts.Worktree{}, core.ErrTransition},
		{"wrong base", project.Project{Root: "project", Head: "other-base"}, candidate, contracts.GateResult{}, contracts.Worktree{}, core.ErrRevision},
		{"dirty candidate", project.Project{Root: "project", Head: "base"}, dirtyCandidate(candidate), contracts.GateResult{Result: "CLEAN", Revision: "candidate", Evidence: "gate", CleanEvidence: "review"}, contracts.Worktree{}, core.ErrTransition},
		{"wrong gate revision", project.Project{Root: "project", Head: "base"}, candidate, contracts.GateResult{Result: "CLEAN", Revision: "other", CleanEvidence: "review"}, contracts.Worktree{}, core.ErrRevision},
		{"mutated inspection", project.Project{Root: "project", Head: "base"}, candidate, contracts.GateResult{}, contracts.Worktree{Run: "RUN", Team: "TEAM", Path: "/tmp/task", Branch: "branch", Base: "other-base"}, core.ErrRevision},
	} {
		t.Run(tc.name, func(t *testing.T) {
			manager := &recordingManager{inspectOut: tc.inspectOut}
			state := store.New(t.TempDir(), core.StorageLimits{CanonicalBytes: 16 << 20})
			integrator := integrate.NewIntegrator(tc.project, state, manager)
			actualGate := tc.gate
			if actualGate.Result == "" {
				actualGate = durableGate(t, state, tc.candidate)
			}
			if _, err := integrator.Integrate(context.Background(), tc.candidate, actualGate); !errors.Is(err, tc.want) {
				t.Fatalf("Integrate() error = %v, want %v", err, tc.want)
			}
			if manager.integrations != 0 {
				t.Fatalf("mutated worktree: integrations = %d", manager.integrations)
			}
		})
	}
}

func TestIntegratorRejectsInterruptedContextBeforeWorktreeMutation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	manager := &recordingManager{}
	candidate := validCandidate("TASK", "candidate")
	state := store.New(t.TempDir(), core.StorageLimits{CanonicalBytes: 16 << 20})
	integrator := integrate.NewIntegrator(project.Project{Root: "project", Head: "base"}, state, manager)
	if _, err := integrator.Integrate(ctx, candidate, durableGate(t, state, candidate)); !errors.Is(err, core.ErrTransition) {
		t.Fatalf("interrupted Integrate() error = %v, want ErrTransition", err)
	}
	if manager.integrations != 0 || len(manager.order) != 0 {
		t.Fatalf("interrupted integration mutated manager: %+v", manager)
	}
}

func TestIntegratorsShareProjectStoreGuard(t *testing.T) {
	state := store.New(t.TempDir(), core.StorageLimits{CanonicalBytes: 16 << 20})
	project := project.Project{Root: "project", Head: "base"}
	manager := &concurrentManager{}
	first, second := validCandidate("TASK-1", "candidate-1"), validCandidate("TASK-2", "candidate-2")
	firstGate, secondGate := durableGate(t, state, first), durableGate(t, state, second)
	left := integrate.NewIntegrator(project, state, manager)
	right := integrate.NewIntegrator(project, state, manager)
	start := make(chan struct{})
	results := make(chan integrate.Integration, 2)
	errs := make(chan error, 2)
	for _, call := range []struct {
		integrator integrate.Integrator
		candidate  contracts.Candidate
		gate       contracts.GateResult
	}{{left, first, firstGate}, {right, second, secondGate}} {
		go func(call struct {
			integrator integrate.Integrator
			candidate  contracts.Candidate
			gate       contracts.GateResult
		}) {
			<-start
			result, err := call.integrator.Integrate(context.Background(), call.candidate, call.gate)
			results <- result
			errs <- err
		}(call)
	}
	close(start)
	firstResult, secondResult := <-results, <-results
	errsSeen := []error{<-errs, <-errs}
	transitions := 0
	for _, err := range errsSeen {
		if errors.Is(err, core.ErrTransition) {
			transitions++
		} else if err != nil {
			t.Fatal(err)
		}
	}
	if manager.max != 1 || manager.integrations != 1 || transitions != 1 || (firstResult.Order != 1 && secondResult.Order != 1) {
		t.Fatalf("serial results=%+v,%+v manager=%+v", firstResult, secondResult, manager)
	}
}

func validCandidate(task core.TaskID, revision string) contracts.Candidate {
	return contracts.Candidate{Task: task, Revision: revision, Base: "base", Worktree: contracts.Worktree{Run: "RUN", Team: "TEAM", Path: "/tmp/task", Branch: "branch", Base: "base", Candidate: revision}}
}

func dirtyCandidate(candidate contracts.Candidate) contracts.Candidate {
	candidate.Worktree.Dirty = true
	return candidate
}

func durableGate(t *testing.T, state *store.Store, candidate contracts.Candidate) contracts.GateResult {
	t.Helper()
	input := contracts.GateInput{Run: candidate.Worktree.Run, Task: candidate.Task, Candidate: candidate, TrackerRevision: 1, ReceiptRevision: 1, ScopeFingerprint: "scope", ReceiptDigest: "receipt", ReviewDigest: "review"}
	result, err := gate.NewGateWithAuthority(nil, state, integrationAuthority{}).Check(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

type integrationAuthority struct{}

func (integrationAuthority) Validate(context.Context, contracts.GateInput) (core.RecordEnvelope, error) {
	return core.RecordEnvelope{Schema: 1, Project: "project", RunID: "RUN", Revision: 1, WrittenAt: time.Now().UTC().Format(time.RFC3339)}, nil
}

type recordingManager struct {
	inspectOut   contracts.Worktree
	order        []string
	integrations int
}

type concurrentManager struct {
	mu                        sync.Mutex
	active, max, integrations int
}

func (m *concurrentManager) Create(context.Context, contracts.WorktreeSpec) (contracts.Worktree, error) {
	return contracts.Worktree{}, errors.New("unexpected Create")
}
func (m *concurrentManager) Inspect(_ context.Context, worktree contracts.Worktree) (contracts.Worktree, error) {
	return worktree, nil
}
func (m *concurrentManager) Integrate(_ context.Context, candidate contracts.Candidate) (contracts.Candidate, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.active++
	if m.active > m.max {
		m.max = m.active
	}
	m.integrations++
	m.active--
	return candidate, nil
}
func (m *concurrentManager) Cleanup(context.Context, core.TeamID) error {
	return errors.New("unexpected Cleanup")
}

func (m *recordingManager) Create(context.Context, contracts.WorktreeSpec) (contracts.Worktree, error) {
	return contracts.Worktree{}, errors.New("unexpected Create")
}

func (m *recordingManager) Inspect(_ context.Context, worktree contracts.Worktree) (contracts.Worktree, error) {
	m.order = append(m.order, "inspect:"+string(worktree.Run))
	if m.inspectOut != (contracts.Worktree{}) {
		return m.inspectOut, nil
	}
	return worktree, nil
}

func (m *recordingManager) Integrate(_ context.Context, candidate contracts.Candidate) (contracts.Candidate, error) {
	m.order = append(m.order, "integrate:"+string(candidate.Task))
	m.integrations++
	return candidate, nil
}

func (m *recordingManager) Cleanup(context.Context, core.TeamID) error {
	return errors.New("unexpected Cleanup")
}
