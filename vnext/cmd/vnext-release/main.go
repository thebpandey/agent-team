package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"

	"github.com/thebpandey/agent-team/vnext/internal/release"
)

func main() { os.Exit(run(os.Args[1:])) }

func run(args []string) int {
	if len(args) < 1 {
		return 2
	}
	switch args[0] {
	case "package":
		set := flag.NewFlagSet("package", flag.ContinueOnError)
		set.SetOutput(os.Stderr)
		version, commit := set.String("version", "", ""), set.String("commit", "", "")
		if set.Parse(args[1:]) != nil || set.NArg() != 0 || release.ValidatePackageArgs(*version, *commit) != nil {
			return 2
		}
		command := exec.Command("go", "build", "-trimpath", "-o", "agent-teamctl", "./cmd/agent-teamctl")
		command.Stdout, command.Stderr = os.Stdout, os.Stderr
		if command.Run() != nil || release.BuildReleasePackage("release-artifacts", *version, *commit) != nil {
			return 1
		}
		return 0
	case "verify-gates":
		if _, _, err := parseVerifyGates(args[1:]); err != nil {
			return 2
		}
		// The final readiness task supplies the executable verifier. Until then,
		// fail closed instead of accepting an unverified evidence file.
		return 1
	default:
		return 2
	}
}

func parseVerifyGates(args []string) (string, string, error) {
	set := flag.NewFlagSet("verify-gates", flag.ContinueOnError)
	set.SetOutput(os.Stderr)
	evidence, revision := set.String("evidence", "", ""), set.String("revision", "", "")
	if err := set.Parse(args); err != nil || set.NArg() != 0 || *evidence == "" || *revision == "" {
		return "", "", fmt.Errorf("verify-gates requires --evidence and --revision")
	}
	return *evidence, *revision, nil
}
