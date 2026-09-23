package start

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/thebpandey/agent-team/vnext/internal/contracts"
	"github.com/thebpandey/agent-team/vnext/internal/core"
	"github.com/thebpandey/agent-team/vnext/internal/run"
	"github.com/thebpandey/agent-team/vnext/internal/store"
	"github.com/thebpandey/agent-team/vnext/internal/tracker"
)

// AdmitOneOffRegistered reserves a single explicitly supplied immutable task.
// It shares ordinary packet persistence, replay and acknowledgment with plan
// admission. Audit/review packets grant no writable scope or worker launch.
func AdmitOneOffRegistered(ctx context.Context, st *store.Store, project string, kind run.OneOffKind, task core.Task, manager contracts.WorktreeManager, owner, base string) (Result, error) {
	if !nativeHost(owner) || base == "" {
		return Result{}, core.ErrSettings
	}
	if len(task.Dependencies) != 0 || task.Archived || (task.State != "" && task.State != core.Ready) {
		return Result{}, fmt.Errorf("%w: one-off task must be ready and self-contained", core.ErrSettings)
	}
	task.State = core.Ready
	manifest, err := run.CreateOneOff(ctx, project, kind, task.Objective, []core.Task{task})
	if err != nil {
		return Result{}, err
	}
	team := manifest.Teams[0]
	prepared := run.PreparedPlanAdmission{Run: manifest, Team: team, Batch: run.AdmissionBatch{Tasks: append([]core.TaskID(nil), team.Queue...), Paths: append([]string(nil), team.Paths...)}}
	if err := validateStoreProject(st, manifest.Root); err != nil {
		return Result{}, err
	}
	var result Result
	err = withProjectLock(ctx, st, func() error {
		if existing, err := run.NewRepositories(st).Runs.Read(ctx, manifest.ID); err == nil {
			current, err := reconcileTeamLocked(ctx, st, manifest.ID, team.ID)
			if err != nil {
				return err
			}
			if len(current.Queue) == 0 && current.RetainedHandle.Identity != "" {
				stored, path, err := readPacket(st, team.ID, current.RetainedHandle.PacketDigest)
				if err != nil {
					return err
				}
				result = Result{Run: existing, Team: current, Packet: stored.Packet, PacketDigest: stored.Digest, PacketPath: path, AlreadyAdmitted: true}
				return nil
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if err := AdmissionAllowed(ctx, st, core.AssignmentPacket{RecordEnvelope: manifest.RecordEnvelope, Team: team.ID, Task: task.ID}); err != nil {
			return err
		}
		worktree := manifest.Root
		if kind == run.Feature {
			if manager == nil {
				return core.ErrSettings
			}
			if ownerRun, found, err := existingTaskOwner(st, task.ID); err != nil {
				return err
			} else if found && ownerRun != manifest.ID {
				return fmt.Errorf("%w: task already admitted by %s", core.ErrRevision, ownerRun)
			}
			worktree = filepath.Join(manifest.Root, ".agent-team", "worktrees", string(team.ID))
			registered, err := manager.Create(ctx, contracts.WorktreeSpec{Run: manifest.ID, Team: team.ID, Root: worktree, Base: base, WritablePaths: append([]string(nil), team.Paths...)})
			if err != nil {
				return err
			}
			if registered.Run != manifest.ID || registered.Team != team.ID || registered.Path == "" {
				return core.ErrRevision
			}
			worktree = registered.Path
		}
		var err error
		result, err = admitPreparedLocked(ctx, st, oneOffTracker{manifest}, prepared, owner, worktree, base)
		if result.HostDispatchRequired {
			result.AlreadyAdmitted = false
		}
		return err
	})
	return result, err
}

func readOnlyOneOff(manifest run.Run) bool {
	return manifest.Mode == "one-off" && (manifest.OneOffKind == run.Audit || manifest.OneOffKind == run.Review)
}

func validateReadOnlyOneOffBoundary(prepared run.PreparedPlanAdmission, worktree, base string) error {
	if !readOnlyOneOff(prepared.Run) || len(prepared.Run.Tasks) != 1 || len(prepared.Batch.Paths) != 0 || len(prepared.Run.Tasks[0].WritablePaths) != 0 || filepath.Clean(worktree) != prepared.Run.Root || base != "HEAD" {
		return core.ErrPath
	}
	return nil
}

func assignmentSpecRevision(manifest run.Run) string {
	if manifest.Mode == "one-off" {
		return manifest.ManifestDigest
	}
	return manifest.SpecRevision
}

// oneOffTracker exposes only the already approved immutable manifest to the
// shared reservation path. It cannot create or edit a project tracker.
type oneOffTracker struct{ manifest run.Run }

// TrackerForOneOff lets native follow-up controls read an immutable one-off
// run without looking up or changing the selected project tracker.
func TrackerForOneOff(manifest run.Run) (tracker.Tracker, error) {
	if manifest.Mode != "one-off" {
		return nil, core.ErrSettings
	}
	return oneOffTracker{manifest}, nil
}

func (t oneOffTracker) Page(context.Context, string, int) (core.TrackerPage, error) {
	return core.TrackerPage{Tasks: t.manifest.Tasks, TotalNonArchived: len(t.manifest.Tasks)}, nil
}
func (t oneOffTracker) Refresh(ctx context.Context, revision uint64) (core.TrackerPage, error) {
	if revision != 0 {
		return core.TrackerPage{}, core.ErrRevision
	}
	return t.Page(ctx, "", len(t.manifest.Tasks))
}
func (t oneOffTracker) Get(_ context.Context, id core.TaskID, revision uint64) (core.Task, error) {
	if revision != 0 {
		return core.Task{}, core.ErrRevision
	}
	for _, task := range t.manifest.Tasks {
		if task.ID == id {
			return task, nil
		}
	}
	return core.Task{}, core.ErrPath
}
func (oneOffTracker) Create(context.Context, core.Task, uint64) (core.Task, error) {
	return core.Task{}, core.ErrTransition
}
func (oneOffTracker) Archive(context.Context, core.TaskID, string, uint64) error {
	return core.ErrTransition
}
