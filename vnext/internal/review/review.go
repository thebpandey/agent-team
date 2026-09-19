// Package review provides immutable, foreground candidate review receipts.
package review

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"github.com/thebpandey/agent-team/vnext/internal/contracts"
	"github.com/thebpandey/agent-team/vnext/internal/core"
	"github.com/thebpandey/agent-team/vnext/internal/host"
	"github.com/thebpandey/agent-team/vnext/internal/store"
)

const (
	maxChecks          = 32
	maxCommandParts    = 32
	maxCheckBytes      = 16 << 10
	maxExecutionBytes  = 32 << 10
	maxFindingBytes    = 8 << 10
	maxEvidenceBytes   = 4 << 10
	maxReceiptBytes    = 64 << 10
	maxFindingPerCheck = 512
)

type Verdict string

const (
	FIX   Verdict = "FIX"
	CLEAN Verdict = "CLEAN"
)

type Input struct {
	Task            core.Task
	Candidate       contracts.Candidate
	Developer       contracts.WorkerHandle
	Checks          []core.Check
	CandidateDigest string
}
type Result struct {
	core.RecordEnvelope
	Verdict          Verdict
	Findings         []string
	EvidencePointer  string
	ReviewerIdentity string
	CandidateDigest  string
}
type Reviewer interface {
	Review(context.Context, Input) (Result, error)
}

// provenance is the immutable review-attempt key. Repair changes candidate
// revision/digest and therefore creates a new receipt without overwriting old evidence.
type provenance struct {
	Task            core.Task              `json:"task"`
	Candidate       contracts.Candidate    `json:"candidate"`
	CandidateDigest string                 `json:"candidateDigest"`
	Developer       contracts.WorkerHandle `json:"developer"`
	Checks          []core.Check           `json:"checks"`
}
type durableResult struct {
	Result
	Provenance provenance             `json:"provenance"`
	Reviewer   contracts.WorkerHandle `json:"reviewer"`
	Digest     string                 `json:"digest"`
}
type reviewer struct {
	host   host.Adapter
	runner host.CommandRunner
	store  *store.Store
}

func NewReviewer(adapter host.Adapter, runner host.CommandRunner, state *store.Store) Reviewer {
	return &reviewer{host: adapter, runner: runner, store: state}
}

func (r *reviewer) Review(ctx context.Context, input Input) (Result, error) {
	if r == nil || r.host == nil || r.store == nil || ctx == nil || ctx.Err() != nil {
		return Result{}, core.ErrCapacity
	}
	p, err := validateInput(input)
	if err != nil {
		return Result{}, err
	}
	key, err := provenanceKey(p)
	if err != nil {
		return Result{}, core.ErrRevision
	}
	path := reviewPath(p.Task.RunID, p.Task.ID, key)
	if result, found, err := r.load(ctx, path, p); err != nil {
		return Result{}, err
	} else if found {
		return result, nil
	}
	reviewerHandle, err := r.reserve(ctx, p)
	if err != nil {
		return Result{}, err
	}
	findings, err := r.runChecks(ctx, p.Checks)
	if err != nil {
		return Result{}, err
	}
	verdict := CLEAN
	if len(findings) != 0 {
		verdict = FIX
	}
	result := Result{RecordEnvelope: core.RecordEnvelope{Schema: 1, Project: p.Task.Project, RunID: p.Task.RunID, WrittenAt: time.Now().UTC().Format(time.RFC3339Nano), Revision: 1}, Verdict: verdict, Findings: append([]string(nil), findings...), EvidencePointer: path, ReviewerIdentity: reviewerHandle.Identity, CandidateDigest: p.CandidateDigest}
	record := durableResult{Result: result, Provenance: p, Reviewer: reviewerHandle}
	record.Digest = receiptDigest(record)
	if _, err := r.store.CreateJSON(path, record, maxReceiptBytes); err == nil {
		return copyResult(result), nil
	} else if !errors.Is(err, store.ErrAlreadyExists) {
		return Result{}, err
	}
	// Concurrent reviews may both reserve, but CreateJSON publishes exactly one immutable winner.
	winner, found, err := r.load(ctx, path, p)
	if err != nil || !found {
		return Result{}, core.ErrRevision
	}
	return winner, nil
}

func (r *reviewer) load(ctx context.Context, path string, p provenance) (Result, bool, error) {
	var record durableResult
	err := r.store.ReadJSON(path, maxReceiptBytes, &record)
	if errors.Is(err, os.ErrNotExist) {
		return Result{}, false, nil
	}
	if err != nil {
		return Result{}, false, core.ErrPath
	}
	caps, capErr := r.host.Probe(ctx)
	if capErr != nil || !safeValue(caps.Host) || !validRecord(record, path, p, caps.Host) {
		return Result{}, false, core.ErrRevision
	}
	return copyResult(record.Result), true, nil
}

func (r *reviewer) reserve(ctx context.Context, p provenance) (contracts.WorkerHandle, error) {
	caps, err := r.host.Probe(ctx)
	if err != nil || !safeValue(caps.Host) {
		return contracts.WorkerHandle{}, core.ErrCapacity
	}
	packet := core.AssignmentPacket{RecordEnvelope: core.RecordEnvelope{RunID: p.Task.RunID}, Team: p.Candidate.Worktree.Team, Task: p.Task.ID, SpecRevision: p.Candidate.Revision, QueueFingerprint: p.CandidateDigest}
	req := contracts.WorkerRequest{Packet: packet, Worktree: contracts.WorktreeSpec{Run: p.Task.RunID, Team: packet.Team, Root: p.Candidate.Worktree.Canonical, Base: p.Candidate.Base}, Reviewer: true}
	h, err := r.host.StartReviewer(ctx, req, p.Developer)
	if err != nil {
		return contracts.WorkerHandle{}, err
	}
	if !validReviewer(h, p, caps.Host) || h.Identity == p.Developer.Identity {
		return contracts.WorkerHandle{}, core.ErrRevision
	}
	id, err := r.host.ReadIdentity(ctx, h)
	if err != nil || id == "" || id != h.Identity || id == p.Developer.Identity {
		return contracts.WorkerHandle{}, core.ErrRevision
	}
	return h, nil
}

func (r *reviewer) runChecks(ctx context.Context, checks []core.Check) ([]string, error) {
	if len(checks) == 0 {
		return nil, nil
	}
	if r.runner == nil {
		return nil, core.ErrCapacity
	}
	findings := make([]string, 0, len(checks))
	execution, findingBytes := 0, 0
	for _, check := range checks {
		result := r.runner.Run(ctx, check.Command[0], append([]string(nil), check.Command[1:]...)...)
		execution += len(result.Stdout) + len(result.Stderr)
		if execution > maxExecutionBytes {
			return nil, core.ErrLimit
		}
		if result.Transport != nil || result.TimedOut {
			return nil, core.ErrCapacity
		}
		if result.Exit == 0 {
			continue
		}
		message := strings.TrimSpace(string(result.Stderr))
		if message == "" {
			message = strings.TrimSpace(string(result.Stdout))
		}
		if len(message) > maxFindingPerCheck {
			message = message[:maxFindingPerCheck]
		}
		finding := check.Name + ": " + message
		findingBytes += len(finding)
		if findingBytes > maxFindingBytes {
			return nil, core.ErrLimit
		}
		findings = append(findings, finding)
	}
	return findings, nil
}

func validateInput(input Input) (provenance, error) {
	task, candidate, worktree := input.Task, input.Candidate, input.Candidate.Worktree
	if !safeID(string(task.ID)) || !safeID(string(task.RunID)) || !safeValue(task.Project) || candidate.Task != task.ID || !safeValue(candidate.Revision) || !safeValue(candidate.Base) || !safeValue(input.CandidateDigest) || worktree.Run != task.RunID || !safeID(string(worktree.Team)) || !safeValue(worktree.Base) || !safeValue(worktree.Branch) || worktree.Base != candidate.Base || !verifiedTaskRoot(worktree.Path, worktree.Canonical) || !validDeveloper(input.Developer, task, candidate, input.CandidateDigest) {
		return provenance{}, core.ErrPath
	}
	checks, err := copyChecks(input.Checks)
	if err != nil {
		return provenance{}, err
	}
	return provenance{Task: task, Candidate: candidate, CandidateDigest: input.CandidateDigest, Developer: input.Developer, Checks: checks}, nil
}

func validDeveloper(h contracts.WorkerHandle, task core.Task, candidate contracts.Candidate, digest string) bool {
	return h.Host != "" && h.Identity != "" && !h.Reviewer && h.Run == task.RunID && h.Team == candidate.Worktree.Team && h.Task == task.ID && h.CandidateRevision == candidate.Revision && h.PacketDigest == digest
}

// verifiedTaskRoot consumes the manager's canonical handle and derives the
// one available local authority (the module checkout). It fails closed unless
// the physical root is a strict descendant of its managed-worktree directory.
func verifiedTaskRoot(path, canonical string) bool {
	if !safePath(path) || !safePath(canonical) {
		return false
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return false
	}
	resolved, err = filepath.Abs(resolved)
	if err != nil || filepath.Clean(resolved) != filepath.Clean(canonical) {
		return false
	}
	info, err := os.Stat(resolved)
	if err != nil || !info.IsDir() {
		return false
	}
	root, err := moduleRoot()
	if err != nil {
		return false
	}
	managed := filepath.Join(root, ".agent-team", "worktrees")
	rel, err := filepath.Rel(managed, resolved)
	return err == nil && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func moduleRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if info, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil && !info.IsDir() {
			return filepath.EvalSymlinks(dir)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", os.ErrNotExist
		}
		dir = parent
	}
}

func copyChecks(checks []core.Check) ([]core.Check, error) {
	if len(checks) > maxChecks {
		return nil, core.ErrLimit
	}
	total := 0
	out := make([]core.Check, len(checks))
	for i, check := range checks {
		if len(check.Command) == 0 || len(check.Command) > maxCommandParts {
			return nil, core.ErrLimit
		}
		if !safeValue(check.Name) {
			return nil, core.ErrPath
		}
		out[i].Name = check.Name
		out[i].Command = append([]string(nil), check.Command...)
		for _, part := range check.Command {
			if !safeValue(part) {
				return nil, core.ErrPath
			}
			total += len(part)
		}
		total += len(check.Name)
		if total > maxCheckBytes {
			return nil, core.ErrLimit
		}
	}
	return out, nil
}
func validReviewer(h contracts.WorkerHandle, p provenance, expectedHost string) bool {
	return h.Host == expectedHost && h.Identity != "" && h.Reviewer && h.Run == p.Task.RunID && h.Team == p.Candidate.Worktree.Team && h.Task == p.Task.ID && h.CandidateRevision == p.Candidate.Revision && h.PacketDigest == p.CandidateDigest
}
func validRecord(record durableResult, path string, p provenance, expectedHost string) bool {
	if record.Schema != 1 || record.Project != p.Task.Project || record.Revision != 1 || record.RunID != p.Task.RunID || record.EvidencePointer != path || len(record.EvidencePointer) > maxEvidenceBytes || record.CandidateDigest != p.CandidateDigest || !safeValue(record.ReviewerIdentity) || record.ReviewerIdentity != record.Reviewer.Identity || !validReviewer(record.Reviewer, p, expectedHost) || record.Reviewer.Identity == p.Developer.Identity || validateVerdict(record.Verdict) != nil || !sameProvenance(record.Provenance, p) || record.Digest != receiptDigest(record) {
		return false
	}
	_, err := time.Parse(time.RFC3339Nano, record.WrittenAt)
	return err == nil
}
func sameProvenance(a, b provenance) bool {
	ka, errA := provenanceKey(a)
	kb, errB := provenanceKey(b)
	return errA == nil && errB == nil && ka == kb
}
func provenanceKey(p provenance) (string, error) {
	encoded, err := json.Marshal(p)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:]), nil
}

// receiptDigest covers the envelope, candidate/base, developer, checks, evidence, reviewer and findings.
func receiptDigest(record durableResult) string {
	record.Digest = ""
	encoded, err := json.Marshal(record)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(encoded)
	return "sha256:" + hex.EncodeToString(sum[:])
}
func validateVerdict(v Verdict) error {
	if v != FIX && v != CLEAN {
		return core.ErrTransition
	}
	return nil
}
func reviewPath(run core.RunID, task core.TaskID, key string) string {
	return filepath.ToSlash(filepath.Join(".agent-team", "runtime", "reviews", string(run), string(task), key+".json"))
}
func copyResult(result Result) Result {
	result.Findings = append([]string(nil), result.Findings...)
	return result
}
func safeID(value string) bool {
	return safeValue(value) && value != "." && value != ".." && !strings.ContainsAny(value, `/\\`)
}
func safeValue(value string) bool {
	if value == "" || len(value) > 1024 {
		return false
	}
	for _, r := range value {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}
func safePath(value string) bool {
	return value != "" && filepath.IsAbs(value) && filepath.Clean(value) != string(filepath.Separator) && !strings.ContainsRune(value, 0)
}
