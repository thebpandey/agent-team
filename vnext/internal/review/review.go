// Package review provides deterministic, foreground candidate review.
package review

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/thebpandey/agent-team/vnext/internal/contracts"
	"github.com/thebpandey/agent-team/vnext/internal/core"
	"github.com/thebpandey/agent-team/vnext/internal/host"
	"github.com/thebpandey/agent-team/vnext/internal/store"
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
	Verdict          Verdict  `json:"verdict"`
	Findings         []string `json:"findings"`
	EvidencePointer  string   `json:"evidencePointer"`
	ReviewerIdentity string   `json:"reviewerIdentity"`
	CandidateDigest  string   `json:"candidateDigest"`
}

type Reviewer interface {
	Review(context.Context, Input) (Result, error)
}

type durableResult struct {
	Result
	TaskID            core.TaskID `json:"taskId"`
	CandidateRevision string      `json:"candidateRevision"`
	BaseDigest        string      `json:"baseDigest"`
	DeveloperIdentity string      `json:"developerIdentity"`
}

type reviewer struct {
	host   host.Adapter
	runner host.CommandRunner
	store  *store.Store
	mu     sync.Mutex
}

func NewReviewer(adapter host.Adapter, runner host.CommandRunner, state *store.Store) Reviewer {
	return &reviewer{host: adapter, runner: runner, store: state}
}

func (r *reviewer) Review(ctx context.Context, input Input) (Result, error) {
	if r == nil || r.store == nil || ctx == nil || ctx.Err() != nil {
		return Result{}, core.ErrTransition
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	task, run, err := reviewScope(input)
	if err != nil || input.CandidateDigest == "" {
		return Result{}, core.ErrPath
	}
	path := reviewPath(run, task)
	baseDigest := digest(input.Candidate.Base)
	developer := input.Developer.Identity
	var prior durableResult
	if err := r.store.ReadJSON(path, 64<<10, &prior); err == nil {
		if !validDurable(prior, run, task) || prior.CandidateDigest != input.CandidateDigest || prior.CandidateRevision != input.Candidate.Revision || prior.BaseDigest != baseDigest || prior.DeveloperIdentity != developer {
			return Result{}, core.ErrRevision
		}
		return copyResult(prior.Result), nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return Result{}, core.ErrPath
	}

	reviewerIdentity, err := r.reserve(ctx, input, run, task)
	if err != nil {
		return Result{}, err
	}
	findings, err := r.runChecks(ctx, input.Checks)
	if err != nil {
		return Result{}, err
	}
	verdict := CLEAN
	if len(findings) != 0 {
		verdict = FIX
	}
	result := Result{RecordEnvelope: core.RecordEnvelope{Schema: 1, Project: input.Task.Project, RunID: run, WrittenAt: time.Now().UTC().Format(time.RFC3339Nano), Revision: 1}, Verdict: verdict, Findings: findings, EvidencePointer: path, ReviewerIdentity: reviewerIdentity, CandidateDigest: input.CandidateDigest}
	if err := validateVerdict(result.Verdict); err != nil {
		return Result{}, err
	}
	record := durableResult{Result: result, TaskID: task, CandidateRevision: input.Candidate.Revision, BaseDigest: baseDigest, DeveloperIdentity: developer}
	if _, err := r.store.WriteJSON(path, record, 64<<10); err != nil {
		return Result{}, err
	}
	return copyResult(result), nil
}

func (r *reviewer) reserve(ctx context.Context, input Input, run core.RunID, task core.TaskID) (string, error) {
	if r.host == nil {
		if input.Developer.Identity == "native-reviewer" {
			return "", core.ErrRevision
		}
		return "native-reviewer", nil
	}
	if input.Developer.Identity == "" || input.Candidate.Worktree.Path == "" || input.Candidate.Worktree.Team == "" {
		return "", core.ErrPath
	}
	packet := core.AssignmentPacket{RecordEnvelope: core.RecordEnvelope{RunID: run}, Team: input.Candidate.Worktree.Team, Task: task, SpecRevision: input.Candidate.Revision, QueueFingerprint: input.CandidateDigest}
	request := contracts.WorkerRequest{Packet: packet, Worktree: contracts.WorktreeSpec{Run: run, Team: packet.Team, Root: input.Candidate.Worktree.Path, Base: input.Candidate.Base}, Reviewer: true}
	handle, err := r.host.StartReviewer(ctx, request, input.Developer)
	if err != nil {
		return "", err
	}
	if handle.Identity == "" || !handle.Reviewer || handle.Identity == input.Developer.Identity || handle.Run != run || handle.Team != packet.Team || handle.Task != task || handle.CandidateRevision != input.Candidate.Revision || handle.PacketDigest != input.CandidateDigest {
		return "", core.ErrRevision
	}
	identity, err := r.host.ReadIdentity(ctx, handle)
	if err != nil || identity == "" || identity != handle.Identity || identity == input.Developer.Identity {
		return "", core.ErrRevision
	}
	return identity, nil
}

func (r *reviewer) runChecks(ctx context.Context, checks []core.Check) ([]string, error) {
	if len(checks) == 0 {
		return nil, nil
	}
	if r.runner == nil {
		return nil, core.ErrCapacity
	}
	findings := make([]string, 0, len(checks))
	for _, check := range checks {
		if check.Name == "" || len(check.Command) == 0 || check.Command[0] == "" {
			return nil, core.ErrPath
		}
		result := r.runner.Run(ctx, check.Command[0], append([]string(nil), check.Command[1:]...)...)
		if result.Transport != nil || result.TimedOut {
			return nil, core.ErrCapacity
		}
		if result.Exit != 0 {
			if len(findings) == 32 {
				return nil, core.ErrLimit
			}
			message := strings.TrimSpace(string(result.Stderr))
			if message == "" {
				message = strings.TrimSpace(string(result.Stdout))
			}
			if len(message) > 512 {
				message = message[:512]
			}
			findings = append(findings, fmt.Sprintf("%s: %s", check.Name, message))
		}
	}
	return findings, nil
}

func reviewScope(input Input) (core.TaskID, core.RunID, error) {
	task := input.Task.ID
	if task == "" {
		task = input.Candidate.Task
	}
	run := input.Task.RunID
	if run == "" {
		run = input.Candidate.Worktree.Run
	}
	if run == "" {
		run = "local"
	}
	if !safeID(string(task)) || !safeID(string(run)) || (input.Candidate.Task != "" && input.Candidate.Task != task) || (input.Candidate.Worktree.Run != "" && input.Candidate.Worktree.Run != run) {
		return "", "", core.ErrPath
	}
	return task, run, nil
}

func validDurable(record durableResult, run core.RunID, task core.TaskID) bool {
	return record.Schema == 1 && record.RunID == run && record.TaskID == task && record.Revision == 1 && record.EvidencePointer == reviewPath(run, task) && record.ReviewerIdentity != "" && record.CandidateDigest != "" && validateVerdict(record.Verdict) == nil
}

func validateVerdict(verdict Verdict) error {
	if verdict != FIX && verdict != CLEAN {
		return core.ErrTransition
	}
	return nil
}
func reviewPath(run core.RunID, task core.TaskID) string {
	return filepath.ToSlash(filepath.Join(".agent-team", "runtime", "reviews", string(run), string(task)+".json"))
}
func digest(value string) string {
	sum := sha256.Sum256([]byte(value))
	return "sha256:" + hex.EncodeToString(sum[:])
}
func copyResult(result Result) Result {
	result.Findings = append([]string(nil), result.Findings...)
	return result
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
