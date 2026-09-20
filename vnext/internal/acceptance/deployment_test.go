package acceptance

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"reflect"
	"testing"

	"github.com/thebpandey/agent-team/vnext/internal/cli"
	"github.com/thebpandey/agent-team/vnext/internal/contracts"
	"github.com/thebpandey/agent-team/vnext/internal/core"
	"github.com/thebpandey/agent-team/vnext/internal/deploy"
	"github.com/thebpandey/agent-team/vnext/internal/knowledge"
)

func TestCanonicalDeploymentCLI(t *testing.T) {
	var calls int
	var seen []string
	deps := core.Dependencies{Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}, Deployment: func(_ context.Context, args []string, _, _ io.Writer) int {
		calls++
		seen = append([]string(nil), args...)
		return 0
	}}
	args := []string{"deploy", "--run", "R-1", "--target", "staging", "--profile", "staging", "--batch-size", "3"}
	if code := cli.Run(context.Background(), args, deps); code != 0 || calls != 1 || !reflect.DeepEqual(seen, args) {
		t.Fatal(code, calls, seen)
	}
	if code := cli.Run(context.Background(), []string{"review"}, deps); code == 0 || calls != 1 {
		t.Fatal("legacy action accepted", code)
	}
	if _, err := deploy.ParseDeployArgs([]string{"deploy", "--target", "staging", "--run", "R-1", "--profile", "staging", "--batch-size", "0"}); !errors.Is(err, core.ErrBatch) {
		t.Fatal(err)
	}
	parsedFull, err := deploy.ParseDeployArgs(args)
	if err != nil || parsedFull.RunID != "R-1" || parsedFull.BatchSize != 3 {
		t.Fatal(parsedFull, err)
	}
	parsed, err := deploy.ParseDeployArgs([]string{"deploy", "--resume"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := deploy.ResolveDeployRun(parsed, nil); !errors.Is(err, core.ErrSettings) {
		t.Fatal(err)
	}
	if _, err := deploy.ResolveDeployRun(parsed, []string{"R-1", "R-2"}); !errors.Is(err, core.ErrSettings) {
		t.Fatal(err)
	}
	one, err := deploy.ResolveDeployRun(parsed, []string{"R-1"})
	if err != nil || one.RunID != "R-1" {
		t.Fatal(one, err)
	}
	one, err = deploy.ApplyDeployDefaults(one, "staging", "staging")
	if err != nil || one.Target != "staging" || one.ProfileID != "staging" {
		t.Fatal(one, err)
	}
	explicit := parsed
	explicit.RunID = "R-2"
	if _, err := deploy.ResolveDeployRun(explicit, []string{"R-1"}); !errors.Is(err, core.ErrSettings) {
		t.Fatal(err)
	}
}

func TestDeploymentDefaultsAndRunResolution(t *testing.T) {
	base, err := deploy.ParseDeployArgs([]string{"deploy", "--resume"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = deploy.ApplyDeployDefaults(base, "production", "staging"); !errors.Is(err, core.ErrSettings) {
		t.Fatal(err)
	}
	if _, err = deploy.ApplyDeployDefaults(base, "", "staging"); !errors.Is(err, core.ErrSettings) {
		t.Fatal(err)
	}
	if _, err = deploy.ApplyDeployDefaults(base, "staging", ""); !errors.Is(err, core.ErrSettings) {
		t.Fatal(err)
	}
}

type verticalProvider struct {
	calls   []string
	unknown bool
}

func (p *verticalProvider) Submit(context.Context, contracts.DeploymentBatch, string) (contracts.Operation, error) {
	p.calls = append(p.calls, "submit")
	if p.unknown {
		return contracts.Operation{Provider: "fake", ProviderID: "op-vertical", State: "unknown", Unknown: true}, errors.New("transport")
	}
	return contracts.Operation{Provider: "fake", ProviderID: "op-vertical", State: "submitted"}, nil
}
func (p *verticalProvider) Query(context.Context, contracts.Operation, string) (contracts.Operation, error) {
	p.calls = append(p.calls, "query")
	return contracts.Operation{Provider: "fake", ProviderID: "op-vertical", State: "succeeded"}, nil
}
func (p *verticalProvider) Verify(context.Context, contracts.Operation, string) (contracts.Verification, error) {
	p.calls = append(p.calls, "verify")
	return contracts.Verification{State: "failed", OutputPointer: "memory:vertical"}, errors.New("verification failed")
}

type verticalBlockers struct{ items []knowledge.Blocker }

func (b *verticalBlockers) Create(_ context.Context, value knowledge.Blocker) (knowledge.Blocker, error) {
	value.ID = "vertical-blocker"
	b.items = append(b.items, value)
	return value, nil
}

type verticalHolds struct{ calls []string }

func (h *verticalHolds) HoldLater(_ context.Context, _ core.RunID, batch string) error {
	h.calls = append(h.calls, batch)
	return nil
}

type verticalDashboard struct{}

func (verticalDashboard) Trigger(context.Context, deploy.DashboardTriggerRequest) error {
	return errors.New("dashboard unavailable")
}

func TestDeploymentVerticalRecovery(t *testing.T) {
	profile := deploy.TargetProfile{ID: "staging", Target: "staging", AuthorizationRef: "approval-1", ApprovalScope: "staging", ExecutorCommand: []string{"provider"}, QueryCommand: []string{"provider-query"}, VerificationCommand: []string{"provider-verify"}, DefaultBatchSize: 3, Enabled: true}
	profile.ProfileDigest, _ = deploy.DeriveProfileDigest(profile)
	tasks := make([]deploy.EligibleTask, 7)
	for i := range tasks {
		tasks[i] = deploy.EligibleTask{ID: core.TaskID(fmt.Sprintf("T-%d", i)), CanonicalRevision: fmt.Sprintf("rev-%d", i), ArtifactDigests: []string{fmt.Sprintf("artifact-%d", i)}, Integrated: true, GatePassed: true, DependenciesComplete: true}
	}
	batches, err := deploy.PlanBatches(context.Background(), deploy.BatchRequest{RunID: "R-vertical", Target: "staging", Profile: profile, Tasks: tasks, BatchSize: 3})
	if err != nil || len(batches) != 3 || len(batches[0].TaskIDs) != 3 || len(batches[1].TaskIDs) != 3 || len(batches[2].TaskIDs) != 1 {
		t.Fatal(len(batches), err)
	}
	provider := &verticalProvider{unknown: true}
	repository := deploy.NewMemoryRepository()
	executor, err := deploy.NewBoundExecutor(profile, provider)
	if err != nil {
		t.Fatal(err)
	}
	first, err := deploy.SubmitOrReconcile(context.Background(), repository, executor, batches[0])
	if err == nil || first.Receipt.State != "unknown" || !first.CodingMayContinue {
		t.Fatal(first, err)
	}
	resumed, err := deploy.ResumeBatch(context.Background(), repository, executor, batches[0].BatchID)
	if err == nil || resumed.Receipt.State != "failed" || !resumed.CodingMayContinue || !reflect.DeepEqual(provider.calls, []string{"submit", "query", "verify"}) {
		t.Fatal(resumed, err, provider.calls)
	}
	blockers, holds := &verticalBlockers{}, &verticalHolds{}
	id, err := (deploy.RecoveryService{Blockers: blockers, Holds: holds, Dashboard: verticalDashboard{}}).RecordState(context.Background(), resumed.Receipt)
	if err != nil || id == "" || len(blockers.items) != 1 || len(holds.calls) != 1 || len(repository.Evidence()) < 2 {
		t.Fatal(id, err, blockers, holds)
	}
}
