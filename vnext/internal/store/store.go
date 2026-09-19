// Package store persists bounded canonical records beneath a rooted directory.
package store

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/thebpandey/agent-team/vnext/internal/core"
)

const maxStorageBytes int64 = 16 << 20

// ErrAlreadyExists marks a no-replace create whose canonical destination was
// already published. It also wraps fs.ErrExist for callers using standard
// filesystem error matching.
var ErrAlreadyExists = fmt.Errorf("already exists: %w", fs.ErrExist)

// Store is a root-relative, bounded persistence store.
type Store struct {
	Root   string
	Limits core.StorageLimits

	mu      sync.Mutex
	probe   func(*os.Root, string) (probeResult, error)
	verify  func(*os.Root, string, int64) (AtomicResult, error)
	replace func(*os.Root, string, string) error
	restore func(*os.Root, string, string) error
}

// AtomicResult describes the fully flushed bytes that replaced a destination.
type AtomicResult struct {
	Bytes  int64
	SHA256 string
}

// New creates a Store rooted at root. Directories are created only by writes.
func New(root string, limits core.StorageLimits) *Store {
	return &Store{Root: root, Limits: limits}
}

// ReadJSON decodes one bounded JSON document beneath the store root.
func (s *Store) ReadJSON(relative string, maxBytes int64, destination any) error {
	limit, err := s.limit(maxBytes)
	if err != nil {
		return err
	}
	relative, err = validateRelative(relative)
	if err != nil {
		return err
	}
	root, _, err := s.openRoot(false)
	if err != nil {
		return pathError("open root", s.Root, err)
	}
	defer root.Close()
	info, err := root.Lstat(relative)
	if err != nil {
		return pathError("lstat", relative, err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return fmt.Errorf("%w: %s is not a regular file", core.ErrPath, relative)
	}
	file, err := root.Open(relative)
	if err != nil {
		return pathError("open", relative, err)
	}
	defer file.Close()
	reader := &boundedReader{reader: file, remaining: limit}
	decoder := json.NewDecoder(reader)
	if err := decoder.Decode(destination); err != nil {
		return pathError("decode", relative, err)
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return pathError("decode", relative, errors.New("trailing JSON value"))
		}
		return pathError("decode", relative, err)
	}
	return nil
}

// WriteJSON streams value into a bounded temporary file before replacement.
func (s *Store) WriteJSON(relative string, value any, maxBytes int64) (AtomicResult, error) {
	return s.write(relative, maxBytes, func(writer io.Writer) error {
		return json.NewEncoder(writer).Encode(value)
	})
}

// CreateJSON publishes a fully-written, synced JSON document only if relative
// does not yet exist. Link is the commit primitive: unlike replacement it is
// an atomic no-replace operation across independent processes sharing a root.
func (s *Store) CreateJSON(relative string, value any, maxBytes int64) (AtomicResult, error) {
	limit, err := s.limit(maxBytes)
	if err != nil {
		return AtomicResult{}, err
	}
	relative, err = validateRelative(relative)
	if err != nil {
		return AtomicResult{}, err
	}
	root, _, err := s.openRoot(true)
	if err != nil {
		return AtomicResult{}, pathError("open root", s.Root, err)
	}
	defer root.Close()
	if directory := path.Dir(relative); directory != "." {
		if err := root.MkdirAll(directory, 0o700); err != nil {
			return AtomicResult{}, pathError("create parent", relative, err)
		}
	}
	if info, err := root.Lstat(relative); err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return AtomicResult{}, fmt.Errorf("%w: destination %s is a symbolic link", core.ErrPath, relative)
		}
		return AtomicResult{}, fmt.Errorf("%w: %s", ErrAlreadyExists, relative)
	} else if !errors.Is(err, os.ErrNotExist) {
		return AtomicResult{}, pathError("lstat", relative, err)
	}
	temporary, file, err := createOwnedTemp(root, path.Dir(relative), ".agent-team-create-")
	if err != nil {
		return AtomicResult{}, pathError("create temporary", relative, err)
	}
	defer func() { _ = removeOwned(root, temporary) }()
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		return AtomicResult{}, pathError("chmod temporary", relative, err)
	}
	writer := &boundedWriter{writer: file, remaining: limit}
	if err := json.NewEncoder(writer).Encode(value); err != nil {
		_ = file.Close()
		if errors.Is(err, errTooLarge) {
			return AtomicResult{}, limitError(relative, limit)
		}
		return AtomicResult{}, pathError("encode temporary", relative, err)
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return AtomicResult{}, pathError("flush temporary", relative, err)
	}
	if err := file.Close(); err != nil {
		return AtomicResult{}, pathError("close temporary", relative, err)
	}
	result, err := hashRootFile(root, temporary.name, limit)
	if err != nil {
		return AtomicResult{}, pathError("verify temporary", relative, err)
	}
	if err := root.Link(temporary.name, relative); err != nil {
		if errors.Is(err, fs.ErrExist) || errors.Is(err, os.ErrExist) {
			return AtomicResult{}, fmt.Errorf("%w: %s", ErrAlreadyExists, relative)
		}
		return AtomicResult{}, pathError("publish no-replace", relative, err)
	}
	confirmed, err := hashRootFile(root, relative, limit)
	if err != nil {
		return AtomicResult{}, pathError("verify publication", relative, err)
	}
	if confirmed != result {
		return AtomicResult{}, fmt.Errorf("%w: publication checksum mismatch", core.ErrRevision)
	}
	return result, nil
}

// WriteMarkdown atomically persists valid UTF-8 Markdown bytes.
func (s *Store) WriteMarkdown(relative string, value []byte, maxBytes int64) (AtomicResult, error) {
	if !utf8.Valid(value) {
		return AtomicResult{}, fmt.Errorf("%w: Markdown is not valid UTF-8", core.ErrPath)
	}
	return s.write(relative, maxBytes, func(writer io.Writer) error {
		_, err := writer.Write(value)
		return err
	})
}

func (s *Store) write(relative string, maxBytes int64, encode func(io.Writer) error) (AtomicResult, error) {
	limit, err := s.limit(maxBytes)
	if err != nil {
		return AtomicResult{}, err
	}
	relative, err = validateRelative(relative)
	if err != nil {
		return AtomicResult{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	root, rootKey, err := s.openRoot(true)
	if err != nil {
		return AtomicResult{}, pathError("open root", s.Root, err)
	}
	defer root.Close()
	if directory := path.Dir(relative); directory != "." {
		if err := root.MkdirAll(directory, 0o700); err != nil {
			return AtomicResult{}, pathError("create parent", relative, err)
		}
	}
	if info, err := root.Lstat(relative); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return AtomicResult{}, fmt.Errorf("%w: destination %s is a symbolic link", core.ErrPath, relative)
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return AtomicResult{}, pathError("lstat", relative, err)
	}

	mode, err := s.replacementProbe(root, path.Dir(relative))
	if err != nil {
		return AtomicResult{}, pathError("probe replacement", relative, err)
	}
	if mode == probeFallback {
		lock := sharedFallbackLock(rootKey)
		lock.Lock()
		defer lock.Unlock()
	}

	temporary, file, err := createOwnedTemp(root, path.Dir(relative), ".agent-team-tmp-")
	if err != nil {
		return AtomicResult{}, pathError("create temporary", relative, err)
	}
	keepTemporary := false
	defer func() {
		if !keepTemporary {
			_ = removeOwned(root, temporary)
		}
	}()
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		return AtomicResult{}, pathError("chmod temporary", relative, err)
	}
	writer := &boundedWriter{writer: file, remaining: limit}
	if err := encode(writer); err != nil {
		_ = file.Close()
		if errors.Is(err, errTooLarge) {
			return AtomicResult{}, limitError(relative, limit)
		}
		return AtomicResult{}, pathError("encode temporary", relative, err)
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return AtomicResult{}, pathError("flush temporary", relative, err)
	}
	if err := file.Close(); err != nil {
		return AtomicResult{}, pathError("close temporary", relative, err)
	}
	result, err := hashRootFile(root, temporary.name, limit)
	if err != nil {
		return AtomicResult{}, pathError("verify temporary", relative, err)
	}

	backup, err := snapshotDestination(root, relative, limit)
	if err != nil {
		return AtomicResult{}, pathError("preserve last good", relative, err)
	}
	retainBackup := false
	if backup != nil {
		defer func() {
			if !retainBackup {
				_ = removeOwned(root, *backup)
			}
		}()
	}
	if err := s.replaceDestination(root, temporary.name, relative); err != nil {
		keepTemporary = true
		return AtomicResult{}, pathError("replace", relative, err)
	}
	keepTemporary = true

	confirmed, err := s.replacementVerify(root, relative, limit)
	if err == nil && confirmed != result {
		err = errors.New("replacement checksum mismatch")
	}
	if err != nil {
		if restoreErr := s.restoreDestination(root, relative, backup, temporary); restoreErr != nil {
			retainBackup = backup != nil
			return AtomicResult{}, pathError("verify replacement", relative, recoveryError{
				path:  recoveryPath(relative),
				cause: errors.Join(err, restoreErr),
			})
		}
		return AtomicResult{}, pathError("verify replacement", relative, err)
	}
	return result, nil
}

func (s *Store) limit(requested int64) (int64, error) {
	if requested <= 0 || requested > maxStorageBytes {
		return 0, fmt.Errorf("%w: storage bound must be between 1 and %d", core.ErrLimit, maxStorageBytes)
	}
	if s.Limits.CanonicalBytes > 0 && s.Limits.CanonicalBytes < requested {
		return s.Limits.CanonicalBytes, nil
	}
	return requested, nil
}

func validateRelative(relative string) (string, error) {
	if filepath.IsAbs(relative) || strings.HasPrefix(relative, "/") || strings.HasPrefix(relative, `\\`) {
		return "", fmt.Errorf("%w: absolute path %q", core.ErrPath, relative)
	}
	parts := strings.FieldsFunc(relative, func(r rune) bool { return r == '/' || r == '\\' })
	if len(parts) == 0 {
		return "", fmt.Errorf("%w: empty path", core.ErrPath)
	}
	for _, part := range parts {
		if part == "." || part == ".." || part == "" {
			return "", fmt.Errorf("%w: unsafe path %q", core.ErrPath, relative)
		}
	}
	return strings.Join(parts, "/"), nil
}

func (s *Store) openRoot(create bool) (*os.Root, string, error) {
	if s.Root == "" {
		return nil, "", errors.New("empty store root")
	}
	if create {
		if err := os.MkdirAll(s.Root, 0o700); err != nil {
			return nil, "", err
		}
	}
	absolute, err := filepath.Abs(s.Root)
	if err != nil {
		return nil, "", err
	}
	root, err := os.OpenRoot(s.Root)
	if err != nil {
		return nil, "", err
	}
	if resolved, err := filepath.EvalSymlinks(absolute); err == nil {
		absolute = resolved
	}
	return root, absolute, nil
}

type probeResult uint8

const (
	probeAtomic probeResult = iota
	probeFallback
)

func (s *Store) replacementProbe(root *os.Root, directory string) (probeResult, error) {
	if s.probe != nil {
		return s.probe(root, directory)
	}
	if err := probeSameDirectoryReplace(root, directory); err != nil {
		return probeAtomic, err
	}
	// A rename probe cannot prove durable atomicity on an arbitrary network
	// mount. Conservatively choose the serialized read-after-write path.
	return probeFallback, nil
}

func (s *Store) replacementVerify(root *os.Root, relative string, limit int64) (AtomicResult, error) {
	if s.verify != nil {
		return s.verify(root, relative, limit)
	}
	return hashRootFile(root, relative, limit)
}

func (s *Store) replaceDestination(root *os.Root, source, destination string) error {
	if s.replace != nil {
		return s.replace(root, source, destination)
	}
	return replaceFile(root, source, destination)
}

var fallbackLocks sync.Map

func sharedFallbackLock(root string) *sync.Mutex {
	lock, _ := fallbackLocks.LoadOrStore(root, &sync.Mutex{})
	return lock.(*sync.Mutex)
}

type ownedTemp struct {
	name string
	info os.FileInfo
}

func createOwnedTemp(root *os.Root, directory, prefix string) (ownedTemp, *os.File, error) {
	if directory == "." {
		directory = ""
	}
	for attempt := 0; attempt < 32; attempt++ {
		var token [16]byte
		if _, err := rand.Read(token[:]); err != nil {
			return ownedTemp{}, nil, err
		}
		name := path.Join(directory, prefix+hex.EncodeToString(token[:]))
		owned, file, err := createOwnedFile(root, name)
		if errors.Is(err, os.ErrExist) {
			continue
		}
		if err != nil {
			return ownedTemp{}, nil, err
		}
		return owned, file, nil
	}
	return ownedTemp{}, nil, errors.New("temporary name collision exhaustion")
}

func createOwnedFile(root *os.Root, name string) (ownedTemp, *os.File, error) {
	file, err := createTemporary(root, name)
	if err != nil {
		return ownedTemp{}, nil, err
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return ownedTemp{}, nil, err
	}
	return ownedTemp{name: name, info: info}, file, nil
}

func removeOwned(root *os.Root, owned ownedTemp) error {
	if owned.name == "" {
		return nil
	}
	info, err := root.Lstat(owned.name)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !os.SameFile(owned.info, info) {
		return errors.New("temporary ownership changed")
	}
	return cleanupTemporary(root, owned.name)
}

func probeSameDirectoryReplace(root *os.Root, directory string) error {
	from, fromFile, err := createOwnedTemp(root, directory, ".agent-team-probe-from-")
	if err != nil {
		return err
	}
	if err := fromFile.Close(); err != nil {
		_ = removeOwned(root, from)
		return err
	}
	defer removeOwned(root, from)
	to, toFile, err := createOwnedTemp(root, directory, ".agent-team-probe-to-")
	if err != nil {
		return err
	}
	if err := toFile.Close(); err != nil {
		_ = removeOwned(root, to)
		return err
	}
	defer removeOwned(root, to)
	return replaceFile(root, from.name, to.name)
}

func snapshotDestination(root *os.Root, relative string, limit int64) (*ownedTemp, error) {
	info, err := root.Lstat(relative)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return nil, errors.New("destination is not a regular file")
	}
	file, err := root.Open(relative)
	if err != nil {
		return nil, err
	}
	recovery := recoveryPath(relative)
	if err := root.MkdirAll(path.Dir(recovery), 0o700); err != nil {
		_ = file.Close()
		return nil, err
	}
	backup, backupFile, err := createOwnedFile(root, recovery)
	if err != nil {
		_ = file.Close()
		return nil, err
	}
	if _, err := copyBounded(backupFile, file, limit); err != nil {
		_ = file.Close()
		_ = backupFile.Close()
		_ = removeOwned(root, backup)
		return nil, err
	}
	if err := file.Close(); err != nil {
		_ = backupFile.Close()
		_ = removeOwned(root, backup)
		return nil, err
	}
	if err := backupFile.Sync(); err != nil {
		_ = backupFile.Close()
		_ = removeOwned(root, backup)
		return nil, err
	}
	if err := backupFile.Close(); err != nil {
		_ = removeOwned(root, backup)
		return nil, err
	}
	return &backup, nil
}

func (s *Store) restoreDestination(root *os.Root, relative string, backup *ownedTemp, replacement ownedTemp) error {
	if backup == nil {
		// A first write has no last-good value. Remove only the replacement we
		// created rather than leaving a failed, unverified canonical file.
		replacement.name = relative
		return removeOwned(root, replacement)
	}
	if s.restore != nil {
		if err := s.restore(root, backup.name, relative); err != nil {
			return err
		}
	} else if err := replaceFile(root, backup.name, relative); err != nil {
		return err
	}
	backup.name = ""
	return nil
}

var errTooLarge = fmt.Errorf("%w: content exceeds bound", core.ErrLimit)

type recoveryError struct {
	path  string
	cause error
}

func (e recoveryError) Error() string {
	return fmt.Sprintf("last-good recovery retained at %s", e.path)
}

func (e recoveryError) Unwrap() error { return e.cause }

func recoveryPath(relative string) string {
	digest := sha256.Sum256([]byte(relative))
	return path.Join(".agent-team-recovery", hex.EncodeToString(digest[:]))
}

type boundedWriter struct {
	writer    io.Writer
	remaining int64
}

func (w *boundedWriter) Write(data []byte) (int, error) {
	if int64(len(data)) > w.remaining {
		allowed := int(w.remaining)
		if allowed > 0 {
			n, err := w.writer.Write(data[:allowed])
			w.remaining -= int64(n)
			if err != nil {
				return n, err
			}
			return n, errTooLarge
		}
		return 0, errTooLarge
	}
	n, err := w.writer.Write(data)
	w.remaining -= int64(n)
	return n, err
}

type boundedReader struct {
	reader    io.Reader
	remaining int64
}

func (r *boundedReader) Read(data []byte) (int, error) {
	if r.remaining == 0 {
		var extra [1]byte
		n, err := r.reader.Read(extra[:])
		if n > 0 {
			return 0, errTooLarge
		}
		return 0, err
	}
	if int64(len(data)) > r.remaining {
		data = data[:int(r.remaining)]
	}
	n, err := r.reader.Read(data)
	r.remaining -= int64(n)
	return n, err
}

func copyBounded(destination io.Writer, source io.Reader, limit int64) (int64, error) {
	if limit <= 0 || limit > maxStorageBytes {
		return 0, errTooLarge
	}
	return io.Copy(destination, &boundedReader{reader: source, remaining: limit})
}

func hashRootFile(root *os.Root, relative string, limit int64) (AtomicResult, error) {
	file, err := root.Open(relative)
	if err != nil {
		return AtomicResult{}, err
	}
	defer file.Close()
	hash := sha256.New()
	bytes, err := copyBounded(hash, file, limit)
	if err != nil {
		return AtomicResult{}, err
	}
	return AtomicResult{Bytes: bytes, SHA256: hex.EncodeToString(hash.Sum(nil))}, nil
}

func limitError(relative string, limit int64) error {
	return fmt.Errorf("%w: %s exceeds %d bytes", core.ErrLimit, relative, limit)
}

func pathError(operation, path string, err error) error {
	return fmt.Errorf("%w: %s %s: %w", core.ErrPath, operation, path, err)
}
