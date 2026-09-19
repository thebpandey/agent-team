package tracker

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

	"github.com/thebpandey/agent-team/vnext/internal/core"
	"github.com/thebpandey/agent-team/vnext/internal/store"
)

// Removing a field parser, revision check, cursor calculation, or capacity
// guard must make one of these independently specified tracker contracts fail.
func TestTasksMDPageCarriesCompleteTasksAndRevision(t *testing.T) {
	path := writeTracker(t, taskDocument(900))
	tr := NewTasksMD(path, store.New(t.TempDir(), core.StorageLimits{TrackerBytes: 2 << 20}))

	page, err := tr.Page(context.Background(), "", 8)
	if err != nil {
		t.Fatal(err)
	}
	if page.TrackerRevision == 0 || page.TotalNonArchived != 900 || page.Cursor != "8" || len(page.Tasks) != 8 {
		t.Fatalf("page = %#v", page)
	}
	first := page.Tasks[0]
	if first.ID != "T-0001" || len(first.Dependencies) != 1 || len(first.Criteria) != 1 || len(first.Checks) != 1 || len(first.WritablePaths) != 1 || len(first.Resources) != 1 || len(first.EvidencePointers) != 1 {
		t.Fatalf("incomplete task = %#v", first)
	}

	next, err := tr.Page(context.Background(), page.Cursor, 8)
	if err != nil || next.Tasks[0].ID != "T-0009" || next.TrackerRevision != page.TrackerRevision {
		t.Fatalf("next = %#v, %v", next, err)
	}
	got, err := tr.Get(context.Background(), first.ID, page.TrackerRevision)
	if err != nil || got.ID != first.ID {
		t.Fatalf("get = %#v, %v", got, err)
	}
	if _, err := tr.Get(context.Background(), first.ID, page.TrackerRevision+1); !errors.Is(err, core.ErrRevision) {
		t.Fatalf("stale Get error = %v", err)
	}
	if _, err := tr.Refresh(context.Background(), page.TrackerRevision+1); !errors.Is(err, core.ErrRevision) {
		t.Fatalf("stale Refresh error = %v", err)
	}
	if err := tr.Archive(context.Background(), first.ID, "done", page.TrackerRevision+1); !errors.Is(err, core.ErrRevision) {
		t.Fatalf("stale Archive error = %v", err)
	}
	if warning, ok := tr.(interface{ Warning() string }); !ok || warning.Warning() == "" {
		t.Fatal("tracker did not expose the 900-task capacity warning")
	}
}

func TestTasksMDCapacityAndMalformedInput(t *testing.T) {
	for _, tc := range []struct {
		name  string
		count int
		want  error
	}{
		{name: "below capacity", count: 999},
		{name: "at capacity", count: 1000},
		{name: "over capacity", count: 1001, want: core.ErrCapacity},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tr := NewTasksMD(writeTracker(t, taskDocument(tc.count)), store.New(t.TempDir(), core.StorageLimits{TrackerBytes: 2 << 20}))
			_, err := tr.Page(context.Background(), "", 8)
			if !errors.Is(err, tc.want) {
				t.Fatalf("Page error = %v, want %v", err, tc.want)
			}
		})
	}
	for _, tc := range []struct {
		name  string
		body  string
		limit int64
		want  error
	}{
		{name: "malformed", body: "## \nObjective: missing id\n", limit: 1024, want: core.ErrPath},
		{name: "unbounded", body: taskDocument(1), limit: 0, want: core.ErrLimit},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tr := NewTasksMD(writeTracker(t, tc.body), store.New(t.TempDir(), core.StorageLimits{TrackerBytes: tc.limit}))
			_, err := tr.Page(context.Background(), "", 8)
			if !errors.Is(err, tc.want) {
				t.Fatalf("Page error = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestBeadsFailuresAndSnapshot(t *testing.T) {
	for _, result := range []CommandResult{{TimedOut: true}, {Exit: 7}, {Transport: errors.New("missing")}, {Stdout: []byte("{")}} {
		tr := NewBeads(NewFakeRunner(result))
		if _, err := tr.Page(context.Background(), "", 8); err == nil {
			t.Fatalf("result %#v unexpectedly succeeded", result)
		}
	}

	data := []byte(`[{"id":"B-1","objective":"ship","status":"open","dependencies":["B-0"],"criteria":["works"],"checks":[{"name":"unit","command":["go","test","./..."]}],"writablePaths":["internal/tracker/**"],"resources":["browser:1"],"evidencePointers":["receipt:B-1"]}]`)
	tr := NewBeads(NewFakeRunner(CommandResult{Stdout: data}))
	page, err := tr.Page(context.Background(), "", 8)
	if err != nil || page.TrackerRevision == 0 || len(page.Tasks) != 1 || len(page.Tasks[0].Checks) != 1 {
		t.Fatalf("Page = %#v, %v", page, err)
	}
	if _, err := tr.Get(context.Background(), "B-1", page.TrackerRevision+1); !errors.Is(err, core.ErrRevision) {
		t.Fatalf("stale Beads Get error = %v", err)
	}
	if _, err := tr.Create(context.Background(), core.Task{ID: "B-2", Objective: "new"}, page.TrackerRevision+1); !errors.Is(err, core.ErrRevision) {
		t.Fatalf("stale Beads Create error = %v", err)
	}
	if err := tr.Archive(context.Background(), "B-1", "done", page.TrackerRevision+1); !errors.Is(err, core.ErrRevision) {
		t.Fatalf("stale Beads Archive error = %v", err)
	}
}

func TestExecutableResolverAcceptsBeadsPlatformNames(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"bd", "bd.exe", "bd.cmd"} {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
			t.Fatal(err)
		}
		got, err := resolveExecutable(path)
		if err != nil || got == "" {
			t.Fatalf("resolve %q = %q, %v", name, got, err)
		}
	}
}

func TestBoundedCaptureRetainsOnlyLimitWithoutShortWrite(t *testing.T) {
	capture := &boundedCapture{limit: 3}
	written, err := capture.Write([]byte("abcdef"))
	if err != nil || written != 6 || string(capture.Bytes()) != "abc" {
		t.Fatalf("Write = %d, %v; bytes = %q", written, err, capture.Bytes())
	}
}

func TestTasksMDRejectsAmbiguousHeadingsDuplicateIDsAndDuplicateFields(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
	}{
		{name: "level three heading", body: "# Tasks\n\n### T-1\nObjective: x\n"},
		{name: "duplicate ID", body: "# Tasks\n\n## T-1\nObjective: x\n\n## T-1\nObjective: y\n"},
		{name: "duplicate field", body: "# Tasks\n\n## T-1\nObjective: x\nObjective: y\n"},
		{name: "unknown state", body: "# Tasks\n\n## T-1\nObjective: x\nState: mystery\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseTasksMD([]byte(tc.body))
			if !errors.Is(err, core.ErrPath) {
				t.Fatalf("parseTasksMD error = %v, want ErrPath", err)
			}
		})
	}
}

func TestBeadsRejectsNullUnknownStatusAndDuplicateIDs(t *testing.T) {
	valid := `{"id":"B-1","title":"ship","status":"open","dependencies":[],"criteria":[],"checks":[],"writablePaths":[],"resources":[],"evidencePointers":[]}`
	for _, tc := range []struct {
		name string
		body string
	}{
		{name: "null result", body: `null`},
		{name: "null title", body: `[{"id":"B-1","title":null,"status":"open"}]`},
		{name: "unknown status", body: `[{"id":"B-1","title":"ship","status":"mystery"}]`},
		{name: "duplicate ID", body: "[" + valid + "," + valid + "]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tr := NewBeads(NewFakeRunner(CommandResult{Stdout: []byte(tc.body)}))
			_, err := tr.Page(context.Background(), "", 8)
			if !errors.Is(err, core.ErrPath) {
				t.Fatalf("Page error = %v, want ErrPath", err)
			}
		})
	}
	closed := NewBeads(NewFakeRunner(CommandResult{Stdout: []byte(`[{"id":"B-2","title":"closed","status":"closed"}]`)}))
	page, err := closed.Page(context.Background(), "", 8)
	if err != nil || len(page.Tasks) != 0 {
		t.Fatalf("closed page = %#v, %v", page, err)
	}
	task, err := closed.Get(context.Background(), "B-2", trackerRevision([]byte(`[{"id":"B-2","title":"closed","status":"closed"}]`)))
	if err != nil || task.State != core.Archived || !task.Archived {
		t.Fatalf("closed task = %#v, %v", task, err)
	}
}

func TestBeadsCreateReturnsVerifiedTaskAndReusesIdenticalID(t *testing.T) {
	initial := `[{"id":"B-1","title":"existing","status":"open"}]`
	created := `{"id":"B-2","title":"new","status":"open","dependencies":["B-1"],"metadata":{"criteria":["works"],"checks":[{"name":"unit","command":["go","test","./..."]}],"writablePaths":["internal/tracker/**"],"resources":["browser:1"],"evidencePointers":["receipt:B-2"]}}`
	final := strings.TrimSuffix(initial, "]") + `,` + created + `]`
	runner := &scriptedRunner{results: []CommandResult{{Stdout: []byte(initial)}, {Stdout: []byte(initial)}, {Stdout: []byte(created)}, {Stdout: []byte(final)}}}
	tr := NewBeads(runner)
	page, err := tr.Page(context.Background(), "", 8)
	if err != nil {
		t.Fatal(err)
	}
	want := core.Task{ID: "B-2", Objective: "new", State: core.Ready, Dependencies: []core.TaskID{"B-1"}, Criteria: []string{"works"}, Checks: []core.Check{{Name: "unit", Command: []string{"go", "test", "./..."}}}, WritablePaths: []string{"internal/tracker/**"}, Resources: []string{"browser:1"}, EvidencePointers: []string{"receipt:B-2"}}
	got, err := tr.Create(context.Background(), want, page.TrackerRevision)
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("Create = %#v, %v", got, err)
	}
	if calls := runner.Calls(); len(calls) != 4 || strings.Join(calls[2], " ") == "" || !containsArgs(calls[2], "--id", "B-2") {
		t.Fatalf("unsafe or incomplete create calls: %#v", calls)
	}

	reuseRunner := &scriptedRunner{results: []CommandResult{{Stdout: []byte(final)}}}
	reuse := NewBeads(reuseRunner)
	reused, err := reuse.Create(context.Background(), want, trackerRevision([]byte(final)))
	if err != nil || !reflect.DeepEqual(reused, want) || len(reuseRunner.Calls()) != 1 {
		t.Fatalf("idempotent Create = %#v, %v, calls=%#v", reused, err, reuseRunner.Calls())
	}
}

func TestMutationsRespectCancellationAfterLockAcquisition(t *testing.T) {
	t.Run("TASKS", func(t *testing.T) {
		path := writeTracker(t, taskDocument(1))
		concrete := NewTasksMD(path, store.New(filepath.Dir(path), core.StorageLimits{TrackerBytes: 2 << 20})).(*tasksMD)
		page, err := concrete.Page(context.Background(), "", 8)
		if err != nil {
			t.Fatal(err)
		}
		before, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		concrete.mu.Lock()
		ctx := newStagedContext()
		done := make(chan error, 1)
		go func() { done <- concrete.Archive(ctx, "T-0001", "done", page.TrackerRevision) }()
		<-ctx.first
		ctx.Cancel()
		concrete.mu.Unlock()
		if err := <-done; !errors.Is(err, context.Canceled) {
			t.Fatalf("Archive error = %v, want context.Canceled", err)
		}
		after, err := os.ReadFile(path)
		if err != nil || string(after) != string(before) {
			t.Fatalf("cancelled TASKS mutation changed file: %v", err)
		}
	})
	t.Run("Beads", func(t *testing.T) {
		runner := &scriptedRunner{}
		concrete := NewBeads(runner).(*beads)
		concrete.mu.Lock()
		ctx := newStagedContext()
		done := make(chan error, 1)
		go func() { done <- concrete.Archive(ctx, "B-1", "done", 1) }()
		<-ctx.first
		ctx.Cancel()
		concrete.mu.Unlock()
		if err := <-done; !errors.Is(err, context.Canceled) {
			t.Fatalf("Archive error = %v, want context.Canceled", err)
		}
		if calls := runner.Calls(); len(calls) != 0 {
			t.Fatalf("cancelled Beads mutation ran commands: %#v", calls)
		}
	})
}

func TestTasksJSONRejectsNullDuplicateUnknownAndUnknownState(t *testing.T) {
	valid := `{"id":"J-1","objective":"json","state":"ready","dependencies":[],"criteria":[],"checks":[],"writablePaths":[],"resources":[],"evidencePointers":[],"archived":false}`
	for _, tc := range []struct{ name, body string }{
		{name: "null", body: `null`},
		{name: "duplicate ID", body: `[` + valid + `,` + valid + `]`},
		{name: "unknown field", body: `[{"id":"J-1","objective":"json","state":"ready","surprise":true}]`},
		{name: "unknown state", body: `[{"id":"J-1","objective":"json","state":"mystery"}]`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tr := NewTasksMD(writeTracker(t, tc.body), store.New(t.TempDir(), core.StorageLimits{TrackerBytes: 2 << 20}))
			if _, err := tr.Page(context.Background(), "", 8); !errors.Is(err, core.ErrPath) {
				t.Fatalf("Page error = %v, want ErrPath", err)
			}
		})
	}
}

func TestBeadsCreateRejectsUnsupportedStateBeforeMutation(t *testing.T) {
	runner := &scriptedRunner{}
	tr := NewBeads(runner)
	if _, err := tr.Create(context.Background(), core.Task{ID: "B-9", Objective: "review", State: core.Reviewing}, 1); !errors.Is(err, core.ErrSettings) {
		t.Fatalf("Create error = %v, want ErrSettings", err)
	}
	if calls := runner.Calls(); len(calls) != 0 {
		t.Fatalf("unsupported state ran Beads command: %#v", calls)
	}
}

func TestBeadsArchiveReusesAlreadyArchivedTask(t *testing.T) {
	body := `[{"id":"B-closed","title":"done","status":"closed"}]`
	runner := &scriptedRunner{results: []CommandResult{{Stdout: []byte(body)}}}
	tr := NewBeads(runner)
	if err := tr.Archive(context.Background(), "B-closed", "done", trackerRevision([]byte(body))); err != nil {
		t.Fatal(err)
	}
	if calls := runner.Calls(); len(calls) != 1 || !containsArgs(calls[0], "list", "--json") {
		t.Fatalf("archived task should only snapshot, calls=%#v", calls)
	}
}

type scriptedRunner struct {
	mu      sync.Mutex
	results []CommandResult
	calls   [][]string
}

func (r *scriptedRunner) Run(_ context.Context, name string, args ...string) CommandResult {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, append([]string{name}, args...))
	if len(r.results) == 0 {
		return CommandResult{Transport: errors.New("unexpected command")}
	}
	result := r.results[0]
	r.results = r.results[1:]
	return result
}

func (r *scriptedRunner) Calls() [][]string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([][]string(nil), r.calls...)
}

type stagedContext struct {
	context.Context
	mu        sync.Mutex
	cancelled bool
	first     chan struct{}
	once      sync.Once
}

func newStagedContext() *stagedContext {
	return &stagedContext{Context: context.Background(), first: make(chan struct{})}
}

func (c *stagedContext) Err() error {
	c.once.Do(func() { close(c.first) })
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cancelled {
		return context.Canceled
	}
	return nil
}

func (c *stagedContext) Cancel() {
	c.mu.Lock()
	c.cancelled = true
	c.mu.Unlock()
}

func containsArgs(args []string, want ...string) bool {
	for i := 0; i+len(want) <= len(args); i++ {
		if strings.Join(args[i:i+len(want)], "\x00") == strings.Join(want, "\x00") {
			return true
		}
	}
	return false
}

func writeTracker(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "TASKS.md")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func taskDocument(count int) string {
	var b strings.Builder
	b.WriteString("# Tasks\n\n")
	for i := 1; i <= count; i++ {
		fmt.Fprintf(&b, "## T-%04d\nObjective: task %d\nState: ready\n", i, i)
		if i == 1 {
			b.WriteString("Dependencies: T-0000\nCriteria:\n- works\nChecks:\n- unit | go test ./...\nWritable paths:\n- internal/tracker/**\nResources:\n- browser:1\nEvidence pointers:\n- receipt:T-0001\n")
		}
		b.WriteByte('\n')
	}
	return b.String()
}
