package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestVerifyGatesCLI(t *testing.T) {
	if _, _, err := parseVerifyGates(nil); err == nil {
		t.Fatal("missing evidence accepted")
	}
	evidence, revision, err := parseVerifyGates([]string{"--evidence", "readiness.json", "--revision", "abc123"})
	if err != nil || evidence != "readiness.json" || revision != "abc123" {
		t.Fatal(evidence, revision, err)
	}
	if _, _, err := parseVerifyGates([]string{"--evidence", "readiness.json", "--revision", "abc123", "extra"}); err == nil {
		t.Fatal("positional extra accepted")
	}
}

func TestEvidenceCLIParsers(t *testing.T) {
	if _, _, _, _, err := parseCollectEvidence([]string{"--revision", "r", "--out", "e", "--native", "n", "--benchmark", "b", "extra"}); err == nil {
		t.Fatal("collect extra accepted")
	}
	revision, output, native, benchmark, err := parseCollectEvidence([]string{"--revision", "r", "--out", "e", "--native", "n", "--benchmark", "b"})
	if err != nil || revision != "r" || output != "e" || native != "n" || benchmark != "b" {
		t.Fatal(revision, output, native, benchmark, err)
	}
	if _, _, _, _, _, _, _, _, err := parseFinalizeEvidence([]string{"--revision", "r", "--out", "e"}); err == nil {
		t.Fatal("finalize missing accepted")
	}
	args := []string{"--revision", "r", "--out", "e", "--artifact", "a", "--sbom", "s", "--canary", "c", "--rollback", "rb", "--provider", "p", "--installed", "i"}
	revision, output, artifact, sbom, canary, rollback, provider, installed, err := parseFinalizeEvidence(args)
	if err != nil || revision != "r" || output != "e" || artifact != "a" || sbom != "s" || canary != "c" || rollback != "rb" || provider != "p" || installed != "i" {
		t.Fatal(revision, output, artifact, sbom, canary, rollback, provider, installed, err)
	}
}

func TestReviewEvidenceRejectsForgedMissingAndStale(t *testing.T) {
	path := filepath.Join(t.TempDir(), "phase1-review.json")
	if err := os.WriteFile(path, []byte(`{"Phase":"phase1","Revision":"r","Passed":true}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := validateReviewEvidence(path, "r"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"Phase":"phase1","Revision":"r","Passed":false}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := validateReviewEvidence(path, "r"); err == nil {
		t.Fatal("forged review accepted")
	}
	if err := validateReviewEvidence(filepath.Join(filepath.Dir(path), "missing.json"), "r"); err == nil {
		t.Fatal("missing review accepted")
	}
	if err := os.WriteFile(path, []byte(`{"Phase":"phase1","Revision":"old","Passed":true}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := validateReviewEvidence(path, "r"); err == nil {
		t.Fatal("stale review accepted")
	}
}

func TestCheckEvidenceRejectsForgedAndStale(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "native-test.json")
	if err := os.WriteFile(source, []byte("passing evidence"), 0o644); err != nil {
		t.Fatal(err)
	}
	digest, err := readPassingTest(source)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "native.json")
	good := `{"Phase":"phase1","Revision":"r","Kind":"native","Source":"` + source + `","Hash":"` + digest + `","Passed":true}`
	if err := os.WriteFile(path, []byte(good), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := validateCheckEvidence(path, "r", "native"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(source, []byte("changed"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := validateCheckEvidence(path, "r", "native"); err == nil {
		t.Fatal("changed source accepted")
	}
}
