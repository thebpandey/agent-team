package deploy

import "errors"

var (
	ErrNotFound    = errors.New("deployment record not found")
	ErrAction      = errors.New("deployment action invalid")
	ErrProvider    = errors.New("deployment provider failed")
	ErrOutputLimit = errors.New("deployment output limit exceeded")
)
