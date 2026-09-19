package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/thebpandey/agent-team/vnext/internal/core"
)

const version = "0.0.0-dev"

type outcome struct {
	Schema  int    `json:"schema"`
	Action  string `json:"action"`
	Status  string `json:"status"`
	Message string `json:"message"`
}

func Run(_ context.Context, args []string, deps core.Dependencies) int {
	stdout := deps.Stdout
	if stdout == nil {
		stdout = io.Discard
	}
	if deps.Stderr == nil {
		deps.Stderr = io.Discard
	}
	action, err := Parse(args)
	if err != nil {
		return writeFailure(stdout, deps.Stderr, args, err)
	}
	if action.Name == "version" {
		if action.JSON {
			return writeBoundedJSON(stdout, map[string]any{"schema": 1, "version": version})
		}
		if _, err := io.WriteString(stdout, version+"\n"); err != nil {
			return 1
		}
		return 0
	}

	status := "accepted"
	if deferred(action) {
		status = "deferred"
	}
	if action.Name == "setup" && len(action.Args) == 1 && action.Args[0] == "--refuse-kickoff" {
		status = "rejected"
	}
	message := action.Name + " " + status
	if action.JSON {
		if code := writeBoundedJSON(stdout, outcome{Schema: 1, Action: action.Name, Status: status, Message: message}); code != 0 {
			return code
		}
		return outcomeExit(status)
	}
	if _, err := fmt.Fprintln(stdout, message); err != nil {
		return 1
	}
	return outcomeExit(status)
}

func outcomeExit(status string) int {
	if status == "deferred" || status == "rejected" {
		return phaseExit(core.ErrPhase)
	}
	return 0
}

func phaseExit(err error) int {
	if errors.Is(err, core.ErrPhase) {
		return 2
	}
	return 1
}

func deferred(action Action) bool {
	if action.Name == "start" || action.Name == "cleanup" || action.Name == "deploy" {
		return true
	}
	return action.Name == "task add" && len(action.Args) >= 1 && action.Args[0] == "--execute"
}

func writeFailure(stdout, stderr io.Writer, args []string, err error) int {
	if stdout == nil {
		stdout = io.Discard
	}
	if stderr == nil {
		stderr = io.Discard
	}
	jsonOutput := len(args) > 0 && args[len(args)-1] == "--json"
	actionName := ""
	if len(args) > 0 {
		actionName = args[0]
	}
	message := "phase rejected"
	if err != nil && !strings.Contains(err.Error(), "phase") {
		message = err.Error()
	}
	if jsonOutput {
		if code := writeBoundedJSON(stdout, outcome{Schema: 1, Action: actionName, Status: "rejected", Message: message}); code != 0 {
			return code
		}
		return phaseExit(err)
	}
	if _, writeErr := fmt.Fprintln(stderr, message); writeErr != nil {
		return 1
	}
	return phaseExit(err)
}

func writeBoundedJSON(w io.Writer, value any) int {
	raw, err := json.Marshal(value)
	if err != nil {
		return 1
	}
	if len(raw) > 1<<20 {
		return 1
	}
	if _, err := fmt.Fprintln(w, strings.TrimSpace(string(raw))); err != nil {
		return 1
	}
	return 0
}
