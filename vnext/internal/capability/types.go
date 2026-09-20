// Package capability describes optional, explicitly consented tools. It does
// not make any capability a requirement for native project work.
package capability

import (
	"context"

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
	Available bool
	Healthy   bool
	Reason    string
}

// Consent is the user's one-time, explicit selection of an optional adapter.
// It deliberately contains no credentials or host configuration.
type Consent struct {
	Name             Name
	Enabled          bool
	Mode             Mode
	InstallerPackage string
	Source           string
	Rollback         string
}

type OwnedFile struct {
	Path   string
	Role   string
	SHA256 string
}

// RollbackEntry identifies both the file to restore and the authority used to
// restore it. A path and expected hash alone cannot restore prior state.
type RollbackEntry struct {
	Path    string
	SHA256  string
	Backup  string
	Command []string
}

type InstallPlan struct {
	Name            Name
	Package         string
	Source          string
	VerifiedVersion string
	Scope           string
	OwnedFiles      []OwnedFile
	SettingsChanged []string
	Rollback        []RollbackEntry
	Command         []string
	ProbeCommand    []string
	Explicit        bool
}

// NativeResult aliases the repository's bounded, shell-free command outcome.
type NativeResult = tracker.CommandResult

// NativeRunner deliberately matches tracker.CommandRunner so existing bounded
// execution can be injected directly. Commands are executable plus argv only.
type NativeRunner interface {
	Run(context.Context, string, ...string) tracker.CommandResult
}

type OutputStore interface {
	Write(context.Context, string, []byte, int64) (string, error)
}

type TokenSource interface {
	Read(context.Context, string) (int, error)
}
