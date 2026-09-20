package deploy

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"sync"

	"github.com/thebpandey/agent-team/vnext/internal/contracts"
	"github.com/thebpandey/agent-team/vnext/internal/core"
	"github.com/thebpandey/agent-team/vnext/internal/store"
)

const deploymentRecordLimit = 1 << 20

type MemoryRepository struct {
	mu         sync.Mutex
	operations map[string]OperationRecord
	receipts   map[string]DeploymentReceipt
	evidence   []DeploymentEvidence
}

func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{operations: map[string]OperationRecord{}, receipts: map[string]DeploymentReceipt{}}
}

func (r *MemoryRepository) ReadOperation(_ context.Context, id string) (OperationRecord, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	value, ok := r.operations[id]
	if !ok {
		return OperationRecord{}, ErrNotFound
	}
	return cloneOperation(value), nil
}

func (r *MemoryRepository) CompareAndSwapOperation(_ context.Context, expected uint64, value OperationRecord) (OperationRecord, error) {
	if err := validateOperationRecord(value); err != nil {
		return OperationRecord{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	old := r.operations[value.BatchID]
	if old.Revision != expected {
		return OperationRecord{}, core.ErrRevision
	}
	value.Revision = expected + 1
	r.operations[value.BatchID] = cloneOperation(value)
	return cloneOperation(value), nil
}

func (r *MemoryRepository) WriteEvidence(_ context.Context, value DeploymentEvidence) (string, error) {
	if value.BatchID == "" || value.ProfileID == "" || value.Target == "" || value.Fingerprint == "" || value.State == "" {
		return "", core.ErrSettings
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	value.Revision = uint64(len(r.evidence) + 1)
	r.evidence = append(r.evidence, cloneEvidence(value))
	return fmt.Sprintf("memory:evidence/%d", value.Revision), nil
}

func (r *MemoryRepository) ReadReceipt(_ context.Context, id string) (DeploymentReceipt, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	value, ok := r.receipts[id]
	if !ok {
		return DeploymentReceipt{}, ErrNotFound
	}
	return cloneReceipt(value), nil
}

func (r *MemoryRepository) CompareAndSwapReceipt(_ context.Context, expected uint64, value DeploymentReceipt) (DeploymentReceipt, error) {
	if err := validateReceipt(value); err != nil {
		return DeploymentReceipt{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	old := r.receipts[value.BatchID]
	if old.Revision != expected {
		return DeploymentReceipt{}, core.ErrRevision
	}
	value.Revision = expected + 1
	r.receipts[value.BatchID] = cloneReceipt(value)
	return cloneReceipt(value), nil
}

func (r *MemoryRepository) Evidence() []DeploymentEvidence {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]DeploymentEvidence, len(r.evidence))
	for i := range r.evidence {
		out[i] = cloneEvidence(r.evidence[i])
	}
	return out
}

type FileRepository struct {
	mu    sync.Mutex
	store *store.Store
}

func NewRepository(state *store.Store) Repository { return &FileRepository{store: state} }

func (r *FileRepository) ReadOperation(ctx context.Context, id string) (OperationRecord, error) {
	if !validProfileID(ProfileID(id)) {
		return OperationRecord{}, core.ErrSettings
	}
	var value OperationRecord
	if err := r.read(ctx, recordPath("operations", id), &value); err != nil {
		return OperationRecord{}, err
	}
	return value, nil
}

func (r *FileRepository) CompareAndSwapOperation(ctx context.Context, expected uint64, value OperationRecord) (OperationRecord, error) {
	if err := validateOperationRecord(value); err != nil {
		return OperationRecord{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.compareRevision(ctx, recordPath("operations", value.BatchID), expected); err != nil {
		return OperationRecord{}, err
	}
	value.Revision = expected + 1
	if _, err := r.store.WriteJSON(recordPath("operations", value.BatchID), value, deploymentRecordLimit); err != nil {
		return OperationRecord{}, err
	}
	return value, nil
}

func (r *FileRepository) WriteEvidence(ctx context.Context, value DeploymentEvidence) (string, error) {
	if ctx == nil || ctx.Err() != nil || r == nil || r.store == nil || !validProfileID(ProfileID(value.BatchID)) {
		return "", core.ErrSettings
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	path := "deploy/evidence/" + hex.EncodeToString(sum[:]) + ".json"
	if _, err := r.store.WriteJSON(path, value, deploymentRecordLimit); err != nil {
		return "", err
	}
	return path, nil
}

func (r *FileRepository) ReadReceipt(ctx context.Context, id string) (DeploymentReceipt, error) {
	if !validProfileID(ProfileID(id)) {
		return DeploymentReceipt{}, core.ErrSettings
	}
	var value DeploymentReceipt
	if err := r.read(ctx, recordPath("receipts", id), &value); err != nil {
		return DeploymentReceipt{}, err
	}
	return value, nil
}

func (r *FileRepository) CompareAndSwapReceipt(ctx context.Context, expected uint64, value DeploymentReceipt) (DeploymentReceipt, error) {
	if err := validateReceipt(value); err != nil {
		return DeploymentReceipt{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.compareRevision(ctx, recordPath("receipts", value.BatchID), expected); err != nil {
		return DeploymentReceipt{}, err
	}
	value.Revision = expected + 1
	if _, err := r.store.WriteJSON(recordPath("receipts", value.BatchID), value, deploymentRecordLimit); err != nil {
		return DeploymentReceipt{}, err
	}
	return value, nil
}

func (r *FileRepository) read(ctx context.Context, path string, destination any) error {
	if ctx == nil || ctx.Err() != nil || r == nil || r.store == nil {
		return core.ErrSettings
	}
	if err := r.store.ReadJSON(path, deploymentRecordLimit, destination); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return ErrNotFound
		}
		return err
	}
	return nil
}

func (r *FileRepository) compareRevision(ctx context.Context, path string, expected uint64) error {
	var envelope core.RecordEnvelope
	err := r.read(ctx, path, &envelope)
	if errors.Is(err, ErrNotFound) && expected == 0 {
		return nil
	}
	if err != nil {
		return err
	}
	if envelope.Revision != expected {
		return core.ErrRevision
	}
	return nil
}

func NewBoundExecutor(profile TargetProfile, provider ProviderAdapter) (*BoundExecutor, error) {
	if provider == nil {
		return nil, core.ErrSettings
	}
	if err := ValidateProfile(profile); err != nil {
		return nil, err
	}
	return &BoundExecutor{Profile: cloneProfile(profile), Provider: provider}, nil
}

func SubmitOrReconcile(ctx context.Context, repository Repository, executor *BoundExecutor, batch BatchManifest) (DeployOutcome, error) {
	if ctx == nil || repository == nil || executor == nil || executor.Provider == nil || batch.BatchID == "" || batch.Fingerprint == "" || batch.IdempotencyKey == "" {
		return DeployOutcome{}, core.ErrSettings
	}
	existing, err := repository.ReadOperation(ctx, batch.BatchID)
	if err == nil {
		if existing.Fingerprint != batch.Fingerprint || existing.Operation.IdempotencyKey != batch.IdempotencyKey {
			return DeployOutcome{}, core.ErrRevision
		}
		if receipt, readErr := repository.ReadReceipt(ctx, batch.BatchID); readErr == nil && (receipt.State == "succeeded" || receipt.State == "failed") {
			return DeployOutcome{Receipt: receipt, CodingMayContinue: true}, nil
		}
		return ResumeBatch(ctx, repository, executor, batch.BatchID)
	}
	if !errors.Is(err, ErrNotFound) {
		return DeployOutcome{}, err
	}
	pending := OperationRecord{RecordEnvelope: core.RecordEnvelope{Schema: 1, RunID: batch.RunID}, BatchID: batch.BatchID, Fingerprint: batch.Fingerprint, ProfileID: string(executor.Profile.ID), Target: batch.Target, State: "pending", Attempt: 1, Operation: contracts.Operation{State: "pending", IdempotencyKey: batch.IdempotencyKey}}
	written, err := repository.CompareAndSwapOperation(ctx, 0, pending)
	if err != nil {
		return DeployOutcome{}, err
	}
	contract := contracts.DeploymentBatch{Run: batch.RunID, TaskIDs: append([]core.TaskID(nil), batch.TaskIDs...), Revisions: append([]string(nil), batch.Revisions...), TargetProfile: string(executor.Profile.ID), AuthorizationRef: executor.Profile.AuthorizationRef, ExecutorCommand: append([]string(nil), executor.Profile.ExecutorCommand...), VerificationCommand: append([]string(nil), executor.Profile.VerificationCommand...), Fingerprint: batch.Fingerprint, IdempotencyKey: batch.IdempotencyKey}
	operation, submitErr := executor.Provider.Submit(ctx, contract, batch.IdempotencyKey)
	operation.IdempotencyKey = batch.IdempotencyKey
	state := "submitted"
	if submitErr != nil || operation.Unknown {
		state = "unknown"
	}
	written.Operation, written.State = operation, state
	written, err = repository.CompareAndSwapOperation(ctx, written.Revision, written)
	if err != nil {
		return DeployOutcome{}, err
	}
	receipt := DeploymentReceipt{RecordEnvelope: core.RecordEnvelope{Schema: 1, RunID: batch.RunID}, RunID: batch.RunID, BatchID: batch.BatchID, ProviderID: operation.ProviderID, ProfileID: string(executor.Profile.ID), Target: batch.Target, Fingerprint: batch.Fingerprint, IdempotencyKey: batch.IdempotencyKey, State: state, CodingMayContinue: true}
	pointer, err := repository.WriteEvidence(ctx, DeploymentEvidence{RecordEnvelope: core.RecordEnvelope{Schema: 1, RunID: batch.RunID}, BatchID: batch.BatchID, ProfileID: string(executor.Profile.ID), Target: batch.Target, Fingerprint: batch.Fingerprint, State: state, Error: errorText(submitErr), Attempt: written.Attempt})
	if err != nil {
		return DeployOutcome{Receipt: receipt, CodingMayContinue: true}, err
	}
	receipt.EvidencePointers = []string{pointer}
	receipt, err = repository.CompareAndSwapReceipt(ctx, 0, receipt)
	if err != nil {
		return DeployOutcome{Receipt: receipt, CodingMayContinue: true}, err
	}
	return DeployOutcome{Receipt: receipt, CodingMayContinue: true}, submitErr
}

func ResumeBatch(ctx context.Context, repository Repository, executor *BoundExecutor, id string) (DeployOutcome, error) {
	if ctx == nil || repository == nil || executor == nil || executor.Provider == nil || id == "" {
		return DeployOutcome{}, core.ErrSettings
	}
	operation, err := repository.ReadOperation(ctx, id)
	if err != nil {
		return DeployOutcome{}, err
	}
	oldReceipt, readErr := repository.ReadReceipt(ctx, id)
	if readErr == nil && (oldReceipt.State == "succeeded" || oldReceipt.State == "failed") {
		return DeployOutcome{Receipt: oldReceipt, CodingMayContinue: true}, nil
	}
	if readErr != nil && !errors.Is(readErr, ErrNotFound) {
		return DeployOutcome{}, readErr
	}
	expected := uint64(0)
	if readErr == nil {
		expected = oldReceipt.Revision
	}
	queried, queryErr := executor.Provider.Query(ctx, operation.Operation, id)
	queried.IdempotencyKey = operation.Operation.IdempotencyKey
	state := "unknown"
	if queryErr == nil && queried.State == "succeeded" {
		state = "succeeded"
	}
	queryPointer, err := repository.WriteEvidence(ctx, DeploymentEvidence{RecordEnvelope: core.RecordEnvelope{Schema: 1, RunID: operation.RunID}, BatchID: operation.BatchID, ProfileID: operation.ProfileID, Target: operation.Target, Fingerprint: operation.Fingerprint, State: state, Error: errorText(queryErr), Attempt: operation.Attempt})
	if err != nil {
		return DeployOutcome{}, err
	}
	operation.Operation, operation.State = queried, state
	operation, err = repository.CompareAndSwapOperation(ctx, operation.Revision, operation)
	if err != nil {
		return DeployOutcome{}, err
	}
	receipt := DeploymentReceipt{RecordEnvelope: core.RecordEnvelope{Schema: 1, RunID: operation.RunID}, RunID: operation.RunID, BatchID: operation.BatchID, ProviderID: queried.ProviderID, ProfileID: operation.ProfileID, Target: operation.Target, Fingerprint: operation.Fingerprint, IdempotencyKey: queried.IdempotencyKey, State: state, EvidencePointers: []string{queryPointer}, CodingMayContinue: true}
	if queryErr != nil || state != "succeeded" {
		receipt, err = repository.CompareAndSwapReceipt(ctx, expected, receipt)
		if err != nil {
			return DeployOutcome{Receipt: receipt, CodingMayContinue: true}, err
		}
		return DeployOutcome{Receipt: receipt, CodingMayContinue: true}, queryErr
	}
	verification, verifyErr := executor.Provider.Verify(ctx, queried, id)
	if verifyErr != nil || verification.State != "succeeded" {
		state = "failed"
	}
	verifyPointer, err := repository.WriteEvidence(ctx, DeploymentEvidence{RecordEnvelope: core.RecordEnvelope{Schema: 1, RunID: operation.RunID}, BatchID: operation.BatchID, ProfileID: operation.ProfileID, Target: operation.Target, Fingerprint: operation.Fingerprint, State: state, Error: errorText(verifyErr), OutputPointer: verification.OutputPointer, Attempt: operation.Attempt, Exit: verification.Exit, Command: append([]string(nil), verification.Command...)})
	if err != nil {
		return DeployOutcome{}, err
	}
	receipt.State, receipt.Verification = state, verification
	receipt.EvidencePointers = append(receipt.EvidencePointers, verifyPointer)
	receipt, err = repository.CompareAndSwapReceipt(ctx, expected, receipt)
	if err != nil {
		return DeployOutcome{Receipt: receipt, CodingMayContinue: true}, err
	}
	return DeployOutcome{Receipt: receipt, CodingMayContinue: true}, verifyErr
}

func recordPath(kind, id string) string {
	if !validProfileID(ProfileID(id)) {
		return "deploy/invalid"
	}
	return "deploy/" + kind + "/" + id + ".json"
}

func errorText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func cloneOperation(value OperationRecord) OperationRecord { return value }

func cloneReceipt(value DeploymentReceipt) DeploymentReceipt {
	value.EvidencePointers = append([]string(nil), value.EvidencePointers...)
	value.Verification.Command = append([]string(nil), value.Verification.Command...)
	return value
}

func cloneEvidence(value DeploymentEvidence) DeploymentEvidence {
	value.Command = append([]string(nil), value.Command...)
	return value
}

func validateOperationRecord(value OperationRecord) error {
	if !validProfileID(ProfileID(value.BatchID)) || !validProfileID(ProfileID(value.ProfileID)) || value.Fingerprint == "" || value.Target == "" || value.State == "" || value.Attempt < 1 || value.Operation.IdempotencyKey == "" {
		return core.ErrSettings
	}
	return nil
}

func validateReceipt(value DeploymentReceipt) error {
	if !validProfileID(ProfileID(value.BatchID)) || !validProfileID(ProfileID(value.ProfileID)) || value.RunID == "" || value.Target == "" || value.Fingerprint == "" || value.IdempotencyKey == "" || value.State == "" {
		return core.ErrSettings
	}
	return nil
}
