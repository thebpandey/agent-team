package lifecycle_test

import (
	"context"
	"errors"
	"testing"

	"github.com/thebpandey/agent-team/vnext/internal/core"
	"github.com/thebpandey/agent-team/vnext/internal/lifecycle"
	"github.com/thebpandey/agent-team/vnext/internal/run"
	"github.com/thebpandey/agent-team/vnext/internal/store"
)

func TestScopedResumeIsDurableAndIdempotent(t *testing.T) {
	s := store.New(t.TempDir(), core.StorageLimits{CanonicalBytes: 16 << 20})
	manifest, err := run.CreateOneOff(context.Background(), t.TempDir(), run.OneOffFeature, "x", []core.Task{{ID: "T1", Objective: "x", State: core.Ready, Criteria: []string{"done"}, WritablePaths: []string{"x"}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := run.NewRepositories(s).Runs.Initialize(context.Background(), manifest); err != nil {
		t.Fatal(err)
	}
	l := lifecycle.New(s, nil)
	scope := core.Scope{Kind: core.ScopeTask, ID: "T1"}
	if err := l.Pause(context.Background(), scope, "user"); err != nil {
		t.Fatal(err)
	}
	if err := l.Pause(context.Background(), scope, "user"); err != nil {
		t.Fatal(err)
	}
	if err := l.Resume(context.Background(), scope); err != nil {
		t.Fatal(err)
	}
	if err := l.Resume(context.Background(), scope); err != nil {
		t.Fatal(err)
	}
}

func TestTaskScopeWithoutUniqueCanonicalMembershipFailsClosed(t *testing.T) {
	l := lifecycle.New(store.New(t.TempDir(), core.StorageLimits{CanonicalBytes: 16 << 20}), nil)
	if err := l.Pause(context.Background(), core.Scope{Kind: core.ScopeTask, ID: "T1"}, "user"); !errors.Is(err, core.ErrTransition) {
		t.Fatal(err)
	}
}

func TestTaskScopeWithAmbiguousCanonicalMembershipFailsClosed(t *testing.T) {
	s := store.New(t.TempDir(), core.StorageLimits{CanonicalBytes: 16 << 20})
	for _, objective := range []string{"x", "y"} {
		manifest, err := run.CreateOneOff(context.Background(), t.TempDir(), run.OneOffFeature, objective, []core.Task{{ID: "T1", Objective: objective, State: core.Ready, Criteria: []string{"done"}, WritablePaths: []string{"x"}}})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := run.NewRepositories(s).Runs.Initialize(context.Background(), manifest); err != nil {
			t.Fatal(err)
		}
	}
	if err := lifecycle.New(s, nil).Pause(context.Background(), core.Scope{Kind: core.ScopeTask, ID: "T1"}, "user"); !errors.Is(err, core.ErrTransition) {
		t.Fatal(err)
	}
}

type lookup struct{ team, task bool }

func (l lookup) TeamMember(context.Context, core.RunID, core.TeamID) (bool, error) {
	return l.team, nil
}
func (l lookup) TaskMember(context.Context, core.RunID, core.TaskID) (bool, error) {
	return l.task, nil
}

func TestResolveScopeRejectsAmbiguityAndNonMembers(t *testing.T) {
	active := []core.RunID{"R1"}
	if got, err := lifecycle.ResolveScope(context.Background(), nil, active, lookup{}); err != nil || got != (core.Scope{Kind: core.ScopeRun, ID: "R1"}) {
		t.Fatalf("scope=%+v err=%v", got, err)
	}
	for _, active := range [][]core.RunID{nil, {"R1", "R2"}} {
		if _, err := lifecycle.ResolveScope(context.Background(), nil, active, lookup{}); !errors.Is(err, core.ErrTransition) {
			t.Fatalf("active=%v err=%v", active, err)
		}
	}
	if _, err := lifecycle.ResolveScope(context.Background(), []string{"--team", "TEAM"}, active, lookup{}); !errors.Is(err, core.ErrTransition) {
		t.Fatal(err)
	}
	if _, err := lifecycle.ResolveScope(context.Background(), []string{"--task", "TASK"}, active, lookup{task: true}); err != nil {
		t.Fatal(err)
	}
}

func TestCheckpointAndCancelFailClosedOnConflict(t *testing.T) {
	s := store.New(t.TempDir(), core.StorageLimits{CanonicalBytes: 16 << 20})
	l := lifecycle.New(s, nil)
	scope := core.Scope{Kind: core.ScopeRun, ID: "R1"}
	digest := "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	if err := l.Checkpoint(context.Background(), scope, digest); err != nil {
		t.Fatal(err)
	}
	if err := l.Checkpoint(context.Background(), scope, digest); err != nil {
		t.Fatal(err)
	}
	if err := l.Checkpoint(context.Background(), scope, "sha256:abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"); !errors.Is(err, core.ErrRevision) {
		t.Fatal(err)
	}
	if err := l.Cancel(context.Background(), scope, "user"); err != nil {
		t.Fatal(err)
	}
	if err := l.Resume(context.Background(), scope); !errors.Is(err, core.ErrTransition) {
		t.Fatal(err)
	}
	if err := l.Pause(context.Background(), core.Scope{Kind: core.ScopeTask, ID: "../bad"}, "user"); !errors.Is(err, core.ErrTransition) {
		t.Fatal(err)
	}
}
