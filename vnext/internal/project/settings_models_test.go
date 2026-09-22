package project

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/thebpandey/agent-team/vnext/internal/core"
)

func TestRoleModelsUpgradeOnlyKnownHostAliases(t *testing.T) {
	for _, migrated := range []bool{false, true} {
		t.Run(map[bool]string{false: "fresh", true: "migrated"}[migrated], func(t *testing.T) {
			st := roleSettingsStore(t, migrated)
			service := NewSettingsService(st)
			if _, err := service.Update(context.Background(), map[string]string{"parallel_teams": "2"}); err != nil {
				t.Fatal(err)
			}
			var doc map[string]json.RawMessage
			if err := st.ReadJSON(settingsPath, core.DefaultConfig().Storage.CanonicalBytes, &doc); err != nil {
				t.Fatal(err)
			}
			doc["extension"] = json.RawMessage(`{"exact":9007199254740993}`)
			doc["hosts"] = json.RawMessage(`{"custom":{"keep":"gpt-5.6-sol"},"codex":{"custom":true,"roles":{"orchestrator":{"model":"inherit"},"developer":{"model":"gpt-5.6-sol","effort":"high","custom":9007199254740993},"reviewer":{"model":"gateway/gpt-5.6-sol"},"visual_reviewer":{"model":"gpt-5.6-luna","effort":"medium"}}},"claude":{"roles":{"orchestrator":{},"developer":{"model":"claude-opus-5","effort":"high"},"reviewer":{"model":"claude-opus-4-8"},"visual_reviewer":{"model":"gpt-5.6-sol"}}}}`)
			if _, err := st.WriteJSON(settingsPath, doc, core.DefaultConfig().Storage.CanonicalBytes); err != nil {
				t.Fatal(err)
			}
			before, _ := os.ReadFile(filepath.Join(st.Root, settingsPath))
			got, err := service.Inspect(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if len(got.ModelUpdates) != 3 {
				t.Fatalf("normalization not reported: %#v", got.ModelUpdates)
			}
			for key, want := range map[string]string{"codex.developer": "gpt-6-sol", "codex.reviewer": "gateway/gpt-5.6-sol", "codex.visual_reviewer": "gpt-6-luna", "codex.orchestrator": "", "claude.developer": "claude-opus-5-5", "claude.reviewer": "claude-opus-4-8", "claude.visual_reviewer": "gpt-5.6-sol", "claude.orchestrator": ""} {
				parts := strings.Split(key, ".")
				if actual := got.Profile(parts[0], parts[1]).Model; actual != want {
					t.Fatalf("%s=%s want %s", key, actual, want)
				}
			}
			if got.Profile("codex", "developer").Effort != "high" || got.Profile("codex", "visual_reviewer").Effort != "medium" || got.Profile("claude", "developer").Effort != "high" {
				t.Fatal("migration changed effort")
			}
			after, _ := os.ReadFile(filepath.Join(st.Root, settingsPath))
			if string(before) != string(after) {
				t.Fatal("inspect wrote migration")
			}
			if _, err := service.Update(context.Background(), map[string]string{"continuous": "true"}); err != nil {
				t.Fatal(err)
			}
			after, _ = os.ReadFile(filepath.Join(st.Root, settingsPath))
			for _, want := range []string{`gpt-6-sol`, `gpt-6-luna`, `claude-opus-5-5`, `gateway/gpt-5.6-sol`, `claude-opus-4-8`, `9007199254740993`, `"custom"`} {
				if !strings.Contains(string(after), want) {
					t.Fatalf("lost %s: %s", want, after)
				}
			}
			again, err := service.Inspect(context.Background())
			if err != nil || again.BaseConfigDigest != got.BaseConfigDigest || again.ReceiptDigest != got.ReceiptDigest || again.Revision != got.Revision+1 {
				t.Fatalf("binding changed: %#v %v", again, err)
			}
		})
	}
}

func TestCurrentRoleDefaultsPreserveExplicitInheritance(t *testing.T) {
	st := roleSettingsStore(t, false)
	service := NewSettingsService(st)
	for _, host := range []string{"codex", "claude"} {
		if _, err := service.Update(context.Background(), map[string]string{host + ".developer.model": "inherit", host + ".reviewer.effort": "high"}); err != nil {
			t.Fatal(err)
		}
	}
	got, err := service.Inspect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, host := range []string{"codex", "claude"} {
		if got.Profile(host, "developer").Model != "" || got.Profile(host, "orchestrator").Model != "" {
			t.Fatal("explicit inheritance replaced")
		}
		if got.Profile(host, "reviewer").Model != defaultRoleProfile(host, "reviewer").Model {
			t.Fatal("default lost during effort update")
		}
	}
	if (Settings{}).Profile("codex", "coder").Model != "gpt-6-sol" {
		t.Fatal("one-off default missing")
	}
}
