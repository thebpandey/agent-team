package main

import (
	"archive/zip"
	"crypto/sha256"
	"debug/pe"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/thebpandey/agent-team/vnext/internal/release"
)

func TestBuildAgentTeamctlWindowsAMD64(t *testing.T) {
	output := filepath.Join(t.TempDir(), "agent-teamctl.exe")
	if err := buildAgentTeamctlFor(filepath.Clean(filepath.Join("..", "..")), output, "1.0.0", "0123456789abcdef0123456789abcdef01234567", "windows", "amd64"); err != nil {
		t.Fatal(err)
	}
	image, err := pe.Open(output)
	if err != nil || image.FileHeader.Machine != pe.IMAGE_FILE_MACHINE_AMD64 {
		t.Fatalf("Windows amd64 binary = %#v, %v", image, err)
	}
}

func TestPackageCommandBuildsWindowsBundle(t *testing.T) {
	workingDirectory, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(filepath.Clean(filepath.Join("..", ".."))); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(workingDirectory) })
	for _, name := range []string{"agent-teamctl", "agent-teamctl.exe"} {
		if _, err := os.Lstat(name); err == nil {
			t.Skipf("%s already exists", name)
		} else if !os.IsNotExist(err) {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		for _, name := range []string{"agent-teamctl", "agent-teamctl.exe"} {
			if err := os.Remove(name); err != nil && !os.IsNotExist(err) {
				t.Error(err)
			}
		}
	})
	versionRaw, err := os.ReadFile("VERSION")
	if err != nil {
		t.Fatal(err)
	}
	version := strings.TrimSpace(string(versionRaw))
	output := t.TempDir()
	commit := "0123456789abcdef0123456789abcdef01234567"
	if code := run([]string{"package", "--version", version, "--commit", commit, "--output", output}); code != 0 {
		t.Fatalf("package command exit code = %d", code)
	}
	if err := release.VerifyPackageOutputs(output, version, commit); err != nil {
		t.Fatal(err)
	}
	bundle, err := zip.OpenReader(filepath.Join(output, "agent-teamctl-"+version+"-windows-amd64.zip"))
	if err != nil {
		t.Fatal(err)
	}
	defer bundle.Close()
	for _, member := range bundle.File {
		if member.Name != "agent-teamctl.exe" {
			continue
		}
		reader, openErr := member.Open()
		if openErr != nil {
			t.Fatal(openErr)
		}
		body, readErr := io.ReadAll(reader)
		_ = reader.Close()
		if readErr != nil {
			t.Fatal(readErr)
		}
		path := filepath.Join(t.TempDir(), "agent-teamctl.exe")
		if err := os.WriteFile(path, body, 0o644); err != nil {
			t.Fatal(err)
		}
		image, err := pe.Open(path)
		if err != nil || image.FileHeader.Machine != pe.IMAGE_FILE_MACHINE_AMD64 {
			t.Fatalf("packaged Windows executable = %#v, %v", image, err)
		}
		return
	}
	t.Fatal("Windows executable missing from package command bundle")
}

func TestBuildAgentTeamctlEmbedsReleaseIdentity(t *testing.T) {
	name := "agent-teamctl"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	output := filepath.Join(t.TempDir(), name)
	revision := strings.Repeat("a", 40)
	if err := buildAgentTeamctl(filepath.Clean(filepath.Join("..", "..")), output, "8.0.0", revision); err != nil {
		t.Fatal(err)
	}
	raw, err := exec.Command(output, "version", "--json").Output()
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Version  string `json:"version"`
		Revision string `json:"revision"`
	}
	if err := json.Unmarshal(raw, &got); err != nil || got.Version != "8.0.0" || got.Revision != revision {
		t.Fatalf("packaged identity = %+v, %v", got, err)
	}
}

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
	root := t.TempDir()
	source := writeEvents(t, root, "review.json", []string{"TestReview"}, "")
	digest, err := readPassingTest(source)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "phase1-review.json")
	good := ReviewEvidence{Phase: "phase1", Revision: "r", Author: "builder", Reviewer: "reviewer", Source: source, Digest: digest, Result: "CLEAN"}
	writeJSONTest(t, path, good)
	if err := validateReviewEvidence(path, "r", "phase1"); err != nil {
		t.Fatal(err)
	}
	if err := validateReviewEvidence(filepath.Join(root, "missing.json"), "r", "phase1"); err == nil {
		t.Fatal("missing review accepted")
	}
	cases := map[string]func(*ReviewEvidence){
		"wrong revision": func(v *ReviewEvidence) { v.Revision = "old" },
		"wrong phase":    func(v *ReviewEvidence) { v.Phase = "phase2" },
		"self review":    func(v *ReviewEvidence) { v.Reviewer = v.Author },
		"missing source": func(v *ReviewEvidence) { v.Source = "" },
		"bad digest":     func(v *ReviewEvidence) { v.Digest = string(make([]byte, 64)) },
		"not clean":      func(v *ReviewEvidence) { v.Result = "FIX" },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			candidate := good
			mutate(&candidate)
			writeJSONTest(t, path, candidate)
			if err := validateReviewEvidence(path, "r", "phase1"); err == nil {
				t.Fatal("forged review accepted")
			}
		})
	}
}

func TestCheckEvidenceRejectsForgedAndStale(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "native-test.json")
	writeEvents(t, root, "native-test.json", []string{"TestNative"}, "")
	digest, err := readPassingTest(source)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "native.json")
	sourceJSON, _ := json.Marshal(source)
	good := `{"Phase":"phase","Revision":"r","Kind":"native","Source":` + string(sourceJSON) + `,"Hash":"` + digest + `","Passed":true}`
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

func TestPassingTestRequiresStructuredSuccessfulTestAndPackageEvents(t *testing.T) {
	root := t.TempDir()
	for name, body := range map[string]string{
		"arbitrary": `passing evidence`,
		"zero":      `{"Action":"pass","Package":"example"}` + "\n",
		"failure":   `{"Action":"pass","Package":"example","Test":"TestOne"}` + "\n" + `{"Action":"fail","Package":"example"}` + "\n",
		"unknown":   `{"Action":"invented","Package":"example","Test":"TestOne"}` + "\n",
	} {
		path := filepath.Join(root, name+".json")
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := readPassingTest(path); err == nil {
			t.Fatalf("%s evidence accepted", name)
		}
	}
	valid := writeEvents(t, root, "valid.json", []string{"TestOne"}, "")
	if _, err := readPassingTest(valid); err != nil {
		t.Fatal(err)
	}
}

func TestFinalizeEvidenceValidCompletePipeline(t *testing.T) {
	root := t.TempDir()
	out := filepath.Join(root, "evidence")
	revision := "0123456789abcdef0123456789abcdef01234567"
	native := writeEvents(t, root, "native.json", []string{"TestNative"}, "")
	benchmark := writeEvents(t, root, "benchmark.json", []string{"TestReleaseReport"}, "")
	phase1 := exactPhase1Evidence(t, root)
	if err := collectEvidence(revision, out, native, benchmark); err != nil {
		t.Fatal(err)
	}
	for index := 1; index <= 5; index++ {
		phase := "phase" + string(rune('0'+index))
		source := writeEvents(t, root, phase+".json", []string{"TestReview"}, "")
		if index == 1 {
			source = phase1
		}
		digest, _ := readPassingTest(source)
		writeJSONTest(t, filepath.Join(out, phase+"-review.json"), ReviewEvidence{Phase: phase, Revision: revision, Author: "builder", Reviewer: "reviewer-" + phase, Source: source, Digest: digest, Result: "CLEAN"})
	}
	component := []byte("binary")
	sum := sha256.Sum256(component)
	digest := hex.EncodeToString(sum[:])
	manifest := release.Manifest{Version: "1.0.0", Commit: revision, SpecRevision: "vnext-8.0.0", Executable: "agent-teamctl", Files: []string{"agent-teamctl"}, Checksums: map[string]string{"agent-teamctl": digest}, SBOMPath: "SBOM.cdx.json", SBOMTool: "native"}
	artifact := filepath.Join(root, "RELEASE.json")
	writeJSONTest(t, artifact, manifest)
	sbom, err := release.BuildSBOM(manifest)
	if err != nil {
		t.Fatal(err)
	}
	sbomPath := filepath.Join(root, "SBOM.cdx.json")
	writeJSONTest(t, sbomPath, sbom)
	canary := writeEvents(t, root, "canary.json", []string{"TestCodexCanary", "TestClaudeCanary", "TestPackagedArchiveCanary", "TestCutover"}, "")
	rollback := writeEvents(t, root, "rollback.json", []string{"TestBothHostsRollbackAndActions"}, "")
	provider := writeEvents(t, root, "provider.json", []string{"TestProviderVerification"}, "")
	installed := writeEvents(t, root, "installed.json", []string{"TestPackagedArchiveCanary"}, "")
	if err := finalizeEvidence(revision, out, artifact, sbomPath, canary, rollback, provider, installed); err != nil {
		t.Fatal(err)
	}
	var gates release.Gates
	raw, _ := os.ReadFile(filepath.Join(out, "release-gates.json"))
	if json.Unmarshal(raw, &gates) != nil || gates.Validate() != nil {
		t.Fatalf("invalid gates: %s", raw)
	}
	readiness, err := release.BuildReadinessEvidence(revision, filepath.Join(out, "release-gates.json"), map[string]string{
		"benchmark": filepath.Join(out, "benchmark.json"), "artifact": artifact, "sbom": sbomPath,
		"canary": filepath.Join(out, "canary.json"), "rollback": filepath.Join(out, "rollback.json"),
		"provider": filepath.Join(out, "provider.json"), "installed": filepath.Join(out, "installed-skill.json"),
	})
	if err != nil {
		t.Fatal(err)
	}
	readinessPath := filepath.Join(root, "release-readiness.json")
	if err := release.WriteReadinessEvidence(readinessPath, readiness); err != nil {
		t.Fatal(err)
	}
	if err := release.VerifyReadinessEvidence(readinessPath, revision); err != nil {
		t.Fatal(err)
	}
}

func exactPhase1Evidence(t *testing.T, root string) string {
	t.Helper()
	path := filepath.Join(root, "core-phase1.json")
	command := exec.Command("go", "test", "-json", "./internal/core", "-count=1")
	command.Dir = filepath.Clean(filepath.Join("..", ".."))
	raw, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("run exact phase 1 evidence command: %v\n%s", err, raw)
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func writeEvents(t *testing.T, root, name string, tests []string, failure string) string {
	t.Helper()
	path := filepath.Join(root, name)
	body := ""
	for _, test := range tests {
		body += `{"Action":"run","Package":"example","Test":"` + test + `"}` + "\n"
		body += `{"Action":"pass","Package":"example","Test":"` + test + `"}` + "\n"
	}
	if failure != "" {
		body += `{"Action":"fail","Package":"example","Test":"` + failure + `"}` + "\n"
	}
	body += `{"Action":"pass","Package":"example"}` + "\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func writeJSONTest(t *testing.T, path string, value any) {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(raw, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
}
