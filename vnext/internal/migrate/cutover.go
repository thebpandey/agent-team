package migrate

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/thebpandey/agent-team/vnext/internal/core"
	"github.com/thebpandey/agent-team/vnext/internal/install"
	state "github.com/thebpandey/agent-team/vnext/internal/store"
)

func NewStore(root string) *Store {
	key, _ := filepath.Abs(root)
	lock, _ := migrationLocks.LoadOrStore(key, &sync.Mutex{})
	return &Store{Root: root, mu: lock.(*sync.Mutex)}
}

func (s *Store) CompareAndSwap(ctx context.Context, expected uint64, next CanaryRecord) (CanaryRecord, error) {
	if s == nil {
		return CanaryRecord{}, core.ErrPath
	}
	release, err := state.AcquireProjectMutation(ctx, s.Root)
	if err != nil {
		return CanaryRecord{}, err
	}
	defer func() { _ = release() }()
	return s.compareAndSwapLocked(ctx, expected, next)
}

func (s *Store) compareAndSwapLocked(ctx context.Context, expected uint64, next CanaryRecord) (CanaryRecord, error) {
	if ctx == nil || ctx.Err() != nil || s == nil || s.mu == nil {
		return CanaryRecord{}, core.ErrPath
	}
	project, err := cleanProject(s.Root)
	if err != nil || next.Project != project || !validID(next.ID) {
		return CanaryRecord{}, core.ErrPath
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	current, err := s.read(next.ID)
	absent := errors.Is(err, os.ErrNotExist)
	if err != nil && !absent {
		return CanaryRecord{}, err
	}
	if current.Revision != expected || absent != (expected == 0) {
		return CanaryRecord{}, fmt.Errorf("%w: expected canary revision %d, current %d", core.ErrRevision, expected, current.Revision)
	}
	if !absent && (current.ID != next.ID || current.Project != next.Project || current.V7InventoryDigest != next.V7InventoryDigest) {
		return CanaryRecord{}, fmt.Errorf("%w: immutable canary identity changed", core.ErrRevision)
	}
	next.Schema = 1
	next.Revision = expected + 1
	next.WrittenAt = time.Now().UTC().Format(time.RFC3339Nano)
	next.Idempotent = false
	if err := validateRecord(next); err != nil {
		return CanaryRecord{}, err
	}
	if _, err := state.New(project, core.StorageLimits{CanonicalBytes: migrationLimit}).WriteJSON(canaryPath(next.ID), next, migrationLimit); err != nil {
		return CanaryRecord{}, err
	}
	return next, nil
}

func BeginCanary(ctx context.Context, store *Store, project string, host install.Host, approved bool, runner CanaryRunner) (CanaryRecord, error) {
	if !approved || runner == nil || store == nil || store.Root != project || !validHost(host) {
		return CanaryRecord{}, core.ErrSettings
	}
	project, err := cleanProject(project)
	if err != nil {
		return CanaryRecord{}, err
	}
	release, err := state.AcquireProjectMutation(ctx, project)
	if err != nil {
		return CanaryRecord{}, err
	}
	defer func() { _ = release() }()
	inv, err := readInventory(project)
	if err != nil {
		return CanaryRecord{}, err
	}
	observation, err := runner.Observe(ctx, project, host)
	if err != nil {
		return CanaryRecord{}, err
	}
	if err := validateObservation(observation, host); err != nil {
		return CanaryRecord{}, err
	}
	id, err := newID()
	if err != nil {
		return CanaryRecord{}, err
	}
	record := CanaryRecord{ID: id, Project: project, V7InventoryDigest: inv.Digest, State: "observing"}
	setObservation(&record, host, observation)
	return store.compareAndSwapLocked(ctx, 0, record)
}

func ResumeCanary(ctx context.Context, store *Store, id string, host install.Host, expected uint64, inventoryDigest string, runner CanaryRunner) (CanaryRecord, error) {
	if store == nil || runner == nil || !validHost(host) || !validID(id) {
		return CanaryRecord{}, core.ErrSettings
	}
	release, err := state.AcquireProjectMutation(ctx, store.Root)
	if err != nil {
		return CanaryRecord{}, err
	}
	defer func() { _ = release() }()
	record, err := store.read(id)
	if err != nil {
		return CanaryRecord{}, err
	}
	if record.Revision != expected || record.V7InventoryDigest != inventoryDigest {
		return CanaryRecord{}, fmt.Errorf("%w: stale canary or inventory", core.ErrRevision)
	}
	inv, err := readInventory(record.Project)
	if err != nil || inv.Digest != inventoryDigest {
		return CanaryRecord{}, fmt.Errorf("%w: current v7 inventory changed", core.ErrRevision)
	}
	if record.State == "rolled_back" {
		return CanaryRecord{}, fmt.Errorf("%w: canary is terminal", core.ErrRevision)
	}
	observation, err := runner.Observe(ctx, record.Project, host)
	if err != nil {
		return CanaryRecord{}, err
	}
	if err := validateObservation(observation, host); err != nil {
		return CanaryRecord{}, err
	}
	if reflect.DeepEqual(selectedObservation(record, host), observation) && record.State != "observing" {
		record.Idempotent = true
		return record, nil
	}
	setObservation(&record, host, observation)
	record.State = observationState(record)
	return store.compareAndSwapLocked(ctx, expected, record)
}

func RollbackCanary(ctx context.Context, store *Store, id string) (CanaryRecord, error) {
	if store == nil || !validID(id) {
		return CanaryRecord{}, core.ErrPath
	}
	release, err := state.AcquireProjectMutation(ctx, store.Root)
	if err != nil {
		return CanaryRecord{}, err
	}
	defer func() { _ = release() }()
	record, err := store.read(id)
	if err != nil {
		return CanaryRecord{}, err
	}
	if record.State == "rolled_back" {
		record.Idempotent = true
		return record, nil
	}
	retained := make([]string, 0)
	removed := make([]string, 0)
	for _, artifact := range record.Owned {
		relative, pathErr := projectRelative(record.Project, artifact.Path)
		if pathErr != nil || !strings.HasPrefix(filepath.ToSlash(relative), ".agent-team/migration/") {
			retained = append(retained, artifact.Path)
			continue
		}
		digest, _, hashErr := hashRegular(artifact.Path, migrationLimit)
		if hashErr != nil || digest != artifact.SHA256 {
			retained = append(retained, artifact.Path)
			continue
		}
		root, openErr := os.OpenRoot(record.Project)
		if openErr != nil {
			return CanaryRecord{}, openErr
		}
		removeErr := root.Remove(relative)
		closeErr := root.Close()
		if removeErr != nil || closeErr != nil {
			retained = append(retained, artifact.Path)
			continue
		}
		removed = append(removed, artifact.Path)
	}
	sort.Strings(retained)
	sort.Strings(removed)
	evidence := sha256.Sum256([]byte(strings.Join(append(append([]string{}, removed...), retained...), "\n")))
	record.Retained = retained
	record.RollbackEvidence = hex.EncodeToString(evidence[:])
	record.State = "rolled_back"
	return store.compareAndSwapLocked(ctx, record.Revision, record)
}

func (s *Store) read(id string) (CanaryRecord, error) {
	if s == nil || !validID(id) {
		return CanaryRecord{}, core.ErrPath
	}
	project, err := cleanProject(s.Root)
	if err != nil {
		return CanaryRecord{}, err
	}
	var record CanaryRecord
	if err := state.New(project, core.StorageLimits{CanonicalBytes: migrationLimit}).ReadJSON(canaryPath(id), migrationLimit, &record); err != nil {
		return CanaryRecord{}, err
	}
	if record.Project != project {
		return CanaryRecord{}, fmt.Errorf("%w: canary project changed", core.ErrRevision)
	}
	if err := validateRecord(record); err != nil {
		return CanaryRecord{}, err
	}
	return record, nil
}

func validateRecord(record CanaryRecord) error {
	if record.Schema != 1 || record.Revision == 0 || !validID(record.ID) || record.Project == "" || !validDigest(record.V7InventoryDigest) {
		return fmt.Errorf("%w: invalid canary record", core.ErrRevision)
	}
	if record.State != "observing" && record.State != "codex_ready" && record.State != "claude_ready" && record.State != "ready" && record.State != "rolled_back" {
		return fmt.Errorf("%w: invalid canary state", core.ErrRevision)
	}
	return nil
}

func validateObservation(observation Observation, host install.Host) error {
	if observation.Host != host || observation.Identity == "" || observation.Revision == "" || observation.TrackerDigest == "" || observation.ReceiptDigest == "" || observation.State != "ready" {
		return fmt.Errorf("%w: invalid host observation", core.ErrRevision)
	}
	return nil
}

func validHost(host install.Host) bool { return host == install.Codex || host == install.Claude }

func setObservation(record *CanaryRecord, host install.Host, observation Observation) {
	if host == install.Codex {
		record.Codex = observation
	} else {
		record.Claude = observation
	}
}

func selectedObservation(record CanaryRecord, host install.Host) Observation {
	if host == install.Codex {
		return record.Codex
	}
	return record.Claude
}

func observationState(record CanaryRecord) string {
	if record.Codex.State == "ready" && record.Claude.State == "ready" {
		return "ready"
	}
	if record.Codex.State == "ready" {
		return "codex_ready"
	}
	return "claude_ready"
}

func newID() (string, error) {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw), nil
}

func validID(id string) bool {
	if len(id) != 32 {
		return false
	}
	_, err := hex.DecodeString(id)
	return err == nil
}

func validDigest(digest string) bool {
	if len(digest) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(digest)
	return err == nil
}

func canaryPath(id string) string { return ".agent-team/migration/canary-" + id + ".json" }
