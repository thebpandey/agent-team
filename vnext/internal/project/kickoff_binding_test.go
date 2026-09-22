package project

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/thebpandey/agent-team/vnext/internal/core"
	"github.com/thebpandey/agent-team/vnext/internal/store"
	"github.com/thebpandey/agent-team/vnext/internal/testkit"
)

func TestAttachKickoffPreservesSetupSettingsAndSurvivesNewHEAD(t *testing.T) {
	ctx := context.Background()
	root, source := kickoffFixture(t)
	setup, err := Onboard(ctx, SetupOptions{Root: root, Tracker: "tasks-md", Approved: true})
	if err != nil {
		t.Fatal(err)
	}
	st := store.New(root, core.DefaultConfig().Storage)
	settings, err := NewSettingsService(st).Update(ctx, map[string]string{"claude.developer.model": "custom-model"})
	if err != nil {
		t.Fatal(err)
	}
	path := writeKickoffFixture(t, root, source)
	protected := map[string][]byte{}
	for _, name := range []string{configPath, settingsPath, setup.ReceiptPath, "TASKS.md", "DECISIONS.md", "AGENT_TEAM_RULES.md", path} {
		raw, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Fatal(err)
		}
		protected[name] = raw
	}
	attached, err := AttachKickoff(ctx, root, path)
	if err != nil {
		t.Fatal(err)
	}
	if attached.Handoff.TrackerKind != "tasks-md" || len(attached.Handoff.TaskIDs) != 2 || attached.ReceiptPath != setup.ReceiptPath {
		t.Fatalf("incomplete attachment: %#v", attached)
	}
	bindingPath := filepath.Join(root, ".agent-team/v8/kickoff.json")
	before, err := os.ReadFile(bindingPath)
	if err != nil {
		t.Fatal(err)
	}
	infoBefore, _ := os.Stat(bindingPath)
	again, err := AttachKickoff(ctx, root, path)
	if err != nil || !reflect.DeepEqual(again.Handoff, attached.Handoff) {
		t.Fatalf("idempotent attach: %#v %v", again, err)
	}
	infoAfter, _ := os.Stat(bindingPath)
	after, _ := os.ReadFile(bindingPath)
	if !bytes.Equal(before, after) || !os.SameFile(infoBefore, infoAfter) {
		t.Fatal("unchanged handoff rewrote binding")
	}
	for name, want := range protected {
		got, _ := os.ReadFile(filepath.Join(root, name))
		if !bytes.Equal(want, got) {
			t.Fatalf("attachment changed %s", name)
		}
	}
	reread, err := NewSettingsService(st).Inspect(ctx)
	if err != nil || !reflect.DeepEqual(reread, settings) {
		t.Fatalf("settings binding changed: %#v %v", reread, err)
	}
	if _, err := git(ctx, root, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "--allow-empty", "-m", "Later work"); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(root, path)); err != nil {
		t.Fatal(err)
	}
	got, err := InspectSetup(ctx, root)
	if err != nil || !reflect.DeepEqual(got.Handoff, attached.Handoff) {
		t.Fatalf("future HEAD/source removal invalidated attachment: %#v %v", got, err)
	}
}

func TestAttachKickoffRejectsTrackerMismatchOrUnapprovedSource(t *testing.T) {
	for _, scenario := range []string{"kind", "path", "approval"} {
		t.Run(scenario, func(t *testing.T) {
			ctx := context.Background()
			root, source := kickoffFixture(t)
			if _, err := Onboard(ctx, SetupOptions{Root: root, Tracker: "tasks-md", Approved: true}); err != nil {
				t.Fatal(err)
			}
			switch scenario {
			case "kind":
				source["tracker"] = map[string]any{"kind": "beads", "executable": filepath.Join(root, "bd")}
			case "path":
				source["tracker"].(map[string]any)["path"] = ".agent-team/TASKS.md"
			case "approval":
				source["status"] = "pending"
			}
			path := writeKickoffFixture(t, root, source)
			before := testkit.SnapshotProjectTree(t, root)
			if _, err := AttachKickoff(ctx, root, path); err == nil {
				t.Fatal("accepted invalid handoff")
			}
			if after := testkit.SnapshotProjectTree(t, root); !reflect.DeepEqual(before, after) {
				t.Fatal("rejected handoff changed project files")
			}
		})
	}
}

func TestInspectSetupRejectsModifiedKickoffBinding(t *testing.T) {
	for _, field := range []string{"receiptPath", "receiptDigest", "handoff", "digest", "unknown"} {
		t.Run(field, func(t *testing.T) {
			ctx := context.Background()
			root, source := kickoffFixture(t)
			if _, err := Onboard(ctx, SetupOptions{Root: root, Tracker: "tasks-md", Approved: true}); err != nil {
				t.Fatal(err)
			}
			if _, err := AttachKickoff(ctx, root, writeKickoffFixture(t, root, source)); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(root, ".agent-team/v8/kickoff.json")
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var binding map[string]any
			if err := json.Unmarshal(raw, &binding); err != nil {
				t.Fatal(err)
			}
			if field == "handoff" {
				binding[field].(map[string]any)["trackerRef"] = ".agent-team/TASKS.md"
			} else {
				binding[field] = "modified"
			}
			altered, err := json.Marshal(binding)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, altered, 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := InspectSetup(ctx, root); !errors.Is(err, core.ErrRevision) {
				t.Fatalf("accepted modified binding: %v", err)
			}
		})
	}
}

func TestLoadKickoffPreservesBeadsExecutable(t *testing.T) {
	root, source := kickoffFixture(t)
	selected := filepath.Join(root, "custom tools", "bd")
	source["tracker"] = map[string]any{"kind": "beads", "executable": selected}
	got, err := LoadKickoff(root, writeKickoffFixture(t, root, source))
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(got)
	var view map[string]any
	if err := json.Unmarshal(raw, &view); err != nil {
		t.Fatal(err)
	}
	if view["trackerExecutable"] != selected {
		t.Fatalf("selected executable lost: %s", raw)
	}
	for _, invalid := range []string{"bd", "./bd", "/tmp/bd\nother"} {
		view["trackerExecutable"] = invalid
		if _, err := LoadKickoff(root, writeKickoffFixture(t, root, view)); err == nil {
			t.Fatalf("accepted unsafe executable %q", invalid)
		}
	}
}
