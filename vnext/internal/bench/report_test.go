package bench_test

import (
	"context"
	"strings"
	"testing"

	"github.com/thebpandey/agent-team/vnext/internal/bench"
)

func TestReleaseReport(t *testing.T) {
	measurements, err := bench.Run(context.Background(), bench.Fixtures(), benchmarkRunner{}, benchmarkRouter{}, benchmarkTokens{})
	if err != nil || len(measurements) != 48 {
		t.Fatalf("measurements=%d err=%v", len(measurements), err)
	}
	base, optional := make([]bench.Measurement, 0, 24), make([]bench.Measurement, 0, 24)
	for _, measurement := range measurements {
		if measurement.Mode == "native" {
			base = append(base, measurement)
		} else if measurement.Mode == "optional" {
			optional = append(optional, measurement)
		}
	}
	report, err := bench.BuildReport(base, optional, "8.0.0", "abc")
	if err != nil || report.Compared != 24 || len(report.RawEvidence) != 48 || report.QualityRegressions != 0 || report.MissingCounters != 24 || report.MeanTokenDelta != 0 {
		t.Fatal(report, err)
	}
	raw, err := bench.RenderReport(report)
	if err != nil || !strings.Contains(string(raw), "Agent-Team vNext Optional Capability Benchmark") || !strings.Contains(string(raw), "Raw evidence pointers") {
		t.Fatal(string(raw), err)
	}
	regression := optional[0]
	regression.QualityScore = 4
	report, err = bench.BuildReport(base[:1], []bench.Measurement{regression}, "8.0.0", "abc")
	if err != nil || report.QualityRegressions != 1 {
		t.Fatal(report, err)
	}
	duplicate := append([]bench.Measurement(nil), base...)
	duplicate[1] = duplicate[0]
	if _, err := bench.BuildReport(duplicate, optional, "8.0.0", "abc"); err == nil {
		t.Fatal("duplicate baseline accepted")
	}
	unmatched := append([]bench.Measurement(nil), optional...)
	unmatched[0].Fixture = "not-a-fixture"
	if _, err := bench.BuildReport(base, unmatched, "8.0.0", "abc"); err == nil {
		t.Fatal("unmatched optional accepted")
	}
	if _, err := bench.BuildReport(base[:23], optional, "8.0.0", "abc"); err == nil {
		t.Fatal("missing pair accepted")
	}
}
