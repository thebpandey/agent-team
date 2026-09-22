package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/thebpandey/agent-team/vnext/internal/cli"
	"github.com/thebpandey/agent-team/vnext/internal/contracts"
	"github.com/thebpandey/agent-team/vnext/internal/core"
	"github.com/thebpandey/agent-team/vnext/internal/install"
	"github.com/thebpandey/agent-team/vnext/internal/lifecycle"
	"github.com/thebpandey/agent-team/vnext/internal/migrate"
	"github.com/thebpandey/agent-team/vnext/internal/project"
	releasepkg "github.com/thebpandey/agent-team/vnext/internal/release"
	"github.com/thebpandey/agent-team/vnext/internal/run"
	"github.com/thebpandey/agent-team/vnext/internal/start"
	"github.com/thebpandey/agent-team/vnext/internal/store"
	"github.com/thebpandey/agent-team/vnext/internal/tracker"
	"github.com/thebpandey/agent-team/vnext/internal/worktree"
)

func main() {
	interruptible, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	ctx, cancel := context.WithTimeout(interruptible, 5*time.Minute)
	defer cancel()
	code := cli.Run(ctx, os.Args[1:], core.Dependencies{
		ProjectRoot: ".",
		Stdout:      os.Stdout,
		Stderr:      os.Stderr,
		Management:  runManagement,
		ExecuteLifecycle: func(ctx context.Context, name string, selector []string) error {
			return lifecycle.ExecuteLifecycle(ctx, lifecycle.ParsedAction{Name: name, Selector: selector, ScopeRequired: true}, nil, nil, nil)
		},
	})
	os.Exit(code)
}

func runManagement(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if len(args) > 0 && (args[0] == "task" || args[0] == "one-off") {
		return runTaskAction(ctx, args, stdout, stderr)
	}
	if len(args) > 0 && (args[0] == "pause" || args[0] == "resume" || args[0] == "stop" || args[0] == "cancel") {
		return runLifecycle(ctx, args, stdout, stderr)
	}
	if len(args) > 0 && args[0] == "setup" {
		return runSetup(ctx, args, stdout, stderr)
	}
	if len(args) > 0 && args[0] == "status" {
		return runStatus(ctx, args, stdout, stderr)
	}
	if len(args) > 0 && args[0] == "start" {
		return runStart(ctx, args, stdout, stderr)
	}
	if len(args) > 0 && args[0] == "settings" {
		action, err := cli.Parse(args)
		if err != nil || action.Name != "settings" {
			return managementError(args, stdout, stderr, core.ErrPhase)
		}
		p, discoverErr := project.Discover(ctx, ".")
		if discoverErr != nil {
			return managementError(args, stdout, stderr, discoverErr)
		}
		service := project.NewSettingsService(store.New(p.TopLevel, core.DefaultConfig().Storage))
		var settings project.Settings
		if len(action.Args) == 0 {
			settings, err = service.Inspect(ctx)
		} else {
			updates := make(map[string]string, len(action.Args))
			for _, setting := range action.Args {
				key, value, _ := strings.Cut(setting, "=")
				updates[key] = value
			}
			settings, err = service.Update(ctx, updates)
		}
		if err != nil {
			return managementError(args, stdout, stderr, err)
		}
		return managementResult(args, stdout, map[string]any{"ok": true, "action": "settings", "settings": settings, "codex_developer": settings.CodexDeveloper})
	}
	if len(args) > 0 && args[0] == "cleanup" {
		owner, err := recoverMutationLock(ctx, ".", args, store.NativeLiveness{})
		if err != nil {
			return managementError(args, stdout, stderr, err)
		}
		return managementResult(args, stdout, map[string]any{"ok": true, "action": "cleanup", "recovered": true, "target": args[2], "operation": owner.OperationID, "owner_token": owner.Token})
	}
	if len(args) > 0 && args[0] == "cutover" {
		if len(args) != 3 || args[1] != "--request" {
			return managementError(args, stdout, stderr, core.ErrPhase)
		}
		requestAction, actionErr := cutoverRequestAction(args[2])
		if actionErr != nil {
			return managementError(args, stdout, stderr, actionErr)
		}
		if strings.HasPrefix(requestAction, "host-") {
			env := installEnvironment()
			layout, layoutErr := install.ResolveLayout(runtime.GOOS, env)
			if layoutErr != nil {
				return managementError(args, stdout, stderr, layoutErr)
			}
			if manifest, readErr := install.NewManifestStore(layout).Read(ctx); readErr == nil {
				layout, layoutErr = install.ResolveInstalledLayout(runtime.GOOS, env, manifest)
				if layoutErr != nil {
					return managementError(args, stdout, stderr, layoutErr)
				}
			}
			var request install.LegacyHostCutoverRequest
			if err := readStrictJSON(args[2], &request); err != nil {
				return managementError(args, stdout, stderr, err)
			}
			var release install.Release
			if request.Action == "host-cutover" {
				var releaseErr error
				release, releaseErr = localRelease("")
				if releaseErr != nil {
					return managementError(args, stdout, stderr, releaseErr)
				}
			}
			result, err := install.CutoverLegacyHosts(ctx, layout, release, request)
			if err != nil {
				return managementError(args, stdout, stderr, err)
			}
			return managementResult(args, stdout, map[string]any{"ok": true, "action": request.Action, "revision": result.ManifestRevision, "receipt_digest": result.ReceiptDigest, "idempotent": result.Idempotent})
		}
		result, err := migrate.ExecuteAuthorityRequest(ctx, args[2])
		if err != nil {
			return managementError(args, stdout, stderr, err)
		}
		output := map[string]any{"ok": true, "action": result.Action, "revision": result.TargetRevision, "receipt_digest": result.ReceiptDigest, "held": result.Held, "idempotent": result.Idempotent}
		if result.Action == "prepare" {
			output["payload_path"], output["payload_sha256"] = result.PayloadPath, result.PayloadSHA256
			output["request_path"], output["request_sha256"] = result.RequestPath, result.RequestSHA256
			output["trust_notice"] = "external operator trust is only the one-time fallback for stale or unverifiable v7 cutover; normal v8 setup and schema-4 receipt migrations do not require it"
		}
		return managementResult(args, stdout, output)
	}
	env := installEnvironment()
	layout, err := install.ResolveLayout(runtime.GOOS, env)
	if err != nil {
		return managementError(args, stdout, stderr, err)
	}
	action := args[0]
	manifest, readErr := install.NewManifestStore(layout).Read(ctx)
	if action != "install" && readErr == nil {
		layout, err = install.ResolveInstalledLayout(runtime.GOOS, env, manifest)
		if err != nil {
			return managementError(args, stdout, stderr, err)
		}
	}
	expected := manifest.Revision
	var outcome install.CASOutcome
	switch action {
	case "install":
		if readErr == nil || !errors.Is(readErr, fs.ErrNotExist) {
			return managementError(args, stdout, stderr, core.ErrRevision)
		}
		release, releaseErr := localRelease("")
		if releaseErr != nil {
			return managementError(args, stdout, stderr, releaseErr)
		}
		hosts := []install.Host{install.Host(args[2])}
		if args[2] == "both" {
			hosts = []install.Host{install.Codex, install.Claude}
		}
		outcome, err = install.Install(ctx, layout, release, hosts, 0)
	case "update":
		if readErr != nil {
			return managementError(args, stdout, stderr, readErr)
		}
		release, releaseErr := localRelease(args[2])
		if releaseErr != nil {
			return managementError(args, stdout, stderr, releaseErr)
		}
		outcome, err = install.Update(ctx, layout, release, expected)
	case "rollback":
		if readErr != nil {
			return managementError(args, stdout, stderr, readErr)
		}
		revision := ""
		for index := 3; index+1 < len(args); index++ {
			if args[index] == "--revision" {
				revision = args[index+1]
				break
			}
		}
		if revision != "" {
			outcome, err = install.RollbackRelease(ctx, layout, args[2], revision, expected)
		} else {
			outcome, err = install.Rollback(ctx, layout, args[2], expected)
		}
	case "uninstall":
		if readErr != nil {
			return managementError(args, stdout, stderr, readErr)
		}
		_, outcome, err = install.Uninstall(ctx, layout, expected)
	}
	if err != nil {
		return managementError(args, stdout, stderr, err)
	}
	return managementResult(args, stdout, map[string]any{"ok": true, "action": action, "revision": outcome.Manifest.Revision, "retained": outcome.Retained, "binary_path": layout.BinaryPath})
}

func runStart(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	action, err := cli.Parse(args)
	if err != nil || action.Name != "start" {
		return managementError(args, stdout, stderr, core.ErrPhase)
	}
	p, err := project.Discover(ctx, ".")
	if err != nil {
		return managementError(args, stdout, stderr, err)
	}
	root := p.TopLevel
	st := store.New(root, core.DefaultConfig().Storage)
	values := startValues(action.Args)
	var oneOff *run.Run
	if teamID := core.TeamID(values["--team"]); teamID != "" {
		team, readErr := run.NewRepositories(st).Teams.Read(ctx, teamID)
		if readErr != nil {
			return managementError(args, stdout, stderr, readErr)
		}
		manifest, readErr := run.NewRepositories(st).Runs.Read(ctx, team.RunID)
		if readErr != nil {
			return managementError(args, stdout, stderr, readErr)
		}
		if manifest.Mode == "one-off" {
			oneOff = &manifest
		}
	}
	settings, err := project.NewSettingsService(st).Inspect(ctx)
	if oneOff != nil {
		settings, err = optionalOneOffSettings(ctx, st)
	}
	if err != nil {
		if _, statErr := os.Stat(filepath.Join(root, ".agent-team", "config.json")); errors.Is(statErr, os.ErrNotExist) {
			if _, oldErr := os.Stat(filepath.Join(root, ".agent-team", "v8", "authority.json")); errors.Is(oldErr, os.ErrNotExist) {
				return managementResult(args, stdout, map[string]any{"ok": true, "action": "start", "status": "setup_required", "next_action": "setup", "message": "Run Agent-Team setup to choose dependencies, tracker, and role settings."})
			}
		}
		return managementError(args, stdout, stderr, err)
	}
	hostName := values["--host"]
	if hostName == "" {
		hostName = "codex"
	}
	if hostName != "codex" && hostName != "claude" {
		return managementError(args, stdout, stderr, core.ErrSettings)
	}
	role := "developer"
	if oneOff != nil {
		role = assignmentRole(*oneOff)
	}
	profile := settings.Profile(hostName, role)
	var selected tracker.Tracker
	var setup project.SetupResult
	if values["--action"] == "" || values["--action"] == "next" {
		if oneOff != nil {
			selected, err = start.TrackerForOneOff(*oneOff)
		} else {
			selected, setup, err = projectTracker(ctx, root)
		}
		if err != nil {
			if values["--action"] == "" && values["--run"] == "" {
				return managementResult(args, stdout, map[string]any{"ok": true, "action": "start", "status": "needs_input", "next_action": "repair_tracker", "message": err.Error()})
			}
			return managementError(args, stdout, stderr, err)
		}
	}
	if values["--action"] == "" && values["--run"] == "" {
		if settings.Revision == 0 {
			return managementResult(args, stdout, map[string]any{"ok": true, "action": "start", "status": "settings_required", "next_action": "settings", "message": "Choose role models and effort, or accept inherited defaults, before starting.", "settings": settings})
		}
		if err := requiredCapabilities(ctx, root, setup.Handoff.Capabilities); err != nil {
			return managementResult(args, stdout, map[string]any{"ok": true, "action": "start", "status": "needs_input", "next_action": "prepare_dependencies", "message": err.Error()})
		}
		if p.Head == "" {
			return managementResult(args, stdout, map[string]any{"ok": true, "action": "start", "status": "commit_required", "next_action": "project_kickoff", "message": "Commit the approved project scaffold before creating task worktrees."})
		}
		page, readErr := selected.Page(ctx, "", 1000)
		if readErr != nil {
			return managementResult(args, stdout, map[string]any{"ok": true, "action": "start", "status": "needs_input", "next_action": "repair_tracker", "message": "The selected tracker could not be read: " + readErr.Error()})
		}
		if page.TotalNonArchived == 0 {
			return managementResult(args, stdout, map[string]any{"ok": true, "action": "start", "status": "no_ready_tasks", "next_action": "project_kickoff", "message": "The selected tracker is empty. Continue Project Kickoff or add a bounded task before starting."})
		}
		// The native command reserves work only. A Codex skill later performs the
		// actual collaboration.spawn_agent call and acknowledges its exact handle.
		manager := worktree.NewManager(root, root, st, tracker.NewCommandRunner())
		result, err := start.AdmitDefaultRegistered(ctx, st, root, selected, manager, hostName, "HEAD")
		if err != nil {
			var held *start.AdmissionHeldError
			if errors.As(err, &held) {
				return startAdmissionError(args, stdout, stderr, err)
			}
			if errors.Is(err, core.ErrPhase) {
				return managementResult(args, stdout, map[string]any{"ok": true, "action": "start", "status": "no_ready_tasks", "next_action": "inspect_tracker", "message": "No approved task is ready with satisfied dependencies. Review the selected tracker or continue Project Kickoff."})
			}
			if strings.Contains(err.Error(), "needs approved writable paths") {
				return managementResult(args, stdout, map[string]any{"ok": true, "action": "start", "status": "needs_input", "next_action": "task_details", "message": err.Error()})
			}
			return managementError(args, stdout, stderr, err)
		}
		if result.Packet.Owner == "codex" || result.Packet.Owner == "claude" {
			profile = settings.Profile(result.Packet.Owner, assignmentRole(result.Run))
		}
		return managementResult(args, stdout, map[string]any{"ok": true, "action": "start", "actual_host": result.Packet.Owner, "observation_required": result.AlreadyAdmitted, "host_dispatch_required": result.HostDispatchRequired, "packet": result.Packet, "packet_digest": result.PacketDigest, "packet_path": result.PacketPath, "run": result.Run.ID, "team": result.Team.ID, "profile": profile, "already_admitted": result.AlreadyAdmitted})
	}
	if runValue := values["--run"]; runValue != "" {
		manifest, readErr := run.NewRepositories(st).Runs.Read(ctx, core.RunID(runValue))
		if readErr != nil || len(manifest.Teams) != 1 {
			if readErr == nil {
				readErr = core.ErrSettings
			}
			return managementError(args, stdout, stderr, readErr)
		}
		ids := startTaskIDs(action.Args)
		if len(ids) == 0 {
			return managementError(args, stdout, stderr, core.ErrPhase)
		}
		if selectedHost := retainedHost(manifest.Teams[0]); selectedHost != "" {
			profile = settings.Profile(selectedHost, assignmentRole(manifest))
		}
		team, queueErr := start.AppendQueue(ctx, st, selected, manifest.ID, manifest.Teams[0].ID, ids)
		if queueErr != nil {
			return startAdmissionError(args, stdout, stderr, queueErr)
		}
		if delta, reserved, reserveErr := start.ReserveRetainedHead(ctx, st, selected, team.ID); reserveErr != nil {
			return startAdmissionError(args, stdout, stderr, reserveErr)
		} else if delta.AlreadyAdmitted {
			return managementResult(args, stdout, map[string]any{"ok": true, "action": "start", "queue_appended": true, "host_followup_required": false, "already_admitted": true, "observation_required": true, "packet": delta.Packet, "packet_digest": delta.PacketDigest, "packet_path": delta.PacketPath, "retained_handle": delta.Retained, "team": delta.Team, "profile": profile})
		} else if reserved {
			return managementResult(args, stdout, map[string]any{"ok": true, "action": "start", "queue_appended": true, "host_followup_required": true, "already_admitted": false, "packet": delta.Packet, "packet_digest": delta.PacketDigest, "packet_path": delta.PacketPath, "retained_handle": delta.Retained, "team": delta.Team, "profile": profile})
		}
		return managementResult(args, stdout, map[string]any{"ok": true, "action": "start", "queue_appended": true, "team": team, "profile": profile})
	}
	teamID := core.TeamID(values["--team"])
	var team any
	current, currentErr := run.NewRepositories(st).Teams.Read(ctx, teamID)
	if currentErr != nil {
		return managementError(args, stdout, stderr, currentErr)
	}
	if selectedHost := retainedHost(current); selectedHost != "" {
		profile = settings.Profile(selectedHost, role)
	}
	switch values["--action"] {
	case "ack":
		handle := startHandle(values, current.RunID)
		team, err = start.Acknowledge(ctx, st, teamID, values["--packet-digest"], handle)
	case "complete":
		team, err = start.Complete(ctx, st, teamID, startHandle(values, current.RunID))
	case "clean":
		team, err = start.RecordIndependentClean(ctx, st, teamID, values["--reviewer"])
	case "idle":
		team, err = start.RecordIdle(ctx, st, teamID, startHandle(values, current.RunID))
	case "next":
		if delta, reserved, reserveErr := start.ReserveRetainedHead(ctx, st, selected, teamID); reserveErr != nil {
			err = reserveErr
		} else if delta.AlreadyAdmitted {
			return managementResult(args, stdout, map[string]any{"ok": true, "action": "start", "host_followup_required": false, "already_admitted": true, "observation_required": true, "packet": delta.Packet, "packet_digest": delta.PacketDigest, "packet_path": delta.PacketPath, "retained_handle": delta.Retained, "team": delta.Team, "profile": profile})
		} else if reserved {
			return managementResult(args, stdout, map[string]any{"ok": true, "action": "start", "host_followup_required": true, "already_admitted": false, "packet": delta.Packet, "packet_digest": delta.PacketDigest, "packet_path": delta.PacketPath, "retained_handle": delta.Retained, "team": delta.Team, "profile": profile})
		}
		if err != nil {
			break
		}
		if len(current.Queue) == 1 {
			consumed, retained, idle, terminalErr := start.ConsumeHead(ctx, st, teamID)
			if terminalErr != nil {
				err = terminalErr
			} else {
				return managementResult(args, stdout, map[string]any{"ok": true, "action": "start", "consumed": consumed, "host_followup_required": false, "retained_handle": retained, "team": idle, "profile": profile})
			}
			break
		}
		consumed, delta, waiting, nextErr := start.ConsumeForFollowup(ctx, st, selected, teamID)
		if nextErr != nil {
			err = nextErr
		} else {
			return managementResult(args, stdout, map[string]any{"ok": true, "action": "start", "consumed": consumed, "host_followup_required": true, "packet": delta.Packet, "packet_digest": delta.PacketDigest, "packet_path": delta.PacketPath, "retained_handle": delta.Retained, "team": waiting, "profile": profile})
		}
	}
	if err != nil {
		return startAdmissionError(args, stdout, stderr, err)
	}
	return managementResult(args, stdout, map[string]any{"ok": true, "action": "start", "team": team, "profile": profile})
}

func startAdmissionError(args []string, stdout, stderr io.Writer, err error) int {
	var held *start.AdmissionHeldError
	if !errors.As(err, &held) {
		return managementError(args, stdout, stderr, err)
	}
	output := map[string]any{"ok": true, "action": "start", "status": "admission_held", "admission_held": true, "host_dispatch_required": false, "host_followup_required": false, "control": held.Control}
	addControlGuidance(output, []start.ControlResult{held.Control})
	return managementResult(args, stdout, output)
}

func addControlGuidance(output map[string]any, controls []start.ControlResult) bool {
	for _, control := range controls {
		if !control.AdmissionHeld && !control.ObservationRequired {
			continue
		}
		output["controls"] = controls
		output["status"], output["next_action"] = "admission_held", "resume"
		output["message"] = "Admission is held. Run resume for the listed scope when ready, then continue its existing workers."
		if !control.AdmissionHeld {
			output["status"] = control.Status
		}
		if control.ObservationRequired {
			output["next_action"] = "observe_control"
			output["message"] = "Complete the requested native host action for the listed exact handles and acknowledge its observation. Then resume the held scope when ready; keep existing assignments."
			if control.Action == "resume" {
				output["message"] = "Continue the pending exact handles through their native host using the existing assignments, then acknowledge running observations."
			}
			if len(control.UnacknowledgedTeams) > 0 {
				output["message"] = "Reconcile the listed unacknowledged teams to recover their actual host handles, then repeat the control request and record its observations."
			}
		}
		return true
	}
	return false
}

func retainedHost(team run.TeamRecord) string {
	if team.Handle.Host != "" {
		return team.Handle.Host
	}
	return team.RetainedHandle.Host
}

func startValues(args []string) map[string]string {
	values := make(map[string]string, len(args)/2)
	for index := 0; index+1 < len(args); index += 2 {
		values[args[index]] = args[index+1]
	}
	return values
}

func startTaskIDs(args []string) []core.TaskID {
	var ids []core.TaskID
	for index := 0; index+1 < len(args); index += 2 {
		if args[index] == "--task" {
			ids = append(ids, core.TaskID(args[index+1]))
		}
	}
	return ids
}

func startHandle(values map[string]string, runID core.RunID) contracts.WorkerHandle {
	return contracts.WorkerHandle{Host: values["--host"], Identity: values["--identity"], Run: runID, Team: core.TeamID(values["--team"]), Task: core.TaskID(values["--task"]), PacketDigest: values["--packet-digest"], CandidateRevision: values["--candidate"]}
}

func installEnvironment() map[string]string {
	return map[string]string{
		"LOCALAPPDATA":  os.Getenv("LOCALAPPDATA"),
		"XDG_DATA_HOME": os.Getenv("XDG_DATA_HOME"),
		"HOME":          os.Getenv("HOME"),
		"USERPROFILE":   os.Getenv("USERPROFILE"),
		"CODEX_HOME":    os.Getenv("CODEX_HOME"),
		"CLAUDE_HOME":   os.Getenv("CLAUDE_HOME"),
	}
}

func cutoverRequestAction(path string) (string, error) {
	if path == "" || !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return "", core.ErrPath
	}
	raw, _, err := store.New(filepath.Dir(path), core.StorageLimits{CanonicalBytes: 1 << 20}).ReadFile(filepath.Base(path), 1<<20)
	if err != nil || len(raw) == 0 {
		return "", core.ErrPath
	}
	var envelope struct {
		Action string `json:"action"`
	}
	if json.Unmarshal(raw, &envelope) != nil || envelope.Action == "" {
		return "", core.ErrPhase
	}
	return envelope.Action, nil
}

func recoverMutationLock(ctx context.Context, root string, args []string, proof store.HolderLiveness) (store.MutationOwner, error) {
	if len(args) < 8 || args[1] != "--mutation-lock" || args[3] != "--owner-token" || args[5] != "--operation" || args[7] != "--confirm-dead" {
		return store.MutationOwner{}, core.ErrPhase
	}
	request := store.MutationRecoveryRequest{Target: args[2], Token: args[4], OperationID: args[6]}
	return store.RecoverProjectMutation(ctx, root, request, proof)
}

func localRelease(requested string) (install.Release, error) {
	executable, err := os.Executable()
	if err != nil {
		return install.Release{}, err
	}
	return localReleaseFrom(filepath.Dir(executable), executable, requested)
}

func localReleaseFrom(root, executable, requested string) (install.Release, error) {
	manifestRaw, err := os.ReadFile(filepath.Join(root, "RELEASE.json"))
	if err != nil {
		return install.Release{}, core.ErrRevision
	}
	var manifest releasepkg.Manifest
	decoder := json.NewDecoder(bytes.NewReader(manifestRaw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&manifest) != nil || decoder.Decode(&struct{}{}) != io.EOF || releasepkg.VerifyManifest(manifest) != nil || (requested != "" && requested != manifest.Version) {
		return install.Release{}, core.ErrRevision
	}
	wantMembers := []string{"WORKER-CONTRACT", "VERSION", "claude/SKILL.md", "codex/SKILL.md", manifest.Executable}
	sort.Strings(wantMembers)
	if len(manifest.Files) != len(wantMembers) {
		return install.Release{}, core.ErrRevision
	}
	for index := range wantMembers {
		if manifest.Files[index] != wantMembers[index] {
			return install.Release{}, core.ErrRevision
		}
	}
	relativeExecutable, err := filepath.Rel(root, executable)
	if err != nil || filepath.ToSlash(relativeExecutable) != manifest.Executable {
		return install.Release{}, core.ErrPath
	}
	checksums, err := readDistributionChecksums(root, manifest.Version)
	if err != nil {
		return install.Release{}, err
	}
	if digestBytes(manifestRaw) != checksums["RELEASE.json"] {
		return install.Release{}, core.ErrRevision
	}
	var sbom releasepkg.SBOM
	if err := readStrictJSON(filepath.Join(root, "SBOM.cdx.json"), &sbom); err != nil || releasepkg.VerifySBOM(sbom, manifest) != nil {
		return install.Release{}, core.ErrRevision
	}
	if err := verifyDistributionFiles(root, manifest, checksums); err != nil {
		return install.Release{}, err
	}
	versionRaw, err := os.ReadFile(filepath.Join(root, "VERSION"))
	if err != nil || strings.TrimSpace(string(versionRaw)) != manifest.Version {
		return install.Release{}, core.ErrRevision
	}
	file := func(name string) install.ReleaseFile {
		path := filepath.Join(root, filepath.FromSlash(name))
		info, _ := os.Stat(path)
		return install.ReleaseFile{Path: path, SHA256: manifest.Checksums[name], Bytes: info.Size()}
	}
	rel := install.Release{Version: manifest.Version, Revision: manifest.Commit, Binary: file(manifest.Executable), Contract: file("WORKER-CONTRACT"), Entrypoints: map[install.Host]install.ReleaseFile{install.Codex: file("codex/SKILL.md"), install.Claude: file("claude/SKILL.md")}}
	if err := install.VerifyRelease(rel); err != nil {
		return install.Release{}, err
	}
	return rel, nil
}

func readDistributionChecksums(root, version string) (map[string]string, error) {
	raw, err := os.ReadFile(filepath.Join(root, "SHA256SUMS"))
	if err != nil {
		return nil, core.ErrRevision
	}
	want := []string{"RELEASE.json", "SBOM.cdx.json", "agent-teamctl-" + version + ".zip"}
	sort.Strings(want)
	lines := strings.Split(strings.TrimSuffix(string(raw), "\n"), "\n")
	if len(lines) != len(want) {
		return nil, core.ErrRevision
	}
	checksums := map[string]string{}
	for index, line := range lines {
		if len(line) < 67 || line[64:66] != "  " || line[66:] != want[index] || !validDigest(line[:64]) {
			return nil, core.ErrRevision
		}
		checksums[want[index]] = line[:64]
	}
	return checksums, nil
}

func verifyDistributionFiles(root string, manifest releasepkg.Manifest, checksums map[string]string) error {
	want := map[string]bool{"RELEASE.json": true, "SBOM.cdx.json": true, "SHA256SUMS": true, "agent-teamctl-" + manifest.Version + ".zip": true}
	for _, name := range manifest.Files {
		want[name] = true
		path := filepath.Join(root, filepath.FromSlash(name))
		if digest, err := digestRegular(path); err != nil || digest != manifest.Checksums[name] {
			return core.ErrRevision
		}
	}
	for name, digest := range checksums {
		if got, err := digestRegular(filepath.Join(root, name)); err != nil || got != digest {
			return core.ErrRevision
		}
	}
	wantDirs := map[string]bool{"codex": true, "claude": true}
	return filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return core.ErrPath
		}
		if path == root {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return core.ErrPath
		}
		relative = filepath.ToSlash(relative)
		if entry.Type()&os.ModeSymlink != 0 {
			return core.ErrPath
		}
		if entry.IsDir() {
			if !wantDirs[relative] {
				return core.ErrPath
			}
			return nil
		}
		if !entry.Type().IsRegular() || !want[relative] {
			return core.ErrPath
		}
		return nil
	})
}

func readStrictJSON(path string, value any) error {
	if path == "" || !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return core.ErrPath
	}
	raw, _, err := store.New(filepath.Dir(path), core.StorageLimits{CanonicalBytes: 16 << 20}).ReadFile(filepath.Base(path), 16<<20)
	if err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return core.ErrRevision
	}
	return nil
}

func digestRegular(path string) (string, error) {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() {
		return "", core.ErrPath
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return digestBytes(body), nil
}

func digestBytes(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

func validDigest(value string) bool {
	if len(value) != 64 || value != strings.ToLower(value) {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func managementError(args []string, stdout, stderr io.Writer, err error) int {
	if len(args) > 0 && args[len(args)-1] == "--json" {
		return managementResult(args, stdout, map[string]any{"ok": false, "error": err.Error()})
	}
	_, _ = fmt.Fprintln(stderr, err)
	return 1
}

func managementResult(args []string, stdout io.Writer, value map[string]any) int {
	if len(args) > 0 && args[len(args)-1] == "--json" {
		raw, err := json.Marshal(value)
		if err != nil {
			return 1
		}
		_, err = stdout.Write(raw)
		if err != nil {
			return 1
		}
		if ok, _ := value["ok"].(bool); !ok {
			return 1
		}
		return 0
	}
	ok, _ := value["ok"].(bool)
	action, _ := value["action"].(string)
	if action == "" && len(args) > 0 {
		action = args[0]
	}
	if action == "" {
		action = "Command"
	}
	message, _ := value["message"].(string)
	status, _ := value["status"].(string)
	dispatch, _ := value["host_dispatch_required"].(bool)
	followup, _ := value["host_followup_required"].(bool)
	observe, _ := value["observation_required"].(bool)
	if message == "" && !ok {
		message, _ = value["error"].(string)
	}
	if message == "" {
		switch {
		case status != "":
			message = action + ": " + strings.ReplaceAll(status, "_", " ")
		case !ok:
			message = action + " failed"
		case dispatch || followup:
			message = action + ": packet reserved"
		case observe:
			message = action + ": existing reservation"
		default:
			message = action + " completed"
		}
	}
	lines := []string{message}
	var required []string
	switch fields := value["required_fields"].(type) {
	case []string:
		required = fields
	case []any:
		for _, field := range fields {
			if name, ok := field.(string); ok {
				required = append(required, name)
			}
		}
	}
	if len(required) != 0 {
		lines = append(lines, "Required: "+strings.Join(required, ", "))
	}
	if dispatch {
		lines = append(lines, "Host dispatch required; acknowledge the returned worker handle.")
	}
	if followup {
		lines = append(lines, "Host follow-up required; resume the retained worker and acknowledge its handle.")
	}
	if observe {
		lines = append(lines, "Observe the existing worker before taking further action.")
	}
	if next, _ := value["next_action"].(string); next != "" {
		lines = append(lines, "Next: "+strings.ReplaceAll(next, "_", " "))
	}
	_, err := fmt.Fprintln(stdout, strings.Join(lines, "\n"))
	if err != nil {
		return 1
	}
	if !ok {
		return 1
	}
	return 0
}
