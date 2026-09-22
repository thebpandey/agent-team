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

// AuthorityMetadata identifies the project-local Beads authority. It is only
// descriptive and deliberately does not invoke bd or touch tracker state.
func (b *beads) AuthorityMetadata() AuthorityMetadata {
	return AuthorityMetadata{Kind: "beads", Ref: ".beads"}
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
	if task.State == "" {
		task.State = core.Ready
	}
	if task.State != core.Ready {
		return core.Task{}, fmt.Errorf("%w: Beads create only supports ready tasks", core.ErrSettings)
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return core.Task{}, err
	}
	tasks, revision, err := b.snapshot(ctx)
	if err != nil {
		return core.Task{}, err
	}
	if err := requireRevision(expected, revision); err != nil {
		return core.Task{}, err
	}
	if err := ctx.Err(); err != nil {
		return core.Task{}, err
	}
	for _, existing := range tasks {
		if existing.ID == task.ID {
			if sameTask(existing, task) {
				return existing, nil
			}
			return core.Task{}, fmt.Errorf("%w: conflicting task ID %q", core.ErrPath, task.ID)
		}
	}
	if !task.Archived && nonArchived(tasks) >= capacity {
		return core.Task{}, fmt.Errorf("%w: creation would exceed %d tasks", core.ErrCapacity, capacity)
	}
	metadata, err := json.Marshal(struct {
		Criteria         []string     `json:"criteria"`
		Checks           []core.Check `json:"checks"`
		WritablePaths    []string     `json:"writablePaths"`
		Resources        []string     `json:"resources"`
		EvidencePointers []string     `json:"evidencePointers"`
	}{task.Criteria, task.Checks, task.WritablePaths, task.Resources, task.EvidencePointers})
	if err != nil {
		return core.Task{}, fmt.Errorf("%w: encode Beads metadata: %v", core.ErrPath, err)
	}
	args := []string{"create", "--id", string(task.ID), "--title", task.Objective, "--description", task.Objective, "--type", "task", "--metadata", string(metadata), "--json"}
	if len(task.Dependencies) > 0 {
		deps := make([]string, len(task.Dependencies))
		for i, dependency := range task.Dependencies {
			deps[i] = string(dependency)
		}
		args = append(args, "--deps", strings.Join(deps, ","))
	}
	if err := ctx.Err(); err != nil {
		return core.Task{}, err
	}
	if err := commandResultError(b.runner.Run(ctx, "bd", args...)); err != nil {
		return core.Task{}, err
	}
	updated, _, err := b.snapshot(ctx)
	if err != nil {
		return core.Task{}, err
	}
	for _, created := range updated {
		if created.ID == task.ID {
			if !sameTask(created, task) {
				return core.Task{}, fmt.Errorf("%w: Beads did not preserve task %q metadata", core.ErrPath, task.ID)
			}
			return created, nil
		}
	}
	return core.Task{}, fmt.Errorf("%w: created Beads task %q was not found", core.ErrPath, task.ID)
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
	if err := ctx.Err(); err != nil {
		return err
	}
	tasks, revision, err := b.snapshot(ctx)
	if err != nil {
		return err
	}
	if err := requireRevision(expected, revision); err != nil {
		return err
	}
	for _, task := range tasks {
		if task.ID == id && task.Archived {
			return nil
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return commandResultError(b.runner.Run(ctx, "bd", "close", string(id), "--reason", reason))
}

func (b *beads) snapshot(ctx context.Context) ([]core.Task, uint64, error) {
	result := b.runner.Run(ctx, "bd", "list", "--json", "--all", "--limit", "0")
	if err := commandResultError(result); err != nil {
		return nil, 0, err
	}
	tasks, err := parseBeads(result.Stdout)
	if err != nil {
		return nil, 0, err
	}
	revision := trackerRevision(result.Stdout)
	for index := range tasks {
		tasks[index].Revision = revision
	}
	return tasks, revision, nil
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
	ID               core.TaskID     `json:"id"`
	Objective        string          `json:"objective"`
	Title            string          `json:"title"`
	Description      string          `json:"description"`
	State            core.TaskState  `json:"state"`
	Status           string          `json:"status"`
	Dependencies     []core.TaskID   `json:"dependencies"`
	DependencyIDs    []core.TaskID   `json:"dependency_ids"`
	Criteria         []string        `json:"criteria"`
	Checks           []core.Check    `json:"checks"`
	WritablePaths    []string        `json:"writablePaths"`
	Resources        []string        `json:"resources"`
	EvidencePointers []string        `json:"evidencePointers"`
	Metadata         json.RawMessage `json:"metadata"`
	Archived         bool            `json:"archived"`
}

func parseBeads(data []byte) ([]core.Task, error) {
	data = bytes.TrimSpace(data)
	if len(data) == 0 || bytes.Equal(data, []byte("null")) {
		return nil, fmt.Errorf("%w: null Beads result", core.ErrPath)
	}
	var raw []json.RawMessage
	if data[0] == '[' {
		if err := json.Unmarshal(data, &raw); err != nil || raw == nil {
			return nil, fmt.Errorf("%w: malformed Beads array", core.ErrPath)
		}
	} else {
		var wrapper map[string]json.RawMessage
		if err := json.Unmarshal(data, &wrapper); err != nil || wrapper == nil {
			return nil, fmt.Errorf("%w: malformed Beads JSON", core.ErrPath)
		}
		if item, ok := wrapper["id"]; ok {
			raw = []json.RawMessage{data}
			_ = item
		} else if items, ok := wrapper["issues"]; ok && !bytes.Equal(items, []byte("null")) {
			if err := json.Unmarshal(items, &raw); err != nil || raw == nil {
				return nil, fmt.Errorf("%w: malformed Beads issues", core.ErrPath)
			}
		} else if items, ok := wrapper["tasks"]; ok && !bytes.Equal(items, []byte("null")) {
			if err := json.Unmarshal(items, &raw); err != nil || raw == nil {
				return nil, fmt.Errorf("%w: malformed Beads tasks", core.ErrPath)
			}
		} else {
			return nil, fmt.Errorf("%w: Beads JSON has no issues", core.ErrPath)
		}
	}
	tasks := make([]core.Task, 0, len(raw))
	seen := map[core.TaskID]bool{}
	for _, encoded := range raw {
		if bytes.Equal(bytes.TrimSpace(encoded), []byte("null")) {
			return nil, fmt.Errorf("%w: null Beads issue", core.ErrPath)
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(encoded, &fields); err != nil || fields == nil {
			return nil, fmt.Errorf("%w: malformed Beads issue", core.ErrPath)
		}
		allowed := map[string]bool{
			"id": true, "objective": true, "title": true, "description": true, "state": true, "status": true,
			"dependencies": true, "dependency_ids": true, "criteria": true, "checks": true, "writablePaths": true,
			"resources": true, "evidencePointers": true, "metadata": true, "archived": true,
			// These are observational fields emitted by `bd list --json`; they do not
			// carry task authority and are deliberately ignored after type validation.
			"priority": true, "issue_type": true, "owner": true, "created_at": true, "created_by": true,
			"updated_at": true, "dependency_count": true, "dependent_count": true, "comment_count": true,
			"acceptance_criteria": true, "notes": true, "assignee": true, "started_at": true, "labels": true,
			"parent": true,
		}
		for key, value := range fields {
			if !allowed[key] || (bytes.Equal(bytes.TrimSpace(value), []byte("null")) && !beadsObservationalField(key)) {
				return nil, fmt.Errorf("%w: invalid Beads field %q", core.ErrPath, key)
			}
		}
		if _, ok := fields["id"]; !ok {
			return nil, fmt.Errorf("%w: Beads issue has no ID", core.ErrPath)
		}
		if _, ok := fields["status"]; !ok {
			return nil, fmt.Errorf("%w: Beads issue has no status", core.ErrPath)
		}
		dependencies, err := parseBeadsDependencies(fields["dependencies"])
		if err != nil {
			return nil, err
		}
		if fields["dependencies"] != nil {
			delete(fields, "dependencies")
			encoded, err = json.Marshal(fields)
			if err != nil {
				return nil, fmt.Errorf("%w: normalize Beads dependencies", core.ErrPath)
			}
		}
		var item beadTask
		if err := json.Unmarshal(encoded, &item); err != nil {
			return nil, fmt.Errorf("%w: malformed Beads issue", core.ErrPath)
		}
		item.Dependencies = dependencies
		objective := item.Objective
		if objective == "" {
			objective = item.Title
		}
		if objective == "" {
			objective = item.Description
		}
		state, archived, ok := beadsState(item.Status)
		if !ok {
			return nil, fmt.Errorf("%w: unknown Beads status %q", core.ErrPath, item.Status)
		}
		if item.State != "" {
			declared, declaredArchived, valid := beadsState(string(item.State))
			if !valid || declared != state || declaredArchived != archived {
				return nil, fmt.Errorf("%w: contradictory Beads state %q", core.ErrPath, item.State)
			}
		}
		if _, present := fields["archived"]; present && item.Archived != archived {
			return nil, fmt.Errorf("%w: contradictory Beads archived flag", core.ErrPath)
		}
		taskDependencies := item.Dependencies
		if taskDependencies == nil {
			taskDependencies = item.DependencyIDs
		}
		if len(item.Metadata) > 0 {
			var metadataFields map[string]json.RawMessage
			if err := json.Unmarshal(item.Metadata, &metadataFields); err != nil || metadataFields == nil {
				return nil, fmt.Errorf("%w: malformed Beads metadata", core.ErrPath)
			}
			allowedMetadata := map[string]bool{"criteria": true, "checks": true, "writablePaths": true, "resources": true, "evidencePointers": true}
			for key, value := range metadataFields {
				if !allowedMetadata[key] || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
					return nil, fmt.Errorf("%w: invalid Beads metadata field %q", core.ErrPath, key)
				}
			}
			var metadata struct {
				Criteria         []string     `json:"criteria"`
				Checks           []core.Check `json:"checks"`
				WritablePaths    []string     `json:"writablePaths"`
				Resources        []string     `json:"resources"`
				EvidencePointers []string     `json:"evidencePointers"`
			}
			if err := json.Unmarshal(item.Metadata, &metadata); err != nil {
				return nil, fmt.Errorf("%w: malformed Beads metadata", core.ErrPath)
			}
			if item.Criteria == nil {
				item.Criteria = metadata.Criteria
			}
			if item.Checks == nil {
				item.Checks = metadata.Checks
			}
			if item.WritablePaths == nil {
				item.WritablePaths = metadata.WritablePaths
			}
			if item.Resources == nil {
				item.Resources = metadata.Resources
			}
			if item.EvidencePointers == nil {
				item.EvidencePointers = metadata.EvidencePointers
			}
		}
		task := core.Task{ID: item.ID, Objective: objective, State: state, Dependencies: taskDependencies, Criteria: item.Criteria, Checks: item.Checks, WritablePaths: item.WritablePaths, Resources: item.Resources, EvidencePointers: item.EvidencePointers, Archived: archived}
		if err := validateTask(task); err != nil {
			return nil, err
		}
		if seen[task.ID] {
			return nil, fmt.Errorf("%w: duplicate Beads ID %q", core.ErrPath, task.ID)
		}
		seen[task.ID] = true
		tasks = append(tasks, task)
	}
	return tasks, nil
}

// parseBeadsDependencies accepts Beads' compact ID list and its documented
// expanded dependency records. Only blocking dependencies carry scheduling
// authority; other relation types are rejected instead of silently projected.
func parseBeadsDependencies(value json.RawMessage) ([]core.TaskID, error) {
	if value == nil {
		return nil, nil
	}
	var raw []json.RawMessage
	if err := json.Unmarshal(value, &raw); err != nil || raw == nil {
		return nil, fmt.Errorf("%w: malformed Beads dependencies", core.ErrPath)
	}
	result := make([]core.TaskID, 0, len(raw))
	for _, encoded := range raw {
		var id core.TaskID
		if err := json.Unmarshal(encoded, &id); err == nil {
			result = append(result, id)
			continue
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(encoded, &fields); err != nil || fields == nil {
			return nil, fmt.Errorf("%w: malformed Beads dependency", core.ErrPath)
		}
		allowed := map[string]bool{
			"id": true, "objective": true, "title": true, "description": true, "state": true, "status": true,
			"metadata": true, "priority": true, "issue_type": true, "owner": true, "created_at": true,
			"created_by": true, "updated_at": true, "dependency_type": true, "issue_id": true,
			"depends_on_id": true, "type": true,
		}
		for key, field := range fields {
			if !allowed[key] || bytes.Equal(bytes.TrimSpace(field), []byte("null")) {
				return nil, fmt.Errorf("%w: invalid Beads dependency field %q", core.ErrPath, key)
			}
		}
		var relation struct {
			ID          core.TaskID `json:"id"`
			DependsOnID core.TaskID `json:"depends_on_id"`
			Type        string      `json:"type"`
			LegacyType  string      `json:"dependency_type"`
		}
		if err := json.Unmarshal(encoded, &relation); err != nil {
			return nil, fmt.Errorf("%w: malformed Beads dependency relation", core.ErrPath)
		}
		kind := relation.Type
		if kind == "" {
			kind = relation.LegacyType
		}
		switch kind {
		case "blocks":
			id := relation.ID
			if relation.DependsOnID != "" {
				id = relation.DependsOnID
			}
			if id == "" {
				return nil, fmt.Errorf("%w: invalid Beads blocking dependency", core.ErrPath)
			}
			result = append(result, id)
		case "parent-child", "related":
			// Provenance edges are not scheduling blockers.
		default:
			return nil, fmt.Errorf("%w: unknown Beads dependency relation %q", core.ErrPath, kind)
		}
	}
	return result, nil
}

func beadsObservationalField(key string) bool {
	switch key {
	case "priority", "issue_type", "owner", "created_at", "created_by", "updated_at", "dependency_count", "dependent_count", "comment_count", "acceptance_criteria", "notes", "assignee", "started_at", "labels", "parent":
		return true
	default:
		return false
	}
}

func beadsState(status string) (core.TaskState, bool, bool) {
	switch status {
	case "open", "ready":
		return core.Ready, false, true
	case "in_progress", "working":
		return core.Working, false, true
	case "implementing":
		return core.Implementing, false, true
	case "reviewing":
		return core.Reviewing, false, true
	case "blocked":
		return core.Blocked, false, true
	case "paused":
		return core.Paused, false, true
	case "closed", "archived":
		return core.Archived, true, true
	default:
		return "", false, false
	}
}

func sameTask(a, b core.Task) bool {
	a.RecordEnvelope, b.RecordEnvelope = core.RecordEnvelope{}, core.RecordEnvelope{}
	left, _ := json.Marshal(a)
	right, _ := json.Marshal(b)
	return bytes.Equal(left, right)
}
