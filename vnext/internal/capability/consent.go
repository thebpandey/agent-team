package capability

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
)

const managedToolsDir = ".agent-team/tools"

func BuildInstallPlan(probe Probe, consent Consent, installers Installers) (InstallPlan, error) {
	if err := validateConsent(probe, consent); err != nil {
		return InstallPlan{}, err
	}
	installer, ok := installers[consent.Name]
	if !ok {
		return InstallPlan{}, fmt.Errorf("installer unavailable; use native fallback")
	}
	if installer.Name != consent.Name || !sameSource(installer.Source, consent.Source) {
		return InstallPlan{}, fmt.Errorf("installer source mismatch")
	}
	exe, err := exec.LookPath(installer.Executable)
	if err != nil {
		return InstallPlan{}, fmt.Errorf("installer executable unavailable; use native fallback")
	}
	project, err := filepath.Abs(consent.ProjectRoot)
	if err != nil {
		return InstallPlan{}, err
	}
	if filepath.Clean(project) == filepath.VolumeName(project)+string(filepath.Separator) {
		return InstallPlan{}, fmt.Errorf("filesystem root is not a project")
	}
	info, err := os.Stat(project)
	if err != nil || !info.IsDir() {
		return InstallPlan{}, fmt.Errorf("invalid project root")
	}
	root := filepath.Join(project, managedToolsDir)
	if !reflect.DeepEqual(consent.Install.Argv, installer.Install) || !reflect.DeepEqual(consent.Rollback.Argv, installer.Rollback) || !reflect.DeepEqual(consent.ProbeArgs, installer.ProbeArgs) || len(consent.Install.Argv) == 0 || consent.Install.Argv[0] != exe || len(consent.Rollback.Argv) == 0 || consent.Rollback.Argv[0] != exe {
		return InstallPlan{}, fmt.Errorf("consent action mismatch")
	}
	owned := append([]OwnedFile(nil), consent.OwnedFiles...)
	if len(owned) != 1 || !validOwned(owned[0]) {
		return InstallPlan{}, fmt.Errorf("unsafe owned file")
	}
	artifact := filepath.Join(root, owned[0].Path)
	state := &planState{name: consent.Name, source: consent.Source, version: consent.VerifiedVersion, root: root, install: append([]string(nil), consent.Install.Argv...), rollback: append([]string(nil), consent.Rollback.Argv...), probe: append([]string{artifact}, consent.ProbeArgs...), owned: owned}
	state.binding = planBinding(state)
	return InstallPlan{state: state}, nil
}
func validateConsent(p Probe, c Consent) error {
	if !known(c.Name) || c.Name == Native || !c.Enabled || c.Name != p.Name || c.Mode != modeFor(c.Name) || !validSource(c.Source) || c.VerifiedVersion == "" || c.VerifiedVersion != c.Source.Version || c.ProjectRoot == "" {
		return fmt.Errorf("invalid explicit consent")
	}
	if p.Available && (!p.Healthy || p.Version != c.VerifiedVersion || !sameSource(p.Source, c.Source)) {
		return fmt.Errorf("consent does not match probe")
	}
	return nil
}
func validSource(s VerifiedSource) bool {
	return s.Identity != "" && s.Version != "" && strings.HasPrefix(s.Digest, "sha256:") && len(s.Digest) > 7
}
func sameSource(a, b VerifiedSource) bool {
	return a.Identity == b.Identity && a.Digest == b.Digest && a.Version == b.Version
}
func validOwned(f OwnedFile) bool {
	if !strings.HasPrefix(f.SHA256, "sha256:") || unsafePortablePath(f.Path) || filepath.Clean(f.Path) != f.Path {
		return false
	}
	return f.Role == ToolBinary && strings.HasPrefix(f.Path, "bin"+string(filepath.Separator))
}
