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

	refused := core.Dependencies{ProjectRoot: root, Stdout: io.Discard, Stderr: io.Discard}
	if code := cli.Run(ctx, []string{"setup", "--refuse-kickoff", "--json"}, refused); code == 0 {
		t.Fatal("refused setup succeeded")
	}
	testkit.RequireNoWrites(t, root, beforeRoot)
	testkit.RequireNoWrites(t, outside, beforeOutside)

	approved := core.Dependencies{ProjectRoot: root, Stdout: io.Discard, Stderr: io.Discard, Confirmations: map[string]bool{"kickoff": true}}
	if code := cli.Run(ctx, []string{"setup", "--approve-kickoff", "--json"}, approved); code != 0 {
		t.Fatalf("approved setup failed: %d", code)
	}

	cases := []struct {
		name     string
		args     []string
		wantZero bool
	}{
		{"settings", []string{"settings", "runtime.kind=native", "--json"}, true},
		{"status", []string{"status", "--json"}, true},
		{"start deferred", []string{"start", "--run", "RUN-1", "--json"}, false},
		{"queue task", []string{"task", "add", "--queue", "queue work", "--json"}, true},
		{"execute deferred", []string{"task", "add", "--execute", "execute work", "--json"}, false},
		{"feature read only", []string{"one-off", "feature", "--objective", "feature work", "--json"}, true},
		{"audit read only", []string{"one-off", "audit", "--objective", "audit work", "--json"}, true},
		{"review read only", []string{"one-off", "review", "--objective", "review work", "--json"}, true},
		{"pause", []string{"pause", "--scope", "team:TEAM-1", "--json"}, true},
		{"inspect", []string{"inspect", "--run", "RUN-1", "--json"}, true},
		{"cleanup deferred", []string{"cleanup", "--team", "TEAM-1", "--json"}, false},
		{"deploy deferred", []string{"deploy", "--run", "RUN-1", "--target", "staging", "--json"}, false},
		{"unsupported deferred", []string{"checkpoint", "--json"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			code := cli.Run(ctx, tc.args, core.Dependencies{ProjectRoot: root, Stdout: &out, Stderr: &out})
			if (code == 0) != tc.wantZero {
				t.Fatalf("args=%v code=%d wantZero=%v output=%q", tc.args, code, tc.wantZero, out.String())
			}
			var envelope map[string]any
			if err := json.Unmarshal(out.Bytes(), &envelope); err != nil {
				t.Fatalf("args=%v invalid JSON=%q: %v", tc.args, out.String(), err)
			}
			if envelope["schema"] != float64(1) {
				t.Fatalf("args=%v schema=%v, want schema 1", tc.args, envelope["schema"])
			}
		})
	}
	testkit.RequireNoWrites(t, root, beforeRoot)
	testkit.RequireNoWrites(t, outside, beforeOutside)

	for _, args := range [][]string{{"start"}, {"task", "add", "--execute"}, {"cleanup"}, {"deploy"}, {"gate"}} {
		if _, err := cli.Parse(args); !errors.Is(err, core.ErrPhase) && args[0] == "gate" {
			t.Fatalf("args=%v error=%v, want ErrPhase", args, err)
		}
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
			page, err := tracker.NewTasksMD(path, store.New(filepath.Dir(path), core.DefaultConfig().Storage)).Page(ctx, "", 8)
			if count == 1001 {
				if !errors.Is(err, core.ErrCapacity) {
					t.Fatalf("count=%d error=%v, want ErrCapacity", count, err)
				}
				return
			}
			if err != nil || page.TotalNonArchived != count || page.TrackerRevision == 0 {
				t.Fatalf("count=%d page=%#v error=%v", count, page, err)
			}
		})
	}

	beads := tracker.NewBeads(tracker.NewFakeRunner(tracker.CommandResult{Stdout: []byte(`[{"id":"B-1","title":"fixture","status":"open"}]`)}))
	page, err := beads.Page(ctx, "", 8)
	if err != nil || page.TotalNonArchived != 1 || page.Tasks[0].ID != "B-1" {
		t.Fatalf("Beads fixture page=%#v error=%v", page, err)
	}
}

func TestPhase1InvariantAcceptance(t *testing.T) {
	ctx := context.Background()
	for _, kind := range []run.OneOffKind{run.Feature, run.Audit, run.Review} {
		t.Run(string(kind), func(t *testing.T) {
			task := core.Task{ID: "TASK-1", Objective: "bounded objective", State: core.Ready, Criteria: []string{"passes"}}
			if kind == run.Feature {
				task.WritablePaths = []string{"vnext/internal"}
			}
			manifest, err := run.CreateOneOff(ctx, t.TempDir(), kind, "bounded objective", []core.Task{task})
			if err != nil {
				t.Fatal(err)
			}
			if manifest.Schema != 1 || manifest.TrackerKind != "none" || manifest.OneOffKind != kind {
				t.Fatalf("one-off=%#v", manifest)
			}
		})
	}

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
