package migrate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/thebpandey/agent-team/vnext/internal/core"
	state "github.com/thebpandey/agent-team/vnext/internal/store"
)

const (
	migrationLimit = 1 << 20
	inventoryPath  = ".agent-team/migration/v7-inventory.json"
)

var migrationLocks sync.Map

type inventoryDocument struct {
	Schema int `json:"schema"`
	Inventory
}

func InventoryV7(ctx context.Context, project, legacyPath string) (Inventory, error) {
	if ctx == nil || ctx.Err() != nil {
		return Inventory{}, core.ErrPath
	}
	project, err := cleanProject(project)
	if err != nil {
		return Inventory{}, err
	}
	relative, err := projectRelative(project, legacyPath)
	if err != nil {
		return Inventory{}, err
	}
	root, err := os.OpenRoot(project)
	if err != nil {
		return Inventory{}, core.ErrPath
	}
	defer root.Close()
	file, err := root.Open(relative)
	if err != nil {
		return Inventory{}, core.ErrPath
	}
	digest, size, err := hashFile(file, migrationLimit)
	closeErr := file.Close()
	if err != nil {
		return Inventory{}, err
	}
	if closeErr != nil {
		return Inventory{}, core.ErrPath
	}
	inv := Inventory{Project: project, Files: []V7File{{Path: filepath.ToSlash(relative), SHA256: digest, Bytes: size}}, BackupPath: inventoryPath}
	canonical, err := json.Marshal(struct {
		Project string   `json:"project"`
		Files   []V7File `json:"files"`
	}{inv.Project, inv.Files})
	if err != nil {
		return Inventory{}, err
	}
	sum := sha256.Sum256(canonical)
	inv.Digest = hex.EncodeToString(sum[:])
	if _, err := state.New(project, core.StorageLimits{CanonicalBytes: migrationLimit}).WriteJSON(inventoryPath, inventoryDocument{Schema: 1, Inventory: inv}, migrationLimit); err != nil {
		return Inventory{}, err
	}
	return inv, nil
}

func cleanProject(project string) (string, error) {
	if project == "" || !filepath.IsAbs(project) || filepath.Clean(project) != project {
		return "", core.ErrPath
	}
	info, err := os.Stat(project)
	if err != nil || !info.IsDir() {
		return "", core.ErrPath
	}
	return project, nil
}

func projectRelative(project, target string) (string, error) {
	if target == "" || !filepath.IsAbs(target) || filepath.Clean(target) != target {
		return "", core.ErrPath
	}
	relative, err := filepath.Rel(project, target)
	if err != nil || relative == "." || relative == ".." || filepath.IsAbs(relative) || startsDotDot(relative) {
		return "", core.ErrPath
	}
	return relative, nil
}

func startsDotDot(path string) bool {
	return len(path) > 3 && path[:3] == ".."+string(filepath.Separator)
}

func hashRegular(path string, limit int64) (string, int64, error) {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() > limit {
		return "", 0, core.ErrPath
	}
	file, err := os.Open(path)
	if err != nil {
		return "", 0, core.ErrPath
	}
	defer file.Close()
	return hashFile(file, limit)
}

func hashFile(file *os.File, limit int64) (string, int64, error) {
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > limit {
		return "", 0, core.ErrPath
	}
	hash := sha256.New()
	n, err := io.Copy(hash, io.LimitReader(file, limit+1))
	if err != nil || n > limit || n != info.Size() {
		return "", 0, core.ErrPath
	}
	return hex.EncodeToString(hash.Sum(nil)), n, nil
}

func readInventory(project string) (Inventory, error) {
	var doc inventoryDocument
	err := state.New(project, core.StorageLimits{CanonicalBytes: migrationLimit}).ReadJSON(inventoryPath, migrationLimit, &doc)
	if errors.Is(err, os.ErrNotExist) {
		sum := sha256.Sum256(nil)
		return Inventory{Project: project, Files: []V7File{}, Digest: hex.EncodeToString(sum[:]), BackupPath: inventoryPath}, nil
	}
	if err != nil || doc.Schema != 1 || doc.Project != project || !validDigest(doc.Digest) {
		return Inventory{}, fmt.Errorf("%w: invalid v7 inventory", core.ErrRevision)
	}
	canonical, marshalErr := json.Marshal(struct {
		Project string   `json:"project"`
		Files   []V7File `json:"files"`
	}{doc.Project, doc.Files})
	sum := sha256.Sum256(canonical)
	if marshalErr != nil || doc.Digest != hex.EncodeToString(sum[:]) {
		return Inventory{}, fmt.Errorf("%w: invalid v7 inventory digest", core.ErrRevision)
	}
	return doc.Inventory, nil
}
