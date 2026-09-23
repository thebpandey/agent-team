package start

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/thebpandey/agent-team/vnext/internal/contracts"
	"github.com/thebpandey/agent-team/vnext/internal/core"
	"github.com/thebpandey/agent-team/vnext/internal/run"
	"github.com/thebpandey/agent-team/vnext/internal/store"
)

func TestNativeHostSwitchObservesExistingReservation(t *testing.T) {
	for _, originalHost := range []string{"codex", "claude"} {
		for _, state := range []string{"reserved", "acknowledged", "reviewed-idle"} {
			t.Run(originalHost+"/"+state, func(t *testing.T) {
				ctx := context.Background()
				root := t.TempDir()
				if err := os.Mkdir(filepath.Join(root, ".beads"), 0700); err != nil {
					t.Fatal(err)
				}
				otherHost := "codex"
				if originalHost == otherHost {
					otherHost = "claude"
				}
				task := core.Task{RecordEnvelope: core.RecordEnvelope{Schema: 1, Revision: 7}, ID: "TASK-1", Objective: "implement", State: core.Ready, WritablePaths: []string{"src"}}
				selected := &fakeTracker{page: core.TrackerPage{TrackerRevision: 7, TotalNonArchived: 1, Tasks: []core.Task{task}}}
				st := store.New(root, core.DefaultConfig().Storage)
				worktree := filepath.Join(root, "worktree")
				original, err := AdmitDefault(ctx, st, root, selected, originalHost, worktree, "base")
				if err != nil {
					t.Fatal(err)
				}
				handle := contracts.WorkerHandle{Host: originalHost, Identity: "native-worker", Run: original.Run.ID, Team: original.Team.ID, Task: task.ID, PacketDigest: original.PacketDigest, CandidateRevision: original.Packet.SpecRevision}
				if state != "reserved" {
					if _, err := Acknowledge(ctx, st, original.Team.ID, original.PacketDigest, handle); err != nil {
						t.Fatal(err)
					}
				}
				if state == "reviewed-idle" {
					if _, err := Complete(ctx, st, original.Team.ID, handle); err != nil {
						t.Fatal(err)
					}
					if _, err := RecordIndependentClean(ctx, st, original.Team.ID, "independent-reviewer"); err != nil {
						t.Fatal(err)
					}
					if _, err := RecordIdle(ctx, st, original.Team.ID, handle); err != nil {
						t.Fatal(err)
					}
				}
				before, err := run.NewRepositories(st).Teams.Read(ctx, original.Team.ID)
				if err != nil {
					t.Fatal(err)
				}
				replay, err := AdmitDefault(ctx, st, root, selected, otherHost, worktree, "base")
				if err != nil {
					t.Fatalf("foreground host switch failed: %v", err)
				}
				if !replay.AlreadyAdmitted || replay.HostDispatchRequired || replay.PacketDigest != original.PacketDigest || replay.PacketPath != original.PacketPath || !reflect.DeepEqual(replay.Packet, original.Packet) || !reflect.DeepEqual(replay.Team, before) {
					t.Fatalf("host switch changed reservation: original=%#v replay=%#v prior team=%#v", original, replay, before)
				}
				wrongHost := handle
				wrongHost.Host = otherHost
				if _, err := Acknowledge(ctx, st, original.Team.ID, original.PacketDigest, wrongHost); err == nil {
					t.Fatal("foreground host switch transferred worker ownership")
				}
				if state == "reserved" {
					if _, err := Acknowledge(ctx, st, original.Team.ID, original.PacketDigest, handle); err != nil {
						t.Fatalf("original host cannot acknowledge after observed retry: %v", err)
					}
				} else if _, err := Complete(ctx, st, original.Team.ID, wrongHost); err == nil {
					t.Fatal("switched host completed original worker's task")
				}
			})
		}
	}
}
