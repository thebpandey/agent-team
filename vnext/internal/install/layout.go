package install

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func ResolveLayout(goos string, env map[string]string) (Layout, error) {
	var base, dataRoot, codexHome, claudeHome string
	switch goos {
	case "windows":
		base = strings.TrimSpace(env["LOCALAPPDATA"])
		if base == "" {
			base, _ = os.UserConfigDir()
		}
		dataRoot = filepath.Join(base, "AgentTeam")
		codexHome = env["CODEX_HOME"]
		if codexHome == "" {
			codexHome = filepath.Join(base, "Codex")
		}
		claudeHome = env["CLAUDE_HOME"]
		if claudeHome == "" {
			claudeHome = filepath.Join(base, "Claude")
		}
	case "linux", "darwin":
		base = strings.TrimSpace(env["XDG_DATA_HOME"])
		if base == "" {
			base, _ = os.UserConfigDir()
		}
		dataRoot = filepath.Join(base, "agent-team")
		codexHome, claudeHome = env["CODEX_HOME"], env["CLAUDE_HOME"]
		if codexHome == "" {
			codexHome = filepath.Join(base, "codex")
		}
		if claudeHome == "" {
			claudeHome = filepath.Join(base, "claude")
		}
	default:
		return Layout{}, fmt.Errorf("unsupported platform %q", goos)
	}
	binary := "agent-teamctl"
	if goos == "windows" {
		binary += ".exe"
	}
	layout := Layout{
		DataRoot: dataRoot, BinaryPath: filepath.Join(dataRoot, "bin", binary), ContractPath: filepath.Join(dataRoot, "WORKER-CONTRACT"), ManifestPath: filepath.Join(dataRoot, "install-manifest.json"),
		SkillRoots: map[Host]string{Codex: filepath.Join(codexHome, "skills", "agent-team"), Claude: filepath.Join(claudeHome, "skills", "agent-team")},
	}
	if err := ValidateLayout(layout); err != nil {
		return Layout{}, err
	}
	return layout, nil
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
