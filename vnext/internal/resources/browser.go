package resources

import (
	"fmt"
	"strings"

	"github.com/thebpandey/agent-team/vnext/internal/core"
)

func validateBrowser(record BrowserRecord) error {
	if err := validateOwner(record.Owner); err != nil {
		return err
	}
	if !strings.HasPrefix(record.ID, "B-") {
		return fmt.Errorf("%w: browser ID must use B- prefix", core.ErrPath)
	}
	if err := validateText(record.ID); err != nil {
		return err
	}
	if invalidText(record.Session) || invalidText(record.Target) || invalidText(record.Purpose) {
		return fmt.Errorf("%w: invalid browser record", core.ErrPath)
	}
	if record.URL != "" && invalidText(record.URL) {
		return fmt.Errorf("%w: invalid browser URL", core.ErrPath)
	}
	if record.Ownership != Managed && record.Ownership != Unknown && record.Ownership != UserOwned {
		return fmt.Errorf("%w: invalid browser ownership", core.ErrRevision)
	}
	return nil
}

func browserCollision(records []BrowserRecord, want BrowserRecord) string {
	for _, record := range records {
		if record.ID == want.ID {
			return "id:" + want.ID
		}
		if record.Session == want.Session {
			return "session:" + want.Session
		}
		if want.URL != "" && record.URL == want.URL {
			return "url:" + want.URL
		}
	}
	return ""
}

func reusableBrowser(records []BrowserRecord, want BrowserRecord) *BrowserRecord {
	for i := range records {
		record := &records[i]
		// Stable IDs make reuse explicit; identical purpose alone is not an
		// authorization to collapse two independently requested sessions.
		if record.ID == want.ID && record.Ownership == Managed && !terminal(record.State) && record.Owner.Worktree == want.Owner.Worktree && record.Owner.Revision == want.Owner.Revision && record.Purpose == want.Purpose {
			return record
		}
	}
	return nil
}

func managedBrowserCount(records []BrowserRecord) int {
	count := 0
	for _, record := range records {
		if record.Ownership == Managed || record.Ownership == Unknown {
			count++
		}
	}
	return count
}

func findBrowser(records []BrowserRecord, id string) *BrowserRecord {
	for i := range records {
		if records[i].ID == id {
			return &records[i]
		}
	}
	return nil
}

func browserOutcome(record BrowserRecord, expected, revision uint64) ReserveOutcome {
	return ReserveOutcome{ID: record.ID, ExpectedRevision: expected, Revision: revision, State: record.State, TraceEvidence: record.TraceEvidence, ExternalRef: record.ExternalRef}
}

func releaseBrowser(record BrowserRecord, expected, revision uint64, released bool, protected string) ReleaseOutcome {
	return ReleaseOutcome{ID: record.ID, ExpectedRevision: expected, Revision: revision, Released: released, State: record.State, TraceEvidence: record.TraceEvidence, ExternalRef: record.ExternalRef, ProtectedReason: protected}
}

func releaseManagedBrowser(doc *registryDocument, record *BrowserRecord, expected uint64, outcome *ReleaseOutcome) error {
	if record.Ownership != Managed {
		*outcome = releaseBrowser(*record, expected, doc.Revision, false, "resource is not managed")
		return fmt.Errorf("%w: resource %q is protected", core.ErrRevision, record.ID)
	}
	if terminal(record.State) {
		*outcome = releaseBrowser(*record, expected, doc.Revision, true, "")
		return nil
	}
	if record.State != "reserved" && record.State != "stopped" {
		return fmt.Errorf("%w: only unstarted or durably stopped browsers can release", core.ErrTransition)
	}
	if record.State == "stopped" && !strings.Contains(record.TraceEvidence, "stop:") {
		return fmt.Errorf("%w: stopped browser lacks stop evidence", core.ErrTransition)
	}
	record.State = "released"
	doc.Revision++
	stampBrowser(record, record.Owner, doc.Revision)
	*outcome = releaseBrowser(*record, expected, doc.Revision, true, "")
	return nil
}
