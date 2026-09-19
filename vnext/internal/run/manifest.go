// Package run creates and validates the bounded, canonical run manifests.
package run

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path"
	"reflect"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/thebpandey/agent-team/vnext/internal/core"
	"github.com/thebpandey/agent-team/vnext/internal/tracker"
)

const (
	maxTeamQueue       = 8
	maxOneOffTeams     = 2
	canonicalWrittenAt = "1970-01-01T00:00:00Z"
)

// OneOffKind is the deliberately small set of trackerless run contracts.
type OneOffKind string

const (
	OneOffFeature OneOffKind = "feature"
	OneOffAudit   OneOffKind = "audit"
	OneOffReview  OneOffKind = "review"

	// Short names keep call sites close to the user-facing one-off verbs.
	Feature = OneOffFeature
	Audit   = OneOffAudit
	Review  = OneOffReview
)

// TeamRecord is the durable bounded queue assigned to one retained team.
type TeamRecord struct {
	core.RecordEnvelope
	ID               core.TeamID           `json:"teamId"`
	Queue            []core.TaskID         `json:"queue"`
	QueueFingerprint string                `json:"queueFingerprint"`
	State            core.TaskState        `json:"state"`
	Worktree         string                `json:"worktree,omitempty"`
	Base             string                `json:"base,omitempty"`
	WritablePaths    []string              `json:"writablePaths,omitempty"`
	ResourceRefs     core.ResourceSnapshot `json:"resourceRefs,omitempty"`
}

// Run is the schema-1 authority for either a selected tracker snapshot or a
// trackerless one-off request. Plan runs retain only a snapshot digest, never
// copied tracker task payloads; Tasks belongs exclusively to one-off manifests.
type Run struct {
	core.RecordEnvelope
	ID                    core.RunID     `json:"id"`
	Mode                  string         `json:"mode"`
	OneOffKind            OneOffKind     `json:"oneOffKind,omitempty"`
	Objective             string         `json:"objective,omitempty"`
	SpecRevision          string         `json:"specRevision,omitempty"`
	TrackerKind           string         `json:"trackerKind"`
	TrackerRevision       uint64         `json:"trackerRevision"`
	TrackerSnapshotDigest string         `json:"trackerSnapshotDigest,omitempty"`
	CanonicalRevision     string         `json:"canonicalRevision,omitempty"`
	State                 core.TaskState `json:"state"`
	Tasks                 []core.Task    `json:"tasks"`
	Teams                 []TeamRecord   `json:"teams,omitempty"`
	ManifestDigest        string         `json:"manifestDigest"`
}

// AdmissionBatch is the bounded proposed addition to one team queue. The
// admission service owns the append-only history; this value is only its
// validated canonical payload.
type AdmissionBatch struct {
	BatchID                  string        `json:"batchId"`
	TeamID                   core.TeamID   `json:"teamId"`
	TaskIDs                  []core.TaskID `json:"taskIds"`
	PreviousQueueFingerprint string        `json:"previousQueueFingerprint,omitempty"`
	QueueFingerprint         string        `json:"queueFingerprint,omitempty"`
	TrackerRevision          uint64        `json:"trackerRevision"`
	AdmittedAt               string        `json:"admittedAt,omitempty"`
}

// CreatePlan snapshots exactly one selected tracker. The returned ID and
// digest use only normalized snapshot inputs, never process-local state.
func CreatePlan(ctx context.Context, project string, selected tracker.Tracker) (Run, error) {
	if err := ctx.Err(); err != nil {
		return Run{}, err
	}
	if err := validateProject(project); err != nil {
		return Run{}, err
	}
	if selected == nil {
		return Run{}, fmt.Errorf("%w: selected tracker is required", core.ErrSettings)
	}
	page, err := selected.Page(ctx, "", 1000)
	if err != nil {
		return Run{}, err
	}
	if err := ctx.Err(); err != nil {
		return Run{}, err
	}
	if page.TrackerRevision == 0 || page.TotalNonArchived < 0 || page.TotalNonArchived > 1000 || page.Cursor != "" || len(page.Tasks) != page.TotalNonArchived {
		return Run{}, fmt.Errorf("%w: incomplete selected tracker snapshot", core.ErrRevision)
	}
	snapshotDigest, err := taskSnapshotDigest(page.Tasks)
	if err != nil {
		return Run{}, err
	}
	r := Run{
		RecordEnvelope:        core.RecordEnvelope{Schema: 1, Project: strings.TrimSpace(project), WrittenAt: canonicalWrittenAt, Revision: 1},
		Mode:                  "plan",
		TrackerKind:           selectedTrackerKind(selected),
		TrackerRevision:       page.TrackerRevision,
		TrackerSnapshotDigest: snapshotDigest,
		State:                 core.Ready,
	}
	if r.TrackerKind == "" || r.TrackerKind == "none" {
		return Run{}, fmt.Errorf("%w: unknown selected tracker", core.ErrSettings)
	}
	return finalizeRun(r)
}

// CreateOneOff creates a trackerless immutable request. Audit and review
// manifests erase writable paths and can never be used to authorize writes.
func CreateOneOff(ctx context.Context, project string, kind OneOffKind, objective string, tasks []core.Task) (Run, error) {
	if err := ctx.Err(); err != nil {
		return Run{}, err
	}
	if err := validateProject(project); err != nil {
		return Run{}, err
	}
	objective = strings.TrimSpace(objective)
	if objective == "" || !utf8.ValidString(objective) || len(objective) > 256<<10 {
		return Run{}, fmt.Errorf("%w: one-off objective is required and bounded", core.ErrPath)
	}
	if !validOneOffKind(kind) {
		return Run{}, fmt.Errorf("%w: unsupported one-off kind %q", core.ErrSettings, kind)
	}
	if len(tasks) == 0 || len(tasks) > maxOneOffTeams*maxTeamQueue {
		return Run{}, fmt.Errorf("%w: one-off contains %d tasks", core.ErrBatch, len(tasks))
	}
	for i := range tasks {
		if kind == OneOffAudit || kind == OneOffReview {
			// A caller may reuse a task-shaped input from a plan. The immutable
			// read-only manifest deliberately drops that write authority.
			tasks[i].WritablePaths = nil
		} else if len(tasks[i].WritablePaths) == 0 {
			return Run{}, fmt.Errorf("%w: feature task %q has no bounded writable path", core.ErrBatch, tasks[i].ID)
		}
	}
	normalized, err := normalizeTasks(tasks)
	if err != nil {
		return Run{}, err
	}
	teams, err := planOneOffTeams(normalized)
	if err != nil {
		return Run{}, err
	}
	r := Run{
		RecordEnvelope: core.RecordEnvelope{Schema: 1, Project: strings.TrimSpace(project), WrittenAt: canonicalWrittenAt, Revision: 1},
		Mode:           "one-off",
		OneOffKind:     kind,
		Objective:      objective,
		TrackerKind:    "none",
		State:          core.Ready,
		Tasks:          normalized,
		Teams:          teams,
	}
	return finalizeRun(r)
}

// ValidateConflict reports whether two task scopes cannot run independently.
// Unsafe paths/resources are conflicts too: callers must fail closed.
func ValidateConflict(a, b core.Task) bool {
	na, err := normalizeTask(a)
	if err != nil {
		return true
	}
	nb, err := normalizeTask(b)
	if err != nil {
		return true
	}
	for _, left := range na.WritablePaths {
		for _, right := range nb.WritablePaths {
			if pathsOverlap(left, right) {
				return true
			}
		}
	}
	for _, left := range na.Resources {
		for _, right := range nb.Resources {
			if left == right {
				return true
			}
		}
	}
	return false
}

func finalizeRun(r Run) (Run, error) {
	digest, err := manifestDigest(r)
	if err != nil {
		return Run{}, err
	}
	r.ManifestDigest = digest
	r.ID = core.RunID("run-" + strings.TrimPrefix(digest, "sha256:")[:24])
	r.RunID = r.ID
	for i := range r.Tasks {
		r.Tasks[i].RecordEnvelope = core.RecordEnvelope{Schema: 1, Project: r.Project, RunID: r.ID, WrittenAt: canonicalWrittenAt, Revision: 1}
	}
	for i := range r.Teams {
		r.Teams[i].ID = core.TeamID(fmt.Sprintf("%s-team-%d", r.ID, i+1))
		r.Teams[i].RecordEnvelope = core.RecordEnvelope{Schema: 1, Project: r.Project, RunID: r.ID, WrittenAt: canonicalWrittenAt, Revision: 1}
		r.Teams[i].QueueFingerprint = queueFingerprint(r.Teams[i].Queue)
	}
	if err := validateRun(r); err != nil {
		return Run{}, err
	}
	return r, nil
}

func planOneOffTeams(tasks []core.Task) ([]TeamRecord, error) {
	components := make([][]core.TaskID, 0, len(tasks))
	used := make([]bool, len(tasks))
	for i := range tasks {
		if used[i] {
			continue
		}
		used[i] = true
		component := []int{i}
		for cursor := 0; cursor < len(component); cursor++ {
			for j := range tasks {
				if !used[j] && ValidateConflict(tasks[component[cursor]], tasks[j]) {
					used[j] = true
					component = append(component, j)
				}
			}
		}
		queue := make([]core.TaskID, 0, len(component))
		for _, index := range component {
			queue = append(queue, tasks[index].ID)
		}
		if len(queue) > maxTeamQueue {
			return nil, fmt.Errorf("%w: overlapping one-off tasks exceed one team queue", core.ErrBatch)
		}
		components = append(components, queue)
	}
	// Components are internally serial. Greedy packing is deterministic because
	// tasks and component roots are ID-sorted by normalizeTasks.
	teams := make([]TeamRecord, 0, maxOneOffTeams)
	for _, component := range components {
		placed := false
		for i := range teams {
			if len(teams[i].Queue)+len(component) <= maxTeamQueue {
				teams[i].Queue = append(teams[i].Queue, component...)
				placed = true
				break
			}
		}
		if !placed {
			if len(teams) == maxOneOffTeams {
				return nil, fmt.Errorf("%w: one-off needs more than two independent teams", core.ErrBatch)
			}
			teams = append(teams, TeamRecord{Queue: append([]core.TaskID(nil), component...), State: core.Working})
		}
	}
	return teams, nil
}

func normalizeTasks(tasks []core.Task) ([]core.Task, error) {
	if len(tasks) == 0 {
		return nil, fmt.Errorf("%w: task list is empty", core.ErrBatch)
	}
	normalized := make([]core.Task, len(tasks))
	for i, task := range tasks {
		value, err := normalizeTask(task)
		if err != nil {
			return nil, err
		}
		normalized[i] = value
	}
	sort.Slice(normalized, func(i, j int) bool { return normalized[i].ID < normalized[j].ID })
	for i := 1; i < len(normalized); i++ {
		if normalized[i-1].ID == normalized[i].ID {
			return nil, fmt.Errorf("%w: duplicate task ID %q", core.ErrBatch, normalized[i].ID)
		}
	}
	return normalized, nil
}

func taskSnapshotDigest(tasks []core.Task) (string, error) {
	if len(tasks) == 0 {
		return digestJSON([]core.Task{})
	}
	normalized, err := normalizeTasks(tasks)
	if err != nil {
		return "", err
	}
	return digestJSON(normalized)
}

func normalizeTask(task core.Task) (core.Task, error) {
	if err := validateID(string(task.ID)); err != nil {
		return core.Task{}, err
	}
	task.Objective = strings.TrimSpace(task.Objective)
	if task.Objective == "" || !utf8.ValidString(task.Objective) || len(task.Objective) > 256<<10 {
		return core.Task{}, fmt.Errorf("%w: task %q objective", core.ErrPath, task.ID)
	}
	if !validTaskState(task.State) || task.State == core.Archived || task.Archived {
		return core.Task{}, fmt.Errorf("%w: invalid task state for %q", core.ErrPhase, task.ID)
	}
	var err error
	if task.Dependencies, err = normalizeIDs(task.Dependencies); err != nil {
		return core.Task{}, err
	}
	if task.Criteria, err = normalizeStrings(task.Criteria, "criterion"); err != nil {
		return core.Task{}, err
	}
	if task.WritablePaths, err = normalizePaths(task.WritablePaths); err != nil {
		return core.Task{}, err
	}
	if task.Resources, err = normalizeResources(task.Resources); err != nil {
		return core.Task{}, err
	}
	if task.EvidencePointers, err = normalizeStrings(task.EvidencePointers, "evidence pointer"); err != nil {
		return core.Task{}, err
	}
	if task.Checks, err = normalizeChecks(task.Checks); err != nil {
		return core.Task{}, err
	}
	task.RecordEnvelope = core.RecordEnvelope{}
	return task, nil
}

func normalizeIDs(values []core.TaskID) ([]core.TaskID, error) {
	if len(values) == 0 {
		return nil, nil
	}
	out := append([]core.TaskID(nil), values...)
	for _, value := range out {
		if err := validateID(string(value)); err != nil {
			return nil, err
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	for i := 1; i < len(out); i++ {
		if out[i-1] == out[i] {
			return nil, fmt.Errorf("%w: duplicate task dependency", core.ErrBatch)
		}
	}
	return out, nil
}

func normalizeStrings(values []string, label string) ([]string, error) {
	if len(values) == 0 {
		return nil, nil
	}
	out := make([]string, len(values))
	for i, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || !utf8.ValidString(value) || strings.ContainsAny(value, "\x00\r\n") || len(value) > 4096 {
			return nil, fmt.Errorf("%w: invalid %s", core.ErrPath, label)
		}
		out[i] = value
	}
	sort.Strings(out)
	for i := 1; i < len(out); i++ {
		if out[i-1] == out[i] {
			return nil, fmt.Errorf("%w: duplicate %s", core.ErrBatch, label)
		}
	}
	return out, nil
}

func normalizeChecks(values []core.Check) ([]core.Check, error) {
	if len(values) == 0 {
		return nil, nil
	}
	out := make([]core.Check, len(values))
	for i, check := range values {
		check.Name = strings.TrimSpace(check.Name)
		if check.Name == "" || len(check.Command) == 0 || !utf8.ValidString(check.Name) {
			return nil, fmt.Errorf("%w: invalid check", core.ErrPath)
		}
		check.Command = append([]string(nil), check.Command...)
		for _, arg := range check.Command {
			if strings.TrimSpace(arg) == "" || !utf8.ValidString(arg) || strings.ContainsRune(arg, 0) {
				return nil, fmt.Errorf("%w: invalid check command", core.ErrPath)
			}
		}
		out[i] = check
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Name == out[j].Name {
			return strings.Join(out[i].Command, "\x00") < strings.Join(out[j].Command, "\x00")
		}
		return out[i].Name < out[j].Name
	})
	for i := 1; i < len(out); i++ {
		if reflect.DeepEqual(out[i-1], out[i]) {
			return nil, fmt.Errorf("%w: duplicate check", core.ErrBatch)
		}
	}
	return out, nil
}

func normalizePaths(values []string) ([]string, error) {
	if len(values) == 0 {
		return nil, nil
	}
	out := make([]string, len(values))
	for i, value := range values {
		value, err := normalizePath(value)
		if err != nil {
			return nil, err
		}
		out[i] = value
	}
	sort.Strings(out)
	for i := 1; i < len(out); i++ {
		if out[i-1] == out[i] {
			return nil, fmt.Errorf("%w: duplicate writable path %q", core.ErrPath, out[i])
		}
	}
	return out, nil
}

func normalizePath(value string) (string, error) {
	value = strings.TrimSpace(strings.ReplaceAll(value, "\\", "/"))
	if value == "" || !utf8.ValidString(value) || len(value) > 4096 || strings.HasPrefix(value, "/") || strings.ContainsRune(value, 0) || (len(value) > 1 && value[1] == ':') {
		return "", fmt.Errorf("%w: unsafe writable path", core.ErrPath)
	}
	glob := strings.HasSuffix(value, "/**")
	base := value
	if glob {
		base = strings.TrimSuffix(base, "/**")
	}
	clean := path.Clean(base)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") || strings.Contains(clean, "//") {
		return "", fmt.Errorf("%w: unsafe writable path %q", core.ErrPath, value)
	}
	for _, part := range strings.Split(clean, "/") {
		if part == "" || part == "." || part == ".." {
			return "", fmt.Errorf("%w: unsafe writable path %q", core.ErrPath, value)
		}
	}
	if glob {
		return strings.ToLower(clean) + "/**", nil
	}
	return strings.ToLower(clean), nil
}

func normalizeResources(values []string) ([]string, error) {
	if len(values) == 0 {
		return nil, nil
	}
	out := make([]string, len(values))
	for i, value := range values {
		value = strings.ToLower(strings.TrimSpace(strings.ReplaceAll(value, "\\", "/")))
		if value == "" || !utf8.ValidString(value) || len(value) > 4096 || strings.ContainsAny(value, "\x00\r\n") || value == "." || value == ".." || strings.Contains(value, "../") {
			return nil, fmt.Errorf("%w: unsafe resource", core.ErrPath)
		}
		out[i] = value
	}
	sort.Strings(out)
	for i := 1; i < len(out); i++ {
		if out[i-1] == out[i] {
			return nil, fmt.Errorf("%w: duplicate resource %q", core.ErrBatch, out[i])
		}
	}
	return out, nil
}

func pathsOverlap(a, b string) bool {
	a, b = strings.TrimSuffix(a, "/**"), strings.TrimSuffix(b, "/**")
	return a == b || strings.HasPrefix(a, b+"/") || strings.HasPrefix(b, a+"/")
}

func manifestDigest(r Run) (string, error) {
	tasks := []core.Task(nil)
	if r.Mode == "one-off" {
		var err error
		tasks, err = normalizeTasks(r.Tasks)
		if err != nil {
			return "", err
		}
	}
	type team struct {
		Queue         []core.TaskID         `json:"queue"`
		WritablePaths []string              `json:"writablePaths,omitempty"`
		Resources     core.ResourceSnapshot `json:"resources,omitempty"`
	}
	teams := make([]team, len(r.Teams))
	for i, record := range r.Teams {
		paths, err := normalizePaths(record.WritablePaths)
		if err != nil {
			return "", err
		}
		teams[i] = team{Queue: append([]core.TaskID(nil), record.Queue...), WritablePaths: paths, Resources: normalizeSnapshot(record.ResourceRefs)}
	}
	wire := struct {
		Project               string      `json:"project"`
		Mode                  string      `json:"mode"`
		OneOffKind            OneOffKind  `json:"oneOffKind,omitempty"`
		Objective             string      `json:"objective,omitempty"`
		SpecRevision          string      `json:"specRevision,omitempty"`
		TrackerKind           string      `json:"trackerKind"`
		TrackerRevision       uint64      `json:"trackerRevision"`
		TrackerSnapshotDigest string      `json:"trackerSnapshotDigest,omitempty"`
		Tasks                 []core.Task `json:"tasks,omitempty"`
		Teams                 []team      `json:"teams,omitempty"`
	}{strings.TrimSpace(r.Project), r.Mode, r.OneOffKind, strings.TrimSpace(r.Objective), r.SpecRevision, r.TrackerKind, r.TrackerRevision, r.TrackerSnapshotDigest, tasks, teams}
	raw, err := json.Marshal(wire)
	if err != nil {
		return "", fmt.Errorf("%w: canonical manifest: %v", core.ErrRevision, err)
	}
	sum := sha256.Sum256(raw)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

func queueFingerprint(queue []core.TaskID) string {
	raw, _ := json.Marshal(queue)
	sum := sha256.Sum256(raw)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func digestJSON(value any) (string, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("%w: canonical manifest: %v", core.ErrRevision, err)
	}
	sum := sha256.Sum256(raw)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

func normalizeSnapshot(snapshot core.ResourceSnapshot) core.ResourceSnapshot {
	for _, part := range []*[]string{&snapshot.Servers, &snapshot.Browsers, &snapshot.External} {
		values := *part
		for i := range values {
			values[i] = strings.ToLower(strings.TrimSpace(values[i]))
		}
		sort.Strings(values)
		*part = values
	}
	return snapshot
}

func selectedTrackerKind(selected tracker.Tracker) string {
	name := strings.ToLower(fmt.Sprintf("%T", selected))
	switch {
	case strings.Contains(name, "beads"):
		return "beads"
	case strings.Contains(name, "tasksmd") || strings.Contains(name, "tasks_md"):
		return "tasks-md"
	default:
		// A future selected adapter still represents exactly one authority. Do
		// not persist its Go type spelling, which is not a stable contract.
		return "selected"
	}
}

func validateProject(project string) error {
	project = strings.TrimSpace(project)
	if project == "" || !utf8.ValidString(project) || len(project) > 4096 || strings.ContainsAny(project, "\x00\r\n") {
		return fmt.Errorf("%w: invalid project", core.ErrPath)
	}
	return nil
}

func validateID(value string) error {
	if value == "" || len(value) > 128 || !utf8.ValidString(value) || value == "." || value == ".." {
		return fmt.Errorf("%w: unsafe ID %q", core.ErrPath, value)
	}
	for _, r := range value {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' || r == '.') {
			return fmt.Errorf("%w: unsafe ID %q", core.ErrPath, value)
		}
	}
	return nil
}

func validOneOffKind(kind OneOffKind) bool {
	return kind == OneOffFeature || kind == OneOffAudit || kind == OneOffReview
}
func validTaskState(state core.TaskState) bool {
	switch state {
	case core.Ready, core.Idle, core.Working, core.Implementing, core.Reviewing, core.Fix, core.Clean, core.Gated, core.Integrated, core.Paused, core.Blocked, core.Interrupted, core.Cancelled:
		return true
	}
	return false
}
func validTeamState(state core.TaskState) bool {
	return state == core.Idle || state == core.Working || state == core.Paused || state == core.Blocked || state == core.Cancelled
}

func validateAdmission(batch AdmissionBatch) error {
	if err := validateID(batch.BatchID); err != nil {
		return err
	}
	if err := validateID(string(batch.TeamID)); err != nil {
		return err
	}
	if len(batch.TaskIDs) == 0 || len(batch.TaskIDs) > maxTeamQueue {
		return fmt.Errorf("%w: admission has %d tasks", core.ErrBatch, len(batch.TaskIDs))
	}
	if batch.PreviousQueueFingerprint != "" && !validDigest(batch.PreviousQueueFingerprint) {
		return fmt.Errorf("%w: invalid prior queue fingerprint", core.ErrRevision)
	}
	if batch.QueueFingerprint != "" && !validDigest(batch.QueueFingerprint) {
		return fmt.Errorf("%w: invalid queue fingerprint", core.ErrRevision)
	}
	if batch.AdmittedAt != "" {
		if _, err := time.Parse(time.RFC3339, batch.AdmittedAt); err != nil {
			return fmt.Errorf("%w: invalid admission timestamp", core.ErrRevision)
		}
	}
	_, err := normalizeIDs(batch.TaskIDs)
	return err
}

func validDigest(value string) bool {
	if len(value) != len("sha256:")+sha256.Size*2 || !strings.HasPrefix(value, "sha256:") {
		return false
	}
	_, err := hex.DecodeString(strings.TrimPrefix(value, "sha256:"))
	return err == nil
}

func validateEnvelope(envelope core.RecordEnvelope, id core.RunID) error {
	if envelope.Schema != 1 || envelope.Revision == 0 || envelope.RunID != id {
		return fmt.Errorf("%w: invalid record envelope", core.ErrRevision)
	}
	if err := validateProject(envelope.Project); err != nil {
		return err
	}
	if err := validateID(string(envelope.RunID)); err != nil {
		return err
	}
	if _, err := time.Parse(time.RFC3339, envelope.WrittenAt); err != nil {
		return fmt.Errorf("%w: invalid record timestamp", core.ErrRevision)
	}
	return nil
}

func validateRun(r Run) error {
	if err := validateID(string(r.ID)); err != nil {
		return err
	}
	if err := validateEnvelope(r.RecordEnvelope, r.ID); err != nil {
		return err
	}
	if !validTaskState(r.State) || (r.Mode != "plan" && r.Mode != "one-off") {
		return fmt.Errorf("%w: invalid run state or mode", core.ErrPhase)
	}
	if r.Mode == "plan" && (r.TrackerKind == "" || r.TrackerKind == "none" || r.TrackerRevision == 0 || !validDigest(r.TrackerSnapshotDigest) || r.OneOffKind != "" || len(r.Tasks) != 0) {
		return fmt.Errorf("%w: invalid plan tracker authority", core.ErrRevision)
	}
	if r.Mode == "one-off" && (r.TrackerKind != "none" || r.TrackerRevision != 0 || r.TrackerSnapshotDigest != "" || !validOneOffKind(r.OneOffKind) || strings.TrimSpace(r.Objective) == "") {
		return fmt.Errorf("%w: invalid one-off authority", core.ErrRevision)
	}
	if r.Mode == "one-off" {
		tasks, err := normalizeTasks(r.Tasks)
		if err != nil {
			return err
		}
		for i := range tasks {
			if !reflect.DeepEqual(tasks[i], withoutEnvelope(r.Tasks[i])) {
				return fmt.Errorf("%w: noncanonical task manifest", core.ErrRevision)
			}
		}
	}
	if len(r.Teams) > maxOneOffTeams {
		return fmt.Errorf("%w: too many teams", core.ErrBatch)
	}
	if r.Mode == "one-off" && len(r.Teams) == 0 {
		return fmt.Errorf("%w: one-off has no team", core.ErrBatch)
	}
	seen := map[core.TaskID]bool{}
	byID := map[core.TaskID]core.Task{}
	for _, task := range r.Tasks {
		if err := validateEnvelope(task.RecordEnvelope, r.ID); err != nil {
			return fmt.Errorf("%w: task envelope: %v", core.ErrRevision, err)
		}
		byID[task.ID] = task
	}
	for _, team := range r.Teams {
		if team.RunID != r.ID {
			return fmt.Errorf("%w: team belongs to a different run", core.ErrRevision)
		}
		if err := validateTeam(team); err != nil {
			return err
		}
		for _, id := range team.Queue {
			if seen[id] || byID[id].ID == "" {
				return fmt.Errorf("%w: invalid team task %q", core.ErrBatch, id)
			}
			seen[id] = true
		}
	}
	if r.Mode == "one-off" && len(seen) != len(r.Tasks) {
		return fmt.Errorf("%w: one-off task omitted from team", core.ErrBatch)
	}
	digest, err := manifestDigest(r)
	if err != nil {
		return err
	}
	if r.ManifestDigest != digest {
		return fmt.Errorf("%w: manifest digest mismatch", core.ErrRevision)
	}
	if r.ID != core.RunID("run-"+strings.TrimPrefix(digest, "sha256:")[:24]) {
		return fmt.Errorf("%w: manifest ID mismatch", core.ErrRevision)
	}
	return nil
}

// validateRepositoryRun also accepts the envelope-only record used while a
// caller reserves a run identity before a planner has populated a manifest.
// It is not created by CreatePlan/CreateOneOff and contains no task authority.
func validateRepositoryRun(r Run) error {
	if r.Mode != "" {
		return validateRun(r)
	}
	if err := validateID(string(r.ID)); err != nil {
		return err
	}
	if err := validateEnvelope(r.RecordEnvelope, r.ID); err != nil {
		return err
	}
	if r.OneOffKind != "" || r.Objective != "" || r.SpecRevision != "" || r.TrackerKind != "" || r.TrackerRevision != 0 || r.TrackerSnapshotDigest != "" || r.CanonicalRevision != "" || r.State != "" || r.ManifestDigest != "" || len(r.Tasks) != 0 || len(r.Teams) != 0 {
		return fmt.Errorf("%w: incomplete run manifest", core.ErrRevision)
	}
	return nil
}

func withoutEnvelope(task core.Task) core.Task {
	task.RecordEnvelope = core.RecordEnvelope{}
	return task
}

func validateTeam(team TeamRecord) error {
	if err := validateID(string(team.ID)); err != nil {
		return err
	}
	if err := validateEnvelope(team.RecordEnvelope, team.RunID); err != nil {
		return err
	}
	if !validTeamState(team.State) || len(team.Queue) > maxTeamQueue {
		return fmt.Errorf("%w: invalid team record", core.ErrPhase)
	}
	if len(team.Queue) == 0 {
		if team.State != core.Idle || (team.QueueFingerprint != "" && team.QueueFingerprint != queueFingerprint(nil)) {
			return fmt.Errorf("%w: invalid empty team queue", core.ErrRevision)
		}
		return nil
	}
	if team.QueueFingerprint != queueFingerprint(team.Queue) {
		return fmt.Errorf("%w: invalid team queue", core.ErrRevision)
	}
	paths, err := normalizePaths(team.WritablePaths)
	if err != nil || !reflect.DeepEqual(paths, team.WritablePaths) {
		return fmt.Errorf("%w: invalid team writable paths", core.ErrPath)
	}
	for _, values := range [][]string{team.ResourceRefs.Servers, team.ResourceRefs.Browsers, team.ResourceRefs.External} {
		resources, err := normalizeResources(values)
		if err != nil || !reflect.DeepEqual(resources, values) {
			return fmt.Errorf("%w: invalid team resources", core.ErrPath)
		}
	}
	for _, id := range team.Queue {
		if err := validateID(string(id)); err != nil {
			return err
		}
	}
	return nil
}
