package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/thebpandey/agent-team/vnext/internal/cli"
	"github.com/thebpandey/agent-team/vnext/internal/core"
	"github.com/thebpandey/agent-team/vnext/internal/install"
	"github.com/thebpandey/agent-team/vnext/internal/lifecycle"
)

func main() {
	code := cli.Run(context.Background(), os.Args[1:], core.Dependencies{
		ProjectRoot: ".",
		Stdout:      os.Stdout,
		Stderr:      os.Stderr,
		Management:  runManagement,
		ExecuteLifecycle: func(ctx context.Context, name string, selector []string) error {
			return lifecycle.ExecuteLifecycle(ctx, lifecycle.ParsedAction{Name: name, Selector: selector, ScopeRequired: true}, nil, nil, nil)
		},
	})
	os.Exit(code)
}

func runManagement(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	layout, err := install.ResolveLayout(runtime.GOOS, map[string]string{
		"LOCALAPPDATA":  os.Getenv("LOCALAPPDATA"),
		"XDG_DATA_HOME": os.Getenv("XDG_DATA_HOME"),
		"CODEX_HOME":    os.Getenv("CODEX_HOME"),
		"CLAUDE_HOME":   os.Getenv("CLAUDE_HOME"),
	})
	if err != nil {
		return managementError(args, stdout, stderr, err)
	}
	action := args[0]
	manifest, readErr := install.NewManifestStore(layout).Read(ctx)
	expected := manifest.Revision
	var outcome install.CASOutcome
	switch action {
	case "install":
		if readErr == nil || !errors.Is(readErr, fs.ErrNotExist) {
			return managementError(args, stdout, stderr, core.ErrRevision)
		}
		release, releaseErr := localRelease("")
		if releaseErr != nil {
			return managementError(args, stdout, stderr, releaseErr)
		}
		hosts := []install.Host{install.Host(args[2])}
		if args[2] == "both" {
			hosts = []install.Host{install.Codex, install.Claude}
		}
		outcome, err = install.Install(ctx, layout, release, hosts, 0)
	case "update":
		if readErr != nil {
			return managementError(args, stdout, stderr, readErr)
		}
		release, releaseErr := localRelease(args[2])
		if releaseErr != nil {
			return managementError(args, stdout, stderr, releaseErr)
		}
		outcome, err = install.Update(ctx, layout, release, expected)
	case "rollback":
		if readErr != nil {
			return managementError(args, stdout, stderr, readErr)
		}
		outcome, err = install.Rollback(ctx, layout, args[2], expected)
	case "uninstall":
		if readErr != nil {
			return managementError(args, stdout, stderr, readErr)
		}
		_, outcome, err = install.Uninstall(ctx, layout, expected)
	}
	if err != nil {
		return managementError(args, stdout, stderr, err)
	}
	return managementResult(args, stdout, map[string]any{"ok": true, "action": action, "revision": outcome.Manifest.Revision, "retained": outcome.Retained})
}

func localRelease(requested string) (install.Release, error) {
	executable, err := os.Executable()
	if err != nil {
		return install.Release{}, err
	}
	root := filepath.Dir(executable)
	versionBytes, err := os.ReadFile(filepath.Join(root, "VERSION"))
	version := strings.TrimSpace(string(versionBytes))
	if err != nil || version == "" || (requested != "" && requested != version) {
		return install.Release{}, core.ErrRevision
	}
	file := func(path string) (install.ReleaseFile, error) {
		body, readErr := os.ReadFile(path)
		if readErr != nil {
			return install.ReleaseFile{}, readErr
		}
		sum := sha256.Sum256(body)
		return install.ReleaseFile{Path: path, SHA256: hex.EncodeToString(sum[:]), Bytes: int64(len(body))}, nil
	}
	binary, err := file(executable)
	if err != nil {
		return install.Release{}, err
	}
	contract, err := file(filepath.Join(root, "WORKER-CONTRACT"))
	if err != nil {
		return install.Release{}, err
	}
	codex, err := file(filepath.Join(root, "codex", "SKILL.md"))
	if err != nil {
		return install.Release{}, err
	}
	claude, err := file(filepath.Join(root, "claude", "SKILL.md"))
	if err != nil {
		return install.Release{}, err
	}
	return install.Release{Version: version, Binary: binary, Contract: contract, Entrypoints: map[install.Host]install.ReleaseFile{install.Codex: codex, install.Claude: claude}}, nil
}

func managementError(args []string, stdout, stderr io.Writer, err error) int {
	if len(args) > 0 && args[len(args)-1] == "--json" {
		return managementResult(args, stdout, map[string]any{"ok": false, "error": err.Error()})
	}
	_, _ = fmt.Fprintln(stderr, err)
	return 1
}

func managementResult(args []string, stdout io.Writer, value map[string]any) int {
	if len(args) > 0 && args[len(args)-1] == "--json" {
		raw, err := json.Marshal(value)
		if err != nil {
			return 1
		}
		_, err = stdout.Write(raw)
		if err != nil {
			return 1
		}
		if ok, _ := value["ok"].(bool); !ok {
			return 1
		}
		return 0
	}
	_, err := fmt.Fprintf(stdout, "%s accepted\n", args[0])
	if err != nil {
		return 1
	}
	if ok, _ := value["ok"].(bool); !ok {
		return 1
	}
	return 0
}
