package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/thebpandey/agent-team/vnext/internal/core"
)

const version = "0.0.0-dev"

func Run(_ context.Context, args []string, deps core.Dependencies) int {
	stdout := deps.Stdout
	if stdout == nil {
		stdout = io.Discard
	}
	if deps.Stderr == nil {
		deps.Stderr = io.Discard
	}
	if len(args) == 0 || args[0] != "version" || (len(args) != 2 && len(args) != 1) {
		return 2
	}
	jsonOutput := len(args) == 2 && args[1] == "--json"
	if len(args) == 2 && !jsonOutput {
		return 2
	}
	if jsonOutput {
		return writeBoundedJSON(stdout, map[string]any{"schema": 1, "version": version})
	}
	if _, err := io.WriteString(stdout, version+"\n"); err != nil {
		return 1
	}
	return 0
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
