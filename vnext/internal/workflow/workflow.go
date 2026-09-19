// Package workflow contains the small, deterministic state machine used by
// vNext lifecycle operations. It deliberately has no process, lease, or host
// liveness authority.
package workflow

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/thebpandey/agent-team/vnext/internal/core"
	"github.com/thebpandey/agent-team/vnext/internal/knowledge"
	"github.com/thebpandey/agent-team/vnext/internal/project"
	"github.com/thebpandey/agent-team/vnext/internal/store"
)

// EventKind is one explicit, durable lifecycle fact.
type EventKind string

const (
	Pause  EventKind = "pause"
	Stop   EventKind = "stop"
	Cancel EventKind = "cancel"
	Resume EventKind = "resume"

	// Checkpoint is the function below; these aliases provide the checkpoint
	// event kind without colliding with that required Go API.
	EventCheckpoint EventKind = "checkpoint"
	CheckpointEvent EventKind = EventCheckpoint
	CheckpointKind  EventKind = EventCheckpoint
	EventPause      EventKind = Pause
	EventStop       EventKind = Stop
	EventCancel     EventKind = Cancel
	EventResume     EventKind = Resume
)

// Event is a scoped state transition proposal. Run is optional for the pure
// transition helper (callers that have a run must supply it); Checkpoint uses
// the explicit run argument and validates it before writing.
type Event struct {
	Run              core.RunID     `json:"runId,omitempty"`
	Scope            core.Scope     `json:"scope"`
	Kind             EventKind      `json:"kind"`
	From             core.TaskState `json:"from,omitempty"`
	To               core.TaskState `json:"to,omitempty"`
	Reason           string         `json:"reason,omitempty"`
	CheckpointDigest string         `json:"checkpointDigest,omitempty"`
	// Digest is retained as a compatibility spelling for callers that use the
	// shorter field; new records are always serialized with CheckpointDigest.
	Digest        string `json:"-"`
	Revision      uint64 `json:"revision,omitempty"`
	Confirmed     bool   `json:"confirmed,omitempty"`
	AdmissionHeld bool   `json:"admissionHeld,omitempty"`
	RefillHeld    bool   `json:"refillHeld,omitempty"`
}

// CheckpointRecord is canonical, bounded recovery state. It is factual only:
// it contains no claims about ownership, process liveness, leases, or hooks.
type CheckpointRecord struct {
	Schema    int        `json:"schema"`
	RunID     core.RunID `json:"runId"`
	Scope     core.Scope `json:"scope"`
	Revision  uint64     `json:"revision"`
	Digest    string     `json:"digest"`
	WrittenAt string     `json:"writtenAt"`
}

var checkpointLocks sync.Map // map[string]*sync.Mutex, keyed by canonical file

func lockFor(path string) *sync.Mutex {
	value, _ := checkpointLocks.LoadOrStore(path, &sync.Mutex{})
	return value.(*sync.Mutex)
}

func transitionError(format string, args ...any) error {
	return fmt.Errorf("%w: %s", core.ErrTransition, fmt.Sprintf(format, args...))
}

func validScope(scope core.Scope) error {
	switch scope.Kind {
	case core.ScopeProject, core.ScopeRun, core.ScopeTeam, core.ScopeTask:
	default:
		return transitionError("unknown scope kind %q", scope.Kind)
	}
	if err := project.ValidateSegment(scope.ID); err != nil {
		return transitionError("invalid scope ID: %v", err)
	}
	return nil
}

func validRun(run core.RunID) error {
	if run == "" {
		return nil
	}
	if err := project.ValidateSegment(string(run)); err != nil {
		return transitionError("invalid run ID: %v", err)
	}
	return nil
}

func knownState(state core.TaskState) bool {
	switch state {
	case core.Ready, core.Idle, core.Working, core.Implementing,
		core.Reviewing, core.Fix, core.Clean, core.Gated, core.Integrated,
		core.Paused, core.Blocked, core.Interrupted, core.Cancelled, core.Archived:
		return true
	default:
		return false
	}
}

func digestValue(event Event) string {
	if event.CheckpointDigest != "" {
		return event.CheckpointDigest
	}
	return event.Digest
}

func validDigest(value string) bool {
	if !strings.HasPrefix(value, "sha256:") {
		return false
	}
	hexValue := strings.TrimPrefix(value, "sha256:")
	if len(hexValue) != 64 {
		return false
	}
	for _, r := range hexValue {
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')) {
			return false
		}
	}
	return true
}

// Transition validates and applies one event. It does not inspect or mutate
// any other scope: scope isolation is therefore structural rather than an
// inferred side effect.
func Transition(state core.TaskState, event Event) (core.TaskState, error) {
	if !knownState(state) {
		return "", transitionError("unknown current state %q", state)
	}
	if err := validRun(event.Run); err != nil {
		return "", err
	}
	if err := validScope(event.Scope); err != nil {
		return "", err
	}
	if event.Scope.Kind == core.ScopeRun && event.Run != "" && event.Scope.ID != string(event.Run) {
		return "", transitionError("run scope %q does not match event run %q", event.Scope.ID, event.Run)
	}
	if event.From != "" {
		if !knownState(event.From) || event.From != state {
			return "", transitionError("from state %q does not match current state %q", event.From, state)
		}
	}
	if event.To != "" && !knownState(event.To) {
		return "", transitionError("unknown target state %q", event.To)
	}

	var want core.TaskState
	switch event.Kind {
	case Pause:
		if !event.AdmissionHeld || !event.RefillHeld {
			return "", transitionError("pause requires admission and refill holds")
		}
		if event.Run != "" && strings.TrimSpace(event.Reason) == "" {
			return "", transitionError("pause requires a reason")
		}
		if event.Scope.Kind == core.ScopeProject && !event.Confirmed {
			return "", transitionError("project pause requires confirmation")
		}
		if state == core.Clean || state == core.Integrated || state == core.Cancelled || state == core.Archived {
			return "", transitionError("state %q cannot be paused", state)
		}
		want = core.Paused
	case Stop:
		if !event.AdmissionHeld || !event.RefillHeld {
			return "", transitionError("stop requires admission and refill holds")
		}
		if strings.TrimSpace(event.Reason) == "" {
			return "", transitionError("stop requires a reason")
		}
		if event.Scope.Kind == core.ScopeProject && !event.Confirmed {
			return "", transitionError("project stop requires confirmation")
		}
		if state == core.Clean || state == core.Integrated || state == core.Cancelled || state == core.Archived {
			return "", transitionError("state %q cannot be stopped", state)
		}
		want = core.Interrupted
	case Cancel:
		if !event.Confirmed || strings.TrimSpace(event.Reason) == "" {
			return "", transitionError("cancel requires explicit confirmation and a reason")
		}
		if state == core.Cancelled || state == core.Archived || state == core.Integrated {
			return "", transitionError("state %q cannot be cancelled", state)
		}
		want = core.Cancelled
	case Resume:
		if state != core.Paused && state != core.Blocked && state != core.Interrupted {
			return "", transitionError("state %q cannot be resumed", state)
		}
		want = core.Ready
	case EventCheckpoint:
		if !event.AdmissionHeld || !event.RefillHeld {
			return "", transitionError("checkpoint requires admission and refill holds")
		}
		if event.Revision == 0 || !validDigest(digestValue(event)) {
			return "", transitionError("checkpoint requires a valid digest and revision")
		}
		// A checkpoint is evidence, not a hidden lifecycle mutation. In
		// particular, interrupted remains interrupted until an explicit resume.
		want = state
	default:
		return "", transitionError("unknown event kind %q", event.Kind)
	}
	if event.To != "" && event.To != want {
		return "", transitionError("target state %q does not match event %q", event.To, event.Kind)
	}
	return want, nil
}

func checkpointPath(run core.RunID, scope core.Scope) string {
	return path.Join(".agent-team", "checkpoints", string(run), string(scope.Kind)+"-"+scope.ID+".json")
}

func checkpointLimit(s *store.Store) int64 {
	if s != nil && s.Limits.CanonicalBytes > 0 && s.Limits.CanonicalBytes < 16<<20 {
		return s.Limits.CanonicalBytes
	}
	return 16 << 20
}

func validateCheckpointRecord(record CheckpointRecord, run core.RunID, scope core.Scope) error {
	if record.Schema != 1 || record.RunID != run || record.Scope != scope || record.Revision != 1 || !validDigest(record.Digest) {
		return fmt.Errorf("%w: invalid checkpoint record", core.ErrRevision)
	}
	if _, err := time.Parse(time.RFC3339Nano, record.WrittenAt); err != nil {
		return fmt.Errorf("%w: invalid checkpoint timestamp", core.ErrRevision)
	}
	return nil
}

// Checkpoint atomically records one bounded checkpoint. Existing identical
// bytes are idempotent; a changed digest is a revision conflict. The function
// only updates receipts that already exist and match the supplied run/scope.
func Checkpoint(ctx context.Context, s *store.Store, run core.RunID, scope core.Scope, digest string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if s == nil {
		return fmt.Errorf("%w: nil store", core.ErrPath)
	}
	if err := project.ValidateSegment(string(run)); err != nil {
		return err
	}
	if err := validScopeForCheckpoint(scope); err != nil {
		return err
	}
	if scope.Kind == core.ScopeRun && scope.ID != string(run) {
		return fmt.Errorf("%w: run scope does not match run", core.ErrTransition)
	}
	if !validDigest(digest) {
		return fmt.Errorf("%w: invalid checkpoint digest", core.ErrRevision)
	}
	relative := checkpointPath(run, scope)
	mutex := lockFor(filepath.Join(filepath.Clean(s.Root), filepath.FromSlash(relative)))
	mutex.Lock()
	defer mutex.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	var previous CheckpointRecord
	err := s.ReadJSON(relative, checkpointLimit(s), &previous)
	if err == nil {
		if validateErr := validateCheckpointRecord(previous, run, scope); validateErr != nil {
			return validateErr
		}
		if previous.Digest != digest {
			return fmt.Errorf("%w: checkpoint digest changed", core.ErrRevision)
		}
		return nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	record := CheckpointRecord{Schema: 1, RunID: run, Scope: scope, Revision: 1, Digest: digest, WrittenAt: time.Now().UTC().Format(time.RFC3339Nano)}
	if _, err := s.WriteJSON(relative, record, checkpointLimit(s)); err != nil {
		return err
	}
	if err := updateExistingReceipts(ctx, s, run, scope, relative); err != nil {
		return err
	}
	return nil
}

func validScopeForCheckpoint(scope core.Scope) error {
	switch scope.Kind {
	case core.ScopeProject, core.ScopeRun, core.ScopeTeam, core.ScopeTask:
	default:
		return fmt.Errorf("%w: unknown scope kind", core.ErrTransition)
	}
	if err := project.ValidateSegment(scope.ID); err != nil {
		return err
	}
	return nil
}

func updateExistingReceipts(ctx context.Context, s *store.Store, run core.RunID, scope core.Scope, checkpoint string) error {
	receiptRoot := filepath.Join(s.Root, ".agent-team", "receipts")
	entries, err := os.ReadDir(receiptRoot)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("%w: read receipts: %v", core.ErrPath, err)
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		name := strings.TrimSuffix(entry.Name(), ".json")
		if err := project.ValidateSegment(name); err != nil {
			continue
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		relative := path.Join(".agent-team", "receipts", entry.Name())
		var receipt knowledge.Receipt
		if err := s.ReadJSON(relative, checkpointLimit(s), &receipt); err != nil {
			return err
		}
		if receipt.RunID != run || !receiptApplies(receipt, scope) {
			continue
		}
		found := false
		for _, pointer := range receipt.EvidencePointers {
			if pointer == checkpoint {
				found = true
				break
			}
		}
		if found {
			continue
		}
		receipt.EvidencePointers = append(receipt.EvidencePointers, checkpoint)
		receipt.Revision++
		if err := knowledge.WriteReceipt(ctx, s, receipt); err != nil {
			return err
		}
	}
	return nil
}

func receiptApplies(receipt knowledge.Receipt, scope core.Scope) bool {
	switch scope.Kind {
	case core.ScopeTask:
		return receipt.Task == scope.ID
	case core.ScopeTeam:
		return receipt.Team == scope.ID
	case core.ScopeRun, core.ScopeProject:
		return true
	default:
		return false
	}
}
