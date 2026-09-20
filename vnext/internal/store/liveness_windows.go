//go:build windows

package store

import (
	"context"
	"fmt"
	"os"
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
		return "", 0, err
	}
	var created, exited, kernel, user syscall.Filetime
	if err := syscall.GetProcessTimes(handle, &created, &exited, &kernel, &user); err != nil {
		syscall.CloseHandle(handle)
		return "", 0, err
	}
	return fmt.Sprintf("%08x%08x", created.HighDateTime, created.LowDateTime), handle, nil
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
	if err != nil || owner.Host != host || owner.PID <= 0 {
		return false, core.ErrRevision
	}
	identity, handle, err := windowsProcessIdentity(owner.PID)
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
