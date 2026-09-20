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
	reapply := readAuthorityRequestTest(t, request)
	reapply.OperationID = "cutover-reapply"
	reapplyPath := writeAuthorityJSON(t, project, "reapply.json", reapply)
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
	request.ApprovalOperationID = "caller-invented"
	if _, err := ExecuteAuthorityRequest(context.Background(), writeAuthorityJSON(t, project, "forged.json", request)); err == nil {
		t.Fatal("forged approval accepted")
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

func TestAuthorityCutoverRechecksRemoteBeforeReceiptPublication(t *testing.T) {
	project := authorityFixture(t)
	requestPath := authorityRequestFixture(t, project)
	request := readAuthorityRequestTest(t, requestPath)
	var calls int
	authorityRemoteObserver = func(context.Context, string, string, string, string) (RemoteObservation, error) {
		calls++
		remote := RemoteObservation{Name: "origin", BaseRef: "refs/heads/main", BaseRevision: request.TargetRevision, TargetRef: "refs/heads/main", TargetRevision: request.TargetRevision}
		if calls == 2 {
			remote.TargetRevision = strings.Repeat("0", 40)
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

func TestAuthorityCutoverSignedApprovalBridgesStaleTrackerState(t *testing.T) {
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
	issued := time.Now().UTC()
	approval := signedCutoverApproval{Schema: 1, ID: "user-approved-v8-cutover", IssuedAt: issued.Format(time.RFC3339Nano), ExpiresAt: issued.Add(time.Hour).Format(time.RFC3339Nano), TargetRevision: request.TargetRevision, TrackerFingerprint: request.Tracker.Fingerprint, ParentID: request.Tracker.ParentID, Cause: "publish the independently reviewed 43-task v8 implementation", RecoveryDisposition: "restore archived v7 state and leave the remote unchanged", SignerKeyID: keyID, TaskIDs: request.Tracker.TaskIDs, Remote: RemoteObservation{Name: "origin", BaseRef: "refs/heads/main", BaseRevision: request.TargetRevision, TargetRef: "refs/heads/main", TargetRevision: request.TargetRevision}, RemoteMainDeploys: true}
	payload, _ := json.Marshal(approval)
	approval.Signature = base64.StdEncoding.EncodeToString(ed25519.Sign(privateKey, payload))
	approvalPath := writeAuthorityJSON(t, project, "signed-approval.json", approval)
	request.ApprovalOperationID = ""
	request.Approval = EvidenceReference{Path: approvalPath, SHA256: digestFileTest(t, approvalPath)}
	for index := range request.Legacy {
		if request.Legacy[index].Path == ".agent-team/setup.json" {
			request.Legacy[index].SHA256 = digestFileTest(t, setupPath)
		}
	}
	forged := approval
	forged.Cause = "caller supplied cause"
	forgedPath := writeAuthorityJSON(t, project, "forged-signed-approval.json", forged)
	forgedRequest := request
	forgedRequest.Approval = EvidenceReference{Path: forgedPath, SHA256: digestFileTest(t, forgedPath)}
	if _, err := ExecuteAuthorityRequest(context.Background(), writeAuthorityJSON(t, project, "forged-signed-request.json", forgedRequest)); err == nil {
		t.Fatal("forged signed approval accepted")
	}
	requestPath = writeAuthorityJSON(t, project, "signed-request.json", request)
	result, err := ExecuteAuthorityRequest(context.Background(), requestPath)
	if err != nil || !result.Held {
		t.Fatalf("signed cutover = %#v, %v", result, err)
	}
	receipt, err := AuthorityStatus(project)
	if err != nil || receipt.ApprovalOperationID != approval.ID || receipt.Authorization.GrantedBy != keyID || receipt.Authorization.Cause != approval.Cause || receipt.RecoveryDisposition != approval.RecoveryDisposition {
		t.Fatalf("signed receipt = %#v, %v", receipt, err)
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
	if err != nil || got.BaseRevision != string(bytesTrim(head)) || got.TargetRevision != got.BaseRevision || got.TargetAbsent {
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
	review := map[string]any{"phase": "cutover", "revision": string(bytesTrim(head)), "author": "builder", "reviewer": "independent", "source": testSource, "digest": digestFileTest(t, testSource), "result": "CLEAN"}
	reviewPath := writeAuthorityJSON(t, project, "review.json", review)
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
	approvalID := "trusted-integration-approval"
	now := time.Now().UTC().Format(time.RFC3339Nano)
	remote := RemoteObservation{Name: "origin", BaseRef: "refs/heads/main", BaseRevision: string(bytesTrim(head)), TargetRef: "refs/heads/main", TargetRevision: string(bytesTrim(head)), ObservedAt: now}
	authorityRemoteObserver = func(context.Context, string, string, string, string) (RemoteObservation, error) { return remote, nil }
	t.Cleanup(func() { authorityRemoteObserver = observeGitRemote })
	approvalEvidence := map[string]any{"status": "passed", "revision": string(bytesTrim(head)), "taskIds": ids,
		"remote":        map[string]any{"name": "origin", "baseRef": "refs/heads/main", "revision": string(bytesTrim(head)), "targetRef": "refs/heads/main", "targetRevision": string(bytesTrim(head))},
		"authorization": map[string]any{"source": "explicit user-approved legacy-v8 cutover", "scope": "integration", "ownerSessionId": "trusted-owner", "revision": string(bytesTrim(head)), "taskIds": ids},
		"recovery":      map[string]any{"status": "reconciled", "revision": string(bytesTrim(head)), "taskIds": ids, "action": "restore archived v7 authority and keep remote unchanged"}, "preview": map[string]any{"required": false}, "remoteMainDeploys": true}
	approvalPath := writeAuthorityJSON(t, project, "approval.json", approvalEvidence)
	legacyState := map[string]any{"schemaVersion": 1, "stateVersion": 59,
		"integration": map[string]any{"hold": true, "ownerSessionId": "trusted-owner", "recordedEvidence": map[string]any{"path": approvalPath, "fingerprint": digestFileTest(t, approvalPath), "revision": string(bytesTrim(head)), "taskIds": ids, "operationId": approvalID, "observedAt": now}},
		"release":     map[string]any{"hold": true}, "operationReceipts": map[string]any{approvalID: map[string]any{"signature": strings.Repeat("a", 64), "result": map[string]any{"gate": "integration", "trackerFingerprint": digestFileTest(t, tracker), "revision": string(bytesTrim(head))}, "appliedAt": now}}}
	writeAuthorityJSON(t, filepath.Join(project, ".agent-team"), "state.json", legacyState)
	request := AuthorityRequest{
		Schema: 1, Action: "cutover", Project: project, OperationID: "cutover-fixture", TargetRevision: string(bytesTrim(head)),
		Tracker: TrackerAuthority{Export: tracker, SHA256: digestFileTest(t, tracker), Fingerprint: digestFileTest(t, tracker), ParentID: "atv-5sh", TaskIDs: ids, TaskCount: 44},
		Reviews: []EvidenceReference{{Path: reviewPath, SHA256: digestFileTest(t, reviewPath)}},
		Tests:   []EvidenceReference{{Path: testSource, SHA256: digestFileTest(t, testSource)}}, Readiness: EvidenceReference{Path: readiness, SHA256: digestFileTest(t, readiness)},
		Legacy:              []OwnedLegacyFile{{Path: ".agent-team/setup.json", SHA256: digestFileTest(t, filepath.Join(project, ".agent-team/setup.json"))}, {Path: ".agent-team/state.json", SHA256: digestFileTest(t, filepath.Join(project, ".agent-team/state.json"))}},
		ApprovalOperationID: approvalID,
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
