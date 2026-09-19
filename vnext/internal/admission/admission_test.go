package admission_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/thebpandey/agent-team/vnext/internal/admission"
	"github.com/thebpandey/agent-team/vnext/internal/core"
	"github.com/thebpandey/agent-team/vnext/internal/testkit"
)

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
