package cli

import "github.com/thebpandey/agent-team/vnext/internal/core"

type Request struct {
	Action string
	JSON   bool
}

func Parse(args []string) (Request, error) {
	if len(args) == 1 && args[0] == "version" {
		return Request{Action: "version"}, nil
	}
	if len(args) == 2 && args[0] == "version" && args[1] == "--json" {
		return Request{Action: "version", JSON: true}, nil
	}
	return Request{}, core.ErrPhase
}
