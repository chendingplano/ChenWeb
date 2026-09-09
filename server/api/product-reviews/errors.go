package productreviews

import "fmt"

// Error codes for the Product Metric Reviewer (openspec change
// product-metric-reviewer). The CWB_KB_PMR_0xx range is reserved for this app.
//
//	010 — a profile edit would introduce a cycle in the node tree
//	011 — the decomposition proposal LLM call failed or returned unparseable output
//	020 — a run requested an artifact type that is not registered
//	030 — a run was requested against a profile whose status is still `draft`
const (
	CodeCycle            = "CWB_KB_PMR_010"
	CodeProposalFailed   = "CWB_KB_PMR_011"
	CodeUnregisteredType = "CWB_KB_PMR_020"
	CodeDraftProfile     = "CWB_KB_PMR_030"
)

// PMRError is a coded error the handler layer maps to an HTTP response. The Code
// is one of the CWB_KB_PMR_0xx constants; Msg is a human-readable explanation.
type PMRError struct {
	Code string
	Msg  string
}

func (e *PMRError) Error() string { return fmt.Sprintf("%s: %s", e.Code, e.Msg) }

func codedError(code, format string, args ...any) *PMRError {
	return &PMRError{Code: code, Msg: fmt.Sprintf(format, args...)}
}

// ErrCycle reports an attempted edit that would make a node its own ancestor.
func ErrCycle(nodeID int64) *PMRError {
	return codedError(CodeCycle, "node %d cannot become its own ancestor", nodeID)
}

// ErrProposalFailed wraps an LLM decomposition failure; the profile is untouched.
func ErrProposalFailed(cause error) *PMRError {
	return codedError(CodeProposalFailed, "decomposition proposal failed: %v", cause)
}

// ErrUnregisteredType reports a run that named an artifact type with no adapter.
func ErrUnregisteredType(artifactType string) *PMRError {
	return codedError(CodeUnregisteredType, "artifact type %q is not registered", artifactType)
}

// ErrDraftProfile reports a run requested against a profile still in `draft`.
func ErrDraftProfile(profileID int64) *PMRError {
	return codedError(CodeDraftProfile, "profile %d is still draft; mark it ready before running", profileID)
}
