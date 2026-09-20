package install

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/thebpandey/agent-team/vnext/internal/core"
	"github.com/thebpandey/agent-team/vnext/internal/store"
)

const installAttemptPath = ".agent-team/install-attempt.json"

var installMutationHook func()
var installCreateHook func(int) error

type installAttempt struct {
	Schema           int         `json:"schema"`
	ExpectedRevision uint64      `json:"expected_revision"`
	Created          []OwnedFile `json:"created"`
}

func Install(ctx context.Context, layout Layout, release Release, hosts []Host, expected uint64) (CASOutcome, error) {
	releaseGuard, err := store.AcquireProjectMutation(ctx, layout.DataRoot)
	if err != nil {
		return CASOutcome{}, err
	}
	defer func() { _ = releaseGuard() }()
	if installMutationHook != nil {
		installMutationHook()
	}
	return installLocked(ctx, layout, release, hosts, expected)
}

func installLocked(ctx context.Context, layout Layout, release Release, hosts []Host, expected uint64) (CASOutcome, error) {
	if err := validateLifecycle(ctx, layout, release); err != nil {
		return CASOutcome{}, err
	}
	hosts, err := normalizeHosts(hosts)
	if err != nil || len(hosts) == 0 {
		return CASOutcome{}, core.ErrSettings
	}
	manifestStore := NewManifestStore(layout)
	if current, readErr := manifestStore.Read(ctx); readErr == nil {
		_ = removeInstallAttempt(layout)
		return CASOutcome{Kind: CASStale, Manifest: current, ExpectedRevision: expected, ObservedRevision: current.Revision, Retained: ownedPaths(current)}, core.ErrRevision
	} else if !errors.Is(readErr, fs.ErrNotExist) {
		return CASOutcome{}, readErr
	}
	if expected != 0 {
		return CASOutcome{Kind: CASStale, ExpectedRevision: expected}, core.ErrRevision
	}
	if err := recoverInstallAttempt(layout, expected); err != nil {
		return CASOutcome{}, err
	}
	desired := releaseFiles(layout, release, hosts)
	for _, file := range desired {
		if _, err := os.Lstat(file.owned.Path); !errors.Is(err, fs.ErrNotExist) {
			return CASOutcome{Retained: []string{file.owned.Path}}, core.ErrRevision
		}
	}
	attempt := installAttempt{Schema: 1, ExpectedRevision: expected}
	if err := writeInstallAttempt(layout, attempt); err != nil {
		return CASOutcome{}, err
	}
	fail := func(cause error) (CASOutcome, error) {
		if cleanupErr := cleanupInstallAttempt(layout, attempt); cleanupErr != nil {
			return CASOutcome{Retained: ownedFilePaths(attempt.Created)}, fmt.Errorf("%v; install cleanup: %w", cause, cleanupErr)
		}
		return CASOutcome{}, cause
	}
	for index, file := range desired {
		data, err := verifiedReleaseBytes(file.source)
		if err != nil {
			return fail(err)
		}
		mode := fs.FileMode(0o600)
		if file.owned.Role == BinaryRole {
			mode = 0o700
		}
		if installCreateHook != nil {
			if err := installCreateHook(index); err != nil {
				return fail(err)
			}
		}
		if err := atomicCreate(layout, file.owned.Path, data, mode); err != nil {
			return fail(err)
		}
		attempt.Created = append(attempt.Created, file.owned)
		if err := writeInstallAttempt(layout, attempt); err != nil {
			return fail(err)
		}
	}
	manifest := InstallManifest{Schema: 1, Version: release.Version, ReleaseRevision: release.Revision, Hosts: hosts}
	for _, file := range desired {
		manifest.Files = append(manifest.Files, file.owned)
	}
	outcome, err := manifestStore.compareAndSwapLocked(ctx, expected, manifest)
	if err != nil {
		return fail(err)
	}
	if err := removeInstallAttempt(layout); err != nil {
		return CASOutcome{Manifest: outcome.Manifest, Retained: ownedPaths(outcome.Manifest)}, err
	}
	return outcome, nil
}

func writeInstallAttempt(layout Layout, attempt installAttempt) error {
	_, err := store.New(layout.DataRoot, core.StorageLimits{CanonicalBytes: manifestLimit}).WriteJSON(installAttemptPath, attempt, manifestLimit)
	return err
}

func recoverInstallAttempt(layout Layout, expected uint64) error {
	var attempt installAttempt
	err := store.New(layout.DataRoot, core.StorageLimits{CanonicalBytes: manifestLimit}).ReadJSON(installAttemptPath, manifestLimit, &attempt)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil || attempt.Schema != 1 || attempt.ExpectedRevision != expected {
		return core.ErrRevision
	}
	return cleanupInstallAttempt(layout, attempt)
}

func cleanupInstallAttempt(layout Layout, attempt installAttempt) error {
	for _, file := range attempt.Created {
		if !diskMatches(file.Path, file.SHA256, file.Bytes) {
			if _, err := os.Lstat(file.Path); errors.Is(err, fs.ErrNotExist) {
				continue
			}
			return fmt.Errorf("%w: created file ownership changed: %s", core.ErrRevision, file.Path)
		}
		if err := removeOwnedPath(layout, file.Path); err != nil {
			return err
		}
	}
	return removeInstallAttempt(layout)
}

func removeInstallAttempt(layout Layout) error {
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

func ownedFilePaths(files []OwnedFile) []string {
	paths := make([]string, 0, len(files))
	for _, file := range files {
		paths = append(paths, file.Path)
	}
	return paths
}

func Update(ctx context.Context, layout Layout, release Release, expected uint64) (CASOutcome, error) {
	releaseGuard, err := store.AcquireProjectMutation(ctx, layout.DataRoot)
	if err != nil {
		return CASOutcome{}, err
	}
	defer func() { _ = releaseGuard() }()
	return updateLocked(ctx, layout, release, expected)
}

func updateLocked(ctx context.Context, layout Layout, release Release, expected uint64) (CASOutcome, error) {
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
		if err := AtomicReplace(layout, backup.Path, oldBytes, 0o600); err != nil {
			return CASOutcome{Retained: append(retained, target.owned.Path)}, err
		}
		data, err := verifiedReleaseBytes(target.source)
		if err != nil {
			return CASOutcome{Retained: append(retained, target.owned.Path)}, err
		}
		mode := fs.FileMode(0o600)
		if target.owned.Role == BinaryRole {
			mode = 0o700
		}
		if err := AtomicReplace(layout, target.owned.Path, data, mode); err != nil {
			return CASOutcome{Retained: append(retained, target.owned.Path)}, err
		}
		next.Backups = appendBackup(next.Backups, backup)
		next.Files[index] = target.owned
	}
	outcome, err := manifestStore.compareAndSwapLocked(ctx, expected, next)
	outcome.Retained = append([]string(nil), retained...)
	return outcome, err
}

func Rollback(ctx context.Context, layout Layout, version string, expected uint64) (CASOutcome, error) {
	releaseGuard, err := store.AcquireProjectMutation(ctx, layout.DataRoot)
	if err != nil {
		return CASOutcome{}, err
	}
	defer func() { _ = releaseGuard() }()
	return rollbackLocked(ctx, layout, version, expected)
}

func rollbackLocked(ctx context.Context, layout Layout, version string, expected uint64) (CASOutcome, error) {
	if ctx == nil || ctx.Err() != nil || ValidateLayout(layout) != nil || strings.TrimSpace(version) == "" {
		return CASOutcome{}, core.ErrSettings
	}
	manifestStore := NewManifestStore(layout)
	current, stale, err := currentManifest(ctx, manifestStore, expected)
	if err != nil {
		return stale, err
	}
	next := cloneManifest(current)
	retained := []string{}
	restored := 0
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
		if err := AtomicReplace(layout, next.Files[index].Path, data, mode); err != nil {
			return CASOutcome{Retained: append(retained, next.Files[index].Path)}, err
		}
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
	outcome, err := manifestStore.compareAndSwapLocked(ctx, expected, next)
	outcome.Retained = retained
	return outcome, err
}

func Uninstall(ctx context.Context, layout Layout, expected uint64) ([]string, CASOutcome, error) {
	releaseGuard, err := store.AcquireProjectMutation(ctx, layout.DataRoot)
	if err != nil {
		return nil, CASOutcome{}, err
	}
	defer func() { _ = releaseGuard() }()
	return uninstallLocked(ctx, layout, expected)
}

func uninstallLocked(ctx context.Context, layout Layout, expected uint64) ([]string, CASOutcome, error) {
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
	next.Files = nil
	for _, file := range current.Files {
		if !diskMatches(file.Path, file.SHA256, file.Bytes) {
			retained = append(retained, file.Path)
			next.Files = append(next.Files, file)
			continue
		}
		if err := removeOwnedPath(layout, file.Path); err != nil {
			retained = append(retained, file.Path)
			next.Files = append(next.Files, file)
		}
	}
	next.Backups = nil
	for _, backup := range current.Backups {
		if !diskMatches(backup.Path, backup.SHA256, backup.Bytes) {
			retained = append(retained, backup.Path)
			next.Backups = append(next.Backups, backup)
			continue
		}
		if err := removeOwnedPath(layout, backup.Path); err != nil {
			retained = append(retained, backup.Path)
			next.Backups = append(next.Backups, backup)
		}
	}
	outcome, err := manifestStore.compareAndSwapLocked(ctx, expected, next)
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
