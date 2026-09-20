// Package orchestrator provides foreground-only canonical run control.
package orchestrator

import (
	"context"
	"fmt"

	"github.com/thebpandey/agent-team/vnext/internal/core"
	"github.com/thebpandey/agent-team/vnext/internal/dispatch"
	"github.com/thebpandey/agent-team/vnext/internal/gate"
	"github.com/thebpandey/agent-team/vnext/internal/integrate"
	"github.com/thebpandey/agent-team/vnext/internal/review"
	"github.com/thebpandey/agent-team/vnext/internal/run"
	"github.com/thebpandey/agent-team/vnext/internal/store"
	"github.com/thebpandey/agent-team/vnext/internal/supervise"
	"github.com/thebpandey/agent-team/vnext/internal/tracker"
)

// Orchestrator coordinates only explicitly supplied execution boundaries.
type Orchestrator interface {
	Start(context.Context, core.RunID) error
	Execute(context.Context, core.RunID) error
	Resume(context.Context, core.RunID) error
}

type foreground struct{ store *store.Store }

func New(state *store.Store, _ tracker.Tracker, _ dispatch.Dispatcher, _ supervise.Supervisor, _ review.Reviewer, _ gate.Gate, _ integrate.Integrator) Orchestrator {
	return &foreground{store: state}
}

func NewOrchestrator(state *store.Store, tasks tracker.Tracker, dispatcher dispatch.Dispatcher, supervisor supervise.Supervisor, reviewer review.Reviewer, gate gate.Gate, integrator integrate.Integrator) Orchestrator {
	return New(state, tasks, dispatcher, supervisor, reviewer, gate, integrator)
}

func (o *foreground) Start(ctx context.Context, id core.RunID) error {
	return o.control(ctx, id, false)
}
func (o *foreground) Execute(ctx context.Context, id core.RunID) error {
	return o.control(ctx, id, false)
}
func (o *foreground) Resume(ctx context.Context, id core.RunID) error {
	return o.control(ctx, id, true)
}

func (o *foreground) control(ctx context.Context, id core.RunID, resume bool) error {
	if ctx == nil || ctx.Err() != nil || o == nil || o.store == nil {
		return core.ErrTransition
	}
	manifest, err := run.NewRepositories(o.store).Runs.Read(ctx, id)
	if err != nil {
		return fmt.Errorf("%w: canonical run: %v", core.ErrRevision, err)
	}
	if manifest.ID != id || manifest.State == core.Cancelled || manifest.State == core.Archived || (!resume && (manifest.State == core.Paused || manifest.State == core.Interrupted)) {
		return core.ErrTransition
	}
	// Task assignment and candidate construction are owned by the later
	// acceptance layer. This control surface deliberately does not fabricate
	// packets, worktrees, or evidence to bypass those boundaries.
	return core.ErrPhase
}
