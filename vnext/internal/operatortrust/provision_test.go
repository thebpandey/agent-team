//go:build !windows

package operatortrust

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestProvisionScriptCreatesTrustOnce(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("provisioning requires root")
	}
	for _, command := range []string{"bash", "openssl", "jq", "sha256sum", "base64", "install", "tail", "stat", "tr", "mktemp"} {
		if _, err := exec.LookPath(command); err != nil {
			t.Skipf("%s unavailable", command)
		}
	}
	root := t.TempDir()
	destination := filepath.Join(root, "trust")
	privateKey := filepath.Join(root, "operator-private.pem")
	script := filepath.Join("..", "..", "..", "scripts", "provision-cutover-trust.sh")
	command := exec.Command("bash", script)
	command.Env = append(os.Environ(), "AGENT_TEAM_TEST_MODE=1", "DEST_DIR="+destination, "PRIVATE_KEY="+privateKey)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("provision: %s: %v", output, err)
	}
	privateRaw, err := os.ReadFile(privateKey)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(output), string(privateRaw)) || !strings.Contains(string(output), "key id:") {
		t.Fatalf("unsafe output: %s", output)
	}
	trustPath := filepath.Join(destination, "cutover-trust.json")
	trustRaw, err := os.ReadFile(trustPath)
	if err != nil {
		t.Fatal(err)
	}
	var trust document
	if json.Unmarshal(trustRaw, &trust) != nil || trust.Schema != 1 || len(trust.Keys) != 1 {
		t.Fatalf("trust = %s", trustRaw)
	}
	if info, err := os.Stat(privateKey); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("private mode: %v, %v", info, err)
	}
	if info, err := os.Stat(trustPath); err != nil || info.Mode().Perm() != 0o644 {
		t.Fatalf("trust mode: %v, %v", info, err)
	}

	command = exec.Command("bash", script)
	command.Env = append(os.Environ(), "AGENT_TEAM_TEST_MODE=1", "DEST_DIR="+destination, "PRIVATE_KEY="+privateKey)
	if repeat, err := command.CombinedOutput(); err == nil || !strings.Contains(string(repeat), "refusing to overwrite") {
		t.Fatalf("repeat = %s, %v", repeat, err)
	}
}

func TestProvisionScriptRejectsUnsafeOverrides(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() != 0 {
		t.Skip("Linux root behavior")
	}
	script := filepath.Join("..", "..", "..", "scripts", "provision-cutover-trust.sh")
	command := exec.Command("bash", script)
	command.Env = append(os.Environ(), "AGENT_TEAM_TEST_MODE=1", "DEST_DIR=/etc/agent-team-test", "PRIVATE_KEY=/root/test.pem")
	if output, err := command.CombinedOutput(); err == nil || !strings.Contains(string(output), "below /tmp") {
		t.Fatalf("unsafe override = %s, %v", output, err)
	}
}
