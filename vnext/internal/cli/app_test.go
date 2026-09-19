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
		{"setup", "--mode", "plan"},
		{"settings", "runtime.kind=go", "tracker.kind=tasks-md"},
		{"status"},
		{"start"},
		{"start", "--run", "R-1", "--task", "T-1", "--task", "T-2"},
		{"task", "add", "--queue", "inspect feature"},
		{"task", "add", "--execute", "inspect feature"},
		{"task", "add", "--queue"},
		{"task", "add", "--execute"},
		{"one-off", "feature", "inspect feature"},
		{"one-off", "audit", "inspect config"},
		{"one-off", "review", "review change"},
		{"one-off", "feature", "--objective", "inspect feature"},
		{"pause", "--team", "TEAM-1"},
		{"pause", "--scope", "team:TEAM-1"},
		{"stop", "--run", "R-1"},
		{"cancel", "--task", "T-1"},
		{"resume", "--run", "R-1"},
		{"inspect", "--run", "R-1"},
		{"cleanup", "--team", "TEAM-1"},
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
		{[]string{"task", "add", "--queue", "inspect feature", "--json"}, cli.Action{Name: "task add", Args: []string{"--queue", "inspect feature"}, JSON: true}},
		{[]string{"one-off", "feature", "inspect feature", "--json"}, cli.Action{Name: "one-off feature", Args: []string{"inspect feature"}, JSON: true}},
		{[]string{"one-off", "feature", "--objective", "inspect feature"}, cli.Action{Name: "one-off feature", Args: []string{"inspect feature"}}},
		{[]string{"pause", "--scope", "team:TEAM-1"}, cli.Action{Name: "pause", Args: []string{"--team", "TEAM-1"}}},
		{[]string{"task", "add", "--queue"}, cli.Action{Name: "task add", Args: []string{"--queue"}}},
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
		{"execute deferred", []string{"task", "add", "--execute", "inspect feature"}, 2, "task add deferred"},
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

func TestCanonicalSelectorsNormalizeAndRepeat(t *testing.T) {
	cases := []struct {
		args []string
		want []string
	}{
		{[]string{"start", "--run", " R-1 ", "--task", " T-1 ", "--task", " T-2 "}, []string{"--run", "R-1", "--task", "T-1", "--task", "T-2"}},
		{[]string{"inspect", "--run", " R-1 "}, []string{"--run", "R-1"}},
		{[]string{"cleanup", "--team", " TEAM-1 "}, []string{"--team", "TEAM-1"}},
		{[]string{"deploy", "--batch-size", " 1 "}, []string{"--batch-size", "1"}},
	}
	for _, tc := range cases {
		got, err := cli.Parse(tc.args)
		if err != nil || !equalStrings(got.Args, tc.want) {
			t.Fatalf("args=%v got=%+v err=%v want args=%v", tc.args, got, err, tc.want)
		}
	}
}

func TestCanonicalRejectsMalformedArguments(t *testing.T) {
	for _, args := range [][]string{
		{"settings", "runtime.kind="},
		{"start", "--run"},
		{"start", "--task"},
		{"inspect", "--team"},
		{"cleanup", "--team"},
		{"deploy", "--batch-size", "00"},
		{"deploy", "--batch-size", "01"},
		{"deploy", "--batch-size", "-1"},
	} {
		if _, err := cli.Parse(args); !errors.Is(err, core.ErrPhase) {
			t.Fatalf("args=%v accepted: %v", args, err)
		}
	}
}

func TestDeferredJSONOutcomesRetainPhaseExit(t *testing.T) {
	for _, args := range [][]string{
		{"start", "--json"},
		{"cleanup", "--team", "TEAM-1", "--json"},
		{"deploy", "--json"},
		{"task", "add", "--execute", "inspect feature", "--json"},
		{"setup", "--refuse-kickoff", "--json"},
		{"setup", "--mode", "plan", "--refuse-kickoff", "--json"},
	} {
		var out bytes.Buffer
		if code := cli.Run(context.Background(), args, core.Dependencies{Stdout: &out, Stderr: &out}); code != 2 {
			t.Fatalf("args=%v code=%d output=%q", args, code, out.String())
		}
		var envelope map[string]any
		if err := json.Unmarshal(out.Bytes(), &envelope); err != nil || envelope["status"] == "accepted" {
			t.Fatalf("args=%v output=%q err=%v", args, out.String(), err)
		}
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
