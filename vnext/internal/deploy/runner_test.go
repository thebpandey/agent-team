package deploy

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/thebpandey/agent-team/vnext/internal/contracts"
	"github.com/thebpandey/agent-team/vnext/internal/core"
)

func TestRunnerChild(t *testing.T) {
	if os.Getenv("AGENT_TEAM_RUNNER_CHILD") != "1" {
		return
	}
	_, _ = os.Stdout.Write([]byte(strings.Repeat("x", 1<<20)))
	_, _ = os.Stderr.Write([]byte(strings.Repeat("e", 1<<20)))
	os.Exit(0)
}

func TestCommandValidationAndStartFailure(t *testing.T) {
	for _, bad := range [][]string{{}, {"sh", "-c", "echo x"}, {"cmd.exe", "/c", "echo x"}, {"powershell.exe", "-Command", "echo x"}, {"provider", "$(secret)"}, {"provider", "${TOKEN}"}, {"provider", `..\escape`}} {
		if err := ValidateCommandTemplate(bad); err == nil {
			t.Fatal("accepted", bad)
		}
	}
	result := (NativeRunner{Root: t.TempDir(), Limit: 8}).Run(context.Background(), CommandInvocation{Executable: ""})
	if result.Started || result.Exit != -1 || result.Transport == nil {
		t.Fatal(result)
	}
}

func TestOwnedArgumentFile(t *testing.T) {
	root := t.TempDir()
	path, err := WriteOwnedArgumentFile(root, "args.json", []byte(`{"target":"staging"}`), 64)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(path, root) {
		t.Fatal(path)
	}
	if _, err := WriteOwnedArgumentFile(root, filepath.Join("..", "escape"), []byte("x"), 64); !errors.Is(err, core.ErrPath) {
		t.Fatal(err)
	}
}

func TestNativeRunnerBoundsOutput(t *testing.T) {
	result := (NativeRunner{Root: t.TempDir(), Limit: 8}).Run(context.Background(), CommandInvocation{Executable: os.Args[0], Args: []string{"-test.run=TestRunnerChild"}, Env: append(os.Environ(), "AGENT_TEAM_RUNNER_CHILD=1"), OutputLimit: 8})
	if len(result.Stdout) > 8 || !result.OutputTruncated {
		t.Fatal(result)
	}
}

func TestNativeRunnerDrainsNoisyChild(t *testing.T) {
	result := (NativeRunner{Root: t.TempDir(), Limit: 8}).Run(context.Background(), CommandInvocation{Executable: os.Args[0], Args: []string{"-test.run=TestRunnerChild"}, Env: append(os.Environ(), "AGENT_TEAM_RUNNER_CHILD=1"), OutputLimit: 8})
	if result.Exit != 0 || len(result.Stderr) > 8 {
		t.Fatal(result)
	}
}

type outputLimitedRunner struct{}

func (outputLimitedRunner) Run(context.Context, CommandInvocation) CommandResult {
	return CommandResult{Started: true, OutputTruncated: true}
}

func TestCommandProviderOutputLimitIsUnknown(t *testing.T) {
	profile := TargetProfile{ExecutorCommand: []string{"provider"}, QueryCommand: []string{"provider-query"}, VerificationCommand: []string{"provider-verify"}}
	provider := CommandProvider{Profile: profile, Runner: outputLimitedRunner{}, Root: t.TempDir(), Limit: 8}
	op, err := provider.Submit(context.Background(), contracts.DeploymentBatch{Run: "R-1", IdempotencyKey: "batch-1"}, "key-1")
	if !errors.Is(err, ErrOutputLimit) || !op.Unknown || op.State != "unknown" {
		t.Fatal(op, err)
	}
	query, err := provider.Query(context.Background(), contracts.Operation{ProviderID: "op", IdempotencyKey: "key-1"}, "batch-1")
	if !errors.Is(err, ErrOutputLimit) || !query.Unknown {
		t.Fatal(query, err)
	}
	verification, err := provider.Verify(context.Background(), contracts.Operation{ProviderID: "op", IdempotencyKey: "key-1"}, "batch-1")
	if !errors.Is(err, ErrOutputLimit) || verification.State != "failed" {
		t.Fatal(verification, err)
	}
}
