//go:build windows

package install

import (
	"errors"
	"syscall"
)

func nativeOpenReplacementDenied(err error) bool {
	return errors.Is(err, syscall.ERROR_ACCESS_DENIED) || errors.Is(err, syscall.Errno(32)) // ERROR_SHARING_VIOLATION
}
