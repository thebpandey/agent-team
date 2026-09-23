package tracker

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/thebpandey/agent-team/vnext/internal/core"
)

func TestBeadsIgnoresUnrelatedMetadataButValidatesAuthority(t *testing.T) {
	for _, metadata := range []string{`{"external":{"ticket":12},"nullable":null,"criteria":["works"]}`, `{}`} {
		body := []byte(`[{"id":"B-1","title":"task","status":"open","metadata":` + metadata + `}]`)
		if _, err := parseBeads(body); err != nil {
			t.Fatalf("unrelated metadata: %v", err)
		}
	}
	for _, metadata := range []string{`{"criteria":null}`, `{"checks":"wrong"}`, `{"writablePaths":12}`} {
		body := []byte(`[{"id":"B-1","title":"task","status":"open","metadata":` + metadata + `}]`)
		if _, err := parseBeads(body); err == nil {
			t.Fatalf("accepted invalid authority: %s", metadata)
		}
	}
}

func TestBeadsFailureDiagnosticIsBoundedAndRedacted(t *testing.T) {
	t.Setenv("BEADS_TEST_TOKEN", "fixture-secret-token")
	err := commandResultError(CommandResult{Exit: 1, Stderr: []byte("database offline https://user:password@localhost/db token=literal-secret fixture-secret-token\n" + strings.Repeat("x", 2000))})
	message := err.Error()
	if !strings.Contains(message, "database offline") || len(message) > 600 {
		t.Fatalf("unhelpful diagnostic: %q", message)
	}
	for _, secret := range []string{"password", "literal-secret", "fixture-secret-token"} {
		if strings.Contains(message, secret) {
			t.Fatalf("exposed secret %q", secret)
		}
	}
	for _, input := range []string{`{"password":"quoted secret"}`, `Authorization: Bearer bearer-secret`, "\x1b[31mdatabase offline"} {
		got := beadsDiagnostic([]byte(input))
		if strings.Contains(got, "quoted secret") || strings.Contains(got, "bearer-secret") || strings.ContainsRune(got, '\x1b') {
			t.Fatalf("unsafe diagnostic: %q", got)
		}
	}
}

func TestBeadsClosedPrerequisiteRemainsInSnapshot(t *testing.T) {
	body := []byte(`[{"id":"B-1","title":"prerequisite","status":"closed","closed_at":"2026-09-22T19:53:30Z","close_reason":"done"},{"id":"B-2","title":"dependent","status":"open","dependencies":[{"issue_id":"B-2","depends_on_id":"B-1","type":"blocks"}]},{"id":"B-old","title":"archived","status":"archived"}]`)
	page, err := NewBeads(NewFakeRunner(CommandResult{Stdout: body})).Page(context.Background(), "", 8)
	if err != nil || len(page.Tasks) != 2 || page.Tasks[0].State != core.Integrated || len(page.Tasks[1].Dependencies) != 1 {
		t.Fatalf("closed prerequisite lost: %#v, %v", page, err)
	}
}

func TestBeadsCreateMinimalTaskOmitsNullAuthority(t *testing.T) {
	created := `[{"id":"B-1","title":"minimal","status":"open","metadata":{}}]`
	runner := &scriptedRunner{results: []CommandResult{{Stdout: []byte(`[]`)}, {}, {Stdout: []byte(created)}}}
	_, err := NewBeads(runner).Create(context.Background(), core.Task{ID: "B-1", Objective: "minimal"}, beadsFixtureRevision(t, `[]`))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.Join(runner.Calls()[1], " "), "null") {
		t.Fatal("create sent null authority fields")
	}
}

func TestBeadsRejectsPossiblyTruncatedSnapshot(t *testing.T) {
	issues := make([]string, 1001)
	for i := range issues {
		issues[i] = fmt.Sprintf(`{"id":"B-%d","title":"task","status":"closed"}`, i)
	}
	_, err := NewBeads(NewFakeRunner(CommandResult{Stdout: []byte("[" + strings.Join(issues, ",") + "]")})).Page(context.Background(), "", 1000)
	if !errors.Is(err, core.ErrCapacity) {
		t.Fatalf("truncated snapshot accepted: %v", err)
	}
}

// Explicitly opt in only with a disposable, already initialized Beads project.
// This creates one named issue; reruns reuse its exact metadata.
func TestBeadsPinnedDisposableRuntime(t *testing.T) {
	root := os.Getenv("AGENT_TEAM_BEADS_SMOKE_ROOT")
	if root == "" {
		t.Skip("requires disposable Beads fixture")
	}
	t.Chdir(root)
	runner := smokeBeadsRunner{filepath.Join(root, ".agent-team/dependencies/bin/bd")}
	tr := NewBeads(runner)
	page, err := tr.Page(context.Background(), "", 1000)
	if err != nil {
		t.Fatal(err)
	}
	// A previous smoke may have closed the disposable issue. It must no longer
	// consume active capacity, but an active task referencing it can read it via
	// the actual pinned batch-show payload.
	historical := runner.Run(context.Background(), "bd", "--readonly", "show", "--json", "--", "project-adapter-smoke")
	if historical.Exit == 0 && historical.Transport == nil {
		issues, err := parseBeads(historical.Stdout)
		if err != nil {
			t.Fatal(err)
		}
		if len(issues) == 1 && issues[0].State == core.Integrated {
			if _, err := tr.Get(context.Background(), issues[0].ID, page.TrackerRevision); !errors.Is(err, core.ErrPath) {
				t.Fatalf("unreferenced history in active view: %v", err)
			}
			dependent := NewBeads(smokeActiveBeadsRunner{runner})
			view, err := dependent.Page(context.Background(), "", 1000)
			if err != nil {
				t.Fatal(err)
			}
			got, err := dependent.Get(context.Background(), issues[0].ID, view.TrackerRevision)
			if err != nil || got.State != core.Integrated {
				t.Fatalf("referenced closed fixture: %#v, %v", got, err)
			}
			return
		}
	}
	for _, existing := range page.Tasks {
		if existing.ID == "project-adapter-smoke" && existing.State == core.Integrated {
			got, err := tr.Get(context.Background(), existing.ID, page.TrackerRevision)
			if err != nil || got.State != core.Integrated {
				t.Fatalf("closed fixture: %#v, %v", got, err)
			}
			return
		}
	}
	want := core.Task{ID: "project-adapter-smoke", Objective: "Disposable adapter smoke", State: core.Ready, Criteria: []string{"adapter round trip succeeds"}, Checks: []core.Check{{Name: "smoke", Command: []string{"true"}}}, WritablePaths: []string{"reports"}, Resources: []string{}, EvidencePointers: []string{}}
	got, err := tr.Create(context.Background(), want, page.TrackerRevision)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := tr.Get(context.Background(), got.ID, got.Revision)
	if err != nil || !sameTask(got, loaded) {
		t.Fatalf("Get = %#v, %v", loaded, err)
	}
}

type smokeBeadsRunner struct{ path string }

func (r smokeBeadsRunner) Run(ctx context.Context, _ string, args ...string) CommandResult {
	return NewCommandRunner().Run(ctx, r.path, args...)
}

type smokeActiveBeadsRunner struct{ smokeBeadsRunner }

func (r smokeActiveBeadsRunner) Run(ctx context.Context, name string, args ...string) CommandResult {
	if len(args) > 1 && args[1] == "list" {
		return CommandResult{Stdout: []byte(`[{"id":"project-dependent-view","title":"synthetic active view","status":"open","dependencies":["project-adapter-smoke"]}]`)}
	}
	return r.smokeBeadsRunner.Run(ctx, name, args...)
}
