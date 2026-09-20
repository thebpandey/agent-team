//go:build windows

package store

import (
	"errors"
	"syscall"
	"testing"
)

func TestWindowsDirectorySyncErrorClassification(t *testing.T) {
	if err := windowsDirectorySyncResult(windowsErrorInvalidHandle); err != nil {
		t.Fatalf("unsupported directory flush = %v", err)
	}
	if err := windowsDirectorySyncResult(syscall.ERROR_ACCESS_DENIED); !errors.Is(err, syscall.ERROR_ACCESS_DENIED) {
		t.Fatalf("access denial swallowed: %v", err)
	}
}
