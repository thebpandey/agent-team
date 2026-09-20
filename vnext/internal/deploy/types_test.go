package deploy

import (
	"encoding/json"
	"testing"

	"github.com/thebpandey/agent-team/vnext/internal/core"
)

func TestDeploymentContractsRoundTrip(t *testing.T) {
	p := TargetProfile{ID: "staging", Target: "staging", ExecutorCommand: []string{"provider"}, QueryCommand: []string{"provider-query"}, VerificationCommand: []string{"provider-verify"}, DefaultBatchSize: 3}
	raw, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	var got TargetProfile
	if err := json.Unmarshal(raw, &got); err != nil || got.ID != p.ID || got.DefaultBatchSize != 3 {
		t.Fatal(err, got)
	}
	var _ core.TaskID = core.TaskID("T-1")
}
