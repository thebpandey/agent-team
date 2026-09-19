package model

import (
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
	for _, advertised := range capabilities.Models {
		if advertised == requested {
			return ModelRoute{Harness: harness, Requested: requested, Resolved: advertised, Reviewer: reviewer}, nil
		}
	}
	return ModelRoute{}, core.ErrCapacity
}
