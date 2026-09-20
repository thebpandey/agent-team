package resources

import (
	"fmt"
	"strings"

	"github.com/thebpandey/agent-team/vnext/internal/core"
)

func validateServer(record ServerRecord) error {
	if err := validateOwner(record.Owner); err != nil {
		return err
	}
	if !strings.HasPrefix(record.ID, "S-") {
		return fmt.Errorf("%w: server ID must use S- prefix", core.ErrPath)
	}
	if err := validateText(record.ID); err != nil {
		return err
	}
	if record.Port < 0 || record.Port > 65535 || invalidText(record.Target) || invalidText(record.Purpose) {
		return fmt.Errorf("%w: invalid server record", core.ErrPath)
	}
	if record.URL != "" && invalidText(record.URL) {
		return fmt.Errorf("%w: invalid server URL", core.ErrPath)
	}
	if record.Ownership != Managed && record.Ownership != Unknown && record.Ownership != UserOwned {
		return fmt.Errorf("%w: invalid server ownership", core.ErrRevision)
	}
	return nil
}

func serverCollision(records []ServerRecord, want ServerRecord) string {
	for _, record := range records {
		if record.ID == want.ID {
			return "id:" + want.ID
		}
		if want.Port != 0 && record.Port == want.Port {
			return fmt.Sprintf("port:%d", want.Port)
		}
		if want.URL != "" && record.URL == want.URL {
			return "url:" + want.URL
		}
	}
	return ""
}

func reusableServer(records []ServerRecord, want ServerRecord) *ServerRecord {
	for i := range records {
		record := &records[i]
		// Stable IDs make reuse explicit. Matching only worktree/revision/purpose
		// would accidentally merge two independently requested server slots.
		if record.ID == want.ID && record.Ownership == want.Ownership && record.Ownership == Managed && !terminal(record.State) && record.Owner == want.Owner && record.Target == want.Target && record.Purpose == want.Purpose && record.Port == want.Port && record.URL == want.URL && record.ExternalRef == want.ExternalRef {
			return record
		}
	}
	return nil
}

func managedServerCount(records []ServerRecord) int {
	count := 0
	for _, record := range records {
		if record.Ownership == Managed || record.Ownership == Unknown {
			count++
		}
	}
	return count
}

func findServer(records []ServerRecord, id string) *ServerRecord {
	for i := range records {
		if records[i].ID == id {
			return &records[i]
		}
	}
	return nil
}

func reserveOutcome(record ServerRecord, expected, revision uint64) ReserveOutcome {
	return ReserveOutcome{ID: record.ID, ExpectedRevision: expected, Revision: revision, State: record.State, TraceEvidence: record.TraceEvidence, ExternalRef: record.ExternalRef}
}

func releaseServer(record ServerRecord, expected, revision uint64, released bool, protected string) ReleaseOutcome {
	return ReleaseOutcome{ID: record.ID, ExpectedRevision: expected, Revision: revision, Released: released, State: record.State, TraceEvidence: record.TraceEvidence, ExternalRef: record.ExternalRef, ProtectedReason: protected}
}

func releaseManagedServer(doc *registryDocument, record *ServerRecord, expected uint64, outcome *ReleaseOutcome) error {
	if record.Ownership != Managed {
		*outcome = releaseServer(*record, expected, doc.Revision, false, "resource is not managed")
		return fmt.Errorf("%w: resource %q is protected", core.ErrRevision, record.ID)
	}
	if terminal(record.State) {
		*outcome = releaseServer(*record, expected, doc.Revision, true, "")
		return nil
	}
	if record.State != "reserved" && record.State != "stopped" {
		return fmt.Errorf("%w: only unstarted or durably stopped servers can release", core.ErrTransition)
	}
	if record.State == "stopped" && !strings.Contains(record.TraceEvidence, "stop:") {
		return fmt.Errorf("%w: stopped server lacks stop evidence", core.ErrTransition)
	}
	record.State = "released"
	doc.Revision++
	stampServer(record, record.Owner, doc.Revision)
	*outcome = releaseServer(*record, expected, doc.Revision, true, "")
	return nil
}
