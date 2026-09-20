package bench

import (
	"fmt"
	"strings"

	"github.com/thebpandey/agent-team/vnext/internal/core"
)

type Report struct {
	Version, Commit                               string
	Baseline, Optional                            []Measurement
	MeanTokenDelta, MeanDurationDelta             float64
	QualityRegressions, MissingCounters, Compared int
	RawEvidence                                   []string
}

func BuildReport(base, optional []Measurement, version, commit string) (Report, error) {
	if version == "" || commit == "" || len(base) == 0 || len(base) != len(optional) {
		return Report{}, core.ErrRevision
	}
	byFixture := map[string]Measurement{}
	pointers := map[string]bool{}
	report := Report{Version: version, Commit: commit, Baseline: append([]Measurement(nil), base...), Optional: append([]Measurement(nil), optional...)}
	for _, measurement := range base {
		if invalidMeasurement(measurement, "native") || byFixture[measurement.Fixture].Fixture != "" || pointers[measurement.RawOutputPointer] {
			return Report{}, core.ErrRevision
		}
		byFixture[measurement.Fixture] = measurement
		pointers[measurement.RawOutputPointer] = true
		report.RawEvidence = append(report.RawEvidence, measurement.RawOutputPointer)
	}
	seenOptional := map[string]bool{}
	var tokenDelta, durationDelta int64
	for _, measurement := range optional {
		if invalidMeasurement(measurement, "optional") || seenOptional[measurement.Fixture] || pointers[measurement.RawOutputPointer] {
			return Report{}, core.ErrRevision
		}
		baseline, ok := byFixture[measurement.Fixture]
		if !ok {
			return Report{}, core.ErrRevision
		}
		seenOptional[measurement.Fixture] = true
		pointers[measurement.RawOutputPointer] = true
		report.RawEvidence = append(report.RawEvidence, measurement.RawOutputPointer)
		report.Compared++
		durationDelta += measurement.DurationMillis - baseline.DurationMillis
		if measurement.QualityScore < baseline.QualityScore {
			report.QualityRegressions++
		}
		if measurement.TokenSource == "counter" && baseline.TokenSource == "counter" {
			tokenDelta += int64((measurement.TokensAfter - measurement.TokensBefore) - (baseline.TokensAfter - baseline.TokensBefore))
		} else {
			report.MissingCounters++
		}
	}
	if report.Compared != len(base) {
		return Report{}, core.ErrRevision
	}
	report.MeanTokenDelta = float64(tokenDelta) / float64(report.Compared)
	report.MeanDurationDelta = float64(durationDelta) / float64(report.Compared)
	return report, nil
}

func RenderReport(report Report) ([]byte, error) {
	if report.Version == "" || report.Commit == "" || report.Compared == 0 || len(report.RawEvidence) != report.Compared*2 {
		return nil, core.ErrRevision
	}
	var out strings.Builder
	fmt.Fprintf(&out, "# Agent-Team vNext Optional Capability Benchmark\n\nVersion: `%s`  \nCommit: `%s`\n\n", report.Version, report.Commit)
	fmt.Fprintf(&out, "| Compared pairs | Mean token delta | Mean duration delta (ms) | Quality regressions | Missing token counters |\n|---:|---:|---:|---:|---:|\n| %d | %.2f | %.2f | %d | %d |\n\n", report.Compared, report.MeanTokenDelta, report.MeanDurationDelta, report.QualityRegressions, report.MissingCounters)
	out.WriteString("Token deltas are zero when either side lacks a real counter; they are never estimated. Raw evidence remains outside release archives.\n\n## Raw evidence pointers\n\n")
	for _, pointer := range report.RawEvidence {
		if pointer == "" || strings.ContainsAny(pointer, "\r\n") {
			return nil, core.ErrRevision
		}
		fmt.Fprintf(&out, "- `%s`\n", pointer)
	}
	return []byte(out.String()), nil
}

func invalidMeasurement(measurement Measurement, mode string) bool {
	return measurement.Mode != mode || measurement.Fixture == "" || measurement.RawOutputPointer == "" || strings.ContainsAny(measurement.RawOutputPointer, "\r\n") || measurement.DurationMillis < 0
}
