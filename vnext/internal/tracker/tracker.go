// Package tracker provides the bounded, revisioned adapters for the selected
// project tracker. A run selects exactly one adapter; adapters never merge
// TASKS.md and Beads data.
package tracker

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/thebpandey/agent-team/vnext/internal/core"
)

const (
	capacity     = 1000
	warningAt    = 900
	maxPageLimit = 1000
)

// Tracker is the sole authority selected for a plan-mode run.
type Tracker interface {
	Page(ctx context.Context, cursor string, limit int) (core.TrackerPage, error)
	Get(ctx context.Context, id core.TaskID, expectedTrackerRevision uint64) (core.Task, error)
	Refresh(ctx context.Context, expectedTrackerRevision uint64) (core.TrackerPage, error)
	Create(ctx context.Context, task core.Task, expectedTrackerRevision uint64) (core.Task, error)
	Archive(ctx context.Context, id core.TaskID, reason string, expectedTrackerRevision uint64) error
}

// AuthorityMetadata is the immutable identity of one built-in tracker. It is
// deliberately read-only: run creation may record it, but never configures or
// mutates the tracker.
type AuthorityMetadata struct {
	Kind string
	Ref  string
}

// AuthorityMetadataProvider is implemented by native tracker adapters. A run
// rejects adapters without this identity rather than inventing an authority.
type AuthorityMetadataProvider interface {
	AuthorityMetadata() AuthorityMetadata
}

// AutomaticAdmissionScope narrows automatic task selection without hiding any
// tracker records or changing their status. Explicit user-requested admission
// continues to use the complete selected tracker and its own authorization.
type AutomaticAdmissionScope interface {
	AllowsAutomaticAdmission(core.TaskID) bool
}

func trackerRevision(data []byte) uint64 {
	sum := sha256.Sum256(data)
	revision := binary.BigEndian.Uint64(sum[:8])
	if revision == 0 {
		return 1
	}
	return revision
}

func pageFor(tasks []core.Task, revision uint64, cursor string, limit int) (core.TrackerPage, error) {
	if limit < 1 || limit > maxPageLimit {
		return core.TrackerPage{}, fmt.Errorf("%w: page limit %d", core.ErrLimit, limit)
	}
	offset := 0
	if cursor != "" {
		parsed, err := strconv.Atoi(cursor)
		if err != nil || parsed < 0 {
			return core.TrackerPage{}, fmt.Errorf("%w: invalid cursor %q", core.ErrPath, cursor)
		}
		offset = parsed
	}

	active := make([]core.Task, 0, len(tasks))
	for _, task := range tasks {
		if task.ID == "" {
			return core.TrackerPage{}, fmt.Errorf("%w: task without ID", core.ErrPath)
		}
		if !task.Archived {
			active = append(active, task)
		}
	}
	if len(active) > capacity {
		return core.TrackerPage{}, fmt.Errorf("%w: %d non-archived tasks exceeds %d", core.ErrCapacity, len(active), capacity)
	}
	sort.SliceStable(active, func(i, j int) bool { return active[i].ID < active[j].ID })
	if offset > len(active) {
		return core.TrackerPage{}, fmt.Errorf("%w: cursor %d beyond %d tasks", core.ErrPath, offset, len(active))
	}
	end := offset + limit
	if end > len(active) {
		end = len(active)
	}
	page := core.TrackerPage{
		TrackerRevision:  revision,
		TotalNonArchived: len(active),
		Tasks:            append([]core.Task(nil), active[offset:end]...),
	}
	if end < len(active) {
		page.Cursor = strconv.Itoa(end)
	}
	return page, nil
}

func requireRevision(expected, actual uint64) error {
	if expected != actual {
		return fmt.Errorf("%w: expected tracker revision %d, current %d", core.ErrRevision, expected, actual)
	}
	return nil
}

func validateTask(task core.Task) error {
	if strings.TrimSpace(string(task.ID)) == "" || strings.ContainsAny(string(task.ID), "\t\r\n ") {
		return fmt.Errorf("%w: invalid task ID %q", core.ErrPath, task.ID)
	}
	if strings.TrimSpace(task.Objective) == "" {
		return fmt.Errorf("%w: task %s has no objective", core.ErrPath, task.ID)
	}
	return nil
}

func warningFor(total int) string {
	if total >= warningAt {
		return fmt.Sprintf("tracker has %d of %d non-archived tasks", total, capacity)
	}
	return ""
}
