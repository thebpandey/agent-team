package start

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/thebpandey/agent-team/vnext/internal/contracts"
	"github.com/thebpandey/agent-team/vnext/internal/core"
	"github.com/thebpandey/agent-team/vnext/internal/knowledge"
	"github.com/thebpandey/agent-team/vnext/internal/run"
	"github.com/thebpandey/agent-team/vnext/internal/store"
)

func TestOneOffCannotReservePastProjectPause(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	st := store.New(root, core.DefaultConfig().Storage)
	if _, err := RequestControl(ctx, st, "pause", core.Scope{Kind: core.ScopeProject, ID: root}); err != nil {
		t.Fatal(err)
	}
	manager := &recordingWorktree{path: filepath.Join(root, "worktree")}
	task := core.Task{ID: "one-task", Objective: "Requested change", Criteria: []string{"passes"}, WritablePaths: []string{"src"}}
	for _, kind := range []run.OneOffKind{run.Feature, run.Audit, run.Review} {
		if _, err := AdmitOneOffRegistered(ctx, st, root, kind, task, manager, "claude", "HEAD"); !errors.Is(err, core.ErrTransition) {
			t.Fatalf("%s bypassed pause: %v", kind, err)
		}
	}
	if manager.called {
		t.Fatal("paused one-off created a worktree")
	}
	if _, err := os.Stat(filepath.Join(root, ".agent-team", "runs")); !os.IsNotExist(err) {
		t.Fatalf("paused one-off created run state: %v", err)
	}
}

func TestOneOffSharesNativeReservationAndCompletion(t *testing.T) {
	for _, kind := range []run.OneOffKind{run.Feature, run.Audit, run.Review} {
		t.Run(string(kind), func(t *testing.T) {
			ctx := context.Background()
			root := t.TempDir()
			st := store.New(root, core.DefaultConfig().Storage)
			task := core.Task{ID: "one-task", Objective: "Inspect approved change", Criteria: []string{"Report findings"}, WritablePaths: []string{"src"}, Resources: []string{"browser:1"}, Checks: []core.Check{{Name: "test", Command: []string{"go", "test", "./..."}}}}
			manager := &recordingWorktree{path: filepath.Join(root, "worktree")}
			first, err := AdmitOneOffRegistered(ctx, st, root, kind, task, manager, "claude", "HEAD")
			if err != nil {
				t.Fatal(err)
			}
			if !first.HostDispatchRequired || first.AlreadyAdmitted || first.Run.Mode != "one-off" || first.Packet.SpecRevision != first.Run.ManifestDigest {
				t.Fatalf("one-off reservation=%+v", first)
			}
			if kind == run.Feature {
				if !manager.called || len(first.Packet.Scope) != 1 {
					t.Fatalf("feature has no managed scope: %+v", first.Packet)
				}
			} else if manager.called || len(first.Packet.Scope) != 0 || first.Packet.Worktree != root {
				t.Fatalf("read-only task acquired write scope: %+v", first.Packet)
			}
			if err := knowledge.ValidatePacket(first.Packet, first.PacketDigest); err != nil {
				t.Fatal(err)
			}
			replay, err := AdmitOneOffRegistered(ctx, st, root, kind, task, manager, "codex", "HEAD")
			if err != nil || !replay.AlreadyAdmitted || replay.HostDispatchRequired || replay.PacketDigest != first.PacketDigest || replay.Packet.Owner != "claude" {
				t.Fatalf("replay=%+v err=%v", replay, err)
			}
			handle := contracts.WorkerHandle{Host: "claude", Identity: "observed-agent", Run: first.Run.ID, Team: first.Team.ID, Task: task.ID, PacketDigest: first.PacketDigest, CandidateRevision: first.Packet.SpecRevision}
			if _, err := Acknowledge(ctx, st, first.Team.ID, first.PacketDigest, handle); err != nil {
				t.Fatal(err)
			}
			if _, err := Complete(ctx, st, first.Team.ID, handle); err != nil {
				t.Fatal(err)
			}
			if _, err := RecordIndependentClean(ctx, st, first.Team.ID, "independent-reviewer"); err != nil {
				t.Fatal(err)
			}
			if _, err := RecordIdle(ctx, st, first.Team.ID, handle); err != nil {
				t.Fatal(err)
			}
			if _, _, _, err := ConsumeHead(ctx, st, first.Team.ID); err != nil {
				t.Fatal(err)
			}
			finished, err := AdmitOneOffRegistered(ctx, st, root, kind, task, manager, "codex", "HEAD")
			if err != nil || !finished.AlreadyAdmitted || finished.HostDispatchRequired || len(finished.Team.Queue) != 0 || finished.PacketDigest != first.PacketDigest {
				t.Fatalf("completed task was readmitted: %+v err=%v", finished, err)
			}
			if _, err := os.Stat(filepath.Join(root, ".agent-team", "config.json")); !os.IsNotExist(err) {
				t.Fatalf("one-off changed project setup: %v", err)
			}
		})
	}
}
