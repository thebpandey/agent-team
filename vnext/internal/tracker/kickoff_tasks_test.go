package tracker

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/thebpandey/agent-team/vnext/internal/core"
	"github.com/thebpandey/agent-team/vnext/internal/store"
)

const kickoffTasks = `# Demo — Agent-Team Tasks

Updated: 2026-09-22
Writer: project owner

## Plan-to-tracker mapping
| Plan ID | Tracker ID | Parent | Requirement IDs | Baseline acceptance summary |
| --- | --- | --- | --- | --- |
| TASK-001 | AT-001 | STORY-001 | REQ-001 | preserves data |

## Active tasks
| ID | Plan ID / requirements | Intended outcome / acceptance pointer | Owner | Depends on | Status | Revision / evidence | Next action | Custom |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| AT-001 | TASK-001 / REQ-001 | Preserve existing data | None | None | verified | proof.md | Done | KEEP EXACT |
| AT-002 | TASK-002 / REQ-002 | Add feature | None | AT-001 | ready | None yet | Implement | custom |

## Failures
### F-001 — retained incident
Unknown user prose and evidence.
`

func TestKickoffTableReadsOnlyActiveTasks(t *testing.T) {
	tasks, err := parseTasksMD([]byte(kickoffTasks))
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 2 || tasks[0].ID != "AT-001" || tasks[0].State != core.Integrated || tasks[1].Objective != "Add feature" || !reflect.DeepEqual(tasks[1].Dependencies, []core.TaskID{"AT-001"}) {
		t.Fatalf("parsed tasks: %#v", tasks)
	}
}

func TestKickoffTableRejectsAmbiguousOrInvalidRows(t *testing.T) {
	for _, row := range []string{
		"| AT-1 | first | ready | None |\n| AT-1 | duplicate | ready | None |",
		"| AT-1 | first | invented-state | None |",
		"| AT-1 | first | ready | AT-1 |",
	} {
		body := "## Active tasks\n| ID | Intended outcome / acceptance pointer | Status | Depends on |\n| --- | --- | --- | --- |\n" + row + "\n"
		if _, err := parseTasksMD([]byte(body)); err == nil {
			t.Fatalf("accepted invalid rows %s", row)
		}
	}
}

func TestKickoffTableMutationNeverRewritesUnknownContent(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "TASKS.md")
	if err := os.WriteFile(path, []byte(kickoffTasks), 0600); err != nil {
		t.Fatal(err)
	}
	tr := NewTasksMD(path, store.New(root, core.DefaultConfig().Storage))
	page, err := tr.Page(context.Background(), "", 100)
	if err != nil {
		t.Fatal(err)
	}
	created, err := tr.Create(context.Background(), core.Task{ID: "AT-3", Objective: "new", State: core.Ready, Dependencies: []core.TaskID{"AT-002"}}, page.TrackerRevision)
	if err != nil {
		t.Fatal(err)
	}
	if err := tr.Archive(context.Background(), "AT-001", "done", created.Revision); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil || !strings.Contains(string(got), "| TASK-001 | AT-001 | STORY-001 | REQ-001 | preserves data |") || !strings.Contains(string(got), "| AT-001 | TASK-001 / REQ-001 | Preserve existing data | None | None | archived | proof.md | Done | KEEP EXACT |") || !strings.Contains(string(got), "## Failures\n### F-001 — retained incident\nUnknown user prose and evidence.") {
		t.Fatalf("mutation altered unknown table data: %s", got)
	}
	page, err = tr.Page(context.Background(), "", 100)
	if err != nil || len(page.Tasks) != 2 || page.Tasks[1].ID != "AT-3" || !reflect.DeepEqual(page.Tasks[1].Dependencies, []core.TaskID{"AT-002"}) {
		t.Fatalf("mutation not readable: %#v %v", page, err)
	}
	before := string(got)
	if _, err := tr.Create(context.Background(), core.Task{ID: "AT-4", Objective: "invalid\nrow"}, page.TrackerRevision); err == nil {
		t.Fatal("accepted an invalid table row")
	}
	got, _ = os.ReadFile(path)
	if string(got) != before {
		t.Fatal("failed mutation changed tracker")
	}
}

func TestKickoffTableCreatesFullTaskWithoutLosingExistingContent(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "TASKS.md")
	if err := os.WriteFile(path, []byte(kickoffTasks), 0600); err != nil {
		t.Fatal(err)
	}
	tr := NewTasksMD(path, store.New(root, core.DefaultConfig().Storage))
	page, err := tr.Page(context.Background(), "", 100)
	if err != nil {
		t.Fatal(err)
	}
	want := core.Task{
		ID: "AT-003", Objective: "Deliver full approved task", State: core.Ready, Dependencies: []core.TaskID{"AT-001"},
		Criteria:      []string{"Accept | reject correctly", "Preserve \"quotes\"\nand lines"},
		Checks:        []core.Check{{Name: "shell | check", Command: []string{"sh", "-c", `printf '%s' 'path\name | quoted'`, "literal\nargument"}}},
		WritablePaths: []string{"src/**", "tests/**"}, Resources: []string{"db:fixture"}, EvidencePointers: []string{"docs/checks.md", "evidence|raw"},
	}
	created, err := tr.Create(context.Background(), want, page.TrackerRevision)
	if err != nil {
		t.Fatal(err)
	}
	got, err := tr.Get(context.Background(), want.ID, created.Revision)
	if err != nil {
		t.Fatal(err)
	}
	got.RecordEnvelope = core.RecordEnvelope{}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("task details lost:\ngot %#v\nwant %#v", got, want)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	beforeLines := strings.Split(kickoffTasks, "\n")
	afterLines := strings.Split(string(data), "\n")
	for _, line := range beforeLines {
		found := false
		for _, after := range afterLines {
			if after == line || (strings.HasPrefix(line, "|") && strings.HasPrefix(after, line)) {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("old table/custom/prose bytes changed: %q", line)
		}
	}
	page, err = tr.Page(context.Background(), "", 100)
	if err != nil || len(page.Tasks) != 3 || page.Tasks[0].Objective != "Preserve existing data" || len(page.Tasks[0].Checks) != 0 || !reflect.DeepEqual(page.Tasks[0].EvidencePointers, []string{"proof.md"}) {
		t.Fatalf("extension changed old task facts: %#v %v", page, err)
	}
	header := strings.Split(strings.Split(string(data), "## Active tasks\n")[1], "\n")[0]
	want.ID = "AT-004"
	if _, err := tr.Create(context.Background(), want, page.TrackerRevision); err != nil {
		t.Fatal(err)
	}
	data, err = os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if updated := strings.Split(strings.Split(string(data), "## Active tasks\n")[1], "\n")[0]; updated != header {
		t.Fatal("repeated Create duplicated optional columns")
	}
}

func TestKickoffTableRejectsMalformedStructuredCells(t *testing.T) {
	for _, cell := range []string{`null`, `{"name":"not-an-array"}`, `[{"name":"test","command":"not-argv"}]`, `[{"name":"test","command":["go"],"unknown":true}]`, `[] []`} {
		body := "## Active tasks\n| ID | Intended outcome / acceptance pointer | Status | Depends on | Checks |\n| --- | --- | --- | --- | --- |\n| AT-1 | task | ready | None | " + cell + " |\n"
		if _, err := parseTasksMD([]byte(body)); err == nil {
			t.Fatalf("accepted malformed check cell %s", cell)
		}
	}
}

func TestKickoffTableDraftDoesNotAddUnusedColumns(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "TASKS.md")
	if err := os.WriteFile(path, []byte(kickoffTasks), 0600); err != nil {
		t.Fatal(err)
	}
	tr := NewTasksMD(path, store.New(root, core.DefaultConfig().Storage))
	page, err := tr.Page(context.Background(), "", 100)
	if err != nil {
		t.Fatal(err)
	}
	draft, err := tr.Create(context.Background(), core.Task{ID: "AT-DRAFT", Objective: "Refine requirements", State: core.Blocked}, page.TrackerRevision)
	if err != nil {
		t.Fatal(err)
	}
	got, err := tr.Get(context.Background(), draft.ID, draft.Revision)
	if err != nil || got.State != core.Blocked {
		t.Fatalf("draft lost blocked state: %#v %v", got, err)
	}
	data, err := os.ReadFile(path)
	if err != nil || strings.Contains(string(data), "| Checks |") {
		t.Fatal("draft added unused columns")
	}
}

func TestKickoffTableOversizedDetailsLeaveFileUntouched(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "TASKS.md")
	if err := os.WriteFile(path, []byte(kickoffTasks), 0600); err != nil {
		t.Fatal(err)
	}
	tr := NewTasksMD(path, store.New(root, core.DefaultConfig().Storage))
	page, err := tr.Page(context.Background(), "", 100)
	if err != nil {
		t.Fatal(err)
	}
	_, err = tr.Create(context.Background(), core.Task{ID: "AT-BIG", Objective: "too large", Criteria: []string{strings.Repeat("x", 65<<10)}}, page.TrackerRevision)
	if !errors.Is(err, core.ErrLimit) {
		t.Fatalf("oversized details not bounded: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != kickoffTasks {
		t.Fatal("failed extension changed existing tracker")
	}
}
