//go:build windows

package install

import "io/fs"

// Go reports regular Windows files as 0666; access control is enforced by the
// containing root's ACL rather than POSIX permission bits.
func lifecycleMode(fs.FileMode) uint32 { return 0o666 }
