package install

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/thebpandey/agent-team/vnext/internal/core"
)

func TestLegacyHostCutoverRollbackAndRetry(t *testing.T) {
	root := t.TempDir()
	layout, err := ResolveLayout("linux", map[string]string{"XDG_DATA_HOME": filepath.Join(root, "data"), "CODEX_HOME": filepath.Join(root, "codex"), "CLAUDE_HOME": filepath.Join(root, "claude")})
	if err != nil {
		t.Fatal(err)
	}
	release := legacyReleaseFixture(t, root)
	if _, err := Install(context.Background(), layout, release, []Host{Codex, Claude}, 0); err != nil {
		t.Fatal(err)
	}
	legacyReceipt := legacyHostFixture(t, layout)
	configBefore := map[Host][]byte{}
	for _, host := range []Host{Codex, Claude} {
		configBefore[host], _ = os.ReadFile(layout.ConfigPaths[host])
	}
	receiptDigest, _, err := sha256File(legacyReceipt)
	if err != nil {
		t.Fatal(err)
	}
	request := LegacyHostCutoverRequest{Schema: 1, Action: "host-cutover", OperationID: "host-cutover", LegacyReceipt: legacyReceipt, LegacyReceiptSHA256: receiptDigest, ExpectedManifestRevision: 1, Hosts: []Host{Codex, Claude}}

	result, err := CutoverLegacyHosts(context.Background(), layout, release, request)
	if err != nil || result.Idempotent {
		t.Fatalf("cutover = %#v, %v", result, err)
	}
	for _, host := range request.Hosts {
		assertFileDigest(t, filepath.Join(layout.SkillRoots[host], "SKILL.md"), release.Entrypoints[host].SHA256)
		if _, err := os.Stat(filepath.Join(layout.SkillRoots[host], "agent-team-vnext", "SKILL.md")); !os.IsNotExist(err) {
			t.Fatal("nested v8 entrypoint retained")
		}
	}
	configRaw, err := os.ReadFile(layout.ConfigPaths[Claude])
	if err != nil || !json.Valid(configRaw) || !containsBytes(configRaw, []byte("foreign-command")) || containsBytes(configRaw, []byte("agent-team-hook.mjs")) {
		t.Fatalf("claude config changed incorrectly: %s %v", configRaw, err)
	}

	request.ExpectedManifestRevision = result.ManifestRevision
	result, err = CutoverLegacyHosts(context.Background(), layout, release, request)
	if err != nil || !result.Idempotent {
		t.Fatalf("retry = %#v, %v", result, err)
	}

	request.Action = "host-rollback"
	request.ExpectedReceiptDigest = result.ReceiptDigest
	result, err = CutoverLegacyHosts(context.Background(), layout, release, request)
	if err != nil {
		t.Fatal(err)
	}
	for _, host := range request.Hosts {
		assertFileDigest(t, filepath.Join(layout.SkillRoots[host], "SKILL.md"), digestText("legacy-"+string(host)+"\n"))
		assertFileDigest(t, filepath.Join(layout.SkillRoots[host], "agent-team-vnext", "SKILL.md"), release.Entrypoints[host].SHA256)
		after, _ := os.ReadFile(layout.ConfigPaths[host])
		if string(after) != string(configBefore[host]) {
			t.Fatalf("%s config rollback differs", host)
		}
	}
}

func TestLegacyHostCutoverRejectsForeignOwnedFileBeforeMutation(t *testing.T) {
	root := t.TempDir()
	layout, _ := ResolveLayout("linux", map[string]string{"XDG_DATA_HOME": filepath.Join(root, "data"), "CODEX_HOME": filepath.Join(root, "codex"), "CLAUDE_HOME": filepath.Join(root, "claude")})
	release := legacyReleaseFixture(t, root)
	if _, err := Install(context.Background(), layout, release, []Host{Codex, Claude}, 0); err != nil {
		t.Fatal(err)
	}
	receipt := legacyHostFixture(t, layout)
	digest, _, _ := sha256File(receipt)
	if err := os.WriteFile(filepath.Join(layout.SkillRoots[Codex], "SKILL.md"), []byte("foreign\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	request := LegacyHostCutoverRequest{Schema: 1, Action: "host-cutover", OperationID: "host-cutover", LegacyReceipt: receipt, LegacyReceiptSHA256: digest, ExpectedManifestRevision: 1, Hosts: []Host{Codex, Claude}}
	if _, err := CutoverLegacyHosts(context.Background(), layout, release, request); err == nil {
		t.Fatal("foreign legacy file accepted")
	}
	assertFileDigest(t, filepath.Join(layout.SkillRoots[Codex], "SKILL.md"), digestText("foreign\n"))
}

func TestLegacyHostCutoverRequiresReceiptOwnedTopLevelSkill(t *testing.T) {
	root := t.TempDir()
	layout, _ := ResolveLayout("linux", map[string]string{"XDG_DATA_HOME": filepath.Join(root, "data"), "CODEX_HOME": filepath.Join(root, "codex"), "CLAUDE_HOME": filepath.Join(root, "claude")})
	release := legacyReleaseFixture(t, root)
	if _, err := Install(context.Background(), layout, release, []Host{Codex, Claude}, 0); err != nil {
		t.Fatal(err)
	}
	receiptPath := legacyHostFixture(t, layout)
	var receipt map[string]any
	raw, _ := os.ReadFile(receiptPath)
	if json.Unmarshal(raw, &receipt) != nil {
		t.Fatal("receipt")
	}
	maps := receipt["installedFileMaps"].(map[string]any)
	codex := maps["codex"].(map[string]any)
	files := codex["Files"].(map[string]any)
	delete(files, "SKILL.md")
	codex["Digest"] = digestLegacyMap(map[string]any{})
	updated, _ := json.Marshal(receipt)
	if err := os.WriteFile(receiptPath, updated, 0o600); err != nil {
		t.Fatal(err)
	}
	digest, _, _ := sha256File(receiptPath)
	request := LegacyHostCutoverRequest{Schema: 1, Action: "host-cutover", OperationID: "missing-top-skill", LegacyReceipt: receiptPath, LegacyReceiptSHA256: digest, ExpectedManifestRevision: 1, Hosts: []Host{Codex, Claude}}
	if _, err := CutoverLegacyHosts(context.Background(), layout, release, request); err == nil {
		t.Fatal("unowned top-level skill accepted")
	}
	assertFileDigest(t, filepath.Join(layout.SkillRoots[Codex], "SKILL.md"), digestText("legacy-codex\n"))
}

func TestLegacyHostCutoverRejectsTopLevelSkillSymlinkAndSwap(t *testing.T) {
	for _, scenario := range []string{"symlink", "swap"} {
		t.Run(scenario, func(t *testing.T) {
			root := t.TempDir()
			layout, _ := ResolveLayout("linux", map[string]string{"XDG_DATA_HOME": filepath.Join(root, "data"), "CODEX_HOME": filepath.Join(root, "codex"), "CLAUDE_HOME": filepath.Join(root, "claude")})
			release := legacyReleaseFixture(t, root)
			if _, err := Install(context.Background(), layout, release, []Host{Codex, Claude}, 0); err != nil {
				t.Fatal(err)
			}
			receipt := legacyHostFixture(t, layout)
			digest, _, _ := sha256File(receipt)
			top := filepath.Join(layout.SkillRoots[Codex], "SKILL.md")
			if scenario == "symlink" {
				target := filepath.Join(root, "foreign-skill")
				if err := os.WriteFile(target, []byte("legacy-codex\n"), 0o600); err != nil {
					t.Fatal(err)
				}
				if err := os.Remove(top); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(target, top); err != nil {
					t.Fatal(err)
				}
			} else {
				lifecycleMutationHook = func(operation string, index int) error {
					if operation == "update" && index == 0 {
						return os.WriteFile(top, []byte("foreign replacement\n"), 0o600)
					}
					return nil
				}
				t.Cleanup(func() { lifecycleMutationHook = nil })
			}
			request := LegacyHostCutoverRequest{Schema: 1, Action: "host-cutover", OperationID: "skill-" + scenario, LegacyReceipt: receipt, LegacyReceiptSHA256: digest, ExpectedManifestRevision: 1, Hosts: []Host{Codex, Claude}}
			if _, err := CutoverLegacyHosts(context.Background(), layout, release, request); err == nil {
				t.Fatal("unsafe top-level skill accepted")
			}
			if scenario == "swap" {
				lifecycleMutationHook = nil
				raw, _ := os.ReadFile(top)
				if string(raw) != "foreign replacement\n" {
					t.Fatalf("replacement changed: %q", raw)
				}
			}
		})
	}
}

func TestRetireLegacyHandlersPreservesUnrelatedBytes(t *testing.T) {
	handler := []byte(`{"type":"command","command":"node agent-team-hook.mjs","timeout":3}`)
	compact := new(bytes.Buffer)
	if json.Compact(compact, handler) != nil {
		t.Fatal("handler")
	}
	raw := []byte("{\n  \"number\": 1.00,\n  \"nested\": { \"orderB\":2, \"orderA\":1 },\n  \"hooks\": {\n    \"SessionStart\": [ { \"hooks\": [ " + string(handler) + ", {\"command\":\"foreign\",\"type\":\"command\"} ] } ]\n  },\n  \"tail\": \"keep\"\n}\n")
	receipt := legacyHandler{Runtime: "codex", Event: "SessionStart", HandlerID: "codex:SessionStart:0:0", Digest: digestContent(compact.Bytes()), Handler: handler, ConfigPath: "/config.json"}
	got, err := retireLegacyHandlers(raw, "/config.json", "codex", []legacyHandler{receipt})
	if err != nil {
		t.Fatal(err)
	}
	for _, unchanged := range [][]byte{[]byte("\"number\": 1.00"), []byte("{ \"orderB\":2, \"orderA\":1 }"), []byte("{\"command\":\"foreign\",\"type\":\"command\"}"), []byte("\"tail\": \"keep\"")} {
		if !bytes.Contains(got, unchanged) {
			t.Fatalf("unrelated bytes changed: missing %q in %s", unchanged, got)
		}
	}
	if bytes.Contains(got, []byte("agent-team-hook")) || !json.Valid(got) {
		t.Fatalf("handler not safely removed: %s", got)
	}
}

func TestLegacyHostCutoverRecoversInterruptedMutation(t *testing.T) {
	root := t.TempDir()
	layout, _ := ResolveLayout("linux", map[string]string{"XDG_DATA_HOME": filepath.Join(root, "data"), "CODEX_HOME": filepath.Join(root, "codex"), "CLAUDE_HOME": filepath.Join(root, "claude")})
	release := legacyReleaseFixture(t, root)
	if _, err := Install(context.Background(), layout, release, []Host{Codex, Claude}, 0); err != nil {
		t.Fatal(err)
	}
	receipt := legacyHostFixture(t, layout)
	digest, _, _ := sha256File(receipt)
	request := LegacyHostCutoverRequest{Schema: 1, Action: "host-cutover", OperationID: "host-crash", LegacyReceipt: receipt, LegacyReceiptSHA256: digest, ExpectedManifestRevision: 1, Hosts: []Host{Codex, Claude}}
	lifecycleInterruptHook = func(operation string, index int) bool { return operation == "update" && index == 2 }
	if _, err := CutoverLegacyHosts(context.Background(), layout, release, request); !errors.Is(err, core.ErrTransition) {
		t.Fatalf("interruption = %v", err)
	}
	lifecycleInterruptHook = nil
	t.Cleanup(func() { lifecycleInterruptHook = nil })
	result, err := CutoverLegacyHosts(context.Background(), layout, release, request)
	if err != nil || result.ManifestRevision != 2 {
		t.Fatalf("recovered cutover = %#v, %v", result, err)
	}
}

func TestLegacyHostCutoverRetainsConfigReplacement(t *testing.T) {
	root := t.TempDir()
	layout, _ := ResolveLayout("linux", map[string]string{"XDG_DATA_HOME": filepath.Join(root, "data"), "CODEX_HOME": filepath.Join(root, "codex"), "CLAUDE_HOME": filepath.Join(root, "claude")})
	release := legacyReleaseFixture(t, root)
	if _, err := Install(context.Background(), layout, release, []Host{Codex, Claude}, 0); err != nil {
		t.Fatal(err)
	}
	receipt := legacyHostFixture(t, layout)
	digest, _, _ := sha256File(receipt)
	request := LegacyHostCutoverRequest{Schema: 1, Action: "host-cutover", OperationID: "host-config-race", LegacyReceipt: receipt, LegacyReceiptSHA256: digest, ExpectedManifestRevision: 1, Hosts: []Host{Codex, Claude}}
	lifecycleMutationHook = func(operation string, index int) error {
		if operation == "update" && index == 2 {
			return os.WriteFile(layout.ConfigPaths[Codex], []byte("foreign\n"), 0o600)
		}
		return nil
	}
	t.Cleanup(func() { lifecycleMutationHook = nil })
	if _, err := CutoverLegacyHosts(context.Background(), layout, release, request); err == nil {
		t.Fatal("config replacement race accepted")
	}
	lifecycleMutationHook = nil
	raw, _ := os.ReadFile(layout.ConfigPaths[Codex])
	if string(raw) != "foreign\n" {
		t.Fatalf("foreign replacement changed: %q", raw)
	}
}

func TestLegacyHostCutoverRejectsForeignHandlerBeforeMutation(t *testing.T) {
	root := t.TempDir()
	layout, _ := ResolveLayout("linux", map[string]string{"XDG_DATA_HOME": filepath.Join(root, "data"), "CODEX_HOME": filepath.Join(root, "codex"), "CLAUDE_HOME": filepath.Join(root, "claude")})
	release := legacyReleaseFixture(t, root)
	if _, err := Install(context.Background(), layout, release, []Host{Codex, Claude}, 0); err != nil {
		t.Fatal(err)
	}
	receipt := legacyHostFixture(t, layout)
	digest, _, _ := sha256File(receipt)
	before, _ := os.ReadFile(filepath.Join(layout.SkillRoots[Codex], "SKILL.md"))
	if err := os.WriteFile(layout.ConfigPaths[Codex], []byte("{\"hooks\":{}}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	request := LegacyHostCutoverRequest{Schema: 1, Action: "host-cutover", OperationID: "host-foreign-handler", LegacyReceipt: receipt, LegacyReceiptSHA256: digest, ExpectedManifestRevision: 1, Hosts: []Host{Codex, Claude}}
	if _, err := CutoverLegacyHosts(context.Background(), layout, release, request); err == nil {
		t.Fatal("foreign handler accepted")
	}
	after, _ := os.ReadFile(filepath.Join(layout.SkillRoots[Codex], "SKILL.md"))
	if string(after) != string(before) {
		t.Fatal("validation failure mutated skill")
	}
}

func legacyHostFixture(t *testing.T, layout Layout) string {
	t.Helper()
	type legacyFile struct {
		SHA256 string `json:"sha256"`
		Mode   uint32 `json:"mode"`
		Size   int64  `json:"size"`
	}
	type fileMap struct {
		Target, Digest string
		Files          map[string]legacyFile
	}
	installed := map[string]fileMap{}
	handlers := []map[string]any{}
	for _, host := range []Host{Codex, Claude} {
		body := []byte("legacy-" + string(host) + "\n")
		path := filepath.Join(layout.SkillRoots[host], "SKILL.md")
		if err := os.WriteFile(path, body, 0o600); err != nil {
			t.Fatal(err)
		}
		files := map[string]legacyFile{"SKILL.md": {SHA256: digestBytesInstall(body), Mode: 0o600, Size: int64(len(body))}}
		installed[string(host)] = fileMap{Target: layout.SkillRoots[host], Digest: legacyFileMapDigest(map[string]legacyInstalledFile{"SKILL.md": {SHA256: files["SKILL.md"].SHA256, Mode: files["SKILL.md"].Mode, Size: files["SKILL.md"].Size}}), Files: files}
		handler := map[string]any{"type": "command", "command": "node agent-team-hook.mjs", "timeout": float64(3)}
		handlers = append(handlers, map[string]any{"runtime": string(host), "event": "SessionStart", "handlerId": string(host) + ":SessionStart:0:0", "digest": digestLegacyMap(handler), "handler": handler, "preexisting": false, "configPath": layout.ConfigPaths[host]})
		config := map[string]any{"unrelated": "keep", "hooks": map[string]any{"SessionStart": []any{map[string]any{"hooks": []any{handler, map[string]any{"type": "command", "command": "foreign-command"}}}}}}
		raw, _ := json.MarshalIndent(config, "", "  ")
		if err := os.MkdirAll(filepath.Dir(layout.ConfigPaths[host]), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(layout.ConfigPaths[host], append(raw, '\n'), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	receipt := map[string]any{"schemaVersion": 4, "version": "7.3.1", "installedFileMaps": installed, "handlers": handlers, "resourceConflicts": []any{}, "handlerConflicts": []any{}}
	raw, _ := json.MarshalIndent(receipt, "", "  ")
	path := filepath.Join(layout.DataRoot, "legacy-install.json")
	if err := os.WriteFile(path, append(raw, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func legacyReleaseFixture(t *testing.T, root string) Release {
	t.Helper()
	write := func(name, body string) ReleaseFile {
		path := filepath.Join(root, name)
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		return ReleaseFile{Path: path, SHA256: digestText(body), Bytes: int64(len(body))}
	}
	return Release{Version: "8.0.0", Revision: "0123456789abcdef0123456789abcdef01234567", Binary: write("agent-teamctl", "binary\n"), Contract: write("WORKER-CONTRACT", "contract\n"), Entrypoints: map[Host]ReleaseFile{Codex: write("codex-SKILL.md", "codex-v8\n"), Claude: write("claude-SKILL.md", "claude-v8\n")}}
}

func digestLegacyMap(value any) string {
	raw, _ := json.Marshal(value)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
func digestBytesInstall(value []byte) string {
	sum := sha256.Sum256(value)
	return hex.EncodeToString(sum[:])
}
func digestText(value string) string { return digestBytesInstall([]byte(value)) }
func containsBytes(haystack, needle []byte) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if string(haystack[i:i+len(needle)]) == string(needle) {
			return true
		}
	}
	return false
}
func assertFileDigest(t *testing.T, path, want string) {
	t.Helper()
	got, _, err := sha256File(path)
	if err != nil || got != want {
		t.Fatalf("%s digest=%s want=%s err=%v", path, got, want, err)
	}
}
