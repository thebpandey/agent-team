package install_test

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	install "github.com/thebpandey/agent-team/vnext/internal/install"
)

func writeReleaseFixture(t *testing.T, root string) install.Release {
	t.Helper()
	files := map[string]string{"agent-teamctl.bin": "vnext-binary", "WORKER-CONTRACT": "contract-v1", "SKILL.md": "entrypoint-v1"}
	paths := make(map[string]string, len(files))
	for name, contents := range files {
		path := filepath.Join(root, name)
		if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
		paths[name] = path
	}
	file := func(name string) install.ReleaseFile {
		data, err := os.ReadFile(paths[name])
		if err != nil {
			t.Fatal(err)
		}
		return install.ReleaseFile{Path: paths[name], SHA256: fmt.Sprintf("%x", sha256.Sum256(data)), Bytes: int64(len(data))}
	}
	return install.Release{Version: "1.0.0", Binary: file("agent-teamctl.bin"), Contract: file("WORKER-CONTRACT"), Entrypoints: map[install.Host]install.ReleaseFile{install.Codex: file("SKILL.md"), install.Claude: file("SKILL.md")}}
}
