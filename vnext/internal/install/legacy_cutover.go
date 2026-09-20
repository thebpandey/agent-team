package install

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/thebpandey/agent-team/vnext/internal/core"
	"github.com/thebpandey/agent-team/vnext/internal/store"
)

const (
	legacyReceiptLimit    = 4 << 20
	hostCutoverReceiptRel = ".agent-team/host-cutover.json"
)

type LegacyHostCutoverRequest struct {
	Schema                   int    `json:"schema"`
	Action                   string `json:"action"`
	OperationID              string `json:"operationId"`
	LegacyReceipt            string `json:"legacyReceipt"`
	LegacyReceiptSHA256      string `json:"legacyReceiptSha256"`
	ExpectedManifestRevision uint64 `json:"expectedManifestRevision"`
	ExpectedReceiptDigest    string `json:"expectedReceiptDigest,omitempty"`
	AuthorityReceipt         string `json:"authorityReceipt,omitempty"`
	AuthorityReceiptSHA256   string `json:"authorityReceiptSha256,omitempty"`
	Hosts                    []Host `json:"hosts"`
	// Inventories is populated only after the CLI verifies the signed project
	// authority receipt. It is deliberately not accepted from request JSON.
	Inventories []LegacyHostInventory `json:"-"`
}

type LegacyFileIdentity struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Mode   uint32 `json:"mode"`
	Size   int64  `json:"size"`
}

type LegacyHandlerInventory struct {
	Runtime    string          `json:"runtime"`
	Event      string          `json:"event"`
	HandlerID  string          `json:"handlerId"`
	Digest     string          `json:"digest"`
	ConfigPath string          `json:"configPath"`
	Handler    json.RawMessage `json:"handler"`
}

type LegacyHostInventory struct {
	Host           Host                     `json:"host"`
	Root           string                   `json:"root"`
	Source         LegacyFileIdentity       `json:"source"`
	Skill          LegacyFileIdentity       `json:"skill"`
	Config         *LegacyFileIdentity      `json:"config,omitempty"`
	Handlers       []LegacyHandlerInventory `json:"handlers"`
	NativeManifest LegacyFileIdentity       `json:"nativeManifest"`
	NativeRevision uint64                   `json:"nativeRevision"`
	NativeRelease  string                   `json:"nativeReleaseRevision"`
	NativeFiles    []OwnedFile              `json:"nativeFiles"`
}

type LegacyHostCutoverResult struct {
	ReceiptDigest    string `json:"receiptDigest,omitempty"`
	ManifestRevision uint64 `json:"manifestRevision"`
	Idempotent       bool   `json:"idempotent,omitempty"`
}

type legacyInstalledFile struct {
	SHA256 string `json:"sha256"`
	Mode   uint32 `json:"mode"`
	Size   int64  `json:"size"`
}

type legacyInstalledMap struct {
	Target string                         `json:"target"`
	Digest string                         `json:"digest"`
	Files  map[string]legacyInstalledFile `json:"files"`
}

type legacyHandler struct {
	Runtime     string          `json:"runtime"`
	Event       string          `json:"event"`
	HandlerID   string          `json:"handlerId"`
	Digest      string          `json:"digest"`
	ConfigPath  string          `json:"configPath"`
	Handler     json.RawMessage `json:"handler"`
	Preexisting bool            `json:"preexisting"`
}

type legacyInstallReceipt struct {
	SchemaVersion     int                           `json:"schemaVersion"`
	Version           string                        `json:"version"`
	InstalledFileMaps map[string]legacyInstalledMap `json:"installedFileMaps"`
	Handlers          []legacyHandler               `json:"handlers"`
	HandlerConflicts  []any                         `json:"handlerConflicts"`
	ResourceConflicts []any                         `json:"resourceConflicts"`
}

type hostCutoverReceipt struct {
	Schema              int                    `json:"schema"`
	LegacySchema        int                    `json:"legacySchema"`
	OperationID         string                 `json:"operationId"`
	Version             string                 `json:"version"`
	Revision            string                 `json:"revision"`
	LegacyReceipt       string                 `json:"legacyReceipt"`
	LegacyReceiptSHA256 string                 `json:"legacyReceiptSha256"`
	Previous            InstallManifest        `json:"previous"`
	Preimages           []hostCutoverPreimage  `json:"preimages"`
	Postimages          []hostCutoverPostimage `json:"postimages"`
	Retained            []hostCutoverPostimage `json:"retained"`
	WrittenAt           string                 `json:"writtenAt"`
	ReceiptDigest       string                 `json:"receiptDigest"`
}

type hostCutoverPreimage struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Mode   uint32 `json:"mode"`
	Bytes  []byte `json:"bytes"`
}

type hostCutoverPostimage struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256,omitempty"`
	Absent bool   `json:"absent,omitempty"`
}

func CutoverLegacyHosts(ctx context.Context, layout Layout, release Release, request LegacyHostCutoverRequest) (LegacyHostCutoverResult, error) {
	if ctx == nil || ctx.Err() != nil || ValidateLayout(layout) != nil || request.Schema != 1 || request.OperationID == "" || (request.Action != "host-cutover" && request.Action != "host-rollback" && request.Action != "host-status") {
		return LegacyHostCutoverResult{}, core.ErrSettings
	}
	guard, err := store.AcquireProjectMutation(ctx, layout.DataRoot, "host-cutover", request.OperationID+":"+request.Action)
	if err != nil {
		return LegacyHostCutoverResult{}, err
	}
	defer func() { _ = guard.Release() }()
	manifestStore := NewManifestStore(layout)
	if err := recoverLifecycleJournal(ctx, layout, manifestStore); err != nil {
		return LegacyHostCutoverResult{}, err
	}
	if request.Action == "host-status" {
		receipt, err := readHostCutoverReceipt(layout)
		if err != nil {
			return LegacyHostCutoverResult{}, err
		}
		manifest, err := manifestStore.Read(ctx)
		if err != nil || verifyHostCutoverState(receipt) != nil || verifyManifestFiles(manifest, nil) != nil {
			return LegacyHostCutoverResult{}, core.ErrRevision
		}
		return LegacyHostCutoverResult{ReceiptDigest: receipt.ReceiptDigest, ManifestRevision: manifest.Revision}, nil
	}
	if request.Action == "host-rollback" {
		return rollbackLegacyHosts(ctx, layout, request, guard.Owner())
	}
	return cutoverLegacyHosts(ctx, layout, release, request, guard.Owner())
}

func cutoverLegacyHosts(ctx context.Context, layout Layout, release Release, request LegacyHostCutoverRequest, owner store.MutationOwner) (LegacyHostCutoverResult, error) {
	if existing, err := readHostCutoverReceipt(layout); err == nil {
		if existing.Revision == release.Revision && existing.LegacyReceiptSHA256 == request.LegacyReceiptSHA256 {
			manifest, readErr := NewManifestStore(layout).Read(ctx)
			if readErr != nil || verifyManifestFiles(manifest, nil) != nil || verifyHostCutoverState(existing) != nil {
				return LegacyHostCutoverResult{}, core.ErrRevision
			}
			return LegacyHostCutoverResult{ReceiptDigest: existing.ReceiptDigest, ManifestRevision: manifest.Revision, Idempotent: true}, nil
		}
		return LegacyHostCutoverResult{}, core.ErrRevision
	} else if !errors.Is(err, fs.ErrNotExist) {
		return LegacyHostCutoverResult{}, err
	}
	if VerifyRelease(release) != nil || !validSHA256(request.LegacyReceiptSHA256) {
		return LegacyHostCutoverResult{}, core.ErrRevision
	}
	hosts, err := normalizeHosts(request.Hosts)
	if err != nil || len(hosts) == 0 || len(layout.ConfigPaths) != 2 {
		return LegacyHostCutoverResult{}, core.ErrSettings
	}
	legacy, signedInventory, err := legacyAuthority(layout, request, hosts)
	if err != nil {
		return LegacyHostCutoverResult{}, err
	}
	current, err := NewManifestStore(layout).Read(ctx)
	if err != nil || current.Revision != request.ExpectedManifestRevision || current.ReleaseRevision != release.Revision {
		return LegacyHostCutoverResult{}, core.ErrRevision
	}
	previous, intended := cloneManifest(current), cloneManifest(current)
	intended.Revision = current.Revision + 1
	journal := lifecycleJournal{Schema: 1, Operation: "update", ExpectedRevision: current.Revision, Owner: owner, Previous: &previous, Intended: intended}
	receipt := hostCutoverReceipt{Schema: 1, LegacySchema: legacy.SchemaVersion, OperationID: request.OperationID, Version: release.Version, Revision: release.Revision, LegacyReceipt: request.LegacyReceipt, LegacyReceiptSHA256: request.LegacyReceiptSHA256, Previous: previous, WrittenAt: time.Now().UTC().Format(time.RFC3339Nano)}
	for _, host := range hosts {
		inventory := inventoryForHost(request.Inventories, host)
		if signedInventory {
			sourceIdentity, _, sourceErr := stableLegacyIdentity(inventory.Source.Path, legacyReceiptLimit)
			if sourceErr != nil || sourceIdentity != inventory.Source {
				return LegacyHostCutoverResult{}, fmt.Errorf("%w: signed host provenance changed", core.ErrRevision)
			}
		}
		for relative, file := range legacy.InstalledFileMaps[string(host)].Files {
			path := filepath.Join(layout.SkillRoots[host], filepath.FromSlash(relative))
			if relative != "SKILL.md" && path != filepath.Join(layout.SkillRoots[host], "agent-team-vnext", "SKILL.md") {
				receipt.Retained = append(receipt.Retained, hostCutoverPostimage{Path: path, SHA256: file.SHA256})
			}
		}
		top := filepath.Join(layout.SkillRoots[host], "SKILL.md")
		nested := filepath.Join(layout.SkillRoots[host], "agent-team-vnext", "SKILL.md")
		source := release.Entrypoints[host]
		replacement, err := readStableRegular(filepath.Dir(source.Path), source.Path, source.Bytes, nil, "")
		if err != nil || digestContent(replacement) != source.SHA256 {
			return LegacyHostCutoverResult{}, core.ErrRevision
		}
		mutation, err := prepareMutation(layout, top, replacement, 0o600, false, false, nil)
		if err != nil {
			return LegacyHostCutoverResult{}, err
		}
		if signedInventory && !mutationMatchesLegacyIdentity(mutation, inventory.Skill) {
			return LegacyHostCutoverResult{}, fmt.Errorf("%w: signed top-level skill changed", core.ErrRevision)
		}
		journal.Mutations = append(journal.Mutations, mutation)
		receipt.addMutation(mutation)
		mutation, err = prepareMutation(layout, nested, nil, 0, true, false, nil)
		if err != nil {
			return LegacyHostCutoverResult{}, err
		}
		if signedInventory {
			index := ownedIndex(current.Files, EntrypointRole, host)
			if index < 0 || !mutation.Existed || mutation.PreSHA256 != current.Files[index].SHA256 || int64(len(mutation.Preimage)) != current.Files[index].Bytes {
				return LegacyHostCutoverResult{}, fmt.Errorf("%w: staged native entrypoint changed", core.ErrRevision)
			}
		}
		journal.Mutations = append(journal.Mutations, mutation)
		receipt.addMutation(mutation)
		index := ownedIndex(journal.Intended.Files, EntrypointRole, host)
		if index < 0 || journal.Intended.Files[index].Path != nested {
			return LegacyHostCutoverResult{}, core.ErrRevision
		}
		journal.Intended.Files[index].Path = top
		if !signedInventory || inventory.Config != nil && len(inventory.Handlers) > 0 {
			mutation, err = prepareMutation(layout, layout.ConfigPaths[host], nil, 0o600, false, false, nil)
			if err != nil {
				return LegacyHostCutoverResult{}, err
			}
			if signedInventory && !mutationMatchesLegacyIdentity(mutation, *inventory.Config) {
				return LegacyHostCutoverResult{}, fmt.Errorf("%w: signed host config changed", core.ErrRevision)
			}
			configReplacement, err := retireLegacyHandlers(mutation.Preimage, layout.ConfigPaths[host], string(host), legacy.Handlers)
			if err != nil {
				return LegacyHostCutoverResult{}, err
			}
			mutation.Replacement = configReplacement
			mutation.PostSHA256 = digestContent(configReplacement)
			mutation.PostBytes = int64(len(configReplacement))
			journal.Mutations = append(journal.Mutations, mutation)
			receipt.addMutation(mutation)
		}
	}
	sort.Slice(receipt.Retained, func(i, j int) bool { return receipt.Retained[i].Path < receipt.Retained[j].Path })
	receipt.ReceiptDigest = digestHostReceipt(receipt)
	receiptRaw, err := json.MarshalIndent(receipt, "", "  ")
	if err != nil {
		return LegacyHostCutoverResult{}, err
	}
	receiptRaw = append(receiptRaw, '\n')
	receiptPath := filepath.Join(layout.DataRoot, filepath.FromSlash(hostCutoverReceiptRel))
	mutation, err := prepareMutation(layout, receiptPath, receiptRaw, 0o600, false, true, nil)
	if err != nil {
		return LegacyHostCutoverResult{}, err
	}
	journal.Mutations = append(journal.Mutations, mutation)
	if raw, marshalErr := json.Marshal(journal); marshalErr != nil || len(raw) > installJournalLimit {
		return LegacyHostCutoverResult{}, errJournalBudget
	}
	outcome, err := executeLifecycleJournal(ctx, layout, NewManifestStore(layout), &journal)
	if err != nil {
		return LegacyHostCutoverResult{}, err
	}
	return LegacyHostCutoverResult{ReceiptDigest: receipt.ReceiptDigest, ManifestRevision: outcome.Manifest.Revision}, nil
}

func rollbackLegacyHosts(ctx context.Context, layout Layout, request LegacyHostCutoverRequest, owner store.MutationOwner) (LegacyHostCutoverResult, error) {
	receipt, err := readHostCutoverReceipt(layout)
	if err != nil || request.ExpectedReceiptDigest == "" || request.ExpectedReceiptDigest != receipt.ReceiptDigest || request.LegacyReceiptSHA256 != receipt.LegacyReceiptSHA256 {
		return LegacyHostCutoverResult{}, core.ErrRevision
	}
	current, err := NewManifestStore(layout).Read(ctx)
	if err != nil || current.Revision != request.ExpectedManifestRevision {
		return LegacyHostCutoverResult{}, core.ErrRevision
	}
	for _, post := range receipt.Postimages {
		if post.Absent {
			if _, err := os.Lstat(post.Path); !errors.Is(err, fs.ErrNotExist) {
				return LegacyHostCutoverResult{}, core.ErrRevision
			}
		} else if digestPathBounded(post.Path, installJournalLimit) != post.SHA256 {
			return LegacyHostCutoverResult{}, core.ErrRevision
		}
	}
	previous := cloneManifest(current)
	intended := cloneManifest(receipt.Previous)
	intended.Revision = current.Revision + 1
	journal := lifecycleJournal{Schema: 1, Operation: "rollback", ExpectedRevision: current.Revision, Owner: owner, Previous: &previous, Intended: intended}
	for _, preimage := range receipt.Preimages {
		mutation, err := prepareMutation(layout, preimage.Path, preimage.Bytes, fs.FileMode(preimage.Mode), false, false, nil)
		if err != nil {
			return LegacyHostCutoverResult{}, err
		}
		journal.Mutations = append(journal.Mutations, mutation)
	}
	receiptPath := filepath.Join(layout.DataRoot, filepath.FromSlash(hostCutoverReceiptRel))
	mutation, err := prepareMutation(layout, receiptPath, nil, 0, true, false, nil)
	if err != nil {
		return LegacyHostCutoverResult{}, err
	}
	journal.Mutations = append(journal.Mutations, mutation)
	if raw, marshalErr := json.Marshal(journal); marshalErr != nil || len(raw) > installJournalLimit {
		return LegacyHostCutoverResult{}, errJournalBudget
	}
	outcome, err := executeLifecycleJournal(ctx, layout, NewManifestStore(layout), &journal)
	if err != nil {
		return LegacyHostCutoverResult{}, err
	}
	return LegacyHostCutoverResult{ManifestRevision: outcome.Manifest.Revision}, nil
}

func (receipt *hostCutoverReceipt) addMutation(mutation lifecycleMutation) {
	if mutation.Existed {
		receipt.Preimages = append(receipt.Preimages, hostCutoverPreimage{Path: mutation.Path, SHA256: mutation.PreSHA256, Mode: mutation.PreMode, Bytes: append([]byte(nil), mutation.Preimage...)})
	}
	receipt.Postimages = append(receipt.Postimages, hostCutoverPostimage{Path: mutation.Path, SHA256: mutation.PostSHA256, Absent: mutation.PostAbsent})
}

func readLegacyInstallReceipt(path, expectedDigest string) (legacyInstallReceipt, error) {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > legacyReceiptLimit {
		return legacyInstallReceipt{}, core.ErrPath
	}
	raw, err := readStableRegular(filepath.Dir(path), path, info.Size(), nil, "")
	if err != nil || digestContent(raw) != expectedDigest {
		return legacyInstallReceipt{}, core.ErrPath
	}
	var receipt legacyInstallReceipt
	// A v7 receipt has additional provenance fields. Decode normally, then
	// validate only the authority-bearing fields used by this transaction.
	if json.Unmarshal(raw, &receipt) != nil || receipt.SchemaVersion != 4 || receipt.Version == "" || len(receipt.InstalledFileMaps) == 0 || len(receipt.HandlerConflicts) != 0 || len(receipt.ResourceConflicts) != 0 {
		return legacyInstallReceipt{}, core.ErrRevision
	}
	return receipt, nil
}

func legacyAuthority(layout Layout, request LegacyHostCutoverRequest, hosts []Host) (legacyInstallReceipt, bool, error) {
	if len(request.Inventories) == 0 {
		legacy, err := readLegacyInstallReceipt(request.LegacyReceipt, request.LegacyReceiptSHA256)
		if err != nil {
			return legacyInstallReceipt{}, false, err
		}
		if err := verifyLegacyOwnership(layout, legacy, hosts); err != nil {
			return legacyInstallReceipt{}, false, err
		}
		return legacy, false, nil
	}
	observed, err := InventoryLegacyHosts(layout, hosts)
	if err != nil || !sameLegacyInventories(observed, request.Inventories) {
		return legacyInstallReceipt{}, true, fmt.Errorf("%w: signed legacy inventory changed", core.ErrRevision)
	}
	legacy := legacyInstallReceipt{SchemaVersion: 4, Version: "signed-inventory", InstalledFileMaps: map[string]legacyInstalledMap{}}
	for _, inventory := range request.Inventories {
		files := map[string]legacyInstalledFile{"SKILL.md": {SHA256: inventory.Skill.SHA256, Mode: inventory.Skill.Mode, Size: inventory.Skill.Size}}
		legacy.InstalledFileMaps[string(inventory.Host)] = legacyInstalledMap{Target: inventory.Root, Digest: legacyFileMapDigest(files), Files: files}
		for _, handler := range inventory.Handlers {
			legacy.Handlers = append(legacy.Handlers, legacyHandler{Runtime: handler.Runtime, Event: handler.Event, HandlerID: handler.HandlerID, Digest: handler.Digest, ConfigPath: handler.ConfigPath, Handler: append(json.RawMessage(nil), handler.Handler...)})
		}
	}
	return legacy, true, nil
}

func sameLegacyInventories(a, b []LegacyHostInventory) bool {
	if len(a) != len(b) {
		return false
	}
	normalize := func(values []LegacyHostInventory) []LegacyHostInventory {
		copyValues := append([]LegacyHostInventory(nil), values...)
		for i := range copyValues {
			copyValues[i].Handlers = append([]LegacyHandlerInventory(nil), copyValues[i].Handlers...)
			for j := range copyValues[i].Handlers {
				compact := new(bytes.Buffer)
				if json.Compact(compact, copyValues[i].Handlers[j].Handler) == nil {
					copyValues[i].Handlers[j].Handler = compact.Bytes()
				}
			}
		}
		return copyValues
	}
	observed, signed := normalize(a), normalize(b)
	for i := range observed {
		if observed[i].NativeManifest.Path != signed[i].NativeManifest.Path || observed[i].NativeRevision < signed[i].NativeRevision || (observed[i].NativeRevision-signed[i].NativeRevision)%2 != 0 {
			return false
		}
		// A completed cutover followed by its exact rollback advances the CAS
		// revision by two while restoring the signed native file set.
		observed[i].NativeRevision = signed[i].NativeRevision
		observed[i].NativeManifest = signed[i].NativeManifest
	}
	return reflect.DeepEqual(observed, signed)
}

func inventoryForHost(inventories []LegacyHostInventory, host Host) LegacyHostInventory {
	for _, inventory := range inventories {
		if inventory.Host == host {
			return inventory
		}
	}
	return LegacyHostInventory{}
}

func mutationMatchesLegacyIdentity(mutation lifecycleMutation, identity LegacyFileIdentity) bool {
	return mutation.Existed && mutation.Path == identity.Path && mutation.PreSHA256 == identity.SHA256 && mutation.PreMode == identity.Mode && int64(len(mutation.Preimage)) == identity.Size
}

func verifyLegacyOwnership(layout Layout, receipt legacyInstallReceipt, hosts []Host) error {
	for _, host := range hosts {
		installed, ok := receipt.InstalledFileMaps[string(host)]
		if !ok || !sameHostPath(installed.Target, layout.SkillRoots[host]) || len(installed.Files) == 0 {
			return core.ErrRevision
		}
		if skill, owned := installed.Files["SKILL.md"]; !owned || !validSHA256(skill.SHA256) || skill.Size < 0 {
			return fmt.Errorf("%w: top-level SKILL.md is not receipt-owned", core.ErrRevision)
		}
		for relative, file := range installed.Files {
			if !safeInstallRelative(relative) || !validSHA256(file.SHA256) || file.Size < 0 {
				return core.ErrRevision
			}
			path := filepath.Join(installed.Target, filepath.FromSlash(relative))
			info, statErr := os.Lstat(path)
			if statErr != nil || !info.Mode().IsRegular() || info.Size() != file.Size || uint32(info.Mode().Perm()) != file.Mode || digestPathBounded(path, installJournalLimit) != file.SHA256 {
				return fmt.Errorf("%w: legacy owned file changed: %s", core.ErrRevision, path)
			}
		}
		if installed.Digest != legacyFileMapDigest(installed.Files) {
			return fmt.Errorf("%w: legacy file map digest changed", core.ErrRevision)
		}
		handlers := 0
		for _, handler := range receipt.Handlers {
			if handler.Runtime == string(host) && !handler.Preexisting {
				handlers++
			}
		}
		if handlers == 0 {
			return fmt.Errorf("%w: legacy handler ownership missing", core.ErrRevision)
		}
	}
	return nil
}

func retireLegacyHandlers(raw []byte, path, runtime string, receipts []legacyHandler) ([]byte, error) {
	if len(raw) > legacyReceiptLimit {
		return nil, core.ErrPath
	}
	root, err := parseJSONSpans(raw)
	if err != nil {
		return nil, core.ErrRevision
	}
	hooks := root.member("hooks")
	if hooks == nil || hooks.kind != '{' {
		return nil, core.ErrRevision
	}
	ownedByArray := map[*jsonNode]map[*jsonNode]bool{}
	for _, receipt := range receipts {
		if receipt.Runtime != runtime || receipt.Preexisting {
			continue
		}
		var expected map[string]any
		compact := new(bytes.Buffer)
		if !sameHostPath(receipt.ConfigPath, path) || receipt.Event == "" || receipt.HandlerID == "" || !validSHA256(receipt.Digest) || len(receipt.Handler) == 0 || json.Unmarshal(receipt.Handler, &expected) != nil || json.Compact(compact, receipt.Handler) != nil || digestContent(compact.Bytes()) != receipt.Digest {
			return nil, core.ErrRevision
		}
		groups := hooks.member(receipt.Event)
		if groups == nil || groups.kind != '[' {
			return nil, core.ErrRevision
		}
		var matched, matchedArray *jsonNode
		for _, group := range groups.items {
			if group.kind != '{' {
				continue
			}
			handlers := group.member("hooks")
			if handlers == nil || handlers.kind != '[' {
				continue
			}
			for _, candidate := range handlers.items {
				var candidateMap map[string]any
				_ = json.Unmarshal(raw[candidate.start:candidate.end], &candidateMap)
				if reflect.DeepEqual(candidateMap, expected) {
					if matched != nil {
						return nil, fmt.Errorf("%w: legacy handler is missing or ambiguous", core.ErrRevision)
					}
					matched, matchedArray = candidate, handlers
				}
			}
		}
		if matched == nil {
			return nil, fmt.Errorf("%w: legacy handler is missing or ambiguous", core.ErrRevision)
		}
		if ownedByArray[matchedArray] == nil {
			ownedByArray[matchedArray] = map[*jsonNode]bool{}
		}
		ownedByArray[matchedArray][matched] = true
	}
	var removals []jsonSpan
	for array, owned := range ownedByArray {
		removals = append(removals, array.removals(owned)...)
	}
	sort.Slice(removals, func(i, j int) bool { return removals[i].start > removals[j].start })
	encoded := append([]byte(nil), raw...)
	for _, span := range removals {
		encoded = append(encoded[:span.start], encoded[span.end:]...)
	}
	if !json.Valid(encoded) {
		return nil, core.ErrRevision
	}
	return encoded, nil
}

func readHostCutoverReceipt(layout Layout) (hostCutoverReceipt, error) {
	path := filepath.Join(layout.DataRoot, filepath.FromSlash(hostCutoverReceiptRel))
	info, err := os.Lstat(path)
	if err != nil {
		return hostCutoverReceipt{}, err
	}
	if !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > installJournalLimit {
		return hostCutoverReceipt{}, core.ErrPath
	}
	raw, err := readStableRegular(layout.DataRoot, path, info.Size(), nil, "")
	if err != nil {
		return hostCutoverReceipt{}, err
	}
	var receipt hostCutoverReceipt
	if json.Unmarshal(raw, &receipt) != nil || receipt.Schema != 1 || receipt.ReceiptDigest != digestHostReceipt(receipt) {
		return hostCutoverReceipt{}, core.ErrRevision
	}
	return receipt, nil
}

func verifyHostCutoverState(receipt hostCutoverReceipt) error {
	for _, image := range append(append([]hostCutoverPostimage(nil), receipt.Postimages...), receipt.Retained...) {
		if image.Absent {
			if _, err := os.Lstat(image.Path); !errors.Is(err, fs.ErrNotExist) {
				return core.ErrRevision
			}
			continue
		}
		if digestPathBounded(image.Path, installJournalLimit) != image.SHA256 {
			return core.ErrRevision
		}
	}
	return nil
}

func digestHostReceipt(receipt hostCutoverReceipt) string {
	receipt.ReceiptDigest = ""
	raw, _ := json.Marshal(receipt)
	return digestContent(raw)
}
func digestPathBounded(path string, limit int64) string {
	digest, size, err := sha256File(path)
	if err != nil || size > limit {
		return ""
	}
	return digest
}
func safeInstallRelative(value string) bool {
	clean := filepath.Clean(filepath.FromSlash(value))
	return value != "" && !filepath.IsAbs(value) && clean != "." && clean != ".." && !strings.HasPrefix(clean, ".."+string(filepath.Separator))
}

func legacyFileMapDigest(files map[string]legacyInstalledFile) string {
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	entries := make([][]any, 0, len(names))
	for _, name := range names {
		file := files[name]
		entries = append(entries, []any{name, file.SHA256, file.Mode, file.Size})
	}
	raw, _ := json.Marshal(entries)
	return digestContent(append(raw, '\n'))
}

func sameHostPath(left, right string) bool {
	left, right = filepath.Clean(left), filepath.Clean(right)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(left, right)
	}
	return left == right
}
