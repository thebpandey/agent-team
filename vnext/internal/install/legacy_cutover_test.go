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
	"reflect"
	"runtime"
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

func TestUpdateAfterLegacyHostCutoverKeepsActiveEntrypointsCoherent(t *testing.T) {
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
	receiptDigest, _, err := sha256File(legacyReceipt)
	if err != nil {
		t.Fatal(err)
	}
	request := LegacyHostCutoverRequest{Schema: 1, Action: "host-cutover", OperationID: "update-active", LegacyReceipt: legacyReceipt, LegacyReceiptSHA256: receiptDigest, ExpectedManifestRevision: 1, Hosts: []Host{Codex, Claude}}
	cutover, err := CutoverLegacyHosts(context.Background(), layout, release, request)
	if err != nil {
		t.Fatal(err)
	}

	updated := legacyReleaseFixtureVersion(t, root, "8.0.2", "1123456789abcdef0123456789abcdef01234567", "updated")
	outcome, err := Update(context.Background(), layout, updated, cutover.ManifestRevision)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := readHostCutoverReceipt(layout)
	if err != nil {
		t.Fatal(err)
	}
	if receipt.Version != updated.Version || receipt.Revision != updated.Revision {
		t.Fatalf("receipt release = %s@%s", receipt.Version, receipt.Revision)
	}
	for _, host := range request.Hosts {
		top := filepath.Join(layout.SkillRoots[host], "SKILL.md")
		nested := filepath.Join(layout.SkillRoots[host], "agent-team-vnext", "SKILL.md")
		assertFileDigest(t, top, updated.Entrypoints[host].SHA256)
		if _, err := os.Lstat(nested); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("nested entrypoint recreated for %s: %v", host, err)
		}
		index := ownedIndex(outcome.Manifest.Files, EntrypointRole, host)
		if index < 0 || outcome.Manifest.Files[index].Path != top {
			t.Fatalf("manifest entrypoint for %s = %#v", host, outcome.Manifest.Files)
		}
	}
	request.Action, request.ExpectedManifestRevision, request.ExpectedReceiptDigest = "host-status", outcome.Manifest.Revision, receipt.ReceiptDigest
	if _, err := CutoverLegacyHosts(context.Background(), layout, updated, request); err != nil {
		t.Fatalf("updated host status: %v", err)
	}
	retry, err := Update(context.Background(), layout, updated, outcome.Manifest.Revision)
	if err != nil || !retry.Idempotent || retry.Manifest.Revision != outcome.Manifest.Revision {
		t.Fatalf("idempotent update = %#v, %v", retry, err)
	}
	rolledBack, err := RollbackRelease(context.Background(), layout, release.Version, release.Revision, outcome.Manifest.Revision)
	if err != nil {
		t.Fatal(err)
	}
	rolledReceipt, err := readHostCutoverReceipt(layout)
	if err != nil || rolledReceipt.Version != release.Version || rolledReceipt.Revision != release.Revision {
		t.Fatalf("rollback receipt = %#v, %v", rolledReceipt, err)
	}
	request.ExpectedManifestRevision, request.ExpectedReceiptDigest = rolledBack.Manifest.Revision, rolledReceipt.ReceiptDigest
	if _, err := CutoverLegacyHosts(context.Background(), layout, release, request); err != nil {
		t.Fatalf("rolled-back host status: %v", err)
	}
}

func TestUpdateAfterLegacyHostCutoverRejectsForeignNestedEntrypoint(t *testing.T) {
	layout, _, _, _ := legacyRollbackFixture(t)
	manifest, err := NewManifestStore(layout).Read(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(layout.SkillRoots[Codex], "agent-team-vnext", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(nested), 0o700); err != nil {
		t.Fatal(err)
	}
	foreign := []byte("foreign\n")
	if err := os.WriteFile(nested, foreign, 0o600); err != nil {
		t.Fatal(err)
	}
	updated := legacyReleaseFixtureVersion(t, t.TempDir(), "8.0.2", "1123456789abcdef0123456789abcdef01234567", "updated")
	if _, err := Update(context.Background(), layout, updated, manifest.Revision); !errors.Is(err, core.ErrRevision) {
		t.Fatalf("foreign nested update error = %v", err)
	}
	after, err := os.ReadFile(nested)
	if err != nil || !bytes.Equal(after, foreign) {
		t.Fatalf("foreign nested entrypoint changed: %q, %v", after, err)
	}
	unchanged, err := NewManifestStore(layout).Read(context.Background())
	if err != nil || !reflect.DeepEqual(unchanged, manifest) {
		t.Fatalf("manifest changed on refusal: %#v, %v", unchanged, err)
	}
}

func TestUpdateRelinquishesDriftedHostSettingsWithoutMutatingThem(t *testing.T) {
	layout, _, request, _ := legacyRollbackFixture(t)
	settingsPath := layout.ConfigPaths[Claude]
	settings := []byte("{\n  \"userSetting\": \"preserve exactly\"\n}\n")
	if err := os.WriteFile(settingsPath, settings, 0o600); err != nil {
		t.Fatal(err)
	}
	manifest, err := NewManifestStore(layout).Read(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	updated := legacyReleaseFixtureVersion(t, t.TempDir(), "8.0.3", "2123456789abcdef0123456789abcdef01234567", "updated")
	outcome, err := Update(context.Background(), layout, updated, manifest.Revision)
	if err != nil {
		t.Fatalf("update with user settings drift: %v", err)
	}
	if after, err := os.ReadFile(settingsPath); err != nil || !bytes.Equal(after, settings) {
		t.Fatalf("settings changed during update: %q, %v", after, err)
	}
	retry, err := Update(context.Background(), layout, updated, outcome.Manifest.Revision)
	if err != nil || !retry.Idempotent || retry.Manifest.Revision != outcome.Manifest.Revision {
		t.Fatalf("exact update retry = %#v, %v", retry, err)
	}
	receipt, err := readHostCutoverReceipt(layout)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(receipt)
	if err != nil || !bytes.Contains(raw, []byte(`"relinquished"`)) || !bytes.Contains(raw, []byte(digestBytesInstall(settings))) {
		t.Fatalf("relinquishment audit missing: %s, %v", raw, err)
	}
	request.Action, request.ExpectedManifestRevision, request.ExpectedReceiptDigest = "host-status", outcome.Manifest.Revision, receipt.ReceiptDigest
	if _, err := CutoverLegacyHosts(context.Background(), layout, updated, request); err != nil {
		t.Fatalf("host status after relinquishment: %v", err)
	}
	request.Action = "host-rollback"
	if _, err := CutoverLegacyHosts(context.Background(), layout, updated, request); err != nil {
		t.Fatalf("host rollback after relinquishment: %v", err)
	}
	if after, err := os.ReadFile(settingsPath); err != nil || !bytes.Equal(after, settings) {
		t.Fatalf("settings changed during rollback: %q, %v", after, err)
	}
	for _, host := range request.Hosts {
		assertFileDigest(t, filepath.Join(layout.SkillRoots[host], "SKILL.md"), digestText("legacy-"+string(host)+"\n"))
		assertFileDigest(t, filepath.Join(layout.SkillRoots[host], "agent-team-vnext", "SKILL.md"), updated.Entrypoints[host].SHA256)
	}
}

func TestUpdateRefusesSymlinkedHostSettingsBeforeMutation(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires privileges on Windows")
	}
	layout, _, _, _ := legacyRollbackFixture(t)
	settingsPath := layout.ConfigPaths[Claude]
	outside := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(outside, []byte("outside\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(settingsPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, settingsPath); err != nil {
		t.Fatal(err)
	}
	manifest, err := NewManifestStore(layout).Read(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	receiptPath := filepath.Join(layout.DataRoot, filepath.FromSlash(hostCutoverReceiptRel))
	receiptBefore, err := os.ReadFile(receiptPath)
	if err != nil {
		t.Fatal(err)
	}
	updated := legacyReleaseFixtureVersion(t, t.TempDir(), "8.0.3", "3123456789abcdef0123456789abcdef01234567", "updated")
	if _, err := Update(context.Background(), layout, updated, manifest.Revision); !errors.Is(err, core.ErrRevision) {
		t.Fatalf("symlinked settings update = %v", err)
	}
	unchanged, err := NewManifestStore(layout).Read(context.Background())
	receiptAfter, readErr := os.ReadFile(receiptPath)
	outsideAfter, outsideErr := os.ReadFile(outside)
	if err != nil || readErr != nil || outsideErr != nil || !reflect.DeepEqual(unchanged, manifest) || !bytes.Equal(receiptAfter, receiptBefore) || string(outsideAfter) != "outside\n" {
		t.Fatalf("refusal mutated state: manifest=%v receipt=%v outside=%q errors=%v/%v/%v", reflect.DeepEqual(unchanged, manifest), bytes.Equal(receiptAfter, receiptBefore), outsideAfter, err, readErr, outsideErr)
	}
	assertNoJournal(t, layout)
}

func TestUpdateRefusesHostSettingsPathSwapBeforeMutation(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires privileges on Windows")
	}
	layout, _, _, _ := legacyRollbackFixture(t)
	settingsPath := layout.ConfigPaths[Claude]
	manifest, err := NewManifestStore(layout).Read(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	receiptPath := filepath.Join(layout.DataRoot, filepath.FromSlash(hostCutoverReceiptRel))
	receiptBefore, err := os.ReadFile(receiptPath)
	if err != nil {
		t.Fatal(err)
	}
	original := settingsPath + ".original"
	outside := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(outside, []byte("outside\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	stableReadHook = func(path string) {
		if path != settingsPath {
			return
		}
		stableReadHook = nil
		if err := os.Rename(settingsPath, original); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(outside, settingsPath); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() { stableReadHook = nil })
	updated := legacyReleaseFixtureVersion(t, t.TempDir(), "8.0.3", "4123456789abcdef0123456789abcdef01234567", "updated")
	if _, err := Update(context.Background(), layout, updated, manifest.Revision); !errors.Is(err, core.ErrRevision) {
		t.Fatalf("swapped settings update = %v", err)
	}
	stableReadHook = nil
	unchanged, err := NewManifestStore(layout).Read(context.Background())
	receiptAfter, readErr := os.ReadFile(receiptPath)
	outsideAfter, outsideErr := os.ReadFile(outside)
	if err != nil || readErr != nil || outsideErr != nil || !reflect.DeepEqual(unchanged, manifest) || !bytes.Equal(receiptAfter, receiptBefore) || string(outsideAfter) != "outside\n" {
		t.Fatalf("swap refusal mutated authoritative state: manifest=%v receipt=%v outside=%q errors=%v/%v/%v", reflect.DeepEqual(unchanged, manifest), bytes.Equal(receiptAfter, receiptBefore), outsideAfter, err, readErr, outsideErr)
	}
	assertNoJournal(t, layout)
}

func TestUpdateRecoversRelinquishedHostSettingsJournal(t *testing.T) {
	layout, _, _, _ := legacyRollbackFixture(t)
	settingsPath := layout.ConfigPaths[Claude]
	settings := []byte("{\n  \"userSetting\": \"survives recovery\"\n}\n")
	if err := os.WriteFile(settingsPath, settings, 0o600); err != nil {
		t.Fatal(err)
	}
	manifest, err := NewManifestStore(layout).Read(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	updated := legacyReleaseFixtureVersion(t, t.TempDir(), "8.0.3", "5123456789abcdef0123456789abcdef01234567", "updated")
	lifecycleInterruptHook = func(operation string, index int) bool { return operation == "update" && index == 1 }
	if _, err := Update(context.Background(), layout, updated, manifest.Revision); !errors.Is(err, core.ErrTransition) {
		t.Fatalf("interrupted update = %v", err)
	}
	lifecycleInterruptHook = nil
	t.Cleanup(func() { lifecycleInterruptHook = nil })
	_, _ = Update(context.Background(), layout, updated, manifest.Revision)
	recovered, err := NewManifestStore(layout).Read(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	retry, err := Update(context.Background(), layout, updated, recovered.Revision)
	if err != nil || !retry.Idempotent {
		t.Fatalf("recovered update retry = %#v, %v", retry, err)
	}
	if after, err := os.ReadFile(settingsPath); err != nil || !bytes.Equal(after, settings) {
		t.Fatalf("settings changed during recovery: %q, %v", after, err)
	}
	receipt, err := readHostCutoverReceipt(layout)
	if err != nil || len(receipt.Relinquished) != 1 || receipt.Relinquished[0].ObservedSHA256 != digestBytesInstall(settings) {
		t.Fatalf("recovered receipt = %#v, %v", receipt.Relinquished, err)
	}
	assertNoJournal(t, layout)
}

func TestLegacyHostRollbackCompletesMixedExactState(t *testing.T) {
	layout, release, request, receipt := legacyRollbackFixture(t)
	already := receipt.Preimages[2]
	if err := os.WriteFile(already.Path, already.Bytes, os.FileMode(already.Mode)); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(already.Path)
	if err != nil {
		t.Fatal(err)
	}
	result, err := CutoverLegacyHosts(context.Background(), layout, release, request)
	if err != nil {
		t.Fatalf("mixed rollback = %#v, %v", result, err)
	}
	after, err := os.Stat(already.Path)
	if err != nil || !os.SameFile(before, after) {
		t.Fatalf("already-restored path replaced: %v", err)
	}
	assertFileDigest(t, already.Path, already.SHA256)
	assertLegacyRollbackState(t, layout, receipt)
}

func TestLegacyHostRollbackFinalizesAllPreimages(t *testing.T) {
	layout, release, request, receipt := legacyRollbackFixture(t)
	for _, preimage := range receipt.Preimages {
		if err := os.WriteFile(preimage.Path, preimage.Bytes, os.FileMode(preimage.Mode)); err != nil {
			t.Fatal(err)
		}
	}
	before, err := os.Stat(receipt.Preimages[0].Path)
	if err != nil {
		t.Fatal(err)
	}
	result, err := CutoverLegacyHosts(context.Background(), layout, release, request)
	if err != nil || !result.Idempotent {
		t.Fatalf("all-preimage rollback = %#v, %v", result, err)
	}
	after, err := os.Stat(receipt.Preimages[0].Path)
	if err != nil || !os.SameFile(before, after) {
		t.Fatalf("all-preimage path replaced: %v", err)
	}
	assertLegacyRollbackState(t, layout, receipt)
}

func TestLegacyHostRollbackRejectsThirdStateBeforeMutation(t *testing.T) {
	layout, release, request, receipt := legacyRollbackFixture(t)
	tampered := receipt.Preimages[2]
	if err := os.WriteFile(tampered.Path, tampered.Bytes, os.FileMode(tampered.Mode)); err != nil {
		t.Fatal(err)
	}
	driftMode := lifecycleDriftMode()
	if err := os.Chmod(tampered.Path, driftMode); err != nil {
		t.Fatal(err)
	}
	untouched := receipt.Postimages[0].Path
	before, err := os.Stat(untouched)
	if err != nil {
		t.Fatal(err)
	}
	manifestBefore, _ := os.ReadFile(layout.ManifestPath)
	receiptPath := filepath.Join(layout.DataRoot, filepath.FromSlash(hostCutoverReceiptRel))
	receiptBefore, _ := os.ReadFile(receiptPath)
	if _, err := CutoverLegacyHosts(context.Background(), layout, release, request); !errors.Is(err, core.ErrRevision) {
		t.Fatalf("third state rollback = %v", err)
	}
	if info, err := os.Stat(tampered.Path); err != nil || info.Mode().Perm() != driftMode.Perm() {
		t.Fatalf("tampered mode changed: %v", err)
	}
	after, err := os.Stat(untouched)
	if err != nil || !os.SameFile(before, after) {
		t.Fatalf("validated postimage changed: %v", err)
	}
	if got, _ := os.ReadFile(layout.ManifestPath); !bytes.Equal(got, manifestBefore) {
		t.Fatal("manifest changed after rejected rollback")
	}
	if got, _ := os.ReadFile(receiptPath); !bytes.Equal(got, receiptBefore) {
		t.Fatal("receipt changed after rejected rollback")
	}
}

func TestLegacyHostRollbackRejectsPostimageModeDriftBeforeMutation(t *testing.T) {
	for _, name := range []string{"top-level skill", "transformed settings"} {
		t.Run(name, func(t *testing.T) {
			layout, release, request, receipt := legacyRollbackFixture(t)
			if receipt.Schema != 1 {
				t.Fatalf("fixture receipt schema = %d", receipt.Schema)
			}
			target := receipt.Postimages[0].Path
			if name == "transformed settings" {
				target = layout.ConfigPaths[Claude]
			}
			if err := os.Chmod(target, lifecycleDriftMode()); err != nil {
				t.Fatal(err)
			}
			before := snapshotRollbackPaths(t, layout, receipt)
			if _, err := CutoverLegacyHosts(context.Background(), layout, release, request); !errors.Is(err, core.ErrRevision) {
				t.Fatalf("mode-drift rollback = %v", err)
			}
			assertRollbackPathsUnchanged(t, before)
			assertNoJournal(t, layout)
		})
	}
}

func TestLegacyHostRollbackRejectsPostimagePathReplacementBeforeMutation(t *testing.T) {
	layout, release, request, receipt := legacyRollbackFixture(t)
	target := receipt.Postimages[0].Path
	targetRaw, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	before := snapshotRollbackPaths(t, layout, receipt)
	var replacement os.FileInfo
	var replacementErr, rollbackErr error
	replacementDenied := new(int)
	stableReadHook = func(path string) {
		if path != target {
			return
		}
		stableReadHook = nil
		temporary := filepath.Join(filepath.Dir(target), "replacement")
		if err := os.WriteFile(temporary, targetRaw, 0o600); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Remove(temporary) })
		replacement, err = os.Stat(temporary)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(temporary, target); err != nil {
			replacementErr = err
			if nativeOpenReplacementDenied(err) {
				panic(replacementDenied)
			}
			t.Fatal(err)
		}
	}
	t.Cleanup(func() { stableReadHook = nil })
	func() {
		defer func() {
			if recovered := recover(); recovered != nil && recovered != replacementDenied {
				panic(recovered)
			}
		}()
		_, rollbackErr = CutoverLegacyHosts(context.Background(), layout, release, request)
	}()
	stableReadHook = nil
	if replacementErr != nil {
		assertRollbackPathsUnchanged(t, before)
		assertNoJournal(t, layout)
		return
	}
	if !errors.Is(rollbackErr, core.ErrRevision) {
		t.Fatalf("replacement rollback = %v", rollbackErr)
	}
	after, err := os.Stat(target)
	if err != nil || replacement == nil || !os.SameFile(replacement, after) {
		t.Fatalf("replacement path changed: %v", err)
	}
	delete(before, target)
	assertRollbackPathsUnchanged(t, before)
	assertNoJournal(t, layout)
}

func TestLegacyHostRollbackRejectsSameInodeModeChangeDuringPreparation(t *testing.T) {
	layout, release, request, receipt := legacyRollbackFixture(t)
	target := receipt.Postimages[0].Path
	targetRaw, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	targetBefore, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	before := snapshotRollbackPaths(t, layout, receipt)
	delete(before, target)
	reads := 0
	stableReadHook = func(path string) {
		if path != target {
			return
		}
		reads++
		if reads == 2 {
			if err := os.Chmod(path, lifecycleDriftMode()); err != nil {
				t.Fatal(err)
			}
		}
	}
	t.Cleanup(func() { stableReadHook = nil })
	if _, err := CutoverLegacyHosts(context.Background(), layout, release, request); !errors.Is(err, core.ErrRevision) {
		t.Fatalf("same-inode mode-change rollback = %v", err)
	}
	stableReadHook = nil
	targetAfter, err := os.Stat(target)
	afterRaw, readErr := os.ReadFile(target)
	if err != nil || readErr != nil || !os.SameFile(targetBefore, targetAfter) || targetAfter.Mode().Perm() != lifecycleDriftMode().Perm() || !bytes.Equal(afterRaw, targetRaw) {
		t.Fatalf("mode-changed path mutated: stat=%v read=%v", err, readErr)
	}
	assertRollbackPathsUnchanged(t, before)
	assertNoJournal(t, layout)
}

func TestLegacyHostRollbackRecoveryRejectsPostimageModeDrift(t *testing.T) {
	layout, release, request, receipt := legacyRollbackFixture(t)
	lifecycleInterruptHook = func(operation string, index int) bool { return operation == "rollback" && index == 0 }
	if _, err := CutoverLegacyHosts(context.Background(), layout, release, request); !errors.Is(err, core.ErrTransition) {
		t.Fatalf("rollback interruption = %v", err)
	}
	lifecycleInterruptHook = nil
	t.Cleanup(func() { lifecycleInterruptHook = nil })
	journal, err := readLifecycleJournal(layout)
	if err != nil || len(journal.Mutations) == 0 {
		t.Fatalf("read interrupted journal: %v", err)
	}
	target := journal.Mutations[0].Path
	if err := os.Chmod(target, lifecycleDriftMode()); err != nil {
		t.Fatal(err)
	}
	targetBefore, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	before := snapshotRollbackPaths(t, layout, receipt)
	if _, err := CutoverLegacyHosts(context.Background(), layout, release, request); !errors.Is(err, core.ErrRevision) {
		t.Fatalf("mode-drift recovery = %v", err)
	}
	targetAfter, err := os.Stat(target)
	if err != nil || !os.SameFile(targetBefore, targetAfter) || targetAfter.Mode().Perm() != lifecycleDriftMode().Perm() {
		t.Fatalf("recovery changed drifted path: %v", err)
	}
	assertRollbackPathsUnchanged(t, before)
	if _, err := readLifecycleJournal(layout); err != nil {
		t.Fatalf("recovery removed journal: %v", err)
	}
}

func TestLegacyHostRollbackRecoversInterruptedMixedState(t *testing.T) {
	layout, release, request, receipt := legacyRollbackFixture(t)
	already := receipt.Preimages[2]
	if err := os.WriteFile(already.Path, already.Bytes, os.FileMode(already.Mode)); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(already.Path)
	if err != nil {
		t.Fatal(err)
	}
	lifecycleInterruptHook = func(operation string, index int) bool { return operation == "rollback" && index == 0 }
	if _, err := CutoverLegacyHosts(context.Background(), layout, release, request); !errors.Is(err, core.ErrTransition) {
		t.Fatalf("mixed interruption = %v", err)
	}
	journal, err := readLifecycleJournal(layout)
	if err != nil {
		t.Fatal(err)
	}
	for _, mutation := range journal.Mutations {
		if mutation.Path == already.Path {
			t.Fatal("already-restored path was journaled")
		}
	}
	lifecycleInterruptHook = nil
	t.Cleanup(func() { lifecycleInterruptHook = nil })
	result, err := CutoverLegacyHosts(context.Background(), layout, release, request)
	if err != nil {
		t.Fatalf("mixed recovery = %#v, %v", result, err)
	}
	after, err := os.Stat(already.Path)
	if err != nil || !os.SameFile(before, after) {
		t.Fatalf("already-restored path changed during recovery: %v", err)
	}
	assertLegacyRollbackState(t, layout, receipt)
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
		info, err := os.Stat(filepath.Join(layout.SkillRoots[host], "SKILL.md"))
		if err != nil {
			t.Fatal(err)
		}
		source := map[string]any{"version": "7.3.1", "sourceRevision": strings.Repeat("a", 40), "packageFileMap": map[string]any{"SKILL.md": map[string]any{"sha256": digestBytesInstall(skill), "mode": info.Mode().Perm(), "size": len(skill)}}}
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
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		files := map[string]legacyFile{"SKILL.md": {SHA256: digestBytesInstall(body), Mode: uint32(info.Mode().Perm()), Size: int64(len(body))}}
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
	return legacyReleaseFixtureVersion(t, root, "8.0.0", "0123456789abcdef0123456789abcdef01234567", "")
}

func legacyReleaseFixtureVersion(t *testing.T, root, version, revision, suffix string) Release {
	t.Helper()
	write := func(name, body string) ReleaseFile {
		path := filepath.Join(root, version+"-"+name)
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		return ReleaseFile{Path: path, SHA256: digestText(body), Bytes: int64(len(body))}
	}
	return Release{Version: version, Revision: revision, Binary: write("agent-teamctl", "binary"+suffix+"\n"), Contract: write("WORKER-CONTRACT", "contract"+suffix+"\n"), Entrypoints: map[Host]ReleaseFile{Codex: write("codex-SKILL.md", "codex-v8"+suffix+"\n"), Claude: write("claude-SKILL.md", "claude-v8"+suffix+"\n")}}
}

func legacyRollbackFixture(t *testing.T) (Layout, Release, LegacyHostCutoverRequest, hostCutoverReceipt) {
	t.Helper()
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
	receiptDigest, _, err := sha256File(legacyReceipt)
	if err != nil {
		t.Fatal(err)
	}
	request := LegacyHostCutoverRequest{Schema: 1, Action: "host-cutover", OperationID: "partial-rollback", LegacyReceipt: legacyReceipt, LegacyReceiptSHA256: receiptDigest, ExpectedManifestRevision: 1, Hosts: []Host{Codex, Claude}}
	result, err := CutoverLegacyHosts(context.Background(), layout, release, request)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := readHostCutoverReceipt(layout)
	if err != nil {
		t.Fatal(err)
	}
	request.Action, request.ExpectedManifestRevision, request.ExpectedReceiptDigest = "host-rollback", result.ManifestRevision, result.ReceiptDigest
	return layout, release, request, receipt
}

func assertLegacyRollbackState(t *testing.T, layout Layout, receipt hostCutoverReceipt) {
	t.Helper()
	for _, preimage := range receipt.Preimages {
		info, err := os.Lstat(preimage.Path)
		if err != nil || !info.Mode().IsRegular() || uint32(info.Mode().Perm()) != preimage.Mode {
			t.Fatalf("preimage mode %s: %v", preimage.Path, err)
		}
		assertFileDigest(t, preimage.Path, preimage.SHA256)
	}
	if _, err := os.Lstat(filepath.Join(layout.DataRoot, filepath.FromSlash(hostCutoverReceiptRel))); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("cutover receipt retained: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(layout.DataRoot, filepath.FromSlash(installAttemptPath))); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("lifecycle journal retained: %v", err)
	}
}

type rollbackPathSnapshot struct {
	info   os.FileInfo
	raw    []byte
	absent bool
}

func snapshotRollbackPaths(t *testing.T, layout Layout, receipt hostCutoverReceipt) map[string]rollbackPathSnapshot {
	t.Helper()
	paths := []string{layout.ManifestPath, filepath.Join(layout.DataRoot, filepath.FromSlash(hostCutoverReceiptRel))}
	for _, image := range receipt.Postimages {
		paths = append(paths, image.Path)
	}
	result := make(map[string]rollbackPathSnapshot, len(paths))
	for _, path := range paths {
		info, err := os.Lstat(path)
		if errors.Is(err, os.ErrNotExist) {
			result[path] = rollbackPathSnapshot{absent: true}
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		result[path] = rollbackPathSnapshot{info: info, raw: raw}
	}
	return result
}

func assertRollbackPathsUnchanged(t *testing.T, before map[string]rollbackPathSnapshot) {
	t.Helper()
	for path, want := range before {
		info, err := os.Lstat(path)
		if want.absent {
			if !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("absent path changed %s: %v", path, err)
			}
			continue
		}
		raw, readErr := os.ReadFile(path)
		if err != nil || readErr != nil || !os.SameFile(want.info, info) || info.Mode() != want.info.Mode() || !bytes.Equal(raw, want.raw) {
			t.Fatalf("path changed %s: stat=%v read=%v", path, err, readErr)
		}
	}
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
