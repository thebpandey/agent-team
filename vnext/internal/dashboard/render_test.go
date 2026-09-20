package dashboard

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/thebpandey/agent-team/vnext/internal/core"
	"github.com/thebpandey/agent-team/vnext/internal/store"
)

type receipts struct{ values []DashboardRefreshReceipt }

type countedRenderer struct {
	Renderer
	publishes int
}

func (r *countedRenderer) Publish(ctx context.Context, input Snapshot) error {
	r.publishes++
	return r.Renderer.Publish(ctx, input)
}

func (r *receipts) WriteRefreshReceipt(_ context.Context, value DashboardRefreshReceipt) error {
	r.values = append(r.values, value)
	return nil
}

func snapshot() Snapshot {
	return Snapshot{
		Schema: 1, Project: "project", RunID: "run", CanonicalRevision: "rev", LastIntegration: "task",
		Status: Current, GeneratedAt: "2026-09-19T00:00:00Z",
		Tasks:     []TaskSummary{{ID: "task", State: "ready", Next: "run"}},
		Teams:     []TeamSummary{{ID: "team", State: "active", QueueFingerprint: "queue", Tasks: []string{"task"}}},
		Resources: core.ResourceSnapshot{Servers: []string{"http://localhost:3000"}}, Evidence: []string{"evidence:one"},
	}
}

func TestPublishEscapesAndWritesOneStaticFile(t *testing.T) {
	root := t.TempDir()
	renderer := NewRenderer(store.New(root, core.StorageLimits{CanonicalBytes: 16 << 20}))
	input := snapshot()
	input.Project = `<img src=x onerror=alert(1)>`
	input.Tasks[0].Next = `"quoted" & <tag>`
	input.LastIntegration = `<integration>`
	if err := renderer.Publish(context.Background(), input); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, ".agent-team", "dashboard", "index.html")
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(got)
	if strings.Contains(text, `<img src=x`) || strings.Contains(text, `"quoted" & <tag>`) || strings.Contains(text, `<integration>`) {
		t.Fatalf("unescaped dashboard: %s", text)
	}
	if !strings.Contains(text, "&lt;img src=x onerror=alert(1)&gt;") || !strings.Contains(text, "&#34;quoted&#34; &amp; &lt;tag&gt;") || !strings.Contains(text, "<h2>Integrations</h2><p>&lt;integration&gt;</p>") {
		t.Fatalf("missing escaped content: %s", text)
	}
	if strings.Contains(text, `src="http`) || strings.Contains(text, `href="http`) || strings.Contains(text, "<script") {
		t.Fatalf("external or script asset in dashboard: %s", text)
	}
}

func TestPublishContainsPathAndRejectsBoundedInput(t *testing.T) {
	root := t.TempDir()
	renderer := NewRenderer(store.New(root, core.StorageLimits{CanonicalBytes: 16 << 20}))
	input := snapshot()
	input.Project = "../../outside"
	if err := renderer.Publish(context.Background(), input); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, ".agent-team", "dashboard", "index.html")); err != nil {
		t.Fatalf("fixed contained path missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "outside")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("input escaped dashboard path: %v", err)
	}
	input.Project = strings.Repeat("x", 1<<20)
	if err := renderer.Publish(context.Background(), input); err == nil {
		t.Fatal("oversized validation input accepted")
	}
	input = snapshot()
	input.Evidence = make([]string, 257)
	if err := renderer.Publish(context.Background(), input); err == nil {
		t.Fatal("too many rows accepted")
	}
}

func TestObserverPublishesOnceAndRetainsLastGoodOnFailure(t *testing.T) {
	root := t.TempDir()
	published := &countedRenderer{Renderer: NewRenderer(store.New(root, core.StorageLimits{CanonicalBytes: 16 << 20}))}
	receipt := &receipts{}
	observer := NewIntegrationObserver(published, receipt)
	input := snapshot()
	result := IntegrationResult{Success: true, RunID: "run", TaskID: "task", CanonicalRevision: "rev", Revision: 2}
	if err := observer.AfterIntegration(context.Background(), result, input); err != nil {
		t.Fatal(err)
	}
	if published.publishes != 1 {
		t.Fatalf("publishes = %d, want 1", published.publishes)
	}
	path := filepath.Join(root, ".agent-team", "dashboard", "index.html")
	lastGood, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := published.Publish(context.Background(), input); err != nil {
		t.Fatal(err)
	}
	deterministic, err := os.ReadFile(path)
	if err != nil || string(deterministic) != string(lastGood) {
		t.Fatalf("non-deterministic publication: %v", err)
	}
	lastGood = deterministic
	if len(receipt.values) != 1 || !receipt.values[0].Success || receipt.values[0].Status != Current {
		t.Fatalf("current receipt: %#v", receipt.values)
	}
	if err := observer.AfterIntegration(context.Background(), IntegrationResult{Success: false, RunID: "run", TaskID: "task", Error: "integration failed", Revision: 3}, input); err != nil {
		t.Fatal(err)
	}
	if published.publishes != 2 {
		t.Fatalf("integration failure published %d times", published.publishes)
	}
	if len(receipt.values) != 2 || receipt.values[1].Success || receipt.values[1].Status != Stale {
		t.Fatalf("stale receipt: %#v", receipt.values)
	}
	afterIntegrationFailure, err := os.ReadFile(path)
	if err != nil || string(afterIntegrationFailure) != string(lastGood) {
		t.Fatalf("last-good changed after integration failure: %v", err)
	}
	failing := published.Renderer.(*renderer)
	failing.render = func(Snapshot) ([]byte, error) { return nil, errors.New("render failed") }
	if err := observer.AfterIntegration(context.Background(), result, input); err != nil {
		t.Fatal(err)
	}
	if published.publishes != 3 {
		t.Fatalf("render failure did not make exactly one publish attempt: %d", published.publishes)
	}
	if len(receipt.values) != 3 || receipt.values[2].Success || receipt.values[2].Status != Unavailable {
		t.Fatalf("render failure receipt: %#v", receipt.values)
	}
	afterRenderFailure, err := os.ReadFile(path)
	if err != nil || string(afterRenderFailure) != string(lastGood) {
		t.Fatalf("last-good changed after render failure: %v", err)
	}
	writeFailing := NewIntegrationObserver(NewRenderer(store.New(root, core.StorageLimits{CanonicalBytes: 1})), receipt)
	if err := writeFailing.AfterIntegration(context.Background(), result, snapshot()); err != nil {
		t.Fatal(err)
	}
	if len(receipt.values) != 4 || receipt.values[3].Success || receipt.values[3].Status != Unavailable {
		t.Fatalf("write failure receipt: %#v", receipt.values)
	}
	afterWriteFailure, err := os.ReadFile(path)
	if err != nil || string(afterWriteFailure) != string(lastGood) {
		t.Fatalf("last-good changed after write failure: %v", err)
	}
}
