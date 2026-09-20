package capability

import (
	"fmt"
	"strings"

	"github.com/thebpandey/agent-team/vnext/internal/core"
)

type VisualRequest struct {
	Task             core.TaskID
	RequiredEvidence bool
	Stack            string
	DesignAuthority  string
	RequestedSkills  []Name
}

func VisualSkills(request VisualRequest, probes []Probe) ([]core.SkillRef, error) {
	if !request.RequiredEvidence {
		return nil, nil
	}
	if request.Task == "" || request.DesignAuthority == "" {
		return nil, fmt.Errorf("visual work requires task and design authority")
	}
	names := []Name{Impeccable, UIUXProMax}
	wantStyling := stackUsesUIStyling(request.Stack)
	for _, name := range request.RequestedSkills {
		switch name {
		case Impeccable, UIUXProMax:
		case UIStyling:
			wantStyling = true
		default:
			return nil, fmt.Errorf("unsupported visual skill %q", name)
		}
	}
	if wantStyling {
		names = append(names, UIStyling)
	}
	byName := make(map[Name]Probe, len(probes))
	for _, probe := range probes {
		if _, duplicate := byName[probe.Name]; duplicate {
			return nil, fmt.Errorf("duplicate visual skill probe %q", probe.Name)
		}
		byName[probe.Name] = probe
	}
	refs := make([]core.SkillRef, 0, len(names))
	for _, name := range names {
		ref := core.SkillRef{Name: string(name), Purpose: "task-scoped visual evidence", DesignAuthority: request.DesignAuthority, Required: name != UIStyling}
		if probe, ok := byName[name]; ok {
			if probe.Mode != SkillContent || !probe.Available || !probe.Healthy || probe.Path == "" || probe.Digest == "" {
				return nil, fmt.Errorf("visual skill %q is not a verified skill-content probe", name)
			}
			ref.Path, ref.Digest = probe.Path, probe.Digest
		}
		refs = append(refs, ref)
	}
	return refs, nil
}

func BrowserEvidenceRequired(request VisualRequest) bool { return request.RequiredEvidence }

func stackUsesUIStyling(stack string) bool {
	stack = strings.ToLower(stack)
	return strings.Contains(stack, "shadcn") || strings.Contains(stack, "tailwind")
}
