//go:build windows

package store

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"syscall"

	"github.com/thebpandey/agent-team/vnext/internal/core"
)

const (
	processSynchronize = 0x00100000
	waitObject0        = 0
	waitTimeout        = 258
)

type NativeLiveness struct{}

func windowsProcessIdentity(pid int) (string, syscall.Handle, error) {
	handle, err := syscall.OpenProcess(syscall.PROCESS_QUERY_INFORMATION|processSynchronize, false, uint32(pid))
	if err != nil {
		return "", 0, windowsProcessOpenError(err)
	}
	var created, exited, kernel, user syscall.Filetime
	if err := syscall.GetProcessTimes(handle, &created, &exited, &kernel, &user); err != nil {
		syscall.CloseHandle(handle)
		return "", 0, err
	}
	return fmt.Sprintf("%08x%08x", created.HighDateTime, created.LowDateTime), handle, nil
}

// With fixed access flags and a validated PID, OpenProcess error 87 means the
// process object is gone. Errors from GetProcessTimes must never use this path.
func windowsProcessOpenError(err error) error {
	if err == syscall.Errno(87) { // ERROR_INVALID_PARAMETER
		return os.ErrNotExist
	}
	return err
}

func validWindowsProcessOwner(owner MutationOwner) bool {
	if owner.PID <= 0 || uint64(owner.PID) >= uint64(^uint32(0)) {
		return false
	}
	created, err := strconv.ParseUint(owner.ProcessStart, 16, 64)
	return err == nil && created != 0 && fmt.Sprintf("%016x", created) == owner.ProcessStart
}

func currentProcessIdentity() (string, error) {
	identity, handle, err := windowsProcessIdentity(os.Getpid())
	if handle != 0 {
		syscall.CloseHandle(handle)
	}
	return identity, err
}

func (NativeLiveness) HolderDead(_ context.Context, owner MutationOwner) (bool, error) {
	host, err := os.Hostname()
	if err != nil || host == "" || owner.Host != host || !validWindowsProcessOwner(owner) {
		return false, core.ErrRevision
	}
	identity, handle, err := windowsProcessIdentity(owner.PID)
	if errors.Is(err, os.ErrNotExist) {
		return true, nil
	}
	if err != nil {
		return false, core.ErrRevision
	}
	defer syscall.CloseHandle(handle)
	state, err := syscall.WaitForSingleObject(handle, 0)
	if err != nil {
		return false, core.ErrRevision
	}
	if state == waitObject0 {
		exited := true
		return decideHolderDead(owner.ProcessStart, identity, &exited)
	}
	if state == waitTimeout {
		exited := false
		return decideHolderDead(owner.ProcessStart, identity, &exited)
	}
	return false, core.ErrRevision
}
