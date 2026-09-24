package project

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/thebpandey/agent-team/vnext/internal/core"
	"github.com/thebpandey/agent-team/vnext/internal/store"
)

// SetupOptions contains decisions made by the foreground host's setup dialogue.
// Inspection never assumes consent to create artifacts or install dependencies.
type SetupOptions struct {
	Root, Tracker, Kickoff                  string
	Approved, ApproveKickoff, IgnoreKickoff bool
}

type Onboarding struct {
	Status           string               `json:"status"`
	Project          string               `json:"project"`
	Tracker          core.TrackerConfig   `json:"tracker"`
	TrackerOptions   []string             `json:"tracker_options,omitempty"`
	Missing          []string             `json:"missing,omitempty"`
	Generated        []string             `json:"generated,omitempty"`
	NextAction       string               `json:"next_action"`
	Message          string               `json:"message"`
	ReceiptPath      string               `json:"receipt_path,omitempty"`
	ConfigRevision   uint64               `json:"config_revision,omitempty"`
	Handoff          *core.KickoffHandoff `json:"handoff,omitempty"`
	SettingsRequired bool                 `json:"settings_required"`
}

// InspectSetup reads the committed setup binding, not a new tracker snapshot.
// Updating tasks after setup must not make a subsequent setup invocation fail.
func InspectSetup(ctx context.Context, root string) (SetupResult, error) {
	p, err := Discover(ctx, root)
	if err != nil {
		return SetupResult{}, err
	}
	st := store.New(p.TopLevel, core.DefaultConfig().Storage)
	var config configRecord
	if err := st.ReadJSON(configPath, st.Limits.CanonicalBytes, &config); err != nil {
		return SetupResult{}, err
	}
	receipt, err := (&setupService{store: st}).readConfigReceipt(p.TopLevel, config)
	if err != nil {
		return SetupResult{}, err
	}
	handoff := receipt.Handoff
	if binding, found, err := readKickoffBinding(st, config); err != nil {
		return SetupResult{}, err
	} else if found {
		handoff = binding.Handoff
	}
	return SetupResult{Project: p, Config: config.toConfig(), ConfigRevision: config.Revision, ReceiptPath: config.ReceiptPath, Handoff: handoff, ArtifactDigests: receipt.ArtifactDigests}, nil
}

func Onboard(ctx context.Context, options SetupOptions) (Onboarding, error) {
	p, err := Discover(ctx, options.Root)
	if err != nil {
		return Onboarding{}, fmt.Errorf("open the project Git repository before setup: %w", err)
	}
	root := p.TopLevel
	result := Onboarding{Status: "needs_input", Project: root, TrackerOptions: []string{"beads", "tasks-md"}}
	for _, path := range []string{".agent-team", "DECISIONS.md", "AGENT_TEAM_RULES.md", "TASKS.md", ".beads"} {
		if _, err := Contain(root, filepath.Join(root, path)); err != nil {
			return result, err
		}
	}
	if existing, readErr := InspectSetup(ctx, root); readErr == nil {
		if options.Tracker != "" && options.Tracker != existing.Config.Tracker.Kind {
			return result, fmt.Errorf("%w: this project already uses %s; changing trackers requires an explicit data migration", core.ErrSettings, existing.Config.Tracker.Kind)
		}
		path := options.Kickoff
		if path == "" && existing.Handoff.TrackerKind == "" {
			candidate := ".project-kickoff/AGENT_TEAM_HANDOFF.json"
			if _, err := os.Lstat(filepath.Join(root, candidate)); err == nil {
				path = candidate
			} else if !errors.Is(err, os.ErrNotExist) {
				return result, err
			}
		}
		if path != "" {
			if !options.ApproveKickoff {
				result.Tracker = existing.Config.Tracker
				result.NextAction = "approve_kickoff"
				result.Message = "Confirm attaching the approved Project Kickoff handoff to this project's existing setup."
				return result, nil
			}
			var err error
			existing, err = AttachKickoff(ctx, root, path)
			if err != nil {
				return result, err
			}
		}
		return initializedOnboarding(ctx, root, existing)
	} else if !errors.Is(readErr, os.ErrNotExist) {
		return result, readErr
	}
	selected := core.TrackerConfig{Kind: options.Tracker}
	kickoffPath := options.Kickoff
	var ignoredKickoff *IgnoredKickoff
	if kickoffPath == "" {
		candidate := ".project-kickoff/AGENT_TEAM_HANDOFF.json"
		if _, err := os.Lstat(filepath.Join(root, candidate)); err == nil {
			if options.IgnoreKickoff {
				inspection, raw, readErr := inspectKickoff(root, candidate)
				if readErr != nil {
					return result, readErr
				}
				reason := inspection.Reason
				if reason == "" {
					reason = "explicitly ignored by setup"
				}
				ignoredKickoff = &IgnoredKickoff{Path: candidate, Digest: digestBytes(raw), DetectedVersion: inspection.DetectedVersion, Decision: "ignored", Reason: reason}
			} else {
				kickoffPath = candidate
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return result, err
		}
	}
	var kickoff *KickoffDecision
	if kickoffPath != "" {
		handoff, err := LoadKickoff(root, kickoffPath)
		if err != nil {
			return result, err
		}
		result.Handoff = &handoff
		if selected.Kind != "" && selected.Kind != handoff.TrackerKind {
			return result, fmt.Errorf("%w: selected tracker differs from approved Kickoff handoff", core.ErrSettings)
		}
		selected = core.TrackerConfig{Kind: handoff.TrackerKind, Path: handoff.TrackerRef}
		result.Tracker = selected
		if !options.ApproveKickoff {
			result.NextAction = "approve_kickoff"
			result.Message = "Project Kickoff handoff found. Confirm using its approved plan and tracker."
			return result, nil
		}
		relative, _, err := inputPath(root, kickoffPath)
		if err != nil {
			return result, err
		}
		raw, err := readBoundedContained(root, relative, core.DefaultConfig().Storage.CanonicalBytes)
		if err != nil {
			return result, err
		}
		kickoff = &KickoffDecision{Path: relative, Digest: digestBytes(raw), Confirmation: Approved}
	}
	if selected.Kind == "" {
		var candidates []string
		for _, kind := range []string{"beads", "tasks-md"} {
			path := map[string]string{"beads": ".beads", "tasks-md": "TASKS.md"}[kind]
			if _, err := os.Lstat(filepath.Join(root, path)); err == nil {
				candidates = append(candidates, kind)
			} else if !errors.Is(err, os.ErrNotExist) {
				return result, err
			}
		}
		if len(candidates) > 0 {
			selected.Kind = candidates[0]
		} else if options.ApproveKickoff {
			selected.Kind = "tasks-md"
		} else {
			result.NextAction = "choose_tracker"
			result.Missing = []string{"TASKS.md or .beads", "DECISIONS.md", "AGENT_TEAM_RULES.md"}
			result.Message = "Choose Beads or TASKS.md, then approve setup. --approve-kickoff also creates minimal TASKS.md, DECISIONS.md, and AGENT_TEAM_RULES.md when no Kickoff handoff exists."
			return result, nil
		}
	}
	if selected.Path == "" {
		switch selected.Kind {
		case "beads":
			selected.Path = ".beads"
		case "tasks-md":
			selected.Path = "TASKS.md"
		default:
			return result, fmt.Errorf("%w: choose beads or tasks-md", core.ErrSettings)
		}
	}
	result.Tracker = selected
	paths := []string{selected.Path, "DECISIONS.md", "AGENT_TEAM_RULES.md"}
	for _, path := range paths {
		if _, _, err := inputPath(root, path); err != nil {
			return result, err
		}
		if _, err := os.Lstat(filepath.Join(root, path)); errors.Is(err, os.ErrNotExist) {
			result.Missing = append(result.Missing, path)
		} else if err != nil {
			return result, err
		}
	}
	if selected.Kind == "beads" {
		for _, path := range result.Missing {
			if path == selected.Path {
				result.NextAction = "initialize_beads"
				result.Message = "Initialize the selected Beads tracker after dependency approval, then resume setup. You can choose TASKS.md instead."
				return result, nil
			}
		}
	}
	if len(result.Missing) > 0 && !options.Approved && !options.ApproveKickoff {
		result.NextAction = "approve_artifacts"
		result.Message = "Approve creation of the missing setup files. Existing files will be reused."
		return result, nil
	}
	// Validate all destinations before the first mutation; exclusive creation
	// makes retries preserve user edits and concurrent creations.
	input := SetupInput{Root: root, Mode: PlanMode, Tracker: selected, Kickoff: kickoff, IgnoredKickoff: ignoredKickoff}
	for _, path := range paths {
		mode := ExistingArtifact
		for _, missing := range result.Missing {
			if path == missing {
				mode = GeneratedArtifact
			}
		}
		input.Artifacts = append(input.Artifacts, ArtifactDecision{Path: path, Mode: mode, Confirmation: Approved})
	}
	if _, err := ValidateSetup(ctx, input); err != nil {
		return result, err
	}
	for _, path := range result.Missing {
		body := setupTemplate(path, selected)
		if body == "" {
			return result, fmt.Errorf("%w: missing approved artifact %s", core.ErrSettings, path)
		}
	}
	for _, path := range result.Missing {
		if err := createSetupFile(root, path, setupTemplate(path, selected)); err != nil {
			return result, err
		}
	}
	for index := range input.Artifacts {
		input.Artifacts[index].Mode = ExistingArtifact
	}
	setup, err := NewSetupService(store.New(root, core.DefaultConfig().Storage)).Initialize(ctx, input)
	if err != nil {
		return result, err
	}
	ready, err := initializedOnboarding(ctx, root, setup)
	ready.Generated = append([]string(nil), result.Missing...)
	return ready, err
}

func initializedOnboarding(ctx context.Context, root string, setup SetupResult) (Onboarding, error) {
	settings, err := NewSettingsService(store.New(root, core.DefaultConfig().Storage)).Inspect(ctx)
	if err != nil {
		return Onboarding{}, err
	}
	result := Onboarding{Status: "initialized", Project: root, Tracker: setup.Config.Tracker, ReceiptPath: setup.ReceiptPath, ConfigRevision: setup.ConfigRevision, SettingsRequired: settings.Revision == 0, NextAction: "start", Message: "Project setup is ready."}
	if setup.Handoff.TrackerKind != "" {
		result.Handoff = &setup.Handoff
	}
	if result.SettingsRequired {
		result.NextAction = "settings"
		result.Message = "Choose role models and effort for Codex and Claude, or accept inherited host defaults, before starting."
	}
	return result, nil
}

func setupTemplate(path string, selected core.TrackerConfig) string {
	switch path {
	case "DECISIONS.md":
		return "# Decisions\n\nRecord approved project decisions here.\n"
	case "AGENT_TEAM_RULES.md":
		return "# Agent-Team rules\n\nUse " + selected.Path + " as the single execution tracker. Follow the approved plan and task scope.\n\nCodex and Claude share project state. Switching foreground hosts requires no ownership transfer. Never duplicate an active task or reuse another host's worker handle.\n\nUse bounded task briefs and independent review. Load only the instructions needed for the current task. No external hooks are required.\n"
	default:
		if selected.Kind == "tasks-md" && path == selected.Path {
			return "# Tasks\n\n<!-- Add approved tasks here, or run Project Kickoff to prepare the plan and tracker. -->\n"
		}
	}
	return ""
}

func createSetupFile(root, path, body string) error {
	if _, _, err := inputPath(root, path); err != nil {
		return err
	}
	dir, err := os.OpenRoot(root)
	if err != nil {
		return err
	}
	defer dir.Close()
	if parent := filepath.Dir(path); parent != "." {
		if err := dir.MkdirAll(parent, 0700); err != nil {
			return err
		}
	}
	f, err := dir.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if errors.Is(err, os.ErrExist) {
		return nil
	}
	if err != nil {
		return err
	}
	_, writeErr := f.WriteString(body)
	syncErr := f.Sync()
	closeErr := f.Close()
	return errors.Join(writeErr, syncErr, closeErr)
}
