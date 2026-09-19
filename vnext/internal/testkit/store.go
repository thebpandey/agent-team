package testkit

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/thebpandey/agent-team/vnext/internal/core"
	"github.com/thebpandey/agent-team/vnext/internal/run"
	"github.com/thebpandey/agent-team/vnext/internal/store"
	"github.com/thebpandey/agent-team/vnext/internal/tracker"
)

// NewAdmissionFixture creates a canonical plan run with one idle retained
// team and twelve ready, dependency-free tracker tasks.
func NewAdmissionFixture(t *testing.T) AdmissionFixture {
	t.Helper()
	root := t.TempDir()
	trackerPath := filepath.Join(root, "TASKS.md")
	if err := os.WriteFile(trackerPath, []byte("# fixture\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	const revision uint64 = 101
	tasks := make([]core.Task, 12)
	revisions := make(map[core.TaskID]uint64, len(tasks))
	for i := range tasks {
		id := core.TaskID(fmt.Sprintf("T-%04d", i+1))
		tasks[i] = core.Task{RecordEnvelope: core.RecordEnvelope{Revision: uint64(1000 + i)}, ID: id, Objective: "fixture task", State: core.Ready, Criteria: []string{"passes"}, WritablePaths: []string{fmt.Sprintf("src/task-%04d", i+1)}, Resources: []string{fmt.Sprintf("resource:%04d", i+1)}}
		revisions[id] = tasks[i].Revision
	}
	selected := fixtureTracker{path: trackerPath, revision: revision, tasks: tasks}
	manifest, err := run.CreatePlan(context.Background(), root, selected)
	if err != nil {
		t.Fatal(err)
	}
	team := run.TeamRecord{
		RecordEnvelope: core.RecordEnvelope{Schema: 1, Project: manifest.Project, RunID: manifest.ID, WrittenAt: manifest.WrittenAt, Revision: 1},
		ID:             core.TeamID(string(manifest.ID) + "-team-1"),
		State:          core.Idle,
	}
	manifest.Teams = []run.TeamRecord{team}
	st := store.New(root, core.StorageLimits{})
	repos := run.NewRepositories(st)
	if _, err := repos.Runs.Initialize(context.Background(), manifest); err != nil {
		t.Fatal(err)
	}
	if _, err := repos.Teams.Initialize(context.Background(), team); err != nil {
		t.Fatal(err)
	}
	return AdmissionFixture{Store: st, Tracker: selected, Run: manifest.ID, RunRevision: manifest.Revision, Team: team.ID, TeamRevision: team.Revision, TrackerRevision: revision, TaskRevisions: revisions, project: manifest.Project, writtenAt: manifest.WrittenAt}
}

type fixtureTracker struct {
	path     string
	revision uint64
	tasks    []core.Task
}

func (f fixtureTracker) AuthorityMetadata() tracker.AuthorityMetadata {
	return tracker.AuthorityMetadata{Kind: "tasks-md", Ref: f.path}
}

func (f fixtureTracker) Page(ctx context.Context, cursor string, limit int) (core.TrackerPage, error) {
	if err := ctx.Err(); err != nil {
		return core.TrackerPage{}, err
	}
	if cursor != "" || limit < len(f.tasks) {
		return core.TrackerPage{}, fmt.Errorf("%w: fixture requires one snapshot", core.ErrLimit)
	}
	return core.TrackerPage{TrackerRevision: f.revision, TotalNonArchived: len(f.tasks), Tasks: append([]core.Task(nil), f.tasks...)}, nil
}

func (f fixtureTracker) Get(ctx context.Context, id core.TaskID, expected uint64) (core.Task, error) {
	if err := ctx.Err(); err != nil {
		return core.Task{}, err
	}
	if expected != f.revision {
		return core.Task{}, fmt.Errorf("%w: tracker revision", core.ErrRevision)
	}
	for _, task := range f.tasks {
		if task.ID == id {
			return task, nil
		}
	}
	return core.Task{}, fmt.Errorf("%w: task %s", core.ErrPath, id)
}
func (f fixtureTracker) Refresh(ctx context.Context, expected uint64) (core.TrackerPage, error) {
	return f.Page(ctx, "", len(f.tasks))
}
func (f fixtureTracker) Create(context.Context, core.Task, uint64) (core.Task, error) {
	return core.Task{}, fmt.Errorf("%w: fixture", core.ErrSettings)
}
func (f fixtureTracker) Archive(context.Context, core.TaskID, string, uint64) error {
	return fmt.Errorf("%w: fixture", core.ErrSettings)
}
