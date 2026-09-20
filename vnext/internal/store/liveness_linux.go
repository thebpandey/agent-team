//go:build linux

package store

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"syscall"

	"github.com/thebpandey/agent-team/vnext/internal/core"
)

type NativeLiveness struct{}

func currentProcessIdentity() (string, error) { return linuxProcessIdentity(os.Getpid()) }

func linuxProcessIdentity(pid int) (string, error) {
	body, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return "", err
	}
	end := strings.LastIndexByte(string(body), ')')
	if end < 0 {
		return "", core.ErrRevision
	}
	fields := strings.Fields(string(body[end+1:]))
	if len(fields) < 20 {
		return "", core.ErrRevision
	}
	if _, err := strconv.ParseUint(fields[19], 10, 64); err != nil {
		return "", core.ErrRevision
	}
	return fields[19], nil
}

func (NativeLiveness) HolderDead(_ context.Context, owner MutationOwner) (bool, error) {
	host, err := os.Hostname()
	if err != nil || host == "" || owner.Host != host || owner.PID <= 0 {
		return false, core.ErrRevision
	}
	identity, err := linuxProcessIdentity(owner.PID)
	if err == nil {
		live := false
		return decideHolderDead(owner.ProcessStart, identity, &live)
	}
	if errors.Is(err, os.ErrNotExist) || errors.Is(err, syscall.ESRCH) {
		return true, nil
	}
	return false, core.ErrRevision
}
