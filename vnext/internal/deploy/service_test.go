package deploy

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/thebpandey/agent-team/vnext/internal/contracts"
	"github.com/thebpandey/agent-team/vnext/internal/core"
	"github.com/thebpandey/agent-team/vnext/internal/store"
)

type fakeProvider struct {
	mu                                sync.Mutex
	calls                             []string
	unknown, queryErr, verifyNilError bool
	verifyState                       string
}

type submitBarrierProvider struct {
	entered chan struct{}
	resume  chan struct{}
	mu      sync.Mutex
	calls   int
}

func (p *submitBarrierProvider) Submit(context.Context, contracts.DeploymentBatch, string) (contracts.Operation, error) {
	p.mu.Lock()
	p.calls++
	call := p.calls
	p.mu.Unlock()
	if call == 1 {
		close(p.entered)
		<-p.resume
	}
	return contracts.Operation{Provider: "fake", ProviderID: "op-1", State: "submitted"}, nil
}

func (p *submitBarrierProvider) Query(context.Context, contracts.Operation, string) (contracts.Operation, error) {
	return contracts.Operation{}, errors.New("unexpected query")
}

func (p *submitBarrierProvider) Verify(context.Context, contracts.Operation, string) (contracts.Verification, error) {
	return contracts.Verification{}, errors.New("unexpected verify")
}

func TestSeparateFileRepositoriesHaveOneSubmitter(t *testing.T) {
	root := t.TempDir()
	provider := &submitBarrierProvider{entered: make(chan struct{}), resume: make(chan struct{})}
	executor, err := NewBoundExecutor(testProfile(), provider)
	if err != nil {
		t.Fatal(err)
	}
	results := make(chan error, 2)
	run := func() {
		repository := NewRepository(store.New(root, core.StorageLimits{CanonicalBytes: 1 << 20}))
		_, err := SubmitOrReconcile(context.Background(), repository, executor, testBatch("B-guard"))
		results <- err
	}
	go run()
	<-provider.entered
	go run()
	select {
	case <-results:
		t.Fatal("second deploy escaped durable guard")
	case <-time.After(20 * time.Millisecond):
	}
	close(provider.resume)
	if err := <-results; err != nil {
		t.Fatal(err)
	}
	if err := <-results; err != nil {
		t.Fatal(err)
	}
	provider.mu.Lock()
	calls := provider.calls
	provider.mu.Unlock()
	if calls != 1 {
		t.Fatalf("provider submits = %d, want 1", calls)
	}
}

func TestFileRepositoryCASAndRestart(t *testing.T) {
	root := t.TempDir()
	state := store.New(root, core.StorageLimits{CanonicalBytes: 1 << 20})
	repository := NewRepository(state)
	operation := OperationRecord{RecordEnvelope: core.RecordEnvelope{Schema: 1, RunID: "R-1"}, BatchID: "B-1", Fingerprint: "sha256:f", ProfileID: "staging", Target: "staging", State: "pending", Attempt: 1, Operation: contracts.Operation{State: "pending", IdempotencyKey: "deploy-v1:key"}}
	written, err := repository.CompareAndSwapOperation(context.Background(), 0, operation)
	if err != nil || written.Revision != 1 {
		t.Fatal(written, err)
	}
	restarted := NewRepository(store.New(root, core.StorageLimits{CanonicalBytes: 1 << 20}))
	got, err := restarted.ReadOperation(context.Background(), "B-1")
	if err != nil || got.Fingerprint != operation.Fingerprint || got.Revision != 1 {
		t.Fatal(got, err)
	}
	if _, err := restarted.CompareAndSwapOperation(context.Background(), 0, operation); !errors.Is(err, core.ErrRevision) {
		t.Fatal("stale operation CAS accepted", err)
	}
}

func (p *fakeProvider) Submit(context.Context, contracts.DeploymentBatch, string) (contracts.Operation, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls = append(p.calls, "submit")
	if p.unknown {
		return contracts.Operation{Provider: "fake", ProviderID: "op-1", State: "unknown", Unknown: true}, errors.New("transport")
	}
	return contracts.Operation{Provider: "fake", ProviderID: "op-1", State: "submitted", IdempotencyKey: "key"}, nil
}

func (p *fakeProvider) Query(context.Context, contracts.Operation, string) (contracts.Operation, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls = append(p.calls, "query")
	if p.queryErr {
		return contracts.Operation{Provider: "fake", ProviderID: "op-1", State: "unknown", Unknown: true, IdempotencyKey: "key"}, errors.New("query unavailable")
	}
	return contracts.Operation{Provider: "fake", ProviderID: "op-1", State: "succeeded", IdempotencyKey: "key"}, nil
}

func (p *fakeProvider) Verify(context.Context, contracts.Operation, string) (contracts.Verification, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls = append(p.calls, "verify")
	verification := contracts.Verification{State: p.verifyState, OutputPointer: "memory:verify"}
	if p.verifyState == "failed" && !p.verifyNilError {
		return verification, errors.New("verification failed")
	}
	return verification, nil
}

func testProfile() TargetProfile {
	profile := TargetProfile{ID: "staging", Target: "staging", AuthorizationRef: "approval-1", ApprovalScope: "staging", ExecutorCommand: []string{"provider"}, QueryCommand: []string{"provider-query"}, VerificationCommand: []string{"provider-verify"}, DefaultBatchSize: 1, Enabled: true}
	profile.ProfileDigest, _ = DeriveProfileDigest(profile)
	return profile
}

func testBatch(id string) BatchManifest {
	return BatchManifest{RunID: "R-1", BatchID: id, Target: "staging", ProfileDigest: "sha256:profile", Fingerprint: "sha256:" + id, IdempotencyKey: "deploy-v1:" + id, TaskIDs: []core.TaskID{"T-1"}, Revisions: []string{"rev-1"}, ArtifactDigests: [][]string{{"artifact-1"}}}
}

func TestUnknownResumeAndFailedVerificationAreDurable(t *testing.T) {
	provider := &fakeProvider{unknown: true, verifyState: "succeeded"}
	repo := NewMemoryRepository()
	executor, err := NewBoundExecutor(testProfile(), provider)
	if err != nil {
		t.Fatal(err)
	}
	batch := testBatch("B-1")
	first, err := SubmitOrReconcile(context.Background(), repo, executor, batch)
	if err == nil || first.Receipt.State != "unknown" || !reflect.DeepEqual(provider.calls, []string{"submit"}) {
		t.Fatal(first, err, provider.calls)
	}
	operation, err := repo.ReadOperation(context.Background(), batch.BatchID)
	if err != nil || operation.Operation.ProviderID != "op-1" || operation.State != "unknown" {
		t.Fatal(operation, err)
	}
	provider.unknown = false
	resumed, err := ResumeBatch(context.Background(), repo, executor, batch.BatchID)
	if err != nil || resumed.Receipt.State != "succeeded" || !reflect.DeepEqual(provider.calls, []string{"submit", "query", "verify"}) {
		t.Fatal(resumed, err, provider.calls)
	}
	provider.verifyState = "failed"
	provider.verifyNilError = true
	failedBatch := testBatch("B-2")
	if _, err := SubmitOrReconcile(context.Background(), repo, executor, failedBatch); err != nil {
		t.Fatal(err)
	}
	failed, err := ResumeBatch(context.Background(), repo, executor, failedBatch.BatchID)
	if err != nil || failed.Receipt.State != "failed" {
		t.Fatal(failed, err)
	}
	if len(repo.Evidence()) < 3 {
		t.Fatal("failed verification evidence missing")
	}
	queryFail := &fakeProvider{queryErr: true, verifyState: "succeeded"}
	queryRepo := NewMemoryRepository()
	queryExecutor, _ := NewBoundExecutor(testProfile(), queryFail)
	queryBatch := testBatch("B-query")
	_, _ = SubmitOrReconcile(context.Background(), queryRepo, queryExecutor, queryBatch)
	queryOutcome, queryErr := ResumeBatch(context.Background(), queryRepo, queryExecutor, queryBatch.BatchID)
	if queryErr == nil || queryOutcome.Receipt.State != "unknown" {
		t.Fatal(queryOutcome, queryErr)
	}
	persisted, _ := queryRepo.ReadReceipt(context.Background(), queryBatch.BatchID)
	if persisted.State != "unknown" || len(persisted.EvidencePointers) != 1 {
		t.Fatal(persisted)
	}
}
