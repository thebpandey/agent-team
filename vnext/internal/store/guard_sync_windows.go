//go:build windows

package store

import (
	"syscall"
)

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
	return syscall.FlushFileBuffers(handle)
}
