// Package candidates implements the governed ontology-candidate lifecycle
// (spec 2026072702 §9.3): proposals for terms, labels, mappings, and axioms
// land in kb.ontology_candidates, and nothing reaches the content tables
// except through this state machine. This is the code-enforced form of the
// "no LLM activates ontology content" guarantee (ADR 2026072701 DR6).
package candidates

import "errors"

// errNoRetryWithoutDependencyChange is returned by RetryDeferred when a
// deferred candidate is not eligible to retry because its dependency
// fingerprint has not changed (spec §9.3, §16.3 item 12).
var errNoRetryWithoutDependencyChange = errors.New("dependency fingerprint unchanged; deferred candidate is not eligible for retry")

// CandidateStatus values follow the governed ontology-content state machine.
const (
	StatusDiscovered        = "discovered"
	StatusDraft             = "draft"
	StatusInReview          = "in_review"
	StatusApproved          = "approved"
	StatusIncludedInRelease = "included_in_release"
	StatusRejected          = "rejected"
	StatusDeferred          = "deferred"
	StatusSuperseded        = "superseded"
)

// candidateTransitions is the spec §9.3 governed ontology-content machine.
//
//	discovered -> draft -> in_review -> approved -> included_in_release
//	                    \-> rejected                    \-> superseded
//	                    \-> deferred
//	deferred --dependency changed--> draft
//
// deferred -> draft is NOT listed here because it requires the dependency
// fingerprint to have changed; it is only reachable via RetryDeferred, which
// verifies that. rejected and superseded are terminal for the candidate
// revision; reconsideration creates a new candidate.
var candidateTransitions = map[string]map[string]bool{
	StatusDiscovered:        {StatusDraft: true, StatusRejected: true, StatusDeferred: true},
	StatusDraft:             {StatusInReview: true, StatusRejected: true, StatusDeferred: true},
	StatusInReview:          {StatusApproved: true, StatusRejected: true, StatusDeferred: true},
	StatusApproved:          {StatusIncludedInRelease: true, StatusSuperseded: true},
	StatusIncludedInRelease: {StatusSuperseded: true},
	StatusDeferred:          {}, // only via RetryDeferred
	StatusRejected:          {},
	StatusSuperseded:        {},
}

// transitionAllowed reports whether the governed machine permits from -> to.
func transitionAllowed(from, to string) bool {
	return candidateTransitions[from][to]
}

// ValidateCandidateStatus reports whether a status value is a legal candidate
// status.
func ValidateCandidateStatus(status string) bool {
	_, ok := candidateTransitions[status]
	return ok
}
