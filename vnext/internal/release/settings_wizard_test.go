package release_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// These are executable packaging checks for the native instruction contract;
// host conversation behavior is additionally exercised by pressure scenarios.
func TestNativeSettingsWizardHasNumberedTransactionalHostChoices(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", ".."))
	for _, host := range []string{"codex", "claude"} {
		body, err := os.ReadFile(filepath.Join(root, host, "SKILL.md"))
		if err != nil {
			t.Fatal(err)
		}
		text := string(body)
		begin := strings.Index(text, "## Numbered role settings wizard")
		if begin < 0 {
			t.Fatalf("%s has no numbered settings wizard", host)
		}
		wizard := text[begin:]
		if end := strings.Index(wizard[3:], "\n## "); end >= 0 {
			wizard = wizard[:end+3]
		}
		for _, rule := range []string{
			"orchestrator", "developer", "reviewer", "visual_reviewer",
			"1. Keep", "2. Inherit", "3 onward", "index -> exact model ID",
			"Never ask the user to type a model ID", "reprompt only that role",
			"catalog is missing", "native model picker", "numbered effort menu",
			"Cancel, no answer, or interruption saves nothing", "one settings command",
			"Selections authorize saving", "no extra approval", "current parent model",
			"original setup/start request", host + ".<role>.model", host + ".<role>.effort",
			"Inheritance stays `inherit`", "picker only if actually available",
			"Keeping effort after changing models requires checking compatibility",
		} {
			if !strings.Contains(wizard, rule) {
				t.Errorf("%s wizard missing %q", host, rule)
			}
		}
		if host == "claude" && !strings.Contains(wizard, "proves the alias resolves to that exact model ID") {
			t.Error("Claude wizard does not require proven alias identity")
		}
	}
}
