package start

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/thebpandey/agent-team/vnext/internal/contracts"
	"github.com/thebpandey/agent-team/vnext/internal/core"
	"github.com/thebpandey/agent-team/vnext/internal/knowledge"
	"github.com/thebpandey/agent-team/vnext/internal/store"
)

func TestLegacyPacketWithoutObjectiveKeepsDigest(t *testing.T) {
	// This is the serialized field order from before packets carried objectives.
	raw := []byte(`{"schema":1,"project":"project","runId":"RUN-1","writtenAt":"2026-01-01T00:00:00Z","revision":1,"specRevision":"spec","task":"TASK-1","team":"TEAM-1","queueFingerprint":"queue","owner":"codex","worktree":"worktree","base":"base","criteria":null,"scope":null,"checks":null,"capabilities":null,"skills":null,"resources":{"servers":null,"browsers":null,"external":null},"accelerators":null,"review":"","gate":"","nextAction":"host_dispatch_required","receiptPath":""}`)
	var packet core.AssignmentPacket
	if err := json.Unmarshal(raw, &packet); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(raw)
	if err := knowledge.ValidatePacket(packet, "sha256:"+hex.EncodeToString(sum[:])); err != nil {
		t.Fatalf("legacy packet digest changed: %v", err)
	}
}

func TestPacketsCarryImmutableTaskInstructions(t *testing.T) {
	for _, host := range []string{"codex", "claude"} {
		for _, mode := range []string{"fresh", "queued", "retained"} {
			t.Run(host+"/"+mode, func(t *testing.T) {
				ctx := context.Background()
				root := t.TempDir()
				if err := os.Mkdir(filepath.Join(root, ".beads"), 0o700); err != nil {
					t.Fatal(err)
				}
				tasks := []core.Task{
					{RecordEnvelope: core.RecordEnvelope{Schema: 1, Revision: 7}, ID: "TASK-1", Objective: "Implement first task", State: core.Ready, Criteria: []string{"first works"}, Checks: []core.Check{{Name: "first test", Command: []string{"go", "test", "./first"}}}, WritablePaths: []string{"src"}, Resources: []string{"browser:1"}},
					{RecordEnvelope: core.RecordEnvelope{Schema: 1, Revision: 7}, ID: "TASK-2", Objective: "Implement second task", State: core.Ready, Criteria: []string{"second works"}, Checks: []core.Check{{Name: "second test", Command: []string{"go", "test", "./second"}}}, WritablePaths: []string{"src"}, Resources: []string{"browser:2"}},
				}
				selected := &fakeTracker{page: core.TrackerPage{TrackerRevision: 7, TotalNonArchived: len(tasks), Tasks: tasks}}
				st := store.New(root, core.DefaultConfig().Storage)
				admitted, err := AdmitDefault(ctx, st, root, selected, host, filepath.Join(root, "worktree"), "base")
				if err != nil {
					t.Fatal(err)
				}
				packet, digest := admitted.Packet, admitted.PacketDigest
				taskIndex := 0
				if mode != "fresh" {
					handle := contracts.WorkerHandle{Host: host, Identity: "retained-worker", Run: admitted.Run.ID, Team: admitted.Team.ID, Task: tasks[0].ID, PacketDigest: digest, CandidateRevision: packet.SpecRevision}
					if _, err := Acknowledge(ctx, st, admitted.Team.ID, digest, handle); err != nil {
						t.Fatal(err)
					}
					if mode == "queued" {
						if _, err := AppendQueue(ctx, st, selected, admitted.Run.ID, admitted.Team.ID, []core.TaskID{tasks[1].ID}); err != nil {
							t.Fatal(err)
						}
					}
					if _, err := Complete(ctx, st, admitted.Team.ID, handle); err != nil {
						t.Fatal(err)
					}
					if _, err := RecordIndependentClean(ctx, st, admitted.Team.ID, "independent-reviewer"); err != nil {
						t.Fatal(err)
					}
					if _, err := RecordIdle(ctx, st, admitted.Team.ID, handle); err != nil {
						t.Fatal(err)
					}
					var delta Delta
					if mode == "queued" {
						_, delta, _, err = ConsumeForFollowup(ctx, st, selected, admitted.Team.ID)
					} else {
						if _, _, _, err := ConsumeHead(ctx, st, admitted.Team.ID); err != nil {
							t.Fatal(err)
						}
						if _, err := AppendQueue(ctx, st, selected, admitted.Run.ID, admitted.Team.ID, []core.TaskID{tasks[1].ID}); err != nil {
							t.Fatal(err)
						}
						delta, _, err = ReserveRetainedHead(ctx, st, selected, admitted.Team.ID)
					}
					if err != nil {
						t.Fatal(err)
					}
					packet, digest, taskIndex = delta.Packet, delta.PacketDigest, 1
				}
				want := tasks[taskIndex]
				if packet.Objective != want.Objective || !reflect.DeepEqual(packet.Checks, want.Checks) || !reflect.DeepEqual(packet.Criteria, want.Criteria) || !reflect.DeepEqual(packet.Scope, want.WritablePaths) || !reflect.DeepEqual(packet.Resources.External, want.Resources) {
					t.Fatalf("packet instructions=%#v, want task=%#v", packet, want)
				}
				if len(packet.Skills) != 0 || len(packet.Capabilities) != 0 || len(packet.Resources.Servers) != 0 || len(packet.Resources.Browsers) != 0 {
					t.Fatalf("packet invented execution facts: %#v", packet)
				}
				stored, _, err := readPacket(st, admitted.Team.ID, digest)
				if err != nil || !reflect.DeepEqual(stored.Packet, packet) {
					t.Fatalf("stored packet=%#v err=%v", stored, err)
				}
				for _, mutate := range []func(*core.AssignmentPacket){
					func(p *core.AssignmentPacket) { p.Objective += " changed" },
					func(p *core.AssignmentPacket) { p.Checks[0].Command[0] = "changed" },
					func(p *core.AssignmentPacket) { p.Resources.External[0] = "changed" },
				} {
					changed := packet.Clone()
					mutate(&changed)
					if err := knowledge.ValidatePacket(changed, digest); err == nil {
						t.Fatal("changed instructions retained valid digest")
					}
				}
				want.Checks[0].Command[0] = "mutated tracker command"
				want.Checks[0].Name = "mutated tracker check"
				want.Criteria[0] = "mutated tracker criterion"
				want.WritablePaths[0] = "mutated tracker path"
				want.Resources[0] = "mutated tracker resource"
				if err := knowledge.ValidatePacket(packet, digest); err != nil {
					t.Fatalf("packet aliases tracker data: %v", err)
				}
			})
		}
	}
}
