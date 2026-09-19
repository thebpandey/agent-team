// Package workflow contains the deterministic, scoped lifecycle state
// machine and its bounded recovery checkpoint transaction.
package workflow

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"strings"
	"time"

	"github.com/thebpandey/agent-team/vnext/internal/core"
	"github.com/thebpandey/agent-team/vnext/internal/knowledge"
	"github.com/thebpandey/agent-team/vnext/internal/project"
	"github.com/thebpandey/agent-team/vnext/internal/run"
	"github.com/thebpandey/agent-team/vnext/internal/store"
)

type EventKind string

const (
	Pause           EventKind = "pause"
	Stop            EventKind = "stop"
	Cancel          EventKind = "cancel"
	Resume          EventKind = "resume"
	CheckpointEvent EventKind = "checkpoint"
)

type Event struct {
	Run              core.RunID     `json:"runId,omitempty"`
	Scope            core.Scope     `json:"scope"`
	Kind             EventKind      `json:"kind"`
	From             core.TaskState `json:"from,omitempty"`
	To               core.TaskState `json:"to,omitempty"`
	Reason           string         `json:"reason,omitempty"`
	CheckpointDigest string         `json:"checkpointDigest,omitempty"`
	Confirmed        bool           `json:"confirmed,omitempty"`
	AdmissionHeld    bool           `json:"admissionHeld,omitempty"`
	RefillHeld       bool           `json:"refillHeld,omitempty"`
}

// ReceiptProjection is the exact receipt identity and digest observed before
// and after the checkpoint transaction. It allows a committed checkpoint to
// repair an interrupted receipt projection without inventing authority.
type ReceiptProjection struct {
	Path           string `json:"path"`
	BeforeIdentity string `json:"beforeIdentity"`
	BeforeDigest   string `json:"beforeDigest"`
	AfterIdentity  string `json:"afterIdentity"`
	AfterDigest    string `json:"afterDigest"`
}

type CheckpointRecord struct {
	core.RecordEnvelope
	Scope         core.Scope          `json:"scope"`
	Digest        string              `json:"digest"`
	AdmissionHeld bool                `json:"admissionHeld"`
	RefillHeld    bool                `json:"refillHeld"`
	Receipts      []ReceiptProjection `json:"receipts"`
}

const (
	maxReceiptProjections = 8
	maxReceiptBytes       = 1 << 20
	maxReceiptTotalBytes  = 8 << 20
)

func transitionError(format string, args ...any) error {
	return fmt.Errorf("%w: %s", core.ErrTransition, fmt.Sprintf(format, args...))
}

func validScope(scope core.Scope) error {
	switch scope.Kind {
	case core.ScopeProject:
		canonical, err := project.Contain(scope.ID, scope.ID)
		if err != nil || canonical != scope.ID {
			return transitionError("invalid project scope ID")
		}
	case core.ScopeRun, core.ScopeTeam, core.ScopeTask:
	default:
		return transitionError("unknown scope kind %q", scope.Kind)
	}
	if scope.Kind != core.ScopeProject {
		if err := project.ValidateSegment(scope.ID); err != nil {
			return err
		}
	}
	return nil
}

func validCheckpointScope(scope core.Scope) error {
	if err := validScope(scope); err != nil {
		return err
	}
	return nil
}

func validRun(id core.RunID) error {
	if id == "" {
		return nil
	}
	if err := project.ValidateSegment(string(id)); err != nil {
		return transitionError("invalid run ID: %v", err)
	}
	return nil
}

func knownState(state core.TaskState) bool {
	switch state {
	case core.Ready, core.Idle, core.Working, core.Implementing, core.Reviewing,
		core.Fix, core.Clean, core.Gated, core.Integrated, core.Paused,
		core.Blocked, core.Interrupted, core.Cancelled, core.Archived:
		return true
	default:
		return false
	}
}

func validDigest(value string) bool {
	if !strings.HasPrefix(value, "sha256:") || len(value) != len("sha256:")+64 {
		return false
	}
	_, err := hex.DecodeString(strings.TrimPrefix(value, "sha256:"))
	return err == nil
}

func Transition(state core.TaskState, event Event) (core.TaskState, error) {
	if !knownState(state) {
		return "", transitionError("unknown current state %q", state)
	}
	if err := validRun(event.Run); err != nil {
		return "", err
	}
	if err := validScope(event.Scope); err != nil {
		return "", transitionError("invalid scope: %v", err)
	}
	if event.Scope.Kind == core.ScopeRun && event.Run != "" && event.Scope.ID != string(event.Run) {
		return "", transitionError("run scope %q does not match event run %q", event.Scope.ID, event.Run)
	}
	if event.From != "" && (!knownState(event.From) || event.From != state) {
		return "", transitionError("from state %q does not match current state %q", event.From, state)
	}
	if event.To != "" && !knownState(event.To) {
		return "", transitionError("unknown target state %q", event.To)
	}

	var next core.TaskState
	switch event.Kind {
	case Pause:
		if !event.AdmissionHeld || !event.RefillHeld {
			return "", transitionError("pause requires admission and refill holds")
		}
		if event.Run != "" && strings.TrimSpace(event.Reason) == "" {
			return "", transitionError("pause requires a reason")
		}
		if event.Scope.Kind == core.ScopeProject && !event.Confirmed {
			return "", transitionError("project pause requires confirmation")
		}
		if state == core.Clean || state == core.Integrated || state == core.Cancelled || state == core.Archived {
			return "", transitionError("state %q cannot be paused", state)
		}
		next = core.Paused
	case Stop:
		if !event.AdmissionHeld || !event.RefillHeld {
			return "", transitionError("stop requires admission and refill holds")
		}
		if strings.TrimSpace(event.Reason) == "" {
			return "", transitionError("stop requires a reason")
		}
		if event.Scope.Kind == core.ScopeProject && !event.Confirmed {
			return "", transitionError("project stop requires confirmation")
		}
		if state == core.Clean || state == core.Integrated || state == core.Cancelled || state == core.Archived {
			return "", transitionError("state %q cannot be stopped", state)
		}
		next = core.Interrupted
	case Cancel:
		if !event.Confirmed || strings.TrimSpace(event.Reason) == "" {
			return "", transitionError("cancel requires explicit confirmation and a reason")
		}
		if state == core.Cancelled || state == core.Archived || state == core.Integrated {
			return "", transitionError("state %q cannot be cancelled", state)
		}
		next = core.Cancelled
	case Resume:
		if state != core.Paused && state != core.Blocked && state != core.Interrupted {
			return "", transitionError("state %q cannot be resumed", state)
		}
		next = core.Ready
	case CheckpointEvent:
		if !event.AdmissionHeld || !event.RefillHeld || !validDigest(event.CheckpointDigest) {
			return "", transitionError("checkpoint requires holds and a valid digest")
		}
		next = state
	default:
		return "", transitionError("unknown event kind %q", event.Kind)
	}
	if event.To != "" && event.To != next {
		return "", transitionError("target state %q does not match event %q", event.To, event.Kind)
	}
	return next, nil
}

func checkpointPath(id core.RunID, scope core.Scope) string {
	leaf := string(scope.Kind) + "-" + scope.ID
	if scope.Kind == core.ScopeProject {
		sum := sha256.Sum256([]byte(scope.ID))
		leaf = string(scope.Kind) + "-" + hex.EncodeToString(sum[:])
	}
	return path.Join(".agent-team", "checkpoints", string(id), leaf+".json")
}

func checkpointLimit(s *store.Store) int64 {
	if s != nil && s.Limits.CanonicalBytes > 0 && s.Limits.CanonicalBytes < 16<<20 {
		return s.Limits.CanonicalBytes
	}
	return 16 << 20
}

func receiptPath(team core.TeamID) (string, error) {
	if err := project.ValidateSegment(string(team)); err != nil {
		return "", err
	}
	return path.Join(".agent-team", "receipts", string(team)+".json"), nil
}

func receiptIdentity(receipt knowledge.Receipt) string {
	return fmt.Sprintf("project=%s;run=%s;team=%s;task=%s;attempt=%d;state=%s;revision=%d", receipt.Project, receipt.RunID, receipt.Team, receipt.Task, receipt.Attempt, receipt.State, receipt.Revision)
}

func receiptDigest(receipt knowledge.Receipt) (string, error) {
	encoded, err := json.Marshal(receipt)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(encoded)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

func readReceipt(ctx context.Context, s *store.Store, team run.TeamRecord, manifest run.Run) (knowledge.Receipt, string, error) {
	if err := ctx.Err(); err != nil {
		return knowledge.Receipt{}, "", err
	}
	relative, err := receiptPath(team.ID)
	if err != nil {
		return knowledge.Receipt{}, "", err
	}
	var receipt knowledge.Receipt
	if err := s.ReadJSON(relative, maxReceiptBytes, &receipt); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return knowledge.Receipt{}, "", fmt.Errorf("%w: missing receipt %s", core.ErrRevision, relative)
		}
		return knowledge.Receipt{}, "", err
	}
	if receipt.Schema != 1 || receipt.Project != manifest.Project || receipt.RunID != manifest.ID || receipt.Team != string(team.ID) || receipt.Task == "" || receipt.Attempt < 1 || receipt.State == "" || receipt.NextAction == "" || receipt.Revision == 0 {
		return knowledge.Receipt{}, "", fmt.Errorf("%w: inconsistent receipt %s", core.ErrRevision, relative)
	}
	if err := project.ValidateSegment(receipt.Task); err != nil || len(receipt.EvidencePointers) > 128 || len(receipt.NextAction) > 4096 {
		return knowledge.Receipt{}, "", fmt.Errorf("%w: invalid receipt bounds", core.ErrRevision)
	}
	for _, pointer := range receipt.EvidencePointers {
		if !validPointer(pointer) {
			return knowledge.Receipt{}, "", fmt.Errorf("%w: invalid receipt evidence pointer", core.ErrRevision)
		}
	}
	queued := false
	for _, task := range team.Queue {
		if string(task) == receipt.Task {
			queued = true
			break
		}
	}
	if !queued {
		return knowledge.Receipt{}, "", fmt.Errorf("%w: receipt task is not queued by team", core.ErrRevision)
	}
	if _, err := time.Parse(time.RFC3339, receipt.WrittenAt); err != nil {
		return knowledge.Receipt{}, "", fmt.Errorf("%w: invalid receipt timestamp", core.ErrRevision)
	}
	digest, err := receiptDigest(receipt)
	return receipt, digest, err
}

func validPointer(pointer string) bool {
	if pointer == "" || len(pointer) > 4096 || strings.HasPrefix(pointer, "/") || strings.Contains(pointer, "\\") || path.Clean(pointer) != pointer {
		return false
	}
	for _, segment := range strings.Split(pointer, "/") {
		if err := project.ValidateSegment(segment); err != nil {
			return false
		}
	}
	return true
}

func teamsForScope(manifest run.Run, scope core.Scope) ([]run.TeamRecord, error) {
	if len(manifest.Teams) > maxReceiptProjections {
		return nil, fmt.Errorf("%w: receipt projection bound exceeded", core.ErrLimit)
	}
	if scope.Kind == core.ScopeTeam {
		for _, team := range manifest.Teams {
			if string(team.ID) == scope.ID {
				return []run.TeamRecord{team}, nil
			}
		}
		return nil, fmt.Errorf("%w: team is not in run", core.ErrRevision)
	}
	if scope.Kind == core.ScopeTask {
		found := false
		for _, task := range manifest.Tasks {
			if string(task.ID) == scope.ID {
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("%w: task is not in run", core.ErrRevision)
		}
		for _, team := range manifest.Teams {
			for _, task := range team.Queue {
				if string(task) == scope.ID {
					return []run.TeamRecord{team}, nil
				}
			}
		}
		return nil, fmt.Errorf("%w: task has no canonical team", core.ErrRevision)
	}
	if scope.Kind != core.ScopeRun && scope.Kind != core.ScopeProject {
		return nil, fmt.Errorf("%w: invalid checkpoint scope", core.ErrTransition)
	}
	if len(manifest.Teams) == 0 {
		return nil, fmt.Errorf("%w: run has no receipt authority", core.ErrRevision)
	}
	return append([]run.TeamRecord(nil), manifest.Teams...), nil
}

func teamForProjection(manifest run.Run, scope core.Scope, relative string) (run.TeamRecord, error) {
	teams, err := teamsForScope(manifest, scope)
	if err != nil {
		return run.TeamRecord{}, err
	}
	for _, team := range teams {
		teamPath, pathErr := receiptPath(team.ID)
		if pathErr == nil && teamPath == relative {
			return team, nil
		}
	}
	return run.TeamRecord{}, fmt.Errorf("%w: receipt projection is not canonical", core.ErrRevision)
}

func projectionFor(ctx context.Context, s *store.Store, manifest run.Run, team run.TeamRecord, checkpoint string) (ReceiptProjection, knowledge.Receipt, error) {
	receipt, beforeDigest, err := readReceipt(ctx, s, team, manifest)
	if err != nil {
		return ReceiptProjection{}, knowledge.Receipt{}, err
	}
	relative, _ := receiptPath(team.ID)
	before := receiptIdentity(receipt)
	afterReceipt := receipt
	found := false
	for _, pointer := range afterReceipt.EvidencePointers {
		if pointer == checkpoint {
			found = true
			break
		}
	}
	if !found {
		afterReceipt.EvidencePointers = append(afterReceipt.EvidencePointers, checkpoint)
		afterReceipt.Revision++
	}
	afterDigest, err := receiptDigest(afterReceipt)
	if err != nil {
		return ReceiptProjection{}, knowledge.Receipt{}, err
	}
	return ReceiptProjection{Path: relative, BeforeIdentity: before, BeforeDigest: beforeDigest, AfterIdentity: receiptIdentity(afterReceipt), AfterDigest: afterDigest}, afterReceipt, nil
}

func buildRecord(ctx context.Context, s *store.Store, manifest run.Run, scope core.Scope, digest string) (CheckpointRecord, []knowledge.Receipt, error) {
	teams, err := teamsForScope(manifest, scope)
	if err != nil {
		return CheckpointRecord{}, nil, err
	}
	record := CheckpointRecord{RecordEnvelope: core.RecordEnvelope{Schema: 1, Project: manifest.Project, RunID: manifest.ID, WrittenAt: manifest.WrittenAt, Revision: manifest.Revision}, Scope: scope, Digest: digest, AdmissionHeld: true, RefillHeld: true, Receipts: make([]ReceiptProjection, 0, len(teams))}
	receipts := make([]knowledge.Receipt, 0, len(teams))
	var total int64
	for _, team := range teams {
		projection, after, err := projectionFor(ctx, s, manifest, team, checkpointPath(manifest.ID, scope))
		if err != nil {
			return CheckpointRecord{}, nil, err
		}
		record.Receipts = append(record.Receipts, projection)
		receipts = append(receipts, after)
		total += int64(len(projection.BeforeIdentity) + len(projection.AfterIdentity) + len(projection.BeforeDigest) + len(projection.AfterDigest) + len(projection.Path))
		if total > maxReceiptTotalBytes {
			return CheckpointRecord{}, nil, fmt.Errorf("%w: receipt projection bytes exceeded", core.ErrLimit)
		}
	}
	return record, receipts, nil
}

func validateRecord(record CheckpointRecord, manifest run.Run, scope core.Scope, digest string) error {
	if record.Schema != 1 || record.Project != manifest.Project || record.RunID != manifest.ID || record.WrittenAt != manifest.WrittenAt || record.Revision != manifest.Revision || record.Scope != scope || record.Digest != digest || !validDigest(record.Digest) || !record.AdmissionHeld || !record.RefillHeld || len(record.Receipts) == 0 || len(record.Receipts) > maxReceiptProjections {
		return fmt.Errorf("%w: checkpoint provenance conflict", core.ErrRevision)
	}
	teams, err := teamsForScope(manifest, scope)
	if err != nil || len(teams) != len(record.Receipts) {
		return fmt.Errorf("%w: checkpoint receipt authority changed", core.ErrRevision)
	}
	expected := make(map[string]bool, len(teams))
	for _, team := range teams {
		relative, pathErr := receiptPath(team.ID)
		if pathErr != nil {
			return fmt.Errorf("%w: checkpoint receipt path", core.ErrRevision)
		}
		expected[relative] = true
	}
	seen := make(map[string]bool, len(record.Receipts))
	for _, projection := range record.Receipts {
		if seen[projection.Path] || !expected[projection.Path] || path.Clean(projection.Path) != projection.Path || !strings.HasPrefix(projection.Path, ".agent-team/receipts/") || strings.Contains(projection.Path, "\\") || !validDigest(projection.BeforeDigest) || !validDigest(projection.AfterDigest) || len(projection.BeforeIdentity) > 4096 || len(projection.AfterIdentity) > 4096 || projection.BeforeIdentity == "" || projection.AfterIdentity == "" {
			return fmt.Errorf("%w: invalid receipt projection", core.ErrRevision)
		}
		seen[projection.Path] = true
	}
	if len(seen) != len(expected) {
		return fmt.Errorf("%w: checkpoint receipt projection set is incomplete", core.ErrRevision)
	}
	return nil
}

func validateStoredProjections(ctx context.Context, s *store.Store, manifest run.Run, record CheckpointRecord) error {
	for _, projection := range record.Receipts {
		if err := ctx.Err(); err != nil {
			return err
		}
		team, err := teamForProjection(manifest, record.Scope, projection.Path)
		if err != nil {
			return err
		}
		receipt, beforeDigest, err := readReceipt(ctx, s, team, manifest)
		if err != nil {
			return err
		}
		beforeIdentity := receiptIdentity(receipt)
		if beforeIdentity == projection.AfterIdentity && beforeDigest == projection.AfterDigest {
			continue
		}
		if beforeIdentity != projection.BeforeIdentity || beforeDigest != projection.BeforeDigest {
			return fmt.Errorf("%w: receipt projection provenance changed", core.ErrRevision)
		}
		candidate := receipt
		candidate.EvidencePointers = append([]string(nil), candidate.EvidencePointers...)
		candidate.EvidencePointers = append(candidate.EvidencePointers, checkpointPath(manifest.ID, record.Scope))
		candidate.Revision++
		afterDigest, digestErr := receiptDigest(candidate)
		if digestErr != nil {
			return digestErr
		}
		if receiptIdentity(candidate) != projection.AfterIdentity || afterDigest != projection.AfterDigest {
			return fmt.Errorf("%w: receipt projection after-state conflict", core.ErrRevision)
		}
	}
	return nil
}

func convergeReceipt(ctx context.Context, s *store.Store, projection ReceiptProjection, manifest run.Run, scope core.Scope) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	team, err := teamForProjection(manifest, scope, projection.Path)
	if err != nil {
		return err
	}
	current, digest, err := readReceipt(ctx, s, team, manifest)
	if err != nil {
		return err
	}
	identity := receiptIdentity(current)
	if identity == projection.AfterIdentity && digest == projection.AfterDigest {
		return nil
	}
	if identity != projection.BeforeIdentity || digest != projection.BeforeDigest {
		return fmt.Errorf("%w: receipt changed during checkpoint recovery", core.ErrRevision)
	}
	current.EvidencePointers = append(current.EvidencePointers, checkpointPath(manifest.ID, scope))
	current.Revision++
	if err := ctx.Err(); err != nil {
		return err
	}
	return knowledge.WriteReceipt(ctx, s, current)
}

// Checkpoint commits exactly one no-replace checkpoint record, then converges
// the already-existing receipts named by that record. A projection failure
// leaves the checkpoint as recoverable durable evidence for the next retry.
func Checkpoint(ctx context.Context, s *store.Store, runID core.RunID, scope core.Scope, digest string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if s == nil {
		return fmt.Errorf("%w: nil store", core.ErrPath)
	}
	if err := project.ValidateSegment(string(runID)); err != nil {
		return err
	}
	if err := validCheckpointScope(scope); err != nil {
		return err
	}
	if scope.Kind == core.ScopeRun && scope.ID != string(runID) {
		return fmt.Errorf("%w: run scope does not match run", core.ErrTransition)
	}
	if !validDigest(digest) {
		return fmt.Errorf("%w: invalid checkpoint digest", core.ErrRevision)
	}
	manifest, err := run.NewRepositories(s).Runs.Read(ctx, runID)
	if err != nil {
		return fmt.Errorf("%w: canonical run: %v", core.ErrRevision, err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	relative := checkpointPath(runID, scope)
	record, receipts, err := buildRecord(ctx, s, manifest, scope, digest)
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	_, err = s.CreateJSON(relative, record, checkpointLimit(s))
	if err == nil {
		return publishReceipts(ctx, s, manifest, record, receipts)
	}
	if !errors.Is(err, fs.ErrExist) {
		return err
	}
	var existing CheckpointRecord
	if readErr := s.ReadJSON(relative, checkpointLimit(s), &existing); readErr != nil {
		return readErr
	}
	if err := validateRecord(existing, manifest, scope, digest); err != nil {
		return err
	}
	if err := validateStoredProjections(ctx, s, manifest, existing); err != nil {
		return err
	}
	return publishReceipts(ctx, s, manifest, existing, nil)
}

func publishReceipts(ctx context.Context, s *store.Store, manifest run.Run, record CheckpointRecord, desired []knowledge.Receipt) error {
	for index, projection := range record.Receipts {
		if err := ctx.Err(); err != nil {
			return err
		}
		if desired != nil && index < len(desired) {
			team, teamErr := teamForProjection(manifest, record.Scope, projection.Path)
			if teamErr != nil {
				return teamErr
			}
			current, currentDigest, readErr := readReceipt(ctx, s, team, manifest)
			if readErr != nil {
				return readErr
			}
			if receiptIdentity(current) == projection.AfterIdentity && currentDigest == projection.AfterDigest {
				continue
			}
			if err := knowledge.WriteReceipt(ctx, s, desired[index]); err != nil {
				return err
			}
			continue
		}
		if err := convergeReceipt(ctx, s, projection, manifest, record.Scope); err != nil {
			return err
		}
	}
	return nil
}
