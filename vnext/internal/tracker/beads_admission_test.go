package tracker_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/thebpandey/agent-team/vnext/internal/run"
	"github.com/thebpandey/agent-team/vnext/internal/tracker"
)

func TestBeadsCompletedPrerequisiteAllowsDependentAdmission(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".beads"), 0700); err != nil {
		t.Fatal(err)
	}
	body := []byte(`[{"id":"B-1","title":"prerequisite","status":"closed","closed_at":"2026-09-22T19:53:30Z","close_reason":"verified"},{"id":"B-2","title":"dependent","status":"open","dependencies":[{"issue_id":"B-2","depends_on_id":"B-1","type":"blocks"}],"metadata":{"writablePaths":["src"],"criteria":["works"],"checks":[{"name":"test","command":["true"]}]}}]`)
	selected := tracker.NewBeads(tracker.NewFakeRunner(tracker.CommandResult{Stdout: body}))
	got, err := run.PrepareSinglePlanAdmission(context.Background(), root, selected)
	if err != nil || len(got.Batch.Tasks) != 1 || got.Batch.Tasks[0] != "B-2" {
		t.Fatalf("dependent admission = %#v, %v", got, err)
	}
}
