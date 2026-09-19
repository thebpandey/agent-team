package tracker

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/thebpandey/agent-team/vnext/internal/core"
)

type commandFailureKind string

const (
	commandTimedOut  commandFailureKind = "timed_out"
	commandExited    commandFailureKind = "exit"
	commandTransport commandFailureKind = "transport"
)

// CommandFailure distinguishes a timed-out command, a normal non-zero exit,
// and a failure to launch or transport the command.
type CommandFailure struct {
	Kind commandFailureKind
	Exit int
	Err  error
}

func (e *CommandFailure) Error() string {
	switch e.Kind {
	case commandTimedOut:
		return "beads command timed out or was cancelled"
	case commandExited:
		return fmt.Sprintf("beads command exited with status %d", e.Exit)
	default:
		return fmt.Sprintf("beads command transport failed: %v", e.Err)
	}
}

func (e *CommandFailure) Unwrap() error { return e.Err }

type beads struct {
	runner  CommandRunner
	mu      sync.Mutex
	warning string
}

// NewBeads returns the Beads authority. It never reads TASKS.md, so callers
// cannot accidentally combine two live trackers in one adapter.
func NewBeads(runner CommandRunner) Tracker {
	if runner == nil {
		runner = commandRunner{}
	}
	return &beads{runner: runner}
}

func (b *beads) Warning() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.warning
}

func (b *beads) Page(ctx context.Context, cursor string, limit int) (core.TrackerPage, error) {
	if err := ctx.Err(); err != nil {
		return core.TrackerPage{}, err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	tasks, revision, err := b.snapshot(ctx)
	if err != nil {
		return core.TrackerPage{}, err
	}
	page, err := pageFor(tasks, revision, cursor, limit)
	if err == nil {
		b.warning = warningFor(page.TotalNonArchived)
	}
	return page, err
}

func (b *beads) Get(ctx context.Context, id core.TaskID, expected uint64) (core.Task, error) {
	if err := ctx.Err(); err != nil {
		return core.Task{}, err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	tasks, revision, err := b.snapshot(ctx)
	if err != nil {
		return core.Task{}, err
	}
	if err := requireRevision(expected, revision); err != nil {
		return core.Task{}, err
	}
	for _, task := range tasks {
		if task.ID == id {
			return task, nil
		}
	}
	return core.Task{}, fmt.Errorf("%w: task %q", core.ErrPath, id)
}

func (b *beads) Refresh(ctx context.Context, expected uint64) (core.TrackerPage, error) {
	if err := ctx.Err(); err != nil {
		return core.TrackerPage{}, err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	tasks, revision, err := b.snapshot(ctx)
	if err != nil {
		return core.TrackerPage{}, err
	}
	if err := requireRevision(expected, revision); err != nil {
		return core.TrackerPage{}, err
	}
	page, err := pageFor(tasks, revision, "", 8)
	if err == nil {
		b.warning = warningFor(page.TotalNonArchived)
	}
	return page, err
}

func (b *beads) Create(ctx context.Context, task core.Task, expected uint64) (core.Task, error) {
	if err := ctx.Err(); err != nil {
		return core.Task{}, err
	}
	if err := validateTask(task); err != nil {
		return core.Task{}, err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	tasks, revision, err := b.snapshot(ctx)
	if err != nil {
		return core.Task{}, err
	}
	if err := requireRevision(expected, revision); err != nil {
		return core.Task{}, err
	}
	if !task.Archived && nonArchived(tasks) >= capacity {
		return core.Task{}, fmt.Errorf("%w: creation would exceed %d tasks", core.ErrCapacity, capacity)
	}
	if result := b.runner.Run(ctx, "bd", "create", "--title", task.Objective, "--json"); commandResultError(result) != nil {
		return core.Task{}, commandResultError(result)
	}
	return task, nil
}

func (b *beads) Archive(ctx context.Context, id core.TaskID, reason string, expected uint64) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if strings.TrimSpace(string(id)) == "" || strings.TrimSpace(reason) == "" {
		return fmt.Errorf("%w: archive task and reason are required", core.ErrPath)
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	_, revision, err := b.snapshot(ctx)
	if err != nil {
		return err
	}
	if err := requireRevision(expected, revision); err != nil {
		return err
	}
	return commandResultError(b.runner.Run(ctx, "bd", "close", string(id), "--reason", reason))
}

func (b *beads) snapshot(ctx context.Context) ([]core.Task, uint64, error) {
	result := b.runner.Run(ctx, "bd", "list", "--json")
	if err := commandResultError(result); err != nil {
		return nil, 0, err
	}
	tasks, err := parseBeads(result.Stdout)
	if err != nil {
		return nil, 0, err
	}
	return tasks, trackerRevision(result.Stdout), nil
}

func commandResultError(result CommandResult) error {
	if result.TimedOut {
		return &CommandFailure{Kind: commandTimedOut}
	}
	if result.Transport != nil {
		return &CommandFailure{Kind: commandTransport, Err: errors.Join(core.ErrPath, result.Transport)}
	}
	if result.Exit != 0 {
		return &CommandFailure{Kind: commandExited, Exit: result.Exit}
	}
	return nil
}

type beadTask struct {
	ID               core.TaskID    `json:"id"`
	Objective        string         `json:"objective"`
	Title            string         `json:"title"`
	Description      string         `json:"description"`
	State            core.TaskState `json:"state"`
	Status           string         `json:"status"`
	Dependencies     []core.TaskID  `json:"dependencies"`
	DependencyIDs    []core.TaskID  `json:"dependency_ids"`
	Criteria         []string       `json:"criteria"`
	Checks           []core.Check   `json:"checks"`
	WritablePaths    []string       `json:"writablePaths"`
	Resources        []string       `json:"resources"`
	EvidencePointers []string       `json:"evidencePointers"`
	Archived         bool           `json:"archived"`
}

func parseBeads(data []byte) ([]core.Task, error) {
	data = bytes.TrimSpace(data)
	var raw []beadTask
	if err := json.Unmarshal(data, &raw); err != nil {
		var wrapped struct {
			Issues []beadTask `json:"issues"`
			Tasks  []beadTask `json:"tasks"`
		}
		if wrappedErr := json.Unmarshal(data, &wrapped); wrappedErr != nil {
			return nil, fmt.Errorf("%w: malformed Beads JSON: %v", core.ErrPath, err)
		}
		if wrapped.Issues != nil {
			raw = wrapped.Issues
		} else if wrapped.Tasks != nil {
			raw = wrapped.Tasks
		} else {
			return nil, fmt.Errorf("%w: Beads JSON has no issues", core.ErrPath)
		}
	}
	tasks := make([]core.Task, 0, len(raw))
	for _, item := range raw {
		objective := item.Objective
		if objective == "" {
			objective = item.Title
		}
		if objective == "" {
			objective = item.Description
		}
		state := item.State
		if state == "" {
			state = core.TaskState(item.Status)
		}
		archived := item.Archived || state == "closed" || state == "archived"
		dependencies := item.Dependencies
		if dependencies == nil {
			dependencies = item.DependencyIDs
		}
		task := core.Task{ID: item.ID, Objective: objective, State: state, Dependencies: dependencies, Criteria: item.Criteria, Checks: item.Checks, WritablePaths: item.WritablePaths, Resources: item.Resources, EvidencePointers: item.EvidencePointers, Archived: archived}
		if err := validateTask(task); err != nil {
			return nil, err
		}
		tasks = append(tasks, task)
	}
	return tasks, nil
}
