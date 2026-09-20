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
	"github.com/thebpandey/agent-team/vnext/internal/core"
	"github.com/thebpandey/agent-team/vnext/internal/install"
	"github.com/thebpandey/agent-team/vnext/internal/lifecycle"
	"github.com/thebpandey/agent-team/vnext/internal/migrate"
	releasepkg "github.com/thebpandey/agent-team/vnext/internal/release"
	"github.com/thebpandey/agent-team/vnext/internal/store"
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
			layout, layoutErr := install.ResolveLayout(runtime.GOOS, map[string]string{"LOCALAPPDATA": os.Getenv("LOCALAPPDATA"), "XDG_DATA_HOME": os.Getenv("XDG_DATA_HOME"), "CODEX_HOME": os.Getenv("CODEX_HOME"), "CLAUDE_HOME": os.Getenv("CLAUDE_HOME")})
			if layoutErr != nil {
				return managementError(args, stdout, stderr, layoutErr)
			}
			var request install.LegacyHostCutoverRequest
			if err := readStrictJSON(args[2], &request); err != nil {
				return managementError(args, stdout, stderr, err)
			}
			if request.AuthorityReceipt != "" {
				request.LegacyReceiptSHA256 = request.AuthorityReceiptSHA256
				if request.Action == "host-cutover" {
					inventories, verifyErr := migrate.VerifiedHostInventories(request.AuthorityReceipt, request.AuthorityReceiptSHA256)
					if verifyErr != nil {
						return managementError(args, stdout, stderr, verifyErr)
					}
					request.Inventories = inventories
				}
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
		}
		return managementResult(args, stdout, output)
	}
	layout, err := install.ResolveLayout(runtime.GOOS, map[string]string{
		"LOCALAPPDATA":  os.Getenv("LOCALAPPDATA"),
		"XDG_DATA_HOME": os.Getenv("XDG_DATA_HOME"),
		"CODEX_HOME":    os.Getenv("CODEX_HOME"),
		"CLAUDE_HOME":   os.Getenv("CLAUDE_HOME"),
	})
	if err != nil {
		return managementError(args, stdout, stderr, err)
	}
	action := args[0]
	manifest, readErr := install.NewManifestStore(layout).Read(ctx)
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
	return managementResult(args, stdout, map[string]any{"ok": true, "action": action, "revision": outcome.Manifest.Revision, "retained": outcome.Retained})
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
	_, err := fmt.Fprintf(stdout, "%s accepted\n", args[0])
	if err != nil {
		return 1
	}
	if ok, _ := value["ok"].(bool); !ok {
		return 1
	}
	return 0
}
