package release_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
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
		for _, want := range []string{"TASKS.md", "Beads", "Codex", "Claude", "BLOCKERS.md", "DECISIONS.md", "Windows", "macOS", "Linux", "legacy", "FIX", "CLEAN", "rollback", "uninstall"} {
			if !strings.Contains(text, want) {
				t.Fatalf("%s missing %q", path, want)
			}
		}
		if (path == "README.md" || path == "GETTING_STARTED.md") && !strings.Contains(text, "agent-teamctl install --host both --json") {
			t.Fatalf("%s missing concrete install command", path)
		}
	}
}

func TestHostSkillEntrypointsHaveValidFrontmatter(t *testing.T) {
	vnext := filepath.Clean(filepath.Join("..", ".."))
	for _, host := range []string{"codex", "claude"} {
		body, err := os.ReadFile(filepath.Join(vnext, host, "SKILL.md"))
		if err != nil {
			t.Fatal(err)
		}
		lf := strings.ReplaceAll(string(body), "\r\n", "\n")
		for _, ending := range []string{"\n", "\r\n"} {
			validateHostFrontmatter(t, host, strings.ReplaceAll(lf, "\n", ending))
		}
	}
}

func TestRootSkillRoutesLatestRepositoryToNativeRelease(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", "..", ".."))
	versionRaw, err := os.ReadFile(filepath.Join(root, "vnext", "VERSION"))
	if err != nil {
		t.Fatal(err)
	}
	version := strings.TrimSpace(string(versionRaw))
	for _, path := range []string{"SKILL.md", "vnext/codex/SKILL.md", "vnext/claude/SKILL.md"} {
		body, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(path)))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(body), "metadata:\n  version: \""+version+"\"") {
			t.Fatalf("%s metadata version does not match vnext/VERSION %q", path, version)
		}
	}
	rootSkill, err := os.ReadFile(filepath.Join(root, "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(rootSkill)
	for _, want := range []string{
		"Route `setup`, `status`, and `start` through the installed native `agent-teamctl` contract.",
		"agent-teamctl install --host both --json",
		"Report the installed native binary version when available.",
		"require a checksum-verified native update",
		"report the repository version as available and installation-needed",
		"Historical Node materials do not supply an alternate current-action route.",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("root SKILL.md does not route latest-repository use to native v%s: missing %q", version, want)
		}
	}
	if strings.Contains(text, "Node-based v7.3.1 runtime described by legacy references is not vNext authority") {
		t.Fatal("root SKILL.md retains misleading v7 runtime routing")
	}
}

func TestDisplayedCurrentVersionsFollowNativeReleaseAuthority(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", "..", ".."))
	versionRaw, err := os.ReadFile(filepath.Join(root, "vnext", "VERSION"))
	if err != nil {
		t.Fatal(err)
	}
	version := strings.TrimSpace(string(versionRaw))
	read := func(path string) string {
		t.Helper()
		body, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(path)))
		if err != nil {
			t.Fatal(err)
		}
		return string(body)
	}
	var release struct{ Version string }
	if err := json.Unmarshal([]byte(read("vnext/RELEASE.json")), &release); err != nil || release.Version != version {
		t.Fatalf("RELEASE.json version does not match vnext/VERSION %q: %v", version, err)
	}
	var contract struct {
		Schema  int
		Version string
		Actions []string
	}
	if err := json.Unmarshal([]byte(read("vnext/WORKER-CONTRACT")), &contract); err != nil {
		t.Fatal(err)
	}
	if contract.Schema != 1 || contract.Version != version || strings.Join(contract.Actions, ",") != "setup,status,start,task add,one-off,pause,stop,cancel,resume" {
		t.Fatalf("WORKER-CONTRACT must retain schema-1 actions and match vnext/VERSION %q", version)
	}
	for _, path := range []string{"SKILL.md", "vnext/codex/SKILL.md", "vnext/claude/SKILL.md"} {
		if !strings.Contains(read(path), "metadata:\n  version: \""+version+"\"") {
			t.Fatalf("%s metadata version does not match vnext/VERSION %q", path, version)
		}
	}
	for path, want := range map[string]string{
		"README.md":          "native version is **[v" + version + "]",
		"GETTING_STARTED.md": "agent-teamctl-" + version + "-windows-amd64.zip",
		"CHANGELOG.md":       "## " + version + " - ",
		"docs/releases/" + version + "-readiness.md": "# Agent-Team " + version + " release readiness",
		"references/WORDMARK.md":                     "from metadata.version in the installed SKILL.md",
	} {
		if !strings.Contains(read(path), want) {
			t.Fatalf("%s missing current version authority %q", path, want)
		}
	}
	index := read("index.html")
	for _, displayed := range regexp.MustCompile(`\b8\.\d+\.\d+\b`).FindAllString(index, -1) {
		if displayed != version {
			t.Fatalf("Pages displays stale native version %q; want %q", displayed, version)
		}
	}
	for _, want := range []string{"Agent<span>-Team</span> / " + version, "AGENT-TEAM / " + version, "Agent-Team " + version + " · Created by", "releases/download/v" + version} {
		if !strings.Contains(index, want) {
			t.Fatalf("Pages missing current version surface %q", want)
		}
	}
	if !strings.Contains(read("references/WORDMARK.md"), "Use the installed version, not a guessed latest version.") {
		t.Fatal("wordmark must use installed skill metadata rather than a guessed version")
	}
}

func TestHistoricalVersionsAreExplicitlyLabeled(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", "..", ".."))
	read := func(path string) string {
		t.Helper()
		body, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(path)))
		if err != nil {
			t.Fatal(err)
		}
		return string(body)
	}
	if !strings.Contains(read("README.md"), "historical Node package **v7.3.1**") || !strings.Contains(read("GETTING_STARTED.md"), "Node-based Agent-Team 7.3.1 instructions below are legacy") {
		t.Fatal("the retained v7.3.1 package must be labeled historical")
	}
	for _, path := range []string{"agent-team-guide-v7.0.2.html", "agent-team-guide-v7.1.0.html"} {
		guide := read(path)
		if !strings.Contains(guide, "Historical Agent-Team v") || !strings.Contains(guide, "historical guide; it is not the current native release") {
			t.Fatalf("%s must visibly label its historical release", path)
		}
	}
}

func TestWindowsDownloadInstructionsUseSeparateExtractionDirectory(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", "..", ".."))
	versionRaw, err := os.ReadFile(filepath.Join(root, "vnext", "VERSION"))
	if err != nil {
		t.Fatal(err)
	}
	version := strings.TrimSpace(string(versionRaw))
	gettingStarted, err := os.ReadFile(filepath.Join(root, "GETTING_STARTED.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(gettingStarted), "$distribution = \"agent-teamctl-"+version+"-windows-amd64\"") || !strings.Contains(string(gettingStarted), "-DestinationPath $distribution") || !strings.Contains(string(gettingStarted), "Join-Path $distribution \"agent-teamctl.exe\"") || strings.Contains(string(gettingStarted), "-DestinationPath .\n") {
		t.Fatal("Windows instructions must keep downloaded assets outside the extracted distribution")
	}
	readme, err := os.ReadFile(filepath.Join(root, "README.md"))
	if err != nil || !strings.Contains(string(readme), "extracted folder") {
		t.Fatal("README must direct Windows users to run from the extracted folder")
	}
}

func validateHostFrontmatter(t *testing.T, host, body string) {
	t.Helper()
	allowed := map[string]bool{
		"allowed-tools": true,
		"description":   true,
		"license":       true,
		"metadata":      true,
		"name":          true,
	}
	parts := strings.SplitN(body, "---", 3)
	if len(parts) != 3 || strings.TrimSpace(parts[0]) != "" {
		t.Fatalf("%s/SKILL.md missing YAML frontmatter", host)
	}
	fields := map[string]string{}
	for _, line := range strings.Split(parts[1], "\n") {
		line = strings.TrimSuffix(line, "\r")
		if line == "" || strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") {
			continue
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			t.Fatalf("%s/SKILL.md has malformed frontmatter line %q", host, line)
		}
		key = strings.TrimSpace(key)
		if !allowed[key] {
			t.Fatalf("%s/SKILL.md has unsupported top-level field %q", host, key)
		}
		fields[key] = strings.TrimSpace(value)
	}
	if fields["name"] != "agent-team" {
		t.Fatalf("%s/SKILL.md has invalid name %q", host, fields["name"])
	}
	if fields["description"] == "" {
		t.Fatalf("%s/SKILL.md missing required description", host)
	}
}
