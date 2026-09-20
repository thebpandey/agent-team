//go:build windows

package store

import (
	"syscall"
)

const windowsErrorInvalidHandle syscall.Errno = 6

func syncGuardDirectory(path string) error {
	name, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	handle, err := syscall.CreateFile(name, syscall.GENERIC_WRITE, syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE|syscall.FILE_SHARE_DELETE, nil, syscall.OPEN_EXISTING, syscall.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if err != nil {
		return err
	}
	defer syscall.CloseHandle(handle)
	return windowsDirectorySyncResult(syscall.FlushFileBuffers(handle))
}

// Windows does not require file systems to support FlushFileBuffers on a
// directory handle. The owner file itself is flushed before publication, so
// only ERROR_INVALID_HANDLE (the documented unsupported-handle result) may be
// accepted here; access and I/O failures remain fail-closed.
func windowsDirectorySyncResult(err error) error {
	if err == windowsErrorInvalidHandle {
		return nil
	}
	return err
}
