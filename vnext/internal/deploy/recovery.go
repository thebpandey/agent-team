package deploy

import (
	"context"
	"strings"

	"github.com/thebpandey/agent-team/vnext/internal/core"
	"github.com/thebpandey/agent-team/vnext/internal/knowledge"
)

func (service RecoveryService) RecordState(ctx context.Context, receipt DeploymentReceipt) (string, error) {
	if ctx == nil || ctx.Err() != nil {
		return "", core.ErrSettings
	}
	if receipt.State != "failed" {
		return "", nil
	}
	if service.Blockers == nil || receipt.RunID == "" || receipt.BatchID == "" {
		return "", core.ErrSettings
	}
	blocker, err := service.Blockers.Create(ctx, knowledge.Blocker{
		RecordEnvelope: core.RecordEnvelope{Schema: 1, Project: receipt.Project, RunID: receipt.RunID, WrittenAt: receipt.WrittenAt},
		ID:             "deployment-" + receipt.BatchID, Severity: "high", State: "open", Affected: []string{receipt.BatchID}, EvidencePointer: firstPointer(receipt.EvidencePointers), Reason: "deployment verification failed",
	})
	if err != nil {
		return "", err
	}
	if service.Holds != nil {
		if err := service.Holds.HoldLater(ctx, receipt.RunID, receipt.BatchID); err != nil {
			return "", err
		}
	}
	if service.Dashboard != nil {
		_ = service.Dashboard.Trigger(ctx, DashboardTriggerRequest{RunID: receipt.RunID, BatchID: receipt.BatchID, ReceiptPointer: firstPointer(receipt.EvidencePointers), Reason: "failed"})
	}
	return blocker.ID, nil
}

func ValidateRollbackAuthorization(confirmed bool, authorizationRef string) error {
	if !confirmed || strings.TrimSpace(authorizationRef) == "" {
		return core.ErrSettings
	}
	return nil
}

func firstPointer(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return values[0]
}
