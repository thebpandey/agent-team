package core_test

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/thebpandey/agent-team/vnext/internal/core"
)

func TestPhase1CoreContracts(t *testing.T) {
	config := core.DefaultConfig()
	if config.Storage.TrackerWarnPercent != 90 || config.Limits.ProjectTaskCapacity != 1000 {
		t.Fatal(config)
	}
	if _, err := core.ApplySettings(config, map[string]string{"runtime.kind": "node"}); !errors.Is(err, core.ErrSettings) {
		t.Fatal(err)
	}

	want := core.RecordEnvelope{Schema: 1, Project: "p", RunID: "R-1", WrittenAt: "2026-09-19T00:00:00Z", Revision: 1}
	raw, err := json.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	var got core.RecordEnvelope
	if err := json.Unmarshal(raw, &got); err != nil || got != want {
		t.Fatalf("got=%+v err=%v", got, err)
	}
}
