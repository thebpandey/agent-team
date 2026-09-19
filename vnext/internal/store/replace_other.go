//go:build !aix && !darwin && !dragonfly && !freebsd && !linux && !netbsd && !openbsd && !solaris && !windows

package store

import "os"

// replaceFile is the portable fallback for platforms without a specialized
// replacement implementation.
func replaceFile(source, destination string) error {
	return os.Rename(source, destination)
}

func createTemporary(directory, pattern string) (*os.File, error) {
	return os.CreateTemp(directory, pattern)
}

func cleanupTemporary(path string) error {
	return os.Remove(path)
}
