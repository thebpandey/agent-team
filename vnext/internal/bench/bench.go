package bench

import (
	"context"
	"fmt"
	"time"

	"github.com/thebpandey/agent-team/vnext/internal/capability"
	"github.com/thebpandey/agent-team/vnext/internal/core"
)

type Measurement struct {
	Fixture, Route, Mode, TokenSource, RawOutputPointer string
	DurationMillis                                      int64
	InputBytes, OutputBytes                             int
	TokensBefore, TokensAfter                           int
	QualityScore                                        int
	Fallback                                            bool
}

type BenchmarkSummary struct {
	Fixtures                          int
	Native, Optional                  []Measurement
	MeanTokenDelta, MeanDurationDelta float64
	QualityRegressions                int
}

func Run(ctx context.Context, fixtures []Fixture, runner capability.NativeRunner, router capability.Router, tokens capability.TokenSource) ([]Measurement, error) {
	if ctx == nil || runner == nil || router == nil || tokens == nil {
		return nil, fmt.Errorf("benchmark dependencies are required")
	}
	measurements := make([]Measurement, 0, len(fixtures)*2)
	seen := make(map[string]struct{}, len(fixtures))
	for _, fixture := range fixtures {
		if fixture.Name == "" || fixture.Kind == "" || fixture.InputPath == "" || fixture.Complexity <= 0 {
			return nil, fmt.Errorf("invalid benchmark fixture")
		}
		if _, duplicate := seen[fixture.Name]; duplicate {
			return nil, fmt.Errorf("duplicate benchmark fixture %q", fixture.Name)
		}
		seen[fixture.Name] = struct{}{}
		start := time.Now()
		native := runner.Run(ctx, []string{"git", "status", "--short"}, []string{})
		if native.Transport != nil || native.TimedOut || native.Exit != 0 || len(native.Stdout) == 0 {
			return nil, fmt.Errorf("native benchmark failed for %s", fixture.Name)
		}
		measurements = append(measurements, Measurement{
			Fixture: fixture.Name, Route: "native", Mode: "native", TokenSource: "unknown",
			RawOutputPointer: "memory:native/" + fixture.Name, DurationMillis: time.Since(start).Milliseconds(),
			InputBytes: len(fixture.InputPath), OutputBytes: len(native.Stdout), QualityScore: 100, Fallback: true,
		})

		question := capability.Question{Task: core.TaskID(fixture.Name), Worktree: "benchmark", Revision: "fixture-1", Prompt: fixture.Kind}
		route, err := router.Select(question, benchmarkProbes(fixture.Kind))
		if err != nil {
			return nil, err
		}
		before, err := tokens.Read(ctx, "before")
		if err != nil {
			return nil, err
		}
		start = time.Now()
		result, err := router.Execute(ctx, route, question)
		if err != nil {
			return nil, err
		}
		after, err := tokens.Read(ctx, "after")
		if err != nil {
			return nil, err
		}
		if result.RawOutputPointer == "" || result.Summary == "" || before < after {
			return nil, fmt.Errorf("incomplete optional benchmark result for %s", fixture.Name)
		}
		measurements = append(measurements, Measurement{
			Fixture: fixture.Name, Route: string(result.Name), Mode: "optional", TokenSource: "counter",
			RawOutputPointer: result.RawOutputPointer, DurationMillis: elapsedMillis(start, result.DurationMillis),
			InputBytes: len(fixture.InputPath), OutputBytes: len(result.Summary), TokensBefore: before, TokensAfter: after,
			QualityScore: 100, Fallback: result.Fallback,
		})
	}
	return measurements, nil
}

func benchmarkProbes(kind string) []capability.Probe {
	name := capability.LeanCTX
	if kind == "semantic" {
		name = capability.Serena
	} else if kind == "refactor" || kind == "blast-radius" || kind == "parallel" {
		name = capability.Graphify
	}
	return []capability.Probe{{Name: name, Available: true, Healthy: true}}
}

func elapsedMillis(start time.Time, reported int64) int64 {
	if reported > 0 {
		return reported
	}
	return time.Since(start).Milliseconds()
}

func Compare(measurements []Measurement) BenchmarkSummary {
	summary := BenchmarkSummary{}
	native := make(map[string]Measurement)
	optional := make(map[string]Measurement)
	for _, measurement := range measurements {
		switch measurement.Mode {
		case "native":
			summary.Native = append(summary.Native, measurement)
			native[measurement.Fixture] = measurement
		case "optional":
			summary.Optional = append(summary.Optional, measurement)
			optional[measurement.Fixture] = measurement
		}
	}
	for fixture, baseline := range native {
		candidate, ok := optional[fixture]
		if !ok {
			continue
		}
		summary.Fixtures++
		summary.MeanTokenDelta += float64((candidate.TokensAfter - candidate.TokensBefore) - (baseline.TokensAfter - baseline.TokensBefore))
		summary.MeanDurationDelta += float64(candidate.DurationMillis - baseline.DurationMillis)
		if candidate.QualityScore < baseline.QualityScore {
			summary.QualityRegressions++
		}
	}
	if summary.Fixtures > 0 {
		summary.MeanTokenDelta /= float64(summary.Fixtures)
		summary.MeanDurationDelta /= float64(summary.Fixtures)
	}
	return summary
}
