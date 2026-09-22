package project

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/thebpandey/agent-team/vnext/internal/core"
	"github.com/thebpandey/agent-team/vnext/internal/store"
)

const kickoffBindingPath = ".agent-team/v8/kickoff.json"

type kickoffBinding struct {
	Schema        int                 `json:"schema"`
	Project       string              `json:"project"`
	ReceiptPath   string              `json:"receiptPath"`
	ReceiptDigest string              `json:"receiptDigest"`
	Handoff       core.KickoffHandoff `json:"handoff"`
	Digest        string              `json:"digest"`
}

// AttachKickoff attaches an explicitly approved handoff for future dispatches.
// The caller must obtain approval before calling. Existing setup, settings,
// tracker contents and active run packets retain their independent bindings.
func AttachKickoff(ctx context.Context, root, path string) (SetupResult, error) {
	canonical, err := CanonicalRoot(root)
	if err != nil {
		return SetupResult{}, err
	}
	// Reject unapproved or mismatched input before creating a mutation guard.
	existing, err := InspectSetup(ctx, canonical)
	if err != nil {
		return SetupResult{}, err
	}
	handoff, err := LoadKickoff(canonical, path)
	if err != nil {
		return SetupResult{}, err
	}
	if !kickoffMatchesTracker(handoff, existing.Config.Tracker) {
		return SetupResult{}, fmt.Errorf("%w: selected tracker differs from approved Kickoff handoff", core.ErrSettings)
	}
	guard, err := store.AcquireProjectMutation(ctx, canonical, "kickoff", "kickoff-binding")
	if err != nil {
		return SetupResult{}, err
	}
	defer guard.Release()
	// Re-read authority and approved facts under the project mutation lock.
	existing, err = InspectSetup(ctx, canonical)
	if err != nil {
		return SetupResult{}, err
	}
	current, err := LoadKickoff(canonical, path)
	if err != nil {
		return SetupResult{}, err
	}
	if digestNormalizedHandoff(current) != digestNormalizedHandoff(handoff) {
		return SetupResult{}, fmt.Errorf("%w: Kickoff changed during attachment", core.ErrRevision)
	}
	if !kickoffMatchesTracker(current, existing.Config.Tracker) {
		return SetupResult{}, fmt.Errorf("%w: selected tracker differs from approved Kickoff handoff", core.ErrSettings)
	}
	st := store.New(canonical, core.DefaultConfig().Storage)
	var config configRecord
	if err := st.ReadJSON(configPath, st.Limits.CanonicalBytes, &config); err != nil {
		return SetupResult{}, err
	}
	wanted := kickoffBinding{Schema: 1, Project: canonical, ReceiptPath: config.ReceiptPath, ReceiptDigest: config.ReceiptDigest, Handoff: current}
	wanted.Digest = digestKickoffBinding(wanted)
	saved, found, err := readKickoffBinding(st, config)
	if err != nil {
		return SetupResult{}, err
	}
	if !found || saved.Digest != wanted.Digest {
		if _, err := st.WriteJSON(kickoffBindingPath, wanted, kickoffMaxBytes); err != nil {
			return SetupResult{}, err
		}
	}
	existing.Handoff = current
	return existing, nil
}

func kickoffMatchesTracker(handoff core.KickoffHandoff, tracker core.TrackerConfig) bool {
	return handoff.TrackerKind == tracker.Kind && handoff.TrackerRef == tracker.Path
}

func digestNormalizedHandoff(handoff core.KickoffHandoff) string {
	encoded, _ := json.Marshal(handoff)
	return digestBytes(encoded)
}

func digestKickoffBinding(binding kickoffBinding) string {
	binding.Digest = ""
	encoded, _ := json.Marshal(binding)
	return digestBytes(encoded)
}

// Reading a persisted approval checks its immutable binding and checksum, not
// today's HEAD or source file: implementation commits must not invalidate it.
func readKickoffBinding(st *store.Store, config configRecord) (kickoffBinding, bool, error) {
	var raw json.RawMessage
	if err := st.ReadJSON(kickoffBindingPath, kickoffMaxBytes, &raw); errors.Is(err, os.ErrNotExist) {
		return kickoffBinding{}, false, nil
	} else if err != nil {
		return kickoffBinding{}, false, fmt.Errorf("%w: read Kickoff binding: %v", core.ErrRevision, err)
	}
	var binding kickoffBinding
	if err := decodeKickoff(raw, &binding); err != nil {
		return kickoffBinding{}, false, fmt.Errorf("%w: malformed Kickoff binding", core.ErrRevision)
	}
	if binding.Schema != 1 || binding.Project != st.Root || binding.ReceiptPath != config.ReceiptPath || binding.ReceiptDigest != config.ReceiptDigest || binding.Digest != digestKickoffBinding(binding) || !kickoffMatchesTracker(binding.Handoff, config.Tracker) {
		return kickoffBinding{}, false, fmt.Errorf("%w: Kickoff setup binding or checksum mismatch", core.ErrRevision)
	}
	return binding, true, nil
}
