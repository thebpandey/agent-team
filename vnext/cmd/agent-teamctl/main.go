package main

import (
	"context"
	"os"

	"github.com/thebpandey/agent-team/vnext/internal/cli"
	"github.com/thebpandey/agent-team/vnext/internal/core"
	"github.com/thebpandey/agent-team/vnext/internal/lifecycle"
)

func main() {
	code := cli.Run(context.Background(), os.Args[1:], core.Dependencies{
		ProjectRoot:      ".",
		Stdout:           os.Stdout,
		Stderr:           os.Stderr,
		ExecuteLifecycle: func(ctx context.Context, name string, selector []string) error {
			return lifecycle.ExecuteLifecycle(ctx, lifecycle.ParsedAction{Name: name, Selector: selector, ScopeRequired: true}, nil, nil, nil)
		},
	})
	os.Exit(code)
}
