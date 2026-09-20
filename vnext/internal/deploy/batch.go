package deploy

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/thebpandey/agent-team/vnext/internal/contracts"
	"github.com/thebpandey/agent-team/vnext/internal/core"
)

func ResolveBatchSize(profile TargetProfile, requested int) (int, error) {
	size := requested
	if size == 0 {
		size = profile.DefaultBatchSize
	}
	if size < 1 || size > 100 {
		return 0, core.ErrBatch
	}
	return size, nil
}

func PlanBatches(ctx context.Context, request BatchRequest) ([]BatchManifest, error) {
	if ctx == nil || ctx.Err() != nil {
		return nil, core.ErrBatch
	}
	if err := ValidateProfile(request.Profile); err != nil {
		return nil, err
	}
	size, err := ResolveBatchSize(request.Profile, request.BatchSize)
	if err != nil {
		return nil, err
	}
	if request.RunID == "" || request.Target == "" || request.Target != request.Profile.Target || len(request.Tasks) == 0 {
		return nil, core.ErrBatch
	}
	seen := make(map[core.TaskID]bool, len(request.Tasks))
	for _, task := range request.Tasks {
		if task.ID == "" || seen[task.ID] || task.CanonicalRevision == "" || !task.Integrated || !task.GatePassed || !task.DependenciesComplete {
			return nil, core.ErrBatch
		}
		seen[task.ID] = true
	}
	out := make([]BatchManifest, 0, (len(request.Tasks)+size-1)/size)
	for start, sequence := 0, 0; start < len(request.Tasks); start, sequence = start+size, sequence+1 {
		end := start + size
		if end > len(request.Tasks) {
			end = len(request.Tasks)
		}
		manifest := BatchManifest{RecordEnvelope: core.RecordEnvelope{Schema: 1, RunID: request.RunID}, BatchID: fmt.Sprintf("%s-batch-%d", request.RunID, sequence+1), RunID: request.RunID, Sequence: sequence, Target: request.Target, ProfileDigest: request.Profile.ProfileDigest, Final: end == len(request.Tasks)}
		for _, task := range request.Tasks[start:end] {
			manifest.TaskIDs = append(manifest.TaskIDs, task.ID)
			manifest.Revisions = append(manifest.Revisions, task.CanonicalRevision)
			manifest.ArtifactDigests = append(manifest.ArtifactDigests, append([]string(nil), task.ArtifactDigests...))
		}
		manifest.Fingerprint, err = DeriveBatchFingerprint(manifest)
		if err != nil {
			return nil, err
		}
		manifest.IdempotencyKey = DeriveBatchIdempotencyKey(manifest.RunID, manifest.Sequence, manifest.Fingerprint)
		out = append(out, manifest)
	}
	return out, nil
}

func DeriveBatchFingerprint(batch BatchManifest) (string, error) {
	wire := struct {
		Run             core.RunID
		Target, Profile string
		Tasks           []core.TaskID
		Revisions       []string
		Artifacts       [][]string
		Sequence        int
	}{batch.RunID, batch.Target, batch.ProfileDigest, batch.TaskIDs, batch.Revisions, batch.ArtifactDigests, batch.Sequence}
	raw, err := json.Marshal(wire)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

func DeriveBatchIdempotencyKey(run core.RunID, sequence int, fingerprint string) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s/%d/%s", run, sequence, fingerprint)))
	return "deploy-v1:" + hex.EncodeToString(sum[:])
}

func (batch BatchManifest) ContractBatch(profile TargetProfile) (contracts.DeploymentBatch, error) {
	if err := ValidateProfile(profile); err != nil {
		return contracts.DeploymentBatch{}, err
	}
	fingerprint, err := DeriveBatchFingerprint(batch)
	if err != nil || batch.RunID == "" || batch.Target != profile.Target || batch.ProfileDigest != profile.ProfileDigest || fingerprint != batch.Fingerprint || batch.IdempotencyKey != DeriveBatchIdempotencyKey(batch.RunID, batch.Sequence, batch.Fingerprint) || len(batch.TaskIDs) == 0 || len(batch.TaskIDs) != len(batch.Revisions) || len(batch.TaskIDs) != len(batch.ArtifactDigests) {
		return contracts.DeploymentBatch{}, core.ErrBatch
	}
	return contracts.DeploymentBatch{Run: batch.RunID, TaskIDs: append([]core.TaskID(nil), batch.TaskIDs...), Revisions: append([]string(nil), batch.Revisions...), TargetProfile: string(profile.ID), AuthorizationRef: profile.AuthorizationRef, ExecutorCommand: append([]string(nil), profile.ExecutorCommand...), VerificationCommand: append([]string(nil), profile.VerificationCommand...), Fingerprint: batch.Fingerprint, IdempotencyKey: batch.IdempotencyKey}, nil
}
