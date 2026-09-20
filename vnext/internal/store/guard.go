package store

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"syscall"
	"time"

	"github.com/thebpandey/agent-team/vnext/internal/core"
)

const (
	projectMutationLock  = ".agent-team/mutation.lock"
	mutationRecoveryLock = ".agent-team/mutation.recovery"
	mutationOwnerFile    = "owner.json"
	defaultMutationWait  = 30 * time.Second
)

var processStarted = time.Now().UTC().Format(time.RFC3339Nano)

type MutationOwner struct {
	Token, Scope, OperationID, Host, ProcessStart, AcquiredAt, HeartbeatAt string
	PID                                                                    int
}

type MutationGuard struct {
	root, relative string
	owner          MutationOwner
}

func (g *MutationGuard) Owner() MutationOwner {
	if g == nil {
		return MutationOwner{}
	}
	return g.owner
}

func (g *MutationGuard) Release() error {
	if g == nil || g.root == "" {
		return core.ErrPath
	}
	relative := g.relative
	if relative == "" {
		relative = projectMutationLock
	}
	current, err := readOwner(filepath.Join(g.root, filepath.FromSlash(relative), mutationOwnerFile))
	if err != nil || !reflect.DeepEqual(current, g.owner) {
		return core.ErrRevision
	}
	return removeOwnedLock(g.root, relative, g.owner)
}

type HolderLiveness interface {
	HolderDead(context.Context, MutationOwner) (bool, error)
}

type NativeLiveness struct{}

func (NativeLiveness) HolderDead(_ context.Context, owner MutationOwner) (bool, error) {
	host, err := os.Hostname()
	if err != nil || host == "" || owner.Host != host || owner.PID <= 0 {
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
	if errors.Is(err, os.ErrProcessDone) {
		return true, nil
	}
	return false, core.ErrRevision
}

// AcquireProjectMutation serializes mutations and durably records exact guard ownership.
func AcquireProjectMutation(ctx context.Context, root, scope, operationID string) (*MutationGuard, error) {
	ctx, cancel, err := boundedMutationContext(ctx)
	if err != nil {
		return nil, err
	}
	defer cancel()
	canonical, err := canonicalMutationRoot(root)
	if err != nil {
		return nil, err
	}
	if !validGuardText(scope) || !validGuardText(operationID) {
		return nil, core.ErrSettings
	}
	host, err := os.Hostname()
	if err != nil || host == "" {
		return nil, core.ErrRevision
	}
	token, err := randomToken()
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	owner := MutationOwner{Token: token, Scope: scope, OperationID: operationID, Host: host, PID: os.Getpid(), ProcessStart: processStarted, AcquiredAt: now, HeartbeatAt: now}
	lock := filepath.Join(canonical, filepath.FromSlash(projectMutationLock))
	for {
		if err := os.Mkdir(lock, 0o700); err == nil {
			if err := writeOwner(filepath.Join(lock, mutationOwnerFile), owner); err != nil {
				_ = os.Remove(lock)
				return nil, err
			}
			return &MutationGuard{root: canonical, relative: projectMutationLock, owner: owner}, nil
		} else if !errors.Is(err, os.ErrExist) {
			return nil, fmt.Errorf("%w: mutation guard: %v", core.ErrPath, err)
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(2 * time.Millisecond):
		}
	}
}

// RecoverProjectMutation removes only the exact expected guard after a liveness
// authority proves its holder dead. Age alone never authorizes recovery.
func RecoverProjectMutation(ctx context.Context, root string, expected MutationOwner, proof HolderLiveness) error {
	if proof == nil || !validMutationOwner(expected) {
		return core.ErrRevision
	}
	ctx, cancel, err := boundedMutationContext(ctx)
	if err != nil {
		return err
	}
	defer cancel()
	canonical, err := canonicalMutationRoot(root)
	if err != nil {
		return err
	}
	claim, err := acquireRecoveryClaim(canonical)
	if err != nil {
		return err
	}
	defer claim.Release()
	current, err := readMutationOwner(canonical)
	if err != nil || !reflect.DeepEqual(current, expected) {
		return core.ErrRevision
	}
	dead, err := proof.HolderDead(ctx, current)
	if err != nil || !dead {
		return core.ErrRevision
	}
	current, err = readMutationOwner(canonical)
	if err != nil || !reflect.DeepEqual(current, expected) {
		return core.ErrRevision
	}
	return removeOwnedLock(canonical, projectMutationLock, expected)
}

func acquireRecoveryClaim(root string) (*MutationGuard, error) {
	token, err := randomToken()
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	host, _ := os.Hostname()
	owner := MutationOwner{Token: token, Scope: "recovery", OperationID: token, Host: host, PID: os.Getpid(), ProcessStart: processStarted, AcquiredAt: now, HeartbeatAt: now}
	lock := filepath.Join(root, filepath.FromSlash(mutationRecoveryLock))
	if err := os.Mkdir(lock, 0o700); err != nil {
		if errors.Is(err, os.ErrExist) {
			return nil, core.ErrRevision
		}
		return nil, err
	}
	if err := writeOwner(filepath.Join(lock, mutationOwnerFile), owner); err != nil {
		_ = os.Remove(lock)
		return nil, err
	}
	return &MutationGuard{root: root, relative: mutationRecoveryLock, owner: owner}, nil
}

func removeOwnedLock(root, relative string, expected MutationOwner) error {
	path := filepath.Join(root, filepath.FromSlash(relative))
	var current MutationOwner
	raw, err := os.ReadFile(filepath.Join(path, mutationOwnerFile))
	if err != nil || json.Unmarshal(raw, &current) != nil || !reflect.DeepEqual(current, expected) {
		return core.ErrRevision
	}
	if err := os.Remove(filepath.Join(path, mutationOwnerFile)); err != nil {
		return err
	}
	return os.Remove(path)
}

func readMutationOwner(root string) (MutationOwner, error) {
	return readOwner(filepath.Join(root, filepath.FromSlash(projectMutationLock), mutationOwnerFile))
}

func readOwner(path string) (MutationOwner, error) {
	raw, err := os.ReadFile(path)
	if err != nil || len(raw) > 16<<10 {
		return MutationOwner{}, core.ErrRevision
	}
	var owner MutationOwner
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&owner) != nil || decoder.Decode(&struct{}{}) != io.EOF || !validMutationOwner(owner) {
		return MutationOwner{}, core.ErrRevision
	}
	return owner, nil
}

func writeOwner(path string, owner MutationOwner) error {
	raw, err := json.Marshal(owner)
	if err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if _, err = file.Write(append(raw, '\n')); err == nil {
		err = file.Sync()
	}
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	return err
}

func validMutationOwner(owner MutationOwner) bool {
	return len(owner.Token) == 32 && validGuardText(owner.Scope) && validGuardText(owner.OperationID) && owner.PID > 0 && owner.Host != "" && owner.ProcessStart != "" && owner.AcquiredAt != "" && owner.HeartbeatAt != ""
}

func ValidateMutationOwner(owner MutationOwner) error {
	if !validMutationOwner(owner) {
		return core.ErrRevision
	}
	return nil
}

func validGuardText(value string) bool {
	return value != "" && len(value) <= 256 && strings.TrimSpace(value) == value
}

func randomToken() (string, error) {
	var token [16]byte
	if _, err := rand.Read(token[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(token[:]), nil
}

func boundedMutationContext(ctx context.Context) (context.Context, context.CancelFunc, error) {
	if ctx == nil {
		return nil, nil, core.ErrPath
	}
	if _, ok := ctx.Deadline(); ok {
		bounded, cancel := context.WithCancel(ctx)
		return bounded, cancel, nil
	}
	bounded, cancel := context.WithTimeout(ctx, defaultMutationWait)
	return bounded, cancel, nil
}

func canonicalMutationRoot(root string) (string, error) {
	if root == "" {
		return "", core.ErrPath
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return "", fmt.Errorf("%w: project root: %v", core.ErrPath, err)
	}
	canonical, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", fmt.Errorf("%w: canonical project root: %v", core.ErrPath, err)
	}
	canonical, err = filepath.Abs(canonical)
	if err != nil {
		return "", fmt.Errorf("%w: absolute project root: %v", core.ErrPath, err)
	}
	if err := os.MkdirAll(filepath.Join(canonical, ".agent-team"), 0o700); err != nil {
		return "", fmt.Errorf("%w: mutation guard parent: %v", core.ErrPath, err)
	}
	return canonical, nil
}
