package project

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/thebpandey/agent-team/vnext/internal/core"
)

// digestBeadsIdentity fingerprints metadata plus a bounded passive snapshot.
// Prefer the selected embedded Dolt manifest root, then issues.jsonl up to the
// caller's artifact limit. Missing, oversized or unknown-format snapshot
// evidence falls back to metadata identity. Task contents/revisions still come
// from tracker adapters. Never walk storage or open a database during setup.
func digestBeadsIdentity(rootPath, relative string, limit int64) (string, error) {
	if relative != ".beads" || limit <= 0 {
		return "", fmt.Errorf("%w: invalid Beads identity input", core.ErrSettings)
	}
	root, err := os.OpenRoot(rootPath)
	if err != nil {
		return "", err
	}
	defer root.Close()
	info, err := root.Lstat(relative)
	if err != nil {
		return "", err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("%w: Beads authority must be a directory", core.ErrPath)
	}
	metadataLimit := min(limit, int64(64<<10))
	raw, exists, err := readBeadsEvidence(root, filepath.Join(relative, "metadata.json"), metadataLimit)
	if err != nil {
		return "", fmt.Errorf("%w: Beads metadata: %w", core.ErrSettings, err)
	}
	document := map[string]json.RawMessage{}
	identity := digestText("beads-directory-identity-v1")
	if exists {
		if json.Unmarshal(raw, &document) != nil || document == nil {
			return "", fmt.Errorf("%w: Beads metadata must be a JSON object", core.ErrSettings)
		}
		for _, field := range []string{"database", "backend", "jsonl_export", "dolt_database", "dolt_mode"} {
			if value, present := document[field]; present {
				var text string
				if json.Unmarshal(value, &text) != nil || text == "" {
					return "", fmt.Errorf("%w: malformed Beads metadata %s", core.ErrSettings, field)
				}
			}
		}
		canonical, err := json.Marshal(document)
		if err != nil {
			return "", err
		}
		identity = digestText("beads-metadata-identity-v1\x00" + string(canonical))
	}
	var mode, database string
	_ = json.Unmarshal(document["dolt_mode"], &mode)
	_ = json.Unmarshal(document["dolt_database"], &database)
	if mode == "embedded" && database != "" {
		if err := ValidateSegment(database); err != nil {
			return "", err
		}
		manifest := filepath.Join(relative, "embeddeddolt", database, ".dolt", "noms", "manifest")
		raw, exists, err := readBeadsEvidence(root, manifest, metadataLimit)
		if err != nil && !errors.Is(err, core.ErrLimit) {
			return "", err
		}
		if err == nil && exists {
			if snapshot, ok := doltManifestRoot(raw); ok {
				return digestText("beads-snapshot-v1\x00" + identity + "\x00dolt-root:" + snapshot), nil
			}
		}
	}
	raw, exists, err = readBeadsEvidence(root, filepath.Join(relative, "issues.jsonl"), limit)
	if err != nil && !errors.Is(err, core.ErrLimit) {
		return "", err
	}
	if err == nil && exists {
		return digestText("beads-snapshot-v1\x00" + identity + "\x00issues-jsonl:" + digestBytes(raw)), nil
	}
	return identity, nil
}

// Read only the named path, rejecting links in every component. Size checks
// happen before opening file content, including for oversized storage manifests.
func readBeadsEvidence(root *os.Root, relative string, limit int64) ([]byte, bool, error) {
	current := root
	var openedRoots []*os.Root
	defer func() {
		for _, opened := range openedRoots {
			_ = opened.Close()
		}
	}()
	parts := strings.Split(filepath.ToSlash(relative), "/")
	for index, part := range parts {
		info, err := current.Lstat(part)
		if errors.Is(err, os.ErrNotExist) {
			return nil, false, nil
		}
		if err != nil {
			return nil, false, err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil, false, fmt.Errorf("%w: symlink in Beads evidence", core.ErrPath)
		}
		if index < len(parts)-1 {
			if !info.IsDir() {
				return nil, false, fmt.Errorf("%w: non-directory Beads evidence parent", core.ErrPath)
			}
			child, err := current.OpenRoot(part)
			if err != nil {
				return nil, false, err
			}
			openedRoots = append(openedRoots, child)
			actual, err := child.Stat(".")
			if err != nil || !os.SameFile(info, actual) {
				return nil, false, fmt.Errorf("%w: Beads evidence parent changed", core.ErrPath)
			}
			current = child
			continue
		}
		if !info.Mode().IsRegular() {
			return nil, false, fmt.Errorf("%w: non-regular Beads evidence", core.ErrPath)
		}
		if info.Size() > limit {
			return nil, false, core.ErrLimit
		}
		file, err := current.Open(part)
		if err != nil {
			return nil, false, err
		}
		defer file.Close()
		actual, err := file.Stat()
		if err != nil || !os.SameFile(info, actual) {
			return nil, false, fmt.Errorf("%w: Beads evidence changed", core.ErrPath)
		}
		raw, err := readBoundedFile(file, limit)
		return raw, true, err
	}
	return nil, false, core.ErrPath
}

// Dolt's v4/v5 layout is documented by its own parser:
// https://github.com/dolthub/dolt/blob/main/go/store/nbs/file_manifest.go
// Root is field 3 in both versions. Lock, GC and physical table changes do not
// change the logical snapshot, so only the validated root enters our digest.
func doltManifestRoot(raw []byte) (string, bool) {
	fields := strings.Split(string(raw), ":")
	if len(fields) < 4 {
		return "", false
	}
	prefix := 4
	switch fields[0] {
	case "4":
	case "5":
		prefix = 5
	default:
		return "", false
	}
	if len(fields) < prefix || (len(fields)-prefix)%2 != 0 || fields[1] == "" || !doltHash(fields[2]) || !doltHash(fields[3]) {
		return "", false
	}
	if prefix == 5 && !doltHash(fields[4]) {
		return "", false
	}
	for index := prefix; index < len(fields); index += 2 {
		if !doltHash(fields[index]) {
			return "", false
		}
		if _, err := strconv.ParseUint(fields[index+1], 10, 32); err != nil {
			return "", false
		}
	}
	return fields[3], true
}

func doltHash(value string) bool {
	if len(value) != 32 {
		return false
	}
	for _, character := range value {
		if !(character >= '0' && character <= '9' || character >= 'a' && character <= 'v') {
			return false
		}
	}
	return true
}
