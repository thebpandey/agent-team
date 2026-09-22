//go:build windows

package store

import (
	"context"
	"errors"
	"os"
	"strconv"
	"syscall"
	"testing"

	"github.com/thebpandey/agent-team/vnext/internal/core"
)

func TestWindowsProcessOpenErrorOnlyAcceptsAbsentProcess(t *testing.T) {
	for _, test := range []struct {
		name   string
		err    error
		absent bool
	}{
		{"absent PID", syscall.Errno(87), true},
		{"access denied", syscall.ERROR_ACCESS_DENIED, false},
		{"invalid handle", syscall.Errno(6), false},
		{"unknown error", syscall.Errno(1234), false},
	} {
		t.Run(test.name, func(t *testing.T) {
			got := windowsProcessOpenError(test.err)
			if errors.Is(got, os.ErrNotExist) != test.absent {
				t.Fatalf("unexpected absent process classification: %v", got)
			}
			if !test.absent && got != test.err {
				t.Fatalf("changed unknown error: %v", got)
			}
		})
	}
	// The same error from a later API is not proof that OpenProcess found no PID.
	if errors.Is(syscall.Errno(87), os.ErrNotExist) {
		t.Fatal("GetProcessTimes invalid parameter would authorize recovery")
	}
}

func TestWindowsLivenessRejectsInvalidOwnerIdentity(t *testing.T) {
	owner, err := newMutationOwner("test", "windows-invalid-owner")
	if err != nil {
		t.Fatal(err)
	}
	if !validWindowsProcessOwner(owner) {
		t.Fatal("native owner identity rejected")
	}
	for _, identity := range []string{"", "unknown", "0000000000000000", "00000000000000AB", "1", "-000000000000001"} {
		t.Run("identity="+identity, func(t *testing.T) {
			invalid := owner
			invalid.ProcessStart = identity
			if dead, err := (NativeLiveness{}).HolderDead(context.Background(), invalid); dead || !errors.Is(err, core.ErrRevision) {
				t.Fatalf("unknown identity accepted: dead=%v err=%v", dead, err)
			}
		})
	}
	pids := []int{0, -1}
	if strconv.IntSize == 64 {
		maxDWORD := uint64(^uint32(0))
		pids = append(pids, int(maxDWORD), int(maxDWORD+1))
	}
	for _, pid := range pids {
		invalid := owner
		invalid.PID = pid
		if dead, err := (NativeLiveness{}).HolderDead(context.Background(), invalid); dead || !errors.Is(err, core.ErrRevision) {
			t.Fatalf("invalid PID %d accepted: dead=%v err=%v", pid, dead, err)
		}
	}
	owner.Host += "-another-host"
	if dead, err := (NativeLiveness{}).HolderDead(context.Background(), owner); dead || !errors.Is(err, core.ErrRevision) {
		t.Fatalf("foreign host accepted: dead=%v err=%v", dead, err)
	}
}
