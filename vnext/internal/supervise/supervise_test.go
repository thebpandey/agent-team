package supervise_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/thebpandey/agent-team/vnext/internal/contracts"
	"github.com/thebpandey/agent-team/vnext/internal/core"
	"github.com/thebpandey/agent-team/vnext/internal/knowledge"
	"github.com/thebpandey/agent-team/vnext/internal/run"
	"github.com/thebpandey/agent-team/vnext/internal/store"
	"github.com/thebpandey/agent-team/vnext/internal/supervise"
	"github.com/thebpandey/agent-team/vnext/internal/workflow"
)

const digest = "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

type adapter struct {
	request contracts.WorkerRequest
	handle  contracts.WorkerHandle
	poll    string
	pollErr error
}

func (a *adapter) Probe(context.Context) (contracts.HostCapabilities, error) {
	return contracts.HostCapabilities{}, nil
}
func (a *adapter) StartWorker(_ context.Context, request contracts.WorkerRequest) (contracts.WorkerHandle, error) {
	a.request = request
	return a.handle, nil
}
func (a *adapter) StartReviewer(context.Context, contracts.WorkerRequest, contracts.WorkerHandle) (contracts.WorkerHandle, error) {
	return contracts.WorkerHandle{}, core.ErrTransition
}
func (a *adapter) Poll(context.Context, contracts.WorkerHandle) (string, error) {
	return a.poll, a.pollErr
}
func (a *adapter) Stop(context.Context, contracts.WorkerHandle, core.Scope) error { return nil }
func (a *adapter) ReadIdentity(context.Context, contracts.WorkerHandle) (string, error) {
	return "worker", nil
}

func TestStartMakesExactCopiedRequestAndRejectsForgedHandle(t *testing.T) {
	packet := core.AssignmentPacket{
		RecordEnvelope:   core.RecordEnvelope{RunID: "RUN"},
		Team:             "TEAM",
		Task:             "TASK",
		Worktree:         t.TempDir(),
		Base:             "base",
		Scope:            []string{"src"},
		Criteria:         []string{"criterion"},
		QueueFingerprint: "packet",
		SpecRevision:     "spec",
	}
	a := &adapter{handle: contracts.WorkerHandle{Host: "host", Identity: "worker", Run: "RUN", Team: "TEAM", Task: "TASK", PacketDigest: "packet", CandidateRevision: "spec"}}
	s := supervise.NewSupervisor(store.New(t.TempDir(), core.StorageLimits{CanonicalBytes: 16 << 20}), a, nil)
	if got, err := s.Start(context.Background(), packet); err != nil || got.Identity != "worker" {
		t.Fatalf("Start() = %+v, %v", got, err)
	}
	want := contracts.WorkerRequest{Packet: packet, Worktree: contracts.WorktreeSpec{Run: "RUN", Team: "TEAM", Root: packet.Worktree, Base: "base", WritablePaths: []string{"src"}}, WritablePaths: []string{"src"}}
	if !reflect.DeepEqual(a.request, want) {
		t.Fatalf("request = %#v, want %#v", a.request, want)
	}
	packet.Scope[0], packet.Criteria[0] = "mutated", "mutated"
	if a.request.Packet.Scope[0] != "src" || a.request.Packet.Criteria[0] != "criterion" || a.request.Worktree.WritablePaths[0] != "src" {
		t.Fatalf("request was not copied: %#v", a.request)
	}

	a.handle.Team = "OTHER"
	if _, err := s.Start(context.Background(), want.Packet); !errors.Is(err, core.ErrRevision) {
		t.Fatalf("forged handle error = %v, want ErrRevision", err)
	}
}

func TestNilDependenciesFailClosed(t *testing.T) {
	packet := core.AssignmentPacket{RecordEnvelope: core.RecordEnvelope{RunID: "RUN"}, Team: "TEAM", Task: "TASK", Worktree: t.TempDir(), Base: "base", Scope: []string{"src"}, QueueFingerprint: "packet"}
	if _, err := supervise.NewSupervisor(nil, nil, nil).Start(context.Background(), packet); !errors.Is(err, core.ErrPath) {
		t.Fatalf("nil store Start() error = %v", err)
	}
	state := store.New(t.TempDir(), core.StorageLimits{CanonicalBytes: 16 << 20})
	if _, err := supervise.NewSupervisor(state, nil, nil).Start(context.Background(), packet); !errors.Is(err, core.ErrCapacity) {
		t.Fatalf("nil adapter Start() error = %v", err)
	}
	if _, err := supervise.NewSupervisor(state, nil, nil).Turn(context.Background(), contracts.WorkerHandle{Host: "host", Identity: "worker", Run: "RUN", Team: "TEAM", Task: "TASK", PacketDigest: "packet"}); !errors.Is(err, core.ErrCapacity) {
		t.Fatalf("nil adapter Turn() error = %v", err)
	}
}

func TestTurnPollsOnceAndBoundsOutput(t *testing.T) {
	handle := contracts.WorkerHandle{Host: "host", Identity: "worker", Run: "RUN", Team: "TEAM", Task: "TASK", PacketDigest: "packet", CandidateRevision: "spec"}
	a := &adapter{handle: handle, poll: string(make([]byte, 256<<10))}
	s := supervise.NewSupervisor(store.New(t.TempDir(), core.StorageLimits{CanonicalBytes: 16 << 20}), a, nil)
	got, err := s.Turn(context.Background(), handle)
	if err != nil || len(got) == 0 || len(got) >= len(a.poll) {
		t.Fatalf("Turn() = %d bytes, %v", len(got), err)
	}
}

func TestTurnReturnsBoundedPartialOutputOnAdapterError(t *testing.T) {
	handle := contracts.WorkerHandle{Host: "host", Identity: "worker", Run: "RUN", Team: "TEAM", Task: "TASK", PacketDigest: "packet", CandidateRevision: "spec"}
	pollErr := errors.New("adapter failed")
	a := &adapter{handle: handle, poll: string(make([]byte, 256<<10)), pollErr: pollErr}
	s := supervise.NewSupervisor(store.New(t.TempDir(), core.StorageLimits{CanonicalBytes: 16 << 20}), a, nil)
	got, err := s.Turn(context.Background(), handle)
	if !errors.Is(err, pollErr) || len(got) == 0 || len(got) >= len(a.poll) {
		t.Fatalf("Turn() = %d bytes, %v", len(got), err)
	}
}

func interruptionFixture(t *testing.T) (*store.Store, run.Run, contracts.WorkerHandle) {
	t.Helper()
	manifest, err := run.CreateOneOff(context.Background(), t.TempDir(), run.OneOffFeature, "supervise fixture", []core.Task{{
		ID: "TASK", Objective: "fixture", State: core.Ready, Criteria: []string{"done"}, WritablePaths: []string{"src"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	state := store.New(manifest.Root, core.StorageLimits{CanonicalBytes: 16 << 20})
	if _, err := run.NewRepositories(state).Runs.Initialize(context.Background(), manifest); err != nil {
		t.Fatal(err)
	}
	receipt := knowledge.Receipt{RecordEnvelope: core.RecordEnvelope{Schema: 1, Project: manifest.Project, RunID: manifest.ID, WrittenAt: manifest.WrittenAt, Revision: 1}, Team: string(manifest.Teams[0].ID), Task: "TASK", Attempt: 1, State: core.Implementing, NextAction: "continue"}
	if err := knowledge.WriteReceipt(context.Background(), state, receipt); err != nil {
		t.Fatal(err)
	}
	return state, manifest, contracts.WorkerHandle{Host: "host", Identity: "worker", Run: manifest.ID, Team: manifest.Teams[0].ID, Task: "TASK", PacketDigest: "packet", CandidateRevision: "spec"}
}

func readReceipt(t *testing.T, state *store.Store, team core.TeamID) knowledge.Receipt {
	t.Helper()
	var receipt knowledge.Receipt
	if err := state.ReadJSON(filepath.Join(".agent-team", "receipts", string(team)+".json"), 16<<20, &receipt); err != nil {
		t.Fatal(err)
	}
	return receipt
}

func TestCancelledTurnPersistsPartialEvidenceAndConverges(t *testing.T) {
	state, _, handle := interruptionFixture(t)
	a := &adapter{handle: handle, poll: "partial observation", pollErr: context.Canceled}
	s := supervise.NewSupervisor(state, a, nil)
	got, err := s.Turn(context.Background(), handle)
	if got != "partial observation" || !errors.Is(err, core.ErrTransition) {
		t.Fatalf("Turn() = %q, %v", got, err)
	}
	first := readReceipt(t, state, handle.Team)
	if first.State != core.Interrupted || first.NextAction != "resume" || len(first.EvidencePointers) < 2 || first.EvidencePointers[len(first.EvidencePointers)-1][:11] != "supervision" {
		t.Fatalf("interrupted receipt = %+v", first)
	}
	events, err := os.ReadDir(filepath.Join(state.Root, ".agent-team", "supervision", "events", string(handle.Run), "task-TASK"))
	if err != nil || len(events) != 1 {
		t.Fatalf("interruption event directory = %v, %v", events, err)
	}
	var record map[string]any
	if err := state.ReadJSON(filepath.Join(".agent-team", "supervision", "events", string(handle.Run), "task-TASK", events[0].Name()), 64<<10, &record); err != nil {
		t.Fatal(err)
	}
	evidence, ok := record["evidence"].(map[string]any)
	if !ok || evidence["observation"] != "partial observation" || evidence["argument"] != "adapter.Poll" {
		t.Fatalf("event omitted partial evidence: %#v", record)
	}
	if _, err := s.Turn(context.Background(), handle); !errors.Is(err, core.ErrTransition) {
		t.Fatalf("retry error = %v", err)
	}
	second := readReceipt(t, state, handle.Team)
	if second.Revision != first.Revision || !reflect.DeepEqual(second.EvidencePointers, first.EvidencePointers) {
		t.Fatalf("retry changed receipt: first=%+v second=%+v", first, second)
	}
}

func TestCancellationProvenanceMismatchDoesNotMutateReceipt(t *testing.T) {
	state, _, handle := interruptionFixture(t)
	before := readReceipt(t, state, handle.Team)
	handle.Task = "OTHER"
	s := supervise.NewSupervisor(state, &adapter{handle: handle, poll: "partial", pollErr: context.Canceled}, nil)
	if _, err := s.Turn(context.Background(), handle); !errors.Is(err, core.ErrRevision) {
		t.Fatalf("Turn() error = %v, want ErrRevision", err)
	}
	after := readReceipt(t, state, core.TeamID(before.Team))
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("mismatched handle mutated receipt: before=%+v after=%+v", before, after)
	}
}

func TestCheckpointEventIdempotenceCollisionAndSafePaths(t *testing.T) {
	state, manifest, _ := interruptionFixture(t)
	s := supervise.NewSupervisor(state, nil, nil)
	scope := core.Scope{Kind: core.ScopeTask, ID: "TASK"}
	if err := s.Checkpoint(context.Background(), manifest.ID, scope, digest); err != nil {
		t.Fatal(err)
	}
	if err := s.Checkpoint(context.Background(), manifest.ID, scope, digest); err != nil {
		t.Fatalf("idempotent checkpoint = %v", err)
	}
	directory := filepath.Join(state.Root, ".agent-team", "supervision", "events", string(manifest.ID), "task-TASK")
	entries, err := os.ReadDir(directory)
	if err != nil || len(entries) != 1 {
		t.Fatalf("checkpoint events = %v, %v", entries, err)
	}
	if _, err := state.WriteJSON(filepath.Join(".agent-team", "supervision", "events", string(manifest.ID), "task-TASK", entries[0].Name()), map[string]any{"schema": 99}, 64<<10); err != nil {
		t.Fatal(err)
	}
	if err := s.Checkpoint(context.Background(), manifest.ID, scope, digest); !errors.Is(err, core.ErrRevision) {
		t.Fatalf("collision error = %v, want ErrRevision", err)
	}
	outside := filepath.Join(t.TempDir(), "outside")
	if err := s.Checkpoint(context.Background(), core.RunID(".."), core.Scope{Kind: core.ScopeTask, ID: "C:\\outside"}, digest); err == nil {
		t.Fatal("unsafe identifiers accepted")
	}
	if _, err := os.Stat(outside); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("write escaped store root: %v", err)
	}
}

func TestForegroundEventsAreOrderedAndHeadRepairsOnRetry(t *testing.T) {
	state, manifest, _ := interruptionFixture(t)
	s := supervise.NewSupervisor(state, nil, nil)
	scope := core.Scope{Kind: core.ScopeTask, ID: "TASK"}
	first := workflow.Event{Run: manifest.ID, Scope: scope, Kind: workflow.CheckpointEvent, CheckpointDigest: digest, AdmissionHeld: true, RefillHeld: true, Reason: "first foreground event"}
	second := first
	second.Reason = "second foreground event"
	if err := s.Emit(context.Background(), first); err != nil {
		t.Fatal(err)
	}
	if err := s.Emit(context.Background(), second); err != nil {
		t.Fatalf("second foreground event = %v", err)
	}
	directory := filepath.Join(state.Root, ".agent-team", "supervision", "events", string(manifest.ID), "task-TASK")
	entries, err := os.ReadDir(directory)
	if err != nil || len(entries) != 2 {
		t.Fatalf("ordered records = %v, %v", entries, err)
	}
	head := filepath.Join(state.Root, ".agent-team", "supervision", "heads", string(manifest.ID), "task-TASK.json")
	if err := os.Remove(head); err != nil {
		t.Fatal(err)
	}
	if err := s.Emit(context.Background(), second); err != nil {
		t.Fatalf("retry did not repair head: %v", err)
	}
	if _, err := os.Stat(head); err != nil {
		t.Fatalf("repaired head = %v", err)
	}
}

func TestOrdinaryEventDoesNotCreateCanonicalCheckpoint(t *testing.T) {
	state, manifest, _ := interruptionFixture(t)
	s := supervise.NewSupervisor(state, nil, nil)
	event := workflow.Event{Run: manifest.ID, Scope: core.Scope{Kind: core.ScopeTask, ID: "TASK"}, Kind: workflow.Pause, From: core.Working, Reason: "operator pause", AdmissionHeld: true, RefillHeld: true}
	if err := s.Emit(context.Background(), event); err != nil {
		t.Fatal(err)
	}
	checkpoint := filepath.Join(state.Root, ".agent-team", "checkpoints", string(manifest.ID), "task-TASK.json")
	if _, err := os.Stat(checkpoint); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("ordinary event invented checkpoint: %v", err)
	}
}

func TestMalformedEventFailsBeforeCanonicalWrite(t *testing.T) {
	state, manifest, _ := interruptionFixture(t)
	s := supervise.NewSupervisor(state, nil, nil)
	event := workflow.Event{Run: manifest.ID, Scope: core.Scope{Kind: core.ScopeTask, ID: "TASK"}, Kind: workflow.CheckpointEvent, CheckpointDigest: "bad", AdmissionHeld: true, RefillHeld: true}
	if err := s.Emit(context.Background(), event); !errors.Is(err, core.ErrTransition) {
		t.Fatalf("Emit() error = %v", err)
	}
	checkpoint := filepath.Join(state.Root, ".agent-team", "checkpoints", string(manifest.ID), "task-TASK.json")
	if _, err := os.Stat(checkpoint); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("malformed event wrote checkpoint: %v", err)
	}
}

func TestCancelledContextReturnsPromptlyWithoutAdapterCall(t *testing.T) {
	state, _, handle := interruptionFixture(t)
	s := supervise.NewSupervisor(state, &adapter{handle: handle, poll: "must not poll"}, nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	started := time.Now()
	if _, err := s.Turn(ctx, handle); !errors.Is(err, core.ErrTransition) {
		t.Fatalf("Turn() error = %v", err)
	}
	if time.Since(started) > time.Second {
		t.Fatal("cancelled turn was not synchronous")
	}
	if got := readReceipt(t, state, handle.Team); got.State != core.Interrupted || got.NextAction != "resume" {
		t.Fatalf("cancelled context did not preserve handle evidence: %+v", got)
	}
}

func TestCancelledTurn(t *testing.T) {
	s := supervise.NewSupervisor(store.New(t.TempDir(), core.StorageLimits{CanonicalBytes: 16 << 20}), nil, nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := s.Turn(ctx, contracts.WorkerHandle{Run: "RUN", Team: "TEAM", Task: "TASK"})
	if !errors.Is(err, core.ErrTransition) {
		t.Fatalf("Turn() error = %v, want ErrTransition", err)
	}
}

func TestInterruptedRepresentation(t *testing.T) {
	handle := contracts.WorkerHandle{Run: "RUN", Task: "TASK"}
	event := supervise.InterruptedEvent(handle)
	if event.Kind != workflow.CheckpointEvent || event.Run != "RUN" || event.Scope.Kind != core.ScopeTask || event.Scope.ID != "TASK" {
		t.Fatalf("InterruptedEvent() = %+v", event)
	}
	if got := supervise.InterruptedState(); got != core.Interrupted {
		t.Fatalf("InterruptedState() = %q, want %q", got, core.Interrupted)
	}
}
