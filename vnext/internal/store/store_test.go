package store

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math"
	"os"
	"os/exec"
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
	if _, err := s.CreateJSON("link/escape.json", map[string]string{"no": "escape"}, 64); !errors.Is(err, core.ErrPath) {
		t.Fatalf("CreateJSON symlink escape error = %v, want ErrPath", err)
	}
	if _, err := os.Stat(filepath.Join(outside, "escape.md")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("write escaped root: %v", err)
	}
}

func TestBoundedReaderDetectsBytesAfterBoundary(t *testing.T) {
	data, err := io.ReadAll(&boundedReader{reader: strings.NewReader("xy"), remaining: 1})
	if !errors.Is(err, core.ErrLimit) || string(data) != "x" {
		t.Fatalf("bounded stream = %q, %v; want first byte and ErrLimit", data, err)
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
	entries, err := os.ReadDir(s.Root)
	if err != nil || len(entries) != 1 || entries[0].Name() != "record.md" {
		t.Fatalf("probe residue = %v, %v", entries, err)
	}
}

func TestStoreProbeCleansOwnedFilesAfterReplaceFailure(t *testing.T) {
	rootPath := t.TempDir()
	root, err := os.OpenRoot(rootPath)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	if err := probeSameDirectoryReplaceWith(root, ".", func(*os.Root, string, string) error {
		return errors.New("replace failed")
	}, removeOwned); err == nil {
		t.Fatal("replace failure accepted")
	}
	entries, err := os.ReadDir(rootPath)
	if err != nil || len(entries) != 0 {
		t.Fatalf("failed probe residue = %v, %v", entries, err)
	}
}

func TestStoreProbeCleansOwnedDestinationAfterReportedReplaceFailure(t *testing.T) {
	rootPath := t.TempDir()
	root, err := os.OpenRoot(rootPath)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	if err := probeSameDirectoryReplaceWith(root, ".", func(root *os.Root, from, to string) error {
		if err := replaceFile(root, from, to); err != nil {
			return err
		}
		return errors.New("replace reported failure")
	}, removeOwned); err == nil {
		t.Fatal("reported replace failure accepted")
	}
	entries, err := os.ReadDir(rootPath)
	if err != nil || len(entries) != 0 {
		t.Fatalf("reported failure residue = %v, %v", entries, err)
	}
}

func TestStoreProbeRetainsExchangedDestination(t *testing.T) {
	rootPath := t.TempDir()
	root, err := os.OpenRoot(rootPath)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	var destination string
	err = probeSameDirectoryReplaceWith(root, ".", func(root *os.Root, _, to string) error {
		destination = to
		if err := root.Rename(to, to+"-held"); err != nil {
			return err
		}
		file, err := root.OpenFile(to, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err != nil {
			return err
		}
		_, writeErr := file.Write([]byte("foreign"))
		closeErr := file.Close()
		return errors.Join(errors.New("replace failed"), writeErr, closeErr)
	}, removeOwned)
	if err == nil {
		t.Fatal("exchanged replacement accepted")
	}
	got, readErr := os.ReadFile(filepath.Join(rootPath, destination))
	if readErr != nil || string(got) != "foreign" {
		t.Fatalf("foreign replacement = %q, %v", got, readErr)
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
	held := filepath.Join(rootPath, owned.name+"-held")
	if err := os.Rename(filepath.Join(rootPath, owned.name), held); err != nil {
		t.Fatal(err)
	}
	defer os.Remove(held)
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

func TestOwnedCleanupNeverDeletesReplacementAfterIdentityCheck(t *testing.T) {
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
	checked, swapped := make(chan struct{}), make(chan struct{})
	ownedRemoveHook = func(*os.Root, ownedTemp) {
		close(checked)
		<-swapped
	}
	defer func() { ownedRemoveHook = nil }()
	done := make(chan error, 1)
	go func() { done <- removeOwned(root, owned) }()
	<-checked
	original := filepath.Join(rootPath, owned.name)
	held := original + "-held"
	if err := os.Rename(original, held); err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	defer os.Remove(held)
	if err := os.WriteFile(original, []byte("foreign"), 0o600); err != nil {
		t.Fatal(err)
	}
	close(swapped)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(original); err != nil || string(got) != "foreign" {
		t.Fatalf("replacement = %q, %v", got, err)
	}
}

func TestStoreProbeSurfacesCleanupFailures(t *testing.T) {
	for _, tc := range []struct {
		name    string
		bodyErr error
	}{
		{name: "successful body"},
		{name: "failed body", bodyErr: errors.New("replace failed")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root, err := os.OpenRoot(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			defer root.Close()
			cleanupErr := errors.New("cleanup failed")
			err = probeSameDirectoryReplaceWith(root, ".", func(root *os.Root, from, to string) error {
				if tc.bodyErr != nil {
					return tc.bodyErr
				}
				return replaceFile(root, from, to)
			}, func(*os.Root, ownedTemp) error { return cleanupErr })
			if !errors.Is(err, cleanupErr) || (tc.bodyErr != nil && !errors.Is(err, tc.bodyErr)) {
				t.Fatalf("probe error = %v", err)
			}
		})
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

func TestStoreRetainsDiscoverableLastGoodRecoveryOnRestoreFailure(t *testing.T) {
	s := New(t.TempDir(), core.StorageLimits{CanonicalBytes: 64})
	if _, err := s.WriteMarkdown("record.md", []byte("last good"), 64); err != nil {
		t.Fatal(err)
	}
	s.verify = func(*os.Root, string, int64) (AtomicResult, error) {
		return AtomicResult{}, errors.New("simulated verification failure")
	}
	s.restore = func(*os.Root, string, string) error {
		return errors.New("simulated persistent sharing violation")
	}
	if _, err := s.WriteMarkdown("record.md", []byte("new value"), 64); err == nil {
		t.Fatal("write unexpectedly succeeded")
	} else if !strings.Contains(err.Error(), recoveryPath("record.md")) {
		t.Fatalf("recovery location missing from error: %v", err)
	}
	entries, err := filepath.Glob(filepath.Join(s.Root, recoveryPath("record.md")+"-*"))
	if err != nil || len(entries) != 1 {
		t.Fatalf("retained recovery entries = %v, %v", entries, err)
	}
	recovery, err := os.ReadFile(entries[0])
	if err != nil || string(recovery) != "last good" {
		t.Fatalf("retained last-good recovery = %q, %v", recovery, err)
	}
}

func TestStoreCanonicalHardLimitIsThirtyTwoMiB(t *testing.T) {
	const hard = 32 << 20
	boundary := bytes.Repeat([]byte("x"), hard)
	for _, limits := range []core.StorageLimits{{}, {CanonicalBytes: 64 << 20}} {
		s := New(t.TempDir(), limits)
		if result, err := s.WriteMarkdown("boundary.md", boundary, hard); err != nil || result.Bytes != hard {
			t.Fatalf("boundary write limits=%+v result=%+v err=%v", limits, result, err)
		}
		if _, err := s.WriteMarkdown("oversized.md", []byte("x"), hard+1); !errors.Is(err, core.ErrLimit) {
			t.Fatalf("oversized caller limit=%v, want ErrLimit", err)
		}
	}
	if _, err := New(t.TempDir(), core.StorageLimits{}).CreateJSON("zero.json", map[string]string{}, 0); !errors.Is(err, core.ErrLimit) {
		t.Fatalf("zero create limit = %v, want ErrLimit", err)
	}
	jsonStore := New(t.TempDir(), core.StorageLimits{CanonicalBytes: hard})
	boundaryJSON := strings.Repeat("x", hard-3) // quotes plus newline make exactly hard bytes.
	if result, err := jsonStore.CreateJSON("boundary.json", boundaryJSON, hard); err != nil || result.Bytes != hard || result.SHA256 == "" {
		t.Fatalf("JSON boundary result=%+v err=%v", result, err)
	}
	if _, err := jsonStore.CreateJSON("too-large.json", strings.Repeat("x", hard-2), hard); !errors.Is(err, core.ErrLimit) {
		t.Fatalf("oversized JSON = %v, want ErrLimit", err)
	}
}

func TestStoreCreateJSONNeverReplacesAnExistingRecord(t *testing.T) {
	s := New(t.TempDir(), core.StorageLimits{})
	first, err := s.CreateJSON("commits/one.json", map[string]string{"winner": "first"}, 1024)
	if err != nil || first.Bytes == 0 {
		t.Fatalf("first create = %#v, %v", first, err)
	}
	if _, err := s.CreateJSON("commits/one.json", map[string]string{"winner": "second"}, 1024); !errors.Is(err, ErrAlreadyExists) {
		t.Fatalf("second create = %v, want ErrAlreadyExists", err)
	} else if !errors.Is(err, fs.ErrExist) || !errors.Is(err, core.ErrRevision) {
		t.Fatalf("second create error chain = %v, want fs.ErrExist and core.ErrRevision", err)
	}
	var got map[string]string
	if err := s.ReadJSON("commits/one.json", 1024, &got); err != nil || got["winner"] != "first" {
		t.Fatalf("winner changed: %#v, %v", got, err)
	}
	data, err := os.ReadFile(filepath.Join(s.Root, "commits", "one.json"))
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	if first.Bytes != int64(len(data)) || first.SHA256 != hex.EncodeToString(sum[:]) {
		t.Fatalf("CreateJSON result=%#v does not match persisted bytes", first)
	}
}

func TestStoreCreateJSONConcurrentAndUnsupportedLink(t *testing.T) {
	s := New(t.TempDir(), core.StorageLimits{})
	var wg sync.WaitGroup
	results := make(chan error, 8)
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := s.CreateJSON("commit.json", map[string]string{"winner": "one"}, 128)
			results <- err
		}()
	}
	wg.Wait()
	close(results)
	winners := 0
	for err := range results {
		if err == nil {
			winners++
		} else if !errors.Is(err, fs.ErrExist) || !errors.Is(err, core.ErrRevision) {
			t.Fatal(err)
		}
	}
	if winners != 1 {
		t.Fatalf("winners=%d", winners)
	}
	fail := New(t.TempDir(), core.StorageLimits{})
	fail.link = func(*os.Root, string, string) error { return errors.New("unsupported hardlink") }
	if _, err := fail.CreateJSON("commit.json", map[string]string{"no": "publication"}, 128); err == nil {
		t.Fatal("unsupported link succeeded")
	}
	var got map[string]string
	if err := fail.ReadJSON("commit.json", 128, &got); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("canonical publication remains: %v", err)
	}
}

// TestStoreCreateJSONSubprocessHelper deliberately runs in a distinct process.
// The parent test below depends on the no-replace filesystem primitive, not the
// Store's in-process mutexes.
func TestStoreCreateJSONSubprocessHelper(t *testing.T) {
	if os.Getenv("STORE_CREATE_HELPER") != "1" {
		return
	}
	s := New(os.Getenv("STORE_CREATE_ROOT"), core.StorageLimits{})
	_, err := s.CreateJSON("commit.json", map[string]string{"winner": os.Getenv("STORE_CREATE_VALUE")}, 1024)
	switch {
	case err == nil:
		fmt.Fprint(os.Stdout, "STORE_CREATE=created")
	case errors.Is(err, fs.ErrExist) && errors.Is(err, core.ErrRevision):
		fmt.Fprint(os.Stdout, "STORE_CREATE=exists")
	default:
		t.Fatalf("subprocess CreateJSON: %v", err)
	}
}

func TestStoreCreateJSONSubprocessContention(t *testing.T) {
	for _, values := range [][]string{{"same", "same"}, {"left", "right"}} {
		t.Run(strings.Join(values, "-"), func(t *testing.T) {
			root := t.TempDir()
			results := make(chan string, len(values))
			for _, value := range values {
				value := value
				go func() {
					cmd := exec.Command(os.Args[0], "-test.run=^TestStoreCreateJSONSubprocessHelper$")
					cmd.Env = append(os.Environ(), "STORE_CREATE_HELPER=1", "STORE_CREATE_ROOT="+root, "STORE_CREATE_VALUE="+value)
					out, err := cmd.CombinedOutput()
					if err != nil {
						results <- "error: " + err.Error() + ": " + string(out)
						return
					}
					results <- string(out)
				}()
			}
			created := 0
			for range values {
				result := <-results
				if strings.Contains(result, "STORE_CREATE=created") {
					created++
					continue
				}
				if !strings.Contains(result, "STORE_CREATE=exists") {
					t.Fatal(result)
				}
			}
			if created != 1 {
				t.Fatalf("subprocess winners=%d", created)
			}
			data, err := os.ReadFile(filepath.Join(root, "commit.json"))
			if err != nil {
				t.Fatal(err)
			}
			sum := sha256.Sum256(data)
			var value map[string]string
			if err := New(root, core.StorageLimits{}).ReadJSON("commit.json", 1024, &value); err != nil {
				t.Fatal(err)
			}
			if value["winner"] != values[0] && value["winner"] != values[1] {
				t.Fatalf("unexpected subprocess winner: %#v", value)
			}
			if hex.EncodeToString(sum[:]) == "" || len(data) == 0 {
				t.Fatalf("missing persisted hash/size: hash=%x bytes=%d", sum, len(data))
			}
		})
	}
}
