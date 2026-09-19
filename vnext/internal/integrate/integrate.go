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
	"sync"
	"time"

	"github.com/thebpandey/agent-team/vnext/internal/contracts"
	"github.com/thebpandey/agent-team/vnext/internal/core"
	"github.com/thebpandey/agent-team/vnext/internal/gate"
	"github.com/thebpandey/agent-team/vnext/internal/project"
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
	mu   sync.Mutex
	next int
}

var integrationGuards sync.Map

type evidenceRecord struct {
	Candidate   contracts.Candidate  `json:"candidate"`
	Gate        contracts.GateResult `json:"gate"`
	Integration Integration          `json:"integration"`
	Digest      string               `json:"digest"`
}

type intentRecord struct {
	Candidate contracts.Candidate  `json:"candidate"`
	Gate      contracts.GateResult `json:"gate"`
	Stage     string               `json:"stage"`
}

// NewIntegrator creates a foreground serial integrator. It delegates all Git
// operations and exact worktree identity checks to the inherited manager.
func NewIntegrator(project project.Project, state *store.Store, manager contracts.WorktreeManager) Integrator {
	key := project.Root
	if root, err := filepath.Abs(project.Root); err == nil {
		key = root
	}
	if state != nil {
		if root, err := filepath.Abs(state.Root); err == nil {
			key += "\x00" + root
		}
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
	if err := i.validate(candidate, gateResult); err != nil {
		return Integration{}, err
	}
	if err := gate.ValidateEvidence(i.store, candidate, gateResult); err != nil {
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
	if existing, found, err := i.existing(pointer, candidate, gateResult); err != nil {
		return Integration{}, err
	} else if found {
		return existing, nil
	}
	intentPointer := "integrations/intents/" + integrationFingerprint(candidate) + ".json"
	intent := intentRecord{Candidate: candidate, Gate: gateResult, Stage: "intent"}
	if _, err := i.store.CreateJSON(intentPointer, intent, evidenceLimit); err != nil {
		if !errors.Is(err, store.ErrAlreadyExists) {
			return Integration{}, err
		}
		var persisted intentRecord
		if readErr := i.store.ReadJSON(intentPointer, evidenceLimit, &persisted); readErr != nil || persisted.Candidate != candidate || !reflect.DeepEqual(persisted.Gate, gateResult) || persisted.Stage != "integrated" {
			return Integration{}, core.ErrTransition
		}
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
	i.guard.next++
	result := Integration{
		RecordEnvelope: core.RecordEnvelope{Schema: 1, Project: i.project.Root, RunID: candidate.Worktree.Run, Revision: uint64(i.guard.next), WrittenAt: "1970-01-01T00:00:00Z"},
		Task:           candidate.Task, Base: candidate.Base, Candidate: candidate.Revision, Commit: integrated.Revision,
		Order: i.guard.next, EvidencePointer: pointer,
	}
	record := evidenceRecord{Candidate: candidate, Gate: gateResult, Integration: result}
	record.Digest = evidenceDigest(record)
	if _, err := i.store.CreateJSON(pointer, record, evidenceLimit); err == nil {
		if _, err := i.store.WriteJSON(intentPointer, intentRecord{Candidate: candidate, Gate: gateResult, Stage: "integrated"}, evidenceLimit); err != nil {
			return Integration{}, err
		}
		return result, nil
	} else if !errors.Is(err, store.ErrAlreadyExists) {
		return Integration{}, err
	}
	// A concurrent foreground caller may have persisted the same accepted work
	// after the manager completed. Recover only the exact durable record.
	existing, found, err := i.existing(pointer, candidate, gateResult)
	if err != nil || !found {
		return Integration{}, core.ErrRevision
	}
	return existing, nil
}

func (i *serialIntegrator) validate(candidate contracts.Candidate, gate contracts.GateResult) error {
	if i.project.Root == "" || i.project.Head == "" || i.project.Dirty || i.project.Detached {
		return core.ErrTransition
	}
	worktree := candidate.Worktree
	if candidate.Task == "" || candidate.Revision == "" || candidate.Base == "" || candidate.Base != i.project.Head ||
		worktree.Run == "" || worktree.Team == "" || worktree.Path == "" || worktree.Branch == "" ||
		worktree.Base != candidate.Base || worktree.Dirty ||
		(worktree.Candidate != "" && worktree.Candidate != candidate.Revision) ||
		(worktree.Canonical != "" && worktree.Canonical != candidate.Revision) {
		if worktree.Dirty {
			return core.ErrTransition
		}
		return core.ErrRevision
	}
	if gate.Result != "CLEAN" || gate.Revision != candidate.Revision || gate.Evidence == "" || gate.CleanEvidence == "" {
		return core.ErrRevision
	}
	return nil
}

func (i *serialIntegrator) existing(pointer string, candidate contracts.Candidate, gate contracts.GateResult) (Integration, bool, error) {
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
		record.Integration.Schema != 1 || record.Integration.Project != i.project.Root ||
		record.Integration.RunID != candidate.Worktree.Run || record.Integration.Revision != uint64(record.Integration.Order) ||
		record.Integration.WrittenAt == "" {
		return Integration{}, false, core.ErrRevision
	}
	if _, err := time.Parse(time.RFC3339, record.Integration.WrittenAt); err != nil {
		return Integration{}, false, core.ErrRevision
	}
	if record.Integration.Order > i.guard.next {
		i.guard.next = record.Integration.Order
	}
	return record.Integration, true, nil
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
