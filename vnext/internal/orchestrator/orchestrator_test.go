package orchestrator_test

import (
	"context"
	"errors"
	"testing"

	"github.com/thebpandey/agent-team/vnext/internal/core"
	"github.com/thebpandey/agent-team/vnext/internal/orchestrator"
	"github.com/thebpandey/agent-team/vnext/internal/store"
)

func TestForegroundControlFailsClosedWithoutCanonicalRun(t *testing.T) {
	o := orchestrator.New(store.New(t.TempDir(), core.StorageLimits{CanonicalBytes: 16 << 20}), nil, nil, nil, nil, nil, nil)
	if err := o.Start(context.Background(), "R1"); !errors.Is(err, core.ErrRevision) {
		t.Fatal(err)
	}
	if err := o.Execute(context.Background(), "../bad"); !errors.Is(err, core.ErrRevision) {
		t.Fatal(err)
	}
}
