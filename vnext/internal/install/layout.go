package install

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/thebpandey/agent-team/vnext/internal/core"
)

func ResolveLayout(goos string, env map[string]string) (Layout, error) {
	var base, dataRoot, home, codexHome, claudeHome string
	switch goos {
	case "windows":
		base = strings.TrimSpace(env["LOCALAPPDATA"])
		if base == "" {
			base, _ = os.UserConfigDir()
		}
		dataRoot = filepath.Join(base, "AgentTeam")
		home = strings.TrimSpace(env["USERPROFILE"])
	case "linux", "darwin":
		base = strings.TrimSpace(env["XDG_DATA_HOME"])
		if base == "" {
			base, _ = os.UserConfigDir()
		}
		dataRoot = filepath.Join(base, "agent-team")
		home = strings.TrimSpace(env["HOME"])
	default:
		return Layout{}, fmt.Errorf("unsupported platform %q", goos)
	}
	discoveryHome := home
	if home == "" {
		home, _ = os.UserHomeDir()
	}
	codexHome, claudeHome = strings.TrimSpace(env["CODEX_HOME"]), strings.TrimSpace(env["CLAUDE_HOME"])
	if codexHome == "" {
		codexHome = filepath.Join(home, ".agents")
	}
	if claudeHome == "" {
		claudeHome = filepath.Join(home, ".claude")
	}
	binary := "agent-teamctl"
	if goos == "windows" {
		binary += ".exe"
	}
	discoveryPaths := []string{filepath.Join(codexHome, "skills", "agent-team", "SKILL.md")}
	if discoveryHome != "" {
		discoveryPaths = append(discoveryPaths,
			filepath.Join(discoveryHome, ".codex", "skills", "agent-team", "SKILL.md"),
			filepath.Join(discoveryHome, ".agents", "skills", "agent-team", "SKILL.md"))
	}
	layout := Layout{
		DataRoot: dataRoot, BinaryPath: filepath.Join(dataRoot, "bin", binary), ContractPath: filepath.Join(dataRoot, "WORKER-CONTRACT"), ManifestPath: filepath.Join(dataRoot, "install-manifest.json"),
		SkillRoots:               map[Host]string{Codex: filepath.Join(codexHome, "skills", "agent-team"), Claude: filepath.Join(claudeHome, "skills", "agent-team")},
		ConfigPaths:              map[Host]string{Codex: filepath.Join(codexHome, "hooks.json"), Claude: filepath.Join(claudeHome, "settings.json")},
		CodexDiscoverySkillPaths: discoveryPaths,
	}
	if err := ValidateLayout(layout); err != nil {
		return Layout{}, err
	}
	return layout, nil
}

// ResolveInstalledLayout binds lifecycle operations to the host homes chosen
// at install time. Old manifests are migrated only from exact owned entrypoint
// paths; ambient defaults never relocate an existing installation.
func ResolveInstalledLayout(goos string, env map[string]string, manifest InstallManifest) (Layout, error) {
	layout, err := ResolveLayout(goos, env)
	if err != nil {
		return Layout{}, err
	}
	inferred, err := inferManifestHostHomes(manifest)
	if err != nil {
		return Layout{}, err
	}
	homes := inferred
	if len(manifest.HostHomes) != 0 {
		homes = manifest.HostHomes
	}
	if len(homes) == 2 && (!absoluteClean(homes[Codex]) || !absoluteClean(homes[Claude]) || samePath(goos, homes[Codex], homes[Claude])) || len(homes) != len(inferred) && len(homes) != 2 {
		return Layout{}, core.ErrRevision
	}
	for host, home := range inferred {
		if !samePath(goos, home, homes[host]) {
			return Layout{}, core.ErrRevision
		}
	}
	for _, host := range []Host{Codex, Claude} {
		key := "CODEX_HOME"
		config := "hooks.json"
		if host == Claude {
			key, config = "CLAUDE_HOME", "settings.json"
		}
		if homes[host] == "" {
			homes[host], err = hostHomeFromSkillRoot(layout.SkillRoots[host])
			if err != nil {
				return Layout{}, core.ErrRevision
			}
		}
		if override := strings.TrimSpace(env[key]); override != "" && !samePath(goos, override, homes[host]) {
			return Layout{}, core.ErrRevision
		}
		layout.SkillRoots[host] = filepath.Join(homes[host], "skills", "agent-team")
		layout.ConfigPaths[host] = filepath.Join(homes[host], config)
	}
	if samePath(goos, homes[Codex], homes[Claude]) {
		return Layout{}, core.ErrRevision
	}
	if ValidateLayout(layout) != nil {
		return Layout{}, core.ErrRevision
	}
	return layout, nil
}

func inferManifestHostHomes(manifest InstallManifest) (map[Host]string, error) {
	homes := map[Host]string{}
	entrypoints := 0
	for _, file := range manifest.Files {
		if file.Role == EntrypointRole {
			entrypoints++
		}
	}
	for _, host := range manifest.Hosts {
		if host != Codex && host != Claude || homes[host] != "" {
			return nil, core.ErrRevision
		}
		for _, file := range manifest.Files {
			if file.Role != EntrypointRole || file.Host != host {
				continue
			}
			if homes[host] != "" {
				return nil, core.ErrRevision
			}
			home := ""
			for _, suffix := range []string{filepath.Join("skills", "agent-team", "SKILL.md"), filepath.Join("skills", "agent-team", "agent-team-vnext", "SKILL.md")} {
				if marker := string(filepath.Separator) + suffix; strings.HasSuffix(file.Path, marker) {
					home = strings.TrimSuffix(file.Path, marker)
					break
				}
			}
			if !absoluteClean(home) {
				return nil, core.ErrRevision
			}
			homes[host] = home
		}
		if homes[host] == "" {
			return nil, core.ErrRevision
		}
	}
	if len(homes) == 0 || entrypoints != len(homes) {
		return nil, core.ErrRevision
	}
	return homes, nil
}

func hostHomeFromSkillRoot(root string) (string, error) {
	marker := string(filepath.Separator) + filepath.Join("skills", "agent-team")
	if !strings.HasSuffix(root, marker) {
		return "", core.ErrRevision
	}
	home := strings.TrimSuffix(root, marker)
	if !absoluteClean(home) {
		return "", core.ErrRevision
	}
	return home, nil
}

func layoutHostHomes(layout Layout) (map[Host]string, error) {
	homes := make(map[Host]string, 2)
	for _, host := range []Host{Codex, Claude} {
		home, err := hostHomeFromSkillRoot(layout.SkillRoots[host])
		if err != nil {
			return nil, err
		}
		homes[host] = home
	}
	return homes, nil
}

func validateInstalledLayout(layout Layout, manifest InstallManifest) error {
	inferred, err := inferManifestHostHomes(manifest)
	if err != nil {
		return err
	}
	expected := inferred
	if len(manifest.HostHomes) != 0 {
		expected = manifest.HostHomes
		if len(expected) != 2 || !absoluteClean(expected[Codex]) || !absoluteClean(expected[Claude]) || sameHostPath(expected[Codex], expected[Claude]) {
			return core.ErrRevision
		}
	}
	for host, home := range inferred {
		if !sameHostPath(home, expected[host]) {
			return core.ErrRevision
		}
	}
	actual, err := layoutHostHomes(layout)
	if err != nil {
		return core.ErrRevision
	}
	for host, home := range expected {
		if !sameHostPath(home, actual[host]) {
			return core.ErrRevision
		}
	}
	return nil
}

func samePath(goos, left, right string) bool {
	left, right = filepath.Clean(left), filepath.Clean(right)
	return left == right || goos == "windows" && strings.EqualFold(left, right)
}

func ValidateLayout(layout Layout) error {
	for _, path := range []string{layout.DataRoot, layout.BinaryPath, layout.ContractPath, layout.ManifestPath} {
		if !absoluteClean(path) {
			return fmt.Errorf("invalid install layout path")
		}
	}
	for _, path := range []string{layout.BinaryPath, layout.ContractPath, layout.ManifestPath} {
		if !contained(layout.DataRoot, path) {
			return fmt.Errorf("install artifact escapes data root")
		}
	}
	if len(layout.SkillRoots) != 2 {
		return fmt.Errorf("both host skill roots are required")
	}
	for _, host := range []Host{Codex, Claude} {
		if !absoluteClean(layout.SkillRoots[host]) {
			return fmt.Errorf("invalid %s skill root", host)
		}
		home, err := hostHomeFromSkillRoot(layout.SkillRoots[host])
		config := "hooks.json"
		if host == Claude {
			config = "settings.json"
		}
		if err != nil || len(layout.ConfigPaths) != 0 && layout.ConfigPaths[host] != filepath.Join(home, config) {
			return fmt.Errorf("invalid %s host layout", host)
		}
	}
	return nil
}

func absoluteClean(path string) bool {
	return path != "" && filepath.IsAbs(path) && filepath.Clean(path) == path
}

func contained(root, candidate string) bool {
	relative, err := filepath.Rel(root, candidate)
	return err == nil && relative != "." && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}
