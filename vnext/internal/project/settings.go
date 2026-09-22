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
	if err := applyRunDefaultUpdates(&record.Defaults, updates); err != nil {
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
	if raw == nil {
		raw = make(map[string]json.RawMessage)
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
	return record, document, nil
}

func defaultRunDefaults() RunDefaults { return RunDefaults{ParallelTeams: 1} }

func validRunDefaults(value RunDefaults) bool {
	return value.ParallelTeams >= 1 && value.ParallelTeams <= 6 && (value.DeployBatchTasks == nil || *value.DeployBatchTasks > 0)
}

func applyRunDefaultUpdates(defaults *RunDefaults, updates map[string]string) error {
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
		default:
			return fmt.Errorf("%w: unknown setting %s", core.ErrSettings, key)
		}
	}
	return nil
}
