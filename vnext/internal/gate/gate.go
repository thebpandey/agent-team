// Package gate binds a reviewed candidate to deterministic completion evidence.
package gate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/thebpandey/agent-team/vnext/internal/contracts"
	"github.com/thebpandey/agent-team/vnext/internal/core"
	"github.com/thebpandey/agent-team/vnext/internal/host"
	"github.com/thebpandey/agent-team/vnext/internal/knowledge"
	"github.com/thebpandey/agent-team/vnext/internal/run"
	"github.com/thebpandey/agent-team/vnext/internal/store"
)

const evidenceLimit = 64 << 10

// Gate checks a candidate's revision-bound receipt and review evidence.
type Gate interface {
	Check(context.Context, contracts.GateInput) (contracts.GateResult, error)
}

type completionGate struct {
	runner    host.CommandRunner
	store     *store.Store
	authority EvidenceAuthority
	canonical bool
}

type evidenceRecord struct {
	Envelope core.RecordEnvelope  `json:"envelope"`
	Binding  gateBinding          `json:"binding"`
	Result   contracts.GateResult `json:"result"`
	Digest   string               `json:"digest"`
}

type gateBinding struct {
	Project         string              `json:"project"`
	Canonical       bool                `json:"canonical"`
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

// EvidenceAuthority resolves durable receipt, independent review, and passed
// check evidence before the gate can create CLEAN evidence.
type EvidenceAuthority interface {
	Validate(context.Context, contracts.GateInput) (core.RecordEnvelope, error)
}

// NewGate creates a deterministic, foreground completion gate. The runner is
// retained for later concrete check execution; this boundary receives only
// already-fingerprinted checks and never interprets them as shell commands.
func NewGate(runner host.CommandRunner, state *store.Store) Gate {
	return &completionGate{runner: runner, store: state, authority: storeAuthority{store: state}, canonical: true}
}

// NewGateWithAuthority is a narrow injection point for durable evidence
// backends. It does not grant caller-provided fingerprints any authority.
func NewGateWithAuthority(runner host.CommandRunner, state *store.Store, authority EvidenceAuthority) Gate {
	return &completionGate{runner: runner, store: state, authority: authority}
}

func (g *completionGate) Check(ctx context.Context, input contracts.GateInput) (contracts.GateResult, error) {
	if ctx == nil || ctx.Err() != nil {
		return contracts.GateResult{}, core.ErrTransition
	}
	if g == nil || g.store == nil {
		return contracts.GateResult{}, core.ErrPath
	}
	if g.authority == nil {
		return contracts.GateResult{}, core.ErrRevision
	}
	provided, err := normalizedChecks(input.RequiredCheckFingerprints)
	if err != nil {
		return contracts.GateResult{}, err
	}
	if _, err := bind(input, provided); err != nil {
		return contracts.GateResult{}, err
	}
	binding, err := bind(input, provided)
	if err != nil {
		return contracts.GateResult{}, err
	}
	if g.canonical {
		binding, err = canonicalBinding(ctx, g.store, input)
		if err != nil {
			return contracts.GateResult{}, err
		}
		binding.Canonical = true
	}
	canonicalInput := input
	canonicalInput.RequiredCheckFingerprints = append([]string(nil), binding.Checks...)
	envelope, err := g.authority.Validate(ctx, canonicalInput)
	if err != nil {
		return contracts.GateResult{}, err
	}
	if !binding.Canonical {
		binding.Project = envelope.Project
	}
	if err := validateEnvelope(envelope, binding); err != nil {
		return contracts.GateResult{}, err
	}
	pointer := "gates/" + fingerprint(binding) + ".json"
	result := contracts.GateResult{
		Revision:          binding.Candidate.Revision,
		Result:            "CLEAN",
		Evidence:          pointer,
		CleanEvidence:     binding.ReviewDigest,
		CheckFingerprints: append([]string(nil), binding.Checks...),
	}
	record := evidenceRecord{Envelope: envelope, Binding: binding, Result: result}
	record.Digest = recordDigest(record)
	if _, err := g.store.CreateJSON(pointer, record, evidenceLimit); err == nil {
		return result, nil
	} else if !errors.Is(err, store.ErrAlreadyExists) {
		return contracts.GateResult{}, err
	}
	var persisted evidenceRecord
	if err := g.store.ReadJSON(pointer, evidenceLimit, &persisted); err != nil {
		return contracts.GateResult{}, err
	}
	if !reflect.DeepEqual(persisted.Binding, binding) || !validResult(persisted.Result, result) || persisted.Digest != recordDigest(persisted) || validateEnvelope(persisted.Envelope, binding) != nil {
		return contracts.GateResult{}, core.ErrRevision
	}
	return persisted.Result, nil
}

// ValidateEvidence resolves and validates a tamper-evident durable gate record
// for exactly candidate. Integrators must call this instead of trusting a
// caller-supplied GateResult.
func ValidateEvidence(state *store.Store, candidate contracts.Candidate, result contracts.GateResult) error {
	if state == nil || result.Evidence == "" {
		return core.ErrRevision
	}
	var record evidenceRecord
	if err := state.ReadJSON(result.Evidence, evidenceLimit, &record); err != nil {
		return fmt.Errorf("%w: gate evidence unavailable: %v", core.ErrRevision, err)
	}
	if record.Digest != recordDigest(record) || record.Binding.Candidate != candidate ||
		record.Result.Evidence != result.Evidence || !validResult(record.Result, result) ||
		result.Evidence != "gates/"+fingerprint(record.Binding)+".json" ||
		(record.Binding.Canonical && validateCanonicalBinding(state, record.Binding) != nil) ||
		validateEnvelope(record.Envelope, record.Binding) != nil {
		return core.ErrRevision
	}
	if record.Binding.Canonical {
		input := contracts.GateInput{Run: record.Binding.Run, Task: record.Binding.Task, Candidate: candidate,
			TrackerRevision: record.Binding.TrackerRevision, ReceiptRevision: record.Binding.ReceiptRevision,
			RequiredCheckFingerprints: record.Binding.Checks, ScopeFingerprint: record.Binding.ScopeDigest,
			ReceiptDigest: record.Binding.ReceiptDigest, ReviewDigest: record.Binding.ReviewDigest}
		envelope, err := (storeAuthority{store: state}).Validate(context.Background(), input)
		if err != nil || validateEnvelope(envelope, record.Binding) != nil {
			return core.ErrRevision
		}
	}
	return nil
}

// CanonicalEvidence reports whether an already validated record was produced
// by NewGate's canonical-run authority. Injection-only test authorities retain
// their legacy isolated behavior and are never accepted as canonical evidence.
func CanonicalEvidence(state *store.Store, candidate contracts.Candidate, result contracts.GateResult) (bool, error) {
	if err := ValidateEvidence(state, candidate, result); err != nil {
		return false, err
	}
	var record evidenceRecord
	if err := state.ReadJSON(result.Evidence, evidenceLimit, &record); err != nil {
		return false, core.ErrRevision
	}
	return record.Binding.Canonical, nil
}

func canonicalBinding(ctx context.Context, state *store.Store, input contracts.GateInput) (gateBinding, error) {
	if state == nil {
		return gateBinding{}, core.ErrPath
	}
	manifest, err := run.NewRepositories(state).Runs.Read(ctx, input.Run)
	if err != nil {
		return gateBinding{}, fmt.Errorf("%w: canonical run unavailable: %v", core.ErrRevision, err)
	}
	var task core.Task
	found := false
	for _, item := range manifest.Tasks {
		if item.ID == input.Task {
			if found {
				return gateBinding{}, core.ErrRevision
			}
			task, found = item, true
		}
	}
	if !found || manifest.Project == "" || input.TrackerRevision != manifest.TrackerRevision {
		return gateBinding{}, core.ErrRevision
	}
	checks, err := canonicalChecks(task.Checks)
	if err != nil {
		return gateBinding{}, err
	}
	provided, err := normalizedChecks(input.RequiredCheckFingerprints)
	if err != nil || !reflect.DeepEqual(provided, checks) {
		return gateBinding{}, core.ErrRevision
	}
	binding, err := bind(input, checks)
	if err != nil {
		return gateBinding{}, err
	}
	binding.Project = manifest.Project
	return binding, nil
}

func validateCanonicalBinding(state *store.Store, binding gateBinding) error {
	input := contracts.GateInput{Run: binding.Run, Task: binding.Task, Candidate: binding.Candidate,
		TrackerRevision: binding.TrackerRevision, ReceiptRevision: binding.ReceiptRevision,
		RequiredCheckFingerprints: binding.Checks, ScopeFingerprint: binding.ScopeDigest,
		ReceiptDigest: binding.ReceiptDigest, ReviewDigest: binding.ReviewDigest}
	expected, err := canonicalBinding(context.Background(), state, input)
	expected.Canonical = true
	if err != nil || !reflect.DeepEqual(expected, binding) {
		return core.ErrRevision
	}
	return nil
}

func bind(input contracts.GateInput, checks []string) (gateBinding, error) {
	if input.WorktreeDirty || input.Candidate.Worktree.Dirty || input.ReceiptDigest == "" || input.ReviewDigest == "" {
		return gateBinding{}, core.ErrTransition
	}
	if input.Run == "" || input.Task == "" || input.ReceiptRevision == 0 || input.ScopeFingerprint == "" {
		return gateBinding{}, core.ErrRevision
	}
	candidate, worktree := input.Candidate, input.Candidate.Worktree
	if candidate.Task == "" || candidate.Revision == "" || candidate.Base == "" || candidate.Task != input.Task ||
		worktree.Run != input.Run || worktree.Run == "" || worktree.Team == "" || worktree.Path == "" || worktree.Branch == "" ||
		worktree.Base != candidate.Base || (worktree.Candidate != "" && worktree.Candidate != candidate.Revision) ||
		(worktree.Canonical != "" && worktree.Canonical != candidate.Revision) {
		return gateBinding{}, core.ErrRevision
	}
	return gateBinding{
		Run: input.Run, Task: input.Task, Candidate: candidate,
		TrackerRevision: input.TrackerRevision, ReceiptRevision: input.ReceiptRevision,
		Checks: checks, ScopeDigest: input.ScopeFingerprint, ReceiptDigest: input.ReceiptDigest, ReviewDigest: input.ReviewDigest,
	}, nil
}

func canonicalChecks(checks []core.Check) ([]string, error) {
	if len(checks) == 0 {
		return nil, core.ErrRevision
	}
	fingerprints := make([]string, len(checks))
	for index, check := range checks {
		if strings.TrimSpace(check.Name) == "" || len(check.Command) == 0 {
			return nil, core.ErrRevision
		}
		canonical, err := json.Marshal(check)
		if err != nil {
			return nil, core.ErrRevision
		}
		sum := sha256.Sum256(canonical)
		fingerprints[index] = "sha256:" + hex.EncodeToString(sum[:])
	}
	return normalizedChecks(fingerprints)
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

func recordDigest(record evidenceRecord) string {
	record.Digest = ""
	canonical, _ := json.Marshal(record)
	sum := sha256.Sum256(canonical)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func validateEnvelope(envelope core.RecordEnvelope, binding gateBinding) error {
	if envelope.Schema != 1 || envelope.Project != binding.Project || envelope.RunID != binding.Run || envelope.Revision != binding.ReceiptRevision || envelope.WrittenAt == "" {
		return core.ErrRevision
	}
	if _, err := time.Parse(time.RFC3339, envelope.WrittenAt); err != nil {
		return core.ErrRevision
	}
	return nil
}

type storeAuthority struct{ store *store.Store }

func (a storeAuthority) Validate(_ context.Context, input contracts.GateInput) (core.RecordEnvelope, error) {
	if a.store == nil {
		return core.RecordEnvelope{}, core.ErrPath
	}
	team := string(input.Candidate.Worktree.Team)
	if team == "" {
		return core.RecordEnvelope{}, core.ErrRevision
	}
	var receipt knowledge.Receipt
	if err := a.store.ReadJSON(".agent-team/receipts/"+team+".json", evidenceLimit, &receipt); err != nil {
		return core.RecordEnvelope{}, fmt.Errorf("%w: receipt unavailable: %v", core.ErrRevision, err)
	}
	if receipt.Schema != 1 || receipt.Project == "" || receipt.RunID != input.Run || receipt.Revision != input.ReceiptRevision || receipt.WrittenAt == "" ||
		receipt.Team != team || receipt.Task != string(input.Task) || receipt.State != core.Clean || receipt.Base != input.Candidate.Base || receipt.Head != input.Candidate.Revision || receipt.Review != input.ReviewDigest {
		return core.RecordEnvelope{}, core.ErrRevision
	}
	encoded, _ := json.Marshal(receipt)
	sum := sha256.Sum256(encoded)
	if input.ReceiptDigest != "sha256:"+hex.EncodeToString(sum[:]) {
		return core.RecordEnvelope{}, core.ErrRevision
	}
	needed := map[string]bool{}
	for _, fingerprint := range input.RequiredCheckFingerprints {
		needed[fingerprint] = true
	}
	if len(receipt.EvidencePointers) != len(needed) {
		return core.RecordEnvelope{}, core.ErrRevision
	}
	for _, pointer := range receipt.EvidencePointers {
		var evidence knowledge.Evidence
		if err := a.store.ReadJSON(pointer, evidenceLimit, &evidence); err != nil {
			return core.RecordEnvelope{}, core.ErrRevision
		}
		if evidence.Schema != 1 || evidence.Project != receipt.Project || evidence.RunID != receipt.RunID || evidence.Task != receipt.Task || evidence.Attempt < 1 || evidence.Exit != 0 || evidence.TimedOut || evidence.Transport != "" ||
			pointer != ".agent-team/evidence/"+receipt.Task+"/"+strconv.Itoa(evidence.Attempt)+"/evidence.json" || !needed[evidence.InputFingerprint] {
			return core.RecordEnvelope{}, core.ErrRevision
		}
		delete(needed, evidence.InputFingerprint)
	}
	if len(needed) != 0 {
		return core.RecordEnvelope{}, core.ErrRevision
	}
	return receipt.RecordEnvelope, nil
}

func validResult(actual, expected contracts.GateResult) bool {
	return actual.Revision == expected.Revision && actual.Result == expected.Result && actual.Evidence == expected.Evidence &&
		actual.CleanEvidence == expected.CleanEvidence && reflect.DeepEqual(actual.CheckFingerprints, expected.CheckFingerprints)
}
