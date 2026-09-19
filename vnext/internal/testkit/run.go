package testkit

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/thebpandey/agent-team/vnext/internal/core"
	"github.com/thebpandey/agent-team/vnext/internal/run"
	"github.com/thebpandey/agent-team/vnext/internal/store"
	"github.com/thebpandey/agent-team/vnext/internal/tracker"
)

// AdmissionFixture is a complete, deterministic plan/team/tracker snapshot for
// admission tests. Its public fields deliberately mirror AppendAdmission.
type AdmissionFixture struct {
	Store           *store.Store
	Tracker         tracker.Tracker
	Run             core.RunID
	RunRevision     uint64
	Team            core.TeamID
	TeamRevision    uint64
	TrackerRevision uint64
	TaskRevisions   map[core.TaskID]uint64
	project         string
	writtenAt       string
}

// Batch returns a canonical, disjoint admission payload for n tasks. n is not
// constrained here so callers can exercise the service's batch-bound checks.
func (f AdmissionFixture) Batch(n int) run.AdmissionBatch {
	tasks := make([]core.TaskID, 0, n)
	paths := make([]string, 0, n)
	resources := make([]string, 0, n)
	for i := 0; i < n; i++ {
		tasks = append(tasks, core.TaskID(fmt.Sprintf("T-%04d", i+1)))
		paths = append(paths, fmt.Sprintf("src/task-%04d", i+1))
		resources = append(resources, fmt.Sprintf("resource:%04d", i+1))
	}
	batch := run.AdmissionBatch{
		RecordEnvelope:  core.RecordEnvelope{Schema: 1, Project: f.project, RunID: f.Run, WrittenAt: f.writtenAt, Revision: 1},
		BatchID:         fmt.Sprintf("batch-%d", n),
		Tasks:           tasks,
		Team:            f.Team,
		Sequence:        uint64(n),
		TrackerRevision: f.TrackerRevision,
		Paths:           paths,
		Resources:       resources,
	}
	batch.Fingerprint = admissionFingerprint(batch)
	return batch
}

func admissionFingerprint(batch run.AdmissionBatch) string {
	value := struct {
		Schema, Revision                         uint64
		Project, RunID, WrittenAt, BatchID, Team string
		Sequence, TrackerRevision                uint64
		Tasks                                    []core.TaskID
		Paths, Resources                         []string
	}{uint64(batch.Schema), batch.Revision, batch.Project, string(batch.RunID), batch.WrittenAt, batch.BatchID, string(batch.Team), batch.Sequence, batch.TrackerRevision, batch.Tasks, batch.Paths, batch.Resources}
	raw, _ := json.Marshal(value)
	sum := sha256.Sum256(raw)
	return "sha256:" + hex.EncodeToString(sum[:])
}
