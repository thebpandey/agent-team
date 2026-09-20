package install

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"github.com/thebpandey/agent-team/vnext/internal/core"
	"github.com/thebpandey/agent-team/vnext/internal/store"
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

func TestLifecycleOperationsRestoreAfterLateAndManifestFailures(t *testing.T) {
	for _, operation := range []string{"update", "rollback", "uninstall"} {
		for _, failure := range []string{"late-file", "manifest"} {
			t.Run(operation+"/"+failure, func(t *testing.T) {
				layout, release1, release2, current := installedFixture(t)
				if operation == "rollback" {
					outcome, err := Update(context.Background(), layout, release2, current.Revision)
					if err != nil {
						t.Fatal(err)
					}
					current = outcome.Manifest
				}
				before := cloneManifest(current)
				beforeInfo, statErr := os.Stat(layout.BinaryPath)
				if statErr != nil {
					t.Fatal(statErr)
				}
				if failure == "late-file" {
					lifecycleMutationHook = func(op string, index int) error {
						if op == operation && index == 1 {
							return errors.New("injected late file failure")
						}
						return nil
					}
					defer func() { lifecycleMutationHook = nil }()
				} else {
					manifestWriteHook = func() error { return errors.New("injected manifest failure") }
					defer func() { manifestWriteHook = nil }()
				}
				var err error
				switch operation {
				case "update":
					_, err = Update(context.Background(), layout, release2, current.Revision)
				case "rollback":
					_, err = Rollback(context.Background(), layout, release1.Version, current.Revision)
				case "uninstall":
					_, _, err = Uninstall(context.Background(), layout, current.Revision)
				}
				if err == nil {
					t.Fatal("injected failure accepted")
				}
				got, readErr := NewManifestStore(layout).Read(context.Background())
				if readErr != nil || !reflect.DeepEqual(got, before) {
					t.Fatalf("manifest changed: %#v, %v", got, readErr)
				}
				if err := verifyManifestFiles(before, nil); err != nil {
					t.Fatal(err)
				}
				afterInfo, statErr := os.Stat(layout.BinaryPath)
				if statErr != nil || afterInfo.Mode().Perm() != beforeInfo.Mode().Perm() {
					t.Fatalf("mode changed: %v, %v", afterInfo, statErr)
				}
				if _, err := os.Lstat(filepath.Join(layout.DataRoot, filepath.FromSlash(installAttemptPath))); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("journal retained: %v", err)
				}
			})
		}
	}
}

func TestUpdateRecoversCrashAndRetryIsIdempotent(t *testing.T) {
	layout, _, release2, current := installedFixture(t)
	lifecycleInterruptHook = func(op string, index int) bool { return op == "update" && index == 0 }
	t.Cleanup(func() { lifecycleInterruptHook = nil })
	if _, err := Update(context.Background(), layout, release2, current.Revision); !errors.Is(err, core.ErrTransition) {
		t.Fatal(err)
	}
	lifecycleInterruptHook = nil
	outcome, err := Update(context.Background(), layout, release2, current.Revision)
	if err != nil || outcome.Manifest.Version != release2.Version {
		t.Fatal(outcome, err)
	}
	if _, err := os.Lstat(filepath.Join(layout.DataRoot, filepath.FromSlash(installAttemptPath))); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("journal retained: %v", err)
	}
}

func TestUpdateRecoveryRefusesForeignJournalPath(t *testing.T) {
	layout, _, release2, current := installedFixture(t)
	lifecycleInterruptHook = func(op string, index int) bool { return op == "update" && index == 0 }
	t.Cleanup(func() { lifecycleInterruptHook = nil })
	if _, err := Update(context.Background(), layout, release2, current.Revision); !errors.Is(err, core.ErrTransition) {
		t.Fatal(err)
	}
	lifecycleInterruptHook = nil
	journal, err := readLifecycleJournal(layout)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(journal.Mutations[0].Path, []byte("foreign"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Update(context.Background(), layout, release2, current.Revision); !errors.Is(err, core.ErrRevision) {
		t.Fatalf("foreign recovery = %v", err)
	}
	if got, _ := os.ReadFile(journal.Mutations[0].Path); string(got) != "foreign" {
		t.Fatalf("foreign path changed: %q", got)
	}
	if _, err := os.Stat(filepath.Join(layout.DataRoot, filepath.FromSlash(installAttemptPath))); err != nil {
		t.Fatalf("journal removed: %v", err)
	}
}

func TestSeparateUpdateStoresHaveOneMutationWinner(t *testing.T) {
	layout, _, release2, current := installedFixture(t)
	entered, resume := make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	lifecycleMutationHook = func(op string, index int) error {
		if op == "update" && index == 0 && calls.Add(1) == 1 {
			close(entered)
			<-resume
		}
		return nil
	}
	t.Cleanup(func() { lifecycleMutationHook = nil })
	errs := make(chan error, 2)
	run := func() { _, err := Update(context.Background(), layout, release2, current.Revision); errs <- err }
	go run()
	<-entered
	go run()
	select {
	case <-errs:
		t.Fatal("second update escaped durable guard")
	case <-time.After(20 * time.Millisecond):
	}
	close(resume)
	a, b := <-errs, <-errs
	if (a == nil) == (b == nil) {
		t.Fatalf("errors = %v, %v", a, b)
	}
	if calls.Load() != 1 {
		t.Fatalf("mutation calls = %d", calls.Load())
	}
}

func TestLifecycleRejectsCorruptJournalWithoutMutation(t *testing.T) {
	layout, _, release2, current := installedFixture(t)
	path := filepath.Join(layout.DataRoot, filepath.FromSlash(installAttemptPath))
	if err := os.WriteFile(path, []byte(`{"schema":1,"operation":"update","owner":{}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Update(context.Background(), layout, release2, current.Revision); !errors.Is(err, core.ErrRevision) {
		t.Fatalf("corrupt journal = %v", err)
	}
	if err := verifyManifestFiles(current, nil); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(path); err != nil || len(got) == 0 {
		t.Fatalf("corrupt journal removed: %q, %v", got, err)
	}
}

func installedFixture(t *testing.T) (Layout, Release, Release, InstallManifest) {
	t.Helper()
	layout, first := internalFixture(t, "first")
	secondRoot := filepath.Join(filepath.Dir(layout.DataRoot), "release-two")
	_, second := internalFixtureAt(t, secondRoot, "second")
	second.Version = "2.0.0"
	second.Revision = "abcdef0123456789abcdef0123456789abcdef01"
	outcome, err := Install(context.Background(), layout, first, []Host{Codex}, 0)
	if err != nil {
		t.Fatal(err)
	}
	return layout, first, second, outcome.Manifest
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
	mutation, err := prepareMutation(layout, desired[0].owned.Path, data, 0o700, false, true, newJournalBudget())
	if err != nil {
		t.Fatal(err)
	}
	if err := atomicCreate(layout, desired[0].owned.Path, data, 0o700); err != nil {
		t.Fatal(err)
	}
	manifest := InstallManifest{Schema: 1, Revision: 1, Version: release.Version, ReleaseRevision: release.Revision, Hosts: []Host{Codex}}
	for _, file := range desired {
		manifest.Files = append(manifest.Files, file.owned)
	}
	owner := store.MutationOwner{Token: "0123456789abcdef0123456789abcdef", Scope: "install", OperationID: "crashed", Host: "test", PID: 1, ProcessStart: "start", AcquiredAt: "acquired", HeartbeatAt: "heartbeat"}
	journal := lifecycleJournal{Schema: 1, Operation: "install", Owner: owner, Intended: manifest, Mutations: []lifecycleMutation{mutation}, Progress: 1}
	if err := writeLifecycleJournal(layout, journal); err != nil {
		t.Fatal(err)
	}
	if _, err := Install(context.Background(), layout, release, []Host{Codex}, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(layout.DataRoot, filepath.FromSlash(installAttemptPath))); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("attempt ledger retained: %v", err)
	}
}

func TestLifecycleJournalBudgetRejectsBeforeMutation(t *testing.T) {
	const oversize = 13 << 20
	t.Run("install replacement", func(t *testing.T) {
		layout, release := internalFixture(t, "oversize-install")
		resizeReleaseFile(t, &release.Binary, oversize)
		if _, err := Install(context.Background(), layout, release, []Host{Codex}, 0); !errors.Is(err, core.ErrRevision) {
			t.Fatalf("install error = %v", err)
		}
		assertNoJournal(t, layout)
		if _, err := os.Lstat(layout.BinaryPath); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("install mutated binary: %v", err)
		}
	})

	t.Run("update replacement", func(t *testing.T) {
		layout, _, release, current := installedFixture(t)
		resizeReleaseFile(t, &release.Binary, oversize)
		if _, err := Update(context.Background(), layout, release, current.Revision); !errors.Is(err, core.ErrRevision) {
			t.Fatalf("update error = %v", err)
		}
		assertLifecycleUnchanged(t, layout, current)
	})

	t.Run("rollback preimage", func(t *testing.T) {
		layout, first, second, current := installedFixture(t)
		outcome, err := Update(context.Background(), layout, second, current.Revision)
		if err != nil {
			t.Fatal(err)
		}
		current = resizeManagedFile(t, layout, outcome.Manifest, BinaryRole, oversize)
		if _, err := Rollback(context.Background(), layout, first.Version, current.Revision); !errors.Is(err, core.ErrRevision) {
			t.Fatalf("rollback error = %v", err)
		}
		assertLifecycleUnchanged(t, layout, current)
	})

	t.Run("uninstall preimage", func(t *testing.T) {
		layout, _, _, current := installedFixture(t)
		current = resizeManagedFile(t, layout, current, BinaryRole, oversize)
		if _, _, err := Uninstall(context.Background(), layout, current.Revision); !errors.Is(err, core.ErrRevision) {
			t.Fatalf("uninstall error = %v", err)
		}
		assertLifecycleUnchanged(t, layout, current)
	})

	t.Run("aggregate preimages", func(t *testing.T) {
		layout, _, _, current := installedFixture(t)
		current = resizeManagedFile(t, layout, current, BinaryRole, 6<<20)
		current = resizeManagedFile(t, layout, current, ContractRole, 6<<20)
		if _, _, err := Uninstall(context.Background(), layout, current.Revision); !errors.Is(err, core.ErrRevision) {
			t.Fatalf("uninstall error = %v", err)
		}
		assertLifecycleUnchanged(t, layout, current)
	})
}

func TestLifecycleJournalBudgetBoundary(t *testing.T) {
	budget := newJournalBudget()
	boundary := int64(installJournalLimit-journalMetadataSize) / 4 * 3
	if err := budget.reserve(boundary); err != nil {
		t.Fatalf("boundary rejected: %v", err)
	}
	if err := budget.reserve(1); !errors.Is(err, core.ErrRevision) {
		t.Fatalf("overflow accepted: %v", err)
	}
}

func resizeReleaseFile(t *testing.T, file *ReleaseFile, size int64) {
	t.Helper()
	if err := os.Truncate(file.Path, size); err != nil {
		t.Fatal(err)
	}
	digest, bytes, err := sha256File(file.Path)
	if err != nil {
		t.Fatal(err)
	}
	file.SHA256, file.Bytes = digest, bytes
}

func resizeManagedFile(t *testing.T, layout Layout, manifest InstallManifest, role FileRole, size int64) InstallManifest {
	t.Helper()
	next := cloneManifest(manifest)
	found := false
	for index := range next.Files {
		if next.Files[index].Role != role {
			continue
		}
		if err := os.Truncate(next.Files[index].Path, size); err != nil {
			t.Fatal(err)
		}
		digest, bytes, err := sha256File(next.Files[index].Path)
		if err != nil {
			t.Fatal(err)
		}
		next.Files[index].SHA256, next.Files[index].Bytes = digest, bytes
		found = true
		break
	}
	if !found {
		t.Fatal("managed role not found")
	}
	next.Revision = 0
	outcome, err := NewManifestStore(layout).CompareAndSwap(context.Background(), manifest.Revision, next)
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range outcome.Manifest.Files {
		if file.Role == role && !diskMatches(file.Path, file.SHA256, file.Bytes) {
			t.Fatal("resized managed file is not authoritative")
		}
	}
	return outcome.Manifest
}

func assertLifecycleUnchanged(t *testing.T, layout Layout, expected InstallManifest) {
	t.Helper()
	got, err := NewManifestStore(layout).Read(context.Background())
	if err != nil || !reflect.DeepEqual(got, expected) {
		t.Fatalf("manifest drift: %#v, %v", got, err)
	}
	if err := verifyManifestFiles(expected, nil); err != nil {
		t.Fatal(err)
	}
	assertNoJournal(t, layout)
}

func assertNoJournal(t *testing.T, layout Layout) {
	t.Helper()
	if _, err := os.Lstat(filepath.Join(layout.DataRoot, filepath.FromSlash(installAttemptPath))); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("journal drift: %v", err)
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
	return layout, Release{Version: "1.0.0", Revision: "0123456789abcdef0123456789abcdef01234567", Binary: write("binary", name+" binary"), Contract: write("contract", name+" contract"), Entrypoints: map[Host]ReleaseFile{Codex: write("codex.md", name+" codex"), Claude: write("claude.md", name+" claude")}}
}
