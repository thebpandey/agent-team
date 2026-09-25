// Package admission appends revision-checked team admissions.
package admission

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/thebpandey/agent-team/vnext/internal/core"
	"github.com/thebpandey/agent-team/vnext/internal/run"
	"github.com/thebpandey/agent-team/vnext/internal/store"
	"github.com/thebpandey/agent-team/vnext/internal/tracker"
)

const maxRecordBytes int64 = 16 << 20

type OutcomeKind string

const (
	Created   OutcomeKind = "created"
	Duplicate OutcomeKind = "duplicate"
	Stale     OutcomeKind = "stale"
)

type AdmissionOutcome struct {
	Kind         OutcomeKind
	Run          core.RunRecord
	TeamRevision uint64
	Fingerprint  string
	Warning      string
}

// committedAdmission is both the append-only history item and the sole commit
// authority. It carries an immutable before/after bundle so a process stopped
// between projection publications can deterministically converge on restart.
// It is intentionally not a lock, lease, PID, or second mutable ledger.
type committedAdmission struct {
	Schema                  int                `json:"schema"`
	Batch                   run.AdmissionBatch `json:"batch"`
	ExpectedRunRevision     uint64             `json:"expectedRunRevision"`
	ExpectedTeamRevision    uint64             `json:"expectedTeamRevision"`
	ExpectedTrackerRevision uint64             `json:"expectedTrackerRevision"`
	BeforeRun               run.Run            `json:"beforeRun"`
	BeforeTeam              run.TeamRecord     `json:"beforeTeam"`
	AfterRun                run.Run            `json:"afterRun"`
	AfterTeam               run.TeamRecord     `json:"afterTeam"`
	Warning                 string             `json:"warning,omitempty"`
}

var admissionLocks sync.Map // map[string]*sync.Mutex, process-local serialization only

// admissionFault is a deliberately narrow test seam for interruption at each
// durable boundary. Production leaves it nil. The commit itself remains the
// only recovery authority; the seam never changes what is written.
type admissionFaultPoint uint8

const (
	faultBeforeCommit admissionFaultPoint = iota
	faultAfterCommit
	faultAfterRunProjection
	faultAfterTeamProjection
	faultAfterCommitLookup
)

var admissionFault func(admissionFaultPoint) error

func failAdmission(point admissionFaultPoint) error {
	if admissionFault == nil {
		return nil
	}
	return admissionFault(point)
}

// AppendAdmission validates a single tracker snapshot, writes one immutable
// admission commit, and then projects its prepared run/team state. A committed
// record is never reported as success until both projections exist.
func AppendAdmission(ctx context.Context, st *store.Store, tr tracker.Tracker, runID core.RunID, expectedRunRevision uint64, teamID core.TeamID, expectedTeamRevision uint64, expectedTrackerRevision uint64, expectedTaskRevisions map[core.TaskID]uint64, batch run.AdmissionBatch) (AdmissionOutcome, error) {
	if err := ctx.Err(); err != nil {
		return AdmissionOutcome{}, err
	}
	if st == nil || tr == nil {
		return AdmissionOutcome{}, fmt.Errorf("%w: store and tracker are required", core.ErrSettings)
	}
	if err := validateID(string(runID)); err != nil {
		return AdmissionOutcome{}, err
	}
	if err := validateID(string(teamID)); err != nil {
		return AdmissionOutcome{}, err
	}
	if err := validateBatch(batch, runID, teamID); err != nil {
		return AdmissionOutcome{}, err
	}
	root, err := canonicalRoot(st.Root)
	if err != nil {
		return AdmissionOutcome{}, err
	}
	lockValue, _ := admissionLocks.LoadOrStore(root, &sync.Mutex{})
	mu := lockValue.(*sync.Mutex)
	mu.Lock()
	defer mu.Unlock()
	if err := ctx.Err(); err != nil {
		return AdmissionOutcome{}, err
	}
	repositories := run.NewRepositories(st)

	commitPath := runCommitPath(runID, expectedRunRevision)
	if existing, found, err := readCommit(st, commitPath); err != nil {
		return AdmissionOutcome{}, err
	} else if found {
		if err := validateCommit(existing, runID, expectedRunRevision); err != nil {
			return AdmissionOutcome{}, err
		}
		// A durable commit is authoritative even for a conflicting retry. Finish
		// its canonical projections before returning Duplicate or Stale.
		if err := project(ctx, repositories, existing); err != nil {
			return AdmissionOutcome{}, err
		}
		if existing.Batch.Fingerprint != batch.Fingerprint || !reflect.DeepEqual(existing.Batch, batch) {
			// The run-revision slot belongs to the already committed admission.
			// A different proposal is stale, while an exact proposal below is a
			// duplicate and must first finish that commit's projection.
			return staleOutcome(ctx, repositories, runID, teamID, batch.Fingerprint)
		}
		if existing.ExpectedRunRevision == expectedRunRevision && existing.ExpectedTeamRevision == expectedTeamRevision && existing.ExpectedTrackerRevision == expectedTrackerRevision {
			return outcome(Duplicate, existing), nil
		}
		return staleOutcome(ctx, repositories, runID, teamID, batch.Fingerprint)
	}
	if err := failAdmission(faultAfterCommitLookup); err != nil {
		return AdmissionOutcome{}, err
	}
	currentRun, err := repositories.Runs.Read(ctx, runID)
	if err != nil {
		return AdmissionOutcome{}, err
	}
	currentTeam, err := repositories.Teams.Read(ctx, teamID)
	if err != nil {
		return AdmissionOutcome{}, err
	}
	if currentRun.ID != runID || currentTeam.ID != teamID {
		return AdmissionOutcome{}, fmt.Errorf("%w: noncanonical run/team slot", core.ErrRevision)
	}
	// An earlier durable commit may have stopped before either projection. It
	// must converge before this caller is classified as stale.
	if err := recoverPrevious(ctx, st, repositories, runID, expectedRunRevision); err != nil {
		return AdmissionOutcome{}, err
	}
	currentRun, err = repositories.Runs.Read(ctx, runID)
	if err != nil {
		return AdmissionOutcome{}, err
	}
	currentTeam, err = repositories.Teams.Read(ctx, teamID)
	if err != nil {
		return AdmissionOutcome{}, err
	}
	if currentRun.ID != runID || currentTeam.ID != teamID {
		return AdmissionOutcome{}, fmt.Errorf("%w: noncanonical run/team slot", core.ErrRevision)
	}
	if currentRun.Revision != expectedRunRevision || currentTeam.Revision != expectedTeamRevision {
		if existing, found, err := readCommit(st, commitPath); err != nil {
			return AdmissionOutcome{}, err
		} else if found {
			if err := validateCommit(existing, runID, expectedRunRevision); err != nil {
				return AdmissionOutcome{}, err
			}
			if err := project(ctx, repositories, existing); err != nil {
				return AdmissionOutcome{}, err
			}
			if existing.Batch.Fingerprint == batch.Fingerprint && reflect.DeepEqual(existing.Batch, batch) && existing.ExpectedTeamRevision == expectedTeamRevision && existing.ExpectedTrackerRevision == expectedTrackerRevision {
				return outcome(Duplicate, existing), nil
			}
			return staleOutcome(ctx, repositories, runID, teamID, batch.Fingerprint)
		}
		return outcome(Stale, committedAdmission{Batch: batch, AfterRun: currentRun, AfterTeam: currentTeam}), nil
	}
	if batch.Project != currentRun.Project || currentTeam.Project != currentRun.Project || currentTeam.RunID != runID {
		return AdmissionOutcome{}, fmt.Errorf("%w: admission project or team binding", core.ErrRevision)
	}
	if err := rejectReusedFingerprint(st, teamID, batch); err != nil {
		return AdmissionOutcome{}, err
	}

	// Exactly one bounded tracker snapshot is taken before any publication.
	page, err := tr.Page(ctx, "", 1000)
	if err != nil {
		return AdmissionOutcome{}, err
	}
	if page.TotalNonArchived > 1000 {
		return AdmissionOutcome{}, fmt.Errorf("%w: tracker capacity", core.ErrCapacity)
	}
	if page.Cursor != "" || page.TotalNonArchived != len(page.Tasks) || page.TotalNonArchived < 0 {
		return AdmissionOutcome{}, fmt.Errorf("%w: incomplete tracker snapshot", core.ErrRevision)
	}
	if page.TrackerRevision != expectedTrackerRevision || batch.TrackerRevision != expectedTrackerRevision {
		return outcome(Stale, committedAdmission{Batch: batch, AfterRun: currentRun, AfterTeam: currentTeam}), nil
	}
	selected, err := validateSelectedTasks(page.Tasks, batch, expectedTaskRevisions)
	if err != nil {
		return AdmissionOutcome{}, err
	}
	paths, resources := taskAuthority(selected)
	paths, resources, err = run.NormalizeAuthority(paths, resources)
	if err != nil {
		return AdmissionOutcome{}, err
	}
	if !reflect.DeepEqual(paths, batch.Paths) || !reflect.DeepEqual(resources, batch.Resources) {
		return AdmissionOutcome{}, fmt.Errorf("%w: admission scope is not tracker-derived", core.ErrRevision)
	}
	if len(currentTeam.Queue)+len(batch.Tasks) > 8 {
		return AdmissionOutcome{}, fmt.Errorf("%w: team queue would exceed 8", core.ErrBatch)
	}
	if err := checkConflicts(st, currentRun, teamID, batch); err != nil {
		return AdmissionOutcome{}, err
	}

	afterRun, afterTeam, err := projectedAdmission(currentRun, currentTeam, batch)
	if err != nil {
		return AdmissionOutcome{}, err
	}
	warning := ""
	if page.TotalNonArchived >= 900 {
		warning = fmt.Sprintf("tracker has %d of %d non-archived tasks", page.TotalNonArchived, 1000)
	}
	commit := committedAdmission{Schema: 1, Batch: batch, ExpectedRunRevision: expectedRunRevision, ExpectedTeamRevision: expectedTeamRevision, ExpectedTrackerRevision: expectedTrackerRevision, BeforeRun: currentRun, BeforeTeam: currentTeam, AfterRun: afterRun, AfterTeam: afterTeam, Warning: warning}
	if err := validateCommit(commit, runID, expectedRunRevision); err != nil {
		return AdmissionOutcome{}, err
	}
	if err := ctx.Err(); err != nil {
		return AdmissionOutcome{}, err
	}
	if err := failAdmission(faultBeforeCommit); err != nil {
		return AdmissionOutcome{}, err
	}
	if _, err := st.CreateJSON(commitPath, commit, maxRecordBytes); err != nil {
		if !errors.Is(err, fs.ErrExist) {
			return AdmissionOutcome{}, err
		}
		winner, found, readErr := readCommit(st, commitPath)
		if readErr != nil || !found {
			return AdmissionOutcome{}, fmt.Errorf("%w: concurrent admission commit unreadable: %v", core.ErrRevision, readErr)
		}
		if err := validateCommit(winner, runID, expectedRunRevision); err != nil {
			return AdmissionOutcome{}, err
		}
		if winner.Batch.Fingerprint == batch.Fingerprint && reflect.DeepEqual(winner.Batch, batch) {
			if err := project(ctx, repositories, winner); err != nil {
				return AdmissionOutcome{}, err
			}
			return outcome(Duplicate, winner), nil
		}
		return staleOutcome(ctx, repositories, runID, teamID, batch.Fingerprint)
	}
	if err := failAdmission(faultAfterCommit); err != nil {
		return AdmissionOutcome{}, err
	}
	if err := project(ctx, repositories, commit); err != nil {
		return AdmissionOutcome{}, err
	}
	// The per-team record is immutable audit history; it is not read as active
	// coordination authority and therefore cannot retain released scope.
	if _, err := st.CreateJSON(admissionPath(teamID, batch.BatchID), commit, maxRecordBytes); err != nil && !errors.Is(err, fs.ErrExist) {
		return AdmissionOutcome{}, err
	}
	return outcome(Created, commit), nil
}

func project(ctx context.Context, repositories run.Repositories, commit committedAdmission) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	plan, err := preflightProjection(ctx, repositories, commit)
	if err != nil {
		return err
	}
	if err := convergeRun(ctx, repositories, commit, plan); err != nil {
		return err
	}
	if err := failAdmission(faultAfterRunProjection); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	// A competing process can have completed the run projection. Check both
	// records again before the team CAS; this never accepts a third state.
	plan, err = preflightProjection(ctx, repositories, commit)
	if err != nil {
		return err
	}
	if err := convergeTeam(ctx, repositories, commit, plan); err != nil {
		return err
	}
	return failAdmission(faultAfterTeamProjection)
}

type projectionPlan struct {
	run  run.Run
	team run.TeamRecord
}

// preflightProjection validates both canonical records before any projection
// write. Each may only be this commit's exact before or exact after state.
func preflightProjection(ctx context.Context, repositories run.Repositories, commit committedAdmission) (projectionPlan, error) {
	expectedRun, expectedTeam, err := projectedAdmission(commit.BeforeRun, commit.BeforeTeam, commit.Batch)
	if err != nil || !reflect.DeepEqual(expectedRun, commit.AfterRun) || !reflect.DeepEqual(expectedTeam, commit.AfterTeam) {
		return projectionPlan{}, fmt.Errorf("%w: invalid committed projection", core.ErrRevision)
	}
	currentRun, err := repositories.Runs.Read(ctx, commit.AfterRun.ID)
	if err != nil {
		return projectionPlan{}, err
	}
	if !reflect.DeepEqual(currentRun, commit.BeforeRun) && !reflect.DeepEqual(currentRun, expectedRun) {
		return projectionPlan{}, fmt.Errorf("%w: committed run projection preflight conflicts", core.ErrRevision)
	}
	currentTeam, err := repositories.Teams.Read(ctx, commit.AfterTeam.ID)
	if err != nil {
		return projectionPlan{}, err
	}
	if !reflect.DeepEqual(currentTeam, commit.BeforeTeam) && !reflect.DeepEqual(currentTeam, expectedTeam) {
		return projectionPlan{}, fmt.Errorf("%w: committed team projection preflight conflicts", core.ErrRevision)
	}
	return projectionPlan{run: expectedRun, team: expectedTeam}, nil
}

func convergeRun(ctx context.Context, repositories run.Repositories, commit committedAdmission, plan projectionPlan) error {
	current, err := repositories.Runs.Read(ctx, commit.AfterRun.ID)
	if err != nil {
		return err
	}
	if reflect.DeepEqual(current, plan.run) {
		return nil
	}
	if !reflect.DeepEqual(current, commit.BeforeRun) {
		return fmt.Errorf("%w: committed run projection conflicts", core.ErrRevision)
	}
	_, err = repositories.Runs.CompareAndSwap(ctx, current.ID, current.Revision, plan.run)
	if err != nil {
		// Repository CAS is process-local; another process may have projected
		// this exact immutable commit after our read. Re-read canonical state
		// before treating that race as a conflict.
		if observed, readErr := repositories.Runs.Read(ctx, commit.AfterRun.ID); readErr == nil && reflect.DeepEqual(observed, plan.run) {
			return nil
		}
	}
	return err
}

func convergeTeam(ctx context.Context, repositories run.Repositories, commit committedAdmission, plan projectionPlan) error {
	current, err := repositories.Teams.Read(ctx, commit.AfterTeam.ID)
	if err != nil {
		return err
	}
	if reflect.DeepEqual(current, plan.team) {
		return nil
	}
	if !reflect.DeepEqual(current, commit.BeforeTeam) {
		return fmt.Errorf("%w: committed team projection conflicts", core.ErrRevision)
	}
	_, err = repositories.Teams.CompareAndSwap(ctx, current.ID, current.Revision, plan.team)
	if err != nil {
		if observed, readErr := repositories.Teams.Read(ctx, commit.AfterTeam.ID); readErr == nil && reflect.DeepEqual(observed, plan.team) {
			return nil
		}
	}
	return err
}

func staleOutcome(ctx context.Context, repositories run.Repositories, runID core.RunID, teamID core.TeamID, fingerprint string) (AdmissionOutcome, error) {
	r, err := repositories.Runs.Read(ctx, runID)
	if err != nil {
		return AdmissionOutcome{}, err
	}
	t, err := repositories.Teams.Read(ctx, teamID)
	if err != nil {
		return AdmissionOutcome{}, err
	}
	return outcome(Stale, committedAdmission{Batch: run.AdmissionBatch{Fingerprint: fingerprint}, AfterRun: r, AfterTeam: t}), nil
}

func outcome(kind OutcomeKind, commit committedAdmission) AdmissionOutcome {
	r := commit.AfterRun
	return AdmissionOutcome{Kind: kind, Run: core.RunRecord{RecordEnvelope: r.RecordEnvelope, ID: r.ID, SpecRevision: r.SpecRevision, TrackerKind: r.TrackerKind, TrackerRevision: r.TrackerRevision, CanonicalRevision: r.CanonicalRevision, State: r.State}, TeamRevision: commit.AfterTeam.Revision, Fingerprint: commit.Batch.Fingerprint, Warning: commit.Warning}
}

func readCommit(st *store.Store, relative string) (committedAdmission, bool, error) {
	var commit committedAdmission
	err := st.ReadJSON(relative, maxRecordBytes, &commit)
	if errors.Is(err, fs.ErrNotExist) || errors.Is(err, os.ErrNotExist) {
		return committedAdmission{}, false, nil
	}
	if err != nil {
		return committedAdmission{}, false, err
	}
	if commit.Schema != 1 {
		return committedAdmission{}, false, fmt.Errorf("%w: invalid admission commit", core.ErrRevision)
	}
	return commit, true, nil
}

func recoverPrevious(ctx context.Context, st *store.Store, repositories run.Repositories, runID core.RunID, expected uint64) error {
	if expected == 0 {
		return nil
	}
	commit, found, err := readCommit(st, runCommitPath(runID, expected-1))
	if err != nil || !found {
		return err
	}
	if err := validateCommit(commit, runID, expected-1); err != nil {
		return err
	}
	return project(ctx, repositories, commit)
}

func validateCommit(commit committedAdmission, runID core.RunID, expected uint64) error {
	if commit.Schema != 1 || commit.ExpectedRunRevision != expected || commit.BeforeRun.ID != runID || commit.AfterRun.ID != runID || commit.Batch.RunID != runID || commit.Batch.Project != commit.BeforeRun.Project || commit.BeforeTeam.Project != commit.BeforeRun.Project || commit.AfterRun.Project != commit.BeforeRun.Project || commit.AfterTeam.Project != commit.BeforeRun.Project || commit.ExpectedTeamRevision != commit.BeforeTeam.Revision || commit.ExpectedTrackerRevision != commit.Batch.TrackerRevision {
		return fmt.Errorf("%w: invalid admission commit", core.ErrRevision)
	}
	if commit.AfterRun.Revision != commit.BeforeRun.Revision+1 || commit.AfterTeam.Revision != commit.BeforeTeam.Revision+1 || commit.BeforeRun.Revision != expected || commit.BeforeTeam.ID != commit.AfterTeam.ID || commit.BeforeTeam.RunID != runID || commit.AfterTeam.RunID != runID || commit.BeforeRun.Schema != 1 || commit.AfterRun.Schema != 1 || commit.BeforeTeam.Schema != 1 || commit.AfterTeam.Schema != 1 || commit.BeforeRun.RunID != runID || commit.AfterRun.RunID != runID || commit.BeforeTeam.WrittenAt != commit.BeforeRun.WrittenAt || commit.AfterTeam.WrittenAt != commit.BeforeRun.WrittenAt {
		return fmt.Errorf("%w: invalid admission commit revisions", core.ErrRevision)
	}
	if _, err := time.Parse(time.RFC3339, commit.BeforeRun.WrittenAt); err != nil || commit.BeforeRun.Root != commit.BeforeRun.Project {
		return fmt.Errorf("%w: invalid admission commit envelope", core.ErrRevision)
	}
	root, err := canonicalRoot(commit.BeforeRun.Root)
	if err != nil || root != commit.BeforeRun.Root {
		return fmt.Errorf("%w: invalid admission commit root", core.ErrRevision)
	}
	if err := validateBatch(commit.Batch, runID, commit.BeforeTeam.ID); err != nil {
		return fmt.Errorf("%w: invalid admission commit batch: %v", core.ErrRevision, err)
	}
	if !teamInRun(commit.BeforeRun, commit.BeforeTeam) {
		return fmt.Errorf("%w: invalid admission commit projection: team is not in before run", core.ErrRevision)
	}
	var slot bool
	for _, team := range commit.BeforeRun.Teams {
		if team.ID == commit.BeforeTeam.ID && reflect.DeepEqual(team, commit.BeforeTeam) {
			slot = true
			break
		}
	}
	if !slot {
		return fmt.Errorf("%w: invalid admission commit slot", core.ErrRevision)
	}
	_, expectedTeam, err := projectedAdmission(commit.BeforeRun, commit.BeforeTeam, commit.Batch)
	if err != nil {
		return fmt.Errorf("%w: invalid admission commit projection", core.ErrRevision)
	}
	if !reflect.DeepEqual(expectedTeam, commit.AfterTeam) {
		return fmt.Errorf("%w: invalid admission commit team projection", core.ErrRevision)
	}
	expectedRun, _, err := projectedAdmission(commit.BeforeRun, commit.BeforeTeam, commit.Batch)
	if err != nil {
		return fmt.Errorf("%w: invalid admission commit projection", core.ErrRevision)
	}
	if !reflect.DeepEqual(expectedRun, commit.AfterRun) {
		return fmt.Errorf("%w: invalid admission commit run projection", core.ErrRevision)
	}
	return nil
}

// projectedAdmission computes the only permitted projection from a canonical
// before-state. Callers compare its result with the durable commit rather than
// treating embedded after-state as authority.
func projectedAdmission(beforeRun run.Run, beforeTeam run.TeamRecord, batch run.AdmissionBatch) (run.Run, run.TeamRecord, error) {
	slot := -1
	for i, team := range beforeRun.Teams {
		if team.ID == beforeTeam.ID {
			if !reflect.DeepEqual(team, beforeTeam) {
				return run.Run{}, run.TeamRecord{}, fmt.Errorf("%w: team differs from canonical run slot", core.ErrRevision)
			}
			slot = i
			break
		}
	}
	if slot < 0 {
		return run.Run{}, run.TeamRecord{}, fmt.Errorf("%w: team is not a canonical run slot", core.ErrRevision)
	}
	afterTeam := cloneTeam(beforeTeam)
	afterTeam.Queue = append(afterTeam.Queue, batch.Tasks...)
	afterTeam.Paths = union(afterTeam.Paths, batch.Paths)
	afterTeam.Resources = union(afterTeam.Resources, batch.Resources)
	afterTeam.QueueFingerprint = queueFingerprint(afterTeam.Queue)
	afterTeam.State = core.Working
	afterTeam.Revision++
	afterRun := cloneRun(beforeRun)
	afterRun.Teams[slot] = afterTeam
	afterRun.Revision++
	return afterRun, afterTeam, nil
}

func rejectReusedFingerprint(st *store.Store, team core.TeamID, batch run.AdmissionBatch) error {
	directory := filepath.Join(st.Root, ".agent-team", "admissions", string(team))
	entries, err := os.ReadDir(directory)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("%w: admission directory: %v", core.ErrPath, err)
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		id := strings.TrimSuffix(entry.Name(), ".json")
		if err := validateID(id); err != nil {
			return fmt.Errorf("%w: unsafe admission entry", core.ErrPath)
		}
		other, found, err := readCommit(st, admissionPath(team, id))
		if err != nil {
			return err
		}
		if found && other.Batch.Fingerprint == batch.Fingerprint {
			return fmt.Errorf("%w: fingerprint already belongs to %q", core.ErrRevision, id)
		}
	}
	return nil
}

func checkConflicts(st *store.Store, current run.Run, teamID core.TeamID, batch run.AdmissionBatch) error {
	for _, team := range current.Teams {
		if team.ID == teamID || team.State == core.Idle || team.State == core.Cancelled {
			continue
		}
		if run.ValidateConflict(batch, run.AdmissionBatch{Paths: team.Paths, Resources: team.Resources}) {
			return fmt.Errorf("%w: active team scope conflict", core.ErrBatch)
		}
	}
	return nil
}

func validateSelectedTasks(tasks []core.Task, batch run.AdmissionBatch, expected map[core.TaskID]uint64) ([]core.Task, error) {
	byID := make(map[core.TaskID]core.Task, len(tasks))
	for _, task := range tasks {
		byID[task.ID] = task
	}
	selected := make([]core.Task, 0, len(batch.Tasks))
	for _, id := range batch.Tasks {
		task, ok := byID[id]
		if !ok || task.Archived || task.State != core.Ready {
			return nil, fmt.Errorf("%w: task %q is not ready", core.ErrPhase, id)
		}
		if got, ok := expected[id]; !ok || got != task.Revision {
			return nil, fmt.Errorf("%w: task %q revision", core.ErrRevision, id)
		}
		for _, dep := range task.Dependencies {
			prerequisite, ok := byID[dep]
			if !ok || (prerequisite.State != core.Clean && prerequisite.State != core.Gated && prerequisite.State != core.Integrated) {
				return nil, fmt.Errorf("%w: dependency %q", core.ErrPhase, dep)
			}
		}
		selected = append(selected, task)
	}
	return selected, nil
}

func validateBatch(batch run.AdmissionBatch, runID core.RunID, teamID core.TeamID) error {
	if batch.Schema != 1 || batch.Revision == 0 || batch.RunID != runID || batch.Team != teamID || batch.Project == "" || !utf8.ValidString(batch.Project) || batch.WrittenAt == "" {
		return fmt.Errorf("%w: invalid admission envelope", core.ErrRevision)
	}
	if _, err := time.Parse(time.RFC3339, batch.WrittenAt); err != nil {
		return fmt.Errorf("%w: invalid admission timestamp", core.ErrRevision)
	}
	if err := validateID(batch.BatchID); err != nil {
		return err
	}
	if len(batch.Tasks) < 1 || len(batch.Tasks) > 8 {
		return fmt.Errorf("%w: admission has %d tasks", core.ErrBatch, len(batch.Tasks))
	}
	if !sortedUniqueIDs(batch.Tasks) {
		return fmt.Errorf("%w: invalid admission task IDs", core.ErrBatch)
	}
	if !sortedUnique(batch.Paths, false) || !sortedUnique(batch.Resources, true) {
		return fmt.Errorf("%w: invalid admission scope", core.ErrPath)
	}
	if batch.Fingerprint != fingerprint(batch) {
		return fmt.Errorf("%w: admission fingerprint mismatch", core.ErrRevision)
	}
	return nil
}

func taskAuthority(tasks []core.Task) ([]string, []string) {
	paths := make([]string, 0)
	resources := make([]string, 0)
	for _, task := range tasks {
		paths = append(paths, task.WritablePaths...)
		resources = append(resources, task.Resources...)
	}
	return union(nil, paths), union(nil, resources)
}
func union(left, right []string) []string {
	values := append(append([]string(nil), left...), right...)
	if len(values) == 0 {
		return nil
	}
	slices.Sort(values)
	return slices.Compact(values)
}
func sortedUniqueIDs(values []core.TaskID) bool {
	for i, value := range values {
		if validateID(string(value)) != nil || (i > 0 && values[i-1] >= value) {
			return false
		}
	}
	return true
}
func sortedUnique(values []string, resource bool) bool {
	for i, value := range values {
		if value == "" || !utf8.ValidString(value) || strings.ContainsAny(value, "\x00\r\n") || (!resource && (strings.HasPrefix(value, "/") || strings.Contains(value, ".."))) || (i > 0 && values[i-1] >= value) {
			return false
		}
	}
	return true
}
func validateID(value string) error {
	if value == "" || len(value) > 128 || !utf8.ValidString(value) || value == "." || value == ".." {
		return fmt.Errorf("%w: unsafe ID", core.ErrPath)
	}
	for _, r := range value {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' || r == '.') {
			return fmt.Errorf("%w: unsafe ID", core.ErrPath)
		}
	}
	return nil
}
func canonicalRoot(value string) (string, error) {
	if value == "" {
		return "", fmt.Errorf("%w: empty store root", core.ErrPath)
	}
	absolute, err := filepath.Abs(value)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(resolved)
	if err != nil || !info.IsDir() {
		return "", fmt.Errorf("%w: invalid store root", core.ErrPath)
	}
	return resolved, nil
}
func teamInRun(value run.Run, team run.TeamRecord) bool {
	for _, slot := range value.Teams {
		if slot.ID == team.ID && slot.RunID == team.RunID && slot.Project == team.Project {
			return true
		}
	}
	return false
}

func cloneRun(value run.Run) run.Run {
	value.Tasks = append([]core.Task(nil), value.Tasks...)
	teams := value.Teams
	value.Teams = make([]run.TeamRecord, len(teams))
	for i := range teams {
		value.Teams[i] = cloneTeam(teams[i])
	}
	return value
}

func cloneTeam(value run.TeamRecord) run.TeamRecord {
	value.Queue = append([]core.TaskID(nil), value.Queue...)
	value.Paths = append([]string(nil), value.Paths...)
	value.Resources = append([]string(nil), value.Resources...)
	return value
}
func runPath(id core.RunID) string   { return ".agent-team/runs/" + string(id) + ".json" }
func teamPath(id core.TeamID) string { return ".agent-team/teams/" + string(id) + ".json" }
func admissionPath(team core.TeamID, batch string) string {
	return ".agent-team/admissions/" + string(team) + "/" + batch + ".json"
}
func runCommitPath(runID core.RunID, revision uint64) string {
	return ".agent-team/admissions/by-run/" + string(runID) + "/" + fmt.Sprintf("%d.json", revision)
}
func queueFingerprint(queue []core.TaskID) string {
	raw, _ := json.Marshal(queue)
	sum := sha256.Sum256(raw)
	return "sha256:" + hex.EncodeToString(sum[:])
}
func fingerprint(batch run.AdmissionBatch) string {
	value := struct {
		Schema, Revision                         uint64
		Project, RunID, WrittenAt, BatchID, Team string
		Sequence, TrackerRevision                uint64
		Tasks                                    []core.TaskID
		Paths, Resources                         []string
	}{uint64(batch.Schema), batch.Revision, batch.Project, string(batch.RunID), batch.WrittenAt, batch.BatchID, string(batch.Team), batch.Sequence, batch.TrackerRevision, batch.Tasks, batch.Paths, batch.Resources}
	raw, _ := json.Marshal(value)
	sum := sha256.Sum256(raw)
	return "sha256:" + hex.EncodeToString(sum[:])
}
