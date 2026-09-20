//go:build !windows

package install

import "io/fs"

func lifecycleMode(mode fs.FileMode) uint32 { return uint32(mode.Perm()) }
