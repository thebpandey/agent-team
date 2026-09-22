package release

import (
	"archive/zip"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/thebpandey/agent-team/vnext/internal/core"
)

func VerifyArtifact(artifact Artifact, manifest Manifest) error {
	if err := validateManifest(manifest); err != nil {
		return core.ErrRevision
	}
	raw, err := os.ReadFile(artifact.Path)
	if err != nil || int64(len(raw)) != artifact.Bytes || sha256Hex(raw) != artifact.SHA256 {
		return core.ErrRevision
	}
	archive, err := zip.OpenReader(artifact.Path)
	if err != nil {
		return core.ErrRevision
	}
	defer archive.Close()
	return verifyArchiveFiles(archive.File, manifest)
}

func verifyArchiveFiles(files []*zip.File, manifest Manifest) error {
	if len(files) != len(manifest.Files) {
		return core.ErrRevision
	}
	seen := map[string]bool{}
	for index, entry := range files {
		clean := filepath.ToSlash(filepath.Clean(filepath.FromSlash(strings.ReplaceAll(entry.Name, "\\", "/"))))
		if clean != entry.Name || clean == "." || strings.HasPrefix(clean, "../") || filepath.IsAbs(entry.Name) || seen[entry.Name] || entry.Name != manifest.Files[index] {
			return core.ErrRevision
		}
		seen[entry.Name] = true
		mode := os.FileMode(0o644)
		if entry.Name == manifest.Executable {
			mode = 0o755
		}
		if !entry.Modified.UTC().Equal(archiveEpoch) || entry.Mode().Perm() != mode {
			return core.ErrRevision
		}
		reader, openErr := entry.Open()
		if openErr != nil {
			return core.ErrRevision
		}
		body, readErr := io.ReadAll(reader)
		closeErr := reader.Close()
		if readErr != nil || closeErr != nil || sha256Hex(body) != manifest.Checksums[entry.Name] || entry.Name == manifest.Executable && !executableModulesMatch(body, manifest) {
			return core.ErrRevision
		}
	}
	return nil
}
