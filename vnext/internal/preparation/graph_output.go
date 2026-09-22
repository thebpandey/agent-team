package preparation

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Hash tracked and untracked nonignored source files, excluding generated
// tool state. File bytes detect further edits even when Git status is unchanged.
func graphSourceDigest(ctx context.Context, root, git string, run runner) (string, error) {
	callCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	names, err := run.execute(callCtx, invocation{Path: git, Dir: root, Args: []string{"ls-files", "--cached", "--others", "--exclude-standard", "-z", "--", ".", ":(exclude).agent-team", ":(exclude)graphify-out", ":(exclude).serena", ":(exclude).beads"}, OutputLimit: 4 << 20})
	if err != nil {
		return "", fmt.Errorf("list graph source files: %w", err)
	}
	if names != "" && !strings.HasSuffix(names, "\x00") {
		return "", errors.New("Git returned an incomplete source file list")
	}
	paths := strings.Split(strings.TrimSuffix(names, "\x00"), "\x00")
	sort.Strings(paths)
	if len(paths) > 50000 {
		return "", errors.New("Graphify source fingerprint exceeds file count limit")
	}
	hash := sha256.New()
	var total int64
	previous := ""
	for _, rel := range paths {
		if rel == "" || rel == previous {
			continue
		}
		previous = rel
		if err := ctx.Err(); err != nil {
			return "", err
		}
		clean := filepath.Clean(filepath.FromSlash(rel))
		if filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(os.PathSeparator)) {
			return "", errors.New("Git source path escapes project")
		}
		path := filepath.Join(root, clean)
		info, err := os.Lstat(path)
		if os.IsNotExist(err) {
			_, _ = fmt.Fprintf(hash, "missing %q\n", rel)
			continue
		}
		if err != nil {
			return "", err
		}
		if !info.Mode().IsRegular() {
			return "", fmt.Errorf("Graphify source fingerprint requires regular files: %s", rel)
		}
		if info.Size() > 256<<20-total {
			return "", errors.New("Graphify source fingerprint exceeds byte limit")
		}
		total += info.Size()
		file, err := os.Open(path)
		if err != nil {
			return "", err
		}
		fileHash := sha256.New()
		size, readErr := io.Copy(fileHash, io.LimitReader(file, info.Size()+1))
		closeErr := file.Close()
		if readErr != nil || closeErr != nil {
			return "", errors.Join(readErr, closeErr)
		}
		if size != info.Size() {
			return "", errors.New("source changed during graph fingerprint")
		}
		_, _ = fmt.Fprintf(hash, "file %q %x\n", rel, fileHash.Sum(nil))
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

// Bind all output bytes and paths, including manifests, build configuration and
// caches. A graph.json-only receipt cannot authorize overwriting its sidecars.
func graphTreeDigest(root string) (string, error) {
	info, err := os.Lstat(root)
	if err != nil {
		return "", err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return "", errors.New("Graphify output must be an ordinary directory")
	}
	hash := sha256.New()
	count := 0
	var total int64
	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		count++
		if count > 10000 {
			return errors.New("Graphify output exceeds file count limit")
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if info.IsDir() {
			_, _ = fmt.Fprintf(hash, "dir %q\n", filepath.ToSlash(rel))
			return nil
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("Graphify output contains a symlink or special file: %s", path)
		}
		if info.Size() > 256<<20-total {
			return errors.New("Graphify output exceeds total byte limit")
		}
		total += info.Size()
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		fileHash := sha256.New()
		size, readErr := io.Copy(fileHash, io.LimitReader(file, info.Size()+1))
		closeErr := file.Close()
		if readErr != nil || closeErr != nil {
			return errors.Join(readErr, closeErr)
		}
		if size != info.Size() {
			return errors.New("Graphify output changed during validation")
		}
		_, _ = fmt.Fprintf(hash, "file %q %x\n", filepath.ToSlash(rel), fileHash.Sum(nil))
		return nil
	})
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

// Publish by renaming validated trees on the same filesystem. Keep the old tree
// until the receipt commits, restoring it if publication or receipt writing fails.
func publishGraph(stage, dest, expectedTree, receiptPath string, oldReceipt, newReceipt []byte) error {
	currentReceipt, err := os.ReadFile(receiptPath)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if !bytes.Equal(currentReceipt, oldReceipt) {
		return errors.New("Graphify receipt changed during extraction")
	}
	currentTree, err := graphTreeDigest(dest)
	if expectedTree == "" {
		if !os.IsNotExist(err) {
			return errors.New("Graphify output appeared during extraction; preserving it")
		}
	} else if err != nil || currentTree != expectedTree {
		return errors.New("Graphify output changed during extraction; preserving it")
	}
	backup := ""
	if expectedTree != "" {
		backup, err = os.MkdirTemp(filepath.Dir(stage), ".graphify-backup-")
		if err != nil {
			return err
		}
		if err = os.Remove(backup); err != nil {
			return err
		}
		if err = os.Rename(dest, backup); err != nil {
			return err
		}
	}
	if err = os.Rename(stage, dest); err != nil {
		if backup != "" {
			return errors.Join(err, os.Rename(backup, dest))
		}
		return err
	}
	if err = writeReceipt(receiptPath, newReceipt); err != nil {
		moveErr := os.Rename(dest, stage)
		if backup != "" && moveErr == nil {
			return errors.Join(err, os.Rename(backup, dest))
		}
		return errors.Join(err, moveErr)
	}
	if backup != "" {
		_ = os.RemoveAll(backup)
	}
	return nil
}
