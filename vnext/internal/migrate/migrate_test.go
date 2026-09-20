package migrate_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/thebpandey/agent-team/vnext/internal/core"
	"github.com/thebpandey/agent-team/vnext/internal/install"
	"github.com/thebpandey/agent-team/vnext/internal/migrate"
)

type fakeRunner struct{}

func (fakeRunner) Observe(_ context.Context, project string, host install.Host) (migrate.Observation, error) {
	return migrate.Observation{Host: host, Identity: string(host) + "-identity", Revision: "git-1", TrackerDigest: "tracker-1", ReceiptDigest: project + "-receipt", State: "ready"}, nil
}

func digestFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return fmt.Sprintf("%x", sha256.Sum256(b))
}

func TestInventoryIsRedactedAndBothHostsCanaryWithoutTransfer(t *testing.T) {
	project := t.TempDir()
	legacy := filepath.Join(project, "v7-state.json")
	if err := os.WriteFile(legacy, []byte(`{"lease":"secret","owner":"v7"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	inv, err := migrate.InventoryV7(context.Background(), project, legacy)
	if err != nil || inv.Digest == "" || len(inv.Files) != 1 {
		t.Fatal(inv, err)
	}
	store := migrate.NewStore(project)
	codex, err := migrate.BeginCanary(context.Background(), store, project, install.Codex, true, fakeRunner{})
	if err != nil {
		t.Fatal(err)
	}
	codexResumed, err := migrate.ResumeCanary(context.Background(), store, codex.ID, install.Codex, codex.Revision, codex.V7InventoryDigest, fakeRunner{})
	if err != nil {
		t.Fatal(err)
	}
	duplicate, err := migrate.ResumeCanary(context.Background(), store, codex.ID, install.Codex, codexResumed.Revision, codex.V7InventoryDigest, fakeRunner{})
	if err != nil || !duplicate.Idempotent || duplicate.Revision != codexResumed.Revision {
		t.Fatal(duplicate, err)
	}
	claude, err := migrate.ResumeCanary(context.Background(), store, codex.ID, install.Claude, duplicate.Revision, codex.V7InventoryDigest, fakeRunner{})
	if err != nil {
		t.Fatal(err)
	}
	if claude.Codex.Identity == claude.Claude.Identity || claude.Codex.Host == claude.Claude.Host {
		t.Fatal(claude)
	}
	raw, err := os.ReadFile(filepath.Join(project, ".agent-team", "migration", "canary-"+codex.ID+".json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "secret") || strings.Contains(string(raw), "lease") {
		t.Fatal("v7 claim copied")
	}
	var record migrate.CanaryRecord
	if json.Unmarshal(raw, &record) != nil || record.State != "ready" {
		t.Fatal(string(raw))
	}
	if got, _ := os.ReadFile(legacy); string(got) != `{"lease":"secret","owner":"v7"}` {
		t.Fatal("legacy changed")
	}
}

func TestDeclinedCanaryWritesNothing(t *testing.T) {
	project := t.TempDir()
	_, err := migrate.BeginCanary(context.Background(), migrate.NewStore(project), project, install.Codex, false, fakeRunner{})
	if err == nil {
		t.Fatal("declined canary accepted")
	}
}

type failedRunner struct{}

func (failedRunner) Observe(context.Context, string, install.Host) (migrate.Observation, error) {
	return migrate.Observation{}, errors.New("host observation failed")
}

func TestFailedObservationAndRollbackPreserveLegacy(t *testing.T) {
	project := t.TempDir()
	legacy := filepath.Join(project, "v7-state.json")
	original := []byte(`{"lease":"secret","owner":"v7"}`)
	if err := os.WriteFile(legacy, original, 0o600); err != nil {
		t.Fatal(err)
	}
	store := migrate.NewStore(project)
	if _, err := migrate.BeginCanary(context.Background(), store, project, install.Codex, true, failedRunner{}); err == nil {
		t.Fatal("failed observation accepted")
	}
	record, err := migrate.BeginCanary(context.Background(), store, project, install.Codex, true, fakeRunner{})
	if err != nil {
		t.Fatal(err)
	}
	cleanPath := filepath.Join(project, ".agent-team", "migration", "clean-owned.txt")
	changedPath := filepath.Join(project, ".agent-team", "migration", "changed-owned.txt")
	if err := os.WriteFile(cleanPath, []byte("owned"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(changedPath, []byte("owned"), 0o600); err != nil {
		t.Fatal(err)
	}
	record.Owned = []migrate.CanaryArtifact{{Path: cleanPath, SHA256: digestFile(t, cleanPath)}, {Path: changedPath, SHA256: digestFile(t, changedPath)}}
	record, err = store.CompareAndSwap(context.Background(), record.Revision, record)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(changedPath, []byte("user change"), 0o600); err != nil {
		t.Fatal(err)
	}
	rolledBack, err := migrate.RollbackCanary(context.Background(), store, record.ID)
	if err != nil {
		t.Fatal(err)
	}
	if rolledBack.State != "rolled_back" || rolledBack.RollbackEvidence == "" || len(rolledBack.Retained) != 1 || rolledBack.Retained[0] != changedPath {
		t.Fatal(rolledBack)
	}
	if _, err := os.Stat(cleanPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("unchanged canary artifact remains err=%v", err)
	}
	if got, _ := os.ReadFile(changedPath); string(got) != "user change" {
		t.Fatalf("changed canary artifact=%q", got)
	}
	raw, err := os.ReadFile(filepath.Join(project, ".agent-team", "migration", "canary-"+record.ID+".json"))
	if err != nil {
		t.Fatal(err)
	}
	var persisted migrate.CanaryRecord
	if err := json.Unmarshal(raw, &persisted); err != nil || persisted.State != "rolled_back" || persisted.RollbackEvidence == "" {
		t.Fatalf("rollback evidence not durable: %s err=%v", raw, err)
	}
	if got, _ := os.ReadFile(legacy); !bytes.Equal(got, original) {
		t.Fatal("rollback changed v7 bytes")
	}
}

func TestCanaryResumeRejectsStaleOrMismatchedSnapshot(t *testing.T) {
	project := t.TempDir()
	store := migrate.NewStore(project)
	record, err := migrate.BeginCanary(context.Background(), store, project, install.Codex, true, fakeRunner{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := migrate.ResumeCanary(context.Background(), store, record.ID, install.Codex, record.Revision, "wrong-inventory-digest", fakeRunner{}); err == nil {
		t.Fatal("mismatched inventory accepted")
	}
	current, err := migrate.ResumeCanary(context.Background(), store, record.ID, install.Codex, record.Revision, record.V7InventoryDigest, fakeRunner{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := migrate.ResumeCanary(context.Background(), store, record.ID, install.Claude, record.Revision, record.V7InventoryDigest, fakeRunner{}); !errors.Is(err, core.ErrRevision) {
		t.Fatal("stale resume accepted", err)
	}
	if current.Revision <= record.Revision {
		t.Fatal(current)
	}
}

func TestInventoryRejectsPathsOutsideProject(t *testing.T) {
	project := t.TempDir()
	outside := filepath.Join(t.TempDir(), "v7-state.json")
	if err := os.WriteFile(outside, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := migrate.InventoryV7(context.Background(), project, outside); !errors.Is(err, core.ErrPath) {
		t.Fatalf("outside inventory error = %v, want ErrPath", err)
	}
}
