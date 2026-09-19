package knowledge

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/thebpandey/agent-team/vnext/internal/core"
	"github.com/thebpandey/agent-team/vnext/internal/store"
)

// HandoffSnapshot contains recovery facts for a particular run revision. It is
// informational only: a future host must reconcile canonical facts before it
// acts on any item in this snapshot.
type HandoffSnapshot struct {
	core.RecordEnvelope
	Team       string                `json:"team,omitempty"`
	Gates      []string              `json:"gates,omitempty"`
	Reviews    []string              `json:"reviews,omitempty"`
	Resources  core.ResourceSnapshot `json:"resources"`
	Decisions  []Decision            `json:"decisions,omitempty"`
	Blockers   []Blocker             `json:"blockers,omitempty"`
	NextAction string                `json:"nextAction"`
	Freshness  string                `json:"freshness"`
}

// HandoffLimitError means no handoff file was written because the hard bound
// would have been exceeded.
type HandoffLimitError struct{ Bytes, Limit int64 }

func (e *HandoffLimitError) Error() string {
	return fmt.Sprintf("%v: handoff %d bytes exceeds %d", core.ErrLimit, e.Bytes, e.Limit)
}
func (e *HandoffLimitError) Unwrap() error { return core.ErrLimit }

func validateHandoff(snapshot HandoffSnapshot) error {
	if err := validateEnvelope(snapshot.RecordEnvelope); err != nil {
		return err
	}
	if snapshot.NextAction == "" || snapshot.Freshness == "" {
		return fmt.Errorf("%w: handoff next action and freshness are required", core.ErrRevision)
	}
	if err := validateText(snapshot.NextAction, "handoff next action"); err != nil {
		return err
	}
	if err := validateText(snapshot.Freshness, "handoff freshness"); err != nil {
		return err
	}
	if snapshot.Team != "" {
		if err := validateSegment(snapshot.Team); err != nil {
			return err
		}
		if err := validateText(snapshot.Team, "handoff team"); err != nil {
			return err
		}
	}
	for name, values := range map[string][]string{"gates": snapshot.Gates, "reviews": snapshot.Reviews, "servers": snapshot.Resources.Servers, "browsers": snapshot.Resources.Browsers, "external": snapshot.Resources.External} {
		if len(values) > 128 {
			return fmt.Errorf("%w: too many handoff %s", core.ErrLimit, name)
		}
		for _, value := range values {
			if err := validateText(value, name); err != nil {
				return err
			}
		}
	}
	if len(snapshot.Decisions) > 128 || len(snapshot.Blockers) > 128 {
		return fmt.Errorf("%w: too many handoff decisions or blockers", core.ErrLimit)
	}
	decisionIDs := make(map[string]struct{}, len(snapshot.Decisions))
	for _, decision := range snapshot.Decisions {
		if decision.ID == "" || !decisionIDPattern.MatchString(decision.ID) {
			return fmt.Errorf("%w: handoff decision ID", core.ErrRevision)
		}
		if _, exists := decisionIDs[decision.ID]; exists {
			return fmt.Errorf("%w: duplicate handoff decision", core.ErrRevision)
		}
		decisionIDs[decision.ID] = struct{}{}
		for name, value := range map[string]string{"decision summary": decision.Summary, "decision rationale": decision.Rationale, "decision provenance": decision.Provenance} {
			if err := validateText(value, name); err != nil {
				return err
			}
		}
	}
	blockerIDs := make(map[string]struct{}, len(snapshot.Blockers))
	for _, blocker := range snapshot.Blockers {
		if blocker.ID == "" || blocker.State == "" {
			return fmt.Errorf("%w: handoff blocker", core.ErrRevision)
		}
		if err := validateSegment(blocker.ID); err != nil {
			return err
		}
		if blocker.State != blockerOpen && blocker.State != blockerResolved {
			return fmt.Errorf("%w: handoff blocker state", core.ErrRevision)
		}
		if _, exists := blockerIDs[blocker.ID]; exists {
			return fmt.Errorf("%w: duplicate handoff blocker", core.ErrRevision)
		}
		blockerIDs[blocker.ID] = struct{}{}
		for name, value := range map[string]string{"blocker severity": blocker.Severity, "blocker reason": blocker.Reason, "blocker requested choice": blocker.RequestedChoice, "blocker safe default": blocker.SafeDefault} {
			if err := validateText(value, name); err != nil {
				return err
			}
		}
	}
	return nil
}

func bulletLines(values []string) []string {
	values = sortedStrings(values)
	if len(values) == 0 {
		return []string{"- none"}
	}
	lines := make([]string, 0, len(values))
	for _, value := range values {
		lines = append(lines, "- "+markdownEscape(value))
	}
	return lines
}

func appendSection(lines []string, title string, values []string) []string {
	lines = append(lines, "", "## "+title)
	return append(lines, bulletLines(values)...)
}

// DeriveHandoff produces deterministic, escaped Markdown from only the passed
// snapshot. The rendered text explicitly has no authority to resume work.
func DeriveHandoff(snapshot HandoffSnapshot) ([]byte, error) {
	if err := validateHandoff(snapshot); err != nil {
		return nil, err
	}
	lines := []string{
		"# Agent-Team Handoff",
		"",
		"> This handoff is a recovery aid, not authority. Reconcile canonical facts before taking action.",
		"",
		"- Run: `" + markdownEscape(string(snapshot.RunID)) + "`",
		"- Run revision: " + strconv.FormatUint(snapshot.Revision, 10),
		"- Project: " + markdownEscape(snapshot.Project),
		"- Written: `" + markdownEscape(snapshot.WrittenAt) + "`",
		"- Team: `" + markdownEscape(snapshot.Team) + "`",
		"- Freshness: " + markdownEscape(snapshot.Freshness),
		"- Next action: " + markdownEscape(snapshot.NextAction),
	}
	lines = appendSection(lines, "Gates", snapshot.Gates)
	lines = appendSection(lines, "Reviews", snapshot.Reviews)
	lines = appendSection(lines, "Resources: Servers", snapshot.Resources.Servers)
	lines = appendSection(lines, "Resources: Browsers", snapshot.Resources.Browsers)
	lines = appendSection(lines, "Resources: External", snapshot.Resources.External)

	lines = append(lines, "", "## Decisions")
	decisions := append([]Decision(nil), snapshot.Decisions...)
	sort.Slice(decisions, func(i, j int) bool { return decisions[i].ID < decisions[j].ID })
	if len(decisions) == 0 {
		lines = append(lines, "- none")
	}
	for _, decision := range decisions {
		lines = append(lines, "- `"+markdownEscape(decision.ID)+"`: "+markdownEscape(decision.Summary))
	}
	lines = append(lines, "", "## Open blockers")
	blockers := make([]Blocker, 0, len(snapshot.Blockers))
	for _, blocker := range snapshot.Blockers {
		if blocker.State != blockerResolved {
			blockers = append(blockers, blocker)
		}
	}
	sort.Slice(blockers, func(i, j int) bool { return blockers[i].ID < blockers[j].ID })
	if len(blockers) == 0 {
		lines = append(lines, "- none")
	}
	for _, blocker := range blockers {
		lines = append(lines, "- `"+markdownEscape(blocker.ID)+"` ("+markdownEscape(blocker.Severity)+"): "+markdownEscape(blocker.Reason))
	}
	output := []byte(strings.Join(lines, "\n") + "\n")
	if int64(len(output)) > 256<<10 {
		return nil, &HandoffLimitError{Bytes: int64(len(output)), Limit: 256 << 10}
	}
	return output, nil
}

// ProjectionWriter regenerates derived Markdown only from supplied snapshots
// and stored canonical facts. It never removes canonical evidence or blockers.
type ProjectionWriter interface {
	WriteHandoff(context.Context, HandoffSnapshot) ([]byte, error)
	RegenerateBlockers(context.Context, []Decision, []Receipt, core.ResourceSnapshot) ([]Blocker, error)
}

type projectionWriter struct{ store *store.Store }

// NewProjectionWriter creates a writer for deterministic, bounded projections.
func NewProjectionWriter(s *store.Store) ProjectionWriter { return &projectionWriter{store: s} }

func handoffPath(run core.RunID) (string, error) {
	if err := validateSegment(string(run)); err != nil {
		return "", err
	}
	return ".agent-team/handoffs/" + string(run) + ".md", nil
}

func handoffHardLimit(s *store.Store) int64 {
	if s != nil && s.Limits.HandoffHardBytes > 0 && s.Limits.HandoffHardBytes < 256<<10 {
		return s.Limits.HandoffHardBytes
	}
	return 256 << 10
}

func handoffWarnLimit(s *store.Store) int64 {
	if s != nil && s.Limits.HandoffWarnBytes > 0 && s.Limits.HandoffWarnBytes < handoffHardLimit(s) {
		return s.Limits.HandoffWarnBytes
	}
	return 192 << 10
}

func (p *projectionWriter) WriteHandoff(ctx context.Context, snapshot HandoffSnapshot) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if p == nil || p.store == nil {
		return nil, fmt.Errorf("%w: nil store", core.ErrPath)
	}
	data, err := DeriveHandoff(snapshot)
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > handoffHardLimit(p.store) {
		return nil, &HandoffLimitError{Bytes: int64(len(data)), Limit: handoffHardLimit(p.store)}
	}
	if int64(len(data)) >= handoffWarnLimit(p.store) {
		warning := []byte("> Warning: this handoff has reached the configured warning threshold.\n\n")
		data = append(data[:len("# Agent-Team Handoff\n\n")], append(warning, data[len("# Agent-Team Handoff\n\n"):]...)...)
		if int64(len(data)) > handoffHardLimit(p.store) {
			return nil, &HandoffLimitError{Bytes: int64(len(data)), Limit: handoffHardLimit(p.store)}
		}
	}
	path, err := handoffPath(snapshot.RunID)
	if err != nil {
		return nil, err
	}
	lock := knowledgeLock(p.store.Root)
	lock.Lock()
	defer lock.Unlock()
	if _, err := p.store.WriteMarkdown(path, data, handoffHardLimit(p.store)); err != nil {
		return nil, err
	}
	return data, nil
}

func blockersMarkdown(items []Blocker) []byte {
	lines := []string{"# Open Blockers", "", "> Derived from canonical blocker facts; resolution history remains in blocker records.", ""}
	if len(items) == 0 {
		lines = append(lines, "- none")
	}
	for _, item := range items {
		lines = append(lines, "## `"+markdownEscape(item.ID)+"`", "", "- Severity: "+markdownEscape(item.Severity), "- Run: `"+markdownEscape(string(item.RunID))+"`")
		if len(item.Affected) != 0 {
			lines = append(lines, "- Affected: "+strings.Join(bulletInline(item.Affected), ", "))
		}
		if item.EvidencePointer != "" {
			lines = append(lines, "- Evidence: `"+markdownEscape(item.EvidencePointer)+"`")
		}
		if item.Reason != "" {
			lines = append(lines, "- Reason: "+markdownEscape(item.Reason))
		}
		lines = append(lines, "")
	}
	return []byte(strings.Join(lines, "\n") + "\n")
}

func bulletInline(values []string) []string {
	values = sortedStrings(values)
	for i := range values {
		values[i] = "`" + markdownEscape(values[i]) + "`"
	}
	return values
}

func (p *projectionWriter) RegenerateBlockers(ctx context.Context, decisions []Decision, receipts []Receipt, resources core.ResourceSnapshot) ([]Blocker, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if p == nil || p.store == nil {
		return nil, fmt.Errorf("%w: nil store", core.ErrPath)
	}
	// The interface keeps these facts available to callers, but a canonical
	// root projection is rendered only from persisted blocker records.
	_ = decisions
	_ = receipts
	_ = resources
	lock := knowledgeLock(p.store.Root)
	lock.Lock()
	defer lock.Unlock()
	all, err := readCanonicalBlockers(p.store)
	if err != nil {
		return nil, err
	}
	open := make([]Blocker, 0, len(all))
	for _, item := range all {
		if item.State == blockerOpen {
			open = append(open, item)
		}
	}
	data := blockersMarkdown(open)
	if int64(len(data)) > canonicalLimit(p.store) {
		return nil, fmt.Errorf("%w: blocker projection", core.ErrLimit)
	}
	if _, err := p.store.WriteMarkdown("BLOCKERS.md", data, canonicalLimit(p.store)); err != nil {
		return nil, err
	}
	return open, nil
}
