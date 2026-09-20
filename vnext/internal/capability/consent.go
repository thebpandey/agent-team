package capability

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const managedToolsDir = ".agent-team/tools"

// BuildInstallPlan binds a complete consent record to a single immutable plan.
// It never invents commands, paths, package versions, or rollback authority.
func BuildInstallPlan(probe Probe, consent Consent) (InstallPlan, error) {
	if err := validateConsent(probe, consent); err != nil {
		return InstallPlan{}, err
	}
	project, err := filepath.Abs(consent.ProjectRoot)
	if err != nil {
		return InstallPlan{}, fmt.Errorf("project root: %w", err)
	}
	if filepath.Clean(project) == filepath.VolumeName(project)+string(filepath.Separator) {
		return InstallPlan{}, fmt.Errorf("filesystem root is not a project")
	}
	info, err := os.Stat(project)
	if err != nil || !info.IsDir() {
		return InstallPlan{}, fmt.Errorf("invalid project root")
	}
	root := filepath.Join(project, managedToolsDir)
	if err := validateAction("install", consent.Install.Argv, root, consent.InstallerPackage, consent.VerifiedVersion); err != nil {
		return InstallPlan{}, err
	}
	if err := validateAction("probe", consent.Probe.Argv, root, consent.InstallerPackage, consent.VerifiedVersion); err != nil {
		return InstallPlan{}, err
	}
	if err := validateAction("rollback", consent.Rollback.Argv, root, consent.InstallerPackage, consent.VerifiedVersion); err != nil {
		return InstallPlan{}, err
	}
	owned := append([]OwnedFile(nil), consent.OwnedFiles...)
	if len(owned) == 0 {
		return InstallPlan{}, fmt.Errorf("install has no owned files")
	}
	seen := make(map[string]struct{}, len(owned))
	for _, file := range owned {
		if !validOwned(file) {
			return InstallPlan{}, fmt.Errorf("unsafe owned file")
		}
		if _, exists := seen[file.Path]; exists {
			return InstallPlan{}, fmt.Errorf("duplicate owned file")
		}
		seen[file.Path] = struct{}{}
	}
	return InstallPlan{state: &planState{
		name: consent.Name, source: consent.Source, version: consent.VerifiedVersion, pkg: consent.InstallerPackage, root: root,
		install: append([]string(nil), consent.Install.Argv...), probe: append([]string(nil), consent.Probe.Argv...), rollback: append([]string(nil), consent.Rollback.Argv...), owned: owned,
	}}, nil
}

func validateConsent(probe Probe, consent Consent) error {
	if !known(consent.Name) || consent.Name == Native || !consent.Enabled || consent.Name != probe.Name || consent.Mode != modeFor(consent.Name) || !verified(consent.Source) || consent.VerifiedVersion == "" || consent.InstallerPackage == "" || consent.ProjectRoot == "" {
		return fmt.Errorf("invalid explicit consent")
	}
	if probe.Available {
		if !probe.Healthy || probe.Mode != consent.Mode || probe.Source != consent.Source || probe.Version != consent.VerifiedVersion {
			return fmt.Errorf("consent does not match verified probe")
		}
	}
	if packageVersion(consent.InstallerPackage) != consent.VerifiedVersion {
		return fmt.Errorf("package version does not match consent")
	}
	return nil
}

func validateAction(kind string, argv []string, root, pkg, version string) error {
	want := []string{"agent-team-capability-" + kind, "--root", root, "--package", pkg, "--version", version}
	if len(argv) != len(want) {
		return fmt.Errorf("untrusted %s action", kind)
	}
	for i := range want {
		if argv[i] != want[i] {
			return fmt.Errorf("untrusted %s action", kind)
		}
	}
	return nil
}

func validOwned(file OwnedFile) bool {
	if file.SHA256 == "" || !strings.HasPrefix(file.SHA256, "sha256:") || unsafePortablePath(file.Path) || filepath.Clean(file.Path) != file.Path {
		return false
	}
	switch file.Role {
	case ToolBinary:
		return strings.HasPrefix(file.Path, "bin"+string(filepath.Separator))
	case ToolMetadata:
		return strings.HasPrefix(file.Path, "metadata"+string(filepath.Separator))
	default:
		return false
	}
}

func verified(source string) bool {
	return strings.HasPrefix(source, "verified:") && len(source) > len("verified:")
}

func packageVersion(pkg string) string {
	at := strings.LastIndex(pkg, "@")
	if at <= 0 || at == len(pkg)-1 {
		return ""
	}
	return pkg[at+1:]
}
