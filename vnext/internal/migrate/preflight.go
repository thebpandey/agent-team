package migrate

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/thebpandey/agent-team/vnext/internal/core"
	"github.com/thebpandey/agent-team/vnext/internal/install"
	"github.com/thebpandey/agent-team/vnext/internal/release"
)

func prepareAuthority(ctx context.Context, request AuthorityRequest) (AuthorityResult, error) {
	prep := request.Prepare
	if ctx == nil || ctx.Err() != nil || request.Schema != 1 || prep == nil || request.OperationID == "" || prep.ApprovalID == "" || prep.SignerKeyID == "" || prep.Cause == "" || prep.RecoveryDisposition == "" || !absoluteOutput(prep.PayloadPath) || !absoluteOutput(prep.RequestPath) || !absoluteOutput(prep.SignaturePath) || !absoluteOutput(prep.NativeManifest) || len(prep.HostRoots) != 2 {
		return AuthorityResult{}, core.ErrPhase
	}
	if prep.PayloadPath == prep.RequestPath || prep.PayloadPath == prep.SignaturePath || prep.RequestPath == prep.SignaturePath {
		return AuthorityResult{}, core.ErrPath
	}
	project, err := cleanProject(request.Project)
	if err != nil || project != request.Project || validateLegacyAuthority(project) != nil {
		return AuthorityResult{}, core.ErrRevision
	}
	head, err := gitHead(ctx, project)
	if err != nil || request.TargetRevision != "" && request.TargetRevision != head {
		return AuthorityResult{}, fmt.Errorf("%w: target git HEAD changed", core.ErrRevision)
	}
	request.TargetRevision = head
	request.Tracker.Export, err = projectEvidencePath(project, request.Tracker.Export)
	if err != nil {
		return AuthorityResult{}, err
	}
	trackerRaw, err := readAbsoluteBounded(request.Tracker.Export)
	if err != nil {
		return AuthorityResult{}, err
	}
	request.Tracker.SHA256 = digestBytes(trackerRaw)
	request.Tracker.Fingerprint = request.Tracker.SHA256
	request.Tracker.TaskIDs, err = closedTrackerIDs(trackerRaw, request.Tracker.ParentID)
	if err != nil {
		return AuthorityResult{}, err
	}
	request.Tracker.TaskCount = len(request.Tracker.TaskIDs)
	if err := validateTracker(request.Tracker); err != nil {
		return AuthorityResult{}, err
	}
	if request.Reviews, err = prepareEvidence(project, request.Reviews); err != nil {
		return AuthorityResult{}, err
	}
	if request.Tests, err = prepareEvidence(project, request.Tests); err != nil {
		return AuthorityResult{}, err
	}
	readiness, err := prepareEvidence(project, []EvidenceReference{request.Readiness})
	if err != nil || len(readiness) != 1 {
		return AuthorityResult{}, core.ErrPhase
	}
	request.Readiness = readiness[0]
	if len(request.Reviews) == 0 || len(request.Tests) == 0 {
		return AuthorityResult{}, core.ErrPhase
	}
	for _, review := range request.Reviews {
		if err := validateReview(review, head); err != nil {
			return AuthorityResult{}, err
		}
	}
	for _, evidence := range append(append([]EvidenceReference(nil), request.Tests...), request.Readiness) {
		if err := verifyEvidenceReference(evidence); err != nil {
			return AuthorityResult{}, err
		}
	}
	if err := release.VerifyReadinessEvidence(request.Readiness.Path, head); err != nil {
		return AuthorityResult{}, err
	}
	issued, issueErr := time.Parse(time.RFC3339Nano, prep.IssuedAt)
	expires, expiryErr := time.Parse(time.RFC3339Nano, prep.ExpiresAt)
	if issueErr != nil || expiryErr != nil || !expires.After(issued) || expires.Sub(issued) > 24*time.Hour {
		return AuthorityResult{}, core.ErrPhase
	}
	trust, err := readOperatorTrustStore()
	if err != nil {
		return AuthorityResult{}, err
	}
	trustedKey := false
	for _, key := range trust.Keys {
		if key.KeyID == prep.SignerKeyID {
			trustedKey = true
			break
		}
	}
	if !trustedKey {
		return AuthorityResult{}, fmt.Errorf("%w: approval signer is not operator-trusted", core.ErrRevision)
	}
	remote, err := authorityRemoteObserver(ctx, project, prep.RemoteName, prep.BaseRef, prep.TargetRef)
	if err != nil {
		return AuthorityResult{}, fmt.Errorf("%w: configured remote differs or is unavailable", core.ErrRevision)
	}
	remote.ObservedAt = prep.IssuedAt
	layout, err := prepareLayout(prep)
	if err != nil {
		return AuthorityResult{}, err
	}
	hosts, err := install.InventoryLegacyHosts(layout, []install.Host{install.Claude, install.Codex})
	if err != nil {
		return AuthorityResult{}, err
	}
	approval := signedCutoverApproval{Schema: 1, ID: prep.ApprovalID, IssuedAt: prep.IssuedAt, ExpiresAt: prep.ExpiresAt, Project: project, OperationID: request.OperationID, TargetRevision: head, TrackerFingerprint: request.Tracker.Fingerprint, ParentID: request.Tracker.ParentID, Cause: prep.Cause, RecoveryDisposition: prep.RecoveryDisposition, SignerKeyID: prep.SignerKeyID, TaskIDs: request.Tracker.TaskIDs, Reviews: request.Reviews, Tests: request.Tests, Readiness: request.Readiness, Remote: remote, RemoteMainDeploys: prep.RemoteMainDeploys, HostInventories: hosts}
	payload, err := json.Marshal(approval)
	if err != nil {
		return AuthorityResult{}, err
	}
	payloadSHA := digestBytes(payload)
	cutover := request
	cutover.Action, cutover.Prepare = "cutover", nil
	cutover.Approval = EvidenceReference{ID: prep.ApprovalID, Path: prep.PayloadPath, SHA256: payloadSHA}
	cutover.ApprovalSignature = EvidenceReference{ID: prep.ApprovalID + "-signature", Path: prep.SignaturePath}
	cutover.ApprovalSignerKeyID = prep.SignerKeyID
	requestRaw, err := json.Marshal(cutover)
	if err != nil {
		return AuthorityResult{}, err
	}
	if err := writeExclusive(prep.PayloadPath, payload); err != nil {
		return AuthorityResult{}, err
	}
	if err := writeExclusive(prep.RequestPath, append(requestRaw, '\n')); err != nil {
		_ = os.Remove(prep.PayloadPath)
		return AuthorityResult{}, err
	}
	return AuthorityResult{Action: "prepare", ReceiptDigest: payloadSHA, TargetRevision: head, PayloadPath: prep.PayloadPath, PayloadSHA256: payloadSHA, RequestPath: prep.RequestPath, RequestSHA256: digestBytes(append(requestRaw, '\n'))}, nil
}

func gitHead(ctx context.Context, project string) (string, error) {
	output, err := execCommand(ctx, "git", "-C", project, "rev-parse", "HEAD")
	if err != nil || !validRevision(strings.TrimSpace(string(output))) {
		return "", core.ErrRevision
	}
	return strings.TrimSpace(string(output)), nil
}

var execCommand = func(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).Output()
}

func projectEvidencePath(project, path string) (string, error) {
	if !filepath.IsAbs(path) {
		path = filepath.Join(project, filepath.FromSlash(path))
	}
	path = filepath.Clean(path)
	if !filepath.IsAbs(path) {
		return "", core.ErrPath
	}
	return path, nil
}

func prepareEvidence(project string, refs []EvidenceReference) ([]EvidenceReference, error) {
	result := append([]EvidenceReference(nil), refs...)
	for i := range result {
		path, err := projectEvidencePath(project, result[i].Path)
		if err != nil {
			return nil, err
		}
		raw, err := readAbsoluteBounded(path)
		if err != nil {
			return nil, err
		}
		result[i].Path, result[i].SHA256 = path, digestBytes(raw)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].ID < result[j].ID || result[i].ID == result[j].ID && result[i].Path < result[j].Path
	})
	if validateEvidenceIdentities(result) != nil {
		return nil, core.ErrRevision
	}
	return result, nil
}

func closedTrackerIDs(raw []byte, parent string) ([]string, error) {
	var tasks []struct{ ID, Status, Parent, ParentID string }
	if parent == "" || json.Unmarshal(raw, &tasks) != nil {
		return nil, core.ErrRevision
	}
	ids := make([]string, 0, len(tasks))
	for _, task := range tasks {
		if strings.ToLower(task.Status) != "closed" || task.ID == "" || task.ID != parent && task.Parent != parent && task.ParentID != parent && !strings.HasPrefix(task.ID, parent+".") {
			return nil, core.ErrRevision
		}
		ids = append(ids, task.ID)
	}
	sort.Strings(ids)
	if !canonicalTaskIDs(ids) {
		return nil, core.ErrRevision
	}
	return ids, nil
}

func prepareLayout(prep *AuthorityPrepare) (install.Layout, error) {
	manifest := filepath.Clean(prep.NativeManifest)
	data := filepath.Dir(manifest)
	roots := map[install.Host]string{}
	configs := map[install.Host]string{}
	for _, host := range []install.Host{install.Codex, install.Claude} {
		root := filepath.Clean(prep.HostRoots[host])
		if !filepath.IsAbs(root) || root != prep.HostRoots[host] {
			return install.Layout{}, core.ErrPath
		}
		home := filepath.Dir(filepath.Dir(root))
		roots[host] = root
		if host == install.Codex {
			configs[host] = filepath.Join(home, "hooks.json")
		} else {
			configs[host] = filepath.Join(home, "settings.json")
		}
	}
	return install.Layout{DataRoot: data, ManifestPath: manifest, BinaryPath: filepath.Join(data, "bin", "agent-teamctl"), ContractPath: filepath.Join(data, "WORKER-CONTRACT"), SkillRoots: roots, ConfigPaths: configs}, nil
}

func absoluteOutput(path string) bool { return filepath.IsAbs(path) && filepath.Clean(path) == path }

func writeExclusive(path string, raw []byte) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	if _, err = file.Write(raw); err == nil {
		err = file.Sync()
	}
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		_ = os.Remove(path)
	}
	return err
}
