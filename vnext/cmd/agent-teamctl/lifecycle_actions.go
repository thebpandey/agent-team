package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/thebpandey/agent-team/vnext/internal/cli"
	"github.com/thebpandey/agent-team/vnext/internal/contracts"
	"github.com/thebpandey/agent-team/vnext/internal/core"
	"github.com/thebpandey/agent-team/vnext/internal/project"
	"github.com/thebpandey/agent-team/vnext/internal/start"
	"github.com/thebpandey/agent-team/vnext/internal/store"
)

func runLifecycle(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	action, err := cli.Parse(args)
	if err != nil || !action.ScopeRequired {
		return managementError(args, stdout, stderr, core.ErrPhase)
	}
	discovered, err := project.Discover(ctx, ".")
	if err != nil {
		return managementError(args, stdout, stderr, err)
	}
	st := store.New(discovered.TopLevel, core.DefaultConfig().Storage)
	values := map[string]string{}
	for index := 0; index+1 < len(action.Args); index += 2 {
		values[action.Args[index]] = action.Args[index+1]
	}
	var result start.ControlResult
	if values["--action"] != "" {
		if values["--action"] != "ack" || len(values) != 10 {
			return managementError(args, stdout, stderr, core.ErrPhase)
		}
		for _, key := range []string{"--control-id", "--run", "--team", "--task", "--host", "--identity", "--packet-digest", "--candidate", "--observation"} {
			if values[key] == "" {
				return managementError(args, stdout, stderr, core.ErrPhase)
			}
		}
		observation := map[string]string{"pause": "paused", "stop": "stopped", "cancel": "stopped", "resume": "running"}[action.Name]
		if values["--observation"] != observation {
			return managementError(args, stdout, stderr, core.ErrPhase)
		}
		handle := contracts.WorkerHandle{Host: values["--host"], Identity: values["--identity"], Run: core.RunID(values["--run"]), Team: core.TeamID(values["--team"]), Task: core.TaskID(values["--task"]), PacketDigest: values["--packet-digest"], CandidateRevision: values["--candidate"]}
		result, err = start.AcknowledgeControl(ctx, st, values["--control-id"], handle, observation)
	} else {
		scope := core.Scope{Kind: core.ScopeProject, ID: discovered.TopLevel}
		if len(action.Args) != 0 {
			if len(action.Args) != 2 {
				return managementError(args, stdout, stderr, core.ErrPhase)
			}
			scope.ID = action.Args[1]
			switch action.Args[0] {
			case "--project":
				scope.ID, err = project.CanonicalRoot(scope.ID)
			case "--run":
				scope.Kind = core.ScopeRun
			case "--team":
				scope.Kind = core.ScopeTeam
			case "--task":
				scope.Kind = core.ScopeTask
			default:
				err = core.ErrPhase
			}
			if err != nil {
				return managementError(args, stdout, stderr, err)
			}
		}
		result, err = start.RequestControl(ctx, st, action.Name, scope)
	}
	if err != nil {
		return managementError(args, stdout, stderr, err)
	}
	message := "Admission control recorded. Worker state changes only after an exact native host observation."
	if result.Action == "resume" {
		message = "Only this scope's admission hold was cleared. Continue pending handles through the native host using their existing assignment, then record running observations; do not spawn replacements."
	}
	if len(result.UnacknowledgedTeams) > 0 {
		message += " Dispatch intent has no acknowledged handle: reconcile those teams, then repeat this control request."
	}
	if len(result.BlockingScopes) > 0 {
		message += " Other scope holds still apply; blocked handles must remain paused or stopped."
	}
	if !action.JSON {
		if _, err := fmt.Fprintf(stdout, "%s: %s\n%s\n", result.Action, result.Status, message); err != nil {
			return 1
		}
		for _, handle := range result.PendingHandles {
			if _, err := fmt.Fprintf(stdout, "%s %s team=%s task=%s packet=%s candidate=%s\n", handle.Host, handle.Identity, handle.Team, handle.Task, handle.PacketDigest, handle.CandidateRevision); err != nil {
				return 1
			}
		}
		return 0
	}
	raw, _ := json.Marshal(result)
	var output map[string]any
	if err := json.Unmarshal(raw, &output); err != nil {
		return managementError(args, stdout, stderr, err)
	}
	output["ok"], output["message"] = true, message
	return managementResult(args, stdout, output)
}
