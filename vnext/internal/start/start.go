// Package start admits one default plan task and produces, but never launches,
// its immutable host packet.
package start

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/thebpandey/agent-team/vnext/internal/admission"
	"github.com/thebpandey/agent-team/vnext/internal/core"
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
	task, err := selected.Get(ctx, team.Queue[0], currentRun.TrackerRevision)
	if err != nil {
		return Result{}, err
	}
	packet := core.AssignmentPacket{RecordEnvelope: core.RecordEnvelope{Schema: 1, Project: currentRun.Project, RunID: currentRun.ID, WrittenAt: currentRun.WrittenAt, Revision: currentRun.Revision}, SpecRevision: currentRun.SpecRevision, Task: task.ID, Team: team.ID, QueueFingerprint: team.QueueFingerprint, Owner: owner, Worktree: worktree, Base: base, Criteria: append([]string(nil), task.Criteria...), Scope: append([]string(nil), task.WritablePaths...), NextAction: "host_dispatch_required"}
	digest, err := knowledge.PacketDigest(packet)
	if err != nil {
		return Result{}, err
	}
	packetPath := filepath.ToSlash(filepath.Join(".agent-team", "packets", string(team.ID)+".json"))
	record := struct {
		Packet core.AssignmentPacket `json:"packet"`
		Digest string                `json:"digest"`
	}{Packet: packet, Digest: digest}
	if _, err := st.WriteJSON(packetPath, record, core.DefaultConfig().Storage.CanonicalBytes); err != nil {
		return Result{}, err
	}
	return Result{Run: currentRun, Team: team, Packet: packet, PacketDigest: digest, PacketPath: packetPath, HostDispatchRequired: true, AlreadyAdmitted: already}, nil
}

func taskRevision(manifest run.Run, id core.TaskID) uint64 {
	for _, task := range manifest.Tasks {
		if task.ID == id {
			return task.Revision
		}
	}
	return 0
}
