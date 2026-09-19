package integrate_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/thebpandey/agent-team/vnext/internal/contracts"
	"github.com/thebpandey/agent-team/vnext/internal/core"
	"github.com/thebpandey/agent-team/vnext/internal/gate"
	"github.com/thebpandey/agent-team/vnext/internal/integrate"
	"github.com/thebpandey/agent-team/vnext/internal/knowledge"
	"github.com/thebpandey/agent-team/vnext/internal/project"
	"github.com/thebpandey/agent-team/vnext/internal/run"
	"github.com/thebpandey/agent-team/vnext/internal/store"
)

func TestIntegratorUsesExactWorktreeProvenanceSeriallyAndIdempotently(t *testing.T) {
	manager := &recordingManager{}
	state := store.New(t.TempDir(), core.StorageLimits{CanonicalBytes: 16 << 20})
	integrator := integrate.NewIntegrator(project.Project{Root: t.TempDir(), Head: "base", Readable: true, Writable: true}, state, manager)
	firstCandidate := validCandidate("TASK-1", "candidate-1")
	if _, err := integrator.Integrate(context.Background(), firstCandidate, durableGate(t, state, firstCandidate)); !errors.Is(err, core.ErrRevision) || manager.integrations != 0 {
		t.Fatalf("noncanonical gate integrated: %v, calls=%d", err, manager.integrations)
	}
	if _, err := os.Stat(filepath.Join(state.Root, "integrations")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("noncanonical gate wrote integration intent: %v", err)
	}
}

func TestIntegratorRestoresOrderFromCanonicalHistory(t *testing.T) {
	manager := &recordingManager{}
	state := store.New(t.TempDir(), core.StorageLimits{CanonicalBytes: 16 << 20})
	project := project.Project{Root: t.TempDir(), Head: "base", Readable: true, Writable: true}
	first := validCandidate("TASK-1", "candidate-1")
	if _, err := integrate.NewIntegrator(project, state, manager).Integrate(context.Background(), first, durableGate(t, state, first)); !errors.Is(err, core.ErrRevision) {
		t.Fatal(err)
	}
	second := validCandidate("TASK-2", "candidate-2")
	if _, err := integrate.NewIntegrator(project, state, manager).Integrate(context.Background(), second, durableGate(t, state, second)); !errors.Is(err, core.ErrRevision) || manager.integrations != 0 {
		t.Fatalf("noncanonical restart integrated: %v, calls=%d", err, manager.integrations)
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
			if tc.project.Root == "project" {
				tc.project.Root = t.TempDir()
			}
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
	project := project.Project{Root: t.TempDir(), Head: "base"}
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
		} else if !errors.Is(err, core.ErrRevision) {
			t.Fatal(err)
		}
	}
	if manager.max != 0 || manager.integrations != 0 || transitions != 0 || firstResult.Order != 0 || secondResult.Order != 0 {
		t.Fatalf("serial results=%+v,%+v manager=%+v", firstResult, secondResult, manager)
	}
}

func TestIntegratorsShareResolvedAliasGuardAndPinnedStore(t *testing.T) {
	root, stateRoot := t.TempDir(), t.TempDir()
	projectAlias, stateAlias := filepath.Join(t.TempDir(), "project"), filepath.Join(t.TempDir(), "state")
	if err := os.Symlink(root, projectAlias); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if err := os.Symlink(stateRoot, stateAlias); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	state, candidate, gateResult := canonicalInput(t, root, stateRoot)
	manager := &concurrentManager{}
	left := integrate.NewIntegrator(project.Project{Root: root, CommonDir: root, Head: "base"}, state, manager)
	right := integrate.NewIntegrator(project.Project{Root: projectAlias, CommonDir: projectAlias, Head: "base"}, store.New(stateAlias, state.Limits), manager)
	start, errs := make(chan struct{}), make(chan error, 2)
	for _, integrator := range []integrate.Integrator{left, right} {
		go func(integrator integrate.Integrator) {
			<-start
			_, err := integrator.Integrate(context.Background(), candidate, gateResult)
			errs <- err
		}(integrator)
	}
	close(start)
	errsSeen := []error{<-errs, <-errs}
	transitions := 0
	for _, err := range errsSeen {
		if errors.Is(err, core.ErrTransition) {
			transitions++
		} else if err != nil {
			t.Fatal(err)
		}
	}
	if transitions != 1 || manager.integrations != 1 || manager.max != 1 {
		t.Fatalf("alias calls transitions=%d manager=%+v", transitions, manager)
	}
	if err := os.Remove(stateAlias); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(t.TempDir(), stateAlias); err != nil {
		t.Fatal(err)
	}
	if _, err := right.Integrate(context.Background(), candidate, gateResult); err != nil || manager.integrations != 1 {
		t.Fatalf("retargeted alias replayed: %v manager=%+v", err, manager)
	}
}

func canonicalInput(t *testing.T, root, stateRoot string) (*store.Store, contracts.Candidate, contracts.GateResult) {
	t.Helper()
	state := store.New(stateRoot, core.StorageLimits{CanonicalBytes: 16 << 20})
	check := core.Check{Name: "check", Command: []string{"check"}}
	manifest, err := run.CreateOneOff(context.Background(), root, run.Feature, "objective", []core.Task{{ID: "TASK", Objective: "objective", State: core.Ready, Criteria: []string{"criterion"}, Checks: []core.Check{check}, WritablePaths: []string{"vnext"}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := run.NewRepositories(state).Runs.Initialize(context.Background(), manifest); err != nil {
		t.Fatal(err)
	}
	candidate := contracts.Candidate{Task: "TASK", Revision: "candidate", Base: "base", Worktree: contracts.Worktree{Run: manifest.ID, Team: "TEAM", Path: "/tmp/task", Branch: "branch", Base: "base", Candidate: "candidate"}}
	envelope := core.RecordEnvelope{Schema: 1, Project: manifest.Project, RunID: manifest.ID, Revision: 1, WrittenAt: "2026-09-19T00:00:00Z"}
	encoded, _ := json.Marshal(check)
	sum := sha256.Sum256(encoded)
	fingerprint := "sha256:" + hex.EncodeToString(sum[:])
	if err := knowledge.WriteEvidence(context.Background(), state, knowledge.Evidence{RecordEnvelope: envelope, Task: "TASK", Attempt: 1, Exit: 0, InputFingerprint: fingerprint}); err != nil {
		t.Fatal(err)
	}
	receipt := knowledge.Receipt{RecordEnvelope: envelope, Team: "TEAM", Task: "TASK", Attempt: 1, State: core.Clean, Base: "base", Head: "candidate", Review: "review", EvidencePointers: []string{".agent-team/evidence/TASK/1/evidence.json"}, NextAction: "integrate"}
	if err := knowledge.WriteReceipt(context.Background(), state, receipt); err != nil {
		t.Fatal(err)
	}
	encoded, _ = json.Marshal(receipt)
	sum = sha256.Sum256(encoded)
	result, err := gate.NewGate(nil, state).Check(context.Background(), contracts.GateInput{Run: manifest.ID, Task: "TASK", Candidate: candidate, ReceiptRevision: 1, RequiredCheckFingerprints: []string{fingerprint}, ScopeFingerprint: "scope", ReceiptDigest: "sha256:" + hex.EncodeToString(sum[:]), ReviewDigest: "review"})
	if err != nil {
		t.Fatal(err)
	}
	return state, candidate, result
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
	m.active++
	if m.active > m.max {
		m.max = m.active
	}
	m.integrations++
	m.mu.Unlock()
	time.Sleep(5 * time.Millisecond)
	m.mu.Lock()
	m.active--
	m.mu.Unlock()
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
