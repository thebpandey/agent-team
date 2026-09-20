package migrate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

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
	request := AuthorityRequest{
		Schema: 1, Action: "cutover", Project: project, OperationID: "cutover-fixture", TargetRevision: string(bytesTrim(head)),
		Tracker: TrackerAuthority{Export: tracker, SHA256: digestFileTest(t, tracker), Fingerprint: digestFileTest(t, tracker), ParentID: "atv-5sh", TaskIDs: ids, TaskCount: 44},
		Reviews: []EvidenceReference{{Path: reviewPath, SHA256: digestFileTest(t, reviewPath)}},
		Tests:   []EvidenceReference{{Path: testSource, SHA256: digestFileTest(t, testSource)}}, Readiness: EvidenceReference{Path: readiness, SHA256: digestFileTest(t, readiness)},
		Legacy:              []OwnedLegacyFile{{Path: ".agent-team/setup.json", SHA256: digestFileTest(t, filepath.Join(project, ".agent-team/setup.json"))}, {Path: ".agent-team/state.json", SHA256: digestFileTest(t, filepath.Join(project, ".agent-team/state.json"))}},
		Remote:              RemoteObservation{Name: "origin", BaseRef: "refs/heads/main", BaseRevision: string(bytesTrim(head)), TargetRef: "refs/heads/main", TargetRevision: string(bytesTrim(head)), ObservedAt: "2026-09-20T00:00:00Z"},
		Authorization:       CutoverAuthorization{GrantedBy: "user", Source: "explicit request", Cause: "legacy-v7-authority", Scope: "atv-5sh and 43 children", GrantedAt: "2026-09-20T00:00:00Z", Revision: string(bytesTrim(head)), TaskIDs: ids, RemoteMainDeploys: true},
		RecoveryDisposition: "restore archived v7 authority and keep remote unchanged",
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
