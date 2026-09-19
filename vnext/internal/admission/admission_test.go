package admission_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/thebpandey/agent-team/vnext/internal/admission"
	"github.com/thebpandey/agent-team/vnext/internal/core"
	"github.com/thebpandey/agent-team/vnext/internal/run"
	"github.com/thebpandey/agent-team/vnext/internal/testkit"
)

func TestAdmissionCorruptCommittedRecordBlocksWithoutMutation(t *testing.T) {
	f := testkit.NewAdmissionFixture(t)
	path := ".agent-team/admissions/by-run/" + string(f.Run) + "/1.json"
	if _, err := f.Store.CreateJSON(path, map[string]any{"schema": 99}, 1024); err != nil {
		t.Fatal(err)
	}
	before := testkit.SnapshotTree(t, f.Store.Root)
	_, err := admission.AppendAdmission(context.Background(), f.Store, f.Tracker, f.Run, f.RunRevision, f.Team, f.TeamRevision, f.TrackerRevision, f.TaskRevisions, f.Batch(1))
	if !errors.Is(err, core.ErrRevision) {
		t.Fatalf("corrupt commit accepted: %v", err)
	}
	testkit.RequireNoWrites(t, f.Store.Root, before)
}

func TestAdmissionRejectsEnvelopeAndTaskRevisionWithoutWrites(t *testing.T) {
	for _, mutate := range []func(*testkit.AdmissionFixture, *run.AdmissionBatch){
		func(_ *testkit.AdmissionFixture, b *run.AdmissionBatch) { b.Schema = 2 },
		func(f *testkit.AdmissionFixture, _ *run.AdmissionBatch) { f.TaskRevisions["T-0001"]++ },
	} {
		f := testkit.NewAdmissionFixture(t)
		batch := f.Batch(1)
		mutate(&f, &batch)
		before := testkit.SnapshotTree(t, f.Store.Root)
		_, err := admission.AppendAdmission(context.Background(), f.Store, f.Tracker, f.Run, f.RunRevision, f.Team, f.TeamRevision, f.TrackerRevision, f.TaskRevisions, batch)
		if err == nil {
			t.Fatal("invalid admission accepted")
		}
		testkit.RequireNoWrites(t, f.Store.Root, before)
	}
}

func TestAdmissionOutcomes(t *testing.T) {
	f := testkit.NewAdmissionFixture(t)
	for _, n := range []int{0, 1, 8, 9} {
		batch := f.Batch(n)
		out, err := admission.AppendAdmission(context.Background(), f.Store, f.Tracker, f.Run, f.RunRevision, f.Team, f.TeamRevision, f.TrackerRevision, f.TaskRevisions, batch)
		if n == 1 && (err != nil || out.Kind != admission.Created) {
			t.Fatal(n, err, out)
		}
		if n == 0 || n == 9 {
			if err == nil || !errors.Is(err, core.ErrBatch) {
				t.Fatal(n, err)
			}
		}
	}
	duplicate, err := admission.AppendAdmission(context.Background(), f.Store, f.Tracker, f.Run, f.RunRevision, f.Team, f.TeamRevision, f.TrackerRevision, f.TaskRevisions, f.Batch(1))
	if err != nil || duplicate.Kind != admission.Duplicate {
		t.Fatal(err, duplicate)
	}
	f.RunRevision++
	stale, err := admission.AppendAdmission(context.Background(), f.Store, f.Tracker, f.Run, f.RunRevision, f.Team, f.TeamRevision, f.TrackerRevision, f.TaskRevisions, f.Batch(1))
	if err != nil || stale.Kind != admission.Stale {
		t.Fatal(err, stale)
	}
}

func TestAppendAdmissionConcurrentIdenticalIsCreatedThenDuplicate(t *testing.T) {
	f := testkit.NewAdmissionFixture(t)
	batch := f.Batch(1)
	results := make(chan admission.AdmissionOutcome, 2)
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			out, err := admission.AppendAdmission(context.Background(), f.Store, f.Tracker, f.Run, f.RunRevision, f.Team, f.TeamRevision, f.TrackerRevision, f.TaskRevisions, batch)
			results <- out
			errs <- err
		}()
	}
	wg.Wait()
	close(results)
	close(errs)
	created, duplicate := 0, 0
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	for out := range results {
		switch out.Kind {
		case admission.Created:
			created++
		case admission.Duplicate:
			duplicate++
		default:
			t.Fatalf("unexpected outcome: %#v", out)
		}
	}
	if created != 1 || duplicate != 1 {
		t.Fatalf("created=%d duplicate=%d", created, duplicate)
	}
}

func TestAdmissionCapacityWarningAndLimit(t *testing.T) {
	for _, n := range []int{900, 999, 1000, 1001} {
		t.Run("capacity", func(t *testing.T) {
			f := testkit.NewAdmissionFixtureWithCapacity(t, n)
			out, err := admission.AppendAdmission(context.Background(), f.Store, f.Tracker, f.Run, f.RunRevision, f.Team, f.TeamRevision, f.TrackerRevision, f.TaskRevisions, f.Batch(1))
			if n == 1001 {
				if !errors.Is(err, core.ErrCapacity) {
					t.Fatalf("capacity %d: %v", n, err)
				}
				return
			}
			wantWarning := fmt.Sprintf("tracker has %d of %d non-archived tasks", n, 1000)
			if err != nil || out.Kind != admission.Created || out.Warning != wantWarning {
				t.Fatalf("capacity %d: %#v %v", n, out, err)
			}
		})
	}
}
