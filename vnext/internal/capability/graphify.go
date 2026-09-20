package capability

import (
	"context"
	"fmt"
	"time"

	"github.com/thebpandey/agent-team/vnext/internal/core"
)

func RunGraphify(ctx context.Context, runner NativeRunner, output OutputStore, q Question) (Result, error) {
	if ctx == nil || runner == nil || output == nil || q.Task == "" || q.Worktree == "" || q.Revision == "" || q.Prompt == "" {
		return Result{}, fmt.Errorf("invalid graphify request")
	}
	start := time.Now()
	callCtx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	extract := runner.Run(callCtx, []string{"graphify", "extract", "--code-only", "--no-viz", "--worktree", q.Worktree, "--revision", q.Revision}, []string{})
	if failed(extract) {
		return Result{}, fmt.Errorf("graphify extract failed: %s", commandReason(extract))
	}
	explain := runner.Run(callCtx, []string{"graphify", "explain", "--worktree", q.Worktree, "--revision", q.Revision, "--question", q.Prompt}, []string{})
	if failed(explain) || len(explain.Stdout)+len(explain.Stderr) > adapterOutputLimit {
		return Result{}, fmt.Errorf("graphify explain failed: %s", commandReason(explain))
	}
	pointer, err := output.Write(ctx, "graphify-raw", append(append([]byte(nil), explain.Stdout...), explain.Stderr...), adapterOutputLimit)
	if err != nil {
		return Result{}, err
	}
	return Result{Name: Graphify, Worktree: q.Worktree, Revision: q.Revision, Skills: append([]core.SkillRef(nil), q.Skills...), Used: true, Summary: "graphify code-only query completed", RawOutputPointer: pointer, DurationMillis: time.Since(start).Milliseconds()}, nil
}
