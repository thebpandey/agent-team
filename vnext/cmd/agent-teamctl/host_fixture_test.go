package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/thebpandey/agent-team/vnext/internal/core"
	"github.com/thebpandey/agent-team/vnext/internal/project"
	"github.com/thebpandey/agent-team/vnext/internal/store"
)

// A renamed test executable supplies bounded Beads output for the packaged
// Windows command canary. It is tracker input, never a host-launch substitute.
func TestMain(m *testing.M) {
	if name := strings.TrimSuffix(filepath.Base(os.Args[0]), ".exe"); name == "bd" {
		if strings.Join(os.Args[1:], " ") != "list --json --all --limit 0" {
			os.Exit(2)
		}
		raw, err := os.ReadFile(os.Getenv("AGENT_TEAM_BD_FIXTURE_JSON"))
		if err != nil {
			os.Exit(2)
		}
		_, _ = os.Stdout.Write(raw)
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// Explicitly requested only. This leaves a real isolated Beads project for
// actual Codex host-tool acceptance; it does not create run/team state.
func TestRetainedNativeHostFixture(t *testing.T) {
	root := os.Getenv("VNEXT_ACTION_FIXTURE_DIR")
	if root == "" {
		t.Skip("set VNEXT_ACTION_FIXTURE_DIR for retained real-host fixture")
	}
	if !filepath.IsAbs(root) {
		t.Fatal("fixture path must be absolute")
	}
	if _, err := os.Lstat(root); !os.IsNotExist(err) {
		t.Fatal("fixture path must not exist")
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	initializeActionFixture(t, root, true)
	controller := filepath.Join(filepath.Dir(root), "agent-teamctl-host-fixture"+executableSuffix())
	fixtureCommand(t, "", nil, "go", "build", "-o", controller, ".")
	t.Logf("retained project=%s controller=%s tasks=atf-1,atf-2; no run admitted", root, controller)
}

func initializeActionFixture(t *testing.T, root string, realBeads bool) {
	t.Helper()
	fixtureCommand(t, root, nil, "git", "init", "-q")
	fixtureCommand(t, root, nil, "git", "config", "user.name", "Native fixture")
	fixtureCommand(t, root, nil, "git", "config", "user.email", "fixture@example.invalid")
	for name, text := range map[string]string{
		"README.md":    "# Isolated host acceptance\n\nTask one reports ALPHA. Task two reports BETA. Do not change this file.\n",
		"DECISIONS.md": "# Decisions\n", "AGENT_TEAM_RULES.md": "# Rules\nOnly the explicit task packet grants write scope.\n",
		".gitignore": ".beads/\n.agent-team/\n",
	} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	fixtureCommand(t, root, nil, "git", "add", "README.md", "DECISIONS.md", "AGENT_TEAM_RULES.md", ".gitignore")
	fixtureCommand(t, root, nil, "git", "commit", "-qm", "Initialize isolated native action fixture")
	if realBeads {
		fixtureCommand(t, root, nil, "bd", "init", "--prefix", "atf", "--non-interactive", "--skip-agents", "--skip-hooks", "--quiet")
		for index, word := range []string{"ALPHA", "BETA"} {
			metadata, _ := json.Marshal(map[string]any{"criteria": []string{"Read README.md and report " + word + ". Do not modify any files."}, "writablePaths": []string{fmt.Sprintf("result-%d.txt", index+1)}})
			fixtureCommand(t, root, nil, "bd", "create", "--id", fmt.Sprintf("atf-%d", index+1), "--title", "Report "+word, "--metadata", string(metadata), "--json")
		}
	} else if err := os.Mkdir(filepath.Join(root, ".beads"), 0o755); err != nil {
		t.Fatal(err)
	}
	_, err := project.NewSetupService(store.New(root, core.DefaultConfig().Storage)).Initialize(context.Background(), project.SetupInput{Root: root, Mode: project.PlanMode, Artifacts: []project.ArtifactDecision{
		{Path: ".beads", Mode: project.ExistingArtifact, Confirmation: project.Approved},
		{Path: "DECISIONS.md", Mode: project.ExistingArtifact, Confirmation: project.Approved},
		{Path: "AGENT_TEAM_RULES.md", Mode: project.ExistingArtifact, Confirmation: project.Approved},
	}})
	if err != nil {
		t.Fatal(err)
	}
}

func executableSuffix() string {
	if runtime.GOOS == "windows" {
		return ".exe"
	}
	return ""
}

func fixtureCommand(t *testing.T, root string, extraEnv []string, name string, args ...string) []byte {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = root
	cmd.Env = append(os.Environ(), extraEnv...)
	raw, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %v: %v\n%s", name, args, err, raw)
	}
	return raw
}

// Runs the actual extracted release executable. Synthetic host identities here
// test persisted protocol checks only; actual collaboration is a separate gate.
func TestPackagedNativeActions(t *testing.T) {
	controller := os.Getenv("VNEXT_RELEASE_BINARY")
	if controller == "" {
		controller = filepath.Join(t.TempDir(), "agent-teamctl"+executableSuffix())
		fixtureCommand(t, "", nil, "go", "build", "-o", controller, ".")
	}
	if !filepath.IsAbs(controller) {
		t.Fatal("release executable must be absolute")
	}
	root := t.TempDir()
	initializeActionFixture(t, root, false)
	bin := t.TempDir()
	fixtureCommand(t, "", nil, "go", "test", "-c", "-o", filepath.Join(bin, "bd"+executableSuffix()), ".")
	snapshot := filepath.Join(bin, "beads.json")
	if err := os.WriteFile(snapshot, []byte(`[{"id":"atf-1","title":"Report ALPHA","status":"open","priority":2,"issue_type":"task","metadata":{"criteria":["Report ALPHA"],"writablePaths":["result-1.txt"]}},{"id":"atf-2","title":"Report BETA","status":"open","priority":2,"issue_type":"task","metadata":{"criteria":["Report BETA"],"writablePaths":["result-2.txt"]}},{"id":"atf-3","title":"Report GAMMA","status":"open","priority":2,"issue_type":"task","metadata":{"criteria":["Report GAMMA"],"writablePaths":["result-3.txt"]}}]`), 0o644); err != nil {
		t.Fatal(err)
	}
	env := []string{"PATH=" + bin + string(os.PathListSeparator) + os.Getenv("PATH"), "AGENT_TEAM_BD_FIXTURE_JSON=" + snapshot}
	invoke := func(args ...string) map[string]json.RawMessage {
		t.Helper()
		raw := fixtureCommand(t, root, env, controller, append(args, "--json")...)
		var result map[string]json.RawMessage
		if json.Unmarshal(raw, &result) != nil || string(result["ok"]) != "true" {
			t.Fatalf("invalid native result: %s", raw)
		}
		return result
	}
	configPath := filepath.Join(root, ".agent-team", "config.json")
	before, _ := os.ReadFile(configPath)
	invoke("settings", "parallel_teams=2", "codex.developer.model=gpt-5.6-terra", "codex.developer.effort=high")
	saved, err := project.NewSettingsService(store.New(root, core.DefaultConfig().Storage)).Inspect(context.Background())
	if err != nil || saved.Defaults.ParallelTeams != 2 || saved.CodexDeveloper.Model != "gpt-5.6-terra" || saved.CodexDeveloper.Effort != "high" {
		t.Fatalf("persisted settings=%+v error=%v", saved, err)
	}
	first := invoke("start")
	if string(first["host_dispatch_required"]) != "true" || first["launched"] != nil {
		t.Fatalf("admission misreported as launch: %s", first)
	}
	duplicate := invoke("start")
	if string(duplicate["host_dispatch_required"]) != "false" || string(duplicate["already_admitted"]) != "true" {
		t.Fatalf("duplicate start authorizes another host launch: %s", duplicate)
	}
	var packet core.AssignmentPacket
	if err := json.Unmarshal(first["packet"], &packet); err != nil || packet.Task != "atf-1" {
		t.Fatalf("packet=%+v error=%v", packet, err)
	}
	firstWorktree, firstBase, firstTeam := packet.Worktree, packet.Base, packet.Team
	var digest string
	_ = json.Unmarshal(first["packet_digest"], &digest)
	invoke("start", "--run", string(packet.RunID), "--task", "atf-2")
	fields := []string{"--team", string(packet.Team), "--packet-digest", digest, "--host", "codex", "--identity", "/fixture/retained-worker", "--task", string(packet.Task), "--candidate", packet.SpecRevision}
	for _, action := range []string{"ack", "complete"} {
		invoke(append([]string{"start", "--action", action}, fields...)...)
	}
	invoke("start", "--action", "clean", "--team", string(packet.Team), "--reviewer", "/fixture/independent-reviewer")
	invoke(append([]string{"start", "--action", "idle"}, fields...)...)
	next := invoke("start", "--action", "next", "--team", string(packet.Team))
	if string(next["host_followup_required"]) != "true" {
		t.Fatalf("missing retained follow-up: %s", next)
	}
	if err := json.Unmarshal(next["packet"], &packet); err != nil || packet.Task != "atf-2" {
		t.Fatalf("next packet=%+v error=%v", packet, err)
	}
	if packet.Worktree != firstWorktree || packet.Base != firstBase || packet.Team != firstTeam || len(packet.Scope) != 1 || packet.Scope[0] != "result-2.txt" {
		t.Fatalf("next packet lost retained identity or fresh scope: %+v", packet)
	}
	_ = json.Unmarshal(next["packet_digest"], &digest)
	fields = []string{"--team", string(packet.Team), "--packet-digest", digest, "--host", "codex", "--identity", "/fixture/retained-worker", "--task", string(packet.Task), "--candidate", packet.SpecRevision}
	for _, action := range []string{"ack", "complete"} {
		invoke(append([]string{"start", "--action", action}, fields...)...)
	}
	invoke("start", "--action", "clean", "--team", string(packet.Team), "--reviewer", "/fixture/independent-reviewer")
	invoke(append([]string{"start", "--action", "idle"}, fields...)...)
	terminal := invoke("start", "--action", "next", "--team", string(packet.Team))
	var terminalTeam struct {
		Queue []string `json:"queue"`
		State string   `json:"state"`
	}
	if json.Unmarshal(terminal["team"], &terminalTeam) != nil || len(terminalTeam.Queue) != 0 || terminalTeam.State != "idle" || string(terminal["host_followup_required"]) != "false" {
		t.Fatalf("last task was not consumed into an idle retained team: %s", terminal)
	}
	reused := invoke("start", "--run", string(packet.RunID), "--task", "atf-3")
	if string(reused["host_followup_required"]) != "true" || json.Unmarshal(reused["packet"], &packet) != nil || packet.Task != "atf-3" || packet.Team != firstTeam || packet.Worktree != firstWorktree || len(packet.Scope) != 1 || packet.Scope[0] != "result-3.txt" {
		t.Fatalf("empty retained team was not reused with a fresh bounded packet: %s", reused)
	}
	_ = json.Unmarshal(reused["packet_digest"], &digest)
	invoke("start", "--action", "ack", "--team", string(packet.Team), "--packet-digest", digest, "--host", "codex", "--identity", "/fixture/retained-worker", "--task", string(packet.Task), "--candidate", packet.SpecRevision)
	after, _ := os.ReadFile(configPath)
	if !bytes.Equal(before, after) {
		t.Fatal("native actions changed immutable config")
	}
}
