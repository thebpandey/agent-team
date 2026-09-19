package knowledge

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/thebpandey/agent-team/vnext/internal/core"
	"github.com/thebpandey/agent-team/vnext/internal/store"
)

const (
	blockerOpen                = "open"
	blockerResolved            = "resolved"
	maxBlockerDirectoryEntries = 256
)

// Blocker is a revisioned canonical fact. Resolution is retained in this
// record, while the derived root projection contains open items only.
type Blocker struct {
	core.RecordEnvelope
	ID                 string   `json:"id"`
	Severity           string   `json:"severity"`
	State              string   `json:"state"`
	Affected           []string `json:"affected,omitempty"`
	EvidencePointer    string   `json:"evidencePointer,omitempty"`
	RequestedChoice    string   `json:"requestedChoice,omitempty"`
	SafeDefault        string   `json:"safeDefault,omitempty"`
	ResolutionDecision string   `json:"resolutionDecision,omitempty"`
	Reason             string   `json:"reason,omitempty"`
}

// BlockerStore provides CAS-like updates for canonical blockers.
type BlockerStore interface {
	Create(context.Context, Blocker) (Blocker, error)
	Update(context.Context, Blocker) (Blocker, error)
	Resolve(context.Context, string, string) error
}

type blockerStore struct{ store *store.Store }

// NewBlockerStore creates a blocker store backed by the supplied canonical
// store. All methods observe the same root-local lock.
func NewBlockerStore(s *store.Store) BlockerStore { return &blockerStore{store: s} }

func blockerPath(id string) (string, error) {
	if err := validateSegment(id); err != nil {
		return "", err
	}
	return ".agent-team/blockers/" + id + ".json", nil
}

func validateBlocker(b Blocker) error {
	if err := validateEnvelope(b.RecordEnvelope); err != nil {
		return err
	}
	if _, err := blockerPath(b.ID); err != nil {
		return err
	}
	if b.Severity == "" || (b.State != blockerOpen && b.State != blockerResolved) {
		return fmt.Errorf("%w: blocker severity and state are required", core.ErrRevision)
	}
	if len(b.Affected) > 128 {
		return fmt.Errorf("%w: too many affected items", core.ErrLimit)
	}
	for _, id := range b.Affected {
		if err := validateSegment(id); err != nil {
			return err
		}
	}
	if b.EvidencePointer != "" {
		if err := validatePointer(b.EvidencePointer); err != nil {
			return err
		}
	}
	if b.ResolutionDecision != "" && !decisionIDPattern.MatchString(b.ResolutionDecision) {
		return fmt.Errorf("%w: invalid resolution decision", core.ErrPath)
	}
	if b.State == blockerOpen && b.ResolutionDecision != "" {
		return fmt.Errorf("%w: open blocker cannot have a resolution decision", core.ErrRevision)
	}
	if b.State == blockerResolved && b.ResolutionDecision == "" {
		return fmt.Errorf("%w: resolved blocker needs a resolution decision", core.ErrRevision)
	}
	for name, value := range map[string]string{"severity": b.Severity, "reason": b.Reason, "requestedChoice": b.RequestedChoice, "safeDefault": b.SafeDefault} {
		if err := validateText(value, name); err != nil {
			return err
		}
	}
	return nil
}

func decisionExists(s *store.Store, id string) (bool, error) {
	contents, err := readCanonicalMarkdown(s, "DECISIONS.md")
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return validDecisionMarkdown(string(contents), id), nil
}

func (b *blockerStore) Create(ctx context.Context, blocker Blocker) (Blocker, error) {
	if err := ctx.Err(); err != nil {
		return Blocker{}, err
	}
	if b == nil || b.store == nil {
		return Blocker{}, fmt.Errorf("%w: nil store", core.ErrPath)
	}
	if blocker.Revision == 0 {
		blocker.Revision = 1
	}
	if err := validateBlocker(blocker); err != nil {
		return Blocker{}, err
	}
	path, err := blockerPath(blocker.ID)
	if err != nil {
		return Blocker{}, err
	}
	lock := knowledgeLock(b.store.Root)
	lock.Lock()
	defer lock.Unlock()
	var existing Blocker
	err = b.store.ReadJSON(path, canonicalLimit(b.store), &existing)
	if err == nil {
		return Blocker{}, fmt.Errorf("%w: blocker already exists", ErrConflict)
	}
	if !errors.Is(err, os.ErrNotExist) {
		return Blocker{}, err
	}
	if blocker.Revision != 1 {
		return Blocker{}, fmt.Errorf("%w: new blocker revision must be 1", core.ErrRevision)
	}
	if blocker.State != blockerOpen {
		return Blocker{}, fmt.Errorf("%w: new blocker must be open", core.ErrRevision)
	}
	if _, err := b.store.WriteJSON(path, blocker, canonicalLimit(b.store)); err != nil {
		return Blocker{}, err
	}
	return blocker, nil
}

func (b *blockerStore) Update(ctx context.Context, blocker Blocker) (Blocker, error) {
	if err := ctx.Err(); err != nil {
		return Blocker{}, err
	}
	if b == nil || b.store == nil {
		return Blocker{}, fmt.Errorf("%w: nil store", core.ErrPath)
	}
	if err := validateBlocker(blocker); err != nil {
		return Blocker{}, err
	}
	path, err := blockerPath(blocker.ID)
	if err != nil {
		return Blocker{}, err
	}
	lock := knowledgeLock(b.store.Root)
	lock.Lock()
	defer lock.Unlock()
	var existing Blocker
	if err := b.store.ReadJSON(path, canonicalLimit(b.store), &existing); err != nil {
		return Blocker{}, err
	}
	if existing.Project != blocker.Project || existing.RunID != blocker.RunID || blocker.Revision != existing.Revision {
		return Blocker{}, fmt.Errorf("%w: blocker revision or provenance", core.ErrRevision)
	}
	if existing.State != blockerOpen || blocker.State != blockerOpen {
		return Blocker{}, fmt.Errorf("%w: use Resolve for blocker resolution", core.ErrRevision)
	}
	blocker.Revision++
	if blocker.WrittenAt == "" {
		blocker.WrittenAt = existing.WrittenAt
	}
	if _, err := b.store.WriteJSON(path, blocker, canonicalLimit(b.store)); err != nil {
		return Blocker{}, err
	}
	return blocker, nil
}

func (b *blockerStore) Resolve(ctx context.Context, id, decisionID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if b == nil || b.store == nil {
		return fmt.Errorf("%w: nil store", core.ErrPath)
	}
	path, err := blockerPath(id)
	if err != nil {
		return err
	}
	if !decisionIDPattern.MatchString(decisionID) {
		return fmt.Errorf("%w: resolution needs a DEC ID", core.ErrRevision)
	}
	lock := knowledgeLock(b.store.Root)
	lock.Lock()
	defer lock.Unlock()
	var blocker Blocker
	if err := b.store.ReadJSON(path, canonicalLimit(b.store), &blocker); err != nil {
		return err
	}
	if blocker.State == blockerResolved {
		if blocker.ResolutionDecision == decisionID {
			return nil
		}
		return fmt.Errorf("%w: blocker was resolved by another decision", ErrConflict)
	}
	if blocker.State != blockerOpen {
		return fmt.Errorf("%w: blocker is not open", core.ErrRevision)
	}
	exists, err := decisionExists(b.store, decisionID)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("%w: resolution decision is absent", core.ErrRevision)
	}
	blocker.State, blocker.ResolutionDecision, blocker.Revision = blockerResolved, decisionID, blocker.Revision+1
	if err := validateBlocker(blocker); err != nil {
		return err
	}
	_, err = b.store.WriteJSON(path, blocker, canonicalLimit(b.store))
	return err
}

func readCanonicalBlockers(s *store.Store) ([]Blocker, error) {
	root, err := os.OpenRoot(s.Root)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	const directory = ".agent-team/blockers"
	info, err := root.Lstat(directory)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return nil, fmt.Errorf("%w: blocker directory is not a directory", core.ErrPath)
	}
	dir, err := root.Open(directory)
	if err != nil {
		return nil, err
	}
	defer dir.Close()
	openedInfo, err := dir.Stat()
	if err != nil {
		return nil, err
	}
	currentInfo, err := root.Lstat(directory)
	if err != nil {
		return nil, err
	}
	if currentInfo.Mode()&os.ModeSymlink != 0 || !os.SameFile(info, openedInfo) || !os.SameFile(info, currentInfo) {
		return nil, fmt.Errorf("%w: blocker directory changed during open", core.ErrPath)
	}
	items := make([]Blocker, 0, 16)
	scanned := 0
	for {
		entries, readErr := dir.ReadDir(32)
		for _, entry := range entries {
			scanned++
			if scanned > maxBlockerDirectoryEntries {
				return nil, fmt.Errorf("%w: too many blocker directory entries", core.ErrLimit)
			}
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
				continue
			}
			if len(items) == 128 {
				return nil, fmt.Errorf("%w: too many canonical blockers", core.ErrLimit)
			}
			id := strings.TrimSuffix(entry.Name(), ".json")
			path, err := blockerPath(id)
			if err != nil {
				return nil, err
			}
			var item Blocker
			if err := s.ReadJSON(path, canonicalLimit(s), &item); err != nil {
				return nil, err
			}
			if item.ID != id {
				return nil, fmt.Errorf("%w: blocker filename and ID conflict", core.ErrRevision)
			}
			if err := validateBlocker(item); err != nil {
				return nil, err
			}
			items = append(items, item)
		}
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			return nil, readErr
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	return items, nil
}
