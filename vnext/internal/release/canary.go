package release

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/thebpandey/agent-team/vnext/internal/core"
	"github.com/thebpandey/agent-team/vnext/internal/install"
)

type RestoredFile struct {
	Path, SHA256 string
}

type Canary struct {
	Hosts                                     []install.Host
	Layout                                    install.Layout
	Release                                   install.Release
	Manifest                                  install.InstallManifest
	ArchivePath, ArchiveSHA256                string
	BinarySHA256, SkillSHA256, ContractSHA256 string
	HostResults                               map[install.Host]HostCanary
	Actions                                   []string
	RollbackVerified                          bool
	Preserved                                 []string
	UnrelatedBefore, UnrelatedAfter           []byte
	RestoredFiles                             []RestoredFile
	RestoredManifestSHA256                    string
	StagingEvidenceDigest                     string
	StagingCleaned                            bool
}

type HostCanary struct {
	Host                                      install.Host
	Actions                                   []string
	BinarySHA256, SkillSHA256, ContractSHA256 string
	RollbackVerified, UnrelatedPreserved      bool
}

type Cutover struct {
	Canary                                                 Canary
	ProviderVerified, InstalledVerified, RollbackRehearsed bool
	Evidence                                               []string
}

func RunInstallCanary(ctx context.Context, layout install.Layout, rel install.Release, prior install.InstallManifest, hosts []install.Host) (Canary, error) {
	if ctx == nil || ctx.Err() != nil || install.ValidateLayout(layout) != nil || install.VerifyRelease(rel) != nil {
		return Canary{}, core.ErrSettings
	}
	hosts, err := validateCanaryHosts(hosts)
	if err != nil {
		return Canary{}, err
	}
	unrelatedPath := filepath.Join(layout.DataRoot, "unrelated-host-setting.json")
	before, err := os.ReadFile(unrelatedPath)
	if err != nil {
		return Canary{}, err
	}
	seedManifest := selectManifestHosts(prior, hosts)
	seedManifest.Revision = 0
	seed, err := install.NewManifestStore(layout).CompareAndSwap(ctx, 0, seedManifest)
	if err != nil {
		return Canary{}, err
	}
	updated, err := install.Update(ctx, layout, rel, seed.ObservedRevision)
	if err != nil {
		return Canary{}, err
	}
	binaryDigest, err := installedDigest(layout.BinaryPath)
	if err != nil || binaryDigest != rel.Binary.SHA256 {
		return Canary{}, core.ErrRevision
	}
	contractDigest, err := installedDigest(layout.ContractPath)
	if err != nil || contractDigest != rel.Contract.SHA256 {
		return Canary{}, core.ErrRevision
	}
	result := Canary{Hosts: hosts, Layout: layout, Release: rel, Manifest: updated.Manifest, HostResults: map[install.Host]HostCanary{}, BinarySHA256: binaryDigest, ContractSHA256: contractDigest, UnrelatedBefore: before}
	for _, host := range hosts {
		skill, ok := installedSkill(updated.Manifest, host)
		if !ok {
			return Canary{}, core.ErrRevision
		}
		skillDigest, digestErr := installedDigest(skill.Path)
		if digestErr != nil || skillDigest != skill.SHA256 {
			return Canary{}, core.ErrRevision
		}
		actions, actionErr := requireActions(skill.Path)
		if actionErr != nil {
			return Canary{}, actionErr
		}
		result.Actions = actions
		result.SkillSHA256 = skillDigest
		result.HostResults[host] = HostCanary{Host: host, Actions: append([]string(nil), actions...), BinarySHA256: binaryDigest, SkillSHA256: skillDigest, ContractSHA256: contractDigest}
	}
	rolled, err := install.Rollback(ctx, layout, prior.Version, updated.ObservedRevision)
	if err != nil {
		return Canary{}, err
	}
	after, err := os.ReadFile(unrelatedPath)
	if err != nil || !bytes.Equal(before, after) {
		return Canary{}, core.ErrRevision
	}
	result.UnrelatedAfter = after
	result.Preserved = append(append([]string(nil), updated.Retained...), rolled.Retained...)
	result.Manifest = rolled.Manifest
	result.RestoredFiles, err = restoredFiles(rolled.Manifest)
	if err != nil {
		return Canary{}, err
	}
	result.RestoredManifestSHA256 = ManifestFingerprint(rolled.Manifest)
	result.RollbackVerified = true
	for host, hostResult := range result.HostResults {
		hostResult.RollbackVerified = true
		hostResult.UnrelatedPreserved = true
		result.HostResults[host] = hostResult
	}
	return result, nil
}

func RunInstallCanaryFromArchive(ctx context.Context, archivePath string, layout install.Layout, rel install.Release, prior install.InstallManifest, hosts []install.Host) (Canary, error) {
	raw, err := os.ReadFile(archivePath)
	if err != nil {
		return Canary{}, core.ErrPath
	}
	archive, err := zip.OpenReader(archivePath)
	if err != nil {
		return Canary{}, core.ErrRevision
	}
	defer archive.Close()
	stage, err := os.MkdirTemp("", "agent-team-canary-")
	if err != nil {
		return Canary{}, err
	}
	cleaned := false
	defer func() {
		if !cleaned {
			_ = os.RemoveAll(stage)
		}
	}()
	want := map[string]install.ReleaseFile{"agent-teamctl": rel.Binary, "WORKER-CONTRACT": rel.Contract, "codex/SKILL.md": rel.Entrypoints[install.Codex], "claude/SKILL.md": rel.Entrypoints[install.Claude]}
	seen := map[string]bool{}
	versionSeen := false
	for _, entry := range archive.File {
		member := path.Clean(strings.ReplaceAll(entry.Name, "\\", "/"))
		if member != entry.Name || member == "." || strings.HasPrefix(member, "../") || strings.HasPrefix(member, "/") || seen[member] || (member == "VERSION" && versionSeen) {
			return Canary{}, core.ErrRevision
		}
		if member == "VERSION" {
			body, readErr := readArchiveEntry(entry, 128)
			if readErr != nil || strings.TrimSpace(string(body)) != rel.Version {
				return Canary{}, core.ErrRevision
			}
			versionSeen = true
			continue
		}
		expected, ok := want[member]
		if !ok || entry.UncompressedSize64 != uint64(expected.Bytes) {
			return Canary{}, core.ErrRevision
		}
		body, readErr := readArchiveEntry(entry, expected.Bytes)
		if readErr != nil || int64(len(body)) != expected.Bytes || sha256Hex(body) != expected.SHA256 {
			return Canary{}, core.ErrRevision
		}
		target := filepath.Join(stage, filepath.FromSlash(member))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return Canary{}, err
		}
		if err := os.WriteFile(target, body, 0o600); err != nil {
			return Canary{}, err
		}
		seen[member] = true
	}
	if len(seen) != len(want) || !versionSeen {
		return Canary{}, core.ErrRevision
	}
	staged := install.Release{Version: rel.Version, Binary: rel.Binary, Contract: rel.Contract, Entrypoints: map[install.Host]install.ReleaseFile{}}
	staged.Binary.Path = filepath.Join(stage, "agent-teamctl")
	staged.Contract.Path = filepath.Join(stage, "WORKER-CONTRACT")
	for host, file := range rel.Entrypoints {
		file.Path = filepath.Join(stage, string(host), "SKILL.md")
		staged.Entrypoints[host] = file
	}
	canary, err := RunInstallCanary(ctx, layout, staged, prior, hosts)
	if err != nil {
		return Canary{}, err
	}
	canary.Release = rel
	canary.ArchivePath = archivePath
	canary.ArchiveSHA256 = sha256Hex(raw)
	canary.StagingEvidenceDigest = RestoreEvidenceDigest(canary)
	if err := os.RemoveAll(stage); err != nil {
		return Canary{}, err
	}
	cleaned = true
	canary.StagingCleaned = true
	if err := VerifyRollback(ctx, canary); err != nil {
		return Canary{}, err
	}
	return canary, nil
}

func VerifyRollback(ctx context.Context, canary Canary) error {
	if ctx == nil || ctx.Err() != nil || !canary.RollbackVerified || canary.RestoredManifestSHA256 == "" {
		return core.ErrRevision
	}
	if canary.StagingCleaned && (canary.StagingEvidenceDigest == "" || canary.StagingEvidenceDigest != RestoreEvidenceDigest(canary)) {
		return core.ErrRevision
	}
	raw, err := os.ReadFile(canary.Layout.ManifestPath)
	if err != nil {
		return core.ErrRevision
	}
	var restored install.InstallManifest
	if err := json.Unmarshal(raw, &restored); err != nil || restored.Schema != 1 || restored.Version == "" || canary.RestoredManifestSHA256 != ManifestFingerprint(restored) {
		return core.ErrRevision
	}
	expected := map[string]string{}
	for _, file := range restored.Files {
		expected[file.Path] = file.SHA256
	}
	if len(canary.RestoredFiles) != len(expected) {
		return core.ErrRevision
	}
	seen := map[string]bool{}
	for _, file := range canary.RestoredFiles {
		digest, digestErr := installedDigest(file.Path)
		if digestErr != nil || seen[file.Path] || digest != file.SHA256 || expected[file.Path] != file.SHA256 {
			return core.ErrRevision
		}
		seen[file.Path] = true
	}
	return nil
}

func VerifyCutover(cutover Cutover) error {
	hosts := map[install.Host]bool{}
	for _, host := range cutover.Canary.Hosts {
		hosts[host] = true
	}
	if !hosts[install.Codex] || !hosts[install.Claude] || !cutover.Canary.RollbackVerified || !cutover.ProviderVerified || !cutover.InstalledVerified || !cutover.RollbackRehearsed {
		return core.ErrPhase
	}
	return nil
}

func ManifestFingerprint(manifest install.InstallManifest) string {
	raw, err := json.Marshal(manifest)
	if err != nil {
		return ""
	}
	return sha256Hex(raw)
}

func RestoreEvidenceDigest(canary Canary) string {
	paths := make([]string, 0, len(canary.RestoredFiles))
	for _, file := range canary.RestoredFiles {
		paths = append(paths, file.Path+":"+file.SHA256)
	}
	sort.Strings(paths)
	return sha256Hex([]byte(canary.ArchiveSHA256 + "\n" + canary.RestoredManifestSHA256 + "\n" + strings.Join(paths, "\n")))
}

func validateCanaryHosts(hosts []install.Host) ([]install.Host, error) {
	if len(hosts) == 0 {
		return nil, core.ErrSettings
	}
	seen := map[install.Host]bool{}
	out := append([]install.Host(nil), hosts...)
	for _, host := range out {
		if (host != install.Codex && host != install.Claude) || seen[host] {
			return nil, core.ErrSettings
		}
		seen[host] = true
	}
	return out, nil
}

func selectManifestHosts(manifest install.InstallManifest, hosts []install.Host) install.InstallManifest {
	wanted := map[install.Host]bool{}
	for _, host := range hosts {
		wanted[host] = true
	}
	manifest.Hosts = append([]install.Host(nil), hosts...)
	files := manifest.Files[:0:0]
	for _, file := range manifest.Files {
		if file.Role != install.EntrypointRole || wanted[file.Host] {
			files = append(files, file)
		}
	}
	manifest.Files = files
	manifest.Backups = nil
	return manifest
}

func installedSkill(manifest install.InstallManifest, host install.Host) (install.OwnedFile, bool) {
	for _, file := range manifest.Files {
		if file.Role == install.EntrypointRole && file.Host == host {
			return file, true
		}
	}
	return install.OwnedFile{}, false
}

func installedDigest(path string) (string, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return sha256Hex(body), nil
}

func requireActions(path string) ([]string, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	actions := []string{"setup", "status", "start"}
	for _, action := range actions {
		if !strings.Contains(string(body), action) {
			return nil, core.ErrPhase
		}
	}
	return actions, nil
}

func restoredFiles(manifest install.InstallManifest) ([]RestoredFile, error) {
	files := make([]RestoredFile, 0, len(manifest.Files))
	for _, file := range manifest.Files {
		digest, err := installedDigest(file.Path)
		if err != nil || digest != file.SHA256 {
			return nil, core.ErrRevision
		}
		files = append(files, RestoredFile{Path: file.Path, SHA256: digest})
	}
	return files, nil
}

func readArchiveEntry(entry *zip.File, limit int64) ([]byte, error) {
	reader, err := entry.Open()
	if err != nil {
		return nil, err
	}
	body, readErr := io.ReadAll(io.LimitReader(reader, limit+1))
	closeErr := reader.Close()
	if readErr != nil || closeErr != nil || int64(len(body)) > limit {
		return nil, core.ErrRevision
	}
	return body, nil
}
