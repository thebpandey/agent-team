package cli

import (
	"path/filepath"
	"slices"
	"strings"

	"github.com/thebpandey/agent-team/vnext/internal/core"
	"github.com/thebpandey/agent-team/vnext/internal/deploy"
)

// Action is the deliberately small, argument-only command representation used
// by the native CLI boundary. Args never contain the command name or --json.
type Action struct {
	Name          string
	Args          []string
	JSON          bool
	ScopeRequired bool
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
	jsonOutput := false
	if args[len(args)-1] == "--json" {
		jsonOutput = true
		args = args[:len(args)-1]
	}
	if slices.Contains(args, "--json") {
		return Action{}, core.ErrPhase
	}
	if len(args) == 0 {
		return Action{}, core.ErrPhase
	}

	name := args[0]
	var (
		actionArgs []string
		err        error
	)
	switch name {
	case "version":
		if len(args) != 1 {
			return Action{}, core.ErrPhase
		}
	case "setup":
		actionArgs, err = parseSetupArgs(args[1:])
	case "settings":
		actionArgs, err = parseSettingsArgs(args[1:])
	case "status":
		actionArgs, err = parseSelectors(args[1:], map[string]bool{"--run": true}, false)
	case "start":
		actionArgs, err = parseSelectors(args[1:], map[string]bool{"--run": true, "--task": true}, true)
	case "task":
		if len(args) < 3 || args[1] != "add" || (args[2] != "--queue" && args[2] != "--execute") {
			return Action{}, core.ErrPhase
		}
		name = "task add"
		actionArgs = []string{args[2]}
		if len(args) > 3 {
			objective := strings.TrimSpace(strings.Join(args[3:], " "))
			if objective == "" {
				return Action{}, core.ErrPhase
			}
			actionArgs = append(actionArgs, objective)
		}
	case "one-off":
		if len(args) < 3 || (args[1] != "feature" && args[1] != "audit" && args[1] != "review") {
			return Action{}, core.ErrPhase
		}
		objectiveArgs := args[2:]
		if args[2] == "--objective" {
			if len(args) < 4 {
				return Action{}, core.ErrPhase
			}
			objectiveArgs = args[3:]
		}
		objective := strings.TrimSpace(strings.Join(objectiveArgs, " "))
		if objective == "" {
			return Action{}, core.ErrPhase
		}
		name = "one-off " + args[1]
		actionArgs = []string{objective}
	case "pause", "stop", "cancel", "resume":
		if name == "pause" && len(args) == 3 && args[1] == "--scope" {
			actionArgs, err = parseScope(args[2])
		} else {
			allowed := map[string]bool{"--run": true, "--team": true, "--task": true}
			if name == "pause" || name == "stop" {
				allowed["--project"] = true
			}
			actionArgs, err = parseSelectors(args[1:], allowed, false)
		}
	case "cleanup":
		if len(args) > 1 && args[1] == "--mutation-lock" {
			actionArgs, err = parseMutationCleanup(args[1:])
		} else {
			actionArgs, err = parseSelectors(args[1:], map[string]bool{"--run": true, "--team": true, "--task": true}, false)
		}
	case "inspect":
		actionArgs, err = parseSelectors(args[1:], map[string]bool{"--run": true, "--team": true, "--task": true}, false)
	case "deploy":
		actionArgs, err = parseDeployArgs(args[1:])
	case "install":
		actionArgs, err = parseExactPair(args[1:], "--host", map[string]bool{"codex": true, "claude": true, "both": true})
	case "update":
		actionArgs, err = parseExactPair(args[1:], "--version", nil)
	case "rollback":
		actionArgs, err = parseRollbackArgs(args[1:])
	case "uninstall":
		if len(args) != 1 {
			return Action{}, core.ErrPhase
		}
	case "cutover":
		actionArgs, err = parseAbsoluteRequest(args[1:])
	default:
		return Action{}, core.ErrPhase
	}
	if err != nil {
		return Action{}, err
	}

	return Action{Name: name, Args: actionArgs, JSON: jsonOutput, ScopeRequired: name == "pause" || name == "stop" || name == "cancel" || name == "resume"}, nil
}

func parseAbsoluteRequest(args []string) ([]string, error) {
	if len(args) != 2 || args[0] != "--request" || !filepath.IsAbs(args[1]) || filepath.Clean(args[1]) != args[1] {
		return nil, core.ErrPhase
	}
	return append([]string(nil), args...), nil
}

func parseRollbackArgs(args []string) ([]string, error) {
	if len(args) != 2 && len(args) != 4 || len(args) < 2 || args[0] != "--version" || strings.TrimSpace(args[1]) == "" {
		return nil, core.ErrPhase
	}
	if len(args) == 4 && (args[2] != "--revision" || !validRevision(args[3])) {
		return nil, core.ErrPhase
	}
	return append([]string(nil), args...), nil
}

func validRevision(value string) bool {
	if len(value) < 40 || len(value) > 64 {
		return false
	}
	for _, character := range value {
		if !strings.ContainsRune("0123456789abcdefABCDEF", character) {
			return false
		}
	}
	return true
}

func parseMutationCleanup(args []string) ([]string, error) {
	if len(args) != 7 || args[0] != "--mutation-lock" || (args[1] != "primary" && args[1] != "recovery") || args[2] != "--owner-token" || len(args[3]) != 32 || args[4] != "--operation" || strings.TrimSpace(args[5]) == "" || args[5] != strings.TrimSpace(args[5]) || args[6] != "--confirm-dead" {
		return nil, core.ErrPhase
	}
	for _, character := range args[3] {
		if !strings.ContainsRune("0123456789abcdef", character) {
			return nil, core.ErrPhase
		}
	}
	return append([]string(nil), args...), nil
}

func parseExactPair(args []string, flag string, allowed map[string]bool) ([]string, error) {
	if len(args) != 2 || args[0] != flag {
		return nil, core.ErrPhase
	}
	value := strings.TrimSpace(args[1])
	if value == "" || (allowed != nil && !allowed[value]) {
		return nil, core.ErrPhase
	}
	return []string{flag, value}, nil
}

func parseSetupArgs(args []string) ([]string, error) {
	var out []string
	seenMode, seenApprove, seenRefuse := false, false, false
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--mode":
			if seenMode || i+1 >= len(args) {
				return nil, core.ErrPhase
			}
			mode := strings.TrimSpace(args[i+1])
			if mode != "plan" && mode != "one-off" {
				return nil, core.ErrPhase
			}
			seenMode = true
			out = append(out, "--mode", mode)
			i++
		case "--approve-kickoff":
			if seenApprove || seenRefuse {
				return nil, core.ErrPhase
			}
			seenApprove = true
			out = append(out, args[i])
		case "--refuse-kickoff":
			if seenRefuse || seenApprove {
				return nil, core.ErrPhase
			}
			seenRefuse = true
			out = append(out, args[i])
		default:
			return nil, core.ErrPhase
		}
	}
	return out, nil
}

func parseSettingsArgs(args []string) ([]string, error) {
	out := make([]string, 0, len(args))
	seen := make(map[string]bool, len(args))
	for _, arg := range args {
		key, value, ok := strings.Cut(arg, "=")
		key, value = strings.TrimSpace(key), strings.TrimSpace(value)
		if !ok || key == "" || value == "" || strings.HasPrefix(key, "-") || seen[key] {
			return nil, core.ErrPhase
		}
		seen[key] = true
		out = append(out, key+"="+value)
	}
	return out, nil
}

func parseSelectors(args []string, allowed map[string]bool, repeatTask bool) ([]string, error) {
	if len(args)%2 != 0 {
		return nil, core.ErrPhase
	}
	seen := make(map[string]bool)
	out := make([]string, 0, len(args))
	for i := 0; i < len(args); i += 2 {
		flag, value := args[i], strings.TrimSpace(args[i+1])
		if !allowed[flag] || value == "" || (seen[flag] && !(repeatTask && flag == "--task")) {
			return nil, core.ErrPhase
		}
		seen[flag] = true
		out = append(out, flag, value)
	}
	return out, nil
}

func parseScope(value string) ([]string, error) {
	kind, id, ok := strings.Cut(strings.TrimSpace(value), ":")
	id = strings.TrimSpace(id)
	if !ok || id == "" || (kind != "project" && kind != "run" && kind != "team" && kind != "task") {
		return nil, core.ErrPhase
	}
	return []string{"--" + kind, id}, nil
}

func parseDeployArgs(args []string) ([]string, error) {
	normalized := append([]string(nil), args...)
	for i := 0; i < len(normalized); i++ {
		if normalized[i] != "--resume" {
			if i+1 >= len(normalized) {
				return nil, core.ErrPhase
			}
			normalized[i+1] = strings.TrimSpace(normalized[i+1])
			i++
		}
	}
	if _, err := deploy.ParseDeployArgs(append([]string{"deploy"}, normalized...)); err != nil {
		return nil, core.ErrPhase
	}
	return normalized, nil
}
