package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/thebpandey/agent-team/vnext/internal/cli"
	"github.com/thebpandey/agent-team/vnext/internal/core"
	"github.com/thebpandey/agent-team/vnext/internal/project"
	"github.com/thebpandey/agent-team/vnext/internal/run"
	"github.com/thebpandey/agent-team/vnext/internal/start"
	"github.com/thebpandey/agent-team/vnext/internal/store"
	"github.com/thebpandey/agent-team/vnext/internal/tracker"
	"github.com/thebpandey/agent-team/vnext/internal/worktree"
)

func runTaskAction(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	action, err := cli.Parse(args)
	if err != nil || (action.Name != "task add" && !strings.HasPrefix(action.Name, "one-off ")) {
		return managementError(args, stdout, stderr, core.ErrPhase)
	}
	requestArgs := action.Args
	mode := ""
	if action.Name == "task add" {
		mode, requestArgs = requestArgs[0], requestArgs[1:]
	}
	task, hostName, fromFile, err := taskRequest(requestArgs)
	if err != nil {
		return managementError(args, stdout, stderr, err)
	}
	task.WritablePaths, task.Resources, err = run.NormalizeAuthority(task.WritablePaths, task.Resources)
	if err != nil {
		return managementError(args, stdout, stderr, fmt.Errorf("task %q: %w", task.ID, err))
	}
	kind := run.OneOffKind(strings.TrimPrefix(action.Name, "one-off "))
	readOnly := kind == run.Audit || kind == run.Review
	missing := taskDetails(task, readOnly)
	if strings.TrimSpace(task.Objective) == "" {
		return taskNeedsInput(args, stdout, action.Name, task, false, missing)
	}
	if action.Name != "task add" && (!fromFile || len(missing) != 0) {
		return taskNeedsInput(args, stdout, action.Name, task, false, missing)
	}
	p, err := project.Discover(ctx, ".")
	if err != nil {
		return managementError(args, stdout, stderr, err)
	}
	root := p.TopLevel
	st := store.New(root, core.DefaultConfig().Storage)
	if action.Name != "task add" {
		settings, err := optionalOneOffSettings(ctx, st)
		if err != nil {
			return managementError(args, stdout, stderr, err)
		}
		manager := worktree.NewManager(root, root, st, tracker.NewCommandRunner())
		result, err := start.AdmitOneOffRegistered(ctx, st, root, kind, task, manager, hostName, "HEAD")
		if err != nil {
			return managementError(args, stdout, stderr, err)
		}
		return taskReservationResult(args, stdout, action.Name, result, settings, readOnly)
	}
	selected, setup, err := projectTracker(ctx, root)
	if err != nil {
		return managementError(args, stdout, stderr, err)
	}
	if len(missing) != 0 {
		// The Beads adapter creates ready tasks only. Do not turn an incomplete
		// objective into executable authority to work around that limitation.
		if setup.Config.Tracker.Kind == "beads" {
			return taskNeedsInput(args, stdout, action.Name, task, false, missing)
		}
		task.State = core.Blocked
	} else if task.State == "" {
		task.State = core.Ready
	}
	page, err := selected.Page(ctx, "", 1000)
	if err != nil {
		return managementError(args, stdout, stderr, err)
	}
	created, replay := task, false
	for _, existing := range page.Tasks {
		if existing.ID != task.ID {
			continue
		}
		if !sameRequestedTask(existing, task) {
			return managementError(args, stdout, stderr, fmt.Errorf("%w: task %s exists with different details; edit the selected tracker explicitly", core.ErrRevision, task.ID))
		}
		created, replay = existing, true
		break
	}
	if !replay {
		created, err = selected.Create(ctx, task, page.TrackerRevision)
		if err != nil {
			return managementError(args, stdout, stderr, err)
		}
	}
	if len(missing) != 0 {
		return taskNeedsInput(args, stdout, action.Name, created, true, missing)
	}
	if mode == "--queue" {
		return managementResult(args, stdout, map[string]any{"ok": true, "action": action.Name, "status": "queued", "task": created, "already_created": replay, "host_dispatch_required": false, "next_action": "start"})
	}
	settings, err := project.NewSettingsService(st).Inspect(ctx)
	if err != nil {
		return managementError(args, stdout, stderr, err)
	}
	if settings.Revision == 0 {
		return managementResult(args, stdout, map[string]any{"ok": true, "action": action.Name, "status": "settings_required", "task": created, "next_action": "settings", "host_dispatch_required": false})
	}
	if err := requiredCapabilities(ctx, root, setup.Handoff.Capabilities); err != nil {
		return managementError(args, stdout, stderr, err)
	}
	manager := worktree.NewManager(root, root, st, tracker.NewCommandRunner())
	result, err := start.AdmitDefaultRegistered(ctx, st, root, taskSelection{Tracker: selected, id: created.ID}, manager, hostName, "HEAD")
	if err != nil {
		return managementError(args, stdout, stderr, err)
	}
	return taskReservationResult(args, stdout, action.Name, result, settings, false)
}

func taskRequest(args []string) (core.Task, string, bool, error) {
	hostName := "codex"
	var task core.Task
	fromFile := len(args) > 0 && args[0] == "--from"
	if fromFile {
		file, err := os.Open(args[1])
		if err != nil {
			return task, hostName, true, err
		}
		defer file.Close()
		info, err := file.Stat()
		if err != nil || !info.Mode().IsRegular() || info.Size() > 256<<10 {
			return task, hostName, true, fmt.Errorf("%w: task JSON must be a regular file no larger than 256 KiB", core.ErrLimit)
		}
		decoder := json.NewDecoder(io.LimitReader(file, 256<<10+1))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&task); err != nil {
			return task, hostName, true, err
		}
		var extra any
		if err := decoder.Decode(&extra); err != io.EOF {
			return task, hostName, true, fmt.Errorf("%w: expected exactly one task JSON object", core.ErrSettings)
		}
		for i := 2; i+1 < len(args); i += 2 {
			if args[i] == "--host" {
				hostName = args[i+1]
			}
		}
	} else if len(args) != 0 {
		task.Objective = args[0]
	}
	task.Objective = strings.TrimSpace(task.Objective)
	if len(task.Objective) > 256<<10 || task.Archived || (task.State != "" && task.State != core.Ready && task.State != core.Blocked) {
		return task, hostName, fromFile, fmt.Errorf("%w: task must be an unarchived ready or blocked request with a bounded objective", core.ErrSettings)
	}
	// User files specify task facts, never canonical record identities.
	task.RecordEnvelope = core.RecordEnvelope{}
	if task.ID == "" && task.Objective != "" {
		task.ID = core.TaskID(fmt.Sprintf("task-%x", sha256.Sum256([]byte(task.Objective)))[:21])
	}
	return task, hostName, fromFile, nil
}

func taskDetails(task core.Task, readOnly bool) []string {
	var missing []string
	if strings.TrimSpace(task.Objective) == "" {
		missing = append(missing, "objective")
	}
	if len(task.Criteria) == 0 {
		missing = append(missing, "criteria")
	}
	if !readOnly {
		if len(task.Checks) == 0 {
			missing = append(missing, "checks")
		}
		if len(task.WritablePaths) == 0 {
			missing = append(missing, "writablePaths")
		}
	}
	for _, check := range task.Checks {
		if strings.TrimSpace(check.Name) == "" || len(check.Command) == 0 || strings.TrimSpace(check.Command[0]) == "" {
			missing = append(missing, "checks with a name and nonempty command array")
			break
		}
	}
	return missing
}

func taskNeedsInput(args []string, stdout io.Writer, action string, task core.Task, created bool, missing []string) int {
	return managementResult(args, stdout, map[string]any{"ok": true, "action": action, "status": "needs_input", "next_action": "provide_task_details", "task": task, "created": created, "required_fields": missing, "host_dispatch_required": false, "message": "Provide the missing approved task details in one JSON object and pass --from <task.json>. Incomplete saved drafts remain blocked; edit their existing tracker record before execution."})
}

func sameRequestedTask(a, b core.Task) bool {
	if a.Objective != b.Objective || !slices.Equal(a.Criteria, b.Criteria) || !slices.Equal(a.WritablePaths, b.WritablePaths) || !slices.Equal(a.Dependencies, b.Dependencies) || !slices.Equal(a.Resources, b.Resources) || len(a.Checks) != len(b.Checks) {
		return false
	}
	for i := range a.Checks {
		if a.Checks[i].Name != b.Checks[i].Name || !slices.Equal(a.Checks[i].Command, b.Checks[i].Command) {
			return false
		}
	}
	return true
}

type taskSelection struct {
	tracker.Tracker
	id core.TaskID
}

func (t taskSelection) AuthorityMetadata() tracker.AuthorityMetadata {
	return t.Tracker.(tracker.AuthorityMetadataProvider).AuthorityMetadata()
}
func (t taskSelection) AllowsAutomaticAdmission(id core.TaskID) bool { return id == t.id }

func optionalOneOffSettings(ctx context.Context, st *store.Store) (project.Settings, error) {
	settings, err := project.NewSettingsService(st).Inspect(ctx)
	if err != nil && errors.Is(err, os.ErrNotExist) {
		if _, nativeErr := os.Stat(filepath.Join(st.Root, ".agent-team", "config.json")); errors.Is(nativeErr, os.ErrNotExist) {
			if _, legacyErr := os.Stat(filepath.Join(st.Root, ".agent-team", "v8", "authority.json")); errors.Is(legacyErr, os.ErrNotExist) {
				return project.Settings{}, nil
			}
		}
	}
	return settings, err
}

func assignmentRole(manifest run.Run) string {
	if manifest.Mode == "one-off" && (manifest.OneOffKind == run.Audit || manifest.OneOffKind == run.Review) {
		return "reviewer"
	}
	return "developer"
}

func taskReservationResult(args []string, stdout io.Writer, action string, result start.Result, settings project.Settings, readOnly bool) int {
	status := "reserved"
	if result.AlreadyAdmitted {
		status = "already_reserved"
	}
	if len(result.Team.Queue) == 0 {
		status = "completed"
	}
	return managementResult(args, stdout, map[string]any{"ok": true, "action": action, "status": status, "host_dispatch_required": result.HostDispatchRequired, "already_admitted": result.AlreadyAdmitted, "observation_required": result.AlreadyAdmitted && len(result.Team.Queue) != 0, "packet": result.Packet, "packet_digest": result.PacketDigest, "packet_path": result.PacketPath, "run": result.Run.ID, "team": result.Team.ID, "actual_host": result.Packet.Owner, "profile": settings.Profile(result.Packet.Owner, assignmentRole(result.Run)), "read_only": readOnly})
}
