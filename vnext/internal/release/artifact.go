package release

import (
	"archive/zip"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/thebpandey/agent-team/vnext/internal/core"
)

var archiveEpoch = time.Date(1980, 1, 1, 0, 0, 0, 0, time.UTC)

func BuildArtifact(root, output string, manifest Manifest) (Artifact, error) {
	if err := validateManifest(manifest); err != nil {
		return Artifact{}, err
	}
	if err := os.MkdirAll(filepath.Dir(output), 0o755); err != nil {
		return Artifact{}, err
	}
	file, err := os.Create(output)
	if err != nil {
		return Artifact{}, err
	}
	archive := zip.NewWriter(file)
	for _, path := range manifest.Files {
		full, pathErr := safeReleasePath(root, path)
		if pathErr != nil {
			archive.Close()
			file.Close()
			return Artifact{}, pathErr
		}
		body, readErr := os.ReadFile(full)
		if readErr != nil || sha256Hex(body) != manifest.Checksums[path] {
			archive.Close()
			file.Close()
			return Artifact{}, core.ErrRevision
		}
		header := &zip.FileHeader{Name: path, Method: zip.Store}
		header.SetModTime(archiveEpoch)
		mode := os.FileMode(0o644)
		if path == manifest.Executable {
			mode = 0o755
		}
		header.SetMode(mode)
		writer, writeErr := archive.CreateHeader(header)
		if writeErr != nil {
			archive.Close()
			file.Close()
			return Artifact{}, writeErr
		}
		if _, writeErr = writer.Write(body); writeErr != nil {
			archive.Close()
			file.Close()
			return Artifact{}, writeErr
		}
	}
	if err := archive.Close(); err != nil {
		file.Close()
		return Artifact{}, err
	}
	if err := file.Close(); err != nil {
		return Artifact{}, err
	}
	raw, err := os.ReadFile(output)
	if err != nil {
		return Artifact{}, err
	}
	return Artifact{Path: output, SHA256: sha256Hex(raw), Bytes: int64(len(raw))}, nil
}

func validateManifest(manifest Manifest) error {
	if manifest.Version == "" || len(manifest.Commit) < 40 || manifest.SpecRevision == "" || len(manifest.Files) == 0 || len(manifest.Checksums) != len(manifest.Files) || manifest.SBOMPath != "SBOM.cdx.json" || manifest.SBOMTool != "native" {
		return core.ErrRevision
	}
	if manifest.Executable != "agent-teamctl" && manifest.Executable != "agent-teamctl.exe" {
		return core.ErrRevision
	}
	if !sort.StringsAreSorted(manifest.Files) || !contains(manifest.Files, manifest.Executable) {
		return core.ErrRevision
	}
	seen := map[string]bool{}
	for _, path := range manifest.Files {
		if seen[path] || !releasePaths[path] || len(manifest.Checksums[path]) != 64 {
			return core.ErrPath
		}
		seen[path] = true
	}
	return nil
}
