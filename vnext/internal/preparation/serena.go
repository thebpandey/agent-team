package preparation

import (
	"errors"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
)

// Serena 1.7 project create infers languages interactively. Passing its documented
// --language options avoids stdin prompts without starting language servers.
func serenaLanguages(root string) ([]string, error) {
	extensions := map[string]string{
		".py": "python", ".pyi": "python", ".js": "typescript", ".jsx": "typescript", ".mjs": "typescript", ".cjs": "typescript", ".ts": "typescript", ".tsx": "typescript",
		".go": "go", ".rs": "rust", ".java": "java", ".kt": "kotlin", ".kts": "kotlin", ".rb": "ruby", ".dart": "dart", ".cs": "csharp", ".php": "php",
		".c": "cpp", ".h": "cpp", ".cc": "cpp", ".cpp": "cpp", ".hpp": "cpp", ".swift": "swift", ".sh": "bash", ".lua": "lua", ".luau": "luau",
		".ex": "elixir", ".exs": "elixir", ".erl": "erlang", ".scala": "scala", ".clj": "clojure", ".cljs": "clojure", ".hs": "haskell", ".ml": "ocaml", ".nix": "nix", ".tf": "terraform", ".zig": "zig", ".vue": "vue", ".svelte": "svelte",
	}
	skip := map[string]bool{".git": true, ".agent-team": true, ".serena": true, ".beads": true, "graphify-out": true, "node_modules": true, ".venv": true, "venv": true, "vendor": true, "dist": true, "build": true, "target": true}
	set := map[string]bool{}
	count := 0
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		count++
		if count > 50000 {
			return errors.New("Serena language selection exceeds file limit; configure project languages explicitly")
		}
		if entry.IsDir() {
			if path != root && skip[entry.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.Type().IsRegular() {
			if language := extensions[strings.ToLower(filepath.Ext(entry.Name()))]; language != "" {
				set[language] = true
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if len(set) == 0 {
		return nil, errors.New("no supported source language found; create source files or configure Serena with explicit project --language options")
	}
	languages := make([]string, 0, len(set))
	for language := range set {
		languages = append(languages, language)
	}
	sort.Strings(languages)
	return languages, nil
}
