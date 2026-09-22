package start

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/thebpandey/agent-team/vnext/internal/core"
	"github.com/thebpandey/agent-team/vnext/internal/knowledge"
	"github.com/thebpandey/agent-team/vnext/internal/run"
	"github.com/thebpandey/agent-team/vnext/internal/store"
	"github.com/thebpandey/agent-team/vnext/internal/tracker"
)

func TestAdmitDefaultPersistsCanonicalRunTeamAndPacket(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".beads"), 0o700); err != nil {
		t.Fatal(err)
	}
	worktree := filepath.Join(root, "worktree")
	if err := os.Mkdir(worktree, 0o700); err != nil {
		t.Fatal(err)
	}
	task := core.Task{RecordEnvelope: core.RecordEnvelope{Schema: 1, Revision: 7}, ID: "TASK-1", Objective: "implement", State: core.Ready, Criteria: []string{"works"}, WritablePaths: []string{"src"}}
	selected := &fakeTracker{page: core.TrackerPage{TrackerRevision: 7, TotalNonArchived: 1, Tasks: []core.Task{task}}}
	got, err := AdmitDefault(context.Background(), store.New(root, core.DefaultConfig().Storage), root, selected, "codex", worktree, "base")
	if err != nil || !got.HostDispatchRequired || got.PacketPath == "" || got.Run.ID == "" || got.Team.ID == "" || got.Packet.Task != task.ID {
		t.Fatalf("admission=%#v err=%v", got, err)
	}
	if err := knowledge.ValidatePacket(got.Packet, got.PacketDigest); err != nil {
		t.Fatalf("packet=%#v digest=%q err=%v", got.Packet, got.PacketDigest, err)
	}
	if raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(got.PacketPath))); err != nil || len(raw) == 0 {
		t.Fatalf("packet bytes=%q err=%v", raw, err)
	}
	repos := run.NewRepositories(store.New(root, core.DefaultConfig().Storage))
	persistedRun, err := repos.Runs.Read(context.Background(), got.Run.ID)
	if err != nil || len(persistedRun.Teams) != 1 || persistedRun.Teams[0].Queue[0] != task.ID {
		t.Fatalf("run=%#v err=%v", persistedRun, err)
	}
	persistedTeam, err := repos.Teams.Read(context.Background(), got.Team.ID)
	if err != nil || persistedTeam.QueueFingerprint != got.Packet.QueueFingerprint || persistedTeam.State != core.Working {
		t.Fatalf("team=%#v err=%v", persistedTeam, err)
	}
	again, err := AdmitDefault(context.Background(), store.New(root, core.DefaultConfig().Storage), root, selected, "codex", worktree, "base")
	if err != nil || !again.AlreadyAdmitted || again.PacketDigest != got.PacketDigest {
		t.Fatalf("duplicate=%#v err=%v", again, err)
	}
}

type fakeTracker struct{ page core.TrackerPage }

func (f *fakeTracker) AuthorityMetadata() tracker.AuthorityMetadata {
	return tracker.AuthorityMetadata{Kind: "beads", Ref: ".beads"}
}
func (f *fakeTracker) Page(context.Context, string, int) (core.TrackerPage, error) {
	return f.page, nil
}
func (f *fakeTracker) Get(_ context.Context, id core.TaskID, _ uint64) (core.Task, error) {
	for _, task := range f.page.Tasks {
		if task.ID == id {
			return task, nil
		}
	}
	return core.Task{}, core.ErrPath
}
func (f *fakeTracker) Refresh(context.Context, uint64) (core.TrackerPage, error) { return f.page, nil }
func (f *fakeTracker) Create(context.Context, core.Task, uint64) (core.Task, error) {
	return core.Task{}, core.ErrPhase
}
func (f *fakeTracker) Archive(context.Context, core.TaskID, string, uint64) error {
	return core.ErrPhase
}
