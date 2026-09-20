package model

import (
	"slices"

	"github.com/thebpandey/agent-team/vnext/internal/core"
	"github.com/thebpandey/agent-team/vnext/internal/host"
)

// Harness is an explicit foreground harness label.
type Harness string

const (
	Codex  Harness = "codex"
	Claude Harness = "claude"
)

// ModelRoute records an exact requested-to-advertised model selection.
type ModelRoute struct {
	Harness   Harness
	Requested string
	Resolved  string
	Reviewer  bool
}

// RouteModel accepts only a model explicitly advertised by the selected host.
func RouteModel(capabilities host.Capabilities, harness Harness, requested string, reviewer bool) (ModelRoute, error) {
	if requested == "" || (harness != Codex && harness != Claude) {
		return ModelRoute{}, core.ErrCapacity
	}
	if slices.Contains(capabilities.Models, requested) {
		return ModelRoute{Harness: harness, Requested: requested, Resolved: requested, Reviewer: reviewer}, nil
	}
	return ModelRoute{}, core.ErrCapacity
}
