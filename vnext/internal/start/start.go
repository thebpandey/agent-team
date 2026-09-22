// Package start admits one default plan task and produces, but never launches,
// its immutable host packet.
package start

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"

	"github.com/thebpandey/agent-team/vnext/internal/admission"
	"github.com/thebpandey/agent-team/vnext/internal/contracts"
	"github.com/thebpandey/agent-team/vnext/internal/core"
	"github.com/thebpandey/agent-team/vnext/internal/dispatch"
	"github.com/thebpandey/agent-team/vnext/internal/knowledge"
	"github.com/thebpandey/agent-team/vnext/internal/run"
	"github.com/thebpandey/agent-team/vnext/internal/store"
	"github.com/thebpandey/agent-team/vnext/internal/tracker"
)

// Result is one durable admission and its host handoff request. The caller
// must obtain a native host acknowledgement before claiming a worker started.
type Result struct {
	Run                  run.Run
	Team                 run.TeamRecord
	Packet               core.AssignmentPacket
	PacketDigest         string
	PacketPath           string
	HostDispatchRequired bool
	AlreadyAdmitted      bool
}

// Delta is a fresh bounded packet for the next queued task. The retained
// handle is evidence of the idle host, not an acknowledgement of this packet.
type Delta struct {
	Packet       core.AssignmentPacket
	PacketDigest string
	PacketPath   string
	Retained     contracts.WorkerHandle
}

type packetRecord struct {
	Packet core.AssignmentPacket `json:"packet"`
	Digest string                `json:"digest"`
}

var projectLocks sync.Map // map[string]*sync.Mutex; matches admission's local project serialization.

// AdmitDefault selects exactly one currently ready plan task, writes its run
// and team projection through the existing admission commit, and returns an
// immutable packet for an external native host handoff.
func AdmitDefault(ctx context.Context, st *store.Store, project string, selected tracker.Tracker, owner, worktree, base string) (Result, error) {
	if st == nil || selected == nil || owner == "" || worktree == "" || base == "" {
		return Result{}, fmt.Errorf("%w: start admission input", core.ErrSettings)
	}
	prepared, err := run.PrepareSinglePlanAdmission(ctx, project, selected)
	if err != nil {
		return Result{}, err
	}
	return admitPrepared(ctx, st, selected, prepared, owner, worktree, base)
}

// AdmitDefaultRegistered creates the exact registered feature worktree for a
// prepared team before publishing the packet that names it.
func AdmitDefaultRegistered(ctx context.Context, st *store.Store, project string, selected tracker.Tracker, manager contracts.WorktreeManager, owner, base string) (Result, error) {
	if manager == nil {
		return Result{}, fmt.Errorf("%w: worktree manager", core.ErrSettings)
	}
	prepared, err := run.PrepareSinglePlanAdmission(ctx, project, selected)
	if err != nil {
		return Result{}, err
	}
	if err := validateStoreProject(st, prepared.Run.Root); err != nil {
		return Result{}, err
	}
	root := filepath.Join(prepared.Run.Root, ".agent-team", "worktrees", string(prepared.Team.ID))
	if err := validateWorktreeSpec(prepared, root, base); err != nil {
		return Result{}, err
	}
	var result Result
	err = withProjectLock(ctx, st, func() error {
		if ownerRun, found, ownerErr := existingTaskOwner(st, prepared.Batch.Tasks[0]); ownerErr != nil {
			return ownerErr
		} else if found && ownerRun != prepared.Run.ID {
			return fmt.Errorf("%w: task already admitted by %s", core.ErrRevision, ownerRun)
		}
		registered, createErr := manager.Create(ctx, contracts.WorktreeSpec{Run: prepared.Run.ID, Team: prepared.Team.ID, Root: root, Base: base, WritablePaths: append([]string(nil), prepared.Batch.Paths...)})
		if createErr != nil {
			return createErr
		}
		if registered.Run != prepared.Run.ID || registered.Team != prepared.Team.ID || registered.Path == "" {
			return fmt.Errorf("%w: registered worktree identity", core.ErrRevision)
		}
		var admissionErr error
		result, admissionErr = admitPreparedLocked(ctx, st, selected, prepared, owner, registered.Path, base)
		return admissionErr
	})
	return result, err
}

func admitPrepared(ctx context.Context, st *store.Store, selected tracker.Tracker, prepared run.PreparedPlanAdmission, owner, worktree, base string) (Result, error) {
	var result Result
	err := withProjectLock(ctx, st, func() error {
		var err error
		result, err = admitPreparedLocked(ctx, st, selected, prepared, owner, worktree, base)
		return err
	})
	return result, err
}

func admitPreparedLocked(ctx context.Context, st *store.Store, selected tracker.Tracker, prepared run.PreparedPlanAdmission, owner, worktree, base string) (Result, error) {
	if err := validateBoundary(st, prepared, worktree, base); err != nil {
		return Result{}, err
	}
	if ownerRun, found, err := existingTaskOwner(st, prepared.Batch.Tasks[0]); err != nil {
		return Result{}, err
	} else if found && ownerRun != prepared.Run.ID {
		return Result{}, fmt.Errorf("%w: task already admitted by %s", core.ErrRevision, ownerRun)
	}
	repos := run.NewRepositories(st)
	currentRun, err := repos.Runs.Read(ctx, prepared.Run.ID)
	if errors.Is(err, os.ErrNotExist) {
		if currentRun, err = repos.Runs.Initialize(ctx, prepared.Run); err != nil {
			return Result{}, err
		}
		if _, err := repos.Teams.Initialize(ctx, prepared.Team); err != nil {
			return Result{}, err
		}
	} else if err != nil {
		return Result{}, err
	}
	team, err := repos.Teams.Read(ctx, prepared.Team.ID)
	if errors.Is(err, os.ErrNotExist) {
		for _, slot := range currentRun.Teams {
			if slot.ID == prepared.Team.ID {
				team, err = repos.Teams.Initialize(ctx, slot)
				break
			}
		}
	}
	if err != nil {
		return Result{}, err
	}
	if _, err := reconcileTeamLocked(ctx, st, currentRun.ID, team.ID); err != nil {
		return Result{}, err
	}
	currentRun, err = repos.Runs.Read(ctx, currentRun.ID)
	if err != nil {
		return Result{}, err
	}
	team, err = repos.Teams.Read(ctx, team.ID)
	if err != nil {
		return Result{}, err
	}
	already := len(team.Queue) > 0
	if !already {
		if _, err := admission.AppendAdmission(ctx, st, selected, currentRun.ID, currentRun.Revision, team.ID, team.Revision, prepared.Batch.TrackerRevision, map[core.TaskID]uint64{prepared.Batch.Tasks[0]: taskRevision(prepared.Run, prepared.Batch.Tasks[0])}, prepared.Batch); err != nil {
			return Result{}, err
		}
		currentRun, err = repos.Runs.Read(ctx, currentRun.ID)
		if err != nil {
			return Result{}, err
		}
		team, err = repos.Teams.Read(ctx, team.ID)
		if err != nil {
			return Result{}, err
		}
	}
	if len(team.Queue) != 1 || team.Queue[0] != prepared.Batch.Tasks[0] {
		return Result{}, fmt.Errorf("%w: ambiguous in-flight start", core.ErrRevision)
	}
	if team.IntentDigest != "" {
		existing, packetPath, err := readPacket(st, team.ID, team.IntentDigest)
		if err != nil {
			return Result{}, fmt.Errorf("%w: admitted packet unavailable", core.ErrRevision)
		}
		if existing.Packet.RunID != currentRun.ID || existing.Packet.Team != team.ID || existing.Packet.Task != team.Queue[0] || existing.Packet.Owner != owner || existing.Packet.Worktree != worktree || existing.Packet.Base != base {
			return Result{}, fmt.Errorf("%w: admitted packet differs", core.ErrRevision)
		}
		return Result{Run: currentRun, Team: team, Packet: existing.Packet, PacketDigest: existing.Digest, PacketPath: packetPath, HostDispatchRequired: true, AlreadyAdmitted: true}, nil
	}
	task, err := selected.Get(ctx, team.Queue[0], currentRun.TrackerRevision)
	if err != nil {
		return Result{}, err
	}
	packet := core.AssignmentPacket{RecordEnvelope: core.RecordEnvelope{Schema: 1, Project: currentRun.Project, RunID: currentRun.ID, WrittenAt: currentRun.WrittenAt, Revision: currentRun.Revision}, SpecRevision: currentRun.SpecRevision, Task: task.ID, Team: team.ID, QueueFingerprint: team.QueueFingerprint, Owner: owner, Worktree: worktree, Base: base, Criteria: append([]string(nil), task.Criteria...), Scope: append([]string(nil), task.WritablePaths...), NextAction: "host_dispatch_required"}
	digest, err := knowledge.PacketDigest(packet)
	if err != nil {
		return Result{}, err
	}
	packetPath := packetPathFor(team.ID, digest)
	if err := createOrReadPacket(st, packetPath, packetRecord{Packet: packet, Digest: digest}); err != nil {
		return Result{}, err
	}
	team, err = mutateTeamLocked(ctx, st, currentRun.ID, team.ID, func(value *run.TeamRecord) error {
		value.IntentDigest = digest
		return nil
	})
	if err != nil {
		return Result{}, err
	}
	return Result{Run: currentRun, Team: team, Packet: packet, PacketDigest: digest, PacketPath: packetPath, HostDispatchRequired: true, AlreadyAdmitted: already}, nil
}

// Acknowledge records the exact native host handle for a reserved packet. It
// accepts no substitute task, queue fingerprint, or candidate revision.
func Acknowledge(ctx context.Context, st *store.Store, teamID core.TeamID, digest string, handle contracts.WorkerHandle) (run.TeamRecord, error) {
	team, err := consistentTeam(ctx, st, teamID)
	if err != nil {
		return run.TeamRecord{}, err
	}
	packet, _, err := readPacket(st, team.ID, digest)
	if err != nil || packet.Digest != team.IntentDigest || packet.Packet.Task != team.Queue[0] || packet.Packet.Team != team.ID {
		return run.TeamRecord{}, fmt.Errorf("%w: stored host packet", core.ErrRevision)
	}
	if team.IntentDigest != digest || team.Handle.Identity != "" || len(team.Queue) == 0 || handle.Identity == "" || handle.Reviewer || handle.Run != team.RunID || handle.Team != team.ID || handle.Task != team.Queue[0] || handle.PacketDigest != digest || handle.CandidateRevision != packet.Packet.SpecRevision || (team.RetainedHandle.Identity != "" && (handle.Host != team.RetainedHandle.Host || handle.Identity != team.RetainedHandle.Identity)) {
		return run.TeamRecord{}, fmt.Errorf("%w: host acknowledgement", core.ErrRevision)
	}
	return mutateTeam(ctx, st, team.RunID, team.ID, func(value *run.TeamRecord) error {
		value.Handle = handle
		if value.RetainedHandle.Identity == "" {
			value.RetainedHandle = handle
		}
		return nil
	})
}

// Complete records a completed head task only from the retained acknowledged
// handle. Independent review and idle proof are separate transitions.
func Complete(ctx context.Context, st *store.Store, teamID core.TeamID, handle contracts.WorkerHandle) (run.TeamRecord, error) {
	return mutateOwnedHead(ctx, st, teamID, handle, func(value *run.TeamRecord) { value.CompletedTask = value.Queue[0] })
}

// RecordIndependentClean requires a reviewer identity distinct from the
// retained developer handle before a completed head can be consumed.
func RecordIndependentClean(ctx context.Context, st *store.Store, teamID core.TeamID, reviewer string) (run.TeamRecord, error) {
	if reviewer == "" {
		return run.TeamRecord{}, core.ErrRevision
	}
	return mutateTeamByID(ctx, st, teamID, func(value *run.TeamRecord) error {
		if value.Handle.Identity == "" || value.CompletedTask != value.Queue[0] || reviewer == value.Handle.Identity {
			return fmt.Errorf("%w: independent clean review", core.ErrRevision)
		}
		value.ReviewedTask = value.Queue[0]
		return nil
	})
}

// RecordIdle records native evidence that the acknowledged retained handle is
// idle after completion and independent CLEAN review.
func RecordIdle(ctx context.Context, st *store.Store, teamID core.TeamID, handle contracts.WorkerHandle) (run.TeamRecord, error) {
	return mutateTeamByID(ctx, st, teamID, func(value *run.TeamRecord) error {
		if len(value.Queue) == 0 || value.Handle != handle || value.CompletedTask != value.Queue[0] || value.ReviewedTask != value.Queue[0] {
			return fmt.Errorf("%w: idle before independent clean", core.ErrRevision)
		}
		value.HostIdle = true
		return nil
	})
}

// ConsumeHead removes only a completed, independently CLEAN-reviewed head
// after the retained handle is proven idle. A non-empty remaining queue keeps
// that same handle available for a bounded follow-up delta.
func ConsumeHead(ctx context.Context, st *store.Store, teamID core.TeamID) (core.TaskID, contracts.WorkerHandle, run.TeamRecord, error) {
	var consumed core.TaskID
	var handle contracts.WorkerHandle
	team, err := mutateTeamByID(ctx, st, teamID, func(value *run.TeamRecord) error {
		if len(value.Queue) == 0 || value.CompletedTask != value.Queue[0] || value.ReviewedTask != value.Queue[0] || !value.HostIdle || value.Handle.Identity == "" {
			return fmt.Errorf("%w: queue head is not reusable", core.ErrTransition)
		}
		if len(value.Queue) > 1 {
			return fmt.Errorf("%w: follow-up delta required", core.ErrTransition)
		}
		consumed, handle = value.Queue[0], value.Handle
		value.Queue = append([]core.TaskID(nil), value.Queue[1:]...)
		value.QueueFingerprint = run.QueueFingerprint(value.Queue)
		value.CompletedTask, value.ReviewedTask, value.HostIdle = "", "", false
		if len(value.Queue) == 0 {
			value.State, value.IntentDigest, value.Handle = core.Idle, "", contracts.WorkerHandle{}
		}
		return nil
	})
	return consumed, handle, team, err
}

// ConsumeForFollowup consumes a proven completed head and reserves a fresh
// immutable packet for the next already-queued task. The old handle remains
// evidence of the retained host; it cannot execute the new task until that
// host acknowledges the new packet with the same identity.
func ConsumeForFollowup(ctx context.Context, st *store.Store, selected tracker.Tracker, teamID core.TeamID) (core.TaskID, Delta, run.TeamRecord, error) {
	if selected == nil {
		return "", Delta{}, run.TeamRecord{}, core.ErrSettings
	}
	var consumed core.TaskID
	var delta Delta
	var updated run.TeamRecord
	err := withProjectLock(ctx, st, func() error {
		repos := run.NewRepositories(st)
		team, err := repos.Teams.Read(ctx, teamID)
		if err != nil {
			return err
		}
		team, err = reconcileTeamLocked(ctx, st, team.RunID, teamID)
		if err != nil {
			return err
		}
		if len(team.Queue) < 2 || team.CompletedTask != team.Queue[0] || team.ReviewedTask != team.Queue[0] || !team.HostIdle || team.Handle.Identity == "" {
			return fmt.Errorf("%w: queue head is not reusable", core.ErrTransition)
		}
		prior, _, err := readPacket(st, team.ID, team.IntentDigest)
		if err != nil || prior.Packet.Task != team.Queue[0] {
			return fmt.Errorf("%w: prior packet intent", core.ErrRevision)
		}
		manifest, err := repos.Runs.Read(ctx, team.RunID)
		if err != nil {
			return err
		}
		nextTask, err := selected.Get(ctx, team.Queue[1], manifest.TrackerRevision)
		if err != nil || nextTask.ID != team.Queue[1] {
			return fmt.Errorf("%w: queued follow-up task", core.ErrRevision)
		}
		nextQueue := append([]core.TaskID(nil), team.Queue[1:]...)
		packet := core.AssignmentPacket{
			RecordEnvelope:   core.RecordEnvelope{Schema: 1, Project: manifest.Project, RunID: manifest.ID, WrittenAt: manifest.WrittenAt, Revision: manifest.Revision},
			SpecRevision:     prior.Packet.SpecRevision,
			Task:             nextTask.ID,
			Team:             team.ID,
			QueueFingerprint: run.QueueFingerprint(nextQueue),
			Owner:            prior.Packet.Owner,
			Worktree:         prior.Packet.Worktree,
			Base:             prior.Packet.Base,
			Criteria:         append([]string(nil), nextTask.Criteria...),
			Scope:            append([]string(nil), nextTask.WritablePaths...),
			NextAction:       "host_followup_required",
		}
		digest, err := knowledge.PacketDigest(packet)
		if err != nil {
			return err
		}
		path := packetPathFor(team.ID, digest)
		if err := createOrReadPacket(st, path, packetRecord{Packet: packet, Digest: digest}); err != nil {
			return err
		}
		consumed = team.Queue[0]
		retained := team.Handle
		updated, err = mutateTeamLocked(ctx, st, team.RunID, team.ID, func(value *run.TeamRecord) error {
			if len(value.Queue) < 2 || value.Queue[0] != consumed || value.Handle != retained || value.IntentDigest != prior.Digest || value.CompletedTask != consumed || value.ReviewedTask != consumed || !value.HostIdle {
				return fmt.Errorf("%w: follow-up state changed", core.ErrRevision)
			}
			value.Queue = append([]core.TaskID(nil), value.Queue[1:]...)
			value.QueueFingerprint = run.QueueFingerprint(value.Queue)
			value.IntentDigest = digest
			value.RetainedHandle = retained
			value.Handle = contracts.WorkerHandle{}
			value.CompletedTask, value.ReviewedTask, value.HostIdle = "", "", false
			return nil
		})
		if err != nil {
			return err
		}
		delta = Delta{Packet: packet, PacketDigest: digest, PacketPath: path, Retained: retained}
		return nil
	})
	return consumed, delta, updated, err
}

func mutateOwnedHead(ctx context.Context, st *store.Store, teamID core.TeamID, handle contracts.WorkerHandle, mutate func(*run.TeamRecord)) (run.TeamRecord, error) {
	return mutateTeamByID(ctx, st, teamID, func(value *run.TeamRecord) error {
		if len(value.Queue) == 0 || value.Handle != handle {
			return fmt.Errorf("%w: retained host handle", core.ErrRevision)
		}
		mutate(value)
		if value.CompletedTask == "" && value.HostIdle {
			return core.ErrRevision
		}
		return nil
	})
}

func mutateTeamByID(ctx context.Context, st *store.Store, teamID core.TeamID, mutate func(*run.TeamRecord) error) (run.TeamRecord, error) {
	team, err := consistentTeam(ctx, st, teamID)
	if err != nil {
		return run.TeamRecord{}, err
	}
	return mutateTeam(ctx, st, team.RunID, teamID, mutate)
}

func mutateTeam(ctx context.Context, st *store.Store, runID core.RunID, teamID core.TeamID, mutate func(*run.TeamRecord) error) (run.TeamRecord, error) {
	var updated run.TeamRecord
	err := withProjectLock(ctx, st, func() error {
		var err error
		updated, err = mutateTeamLocked(ctx, st, runID, teamID, mutate)
		return err
	})
	return updated, err
}

func mutateTeamLocked(ctx context.Context, st *store.Store, runID core.RunID, teamID core.TeamID, mutate func(*run.TeamRecord) error) (run.TeamRecord, error) {
	repos := run.NewRepositories(st)
	currentRun, err := repos.Runs.Read(ctx, runID)
	if err != nil {
		return run.TeamRecord{}, err
	}
	if _, err := reconcileTeamLocked(ctx, st, currentRun.ID, teamID); err != nil {
		return run.TeamRecord{}, err
	}
	currentTeam, err := repos.Teams.Read(ctx, teamID)
	if err != nil {
		return run.TeamRecord{}, err
	}
	next := currentTeam
	if err := mutate(&next); err != nil {
		return run.TeamRecord{}, err
	}
	slot := -1
	for index := range currentRun.Teams {
		if currentRun.Teams[index].ID == teamID {
			slot = index
			break
		}
	}
	if slot < 0 {
		return run.TeamRecord{}, core.ErrRevision
	}
	projected := next
	projected.Revision = currentTeam.Revision + 1
	nextRun := currentRun
	nextRun.Teams = append([]run.TeamRecord(nil), currentRun.Teams...)
	nextRun.Teams[slot] = projected
	if _, err := repos.Runs.CompareAndSwap(ctx, currentRun.ID, currentRun.Revision, nextRun); err != nil {
		return run.TeamRecord{}, err
	}
	updated, err := repos.Teams.CompareAndSwap(ctx, currentTeam.ID, currentTeam.Revision, next)
	if err == nil {
		return updated, nil
	}
	// The run slot is authoritative. A crash or a failed second projection is
	// recoverable under the same project lock on the next transition.
	if repaired, repairErr := reconcileTeamLocked(ctx, st, currentRun.ID, teamID); repairErr == nil {
		return repaired, nil
	}
	return run.TeamRecord{}, err
}

func consistentTeam(ctx context.Context, st *store.Store, teamID core.TeamID) (run.TeamRecord, error) {
	var value run.TeamRecord
	err := withProjectLock(ctx, st, func() error {
		repos := run.NewRepositories(st)
		team, err := repos.Teams.Read(ctx, teamID)
		if err != nil {
			return err
		}
		value, err = reconcileTeamLocked(ctx, st, team.RunID, teamID)
		return err
	})
	return value, err
}

func reconcileTeam(ctx context.Context, st *store.Store, runID core.RunID, teamID core.TeamID) error {
	return withProjectLock(ctx, st, func() error {
		_, err := reconcileTeamLocked(ctx, st, runID, teamID)
		return err
	})
}

func reconcileTeamLocked(ctx context.Context, st *store.Store, runID core.RunID, teamID core.TeamID) (run.TeamRecord, error) {
	repos := run.NewRepositories(st)
	manifest, err := repos.Runs.Read(ctx, runID)
	if err != nil {
		return run.TeamRecord{}, err
	}
	var slot run.TeamRecord
	found := false
	for _, candidate := range manifest.Teams {
		if candidate.ID == teamID {
			slot, found = candidate, true
			break
		}
	}
	if !found {
		return run.TeamRecord{}, core.ErrRevision
	}
	team, err := repos.Teams.Read(ctx, teamID)
	if errors.Is(err, os.ErrNotExist) {
		return repos.Teams.Initialize(ctx, slot)
	}
	if err != nil {
		return run.TeamRecord{}, err
	}
	if reflect.DeepEqual(team, slot) {
		return team, nil
	}
	if team.Revision+1 != slot.Revision {
		return run.TeamRecord{}, fmt.Errorf("%w: inconsistent team projection", core.ErrRevision)
	}
	return repos.Teams.CompareAndSwap(ctx, teamID, team.Revision, slot)
}

func withProjectLock(ctx context.Context, st *store.Store, action func() error) error {
	if st == nil {
		return core.ErrSettings
	}
	root, err := filepath.Abs(st.Root)
	if err != nil {
		return core.ErrPath
	}
	value, _ := projectLocks.LoadOrStore(root, &sync.Mutex{})
	mu := value.(*sync.Mutex)
	mu.Lock()
	defer mu.Unlock()
	guard, err := store.AcquireProjectMutation(ctx, root, "start", "team-transition")
	if err != nil {
		return err
	}
	defer guard.Release()
	return action()
}

func existingTaskOwner(st *store.Store, task core.TaskID) (core.RunID, bool, error) {
	entries, err := os.ReadDir(filepath.Join(st.Root, ".agent-team", "runs"))
	if errors.Is(err, os.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("%w: runs directory", core.ErrPath)
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		id := core.RunID(strings.TrimSuffix(entry.Name(), ".json"))
		manifest, err := run.NewRepositories(st).Runs.Read(context.Background(), id)
		if err != nil {
			return "", false, fmt.Errorf("%w: stored run", core.ErrRevision)
		}
		for _, team := range manifest.Teams {
			for _, queued := range team.Queue {
				if queued == task {
					return manifest.ID, true, nil
				}
			}
		}
	}
	// Admission history remains the bounded ownership evidence after a completed
	// queue head is consumed. It prevents an unchanged ready tracker task from
	// being admitted again without treating every snapshot task as admitted.
	historyRoot := filepath.Join(st.Root, ".agent-team", "admissions")
	teams, err := os.ReadDir(historyRoot)
	if errors.Is(err, os.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("%w: admission history", core.ErrPath)
	}
	for _, team := range teams {
		if !team.IsDir() || team.Name() == "by-run" {
			continue
		}
		entries, err := os.ReadDir(filepath.Join(historyRoot, team.Name()))
		if err != nil {
			return "", false, fmt.Errorf("%w: admission history", core.ErrPath)
		}
		for _, entry := range entries {
			if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
				continue
			}
			var commit struct {
				Batch struct {
					Tasks []core.TaskID `json:"tasks"`
				} `json:"batch"`
				AfterRun struct {
					ID core.RunID `json:"id"`
				} `json:"afterRun"`
			}
			relative := filepath.ToSlash(filepath.Join(".agent-team", "admissions", team.Name(), entry.Name()))
			if err := st.ReadJSON(relative, core.DefaultConfig().Storage.CanonicalBytes, &commit); err != nil {
				return "", false, fmt.Errorf("%w: admission history", core.ErrRevision)
			}
			for _, admitted := range commit.Batch.Tasks {
				if admitted == task {
					return commit.AfterRun.ID, true, nil
				}
			}
		}
	}
	return "", false, nil
}

func validateBoundary(st *store.Store, prepared run.PreparedPlanAdmission, worktree, base string) error {
	if err := validateStoreProject(st, prepared.Run.Root); err != nil {
		return err
	}
	return validateWorktreeSpec(prepared, worktree, base)
}

func validateStoreProject(st *store.Store, project string) error {
	if st == nil {
		return core.ErrSettings
	}
	storeRoot, err := filepath.Abs(st.Root)
	if err != nil {
		return core.ErrPath
	}
	projectRoot, err := filepath.Abs(project)
	if err != nil {
		return core.ErrPath
	}
	if real, err := filepath.EvalSymlinks(storeRoot); err == nil {
		storeRoot = real
	}
	if real, err := filepath.EvalSymlinks(projectRoot); err == nil {
		projectRoot = real
	}
	if storeRoot != projectRoot {
		return fmt.Errorf("%w: store/project mismatch", core.ErrPath)
	}
	return nil
}

func validateWorktreeSpec(prepared run.PreparedPlanAdmission, worktree, base string) error {
	spec := contracts.WorktreeSpec{Run: prepared.Run.ID, Team: prepared.Team.ID, Root: worktree, Base: base, WritablePaths: append([]string(nil), prepared.Batch.Paths...)}
	return dispatch.ValidatePacket(core.AssignmentPacket{RecordEnvelope: core.RecordEnvelope{RunID: prepared.Run.ID}, Team: prepared.Team.ID, Worktree: worktree, Base: base}, spec)
}

func packetPathFor(teamID core.TeamID, digest string) string {
	return filepath.ToSlash(filepath.Join(".agent-team", "packets", string(teamID), strings.TrimPrefix(digest, "sha256:")+".json"))
}

func readPacket(st *store.Store, teamID core.TeamID, digest string) (packetRecord, string, error) {
	path := packetPathFor(teamID, digest)
	var record packetRecord
	if err := st.ReadJSON(path, core.DefaultConfig().Storage.CanonicalBytes, &record); err != nil {
		return packetRecord{}, path, err
	}
	if record.Digest != digest || knowledge.ValidatePacket(record.Packet, record.Digest) != nil || record.Packet.Team != teamID {
		return packetRecord{}, path, core.ErrRevision
	}
	return record, path, nil
}

func createOrReadPacket(st *store.Store, path string, wanted packetRecord) error {
	if _, err := st.CreateJSON(path, wanted, core.DefaultConfig().Storage.CanonicalBytes); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrExist) {
		return err
	}
	var existing packetRecord
	if err := st.ReadJSON(path, core.DefaultConfig().Storage.CanonicalBytes, &existing); err != nil {
		return err
	}
	if existing.Digest != wanted.Digest || !reflect.DeepEqual(existing.Packet, wanted.Packet) || knowledge.ValidatePacket(existing.Packet, existing.Digest) != nil {
		return fmt.Errorf("%w: conflicting packet reservation", core.ErrRevision)
	}
	return nil
}

func taskRevision(manifest run.Run, id core.TaskID) uint64 {
	for _, task := range manifest.Tasks {
		if task.ID == id {
			return task.Revision
		}
	}
	return 0
}
