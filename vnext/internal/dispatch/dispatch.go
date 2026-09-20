// Package dispatch validates immutable assignment packets before handing them
// to a host adapter.
package dispatch

import (
	"context"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/thebpandey/agent-team/vnext/internal/contracts"
	"github.com/thebpandey/agent-team/vnext/internal/core"
	"github.com/thebpandey/agent-team/vnext/internal/lifecycle"
	"github.com/thebpandey/agent-team/vnext/internal/store"
)

// Dispatcher starts one worker from a validated, immutable request.
type Dispatcher interface {
	Dispatch(context.Context, core.AssignmentPacket, contracts.WorktreeSpec) (contracts.WorkerHandle, error)
}

type dispatcher struct {
	adapter contracts.HostAdapter
	state   *store.Store
}

// NewDispatcher returns a dispatcher backed by adapter.
func NewDispatcher(state *store.Store, adapter contracts.HostAdapter) Dispatcher {
	return &dispatcher{state: state, adapter: adapter}
}

func (d *dispatcher) Dispatch(ctx context.Context, packet core.AssignmentPacket, worktree contracts.WorktreeSpec) (contracts.WorkerHandle, error) {
	if d == nil || d.adapter == nil || d.state == nil {
		return contracts.WorkerHandle{}, core.ErrCapacity
	}
	if err := ValidatePacket(packet, worktree); err != nil {
		return contracts.WorkerHandle{}, err
	}
	request := contracts.WorkerRequest{
		Packet:        copyPacket(packet),
		Worktree:      copyWorktreeSpec(worktree),
		WritablePaths: append([]string(nil), worktree.WritablePaths...),
		Reviewer:      false,
	}
	handle, err := lifecycle.StartWorkerAllowed(ctx, d.state, packet, d.adapter, request)
	if err != nil {
		return contracts.WorkerHandle{}, err
	}
	if handle.Identity == "" || handle.Reviewer ||
		handle.Run != packet.RunID || handle.Team != packet.Team ||
		handle.Task != packet.Task || handle.PacketDigest != packet.QueueFingerprint ||
		handle.CandidateRevision != packet.SpecRevision {
		return contracts.WorkerHandle{}, core.ErrRevision
	}
	return handle, nil
}

// ValidatePacket verifies the packet/worktree binding before any Git command
// or adapter call. It intentionally does not decide which repository owns the
// root; that manager-specific control is enforced by worktree.Manager.
func ValidatePacket(packet core.AssignmentPacket, worktree contracts.WorktreeSpec) error {
	if !safeID(string(packet.RunID)) || !safeID(string(packet.Team)) ||
		worktree.Run != packet.RunID || worktree.Team != packet.Team ||
		packet.Base == "" || worktree.Base != packet.Base || !safeGitAtom(packet.Base) ||
		worktree.Root == "" || !filepath.IsAbs(worktree.Root) || len(worktree.WritablePaths) == 0 {
		return core.ErrPath
	}
	root, err := canonicalProspective(worktree.Root)
	if err != nil || root == filepath.Clean(".") {
		return core.ErrPath
	}
	if packet.Worktree != "" {
		packetRoot, err := canonicalProspective(packet.Worktree)
		if err != nil || packetRoot != root {
			return core.ErrPath
		}
	}
	for _, raw := range worktree.WritablePaths {
		if raw == "" {
			return core.ErrPath
		}
		candidate := raw
		if !filepath.IsAbs(candidate) {
			candidate = filepath.Join(root, candidate)
		}
		candidate, err = canonicalCandidate(candidate)
		if err != nil || !strictDescendant(root, candidate) {
			return core.ErrPath
		}
	}
	return nil
}

func copyPacket(packet core.AssignmentPacket) core.AssignmentPacket {
	packet.Criteria = append([]string(nil), packet.Criteria...)
	packet.Scope = append([]string(nil), packet.Scope...)
	packet.Checks = append([]core.Check(nil), packet.Checks...)
	for i := range packet.Checks {
		packet.Checks[i].Command = append([]string(nil), packet.Checks[i].Command...)
	}
	packet.Capabilities = append([]string(nil), packet.Capabilities...)
	packet.Skills = append([]core.SkillRef(nil), packet.Skills...)
	packet.Resources.Servers = append([]string(nil), packet.Resources.Servers...)
	packet.Resources.Browsers = append([]string(nil), packet.Resources.Browsers...)
	packet.Resources.External = append([]string(nil), packet.Resources.External...)
	packet.Accelerators = append([]core.AcceleratorRef(nil), packet.Accelerators...)
	return packet
}

func copyWorktreeSpec(spec contracts.WorktreeSpec) contracts.WorktreeSpec {
	spec.WritablePaths = append([]string(nil), spec.WritablePaths...)
	return spec
}

func canonicalProspective(path string) (string, error) {
	abs, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return "", err
	}
	if real, err := filepath.EvalSymlinks(abs); err == nil {
		return real, nil
	}
	return canonicalCandidate(abs)
}

// canonicalCandidate resolves every existing path component. This permits a
// prospective worktree while still rejecting a writable path through an
// existing symlink.
func canonicalCandidate(path string) (string, error) {
	abs, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return "", err
	}
	for probe := abs; ; probe = filepath.Dir(probe) {
		if real, err := filepath.EvalSymlinks(probe); err == nil {
			suffix, err := filepath.Rel(probe, abs)
			if err != nil {
				return "", err
			}
			return filepath.Join(real, suffix), nil
		}
		if parent := filepath.Dir(probe); parent == probe {
			return "", core.ErrPath
		}
	}
}

func strictDescendant(root, candidate string) bool {
	rel, err := filepath.Rel(root, candidate)
	if err != nil || rel == "." || rel == ".." {
		return false
	}
	return !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func safeID(value string) bool {
	if value == "" || value == "." || value == ".." || strings.ContainsAny(value, `/\\`) {
		return false
	}
	for _, r := range value {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}

func safeGitAtom(value string) bool {
	if value == "" || strings.HasPrefix(value, "-") {
		return false
	}
	for _, r := range value {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}
