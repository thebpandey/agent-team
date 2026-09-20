package migrate

import (
	"bufio"
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/thebpandey/agent-team/vnext/internal/core"
	"github.com/thebpandey/agent-team/vnext/internal/release"
	"github.com/thebpandey/agent-team/vnext/internal/store"
)

const (
	authorityLimit       = 16 << 20
	authorityReceiptPath = ".agent-team/v8/authority.json"
	authorityJournalPath = ".agent-team/v8/cutover-journal.json"
)

var authorityMutationHook func(int) bool
var authorityRemoteObserver = observeGitRemote

type EvidenceReference struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

type TrackerAuthority struct {
	Export      string   `json:"export"`
	SHA256      string   `json:"sha256"`
	Fingerprint string   `json:"fingerprint"`
	ParentID    string   `json:"parentId"`
	TaskIDs     []string `json:"taskIds"`
	TaskCount   int      `json:"taskCount"`
}

type OwnedLegacyFile struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Retire bool   `json:"retire,omitempty"`
}

type RemoteObservation struct {
	Name           string `json:"name"`
	BaseRef        string `json:"baseRef"`
	BaseRevision   string `json:"baseRevision"`
	TargetRef      string `json:"targetRef,omitempty"`
	TargetRevision string `json:"targetRevision,omitempty"`
	ObservedAt     string `json:"observedAt"`
	TargetAbsent   bool   `json:"targetAbsent"`
}

type CutoverAuthorization struct {
	GrantedBy         string   `json:"grantedBy"`
	Source            string   `json:"source"`
	Cause             string   `json:"cause"`
	Scope             string   `json:"scope"`
	GrantedAt         string   `json:"grantedAt"`
	Revision          string   `json:"revision"`
	TaskIDs           []string `json:"taskIds"`
	RemoteMainDeploys bool     `json:"remoteMainDeploys"`
}

type AuthorityRequest struct {
	Schema                int                 `json:"schema"`
	Action                string              `json:"action"`
	Project               string              `json:"project"`
	OperationID           string              `json:"operationId"`
	TargetRevision        string              `json:"targetRevision"`
	Tracker               TrackerAuthority    `json:"tracker"`
	Reviews               []EvidenceReference `json:"reviews"`
	Tests                 []EvidenceReference `json:"tests"`
	Readiness             EvidenceReference   `json:"readiness"`
	Legacy                []OwnedLegacyFile   `json:"legacy"`
	ApprovalOperationID   string              `json:"approvalOperationId,omitempty"`
	Approval              EvidenceReference   `json:"approval,omitempty"`
	ExpectedReceiptDigest string              `json:"expectedReceiptDigest,omitempty"`
}

type authorityArchive struct {
	Schema        int                 `json:"schema"`
	Project       string              `json:"project"`
	OperationID   string              `json:"operationId"`
	RequestDigest string              `json:"requestDigest"`
	Files         []authorityPreimage `json:"files"`
}

type authorityPreimage struct {
	Path   string `json:"path"`
	Mode   uint32 `json:"mode"`
	SHA256 string `json:"sha256"`
	Bytes  []byte `json:"bytes"`
}

type AuthorityReceipt struct {
	Schema              int                  `json:"schema"`
	Project             string               `json:"project"`
	OperationID         string               `json:"operationId"`
	RequestDigest       string               `json:"requestDigest"`
	TargetRevision      string               `json:"targetRevision"`
	Tracker             TrackerAuthority     `json:"tracker"`
	Reviews             []EvidenceReference  `json:"reviews"`
	Tests               []EvidenceReference  `json:"tests"`
	Readiness           EvidenceReference    `json:"readiness"`
	Remote              RemoteObservation    `json:"remote"`
	Authorization       CutoverAuthorization `json:"authorization"`
	RecoveryDisposition string               `json:"recoveryDisposition"`
	ApprovalOperationID string               `json:"approvalOperationId"`
	ApprovalSHA256      string               `json:"approvalSha256"`
	ArchivePath         string               `json:"archivePath"`
	ArchiveSHA256       string               `json:"archiveSha256"`
	Held                bool                 `json:"held"`
	HoldCause           string               `json:"holdCause,omitempty"`
	WrittenAt           string               `json:"writtenAt"`
	ReceiptDigest       string               `json:"receiptDigest"`
	Mutated             []string             `json:"mutated"`
}

type trustedAuthority struct {
	ID          string
	Remote      RemoteObservation
	Approval    CutoverAuthorization
	Recovery    string
	EvidenceSHA string
}

type trustedEvidencePointer struct {
	Path, Fingerprint, Revision, OperationID, ObservedAt string
	TaskIDs                                              []string
}

type trustedLegacyState struct {
	Integration struct {
		OwnerSessionID   string                 `json:"ownerSessionId"`
		RecordedEvidence trustedEvidencePointer `json:"recordedEvidence"`
	} `json:"integration"`
	OperationReceipts map[string]struct {
		Signature string `json:"signature"`
		Result    struct {
			Gate, TrackerFingerprint, Revision string
		} `json:"result"`
		AppliedAt string `json:"appliedAt"`
	} `json:"operationReceipts"`
}

type trustedIntegrationEvidence struct {
	Status, Revision string
	TaskIDs          []string `json:"taskIds"`
	Remote           struct {
		Name, BaseRef, Revision, TargetRef, TargetRevision string
		TargetAbsent                                       bool `json:"targetAbsent"`
	} `json:"remote"`
	Authorization struct {
		Source, Scope, OwnerSessionID, Revision string
		TaskIDs                                 []string `json:"taskIds"`
	} `json:"authorization"`
	Recovery struct {
		Status, Revision, Action string
		TaskIDs                  []string `json:"taskIds"`
	} `json:"recovery"`
	RemoteMainDeploys bool `json:"remoteMainDeploys"`
}

type signedApprovalTrust struct {
	Algorithm string `json:"algorithm"`
	KeyID     string `json:"keyId"`
	PublicKey string `json:"publicKey"`
}

type signedCutoverApproval struct {
	Schema              int               `json:"schema"`
	ID                  string            `json:"id"`
	IssuedAt            string            `json:"issuedAt"`
	ExpiresAt           string            `json:"expiresAt"`
	TargetRevision      string            `json:"targetRevision"`
	TrackerFingerprint  string            `json:"trackerFingerprint"`
	ParentID            string            `json:"parentId"`
	Cause               string            `json:"cause"`
	RecoveryDisposition string            `json:"recoveryDisposition"`
	SignerKeyID         string            `json:"signerKeyId"`
	Signature           string            `json:"signature"`
	TaskIDs             []string          `json:"taskIds"`
	Remote              RemoteObservation `json:"remote"`
	RemoteMainDeploys   bool              `json:"remoteMainDeploys"`
}

type AuthorityResult struct {
	Action, ReceiptDigest, TargetRevision string
	Held, Idempotent                      bool
}

type authorityJournal struct {
	Schema        int              `json:"schema"`
	OperationID   string           `json:"operationId"`
	RequestDigest string           `json:"requestDigest"`
	Archive       authorityArchive `json:"archive"`
	Created       []string         `json:"created"`
	Restore       []string         `json:"restore"`
	Progress      int              `json:"progress"`
	ReceiptSHA256 string           `json:"receiptSha256,omitempty"`
}

func ExecuteAuthorityRequest(ctx context.Context, requestPath string) (AuthorityResult, error) {
	request, requestDigest, err := readAuthorityRequest(requestPath)
	if err != nil {
		return AuthorityResult{}, err
	}
	if request.Action == "status" {
		receipt, statusErr := AuthorityStatus(request.Project)
		if statusErr != nil {
			return AuthorityResult{}, statusErr
		}
		return AuthorityResult{Action: "status", ReceiptDigest: receipt.ReceiptDigest, TargetRevision: receipt.TargetRevision, Held: receipt.Held}, nil
	}
	guard, err := store.AcquireProjectMutation(ctx, request.Project, "authority-cutover", request.OperationID+":"+request.Action)
	if err != nil {
		return AuthorityResult{}, err
	}
	defer func() { _ = guard.Release() }()
	if err := recoverAuthorityJournal(request.Project); err != nil {
		return AuthorityResult{}, err
	}
	switch request.Action {
	case "cutover":
		return cutoverAuthority(ctx, request, requestDigest)
	case "reconcile":
		return reconcileAuthority(ctx, request)
	case "rollback":
		return rollbackAuthority(request)
	default:
		return AuthorityResult{}, fmt.Errorf("%w: unsupported cutover action", core.ErrPhase)
	}
}

func AuthorityStatus(project string) (AuthorityReceipt, error) {
	project, err := cleanProject(project)
	if err != nil {
		return AuthorityReceipt{}, err
	}
	var receipt AuthorityReceipt
	if err := store.New(project, core.StorageLimits{CanonicalBytes: authorityLimit}).ReadJSON(authorityReceiptPath, authorityLimit, &receipt); err != nil {
		return AuthorityReceipt{}, err
	}
	if err := validateAuthorityReceipt(receipt); err != nil {
		return AuthorityReceipt{}, err
	}
	return receipt, nil
}

func cutoverAuthority(ctx context.Context, request AuthorityRequest, requestDigest string) (AuthorityResult, error) {
	if existing, err := AuthorityStatus(request.Project); err == nil {
		if existing.RequestDigest == requestDigest {
			return AuthorityResult{Action: "cutover", ReceiptDigest: existing.ReceiptDigest, TargetRevision: existing.TargetRevision, Held: existing.Held, Idempotent: true}, nil
		}
		return AuthorityResult{}, fmt.Errorf("%w: another authority cutover is active", core.ErrRevision)
	} else if !errors.Is(err, os.ErrNotExist) {
		return AuthorityResult{}, err
	}
	trusted, err := validateAuthorityRequest(ctx, request)
	if err != nil {
		return AuthorityResult{}, err
	}
	archive, err := snapshotLegacy(request, requestDigest)
	if err != nil {
		return AuthorityResult{}, err
	}
	archiveRelative := filepath.ToSlash(filepath.Join(".agent-team", "v8", "archive", requestDigest+".json"))
	archiveRaw, err := json.Marshal(archive)
	if err != nil || len(archiveRaw) > authorityLimit {
		return AuthorityResult{}, core.ErrLimit
	}
	archiveSum := sha256.Sum256(append(archiveRaw, '\n'))
	mutated := []string{".agent-team/setup.json", ".agent-team/state.json"}
	for _, legacy := range request.Legacy {
		if legacy.Retire && legacy.Path != mutated[0] && legacy.Path != mutated[1] {
			mutated = append(mutated, legacy.Path)
		}
	}
	receipt := AuthorityReceipt{Schema: 1, Project: request.Project, OperationID: request.OperationID, RequestDigest: requestDigest, TargetRevision: request.TargetRevision, Tracker: request.Tracker, Reviews: request.Reviews, Tests: request.Tests, Readiness: request.Readiness, Remote: trusted.Remote, Authorization: trusted.Approval, RecoveryDisposition: trusted.Recovery, ApprovalOperationID: trusted.ID, ApprovalSHA256: trusted.EvidenceSHA, ArchivePath: archiveRelative, ArchiveSHA256: hex.EncodeToString(archiveSum[:]), Held: true, HoldCause: trusted.Approval.Cause, WrittenAt: time.Now().UTC().Format(time.RFC3339Nano), Mutated: mutated}
	receipt.ReceiptDigest = digestReceipt(receipt)
	receiptSHA256 := jsonStoredDigest(receipt)
	journal := authorityJournal{Schema: 1, OperationID: request.OperationID, RequestDigest: requestDigest, Archive: archive, Created: []string{archiveRelative, authorityReceiptPath}, Restore: mutated, ReceiptSHA256: receiptSHA256}
	state := store.New(request.Project, core.StorageLimits{CanonicalBytes: authorityLimit})
	if _, err := state.WriteJSON(authorityJournalPath, journal, authorityLimit); err != nil {
		return AuthorityResult{}, err
	}
	fail := func(cause error) (AuthorityResult, error) {
		if rollbackErr := restoreAuthorityArchive(request.Project, journal); rollbackErr != nil {
			return AuthorityResult{}, fmt.Errorf("%v; authority rollback: %w", cause, rollbackErr)
		}
		return AuthorityResult{}, cause
	}
	if _, err := state.CreateJSON(archiveRelative, archive, authorityLimit); err != nil {
		return fail(err)
	}
	setup := map[string]any{"schema": 1, "authority": "v8", "project": request.Project, "revision": request.TargetRevision, "tracker": map[string]any{"kind": "beads", "fingerprint": request.Tracker.Fingerprint, "parentId": request.Tracker.ParentID, "taskIds": request.Tracker.TaskIDs}, "receiptPath": authorityReceiptPath, "receiptDigest": receipt.ReceiptDigest}
	current := map[string]any{"schema": 1, "revision": request.TargetRevision, "integration": map[string]any{"authorized": false, "hold": true, "cause": trusted.Approval.Cause}, "release": map[string]any{"authorized": false, "hold": true, "cause": trusted.Approval.Cause}}
	for index, mutation := range []struct {
		path  string
		value any
	}{{".agent-team/setup.json", setup}, {".agent-team/state.json", current}} {
		if _, err := state.WriteJSON(mutation.path, mutation.value, authorityLimit); err != nil {
			return fail(err)
		}
		journal.Progress = index + 1
		if _, err := state.WriteJSON(authorityJournalPath, journal, authorityLimit); err != nil {
			return fail(err)
		}
		if authorityMutationHook != nil && authorityMutationHook(index) {
			return AuthorityResult{}, core.ErrTransition
		}
	}
	for _, legacy := range request.Legacy {
		if legacy.Retire && legacy.Path != ".agent-team/setup.json" && legacy.Path != ".agent-team/state.json" {
			if err := removeVerified(request.Project, legacy.Path, legacy.SHA256); err != nil {
				return fail(err)
			}
		}
	}
	observed, err := authorityRemoteObserver(ctx, request.Project, trusted.Remote.Name, trusted.Remote.BaseRef, trusted.Remote.TargetRef)
	if err != nil || !sameRemote(observed, trusted.Remote) || digestAbsolute(trusted.Approval.Source) != trusted.EvidenceSHA {
		return fail(fmt.Errorf("%w: approval or configured remote changed", core.ErrRevision))
	}
	if _, err := state.CreateJSON(authorityReceiptPath, receipt, authorityLimit); err != nil {
		return fail(err)
	}
	if err := removeAuthorityJournal(request.Project, journal); err != nil {
		return AuthorityResult{}, err
	}
	return AuthorityResult{Action: "cutover", ReceiptDigest: receipt.ReceiptDigest, TargetRevision: receipt.TargetRevision, Held: true}, nil
}

func reconcileAuthority(ctx context.Context, request AuthorityRequest) (AuthorityResult, error) {
	receipt, err := AuthorityStatus(request.Project)
	if err != nil {
		return AuthorityResult{}, err
	}
	if !receipt.Held {
		return AuthorityResult{Action: "reconcile", ReceiptDigest: receipt.ReceiptDigest, TargetRevision: receipt.TargetRevision, Idempotent: true}, nil
	}
	if request.ExpectedReceiptDigest == "" || request.ExpectedReceiptDigest != receipt.ReceiptDigest || request.TargetRevision != receipt.TargetRevision || !requestMatchesApproval(request, receipt) {
		return AuthorityResult{}, fmt.Errorf("%w: hold reconciliation does not match recorded authority", core.ErrRevision)
	}
	if err := revalidateEvidence(receipt); err != nil {
		return AuthorityResult{}, err
	}
	observed, err := authorityRemoteObserver(ctx, request.Project, receipt.Remote.Name, receipt.Remote.BaseRef, receipt.Remote.TargetRef)
	if err != nil || !sameRemote(observed, receipt.Remote) {
		return AuthorityResult{}, fmt.Errorf("%w: configured remote changed or unavailable", core.ErrRevision)
	}
	receipt.Held, receipt.HoldCause = false, ""
	receipt.WrittenAt = time.Now().UTC().Format(time.RFC3339Nano)
	receipt.ReceiptDigest = digestReceipt(receipt)
	state := store.New(request.Project, core.StorageLimits{CanonicalBytes: authorityLimit})
	archive, err := snapshotAuthorityPaths(request.Project, request.OperationID, receipt.RequestDigest, []string{authorityReceiptPath, ".agent-team/setup.json", ".agent-team/state.json"})
	if err != nil {
		return AuthorityResult{}, err
	}
	journal := authorityJournal{Schema: 1, OperationID: request.OperationID, RequestDigest: receipt.RequestDigest, Archive: archive, Restore: []string{authorityReceiptPath, ".agent-team/setup.json", ".agent-team/state.json"}}
	if _, err := state.WriteJSON(authorityJournalPath, journal, authorityLimit); err != nil {
		return AuthorityResult{}, err
	}
	fail := func(cause error) (AuthorityResult, error) {
		if rollbackErr := restoreAuthorityArchive(request.Project, journal); rollbackErr != nil {
			return AuthorityResult{}, fmt.Errorf("%v; authority reconciliation rollback: %w", cause, rollbackErr)
		}
		return AuthorityResult{}, cause
	}
	if _, err := state.WriteJSON(authorityReceiptPath, receipt, authorityLimit); err != nil {
		return fail(err)
	}
	journal.Progress = 1
	if _, err := state.WriteJSON(authorityJournalPath, journal, authorityLimit); err != nil {
		return fail(err)
	}
	if authorityMutationHook != nil && authorityMutationHook(0) {
		return AuthorityResult{}, core.ErrTransition
	}
	var setup map[string]any
	if err := state.ReadJSON(".agent-team/setup.json", authorityLimit, &setup); err != nil {
		return fail(err)
	}
	setup["receiptDigest"] = receipt.ReceiptDigest
	if _, err := state.WriteJSON(".agent-team/setup.json", setup, authorityLimit); err != nil {
		return fail(err)
	}
	journal.Progress = 2
	if _, err := state.WriteJSON(authorityJournalPath, journal, authorityLimit); err != nil {
		return fail(err)
	}
	if authorityMutationHook != nil && authorityMutationHook(1) {
		return AuthorityResult{}, core.ErrTransition
	}
	current := map[string]any{"schema": 1, "revision": receipt.TargetRevision, "integration": map[string]any{"authorized": true, "hold": false, "receiptDigest": receipt.ReceiptDigest}, "release": map[string]any{"authorized": true, "hold": false, "receiptDigest": receipt.ReceiptDigest}}
	if _, err := state.WriteJSON(".agent-team/state.json", current, authorityLimit); err != nil {
		return fail(err)
	}
	journal.Progress = 3
	if _, err := state.WriteJSON(authorityJournalPath, journal, authorityLimit); err != nil {
		return fail(err)
	}
	if authorityMutationHook != nil && authorityMutationHook(2) {
		return AuthorityResult{}, core.ErrTransition
	}
	if err := removeAuthorityJournal(request.Project, journal); err != nil {
		return AuthorityResult{}, err
	}
	return AuthorityResult{Action: "reconcile", ReceiptDigest: receipt.ReceiptDigest, TargetRevision: receipt.TargetRevision}, nil
}

func rollbackAuthority(request AuthorityRequest) (AuthorityResult, error) {
	receipt, err := AuthorityStatus(request.Project)
	if err != nil {
		return AuthorityResult{}, err
	}
	if request.ExpectedReceiptDigest != receipt.ReceiptDigest || request.TargetRevision != receipt.TargetRevision || !requestMatchesApproval(request, receipt) {
		return AuthorityResult{}, fmt.Errorf("%w: rollback authority mismatch", core.ErrRevision)
	}
	if digestFile(receipt.ArchivePath, request.Project) != receipt.ArchiveSHA256 {
		return AuthorityResult{}, fmt.Errorf("%w: cutover archive changed", core.ErrRevision)
	}
	var archive authorityArchive
	if err := store.New(request.Project, core.StorageLimits{CanonicalBytes: authorityLimit}).ReadJSON(receipt.ArchivePath, authorityLimit, &archive); err != nil {
		return AuthorityResult{}, err
	}
	journal := authorityJournal{Schema: 1, OperationID: request.OperationID, RequestDigest: receipt.RequestDigest, Archive: archive, Created: []string{authorityReceiptPath}, Restore: receipt.Mutated, ReceiptSHA256: jsonStoredDigest(receipt)}
	if _, err := store.New(request.Project, core.StorageLimits{CanonicalBytes: authorityLimit}).WriteJSON(authorityJournalPath, journal, authorityLimit); err != nil {
		return AuthorityResult{}, err
	}
	if err := restoreAuthorityArchive(request.Project, journal); err != nil {
		return AuthorityResult{}, err
	}
	return AuthorityResult{Action: "rollback", TargetRevision: receipt.TargetRevision}, nil
}

func requestMatchesApproval(request AuthorityRequest, receipt AuthorityReceipt) bool {
	return request.ApprovalOperationID != "" && request.ApprovalOperationID == receipt.ApprovalOperationID || request.Approval.Path == receipt.Authorization.Source && request.Approval.SHA256 == receipt.ApprovalSHA256
}

func validateAuthorityRequest(ctx context.Context, request AuthorityRequest) (trustedAuthority, error) {
	if ctx == nil || ctx.Err() != nil || request.Schema != 1 || request.Action != "cutover" || request.OperationID == "" || (request.ApprovalOperationID == "" && request.Approval.Path == "") {
		return trustedAuthority{}, core.ErrPhase
	}
	project, err := cleanProject(request.Project)
	if err != nil || project != request.Project || !validRevision(request.TargetRevision) {
		return trustedAuthority{}, core.ErrRevision
	}
	head, err := exec.CommandContext(ctx, "git", "-C", project, "rev-parse", "HEAD").Output()
	if err != nil || strings.TrimSpace(string(head)) != request.TargetRevision {
		return trustedAuthority{}, fmt.Errorf("%w: target git HEAD changed", core.ErrRevision)
	}
	if err := validateLegacyAuthority(request); err != nil {
		return trustedAuthority{}, err
	}
	if err := validateTracker(request.Tracker); err != nil {
		return trustedAuthority{}, err
	}
	trusted, err := loadTrustedAuthority(request)
	if err != nil {
		return trustedAuthority{}, err
	}
	observed, err := authorityRemoteObserver(ctx, project, trusted.Remote.Name, trusted.Remote.BaseRef, trusted.Remote.TargetRef)
	if err != nil || !sameRemote(observed, trusted.Remote) {
		return trustedAuthority{}, fmt.Errorf("%w: configured remote differs or is unavailable", core.ErrRevision)
	}
	for _, evidence := range append(append([]EvidenceReference(nil), request.Tests...), request.Readiness) {
		if err := verifyEvidenceReference(evidence); err != nil {
			return trustedAuthority{}, err
		}
	}
	if err := release.VerifyReadinessEvidence(request.Readiness.Path, request.TargetRevision); err != nil {
		return trustedAuthority{}, fmt.Errorf("%w: invalid readiness evidence", core.ErrPhase)
	}
	if len(request.Reviews) == 0 || len(request.Tests) == 0 {
		return trustedAuthority{}, core.ErrPhase
	}
	for _, review := range request.Reviews {
		if err := validateReview(review, request.TargetRevision); err != nil {
			return trustedAuthority{}, err
		}
	}
	return trusted, nil
}

func loadTrustedAuthority(request AuthorityRequest) (trustedAuthority, error) {
	if request.Approval.Path != "" {
		return loadSignedAuthority(request)
	}
	var state trustedLegacyState
	if err := store.New(request.Project, core.StorageLimits{CanonicalBytes: authorityLimit}).ReadJSON(".agent-team/state.json", authorityLimit, &state); err != nil {
		return trustedAuthority{}, fmt.Errorf("%w: trusted legacy state unavailable", core.ErrRevision)
	}
	pointer := state.Integration.RecordedEvidence
	operation, ok := state.OperationReceipts[request.ApprovalOperationID]
	if !ok || pointer.OperationID != request.ApprovalOperationID || !validDigest(operation.Signature) || operation.Result.Gate != "integration" || operation.Result.Revision != request.TargetRevision || operation.Result.TrackerFingerprint != request.Tracker.Fingerprint || pointer.Revision != request.TargetRevision || pointer.Fingerprint == "" || pointer.Path == "" {
		return trustedAuthority{}, fmt.Errorf("%w: approval is not anchored by canonical gate evidence", core.ErrRevision)
	}
	approvedTasks := append([]string(nil), pointer.TaskIDs...)
	trackerTasks := append([]string(nil), request.Tracker.TaskIDs...)
	sort.Strings(approvedTasks)
	sort.Strings(trackerTasks)
	if !equalStrings(approvedTasks, trackerTasks) {
		return trustedAuthority{}, fmt.Errorf("%w: approval scope differs", core.ErrRevision)
	}
	appliedAt, appliedErr := time.Parse(time.RFC3339Nano, operation.AppliedAt)
	observedAt, observedErr := time.Parse(time.RFC3339Nano, pointer.ObservedAt)
	now := time.Now().UTC()
	if appliedErr != nil || observedErr != nil || observedAt.Sub(appliedAt) > 5*time.Minute || appliedAt.Sub(observedAt) > 5*time.Minute || now.Before(appliedAt) || now.Sub(appliedAt) > 24*time.Hour {
		return trustedAuthority{}, fmt.Errorf("%w: approval is stale or malformed", core.ErrRevision)
	}
	raw, err := readAbsoluteBounded(pointer.Path)
	if err != nil || digestBytes(raw) != pointer.Fingerprint {
		return trustedAuthority{}, fmt.Errorf("%w: approval evidence changed", core.ErrRevision)
	}
	var evidence trustedIntegrationEvidence
	decoder := json.NewDecoder(bytes.NewReader(raw))
	if decoder.Decode(&evidence) != nil || evidence.Status != "passed" || evidence.Revision != request.TargetRevision || evidence.Authorization.Scope != "integration" || evidence.Authorization.Source == "" || evidence.Authorization.OwnerSessionID == "" || evidence.Authorization.OwnerSessionID != state.Integration.OwnerSessionID || evidence.Authorization.Revision != request.TargetRevision || evidence.Recovery.Status != "reconciled" || evidence.Recovery.Revision != request.TargetRevision || evidence.Recovery.Action == "" || !sameTasks(evidence.TaskIDs, trackerTasks) || !sameTasks(evidence.Authorization.TaskIDs, trackerTasks) || !sameTasks(evidence.Recovery.TaskIDs, trackerTasks) {
		return trustedAuthority{}, fmt.Errorf("%w: approval evidence is invalid", core.ErrRevision)
	}
	remote := RemoteObservation{Name: evidence.Remote.Name, BaseRef: evidence.Remote.BaseRef, BaseRevision: evidence.Remote.Revision, TargetRef: evidence.Remote.TargetRef, TargetRevision: evidence.Remote.TargetRevision, TargetAbsent: evidence.Remote.TargetAbsent, ObservedAt: pointer.ObservedAt}
	if remote.Name == "" || remote.BaseRef == "" || remote.TargetRef == "" || !validRevision(remote.BaseRevision) || remote.TargetAbsent == (remote.TargetRevision != "") || (!remote.TargetAbsent && !validRevision(remote.TargetRevision)) {
		return trustedAuthority{}, fmt.Errorf("%w: approval remote is invalid", core.ErrRevision)
	}
	approval := CutoverAuthorization{GrantedBy: state.Integration.OwnerSessionID, Source: pointer.Path, Cause: evidence.Authorization.Source, Scope: "integration", GrantedAt: operation.AppliedAt, Revision: request.TargetRevision, TaskIDs: trackerTasks, RemoteMainDeploys: evidence.RemoteMainDeploys}
	return trustedAuthority{ID: request.ApprovalOperationID, Remote: remote, Approval: approval, Recovery: evidence.Recovery.Action, EvidenceSHA: pointer.Fingerprint}, nil
}

func loadSignedAuthority(request AuthorityRequest) (trustedAuthority, error) {
	if request.ApprovalOperationID != "" || verifyEvidenceReference(request.Approval) != nil {
		return trustedAuthority{}, fmt.Errorf("%w: invalid signed approval reference", core.ErrRevision)
	}
	var setup struct {
		CutoverApproval signedApprovalTrust `json:"cutoverApproval"`
	}
	if err := store.New(request.Project, core.StorageLimits{CanonicalBytes: authorityLimit}).ReadJSON(".agent-team/setup.json", authorityLimit, &setup); err != nil {
		return trustedAuthority{}, fmt.Errorf("%w: pinned cutover approval key unavailable", core.ErrRevision)
	}
	publicKey, err := base64.StdEncoding.DecodeString(setup.CutoverApproval.PublicKey)
	if err != nil || setup.CutoverApproval.Algorithm != "ed25519" || len(publicKey) != ed25519.PublicKeySize || digestBytes(publicKey) != setup.CutoverApproval.KeyID {
		return trustedAuthority{}, fmt.Errorf("%w: invalid pinned cutover approval key", core.ErrRevision)
	}
	raw, err := readAbsoluteBounded(request.Approval.Path)
	if err != nil || digestBytes(raw) != request.Approval.SHA256 {
		return trustedAuthority{}, fmt.Errorf("%w: signed approval changed", core.ErrRevision)
	}
	var approval signedCutoverApproval
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&approval) != nil || approval.Schema != 1 || approval.ID == "" || approval.SignerKeyID != setup.CutoverApproval.KeyID {
		return trustedAuthority{}, fmt.Errorf("%w: malformed signed approval", core.ErrRevision)
	}
	signature, err := base64.StdEncoding.DecodeString(approval.Signature)
	approval.Signature = ""
	payload, marshalErr := json.Marshal(approval)
	if err != nil || marshalErr != nil || !ed25519.Verify(publicKey, payload, signature) {
		return trustedAuthority{}, fmt.Errorf("%w: signed approval verification failed", core.ErrRevision)
	}
	issued, issueErr := time.Parse(time.RFC3339Nano, approval.IssuedAt)
	expires, expiryErr := time.Parse(time.RFC3339Nano, approval.ExpiresAt)
	now := time.Now().UTC()
	if issueErr != nil || expiryErr != nil || now.Before(issued) || !now.Before(expires) || expires.Sub(issued) > 24*time.Hour {
		return trustedAuthority{}, fmt.Errorf("%w: signed approval is stale", core.ErrRevision)
	}
	if approval.TargetRevision != request.TargetRevision || approval.TrackerFingerprint != request.Tracker.Fingerprint || approval.ParentID != request.Tracker.ParentID || approval.Cause == "" || approval.RecoveryDisposition == "" || !sameTasks(approval.TaskIDs, request.Tracker.TaskIDs) || approval.Remote.Name == "" || approval.Remote.BaseRef == "" || approval.Remote.TargetRef == "" || !validRevision(approval.Remote.BaseRevision) || approval.Remote.TargetAbsent == (approval.Remote.TargetRevision != "") || (!approval.Remote.TargetAbsent && !validRevision(approval.Remote.TargetRevision)) {
		return trustedAuthority{}, fmt.Errorf("%w: signed approval scope differs", core.ErrRevision)
	}
	derived := CutoverAuthorization{GrantedBy: setup.CutoverApproval.KeyID, Source: request.Approval.Path, Cause: approval.Cause, Scope: approval.ParentID, GrantedAt: approval.IssuedAt, Revision: approval.TargetRevision, TaskIDs: append([]string(nil), approval.TaskIDs...), RemoteMainDeploys: approval.RemoteMainDeploys}
	return trustedAuthority{ID: approval.ID, Remote: approval.Remote, Approval: derived, Recovery: approval.RecoveryDisposition, EvidenceSHA: request.Approval.SHA256}, nil
}

func sameTasks(left, right []string) bool {
	a, b := append([]string(nil), left...), append([]string(nil), right...)
	sort.Strings(a)
	sort.Strings(b)
	return equalStrings(a, b)
}

func sameRemote(left, right RemoteObservation) bool {
	return left.Name == right.Name && left.BaseRef == right.BaseRef && left.BaseRevision == right.BaseRevision && left.TargetRef == right.TargetRef && left.TargetRevision == right.TargetRevision && left.TargetAbsent == right.TargetAbsent
}

func observeGitRemote(ctx context.Context, project, name, baseRef, targetRef string) (RemoteObservation, error) {
	if name == "" || strings.ContainsAny(name, "\r\n\x00") || baseRef == "" || targetRef == "" {
		return RemoteObservation{}, core.ErrRevision
	}
	if _, err := exec.CommandContext(ctx, "git", "-C", project, "remote", "get-url", name).Output(); err != nil {
		return RemoteObservation{}, err
	}
	output, err := exec.CommandContext(ctx, "git", "-C", project, "ls-remote", "--refs", name, baseRef, targetRef).Output()
	if err != nil {
		return RemoteObservation{}, err
	}
	refs := map[string]string{}
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 {
			refs[fields[1]] = fields[0]
		}
	}
	base := refs[baseRef]
	if !validRevision(base) {
		return RemoteObservation{}, core.ErrRevision
	}
	target, found := refs[targetRef]
	return RemoteObservation{Name: name, BaseRef: baseRef, BaseRevision: base, TargetRef: targetRef, TargetRevision: target, TargetAbsent: !found, ObservedAt: time.Now().UTC().Format(time.RFC3339Nano)}, nil
}

func validateLegacyAuthority(request AuthorityRequest) error {
	wanted := map[string]string{}
	for _, file := range request.Legacy {
		if !safeRelative(file.Path) || !validDigest(file.SHA256) || wanted[file.Path] != "" {
			return core.ErrPath
		}
		wanted[file.Path] = file.SHA256
		if digestFile(file.Path, request.Project) != file.SHA256 {
			return fmt.Errorf("%w: legacy ownership changed: %s", core.ErrRevision, file.Path)
		}
	}
	for _, required := range []string{".agent-team/setup.json", ".agent-team/state.json"} {
		if wanted[required] == "" {
			return fmt.Errorf("%w: unverified legacy authority", core.ErrRevision)
		}
		var value map[string]any
		if err := store.New(request.Project, core.StorageLimits{CanonicalBytes: authorityLimit}).ReadJSON(required, authorityLimit, &value); err != nil || value["schemaVersion"] != float64(1) {
			return fmt.Errorf("%w: malformed legacy authority", core.ErrRevision)
		}
		if required == ".agent-team/setup.json" {
			tracker, _ := value["tracker"].(map[string]any)
			if tracker["kind"] != "markdown" {
				return fmt.Errorf("%w: legacy tracker is not markdown", core.ErrRevision)
			}
		}
	}
	return nil
}

func validateTracker(tracker TrackerAuthority) error {
	if tracker.Export == "" || !filepath.IsAbs(tracker.Export) || tracker.SHA256 == "" || tracker.Fingerprint != tracker.SHA256 || tracker.ParentID == "" || tracker.TaskCount != len(tracker.TaskIDs) || tracker.TaskCount < 2 {
		return fmt.Errorf("%w: invalid tracker evidence", core.ErrRevision)
	}
	raw, err := readAbsoluteBounded(tracker.Export)
	if err != nil || digestBytes(raw) != tracker.SHA256 {
		return core.ErrPath
	}
	var tasks []struct {
		ID, Status, Parent, ParentID string
	}
	if json.Unmarshal(raw, &tasks) != nil {
		return fmt.Errorf("%w: malformed tracker export", core.ErrRevision)
	}
	want := append([]string(nil), tracker.TaskIDs...)
	sort.Strings(want)
	seen := make([]string, 0, len(tasks))
	parentSeen := false
	for _, task := range tasks {
		if strings.ToLower(task.Status) != "closed" || task.ID == "" {
			return fmt.Errorf("%w: tracker scope is not closed", core.ErrRevision)
		}
		if task.ID == tracker.ParentID {
			parentSeen = true
		} else if task.Parent != tracker.ParentID && task.ParentID != tracker.ParentID && !strings.HasPrefix(task.ID, tracker.ParentID+".") {
			return fmt.Errorf("%w: task outside tracker parent", core.ErrRevision)
		}
		seen = append(seen, task.ID)
	}
	sort.Strings(seen)
	if !parentSeen || len(seen) != tracker.TaskCount || !equalStrings(seen, want) {
		return fmt.Errorf("%w: tracker scope differs", core.ErrRevision)
	}
	return nil
}

func validateReview(reference EvidenceReference, revision string) error {
	if err := verifyEvidenceReference(reference); err != nil {
		return err
	}
	var review struct{ Phase, Revision, Author, Reviewer, Source, Digest, Result string }
	raw, err := readAbsoluteBounded(reference.Path)
	if err != nil || json.Unmarshal(raw, &review) != nil || review.Phase == "" || review.Revision != revision || review.Author == "" || review.Reviewer == "" || review.Author == review.Reviewer || review.Source == "" || review.Digest == "" || (review.Result != "CLEAN" && review.Result != "PASS") {
		return core.ErrPhase
	}
	source, err := readAbsoluteBounded(review.Source)
	if err != nil || digestBytes(source) != review.Digest || !passingGoTestJSON(source) {
		return core.ErrPhase
	}
	return nil
}

func passingGoTestJSON(raw []byte) bool {
	scanner := bufio.NewScanner(bytes.NewReader(raw))
	named, pkg := false, false
	for scanner.Scan() {
		var event struct{ Action, Package, Test string }
		if json.Unmarshal(scanner.Bytes(), &event) != nil || event.Package == "" || event.Action == "fail" {
			return false
		}
		if event.Action == "pass" && event.Test != "" {
			named = true
		}
		if event.Action == "pass" && event.Test == "" {
			pkg = true
		}
	}
	return scanner.Err() == nil && named && pkg
}

func snapshotLegacy(request AuthorityRequest, requestDigest string) (authorityArchive, error) {
	archive := authorityArchive{Schema: 1, Project: request.Project, OperationID: request.OperationID, RequestDigest: requestDigest}
	total := int64(0)
	wanted := map[string]string{}
	for _, owned := range request.Legacy {
		wanted[owned.Path] = owned.SHA256
	}
	err := filepath.WalkDir(filepath.Join(request.Project, ".agent-team"), func(full string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, relErr := filepath.Rel(request.Project, full)
		if relErr != nil {
			return relErr
		}
		rel = filepath.ToSlash(rel)
		if entry.IsDir() {
			if rel == ".agent-team/v8" {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 || !entry.Type().IsRegular() {
			return core.ErrPath
		}
		info, err := os.Lstat(full)
		if err != nil || !info.Mode().IsRegular() || info.Size() < 0 || total > authorityLimit-info.Size() {
			return core.ErrLimit
		}
		raw, mode, err := store.New(request.Project, core.StorageLimits{CanonicalBytes: authorityLimit}).ReadFile(rel, authorityLimit-total)
		digest := digestBytes(raw)
		if err != nil || int64(len(raw)) != info.Size() || (wanted[rel] != "" && digest != wanted[rel]) {
			return core.ErrRevision
		}
		total += info.Size()
		archive.Files = append(archive.Files, authorityPreimage{Path: rel, Mode: uint32(mode.Perm()), SHA256: digest, Bytes: raw})
		return nil
	})
	if err != nil {
		return authorityArchive{}, err
	}
	sort.Slice(archive.Files, func(i, j int) bool { return archive.Files[i].Path < archive.Files[j].Path })
	return archive, nil
}

func snapshotAuthorityPaths(project, operationID, requestDigest string, paths []string) (authorityArchive, error) {
	archive := authorityArchive{Schema: 1, Project: project, OperationID: operationID, RequestDigest: requestDigest}
	total := int64(0)
	for _, relative := range paths {
		if !safeRelative(relative) {
			return authorityArchive{}, core.ErrPath
		}
		full := filepath.Join(project, filepath.FromSlash(relative))
		info, err := os.Lstat(full)
		if err != nil || !info.Mode().IsRegular() || info.Size() < 0 || total > authorityLimit-info.Size() {
			return authorityArchive{}, core.ErrLimit
		}
		raw, mode, err := store.New(project, core.StorageLimits{CanonicalBytes: authorityLimit}).ReadFile(relative, authorityLimit-total)
		if err != nil || int64(len(raw)) != info.Size() {
			return authorityArchive{}, core.ErrRevision
		}
		total += info.Size()
		archive.Files = append(archive.Files, authorityPreimage{Path: relative, Mode: uint32(mode.Perm()), SHA256: digestBytes(raw), Bytes: raw})
	}
	return archive, nil
}

func recoverAuthorityJournal(project string) error {
	var journal authorityJournal
	err := store.New(project, core.StorageLimits{CanonicalBytes: authorityLimit}).ReadJSON(authorityJournalPath, authorityLimit, &journal)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil || journal.Schema != 1 || journal.Archive.Project != project {
		return fmt.Errorf("%w: invalid authority journal", core.ErrRevision)
	}
	return restoreAuthorityArchive(project, journal)
}

func restoreAuthorityArchive(project string, journal authorityJournal) error {
	state := store.New(project, core.StorageLimits{CanonicalBytes: authorityLimit})
	wanted := map[string]bool{}
	for _, path := range journal.Restore {
		wanted[path] = true
	}
	for _, preimage := range journal.Archive.Files {
		if !wanted[preimage.Path] {
			continue
		}
		if !safeRelative(preimage.Path) || digestBytes(preimage.Bytes) != preimage.SHA256 {
			return core.ErrRevision
		}
		if _, err := state.WriteMarkdown(preimage.Path, preimage.Bytes, authorityLimit); err != nil {
			return err
		}
		_ = os.Chmod(filepath.Join(project, filepath.FromSlash(preimage.Path)), os.FileMode(preimage.Mode))
	}
	for _, created := range journal.Created {
		var digest string
		switch created {
		case authorityReceiptPath:
			digest = journal.ReceiptSHA256
		case journalArchivePath(journal):
			digest = jsonStoredDigest(journal.Archive)
		}
		if digest != "" {
			if err := store.New(project, core.StorageLimits{CanonicalBytes: authorityLimit}).RemoveExact(created, digest, authorityLimit); err != nil {
				return err
			}
		}
	}
	return removeAuthorityJournal(project, journal)
}

func journalArchivePath(journal authorityJournal) string {
	return filepath.ToSlash(filepath.Join(".agent-team", "v8", "archive", journal.RequestDigest+".json"))
}

func revalidateEvidence(receipt AuthorityReceipt) error {
	if !validRevision(receipt.TargetRevision) || !validDigest(receipt.ApprovalSHA256) || digestAbsolute(receipt.Authorization.Source) != receipt.ApprovalSHA256 {
		return core.ErrRevision
	}
	head, err := exec.Command("git", "-C", receipt.Project, "rev-parse", "HEAD").Output()
	if err != nil || strings.TrimSpace(string(head)) != receipt.TargetRevision {
		return core.ErrRevision
	}
	if err := validateTracker(receipt.Tracker); err != nil {
		return err
	}
	for _, evidence := range append(append([]EvidenceReference(nil), receipt.Tests...), receipt.Readiness) {
		if err := verifyEvidenceReference(evidence); err != nil {
			return err
		}
	}
	if err := release.VerifyReadinessEvidence(receipt.Readiness.Path, receipt.TargetRevision); err != nil {
		return core.ErrPhase
	}
	for _, review := range receipt.Reviews {
		if err := validateReview(review, receipt.TargetRevision); err != nil {
			return err
		}
	}
	return nil
}

func readAuthorityRequest(path string) (AuthorityRequest, string, error) {
	if path == "" || !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return AuthorityRequest{}, "", core.ErrPath
	}
	raw, err := readAbsoluteBounded(path)
	if err != nil || len(raw) == 0 {
		return AuthorityRequest{}, "", core.ErrPath
	}
	var request AuthorityRequest
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&request) != nil || decoder.Decode(&struct{}{}) != io.EOF {
		return AuthorityRequest{}, "", core.ErrPhase
	}
	return request, digestBytes(raw), nil
}

func validateAuthorityReceipt(receipt AuthorityReceipt) error {
	if receipt.Schema != 1 || receipt.Project == "" || receipt.OperationID == "" || receipt.RequestDigest == "" || receipt.ApprovalOperationID == "" || !validDigest(receipt.ApprovalSHA256) || !validRevision(receipt.TargetRevision) || receipt.ReceiptDigest != digestReceipt(receipt) {
		return core.ErrRevision
	}
	return nil
}

func digestReceipt(receipt AuthorityReceipt) string {
	receipt.ReceiptDigest = ""
	raw, _ := json.Marshal(receipt)
	return digestBytes(raw)
}
func jsonStoredDigest(value any) string {
	raw, _ := json.Marshal(value)
	return digestBytes(append(raw, '\n'))
}
func digestBytes(raw []byte) string { sum := sha256.Sum256(raw); return hex.EncodeToString(sum[:]) }
func digestAbsolute(path string) string {
	raw, err := readAbsoluteBounded(path)
	if err != nil {
		return ""
	}
	return digestBytes(raw)
}
func readAbsoluteBounded(path string) ([]byte, error) {
	if path == "" || !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return nil, core.ErrPath
	}
	raw, _, err := store.New(filepath.Dir(path), core.StorageLimits{CanonicalBytes: authorityLimit}).ReadFile(filepath.Base(path), authorityLimit)
	return raw, err
}
func digestFile(relative, root string) string {
	if !safeRelative(relative) {
		return ""
	}
	raw, _, err := store.New(root, core.StorageLimits{CanonicalBytes: authorityLimit}).ReadFile(relative, authorityLimit)
	if err != nil {
		return ""
	}
	return digestBytes(raw)
}
func verifyEvidenceReference(ref EvidenceReference) error {
	if ref.Path == "" || !filepath.IsAbs(ref.Path) || !validDigest(ref.SHA256) || digestAbsolute(ref.Path) != ref.SHA256 {
		return core.ErrRevision
	}
	return nil
}
func validRevision(value string) bool {
	if len(value) != 40 && len(value) != 64 {
		return false
	}
	for _, c := range value {
		if !strings.ContainsRune("0123456789abcdefABCDEF", c) {
			return false
		}
	}
	return true
}
func safeRelative(value string) bool {
	clean := filepath.Clean(filepath.FromSlash(value))
	return value != "" && !filepath.IsAbs(value) && clean != "." && clean != ".." && !strings.HasPrefix(clean, ".."+string(filepath.Separator))
}
func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func removeVerified(project, relative, digest string) error {
	return store.New(project, core.StorageLimits{CanonicalBytes: authorityLimit}).RemoveExact(relative, digest, authorityLimit)
}

func removeAuthorityJournal(project string, journal authorityJournal) error {
	return store.New(project, core.StorageLimits{CanonicalBytes: authorityLimit}).RemoveExact(authorityJournalPath, jsonStoredDigest(journal), authorityLimit)
}
