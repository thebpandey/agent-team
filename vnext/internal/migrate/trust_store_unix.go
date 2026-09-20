//go:build !windows

package migrate

import (
	"os"
	"syscall"
)

func systemTrustStorePath() string { return "/etc/agent-team/cutover-trust.json" }

func systemTrustStoreOwner(_ string, info os.FileInfo) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && stat.Uid == 0 && info.Mode().Perm()&0o022 == 0
}
