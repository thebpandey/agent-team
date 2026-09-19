package run

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/thebpandey/agent-team/vnext/internal/core"
	"github.com/thebpandey/agent-team/vnext/internal/store"
	"github.com/thebpandey/agent-team/vnext/internal/tracker"
)

// These literals are compile-time API contracts from the Phase 1 plan. Keep
// them here so changing the public wire shape cannot silently strand admission.
var _ = TeamRecord{Queue: []core.TaskID{"T-1"}, QueueFingerprint: "sha256:0000000000000000000000000000000000000000000000000000000000000000", State: core.Working, Paths: []string{"src"}, Resources: []string{"db:test"}}
var _ = Run{Root: "project-root"}
var _ = AdmissionBatch{RecordEnvelope: core.RecordEnvelope{Schema: 1}, BatchID: "B-1", Fingerprint: "sha256:0000000000000000000000000000000000000000000000000000000000000000", Tasks: []core.Task{oneOffTask("T-1", "task", []string{"src"}, nil)}, Team: "team-1", Sequence: 1, TrackerRevision: 1, Paths: []string{"src"}, Resources: []string{"db:test"}}

func TestAdmissionBatchConflictContract(t *testing.T) {
	a := AdmissionBatch{Tasks: []core.Task{oneOffTask("T-1", "task", []string{"src/api"}, []string{"db:test"})}}
	b := AdmissionBatch{Tasks: []core.Task{oneOffTask("T-2", "task", []string{"src/api/handlers"}, []string{"DB:TEST"})}}
	if !ValidateConflict(a, b) {
		t.Fatal("ancestor and resource conflict was not detected")
	}
}

func TestCreateOneOffCanonicalReadOnlyAndConflicts(t *testing.T) {
	ctx := context.Background()
	first := []core.Task{
		oneOffTask("AUD-2", "inspect billing", []string{"pkg/billing"}, []string{"DB:Read"}),
		oneOffTask("AUD-1", "inspect auth", []string{"pkg/auth"}, []string{"cache:read"}),
	}
	second := []core.Task{first[1], first[0]}

	one, err := CreateOneOff(ctx, "project-a", OneOffAudit, "audit the project", first)
	if err != nil {
		t.Fatal(err)
	}
	two, err := CreateOneOff(ctx, "project-a", OneOffAudit, "audit the project", second)
	if err != nil {
		t.Fatal(err)
	}
	if one.ID != two.ID || one.ManifestDigest != two.ManifestDigest || !reflect.DeepEqual(one.Tasks, two.Tasks) {
		t.Fatalf("non-deterministic manifest:\n%#v\n%#v", one, two)
	}
	if one.TrackerKind != "none" || one.OneOffKind != OneOffAudit || len(one.Tasks) != 2 || one.Tasks[0].WritablePaths != nil {
		t.Fatalf("unexpected audit manifest: %#v", one)
	}
	if one.ID == "" || one.RecordEnvelope.Schema != 1 || one.Revision != 1 {
		t.Fatalf("missing immutable envelope: %#v", one)
	}
	readonly, err := CreateOneOff(ctx, "project-a", OneOffAudit, "audit", []core.Task{oneOffTask("AUD-3", "input writes are stripped", []string{"src"}, nil)})
	if err != nil || readonly.Tasks[0].WritablePaths != nil {
		t.Fatalf("audit retained writable authority: %#v err=%v", readonly, err)
	}

	a := oneOffTask("F-1", "change one", []string{"src/api"}, []string{"db:test"})
	b := oneOffTask("F-2", "change two", []string{"src/api/handlers"}, []string{"DB:TEST"})
	if !ValidateConflict(AdmissionBatch{Tasks: []core.Task{a}}, AdmissionBatch{Tasks: []core.Task{b}}) {
		t.Fatal("ancestor path/resource overlap was admitted")
	}
	serial, err := CreateOneOff(ctx, "project-a", OneOffFeature, "feature", []core.Task{a, b})
	if err != nil || len(serial.Teams) != 1 || len(serial.Teams[0].Queue) != 2 {
		t.Fatalf("overlap did not serialise: %#v err=%v", serial.Teams, err)
	}
	bad := oneOffTask("F-3", "bad root", []string{"../outside"}, nil)
	if _, err := CreateOneOff(ctx, "project-a", OneOffFeature, "feature", []core.Task{bad}); !errors.Is(err, core.ErrPath) {
		t.Fatalf("unsafe path accepted: %v", err)
	}
}

func TestCreatePlanSnapshotsOnlySelectedTracker(t *testing.T) {
	tr := trackerStub{page: core.TrackerPage{TrackerRevision: 41, TotalNonArchived: 2, Tasks: []core.Task{
		oneOffTask("P-2", "second", []string{"src/two"}, []string{"db:two"}),
		oneOffTask("P-1", "first", []string{"src/one"}, []string{"db:one"}),
	}}}
	plan, err := CreatePlan(context.Background(), "project-a", tr)
	if err != nil || plan.Mode != "plan" || plan.Root != "project-a" || plan.TrackerKind != "tasks-md" || plan.TrackerRevision != 41 || plan.TrackerSnapshotDigest == "" || plan.SpecRevision == "" || len(plan.Tasks) != 2 || plan.Tasks[0].Objective != "" {
		t.Fatalf("plan=%#v err=%v", plan, err)
	}
	if plan.ID == "" || plan.ManifestDigest == "" {
		t.Fatal("plan was not canonicalized")
	}
}

func TestCreatePlanRejectsTrackerWithoutAuthorityMetadata(t *testing.T) {
	bare := noMetadataTracker{Tracker: trackerStub{page: core.TrackerPage{TrackerRevision: 1}}}
	if _, err := CreatePlan(context.Background(), "project-a", bare); !errors.Is(err, core.ErrSettings) {
		t.Fatalf("unknown tracker authority accepted: %v", err)
	}
}

func TestRepositoriesCASAndImmutableOneOff(t *testing.T) {
	ctx := context.Background()
	repos := NewRepositories(store.New(t.TempDir(), core.StorageLimits{CanonicalBytes: 16 << 20}))
	one, err := CreateOneOff(ctx, "p", OneOffReview, "review", []core.Task{oneOffTask("R-1", "review", nil, nil)})
	if err != nil {
		t.Fatal(err)
	}
	saved, err := repos.Runs.Initialize(ctx, one)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repos.Runs.Initialize(ctx, one); err != nil {
		t.Fatalf("exact initialize not idempotent: %v", err)
	}
	changed := one
	changed.Objective = "different"
	if _, err := repos.Runs.Initialize(ctx, changed); !errors.Is(err, core.ErrRevision) {
		t.Fatalf("conflicting initialize accepted: %v", err)
	}
	if _, err := repos.Runs.CompareAndSwap(ctx, saved.ID, saved.Revision-1, saved); !errors.Is(err, core.ErrRevision) {
		t.Fatalf("stale run CAS accepted: %v", err)
	}
	changed = saved
	changed.Objective = "different"
	if _, err := repos.Runs.CompareAndSwap(ctx, saved.ID, saved.Revision, changed); !errors.Is(err, core.ErrRevision) {
		t.Fatalf("one-off immutable field changed: %v", err)
	}
	updated, err := repos.Runs.CompareAndSwap(ctx, saved.ID, saved.Revision, saved)
	if err != nil || updated.Revision != saved.Revision+1 {
		t.Fatalf("run CAS=%#v err=%v", updated, err)
	}
	if got, err := repos.Runs.Read(ctx, saved.ID); err != nil || got.Revision != updated.Revision {
		t.Fatalf("last good record=%#v err=%v", got, err)
	}

	team := TeamRecord{RecordEnvelope: core.RecordEnvelope{Schema: 1, Project: "p", RunID: saved.ID, WrittenAt: saved.WrittenAt, Revision: 1}, ID: "team-1", State: core.Idle}
	teamSaved, err := repos.Teams.Initialize(ctx, team)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repos.Teams.CompareAndSwap(ctx, team.ID, teamSaved.Revision, teamSaved); err != nil {
		t.Fatal(err)
	}
}

func TestRunRepositoryCASWithReservedEnvelope(t *testing.T) {
	ctx := context.Background()
	repos := NewRepositories(store.New(t.TempDir(), core.StorageLimits{CanonicalBytes: 16 << 20}))
	reserved := Run{RecordEnvelope: core.RecordEnvelope{Schema: 1, Project: "p", RunID: "R-1", WrittenAt: "2026-09-19T00:00:00Z", Revision: 1}, ID: "R-1"}
	saved, err := repos.Runs.Initialize(ctx, reserved)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repos.Runs.CompareAndSwap(ctx, saved.ID, 0, saved); !errors.Is(err, core.ErrRevision) {
		t.Fatal(err)
	}
	if updated, err := repos.Runs.CompareAndSwap(ctx, saved.ID, saved.Revision, saved); err != nil || updated.Revision != 2 {
		t.Fatalf("CAS=%#v err=%v", updated, err)
	}
}

func TestOneOffTeamAndAdmissionBounds(t *testing.T) {
	tasks := make([]core.Task, 9)
	for i := range tasks {
		tasks[i] = oneOffTask(core.TaskID("F-"+string(rune('A'+i))), "feature", []string{"src/" + string(rune('a'+i))}, nil)
	}
	run, err := CreateOneOff(context.Background(), "p", OneOffFeature, "feature", tasks)
	if err != nil || len(run.Teams) != 2 {
		t.Fatalf("teams=%#v err=%v", run.Teams, err)
	}
	for _, team := range run.Teams {
		if len(team.Queue) == 0 || len(team.Queue) > 8 {
			t.Fatalf("unbounded team: %#v", team)
		}
	}
	if _, err := CreateOneOff(context.Background(), "p", OneOffFeature, "feature", append(tasks, oneOffTask("F-Z", "feature", []string{"src/z"}, nil), oneOffTask("F-Y", "feature", []string{"src/y"}, nil), oneOffTask("F-X", "feature", []string{"src/x"}, nil), oneOffTask("F-W", "feature", []string{"src/w"}, nil), oneOffTask("F-V", "feature", []string{"src/v"}, nil), oneOffTask("F-U", "feature", []string{"src/u"}, nil), oneOffTask("F-T", "feature", []string{"src/t"}, nil), oneOffTask("F-S", "feature", []string{"src/s"}, nil))); !errors.Is(err, core.ErrBatch) {
		t.Fatalf("more than two bounded teams accepted: %v", err)
	}
	batch := AdmissionBatch{RecordEnvelope: core.RecordEnvelope{Schema: 1, Project: "p", RunID: "R-1", WrittenAt: "2026-09-19T00:00:00Z", Revision: 1}, BatchID: "batch-1", Fingerprint: "sha256:0000000000000000000000000000000000000000000000000000000000000000", Team: "team-1", Tasks: []core.Task{oneOffTask("A", "task", []string{"src/a"}, nil)}}
	if err := validateAdmission(batch); err != nil {
		t.Fatal(err)
	}
	batch.Tasks = make([]core.Task, 9)
	if err := validateAdmission(batch); !errors.Is(err, core.ErrBatch) {
		t.Fatalf("unbounded batch accepted: %v", err)
	}
}

func TestOneOffTeamsAndAdmissionsCarryCanonicalScopes(t *testing.T) {
	run, err := CreateOneOff(context.Background(), "p", Feature, "feature", []core.Task{oneOffTask("F-1", "feature", []string{"SRC/api"}, []string{"DB:TEST"})})
	if err != nil || len(run.Teams) != 1 || !reflect.DeepEqual(run.Teams[0].Paths, []string{"src/api"}) || !reflect.DeepEqual(run.Teams[0].Resources, []string{"db:test"}) {
		t.Fatalf("team scopes=%#v err=%v", run.Teams, err)
	}
	batch := AdmissionBatch{RecordEnvelope: core.RecordEnvelope{Schema: 1, Project: "p", RunID: run.ID, WrittenAt: run.WrittenAt, Revision: 1}, BatchID: "batch-1", Fingerprint: run.ManifestDigest, Team: run.Teams[0].ID, Tasks: []core.Task{oneOffTask("F-1", "feature", []string{"src/api"}, []string{"db:test"})}, Paths: []string{"src/api"}, Resources: []string{"db:test"}}
	if err := validateAdmission(batch); err != nil {
		t.Fatal(err)
	}
	batch.Schema = 2
	if err := validateAdmission(batch); !errors.Is(err, core.ErrRevision) {
		t.Fatalf("malformed admission envelope accepted: %v", err)
	}
}

func TestOneOffRejectsAmbiguousScopesAndNeverMutatesInputs(t *testing.T) {
	for _, scope := range []string{"src/CON", "src/file.txt:zone", "src/trailing. ", "src/trailing ", "src/COM1"} {
		if _, err := CreateOneOff(context.Background(), "p", Feature, "feature", []core.Task{oneOffTask("F-1", "feature", []string{scope}, nil)}); !errors.Is(err, core.ErrPath) {
			t.Fatalf("unsafe scope %q accepted: %v", scope, err)
		}
	}
	input := oneOffTask("A-1", "audit", []string{"src"}, nil)
	before := append([]string(nil), input.WritablePaths...)
	if _, err := CreateOneOff(context.Background(), "p", Audit, "audit", []core.Task{input}); err != nil || !reflect.DeepEqual(input.WritablePaths, before) {
		t.Fatalf("caller input mutated or audit failed: paths=%#v err=%v", input.WritablePaths, err)
	}
	missing := oneOffTask("F-1", "feature", []string{"src"}, nil)
	missing.Criteria = nil
	if _, err := CreateOneOff(context.Background(), "p", Feature, "feature", []core.Task{missing}); !errors.Is(err, core.ErrBatch) || missing.WritablePaths[0] != "src" {
		t.Fatalf("criteria failure did not preserve input: %#v err=%v", missing, err)
	}
	over := oneOffTask("F-2", strings.Repeat("x", 256<<10), []string{"src"}, nil)
	if _, err := CreateOneOff(context.Background(), "p", Feature, "feature", []core.Task{over}); !errors.Is(err, core.ErrLimit) {
		t.Fatalf("aggregate limit was not enforced: %v", err)
	}
}

func oneOffTask(id core.TaskID, objective string, writable, resources []string) core.Task {
	return core.Task{ID: id, Objective: objective, State: core.Ready, WritablePaths: writable, Resources: resources, Criteria: []string{"criterion"}}
}

type trackerStub struct{ page core.TrackerPage }

type noMetadataTracker struct{ tracker.Tracker }

func (s trackerStub) AuthorityMetadata() tracker.AuthorityMetadata {
	return tracker.AuthorityMetadata{Kind: "tasks-md", Ref: "project-a/TASKS.md"}
}

func (s trackerStub) Page(context.Context, string, int) (core.TrackerPage, error) { return s.page, nil }
func (s trackerStub) Get(context.Context, core.TaskID, uint64) (core.Task, error) {
	return core.Task{}, errors.New("not implemented")
}
func (s trackerStub) Refresh(context.Context, uint64) (core.TrackerPage, error) { return s.page, nil }
func (s trackerStub) Create(context.Context, core.Task, uint64) (core.Task, error) {
	return core.Task{}, errors.New("not implemented")
}
func (s trackerStub) Archive(context.Context, core.TaskID, string, uint64) error {
	return errors.New("not implemented")
}
