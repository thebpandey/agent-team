package migrate

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
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/thebpandey/agent-team/vnext/internal/core"
	"github.com/thebpandey/agent-team/vnext/internal/install"
	"github.com/thebpandey/agent-team/vnext/internal/release"
	"github.com/thebpandey/agent-team/vnext/internal/store"
)

func TestAuthorityPrepareWritesCanonicalDetachedArtifacts(t *testing.T) {
	project := authorityFixture(t)
	request := readAuthorityRequestTest(t, authorityRequestFixture(t, project))
	root := t.TempDir()
	layout, err := install.ResolveLayout("linux", map[string]string{"XDG_DATA_HOME": filepath.Join(root, "data"), "CODEX_HOME": filepath.Join(root, "codex"), "CLAUDE_HOME": filepath.Join(root, "claude")})
	if err != nil {
		t.Fatal(err)
	}
	releaseFixture := install.Release{Version: "8.0.0", Revision: request.TargetRevision, Entrypoints: map[install.Host]install.ReleaseFile{}}
	for name, target := range map[string]*install.ReleaseFile{"binary": &releaseFixture.Binary, "contract": &releaseFixture.Contract} {
		path := filepath.Join(root, name)
		raw := []byte(name + "\n")
		if os.WriteFile(path, raw, 0o600) != nil {
			t.Fatal("release")
		}
		*target = install.ReleaseFile{Path: path, SHA256: digestBytes(raw), Bytes: int64(len(raw))}
	}
	for _, host := range []install.Host{install.Codex, install.Claude} {
		path := filepath.Join(root, string(host)+"-entry")
		raw := []byte("native " + string(host) + "\n")
		if os.WriteFile(path, raw, 0o600) != nil {
			t.Fatal("entry")
		}
		releaseFixture.Entrypoints[host] = install.ReleaseFile{Path: path, SHA256: digestBytes(raw), Bytes: int64(len(raw))}
	}
	if _, err := install.Install(context.Background(), layout, releaseFixture, []install.Host{install.Codex, install.Claude}, 0); err != nil {
		t.Fatal(err)
	}
	for _, host := range []install.Host{install.Codex, install.Claude} {
		skill := []byte("legacy " + string(host) + "\n")
		if err := os.WriteFile(filepath.Join(layout.SkillRoots[host], "SKILL.md"), skill, 0o600); err != nil {
			t.Fatal(err)
		}
		metadata := map[string]any{"version": "7.3.1", "sourceRevision": strings.Repeat("a", 40), "packageFileMap": map[string]any{"SKILL.md": map[string]any{"sha256": digestBytes(skill), "mode": 384, "size": len(skill)}}}
		if raw, marshalErr := json.Marshal(metadata); marshalErr != nil || os.WriteFile(filepath.Join(layout.SkillRoots[host], ".agent-team-source.json"), raw, 0o600) != nil {
			t.Fatal(marshalErr)
		}
	}
	for i := range request.Reviews {
		request.Reviews[i].Path, _ = filepath.Rel(project, request.Reviews[i].Path)
		request.Reviews[i].SHA256 = ""
	}
	for i := range request.Tests {
		request.Tests[i].Path, _ = filepath.Rel(project, request.Tests[i].Path)
		request.Tests[i].SHA256 = ""
	}
	request.Readiness.Path, _ = filepath.Rel(project, request.Readiness.Path)
	request.Readiness.SHA256 = ""
	request.Tracker.Export, _ = filepath.Rel(project, request.Tracker.Export)
	request.Tracker.SHA256, request.Tracker.Fingerprint, request.Tracker.TaskIDs, request.Tracker.TaskCount = "", "", nil, 0
	issued := time.Now().UTC().Add(-time.Minute)
	request.Action, request.Approval = "prepare", EvidenceReference{}
	privateKey := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{3}, ed25519.SeedSize))
	keyID := digestBytes(privateKey.Public().(ed25519.PublicKey))
	request.Prepare = &AuthorityPrepare{ApprovalID: "operator-approval", SignerKeyID: keyID, IssuedAt: issued.Format(time.RFC3339Nano), ExpiresAt: issued.Add(time.Hour).Format(time.RFC3339Nano), Cause: "authorized cutover", RecoveryDisposition: "restore archive", RemoteName: "origin", BaseRef: "refs/heads/main", TargetRef: "refs/heads/main", RemoteMainDeploys: true, PayloadPath: filepath.Join(project, "unsigned.json"), RequestPath: filepath.Join(project, "cutover.json"), SignaturePath: filepath.Join(project, "unsigned.sig"), NativeManifest: layout.ManifestPath, HostRoots: layout.SkillRoots}
	path := writeAuthorityJSON(t, project, "prepare.json", request)
	result, err := ExecuteAuthorityRequest(context.Background(), path)
	if err != nil || result.Action != "prepare" || !validDigest(result.ReceiptDigest) {
		t.Fatalf("prepare = %#v, %v", result, err)
	}
	if _, err := os.Stat(filepath.Join(project, authorityReceiptPath)); !os.IsNotExist(err) {
		t.Fatal("prepare mutated authority")
	}
	payload, err := os.ReadFile(request.Prepare.PayloadPath)
	if err != nil || digestBytes(payload) != result.ReceiptDigest {
		t.Fatalf("payload: %v", err)
	}
	if info, statErr := os.Stat(request.Prepare.PayloadPath); statErr != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("payload mode = %v, %v", info, statErr)
	}
	var approval signedCutoverApproval
	if json.Unmarshal(payload, &approval) != nil || len(approval.HostInventories) != 2 || approval.Signature != "" {
		t.Fatal("invalid unsigned payload")
	}
	var skeleton AuthorityRequest
	raw, _ := os.ReadFile(request.Prepare.RequestPath)
	if json.Unmarshal(raw, &skeleton) != nil || skeleton.Action != "cutover" || skeleton.Approval.Path != request.Prepare.PayloadPath || skeleton.ApprovalSignature.Path != request.Prepare.SignaturePath {
		t.Fatal("invalid request skeleton")
	}
	second := request
	copyPrepare := *request.Prepare
	second.Prepare = &copyPrepare
	second.Prepare.PayloadPath = filepath.Join(project, "unsigned-2.json")
	second.Prepare.RequestPath = filepath.Join(project, "cutover-2.json")
	second.Prepare.SignaturePath = filepath.Join(project, "unsigned-2.sig")
	if _, err := ExecuteAuthorityRequest(context.Background(), writeAuthorityJSON(t, project, "prepare-2.json", second)); err != nil {
		t.Fatal(err)
	}
	secondPayload, _ := os.ReadFile(second.Prepare.PayloadPath)
	if !bytes.Equal(payload, secondPayload) {
		t.Fatal("prepare payload is not deterministic")
	}
	if _, err := ExecuteAuthorityRequest(context.Background(), path); err == nil {
		t.Fatal("prepare overwrote existing outputs")
	}
	if err := os.WriteFile(request.Prepare.SignaturePath, ed25519.Sign(privateKey, payload), 0o600); err != nil {
		t.Fatal(err)
	}
	cutover, err := ExecuteAuthorityRequest(context.Background(), request.Prepare.RequestPath)
	if err != nil || cutover.Action != "cutover" {
		t.Fatalf("prepared cutover = %#v, %v", cutover, err)
	}
	receipt, err := AuthorityStatus(project)
	if err != nil || len(receipt.HostInventories) != 2 {
		t.Fatalf("authority status = %#v, %v", receipt, err)
	}
}

func TestProjectEvidencePathRejectsRelativeEscape(t *testing.T) {
	project := t.TempDir()
	for _, candidate := range []string{"../outside.json", filepath.Join("nested", "..", "..", "outside.json")} {
		if _, err := projectEvidencePath(project, candidate); !errors.Is(err, core.ErrPath) {
			t.Fatalf("projectEvidencePath(%q) = %v, want ErrPath", candidate, err)
		}
	}
	external := t.TempDir()
	link := filepath.Join(project, "linked")
	if err := os.Symlink(external, link); err == nil {
		if _, err := projectEvidencePath(project, filepath.Join("linked", "evidence.json")); !errors.Is(err, core.ErrPath) {
			t.Fatalf("symlinked relative evidence = %v, want ErrPath", err)
		}
	}
	absolute := filepath.Join(external, "evidence.json")
	if got, err := projectEvidencePath(project, absolute); err != nil || got != absolute {
		t.Fatalf("absolute external evidence = %q, %v", got, err)
	}
}

func TestPreparedArtifactCleanupRetainsReplacement(t *testing.T) {
	root := t.TempDir()
	payloadPath := filepath.Join(root, "approval.json")
	requestPath := filepath.Join(root, "request.json")
	if err := os.WriteFile(requestPath, []byte("occupied"), 0o600); err != nil {
		t.Fatal(err)
	}
	preflightCleanupHook = func(path string) {
		if path != payloadPath {
			t.Fatalf("cleanup hook path = %q", path)
		}
		if err := os.Rename(path, path+".owned"); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("owned"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() { preflightCleanupHook = nil })
	if err := writePreparedArtifacts(payloadPath, requestPath, []byte("owned"), []byte("request")); err == nil {
		t.Fatal("artifact collision unexpectedly succeeded")
	}
	raw, err := os.ReadFile(payloadPath)
	if err != nil || string(raw) != "owned" {
		t.Fatalf("replacement = %q, %v", raw, err)
	}
}

func TestAuthorityCutoverAcceptsDetachedCanonicalSignature(t *testing.T) {
	project := authorityFixture(t)
	request := readAuthorityRequestTest(t, authorityRequestFixture(t, project))
	raw, err := os.ReadFile(request.Approval.Path)
	var approval signedCutoverApproval
	if err != nil || json.Unmarshal(raw, &approval) != nil {
		t.Fatal(err)
	}
	approval.Signature = ""
	payload, _ := json.Marshal(approval)
	payloadPath := filepath.Join(project, "detached-approval.json")
	signaturePath := filepath.Join(project, "detached-approval.sig")
	if os.WriteFile(payloadPath, payload, 0o600) != nil || os.WriteFile(signaturePath, ed25519.Sign(ed25519.NewKeyFromSeed(bytes.Repeat([]byte{3}, ed25519.SeedSize)), payload), 0o600) != nil {
		t.Fatal("detached approval")
	}
	request.Approval = EvidenceReference{ID: approval.ID, Path: payloadPath, SHA256: digestBytes(payload)}
	request.ApprovalSignature = EvidenceReference{ID: approval.ID + "-signature", Path: signaturePath}
	request.ApprovalSignerKeyID = approval.SignerKeyID
	result, err := ExecuteAuthorityRequest(context.Background(), writeAuthorityJSON(t, project, "detached-request.json", request))
	if err != nil || result.Action != "cutover" {
		t.Fatalf("detached cutover = %#v, %v", result, err)
	}
}

func TestAuthorityCutoverReconcileStatusAndRollback(t *testing.T) {
	project := authorityFixture(t)
	request := authorityRequestFixture(t, project)

	result, err := ExecuteAuthorityRequest(context.Background(), request)
	if err != nil || !result.Held || result.Idempotent {
		t.Fatalf("cutover = %#v, %v", result, err)
	}
	status, err := AuthorityStatus(project)
	if err != nil || !status.Held || status.TargetRevision == "" || status.Tracker.TaskCount != 44 {
		t.Fatalf("status = %#v, %v", status, err)
	}
	result, err = ExecuteAuthorityRequest(context.Background(), request)
	if err != nil || !result.Idempotent {
		t.Fatalf("retry = %#v, %v", result, err)
	}

	reconcile := readAuthorityRequestTest(t, request)
	reconcile.Action = "reconcile"
	reconcile.ExpectedReceiptDigest = status.ReceiptDigest
	reconcilePath := writeAuthorityJSON(t, project, "reconcile.json", reconcile)
	result, err = ExecuteAuthorityRequest(context.Background(), reconcilePath)
	if err != nil || result.Held {
		t.Fatalf("reconcile = %#v, %v", result, err)
	}
	status, err = AuthorityStatus(project)
	if err != nil {
		t.Fatal(err)
	}
	archivePath := filepath.Join(project, filepath.FromSlash(status.ArchivePath))
	archiveBefore, err := os.Stat(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	archiveDigest := digestFileTest(t, archivePath)

	rollback := reconcile
	rollback.Action = "rollback"
	rollback.ExpectedReceiptDigest = result.ReceiptDigest
	rollbackPath := writeAuthorityJSON(t, project, "rollback.json", rollback)
	if _, err := ExecuteAuthorityRequest(context.Background(), rollbackPath); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{".agent-team/setup.json", ".agent-team/state.json"} {
		raw, err := os.ReadFile(filepath.Join(project, name))
		var legacy map[string]any
		if err != nil || json.Unmarshal(raw, &legacy) != nil || legacy["schemaVersion"] != float64(1) {
			t.Fatalf("%s not restored: %q %v", name, raw, err)
		}
	}
	result, err = ExecuteAuthorityRequest(context.Background(), request)
	if err != nil || !result.Held || result.Idempotent {
		t.Fatalf("reapply = %#v, %v", result, err)
	}
	archiveAfter, err := os.Stat(archivePath)
	if err != nil || !os.SameFile(archiveBefore, archiveAfter) || digestFileTest(t, archivePath) != archiveDigest {
		t.Fatalf("archive changed across rollback/reapply: %v", err)
	}
}

func TestAuthorityReapplyRejectsArchiveCollisionBeforeMutation(t *testing.T) {
	project := authorityFixture(t)
	request := authorityRequestFixture(t, project)
	result, err := ExecuteAuthorityRequest(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	reconcile := readAuthorityRequestTest(t, request)
	reconcile.Action, reconcile.ExpectedReceiptDigest = "reconcile", result.ReceiptDigest
	result, err = ExecuteAuthorityRequest(context.Background(), writeAuthorityJSON(t, project, "reconcile.json", reconcile))
	if err != nil {
		t.Fatal(err)
	}
	status, err := AuthorityStatus(project)
	if err != nil {
		t.Fatal(err)
	}
	rollback := reconcile
	rollback.Action, rollback.ExpectedReceiptDigest = "rollback", result.ReceiptDigest
	if _, err := ExecuteAuthorityRequest(context.Background(), writeAuthorityJSON(t, project, "rollback.json", rollback)); err != nil {
		t.Fatal(err)
	}
	archivePath := filepath.Join(project, filepath.FromSlash(status.ArchivePath))
	collision := []byte("foreign archive\n")
	if err := os.WriteFile(archivePath, collision, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ExecuteAuthorityRequest(context.Background(), request); !errors.Is(err, core.ErrRevision) {
		t.Fatalf("collision reapply = %v", err)
	}
	if raw, err := os.ReadFile(archivePath); err != nil || !bytes.Equal(raw, collision) {
		t.Fatalf("collision changed: %q, %v", raw, err)
	}
	if _, err := os.Stat(filepath.Join(project, filepath.FromSlash(authorityJournalPath))); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("journal exists after rejected collision: %v", err)
	}
	for _, name := range []string{".agent-team/setup.json", ".agent-team/state.json"} {
		raw, err := os.ReadFile(filepath.Join(project, name))
		var legacy map[string]any
		if err != nil || json.Unmarshal(raw, &legacy) != nil || legacy["schemaVersion"] != float64(1) {
			t.Fatalf("%s mutated: %q, %v", name, raw, err)
		}
	}
}

func TestAuthorityReapplyReusesLegacyArchiveWithMutationLock(t *testing.T) {
	project := authorityFixture(t)
	request := authorityRequestFixture(t, project)
	result, err := ExecuteAuthorityRequest(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	reconcile := readAuthorityRequestTest(t, request)
	reconcile.Action, reconcile.ExpectedReceiptDigest = "reconcile", result.ReceiptDigest
	result, err = ExecuteAuthorityRequest(context.Background(), writeAuthorityJSON(t, project, "reconcile.json", reconcile))
	if err != nil {
		t.Fatal(err)
	}
	status, err := AuthorityStatus(project)
	if err != nil {
		t.Fatal(err)
	}
	rollback := reconcile
	rollback.Action, rollback.ExpectedReceiptDigest = "rollback", result.ReceiptDigest
	if _, err := ExecuteAuthorityRequest(context.Background(), writeAuthorityJSON(t, project, "rollback.json", rollback)); err != nil {
		t.Fatal(err)
	}
	archivePath := filepath.Join(project, filepath.FromSlash(status.ArchivePath))
	var archive authorityArchive
	archiveRaw, err := os.ReadFile(archivePath)
	if err != nil || json.Unmarshal(archiveRaw, &archive) != nil {
		t.Fatalf("read archive: %v", err)
	}
	owner := store.MutationOwner{Token: strings.Repeat("a", 32), Scope: "authority-cutover", OperationID: reconcile.OperationID + ":cutover", Host: "legacy-host", PID: 1, ProcessStart: "legacy-process", AcquiredAt: "2026-01-01T00:00:00Z", HeartbeatAt: "2026-01-01T00:00:00Z"}
	ownerRaw, err := json.Marshal(owner)
	if err != nil {
		t.Fatal(err)
	}
	ownerRaw = append(ownerRaw, '\n')
	archive.Files = append(archive.Files, authorityPreimage{Path: ".agent-team/mutation.lock", Mode: 0o600, SHA256: digestBytes(ownerRaw), Bytes: ownerRaw})
	sort.Slice(archive.Files, func(i, j int) bool { return archive.Files[i].Path < archive.Files[j].Path })
	if _, err := store.New(project, core.StorageLimits{CanonicalBytes: authorityLimit}).WriteJSON(status.ArchivePath, archive, authorityLimit); err != nil {
		t.Fatal(err)
	}
	archiveBefore, err := os.Stat(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	archiveDigest := digestFileTest(t, archivePath)
	result, err = ExecuteAuthorityRequest(context.Background(), request)
	if err != nil || !result.Held {
		t.Fatalf("legacy archive reapply = %#v, %v", result, err)
	}
	archiveAfter, err := os.Stat(archivePath)
	if err != nil || !os.SameFile(archiveBefore, archiveAfter) || digestFileTest(t, archivePath) != archiveDigest {
		t.Fatalf("legacy archive changed during reapply: %v", err)
	}
}

func TestAuthorityReapplyCrashRecoveryRetainsReusedArchive(t *testing.T) {
	project := authorityFixture(t)
	request := authorityRequestFixture(t, project)
	result, err := ExecuteAuthorityRequest(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	reconcile := readAuthorityRequestTest(t, request)
	reconcile.Action, reconcile.ExpectedReceiptDigest = "reconcile", result.ReceiptDigest
	result, err = ExecuteAuthorityRequest(context.Background(), writeAuthorityJSON(t, project, "reconcile.json", reconcile))
	if err != nil {
		t.Fatal(err)
	}
	status, err := AuthorityStatus(project)
	if err != nil {
		t.Fatal(err)
	}
	rollback := reconcile
	rollback.Action, rollback.ExpectedReceiptDigest = "rollback", result.ReceiptDigest
	if _, err := ExecuteAuthorityRequest(context.Background(), writeAuthorityJSON(t, project, "rollback.json", rollback)); err != nil {
		t.Fatal(err)
	}
	archivePath := filepath.Join(project, filepath.FromSlash(status.ArchivePath))
	archiveBefore, err := os.Stat(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	archiveDigest := digestFileTest(t, archivePath)
	authorityMutationHook = func(index int) bool { return index == 0 }
	if _, err := ExecuteAuthorityRequest(context.Background(), request); !errors.Is(err, core.ErrTransition) {
		t.Fatalf("interrupted reapply = %v", err)
	}
	archiveAfterCrash, err := os.Stat(archivePath)
	if err != nil || !os.SameFile(archiveBefore, archiveAfterCrash) || digestFileTest(t, archivePath) != archiveDigest {
		t.Fatalf("reused archive changed by interrupted reapply: %v", err)
	}
	authorityMutationHook = nil
	t.Cleanup(func() { authorityMutationHook = nil })
	result, err = ExecuteAuthorityRequest(context.Background(), request)
	if err != nil || !result.Held {
		t.Fatalf("recovered reapply = %#v, %v", result, err)
	}
	archiveAfter, err := os.Stat(archivePath)
	if err != nil || !os.SameFile(archiveBefore, archiveAfter) || digestFileTest(t, archivePath) != archiveDigest {
		t.Fatalf("reused archive changed during recovery: %v", err)
	}
}

func TestAuthorityCutoverRejectsForeignLegacyAndStaleEvidence(t *testing.T) {
	project := authorityFixture(t)
	request := authorityRequestFixture(t, project)
	if err := os.WriteFile(filepath.Join(project, ".agent-team", "state.json"), []byte("foreign\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ExecuteAuthorityRequest(context.Background(), request); err == nil {
		t.Fatal("foreign legacy state accepted")
	}
	if _, err := os.Stat(filepath.Join(project, authorityReceiptPath)); !os.IsNotExist(err) {
		t.Fatal("failed validation mutated authority")
	}

	project = authorityFixture(t)
	request = authorityRequestFixture(t, project)
	candidate := readAuthorityRequestTest(t, request)
	candidate.TargetRevision = "0000000000000000000000000000000000000000"
	request = writeAuthorityJSON(t, project, "stale.json", candidate)
	if _, err := ExecuteAuthorityRequest(context.Background(), request); err == nil {
		t.Fatal("stale head accepted")
	}
}

func TestAuthorityCutoverRejectsForgedApprovalAndRemoteObservation(t *testing.T) {
	project := authorityFixture(t)
	requestPath := authorityRequestFixture(t, project)
	request := readAuthorityRequestTest(t, requestPath)
	request.Approval.ID = "caller-invented"
	if _, err := ExecuteAuthorityRequest(context.Background(), writeAuthorityJSON(t, project, "forged.json", request)); err == nil {
		t.Fatal("forged approval accepted")
	}
	request = readAuthorityRequestTest(t, requestPath)
	var signed signedCutoverApproval
	signedRaw, _ := os.ReadFile(request.Approval.Path)
	if json.Unmarshal(signedRaw, &signed) != nil {
		t.Fatal("approval")
	}
	signed.Cause = "forged cause with retained signature"
	forgedPath := writeAuthorityJSON(t, project, "forged-signature.json", signed)
	request.Approval.Path, request.Approval.SHA256 = forgedPath, digestFileTest(t, forgedPath)
	if _, err := ExecuteAuthorityRequest(context.Background(), writeAuthorityJSON(t, project, "forged-signature-request.json", request)); err == nil {
		t.Fatal("forged signature accepted")
	}
	request = readAuthorityRequestTest(t, requestPath)
	if err := os.WriteFile(filepath.Join(project, "approval.json"), []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ExecuteAuthorityRequest(context.Background(), writeAuthorityJSON(t, project, "tampered.json", request)); err == nil {
		t.Fatal("tampered approval accepted")
	}
	requestPath = authorityRequestFixture(t, project)

	request = readAuthorityRequestTest(t, requestPath)
	authorityRemoteObserver = func(context.Context, string, string, string, string) (RemoteObservation, error) {
		return RemoteObservation{Name: "origin", BaseRef: "refs/heads/main", BaseRevision: strings.Repeat("0", 40), TargetRef: "refs/heads/main", TargetRevision: strings.Repeat("0", 40)}, nil
	}
	if _, err := ExecuteAuthorityRequest(context.Background(), writeAuthorityJSON(t, project, "remote-mismatch.json", request)); err == nil {
		t.Fatal("remote mismatch accepted")
	}
	authorityRemoteObserver = func(context.Context, string, string, string, string) (RemoteObservation, error) {
		return RemoteObservation{}, errors.New("unavailable")
	}
	if _, err := ExecuteAuthorityRequest(context.Background(), writeAuthorityJSON(t, project, "remote-unavailable.json", request)); err == nil {
		t.Fatal("unavailable remote accepted")
	}
}

func TestAuthorityCutoverRejectsUnsignedEvidenceChanges(t *testing.T) {
	project := authorityFixture(t)
	requestPath := authorityRequestFixture(t, project)
	request := readAuthorityRequestTest(t, requestPath)
	request.Reviews[0] = EvidenceReference{ID: request.Reviews[0].ID, Path: request.Tests[0].Path, SHA256: request.Tests[0].SHA256}
	if _, err := ExecuteAuthorityRequest(context.Background(), writeAuthorityJSON(t, project, "path-swap.json", request)); err == nil {
		t.Fatal("signed review path swap accepted")
	}
	request = readAuthorityRequestTest(t, requestPath)
	request.Tests = append(request.Tests, request.Tests[0])
	if _, err := ExecuteAuthorityRequest(context.Background(), writeAuthorityJSON(t, project, "duplicate-evidence.json", request)); err == nil {
		t.Fatal("duplicate signed evidence accepted")
	}
	request = readAuthorityRequestTest(t, requestPath)
	request.Tests[0], request.Tests[1] = request.Tests[1], request.Tests[0]
	if _, err := ExecuteAuthorityRequest(context.Background(), writeAuthorityJSON(t, project, "reordered-evidence.json", request)); err == nil {
		t.Fatal("reordered signed evidence accepted")
	}
	request = readAuthorityRequestTest(t, requestPath)
	request.Reviews = request.Reviews[:1]
	if _, err := ExecuteAuthorityRequest(context.Background(), writeAuthorityJSON(t, project, "omitted-evidence.json", request)); err == nil {
		t.Fatal("omitted signed evidence accepted")
	}
	request = readAuthorityRequestTest(t, requestPath)
	if err := os.WriteFile(request.Readiness.Path, []byte("stale\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ExecuteAuthorityRequest(context.Background(), writeAuthorityJSON(t, project, "stale-evidence.json", request)); err == nil {
		t.Fatal("stale signed evidence accepted")
	}
}

func TestAuthorityCutoverRejectsNoncanonicalTaskIDsBeforeMutation(t *testing.T) {
	tests := []struct {
		name          string
		mutateRequest func(*AuthorityRequest)
		mutateSigned  func(*signedCutoverApproval)
	}{
		{name: "reordered signed payload", mutateSigned: func(approval *signedCutoverApproval) {
			approval.TaskIDs[1], approval.TaskIDs[2] = approval.TaskIDs[2], approval.TaskIDs[1]
		}},
		{name: "duplicate signed payload", mutateSigned: func(approval *signedCutoverApproval) {
			approval.TaskIDs[2] = approval.TaskIDs[1]
		}},
		{name: "reordered request", mutateRequest: func(request *AuthorityRequest) {
			request.Tracker.TaskIDs[1], request.Tracker.TaskIDs[2] = request.Tracker.TaskIDs[2], request.Tracker.TaskIDs[1]
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			project := authorityFixture(t)
			requestPath := authorityRequestFixture(t, project)
			request := readAuthorityRequestTest(t, requestPath)
			if test.mutateSigned != nil {
				resignAuthorityApprovalTest(t, project, &request, test.mutateSigned)
			}
			if test.mutateRequest != nil {
				test.mutateRequest(&request)
			}
			if _, err := ExecuteAuthorityRequest(context.Background(), writeAuthorityJSON(t, project, "noncanonical-request.json", request)); err == nil {
				t.Fatal("noncanonical task IDs accepted")
			}
			assertLegacyAuthorityUnchanged(t, project)
		})
	}
}

func TestAuthorityCutoverRejectsReplayAndRequestSelectedLegacyAuthority(t *testing.T) {
	project := authorityFixture(t)
	requestPath := authorityRequestFixture(t, project)
	request := readAuthorityRequestTest(t, requestPath)
	request.OperationID = "replayed-operation"
	if _, err := ExecuteAuthorityRequest(context.Background(), writeAuthorityJSON(t, project, "replay.json", request)); err == nil {
		t.Fatal("approval replay under another operation accepted")
	}
	raw, _ := os.ReadFile(requestPath)
	var body map[string]any
	if json.Unmarshal(raw, &body) != nil {
		t.Fatal("request")
	}
	body["legacy"] = []any{map[string]any{"path": ".agent-team/state.json", "sha256": strings.Repeat("a", 64)}}
	if _, err := ExecuteAuthorityRequest(context.Background(), writeAuthorityJSON(t, project, "caller-legacy.json", body)); err == nil {
		t.Fatal("request-selected legacy authority accepted")
	}
}

func TestAuthorityCutoverRejectsMutableOperatorTrustStore(t *testing.T) {
	project := authorityFixture(t)
	requestPath := authorityRequestFixture(t, project)
	authorityTrustStoreOwner = func(string, os.FileInfo) bool { return false }
	if _, err := ExecuteAuthorityRequest(context.Background(), requestPath); err == nil {
		t.Fatal("mutable operator trust store accepted")
	}
}

func TestSystemTrustStoreRejectsWritableFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "trust.json")
	if err := os.WriteFile(path, []byte("{}\n"), 0o666); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o666); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	trusted := systemTrustStoreOwner(path, info)
	if runtime.GOOS == "windows" {
		// Windows ignores POSIX chmod bits. Its trust decision comes from the
		// owner/DACL contract covered by TestTrustedWindowsACL.
		if trustedWindowsACL(true, true, []windowsTrustACE{{allow: true, known: true, mask: 0x40000000}}) {
			t.Fatal("untrusted Windows writer accepted")
		}
		return
	}
	if trusted {
		t.Fatal("group/world-writable trust store accepted")
	}
}

func TestAuthorityCutoverRechecksRemoteBeforeReceiptPublication(t *testing.T) {
	project := authorityFixture(t)
	requestPath := authorityRequestFixture(t, project)
	request := readAuthorityRequestTest(t, requestPath)
	var calls int
	authorityRemoteObserver = func(context.Context, string, string, string, string) (RemoteObservation, error) {
		calls++
		remote := RemoteObservation{Name: "origin", URL: "file:///trusted/origin.git", BaseRef: "refs/heads/main", BaseRevision: request.TargetRevision, TargetRef: "refs/heads/main", TargetRevision: request.TargetRevision}
		if calls == 2 {
			remote.URL = "file:///foreign/origin.git"
		}
		return remote, nil
	}
	if _, err := ExecuteAuthorityRequest(context.Background(), requestPath); err == nil {
		t.Fatal("late remote change accepted")
	}
	if calls != 2 {
		t.Fatalf("remote observations = %d", calls)
	}
	if _, err := os.Stat(filepath.Join(project, authorityReceiptPath)); !os.IsNotExist(err) {
		t.Fatal("failed cutover left authority receipt")
	}
	raw, _ := os.ReadFile(filepath.Join(project, ".agent-team", "state.json"))
	var state map[string]any
	if json.Unmarshal(raw, &state) != nil || state["schemaVersion"] != float64(1) {
		t.Fatalf("legacy state not restored: %s", raw)
	}
}

func TestAuthorityCutoverRechecksOperatorTrustBeforeReceiptPublication(t *testing.T) {
	project := authorityFixture(t)
	requestPath := authorityRequestFixture(t, project)
	authorityMutationHook = func(index int) bool {
		if index == 1 {
			authorityTrustStoreOwner = func(string, os.FileInfo) bool { return false }
		}
		return false
	}
	t.Cleanup(func() { authorityMutationHook = nil })
	if _, err := ExecuteAuthorityRequest(context.Background(), requestPath); err == nil {
		t.Fatal("late operator trust change accepted")
	}
	if _, err := os.Stat(filepath.Join(project, authorityReceiptPath)); !os.IsNotExist(err) {
		t.Fatal("failed cutover left authority receipt")
	}
	raw, _ := os.ReadFile(filepath.Join(project, ".agent-team", "state.json"))
	var state map[string]any
	if json.Unmarshal(raw, &state) != nil || state["schemaVersion"] != float64(1) {
		t.Fatalf("legacy state not restored: %s", raw)
	}
}

func TestAuthorityCutoverRejectsProjectLocalSelfPinnedApproval(t *testing.T) {
	project := authorityFixture(t)
	requestPath := authorityRequestFixture(t, project)
	request := readAuthorityRequestTest(t, requestPath)
	seed := bytes.Repeat([]byte{7}, ed25519.SeedSize)
	privateKey := ed25519.NewKeyFromSeed(seed)
	publicKey := privateKey.Public().(ed25519.PublicKey)
	keyID := digestBytes(publicKey)
	setupPath := filepath.Join(project, ".agent-team", "setup.json")
	var setup map[string]any
	setupRaw, _ := os.ReadFile(setupPath)
	if json.Unmarshal(setupRaw, &setup) != nil {
		t.Fatal("setup")
	}
	setup["cutoverApproval"] = signedApprovalTrust{Algorithm: "ed25519", KeyID: keyID, PublicKey: base64.StdEncoding.EncodeToString(publicKey)}
	writeAuthorityJSON(t, filepath.Join(project, ".agent-team"), "setup.json", setup)
	writeAuthorityJSON(t, filepath.Join(project, ".agent-team"), "state.json", map[string]any{"schemaVersion": 1, "operationReceipts": map[string]any{"self-pinned": map[string]any{"signature": strings.Repeat("a", 64)}}})
	issued := time.Now().UTC()
	approval := signedCutoverApproval{Schema: 1, ID: "user-approved-v8-cutover", IssuedAt: issued.Format(time.RFC3339Nano), ExpiresAt: issued.Add(time.Hour).Format(time.RFC3339Nano), Project: request.Project, OperationID: request.OperationID, TargetRevision: request.TargetRevision, TrackerFingerprint: request.Tracker.Fingerprint, ParentID: request.Tracker.ParentID, Cause: "publish the independently reviewed 43-task v8 implementation", RecoveryDisposition: "restore archived v7 state and leave the remote unchanged", SignerKeyID: keyID, TaskIDs: request.Tracker.TaskIDs, Reviews: request.Reviews, Tests: request.Tests, Readiness: request.Readiness, Remote: RemoteObservation{Name: "origin", URL: "file:///trusted/origin.git", BaseRef: "refs/heads/main", BaseRevision: request.TargetRevision, TargetRef: "refs/heads/main", TargetRevision: request.TargetRevision}, RemoteMainDeploys: true}
	payload, _ := json.Marshal(approval)
	approval.Signature = base64.StdEncoding.EncodeToString(ed25519.Sign(privateKey, payload))
	approvalPath := writeAuthorityJSON(t, project, "signed-approval.json", approval)
	request.Approval = EvidenceReference{ID: approval.ID, Path: approvalPath, SHA256: digestFileTest(t, approvalPath)}
	authorityTrustStorePath = filepath.Join(t.TempDir(), "missing-trust.json")
	forged := approval
	forged.Cause = "caller supplied cause"
	forgedPath := writeAuthorityJSON(t, project, "forged-signed-approval.json", forged)
	forgedRequest := request
	forgedRequest.Approval = EvidenceReference{Path: forgedPath, SHA256: digestFileTest(t, forgedPath)}
	if _, err := ExecuteAuthorityRequest(context.Background(), writeAuthorityJSON(t, project, "forged-signed-request.json", forgedRequest)); err == nil {
		t.Fatal("forged signed approval accepted")
	}
	requestPath = writeAuthorityJSON(t, project, "signed-request.json", request)
	if _, err := ExecuteAuthorityRequest(context.Background(), requestPath); err == nil {
		t.Fatal("project-local self-pinned approval accepted")
	}
}

func TestObserveGitRemoteReadsConfiguredRefs(t *testing.T) {
	project := authorityFixture(t)
	remote := filepath.Join(t.TempDir(), "origin.git")
	if output, err := exec.Command("git", "clone", "-q", "--bare", project, remote).CombinedOutput(); err != nil {
		t.Fatalf("clone: %s: %v", output, err)
	}
	if output, err := exec.Command("git", "-C", project, "remote", "add", "origin", remote).CombinedOutput(); err != nil {
		t.Fatalf("remote: %s: %v", output, err)
	}
	head, _ := exec.Command("git", "-C", project, "rev-parse", "HEAD").Output()
	got, err := observeGitRemote(context.Background(), project, "origin", "refs/heads/master", "refs/heads/master")
	if err != nil || got.URL != remote || got.BaseRevision != string(bytesTrim(head)) || got.TargetRevision != got.BaseRevision || got.TargetAbsent {
		t.Fatalf("observation = %#v, %v", got, err)
	}
}

func TestAuthorityCutoverRecoversInterruptedMutation(t *testing.T) {
	project := authorityFixture(t)
	request := authorityRequestFixture(t, project)
	authorityMutationHook = func(index int) bool { return index == 0 }
	if _, err := ExecuteAuthorityRequest(context.Background(), request); !errors.Is(err, core.ErrTransition) {
		t.Fatalf("interruption = %v", err)
	}
	authorityMutationHook = nil
	t.Cleanup(func() { authorityMutationHook = nil })
	result, err := ExecuteAuthorityRequest(context.Background(), request)
	if err != nil || !result.Held {
		t.Fatalf("recovered cutover = %#v, %v", result, err)
	}
}

func TestAuthorityReconcileRecoversInterruptedMutation(t *testing.T) {
	project := authorityFixture(t)
	request := authorityRequestFixture(t, project)
	if _, err := ExecuteAuthorityRequest(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	status, _ := AuthorityStatus(project)
	reconcile := readAuthorityRequestTest(t, request)
	reconcile.Action, reconcile.ExpectedReceiptDigest = "reconcile", status.ReceiptDigest
	reconcilePath := writeAuthorityJSON(t, project, "reconcile-crash.json", reconcile)
	authorityMutationHook = func(index int) bool { return index == 1 }
	if _, err := ExecuteAuthorityRequest(context.Background(), reconcilePath); !errors.Is(err, core.ErrTransition) {
		t.Fatalf("interruption = %v", err)
	}
	authorityMutationHook = nil
	t.Cleanup(func() { authorityMutationHook = nil })
	result, err := ExecuteAuthorityRequest(context.Background(), reconcilePath)
	if err != nil || result.Held {
		t.Fatalf("recovered reconcile = %#v, %v", result, err)
	}
}

func authorityFixture(t *testing.T) string {
	t.Helper()
	project := t.TempDir()
	if err := os.MkdirAll(filepath.Join(project, ".agent-team"), 0o700); err != nil {
		t.Fatal(err)
	}
	legacy := map[string]any{"setup.json": map[string]any{"schemaVersion": 1, "version": 6, "tracker": map[string]any{"kind": "markdown", "path": "TASKS.md"}}, "state.json": map[string]any{"schemaVersion": 1, "stateVersion": 59, "integration": map[string]any{"hold": true}, "release": map[string]any{"hold": true}}}
	for name, value := range legacy {
		raw, _ := json.Marshal(value)
		if err := os.WriteFile(filepath.Join(project, ".agent-team", name), append(raw, '\n'), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	cmd := exec.Command("git", "init", "-q", project)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init: %s: %v", output, err)
	}
	for _, args := range [][]string{{"-C", project, "config", "user.email", "fixture@example.invalid"}, {"-C", project, "config", "user.name", "Fixture"}, {"-C", project, "add", "."}, {"-C", project, "commit", "-qm", "fixture"}} {
		if output, err := exec.Command("git", args...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %s: %v", args, output, err)
		}
	}
	return project
}

func authorityRequestFixture(t *testing.T, project string) string {
	t.Helper()
	head, err := exec.Command("git", "-C", project, "rev-parse", "HEAD").Output()
	if err != nil {
		t.Fatal(err)
	}
	tasks := make([]map[string]any, 0, 44)
	tasks = append(tasks, map[string]any{"id": "atv-5sh", "status": "closed"})
	ids := []string{"atv-5sh"}
	for index := 1; index <= 43; index++ {
		id := "atv-5sh." + itoa(index)
		tasks = append(tasks, map[string]any{"id": id, "status": "closed", "parent": "atv-5sh"})
		ids = append(ids, id)
	}
	sort.Strings(ids)
	tracker := writeAuthorityJSON(t, project, "tracker.json", tasks)
	testSource := filepath.Join(project, "tests.json")
	if err := os.WriteFile(testSource, []byte("{\"Action\":\"pass\",\"Package\":\"example\",\"Test\":\"TestCutover\"}\n{\"Action\":\"pass\",\"Package\":\"example\"}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	testSourceB := filepath.Join(project, "tests-b.json")
	if err := os.WriteFile(testSourceB, []byte("{\"Action\":\"pass\",\"Package\":\"example/b\",\"Test\":\"TestCutoverB\"}\n{\"Action\":\"pass\",\"Package\":\"example/b\"}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	review := map[string]any{"phase": "cutover", "revision": string(bytesTrim(head)), "author": "builder", "reviewer": "independent", "source": testSource, "digest": digestFileTest(t, testSource), "result": "CLEAN"}
	reviewPath := writeAuthorityJSON(t, project, "review.json", review)
	reviewB := map[string]any{"phase": "cutover", "revision": string(bytesTrim(head)), "author": "builder-b", "reviewer": "independent-b", "source": testSourceB, "digest": digestFileTest(t, testSourceB), "result": "CLEAN"}
	reviewPathB := writeAuthorityJSON(t, project, "review-b.json", reviewB)
	evidencePaths := map[string]string{}
	for _, name := range []string{"benchmark", "artifact", "sbom", "canary", "rollback", "provider", "installed"} {
		path := filepath.Join(project, name+"-evidence.json")
		if err := os.WriteFile(path, []byte(name+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		evidencePaths[name] = path
	}
	gates := release.Gates{Phase1: true, Phase2: true, Phase3: true, Phase4: true, Phase5: true, Deploy: true, Install: true, ReviewsClean: true, Native: true, Benchmark: true, Artifacts: true, Canaries: true, Rollback: true, Provider: true, Cutover: true}
	gatesPath := writeAuthorityJSON(t, project, "gates.json", gates)
	readiness := filepath.Join(project, "readiness.json")
	readinessValue, err := release.BuildReadinessEvidence(string(bytesTrim(head)), gatesPath, evidencePaths)
	if err != nil || release.WriteReadinessEvidence(readiness, readinessValue) != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	remote := RemoteObservation{Name: "origin", URL: "file:///trusted/origin.git", BaseRef: "refs/heads/main", BaseRevision: string(bytesTrim(head)), TargetRef: "refs/heads/main", TargetRevision: string(bytesTrim(head)), ObservedAt: now}
	authorityRemoteObserver = func(context.Context, string, string, string, string) (RemoteObservation, error) { return remote, nil }
	t.Cleanup(func() { authorityRemoteObserver = observeGitRemote })
	reviews := []EvidenceReference{{ID: "review-a", Path: reviewPath, SHA256: digestFileTest(t, reviewPath)}, {ID: "review-b", Path: reviewPathB, SHA256: digestFileTest(t, reviewPathB)}}
	tests := []EvidenceReference{{ID: "go-test-a", Path: testSource, SHA256: digestFileTest(t, testSource)}, {ID: "go-test-b", Path: testSourceB, SHA256: digestFileTest(t, testSourceB)}}
	readinessRef := EvidenceReference{ID: "release-readiness", Path: readiness, SHA256: digestFileTest(t, readiness)}
	seed := bytes.Repeat([]byte{3}, ed25519.SeedSize)
	privateKey := ed25519.NewKeyFromSeed(seed)
	publicKey := privateKey.Public().(ed25519.PublicKey)
	keyID := digestBytes(publicKey)
	trustPath := writeAuthorityJSON(t, t.TempDir(), "cutover-trust.json", operatorTrustStore{Schema: 1, Keys: []signedApprovalTrust{{Algorithm: "ed25519", KeyID: keyID, PublicKey: base64.StdEncoding.EncodeToString(publicKey)}}})
	authorityTrustStorePath = trustPath
	authorityTrustStoreOwner = func(string, os.FileInfo) bool { return true }
	t.Cleanup(func() {
		authorityTrustStorePath = systemTrustStorePath()
		authorityTrustStoreOwner = systemTrustStoreOwner
	})
	issued := time.Now().UTC()
	approval := signedCutoverApproval{Schema: 1, ID: "trusted-operator-approval", IssuedAt: issued.Format(time.RFC3339Nano), ExpiresAt: issued.Add(time.Hour).Format(time.RFC3339Nano), Project: project, OperationID: "cutover-fixture", TargetRevision: string(bytesTrim(head)), TrackerFingerprint: digestFileTest(t, tracker), ParentID: "atv-5sh", Cause: "explicit user-approved legacy-v8 cutover", RecoveryDisposition: "restore archived v7 authority and keep remote unchanged", SignerKeyID: keyID, TaskIDs: ids, Reviews: reviews, Tests: tests, Readiness: readinessRef, Remote: remote, RemoteMainDeploys: true}
	payload, _ := json.Marshal(approval)
	approval.Signature = base64.StdEncoding.EncodeToString(ed25519.Sign(privateKey, payload))
	approvalPath := writeAuthorityJSON(t, project, "approval.json", approval)
	request := AuthorityRequest{
		Schema: 1, Action: "cutover", Project: project, OperationID: "cutover-fixture", TargetRevision: string(bytesTrim(head)),
		Tracker: TrackerAuthority{Export: tracker, SHA256: digestFileTest(t, tracker), Fingerprint: digestFileTest(t, tracker), ParentID: "atv-5sh", TaskIDs: ids, TaskCount: 44},
		Reviews: reviews, Tests: tests, Readiness: readinessRef,
		Approval: EvidenceReference{ID: approval.ID, Path: approvalPath, SHA256: digestFileTest(t, approvalPath)},
	}
	return writeAuthorityJSON(t, project, "request.json", request)
}

func resignAuthorityApprovalTest(t *testing.T, project string, request *AuthorityRequest, mutate func(*signedCutoverApproval)) {
	t.Helper()
	raw, err := os.ReadFile(request.Approval.Path)
	var approval signedCutoverApproval
	if err != nil || json.Unmarshal(raw, &approval) != nil {
		t.Fatal(err)
	}
	mutate(&approval)
	approval.Signature = ""
	payload, err := json.Marshal(approval)
	if err != nil {
		t.Fatal(err)
	}
	privateKey := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{3}, ed25519.SeedSize))
	approval.Signature = base64.StdEncoding.EncodeToString(ed25519.Sign(privateKey, payload))
	path := writeAuthorityJSON(t, project, "noncanonical-approval.json", approval)
	request.Approval.Path = path
	request.Approval.SHA256 = digestFileTest(t, path)
}

func assertLegacyAuthorityUnchanged(t *testing.T, project string) {
	t.Helper()
	if _, err := os.Stat(filepath.Join(project, authorityReceiptPath)); !os.IsNotExist(err) {
		t.Fatal("rejected cutover left an authority receipt")
	}
	raw, err := os.ReadFile(filepath.Join(project, ".agent-team", "state.json"))
	var state map[string]any
	if err != nil || json.Unmarshal(raw, &state) != nil || state["schemaVersion"] != float64(1) {
		t.Fatalf("legacy state changed: %s (%v)", raw, err)
	}
}

func readAuthorityRequestTest(t *testing.T, path string) AuthorityRequest {
	t.Helper()
	var request AuthorityRequest
	raw, err := os.ReadFile(path)
	if err != nil || json.Unmarshal(raw, &request) != nil {
		t.Fatal(err)
	}
	return request
}

func writeAuthorityJSON(t *testing.T, project, name string, value any) string {
	t.Helper()
	path := filepath.Join(project, name)
	raw, err := json.Marshal(value)
	if err != nil || os.WriteFile(path, append(raw, '\n'), 0o600) != nil {
		t.Fatal(err)
	}
	return path
}

func digestFileTest(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func bytesTrim(value []byte) []byte {
	for len(value) > 0 && (value[len(value)-1] == '\n' || value[len(value)-1] == '\r') {
		value = value[:len(value)-1]
	}
	return value
}

func itoa(value int) string {
	const digits = "0123456789"
	if value < 10 {
		return string(digits[value])
	}
	return string([]byte{digits[value/10], digits[value%10]})
}
