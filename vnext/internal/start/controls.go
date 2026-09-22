package start

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"github.com/thebpandey/agent-team/vnext/internal/contracts"
	"github.com/thebpandey/agent-team/vnext/internal/core"
	"github.com/thebpandey/agent-team/vnext/internal/lifecycle"
	"github.com/thebpandey/agent-team/vnext/internal/project"
	"github.com/thebpandey/agent-team/vnext/internal/run"
	"github.com/thebpandey/agent-team/vnext/internal/store"
)

const controlsPath = ".agent-team/v8/controls.json"
const controlsLimit = 128 << 10

// ControlResult describes durable admission intent and actual host work still
// required. Only an exact host observation changes requested into observed.
type ControlResult struct {
	Action              string                   `json:"action"`
	Scope               core.Scope               `json:"scope"`
	ControlID           string                   `json:"control_id"`
	Status              string                   `json:"status"`
	AdmissionHeld       bool                     `json:"admission_held"`
	HostControlRequired bool                     `json:"host_control_required"`
	ObservationRequired bool                     `json:"observation_required"`
	Handles             []contracts.WorkerHandle `json:"handles"`
	PendingHandles      []contracts.WorkerHandle `json:"pending_handles"`
	UnacknowledgedTeams []core.TeamID            `json:"unacknowledged_teams,omitempty"`
	BlockingScopes      []core.Scope             `json:"blocking_scopes,omitempty"`
}

// AdmissionHeldError retains the control that prevented a reservation so native
// callers can explain the required resume or host observation without guessing.
type AdmissionHeldError struct {
	Control ControlResult
}

func (e *AdmissionHeldError) Error() string {
	return fmt.Sprintf("%v: admission held by %s %s", core.ErrTransition, e.Control.Scope.Kind, e.Control.Scope.ID)
}

func (e *AdmissionHeldError) Unwrap() error { return core.ErrTransition }

// InspectControls reads durable intent and observations without reconciling
// workers, repairing projections, or changing any project state.
func InspectControls(ctx context.Context, st *store.Store) ([]ControlResult, error) {
	if ctx == nil || ctx.Err() != nil {
		return nil, core.ErrTransition
	}
	canonical, err := canonicalControlStore(st)
	if err != nil {
		return nil, err
	}
	state, err := readControls(canonical)
	if err != nil {
		return nil, err
	}
	results := make([]ControlResult, 0, len(state.Controls))
	for _, record := range state.Controls {
		results = append(results, controlResult(record, state))
	}
	return results, nil
}

type controlRecord struct {
	Action       string                   `json:"action"`
	Scope        core.Scope               `json:"scope"`
	ID           string                   `json:"id"`
	Revision     uint64                   `json:"revision"`
	Handles      []contracts.WorkerHandle `json:"handles"`
	Observed     []contracts.WorkerHandle `json:"observed,omitempty"`
	PendingTeams []core.TeamID            `json:"pendingTeams,omitempty"`
}

type controlState struct {
	Schema   int             `json:"schema"`
	Project  string          `json:"project"`
	Revision uint64          `json:"revision"`
	Controls []controlRecord `json:"controls"`
}

// RequestControl records a user control intent without invoking a host or
// changing run/packet/worker ownership. Resume reopens only the selected scope;
// exact retained workers still require a real host resume and observation.
func RequestControl(ctx context.Context, st *store.Store, action string, scope core.Scope) (ControlResult, error) {
	var result ControlResult
	canonical, err := canonicalControlStore(st)
	if err != nil {
		return result, err
	}
	st = canonical
	if !controlAction(action) || validateControlScope(st.Root, scope) != nil {
		return result, core.ErrTransition
	}
	err = withProjectLock(ctx, st, func() error {
		handles, pending, err := controlTargets(ctx, st, scope)
		if err != nil {
			return err
		}
		state, err := readControls(st)
		if err != nil {
			return err
		}
		index := -1
		for i, prior := range state.Controls {
			if prior.Scope == scope {
				index = i
				break
			}
		}
		if index >= 0 {
			prior := state.Controls[index]
			if prior.Action == action && reflect.DeepEqual(prior.Handles, handles) && reflect.DeepEqual(prior.PendingTeams, pending) {
				result = controlResult(prior, state)
				return nil
			}
		}
		if index < 0 && len(state.Controls) >= 128 {
			return core.ErrLimit
		}
		state.Revision++
		record := controlRecord{Action: action, Scope: scope, Revision: state.Revision, Handles: handles, PendingTeams: pending}
		record.ID = controlDigest(st.Root, record)
		if index < 0 {
			state.Controls = append(state.Controls, record)
		} else {
			state.Controls[index] = record
		}
		if _, err := st.WriteJSON(controlsPath, state, controlsLimit); err != nil {
			return err
		}
		result = controlResult(record, state)
		return nil
	})
	return result, err
}

// AcknowledgeControl records only an observation of the exact current native
// handle named by a still-current control request. It never replaces a worker.
func AcknowledgeControl(ctx context.Context, st *store.Store, id string, handle contracts.WorkerHandle, observation string) (ControlResult, error) {
	var result ControlResult
	canonical, err := canonicalControlStore(st)
	if err != nil {
		return result, err
	}
	st = canonical
	err = withProjectLock(ctx, st, func() error {
		state, err := readControls(st)
		if err != nil {
			return err
		}
		for index, record := range state.Controls {
			if record.ID != id {
				continue
			}
			if observation != controlObservation(record.Action) || !containsHandle(record.Handles, handle) {
				return core.ErrRevision
			}
			current, _, err := controlTargets(ctx, st, record.Scope)
			if err != nil {
				return err
			}
			if !containsHandle(current, handle) {
				return core.ErrRevision
			}
			if record.Action == "resume" {
				packet := core.AssignmentPacket{Team: handle.Team, Task: handle.Task}
				packet.RunID = handle.Run
				if err := AdmissionAllowed(ctx, st, packet); err != nil {
					return err
				}
			}
			if !containsHandle(record.Observed, handle) {
				record.Observed = append(record.Observed, handle)
				state.Controls[index] = record
				state.Revision++
				if _, err := st.WriteJSON(controlsPath, state, controlsLimit); err != nil {
					return err
				}
			}
			result = controlResult(record, state)
			return nil
		}
		return core.ErrRevision
	})
	return result, err
}

// AdmissionAllowed must be called under the same project mutation guard as a
// fresh reservation. It works before run creation and also reads legacy holds.
func AdmissionAllowed(ctx context.Context, st *store.Store, packet core.AssignmentPacket) error {
	if ctx == nil || ctx.Err() != nil || st == nil || packet.RunID == "" || packet.Team == "" || packet.Task == "" {
		return core.ErrTransition
	}
	state, err := readControls(st)
	if err != nil {
		return err
	}
	for _, record := range state.Controls {
		if record.Action != "resume" && scopeContains(record.Scope, st.Root, packet.RunID, packet.Team, packet.Task) {
			return &AdmissionHeldError{Control: controlResult(record, state)}
		}
	}
	packet.Project = st.Root
	return lifecycle.AdmissionBarriersAllowed(ctx, st, packet)
}

func canonicalControlStore(st *store.Store) (*store.Store, error) {
	root, err := project.CanonicalStoreRoot(st)
	if err != nil {
		return nil, err
	}
	return store.New(root, st.Limits), nil
}
func controlAction(action string) bool {
	return action == "pause" || action == "stop" || action == "cancel" || action == "resume"
}
func controlObservation(action string) string {
	switch action {
	case "pause":
		return "paused"
	case "resume":
		return "running"
	default:
		return "stopped"
	}
}
func validateControlScope(root string, scope core.Scope) error {
	if scope.Kind == core.ScopeProject {
		if scope.ID != root {
			return core.ErrPath
		}
		return nil
	}
	if scope.Kind != core.ScopeRun && scope.Kind != core.ScopeTeam && scope.Kind != core.ScopeTask {
		return core.ErrTransition
	}
	return project.ValidateSegment(scope.ID)
}
func scopeContains(scope core.Scope, root string, runID core.RunID, team core.TeamID, task core.TaskID) bool {
	switch scope.Kind {
	case core.ScopeProject:
		return scope.ID == root
	case core.ScopeRun:
		return scope.ID == string(runID)
	case core.ScopeTeam:
		return scope.ID == string(team)
	case core.ScopeTask:
		return scope.ID == string(task)
	}
	return false
}
func containsHandle(handles []contracts.WorkerHandle, handle contracts.WorkerHandle) bool {
	for _, current := range handles {
		if current == handle {
			return true
		}
	}
	return false
}
func controlDigest(root string, record controlRecord) string {
	record.ID = ""
	record.Observed = nil
	raw, _ := json.Marshal(struct {
		Root   string
		Record controlRecord
	}{root, record})
	sum := sha256.Sum256(raw)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func controlResult(record controlRecord, state controlState) ControlResult {
	result := ControlResult{Action: record.Action, Scope: record.Scope, ControlID: record.ID, AdmissionHeld: record.Action != "resume", Handles: record.Handles, PendingHandles: []contracts.WorkerHandle{}, UnacknowledgedTeams: record.PendingTeams}
	blocked := map[contracts.WorkerHandle]bool{}
	if record.Action == "resume" {
		for _, other := range state.Controls {
			if other.Action == "resume" || other.Scope == record.Scope {
				continue
			}
			relevant := record.Scope.Kind == core.ScopeProject || other.Scope.Kind == core.ScopeProject
			for _, handle := range record.Handles {
				if scopeContains(other.Scope, state.Project, handle.Run, handle.Team, handle.Task) {
					blocked[handle] = true
					relevant = true
				}
			}
			if relevant {
				result.BlockingScopes = append(result.BlockingScopes, other.Scope)
			}
		}
		result.AdmissionHeld = len(result.BlockingScopes) > 0
	}
	for _, handle := range record.Handles {
		if !containsHandle(record.Observed, handle) && !blocked[handle] {
			result.PendingHandles = append(result.PendingHandles, handle)
		}
	}
	result.HostControlRequired = len(result.PendingHandles) > 0
	result.ObservationRequired = result.HostControlRequired || len(record.PendingTeams) > 0
	if len(result.BlockingScopes) > 0 && !result.HostControlRequired {
		result.Status = "admission_held"
		return result
	}
	if len(record.Handles) == 0 && len(record.PendingTeams) == 0 {
		if record.Action == "resume" {
			result.Status = "admission_resumed"
		} else {
			result.Status = "admission_held"
		}
		return result
	}
	result.Status = map[string]string{"pause": "pause_requested", "stop": "stop_requested", "cancel": "cancel_requested", "resume": "resume_requested"}[record.Action]
	if !result.ObservationRequired {
		result.Status = map[string]string{"pause": "paused", "stop": "stopped", "cancel": "cancelled", "resume": "resumed"}[record.Action]
	}
	return result
}

func readControls(st *store.Store) (controlState, error) {
	state := controlState{Schema: 1, Project: st.Root, Controls: []controlRecord{}}
	err := st.ReadJSON(controlsPath, controlsLimit, &state)
	if errors.Is(err, os.ErrNotExist) {
		return state, nil
	}
	if err != nil {
		return controlState{}, fmt.Errorf("%w: unreadable control state", core.ErrRevision)
	}
	if state.Schema != 1 || state.Project != st.Root || state.Revision == 0 || len(state.Controls) > 128 {
		return controlState{}, core.ErrRevision
	}
	scopes := map[core.Scope]bool{}
	for _, record := range state.Controls {
		if !controlAction(record.Action) || validateControlScope(st.Root, record.Scope) != nil || scopes[record.Scope] || record.Revision == 0 || record.Revision > state.Revision || record.ID != controlDigest(st.Root, record) || len(record.Handles) > 128 || len(record.PendingTeams) > 128 {
			return controlState{}, core.ErrRevision
		}
		scopes[record.Scope] = true
		seen := map[contracts.WorkerHandle]bool{}
		for _, handle := range record.Handles {
			if handle.Host == "" || handle.Identity == "" || handle.Run == "" || handle.Team == "" || handle.Task == "" || handle.PacketDigest == "" || handle.Reviewer || seen[handle] {
				return controlState{}, core.ErrRevision
			}
			seen[handle] = true
		}
		for _, handle := range record.Observed {
			if !seen[handle] {
				return controlState{}, core.ErrRevision
			}
		}
	}
	return state, nil
}

// The run projection is authoritative and already contains the exact retained
// handles. This read does not repair teams or change their lifecycle states.
func controlTargets(ctx context.Context, st *store.Store, scope core.Scope) ([]contracts.WorkerHandle, []core.TeamID, error) {
	handles := []contracts.WorkerHandle{}
	pending := []core.TeamID{}
	root, err := os.OpenRoot(st.Root)
	if err != nil {
		return nil, nil, err
	}
	defer root.Close()
	const directory = ".agent-team/runs"
	info, err := root.Lstat(directory)
	if errors.Is(err, os.ErrNotExist) && scope.Kind == core.ScopeProject {
		return handles, pending, nil
	}
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, nil, core.ErrTransition
	}
	dir, err := root.Open(directory)
	if err != nil {
		return nil, nil, err
	}
	defer dir.Close()
	entries, err := dir.ReadDir(65)
	if err != nil || len(entries) > 64 {
		return nil, nil, core.ErrLimit
	}
	matches := 0
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".agent-team-probe-to-") && !entry.IsDir() && entry.Type()&os.ModeSymlink == 0 {
			continue
		}
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 || filepath.Ext(entry.Name()) != ".json" {
			return nil, nil, core.ErrRevision
		}
		id := core.RunID(strings.TrimSuffix(entry.Name(), ".json"))
		manifest, err := run.NewRepositories(st).Runs.Read(ctx, id)
		if err != nil {
			return nil, nil, err
		}
		if manifest.Project != st.Root {
			return nil, nil, core.ErrRevision
		}
		member := scope.Kind == core.ScopeProject || scope.Kind == core.ScopeRun && scope.ID == string(id)
		if scope.Kind == core.ScopeTask {
			for _, task := range manifest.Tasks {
				if scope.ID == string(task.ID) {
					member = true
				}
			}
		}
		if scope.Kind == core.ScopeTeam {
			for _, team := range manifest.Teams {
				if scope.ID == string(team.ID) {
					member = true
				}
			}
		}
		if !member {
			continue
		}
		matches++
		for _, team := range manifest.Teams {
			if scope.Kind == core.ScopeTeam && scope.ID != string(team.ID) {
				continue
			}
			handle := team.Handle
			if handle.Identity == "" {
				handle = team.RetainedHandle
			}
			if handle.Identity != "" && (scope.Kind != core.ScopeTask || scope.ID == string(handle.Task)) {
				if !containsHandle(handles, handle) {
					handles = append(handles, handle)
				}
			}
			if team.IntentDigest != "" && team.Handle.Identity == "" && len(team.Queue) > 0 && (scope.Kind != core.ScopeTask || scope.ID == string(team.Queue[0])) {
				pending = append(pending, team.ID)
			}
		}
	}
	if scope.Kind != core.ScopeProject && matches != 1 {
		return nil, nil, core.ErrTransition
	}
	if len(handles) > 128 || len(pending) > 128 {
		return nil, nil, core.ErrLimit
	}
	sort.Slice(handles, func(i, j int) bool {
		if handles[i].Team != handles[j].Team {
			return handles[i].Team < handles[j].Team
		}
		return handles[i].Identity < handles[j].Identity
	})
	sort.Slice(pending, func(i, j int) bool { return pending[i] < pending[j] })
	return handles, pending, nil
}
