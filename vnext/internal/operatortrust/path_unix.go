//go:build !windows

package operatortrust

import (
	"os"
	"syscall"
)

func Path() string { return "/etc/agent-team/cutover-trust.json" }

func Owner(_ string, info os.FileInfo) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && stat.Uid == 0 && info.Mode().Perm()&0o022 == 0
}
