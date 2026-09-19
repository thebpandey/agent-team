// Package host adapts explicit foreground harnesses to Phase 1 contracts.
package host

import (
	"context"

	"github.com/thebpandey/agent-team/vnext/internal/contracts"
	"github.com/thebpandey/agent-team/vnext/internal/core"
	"github.com/thebpandey/agent-team/vnext/internal/tracker"
)

// These aliases deliberately keep host boundaries on the Phase 1 contracts.
type (
	AssignmentPacket = core.AssignmentPacket
	WorkerRequest    = contracts.WorkerRequest
	WorkerHandle     = contracts.WorkerHandle
	WorktreeSpec     = contracts.WorktreeSpec
	CommandRunner    = tracker.CommandRunner
)

// Capabilities are host discovery data. They are intentionally local because
// Phase 2 discovers a harness before it is admitted to a Phase 1 port.
type Capabilities struct {
	Host            string
	OS              string
	ConfiguredSlots int
	ObservedSlots   int
	UsableSlots     int
	DeveloperSlots  int
	ReviewerSlots   int
	Models          []string
	Unknown         bool
}

// Adapter is a foreground host adapter using Phase 1 request and handle types.
type Adapter interface {
	Probe(context.Context) (Capabilities, error)
	StartWorker(context.Context, WorkerRequest) (WorkerHandle, error)
	StartReviewer(context.Context, WorkerRequest, WorkerHandle) (WorkerHandle, error)
	Poll(context.Context, WorkerHandle) (string, error)
	Stop(context.Context, WorkerHandle, core.Scope) error
	ReadIdentity(context.Context, WorkerHandle) (string, error)
}
