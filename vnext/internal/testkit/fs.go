package testkit

import (
	"os"
	"path/filepath"
	"testing"
)

func Fixture(t *testing.T, name string) string {
	t.Helper()
	root := t.TempDir()
	if name == "" {
		return root
	}
	path := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}
