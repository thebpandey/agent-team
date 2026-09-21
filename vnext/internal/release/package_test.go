package release_test

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/thebpandey/agent-team/vnext/internal/core"
	"github.com/thebpandey/agent-team/vnext/internal/release"
)

func TestPackageCommand(t *testing.T) {
	if err := release.ValidatePackageArgs("8.0", "bad"); !errors.Is(err, core.ErrRevision) {
		t.Fatal(err)
	}
	if err := release.ValidatePackageArgs("8.1.0", strings.Repeat("a", 40)); err != nil {
		t.Fatal(err)
	}
	if err := release.VerifyOutputAllowlist("8.1.0", []string{"agent-teamctl-8.1.0.zip", "SHA256SUMS", "RELEASE.json", "SBOM.cdx.json"}); err != nil {
		t.Fatal(err)
	}
	if err := release.VerifyOutputAllowlist("8.1.0", []string{"secret.env"}); !errors.Is(err, core.ErrPath) {
		t.Fatal(err)
	}
}

func TestPackageRejectsMissingInputs(t *testing.T) {
	source, output := t.TempDir(), t.TempDir()
	if err := release.BuildReleasePackageFrom(source, output, "8.0.0", strings.Repeat("a", 40)); !errors.Is(err, core.ErrPath) {
		t.Fatal("missing executable/contract/entrypoints accepted", err)
	}
	if err := os.WriteFile(filepath.Join(source, "agent-teamctl"), []byte("binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := release.BuildReleasePackageFrom(source, output, "8.0.0", strings.Repeat("a", 40)); !errors.Is(err, core.ErrPath) {
		t.Fatal("missing contract/entrypoints accepted", err)
	}
}

func TestReleasePackageOutputs(t *testing.T) {
	source, output := t.TempDir(), t.TempDir()
	for path, body := range map[string]string{"agent-teamctl": "binary", "WORKER-CONTRACT": "contract", "codex/SKILL.md": "codex", "claude/SKILL.md": "claude", "VERSION": "8.0.0\n"} {
		full := filepath.Join(source, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	commit := strings.Repeat("a", 40)
	if err := release.BuildReleasePackageFrom(source, output, "8.0.0", commit); err != nil {
		t.Fatal(err)
	}
	if err := release.BuildReleasePackageFrom(source, t.TempDir(), "8.0.1", commit); !errors.Is(err, core.ErrRevision) {
		t.Fatal("mismatched VERSION accepted", err)
	}
	outputs, err := release.ListPackageOutputs(output, "8.0.0")
	if err != nil || len(outputs) != 4 {
		t.Fatal(outputs, err)
	}
	raw, err := os.ReadFile(filepath.Join(output, "RELEASE.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest release.Manifest
	if err := json.Unmarshal(raw, &manifest); err != nil || manifest.Version != "8.0.0" || manifest.Commit != commit {
		t.Fatal(manifest, err)
	}
	if err := os.WriteFile(filepath.Join(output, "extra"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := release.ListPackageOutputs(output, "8.0.0"); !errors.Is(err, core.ErrPath) {
		t.Fatal("extra output accepted", err)
	}
}

func TestWindowsBundleOutputs(t *testing.T) {
	source, output := t.TempDir(), t.TempDir()
	for path, body := range map[string]string{"agent-teamctl.exe": "windows-binary", "WORKER-CONTRACT": "contract", "codex/SKILL.md": "codex", "claude/SKILL.md": "claude", "VERSION": "8.0.0\n"} {
		full := filepath.Join(source, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	commit := strings.Repeat("a", 40)
	if err := release.BuildWindowsBundleFrom(source, output, "8.0.0", commit); err != nil {
		t.Fatal(err)
	}
	bundle := filepath.Join(output, "agent-teamctl-8.0.0-windows-amd64.zip")
	if err := release.VerifyWindowsBundle(bundle, filepath.Join(output, "agent-teamctl-8.0.0-windows-amd64.zip.sha256"), "8.0.0", commit); err != nil {
		t.Fatal(err)
	}
	secondOutput := t.TempDir()
	if err := release.BuildWindowsBundleFrom(source, secondOutput, "8.0.0", commit); err != nil {
		t.Fatal(err)
	}
	firstRaw, err := os.ReadFile(bundle)
	if err != nil {
		t.Fatal(err)
	}
	secondRaw, err := os.ReadFile(filepath.Join(secondOutput, "agent-teamctl-8.0.0-windows-amd64.zip"))
	if err != nil || !bytes.Equal(firstRaw, secondRaw) {
		t.Fatalf("Windows bundle is not deterministic: %v", err)
	}
	archive, err := zip.OpenReader(bundle)
	if err != nil {
		t.Fatal(err)
	}
	defer archive.Close()
	want := []string{"RELEASE.json", "SBOM.cdx.json", "SHA256SUMS", "VERSION", "WORKER-CONTRACT", "agent-teamctl-8.0.0.zip", "agent-teamctl.exe", "claude/SKILL.md", "codex/SKILL.md"}
	if len(archive.File) != len(want) {
		t.Fatalf("unexpected Windows bundle entries: %#v", archive.File)
	}
	for index, entry := range archive.File {
		if entry.Name != want[index] {
			t.Fatalf("Windows bundle entry %d = %q, want %q", index, entry.Name, want[index])
		}
	}
	raw, err := os.ReadFile(bundle)
	if err != nil {
		t.Fatal(err)
	}
	index := bytes.LastIndex(raw, []byte("windows-binary"))
	if index < 0 {
		t.Fatal("outer Windows executable bytes missing")
	}
	copy(raw[index:index+len("windows-binary")], []byte("altered-binary"))
	if err := os.WriteFile(bundle, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(raw)
	if err := os.WriteFile(bundle+".sha256", []byte(hex.EncodeToString(sum[:])+"  agent-teamctl-8.0.0-windows-amd64.zip\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := release.VerifyWindowsBundle(bundle, bundle+".sha256", "8.0.0", commit); err == nil {
		t.Fatal("tampered outer executable accepted")
	}
}

func TestPackageOutputsIncludeWindowsBundle(t *testing.T) {
	source, output := t.TempDir(), t.TempDir()
	for path, body := range map[string]string{"agent-teamctl": "linux-binary", "agent-teamctl.exe": "windows-binary", "WORKER-CONTRACT": "contract", "codex/SKILL.md": "codex", "claude/SKILL.md": "claude", "VERSION": "8.0.0\n"} {
		full := filepath.Join(source, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	commit := strings.Repeat("a", 40)
	if err := release.BuildReleasePackageFrom(source, output, "8.0.0", commit); err != nil {
		t.Fatal(err)
	}
	if err := release.BuildWindowsBundleFrom(source, output, "8.0.0", commit); err != nil {
		t.Fatal(err)
	}
	if err := release.VerifyPackageOutputs(output, "8.0.0", commit); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(output, "agent-teamctl-8.0.0.zip"), []byte("tampered Linux archive"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := release.VerifyPackageOutputs(output, "8.0.0", commit); err == nil {
		t.Fatal("tampered Linux archive accepted")
	}
}

func TestReleaseMetadata(t *testing.T) {
	vnext := filepath.Clean(filepath.Join("..", ".."))
	versionRaw, err := os.ReadFile(filepath.Join(vnext, "VERSION"))
	if err != nil {
		t.Fatal(err)
	}
	version := strings.TrimSpace(string(versionRaw))
	if err := release.ValidatePackageArgs(version, strings.Repeat("a", 40)); err != nil {
		t.Fatal(err)
	}
	releaseRaw, err := os.ReadFile(filepath.Join(vnext, "RELEASE.json"))
	if err != nil {
		t.Fatal(err)
	}
	var metadata struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(releaseRaw, &metadata); err != nil || metadata.Version != version {
		t.Fatal(metadata, err)
	}
	repository := filepath.Join(vnext, "..")
	for _, path := range []string{"CHANGELOG.md"} {
		body, err := os.ReadFile(filepath.Join(repository, filepath.FromSlash(path)))
		if err != nil || !strings.Contains(string(body), version) {
			t.Fatal(path, err)
		}
	}
	workflow, _ := os.ReadFile(filepath.Join(repository, ".github", "workflows", "vnext-release.yml"))
	workflowText := strings.ReplaceAll(string(workflow), "\r\n", "\n")
	checksumStep := "- run: sha256sum -c SHA256SUMS\n        working-directory: vnext/release-artifacts"
	canonicalTempStep := "- name: Use canonical test temp\n        shell: bash\n        run: |\n          printf 'TMPDIR=%s\\nTMP=%s\\nTEMP=%s\\n' \"$RUNNER_TEMP\" \"$RUNNER_TEMP\" \"$RUNNER_TEMP\" >> \"$GITHUB_ENV\""
	readinessUpload := "with:\n          name: vnext-release-readiness\n          path: |\n            vnext/release-readiness.json\n            vnext/release-evidence/*\n            vnext/release-artifacts/*"
	readinessDownload := "with: { name: vnext-release-readiness, path: vnext }"
	tagCommand := `git -c user.name="github-actions[bot]" -c user.email="41898282+github-actions[bot]@users.noreply.github.com" tag -a "v${{ inputs.version }}" -m "Agent-Team v${{ inputs.version }}"`
	windowsBundleCanary := "windows-bundle-canary:\n    needs: package\n    permissions: { contents: read }\n    runs-on: windows-latest"
	rootSkillCanary := []string{"root-skill-home", ".codex/skills/agent-team", "Copy-Item -LiteralPath (Join-Path $env:GITHUB_WORKSPACE \"SKILL.md\")", "Windows source-root conflict was not rejected", "Conflict created a native manifest", "Move-Item -LiteralPath $staleRoot", "agent-team-vnext/SKILL.md"}
	if !strings.Contains(workflowText, "${{ inputs.version }}") || !strings.Contains(workflowText, "permissions:\n  contents: read") || !strings.Contains(workflowText, "permissions: { contents: write }") || !strings.Contains(workflowText, "${{ github.workspace }}/vnext/release-artifacts") || strings.Count(workflowText, checksumStep) != 2 || strings.Count(workflowText, canonicalTempStep) != 2 || !strings.Contains(workflowText, readinessUpload) || !strings.Contains(workflowText, readinessDownload) || !strings.Contains(workflowText, "verify-gates --evidence release-readiness.json") || !strings.Contains(workflowText, tagCommand) || !strings.Contains(workflowText, windowsBundleCanary) || !strings.Contains(workflowText, "agent-teamctl-${{ inputs.version }}-windows-amd64.zip") || !strings.Contains(workflowText, "install --host both --json") {
		t.Fatal("workflow release contract is incomplete")
	}
	for _, want := range rootSkillCanary {
		if !strings.Contains(workflowText, want) {
			t.Fatalf("workflow omits Windows source-root canary contract %q", want)
		}
	}
}
