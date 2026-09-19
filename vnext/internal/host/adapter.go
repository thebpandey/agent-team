package host

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"runtime"
	"strings"

	"github.com/thebpandey/agent-team/vnext/internal/contracts"
	"github.com/thebpandey/agent-team/vnext/internal/core"
	"github.com/thebpandey/agent-team/vnext/internal/tracker"
)

type harnessAdapter struct {
	name       string
	executable string
	runner     CommandRunner
}

var _ contracts.HostAdapter = (*harnessAdapter)(nil)

// NewCodex creates the explicit Codex foreground adapter.
func NewCodex(runner CommandRunner) Adapter {
	return &harnessAdapter{name: "codex", executable: "codex", runner: runner}
}

// NewClaude creates the explicit Claude foreground adapter.
func NewClaude(runner CommandRunner) Adapter {
	return &harnessAdapter{name: "claude", executable: "claude", runner: runner}
}

func (a *harnessAdapter) Probe(ctx context.Context) (Capabilities, error) {
	if a == nil || a.runner == nil || a.executable == "" {
		return Capabilities{}, core.ErrCapacity
	}
	result := a.runner.Run(ctx, a.executable, "--version")
	if unavailable(result) || strings.TrimSpace(string(result.Stdout)) == "" {
		return Capabilities{}, core.ErrCapacity
	}
	return Capabilities{
		Host:            a.name,
		OS:              runtime.GOOS,
		ConfiguredSlots: 1,
		ObservedSlots:   1,
		UsableSlots:     1,
		DeveloperSlots:  1,
		ReviewerSlots:   1,
		Models:          []string{"default"},
	}, nil
}

func (a *harnessAdapter) StartWorker(ctx context.Context, request WorkerRequest) (WorkerHandle, error) {
	return a.start(ctx, request, false)
}

func (a *harnessAdapter) StartReviewer(ctx context.Context, request WorkerRequest, author WorkerHandle) (WorkerHandle, error) {
	if a == nil || a.runner == nil {
		return WorkerHandle{}, core.ErrCapacity
	}
	if !validRequest(request) || author.Identity == "" {
		return WorkerHandle{}, core.ErrPath
	}
	if author.Host == "" || author.Reviewer ||
		author.Run != request.Packet.RunID || author.Team != request.Packet.Team ||
		author.Task != request.Packet.Task || author.PacketDigest != request.Packet.QueueFingerprint ||
		author.CandidateRevision != request.Packet.SpecRevision ||
		author.Identity == a.identity(true, request.Packet) {
		return WorkerHandle{}, core.ErrRevision
	}
	if author.Host == a.name {
		if err := a.validateHandle(author); err != nil {
			return WorkerHandle{}, err
		}
	}
	return a.start(ctx, request, true)
}

func (a *harnessAdapter) start(ctx context.Context, request WorkerRequest, reviewer bool) (WorkerHandle, error) {
	if a == nil || a.runner == nil || a.executable == "" {
		return WorkerHandle{}, core.ErrCapacity
	}
	if !validRequest(request) {
		return WorkerHandle{}, core.ErrPath
	}
	packet, worktree := request.Packet, request.Worktree
	role := "worker"
	if reviewer {
		role = "review"
	}
	result := a.runner.Run(ctx, a.executable,
		role,
		"--run", string(packet.RunID),
		"--team", string(packet.Team),
		"--task", string(packet.Task),
		"--worktree", worktree.Root,
	)
	if unavailable(result) {
		return WorkerHandle{}, core.ErrCapacity
	}
	return WorkerHandle{
		Host:              a.name,
		Identity:          a.identity(reviewer, packet),
		PacketDigest:      packet.QueueFingerprint,
		CandidateRevision: packet.SpecRevision,
		Run:               packet.RunID,
		Team:              packet.Team,
		Task:              packet.Task,
		Reviewer:          reviewer,
	}, nil
}

func (a *harnessAdapter) Poll(ctx context.Context, handle WorkerHandle) (string, error) {
	if err := a.validateHandle(handle); err != nil {
		return "", err
	}
	result := a.runner.Run(ctx, a.executable, "poll", "--identity", handle.Identity)
	if unavailable(result) {
		return "", core.ErrCapacity
	}
	return string(result.Stdout), nil
}

func (a *harnessAdapter) Stop(ctx context.Context, handle WorkerHandle, _ core.Scope) error {
	if err := a.validateHandle(handle); err != nil {
		return err
	}
	if unavailable(a.runner.Run(ctx, a.executable, "stop", "--identity", handle.Identity)) {
		return core.ErrCapacity
	}
	return nil
}

func (a *harnessAdapter) ReadIdentity(_ context.Context, handle WorkerHandle) (string, error) {
	if err := a.validateHandle(handle); err != nil {
		return "", err
	}
	return handle.Identity, nil
}

func (a *harnessAdapter) validateHandle(handle WorkerHandle) error {
	if a == nil || a.runner == nil {
		return core.ErrCapacity
	}
	if handle.Identity == "" {
		return core.ErrPath
	}
	if handle.Host != a.name ||
		handle.Run == "" || handle.Team == "" || handle.Task == "" || handle.PacketDigest == "" ||
		handle.Identity != a.identity(handle.Reviewer, AssignmentPacket{
			RecordEnvelope:   core.RecordEnvelope{RunID: handle.Run},
			Team:             handle.Team,
			Task:             handle.Task,
			SpecRevision:     handle.CandidateRevision,
			QueueFingerprint: handle.PacketDigest,
		}) {
		return core.ErrRevision
	}
	return nil
}

func validRequest(request WorkerRequest) bool {
	packet, worktree := request.Packet, request.Worktree
	return packet.RunID != "" && packet.Team != "" && packet.Task != "" && packet.QueueFingerprint != "" &&
		worktree.Root != "" && worktree.Run == packet.RunID && worktree.Team == packet.Team
}

func (a *harnessAdapter) identity(reviewer bool, packet AssignmentPacket) string {
	role := "worker"
	if reviewer {
		role = "review"
	}
	parts := []string{a.name, role, string(packet.RunID), string(packet.Team), string(packet.Task), packet.SpecRevision, packet.QueueFingerprint}
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return a.name + ":" + role + ":" + hex.EncodeToString(sum[:12])
}

func unavailable(result tracker.CommandResult) bool {
	return result.Transport != nil || result.TimedOut || result.Exit != 0
}
