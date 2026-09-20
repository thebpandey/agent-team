package deploy

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/thebpandey/agent-team/vnext/internal/contracts"
	"github.com/thebpandey/agent-team/vnext/internal/core"
	"github.com/thebpandey/agent-team/vnext/internal/store"
)

func WriteOwnedArgumentFile(root, name string, data []byte, limit int64) (string, error) {
	if root == "" || limit <= 0 || int64(len(data)) > limit {
		return "", core.ErrPath
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return "", core.ErrPath
	}
	relative, err := filepath.Rel(absolute, filepath.Join(absolute, name))
	if err != nil || relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(name) {
		return "", core.ErrPath
	}
	state := store.New(absolute, core.StorageLimits{CanonicalBytes: limit})
	if _, err := state.WriteMarkdown(filepath.ToSlash(relative), data, limit); err != nil {
		return "", err
	}
	return filepath.Join(absolute, relative), nil
}

type NativeRunner struct {
	Root  string
	Limit int64
}

func (r NativeRunner) Run(ctx context.Context, invocation CommandInvocation) CommandResult {
	if ctx == nil {
		return CommandResult{Exit: -1, Transport: core.ErrSettings}
	}
	if err := ValidateCommandTemplate(append([]string{invocation.Executable}, invocation.Args...)); err != nil {
		return CommandResult{Exit: -1, Transport: err}
	}
	if r.Root == "" || r.Limit <= 0 {
		return CommandResult{Exit: -1, Transport: core.ErrSettings}
	}
	if invocation.ArgumentFile != "" && !contained(invocation.OwnedRoot, invocation.ArgumentFile) {
		return CommandResult{Exit: -1, Transport: core.ErrPath}
	}
	limit := invocation.OutputLimit
	if limit <= 0 || limit > r.Limit {
		limit = r.Limit
	}
	cmd := exec.CommandContext(ctx, invocation.Executable, invocation.Args...)
	cmd.Dir = r.Root
	cmd.Env = append([]string(nil), invocation.Env...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return CommandResult{Exit: -1, Transport: err}
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return CommandResult{Exit: -1, Transport: err}
	}
	if err := cmd.Start(); err != nil {
		return CommandResult{Exit: -1, Transport: err}
	}
	var out, errOut bytes.Buffer
	outTruncated, errTruncated := false, false
	copies := make(chan error, 2)
	go func() {
		_, copyErr := io.Copy(&boundedWriter{dst: &out, limit: limit, truncated: &outTruncated}, stdout)
		copies <- copyErr
	}()
	go func() {
		_, copyErr := io.Copy(&boundedWriter{dst: &errOut, limit: limit, truncated: &errTruncated}, stderr)
		copies <- copyErr
	}()
	waitErr := cmd.Wait()
	copyErrA, copyErrB := <-copies, <-copies
	result := CommandResult{Stdout: append([]byte(nil), out.Bytes()...), Stderr: append([]byte(nil), errOut.Bytes()...), Exit: 0, Started: true, OutputTruncated: outTruncated || errTruncated}
	if cmd.ProcessState != nil {
		result.Exit = cmd.ProcessState.ExitCode()
	}
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		result.TimedOut = true
	}
	if copyErrA != nil {
		result.Transport = copyErrA
	} else if copyErrB != nil {
		result.Transport = copyErrB
	} else if waitErr != nil && result.Exit == 0 {
		result.Transport = waitErr
	}
	return result
}

type boundedWriter struct {
	dst       *bytes.Buffer
	limit, n  int64
	truncated *bool
}

func (w *boundedWriter) Write(data []byte) (int, error) {
	before := w.n
	w.n += int64(len(data))
	if before < w.limit {
		keep := int64(len(data))
		if remaining := w.limit - before; keep > remaining {
			keep = remaining
		}
		_, _ = w.dst.Write(data[:keep])
	}
	if w.n > w.limit {
		*w.truncated = true
	}
	return len(data), nil
}

func (p CommandProvider) invoke(ctx context.Context, command []string, batchID, key string) (CommandResult, error) {
	if p.Runner == nil || p.Root == "" || p.Limit <= 0 || batchID == "" || key == "" {
		return CommandResult{}, core.ErrSettings
	}
	if err := ValidateCommandTemplate(command); err != nil {
		return CommandResult{}, err
	}
	root, err := os.MkdirTemp(p.Root, "deploy-")
	if err != nil {
		return CommandResult{}, err
	}
	defer os.RemoveAll(root)
	argumentFile, err := WriteOwnedArgumentFile(root, "request.json", []byte(key), p.Limit)
	if err != nil {
		return CommandResult{}, err
	}
	args := append([]string(nil), command[1:]...)
	args = append(args, "--batch-id", batchID, "--argument-file", argumentFile)
	result := p.Runner.Run(ctx, CommandInvocation{Executable: command[0], Args: args, OwnedRoot: root, ArgumentFile: argumentFile, OutputLimit: p.Limit})
	if !result.Started || result.Transport != nil {
		return result, ErrProvider
	}
	if result.OutputTruncated {
		return result, ErrOutputLimit
	}
	return result, nil
}

func (p CommandProvider) Submit(ctx context.Context, batch contracts.DeploymentBatch, key string) (contracts.Operation, error) {
	batchID := batch.IdempotencyKey
	unknown := contracts.Operation{Provider: "native", State: "unknown", Unknown: true, IdempotencyKey: key}
	if batchID == "" {
		return unknown, core.ErrBatch
	}
	result, err := p.invoke(ctx, p.Profile.ExecutorCommand, batchID, key)
	if err != nil || result.Exit != 0 {
		if err == nil {
			err = ErrProvider
		}
		return unknown, err
	}
	sum := sha256.Sum256(result.Stdout)
	return contracts.Operation{Provider: "native", ProviderID: hex.EncodeToString(sum[:]), State: "submitted", IdempotencyKey: key}, nil
}

func (p CommandProvider) Query(ctx context.Context, operation contracts.Operation, batchID string) (contracts.Operation, error) {
	result, err := p.invoke(ctx, p.Profile.QueryCommand, batchID, operation.IdempotencyKey)
	if err != nil || result.Exit != 0 {
		operation.State, operation.Unknown = "unknown", true
		if err == nil {
			err = ErrProvider
		}
		return operation, err
	}
	operation.State, operation.ExternalState, operation.Unknown = "succeeded", strings.TrimSpace(string(result.Stdout)), false
	return operation, nil
}

func (p CommandProvider) Verify(ctx context.Context, operation contracts.Operation, batchID string) (contracts.Verification, error) {
	result, err := p.invoke(ctx, p.Profile.VerificationCommand, batchID, operation.IdempotencyKey)
	verification := contracts.Verification{State: "succeeded", OutputPointer: "native:verify/" + batchID, InputFingerprint: operation.IdempotencyKey, Exit: result.Exit}
	if err != nil || result.Exit != 0 || strings.TrimSpace(string(result.Stdout)) == "failed" {
		verification.State = "failed"
		if err == nil {
			err = ErrProvider
		}
		return verification, err
	}
	return verification, nil
}

func contained(root, candidate string) bool {
	if root == "" || candidate == "" {
		return false
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return false
	}
	absCandidate, err := filepath.Abs(candidate)
	if err != nil {
		return false
	}
	relative, err := filepath.Rel(absRoot, absCandidate)
	return err == nil && relative != "." && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}
