package capability

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/thebpandey/agent-team/vnext/internal/core"
	"github.com/thebpandey/agent-team/vnext/internal/tracker"
)

const (
	operationTimeout = 30 * time.Second
	probeOutputLimit = 8 << 10
)

// ProbeAll only errors for malformed requests. Missing or unhealthy optional
// capabilities are returned as diagnostic probes so native fallback continues.
func ProbeAll(ctx context.Context, runner tracker.CommandRunner, names []Name) ([]Probe, error) {
	if ctx == nil {
		return nil, fmt.Errorf("capability probe requires context")
	}
	seen := make(map[Name]struct{}, len(names))
	probes := make([]Probe, 0, len(names))
	for _, name := range names {
		if !known(name) {
			return nil, fmt.Errorf("unknown capability %q", name)
		}
		if _, exists := seen[name]; exists {
			return nil, fmt.Errorf("duplicate capability %q", name)
		}
		seen[name] = struct{}{}
		if name == Native {
			probes = append(probes, Probe{Name: Native, Mode: CLI, Source: "native", Version: "native", Available: true, Healthy: true})
			continue
		}
		if modeFor(name) == SkillContent {
			probes = append(probes, probeSkill(name))
			continue
		}
		probes = append(probes, probeExecutable(ctx, runner, name))
	}
	return probes, nil
}

func probeExecutable(ctx context.Context, runner tracker.CommandRunner, name Name) Probe {
	path, err := exec.LookPath(string(name))
	if err != nil {
		return Probe{Name: name, Mode: modeFor(name), Reason: "executable unavailable"}
	}
	p := Probe{Name: name, Mode: modeFor(name), Path: path, Source: "verified:" + path}
	if runner == nil {
		p.Reason = "probe runner unavailable"
		return p
	}
	callCtx, cancel := context.WithTimeout(ctx, operationTimeout)
	defer cancel()
	result := runner.Run(callCtx, path, "--version")
	if failed(result) {
		p.Reason = commandReason(result)
		return p
	}
	p.Version = boundedText(result.Stdout)
	if p.Version == "" {
		p.Reason = "empty version output"
		return p
	}
	p.Available, p.Healthy = true, true
	return p
}

func probeSkill(name Name) Probe {
	root := os.Getenv("AGENT_TEAM_SKILL_ROOT")
	path := filepath.Join(root, string(name), "SKILL.md")
	p := Probe{Name: name, Mode: SkillContent, Path: path, Source: "verified:" + path}
	if root == "" {
		p.Reason = "skill root unavailable"
		return p
	}
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() > core.DefaultConfig().Storage.CanonicalBytes {
		p.Reason = "skill content unavailable"
		return p
	}
	data, err := os.ReadFile(path)
	if err != nil {
		p.Reason = "skill content unreadable"
		return p
	}
	sum := sha256.Sum256(data)
	p.Digest = "sha256:" + hex.EncodeToString(sum[:])
	p.Available, p.Healthy = true, true
	return p
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
		return "command timed out"
	case result.Transport != nil:
		return "command transport failed"
	case result.Exit != 0:
		return fmt.Sprintf("command exited %d", result.Exit)
	default:
		return "command failed"
	}
}
