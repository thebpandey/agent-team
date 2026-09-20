//go:build windows

package install

import (
	"io/fs"
	"testing"
)

func lifecycleDriftMode() fs.FileMode { return 0o444 }

func TestLifecycleModeUsesWindowsFileContract(t *testing.T) {
	if got := lifecycleMode(0); got != 0 {
		t.Fatalf("lifecycleMode(0) = %o", got)
	}
	for _, requested := range []uint32{0o600, 0o700} {
		if got := lifecycleMode(fs.FileMode(requested)); got != 0o666 {
			t.Fatalf("lifecycleMode(%o) = %o", requested, got)
		}
	}
}
