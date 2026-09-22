package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/thebpandey/agent-team/vnext/internal/cli"
	"github.com/thebpandey/agent-team/vnext/internal/core"
	"github.com/thebpandey/agent-team/vnext/internal/preparation"
	"github.com/thebpandey/agent-team/vnext/internal/project"
	"github.com/thebpandey/agent-team/vnext/internal/testkit"
)

func invokeOnboarding(t *testing.T, args ...string) (int, map[string]any) {
	t.Helper()
	var out bytes.Buffer
	code := cli.Run(context.Background(), append(args, "--json"), core.Dependencies{Stdout: &out, Stderr: &out, Management: runManagement})
	var result map[string]any
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatalf("%v: code=%d output=%s", args, code, out.String())
	}
	return code, result
}

func TestFirstUseCLISelectsMarkdownAndClaudeWithoutCutover(t *testing.T) {
	root := testkit.GitRepo(t)
	t.Chdir(root)
	code, result := invokeOnboarding(t, "setup", "--tracker", "tasks-md", "--approve", "--host", "claude")
	if code != 0 || result["next_action"] != "settings" {
		t.Fatalf("setup: %d %+v", code, result)
	}
	if code, result = invokeOnboarding(t, "settings", "claude.developer.model=inherit", "claude.reviewer.model=inherit"); code != 0 {
		t.Fatalf("settings: %+v", result)
	}
	fixture := []core.Task{{RecordEnvelope: core.RecordEnvelope{Schema: 1, Project: root, Revision: 1}, ID: "task-1", Objective: "Implement approved test change", State: core.Ready, Dependencies: []core.TaskID{}, Resources: []string{}, EvidencePointers: []string{}, Criteria: []string{"test passes"}, WritablePaths: []string{"src"}, Checks: []core.Check{{Name: "test", Command: []string{"npm", "test"}}}}}
	raw, _ := json.Marshal(fixture)
	if err := os.WriteFile(filepath.Join(root, "TASKS.md"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	code, result = invokeOnboarding(t, "start", "--host", "claude")
	if code != 0 || result["host_dispatch_required"] != true {
		t.Fatalf("start: %d %+v", code, result)
	}
	packet := result["packet"].(map[string]any)
	if packet["owner"] != "claude" || packet["task"] != "task-1" {
		t.Fatalf("packet=%+v", packet)
	}
	if code, result = invokeOnboarding(t, "setup", "--host", "codex"); code != 0 || result["status"] != "initialized" {
		t.Fatalf("host switch: %d %+v", code, result)
	}
	if code, result = invokeOnboarding(t, "start", "--host", "codex"); code != 0 || result["already_admitted"] != true || result["host_dispatch_required"] != false {
		t.Fatalf("cross-host replay: %d %+v", code, result)
	}
	if _, err := os.Stat(filepath.Join(root, ".agent-team/v8/authority.json")); !os.IsNotExist(err) {
		t.Fatal("ordinary setup created cutover authority")
	}
}

func TestStatusCLIReportsCorruptKickoffBinding(t *testing.T) {
	root := testkit.GitRepo(t)
	t.Chdir(root)
	if code, result := invokeOnboarding(t, "setup", "--tracker", "tasks-md", "--approve"); code != 0 {
		t.Fatalf("setup: %d %+v", code, result)
	}
	if err := os.MkdirAll(filepath.Join(root, ".agent-team/v8"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".agent-team/v8/kickoff.json"), []byte("broken"), 0600); err != nil {
		t.Fatal(err)
	}
	if code, result := invokeOnboarding(t, "status"); code == 0 || result["ok"] != false {
		t.Fatalf("status hid corrupt authority: %d %+v", code, result)
	}
}

func TestFirstUseCLIReportsSetupBeforeStart(t *testing.T) {
	t.Chdir(testkit.GitRepo(t))
	code, result := invokeOnboarding(t, "start", "--host", "codex")
	if code != 0 || result["status"] != "setup_required" || result["next_action"] != "setup" {
		t.Fatalf("start: %d %+v", code, result)
	}
}

func TestStatusCLIDoesNotClaimUninitializedProjectAccepted(t *testing.T) {
	root := testkit.GitRepo(t)
	t.Chdir(root)
	code, result := invokeOnboarding(t, "status")
	if code != 0 || result["status"] != "setup_required" {
		t.Fatalf("status: %d %+v", code, result)
	}
	if _, err := os.Stat(filepath.Join(root, ".agent-team")); !os.IsNotExist(err) {
		t.Fatal("status wrote project state")
	}
}

func TestSetupRefusalPreservesReadyExistingInputs(t *testing.T) {
	root := testkit.GitRepo(t)
	t.Chdir(root)
	for _, name := range []string{"TASKS.md", "DECISIONS.md", "AGENT_TEAM_RULES.md"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("# Existing user file\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	code, result := invokeOnboarding(t, "setup", "--refuse-kickoff")
	if code != 0 || result["status"] != "cancelled" {
		t.Fatalf("refusal: %d %+v", code, result)
	}
	if _, err := os.Stat(filepath.Join(root, ".agent-team")); !os.IsNotExist(err) {
		t.Fatal("refusal initialized project")
	}
}

func TestSetupCLILargeEmbeddedDoltDoesNotConsumeArtifactBudget(t *testing.T) {
	root := testkit.GitRepo(t)
	t.Chdir(root)
	dbPath := filepath.Join(root, ".beads/embeddeddolt/project/.dolt/noms")
	if err := os.MkdirAll(dbPath, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".beads/metadata.json"), []byte(`{"backend":"dolt","dolt_mode":"embedded","dolt_database":"project"}`), 0600); err != nil {
		t.Fatal(err)
	}
	db, err := os.Create(filepath.Join(dbPath, "oversized-dolt-table"))
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Truncate(64 << 20); err != nil {
		db.Close()
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "TASKS.md"), []byte("# Retained legacy tracker\n"), 0600); err != nil {
		t.Fatal(err)
	}
	code, result := invokeOnboarding(t, "setup", "--approve-kickoff")
	if code != 0 || result["ok"] != true {
		t.Fatalf("large Beads setup: %d %+v", code, result)
	}
	if selected, ok := result["tracker"].(map[string]any); !ok || selected["kind"] != "beads" {
		t.Fatalf("wrong tracker: %+v", result)
	}
	setup, err := project.InspectSetup(context.Background(), root)
	if err != nil || setup.ArtifactDigests[".beads"] == "" {
		t.Fatalf("missing stable Beads receipt digest: %+v %v", setup, err)
	}
	if body, err := os.ReadFile(filepath.Join(root, "TASKS.md")); err != nil || string(body) != "# Retained legacy tracker\n" {
		t.Fatal("setup changed legacy TASKS.md")
	}
}

func TestKickoffCLIStartsDesignatedMarkdownFromSubdirectory(t *testing.T) {
	root := testkit.GitRepo(t)
	t.Chdir(root)
	p, err := project.Discover(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	branchBytes, err := exec.Command("git", "symbolic-ref", "--short", "HEAD").Output()
	if err != nil {
		t.Fatal(err)
	}
	branch := strings.TrimSpace(string(branchBytes))
	if err := os.MkdirAll(filepath.Join(root, ".agent-team"), 0700); err != nil {
		t.Fatal(err)
	}
	tasks := "## Active tasks\n| ID | Intended outcome / acceptance pointer | Status | Depends on |\n| --- | --- | --- | --- |\n| AT-001 | Unrelated work | ready | None |\n| AT-002 | Approved feature | ready | None |\n"
	if err := os.WriteFile(filepath.Join(root, ".agent-team/TASKS.md"), []byte(tasks), 0600); err != nil {
		t.Fatal(err)
	}
	handoff := map[string]any{
		"schemaVersion": 1, "kind": "project-kickoff-agent-team-handoff", "status": "approved",
		"projectKickoff": map[string]any{"version": "0.5.0", "approvalId": "APR-001", "approvedRevision": p.Head},
		"agentTeam":      map[string]any{"testedVersion": "8.0.10", "initializationSource": "existing"},
		"project":        map[string]any{"id": "demo", "root": root, "branch": branch, "revision": p.Head},
		"tracker":        map[string]any{"kind": "markdown", "path": ".agent-team/TASKS.md"},
		"plan":           map[string]any{"scope": "Approved feature", "branch": branch, "acceptance": []string{"tests pass"}, "verification": []string{"npm test"}, "authority": map[string]any{"ownedPaths": []string{"src"}, "externalActions": []string{}}, "tasks": []map[string]string{{"id": "AT-002"}}},
	}
	raw, _ := json.Marshal(handoff)
	if err := os.WriteFile(filepath.Join(root, "kickoff.json"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	if code, result := invokeOnboarding(t, "setup", "--kickoff", "kickoff.json", "--approve-kickoff"); code != 0 || result["next_action"] != "settings" {
		t.Fatalf("setup: %d %+v", code, result)
	}
	if err := os.MkdirAll(filepath.Join(root, "src"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Chdir(filepath.Join(root, "src"))
	if code, result := invokeOnboarding(t, "settings", "codex.developer.model=inherit"); code != 0 {
		t.Fatalf("settings: %d %+v", code, result)
	}
	code, result := invokeOnboarding(t, "start", "--host", "codex")
	if code != 0 || result["host_dispatch_required"] != true {
		t.Fatalf("start: %d %+v", code, result)
	}
	packet := result["packet"].(map[string]any)
	if packet["task"] != "AT-002" || packet["objective"] != "Approved feature" {
		t.Fatalf("wrong scope: %+v", packet)
	}
	if checks, ok := packet["checks"].([]any); !ok || len(checks) != 1 {
		t.Fatalf("missing checks: %+v", packet)
	}
}

func TestPrepareOnlyDoesNotBindUnfinishedKickoff(t *testing.T) {
	root := testkit.GitRepo(t)
	t.Chdir(root)
	oldInstall, oldInitialize := installDependencies, initializeDependencies
	t.Cleanup(func() { installDependencies, initializeDependencies = oldInstall, oldInitialize })
	installed, initialized := false, false
	installDependencies = func(_ context.Context, gotRoot string, names []string, approved bool) ([]preparation.Dependency, error) {
		if gotRoot != root || len(names) != 1 || names[0] != "graphify" || !approved {
			t.Fatal("unexpected install request")
		}
		installed = true
		return []preparation.Dependency{{Name: "graphify", Available: true}}, nil
	}
	initializeDependencies = func(_ context.Context, _ string, _ []string, approved bool) ([]preparation.Dependency, error) {
		if !approved {
			t.Fatal("missing approval")
		}
		initialized = true
		return []preparation.Dependency{{Name: "graphify", Available: true, Prepared: true}}, nil
	}
	code, result := invokeOnboarding(t, "setup", "--prepare-only", "--install", "graphify", "--approve")
	if code != 0 || result["status"] != "prepared" || !installed || !initialized {
		t.Fatalf("prepare: %d %+v", code, result)
	}
	for _, name := range []string{"TASKS.md", "DECISIONS.md", "AGENT_TEAM_RULES.md", ".agent-team/config.json"} {
		if _, err := os.Stat(filepath.Join(root, name)); !os.IsNotExist(err) {
			t.Fatalf("premature setup write: %s", name)
		}
	}
}

func TestEmptyTrackerDirectsPlanningAndInvalidTrackerDirectsRepair(t *testing.T) {
	root := testkit.GitRepo(t)
	t.Chdir(root)
	if code, r := invokeOnboarding(t, "setup", "--tracker", "tasks-md", "--approve"); code != 0 {
		t.Fatalf("setup: %d %+v", code, r)
	}
	if code, r := invokeOnboarding(t, "settings", "codex.developer.model=inherit"); code != 0 {
		t.Fatalf("settings: %d %+v", code, r)
	}
	for _, action := range []string{"status", "setup", "start"} {
		code, r := invokeOnboarding(t, action)
		if code != 0 || r["next_action"] != "project_kickoff" || r["status"] != "no_ready_tasks" {
			t.Fatalf("empty %s: %d %+v", action, code, r)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "TASKS.md"), []byte("not a valid tracker"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, action := range []string{"status", "setup", "start"} {
		code, r := invokeOnboarding(t, action)
		if code != 0 || r["next_action"] != "repair_tracker" {
			t.Fatalf("invalid %s: %d %+v", action, code, r)
		}
	}
}

func TestRequestedDependencyFailureDoesNotAdvanceSetup(t *testing.T) {
	root := testkit.GitRepo(t)
	t.Chdir(root)
	old := installDependencies
	t.Cleanup(func() { installDependencies = old })
	installDependencies = func(context.Context, string, []string, bool) ([]preparation.Dependency, error) {
		return []preparation.Dependency{{Name: "graphify", Status: "failed", Error: "download failed"}}, nil
	}
	code, r := invokeOnboarding(t, "setup", "--install", "graphify", "--tracker", "tasks-md", "--approve")
	if code != 0 || r["next_action"] != "resolve_dependencies" {
		t.Fatalf("failed install: %d %+v", code, r)
	}
	if _, err := os.Stat(filepath.Join(root, ".agent-team/config.json")); !os.IsNotExist(err) {
		t.Fatal("failed preparation advanced setup")
	}
}

func TestDeferredPreparationPermitsPlanningWithoutClaimingPrepared(t *testing.T) {
	root := testkit.GitRepo(t)
	t.Chdir(root)
	oldInstall, oldInitialize := installDependencies, initializeDependencies
	t.Cleanup(func() { installDependencies, initializeDependencies = oldInstall, oldInitialize })
	installDependencies = func(context.Context, string, []string, bool) ([]preparation.Dependency, error) {
		return []preparation.Dependency{{Name: "serena", Available: true}}, nil
	}
	initializeDependencies = func(context.Context, string, []string, bool) ([]preparation.Dependency, error) {
		return []preparation.Dependency{{Name: "serena", Available: true, Status: "deferred"}}, nil
	}
	code, r := invokeOnboarding(t, "setup", "--prepare-only", "--install", "serena", "--approve")
	if code != 0 || r["status"] != "deferred" || r["next_action"] != "project_kickoff" {
		t.Fatalf("deferred: %d %+v", code, r)
	}
}
