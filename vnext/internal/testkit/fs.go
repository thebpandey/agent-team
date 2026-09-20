package testkit

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/thebpandey/agent-team/vnext/internal/core"
	"github.com/thebpandey/agent-team/vnext/internal/store"
)

func Fixture(t *testing.T, name string) string {
	t.Helper()
	root := t.TempDir()
	if name == "" {
		return root
	}
	path := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

// PosixContainmentAndAtomicRename exercises the store's native replacement
// path. Callers pair it with project.Contain for their scoped path assertion.
func PosixContainmentAndAtomicRename() error {
	root, err := os.MkdirTemp("", "agent-team-posix-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(root)
	return replaceRecord(root)
}

// WindowsSharingReplacement exercises the bounded Windows replacement path.
func WindowsSharingReplacement() error {
	root, err := os.MkdirTemp("", "agent-team-windows-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(root)
	return replaceRecord(root)
}

func replaceRecord(root string) error {
	state := store.New(root, core.DefaultConfig().Storage)
	if _, err := state.WriteJSON("record.json", map[string]string{"state": "before"}, 1024); err != nil {
		return err
	}
	if _, err := state.WriteJSON("record.json", map[string]string{"state": "after"}, 1024); err != nil {
		return err
	}
	var got map[string]string
	if err := state.ReadJSON("record.json", 1024, &got); err != nil {
		return err
	}
	if got["state"] != "after" {
		return fmt.Errorf("replacement state %q", got["state"])
	}
	return nil
}
