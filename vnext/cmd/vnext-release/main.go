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
		if buildAgentTeamctl(".", "agent-teamctl", *version, *commit) != nil || release.BuildReleasePackage("release-artifacts", *version, *commit) != nil {
			return 1
		}
		return 0
	case "verify-gates":
		evidence, revision, err := parseVerifyGates(args[1:])
		if err != nil {
			return 2
		}
		if release.VerifyReadinessEvidence(evidence, revision) != nil {
			return 1
		}
		return 0
	case "collect-evidence":
		revision, output, native, benchmark, err := parseCollectEvidence(args[1:])
		if err != nil {
			return 2
		}
		if collectEvidence(revision, output, native, benchmark) != nil {
			return 1
		}
		return 0
	case "finalize-evidence":
		revision, output, artifact, sbom, canary, rollback, provider, installed, err := parseFinalizeEvidence(args[1:])
		if err != nil {
			return 2
		}
		if finalizeEvidence(revision, output, artifact, sbom, canary, rollback, provider, installed) != nil {
			return 1
		}
		return 0
	case "write-readiness":
		revision, gates, paths, err := parseReadiness(args[1:])
		if err != nil {
			return 2
		}
		evidence, err := release.BuildReadinessEvidence(revision, gates, paths)
		if err != nil || release.WriteReadinessEvidence("release-readiness.json", evidence) != nil {
			return 1
		}
		return 0
	default:
		return 2
	}
}

func buildAgentTeamctl(source, output, version, revision string) error {
	ldflags := fmt.Sprintf("-X github.com/thebpandey/agent-team/vnext/internal/cli.version=%s -X github.com/thebpandey/agent-team/vnext/internal/cli.revision=%s", version, revision)
	command := exec.Command("go", "build", "-trimpath", "-ldflags", ldflags, "-o", output, "./cmd/agent-teamctl")
	command.Dir, command.Stdout, command.Stderr = source, os.Stdout, os.Stderr
	return command.Run()
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
