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
)

const (
	probeOutputLimit  = 8 << 10
	skillContentLimit = 16 << 20
	probeTimeout      = 30 * time.Second
)

// ProbeAll reports unavailable optional tools as unhealthy probes; only an
// invalid request is an error, preserving the native fallback.
func ProbeAll(ctx context.Context, runner NativeRunner, names []Name) ([]Probe, error) {
	if ctx == nil {
		return nil, fmt.Errorf("capability probe requires context")
	}
	seen := make(map[Name]struct{}, len(names))
	probes := make([]Probe, 0, len(names))
	for _, name := range names {
		if !known(name) {
			return nil, fmt.Errorf("unknown capability %q", name)
		}
		if _, duplicate := seen[name]; duplicate {
			return nil, fmt.Errorf("duplicate capability %q", name)
		}
		seen[name] = struct{}{}
		switch {
		case name == Native:
			probes = append(probes, Probe{Name: Native, Mode: CLI, Version: "native", Available: true, Healthy: true})
		case modeFor(name) == SkillContent:
			probes = append(probes, probeSkill(name))
		default:
			probes = append(probes, probeExecutable(ctx, runner, name))
		}
	}
	return probes, nil
}

func probeExecutable(ctx context.Context, runner NativeRunner, name Name) Probe {
	path, err := exec.LookPath(string(name))
	if err != nil {
		return Probe{Name: name, Mode: modeFor(name), Reason: "executable unavailable"}
	}
	p := Probe{Name: name, Mode: modeFor(name), Path: path}
	if runner == nil {
		p.Reason = "probe runner unavailable"
		return p
	}
	data, err := readRegular(path, skillContentLimit)
	if err != nil {
		p.Reason = "executable unreadable"
		return p
	}
	p.Digest = sha256Digest(data)
	callCtx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	result := runner.Run(callCtx, []string{path, "--version"}, []string{})
	if failed(result) {
		p.Reason = commandReason(result)
		return p
	}
	if len(result.Stdout) > probeOutputLimit {
		p.Reason = "version output too large"
		return p
	}
	p.Version = strings.TrimSpace(string(result.Stdout))
	if p.Version == "" {
		p.Reason = "empty version output"
		return p
	}
	after, err := readRegular(path, skillContentLimit)
	if err != nil {
		p.Reason = "executable unreadable"
		return p
	}
	if sha256Digest(after) != p.Digest {
		p.Reason = "executable changed during probe"
		return p
	}
	p.Available, p.Healthy = true, true
	return p
}

func probeSkill(name Name) Probe {
	root := os.Getenv("AGENT_TEAM_SKILL_ROOT")
	if root == "" {
		return Probe{Name: name, Mode: SkillContent, Reason: "skill root unavailable"}
	}
	path := filepath.Join(root, string(name), "SKILL.md")
	p := Probe{Name: name, Mode: SkillContent, Path: path}
	data, err := readRegular(path, skillContentLimit)
	if err != nil {
		p.Reason = "skill content unavailable"
		return p
	}
	p.Digest = sha256Digest(data)
	p.Version = p.Digest
	p.Available, p.Healthy = true, true
	return p
}

func readRegular(path string, limit int64) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() > limit {
		return nil, fmt.Errorf("not a bounded regular file")
	}
	return os.ReadFile(path)
}

func sha256Digest(data []byte) string {
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func known(name Name) bool {
	switch name {
	case Native, UsingSuperpowers, LeanCTX, Serena, Graphify, Playwright, Impeccable, UIUXProMax, UIStyling:
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
