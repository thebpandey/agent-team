package start

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/thebpandey/agent-team/vnext/internal/contracts"
	"github.com/thebpandey/agent-team/vnext/internal/core"
	"github.com/thebpandey/agent-team/vnext/internal/run"
	"github.com/thebpandey/agent-team/vnext/internal/store"
)

func controlFixture(t *testing.T) (*store.Store, *fakeTracker, string) {
	t.Helper()
	root := t.TempDir()
	worktree := filepath.Join(root, "worktree")
	for _, path := range []string{worktree, filepath.Join(root, ".beads")} {
		if err := os.Mkdir(path, 0700); err != nil {
			t.Fatal(err)
		}
	}
	tasks := []core.Task{{RecordEnvelope: core.RecordEnvelope{Schema: 1, Revision: 7}, ID: "TASK-1", Objective: "first", State: core.Ready, Criteria: []string{"works"}, WritablePaths: []string{"src"}}, {RecordEnvelope: core.RecordEnvelope{Schema: 1, Revision: 7}, ID: "TASK-2", Objective: "second", State: core.Ready, Criteria: []string{"works"}, WritablePaths: []string{"src"}}}
	return store.New(root, core.DefaultConfig().Storage), &fakeTracker{page: core.TrackerPage{TrackerRevision: 7, TotalNonArchived: 2, Tasks: tasks}}, worktree
}

func TestProjectControlHoldsFreshAdmissionBeforeAnyRun(t *testing.T) {
	ctx := context.Background()
	st, selected, worktree := controlFixture(t)
	scope := core.Scope{Kind: core.ScopeProject, ID: st.Root}
	held, err := RequestControl(ctx, st, "pause", scope)
	if err != nil || !held.AdmissionHeld || held.HostControlRequired || held.Status != "admission_held" {
		t.Fatalf("hold=%#v err=%v", held, err)
	}
	manager := &recordingWorktree{path: worktree}
	if _, err := AdmitDefaultRegistered(ctx, st, st.Root, selected, manager, "codex", "base"); !errors.Is(err, core.ErrTransition) {
		t.Fatalf("fresh reservation ignored hold: %v", err)
	}
	if manager.called {
		t.Fatal("held reservation created worktree")
	}
	if _, err := os.Stat(filepath.Join(st.Root, ".agent-team", "runs")); !os.IsNotExist(err) {
		t.Fatal("held reservation wrote run")
	}
	if _, err := RequestControl(ctx, st, "resume", scope); err != nil {
		t.Fatal(err)
	}
	if _, err := AdmitDefault(ctx, st, st.Root, selected, "codex", worktree, "base"); err != nil {
		t.Fatalf("resume did not reopen admission: %v", err)
	}
}

func TestControlRequestsRequireExactHostObservationAndPreserveWorker(t *testing.T) {
	ctx := context.Background()
	st, selected, worktree := controlFixture(t)
	admitted, err := AdmitDefault(ctx, st, st.Root, selected, "codex", worktree, "base")
	if err != nil {
		t.Fatal(err)
	}
	handle := contracts.WorkerHandle{Host: "codex", Identity: "real-worker", Run: admitted.Run.ID, Team: admitted.Team.ID, Task: admitted.Packet.Task, PacketDigest: admitted.PacketDigest, CandidateRevision: admitted.Packet.SpecRevision}
	if _, err := Acknowledge(ctx, st, handle.Team, handle.PacketDigest, handle); err != nil {
		t.Fatal(err)
	}
	scope := core.Scope{Kind: core.ScopeTeam, ID: string(handle.Team)}
	before, err := run.NewRepositories(st).Teams.Read(ctx, handle.Team)
	if err != nil {
		t.Fatal(err)
	}
	paused, err := RequestControl(ctx, st, "pause", scope)
	if err != nil || paused.Status != "pause_requested" || !paused.HostControlRequired || len(paused.Handles) != 1 || paused.Handles[0] != handle {
		t.Fatalf("request=%#v err=%v", paused, err)
	}
	foreign := handle
	foreign.Identity = "other-worker"
	if _, err := AcknowledgeControl(ctx, st, paused.ControlID, foreign, "paused"); !errors.Is(err, core.ErrRevision) {
		t.Fatalf("foreign acknowledgement accepted: %v", err)
	}
	observed, err := AcknowledgeControl(ctx, st, paused.ControlID, handle, "paused")
	if err != nil || observed.Status != "paused" || observed.HostControlRequired {
		t.Fatalf("observed=%#v err=%v", observed, err)
	}
	resumed, err := RequestControl(ctx, st, "resume", scope)
	if err != nil || resumed.Status != "resume_requested" || resumed.AdmissionHeld || !resumed.HostControlRequired || resumed.Handles[0] != handle {
		t.Fatalf("resume=%#v err=%v", resumed, err)
	}
	if _, err := AcknowledgeControl(ctx, st, paused.ControlID, handle, "paused"); !errors.Is(err, core.ErrRevision) {
		t.Fatalf("stale acknowledgement accepted: %v", err)
	}
	if _, err := AcknowledgeControl(ctx, st, resumed.ControlID, handle, "running"); err != nil {
		t.Fatal(err)
	}
	after, err := run.NewRepositories(st).Teams.Read(ctx, handle.Team)
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatal("control retagged or changed worker authority")
	}
}

func TestControlHoldsQueuedAndRetainedFollowupReservations(t *testing.T) {
	ctx := context.Background()
	st, selected, worktree := controlFixture(t)
	admitted, err := AdmitDefault(ctx, st, st.Root, selected, "codex", worktree, "base")
	if err != nil {
		t.Fatal(err)
	}
	handle := contracts.WorkerHandle{Host: "codex", Identity: "real-worker", Run: admitted.Run.ID, Team: admitted.Team.ID, Task: admitted.Packet.Task, PacketDigest: admitted.PacketDigest, CandidateRevision: admitted.Packet.SpecRevision}
	if _, err := Acknowledge(ctx, st, handle.Team, handle.PacketDigest, handle); err != nil {
		t.Fatal(err)
	}
	scope := core.Scope{Kind: core.ScopeProject, ID: st.Root}
	if _, err := RequestControl(ctx, st, "stop", scope); err != nil {
		t.Fatal(err)
	}
	if _, err := AppendQueue(ctx, st, selected, admitted.Run.ID, admitted.Team.ID, []core.TaskID{"TASK-2"}); !errors.Is(err, core.ErrTransition) {
		t.Fatalf("append bypassed stop: %v", err)
	}
	if _, err := RequestControl(ctx, st, "resume", scope); err != nil {
		t.Fatal(err)
	}
	if _, err := AppendQueue(ctx, st, selected, admitted.Run.ID, admitted.Team.ID, []core.TaskID{"TASK-2"}); err != nil {
		t.Fatal(err)
	}
	if _, err := Complete(ctx, st, handle.Team, handle); err != nil {
		t.Fatal(err)
	}
	if _, err := RecordIndependentClean(ctx, st, handle.Team, "reviewer"); err != nil {
		t.Fatal(err)
	}
	if _, err := RecordIdle(ctx, st, handle.Team, handle); err != nil {
		t.Fatal(err)
	}
	before, _ := run.NewRepositories(st).Teams.Read(ctx, handle.Team)
	if _, err := RequestControl(ctx, st, "pause", scope); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := ConsumeForFollowup(ctx, st, selected, handle.Team); !errors.Is(err, core.ErrTransition) {
		t.Fatalf("retained followup bypassed hold: %v", err)
	}
	after, _ := run.NewRepositories(st).Teams.Read(ctx, handle.Team)
	if !reflect.DeepEqual(before, after) {
		t.Fatal("held followup consumed task")
	}
}

func TestResumeOneScopeDoesNotClearBroaderHold(t *testing.T) {
	ctx := context.Background()
	st, selected, worktree := controlFixture(t)
	admitted, err := AdmitDefault(ctx, st, st.Root, selected, "codex", worktree, "base")
	if err != nil {
		t.Fatal(err)
	}
	handle := contracts.WorkerHandle{Host: "codex", Identity: "real-worker", Run: admitted.Run.ID, Team: admitted.Team.ID, Task: admitted.Packet.Task, PacketDigest: admitted.PacketDigest, CandidateRevision: admitted.Packet.SpecRevision}
	if _, err := Acknowledge(ctx, st, handle.Team, handle.PacketDigest, handle); err != nil {
		t.Fatal(err)
	}
	projectScope := core.Scope{Kind: core.ScopeProject, ID: st.Root}
	teamScope := core.Scope{Kind: core.ScopeTeam, ID: string(admitted.Team.ID)}
	if _, err := RequestControl(ctx, st, "pause", projectScope); err != nil {
		t.Fatal(err)
	}
	if _, err := RequestControl(ctx, st, "pause", teamScope); err != nil {
		t.Fatal(err)
	}
	resumed, err := RequestControl(ctx, st, "resume", teamScope)
	if err != nil || !resumed.AdmissionHeld || resumed.HostControlRequired || len(resumed.BlockingScopes) != 1 {
		t.Fatalf("team resume offered continuation through project hold: %#v %v", resumed, err)
	}
	if err := AdmissionAllowed(ctx, st, admitted.Packet); !errors.Is(err, core.ErrTransition) {
		t.Fatalf("team resume bypassed project hold: %v", err)
	}
	if _, err := AcknowledgeControl(ctx, st, resumed.ControlID, handle, "running"); !errors.Is(err, core.ErrTransition) {
		t.Fatalf("resume observation bypassed project hold: %v", err)
	}
}

func TestRetainedEmptyQueueCannotReservePastHold(t *testing.T) {
	ctx := context.Background()
	st, selected, worktree := controlFixture(t)
	admitted, err := AdmitDefault(ctx, st, st.Root, selected, "codex", worktree, "base")
	if err != nil {
		t.Fatal(err)
	}
	handle := contracts.WorkerHandle{Host: "codex", Identity: "retained", Run: admitted.Run.ID, Team: admitted.Team.ID, Task: admitted.Packet.Task, PacketDigest: admitted.PacketDigest, CandidateRevision: admitted.Packet.SpecRevision}
	if _, err := Acknowledge(ctx, st, handle.Team, handle.PacketDigest, handle); err != nil {
		t.Fatal(err)
	}
	if _, err := Complete(ctx, st, handle.Team, handle); err != nil {
		t.Fatal(err)
	}
	if _, err := RecordIndependentClean(ctx, st, handle.Team, "reviewer"); err != nil {
		t.Fatal(err)
	}
	if _, err := RecordIdle(ctx, st, handle.Team, handle); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := ConsumeHead(ctx, st, handle.Team); err != nil {
		t.Fatal(err)
	}
	if _, err := AppendQueue(ctx, st, selected, handle.Run, handle.Team, []core.TaskID{"TASK-2"}); err != nil {
		t.Fatal(err)
	}
	before, _ := run.NewRepositories(st).Teams.Read(ctx, handle.Team)
	scope := core.Scope{Kind: core.ScopeProject, ID: st.Root}
	if _, err := RequestControl(ctx, st, "cancel", scope); err != nil {
		t.Fatal(err)
	}
	if _, reserved, err := ReserveRetainedHead(ctx, st, selected, handle.Team); !errors.Is(err, core.ErrTransition) || reserved {
		t.Fatalf("retained reservation bypassed cancel: reserved=%v err=%v", reserved, err)
	}
	after, _ := run.NewRepositories(st).Teams.Read(ctx, handle.Team)
	if !reflect.DeepEqual(before, after) {
		t.Fatal("held reservation changed team")
	}
}

func TestMalformedControlsFailClosed(t *testing.T) {
	ctx := context.Background()
	st, selected, worktree := controlFixture(t)
	if _, err := RequestControl(ctx, st, "pause", core.Scope{Kind: core.ScopeProject, ID: st.Root}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(st.Root, controlsPath), []byte(`{"schema":1}`), 0600); err != nil {
		t.Fatal(err)
	}
	manager := &recordingWorktree{path: worktree}
	if _, err := AdmitDefaultRegistered(ctx, st, st.Root, selected, manager, "codex", "base"); !errors.Is(err, core.ErrRevision) || manager.called {
		t.Fatalf("malformed control allowed admission: %v called=%v", err, manager.called)
	}
}
