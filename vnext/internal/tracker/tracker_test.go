package tracker

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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

	data := []byte(`[{"id":"B-1","objective":"ship","state":"ready","dependencies":["B-0"],"criteria":["works"],"checks":[{"name":"unit","command":["go","test","./..."]}],"writablePaths":["internal/tracker/**"],"resources":["browser:1"],"evidencePointers":["receipt:B-1"]}]`)
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
