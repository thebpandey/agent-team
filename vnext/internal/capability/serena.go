package capability

import "fmt"

type SerenaOperation string

const (
	SerenaFindDefinition SerenaOperation = "find-definition"
	SerenaFindReferences SerenaOperation = "find-references"
	SerenaFindCallers    SerenaOperation = "find-callers"
	SerenaSearchSymbols  SerenaOperation = "search-symbols"
)

type SerenaRequest struct {
	Operation                SerenaOperation
	Worktree, Revision, Path string
	Symbol, Query            string
	ReadOnly                 bool
}

type SerenaResponse struct {
	Operation               SerenaOperation
	Result, EvidencePointer string
}

func AllowSerena(consent Consent, request SerenaRequest) error {
	if !consent.Enabled || consent.Name != Serena || consent.Mode != ReadOnlyMCP || !request.ReadOnly {
		return fmt.Errorf("serena requires explicit read-only consent")
	}
	switch request.Operation {
	case SerenaFindDefinition, SerenaFindReferences, SerenaFindCallers, SerenaSearchSymbols:
		return nil
	default:
		return fmt.Errorf("serena operation is not allowlisted")
	}
}
