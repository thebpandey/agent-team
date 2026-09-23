package project

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
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

func embeddedBeadsFixture(t *testing.T) (string, string) {
	t.Helper()
	root := t.TempDir()
	manifest := filepath.Join(root, ".beads", "embeddeddolt", "selected", ".dolt", "noms", "manifest")
	if err := os.MkdirAll(filepath.Dir(manifest), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".beads", "metadata.json"), []byte(`{"database":"dolt","backend":"dolt","dolt_mode":"embedded","dolt_database":"selected"}`), 0600); err != nil {
		t.Fatal(err)
	}
	return root, manifest
}

func TestBeadsSnapshotTracksManifestRootNotStorageLayout(t *testing.T) {
	root, manifest := embeddedBeadsFixture(t)
	first := "5:__DOLT__:00000000000000000000000000000000:kisvdtabvp5ekb0vq66a7ctib4iuctpb:00000000000000000000000000000000:vvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvv:3115"
	write := func(body string) {
		t.Helper()
		if err := os.WriteFile(manifest, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	fingerprint := func() string {
		t.Helper()
		value, err := digestArtifact(root, ".beads", filepath.Join(root, ".beads"), 1024)
		if err != nil {
			t.Fatal(err)
		}
		return value
	}
	write(first)
	before := fingerprint()
	storage, err := os.Create(filepath.Join(filepath.Dir(manifest), "large-storage"))
	if err != nil {
		t.Fatal(err)
	}
	if err := storage.Truncate(256 << 20); err != nil {
		storage.Close()
		t.Fatal(err)
	}
	storage.Close()
	// Repacking changes physical table files, lock and GC generation, not root.
	write("5:__DOLT__:11111111111111111111111111111111:kisvdtabvp5ekb0vq66a7ctib4iuctpb:22222222222222222222222222222222:33333333333333333333333333333333:1")
	if after := fingerprint(); after != before {
		t.Fatal("storage-only changes affected snapshot")
	}
	write(strings.Replace(first, "kisvdtabvp5ekb0vq66a7ctib4iuctpb", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", 1))
	if fingerprint() == before {
		t.Fatal("manifest root change was ignored")
	}
	write("4:__DOLT__:00000000000000000000000000000000:kisvdtabvp5ekb0vq66a7ctib4iuctpb:vvvvvvvvvvvvvvvvvvvvvvvvvvvvvvvv:3115")
	if fingerprint() != before {
		t.Fatal("v4 root differs from equivalent v5 root")
	}
}

func TestBeadsSnapshotPrefersManifestThenBoundedPassiveExport(t *testing.T) {
	root, manifest := embeddedBeadsFixture(t)
	export := filepath.Join(root, ".beads", "issues.jsonl")
	fingerprint := func() string {
		t.Helper()
		value, err := digestArtifact(root, ".beads", filepath.Join(root, ".beads"), 1024)
		if err != nil {
			t.Fatal(err)
		}
		return value
	}
	identity := fingerprint()
	if err := os.WriteFile(export, []byte("{\"id\":\"one\"}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	first := fingerprint()
	if first == identity {
		t.Fatal("passive export ignored")
	}
	if err := os.WriteFile(export, []byte("{\"id\":\"two\"}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	second := fingerprint()
	if second == first {
		t.Fatal("passive export change ignored")
	}
	for _, unsupported := range []string{"future:format", "5:__DOLT__:bad:bad:bad", strings.Repeat("x", 1025)} {
		if err := os.WriteFile(manifest, []byte(unsupported), 0600); err != nil {
			t.Fatal(err)
		}
		if fingerprint() != second {
			t.Fatal("unavailable root did not use passive export")
		}
	}
	if err := os.WriteFile(manifest, []byte("5:__DOLT__:00000000000000000000000000000000:kisvdtabvp5ekb0vq66a7ctib4iuctpb:00000000000000000000000000000000"), 0600); err != nil {
		t.Fatal(err)
	}
	preferred := fingerprint()
	if err := os.WriteFile(export, []byte("changed passive export"), 0600); err != nil {
		t.Fatal(err)
	}
	if fingerprint() != preferred {
		t.Fatal("passive export overrode manifest root")
	}
	if err := os.Remove(manifest); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(export, []byte(strings.Repeat("x", 1025)), 0600); err != nil {
		t.Fatal(err)
	}
	if fingerprint() != identity {
		t.Fatal("oversized export did not use identity fallback")
	}
}

func TestBeadsSnapshotRejectsUnsafeSelectedDatabaseOrSnapshotLinks(t *testing.T) {
	for _, database := range []string{"../outside", "a/b", "a\\b", "/tmp/db", "C:drive", ".", "..", "NUL"} {
		t.Run(database, func(t *testing.T) {
			root, _ := embeddedBeadsFixture(t)
			body := `{"backend":"dolt","dolt_mode":"embedded","dolt_database":` + strconv.Quote(database) + `}`
			if err := os.WriteFile(filepath.Join(root, ".beads", "metadata.json"), []byte(body), 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := digestArtifact(root, ".beads", filepath.Join(root, ".beads"), 1024); !errors.Is(err, core.ErrPath) {
				t.Fatalf("unsafe database accepted: %v", err)
			}
		})
	}
	for _, relative := range []string{"embeddeddolt/selected/.dolt/noms/manifest", "embeddeddolt/selected", "issues.jsonl"} {
		t.Run(relative, func(t *testing.T) {
			root, _ := embeddedBeadsFixture(t)
			link := filepath.Join(root, ".beads", filepath.FromSlash(relative))
			if err := os.RemoveAll(link); err != nil {
				t.Fatal(err)
			}
			target := filepath.Join(root, "missing-target")
			if err := os.Symlink(target, link); err != nil {
				t.Skipf("symlink unavailable: %v", err)
			}
			if _, err := digestArtifact(root, ".beads", filepath.Join(root, ".beads"), 1024); !errors.Is(err, core.ErrPath) {
				t.Fatalf("snapshot link accepted: %v", err)
			}
		})
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
