// Package run creates and validates the bounded, canonical run manifests.
package run

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/thebpandey/agent-team/vnext/internal/contracts"
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
	ID               core.TeamID            `json:"teamId"`
	Queue            []core.TaskID          `json:"queue"`
	QueueFingerprint string                 `json:"queueFingerprint"`
	State            core.TaskState         `json:"state"`
	Paths            []string               `json:"paths,omitempty"`
	Resources        []string               `json:"resources,omitempty"`
	IntentDigest     string                 `json:"intentDigest,omitempty"`
	Handle           contracts.WorkerHandle `json:"handle,omitempty"`
	// RetainedHandle is the last exact native handle while a queued follow-up
	// waits for its own packet-bound acknowledgement. It is never retagged as
	// acknowledgement of the next task.
	RetainedHandle contracts.WorkerHandle `json:"retainedHandle,omitempty"`
	CompletedTask  core.TaskID            `json:"completedTask,omitempty"`
	ReviewedTask   core.TaskID            `json:"reviewedTask,omitempty"`
	HostIdle       bool                   `json:"hostIdle,omitempty"`
}

// Run is the schema-1 authority for either a selected tracker snapshot or a
// trackerless one-off request. Plan runs retain only minimal task references,
// never copied tracker task payloads or criteria.
type Run struct {
	core.RecordEnvelope
	ID                    core.RunID     `json:"id"`
	Root                  string         `json:"root"`
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
	core.RecordEnvelope
	BatchID         string        `json:"batchId"`
	Fingerprint     string        `json:"fingerprint"`
	Tasks           []core.TaskID `json:"tasks"`
	Team            core.TeamID   `json:"team"`
	Sequence        uint64        `json:"sequence"`
	TrackerRevision uint64        `json:"trackerRevision"`
	Paths           []string      `json:"paths"`
	Resources       []string      `json:"resources"`
}

// PreparedPlanAdmission is one tracker-derived ready task prepared for the
// existing append-only admission service. It never changes the tracker.
type PreparedPlanAdmission struct {
	Run   Run
	Team  TeamRecord
	Batch AdmissionBatch
}

// PrepareQueueAdmission constructs one bounded append for an existing team.
// It is deliberately limited to the team queue's remaining capacity; callers
// still use admission.AppendAdmission for current-snapshot validation and the
// durable admission commit.
func PrepareQueueAdmission(manifest Run, team TeamRecord, tasks []core.Task) (AdmissionBatch, map[core.TaskID]uint64, error) {
	if len(tasks) == 0 || len(tasks) > maxTeamQueue-len(team.Queue) || team.RunID != manifest.ID {
		return AdmissionBatch{}, nil, core.ErrBatch
	}
	originalRevisions := make(map[core.TaskID]uint64, len(tasks))
	for _, task := range tasks {
		originalRevisions[task.ID] = task.Revision
	}
	normalized, err := normalizeTasks(tasks)
	if err != nil {
		return AdmissionBatch{}, nil, err
	}
	ids := make([]core.TaskID, len(normalized))
	revisions := make(map[core.TaskID]uint64, len(normalized))
	for index, task := range normalized {
		if task.Archived || task.State != core.Ready {
			return AdmissionBatch{}, nil, core.ErrPhase
		}
		revision := originalRevisions[task.ID]
		if revision == 0 {
			revision = manifest.TrackerRevision
		}
		ids[index], revisions[task.ID] = task.ID, revision
	}
	var rawPaths, rawResources []string
	for _, task := range normalized {
		rawPaths = append(rawPaths, task.WritablePaths...)
		rawResources = append(rawResources, task.Resources...)
	}
	paths, err := normalizePaths(rawPaths)
	if err != nil {
		return AdmissionBatch{}, nil, err
	}
	resources, err := normalizeResources(rawResources)
	if err != nil {
		return AdmissionBatch{}, nil, err
	}
	batch := AdmissionBatch{RecordEnvelope: core.RecordEnvelope{Schema: 1, Project: manifest.Project, RunID: manifest.ID, WrittenAt: manifest.WrittenAt, Revision: 1}, BatchID: fmt.Sprintf("batch-%d", team.Revision+1), Tasks: ids, Team: team.ID, Sequence: team.Revision + 1, TrackerRevision: manifest.TrackerRevision, Paths: paths, Resources: resources}
	batch.Fingerprint = admissionFingerprint(batch)
	return batch, revisions, nil
}

// PrepareSinglePlanAdmission snapshots the selected authority and derives one
// ready, dependency-satisfied task for one initially idle team. Callers must
// persist the returned run/team and pass Batch to admission.AppendAdmission.
func PrepareSinglePlanAdmission(ctx context.Context, project string, selected tracker.Tracker) (PreparedPlanAdmission, error) {
	plan, err := CreatePlan(ctx, project, selected)
	if err != nil {
		return PreparedPlanAdmission{}, err
	}
	page, err := selected.Page(ctx, "", 1000)
	if err != nil {
		return PreparedPlanAdmission{}, err
	}
	byID := make(map[core.TaskID]core.Task, len(page.Tasks))
	for _, task := range page.Tasks {
		byID[task.ID] = task
	}
	var chosen core.Task
	for _, task := range page.Tasks {
		if task.Archived || task.State != core.Ready {
			continue
		}
		ready := true
		for _, dependency := range task.Dependencies {
			prior, found := byID[dependency]
			if !found || (prior.State != core.Clean && prior.State != core.Gated && prior.State != core.Integrated) {
				ready = false
				break
			}
		}
		if ready && (chosen.ID == "" || task.ID < chosen.ID) {
			chosen = task
		}
	}
	if chosen.ID == "" {
		return PreparedPlanAdmission{}, core.ErrPhase
	}
	paths, err := normalizePaths(chosen.WritablePaths)
	if err != nil || len(paths) == 0 {
		return PreparedPlanAdmission{}, core.ErrPath
	}
	resources, err := normalizeResources(chosen.Resources)
	if err != nil {
		return PreparedPlanAdmission{}, core.ErrPath
	}
	plan.Teams = []TeamRecord{{State: core.Idle}}
	plan, err = finalizeRun(plan)
	if err != nil {
		return PreparedPlanAdmission{}, err
	}
	team := plan.Teams[0]
	batch := AdmissionBatch{RecordEnvelope: core.RecordEnvelope{Schema: 1, Project: plan.Project, RunID: plan.ID, WrittenAt: canonicalWrittenAt, Revision: 1}, BatchID: "batch-1", Tasks: []core.TaskID{chosen.ID}, Team: team.ID, Sequence: 1, TrackerRevision: page.TrackerRevision, Paths: paths, Resources: resources}
	batch.Fingerprint = admissionFingerprint(batch)
	return PreparedPlanAdmission{Run: plan, Team: team, Batch: batch}, nil
}

// CreatePlan snapshots exactly one selected tracker. The returned ID and
// digest use only normalized snapshot inputs, never process-local state.
func CreatePlan(ctx context.Context, project string, selected tracker.Tracker) (Run, error) {
	if err := ctx.Err(); err != nil {
		return Run{}, err
	}
	root, err := canonicalRoot(project)
	if err != nil {
		return Run{}, err
	}
	if selected == nil {
		return Run{}, fmt.Errorf("%w: selected tracker is required", core.ErrSettings)
	}
	authority, ok := selected.(tracker.AuthorityMetadataProvider)
	if !ok {
		return Run{}, fmt.Errorf("%w: tracker lacks canonical authority metadata", core.ErrSettings)
	}
	metadata := authority.AuthorityMetadata()
	ref, refErr := canonicalAuthorityRef(root, metadata.Kind, metadata.Ref)
	expectedRef, expectedErr := canonicalTrackerRef(root, metadata.Kind)
	if metadata.Kind != "tasks-md" && metadata.Kind != "beads" || refErr != nil || expectedErr != nil || ref != expectedRef {
		return Run{}, fmt.Errorf("%w: invalid tracker authority metadata", core.ErrSettings)
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
	normalized, err := normalizeTasks(page.Tasks)
	if err != nil {
		return Run{}, err
	}
	snapshotDigest, err := digestJSON(normalized)
	if err != nil {
		return Run{}, err
	}
	refs := planTaskReferences(normalized, page.TrackerRevision)
	r := Run{
		RecordEnvelope:        core.RecordEnvelope{Schema: 1, Project: root, WrittenAt: canonicalWrittenAt, Revision: 1},
		Root:                  root,
		Mode:                  "plan",
		TrackerKind:           metadata.Kind,
		TrackerRevision:       page.TrackerRevision,
		TrackerSnapshotDigest: snapshotDigest,
		State:                 core.Ready,
		Tasks:                 refs,
	}
	r.SpecRevision = planSpecRevision(r.Root, metadata.Kind, ref, page.TrackerRevision, refs, snapshotDigest)
	return finalizeRun(r)
}

// CreateOneOff creates a trackerless immutable request. Audit and review
// manifests erase writable paths and can never be used to authorize writes.
func CreateOneOff(ctx context.Context, project string, kind OneOffKind, objective string, tasks []core.Task) (Run, error) {
	if err := ctx.Err(); err != nil {
		return Run{}, err
	}
	root, err := canonicalRoot(project)
	if err != nil {
		return Run{}, err
	}
	objective = strings.TrimSpace(objective)
	if objective == "" || !utf8.ValidString(objective) || len(objective) > 256<<10 {
		return Run{}, fmt.Errorf("%w: one-off objective is required and bounded", core.ErrPath)
	}
	if !validOneOffKind(kind) {
		return Run{}, fmt.Errorf("%w: unsupported one-off kind %q", core.ErrSettings, kind)
	}
	tasks = cloneTasks(tasks)
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
	for _, task := range normalized {
		if len(task.Criteria) == 0 {
			return Run{}, fmt.Errorf("%w: one-off task %q has no criteria", core.ErrBatch, task.ID)
		}
	}
	teams, err := planOneOffTeams(kind, normalized)
	if err != nil {
		return Run{}, err
	}
	r := Run{
		RecordEnvelope: core.RecordEnvelope{Schema: 1, Project: root, WrittenAt: canonicalWrittenAt, Revision: 1},
		Root:           root,
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

// ValidateConflict reports whether two proposed admissions cannot run
// independently. Unsafe paths/resources are conflicts too: callers fail
// closed before assigning different teams.
func ValidateConflict(a, b AdmissionBatch) bool {
	return pathsOrResourcesConflict(a.Paths, a.Resources, b.Paths, b.Resources)
}

func taskConflict(a, b core.Task) bool {
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

func pathsOrResourcesConflict(leftPaths, leftResources, rightPaths, rightResources []string) bool {
	leftPaths, err := normalizePaths(leftPaths)
	if err != nil {
		return true
	}
	rightPaths, err = normalizePaths(rightPaths)
	if err != nil {
		return true
	}
	leftResources, err = normalizeResources(leftResources)
	if err != nil {
		return true
	}
	rightResources, err = normalizeResources(rightResources)
	if err != nil {
		return true
	}
	for _, left := range leftPaths {
		for _, right := range rightPaths {
			if pathsOverlap(left, right) {
				return true
			}
		}
	}
	for _, left := range leftResources {
		for _, right := range rightResources {
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
		revision := uint64(1)
		if r.Mode == "plan" && r.Tasks[i].Revision != 0 {
			revision = r.Tasks[i].Revision
		}
		r.Tasks[i].RecordEnvelope = core.RecordEnvelope{Schema: 1, Project: r.Project, RunID: r.ID, WrittenAt: canonicalWrittenAt, Revision: revision}
	}
	for i := range r.Teams {
		r.Teams[i].ID = canonicalTeamID(r.ID, i+1)
		r.Teams[i].RecordEnvelope = core.RecordEnvelope{Schema: 1, Project: r.Project, RunID: r.ID, WrittenAt: canonicalWrittenAt, Revision: 1}
		r.Teams[i].QueueFingerprint = queueFingerprint(r.Teams[i].Queue)
	}
	if err := validateRun(r); err != nil {
		return Run{}, err
	}
	return r, nil
}

func planOneOffTeams(kind OneOffKind, tasks []core.Task) ([]TeamRecord, error) {
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
				if !used[j] && taskConflict(tasks[component[cursor]], tasks[j]) {
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
	for i := range teams {
		paths, resources, err := derivedOneOffAuthority(kind, tasks, teams[i].Queue)
		if err != nil {
			return nil, err
		}
		teams[i].Paths, teams[i].Resources = paths, resources
	}
	return teams, nil
}

// derivedOneOffAuthority is the only source of mutable team scope for a
// trackerless run. It deliberately derives scope from immutable task details,
// rather than accepting authority from an admission caller.
func derivedOneOffAuthority(kind OneOffKind, tasks []core.Task, queue []core.TaskID) ([]string, []string, error) {
	byID := make(map[core.TaskID]core.Task, len(tasks))
	for _, task := range tasks {
		byID[task.ID] = task
	}
	paths := map[string]bool{}
	resources := map[string]bool{}
	seen := map[core.TaskID]bool{}
	for _, id := range queue {
		task, ok := byID[id]
		if !ok || seen[id] {
			return nil, nil, fmt.Errorf("%w: invalid one-off team task %q", core.ErrBatch, id)
		}
		seen[id] = true
		if kind == OneOffFeature {
			for _, value := range task.WritablePaths {
				paths[value] = true
			}
		}
		for _, value := range task.Resources {
			resources[value] = true
		}
	}
	var pathValues []string
	if len(paths) > 0 {
		pathValues = make([]string, 0, len(paths))
	}
	for value := range paths {
		pathValues = append(pathValues, value)
	}
	var resourceValues []string
	if len(resources) > 0 {
		resourceValues = make([]string, 0, len(resources))
	}
	for value := range resources {
		resourceValues = append(resourceValues, value)
	}
	sort.Strings(pathValues)
	sort.Strings(resourceValues)
	return pathValues, resourceValues, nil
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
	var total int
	for _, task := range normalized {
		total += len(task.Objective)
		for _, value := range task.Criteria {
			total += len(value)
		}
		for _, check := range task.Checks {
			total += len(check.Name)
			for _, arg := range check.Command {
				total += len(arg)
			}
		}
		for _, value := range task.WritablePaths {
			total += len(value)
		}
		for _, value := range task.Resources {
			total += len(value)
		}
		for _, value := range task.EvidencePointers {
			total += len(value)
		}
	}
	if total > 256<<10 {
		return nil, fmt.Errorf("%w: aggregate task manifest input exceeds 256 KiB", core.ErrLimit)
	}
	return normalized, nil
}

func planTaskReferences(tasks []core.Task, trackerRevision uint64) []core.Task {
	refs := make([]core.Task, len(tasks))
	for i, task := range tasks {
		revision := task.Revision
		if revision == 0 {
			revision = trackerRevision
		}
		refs[i] = core.Task{RecordEnvelope: core.RecordEnvelope{Schema: 1, Revision: revision}, ID: task.ID}
	}
	return refs
}

func planSpecRevision(root, kind, ref string, trackerRevision uint64, tasks []core.Task, snapshotDigest string) string {
	ids := make([]core.TaskID, len(tasks))
	for i, task := range tasks {
		ids[i] = task.ID
	}
	value := struct {
		Root, Kind, Ref, Snapshot string
		Revision                  uint64
		Tasks                     []core.TaskID
	}{root, kind, ref, snapshotDigest, trackerRevision, ids}
	digest, _ := digestJSON(value)
	return digest
}

func canonicalTrackerRef(root, kind string) (string, error) {
	leaf := "TASKS.md"
	if kind == "beads" {
		leaf = ".beads"
	}
	if kind != "tasks-md" && kind != "beads" {
		return "", fmt.Errorf("%w: unknown tracker kind", core.ErrSettings)
	}
	return canonicalAuthorityRef(root, kind, leaf)
}

func canonicalRoot(root string) (string, error) {
	if strings.TrimSpace(root) == "" {
		return "", fmt.Errorf("%w: empty root", core.ErrPath)
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("%w: root: %v", core.ErrPath, err)
	}
	resolved, err := filepath.EvalSymlinks(filepath.Clean(abs))
	if err != nil {
		return "", fmt.Errorf("%w: resolve root: %v", core.ErrPath, err)
	}
	info, err := os.Stat(resolved)
	if err != nil || !info.IsDir() {
		return "", fmt.Errorf("%w: root is not a directory", core.ErrPath)
	}
	return filepath.Clean(resolved), nil
}

func canonicalAuthorityRef(root, kind, ref string) (string, error) {
	if ref == "" {
		return "", fmt.Errorf("%w: empty tracker ref", core.ErrPath)
	}
	candidate := ref
	if !filepath.IsAbs(candidate) {
		candidate = filepath.Join(root, filepath.FromSlash(candidate))
	}
	resolved, err := filepath.EvalSymlinks(filepath.Clean(candidate))
	if err != nil {
		return "", fmt.Errorf("%w: resolve tracker ref: %v", core.ErrPath, err)
	}
	relative, err := filepath.Rel(root, resolved)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("%w: tracker ref escapes root", core.ErrPath)
	}
	_ = kind
	return filepath.Clean(resolved), nil
}

func cloneTasks(tasks []core.Task) []core.Task {
	out := make([]core.Task, len(tasks))
	for i, task := range tasks {
		out[i] = task
		out[i].Dependencies = append([]core.TaskID(nil), task.Dependencies...)
		out[i].Criteria = append([]string(nil), task.Criteria...)
		out[i].Checks = append([]core.Check(nil), task.Checks...)
		for j := range out[i].Checks {
			out[i].Checks[j].Command = append([]string(nil), task.Checks[j].Command...)
		}
		out[i].WritablePaths = append([]string(nil), task.WritablePaths...)
		out[i].Resources = append([]string(nil), task.Resources...)
		out[i].EvidencePointers = append([]string(nil), task.EvidencePointers...)
	}
	return out
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
	value = strings.ReplaceAll(value, "\\", "/")
	if strings.TrimSpace(value) != value {
		return "", fmt.Errorf("%w: unsafe writable path", core.ErrPath)
	}
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
		if !safePathSegment(part) {
			return "", fmt.Errorf("%w: unsafe writable path %q", core.ErrPath, value)
		}
	}
	if glob {
		return strings.ToLower(clean) + "/**", nil
	}
	return strings.ToLower(clean), nil
}

func safePathSegment(part string) bool {
	if part == "" || part == "." || part == ".." || strings.TrimRight(part, ". ") != part || strings.ContainsAny(part, ":<>\"|?*") {
		return false
	}
	upper := strings.ToUpper(strings.Split(part, ".")[0])
	if upper == "CON" || upper == "PRN" || upper == "AUX" || upper == "NUL" || upper == "CLOCK$" {
		return false
	}
	if len(upper) == 4 && (strings.HasPrefix(upper, "COM") || strings.HasPrefix(upper, "LPT")) && upper[3] >= '1' && upper[3] <= '9' {
		return false
	}
	return true
}

func normalizeResources(values []string) ([]string, error) {
	if len(values) == 0 {
		return nil, nil
	}
	out := make([]string, len(values))
	for i, value := range values {
		value = strings.ReplaceAll(value, "\\", "/")
		if strings.TrimSpace(value) != value {
			return nil, fmt.Errorf("%w: unsafe resource", core.ErrPath)
		}
		value = strings.ToLower(value)
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
	taskIDs := []core.TaskID(nil)
	taskRefs := []struct {
		ID       core.TaskID `json:"id"`
		Revision uint64      `json:"revision"`
	}(nil)
	if r.Mode == "one-off" {
		var err error
		tasks, err = normalizeTasks(r.Tasks)
		if err != nil {
			return "", err
		}
	} else {
		for _, task := range r.Tasks {
			taskIDs = append(taskIDs, task.ID)
			taskRefs = append(taskRefs, struct {
				ID       core.TaskID `json:"id"`
				Revision uint64      `json:"revision"`
			}{task.ID, task.Revision})
		}
	}
	wire := struct {
		Schema                int           `json:"schema"`
		Root                  string        `json:"root"`
		Project               string        `json:"project"`
		Mode                  string        `json:"mode"`
		OneOffKind            OneOffKind    `json:"oneOffKind,omitempty"`
		Objective             string        `json:"objective,omitempty"`
		SpecRevision          string        `json:"specRevision,omitempty"`
		TrackerKind           string        `json:"trackerKind"`
		TrackerRevision       uint64        `json:"trackerRevision"`
		TrackerSnapshotDigest string        `json:"trackerSnapshotDigest,omitempty"`
		TaskIDs               []core.TaskID `json:"taskIds,omitempty"`
		TaskRefs              []struct {
			ID       core.TaskID `json:"id"`
			Revision uint64      `json:"revision"`
		} `json:"taskRefs,omitempty"`
		Tasks []core.Task `json:"tasks,omitempty"`
	}{r.Schema, r.Root, strings.TrimSpace(r.Project), r.Mode, r.OneOffKind, strings.TrimSpace(r.Objective), r.SpecRevision, r.TrackerKind, r.TrackerRevision, r.TrackerSnapshotDigest, taskIDs, taskRefs, tasks}
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

// QueueFingerprint exposes the canonical bounded queue binding for retained
// team transitions without giving callers permission to alter queue rules.
func QueueFingerprint(queue []core.TaskID) string { return queueFingerprint(queue) }

func digestJSON(value any) (string, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("%w: canonical manifest: %v", core.ErrRevision, err)
	}
	sum := sha256.Sum256(raw)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

func admissionFingerprint(batch AdmissionBatch) string {
	value := struct {
		Schema, Revision                         uint64
		Project, RunID, WrittenAt, BatchID, Team string
		Sequence, TrackerRevision                uint64
		Tasks                                    []core.TaskID
		Paths, Resources                         []string
	}{uint64(batch.Schema), batch.Revision, batch.Project, string(batch.RunID), batch.WrittenAt, batch.BatchID, string(batch.Team), batch.Sequence, batch.TrackerRevision, batch.Tasks, batch.Paths, batch.Resources}
	digest, _ := digestJSON(value)
	return digest
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
	if err := validateEnvelope(batch.RecordEnvelope, batch.RunID); err != nil {
		return err
	}
	if err := validateID(batch.BatchID); err != nil {
		return err
	}
	if err := validateID(string(batch.Team)); err != nil {
		return err
	}
	if len(batch.Tasks) == 0 || len(batch.Tasks) > maxTeamQueue {
		return fmt.Errorf("%w: admission has %d tasks", core.ErrBatch, len(batch.Tasks))
	}
	paths, err := normalizePaths(batch.Paths)
	if err != nil || !reflect.DeepEqual(paths, batch.Paths) {
		return fmt.Errorf("%w: invalid admission paths", core.ErrPath)
	}
	resources, err := normalizeResources(batch.Resources)
	if err != nil || !reflect.DeepEqual(resources, batch.Resources) {
		return fmt.Errorf("%w: invalid admission resources", core.ErrPath)
	}
	tasks, err := normalizeIDs(batch.Tasks)
	if err != nil || !reflect.DeepEqual(tasks, batch.Tasks) {
		return fmt.Errorf("%w: invalid admission tasks", core.ErrBatch)
	}
	if batch.Fingerprint != admissionFingerprint(batch) {
		return fmt.Errorf("%w: admission fingerprint mismatch", core.ErrRevision)
	}
	return nil
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
	root, err := canonicalRoot(r.Root)
	if err != nil || root != r.Root || r.Project != r.Root {
		return fmt.Errorf("%w: noncanonical run root", core.ErrRevision)
	}
	if err := validateID(string(r.ID)); err != nil {
		return err
	}
	if err := validateEnvelope(r.RecordEnvelope, r.ID); err != nil {
		return err
	}
	if !validTaskState(r.State) || (r.Mode != "plan" && r.Mode != "one-off") {
		return fmt.Errorf("%w: invalid run state or mode", core.ErrPhase)
	}
	if r.Mode == "plan" && (r.TrackerKind != "tasks-md" && r.TrackerKind != "beads" || r.TrackerRevision == 0 || !validDigest(r.TrackerSnapshotDigest) || !validDigest(r.SpecRevision) || r.OneOffKind != "") {
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
	if r.Mode == "plan" {
		for i, task := range r.Tasks {
			if err := validateID(string(task.ID)); err != nil {
				return err
			}
			if err := validateEnvelope(task.RecordEnvelope, r.ID); err != nil {
				return fmt.Errorf("%w: plan task envelope: %v", core.ErrRevision, err)
			}
			if task.Objective != "" || task.State != "" || len(task.Dependencies) != 0 || len(task.Criteria) != 0 || len(task.Checks) != 0 || len(task.WritablePaths) != 0 || len(task.Resources) != 0 || len(task.EvidencePointers) != 0 || task.Archived || (i > 0 && r.Tasks[i-1].ID >= task.ID) {
				return fmt.Errorf("%w: plan task reference is not minimal", core.ErrRevision)
			}
		}
		ref, err := canonicalTrackerRef(r.Root, r.TrackerKind)
		if err != nil || r.SpecRevision != planSpecRevision(r.Root, r.TrackerKind, ref, r.TrackerRevision, r.Tasks, r.TrackerSnapshotDigest) {
			return fmt.Errorf("%w: plan authority binding mismatch", core.ErrRevision)
		}
	}
	if r.Mode == "one-off" && len(r.Teams) > maxOneOffTeams {
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
		if task.Project != r.Project || task.RunID != r.ID {
			return fmt.Errorf("%w: task belongs to another run", core.ErrRevision)
		}
		byID[task.ID] = task
	}
	for index, team := range r.Teams {
		if team.RunID != r.ID || team.Project != r.Project {
			return fmt.Errorf("%w: team belongs to a different run", core.ErrRevision)
		}
		if team.ID != canonicalTeamID(r.ID, index+1) {
			return fmt.Errorf("%w: noncanonical team slot", core.ErrRevision)
		}
		if err := validateTeam(team); err != nil {
			return err
		}
		if r.Mode == "one-off" {
			paths, resources, err := derivedOneOffAuthority(r.OneOffKind, r.Tasks, team.Queue)
			if err != nil {
				return err
			}
			if !reflect.DeepEqual(paths, team.Paths) || !reflect.DeepEqual(resources, team.Resources) {
				return fmt.Errorf("%w: one-off team authority is not derived from its queue", core.ErrRevision)
			}
		}
		for _, id := range team.Queue {
			if seen[id] || byID[id].ID == "" {
				return fmt.Errorf("%w: invalid team task %q", core.ErrBatch, id)
			}
			seen[id] = true
		}
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
	if !isCanonicalTeamID(team.ID, team.RunID) {
		return fmt.Errorf("%w: noncanonical team ID", core.ErrRevision)
	}
	if !validTeamState(team.State) || len(team.Queue) > maxTeamQueue {
		return fmt.Errorf("%w: invalid team record", core.ErrPhase)
	}
	paths, err := normalizePaths(team.Paths)
	if err != nil || !reflect.DeepEqual(paths, team.Paths) {
		return fmt.Errorf("%w: invalid team writable paths", core.ErrPath)
	}
	resources, err := normalizeResources(team.Resources)
	if err != nil || !reflect.DeepEqual(resources, team.Resources) {
		return fmt.Errorf("%w: invalid team resources", core.ErrPath)
	}
	if len(team.Queue) == 0 {
		if team.State != core.Idle || (team.QueueFingerprint != "" && team.QueueFingerprint != queueFingerprint(nil)) || team.IntentDigest != "" || team.Handle.Identity != "" || team.CompletedTask != "" || team.ReviewedTask != "" || team.HostIdle {
			return fmt.Errorf("%w: invalid empty team queue", core.ErrRevision)
		}
		if team.RetainedHandle.Identity != "" && (team.RetainedHandle.Run != team.RunID || team.RetainedHandle.Team != team.ID || team.RetainedHandle.Task == "" || team.RetainedHandle.Reviewer || !validDigest(team.RetainedHandle.PacketDigest)) {
			return fmt.Errorf("%w: invalid retained idle host", core.ErrRevision)
		}
		return nil
	}
	if team.QueueFingerprint != queueFingerprint(team.Queue) {
		return fmt.Errorf("%w: invalid team queue", core.ErrRevision)
	}
	for _, id := range team.Queue {
		if err := validateID(string(id)); err != nil {
			return err
		}
	}
	if team.IntentDigest == "" {
		if team.Handle.Identity != "" || team.CompletedTask != "" || team.ReviewedTask != "" || team.HostIdle {
			return fmt.Errorf("%w: team intent state", core.ErrRevision)
		}
		if team.RetainedHandle.Identity != "" && (len(team.Queue) != 1 || team.State != core.Working || team.RetainedHandle.Run != team.RunID || team.RetainedHandle.Team != team.ID || team.RetainedHandle.Task == "" || team.RetainedHandle.Reviewer || !validDigest(team.RetainedHandle.PacketDigest)) {
			return fmt.Errorf("%w: retained reservation state", core.ErrRevision)
		}
		return nil
	}
	if !validDigest(team.IntentDigest) {
		return fmt.Errorf("%w: team host intent", core.ErrRevision)
	}
	if team.Handle.Identity == "" {
		if team.CompletedTask != "" || team.ReviewedTask != "" || team.HostIdle {
			return fmt.Errorf("%w: team completion without acknowledgement", core.ErrRevision)
		}
		if team.RetainedHandle.Identity != "" && (team.RetainedHandle.Run != team.RunID || team.RetainedHandle.Team != team.ID || team.RetainedHandle.Task == "" || team.RetainedHandle.Reviewer || !validDigest(team.RetainedHandle.PacketDigest)) {
			return fmt.Errorf("%w: retained team host handle", core.ErrRevision)
		}
		return nil
	}
	if team.Handle.Run != team.RunID || team.Handle.Team != team.ID || team.Handle.Task != team.Queue[0] || team.Handle.Reviewer || team.Handle.PacketDigest != team.IntentDigest {
		return fmt.Errorf("%w: team host handle", core.ErrRevision)
	}
	if team.RetainedHandle.Identity != "" && (team.RetainedHandle.Host != team.Handle.Host || team.RetainedHandle.Identity != team.Handle.Identity) {
		return fmt.Errorf("%w: retained team host identity", core.ErrRevision)
	}
	if (team.CompletedTask != "" && team.CompletedTask != team.Queue[0]) || (team.ReviewedTask != "" && team.ReviewedTask != team.Queue[0]) || team.HostIdle && (team.CompletedTask == "" || team.ReviewedTask == "") {
		return fmt.Errorf("%w: team completion state", core.ErrRevision)
	}
	return nil
}

func canonicalTeamID(runID core.RunID, ordinal int) core.TeamID {
	return core.TeamID(fmt.Sprintf("%s-team-%d", runID, ordinal))
}

func isCanonicalTeamID(teamID core.TeamID, runID core.RunID) bool {
	prefix := string(runID) + "-team-"
	if !strings.HasPrefix(string(teamID), prefix) {
		return false
	}
	ordinal, err := strconv.Atoi(strings.TrimPrefix(string(teamID), prefix))
	return err == nil && ordinal > 0 && string(teamID) == string(canonicalTeamID(runID, ordinal))
}
