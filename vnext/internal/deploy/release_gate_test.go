package deploy

import (
	"context"
	"errors"
	"testing"

	"github.com/thebpandey/agent-team/vnext/internal/core"
)

func TestProviderVerification(t *testing.T) {
	provider := &fakeProvider{verifyState: "succeeded", verifyNilError: true}
	repository := NewMemoryRepository()
	executor, err := NewBoundExecutor(testProfile(), provider)
	if err != nil {
		t.Fatal(err)
	}
	outcome, err := SubmitOrReconcile(context.Background(), repository, executor, testBatch("release-provider-B1"))
	if err != nil || outcome.Receipt.State != "submitted" {
		t.Fatal(outcome, err)
	}
	resumed, err := ResumeBatch(context.Background(), repository, executor, "release-provider-B1")
	if err != nil || resumed.Receipt.State != "succeeded" || len(repository.Evidence()) < 2 {
		t.Fatal(resumed, err)
	}
	if len(provider.calls) != 3 || provider.calls[0] != "submit" || provider.calls[1] != "query" || provider.calls[2] != "verify" {
		t.Fatalf("provider calls=%v", provider.calls)
	}
	if err := ValidateProfile(testProfile()); err != nil {
		t.Fatal(err)
	}
	if err := ValidateProfile(TargetProfile{ID: "prod", Target: "production", ApprovalScope: "production", Enabled: true}); !errors.Is(err, core.ErrSettings) {
		t.Fatal("unauthorized production profile accepted")
	}
}
