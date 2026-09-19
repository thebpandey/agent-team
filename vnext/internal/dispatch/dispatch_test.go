package dispatch

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/thebpandey/agent-team/vnext/internal/contracts"
	"github.com/thebpandey/agent-team/vnext/internal/core"
)

func TestValidatePacketRejectsMainEscapesAndMalformedIdentity(t *testing.T) {
	root := filepath.Join(t.TempDir(), "task")
	packet := packetForTest()

	for _, test := range []struct {
		name   string
		packet core.AssignmentPacket
		spec   contracts.WorktreeSpec
	}{
		{"main", packet, specForTest(packet, ".", "src")},
		{"empty root", packet, specForTest(packet, "", "src")},
		{"ancestor", packet, specForTest(packet, root, "../outside")},
		{"absolute outside", packet, specForTest(packet, root, t.TempDir())},
		{"run traversal", withRun(packet, "../R"), specForTest(withRun(packet, "../R"), root, "src")},
		{"team separator", withTeam(packet, "A/B"), specForTest(withTeam(packet, "A/B"), root, "src")},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := ValidatePacket(test.packet, test.spec); !errors.Is(err, core.ErrPath) {
				t.Fatalf("ValidatePacket() error = %v, want ErrPath", err)
			}
		})
	}
}

func TestValidatePacketAllowsStrictProspectiveDescendant(t *testing.T) {
	packet := packetForTest()
	root := filepath.Join(t.TempDir(), "future", "worktree")
	if err := ValidatePacket(packet, specForTest(packet, root, filepath.Join(root, "src"))); err != nil {
		t.Fatalf("ValidatePacket() error = %v", err)
	}
	if err := ValidatePacket(packet, specForTest(packet, root, ".")); !errors.Is(err, core.ErrPath) {
		t.Fatalf("root itself was writable: %v", err)
	}
}

func TestValidatePacketBindsPacketWorktreeWhenPresent(t *testing.T) {
	packet := packetForTest()
	root := filepath.Join(t.TempDir(), "task")
	packet.Worktree = root
	if err := ValidatePacket(packet, specForTest(packet, root, "src")); err != nil {
		t.Fatalf("matching packet worktree error = %v", err)
	}
	if err := ValidatePacket(packet, specForTest(packet, filepath.Join(t.TempDir(), "other"), "src")); !errors.Is(err, core.ErrPath) {
		t.Fatalf("mismatched packet worktree error = %v", err)
	}
}

func TestValidatePacketRejectsSymlinkEscape(t *testing.T) {
	packet := packetForTest()
	root, outside := t.TempDir(), t.TempDir()
	link := filepath.Join(root, "escape")
	if err := os.Symlink(outside, link); err != nil {
		if runtime.GOOS == "windows" {
			t.Skip("symlink privilege unavailable")
		}
		t.Fatal(err)
	}
	if err := ValidatePacket(packet, specForTest(packet, root, filepath.Join(link, "owned.go"))); !errors.Is(err, core.ErrPath) {
		t.Fatalf("ValidatePacket() error = %v, want ErrPath", err)
	}
}

func TestDispatcherCopiesInputsAndRejectsForgedWorker(t *testing.T) {
	packet := packetForTest()
	spec := specForTest(packet, filepath.Join(t.TempDir(), "task"), "src")
	adapter := &recordingAdapter{}
	dispatcher := NewDispatcher(adapter)
	got, err := dispatcher.Dispatch(context.Background(), packet, spec)
	if err != nil || got.Identity != "worker" {
		t.Fatalf("Dispatch() = %#v, %v", got, err)
	}
	packet.Scope[0] = "changed"
	spec.WritablePaths[0] = "changed"
	if adapter.request.Packet.Scope[0] != "src" || adapter.request.WritablePaths[0] != "src" || adapter.request.Worktree.WritablePaths[0] != "src" {
		t.Fatalf("adapter observed mutable inputs: %#v", adapter.request)
	}
	adapter.handle = contracts.WorkerHandle{Identity: "forged", Run: "other"}
	if _, err := dispatcher.Dispatch(context.Background(), packetForTest(), specForTest(packetForTest(), filepath.Join(t.TempDir(), "task"), "src")); !errors.Is(err, core.ErrRevision) {
		t.Fatalf("forged handle error = %v, want ErrRevision", err)
	}
}

type recordingAdapter struct {
	request contracts.WorkerRequest
	handle  contracts.WorkerHandle
}

func (a *recordingAdapter) Probe(context.Context) (contracts.HostCapabilities, error) {
	return contracts.HostCapabilities{}, nil
}
func (a *recordingAdapter) StartWorker(_ context.Context, request contracts.WorkerRequest) (contracts.WorkerHandle, error) {
	a.request = request
	if a.handle.Identity != "" {
		return a.handle, nil
	}
	return contracts.WorkerHandle{Identity: "worker", Run: request.Packet.RunID, Team: request.Packet.Team, Task: request.Packet.Task, PacketDigest: request.Packet.QueueFingerprint, CandidateRevision: request.Packet.SpecRevision}, nil
}
func (*recordingAdapter) StartReviewer(context.Context, contracts.WorkerRequest, contracts.WorkerHandle) (contracts.WorkerHandle, error) {
	return contracts.WorkerHandle{}, nil
}
func (*recordingAdapter) Poll(context.Context, contracts.WorkerHandle) (string, error) {
	return "", nil
}
func (*recordingAdapter) Stop(context.Context, contracts.WorkerHandle, core.Scope) error { return nil }
func (*recordingAdapter) ReadIdentity(context.Context, contracts.WorkerHandle) (string, error) {
	return "", nil
}

func packetForTest() core.AssignmentPacket {
	return core.AssignmentPacket{RecordEnvelope: core.RecordEnvelope{RunID: "run-1"}, Team: "team-1", Task: "task-1", Base: "base", QueueFingerprint: "packet", Scope: []string{"src"}}
}

func specForTest(packet core.AssignmentPacket, root, writable string) contracts.WorktreeSpec {
	return contracts.WorktreeSpec{Run: packet.RunID, Team: packet.Team, Root: root, Base: packet.Base, WritablePaths: []string{writable}}
}

func withRun(packet core.AssignmentPacket, run core.RunID) core.AssignmentPacket {
	packet.RunID = run
	return packet
}
func withTeam(packet core.AssignmentPacket, team core.TeamID) core.AssignmentPacket {
	packet.Team = team
	return packet
}
