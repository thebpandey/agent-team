package project

import (
	"context"
	"crypto/rand"
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

	"github.com/thebpandey/agent-team/vnext/internal/core"
	"github.com/thebpandey/agent-team/vnext/internal/store"
)

const (
	configPath = ".agent-team/config.json"
)

type setupService struct {
	store     *store.Store
	writeJSON func(string, any, int64) (store.AtomicResult, error) // test-only fault seam
	link      func(*os.Root, string, string) error                 // test-only CAS seam
}

type setupReceipt struct {
	core.RecordEnvelope
	InputDigest     string              `json:"inputDigest"`
	ArtifactDigests map[string]string   `json:"artifactDigests"`
	Handoff         core.KickoffHandoff `json:"handoff"`
	IgnoredKickoff  *IgnoredKickoff     `json:"ignoredKickoff,omitempty"`
	ConfigDigest    string              `json:"configDigest"`
}

// configRecord deliberately keeps Config's published shape while adding the
// durable record envelope required to bind it to this project and revision.
type configRecord struct {
	Schema        int                `json:"schema"`
	Project       string             `json:"project"`
	RunID         core.RunID         `json:"runId"`
	WrittenAt     string             `json:"writtenAt"`
	Revision      uint64             `json:"revision"`
	Runtime       core.RuntimeConfig `json:"runtime"`
	Tracker       core.TrackerConfig `json:"tracker"`
	Limits        core.Limits        `json:"limits"`
	Storage       core.StorageLimits `json:"storage"`
	ReceiptPath   string             `json:"receiptPath"`
	ReceiptDigest string             `json:"receiptDigest"`
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
	}
	if input.Tracker.Kind != "" {
		if input.Tracker.Kind != "beads" && input.Tracker.Kind != "tasks-md" {
			return SetupResult{}, fmt.Errorf("%w: unsupported tracker %q", core.ErrSettings, input.Tracker.Kind)
		}
		if _, _, err := inputPath(project.Root, input.Tracker.Path); err != nil {
			return SetupResult{}, err
		}
		result.Config.Tracker = input.Tracker
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
			digest, err := digestArtifact(project.Root, relative, fullPath, result.Config.Storage.CanonicalBytes)
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
		if input.Tracker.Kind == "" && relative == "TASKS.md" {
			result.Config.Tracker = core.TrackerConfig{Kind: "tasks-md", Path: relative}
		}
		if input.Tracker.Kind == "" && relative == ".beads" {
			result.Config.Tracker = core.TrackerConfig{Kind: "beads", Path: relative}
		}
	}
	if input.Mode == OneOffMode && len(input.Artifacts) != 0 {
		return SetupResult{}, fmt.Errorf("%w: one-off setup cannot select tracker artifacts", core.ErrSettings)
	}
	if input.Mode == PlanMode {
		if err := validateSelectedPlanArtifacts(seen, result.Config.Tracker); err != nil {
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
		handoff, err := decodeKickoffHandoff(project.Root, contents)
		if err != nil {
			return SetupResult{}, err
		}
		if err := validateHandoff(project.Root, handoff); err != nil {
			return SetupResult{}, err
		}
		result.Handoff = handoff
		if handoff.TrackerKind == "tasks-md" || handoff.TrackerKind == "beads" {
			if handoff.TrackerKind != result.Config.Tracker.Kind || handoff.TrackerRef != result.Config.Tracker.Path {
				return SetupResult{}, fmt.Errorf("%w: kickoff tracker differs from selected tracker", core.ErrSettings)
			}
			result.Config.Tracker = core.TrackerConfig{Kind: handoff.TrackerKind, Path: handoff.TrackerRef}
		}
	}
	if input.IgnoredKickoff != nil {
		ignored := input.IgnoredKickoff
		relative, _, err := inputPath(project.TopLevel, ignored.Path)
		if err != nil || !validDigest(ignored.Digest) || ignored.Decision != "ignored" || strings.TrimSpace(ignored.Reason) == "" || len(ignored.Reason) > 512 {
			return SetupResult{}, fmt.Errorf("%w: malformed ignored Project Kickoff decision", core.ErrSettings)
		}
		contents, err := readBoundedContained(project.TopLevel, relative, kickoffMaxBytes)
		if err != nil {
			return SetupResult{}, fmt.Errorf("%w: ignored Project Kickoff handoff is unavailable", core.ErrSettings)
		}
		if digestBytes(contents) != ignored.Digest {
			return SetupResult{}, fmt.Errorf("%w: ignored Project Kickoff digest mismatch", core.ErrSettings)
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

func validateSelectedPlanArtifacts(artifacts map[string]struct{}, selected core.TrackerConfig) error {
	if selected.Path == "TASKS.md" || selected.Path == ".beads" {
		return validatePlanArtifacts(artifacts)
	}
	if selected.Kind != "tasks-md" {
		return fmt.Errorf("%w: unsupported tracker path", core.ErrSettings)
	}
	for _, path := range []string{selected.Path, "DECISIONS.md", "AGENT_TEAM_RULES.md"} {
		if _, ok := artifacts[path]; !ok {
			return fmt.Errorf("%w: plan mode requires approved %s", core.ErrSettings, path)
		}
	}
	if len(artifacts) != 3 {
		return fmt.Errorf("%w: select exactly one tracker", core.ErrSettings)
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
	var config configRecord
	if err := s.store.ReadJSON(configPath, result.Config.Storage.CanonicalBytes, &config); errors.Is(err, os.ErrNotExist) {
		// Orphan receipts and staging have no authority without config.json.
		return result, nil
	} else if err != nil {
		return SetupResult{}, err
	}
	receipt, err := s.readConfigReceipt(result.Project.TopLevel, config)
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
	result.ConfigRevision, result.ReceiptPath = receipt.Revision, config.ReceiptPath
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
	if err := probeInitializeWritable(preflight.Project.Root); err != nil {
		return SetupResult{}, err
	}
	project, err := Discover(ctx, input.Root)
	if err != nil {
		return SetupResult{}, err
	}
	if canonicalStore, err := Contain(project.Root, s.store.Root); err != nil || canonicalStore != project.Root {
		return SetupResult{}, fmt.Errorf("%w: setup store must be the project root", core.ErrPath)
	}
	return s.withLock(ctx, project.Root, func() (SetupResult, error) {
		result, err := ValidateSetup(ctx, input)
		if err != nil {
			return SetupResult{}, err
		}
		inputDigest, err := setupDigest(result, input)
		if err != nil {
			return SetupResult{}, err
		}
		var existing configRecord
		if err := s.store.ReadJSON(configPath, result.Config.Storage.CanonicalBytes, &existing); err == nil {
			receipt, err := s.readConfigReceipt(result.Project.TopLevel, existing)
			if err != nil {
				return SetupResult{}, err
			}
			if receipt.InputDigest != inputDigest {
				return SetupResult{}, fmt.Errorf("%w: setup is already initialized at revision %d", core.ErrRevision, existing.Revision)
			}
			return s.Validate(ctx, input)
		} else if !errors.Is(err, os.ErrNotExist) {
			return SetupResult{}, err
		}
		receiptFile := ".agent-team/receipts/setup-" + strings.TrimPrefix(inputDigest, "sha256:") + ".json"
		record := configFrom(result.Config, result.Project.TopLevel, 1)
		receipt := setupReceipt{RecordEnvelope: core.RecordEnvelope{Schema: 1, Project: result.Project.TopLevel, WrittenAt: record.WrittenAt, Revision: 1}, InputDigest: inputDigest, ArtifactDigests: result.ArtifactDigests, Handoff: result.Handoff, IgnoredKickoff: input.IgnoredKickoff}
		record.ReceiptPath = receiptFile
		record.ReceiptDigest = digestReceiptBinding(receipt)
		receipt.ConfigDigest = digestRecord(record)
		if err := s.writeImmutableReceipt(receiptFile, receipt); err != nil {
			return SetupResult{}, err
		}
		if err := s.ensureConfig(record); err != nil {
			return SetupResult{}, err
		}
		result.ConfigRevision, result.ReceiptPath = 1, receiptFile
		return result, nil
	})
}

func probeInitializeWritable(rootPath string) error {
	root, err := os.OpenRoot(rootPath)
	if err != nil {
		return err
	}
	defer root.Close()
	var token [16]byte
	if _, err := rand.Read(token[:]); err != nil {
		return err
	}
	name := ".agent-team-probe-" + hex.EncodeToString(token[:])
	file, err := root.OpenFile(name, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("%w: project is not writable: %v", core.ErrPath, err)
	}
	value := []byte(name)
	_, writeErr := file.Write(value)
	syncErr := file.Sync()
	closeErr := file.Close()
	if writeErr != nil || syncErr != nil || closeErr != nil {
		return fmt.Errorf("%w: write project probe", core.ErrPath)
	}
	opened, err := root.Open(name)
	if err != nil {
		return err
	}
	stored, readErr := io.ReadAll(io.LimitReader(opened, int64(len(value)+1)))
	_ = opened.Close()
	if readErr != nil || string(stored) != string(value) {
		return fmt.Errorf("%w: probe ownership changed", core.ErrPath)
	}
	if err := root.Remove(name); err != nil {
		return fmt.Errorf("%w: remove owned project probe: %v", core.ErrPath, err)
	}
	return nil
}

// withLock is retained as a narrow call boundary; publication itself is
// lock-free and uses a rooted no-replace link as its cross-process CAS.
func (s *setupService) withLock(ctx context.Context, root string, action func() (SetupResult, error)) (SetupResult, error) {
	if err := ctx.Err(); err != nil {
		return SetupResult{}, err
	}
	return action()
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
	var token [16]byte
	if _, err := rand.Read(token[:]); err != nil {
		return err
	}
	temporary := ".agent-team/setup/config-" + hex.EncodeToString(token[:]) + ".json"
	if _, err := s.write(temporary, want, core.DefaultConfig().Storage.CanonicalBytes); err != nil {
		return err
	}
	root, err := os.OpenRoot(s.store.Root)
	if err != nil {
		return err
	}
	defer root.Close()
	var linkErr error
	if s.link != nil {
		linkErr = s.link(root, temporary, configPath)
	} else {
		linkErr = root.Link(temporary, configPath)
	}
	if linkErr != nil {
		// Destination-exists is a competing CAS winner; compare its immutable
		// record rather than overwriting it. Other link failures publish nothing.
		var got configRecord
		if readErr := s.store.ReadJSON(configPath, core.DefaultConfig().Storage.CanonicalBytes, &got); readErr == nil && digestRecord(got) == digestRecord(want) {
			s.removeConfigTemp(root, temporary, want)
			return nil
		}
		s.removeConfigTemp(root, temporary, want)
		return fmt.Errorf("%w: setup config CAS: %v", core.ErrRevision, linkErr)
	}
	s.removeConfigTemp(root, temporary, want)
	return nil
}

func (s *setupService) removeConfigTemp(root *os.Root, relative string, want configRecord) {
	var got configRecord
	if err := s.store.ReadJSON(relative, core.DefaultConfig().Storage.CanonicalBytes, &got); err == nil && digestRecord(got) == digestRecord(want) {
		_ = root.Remove(relative)
	}
}

func (s *setupService) writeImmutableReceipt(relative string, want setupReceipt) error {
	var got setupReceipt
	if err := s.store.ReadJSON(relative, core.DefaultConfig().Storage.CanonicalBytes, &got); err == nil {
		if digestReceiptBinding(got) != digestReceiptBinding(want) || got.ConfigDigest != want.ConfigDigest {
			return fmt.Errorf("%w: conflicting immutable setup receipt", core.ErrRevision)
		}
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	_, err := s.write(relative, want, core.DefaultConfig().Storage.CanonicalBytes)
	return err
}

func (s *setupService) readConfigReceipt(project string, config configRecord) (setupReceipt, error) {
	if config.Schema != 1 || config.Project != project || config.Revision == 0 || config.WrittenAt == "" || config.ReceiptPath == "" || !validDigest(config.ReceiptDigest) {
		return setupReceipt{}, fmt.Errorf("%w: malformed committed config", core.ErrRevision)
	}
	var receipt setupReceipt
	if err := s.store.ReadJSON(config.ReceiptPath, core.DefaultConfig().Storage.CanonicalBytes, &receipt); err != nil {
		return setupReceipt{}, fmt.Errorf("%w: committed receipt unavailable", core.ErrRevision)
	}
	if err := validateReceipt(receipt, config.Project); err != nil {
		return setupReceipt{}, err
	}
	expectedPath := ".agent-team/receipts/setup-" + strings.TrimPrefix(receipt.InputDigest, "sha256:") + ".json"
	if config.ReceiptPath != expectedPath || receipt.RunID != config.RunID || receipt.Project != config.Project || receipt.Revision != config.Revision || receipt.ConfigDigest != digestRecord(config) || digestReceiptBinding(receipt) != config.ReceiptDigest {
		return setupReceipt{}, fmt.Errorf("%w: committed config/receipt mismatch", core.ErrRevision)
	}
	return receipt, nil
}

func configFrom(config core.Config, project string, revision uint64) configRecord {
	// The canonical initial configuration is content-addressed. Its timestamp is
	// deliberately stable so equal concurrent initializers produce byte-identical
	// CAS candidates; the receipt records the same immutable initial revision.
	return configRecord{Schema: 1, Project: project, WrittenAt: "1970-01-01T00:00:00Z", Revision: revision, Runtime: config.Runtime, Tracker: config.Tracker, Limits: config.Limits, Storage: config.Storage}
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

func digestReceiptBinding(receipt setupReceipt) string {
	receipt.ConfigDigest = ""
	return digestReceipt(receipt)
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

func digestArtifact(root, relative, full string, limit int64) (string, error) {
	info, err := os.Lstat(full)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		if relative != ".beads" {
			return "", fmt.Errorf("directory artifact is not a tracker authority")
		}
		return digestBeadsIdentity(root, relative, limit)
	}
	return digestFile(root, relative, limit)
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
	opened, err := file.Stat()
	if err != nil || !os.SameFile(info, opened) {
		return nil, fmt.Errorf("artifact identity changed")
	}
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
	if len(handoff.TaskIDs) == 0 || len(handoff.Acceptance) == 0 || len(handoff.Checks) == 0 || len(handoff.WritablePaths) == 0 {
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
		IgnoredKickoff  *IgnoredKickoff
		Config          core.Config
	}{input.Mode, artifacts, result.ArtifactDigests, result.Handoff, input.IgnoredKickoff, result.Config}
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return digestBytes(encoded), nil
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
