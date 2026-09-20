package capability

import (
	"context"
	"fmt"

	"github.com/thebpandey/agent-team/vnext/internal/core"
)

type Question struct {
	Task                   core.TaskID
	Worktree, Revision     string
	Prompt, SecondQuestion string
	Files                  []string
	Skills                 []core.SkillRef
}

type Result struct {
	Name                      Name
	Version, Worktree         string
	Revision                  string
	Skills                    []core.SkillRef
	Used, Fallback            bool
	Summary, RawOutputPointer string
	DurationMillis            int64
	TokensBefore, TokensAfter int
}

type Route struct {
	Primary        Name
	Secondary      *Name
	Reason         string
	NativeFallback []string
}

type ResultWriter interface {
	WriteCapabilityResult(context.Context, Result) error
}

type Router interface {
	Select(Question, []Probe) (Route, error)
	Execute(context.Context, Route, Question) (Result, error)
}

type router struct {
	runner NativeRunner
	output OutputStore
	tokens TokenSource
	writer ResultWriter
}

func NewRouter(runner NativeRunner, output OutputStore, tokens TokenSource, writer ResultWriter) Router {
	return &router{runner: runner, output: output, tokens: tokens, writer: writer}
}

func (r *router) Select(q Question, probes []Probe) (Route, error) {
	if q.Task == "" || q.Prompt == "" {
		return Route{}, fmt.Errorf("capability question requires task and prompt")
	}
	primary := Native
	reason := "native fallback"
	for _, probe := range probes {
		if analytical(probe.Name) && probe.Available && probe.Healthy {
			primary, reason = probe.Name, "healthy task-scoped capability"
			break
		}
	}
	route := Route{Primary: primary, Reason: reason, NativeFallback: []string{"git", "rg", "project-checks"}}
	if q.SecondQuestion != "" {
		secondary := Native
		route.Secondary = &secondary
	}
	return route, nil
}

func analytical(name Name) bool {
	return name == LeanCTX || name == Serena || name == Graphify || name == AstGrep
}

func (r *router) Execute(ctx context.Context, route Route, q Question) (Result, error) {
	if ctx == nil || r == nil {
		return Result{}, fmt.Errorf("capability execution requires context and router")
	}
	var result Result
	var err error
	switch route.Primary {
	case LeanCTX:
		result, err = RunLeanCTX(ctx, r.runner, r.output, r.tokens, q)
	case Graphify:
		result, err = RunGraphify(ctx, r.runner, r.output, q)
	case Native:
		result = Result{Name: Native, Worktree: q.Worktree, Revision: q.Revision, Skills: append([]core.SkillRef(nil), q.Skills...), Used: true, Fallback: true, Summary: "native fallback"}
	default:
		result = Result{Name: Native, Worktree: q.Worktree, Revision: q.Revision, Skills: append([]core.SkillRef(nil), q.Skills...), Used: true, Fallback: true, Summary: "optional capability requires its bounded host adapter"}
	}
	if err != nil {
		result = Result{Name: Native, Worktree: q.Worktree, Revision: q.Revision, Skills: append([]core.SkillRef(nil), q.Skills...), Used: true, Fallback: true, Summary: "optional capability unavailable; use native fallback"}
	}
	if r.writer != nil {
		if writeErr := r.writer.WriteCapabilityResult(ctx, result); writeErr != nil {
			return result, writeErr
		}
	}
	return result, nil
}
