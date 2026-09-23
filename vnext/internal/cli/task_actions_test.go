package cli_test

import (
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/thebpandey/agent-team/vnext/internal/cli"
	"github.com/thebpandey/agent-team/vnext/internal/core"
)

func TestTaskActionsRequireAndInvokeNativeHandler(t *testing.T) {
	for _, args := range [][]string{
		{"task", "add", "--queue", "Record this task"},
		{"task", "add", "--execute", "--from", "task.json", "--host", "claude"},
		{"one-off", "audit", "--from", "task.json"},
	} {
		var out bytes.Buffer
		if code := cli.Run(context.Background(), args, core.Dependencies{Stdout: &out, Stderr: &out}); code == 0 {
			t.Fatalf("missing handler gave false success: %v %s", args, out.String())
		}
		called := false
		code := cli.Run(context.Background(), args, core.Dependencies{Stdout: &out, Stderr: &out, Management: func(_ context.Context, got []string, stdout, stderr io.Writer) int {
			called = true
			return 7
		}})
		if code != 7 || !called {
			t.Fatalf("native task handler bypassed: %v code=%d", args, code)
		}
	}
}

func TestSetupCancellationReachesNativeHandler(t *testing.T) {
	called := false
	if code := cli.Run(context.Background(), []string{"setup", "--refuse-kickoff"}, core.Dependencies{Management: func(context.Context, []string, io.Writer, io.Writer) int {
		called = true
		return 0
	}}); code != 0 || !called {
		t.Fatalf("cancellation bypassed native handler: code=%d called=%v", code, called)
	}
}

func TestTaskInputParserRejectsUnsupportedFlags(t *testing.T) {
	for _, args := range [][]string{
		{"task", "add", "--queue", "--from"},
		{"task", "add", "--queue", "--from", "task.json", "--host", "unknown"},
		{"one-off", "feature", "--from", "task.json", "--force", "true"},
		{"task", "add", "--execute", "--unknown", "value"},
	} {
		if _, err := cli.Parse(args); err == nil {
			t.Fatalf("unsupported request accepted: %v", args)
		}
	}
}
