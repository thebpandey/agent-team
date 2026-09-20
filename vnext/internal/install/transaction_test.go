package install

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func TestInstallSeparateStoresHaveOneWinnerAndNoForeignMutation(t *testing.T) {
	layout, first := internalFixture(t, "first")
	_, second := internalFixtureAt(t, filepath.Join(filepath.Dir(layout.DataRoot), "second"), "second")
	entered := make(chan struct{})
	proceed := make(chan struct{})
	var calls atomic.Int32
	installMutationHook = func() {
		if calls.Add(1) == 1 {
			close(entered)
			<-proceed
		}
	}
	defer func() { installMutationHook = nil }()
	type result struct{ err error }
	results := make(chan result, 2)
	go func() {
		_, err := Install(context.Background(), layout, first, []Host{Codex}, 0)
		results <- result{err}
	}()
	<-entered
	go func() {
		_, err := Install(context.Background(), layout, second, []Host{Codex}, 0)
		results <- result{err}
	}()
	select {
	case <-results:
		t.Fatal("second install escaped durable guard")
	case <-time.After(20 * time.Millisecond):
	}
	close(proceed)
	a, b := <-results, <-results
	if (a.err == nil) == (b.err == nil) {
		t.Fatalf("errors = %v, %v; want one winner", a.err, b.err)
	}
	want, _ := os.ReadFile(first.Binary.Path)
	got, err := os.ReadFile(layout.BinaryPath)
	if err != nil || string(got) != string(want) {
		t.Fatalf("installed binary = %q, %v", got, err)
	}
}

func TestInstallCleansCreatedFilesAfterInjectedCreateFailure(t *testing.T) {
	layout, release := internalFixture(t, "release")
	installCreateHook = func(index int) error {
		if index == 1 {
			return errors.New("injected create failure")
		}
		return nil
	}
	defer func() { installCreateHook = nil }()
	if _, err := Install(context.Background(), layout, release, []Host{Codex}, 0); err == nil {
		t.Fatal("injected create failure accepted")
	}
	for _, file := range releaseFiles(layout, release, []Host{Codex}) {
		if _, err := os.Lstat(file.owned.Path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("attempt file retained: %s (%v)", file.owned.Path, err)
		}
	}
}

func TestInstallCleansCreatedFilesAfterManifestFailure(t *testing.T) {
	layout, release := internalFixture(t, "release")
	manifestWriteHook = func() error { return errors.New("injected manifest failure") }
	defer func() { manifestWriteHook = nil }()
	if _, err := Install(context.Background(), layout, release, []Host{Codex}, 0); err == nil {
		t.Fatal("injected manifest failure accepted")
	}
	for _, file := range releaseFiles(layout, release, []Host{Codex}) {
		if _, err := os.Lstat(file.owned.Path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("attempt file retained: %s (%v)", file.owned.Path, err)
		}
	}
}

func TestInstallRecoversInterruptedAttempt(t *testing.T) {
	layout, release := internalFixture(t, "release")
	desired := releaseFiles(layout, release, []Host{Codex})
	data, _ := os.ReadFile(desired[0].source.Path)
	if err := atomicCreate(layout, desired[0].owned.Path, data, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := writeInstallAttempt(layout, installAttempt{Schema: 1, ExpectedRevision: 0, Created: []OwnedFile{desired[0].owned}}); err != nil {
		t.Fatal(err)
	}
	if _, err := Install(context.Background(), layout, release, []Host{Codex}, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(layout.DataRoot, filepath.FromSlash(installAttemptPath))); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("attempt ledger retained: %v", err)
	}
}

func internalFixture(t *testing.T, name string) (Layout, Release) {
	t.Helper()
	root := t.TempDir()
	return internalFixtureAt(t, root, name)
}

func internalFixtureAt(t *testing.T, root, name string) (Layout, Release) {
	t.Helper()
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	layout, err := ResolveLayout("linux", map[string]string{"XDG_DATA_HOME": filepath.Join(root, "data"), "CODEX_HOME": filepath.Join(root, "codex"), "CLAUDE_HOME": filepath.Join(root, "claude")})
	if err != nil {
		t.Fatal(err)
	}
	write := func(filename, content string) ReleaseFile {
		path := filepath.Join(root, filename)
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		digest, size, err := sha256File(path)
		if err != nil {
			t.Fatal(err)
		}
		return ReleaseFile{Path: path, SHA256: digest, Bytes: size}
	}
	return layout, Release{Version: "1.0.0", Binary: write("binary", name+" binary"), Contract: write("contract", name+" contract"), Entrypoints: map[Host]ReleaseFile{Codex: write("codex.md", name+" codex"), Claude: write("claude.md", name+" claude")}}
}
