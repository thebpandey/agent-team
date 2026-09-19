package host

import (
	"context"
	"runtime"
	"strings"

	"github.com/thebpandey/agent-team/vnext/internal/core"
	"github.com/thebpandey/agent-team/vnext/internal/tracker"
)

type harnessAdapter struct {
	name       string
	executable string
	runner     CommandRunner
}

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
	if author.Identity == "" {
		return WorkerHandle{}, core.ErrPath
	}
	if author.Host != a.name || author.Reviewer || author.Identity != a.identity(false) ||
		author.Run != request.Packet.RunID || author.Team != request.Packet.Team ||
		author.Task != request.Packet.Task || author.PacketDigest != request.Packet.QueueFingerprint {
		return WorkerHandle{}, core.ErrRevision
	}
	return a.start(ctx, request, true)
}

func (a *harnessAdapter) start(ctx context.Context, request WorkerRequest, reviewer bool) (WorkerHandle, error) {
	if a == nil || a.runner == nil || a.executable == "" {
		return WorkerHandle{}, core.ErrCapacity
	}
	packet, worktree := request.Packet, request.Worktree
	if packet.RunID == "" || packet.Team == "" || packet.Task == "" || packet.QueueFingerprint == "" ||
		worktree.Root == "" || worktree.Run != packet.RunID || worktree.Team != packet.Team {
		return WorkerHandle{}, core.ErrPath
	}
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
		Identity:          a.identity(reviewer),
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
		(handle.Reviewer && handle.Identity != a.identity(true)) ||
		(!handle.Reviewer && handle.Identity != a.identity(false)) {
		return core.ErrRevision
	}
	return nil
}

func (a *harnessAdapter) identity(reviewer bool) string {
	if reviewer {
		return a.name + ":review"
	}
	return a.name + ":worker"
}

func unavailable(result tracker.CommandResult) bool {
	return result.Transport != nil || result.TimedOut || result.Exit != 0
}
