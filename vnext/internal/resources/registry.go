package resources

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/thebpandey/agent-team/vnext/internal/core"
	"github.com/thebpandey/agent-team/vnext/internal/store"
	"github.com/thebpandey/agent-team/vnext/internal/tracker"
)

const (
	registryPath = ".agent-team/resources/registry.json"
	registryLock = ".agent-team/resources/registry.lock"
	registryMax  = 1 << 20
	recordLimit  = 64
	serverSlots  = 2
	browserSlots = 2
)

// Ownership distinguishes resources Agent-Team may stop from resources it may
// only report. PID and port observations never change this value.
type Ownership string

const (
	Managed   Ownership = "managed"
	Unknown   Ownership = "unknown"
	UserOwned Ownership = "user-owned"
)

// ResourceOwner is the immutable provenance required before a resource can be
// managed. It is copied into each version of the project registry.
type ResourceOwner struct {
	Run      core.RunID  `json:"run"`
	Team     core.TeamID `json:"team"`
	Task     core.TaskID `json:"task"`
	Worktree string      `json:"worktree"`
	Revision string      `json:"revision"`
}

type ServerRecord struct {
	core.RecordEnvelope
	ID, URL, Target, Purpose                             string
	Port                                                 int
	Owner                                                ResourceOwner
	State                                                string
	Ownership                                            Ownership
	StartedAt, TraceEvidence, ExternalRef, PIDDiagnostic string
	StopAttempt                                          string
}

type BrowserRecord struct {
	core.RecordEnvelope
	ID, Session, URL, Target, Purpose     string
	Owner                                 ResourceOwner
	State                                 string
	Ownership                             Ownership
	StartedAt, TraceEvidence, ExternalRef string
	StopAttempt                           string
}

type ReserveOutcome struct {
	ID                                              string
	ExpectedRevision, Revision                      uint64
	CollisionKey, State, TraceEvidence, ExternalRef string
}

type ReleaseOutcome struct {
	ID                                                 string
	ExpectedRevision, Revision                         uint64
	Released                                           bool
	State, TraceEvidence, ExternalRef, ProtectedReason string
}

// Registry is the lifecycle authority for managed development resources.
// Consumers such as Playwright receive validated references; they never use
// this interface to reserve, release, or stop a resource on their own.
type Registry interface {
	List(context.Context, string) ([]ServerRecord, []BrowserRecord, error)
	ReserveServer(context.Context, ServerRecord, uint64) (ReserveOutcome, error)
	ReserveBrowser(context.Context, BrowserRecord, uint64) (ReserveOutcome, error)
	MarkStarted(context.Context, string, uint64, string, string) (ReserveOutcome, error)
	Release(context.Context, string, uint64) (ReleaseOutcome, error)
	StopManaged(context.Context, string, uint64) (ReleaseOutcome, error)
	MarkUnknown(context.Context, string, uint64, string) (ReleaseOutcome, error)
}
type StopRequest struct {
	Kind    string
	Server  *ServerRecord
	Browser *BrowserRecord
}
type StopFunc func(context.Context, StopRequest) tracker.CommandResult

type registry struct {
	store   *store.Store
	stopper StopFunc
}

type registryDocument struct {
	Schema   int             `json:"schema"`
	Revision uint64          `json:"revision"`
	Servers  []ServerRecord  `json:"servers"`
	Browsers []BrowserRecord `json:"browsers"`
}

type stopIntent struct {
	kind, id, token string
	expected        uint64
}

// NewRegistry returns the one durable project registry rooted in s. The lock
// is a portable, scoped directory CAS, so independently constructed Stores
// coordinating the same root serialize and then re-read durable state.
func NewRegistry(s *store.Store, stopper StopFunc) Registry {
	return &registry{store: s, stopper: stopper}
}

func (r *registry) List(ctx context.Context, run string) ([]ServerRecord, []BrowserRecord, error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	doc, err := r.read()
	if err != nil {
		return nil, nil, err
	}
	servers := make([]ServerRecord, 0, len(doc.Servers))
	browsers := make([]BrowserRecord, 0, len(doc.Browsers))
	for _, record := range doc.Servers {
		if run == "" || string(record.Owner.Run) == run {
			servers = append(servers, record)
		}
	}
	for _, record := range doc.Browsers {
		if run == "" || string(record.Owner.Run) == run {
			browsers = append(browsers, record)
		}
	}
	return servers, browsers, nil
}

func (r *registry) ReserveServer(ctx context.Context, record ServerRecord, expected uint64) (ReserveOutcome, error) {
	if err := validateServer(record); err != nil {
		return ReserveOutcome{}, err
	}
	var outcome ReserveOutcome
	err := r.withDocument(ctx, func(doc *registryDocument) error {
		cleanupTerminals(doc)
		if reused := reusableServer(doc.Servers, record); reused != nil {
			outcome = reserveOutcome(*reused, expected, doc.Revision)
			return nil
		}
		if key := serverCollision(doc.Servers, record); key != "" {
			outcome = ReserveOutcome{ID: record.ID, ExpectedRevision: expected, Revision: doc.Revision, CollisionKey: key}
			return fmt.Errorf("%w: server %s", core.ErrCapacity, key)
		}
		if err := checkReserveRevision(expected, doc.Revision); err != nil {
			return err
		}
		if len(doc.Servers) >= recordLimit || reserveManaged("server", managedServerCount(doc.Servers)) != nil {
			return fmt.Errorf("%w: two server slots are occupied", core.ErrCapacity)
		}
		record.State = initialState(record.Ownership)
		doc.Servers = append(doc.Servers, record)
		doc.Revision++
		stampServer(&doc.Servers[len(doc.Servers)-1], record.Owner, doc.Revision)
		outcome = reserveOutcome(record, expected, doc.Revision)
		return nil
	})
	return outcome, err
}

func (r *registry) ReserveBrowser(ctx context.Context, record BrowserRecord, expected uint64) (ReserveOutcome, error) {
	if err := validateBrowser(record); err != nil {
		return ReserveOutcome{}, err
	}
	var outcome ReserveOutcome
	err := r.withDocument(ctx, func(doc *registryDocument) error {
		cleanupTerminals(doc)
		if reused := reusableBrowser(doc.Browsers, record); reused != nil {
			outcome = browserOutcome(*reused, expected, doc.Revision)
			return nil
		}
		if key := browserCollision(doc.Browsers, record); key != "" {
			outcome = ReserveOutcome{ID: record.ID, ExpectedRevision: expected, Revision: doc.Revision, CollisionKey: key}
			return fmt.Errorf("%w: browser %s", core.ErrCapacity, key)
		}
		if err := checkReserveRevision(expected, doc.Revision); err != nil {
			return err
		}
		if len(doc.Browsers) >= recordLimit || reserveManaged("browser", managedBrowserCount(doc.Browsers)) != nil {
			return fmt.Errorf("%w: two browser slots are occupied", core.ErrCapacity)
		}
		record.State = initialState(record.Ownership)
		doc.Browsers = append(doc.Browsers, record)
		doc.Revision++
		stampBrowser(&doc.Browsers[len(doc.Browsers)-1], record.Owner, doc.Revision)
		outcome = browserOutcome(record, expected, doc.Revision)
		return nil
	})
	return outcome, err
}

func (r *registry) MarkStarted(ctx context.Context, id string, expected uint64, trace, external string) (ReserveOutcome, error) {
	if id == "" || trace == "" || external == "" {
		return ReserveOutcome{}, fmt.Errorf("%w: id, trace evidence, and external reference are required", core.ErrRevision)
	}
	var outcome ReserveOutcome
	err := r.withDocument(ctx, func(doc *registryDocument) error {
		if err := checkRevision(expected, doc.Revision); err != nil {
			return err
		}
		if record := findServer(doc.Servers, id); record != nil {
			if record.Ownership != Managed || record.State != "reserved" {
				return fmt.Errorf("%w: server is not a startable managed reservation", core.ErrTransition)
			}
			record.State, record.TraceEvidence, record.ExternalRef, record.StartedAt = "started", trace, external, now()
			doc.Revision++
			stampServer(record, record.Owner, doc.Revision)
			outcome = reserveOutcome(*record, expected, doc.Revision)
			return nil
		}
		if record := findBrowser(doc.Browsers, id); record != nil {
			if record.Ownership != Managed || record.State != "reserved" {
				return fmt.Errorf("%w: browser is not a startable managed reservation", core.ErrTransition)
			}
			record.State, record.TraceEvidence, record.ExternalRef, record.StartedAt = "started", trace, external, now()
			doc.Revision++
			stampBrowser(record, record.Owner, doc.Revision)
			outcome = browserOutcome(*record, expected, doc.Revision)
			return nil
		}
		return fmt.Errorf("%w: resource %q", core.ErrRevision, id)
	})
	return outcome, err
}

func (r *registry) MarkUnknown(ctx context.Context, id string, expected uint64, reason string) (ReleaseOutcome, error) {
	if id == "" || reason == "" {
		return ReleaseOutcome{}, fmt.Errorf("%w: id and reason are required", core.ErrRevision)
	}
	var outcome ReleaseOutcome
	err := r.withDocument(ctx, func(doc *registryDocument) error {
		if err := checkRevision(expected, doc.Revision); err != nil {
			return err
		}
		if record := findServer(doc.Servers, id); record != nil {
			record.Ownership, record.State, record.TraceEvidence = Unknown, "unknown", reason
			doc.Revision++
			stampServer(record, record.Owner, doc.Revision)
			outcome = releaseServer(*record, expected, doc.Revision, false, "")
			return nil
		}
		if record := findBrowser(doc.Browsers, id); record != nil {
			record.Ownership, record.State, record.TraceEvidence = Unknown, "unknown", reason
			doc.Revision++
			stampBrowser(record, record.Owner, doc.Revision)
			outcome = releaseBrowser(*record, expected, doc.Revision, false, "")
			return nil
		}
		return fmt.Errorf("%w: resource %q", core.ErrRevision, id)
	})
	return outcome, err
}

func (r *registry) Release(ctx context.Context, id string, expected uint64) (ReleaseOutcome, error) {
	var outcome ReleaseOutcome
	err := r.withDocument(ctx, func(doc *registryDocument) error {
		if err := checkRevision(expected, doc.Revision); err != nil {
			return err
		}
		if record := findServer(doc.Servers, id); record != nil {
			return releaseManagedServer(doc, record, expected, &outcome)
		}
		if record := findBrowser(doc.Browsers, id); record != nil {
			return releaseManagedBrowser(doc, record, expected, &outcome)
		}
		return fmt.Errorf("%w: resource %q", core.ErrRevision, id)
	})
	return outcome, err
}

func (r *registry) StopManaged(ctx context.Context, id string, expected uint64) (ReleaseOutcome, error) {
	if r.stopper == nil {
		return ReleaseOutcome{}, fmt.Errorf("%w: nil resource stopper", core.ErrSettings)
	}
	intent, request, outcome, err := r.claimStop(ctx, id, expected)
	if err != nil || outcome.Released {
		return outcome, err
	}
	stopCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	result := r.stopper(stopCtx, request)
	return r.finishStop(ctx, intent, result)
}

func (r *registry) claimStop(ctx context.Context, id string, expected uint64) (stopIntent, StopRequest, ReleaseOutcome, error) {
	var intent stopIntent
	var request StopRequest
	var outcome ReleaseOutcome
	err := r.withDocument(ctx, func(doc *registryDocument) error {
		if err := checkRevision(expected, doc.Revision); err != nil {
			return err
		}
		if record := findServer(doc.Servers, id); record != nil {
			outcome = releaseServer(*record, expected, doc.Revision, false, "")
			if record.State == "stopped" {
				outcome.Released = true
				return nil
			}
			if record.Ownership != Managed || record.State != "started" || record.ExternalRef == "" || record.TraceEvidence == "" {
				return fmt.Errorf("%w: resource %q is not authorized for stop", core.ErrRevision, id)
			}
			record.State, record.StopAttempt = "stopping", fmt.Sprintf("%s-%d", id, doc.Revision+1)
			doc.Revision++
			stampServer(record, record.Owner, doc.Revision)
			intent = stopIntent{"server", id, record.StopAttempt, expected}
			copy := *record
			request = StopRequest{Kind: "server", Server: &copy}
			outcome = releaseServer(*record, expected, doc.Revision, false, "")
			return nil
		}
		if record := findBrowser(doc.Browsers, id); record != nil {
			outcome = releaseBrowser(*record, expected, doc.Revision, false, "")
			if record.State == "stopped" {
				outcome.Released = true
				return nil
			}
			if record.Ownership != Managed || record.State != "started" || record.ExternalRef == "" || record.TraceEvidence == "" {
				return fmt.Errorf("%w: resource %q is not authorized for stop", core.ErrRevision, id)
			}
			record.State, record.StopAttempt = "stopping", fmt.Sprintf("%s-%d", id, doc.Revision+1)
			doc.Revision++
			stampBrowser(record, record.Owner, doc.Revision)
			intent = stopIntent{"browser", id, record.StopAttempt, expected}
			copy := *record
			request = StopRequest{Kind: "browser", Browser: &copy}
			outcome = releaseBrowser(*record, expected, doc.Revision, false, "")
			return nil
		}
		return fmt.Errorf("%w: resource %q", core.ErrRevision, id)
	})
	return intent, request, outcome, err
}

func (r *registry) finishStop(ctx context.Context, intent stopIntent, result tracker.CommandResult) (ReleaseOutcome, error) {
	var outcome ReleaseOutcome
	var operationErr error
	err := r.withDocument(ctx, func(doc *registryDocument) error {
		if intent.kind == "server" {
			if record := findServer(doc.Servers, intent.id); record != nil && record.State == "stopping" && record.StopAttempt == intent.token {
				operationErr = finishServer(doc, record, result, intent.expected, &outcome)
				return nil
			}
		}
		if intent.kind == "browser" {
			if record := findBrowser(doc.Browsers, intent.id); record != nil && record.State == "stopping" && record.StopAttempt == intent.token {
				operationErr = finishBrowser(doc, record, result, intent.expected, &outcome)
				return nil
			}
		}
		return fmt.Errorf("%w: stop attempt changed", core.ErrRevision)
	})
	if err != nil {
		return outcome, err
	}
	return outcome, operationErr
}

func stopEvidence(result tracker.CommandResult) string {
	if result.TimedOut {
		return "stop:timeout"
	}
	if result.Transport != nil {
		return "stop:transport"
	}
	return fmt.Sprintf("stop:exit=%d", result.Exit)
}
func finishServer(doc *registryDocument, record *ServerRecord, result tracker.CommandResult, expected uint64, outcome *ReleaseOutcome) error {
	record.TraceEvidence = stopEvidence(result)
	if result.Transport != nil || result.TimedOut || result.Exit != 0 {
		record.Ownership, record.State = Unknown, "unknown"
	} else {
		record.State = "stopped"
	}
	doc.Revision++
	stampServer(record, record.Owner, doc.Revision)
	*outcome = releaseServer(*record, expected, doc.Revision, record.State == "stopped", "")
	if record.State != "stopped" {
		return fmt.Errorf("%w: managed resource stop failed", core.ErrTransition)
	}
	return nil
}
func finishBrowser(doc *registryDocument, record *BrowserRecord, result tracker.CommandResult, expected uint64, outcome *ReleaseOutcome) error {
	record.TraceEvidence = stopEvidence(result)
	if result.Transport != nil || result.TimedOut || result.Exit != 0 {
		record.Ownership, record.State = Unknown, "unknown"
	} else {
		record.State = "stopped"
	}
	doc.Revision++
	stampBrowser(record, record.Owner, doc.Revision)
	*outcome = releaseBrowser(*record, expected, doc.Revision, record.State == "stopped", "")
	if record.State != "stopped" {
		return fmt.Errorf("%w: managed resource stop failed", core.ErrTransition)
	}
	return nil
}

func (r *registry) read() (registryDocument, error) {
	if r == nil || r.store == nil {
		return registryDocument{}, fmt.Errorf("%w: nil resource store", core.ErrSettings)
	}
	var doc registryDocument
	err := r.store.ReadJSON(registryPath, registryMax, &doc)
	if errors.Is(err, os.ErrNotExist) {
		return registryDocument{Schema: 1}, nil
	}
	if err != nil {
		return registryDocument{}, fmt.Errorf("%w: registry read: %v", core.ErrRevision, err)
	}
	if doc.Schema != 1 || len(doc.Servers) > recordLimit || len(doc.Browsers) > recordLimit {
		return registryDocument{}, fmt.Errorf("%w: invalid registry", core.ErrRevision)
	}
	return doc, nil
}

func (r *registry) withDocument(ctx context.Context, action func(*registryDocument) error) error {
	if r == nil || r.store == nil {
		return fmt.Errorf("%w: nil resource store", core.ErrSettings)
	}
	if err := acquireLock(ctx, r.store.Root); err != nil {
		return err
	}
	defer func() { _ = os.Remove(filepath.Join(r.store.Root, filepath.FromSlash(registryLock))) }()

	// The re-read is intentional: another process can have committed while this
	// caller waited on the durable lock. Never decide from a pre-contention view.
	doc, err := r.read()
	if err != nil {
		return err
	}
	before := doc.Revision
	if err := action(&doc); err != nil {
		return err
	}
	if doc.Revision == before {
		return nil
	}
	if _, err := r.store.WriteJSON(registryPath, doc, registryMax); err != nil {
		return fmt.Errorf("%w: registry write: %v", core.ErrRevision, err)
	}
	return nil
}

func acquireLock(ctx context.Context, root string) error {
	if root == "" {
		return fmt.Errorf("%w: empty resource store root", core.ErrPath)
	}
	parent := filepath.Join(root, ".agent-team", "resources")
	if err := os.MkdirAll(parent, 0o700); err != nil {
		return fmt.Errorf("%w: registry lock parent: %v", core.ErrPath, err)
	}
	lock := filepath.Join(root, filepath.FromSlash(registryLock))
	for {
		err := os.Mkdir(lock, 0o700)
		if err == nil {
			return nil
		}
		if !errors.Is(err, os.ErrExist) {
			return fmt.Errorf("%w: registry lock: %v", core.ErrPath, err)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Millisecond):
		}
	}
}

func checkReserveRevision(expected, actual uint64) error {
	if expected != actual {
		return fmt.Errorf("%w: expected registry revision %d, current %d", core.ErrRevision, expected, actual)
	}
	return nil
}

func checkRevision(expected, actual uint64) error {
	if expected != actual {
		return fmt.Errorf("%w: expected registry revision %d, current %d", core.ErrRevision, expected, actual)
	}
	return nil
}

func initialState(ownership Ownership) string {
	switch ownership {
	case Managed:
		return "reserved"
	case Unknown:
		return "unknown"
	default:
		return "observed"
	}
}

func reserveManaged(kind string, used int) error {
	return Reserve([]Capacity{{Name: kind, Effective: 2}}, []Reservation{{Name: kind, Count: used + 1}})
}

func terminal(state string) bool { return state == "released" || state == "stopped" }

func cleanupTerminals(doc *registryDocument) {
	doc.Servers = keepServers(doc.Servers)
	doc.Browsers = keepBrowsers(doc.Browsers)
}

func keepServers(records []ServerRecord) []ServerRecord {
	kept := records[:0]
	for _, record := range records {
		if !terminal(record.State) {
			kept = append(kept, record)
		}
	}
	return kept
}

func keepBrowsers(records []BrowserRecord) []BrowserRecord {
	kept := records[:0]
	for _, record := range records {
		if !terminal(record.State) {
			kept = append(kept, record)
		}
	}
	return kept
}

func stampServer(record *ServerRecord, owner ResourceOwner, revision uint64) {
	if record.Schema == 0 {
		record.Schema = 1
	}
	if record.Project == "" {
		record.Project = filepath.Base(owner.Worktree)
	}
	if record.RunID == "" {
		record.RunID = owner.Run
	}
	record.WrittenAt, record.Revision = now(), revision
}

func stampBrowser(record *BrowserRecord, owner ResourceOwner, revision uint64) {
	if record.Schema == 0 {
		record.Schema = 1
	}
	if record.Project == "" {
		record.Project = filepath.Base(owner.Worktree)
	}
	if record.RunID == "" {
		record.RunID = owner.Run
	}
	record.WrittenAt, record.Revision = now(), revision
}

func now() string { return time.Now().UTC().Format(time.RFC3339Nano) }

func validateOwner(owner ResourceOwner) error {
	if owner.Run == "" || owner.Team == "" || owner.Task == "" || invalidText(owner.Worktree) || invalidText(owner.Revision) {
		return fmt.Errorf("%w: complete typed resource ownership is required", core.ErrRevision)
	}
	return nil
}

func validateText(value string) error {
	if invalidText(value) {
		return fmt.Errorf("%w: invalid resource value", core.ErrPath)
	}
	return nil
}

func invalidText(value string) bool {
	return value == "" || len(value) > 1024 || strings.ContainsAny(value, "\x00\r\n")
}
