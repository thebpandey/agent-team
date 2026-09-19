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
