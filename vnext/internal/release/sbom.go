package release

import (
	"path/filepath"

	"github.com/thebpandey/agent-team/vnext/internal/core"
)

type SBOMComponent struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Path    string `json:"path"`
	SHA256  string `json:"sha256"`
	Type    string `json:"type"`
	BOMRef  string `json:"bom-ref"`
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
	return sbom, nil
}

func VerifySBOM(sbom SBOM, manifest Manifest) error {
	if err := validateManifest(manifest); err != nil {
		return core.ErrRevision
	}
	if sbom.Format != "CycloneDX" || sbom.SpecVersion != "1.5" || sbom.Tool != "native" || sbom.Serial != "urn:agent-team:"+ManifestSHA256(manifest) || len(sbom.Components) != len(manifest.Files) {
		return core.ErrRevision
	}
	for index, component := range sbom.Components {
		path := manifest.Files[index]
		digest := manifest.Checksums[path]
		if component.Path != path || component.Name != filepath.Base(path) || component.Version != manifest.Version || component.SHA256 != digest || component.Type != "file" || component.BOMRef != "sha256:"+digest {
			return core.ErrRevision
		}
	}
	return nil
}
