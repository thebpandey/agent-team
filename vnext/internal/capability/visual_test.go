package capability_test

import (
	"testing"

	capability "github.com/thebpandey/agent-team/vnext/internal/capability"
)

func TestVisualPair(t *testing.T) {
	got, err := capability.VisualSkills(capability.VisualRequest{Task: "T-1", RequiredEvidence: true, DesignAuthority: "sha256:d"}, nil)
	if err != nil || len(got) < 2 || got[0].Name != string(capability.Impeccable) || got[1].Name != string(capability.UIUXProMax) {
		t.Fatal(got, err)
	}
}

func TestVisualDigestInvalidationAndOptionalStyling(t *testing.T) {
	request := capability.VisualRequest{Task: "T-1", RequiredEvidence: true, DesignAuthority: "sha256:d", RequestedSkills: []capability.Name{capability.UIStyling}}
	probes := []capability.Probe{
		{Name: capability.Impeccable, Mode: capability.SkillContent, Path: "/skills/impeccable/SKILL.md", Digest: "sha256:one", Available: true, Healthy: true},
		{Name: capability.UIUXProMax, Mode: capability.SkillContent, Path: "/skills/ui-ux-pro-max/SKILL.md", Digest: "sha256:two", Available: true, Healthy: true},
		{Name: capability.UIStyling, Mode: capability.SkillContent, Path: "/skills/ui-styling/SKILL.md", Digest: "sha256:three", Available: true, Healthy: true},
	}
	first, err := capability.VisualSkills(request, probes)
	if err != nil || len(first) != 3 {
		t.Fatal(first, err)
	}
	probes[0].Digest = "sha256:changed"
	second, err := capability.VisualSkills(request, probes)
	if err != nil || first[0].Digest == second[0].Digest {
		t.Fatal("changed skill digest was reused", first, second, err)
	}
}

func TestNonVisualAndDigestInvalidation(t *testing.T) {
	got, err := capability.VisualSkills(capability.VisualRequest{Task: "T-1", RequiredEvidence: false}, nil)
	if err != nil || len(got) != 0 {
		t.Fatal(got, err)
	}
	if capability.BrowserEvidenceRequired(capability.VisualRequest{Task: "T-1", RequiredEvidence: false}) {
		t.Fatal("browser evidence required without criteria")
	}
}
