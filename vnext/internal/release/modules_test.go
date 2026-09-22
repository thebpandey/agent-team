package release_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/thebpandey/agent-team/vnext/internal/release"
)

func TestBuiltExecutableModuleProvenanceReachesManifestAndSBOM(t *testing.T) {
	root := t.TempDir()
	binary := filepath.Join(root, "agent-teamctl")
	command := exec.Command("go", "build", "-trimpath", "-buildvcs=false", "-o", binary, "./cmd/agent-teamctl")
	command.Dir = filepath.Join("..", "..")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("build real release executable: %s: %v", output, err)
	}
	manifest, err := release.BuildManifest(root, "8.0.11", strings.Repeat("a", 40), "spec-1", []string{"agent-teamctl"})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]json.RawMessage
	if err := json.Unmarshal(raw, &document); err != nil {
		t.Fatal(err)
	}
	var modules []struct{ Path, Version, Sum string }
	if err := json.Unmarshal(document["goModules"], &modules); err != nil {
		t.Fatalf("executable module provenance absent: %s: %v", raw, err)
	}
	sums, err := os.ReadFile(filepath.Join("..", "..", "go.sum"))
	if err != nil {
		t.Fatal(err)
	}
	wantSum := ""
	for _, line := range strings.Split(string(sums), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 3 && fields[0] == "go.yaml.in/yaml/v3" && fields[1] == "v3.0.5" {
			wantSum = fields[2]
		}
	}
	if wantSum == "" {
		t.Fatal("pinned YAML module checksum absent")
	}
	found := false
	for _, module := range modules {
		if module.Path == "go.yaml.in/yaml/v3" {
			found = module.Version == "v3.0.5" && module.Sum == wantSum
		}
	}
	if !found {
		t.Fatalf("YAML provenance differs from pinned source: %s", raw)
	}
	sbom, err := release.BuildSBOM(manifest)
	if err != nil {
		t.Fatal(err)
	}
	sbomRaw, _ := json.Marshal(sbom)
	var bom struct {
		Components []struct {
			Name, Version, Type, PURL string
			Properties                []struct{ Name, Value string }
		}
	}
	if err := json.Unmarshal(sbomRaw, &bom); err != nil {
		t.Fatal(err)
	}
	found = false
	for _, component := range bom.Components {
		if component.Name != "go.yaml.in/yaml/v3" {
			continue
		}
		if component.Type != "library" || component.Version != "v3.0.5" || component.PURL != "pkg:golang/go.yaml.in/yaml/v3@v3.0.5" {
			t.Fatalf("incorrect library component: %s", sbomRaw)
		}
		for _, property := range component.Properties {
			if property.Name == "go:module:sum" && property.Value == wantSum {
				found = true
			}
		}
	}
	if !found || strings.Contains(string(sbomRaw), "license") {
		t.Fatalf("missing faithful module checksum or invented license: %s", sbomRaw)
	}
	if err := release.VerifySBOM(sbom, manifest); err != nil {
		t.Fatalf("module SBOM verification: %v", err)
	}
	tamperedSBOM := sbom
	tamperedSBOM.Components = append([]release.SBOMComponent(nil), sbom.Components...)
	tamperedSBOM.Components[len(tamperedSBOM.Components)-1].PURL = "pkg:golang/foreign.example/module@v1.0.0"
	if err := release.VerifySBOM(tamperedSBOM, manifest); err == nil {
		t.Fatal("forged module SBOM accepted")
	}
	artifact, err := release.BuildArtifact(root, filepath.Join(t.TempDir(), "release.zip"), manifest)
	if err != nil || release.VerifyArtifact(artifact, manifest) != nil {
		t.Fatalf("real module archive: %v", err)
	}
	delete(document, "goModules")
	omitted, _ := json.Marshal(document)
	var forged release.Manifest
	if err := json.Unmarshal(omitted, &forged); err != nil {
		t.Fatal(err)
	}
	if err := release.VerifyArtifact(artifact, forged); err == nil {
		t.Fatal("omitted executable dependencies accepted")
	}
	if _, err := release.BuildArtifact(root, filepath.Join(t.TempDir(), "forged.zip"), forged); err == nil {
		t.Fatal("forged dependency-free manifest packaged")
	}
	forged = manifest
	forged.GoModules = append([]release.GoModule(nil), manifest.GoModules...)
	forged.GoModules[0].Sum = "h1:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="
	if err := release.VerifyManifest(forged); err != nil {
		t.Fatalf("checksum fixture must be structurally valid: %v", err)
	}
	if err := release.VerifyArtifact(artifact, forged); err == nil {
		t.Fatal("module checksum disconnected from executable accepted")
	}
}

func TestDependencyFreeFixtureManifestKeepsLegacySerialization(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root)
	manifest := testManifest(t, root)
	raw, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "goModules") {
		t.Fatalf("empty metadata changes old manifest: %s", raw)
	}
	sbom, err := release.BuildSBOM(manifest)
	if err != nil || len(sbom.Components) != len(manifest.Files) || release.VerifySBOM(sbom, manifest) != nil {
		t.Fatalf("old fixture rejected: %#v %v", sbom, err)
	}
}
