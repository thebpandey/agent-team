package project

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/thebpandey/agent-team/vnext/internal/core"
)

const kickoffMaxBytes = 250 << 10

var kickoffID = regexp.MustCompile(`^[A-Za-z0-9_.:-]{1,128}$`)
var kickoffRevision = regexp.MustCompile(`^[a-f0-9]{40}([a-f0-9]{24})?$`)

type kickoff050 struct {
	SchemaVersion  int    `json:"schemaVersion"`
	Kind           string `json:"kind"`
	Status         string `json:"status"`
	ProjectKickoff struct {
		Version          string `json:"version"`
		ApprovalID       string `json:"approvalId"`
		ApprovedRevision string `json:"approvedRevision"`
	} `json:"projectKickoff"`
	AgentTeam struct {
		TestedVersion        string `json:"testedVersion"`
		InitializationSource string `json:"initializationSource"`
	} `json:"agentTeam"`
	Project struct {
		ID       string `json:"id"`
		Root     string `json:"root"`
		Branch   string `json:"branch"`
		Revision string `json:"revision"`
	} `json:"project"`
	Tracker struct {
		Kind       string `json:"kind"`
		Path       string `json:"path"`
		Executable string `json:"executable"`
	} `json:"tracker"`
	Plan struct {
		Scope                string   `json:"scope"`
		Branch               string   `json:"branch"`
		Acceptance           []string `json:"acceptance"`
		Verification         []string `json:"verification"`
		RequiredCapabilities []string `json:"requiredCapabilities"`
		Authority            struct {
			OwnedPaths      []string `json:"ownedPaths"`
			ExternalActions []string `json:"externalActions"`
		} `json:"authority"`
		Tasks []struct {
			ID core.TaskID `json:"id"`
		} `json:"tasks"`
	} `json:"plan"`
}

// LoadKickoff reads approved handoff facts without writing project or runtime
// state. The 0.5.0 envelope is a compatibility input, not a v7 runtime request.
// Verification strings retain their shell semantics and are never executed here.
func LoadKickoff(root, path string) (core.KickoffHandoff, error) {
	var result core.KickoffHandoff
	canonical, err := CanonicalRoot(root)
	if err != nil {
		return result, err
	}
	relative, _, err := inputPath(canonical, path)
	if err != nil {
		return result, err
	}
	data, err := readBoundedContained(canonical, relative, kickoffMaxBytes)
	if err != nil {
		return result, err
	}
	return decodeKickoffHandoff(canonical, data)
}

// Setup passes the exact bytes it has already digest-checked, so a changed file
// cannot substitute different approved facts between hashing and decoding.
func decodeKickoffHandoff(canonical string, data []byte) (core.KickoffHandoff, error) {
	var result core.KickoffHandoff
	if len(data) > kickoffMaxBytes {
		return result, core.ErrLimit
	}
	var shape map[string]json.RawMessage
	if err := json.Unmarshal(data, &shape); err != nil || shape == nil {
		return result, fmt.Errorf("%w: invalid Project Kickoff JSON", core.ErrSettings)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	current, err := Discover(ctx, canonical)
	if err != nil {
		return result, err
	}
	if current.Root != current.TopLevel || current.Detached {
		return result, fmt.Errorf("%w: kickoff requires the selected Git root and branch", core.ErrSettings)
	}
	_, nested := shape["schemaVersion"]
	if nested {
		var source kickoff050
		if err := decodeKickoff(data, &source); err != nil {
			return result, err
		}
		if source.SchemaVersion != 1 || source.Kind != "project-kickoff-agent-team-handoff" || source.Status != "approved" || source.ProjectKickoff.Version != "0.5.0" || !kickoffID.MatchString(source.ProjectKickoff.ApprovalID) || !kickoffID.MatchString(source.Project.ID) || source.AgentTeam.InitializationSource != "existing" {
			return result, fmt.Errorf("%w: unsupported or unapproved Project Kickoff handoff", core.ErrSettings)
		}
		if !filepath.IsAbs(source.Project.Root) || filepath.Clean(source.Project.Root) != canonical || source.Project.Revision != current.Head || source.Plan.Branch != source.Project.Branch || strings.TrimSpace(source.Plan.Scope) == "" {
			return result, fmt.Errorf("%w: Project Kickoff project or revision mismatch", core.ErrRevision)
		}
		if len(source.Plan.Authority.ExternalActions) != 0 {
			return result, fmt.Errorf("%w: external actions require a separate native authority decision", core.ErrSettings)
		}
		result = core.KickoffHandoff{ApprovedPlanRevision: source.ProjectKickoff.ApprovedRevision, Branch: source.Project.Branch, Acceptance: source.Plan.Acceptance, WritablePaths: source.Plan.Authority.OwnedPaths, Capabilities: source.Plan.RequiredCapabilities}
		switch source.Tracker.Kind {
		case "markdown":
			result.TrackerKind, result.TrackerRef = "tasks-md", source.Tracker.Path
		case "beads":
			if !filepath.IsAbs(source.Tracker.Executable) {
				return core.KickoffHandoff{}, fmt.Errorf("%w: Beads executable must be absolute", core.ErrSettings)
			}
			result.TrackerKind, result.TrackerRef = "beads", ".beads"
			result.TrackerExecutable = source.Tracker.Executable
		default:
			return core.KickoffHandoff{}, fmt.Errorf("%w: unsupported kickoff tracker", core.ErrSettings)
		}
		for _, task := range source.Plan.Tasks {
			result.TaskIDs = append(result.TaskIDs, task.ID)
		}
		for i, command := range source.Plan.Verification {
			argv := []string{"sh", "-c", command}
			if runtime.GOOS == "windows" {
				argv = []string{"cmd", "/D", "/S", "/C", command}
			}
			result.Checks = append(result.Checks, core.Check{Name: fmt.Sprintf("kickoff-%d", i+1), Command: argv})
		}
	} else if err := decodeKickoff(data, &result); err != nil {
		return result, err
	}
	if err := validateLoadedKickoff(ctx, current, result, nested); err != nil {
		return core.KickoffHandoff{}, err
	}
	return result, nil
}

func decodeKickoff(data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("%w: malformed kickoff: %v", core.ErrSettings, err)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return fmt.Errorf("%w: trailing kickoff data", core.ErrSettings)
	}
	return nil
}

func validateLoadedKickoff(ctx context.Context, project Project, handoff core.KickoffHandoff, nested bool) error {
	if handoff.TrackerExecutable != "" && (handoff.TrackerKind != "beads" || !filepath.IsAbs(handoff.TrackerExecutable) || strings.ContainsAny(handoff.TrackerExecutable, "\x00\r\n")) {
		return fmt.Errorf("%w: Beads executable must be an absolute path", core.ErrSettings)
	}
	if strings.TrimSpace(handoff.ApprovedPlanRevision) == "" || strings.TrimSpace(handoff.Branch) == "" {
		return fmt.Errorf("%w: missing approved kickoff baseline", core.ErrSettings)
	}
	if nested {
		if !kickoffRevision.MatchString(handoff.ApprovedPlanRevision) || handoff.Branch != branch(ctx, project) {
			return fmt.Errorf("%w: kickoff branch or approved revision mismatch", core.ErrRevision)
		}
		if _, err := gitAllowEmpty(ctx, project.Root, "merge-base", "--is-ancestor", handoff.ApprovedPlanRevision, project.Head); err != nil {
			return fmt.Errorf("%w: approved kickoff revision is not an ancestor of HEAD", core.ErrRevision)
		}
	}
	if (handoff.TrackerKind != "tasks-md" || (handoff.TrackerRef != "TASKS.md" && handoff.TrackerRef != ".agent-team/TASKS.md")) && (handoff.TrackerKind != "beads" || handoff.TrackerRef != ".beads") {
		return fmt.Errorf("%w: unsupported kickoff tracker reference", core.ErrSettings)
	}
	if _, _, err := inputPath(project.Root, handoff.TrackerRef); err != nil {
		return err
	}
	if len(handoff.TaskIDs) == 0 || len(handoff.TaskIDs) > 1000 || len(handoff.Acceptance) == 0 || len(handoff.Checks) == 0 || len(handoff.WritablePaths) == 0 {
		return fmt.Errorf("%w: missing approved kickoff facts", core.ErrSettings)
	}
	seen := map[core.TaskID]bool{}
	for _, id := range handoff.TaskIDs {
		if !kickoffID.MatchString(string(id)) || seen[id] {
			return fmt.Errorf("%w: invalid or duplicate kickoff task ID", core.ErrSettings)
		}
		seen[id] = true
	}
	for _, values := range [][]string{handoff.Acceptance, handoff.WritablePaths, handoff.Resources, handoff.Capabilities} {
		if len(values) > 100 {
			return core.ErrLimit
		}
		for _, value := range values {
			if strings.TrimSpace(value) == "" || len(value) > 4096 || strings.ContainsRune(value, 0) {
				return fmt.Errorf("%w: invalid kickoff fact", core.ErrSettings)
			}
		}
	}
	seenCaps := map[string]bool{}
	for _, capability := range handoff.Capabilities {
		if !kickoffID.MatchString(capability) || seenCaps[capability] {
			return fmt.Errorf("%w: invalid or duplicate kickoff capability", core.ErrSettings)
		}
		seenCaps[capability] = true
	}
	for _, owned := range handoff.WritablePaths {
		if filepath.IsAbs(owned) || portableAbsolute(owned) || strings.ContainsAny(owned, "\\\r\n\x00") {
			return fmt.Errorf("%w: unsafe kickoff owned path", core.ErrPath)
		}
		for _, part := range strings.Split(owned, "/") {
			if part == ".." || part == "" {
				return fmt.Errorf("%w: unsafe kickoff owned path", core.ErrPath)
			}
		}
		// Check existing parents before a glob, without interpreting the glob as
		// a filename or broadening the approved pattern.
		prefix := owned
		if i := strings.IndexAny(prefix, "*?["); i >= 0 {
			prefix = filepath.Dir(prefix[:i] + "placeholder")
		}
		if _, err := Contain(project.Root, filepath.Join(project.Root, prefix)); err != nil {
			return err
		}
	}
	if len(handoff.Checks) > 100 {
		return core.ErrLimit
	}
	for _, check := range handoff.Checks {
		if strings.TrimSpace(check.Name) == "" || len(check.Command) == 0 || len(check.Command) > 100 {
			return fmt.Errorf("%w: missing kickoff check", core.ErrSettings)
		}
		for _, arg := range check.Command {
			if strings.TrimSpace(arg) == "" || len(arg) > 4096 || strings.ContainsRune(arg, 0) {
				return fmt.Errorf("%w: invalid kickoff check", core.ErrSettings)
			}
		}
	}
	return nil
}
