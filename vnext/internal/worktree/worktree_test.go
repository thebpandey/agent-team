package worktree

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/thebpandey/agent-team/vnext/internal/contracts"
	"github.com/thebpandey/agent-team/vnext/internal/core"
	"github.com/thebpandey/agent-team/vnext/internal/store"
	"github.com/thebpandey/agent-team/vnext/internal/testkit"
	"github.com/thebpandey/agent-team/vnext/internal/tracker"
)

func TestManagerCreatesOnlyDedicatedWorktreesAndResumesExactIdentity(t *testing.T) {
	repo := testkit.GitRepo(t)
	runner := &gitRunner{}
	manager := NewManager(repo, "project", store.New(repo, core.StorageLimits{CanonicalBytes: 16 << 20}), runner)
	var _ contracts.WorktreeManager = manager

	spec := worktreeSpec("run-1", "team-1", filepath.Join(repo, ".agent-team", "worktrees", "run-1-team-1"))
	got, err := manager.Create(context.Background(), spec)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"git", "-C", repo, "worktree", "add", "-b", "agent-team/run-1/team-1", spec.Root, "base"}
	if !reflect.DeepEqual(runner.calls, [][]string{want}) {
		t.Fatalf("git calls = %#v", runner.calls)
	}
	if retry, err := manager.Create(context.Background(), spec); err != nil || retry != got {
		t.Fatalf("retry = %#v, %v; want %#v", retry, err, got)
	}
	fresh := NewManager(repo, "project", store.New(repo, core.StorageLimits{CanonicalBytes: 16 << 20}), runner)
	if inspected, err := fresh.Inspect(context.Background(), got); err != nil || inspected != got {
		t.Fatalf("fresh Inspect = %#v, %v", inspected, err)
	}
	forged := got
	forged.Path = filepath.Join(repo, ".agent-team", "worktrees", "other")
	if _, err := fresh.Inspect(context.Background(), forged); !errors.Is(err, core.ErrPath) {
		t.Fatalf("forged Inspect error = %v", err)
	}
}

func TestManagerRejectsMainOutsideAndCrossRunBeforeGit(t *testing.T) {
	repo := testkit.GitRepo(t)
	runner := &gitRunner{}
	manager := NewManager(repo, "project", store.New(repo, core.StorageLimits{CanonicalBytes: 16 << 20}), runner)
	for _, spec := range []contracts.WorktreeSpec{
		worktreeSpec("run-1", "team-1", repo),
		worktreeSpec("run-1", "team-1", filepath.Join(repo, "product")),
		worktreeSpec("run-1", "team-1", filepath.Join(t.TempDir(), "outside")),
		{Run: "run-1", Team: "team-1", Root: filepath.Join(repo, ".agent-team", "worktrees", "safe"), Base: "base", WritablePaths: []string{"../../outside"}},
	} {
		if _, err := manager.Create(context.Background(), spec); !errors.Is(err, core.ErrPath) {
			t.Fatalf("Create(%#v) error = %v, want ErrPath", spec, err)
		}
	}
	if len(runner.calls) != 0 {
		t.Fatalf("git was called before validation: %#v", runner.calls)
	}
}

func TestManagerRejectsSymlinkedManagedRootBeforeGit(t *testing.T) {
	repo := testkit.GitRepo(t)
	if err := os.MkdirAll(filepath.Join(repo, ".agent-team"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(t.TempDir(), filepath.Join(repo, ".agent-team", "worktrees")); err != nil {
		t.Fatal(err)
	}
	runner := &gitRunner{}
	manager := NewManager(repo, "project", store.New(repo, core.StorageLimits{CanonicalBytes: 16 << 20}), runner)
	_, err := manager.Create(context.Background(), worktreeSpec("run", "team", filepath.Join(repo, ".agent-team", "worktrees", "task")))
	if !errors.Is(err, core.ErrPath) || len(runner.calls) != 0 {
		t.Fatalf("symlink root Create() = %v, calls %#v", err, runner.calls)
	}
}

func TestManagerSeparatesSameTeamAcrossRunsAndRemovesOnlyExactCleanIdentity(t *testing.T) {
	repo := testkit.GitRepo(t)
	runner := &gitRunner{}
	manager := NewManager(repo, "project", store.New(repo, core.StorageLimits{CanonicalBytes: 16 << 20}), runner)
	a, err := manager.Create(context.Background(), worktreeSpec("run-a", "team", filepath.Join(repo, ".agent-team", "worktrees", "a")))
	if err != nil {
		t.Fatal(err)
	}
	b, err := manager.Create(context.Background(), worktreeSpec("run-b", "team", filepath.Join(repo, ".agent-team", "worktrees", "b")))
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Cleanup(context.Background(), "team"); !errors.Is(err, core.ErrPath) {
		t.Fatalf("ambiguous cleanup error = %v", err)
	}
	dirty := a
	dirty.Dirty = true
	if err := manager.RemoveExact(context.Background(), dirty); !errors.Is(err, core.ErrPath) {
		t.Fatalf("dirty remove error = %v", err)
	}
	if err := manager.RemoveExact(context.Background(), a); err != nil {
		t.Fatal(err)
	}
	if err := manager.RemoveExact(context.Background(), b); err != nil {
		t.Fatal(err)
	}
	if got := runner.calls[len(runner.calls)-4:]; !reflect.DeepEqual(got, [][]string{
		{"git", "-C", repo, "worktree", "remove", a.Path},
		{"git", "-C", repo, "branch", "-d", a.Branch},
		{"git", "-C", repo, "worktree", "remove", b.Path},
		{"git", "-C", repo, "branch", "-d", b.Branch},
	}) {
		t.Fatalf("cleanup calls = %#v", got)
	}
}

func TestManagerRejectsForgedCandidateBeforeGit(t *testing.T) {
	repo := testkit.GitRepo(t)
	runner := &gitRunner{}
	manager := NewManager(repo, "project", store.New(repo, core.StorageLimits{CanonicalBytes: 16 << 20}), runner)
	w, err := manager.Create(context.Background(), worktreeSpec("run", "team", filepath.Join(repo, ".agent-team", "worktrees", "task")))
	if err != nil {
		t.Fatal(err)
	}
	before := len(runner.calls)
	bad := contracts.Candidate{Task: "task", Revision: "candidate", Base: "other", Worktree: w}
	if _, err := manager.Integrate(context.Background(), bad); !errors.Is(err, core.ErrPath) {
		t.Fatalf("Integrate error = %v", err)
	}
	if len(runner.calls) != before {
		t.Fatalf("forged candidate ran git: %#v", runner.calls)
	}
}

func TestManagerRejectsUnknownRemovalBeforeGit(t *testing.T) {
	repo := testkit.GitRepo(t)
	runner := &gitRunner{}
	manager := NewManager(repo, "project", store.New(repo, core.StorageLimits{CanonicalBytes: 16 << 20}), runner)
	err := manager.RemoveExact(context.Background(), contracts.Worktree{Run: "run", Team: "team", Path: filepath.Join(repo, ".agent-team", "worktrees", "unknown"), Branch: "agent-team/run/team"})
	if !errors.Is(err, core.ErrPath) || len(runner.calls) != 0 {
		t.Fatalf("unknown RemoveExact() = %v, calls %#v", err, runner.calls)
	}
}

func TestManagerCompensatesWhenDurableStateFails(t *testing.T) {
	repo := testkit.GitRepo(t)
	if err := os.MkdirAll(filepath.Join(repo, ".agent-team"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(t.TempDir(), filepath.Join(repo, ".agent-team", "runtime")); err != nil {
		t.Fatal(err)
	}
	runner := &gitRunner{}
	manager := NewManager(repo, "project", store.New(repo, core.StorageLimits{CanonicalBytes: 16 << 20}), runner)
	_, err := manager.Create(context.Background(), worktreeSpec("run", "team", filepath.Join(repo, ".agent-team", "worktrees", "task")))
	if err == nil {
		t.Fatal("Create() succeeded with state publication through symlink")
	}
	if got, want := runner.calls, [][]string{
		{"git", "-C", repo, "worktree", "add", "-b", "agent-team/run/team", filepath.Join(repo, ".agent-team", "worktrees", "task"), "base"},
		{"git", "-C", repo, "worktree", "remove", filepath.Join(repo, ".agent-team", "worktrees", "task")},
		{"git", "-C", repo, "branch", "-d", "agent-team/run/team"},
	}; !reflect.DeepEqual(got, want) {
		t.Fatalf("state failure calls = %#v", got)
	}
}

type gitRunner struct {
	calls  [][]string
	result tracker.CommandResult
}

func (r *gitRunner) Run(_ context.Context, name string, args ...string) tracker.CommandResult {
	r.calls = append(r.calls, append([]string{name}, args...))
	return r.result
}

func worktreeSpec(run core.RunID, team core.TeamID, root string) contracts.WorktreeSpec {
	return contracts.WorktreeSpec{Run: run, Team: team, Root: root, Base: "base", WritablePaths: []string{"src"}}
}
