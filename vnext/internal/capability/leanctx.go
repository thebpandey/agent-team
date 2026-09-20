package capability

import (
	"context"
	"fmt"
	"time"

	"github.com/thebpandey/agent-team/vnext/internal/core"
)

const adapterOutputLimit = 1 << 20

func RunLeanCTX(ctx context.Context, runner NativeRunner, output OutputStore, tokens TokenSource, q Question) (Result, error) {
	if ctx == nil || runner == nil || output == nil || tokens == nil || q.Task == "" || q.Worktree == "" || q.Revision == "" || q.Prompt == "" {
		return Result{}, fmt.Errorf("invalid leanctx request")
	}
	before, err := tokens.Read(ctx, "before")
	if err != nil {
		return Result{}, err
	}
	start := time.Now()
	callCtx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	call := runner.Run(callCtx, []string{"leanctx", "compress", "--worktree", q.Worktree, "--revision", q.Revision, "--prompt", q.Prompt}, []string{})
	if failed(call) || len(call.Stdout)+len(call.Stderr) > adapterOutputLimit {
		return Result{}, fmt.Errorf("leanctx failed: %s", commandReason(call))
	}
	pointer, err := output.Write(ctx, "leanctx-raw", append(append([]byte(nil), call.Stdout...), call.Stderr...), adapterOutputLimit)
	if err != nil {
		return Result{}, err
	}
	after, err := tokens.Read(ctx, "after")
	if err != nil {
		return Result{}, err
	}
	return Result{Name: LeanCTX, Worktree: q.Worktree, Revision: q.Revision, Skills: append([]core.SkillRef(nil), q.Skills...), Used: true, Summary: "leanctx compression completed", RawOutputPointer: pointer, DurationMillis: time.Since(start).Milliseconds(), TokensBefore: before, TokensAfter: after}, nil
}
