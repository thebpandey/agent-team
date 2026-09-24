package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
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

func TestStatusCLIResolvesUnsupportedAutoDiscoveredKickoffReadOnly(t *testing.T) {
	root := testkit.GitRepo(t)
	handoffPath := filepath.Join(root, ".project-kickoff", "AGENT_TEAM_HANDOFF.json")
	if err := os.MkdirAll(filepath.Dir(handoffPath), 0700); err != nil {
		t.Fatal(err)
	}
	body := `{"schemaVersion":1,"kind":"project-kickoff-agent-team-handoff","status":"approved","projectKickoff":{"version":"0.4.0","approvalId":"APR-old","approvedRevision":"1111111111111111111111111111111111111111"},"project":{"revision":"2222222222222222222222222222222222222222"}}`
	if err := os.WriteFile(handoffPath, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	before := testkit.SnapshotTree(t, root)
	t.Chdir(root)

	code, result := invokeOnboarding(t, "status")
	if code != 0 || result["ok"] != true || result["status"] != "needs_input" || result["next_action"] != "resolve_kickoff" {
		t.Fatalf("status: %d %+v", code, result)
	}
	facts, ok := result["kickoff_resolution"].(map[string]any)
	if !ok {
		t.Fatalf("missing kickoff resolution facts: %+v", result)
	}
	if facts["path"] != ".project-kickoff/AGENT_TEAM_HANDOFF.json" || facts["detected_version"] != "0.4.0" || facts["handoff_revision"] != "1111111111111111111111111111111111111111" || facts["project_revision"] != "2222222222222222222222222222222222222222" {
		t.Fatalf("kickoff facts: %+v", facts)
	}
	accepted, ok := facts["accepted_versions"].([]any)
	if !ok || len(accepted) != 2 || accepted[0] != "0.5.0" || accepted[1] != "0.5.1" || facts["head"] == "" {
		t.Fatalf("kickoff compatibility facts: %+v", facts)
	}
	reason, ok := facts["reason"].(string)
	if !ok || !strings.Contains(reason, "unsupported") || len(reason) > 512 {
		t.Fatalf("bounded reason: %#v", facts["reason"])
	}
	if after := testkit.SnapshotTree(t, root); !reflect.DeepEqual(before, after) {
		t.Fatalf("status wrote project state: before=%v after=%v", before, after)
	}
}

func TestStatusCLIResolvesStaleAutoDiscoveredKickoff(t *testing.T) {
	root := testkit.GitRepo(t)
	p, err := project.Discover(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	branchBytes, err := exec.Command("git", "-C", root, "symbolic-ref", "--short", "HEAD").Output()
	if err != nil {
		t.Fatal(err)
	}
	branch := strings.TrimSpace(string(branchBytes))
	stale := strings.Repeat("2", 40)
	handoff := map[string]any{
		"schemaVersion": 1, "kind": "project-kickoff-agent-team-handoff", "status": "approved",
		"projectKickoff": map[string]any{"version": "0.5.1", "approvalId": "APR-stale", "approvedRevision": p.Head},
		"agentTeam":      map[string]any{"initializationSource": "existing"},
		"project":        map[string]any{"id": "project-1", "root": root, "branch": branch, "revision": stale},
		"tracker":        map[string]any{"kind": "markdown", "path": "TASKS.md"},
		"plan":           map[string]any{"scope": "stale handoff", "branch": branch},
	}
	body, _ := json.Marshal(handoff)
	handoffPath := filepath.Join(root, ".project-kickoff", "AGENT_TEAM_HANDOFF.json")
	if err := os.MkdirAll(filepath.Dir(handoffPath), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(handoffPath, body, 0600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(root)

	code, result := invokeOnboarding(t, "status")
	facts, _ := result["kickoff_resolution"].(map[string]any)
	if code != 0 || result["status"] != "needs_input" || result["next_action"] != "resolve_kickoff" || facts["detected_version"] != "0.5.1" || facts["project_revision"] != stale || facts["head"] != p.Head || !strings.Contains(facts["reason"].(string), "revision mismatch") {
		t.Fatalf("stale status: %d result=%+v facts=%+v", code, result, facts)
	}
}

func TestSetupCLIIgnoresAutoDiscoveredKickoffAndBindsReceipt(t *testing.T) {
	root := testkit.GitRepo(t)
	handoffPath := filepath.Join(root, ".project-kickoff", "AGENT_TEAM_HANDOFF.json")
	if err := os.MkdirAll(filepath.Dir(handoffPath), 0700); err != nil {
		t.Fatal(err)
	}
	body := []byte(`{"schemaVersion":1,"kind":"project-kickoff-agent-team-handoff","status":"approved","projectKickoff":{"version":"0.4.0","approvalId":"APR-old","approvedRevision":"1111111111111111111111111111111111111111"},"project":{"revision":"2222222222222222222222222222222222222222"}}`)
	if err := os.WriteFile(handoffPath, body, 0600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(root)

	code, result := invokeOnboarding(t, "setup", "--ignore-kickoff", "--tracker", "tasks-md", "--approve")
	if code != 0 || result["ok"] != true || result["status"] != "initialized" {
		t.Fatalf("setup: %d %+v", code, result)
	}
	if got, err := os.ReadFile(handoffPath); err != nil || !bytes.Equal(got, body) {
		t.Fatalf("ignored handoff changed: %q err=%v", got, err)
	}
	configBytes, err := os.ReadFile(filepath.Join(root, ".agent-team", "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	var config struct {
		ReceiptPath string `json:"receiptPath"`
	}
	if err := json.Unmarshal(configBytes, &config); err != nil || config.ReceiptPath == "" {
		t.Fatalf("config: %s err=%v", configBytes, err)
	}
	receiptBytes, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(config.ReceiptPath)))
	if err != nil {
		t.Fatal(err)
	}
	var receipt struct {
		Ignored struct {
			Path            string `json:"path"`
			Digest          string `json:"digest"`
			DetectedVersion string `json:"detectedVersion"`
			Decision        string `json:"decision"`
			Reason          string `json:"reason"`
		} `json:"ignoredKickoff"`
	}
	if err := json.Unmarshal(receiptBytes, &receipt); err != nil {
		t.Fatal(err)
	}
	wantDigest := fmt.Sprintf("sha256:%x", sha256.Sum256(body))
	if receipt.Ignored.Path != ".project-kickoff/AGENT_TEAM_HANDOFF.json" || receipt.Ignored.Digest != wantDigest || receipt.Ignored.DetectedVersion != "0.4.0" || receipt.Ignored.Decision != "ignored" || !strings.Contains(receipt.Ignored.Reason, "unsupported") {
		t.Fatalf("ignored kickoff receipt: %+v", receipt.Ignored)
	}
}

func TestSetupCLIExplicitKickoffErrorIncludesResolutionFacts(t *testing.T) {
	root := testkit.GitRepo(t)
	p, err := project.Discover(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	branchBytes, err := exec.Command("git", "-C", root, "symbolic-ref", "--short", "HEAD").Output()
	if err != nil {
		t.Fatal(err)
	}
	branch := strings.TrimSpace(string(branchBytes))
	handoffPath := filepath.Join(root, "incoming", "custom.json")
	if err := os.MkdirAll(filepath.Dir(handoffPath), 0700); err != nil {
		t.Fatal(err)
	}
	stale := strings.Repeat("2", 40)
	handoff := map[string]any{
		"schemaVersion": 1, "kind": "project-kickoff-agent-team-handoff", "status": "approved",
		"projectKickoff": map[string]any{"version": "0.5.1", "approvalId": "APR-stale", "approvedRevision": p.Head},
		"agentTeam":      map[string]any{"initializationSource": "existing"},
		"project":        map[string]any{"id": "project-1", "root": root, "branch": branch, "revision": stale},
		"tracker":        map[string]any{"kind": "markdown", "path": "TASKS.md"},
		"plan":           map[string]any{"scope": "stale handoff", "branch": branch},
	}
	body, _ := json.Marshal(handoff)
	if err := os.WriteFile(handoffPath, body, 0600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(root)

	code, result := invokeOnboarding(t, "setup", "--kickoff", "incoming/custom.json")
	message, _ := result["error"].(string)
	if code == 0 || result["ok"] != false {
		t.Fatalf("explicit unsupported kickoff accepted: %d %+v", code, result)
	}
	for _, want := range []string{"incoming/custom.json", "0.5.0", "0.5.1", p.Head, stale, "project or revision mismatch"} {
		if !strings.Contains(message, want) {
			t.Fatalf("explicit error missing %q: %q", want, message)
		}
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
