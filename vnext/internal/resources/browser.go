package resources

import (
	"context"
	"fmt"

	"github.com/thebpandey/agent-team/vnext/internal/core"
)

func validateBrowser(record BrowserRecord) error {
	if err := validateOwner(record.Owner); err != nil {
		return err
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
	if record.State == "started" && (record.TraceEvidence == "" || record.ExternalRef == "") {
		return fmt.Errorf("%w: started browser lacks lifecycle evidence", core.ErrTransition)
	}
	record.State = "released"
	doc.Revision++
	stampDocument(doc, record.Owner)
	*outcome = releaseBrowser(*record, expected, doc.Revision, true, "")
	return nil
}

func (r *registry) stopBrowser(ctx context.Context, doc *registryDocument, record *BrowserRecord, expected uint64, outcome *ReleaseOutcome) error {
	if record.Ownership != Managed || record.State != "started" || record.ExternalRef == "" || record.TraceEvidence == "" {
		*outcome = releaseBrowser(*record, expected, doc.Revision, false, "exact managed start evidence is required")
		return fmt.Errorf("%w: resource %q is not authorized for stop", core.ErrRevision, record.ID)
	}
	if r.runner == nil {
		return fmt.Errorf("%w: nil resource command runner", core.ErrSettings)
	}
	result := r.runner.Run(ctx, "agent-team-resource-stop", "browser", record.ID, record.ExternalRef)
	if result.Transport != nil || result.TimedOut || result.Exit != 0 {
		record.Ownership, record.State = Unknown, "unknown"
		if record.TraceEvidence == "" {
			record.TraceEvidence = "stop-failed"
		}
		doc.Revision++
		stampDocument(doc, record.Owner)
		*outcome = releaseBrowser(*record, expected, doc.Revision, false, "stop failed; retained as unknown")
		return fmt.Errorf("%w: managed browser stop failed", core.ErrTransition)
	}
	record.State = "stopped"
	doc.Revision++
	stampDocument(doc, record.Owner)
	*outcome = releaseBrowser(*record, expected, doc.Revision, true, "")
	return nil
}
