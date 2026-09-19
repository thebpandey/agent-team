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
	"path"
	"reflect"
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
	observationBytes = 64 << 10
	eventLimit       = 64 << 10
	interruptTimeout = 2 * time.Second
)

type supervisor struct {
	state   *store.Store
	adapter host.Adapter
	// runner is retained as the explicit host boundary declared by the Phase 2
	// contract. Supervision never invokes it: adapter.Poll is the sole turn.
	runner host.CommandRunner
	mu     *sync.Mutex
}

var locks sync.Map

func lockFor(root string) *sync.Mutex {
	value, _ := locks.LoadOrStore(root, &sync.Mutex{})
	return value.(*sync.Mutex)
}

// NewSupervisor creates a foreground-only supervisor.
func NewSupervisor(state *store.Store, adapter host.Adapter, runner host.CommandRunner) Supervisor {
	return &supervisor{state: state, adapter: adapter, runner: runner, mu: lockFor(storeRoot(state))}
}

func storeRoot(state *store.Store) string {
	if state == nil {
		return "<nil>"
	}
	return state.Root
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
	if s.adapter == nil {
		return contracts.WorkerHandle{}, core.ErrCapacity
	}
	request, err := workerRequest(packet)
	if err != nil {
		return contracts.WorkerHandle{}, err
	}
	handle, err := s.adapter.StartWorker(ctx, request)
	if err != nil {
		return contracts.WorkerHandle{}, err
	}
	if err := validateHandle(packet, handle); err != nil {
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
	return nil
}

func (s *supervisor) Turn(ctx context.Context, handle contracts.WorkerHandle) (Observation, error) {
	if err := ctx.Err(); err != nil {
		// A cancellation before Poll has no partial output, but it still has a
		// scoped handle and attempted adapter argument worth preserving. Keep the
		// persistence synchronous and detached only from the caller cancellation.
		if s != nil && s.state != nil && s.adapter != nil {
			if handleErr := validHandleIdentity(handle); handleErr != nil {
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
	if s.adapter == nil {
		return "", core.ErrCapacity
	}
	if err := validHandleIdentity(handle); err != nil {
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
	if err := validateEvent(event); err != nil {
		return err
	}
	if event.Kind == workflow.CheckpointEvent {
		if err := workflow.Checkpoint(ctx, s.state, event.Run, event.Scope, event.CheckpointDigest); err != nil {
			return err
		}
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
	s.mu.Lock()
	defer s.mu.Unlock()

	var head eventHead
	headErr := s.state.ReadJSON(headPath(event), eventLimit, &head)
	if headErr != nil && !errors.Is(headErr, fs.ErrNotExist) {
		return headErr
	}
	if headErr == nil && !validHead(head, event) {
		return core.ErrRevision
	}

	recordPath := eventPath(event, evidence)
	var record eventRecord
	readErr := s.state.ReadJSON(recordPath, eventLimit, &record)
	if readErr == nil {
		if !validRecord(record, event, evidence) {
			return core.ErrRevision
		}
		if headErr == nil && (head.Sequence > record.Sequence || (head.Sequence == record.Sequence && head.Digest != record.Digest)) {
			return core.ErrRevision
		}
		return s.writeHead(eventHead{Schema: 1, Run: event.Run, Scope: event.Scope, Sequence: record.Sequence, Digest: record.Digest})
	}
	if !errors.Is(readErr, fs.ErrNotExist) {
		return readErr
	}

	sequence, previous := uint64(1), ""
	if headErr == nil {
		sequence, previous = head.Sequence+1, head.Digest
	}
	record = eventRecord{Schema: 1, Run: event.Run, Scope: event.Scope, Kind: event.Kind, Key: eventKey(event, evidence), Digest: recordDigest(sequence, previous, event, evidence), CheckpointDigest: event.CheckpointDigest, Sequence: sequence, Previous: previous, Event: event, Evidence: evidence}
	if _, err := s.state.CreateJSON(recordPath, record, eventLimit); err != nil {
		if !errors.Is(err, fs.ErrExist) {
			return err
		}
		var existing eventRecord
		if readErr := s.state.ReadJSON(recordPath, eventLimit, &existing); readErr != nil || !validRecord(existing, event, evidence) {
			return core.ErrRevision
		}
		record = existing
	}
	return s.writeHead(eventHead{Schema: 1, Run: event.Run, Scope: event.Scope, Sequence: record.Sequence, Digest: record.Digest})
}

func (s *supervisor) writeHead(head eventHead) error {
	_, err := s.state.WriteJSON(headPath(workflow.Event{Run: head.Run, Scope: head.Scope}), head, eventLimit)
	return err
}

func validHead(head eventHead, event workflow.Event) bool {
	return head.Schema == 1 && head.Run == event.Run && head.Scope == event.Scope && head.Sequence > 0 && validDigest(head.Digest)
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
	if err := project.ValidateSegment(string(handle.Team)); err != nil || project.ValidateSegment(string(handle.Task)) != nil || handle.Host == "" || handle.Identity == "" || handle.PacketDigest == "" || handle.Reviewer {
		return core.ErrRevision
	}
	return nil
}

func (s *supervisor) interrupt(ctx context.Context, handle contracts.WorkerHandle, observation string, turnErr error) error {
	persistCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), interruptTimeout)
	defer cancel()

	receipt, manifest, err := s.receiptFor(persistCtx, handle)
	if err != nil {
		return err
	}
	digest := interruptionDigest(handle, observation, errorText(turnErr))
	pointer := "supervision/" + strings.TrimPrefix(digest, "sha256:")
	if receipt.State == core.Interrupted {
		if receipt.NextAction == "resume" && contains(receipt.EvidencePointers, pointer) {
			return nil
		}
		return core.ErrRevision
	}
	event := InterruptedEvent(handle)
	event.CheckpointDigest = digest
	event.Reason = "foreground turn interrupted"
	if err := workflow.Checkpoint(persistCtx, s.state, event.Run, event.Scope, event.CheckpointDigest); err != nil {
		return err
	}
	if err := s.persistEventEvidence(event, &eventEvidence{Handle: handle, Observation: observation, Argument: "adapter.Poll", Error: bounded(errorText(turnErr), 4096)}); err != nil {
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

func (s *supervisor) receiptFor(ctx context.Context, handle contracts.WorkerHandle) (knowledge.Receipt, run.Run, error) {
	if s == nil || s.state == nil {
		return knowledge.Receipt{}, run.Run{}, core.ErrPath
	}
	if err := validHandleIdentity(handle); err != nil {
		return knowledge.Receipt{}, run.Run{}, err
	}
	manifest, err := run.NewRepositories(s.state).Runs.Read(ctx, handle.Run)
	if err != nil {
		return knowledge.Receipt{}, run.Run{}, fmt.Errorf("%w: canonical run: %v", core.ErrRevision, err)
	}
	var team run.TeamRecord
	found := false
	for _, candidate := range manifest.Teams {
		if candidate.ID == handle.Team {
			team, found = candidate, true
			break
		}
	}
	if !found || !containsTask(team.Queue, handle.Task) {
		return knowledge.Receipt{}, run.Run{}, core.ErrRevision
	}
	var receipt knowledge.Receipt
	relative := path.Join(".agent-team", "receipts", string(handle.Team)+".json")
	if err := s.state.ReadJSON(relative, eventLimit, &receipt); err != nil {
		return knowledge.Receipt{}, run.Run{}, fmt.Errorf("%w: receipt: %v", core.ErrRevision, err)
	}
	if receipt.Schema != 1 || receipt.Project != manifest.Project || receipt.RunID != handle.Run || receipt.Team != string(handle.Team) ||
		receipt.Task != string(handle.Task) || receipt.Revision == 0 || receipt.Attempt < 1 || receipt.WrittenAt == "" {
		return knowledge.Receipt{}, run.Run{}, core.ErrRevision
	}
	return receipt, manifest, nil
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
