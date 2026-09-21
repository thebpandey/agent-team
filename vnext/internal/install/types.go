package install

import (
	"sync"
)

type Host string

const (
	Codex  Host = "codex"
	Claude Host = "claude"
)

type FileRole string

const (
	BinaryRole     FileRole = "binary"
	ContractRole   FileRole = "worker-contract"
	EntrypointRole FileRole = "skill-entrypoint"
)

type ReleaseFile struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Bytes  int64  `json:"bytes"`
}

type Release struct {
	Version     string               `json:"version"`
	Revision    string               `json:"revision"`
	Binary      ReleaseFile          `json:"binary"`
	Contract    ReleaseFile          `json:"contract"`
	Entrypoints map[Host]ReleaseFile `json:"entrypoints"`
}

type OwnedFile struct {
	Role     FileRole `json:"role"`
	Host     Host     `json:"host,omitempty"`
	Path     string   `json:"path"`
	SHA256   string   `json:"sha256"`
	Version  string   `json:"version"`
	Revision string   `json:"release_revision"`
	Bytes    int64    `json:"bytes"`
}

type Backup struct {
	Role     FileRole `json:"role"`
	Host     Host     `json:"host,omitempty"`
	Path     string   `json:"path"`
	SHA256   string   `json:"sha256"`
	Version  string   `json:"version"`
	Revision string   `json:"release_revision"`
	Bytes    int64    `json:"bytes"`
}

type InstallManifest struct {
	Schema          int             `json:"schema"`
	Revision        uint64          `json:"revision"`
	Version         string          `json:"version"`
	ReleaseRevision string          `json:"release_revision"`
	Hosts           []Host          `json:"hosts"`
	HostHomes       map[Host]string `json:"host_homes,omitempty"`
	Files           []OwnedFile     `json:"files"`
	Backups         []Backup        `json:"backups"`
}

type CASKind string

const (
	CASCreated   CASKind = "created"
	CASUpdated   CASKind = "updated"
	CASDuplicate CASKind = "duplicate"
	CASStale     CASKind = "stale"
)

type CASOutcome struct {
	Kind                               CASKind
	Manifest                           InstallManifest
	ExpectedRevision, ObservedRevision uint64
	Idempotent                         bool
	Retained                           []string
}

type Layout struct {
	DataRoot, BinaryPath, ContractPath, ManifestPath string
	SkillRoots                                       map[Host]string
	ConfigPaths                                      map[Host]string
	CodexDiscoverySkillPaths                         []string
}

type ManifestStore struct {
	Root string
	Path string
	mu   *sync.Mutex
}
