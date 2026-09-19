package run

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
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
var _ = AdmissionBatch{RecordEnvelope: core.RecordEnvelope{Schema: 1}, BatchID: "B-1", Fingerprint: "sha256:0000000000000000000000000000000000000000000000000000000000000000", Tasks: []core.TaskID{"T-1"}, Team: "team-1", Sequence: 1, TrackerRevision: 1, Paths: []string{"src"}, Resources: []string{"db:test"}}

func TestAdmissionBatchConflictContract(t *testing.T) {
	a := AdmissionBatch{Tasks: []core.TaskID{"T-1"}, Paths: []string{"src/api"}, Resources: []string{"db:test"}}
	b := AdmissionBatch{Tasks: []core.TaskID{"T-2"}, Paths: []string{"src/api/handlers"}, Resources: []string{"DB:TEST"}}
	if !ValidateConflict(a, b) {
		t.Fatal("ancestor and resource conflict was not detected")
	}
}

func TestCreateOneOffCanonicalReadOnlyAndConflicts(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	first := []core.Task{
		oneOffTask("AUD-2", "inspect billing", []string{"pkg/billing"}, []string{"DB:Read"}),
		oneOffTask("AUD-1", "inspect auth", []string{"pkg/auth"}, []string{"cache:read"}),
	}
	second := []core.Task{first[1], first[0]}

	one, err := CreateOneOff(ctx, root, OneOffAudit, "audit the project", first)
	if err != nil {
		t.Fatal(err)
	}
	two, err := CreateOneOff(ctx, root, OneOffAudit, "audit the project", second)
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
	readonly, err := CreateOneOff(ctx, root, OneOffAudit, "audit", []core.Task{oneOffTask("AUD-3", "input writes are stripped", []string{"src"}, nil)})
	if err != nil || readonly.Tasks[0].WritablePaths != nil {
		t.Fatalf("audit retained writable authority: %#v err=%v", readonly, err)
	}

	a := oneOffTask("F-1", "change one", []string{"src/api"}, []string{"db:test"})
	b := oneOffTask("F-2", "change two", []string{"src/api/handlers"}, []string{"DB:TEST"})
	if !ValidateConflict(AdmissionBatch{Tasks: []core.TaskID{a.ID}, Paths: a.WritablePaths, Resources: a.Resources}, AdmissionBatch{Tasks: []core.TaskID{b.ID}, Paths: b.WritablePaths, Resources: b.Resources}) {
		t.Fatal("ancestor path/resource overlap was admitted")
	}
	serial, err := CreateOneOff(ctx, root, OneOffFeature, "feature", []core.Task{a, b})
	if err != nil || len(serial.Teams) != 1 || len(serial.Teams[0].Queue) != 2 {
		t.Fatalf("overlap did not serialise: %#v err=%v", serial.Teams, err)
	}
	bad := oneOffTask("F-3", "bad root", []string{"../outside"}, nil)
	if _, err := CreateOneOff(ctx, root, OneOffFeature, "feature", []core.Task{bad}); !errors.Is(err, core.ErrPath) {
		t.Fatalf("unsafe path accepted: %v", err)
	}
}

func TestCreatePlanSnapshotsOnlySelectedTracker(t *testing.T) {
	root := t.TempDir()
	tr := trackerStub{page: core.TrackerPage{TrackerRevision: 41, TotalNonArchived: 2, Tasks: []core.Task{
		oneOffTask("P-2", "second", []string{"src/two"}, []string{"db:two"}),
		oneOffTask("P-1", "first", []string{"src/one"}, []string{"db:one"}),
	}}}
	tr.ref = filepath.Join(root, "TASKS.md")
	if err := os.WriteFile(tr.ref, []byte("# tasks\n"), 0600); err != nil {
		t.Fatal(err)
	}
	plan, err := CreatePlan(context.Background(), root, tr)
	if err != nil || plan.Mode != "plan" || plan.Root != root || plan.TrackerKind != "tasks-md" || plan.TrackerRevision != 41 || plan.TrackerSnapshotDigest == "" || plan.SpecRevision == "" || len(plan.Tasks) != 2 || plan.Tasks[0].Objective != "" {
		t.Fatalf("plan=%#v err=%v", plan, err)
	}
	if plan.ID == "" || plan.ManifestDigest == "" {
		t.Fatal("plan was not canonicalized")
	}
}

func TestCreatePlanRejectsTrackerWithoutAuthorityMetadata(t *testing.T) {
	bare := noMetadataTracker{Tracker: trackerStub{page: core.TrackerPage{TrackerRevision: 1}}}
	if _, err := CreatePlan(context.Background(), t.TempDir(), bare); !errors.Is(err, core.ErrSettings) {
		t.Fatalf("unknown tracker authority accepted: %v", err)
	}
}

func TestRepositoriesCASAndImmutableOneOff(t *testing.T) {
	ctx := context.Background()
	repos := NewRepositories(store.New(t.TempDir(), core.StorageLimits{CanonicalBytes: 16 << 20}))
	root := t.TempDir()
	one, err := CreateOneOff(ctx, root, OneOffReview, "review", []core.Task{oneOffTask("R-1", "review", nil, nil)})
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

	team := saved.Teams[0]
	teamSaved, err := repos.Teams.Initialize(ctx, team)
	if err != nil {
		t.Fatal(err)
	}
	parent := updated
	parent.Teams = append([]TeamRecord(nil), updated.Teams...)
	parent.Teams[0].Revision = teamSaved.Revision + 1
	if _, err := repos.Runs.CompareAndSwap(ctx, updated.ID, updated.Revision, parent); err != nil {
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
	root := t.TempDir()
	tasks := make([]core.Task, 9)
	for i := range tasks {
		tasks[i] = oneOffTask(core.TaskID("F-"+string(rune('A'+i))), "feature", []string{"src/" + string(rune('a'+i))}, nil)
	}
	run, err := CreateOneOff(context.Background(), root, OneOffFeature, "feature", tasks)
	if err != nil || len(run.Teams) != 2 {
		t.Fatalf("teams=%#v err=%v", run.Teams, err)
	}
	for _, team := range run.Teams {
		if len(team.Queue) == 0 || len(team.Queue) > 8 {
			t.Fatalf("unbounded team: %#v", team)
		}
	}
	if _, err := CreateOneOff(context.Background(), root, OneOffFeature, "feature", append(tasks, oneOffTask("F-Z", "feature", []string{"src/z"}, nil), oneOffTask("F-Y", "feature", []string{"src/y"}, nil), oneOffTask("F-X", "feature", []string{"src/x"}, nil), oneOffTask("F-W", "feature", []string{"src/w"}, nil), oneOffTask("F-V", "feature", []string{"src/v"}, nil), oneOffTask("F-U", "feature", []string{"src/u"}, nil), oneOffTask("F-T", "feature", []string{"src/t"}, nil), oneOffTask("F-S", "feature", []string{"src/s"}, nil))); !errors.Is(err, core.ErrBatch) {
		t.Fatalf("more than two bounded teams accepted: %v", err)
	}
	batch := AdmissionBatch{RecordEnvelope: core.RecordEnvelope{Schema: 1, Project: root, RunID: "R-1", WrittenAt: "2026-09-19T00:00:00Z", Revision: 1}, BatchID: "batch-1", Team: "team-1", Tasks: []core.TaskID{"A"}}
	batch.Fingerprint = admissionFingerprint(batch)
	if err := validateAdmission(batch); err != nil {
		t.Fatal(err)
	}
	batch.Tasks = make([]core.TaskID, 9)
	if err := validateAdmission(batch); !errors.Is(err, core.ErrBatch) {
		t.Fatalf("unbounded batch accepted: %v", err)
	}
}

func TestOneOffTeamsAndAdmissionsCarryCanonicalScopes(t *testing.T) {
	root := t.TempDir()
	run, err := CreateOneOff(context.Background(), root, Feature, "feature", []core.Task{oneOffTask("F-1", "feature", []string{"SRC/api"}, []string{"DB:TEST"})})
	if err != nil || len(run.Teams) != 1 || !reflect.DeepEqual(run.Teams[0].Paths, []string{"src/api"}) || !reflect.DeepEqual(run.Teams[0].Resources, []string{"db:test"}) {
		t.Fatalf("team scopes=%#v err=%v", run.Teams, err)
	}
	batch := AdmissionBatch{RecordEnvelope: core.RecordEnvelope{Schema: 1, Project: run.Project, RunID: run.ID, WrittenAt: run.WrittenAt, Revision: 1}, BatchID: "batch-1", Team: run.Teams[0].ID, Tasks: []core.TaskID{"F-1"}, Paths: []string{"src/api"}, Resources: []string{"db:test"}}
	batch.Fingerprint = admissionFingerprint(batch)
	if err := validateAdmission(batch); err != nil {
		t.Fatal(err)
	}
	batch.Schema = 2
	if err := validateAdmission(batch); !errors.Is(err, core.ErrRevision) {
		t.Fatalf("malformed admission envelope accepted: %v", err)
	}
}

func TestAdmissionFingerprintBindsEveryImmutableField(t *testing.T) {
	batch := AdmissionBatch{RecordEnvelope: core.RecordEnvelope{Schema: 1, Project: "project", RunID: "R-1", WrittenAt: "2026-09-19T00:00:00Z", Revision: 1}, BatchID: "B-1", Team: "T-1", Sequence: 1, TrackerRevision: 7, Tasks: []core.TaskID{"A-1", "A-2"}, Paths: []string{"src/api"}, Resources: []string{"db:test"}}
	batch.Fingerprint = admissionFingerprint(batch)
	if err := validateAdmission(batch); err != nil {
		t.Fatal(err)
	}
	mutations := []func(*AdmissionBatch){
		func(v *AdmissionBatch) { v.Project = "other" }, func(v *AdmissionBatch) { v.RunID = "R-2" }, func(v *AdmissionBatch) { v.WrittenAt = "2026-09-20T00:00:00Z" }, func(v *AdmissionBatch) { v.BatchID = "B-2" }, func(v *AdmissionBatch) { v.Team = "T-2" }, func(v *AdmissionBatch) { v.Sequence++ }, func(v *AdmissionBatch) { v.TrackerRevision++ }, func(v *AdmissionBatch) { v.Tasks[1] = "A-3" }, func(v *AdmissionBatch) { v.Paths[0] = "src/other" }, func(v *AdmissionBatch) { v.Resources[0] = "cache:test" },
	}
	for _, mutate := range mutations {
		changed := batch
		changed.Tasks = append([]core.TaskID(nil), batch.Tasks...)
		changed.Paths = append([]string(nil), batch.Paths...)
		changed.Resources = append([]string(nil), batch.Resources...)
		mutate(&changed)
		if err := validateAdmission(changed); !errors.Is(err, core.ErrRevision) {
			t.Fatalf("fingerprint mutation accepted: %#v err=%v", changed, err)
		}
	}
}

func TestPlanAllowsMoreThanTwoRetainedTeams(t *testing.T) {
	root := t.TempDir()
	ref := filepath.Join(root, "TASKS.md")
	if err := os.WriteFile(ref, []byte("# tasks\n"), 0600); err != nil {
		t.Fatal(err)
	}
	plan, err := CreatePlan(context.Background(), root, trackerStub{ref: ref, page: core.TrackerPage{TrackerRevision: 2, TotalNonArchived: 3, Tasks: []core.Task{oneOffTask("A-1", "one", []string{"src/a"}, nil), oneOffTask("A-2", "two", []string{"src/b"}, nil), oneOffTask("A-3", "three", []string{"src/c"}, nil)}}})
	if err != nil {
		t.Fatal(err)
	}
	plan.Teams = make([]TeamRecord, 3)
	for i := range plan.Teams {
		plan.Teams[i] = TeamRecord{RecordEnvelope: core.RecordEnvelope{Schema: 1, Project: plan.Project, RunID: plan.ID, WrittenAt: plan.WrittenAt, Revision: 1}, ID: canonicalTeamID(plan.ID, i+1), Queue: []core.TaskID{plan.Tasks[i].ID}, State: core.Working}
		plan.Teams[i].QueueFingerprint = queueFingerprint(plan.Teams[i].Queue)
	}
	plan.ManifestDigest, err = manifestDigest(plan)
	if err != nil {
		t.Fatal(err)
	}
	plan.ID = core.RunID("run-" + strings.TrimPrefix(plan.ManifestDigest, "sha256:")[:24])
	plan.RunID = plan.ID
	for i := range plan.Tasks {
		plan.Tasks[i].RunID = plan.ID
	}
	for i := range plan.Teams {
		plan.Teams[i].RunID = plan.ID
	}
	if err := validateRun(plan); err != nil {
		t.Fatalf("plan team capacity was capped here: %v", err)
	}
}

func TestPlanCASKeepsManifestIdentityWhileTeamsChange(t *testing.T) {
	ctx, root := context.Background(), t.TempDir()
	ref := filepath.Join(root, "TASKS.md")
	if err := os.WriteFile(ref, []byte("# tasks\n"), 0600); err != nil {
		t.Fatal(err)
	}
	plan, err := CreatePlan(ctx, root, trackerStub{ref: ref, page: core.TrackerPage{TrackerRevision: 2, TotalNonArchived: 3, Tasks: []core.Task{oneOffTask("A-1", "one", []string{"src/a"}, nil), oneOffTask("A-2", "two", []string{"src/b"}, nil), oneOffTask("A-3", "three", []string{"src/c"}, nil)}}})
	if err != nil {
		t.Fatal(err)
	}
	repos := NewRepositories(store.New(t.TempDir(), core.StorageLimits{CanonicalBytes: 16 << 20}))
	saved, err := repos.Runs.Initialize(ctx, plan)
	if err != nil {
		t.Fatal(err)
	}
	admitted := saved
	admitted.Teams = []TeamRecord{teamSlot(saved, 1, "A-1"), teamSlot(saved, 2, "A-2"), teamSlot(saved, 3, "A-3")}
	updated, err := repos.Runs.CompareAndSwap(ctx, saved.ID, saved.Revision, admitted)
	if err != nil || updated.ID != saved.ID || updated.ManifestDigest != saved.ManifestDigest || len(updated.Teams) != 3 {
		t.Fatalf("team admission=%#v err=%v", updated, err)
	}
	updated.Teams[2].Queue = nil
	updated.Teams[2].QueueFingerprint = ""
	updated.Teams[2].State = core.Idle
	updated.Teams[0].Queue = []core.TaskID{"A-3"}
	updated.Teams[0].QueueFingerprint = queueFingerprint(updated.Teams[0].Queue)
	updated.Teams[0].State = core.Paused
	reused, err := repos.Runs.CompareAndSwap(ctx, updated.ID, updated.Revision, updated)
	if err != nil || reused.ID != saved.ID || reused.ManifestDigest != saved.ManifestDigest || reused.Teams[0].State != core.Paused {
		t.Fatalf("team reuse=%#v err=%v", reused, err)
	}
	immutable := reused
	immutable.Objective = "changed"
	if _, err := repos.Runs.CompareAndSwap(ctx, reused.ID, reused.Revision, immutable); !errors.Is(err, core.ErrRevision) {
		t.Fatalf("immutable mutation accepted: %v", err)
	}
	for _, mutate := range []func(*Run){
		func(v *Run) { v.Teams[1].ID = v.Teams[0].ID }, func(v *Run) { v.Teams[0], v.Teams[1] = v.Teams[1], v.Teams[0] }, func(v *Run) { v.Teams[2].ID = "other-team" },
	} {
		changed := reused
		changed.Teams = append([]TeamRecord(nil), reused.Teams...)
		mutate(&changed)
		if _, err := repos.Runs.CompareAndSwap(ctx, reused.ID, reused.Revision, changed); !errors.Is(err, core.ErrRevision) {
			t.Fatalf("unstable team slots accepted: %#v err=%v", changed.Teams, err)
		}
	}
	wrong := teamSlot(saved, 1, "A-1")
	wrong.ID = "other-team"
	if _, err := repos.Teams.Initialize(ctx, wrong); !errors.Is(err, core.ErrRevision) {
		t.Fatalf("team repository accepted noncanonical ID: %v", err)
	}
}

func TestOneOffCASKeepsOnlyDerivedTeamAuthority(t *testing.T) {
	ctx, root := context.Background(), t.TempDir()
	repos := NewRepositories(store.New(t.TempDir(), core.StorageLimits{CanonicalBytes: 16 << 20}))
	feature, err := CreateOneOff(ctx, root, Feature, "feature", []core.Task{oneOffTask("F-1", "one", []string{"src/a"}, []string{"db:read"}), oneOffTask("F-2", "two", []string{"src/b"}, []string{"cache:read"})})
	if err != nil {
		t.Fatal(err)
	}
	saved, err := repos.Runs.Initialize(ctx, feature)
	if err != nil {
		t.Fatal(err)
	}
	broad := saved
	broad.Teams = append([]TeamRecord(nil), saved.Teams...)
	broad.Teams[0].Paths = append(broad.Teams[0].Paths, "src/extra")
	if _, err := repos.Runs.CompareAndSwap(ctx, saved.ID, saved.Revision, broad); err == nil {
		t.Fatal("feature team scope broadened")
	}
	broadResource := saved
	broadResource.Teams = append([]TeamRecord(nil), saved.Teams...)
	broadResource.Teams[0].Resources = append(broadResource.Teams[0].Resources, "cache:write")
	if _, err := repos.Runs.CompareAndSwap(ctx, saved.ID, saved.Revision, broadResource); err == nil {
		t.Fatal("feature team resource broadened")
	}
	valid := saved
	valid.Teams = append([]TeamRecord(nil), saved.Teams...)
	valid.Teams[0].State = core.Paused
	valid.Teams[0].Queue = []core.TaskID{"F-2", "F-1"}
	valid.Teams[0].QueueFingerprint = queueFingerprint(valid.Teams[0].Queue)
	if _, err := repos.Runs.CompareAndSwap(ctx, saved.ID, saved.Revision, valid); err != nil {
		t.Fatalf("valid state transition failed: %v", err)
	}
	audit, err := CreateOneOff(ctx, root, Audit, "audit", []core.Task{oneOffTask("A-1", "audit", []string{"src/a"}, []string{"db:test"})})
	if err != nil {
		t.Fatal(err)
	}
	audit, err = repos.Runs.Initialize(ctx, audit)
	if err != nil {
		t.Fatal(err)
	}
	write := audit
	write.Teams = append([]TeamRecord(nil), audit.Teams...)
	write.Teams[0].Paths = []string{"src/a"}
	if _, err := repos.Runs.CompareAndSwap(ctx, audit.ID, audit.Revision, write); err == nil {
		t.Fatal("audit write path restored")
	}
	writeResource := audit
	writeResource.Teams = append([]TeamRecord(nil), audit.Teams...)
	writeResource.Teams[0].Resources = []string{"db:test"}
	if _, err := repos.Runs.CompareAndSwap(ctx, audit.ID, audit.Revision, writeResource); err != nil {
		t.Fatalf("audit resource reservation was not rederived: %v", err)
	}
	review, err := CreateOneOff(ctx, root, Review, "review", []core.Task{oneOffTask("R-1", "review", []string{"src/a"}, []string{"db:test"})})
	if err != nil {
		t.Fatal(err)
	}
	review, err = repos.Runs.Initialize(ctx, review)
	if err != nil {
		t.Fatal(err)
	}
	reviewWrite := review
	reviewWrite.Teams = append([]TeamRecord(nil), review.Teams...)
	reviewWrite.Teams[0].Paths = []string{"src/a"}
	if _, err := repos.Runs.CompareAndSwap(ctx, review.ID, review.Revision, reviewWrite); err == nil {
		t.Fatal("review write path restored")
	}
}

func TestOneOffQueuesReleaseImmutableTasksAndKeepAuditReservations(t *testing.T) {
	ctx, root := context.Background(), t.TempDir()
	repos := NewRepositories(store.New(t.TempDir(), core.StorageLimits{CanonicalBytes: 16 << 20}))
	audit, err := CreateOneOff(ctx, root, Audit, "audit", []core.Task{oneOffTask("A-1", "audit", []string{"src/a"}, []string{"DB:TEST"})})
	if err != nil {
		t.Fatal(err)
	}
	if got := audit.Teams[0]; got.Paths != nil || !reflect.DeepEqual(got.Resources, []string{"db:test"}) {
		t.Fatalf("audit authority=%#v, want no paths and db:test reservation", got)
	}
	saved, err := repos.Runs.Initialize(ctx, audit)
	if err != nil {
		t.Fatal(err)
	}
	released := saved
	released.Teams = append([]TeamRecord(nil), saved.Teams...)
	released.Teams[0].Queue = nil
	released.Teams[0].QueueFingerprint = ""
	released.Teams[0].State = core.Idle
	released.Teams[0].Paths = nil
	released.Teams[0].Resources = nil
	released, err = repos.Runs.CompareAndSwap(ctx, saved.ID, saved.Revision, released)
	if err != nil {
		t.Fatalf("finished task release rejected: %v", err)
	}
	if len(released.Tasks) != 1 || len(released.Teams[0].Queue) != 0 || released.Teams[0].Paths != nil || released.Teams[0].Resources != nil {
		t.Fatalf("release did not preserve immutable task history with empty authority: %#v", released)
	}
	unknown := released
	unknown.Teams = append([]TeamRecord(nil), released.Teams...)
	unknown.Teams[0].Queue = []core.TaskID{"A-unknown"}
	unknown.Teams[0].QueueFingerprint = queueFingerprint(unknown.Teams[0].Queue)
	unknown.Teams[0].State = core.Working
	if _, err := repos.Runs.CompareAndSwap(ctx, released.ID, released.Revision, unknown); !errors.Is(err, core.ErrBatch) {
		t.Fatalf("unknown active task accepted: %v", err)
	}

	review, err := CreateOneOff(ctx, root, Review, "review", []core.Task{oneOffTask("R-1", "review", []string{"src/r"}, []string{"DB:TEST"})})
	if err != nil {
		t.Fatal(err)
	}
	if got := review.Teams[0]; got.Paths != nil || !reflect.DeepEqual(got.Resources, []string{"db:test"}) {
		t.Fatalf("review authority=%#v, want no paths and db:test reservation", got)
	}
}

func TestOneOffQueuesRejectDuplicateActiveIDs(t *testing.T) {
	ctx, root := context.Background(), t.TempDir()
	repos := NewRepositories(store.New(t.TempDir(), core.StorageLimits{CanonicalBytes: 16 << 20}))
	tasks := make([]core.Task, 9)
	for i := range tasks {
		tasks[i] = oneOffTask(core.TaskID(fmt.Sprintf("A-%d", i+1)), "audit", nil, nil)
	}
	run, err := CreateOneOff(ctx, root, Audit, "audit", tasks)
	if err != nil || len(run.Teams) != 2 {
		t.Fatalf("two-team audit=%#v err=%v", run.Teams, err)
	}
	saved, err := repos.Runs.Initialize(ctx, run)
	if err != nil {
		t.Fatal(err)
	}
	duplicate := saved
	duplicate.Teams = append([]TeamRecord(nil), saved.Teams...)
	duplicate.Teams[1].Queue = append(append([]core.TaskID(nil), saved.Teams[1].Queue...), saved.Teams[0].Queue[0])
	duplicate.Teams[1].QueueFingerprint = queueFingerprint(duplicate.Teams[1].Queue)
	if _, err := repos.Runs.CompareAndSwap(ctx, saved.ID, saved.Revision, duplicate); !errors.Is(err, core.ErrBatch) {
		t.Fatalf("duplicate active task accepted: %v", err)
	}
}

func TestTeamRepositoryRequiresCurrentRunSlot(t *testing.T) {
	ctx, root := context.Background(), t.TempDir()
	repos := NewRepositories(store.New(t.TempDir(), core.StorageLimits{CanonicalBytes: 16 << 20}))
	run, err := CreateOneOff(ctx, root, Feature, "feature", []core.Task{oneOffTask("F-1", "feature", []string{"src/a"}, []string{"db:test"})})
	if err != nil {
		t.Fatal(err)
	}
	saved, err := repos.Runs.Initialize(ctx, run)
	if err != nil {
		t.Fatal(err)
	}
	team := saved.Teams[0]
	if _, err := repos.Teams.Initialize(ctx, team); err != nil {
		t.Fatalf("matching team slot rejected: %v", err)
	}

	orphan := team
	orphan.RunID = "run-orphan"
	orphan.ID = canonicalTeamID(orphan.RunID, 1)
	if _, err := repos.Teams.Initialize(ctx, orphan); !errors.Is(err, core.ErrRevision) {
		t.Fatalf("orphan team accepted: %v", err)
	}
	if _, err := repos.Teams.Read(ctx, orphan.ID); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("orphan write occurred: %v", err)
	}
	mismatch := team
	mismatch.Paths = []string{"src/other"}
	if _, err := repos.Teams.Initialize(ctx, mismatch); !errors.Is(err, core.ErrRevision) {
		t.Fatalf("mismatched team accepted: %v", err)
	}

	parent := saved
	parent.Teams = append([]TeamRecord(nil), saved.Teams...)
	parent.Teams[0].State = core.Paused
	parent.Teams[0].Revision = team.Revision + 1
	parent, err = repos.Runs.CompareAndSwap(ctx, saved.ID, saved.Revision, parent)
	if err != nil {
		t.Fatalf("parent transition: %v", err)
	}
	next := team
	next.State = core.Paused
	if _, err := repos.Teams.CompareAndSwap(ctx, team.ID, team.Revision, next); err != nil {
		t.Fatalf("matching child transition rejected: %v", err)
	}
	wrong := team
	wrong.State = core.Working
	if _, err := repos.Teams.CompareAndSwap(ctx, team.ID, team.Revision+1, wrong); !errors.Is(err, core.ErrRevision) {
		t.Fatalf("contradictory child transition accepted: %v", err)
	}
}

func teamSlot(run Run, ordinal int, task core.TaskID) TeamRecord {
	queue := []core.TaskID{task}
	return TeamRecord{RecordEnvelope: core.RecordEnvelope{Schema: 1, Project: run.Project, RunID: run.ID, WrittenAt: run.WrittenAt, Revision: 1}, ID: core.TeamID(fmt.Sprintf("%s-team-%d", run.ID, ordinal)), Queue: queue, QueueFingerprint: queueFingerprint(queue), State: core.Working}
}

func TestOneOffRejectsAmbiguousScopesAndNeverMutatesInputs(t *testing.T) {
	root := t.TempDir()
	for _, scope := range []string{"src/CON", "src/file.txt:zone", "src/trailing. ", "src/trailing ", "src/COM1"} {
		if _, err := CreateOneOff(context.Background(), root, Feature, "feature", []core.Task{oneOffTask("F-1", "feature", []string{scope}, nil)}); !errors.Is(err, core.ErrPath) {
			t.Fatalf("unsafe scope %q accepted: %v", scope, err)
		}
	}
	input := oneOffTask("A-1", "audit", []string{"src"}, nil)
	before := append([]string(nil), input.WritablePaths...)
	if _, err := CreateOneOff(context.Background(), root, Audit, "audit", []core.Task{input}); err != nil || !reflect.DeepEqual(input.WritablePaths, before) {
		t.Fatalf("caller input mutated or audit failed: paths=%#v err=%v", input.WritablePaths, err)
	}
	missing := oneOffTask("F-1", "feature", []string{"src"}, nil)
	missing.Criteria = nil
	if _, err := CreateOneOff(context.Background(), root, Feature, "feature", []core.Task{missing}); !errors.Is(err, core.ErrBatch) || missing.WritablePaths[0] != "src" {
		t.Fatalf("criteria failure did not preserve input: %#v err=%v", missing, err)
	}
	over := oneOffTask("F-2", strings.Repeat("x", 256<<10), []string{"src"}, nil)
	if _, err := CreateOneOff(context.Background(), root, Feature, "feature", []core.Task{over}); !errors.Is(err, core.ErrLimit) {
		t.Fatalf("aggregate limit was not enforced: %v", err)
	}
}

func TestRunRootAndChildEnvelopesAreCanonicalAndLinked(t *testing.T) {
	root := t.TempDir()
	alias := filepath.Join(t.TempDir(), "root-link")
	if err := os.Symlink(root, alias); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	run, err := CreateOneOff(context.Background(), alias, Audit, "audit", []core.Task{oneOffTask("A-1", "audit", nil, nil)})
	if err != nil || run.Root != root || run.Project != root {
		t.Fatalf("canonical root=%q project=%q err=%v", run.Root, run.Project, err)
	}
	changed := run
	changed.Tasks = append([]core.Task(nil), run.Tasks...)
	changed.Tasks[0].Project = "other"
	if err := validateRun(changed); !errors.Is(err, core.ErrRevision) {
		t.Fatalf("foreign task envelope accepted: %v", err)
	}
	changed = run
	changed.Teams = append([]TeamRecord(nil), run.Teams...)
	changed.Teams[0].Project = "other"
	if err := validateRun(changed); !errors.Is(err, core.ErrRevision) {
		t.Fatalf("foreign team envelope accepted: %v", err)
	}
	changed = run
	changed.Root = alias
	if err := validateRun(changed); !errors.Is(err, core.ErrRevision) {
		t.Fatalf("root alias accepted: %v", err)
	}
}

func oneOffTask(id core.TaskID, objective string, writable, resources []string) core.Task {
	return core.Task{ID: id, Objective: objective, State: core.Ready, WritablePaths: writable, Resources: resources, Criteria: []string{"criterion"}}
}

type trackerStub struct {
	page core.TrackerPage
	ref  string
}

type noMetadataTracker struct{ tracker.Tracker }

func (s trackerStub) AuthorityMetadata() tracker.AuthorityMetadata {
	return tracker.AuthorityMetadata{Kind: "tasks-md", Ref: s.ref}
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
