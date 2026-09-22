package tracker

import (
	"context"
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
	if _, err := tr.Create(context.Background(), core.Task{ID: "AT-4", Objective: "unrepresentable", Checks: []core.Check{{Name: "test", Command: []string{"go", "test"}}}}, page.TrackerRevision); err == nil {
		t.Fatal("silently discarded task check")
	}
	got, _ = os.ReadFile(path)
	if string(got) != before {
		t.Fatal("failed mutation changed tracker")
	}
}
