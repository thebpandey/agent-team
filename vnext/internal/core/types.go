package core

import (
	"context"
	"errors"
	"io"
)

type TaskID string
type RunID string
type TeamID string
type TaskState string

const (
	Ready        TaskState = "ready"
	Idle         TaskState = "idle"
	Working      TaskState = "working"
	Implementing TaskState = "implementing"
	Reviewing    TaskState = "reviewing"
	Fix          TaskState = "fix"
	Clean        TaskState = "clean"
	Gated        TaskState = "gated"
	Integrated   TaskState = "integrated"
	Paused       TaskState = "paused"
	Blocked      TaskState = "blocked"
	Interrupted  TaskState = "interrupted"
	Cancelled    TaskState = "cancelled"
	Archived     TaskState = "archived"
)

type ScopeKind string

const (
	ScopeProject ScopeKind = "project"
	ScopeRun     ScopeKind = "run"
	ScopeTeam    ScopeKind = "team"
	ScopeTask    ScopeKind = "task"
)

type Scope struct {
	Kind ScopeKind `json:"kind"`
	ID   string    `json:"id"`
}

type Check struct {
	Name    string   `json:"name"`
	Command []string `json:"command"`
}

type SkillRef struct {
	Name            string `json:"name"`
	Path            string `json:"path"`
	Digest          string `json:"digest"`
	Purpose         string `json:"purpose"`
	DesignAuthority string `json:"designAuthority,omitempty"`
	Required        bool   `json:"required"`
}

type AcceleratorStatus string

const (
	AcceleratorAvailable   AcceleratorStatus = "available"
	AcceleratorUnavailable AcceleratorStatus = "unavailable"
	AcceleratorFailed      AcceleratorStatus = "failed"
)

type AcceleratorRef struct {
	Name   string            `json:"name"`
	Path   string            `json:"path"`
	Digest string            `json:"digest"`
	Status AcceleratorStatus `json:"status"`
	Reason string            `json:"reason"`
}

type RecordEnvelope struct {
	Schema    int    `json:"schema"`
	Project   string `json:"project"`
	RunID     RunID  `json:"runId"`
	WrittenAt string `json:"writtenAt"`
	Revision  uint64 `json:"revision"`
}

type Task struct {
	RecordEnvelope
	ID               TaskID    `json:"id"`
	Objective        string    `json:"objective"`
	State            TaskState `json:"state"`
	Dependencies     []TaskID  `json:"dependencies"`
	Criteria         []string  `json:"criteria"`
	Checks           []Check   `json:"checks"`
	WritablePaths    []string  `json:"writablePaths"`
	Resources        []string  `json:"resources"`
	EvidencePointers []string  `json:"evidencePointers"`
	Archived         bool      `json:"archived"`
}

type TrackerPage struct {
	TrackerRevision  uint64 `json:"trackerRevision"`
	TotalNonArchived int    `json:"totalNonArchived"`
	Cursor           string `json:"cursor"`
	Tasks            []Task `json:"tasks"`
}

type ResourceSnapshot struct {
	Servers  []string `json:"servers"`
	Browsers []string `json:"browsers"`
	External []string `json:"external"`
}

type RunRecord struct {
	RecordEnvelope
	ID                RunID     `json:"id"`
	SpecRevision      string    `json:"specRevision"`
	TrackerKind       string    `json:"trackerKind"`
	TrackerRevision   uint64    `json:"trackerRevision"`
	CanonicalRevision string    `json:"canonicalRevision"`
	State             TaskState `json:"state"`
}

type KickoffHandoff struct {
	ApprovedPlanRevision string   `json:"approvedPlanRevision"`
	Branch               string   `json:"branch"`
	TrackerKind          string   `json:"trackerKind"`
	TrackerRef           string   `json:"trackerRef"`
	TrackerRevision      uint64   `json:"trackerRevision"`
	TaskIDs              []TaskID `json:"taskIds"`
	Acceptance           []string `json:"acceptance"`
	Checks               []Check  `json:"checks"`
	WritablePaths        []string `json:"writablePaths"`
	Resources            []string `json:"resources"`
	Capabilities         []string `json:"capabilities"`
}

type AssignmentPacket struct {
	RecordEnvelope
	SpecRevision     string           `json:"specRevision"`
	Task             TaskID           `json:"task"`
	Team             TeamID           `json:"team"`
	QueueFingerprint string           `json:"queueFingerprint"`
	Owner            string           `json:"owner"`
	Worktree         string           `json:"worktree"`
	Base             string           `json:"base"`
	Criteria         []string         `json:"criteria"`
	Scope            []string         `json:"scope"`
	Checks           []Check          `json:"checks"`
	Capabilities     []string         `json:"capabilities"`
	Skills           []SkillRef       `json:"skills"`
	Resources        ResourceSnapshot `json:"resources"`
	Accelerators     []AcceleratorRef `json:"accelerators"`
	Review           string           `json:"review"`
	Gate             string           `json:"gate"`
	NextAction       string           `json:"nextAction"`
	ReceiptPath      string           `json:"receiptPath"`
}

// Clone returns a packet whose mutable slices do not alias the original.
func (packet AssignmentPacket) Clone() AssignmentPacket {
	packet.Criteria = append([]string(nil), packet.Criteria...)
	packet.Scope = append([]string(nil), packet.Scope...)
	packet.Checks = append([]Check(nil), packet.Checks...)
	for i := range packet.Checks {
		packet.Checks[i].Command = append([]string(nil), packet.Checks[i].Command...)
	}
	packet.Capabilities = append([]string(nil), packet.Capabilities...)
	packet.Skills = append([]SkillRef(nil), packet.Skills...)
	packet.Resources.Servers = append([]string(nil), packet.Resources.Servers...)
	packet.Resources.Browsers = append([]string(nil), packet.Resources.Browsers...)
	packet.Resources.External = append([]string(nil), packet.Resources.External...)
	packet.Accelerators = append([]AcceleratorRef(nil), packet.Accelerators...)
	return packet
}

type Dependencies struct {
	ProjectRoot      string                                        `json:"projectRoot"`
	Stdout           io.Writer                                     `json:"-"`
	Stderr           io.Writer                                     `json:"-"`
	OutputLimit      int                                           `json:"-"`
	Confirmations    map[string]bool                               `json:"confirmations"`
	ExecuteLifecycle func(context.Context, string, []string) error `json:"-"`
	Deployment       DeploymentAction                              `json:"-"`
	Management       ManagementAction                              `json:"-"`
}

type DeploymentAction func(context.Context, []string, io.Writer, io.Writer) int

type ManagementAction func(context.Context, []string, io.Writer, io.Writer) int

var ErrOutputLimit = errors.New("management output limit exceeded")

const DefaultOutputLimit = 8192

type BoundedOutput struct {
	Limit    int
	Data     []byte
	Overflow bool
}

func (b *BoundedOutput) Write(p []byte) (int, error) {
	limit := b.Limit
	if limit <= 0 {
		limit = DefaultOutputLimit
	}
	if len(b.Data)+len(p) > limit {
		b.Overflow = true
		return 0, ErrOutputLimit
	}
	b.Data = append(b.Data, p...)
	return len(p), nil
}

type ErrorCode string

type ErrorEnvelope struct {
	Code    ErrorCode         `json:"code"`
	Message string            `json:"message"`
	Details map[string]string `json:"details,omitempty"`
}
