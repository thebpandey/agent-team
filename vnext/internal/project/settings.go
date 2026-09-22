package project

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"

	"github.com/thebpandey/agent-team/vnext/internal/core"
	"github.com/thebpandey/agent-team/vnext/internal/migrate"
	"github.com/thebpandey/agent-team/vnext/internal/store"
)

const settingsPath = ".agent-team/v8/settings.json"

// RunDefaults are the settings used only by future run dispatches.
type RunDefaults struct {
	ParallelTeams    int  `json:"parallelTeams"`
	Continuous       bool `json:"continuous"`
	AutoDeploy       bool `json:"autoDeploy"`
	DeployBatchTasks *int `json:"deployBatchTasks"`
}

// Settings is the mutable, receipt-bound project overlay. It never replaces
// the immutable setup config or receipt.
type Settings struct {
	Schema           int         `json:"schema"`
	BaseConfigDigest string      `json:"baseConfigDigest"`
	ReceiptPath      string      `json:"receiptPath"`
	ReceiptDigest    string      `json:"receiptDigest"`
	Revision         uint64      `json:"revision"`
	Defaults         RunDefaults `json:"defaults"`
	CodexDeveloper   RoleProfile `json:"-"`
}

// RoleProfile is the minimal saved Codex routing preference consumed by a
// later native handoff. It does not claim the current host can enforce it.
type RoleProfile struct {
	Model  string `json:"model,omitempty"`
	Effort string `json:"effort,omitempty"`
}

// SettingsService exposes read-only inspection and receipt-bound updates.
type SettingsService interface {
	Inspect(context.Context) (Settings, error)
	Update(context.Context, map[string]string) (Settings, error)
}

type settingsService struct{ store *store.Store }

// NewSettingsService creates the single mutable settings overlay service.
func NewSettingsService(s *store.Store) SettingsService { return &settingsService{store: s} }

func (s *settingsService) Inspect(context.Context) (Settings, error) {
	state, binding, err := s.state()
	if err != nil {
		return Settings{}, err
	}
	return state.inspect(binding)
}

func (s *settingsService) Update(ctx context.Context, updates map[string]string) (Settings, error) {
	if len(updates) == 0 {
		return Settings{}, fmt.Errorf("%w: no settings update", core.ErrSettings)
	}
	state, binding, err := s.state()
	if err != nil {
		return Settings{}, err
	}
	guard, err := store.AcquireProjectMutation(ctx, state.store.Root, "settings", "settings-overlay")
	if err != nil {
		return Settings{}, err
	}
	defer guard.Release()
	// Re-read under the project writer lock so Revision is a real CAS value.
	state, binding, err = s.state()
	if err != nil {
		return Settings{}, err
	}
	record, raw, err := state.load(binding)
	if err != nil {
		return Settings{}, err
	}
	if raw == nil {
		raw = make(map[string]json.RawMessage)
	}
	if err := applySettingsUpdates(&record.Defaults, raw, updates); err != nil {
		return Settings{}, err
	}
	record.Schema = 1
	record.BaseConfigDigest = binding.baseConfigDigest
	record.ReceiptPath = binding.receiptPath
	record.ReceiptDigest = binding.receiptDigest
	record.Revision++
	encodedDefaults, err := json.Marshal(record.Defaults)
	if err != nil {
		return Settings{}, err
	}
	for key, value := range map[string]any{
		"schema":           record.Schema,
		"baseConfigDigest": record.BaseConfigDigest,
		"receiptPath":      record.ReceiptPath,
		"receiptDigest":    record.ReceiptDigest,
		"revision":         record.Revision,
	} {
		encoded, marshalErr := json.Marshal(value)
		if marshalErr != nil {
			return Settings{}, marshalErr
		}
		raw[key] = encoded
	}
	raw["defaults"] = encodedDefaults
	profile, err := codexDeveloper(raw)
	if err != nil {
		return Settings{}, err
	}
	record.CodexDeveloper = profile
	if _, err := state.store.WriteJSON(settingsPath, raw, core.DefaultConfig().Storage.CanonicalBytes); err != nil {
		return Settings{}, err
	}
	return record, nil
}

type settingsState struct{ store *store.Store }

type settingsBinding struct {
	baseConfigDigest string
	receiptPath      string
	receiptDigest    string
}

func (s *settingsService) state() (settingsState, settingsBinding, error) {
	if s == nil || s.store == nil || s.store.Root == "" {
		return settingsState{}, settingsBinding{}, fmt.Errorf("%w: settings store is required", core.ErrSettings)
	}
	root, err := filepath.Abs(s.store.Root)
	if err != nil {
		return settingsState{}, settingsBinding{}, fmt.Errorf("%w: settings root", core.ErrPath)
	}
	if resolved, resolveErr := filepath.EvalSymlinks(root); resolveErr == nil {
		root = resolved
	}
	state := settingsState{store: store.New(root, s.store.Limits)}
	binding, err := state.binding(root)
	if err != nil {
		return settingsState{}, settingsBinding{}, fmt.Errorf("settings authority: %w", err)
	}
	return state, binding, nil
}

func (s settingsState) binding(root string) (settingsBinding, error) {
	var config configRecord
	err := s.store.ReadJSON(configPath, core.DefaultConfig().Storage.CanonicalBytes, &config)
	if err == nil {
		_, receiptErr := (&setupService{store: s.store}).readConfigReceipt(root, config)
		if receiptErr != nil {
			return settingsBinding{}, receiptErr
		}
		return settingsBinding{baseConfigDigest: digestRecord(config), receiptPath: config.ReceiptPath, receiptDigest: config.ReceiptDigest}, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return settingsBinding{}, err
	}
	authority, authorityErr := migrate.AuthorityStatus(root)
	if authorityErr != nil {
		return settingsBinding{}, authorityErr
	}
	if authority.Tracker.Fingerprint == "" || authority.Tracker.ParentID == "" || authority.Tracker.TaskCount == 0 || authority.Tracker.TaskCount != len(authority.Tracker.TaskIDs) {
		return settingsBinding{}, fmt.Errorf("%w: incomplete migrated tracker authority", core.ErrRevision)
	}
	raw, _, readErr := s.store.ReadFile(".agent-team/setup.json", core.DefaultConfig().Storage.CanonicalBytes)
	if readErr != nil {
		return settingsBinding{}, readErr
	}
	var projection struct {
		Schema    int    `json:"schema"`
		Authority string `json:"authority"`
		Project   string `json:"project"`
		Revision  string `json:"revision"`
		Tracker   struct {
			Kind        string `json:"kind"`
			Fingerprint string `json:"fingerprint"`
			ParentID    string `json:"parentId"`
		} `json:"tracker"`
		ReceiptPath   string `json:"receiptPath"`
		ReceiptDigest string `json:"receiptDigest"`
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&projection) != nil || decoder.Decode(&struct{}{}) != io.EOF || projection.Schema != 1 || projection.Authority != "v8" || projection.Project != root || projection.Revision != authority.TargetRevision || projection.Tracker.Kind != "beads" || projection.Tracker.Fingerprint != authority.Tracker.Fingerprint || projection.Tracker.ParentID != authority.Tracker.ParentID || projection.ReceiptPath != ".agent-team/v8/authority.json" || projection.ReceiptDigest != authority.ReceiptDigest {
		return settingsBinding{}, fmt.Errorf("%w: setup authority projection mismatch", core.ErrRevision)
	}
	return settingsBinding{baseConfigDigest: digestBytes(raw), receiptPath: projection.ReceiptPath, receiptDigest: projection.ReceiptDigest}, nil
}

func (s settingsState) inspect(binding settingsBinding) (Settings, error) {
	record, _, err := s.load(binding)
	return record, err
}

func (s settingsState) load(binding settingsBinding) (Settings, map[string]json.RawMessage, error) {
	var document map[string]json.RawMessage
	err := s.store.ReadJSON(settingsPath, core.DefaultConfig().Storage.CanonicalBytes, &document)
	if errors.Is(err, os.ErrNotExist) {
		return Settings{Schema: 1, BaseConfigDigest: binding.baseConfigDigest, ReceiptPath: binding.receiptPath, ReceiptDigest: binding.receiptDigest, Defaults: defaultRunDefaults()}, nil, nil
	}
	if err != nil {
		return Settings{}, nil, err
	}
	encoded, marshalErr := json.Marshal(document)
	var record Settings
	if marshalErr != nil || json.Unmarshal(encoded, &record) != nil || record.Schema != 1 || record.Revision == 0 || record.BaseConfigDigest != binding.baseConfigDigest || record.ReceiptPath != binding.receiptPath || record.ReceiptDigest != binding.receiptDigest || !validRunDefaults(record.Defaults) {
		return Settings{}, nil, fmt.Errorf("%w: settings overlay binding", core.ErrRevision)
	}
	profile, err := codexDeveloper(document)
	if err != nil {
		return Settings{}, nil, err
	}
	record.CodexDeveloper = profile
	return record, document, nil
}

func defaultRunDefaults() RunDefaults { return RunDefaults{ParallelTeams: 1} }

func validRunDefaults(value RunDefaults) bool {
	return value.ParallelTeams >= 1 && value.ParallelTeams <= 6 && (value.DeployBatchTasks == nil || *value.DeployBatchTasks > 0)
}

func applySettingsUpdates(defaults *RunDefaults, document map[string]json.RawMessage, updates map[string]string) error {
	for key, value := range updates {
		switch key {
		case "parallel_teams":
			parsed, err := strconv.Atoi(value)
			if err != nil || parsed < 1 || parsed > 6 {
				return fmt.Errorf("%w: parallel_teams", core.ErrSettings)
			}
			defaults.ParallelTeams = parsed
		case "continuous", "auto_deploy":
			parsed, err := strconv.ParseBool(value)
			if err != nil {
				return fmt.Errorf("%w: %s", core.ErrSettings, key)
			}
			if key == "continuous" {
				defaults.Continuous = parsed
			} else {
				defaults.AutoDeploy = parsed
			}
		case "deploy_batch_tasks":
			if value == "null" {
				defaults.DeployBatchTasks = nil
				continue
			}
			parsed, err := strconv.Atoi(value)
			if err != nil || parsed <= 0 {
				return fmt.Errorf("%w: deploy_batch_tasks", core.ErrSettings)
			}
			defaults.DeployBatchTasks = &parsed
		case "codex.developer.model", "codex.developer.effort":
			if !validProfileValue(value) {
				return fmt.Errorf("%w: %s", core.ErrSettings, key)
			}
			if err := setCodexDeveloper(document, key, value); err != nil {
				return err
			}
		default:
			return fmt.Errorf("%w: unknown setting %s", core.ErrSettings, key)
		}
	}
	return nil
}

func validProfileValue(value string) bool {
	if value == "" || len(value) > 128 {
		return false
	}
	for _, character := range value {
		if character <= ' ' || character == '"' || character == '\\' {
			return false
		}
	}
	return true
}

func setCodexDeveloper(document map[string]json.RawMessage, key, value string) error {
	hosts := map[string]any{}
	if raw := document["hosts"]; len(raw) != 0 && json.Unmarshal(raw, &hosts) != nil {
		return fmt.Errorf("%w: malformed host profiles", core.ErrRevision)
	}
	codex, ok := hosts["codex"].(map[string]any)
	if !ok && hosts["codex"] != nil {
		return fmt.Errorf("%w: malformed Codex profile", core.ErrRevision)
	}
	if codex == nil {
		codex = map[string]any{}
	}
	roles, ok := codex["roles"].(map[string]any)
	if !ok && codex["roles"] != nil {
		return fmt.Errorf("%w: malformed Codex roles", core.ErrRevision)
	}
	if roles == nil {
		roles = map[string]any{}
	}
	developer, ok := roles["developer"].(map[string]any)
	if !ok && roles["developer"] != nil {
		return fmt.Errorf("%w: malformed Codex developer profile", core.ErrRevision)
	}
	if developer == nil {
		developer = map[string]any{}
	}
	if key == "codex.developer.model" {
		developer["model"] = value
	} else {
		developer["effort"] = value
	}
	roles["developer"], codex["roles"], hosts["codex"] = developer, roles, codex
	encoded, err := json.Marshal(hosts)
	if err != nil {
		return err
	}
	document["hosts"] = encoded
	return nil
}

func codexDeveloper(document map[string]json.RawMessage) (RoleProfile, error) {
	raw := document["hosts"]
	if len(raw) == 0 {
		return RoleProfile{}, nil
	}
	var hosts map[string]json.RawMessage
	if json.Unmarshal(raw, &hosts) != nil {
		return RoleProfile{}, fmt.Errorf("%w: malformed host profiles", core.ErrRevision)
	}
	var codex struct {
		Roles map[string]struct {
			Model  string `json:"model"`
			Effort string `json:"effort"`
		} `json:"roles"`
	}
	if raw := hosts["codex"]; len(raw) == 0 {
		return RoleProfile{}, nil
	} else if json.Unmarshal(raw, &codex) != nil {
		return RoleProfile{}, fmt.Errorf("%w: malformed Codex profile", core.ErrRevision)
	}
	profile := codex.Roles["developer"]
	if (profile.Model != "" && !validProfileValue(profile.Model)) || (profile.Effort != "" && !validProfileValue(profile.Effort)) {
		return RoleProfile{}, fmt.Errorf("%w: malformed Codex developer profile", core.ErrRevision)
	}
	return RoleProfile{Model: profile.Model, Effort: profile.Effort}, nil
}
