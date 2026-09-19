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
	"github.com/thebpandey/agent-team/vnext/internal/supervise"
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
	supervisor supervise.Supervisor
	mu         *sync.Mutex
}

var locks sync.Map

// New creates a lifecycle controller. All work remains in the calling goroutine.
func New(state *store.Store, supervisor supervise.Supervisor) Lifecycle {
	key := "<nil>"
	if state != nil {
		key = filepath.Clean(state.Root)
	}
	value, _ := locks.LoadOrStore(key, &sync.Mutex{})
	return &controller{store: state, supervisor: supervisor, mu: value.(*sync.Mutex)}
}

// NewLifecycle is retained as the explicit constructor named by the plan.
func NewLifecycle(state *store.Store, supervisor supervise.Supervisor) Lifecycle {
	return New(state, supervisor)
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
	if scope.Kind == core.ScopeRun && c.supervisor != nil {
		if err := c.supervisor.Checkpoint(ctx, core.RunID(scope.ID), scope, digest); err != nil {
			return err
		}
	}
	if err := c.transition(ctx, scope, workflow.Event{Scope: scope, Kind: workflow.CheckpointEvent, CheckpointDigest: digest, AdmissionHeld: true, RefillHeld: true}); err != nil {
		return err
	}
	return nil
}

func (c *controller) transition(ctx context.Context, scope core.Scope, event workflow.Event) error {
	if ctx == nil || ctx.Err() != nil || c == nil || c.store == nil {
		return core.ErrTransition
	}
	if err := validScope(scope); err != nil {
		return err
	}
	bound, err := c.resolve(scope)
	if err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	path := recordPath(scope)
	current := record{Scope: scope, State: core.Working}
	err = c.store.ReadJSON(path, 64<<10, &current)
	exists := err == nil
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if exists && (current.Scope != scope || current.Run != bound || current.Revision == 0) {
		return core.ErrRevision
	}
	if exists && sameTransition(current, event) {
		return nil
	}
	next, err := workflow.Transition(current.State, event)
	if err != nil {
		return err
	}
	current.State = next
	current.Run = bound
	current.Reason = event.Reason
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
	_, err = c.store.WriteJSON(path, current, 64<<10)
	return err
}

// resolve binds team/task scope only to one complete canonical run. The
// Store is project-scoped; arbitrary task identifiers are never authority.
func (c *controller) resolve(scope core.Scope) (core.RunID, error) {
	if scope.Kind == core.ScopeProject {
		return "", nil
	}
	if scope.Kind == core.ScopeRun {
		return core.RunID(scope.ID), nil
	}
	root, err := os.OpenRoot(c.store.Root)
	if err != nil {
		return "", fmt.Errorf("%w: open canonical store root: %v", core.ErrTransition, err)
	}
	defer root.Close()
	const directory = ".agent-team/runs"
	info, err := root.Lstat(directory)
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return "", fmt.Errorf("%w: canonical run directory: %v", core.ErrTransition, err)
	}
	dir, err := root.Open(directory)
	if err != nil {
		return "", fmt.Errorf("%w: open canonical run directory: %v", core.ErrTransition, err)
	}
	defer dir.Close()
	entries, err := dir.ReadDir(-1)
	if err != nil || len(entries) > 64 {
		return "", fmt.Errorf("%w: list canonical runs: %v", core.ErrTransition, err)
	}
	var found core.RunID
	for _, entry := range entries {
		// Store's same-directory atomicity probe can leave its owned destination
		// marker behind after an interrupted probe. It is not a canonical record.
		if strings.HasPrefix(entry.Name(), ".agent-team-probe-to-") && !entry.IsDir() && entry.Type()&os.ModeSymlink == 0 {
			continue
		}
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 || !strings.HasSuffix(entry.Name(), ".json") {
			return "", fmt.Errorf("%w: malformed canonical run entry %q", core.ErrTransition, entry.Name())
		}
		id := strings.TrimSuffix(entry.Name(), ".json")
		if err := project.ValidateSegment(id); err != nil {
			return "", fmt.Errorf("%w: invalid canonical run ID", core.ErrTransition)
		}
		manifest, err := run.NewRepositories(c.store).Runs.Read(context.Background(), core.RunID(id))
		if err != nil {
			return "", fmt.Errorf("%w: invalid canonical run: %v", core.ErrTransition, err)
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
		if !match {
			continue
		}
		if found != "" {
			return "", core.ErrTransition
		}
		found = manifest.ID
	}
	if found == "" {
		return "", core.ErrTransition
	}
	return found, nil
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
	case core.ScopeProject, core.ScopeRun, core.ScopeTeam, core.ScopeTask:
	default:
		return core.ErrTransition
	}
	if err := project.ValidateSegment(scope.ID); err != nil {
		return core.ErrTransition
	}
	return nil
}

func recordPath(scope core.Scope) string {
	return ".agent-team/lifecycle/" + string(scope.Kind) + "-" + scope.ID + ".json"
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
