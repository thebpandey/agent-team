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
)

type setupService struct{ store *store.Store }

type setupReceipt struct {
	core.RecordEnvelope
	InputDigest     string              `json:"inputDigest"`
	ArtifactDigests map[string]string   `json:"artifactDigests"`
	Handoff         core.KickoffHandoff `json:"handoff"`
	ConfigDigest    string              `json:"configDigest"`
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
		Handoff: core.KickoffHandoff{Branch: branch(ctx, project), TrackerKind: "tasks-md", TrackerRef: "TASKS.md"},
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
			digest, err := digestFile(fullPath, result.Config.Storage.CanonicalBytes)
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
	if input.Kickoff != nil {
		if input.Kickoff.Confirmation != Approved {
			return SetupResult{}, fmt.Errorf("%w: Project Kickoff handoff was not approved", core.ErrSettings)
		}
		_, fullPath, err := inputPath(project.Root, input.Kickoff.Path)
		if err != nil {
			return SetupResult{}, err
		}
		contents, err := readBounded(fullPath, result.Config.Storage.CanonicalBytes)
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

// Validate reads already initialized records when present and compares their
// immutable setup digest. It does not create, replace, or remove anything.
func (s *setupService) Validate(ctx context.Context, input SetupInput) (SetupResult, error) {
	result, err := ValidateSetup(ctx, input)
	if err != nil || s == nil || s.store == nil {
		return result, err
	}
	var config core.Config
	configErr := s.store.ReadJSON(configPath, result.Config.Storage.CanonicalBytes, &config)
	var receipt setupReceipt
	receiptErr := s.store.ReadJSON(receiptPath, result.Config.Storage.CanonicalBytes, &receipt)
	if errors.Is(configErr, os.ErrNotExist) && errors.Is(receiptErr, os.ErrNotExist) {
		return result, nil
	}
	if configErr != nil || receiptErr != nil {
		return SetupResult{}, fmt.Errorf("%w: incomplete setup state", core.ErrRevision)
	}
	if err := validateReceipt(receipt, result.Project.TopLevel); err != nil {
		return SetupResult{}, err
	}
	inputDigest, err := setupDigest(result, input)
	if err != nil {
		return SetupResult{}, err
	}
	if receipt.InputDigest != inputDigest || digestConfig(config) != receipt.ConfigDigest {
		return SetupResult{}, fmt.Errorf("%w: setup input or config differs from initialized revision", core.ErrRevision)
	}
	result.Config, result.Handoff = config, receipt.Handoff
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
	result, err := ValidateSetup(ctx, input)
	if err != nil {
		return SetupResult{}, err
	}
	if canonicalStore, err := Contain(result.Project.Root, s.store.Root); err != nil || canonicalStore != result.Project.Root {
		return SetupResult{}, fmt.Errorf("%w: setup store must be the project root", core.ErrPath)
	}
	var existing setupReceipt
	err = s.store.ReadJSON(receiptPath, result.Config.Storage.CanonicalBytes, &existing)
	if err == nil {
		if err := validateReceipt(existing, result.Project.TopLevel); err != nil {
			return SetupResult{}, err
		}
		inputDigest, err := setupDigest(result, input)
		if err != nil {
			return SetupResult{}, err
		}
		if existing.InputDigest != inputDigest {
			return SetupResult{}, fmt.Errorf("%w: setup is already initialized at revision %d", core.ErrRevision, existing.Revision)
		}
		return s.Validate(ctx, input)
	}
	if !errors.Is(err, os.ErrNotExist) {
		return SetupResult{}, err
	}
	var existingConfig core.Config
	if err := s.store.ReadJSON(configPath, result.Config.Storage.CanonicalBytes, &existingConfig); err == nil || !errors.Is(err, os.ErrNotExist) {
		return SetupResult{}, fmt.Errorf("%w: configuration exists without setup receipt", core.ErrRevision)
	}
	inputDigest, err := setupDigest(result, input)
	if err != nil {
		return SetupResult{}, err
	}
	if _, err := s.store.WriteJSON(configPath, result.Config, result.Config.Storage.CanonicalBytes); err != nil {
		return SetupResult{}, err
	}
	receipt := setupReceipt{
		RecordEnvelope: core.RecordEnvelope{Schema: 1, Project: result.Project.TopLevel, WrittenAt: time.Now().UTC().Format(time.RFC3339Nano), Revision: 1},
		InputDigest:    inputDigest, ArtifactDigests: result.ArtifactDigests, Handoff: result.Handoff, ConfigDigest: digestConfig(result.Config),
	}
	if _, err := s.store.WriteJSON(receiptPath, receipt, result.Config.Storage.CanonicalBytes); err != nil {
		return SetupResult{}, err
	}
	result.ConfigRevision, result.ReceiptPath = receipt.Revision, receiptPath
	return result, nil
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

func digestFile(name string, limit int64) (string, error) {
	info, err := os.Lstat(name)
	if err != nil {
		return "", fmt.Errorf("%w: existing artifact %q: %v", core.ErrSettings, name, err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return "", fmt.Errorf("%w: existing artifact %q is not a regular file", core.ErrSettings, name)
	}
	contents, err := readBounded(name, limit)
	if err != nil {
		return "", fmt.Errorf("%w: existing artifact %q: %v", core.ErrSettings, name, err)
	}
	return digestBytes(contents), nil
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
