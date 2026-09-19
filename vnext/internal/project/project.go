// Package project establishes a portable, canonical identity for one Git project.
package project

import (
	"context"

	"github.com/thebpandey/agent-team/vnext/internal/core"
)

// Project is the preflight identity and accessibility state of a Git worktree.
type Project struct {
	Root, TopLevel, CommonDir, Head string
	Dirty, Detached                 bool
	Readable, Writable              bool
	FreeBytes                       int64
}

// ArtifactMode describes whether setup consumes an existing artifact or records
// an approved request to generate one later.
type ArtifactMode string

const (
	ExistingArtifact  ArtifactMode = "existing"
	GeneratedArtifact ArtifactMode = "generated"
)

// RunMode selects a tracker-backed plan or an immutable trackerless one-off.
type RunMode string

const (
	PlanMode   RunMode = "plan"
	OneOffMode RunMode = "one-off"
)

// Confirmation is an explicit decision supplied by the caller. Setup never
// prompts, infers, or persists a missing confirmation.
type Confirmation string

const (
	Approved Confirmation = "approved"
	Refused  Confirmation = "refused"
)

type ArtifactDecision struct {
	Path         string
	Mode         ArtifactMode
	Confirmation Confirmation
}

type KickoffDecision struct {
	Path, Digest string
	Confirmation Confirmation
}

type SetupInput struct {
	Root      string
	Mode      RunMode
	Artifacts []ArtifactDecision
	Kickoff   *KickoffDecision
}

type SetupResult struct {
	Project         Project
	Config          core.Config
	ArtifactDigests map[string]string
	Handoff         core.KickoffHandoff
	ConfigRevision  uint64
	ReceiptPath     string
}

// SetupService separates read-only setup validation from state initialization.
type SetupService interface {
	Initialize(context.Context, SetupInput) (SetupResult, error)
	Validate(context.Context, SetupInput) (SetupResult, error)
}
