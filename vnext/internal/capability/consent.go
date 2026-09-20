package capability

import "fmt"

// BuildInstallPlan has no production adapter in Task 20. An approved optional
// capability therefore remains an explicit native fallback instead of gaining
// a caller-controlled installer path.
func BuildInstallPlan(probe Probe, consent Consent) (InstallPlan, error) {
	if err := validateConsent(probe, consent); err != nil {
		return InstallPlan{}, err
	}
	return InstallPlan{}, fmt.Errorf("installer unavailable; use native fallback")
}

func validateConsent(probe Probe, consent Consent) error {
	if !known(consent.Name) || consent.Name == Native || !consent.Enabled || consent.Name != probe.Name || consent.Mode != modeFor(consent.Name) || consent.InstallerPackage == "" || consent.Source == "" {
		return fmt.Errorf("invalid explicit consent")
	}
	if probe.Available && (!probe.Healthy || probe.Mode != consent.Mode) {
		return fmt.Errorf("consent does not match probe")
	}
	return nil
}
