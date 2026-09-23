package install

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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
	installAttemptPath = ".agent-team/install-attempt.json"
	// Keep the existing individual-file read bound independent of the complete
	// transaction. Updates journal the old binary twice (preimage and rollback
	// backup), plus its replacement. Three 14 MiB payloads need 56 MiB after
	// base64 encoding, before manifest and skill metadata. The aggregate bound
	// remains enforced before mutation; final-artifact canaries check growth.
	installFileLimit    = 32 << 20
	installJournalLimit = 64 << 20
)

var installMutationHook func()
var installCreateHook func(int) error
var lifecycleMutationHook func(string, int) error
var lifecycleInterruptHook func(string, int) bool
var errJournalBudget = fmt.Errorf("%w: lifecycle journal budget exceeded", core.ErrRevision)
var stableReadHook func(string)
var updateBackupSnapshotHook func()

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

type journalBudget struct {
	remaining int64
	metadata  bool
}

func newJournalBudget() *journalBudget {
	return &journalBudget{remaining: installJournalLimit}
}

func (b *journalBudget) accountMetadata(journal lifecycleJournal) error {
	if b == nil || b.metadata {
		return errJournalBudget
	}
	raw, err := json.Marshal(journal)
	if err != nil || int64(len(raw)) > b.remaining {
		return errJournalBudget
	}
	b.remaining -= int64(len(raw))
	b.metadata = true
	return nil
}

func (b *journalBudget) reserve(size int64, field string) error {
	if b == nil || !b.metadata || size < 0 || size > installFileLimit {
		return errJournalBudget
	}
	if size == 0 {
		return nil
	}
	encoded := ((size + 2) / 3) * 4
	wrapper := int64(len(`,"` + field + `":""`))
	if (field != "preimage" && field != "replacement") || wrapper > b.remaining || encoded > b.remaining-wrapper {
		return errJournalBudget
	}
	b.remaining -= encoded + wrapper
	return nil
}

func Install(ctx context.Context, layout Layout, release Release, hosts []Host, expected uint64) (CASOutcome, error) {
	if containsHost(hosts, Codex) {
		if outcome, err := rejectUnownedCodexSkill(layout, nil); err != nil {
			return outcome, err
		}
	}
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
	if ctx == nil || ctx.Err() != nil || ValidateLayout(layout) != nil {
		return CASOutcome{}, core.ErrSettings
	}
	hosts, err := normalizeHosts(hosts)
	if err != nil || len(hosts) == 0 {
		return CASOutcome{}, core.ErrSettings
	}
	if containsHost(hosts, Codex) {
		if outcome, err := rejectUnownedCodexSkill(layout, nil); err != nil {
			return outcome, err
		}
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
	hostHomes, _ := layoutHostHomes(layout)
	manifest := InstallManifest{Schema: 1, Revision: 1, Version: release.Version, ReleaseRevision: release.Revision, Hosts: hosts, HostHomes: hostHomes}
	journal := lifecycleJournal{Schema: 1, Operation: "install", ExpectedRevision: expected, Owner: owner, Intended: manifest}
	for _, file := range desired {
		mode := uint32(0o600)
		if file.owned.Role == BinaryRole {
			mode = 0o700
		}
		journal.Mutations = append(journal.Mutations, lifecycleMutation{Path: file.owned.Path, PostMode: mode, PostSHA256: file.source.SHA256, PostBytes: file.source.Bytes, Exclusive: true})
		journal.Intended.Files = append(journal.Intended.Files, file.owned)
	}
	budget := newJournalBudget()
	if err := budget.accountMetadata(journal); err != nil {
		return CASOutcome{}, err
	}
	sources, err := validateLifecycle(ctx, layout, release, selectedReleasePaths(desired), budget)
	if err != nil {
		return CASOutcome{}, err
	}
	journal.Mutations = journal.Mutations[:0]
	journal.Intended.Files = journal.Intended.Files[:0]
	for index, file := range desired {
		data, err := verifiedReleaseBytes(file.source, sources)
		if err != nil {
			return CASOutcome{}, err
		}
		mode := fs.FileMode(0o600)
		if file.owned.Role == BinaryRole {
			mode = 0o700
		}
		mutation, err := prepareMutation(layout, file.owned.Path, data, mode, false, true, nil)
		if err != nil {
			return CASOutcome{}, err
		}
		journal.Mutations = append(journal.Mutations, mutation)
		journal.Intended.Files = append(journal.Intended.Files, file.owned)
		_ = index
	}
	return executeLifecycleJournal(ctx, layout, manifestStore, &journal)
}

func prepareMutation(layout Layout, path string, replacement []byte, mode fs.FileMode, postAbsent, exclusive bool, budget *journalBudget) (lifecycleMutation, error) {
	if ownedRoot(layout, path) == "" {
		return lifecycleMutation{}, core.ErrPath
	}
	if len(replacement) > installFileLimit || !postAbsent && mode.Perm() == 0 {
		return lifecycleMutation{}, core.ErrRevision
	}
	mutation := lifecycleMutation{Path: path, Replacement: replacement, PostMode: lifecycleMode(mode), PostAbsent: postAbsent, Exclusive: exclusive}
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
	if exclusive {
		return lifecycleMutation{}, core.ErrRevision
	}
	mutation.Existed = true
	var preMode fs.FileMode
	mutation.Preimage, preMode, err = readStableRegularMode(ownedRoot(layout, path), path, info.Size(), budget, "preimage")
	if err != nil {
		return lifecycleMutation{}, err
	}
	mutation.PreMode = uint32(preMode.Perm())
	if int64(len(mutation.Preimage)) != info.Size() {
		return lifecycleMutation{}, core.ErrRevision
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
	if err := verifyLifecyclePostimages(layout, *journal); err != nil {
		return outcome, err
	}
	if err := removeLifecycleJournal(layout); err != nil {
		return outcome, err
	}
	return outcome, nil
}

func applyLifecycleMutation(layout Layout, mutation lifecycleMutation) error {
	if mutation.Existed && !diskMatchesMode(layout, mutation.Path, mutation.PreSHA256, int64(len(mutation.Preimage)), mutation.PreMode) {
		return core.ErrRevision
	}
	if mutation.PostAbsent {
		return removeOwnedPath(layout, mutation.Path, mutation.PreSHA256)
	}
	mode := fs.FileMode(mutation.PostMode)
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
		if err := verifyLifecyclePostimages(layout, journal); err != nil {
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

func verifyLifecyclePostimages(layout Layout, journal lifecycleJournal) error {
	for _, mutation := range journal.Mutations {
		if mutation.PostAbsent {
			if _, err := os.Lstat(mutation.Path); !errors.Is(err, fs.ErrNotExist) {
				return core.ErrRevision
			}
			continue
		}
		if !diskMatchesMode(layout, mutation.Path, mutation.PostSHA256, mutation.PostBytes, mutation.PostMode) {
			return core.ErrRevision
		}
	}
	return nil
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
		if len(mutation.Preimage) > installFileLimit || len(mutation.Replacement) > installFileLimit {
			return core.ErrRevision
		}
		if mutation.Existed {
			if !validSHA256(mutation.PreSHA256) || digestContent(mutation.Preimage) != mutation.PreSHA256 {
				return core.ErrRevision
			}
		} else if len(mutation.Preimage) != 0 || mutation.PreSHA256 != "" || mutation.PreMode != 0 {
			return core.ErrRevision
		}
		if mutation.PostAbsent {
			if mutation.PostMode != 0 || mutation.PostSHA256 != "" || mutation.PostBytes != 0 || len(mutation.Replacement) != 0 {
				return core.ErrRevision
			}
		} else if mutation.PostMode == 0 || !validSHA256(mutation.PostSHA256) || mutation.PostBytes != int64(len(mutation.Replacement)) || digestContent(mutation.Replacement) != mutation.PostSHA256 {
			return core.ErrRevision
		}
		if mutation.Exclusive && (mutation.Existed || (journal.Operation != "install" && journal.Operation != "update" && journal.Operation != "rollback")) {
			return core.ErrRevision
		}
	}
	for _, path := range journal.Retained {
		if ownedRoot(layout, path) == "" {
			return core.ErrPath
		}
	}
	raw, err := json.Marshal(journal)
	if err != nil || len(raw) > installJournalLimit {
		return core.ErrRevision
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
			if diskMatchesMode(layout, mutation.Path, mutation.PreSHA256, int64(len(mutation.Preimage)), mutation.PreMode) {
				continue
			}
			if mutation.PostAbsent {
				if _, err := os.Lstat(mutation.Path); !errors.Is(err, fs.ErrNotExist) {
					return core.ErrRevision
				}
			} else if !diskMatchesMode(layout, mutation.Path, mutation.PostSHA256, mutation.PostBytes, mutation.PostMode) {
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
		if mutation.PostAbsent || !diskMatchesMode(layout, mutation.Path, mutation.PostSHA256, mutation.PostBytes, mutation.PostMode) {
			return core.ErrRevision
		}
		if err := removeOwnedPath(layout, mutation.Path, mutation.PostSHA256); err != nil {
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
	_, err := store.NewInstallJournal(layout.DataRoot).WriteJSON(installAttemptPath, journal, installJournalLimit)
	return err
}

func readLifecycleJournal(layout Layout) (lifecycleJournal, error) {
	var journal lifecycleJournal
	err := store.NewInstallJournal(layout.DataRoot).ReadJSON(installAttemptPath, installJournalLimit, &journal)
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

func updateJournalPlan(layout Layout, release Release, current InstallManifest, desired []desiredFile, backups map[string]bool, owner store.MutationOwner, expected uint64, reconcilePaths bool) lifecycleJournal {
	previous, intended := cloneManifest(current), cloneManifest(current)
	intended.Revision, intended.Version, intended.ReleaseRevision = expected+1, release.Version, release.Revision
	intended.HostHomes, _ = layoutHostHomes(layout)
	plan := lifecycleJournal{Schema: 1, Operation: "update", ExpectedRevision: expected, Owner: owner, Previous: &previous, Intended: intended}
	for _, target := range desired {
		index := ownedIndex(plan.Intended.Files, target.owned.Role, target.owned.Host)
		if index < 0 {
			continue
		}
		old := plan.Intended.Files[index]
		mode := uint32(0o600)
		if target.owned.Role == BinaryRole {
			mode = 0o700
		}
		if !backups[backupPath(layout, old)] {
			plan.Mutations = append(plan.Mutations, lifecycleMutation{Path: backupPath(layout, old), PostMode: 0o600, PostSHA256: old.SHA256, PostBytes: old.Bytes, Exclusive: true})
		}
		mutationPath := old.Path
		if reconcilePaths {
			mutationPath = target.owned.Path
		}
		plan.Mutations = append(plan.Mutations, lifecycleMutation{Path: mutationPath, Existed: old.Path == mutationPath, PreSHA256: strings.Repeat("0", 64), PreMode: plannedMode(mutationPath), PostMode: mode, PostSHA256: target.source.SHA256, PostBytes: target.source.Bytes})
		if reconcilePaths && old.Path != target.owned.Path {
			plan.Mutations = append(plan.Mutations, lifecycleMutation{Path: old.Path, Existed: true, PreSHA256: strings.Repeat("0", 64), PreMode: plannedMode(old.Path), PostAbsent: true})
		}
		plan.Retained = append(plan.Retained, old.Path, target.owned.Path, backupPath(layout, old))
		plan.Intended.Files[index] = target.owned
	}
	return plan
}

func rollbackJournalPlan(layout Layout, current InstallManifest, version, revision string, backups []Backup, owner store.MutationOwner, expected uint64) lifecycleJournal {
	previous, intended := cloneManifest(current), cloneManifest(current)
	intended.Revision, intended.Version, intended.ReleaseRevision = expected+1, version, revision
	intended.HostHomes, _ = layoutHostHomes(layout)
	plan := lifecycleJournal{Schema: 1, Operation: "rollback", ExpectedRevision: expected, Owner: owner, Previous: &previous, Intended: intended}
	for _, backup := range backups {
		index := ownedIndex(plan.Intended.Files, backup.Role, backup.Host)
		if index < 0 {
			continue
		}
		if plan.Intended.Files[index].Version == version && plan.Intended.Files[index].Revision == revision {
			continue
		}
		mode := uint32(0o600)
		if backup.Role == BinaryRole {
			mode = 0o700
		}
		file := plan.Intended.Files[index]
		plan.Mutations = append(plan.Mutations, lifecycleMutation{Path: file.Path, Existed: true, PreSHA256: strings.Repeat("0", 64), PreMode: plannedMode(file.Path), PostMode: mode, PostSHA256: backup.SHA256, PostBytes: backup.Bytes})
		plan.Retained = append(plan.Retained, file.Path)
		plan.Intended.Files[index].SHA256, plan.Intended.Files[index].Bytes, plan.Intended.Files[index].Version, plan.Intended.Files[index].Revision = backup.SHA256, backup.Bytes, backup.Version, backup.Revision
	}
	return plan
}

func selectRollbackBackups(current InstallManifest, version, revision string) ([]Backup, string, error) {
	revisions := map[string]bool{}
	for _, backup := range current.Backups {
		if backup.Version == version {
			revisions[backup.Revision] = true
		}
	}
	available := make([]string, 0, len(revisions))
	for candidate := range revisions {
		available = append(available, candidate)
	}
	sort.Strings(available)
	if revision == "" {
		if len(available) == 0 {
			return nil, "", fmt.Errorf("%w: rollback %s unavailable", core.ErrRevision, version)
		}
		if len(available) > 1 {
			return nil, "", fmt.Errorf("%w: rollback %s is ambiguous; use --revision with one of %v", core.ErrRevision, version, available)
		}
		revision = available[0]
	} else if !revisions[revision] {
		return nil, "", fmt.Errorf("%w: rollback %s@%s unavailable; revisions %v", core.ErrRevision, version, revision, available)
	}
	selected := []Backup{}
	seen := map[string]bool{}
	for _, backup := range current.Backups {
		if backup.Version != version || backup.Revision != revision {
			continue
		}
		key := string(backup.Role) + "\x00" + string(backup.Host)
		if seen[key] {
			return nil, "", fmt.Errorf("%w: duplicate rollback member for %s@%s", core.ErrRevision, version, revision)
		}
		seen[key] = true
		selected = append(selected, backup)
	}
	for _, file := range current.Files {
		if file.Version == version && file.Revision == revision {
			continue
		}
		key := string(file.Role) + "\x00" + string(file.Host)
		if !seen[key] {
			return nil, "", fmt.Errorf("%w: incomplete rollback %s@%s", core.ErrRevision, version, revision)
		}
	}
	return selected, revision, nil
}

func uninstallJournalPlan(current InstallManifest, owner store.MutationOwner, expected uint64) lifecycleJournal {
	previous, intended := cloneManifest(current), cloneManifest(current)
	intended.Revision = expected + 1
	plan := lifecycleJournal{Schema: 1, Operation: "uninstall", ExpectedRevision: expected, Owner: owner, Previous: &previous, Intended: intended}
	for _, file := range current.Files {
		plan.Mutations = append(plan.Mutations, lifecycleMutation{Path: file.Path, Existed: true, PreSHA256: strings.Repeat("0", 64), PreMode: plannedMode(file.Path), PostAbsent: true})
		plan.Retained = append(plan.Retained, file.Path)
	}
	for _, backup := range current.Backups {
		plan.Mutations = append(plan.Mutations, lifecycleMutation{Path: backup.Path, Existed: true, PreSHA256: strings.Repeat("0", 64), PreMode: plannedMode(backup.Path), PostAbsent: true})
		plan.Retained = append(plan.Retained, backup.Path)
	}
	return plan
}

func plannedMode(path string) uint32 {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() {
		return 0o777
	}
	return uint32(info.Mode().Perm())
}

func snapshotUpdateBackups(layout Layout, current InstallManifest, desired []desiredFile) (map[string]bool, error) {
	backups := make(map[string]bool, len(desired))
	for _, target := range desired {
		index := ownedIndex(current.Files, target.owned.Role, target.owned.Host)
		if index < 0 {
			continue
		}
		path := backupPath(layout, current.Files[index])
		_, err := os.Lstat(path)
		if err == nil {
			backups[path] = true
		} else if !errors.Is(err, fs.ErrNotExist) {
			return nil, err
		}
	}
	return backups, nil
}

func Update(ctx context.Context, layout Layout, release Release, expected uint64) (CASOutcome, error) {
	// Recover an interrupted migration before discovery checks: its new root
	// entrypoint may already contain the next release while the manifest still
	// owns the old nested path. updateLocked checks discovery under this guard.
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
	manifestStore := NewManifestStore(layout)
	current, stale, err := currentManifest(ctx, manifestStore, expected)
	if err != nil {
		return stale, err
	}
	if err := validateInstalledLayout(layout, current); err != nil {
		return CASOutcome{}, err
	}
	original := cloneManifest(current)
	current = reconcileMovedEntrypoints(layout, current)
	if containsHost(current.Hosts, Codex) {
		if outcome, err := rejectUnownedCodexSkill(layout, &current); err != nil {
			return outcome, err
		}
	}
	desired := releaseFiles(layout, release, current.Hosts)
	hostReceipt, err := lifecycleHostCutoverReceipt(layout, current)
	if err != nil {
		return CASOutcome{}, err
	}
	if hostReceipt != nil {
		for index := range desired {
			if desired[index].owned.Role == EntrypointRole {
				desired[index].owned.Path = filepath.Join(layout.SkillRoots[desired[index].owned.Host], "SKILL.md")
			}
		}
	}
	if reflect.DeepEqual(original, current) && installedReleaseMatches(current, desired) {
		if err := VerifyRelease(release); err != nil {
			return CASOutcome{}, core.ErrRevision
		}
		return CASOutcome{Kind: CASDuplicate, Manifest: current, ExpectedRevision: expected, ObservedRevision: current.Revision, Idempotent: true, Retained: ownedPaths(current)}, nil
	}
	desired = pendingReleaseFiles(current, desired)
	backups, err := snapshotUpdateBackups(layout, current, desired)
	if err != nil {
		return CASOutcome{}, err
	}
	budget := newJournalBudget()
	plan := updateJournalPlan(layout, release, current, desired, backups, owner, expected, true)
	var receiptRaw []byte
	if hostReceipt != nil {
		receiptRaw, err = updatedHostCutoverReceipt(*hostReceipt, release.Version, release.Revision, desired)
		if err != nil {
			return CASOutcome{}, err
		}
		receiptPath := filepath.Join(layout.DataRoot, filepath.FromSlash(hostCutoverReceiptRel))
		plan.Mutations = append(plan.Mutations, lifecycleMutation{Path: receiptPath, Existed: true, PreSHA256: hostReceipt.ReceiptDigest, PreMode: plannedMode(receiptPath), PostMode: 0o600, PostSHA256: digestContent(receiptRaw), PostBytes: int64(len(receiptRaw))})
	}
	if err := budget.accountMetadata(plan); err != nil {
		return CASOutcome{}, err
	}
	for _, target := range desired {
		index := ownedIndex(current.Files, target.owned.Role, target.owned.Host)
		if index >= 0 {
			if err := budget.reserve(current.Files[index].Bytes, "preimage"); err != nil {
				return CASOutcome{}, err
			}
			if current.Files[index].Path != target.owned.Path {
				info, statErr := os.Lstat(target.owned.Path)
				if statErr != nil && !errors.Is(statErr, fs.ErrNotExist) || statErr == nil && !info.Mode().IsRegular() {
					return CASOutcome{}, core.ErrRevision
				}
				if statErr == nil {
					if err := budget.reserve(info.Size(), "preimage"); err != nil {
						return CASOutcome{}, err
					}
				}
			}
			if !backups[backupPath(layout, current.Files[index])] {
				if err := budget.reserve(current.Files[index].Bytes, "replacement"); err != nil {
					return CASOutcome{}, err
				}
			}
		}
	}
	if hostReceipt != nil {
		receiptPath := filepath.Join(layout.DataRoot, filepath.FromSlash(hostCutoverReceiptRel))
		info, statErr := os.Lstat(receiptPath)
		if statErr != nil || !info.Mode().IsRegular() {
			return CASOutcome{}, core.ErrRevision
		}
		if err := budget.reserve(info.Size(), "preimage"); err != nil {
			return CASOutcome{}, err
		}
		if err := budget.reserve(int64(len(receiptRaw)), "replacement"); err != nil {
			return CASOutcome{}, err
		}
	}
	if updateBackupSnapshotHook != nil {
		updateBackupSnapshotHook()
	}
	sources, err := validateLifecycle(ctx, layout, release, selectedReleasePaths(desired), budget)
	if err != nil {
		return CASOutcome{}, err
	}
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
		oldBytes, err := readStableRegular(ownedRoot(layout, next.Files[index].Path), next.Files[index].Path, next.Files[index].Bytes, nil, "")
		if err != nil {
			retained = append(retained, target.owned.Path)
			continue
		}
		backup := Backup{Role: next.Files[index].Role, Host: next.Files[index].Host, Path: backupPath(layout, next.Files[index]), SHA256: next.Files[index].SHA256, Version: next.Files[index].Version, Revision: next.Files[index].Revision, Bytes: next.Files[index].Bytes}
		backupExists := backups[backup.Path]
		if backupExists {
			if !diskMatches(backup.Path, backup.SHA256, backup.Bytes) {
				return CASOutcome{Retained: append(retained, target.owned.Path, backup.Path)}, core.ErrRevision
			}
		}
		data, err := verifiedReleaseBytes(target.source, sources)
		if err != nil {
			return CASOutcome{Retained: append(retained, target.owned.Path)}, err
		}
		mode := fs.FileMode(0o600)
		if target.owned.Role == BinaryRole {
			mode = 0o700
		}
		targetMutation, err := prepareMutation(layout, target.owned.Path, data, mode, false, false, nil)
		if err != nil {
			return CASOutcome{Retained: append(retained, target.owned.Path)}, err
		}
		if next.Files[index].Path != target.owned.Path && !targetMutation.Existed {
			targetMutation.Exclusive = true
		}
		if !backupExists {
			backupMutation, err := prepareMutation(layout, backup.Path, oldBytes, 0o600, false, true, nil)
			if err != nil {
				return CASOutcome{Retained: append(retained, target.owned.Path)}, err
			}
			mutations = append(mutations, backupMutation)
		}
		mutations = append(mutations, targetMutation)
		if next.Files[index].Path != target.owned.Path {
			removeMutation, err := prepareMutation(layout, next.Files[index].Path, nil, 0, true, false, nil)
			if err != nil {
				return CASOutcome{Retained: append(retained, next.Files[index].Path)}, err
			}
			mutations = append(mutations, removeMutation)
		}
		next.Backups = appendBackup(next.Backups, backup)
		next.Files[index] = target.owned
	}
	if hostReceipt != nil {
		receiptPath := filepath.Join(layout.DataRoot, filepath.FromSlash(hostCutoverReceiptRel))
		receiptMutation, err := prepareMutation(layout, receiptPath, receiptRaw, 0o600, false, false, nil)
		if err != nil {
			return CASOutcome{Retained: append(retained, receiptPath)}, err
		}
		mutations = append(mutations, receiptMutation)
	}
	previous := original
	intended := cloneManifest(next)
	intended.Revision = expected + 1
	intended.HostHomes, _ = layoutHostHomes(layout)
	journal := lifecycleJournal{Schema: 1, Operation: "update", ExpectedRevision: expected, Owner: owner, Previous: &previous, Intended: intended, Mutations: mutations, Retained: retained}
	if err := prepareDiscoveryJournal(layout, &journal); err != nil {
		return CASOutcome{}, err
	}
	outcome, err := executeLifecycleJournal(ctx, layout, manifestStore, &journal)
	outcome.Retained = append([]string(nil), retained...)
	return outcome, err
}

func installedReleaseMatches(current InstallManifest, desired []desiredFile) bool {
	if len(desired) == 0 || len(current.Files) != len(desired) || current.Version != desired[0].owned.Version || current.ReleaseRevision != desired[0].owned.Revision {
		return false
	}
	for _, target := range desired {
		index := ownedIndex(current.Files, target.owned.Role, target.owned.Host)
		if index < 0 || current.Files[index] != target.owned || !diskMatches(target.owned.Path, target.owned.SHA256, target.owned.Bytes) {
			return false
		}
	}
	return true
}

func pendingReleaseFiles(current InstallManifest, desired []desiredFile) []desiredFile {
	pending := make([]desiredFile, 0, len(desired))
	for _, target := range desired {
		index := ownedIndex(current.Files, target.owned.Role, target.owned.Host)
		if index < 0 || current.Files[index] != target.owned || !diskMatches(target.owned.Path, target.owned.SHA256, target.owned.Bytes) {
			pending = append(pending, target)
		}
	}
	return pending
}

func Rollback(ctx context.Context, layout Layout, version string, expected uint64) (CASOutcome, error) {
	return rollback(ctx, layout, version, "", expected)
}

// RollbackRelease selects one exact release when a version has multiple backups.
func RollbackRelease(ctx context.Context, layout Layout, version, revision string, expected uint64) (CASOutcome, error) {
	return rollback(ctx, layout, version, revision, expected)
}

func rollback(ctx context.Context, layout Layout, version, revision string, expected uint64) (CASOutcome, error) {
	operation := fmt.Sprintf("rollback:%d:%s", expected, version)
	if revision != "" {
		operation += "@" + revision
	}
	releaseGuard, err := store.AcquireProjectMutation(ctx, layout.DataRoot, "install", operation)
	if err != nil {
		return CASOutcome{}, err
	}
	defer func() { _ = releaseGuard.Release() }()
	manifestStore := NewManifestStore(layout)
	if err := recoverLifecycleJournal(ctx, layout, manifestStore); err != nil {
		return CASOutcome{}, err
	}
	return rollbackLocked(ctx, layout, version, revision, expected, releaseGuard.Owner())
}

func rollbackLocked(ctx context.Context, layout Layout, version, revision string, expected uint64, owner store.MutationOwner) (CASOutcome, error) {
	if ctx == nil || ctx.Err() != nil || ValidateLayout(layout) != nil || strings.TrimSpace(version) == "" || (revision != "" && !validReleaseRevision(revision)) {
		return CASOutcome{}, core.ErrSettings
	}
	manifestStore := NewManifestStore(layout)
	budget := newJournalBudget()
	current, stale, err := currentManifest(ctx, manifestStore, expected)
	if err != nil {
		return stale, err
	}
	if err := validateInstalledLayout(layout, current); err != nil {
		return CASOutcome{}, err
	}
	original := cloneManifest(current)
	current = reconcileMovedEntrypoints(layout, current)
	selected, targetRevision, err := selectRollbackBackups(current, version, revision)
	if err != nil {
		return CASOutcome{}, err
	}
	hostReceipt, err := lifecycleHostCutoverReceipt(layout, current)
	if err != nil {
		return CASOutcome{}, err
	}
	plan := rollbackJournalPlan(layout, current, version, targetRevision, selected, owner, expected)
	var receiptRaw []byte
	if hostReceipt != nil {
		receiptDesired := make([]desiredFile, 0, len(selected))
		for _, backup := range selected {
			if backup.Role == EntrypointRole {
				receiptDesired = append(receiptDesired, desiredFile{owned: OwnedFile{Role: backup.Role, Host: backup.Host, Path: filepath.Join(layout.SkillRoots[backup.Host], "SKILL.md"), SHA256: backup.SHA256, Version: backup.Version, Revision: backup.Revision, Bytes: backup.Bytes}})
			}
		}
		receiptRaw, err = updatedHostCutoverReceipt(*hostReceipt, version, targetRevision, receiptDesired)
		if err != nil {
			return CASOutcome{}, err
		}
		receiptPath := filepath.Join(layout.DataRoot, filepath.FromSlash(hostCutoverReceiptRel))
		plan.Mutations = append(plan.Mutations, lifecycleMutation{Path: receiptPath, Existed: true, PreSHA256: strings.Repeat("0", 64), PreMode: plannedMode(receiptPath), PostMode: 0o600, PostSHA256: digestContent(receiptRaw), PostBytes: int64(len(receiptRaw))})
	}
	if err := budget.accountMetadata(plan); err != nil {
		return CASOutcome{}, err
	}
	for _, backup := range selected {
		index := ownedIndex(current.Files, backup.Role, backup.Host)
		if index >= 0 && (current.Files[index].Version != version || current.Files[index].Revision != targetRevision) {
			if err := budget.reserve(backup.Bytes, "replacement"); err != nil {
				return CASOutcome{}, err
			}
			if err := budget.reserve(current.Files[index].Bytes, "preimage"); err != nil {
				return CASOutcome{}, err
			}
		}
	}
	if hostReceipt != nil {
		receiptPath := filepath.Join(layout.DataRoot, filepath.FromSlash(hostCutoverReceiptRel))
		info, statErr := os.Lstat(receiptPath)
		if statErr != nil || !info.Mode().IsRegular() {
			return CASOutcome{}, core.ErrRevision
		}
		if err := budget.reserve(info.Size(), "preimage"); err != nil {
			return CASOutcome{}, err
		}
		if err := budget.reserve(int64(len(receiptRaw)), "replacement"); err != nil {
			return CASOutcome{}, err
		}
	}
	next := cloneManifest(current)
	retained := manifestMismatches(current)
	restored := 0
	mutations := []lifecycleMutation{}
	for _, backup := range selected {
		index := ownedIndex(next.Files, backup.Role, backup.Host)
		if index >= 0 && next.Files[index].Version == version && next.Files[index].Revision == targetRevision {
			if !diskMatches(next.Files[index].Path, next.Files[index].SHA256, next.Files[index].Bytes) {
				retained = append(retained, next.Files[index].Path)
			}
			continue
		}
		if index < 0 || !diskMatches(next.Files[index].Path, next.Files[index].SHA256, next.Files[index].Bytes) || !diskMatches(backup.Path, backup.SHA256, backup.Bytes) {
			return CASOutcome{Retained: append(retained, backup.Path)}, core.ErrRevision
		}
		data, err := readStableRegular(ownedRoot(layout, backup.Path), backup.Path, backup.Bytes, nil, "")
		if err != nil {
			return CASOutcome{Retained: retained}, err
		}
		mode := fs.FileMode(0o600)
		if backup.Role == BinaryRole {
			mode = 0o700
		}
		mutation, err := prepareMutation(layout, next.Files[index].Path, data, mode, false, false, nil)
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
	if hostReceipt != nil {
		receiptPath := filepath.Join(layout.DataRoot, filepath.FromSlash(hostCutoverReceiptRel))
		receiptMutation, err := prepareMutation(layout, receiptPath, receiptRaw, 0o600, false, false, nil)
		if err != nil {
			return CASOutcome{Retained: append(retained, receiptPath)}, err
		}
		mutations = append(mutations, receiptMutation)
	}
	next.Version, next.ReleaseRevision = version, targetRevision
	previous := original
	intended := cloneManifest(next)
	intended.Revision = expected + 1
	intended.HostHomes, _ = layoutHostHomes(layout)
	journal := lifecycleJournal{Schema: 1, Operation: "rollback", ExpectedRevision: expected, Owner: owner, Previous: &previous, Intended: intended, Mutations: mutations, Retained: retained}
	if err := prepareDiscoveryJournal(layout, &journal); err != nil {
		return CASOutcome{}, err
	}
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
	budget := newJournalBudget()
	current, stale, err := currentManifest(ctx, manifestStore, expected)
	if err != nil {
		return stale.Retained, stale, err
	}
	if err := validateInstalledLayout(layout, current); err != nil {
		return nil, CASOutcome{}, err
	}
	original := cloneManifest(current)
	current = reconcileMovedEntrypoints(layout, current)
	if err := budget.accountMetadata(uninstallJournalPlan(current, owner, expected)); err != nil {
		return nil, CASOutcome{}, err
	}
	for _, file := range current.Files {
		if err := budget.reserve(file.Bytes, "preimage"); err != nil {
			return nil, CASOutcome{}, err
		}
	}
	for _, backup := range current.Backups {
		if err := budget.reserve(backup.Bytes, "preimage"); err != nil {
			return nil, CASOutcome{}, err
		}
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
		mutation, err := prepareMutation(layout, file.Path, nil, 0, true, false, nil)
		if err != nil {
			if errors.Is(err, errJournalBudget) {
				return retained, CASOutcome{Retained: retained}, err
			}
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
		mutation, err := prepareMutation(layout, backup.Path, nil, 0, true, false, nil)
		if err != nil {
			if errors.Is(err, errJournalBudget) {
				return retained, CASOutcome{Retained: retained}, err
			}
			retained = append(retained, backup.Path)
			next.Backups = append(next.Backups, backup)
			continue
		}
		mutations = append(mutations, mutation)
	}
	previous := original
	intended := cloneManifest(next)
	intended.Revision = expected + 1
	intended.HostHomes, _ = layoutHostHomes(layout)
	journal := lifecycleJournal{Schema: 1, Operation: "uninstall", ExpectedRevision: expected, Owner: owner, Previous: &previous, Intended: intended, Mutations: mutations, Retained: retained}
	if err := prepareDiscoveryJournal(layout, &journal); err != nil {
		return retained, CASOutcome{}, err
	}
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

// rejectUnownedCodexSkill stops native lifecycle changes when another visible
// Agent-Team skill could win discovery. Only a manifest-owned top-level file is safe to reuse.
func rejectUnownedCodexSkill(layout Layout, current *InstallManifest) (CASOutcome, error) {
	owned := map[string]bool{}
	if current != nil {
		for _, file := range current.Files {
			owned[file.Path] = true
			if file.Role == EntrypointRole && file.Path == nestedEntrypoint(layout, file.Host) {
				top := filepath.Join(layout.SkillRoots[file.Host], "SKILL.md")
				if diskMatches(top, file.SHA256, file.Bytes) {
					owned[top] = true
				}
			}
		}
	}
	paths := append(append([]string(nil), layout.CodexDiscoverySkillPaths...), filepath.Join(layout.SkillRoots[Codex], "SKILL.md"))
	seen := map[string]bool{}
	for _, path := range paths {
		if path == "" || seen[path] {
			continue
		}
		seen[path] = true
		root := filepath.Dir(path)
		if info, err := os.Lstat(root); err == nil && (!info.IsDir() || info.Mode()&os.ModeSymlink != 0) {
			return CASOutcome{Retained: []string{path}}, fmt.Errorf("%w: conflicting Codex Agent-Team skill at %s; move it to a recoverable backup outside discovery paths and retry", core.ErrRevision, path)
		} else if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return CASOutcome{Retained: []string{path}}, fmt.Errorf("%w: conflicting Codex Agent-Team skill at %s; move it to a recoverable backup outside discovery paths and retry", core.ErrRevision, path)
		}
		info, err := os.Lstat(path)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || !owned[path] {
			return CASOutcome{Retained: []string{path}}, fmt.Errorf("%w: conflicting Codex Agent-Team skill at %s; move it to a recoverable backup outside discovery paths and retry", core.ErrRevision, path)
		}
	}
	return CASOutcome{}, nil
}

func containsHost(hosts []Host, want Host) bool {
	for _, host := range hosts {
		if host == want {
			return true
		}
	}
	return false
}

func releaseFiles(layout Layout, release Release, hosts []Host) []desiredFile {
	files := []desiredFile{
		{OwnedFile{Role: BinaryRole, Path: layout.BinaryPath, SHA256: release.Binary.SHA256, Version: release.Version, Revision: release.Revision, Bytes: release.Binary.Bytes}, release.Binary},
		{OwnedFile{Role: ContractRole, Path: layout.ContractPath, SHA256: release.Contract.SHA256, Version: release.Version, Revision: release.Revision, Bytes: release.Contract.Bytes}, release.Contract},
	}
	for _, host := range hosts {
		entrypoint := release.Entrypoints[host]
		files = append(files, desiredFile{OwnedFile{Role: EntrypointRole, Host: host, Path: filepath.Join(layout.SkillRoots[host], "SKILL.md"), SHA256: entrypoint.SHA256, Version: release.Version, Revision: release.Revision, Bytes: entrypoint.Bytes}, entrypoint})
	}
	return files
}

func selectedReleasePaths(desired []desiredFile) map[string]bool {
	paths := make(map[string]bool, len(desired))
	for _, file := range desired {
		paths[file.source.Path] = true
	}
	return paths
}

func validateLifecycle(ctx context.Context, layout Layout, release Release, selected map[string]bool, budget *journalBudget) (map[string][]byte, error) {
	if ctx == nil || ctx.Err() != nil {
		return nil, core.ErrSettings
	}
	if err := ValidateLayout(layout); err != nil {
		return nil, err
	}
	if strings.TrimSpace(release.Version) == "" || !validReleaseRevision(release.Revision) || len(release.Entrypoints) != 2 {
		return nil, core.ErrRevision
	}
	sources := make(map[string][]byte, 4)
	files := []ReleaseFile{release.Binary, release.Contract, release.Entrypoints[Codex], release.Entrypoints[Claude]}
	for _, file := range files {
		if !validSHA256(file.SHA256) || file.Bytes < 0 {
			return nil, core.ErrRevision
		}
		opened, size, err := openStableRegular(filepath.Dir(file.Path), file.Path)
		if err != nil || size != file.Bytes {
			if opened != nil {
				_ = opened.Close()
			}
			return nil, core.ErrRevision
		}
		_ = opened.Close()
		if selected[file.Path] {
			if err := budget.reserve(size, "replacement"); err != nil {
				return nil, err
			}
		}
	}
	for _, file := range files {
		if !selected[file.Path] {
			digest, size, err := sha256File(file.Path)
			if err != nil || digest != file.SHA256 || size != file.Bytes {
				return nil, core.ErrRevision
			}
			continue
		}
		body, err := readStableRegular(filepath.Dir(file.Path), file.Path, file.Bytes, nil, "")
		if err != nil || digestContent(body) != file.SHA256 {
			return nil, core.ErrRevision
		}
		sources[file.Path] = body
	}
	return sources, nil
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

func verifiedReleaseBytes(file ReleaseFile, sources map[string][]byte) ([]byte, error) {
	body, ok := sources[file.Path]
	if !ok || int64(len(body)) != file.Bytes || digestContent(body) != file.SHA256 {
		return nil, core.ErrRevision
	}
	return body, nil
}

func readStableRegular(rootPath, path string, expected int64, budget *journalBudget, field string) ([]byte, error) {
	body, _, err := readStableRegularMode(rootPath, path, expected, budget, field)
	return body, err
}

func readStableRegularMode(rootPath, path string, expected int64, budget *journalBudget, field string) ([]byte, fs.FileMode, error) {
	file, size, err := openStableRegular(rootPath, path)
	if err != nil {
		return nil, 0, err
	}
	defer file.Close()
	if expected >= 0 && size != expected {
		return nil, 0, core.ErrRevision
	}
	if budget != nil {
		if err := budget.reserve(size, field); err != nil {
			return nil, 0, err
		}
	}
	body, err := io.ReadAll(io.LimitReader(file, size+1))
	if err != nil || int64(len(body)) != size {
		return nil, 0, core.ErrRevision
	}
	if err := verifyStableIdentity(file, path); err != nil {
		return nil, 0, err
	}
	info, err := file.Stat()
	if err != nil {
		return nil, 0, core.ErrRevision
	}
	return body, info.Mode(), nil
}

func verifyStableIdentity(file *os.File, path string) error {
	opened, err := file.Stat()
	if err != nil {
		return core.ErrRevision
	}
	current, err := os.Lstat(path)
	if err != nil || !current.Mode().IsRegular() || !os.SameFile(opened, current) {
		return core.ErrRevision
	}
	return nil
}

func openStableRegular(rootPath, path string) (*os.File, int64, error) {
	if rootPath == "" {
		return nil, 0, core.ErrPath
	}
	relative, err := filepath.Rel(rootPath, path)
	if err != nil || relative == "." || filepath.IsAbs(relative) || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return nil, 0, core.ErrPath
	}
	root, err := os.OpenRoot(rootPath)
	if err != nil {
		return nil, 0, err
	}
	file, err := root.OpenFile(relative, os.O_RDONLY|stableReadFlags(), 0)
	_ = root.Close()
	if err != nil {
		return nil, 0, core.ErrPath
	}
	if stableReadHook != nil {
		stableReadHook(path)
	}
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() < 0 || info.Size() > installFileLimit {
		_ = file.Close()
		return nil, 0, core.ErrPath
	}
	return file, info.Size(), nil
}

func diskMatches(path, digest string, bytes int64) bool {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Size() != bytes {
		return false
	}
	gotDigest, gotBytes, err := sha256File(path)
	return err == nil && gotDigest == digest && gotBytes == bytes
}

func diskMatchesMode(layout Layout, path, digest string, bytes int64, mode uint32) bool {
	body, currentMode, err := readStableRegularMode(ownedRoot(layout, path), path, bytes, nil, "")
	return err == nil && uint32(currentMode.Perm()) == mode && digestContent(body) == digest
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
	name := fmt.Sprintf("%s-%s-%s-%s.bak", file.Role, host, file.Revision, file.SHA256)
	return filepath.Join(layout.DataRoot, "backups", file.Version, name)
}

func appendBackup(backups []Backup, candidate Backup) []Backup {
	for _, backup := range backups {
		if backup.Role == candidate.Role && backup.Host == candidate.Host && backup.Version == candidate.Version && backup.Revision == candidate.Revision && backup.SHA256 == candidate.SHA256 {
			return backups
		}
	}
	return append(backups, candidate)
}

func ownedRoot(layout Layout, target string) string {
	for _, config := range layout.ConfigPaths {
		if target == config {
			return filepath.Dir(config)
		}
	}
	roots := []string{layout.DataRoot, layout.SkillRoots[Codex], layout.SkillRoots[Claude]}
	best := ""
	for _, root := range roots {
		if contained(root, target) && len(root) > len(best) {
			best = root
		}
	}
	return best
}

func removeOwnedPath(layout Layout, target, expectedDigest string) error {
	root := ownedRoot(layout, target)
	if root == "" || !validSHA256(expectedDigest) {
		return core.ErrPath
	}
	relative, err := filepath.Rel(root, target)
	if err != nil || relative == "." || filepath.IsAbs(relative) || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return core.ErrPath
	}
	return store.New(root, core.StorageLimits{CanonicalBytes: installFileLimit}).RemoveExact(filepath.ToSlash(relative), expectedDigest, installFileLimit)
}
