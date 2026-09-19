package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"testing"

	"github.com/thebpandey/agent-team/vnext/internal/cli"
	"github.com/thebpandey/agent-team/vnext/internal/core"
)

func TestCoreAndCLIContracts(t *testing.T) {
	var _ core.TaskID = "TASK-1"
	c := core.DefaultConfig()
	if c.Storage.TrackerWarnPercent != 90 || c.Limits.ProjectTaskCapacity != 1000 {
		t.Fatal(c)
	}
	if _, err := core.ApplySettings(c, map[string]string{"runtime.kind": "node"}); !errors.Is(err, core.ErrSettings) {
		t.Fatal(err)
	}
	var out bytes.Buffer
	deps := core.Dependencies{ProjectRoot: t.TempDir(), Stdout: &out, Stderr: io.Discard}
	if cli.Run(context.Background(), []string{"version", "--json"}, deps) != 0 || !bytes.Contains(out.Bytes(), []byte(`"schema":1`)) {
		t.Fatal(out.String())
	}
	first := core.RecordEnvelope{Schema: 1, Project: "p", RunID: "R-1", WrittenAt: "2026-09-19T00:00:00Z", Revision: 1}
	encoded, _ := json.Marshal(first)
	var round core.RecordEnvelope
	if json.Unmarshal(encoded, &round) != nil || round != first {
		t.Fatal(round)
	}
}

func TestVersionParserAcceptsOnlyExactForms(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want cli.Action
		fail bool
	}{
		{name: "plain", args: []string{"version"}, want: cli.Action{Name: "version"}},
		{name: "json", args: []string{"version", "--json"}, want: cli.Action{Name: "version", JSON: true}},
		{name: "extra", args: []string{"version", "--json", "extra"}, fail: true},
		{name: "unknown flag", args: []string{"version", "--json=false"}, fail: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := cli.Parse(tc.args)
			if tc.fail {
				if !errors.Is(err, core.ErrPhase) {
					t.Fatal("invalid version form accepted")
				}
				return
			}
			if err != nil || got.Name != tc.want.Name || got.JSON != tc.want.JSON || !equalStrings(got.Args, tc.want.Args) {
				t.Fatalf("got=%+v err=%v want=%+v", got, err, tc.want)
			}
		})
	}
}

func TestCanonicalActions(t *testing.T) {
	accepted := [][]string{
		{"setup"},
		{"settings"},
		{"status"},
		{"start"},
		{"task", "add", "--queue"},
		{"task", "add", "--execute"},
		{"one-off", "feature", "--objective", "inspect feature"},
		{"one-off", "audit", "--objective", "inspect config"},
		{"one-off", "review", "--objective", "review change"},
		{"pause", "--scope", "team:TEAM-1"},
		{"inspect"},
		{"cleanup"},
		{"deploy"},
	}
	rejected := [][]string{
		{"review"}, {"gate"}, {"integrate"}, {"reconcile"}, {"checkpoint"},
		{"list"}, {"archive"}, {"plan"},
	}
	for _, args := range accepted {
		if _, err := cli.Parse(args); err != nil {
			t.Fatalf("accepted %v: %v", args, err)
		}
	}
	for _, args := range rejected {
		if _, err := cli.Parse(args); !errors.Is(err, core.ErrPhase) {
			t.Fatalf("rejected %v: %v", args, err)
		}
	}
}

func TestCanonicalActionsRetainArgumentsAndJSON(t *testing.T) {
	cases := []struct {
		args []string
		want cli.Action
	}{
		{[]string{"task", "add", "--queue", "--json"}, cli.Action{Name: "task add", Args: []string{"--queue"}, JSON: true}},
		{[]string{"one-off", "feature", "--objective", "inspect feature", "--json"}, cli.Action{Name: "one-off feature", Args: []string{"--objective", "inspect feature"}, JSON: true}},
		{[]string{"status", "--run", "RUN-1"}, cli.Action{Name: "status", Args: []string{"--run", "RUN-1"}}},
	}
	for _, tc := range cases {
		got, err := cli.Parse(tc.args)
		if err != nil || got.Name != tc.want.Name || got.JSON != tc.want.JSON || !equalStrings(got.Args, tc.want.Args) {
			t.Fatalf("args=%v got=%+v err=%v want=%+v", tc.args, got, err, tc.want)
		}
	}
}

func TestRunReportsCanonicalTextAndJSONOutcomes(t *testing.T) {
	cases := []struct {
		name     string
		args     []string
		wantCode int
		wantText string
	}{
		{"settings text", []string{"settings"}, 0, "settings accepted"},
		{"status json", []string{"status", "--json"}, 0, `"action":"status"`},
		{"start deferred", []string{"start"}, 2, "start deferred"},
		{"execute deferred", []string{"task", "add", "--execute"}, 2, "task add deferred"},
		{"invalid", []string{"gate"}, 2, "phase"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			code := cli.Run(context.Background(), tc.args, core.Dependencies{ProjectRoot: t.TempDir(), Stdout: &out, Stderr: &out})
			if code != tc.wantCode || !bytes.Contains(out.Bytes(), []byte(tc.wantText)) {
				t.Fatalf("args=%v code=%d output=%q want code=%d containing %q", tc.args, code, out.String(), tc.wantCode, tc.wantText)
			}
		})
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
