package project

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/thebpandey/agent-team/vnext/internal/core"
	"github.com/thebpandey/agent-team/vnext/internal/testkit"
)

func kickoffFixture(t *testing.T) (string, map[string]any) {
	t.Helper()
	root := testkit.GitRepo(t)
	rev, err := git(context.Background(), root, "rev-parse", "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	branchName, err := git(context.Background(), root, "symbolic-ref", "--short", "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	return root, map[string]any{
		"schemaVersion": 1, "kind": "project-kickoff-agent-team-handoff", "status": "approved",
		"projectKickoff": map[string]any{"version": "0.5.0", "approvalId": "APR-005", "approvedRevision": rev},
		"agentTeam":      map[string]any{"testedVersion": "7.3.1", "initializationSource": "existing"},
		"project":        map[string]any{"id": "demo", "root": root, "branch": branchName, "revision": rev},
		"tracker":        map[string]any{"kind": "markdown", "path": "TASKS.md"},
		"plan": map[string]any{
			"scope": "Approved release", "branch": branchName, "acceptance": []string{"works offline"},
			"verification": []string{"printf '%s' 'quoted value' && go test ./..."},
			"authority":    map[string]any{"ownedPaths": []string{"src/**"}, "externalActions": []string{}},
			"tasks":        []map[string]string{{"id": "AT-001"}, {"id": "AT-002"}},
		},
	}
}

func writeKickoffFixture(t *testing.T, root string, value any) string {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	path := "kickoff.json"
	if err := os.WriteFile(filepath.Join(root, path), data, 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadKickoffNormalizesApproved050WithoutWriting(t *testing.T) {
	root, value := kickoffFixture(t)
	value["plan"].(map[string]any)["requiredCapabilities"] = []string{"graphify", "serena"}
	path := writeKickoffFixture(t, root, value)
	before := testkit.SnapshotTree(t, root)
	got, err := LoadKickoff(root, path)
	if err != nil {
		t.Fatal(err)
	}
	if got.TrackerKind != "tasks-md" || got.TrackerRef != "TASKS.md" || !reflect.DeepEqual(got.TaskIDs, []core.TaskID{"AT-001", "AT-002"}) || !reflect.DeepEqual(got.Capabilities, []string{"graphify", "serena"}) || len(got.Resources) != 0 || !reflect.DeepEqual(got.WritablePaths, []string{"src/**"}) {
		t.Fatalf("lost approved facts: %#v", got)
	}
	if len(got.Checks) != 1 || got.Checks[0].Command[len(got.Checks[0].Command)-1] != "printf '%s' 'quoted value' && go test ./..." {
		t.Fatalf("verification changed: %#v", got.Checks)
	}
	if !reflect.DeepEqual(before, testkit.SnapshotTree(t, root)) {
		t.Fatal("reader changed project")
	}
}

func TestLoadKickoffSupportsOnlyApprovedProducerVersions(t *testing.T) {
	for _, test := range []struct {
		version  string
		accepted bool
	}{
		{"0.5.0", true}, {"0.5.1", true}, {"0.5.2", true}, {"0.6.0", false}, {"0.5.1-dev", false},
	} {
		t.Run(test.version, func(t *testing.T) {
			root, value := kickoffFixture(t)
			value["projectKickoff"].(map[string]any)["version"] = test.version
			value["agentTeam"].(map[string]any)["testedVersion"] = "8.0.11"
			got, err := LoadKickoff(root, writeKickoffFixture(t, root, value))
			if (err == nil) != test.accepted {
				t.Fatalf("producer %s accepted=%t: %v", test.version, err == nil, err)
			}
			if test.accepted && !reflect.DeepEqual(got.TaskIDs, []core.TaskID{"AT-001", "AT-002"}) {
				t.Fatalf("lost approved tasks: %#v", got)
			}
		})
	}
}

func TestLoadKickoffRejectsUnsupportedWritableGlobWithFieldAndGrammar(t *testing.T) {
	root, value := kickoffFixture(t)
	value["plan"].(map[string]any)["authority"].(map[string]any)["ownedPaths"] = []string{"packages/*/result.txt"}
	_, err := LoadKickoff(root, writeKickoffFixture(t, root, value))
	if err == nil || !strings.Contains(err.Error(), "kickoff authority") || !strings.Contains(err.Error(), "packages/*/result.txt") || !strings.Contains(err.Error(), "exact relative path or directory/**") {
		t.Fatalf("invalid path diagnostic: %v", err)
	}
}

func TestLoadKickoffRejectsInvalidApprovedFacts(t *testing.T) {
	for _, name := range []string{"status", "root", "branch", "revision", "approved-revision", "unsafe-path", "duplicate-task", "unknown-tracker", "external-action", "version"} {
		t.Run(name, func(t *testing.T) {
			root, value := kickoffFixture(t)
			project := value["project"].(map[string]any)
			plan := value["plan"].(map[string]any)
			switch name {
			case "status":
				value["status"] = "pending"
			case "root":
				project["root"] = t.TempDir()
			case "branch":
				project["branch"] = "unapproved"
			case "revision":
				project["revision"] = strings.Repeat("0", 40)
			case "approved-revision":
				value["projectKickoff"].(map[string]any)["approvedRevision"] = strings.Repeat("0", 40)
			case "unsafe-path":
				plan["authority"].(map[string]any)["ownedPaths"] = []string{"../outside"}
			case "duplicate-task":
				plan["tasks"] = []map[string]string{{"id": "AT-001"}, {"id": "AT-001"}}
			case "unknown-tracker":
				value["tracker"].(map[string]any)["kind"] = "other"
			case "external-action":
				plan["authority"].(map[string]any)["externalActions"] = []string{"deploy"}
			case "version":
				value["projectKickoff"].(map[string]any)["version"] = "99.0.0"
			}
			if _, err := LoadKickoff(root, writeKickoffFixture(t, root, value)); err == nil {
				t.Fatal("accepted invalid handoff")
			}
		})
	}
}

func TestLoadKickoffFlatNativeAndOptionalCapabilities(t *testing.T) {
	root, value := kickoffFixture(t)
	project := value["project"].(map[string]any)
	flat := core.KickoffHandoff{ApprovedPlanRevision: project["revision"].(string), Branch: project["branch"].(string), TrackerKind: "beads", TrackerRef: ".beads", TaskIDs: []core.TaskID{"AT-1"}, Acceptance: []string{"works"}, Checks: []core.Check{{Name: "test", Command: []string{"go", "test", "./..."}}}, WritablePaths: []string{"src"}}
	got, err := LoadKickoff(root, writeKickoffFixture(t, root, flat))
	if err != nil || !reflect.DeepEqual(got, flat) {
		t.Fatalf("flat handoff: %#v, %v", got, err)
	}
}

func TestLoadKickoffDesignatedMarkdownWithoutOptionalCapabilities(t *testing.T) {
	root, value := kickoffFixture(t)
	value["tracker"].(map[string]any)["path"] = ".agent-team/TASKS.md"
	got, err := LoadKickoff(root, writeKickoffFixture(t, root, value))
	if err != nil || got.TrackerRef != ".agent-team/TASKS.md" || len(got.Capabilities) != 0 || len(got.Resources) != 0 {
		t.Fatalf("optional capabilities became required: %#v, %v", got, err)
	}
}

func TestLoadKickoffBoundsAndContainsInput(t *testing.T) {
	root, value := kickoffFixture(t)
	outside := t.TempDir()
	path := writeKickoffFixture(t, outside, value)
	if _, err := LoadKickoff(root, filepath.Join(outside, path)); err == nil {
		t.Fatal("accepted outside handoff")
	}
	if err := os.Symlink(filepath.Join(outside, path), filepath.Join(root, "link.json")); err == nil {
		if _, err := LoadKickoff(root, "link.json"); err == nil {
			t.Fatal("accepted symlink escape")
		}
	}
	if err := os.WriteFile(filepath.Join(root, "large.json"), []byte(strings.Repeat(" ", 251*1024)), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadKickoff(root, "large.json"); err == nil {
		t.Fatal("accepted oversized handoff")
	}
}

func TestLoadKickoffErrorDoesNotEchoNearLimitUnknownField(t *testing.T) {
	root, value := kickoffFixture(t)
	secret := "DO-NOT-ECHO-" + strings.Repeat("x", 200<<10)
	value[secret] = true
	path := writeKickoffFixture(t, root, value)

	_, err := LoadKickoff(root, path)
	if err == nil {
		t.Fatal("handoff with unknown field was accepted")
	}
	message := err.Error()
	if strings.Contains(message, "DO-NOT-ECHO-") {
		t.Fatalf("error echoed untrusted field: length=%d", len(message))
	}
	if len(message) > 1024 {
		t.Fatalf("error is not tightly bounded: length=%d", len(message))
	}
	for _, want := range []string{path, "0.5.0", "0.5.1"} {
		if !strings.Contains(message, want) {
			t.Fatalf("bounded error missing %q: %q", want, message)
		}
	}
}

func TestLoadKickoffErrorDoesNotEchoInvalidExplicitPath(t *testing.T) {
	root, _ := kickoffFixture(t)
	path := string(filepath.Separator) + "DO-NOT-ECHO-PATH-" + strings.Repeat("x", 32<<10)

	_, err := LoadKickoff(root, path)
	if err == nil {
		t.Fatal("absolute escaping handoff path was accepted")
	}
	message := err.Error()
	if strings.Contains(message, "DO-NOT-ECHO-PATH-") {
		t.Fatalf("error echoed invalid explicit path: length=%d", len(message))
	}
	if len(message) > 1024 {
		t.Fatalf("invalid-path error is not tightly bounded: length=%d", len(message))
	}
	for _, want := range []string{"0.5.0", "0.5.1", "unsafe or inaccessible handoff path"} {
		if !strings.Contains(message, want) {
			t.Fatalf("bounded error missing %q: %q", want, message)
		}
	}
}
