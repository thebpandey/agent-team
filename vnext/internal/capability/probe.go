package capability

import (
	"context"
	"fmt"
	"strings"
	"time"
)

const (
	operationTimeout = 30 * time.Second
	probeOutputLimit = 8 << 10
)

// ProbeAll discovers only recognised optional capability names. A failed probe
// is diagnostic, so callers can retain their native fallback.
func ProbeAll(ctx context.Context, runner NativeRunner, names []Name) ([]Probe, error) {
	if ctx == nil || runner == nil {
		return nil, fmt.Errorf("capability probe requires context and runner")
	}
	probes := make([]Probe, 0, len(names))
	seen := make(map[Name]struct{}, len(names))
	for _, name := range names {
		if !known(name) {
			return nil, fmt.Errorf("unknown capability %q", name)
		}
		if _, ok := seen[name]; ok {
			return nil, fmt.Errorf("duplicate capability %q", name)
		}
		seen[name] = struct{}{}
		mode := modeFor(name)
		if name == Native {
			probes = append(probes, Probe{Name: name, Mode: mode, Available: true, Healthy: true, Version: "native"})
			continue
		}
		callCtx, cancel := context.WithTimeout(ctx, operationTimeout)
		result := runner.Run(callCtx, string(name), "--version")
		cancel()
		probe := Probe{Name: name, Mode: mode, Path: string(name)}
		if result.Transport != nil || result.TimedOut || result.Exit != 0 {
			probe.Reason = commandReason(result)
			probes = append(probes, probe)
			continue
		}
		version := boundedText(result.Stdout)
		if version == "" {
			probe.Reason = "empty version output"
			probes = append(probes, probe)
			continue
		}
		probe.Version, probe.Available, probe.Healthy = version, true, true
		probes = append(probes, probe)
	}
	return probes, nil
}

func known(name Name) bool {
	switch name {
	case Native, UsingSuperpowers, LeanCTX, Serena, Graphify, AstGrep, Playwright, Impeccable, UIUXProMax, UIStyling:
		return true
	default:
		return false
	}
}

func modeFor(name Name) Mode {
	switch name {
	case Serena:
		return ReadOnlyMCP
	case UsingSuperpowers, Impeccable, UIUXProMax, UIStyling:
		return SkillContent
	default:
		return CLI
	}
}

func boundedText(data []byte) string {
	if len(data) > probeOutputLimit {
		data = data[:probeOutputLimit]
	}
	return strings.TrimSpace(string(data))
}

func commandReason(result NativeResult) string {
	switch {
	case result.TimedOut:
		return "probe timed out"
	case result.Transport != nil:
		return "probe transport failed"
	case result.Exit != 0:
		return fmt.Sprintf("probe exited %d", result.Exit)
	default:
		return "probe failed"
	}
}
