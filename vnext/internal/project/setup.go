package project

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/thebpandey/agent-team/vnext/internal/core"
	"github.com/thebpandey/agent-team/vnext/internal/store"
)

const (
	configPath  = ".agent-team/config.json"
	receiptPath = ".agent-team/receipts/setup.json"
	commitPath  = ".agent-team/setup/commit.json"
	pendingPath = ".agent-team/setup/pending.json"
	lockPath    = ".agent-team/setup/initialize.lock"
)

type setupService struct {
	store     *store.Store
	writeJSON func(string, any, int64) (store.AtomicResult, error) // test-only fault seam
}

type setupReceipt struct {
	core.RecordEnvelope
	InputDigest     string              `json:"inputDigest"`
	ArtifactDigests map[string]string   `json:"artifactDigests"`
	Handoff         core.KickoffHandoff `json:"handoff"`
	ConfigDigest    string              `json:"configDigest"`
}

// configRecord deliberately keeps Config's published shape while adding the
// durable record envelope required to bind it to this project and revision.
type configRecord struct {
	Schema    int                `json:"schema"`
	Project   string             `json:"project"`
	RunID     core.RunID         `json:"runId"`
	WrittenAt string             `json:"writtenAt"`
	Revision  uint64             `json:"revision"`
	Runtime   core.RuntimeConfig `json:"runtime"`
	Tracker   core.TrackerConfig `json:"tracker"`
	Limits    core.Limits        `json:"limits"`
	Storage   core.StorageLimits `json:"storage"`
}

type setupCommit struct {
	InputDigest, ConfigDigest, ReceiptDigest string
	Revision                                 uint64
}

type setupPending struct {
	Config  configRecord
	Receipt setupReceipt
	Commit  setupCommit
}

// NewSetupService creates a service rooted at the provided canonical store.
func NewSetupService(s *store.Store) SetupService { return &setupService{store: s} }

// ValidateSetup performs every setup decision and input-file check without
// creating files. It is suitable for a caller that has not yet initialized.
func ValidateSetup(ctx context.Context, input SetupInput) (SetupResult, error) {
	project, err := Discover(ctx, input.Root)
	if err != nil {
		return SetupResult{}, err
	}
	if input.Mode != PlanMode && input.Mode != OneOffMode {
		return SetupResult{}, fmt.Errorf("%w: unsupported run mode %q", core.ErrSettings, input.Mode)
	}

	result := SetupResult{
		Project: project, Config: core.DefaultConfig(), ArtifactDigests: make(map[string]string),
		Handoff: core.KickoffHandoff{ApprovedPlanRevision: project.Head, Branch: branch(ctx, project), TrackerKind: "tasks-md", TrackerRef: "TASKS.md"},
	}
	seen := make(map[string]struct{}, len(input.Artifacts))
	for _, artifact := range input.Artifacts {
		relative, fullPath, err := inputPath(project.Root, artifact.Path)
		if err != nil {
			return SetupResult{}, err
		}
		if _, ok := seen[relative]; ok {
			return SetupResult{}, fmt.Errorf("%w: duplicate artifact %q", core.ErrSettings, artifact.Path)
		}
		seen[relative] = struct{}{}
		if artifact.Confirmation != Approved {
			return SetupResult{}, fmt.Errorf("%w: artifact %q was not approved", core.ErrSettings, artifact.Path)
		}
		switch artifact.Mode {
		case ExistingArtifact:
			digest, err := digestFile(project.Root, relative, result.Config.Storage.CanonicalBytes)
			if err != nil {
				return SetupResult{}, err
			}
			result.ArtifactDigests[relative] = digest
		case GeneratedArtifact:
			if _, err := os.Lstat(fullPath); err == nil {
				return SetupResult{}, fmt.Errorf("%w: generated artifact %q already exists", core.ErrSettings, artifact.Path)
			} else if !errors.Is(err, os.ErrNotExist) {
				return SetupResult{}, fmt.Errorf("%w: inspect generated artifact %q: %v", core.ErrPath, artifact.Path, err)
			}
			result.ArtifactDigests[relative] = digestText("generated:" + relative)
		default:
			return SetupResult{}, fmt.Errorf("%w: unsupported artifact mode %q", core.ErrSettings, artifact.Mode)
		}
		if relative == "TASKS.md" {
			result.Config.Tracker = core.TrackerConfig{Kind: "tasks-md", Path: relative}
			result.Handoff.TrackerKind, result.Handoff.TrackerRef = "tasks-md", relative
		}
	}
	if input.Mode == OneOffMode && len(input.Artifacts) != 0 {
		return SetupResult{}, fmt.Errorf("%w: one-off setup cannot select tracker artifacts", core.ErrSettings)
	}
	if input.Mode == PlanMode {
		if err := validatePlanArtifacts(seen); err != nil {
			return SetupResult{}, err
		}
	}
	if input.Kickoff != nil {
		if input.Kickoff.Confirmation != Approved {
			return SetupResult{}, fmt.Errorf("%w: Project Kickoff handoff was not approved", core.ErrSettings)
		}
		relative, _, err := inputPath(project.Root, input.Kickoff.Path)
		if err != nil {
			return SetupResult{}, err
		}
		contents, err := readBoundedContained(project.Root, relative, result.Config.Storage.CanonicalBytes)
		if err != nil {
			return SetupResult{}, fmt.Errorf("%w: read Project Kickoff handoff: %v", core.ErrSettings, err)
		}
		actual := digestBytes(contents)
		if !validDigest(input.Kickoff.Digest) || input.Kickoff.Digest != actual {
			return SetupResult{}, fmt.Errorf("%w: Project Kickoff digest mismatch", core.ErrSettings)
		}
		var handoff core.KickoffHandoff
		if err := json.Unmarshal(contents, &handoff); err != nil {
			return SetupResult{}, fmt.Errorf("%w: malformed Project Kickoff handoff: %v", core.ErrSettings, err)
		}
		if err := validateHandoff(project.Root, handoff); err != nil {
			return SetupResult{}, err
		}
		result.Handoff = handoff
		if handoff.TrackerKind == "tasks-md" || handoff.TrackerKind == "beads" {
			result.Config.Tracker = core.TrackerConfig{Kind: handoff.TrackerKind, Path: handoff.TrackerRef}
		}
	}
	return result, nil
}

func validatePlanArtifacts(artifacts map[string]struct{}) error {
	trackers := 0
	if _, ok := artifacts["TASKS.md"]; ok {
		trackers++
	}
	if _, ok := artifacts[".beads"]; ok {
		trackers++
	}
	if trackers != 1 {
		return fmt.Errorf("%w: plan mode requires exactly one approved tracker authority", core.ErrSettings)
	}
	for _, required := range []string{"DECISIONS.md", "AGENT_TEAM_RULES.md"} {
		if _, ok := artifacts[required]; !ok {
			return fmt.Errorf("%w: plan mode requires approved %s", core.ErrSettings, required)
		}
	}
	return nil
}

// Validate reads already initialized records when present and compares their
// immutable setup digest. It does not create, replace, or remove anything.
func (s *setupService) Validate(ctx context.Context, input SetupInput) (SetupResult, error) {
	result, err := ValidateSetup(ctx, input)
	if err != nil || s == nil || s.store == nil {
		return result, err
	}
	if input.Mode == OneOffMode {
		return result, nil
	}
	var commit setupCommit
	if err := s.store.ReadJSON(commitPath, result.Config.Storage.CanonicalBytes, &commit); errors.Is(err, os.ErrNotExist) {
		// Partially published canonical records are intentionally invisible.
		return result, nil
	} else if err != nil {
		return SetupResult{}, err
	}
	config, receipt, err := s.readCommitted(result.Project.TopLevel, commit)
	if err != nil {
		return SetupResult{}, err
	}
	inputDigest, err := setupDigest(result, input)
	if err != nil {
		return SetupResult{}, err
	}
	if receipt.InputDigest != inputDigest {
		return SetupResult{}, fmt.Errorf("%w: setup input or config differs from initialized revision", core.ErrRevision)
	}
	result.Config, result.Handoff = config.toConfig(), receipt.Handoff
	result.ConfigRevision, result.ReceiptPath = receipt.Revision, receiptPath
	return result, nil
}

// Initialize writes the immutable configuration and setup receipt only after
// all inputs have been validated. Each canonical record is written atomically
// by Store; an identical setup does not write either record again.
func (s *setupService) Initialize(ctx context.Context, input SetupInput) (SetupResult, error) {
	if s == nil || s.store == nil {
		return SetupResult{}, fmt.Errorf("%w: setup store is required", core.ErrSettings)
	}
	// Validate before creating even the protocol lock directory: a refusal or
	// invalid matrix has a strict no-write guarantee.
	preflight, err := ValidateSetup(ctx, input)
	if err != nil {
		return SetupResult{}, err
	}
	if input.Mode == OneOffMode {
		return preflight, nil
	}
	project, err := Discover(ctx, input.Root)
	if err != nil {
		return SetupResult{}, err
	}
	if canonicalStore, err := Contain(project.Root, s.store.Root); err != nil || canonicalStore != project.Root {
		return SetupResult{}, fmt.Errorf("%w: setup store must be the project root", core.ErrPath)
	}
	return s.withLock(ctx, project.Root, func() (SetupResult, error) {
		if err := s.recoverPending(project.TopLevel); err != nil {
			return SetupResult{}, err
		}
		result, err := ValidateSetup(ctx, input)
		if err != nil {
			return SetupResult{}, err
		}
		inputDigest, err := setupDigest(result, input)
		if err != nil {
			return SetupResult{}, err
		}
		var commit setupCommit
		if err := s.store.ReadJSON(commitPath, result.Config.Storage.CanonicalBytes, &commit); err == nil {
			_, receipt, err := s.readCommitted(result.Project.TopLevel, commit)
			if err != nil {
				return SetupResult{}, err
			}
			if receipt.InputDigest != inputDigest {
				return SetupResult{}, fmt.Errorf("%w: setup is already initialized at revision %d", core.ErrRevision, commit.Revision)
			}
			return s.Validate(ctx, input)
		} else if !errors.Is(err, os.ErrNotExist) {
			return SetupResult{}, err
		}
		record := configFrom(result.Config, result.Project.TopLevel, 1)
		receipt := setupReceipt{RecordEnvelope: core.RecordEnvelope{Schema: 1, Project: result.Project.TopLevel, WrittenAt: time.Now().UTC().Format(time.RFC3339Nano), Revision: 1}, InputDigest: inputDigest, ArtifactDigests: result.ArtifactDigests, Handoff: result.Handoff}
		receipt.ConfigDigest = digestRecord(record)
		pending := setupPending{Config: record, Receipt: receipt, Commit: setupCommit{InputDigest: inputDigest, ConfigDigest: digestRecord(record), ReceiptDigest: digestReceipt(receipt), Revision: 1}}
		if _, err := s.write(pendingPath, pending, result.Config.Storage.CanonicalBytes); err != nil {
			return SetupResult{}, err
		}
		if err := s.publishPending(pending); err != nil {
			return SetupResult{}, err
		}
		result.ConfigRevision, result.ReceiptPath = 1, receiptPath
		return result, nil
	})
}

// withLock is a bounded cross-process initialization CAS. The lock is not a
// durable authority: a stale lock is reclaimed only after its bounded lease,
// and the pending bundle below remains the recovery authority after a crash.
func (s *setupService) withLock(ctx context.Context, root string, action func() (SetupResult, error)) (SetupResult, error) {
	deadline := time.Now().Add(10 * time.Second)
	for {
		if err := ctx.Err(); err != nil {
			return SetupResult{}, err
		}
		if err := os.MkdirAll(filepath.Join(root, ".agent-team", "setup"), 0o700); err != nil {
			return SetupResult{}, err
		}
		file, err := os.OpenFile(filepath.Join(root, filepath.FromSlash(lockPath)), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err == nil {
			_, _ = file.WriteString(time.Now().UTC().Format(time.RFC3339Nano))
			_ = file.Close()
			defer os.Remove(filepath.Join(root, filepath.FromSlash(lockPath)))
			return action()
		}
		if !errors.Is(err, os.ErrExist) {
			return SetupResult{}, err
		}
		if info, statErr := os.Stat(filepath.Join(root, filepath.FromSlash(lockPath))); statErr == nil && time.Since(info.ModTime()) > time.Minute {
			// Only this exact protocol-owned file is ever removed.
			_ = os.Remove(filepath.Join(root, filepath.FromSlash(lockPath)))
			continue
		}
		if time.Now().After(deadline) {
			return SetupResult{}, fmt.Errorf("%w: setup initialization is in progress", core.ErrRevision)
		}
		select {
		case <-ctx.Done():
			return SetupResult{}, ctx.Err()
		case <-time.After(10 * time.Millisecond):
		}
	}
}

func (s *setupService) recoverPending(project string) error {
	var pending setupPending
	if err := s.store.ReadJSON(pendingPath, core.DefaultConfig().Storage.CanonicalBytes, &pending); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	if pending.Config.Project != project || pending.Config.Revision == 0 || pending.Receipt.Project != project || pending.Receipt.Revision != pending.Config.Revision || pending.Commit.ConfigDigest != digestRecord(pending.Config) || pending.Commit.ReceiptDigest != digestReceipt(pending.Receipt) || pending.Commit.InputDigest != pending.Receipt.InputDigest || pending.Commit.Revision != pending.Config.Revision {
		return fmt.Errorf("%w: malformed pending setup publication", core.ErrRevision)
	}
	return s.publishPending(pending)
}

// publishPending makes the pair visible at one logical instant: only the
// commit record makes config+receipt authoritative. Before it appears, readers
// treat either canonical file as absent. A retained pending bundle lets a later
// initializer finish the same publication after interruption.
func (s *setupService) publishPending(pending setupPending) error {
	if err := s.ensureConfig(pending.Config); err != nil {
		return err
	}
	if err := s.ensureReceipt(pending.Receipt); err != nil {
		return err
	}
	var commit setupCommit
	if err := s.store.ReadJSON(commitPath, core.DefaultConfig().Storage.CanonicalBytes, &commit); err == nil {
		if commit != pending.Commit {
			return fmt.Errorf("%w: conflicting setup commit", core.ErrRevision)
		}
	} else if errors.Is(err, os.ErrNotExist) {
		if _, err := s.write(commitPath, pending.Commit, core.DefaultConfig().Storage.CanonicalBytes); err != nil {
			return err
		}
	} else {
		return err
	}
	// Failure to remove this owned staging file is harmless: commit wins and a
	// later recovery observes the matching commit without changing it.
	_ = os.Remove(filepath.Join(s.store.Root, filepath.FromSlash(pendingPath)))
	return nil
}

func (s *setupService) ensureConfig(want configRecord) error {
	var got configRecord
	if err := s.store.ReadJSON(configPath, core.DefaultConfig().Storage.CanonicalBytes, &got); err == nil {
		if digestRecord(got) != digestRecord(want) {
			return fmt.Errorf("%w: conflicting uncommitted config", core.ErrRevision)
		}
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	_, err := s.write(configPath, want, core.DefaultConfig().Storage.CanonicalBytes)
	return err
}

func (s *setupService) ensureReceipt(want setupReceipt) error {
	var got setupReceipt
	if err := s.store.ReadJSON(receiptPath, core.DefaultConfig().Storage.CanonicalBytes, &got); err == nil {
		if digestReceipt(got) != digestReceipt(want) {
			return fmt.Errorf("%w: conflicting uncommitted receipt", core.ErrRevision)
		}
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	_, err := s.write(receiptPath, want, core.DefaultConfig().Storage.CanonicalBytes)
	return err
}

func (s *setupService) readCommitted(project string, commit setupCommit) (configRecord, setupReceipt, error) {
	if commit.Revision == 0 || !validDigest(commit.InputDigest) || !validDigest(commit.ConfigDigest) || !validDigest(commit.ReceiptDigest) {
		return configRecord{}, setupReceipt{}, fmt.Errorf("%w: malformed setup commit", core.ErrRevision)
	}
	var config configRecord
	var receipt setupReceipt
	if err := s.store.ReadJSON(configPath, core.DefaultConfig().Storage.CanonicalBytes, &config); err != nil {
		return configRecord{}, setupReceipt{}, fmt.Errorf("%w: committed config unavailable", core.ErrRevision)
	}
	if err := s.store.ReadJSON(receiptPath, core.DefaultConfig().Storage.CanonicalBytes, &receipt); err != nil {
		return configRecord{}, setupReceipt{}, fmt.Errorf("%w: committed receipt unavailable", core.ErrRevision)
	}
	if config.Project != project || config.Schema != 1 || config.Revision != commit.Revision || digestRecord(config) != commit.ConfigDigest || receipt.InputDigest != commit.InputDigest || digestReceipt(receipt) != commit.ReceiptDigest || receipt.ConfigDigest != commit.ConfigDigest || receipt.Revision != config.Revision || receipt.Project != config.Project {
		return configRecord{}, setupReceipt{}, fmt.Errorf("%w: config and receipt do not match committed revision", core.ErrRevision)
	}
	return config, receipt, nil
}

func configFrom(config core.Config, project string, revision uint64) configRecord {
	return configRecord{Schema: 1, Project: project, WrittenAt: time.Now().UTC().Format(time.RFC3339Nano), Revision: revision, Runtime: config.Runtime, Tracker: config.Tracker, Limits: config.Limits, Storage: config.Storage}
}

func (record configRecord) toConfig() core.Config {
	return core.Config{Schema: record.Schema, Runtime: record.Runtime, Tracker: record.Tracker, Limits: record.Limits, Storage: record.Storage}
}

func digestRecord(record configRecord) string {
	encoded, _ := json.Marshal(record)
	return digestBytes(encoded)
}
func digestReceipt(receipt setupReceipt) string {
	encoded, _ := json.Marshal(receipt)
	return digestBytes(encoded)
}

func (s *setupService) write(relative string, value any, limit int64) (store.AtomicResult, error) {
	if s.writeJSON != nil {
		return s.writeJSON(relative, value, limit)
	}
	return s.store.WriteJSON(relative, value, limit)
}

func inputPath(root, value string) (string, string, error) {
	parts, err := relativeSegments(value)
	if err != nil {
		return "", "", err
	}
	full, err := Contain(root, filepath.Join(append([]string{root}, parts...)...))
	if err != nil {
		return "", "", err
	}
	relative, err := filepath.Rel(root, full)
	if err != nil || relative == "." || strings.HasPrefix(relative, "..") {
		return "", "", fmt.Errorf("%w: artifact %q escapes root", core.ErrPath, value)
	}
	return filepath.ToSlash(relative), full, nil
}

func digestFile(root, relative string, limit int64) (string, error) {
	contents, err := readBoundedContained(root, relative, limit)
	if err != nil {
		return "", fmt.Errorf("%w: existing artifact %q: %v", core.ErrSettings, relative, err)
	}
	return digestBytes(contents), nil
}

// readBoundedContained uses os.Root for the final open, so a symlink/reparse
// swap after path validation cannot redirect a validated artifact outside root.
func readBoundedContained(rootPath, relative string, limit int64) ([]byte, error) {
	root, err := os.OpenRoot(rootPath)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	info, err := root.Lstat(filepath.FromSlash(relative))
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return nil, fmt.Errorf("not a regular file")
	}
	file, err := root.Open(filepath.FromSlash(relative))
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return readBoundedFile(file, limit)
}

func readBounded(name string, limit int64) ([]byte, error) {
	if limit <= 0 {
		return nil, fmt.Errorf("invalid byte limit")
	}
	file, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return readBoundedFile(file, limit)
}

func readBoundedFile(file *os.File, limit int64) ([]byte, error) {
	contents, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(contents)) > limit {
		return nil, fmt.Errorf("%w: exceeds %d bytes", core.ErrLimit, limit)
	}
	return contents, nil
}

func validateHandoff(root string, handoff core.KickoffHandoff) error {
	if handoff.ApprovedPlanRevision == "" || handoff.Branch == "" || handoff.TrackerKind == "" || handoff.TrackerRef == "" {
		return fmt.Errorf("%w: incomplete Project Kickoff handoff", core.ErrSettings)
	}
	if len(handoff.TaskIDs) == 0 || len(handoff.Acceptance) == 0 || len(handoff.Checks) == 0 || len(handoff.WritablePaths) == 0 || len(handoff.Resources) == 0 || len(handoff.Capabilities) == 0 {
		return fmt.Errorf("%w: Project Kickoff handoff lacks required approved facts", core.ErrSettings)
	}
	if handoff.TrackerKind != "tasks-md" && handoff.TrackerKind != "beads" {
		return fmt.Errorf("%w: unsupported Project Kickoff tracker %q", core.ErrSettings, handoff.TrackerKind)
	}
	if _, _, err := inputPath(root, handoff.TrackerRef); err != nil {
		return fmt.Errorf("%w: invalid Project Kickoff tracker reference: %v", core.ErrSettings, err)
	}
	for _, path := range append(append([]string{}, handoff.WritablePaths...), handoff.Resources...) {
		if path == "" {
			return fmt.Errorf("%w: empty Project Kickoff fact", core.ErrSettings)
		}
	}
	for _, task := range handoff.TaskIDs {
		if task == "" {
			return fmt.Errorf("%w: empty Project Kickoff task ID", core.ErrSettings)
		}
	}
	for _, check := range handoff.Checks {
		if check.Name == "" || len(check.Command) == 0 {
			return fmt.Errorf("%w: malformed Project Kickoff check", core.ErrSettings)
		}
	}
	return nil
}

func validateReceipt(receipt setupReceipt, projectRoot string) error {
	if receipt.Schema != 1 || receipt.Project != projectRoot || receipt.Revision == 0 || receipt.WrittenAt == "" || receipt.InputDigest == "" || !validDigest(receipt.ConfigDigest) {
		return fmt.Errorf("%w: malformed setup receipt", core.ErrRevision)
	}
	return nil
}

func setupDigest(result SetupResult, input SetupInput) (string, error) {
	artifacts := make([]ArtifactDecision, len(input.Artifacts))
	copy(artifacts, input.Artifacts)
	sort.Slice(artifacts, func(i, j int) bool { return artifacts[i].Path < artifacts[j].Path })
	value := struct {
		Mode            RunMode
		Artifacts       []ArtifactDecision
		ArtifactDigests map[string]string
		Handoff         core.KickoffHandoff
		Config          core.Config
	}{input.Mode, artifacts, result.ArtifactDigests, result.Handoff, result.Config}
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return digestBytes(encoded), nil
}

func digestConfig(config core.Config) string {
	encoded, _ := json.Marshal(config)
	return digestBytes(encoded)
}

func digestText(value string) string { return digestBytes([]byte(value)) }

func digestBytes(value []byte) string {
	sum := sha256.Sum256(value)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func validDigest(value string) bool {
	if !strings.HasPrefix(value, "sha256:") || len(value) != len("sha256:")+64 {
		return false
	}
	_, err := hex.DecodeString(strings.TrimPrefix(value, "sha256:"))
	return err == nil && value == strings.ToLower(value)
}

func branch(ctx context.Context, project Project) string {
	value, err := git(ctx, project.Root, "symbolic-ref", "-q", "--short", "HEAD")
	if err != nil {
		return "detached"
	}
	return value
}
