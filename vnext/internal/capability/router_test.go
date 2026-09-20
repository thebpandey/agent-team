package capability_test

import (
	"context"
	"testing"

	capability "github.com/thebpandey/agent-team/vnext/internal/capability"
	"github.com/thebpandey/agent-team/vnext/internal/tracker"
)

type adapterRunner struct{ calls [][]string }

func (r *adapterRunner) Run(_ context.Context, argv, _ []string) tracker.CommandResult {
	r.calls = append(r.calls, append([]string(nil), argv...))
	return tracker.CommandResult{Stdout: []byte("bounded result")}
}

type outputStore struct{ writes int }

func (s *outputStore) Write(_ context.Context, _ string, _ []byte, _ int64) (string, error) {
	s.writes++
	return "memory:raw", nil
}

type tokens map[string]int

func (s tokens) Read(_ context.Context, key string) (int, error) { return s[key], nil }

type resultWriter struct{ results []capability.Result }

func (w *resultWriter) WriteCapabilityResult(_ context.Context, result capability.Result) error {
	w.results = append(w.results, result)
	return nil
}

func TestRouterAndPlaywright(t *testing.T) {
	q := capability.PlaywrightQuestion{Question: capability.Question{Task: "T-1", Prompt: "capture UI"}, SessionName: "ui-1", ResourceID: "browser-1", TargetURL: "http://127.0.0.1:3000", Viewport: "1280x800"}
	got, err := capability.RoutePlaywright(q, []capability.BrowserResource{{ID: "browser-1", Session: "ui-1", State: "ready", Ownership: "managed"}})
	if err != nil || got.ResourceID != "browser-1" {
		t.Fatal(got, err)
	}
	router := capability.NewRouter(&adapterRunner{}, &outputStore{}, tokens{}, &resultWriter{})
	route, err := router.Select(capability.Question{Task: "T-1", Prompt: "find callers", SecondQuestion: "which tests cover this?"}, []capability.Probe{{Name: capability.Serena, Available: true, Healthy: true}})
	if err != nil || route.Primary != capability.Serena || route.Secondary == nil {
		t.Fatal(route, err)
	}
}

func TestAdaptersAndForbiddenRoutes(t *testing.T) {
	runner, output := &adapterRunner{}, &outputStore{}
	q := capability.Question{Task: "T-1", Worktree: "wt", Revision: "git-1", Prompt: "inspect"}
	if _, err := capability.RunLeanCTX(context.Background(), runner, output, tokens{"before": 10, "after": 4}, q); err != nil {
		t.Fatal(err)
	}
	if _, err := capability.RunGraphify(context.Background(), runner, output, q); err != nil {
		t.Fatal(err)
	}
	consent := capability.Consent{Name: capability.Serena, Enabled: true, Mode: capability.ReadOnlyMCP}
	if err := capability.AllowSerena(consent, capability.SerenaRequest{Operation: capability.SerenaFindDefinition, ReadOnly: false}); err == nil {
		t.Fatal("non-read-only Serena request accepted")
	}
	if _, err := capability.RoutePlaywright(capability.PlaywrightQuestion{Question: q, SessionName: "ui-1", ResourceID: "browser-1", TargetURL: "http://127.0.0.1:3000", Viewport: "1280x800"}, nil); err == nil {
		t.Fatal("unowned browser route accepted")
	}
	router := capability.NewRouter(runner, output, tokens{}, &resultWriter{})
	route, err := router.Select(capability.Question{Task: "T-1", Prompt: "find callers"}, []capability.Probe{{Name: capability.Serena, Available: true, Healthy: true}})
	if err != nil || route.Secondary != nil {
		t.Fatal("unnamed second question was admitted", route, err)
	}
}
