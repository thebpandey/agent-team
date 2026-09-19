//go:build !aix && !darwin && !dragonfly && !freebsd && !linux && !netbsd && !openbsd && !solaris && !windows

package store

import "os"

// replaceFile is the portable fallback for platforms without a specialized
// replacement implementation.
func replaceFile(root *os.Root, source, destination string) error {
	return root.Rename(source, destination)
}

func createTemporary(root *os.Root, name string) (*os.File, error) {
	return root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
}

func cleanupTemporary(root *os.Root, name string) error {
	return root.Remove(name)
}
