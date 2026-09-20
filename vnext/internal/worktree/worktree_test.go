package worktree

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
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
	want := []string{"git", "-C", repo, "worktree", "add", "-b", "agent-team/run-1/team-1", spec.Root, strings.Repeat("a", 40)}
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
	if len(mutations(runner.calls)) != 0 {
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
	if len(mutations(runner.calls)) != 0 {
		t.Fatalf("state failure calls = %#v", runner.calls)
	}
}

func TestManagerPersistsCreatingIntentBeforeGit(t *testing.T) {
	repo := testkit.GitRepo(t)
	runner := &gitRunner{}
	manager := NewManager(repo, "project", store.New(repo, core.StorageLimits{CanonicalBytes: 16 << 20}), runner)
	manager.create = func(string, WorktreeIdentity) error { return fmt.Errorf("injected write failure") }
	_, err := manager.Create(context.Background(), worktreeSpec("run", "team", filepath.Join(repo, ".agent-team", "worktrees", "task")))
	if err == nil || len(mutations(runner.calls)) != 0 {
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

func TestFreshTeamCleanupIsUnknownEvenWithDurableIdentity(t *testing.T) {
	repo := testkit.GitRepo(t)
	state := store.New(repo, core.StorageLimits{CanonicalBytes: 16 << 20})
	runner := &gitRunner{}
	manager := NewManager(repo, "project", state, runner)
	if _, err := manager.Create(context.Background(), worktreeSpec("run", "team", filepath.Join(repo, ".agent-team", "worktrees", "task"))); err != nil {
		t.Fatal(err)
	}
	fresh := NewManager(repo, "project", state, runner)
	if err := fresh.Cleanup(context.Background(), "team"); !errors.Is(err, core.ErrPath) {
		t.Fatalf("fresh Cleanup error = %v", err)
	}
}

func TestIdentityRevisionAndTimestampAreStrict(t *testing.T) {
	for _, identity := range []WorktreeIdentity{
		{RecordEnvelope: core.RecordEnvelope{Schema: 1, Project: "project", RunID: "run", Revision: 0, WrittenAt: timestamp()}, Team: "team", Path: "/tmp/path", Base: "base", Branch: "agent-team/run/team", Lifecycle: active},
		{RecordEnvelope: core.RecordEnvelope{Schema: 1, Project: "project", RunID: "run", Revision: ^uint64(0), WrittenAt: timestamp()}, Team: "team", Path: "/tmp/path", Base: "base", Branch: "agent-team/run/team", Lifecycle: active},
	} {
		if err := advance(&identity, active, false); !errors.Is(err, core.ErrRevision) {
			t.Fatalf("advance(%d) = %v", identity.Revision, err)
		}
	}
	identity := WorktreeIdentity{RecordEnvelope: core.RecordEnvelope{Revision: 1, WrittenAt: "2020-01-01T00:00:00Z"}}
	if err := advance(&identity, active, false); err != nil || identity.Revision != 2 || !validTimestamp(identity.WrittenAt) {
		t.Fatalf("advance = %#v, %v", identity, err)
	}
}

func TestCanonicalGitPathUsesNativeRules(t *testing.T) {
	root := t.TempDir()
	got, err := canonicalGitPath(filepath.ToSlash(root))
	if err != nil || got != root {
		t.Fatalf("canonicalGitPath = %q, %v", got, err)
	}
	if runtime.GOOS == "windows" {
		if _, err := canonicalGitPath(strings.ReplaceAll(root, `\\`, `/`)); err != nil {
			t.Fatal(err)
		}
	}
}

func TestExactProbeRejectsReusedPathBeforeMutation(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*gitRunner, contracts.Worktree, string)
	}{
		{"different branch", func(r *gitRunner, w contracts.Worktree, _ string) { r.worktrees[w.Path] = "agent-team/other" }},
		{"different repo", func(r *gitRunner, w contracts.Worktree, other string) {
			r.common[w.Path] = filepath.Join(other, ".git")
		}},
		{"detached", func(r *gitRunner, w contracts.Worktree, _ string) {
			r.symbolic[w.Path] = ""
			r.symbolicSet[w.Path] = true
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			repo, other := testkit.GitRepo(t), testkit.GitRepo(t)
			runner := &gitRunner{}
			manager := NewManager(repo, "project", store.New(repo, core.StorageLimits{CanonicalBytes: 16 << 20}), runner)
			w, err := manager.Create(context.Background(), worktreeSpec("run", "team", filepath.Join(repo, ".agent-team", "worktrees", "task")))
			if err != nil {
				t.Fatal(err)
			}
			test.mutate(runner, w, other)
			before := len(mutations(runner.calls))
			if _, err := manager.Integrate(context.Background(), contracts.Candidate{Task: "task", Revision: "candidate", Base: w.Base, Worktree: w}); !errors.Is(err, core.ErrGit) {
				t.Fatalf("Integrate error = %v", err)
			}
			if err := manager.RemoveExact(context.Background(), w); !errors.Is(err, core.ErrGit) {
				t.Fatalf("RemoveExact error = %v", err)
			}
			if len(mutations(runner.calls)) != before {
				t.Fatalf("foreign worktree mutated: %#v", runner.calls)
			}
		})
	}
}

func TestCreateFreezesMutableBaseToOID(t *testing.T) {
	repo := testkit.GitRepo(t)
	runner := &gitRunner{base: strings.Repeat("A", 40)}
	manager := NewManager(repo, "project", store.New(repo, core.StorageLimits{CanonicalBytes: 16 << 20}), runner)
	spec := worktreeSpec("run", "team", filepath.Join(repo, ".agent-team", "worktrees", "task"))
	w, err := manager.Create(context.Background(), spec)
	if err != nil {
		t.Fatal(err)
	}
	want := strings.Repeat("a", 40)
	if w.Base != want || mutations(runner.calls)[0][len(mutations(runner.calls)[0])-1] != want {
		t.Fatalf("worktree/add base = %q, %#v", w.Base, mutations(runner.calls))
	}
	if retry, err := manager.Create(context.Background(), spec); err != nil || retry.Base != want {
		t.Fatalf("retry = %#v, %v", retry, err)
	}
	runner.base = strings.Repeat("b", 40)
	if _, err := manager.Create(context.Background(), spec); !errors.Is(err, core.ErrPath) {
		t.Fatalf("advanced ref retry error = %v", err)
	}
	if _, err := manager.Integrate(context.Background(), contracts.Candidate{Task: "task", Revision: "candidate", Base: "base", Worktree: w}); !errors.Is(err, core.ErrPath) {
		t.Fatalf("advanced ref integration error = %v", err)
	}
}

func TestCreateRejectsMalformedBaseResolutionBeforeMutation(t *testing.T) {
	for _, output := range []string{"short\n", strings.Repeat("a", 40) + "\n" + strings.Repeat("b", 40) + "\n"} {
		t.Run(strings.ReplaceAll(output, "\n", "_"), func(t *testing.T) {
			repo := testkit.GitRepo(t)
			runner := &gitRunner{baseOutput: output}
			manager := NewManager(repo, "project", store.New(repo, core.StorageLimits{CanonicalBytes: 16 << 20}), runner)
			_, err := manager.Create(context.Background(), worktreeSpec("run", "team", filepath.Join(repo, ".agent-team", "worktrees", "task")))
			if !errors.Is(err, core.ErrRevision) || len(mutations(runner.calls)) != 0 {
				t.Fatalf("Create = %v, calls %#v", err, runner.calls)
			}
		})
	}
}

func TestParseResolvedOIDIsByteStrict(t *testing.T) {
	oid := strings.Repeat("a", 40)
	for _, output := range []string{oid, oid + "\n", oid + "\r\n"} {
		if got, err := parseResolvedOID(output); err != nil || got != oid {
			t.Fatalf("parse(%q) = %q, %v", output, got, err)
		}
	}
	for _, output := range []string{oid + "\n\n", oid + "\r\n\r\n", " " + oid, oid + " ", "\t" + oid, oid + "\t"} {
		if _, err := parseResolvedOID(output); !errors.Is(err, core.ErrRevision) {
			t.Fatalf("parse(%q) = %v", output, err)
		}
	}
}

func TestParseRawLineRejectsSurroundingWhitespaceBeforePathCanonicalization(t *testing.T) {
	for _, value := range []string{" path", "path ", "\tpath", "path\t"} {
		if _, err := parseRawLine(value); !errors.Is(err, core.ErrPath) {
			t.Fatalf("parseRawLine(%q) = %v", value, err)
		}
	}
}

func TestExactProbeRejectsNonStrictAuthoritativeLines(t *testing.T) {
	for _, field := range []string{"top", "common", "symbolic"} {
		for _, suffix := range []string{"\n\n", " ", "\t"} {
			t.Run(field+strings.ReplaceAll(suffix, "\n", "_"), func(t *testing.T) {
				repo := testkit.GitRepo(t)
				runner := &gitRunner{}
				manager := NewManager(repo, "project", store.New(repo, core.StorageLimits{CanonicalBytes: 16 << 20}), runner)
				w, err := manager.Create(context.Background(), worktreeSpec("run", "team", filepath.Join(repo, ".agent-team", "worktrees", "task")))
				if err != nil {
					t.Fatal(err)
				}
				switch field {
				case "top":
					runner.topOutput[w.Path] = w.Path + suffix
				case "common":
					runner.commonOutput[w.Path] = filepath.Join(repo, ".git") + suffix
				case "symbolic":
					runner.symbolicOutput[w.Path] = "refs/heads/" + w.Branch + suffix
				}
				before := len(mutations(runner.calls))
				if err := manager.RemoveExact(context.Background(), w); !errors.Is(err, core.ErrGit) {
					t.Fatalf("RemoveExact error = %v", err)
				}
				if len(mutations(runner.calls)) != before {
					t.Fatalf("non-strict %s mutated: %#v", field, runner.calls)
				}
			})
		}
	}
}

func TestExactProbeAcceptsSingleTerminalLineEnding(t *testing.T) {
	for _, suffix := range []string{"\n", "\r\n"} {
		t.Run(strings.ReplaceAll(suffix, "\n", "_"), func(t *testing.T) {
			repo := testkit.GitRepo(t)
			runner := &gitRunner{}
			manager := NewManager(repo, "project", store.New(repo, core.StorageLimits{CanonicalBytes: 16 << 20}), runner)
			w, err := manager.Create(context.Background(), worktreeSpec("run", "team", filepath.Join(repo, ".agent-team", "worktrees", "task")))
			if err != nil {
				t.Fatal(err)
			}
			runner.topOutput[w.Path] = w.Path + suffix
			runner.commonOutput[w.Path] = filepath.Join(repo, ".git") + suffix
			runner.symbolicOutput[w.Path] = "refs/heads/" + w.Branch + suffix
			if err := manager.RemoveExact(context.Background(), w); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestExactProbeAllowsPathsWithSpaces(t *testing.T) {
	repo := testkit.GitRepo(t)
	spaced := filepath.Join(filepath.Dir(repo), "repo with space")
	if err := os.Rename(repo, spaced); err != nil {
		t.Fatal(err)
	}
	runner := &gitRunner{}
	manager := NewManager(spaced, "project", store.New(spaced, core.StorageLimits{CanonicalBytes: 16 << 20}), runner)
	w, err := manager.Create(context.Background(), worktreeSpec("run", "team", filepath.Join(spaced, ".agent-team", "worktrees", "task with space")))
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.RemoveExact(context.Background(), w); err != nil {
		t.Fatal(err)
	}
}

type gitRunner struct {
	calls          [][]string
	result         tracker.CommandResult
	branches       map[string]bool
	worktrees      map[string]string
	heads          map[string]string
	fail           map[string]int
	common         map[string]string
	symbolic       map[string]string
	symbolicSet    map[string]bool
	base           string
	baseOutput     string
	topOutput      map[string]string
	commonOutput   map[string]string
	symbolicOutput map[string]string
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
		r.common = map[string]string{}
		r.symbolic = map[string]string{}
		r.symbolicSet = map[string]bool{}
		r.topOutput = map[string]string{}
		r.commonOutput = map[string]string{}
		r.symbolicOutput = map[string]string{}
	}
	if r.fail[strings.Join(command, " ")] > 0 {
		r.fail[strings.Join(command, " ")]--
		return tracker.CommandResult{Exit: 1}
	}
	switch {
	case len(command) == 4 && command[0] == "show-ref":
		return tracker.CommandResult{Exit: map[bool]int{true: 0, false: 1}[r.branches[strings.TrimPrefix(command[3], "refs/heads/")]]}
	case len(command) == 2 && command[0] == "rev-parse" && command[1] == "--show-toplevel":
		if output, ok := r.topOutput[args[1]]; ok {
			return tracker.CommandResult{Stdout: []byte(output)}
		}
		return tracker.CommandResult{Stdout: []byte(args[1])}
	case len(command) == 3 && command[0] == "rev-parse" && command[1] == "--verify":
		if r.baseOutput != "" {
			return tracker.CommandResult{Stdout: []byte(r.baseOutput)}
		}
		base := r.base
		if base == "" {
			base = strings.Repeat("a", 40)
		}
		return tracker.CommandResult{Stdout: []byte(base + "\n")}
	case len(command) == 3 && command[0] == "rev-parse" && command[2] == "--git-common-dir":
		common := r.common[args[1]]
		if common == "" {
			common = filepath.Join(args[1], ".git")
		}
		if output, ok := r.commonOutput[args[1]]; ok {
			return tracker.CommandResult{Stdout: []byte(output)}
		}
		return tracker.CommandResult{Stdout: []byte(common)}
	case len(command) == 3 && command[0] == "symbolic-ref":
		head := r.symbolic[args[1]]
		if output, ok := r.symbolicOutput[args[1]]; ok {
			return tracker.CommandResult{Stdout: []byte(output)}
		}
		if head == "" && !r.symbolicSet[args[1]] {
			head = "refs/heads/" + r.worktrees[args[1]]
		}
		return tracker.CommandResult{Stdout: []byte(head)}
	case len(command) == 2 && command[0] == "rev-parse":
		return tracker.CommandResult{Stdout: []byte(r.heads[args[1]])}
	case len(command) == 6 && command[0] == "worktree" && command[1] == "add":
		if err := os.MkdirAll(command[4], 0o755); err != nil {
			return tracker.CommandResult{Transport: err}
		}
		r.branches[command[3]] = true
		r.worktrees[command[4]] = command[3]
		r.common[command[4]] = filepath.Join(args[1], ".git")
	case len(command) == 3 && command[0] == "worktree" && command[1] == "remove":
		if err := os.RemoveAll(command[2]); err != nil {
			return tracker.CommandResult{Transport: err}
		}
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
