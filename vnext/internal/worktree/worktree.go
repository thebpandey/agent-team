// Package worktree owns task worktree lifecycle operations.
package worktree

import (
	"context"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"github.com/thebpandey/agent-team/vnext/internal/contracts"
	"github.com/thebpandey/agent-team/vnext/internal/core"
	"github.com/thebpandey/agent-team/vnext/internal/dispatch"
	"github.com/thebpandey/agent-team/vnext/internal/host"
	"github.com/thebpandey/agent-team/vnext/internal/store"
)

// WorktreeManager is the inherited Phase 1 manager boundary.
type WorktreeManager = contracts.WorktreeManager

// ExactWorktreeRemover permits cleanup only of the exact durable worktree.
type ExactWorktreeRemover interface {
	RemoveExact(context.Context, contracts.Worktree) error
}

// WorktreeIdentity is the durable, run-scoped ownership record.
type WorktreeIdentity struct {
	core.RecordEnvelope
	Team    core.TeamID `json:"team"`
	Path    string      `json:"path"`
	Base    string      `json:"base"`
	Branch  string      `json:"branch"`
	Removed bool        `json:"removed"`
}

// Manager is the sole owner of Git worktree lifecycle operations.
type Manager struct {
	repoRoot string
	project  string
	state    *store.Store
	runner   host.CommandRunner
	records  map[string]contracts.Worktree
}

var _ contracts.WorktreeManager = (*Manager)(nil)
var _ ExactWorktreeRemover = (*Manager)(nil)

// NewManager creates a manager rooted at repoRoot. It does not run Git.
func NewManager(repoRoot, project string, state *store.Store, runner host.CommandRunner) *Manager {
	return &Manager{repoRoot: repoRoot, project: project, state: state, runner: runner, records: make(map[string]contracts.Worktree)}
}

func (m *Manager) Create(ctx context.Context, spec contracts.WorktreeSpec) (contracts.Worktree, error) {
	if m == nil || m.runner == nil || m.state == nil || m.project == "" ||
		!safeID(string(spec.Run)) || !safeID(string(spec.Team)) || !safeGitAtom(spec.Base) {
		return contracts.Worktree{}, core.ErrPath
	}
	if err := dispatch.ValidatePacket(core.AssignmentPacket{
		RecordEnvelope: core.RecordEnvelope{RunID: spec.Run}, Team: spec.Team, Base: spec.Base,
	}, spec); err != nil {
		return contracts.Worktree{}, err
	}
	repo, err := canonicalExisting(m.repoRoot)
	if err != nil {
		return contracts.Worktree{}, core.ErrPath
	}
	stateRoot, err := canonicalExisting(m.state.Root)
	if err != nil || stateRoot != repo {
		return contracts.Worktree{}, core.ErrPath
	}
	managedRoot, err := canonicalCandidate(filepath.Join(repo, ".agent-team", "worktrees"))
	if err != nil || !strictDescendant(repo, managedRoot) {
		return contracts.Worktree{}, core.ErrPath
	}
	candidate, err := canonicalCandidate(spec.Root)
	if err != nil || !strictDescendant(managedRoot, candidate) {
		return contracts.Worktree{}, core.ErrPath
	}

	key := worktreeKey(spec.Run, spec.Team)
	if existing, ok := m.records[key]; ok {
		if sameSpec(existing, spec, candidate) {
			return existing, nil
		}
		return contracts.Worktree{}, core.ErrPath
	}
	if existing, err := m.load(spec.Run, spec.Team); err == nil {
		if sameSpec(existing, spec, candidate) {
			m.records[key] = existing
			return existing, nil
		}
		return contracts.Worktree{}, core.ErrPath
	}

	branch := "agent-team/" + string(spec.Run) + "/" + string(spec.Team)
	if err := m.git(ctx, "worktree", "add", "-b", branch, candidate, spec.Base); err != nil {
		return contracts.Worktree{}, err
	}
	worktree := contracts.Worktree{Run: spec.Run, Team: spec.Team, Path: candidate, Canonical: candidate, Branch: branch, Base: spec.Base}
	if err := m.save(worktree); err != nil {
		// Do not leave an unrecorded worktree eligible for future cleanup. Both
		// operations are deliberately non-force; an unsuccessful compensation is
		// still reported as the state failure and never treated as ownership.
		_ = m.git(ctx, "worktree", "remove", candidate)
		_ = m.git(ctx, "branch", "-d", branch)
		return contracts.Worktree{}, err
	}
	m.records[key] = worktree
	return worktree, nil
}

// Inspect returns only a worktree that exactly matches its durable identity.
func (m *Manager) Inspect(_ context.Context, worktree contracts.Worktree) (contracts.Worktree, error) {
	if m == nil || !safeID(string(worktree.Run)) || !safeID(string(worktree.Team)) {
		return contracts.Worktree{}, core.ErrPath
	}
	key := worktreeKey(worktree.Run, worktree.Team)
	expected, ok := m.records[key]
	if !ok {
		var err error
		expected, err = m.load(worktree.Run, worktree.Team)
		if err != nil {
			return contracts.Worktree{}, err
		}
		m.records[key] = expected
	}
	if !sameWorktree(expected, worktree, true) {
		return contracts.Worktree{}, core.ErrPath
	}
	return expected, nil
}

// Integrate merges one exact candidate into the manager checkout.
func (m *Manager) Integrate(ctx context.Context, candidate contracts.Candidate) (contracts.Candidate, error) {
	if candidate.Task == "" || !safeID(string(candidate.Task)) || !safeGitAtom(candidate.Revision) || candidate.Base == "" {
		return contracts.Candidate{}, core.ErrPath
	}
	worktree, err := m.Inspect(ctx, candidate.Worktree)
	if err != nil || candidate.Base != worktree.Base {
		return contracts.Candidate{}, core.ErrPath
	}
	if err := m.git(ctx, "merge", "--no-ff", "--no-edit", candidate.Revision); err != nil {
		return contracts.Candidate{}, err
	}
	return candidate, nil
}

// RemoveExact removes only a recorded, clean task worktree, never a caller
// supplied path that merely resembles one. Base may be omitted by the later
// cleanup boundary, but a supplied base must exactly match the durable value.
func (m *Manager) RemoveExact(ctx context.Context, worktree contracts.Worktree) error {
	if worktree.Dirty {
		return core.ErrPath
	}
	expected, err := m.exactForRemoval(worktree)
	if err != nil {
		return err
	}
	if expected.Dirty {
		return core.ErrPath
	}
	if err := m.git(ctx, "worktree", "remove", expected.Path); err != nil {
		return err
	}
	if err := m.git(ctx, "branch", "-d", expected.Branch); err != nil {
		return err
	}
	if err := m.tombstone(expected); err != nil {
		return err
	}
	delete(m.records, worktreeKey(expected.Run, expected.Team))
	return nil
}

// Cleanup preserves the inherited team-only contract by refusing an ambiguous
// team. Later cleanup uses RemoveExact with run-scoped identity instead.
func (m *Manager) Cleanup(ctx context.Context, team core.TeamID) error {
	if m == nil || !safeID(string(team)) {
		return core.ErrPath
	}
	var found []contracts.Worktree
	for _, worktree := range m.records {
		if worktree.Team == team {
			found = append(found, worktree)
		}
	}
	if len(found) == 0 {
		return nil
	}
	if len(found) != 1 {
		return core.ErrPath
	}
	return m.RemoveExact(ctx, found[0])
}

func (m *Manager) exactForRemoval(worktree contracts.Worktree) (contracts.Worktree, error) {
	if m == nil || !safeID(string(worktree.Run)) || !safeID(string(worktree.Team)) || worktree.Path == "" || worktree.Branch == "" {
		return contracts.Worktree{}, core.ErrPath
	}
	key := worktreeKey(worktree.Run, worktree.Team)
	expected, ok := m.records[key]
	if !ok {
		var err error
		expected, err = m.load(worktree.Run, worktree.Team)
		if err != nil {
			return contracts.Worktree{}, err
		}
		m.records[key] = expected
	}
	if !sameWorktree(expected, worktree, false) {
		return contracts.Worktree{}, core.ErrPath
	}
	return expected, nil
}

func (m *Manager) git(ctx context.Context, args ...string) error {
	if m == nil || m.runner == nil || m.repoRoot == "" {
		return core.ErrPath
	}
	result := m.runner.Run(ctx, "git", append([]string{"-C", m.repoRoot}, args...)...)
	if result.Transport != nil || result.TimedOut || result.Exit != 0 {
		return core.ErrGit
	}
	return nil
}

func (m *Manager) save(worktree contracts.Worktree) error {
	if m == nil || m.state == nil || m.project == "" {
		return core.ErrPath
	}
	_, err := m.state.WriteJSON(worktreeIdentityPath(worktree.Run, worktree.Team), WorktreeIdentity{
		RecordEnvelope: core.RecordEnvelope{Schema: 1, Project: m.project, RunID: worktree.Run, WrittenAt: time.Now().UTC().Format(time.RFC3339Nano), Revision: 1},
		Team:           worktree.Team, Path: worktree.Path, Base: worktree.Base, Branch: worktree.Branch,
	}, 64<<10)
	return err
}

func (m *Manager) tombstone(worktree contracts.Worktree) error {
	if m == nil || m.state == nil || m.project == "" {
		return core.ErrPath
	}
	_, err := m.state.WriteJSON(worktreeIdentityPath(worktree.Run, worktree.Team), WorktreeIdentity{
		RecordEnvelope: core.RecordEnvelope{Schema: 1, Project: m.project, RunID: worktree.Run, WrittenAt: time.Now().UTC().Format(time.RFC3339Nano), Revision: 2},
		Team:           worktree.Team, Removed: true,
	}, 64<<10)
	return err
}

func (m *Manager) load(run core.RunID, team core.TeamID) (contracts.Worktree, error) {
	if m == nil || m.state == nil || !safeID(string(run)) || !safeID(string(team)) {
		return contracts.Worktree{}, core.ErrPath
	}
	var saved WorktreeIdentity
	if err := m.state.ReadJSON(worktreeIdentityPath(run, team), 64<<10, &saved); err != nil ||
		saved.Schema != 1 || saved.Project != m.project || saved.RunID != run || saved.Team != team || saved.Removed ||
		saved.Path == "" || saved.Base == "" || saved.Branch != branchFor(run, team) {
		return contracts.Worktree{}, core.ErrPath
	}
	return contracts.Worktree{Run: run, Team: team, Path: saved.Path, Canonical: saved.Path, Base: saved.Base, Branch: saved.Branch}, nil
}

func sameSpec(worktree contracts.Worktree, spec contracts.WorktreeSpec, path string) bool {
	return worktree.Run == spec.Run && worktree.Team == spec.Team && worktree.Path == path &&
		worktree.Base == spec.Base && worktree.Branch == branchFor(spec.Run, spec.Team) && !worktree.Dirty
}

func sameWorktree(expected, supplied contracts.Worktree, requireBase bool) bool {
	if expected.Run != supplied.Run || expected.Team != supplied.Team || expected.Path != supplied.Path ||
		expected.Branch != supplied.Branch || supplied.Dirty || (supplied.Canonical != "" && supplied.Canonical != expected.Canonical) {
		return false
	}
	return !requireBase || expected.Base == supplied.Base
}

func worktreeKey(run core.RunID, team core.TeamID) string { return string(run) + "\x00" + string(team) }

func worktreeIdentityPath(run core.RunID, team core.TeamID) string {
	return filepath.ToSlash(filepath.Join(".agent-team", "runtime", "worktrees", string(run), string(team)+".json"))
}

func branchFor(run core.RunID, team core.TeamID) string {
	return "agent-team/" + string(run) + "/" + string(team)
}

func canonicalExisting(path string) (string, error) {
	abs, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(abs)
}

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
	return err == nil && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
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
