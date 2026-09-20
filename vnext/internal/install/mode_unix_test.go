//go:build !windows

package install

import (
	"io/fs"
	"testing"
)

func lifecycleDriftMode() fs.FileMode { return 0o644 }

func TestLifecycleModeUsesPOSIXPermissions(t *testing.T) {
	if got := lifecycleMode(0o700); got != 0o700 {
		t.Fatalf("lifecycleMode(0700) = %o", got)
	}
}
