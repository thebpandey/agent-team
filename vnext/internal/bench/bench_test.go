package bench_test

import (
	"context"
	"strings"
	"testing"

	"github.com/thebpandey/agent-team/vnext/internal/bench"
	"github.com/thebpandey/agent-team/vnext/internal/capability"
	"github.com/thebpandey/agent-team/vnext/internal/tracker"
)

type benchmarkRouter struct{}

func (benchmarkRouter) Select(capability.Question, []capability.Probe) (capability.Route, error) {
	return capability.Route{Primary: capability.Serena, NativeFallback: []string{"rg", "git"}}, nil
}

func (benchmarkRouter) Execute(_ context.Context, _ capability.Route, q capability.Question) (capability.Result, error) {
	return capability.Result{Name: capability.Serena, Worktree: q.Worktree, Revision: q.Revision, Used: true, Summary: "bounded", RawOutputPointer: "memory:optional/" + string(q.Task), DurationMillis: 1, TokensBefore: 100, TokensAfter: 60}, nil
}

type benchmarkRunner struct{}

func (benchmarkRunner) Run(context.Context, []string, []string) tracker.CommandResult {
	return tracker.CommandResult{Stdout: []byte("bounded")}
}

type benchmarkTokens struct{}

func (benchmarkTokens) Read(_ context.Context, key string) (int, error) {
	if key == "before" {
		return 100, nil
	}
	return 60, nil
}

func TestFixturesAndMeasurements(t *testing.T) {
	fixtures := bench.Fixtures()
	if len(fixtures) != 24 {
		t.Fatalf("fixtures=%d", len(fixtures))
	}
	wantKinds := map[string]bool{"discovery": true, "test-output": true, "semantic": true, "refactor": true, "blast-radius": true, "parallel": true, "ui": true, "recovery": true}
	seen := map[string]bool{}
	for _, fixture := range fixtures {
		if seen[fixture.Name] || !wantKinds[fixture.Kind] || fixture.InputPath == "" {
			t.Fatalf("bad fixture: %+v", fixture)
		}
		seen[fixture.Name] = true
	}
	got, err := bench.Run(context.Background(), fixtures, benchmarkRunner{}, benchmarkRouter{}, benchmarkTokens{})
	if err != nil || len(got) != 48 {
		t.Fatalf("measurements=%d want=48 err=%v", len(got), err)
	}
	counts := map[string]int{}
	pointers := map[string]map[string]bool{}
	routes := map[string]map[string]bool{}
	for _, measurement := range got {
		if measurement.RawOutputPointer == "" || measurement.InputBytes <= 0 || measurement.OutputBytes <= 0 || measurement.TokensBefore < measurement.TokensAfter {
			t.Fatalf("incomplete measurement: %+v", measurement)
		}
		if strings.Contains(measurement.RawOutputPointer, "secret") {
			t.Fatal("secret pointer")
		}
		if pointers[measurement.Fixture] == nil {
			pointers[measurement.Fixture] = map[string]bool{}
			routes[measurement.Fixture] = map[string]bool{}
		}
		if pointers[measurement.Fixture][measurement.RawOutputPointer] {
			t.Fatalf("duplicate raw pointer: %s", measurement.RawOutputPointer)
		}
		pointers[measurement.Fixture][measurement.RawOutputPointer] = true
		routes[measurement.Fixture][measurement.Route] = true
		counts[measurement.Fixture+":"+measurement.Mode]++
		if measurement.Mode == "native" && (measurement.Route != "native" || measurement.TokenSource != "unknown") {
			t.Fatal("native baseline mislabeled")
		}
		if measurement.Mode == "optional" && (measurement.Route == "native" || measurement.TokenSource != "counter") {
			t.Fatal("optional route mislabeled")
		}
	}
	for _, fixture := range fixtures {
		if counts[fixture.Name+":native"] != 1 || counts[fixture.Name+":optional"] != 1 || len(pointers[fixture.Name]) != 2 || len(routes[fixture.Name]) != 2 {
			t.Fatalf("fixture %s lacks distinct baseline/optional pair", fixture.Name)
		}
	}
	summary := bench.Compare(got)
	if summary.Fixtures != 24 || summary.QualityRegressions != 0 {
		t.Fatalf("summary=%+v", summary)
	}
}
