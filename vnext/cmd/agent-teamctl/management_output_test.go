package main

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/thebpandey/agent-team/vnext/internal/cli"
	"github.com/thebpandey/agent-team/vnext/internal/core"
)

func TestManagementTextReportsActualOutcome(t *testing.T) {
	for _, test := range []struct {
		name   string
		result map[string]any
		want   []string
		code   int
	}{
		{"missing input", map[string]any{"ok": true, "status": "needs_input", "message": "Provide approved task details.", "next_action": "provide_task_details", "required_fields": []string{"criteria", "writablePaths"}}, []string{"Provide approved task details.", "Required: criteria, writablePaths", "Next: provide task details"}, 0},
		{"status without message", map[string]any{"ok": true, "status": "setup_required", "next_action": "setup"}, []string{"start: setup required", "Next: setup"}, 0},
		{"fresh reservation", map[string]any{"ok": true, "status": "reserved", "host_dispatch_required": true}, []string{"start: reserved", "Host dispatch required"}, 0},
		{"retained reservation", map[string]any{"ok": true, "host_followup_required": true}, []string{"Host follow-up required"}, 0},
		{"existing reservation", map[string]any{"ok": true, "status": "already_reserved", "observation_required": true}, []string{"start: already reserved", "Observe the existing worker"}, 0},
		{"failure", map[string]any{"ok": false, "error": "Selected tracker is unavailable."}, []string{"Selected tracker is unavailable."}, 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			var out bytes.Buffer
			if code := managementResult([]string{"start"}, &out, test.result); code != test.code {
				t.Fatalf("code=%d output=%s", code, out.String())
			}
			for _, want := range test.want {
				if !strings.Contains(out.String(), want) {
					t.Fatalf("missing %q in %q", want, out.String())
				}
			}
			if strings.Contains(out.String(), "accepted") || strings.Contains(out.String(), "launched") {
				t.Fatalf("text invented execution success: %q", out.String())
			}
		})
	}
}

func TestObjectiveOnlyOneOffTextRequestsDetails(t *testing.T) {
	var out bytes.Buffer
	code := cli.Run(context.Background(), []string{"one-off", "feature", "Add logging"}, core.Dependencies{Stdout: &out, Stderr: &out, Management: runManagement})
	if code != 0 || !strings.Contains(out.String(), "criteria") || !strings.Contains(out.String(), "Next:") || strings.Contains(out.String(), "accepted") {
		t.Fatalf("code=%d text=%q", code, out.String())
	}
}
