package project

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/thebpandey/agent-team/vnext/internal/core"
	"github.com/thebpandey/agent-team/vnext/internal/migrate"
	"github.com/thebpandey/agent-team/vnext/internal/store"
	"github.com/thebpandey/agent-team/vnext/internal/testkit"
)

func TestContainRejectsEscapesAndLinks(t *testing.T) {
	root := t.TempDir()
	inside := filepath.Join(root, "inside")
	if err := os.Mkdir(inside, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := Contain(root, filepath.Join(root, "..", "outside")); !errors.Is(err, core.ErrPath) {
		t.Fatalf("escape error = %v, want ErrPath", err)
	}
	if err := os.Symlink(t.TempDir(), filepath.Join(inside, "escape")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, err := Contain(root, filepath.Join(inside, "escape", "file")); !errors.Is(err, core.ErrPath) {
		t.Fatalf("link escape error = %v, want ErrPath", err)
	}
	got, err := Contain(root, filepath.Join(inside, "new-file"))
	if err != nil || got != filepath.Join(inside, "new-file") {
		t.Fatalf("contained path = %q, %v", got, err)
	}
}

func TestValidateSegmentPortable(t *testing.T) {
	for _, value := range []string{"", ".", "..", "CON", "nul.txt", "a/b", `a\\b`, "file.", "file ", "a:b", "a?b", "é"} {
		if err := ValidateSegment(value); !errors.Is(err, core.ErrPath) {
			t.Errorf("ValidateSegment(%q) = %v, want ErrPath", value, err)
		}
	}
	if err := ValidateSegment("Tasks-01.md"); err != nil {
		t.Fatalf("ordinary segment rejected: %v", err)
	}
}

func TestContainRejectsCaseAlias(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "Plans"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := Contain(root, filepath.Join(root, "plans", "next.md")); !errors.Is(err, core.ErrPath) {
		t.Fatalf("case alias error = %v, want ErrPath", err)
	}
}

func TestContainRejectsPortableWindowsAliases(t *testing.T) {
	root := t.TempDir()
	for _, candidate := range []string{filepath.Join(root, "NUL"), filepath.Join(root, "file:stream"), `\\?\C:\work`, `\\.\PIPE\agent-team`} {
		if _, err := Contain(root, candidate); !errors.Is(err, core.ErrPath) {
			t.Errorf("Contain(%q) = %v, want ErrPath", candidate, err)
		}
	}
}

func TestDiscoverReportsGitIdentityAndState(t *testing.T) {
	root := testkit.GitRepo(t)
	project, err := Discover(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if project.Root != root || project.TopLevel != root || project.Head == "" || project.CommonDir == "" {
		t.Fatalf("incomplete project identity: %#v", project)
	}
	if project.Dirty || project.Detached || !project.Readable || project.Writable || project.FreeBytes <= 0 {
		t.Fatalf("incorrect clean project state: %#v", project)
	}
	if err := os.WriteFile(filepath.Join(root, "dirty.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	project, err = Discover(context.Background(), root)
	if err != nil || !project.Dirty {
		t.Fatalf("dirty project = %#v, %v", project, err)
	}
}

func TestNoKickoffIsAbsentAndBeadsIsExplicitTracker(t *testing.T) {
	root := testkit.GitRepo(t)
	for name, contents := range map[string]string{"DECISIONS.md": "# Decisions\n", "AGENT_TEAM_RULES.md": "# Rules\n"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(contents), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(root, ".beads"), 0o755); err != nil {
		t.Fatal(err)
	}
	input := SetupInput{Root: root, Mode: PlanMode, Artifacts: []ArtifactDecision{
		{Path: ".beads", Mode: ExistingArtifact, Confirmation: Approved},
		{Path: "DECISIONS.md", Mode: ExistingArtifact, Confirmation: Approved},
		{Path: "AGENT_TEAM_RULES.md", Mode: ExistingArtifact, Confirmation: Approved},
	}}
	validated, err := ValidateSetup(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if validated.Handoff.ApprovedPlanRevision != "" || validated.Handoff.Branch != "" || validated.Handoff.TrackerKind != "" || validated.Handoff.TrackerRef != "" || validated.Handoff.TrackerRevision != 0 || len(validated.Handoff.TaskIDs) != 0 || len(validated.Handoff.Acceptance) != 0 || len(validated.Handoff.Checks) != 0 || len(validated.Handoff.WritablePaths) != 0 || len(validated.Handoff.Resources) != 0 || len(validated.Handoff.Capabilities) != 0 {
		t.Fatalf("no-Kickoff handoff = %#v, want zero", validated.Handoff)
	}
	initialized, err := NewSetupService(store.New(root, core.DefaultConfig().Storage)).Initialize(context.Background(), input)
	if err != nil || initialized.Config.Tracker.Kind != "beads" || initialized.Config.Tracker.Path != ".beads" {
		t.Fatalf("Beads setup = %#v, %v", initialized, err)
	}
}

func TestSettingsServicePersistsReceiptBoundOverlayWithoutChangingSetupFiles(t *testing.T) {
	root := testkit.GitRepo(t)
	for name, contents := range map[string]string{
		"DECISIONS.md":        "# Decisions\n",
		"AGENT_TEAM_RULES.md": "# Rules\n",
	} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(contents), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(root, ".beads"), 0o755); err != nil {
		t.Fatal(err)
	}
	beadsPath := filepath.Join(root, ".beads", "issues.json")
	if err := os.WriteFile(beadsPath, []byte(`[{"id":"B-1"}]\n`), 0o600); err != nil {
		t.Fatal(err)
	}
	setup := NewSetupService(store.New(root, core.DefaultConfig().Storage))
	if _, err := setup.Initialize(context.Background(), SetupInput{Root: root, Mode: PlanMode, Artifacts: []ArtifactDecision{
		{Path: ".beads", Mode: ExistingArtifact, Confirmation: Approved},
		{Path: "DECISIONS.md", Mode: ExistingArtifact, Confirmation: Approved},
		{Path: "AGENT_TEAM_RULES.md", Mode: ExistingArtifact, Confirmation: Approved},
	}}); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(root, ".agent-team", "config.json")
	configBefore, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	var config configRecord
	if err := json.Unmarshal(configBefore, &config); err != nil {
		t.Fatal(err)
	}
	receiptPath := filepath.Join(root, filepath.FromSlash(config.ReceiptPath))
	receiptBefore, err := os.ReadFile(receiptPath)
	if err != nil {
		t.Fatal(err)
	}
	beadsBefore, err := os.ReadFile(beadsPath)
	if err != nil {
		t.Fatal(err)
	}

	service := NewSettingsService(store.New(root, core.DefaultConfig().Storage))
	initial, err := service.Inspect(context.Background())
	if err != nil || initial.Revision != 0 || initial.Defaults.ParallelTeams != 1 || initial.Defaults.Continuous || initial.Defaults.AutoDeploy || initial.Defaults.DeployBatchTasks != nil {
		t.Fatalf("initial settings = %#v, %v", initial, err)
	}
	if _, err := os.Stat(filepath.Join(root, ".agent-team", "v8", "settings.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("inspection wrote overlay: %v", err)
	}
	changed, err := service.Update(context.Background(), map[string]string{"parallel_teams": "3", "continuous": "true", "auto_deploy": "true", "deploy_batch_tasks": "2"})
	if err != nil || changed.Revision != 1 || changed.Defaults.ParallelTeams != 3 || !changed.Defaults.Continuous || !changed.Defaults.AutoDeploy || changed.Defaults.DeployBatchTasks == nil || *changed.Defaults.DeployBatchTasks != 2 {
		t.Fatalf("changed settings = %#v, %v", changed, err)
	}
	raw, err := os.ReadFile(filepath.Join(root, ".agent-team", "v8", "settings.json"))
	if err != nil || !bytes.Contains(raw, []byte(`"baseConfigDigest"`)) || !bytes.Contains(raw, []byte(`"receiptDigest"`)) {
		t.Fatalf("overlay bytes = %q, %v", raw, err)
	}
	reloaded, err := service.Inspect(context.Background())
	if err != nil || reloaded.Revision != 1 || reloaded.Defaults.ParallelTeams != 3 {
		t.Fatalf("reloaded settings = %#v, %v", reloaded, err)
	}
	var saved map[string]any
	if err := json.Unmarshal(raw, &saved); err != nil {
		t.Fatal(err)
	}
	saved["hosts"] = map[string]any{"claude": map[string]any{"roles": map[string]any{"developer": map[string]any{"model": "custom-kept"}}}}
	withHost, _ := json.Marshal(saved)
	if err := os.WriteFile(filepath.Join(root, ".agent-team", "v8", "settings.json"), withHost, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Update(context.Background(), map[string]string{"continuous": "false"}); err != nil {
		t.Fatal(err)
	}
	preserved, err := os.ReadFile(filepath.Join(root, ".agent-team", "v8", "settings.json"))
	if err != nil || !bytes.Contains(preserved, []byte(`"custom-kept"`)) {
		t.Fatalf("unknown host choice was not preserved: %q, %v", preserved, err)
	}
	profiled, err := service.Update(context.Background(), map[string]string{"codex.developer.model": "gpt-6", "codex.developer.effort": "high"})
	if err != nil || profiled.CodexDeveloper.Model != "gpt-6" || profiled.CodexDeveloper.Effort != "high" {
		t.Fatalf("saved Codex profile = %#v, %v", profiled, err)
	}
	profileReloaded, err := service.Inspect(context.Background())
	if err != nil || profileReloaded.CodexDeveloper != profiled.CodexDeveloper {
		t.Fatalf("reloaded Codex profile = %#v, %v", profileReloaded, err)
	}
	if _, err := service.Update(context.Background(), map[string]string{"parallel_teams": "7"}); !errors.Is(err, core.ErrSettings) {
		t.Fatalf("out-of-range update = %v", err)
	}
	if _, err := service.Update(context.Background(), map[string]string{"runtime.kind": "native"}); !errors.Is(err, core.ErrSettings) {
		t.Fatalf("unknown update = %v", err)
	}
	configAfter, _ := os.ReadFile(configPath)
	receiptAfter, _ := os.ReadFile(receiptPath)
	beadsAfter, _ := os.ReadFile(beadsPath)
	if !bytes.Equal(configBefore, configAfter) || !bytes.Equal(receiptBefore, receiptAfter) || !bytes.Equal(beadsBefore, beadsAfter) {
		t.Fatal("settings changed protected setup or tracker bytes")
	}
}

func TestSettingsServiceAcceptsMigratedAuthorityProjectionWithoutConfig(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".agent-team", "v8"), 0o700); err != nil {
		t.Fatal(err)
	}
	receipt := migrate.AuthorityReceipt{Schema: 1, Project: root, OperationID: "cutover", RequestDigest: "request", TargetRevision: strings.Repeat("a", 40), ApprovalID: "approval", ApprovalSHA256: strings.Repeat("b", 64), Tracker: migrate.TrackerAuthority{Fingerprint: strings.Repeat("c", 64), ParentID: "TASK-1", TaskIDs: []string{"TASK-1"}, TaskCount: 1}}
	encoded, err := json.Marshal(receipt)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(encoded)
	receipt.ReceiptDigest = hex.EncodeToString(sum[:])
	if _, err := store.New(root, core.DefaultConfig().Storage).WriteJSON(".agent-team/v8/authority.json", receipt, core.DefaultConfig().Storage.CanonicalBytes); err != nil {
		t.Fatal(err)
	}
	setup := map[string]any{
		"schema": 1, "authority": "v8", "project": root, "revision": receipt.TargetRevision,
		"tracker":     map[string]any{"kind": "beads", "fingerprint": receipt.Tracker.Fingerprint, "parentId": receipt.Tracker.ParentID},
		"receiptPath": ".agent-team/v8/authority.json", "receiptDigest": receipt.ReceiptDigest,
	}
	if _, err := store.New(root, core.DefaultConfig().Storage).WriteJSON(".agent-team/setup.json", setup, core.DefaultConfig().Storage.CanonicalBytes); err != nil {
		t.Fatal(err)
	}
	setupPath := filepath.Join(root, ".agent-team", "setup.json")
	authorityPath := filepath.Join(root, ".agent-team", "v8", "authority.json")
	setupBefore, _ := os.ReadFile(setupPath)
	authorityBefore, _ := os.ReadFile(authorityPath)
	updated, err := NewSettingsService(store.New(root, core.DefaultConfig().Storage)).Update(context.Background(), map[string]string{"parallel_teams": "2"})
	if err != nil || updated.Revision != 1 || updated.Defaults.ParallelTeams != 2 {
		t.Fatalf("migrated settings = %#v, %v", updated, err)
	}
	setupAfter, _ := os.ReadFile(setupPath)
	authorityAfter, _ := os.ReadFile(authorityPath)
	if !bytes.Equal(setupBefore, setupAfter) || !bytes.Equal(authorityBefore, authorityAfter) {
		t.Fatal("settings changed migrated authority bytes")
	}
}

func TestSettingsServiceRejectsIncompleteMigratedAuthorityWithoutWriting(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".agent-team", "v8"), 0o700); err != nil {
		t.Fatal(err)
	}
	receipt := migrate.AuthorityReceipt{Schema: 1, Project: root, OperationID: "cutover", RequestDigest: "request", TargetRevision: strings.Repeat("a", 40), ApprovalID: "approval", ApprovalSHA256: strings.Repeat("b", 64)}
	encoded, _ := json.Marshal(receipt)
	sum := sha256.Sum256(encoded)
	receipt.ReceiptDigest = hex.EncodeToString(sum[:])
	state := store.New(root, core.DefaultConfig().Storage)
	if _, err := state.WriteJSON(".agent-team/v8/authority.json", receipt, core.DefaultConfig().Storage.CanonicalBytes); err != nil {
		t.Fatal(err)
	}
	setup := map[string]any{"schema": 1, "authority": "v8", "project": root, "revision": receipt.TargetRevision, "tracker": map[string]any{"kind": "beads", "fingerprint": "", "parentId": ""}, "receiptPath": ".agent-team/v8/authority.json", "receiptDigest": receipt.ReceiptDigest}
	if _, err := state.WriteJSON(".agent-team/setup.json", setup, core.DefaultConfig().Storage.CanonicalBytes); err != nil {
		t.Fatal(err)
	}
	setupPath := filepath.Join(root, ".agent-team", "setup.json")
	authorityPath := filepath.Join(root, ".agent-team", "v8", "authority.json")
	setupBefore, _ := os.ReadFile(setupPath)
	authorityBefore, _ := os.ReadFile(authorityPath)
	if _, err := NewSettingsService(state).Update(context.Background(), map[string]string{"parallel_teams": "2"}); !errors.Is(err, core.ErrRevision) {
		t.Fatalf("incomplete authority accepted: %v", err)
	}
	setupAfter, _ := os.ReadFile(setupPath)
	authorityAfter, _ := os.ReadFile(authorityPath)
	if !bytes.Equal(setupBefore, setupAfter) || !bytes.Equal(authorityBefore, authorityAfter) {
		t.Fatal("rejected authority changed protected bytes")
	}
	if _, err := os.Stat(filepath.Join(root, ".agent-team", "v8", "settings.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("rejected authority wrote settings: %v", err)
	}
}

func TestBeadsTraversalRejectsNestedLink(t *testing.T) {
	root := testkit.GitRepo(t)
	for name, contents := range map[string]string{"DECISIONS.md": "# Decisions\n", "AGENT_TEAM_RULES.md": "# Rules\n"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(contents), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Join(root, ".beads", "nested"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(t.TempDir(), filepath.Join(root, ".beads", "nested", "outside")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	input := SetupInput{Root: root, Mode: PlanMode, Artifacts: []ArtifactDecision{
		{Path: ".beads", Mode: ExistingArtifact, Confirmation: Approved},
		{Path: "DECISIONS.md", Mode: ExistingArtifact, Confirmation: Approved},
		{Path: "AGENT_TEAM_RULES.md", Mode: ExistingArtifact, Confirmation: Approved},
	}}
	if _, err := ValidateSetup(context.Background(), input); err == nil {
		t.Fatal("nested Beads symlink was accepted")
	}
}

func TestValidateAndRefusalLeaveFilesystemUntouched(t *testing.T) {
	root := testkit.GitRepo(t)
	before := testkit.SnapshotProjectTree(t, root)
	refused := SetupInput{Root: root, Mode: PlanMode, Artifacts: []ArtifactDecision{{Path: "TASKS.md", Mode: ExistingArtifact, Confirmation: Refused}}}
	if _, err := NewSetupService(store.New(root, core.DefaultConfig().Storage)).Validate(context.Background(), refused); !errors.Is(err, core.ErrSettings) {
		t.Fatalf("refused validation error = %v", err)
	}
	if after := testkit.SnapshotProjectTree(t, root); !equalTree(before, after) {
		t.Fatal("Validate/refusal changed filesystem")
	}
}

func TestSetupValidatesDecisionsAndPersistsIdempotently(t *testing.T) {
	root := testkit.GitRepo(t)
	for name, contents := range map[string]string{"TASKS.md": "# Tasks\n", "DECISIONS.md": "# Decisions\n", "AGENT_TEAM_RULES.md": "# Rules\n"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(contents), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	handoff := core.KickoffHandoff{
		ApprovedPlanRevision: "plan-7", Branch: "main", TrackerKind: "tasks-md", TrackerRef: "TASKS.md", TrackerRevision: 3,
		TaskIDs: []core.TaskID{"TASK-1"}, Acceptance: []string{"go test ./..."}, Checks: []core.Check{{Name: "test", Command: []string{"go", "test", "./..."}}},
		WritablePaths: []string{"vnext"}, Resources: []string{"network:none"}, Capabilities: []string{"git"},
	}
	encoded, err := json.Marshal(handoff)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "docs", "kickoff.json"), encoded, 0o644); err != nil {
		t.Fatal(err)
	}
	kickoffDigest := digest(encoded)

	refused := SetupInput{Root: root, Mode: PlanMode, Artifacts: []ArtifactDecision{{Path: "TASKS.md", Mode: ExistingArtifact, Confirmation: Refused}}}
	if _, err := ValidateSetup(context.Background(), refused); !errors.Is(err, core.ErrSettings) {
		t.Fatalf("refused artifact error = %v, want ErrSettings", err)
	}
	approved := planInput(root, &KickoffDecision{Path: "docs/kickoff.json", Digest: kickoffDigest, Confirmation: Approved})
	validated, err := ValidateSetup(context.Background(), approved)
	if err != nil {
		t.Fatalf("approved setup: %v", err)
	}
	if validated.ArtifactDigests["TASKS.md"] == "" || validated.Handoff.ApprovedPlanRevision != "plan-7" || len(validated.Handoff.TaskIDs) != 1 {
		t.Fatalf("validated result missing facts: %#v", validated)
	}
	badDigest := approved
	badDigest.Kickoff = &KickoffDecision{Path: approved.Kickoff.Path, Digest: "sha256:bad", Confirmation: Approved}
	if _, err := ValidateSetup(context.Background(), badDigest); !errors.Is(err, core.ErrSettings) {
		t.Fatalf("bad kickoff digest error = %v, want ErrSettings", err)
	}
	incomplete := handoff
	incomplete.Capabilities = nil
	incompleteBytes, err := json.Marshal(incomplete)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "docs", "kickoff.json"), incompleteBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	incompleteInput := planInput(root, &KickoffDecision{Path: "docs/kickoff.json", Digest: digest(incompleteBytes), Confirmation: Approved})
	if _, err := ValidateSetup(context.Background(), incompleteInput); !errors.Is(err, core.ErrSettings) {
		t.Fatalf("incomplete kickoff error = %v, want ErrSettings", err)
	}
	if err := os.WriteFile(filepath.Join(root, "docs", "kickoff.json"), encoded, 0o644); err != nil {
		t.Fatal(err)
	}

	svc := NewSetupService(store.New(root, core.DefaultConfig().Storage))
	result, err := svc.Initialize(context.Background(), approved)
	if err != nil {
		t.Fatal(err)
	}
	if result.ConfigRevision != 1 || result.ReceiptPath == "" {
		t.Fatalf("bad initialized result: %#v", result)
	}
	before := testkit.SnapshotTree(t, root)
	if _, err := svc.Initialize(context.Background(), approved); err != nil {
		t.Fatalf("idempotent initialize: %v", err)
	}
	if after := testkit.SnapshotTree(t, root); !equalTree(before, after) {
		t.Fatal("idempotent initialize wrote state")
	}
	conflicting := approved
	conflicting.Kickoff = nil
	if _, err := svc.Initialize(context.Background(), conflicting); !errors.Is(err, core.ErrRevision) {
		t.Fatalf("conflicting initialize error = %v, want ErrRevision", err)
	}
}

func TestPlanMatrixAndOneOffPersistence(t *testing.T) {
	root := testkit.GitRepo(t)
	for name, contents := range map[string]string{"TASKS.md": "# Tasks\n", "DECISIONS.md": "# Decisions\n", "AGENT_TEAM_RULES.md": "# Rules\n"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(contents), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	missingGovernance := SetupInput{Root: root, Mode: PlanMode, Artifacts: []ArtifactDecision{{Path: "TASKS.md", Mode: ExistingArtifact, Confirmation: Approved}}}
	if _, err := ValidateSetup(context.Background(), missingGovernance); !errors.Is(err, core.ErrSettings) {
		t.Fatalf("missing governance error = %v, want ErrSettings", err)
	}
	duplicateAuthority := planInput(root, nil)
	duplicateAuthority.Artifacts = append(duplicateAuthority.Artifacts, ArtifactDecision{Path: ".beads", Mode: GeneratedArtifact, Confirmation: Approved})
	if _, err := ValidateSetup(context.Background(), duplicateAuthority); !errors.Is(err, core.ErrSettings) {
		t.Fatalf("duplicate tracker authority error = %v, want ErrSettings", err)
	}

	svc := NewSetupService(store.New(root, core.DefaultConfig().Storage))
	before := testkit.SnapshotTree(t, root)
	if _, err := svc.Initialize(context.Background(), SetupInput{Root: root, Mode: OneOffMode}); err != nil {
		t.Fatalf("one-off initialize: %v", err)
	}
	if after := testkit.SnapshotTree(t, root); !equalTree(before, after) {
		t.Fatal("one-off setup persisted project configuration")
	}
}

func TestSetupConfigAndReceiptHaveOneCommittedRevision(t *testing.T) {
	root := testkit.GitRepo(t)
	for name, contents := range map[string]string{"TASKS.md": "# Tasks\n", "DECISIONS.md": "# Decisions\n", "AGENT_TEAM_RULES.md": "# Rules\n"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(contents), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	svc := NewSetupService(store.New(root, core.DefaultConfig().Storage))
	if _, err := svc.Initialize(context.Background(), planInput(root, nil)); err != nil {
		t.Fatal(err)
	}
	var config map[string]any
	configBytes, err := os.ReadFile(filepath.Join(root, ".agent-team", "config.json"))
	if err != nil || json.Unmarshal(configBytes, &config) != nil {
		t.Fatal(err)
	}
	if config["project"] != root || config["revision"] != float64(1) {
		t.Fatalf("config is not an envelope-bearing revisioned record: %#v", config)
	}
	var receipt map[string]any
	receiptPath, ok := config["receiptPath"].(string)
	if !ok {
		t.Fatalf("config has no immutable receipt path: %#v", config)
	}
	receiptBytes, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(receiptPath)))
	if err != nil || json.Unmarshal(receiptBytes, &receipt) != nil {
		t.Fatal(err)
	}
	if receipt["revision"] != config["revision"] || receipt["configDigest"] == "" {
		t.Fatalf("config/receipt do not share a committed revision: config=%#v receipt=%#v", config, receipt)
	}
}

func TestSetupPublishFailureIsInvisibleAndRecoverable(t *testing.T) {
	root := testkit.GitRepo(t)
	for name, contents := range map[string]string{"TASKS.md": "# Tasks\n", "DECISIONS.md": "# Decisions\n", "AGENT_TEAM_RULES.md": "# Rules\n"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(contents), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	s := store.New(root, core.DefaultConfig().Storage)
	fail := true
	svc := &setupService{store: s, writeJSON: func(relative string, value any, max int64) (store.AtomicResult, error) {
		if strings.HasPrefix(relative, ".agent-team/receipts/") && fail {
			fail = false
			return store.AtomicResult{}, errors.New("injected receipt interruption")
		}
		return s.WriteJSON(relative, value, max)
	}}
	input := planInput(root, nil)
	if _, err := svc.Initialize(context.Background(), input); err == nil {
		t.Fatal("injected publication interruption succeeded")
	}
	before := testkit.SnapshotTree(t, root)
	if _, err := svc.Validate(context.Background(), input); err != nil {
		t.Fatalf("uncommitted publication was visible: %v", err)
	}
	if after := testkit.SnapshotTree(t, root); !equalTree(before, after) {
		t.Fatal("Validate wrote while inspecting interrupted publication")
	}
	if _, err := NewSetupService(s).Initialize(context.Background(), input); err != nil {
		t.Fatalf("recovery initialize: %v", err)
	}
	if _, err := NewSetupService(s).Validate(context.Background(), input); err != nil {
		t.Fatalf("recovered publication did not validate: %v", err)
	}
}

func TestConcurrentInitializersUseOneCommittedCAS(t *testing.T) {
	root := testkit.GitRepo(t)
	for name, contents := range map[string]string{"TASKS.md": "# Tasks\n", "DECISIONS.md": "# Decisions\n", "AGENT_TEAM_RULES.md": "# Rules\n"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(contents), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	input := planInput(root, nil)
	results := make(chan error, 2)
	var group sync.WaitGroup
	for range 2 {
		group.Add(1)
		go func() {
			defer group.Done()
			_, err := NewSetupService(store.New(root, core.DefaultConfig().Storage)).Initialize(context.Background(), input)
			results <- err
		}()
	}
	group.Wait()
	close(results)
	for err := range results {
		if err != nil {
			t.Fatalf("same-input concurrent initializer: %v", err)
		}
	}
}

func TestLinkCASRecoversAbandonedStagingAndFailsSafelyWhenUnsupported(t *testing.T) {
	root := testkit.GitRepo(t)
	for name, contents := range map[string]string{"TASKS.md": "# Tasks\n", "DECISIONS.md": "# Decisions\n", "AGENT_TEAM_RULES.md": "# Rules\n"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(contents), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Join(root, ".agent-team", "setup"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".agent-team", "setup", "abandoned.json"), []byte("staging"), 0o600); err != nil {
		t.Fatal(err)
	}
	input := planInput(root, nil)
	if _, err := NewSetupService(store.New(root, core.DefaultConfig().Storage)).Initialize(context.Background(), input); err != nil {
		t.Fatalf("abandoned staging blocked next session: %v", err)
	}

	unsupported := testkit.GitRepo(t)
	for name, contents := range map[string]string{"TASKS.md": "# Tasks\n", "DECISIONS.md": "# Decisions\n", "AGENT_TEAM_RULES.md": "# Rules\n"} {
		if err := os.WriteFile(filepath.Join(unsupported, name), []byte(contents), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	s := store.New(unsupported, core.DefaultConfig().Storage)
	svc := &setupService{store: s, link: func(*os.Root, string, string) error { return errors.New("link unsupported") }}
	if _, err := svc.Initialize(context.Background(), planInput(unsupported, nil)); err == nil {
		t.Fatal("unsupported link succeeded")
	}
	if _, err := os.Stat(filepath.Join(unsupported, ".agent-team", "config.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("unsupported link published config: %v", err)
	}
}

func TestCommittedConfigRejectsAlternateReceiptPathAndBadEnvelope(t *testing.T) {
	root := testkit.GitRepo(t)
	for name, contents := range map[string]string{"TASKS.md": "# Tasks\n", "DECISIONS.md": "# Decisions\n", "AGENT_TEAM_RULES.md": "# Rules\n"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(contents), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	input := planInput(root, nil)
	result, err := ValidateSetup(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	inputDigest, err := setupDigest(result, input)
	if err != nil {
		t.Fatal(err)
	}
	record := configFrom(result.Config, result.Project.TopLevel, 1)
	record.ReceiptPath = ".agent-team/receipts/not-content-addressed.json"
	receipt := setupReceipt{RecordEnvelope: core.RecordEnvelope{Schema: 1, Project: result.Project.TopLevel, WrittenAt: record.WrittenAt, Revision: 1}, InputDigest: inputDigest}
	record.ReceiptDigest = digestReceiptBinding(receipt)
	receipt.ConfigDigest = digestRecord(record)
	s := store.New(root, core.DefaultConfig().Storage)
	if _, err := s.WriteJSON(record.ReceiptPath, receipt, core.DefaultConfig().Storage.CanonicalBytes); err != nil {
		t.Fatal(err)
	}
	if _, err := s.WriteJSON(configPath, record, core.DefaultConfig().Storage.CanonicalBytes); err != nil {
		t.Fatal(err)
	}
	if _, err := NewSetupService(s).Validate(context.Background(), input); !errors.Is(err, core.ErrRevision) {
		t.Fatalf("alternate receipt path error = %v, want ErrRevision", err)
	}
}

func planInput(root string, kickoff *KickoffDecision) SetupInput {
	return SetupInput{Root: root, Mode: PlanMode, Artifacts: []ArtifactDecision{
		{Path: "TASKS.md", Mode: ExistingArtifact, Confirmation: Approved},
		{Path: "DECISIONS.md", Mode: ExistingArtifact, Confirmation: Approved},
		{Path: "AGENT_TEAM_RULES.md", Mode: ExistingArtifact, Confirmation: Approved},
	}, Kickoff: kickoff}
}

func digest(value []byte) string {
	sum := sha256.Sum256(value)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func equalTree(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for path, value := range a {
		if b[path] != value {
			return false
		}
	}
	return true
}
