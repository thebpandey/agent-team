// Package dashboard publishes a bounded, local-only status snapshot.
package dashboard

import (
	"context"
	"fmt"
	"unicode/utf8"

	"github.com/thebpandey/agent-team/vnext/internal/core"
	"github.com/thebpandey/agent-team/vnext/internal/store"
)

const (
	dashboardPath    = ".agent-team/dashboard/index.html"
	maxTextBytes     = 4 << 10
	maxSnapshotBytes = 1 << 20
	maxRows          = 256
	maxTeamTasks     = 128
)

type TaskSummary struct{ ID, State, Next string }
type TeamSummary struct {
	ID, State, QueueFingerprint string
	Tasks                       []string
}

type DashboardStatus string

const (
	Current DashboardStatus = "current"
	Stale DashboardStatus = "stale"
	Unavailable DashboardStatus = "unavailable"
)

// Snapshot is the complete, bounded dashboard input for one integration.
type Snapshot struct {
	Schema int
	Project, RunID, CanonicalRevision, LastIntegration string
	Status                                           DashboardStatus
	Tasks                                            []TaskSummary
	Teams                                            []TeamSummary
	Resources                                        core.ResourceSnapshot
	Evidence                                         []string
	GeneratedAt                                      string
}

type IntegrationResult struct {
	Success                                      bool
	RunID, TaskID, CanonicalRevision, Error string
	EvidencePointers                             []string
	Revision                                     uint64
}

type DashboardRefreshReceipt struct {
	core.RecordEnvelope
	RunID, TaskID, CanonicalRevision string
	Status           DashboardStatus
	Success          bool
	Error            string
	EvidencePointers []string
}

// Renderer has one operation so a caller cannot accidentally render twice.
type Renderer interface {
	Publish(context.Context, Snapshot) error
}
type ReceiptWriter interface {
	WriteRefreshReceipt(context.Context, DashboardRefreshReceipt) error
}
type IntegrationObserver interface {
	AfterIntegration(context.Context, IntegrationResult, Snapshot) error
}

type renderer struct{ store *store.Store }

func NewRenderer(s *store.Store) Renderer { return &renderer{store: s} }

func (r *renderer) Publish(ctx context.Context, snapshot Snapshot) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if r.store == nil {
		return fmt.Errorf("dashboard store is required")
	}
	if err := validate(snapshot); err != nil {
		return err
	}
	html, err := render(snapshot)
	if err != nil { return err }
	_, err = r.store.WriteMarkdown(dashboardPath, html, maxSnapshotBytes)
	return err
}

type observer struct {
	renderer Renderer
	receipts ReceiptWriter
}

func NewIntegrationObserver(renderer Renderer, receipts ReceiptWriter) IntegrationObserver {
	return &observer{renderer: renderer, receipts: receipts}
}

// AfterIntegration deliberately absorbs dashboard failures: integration remains authoritative.
func (o *observer) AfterIntegration(ctx context.Context, result IntegrationResult, snapshot Snapshot) error {
	receipt := DashboardRefreshReceipt{
		RecordEnvelope: core.RecordEnvelope{Schema: snapshot.Schema, Project: snapshot.Project, RunID: core.RunID(result.RunID), WrittenAt: snapshot.GeneratedAt, Revision: result.Revision},
		RunID:          result.RunID,
		TaskID:         result.TaskID,
		CanonicalRevision: result.CanonicalRevision,
		EvidencePointers: append([]string(nil), result.EvidencePointers...),
	}
	if !result.Success {
		receipt.Status, receipt.Error = Stale, result.Error
		return o.write(ctx, receipt)
	}
	if o.renderer == nil {
		receipt.Status, receipt.Error = Unavailable, "dashboard renderer is unavailable"
		return o.write(ctx, receipt)
	}
	if err := o.renderer.Publish(ctx, snapshot); err != nil {
		receipt.Status, receipt.Error = Unavailable, err.Error()
		return o.write(ctx, receipt)
	}
	receipt.Status, receipt.Success = Current, true
	return o.write(ctx, receipt)
}

func (o *observer) write(ctx context.Context, receipt DashboardRefreshReceipt) error {
	if o.receipts != nil {
		_ = o.receipts.WriteRefreshReceipt(ctx, receipt)
	}
	return nil
}

func validate(s Snapshot) error {
	if s.Schema < 1 || (s.Status != "" && s.Status != Current && s.Status != Stale && s.Status != Unavailable) {
		return fmt.Errorf("invalid dashboard snapshot")
	}
	if len(s.Tasks) > maxRows || len(s.Teams) > maxRows || len(s.Evidence) > maxRows || len(s.Resources.Servers) > maxRows || len(s.Resources.Browsers) > maxRows || len(s.Resources.External) > maxRows {
		return fmt.Errorf("dashboard row limit exceeded")
	}
	remaining := maxSnapshotBytes
	check := func(value string) error {
		if !utf8.ValidString(value) || len(value) > maxTextBytes {
			return fmt.Errorf("dashboard text limit exceeded")
		}
		remaining -= len(value)
		if remaining < 0 {
			return fmt.Errorf("dashboard content limit exceeded")
		}
		return nil
	}
	for _, value := range []string{s.Project, s.RunID, s.CanonicalRevision, s.LastIntegration, s.GeneratedAt} {
		if err := check(value); err != nil {
			return err
		}
	}
	for _, task := range s.Tasks {
		for _, value := range []string{task.ID, task.State, task.Next} {
			if err := check(value); err != nil {
				return err
			}
		}
	}
	for _, team := range s.Teams {
		if len(team.Tasks) > maxTeamTasks {
			return fmt.Errorf("dashboard team task limit exceeded")
		}
		for _, value := range []string{team.ID, team.State, team.QueueFingerprint} {
			if err := check(value); err != nil {
				return err
			}
		}
		for _, value := range team.Tasks {
			if err := check(value); err != nil {
				return err
			}
		}
	}
	for _, values := range [][]string{s.Resources.Servers, s.Resources.Browsers, s.Resources.External, s.Evidence} {
		for _, value := range values {
			if err := check(value); err != nil {
				return err
			}
		}
	}
	return nil
}
