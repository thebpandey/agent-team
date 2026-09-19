package core

import "errors"

var (
	ErrPath       = errors.New("path")
	ErrGit        = errors.New("git")
	ErrLimit      = errors.New("limit")
	ErrCapacity   = errors.New("capacity")
	ErrBatch      = errors.New("batch")
	ErrRevision   = errors.New("revision")
	ErrTransition = errors.New("transition")
	ErrSettings   = errors.New("settings")
	ErrPhase      = errors.New("phase")
)
