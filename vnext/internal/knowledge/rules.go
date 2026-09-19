package knowledge

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/thebpandey/agent-team/vnext/internal/core"
	"github.com/thebpandey/agent-team/vnext/internal/store"
)

// Rule is a project-promoted constraint. Every promotion identifies the DEC
// record that authorized it, and optional supersession remains explicit.
type Rule struct {
	core.RecordEnvelope
	ID         string `json:"id"`
	Text       string `json:"text"`
	DecisionID string `json:"decisionId"`
	Supersedes string `json:"supersedes,omitempty"`
}

func validateRule(rule Rule) error {
	if err := validateEnvelope(rule.RecordEnvelope); err != nil {
		return err
	}
	if err := validateSegment(rule.ID); err != nil {
		return err
	}
	if rule.Text == "" || !decisionIDPattern.MatchString(rule.DecisionID) {
		return fmt.Errorf("%w: rule text and DEC provenance are required", core.ErrRevision)
	}
	if rule.Supersedes != "" && !decisionIDPattern.MatchString(rule.Supersedes) {
		return fmt.Errorf("%w: invalid superseding DEC", core.ErrPath)
	}
	return validateText(rule.Text, "rule text")
}

func ruleMarkdown(rule Rule) string {
	lines := []string{"## " + markdownEscape(rule.ID), "", "- Decision: `" + markdownEscape(rule.DecisionID) + "`", "- Run: `" + markdownEscape(string(rule.RunID)) + "`", "- Revision: " + fmt.Sprintf("%d", rule.Revision), "- Rule: " + markdownEscape(rule.Text)}
	if rule.Supersedes != "" {
		lines = append(lines, "- Supersedes decision: `"+markdownEscape(rule.Supersedes)+"`")
	}
	return strings.Join(lines, "\n") + "\n"
}

// PromoteRule records an explicitly decision-authorized rule in the canonical
// project rules document. Existing text is never silently rewritten.
func PromoteRule(ctx context.Context, s *store.Store, rule Rule) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if s == nil {
		return fmt.Errorf("%w: nil store", core.ErrPath)
	}
	if err := validateRule(rule); err != nil {
		return err
	}
	lock := knowledgeLock(s.Root)
	lock.Lock()
	defer lock.Unlock()
	path := "AGENT_TEAM_RULES.md"
	contents, err := readCanonicalMarkdown(s, path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if int64(len(contents)) > canonicalLimit(s) {
		return fmt.Errorf("%w: rules exceed canonical limit", core.ErrLimit)
	}
	decisionContents, err := readCanonicalMarkdown(s, "DECISIONS.md")
	if err != nil {
		return fmt.Errorf("%w: decision provenance %s is unavailable: %v", core.ErrRevision, rule.DecisionID, err)
	}
	if !validDecisionMarkdown(string(decisionContents), rule.DecisionID) {
		return fmt.Errorf("%w: decision provenance %s is absent", core.ErrRevision, rule.DecisionID)
	}
	marker := "## " + rule.ID + "\n"
	if strings.Contains(string(contents), marker) {
		return fmt.Errorf("%w: rule ID already exists", ErrConflict)
	}
	updated := string(contents)
	if updated == "" {
		updated = "# Agent-Team Rules\n\n"
	}
	if !strings.HasSuffix(updated, "\n") {
		updated += "\n"
	}
	updated += "\n" + ruleMarkdown(rule)
	_, err = s.WriteMarkdown(path, []byte(updated), canonicalLimit(s))
	return err
}
