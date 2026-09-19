// Package host adapts explicit foreground harnesses to Phase 1 contracts.
package host

import (
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

// Capabilities is the Phase 1 host capability contract.
type Capabilities = contracts.HostCapabilities

// Adapter is the Phase 1 host adapter contract.
type Adapter = contracts.HostAdapter
