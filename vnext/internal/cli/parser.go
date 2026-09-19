package cli

import (
	"strings"

	"github.com/thebpandey/agent-team/vnext/internal/core"
)

// Action is the deliberately small, argument-only command representation used
// by the native CLI boundary. Args never contain the command name or --json.
type Action struct {
	Name string
	Args []string
	JSON bool
}

// Request is retained as a source-compatible name for the original version
// parser. New callers should use Action.
type Request = Action

// Parse accepts only the canonical public action vocabulary. It performs no
// filesystem, host, process, tracker, or credential work.
func Parse(args []string) (Action, error) {
	if len(args) == 0 {
		return Action{}, core.ErrPhase
	}
	args = append([]string(nil), args...)

	jsonOutput := false
	if args[len(args)-1] == "--json" {
		jsonOutput = true
		args = args[:len(args)-1]
	}
	for _, arg := range args {
		if arg == "--json" {
			return Action{}, core.ErrPhase
		}
	}
	if len(args) == 0 {
		return Action{}, core.ErrPhase
	}

	name := args[0]
	var actionArgs []string
	switch name {
	case "version":
		if len(args) != 1 {
			return Action{}, core.ErrPhase
		}
	case "setup":
		if err := parseSetupArgs(args[1:]); err != nil {
			return Action{}, err
		}
		actionArgs = args[1:]
	case "settings", "inspect", "cleanup":
		if len(args) != 1 {
			return Action{}, core.ErrPhase
		}
	case "status":
		if err := parseSingleSelector(args[1:], "--run"); err != nil {
			return Action{}, err
		}
		actionArgs = args[1:]
	case "start":
		if err := parseSingleSelector(args[1:], "--task"); err != nil {
			return Action{}, err
		}
		actionArgs = args[1:]
	case "task":
		if len(args) < 3 || args[1] != "add" || (args[2] != "--queue" && args[2] != "--execute") {
			return Action{}, core.ErrPhase
		}
		if len(args) > 3 {
			return Action{}, core.ErrPhase
		}
		name = "task add"
		actionArgs = args[2:]
	case "one-off":
		if len(args) != 4 || (args[1] != "feature" && args[1] != "audit" && args[1] != "review") || args[2] != "--objective" || strings.TrimSpace(args[3]) == "" {
			return Action{}, core.ErrPhase
		}
		name = "one-off " + args[1]
		actionArgs = args[2:]
	case "pause":
		if err := parseScope(args[1:]); err != nil {
			return Action{}, err
		}
		actionArgs = args[1:]
	case "deploy":
		if err := parseDeployArgs(args[1:]); err != nil {
			return Action{}, err
		}
		actionArgs = args[1:]
	default:
		return Action{}, core.ErrPhase
	}

	return Action{Name: name, Args: append([]string(nil), actionArgs...), JSON: jsonOutput}, nil
}

func parseSetupArgs(args []string) error {
	if len(args) > 1 {
		return core.ErrPhase
	}
	if len(args) == 1 && args[0] != "--approve-kickoff" && args[0] != "--refuse-kickoff" {
		return core.ErrPhase
	}
	return nil
}

func parseSingleSelector(args []string, flag string) error {
	if len(args) == 0 {
		return nil
	}
	if len(args) != 2 || args[0] != flag || strings.TrimSpace(args[1]) == "" {
		return core.ErrPhase
	}
	return nil
}

func parseScope(args []string) error {
	if len(args) != 2 || args[0] != "--scope" {
		return core.ErrPhase
	}
	scope := strings.TrimSpace(args[1])
	kind, id, ok := strings.Cut(scope, ":")
	if !ok || id == "" || (kind != "project" && kind != "run" && kind != "team" && kind != "task") {
		return core.ErrPhase
	}
	return nil
}

func parseDeployArgs(args []string) error {
	seen := map[string]bool{}
	for i := 0; i < len(args); i += 2 {
		if i+1 >= len(args) || seen[args[i]] || strings.TrimSpace(args[i+1]) == "" {
			return core.ErrPhase
		}
		seen[args[i]] = true
		switch args[i] {
		case "--run", "--target":
		case "--batch-size":
			for _, char := range args[i+1] {
				if char < '0' || char > '9' {
					return core.ErrPhase
				}
			}
			if args[i+1] == "0" {
				return core.ErrPhase
			}
		default:
			return core.ErrPhase
		}
	}
	return nil
}
