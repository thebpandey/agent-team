package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/thebpandey/agent-team/vnext/internal/core"
	"github.com/thebpandey/agent-team/vnext/internal/run"
	"github.com/thebpandey/agent-team/vnext/internal/store"
	"github.com/thebpandey/agent-team/vnext/internal/testkit"
)

func writeTaskRequest(t *testing.T, task core.Task) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "task.json")
	raw, err := json.Marshal(task)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestTaskQueueCreatesBlockedDraftWithoutChangingSetup(t *testing.T) {
	root := testkit.GitRepo(t)
	t.Chdir(root)
	if code, result := invokeOnboarding(t, "setup", "--tracker", "tasks-md", "--approve"); code != 0 {
		t.Fatalf("setup=%+v", result)
	}
	configPath := filepath.Join(root, ".agent-team", "config.json")
	before, _ := os.ReadFile(configPath)
	for i := 0; i < 2; i++ {
		code, result := invokeOnboarding(t, "task", "add", "--queue", "Add bounded logging")
		if code != 0 || result["status"] != "needs_input" || result["created"] != true || result["host_dispatch_required"] != false {
			t.Fatalf("draft=%+v code=%d", result, code)
		}
		if task := result["task"].(map[string]any); task["state"] != "blocked" {
			t.Fatalf("draft became executable: %+v", task)
		}
	}
	selected, _, err := projectTracker(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	page, err := selected.Page(context.Background(), "", 1000)
	if err != nil || len(page.Tasks) != 1 || page.Tasks[0].State != core.Blocked {
		t.Fatalf("draft persistence=%+v err=%v", page, err)
	}
	after, _ := os.ReadFile(configPath)
	if !bytes.Equal(before, after) {
		t.Fatal("task creation rewrote immutable setup")
	}
}

func TestTaskFromExecutesOnlyExplicitTaskAndReplays(t *testing.T) {
	root := testkit.GitRepo(t)
	t.Chdir(root)
	if code, result := invokeOnboarding(t, "setup", "--tracker", "tasks-md", "--approve"); code != 0 {
		t.Fatalf("setup=%+v", result)
	}
	if code, result := invokeOnboarding(t, "settings", "claude.developer.model=inherit"); code != 0 {
		t.Fatalf("settings=%+v", result)
	}
	task := core.Task{ID: "a-other", Objective: "Other approved task", Criteria: []string{"passes"}, Checks: []core.Check{{Name: "test", Command: []string{"go", "test", "./..."}}}, WritablePaths: []string{"src"}}
	if code, result := invokeOnboarding(t, "task", "add", "--queue", "--from", writeTaskRequest(t, task)); code != 0 || result["status"] != "queued" || result["host_dispatch_required"] != false {
		t.Fatalf("queue=%+v code=%d", result, code)
	}
	task.ID, task.Objective = "z-chosen", "Explicitly requested task"
	request := writeTaskRequest(t, task)
	code, first := invokeOnboarding(t, "task", "add", "--execute", "--from", request, "--host", "claude")
	if code != 0 || first["host_dispatch_required"] != true || first["already_admitted"] != false {
		t.Fatalf("execute=%+v code=%d", first, code)
	}
	packet := first["packet"].(map[string]any)
	if packet["task"] != "z-chosen" || packet["owner"] != "claude" || packet["objective"] != task.Objective {
		t.Fatalf("wrong task reserved: %+v", packet)
	}
	code, again := invokeOnboarding(t, "task", "add", "--execute", "--from", request, "--host", "codex")
	if code != 0 || again["host_dispatch_required"] != false || again["already_admitted"] != true || again["packet_digest"] != first["packet_digest"] || again["actual_host"] != "claude" {
		t.Fatalf("duplicate execution=%+v code=%d", again, code)
	}
}

func TestObjectiveOnlyOneOffRequestsDetailsWithoutFakeRun(t *testing.T) {
	root := testkit.GitRepo(t)
	t.Chdir(root)
	for _, kind := range []string{"feature", "audit", "review"} {
		code, result := invokeOnboarding(t, "one-off", kind, "Inspect this project")
		if code != 0 || result["status"] != "needs_input" || result["created"] != false || result["host_dispatch_required"] != false {
			t.Fatalf("one-off %s=%+v code=%d", kind, result, code)
		}
	}
	if _, err := os.Stat(filepath.Join(root, ".agent-team")); !os.IsNotExist(err) {
		t.Fatalf("incomplete request created canonical state: %v", err)
	}
}

func TestOneOffAuditUsesImmutableReadOnlyRunWithoutSetup(t *testing.T) {
	root := testkit.GitRepo(t)
	t.Chdir(root)
	task := core.Task{ID: "audit-one", Objective: "Audit requested code", Criteria: []string{"Report defects with evidence"}, WritablePaths: []string{"src"}}
	request := writeTaskRequest(t, task)
	code, result := invokeOnboarding(t, "one-off", "audit", "--from", request, "--host", "claude")
	if code != 0 || result["host_dispatch_required"] != true || result["read_only"] != true {
		t.Fatalf("one-off=%+v code=%d", result, code)
	}
	packet := result["packet"].(map[string]any)
	if packet["scope"] != nil || packet["worktree"] != root {
		t.Fatalf("audit acquired writable scope: %+v", packet)
	}
	manifest, err := run.NewRepositories(store.New(root, core.DefaultConfig().Storage)).Runs.Read(context.Background(), core.RunID(result["run"].(string)))
	if err != nil || manifest.Mode != "one-off" || manifest.TrackerKind != "none" || len(manifest.Tasks[0].WritablePaths) != 0 {
		t.Fatalf("manifest=%+v err=%v", manifest, err)
	}
	for _, path := range []string{".agent-team/config.json", "TASKS.md", ".beads"} {
		if _, err := os.Stat(filepath.Join(root, path)); !os.IsNotExist(err) {
			t.Fatalf("one-off created %s: %v", path, err)
		}
	}
	fields := []string{"--team", result["team"].(string), "--packet-digest", result["packet_digest"].(string), "--host", "claude", "--identity", "actual-agent", "--task", "audit-one", "--candidate", packet["specRevision"].(string)}
	for _, action := range []string{"ack", "complete"} {
		if code, response := invokeOnboarding(t, append([]string{"start", "--action", action}, fields...)...); code != 0 {
			t.Fatalf("one-off %s: %+v", action, response)
		}
	}
	if code, response := invokeOnboarding(t, "start", "--action", "clean", "--team", result["team"].(string), "--reviewer", "independent-reviewer"); code != 0 {
		t.Fatalf("one-off clean: %+v", response)
	}
	if code, response := invokeOnboarding(t, append([]string{"start", "--action", "idle"}, fields...)...); code != 0 {
		t.Fatalf("one-off idle: %+v", response)
	}
	if code, response := invokeOnboarding(t, "start", "--action", "next", "--team", result["team"].(string)); code != 0 || response["host_followup_required"] != false {
		t.Fatalf("one-off terminal next: %+v", response)
	}
	if code, response := invokeOnboarding(t, "one-off", "audit", "--from", request, "--host", "codex"); code != 0 || response["status"] != "completed" || response["host_dispatch_required"] != false || response["observation_required"] != false {
		t.Fatalf("finished one-off was offered again: %+v", response)
	}
}

func TestTaskRequestRejectsUnknownOrMultipleJSONObjects(t *testing.T) {
	for _, raw := range []string{`{"objective":"x","unsafe":true}`, `{"objective":"x"} {"objective":"y"}`} {
		path := filepath.Join(t.TempDir(), "task.json")
		if err := os.WriteFile(path, []byte(raw), 0600); err != nil {
			t.Fatal(err)
		}
		if _, _, _, err := taskRequest([]string{"--from", path}); err == nil {
			t.Fatalf("accepted malformed task input: %s", raw)
		}
	}
}
