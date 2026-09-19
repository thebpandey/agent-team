package store

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/thebpandey/agent-team/vnext/internal/core"
)

func TestStoreBoundsAndLastGood(t *testing.T) {
	s := New(t.TempDir(), core.StorageLimits{CanonicalBytes: 32})
	if _, err := s.WriteJSON("record.json", strings.Repeat("x", 33), 32); !errors.Is(err, core.ErrLimit) {
		t.Fatal(err)
	}

	first, err := s.WriteJSON("record.json", map[string]string{"state": "good"}, 32)
	if err != nil || first.SHA256 == "" {
		t.Fatal(err, first)
	}
	if first.Bytes == 0 {
		t.Fatal("write did not report byte count")
	}
	if _, err := s.WriteJSON("record.json", strings.Repeat("y", 33), 32); !errors.Is(err, core.ErrLimit) {
		t.Fatal(err)
	}

	var got map[string]string
	if err := s.ReadJSON("record.json", 32, &got); err != nil || got["state"] != "good" {
		t.Fatal(err, got)
	}
	if _, err := s.WriteMarkdown("utf8.md", []byte("✓"), 8); err != nil {
		t.Fatal(err)
	}
}

func TestStoreRejectsPathsOutsideRoot(t *testing.T) {
	root := t.TempDir()
	s := New(root, core.StorageLimits{CanonicalBytes: 64})
	for _, relative := range []string{"../outside.json", "/outside.json", `..\\outside.json`} {
		if _, err := s.WriteJSON(relative, map[string]string{"ok": "yes"}, 64); !errors.Is(err, core.ErrPath) {
			t.Fatalf("WriteJSON(%q) error = %v, want ErrPath", relative, err)
		}
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(root), "outside.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("write escaped root: %v", err)
	}
}

func TestStoreReadHonorsBound(t *testing.T) {
	s := New(t.TempDir(), core.StorageLimits{CanonicalBytes: 1024})
	if err := os.WriteFile(filepath.Join(s.Root, "large.json"), []byte(`{"message":"too large"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	var destination map[string]string
	if err := s.ReadJSON("large.json", 8, &destination); !errors.Is(err, core.ErrLimit) {
		t.Fatalf("ReadJSON error = %v, want ErrLimit", err)
	}
}
