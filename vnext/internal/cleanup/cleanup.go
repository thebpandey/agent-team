package cleanup

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/thebpandey/agent-team/vnext/internal/contracts"
	"github.com/thebpandey/agent-team/vnext/internal/core"
	"github.com/thebpandey/agent-team/vnext/internal/project"
	"github.com/thebpandey/agent-team/vnext/internal/store"
	"github.com/thebpandey/agent-team/vnext/internal/worktree"
)

type ServerRef struct {
	Run          core.RunID
	Team         core.TeamID
	ResourceID   string
	Revision     uint64
	Ownership    string
	StopEvidence string
}
type BrowserRef struct {
	Run          core.RunID
	Team         core.TeamID
	ResourceID   string
	Revision     uint64
	Ownership    string
	StopEvidence string
}
type CleanupCandidate struct {
	Project                          string
	Run                              core.RunID
	Team                             core.TeamID
	Task                             core.TaskID
	Worktree, Branch, Base, Revision string
	Servers                          []ServerRef
	Browsers                         []BrowserRef
	EvidencePointers                 []string
	CleanMerged, Unknown             bool
}
type ResourceRef struct {
	Project                        string
	Kind                           string
	Run                            core.RunID
	Team                           core.TeamID
	Worktree, Revision, ResourceID string
	Generation                     uint64
}
type StopReceipt struct {
	Project          string
	Resource         ResourceRef
	Evidence, Digest string
}
type CleanupProof struct {
	Project                                string
	Run                                    core.RunID
	Team                                   core.TeamID
	Task                                   core.TaskID
	Worktree, Branch, Base, Revision       string
	ReceiptRevision                        uint64
	ReceiptDigest                          string
	TerminalClean, Integrated, NoConsumers bool
	Resources                              []ResourceRef
}
type CleanupAuthority interface {
	Preflight(context.Context, CleanupCandidate) (CleanupProof, error)
}
type ExactResourceRegistry interface {
	StopExact(context.Context, CleanupProof, []ResourceRef) ([]StopReceipt, error)
	VerifyExact(context.Context, []ResourceRef, []StopReceipt) error
}
type Cleaner interface {
	Cleanup(context.Context, CleanupCandidate) error
}
type CleanupOps interface {
	Stop(context.Context, CleanupCandidate) ([]string, error)
	WriteEvidence(context.Context, []string) error
	Verify(context.Context, []string) error
	RemoveWorktree(context.Context, CleanupCandidate) error
}
type cleaner struct {
	store     *store.Store
	project   string
	authority CleanupAuthority
	resources ExactResourceRegistry
	remover   worktree.ExactWorktreeRemover
}

var cleanupLocks sync.Map

func NewCleaner(s *store.Store, project string, a CleanupAuthority, r ExactResourceRegistry, w worktree.ExactWorktreeRemover) Cleaner {
	return &cleaner{store: s, project: project, authority: a, resources: r, remover: w}
}
func (c *cleaner) Cleanup(ctx context.Context, x CleanupCandidate) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if c == nil || c.store == nil || c.project == "" || c.authority == nil || c.resources == nil || c.remover == nil {
		return core.ErrCapacity
	}
	if err := validateCandidate(x); err != nil {
		return err
	}
	p, err := c.authority.Preflight(ctx, x)
	if err != nil {
		return err
	}
	if err = validateProof(c.project, x, p); err != nil {
		return err
	}
	id := digest(p)
	root, err := project.CanonicalStoreRoot(c.store)
	if err != nil {
		return core.ErrPath
	}
	state := store.New(root, c.store.Limits)
	lock, _ := cleanupLocks.LoadOrStore(root+"\x00"+id, &sync.Mutex{})
	lock.(*sync.Mutex).Lock()
	defer lock.(*sync.Mutex).Unlock()
	p, err = c.authority.Preflight(ctx, x)
	if err != nil || validateProof(c.project, x, p) != nil || digest(p) != id {
		return core.ErrRevision
	}
	path := ".agent-team/cleanup/" + string(x.Run) + "/" + string(x.Team) + "/" + id + ".json"
	var op operation
	err = state.ReadJSON(path, 16<<20, &op)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		op = operation{Identity: id, Proof: p, Stage: "prepared", Revision: 1}
		op.Digest = operationDigest(op)
		if _, err = state.CreateJSON(path, op, 16<<20); err != nil {
			if !errors.Is(err, store.ErrAlreadyExists) {
				return err
			}
			if err = state.ReadJSON(path, 16<<20, &op); err != nil {
				return err
			}
		}
	}
	if !validOperation(op, p, id) {
		return core.ErrRevision
	}
	newStop := false
	if op.Stage == "prepared" {
		if err = advance(&op, "stopping"); err != nil {
			return err
		}
		if _, err = state.WriteJSON(path, op, 16<<20); err != nil {
			return err
		}
		newStop = true
	}
	if op.Stage == "stopping" && !newStop {
		return core.ErrRevision
	}
	if op.Stage == "stopping" {
		rs, e := c.resources.StopExact(ctx, p, p.Resources)
		if e != nil {
			return e
		}
		if !validReceipts(p.Resources, rs) {
			return core.ErrRevision
		}
		op.Receipts = rs
		if err = advance(&op, "stopped"); err != nil {
			return err
		}
		if _, err = state.WriteJSON(path, op, 16<<20); err != nil {
			return err
		}
	}
	if op.Stage == "stopped" {
		if e := c.resources.VerifyExact(ctx, p.Resources, op.Receipts); e != nil {
			return e
		}
		if err = advance(&op, "verified"); err != nil {
			return err
		}
		if _, err = state.WriteJSON(path, op, 16<<20); err != nil {
			return err
		}
	}
	if op.Stage == "verified" {
		if e := c.remover.RemoveExact(ctx, contracts.Worktree{Run: x.Run, Team: x.Team, Path: x.Worktree, Branch: x.Branch, Base: x.Base}); e != nil {
			return e
		}
		if err = advance(&op, "removed"); err != nil {
			return err
		}
		_, err = state.WriteJSON(path, op, 16<<20)
		return err
	}
	return nil
}

type operation struct {
	Identity string
	Proof    CleanupProof
	Receipts []StopReceipt
	Stage    string
	Revision uint64
	Digest   string
}

func operationDigest(op operation) string { op.Digest = ""; return digest(op) }
func advance(op *operation, stage string) error {
	if !validStage(op.Stage, stage) {
		return core.ErrRevision
	}
	op.Stage = stage
	op.Revision++
	op.Digest = operationDigest(*op)
	return nil
}
func validStage(from, to string) bool {
	return (from == "prepared" && to == "stopping") || (from == "stopping" && to == "stopped") || (from == "stopped" && to == "verified") || (from == "verified" && to == "removed")
}
func validOperation(op operation, p CleanupProof, id string) bool {
	if op.Identity != id || op.Digest == "" || op.Digest != operationDigest(op) || digest(op.Proof) != digest(p) {
		return false
	}
	if op.Stage != "prepared" && op.Stage != "stopping" && op.Stage != "stopped" && op.Stage != "verified" && op.Stage != "removed" {
		return false
	}
	if op.Revision == 0 {
		return false
	}
	if op.Stage == "stopped" || op.Stage == "verified" || op.Stage == "removed" {
		return validReceipts(p.Resources, op.Receipts)
	}
	return len(op.Receipts) == 0
}

func validateCandidate(x CleanupCandidate) error {
	if x.Project == "" || x.Unknown || !x.CleanMerged || x.Run == "" || x.Team == "" || x.Task == "" || x.Worktree == "" || x.Branch == "" || x.Base == "" || x.Revision == "" || strings.ContainsAny(x.Worktree+x.Branch, "*?[") || filepath.Clean(x.Worktree) == "." {
		return core.ErrCapacity
	}
	for _, r := range x.Servers {
		if r.Run != x.Run || r.Team != x.Team || r.ResourceID == "" || r.Revision == 0 || r.Ownership != "managed" {
			return core.ErrCapacity
		}
	}
	for _, r := range x.Browsers {
		if r.Run != x.Run || r.Team != x.Team || r.ResourceID == "" || r.Revision == 0 || r.Ownership != "managed" {
			return core.ErrCapacity
		}
	}
	return nil
}
func validateProof(project string, x CleanupCandidate, p CleanupProof) error {
	if p.Project != project || x.Project != project || !p.TerminalClean || !p.Integrated || !p.NoConsumers || p.Run != x.Run || p.Team != x.Team || p.Task != x.Task || p.Worktree != x.Worktree || p.Branch != x.Branch || p.Base != x.Base || p.Revision != x.Revision || p.ReceiptRevision == 0 || p.ReceiptDigest == "" {
		return core.ErrRevision
	}
	for _, r := range p.Resources {
		if r.Project != project || r.Run != x.Run || r.Team != x.Team || r.Worktree != x.Worktree || r.Revision != x.Revision || r.ResourceID == "" || r.Generation == 0 {
			return core.ErrRevision
		}
	}
	return nil
}
func validReceipts(a []ResourceRef, b []StopReceipt) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if b[i].Project != a[i].Project || b[i].Resource != a[i] || b[i].Evidence == "" || b[i].Digest == "" {
			return false
		}
	}
	return true
}
func digest(v any) string {
	b, _ := json.Marshal(v)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
func ExecuteCleanup(ctx context.Context, x CleanupCandidate, o CleanupOps) error {
	if err := validateCandidate(x); err != nil {
		return err
	}
	p, e := o.Stop(ctx, x)
	if e != nil {
		return e
	}
	if len(p) == 0 {
		return core.ErrRevision
	}
	if e = o.WriteEvidence(ctx, p); e != nil {
		return e
	}
	if e = o.Verify(ctx, p); e != nil {
		return e
	}
	return o.RemoveWorktree(ctx, x)
}
