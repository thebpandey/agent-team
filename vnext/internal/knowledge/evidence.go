package knowledge

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"

	"github.com/thebpandey/agent-team/vnext/internal/core"
	"github.com/thebpandey/agent-team/vnext/internal/store"
)

// Evidence is immutable attempt metadata. Large command output stays at its
// validated pointer; this record intentionally stores no command payload.
type Evidence struct {
	core.RecordEnvelope
	Task             string `json:"task"`
	Attempt          int    `json:"attempt"`
	Exit             int    `json:"exit"`
	TimedOut         bool   `json:"timedOut,omitempty"`
	Transport        string `json:"transport,omitempty"`
	InputFingerprint string `json:"inputFingerprint"`
	CommandPointer   string `json:"commandPointer,omitempty"`
	OutputPointer    string `json:"outputPointer,omitempty"`
	Summary          string `json:"summary,omitempty"`
}

func evidencePath(task string, attempt int) (string, error) {
	if err := validateSegment(task); err != nil {
		return "", err
	}
	if attempt < 1 {
		return "", fmt.Errorf("%w: attempt must be positive", core.ErrRevision)
	}
	return ".agent-team/evidence/" + task + "/" + strconv.Itoa(attempt) + "/evidence.json", nil
}

func validateEvidence(e Evidence) error {
	if err := validateEnvelope(e.RecordEnvelope); err != nil {
		return err
	}
	if _, err := evidencePath(e.Task, e.Attempt); err != nil {
		return err
	}
	if e.InputFingerprint == "" || len(e.InputFingerprint) > 256 {
		return fmt.Errorf("%w: evidence input fingerprint is required and bounded", core.ErrLimit)
	}
	for _, pointer := range []string{e.CommandPointer, e.OutputPointer} {
		if pointer != "" {
			if err := validatePointer(pointer); err != nil {
				return err
			}
		}
	}
	for name, value := range map[string]string{"transport": e.Transport, "summary": e.Summary} {
		if err := validateText(value, name); err != nil {
			return err
		}
	}
	return nil
}

// WriteEvidence writes an evidence record once. Equal retries are idempotent;
// differing attempts are rejected without changing the original file.
func WriteEvidence(ctx context.Context, s *store.Store, evidence Evidence) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if s == nil {
		return fmt.Errorf("%w: nil store", core.ErrPath)
	}
	if err := validateEvidence(evidence); err != nil {
		return err
	}
	path, err := evidencePath(evidence.Task, evidence.Attempt)
	if err != nil {
		return err
	}
	lock := knowledgeLock(s.Root)
	lock.Lock()
	defer lock.Unlock()
	var previous Evidence
	err = s.ReadJSON(path, canonicalLimit(s), &previous)
	if err == nil {
		equal, compareErr := sameJSON(previous, evidence)
		if compareErr != nil {
			return compareErr
		}
		if equal {
			return nil
		}
		return fmt.Errorf("%w: evidence %s/%d already exists", ErrConflict, evidence.Task, evidence.Attempt)
	}
	if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	_, err = s.WriteJSON(path, evidence, canonicalLimit(s))
	return err
}
