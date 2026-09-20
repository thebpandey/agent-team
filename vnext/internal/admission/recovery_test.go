package admission

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"

	"github.com/thebpandey/agent-team/vnext/internal/core"
	"github.com/thebpandey/agent-team/vnext/internal/run"
	"github.com/thebpandey/agent-team/vnext/internal/testkit"
)

func TestAppendAdmissionRechecksCommitAfterConcurrentProjection(t *testing.T) {
	f := testkit.NewAdmissionFixture(t)
	batch := f.Batch(1)
	root, err := canonicalRoot(f.Store.Root)
	if err != nil {
		t.Fatal(err)
	}

	reached := make(chan struct{})
	release := make(chan struct{})
	var first sync.Once
	admissionFault = func(point admissionFaultPoint) error {
		if point != faultAfterCommitLookup {
			return nil
		}
		block := false
		first.Do(func() { block = true })
		if block {
			close(reached)
			<-release
		}
		return nil
	}
	t.Cleanup(func() {
		admissionFault = nil
		admissionLocks.Delete(root)
	})

	type result struct {
		out AdmissionOutcome
		err error
	}
	delayed := make(chan result, 1)
	go func() {
		out, err := AppendAdmission(context.Background(), f.Store, f.Tracker, f.Run, f.RunRevision, f.Team, f.TeamRevision, f.TrackerRevision, f.TaskRevisions, batch)
		delayed <- result{out: out, err: err}
	}()
	<-reached

	// A separate process has its own in-memory lock, so make the second call
	// use another mutex while sharing the same durable store.
	admissionLocks.Delete(root)
	winner, winnerErr := AppendAdmission(context.Background(), f.Store, f.Tracker, f.Run, f.RunRevision, f.Team, f.TeamRevision, f.TrackerRevision, f.TaskRevisions, batch)
	close(release)
	retry := <-delayed

	if winnerErr != nil || winner.Kind != Created {
		t.Fatalf("winner = %#v, %v; want created", winner, winnerErr)
	}
	if retry.err != nil || retry.out.Kind != Duplicate {
		t.Fatalf("delayed retry = %#v, %v; want duplicate", retry.out, retry.err)
	}
}

func TestAppendAdmissionInterruptionConvergesOnRetry(t *testing.T) {
	points := []admissionFaultPoint{faultBeforeCommit, faultAfterCommit, faultAfterRunProjection, faultAfterTeamProjection}
	for _, point := range points {
		t.Run(faultName(point), func(t *testing.T) {
			f := testkit.NewAdmissionFixture(t)
			batch := f.Batch(1)
			admissionFault = func(got admissionFaultPoint) error {
				if got == point {
					return errors.New("injected interruption")
				}
				return nil
			}
			t.Cleanup(func() { admissionFault = nil })
			before := testkit.SnapshotTree(t, f.Store.Root)
			if _, err := AppendAdmission(context.Background(), f.Store, f.Tracker, f.Run, f.RunRevision, f.Team, f.TeamRevision, f.TrackerRevision, f.TaskRevisions, batch); err == nil {
				t.Fatal("injected interruption succeeded")
			}
			if point == faultBeforeCommit {
				testkit.RequireNoWrites(t, f.Store.Root, before)
			} else {
				assertInterruptionBoundary(t, f, point)
			}
			admissionFault = nil
			out, err := AppendAdmission(context.Background(), f.Store, f.Tracker, f.Run, f.RunRevision, f.Team, f.TeamRevision, f.TrackerRevision, f.TaskRevisions, batch)
			want := Duplicate
			if point == faultBeforeCommit {
				want = Created
			}
			if err != nil || out.Kind != want {
				t.Fatalf("retry = %#v, %v; want %s", out, err, want)
			}
			assertProjected(t, f, batch)
		})
	}
}

func TestAppendAdmissionRecoversPreviousCommitBeforeClassifyingNextRevision(t *testing.T) {
	f := testkit.NewAdmissionFixture(t)
	first := f.Batch(1)
	admissionFault = func(point admissionFaultPoint) error {
		if point == faultAfterCommit {
			return errors.New("stop after durable commit")
		}
		return nil
	}
	if _, err := AppendAdmission(context.Background(), f.Store, f.Tracker, f.Run, f.RunRevision, f.Team, f.TeamRevision, f.TrackerRevision, f.TaskRevisions, first); err == nil {
		t.Fatal("interruption succeeded")
	}
	admissionFault = nil
	t.Cleanup(func() { admissionFault = nil })

	second := f.Batch(1)
	second.BatchID = "batch-next"
	second.Sequence++
	second.Tasks = []core.TaskID{"T-0002"}
	second.Paths = []string{"src/task-0002"}
	second.Resources = []string{"resource:0002"}
	second.Fingerprint = fingerprint(second)
	out, err := AppendAdmission(context.Background(), f.Store, f.Tracker, f.Run, f.RunRevision+1, f.Team, f.TeamRevision+1, f.TrackerRevision, f.TaskRevisions, second)
	if err != nil || out.Kind != Created {
		t.Fatalf("next revision did not recover and create: %#v, %v", out, err)
	}
}

func TestConflictingRetryConvergesCommittedProjectionBeforeStale(t *testing.T) {
	f := testkit.NewAdmissionFixture(t)
	first := f.Batch(1)
	admissionFault = func(point admissionFaultPoint) error {
		if point == faultAfterCommit {
			return errors.New("stop after durable commit")
		}
		return nil
	}
	if _, err := AppendAdmission(context.Background(), f.Store, f.Tracker, f.Run, f.RunRevision, f.Team, f.TeamRevision, f.TrackerRevision, f.TaskRevisions, first); err == nil {
		t.Fatal("interruption succeeded")
	}
	admissionFault = nil
	t.Cleanup(func() { admissionFault = nil })
	conflict := f.Batch(2)
	out, err := AppendAdmission(context.Background(), f.Store, f.Tracker, f.Run, f.RunRevision, f.Team, f.TeamRevision, f.TrackerRevision, f.TaskRevisions, conflict)
	if err != nil || out.Kind != Stale {
		t.Fatalf("conflicting retry = %#v, %v; want converged stale", out, err)
	}
	assertProjected(t, f, first)
}

func TestProjectionPreflightRejectsDivergentCanonicalRecordsWithoutWrites(t *testing.T) {
	for _, target := range []string{"team", "run"} {
		t.Run(target, func(t *testing.T) {
			f := testkit.NewAdmissionFixture(t)
			batch := f.Batch(1)
			admissionFault = func(point admissionFaultPoint) error {
				if point == faultAfterCommit {
					return errors.New("stop after durable commit")
				}
				return nil
			}
			if _, err := AppendAdmission(context.Background(), f.Store, f.Tracker, f.Run, f.RunRevision, f.Team, f.TeamRevision, f.TrackerRevision, f.TaskRevisions, batch); err == nil {
				t.Fatal("interruption succeeded")
			}
			admissionFault = nil
			t.Cleanup(func() { admissionFault = nil })
			if target == "team" {
				var team run.TeamRecord
				if err := f.Store.ReadJSON(teamPath(f.Team), maxRecordBytes, &team); err != nil {
					t.Fatal(err)
				}
				team.Paths = []string{"src/divergent"}
				if _, err := f.Store.WriteJSON(teamPath(f.Team), team, maxRecordBytes); err != nil {
					t.Fatal(err)
				}
			} else {
				var current run.Run
				if err := f.Store.ReadJSON(runPath(f.Run), maxRecordBytes, &current); err != nil {
					t.Fatal(err)
				}
				current.State = core.Paused
				if _, err := f.Store.WriteJSON(runPath(f.Run), current, maxRecordBytes); err != nil {
					t.Fatal(err)
				}
			}
			before := testkit.SnapshotTree(t, f.Store.Root)
			_, err := AppendAdmission(context.Background(), f.Store, f.Tracker, f.Run, f.RunRevision, f.Team, f.TeamRevision, f.TrackerRevision, f.TaskRevisions, batch)
			if !errors.Is(err, core.ErrRevision) {
				t.Fatalf("divergent %s accepted: %v", target, err)
			}
			testkit.RequireNoWrites(t, f.Store.Root, before)
		})
	}
}

func TestCorruptCommitVariantsBlockWithoutProjection(t *testing.T) {
	mutations := map[string]func(*committedAdmission){
		"schema":          func(c *committedAdmission) { c.Schema = 2 },
		"batch-envelope":  func(c *committedAdmission) { c.Batch.Schema = 0 },
		"project-binding": func(c *committedAdmission) { c.Batch.Project = "other"; c.Batch.Fingerprint = fingerprint(c.Batch) },
		"unsafe-path": func(c *committedAdmission) {
			c.Batch.Paths = []string{"../escape"}
			c.Batch.Fingerprint = fingerprint(c.Batch)
		},
		"fingerprint":   func(c *committedAdmission) { c.Batch.Fingerprint = "sha256:bad" },
		"run-envelope":  func(c *committedAdmission) { c.BeforeRun.Schema = 0 },
		"team-envelope": func(c *committedAdmission) { c.BeforeTeam.Schema = 0 },
		"slot":          func(c *committedAdmission) { c.AfterRun.Teams[0].ID = "other-team" },
		"extra-nested-team": func(c *committedAdmission) {
			extra := cloneTeam(c.BeforeTeam)
			extra.ID = core.TeamID(string(c.BeforeRun.ID) + "-team-2")
			c.BeforeRun.Teams = append(c.BeforeRun.Teams, extra)
			c.AfterRun.Teams = append(c.AfterRun.Teams, extra)
		},
		"renamed-nested-team": func(c *committedAdmission) {
			renamed := core.TeamID(string(c.BeforeRun.ID) + "-team-9")
			c.BeforeTeam.ID = renamed
			c.AfterTeam.ID = renamed
			c.BeforeRun.Teams[0].ID = renamed
			c.AfterRun.Teams[0].ID = renamed
			c.Batch.Team = renamed
			c.Batch.Fingerprint = fingerprint(c.Batch)
		},
		"run-revision":  func(c *committedAdmission) { c.AfterRun.Revision++ },
		"team-revision": func(c *committedAdmission) { c.AfterTeam.Revision++ },
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			f := testkit.NewAdmissionFixture(t)
			batch := f.Batch(1)
			admissionFault = func(point admissionFaultPoint) error {
				if point == faultAfterCommit {
					return errors.New("stop after durable commit")
				}
				return nil
			}
			if _, err := AppendAdmission(context.Background(), f.Store, f.Tracker, f.Run, f.RunRevision, f.Team, f.TeamRevision, f.TrackerRevision, f.TaskRevisions, batch); err == nil {
				t.Fatal("interruption succeeded")
			}
			admissionFault = nil
			path := runCommitPath(f.Run, f.RunRevision)
			commit, found, err := readCommit(f.Store, path)
			if err != nil || !found {
				t.Fatalf("read durable commit: found=%v err=%v", found, err)
			}
			mutate(&commit)
			if _, err := f.Store.WriteJSON(path, commit, maxRecordBytes); err != nil {
				t.Fatal(err)
			}
			before := testkit.SnapshotTree(t, f.Store.Root)
			_, err = AppendAdmission(context.Background(), f.Store, f.Tracker, f.Run, f.RunRevision, f.Team, f.TeamRevision, f.TrackerRevision, f.TaskRevisions, batch)
			if !errors.Is(err, core.ErrRevision) {
				t.Fatalf("corrupt commit accepted: %v", err)
			}
			testkit.RequireNoWrites(t, f.Store.Root, before)
		})
	}
}

func TestAdmissionInputProjectBindingHasNoPartialWrite(t *testing.T) {
	f := testkit.NewAdmissionFixture(t)
	batch := f.Batch(1)
	batch.Project = "other"
	batch.Fingerprint = fingerprint(batch)
	before := testkit.SnapshotTree(t, f.Store.Root)
	_, err := AppendAdmission(context.Background(), f.Store, f.Tracker, f.Run, f.RunRevision, f.Team, f.TeamRevision, f.TrackerRevision, f.TaskRevisions, batch)
	if !errors.Is(err, core.ErrRevision) {
		t.Fatalf("project mismatch = %v", err)
	}
	testkit.RequireNoWrites(t, f.Store.Root, before)
}

func assertProjected(t *testing.T, f testkit.AdmissionFixture, batch run.AdmissionBatch) {
	t.Helper()
	var gotRun run.Run
	var gotTeam run.TeamRecord
	if err := f.Store.ReadJSON(runPath(f.Run), maxRecordBytes, &gotRun); err != nil {
		t.Fatal(err)
	}
	if err := f.Store.ReadJSON(teamPath(f.Team), maxRecordBytes, &gotTeam); err != nil {
		t.Fatal(err)
	}
	if gotRun.Revision != f.RunRevision+1 || gotTeam.Revision != f.TeamRevision+1 || !reflect.DeepEqual(gotTeam.Queue, batch.Tasks) {
		t.Fatalf("nonconvergent projection: run=%d team=%#v", gotRun.Revision, gotTeam)
	}
}

func assertInterruptionBoundary(t *testing.T, f testkit.AdmissionFixture, point admissionFaultPoint) {
	t.Helper()
	var gotRun run.Run
	var gotTeam run.TeamRecord
	if err := f.Store.ReadJSON(runPath(f.Run), maxRecordBytes, &gotRun); err != nil {
		t.Fatal(err)
	}
	if err := f.Store.ReadJSON(teamPath(f.Team), maxRecordBytes, &gotTeam); err != nil {
		t.Fatal(err)
	}
	wantRun, wantTeam := f.RunRevision, f.TeamRevision
	if point == faultAfterRunProjection || point == faultAfterTeamProjection {
		wantRun++
	}
	if point == faultAfterTeamProjection {
		wantTeam++
	}
	if gotRun.Revision != wantRun || gotTeam.Revision != wantTeam {
		t.Fatalf("interruption boundary %s left run=%d team=%d, want %d/%d", faultName(point), gotRun.Revision, gotTeam.Revision, wantRun, wantTeam)
	}
}

func faultName(point admissionFaultPoint) string {
	return [...]string{"before-commit", "after-commit", "after-run", "after-team"}[point]
}
