// Package capability defines optional, non-authoritative capability boundaries.
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
	Name                  Name
	Mode                  Mode
	Path, Version, Digest string
	Available, Healthy    bool
	Reason                string
}

// Consent contains only the facts shown to and approved by a user. Adapter
// command lines and filesystem locations are deliberately not caller inputs.
type Consent struct {
	Name                     Name
	Enabled                  bool
	Mode                     Mode
	InstallerPackage, Source string
}

// NativeRunner receives one exact argv and a complete, scrubbed environment.
// CommandResult remains the single process result type in this module.
type NativeRunner interface {
	Run(context.Context, []string, []string) tracker.CommandResult
}

// InstallPlan is intentionally opaque. A zero value is not an installation.
type InstallPlan struct{ state *planState }

type planState struct {
	mu                 sync.Mutex
	used               bool
	published, removed bool
	spec               adapterSpec
}

// adapterSpec is a closed, same-package seam. Task 21 adapters supply fixed
// facts here; public callers can neither register adapters nor forge plans.
type adapterSpec struct {
	name                                  Name
	mode                                  Mode
	packageName, source, version, project string
	sourceArtifact, sourceDigest          string
	stage, destination, stageDigest       string
	installArgv, probeArgv                []string
}

type OutputStore interface {
	Write(context.Context, string, []byte, int64) (string, error)
}

type TokenSource interface {
	Read(context.Context, string) (int, error)
}
