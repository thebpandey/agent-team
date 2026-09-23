package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/thebpandey/agent-team/vnext/internal/core"
	"github.com/thebpandey/agent-team/vnext/internal/testkit"
)

func TestNativeLifecycleProjectHoldBlocksStartAndResumePreservesHandle(t *testing.T) {
	root := testkit.GitRepo(t)
	t.Chdir(root)
	if code, result := invokeOnboarding(t, "setup", "--tracker", "tasks-md", "--approve"); code != 0 {
		t.Fatalf("setup: %d %#v", code, result)
	}
	if code, result := invokeOnboarding(t, "settings", "codex.developer.model=inherit"); code != 0 {
		t.Fatalf("settings: %d %#v", code, result)
	}
	fixture := []core.Task{{RecordEnvelope: core.RecordEnvelope{Schema: 1, Project: root, Revision: 1}, ID: "TASK-1", Objective: "approved task", State: core.Ready, Criteria: []string{"works"}, WritablePaths: []string{"src"}, Checks: []core.Check{{Name: "test", Command: []string{"go", "test", "./..."}}}, Dependencies: []core.TaskID{}, Resources: []string{}, EvidencePointers: []string{}}}
	raw, _ := json.Marshal(fixture)
	if err := os.WriteFile(filepath.Join(root, "TASKS.md"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	code, hold := invokeOnboarding(t, "pause")
	if code != 0 || hold["status"] != "admission_held" || hold["host_control_required"] != false {
		t.Fatalf("pause: %d %#v", code, hold)
	}
	if code, result := invokeOnboarding(t, "start", "--host", "codex"); code != 0 || result["status"] != "admission_held" || result["next_action"] != "resume" || result["host_dispatch_required"] != false {
		t.Fatalf("hold needs actionable guidance: %d %#v", code, result)
	}
	controlsPath := filepath.Join(root, ".agent-team", "v8", "controls.json")
	beforeStatus, err := os.ReadFile(controlsPath)
	if err != nil {
		t.Fatal(err)
	}
	if code, result := invokeOnboarding(t, "status"); code != 0 || result["status"] != "admission_held" || result["next_action"] != "resume" || len(result["controls"].([]any)) != 1 {
		t.Fatalf("status ignores hold: %d %#v", code, result)
	}
	afterStatus, err := os.ReadFile(controlsPath)
	if err != nil || string(beforeStatus) != string(afterStatus) {
		t.Fatal("status changed control state")
	}
	if code, result := invokeOnboarding(t, "resume"); code != 0 || result["status"] != "admission_resumed" {
		t.Fatalf("resume: %d %#v", code, result)
	}
	code, started := invokeOnboarding(t, "start", "--host", "codex")
	if code != 0 || started["host_dispatch_required"] != true {
		t.Fatalf("start: %d %#v", code, started)
	}
	packet := started["packet"].(map[string]any)
	fields := []string{"--run", started["run"].(string), "--team", started["team"].(string), "--task", packet["task"].(string), "--host", "codex", "--identity", "exact-worker", "--packet-digest", started["packet_digest"].(string), "--candidate", packet["specRevision"].(string)}
	ackFields := append([]string{}, fields[2:]...)
	if code, result := invokeOnboarding(t, append([]string{"start", "--action", "ack"}, ackFields...)...); code != 0 {
		t.Fatalf("ack: %d %#v", code, result)
	}
	code, stop := invokeOnboarding(t, "stop")
	if code != 0 || stop["status"] != "stop_requested" || stop["host_control_required"] != true {
		t.Fatalf("stop falsely completed: %d %#v", code, stop)
	}
	if code, result := invokeOnboarding(t, "start", "--host", "claude"); code != 0 || result["next_action"] != "observe_control" || result["host_dispatch_required"] != false {
		t.Fatalf("held existing worker needs host observation guidance: %d %#v", code, result)
	}
	if code, result := invokeOnboarding(t, "status"); code != 0 || result["next_action"] != "observe_control" {
		t.Fatalf("status lost pending observation: %d %#v", code, result)
	}
	args := append([]string{"stop", "--action", "ack", "--control-id", stop["control_id"].(string), "--observation", "stopped"}, fields...)
	if code, result := invokeOnboarding(t, args...); code != 0 || result["status"] != "stopped" {
		t.Fatalf("host observation: %d %#v", code, result)
	}
	code, resume := invokeOnboarding(t, "resume")
	if code != 0 || resume["status"] != "resume_requested" || resume["host_control_required"] != true || resume["admission_held"] != false {
		t.Fatalf("resume lacks host action: %d %#v", code, resume)
	}
	handles := resume["handles"].([]any)
	if len(handles) != 1 || handles[0].(map[string]any)["Identity"] != "exact-worker" {
		t.Fatalf("resume lost exact handle: %#v", handles)
	}
	if code, result := invokeOnboarding(t, "status"); code != 0 || result["status"] != "resume_requested" || result["next_action"] != "observe_control" {
		t.Fatalf("status falsely resumed worker: %d %#v", code, result)
	}
	args = append([]string{"resume", "--action", "ack", "--control-id", resume["control_id"].(string), "--observation", "running"}, fields...)
	if code, result := invokeOnboarding(t, args...); code != 0 || result["status"] != "resumed" {
		t.Fatalf("resume observation: %d %#v", code, result)
	}
	if code, result := invokeOnboarding(t, "start", "--host", "claude"); code != 0 || result["already_admitted"] != true || result["host_dispatch_required"] != false {
		t.Fatalf("resume duplicated worker: %d %#v", code, result)
	}
}
