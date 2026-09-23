package tracker

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/thebpandey/agent-team/vnext/internal/core"
)

func TestBeadsFetchesOnlyReferencedCompletedHistory(t *testing.T) {
	active := `[{"id":"B-active","title":"active","status":"open","dependencies":["B-closed"]}]`
	closed := `[{"id":"B-closed","title":"closed prerequisite","status":"closed","closed_at":"2026-09-22T19:53:30Z","close_reason":"done","revision":"1125083339202601185"}]`
	r := &scriptedRunner{results: []CommandResult{{Stdout: []byte(active)}, {Stdout: []byte(closed)}, {Stdout: []byte(active)}, {Stdout: []byte(closed)}}}
	tr := NewBeads(r)
	page, err := tr.Page(context.Background(), "", 1000)
	if err != nil || len(page.Tasks) != 2 {
		t.Fatalf("page = %#v, %v", page, err)
	}
	if got, err := tr.Get(context.Background(), "B-closed", page.TrackerRevision); err != nil || got.State != core.Integrated {
		t.Fatalf("Get prerequisite = %#v, %v", got, err)
	}
	calls := r.Calls()
	if !reflect.DeepEqual(calls[0], []string{"bd", "--readonly", "list", "--json", "--limit", "1001"}) || !reflect.DeepEqual(calls[1], []string{"bd", "--readonly", "show", "--json", "--", "B-closed"}) {
		t.Fatalf("unbounded or mutating snapshot: %#v", calls)
	}
}

func TestBeadsCombinedRevisionIgnoresResponseOrderAndBindsPrerequisites(t *testing.T) {
	for _, reversed := range []bool{false, true} {
		active := `[{"id":"B-a","title":"a","status":"open","dependencies":["B-c"]},{"id":"B-b","title":"b","status":"open","dependencies":["B-d"]}]`
		closed := `[{"id":"B-c","title":"c","status":"closed"},{"id":"B-d","title":"d","status":"closed"}]`
		if reversed {
			active = `[{"id":"B-b","title":"b","status":"open","dependencies":["B-d"]},{"id":"B-a","title":"a","status":"open","dependencies":["B-c"]}]`
			closed = `[{"id":"B-d","title":"d","status":"closed"},{"id":"B-c","title":"c","status":"closed"}]`
		}
		r := &scriptedRunner{results: []CommandResult{{Stdout: []byte(active)}, {Stdout: []byte(closed)}}}
		page, err := NewBeads(r).Page(context.Background(), "", 1000)
		if err != nil {
			t.Fatal(err)
		}
		want := beadsFixtureRevision(t, `[{"id":"B-a","title":"a","status":"open","dependencies":["B-c"]},{"id":"B-b","title":"b","status":"open","dependencies":["B-d"]},{"id":"B-c","title":"c","status":"closed"},{"id":"B-d","title":"d","status":"closed"}]`)
		if page.TrackerRevision != want {
			t.Fatalf("order changed revision: got %d want %d", page.TrackerRevision, want)
		}
	}
}

func TestBeadsArchiveFreesUnreferencedClosedIssue(t *testing.T) {
	active := `[{"id":"B-1","title":"task","status":"open"}]`
	r := &scriptedRunner{results: []CommandResult{{Stdout: []byte(active)}, {Stdout: []byte(active)}, {}, {Stdout: []byte(`[]`)}}}
	tr := NewBeads(r)
	before, err := tr.Page(context.Background(), "", 1000)
	if err != nil {
		t.Fatal(err)
	}
	if err := tr.Archive(context.Background(), "B-1", "done", before.TrackerRevision); err != nil {
		t.Fatal(err)
	}
	after, err := tr.Page(context.Background(), "", 1000)
	if err != nil || after.TotalNonArchived != 0 || after.TrackerRevision == before.TrackerRevision {
		t.Fatalf("history retained capacity: %#v, %v", after, err)
	}
}

func TestBeadsArchiveRetryReadsHistoryWithoutMutation(t *testing.T) {
	r := &scriptedRunner{results: []CommandResult{{Stdout: []byte(`[]`)}, {Stdout: []byte(`[{"id":"B-old","title":"done","status":"closed","revision":"123"}]`)}}}
	if err := NewBeads(r).Archive(context.Background(), "B-old", "done", beadsFixtureRevision(t, `[]`)); err != nil {
		t.Fatal(err)
	}
	if calls := r.Calls(); len(calls) != 2 || !containsArgs(calls[1], "--readonly", "show") {
		t.Fatalf("archive rewrote history: %#v", calls)
	}
}

func TestBeadsPrerequisiteClosureAndCAS(t *testing.T) {
	active := `[{"id":"B-a","title":"active","status":"open","dependencies":["B-c"]}]`
	closed := `[{"id":"B-c","title":"closed","status":"closed","dependencies":["B-d"]}]`
	prior := `[{"id":"B-d","title":"earlier prerequisite","status":"closed"}]`
	changed := `[{"id":"B-d","title":"changed prerequisite","status":"closed"}]`
	r := &scriptedRunner{results: []CommandResult{{Stdout: []byte(active)}, {Stdout: []byte(closed)}, {Stdout: []byte(prior)}, {Stdout: []byte(active)}, {Stdout: []byte(closed)}, {Stdout: []byte(changed)}}}
	tr := NewBeads(r)
	page, err := tr.Page(context.Background(), "", 1000)
	if err != nil || len(page.Tasks) != 3 {
		t.Fatalf("closure = %#v, %v", page, err)
	}
	if _, err := tr.Get(context.Background(), "B-a", page.TrackerRevision); !errors.Is(err, core.ErrRevision) {
		t.Fatalf("changed prerequisite didn't invalidate snapshot: %v", err)
	}
}

func TestBeadsPrerequisiteRequestsAreBatchedAndCapacityBounded(t *testing.T) {
	var ids, closed []string
	for i := 0; i < 65; i++ {
		ids = append(ids, fmt.Sprintf(`"B-%03d"`, i))
		closed = append(closed, fmt.Sprintf(`{"id":"B-%03d","title":"closed","status":"closed"}`, i))
	}
	active := `[{"id":"B-active","title":"active","status":"open","dependencies":[` + strings.Join(ids, ",") + `]}]`
	r := &scriptedRunner{results: []CommandResult{{Stdout: []byte(active)}, {Stdout: []byte("[" + strings.Join(closed[:64], ",") + "]")}, {Stdout: []byte("[" + closed[64] + "]")}}}
	page, err := NewBeads(r).Page(context.Background(), "", 1000)
	if err != nil || len(page.Tasks) != 66 {
		t.Fatalf("bounded batches = %#v, %v", page, err)
	}
	if calls := r.Calls(); len(calls) != 3 || len(calls[1]) != 69 || len(calls[2]) != 6 {
		t.Fatalf("unbounded show arguments: %#v", calls)
	}
	// A single active issue cannot make the adapter fetch an unbounded history.
	for i := 65; i < 1000; i++ {
		ids = append(ids, fmt.Sprintf(`"B-%03d"`, i))
	}
	active = `[{"id":"B-active","title":"active","status":"open","dependencies":[` + strings.Join(ids, ",") + `]}]`
	r = &scriptedRunner{results: []CommandResult{{Stdout: []byte(active)}}}
	if _, err := NewBeads(r).Page(context.Background(), "", 1000); !errors.Is(err, core.ErrCapacity) || len(r.Calls()) != 1 {
		t.Fatalf("unbounded prerequisite fetch: %v", err)
	}
}

func TestBeadsRejectsMissingOrActiveSupplementalPrerequisite(t *testing.T) {
	for _, body := range []string{`[]`, `[{"id":"B-other","title":"wrong","status":"closed"}]`, `[{"id":"B-closed","title":"not closed","status":"open"}]`} {
		r := &scriptedRunner{results: []CommandResult{{Stdout: []byte(`[{"id":"B-active","title":"active","status":"open","dependencies":["B-closed"]}]`)}, {Stdout: []byte(body)}}}
		if _, err := NewBeads(r).Page(context.Background(), "", 1000); !errors.Is(err, core.ErrRevision) {
			t.Fatalf("incomplete view accepted: %v", err)
		}
	}
}

func beadsFixtureRevision(t *testing.T, raw string) uint64 {
	t.Helper()
	tasks, err := parseBeads([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	return beadsRevision(tasks)
}
