package admission_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/thebpandey/agent-team/vnext/internal/admission"
	"github.com/thebpandey/agent-team/vnext/internal/core"
	"github.com/thebpandey/agent-team/vnext/internal/run"
	"github.com/thebpandey/agent-team/vnext/internal/store"
	"github.com/thebpandey/agent-team/vnext/internal/testkit"
)

type admissionProcessRequest struct {
	Root            string                 `json:"root"`
	Run             core.RunID             `json:"run"`
	RunRevision     uint64                 `json:"runRevision"`
	Team            core.TeamID            `json:"team"`
	TeamRevision    uint64                 `json:"teamRevision"`
	TrackerRevision uint64                 `json:"trackerRevision"`
	TaskRevisions   map[core.TaskID]uint64 `json:"taskRevisions"`
	Batch           run.AdmissionBatch     `json:"batch"`
	Tasks           []core.Task            `json:"tasks"`
}

type admissionProcessTracker struct {
	revision uint64
	tasks    []core.Task
}

func (t admissionProcessTracker) Page(ctx context.Context, cursor string, limit int) (core.TrackerPage, error) {
	if err := ctx.Err(); err != nil {
		return core.TrackerPage{}, err
	}
	if cursor != "" || limit < len(t.tasks) {
		return core.TrackerPage{}, fmt.Errorf("%w: invalid subprocess page", core.ErrLimit)
	}
	return core.TrackerPage{TrackerRevision: t.revision, TotalNonArchived: len(t.tasks), Tasks: t.tasks}, nil
}

func (t admissionProcessTracker) Get(_ context.Context, id core.TaskID, expected uint64) (core.Task, error) {
	if expected != t.revision {
		return core.Task{}, core.ErrRevision
	}
	for _, task := range t.tasks {
		if task.ID == id {
			return task, nil
		}
	}
	return core.Task{}, os.ErrNotExist
}

func (t admissionProcessTracker) Refresh(ctx context.Context, expected uint64) (core.TrackerPage, error) {
	if expected != t.revision {
		return core.TrackerPage{}, core.ErrRevision
	}
	return t.Page(ctx, "", len(t.tasks))
}
func (admissionProcessTracker) Create(context.Context, core.Task, uint64) (core.Task, error) {
	return core.Task{}, core.ErrSettings
}
func (admissionProcessTracker) Archive(context.Context, core.TaskID, string, uint64) error {
	return core.ErrSettings
}

func TestAdmissionSubprocessHelper(t *testing.T) {
	encoded := os.Getenv("ADMISSION_PROCESS_REQUEST")
	if encoded == "" {
		return
	}
	var request admissionProcessRequest
	if err := json.Unmarshal([]byte(encoded), &request); err != nil {
		t.Fatal(err)
	}
	out, err := admission.AppendAdmission(context.Background(), store.New(request.Root, core.StorageLimits{}), admissionProcessTracker{revision: request.TrackerRevision, tasks: request.Tasks}, request.Run, request.RunRevision, request.Team, request.TeamRevision, request.TrackerRevision, request.TaskRevisions, request.Batch)
	if err != nil {
		t.Fatalf("subprocess admission: %v", err)
	}
	fmt.Fprintf(os.Stdout, "ADMISSION=%s", out.Kind)
}

func TestAppendAdmissionSubprocessContention(t *testing.T) {
	for _, conflict := range []bool{false, true} {
		name := "identical"
		if conflict {
			name = "conflicting"
		}
		t.Run(name, func(t *testing.T) {
			f := testkit.NewAdmissionFixture(t)
			first := f.Batch(1)
			second := first
			if conflict {
				second = f.Batch(2)
			}
			requests := []admissionProcessRequest{processRequest(f, first), processRequest(f, second)}
			results := make(chan string, len(requests))
			for _, request := range requests {
				request := request
				go func() {
					raw, err := json.Marshal(request)
					if err != nil {
						results <- "marshal: " + err.Error()
						return
					}
					cmd := exec.Command(os.Args[0], "-test.run=^TestAdmissionSubprocessHelper$")
					cmd.Env = append(os.Environ(), "ADMISSION_PROCESS_REQUEST="+string(raw))
					output, err := cmd.CombinedOutput()
					if err != nil {
						results <- "subprocess: " + err.Error() + ": " + string(output)
						return
					}
					results <- string(output)
				}()
			}
			created := 0
			for range requests {
				result := <-results
				if strings.Contains(result, "ADMISSION=created") {
					created++
					continue
				}
				if conflict && strings.Contains(result, "ADMISSION=stale") {
					continue
				}
				if !conflict && strings.Contains(result, "ADMISSION=duplicate") {
					continue
				}
				t.Fatal(result)
			}
			if created != 1 {
				t.Fatalf("subprocess created=%d", created)
			}
		})
	}
}

func TestAdmissionSnapshotReadinessDependenciesAndExactRevisions(t *testing.T) {
	for _, test := range []struct {
		name      string
		mutate    func(*admissionProcessTracker, map[core.TaskID]uint64)
		want      error
		wantStale bool
	}{
		{
			name: "tracker-revision",
			mutate: func(tr *admissionProcessTracker, _ map[core.TaskID]uint64) {
				tr.revision++
			},
			wantStale: true,
		},
		{
			name: "task-revision",
			mutate: func(_ *admissionProcessTracker, revisions map[core.TaskID]uint64) {
				revisions["T-0001"]++
			},
			want: core.ErrRevision,
		},
		{
			name: "not-ready",
			mutate: func(tr *admissionProcessTracker, _ map[core.TaskID]uint64) {
				tr.tasks[0].State = core.Blocked
			},
			want: core.ErrPhase,
		},
		{
			name: "unfinished-dependency",
			mutate: func(tr *admissionProcessTracker, _ map[core.TaskID]uint64) {
				tr.tasks[0].Dependencies = []core.TaskID{"DEPENDENCY"}
				tr.tasks = append(tr.tasks, core.Task{ID: "DEPENDENCY", State: core.Ready})
			},
			want: core.ErrPhase,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			f := testkit.NewAdmissionFixture(t)
			request := processRequest(f, f.Batch(1))
			tr := admissionProcessTracker{revision: request.TrackerRevision, tasks: request.Tasks}
			revisions := make(map[core.TaskID]uint64, len(request.TaskRevisions))
			for id, revision := range request.TaskRevisions {
				revisions[id] = revision
			}
			test.mutate(&tr, revisions)
			before := testkit.SnapshotTree(t, f.Store.Root)
			out, err := admission.AppendAdmission(context.Background(), f.Store, tr, request.Run, request.RunRevision, request.Team, request.TeamRevision, request.TrackerRevision, revisions, request.Batch)
			if test.wantStale {
				if err != nil || out.Kind != admission.Stale {
					t.Fatalf("snapshot mismatch = %#v, %v", out, err)
				}
			} else if !errors.Is(err, test.want) {
				t.Fatalf("validation = %v, want %v", err, test.want)
			}
			testkit.RequireNoWrites(t, f.Store.Root, before)
		})
	}
}

func TestAdmissionConflictsReleaseOnlyForIdleOrTerminalTeams(t *testing.T) {
	for _, state := range []core.TaskState{core.Working, core.Idle, core.Cancelled} {
		t.Run(string(state), func(t *testing.T) {
			f := testkit.NewAdmissionFixture(t)
			var current run.Run
			if err := f.Store.ReadJSON(".agent-team/runs/"+string(f.Run)+".json", 16<<20, &current); err != nil {
				t.Fatal(err)
			}
			other := run.TeamRecord{
				RecordEnvelope: core.RecordEnvelope{Schema: 1, Project: current.Project, RunID: current.ID, WrittenAt: current.WrittenAt, Revision: 1},
				ID:             core.TeamID(string(f.Run) + "-team-2"),
				State:          state,
				Paths:          []string{"src/task-0001"},
				Resources:      []string{"resource:0001"},
			}
			if state != core.Idle {
				other.Queue = []core.TaskID{"T-0002"}
				encoded, err := json.Marshal(other.Queue)
				if err != nil {
					t.Fatal(err)
				}
				sum := sha256.Sum256(encoded)
				other.QueueFingerprint = "sha256:" + hex.EncodeToString(sum[:])
			}
			current.Teams = append(current.Teams, other)
			if _, err := f.Store.WriteJSON(".agent-team/runs/"+string(f.Run)+".json", current, 16<<20); err != nil {
				t.Fatal(err)
			}
			before := testkit.SnapshotTree(t, f.Store.Root)
			out, err := admission.AppendAdmission(context.Background(), f.Store, f.Tracker, f.Run, f.RunRevision, f.Team, f.TeamRevision, f.TrackerRevision, f.TaskRevisions, f.Batch(1))
			if state == core.Working {
				if !errors.Is(err, core.ErrBatch) {
					t.Fatalf("active conflict = %#v, %v", out, err)
				}
				testkit.RequireNoWrites(t, f.Store.Root, before)
				return
			}
			if err != nil || out.Kind != admission.Created {
				t.Fatalf("released %s conflict = %#v, %v", state, out, err)
			}
		})
	}
}

func TestAdmissionDuplicatePrecedesStale(t *testing.T) {
	f := testkit.NewAdmissionFixture(t)
	first := f.Batch(1)
	if out, err := admission.AppendAdmission(context.Background(), f.Store, f.Tracker, f.Run, f.RunRevision, f.Team, f.TeamRevision, f.TrackerRevision, f.TaskRevisions, first); err != nil || out.Kind != admission.Created {
		t.Fatalf("first = %#v, %v", out, err)
	}
	if out, err := admission.AppendAdmission(context.Background(), f.Store, f.Tracker, f.Run, f.RunRevision, f.Team, f.TeamRevision, f.TrackerRevision, f.TaskRevisions, first); err != nil || out.Kind != admission.Duplicate {
		t.Fatalf("exact retry = %#v, %v", out, err)
	}
	conflict := f.Batch(2)
	if out, err := admission.AppendAdmission(context.Background(), f.Store, f.Tracker, f.Run, f.RunRevision, f.Team, f.TeamRevision, f.TrackerRevision, f.TaskRevisions, conflict); err != nil || out.Kind != admission.Stale {
		t.Fatalf("different proposal = %#v, %v", out, err)
	}
}

func processRequest(f testkit.AdmissionFixture, batch run.AdmissionBatch) admissionProcessRequest {
	tasks := make([]core.Task, 0, len(batch.Tasks))
	for i, id := range batch.Tasks {
		tasks = append(tasks, core.Task{
			RecordEnvelope: core.RecordEnvelope{Revision: f.TaskRevisions[id]},
			ID:             id,
			Objective:      "subprocess fixture",
			State:          core.Ready,
			WritablePaths:  []string{batch.Paths[i]},
			Resources:      []string{batch.Resources[i]},
		})
	}
	return admissionProcessRequest{Root: f.Store.Root, Run: f.Run, RunRevision: f.RunRevision, Team: f.Team, TeamRevision: f.TeamRevision, TrackerRevision: f.TrackerRevision, TaskRevisions: f.TaskRevisions, Batch: batch, Tasks: tasks}
}
