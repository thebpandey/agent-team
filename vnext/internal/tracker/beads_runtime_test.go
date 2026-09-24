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

// bd 1.2.2 emits optional top-level keys (design, external_ref, mol_type,
// spec_id) only for issues that set them; later bd versions will add more.
// They carry no task authority, so the reader must ignore them, including
// when null, while still rejecting a null or malformed authority field.
func TestBeadsIgnoresUnknownObservationalTopLevelFields(t *testing.T) {
	row := `{"id":"docs-378","title":"epic","status":"open","issue_type":"epic","priority":0,` +
		`"design":"Design notes","external_ref":"TASK-001","mol_type":"swarm","spec_id":"spec-1",` +
		`"future_field":{"nested":true},"future_null":null,"dependencies":[]}`
	tasks, err := parseBeads([]byte("[" + row + "]"))
	if err != nil || len(tasks) != 1 || tasks[0].ID != "docs-378" || tasks[0].Objective != "epic" || tasks[0].State != core.Ready {
		t.Fatalf("bd 1.2.2 row must parse with authority intact: tasks=%+v err=%v", tasks, err)
	}
	for _, bad := range []string{
		`{"id":"B-1","title":"task","status":null,"design":"x"}`,
		`{"id":null,"title":"task","status":"open","spec_id":"s"}`,
		`{"id":"B-1","title":"task","status":"open","dependencies":"not-a-list","external_ref":"r"}`,
	} {
		if _, err := parseBeads([]byte("[" + bad + "]")); err == nil {
			t.Fatalf("unknown fields must not relax authority validation: %s", bad)
		}
	}
}

// bd 1.2.2 has ten relation types; only "blocks" gates readiness. Supported
// provenance relations must be ignored, while relation identity stays strict.
func TestBeadsIgnoresNonBlockingDependencyRelations(t *testing.T) {
	deps := `[{"issue_id":"docs-e1vi.4","depends_on_id":"docs-e1vi","type":"parent-child","created_at":"2026-08-23T21:19:28Z","created_by":"thebpandey","metadata":"{}"},` +
		`{"issue_id":"docs-e1vi.4","depends_on_id":"docs-x","type":"discovered-from"},` +
		`{"issue_id":"docs-e1vi.4","depends_on_id":"docs-b","type":"blocks"}]`
	tasks, err := parseBeads([]byte(`[{"id":"docs-e1vi.4","title":"child","status":"open","dependencies":` + deps + `}]`))
	if err != nil || len(tasks) != 1 || len(tasks[0].Dependencies) != 1 || tasks[0].Dependencies[0] != "docs-b" {
		t.Fatalf("only the blocks relation is a dependency: tasks=%+v err=%v", tasks, err)
	}
	for _, bad := range []string{
		`[{"issue_id":null,"depends_on_id":"b","type":"blocks"}]`,
		`[{"issue_id":"a","depends_on_id":null,"type":"blocks"}]`,
		`[{"issue_id":"a","depends_on_id":"b","type":null}]`,
		`[{"issue_id":"a","depends_on_id":"b"}]`,
		`[{"depends_on_id":"b","type":"blocks"}]`,
		`[{"issue_id":"a","type":"parent-child"}]`,
		`[{"issue_id":"","depends_on_id":"b","type":"blocks"}]`,
		`[{"depends_on_id":"","type":"blocks"}]`,
	} {
		if _, err := parseBeads([]byte(`[{"id":"a","title":"t","status":"open","dependencies":` + bad + `}]`)); err == nil {
			t.Fatalf("relation identity must stay strict: %s", bad)
		}
	}
}

func TestBeadsParsesExpandedDependencyIssueProjections(t *testing.T) {
	deps := `[{"acceptance_criteria":null,"assignee":null,"close_reason":"done","closed_at":"2026-09-22T19:53:30Z",` +
		`"created_at":"2026-09-22T18:00:00Z","created_by":"thebpandey","dependency_type":"blocks",` +
		`"description":"required first","design":null,"id":"docs-prerequisite","issue_type":"task","labels":["release"],` +
		`"notes":null,"owner":"team","priority":1,"spec_id":null,"started_at":null,"status":"closed",` +
		`"title":"prerequisite","updated_at":"2026-09-22T19:53:30Z","future_field":null},` +
		`{"id":"docs-parent","dependency_type":"parent-child","acceptance_criteria":"observational"}]`
	tasks, err := parseBeads([]byte(`[{"id":"docs-child","title":"child","status":"open","dependencies":` + deps + `}]`))
	if err != nil || len(tasks) != 1 || len(tasks[0].Dependencies) != 1 || tasks[0].Dependencies[0] != "docs-prerequisite" {
		t.Fatalf("expanded blocks projection must supply the prerequisite ID: tasks=%+v err=%v", tasks, err)
	}
}

func TestBeadsRejectsAmbiguousOrMalformedExpandedDependencies(t *testing.T) {
	for name, bad := range map[string]string{
		"mixed issue identity":    `[{"id":"b","dependency_type":"blocks","issue_id":"a"}]`,
		"mixed target identity":   `[{"id":"b","dependency_type":"blocks","depends_on_id":"b"}]`,
		"mixed relation type":     `[{"id":"b","dependency_type":"blocks","type":"blocks"}]`,
		"null id":                 `[{"id":null,"dependency_type":"blocks"}]`,
		"empty id":                `[{"id":"","dependency_type":"blocks"}]`,
		"null dependency type":    `[{"id":"b","dependency_type":null}]`,
		"empty dependency type":   `[{"id":"b","dependency_type":""}]`,
		"unknown dependency type": `[{"id":"b","dependency_type":"future-relation"}]`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := parseBeads([]byte(`[{"id":"a","title":"t","status":"open","dependencies":` + bad + `}]`)); err == nil {
				t.Fatalf("ambiguous or malformed expanded relation must be rejected: %s", bad)
			}
		})
	}
}

func TestBeadsRejectsUnknownDependencyRelationShape(t *testing.T) {
	for name, bad := range map[string]string{
		"type": `[{"issue_id":"a","depends_on_id":"b","type":"future-relation"}]`,
		"key":  `[{"issue_id":"a","depends_on_id":"b","type":"related","future_key":1}]`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := parseBeads([]byte(`[{"id":"a","title":"t","status":"open","dependencies":` + bad + `}]`)); err == nil {
				t.Fatalf("unknown relation shape must be rejected: %s", bad)
			}
		})
	}
}
