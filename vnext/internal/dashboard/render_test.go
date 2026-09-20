package dashboard_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/thebpandey/agent-team/vnext/internal/core"
	dashboard "github.com/thebpandey/agent-team/vnext/internal/dashboard"
	"github.com/thebpandey/agent-team/vnext/internal/store"
)

type receipts struct{ values []dashboard.DashboardRefreshReceipt }

type countedRenderer struct {
	dashboard.Renderer
	publishes int
}

func (r *countedRenderer) Publish(ctx context.Context, input dashboard.Snapshot) error {
	r.publishes++
	return r.Renderer.Publish(ctx, input)
}

func (r *receipts) WriteRefreshReceipt(_ context.Context, value dashboard.DashboardRefreshReceipt) error {
	r.values = append(r.values, value)
	return nil
}

func snapshot() dashboard.Snapshot {
	return dashboard.Snapshot{
		Schema: 1, Project: "project", RunID: "run", CanonicalRevision: "rev", LastIntegration: "task",
		Status: dashboard.Current, GeneratedAt: "2026-09-19T00:00:00Z",
		Tasks: []dashboard.TaskSummary{{ID: "task", State: "ready", Next: "run"}},
		Teams: []dashboard.TeamSummary{{ID: "team", State: "active", QueueFingerprint: "queue", Tasks: []string{"task"}}},
		Resources: core.ResourceSnapshot{Servers: []string{"http://localhost:3000"}}, Evidence: []string{"evidence:one"},
	}
}

func TestPublishEscapesAndWritesOneStaticFile(t *testing.T) {
	root := t.TempDir()
	renderer := dashboard.NewRenderer(store.New(root, core.StorageLimits{CanonicalBytes: 16 << 20}))
	input := snapshot()
	input.Project = `<img src=x onerror=alert(1)>`
	input.Tasks[0].Next = `"quoted" & <tag>`
	if err := renderer.Publish(context.Background(), input); err != nil { t.Fatal(err) }
	path := filepath.Join(root, ".agent-team", "dashboard", "index.html")
	got, err := os.ReadFile(path)
	if err != nil { t.Fatal(err) }
	text := string(got)
	if strings.Contains(text, `<img src=x`) || strings.Contains(text, `"quoted" & <tag>`) { t.Fatalf("unescaped dashboard: %s", text) }
	if !strings.Contains(text, "&lt;img src=x onerror=alert(1)&gt;") || !strings.Contains(text, "&#34;quoted&#34; &amp; &lt;tag&gt;") { t.Fatalf("missing escaped content: %s", text) }
	if strings.Contains(text, `src="http`) || strings.Contains(text, `href="http`) || strings.Contains(text, "<script") { t.Fatalf("external or script asset in dashboard: %s", text) }
}

func TestPublishRejectsOutsidePathAndOversizedSnapshot(t *testing.T) {
	root := t.TempDir()
	renderer := dashboard.NewRenderer(store.New(root, core.StorageLimits{CanonicalBytes: 16 << 20}))
	input := snapshot()
	input.Project = "../../outside"
	if err := renderer.Publish(context.Background(), input); err != nil { t.Fatal(err) }
	if _, err := os.Stat(filepath.Join(root, ".agent-team", "dashboard", "index.html")); err != nil { t.Fatalf("fixed contained path missing: %v", err) }
	if _, err := os.Stat(filepath.Join(root, "outside")); !errors.Is(err, os.ErrNotExist) { t.Fatalf("input escaped dashboard path: %v", err) }
	input.Project = strings.Repeat("x", 1<<20)
	if err := renderer.Publish(context.Background(), input); err == nil { t.Fatal("oversized snapshot accepted") }
	input = snapshot()
	input.Evidence = make([]string, 257)
	if err := renderer.Publish(context.Background(), input); err == nil { t.Fatal("too many rows accepted") }
}

func TestObserverPublishesOnceAndRetainsLastGoodOnFailure(t *testing.T) {
	root := t.TempDir()
	renderer := &countedRenderer{Renderer: dashboard.NewRenderer(store.New(root, core.StorageLimits{CanonicalBytes: 16 << 20}))}
	receipt := &receipts{}
	observer := dashboard.NewIntegrationObserver(renderer, receipt)
	input := snapshot()
	result := dashboard.IntegrationResult{Success: true, RunID: "run", TaskID: "task", CanonicalRevision: "rev", Revision: 2}
	if err := observer.AfterIntegration(context.Background(), result, input); err != nil { t.Fatal(err) }
	if renderer.publishes != 1 { t.Fatalf("publishes = %d, want 1", renderer.publishes) }
	path := filepath.Join(root, ".agent-team", "dashboard", "index.html")
	lastGood, err := os.ReadFile(path)
	if err != nil { t.Fatal(err) }
	if err := renderer.Publish(context.Background(), input); err != nil { t.Fatal(err) }
	deterministic, err := os.ReadFile(path)
	if err != nil || string(deterministic) != string(lastGood) { t.Fatalf("non-deterministic publication: %v", err) }
	lastGood = deterministic
	if len(receipt.values) != 1 || !receipt.values[0].Success || receipt.values[0].Status != dashboard.Current { t.Fatalf("current receipt: %#v", receipt.values) }
	if err := observer.AfterIntegration(context.Background(), dashboard.IntegrationResult{Success: false, RunID: "run", TaskID: "task", Error: "integration failed", Revision: 3}, input); err != nil { t.Fatal(err) }
	if renderer.publishes != 2 { t.Fatalf("integration failure published %d times", renderer.publishes) }
	if len(receipt.values) != 2 || receipt.values[1].Success || receipt.values[1].Status != dashboard.Stale { t.Fatalf("stale receipt: %#v", receipt.values) }
	afterIntegrationFailure, err := os.ReadFile(path)
	if err != nil || string(afterIntegrationFailure) != string(lastGood) { t.Fatalf("last-good changed after integration failure: %v", err) }
	input.Project = strings.Repeat("x", 1<<20)
	if err := observer.AfterIntegration(context.Background(), result, input); err != nil { t.Fatal(err) }
	if renderer.publishes != 3 { t.Fatalf("render failure did not make exactly one publish attempt: %d", renderer.publishes) }
	if len(receipt.values) != 3 || receipt.values[2].Success || receipt.values[2].Status != dashboard.Unavailable { t.Fatalf("unavailable receipt: %#v", receipt.values) }
	afterRenderFailure, err := os.ReadFile(path)
	if err != nil || string(afterRenderFailure) != string(lastGood) { t.Fatalf("last-good changed after render failure: %v", err) }
	writeFailing := dashboard.NewIntegrationObserver(dashboard.NewRenderer(store.New(root, core.StorageLimits{CanonicalBytes: 1})), receipt)
	if err := writeFailing.AfterIntegration(context.Background(), result, snapshot()); err != nil { t.Fatal(err) }
	if len(receipt.values) != 4 || receipt.values[3].Success || receipt.values[3].Status != dashboard.Unavailable { t.Fatalf("write failure receipt: %#v", receipt.values) }
	afterWriteFailure, err := os.ReadFile(path)
	if err != nil || string(afterWriteFailure) != string(lastGood) { t.Fatalf("last-good changed after write failure: %v", err) }
}
