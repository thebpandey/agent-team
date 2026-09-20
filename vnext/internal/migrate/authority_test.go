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
	"strings"
	"testing"
	"time"

	"github.com/thebpandey/agent-team/vnext/internal/core"
	"github.com/thebpandey/agent-team/vnext/internal/release"
)

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
	reapplyPath := authorityRequestFixture(t, project)
	result, err = ExecuteAuthorityRequest(context.Background(), reapplyPath)
	if err != nil || !result.Held || result.Idempotent {
		t.Fatalf("reapply = %#v, %v", result, err)
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
	if systemTrustStoreOwner(path, info) {
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
