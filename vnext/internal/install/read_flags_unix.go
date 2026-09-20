//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package install

import "syscall"

func stableReadFlags() int { return syscall.O_NONBLOCK }
