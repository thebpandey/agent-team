package install

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/thebpandey/agent-team/vnext/internal/core"
	"github.com/thebpandey/agent-team/vnext/internal/store"
)

const (
	installAttemptPath  = ".agent-team/install-attempt.json"
	installJournalLimit = 16 << 20
)

var installMutationHook func()
var installCreateHook func(int) error
var lifecycleMutationHook func(string, int) error
var lifecycleInterruptHook func(string, int) bool

type lifecycleMutation struct {
	Path        string `json:"path"`
	Existed     bool   `json:"existed"`
	Preimage    []byte `json:"preimage,omitempty"`
	PreSHA256   string `json:"pre_sha256,omitempty"`
	PreMode     uint32 `json:"pre_mode,omitempty"`
	PostMode    uint32 `json:"post_mode,omitempty"`
	PostSHA256  string `json:"post_sha256,omitempty"`
	PostBytes   int64  `json:"post_bytes,omitempty"`
	PostAbsent  bool   `json:"post_absent,omitempty"`
	Exclusive   bool   `json:"exclusive,omitempty"`
	Replacement []byte `json:"replacement,omitempty"`
}

type lifecycleJournal struct {
	Schema           int                 `json:"schema"`
	Operation        string              `json:"operation"`
	ExpectedRevision uint64              `json:"expected_revision"`
	Owner            store.MutationOwner `json:"owner"`
	Previous         *InstallManifest    `json:"previous,omitempty"`
	Intended         InstallManifest     `json:"intended"`
	Mutations        []lifecycleMutation `json:"mutations"`
	Retained         []string            `json:"retained,omitempty"`
	Progress         int                 `json:"progress"`
}

func Install(ctx context.Context, layout Layout, release Release, hosts []Host, expected uint64) (CASOutcome, error) {
	releaseGuard, err := store.AcquireProjectMutation(ctx, layout.DataRoot, "install", fmt.Sprintf("install:%d:%s", expected, release.Revision))
	if err != nil {
		return CASOutcome{}, err
	}
	defer func() { _ = releaseGuard.Release() }()
	manifestStore := NewManifestStore(layout)
	if err := recoverLifecycleJournal(ctx, layout, manifestStore); err != nil {
		return CASOutcome{}, err
	}
	if installMutationHook != nil {
		installMutationHook()
	}
	return installLocked(ctx, layout, release, hosts, expected, releaseGuard.Owner())
}

func installLocked(ctx context.Context, layout Layout, release Release, hosts []Host, expected uint64, owner store.MutationOwner) (CASOutcome, error) {
	if err := validateLifecycle(ctx, layout, release); err != nil {
		return CASOutcome{}, err
	}
	hosts, err := normalizeHosts(hosts)
	if err != nil || len(hosts) == 0 {
		return CASOutcome{}, core.ErrSettings
	}
	manifestStore := NewManifestStore(layout)
	if current, readErr := manifestStore.Read(ctx); readErr == nil {
		return CASOutcome{Kind: CASStale, Manifest: current, ExpectedRevision: expected, ObservedRevision: current.Revision, Retained: ownedPaths(current)}, core.ErrRevision
	} else if !errors.Is(readErr, fs.ErrNotExist) {
		return CASOutcome{}, readErr
	}
	if expected != 0 {
		return CASOutcome{Kind: CASStale, ExpectedRevision: expected}, core.ErrRevision
	}
	desired := releaseFiles(layout, release, hosts)
	for _, file := range desired {
		if _, err := os.Lstat(file.owned.Path); !errors.Is(err, fs.ErrNotExist) {
			return CASOutcome{Retained: []string{file.owned.Path}}, core.ErrRevision
		}
	}
	manifest := InstallManifest{Schema: 1, Revision: 1, Version: release.Version, ReleaseRevision: release.Revision, Hosts: hosts}
	journal := lifecycleJournal{Schema: 1, Operation: "install", ExpectedRevision: expected, Owner: owner, Intended: manifest}
	for index, file := range desired {
		data, err := verifiedReleaseBytes(file.source)
		if err != nil {
			return CASOutcome{}, err
		}
		mode := fs.FileMode(0o600)
		if file.owned.Role == BinaryRole {
			mode = 0o700
		}
		mutation, err := prepareMutation(layout, file.owned.Path, data, mode, false, true)
		if err != nil {
			return CASOutcome{}, err
		}
		journal.Mutations = append(journal.Mutations, mutation)
		journal.Intended.Files = append(journal.Intended.Files, file.owned)
		_ = index
	}
	return executeLifecycleJournal(ctx, layout, manifestStore, &journal)
}

func prepareMutation(layout Layout, path string, replacement []byte, mode fs.FileMode, postAbsent, exclusive bool) (lifecycleMutation, error) {
	if ownedRoot(layout, path) == "" {
		return lifecycleMutation{}, core.ErrPath
	}
	mutation := lifecycleMutation{Path: path, Replacement: append([]byte(nil), replacement...), PostMode: uint32(mode.Perm()), PostAbsent: postAbsent, Exclusive: exclusive}
	if !postAbsent {
		sum := sha256.Sum256(replacement)
		mutation.PostSHA256, mutation.PostBytes = hex.EncodeToString(sum[:]), int64(len(replacement))
	}
	info, err := os.Lstat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return mutation, nil
	}
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return lifecycleMutation{}, core.ErrPath
	}
	mutation.Existed = true
	mutation.PreMode = uint32(info.Mode().Perm())
	mutation.Preimage, err = os.ReadFile(path)
	if err != nil {
		return lifecycleMutation{}, err
	}
	sum := sha256.Sum256(mutation.Preimage)
	mutation.PreSHA256 = hex.EncodeToString(sum[:])
	return mutation, nil
}

func executeLifecycleJournal(ctx context.Context, layout Layout, manifestStore *ManifestStore, journal *lifecycleJournal) (CASOutcome, error) {
	if err := validateLifecycleJournal(layout, *journal); err != nil {
		return CASOutcome{}, err
	}
	if err := writeLifecycleJournal(layout, *journal); err != nil {
		return CASOutcome{}, err
	}
	fail := func(cause error) (CASOutcome, error) {
		if err := restoreLifecycleJournal(layout, *journal); err != nil {
			return CASOutcome{Retained: journalPaths(*journal)}, fmt.Errorf("%v; lifecycle recovery: %w", cause, err)
		}
		if err := removeLifecycleJournal(layout); err != nil {
			return CASOutcome{}, err
		}
		return CASOutcome{}, cause
	}
	for index := range journal.Mutations {
		journal.Progress = index + 1
		if err := writeLifecycleJournal(layout, *journal); err != nil {
			return fail(err)
		}
		if journal.Operation == "install" && installCreateHook != nil {
			if err := installCreateHook(index); err != nil {
				return fail(err)
			}
		}
		if lifecycleMutationHook != nil {
			if err := lifecycleMutationHook(journal.Operation, index); err != nil {
				return fail(err)
			}
		}
		if err := applyLifecycleMutation(layout, journal.Mutations[index]); err != nil {
			return fail(err)
		}
		if lifecycleInterruptHook != nil && lifecycleInterruptHook(journal.Operation, index) {
			return CASOutcome{Retained: journalPaths(*journal)}, core.ErrTransition
		}
	}
	next := journal.Intended
	next.Revision = 0
	outcome, err := manifestStore.compareAndSwapLocked(ctx, journal.ExpectedRevision, next)
	if err != nil {
		return fail(err)
	}
	journal.Intended = outcome.Manifest
	if err := verifyAuthoritativeManifest(ctx, manifestStore, journal.Intended, journal.Retained); err != nil {
		return outcome, err
	}
	if err := removeLifecycleJournal(layout); err != nil {
		return outcome, err
	}
	return outcome, nil
}

func applyLifecycleMutation(layout Layout, mutation lifecycleMutation) error {
	if mutation.PostAbsent {
		return removeOwnedPath(layout, mutation.Path)
	}
	mode := fs.FileMode(mutation.PostMode)
	if mode == 0 {
		mode = 0o600
	}
	if mutation.Exclusive {
		return atomicCreate(layout, mutation.Path, mutation.Replacement, mode)
	}
	return AtomicReplace(layout, mutation.Path, mutation.Replacement, mode)
}

func recoverLifecycleJournal(ctx context.Context, layout Layout, manifestStore *ManifestStore) error {
	journal, err := readLifecycleJournal(layout)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil || validateLifecycleJournal(layout, journal) != nil {
		return core.ErrRevision
	}
	current, readErr := manifestStore.Read(ctx)
	if readErr == nil && reflect.DeepEqual(current, journal.Intended) {
		if err := verifyManifestFiles(current, journal.Retained); err != nil {
			return err
		}
		return removeLifecycleJournal(layout)
	}
	if journal.Previous == nil {
		if !errors.Is(readErr, fs.ErrNotExist) {
			return core.ErrRevision
		}
	} else if readErr != nil || !reflect.DeepEqual(current, *journal.Previous) {
		return core.ErrRevision
	}
	if err := restoreLifecycleJournal(layout, journal); err != nil {
		return err
	}
	if journal.Previous != nil {
		if err := verifyManifestFiles(*journal.Previous, journal.Retained); err != nil {
			return err
		}
	}
	return removeLifecycleJournal(layout)
}

func validateLifecycleJournal(layout Layout, journal lifecycleJournal) error {
	if journal.Schema != 1 || store.ValidateMutationOwner(journal.Owner) != nil || journal.Progress < 0 || journal.Progress > len(journal.Mutations) {
		return core.ErrRevision
	}
	switch journal.Operation {
	case "install", "update", "rollback", "uninstall":
	default:
		return core.ErrRevision
	}
	if validateManifest(journal.Intended) != nil || journal.Intended.Revision != journal.ExpectedRevision+1 {
		return core.ErrRevision
	}
	if journal.Previous == nil {
		if journal.ExpectedRevision != 0 || journal.Operation != "install" {
			return core.ErrRevision
		}
	} else if validateManifest(*journal.Previous) != nil || journal.Previous.Revision != journal.ExpectedRevision {
		return core.ErrRevision
	}
	for _, mutation := range journal.Mutations {
		if ownedRoot(layout, mutation.Path) == "" {
			return core.ErrPath
		}
		if mutation.Existed {
			if !validSHA256(mutation.PreSHA256) || digestContent(mutation.Preimage) != mutation.PreSHA256 {
				return core.ErrRevision
			}
		} else if len(mutation.Preimage) != 0 || mutation.PreSHA256 != "" || mutation.PreMode != 0 {
			return core.ErrRevision
		}
		if mutation.PostAbsent {
			if mutation.PostSHA256 != "" || mutation.PostBytes != 0 || len(mutation.Replacement) != 0 {
				return core.ErrRevision
			}
		} else if !validSHA256(mutation.PostSHA256) || mutation.PostBytes != int64(len(mutation.Replacement)) || digestContent(mutation.Replacement) != mutation.PostSHA256 {
			return core.ErrRevision
		}
		if mutation.Exclusive && (mutation.Existed || journal.Operation != "install") {
			return core.ErrRevision
		}
	}
	for _, path := range journal.Retained {
		if ownedRoot(layout, path) == "" {
			return core.ErrPath
		}
	}
	return nil
}

func digestContent(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

func restoreLifecycleJournal(layout Layout, journal lifecycleJournal) error {
	for index := journal.Progress - 1; index >= 0; index-- {
		mutation := journal.Mutations[index]
		if mutation.Existed {
			if diskMatches(mutation.Path, mutation.PreSHA256, int64(len(mutation.Preimage))) {
				continue
			}
			if mutation.PostAbsent {
				if _, err := os.Lstat(mutation.Path); !errors.Is(err, fs.ErrNotExist) {
					return core.ErrRevision
				}
			} else if !diskMatches(mutation.Path, mutation.PostSHA256, mutation.PostBytes) {
				return core.ErrRevision
			}
			if err := AtomicReplace(layout, mutation.Path, mutation.Preimage, fs.FileMode(mutation.PreMode)); err != nil {
				return err
			}
			continue
		}
		if _, err := os.Lstat(mutation.Path); errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if mutation.PostAbsent || !diskMatches(mutation.Path, mutation.PostSHA256, mutation.PostBytes) {
			return core.ErrRevision
		}
		if err := removeOwnedPath(layout, mutation.Path); err != nil {
			return err
		}
	}
	return nil
}

func verifyAuthoritativeManifest(ctx context.Context, manifestStore *ManifestStore, expected InstallManifest, retained []string) error {
	current, err := manifestStore.Read(ctx)
	if err != nil || !reflect.DeepEqual(current, expected) {
		return core.ErrRevision
	}
	return verifyManifestFiles(current, retained)
}

func verifyManifestFiles(manifest InstallManifest, retained []string) error {
	ignored := map[string]bool{}
	for _, path := range retained {
		ignored[path] = true
	}
	for _, file := range manifest.Files {
		if !ignored[file.Path] && !diskMatches(file.Path, file.SHA256, file.Bytes) {
			return fmt.Errorf("%w: installed file mismatch: %s", core.ErrRevision, file.Path)
		}
	}
	for _, backup := range manifest.Backups {
		if !ignored[backup.Path] && !diskMatches(backup.Path, backup.SHA256, backup.Bytes) {
			return fmt.Errorf("%w: backup mismatch: %s", core.ErrRevision, backup.Path)
		}
	}
	return nil
}

func writeLifecycleJournal(layout Layout, journal lifecycleJournal) error {
	_, err := store.New(layout.DataRoot, core.StorageLimits{CanonicalBytes: installJournalLimit}).WriteJSON(installAttemptPath, journal, installJournalLimit)
	return err
}

func readLifecycleJournal(layout Layout) (lifecycleJournal, error) {
	var journal lifecycleJournal
	err := store.New(layout.DataRoot, core.StorageLimits{CanonicalBytes: installJournalLimit}).ReadJSON(installAttemptPath, installJournalLimit, &journal)
	return journal, err
}

func removeLifecycleJournal(layout Layout) error {
	root, err := os.OpenRoot(layout.DataRoot)
	if err != nil {
		return err
	}
	defer root.Close()
	err = root.Remove(filepath.FromSlash(installAttemptPath))
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}

func journalPaths(journal lifecycleJournal) []string {
	paths := make([]string, 0, len(journal.Mutations))
	for _, mutation := range journal.Mutations {
		paths = append(paths, mutation.Path)
	}
	return paths
}

func manifestMismatches(manifest InstallManifest) []string {
	paths := []string{}
	for _, file := range manifest.Files {
		if !diskMatches(file.Path, file.SHA256, file.Bytes) {
			paths = append(paths, file.Path)
		}
	}
	for _, backup := range manifest.Backups {
		if !diskMatches(backup.Path, backup.SHA256, backup.Bytes) {
			paths = append(paths, backup.Path)
		}
	}
	return paths
}

func Update(ctx context.Context, layout Layout, release Release, expected uint64) (CASOutcome, error) {
	releaseGuard, err := store.AcquireProjectMutation(ctx, layout.DataRoot, "install", fmt.Sprintf("update:%d:%s", expected, release.Revision))
	if err != nil {
		return CASOutcome{}, err
	}
	defer func() { _ = releaseGuard.Release() }()
	manifestStore := NewManifestStore(layout)
	if err := recoverLifecycleJournal(ctx, layout, manifestStore); err != nil {
		return CASOutcome{}, err
	}
	return updateLocked(ctx, layout, release, expected, releaseGuard.Owner())
}

func updateLocked(ctx context.Context, layout Layout, release Release, expected uint64, owner store.MutationOwner) (CASOutcome, error) {
	if err := validateLifecycle(ctx, layout, release); err != nil {
		return CASOutcome{}, err
	}
	manifestStore := NewManifestStore(layout)
	current, stale, err := currentManifest(ctx, manifestStore, expected)
	if err != nil {
		return stale, err
	}
	desired := releaseFiles(layout, release, current.Hosts)
	next := cloneManifest(current)
	next.Version = release.Version
	next.ReleaseRevision = release.Revision
	retained := []string{}
	mutations := []lifecycleMutation{}
	for _, target := range desired {
		index := ownedIndex(next.Files, target.owned.Role, target.owned.Host)
		if index < 0 || !diskMatches(next.Files[index].Path, next.Files[index].SHA256, next.Files[index].Bytes) {
			retained = append(retained, target.owned.Path)
			continue
		}
		oldBytes, err := readRegular(next.Files[index].Path)
		if err != nil {
			retained = append(retained, target.owned.Path)
			continue
		}
		backup := Backup{Role: next.Files[index].Role, Host: next.Files[index].Host, Path: backupPath(layout, next.Files[index]), SHA256: next.Files[index].SHA256, Version: next.Files[index].Version, Revision: next.Files[index].Revision, Bytes: next.Files[index].Bytes}
		backupExists := false
		if _, statErr := os.Lstat(backup.Path); statErr == nil {
			if !diskMatches(backup.Path, backup.SHA256, backup.Bytes) {
				retained = append(retained, target.owned.Path, backup.Path)
				continue
			}
			backupExists = true
		} else if !errors.Is(statErr, fs.ErrNotExist) {
			return CASOutcome{Retained: append(retained, target.owned.Path)}, statErr
		}
		data, err := verifiedReleaseBytes(target.source)
		if err != nil {
			return CASOutcome{Retained: append(retained, target.owned.Path)}, err
		}
		mode := fs.FileMode(0o600)
		if target.owned.Role == BinaryRole {
			mode = 0o700
		}
		targetMutation, err := prepareMutation(layout, target.owned.Path, data, mode, false, false)
		if err != nil {
			return CASOutcome{Retained: append(retained, target.owned.Path)}, err
		}
		if !backupExists {
			backupMutation, err := prepareMutation(layout, backup.Path, oldBytes, 0o600, false, false)
			if err != nil {
				return CASOutcome{Retained: append(retained, target.owned.Path)}, err
			}
			mutations = append(mutations, backupMutation)
		}
		mutations = append(mutations, targetMutation)
		next.Backups = appendBackup(next.Backups, backup)
		next.Files[index] = target.owned
	}
	previous := cloneManifest(current)
	intended := cloneManifest(next)
	intended.Revision = expected + 1
	journal := lifecycleJournal{Schema: 1, Operation: "update", ExpectedRevision: expected, Owner: owner, Previous: &previous, Intended: intended, Mutations: mutations, Retained: retained}
	outcome, err := executeLifecycleJournal(ctx, layout, manifestStore, &journal)
	outcome.Retained = append([]string(nil), retained...)
	return outcome, err
}

func Rollback(ctx context.Context, layout Layout, version string, expected uint64) (CASOutcome, error) {
	releaseGuard, err := store.AcquireProjectMutation(ctx, layout.DataRoot, "install", fmt.Sprintf("rollback:%d:%s", expected, version))
	if err != nil {
		return CASOutcome{}, err
	}
	defer func() { _ = releaseGuard.Release() }()
	manifestStore := NewManifestStore(layout)
	if err := recoverLifecycleJournal(ctx, layout, manifestStore); err != nil {
		return CASOutcome{}, err
	}
	return rollbackLocked(ctx, layout, version, expected, releaseGuard.Owner())
}

func rollbackLocked(ctx context.Context, layout Layout, version string, expected uint64, owner store.MutationOwner) (CASOutcome, error) {
	if ctx == nil || ctx.Err() != nil || ValidateLayout(layout) != nil || strings.TrimSpace(version) == "" {
		return CASOutcome{}, core.ErrSettings
	}
	manifestStore := NewManifestStore(layout)
	current, stale, err := currentManifest(ctx, manifestStore, expected)
	if err != nil {
		return stale, err
	}
	next := cloneManifest(current)
	retained := manifestMismatches(current)
	restored := 0
	mutations := []lifecycleMutation{}
	for _, backup := range current.Backups {
		if backup.Version != version {
			continue
		}
		index := ownedIndex(next.Files, backup.Role, backup.Host)
		if index < 0 || !diskMatches(next.Files[index].Path, next.Files[index].SHA256, next.Files[index].Bytes) || !diskMatches(backup.Path, backup.SHA256, backup.Bytes) {
			if index >= 0 {
				retained = append(retained, next.Files[index].Path)
			}
			continue
		}
		data, err := readRegular(backup.Path)
		if err != nil {
			return CASOutcome{Retained: retained}, err
		}
		mode := fs.FileMode(0o600)
		if backup.Role == BinaryRole {
			mode = 0o700
		}
		mutation, err := prepareMutation(layout, next.Files[index].Path, data, mode, false, false)
		if err != nil {
			return CASOutcome{Retained: append(retained, next.Files[index].Path)}, err
		}
		mutations = append(mutations, mutation)
		next.Files[index].SHA256, next.Files[index].Bytes, next.Files[index].Version = backup.SHA256, backup.Bytes, backup.Version
		next.Files[index].Revision = backup.Revision
		restored++
	}
	if restored == 0 {
		return CASOutcome{Retained: retained}, core.ErrRevision
	}
	next.Version = version
	for _, backup := range current.Backups {
		if backup.Version == version {
			next.ReleaseRevision = backup.Revision
			break
		}
	}
	previous := cloneManifest(current)
	intended := cloneManifest(next)
	intended.Revision = expected + 1
	journal := lifecycleJournal{Schema: 1, Operation: "rollback", ExpectedRevision: expected, Owner: owner, Previous: &previous, Intended: intended, Mutations: mutations, Retained: retained}
	outcome, err := executeLifecycleJournal(ctx, layout, manifestStore, &journal)
	outcome.Retained = retained
	return outcome, err
}

func Uninstall(ctx context.Context, layout Layout, expected uint64) ([]string, CASOutcome, error) {
	releaseGuard, err := store.AcquireProjectMutation(ctx, layout.DataRoot, "install", fmt.Sprintf("uninstall:%d", expected))
	if err != nil {
		return nil, CASOutcome{}, err
	}
	defer func() { _ = releaseGuard.Release() }()
	manifestStore := NewManifestStore(layout)
	if err := recoverLifecycleJournal(ctx, layout, manifestStore); err != nil {
		return nil, CASOutcome{}, err
	}
	return uninstallLocked(ctx, layout, expected, releaseGuard.Owner())
}

func uninstallLocked(ctx context.Context, layout Layout, expected uint64, owner store.MutationOwner) ([]string, CASOutcome, error) {
	if ctx == nil || ctx.Err() != nil || ValidateLayout(layout) != nil {
		return nil, CASOutcome{}, core.ErrSettings
	}
	manifestStore := NewManifestStore(layout)
	current, stale, err := currentManifest(ctx, manifestStore, expected)
	if err != nil {
		return stale.Retained, stale, err
	}
	next := cloneManifest(current)
	retained := []string{}
	mutations := []lifecycleMutation{}
	next.Files = nil
	for _, file := range current.Files {
		if !diskMatches(file.Path, file.SHA256, file.Bytes) {
			retained = append(retained, file.Path)
			next.Files = append(next.Files, file)
			continue
		}
		mutation, err := prepareMutation(layout, file.Path, nil, 0, true, false)
		if err != nil {
			retained = append(retained, file.Path)
			next.Files = append(next.Files, file)
			continue
		}
		mutations = append(mutations, mutation)
	}
	next.Backups = nil
	for _, backup := range current.Backups {
		if !diskMatches(backup.Path, backup.SHA256, backup.Bytes) {
			retained = append(retained, backup.Path)
			next.Backups = append(next.Backups, backup)
			continue
		}
		mutation, err := prepareMutation(layout, backup.Path, nil, 0, true, false)
		if err != nil {
			retained = append(retained, backup.Path)
			next.Backups = append(next.Backups, backup)
			continue
		}
		mutations = append(mutations, mutation)
	}
	previous := cloneManifest(current)
	intended := cloneManifest(next)
	intended.Revision = expected + 1
	journal := lifecycleJournal{Schema: 1, Operation: "uninstall", ExpectedRevision: expected, Owner: owner, Previous: &previous, Intended: intended, Mutations: mutations, Retained: retained}
	outcome, err := executeLifecycleJournal(ctx, layout, manifestStore, &journal)
	outcome.Retained = append([]string(nil), retained...)
	return retained, outcome, err
}

func AtomicReplace(layout Layout, target string, data []byte, mode fs.FileMode) error {
	if err := ValidateLayout(layout); err != nil {
		return core.ErrPath
	}
	root := ownedRoot(layout, target)
	if root == "" {
		return core.ErrPath
	}
	relative, err := filepath.Rel(root, target)
	if err != nil || relative == "." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return core.ErrPath
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return err
	}
	opened, err := os.OpenRoot(root)
	if err != nil {
		return err
	}
	defer opened.Close()
	directory := filepath.Dir(relative)
	if err := opened.MkdirAll(directory, 0o700); err != nil {
		return err
	}
	if info, err := opened.Lstat(relative); err == nil && (info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular()) {
		return core.ErrPath
	} else if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	var temporary string
	var file *os.File
	for attempt := 0; attempt < 32; attempt++ {
		var token [16]byte
		if _, err := rand.Read(token[:]); err != nil {
			return err
		}
		temporary = filepath.Join(directory, ".agent-team-install-"+hex.EncodeToString(token[:]))
		file, err = opened.OpenFile(temporary, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if errors.Is(err, fs.ErrExist) {
			continue
		}
		break
	}
	if err != nil || file == nil {
		return err
	}
	removeTemporary := true
	defer func() {
		if removeTemporary {
			_ = opened.Remove(temporary)
		}
	}()
	if written, err := file.Write(data); err != nil || written != len(data) {
		_ = file.Close()
		if err == nil {
			err = fmt.Errorf("short install write")
		}
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	if runtime.GOOS != "windows" {
		if err := file.Chmod(mode.Perm()); err != nil {
			_ = file.Close()
			return err
		}
	}
	if err := file.Close(); err != nil {
		return err
	}
	backoff := []time.Duration{0, 25 * time.Millisecond, 50 * time.Millisecond, 100 * time.Millisecond}
	for _, wait := range backoff {
		if wait > 0 {
			time.Sleep(wait)
		}
		if err = opened.Rename(temporary, relative); err == nil {
			removeTemporary = false
			return nil
		}
	}
	return err
}

func atomicCreate(layout Layout, target string, data []byte, mode fs.FileMode) error {
	root := ownedRoot(layout, target)
	if root == "" {
		return core.ErrPath
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return err
	}
	opened, err := os.OpenRoot(root)
	if err != nil {
		return err
	}
	defer opened.Close()
	relative, err := filepath.Rel(root, target)
	if err != nil || relative == "." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return core.ErrPath
	}
	directory := filepath.Dir(relative)
	if err := opened.MkdirAll(directory, 0o700); err != nil {
		return err
	}
	var token [16]byte
	if _, err := rand.Read(token[:]); err != nil {
		return err
	}
	temporary := filepath.Join(directory, ".agent-team-install-"+hex.EncodeToString(token[:]))
	file, err := opened.OpenFile(temporary, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	defer opened.Remove(temporary)
	written, writeErr := file.Write(data)
	if writeErr == nil && written != len(data) {
		writeErr = fmt.Errorf("short install write")
	}
	if writeErr == nil {
		writeErr = file.Sync()
	}
	if writeErr == nil && runtime.GOOS != "windows" {
		writeErr = file.Chmod(mode.Perm())
	}
	closeErr := file.Close()
	if writeErr != nil {
		return writeErr
	}
	if closeErr != nil {
		return closeErr
	}
	if err := opened.Link(temporary, relative); err != nil {
		return err
	}
	return opened.Remove(temporary)
}

type desiredFile struct {
	owned  OwnedFile
	source ReleaseFile
}

func releaseFiles(layout Layout, release Release, hosts []Host) []desiredFile {
	files := []desiredFile{
		{OwnedFile{Role: BinaryRole, Path: layout.BinaryPath, SHA256: release.Binary.SHA256, Version: release.Version, Revision: release.Revision, Bytes: release.Binary.Bytes}, release.Binary},
		{OwnedFile{Role: ContractRole, Path: layout.ContractPath, SHA256: release.Contract.SHA256, Version: release.Version, Revision: release.Revision, Bytes: release.Contract.Bytes}, release.Contract},
	}
	for _, host := range hosts {
		entrypoint := release.Entrypoints[host]
		files = append(files, desiredFile{OwnedFile{Role: EntrypointRole, Host: host, Path: filepath.Join(layout.SkillRoots[host], "agent-team-vnext", "SKILL.md"), SHA256: entrypoint.SHA256, Version: release.Version, Revision: release.Revision, Bytes: entrypoint.Bytes}, entrypoint})
	}
	return files
}

func validateLifecycle(ctx context.Context, layout Layout, release Release) error {
	if ctx == nil || ctx.Err() != nil {
		return core.ErrSettings
	}
	if err := ValidateLayout(layout); err != nil {
		return err
	}
	return VerifyRelease(release)
}

func normalizeHosts(hosts []Host) ([]Host, error) {
	seen := map[Host]bool{}
	out := append([]Host(nil), hosts...)
	for _, host := range out {
		if host != Codex && host != Claude || seen[host] {
			return nil, core.ErrSettings
		}
		seen[host] = true
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out, nil
}

func currentManifest(ctx context.Context, manifestStore *ManifestStore, expected uint64) (InstallManifest, CASOutcome, error) {
	current, err := manifestStore.Read(ctx)
	if err != nil {
		return InstallManifest{}, CASOutcome{}, err
	}
	if current.Revision != expected {
		outcome := CASOutcome{Kind: CASStale, Manifest: current, ExpectedRevision: expected, ObservedRevision: current.Revision, Retained: ownedPaths(current)}
		return InstallManifest{}, outcome, core.ErrRevision
	}
	return current, CASOutcome{}, nil
}

func verifiedReleaseBytes(file ReleaseFile) ([]byte, error) {
	if !diskMatches(file.Path, file.SHA256, file.Bytes) {
		return nil, core.ErrRevision
	}
	return readRegular(file.Path)
}

func readRegular(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() {
		return nil, core.ErrPath
	}
	return os.ReadFile(path)
}

func diskMatches(path, digest string, bytes int64) bool {
	gotDigest, gotBytes, err := sha256File(path)
	return err == nil && gotDigest == digest && gotBytes == bytes
}

func ownedIndex(files []OwnedFile, role FileRole, host Host) int {
	for i := range files {
		if files[i].Role == role && files[i].Host == host {
			return i
		}
	}
	return -1
}

func backupPath(layout Layout, file OwnedFile) string {
	host := string(file.Host)
	if host == "" {
		host = "shared"
	}
	name := fmt.Sprintf("%s-%s-%s.bak", file.Role, host, file.SHA256)
	return filepath.Join(layout.DataRoot, "backups", file.Version, name)
}

func appendBackup(backups []Backup, candidate Backup) []Backup {
	for _, backup := range backups {
		if backup.Role == candidate.Role && backup.Host == candidate.Host && backup.Version == candidate.Version && backup.SHA256 == candidate.SHA256 {
			return backups
		}
	}
	return append(backups, candidate)
}

func ownedRoot(layout Layout, target string) string {
	roots := []string{layout.DataRoot, layout.SkillRoots[Codex], layout.SkillRoots[Claude]}
	best := ""
	for _, root := range roots {
		if contained(root, target) && len(root) > len(best) {
			best = root
		}
	}
	return best
}

func removeOwnedPath(layout Layout, path string) error {
	if ownedRoot(layout, path) == "" {
		return core.ErrPath
	}
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() {
		return core.ErrPath
	}
	return os.Remove(path)
}
