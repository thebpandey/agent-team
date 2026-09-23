package preparation

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

type unsupportedReceipt struct{ Root, Path, Version, Digest string }

func unsupportedPath(root, name string) string {
	return filepath.Join(root, ".agent-team", "dependencies", "prepared", name+"-unsupported.json")
}

func recordUnsupportedFailure(root string, d *Dependency, approved bool, err error) {
	if !approved || d.Scope != "existing" || err == nil {
		return
	}
	message := strings.ToLower(err.Error())
	unsupported := false
	for _, phrase := range []string{"unknown flag", "unknown command", "no such option", "no such command", "unrecognized arguments", "unsupported option", "unsupported flag", "unsupported command"} {
		if strings.Contains(message, phrase) {
			unsupported = true
			break
		}
	}
	if !unsupported {
		return
	}
	path := unsupportedPath(root, d.Name)
	if ensureDirectory(root, filepath.Dir(path)) != nil {
		return
	}
	data, _ := json.Marshal(unsupportedReceipt{Root: root, Path: d.Path, Version: d.Version, Digest: executableDigest(d.Path)})
	_ = writeReceipt(path, data)
}

func hasUnsupportedFailure(root string, d Dependency) bool {
	if d.Scope != "existing" {
		return false
	}
	data, err := readProjectFile(root, unsupportedPath(root, d.Name), 4096)
	if err != nil {
		return false
	}
	var receipt unsupportedReceipt
	if json.Unmarshal(data, &receipt) != nil || receipt.Root != root || receipt.Path != d.Path || receipt.Version != d.Version {
		return false
	}
	return receipt.Digest == "" || receipt.Digest == executableDigest(d.Path)
}

func executableDigest(path string) string {
	file, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > 32<<20 {
		return ""
	}
	data, err := readBounded(file, 32<<20)
	if err != nil {
		return ""
	}
	return digest(data)
}
