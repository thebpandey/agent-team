package project

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/thebpandey/agent-team/vnext/internal/core"
	"github.com/thebpandey/agent-team/vnext/internal/migrate"
	"github.com/thebpandey/agent-team/vnext/internal/store"
	"github.com/thebpandey/agent-team/vnext/internal/testkit"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func roleSettingsStore(t *testing.T, migrated bool) *store.Store {
	t.Helper()
	root := testkit.GitRepo(t)
	st := store.New(root, core.DefaultConfig().Storage)
	if !migrated {
		var artifacts []ArtifactDecision
		for _, name := range []string{"TASKS.md", "DECISIONS.md", "AGENT_TEAM_RULES.md"} {
			if err := os.WriteFile(filepath.Join(root, name), []byte("# Fixture\n"), 0600); err != nil {
				t.Fatal(err)
			}
			artifacts = append(artifacts, ArtifactDecision{Path: name, Mode: ExistingArtifact, Confirmation: Approved})
		}
		if _, err := NewSetupService(st).Initialize(context.Background(), SetupInput{Root: root, Mode: PlanMode, Artifacts: artifacts}); err != nil {
			t.Fatal(err)
		}
		return st
	}
	receipt := migrate.AuthorityReceipt{Schema: 1, Project: root, OperationID: "cutover", RequestDigest: "request", TargetRevision: strings.Repeat("a", 40), ApprovalID: "approval", ApprovalSHA256: strings.Repeat("b", 64), Tracker: migrate.TrackerAuthority{Fingerprint: strings.Repeat("c", 64), ParentID: "TASK-1", TaskIDs: []string{"TASK-1"}, TaskCount: 1}}
	encoded, _ := json.Marshal(receipt)
	sum := sha256.Sum256(encoded)
	receipt.ReceiptDigest = hex.EncodeToString(sum[:])
	for path, value := range map[string]any{".agent-team/v8/authority.json": receipt, ".agent-team/setup.json": map[string]any{"schema": 1, "authority": "v8", "project": root, "revision": receipt.TargetRevision, "tracker": map[string]any{"kind": "beads", "fingerprint": receipt.Tracker.Fingerprint, "parentId": receipt.Tracker.ParentID}, "receiptPath": ".agent-team/v8/authority.json", "receiptDigest": receipt.ReceiptDigest}} {
		if _, err := st.WriteJSON(path, value, core.DefaultConfig().Storage.CanonicalBytes); err != nil {
			t.Fatal(err)
		}
	}
	return st
}

func TestRoleSettingsPersistBothHostsAcrossAuthorityBindings(t *testing.T) {
	for _, migrated := range []bool{false, true} {
		t.Run(map[bool]string{false: "fresh", true: "migrated"}[migrated], func(t *testing.T) {
			st := roleSettingsStore(t, migrated)
			service := NewSettingsService(st)
			initial, err := service.Inspect(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			encoded, _ := json.Marshal(initial)
			var view struct {
				Hosts map[string]struct {
					Roles map[string]RoleProfile `json:"roles"`
				} `json:"hosts"`
			}
			if err := json.Unmarshal(encoded, &view); err != nil {
				t.Fatal(err)
			}
			for _, host := range []string{"codex", "claude"} {
				for _, role := range []string{"orchestrator", "developer", "reviewer", "visual_reviewer"} {
					profile, ok := view.Hosts[host].Roles[role]
					if !ok || profile != defaultRoleProfile(host, role) {
						t.Fatalf("missing current default profile %s.%s: %s", host, role, encoded)
					}
				}
			}
			if _, err := os.Stat(filepath.Join(st.Root, settingsPath)); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("inspect wrote settings: %v", err)
			}
			updates := map[string]string{}
			for _, host := range []string{"codex", "claude"} {
				for _, role := range []string{"orchestrator", "developer", "reviewer", "visual_reviewer"} {
					updates[host+"."+role+".model"] = host + "-" + role
					updates[host+"."+role+".effort"] = "high"
				}
			}
			saved, err := service.Update(context.Background(), updates)
			if err != nil {
				t.Fatal(err)
			}
			again, err := service.Inspect(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			encoded, _ = json.Marshal(again)
			if err := json.Unmarshal(encoded, &view); err != nil {
				t.Fatal(err)
			}
			for _, host := range []string{"codex", "claude"} {
				for _, role := range []string{"orchestrator", "developer", "reviewer", "visual_reviewer"} {
					if got := view.Hosts[host].Roles[role]; got.Model != host+"-"+role || got.Effort != "high" {
						t.Fatalf("wrong profile %s.%s: %#v", host, role, got)
					}
				}
			}
			if saved.CodexDeveloper.Model != "codex-developer" || again.CodexDeveloper != saved.CodexDeveloper {
				t.Fatal("legacy profile lost")
			}
		})
	}
}

func TestRoleSettingsPreserveUnknownFieldsAndClearOverrides(t *testing.T) {
	st := roleSettingsStore(t, false)
	service := NewSettingsService(st)
	if _, err := service.Update(context.Background(), map[string]string{"parallel_teams": "2"}); err != nil {
		t.Fatal(err)
	}
	var doc map[string]json.RawMessage
	if err := st.ReadJSON(settingsPath, core.DefaultConfig().Storage.CanonicalBytes, &doc); err != nil {
		t.Fatal(err)
	}
	doc["custom"] = json.RawMessage(`{"keep":9007199254740993}`)
	doc["hosts"] = json.RawMessage(`{"future":"opaque-extension","claude":{"custom":true,"roles":{"future":{"custom":true},"developer":{"model":"old","effort":"high","custom":9007199254740993}}}}`)
	if _, err := st.WriteJSON(settingsPath, doc, core.DefaultConfig().Storage.CanonicalBytes); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Update(context.Background(), map[string]string{"claude.coder.model": "new-model", "codex.reviewer.model": "review-model"}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Update(context.Background(), map[string]string{"claude.developer.effort": "inherit"}); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(st.Root, settingsPath))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`9007199254740993`, `"opaque-extension"`, `"custom"`, `"new-model"`, `"review-model"`} {
		if !bytes.Contains(raw, []byte(want)) {
			t.Fatalf("lost %s: %s", want, raw)
		}
	}
	result, err := service.Inspect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got := result.Profile("claude", "coder"); got.Model != "new-model" || got.Effort != "" {
		t.Fatalf("clear changed other field: %#v", got)
	}
	for _, updates := range []map[string]string{{"claude.coder.model": "a", "claude.developer.model": "b"}, {"codex.reviewer.effort": "bad value"}, {"unknown.developer.model": "x"}, {"claude.unknown.model": "x"}} {
		if _, err := service.Update(context.Background(), updates); !errors.Is(err, core.ErrSettings) {
			t.Fatalf("accepted bad update %v: %v", updates, err)
		}
		after, _ := os.ReadFile(filepath.Join(st.Root, settingsPath))
		if !bytes.Equal(raw, after) {
			t.Fatal("bad update wrote settings")
		}
	}
}

func TestRoleSettingsRejectMalformedKnownShapesWithoutWriting(t *testing.T) {
	for _, hosts := range []string{`null`, `[]`, `{"claude":null}`, `{"claude":{"roles":[]}}`, `{"claude":{"roles":{"reviewer":null}}}`, `{"codex":{"roles":{"orchestrator":{"model":12}}}}`, `{"claude":{"roles":{"visual_reviewer":{"effort":null}}}}`} {
		t.Run(hosts, func(t *testing.T) {
			st := roleSettingsStore(t, false)
			service := NewSettingsService(st)
			if _, err := service.Update(context.Background(), map[string]string{"parallel_teams": "2"}); err != nil {
				t.Fatal(err)
			}
			var doc map[string]json.RawMessage
			if err := st.ReadJSON(settingsPath, core.DefaultConfig().Storage.CanonicalBytes, &doc); err != nil {
				t.Fatal(err)
			}
			doc["hosts"] = json.RawMessage(hosts)
			if _, err := st.WriteJSON(settingsPath, doc, core.DefaultConfig().Storage.CanonicalBytes); err != nil {
				t.Fatal(err)
			}
			before, _ := os.ReadFile(filepath.Join(st.Root, settingsPath))
			if _, err := service.Inspect(context.Background()); !errors.Is(err, core.ErrRevision) {
				t.Fatalf("inspect accepted malformed profile: %v", err)
			}
			if _, err := service.Update(context.Background(), map[string]string{"continuous": "true"}); !errors.Is(err, core.ErrRevision) {
				t.Fatalf("update accepted malformed profile: %v", err)
			}
			after, _ := os.ReadFile(filepath.Join(st.Root, settingsPath))
			if !bytes.Equal(before, after) {
				t.Fatal("bad profile rewritten")
			}
		})
	}
}
