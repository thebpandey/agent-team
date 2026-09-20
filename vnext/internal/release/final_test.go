package release_test

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/thebpandey/agent-team/vnext/internal/core"
	"github.com/thebpandey/agent-team/vnext/internal/release"
)

func TestFinalGate(t *testing.T) {
	gates := release.Gates{Phase1: true, Phase2: true, Phase3: true, Phase4: true, Phase5: true, Deploy: true, Install: true, ReviewsClean: true, Native: true, Benchmark: true, Artifacts: true, Canaries: true, Rollback: true, Provider: true, Cutover: true}
	if err := gates.Validate(); err != nil {
		t.Fatal(err)
	}
	gates.Cutover = false
	if err := gates.Validate(); !errors.Is(err, core.ErrPhase) {
		t.Fatal(err)
	}
}

func TestReadinessEvidenceDigests(t *testing.T) {
	root := t.TempDir()
	gates := release.Gates{Phase1: true, Phase2: true, Phase3: true, Phase4: true, Phase5: true, Deploy: true, Install: true, ReviewsClean: true, Native: true, Benchmark: true, Artifacts: true, Canaries: true, Rollback: true, Provider: true, Cutover: true}
	gatePath := filepath.Join(root, "gates.json")
	raw, _ := json.Marshal(gates)
	if err := os.WriteFile(gatePath, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	paths := map[string]string{}
	for _, name := range []string{"benchmark", "artifact", "sbom", "canary", "rollback", "provider", "installed"} {
		path := filepath.Join(root, name+".json")
		if err := os.WriteFile(path, []byte(name), 0o644); err != nil {
			t.Fatal(err)
		}
		paths[name] = path
	}
	revision := strings.Repeat("a", 40)
	evidence, err := release.BuildReadinessEvidence(revision, gatePath, paths)
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(root, "readiness.json")
	if err := release.WriteReadinessEvidence(out, evidence); err != nil || release.VerifyReadinessEvidence(out, revision) != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(paths["artifact"], []byte("changed"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !errors.Is(release.VerifyReadinessEvidence(out, revision), core.ErrPhase) {
		t.Fatal("changed evidence accepted")
	}
}
