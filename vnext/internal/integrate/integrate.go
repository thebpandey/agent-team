// Package integrate serially records accepted candidate integrations.
package integrate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"sync"
	"time"

	"github.com/thebpandey/agent-team/vnext/internal/contracts"
	"github.com/thebpandey/agent-team/vnext/internal/core"
	"github.com/thebpandey/agent-team/vnext/internal/gate"
	"github.com/thebpandey/agent-team/vnext/internal/project"
	"github.com/thebpandey/agent-team/vnext/internal/run"
	"github.com/thebpandey/agent-team/vnext/internal/store"
)

const evidenceLimit = 64 << 10

// Integrator serializes accepted candidate integrations.
type Integrator interface {
	Integrate(context.Context, contracts.Candidate, contracts.GateResult) (Integration, error)
}

// Integration is the durable record of one exact accepted candidate merge.
type Integration struct {
	core.RecordEnvelope
	Task            core.TaskID
	Base            string
	Candidate       string
	Commit          string
	Order           int
	EvidencePointer string
}

type serialIntegrator struct {
	project project.Project
	store   *store.Store
	manager contracts.WorktreeManager
	guard   *integrationGuard
}

type integrationGuard struct {
	mu sync.Mutex
}

var integrationGuards sync.Map

type evidenceRecord struct {
	Candidate   contracts.Candidate  `json:"candidate"`
	Gate        contracts.GateResult `json:"gate"`
	Integration Integration          `json:"integration"`
	Digest      string               `json:"digest"`
}

type intentRecord struct {
	Candidate   contracts.Candidate  `json:"candidate"`
	Gate        contracts.GateResult `json:"gate"`
	Stage       string               `json:"stage"`
	Integration Integration          `json:"integration,omitempty"`
}

// NewIntegrator creates a foreground serial integrator. It delegates all Git
// operations and exact worktree identity checks to the inherited manager.
func NewIntegrator(project project.Project, state *store.Store, manager contracts.WorktreeManager) Integrator {
	key := canonicalPath(project.Root) + "\x00" + canonicalPath(project.CommonDir)
	if state != nil {
		key += "\x00" + canonicalPath(state.Root)
	}
	value, _ := integrationGuards.LoadOrStore(key, &integrationGuard{})
	return &serialIntegrator{project: project, store: state, manager: manager, guard: value.(*integrationGuard)}
}

func (i *serialIntegrator) Integrate(ctx context.Context, candidate contracts.Candidate, gateResult contracts.GateResult) (Integration, error) {
	if ctx == nil || ctx.Err() != nil {
		return Integration{}, core.ErrTransition
	}
	if i == nil || i.store == nil || i.manager == nil {
		return Integration{}, core.ErrPath
	}
	if _, err := i.validate(ctx, candidate, gateResult, false); err != nil {
		return Integration{}, err
	}
	if err := gate.ValidateEvidence(i.store, candidate, gateResult); err != nil {
		return Integration{}, err
	}
	canonical, err := gate.CanonicalEvidence(i.store, candidate, gateResult)
	if err != nil {
		return Integration{}, err
	}
	manifest, err := i.validate(ctx, candidate, gateResult, canonical)
	if err != nil {
		return Integration{}, err
	}
	if i.guard == nil {
		return Integration{}, core.ErrPath
	}
	if !i.guard.mu.TryLock() {
		return Integration{}, core.ErrTransition
	}
	defer i.guard.mu.Unlock()
	pointer := integrationPointer(candidate)
	intentPointer := "integrations/intents/" + integrationFingerprint(candidate) + ".json"
	intent, intentExists, err := i.intent(intentPointer)
	if err != nil {
		return Integration{}, err
	}
	if intentExists {
		if intent.Candidate != candidate || !reflect.DeepEqual(intent.Gate, gateResult) {
			return Integration{}, core.ErrTransition
		}
		if intent.Stage != "integrated" {
			return Integration{}, core.ErrTransition
		}
		existing, found, err := i.existing(pointer, candidate, gateResult, manifest.Project)
		if err != nil || !found || !reflect.DeepEqual(intent.Integration, existing) {
			return Integration{}, core.ErrTransition
		}
		return existing, nil
	}
	if _, found, err := i.existing(pointer, candidate, gateResult, manifest.Project); err != nil {
		return Integration{}, err
	} else if found {
		return Integration{}, core.ErrTransition
	}
	next, err := i.history(manifest.Project)
	if err != nil {
		return Integration{}, err
	}
	intent = intentRecord{Candidate: candidate, Gate: gateResult, Stage: "integrating"}
	if _, err := i.store.CreateJSON(intentPointer, intent, evidenceLimit); err != nil {
		return Integration{}, err
	}

	inspected, err := i.manager.Inspect(ctx, candidate.Worktree)
	if err != nil {
		return Integration{}, err
	}
	if inspected != candidate.Worktree {
		return Integration{}, core.ErrRevision
	}
	integrated, err := i.manager.Integrate(ctx, candidate)
	if err != nil {
		return Integration{}, err
	}
	if integrated != candidate {
		return Integration{}, core.ErrRevision
	}
	if inspected, err = i.manager.Inspect(ctx, candidate.Worktree); err != nil || inspected != candidate.Worktree {
		return Integration{}, core.ErrRevision
	}
	result := Integration{
		RecordEnvelope: core.RecordEnvelope{Schema: 1, Project: manifest.Project, RunID: candidate.Worktree.Run, Revision: uint64(next + 1), WrittenAt: "1970-01-01T00:00:00Z"},
		Task:           candidate.Task, Base: candidate.Base, Candidate: candidate.Revision, Commit: integrated.Revision,
		Order: next + 1, EvidencePointer: pointer,
	}
	record := evidenceRecord{Candidate: candidate, Gate: gateResult, Integration: result}
	record.Digest = evidenceDigest(record)
	if _, err := i.store.CreateJSON(pointer, record, evidenceLimit); err != nil {
		return Integration{}, err
	}
	if _, err := i.store.WriteJSON(intentPointer, intentRecord{Candidate: candidate, Gate: gateResult, Stage: "integrated", Integration: result}, evidenceLimit); err != nil {
		return Integration{}, err
	}
	return result, nil
}

func (i *serialIntegrator) validate(ctx context.Context, candidate contracts.Candidate, gate contracts.GateResult, canonical bool) (run.Run, error) {
	if i.project.Root == "" || i.project.Head == "" || i.project.Dirty || i.project.Detached {
		return run.Run{}, core.ErrTransition
	}
	worktree := candidate.Worktree
	if candidate.Task == "" || candidate.Revision == "" || candidate.Base == "" || candidate.Base != i.project.Head ||
		worktree.Run == "" || worktree.Team == "" || worktree.Path == "" || worktree.Branch == "" ||
		worktree.Base != candidate.Base || worktree.Dirty ||
		(worktree.Candidate != "" && worktree.Candidate != candidate.Revision) ||
		(worktree.Canonical != "" && worktree.Canonical != candidate.Revision) {
		if worktree.Dirty {
			return run.Run{}, core.ErrTransition
		}
		return run.Run{}, core.ErrRevision
	}
	if gate.Result != "CLEAN" || gate.Revision != candidate.Revision || gate.Evidence == "" || gate.CleanEvidence == "" {
		return run.Run{}, core.ErrRevision
	}
	manifest, err := run.NewRepositories(i.store).Runs.Read(ctx, candidate.Worktree.Run)
	if err != nil {
		if !canonical {
			return run.Run{RecordEnvelope: core.RecordEnvelope{Project: i.project.Root}}, nil
		}
		return run.Run{}, core.ErrRevision
	}
	if canonicalPath(manifest.Root) != canonicalPath(i.project.Root) || manifest.Project == "" || manifest.Project != manifest.Root {
		return run.Run{}, core.ErrRevision
	}
	if !canonical {
		return manifest, nil
	}
	found := false
	for _, task := range manifest.Tasks {
		if task.ID == candidate.Task {
			if found {
				return run.Run{}, core.ErrRevision
			}
			found = true
		}
	}
	if !found {
		return run.Run{}, core.ErrRevision
	}
	return manifest, nil
}

func (i *serialIntegrator) existing(pointer string, candidate contracts.Candidate, gate contracts.GateResult, projectName string) (Integration, bool, error) {
	var record evidenceRecord
	err := i.store.ReadJSON(pointer, evidenceLimit, &record)
	if errors.Is(err, os.ErrNotExist) {
		return Integration{}, false, nil
	}
	if err != nil {
		return Integration{}, false, err
	}
	if record.Digest != evidenceDigest(record) || record.Candidate != candidate || !reflect.DeepEqual(record.Gate, gate) ||
		record.Integration.Task != candidate.Task || record.Integration.Base != candidate.Base ||
		record.Integration.Candidate != candidate.Revision || record.Integration.Commit != candidate.Revision ||
		record.Integration.EvidencePointer != pointer || record.Integration.Order < 1 ||
		record.Integration.Schema != 1 || record.Integration.Project != projectName ||
		record.Integration.RunID != candidate.Worktree.Run || record.Integration.Revision != uint64(record.Integration.Order) ||
		record.Integration.WrittenAt == "" {
		return Integration{}, false, core.ErrRevision
	}
	if _, err := time.Parse(time.RFC3339, record.Integration.WrittenAt); err != nil {
		return Integration{}, false, core.ErrRevision
	}
	return record.Integration, true, nil
}

func (i *serialIntegrator) intent(pointer string) (intentRecord, bool, error) {
	var record intentRecord
	err := i.store.ReadJSON(pointer, evidenceLimit, &record)
	if errors.Is(err, os.ErrNotExist) {
		return intentRecord{}, false, nil
	}
	if err != nil {
		return intentRecord{}, false, core.ErrTransition
	}
	return record, true, nil
}

// history trusts only complete, digest-valid records in the canonical records
// directory. Intents live below its dedicated subdirectory and never count.
func (i *serialIntegrator) history(projectName string) (int, error) {
	entries, err := os.ReadDir(filepath.Join(i.store.Root, "integrations"))
	if errors.Is(err, os.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, core.ErrTransition
	}
	orders := make([]int, 0, len(entries))
	tasks := map[string]bool{}
	for _, entry := range entries {
		if entry.IsDir() && entry.Name() == "intents" {
			continue
		}
		if entry.IsDir() || !entry.Type().IsRegular() || len(entry.Name()) != 69 || filepath.Ext(entry.Name()) != ".json" {
			return 0, core.ErrTransition
		}
		pointer := "integrations/" + entry.Name()
		var record evidenceRecord
		if err := i.store.ReadJSON(pointer, evidenceLimit, &record); err != nil {
			return 0, core.ErrTransition
		}
		candidate := record.Candidate
		if integrationPointer(candidate) != pointer || record.Digest != evidenceDigest(record) ||
			record.Integration.EvidencePointer != pointer || record.Integration.Project != projectName ||
			record.Integration.Order < 1 || record.Integration.Revision != uint64(record.Integration.Order) ||
			record.Integration.Task != candidate.Task || record.Integration.RunID != candidate.Worktree.Run ||
			record.Integration.Base != candidate.Base || record.Integration.Candidate != candidate.Revision ||
			record.Integration.Commit != candidate.Revision || gate.ValidateEvidence(i.store, candidate, record.Gate) != nil {
			return 0, core.ErrTransition
		}
		key := string(candidate.Worktree.Run) + "\x00" + string(candidate.Task)
		if tasks[key] {
			return 0, core.ErrTransition
		}
		tasks[key] = true
		orders = append(orders, record.Integration.Order)
	}
	sort.Ints(orders)
	for index, order := range orders {
		if order != index+1 {
			return 0, core.ErrTransition
		}
	}
	return len(orders), nil
}

func canonicalPath(value string) string {
	if value == "" {
		return ""
	}
	abs, err := filepath.Abs(value)
	if err != nil {
		return filepath.Clean(value)
	}
	return filepath.Clean(abs)
}

func evidenceDigest(record evidenceRecord) string {
	record.Digest = ""
	canonical, _ := json.Marshal(record)
	sum := sha256.Sum256(canonical)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func integrationPointer(candidate contracts.Candidate) string {
	return "integrations/" + integrationFingerprint(candidate) + ".json"
}

func integrationFingerprint(candidate contracts.Candidate) string {
	canonical, _ := json.Marshal(struct {
		Run  core.RunID  `json:"run"`
		Task core.TaskID `json:"task"`
	}{Run: candidate.Worktree.Run, Task: candidate.Task})
	sum := sha256.Sum256(canonical)
	return hex.EncodeToString(sum[:])
}
