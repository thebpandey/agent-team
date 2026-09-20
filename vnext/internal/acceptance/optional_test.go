package acceptance

import (
	"context"
	"errors"
	"testing"

	"github.com/thebpandey/agent-team/vnext/internal/capability"
	"github.com/thebpandey/agent-team/vnext/internal/core"
	"github.com/thebpandey/agent-team/vnext/internal/dashboard"
	"github.com/thebpandey/agent-team/vnext/internal/resources"
	"github.com/thebpandey/agent-team/vnext/internal/store"
	"github.com/thebpandey/agent-team/vnext/internal/tracker"
)

type optionalRunner struct{}

func (optionalRunner) Run(context.Context, []string, []string) tracker.CommandResult {
	return tracker.CommandResult{Stdout: []byte("ok")}
}

type optionalOutput struct{}

func (optionalOutput) Write(context.Context, string, []byte, int64) (string, error) {
	return "memory:optional-output", nil
}

type optionalTokens struct{}

func (optionalTokens) Read(_ context.Context, key string) (int, error) {
	if key == "before" {
		return 10, nil
	}
	return 5, nil
}

type optionalResults struct{ values []capability.Result }

func (r *optionalResults) WriteCapabilityResult(_ context.Context, value capability.Result) error {
	r.values = append(r.values, value)
	return nil
}

type optionalRenderer struct{ status dashboard.DashboardStatus }

func (r *optionalRenderer) Publish(_ context.Context, snapshot dashboard.Snapshot) error {
	r.status = snapshot.Status
	return nil
}

type optionalReceipts struct {
	values []dashboard.DashboardRefreshReceipt
}

func (r *optionalReceipts) WriteRefreshReceipt(_ context.Context, value dashboard.DashboardRefreshReceipt) error {
	r.values = append(r.values, value)
	return nil
}

func TestOptionalAcceptance(t *testing.T) {
	for _, tc := range []struct {
		name     string
		probe    capability.Probe
		fallback bool
	}{
		{"missing", capability.Probe{}, true},
		{"unhealthy", capability.Probe{Name: capability.Serena, Available: true}, true},
		{"healthy", capability.Probe{Name: capability.Serena, Available: true, Healthy: true}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			results := &optionalResults{}
			router := capability.NewRouter(optionalRunner{}, optionalOutput{}, optionalTokens{}, results)
			got, err := router.Select(capability.Question{Task: "T-1", Prompt: "inspect"}, []capability.Probe{tc.probe})
			if err != nil || got.Primary == "" || got.NativeFallback == nil {
				t.Fatal(got, err)
			}
			if tc.fallback && got.Primary != capability.Native {
				t.Fatal("optional route did not fall back to native", got)
			}
			if !tc.fallback && got.Primary != capability.Serena {
				t.Fatal("healthy optional route not selected", got)
			}
			if tc.fallback {
				result, err := router.Execute(context.Background(), got, capability.Question{Task: "T-1", Prompt: "inspect"})
				if err != nil || !result.Fallback || len(results.values) != 1 || !results.values[0].Fallback {
					t.Fatal("native fallback receipt missing", result, results.values, err)
				}
			}
		})
	}

	registry := resources.NewRegistry(store.New(t.TempDir(), core.StorageLimits{CanonicalBytes: 16 << 20}), func(context.Context, resources.StopRequest) tracker.CommandResult { return tracker.CommandResult{} })
	owner := resources.ResourceOwner{Run: "R", Team: "TEAM", Task: "T", Worktree: "wt", Revision: "rev"}
	first, err := registry.ReserveServer(context.Background(), resources.ServerRecord{ID: "S-1", Port: 3000, Target: "dev", Purpose: "ui", Owner: owner, Ownership: resources.Managed}, 0)
	if err != nil || first.Revision == 0 {
		t.Fatal(first, err)
	}
	if _, err = registry.ReserveServer(context.Background(), resources.ServerRecord{ID: "S-2", Port: 3000, Target: "dev", Purpose: "ui", Owner: owner, Ownership: resources.Managed}, first.Revision); !errors.Is(err, core.ErrCapacity) {
		t.Fatal(err)
	}
	if _, err = registry.Release(context.Background(), "S-1", 0); !errors.Is(err, core.ErrRevision) {
		t.Fatal(err)
	}

	renderer, receipts := &optionalRenderer{}, &optionalReceipts{}
	observer := dashboard.NewIntegrationObserver(renderer, receipts)
	snapshot := dashboard.Snapshot{Schema: 1, Project: "project", RunID: "R", CanonicalRevision: "rev", LastIntegration: "T", Status: dashboard.Current, GeneratedAt: "2026-09-19T00:00:00Z"}
	if err := observer.AfterIntegration(context.Background(), dashboard.IntegrationResult{Success: true, RunID: "R", TaskID: "T", CanonicalRevision: "rev", Revision: 1}, snapshot); err != nil {
		t.Fatal(err)
	}
	if err := observer.AfterIntegration(context.Background(), dashboard.IntegrationResult{Success: false, RunID: "R", TaskID: "T", Error: "refresh", Revision: 2}, snapshot); err != nil || len(receipts.values) != 2 || renderer.status != dashboard.Current || receipts.values[1].Status != dashboard.Stale {
		t.Fatal(err, receipts.values, renderer.status)
	}
}
