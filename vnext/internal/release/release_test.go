package release_test

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/thebpandey/agent-team/vnext/internal/core"
	"github.com/thebpandey/agent-team/vnext/internal/release"
)

func writeFixture(t *testing.T, root string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, "vnext"), 0o755); err != nil {
		t.Fatal(err)
	}
	for path, body := range map[string]string{"README.md": "readme", "vnext/VERSION": "8.0.0\n", "agent-teamctl": "binary"} {
		mode := os.FileMode(0o644)
		if path == "agent-teamctl" {
			mode = 0o755
		}
		if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(path)), []byte(body), mode); err != nil {
			t.Fatal(err)
		}
	}
}

func testManifest(t *testing.T, root string) release.Manifest {
	t.Helper()
	m, err := release.BuildManifest(root, "8.0.0", strings.Repeat("a", 40), "spec-1", []string{"README.md", "agent-teamctl", "vnext/VERSION"})
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestDeterministicArtifact(t *testing.T) {
	roots := []string{t.TempDir(), t.TempDir()}
	for _, root := range roots {
		writeFixture(t, root)
	}
	m1 := testManifest(t, roots[0])
	m2, err := release.BuildManifest(roots[1], m1.Version, m1.Commit, m1.SpecRevision, m1.Files)
	if err != nil || !reflect.DeepEqual(m1, m2) {
		t.Fatal(m1, m2, err)
	}
	a1, err := release.BuildArtifact(roots[0], filepath.Join(t.TempDir(), "a.zip"), m1)
	if err != nil {
		t.Fatal(err)
	}
	a2, err := release.BuildArtifact(roots[1], filepath.Join(t.TempDir(), "b.zip"), m2)
	if err != nil || a1.SHA256 != a2.SHA256 || a1.Bytes != a2.Bytes {
		t.Fatal(a1, a2, err)
	}
	if err := release.VerifyArtifact(a1, m1); err != nil {
		t.Fatal(err)
	}
	sbom, err := release.BuildSBOM(m1)
	if err != nil || sbom.Tool != "native" || sbom.Serial != "urn:agent-team:"+release.ManifestSHA256(m1) || sbom.Components[0].Type != "file" || sbom.Components[0].BOMRef == "" || release.VerifySBOM(sbom, m1) != nil {
		t.Fatal(sbom, err)
	}
	sbom.Components[0], sbom.Components[1] = sbom.Components[1], sbom.Components[0]
	if err := release.VerifySBOM(sbom, m1); !errors.Is(err, core.ErrRevision) {
		t.Fatal("component reorder accepted")
	}
}

func TestExecutableModeAllowlist(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root)
	m := testManifest(t, root)
	a, err := release.BuildArtifact(root, filepath.Join(t.TempDir(), "a.zip"), m)
	if err != nil || release.VerifyArtifact(a, m) != nil {
		t.Fatal(a, err)
	}
	m.Executable = "README.md"
	if err := release.VerifyArtifact(a, m); !errors.Is(err, core.ErrRevision) {
		t.Fatal("non-allowlisted executable mode accepted", err)
	}
}

func TestArtifactMutationAndAllowlist(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root)
	m := testManifest(t, root)
	a, err := release.BuildArtifact(root, filepath.Join(t.TempDir(), "a.zip"), m)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(a.Path)
	if err != nil {
		t.Fatal(err)
	}
	raw[len(raw)-1] ^= 1
	if err := os.WriteFile(a.Path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := release.VerifyArtifact(a, m); !errors.Is(err, core.ErrRevision) {
		t.Fatal(err)
	}
	if _, err := release.BuildManifest(root, "8.0.0", strings.Repeat("a", 40), "spec-1", []string{"secret.env"}); !errors.Is(err, core.ErrPath) {
		t.Fatal(err)
	}
}

func TestArtifactMetadataAndRequiredExecutable(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root)
	m := testManifest(t, root)
	a, err := release.BuildArtifact(root, filepath.Join(t.TempDir(), "a.zip"), m)
	if err != nil {
		t.Fatal(err)
	}
	badHash := a
	badHash.SHA256 = strings.Repeat("0", 64)
	if !errors.Is(release.VerifyArtifact(badHash, m), core.ErrRevision) {
		t.Fatal("artifact hash mutation accepted")
	}
	badBytes := a
	badBytes.Bytes++
	if !errors.Is(release.VerifyArtifact(badBytes, m), core.ErrRevision) {
		t.Fatal("artifact byte mutation accepted")
	}
	if _, err := release.BuildManifest(root, "8.0.0", strings.Repeat("a", 40), "spec-1", []string{"README.md"}); !errors.Is(err, core.ErrPath) {
		t.Fatal("missing executable accepted")
	}
}

func TestArtifactRejectsUnsafeAndDuplicateMembers(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root)
	if _, err := release.BuildManifest(root, "8.0.0", strings.Repeat("a", 40), "spec-1", []string{"agent-teamctl", "agent-teamctl"}); !errors.Is(err, core.ErrPath) {
		t.Fatal("duplicate path accepted", err)
	}
	if _, err := release.BuildManifest(root, "8.0.0", strings.Repeat("a", 40), "spec-1", []string{"../agent-teamctl"}); !errors.Is(err, core.ErrPath) {
		t.Fatal("traversal accepted", err)
	}
	outside := filepath.Join(t.TempDir(), "outside")
	if err := os.WriteFile(outside, []byte("outside"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "link")); err == nil {
		if _, err := release.BuildManifest(root, "8.0.0", strings.Repeat("a", 40), "spec-1", []string{"agent-teamctl", "link"}); !errors.Is(err, core.ErrPath) {
			t.Fatal("symlink accepted", err)
		}
	}
	m := testManifest(t, root)
	path := filepath.Join(t.TempDir(), "duplicate.zip")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	z := zip.NewWriter(f)
	for range 2 {
		w, createErr := z.Create("agent-teamctl")
		if createErr != nil {
			t.Fatal(createErr)
		}
		_, _ = w.Write([]byte("binary"))
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	info, _ := os.Stat(path)
	bad := release.Artifact{Path: path, SHA256: fileDigest(t, path), Bytes: info.Size()}
	if err := release.VerifyArtifact(bad, m); !errors.Is(err, core.ErrRevision) {
		t.Fatal("duplicate archive member accepted", err)
	}
}

func fileDigest(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
