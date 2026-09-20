// Package knowledge stores bounded recovery facts and their deterministic
// Markdown projections. It deliberately has no authority to run or resume work.
package knowledge

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/thebpandey/agent-team/vnext/internal/core"
	"github.com/thebpandey/agent-team/vnext/internal/project"
	"github.com/thebpandey/agent-team/vnext/internal/store"
)

const canonicalHardBytes int64 = 16 << 20

// ErrConflict reports an immutable-record collision or digest mismatch.
var ErrConflict = errors.New("knowledge conflict")

// Receipt is the current, revisioned recovery fact for one team.
type Receipt struct {
	core.RecordEnvelope
	Team             string         `json:"team"`
	Task             string         `json:"task"`
	Attempt          int            `json:"attempt"`
	State            core.TaskState `json:"state"`
	Base             string         `json:"base,omitempty"`
	Head             string         `json:"head,omitempty"`
	Gate             string         `json:"gate,omitempty"`
	Review           string         `json:"review,omitempty"`
	EvidencePointers []string       `json:"evidencePointers,omitempty"`
	// Resources intentionally contains only stable registry references. The
	// registry remains the lifecycle authority for resource payloads.
	Resources  core.ResourceSnapshot `json:"resources,omitempty"`
	NextAction string                `json:"nextAction"`
}

var knowledgeLocks sync.Map

func knowledgeLock(root string) *sync.Mutex {
	value, _ := knowledgeLocks.LoadOrStore(filepath.Clean(root), &sync.Mutex{})
	return value.(*sync.Mutex)
}

func canonicalLimit(s *store.Store) int64 {
	if s != nil && s.Limits.CanonicalBytes > 0 && s.Limits.CanonicalBytes < canonicalHardBytes {
		return s.Limits.CanonicalBytes
	}
	return canonicalHardBytes
}

func validateEnvelope(e core.RecordEnvelope) error {
	if e.Schema != 1 {
		return fmt.Errorf("%w: schema must be 1", core.ErrRevision)
	}
	if e.Project == "" || e.RunID == "" || e.Revision == 0 || e.WrittenAt == "" {
		return fmt.Errorf("%w: project, run ID, writtenAt, and revision are required", core.ErrRevision)
	}
	if _, err := time.Parse(time.RFC3339, e.WrittenAt); err != nil {
		return fmt.Errorf("%w: writtenAt: %v", core.ErrRevision, err)
	}
	if err := validateText(e.Project, "project"); err != nil {
		return err
	}
	if err := validateSegment(string(e.RunID)); err != nil {
		return err
	}
	return nil
}

func readCanonicalMarkdown(s *store.Store, relative string) ([]byte, error) {
	root, err := os.OpenRoot(s.Root)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	listedInfo, err := root.Lstat(relative)
	if err != nil {
		return nil, err
	}
	if listedInfo.Mode()&os.ModeSymlink != 0 || !listedInfo.Mode().IsRegular() {
		return nil, fmt.Errorf("%w: canonical Markdown is not a regular file", core.ErrPath)
	}
	file, err := root.Open(relative)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	openedInfo, err := file.Stat()
	if err != nil {
		return nil, err
	}
	currentInfo, err := root.Lstat(relative)
	if err != nil {
		return nil, err
	}
	if currentInfo.Mode()&os.ModeSymlink != 0 || !os.SameFile(listedInfo, openedInfo) || !os.SameFile(listedInfo, currentInfo) {
		return nil, fmt.Errorf("%w: canonical Markdown changed during open", core.ErrPath)
	}
	if !openedInfo.Mode().IsRegular() || openedInfo.Size() > canonicalLimit(s) {
		return nil, fmt.Errorf("%w: canonical Markdown exceeds limit", core.ErrLimit)
	}
	data, err := io.ReadAll(io.LimitReader(file, canonicalLimit(s)+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > canonicalLimit(s) {
		return nil, fmt.Errorf("%w: canonical Markdown exceeds limit", core.ErrLimit)
	}
	return data, nil
}

func validateSegment(value string) error {
	if err := project.ValidateSegment(value); err != nil {
		return err
	}
	return nil
}

func validatePointer(pointer string) error {
	if pointer == "" || len(pointer) > 4096 || filepath.IsAbs(pointer) || strings.Contains(pointer, "\\") {
		return fmt.Errorf("%w: unsafe evidence pointer", core.ErrPath)
	}
	for _, segment := range strings.Split(pointer, "/") {
		if err := validateSegment(segment); err != nil {
			return fmt.Errorf("%w: evidence pointer", err)
		}
	}
	return nil
}

func validateText(value, name string) error {
	if len(value) > 4096 || strings.Contains(value, "\x00") {
		return fmt.Errorf("%w: %s is too large or malformed", core.ErrLimit, name)
	}
	return nil
}

func sameJSON(left, right any) (bool, error) {
	a, err := json.Marshal(left)
	if err != nil {
		return false, err
	}
	b, err := json.Marshal(right)
	if err != nil {
		return false, err
	}
	return string(a) == string(b), nil
}

func receiptPath(team string) (string, error) {
	if err := validateSegment(team); err != nil {
		return "", err
	}
	return ".agent-team/receipts/" + team + ".json", nil
}

func validateReceipt(r Receipt) error {
	if err := validateEnvelope(r.RecordEnvelope); err != nil {
		return err
	}
	if err := validateSegment(r.Team); err != nil {
		return err
	}
	if err := validateSegment(r.Task); err != nil {
		return err
	}
	if r.Attempt < 1 || r.State == "" || r.NextAction == "" {
		return fmt.Errorf("%w: receipt task, attempt, state, and next action are required", core.ErrRevision)
	}
	if len(r.EvidencePointers) > 128 {
		return fmt.Errorf("%w: too many receipt evidence pointers", core.ErrLimit)
	}
	for _, pointer := range r.EvidencePointers {
		if err := validatePointer(pointer); err != nil {
			return err
		}
	}
	if err := validateResourceReferences(r.Resources); err != nil {
		return err
	}
	for name, value := range map[string]string{"base": r.Base, "head": r.Head, "gate": r.Gate, "review": r.Review, "nextAction": r.NextAction} {
		if err := validateText(value, name); err != nil {
			return err
		}
	}
	return nil
}

func validateResourceReferences(resources core.ResourceSnapshot) error {
	if len(resources.Servers) > 64 || len(resources.Browsers) > 64 || len(resources.External) > 128 {
		return fmt.Errorf("%w: too many resource references", core.ErrLimit)
	}
	seen := map[string]bool{}
	for _, group := range []struct {
		prefix string
		values []string
	}{{"S-", resources.Servers}, {"B-", resources.Browsers}} {
		for _, value := range group.values {
			if !strings.HasPrefix(value, group.prefix) || strings.ContainsAny(value, "{}[]\" \\t\\r\\n") || seen[value] {
				return fmt.Errorf("%w: invalid resource reference", core.ErrPath)
			}
			seen[value] = true
		}
	}
	for _, value := range resources.External {
		if strings.ContainsAny(value, " \t\r\n") || seen[value] {
			return fmt.Errorf("%w: invalid resource reference", core.ErrPath)
		}
		if err := validatePointer(value); err != nil {
			return err
		}
		seen[value] = true
	}
	return nil
}

// WriteReceipt atomically replaces a team's current receipt only with a newer
// revision. Repeating identical current content is harmless.
func WriteReceipt(ctx context.Context, s *store.Store, receipt Receipt) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if s == nil {
		return fmt.Errorf("%w: nil store", core.ErrPath)
	}
	if err := validateReceipt(receipt); err != nil {
		return err
	}
	path, err := receiptPath(receipt.Team)
	if err != nil {
		return err
	}
	lock := knowledgeLock(s.Root)
	lock.Lock()
	defer lock.Unlock()
	var previous Receipt
	err = s.ReadJSON(path, canonicalLimit(s), &previous)
	if err == nil {
		equal, compareErr := sameJSON(previous, receipt)
		if compareErr != nil {
			return compareErr
		}
		if equal {
			return nil
		}
		if previous.Project != receipt.Project || previous.RunID != receipt.RunID || receipt.Revision <= previous.Revision {
			return fmt.Errorf("%w: receipt revision is stale or provenance changed", core.ErrRevision)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	_, err = s.WriteJSON(path, receipt, canonicalLimit(s))
	return err
}

func sortedStrings(values []string) []string {
	copyValues := append([]string(nil), values...)
	sort.Strings(copyValues)
	return copyValues
}
