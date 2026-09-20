//go:build !windows

package install

import "testing"

func TestLifecycleModeUsesPOSIXPermissions(t *testing.T) {
	if got := lifecycleMode(0o700); got != 0o700 {
		t.Fatalf("lifecycleMode(0700) = %o", got)
	}
}
