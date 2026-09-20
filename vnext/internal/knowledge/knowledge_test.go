package knowledge

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

func envelope(run string, revision uint64) core.RecordEnvelope {
	return core.RecordEnvelope{Schema: 1, Project: "project", RunID: core.RunID(run), WrittenAt: "2026-09-19T00:00:00Z", Revision: revision}
}

func TestKnowledgeRejectsIncompleteProvenanceAndOversizedHandoff(t *testing.T) {
	ctx := context.Background()
	s := store.New(t.TempDir(), core.StorageLimits{CanonicalBytes: 16 << 20, HandoffHardBytes: 256 << 10})
	if _, err := AppendDecision(ctx, s, Decision{RecordEnvelope: core.RecordEnvelope{Schema: 1, RunID: "RUN-1", Revision: 1}, Summary: "missing", Rationale: "missing"}); !errors.Is(err, core.ErrRevision) {
		t.Fatalf("incomplete decision envelope = %v", err)
	}
	bs := NewBlockerStore(s)
	b, err := bs.Create(ctx, Blocker{RecordEnvelope: envelope("RUN-1", 1), ID: "BLK-1", Severity: "high", State: "open"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(s.Root, "DECISIONS.md"), []byte("# Decisions\n\n## DEC-000001\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := bs.Resolve(ctx, b.ID, "DEC-000001"); !errors.Is(err, core.ErrRevision) {
		t.Fatalf("heading-only provenance = %v", err)
	}
	_, err = DeriveHandoff(HandoffSnapshot{RecordEnvelope: envelope("RUN-1", 1), NextAction: strings.Repeat("x", 4097), Freshness: "fresh"})
	if !errors.Is(err, core.ErrLimit) {
		t.Fatalf("oversized handoff input = %v", err)
	}
}

func TestEscapedDecisionProvenanceAndHandoffEnums(t *testing.T) {
	ctx := context.Background()
	s := store.New(t.TempDir(), core.StorageLimits{CanonicalBytes: 16 << 20})
	decision, err := AppendDecision(ctx, s, Decision{RecordEnvelope: envelope("RUN_1", 1), Summary: "keep _ escaped", Rationale: "parser round trip"})
	if err != nil {
		t.Fatal(err)
	}
	if err := PromoteRule(ctx, s, Rule{RecordEnvelope: envelope("RUN_1", 1), ID: "RULE-1", Text: "use facts", DecisionID: decision}); err != nil {
		t.Fatal(err)
	}
	_, err = DeriveHandoff(HandoffSnapshot{RecordEnvelope: envelope("RUN-1", 1), NextAction: "wait", Freshness: "fresh", Blockers: []Blocker{{ID: "BLK-1", State: "closed"}}})
	if !errors.Is(err, core.ErrRevision) {
		t.Fatalf("invalid blocker state = %v", err)
	}
}

func TestKnowledgeRecordsRed(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	s := store.New(root, core.StorageLimits{CanonicalBytes: 16 << 20, HandoffWarnBytes: 192 << 10, HandoffHardBytes: 256 << 10})

	r := Receipt{RecordEnvelope: envelope("RUN-1", 1), Team: "TEAM-1", Task: "TASK-1", Attempt: 1, State: core.Working, NextAction: "review"}
	if err := WriteReceipt(ctx, s, r); err != nil {
		t.Fatal(err)
	}
	if err := WriteEvidence(ctx, s, Evidence{RecordEnvelope: envelope("RUN-1", 1), Task: "TASK-1", Attempt: 1, Exit: 0, InputFingerprint: "abc", CommandPointer: "logs/command.txt", OutputPointer: "logs/output.txt"}); err != nil {
		t.Fatal(err)
	}
	if err := WriteEvidence(ctx, s, Evidence{RecordEnvelope: envelope("RUN-1", 1), Task: "TASK-1", Attempt: 1, Exit: 1, InputFingerprint: "abc"}); !errors.Is(err, ErrConflict) {
		t.Fatalf("conflicting immutable evidence = %v", err)
	}
	first, err := AppendDecision(ctx, s, Decision{RecordEnvelope: envelope("RUN-1", 1), Summary: "accept format", Rationale: "portable"})
	if err != nil || !strings.HasPrefix(first, "DEC-") {
		t.Fatalf("decision = %q, %v", first, err)
	}
	second, err := AppendDecision(ctx, s, Decision{RecordEnvelope: envelope("RUN-1", 2), Summary: "accept tests", Rationale: "bounded"})
	if err != nil || first == second {
		t.Fatalf("decision sequence = %q, %q, %v", first, second, err)
	}
	if err := PromoteRule(ctx, s, Rule{RecordEnvelope: envelope("RUN-1", 1), ID: "RULE-1", Text: "Use records.", DecisionID: first}); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(root, ".agent-team", "receipts", "TEAM-1.json")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, ".agent-team", "evidence", "TASK-1", "1", "evidence.json")); err != nil {
		t.Fatal(err)
	}
}

func TestReceiptStoresOnlyResourceReferences(t *testing.T) {
	ctx := context.Background()
	s := store.New(t.TempDir(), core.StorageLimits{CanonicalBytes: 16 << 20})
	r := Receipt{
		RecordEnvelope: envelope("RUN-1", 1), Team: "TEAM-1", Task: "TASK-1", Attempt: 1, State: core.Working, NextAction: "review",
		Resources: core.ResourceSnapshot{Servers: []string{"S-1"}, Browsers: []string{"B-1"}, External: []string{"evidence/TASK-1/1/browser.json"}},
	}
	if err := WriteReceipt(ctx, s, r); err != nil {
		t.Fatal(err)
	}
	var got Receipt
	if err := s.ReadJSON(".agent-team/receipts/TEAM-1.json", 16<<20, &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Resources.Servers) != 1 || got.Resources.Servers[0] != "S-1" || len(got.Resources.Browsers) != 1 || got.Resources.Browsers[0] != "B-1" {
		t.Fatalf("resource references = %#v", got.Resources)
	}
}

func TestBlockerCASAndProjectionRed(t *testing.T) {
	ctx := context.Background()
	s := store.New(t.TempDir(), core.StorageLimits{CanonicalBytes: 16 << 20, HandoffHardBytes: 256 << 10})
	bs := NewBlockerStore(s)
	b, err := bs.Create(ctx, Blocker{RecordEnvelope: envelope("RUN-1", 1), ID: "BLK-1", Severity: "high", State: "open", Affected: []string{"TASK-1"}, EvidencePointer: "evidence/TASK-1/1/evidence.json"})
	if err != nil {
		t.Fatal(err)
	}
	stale := b
	b.Severity = "critical"
	b, err = bs.Update(ctx, b)
	if err != nil || b.Revision != 2 {
		t.Fatalf("update = %#v, %v", b, err)
	}
	if _, err := bs.Update(ctx, stale); !errors.Is(err, core.ErrRevision) {
		t.Fatalf("stale update = %v", err)
	}
	decision, err := AppendDecision(ctx, s, Decision{RecordEnvelope: envelope("RUN-1", 1), Summary: "resolve blocker", Rationale: "evidence reviewed"})
	if err != nil {
		t.Fatal(err)
	}
	if err := bs.Resolve(ctx, b.ID, decision); err != nil {
		t.Fatal(err)
	}
	p := NewProjectionWriter(s)
	items, err := p.RegenerateBlockers(ctx, nil, nil, core.ResourceSnapshot{})
	if err != nil || len(items) != 0 {
		t.Fatalf("items=%#v err=%v", items, err)
	}
	data, err := os.ReadFile(filepath.Join(s.Root, "BLOCKERS.md"))
	if err != nil || strings.Contains(string(data), "BLK-1") {
		t.Fatalf("projection = %q, %v", data, err)
	}
}

func TestHandoffAndPacketRed(t *testing.T) {
	ctx := context.Background()
	s := store.New(t.TempDir(), core.StorageLimits{CanonicalBytes: 16 << 20, HandoffWarnBytes: 32, HandoffHardBytes: 256 << 10})
	snapshot := HandoffSnapshot{RecordEnvelope: envelope("RUN-1", 3), Team: "TEAM-1", Gates: []string{"format: pass"}, Reviews: []string{"review: pass"}, Resources: core.ResourceSnapshot{Servers: []string{"server-1"}}, Decisions: []Decision{{ID: "DEC-000001", Summary: "keep fact"}}, Blockers: []Blocker{{ID: "BLK-2", State: "open", Severity: "low"}}, NextAction: "integrate", Freshness: "fresh"}
	data, err := DeriveHandoff(snapshot)
	if err != nil || !strings.Contains(string(data), "RUN-1") || !strings.Contains(string(data), "not authority") {
		t.Fatalf("handoff = %q, %v", data, err)
	}
	p := NewProjectionWriter(s)
	if _, err := p.WriteHandoff(ctx, snapshot); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(s.Root, ".agent-team", "handoffs", "RUN-1.md")); err != nil {
		t.Fatal(err)
	}

	packet := core.AssignmentPacket{RecordEnvelope: envelope("RUN-1", 1), Task: "TASK-1", Team: "TEAM-1", SpecRevision: "abc", QueueFingerprint: "queue", Owner: "owner", Worktree: "worktree", Base: "base", NextAction: "implement"}
	digest, err := PacketDigest(packet)
	if err != nil || !strings.HasPrefix(digest, "sha256:") {
		t.Fatalf("digest = %q, %v", digest, err)
	}
	if err := ValidatePacket(packet, digest); err != nil {
		t.Fatal(err)
	}
	packet.Owner = "changed"
	if err := ValidatePacket(packet, digest); !errors.Is(err, ErrConflict) {
		t.Fatalf("mutated packet = %v", err)
	}
}
