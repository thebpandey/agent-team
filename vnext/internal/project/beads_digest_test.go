package project

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/thebpandey/agent-team/vnext/internal/core"
)

func TestBeadsIdentityIgnoresDatabaseStorage(t *testing.T) {
	root := t.TempDir()
	beads := filepath.Join(root, ".beads")
	if err := os.MkdirAll(filepath.Join(beads, "dolt"), 0700); err != nil {
		t.Fatal(err)
	}
	metadata := filepath.Join(beads, "metadata.json")
	if err := os.WriteFile(metadata, []byte(`{"backend":"dolt","database":"project"}`), 0600); err != nil {
		t.Fatal(err)
	}
	before, err := digestArtifact(root, ".beads", beads, 1024)
	if err != nil {
		t.Fatal(err)
	}
	db, err := os.Create(filepath.Join(beads, "dolt", "database"))
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Truncate(256 << 20); err != nil {
		db.Close()
		t.Fatal(err)
	}
	db.Close()
	// Traversing this unused storage link would fail the old recursive digest.
	if err := os.Symlink(filepath.Join(root, "missing-storage"), filepath.Join(beads, "dolt", "unused-link")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	after, err := digestArtifact(root, ".beads", beads, 1024)
	if err != nil || before != after {
		t.Fatalf("database changed bounded identity: before=%q after=%q err=%v", before, after, err)
	}
	if err := os.WriteFile(metadata, []byte(`{"backend":"dolt","database":"other"}`), 0600); err != nil {
		t.Fatal(err)
	}
	changed, err := digestArtifact(root, ".beads", beads, 1024)
	if err != nil || changed == before {
		t.Fatalf("metadata did not change identity: %q %v", changed, err)
	}
}

func TestBeadsIdentityRejectsMalformedOrOversizedMetadata(t *testing.T) {
	for _, body := range []string{`null`, `[]`, `"database"`, `{"database":`, `{} {}`, `{"database":null}`, `{"database":42}`, `{"backend":false}`} {
		t.Run(body, func(t *testing.T) {
			root := t.TempDir()
			beads := filepath.Join(root, ".beads")
			if err := os.Mkdir(beads, 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(beads, "metadata.json"), []byte(body), 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := digestArtifact(root, ".beads", beads, 1024); !errors.Is(err, core.ErrSettings) {
				t.Fatalf("malformed metadata accepted: %v", err)
			}
		})
	}
	root := t.TempDir()
	beads := filepath.Join(root, ".beads")
	if err := os.Mkdir(beads, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(beads, "metadata.json"), []byte(`{"database":"long-name"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := digestArtifact(root, ".beads", beads, 8); !errors.Is(err, core.ErrLimit) {
		t.Fatalf("unbounded metadata: %v", err)
	}
}

func TestBeadsIdentityEmptyDirectoryAndMetadataSymlink(t *testing.T) {
	root := t.TempDir()
	beads := filepath.Join(root, ".beads")
	if err := os.Mkdir(beads, 0700); err != nil {
		t.Fatal(err)
	}
	if digest, err := digestArtifact(root, ".beads", beads, 1024); err != nil || digest == "" {
		t.Fatalf("empty tracker identity: %q %v", digest, err)
	}
	target := filepath.Join(root, "outside.json")
	if err := os.WriteFile(target, []byte(`{}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(beads, "metadata.json")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, err := digestArtifact(root, ".beads", beads, 1024); err == nil {
		t.Fatal("followed metadata symlink")
	}
}
