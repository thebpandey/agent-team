// Package capability contains explicit, non-authoritative optional-tool
// contracts. Native project work remains available without these tools.
package capability

import (
	"context"
	"sync"

	"github.com/thebpandey/agent-team/vnext/internal/tracker"
)

type Name string

const (
	Native           Name = "native"
	UsingSuperpowers Name = "using-superpowers"
	LeanCTX          Name = "leanctx"
	Serena           Name = "serena"
	Graphify         Name = "graphify"
	AstGrep          Name = "ast-grep"
	Playwright       Name = "playwright"
	Impeccable       Name = "impeccable"
	UIUXProMax       Name = "ui-ux-pro-max"
	UIStyling        Name = "ui-styling"
)

type Mode string

const (
	CLI          Mode = "cli"
	ReadOnlyMCP  Mode = "readonly-mcp"
	SkillContent Mode = "skill-content"
)

type Probe struct {
	Name      Name
	Mode      Mode
	Path      string
	Version   string
	Digest    string
	Source    string
	Available bool
	Healthy   bool
	Reason    string
}

type FileRole string

const (
	ToolBinary   FileRole = "tool-binary"
	ToolMetadata FileRole = "tool-metadata"
)

type OwnedFile struct {
	Path   string
	Role   FileRole
	SHA256 string
}

// Action is a complete, argument-array action presented for consent. Only the
// three fixed adapter actions accepted by BuildInstallPlan can be used.
type Action struct{ Argv []string }

// Consent is untrusted input at the approval boundary. BuildInstallPlan copies
// it into an opaque plan, so later caller mutation cannot alter authorization.
type Consent struct {
	Name             Name
	Enabled          bool
	Mode             Mode
	Source           string
	VerifiedVersion  string
	InstallerPackage string
	ProjectRoot      string
	Install          Action
	Probe            Action
	Rollback         Action
	OwnedFiles       []OwnedFile
}

// InstallPlan intentionally exposes no mutable authorization fields.
type InstallPlan struct{ state *planState }

type planState struct {
	mu sync.Mutex

	name                       Name
	source, version, pkg, root string
	install, probe, rollback   []string
	owned                      []OwnedFile
	backups                    []backup
}

type backup struct {
	path   string
	exists bool
	bytes  []byte
	hash   string
	mode   uint32
}

// NativeResult is the existing bounded shell-free command outcome.
type NativeResult = tracker.CommandResult

type OutputStore interface {
	Write(context.Context, string, []byte, int64) (string, error)
}

type TokenSource interface {
	Read(context.Context, string) (int, error)
}
