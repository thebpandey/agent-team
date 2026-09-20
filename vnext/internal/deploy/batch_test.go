package deploy

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/thebpandey/agent-team/vnext/internal/core"
)

func TestSevenTasksPlanThreeThreeOneAndArtifactFingerprint(t *testing.T) {
	profile := TargetProfile{ID: "staging", Target: "staging", AuthorizationRef: "approval-1", ApprovalScope: "staging", ExecutorCommand: []string{"provider"}, QueryCommand: []string{"provider-query"}, VerificationCommand: []string{"provider-verify"}, Enabled: true, Confirmed: true, DefaultBatchSize: 3}
	profile.ProfileDigest, _ = DeriveProfileDigest(profile)
	tasks := make([]EligibleTask, 7)
	for i := range tasks {
		tasks[i] = EligibleTask{ID: core.TaskID(fmt.Sprintf("T-%d", i)), CanonicalRevision: fmt.Sprintf("rev-%d", i), ArtifactDigests: []string{fmt.Sprintf("artifact-%d", i)}, Integrated: true, GatePassed: true, DependenciesComplete: true}
	}
	batches, err := PlanBatches(context.Background(), BatchRequest{RunID: "R-1", Target: "staging", Profile: profile, Tasks: tasks, BatchSize: 3})
	if err != nil || len(batches) != 3 || len(batches[0].TaskIDs) != 3 || len(batches[1].TaskIDs) != 3 || len(batches[2].TaskIDs) != 1 {
		t.Fatal(len(batches), err)
	}
	changed := batches[0]
	changed.ArtifactDigests = append([][]string(nil), batches[0].ArtifactDigests...)
	changed.ArtifactDigests[0] = append([]string(nil), batches[0].ArtifactDigests[0]...)
	changed.ArtifactDigests[0][0] = "artifact-changed"
	a, _ := DeriveBatchFingerprint(batches[0])
	b, _ := DeriveBatchFingerprint(changed)
	if a == b {
		t.Fatal("artifact change reused fingerprint")
	}
	if batches[0].IdempotencyKey != DeriveBatchIdempotencyKey("R-1", 0, batches[0].Fingerprint) {
		t.Fatal("unstable idempotency key")
	}
	if n, _ := ResolveBatchSize(profile, 0); n != 3 {
		t.Fatal(n)
	}
	if _, err := ResolveBatchSize(profile, 101); !errors.Is(err, core.ErrBatch) {
		t.Fatal(err)
	}
	if contract, err := batches[0].ContractBatch(profile); err != nil || contract.Fingerprint != batches[0].Fingerprint || len(contract.TaskIDs) != 3 {
		t.Fatal(contract, err)
	}
}
