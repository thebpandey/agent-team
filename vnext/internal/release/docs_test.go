package release_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPublicDocs(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", "..", ".."))
	for _, path := range []string{"README.md", "GETTING_STARTED.md", "SKILL.md", "references/DEPENDENCIES.md"} {
		body, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(path)))
		if err != nil {
			t.Fatal(err)
		}
		text := string(body)
		for _, want := range []string{"agent-teamctl install --host codex|claude|both", "TASKS.md", "Beads", "Codex", "Claude", "BLOCKERS.md", "DECISIONS.md", "Windows", "macOS", "Linux", "legacy", "FIX", "CLEAN", "rollback", "uninstall"} {
			if !strings.Contains(text, want) {
				t.Fatalf("%s missing %q", path, want)
			}
		}
	}
}
