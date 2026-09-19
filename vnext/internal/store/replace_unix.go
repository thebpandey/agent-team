//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package store

import "os"

// replaceFile uses rename's same-filesystem atomic replacement semantics.
func replaceFile(source, destination string) error {
	return os.Rename(source, destination)
}

func createTemporary(directory, pattern string) (*os.File, error) {
	return os.CreateTemp(directory, pattern)
}

func cleanupTemporary(path string) error {
	return os.Remove(path)
}
