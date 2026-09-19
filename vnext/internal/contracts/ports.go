package contracts

import (
	"context"

	"github.com/thebpandey/agent-team/vnext/internal/core"
)

type HostAdapter interface {
	Probe(context.Context) (HostCapabilities, error)
	StartWorker(context.Context, WorkerRequest) (WorkerHandle, error)
	StartReviewer(context.Context, WorkerRequest, WorkerHandle) (WorkerHandle, error)
	Poll(context.Context, WorkerHandle) (string, error)
	Stop(context.Context, WorkerHandle, core.Scope) error
	ReadIdentity(context.Context, WorkerHandle) (string, error)
}

type WorktreeManager interface {
	Create(context.Context, WorktreeSpec) (Worktree, error)
	Inspect(context.Context, Worktree) (Worktree, error)
	Integrate(context.Context, Candidate) (Candidate, error)
	Cleanup(context.Context, core.TeamID) error
}

type CompletionGate interface {
	Check(context.Context, GateInput) (GateResult, error)
}

type DeploymentExecutor interface {
	Submit(context.Context, DeploymentBatch) (Operation, error)
	Query(context.Context, Operation) (Operation, error)
	Verify(context.Context, Operation) (Verification, error)
}
