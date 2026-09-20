package migrate

import (
	"context"
	"sync"

	"github.com/thebpandey/agent-team/vnext/internal/core"
	"github.com/thebpandey/agent-team/vnext/internal/install"
)

type V7File struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Bytes  int64  `json:"bytes"`
}

type Inventory struct {
	Project    string   `json:"project"`
	Files      []V7File `json:"files"`
	Digest     string   `json:"digest"`
	BackupPath string   `json:"backupPath"`
}

type Observation struct {
	Host          install.Host `json:"host"`
	Identity      string       `json:"identity"`
	Revision      string       `json:"revision"`
	TrackerDigest string       `json:"trackerDigest"`
	ReceiptDigest string       `json:"receiptDigest"`
	State         string       `json:"state"`
}

type CanaryArtifact struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

type CanaryRecord struct {
	core.RecordEnvelope
	ID                string           `json:"id"`
	Project           string           `json:"project"`
	V7InventoryDigest string           `json:"v7InventoryDigest"`
	Codex             Observation      `json:"codex,omitempty"`
	Claude            Observation      `json:"claude,omitempty"`
	State             string           `json:"state"`
	RollbackEvidence  string           `json:"rollbackEvidence,omitempty"`
	Owned             []CanaryArtifact `json:"owned,omitempty"`
	Retained          []string         `json:"retained,omitempty"`
	Idempotent        bool             `json:"idempotent,omitempty"`
}

type CanaryRunner interface {
	Observe(context.Context, string, install.Host) (Observation, error)
}

type Store struct {
	Root string
	mu   *sync.Mutex
}
