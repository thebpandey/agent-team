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
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"github.com/thebpandey/agent-team/vnext/internal/core"
)

const (
	projectMutationLock  = ".agent-team/mutation.lock"
	mutationRecoveryLock = ".agent-team/mutation.recovery"
	MutationLockPrimary  = "primary"
	MutationLockRecovery = "recovery"
	defaultMutationWait  = 30 * time.Second
)

var ownerCandidateHook func(string)
var ownerReadHook func(string)
var syncGuardNamespace = syncGuardDirectory

type MutationOwner struct {
	Token, Scope, OperationID, Host, ProcessStart, AcquiredAt, HeartbeatAt string
	PID                                                                    int
}

type MutationRecoveryRequest struct {
	Target, Token, OperationID string
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
	if relative == projectMutationLock {
		if _, err := os.Lstat(filepath.Join(g.root, filepath.FromSlash(mutationRecoveryLock))); err == nil || !errors.Is(err, fs.ErrNotExist) {
			return core.ErrRevision
		}
	}
	return removeExactOwner(g.root, relative, g.owner)
}

type HolderLiveness interface {
	HolderDead(context.Context, MutationOwner) (bool, error)
}

func decideHolderDead(recorded, current string, exited *bool) (bool, error) {
	if recorded == "" || current == "" {
		return false, core.ErrRevision
	}
	if recorded != current {
		return true, nil
	}
	if exited == nil {
		return false, core.ErrRevision
	}
	return *exited, nil
}

// AcquireProjectMutation serializes mutations and atomically publishes a
// complete, durable owner record.
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
	owner, err := newMutationOwner(scope, operationID)
	if err != nil {
		return nil, err
	}
	for {
		err = publishOwner(canonical, projectMutationLock, owner)
		if err == nil {
			return &MutationGuard{root: canonical, relative: projectMutationLock, owner: owner}, nil
		}
		if !errors.Is(err, fs.ErrExist) {
			return nil, err
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(2 * time.Millisecond):
		}
	}
}

// RecoverProjectMutation removes only the requested exact owner after a
// liveness authority proves it dead. Recovering a primary lock is serialized
// by the recovery lock; a crashed recovery owner is recovered directly.
func RecoverProjectMutation(ctx context.Context, root string, request MutationRecoveryRequest, proof HolderLiveness) (MutationOwner, error) {
	if proof == nil || !validGuardText(request.Token) || !validGuardText(request.OperationID) {
		return MutationOwner{}, core.ErrRevision
	}
	ctx, cancel, err := boundedMutationContext(ctx)
	if err != nil {
		return MutationOwner{}, err
	}
	defer cancel()
	canonical, err := canonicalMutationRoot(root)
	if err != nil {
		return MutationOwner{}, err
	}
	relative := projectMutationLock
	var claim *MutationGuard
	switch request.Target {
	case MutationLockPrimary:
		claim, err = acquireRecoveryClaim(canonical, request.OperationID)
		if err != nil {
			return MutationOwner{}, err
		}
		defer claim.Release()
	case MutationLockRecovery:
		relative = mutationRecoveryLock
	default:
		return MutationOwner{}, core.ErrRevision
	}
	owner, err := readOwner(filepath.Join(canonical, filepath.FromSlash(relative)))
	if err != nil || owner.Token != request.Token || owner.OperationID != request.OperationID {
		return MutationOwner{}, core.ErrRevision
	}
	dead, err := proof.HolderDead(ctx, owner)
	if err != nil || !dead {
		return MutationOwner{}, core.ErrRevision
	}
	if err := removeExactOwner(canonical, relative, owner); err != nil {
		return MutationOwner{}, err
	}
	return owner, nil
}

func acquireRecoveryClaim(root, operation string) (*MutationGuard, error) {
	owner, err := newMutationOwner("recovery", "recover:"+operation)
	if err != nil {
		return nil, err
	}
	if err := publishOwner(root, mutationRecoveryLock, owner); err != nil {
		if errors.Is(err, fs.ErrExist) {
			return nil, core.ErrRevision
		}
		return nil, err
	}
	return &MutationGuard{root: root, relative: mutationRecoveryLock, owner: owner}, nil
}

func newMutationOwner(scope, operationID string) (MutationOwner, error) {
	if !validGuardText(scope) || !validGuardText(operationID) {
		return MutationOwner{}, core.ErrSettings
	}
	host, err := os.Hostname()
	if err != nil || host == "" {
		return MutationOwner{}, core.ErrRevision
	}
	token, err := randomToken()
	if err != nil {
		return MutationOwner{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	identity, err := currentProcessIdentity()
	if err != nil {
		return MutationOwner{}, core.ErrRevision
	}
	return MutationOwner{Token: token, Scope: scope, OperationID: operationID, Host: host, PID: os.Getpid(), ProcessStart: identity, AcquiredAt: now, HeartbeatAt: now}, nil
}

func publishOwner(root, relative string, owner MutationOwner) error {
	path := filepath.Join(root, filepath.FromSlash(relative))
	candidate := path + ".candidate-" + owner.Token
	if err := writeOwnerExclusive(candidate, owner); err != nil {
		_ = os.Remove(candidate)
		_ = syncGuardNamespace(filepath.Dir(path))
		return err
	}
	if err := syncGuardNamespace(filepath.Dir(path)); err != nil {
		_ = os.Remove(candidate)
		_ = syncGuardNamespace(filepath.Dir(path))
		return err
	}
	if ownerCandidateHook != nil {
		ownerCandidateHook(relative)
	}
	if err := os.Link(candidate, path); err != nil {
		_ = os.Remove(candidate)
		_ = syncGuardNamespace(filepath.Dir(path))
		return err
	}
	if err := syncGuardNamespace(filepath.Dir(path)); err != nil {
		_ = os.Remove(candidate)
		_ = syncGuardNamespace(filepath.Dir(path))
		return err
	}
	if err := os.Remove(candidate); err != nil {
		return err
	}
	return syncGuardNamespace(filepath.Dir(path))
}

func removeExactOwner(root, relative string, expected MutationOwner) error {
	path := filepath.Join(root, filepath.FromSlash(relative))
	tombstoneRelative := relative + ".removed-" + expected.Token
	tombstone := filepath.Join(root, filepath.FromSlash(tombstoneRelative))
	before, err := os.Stat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return removeOwnerTombstone(root, tombstoneRelative, expected, nil)
	}
	if err != nil {
		return core.ErrRevision
	}
	current, err := readOwner(path)
	if err != nil || !reflect.DeepEqual(current, expected) {
		return core.ErrRevision
	}
	confirmed, err := os.Stat(path)
	if err != nil || !os.SameFile(before, confirmed) {
		return core.ErrRevision
	}
	if err := os.Link(path, tombstone); err != nil {
		if !errors.Is(err, fs.ErrExist) {
			return err
		}
		tombstoneInfo, statErr := os.Stat(tombstone)
		if statErr != nil || !os.SameFile(before, tombstoneInfo) {
			return core.ErrRevision
		}
	} else if err := syncGuardNamespace(filepath.Dir(path)); err != nil {
		return err
	}
	if err := removeOwnerPath(root, relative, expected, before); err != nil {
		return err
	}
	return removeOwnerTombstone(root, tombstoneRelative, expected, before)
}

func removeOwnerTombstone(root, relative string, expected MutationOwner, identity os.FileInfo) error {
	full := filepath.Join(root, filepath.FromSlash(relative))
	if identity == nil {
		var err error
		identity, err = os.Stat(full)
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		if err != nil {
			return core.ErrRevision
		}
	}
	return removeOwnerPath(root, relative, expected, identity)
}

func removeOwnerPath(root, relative string, expected MutationOwner, identity os.FileInfo) error {
	full := filepath.Join(root, filepath.FromSlash(relative))
	current, err := readOwner(full)
	if err != nil || !reflect.DeepEqual(current, expected) {
		return core.ErrRevision
	}
	confirmed, err := os.Stat(full)
	if err != nil || !os.SameFile(identity, confirmed) {
		return core.ErrRevision
	}
	opened, err := os.OpenRoot(root)
	if err != nil {
		return err
	}
	defer opened.Close()
	if err := removeOwned(opened, ownedTemp{name: filepath.ToSlash(relative), info: identity}); err != nil {
		return err
	}
	if _, err := opened.Lstat(filepath.ToSlash(relative)); err == nil {
		_ = syncGuardNamespace(filepath.Dir(full))
		return core.ErrRevision
	} else if !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return syncGuardNamespace(filepath.Dir(full))
}

func readMutationOwner(root string) (MutationOwner, error) {
	return readOwner(filepath.Join(root, filepath.FromSlash(projectMutationLock)))
}

func MutationLockOwner(root, target string) (MutationOwner, error) {
	canonical, err := canonicalMutationRoot(root)
	if err != nil {
		return MutationOwner{}, err
	}
	relative := projectMutationLock
	if target == MutationLockRecovery {
		relative = mutationRecoveryLock
	} else if target != MutationLockPrimary {
		return MutationOwner{}, core.ErrRevision
	}
	return readOwner(filepath.Join(canonical, filepath.FromSlash(relative)))
}

func readOwner(path string) (MutationOwner, error) {
	root, err := os.OpenRoot(filepath.Dir(path))
	if err != nil {
		return MutationOwner{}, core.ErrRevision
	}
	file, err := root.OpenFile(filepath.Base(path), os.O_RDONLY|guardReadFlags(), 0)
	_ = root.Close()
	if err != nil {
		return MutationOwner{}, core.ErrRevision
	}
	defer file.Close()
	if ownerReadHook != nil {
		ownerReadHook(path)
	}
	opened, err := file.Stat()
	if err != nil || !opened.Mode().IsRegular() || opened.Size() < 0 || opened.Size() > 16<<10 {
		return MutationOwner{}, core.ErrRevision
	}
	raw, err := io.ReadAll(io.LimitReader(file, (16<<10)+1))
	if err != nil || int64(len(raw)) != opened.Size() || len(raw) > 16<<10 {
		return MutationOwner{}, core.ErrRevision
	}
	current, err := os.Lstat(path)
	if err != nil || !current.Mode().IsRegular() || !os.SameFile(opened, current) {
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

func writeOwnerExclusive(path string, owner MutationOwner) error {
	raw, err := json.Marshal(owner)
	if err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
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
