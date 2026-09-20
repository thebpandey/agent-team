//go:build aix || darwin || dragonfly || freebsd || netbsd || openbsd || solaris

package store

import (
	"context"
	"errors"
	"fmt"
	"os"
	"syscall"

	"github.com/thebpandey/agent-team/vnext/internal/core"
)

type NativeLiveness struct{}

func currentProcessIdentity() (string, error) { return fmt.Sprintf("pid:%d", os.Getpid()), nil }

func (NativeLiveness) HolderDead(_ context.Context, owner MutationOwner) (bool, error) {
	host, err := os.Hostname()
	if err != nil || owner.Host != host || owner.PID <= 0 {
		return false, core.ErrRevision
	}
	process, err := os.FindProcess(owner.PID)
	if err != nil {
		return false, core.ErrRevision
	}
	err = process.Signal(syscall.Signal(0))
	if err == nil {
		return false, nil
	}
	if errors.Is(err, os.ErrProcessDone) || errors.Is(err, syscall.ESRCH) {
		return true, nil
	}
	return false, core.ErrRevision
}
