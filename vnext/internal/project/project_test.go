package project

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/thebpandey/agent-team/vnext/internal/core"
	"github.com/thebpandey/agent-team/vnext/internal/store"
	"github.com/thebpandey/agent-team/vnext/internal/testkit"
)

func TestContainRejectsEscapesAndLinks(t *testing.T) {
	root := t.TempDir()
	inside := filepath.Join(root, "inside")
	if err := os.Mkdir(inside, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := Contain(root, filepath.Join(root, "..", "outside")); !errors.Is(err, core.ErrPath) {
		t.Fatalf("escape error = %v, want ErrPath", err)
	}
	if err := os.Symlink(t.TempDir(), filepath.Join(inside, "escape")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, err := Contain(root, filepath.Join(inside, "escape", "file")); !errors.Is(err, core.ErrPath) {
		t.Fatalf("link escape error = %v, want ErrPath", err)
	}
	got, err := Contain(root, filepath.Join(inside, "new-file"))
	if err != nil || got != filepath.Join(inside, "new-file") {
		t.Fatalf("contained path = %q, %v", got, err)
	}
}

func TestValidateSegmentPortable(t *testing.T) {
	for _, value := range []string{"", ".", "..", "CON", "nul.txt", "a/b", `a\\b`, "file.", "file ", "a:b", "a?b"} {
		if err := ValidateSegment(value); !errors.Is(err, core.ErrPath) {
			t.Errorf("ValidateSegment(%q) = %v, want ErrPath", value, err)
		}
	}
	if err := ValidateSegment("Tasks-01.md"); err != nil {
		t.Fatalf("ordinary segment rejected: %v", err)
	}
}

func TestDiscoverReportsGitIdentityAndState(t *testing.T) {
	root := testkit.GitRepo(t)
	project, err := Discover(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if project.Root != root || project.TopLevel != root || project.Head == "" || project.CommonDir == "" {
		t.Fatalf("incomplete project identity: %#v", project)
	}
	if project.Dirty || project.Detached || !project.Readable || !project.Writable || project.FreeBytes <= 0 {
		t.Fatalf("incorrect clean project state: %#v", project)
	}
	if err := os.WriteFile(filepath.Join(root, "dirty.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	project, err = Discover(context.Background(), root)
	if err != nil || !project.Dirty {
		t.Fatalf("dirty project = %#v, %v", project, err)
	}
}

func TestSetupValidatesDecisionsAndPersistsIdempotently(t *testing.T) {
	root := testkit.GitRepo(t)
	if err := os.WriteFile(filepath.Join(root, "TASKS.md"), []byte("# Tasks\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	handoff := core.KickoffHandoff{
		ApprovedPlanRevision: "plan-7", Branch: "main", TrackerKind: "tasks-md", TrackerRef: "TASKS.md", TrackerRevision: 3,
		TaskIDs: []core.TaskID{"TASK-1"}, Acceptance: []string{"go test ./..."}, Checks: []core.Check{{Name: "test", Command: []string{"go", "test", "./..."}}},
		WritablePaths: []string{"vnext"}, Resources: []string{"network:none"}, Capabilities: []string{"git"},
	}
	encoded, err := json.Marshal(handoff)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "docs", "kickoff.json"), encoded, 0o644); err != nil {
		t.Fatal(err)
	}
	digest := digest(encoded)

	refused := SetupInput{Root: root, Mode: PlanMode, Artifacts: []ArtifactDecision{{Path: "TASKS.md", Mode: ExistingArtifact, Confirmation: Refused}}}
	if _, err := ValidateSetup(context.Background(), refused); !errors.Is(err, core.ErrSettings) {
		t.Fatalf("refused artifact error = %v, want ErrSettings", err)
	}
	approved := SetupInput{Root: root, Mode: PlanMode, Artifacts: []ArtifactDecision{{Path: "TASKS.md", Mode: ExistingArtifact, Confirmation: Approved}}, Kickoff: &KickoffDecision{Path: "docs/kickoff.json", Digest: digest, Confirmation: Approved}}
	validated, err := ValidateSetup(context.Background(), approved)
	if err != nil {
		t.Fatalf("approved setup: %v", err)
	}
	if validated.ArtifactDigests["TASKS.md"] == "" || validated.Handoff.ApprovedPlanRevision != "plan-7" || len(validated.Handoff.TaskIDs) != 1 {
		t.Fatalf("validated result missing facts: %#v", validated)
	}
	badDigest := approved
	badDigest.Kickoff = &KickoffDecision{Path: approved.Kickoff.Path, Digest: "sha256:bad", Confirmation: Approved}
	if _, err := ValidateSetup(context.Background(), badDigest); !errors.Is(err, core.ErrSettings) {
		t.Fatalf("bad kickoff digest error = %v, want ErrSettings", err)
	}

	svc := NewSetupService(store.New(root, core.DefaultConfig().Storage))
	result, err := svc.Initialize(context.Background(), approved)
	if err != nil {
		t.Fatal(err)
	}
	if result.ConfigRevision != 1 || result.ReceiptPath == "" {
		t.Fatalf("bad initialized result: %#v", result)
	}
	before := testkit.SnapshotTree(t, root)
	if _, err := svc.Initialize(context.Background(), approved); err != nil {
		t.Fatalf("idempotent initialize: %v", err)
	}
	if after := testkit.SnapshotTree(t, root); !equalTree(before, after) {
		t.Fatal("idempotent initialize wrote state")
	}
	conflicting := approved
	conflicting.Kickoff = nil
	if _, err := svc.Initialize(context.Background(), conflicting); !errors.Is(err, core.ErrRevision) {
		t.Fatalf("conflicting initialize error = %v, want ErrRevision", err)
	}
}

func digest(value []byte) string {
	sum := sha256.Sum256(value)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func equalTree(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for path, value := range a {
		if b[path] != value {
			return false
		}
	}
	return true
}
