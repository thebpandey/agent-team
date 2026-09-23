package project

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/thebpandey/agent-team/vnext/internal/testkit"
)

func TestFreshSetupBeforeFirstCommit(t *testing.T) {
	root := t.TempDir()
	if output, err := exec.Command("git", "-C", root, "init").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v %s", err, output)
	}
	result, err := Onboard(context.Background(), SetupOptions{Root: root, Tracker: "tasks-md", Approved: true})
	if err != nil || result.Status != "initialized" {
		t.Fatalf("result=%+v err=%v", result, err)
	}
}

func TestFirstUseExplainsChoicesWithoutWriting(t *testing.T) {
	root := testkit.GitRepo(t)
	result, err := Onboard(context.Background(), SetupOptions{Root: root})
	if err != nil || result.Status != "needs_input" || result.NextAction != "choose_tracker" {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	for _, name := range []string{"TASKS.md", "DECISIONS.md", "AGENT_TEAM_RULES.md", ".agent-team"} {
		if _, err := os.Stat(filepath.Join(root, name)); !os.IsNotExist(err) {
			t.Fatalf("inspection created %s", name)
		}
	}
}

func TestApprovedFreshSetupUsesTasksAndNeedsFirstSettings(t *testing.T) {
	root := testkit.GitRepo(t)
	result, err := Onboard(context.Background(), SetupOptions{Root: root, Tracker: "tasks-md", Approved: true})
	if err != nil || result.Status != "initialized" || result.NextAction != "settings" || result.Tracker.Kind != "tasks-md" {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	if _, err := os.Stat(filepath.Join(root, ".agent-team/v8/authority.json")); !os.IsNotExist(err) {
		t.Fatal("fresh setup must not create release cutover authority")
	}
	if err := os.WriteFile(filepath.Join(root, "TASKS.md"), []byte("# Tasks\n\nUser changed the tracker after setup.\n"), 0600); err != nil {
		t.Fatal(err)
	}
	again, err := Onboard(context.Background(), SetupOptions{Root: root})
	if err != nil || again.Status != "initialized" || again.ReceiptPath != result.ReceiptPath {
		t.Fatalf("resume=%+v err=%v", again, err)
	}
}

func TestSetupPreservesGovernanceAndRequiresTrackerChoice(t *testing.T) {
	root := testkit.GitRepo(t)
	if err := os.Mkdir(filepath.Join(root, ".beads"), 0700); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{"TASKS.md": "# Tasks\n", "DECISIONS.md": "custom decisions\n"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	result, err := Onboard(context.Background(), SetupOptions{Root: root})
	if err != nil || result.Tracker.Kind != "beads" || result.NextAction != "approve_artifacts" {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	result, err = Onboard(context.Background(), SetupOptions{Root: root, Tracker: "tasks-md", Approved: true})
	if err != nil || result.Tracker.Kind != "tasks-md" {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	for name, want := range map[string]string{"DECISIONS.md": "custom decisions\n"} {
		got, _ := os.ReadFile(filepath.Join(root, name))
		if string(got) != want {
			t.Fatalf("overwrote %s", name)
		}
	}
}

func TestSetupRejectsEscapingTrackerBeforeWriting(t *testing.T) {
	root := testkit.GitRepo(t)
	if err := os.Symlink(t.TempDir(), filepath.Join(root, ".agent-team")); err != nil {
		t.Skip(err)
	}
	if _, err := Onboard(context.Background(), SetupOptions{Root: root, Tracker: "tasks-md", Approved: true}); err == nil {
		t.Fatal("accepted external state root")
	}
	if _, err := os.Stat(filepath.Join(root, "TASKS.md")); !os.IsNotExist(err) {
		t.Fatal("created tracker before validating state root")
	}
}
