// Package gate binds a reviewed candidate to deterministic completion evidence.
package gate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"reflect"
	"sort"
	"strings"

	"github.com/thebpandey/agent-team/vnext/internal/contracts"
	"github.com/thebpandey/agent-team/vnext/internal/core"
	"github.com/thebpandey/agent-team/vnext/internal/host"
	"github.com/thebpandey/agent-team/vnext/internal/store"
)

const evidenceLimit = 64 << 10

// Gate checks a candidate's revision-bound receipt and review evidence.
type Gate interface {
	Check(context.Context, contracts.GateInput) (contracts.GateResult, error)
}

type completionGate struct {
	runner host.CommandRunner
	store  *store.Store
}

type evidenceRecord struct {
	Binding gateBinding          `json:"binding"`
	Result  contracts.GateResult `json:"result"`
}

type gateBinding struct {
	Run             core.RunID          `json:"run"`
	Task            core.TaskID         `json:"task"`
	Candidate       contracts.Candidate `json:"candidate"`
	TrackerRevision uint64              `json:"trackerRevision"`
	ReceiptRevision uint64              `json:"receiptRevision"`
	Checks          []string            `json:"checks"`
	ScopeDigest     string              `json:"scopeDigest"`
	ReceiptDigest   string              `json:"receiptDigest"`
	ReviewDigest    string              `json:"reviewDigest"`
}

// NewGate creates a deterministic, foreground completion gate. The runner is
// retained for later concrete check execution; this boundary receives only
// already-fingerprinted checks and never interprets them as shell commands.
func NewGate(runner host.CommandRunner, state *store.Store) Gate {
	return &completionGate{runner: runner, store: state}
}

func (g *completionGate) Check(ctx context.Context, input contracts.GateInput) (contracts.GateResult, error) {
	if ctx == nil || ctx.Err() != nil {
		return contracts.GateResult{}, core.ErrTransition
	}
	binding, err := bind(input)
	if err != nil {
		return contracts.GateResult{}, err
	}
	if g == nil || g.store == nil {
		return contracts.GateResult{}, core.ErrPath
	}
	pointer := "gates/" + fingerprint(binding) + ".json"
	result := contracts.GateResult{
		Revision:          binding.Candidate.Revision,
		Result:            "CLEAN",
		Evidence:          pointer,
		CleanEvidence:     binding.ReviewDigest,
		CheckFingerprints: append([]string(nil), binding.Checks...),
	}
	record := evidenceRecord{Binding: binding, Result: result}
	if _, err := g.store.CreateJSON(pointer, record, evidenceLimit); err == nil {
		return result, nil
	} else if !errors.Is(err, store.ErrAlreadyExists) {
		return contracts.GateResult{}, err
	}
	var persisted evidenceRecord
	if err := g.store.ReadJSON(pointer, evidenceLimit, &persisted); err != nil {
		return contracts.GateResult{}, err
	}
	if !reflect.DeepEqual(persisted.Binding, binding) || !validResult(persisted.Result, result) {
		return contracts.GateResult{}, core.ErrRevision
	}
	return persisted.Result, nil
}

func bind(input contracts.GateInput) (gateBinding, error) {
	if input.WorktreeDirty || input.Candidate.Worktree.Dirty || input.ReceiptDigest == "" || input.ReviewDigest == "" {
		return gateBinding{}, core.ErrTransition
	}
	if input.Run == "" || input.Task == "" || input.TrackerRevision == 0 || input.ReceiptRevision == 0 || input.ScopeFingerprint == "" {
		return gateBinding{}, core.ErrRevision
	}
	candidate, worktree := input.Candidate, input.Candidate.Worktree
	if candidate.Task == "" || candidate.Revision == "" || candidate.Base == "" || candidate.Task != input.Task ||
		worktree.Run != input.Run || worktree.Run == "" || worktree.Team == "" || worktree.Path == "" || worktree.Branch == "" ||
		worktree.Base != candidate.Base || (worktree.Candidate != "" && worktree.Candidate != candidate.Revision) ||
		(worktree.Canonical != "" && worktree.Canonical != candidate.Revision) {
		return gateBinding{}, core.ErrRevision
	}
	checks, err := normalizedChecks(input.RequiredCheckFingerprints)
	if err != nil {
		return gateBinding{}, err
	}
	return gateBinding{
		Run: input.Run, Task: input.Task, Candidate: candidate,
		TrackerRevision: input.TrackerRevision, ReceiptRevision: input.ReceiptRevision,
		Checks: checks, ScopeDigest: input.ScopeFingerprint, ReceiptDigest: input.ReceiptDigest, ReviewDigest: input.ReviewDigest,
	}, nil
}

func normalizedChecks(checks []string) ([]string, error) {
	normalized := append([]string(nil), checks...)
	sort.Strings(normalized)
	for index, check := range normalized {
		if strings.TrimSpace(check) == "" || index > 0 && check == normalized[index-1] {
			return nil, core.ErrRevision
		}
	}
	return normalized, nil
}

func fingerprint(binding gateBinding) string {
	canonical, _ := json.Marshal(binding)
	sum := sha256.Sum256(canonical)
	return hex.EncodeToString(sum[:])
}

func validResult(actual, expected contracts.GateResult) bool {
	return actual.Revision == expected.Revision && actual.Result == expected.Result && actual.Evidence == expected.Evidence &&
		actual.CleanEvidence == expected.CleanEvidence && reflect.DeepEqual(actual.CheckFingerprints, expected.CheckFingerprints)
}
