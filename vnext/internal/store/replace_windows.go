//go:build windows

package store

import (
	"os"
	"time"
)

// replaceFile retries a closed-handle rename to accommodate transient sharing
// violations without deleting the known last-good destination.
func replaceFile(root *os.Root, source, destination string) error {
	var err error
	for attempt := 0; attempt < 5; attempt++ {
		if err = root.Rename(source, destination); err == nil {
			return nil
		}
		time.Sleep(time.Duration(attempt+1) * 10 * time.Millisecond)
	}
	return err
}

func createTemporary(root *os.Root, name string) (*os.File, error) {
	var file *os.File
	var err error
	for attempt := 0; attempt < 5; attempt++ {
		if file, err = root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600); err == nil {
			return file, nil
		}
		time.Sleep(time.Duration(attempt+1) * 10 * time.Millisecond)
	}
	return nil, err
}

func cleanupTemporary(root *os.Root, name string) error {
	var err error
	for attempt := 0; attempt < 5; attempt++ {
		if err = root.Remove(name); err == nil || os.IsNotExist(err) {
			return nil
		}
		time.Sleep(time.Duration(attempt+1) * 10 * time.Millisecond)
	}
	return err
}
