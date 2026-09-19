// Package supervise runs one bounded foreground observation and records its
// recovery evidence. It deliberately owns no process, lease, or background
// loop: a caller must invoke every turn explicitly.
package supervise

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/thebpandey/agent-team/vnext/internal/contracts"
	"github.com/thebpandey/agent-team/vnext/internal/core"
	"github.com/thebpandey/agent-team/vnext/internal/dispatch"
	"github.com/thebpandey/agent-team/vnext/internal/host"
	"github.com/thebpandey/agent-team/vnext/internal/knowledge"
	"github.com/thebpandey/agent-team/vnext/internal/project"
	"github.com/thebpandey/agent-team/vnext/internal/run"
	"github.com/thebpandey/agent-team/vnext/internal/store"
	"github.com/thebpandey/agent-team/vnext/internal/workflow"
)

// Observation is the bounded textual result of one foreground adapter poll.
type Observation = string

// Supervisor is intentionally synchronous. It has no lifecycle separate from
// the host call that invokes it.
type Supervisor interface {
	Start(context.Context, core.AssignmentPacket) (contracts.WorkerHandle, error)
	Turn(context.Context, contracts.WorkerHandle) (Observation, error)
	Emit(context.Context, workflow.Event) error
	Checkpoint(context.Context, core.RunID, core.Scope, string) error
}

const (
	observationBytes = 48 << 10
	eventLimit       = 64 << 10
	interruptTimeout = 2 * time.Second
	handleFieldBytes = 1024
	errorBytes       = 1024
)

type supervisor struct {
	state   *store.Store
	adapter host.Adapter
	// runner is retained as the explicit host boundary declared by the Phase 2
	// contract. Supervision never invokes it: adapter.Poll is the sole turn.
	runner   host.CommandRunner
	mu       *sync.Mutex
	lockRoot string
	lockErr  error
}

var locks sync.Map

func lockFor(root string) *sync.Mutex {
	value, _ := locks.LoadOrStore(root, &sync.Mutex{})
	return value.(*sync.Mutex)
}

func interruptLockFor(s *supervisor, handle contracts.WorkerHandle) *sync.Mutex {
	return lockFor(s.lockRoot + "\x00" + string(handle.Run) + "\x00task\x00" + string(handle.Task))
}

// NewSupervisor creates a foreground-only supervisor.
func NewSupervisor(state *store.Store, adapter host.Adapter, runner host.CommandRunner) Supervisor {
	root, err := canonicalStoreRoot(state)
	if err != nil {
		root = "<invalid-store-root>"
	}
	return &supervisor{state: state, adapter: adapter, runner: runner, mu: lockFor(root), lockRoot: root, lockErr: err}
}

func canonicalStoreRoot(state *store.Store) (string, error) {
	if state == nil || state.Root == "" {
		return "", core.ErrPath
	}
	abs, err := filepath.Abs(filepath.Clean(state.Root))
	if err != nil {
		return "", core.ErrPath
	}
	root, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", core.ErrPath
	}
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		return "", core.ErrPath
	}
	return filepath.Clean(root), nil
}

func (s *supervisor) lockError() error {
	if s == nil || s.lockErr != nil {
		return core.ErrPath
	}
	return nil
}

// InterruptedEvent represents the durable checkpoint which precedes a receipt
// transition. CheckpointEvent, not a second invented event kind, is canonical.
func InterruptedEvent(handle contracts.WorkerHandle) workflow.Event {
	digest := interruptionDigest(handle, "", "interrupted")
	return workflow.Event{
		Run:              handle.Run,
		Scope:            core.Scope{Kind: core.ScopeTask, ID: string(handle.Task)},
		Kind:             workflow.CheckpointEvent,
		CheckpointDigest: digest,
		AdmissionHeld:    true,
		RefillHeld:       true,
		Reason:           "foreground turn interrupted",
	}
}

// InterruptedState is the exact Phase 1 state used by an interrupted receipt.
func InterruptedState() core.TaskState { return core.Interrupted }

func (s *supervisor) Start(ctx context.Context, packet core.AssignmentPacket) (contracts.WorkerHandle, error) {
	if err := ctx.Err(); err != nil {
		return contracts.WorkerHandle{}, fmt.Errorf("%w: start cancelled", core.ErrTransition)
	}
	if s == nil || s.state == nil {
		return contracts.WorkerHandle{}, core.ErrPath
	}
	if err := s.lockError(); err != nil {
		return contracts.WorkerHandle{}, err
	}
	if s.adapter == nil {
		return contracts.WorkerHandle{}, core.ErrCapacity
	}
	request, err := workerRequest(packet)
	if err != nil {
		return contracts.WorkerHandle{}, err
	}
	if err := validRequestCapacity(request); err != nil {
		return contracts.WorkerHandle{}, err
	}
	handle, err := s.adapter.StartWorker(ctx, request)
	if err != nil {
		return contracts.WorkerHandle{}, err
	}
	if err := validateHandle(packet, handle); err != nil {
		return contracts.WorkerHandle{}, err
	}
	if err := s.bind(request, handle); err != nil {
		return contracts.WorkerHandle{}, err
	}
	return handle, nil
}

func workerRequest(packet core.AssignmentPacket) (contracts.WorkerRequest, error) {
	worktree := contracts.WorktreeSpec{
		Run:           packet.RunID,
		Team:          packet.Team,
		Root:          packet.Worktree,
		Base:          packet.Base,
		WritablePaths: append([]string(nil), packet.Scope...),
	}
	if err := dispatch.ValidatePacket(packet, worktree); err != nil {
		return contracts.WorkerRequest{}, err
	}
	return contracts.WorkerRequest{
		Packet:        copyPacket(packet),
		Worktree:      worktree,
		WritablePaths: append([]string(nil), worktree.WritablePaths...),
		Reviewer:      false,
	}, nil
}

func copyPacket(packet core.AssignmentPacket) core.AssignmentPacket {
	packet.Criteria = append([]string(nil), packet.Criteria...)
	packet.Scope = append([]string(nil), packet.Scope...)
	packet.Checks = append([]core.Check(nil), packet.Checks...)
	for index := range packet.Checks {
		packet.Checks[index].Command = append([]string(nil), packet.Checks[index].Command...)
	}
	packet.Capabilities = append([]string(nil), packet.Capabilities...)
	packet.Skills = append([]core.SkillRef(nil), packet.Skills...)
	packet.Resources.Servers = append([]string(nil), packet.Resources.Servers...)
	packet.Resources.Browsers = append([]string(nil), packet.Resources.Browsers...)
	packet.Resources.External = append([]string(nil), packet.Resources.External...)
	packet.Accelerators = append([]core.AcceleratorRef(nil), packet.Accelerators...)
	return packet
}

func validateHandle(packet core.AssignmentPacket, handle contracts.WorkerHandle) error {
	if handle.Host == "" || handle.Identity == "" || handle.Reviewer || handle.Run != packet.RunID || handle.Team != packet.Team ||
		handle.Task != packet.Task || handle.PacketDigest != packet.QueueFingerprint || handle.CandidateRevision != packet.SpecRevision {
		return core.ErrRevision
	}
	return validHandleIdentity(handle)
}

type binding struct {
	Schema  int                     `json:"schema"`
	Request contracts.WorkerRequest `json:"request"`
	Handle  contracts.WorkerHandle  `json:"handle"`
}

func bindingPath(handle contracts.WorkerHandle) string {
	return path.Join(".agent-team", "supervision", "bindings", string(handle.Run), string(handle.Team), string(handle.Task)+".json")
}

func (s *supervisor) bind(request contracts.WorkerRequest, handle contracts.WorkerHandle) error {
	if err := s.lockError(); err != nil {
		return err
	}
	value := binding{Schema: 1, Request: request, Handle: handle}
	if err := validBinding(value); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.state.CreateJSON(bindingPath(handle), value, eventLimit); err == nil {
		return nil
	} else if !errors.Is(err, fs.ErrExist) {
		return err
	}
	var existing binding
	if err := s.state.ReadJSON(bindingPath(handle), eventLimit, &existing); err != nil {
		return core.ErrRevision
	}
	if !reflect.DeepEqual(existing, value) {
		return core.ErrRevision
	}
	return nil
}

func validBinding(value binding) error {
	if value.Schema != 1 || value.Request.Reviewer || value.Request.Packet.RunID != value.Handle.Run ||
		value.Request.Packet.Team != value.Handle.Team || value.Request.Packet.Task != value.Handle.Task ||
		value.Request.Packet.QueueFingerprint != value.Handle.PacketDigest || value.Request.Packet.SpecRevision != value.Handle.CandidateRevision ||
		value.Request.Worktree.Run != value.Handle.Run || value.Request.Worktree.Team != value.Handle.Team ||
		!reflect.DeepEqual(value.Request.WritablePaths, value.Request.Worktree.WritablePaths) {
		return core.ErrRevision
	}
	if err := validateHandle(value.Request.Packet, value.Handle); err != nil {
		return err
	}
	// Re-run the same packet/worktree authority check used by Start, then
	// derive the request again. A persisted request is never trusted merely
	// because its handle fields happen to agree.
	if err := dispatch.ValidatePacket(value.Request.Packet, value.Request.Worktree); err != nil {
		return err
	}
	expected, err := workerRequest(value.Request.Packet)
	if err != nil || !reflect.DeepEqual(expected, value.Request) {
		return core.ErrRevision
	}
	encoded, err := json.Marshal(value)
	if err != nil || len(encoded) > eventLimit {
		return core.ErrLimit
	}
	return nil
}

func validRequestCapacity(request contracts.WorkerRequest) error {
	encoded, err := json.Marshal(request)
	if err != nil || len(encoded) > eventLimit-6*handleFieldBytes {
		return core.ErrLimit
	}
	return nil
}

func (s *supervisor) bindingFor(handle contracts.WorkerHandle) error {
	if err := s.lockError(); err != nil {
		return err
	}
	if err := validHandleIdentity(handle); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var value binding
	if err := s.state.ReadJSON(bindingPath(handle), eventLimit, &value); err != nil {
		return core.ErrRevision
	}
	if err := validBinding(value); err != nil || !reflect.DeepEqual(value.Handle, handle) {
		return core.ErrRevision
	}
	return nil
}

func (s *supervisor) Turn(ctx context.Context, handle contracts.WorkerHandle) (Observation, error) {
	if err := ctx.Err(); err != nil {
		// A cancellation before Poll has no partial output, but it still has a
		// scoped handle and attempted adapter argument worth preserving. Keep the
		// persistence synchronous and detached only from the caller cancellation.
		if s != nil && s.state != nil && s.adapter != nil {
			if handleErr := s.bindingFor(handle); handleErr != nil {
				return "", handleErr
			}
			if persistErr := s.interrupt(ctx, handle, "", err); persistErr != nil {
				return "", persistErr
			}
		}
		return "", fmt.Errorf("%w: turn cancelled", core.ErrTransition)
	}
	if s == nil || s.state == nil {
		return "", core.ErrPath
	}
	if err := s.lockError(); err != nil {
		return "", err
	}
	if s.adapter == nil {
		return "", core.ErrCapacity
	}
	if err := s.bindingFor(handle); err != nil {
		return "", err
	}
	observation, err := s.adapter.Poll(ctx, handle)
	observation = bounded(observation, observationBytes)
	if err == nil && ctx.Err() == nil {
		return observation, nil
	}
	if ctx.Err() != nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		if persistErr := s.interrupt(ctx, handle, observation, err); persistErr != nil {
			return observation, persistErr
		}
		return observation, fmt.Errorf("%w: foreground turn interrupted", core.ErrTransition)
	}
	return observation, err
}

func bounded(value string, limit int) string {
	if limit < 1 || len(value) <= limit {
		return value
	}
	return value[:limit]
}

func (s *supervisor) Checkpoint(ctx context.Context, runID core.RunID, scope core.Scope, digest string) error {
	event := workflow.Event{
		Run:              runID,
		Scope:            scope,
		Kind:             workflow.CheckpointEvent,
		CheckpointDigest: digest,
		AdmissionHeld:    true,
		RefillHeld:       true,
		Reason:           "foreground checkpoint",
	}
	return s.Emit(ctx, event)
}

func (s *supervisor) Emit(ctx context.Context, event workflow.Event) error {
	if s == nil || s.state == nil {
		return core.ErrPath
	}
	if err := s.lockError(); err != nil {
		return err
	}
	if err := validateEvent(event); err != nil {
		return err
	}
	if event.Kind == workflow.CheckpointEvent {
		if err := workflow.Checkpoint(ctx, s.state, event.Run, event.Scope, event.CheckpointDigest); err != nil {
			return err
		}
	} else if err := s.resolveOrdinaryEvent(ctx, event); err != nil {
		return err
	}
	return s.persistEvent(event)
}

func validateEvent(event workflow.Event) error {
	if len(event.Reason) > 4096 || strings.Contains(event.Reason, "\x00") {
		return core.ErrTransition
	}
	if err := project.ValidateSegment(string(event.Run)); err != nil {
		return core.ErrPath
	}
	if event.Scope.Kind != core.ScopeProject && event.Scope.Kind != core.ScopeRun && event.Scope.Kind != core.ScopeTeam && event.Scope.Kind != core.ScopeTask {
		return core.ErrTransition
	}
	if err := project.ValidateSegment(event.Scope.ID); err != nil {
		return core.ErrPath
	}
	if event.Scope.Kind == core.ScopeRun && event.Scope.ID != string(event.Run) {
		return core.ErrTransition
	}
	state := core.Working
	if event.From != "" {
		state = event.From
	}
	if event.Kind == workflow.CheckpointEvent {
		if !event.AdmissionHeld || !event.RefillHeld || !validDigest(event.CheckpointDigest) {
			return core.ErrTransition
		}
	} else if event.CheckpointDigest != "" {
		return core.ErrTransition
	}
	if _, err := workflow.Transition(state, event); err != nil {
		return err
	}
	return nil
}

func validDigest(digest string) bool {
	if !strings.HasPrefix(digest, "sha256:") || len(digest) != len("sha256:")+sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(strings.TrimPrefix(digest, "sha256:"))
	return err == nil
}

type eventRecord struct {
	Schema           int                `json:"schema"`
	Run              core.RunID         `json:"runId"`
	Scope            core.Scope         `json:"scope"`
	Kind             workflow.EventKind `json:"kind"`
	Key              string             `json:"key"`
	Digest           string             `json:"digest"`
	CheckpointDigest string             `json:"checkpointDigest,omitempty"`
	Sequence         uint64             `json:"sequence"`
	Previous         string             `json:"previousDigest,omitempty"`
	Event            workflow.Event     `json:"event"`
	Evidence         *eventEvidence     `json:"evidence,omitempty"`
}

// eventEvidence is deliberately bounded and is written only for an
// interrupted foreground Poll. It records exactly the partial return, the
// adapter call being made, and the immutable handle which scoped it.
type eventEvidence struct {
	Handle      contracts.WorkerHandle `json:"handle"`
	Observation string                 `json:"observation"`
	Argument    string                 `json:"argument"`
	Error       string                 `json:"error,omitempty"`
}

type eventHead struct {
	Schema   int        `json:"schema"`
	Run      core.RunID `json:"runId"`
	Scope    core.Scope `json:"scope"`
	Sequence uint64     `json:"sequence"`
	Digest   string     `json:"digest"`
}

func eventDirectory(event workflow.Event) string {
	return path.Join(".agent-team", "supervision", "events", string(event.Run), string(event.Scope.Kind)+"-"+event.Scope.ID)
}

func eventPath(event workflow.Event, evidence *eventEvidence) string {
	return path.Join(eventDirectory(event), strings.TrimPrefix(eventKey(event, evidence), "sha256:")+".json")
}

func headPath(event workflow.Event) string {
	return path.Join(".agent-team", "supervision", "heads", string(event.Run), string(event.Scope.Kind)+"-"+event.Scope.ID+".json")
}

func (s *supervisor) persistEvent(event workflow.Event) error {
	return s.persistEventEvidence(event, nil)
}

func (s *supervisor) persistEventEvidence(event workflow.Event, evidence *eventEvidence) error {
	if err := s.lockError(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	recordPath := eventPath(event, evidence)
	for attempts := 0; attempts < 2; attempts++ {
		head, err := s.rebuildHead(event)
		if err != nil {
			return err
		}
		var existing eventRecord
		readErr := s.state.ReadJSON(recordPath, eventLimit, &existing)
		if readErr == nil {
			if !validRecord(existing, event, evidence) {
				return core.ErrRevision
			}
			// A historical idempotent retry must not roll the canonical head back.
			return nil
		}
		if !errors.Is(readErr, fs.ErrNotExist) {
			return readErr
		}

		sequence, previous := head.Sequence+1, head.Digest
		record := eventRecord{Schema: 1, Run: event.Run, Scope: event.Scope, Kind: event.Kind, Key: eventKey(event, evidence), Digest: recordDigest(sequence, previous, event, evidence), CheckpointDigest: event.CheckpointDigest, Sequence: sequence, Previous: previous, Event: event, Evidence: evidence}
		if _, err := s.state.CreateJSON(recordPath, record, eventLimit); err == nil {
			_, err = s.rebuildHead(event)
			return err
		} else if !errors.Is(err, fs.ErrExist) {
			return err
		}
	}
	return core.ErrRevision
}

// rebuildHead derives the only permissible head from every immutable record.
// It repairs an absent or stale projection only after proving a contiguous,
// single-rooted chain; malformed, forked, and ambiguous histories fail closed.
func (s *supervisor) rebuildHead(event workflow.Event) (eventHead, error) {
	records, err := s.records(event)
	if err != nil {
		return eventHead{}, err
	}
	head := eventHead{Schema: 1, Run: event.Run, Scope: event.Scope}
	if len(records) > 0 {
		sort.Slice(records, func(i, j int) bool { return records[i].Sequence < records[j].Sequence })
		for index, record := range records {
			if !validStoredRecord(record, event) || record.Sequence != uint64(index+1) {
				return eventHead{}, core.ErrRevision
			}
			if index == 0 {
				if record.Previous != "" {
					return eventHead{}, core.ErrRevision
				}
			} else if record.Previous != records[index-1].Digest {
				return eventHead{}, core.ErrRevision
			}
		}
		tail := records[len(records)-1]
		head.Sequence, head.Digest = tail.Sequence, tail.Digest
	}
	var stored eventHead
	readErr := s.state.ReadJSON(headPath(event), eventLimit, &stored)
	if readErr != nil && !errors.Is(readErr, fs.ErrNotExist) {
		// A damaged projection is repairable only after the immutable chain above
		// has been proven; replace it with the derived projection.
		readErr = nil
	}
	if len(records) == 0 {
		if readErr == nil && !reflect.DeepEqual(stored, head) {
			if err := s.writeHead(head); err != nil {
				return eventHead{}, err
			}
		}
		return head, nil
	}
	if readErr != nil || !reflect.DeepEqual(stored, head) {
		if err := s.writeHead(head); err != nil {
			return eventHead{}, err
		}
	}
	return head, nil
}

func (s *supervisor) records(event workflow.Event) ([]eventRecord, error) {
	root, err := os.OpenRoot(s.state.Root)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	directory := eventDirectory(event)
	info, err := root.Lstat(directory)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return nil, core.ErrRevision
	}
	dir, err := root.Open(directory)
	if err != nil {
		return nil, core.ErrRevision
	}
	defer dir.Close()
	entries, err := dir.ReadDir(-1)
	if err != nil {
		return nil, err
	}
	records := make([]eventRecord, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 || !strings.HasSuffix(entry.Name(), ".json") {
			return nil, core.ErrRevision
		}
		var record eventRecord
		if err := s.state.ReadJSON(path.Join(directory, entry.Name()), eventLimit, &record); err != nil {
			return nil, core.ErrRevision
		}
		records = append(records, record)
	}
	return records, nil
}

func (s *supervisor) writeHead(head eventHead) error {
	_, err := s.state.WriteJSON(headPath(workflow.Event{Run: head.Run, Scope: head.Scope}), head, eventLimit)
	return err
}

func validRecord(record eventRecord, event workflow.Event, evidence *eventEvidence) bool {
	if record.Schema != 1 || record.Run != event.Run || record.Scope != event.Scope || record.Kind != event.Kind || record.Sequence == 0 ||
		record.Key != eventKey(event, evidence) || record.Digest != recordDigest(record.Sequence, record.Previous, event, evidence) ||
		record.CheckpointDigest != event.CheckpointDigest || !reflect.DeepEqual(record.Event, event) || !reflect.DeepEqual(record.Evidence, evidence) ||
		(record.Sequence == 1 && record.Previous != "") || (record.Sequence > 1 && !validDigest(record.Previous)) {
		return false
	}
	return event.Kind != workflow.CheckpointEvent || validDigest(record.CheckpointDigest)
}

func validStoredRecord(record eventRecord, event workflow.Event) bool {
	if record.Schema != 1 || record.Run != event.Run || record.Scope != event.Scope || record.Sequence == 0 ||
		record.Key != eventKey(record.Event, record.Evidence) || record.Digest != recordDigest(record.Sequence, record.Previous, record.Event, record.Evidence) ||
		record.CheckpointDigest != record.Event.CheckpointDigest || !validDigest(record.Digest) ||
		(record.Sequence == 1 && record.Previous != "") || (record.Sequence > 1 && !validDigest(record.Previous)) {
		return false
	}
	if err := validateEvent(record.Event); err != nil {
		return false
	}
	return record.Kind == record.Event.Kind && (record.Kind != workflow.CheckpointEvent || validDigest(record.CheckpointDigest)) && validEvidence(record.Evidence)
}

func validEvidence(evidence *eventEvidence) bool {
	if evidence == nil {
		return true
	}
	return validHandleIdentity(evidence.Handle) == nil && evidence.Argument == "adapter.Poll" &&
		len(evidence.Observation) <= observationBytes && len(evidence.Error) <= errorBytes
}

func eventKey(event workflow.Event, evidence *eventEvidence) string {
	payload := struct {
		Event    workflow.Event `json:"event"`
		Evidence *eventEvidence `json:"evidence,omitempty"`
	}{Event: event, Evidence: evidence}
	encoded, _ := json.Marshal(payload)
	sum := sha256.Sum256(encoded)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func recordDigest(sequence uint64, previous string, event workflow.Event, evidence *eventEvidence) string {
	payload := struct {
		Sequence uint64         `json:"sequence"`
		Previous string         `json:"previousDigest"`
		Event    workflow.Event `json:"event"`
		Evidence *eventEvidence `json:"evidence,omitempty"`
	}{Sequence: sequence, Previous: previous, Event: event, Evidence: evidence}
	encoded, _ := json.Marshal(payload)
	sum := sha256.Sum256(encoded)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func validHandleIdentity(handle contracts.WorkerHandle) error {
	if err := project.ValidateSegment(string(handle.Run)); err != nil {
		return core.ErrPath
	}
	if err := project.ValidateSegment(string(handle.Team)); err != nil || project.ValidateSegment(string(handle.Task)) != nil || handle.Host == "" || handle.Identity == "" || handle.PacketDigest == "" || handle.Reviewer ||
		len(handle.Host) > handleFieldBytes || len(handle.Identity) > handleFieldBytes || len(handle.PacketDigest) > handleFieldBytes || len(handle.CandidateRevision) > handleFieldBytes || handle.Attempt < 0 {
		return core.ErrRevision
	}
	return nil
}

func (s *supervisor) interrupt(ctx context.Context, handle contracts.WorkerHandle, observation string, turnErr error) error {
	if err := s.lockError(); err != nil {
		return err
	}
	guard := interruptLockFor(s, handle)
	guard.Lock()
	defer guard.Unlock()

	persistCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), interruptTimeout)
	defer cancel()

	receipt, manifest, err := s.receiptFor(persistCtx, handle)
	if err != nil {
		return err
	}
	evidence, err := fitEvidence(workflow.Event{Run: handle.Run, Scope: core.Scope{Kind: core.ScopeTask, ID: string(handle.Task)}, Kind: workflow.CheckpointEvent, Reason: "foreground turn interrupted", AdmissionHeld: true, RefillHeld: true}, eventEvidence{Handle: handle, Observation: bounded(observation, observationBytes), Argument: "adapter.Poll", Error: bounded(errorText(turnErr), errorBytes)})
	if err != nil {
		return err
	}
	digest := interruptionDigest(handle, evidence.Observation, evidence.Error)
	pointer := "supervision/" + strings.TrimPrefix(digest, "sha256:")
	event := InterruptedEvent(handle)
	event.CheckpointDigest = digest
	event.Reason = "foreground turn interrupted"
	if receipt.State == core.Interrupted {
		if receipt.NextAction == "resume" && contains(receipt.EvidencePointers, pointer) {
			if err := s.convergeInterruptedCheckpoint(persistCtx, event, manifest); err != nil {
				return err
			}
			if err := s.persistEventEvidence(event, &evidence); err != nil {
				return err
			}
			return nil
		}
		return core.ErrRevision
	}
	if err := workflow.Checkpoint(persistCtx, s.state, event.Run, event.Scope, event.CheckpointDigest); err != nil {
		return err
	}
	if err := s.persistEventEvidence(event, &evidence); err != nil {
		return err
	}

	// workflow.Checkpoint can advance canonical receipt revision while repairing
	// its projection, so reread and validate before the state transition.
	receipt, manifest, err = s.receiptFor(persistCtx, handle)
	if err != nil {
		return err
	}
	_ = manifest
	if receipt.State == core.Interrupted {
		if receipt.NextAction == "resume" && contains(receipt.EvidencePointers, pointer) {
			return nil
		}
		return core.ErrRevision
	}
	receipt.State = core.Interrupted
	receipt.NextAction = "resume"
	receipt.Revision++
	receipt.WrittenAt = time.Now().UTC().Format(time.RFC3339)
	receipt.EvidencePointers = append(receipt.EvidencePointers, pointer)
	if len(receipt.EvidencePointers) > 128 {
		return core.ErrLimit
	}
	return knowledge.WriteReceipt(persistCtx, s.state, receipt)
}

func checkpointRecordPath(event workflow.Event) string {
	return path.Join(".agent-team", "checkpoints", string(event.Run), string(event.Scope.Kind)+"-"+event.Scope.ID+".json")
}

// convergeInterruptedCheckpoint verifies the already-committed canonical
// checkpoint after an interrupted receipt has advanced. If a crash removed the
// checkpoint record, Phase 1 can safely recreate it from the current receipt.
func (s *supervisor) convergeInterruptedCheckpoint(ctx context.Context, event workflow.Event, manifest run.Run) error {
	var record workflow.CheckpointRecord
	err := s.state.ReadJSON(checkpointRecordPath(event), eventLimit, &record)
	if errors.Is(err, fs.ErrNotExist) {
		return workflow.Checkpoint(ctx, s.state, event.Run, event.Scope, event.CheckpointDigest)
	}
	if err != nil {
		return core.ErrRevision
	}
	teams, teamErr := ordinaryTeams(manifest, event.Scope)
	if teamErr != nil || record.Schema != 1 || record.Project != manifest.Project || record.RunID != manifest.ID ||
		record.WrittenAt != manifest.WrittenAt || record.Revision != manifest.Revision || record.Scope != event.Scope ||
		record.Digest != event.CheckpointDigest || !record.AdmissionHeld || !record.RefillHeld || len(record.Receipts) != len(teams) || len(teams) == 0 {
		return core.ErrRevision
	}
	expected := make(map[string]bool, len(teams))
	for _, team := range teams {
		expected[path.Join(".agent-team", "receipts", string(team.ID)+".json")] = true
	}
	for _, projection := range record.Receipts {
		if !expected[projection.Path] || !validDigest(projection.BeforeDigest) || !validDigest(projection.AfterDigest) || projection.BeforeIdentity == "" || projection.AfterIdentity == "" {
			return core.ErrRevision
		}
		delete(expected, projection.Path)
	}
	if len(expected) != 0 {
		return core.ErrRevision
	}
	return nil
}

func fitEvidence(event workflow.Event, evidence eventEvidence) (eventEvidence, error) {
	if !validEvidence(&evidence) {
		return eventEvidence{}, core.ErrRevision
	}
	for {
		probe := eventRecord{Schema: 1, Run: event.Run, Scope: event.Scope, Kind: event.Kind, Key: "sha256:" + strings.Repeat("0", 64), Digest: "sha256:" + strings.Repeat("0", 64), CheckpointDigest: event.CheckpointDigest, Sequence: 1, Event: event, Evidence: &evidence}
		encoded, err := json.Marshal(probe)
		if err != nil {
			return eventEvidence{}, err
		}
		if len(encoded)+1 <= eventLimit {
			return evidence, nil
		}
		if len(evidence.Observation) > 0 {
			evidence.Observation = evidence.Observation[:len(evidence.Observation)/2]
			continue
		}
		if len(evidence.Error) > 0 {
			evidence.Error = evidence.Error[:len(evidence.Error)/2]
			continue
		}
		return eventEvidence{}, core.ErrLimit
	}
}

func (s *supervisor) receiptFor(ctx context.Context, handle contracts.WorkerHandle) (knowledge.Receipt, run.Run, error) {
	if s == nil || s.state == nil {
		return knowledge.Receipt{}, run.Run{}, core.ErrPath
	}
	if err := validHandleIdentity(handle); err != nil {
		return knowledge.Receipt{}, run.Run{}, err
	}
	return s.receiptForTask(ctx, handle.Run, handle.Team, handle.Task)
}

func (s *supervisor) receiptForTask(ctx context.Context, runID core.RunID, teamID core.TeamID, taskID core.TaskID) (knowledge.Receipt, run.Run, error) {
	manifest, err := run.NewRepositories(s.state).Runs.Read(ctx, runID)
	if err != nil {
		return knowledge.Receipt{}, run.Run{}, fmt.Errorf("%w: canonical run: %v", core.ErrRevision, err)
	}
	var team run.TeamRecord
	found := false
	for _, candidate := range manifest.Teams {
		if candidate.ID == teamID {
			team, found = candidate, true
			break
		}
	}
	if !found || !containsTask(team.Queue, taskID) {
		return knowledge.Receipt{}, run.Run{}, core.ErrRevision
	}
	receipt, err := s.receiptForTeam(ctx, manifest, team)
	if err != nil || receipt.Task != string(taskID) {
		return knowledge.Receipt{}, run.Run{}, core.ErrRevision
	}
	return receipt, manifest, nil
}

func (s *supervisor) receiptForTeam(ctx context.Context, manifest run.Run, team run.TeamRecord) (knowledge.Receipt, error) {
	var receipt knowledge.Receipt
	relative := path.Join(".agent-team", "receipts", string(team.ID)+".json")
	if err := s.state.ReadJSON(relative, eventLimit, &receipt); err != nil {
		return knowledge.Receipt{}, fmt.Errorf("%w: receipt: %v", core.ErrRevision, err)
	}
	if receipt.Schema != 1 || receipt.Project != manifest.Project || receipt.RunID != manifest.ID || receipt.Team != string(team.ID) ||
		receipt.Task == "" || receipt.Revision == 0 || receipt.Attempt < 1 || receipt.WrittenAt == "" || !containsTask(team.Queue, core.TaskID(receipt.Task)) {
		return knowledge.Receipt{}, core.ErrRevision
	}
	return receipt, nil
}

func (s *supervisor) resolveOrdinaryEvent(ctx context.Context, event workflow.Event) error {
	manifest, err := run.NewRepositories(s.state).Runs.Read(ctx, event.Run)
	if err != nil {
		return core.ErrRevision
	}
	teams, err := ordinaryTeams(manifest, event.Scope)
	if err != nil {
		return err
	}
	for _, team := range teams {
		receipt, err := s.receiptForTeam(ctx, manifest, team)
		if err != nil {
			return err
		}
		if _, err := workflow.Transition(receipt.State, event); err != nil {
			return core.ErrRevision
		}
	}
	return nil
}

func ordinaryTeams(manifest run.Run, scope core.Scope) ([]run.TeamRecord, error) {
	switch scope.Kind {
	case core.ScopeProject:
		if scope.ID != filepath.Base(manifest.Project) {
			return nil, core.ErrRevision
		}
	case core.ScopeRun:
		if scope.ID != string(manifest.ID) {
			return nil, core.ErrRevision
		}
	case core.ScopeTeam:
		for _, team := range manifest.Teams {
			if string(team.ID) == scope.ID {
				return []run.TeamRecord{team}, nil
			}
		}
		return nil, core.ErrRevision
	case core.ScopeTask:
		taskID := core.TaskID(scope.ID)
		foundTask := false
		for _, task := range manifest.Tasks {
			if task.ID == taskID {
				foundTask = true
				break
			}
		}
		if !foundTask {
			return nil, core.ErrRevision
		}
		var owner *run.TeamRecord
		for index := range manifest.Teams {
			if containsTask(manifest.Teams[index].Queue, taskID) {
				if owner != nil {
					return nil, core.ErrRevision
				}
				owner = &manifest.Teams[index]
			}
		}
		if owner == nil {
			return nil, core.ErrRevision
		}
		return []run.TeamRecord{*owner}, nil
	default:
		return nil, core.ErrRevision
	}
	if len(manifest.Teams) == 0 {
		return nil, core.ErrRevision
	}
	return append([]run.TeamRecord(nil), manifest.Teams...), nil
}

func containsTask(values []core.TaskID, wanted core.TaskID) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func contains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func interruptionDigest(handle contracts.WorkerHandle, observation, reason string) string {
	payload := struct {
		Run, Team, Task, Identity, Packet, Revision, Observation, Reason string
	}{string(handle.Run), string(handle.Team), string(handle.Task), handle.Identity, handle.PacketDigest, handle.CandidateRevision, bounded(observation, observationBytes), bounded(reason, 4096)}
	encoded, _ := json.Marshal(payload)
	sum := sha256.Sum256(encoded)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func errorText(err error) string {
	if err == nil {
		return "context cancelled"
	}
	return err.Error()
}
