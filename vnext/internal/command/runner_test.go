package command_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/thebpandey/agent-team/vnext/internal/command"
	"github.com/thebpandey/agent-team/vnext/internal/host"
)

func TestRunnerSemanticsAndIndependentLimits(t *testing.T) {
	runner := command.NewRunner(11)
	var _ host.CommandRunner = runner

	result := runHelper(t, runner, "both")
	if result.Transport != nil || result.TimedOut || result.Exit != 0 {
		t.Fatalf("successful result = %+v", result)
	}
	if len(result.Stdout) != 11 || len(result.Stderr) != 11 {
		t.Fatalf("separate bounded streams = stdout %d, stderr %d", len(result.Stdout), len(result.Stderr))
	}

	nonzero := runHelper(t, runner, "exit")
	if nonzero.Transport != nil || nonzero.TimedOut || nonzero.Exit != 7 {
		t.Fatalf("nonzero result = %+v", nonzero)
	}

	missing := runner.Run(context.Background(), "agent-team-definitely-not-an-executable")
	if missing.Transport == nil || missing.Exit != -1 || missing.TimedOut {
		t.Fatalf("missing executable result = %+v", missing)
	}

	empty := runner.Run(context.Background(), "")
	if empty.Transport == nil || empty.Exit != -1 {
		t.Fatalf("empty executable result = %+v", empty)
	}
}

func TestRunnerDrainsNoisyChildAndReportsDeadline(t *testing.T) {
	runner := command.NewRunner(16)
	result := runHelper(t, runner, "noisy")
	if result.Transport != nil || result.TimedOut || result.Exit != 0 {
		t.Fatalf("noisy result = %+v", result)
	}
	if len(result.Stdout) != 16 || len(result.Stderr) != 16 {
		t.Fatalf("noisy streams = stdout %d, stderr %d", len(result.Stdout), len(result.Stderr))
	}

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()
	timedOut := runHelperContext(t, ctx, runner, "sleep")
	if !timedOut.TimedOut || timedOut.Transport != nil || timedOut.Exit != -1 {
		t.Fatalf("timeout result = %+v", timedOut)
	}
}

func runHelper(t *testing.T, runner command.Runner, mode string) commandResult {
	t.Helper()
	return runHelperContext(t, context.Background(), runner, mode)
}

type commandResult = struct {
	Stdout    []byte
	Stderr    []byte
	Exit      int
	TimedOut  bool
	Transport error
}

func runHelperContext(t *testing.T, ctx context.Context, runner command.Runner, mode string) commandResult {
	t.Helper()
	result := runner.Run(ctx, os.Args[0], "-test.run=TestCommandRunnerHelperProcess", "--", mode)
	return commandResult(result)
}

func TestCommandRunnerHelperProcess(t *testing.T) {
	if !strings.Contains(strings.Join(os.Args, " "), "--") {
		return
	}
	mode := os.Args[len(os.Args)-1]
	switch mode {
	case "both":
		fmt.Fprint(os.Stdout, "stdout-is-longer-than-limit")
		fmt.Fprint(os.Stderr, "stderr-is-longer-than-limit")
	case "noisy":
		for range 1 << 12 {
			fmt.Fprint(os.Stdout, "out")
			fmt.Fprint(os.Stderr, "err")
		}
	case "exit":
		os.Exit(7)
	case "sleep":
		time.Sleep(time.Second)
	default:
		fmt.Fprint(os.Stderr, "unknown helper mode")
		os.Exit(2)
	}
	os.Exit(0)
}

func TestRunnerCancellationIsTransportFailure(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result := command.NewRunner(8).Run(ctx, os.Args[0], "-test.run=TestCommandRunnerHelperProcess", "--", "sleep")
	if !errors.Is(result.Transport, context.Canceled) || result.Exit != -1 || result.TimedOut {
		t.Fatalf("cancelled result = %+v", result)
	}
}
