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

// AuthorityMetadata identifies the selected TASKS.md file without reading or
// altering it. The run layer verifies this canonical reference against Root.
func (t *tasksMD) AuthorityMetadata() AuthorityMetadata {
	return AuthorityMetadata{Kind: "tasks-md", Ref: filepath.ToSlash(filepath.Clean(t.path))}
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
	if err := t.writeTaskMutation(tasks, revision, &task, ""); err != nil {
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
	if err := t.writeTaskMutation(tasks, revision, nil, id); err != nil {
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
	revision := trackerRevision(data)
	for index := range tasks {
		tasks[index].Revision = revision
	}
	return tasks, revision, nil
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
	if table, found, err := parseKickoffTable(data); found || err != nil {
		return table.tasks, err
	}
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
			if !knownTaskState(current.State) {
				return nil, fmt.Errorf("%w: unknown TASKS.md state %q", core.ErrPath, value)
			}
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

// A Kickoff table is the same selected authority as native TASKS.md. Keep its
// surrounding records and unknown columns intact rather than re-rendering the
// whole document in the native heading format.
type kickoffTable struct {
	lines   []string
	headers map[string]int
	header  int
	rows    map[core.TaskID]int
	tasks   []core.Task
	insert  int
	width   int
}

func parseKickoffTable(data []byte) (kickoffTable, bool, error) {
	table := kickoffTable{lines: strings.Split(string(data), "\n"), headers: map[string]int{}, rows: map[core.TaskID]int{}}
	section := -1
	for i, line := range table.lines {
		if strings.EqualFold(strings.TrimSpace(line), "## Active tasks") {
			if section >= 0 {
				return table, true, fmt.Errorf("%w: multiple Active tasks sections", core.ErrPath)
			}
			section = i
		}
	}
	if section < 0 {
		return table, false, nil
	}
	fail := func(reason string) (kickoffTable, bool, error) {
		return table, true, fmt.Errorf("%w: kickoff task table: %s", core.ErrPath, reason)
	}
	header := section + 1
	for header < len(table.lines) && strings.TrimSpace(table.lines[header]) == "" {
		header++
	}
	if header+1 >= len(table.lines) {
		return fail("missing header")
	}
	fields := kickoffRow(table.lines[header])
	if len(fields) < 3 {
		return fail("missing header")
	}
	table.width = len(fields) - 2
	table.header = header
	for i, raw := range fields[1 : len(fields)-1] {
		key := strings.ToLower(strings.TrimSpace(raw))
		if _, duplicate := table.headers[key]; duplicate {
			return fail("duplicate column")
		}
		table.headers[key] = i + 1
	}
	for _, key := range []string{"id", "intended outcome / acceptance pointer", "status", "depends on"} {
		if _, ok := table.headers[key]; !ok {
			return fail("missing " + key + " column")
		}
	}
	separator := kickoffRow(table.lines[header+1])
	if len(separator) != len(fields) {
		return fail("invalid separator")
	}
	for _, cell := range separator[1 : len(separator)-1] {
		if strings.Trim(strings.TrimSpace(cell), "-:") != "" || !strings.Contains(cell, "-") {
			return fail("invalid separator")
		}
	}
	table.insert = header + 2
	for index := header + 2; index < len(table.lines); index++ {
		line := table.lines[index]
		if !strings.HasPrefix(strings.TrimSpace(line), "|") {
			break
		}
		row := kickoffRow(line)
		if len(row) != len(fields) {
			return fail("wrong column count")
		}
		cell := func(key string) string {
			if pos, ok := table.headers[key]; ok {
				return strings.TrimSpace(row[pos])
			}
			return ""
		}
		task := core.Task{ID: core.TaskID(cell("id")), Objective: cell("intended outcome / acceptance pointer")}
		state, ok := kickoffState(cell("status"))
		if !ok {
			return fail("unknown status")
		}
		task.State, task.Archived = state, state == core.Archived
		if err := validateTask(task); err != nil {
			return table, true, err
		}
		if _, ok := table.rows[task.ID]; ok {
			return fail("duplicate task ID")
		}
		task.Criteria = []string{task.Objective}
		deps := cell("depends on")
		if !kickoffEmpty(deps) {
			seen := map[core.TaskID]bool{}
			for _, id := range appendTaskIDs(nil, deps) {
				if validateTaskID(id) != nil || id == task.ID || seen[id] {
					return fail("invalid dependency")
				}
				seen[id] = true
				task.Dependencies = append(task.Dependencies, id)
			}
		}
		if evidence := cell("revision / evidence"); !kickoffEmpty(evidence) {
			task.EvidencePointers = []string{evidence}
		}
		// Optional JSON cells retain per-task facts without creating another
		// tracker. A blank cell leaves legacy acceptance/evidence pointers intact.
		for _, field := range []struct {
			name   string
			target any
		}{
			{"criteria", &task.Criteria}, {"checks", &task.Checks},
			{"writable paths", &task.WritablePaths}, {"resources", &task.Resources},
			{"evidence pointers", &task.EvidencePointers},
		} {
			if raw := cell(field.name); raw != "" {
				if err := decodeKickoffCell(raw, field.target); err != nil {
					return fail("invalid " + field.name + " JSON cell")
				}
			}
		}
		for _, check := range task.Checks {
			if strings.TrimSpace(check.Name) == "" || len(check.Command) == 0 || strings.TrimSpace(check.Command[0]) == "" {
				return fail("incomplete check")
			}
		}
		table.rows[task.ID] = index
		table.tasks = append(table.tasks, task)
		table.insert = index + 1
	}
	return table, true, nil
}

// Split only unescaped table delimiters. Keeping the outer cells and each
// cell's whitespace lets an archive replace its status without touching any
// other byte on that row.
func kickoffRow(line string) []string {
	if !strings.HasPrefix(strings.TrimSpace(line), "|") || !strings.HasSuffix(strings.TrimSpace(line), "|") {
		return nil
	}
	var cells []string
	start, slashes := 0, 0
	for index, character := range line {
		if character == '|' && slashes%2 == 0 {
			cells = append(cells, line[start:index])
			start = index + 1
		}
		if character == '\\' {
			slashes++
		} else {
			slashes = 0
		}
	}
	return append(cells, line[start:])
}

func kickoffEmpty(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "none", "none yet", "-", "unassigned":
		return true
	}
	return false
}

func kickoffState(value string) (core.TaskState, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "open", "todo", "pending", "ready":
		return core.Ready, true
	case "verified", "deployed", "closed", "done", "complete", "completed", "integrated":
		return core.Integrated, true
	case "in_progress", "active", "working":
		return core.Working, true
	case "deferred", "approved deferred", "approved_deferred", "parked", "paused":
		return core.Paused, true
	case "canceled", "cancelled":
		return core.Cancelled, true
	}
	state := core.TaskState(strings.ToLower(strings.TrimSpace(value)))
	return state, knownTaskState(state)
}

const maxKickoffCellBytes = 64 << 10

func decodeKickoffCell(raw string, target any) error {
	if len(raw) > maxKickoffCellBytes || raw == "null" {
		return core.ErrLimit
	}
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return core.ErrPath
	}
	return nil
}

func encodeKickoffCell(value any) (string, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	// JSON already escapes quotes, controls and backslashes. Escape the table
	// delimiter as JSON too; Markdown escaping would change the decoded value.
	cell := strings.ReplaceAll(string(raw), "|", `\u007c`)
	if len(cell) > maxKickoffCellBytes {
		return "", fmt.Errorf("%w: kickoff task JSON cell exceeds %d bytes", core.ErrLimit, maxKickoffCellBytes)
	}
	return cell, nil
}

func (table *kickoffTable) addColumn(label string) {
	if _, present := table.headers[strings.ToLower(label)]; present {
		return
	}
	table.width++
	table.headers[strings.ToLower(label)] = table.width
	for index := table.header; index < table.insert; index++ {
		value := " "
		if index == table.header {
			value = " " + label + " "
		} else if index == table.header+1 {
			value = " --- "
		}
		cells := kickoffRow(table.lines[index])
		last := len(cells) - 1
		cells = append(cells[:last], append([]string{value}, cells[last:]...)...)
		table.lines[index] = strings.Join(cells, "|")
	}
}

func (t *tasksMD) writeTaskMutation(tasks []core.Task, revision uint64, created *core.Task, archived core.TaskID) error {
	data, err := readBounded(t.path, t.limit())
	if err != nil {
		return err
	}
	if err := requireRevision(revision, trackerRevision(data)); err != nil {
		return err
	}
	table, found, err := parseKickoffTable(data)
	if err != nil {
		return err
	}
	if !found {
		return t.write(renderTasks(tasks))
	}
	if created != nil {
		structured := map[string]string{}
		for _, field := range []struct {
			label string
			count int
			value any
		}{
			{"Criteria", len(created.Criteria), created.Criteria},
			{"Checks", len(created.Checks), created.Checks},
			{"Writable paths", len(created.WritablePaths), created.WritablePaths},
			{"Resources", len(created.Resources), created.Resources},
			{"Evidence pointers", len(created.EvidencePointers), created.EvidencePointers},
		} {
			if field.count == 0 {
				continue
			}
			encoded, err := encodeKickoffCell(field.value)
			if err != nil {
				return err
			}
			table.addColumn(field.label)
			structured[strings.ToLower(field.label)] = encoded
		}
		cells := make([]string, table.width+2)
		for i := 1; i <= table.width; i++ {
			cells[i] = " "
		}
		set := func(key, value string) error {
			if strings.ContainsAny(value, "|\r\n") {
				return fmt.Errorf("%w: task value cannot be represented in kickoff table", core.ErrPath)
			}
			if pos, ok := table.headers[key]; ok {
				cells[pos] = " " + value + " "
				return nil
			}
			if value != "" {
				return fmt.Errorf("%w: kickoff table missing %s column", core.ErrSettings, key)
			}
			return nil
		}
		deps := make([]string, len(created.Dependencies))
		for i, id := range created.Dependencies {
			deps[i] = string(id)
		}
		values := map[string]string{"id": string(created.ID), "intended outcome / acceptance pointer": created.Objective, "status": string(created.State), "depends on": strings.Join(deps, ", ")}
		if created.Archived {
			values["status"] = "archived"
		}
		for key, value := range structured {
			values[key] = value
		}
		for key, value := range values {
			if err := set(key, value); err != nil {
				return err
			}
		}
		row := strings.Join(cells, "|")
		table.lines = append(table.lines[:table.insert], append([]string{row}, table.lines[table.insert:]...)...)
	} else {
		index, ok := table.rows[archived]
		if !ok {
			return core.ErrPath
		}
		cells := kickoffRow(table.lines[index])
		pos := table.headers["status"]
		original := cells[pos]
		leading := original[:len(original)-len(strings.TrimLeft(original, " \t"))]
		trailing := original[len(strings.TrimRight(original, " \t\r")):]
		cells[pos] = leading + "archived" + trailing
		table.lines[index] = strings.Join(cells, "|")
	}
	output := []byte(strings.Join(table.lines, "\n"))
	if _, _, err := parseKickoffTable(output); err != nil {
		return err
	}
	return t.write(output)
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
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		return nil, fmt.Errorf("%w: null tracker JSON", core.ErrPath)
	}
	var records []json.RawMessage
	if err := json.Unmarshal(data, &records); err != nil || records == nil {
		return nil, fmt.Errorf("%w: tracker JSON must be a non-null task array", core.ErrPath)
	}
	tasks := make([]core.Task, 0, len(records))
	seen := map[core.TaskID]bool{}
	for _, raw := range records {
		if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			return nil, fmt.Errorf("%w: null tracker task", core.ErrPath)
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(raw, &fields); err != nil || fields == nil {
			return nil, fmt.Errorf("%w: malformed tracker task", core.ErrPath)
		}
		for key, value := range fields {
			if !knownTaskJSONField(key) || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
				return nil, fmt.Errorf("%w: invalid tracker field %q", core.ErrPath, key)
			}
		}
		for _, required := range []string{"id", "objective", "state"} {
			if _, ok := fields[required]; !ok {
				return nil, fmt.Errorf("%w: tracker task missing %s", core.ErrPath, required)
			}
		}
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.DisallowUnknownFields()
		var task core.Task
		if err := decoder.Decode(&task); err != nil {
			return nil, fmt.Errorf("%w: malformed tracker task", core.ErrPath)
		}
		if err := validateTask(task); err != nil || !knownTaskState(task.State) {
			return nil, fmt.Errorf("%w: invalid tracker task state", core.ErrPath)
		}
		if seen[task.ID] {
			return nil, fmt.Errorf("%w: duplicate task ID %q", core.ErrPath, task.ID)
		}
		seen[task.ID] = true
		tasks = append(tasks, task)
	}
	return tasks, nil
}

func knownTaskJSONField(key string) bool {
	return map[string]bool{"schema": true, "project": true, "runId": true, "writtenAt": true, "revision": true, "id": true, "objective": true, "state": true, "dependencies": true, "criteria": true, "checks": true, "writablePaths": true, "resources": true, "evidencePointers": true, "archived": true}[key]
}

func knownTaskState(state core.TaskState) bool {
	switch state {
	case core.Ready, core.Idle, core.Working, core.Implementing, core.Reviewing, core.Fix, core.Clean, core.Gated, core.Integrated, core.Paused, core.Blocked, core.Interrupted, core.Cancelled, core.Archived:
		return true
	}
	return false
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
