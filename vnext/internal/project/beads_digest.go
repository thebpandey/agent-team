package project

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/thebpandey/agent-team/vnext/internal/core"
)

// digestBeadsIdentity binds setup to the local tracker metadata, not to mutable
// Dolt storage. Task contents and revisions are supplied by tracker snapshots.
// Never walk this directory: an initialized database can be many gigabytes.
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
	metadata := filepath.Join(relative, "metadata.json")
	if _, err := root.Lstat(metadata); errors.Is(err, os.ErrNotExist) {
		return digestText("beads-directory-identity-v1"), nil
	} else if err != nil {
		return "", err
	}
	// Metadata is small even when the backing database is large.
	if limit > 64<<10 {
		limit = 64 << 10
	}
	raw, err := readBoundedContained(rootPath, metadata, limit)
	if err != nil {
		return "", fmt.Errorf("%w: Beads metadata: %w", core.ErrSettings, err)
	}
	var document map[string]json.RawMessage
	if json.Unmarshal(raw, &document) != nil || document == nil {
		return "", fmt.Errorf("%w: Beads metadata must be a JSON object", core.ErrSettings)
	}
	for _, field := range []string{"database", "backend", "jsonl_export", "dolt_database"} {
		if value, present := document[field]; present {
			var text string
			if json.Unmarshal(value, &text) != nil || text == "" {
				return "", fmt.Errorf("%w: malformed Beads metadata %s", core.ErrSettings, field)
			}
		}
	}
	// Normalize insignificant whitespace without decoding numbers through float64.
	canonical, err := json.Marshal(document)
	if err != nil {
		return "", err
	}
	return digestText("beads-metadata-identity-v1\x00" + string(canonical)), nil
}
