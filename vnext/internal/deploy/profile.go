package deploy

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/fs"
	"path/filepath"
	"strings"
	"sync"
	"unicode"

	"github.com/thebpandey/agent-team/vnext/internal/core"
	"github.com/thebpandey/agent-team/vnext/internal/store"
)

const profileLimit = 64 << 10

type fileProfileRepository struct {
	mu    sync.Mutex
	store *store.Store
}

func NewProfileRepository(state *store.Store) ProfileRepository {
	return &fileProfileRepository{store: state}
}

func (r *fileProfileRepository) Get(ctx context.Context, id ProfileID) (TargetProfile, error) {
	if ctx == nil || ctx.Err() != nil || r == nil || r.store == nil || !validProfileID(id) {
		return TargetProfile{}, core.ErrSettings
	}
	var profile TargetProfile
	if err := r.store.ReadJSON(profilePath(id), profileLimit, &profile); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return TargetProfile{}, ErrNotFound
		}
		return TargetProfile{}, err
	}
	if profile.ID != id || ValidateProfile(profile) != nil {
		return TargetProfile{}, core.ErrSettings
	}
	return cloneProfile(profile), nil
}

func (r *fileProfileRepository) ApplyDecision(ctx context.Context, id ProfileID, decision ProfileDecision, expected uint64) (ProfileWriteOutcome, error) {
	if ctx == nil || ctx.Err() != nil || r == nil || r.store == nil || !validProfileID(id) {
		return ProfileWriteOutcome{}, core.ErrSettings
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	old, err := r.Get(ctx, id)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return ProfileWriteOutcome{}, err
	}
	actual := uint64(0)
	if err == nil {
		actual = old.Revision
	}
	if expected != actual {
		return ProfileWriteOutcome{}, core.ErrRevision
	}
	profile, err := profileFromDecision(id, decision)
	if err != nil {
		return ProfileWriteOutcome{}, err
	}
	if actual > 0 && old.ProfileDigest == profile.ProfileDigest {
		return ProfileWriteOutcome{Kind: ProfileDuplicate, Profile: old, ObservedRevision: actual, Idempotent: true}, nil
	}
	profile.Schema = 1
	profile.Revision = actual + 1
	if _, err := r.store.WriteJSON(profilePath(id), profile, profileLimit); err != nil {
		return ProfileWriteOutcome{}, err
	}
	kind := ProfileCreated
	if actual > 0 {
		kind = ProfileUpdated
	}
	return ProfileWriteOutcome{Kind: kind, Profile: cloneProfile(profile), ExpectedRevision: expected, ObservedRevision: profile.Revision}, nil
}

type MemoryProfileRepository struct {
	mu       sync.Mutex
	revision uint64
	profiles map[ProfileID]TargetProfile
}

func NewMemoryProfileRepository() *MemoryProfileRepository {
	return &MemoryProfileRepository{profiles: map[ProfileID]TargetProfile{}}
}

func (r *MemoryProfileRepository) Get(_ context.Context, id ProfileID) (TargetProfile, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	profile, ok := r.profiles[id]
	if !ok {
		return TargetProfile{}, ErrNotFound
	}
	return cloneProfile(profile), nil
}

func (r *MemoryProfileRepository) ApplyDecision(_ context.Context, id ProfileID, decision ProfileDecision, expected uint64) (ProfileWriteOutcome, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if expected != r.revision {
		return ProfileWriteOutcome{}, core.ErrRevision
	}
	profile, err := profileFromDecision(id, decision)
	if err != nil {
		return ProfileWriteOutcome{}, err
	}
	if old, ok := r.profiles[id]; ok && old.ProfileDigest == profile.ProfileDigest {
		return ProfileWriteOutcome{Kind: ProfileDuplicate, Profile: cloneProfile(old), ObservedRevision: r.revision, Idempotent: true}, nil
	}
	r.revision++
	profile.Schema, profile.Revision = 1, r.revision
	r.profiles[id] = cloneProfile(profile)
	kind := ProfileCreated
	if expected > 0 {
		kind = ProfileUpdated
	}
	return ProfileWriteOutcome{Kind: kind, Profile: cloneProfile(profile), ExpectedRevision: expected, ObservedRevision: r.revision}, nil
}

func profileFromDecision(id ProfileID, decision ProfileDecision) (TargetProfile, error) {
	profile := TargetProfile{ID: id, Target: decision.Target, AuthorizationRef: decision.AuthorizationRef, ApprovalScope: decision.ApprovalScope, ExecutorCommand: append([]string(nil), decision.ExecutorCommand...), QueryCommand: append([]string(nil), decision.QueryCommand...), VerificationCommand: append([]string(nil), decision.VerificationCommand...), DefaultBatchSize: decision.DefaultBatchSize, Enabled: decision.Enable, Confirmed: decision.Confirmed}
	var err error
	profile.ProfileDigest, err = DeriveProfileDigest(profile)
	if err != nil {
		return TargetProfile{}, err
	}
	if err := ValidateProfile(profile); err != nil {
		return TargetProfile{}, err
	}
	return profile, nil
}

func DeriveProfileDigest(profile TargetProfile) (string, error) {
	wire := struct {
		ID                                      ProfileID
		Target, AuthorizationRef, ApprovalScope string
		Exec, Query, Verify                     []string
		Size                                    int
		Enabled, Confirmed                      bool
	}{profile.ID, profile.Target, profile.AuthorizationRef, profile.ApprovalScope, profile.ExecutorCommand, profile.QueryCommand, profile.VerificationCommand, profile.DefaultBatchSize, profile.Enabled, profile.Confirmed}
	raw, err := json.Marshal(wire)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

func ValidateProfile(profile TargetProfile) error {
	if !validProfileID(profile.ID) {
		return core.ErrSettings
	}
	if profile.DefaultBatchSize < 1 || profile.DefaultBatchSize > 100 {
		return core.ErrBatch
	}
	if profile.Enabled && (profile.Target == "" || strings.EqualFold(profile.Target, "production") || profile.AuthorizationRef == "" || profile.ApprovalScope == "") {
		return core.ErrSettings
	}
	for _, command := range [][]string{profile.ExecutorCommand, profile.QueryCommand, profile.VerificationCommand} {
		if err := ValidateCommandTemplate(command); err != nil {
			return err
		}
	}
	if profile.Enabled {
		digest, err := DeriveProfileDigest(profile)
		if err != nil || digest != profile.ProfileDigest {
			return core.ErrSettings
		}
	}
	return nil
}

func ValidateCommandTemplate(command []string) error {
	if len(command) == 0 || len(command) > 64 || command[0] == "" {
		return core.ErrSettings
	}
	base := strings.ToLower(filepath.Base(strings.ReplaceAll(command[0], "\\", "/")))
	switch strings.TrimSuffix(base, filepath.Ext(base)) {
	case "sh", "bash", "dash", "zsh", "fish", "cmd", "powershell", "pwsh", "busybox":
		return core.ErrSettings
	}
	for _, arg := range command[1:] {
		lower := strings.ToLower(arg)
		if len(arg) > 4096 || strings.ContainsAny(arg, "\x00\r\n") || strings.Contains(arg, "$(") || strings.Contains(arg, "${") || strings.Contains(arg, "`") || strings.Contains(arg, "%") || strings.Contains(arg, "..") || lower == "-c" || lower == "/c" || lower == "-command" || lower == "--command" {
			return core.ErrSettings
		}
	}
	return nil
}

func validProfileID(id ProfileID) bool {
	if id == "" || len(id) > 128 {
		return false
	}
	for _, r := range id {
		if !(unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_') {
			return false
		}
	}
	return true
}

func profilePath(id ProfileID) string { return "deploy/profiles/" + string(id) + ".json" }

func cloneProfile(profile TargetProfile) TargetProfile {
	profile.ExecutorCommand = append([]string(nil), profile.ExecutorCommand...)
	profile.QueryCommand = append([]string(nil), profile.QueryCommand...)
	profile.VerificationCommand = append([]string(nil), profile.VerificationCommand...)
	return profile
}
