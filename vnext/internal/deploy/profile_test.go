package deploy

import (
	"context"
	"errors"
	"testing"

	"github.com/thebpandey/agent-team/vnext/internal/core"
	"github.com/thebpandey/agent-team/vnext/internal/store"
)

func TestProfileCASAuthorizationDigestAndDuplicate(t *testing.T) {
	repo := NewProfileRepository(store.New(t.TempDir(), core.StorageLimits{CanonicalBytes: 64 * 1024}))
	decision := ProfileDecision{Enable: true, Target: "staging", AuthorizationRef: "approval-1", ApprovalScope: "staging", ExecutorCommand: []string{"provider"}, QueryCommand: []string{"provider-query"}, VerificationCommand: []string{"provider-verify"}, DefaultBatchSize: 3, Confirmed: true}
	first, err := repo.ApplyDecision(context.Background(), "staging", decision, 0)
	if err != nil || first.Kind != ProfileCreated || first.Profile.ProfileDigest == "" {
		t.Fatal(first, err)
	}
	duplicate, err := repo.ApplyDecision(context.Background(), "staging", decision, first.ObservedRevision)
	if err != nil || duplicate.Kind != ProfileDuplicate || !duplicate.Idempotent || duplicate.ObservedRevision != first.ObservedRevision {
		t.Fatal(duplicate, err)
	}
	if _, err := repo.ApplyDecision(context.Background(), "staging", decision, 0); !errors.Is(err, core.ErrRevision) {
		t.Fatal("stale profile accepted", err)
	}
	changed := first.Profile
	changed.ProfileDigest = "sha256:wrong"
	if err := ValidateProfile(changed); !errors.Is(err, core.ErrSettings) {
		t.Fatal("changed digest accepted", err)
	}
}
