package install

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"

	"github.com/thebpandey/agent-team/vnext/internal/core"
	"github.com/thebpandey/agent-team/vnext/internal/store"
)

const manifestLimit = 1 << 20

var manifestLocks sync.Map
var manifestWriteHook func() error

func NewManifestStore(layout Layout) *ManifestStore {
	path := layout.ManifestPath
	key, _ := filepath.Abs(path)
	lock, _ := manifestLocks.LoadOrStore(key, &sync.Mutex{})
	return &ManifestStore{Root: layout.DataRoot, Path: path, mu: lock.(*sync.Mutex)}
}

func (s *ManifestStore) Read(ctx context.Context) (InstallManifest, error) {
	if ctx == nil || ctx.Err() != nil || s == nil || !absoluteClean(s.Path) {
		return InstallManifest{}, core.ErrPath
	}
	var manifest InstallManifest
	state := store.New(filepath.Dir(s.Path), core.StorageLimits{CanonicalBytes: manifestLimit})
	if err := state.ReadJSON(filepath.Base(s.Path), manifestLimit, &manifest); err != nil {
		return InstallManifest{}, err
	}
	if err := validateManifest(manifest); err != nil {
		return InstallManifest{}, err
	}
	return cloneManifest(manifest), nil
}

func (s *ManifestStore) CompareAndSwap(ctx context.Context, expected uint64, next InstallManifest) (CASOutcome, error) {
	if s == nil {
		return CASOutcome{}, core.ErrPath
	}
	release, err := store.AcquireProjectMutation(ctx, s.Root)
	if err != nil {
		return CASOutcome{}, err
	}
	defer func() { _ = release() }()
	return s.compareAndSwapLocked(ctx, expected, next)
}

func (s *ManifestStore) compareAndSwapLocked(ctx context.Context, expected uint64, next InstallManifest) (CASOutcome, error) {
	if ctx == nil || ctx.Err() != nil || s == nil || s.mu == nil || !absoluteClean(s.Path) {
		return CASOutcome{}, core.ErrPath
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	current, err := s.Read(ctx)
	absent := errors.Is(err, fs.ErrNotExist)
	if err != nil && !absent {
		return CASOutcome{}, err
	}
	observed := current.Revision
	if expected != observed || (absent && expected != 0) {
		return CASOutcome{Kind: CASStale, Manifest: current, ExpectedRevision: expected, ObservedRevision: observed, Retained: ownedPaths(current)}, core.ErrRevision
	}
	next.Schema = 1
	next.Revision = 0
	if err := validateManifest(next); err != nil {
		return CASOutcome{}, err
	}
	if !absent && reflect.DeepEqual(manifestIdentity(current), manifestIdentity(next)) {
		return CASOutcome{Kind: CASDuplicate, Manifest: current, ExpectedRevision: expected, ObservedRevision: observed, Idempotent: true, Retained: ownedPaths(current)}, nil
	}
	next.Revision = observed + 1
	if manifestWriteHook != nil {
		if err := manifestWriteHook(); err != nil {
			return CASOutcome{}, err
		}
	}
	state := store.New(filepath.Dir(s.Path), core.StorageLimits{CanonicalBytes: manifestLimit})
	if _, err := state.WriteJSON(filepath.Base(s.Path), next, manifestLimit); err != nil {
		return CASOutcome{}, err
	}
	kind := CASCreated
	if !absent {
		kind = CASUpdated
	}
	return CASOutcome{Kind: kind, Manifest: cloneManifest(next), ExpectedRevision: expected, ObservedRevision: next.Revision}, nil
}

func VerifyRelease(release Release) error {
	if strings.TrimSpace(release.Version) == "" || len(release.Entrypoints) != 2 {
		return fmt.Errorf("release version and both host entrypoints are required")
	}
	files := []ReleaseFile{release.Binary, release.Contract, release.Entrypoints[Codex], release.Entrypoints[Claude]}
	for _, file := range files {
		if !validSHA256(file.SHA256) || file.Bytes < 0 {
			return fmt.Errorf("invalid release file metadata")
		}
		digest, size, err := sha256File(file.Path)
		if err != nil || digest != file.SHA256 || size != file.Bytes {
			return fmt.Errorf("release file verification failed")
		}
	}
	return nil
}

func sha256File(path string) (string, int64, error) {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() {
		return "", 0, fmt.Errorf("release artifact is not a regular file")
	}
	file, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer file.Close()
	hash := sha256.New()
	size, err := io.Copy(hash, file)
	if err != nil {
		return "", 0, err
	}
	return hex.EncodeToString(hash.Sum(nil)), size, nil
}

func validateManifest(manifest InstallManifest) error {
	if manifest.Schema != 1 || strings.TrimSpace(manifest.Version) == "" {
		return core.ErrSettings
	}
	seenHosts := map[Host]bool{}
	for _, host := range manifest.Hosts {
		if host != Codex && host != Claude || seenHosts[host] {
			return core.ErrSettings
		}
		seenHosts[host] = true
	}
	for _, file := range manifest.Files {
		if !absoluteClean(file.Path) || !validSHA256(file.SHA256) || file.Bytes < 0 || file.Version == "" || (file.Role != BinaryRole && file.Role != ContractRole && file.Role != EntrypointRole) {
			return core.ErrSettings
		}
	}
	for _, backup := range manifest.Backups {
		if !absoluteClean(backup.Path) || !validSHA256(backup.SHA256) || backup.Bytes < 0 || backup.Version == "" {
			return core.ErrSettings
		}
	}
	return nil
}

func validSHA256(value string) bool {
	if len(value) != 64 || value != strings.ToLower(value) {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func manifestIdentity(manifest InstallManifest) InstallManifest {
	manifest.Revision = 0
	manifest.Hosts = append([]Host(nil), manifest.Hosts...)
	manifest.Files = append([]OwnedFile(nil), manifest.Files...)
	manifest.Backups = append([]Backup(nil), manifest.Backups...)
	sort.Slice(manifest.Hosts, func(i, j int) bool { return manifest.Hosts[i] < manifest.Hosts[j] })
	sort.Slice(manifest.Files, func(i, j int) bool { return fmt.Sprint(manifest.Files[i]) < fmt.Sprint(manifest.Files[j]) })
	sort.Slice(manifest.Backups, func(i, j int) bool { return fmt.Sprint(manifest.Backups[i]) < fmt.Sprint(manifest.Backups[j]) })
	return manifest
}

func cloneManifest(manifest InstallManifest) InstallManifest {
	manifest.Hosts = append([]Host(nil), manifest.Hosts...)
	manifest.Files = append([]OwnedFile(nil), manifest.Files...)
	manifest.Backups = append([]Backup(nil), manifest.Backups...)
	return manifest
}

func ownedPaths(manifest InstallManifest) []string {
	paths := make([]string, 0, len(manifest.Files)+len(manifest.Backups))
	for _, file := range manifest.Files {
		paths = append(paths, file.Path)
	}
	for _, backup := range manifest.Backups {
		paths = append(paths, backup.Path)
	}
	return paths
}
