package main

import (
	"context"
	"os"

	"github.com/thebpandey/agent-team/vnext/internal/cli"
	"github.com/thebpandey/agent-team/vnext/internal/core"
)

func main() {
	code := cli.Run(context.Background(), os.Args[1:], core.Dependencies{ProjectRoot: ".", Stdout: os.Stdout, Stderr: os.Stderr})
	os.Exit(code)
}
