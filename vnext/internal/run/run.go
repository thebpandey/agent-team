package run

import (
	"context"
	"errors"
	"fmt"
	"os"
	"reflect"
	"sync"

	"github.com/thebpandey/agent-team/vnext/internal/core"
	"github.com/thebpandey/agent-team/vnext/internal/store"
)

// RunRepository persists only complete validated run manifests.
type RunRepository interface {
	Initialize(context.Context, Run) (Run, error)
	Read(context.Context, core.RunID) (Run, error)
	CompareAndSwap(context.Context, core.RunID, uint64, Run) (Run, error)
}

// TeamRepository persists only complete validated team records.
type TeamRepository interface {
	Initialize(context.Context, TeamRecord) (TeamRecord, error)
	Read(context.Context, core.TeamID) (TeamRecord, error)
	CompareAndSwap(context.Context, core.TeamID, uint64, TeamRecord) (TeamRecord, error)
}

// Repositories groups the two canonical repositories over one bounded Store.
type Repositories struct {
	Runs  RunRepository
	Teams TeamRepository
}

// NewRepositories creates canonical run and team repositories. The mutex is
// shared so a process cannot observe a half-completed compare-and-swap.
func NewRepositories(st *store.Store) Repositories {
	mu := &sync.Mutex{}
	return Repositories{Runs: &runStore{store: st, mu: mu}, Teams: &teamStore{store: st, mu: mu}}
}

type runStore struct {
	store *store.Store
	mu    *sync.Mutex
}

func (r *runStore) Initialize(ctx context.Context, value Run) (Run, error) {
	if err := ctx.Err(); err != nil {
		return Run{}, err
	}
	if err := validateRepositoryRun(value); err != nil {
		return Run{}, err
	}
	if r.store == nil {
		return Run{}, fmt.Errorf("%w: nil run store", core.ErrSettings)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return Run{}, err
	}
	existing, err := r.read(ctx, value.ID)
	if err == nil {
		if !reflect.DeepEqual(existing, value) {
			return Run{}, fmt.Errorf("%w: run %q already exists with different content", core.ErrRevision, value.ID)
		}
		return existing, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return Run{}, err
	}
	if err := ctx.Err(); err != nil {
		return Run{}, err
	}
	if _, err := r.store.WriteJSON(runPath(value.ID), value, 16<<20); err != nil {
		return Run{}, err
	}
	return value, nil
}

func (r *runStore) Read(ctx context.Context, id core.RunID) (Run, error) {
	if err := ctx.Err(); err != nil {
		return Run{}, err
	}
	if err := validateID(string(id)); err != nil {
		return Run{}, err
	}
	if r.store == nil {
		return Run{}, fmt.Errorf("%w: nil run store", core.ErrSettings)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.read(ctx, id)
}

func (r *runStore) read(ctx context.Context, id core.RunID) (Run, error) {
	if err := ctx.Err(); err != nil {
		return Run{}, err
	}
	var value Run
	if err := r.store.ReadJSON(runPath(id), 16<<20, &value); err != nil {
		return Run{}, err
	}
	if err := validateRepositoryRun(value); err != nil {
		return Run{}, fmt.Errorf("%w: stored run: %v", core.ErrRevision, err)
	}
	return value, nil
}

func (r *runStore) CompareAndSwap(ctx context.Context, id core.RunID, expected uint64, value Run) (Run, error) {
	if err := ctx.Err(); err != nil {
		return Run{}, err
	}
	if err := validateID(string(id)); err != nil {
		return Run{}, err
	}
	if r.store == nil {
		return Run{}, fmt.Errorf("%w: nil run store", core.ErrSettings)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	existing, err := r.read(ctx, id)
	if err != nil {
		return Run{}, err
	}
	if expected != existing.Revision {
		return Run{}, fmt.Errorf("%w: expected run revision %d, current %d", core.ErrRevision, expected, existing.Revision)
	}
	if value.ID != id || value.RunID != id || value.Project != existing.Project || value.Root != existing.Root {
		return Run{}, fmt.Errorf("%w: run identity changed", core.ErrRevision)
	}
	if existing.Mode == "one-off" && !sameImmutableOneOff(existing, value) {
		return Run{}, fmt.Errorf("%w: one-off manifest fields are immutable", core.ErrRevision)
	}
	value.Revision = existing.Revision + 1
	if value.WrittenAt == "" {
		value.WrittenAt = existing.WrittenAt
	}
	if err := validateRepositoryRun(value); err != nil {
		return Run{}, err
	}
	if err := ctx.Err(); err != nil {
		return Run{}, err
	}
	if _, err := r.store.WriteJSON(runPath(id), value, 16<<20); err != nil {
		return Run{}, err
	}
	return value, nil
}

type teamStore struct {
	store *store.Store
	mu    *sync.Mutex
}

func (r *teamStore) Initialize(ctx context.Context, value TeamRecord) (TeamRecord, error) {
	if err := ctx.Err(); err != nil {
		return TeamRecord{}, err
	}
	if err := validateTeam(value); err != nil {
		return TeamRecord{}, err
	}
	if r.store == nil {
		return TeamRecord{}, fmt.Errorf("%w: nil team store", core.ErrSettings)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	existing, err := r.read(ctx, value.ID)
	if err == nil {
		if !reflect.DeepEqual(existing, value) {
			return TeamRecord{}, fmt.Errorf("%w: team %q already exists with different content", core.ErrRevision, value.ID)
		}
		return existing, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return TeamRecord{}, err
	}
	if err := ctx.Err(); err != nil {
		return TeamRecord{}, err
	}
	if _, err := r.store.WriteJSON(teamPath(value.ID), value, 16<<20); err != nil {
		return TeamRecord{}, err
	}
	return value, nil
}

func (r *teamStore) Read(ctx context.Context, id core.TeamID) (TeamRecord, error) {
	if err := ctx.Err(); err != nil {
		return TeamRecord{}, err
	}
	if err := validateID(string(id)); err != nil {
		return TeamRecord{}, err
	}
	if r.store == nil {
		return TeamRecord{}, fmt.Errorf("%w: nil team store", core.ErrSettings)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.read(ctx, id)
}

func (r *teamStore) read(ctx context.Context, id core.TeamID) (TeamRecord, error) {
	if err := ctx.Err(); err != nil {
		return TeamRecord{}, err
	}
	var value TeamRecord
	if err := r.store.ReadJSON(teamPath(id), 16<<20, &value); err != nil {
		return TeamRecord{}, err
	}
	if err := validateTeam(value); err != nil {
		return TeamRecord{}, fmt.Errorf("%w: stored team: %v", core.ErrRevision, err)
	}
	return value, nil
}

func (r *teamStore) CompareAndSwap(ctx context.Context, id core.TeamID, expected uint64, value TeamRecord) (TeamRecord, error) {
	if err := ctx.Err(); err != nil {
		return TeamRecord{}, err
	}
	if err := validateID(string(id)); err != nil {
		return TeamRecord{}, err
	}
	if r.store == nil {
		return TeamRecord{}, fmt.Errorf("%w: nil team store", core.ErrSettings)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	existing, err := r.read(ctx, id)
	if err != nil {
		return TeamRecord{}, err
	}
	if expected != existing.Revision {
		return TeamRecord{}, fmt.Errorf("%w: expected team revision %d, current %d", core.ErrRevision, expected, existing.Revision)
	}
	if value.ID != id || value.RunID != existing.RunID || value.Project != existing.Project {
		return TeamRecord{}, fmt.Errorf("%w: team identity changed", core.ErrRevision)
	}
	value.Revision = existing.Revision + 1
	if value.WrittenAt == "" {
		value.WrittenAt = existing.WrittenAt
	}
	if err := validateTeam(value); err != nil {
		return TeamRecord{}, err
	}
	if err := ctx.Err(); err != nil {
		return TeamRecord{}, err
	}
	if _, err := r.store.WriteJSON(teamPath(id), value, 16<<20); err != nil {
		return TeamRecord{}, err
	}
	return value, nil
}

func sameImmutableOneOff(a, b Run) bool {
	return a.Root == b.Root && a.Mode == b.Mode && a.OneOffKind == b.OneOffKind && a.Objective == b.Objective && a.TrackerKind == b.TrackerKind && a.TrackerRevision == b.TrackerRevision && a.SpecRevision == b.SpecRevision && a.ManifestDigest == b.ManifestDigest && reflect.DeepEqual(a.Tasks, b.Tasks) && sameTeamManifest(a.Teams, b.Teams)
}

func sameTeamManifest(a, b []TeamRecord) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].ID != b[i].ID || !reflect.DeepEqual(a[i].Queue, b[i].Queue) || a[i].QueueFingerprint != b[i].QueueFingerprint || !reflect.DeepEqual(a[i].Paths, b[i].Paths) || !reflect.DeepEqual(a[i].Resources, b[i].Resources) {
			return false
		}
	}
	return true
}

func runPath(id core.RunID) string   { return ".agent-team/runs/" + string(id) + ".json" }
func teamPath(id core.TeamID) string { return ".agent-team/teams/" + string(id) + ".json" }
