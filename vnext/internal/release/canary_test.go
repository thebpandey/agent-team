package release_test

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"

	"github.com/thebpandey/agent-team/vnext/internal/install"
	"github.com/thebpandey/agent-team/vnext/internal/release"
)

func writeCanaryFile(t *testing.T, path string, body []byte) install.ReleaseFile {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(body)
	return install.ReleaseFile{Path: path, SHA256: hex.EncodeToString(sum[:]), Bytes: int64(len(body))}
}

func canaryFixture(t *testing.T) (install.Layout, install.Release, install.InstallManifest) {
	t.Helper()
	root := t.TempDir()
	installed, source := filepath.Join(root, "installed"), filepath.Join(root, "release")
	codexHome, claudeHome := filepath.Join(root, ".agents"), filepath.Join(root, ".claude")
	oldBinary := writeCanaryFile(t, filepath.Join(installed, "bin", "agent-teamctl"), []byte("binary-7.9.0"))
	oldContract := writeCanaryFile(t, filepath.Join(installed, "WORKER-CONTRACT"), []byte(`{"schema":1,"version":"7.9.0"}`))
	oldCodex := writeCanaryFile(t, filepath.Join(codexHome, "skills", "agent-team", "agent-team-vnext", "SKILL.md"), []byte("old codex skill"))
	oldClaude := writeCanaryFile(t, filepath.Join(claudeHome, "skills", "agent-team", "agent-team-vnext", "SKILL.md"), []byte("old claude skill"))
	binary := writeCanaryFile(t, filepath.Join(source, "agent-teamctl"), []byte("binary-8.0.0"))
	contract := writeCanaryFile(t, filepath.Join(source, "WORKER-CONTRACT"), []byte(`{"schema":1,"version":"8.0.0"}`))
	codex := writeCanaryFile(t, filepath.Join(source, "codex", "SKILL.md"), []byte("setup status start"))
	claude := writeCanaryFile(t, filepath.Join(source, "claude", "SKILL.md"), []byte("setup status start"))
	if err := os.WriteFile(filepath.Join(installed, "unrelated-host-setting.json"), []byte("preserve-me"), 0o644); err != nil {
		t.Fatal(err)
	}
	layout := install.Layout{
		DataRoot: installed, BinaryPath: oldBinary.Path, ContractPath: oldContract.Path, ManifestPath: filepath.Join(installed, "manifest.json"),
		SkillRoots:  map[install.Host]string{install.Codex: filepath.Join(codexHome, "skills", "agent-team"), install.Claude: filepath.Join(claudeHome, "skills", "agent-team")},
		ConfigPaths: map[install.Host]string{install.Codex: filepath.Join(codexHome, "hooks.json"), install.Claude: filepath.Join(claudeHome, "settings.json")},
	}
	rel := install.Release{Version: "8.0.0", Revision: "0123456789abcdef0123456789abcdef01234567", Binary: binary, Contract: contract, Entrypoints: map[install.Host]install.ReleaseFile{install.Codex: codex, install.Claude: claude}}
	manifest := install.InstallManifest{Schema: 1, Version: "7.9.0", ReleaseRevision: "abcdef0123456789abcdef0123456789abcdef01", Hosts: []install.Host{install.Codex, install.Claude}, HostHomes: map[install.Host]string{install.Codex: codexHome, install.Claude: claudeHome}, Files: []install.OwnedFile{
		{Role: install.BinaryRole, Path: oldBinary.Path, SHA256: oldBinary.SHA256, Version: "7.9.0", Revision: "abcdef0123456789abcdef0123456789abcdef01", Bytes: oldBinary.Bytes},
		{Role: install.ContractRole, Path: oldContract.Path, SHA256: oldContract.SHA256, Version: "7.9.0", Revision: "abcdef0123456789abcdef0123456789abcdef01", Bytes: oldContract.Bytes},
		{Role: install.EntrypointRole, Host: install.Codex, Path: oldCodex.Path, SHA256: oldCodex.SHA256, Version: "7.9.0", Revision: "abcdef0123456789abcdef0123456789abcdef01", Bytes: oldCodex.Bytes},
		{Role: install.EntrypointRole, Host: install.Claude, Path: oldClaude.Path, SHA256: oldClaude.SHA256, Version: "7.9.0", Revision: "abcdef0123456789abcdef0123456789abcdef01", Bytes: oldClaude.Bytes},
	}}
	return layout, rel, manifest
}

func TestCodexCanary(t *testing.T) {
	layout, rel, manifest := canaryFixture(t)
	canary, err := release.RunInstallCanary(context.Background(), layout, rel, manifest, []install.Host{install.Codex})
	if err != nil || len(canary.Hosts) != 1 || canary.HostResults[install.Codex].Host != install.Codex || !canary.HostResults[install.Codex].UnrelatedPreserved || canary.HostResults[install.Codex].BinarySHA256 == "" {
		t.Fatal(canary, err)
	}
}

func TestClaudeCanary(t *testing.T) {
	layout, rel, manifest := canaryFixture(t)
	canary, err := release.RunInstallCanary(context.Background(), layout, rel, manifest, []install.Host{install.Claude})
	if err != nil || len(canary.Hosts) != 1 || canary.HostResults[install.Claude].Host != install.Claude || canary.HostResults[install.Claude].SkillSHA256 == "" {
		t.Fatal(canary, err)
	}
}

func TestBothHostsRollbackAndActions(t *testing.T) {
	layout, rel, manifest := canaryFixture(t)
	canary, err := release.RunInstallCanary(context.Background(), layout, rel, manifest, []install.Host{install.Codex, install.Claude})
	if err != nil || len(canary.Hosts) != 2 || !canary.RollbackVerified || canary.BinarySHA256 == "" || canary.SkillSHA256 == "" || canary.ContractSHA256 == "" || !bytes.Equal(canary.UnrelatedBefore, canary.UnrelatedAfter) || canary.RestoredManifestSHA256 == "" || len(canary.RestoredFiles) != len(manifest.Files) {
		t.Fatal(canary, err)
	}
	for _, action := range []string{"setup", "status", "start"} {
		if !slices.Contains(canary.Actions, action) {
			t.Fatalf("missing action %s", action)
		}
	}
	if err := release.VerifyRollback(context.Background(), canary); err != nil {
		t.Fatal(err)
	}
	if err := release.VerifyCutover(release.Cutover{Canary: canary, ProviderVerified: true, InstalledVerified: true, RollbackRehearsed: true, Evidence: []string{"provider", "installed", "rollback"}}); err != nil {
		t.Fatal(err)
	}
}

func writeMatchingArchive(t *testing.T, rel install.Release) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "release.zip")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	archive := zip.NewWriter(file)
	for name, item := range map[string]install.ReleaseFile{"agent-teamctl": rel.Binary, "WORKER-CONTRACT": rel.Contract, "codex/SKILL.md": rel.Entrypoints[install.Codex], "claude/SKILL.md": rel.Entrypoints[install.Claude]} {
		body, err := os.ReadFile(item.Path)
		if err != nil {
			t.Fatal(err)
		}
		writer, err := archive.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := writer.Write(body); err != nil {
			t.Fatal(err)
		}
	}
	writer, err := archive.Create("VERSION")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = writer.Write([]byte(rel.Version + "\n"))
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestArchiveCanaryFixture(t *testing.T) {
	layout, rel, manifest := canaryFixture(t)
	archive := writeMatchingArchive(t, rel)
	canary, err := release.RunInstallCanaryFromArchive(context.Background(), archive, layout, rel, manifest, []install.Host{install.Codex, install.Claude})
	if err != nil || canary.ArchivePath != archive || canary.ArchiveSHA256 == "" || !canary.RollbackVerified || !canary.StagingCleaned || canary.StagingEvidenceDigest == "" {
		t.Fatal(canary, err)
	}
	if err := release.VerifyRollback(context.Background(), canary); err != nil {
		t.Fatal("durable rollback evidence failed", err)
	}
}

func TestPackagedArchiveCanary(t *testing.T) {
	archivePath, manifestPath := os.Getenv("VNEXT_RELEASE_ARCHIVE"), os.Getenv("VNEXT_RELEASE_MANIFEST")
	if archivePath == "" || manifestPath == "" {
		t.Skip("packaged release artifact not supplied")
	}
	raw, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	var packageManifest release.Manifest
	if err := json.Unmarshal(raw, &packageManifest); err != nil {
		t.Fatal(err)
	}
	archive, err := zip.OpenReader(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	defer archive.Close()
	var executable []byte
	for _, entry := range archive.File {
		if entry.Name != packageManifest.Executable {
			continue
		}
		reader, openErr := entry.Open()
		if openErr != nil {
			t.Fatal(openErr)
		}
		executable, err = io.ReadAll(reader)
		_ = reader.Close()
		if err != nil {
			t.Fatal(err)
		}
	}
	if len(executable) == 0 {
		t.Fatal("packaged executable missing")
	}
	executablePath := filepath.Join(t.TempDir(), "agent-teamctl")
	if err := os.WriteFile(executablePath, executable, 0o700); err != nil {
		t.Fatal(err)
	}
	identityRaw, err := exec.Command(executablePath, "version", "--json").Output()
	if err != nil {
		t.Fatal(err)
	}
	var identity struct {
		Version  string `json:"version"`
		Revision string `json:"revision"`
	}
	if err := json.Unmarshal(identityRaw, &identity); err != nil || identity.Version != packageManifest.Version || identity.Revision != packageManifest.Commit {
		t.Fatalf("packaged executable identity = %+v, %v", identity, err)
	}
	metadata := func(name string) install.ReleaseFile {
		t.Helper()
		for _, entry := range archive.File {
			if entry.Name != name {
				continue
			}
			reader, err := entry.Open()
			if err != nil {
				t.Fatal(err)
			}
			body, err := io.ReadAll(reader)
			_ = reader.Close()
			if err != nil {
				t.Fatal(err)
			}
			digest := fileBytesDigest(body)
			if digest != packageManifest.Checksums[name] {
				t.Fatalf("manifest hash mismatch for %s", name)
			}
			return install.ReleaseFile{Path: archivePath, SHA256: digest, Bytes: int64(len(body))}
		}
		t.Fatalf("archive member %s missing", name)
		return install.ReleaseFile{}
	}
	layout, _, prior := canaryFixture(t)
	rel := install.Release{Version: packageManifest.Version, Revision: packageManifest.Commit, Binary: metadata("agent-teamctl"), Contract: metadata("WORKER-CONTRACT"), Entrypoints: map[install.Host]install.ReleaseFile{install.Codex: metadata("codex/SKILL.md"), install.Claude: metadata("claude/SKILL.md")}}
	canary, err := release.RunInstallCanaryFromArchive(context.Background(), archivePath, layout, rel, prior, []install.Host{install.Codex, install.Claude})
	if err != nil || canary.ArchivePath != archivePath || canary.ArchiveSHA256 == "" || !canary.RollbackVerified {
		t.Fatal(canary, err)
	}
}

func fileBytesDigest(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}
