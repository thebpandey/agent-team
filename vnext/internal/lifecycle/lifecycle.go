// Package lifecycle persists foreground scoped control transitions.
package lifecycle

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/thebpandey/agent-team/vnext/internal/core"
	"github.com/thebpandey/agent-team/vnext/internal/project"
	"github.com/thebpandey/agent-team/vnext/internal/run"
	"github.com/thebpandey/agent-team/vnext/internal/store"
	"github.com/thebpandey/agent-team/vnext/internal/workflow"
)

// Lifecycle is the scoped foreground control surface.
type Lifecycle interface {
	Pause(context.Context, core.Scope, string) error
	Stop(context.Context, core.Scope, string) error
	Cancel(context.Context, core.Scope, string) error
	Resume(context.Context, core.Scope) error
	Checkpoint(context.Context, core.Scope, string) error
}

// ParsedAction is the accepted lifecycle intent after command parsing.
type ParsedAction struct {
	Name          string
	Selector      []string
	ScopeRequired bool
}

// ScopeLookup supplies canonical membership for a selector whose run is known.
type ScopeLookup interface {
	TeamMember(context.Context, core.RunID, core.TeamID) (bool, error)
	TaskMember(context.Context, core.RunID, core.TaskID) (bool, error)
}

type record struct {
	Scope            core.Scope     `json:"scope"`
	Run              core.RunID     `json:"runId,omitempty"`
	State            core.TaskState `json:"state"`
	Reason           string         `json:"reason,omitempty"`
	CheckpointDigest string         `json:"checkpointDigest,omitempty"`
	Revision         uint64         `json:"revision"`
}

type controller struct {
	store      *store.Store
	supervisor EventSink
	mu         *sync.Mutex
	err        error
}

// EventSink is the narrow foreground boundary lifecycle needs from a host.
type EventSink interface {
	Emit(context.Context, workflow.Event) error
	Checkpoint(context.Context, core.RunID, core.Scope, string) error
}

type target struct {
	run   core.RunID
	scope core.Scope
}

var locks sync.Map

// New creates a lifecycle controller. All work remains in the calling goroutine.
func New(state *store.Store, supervisor EventSink) Lifecycle {
	key, err := canonicalStoreRoot(state)
	if err != nil {
		key = "<invalid-store-root>"
	}
	value, _ := locks.LoadOrStore(key, &sync.Mutex{})
	if err != nil {
		return &controller{supervisor: supervisor, mu: value.(*sync.Mutex), err: err}
	}
	return &controller{store: store.New(key, state.Limits), supervisor: supervisor, mu: value.(*sync.Mutex)}
}

// NewLifecycle is retained as the explicit constructor named by the plan.
func NewLifecycle(state *store.Store, supervisor EventSink) Lifecycle {
	return New(state, supervisor)
}

// AdmissionAllowed reads the run and project barriers that can prevent a new
// assignment before a caller constructs any host request.
func AdmissionAllowed(ctx context.Context, state *store.Store, packet core.AssignmentPacket) error {
	return WithAdmission(ctx, state, packet, nil)
}

// WithAdmission keeps packet membership, barrier validation, and a foreground
// start callback under the same root-wide lock used by lifecycle transitions.
func WithAdmission(ctx context.Context, state *store.Store, packet core.AssignmentPacket, callback func() error) error {
	if ctx == nil || ctx.Err() != nil || state == nil || packet.RunID == "" || packet.Team == "" || packet.Task == "" {
		return core.ErrTransition
	}
	root, err := canonicalStoreRoot(state)
	if err != nil {
		return core.ErrPath
	}
	state = store.New(root, state.Limits)
	value, _ := locks.LoadOrStore(root, &sync.Mutex{})
	mu := value.(*sync.Mutex)
	mu.Lock()
	defer mu.Unlock()
	if err := admissionAllowedUnlocked(ctx, state, packet); err != nil {
		return err
	}
	if callback != nil {
		return callback()
	}
	return nil
}

func admissionAllowedUnlocked(ctx context.Context, state *store.Store, packet core.AssignmentPacket) error {
	manifest, err := run.NewRepositories(state).Runs.Read(ctx, packet.RunID)
	if err != nil {
		return core.ErrTransition
	}
	teamOK, taskOK := false, false
	for _, team := range manifest.Teams {
		if team.ID != packet.Team {
			continue
		}
		teamOK = true
		for _, task := range team.Queue {
			if task == packet.Task {
				taskOK = true
				break
			}
		}
	}
	if !teamOK || !taskOK {
		return core.ErrTransition
	}
	for _, scope := range []core.Scope{{Kind: core.ScopeProject, ID: manifest.Project}, {Kind: core.ScopeRun, ID: string(packet.RunID)}, {Kind: core.ScopeTeam, ID: string(packet.Team)}, {Kind: core.ScopeTask, ID: string(packet.Task)}} {
		var barrier record
		err := state.ReadJSON(recordPath(packet.RunID, scope), 64<<10, &barrier)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return core.ErrRevision
		}
		if err := validBarrierRecord(barrier, packet.RunID, scope); err != nil {
			return core.ErrRevision
		}
		switch barrier.State {
		case core.Paused, core.Interrupted, core.Cancelled:
			return core.ErrTransition
		}
	}
	return nil
}

func validBarrierRecord(barrier record, runID core.RunID, scope core.Scope) error {
	if barrier.Run != runID || barrier.Scope != scope || barrier.Revision == 0 || len(barrier.Reason) > 4096 || strings.Contains(barrier.Reason, "\x00") {
		return core.ErrRevision
	}
	if barrier.CheckpointDigest != "" && !validDigest(barrier.CheckpointDigest) {
		return core.ErrRevision
	}
	switch barrier.State {
	case core.Ready, core.Paused, core.Interrupted, core.Cancelled:
		return nil
	case core.Working, core.Implementing, core.Reviewing, core.Fix, core.Clean, core.Gated, core.Integrated, core.Blocked, core.Archived, core.Idle:
		if barrier.CheckpointDigest != "" {
			return nil
		}
	}
	return core.ErrRevision
}

func (c *controller) Pause(ctx context.Context, scope core.Scope, reason string) error {
	return c.transition(ctx, scope, workflow.Event{Scope: scope, Kind: workflow.Pause, Reason: reason, Confirmed: true, AdmissionHeld: true, RefillHeld: true})
}

func (c *controller) Stop(ctx context.Context, scope core.Scope, reason string) error {
	return c.transition(ctx, scope, workflow.Event{Scope: scope, Kind: workflow.Stop, Reason: reason, Confirmed: true, AdmissionHeld: true, RefillHeld: true})
}

func (c *controller) Cancel(ctx context.Context, scope core.Scope, reason string) error {
	return c.transition(ctx, scope, workflow.Event{Scope: scope, Kind: workflow.Cancel, Reason: reason, Confirmed: true})
}

func (c *controller) Resume(ctx context.Context, scope core.Scope) error {
	return c.transition(ctx, scope, workflow.Event{Scope: scope, Kind: workflow.Resume})
}

func (c *controller) Checkpoint(ctx context.Context, scope core.Scope, digest string) error {
	if !validDigest(digest) {
		return core.ErrRevision
	}
	if err := c.transition(ctx, scope, workflow.Event{Scope: scope, Kind: workflow.CheckpointEvent, CheckpointDigest: digest, AdmissionHeld: true, RefillHeld: true}); err != nil {
		return err
	}
	return nil
}

func (c *controller) transition(ctx context.Context, scope core.Scope, event workflow.Event) error {
	if ctx == nil || ctx.Err() != nil || c == nil {
		return core.ErrTransition
	}
	if c.err != nil {
		return c.err
	}
	if c.store == nil {
		return core.ErrTransition
	}
	targets, err := c.resolve(scope)
	if err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	next := make([]record, len(targets))
	for i, target := range targets {
		current, exists, err := c.read(target)
		if err != nil {
			return err
		}
		if exists && sameTransition(current, event) {
			next[i] = current
			continue
		}
		event.Run, event.Scope = target.run, target.scope
		state, err := workflow.Transition(current.State, event)
		if err != nil {
			return err
		}
		current.State, current.Run, current.Scope, current.Reason = state, target.run, target.scope, event.Reason
		if event.Kind == workflow.CheckpointEvent {
			if current.CheckpointDigest != "" && current.CheckpointDigest != event.CheckpointDigest {
				return core.ErrRevision
			}
			current.CheckpointDigest = event.CheckpointDigest
		}
		if exists {
			current.Revision++
		} else {
			current.Revision = 1
		}
		next[i] = current
	}
	if c.supervisor == nil {
		return core.ErrCapacity
	}
	// Emit before publishing a resume barrier so a failed supervisor call can
	// never reopen admission. All calls are synchronous foreground work.
	restrictive := event.Kind == workflow.Pause || event.Kind == workflow.Stop || event.Kind == workflow.Cancel
	for i, target := range targets {
		event.Run, event.Scope = target.run, target.scope
		if err := c.supervisor.Emit(ctx, event); err != nil {
			return err
		}
		if event.Kind == workflow.CheckpointEvent {
			if err := c.supervisor.Checkpoint(ctx, target.run, target.scope, event.CheckpointDigest); err != nil {
				return err
			}
		}
		if restrictive {
			if _, err := c.store.WriteJSON(recordPath(next[i].Run, next[i].Scope), next[i], 64<<10); err != nil {
				return err
			}
		}
	}
	if restrictive {
		return nil
	}
	for _, value := range next {
		if _, err := c.store.WriteJSON(recordPath(value.Run, value.Scope), value, 64<<10); err != nil {
			return err
		}
	}
	return nil
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

// resolve binds team/task scope only to one complete canonical run. The
// Store is project-scoped; arbitrary task identifiers are never authority.
func (c *controller) resolve(scope core.Scope) ([]target, error) {
	if err := validScope(scope); err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(c.store.Root)
	if err != nil {
		return nil, fmt.Errorf("%w: open canonical store root: %v", core.ErrTransition, err)
	}
	defer root.Close()
	const directory = ".agent-team/runs"
	info, err := root.Lstat(directory)
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return nil, fmt.Errorf("%w: canonical run directory: %v", core.ErrTransition, err)
	}
	dir, err := root.Open(directory)
	if err != nil {
		return nil, fmt.Errorf("%w: open canonical run directory: %v", core.ErrTransition, err)
	}
	defer dir.Close()
	entries, err := dir.ReadDir(-1)
	if err != nil || len(entries) > 64 {
		return nil, fmt.Errorf("%w: list canonical runs: %v", core.ErrTransition, err)
	}
	var found []target
	for _, entry := range entries {
		// Store's same-directory atomicity probe can leave its owned destination
		// marker behind after an interrupted probe. It is not a canonical record.
		if strings.HasPrefix(entry.Name(), ".agent-team-probe-to-") && !entry.IsDir() && entry.Type()&os.ModeSymlink == 0 {
			continue
		}
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 || !strings.HasSuffix(entry.Name(), ".json") {
			return nil, fmt.Errorf("%w: malformed canonical run entry %q", core.ErrTransition, entry.Name())
		}
		id := strings.TrimSuffix(entry.Name(), ".json")
		if err := project.ValidateSegment(id); err != nil {
			return nil, fmt.Errorf("%w: invalid canonical run ID", core.ErrTransition)
		}
		manifest, err := run.NewRepositories(c.store).Runs.Read(context.Background(), core.RunID(id))
		if err != nil {
			return nil, fmt.Errorf("%w: invalid canonical run: %v", core.ErrTransition, err)
		}
		if scope.Kind == core.ScopeRun {
			if manifest.ID == core.RunID(scope.ID) {
				found = append(found, target{run: manifest.ID, scope: scope})
			}
			continue
		}
		if scope.Kind == core.ScopeProject {
			if manifest.Project == scope.ID {
				found = append(found, target{run: manifest.ID, scope: scope})
			}
			continue
		}
		match := false
		if scope.Kind == core.ScopeTeam {
			for _, team := range manifest.Teams {
				if string(team.ID) == scope.ID {
					match = true
					break
				}
			}
		} else {
			for _, task := range manifest.Tasks {
				if string(task.ID) == scope.ID {
					match = true
					break
				}
			}
		}
		if match {
			found = append(found, target{run: manifest.ID, scope: scope})
		}
	}
	if len(found) == 0 || (scope.Kind != core.ScopeProject && len(found) != 1) {
		return nil, core.ErrTransition
	}
	return found, nil
}

func (c *controller) read(target target) (record, bool, error) {
	current := record{Scope: target.scope, Run: target.run, State: core.Working}
	err := c.store.ReadJSON(recordPath(target.run, target.scope), 64<<10, &current)
	if errors.Is(err, os.ErrNotExist) {
		return current, false, nil
	}
	if err != nil {
		return record{}, false, err
	}
	if err := validBarrierRecord(current, target.run, target.scope); err != nil {
		return record{}, false, err
	}
	return current, true, nil
}

func sameTransition(current record, event workflow.Event) bool {
	switch event.Kind {
	case workflow.Pause:
		return current.State == core.Paused && current.Reason == event.Reason
	case workflow.Stop:
		return current.State == core.Interrupted && current.Reason == event.Reason
	case workflow.Cancel:
		return current.State == core.Cancelled && current.Reason == event.Reason
	case workflow.Resume:
		return current.State == core.Ready
	case workflow.CheckpointEvent:
		return current.CheckpointDigest == event.CheckpointDigest
	default:
		return false
	}
}

func validScope(scope core.Scope) error {
	switch scope.Kind {
	case core.ScopeProject:
		canonical, err := project.Contain(scope.ID, scope.ID)
		if err != nil || canonical != scope.ID {
			return core.ErrTransition
		}
		return nil
	case core.ScopeRun, core.ScopeTeam, core.ScopeTask:
	default:
		return core.ErrTransition
	}
	if err := project.ValidateSegment(scope.ID); err != nil {
		return core.ErrTransition
	}
	return nil
}

func recordPath(run core.RunID, scope core.Scope) string {
	sum := sha256.Sum256([]byte(string(scope.Kind) + "\x00" + scope.ID))
	return ".agent-team/lifecycle/" + string(run) + "/" + string(scope.Kind) + "-" + hex.EncodeToString(sum[:]) + ".json"
}

func validDigest(value string) bool {
	if !strings.HasPrefix(value, "sha256:") || len(value) != len("sha256:")+sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(strings.TrimPrefix(value, "sha256:"))
	return err == nil
}

// ResolveScope accepts only an unambiguous active run or a canonically proven member.
func ResolveScope(ctx context.Context, args []string, active []core.RunID, lookup ScopeLookup) (core.Scope, error) {
	if ctx == nil || ctx.Err() != nil {
		return core.Scope{}, core.ErrTransition
	}
	if len(args) == 0 {
		if len(active) != 1 {
			return core.Scope{}, core.ErrTransition
		}
		return core.Scope{Kind: core.ScopeRun, ID: string(active[0])}, nil
	}
	if len(args) != 2 || strings.TrimSpace(args[1]) == "" {
		return core.Scope{}, core.ErrTransition
	}
	scope := core.Scope{ID: args[1]}
	switch args[0] {
	case "--project":
		scope.Kind = core.ScopeProject
	case "--run":
		for _, id := range active {
			if string(id) == scope.ID {
				scope.Kind = core.ScopeRun
				break
			}
		}
		if scope.Kind == "" {
			return core.Scope{}, core.ErrTransition
		}
	case "--team":
		if len(active) != 1 || lookup == nil {
			return core.Scope{}, core.ErrTransition
		}
		ok, err := lookup.TeamMember(ctx, active[0], core.TeamID(scope.ID))
		if err != nil || !ok {
			return core.Scope{}, core.ErrTransition
		}
		scope.Kind = core.ScopeTeam
	case "--task":
		if len(active) != 1 || lookup == nil {
			return core.Scope{}, core.ErrTransition
		}
		ok, err := lookup.TaskMember(ctx, active[0], core.TaskID(scope.ID))
		if err != nil || !ok {
			return core.Scope{}, core.ErrTransition
		}
		scope.Kind = core.ScopeTask
	default:
		return core.Scope{}, core.ErrTransition
	}
	if err := validScope(scope); err != nil {
		return core.Scope{}, err
	}
	return scope, nil
}

// ExecuteLifecycle routes only already-parsed lifecycle actions.
func ExecuteLifecycle(ctx context.Context, action ParsedAction, active []core.RunID, lookup ScopeLookup, lifecycle Lifecycle) error {
	if !action.ScopeRequired || lifecycle == nil {
		return core.ErrTransition
	}
	scope, err := ResolveScope(ctx, action.Selector, active, lookup)
	if err != nil {
		return err
	}
	switch action.Name {
	case "pause":
		return lifecycle.Pause(ctx, scope, "user")
	case "stop":
		return lifecycle.Stop(ctx, scope, "user")
	case "cancel":
		return lifecycle.Cancel(ctx, scope, "user")
	case "resume":
		return lifecycle.Resume(ctx, scope)
	default:
		return fmt.Errorf("%w: unsupported lifecycle action", core.ErrTransition)
	}
}
