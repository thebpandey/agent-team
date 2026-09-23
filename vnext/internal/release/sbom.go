package release

import (
	"path/filepath"
	"reflect"

	"github.com/thebpandey/agent-team/vnext/internal/core"
)

type SBOMComponent struct {
	Name       string         `json:"name"`
	Version    string         `json:"version,omitempty"`
	Path       string         `json:"path,omitempty"`
	SHA256     string         `json:"sha256,omitempty"`
	Type       string         `json:"type"`
	BOMRef     string         `json:"bom-ref"`
	PURL       string         `json:"purl,omitempty"`
	Properties []SBOMProperty `json:"properties,omitempty"`
}

type SBOMProperty struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type SBOM struct {
	Format      string          `json:"bomFormat"`
	SpecVersion string          `json:"specVersion"`
	Serial      string          `json:"serialNumber"`
	Tool        string          `json:"tool"`
	Components  []SBOMComponent `json:"components"`
}

func BuildSBOM(manifest Manifest) (SBOM, error) {
	if err := validateManifest(manifest); err != nil {
		return SBOM{}, err
	}
	sbom := SBOM{Format: "CycloneDX", SpecVersion: "1.5", Serial: "urn:agent-team:" + ManifestSHA256(manifest), Tool: "native"}
	for _, path := range manifest.Files {
		digest := manifest.Checksums[path]
		sbom.Components = append(sbom.Components, SBOMComponent{Name: filepath.Base(path), Version: manifest.Version, Path: path, SHA256: digest, Type: "file", BOMRef: "sha256:" + digest})
	}
	for _, module := range manifest.GoModules {
		sbom.Components = append(sbom.Components, moduleComponent(module))
	}
	return sbom, nil
}

func VerifySBOM(sbom SBOM, manifest Manifest) error {
	expected, err := BuildSBOM(manifest)
	if err != nil || !reflect.DeepEqual(sbom, expected) {
		return core.ErrRevision
	}
	return nil
}
