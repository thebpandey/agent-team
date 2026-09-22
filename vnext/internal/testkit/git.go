package testkit

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func GitRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	run := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, output)
		}
	}
	run("init")
	// Keep background maintenance from changing .git during tree snapshots.
	run("config", "gc.auto", "0")
	run("config", "maintenance.auto", "false")
	run("config", "user.email", "test@example.invalid")
	run("config", "user.name", "Agent-Team Test")
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("fixture\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", "README.md")
	run("commit", "-m", "fixture")
	return root
}
