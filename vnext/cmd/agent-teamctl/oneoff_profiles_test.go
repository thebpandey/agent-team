package main

import (
	"testing"

	"github.com/thebpandey/agent-team/vnext/internal/core"
	"github.com/thebpandey/agent-team/vnext/internal/testkit"
)

func TestOneOffRoleProfilesFollowOriginalHostThroughReplayAndCompletion(t *testing.T) {
	for _, host := range []string{"codex", "claude"} {
		for _, kind := range []string{"feature", "audit", "review"} {
			t.Run(host+"/"+kind, func(t *testing.T) {
				t.Chdir(testkit.GitRepo(t))
				if code, result := invokeOnboarding(t, "setup", "--tracker", "tasks-md", "--approve"); code != 0 {
					t.Fatalf("setup=%+v", result)
				}
				if code, result := invokeOnboarding(t, "settings", "codex.developer.model=codex-developer", "codex.reviewer.model=codex-reviewer", "claude.developer.model=claude-developer", "claude.reviewer.model=claude-reviewer", "codex.developer.effort=low", "claude.developer.effort=low", "codex.reviewer.effort=high", "claude.reviewer.effort=high"); code != 0 {
					t.Fatalf("settings=%+v", result)
				}
				role, effort := "reviewer", "high"
				if kind == "feature" {
					role, effort = "developer", "low"
				}
				assertProfile := func(code int, result map[string]any) {
					t.Helper()
					profile, ok := result["profile"].(map[string]any)
					if code != 0 || !ok || profile["model"] != host+"-"+role || profile["effort"] != effort {
						t.Fatalf("want %s %s profile; code=%d result=%+v", host, role, code, result)
					}
				}
				request := writeTaskRequest(t, core.Task{ID: "profile-task", Objective: "Approved bounded request", Criteria: []string{"Report results"}, Checks: []core.Check{{Name: "test", Command: []string{"go", "test", "./..."}}}, WritablePaths: []string{"src"}})
				code, first := invokeOnboarding(t, "one-off", kind, "--from", request, "--host", host)
				assertProfile(code, first)
				otherHost := "claude"
				if host == otherHost {
					otherHost = "codex"
				}
				code, replay := invokeOnboarding(t, "one-off", kind, "--from", request, "--host", otherHost)
				assertProfile(code, replay)
				if replay["actual_host"] != host || replay["host_dispatch_required"] != false || replay["packet_digest"] != first["packet_digest"] {
					t.Fatalf("replay transferred ownership: %+v", replay)
				}
				packet := first["packet"].(map[string]any)
				fields := []string{"--team", first["team"].(string), "--packet-digest", first["packet_digest"].(string), "--host", host, "--identity", "observed-worker", "--task", "profile-task", "--candidate", packet["specRevision"].(string)}
				for _, action := range []string{"ack", "complete"} {
					code, result := invokeOnboarding(t, append([]string{"start", "--action", action}, fields...)...)
					assertProfile(code, result)
				}
				code, result := invokeOnboarding(t, "start", "--action", "clean", "--team", first["team"].(string), "--reviewer", "independent-reviewer", "--host", otherHost)
				assertProfile(code, result)
				code, result = invokeOnboarding(t, append([]string{"start", "--action", "idle"}, fields...)...)
				assertProfile(code, result)
				code, result = invokeOnboarding(t, "start", "--action", "next", "--team", first["team"].(string), "--host", otherHost)
				assertProfile(code, result)
			})
		}
	}
}
