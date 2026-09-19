package tracker

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/thebpandey/agent-team/vnext/internal/core"
	"github.com/thebpandey/agent-team/vnext/internal/store"
)

const maxTrackerBytes int64 = 2 << 20

type tasksMD struct {
	path  string
	store *store.Store
	mu    sync.Mutex

	warning string
}

// NewTasksMD returns the TASKS.md authority at path. It does not consult any
// Beads data; selecting Beads is a separate, explicit operation.
func NewTasksMD(path string, st *store.Store) Tracker {
	return &tasksMD{path: path, store: st}
}

func (t *tasksMD) Warning() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.warning
}

func (t *tasksMD) Page(ctx context.Context, cursor string, limit int) (core.TrackerPage, error) {
	if err := ctx.Err(); err != nil {
		return core.TrackerPage{}, err
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	tasks, revision, err := t.snapshot()
	if err != nil {
		return core.TrackerPage{}, err
	}
	page, err := pageFor(tasks, revision, cursor, limit)
	if err == nil {
		t.warning = warningFor(page.TotalNonArchived)
	}
	return page, err
}

func (t *tasksMD) Get(ctx context.Context, id core.TaskID, expected uint64) (core.Task, error) {
	if err := ctx.Err(); err != nil {
		return core.Task{}, err
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	tasks, revision, err := t.snapshot()
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

func (t *tasksMD) Refresh(ctx context.Context, expected uint64) (core.TrackerPage, error) {
	if err := ctx.Err(); err != nil {
		return core.TrackerPage{}, err
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	tasks, revision, err := t.snapshot()
	if err != nil {
		return core.TrackerPage{}, err
	}
	if err := requireRevision(expected, revision); err != nil {
		return core.TrackerPage{}, err
	}
	page, err := pageFor(tasks, revision, "", 8)
	if err == nil {
		t.warning = warningFor(page.TotalNonArchived)
	}
	return page, err
}

func (t *tasksMD) Create(ctx context.Context, task core.Task, expected uint64) (core.Task, error) {
	if err := ctx.Err(); err != nil {
		return core.Task{}, err
	}
	if err := validateTask(task); err != nil {
		return core.Task{}, err
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return core.Task{}, err
	}
	tasks, revision, err := t.snapshot()
	if err != nil {
		return core.Task{}, err
	}
	if err := requireRevision(expected, revision); err != nil {
		return core.Task{}, err
	}
	for _, existing := range tasks {
		if existing.ID == task.ID {
			return core.Task{}, fmt.Errorf("%w: duplicate task %q", core.ErrPath, task.ID)
		}
	}
	if !task.Archived && nonArchived(tasks) >= capacity {
		return core.Task{}, fmt.Errorf("%w: creation would exceed %d tasks", core.ErrCapacity, capacity)
	}
	if task.State == "" {
		task.State = core.Ready
	}
	tasks = append(tasks, task)
	if err := ctx.Err(); err != nil {
		return core.Task{}, err
	}
	if err := t.write(renderTasks(tasks)); err != nil {
		return core.Task{}, err
	}
	_, revision, err = t.snapshot()
	if err != nil {
		return core.Task{}, err
	}
	task.Revision = revision
	t.warning = warningFor(nonArchived(tasks))
	return task, nil
}

func (t *tasksMD) Archive(ctx context.Context, id core.TaskID, reason string, expected uint64) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if strings.TrimSpace(reason) == "" {
		return fmt.Errorf("%w: archive reason", core.ErrPath)
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	tasks, revision, err := t.snapshot()
	if err != nil {
		return err
	}
	if err := requireRevision(expected, revision); err != nil {
		return err
	}
	found := false
	for i := range tasks {
		if tasks[i].ID == id {
			tasks[i].Archived = true
			tasks[i].State = core.Archived
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("%w: task %q", core.ErrPath, id)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := t.write(renderTasks(tasks)); err != nil {
		return err
	}
	t.warning = warningFor(nonArchived(tasks))
	return nil
}

func (t *tasksMD) snapshot() ([]core.Task, uint64, error) {
	data, err := readBounded(t.path, t.limit())
	if err != nil {
		return nil, 0, err
	}
	tasks, err := parseTasksMD(data)
	if err != nil {
		return nil, 0, err
	}
	return tasks, trackerRevision(data), nil
}

func (t *tasksMD) limit() int64 {
	if t.store != nil {
		return t.store.Limits.TrackerBytes
	}
	return maxTrackerBytes
}

func (t *tasksMD) write(data []byte) error {
	if int64(len(data)) > t.limit() {
		return fmt.Errorf("%w: TASKS.md exceeds %d bytes", core.ErrLimit, t.limit())
	}
	if t.store == nil {
		return fmt.Errorf("%w: TASKS.md mutation requires a store", core.ErrPath)
	}
	root, err := filepath.Abs(t.store.Root)
	if err != nil {
		return fmt.Errorf("%w: store root: %v", core.ErrPath, err)
	}
	source, err := filepath.Abs(t.path)
	if err != nil {
		return fmt.Errorf("%w: TASKS.md path: %v", core.ErrPath, err)
	}
	relative, err := filepath.Rel(root, source)
	if err != nil || relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return fmt.Errorf("%w: TASKS.md is outside store root", core.ErrPath)
	}
	_, err = t.store.WriteMarkdown(relative, data, t.limit())
	return err
}

func readBounded(path string, limit int64) ([]byte, error) {
	if limit < 1 || limit > maxTrackerBytes {
		return nil, fmt.Errorf("%w: tracker bound must be between 1 and %d", core.ErrLimit, maxTrackerBytes)
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("%w: open TASKS.md: %v", core.ErrPath, err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%w: TASKS.md is not a regular file", core.ErrPath)
	}
	if info.Size() > limit {
		return nil, fmt.Errorf("%w: TASKS.md exceeds %d bytes", core.ErrLimit, limit)
	}
	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		return nil, fmt.Errorf("%w: read TASKS.md: %v", core.ErrPath, err)
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("%w: TASKS.md exceeds %d bytes", core.ErrLimit, limit)
	}
	return data, nil
}

func parseTasksMD(data []byte) ([]core.Task, error) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) > 0 && (trimmed[0] == '[' || trimmed[0] == '{') {
		return parseTasksJSON(trimmed)
	}
	var tasks []core.Task
	var current *core.Task
	section := ""
	seenIDs := map[core.TaskID]bool{}
	seenFields := map[string]bool{}
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 1024), int(maxTrackerBytes))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "<!--") {
			continue
		}
		if strings.HasPrefix(line, "###") {
			return nil, fmt.Errorf("%w: TASKS.md task headings must use exactly ##", core.ErrPath)
		}
		if strings.HasPrefix(line, "##") {
			if !strings.HasPrefix(line, "## ") {
				return nil, fmt.Errorf("%w: malformed TASKS.md task heading", core.ErrPath)
			}
			id := strings.TrimSpace(strings.TrimPrefix(line, "## "))
			if id == "" {
				return nil, fmt.Errorf("%w: TASKS.md task heading has no ID", core.ErrPath)
			}
			if current != nil {
				tasks = append(tasks, *current)
			}
			current = &core.Task{ID: core.TaskID(id), State: core.Ready}
			if err := validateTaskID(current.ID); err != nil {
				return nil, err
			}
			if seenIDs[current.ID] {
				return nil, fmt.Errorf("%w: duplicate task ID %q", core.ErrPath, current.ID)
			}
			seenIDs[current.ID] = true
			seenFields = map[string]bool{}
			section = ""
			continue
		}
		if current == nil {
			if strings.HasPrefix(line, "#") {
				continue
			}
			return nil, fmt.Errorf("%w: TASKS.md data outside task", core.ErrPath)
		}
		if strings.HasPrefix(line, "- ") {
			if err := appendTaskList(current, section, strings.TrimSpace(strings.TrimPrefix(line, "- "))); err != nil {
				return nil, err
			}
			continue
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			return nil, fmt.Errorf("%w: malformed TASKS.md line %q", core.ErrPath, line)
		}
		section = normalizeField(key)
		if seenFields[section] {
			return nil, fmt.Errorf("%w: duplicate TASKS.md field %q", core.ErrPath, key)
		}
		seenFields[section] = true
		value = strings.TrimSpace(value)
		switch section {
		case "objective":
			current.Objective = value
		case "state":
			current.State = core.TaskState(value)
		case "archived":
			current.Archived = strings.EqualFold(value, "true")
		case "dependencies":
			if value != "" {
				current.Dependencies = appendTaskIDs(current.Dependencies, value)
			}
		case "criteria", "checks", "writablepaths", "resources", "evidencepointers":
			if value != "" {
				if err := appendTaskList(current, section, value); err != nil {
					return nil, err
				}
			}
		default:
			return nil, fmt.Errorf("%w: unknown TASKS.md field %q", core.ErrPath, key)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("%w: scan TASKS.md: %v", core.ErrPath, err)
	}
	if current != nil {
		tasks = append(tasks, *current)
	}
	for _, task := range tasks {
		if err := validateTask(task); err != nil {
			return nil, err
		}
	}
	return tasks, nil
}

func normalizeField(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.ReplaceAll(value, " ", "")
	return value
}

func validateTaskID(id core.TaskID) error {
	if strings.TrimSpace(string(id)) == "" || strings.ContainsAny(string(id), "\t\r\n ") {
		return fmt.Errorf("%w: invalid task ID %q", core.ErrPath, id)
	}
	return nil
}

func appendTaskIDs(existing []core.TaskID, value string) []core.TaskID {
	for _, id := range strings.Split(value, ",") {
		id = strings.TrimSpace(id)
		if id != "" {
			existing = append(existing, core.TaskID(id))
		}
	}
	return existing
}

func appendTaskList(task *core.Task, section, value string) error {
	if section == "" {
		return fmt.Errorf("%w: TASKS.md list without field", core.ErrPath)
	}
	switch section {
	case "dependencies":
		task.Dependencies = appendTaskIDs(task.Dependencies, value)
	case "criteria":
		task.Criteria = append(task.Criteria, value)
	case "checks":
		name, command, found := strings.Cut(value, "|")
		check := core.Check{Name: strings.TrimSpace(name)}
		if found {
			check.Command = strings.Fields(strings.TrimSpace(command))
		}
		if check.Name == "" {
			return fmt.Errorf("%w: TASKS.md check without name", core.ErrPath)
		}
		task.Checks = append(task.Checks, check)
	case "writablepaths":
		task.WritablePaths = append(task.WritablePaths, value)
	case "resources":
		task.Resources = append(task.Resources, value)
	case "evidencepointers":
		task.EvidencePointers = append(task.EvidencePointers, value)
	default:
		return fmt.Errorf("%w: unknown TASKS.md list %q", core.ErrPath, section)
	}
	return nil
}

func parseTasksJSON(data []byte) ([]core.Task, error) {
	var list []core.Task
	if err := json.Unmarshal(data, &list); err == nil {
		for _, task := range list {
			if err := validateTask(task); err != nil {
				return nil, err
			}
		}
		return list, nil
	}
	var wrapped struct {
		Tasks  []core.Task `json:"tasks"`
		Issues []core.Task `json:"issues"`
	}
	if err := json.Unmarshal(data, &wrapped); err != nil {
		return nil, fmt.Errorf("%w: malformed tracker JSON: %v", core.ErrPath, err)
	}
	if wrapped.Tasks != nil {
		list = wrapped.Tasks
	} else if wrapped.Issues != nil {
		list = wrapped.Issues
	} else {
		return nil, fmt.Errorf("%w: tracker JSON has no tasks", core.ErrPath)
	}
	for _, task := range list {
		if err := validateTask(task); err != nil {
			return nil, err
		}
	}
	return list, nil
}

func renderTasks(tasks []core.Task) []byte {
	var b strings.Builder
	b.WriteString("# Tasks\n\n")
	for _, task := range tasks {
		fmt.Fprintf(&b, "## %s\nObjective: %s\nState: %s\nArchived: %t\n", task.ID, task.Objective, task.State, task.Archived)
		writeIDs(&b, "Dependencies", task.Dependencies)
		writeStrings(&b, "Criteria", task.Criteria)
		b.WriteString("Checks:\n")
		for _, check := range task.Checks {
			fmt.Fprintf(&b, "- %s | %s\n", check.Name, strings.Join(check.Command, " "))
		}
		writeStrings(&b, "Writable paths", task.WritablePaths)
		writeStrings(&b, "Resources", task.Resources)
		writeStrings(&b, "Evidence pointers", task.EvidencePointers)
		b.WriteByte('\n')
	}
	return []byte(b.String())
}

func writeIDs(b *strings.Builder, label string, values []core.TaskID) {
	b.WriteString(label + ":\n")
	for _, value := range values {
		fmt.Fprintf(b, "- %s\n", value)
	}
}

func writeStrings(b *strings.Builder, label string, values []string) {
	b.WriteString(label + ":\n")
	for _, value := range values {
		fmt.Fprintf(b, "- %s\n", value)
	}
}

func nonArchived(tasks []core.Task) int {
	count := 0
	for _, task := range tasks {
		if !task.Archived {
			count++
		}
	}
	return count
}
