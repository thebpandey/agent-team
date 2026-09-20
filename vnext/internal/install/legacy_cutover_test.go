package install

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/thebpandey/agent-team/vnext/internal/core"
)

func TestVerifiedLegacyAuthorityRejectsCrossProjectTransplant(t *testing.T) {
	root := t.TempDir()
	projectA, projectB := filepath.Join(root, "a"), filepath.Join(root, "b")
	inventories := []LegacyHostInventory{{Host: Codex, Root: filepath.Join(root, "codex")}, {Host: Claude, Root: filepath.Join(root, "claude")}}
	receiptSHA := signedLegacyAuthorityFixture(t, projectA, inventories)
	verified, err := verifyLegacyProjectAuthority(context.Background(), projectA, receiptSHA)
	if err != nil || len(verified) != 2 {
		t.Fatalf("verified = %#v, %v", verified, err)
	}
	if err := os.MkdirAll(filepath.Join(projectB, ".agent-team", "v8"), 0o700); err != nil {
		t.Fatal(err)
	}
	if output, err := exec.Command("git", "-C", projectB, "init").CombinedOutput(); err != nil {
		t.Fatalf("git init: %s: %v", output, err)
	}
	receiptRaw, _ := os.ReadFile(filepath.Join(projectA, filepath.FromSlash(legacyAuthorityReceiptPath)))
	receiptB := filepath.Join(projectB, filepath.FromSlash(legacyAuthorityReceiptPath))
	if err := os.WriteFile(receiptB, receiptRaw, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := verifyLegacyProjectAuthority(context.Background(), projectB, receiptSHA); err == nil {
		t.Fatal("cross-project authority receipt accepted")
	}
	alias := filepath.Join(root, "a-alias")
	if err := os.Symlink(projectA, alias); err == nil {
		if _, err := verifyLegacyProjectAuthority(context.Background(), alias, receiptSHA); err == nil {
			t.Fatal("symlinked project authority accepted")
		}
	}
	noncanonical := projectA + string(filepath.Separator) + ".." + string(filepath.Separator) + "a"
	if _, err := verifyLegacyProjectAuthority(context.Background(), noncanonical, receiptSHA); err == nil {
		t.Fatal("noncanonical project alias accepted")
	}
}

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
	request.Action = "host-cutover"
	request.ExpectedReceiptDigest = ""
	request.ExpectedManifestRevision = result.ManifestRevision
	result, err = CutoverLegacyHosts(context.Background(), layout, release, request)
	if err != nil || result.Idempotent {
		t.Fatalf("reapply = %#v, %v", result, err)
	}
	for _, host := range request.Hosts {
		assertFileDigest(t, filepath.Join(layout.SkillRoots[host], "SKILL.md"), release.Entrypoints[host].SHA256)
	}
}

func TestLegacyHostCutoverAcceptsOnlyVerifiedSourceInventory(t *testing.T) {
	root := t.TempDir()
	layout, err := ResolveLayout("linux", map[string]string{"XDG_DATA_HOME": filepath.Join(root, "data"), "CODEX_HOME": filepath.Join(root, "codex"), "CLAUDE_HOME": filepath.Join(root, "claude")})
	if err != nil {
		t.Fatal(err)
	}
	release := legacyReleaseFixture(t, root)
	if _, err := Install(context.Background(), layout, release, []Host{Codex, Claude}, 0); err != nil {
		t.Fatal(err)
	}
	for _, host := range []Host{Codex, Claude} {
		skill := []byte("legacy-" + string(host) + "\n")
		if err := os.WriteFile(filepath.Join(layout.SkillRoots[host], "SKILL.md"), skill, 0o600); err != nil {
			t.Fatal(err)
		}
		source := map[string]any{"version": "7.3.1", "sourceRevision": strings.Repeat("a", 40), "packageFileMap": map[string]any{"SKILL.md": map[string]any{"sha256": digestBytesInstall(skill), "mode": 384, "size": len(skill)}}}
		raw, _ := json.Marshal(source)
		if err := os.WriteFile(filepath.Join(layout.SkillRoots[host], ".agent-team-source.json"), raw, 0o600); err != nil {
			t.Fatal(err)
		}
		config := []byte(`{"unrelated":1,"hooks":{"SessionStart":[{"hooks":[{"type":"command","command":"node $HOME/skills/agent-team/hooks/agent-team-hook.mjs --runtime ` + string(host) + ` --event SessionStart"},{"type":"command","command":"foreign"}]}]}}`)
		if err := os.MkdirAll(filepath.Dir(layout.ConfigPaths[host]), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(layout.ConfigPaths[host], config, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	inventories, err := InventoryLegacyHosts(layout, []Host{Codex, Claude})
	if err != nil || len(inventories) != 2 {
		t.Fatalf("inventory = %#v, %v", inventories, err)
	}
	request := LegacyHostCutoverRequest{Schema: 1, Action: "host-cutover", OperationID: "signed-source", LegacyReceiptSHA256: strings.Repeat("b", 64), ExpectedManifestRevision: 1, Hosts: []Host{Codex, Claude}}
	configBefore := map[Host][]byte{}
	for _, host := range request.Hosts {
		configBefore[host], _ = os.ReadFile(layout.ConfigPaths[host])
	}
	if _, err := CutoverLegacyHosts(context.Background(), layout, release, request); err == nil {
		t.Fatal("unsigned source metadata accepted")
	}
	project := filepath.Join(root, "project")
	receiptSHA := signedLegacyAuthorityFixture(t, project, inventories)
	request.Project, request.AuthorityReceiptSHA256 = project, receiptSHA
	transplant := filepath.Join(root, "transplant")
	if err := os.MkdirAll(filepath.Join(transplant, filepath.Dir(filepath.FromSlash(legacyAuthorityReceiptPath))), 0o700); err != nil {
		t.Fatal(err)
	}
	if output, err := exec.Command("git", "-C", transplant, "init").CombinedOutput(); err != nil {
		t.Fatalf("git init: %s: %v", output, err)
	}
	receiptRaw, _ := os.ReadFile(filepath.Join(project, filepath.FromSlash(legacyAuthorityReceiptPath)))
	if err := os.WriteFile(filepath.Join(transplant, filepath.FromSlash(legacyAuthorityReceiptPath)), receiptRaw, 0o600); err != nil {
		t.Fatal(err)
	}
	request.Project = transplant
	if _, err := CutoverLegacyHosts(context.Background(), layout, release, request); err == nil {
		t.Fatal("transplanted project authority accepted by host cutover")
	}
	request.Project = project
	sourcePath := filepath.Join(layout.SkillRoots[Codex], ".agent-team-source.json")
	sourceBefore, _ := os.ReadFile(sourcePath)
	if err := os.WriteFile(sourcePath, []byte(`{"version":"foreign"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := CutoverLegacyHosts(context.Background(), layout, release, request); err == nil {
		t.Fatal("changed signed inventory accepted")
	}
	if err := os.WriteFile(sourcePath, sourceBefore, 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := CutoverLegacyHosts(context.Background(), layout, release, request)
	if err != nil || result.ManifestRevision != 2 {
		t.Fatalf("signed inventory cutover = %#v, %v", result, err)
	}
	for _, host := range []Host{Codex, Claude} {
		raw, err := os.ReadFile(layout.ConfigPaths[host])
		if err != nil || bytes.Contains(raw, []byte("agent-team-hook.mjs")) || !bytes.Contains(raw, []byte("foreign")) {
			t.Fatalf("%s config = %s, %v", host, raw, err)
		}
	}
	cutoverDigest := result.ReceiptDigest
	request.Action, request.ExpectedReceiptDigest, request.ExpectedManifestRevision = "host-rollback", cutoverDigest, result.ManifestRevision
	result, err = CutoverLegacyHosts(context.Background(), layout, release, request)
	if err != nil {
		t.Fatal(err)
	}
	for _, host := range request.Hosts {
		raw, _ := os.ReadFile(layout.ConfigPaths[host])
		if !bytes.Equal(raw, configBefore[host]) {
			t.Fatalf("%s signed rollback config differs", host)
		}
	}
	request.Action, request.ExpectedReceiptDigest, request.ExpectedManifestRevision = "host-cutover", "", result.ManifestRevision
	result, err = CutoverLegacyHosts(context.Background(), layout, release, request)
	if err != nil || result.Idempotent {
		t.Fatalf("signed reapply = %#v, %v", result, err)
	}
}

func signedLegacyAuthorityFixture(t *testing.T, project string, inventories []LegacyHostInventory) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(project, filepath.Dir(filepath.FromSlash(legacyAuthorityReceiptPath))), 0o700); err != nil {
		t.Fatal(err)
	}
	if output, err := exec.Command("git", "-C", project, "init").CombinedOutput(); err != nil {
		t.Fatalf("git init: %s: %v", output, err)
	}
	privateKey := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{7}, ed25519.SeedSize))
	publicKey := privateKey.Public().(ed25519.PublicKey)
	keyID := digestBytesInstall(publicKey)
	trustRaw, _ := json.Marshal(map[string]any{"schema": 1, "keys": []any{map[string]any{"algorithm": "ed25519", "keyId": keyID, "publicKey": base64.StdEncoding.EncodeToString(publicKey)}}})
	trustPath := filepath.Join(t.TempDir(), "trust.json")
	if err := os.WriteFile(trustPath, trustRaw, 0o600); err != nil {
		t.Fatal(err)
	}
	legacyAuthorityTrustStorePath = trustPath
	legacyAuthorityTrustStoreOwner = func(string, os.FileInfo) bool { return true }
	t.Cleanup(func() {
		legacyAuthorityTrustStorePath = systemLegacyAuthorityTrustStorePath()
		legacyAuthorityTrustStoreOwner = systemLegacyAuthorityTrustStoreOwner
	})
	approval := map[string]any{"schema": 1, "id": "approved", "project": project, "signerKeyId": keyID, "signature": "", "hostInventories": inventories}
	payload, _ := json.Marshal(approval)
	payloadPath, signaturePath := filepath.Join(project, "approval.json"), filepath.Join(project, "approval.sig")
	if err := os.WriteFile(payloadPath, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(signaturePath, ed25519.Sign(privateKey, payload), 0o600); err != nil {
		t.Fatal(err)
	}
	receipt := map[string]any{"schema": 1, "project": project, "approvalId": "approved", "approvalSha256": digestBytesInstall(payload), "approvalSignerKeyId": keyID, "approvalSignature": map[string]any{"id": "approved-signature", "path": signaturePath}, "authorization": map[string]any{"source": payloadPath}, "hostInventories": inventories}
	receiptRaw, _ := json.Marshal(receipt)
	if err := os.WriteFile(filepath.Join(project, filepath.FromSlash(legacyAuthorityReceiptPath)), receiptRaw, 0o600); err != nil {
		t.Fatal(err)
	}
	return digestBytesInstall(receiptRaw)
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

func TestRetireLegacyHandlersCoalescesAdjacentOwnedHandlers(t *testing.T) {
	first := []byte(`{"type":"command","command":"node first-owned.mjs","timeout":3}`)
	second := []byte(`{"type":"command","command":"node second-owned.mjs","timeout":4}`)
	foreign := `{"command":"foreign-λ","type":"command","value":1.00}`
	raw := []byte("{\n \"number\": 2.00, \"hooks\": {\"SessionStart\":[{\"hooks\":[\n  " + string(first) + ",\n  " + string(second) + ",\n  " + foreign + "\n ]}]}, \"tail\":{\"β\":1}\n}\n")
	receipt := func(id string, handler []byte) legacyHandler {
		compact := new(bytes.Buffer)
		if json.Compact(compact, handler) != nil {
			t.Fatal("handler")
		}
		return legacyHandler{Runtime: "codex", Event: "SessionStart", HandlerID: id, Digest: digestContent(compact.Bytes()), Handler: handler, ConfigPath: "/config.json"}
	}
	got, err := retireLegacyHandlers(raw, "/config.json", "codex", []legacyHandler{receipt("first", first), receipt("second", second)})
	if err != nil {
		t.Fatal(err)
	}
	want := []byte("{\n \"number\": 2.00, \"hooks\": {\"SessionStart\":[{\"hooks\":[\n  " + foreign + "\n ]}]}, \"tail\":{\"β\":1}\n}\n")
	if !bytes.Equal(got, want) {
		t.Fatalf("lossless removal mismatch\n got: %s\nwant: %s", got, want)
	}
	for _, exact := range [][]byte{[]byte(`"number": 2.00`), []byte(foreign), []byte(`"tail":{"β":1}`)} {
		if !bytes.Contains(got, exact) {
			t.Fatalf("unrelated bytes changed: missing %q in %s", exact, got)
		}
	}
	if bytes.Contains(got, []byte("owned.mjs")) || !json.Valid(got) {
		t.Fatalf("owned handlers not safely removed: %s", got)
	}
	onlyOwned := []byte(`{"hooks":{"SessionStart":[{"hooks":[` + string(first) + `,` + string(second) + `]}]}}`)
	got, err = retireLegacyHandlers(onlyOwned, "/config.json", "codex", []legacyHandler{receipt("first", first), receipt("second", second)})
	if err != nil || string(got) != `{"hooks":{"SessionStart":[{"hooks":[]}]}}` {
		t.Fatalf("complete owned run removal = %s, %v", got, err)
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
