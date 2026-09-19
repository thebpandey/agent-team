package supervise_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sync"
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
	polls   int
}

func bindHandle(t *testing.T, state *store.Store, a *adapter, handle contracts.WorkerHandle) supervise.Supervisor {
	t.Helper()
	s := supervise.NewSupervisor(state, a, nil)
	packet := core.AssignmentPacket{RecordEnvelope: core.RecordEnvelope{RunID: handle.Run}, Team: handle.Team, Task: handle.Task, Worktree: state.Root, Base: "base", Scope: []string{"src"}, QueueFingerprint: handle.PacketDigest, SpecRevision: handle.CandidateRevision}
	got, err := s.Start(context.Background(), packet)
	if err != nil || !reflect.DeepEqual(got, handle) {
		t.Fatalf("bind Start() = %+v, %v", got, err)
	}
	return s
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
	a.polls++
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
	s := bindHandle(t, store.New(t.TempDir(), core.StorageLimits{CanonicalBytes: 16 << 20}), a, handle)
	got, err := s.Turn(context.Background(), handle)
	if err != nil || len(got) == 0 || len(got) >= len(a.poll) {
		t.Fatalf("Turn() = %d bytes, %v", len(got), err)
	}
}

func TestTurnReturnsBoundedPartialOutputOnAdapterError(t *testing.T) {
	handle := contracts.WorkerHandle{Host: "host", Identity: "worker", Run: "RUN", Team: "TEAM", Task: "TASK", PacketDigest: "packet", CandidateRevision: "spec"}
	pollErr := errors.New("adapter failed")
	a := &adapter{handle: handle, poll: string(make([]byte, 256<<10)), pollErr: pollErr}
	s := bindHandle(t, store.New(t.TempDir(), core.StorageLimits{CanonicalBytes: 16 << 20}), a, handle)
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
	s := bindHandle(t, state, a, handle)
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
	beforeHandle := handle
	handle.Task = "OTHER"
	a := &adapter{handle: beforeHandle, poll: "partial", pollErr: context.Canceled}
	s := bindHandle(t, state, a, beforeHandle)
	if _, err := s.Turn(context.Background(), handle); !errors.Is(err, core.ErrRevision) {
		t.Fatalf("Turn() error = %v, want ErrRevision", err)
	}
	after := readReceipt(t, state, core.TeamID(before.Team))
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("mismatched handle mutated receipt: before=%+v after=%+v", before, after)
	}
}

func TestTurnRequiresExactDurableBindingBeforePoll(t *testing.T) {
	state, _, handle := interruptionFixture(t)
	a := &adapter{handle: handle, poll: "must not poll"}
	s := bindHandle(t, state, a, handle)
	forged := handle
	forged.Sequence++
	if _, err := s.Turn(context.Background(), forged); !errors.Is(err, core.ErrRevision) {
		t.Fatalf("forged Turn() error = %v", err)
	}
	if a.polls != 0 {
		t.Fatalf("forged handle reached Poll %d times", a.polls)
	}
}

func TestStartRejectsConflictingDurableBinding(t *testing.T) {
	state := store.New(t.TempDir(), core.StorageLimits{CanonicalBytes: 16 << 20})
	handle := contracts.WorkerHandle{Host: "host", Identity: "worker", Run: "RUN", Team: "TEAM", Task: "TASK", PacketDigest: "packet", CandidateRevision: "spec"}
	a := &adapter{handle: handle}
	s := bindHandle(t, state, a, handle)
	packet := core.AssignmentPacket{RecordEnvelope: core.RecordEnvelope{RunID: handle.Run}, Team: handle.Team, Task: handle.Task, Worktree: t.TempDir(), Base: "base", Scope: []string{"other"}, QueueFingerprint: handle.PacketDigest, SpecRevision: handle.CandidateRevision}
	if _, err := s.Start(context.Background(), packet); !errors.Is(err, core.ErrRevision) {
		t.Fatalf("conflicting Start() error = %v", err)
	}
}

func TestOrdinaryEventRequiresCanonicalReceiptProvenance(t *testing.T) {
	state, manifest, handle := interruptionFixture(t)
	if err := os.Remove(filepath.Join(state.Root, ".agent-team", "receipts", string(handle.Team)+".json")); err != nil {
		t.Fatal(err)
	}
	s := supervise.NewSupervisor(state, nil, nil)
	event := workflow.Event{Run: manifest.ID, Scope: core.Scope{Kind: core.ScopeTask, ID: "TASK"}, Kind: workflow.Pause, From: core.Implementing, Reason: "operator pause", AdmissionHeld: true, RefillHeld: true}
	if err := s.Emit(context.Background(), event); !errors.Is(err, core.ErrRevision) {
		t.Fatalf("ordinary event without receipt = %v", err)
	}
	if _, err := os.Stat(filepath.Join(state.Root, ".agent-team", "supervision", "events", string(manifest.ID), "task-TASK")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("ordinary event persisted without provenance: %v", err)
	}
}

func TestOrdinaryEventsResolveEachCanonicalScope(t *testing.T) {
	state, manifest, handle := interruptionFixture(t)
	s := supervise.NewSupervisor(state, nil, nil)
	for _, scope := range []core.Scope{
		{Kind: core.ScopeTask, ID: "TASK"},
		{Kind: core.ScopeTeam, ID: string(handle.Team)},
		{Kind: core.ScopeRun, ID: string(manifest.ID)},
		{Kind: core.ScopeProject, ID: filepath.Base(manifest.Project)},
	} {
		event := workflow.Event{Run: manifest.ID, Scope: scope, Kind: workflow.Pause, From: core.Implementing, Reason: "operator pause", AdmissionHeld: true, RefillHeld: true, Confirmed: scope.Kind == core.ScopeProject}
		if err := s.Emit(context.Background(), event); err != nil {
			t.Fatalf("Emit(%+v) = %v", scope, err)
		}
	}
}

func TestTamperedBindingFailsBeforePoll(t *testing.T) {
	state, _, handle := interruptionFixture(t)
	a := &adapter{handle: handle, poll: "must not poll"}
	s := bindHandle(t, state, a, handle)
	relative := filepath.Join(".agent-team", "supervision", "bindings", string(handle.Run), string(handle.Team), string(handle.Task)+".json")
	var value map[string]any
	if err := state.ReadJSON(relative, 64<<10, &value); err != nil {
		t.Fatal(err)
	}
	request := value["request"].(map[string]any)
	worktree := request["Worktree"]
	if worktree == nil {
		worktree = request["worktree"]
	}
	worktree.(map[string]any)["Base"] = "forged"
	if _, err := state.WriteJSON(relative, value, 64<<10); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Turn(context.Background(), handle); !errors.Is(err, core.ErrRevision) {
		t.Fatalf("tampered binding Turn() = %v", err)
	}
	if a.polls != 0 {
		t.Fatalf("tampered binding reached Poll %d times", a.polls)
	}
}

func TestInterruptedRetryRepairsMissingEventAndRejectsTampering(t *testing.T) {
	state, _, handle := interruptionFixture(t)
	a := &adapter{handle: handle, poll: "partial", pollErr: context.Canceled}
	s := bindHandle(t, state, a, handle)
	if _, err := s.Turn(context.Background(), handle); !errors.Is(err, core.ErrTransition) {
		t.Fatal(err)
	}
	directory := filepath.Join(state.Root, ".agent-team", "supervision", "events", string(handle.Run), "task-TASK")
	entries, err := os.ReadDir(directory)
	if err != nil || len(entries) != 1 {
		t.Fatalf("initial event = %v, %v", entries, err)
	}
	if err := os.Remove(filepath.Join(directory, entries[0].Name())); err != nil {
		t.Fatal(err)
	}
	head := filepath.Join(state.Root, ".agent-team", "supervision", "heads", string(handle.Run), "task-TASK.json")
	if err := os.Remove(head); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Turn(context.Background(), handle); !errors.Is(err, core.ErrTransition) {
		t.Fatalf("missing-event retry = %v", err)
	}
	entries, err = os.ReadDir(directory)
	if err != nil || len(entries) != 1 {
		t.Fatalf("repaired event = %v, %v", entries, err)
	}
	if _, err := state.WriteJSON(filepath.Join(".agent-team", "supervision", "events", string(handle.Run), "task-TASK", entries[0].Name()), map[string]any{"schema": 1}, 64<<10); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Turn(context.Background(), handle); !errors.Is(err, core.ErrRevision) {
		t.Fatalf("tampered-event retry = %v", err)
	}
}

func TestConcurrentIdenticalInterruptedTurnsConvergeOnce(t *testing.T) {
	state, _, handle := interruptionFixture(t)
	first := &adapter{handle: handle, poll: "same partial", pollErr: context.Canceled}
	second := &adapter{handle: handle, poll: "same partial", pollErr: context.Canceled}
	s1 := bindHandle(t, state, first, handle)
	s2 := bindHandle(t, state, second, handle)
	start := make(chan struct{})
	errs := make(chan error, 2)
	var wait sync.WaitGroup
	for _, supervisor := range []supervise.Supervisor{s1, s2} {
		wait.Add(1)
		go func(s supervise.Supervisor) {
			defer wait.Done()
			<-start
			_, err := s.Turn(context.Background(), handle)
			errs <- err
		}(supervisor)
	}
	close(start)
	wait.Wait()
	close(errs)
	for err := range errs {
		if !errors.Is(err, core.ErrTransition) {
			t.Fatalf("concurrent Turn() = %v", err)
		}
	}
	receipt := readReceipt(t, state, handle.Team)
	if receipt.State != core.Interrupted || receipt.Revision != 3 || len(receipt.EvidencePointers) != 2 {
		t.Fatalf("concurrent receipt = %+v", receipt)
	}
	directory := filepath.Join(state.Root, ".agent-team", "supervision", "events", string(handle.Run), "task-TASK")
	entries, err := os.ReadDir(directory)
	if err != nil || len(entries) != 1 {
		t.Fatalf("concurrent events = %v, %v", entries, err)
	}
	first.poll = "different partial"
	if _, err := s1.Turn(context.Background(), handle); !errors.Is(err, core.ErrRevision) {
		t.Fatalf("non-identical retry = %v", err)
	}
}

func TestOversizeHandleFailsBeforeBindingOrEvidence(t *testing.T) {
	state := store.New(t.TempDir(), core.StorageLimits{CanonicalBytes: 16 << 20})
	handle := contracts.WorkerHandle{Host: "host", Identity: string(make([]byte, 1025)), Run: "RUN", Team: "TEAM", Task: "TASK", PacketDigest: "packet"}
	a := &adapter{handle: handle}
	s := supervise.NewSupervisor(state, a, nil)
	packet := core.AssignmentPacket{RecordEnvelope: core.RecordEnvelope{RunID: "RUN"}, Team: "TEAM", Task: "TASK", Worktree: t.TempDir(), Base: "base", Scope: []string{"src"}, QueueFingerprint: "packet"}
	if _, err := s.Start(context.Background(), packet); !errors.Is(err, core.ErrRevision) {
		t.Fatalf("oversize handle Start() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(state.Root, ".agent-team", "supervision", "bindings")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("oversize handle wrote binding: %v", err)
	}
}

func TestCancellationEvidenceAlwaysFitsEventLimit(t *testing.T) {
	state, _, handle := interruptionFixture(t)
	a := &adapter{handle: handle, poll: string(make([]byte, 256<<10)), pollErr: context.Canceled}
	s := bindHandle(t, state, a, handle)
	if _, err := s.Turn(context.Background(), handle); !errors.Is(err, core.ErrTransition) {
		t.Fatalf("large cancellation Turn() error = %v", err)
	}
	directory := filepath.Join(state.Root, ".agent-team", "supervision", "events", string(handle.Run), "task-TASK")
	entries, err := os.ReadDir(directory)
	if err != nil || len(entries) != 1 {
		t.Fatalf("large evidence records = %v, %v", entries, err)
	}
	info, err := os.Stat(filepath.Join(directory, entries[0].Name()))
	if err != nil || info.Size() > 64<<10 {
		t.Fatalf("event size = %d, %v", info.Size(), err)
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

func TestHeadRepairRejectsTamperedEarlierRecord(t *testing.T) {
	state, manifest, _ := interruptionFixture(t)
	s := supervise.NewSupervisor(state, nil, nil)
	scope := core.Scope{Kind: core.ScopeTask, ID: "TASK"}
	first := workflow.Event{Run: manifest.ID, Scope: scope, Kind: workflow.CheckpointEvent, CheckpointDigest: digest, AdmissionHeld: true, RefillHeld: true, Reason: "first chain record"}
	second := first
	second.Reason = "second chain record"
	if err := s.Emit(context.Background(), first); err != nil {
		t.Fatal(err)
	}
	if err := s.Emit(context.Background(), second); err != nil {
		t.Fatal(err)
	}
	directory := filepath.Join(state.Root, ".agent-team", "supervision", "events", string(manifest.ID), "task-TASK")
	entries, err := os.ReadDir(directory)
	if err != nil || len(entries) != 2 {
		t.Fatalf("event chain = %v, %v", entries, err)
	}
	firstName := ""
	for _, entry := range entries {
		var record map[string]any
		if err := state.ReadJSON(filepath.Join(".agent-team", "supervision", "events", string(manifest.ID), "task-TASK", entry.Name()), 64<<10, &record); err != nil {
			t.Fatal(err)
		}
		event, _ := record["event"].(map[string]any)
		if event["reason"] == first.Reason {
			firstName = entry.Name()
		}
	}
	if firstName == "" {
		t.Fatal("first record not found")
	}
	if _, err := state.WriteJSON(filepath.Join(".agent-team", "supervision", "events", string(manifest.ID), "task-TASK", firstName), map[string]any{"schema": 1}, 64<<10); err != nil {
		t.Fatal(err)
	}
	head := filepath.Join(state.Root, ".agent-team", "supervision", "heads", string(manifest.ID), "task-TASK.json")
	if err := os.Remove(head); err != nil {
		t.Fatal(err)
	}
	if err := s.Emit(context.Background(), second); !errors.Is(err, core.ErrRevision) {
		t.Fatalf("tampered chain retry = %v, want ErrRevision", err)
	}
}

func TestOrdinaryEventDoesNotCreateCanonicalCheckpoint(t *testing.T) {
	state, manifest, _ := interruptionFixture(t)
	s := supervise.NewSupervisor(state, nil, nil)
	event := workflow.Event{Run: manifest.ID, Scope: core.Scope{Kind: core.ScopeTask, ID: "TASK"}, Kind: workflow.Pause, From: core.Implementing, Reason: "operator pause", AdmissionHeld: true, RefillHeld: true}
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
	a := &adapter{handle: handle, poll: "must not poll"}
	s := bindHandle(t, state, a, handle)
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
