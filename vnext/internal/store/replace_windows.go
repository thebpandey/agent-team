//go:build windows

package store

import (
	"os"
	"time"
)

// replaceFile retries a closed-handle rename to accommodate transient sharing
// violations without deleting the known last-good destination.
func replaceFile(source, destination string) error {
	var err error
	for attempt := 0; attempt < 5; attempt++ {
		if err = os.Rename(source, destination); err == nil {
			return nil
		}
		time.Sleep(time.Duration(attempt+1) * 10 * time.Millisecond)
	}
	return err
}

func createTemporary(directory, pattern string) (*os.File, error) {
	var file *os.File
	var err error
	for attempt := 0; attempt < 5; attempt++ {
		if file, err = os.CreateTemp(directory, pattern); err == nil {
			return file, nil
		}
		time.Sleep(time.Duration(attempt+1) * 10 * time.Millisecond)
	}
	return nil, err
}

func cleanupTemporary(path string) error {
	var err error
	for attempt := 0; attempt < 5; attempt++ {
		if err = os.Remove(path); err == nil || os.IsNotExist(err) {
			return nil
		}
		time.Sleep(time.Duration(attempt+1) * 10 * time.Millisecond)
	}
	return err
}
