package start

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/thebpandey/agent-team/vnext/internal/contracts"
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
	packetBefore, _ := os.ReadFile(filepath.Join(root, filepath.FromSlash(got.PacketPath)))
	if _, err := AdmitDefault(context.Background(), store.New(root, core.DefaultConfig().Storage), root, selected, "other-owner", worktree, "base"); err == nil {
		t.Fatal("conflicting retry rebound an admitted packet")
	}
	packetAfter, _ := os.ReadFile(filepath.Join(root, filepath.FromSlash(got.PacketPath)))
	if string(packetBefore) != string(packetAfter) {
		t.Fatal("conflicting retry rewrote packet bytes")
	}
	handle := contracts.WorkerHandle{Host: "codex", Identity: "team-agent", Run: got.Run.ID, Team: got.Team.ID, Task: got.Packet.Task, PacketDigest: got.PacketDigest, CandidateRevision: got.Packet.SpecRevision}
	acknowledged, err := Acknowledge(context.Background(), store.New(root, core.DefaultConfig().Storage), got.Team.ID, got.PacketDigest, handle)
	if err != nil || acknowledged.Handle != handle || acknowledged.IntentDigest != got.PacketDigest {
		t.Fatalf("acknowledged=%#v err=%v", acknowledged, err)
	}
	teamRaw, err := os.ReadFile(filepath.Join(root, ".agent-team", "teams", string(got.Team.ID)+".json"))
	if err != nil || len(teamRaw) == 0 {
		t.Fatalf("team bytes=%q err=%v", teamRaw, err)
	}
	reloadedTeam, err := repos.Teams.Read(context.Background(), got.Team.ID)
	if err != nil || reloadedTeam.Handle != handle {
		t.Fatalf("reloaded team=%#v err=%v", reloadedTeam, err)
	}
	if _, err := Acknowledge(context.Background(), store.New(root, core.DefaultConfig().Storage), got.Team.ID, got.PacketDigest, handle); err == nil {
		t.Fatal("duplicate acknowledgement accepted")
	}
	if _, err := Complete(context.Background(), store.New(root, core.DefaultConfig().Storage), got.Team.ID, handle); err != nil {
		t.Fatal(err)
	}
	if _, err := RecordIndependentClean(context.Background(), store.New(root, core.DefaultConfig().Storage), got.Team.ID, handle.Identity); err == nil {
		t.Fatal("developer self-review accepted")
	}
	if _, err := RecordIndependentClean(context.Background(), store.New(root, core.DefaultConfig().Storage), got.Team.ID, "independent-reviewer"); err != nil {
		t.Fatal(err)
	}
	if _, err := RecordIdle(context.Background(), store.New(root, core.DefaultConfig().Storage), got.Team.ID, handle); err != nil {
		t.Fatal(err)
	}
	consumed, retained, idle, err := ConsumeHead(context.Background(), store.New(root, core.DefaultConfig().Storage), got.Team.ID)
	if err != nil || consumed != got.Packet.Task || retained != handle || idle.State != core.Idle || len(idle.Queue) != 0 || idle.Handle.Identity != "" {
		t.Fatalf("consumed=%q retained=%#v idle=%#v err=%v", consumed, retained, idle, err)
	}
}

func TestAdmitDefaultRegisteredBindsPacketToManagerWorktree(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".beads"), 0o700); err != nil {
		t.Fatal(err)
	}
	task := core.Task{RecordEnvelope: core.RecordEnvelope{Schema: 1, Revision: 7}, ID: "TASK-1", Objective: "implement", State: core.Ready, WritablePaths: []string{"src"}}
	manager := &recordingWorktree{path: filepath.Join(root, ".agent-team", "worktrees", "registered")}
	got, err := AdmitDefaultRegistered(context.Background(), store.New(root, core.DefaultConfig().Storage), root, &fakeTracker{page: core.TrackerPage{TrackerRevision: 7, TotalNonArchived: 1, Tasks: []core.Task{task}}}, manager, "codex", "base")
	if err != nil || !manager.called || got.Packet.Worktree != manager.path || !got.HostDispatchRequired {
		t.Fatalf("result=%#v called=%v err=%v", got, manager.called, err)
	}
}

func TestFollowupRequiresFreshPacketAndSameHostAcknowledgement(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".beads"), 0o700); err != nil {
		t.Fatal(err)
	}
	tasks := []core.Task{
		{ID: "TASK-1", Objective: "first", State: core.Ready, Criteria: []string{"first works"}, WritablePaths: []string{"src"}},
		{ID: "TASK-2", Objective: "second", State: core.Ready, Criteria: []string{"second works"}, WritablePaths: []string{"src"}},
	}
	manifest, err := run.CreateOneOff(context.Background(), root, run.Feature, "bounded follow-up", tasks)
	if err != nil || len(manifest.Teams) != 1 || len(manifest.Teams[0].Queue) != 2 {
		t.Fatalf("manifest=%#v err=%v", manifest, err)
	}
	st := store.New(root, core.DefaultConfig().Storage)
	repos := run.NewRepositories(st)
	if _, err := repos.Runs.Initialize(context.Background(), manifest); err != nil {
		t.Fatal(err)
	}
	if _, err := repos.Teams.Initialize(context.Background(), manifest.Teams[0]); err != nil {
		t.Fatal(err)
	}
	team := manifest.Teams[0]
	worktree := filepath.Join(root, "worktree")
	if err := os.Mkdir(worktree, 0o700); err != nil {
		t.Fatal(err)
	}
	first := core.AssignmentPacket{RecordEnvelope: core.RecordEnvelope{Schema: 1, Project: manifest.Project, RunID: manifest.ID, WrittenAt: manifest.WrittenAt, Revision: manifest.Revision}, SpecRevision: "one-off-spec", Task: tasks[0].ID, Team: team.ID, QueueFingerprint: team.QueueFingerprint, Owner: "codex", Worktree: worktree, Base: "base", Criteria: tasks[0].Criteria, Scope: tasks[0].WritablePaths, NextAction: "host_dispatch_required"}
	digest, err := knowledge.PacketDigest(first)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateJSON(packetPathFor(team.ID, digest), packetRecord{Packet: first, Digest: digest}, core.DefaultConfig().Storage.CanonicalBytes); err != nil {
		t.Fatal(err)
	}
	if _, err := mutateTeam(context.Background(), st, manifest.ID, team.ID, func(value *run.TeamRecord) error { value.IntentDigest = digest; return nil }); err != nil {
		t.Fatal(err)
	}
	selected := &fakeTracker{page: core.TrackerPage{Tasks: tasks}}
	handle := contracts.WorkerHandle{Host: "codex", Identity: "retained-agent", Run: manifest.ID, Team: team.ID, Task: tasks[0].ID, PacketDigest: digest, CandidateRevision: first.SpecRevision}
	if _, err := Acknowledge(context.Background(), st, team.ID, digest, handle); err != nil {
		t.Fatal(err)
	}
	if _, err := Complete(context.Background(), st, team.ID, handle); err != nil {
		t.Fatal(err)
	}
	if _, err := RecordIndependentClean(context.Background(), st, team.ID, "independent-reviewer"); err != nil {
		t.Fatal(err)
	}
	if _, err := RecordIdle(context.Background(), st, team.ID, handle); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(filepath.Join(root, ".agent-team", "teams", string(team.ID)+".json"))
	if err != nil {
		t.Fatal(err)
	}
	consumed, delta, waiting, err := ConsumeForFollowup(context.Background(), st, selected, team.ID)
	if err != nil || consumed != tasks[0].ID || delta.Packet.Task != tasks[1].ID || delta.Retained != handle || waiting.Handle.Identity != "" || waiting.RetainedHandle != handle || waiting.IntentDigest != delta.PacketDigest {
		t.Fatalf("consumed=%q delta=%#v waiting=%#v err=%v", consumed, delta, waiting, err)
	}
	after, err := os.ReadFile(filepath.Join(root, ".agent-team", "teams", string(team.ID)+".json"))
	if err != nil || string(before) == string(after) {
		t.Fatalf("team persistence bytes before=%q after=%q err=%v", before, after, err)
	}
	wrongHost := handle
	wrongHost.Task, wrongHost.PacketDigest = tasks[1].ID, delta.PacketDigest
	wrongHost.CandidateRevision = delta.Packet.SpecRevision
	wrongHost.Identity = "replacement-agent"
	if _, err := Acknowledge(context.Background(), st, team.ID, delta.PacketDigest, wrongHost); err == nil {
		t.Fatal("replacement host acknowledged retained follow-up")
	}
	nextHandle := handle
	nextHandle.Task, nextHandle.PacketDigest = tasks[1].ID, delta.PacketDigest
	nextHandle.CandidateRevision = delta.Packet.SpecRevision
	acknowledged, err := Acknowledge(context.Background(), st, team.ID, delta.PacketDigest, nextHandle)
	if err != nil || acknowledged.Handle != nextHandle || acknowledged.RetainedHandle != handle {
		t.Fatalf("acknowledged=%#v err=%v", acknowledged, err)
	}
	reloaded, err := repos.Teams.Read(context.Background(), team.ID)
	if err != nil || reloaded.Handle != nextHandle || reloaded.IntentDigest != delta.PacketDigest {
		t.Fatalf("reloaded=%#v err=%v", reloaded, err)
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

type recordingWorktree struct {
	path   string
	called bool
}

func (m *recordingWorktree) Create(_ context.Context, spec contracts.WorktreeSpec) (contracts.Worktree, error) {
	m.called = true
	return contracts.Worktree{Run: spec.Run, Team: spec.Team, Path: m.path, Base: spec.Base}, nil
}
func (*recordingWorktree) Inspect(context.Context, contracts.Worktree) (contracts.Worktree, error) {
	return contracts.Worktree{}, core.ErrPhase
}
func (*recordingWorktree) Integrate(context.Context, contracts.Candidate) (contracts.Candidate, error) {
	return contracts.Candidate{}, core.ErrPhase
}
func (*recordingWorktree) Cleanup(context.Context, core.TeamID) error { return core.ErrPhase }
