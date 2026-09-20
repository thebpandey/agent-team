package install

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/thebpandey/agent-team/vnext/internal/core"
)

type legacySourceMetadata struct {
	Version        string                         `json:"version"`
	SourceRevision string                         `json:"sourceRevision"`
	PackageFileMap map[string]legacyInstalledFile `json:"packageFileMap"`
}

// InventoryLegacyHosts records, but does not modify, the exact legacy and
// staged-native bytes an operator is being asked to authorize.
func InventoryLegacyHosts(layout Layout, hosts []Host) ([]LegacyHostInventory, error) {
	if ValidateLayout(layout) != nil {
		return nil, core.ErrSettings
	}
	hosts, err := normalizeHosts(hosts)
	if err != nil || len(hosts) == 0 {
		return nil, core.ErrSettings
	}
	manifestIdentity, manifestRaw, err := stableLegacyIdentity(layout.ManifestPath, installJournalLimit)
	if err != nil {
		return nil, err
	}
	var manifest InstallManifest
	if json.Unmarshal(manifestRaw, &manifest) != nil || manifest.Schema != 1 || manifest.Revision == 0 || manifest.ReleaseRevision == "" || verifyManifestFiles(manifest, nil) != nil {
		return nil, core.ErrRevision
	}
	result := make([]LegacyHostInventory, 0, len(hosts))
	for _, host := range hosts {
		root := layout.SkillRoots[host]
		sourceIdentity, sourceRaw, readErr := stableLegacyIdentity(filepath.Join(root, ".agent-team-source.json"), legacyReceiptLimit)
		if readErr != nil {
			return nil, readErr
		}
		var source legacySourceMetadata
		if json.Unmarshal(sourceRaw, &source) != nil || source.Version == "" || !validReleaseRevision(source.SourceRevision) {
			return nil, core.ErrRevision
		}
		skillIdentity, _, readErr := stableLegacyIdentity(filepath.Join(root, "SKILL.md"), legacyReceiptLimit)
		owned, ok := source.PackageFileMap["SKILL.md"]
		if readErr != nil || !ok || owned.SHA256 != skillIdentity.SHA256 || owned.Mode != skillIdentity.Mode || owned.Size != skillIdentity.Size {
			return nil, fmt.Errorf("%w: legacy top-level SKILL.md differs from source metadata", core.ErrRevision)
		}
		inventory := LegacyHostInventory{Host: host, Root: root, Source: sourceIdentity, Skill: skillIdentity, NativeManifest: manifestIdentity, NativeRevision: manifest.Revision, NativeRelease: manifest.ReleaseRevision, NativeFiles: append([]OwnedFile(nil), manifest.Files...), Handlers: []LegacyHandlerInventory{}}
		configPath := layout.ConfigPaths[host]
		configIdentity, configRaw, configErr := stableLegacyIdentity(configPath, legacyReceiptLimit)
		if configErr == nil {
			inventory.Config = &configIdentity
			inventory.Handlers, err = inventoryLegacyHandlers(configRaw, configPath, host)
			if err != nil {
				return nil, err
			}
		} else if !errors.Is(configErr, fs.ErrNotExist) {
			return nil, configErr
		}
		result = append(result, inventory)
	}
	return result, nil
}

func stableLegacyIdentity(path string, limit int64) (LegacyFileIdentity, []byte, error) {
	if !absoluteClean(path) {
		return LegacyFileIdentity{}, nil, core.ErrPath
	}
	info, err := os.Lstat(path)
	if err != nil {
		return LegacyFileIdentity{}, nil, err
	}
	if !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > limit {
		return LegacyFileIdentity{}, nil, core.ErrPath
	}
	raw, err := readStableRegular(filepath.Dir(path), path, info.Size(), nil, "")
	if err != nil {
		return LegacyFileIdentity{}, nil, err
	}
	return LegacyFileIdentity{Path: path, SHA256: digestContent(raw), Mode: uint32(info.Mode().Perm()), Size: info.Size()}, raw, nil
}

func inventoryLegacyHandlers(raw []byte, path string, host Host) ([]LegacyHandlerInventory, error) {
	root, err := parseJSONSpans(raw)
	if err != nil {
		return nil, core.ErrRevision
	}
	hooks := root.member("hooks")
	if hooks == nil {
		return []LegacyHandlerInventory{}, nil
	}
	if hooks.kind != '{' {
		return nil, core.ErrRevision
	}
	needle := "/skills/agent-team/hooks/agent-team-hook.mjs"
	var result []LegacyHandlerInventory
	for event, groups := range hooks.members {
		if groups.kind != '[' {
			continue
		}
		for _, group := range groups.items {
			handlers := group.member("hooks")
			if handlers == nil || handlers.kind != '[' {
				continue
			}
			for _, candidate := range handlers.items {
				candidateRaw := raw[candidate.start:candidate.end]
				var decoded struct {
					Command string `json:"command"`
				}
				if json.Unmarshal(candidateRaw, &decoded) != nil || !strings.Contains(filepath.ToSlash(decoded.Command), needle) || !strings.Contains(decoded.Command, "--runtime "+string(host)) {
					continue
				}
				compact := new(bytes.Buffer)
				if json.Compact(compact, candidateRaw) != nil {
					return nil, core.ErrRevision
				}
				digest := digestContent(compact.Bytes())
				result = append(result, LegacyHandlerInventory{Runtime: string(host), Event: event, HandlerID: event + ":" + digest[:16], Digest: digest, ConfigPath: path, Handler: append(json.RawMessage(nil), candidateRaw...)})
			}
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].HandlerID < result[j].HandlerID })
	for i := 1; i < len(result); i++ {
		if result[i-1].HandlerID == result[i].HandlerID {
			return nil, fmt.Errorf("%w: legacy handler is ambiguous", core.ErrRevision)
		}
	}
	return result, nil
}
