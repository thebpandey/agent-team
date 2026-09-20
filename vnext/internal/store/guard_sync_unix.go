//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package store

import "os"

func syncGuardDirectory(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return err
	}
	err = directory.Sync()
	if closeErr := directory.Close(); err == nil {
		err = closeErr
	}
	return err
}
