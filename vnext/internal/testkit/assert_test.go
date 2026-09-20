package testkit

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSnapshotProjectTreeExcludesOnlyGitInternals(t *testing.T) {
	root := t.TempDir()
	tracked := filepath.Join(root, "tracked.txt")
	lock := filepath.Join(root, ".git", "objects", "maintenance.lock")
	if err := os.MkdirAll(filepath.Dir(lock), 0o700); err != nil {
		t.Fatal(err)
	}
	for path, body := range map[string]string{tracked: "tracked\n", lock: "transient\n"} {
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	before := SnapshotProjectTree(t, root)
	if err := os.Remove(lock); err != nil {
		t.Fatal(err)
	}
	if after := SnapshotProjectTree(t, root); !sameSnapshot(before, after) {
		t.Fatalf("Git maintenance lock changed project snapshot: before=%v after=%v", before, after)
	}
	if err := os.WriteFile(tracked, []byte("changed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if after := SnapshotProjectTree(t, root); sameSnapshot(before, after) {
		t.Fatal("working-tree change was ignored")
	}
}

func TestSnapshotProjectTreeIgnoresRootGitFile(t *testing.T) {
	root := t.TempDir()
	gitFile := filepath.Join(root, ".git")
	if err := os.WriteFile(gitFile, []byte("gitdir: elsewhere\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	before := SnapshotProjectTree(t, root)
	if err := os.WriteFile(gitFile, []byte("gitdir: changed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if after := SnapshotProjectTree(t, root); !sameSnapshot(before, after) {
		t.Fatalf("root .git file changed project snapshot: before=%v after=%v", before, after)
	}
}

func TestSnapshotProjectTreeIncludesNestedGitContent(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "nested", ".git", "HEAD")
	if err := os.MkdirAll(filepath.Dir(nested), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(nested, []byte("one\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	before := SnapshotProjectTree(t, root)
	if err := os.WriteFile(nested, []byte("two\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if after := SnapshotProjectTree(t, root); sameSnapshot(before, after) {
		t.Fatal("nested .git content was ignored")
	}
}

func TestSnapshotChangesNamesAddedRemovedAndChangedPaths(t *testing.T) {
	got := snapshotChanges(
		map[string]string{"removed": "old", "changed": "old", "same": "same"},
		map[string]string{"added": "new", "changed": "new", "same": "same"},
	)
	want := []string{"added added", "changed changed", "removed removed"}
	if len(got) != len(want) {
		t.Fatalf("snapshotChanges() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("snapshotChanges() = %v, want %v", got, want)
		}
	}
}

func sameSnapshot(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for path, digest := range a {
		if b[path] != digest {
			return false
		}
	}
	return true
}
