package acceptance_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/thebpandey/agent-team/vnext/internal/admission"
	"github.com/thebpandey/agent-team/vnext/internal/cli"
	"github.com/thebpandey/agent-team/vnext/internal/core"
	"github.com/thebpandey/agent-team/vnext/internal/knowledge"
	"github.com/thebpandey/agent-team/vnext/internal/project"
	"github.com/thebpandey/agent-team/vnext/internal/resources"
	"github.com/thebpandey/agent-team/vnext/internal/run"
	"github.com/thebpandey/agent-team/vnext/internal/store"
	"github.com/thebpandey/agent-team/vnext/internal/testkit"
	"github.com/thebpandey/agent-team/vnext/internal/tracker"
	"github.com/thebpandey/agent-team/vnext/internal/workflow"
)

// TestPhase1Acceptance keeps the command boundary deliberately shallow: Phase
// 1 parses and reports actions, but never starts hosts or changes product files.
func TestPhase1Acceptance(t *testing.T) {
	ctx := context.Background()
	root := testkit.GitRepo(t)
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "sentinel"), []byte("outside\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	beforeRoot := testkit.SnapshotTree(t, root)
	beforeOutside := testkit.SnapshotTree(t, outside)

	assertCLIOutcome(t, ctx, root, []string{"setup", "--refuse-kickoff", "--json"}, 2, cliEnvelope{Schema: 1, Action: "setup", Status: "rejected", Message: "setup rejected"})
	testkit.RequireNoWrites(t, root, beforeRoot)
	testkit.RequireNoWrites(t, outside, beforeOutside)

	cases := []struct {
		name string
		args []string
		code int
		want cliEnvelope
	}{
		{"approved setup", []string{"setup", "--approve-kickoff", "--json"}, 0, cliEnvelope{Schema: 1, Action: "setup", Status: "accepted", Message: "setup accepted"}},
		{"settings", []string{"settings", "runtime.kind=native", "--json"}, 0, cliEnvelope{Schema: 1, Action: "settings", Status: "accepted", Message: "settings accepted"}},
		{"status", []string{"status", "--json"}, 0, cliEnvelope{Schema: 1, Action: "status", Status: "accepted", Message: "status accepted"}},
		{"start deferred", []string{"start", "--run", "RUN-1", "--json"}, 2, cliEnvelope{Schema: 1, Action: "start", Status: "deferred", Message: "start deferred"}},
		{"queue task", []string{"task", "add", "--queue", "queue work", "--json"}, 0, cliEnvelope{Schema: 1, Action: "task add", Status: "accepted", Message: "task add accepted"}},
		{"execute deferred", []string{"task", "add", "--execute", "execute work", "--json"}, 2, cliEnvelope{Schema: 1, Action: "task add", Status: "deferred", Message: "task add deferred"}},
		{"feature", []string{"one-off", "feature", "--objective", "feature work", "--json"}, 0, cliEnvelope{Schema: 1, Action: "one-off feature", Status: "accepted", Message: "one-off feature accepted"}},
		{"audit", []string{"one-off", "audit", "--objective", "audit work", "--json"}, 0, cliEnvelope{Schema: 1, Action: "one-off audit", Status: "accepted", Message: "one-off audit accepted"}},
		{"review", []string{"one-off", "review", "--objective", "review work", "--json"}, 0, cliEnvelope{Schema: 1, Action: "one-off review", Status: "accepted", Message: "one-off review accepted"}},
		{"pause", []string{"pause", "--scope", "team:TEAM-1", "--json"}, 1, cliEnvelope{Schema: 1, Action: "pause", Status: "rejected", Message: "transition"}},
		{"inspect", []string{"inspect", "--run", "RUN-1", "--json"}, 0, cliEnvelope{Schema: 1, Action: "inspect", Status: "accepted", Message: "inspect accepted"}},
		{"cleanup deferred", []string{"cleanup", "--team", "TEAM-1", "--json"}, 2, cliEnvelope{Schema: 1, Action: "cleanup", Status: "deferred", Message: "cleanup deferred"}},
		{"deploy deferred", []string{"deploy", "--run", "RUN-1", "--target", "staging", "--json"}, 2, cliEnvelope{Schema: 1, Action: "deploy", Status: "deferred", Message: "deploy deferred"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assertCLIOutcome(t, ctx, root, tc.args, tc.code, tc.want)
		})
	}
	testkit.RequireNoWrites(t, root, beforeRoot)
	testkit.RequireNoWrites(t, outside, beforeOutside)

	for _, args := range [][]string{{"review"}, {"gate"}, {"integrate"}, {"reconcile"}, {"checkpoint"}, {"list"}, {"archive"}, {"plan"}, {"start", "--run"}, {"task", "add", "--execute", "--json", "extra"}, {"cleanup", "--bogus", "x"}, {"deploy", "--batch-size", "00"}} {
		if _, err := cli.Parse(args); !errors.Is(err, core.ErrPhase) {
			t.Fatalf("args=%v error=%v, want ErrPhase", args, err)
		}
	}
}

type cliEnvelope struct {
	Schema  int    `json:"schema"`
	Action  string `json:"action"`
	Status  string `json:"status"`
	Message string `json:"message"`
}

func assertCLIOutcome(t *testing.T, ctx context.Context, root string, args []string, wantCode int, want cliEnvelope) {
	t.Helper()
	var out bytes.Buffer
	code := cli.Run(ctx, args, core.Dependencies{ProjectRoot: root, Stdout: &out, Stderr: &out})
	if code != wantCode {
		t.Fatalf("args=%v code=%d want=%d output=%q", args, code, wantCode, out.String())
	}
	var got cliEnvelope
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("args=%v invalid JSON=%q: %v", args, out.String(), err)
	}
	if got != want {
		t.Fatalf("args=%v outcome=%+v want=%+v", args, got, want)
	}
}

func TestSetupAndTrackerAcceptance(t *testing.T) {
	ctx := context.Background()
	root := testkit.GitRepo(t)
	for name, contents := range map[string]string{
		"TASKS.md":            "# Tasks\n",
		"DECISIONS.md":        "# Decisions\n",
		"AGENT_TEAM_RULES.md": "# Rules\n",
	} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	before := testkit.SnapshotTree(t, root)
	refused := project.SetupInput{Root: root, Mode: project.PlanMode, Artifacts: []project.ArtifactDecision{{Path: "TASKS.md", Mode: project.ExistingArtifact, Confirmation: project.Refused}}}
	if _, err := project.ValidateSetup(ctx, refused); !errors.Is(err, core.ErrSettings) {
		t.Fatalf("refused setup error=%v, want ErrSettings", err)
	}
	testkit.RequireNoWrites(t, root, before)

	for _, count := range []int{900, 1000, 1001} {
		t.Run(fmt.Sprintf("tasks-md-%d", count), func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "TASKS.md")
			if err := os.WriteFile(path, []byte(taskDocument(count)), 0o600); err != nil {
				t.Fatal(err)
			}
			tr := tracker.NewTasksMD(path, store.New(filepath.Dir(path), core.DefaultConfig().Storage))
			page, err := tr.Page(ctx, "", 8)
			if count == 1001 {
				if !errors.Is(err, core.ErrCapacity) {
					t.Fatalf("count=%d error=%v, want ErrCapacity", count, err)
				}
				return
			}
			if err != nil || page.TotalNonArchived != count || len(page.Tasks) != 8 || page.Cursor != "8" || page.TrackerRevision == 0 {
				t.Fatalf("count=%d page=%#v error=%v", count, page, err)
			}
			next, err := tr.Page(ctx, page.Cursor, 8)
			if err != nil || next.TotalNonArchived != count || len(next.Tasks) != 8 || next.TrackerRevision != page.TrackerRevision {
				t.Fatalf("count=%d next page=%#v error=%v", count, next, err)
			}
			if warning := tr.(interface{ Warning() string }).Warning(); warning == "" {
				t.Fatalf("count=%d warning missing", count)
			}
		})
	}

	for _, count := range []int{900, 1000, 1001} {
		t.Run(fmt.Sprintf("beads-%d", count), func(t *testing.T) {
			tr := tracker.NewBeads(&beadsFixture{result: tracker.CommandResult{Stdout: []byte(beadsDocument(count))}})
			page, err := tr.Page(ctx, "", 8)
			if count == 1001 {
				if !errors.Is(err, core.ErrCapacity) {
					t.Fatalf("count=%d error=%v, want ErrCapacity", count, err)
				}
				return
			}
			if err != nil || page.TotalNonArchived != count || len(page.Tasks) != 8 || page.Cursor != "8" || page.TrackerRevision == 0 {
				t.Fatalf("count=%d page=%#v error=%v", count, page, err)
			}
			next, err := tr.Page(ctx, page.Cursor, 8)
			if err != nil || next.TotalNonArchived != count || len(next.Tasks) != 8 || next.TrackerRevision != page.TrackerRevision {
				t.Fatalf("count=%d next page=%#v error=%v", count, next, err)
			}
			if warning := tr.(interface{ Warning() string }).Warning(); warning == "" {
				t.Fatalf("count=%d warning missing", count)
			}
		})
	}
}

func TestPhase1InvariantAcceptance(t *testing.T) {
	ctx := context.Background()
	projectRoot := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "sentinel"), []byte("outside\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	beforeProject := testkit.SnapshotTree(t, projectRoot)
	beforeOutside := testkit.SnapshotTree(t, outside)
	for _, kind := range []run.OneOffKind{run.Feature, run.Audit, run.Review} {
		t.Run(string(kind), func(t *testing.T) {
			task := core.Task{ID: "TASK-1", Objective: "bounded objective", State: core.Ready, Criteria: []string{"passes"}, WritablePaths: []string{"vnext/internal"}, Resources: []string{"db:test"}}
			manifest, err := run.CreateOneOff(ctx, projectRoot, kind, "bounded objective", []core.Task{task})
			if err != nil {
				t.Fatal(err)
			}
			if manifest.Schema != 1 || manifest.TrackerKind != "none" || manifest.OneOffKind != kind {
				t.Fatalf("one-off=%#v", manifest)
			}
			if kind == run.Feature && (len(manifest.Teams) != 1 || len(manifest.Teams[0].Paths) != 1 || manifest.Teams[0].Paths[0] != "vnext/internal" || len(manifest.Teams[0].Resources) != 1) {
				t.Fatalf("feature authority=%#v", manifest.Teams)
			}
			if kind != run.Feature && (len(manifest.Teams) != 1 || len(manifest.Teams[0].Paths) != 0 || len(manifest.Tasks[0].WritablePaths) != 0 || len(manifest.Teams[0].Resources) != 1) {
				t.Fatalf("read-only authority=%#v", manifest)
			}
		})
	}
	testkit.RequireNoWrites(t, projectRoot, beforeProject)
	testkit.RequireNoWrites(t, outside, beforeOutside)

	fixture := testkit.NewAdmissionFixture(t)
	batch := fixture.Batch(1)
	if _, err := admission.AppendAdmission(ctx, fixture.Store, fixture.Tracker, fixture.Run, fixture.RunRevision, fixture.Team, fixture.TeamRevision, fixture.TrackerRevision, fixture.TaskRevisions, batch); err != nil {
		t.Fatal(err)
	}
	duplicate, err := admission.AppendAdmission(ctx, fixture.Store, fixture.Tracker, fixture.Run, fixture.RunRevision, fixture.Team, fixture.TeamRevision, fixture.TrackerRevision, fixture.TaskRevisions, batch)
	if err != nil || duplicate.Kind != admission.Duplicate {
		t.Fatalf("duplicate admission outcome=%#v error=%v, want idempotent existing", duplicate, err)
	}
	stale, err := admission.AppendAdmission(ctx, fixture.Store, fixture.Tracker, fixture.Run, fixture.RunRevision+1, fixture.Team, fixture.TeamRevision, fixture.TrackerRevision, fixture.TaskRevisions, fixture.Batch(2))
	if err != nil || stale.Kind != admission.Stale {
		t.Fatalf("stale admission outcome=%#v error=%v, want stale", stale, err)
	}

	for _, count := range []int{900, 1000, 1001} {
		f := testkit.NewAdmissionFixtureWithCapacity(t, count)
		out, err := admission.AppendAdmission(ctx, f.Store, f.Tracker, f.Run, f.RunRevision, f.Team, f.TeamRevision, f.TrackerRevision, f.TaskRevisions, f.Batch(1))
		if count == 1001 {
			if !errors.Is(err, core.ErrCapacity) {
				t.Fatalf("admission count=%d error=%v", count, err)
			}
			continue
		}
		if err != nil || out.Warning == "" {
			t.Fatalf("admission count=%d outcome=%#v error=%v", count, out, err)
		}
	}

	if err := resources.Reserve([]resources.Capacity{{Name: "worker", Effective: 0}}, []resources.Reservation{{Name: "worker", Count: 1}}); !errors.Is(err, core.ErrCapacity) {
		t.Fatalf("capacity refusal error=%v, want ErrCapacity", err)
	}
	if _, err := workflow.Transition(core.Working, workflow.Event{Run: "RUN-1", Scope: core.Scope{Kind: core.ScopeTeam, ID: "TEAM-1"}, Kind: workflow.Pause, From: core.Working, To: core.Paused, Reason: "operator", AdmissionHeld: true, RefillHeld: true}); err != nil {
		t.Fatalf("scoped pause=%v", err)
	}
	checkpointStore, manifest, scope := checkpointFixture(t)
	checkpointDigest := "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	if err := workflow.Checkpoint(ctx, checkpointStore, manifest.ID, scope, checkpointDigest); err != nil {
		t.Fatalf("scoped checkpoint=%v", err)
	}
	if err := workflow.Checkpoint(ctx, checkpointStore, manifest.ID, core.Scope{Kind: core.ScopeTask, ID: "TASK-2"}, checkpointDigest); !errors.Is(err, core.ErrRevision) {
		t.Fatalf("foreign scoped checkpoint error=%v, want ErrRevision", err)
	}

	tooMany := knowledge.HandoffSnapshot{RecordEnvelope: core.RecordEnvelope{Schema: 1, Project: "project", RunID: "RUN-1", WrittenAt: "1970-01-01T00:00:00Z", Revision: 1}, NextAction: "resume", Freshness: "now", Gates: make([]string, 129)}
	if _, err := knowledge.DeriveHandoff(tooMany); !errors.Is(err, core.ErrLimit) {
		t.Fatalf("bounded handoff error=%v, want ErrLimit", err)
	}
}

func TestPathEscapeAcceptance(t *testing.T) {
	ctx := context.Background()
	root := testkit.GitRepo(t)
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "sentinel"), []byte("outside\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	beforeOutside := testkit.SnapshotTree(t, outside)
	for _, candidate := range []string{filepath.Join(root, "..", filepath.Base(outside), "sentinel"), filepath.Join(outside, "sentinel")} {
		if _, err := project.Contain(root, candidate); !errors.Is(err, core.ErrPath) {
			t.Fatalf("Contain(%q) error=%v, want ErrPath", candidate, err)
		}
	}
	for _, writable := range []string{"../outside", filepath.Join(outside, "product")} {
		_, err := run.CreateOneOff(ctx, root, run.Feature, "escape", []core.Task{{ID: "TASK-1", Objective: "escape", State: core.Ready, Criteria: []string{"blocked"}, WritablePaths: []string{writable}}})
		if !errors.Is(err, core.ErrPath) {
			t.Fatalf("writable path %q error=%v, want ErrPath", writable, err)
		}
	}
	setup := project.SetupInput{Root: root, Mode: project.PlanMode, Artifacts: []project.ArtifactDecision{{Path: filepath.Join(outside, "sentinel"), Mode: project.ExistingArtifact, Confirmation: project.Approved}}}
	if _, err := project.ValidateSetup(ctx, setup); !errors.Is(err, core.ErrPath) {
		t.Fatalf("absolute setup artifact error=%v, want ErrPath", err)
	}
	link := filepath.Join(root, "escape")
	if err := os.Symlink(outside, link); err != nil {
		t.Logf("symlink fixture skipped (Windows privilege may be required): %v", err)
	} else if _, err := project.Contain(root, filepath.Join(link, "sentinel")); !errors.Is(err, core.ErrPath) {
		t.Fatalf("symlink escape error=%v, want ErrPath", err)
	}
	testkit.RequireNoWrites(t, outside, beforeOutside)
}

func TestSchemaOneCanonicalAcceptance(t *testing.T) {
	ctx := context.Background()
	root := testkit.GitRepo(t)
	trackerPath := filepath.Join(root, "TASKS.md")
	if err := os.WriteFile(trackerPath, []byte(taskDocument(1)), 0o600); err != nil {
		t.Fatal(err)
	}
	selected := tracker.NewTasksMD(trackerPath, store.New(root, core.DefaultConfig().Storage))
	plan, err := run.CreatePlan(ctx, root, selected)
	if err != nil || plan.Schema != 1 || plan.TrackerKind != "tasks-md" || plan.TrackerRevision == 0 || plan.TrackerSnapshotDigest == "" {
		t.Fatalf("selected TASKS.md authority plan=%#v error=%v", plan, err)
	}

	manifest, err := run.CreateOneOff(ctx, root, run.Feature, "schema fixture", []core.Task{{ID: "TASK-1", Objective: "fixture", State: core.Ready, Criteria: []string{"done"}, WritablePaths: []string{"src"}}})
	if err != nil {
		t.Fatal(err)
	}
	s := store.New(root, core.StorageLimits{CanonicalBytes: 16 << 20})
	repos := run.NewRepositories(s)
	if _, err := repos.Runs.Initialize(ctx, manifest); err != nil {
		t.Fatal(err)
	}
	if _, err := repos.Teams.Initialize(ctx, manifest.Teams[0]); err != nil {
		t.Fatal(err)
	}
	receipt := knowledge.Receipt{RecordEnvelope: core.RecordEnvelope{Schema: 1, Project: manifest.Project, RunID: manifest.ID, WrittenAt: manifest.WrittenAt, Revision: 1}, Team: string(manifest.Teams[0].ID), Task: "TASK-1", Attempt: 1, State: core.Implementing, NextAction: "continue"}
	if err := knowledge.WriteReceipt(ctx, s, receipt); err != nil {
		t.Fatal(err)
	}
	foreignRoot := testkit.GitRepo(t)
	conflictingRun := manifest
	conflictingRun.Tasks = append([]core.Task(nil), manifest.Tasks...)
	conflictingRun.Teams = append([]run.TeamRecord(nil), manifest.Teams...)
	conflictingRun.Root = foreignRoot
	conflictingRun.Project = foreignRoot
	for i := range conflictingRun.Tasks {
		conflictingRun.Tasks[i].Project = foreignRoot
	}
	for i := range conflictingRun.Teams {
		conflictingRun.Teams[i].Project = foreignRoot
	}
	if _, err := repos.Runs.CompareAndSwap(ctx, manifest.ID, manifest.Revision, conflictingRun); !errors.Is(err, core.ErrRevision) {
		t.Fatalf("conflicting run error=%v, want ErrRevision", err)
	}
	lastGood, err := repos.Runs.Read(ctx, manifest.ID)
	if err != nil || lastGood.Project != manifest.Project || lastGood.Root != manifest.Root || lastGood.Revision != manifest.Revision || lastGood.ManifestDigest != manifest.ManifestDigest {
		t.Fatalf("conflicting run changed last-good=%#v error=%v", lastGood, err)
	}

	badRun := manifest
	badRun.Schema = 2
	writeFixtureJSON(t, filepath.Join(root, ".agent-team", "runs", string(manifest.ID)+".json"), badRun)
	if _, err := repos.Runs.Read(ctx, manifest.ID); !errors.Is(err, core.ErrRevision) {
		t.Fatalf("schema-2 run error=%v, want ErrRevision", err)
	}
	if err := os.WriteFile(filepath.Join(root, ".agent-team", "runs", string(manifest.ID)+".json"), []byte("[]"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := repos.Runs.Read(ctx, manifest.ID); !errors.Is(err, core.ErrPath) {
		t.Fatalf("malformed run error=%v, want ErrPath", err)
	}
	writeFixtureJSON(t, filepath.Join(root, ".agent-team", "runs", string(manifest.ID)+".json"), manifest)

	badTeam := manifest.Teams[0]
	badTeam.Schema = 2
	writeFixtureJSON(t, filepath.Join(root, ".agent-team", "teams", string(badTeam.ID)+".json"), badTeam)
	if _, err := repos.Teams.Read(ctx, badTeam.ID); !errors.Is(err, core.ErrRevision) {
		t.Fatalf("schema-2 team error=%v, want ErrRevision", err)
	}
	if err := os.WriteFile(filepath.Join(root, ".agent-team", "teams", string(badTeam.ID)+".json"), []byte("[]"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := repos.Teams.Read(ctx, badTeam.ID); !errors.Is(err, core.ErrPath) {
		t.Fatalf("malformed team error=%v, want ErrPath", err)
	}
	badTeam = manifest.Teams[0]
	badTeam.Project = "conflicting-project"
	writeFixtureJSON(t, filepath.Join(root, ".agent-team", "teams", string(badTeam.ID)+".json"), badTeam)
	if _, err := repos.Teams.CompareAndSwap(ctx, badTeam.ID, badTeam.Revision, badTeam); !errors.Is(err, core.ErrRevision) {
		t.Fatalf("conflicting team error=%v, want ErrRevision", err)
	}
	writeFixtureJSON(t, filepath.Join(root, ".agent-team", "teams", string(manifest.Teams[0].ID)+".json"), manifest.Teams[0])

	badReceipt := receipt
	badReceipt.Schema = 2
	writeFixtureJSON(t, filepath.Join(root, ".agent-team", "receipts", string(manifest.Teams[0].ID)+".json"), badReceipt)
	digest := "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	if err := workflow.Checkpoint(ctx, s, manifest.ID, core.Scope{Kind: core.ScopeTask, ID: "TASK-1"}, digest); !errors.Is(err, core.ErrRevision) {
		t.Fatalf("schema-2 receipt error=%v, want ErrRevision", err)
	}
	if err := os.WriteFile(filepath.Join(root, ".agent-team", "receipts", string(manifest.Teams[0].ID)+".json"), []byte("[]"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := workflow.Checkpoint(ctx, s, manifest.ID, core.Scope{Kind: core.ScopeTask, ID: "TASK-1"}, digest); !errors.Is(err, core.ErrPath) {
		t.Fatalf("malformed receipt error=%v, want ErrPath", err)
	}
	badReceipt = receipt
	badReceipt.Project = "conflicting-project"
	writeFixtureJSON(t, filepath.Join(root, ".agent-team", "receipts", string(manifest.Teams[0].ID)+".json"), badReceipt)
	if err := workflow.Checkpoint(ctx, s, manifest.ID, core.Scope{Kind: core.ScopeTask, ID: "TASK-1"}, digest); !errors.Is(err, core.ErrRevision) {
		t.Fatalf("conflicting receipt error=%v, want ErrRevision", err)
	}
}

func writeFixtureJSON(t *testing.T, path string, value any) {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestGofmtUsesArgumentArray(t *testing.T) {
	ctx := context.Background()
	cmd := exec.CommandContext(ctx, "gofmt", "-l", ".")
	cmd.Dir = filepath.Clean(filepath.Join("..", ".."))
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = io.Discard
	if err := cmd.Run(); err != nil {
		t.Fatalf("gofmt -l: %v", err)
	}
	if files := strings.TrimSpace(stdout.String()); files != "" {
		t.Fatalf("gofmt reported unformatted files:\n%s", files)
	}
}

func taskDocument(count int) string {
	var b strings.Builder
	b.WriteString("# Tasks\n\n")
	for i := 1; i <= count; i++ {
		fmt.Fprintf(&b, "## T-%04d\nObjective: task %d\nState: ready\n\n", i, i)
	}
	return b.String()
}

type beadsFixture struct {
	result tracker.CommandResult
}

func (f *beadsFixture) Run(context.Context, string, ...string) tracker.CommandResult {
	return f.result
}

func beadsDocument(count int) string {
	var b strings.Builder
	b.WriteByte('[')
	for i := 1; i <= count; i++ {
		if i > 1 {
			b.WriteByte(',')
		}
		fmt.Fprintf(&b, `{"id":"B-%04d","title":"task %d","status":"open"}`, i, i)
	}
	b.WriteByte(']')
	return b.String()
}

func checkpointFixture(t *testing.T) (*store.Store, run.Run, core.Scope) {
	t.Helper()
	ctx := context.Background()
	manifest, err := run.CreateOneOff(ctx, t.TempDir(), run.Feature, "checkpoint fixture", []core.Task{{ID: "TASK-1", Objective: "fixture", State: core.Ready, Criteria: []string{"done"}, WritablePaths: []string{"src"}}})
	if err != nil {
		t.Fatal(err)
	}
	s := store.New(manifest.Root, core.StorageLimits{CanonicalBytes: 16 << 20})
	if _, err := run.NewRepositories(s).Runs.Initialize(ctx, manifest); err != nil {
		t.Fatal(err)
	}
	receipt := knowledge.Receipt{RecordEnvelope: core.RecordEnvelope{Schema: 1, Project: manifest.Project, RunID: manifest.ID, WrittenAt: manifest.WrittenAt, Revision: 1}, Team: string(manifest.Teams[0].ID), Task: "TASK-1", Attempt: 1, State: core.Implementing, NextAction: "continue"}
	if err := knowledge.WriteReceipt(ctx, s, receipt); err != nil {
		t.Fatal(err)
	}
	return s, manifest, core.Scope{Kind: core.ScopeTask, ID: "TASK-1"}
}
