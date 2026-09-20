//go:build windows

package install

import (
	"io/fs"
	"testing"
)

func TestLifecycleModeUsesWindowsFileContract(t *testing.T) {
	for _, requested := range []uint32{0o600, 0o700} {
		if got := lifecycleMode(fs.FileMode(requested)); got != 0o666 {
			t.Fatalf("lifecycleMode(%o) = %o", requested, got)
		}
	}
}
