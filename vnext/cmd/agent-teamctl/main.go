package main

import (
	"context"
	"os"

	"github.com/thebpandey/agent-team/vnext/internal/cli"
)

func main() {
	code := cli.Run(context.Background(), os.Args[1:], cli.Dependencies{ProjectRoot: ".", Stdout: os.Stdout, Stderr: os.Stderr})
	os.Exit(code)
}
