package release

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/thebpandey/agent-team/vnext/internal/core"
)

type Manifest struct {
	Version      string            `json:"version"`
	Commit       string            `json:"commit"`
	SpecRevision string            `json:"specRevision"`
	Executable   string            `json:"executable"`
	Files        []string          `json:"files"`
	Checksums    map[string]string `json:"checksums"`
	SBOMPath     string            `json:"sbomPath"`
	SBOMTool     string            `json:"sbomTool"`
}

type Artifact struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Bytes  int64  `json:"bytes"`
}

type ArchiveFormat string

const ZipArchive ArchiveFormat = "zip-deterministic"

var releasePaths = map[string]bool{
	"README.md": true, "GETTING_STARTED.md": true, "SKILL.md": true,
	"references/DEPENDENCIES.md": true, "agent-teamctl": true, "agent-teamctl.exe": true,
	"WORKER-CONTRACT": true, "codex/SKILL.md": true, "claude/SKILL.md": true,
	"VERSION": true, "vnext/VERSION": true,
}

func BuildManifest(root, version, commit, spec string, files []string) (Manifest, error) {
	if version == "" || len(commit) < 40 || spec == "" || len(files) == 0 {
		return Manifest{}, core.ErrRevision
	}
	sorted := append([]string(nil), files...)
	sort.Strings(sorted)
	executable := "agent-teamctl"
	if contains(sorted, "agent-teamctl.exe") && !contains(sorted, executable) {
		executable = "agent-teamctl.exe"
	}
	m := Manifest{Version: version, Commit: commit, SpecRevision: spec, Executable: executable, Files: sorted, Checksums: map[string]string{}, SBOMPath: "SBOM.cdx.json", SBOMTool: "native"}
	for _, path := range sorted {
		if _, duplicate := m.Checksums[path]; duplicate {
			return Manifest{}, core.ErrPath
		}
		full, err := safeReleasePath(root, path)
		if err != nil {
			return Manifest{}, err
		}
		body, err := os.ReadFile(full)
		if err != nil {
			return Manifest{}, core.ErrPath
		}
		m.Checksums[path] = sha256Hex(body)
	}
	if _, ok := m.Checksums[m.Executable]; !ok {
		return Manifest{}, core.ErrPath
	}
	return m, nil
}

func ManifestSHA256(manifest Manifest) string {
	raw, err := json.Marshal(manifest)
	if err != nil {
		return ""
	}
	return sha256Hex(raw)
}

func safeReleasePath(root, path string) (string, error) {
	clean := filepath.ToSlash(filepath.Clean(filepath.FromSlash(path)))
	if clean != path || filepath.IsAbs(path) || strings.HasPrefix(clean, "../") || !releasePaths[path] {
		return "", core.ErrPath
	}
	base, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", core.ErrPath
	}
	full := filepath.Join(base, filepath.FromSlash(path))
	info, err := os.Lstat(full)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return "", core.ErrPath
	}
	real, err := filepath.EvalSymlinks(full)
	if err != nil {
		return "", core.ErrPath
	}
	relative, err := filepath.Rel(base, real)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", core.ErrPath
	}
	return real, nil
}

func sha256Hex(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
