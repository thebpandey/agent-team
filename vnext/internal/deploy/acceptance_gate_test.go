package deploy

import (
	"context"
	"fmt"
	"testing"

	"github.com/thebpandey/agent-team/vnext/internal/core"
)

func TestDeploymentGate(t *testing.T) {
	profile := testProfile()
	tasks := make([]EligibleTask, 7)
	for i := range tasks {
		tasks[i] = EligibleTask{ID: core.TaskID(fmt.Sprintf("T-%d", i)), CanonicalRevision: fmt.Sprintf("rev-%d", i), ArtifactDigests: []string{fmt.Sprintf("artifact-%d", i)}, Integrated: true, GatePassed: true, DependenciesComplete: true}
	}
	batches, err := PlanBatches(context.Background(), BatchRequest{RunID: "R-gate", Target: "staging", Profile: profile, Tasks: tasks, BatchSize: 3})
	if err != nil || len(batches) != 3 || len(batches[0].TaskIDs) != 3 || len(batches[1].TaskIDs) != 3 || len(batches[2].TaskIDs) != 1 {
		t.Fatal(len(batches), err)
	}
	for _, batch := range batches {
		if batch.Fingerprint == "" || batch.IdempotencyKey == "" || batch.ProfileDigest == "" || batch.Target != "staging" {
			t.Fatal(batch)
		}
	}
	failed := (NativeRunner{Root: t.TempDir(), Limit: 8}).Run(context.Background(), CommandInvocation{Executable: ""})
	if failed.Started || failed.Transport == nil || failed.Exit != -1 {
		t.Fatal(failed)
	}
}
