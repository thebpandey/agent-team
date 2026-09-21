package release

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
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

func WindowsBundleOutputs(version string) []string {
	bundle := "agent-teamctl-" + version + "-windows-amd64.zip"
	return []string{bundle, bundle + ".sha256"}
}

func PackageOutputs(version string) []string {
	return append(append([]string(nil), ReleaseOutputs(version)...), WindowsBundleOutputs(version)...)
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
	return buildReleasePackageFrom(source, root, version, commit, "agent-teamctl")
}

func buildReleasePackageFrom(source, root, version, commit, executable string) error {
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
	files := []string{executable, "WORKER-CONTRACT", "codex/SKILL.md", "claude/SKILL.md", "VERSION"}
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

// BuildWindowsBundleFrom creates the one-file Windows download. Its outer checksum
// stays outside the extracted distribution, so the legacy strict Linux layout is unchanged.
func BuildWindowsBundleFrom(source, root, version, commit string) error {
	if err := ValidatePackageArgs(version, commit); err != nil {
		return err
	}
	staging, err := os.MkdirTemp("", "agent-team-windows-release-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(staging)
	if err := buildReleasePackageFrom(source, staging, version, commit, "agent-teamctl.exe"); err != nil {
		return err
	}
	manifestRaw, err := os.ReadFile(filepath.Join(staging, "RELEASE.json"))
	if err != nil {
		return err
	}
	var manifest Manifest
	if strictJSON(manifestRaw, &manifest) != nil || VerifyManifest(manifest) != nil || manifest.Executable != "agent-teamctl.exe" {
		return core.ErrRevision
	}
	bundle := "agent-teamctl-" + version + "-windows-amd64.zip"
	bundlePath := filepath.Join(root, bundle)
	members := map[string]string{}
	for _, name := range ReleaseOutputs(version) {
		members[name] = filepath.Join(staging, name)
	}
	for _, name := range manifest.Files {
		members[name] = filepath.Join(source, filepath.FromSlash(name))
	}
	if err := writeBundleZip(bundlePath, members); err != nil {
		return err
	}
	raw, err := os.ReadFile(bundlePath)
	if err != nil {
		return err
	}
	sidecar := filepath.Join(root, bundle+".sha256")
	if err := os.WriteFile(sidecar, []byte(sha256Hex(raw)+"  "+bundle+"\n"), 0o644); err != nil {
		return err
	}
	return VerifyWindowsBundle(bundlePath, sidecar, version, commit)
}

func writeBundleZip(output string, members map[string]string) error {
	if err := os.MkdirAll(filepath.Dir(output), 0o755); err != nil {
		return err
	}
	file, err := os.Create(output)
	if err != nil {
		return err
	}
	archive := zip.NewWriter(file)
	names := make([]string, 0, len(members))
	for name := range members {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		body, readErr := os.ReadFile(members[name])
		if readErr != nil {
			archive.Close()
			file.Close()
			return readErr
		}
		header := &zip.FileHeader{Name: name, Method: zip.Store}
		header.SetModTime(archiveEpoch)
		header.SetMode(0o644)
		writer, writeErr := archive.CreateHeader(header)
		if writeErr != nil {
			archive.Close()
			file.Close()
			return writeErr
		}
		if _, writeErr = writer.Write(body); writeErr != nil {
			archive.Close()
			file.Close()
			return writeErr
		}
	}
	if err := archive.Close(); err != nil {
		file.Close()
		return err
	}
	return file.Close()
}

func VerifyWindowsBundle(bundlePath, sidecarPath, version, commit string) error {
	bundle := "agent-teamctl-" + version + "-windows-amd64.zip"
	if filepath.Base(bundlePath) != bundle || filepath.Base(sidecarPath) != bundle+".sha256" {
		return core.ErrPath
	}
	raw, err := os.ReadFile(bundlePath)
	if err != nil {
		return core.ErrPath
	}
	sidecar, err := os.ReadFile(sidecarPath)
	if err != nil || string(sidecar) != sha256Hex(raw)+"  "+bundle+"\n" {
		return core.ErrRevision
	}
	archive, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		return core.ErrRevision
	}
	files := make(map[string][]byte, len(archive.File))
	for _, entry := range archive.File {
		if _, duplicate := files[entry.Name]; duplicate || entry.Mode().Perm() != 0o644 || !entry.Modified.UTC().Equal(archiveEpoch) {
			return core.ErrRevision
		}
		reader, openErr := entry.Open()
		if openErr != nil {
			return core.ErrRevision
		}
		body, readErr := io.ReadAll(reader)
		closeErr := reader.Close()
		if readErr != nil || closeErr != nil {
			return core.ErrRevision
		}
		files[entry.Name] = body
	}
	var manifest Manifest
	if strictJSON(files["RELEASE.json"], &manifest) != nil || VerifyManifest(manifest) != nil || manifest.Version != version || manifest.Commit != commit || manifest.Executable != "agent-teamctl.exe" {
		return core.ErrRevision
	}
	wantManifest := []string{"WORKER-CONTRACT", "VERSION", "agent-teamctl.exe", "claude/SKILL.md", "codex/SKILL.md"}
	sort.Strings(wantManifest)
	if !reflect.DeepEqual(manifest.Files, wantManifest) {
		return core.ErrRevision
	}
	for _, name := range manifest.Files {
		if sha256Hex(files[name]) != manifest.Checksums[name] {
			return core.ErrRevision
		}
	}
	want := append(append([]string(nil), ReleaseOutputs(version)...), manifest.Files...)
	sort.Strings(want)
	if len(archive.File) != len(want) {
		return core.ErrRevision
	}
	for index, entry := range archive.File {
		if entry.Name != want[index] {
			return core.ErrRevision
		}
	}
	var sbom SBOM
	if strictJSON(files["SBOM.cdx.json"], &sbom) != nil || VerifySBOM(sbom, manifest) != nil || verifyChecksums(files["SHA256SUMS"], version, files) != nil {
		return core.ErrRevision
	}
	inner, err := zip.NewReader(bytes.NewReader(files["agent-teamctl-"+version+".zip"]), int64(len(files["agent-teamctl-"+version+".zip"])))
	if err != nil || verifyArchiveFiles(inner.File, manifest) != nil {
		return core.ErrRevision
	}
	return nil
}

func strictJSON(raw []byte, value any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return core.ErrRevision
	}
	return nil
}

func verifyChecksums(raw []byte, version string, files map[string][]byte) error {
	want := []string{"RELEASE.json", "SBOM.cdx.json", "agent-teamctl-" + version + ".zip"}
	sort.Strings(want)
	lines := strings.Split(strings.TrimSuffix(string(raw), "\n"), "\n")
	if len(lines) != len(want) {
		return core.ErrRevision
	}
	for index, line := range lines {
		if len(line) != 66+len(want[index]) || line[64:66] != "  " || line[66:] != want[index] || line[:64] != sha256Hex(files[want[index]]) {
			return core.ErrRevision
		}
	}
	return nil
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

func VerifyPackageOutputs(root, version, commit string) error {
	entries, err := os.ReadDir(root)
	if err != nil {
		return err
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
			return core.ErrPath
		}
		names = append(names, entry.Name())
	}
	want := PackageOutputs(version)
	sort.Strings(names)
	sort.Strings(want)
	if !reflect.DeepEqual(names, want) {
		return core.ErrPath
	}
	files := make(map[string][]byte, len(ReleaseOutputs(version)))
	for _, name := range ReleaseOutputs(version) {
		raw, readErr := os.ReadFile(filepath.Join(root, name))
		if readErr != nil {
			return core.ErrPath
		}
		files[name] = raw
	}
	var manifest Manifest
	if strictJSON(files["RELEASE.json"], &manifest) != nil || VerifyManifest(manifest) != nil || manifest.Version != version || manifest.Commit != commit || manifest.Executable != "agent-teamctl" {
		return core.ErrRevision
	}
	var sbom SBOM
	if strictJSON(files["SBOM.cdx.json"], &sbom) != nil || VerifySBOM(sbom, manifest) != nil || verifyChecksums(files["SHA256SUMS"], version, files) != nil {
		return core.ErrRevision
	}
	archive := files["agent-teamctl-"+version+".zip"]
	if VerifyArtifact(Artifact{Path: filepath.Join(root, "agent-teamctl-"+version+".zip"), SHA256: sha256Hex(archive), Bytes: int64(len(archive))}, manifest) != nil {
		return core.ErrRevision
	}
	bundle := filepath.Join(root, "agent-teamctl-"+version+"-windows-amd64.zip")
	return VerifyWindowsBundle(bundle, bundle+".sha256", version, commit)
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
