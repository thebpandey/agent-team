// Package store persists small canonical records without exposing partially
// written files to readers.
package store

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/thebpandey/agent-team/vnext/internal/core"
)

// Store is a root-relative, bounded persistence store.
type Store struct {
	Root   string
	Limits core.StorageLimits

	mu sync.Mutex
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
	path, err := s.readPath(relative)
	if err != nil {
		return err
	}
	file, err := os.Open(path)
	if err != nil {
		return pathError("open", relative, err)
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return pathError("stat", relative, err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%w: %s is not a regular file", core.ErrPath, relative)
	}
	if info.Size() > limit {
		return limitError(relative, limit)
	}

	data, err := readBounded(file, limit)
	if err != nil {
		if errors.Is(err, errTooLarge) {
			return limitError(relative, limit)
		}
		return pathError("read", relative, err)
	}
	if err := json.Unmarshal(data, destination); err != nil {
		return fmt.Errorf("%w: decode %s: %v", core.ErrPath, relative, err)
	}
	return nil
}

// WriteJSON serializes value and atomically replaces the root-relative file.
func (s *Store) WriteJSON(relative string, value any, maxBytes int64) (AtomicResult, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return AtomicResult{}, fmt.Errorf("%w: encode %s: %v", core.ErrPath, relative, err)
	}
	return s.write(relative, encoded, maxBytes)
}

// WriteMarkdown atomically persists the supplied UTF-8 Markdown bytes.
func (s *Store) WriteMarkdown(relative string, value []byte, maxBytes int64) (AtomicResult, error) {
	return s.write(relative, value, maxBytes)
}

func (s *Store) write(relative string, value []byte, maxBytes int64) (AtomicResult, error) {
	limit, err := s.limit(maxBytes)
	if err != nil {
		return AtomicResult{}, err
	}
	if int64(len(value)) > limit {
		return AtomicResult{}, limitError(relative, limit)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	destination, err := s.writePath(relative)
	if err != nil {
		return AtomicResult{}, err
	}
	directory := filepath.Dir(destination)
	if err := s.probeReplace(directory); err != nil {
		return AtomicResult{}, err
	}

	temporary, err := createTemporary(directory, ".agent-team-tmp-*")
	if err != nil {
		return AtomicResult{}, pathError("create temporary", relative, err)
	}
	temporaryPath := temporary.Name()
	keepTemporary := false
	defer func() {
		if !keepTemporary {
			_ = cleanupTemporary(temporaryPath) // This file was created by this call.
		}
	}()
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return AtomicResult{}, pathError("chmod temporary", relative, err)
	}
	if _, err := temporary.Write(value); err != nil {
		_ = temporary.Close()
		return AtomicResult{}, pathError("write temporary", relative, err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return AtomicResult{}, pathError("flush temporary", relative, err)
	}
	if err := temporary.Close(); err != nil {
		return AtomicResult{}, pathError("close temporary", relative, err)
	}

	result, err := hashFile(temporaryPath, limit)
	if err != nil {
		if errors.Is(err, errTooLarge) {
			return AtomicResult{}, limitError(relative, limit)
		}
		return AtomicResult{}, pathError("verify temporary", relative, err)
	}
	if result.Bytes != int64(len(value)) {
		return AtomicResult{}, fmt.Errorf("%w: temporary size changed for %s", core.ErrPath, relative)
	}

	if err := replaceFile(temporaryPath, destination); err != nil {
		// A failed replacement can be caused by a Windows sharing lock. Retain the
		// known transient for recovery and leave the last-good destination alone.
		keepTemporary = true
		return AtomicResult{}, pathError("replace", relative, err)
	}
	keepTemporary = true // rename consumed the temporary path; never remove destination.

	// Serializing writes and checking the bytes visible at the final path makes
	// network/non-atomic filesystems fail closed instead of reporting an
	// unverified successful replacement.
	confirmed, err := hashFile(destination, limit)
	if err != nil {
		if errors.Is(err, errTooLarge) {
			return AtomicResult{}, limitError(relative, limit)
		}
		return AtomicResult{}, pathError("verify replacement", relative, err)
	}
	if confirmed != result {
		return AtomicResult{}, fmt.Errorf("%w: replacement checksum mismatch for %s", core.ErrPath, relative)
	}
	return result, nil
}

func (s *Store) limit(requested int64) (int64, error) {
	if requested <= 0 {
		return 0, fmt.Errorf("%w: non-positive storage bound", core.ErrLimit)
	}
	if s.Limits.CanonicalBytes > 0 && requested > s.Limits.CanonicalBytes {
		return s.Limits.CanonicalBytes, nil
	}
	return requested, nil
}

func (s *Store) readPath(relative string) (string, error) {
	path, parts, err := s.relativePath(relative)
	if err != nil {
		return "", err
	}
	if err := ensureNoLinks(s.Root, parts, false); err != nil {
		return "", pathError("resolve", relative, err)
	}
	return path, nil
}

func (s *Store) writePath(relative string) (string, error) {
	path, parts, err := s.relativePath(relative)
	if err != nil {
		return "", err
	}
	if err := ensureNoLinks(s.Root, parts, true); err != nil {
		return "", pathError("prepare", relative, err)
	}
	return path, nil
}

func (s *Store) relativePath(relative string) (string, []string, error) {
	if s.Root == "" {
		return "", nil, fmt.Errorf("%w: empty store root", core.ErrPath)
	}
	if filepath.IsAbs(relative) || strings.HasPrefix(relative, "/") || strings.HasPrefix(relative, `\\`) {
		return "", nil, fmt.Errorf("%w: absolute path %q", core.ErrPath, relative)
	}
	parts := strings.FieldsFunc(relative, func(r rune) bool { return r == '/' || r == '\\' })
	if len(parts) == 0 {
		return "", nil, fmt.Errorf("%w: empty path", core.ErrPath)
	}
	for _, part := range parts {
		if part == "." || part == ".." || part == "" {
			return "", nil, fmt.Errorf("%w: unsafe path %q", core.ErrPath, relative)
		}
	}
	root, err := filepath.Abs(s.Root)
	if err != nil {
		return "", nil, pathError("resolve root", s.Root, err)
	}
	return filepath.Join(append([]string{root}, parts...)...), parts, nil
}

func ensureNoLinks(root string, parts []string, createDirectories bool) error {
	info, err := os.Lstat(root)
	if err != nil {
		if !createDirectories || !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if err := os.MkdirAll(root, 0o700); err != nil {
			return err
		}
		info, err = os.Lstat(root)
		if err != nil {
			return err
		}
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("root is not a directory")
	}

	current := root
	last := len(parts) - 1
	for index, part := range parts {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if index == last {
			if err == nil && info.Mode()&os.ModeSymlink != 0 {
				return fmt.Errorf("destination is a symbolic link")
			}
			if err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
			return nil
		}
		if err != nil {
			if !createDirectories || !errors.Is(err, os.ErrNotExist) {
				return err
			}
			if err := os.Mkdir(current, 0o700); err != nil && !errors.Is(err, os.ErrExist) {
				return err
			}
			info, err = os.Lstat(current)
			if err != nil {
				return err
			}
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("path component is not a directory")
		}
	}
	return nil
}

func (s *Store) probeReplace(directory string) error {
	from, err := createTemporary(directory, ".agent-team-probe-from-*")
	if err != nil {
		return pathError("create replacement probe", directory, err)
	}
	fromPath := from.Name()
	defer cleanupTemporary(fromPath)
	if err := from.Close(); err != nil {
		return pathError("close replacement probe", directory, err)
	}
	to, err := createTemporary(directory, ".agent-team-probe-to-*")
	if err != nil {
		return pathError("create replacement probe", directory, err)
	}
	toPath := to.Name()
	if err := to.Close(); err != nil {
		_ = cleanupTemporary(toPath)
		return pathError("close replacement probe", directory, err)
	}
	defer cleanupTemporary(toPath)
	if err := replaceFile(fromPath, toPath); err != nil {
		return pathError("probe replacement", directory, err)
	}
	return nil
}

var errTooLarge = errors.New("storage content exceeds bound")

func readBounded(reader io.Reader, limit int64) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, errTooLarge
	}
	return data, nil
}

func hashFile(path string, limit int64) (AtomicResult, error) {
	file, err := os.Open(path)
	if err != nil {
		return AtomicResult{}, err
	}
	defer file.Close()
	data, err := readBounded(file, limit)
	if err != nil {
		return AtomicResult{}, err
	}
	digest := sha256.Sum256(data)
	return AtomicResult{Bytes: int64(len(data)), SHA256: hex.EncodeToString(digest[:])}, nil
}

func limitError(relative string, limit int64) error {
	return fmt.Errorf("%w: %s exceeds %d bytes", core.ErrLimit, relative, limit)
}

func pathError(operation, path string, err error) error {
	return fmt.Errorf("%w: %s %s: %v", core.ErrPath, operation, path, err)
}
