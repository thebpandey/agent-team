// Package worktree owns task worktree lifecycle operations.
package worktree

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/thebpandey/agent-team/vnext/internal/contracts"
	"github.com/thebpandey/agent-team/vnext/internal/core"
	"github.com/thebpandey/agent-team/vnext/internal/dispatch"
	"github.com/thebpandey/agent-team/vnext/internal/host"
	"github.com/thebpandey/agent-team/vnext/internal/store"
	"github.com/thebpandey/agent-team/vnext/internal/tracker"
)

type WorktreeManager = contracts.WorktreeManager
type ExactWorktreeRemover interface {
	RemoveExact(context.Context, contracts.Worktree) error
}
type lifecycle string

const (
	creating         lifecycle = "creating"
	active           lifecycle = "active"
	removingWorktree lifecycle = "removing-worktree"
	removingBranch   lifecycle = "removing-branch"
	removed          lifecycle = "removed"
)

// WorktreeIdentity is the durable, run-scoped ownership record. Each stage is
// persisted before a mutating Git command, so retries reconcile only this
// exact path and branch.
type WorktreeIdentity struct {
	core.RecordEnvelope
	Team              core.TeamID `json:"team"`
	Path              string      `json:"path"`
	Base              string      `json:"base"`
	Branch            string      `json:"branch"`
	Lifecycle         lifecycle   `json:"lifecycle"`
	CandidateTask     core.TaskID `json:"candidateTask,omitempty"`
	CandidateRevision string      `json:"candidateRevision,omitempty"`
	Removed           bool        `json:"removed"`
}

type Manager struct {
	repoRoot string
	project  string
	state    *store.Store
	runner   host.CommandRunner
	mu       sync.Mutex
	records  map[string]contracts.Worktree
	write    func(string, WorktreeIdentity) error
	create   func(string, WorktreeIdentity) error
}

var _ contracts.WorktreeManager = (*Manager)(nil)
var _ ExactWorktreeRemover = (*Manager)(nil)

func NewManager(repoRoot, project string, state *store.Store, runner host.CommandRunner) *Manager {
	m := &Manager{repoRoot: repoRoot, project: project, state: state, runner: runner, records: map[string]contracts.Worktree{}}
	m.write = m.writeIdentity
	m.create = m.createIdentity
	return m
}

func (m *Manager) Create(ctx context.Context, spec contracts.WorktreeSpec) (contracts.Worktree, error) {
	if m == nil {
		return contracts.Worktree{}, core.ErrPath
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	repo, managed, path, err := m.validateSpec(spec)
	if err != nil {
		return contracts.Worktree{}, err
	}
	_ = managed
	base, err := m.resolveBase(ctx, repo, spec.Base)
	if err != nil {
		return contracts.Worktree{}, err
	}
	identity, exists, err := m.readIdentity(repo, spec.Run, spec.Team)
	if err != nil {
		return contracts.Worktree{}, err
	}
	if !exists {
		identity = WorktreeIdentity{RecordEnvelope: core.RecordEnvelope{Schema: 1, Project: m.project, RunID: spec.Run, WrittenAt: timestamp(), Revision: 1}, Team: spec.Team, Path: path, Base: base, Branch: branchFor(spec.Run, spec.Team), Lifecycle: creating}
		// Creating intent is the ownership boundary: no Git mutation precedes it.
		if err := m.persistNew(spec.Run, spec.Team, identity); err != nil {
			if !errors.Is(err, store.ErrAlreadyExists) {
				return contracts.Worktree{}, err
			}
			identity, exists, err = m.readIdentity(repo, spec.Run, spec.Team)
			if err != nil || !exists || identity.Lifecycle == removed || !sameIdentitySpec(identity, spec, path, base) {
				return contracts.Worktree{}, core.ErrPath
			}
		}
	} else if identity.Lifecycle == removed || !sameIdentitySpec(identity, spec, path, base) {
		return contracts.Worktree{}, core.ErrPath
	}
	worktree, err := m.resumeCreate(ctx, repo, identity)
	if err == nil {
		m.records[worktreeKey(spec.Run, spec.Team)] = worktree
	}
	return worktree, err
}

func (m *Manager) resumeCreate(ctx context.Context, repo string, identity WorktreeIdentity) (contracts.Worktree, error) {
	if identity.Lifecycle != creating && identity.Lifecycle != active {
		return contracts.Worktree{}, core.ErrPath
	}
	worktreePresent, err := m.worktreePresent(ctx, repo, identity)
	if err != nil {
		return contracts.Worktree{}, err
	}
	branchPresent, err := m.branchPresent(ctx, repo, identity.Branch)
	if err != nil {
		return contracts.Worktree{}, err
	}
	if identity.Lifecycle == creating {
		if !worktreePresent && !branchPresent {
			if err := m.git(ctx, repo, "worktree", "add", "-b", identity.Branch, identity.Path, identity.Base); err != nil {
				return contracts.Worktree{}, err
			}
			worktreePresent, branchPresent = true, true
		}
		if !worktreePresent || !branchPresent {
			return contracts.Worktree{}, core.ErrGit
		}
		if err := advance(&identity, active, false); err != nil {
			return contracts.Worktree{}, err
		}
		if err := m.persist(identity.RunID, identity.Team, identity); err != nil {
			return contracts.Worktree{}, err
		}
	}
	if !worktreePresent || !branchPresent {
		return contracts.Worktree{}, core.ErrGit
	}
	return worktreeFrom(identity), nil
}

func (m *Manager) Inspect(_ context.Context, supplied contracts.Worktree) (contracts.Worktree, error) {
	if m == nil {
		return contracts.Worktree{}, core.ErrPath
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	repo, _, err := m.environment()
	if err != nil {
		return contracts.Worktree{}, err
	}
	identity, exists, err := m.readIdentity(repo, supplied.Run, supplied.Team)
	if err != nil || !exists || identity.Lifecycle != active {
		return contracts.Worktree{}, core.ErrPath
	}
	expected := worktreeFrom(identity)
	if !sameWorktree(expected, supplied, true) {
		return contracts.Worktree{}, core.ErrPath
	}
	m.records[worktreeKey(expected.Run, expected.Team)] = expected
	return expected, nil
}

func (m *Manager) Integrate(ctx context.Context, candidate contracts.Candidate) (contracts.Candidate, error) {
	if m == nil {
		return contracts.Candidate{}, core.ErrPath
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if !safeID(string(candidate.Task)) || !safeGitAtom(candidate.Revision) || candidate.Base == "" {
		return contracts.Candidate{}, core.ErrPath
	}
	repo, _, err := m.environment()
	if err != nil {
		return contracts.Candidate{}, err
	}
	identity, exists, err := m.readIdentity(repo, candidate.Worktree.Run, candidate.Worktree.Team)
	if err != nil || !exists || identity.Lifecycle != active || candidate.Base != identity.Base || !sameWorktree(worktreeFrom(identity), candidate.Worktree, true) {
		return contracts.Candidate{}, core.ErrPath
	}
	present, err := m.worktreePresent(ctx, repo, identity)
	if err != nil || !present {
		return contracts.Candidate{}, core.ErrGit
	}
	head, err := m.output(ctx, identity.Path, "rev-parse", "HEAD")
	if err != nil || strings.TrimSpace(head) != candidate.Revision {
		return contracts.Candidate{}, core.ErrRevision
	}
	if err := m.git(ctx, identity.Path, "merge-base", "--is-ancestor", identity.Base, candidate.Revision); err != nil {
		return contracts.Candidate{}, core.ErrRevision
	}
	if identity.CandidateTask != "" && (identity.CandidateTask != candidate.Task || identity.CandidateRevision != candidate.Revision) {
		return contracts.Candidate{}, core.ErrRevision
	}
	if identity.CandidateTask == "" {
		identity.CandidateTask, identity.CandidateRevision = candidate.Task, candidate.Revision
		if err := advance(&identity, identity.Lifecycle, false); err != nil {
			return contracts.Candidate{}, err
		}
		if err := m.persist(identity.RunID, identity.Team, identity); err != nil {
			return contracts.Candidate{}, err
		}
	}
	if err := m.git(ctx, repo, "merge", "--no-ff", "--no-edit", identity.CandidateRevision); err != nil {
		return contracts.Candidate{}, err
	}
	return candidate, nil
}

func (m *Manager) RemoveExact(ctx context.Context, supplied contracts.Worktree) error {
	if m == nil {
		return core.ErrPath
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.removeExact(ctx, supplied)
}

func (m *Manager) removeExact(ctx context.Context, supplied contracts.Worktree) error {
	if supplied.Dirty {
		return core.ErrPath
	}
	repo, _, err := m.environment()
	if err != nil {
		return err
	}
	identity, exists, err := m.readIdentity(repo, supplied.Run, supplied.Team)
	if err != nil || !exists || identity.Lifecycle == removed || !sameWorktree(worktreeFrom(identity), supplied, false) {
		return core.ErrPath
	}
	if identity.Lifecycle != active && identity.Lifecycle != removingWorktree && identity.Lifecycle != removingBranch {
		return core.ErrPath
	}
	if identity.Lifecycle == active {
		if err := advance(&identity, removingWorktree, false); err != nil {
			return err
		}
		if err := m.persist(identity.RunID, identity.Team, identity); err != nil {
			return err
		}
	}
	if identity.Lifecycle == removingWorktree {
		present, err := m.worktreePresent(ctx, repo, identity)
		if err != nil {
			return err
		}
		if present && m.git(ctx, repo, "worktree", "remove", identity.Path) != nil {
			return core.ErrGit
		}
		if err := advance(&identity, removingBranch, false); err != nil {
			return err
		}
		if err := m.persist(identity.RunID, identity.Team, identity); err != nil {
			return err
		}
	}
	if identity.Lifecycle == removingBranch {
		present, err := m.branchPresent(ctx, repo, identity.Branch)
		if err != nil {
			return err
		}
		if present && m.git(ctx, repo, "branch", "-d", identity.Branch) != nil {
			return core.ErrGit
		}
		if err := advance(&identity, removed, true); err != nil {
			return err
		}
		if err := m.persist(identity.RunID, identity.Team, identity); err != nil {
			return err
		}
	}
	delete(m.records, worktreeKey(supplied.Run, supplied.Team))
	return nil
}

func (m *Manager) Cleanup(ctx context.Context, team core.TeamID) error {
	if m == nil || !safeID(string(team)) {
		return core.ErrPath
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	var candidates []contracts.Worktree
	for _, worktree := range m.records {
		if worktree.Team == team {
			candidates = append(candidates, worktree)
		}
	}
	if len(candidates) == 0 {
		return core.ErrPath
	}
	if len(candidates) != 1 {
		return core.ErrPath
	}
	return m.removeExact(ctx, candidates[0])
}

func (m *Manager) validateSpec(spec contracts.WorktreeSpec) (string, string, string, error) {
	if m == nil || m.runner == nil || m.state == nil || m.project == "" || !safeID(string(spec.Run)) || !safeID(string(spec.Team)) || !safeGitAtom(spec.Base) {
		return "", "", "", core.ErrPath
	}
	if err := dispatch.ValidatePacket(core.AssignmentPacket{RecordEnvelope: core.RecordEnvelope{RunID: spec.Run}, Team: spec.Team, Base: spec.Base}, spec); err != nil {
		return "", "", "", err
	}
	repo, managed, err := m.environment()
	if err != nil {
		return "", "", "", err
	}
	path, err := canonicalCandidate(spec.Root)
	if err != nil || !strictDescendant(managed, path) {
		return "", "", "", core.ErrPath
	}
	return repo, managed, path, nil
}

func (m *Manager) environment() (string, string, error) {
	if m == nil || m.state == nil || m.runner == nil || m.project == "" {
		return "", "", core.ErrPath
	}
	repo, err := canonicalExisting(m.repoRoot)
	if err != nil {
		return "", "", core.ErrPath
	}
	stateRoot, err := canonicalExisting(m.state.Root)
	if err != nil || stateRoot != repo {
		return "", "", core.ErrPath
	}
	managed, err := canonicalCandidate(filepath.Join(repo, ".agent-team", "worktrees"))
	if err != nil || !strictDescendant(repo, managed) {
		return "", "", core.ErrPath
	}
	return repo, managed, nil
}

func (m *Manager) readIdentity(repo string, run core.RunID, team core.TeamID) (WorktreeIdentity, bool, error) {
	if !safeID(string(run)) || !safeID(string(team)) {
		return WorktreeIdentity{}, false, core.ErrPath
	}
	var identity WorktreeIdentity
	err := m.state.ReadJSON(worktreeIdentityPath(run, team), 64<<10, &identity)
	if errors.Is(err, os.ErrNotExist) {
		return WorktreeIdentity{}, false, nil
	}
	if err != nil || !m.validIdentity(repo, identity, run, team) {
		return WorktreeIdentity{}, false, core.ErrPath
	}
	return identity, true, nil
}

func (m *Manager) validIdentity(repo string, identity WorktreeIdentity, run core.RunID, team core.TeamID) bool {
	if identity.Schema != 1 || identity.Project != m.project || identity.RunID != run || identity.Team != team || identity.Revision == 0 || identity.Revision == ^uint64(0) || !validTimestamp(identity.WrittenAt) || !safeID(string(identity.RunID)) || !safeID(string(identity.Team)) || !fullOID(identity.Base) || identity.Branch != branchFor(run, team) {
		return false
	}
	if identity.Lifecycle != creating && identity.Lifecycle != active && identity.Lifecycle != removingWorktree && identity.Lifecycle != removingBranch && identity.Lifecycle != removed {
		return false
	}
	managed, err := canonicalCandidate(filepath.Join(repo, ".agent-team", "worktrees"))
	if err != nil || !strictDescendant(repo, managed) {
		return false
	}
	path, err := canonicalCandidate(identity.Path)
	if err != nil || path != identity.Path || !strictDescendant(managed, path) {
		return false
	}
	if (identity.CandidateTask == "") != (identity.CandidateRevision == "") {
		return false
	}
	if identity.CandidateTask != "" && (!safeID(string(identity.CandidateTask)) || !safeGitAtom(identity.CandidateRevision)) {
		return false
	}
	return identity.Removed == (identity.Lifecycle == removed)
}

func (m *Manager) persist(run core.RunID, team core.TeamID, identity WorktreeIdentity) error {
	if m.write == nil {
		return core.ErrPath
	}
	return m.write(worktreeIdentityPath(run, team), identity)
}
func (m *Manager) writeIdentity(path string, identity WorktreeIdentity) error {
	_, err := m.state.WriteJSON(path, identity, 64<<10)
	return err
}

func (m *Manager) persistNew(run core.RunID, team core.TeamID, identity WorktreeIdentity) error {
	if m.create == nil {
		return core.ErrPath
	}
	return m.create(worktreeIdentityPath(run, team), identity)
}

func (m *Manager) createIdentity(path string, identity WorktreeIdentity) error {
	_, err := m.state.CreateJSON(path, identity, 64<<10)
	return err
}
func (m *Manager) run(ctx context.Context, root string, args ...string) tracker.CommandResult {
	return m.runner.Run(ctx, "git", append([]string{"-C", root}, args...)...)
}
func (m *Manager) git(ctx context.Context, root string, args ...string) error {
	r := m.run(ctx, root, args...)
	if r.Transport != nil || r.TimedOut || r.Exit != 0 {
		return core.ErrGit
	}
	return nil
}
func (m *Manager) output(ctx context.Context, root string, args ...string) (string, error) {
	r := m.run(ctx, root, args...)
	if r.Transport != nil || r.TimedOut || r.Exit != 0 {
		return "", core.ErrGit
	}
	return string(r.Stdout), nil
}
func (m *Manager) branchPresent(ctx context.Context, repo, branch string) (bool, error) {
	r := m.run(ctx, repo, "show-ref", "--verify", "--quiet", "refs/heads/"+branch)
	if r.Transport != nil || r.TimedOut || r.Exit < 0 || r.Exit > 1 {
		return false, core.ErrGit
	}
	return r.Exit == 0, nil
}

func (m *Manager) resolveBase(ctx context.Context, repo, supplied string) (string, error) {
	output, err := m.output(ctx, repo, "rev-parse", "--verify", supplied+"^{commit}")
	if err != nil {
		return "", err
	}
	return parseResolvedOID(output)
}

func parseResolvedOID(output string) (string, error) {
	line, err := parseRawLine(output)
	if err != nil {
		return "", core.ErrRevision
	}
	oid := strings.ToLower(line)
	if !fullOID(oid) {
		return "", core.ErrRevision
	}
	return oid, nil
}
func (m *Manager) worktreePresent(ctx context.Context, repo string, identity WorktreeIdentity) (bool, error) {
	info, err := os.Lstat(identity.Path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return false, core.ErrGit
	}
	exact, err := canonicalExisting(identity.Path)
	if err != nil || exact != identity.Path {
		return false, core.ErrGit
	}
	returned, err := m.output(ctx, exact, "rev-parse", "--show-toplevel")
	if err != nil {
		return false, err
	}
	returnedPath, err := canonicalGitPath(returned)
	if err != nil || returnedPath != exact {
		return false, core.ErrGit
	}
	primaryCommon, err := m.gitCommonDir(ctx, repo)
	if err != nil {
		return false, err
	}
	exactCommon, err := m.gitCommonDir(ctx, exact)
	if err != nil || exactCommon != primaryCommon {
		return false, core.ErrGit
	}
	head, err := m.output(ctx, exact, "symbolic-ref", "-q", "HEAD")
	head, parseErr := parseRawLine(head)
	if err != nil || parseErr != nil || head != "refs/heads/"+identity.Branch {
		return false, core.ErrGit
	}
	return true, nil
}

func (m *Manager) gitCommonDir(ctx context.Context, root string) (string, error) {
	output, err := m.output(ctx, root, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return "", err
	}
	return canonicalGitPath(output)
}

func worktreeFrom(i WorktreeIdentity) contracts.Worktree {
	return contracts.Worktree{Run: i.RunID, Team: i.Team, Path: i.Path, Canonical: i.Path, Base: i.Base, Branch: i.Branch}
}
func sameIdentitySpec(i WorktreeIdentity, s contracts.WorktreeSpec, path, base string) bool {
	return i.RunID == s.Run && i.Team == s.Team && i.Path == path && i.Base == base && i.Branch == branchFor(s.Run, s.Team)
}
func sameWorktree(expected, supplied contracts.Worktree, requireBase bool) bool {
	return expected.Run == supplied.Run && expected.Team == supplied.Team && expected.Path == supplied.Path && expected.Branch == supplied.Branch && !supplied.Dirty && (supplied.Canonical == "" || supplied.Canonical == expected.Canonical) && (!requireBase || expected.Base == supplied.Base) && (supplied.Base == "" || supplied.Base == expected.Base)
}
func worktreeKey(run core.RunID, team core.TeamID) string { return string(run) + "\x00" + string(team) }
func worktreeIdentityPath(run core.RunID, team core.TeamID) string {
	return filepath.ToSlash(filepath.Join(".agent-team", "runtime", "worktrees", string(run), string(team)+".json"))
}
func branchFor(run core.RunID, team core.TeamID) string {
	return "agent-team/" + string(run) + "/" + string(team)
}
func timestamp() string { return time.Now().UTC().Format(time.RFC3339Nano) }

func validTimestamp(value string) bool {
	_, err := time.Parse(time.RFC3339Nano, value)
	return err == nil
}

func advance(identity *WorktreeIdentity, next lifecycle, isRemoved bool) error {
	if identity == nil || identity.Revision == 0 || identity.Revision == ^uint64(0) {
		return core.ErrRevision
	}
	previous, err := time.Parse(time.RFC3339Nano, identity.WrittenAt)
	if err != nil {
		return core.ErrRevision
	}
	now := time.Now().UTC()
	if !now.After(previous) {
		now = previous.Add(time.Nanosecond)
	}
	identity.Lifecycle, identity.Removed, identity.Revision, identity.WrittenAt = next, isRemoved, identity.Revision+1, now.Format(time.RFC3339Nano)
	return nil
}

func canonicalGitPath(output string) (string, error) {
	value, err := parseRawLine(output)
	if err != nil {
		return "", err
	}
	return canonicalExisting(filepath.FromSlash(value))
}

func parseRawLine(output string) (string, error) {
	if strings.HasSuffix(output, "\r\n") {
		output = strings.TrimSuffix(output, "\r\n")
	} else if strings.HasSuffix(output, "\n") {
		output = strings.TrimSuffix(output, "\n")
	}
	if output == "" || strings.ContainsAny(output, "\r\n") {
		return "", core.ErrPath
	}
	return output, nil
}

func fullOID(value string) bool {
	if len(value) != 40 && len(value) != 64 {
		return false
	}
	for _, r := range value {
		if !(r >= '0' && r <= '9' || r >= 'a' && r <= 'f') {
			return false
		}
	}
	return true
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
