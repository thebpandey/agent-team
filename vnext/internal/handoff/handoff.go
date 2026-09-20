// Package handoff derives bounded recovery text from canonical run facts.
package handoff

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"

	"github.com/thebpandey/agent-team/vnext/internal/core"
	"github.com/thebpandey/agent-team/vnext/internal/knowledge"
	"github.com/thebpandey/agent-team/vnext/internal/project"
	"github.com/thebpandey/agent-team/vnext/internal/run"
	"github.com/thebpandey/agent-team/vnext/internal/store"
)

// Handoff creates a recovery aid; it never grants authority to resume work.
type Handoff interface {
	Derive(context.Context, core.RunID) ([]byte, error)
}

type service struct{ store *store.Store }

func New(state *store.Store) Handoff        { return &service{store: state} }
func NewHandoff(state *store.Store) Handoff { return New(state) }

func (s *service) Derive(ctx context.Context, id core.RunID) ([]byte, error) {
	if ctx == nil || ctx.Err() != nil || s == nil || s.store == nil {
		return nil, core.ErrRevision
	}
	if err := project.ValidateSegment(string(id)); err != nil {
		return nil, core.ErrTransition
	}
	manifest, err := run.NewRepositories(s.store).Runs.Read(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("%w: canonical run: %v", core.ErrRevision, err)
	}
	snapshot := knowledge.HandoffSnapshot{RecordEnvelope: manifest.RecordEnvelope, NextAction: "inspect", Freshness: "canonical run revision"}
	receipts := make([]knowledge.Receipt, 0, len(manifest.Teams))
	for _, team := range manifest.Teams {
		if err := project.ValidateSegment(string(team.ID)); err != nil {
			return nil, core.ErrRevision
		}
		var receipt knowledge.Receipt
		err := s.store.ReadJSON(".agent-team/receipts/"+string(team.ID)+".json", 1<<20, &receipt)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil || !validReceipt(manifest, team, receipt) {
			return nil, core.ErrRevision
		}
		if snapshot.Team == "" {
			snapshot.Team = receipt.Team
		}
		receipts = append(receipts, receipt)
		snapshot.Resources.External = append(snapshot.Resources.External, team.Resources...)
		if receipt.Gate != "" {
			snapshot.Gates = append(snapshot.Gates, receipt.Gate)
		}
		if receipt.Review != "" {
			snapshot.Reviews = append(snapshot.Reviews, receipt.Review)
		}
		for _, pointer := range receipt.EvidencePointers {
			snapshot.Reviews = append(snapshot.Reviews, pointer)
		}
	}
	snapshot.Gates = unique(snapshot.Gates)
	snapshot.Reviews = unique(snapshot.Reviews)
	snapshot.Resources.External = unique(snapshot.Resources.External)
	blockers, err := knowledge.NewProjectionWriter(s.store).RegenerateBlockers(ctx, nil, receipts, snapshot.Resources)
	if err != nil {
		return nil, err
	}
	for _, blocker := range blockers {
		if blocker.RunID == manifest.ID {
			snapshot.Blockers = append(snapshot.Blockers, blocker)
		}
	}
	if resumable(manifest, receipts, snapshot.Blockers) {
		snapshot.NextAction = "resume"
	}
	return knowledge.NewProjectionWriter(s.store).WriteHandoff(ctx, snapshot)
}

func resumable(manifest run.Run, receipts []knowledge.Receipt, blockers []knowledge.Blocker) bool {
	if manifest.State == core.Cancelled || manifest.State == core.Archived || len(receipts) == 0 || len(blockers) != 0 {
		return false
	}
	paused := false
	for _, receipt := range receipts {
		if receipt.NextAction != "resume" {
			return false
		}
		switch receipt.State {
		case core.Paused, core.Interrupted:
			paused = true
		case core.Cancelled, core.Clean, core.Integrated, core.Archived:
			return false
		}
	}
	return paused
}

func validReceipt(manifest run.Run, team run.TeamRecord, receipt knowledge.Receipt) bool {
	if receipt.Schema != 1 || receipt.Project != manifest.Project || receipt.RunID != manifest.ID || receipt.Team != string(team.ID) || receipt.Task == "" || receipt.Attempt < 1 || receipt.State == "" || receipt.NextAction == "" || receipt.Revision == 0 || len(receipt.EvidencePointers) > 128 {
		return false
	}
	if err := project.ValidateSegment(receipt.Task); err != nil {
		return false
	}
	member := false
	for _, task := range team.Queue {
		if string(task) == receipt.Task {
			member = true
			break
		}
	}
	if !member {
		return false
	}
	for _, value := range append(append([]string{}, receipt.EvidencePointers...), receipt.Gate, receipt.Review, receipt.NextAction) {
		if len(value) > 4096 || strings.Contains(value, "\x00") {
			return false
		}
	}
	for _, pointer := range receipt.EvidencePointers {
		if filepath.IsAbs(pointer) || strings.Contains(pointer, "\\") || pointer == "" {
			return false
		}
		for _, segment := range strings.Split(pointer, "/") {
			if project.ValidateSegment(segment) != nil {
				return false
			}
		}
	}
	return true
}

func unique(values []string) []string {
	sort.Strings(values)
	if len(values) == 0 {
		return nil
	}
	out := values[:1]
	for _, value := range values[1:] {
		if value != out[len(out)-1] {
			out = append(out, value)
		}
	}
	return out
}
