package capability

import (
	"fmt"
	"strings"
)

// BuildInstallPlan validates the consent record, but does not invent an
// installer command. The caller must present any executable argv separately.
func BuildInstallPlan(probe Probe, consent Consent) (InstallPlan, error) {
	if !known(probe.Name) || probe.Name == Native || consent.Name != probe.Name {
		return InstallPlan{}, fmt.Errorf("invalid capability consent")
	}
	if !consent.Enabled || (probe.Available && !probe.Healthy) {
		return InstallPlan{}, fmt.Errorf("capability %q is not eligible", probe.Name)
	}
	probeMode := probe.Mode
	if probeMode == "" {
		probeMode = modeFor(probe.Name)
	}
	mode := consent.Mode
	if mode == "" {
		mode = probeMode
	}
	if mode != probeMode || !trustedSource(consent.Source) || strings.TrimSpace(consent.InstallerPackage) == "" || strings.TrimSpace(consent.Rollback) == "" {
		return InstallPlan{}, fmt.Errorf("capability %q lacks verified explicit consent", probe.Name)
	}
	version := probe.Version
	if version == "" {
		version = installerVersion(consent.InstallerPackage)
	}
	if version == "" {
		return InstallPlan{}, fmt.Errorf("capability %q has no verified version", probe.Name)
	}
	return InstallPlan{
		Name:            probe.Name,
		Package:         consent.InstallerPackage,
		Source:          consent.Source,
		VerifiedVersion: version,
		Scope:           "project",
		Explicit:        true,
	}, nil
}

func trustedSource(source string) bool {
	return source == "verified" || strings.HasPrefix(source, "verified:")
}

func installerVersion(pkg string) string {
	at := strings.LastIndex(pkg, "@")
	if at <= 0 || at == len(pkg)-1 {
		return ""
	}
	return pkg[at+1:]
}
