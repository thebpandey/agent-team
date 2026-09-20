package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/thebpandey/agent-team/vnext/internal/cli"
	"github.com/thebpandey/agent-team/vnext/internal/core"
)

func TestManagementUsesCanonicalRun(t *testing.T) {
	var out, errOut bytes.Buffer
	calls := 0
	deps := core.Dependencies{Stdout: &out, Stderr: &errOut, Management: func(_ context.Context, args []string, stdout, _ io.Writer) int {
		calls++
		if len(args) != 4 || args[0] != "install" || args[2] != "both" {
			t.Fatal(args)
		}
		_, _ = stdout.Write([]byte(`{"ok":true}`))
		return 0
	}}
	if code := cli.Run(context.Background(), []string{"install", "--host", "both", "--json"}, deps); code != 0 || calls != 1 || !bytes.Contains(out.Bytes(), []byte(`"ok":true`)) {
		t.Fatal(code, calls, out.String())
	}
}

func TestManagementRejectsInvalidShape(t *testing.T) {
	deps := core.Dependencies{Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}, Management: func(context.Context, []string, io.Writer, io.Writer) int {
		t.Fatal("callback called")
		return 0
	}}
	if code := cli.Run(context.Background(), []string{"install", "--host", "mars"}, deps); code == 0 {
		t.Fatal("invalid host accepted")
	}
	if code := cli.Run(context.Background(), []string{"update", "--host", "both"}, deps); code == 0 {
		t.Fatal("invalid update shape accepted")
	}
}

func TestManagementActionsAndJSONBound(t *testing.T) {
	calls := 0
	var seen [][]string
	var out, errOut bytes.Buffer
	deps := core.Dependencies{Stdout: &out, Stderr: &errOut, OutputLimit: 64, Management: func(_ context.Context, args []string, stdout, _ io.Writer) int {
		calls++
		seen = append(seen, append([]string(nil), args...))
		if len(args) > 0 && args[len(args)-1] == "--json" {
			_, _ = stdout.Write([]byte(`{"ok":true}`))
		}
		return 0
	}}
	valid := [][]string{{"install", "--host", "codex"}, {"install", "--host", "claude", "--json"}, {"update", "--version", "1.2.3", "--json"}, {"rollback", "--version", "8.0.0", "--json"}, {"rollback", "--version", "8.0.0", "--revision", "0123456789abcdef0123456789abcdef01234567", "--json"}, {"uninstall", "--json"}}
	for _, args := range valid {
		if code := cli.Run(context.Background(), args, deps); code != 0 {
			t.Fatal(args, code)
		}
	}
	for _, args := range [][]string{{"install"}, {"install", "--host"}, {"update"}, {"rollback", "--version"}, {"rollback", "--revision", "0123456789abcdef0123456789abcdef01234567"}, {"rollback", "--version", "8.0.0", "--revision", "bad"}, {"unknown"}, {"uninstall", "--bad"}} {
		if code := cli.Run(context.Background(), args, deps); code == 0 {
			t.Fatal("malformed command accepted", args)
		}
	}
	if calls != len(valid) || !reflect.DeepEqual(seen, valid) {
		t.Fatal(calls, seen)
	}
	out.Reset()
	deps.Management = func(_ context.Context, _ []string, stdout, _ io.Writer) int {
		_, _ = stdout.Write([]byte(`{"ok":true,"padding":"` + strings.Repeat("x", 128) + `"}`))
		return 0
	}
	if code := cli.Run(context.Background(), []string{"uninstall", "--json"}, deps); code == 0 || out.Len() > 64 || !json.Valid(out.Bytes()) || string(out.Bytes()) != `{"ok":false,"error":"output_limit"}` {
		t.Fatalf("bounded output code=%d bytes=%d json=%q", code, out.Len(), out.Bytes())
	}
}
