package testkit

import (
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"testing"
)

func SnapshotTree(t *testing.T, root string) map[string]string {
	return snapshotTree(t, root, false)
}

// SnapshotProjectTree hashes project content while excluding Git's private
// implementation directory, whose maintenance locks may change concurrently.
func SnapshotProjectTree(t *testing.T, root string) map[string]string {
	return snapshotTree(t, root, true)
}

func snapshotTree(t *testing.T, root string, excludeGit bool) map[string]string {
	t.Helper()
	result := map[string]string{}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if excludeGit && filepath.ToSlash(rel) == ".git" {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(body)
		result[filepath.ToSlash(rel)] = hex.EncodeToString(sum[:])
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func RequireNoProjectWrites(t *testing.T, root string, before map[string]string) {
	t.Helper()
	requireNoWrites(t, before, SnapshotProjectTree(t, root))
}

func RequireNoWrites(t *testing.T, root string, before map[string]string) {
	t.Helper()
	requireNoWrites(t, before, SnapshotTree(t, root))
}

func requireNoWrites(t *testing.T, before, after map[string]string) {
	t.Helper()
	if changes := snapshotChanges(before, after); len(changes) != 0 {
		t.Fatalf("tree changed: %v", changes)
	}
}

func snapshotChanges(before, after map[string]string) []string {
	keys := make([]string, 0, len(before)+len(after))
	for key := range before {
		keys = append(keys, key)
	}
	for key := range after {
		if _, ok := before[key]; !ok {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	changes := make([]string, 0)
	for _, key := range keys {
		beforeDigest, beforeOK := before[key]
		afterDigest, afterOK := after[key]
		switch {
		case !beforeOK:
			changes = append(changes, "added "+key)
		case !afterOK:
			changes = append(changes, "removed "+key)
		case beforeDigest != afterDigest:
			changes = append(changes, "changed "+key)
		}
	}
	return changes
}
