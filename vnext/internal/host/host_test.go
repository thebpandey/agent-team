package host_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/thebpandey/agent-team/vnext/internal/core"
	"github.com/thebpandey/agent-team/vnext/internal/host"
	"github.com/thebpandey/agent-team/vnext/internal/model"
	"github.com/thebpandey/agent-team/vnext/internal/tracker"
)

func TestConcreteAdaptersUseExactArgumentArrays(t *testing.T) {
	for _, tc := range []struct {
		name string
		new  func(host.CommandRunner) host.Adapter
	}{
		{name: "codex", new: host.NewCodex},
		{name: "claude", new: host.NewClaude},
	} {
		t.Run(tc.name, func(t *testing.T) {
			runner := &recordingRunner{result: tracker.CommandResult{Stdout: []byte("available")}}
			adapter := tc.new(runner)
			req := validRequest()

			capabilities, err := adapter.Probe(context.Background())
			if err != nil || capabilities.Host != tc.name || !reflect.DeepEqual(capabilities.Models, []string{"default"}) {
				t.Fatalf("Probe() = %+v, %v", capabilities, err)
			}
			worker, err := adapter.StartWorker(context.Background(), req)
			if err != nil || worker.Identity != tc.name+":worker" || worker.Reviewer {
				t.Fatalf("StartWorker() = %+v, %v", worker, err)
			}
			reviewer, err := adapter.StartReviewer(context.Background(), req, worker)
			if err != nil || reviewer.Identity != tc.name+":review" || !reviewer.Reviewer || reviewer.Identity == worker.Identity {
				t.Fatalf("StartReviewer() = %+v, %v", reviewer, err)
			}
			if _, err := adapter.Poll(context.Background(), worker); err != nil {
				t.Fatalf("Poll() error = %v", err)
			}
			if err := adapter.Stop(context.Background(), worker, core.Scope{Kind: core.ScopeTask, ID: "TASK"}); err != nil {
				t.Fatalf("Stop() error = %v", err)
			}
			identity, err := adapter.ReadIdentity(context.Background(), worker)
			if err != nil || identity != worker.Identity {
				t.Fatalf("ReadIdentity() = %q, %v", identity, err)
			}

			want := [][]string{
				{tc.name, "--version"},
				{tc.name, "worker", "--run", "RUN", "--team", "TEAM", "--task", "TASK", "--worktree", "/tmp/task"},
				{tc.name, "review", "--run", "RUN", "--team", "TEAM", "--task", "TASK", "--worktree", "/tmp/task"},
				{tc.name, "poll", "--identity", tc.name + ":worker"},
				{tc.name, "stop", "--identity", tc.name + ":worker"},
			}
			if !reflect.DeepEqual(runner.calls, want) {
				t.Fatalf("commands = %#v, want %#v", runner.calls, want)
			}
		})
	}
}

func TestAdapterRejectsInvalidInputsAndIdentityMismatch(t *testing.T) {
	runner := &recordingRunner{result: tracker.CommandResult{Stdout: []byte("available")}}
	adapter := host.NewCodex(runner)
	for _, req := range []host.WorkerRequest{
		{},
		{Packet: validRequest().Packet},
		{Packet: validRequest().Packet, Worktree: host.WorktreeSpec{Run: "OTHER", Team: "TEAM", Root: "/tmp/task"}},
	} {
		if _, err := adapter.StartWorker(context.Background(), req); !errors.Is(err, core.ErrPath) {
			t.Fatalf("StartWorker(%+v) error = %v, want ErrPath", req, err)
		}
	}
	req := validRequest()
	author := host.WorkerHandle{Host: "codex", Identity: "codex:review", Run: "RUN", Team: "TEAM", Task: "TASK", PacketDigest: "digest"}
	if _, err := adapter.StartReviewer(context.Background(), req, author); !errors.Is(err, core.ErrRevision) {
		t.Fatalf("StartReviewer(same identity) error = %v, want ErrRevision", err)
	}
	author.Identity = "different:worker"
	if _, err := adapter.StartReviewer(context.Background(), req, author); !errors.Is(err, core.ErrRevision) {
		t.Fatalf("StartReviewer(foreign host) error = %v, want ErrRevision", err)
	}
	author = host.WorkerHandle{Host: "codex", Identity: "codex:worker", Reviewer: true, Run: "RUN", Team: "TEAM", Task: "TASK", PacketDigest: "digest"}
	if _, err := adapter.StartReviewer(context.Background(), req, author); !errors.Is(err, core.ErrRevision) {
		t.Fatalf("StartReviewer(role mismatch) error = %v, want ErrRevision", err)
	}
	if _, err := adapter.Poll(context.Background(), host.WorkerHandle{}); !errors.Is(err, core.ErrPath) {
		t.Fatalf("Poll(empty) error = %v", err)
	}
	if err := adapter.Stop(context.Background(), host.WorkerHandle{Identity: "claude:worker"}, core.Scope{}); !errors.Is(err, core.ErrRevision) {
		t.Fatalf("Stop(foreign) error = %v", err)
	}
	if _, err := adapter.ReadIdentity(context.Background(), host.WorkerHandle{Identity: "claude:worker"}); !errors.Is(err, core.ErrRevision) {
		t.Fatalf("ReadIdentity(foreign) error = %v", err)
	}
	mismatched := host.WorkerHandle{Host: "codex", Identity: "codex:worker", Reviewer: true}
	if _, err := adapter.ReadIdentity(context.Background(), mismatched); !errors.Is(err, core.ErrRevision) {
		t.Fatalf("ReadIdentity(role mismatch) error = %v", err)
	}
}

func TestAdapterMapsRunnerFailuresToTypedCapacity(t *testing.T) {
	for _, result := range []tracker.CommandResult{
		{Transport: errors.New("unavailable"), Exit: -1},
		{TimedOut: true, Exit: -1},
		{Exit: 3},
	} {
		adapter := host.NewClaude(&recordingRunner{result: result})
		if _, err := adapter.Probe(context.Background()); !errors.Is(err, core.ErrCapacity) {
			t.Fatalf("Probe(%+v) error = %v, want ErrCapacity", result, err)
		}
		if _, err := adapter.StartWorker(context.Background(), validRequest()); !errors.Is(err, core.ErrCapacity) {
			t.Fatalf("StartWorker(%+v) error = %v, want ErrCapacity", result, err)
		}
	}
	blank := host.NewClaude(&recordingRunner{result: tracker.CommandResult{Stdout: []byte(" \n")}})
	if _, err := blank.Probe(context.Background()); !errors.Is(err, core.ErrCapacity) {
		t.Fatalf("Probe(blank version) error = %v, want ErrCapacity", err)
	}
}

func TestRouteModelRequiresExactCapability(t *testing.T) {
	got, err := model.RouteModel(host.Capabilities{Models: []string{"small", "large"}}, model.Codex, "small", true)
	if err != nil || got.Harness != model.Codex || got.Requested != "small" || got.Resolved != "small" || !got.Reviewer {
		t.Fatalf("RouteModel() = %+v, %v", got, err)
	}
	for _, requested := range []string{"", "Small", "missing"} {
		if _, err := model.RouteModel(host.Capabilities{Models: []string{"small"}}, model.Claude, requested, false); !errors.Is(err, core.ErrCapacity) {
			t.Fatalf("RouteModel(%q) error = %v, want ErrCapacity", requested, err)
		}
	}
}

func validRequest() host.WorkerRequest {
	return host.WorkerRequest{
		Packet: host.AssignmentPacket{
			RecordEnvelope:   core.RecordEnvelope{RunID: "RUN"},
			Team:             "TEAM",
			Task:             "TASK",
			QueueFingerprint: "digest",
			SpecRevision:     "revision",
		},
		Worktree: host.WorktreeSpec{Run: "RUN", Team: "TEAM", Root: "/tmp/task"},
	}
}

type recordingRunner struct {
	result tracker.CommandResult
	calls  [][]string
}

func (r *recordingRunner) Run(_ context.Context, name string, args ...string) tracker.CommandResult {
	r.calls = append(r.calls, append([]string{name}, args...))
	return r.result
}
