package main

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sync"
	"testing"

	"github.com/thebpandey/agent-team/vnext/internal/cli"
	"github.com/thebpandey/agent-team/vnext/internal/core"
	"github.com/thebpandey/agent-team/vnext/internal/install"
	"github.com/thebpandey/agent-team/vnext/internal/project"
	releasepkg "github.com/thebpandey/agent-team/vnext/internal/release"
	"github.com/thebpandey/agent-team/vnext/internal/store"
	"github.com/thebpandey/agent-team/vnext/internal/testkit"
)

const testRevision = "0123456789abcdef0123456789abcdef01234567"

func TestSetupCLIInitializesExistingBeadsProject(t *testing.T) {
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
	t.Chdir(root)
	var out bytes.Buffer
	if code := cli.Run(context.Background(), []string{"setup", "--mode", "plan", "--json"}, core.Dependencies{Stdout: &out, Stderr: &out, Management: runManagement}); code != 0 {
		t.Fatalf("code=%d output=%q", code, out.String())
	}
	if _, err := os.Stat(filepath.Join(root, ".agent-team", "config.json")); err != nil {
		t.Fatalf("setup did not initialize native project authority: %v; output=%q", err, out.String())
	}
	settings, err := project.NewSettingsService(store.New(root, core.DefaultConfig().Storage)).Inspect(context.Background())
	if err != nil || settings.ReceiptPath == "" || settings.ReceiptDigest == "" {
		t.Fatalf("setup did not establish settings/admission binding: settings=%#v err=%v", settings, err)
	}
}

func TestSettingsCLIUsesReceiptBoundPersistence(t *testing.T) {
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
	if _, err := project.NewSetupService(store.New(root, core.DefaultConfig().Storage)).Initialize(context.Background(), project.SetupInput{Root: root, Mode: project.PlanMode, Artifacts: []project.ArtifactDecision{
		{Path: ".beads", Mode: project.ExistingArtifact, Confirmation: project.Approved},
		{Path: "DECISIONS.md", Mode: project.ExistingArtifact, Confirmation: project.Approved},
		{Path: "AGENT_TEAM_RULES.md", Mode: project.ExistingArtifact, Confirmation: project.Approved},
	}}); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(root, ".agent-team", "config.json")
	configBefore, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	var config struct {
		ReceiptPath string `json:"receiptPath"`
	}
	if err := json.Unmarshal(configBefore, &config); err != nil {
		t.Fatal(err)
	}
	receiptPath := filepath.Join(root, filepath.FromSlash(config.ReceiptPath))
	receiptBefore, err := os.ReadFile(receiptPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Chdir(root)
	var out bytes.Buffer
	args := []string{"settings", "parallel_teams=2", "continuous=true", "--json"}
	if code := cli.Run(context.Background(), args, core.Dependencies{Stdout: &out, Stderr: &out, Management: runManagement}); code != 0 || !bytes.Contains(out.Bytes(), []byte(`"revision":1`)) {
		t.Fatalf("code=%d output=%q", code, out.String())
	}
	settingsRaw, err := os.ReadFile(filepath.Join(root, ".agent-team", "v8", "settings.json"))
	if err != nil || !bytes.Contains(settingsRaw, []byte(`"parallelTeams":2`)) {
		t.Fatalf("settings bytes=%q err=%v", settingsRaw, err)
	}
	reloaded, err := project.NewSettingsService(store.New(root, core.DefaultConfig().Storage)).Inspect(context.Background())
	if err != nil || reloaded.Revision != 1 || reloaded.Defaults.ParallelTeams != 2 || !reloaded.Defaults.Continuous {
		t.Fatalf("reloaded=%#v err=%v", reloaded, err)
	}
	configAfter, _ := os.ReadFile(configPath)
	receiptAfter, _ := os.ReadFile(receiptPath)
	if !bytes.Equal(configBefore, configAfter) || !bytes.Equal(receiptBefore, receiptAfter) {
		t.Fatal("CLI settings changed immutable setup bytes")
	}
}

func TestRollbackRevisionDispatchWithJSONAndUniqueCompatibility(t *testing.T) {
	const nextRevision = "abcdef0123456789abcdef0123456789abcdef01"
	for _, test := range []struct {
		name      string
		ambiguous bool
		args      []string
	}{
		{name: "revision with JSON", ambiguous: true, args: []string{"rollback", "--version", "8.0.0", "--revision", testRevision, "--json"}},
		{name: "unique without JSON", args: []string{"rollback", "--version", "8.0.0"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			dataHome := filepath.Join(root, "data")
			t.Setenv("LOCALAPPDATA", dataHome)
			t.Setenv("APPDATA", filepath.Join(root, "roaming"))
			t.Setenv("XDG_DATA_HOME", dataHome)
			t.Setenv("HOME", filepath.Join(root, "home"))
			t.Setenv("CODEX_HOME", filepath.Join(root, "codex"))
			t.Setenv("CLAUDE_HOME", filepath.Join(root, "claude"))
			layout, err := install.ResolveLayout(runtime.GOOS, map[string]string{"LOCALAPPDATA": os.Getenv("LOCALAPPDATA"), "XDG_DATA_HOME": os.Getenv("XDG_DATA_HOME"), "HOME": os.Getenv("HOME"), "CODEX_HOME": os.Getenv("CODEX_HOME"), "CLAUDE_HOME": os.Getenv("CLAUDE_HOME")})
			if err != nil {
				t.Fatal(err)
			}
			old := cliReleaseFixture(t, filepath.Join(root, "old"), "8.0.0", testRevision, "old")
			next := cliReleaseFixture(t, filepath.Join(root, "next"), "8.0.0", nextRevision, "next")
			var installed install.CASOutcome
			if test.ambiguous {
				legacy := cliReleaseFixture(t, filepath.Join(root, "legacy"), "8.0.0", "fedcba9876543210fedcba9876543210fedcba98", "legacy")
				old.Contract, old.Entrypoints = legacy.Contract, legacy.Entrypoints
				next.Contract, next.Entrypoints = legacy.Contract, legacy.Entrypoints
				installed, err = install.Install(context.Background(), layout, legacy, []install.Host{install.Codex, install.Claude}, 0)
				if err == nil {
					installed, err = install.Update(context.Background(), layout, old, installed.Manifest.Revision)
				}
			} else {
				installed, err = install.Install(context.Background(), layout, old, []install.Host{install.Codex, install.Claude}, 0)
			}
			if err != nil {
				t.Fatal(err)
			}
			updated, err := install.Update(context.Background(), layout, next, installed.Manifest.Revision)
			if err != nil {
				t.Fatal(err)
			}
			var output bytes.Buffer
			if code := cli.Run(context.Background(), test.args, core.Dependencies{Stdout: &output, Stderr: &output, Management: runManagement}); code != 0 {
				t.Fatalf("code=%d output=%q", code, output.String())
			}
			got, err := install.NewManifestStore(layout).Read(context.Background())
			if err != nil || got.ReleaseRevision != testRevision || got.Revision != updated.Manifest.Revision+1 {
				t.Fatalf("manifest=%+v err=%v output=%q", got, err, output.String())
			}
			for _, file := range got.Files {
				if file.Revision != testRevision {
					t.Fatalf("incoherent rollback file: %+v", file)
				}
			}
		})
	}
}

func TestLifecycleCLIReusesInstalledCustomHostHomesWithoutEnvironment(t *testing.T) {
	root := t.TempDir()
	dataHome := filepath.Join(root, "data")
	customCodex, customClaude := filepath.Join(root, "custom-codex"), filepath.Join(root, "custom-claude")
	t.Setenv("LOCALAPPDATA", dataHome)
	t.Setenv("XDG_DATA_HOME", dataHome)
	t.Setenv("HOME", filepath.Join(root, "default-home"))
	t.Setenv("CODEX_HOME", customCodex)
	t.Setenv("CLAUDE_HOME", customClaude)
	layout, err := install.ResolveLayout(runtime.GOOS, map[string]string{
		"LOCALAPPDATA": dataHome, "XDG_DATA_HOME": dataHome, "HOME": os.Getenv("HOME"), "USERPROFILE": os.Getenv("USERPROFILE"), "CODEX_HOME": customCodex, "CLAUDE_HOME": customClaude,
	})
	if err != nil {
		t.Fatal(err)
	}
	old := cliReleaseFixture(t, filepath.Join(root, "old"), "8.0.3", testRevision, "old")
	next := cliReleaseFixture(t, filepath.Join(root, "next"), "8.0.4", "abcdef0123456789abcdef0123456789abcdef01", "next")
	installed, err := install.Install(context.Background(), layout, old, []install.Host{install.Codex, install.Claude}, 0)
	if err != nil {
		t.Fatal(err)
	}
	updated, err := install.Update(context.Background(), layout, next, installed.Manifest.Revision)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("CODEX_HOME", "")
	t.Setenv("CLAUDE_HOME", "")
	var output bytes.Buffer
	if code := cli.Run(context.Background(), []string{"rollback", "--version", old.Version, "--revision", old.Revision, "--json"}, core.Dependencies{Stdout: &output, Stderr: &output, Management: runManagement}); code != 0 {
		t.Fatalf("env-free rollback code=%d output=%q", code, output.String())
	}
	manifest, err := install.NewManifestStore(layout).Read(context.Background())
	if err != nil || manifest.Revision != updated.Manifest.Revision+1 || manifest.HostHomes[install.Codex] != customCodex || manifest.HostHomes[install.Claude] != customClaude {
		t.Fatalf("rolled manifest=%+v err=%v", manifest, err)
	}
	for host, file := range old.Entrypoints {
		got, err := os.ReadFile(filepath.Join(layout.SkillRoots[host], "SKILL.md"))
		want, _ := os.ReadFile(file.Path)
		if err != nil || !bytes.Equal(got, want) {
			t.Fatalf("%s entrypoint=%q err=%v", host, got, err)
		}
	}
}

func cliReleaseFixture(t *testing.T, root, version, revision, marker string) install.Release {
	t.Helper()
	file := func(name string) install.ReleaseFile {
		path := filepath.Join(root, name)
		body := []byte(marker + " " + name)
		writeTestFile(t, path, string(body))
		return install.ReleaseFile{Path: path, SHA256: fmt.Sprintf("%x", sha256.Sum256(body)), Bytes: int64(len(body))}
	}
	return install.Release{Version: version, Revision: revision, Binary: file("agent-teamctl"), Contract: file("WORKER-CONTRACT"), Entrypoints: map[install.Host]install.ReleaseFile{install.Codex: file("codex.md"), install.Claude: file("claude.md")}}
}

type recoveryProof struct {
	dead    bool
	entered chan<- struct{}
	wait    <-chan struct{}
}

func (p recoveryProof) HolderDead(context.Context, store.MutationOwner) (bool, error) {
	if p.entered != nil {
		close(p.entered)
	}
	if p.wait != nil {
		<-p.wait
	}
	return p.dead, nil
}

func TestCleanupMutationLockRecoveryCLI(t *testing.T) {
	for _, test := range []struct {
		name  string
		alter func(store.MutationOwner) store.MutationOwner
		proof recoveryProof
		want  int
	}{
		{name: "proven dead", proof: recoveryProof{dead: true}, want: 0},
		{name: "live", proof: recoveryProof{}, want: 1},
		{name: "wrong owner", alter: func(owner store.MutationOwner) store.MutationOwner {
			owner.Token = "00000000000000000000000000000000"
			return owner
		}, proof: recoveryProof{dead: true}, want: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			guard, err := store.AcquireProjectMutation(context.Background(), root, "test", "cli-recovery")
			if err != nil {
				t.Fatal(err)
			}
			owner := guard.Owner()
			if test.alter != nil {
				owner = test.alter(owner)
			}
			args := recoveryCLIArgs(store.MutationLockPrimary, owner)
			var output bytes.Buffer
			code := cli.Run(context.Background(), args, core.Dependencies{Stdout: &output, Stderr: &output, Management: recoveryManagement(root, test.proof)})
			if code != test.want || (code == 0 && !bytes.Contains(output.Bytes(), []byte(`"recovered":true`))) {
				t.Fatalf("code=%d output=%q", code, output.String())
			}
		})
	}
}

func TestCleanupMutationLockRecoveryCLIRejectsCorruptAndIsSingleWinner(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, ".agent-team", "mutation.lock")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("corrupt"), 0o600); err != nil {
		t.Fatal(err)
	}
	owner := store.MutationOwner{Token: "0123456789abcdef0123456789abcdef", OperationID: "corrupt"}
	var output bytes.Buffer
	if code := cli.Run(context.Background(), recoveryCLIArgs(store.MutationLockPrimary, owner), core.Dependencies{Stdout: &output, Stderr: &output, Management: recoveryManagement(root, recoveryProof{dead: true})}); code != 1 {
		t.Fatalf("corrupt code=%d output=%q", code, output.String())
	}

	root = t.TempDir()
	guard, err := store.AcquireProjectMutation(context.Background(), root, "test", "concurrent-cli")
	if err != nil {
		t.Fatal(err)
	}
	args := recoveryCLIArgs(store.MutationLockPrimary, guard.Owner())
	codes := make(chan int, 2)
	var wait sync.WaitGroup
	wait.Add(2)
	for range 2 {
		go func() {
			defer wait.Done()
			codes <- cli.Run(context.Background(), args, core.Dependencies{Stdout: io.Discard, Stderr: io.Discard, Management: recoveryManagement(root, recoveryProof{dead: true})})
		}()
	}
	wait.Wait()
	close(codes)
	winners := 0
	for code := range codes {
		if code == 0 {
			winners++
		}
	}
	if winners != 1 {
		t.Fatalf("winners=%d", winners)
	}
	next, err := store.AcquireProjectMutation(context.Background(), root, "test", "after-cli-recovery")
	if err != nil {
		t.Fatal(err)
	}
	if err := next.Release(); err != nil {
		t.Fatal(err)
	}
}

func TestCleanupMutationLockRecoveryCLIRecoversClaimResidue(t *testing.T) {
	root := t.TempDir()
	primary, err := store.AcquireProjectMutation(context.Background(), root, "test", "claim-residue")
	if err != nil {
		t.Fatal(err)
	}
	entered, resume := make(chan struct{}), make(chan struct{})
	first := make(chan error, 1)
	go func() {
		_, err := store.RecoverProjectMutation(context.Background(), root, store.MutationRecoveryRequest{Target: store.MutationLockPrimary, Token: primary.Owner().Token, OperationID: primary.Owner().OperationID}, recoveryProof{dead: true, entered: entered, wait: resume})
		first <- err
	}()
	<-entered
	claim, err := store.MutationLockOwner(root, store.MutationLockRecovery)
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	code := cli.Run(context.Background(), recoveryCLIArgs(store.MutationLockRecovery, claim), core.Dependencies{Stdout: &output, Stderr: &output, Management: recoveryManagement(root, recoveryProof{dead: true})})
	if code != 0 {
		t.Fatalf("code=%d output=%q", code, output.String())
	}
	close(resume)
	if err := <-first; err != nil {
		t.Fatal(err)
	}
	next, err := store.AcquireProjectMutation(context.Background(), root, "test", "after-claim-residue")
	if err != nil {
		t.Fatal(err)
	}
	if err := next.Release(); err != nil {
		t.Fatal(err)
	}
}

func recoveryCLIArgs(target string, owner store.MutationOwner) []string {
	return []string{"cleanup", "--mutation-lock", target, "--owner-token", owner.Token, "--operation", owner.OperationID, "--confirm-dead", "--json"}
}

func recoveryManagement(root string, proof store.HolderLiveness) core.ManagementAction {
	return func(ctx context.Context, args []string, stdout, stderr io.Writer) int {
		owner, err := recoverMutationLock(ctx, root, args, proof)
		if err != nil {
			return managementError(args, stdout, stderr, err)
		}
		return managementResult(args, stdout, map[string]any{"ok": true, "recovered": true, "target": args[2], "operation": owner.OperationID, "owner_token": owner.Token})
	}
}

func TestLocalReleaseRequiresPackagedProvenance(t *testing.T) {
	root, executable := packagedDirectory(t)
	rel, err := localReleaseFrom(root, executable, "1.0.0")
	if err != nil || rel.Version != "1.0.0" || rel.Revision != testRevision {
		t.Fatal(rel, err)
	}
	for name, mutate := range map[string]func(string){
		"tampered source": func(root string) { writeTestFile(t, filepath.Join(root, "WORKER-CONTRACT"), "tampered") },
		"missing manifest": func(root string) {
			if err := os.Remove(filepath.Join(root, "RELEASE.json")); err != nil {
				t.Fatal(err)
			}
		},
		"missing checksums": func(root string) {
			if err := os.Remove(filepath.Join(root, "SHA256SUMS")); err != nil {
				t.Fatal(err)
			}
		},
		"extra path": func(root string) { writeTestFile(t, filepath.Join(root, "extra"), "unexpected") },
		"symlink source": func(root string) {
			path := filepath.Join(root, "WORKER-CONTRACT")
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(filepath.Join(root, "VERSION"), path); err != nil {
				t.Fatal(err)
			}
		},
		"wrong revision": func(root string) {
			replaceTestBytes(t, filepath.Join(root, "RELEASE.json"), []byte(testRevision), []byte("not-a-release-revision------------------"))
		},
	} {
		t.Run(name, func(t *testing.T) {
			copyRoot, executable := packagedDirectory(t)
			mutate(copyRoot)
			if _, err := localReleaseFrom(copyRoot, executable, "1.0.0"); err == nil {
				t.Fatal("invalid package accepted")
			}
		})
	}
	if _, err := localReleaseFrom(root, executable, "2.0.0"); err == nil {
		t.Fatal("wrong requested version accepted")
	}
}

func TestLocalReleaseAcceptsExtractedWindowsBundle(t *testing.T) {
	source, bundles := t.TempDir(), t.TempDir()
	for path, body := range map[string]string{"agent-teamctl.exe": "windows-binary", "WORKER-CONTRACT": "contract", "codex/SKILL.md": "codex", "claude/SKILL.md": "claude", "VERSION": "1.0.0\n"} {
		writeTestFile(t, filepath.Join(source, filepath.FromSlash(path)), body)
	}
	if err := releasepkg.BuildWindowsBundleFrom(source, bundles, "1.0.0", testRevision); err != nil {
		t.Fatal(err)
	}
	archive, err := zip.OpenReader(filepath.Join(bundles, "agent-teamctl-1.0.0-windows-amd64.zip"))
	if err != nil {
		t.Fatal(err)
	}
	distribution := t.TempDir()
	for _, member := range archive.File {
		reader, openErr := member.Open()
		if openErr != nil {
			t.Fatal(openErr)
		}
		body, readErr := io.ReadAll(reader)
		_ = reader.Close()
		if readErr != nil {
			t.Fatal(readErr)
		}
		writeTestFile(t, filepath.Join(distribution, filepath.FromSlash(member.Name)), string(body))
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	rel, err := localReleaseFrom(distribution, filepath.Join(distribution, "agent-teamctl.exe"), "1.0.0")
	if err != nil || rel.Binary.Path != filepath.Join(distribution, "agent-teamctl.exe") {
		t.Fatal(rel, err)
	}
}

func packagedDirectory(t *testing.T) (string, string) {
	t.Helper()
	source := t.TempDir()
	writeTestFile(t, filepath.Join(source, "agent-teamctl"), "binary")
	writeTestFile(t, filepath.Join(source, "WORKER-CONTRACT"), "contract")
	writeTestFile(t, filepath.Join(source, "codex", "SKILL.md"), "codex")
	writeTestFile(t, filepath.Join(source, "claude", "SKILL.md"), "claude")
	writeTestFile(t, filepath.Join(source, "VERSION"), "1.0.0\n")
	output := t.TempDir()
	if err := releasepkg.BuildReleasePackageFrom(source, output, "1.0.0", testRevision); err != nil {
		t.Fatal(err)
	}
	archive, err := zip.OpenReader(filepath.Join(output, "agent-teamctl-1.0.0.zip"))
	if err != nil {
		t.Fatal(err)
	}
	defer archive.Close()
	for _, member := range archive.File {
		reader, err := member.Open()
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(reader)
		_ = reader.Close()
		if err != nil {
			t.Fatal(err)
		}
		writeTestFile(t, filepath.Join(output, filepath.FromSlash(member.Name)), string(body))
	}
	return output, filepath.Join(output, "agent-teamctl")
}

func writeTestFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
}

func replaceTestBytes(t *testing.T, path string, old, replacement []byte) {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for index := 0; index+len(old) <= len(body); index++ {
		match := true
		for offset := range old {
			if body[index+offset] != old[offset] {
				match = false
				break
			}
		}
		if match {
			body = append(append(append([]byte(nil), body[:index]...), replacement...), body[index+len(old):]...)
			if err := os.WriteFile(path, body, 0o644); err != nil {
				t.Fatal(err)
			}
			return
		}
	}
	t.Fatal("test bytes not found")
}

func TestSetupCLIApproveKickoffBootstrapsFreshRepo(t *testing.T) {
	root := testkit.GitRepo(t)
	t.Chdir(root)
	var out bytes.Buffer
	if code := cli.Run(context.Background(), []string{"setup", "--approve-kickoff", "--json"}, core.Dependencies{Stdout: &out, Stderr: &out, Management: runManagement}); code != 0 {
		t.Fatalf("code=%d output=%q", code, out.String())
	}
	for _, name := range []string{"TASKS.md", "DECISIONS.md", "AGENT_TEAM_RULES.md", filepath.Join(".agent-team", "config.json")} {
		if _, err := os.Stat(filepath.Join(root, name)); err != nil {
			t.Fatalf("approved setup did not create %s: %v; output=%q", name, err, out.String())
		}
	}
	var result struct {
		OK      bool `json:"ok"`
		Tracker struct {
			Kind string `json:"kind"`
		} `json:"tracker"`
		Generated []string `json:"generated"`
	}
	if err := json.Unmarshal(out.Bytes(), &result); err != nil || !result.OK || result.Tracker.Kind != "tasks-md" || len(result.Generated) != 3 {
		t.Fatalf("unexpected setup result: %q err=%v", out.String(), err)
	}
	if _, err := project.NewSettingsService(store.New(root, core.DefaultConfig().Storage)).Inspect(context.Background()); err != nil {
		t.Fatalf("settings binding after fresh setup: %v", err)
	}
}

func TestSetupCLIPrefersBeadsOverLegacyTasksMd(t *testing.T) {
	root := testkit.GitRepo(t)
	if err := os.Mkdir(filepath.Join(root, ".beads"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "TASKS.md"), []byte("# legacy\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(root)
	var out bytes.Buffer
	if code := cli.Run(context.Background(), []string{"setup", "--approve-kickoff", "--json"}, core.Dependencies{Stdout: &out, Stderr: &out, Management: runManagement}); code != 0 {
		t.Fatalf("code=%d output=%q", code, out.String())
	}
	var result struct {
		Tracker struct {
			Kind string `json:"kind"`
		} `json:"tracker"`
		Generated []string `json:"generated"`
	}
	if err := json.Unmarshal(out.Bytes(), &result); err != nil || result.Tracker.Kind != "beads" || len(result.Generated) != 2 {
		t.Fatalf("beads must win over legacy TASKS.md: %q err=%v", out.String(), err)
	}
	if raw, _ := os.ReadFile(filepath.Join(root, "TASKS.md")); string(raw) != "# legacy\n" {
		t.Fatalf("legacy TASKS.md was rewritten: %q", raw)
	}
}

func TestSetupCLIWithoutApprovalNamesMissingInputsAndWritesNothing(t *testing.T) {
	root := testkit.GitRepo(t)
	before := testkit.SnapshotTree(t, root)
	t.Chdir(root)
	var out bytes.Buffer
	if code := cli.Run(context.Background(), []string{"setup", "--json"}, core.Dependencies{Stdout: &out, Stderr: &out, Management: runManagement}); code == 0 {
		t.Fatalf("unapproved setup on an empty repo must fail: %q", out.String())
	}
	for _, want := range []string{"DECISIONS.md", "AGENT_TEAM_RULES.md", "--approve-kickoff"} {
		if !bytes.Contains(out.Bytes(), []byte(want)) {
			t.Fatalf("error must name %s: %q", want, out.String())
		}
	}
	if after := testkit.SnapshotTree(t, root); !reflect.DeepEqual(before, after) {
		t.Fatalf("refused setup wrote files: before=%v after=%v", before, after)
	}
}

func TestSetupCLIApprovedButInvalidTrackerWritesNothing(t *testing.T) {
	root := testkit.GitRepo(t)
	if err := os.Mkdir(filepath.Join(root, ".beads"), 0o755); err != nil {
		t.Fatal(err)
	}
	// A symlink inside an existing input is rejected by digestDirectory.
	if err := os.Mkdir(filepath.Join(root, "DECISIONS.md"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "README.md"), filepath.Join(root, "DECISIONS.md", "link")); err != nil {
		t.Fatal(err)
	}
	before := testkit.SnapshotTree(t, root)
	t.Chdir(root)
	var out bytes.Buffer
	if code := cli.Run(context.Background(), []string{"setup", "--approve-kickoff", "--json"}, core.Dependencies{Stdout: &out, Stderr: &out, Management: runManagement}); code == 0 {
		t.Fatalf("symlinked input must be rejected: %q", out.String())
	}
	if after := testkit.SnapshotTree(t, root); !reflect.DeepEqual(before, after) {
		t.Fatalf("rejected approved setup wrote files: before=%v after=%v", before, after)
	}
}
