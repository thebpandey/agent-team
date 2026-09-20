//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package store

import "syscall"

func guardReadFlags() int { return syscall.O_NONBLOCK }
