package store

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

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

func TestStoreRejectsUnboundedJSONAndInvalidMarkdown(t *testing.T) {
	s := New(t.TempDir(), core.StorageLimits{CanonicalBytes: math.MaxInt64})
	if _, err := s.WriteJSON("record.json", map[string]string{"state": "good"}, math.MaxInt64); !errors.Is(err, core.ErrLimit) {
		t.Fatalf("unbounded JSON error = %v, want ErrLimit", err)
	}
	if _, err := s.WriteMarkdown("bad.md", []byte{0xff}, 8); !errors.Is(err, core.ErrPath) {
		t.Fatalf("invalid UTF-8 error = %v, want ErrPath", err)
	}
	if _, err := s.WriteMarkdown("empty.md", nil, 1); err != nil {
		t.Fatalf("empty markdown: %v", err)
	}
	if result, err := s.WriteMarkdown("exact.md", []byte("12345678"), 8); err != nil || result.Bytes != 8 {
		t.Fatalf("exact boundary = %#v, %v", result, err)
	}
}

func TestStoreRejectsMalformedAndTrailingJSONAndPreservesErrorChain(t *testing.T) {
	s := New(t.TempDir(), core.StorageLimits{CanonicalBytes: 64})
	for name, contents := range map[string][]byte{
		"malformed.json": []byte(`{"state":`),
		"trailing.json":  []byte(`{"state":"good"} {}`),
	} {
		if err := os.WriteFile(filepath.Join(s.Root, name), contents, 0o600); err != nil {
			t.Fatal(err)
		}
		var destination map[string]string
		if err := s.ReadJSON(name, 64, &destination); !errors.Is(err, core.ErrPath) {
			t.Fatalf("ReadJSON(%s) = %v, want typed error", name, err)
		}
	}
	var destination map[string]string
	if err := s.ReadJSON("missing.json", 64, &destination); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing error = %v, want os.ErrNotExist in chain", err)
	}
}

func TestStoreFallbackSerializesAcrossStoresAndVerifiesChecksum(t *testing.T) {
	root := t.TempDir()
	first := New(root, core.StorageLimits{CanonicalBytes: 64})
	second := New(root, core.StorageLimits{CanonicalBytes: 64})
	var active int32
	var overlap int32
	probe := func(*os.Root, string) (probeResult, error) { return probeFallback, nil }
	verify := func(root *os.Root, relative string, limit int64) (AtomicResult, error) {
		if atomic.AddInt32(&active, 1) != 1 {
			atomic.StoreInt32(&overlap, 1)
		}
		defer atomic.AddInt32(&active, -1)
		time.Sleep(20 * time.Millisecond)
		return hashRootFile(root, relative, limit)
	}
	first.probe = probe
	second.probe = probe
	first.verify = verify
	second.verify = verify

	var wg sync.WaitGroup
	for _, writer := range []*Store{first, second} {
		wg.Add(1)
		go func(writer *Store) {
			defer wg.Done()
			if _, err := writer.WriteMarkdown("network.md", []byte("stable"), 64); err != nil {
				t.Error(err)
			}
		}(writer)
	}
	wg.Wait()
	if atomic.LoadInt32(&overlap) != 0 {
		t.Fatal("network fallback writes overlapped")
	}
	if got, err := os.ReadFile(filepath.Join(root, "network.md")); err != nil || !bytes.Equal(got, []byte("stable")) {
		t.Fatalf("network result = %q, %v", got, err)
	}
}

func TestStoreRestoresLastGoodAfterPostReplaceVerificationFailure(t *testing.T) {
	s := New(t.TempDir(), core.StorageLimits{CanonicalBytes: 64})
	if _, err := s.WriteMarkdown("record.md", []byte("last good"), 64); err != nil {
		t.Fatal(err)
	}
	s.verify = func(*os.Root, string, int64) (AtomicResult, error) {
		return AtomicResult{}, errors.New("simulated checksum read failure")
	}
	if _, err := s.WriteMarkdown("record.md", []byte("new value"), 64); !errors.Is(err, core.ErrPath) {
		t.Fatalf("write error = %v, want ErrPath", err)
	}
	got, err := os.ReadFile(filepath.Join(s.Root, "record.md"))
	if err != nil || string(got) != "last good" {
		t.Fatalf("last good not restored: %q, %v", got, err)
	}
}

func TestStoreRootAnchoringRejectsSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "link")); err != nil {
		t.Skipf("symbolic links unavailable: %v", err)
	}
	s := New(root, core.StorageLimits{CanonicalBytes: 64})
	if _, err := s.WriteMarkdown("link/escape.md", []byte("no"), 64); !errors.Is(err, core.ErrPath) {
		t.Fatalf("symlink escape error = %v, want ErrPath", err)
	}
	if _, err := os.Stat(filepath.Join(outside, "escape.md")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("write escaped root: %v", err)
	}
}

func TestReadBoundedDoesNotOverflow(t *testing.T) {
	if _, err := readBounded(strings.NewReader("x"), math.MaxInt64); !errors.Is(err, core.ErrLimit) {
		t.Fatalf("readBounded overflow guard = %v, want ErrLimit", err)
	}
	if _, err := io.ReadAll(strings.NewReader("ok")); err != nil {
		t.Fatal(err)
	}
}

func TestStoreProbesThenFlushesClosesAndHashesReplacement(t *testing.T) {
	s := New(t.TempDir(), core.StorageLimits{CanonicalBytes: 64})
	if result, err := s.WriteMarkdown("record.md", []byte("durable"), 64); err != nil {
		t.Fatal(err)
	} else {
		digest := sha256.Sum256([]byte("durable"))
		if result.Bytes != int64(len("durable")) || result.SHA256 != hex.EncodeToString(digest[:]) {
			t.Fatalf("result = %#v", result)
		}
	}
	info, err := os.Stat(filepath.Join(s.Root, "record.md"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0o077 != 0 {
		t.Fatalf("replacement permissions = %o, want restrictive", info.Mode().Perm())
	}
	root, err := os.OpenRoot(s.Root)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	if err := probeSameDirectoryReplace(root, "."); err != nil {
		t.Fatalf("same-directory probe: %v", err)
	}
}

func TestOwnedCleanupNeverDeletesAnExchangedTemporary(t *testing.T) {
	rootPath := t.TempDir()
	root, err := os.OpenRoot(rootPath)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	owned, file, err := createOwnedTemp(root, ".", ".agent-team-test-")
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(rootPath, owned.name)); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(rootPath, owned.name), []byte("unknown"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := removeOwned(root, owned); err == nil {
		t.Fatal("cleanup accepted an exchanged file")
	}
	if got, err := os.ReadFile(filepath.Join(rootPath, owned.name)); err != nil || string(got) != "unknown" {
		t.Fatalf("exchanged file was removed or changed: %q, %v", got, err)
	}
}

func TestStoreRemovesUnverifiedFirstWrite(t *testing.T) {
	s := New(t.TempDir(), core.StorageLimits{CanonicalBytes: 64})
	s.verify = func(*os.Root, string, int64) (AtomicResult, error) {
		return AtomicResult{}, errors.New("simulated post-replace failure")
	}
	if _, err := s.WriteMarkdown("fresh.md", []byte("unverified"), 64); !errors.Is(err, core.ErrPath) {
		t.Fatalf("write error = %v, want ErrPath", err)
	}
	if _, err := os.Stat(filepath.Join(s.Root, "fresh.md")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("unverified first write remains: %v", err)
	}
}

func TestStoreRetainsTransientAndLastGoodOnSharingRenameFailure(t *testing.T) {
	s := New(t.TempDir(), core.StorageLimits{CanonicalBytes: 64})
	if _, err := s.WriteMarkdown("locked.md", []byte("last good"), 64); err != nil {
		t.Fatal(err)
	}
	s.probe = func(*os.Root, string) (probeResult, error) { return probeFallback, nil }
	s.replace = func(*os.Root, string, string) error {
		return errors.New("simulated sharing violation")
	}
	if _, err := s.WriteMarkdown("locked.md", []byte("new value"), 64); !errors.Is(err, core.ErrPath) {
		t.Fatalf("write error = %v, want ErrPath", err)
	}
	if got, err := os.ReadFile(filepath.Join(s.Root, "locked.md")); err != nil || string(got) != "last good" {
		t.Fatalf("last good changed: %q, %v", got, err)
	}
	matches, err := filepath.Glob(filepath.Join(s.Root, ".agent-team-tmp-*"))
	if err != nil || len(matches) != 1 {
		t.Fatalf("retained transient matches = %v, %v", matches, err)
	}
}
