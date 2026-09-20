package release

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"

	"github.com/thebpandey/agent-team/vnext/internal/core"
)

var semverPattern = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+$`)

func ValidatePackageArgs(version, commit string) error {
	if !semverPattern.MatchString(version) || len(commit) < 40 || len(commit) > 64 {
		return core.ErrRevision
	}
	for _, character := range commit {
		if !(character >= '0' && character <= '9') && !(character >= 'a' && character <= 'f') && !(character >= 'A' && character <= 'F') {
			return core.ErrRevision
		}
	}
	return nil
}

func ReleaseOutputs(version string) []string {
	return []string{"agent-teamctl-" + version + ".zip", "SHA256SUMS", "RELEASE.json", "SBOM.cdx.json"}
}

func VerifyOutputAllowlist(version string, files []string) error {
	want := map[string]bool{}
	for _, file := range ReleaseOutputs(version) {
		want[file] = true
	}
	for _, file := range files {
		if !want[file] {
			return core.ErrPath
		}
	}
	return nil
}

func BuildReleasePackage(root, version, commit string) error {
	return BuildReleasePackageFrom(".", root, version, commit)
}

func BuildReleasePackageFrom(source, root, version, commit string) error {
	if err := ValidatePackageArgs(version, commit); err != nil {
		return err
	}
	versionFile, err := os.ReadFile(filepath.Join(source, "VERSION"))
	if err != nil {
		return core.ErrPath
	}
	if strings.TrimSpace(string(versionFile)) != version {
		return core.ErrRevision
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return err
	}
	files := []string{"agent-teamctl", "WORKER-CONTRACT", "codex/SKILL.md", "claude/SKILL.md", "VERSION"}
	manifest, err := BuildManifest(source, version, commit, "vnext-8.0.0", files)
	if err != nil {
		return err
	}
	artifact, err := BuildArtifact(source, filepath.Join(root, "agent-teamctl-"+version+".zip"), manifest)
	if err != nil || VerifyArtifact(artifact, manifest) != nil {
		return core.ErrRevision
	}
	sbom, err := BuildSBOM(manifest)
	if err != nil || VerifySBOM(sbom, manifest) != nil {
		return core.ErrRevision
	}
	if err := writeJSON(filepath.Join(root, "RELEASE.json"), manifest); err != nil {
		return err
	}
	if err := writeJSON(filepath.Join(root, "SBOM.cdx.json"), sbom); err != nil {
		return err
	}
	if err := writeChecksums(filepath.Join(root, "SHA256SUMS"), root, []string{"agent-teamctl-" + version + ".zip", "RELEASE.json", "SBOM.cdx.json"}); err != nil {
		return err
	}
	return VerifyOutputAllowlistMustEqual(root, version, ReleaseOutputs(version))
}

func ListPackageOutputs(root, version string) ([]string, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			return nil, core.ErrPath
		}
		names = append(names, entry.Name())
	}
	if err := VerifyOutputAllowlist(version, names); err != nil {
		return nil, err
	}
	expected := ReleaseOutputs(version)
	sortedExpected := append([]string(nil), expected...)
	sort.Strings(sortedExpected)
	sort.Strings(names)
	if !reflect.DeepEqual(names, sortedExpected) {
		return nil, core.ErrPath
	}
	return expected, nil
}

func VerifyOutputAllowlistMustEqual(root, version string, want []string) error {
	got, err := ListPackageOutputs(root, version)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(got, want) {
		return core.ErrPath
	}
	return nil
}

func writeJSON(path string, value any) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(raw, '\n'), 0o644)
}

func writeChecksums(path, root string, names []string) error {
	sort.Strings(names)
	lines := make([]string, 0, len(names))
	for _, name := range names {
		body, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			return err
		}
		sum := sha256.Sum256(body)
		lines = append(lines, hex.EncodeToString(sum[:])+"  "+name)
	}
	return os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644)
}
