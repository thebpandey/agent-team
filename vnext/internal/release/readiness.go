package release

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"

	"github.com/thebpandey/agent-team/vnext/internal/core"
)

type Gates struct {
	Phase1, Phase2, Phase3, Phase4, Phase5 bool
	Deploy, Install, ReviewsClean          bool
	Native, Benchmark, Artifacts           bool
	Canaries, Rollback, Provider, Cutover  bool
}

func (g Gates) Validate() error {
	if !g.Phase1 || !g.Phase2 || !g.Phase3 || !g.Phase4 || !g.Phase5 || !g.Deploy || !g.Install || !g.ReviewsClean || !g.Native || !g.Benchmark || !g.Artifacts || !g.Canaries || !g.Rollback || !g.Provider || !g.Cutover {
		return core.ErrPhase
	}
	return nil
}

type EvidencePointer struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

type ReadinessEvidence struct {
	Revision  string          `json:"revision"`
	Gates     Gates           `json:"gates"`
	Benchmark EvidencePointer `json:"benchmark"`
	Artifact  EvidencePointer `json:"artifact"`
	SBOM      EvidencePointer `json:"sbom"`
	Canary    EvidencePointer `json:"canary"`
	Rollback  EvidencePointer `json:"rollback"`
	Provider  EvidencePointer `json:"provider"`
	Installed EvidencePointer `json:"installed"`
}

func BuildReadinessEvidence(revision, gatesPath string, paths map[string]string) (ReadinessEvidence, error) {
	if ValidatePackageArgs("0.0.0", revision) != nil {
		return ReadinessEvidence{}, core.ErrPhase
	}
	raw, err := os.ReadFile(gatesPath)
	if err != nil {
		return ReadinessEvidence{}, core.ErrPhase
	}
	var gates Gates
	if json.Unmarshal(raw, &gates) != nil || gates.Validate() != nil {
		return ReadinessEvidence{}, core.ErrPhase
	}
	seen := map[string]bool{}
	get := func(name string) (EvidencePointer, error) {
		path, ok := paths[name]
		if !ok || seen[path] {
			return EvidencePointer{}, core.ErrPhase
		}
		seen[path] = true
		return hashEvidence(path)
	}
	benchmark, err := get("benchmark")
	if err != nil {
		return ReadinessEvidence{}, err
	}
	artifact, err := get("artifact")
	if err != nil {
		return ReadinessEvidence{}, err
	}
	sbom, err := get("sbom")
	if err != nil {
		return ReadinessEvidence{}, err
	}
	canary, err := get("canary")
	if err != nil {
		return ReadinessEvidence{}, err
	}
	rollback, err := get("rollback")
	if err != nil {
		return ReadinessEvidence{}, err
	}
	provider, err := get("provider")
	if err != nil {
		return ReadinessEvidence{}, err
	}
	installed, err := get("installed")
	if err != nil {
		return ReadinessEvidence{}, err
	}
	return ReadinessEvidence{Revision: revision, Gates: gates, Benchmark: benchmark, Artifact: artifact, SBOM: sbom, Canary: canary, Rollback: rollback, Provider: provider, Installed: installed}, nil
}

func WriteReadinessEvidence(path string, evidence ReadinessEvidence) error {
	if evidence.Revision == "" || evidence.Gates.Validate() != nil {
		return core.ErrPhase
	}
	raw, err := json.MarshalIndent(evidence, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(raw, '\n'), 0o644)
}

func VerifyReadinessEvidence(path, revision string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return core.ErrPhase
	}
	var evidence ReadinessEvidence
	if json.Unmarshal(raw, &evidence) != nil || evidence.Revision != revision || evidence.Gates.Validate() != nil {
		return core.ErrPhase
	}
	seen := map[string]bool{}
	for _, pointer := range []EvidencePointer{evidence.Benchmark, evidence.Artifact, evidence.SBOM, evidence.Canary, evidence.Rollback, evidence.Provider, evidence.Installed} {
		if pointer.Path == "" || pointer.SHA256 == "" || seen[pointer.Path] {
			return core.ErrPhase
		}
		seen[pointer.Path] = true
		current, err := hashEvidence(pointer.Path)
		if err != nil || current.SHA256 != pointer.SHA256 {
			return core.ErrPhase
		}
	}
	return nil
}

func hashEvidence(path string) (EvidencePointer, error) {
	if path == "" {
		return EvidencePointer{}, core.ErrPhase
	}
	raw, err := os.ReadFile(path)
	if err != nil || len(raw) == 0 {
		return EvidencePointer{}, core.ErrPhase
	}
	sum := sha256.Sum256(raw)
	return EvidencePointer{Path: path, SHA256: hex.EncodeToString(sum[:])}, nil
}
