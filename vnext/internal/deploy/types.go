package deploy

import (
	"context"

	"github.com/thebpandey/agent-team/vnext/internal/contracts"
	"github.com/thebpandey/agent-team/vnext/internal/core"
	"github.com/thebpandey/agent-team/vnext/internal/knowledge"
)

type ProfileID string

type ProfileDecision struct {
	Enable                                             bool
	Target, AuthorizationRef, ApprovalScope            string
	ExecutorCommand, QueryCommand, VerificationCommand []string
	DefaultBatchSize                                   int
	Confirmed                                          bool
}

type TargetProfile struct {
	core.RecordEnvelope
	ID                                                     ProfileID
	Target, AuthorizationRef, ApprovalScope, ProfileDigest string
	ExecutorCommand, QueryCommand, VerificationCommand     []string
	DefaultBatchSize                                       int
	Enabled, Confirmed                                     bool
}

type ProfileWriteKind string

const (
	ProfileCreated   ProfileWriteKind = "created"
	ProfileUpdated   ProfileWriteKind = "updated"
	ProfileDuplicate ProfileWriteKind = "duplicate"
)

type ProfileWriteOutcome struct {
	Kind                               ProfileWriteKind
	Profile                            TargetProfile
	ExpectedRevision, ObservedRevision uint64
	Idempotent                         bool
}

type EligibleTask struct {
	ID                                           core.TaskID
	CanonicalRevision                            string
	ArtifactDigests                              []string
	Integrated, GatePassed, DependenciesComplete bool
}

type BatchRequest struct {
	RunID     core.RunID
	Target    string
	Profile   TargetProfile
	Tasks     []EligibleTask
	BatchSize int
}

type BatchManifest struct {
	core.RecordEnvelope
	BatchID, Target, ProfileDigest, Fingerprint, IdempotencyKey string
	RunID                                                       core.RunID
	Sequence                                                    int
	TaskIDs                                                     []core.TaskID
	Revisions                                                   []string
	ArtifactDigests                                             [][]string
	Final                                                       bool
}

type CommandInvocation struct {
	Executable              string
	Args, Env               []string
	ArgumentFile, OwnedRoot string
	OutputLimit             int64
}

type CommandResult struct {
	Stdout, Stderr                     []byte
	Exit                               int
	TimedOut, Started, OutputTruncated bool
	Transport                          error
}

type OperationRecord struct {
	core.RecordEnvelope
	BatchID, Fingerprint, ProfileID, Target, State string
	Operation                                      contracts.Operation
	Attempt                                        int
}

type DeploymentEvidence struct {
	core.RecordEnvelope
	BatchID, ProfileID, Target, Fingerprint, State, Error, OutputPointer string
	Attempt, Exit                                                        int
	Command                                                              []string
}

type DeploymentReceipt struct {
	core.RecordEnvelope
	RunID                                                                      core.RunID
	BatchID, ProviderID, ProfileID, Target, Fingerprint, IdempotencyKey, State string
	Verification                                                               contracts.Verification
	EvidencePointers                                                           []string
	CodingMayContinue                                                          bool
}

type DeployOutcome struct {
	Receipt           DeploymentReceipt
	BlockerID         string
	CodingMayContinue bool
}

type DashboardTriggerRequest struct {
	RunID                           core.RunID
	BatchID, ReceiptPointer, Reason string
}

type ProviderAdapter interface {
	Submit(context.Context, contracts.DeploymentBatch, string) (contracts.Operation, error)
	Query(context.Context, contracts.Operation, string) (contracts.Operation, error)
	Verify(context.Context, contracts.Operation, string) (contracts.Verification, error)
}

type ProcessRunner interface {
	Run(context.Context, CommandInvocation) CommandResult
}

type Repository interface {
	ReadOperation(context.Context, string) (OperationRecord, error)
	CompareAndSwapOperation(context.Context, uint64, OperationRecord) (OperationRecord, error)
	WriteEvidence(context.Context, DeploymentEvidence) (string, error)
	ReadReceipt(context.Context, string) (DeploymentReceipt, error)
	CompareAndSwapReceipt(context.Context, uint64, DeploymentReceipt) (DeploymentReceipt, error)
}

type ProfileRepository interface {
	ApplyDecision(context.Context, ProfileID, ProfileDecision, uint64) (ProfileWriteOutcome, error)
	Get(context.Context, ProfileID) (TargetProfile, error)
}

type BlockerWriter interface {
	Create(context.Context, knowledge.Blocker) (knowledge.Blocker, error)
}
type HoldWriter interface {
	HoldLater(context.Context, core.RunID, string) error
}
type DashboardTrigger interface {
	Trigger(context.Context, DashboardTriggerRequest) error
}

type BoundExecutor struct {
	Profile  TargetProfile
	Provider ProviderAdapter
}

type CommandProvider struct {
	Profile TargetProfile
	Runner  ProcessRunner
	Root    string
	Limit   int64
}

type RecoveryService struct {
	Blockers  BlockerWriter
	Holds     HoldWriter
	Dashboard DashboardTrigger
}
