package main

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/thebpandey/agent-team/vnext/internal/core"
	"github.com/thebpandey/agent-team/vnext/internal/release"
)

type CheckEvidence struct {
	Phase, Revision, Kind, Source, Hash string
	Passed                              bool
}

func parseCollectEvidence(args []string) (string, string, string, string, error) {
	set := flag.NewFlagSet("collect-evidence", flag.ContinueOnError)
	set.SetOutput(os.Stderr)
	revision, output := set.String("revision", "", ""), set.String("out", "", "")
	native, benchmark := set.String("native", "", ""), set.String("benchmark", "", "")
	if err := set.Parse(args); err != nil || set.NArg() != 0 || *revision == "" || *output == "" || *native == "" || *benchmark == "" {
		return "", "", "", "", fmt.Errorf("collect-evidence requires revision, out, native, and benchmark")
	}
	return *revision, *output, *native, *benchmark, nil
}

func parseFinalizeEvidence(args []string) (string, string, string, string, string, string, string, string, error) {
	set := flag.NewFlagSet("finalize-evidence", flag.ContinueOnError)
	set.SetOutput(os.Stderr)
	revision, output := set.String("revision", "", ""), set.String("out", "", "")
	artifact, sbom := set.String("artifact", "", ""), set.String("sbom", "", "")
	canary, rollback := set.String("canary", "", ""), set.String("rollback", "", "")
	provider, installed := set.String("provider", "", ""), set.String("installed", "", "")
	if err := set.Parse(args); err != nil || set.NArg() != 0 || *revision == "" || *output == "" || *artifact == "" || *sbom == "" || *canary == "" || *rollback == "" || *provider == "" || *installed == "" {
		return "", "", "", "", "", "", "", "", fmt.Errorf("finalize-evidence requires revision, out, artifact, sbom, canary, rollback, provider, and installed")
	}
	return *revision, *output, *artifact, *sbom, *canary, *rollback, *provider, *installed, nil
}

func parseReadiness(args []string) (string, string, map[string]string, error) {
	set := flag.NewFlagSet("write-readiness", flag.ContinueOnError)
	set.SetOutput(os.Stderr)
	revision, gates := set.String("revision", "", ""), set.String("gates", "", "")
	names := []string{"benchmark", "artifact", "sbom", "canary", "rollback", "provider", "installed"}
	values := map[string]*string{}
	for _, name := range names {
		values[name] = set.String(name, "", "")
	}
	if err := set.Parse(args); err != nil || set.NArg() != 0 || *revision == "" || *gates == "" {
		return "", "", nil, fmt.Errorf("write-readiness requires revision, gates, and all evidence paths")
	}
	paths := map[string]string{}
	for _, name := range names {
		if *values[name] == "" {
			return "", "", nil, fmt.Errorf("missing --%s", name)
		}
		paths[name] = *values[name]
	}
	return *revision, *gates, paths, nil
}

func collectEvidence(revision, output, native, benchmark string) error {
	if err := os.MkdirAll(output, 0o755); err != nil {
		return err
	}
	if err := writeTestCheck(filepath.Join(output, "native.json"), revision, "phase", "native", native); err != nil {
		return err
	}
	return writeTestCheck(filepath.Join(output, "benchmark.json"), revision, "phase", "benchmark", benchmark)
}

func finalizeEvidence(revision, output, artifact, sbom, canary, rollback, provider, installed string) error {
	if revision == "" || output == "" {
		return core.ErrPhase
	}
	if err := validateCheckEvidence(filepath.Join(output, "native.json"), revision, "native"); err != nil {
		return err
	}
	if err := validateCheckEvidence(filepath.Join(output, "benchmark.json"), revision, "benchmark"); err != nil {
		return err
	}
	for _, name := range []string{"phase1-review.json", "phase2-review.json", "phase3-review.json", "phase4-review.json", "phase5-review.json"} {
		if err := validateReviewEvidence(filepath.Join(output, name), revision); err != nil {
			return err
		}
	}
	if err := validateManifestEvidence(artifact, revision); err != nil {
		return err
	}
	if err := validateSBOMEvidence(sbom, artifact); err != nil {
		return err
	}
	if err := writeFileCheck(filepath.Join(output, "artifact.json"), revision, "artifact", artifact); err != nil {
		return err
	}
	if err := writeFileCheck(filepath.Join(output, "sbom.json"), revision, "sbom", sbom); err != nil {
		return err
	}
	for _, input := range []struct{ kind, path string }{{"canary", canary}, {"rollback", rollback}, {"provider", provider}, {"installed-skill", installed}} {
		if err := writeTestCheck(filepath.Join(output, outputName(input.kind)+".json"), revision, "phase6", input.kind, input.path); err != nil {
			return err
		}
	}
	gates := release.Gates{Phase1: true, Phase2: true, Phase3: true, Phase4: true, Phase5: true, Deploy: true, Install: true, ReviewsClean: true, Native: true, Benchmark: true, Artifacts: true, Canaries: true, Rollback: true, Provider: true, Cutover: true}
	raw, err := json.Marshal(gates)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(output, "release-gates.json"), append(raw, '\n'), 0o644)
}

func writeTestCheck(path, revision, phase, kind, source string) error {
	if err := requireNamedTests(source, kind); err != nil {
		return err
	}
	return writeFileCheckWithPhase(path, revision, phase, kind, source)
}

func writeFileCheck(path, revision, kind, source string) error {
	return writeFileCheckWithPhase(path, revision, "phase6", kind, source)
}

func writeFileCheckWithPhase(path, revision, phase, kind, source string) error {
	digest, err := readPassingTest(source)
	if err != nil {
		return err
	}
	raw, err := json.Marshal(CheckEvidence{Phase: phase, Revision: revision, Kind: kind, Source: source, Hash: digest, Passed: true})
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(raw, '\n'), 0o644)
}

func readPassingTest(path string) (string, error) {
	raw, err := os.ReadFile(path)
	if err != nil || len(raw) == 0 || bytes.Contains(raw, []byte(`"Action":"fail"`)) {
		return "", core.ErrPhase
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

func requireNamedTests(path, kind string) error {
	file, err := os.Open(path)
	if err != nil {
		return core.ErrPhase
	}
	defer file.Close()
	passed := map[string]bool{}
	scanner := bufio.NewScanner(file)
	buffer := make([]byte, 64<<10)
	scanner.Buffer(buffer, 1<<20)
	for scanner.Scan() {
		var event struct{ Action, Test string }
		if json.Unmarshal(scanner.Bytes(), &event) == nil && event.Action == "pass" && event.Test != "" {
			passed[event.Test] = true
		}
	}
	if scanner.Err() != nil {
		return core.ErrPhase
	}
	for _, name := range requiredEvidenceTests(kind) {
		if !passed[name] {
			return core.ErrPhase
		}
	}
	return nil
}

func requiredEvidenceTests(kind string) []string {
	switch kind {
	case "benchmark":
		return []string{"TestReleaseReport"}
	case "canary":
		return []string{"TestCodexCanary", "TestClaudeCanary", "TestPackagedArchiveCanary", "TestCutover"}
	case "rollback":
		return []string{"TestBothHostsRollbackAndActions"}
	case "provider":
		return []string{"TestProviderVerification"}
	case "installed-skill":
		return []string{"TestPackagedArchiveCanary"}
	default:
		return nil
	}
}

func validateManifestEvidence(path, revision string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var manifest release.Manifest
	if json.Unmarshal(raw, &manifest) != nil || manifest.Version == "" || manifest.Commit != revision || len(manifest.Checksums) == 0 {
		return core.ErrRevision
	}
	return nil
}

func validateSBOMEvidence(path, manifestPath string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	manifestRaw, err := os.ReadFile(manifestPath)
	if err != nil {
		return err
	}
	var sbom release.SBOM
	var manifest release.Manifest
	if json.Unmarshal(manifestRaw, &manifest) != nil || json.Unmarshal(raw, &sbom) != nil || release.VerifySBOM(sbom, manifest) != nil {
		return core.ErrRevision
	}
	return nil
}

func validateCheckEvidence(path, revision, kind string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var check CheckEvidence
	if json.Unmarshal(raw, &check) != nil || check.Revision != revision || check.Kind != kind || !check.Passed || check.Hash == "" || check.Source == "" {
		return core.ErrPhase
	}
	digest, err := readPassingTest(check.Source)
	if err != nil || digest != check.Hash {
		return core.ErrPhase
	}
	return nil
}

func validateReviewEvidence(path, revision string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var review struct {
		Phase, Revision string
		Passed          bool
	}
	if json.Unmarshal(raw, &review) != nil || review.Revision != revision || !review.Passed || review.Phase == "" {
		return core.ErrPhase
	}
	return nil
}

func outputName(kind string) string {
	if kind == "installed-skill" {
		return "installed-skill"
	}
	return kind
}
