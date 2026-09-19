package knowledge

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/thebpandey/agent-team/vnext/internal/core"
	"github.com/thebpandey/agent-team/vnext/internal/store"
)

// Decision is an append-only, provenance-bound project decision.
type Decision struct {
	core.RecordEnvelope
	ID                     string   `json:"id,omitempty"`
	Scope                  string   `json:"scope,omitempty"`
	Summary                string   `json:"summary"`
	Rationale              string   `json:"rationale"`
	AlternativesConsidered []string `json:"alternativesConsidered,omitempty"`
	Provenance             string   `json:"provenance,omitempty"`
	Supersedes             string   `json:"supersedes,omitempty"`
	EvidenceRef            string   `json:"evidenceRef,omitempty"`
}

var decisionIDPattern = regexp.MustCompile(`^DEC-([0-9]{6,})$`)
var decisionHeaderPattern = regexp.MustCompile(`(?m)^## DEC-([0-9]{6,})$`)

func validateDecision(d Decision) error {
	if err := validateEnvelope(d.RecordEnvelope); err != nil {
		return err
	}
	if d.ID != "" && !decisionIDPattern.MatchString(d.ID) {
		return fmt.Errorf("%w: invalid decision ID", core.ErrPath)
	}
	if d.Summary == "" || d.Rationale == "" {
		return fmt.Errorf("%w: decision summary and rationale are required", core.ErrRevision)
	}
	for name, value := range map[string]string{"scope": d.Scope, "summary": d.Summary, "rationale": d.Rationale, "provenance": d.Provenance, "supersedes": d.Supersedes, "evidenceRef": d.EvidenceRef} {
		if err := validateText(value, name); err != nil {
			return err
		}
	}
	if d.Supersedes != "" && !decisionIDPattern.MatchString(d.Supersedes) {
		return fmt.Errorf("%w: invalid superseded decision", core.ErrPath)
	}
	if d.EvidenceRef != "" {
		if err := validatePointer(d.EvidenceRef); err != nil {
			return err
		}
	}
	if len(d.AlternativesConsidered) > 64 {
		return fmt.Errorf("%w: too many alternatives", core.ErrLimit)
	}
	for _, alternative := range d.AlternativesConsidered {
		if err := validateText(alternative, "alternative"); err != nil {
			return err
		}
	}
	return nil
}

func markdownEscape(value string) string {
	value = strings.ReplaceAll(value, "\\", "\\\\")
	value = strings.ReplaceAll(value, "`", "\\`")
	value = strings.ReplaceAll(value, "*", "\\*")
	value = strings.ReplaceAll(value, "_", "\\_")
	value = strings.ReplaceAll(value, "[", "\\[")
	value = strings.ReplaceAll(value, "]", "\\]")
	value = strings.ReplaceAll(value, "<", "\\<")
	value = strings.ReplaceAll(value, ">", "\\>")
	value = strings.ReplaceAll(value, "|", "\\|")
	value = strings.ReplaceAll(value, "\r\n", "\n")
	value = strings.ReplaceAll(value, "\r", "\n")
	return strings.ReplaceAll(value, "\n", "<br>")
}

func markdownUnescape(value string) (string, bool) {
	value = strings.ReplaceAll(value, "<br>", "\n")
	var output strings.Builder
	for index := 0; index < len(value); index++ {
		if value[index] != '\\' {
			output.WriteByte(value[index])
			continue
		}
		index++
		if index == len(value) || !strings.ContainsRune("\\`*_[]<>|", rune(value[index])) {
			return "", false
		}
		output.WriteByte(value[index])
	}
	return output.String(), true
}

func decisionMarkdown(d Decision) string {
	lines := []string{
		"## " + d.ID,
		"",
		"- Project: " + markdownEscape(d.Project),
		"- Run: `" + markdownEscape(string(d.RunID)) + "`",
		"- Revision: " + strconv.FormatUint(d.Revision, 10),
		"- Written: `" + markdownEscape(d.WrittenAt) + "`",
		"- Summary: " + markdownEscape(d.Summary),
		"- Rationale: " + markdownEscape(d.Rationale),
	}
	if d.Provenance != "" {
		lines = append(lines, "- Provenance: "+markdownEscape(d.Provenance))
	}
	if len(d.AlternativesConsidered) != 0 {
		lines = append(lines, "- Alternatives considered: "+markdownEscape(strings.Join(sortedStrings(d.AlternativesConsidered), "; ")))
	}
	if d.Supersedes != "" {
		lines = append(lines, "- Supersedes: `"+markdownEscape(d.Supersedes)+"`")
	}
	if d.EvidenceRef != "" {
		lines = append(lines, "- Evidence: `"+markdownEscape(d.EvidenceRef)+"`")
	}
	return strings.Join(lines, "\n") + "\n"
}

// validDecisionMarkdown recognizes the exact bounded canonical decision shape
// emitted by decisionMarkdown. A heading alone is not provenance.
func validDecisionMarkdown(markdown, id string) bool {
	header := "## " + id + "\n"
	if strings.Count(markdown, header) != 1 {
		return false
	}
	start := strings.Index(markdown, header)
	if start > 0 && markdown[start-1] != '\n' {
		return false
	}
	entry := markdown[start+len(header):]
	if next := strings.Index(entry, "\n## "); next >= 0 {
		entry = entry[:next]
	}
	fields := map[string]string{}
	counts := map[string]int{}
	for _, line := range strings.Split(entry, "\n") {
		for _, prefix := range []string{"- Project: ", "- Run: `", "- Revision: ", "- Written: `", "- Summary: ", "- Rationale: "} {
			if strings.HasPrefix(line, prefix) {
				value := strings.TrimPrefix(line, prefix)
				if strings.HasSuffix(prefix, "`") {
					if !strings.HasSuffix(value, "`") {
						return false
					}
					value = strings.TrimSuffix(value, "`")
				}
				fields[prefix] = value
				counts[prefix]++
			}
		}
	}
	for _, prefix := range []string{"- Project: ", "- Run: `", "- Revision: ", "- Written: `", "- Summary: ", "- Rationale: "} {
		if fields[prefix] == "" || counts[prefix] != 1 {
			return false
		}
		value, ok := markdownUnescape(fields[prefix])
		if !ok {
			return false
		}
		fields[prefix] = value
	}
	if err := validateText(fields["- Project: "], "project"); err != nil {
		return false
	}
	if err := validateSegment(fields["- Run: `"]); err != nil {
		return false
	}
	revision, err := strconv.ParseUint(fields["- Revision: "], 10, 64)
	if err != nil || revision == 0 {
		return false
	}
	if _, err := time.Parse(time.RFC3339, fields["- Written: `"]); err != nil {
		return false
	}
	return validateText(fields["- Summary: "], "summary") == nil && validateText(fields["- Rationale: "], "rationale") == nil
}

func nextDecisionID(markdown string) (string, error) {
	max := uint64(0)
	for _, match := range decisionHeaderPattern.FindAllStringSubmatch(markdown, -1) {
		n, err := strconv.ParseUint(match[1], 10, 64)
		if err != nil {
			return "", fmt.Errorf("%w: decision counter overflow", core.ErrLimit)
		}
		if n > max {
			max = n
		}
	}
	if max == ^uint64(0) {
		return "", fmt.Errorf("%w: decision counter overflow", core.ErrLimit)
	}
	return fmt.Sprintf("DEC-%06d", max+1), nil
}

// AppendDecision appends exactly one decision to the canonical Markdown log
// under the store root, allocating a monotonic ID while holding the local
// canonical-fact lock.
func AppendDecision(ctx context.Context, s *store.Store, decision Decision) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if s == nil {
		return "", fmt.Errorf("%w: nil store", core.ErrPath)
	}
	if err := validateDecision(decision); err != nil {
		return "", err
	}
	lock := knowledgeLock(s.Root)
	lock.Lock()
	defer lock.Unlock()
	path := "DECISIONS.md"
	contents, err := readCanonicalMarkdown(s, path)
	if err != nil && !os.IsNotExist(err) {
		return "", err
	}
	if int64(len(contents)) > canonicalLimit(s) {
		return "", fmt.Errorf("%w: decisions exceed canonical limit", core.ErrLimit)
	}
	nextID, err := nextDecisionID(string(contents))
	if err != nil {
		return "", err
	}
	if decision.ID != "" && decision.ID != nextID {
		return "", fmt.Errorf("%w: decision IDs are allocated monotonically", core.ErrRevision)
	}
	decision.ID = nextID
	if strings.Contains(string(contents), "## "+decision.ID+"\n") {
		return "", fmt.Errorf("%w: decision ID already exists", ErrConflict)
	}
	updated := string(contents)
	if updated == "" {
		updated = "# Decisions\n\n"
	}
	if !strings.HasSuffix(updated, "\n") {
		updated += "\n"
	}
	updated += "\n" + decisionMarkdown(decision)
	if _, err := s.WriteMarkdown(path, []byte(updated), canonicalLimit(s)); err != nil {
		return "", err
	}
	return decision.ID, nil
}
