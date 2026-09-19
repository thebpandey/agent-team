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
		want cli.Request
		fail bool
	}{
		{name: "plain", args: []string{"version"}, want: cli.Request{Action: "version"}},
		{name: "json", args: []string{"version", "--json"}, want: cli.Request{Action: "version", JSON: true}},
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
			if err != nil || got != tc.want {
				t.Fatalf("got=%+v err=%v want=%+v", got, err, tc.want)
			}
		})
	}
}
