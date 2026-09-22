package run

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/thebpandey/agent-team/vnext/internal/core"
	"github.com/thebpandey/agent-team/vnext/internal/store"
	"github.com/thebpandey/agent-team/vnext/internal/tracker"
)

func TestCreatePlanRoundTripsExplicitDesignatedTracker(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".agent-team"), 0700); err != nil {
		t.Fatal(err)
	}
	contents := []byte("# Tasks\n\n## TASK-1\nObjective: Implement approved task\nState: ready\nWritable paths:\n- src\n")
	for _, path := range []string{"TASKS.md", ".agent-team/TASKS.md"} {
		if err := os.WriteFile(filepath.Join(root, path), contents, 0600); err != nil {
			t.Fatal(err)
		}
	}
	st := store.New(root, core.DefaultConfig().Storage)
	selected := tracker.NewTasksMD(filepath.Join(root, ".agent-team", "TASKS.md"), st)
	plan, err := CreatePlan(ctx, root, selected)
	if err != nil {
		t.Fatalf("designated tracker rejected: %v", err)
	}
	if err := validateRun(plan); err != nil {
		t.Fatal(err)
	}
	saved, err := NewRepositories(st).Runs.Initialize(ctx, plan)
	if err != nil {
		t.Fatal(err)
	}
	restored, err := NewRepositories(st).Runs.Read(ctx, saved.ID)
	if err != nil {
		t.Fatal(err)
	}
	if restored.SpecRevision != plan.SpecRevision || restored.ID != plan.ID {
		t.Fatal("authority changed on read")
	}
	defaultPlan, err := CreatePlan(ctx, root, tracker.NewTasksMD(filepath.Join(root, "TASKS.md"), st))
	if err != nil {
		t.Fatal(err)
	}
	if defaultPlan.SpecRevision == plan.SpecRevision || defaultPlan.ID == plan.ID {
		t.Fatal("different tracker paths share authority")
	}
	encoded, err := json.Marshal(defaultPlan)
	if err != nil {
		t.Fatal(err)
	}
	var legacy map[string]any
	if err := json.Unmarshal(encoded, &legacy); err != nil {
		t.Fatal(err)
	}
	if _, exists := legacy["trackerRef"]; exists {
		t.Fatal("default tracker unnecessarily changed legacy wire format")
	}
	encoded, err = json.Marshal(plan)
	if err != nil {
		t.Fatal(err)
	}
	var changed map[string]any
	if err := json.Unmarshal(encoded, &changed); err != nil {
		t.Fatal(err)
	}
	for _, replacement := range []any{filepath.Join(root, "TASKS.md"), filepath.Join(t.TempDir(), "TASKS.md"), nil} {
		if replacement == nil {
			delete(changed, "trackerRef")
		} else {
			changed["trackerRef"] = replacement
		}
		encoded, err = json.Marshal(changed)
		if err != nil {
			t.Fatal(err)
		}
		var tampered Run
		if err := json.Unmarshal(encoded, &tampered); err != nil {
			t.Fatal(err)
		}
		if err := validateRun(tampered); err == nil {
			t.Fatalf("accepted tracker reference replacement %v", replacement)
		}
	}
}

func TestCreatePlanRejectsDesignatedTrackerEscape(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	path := filepath.Join(outside, "TASKS.md")
	if err := os.WriteFile(path, []byte("# Tasks\n"), 0600); err != nil {
		t.Fatal(err)
	}
	selected := tracker.NewTasksMD(path, nil)
	if _, err := CreatePlan(context.Background(), root, selected); !errors.Is(err, core.ErrSettings) {
		t.Fatalf("outside tracker accepted: %v", err)
	}
	link := filepath.Join(root, "TASKS-link.md")
	if err := os.Symlink(path, link); err == nil {
		if _, err := CreatePlan(context.Background(), root, tracker.NewTasksMD(link, nil)); !errors.Is(err, core.ErrSettings) {
			t.Fatalf("symlink tracker escape accepted: %v", err)
		}
	}
}

type scopedAdmissionTracker struct {
	trackerStub
	allowed core.TaskID
}

func (s scopedAdmissionTracker) AllowsAutomaticAdmission(id core.TaskID) bool { return id == s.allowed }

func TestPrepareSinglePlanAdmissionKeepsSnapshotButHonorsApprovedScope(t *testing.T) {
	root := t.TempDir()
	ref := filepath.Join(root, "TASKS.md")
	if err := os.WriteFile(ref, []byte("# Tasks\n"), 0600); err != nil {
		t.Fatal(err)
	}
	outside := oneOffTask("A-OUTSIDE", "unrelated ready work", []string{"other"}, nil)
	dependency := oneOffTask("D-DONE", "existing dependency", nil, nil)
	dependency.State = core.Integrated
	inside := oneOffTask("Z-APPROVED", "approved work", []string{"src"}, nil)
	inside.Dependencies = []core.TaskID{dependency.ID}
	selected := scopedAdmissionTracker{trackerStub: trackerStub{ref: ref, page: core.TrackerPage{TrackerRevision: 5, TotalNonArchived: 3, Tasks: []core.Task{outside, dependency, inside}}}, allowed: inside.ID}
	prepared, err := PrepareSinglePlanAdmission(context.Background(), root, selected)
	if err != nil {
		t.Fatal(err)
	}
	if len(prepared.Batch.Tasks) != 1 || prepared.Batch.Tasks[0] != inside.ID {
		t.Fatalf("automatic admission escaped approved scope: %#v", prepared.Batch)
	}
	if len(prepared.Run.Tasks) != 3 {
		t.Fatal("scope filtering erased tracker snapshot or dependency facts")
	}
}
