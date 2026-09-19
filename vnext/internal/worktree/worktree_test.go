package worktree

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
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
	if !reflect.DeepEqual(mutations(runner.calls), [][]string{want}) {
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
	if got := mutations(runner.calls)[2:]; !reflect.DeepEqual(got, [][]string{
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
	if len(runner.calls) != 0 {
		t.Fatalf("state failure calls = %#v", runner.calls)
	}
}

func TestManagerPersistsCreatingIntentBeforeGit(t *testing.T) {
	repo := testkit.GitRepo(t)
	runner := &gitRunner{}
	manager := NewManager(repo, "project", store.New(repo, core.StorageLimits{CanonicalBytes: 16 << 20}), runner)
	manager.create = func(string, WorktreeIdentity) error { return fmt.Errorf("injected write failure") }
	_, err := manager.Create(context.Background(), worktreeSpec("run", "team", filepath.Join(repo, ".agent-team", "worktrees", "task")))
	if err == nil || len(runner.calls) != 0 {
		t.Fatalf("Create() = %v, calls %#v; Git ran without durable creating intent", err, runner.calls)
	}
}

func TestManagerRetriesPromotionWithoutSecondAdd(t *testing.T) {
	repo := testkit.GitRepo(t)
	runner := &gitRunner{}
	manager := NewManager(repo, "project", store.New(repo, core.StorageLimits{CanonicalBytes: 16 << 20}), runner)
	original, writes := manager.write, 0
	manager.write = func(path string, identity WorktreeIdentity) error {
		writes++
		if writes == 1 {
			return fmt.Errorf("promotion failure")
		}
		return original(path, identity)
	}
	spec := worktreeSpec("run", "team", filepath.Join(repo, ".agent-team", "worktrees", "task"))
	if _, err := manager.Create(context.Background(), spec); err == nil {
		t.Fatal("promotion failure accepted")
	}
	manager.write = original
	if _, err := manager.Create(context.Background(), spec); err != nil {
		t.Fatal(err)
	}
	adds := 0
	for _, call := range mutations(runner.calls) {
		if call[3] == "worktree" && call[4] == "add" {
			adds++
		}
	}
	if adds != 1 {
		t.Fatalf("adds = %d, calls %#v", adds, runner.calls)
	}
}

func TestManagerRejectsExternalDurableIdentityBeforeGit(t *testing.T) {
	repo := testkit.GitRepo(t)
	state := store.New(repo, core.StorageLimits{CanonicalBytes: 16 << 20})
	foreign := filepath.Join(t.TempDir(), "foreign")
	_, err := state.WriteJSON(worktreeIdentityPath("run", "team"), WorktreeIdentity{RecordEnvelope: core.RecordEnvelope{Schema: 1, Project: "project", RunID: "run"}, Team: "team", Path: foreign, Base: "base", Branch: "agent-team/run/team", Lifecycle: active}, 64<<10)
	if err != nil {
		t.Fatal(err)
	}
	runner := &gitRunner{}
	manager := NewManager(repo, "project", state, runner)
	_, err = manager.Inspect(context.Background(), contracts.Worktree{Run: "run", Team: "team", Path: foreign, Base: "base", Branch: "agent-team/run/team"})
	if !errors.Is(err, core.ErrPath) || len(runner.calls) != 0 {
		t.Fatalf("Inspect = %v, calls %#v", err, runner.calls)
	}
}

func TestManagerCleanupRetriesOnlyUnfinishedStage(t *testing.T) {
	repo := testkit.GitRepo(t)
	runner := &gitRunner{fail: map[string]int{"branch -d agent-team/run/team": 1}}
	manager := NewManager(repo, "project", store.New(repo, core.StorageLimits{CanonicalBytes: 16 << 20}), runner)
	w, err := manager.Create(context.Background(), worktreeSpec("run", "team", filepath.Join(repo, ".agent-team", "worktrees", "task")))
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.RemoveExact(context.Background(), w); err == nil {
		t.Fatal("branch failure accepted")
	}
	if err := manager.RemoveExact(context.Background(), w); err != nil {
		t.Fatal(err)
	}
	removes, deletes := 0, 0
	for _, call := range mutations(runner.calls) {
		if call[3] == "worktree" && call[4] == "remove" {
			removes++
		}
		if call[3] == "branch" {
			deletes++
		}
	}
	if removes != 1 || deletes != 2 {
		t.Fatalf("remove/delete = %d/%d, calls %#v", removes, deletes, runner.calls)
	}
}

func TestManagerCleanupRetriesTombstoneWithoutRepeatDelete(t *testing.T) {
	repo := testkit.GitRepo(t)
	runner := &gitRunner{}
	manager := NewManager(repo, "project", store.New(repo, core.StorageLimits{CanonicalBytes: 16 << 20}), runner)
	w, err := manager.Create(context.Background(), worktreeSpec("run", "team", filepath.Join(repo, ".agent-team", "worktrees", "task")))
	if err != nil {
		t.Fatal(err)
	}
	original := manager.write
	manager.write = func(path string, identity WorktreeIdentity) error {
		if identity.Lifecycle == removed {
			return fmt.Errorf("tombstone failure")
		}
		return original(path, identity)
	}
	if err := manager.RemoveExact(context.Background(), w); err == nil {
		t.Fatal("tombstone failure accepted")
	}
	manager.write = original
	if err := manager.RemoveExact(context.Background(), w); err != nil {
		t.Fatal(err)
	}
	deletes := 0
	for _, call := range mutations(runner.calls) {
		if call[3] == "branch" {
			deletes++
		}
	}
	if deletes != 1 {
		t.Fatalf("branch deletion repeated: %#v", runner.calls)
	}
}

func TestManagerSerializesConcurrentLifecycleCalls(t *testing.T) {
	repo := testkit.GitRepo(t)
	manager := NewManager(repo, "project", store.New(repo, core.StorageLimits{CanonicalBytes: 16 << 20}), &gitRunner{})
	spec := worktreeSpec("run", "team", filepath.Join(repo, ".agent-team", "worktrees", "task"))
	w, err := manager.Create(context.Background(), spec)
	if err != nil {
		t.Fatal(err)
	}
	var group sync.WaitGroup
	for i := 0; i < 8; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			_, _ = manager.Create(context.Background(), spec)
			_, _ = manager.Inspect(context.Background(), w)
			_ = manager.Cleanup(context.Background(), "other")
		}()
	}
	group.Wait()
	if _, err := manager.Inspect(context.Background(), w); err != nil {
		t.Fatal(err)
	}
}

type gitRunner struct {
	calls     [][]string
	result    tracker.CommandResult
	branches  map[string]bool
	worktrees map[string]string
	heads     map[string]string
	fail      map[string]int
}

func (r *gitRunner) Run(_ context.Context, name string, args ...string) tracker.CommandResult {
	r.calls = append(r.calls, append([]string{name}, args...))
	if len(args) < 3 {
		return r.result
	}
	command := args[2:]
	if r.branches == nil {
		r.branches = map[string]bool{}
		r.worktrees = map[string]string{}
		r.heads = map[string]string{}
	}
	if r.fail[strings.Join(command, " ")] > 0 {
		r.fail[strings.Join(command, " ")]--
		return tracker.CommandResult{Exit: 1}
	}
	switch {
	case len(command) == 3 && reflect.DeepEqual(command, []string{"worktree", "list", "--porcelain"}):
		var out string
		for path, branch := range r.worktrees {
			out += "worktree " + path + "\nbranch refs/heads/" + branch + "\n\n"
		}
		return tracker.CommandResult{Stdout: []byte(out)}
	case len(command) == 4 && command[0] == "show-ref":
		return tracker.CommandResult{Exit: map[bool]int{true: 0, false: 1}[r.branches[strings.TrimPrefix(command[3], "refs/heads/")]]}
	case len(command) == 2 && command[0] == "rev-parse":
		return tracker.CommandResult{Stdout: []byte(r.heads[args[1]])}
	case len(command) == 6 && command[0] == "worktree" && command[1] == "add":
		r.branches[command[3]] = true
		r.worktrees[command[4]] = command[3]
	case len(command) == 3 && command[0] == "worktree" && command[1] == "remove":
		delete(r.worktrees, command[2])
	case len(command) == 3 && command[0] == "branch" && command[1] == "-d":
		delete(r.branches, command[2])
	}
	return r.result
}

func mutations(calls [][]string) [][]string {
	var got [][]string
	for _, call := range calls {
		if len(call) >= 5 && ((call[3] == "worktree" && (call[4] == "add" || call[4] == "remove")) || call[3] == "branch" || call[3] == "merge") {
			got = append(got, call)
		}
	}
	return got
}

func worktreeSpec(run core.RunID, team core.TeamID, root string) contracts.WorktreeSpec {
	return contracts.WorktreeSpec{Run: run, Team: team, Root: root, Base: "base", WritablePaths: []string{"src"}}
}
