package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/thebpandey/agent-team/vnext/internal/cli"
	"github.com/thebpandey/agent-team/vnext/internal/core"
	"github.com/thebpandey/agent-team/vnext/internal/migrate"
	"github.com/thebpandey/agent-team/vnext/internal/preparation"
	"github.com/thebpandey/agent-team/vnext/internal/project"
	"github.com/thebpandey/agent-team/vnext/internal/run"
	"github.com/thebpandey/agent-team/vnext/internal/start"
	"github.com/thebpandey/agent-team/vnext/internal/store"
	"github.com/thebpandey/agent-team/vnext/internal/tracker"
)

var inspectDependencies = preparation.Inspect
var installDependencies = preparation.Install
var initializeDependencies = preparation.Initialize

func optionValue(args []string, key string) string {
	for i := 0; i+1 < len(args); i++ {
		if args[i] == key {
			return args[i+1]
		}
	}
	return ""
}

func runSetup(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	action, err := cli.Parse(args)
	if err != nil || action.Name != "setup" {
		return managementError(args, stdout, stderr, core.ErrPhase)
	}
	if slices.Contains(action.Args, "--refuse-kickoff") {
		return managementResult(args, stdout, map[string]any{"ok": true, "action": "setup", "status": "cancelled", "message": "Setup declined. No project or dependency changes were made."})
	}
	p, err := project.Discover(ctx, ".")
	if err != nil {
		return managementError(args, stdout, stderr, fmt.Errorf("run setup from a Git project: %w", err))
	}
	root := p.TopLevel
	if optionValue(action.Args, "--mode") == "one-off" {
		result, err := project.NewSetupService(store.New(root, core.DefaultConfig().Storage)).Initialize(ctx, project.SetupInput{Root: root, Mode: project.OneOffMode})
		if err != nil {
			return managementError(args, stdout, stderr, err)
		}
		return managementResult(args, stdout, map[string]any{"ok": true, "action": "setup", "status": "validated", "mode": "one-off", "project": result.Project.TopLevel, "message": "Trackerless one-off preflight passed."})
	}
	options := project.SetupOptions{Root: root, Tracker: optionValue(action.Args, "--tracker"), Kickoff: optionValue(action.Args, "--kickoff"), Approved: slices.Contains(action.Args, "--approve"), ApproveKickoff: slices.Contains(action.Args, "--approve-kickoff"), IgnoreKickoff: slices.Contains(action.Args, "--ignore-kickoff")}
	var dependencies []preparation.Dependency
	if names := optionValue(action.Args, "--install"); names != "" {
		dependencies, err = installDependencies(ctx, root, strings.Split(names, ","), options.Approved)
		if err != nil {
			return managementError(args, stdout, stderr, err)
		}
		if !options.Approved {
			return managementResult(args, stdout, map[string]any{"ok": true, "action": "setup", "status": "needs_input", "next_action": "approve_dependencies", "dependencies": dependencies, "message": "Approve the selected dependency installations before continuing."})
		}
		var prepareNames []string
		for _, dep := range dependencies {
			if dep.Available && (dep.Name != "beads" || options.Tracker == "beads") {
				prepareNames = append(prepareNames, dep.Name)
			}
		}
		if len(prepareNames) > 0 {
			prepared, prepareErr := initializeDependencies(ctx, root, prepareNames, true)
			if prepareErr != nil {
				return managementError(args, stdout, stderr, prepareErr)
			}
			for i := range dependencies {
				for _, dep := range prepared {
					if dep.Name == dependencies[i].Name {
						dependencies[i] = dep
					}
				}
			}
		}
	}
	if slices.Contains(action.Args, "--prepare-only") {
		if dependencies == nil {
			dependencies, err = inspectDependencies(ctx, root, preparation.All())
			if err != nil {
				return managementError(args, stdout, stderr, err)
			}
		}
		status, next, message := "prepared", "project_kickoff", "Selected dependencies are prepared. Continue approved Project Kickoff planning and tracker seeding, then import its handoff."
		for _, dep := range dependencies {
			if dep.Status == "deferred" && dep.Available {
				status, message = "deferred", "Tools are installed; project analysis will be prepared after source files and a Git commit exist. Continue Project Kickoff."
				continue
			}
			if !dep.Available || !dep.Prepared {
				status, next, message = "needs_input", "resolve_dependencies", "Review dependency results; retry failed preparation or explicitly choose an available fallback."
				break
			}
		}
		return managementResult(args, stdout, map[string]any{"ok": true, "action": "setup", "status": status, "next_action": next, "dependencies": dependencies, "message": message})
	}
	if optionValue(action.Args, "--install") != "" {
		for _, dep := range dependencies {
			if dep.Status == "failed" || dep.Status == "needs_consent" || !dep.Available {
				return managementResult(args, stdout, map[string]any{"ok": true, "action": "setup", "status": "needs_input", "next_action": "resolve_dependencies", "dependencies": dependencies, "message": "The requested preparation did not finish. Follow its diagnostic, or explicitly continue without an optional tool; project setup has not been advanced."})
			}
		}
	}
	result, err := project.Onboard(ctx, options)
	if err != nil {
		return managementError(args, stdout, stderr, err)
	}
	if result.NextAction == "initialize_beads" && (options.Approved || options.ApproveKickoff) {
		beads, prepareErr := initializeDependencies(ctx, root, []string{"beads"}, true)
		if prepareErr != nil {
			return managementError(args, stdout, stderr, prepareErr)
		}
		dependencies = append(dependencies, beads...)
		if len(beads) == 1 && beads[0].Prepared {
			result, err = project.Onboard(ctx, options)
		}
		if err != nil {
			return managementError(args, stdout, stderr, err)
		}
	}
	if dependencies == nil && optionValue(action.Args, "--host") != "" {
		dependencies, err = inspectDependencies(ctx, root, preparation.All())
		if err != nil {
			return managementError(args, stdout, stderr, err)
		}
	}
	raw, _ := json.Marshal(result)
	output := map[string]any{}
	_ = json.Unmarshal(raw, &output)
	output["ok"], output["action"], output["mode"] = true, "setup", "plan"
	if dependencies != nil {
		output["dependencies"] = dependencies
	}
	if result.Handoff != nil && result.Status == "initialized" {
		if err := requiredCapabilities(ctx, root, result.Handoff.Capabilities); err != nil {
			output["next_action"] = "prepare_dependencies"
			output["message"] = err.Error()
		}
	}
	if result.Status == "initialized" && output["next_action"] == "start" {
		addReadiness(ctx, root, p.Head, output)
	}
	code := managementResult(args, stdout, output)
	if code == 0 && result.Status == "needs_input" {
		return 1
	}
	return code
}

func runStatus(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	action, err := cli.Parse(args)
	if err != nil {
		return managementError(args, stdout, stderr, err)
	}
	p, err := project.Discover(ctx, ".")
	if err != nil {
		return managementError(args, stdout, stderr, err)
	}
	st := store.New(p.TopLevel, core.DefaultConfig().Storage)
	if id := optionValue(action.Args, "--run"); id != "" {
		manifest, readErr := run.NewRepositories(st).Runs.Read(ctx, core.RunID(id))
		if readErr != nil {
			return managementError(args, stdout, stderr, readErr)
		}
		if manifest.Mode == "one-off" {
			settings, settingsErr := optionalOneOffSettings(ctx, st)
			if settingsErr != nil {
				return managementError(args, stdout, stderr, settingsErr)
			}
			return managementResult(args, stdout, map[string]any{"ok": true, "action": "status", "status": manifest.State, "project": p.TopLevel, "run": manifest, "settings": settings, "next_action": "observe_run"})
		}
	}
	settings, err := project.NewSettingsService(st).Inspect(ctx)
	if err != nil {
		if _, configErr := os.Stat(filepath.Join(p.TopLevel, ".agent-team/config.json")); errors.Is(configErr, os.ErrNotExist) {
			if _, authorityErr := os.Stat(filepath.Join(p.TopLevel, ".agent-team/v8/authority.json")); errors.Is(authorityErr, os.ErrNotExist) {
				const candidate = ".project-kickoff/AGENT_TEAM_HANDOFF.json"
				if _, handoffErr := os.Lstat(filepath.Join(p.TopLevel, filepath.FromSlash(candidate))); handoffErr == nil {
					inspection := project.InspectKickoff(p.TopLevel, candidate)
					if inspection.Reason != "" {
						return managementResult(args, stdout, map[string]any{"ok": true, "action": "status", "status": "needs_input", "next_action": "resolve_kickoff", "project": p.TopLevel, "kickoff_resolution": inspection, "message": "Resolve or explicitly ignore the discovered Project Kickoff handoff before setup."})
					}
				} else if !errors.Is(handoffErr, os.ErrNotExist) {
					return managementError(args, stdout, stderr, handoffErr)
				}
				return managementResult(args, stdout, map[string]any{"ok": true, "action": "status", "status": "setup_required", "next_action": "setup", "project": p.TopLevel})
			}
		}
		return managementError(args, stdout, stderr, err)
	}
	output := map[string]any{"ok": true, "action": "status", "status": "initialized", "project": p.TopLevel, "settings": settings, "next_action": "start"}
	if settings.Revision == 0 {
		output["next_action"] = "settings"
	}
	if setup, readErr := project.InspectSetup(ctx, p.TopLevel); readErr == nil {
		output["tracker"] = setup.Config.Tracker
	} else if !errors.Is(readErr, os.ErrNotExist) {
		return managementError(args, stdout, stderr, readErr)
	} else if _, authorityErr := migrate.AuthorityStatus(p.TopLevel); authorityErr != nil {
		return managementError(args, stdout, stderr, readErr)
	}
	if id := optionValue(action.Args, "--run"); id != "" {
		manifest, readErr := run.NewRepositories(st).Runs.Read(ctx, core.RunID(id))
		if readErr != nil {
			return managementError(args, stdout, stderr, readErr)
		}
		output["run"] = manifest
	} else {
		controls, controlsErr := start.InspectControls(ctx, st)
		if controlsErr != nil {
			return managementError(args, stdout, stderr, controlsErr)
		}
		if !addControlGuidance(output, controls) && settings.Revision != 0 {
			addReadiness(ctx, p.TopLevel, p.Head, output)
		}
	}
	return managementResult(args, stdout, output)
}

// Readiness probes are bounded and read-only. They neither repair a tracker nor
// turn an empty project into a run; planning remains an explicit next step.
func addReadiness(ctx context.Context, root, head string, output map[string]any) {
	if head == "" {
		output["status"], output["next_action"] = "commit_required", "project_kickoff"
		output["message"] = "Commit the approved project scaffold before creating task worktrees."
		return
	}
	selected, setup, err := projectTracker(ctx, root)
	if err != nil {
		output["status"], output["next_action"], output["message"] = "needs_input", "repair_tracker", err.Error()
		return
	}
	probe, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	page, err := selected.Page(probe, "", 1000)
	if err != nil {
		output["status"], output["next_action"] = "needs_input", "repair_tracker"
		output["message"] = "The selected tracker could not be read. Preserve its data and repair the reported problem: " + err.Error()
		return
	}
	if page.TotalNonArchived == 0 {
		output["status"], output["next_action"] = "no_ready_tasks", "project_kickoff"
		output["message"] = "The selected tracker is empty. Continue Project Kickoff or add a bounded task before starting."
		return
	}
	if err := requiredCapabilities(probe, root, setup.Handoff.Capabilities); err != nil {
		output["status"], output["next_action"], output["message"] = "needs_input", "prepare_dependencies", err.Error()
	}
}

func requiredCapabilities(ctx context.Context, root string, names []string) error {
	if len(names) == 0 {
		return nil
	}
	deps, err := initializeDependencies(ctx, root, names, false)
	if err != nil {
		return err
	}
	var missing []string
	for _, dep := range deps {
		if !dep.Available || !dep.Prepared {
			missing = append(missing, dep.Name)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("required project capabilities unavailable: %s; approve preparation with setup --install %s --approve", strings.Join(missing, ", "), strings.Join(missing, ","))
	}
	return nil
}

type selectedBeadsRunner struct{ path string }

func (r selectedBeadsRunner) Run(ctx context.Context, name string, args ...string) tracker.CommandResult {
	if name == "bd" {
		name = r.path
	}
	return tracker.NewCommandRunner().Run(ctx, name, args...)
}

func projectTracker(ctx context.Context, root string) (tracker.Tracker, project.SetupResult, error) {
	setup, err := project.InspectSetup(ctx, root)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return nil, setup, err
		}
		if _, err := migrate.AuthorityStatus(root); err != nil {
			return nil, setup, err
		}
		setup.Config.Tracker = core.TrackerConfig{Kind: "beads", Path: ".beads"}
	}
	var selected tracker.Tracker
	switch setup.Config.Tracker.Kind {
	case "tasks-md":
		selected = tracker.NewTasksMD(filepath.Join(root, setup.Config.Tracker.Path), store.New(root, core.DefaultConfig().Storage))
	case "beads":
		if setup.Handoff.TrackerExecutable != "" {
			selected = tracker.NewBeads(selectedBeadsRunner{setup.Handoff.TrackerExecutable})
			break
		}
		deps, err := inspectDependencies(ctx, root, []string{"beads"})
		if err != nil {
			return nil, setup, err
		}
		if len(deps) != 1 || !deps[0].Available {
			return nil, setup, fmt.Errorf("Beads is selected but unavailable; approve setup --install beads --tracker beads --approve")
		}
		selected = tracker.NewBeads(selectedBeadsRunner{deps[0].Path})
	default:
		return nil, setup, fmt.Errorf("%w: unsupported selected tracker %s", core.ErrSettings, setup.Config.Tracker.Kind)
	}
	if setup.Handoff.TrackerKind != "" {
		selected = handoffTracker{Tracker: selected, handoff: setup.Handoff}
	}
	return selected, setup, nil
}

// The selected tracker continues to own task status and definitions. Approved
// handoff scope/checks fill only missing fields for its explicit task IDs.
type handoffTracker struct {
	tracker.Tracker
	handoff core.KickoffHandoff
}

func (t handoffTracker) AllowsAutomaticAdmission(id core.TaskID) bool {
	return slices.Contains(t.handoff.TaskIDs, id)
}

func (t handoffTracker) AuthorityMetadata() tracker.AuthorityMetadata {
	return t.Tracker.(tracker.AuthorityMetadataProvider).AuthorityMetadata()
}
func (t handoffTracker) enrich(task core.Task) core.Task {
	if !slices.Contains(t.handoff.TaskIDs, task.ID) {
		return task
	}
	if len(task.Criteria) == 0 {
		task.Criteria = append([]string(nil), t.handoff.Acceptance...)
	}
	if len(task.Checks) == 0 {
		task.Checks = append([]core.Check(nil), t.handoff.Checks...)
	}
	if len(task.WritablePaths) == 0 {
		task.WritablePaths = append([]string(nil), t.handoff.WritablePaths...)
	}
	return task
}
func (t handoffTracker) Page(ctx context.Context, cursor string, limit int) (core.TrackerPage, error) {
	p, e := t.Tracker.Page(ctx, cursor, limit)
	for i := range p.Tasks {
		p.Tasks[i] = t.enrich(p.Tasks[i])
	}
	return p, e
}
func (t handoffTracker) Get(ctx context.Context, id core.TaskID, rev uint64) (core.Task, error) {
	v, e := t.Tracker.Get(ctx, id, rev)
	return t.enrich(v), e
}
func (t handoffTracker) Refresh(ctx context.Context, rev uint64) (core.TrackerPage, error) {
	p, e := t.Tracker.Refresh(ctx, rev)
	for i := range p.Tasks {
		p.Tasks[i] = t.enrich(p.Tasks[i])
	}
	return p, e
}
