// Package contracts contains value types used by later execution phases.
//
// These types intentionally contain no operational behavior. They describe
// boundaries that a future adapter may implement without making Phase 1
// start workers, mutate worktrees, or submit deployments.
package contracts

import "github.com/thebpandey/agent-team/vnext/internal/core"

type HostCapabilities struct {
	Host            string
	OS              string
	ConfiguredSlots int
	ObservedSlots   int
	UsableSlots     int
	DeveloperSlots  int
	ReviewerSlots   int
	Models          []string
	Servers         int
	Browsers        int
	Unknown         bool
}

type WorkerRequest struct {
	Packet        core.AssignmentPacket
	Worktree      WorktreeSpec
	WritablePaths []string
	Reviewer      bool
	Model         string
}

type WorkerHandle struct {
	Host              string
	Identity          string
	PacketDigest      string
	CandidateRevision string
	Run               core.RunID
	Team              core.TeamID
	Task              core.TaskID
	Attempt           int
	Sequence          uint64
	Reviewer          bool
}

type WorktreeSpec struct {
	Run           core.RunID
	Team          core.TeamID
	Root          string
	Base          string
	WritablePaths []string
}

type Worktree struct {
	Run       core.RunID
	Path      string
	Branch    string
	Base      string
	Candidate string
	Canonical string
	Team      core.TeamID
	Dirty     bool
}

type Candidate struct {
	Task     core.TaskID
	Revision string
	Base     string
	Worktree Worktree
}

type GateInput struct {
	Run                       core.RunID
	Task                      core.TaskID
	Candidate                 Candidate
	TrackerRevision           uint64
	ReceiptRevision           uint64
	RequiredCheckFingerprints []string
	ScopeFingerprint          string
	WorktreeDirty             bool
	ReceiptDigest             string
	ReviewDigest              string
}

type GateResult struct {
	Revision          string
	Result            string
	Evidence          string
	CleanEvidence     string
	CheckFingerprints []string
}

type DeploymentBatch struct {
	Run                 core.RunID
	TaskIDs             []core.TaskID
	Revisions           []string
	TargetProfile       string
	AuthorizationRef    string
	ExecutorCommand     []string
	VerificationCommand []string
	Fingerprint         string
	IdempotencyKey      string
}

type Operation struct {
	Provider       string
	ProviderID     string
	IdempotencyKey string
	State          string
	ExternalState  string
	Unknown        bool
}

type Verification struct {
	State            string
	OutputPointer    string
	InputFingerprint string
	Command          []string
	Exit             int
}
