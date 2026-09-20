# Agent-Team vNext Optional Capability Benchmark

The executable benchmark covers 24 paired fixtures across discovery, test output, semantic navigation, refactoring, blast radius, parallel work, UI work, and recovery. The native path is the baseline; optional routes are compared against it without changing the required quality score.

| Compared pairs | Token result | Quality threshold | Raw evidence |
|---:|---|---|---|
| 24 | Unknown counters produce a zero delta; no estimate is substituted | No regression accepted | Emitted by `go test ./internal/bench -run TestReleaseReport` and retained outside release archives |

`BuildReport` rejects duplicate, missing, unmatched, or reused evidence pointers. `RenderReport` records the exact revision and lists all 48 raw pointers. Duration and token claims must be generated for the release revision; this checked-in guide does not present host-dependent fixture timings as universal performance results.
