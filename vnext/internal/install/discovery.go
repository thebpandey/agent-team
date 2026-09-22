package install

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/thebpandey/agent-team/vnext/internal/core"
)

func nestedEntrypoint(layout Layout, host Host) string {
	return filepath.Join(layout.SkillRoots[host], "agent-team-vnext", "SKILL.md")
}

// A manual move preserves ownership only when the old path is absent and the
// exact manifest bytes exist at the canonical path. Never adopt edited files.
func reconcileMovedEntrypoints(layout Layout, current InstallManifest) InstallManifest {
	current = cloneManifest(current)
	for index, file := range current.Files {
		if file.Role != EntrypointRole || file.Path != nestedEntrypoint(layout, file.Host) {
			continue
		}
		top := filepath.Join(layout.SkillRoots[file.Host], "SKILL.md")
		if _, err := os.Lstat(file.Path); errors.Is(err, fs.ErrNotExist) && diskMatches(top, file.SHA256, file.Bytes) {
			current.Files[index].Path = top
		}
	}
	return current
}

// Keep migration in the existing lifecycle transaction, including rollback and
// recovery. A root destination must be absent or contain exact previously owned
// bytes; a matching name alone never authorizes replacement.
func prepareDiscoveryJournal(layout Layout, journal *lifecycleJournal) error {
	if journal.Previous == nil {
		return nil
	}
	for _, old := range journal.Previous.Files {
		if old.Role != EntrypointRole || old.Path != nestedEntrypoint(layout, old.Host) {
			continue
		}
		top := filepath.Join(layout.SkillRoots[old.Host], "SKILL.md")
		_, oldErr := os.Lstat(old.Path)
		moved := errors.Is(oldErr, fs.ErrNotExist) && diskMatches(top, old.SHA256, old.Bytes)
		if moved {
			// Recovery must accept this pre-existing manual move too.
			journal.Retained = append(journal.Retained, old.Path)
		}
		index := ownedIndex(journal.Intended.Files, EntrypointRole, old.Host)
		if index >= 0 && journal.Operation != "uninstall" {
			if _, err := os.Lstat(top); err == nil {
				if !diskMatches(top, old.SHA256, old.Bytes) {
					return core.ErrRevision
				}
			} else if !errors.Is(err, fs.ErrNotExist) {
				return err
			}
		}
		if index < 0 || journal.Intended.Files[index].Path == top {
			// Update already targets the root; validate its destination before
			// any transaction mutation (including binary replacement) begins.
			for _, mutation := range journal.Mutations {
				if mutation.Path == top && mutation.Existed && mutation.PreSHA256 != old.SHA256 {
					return core.ErrRevision
				}
			}
			continue
		}
		if !diskMatches(old.Path, old.SHA256, old.Bytes) {
			continue // Preserve modified or missing owned files.
		}
		data, err := readStableRegular(ownedRoot(layout, old.Path), old.Path, old.Bytes, nil, "")
		if err != nil {
			return err
		}
		mode := fs.FileMode(0o600)
		mutationIndex := -1
		for i, mutation := range journal.Mutations {
			if mutation.Path == old.Path && !mutation.PostAbsent {
				data, mode, mutationIndex = mutation.Replacement, fs.FileMode(mutation.PostMode), i
				break
			}
		}
		mutation, err := prepareMutation(layout, top, data, mode, false, false, nil)
		if err != nil {
			return err
		}
		mutation.Exclusive = !mutation.Existed
		if mutationIndex >= 0 {
			journal.Mutations[mutationIndex] = mutation
		} else {
			journal.Mutations = append(journal.Mutations, mutation)
		}
		remove, err := prepareMutation(layout, old.Path, nil, 0, true, false, nil)
		if err != nil {
			return err
		}
		journal.Mutations = append(journal.Mutations, remove)
		journal.Intended.Files[index].Path = top
	}
	return nil
}
