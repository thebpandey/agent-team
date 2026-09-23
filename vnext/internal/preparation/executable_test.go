package preparation

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
)

func TestExecutableStageDoesNotExposeFinalPath(t *testing.T) {
	root := t.TempDir()
	dest := filepath.Join(root, "tool")
	data := []byte("complete executable fixture")
	stage, err := stageExecutable(dest, data)
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(stage)
	if _, err := os.Lstat(dest); !os.IsNotExist(err) {
		t.Fatalf("unpublished destination exists: %v", err)
	}
	got, err := os.ReadFile(stage)
	if err != nil || !bytes.Equal(got, data) {
		t.Fatalf("staged bytes=%q error=%v", got, err)
	}
	info, err := os.Stat(stage)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0700 {
		t.Fatalf("staged mode: %v", info.Mode())
	}
}

func TestExecutablePublicationPreservesExistingFileAndSymlink(t *testing.T) {
	for _, symlink := range []bool{false, true} {
		t.Run(map[bool]string{false: "file", true: "symlink"}[symlink], func(t *testing.T) {
			root := t.TempDir()
			dest := filepath.Join(root, "tool")
			original := []byte("custom binary")
			path := dest
			if symlink {
				path = filepath.Join(t.TempDir(), "outside")
			}
			if err := os.WriteFile(path, original, 0700); err != nil {
				t.Fatal(err)
			}
			if symlink {
				if err := os.Symlink(path, dest); err != nil {
					t.Skip(err)
				}
			}
			if err := publishExecutable(dest, []byte("replacement")); !errors.Is(err, os.ErrExist) {
				t.Fatalf("existing destination accepted: %v", err)
			}
			got, err := os.ReadFile(path)
			if err != nil || !bytes.Equal(got, original) {
				t.Fatalf("custom data changed: %q %v", got, err)
			}
			entries, _ := os.ReadDir(root)
			if len(entries) != 1 {
				t.Fatalf("failed publication retained staging files: %v", entries)
			}
		})
	}
}

func TestExecutablePublicationConcurrentWritersPublishOneWholeFile(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "tool")
	payloads := [][]byte{bytes.Repeat([]byte("one"), 10000), bytes.Repeat([]byte("two"), 10000)}
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for _, payload := range payloads {
		wg.Add(1)
		go func(data []byte) { defer wg.Done(); results <- publishExecutable(dest, data) }(payload)
	}
	wg.Wait()
	close(results)
	winners := 0
	for err := range results {
		if err == nil {
			winners++
		} else if !errors.Is(err, os.ErrExist) {
			t.Fatal(err)
		}
	}
	if winners != 1 {
		t.Fatalf("publication winners=%d", winners)
	}
	got, err := os.ReadFile(dest)
	if err != nil || (!bytes.Equal(got, payloads[0]) && !bytes.Equal(got, payloads[1])) {
		t.Fatalf("torn executable: len=%d err=%v", len(got), err)
	}
}

func TestExecutablePublicationInterruptedChild(t *testing.T) {
	dest := os.Getenv("AGENT_TEAM_EXECUTABLE_STAGE_TEST_DEST")
	if dest == "" {
		return
	}
	if _, err := stageExecutable(dest, []byte("abandoned staged bytes")); err != nil {
		t.Fatal(err)
	}
	// Exit without publishing or removing the staging file, as when the parent
	// installer terminates between Sync and exclusive publication.
}

func TestExecutablePublicationCanRetryAfterProcessExit(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "tool")
	cmd := exec.Command(os.Args[0], "-test.run=^TestExecutablePublicationInterruptedChild$")
	cmd.Env = append(os.Environ(), "AGENT_TEAM_EXECUTABLE_STAGE_TEST_DEST="+dest)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("child: %s %v", out, err)
	}
	if _, err := os.Lstat(dest); !os.IsNotExist(err) {
		t.Fatalf("interruption exposed executable: %v", err)
	}
	data := []byte("retry publishes this complete binary")
	if err := publishExecutable(dest, data); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(dest)
	if err != nil || !bytes.Equal(got, data) {
		t.Fatalf("retry produced %q: %v", got, err)
	}
}
