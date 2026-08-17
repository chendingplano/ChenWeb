// Package assertions implements the qualified-assertion and evidence model
// (ADR 2026072901 DR9; spec 2026072702 §8.3) and the spec §9.3 OPERATIONAL
// semantic-decision state machine. This is a distinct lifecycle from the P2
// governed-content machine on kb.ontology_terms/kb.ontology_candidates:
// assertions are operational data (a source's claim, decided through an
// approved policy), never an ontology release.
package assertions

import "errors"

// AssertionStatus values follow the spec §9.3 operational semantic-decision
// state machine, extended by ADR 2026081801 DR6 with StatusRepresented.
const (
	// StatusRepresented means "durably persisted from source, not endorsed"
	// (ADR 2026081801 DR6). Lossless ingestion writes this and never writes
	// StatusAccepted merely because parsing or normalization succeeded --
	// admission into kb.semantic_assertions establishes source fidelity, not
	// truth or conformance.
	StatusRepresented = "represented"
	StatusCandidate   = "candidate"
	StatusInReview    = "in_review"
	StatusAccepted    = "accepted"
	StatusRejected    = "rejected"
	StatusDeferred    = "deferred"
	StatusSuperseded  = "superseded"
	StatusUnsupported = "unsupported"
)

// errIllegalAssertionTransition is returned when a status change does not
// follow the spec §9.3 operational machine.
var errIllegalAssertionTransition = errors.New("illegal assertion status transition")

// assertionTransitions is the spec §9.3 operational machine as revised by ADR
// 2026081801 DR6:
//
//	represented --selected for governance--> candidate
//	candidate -> in_review -> accepted
//	                      \-> rejected
//	                      \-> deferred
//	deferred --dependency changed--> candidate
//	represented/accepted --decision-relevant replacement--> superseded
//	represented/candidate/in_review/deferred/accepted
//	    --last supporting evidence lost--> unsupported
//	unsupported --support restored--> unsupported_prior_status
//
// deferred -> candidate is not listed here because it requires the
// dependency fingerprint to have changed; it is only reachable via
// RetryDeferred (spec §16.3 item 12). rejected and superseded are terminal
// for a specific payload revision; reconsideration creates a new revision, and
// DR6 is explicit that neither transitions merely because evidence changed.
//
// The unsupported -> * edges are the restoration targets: DR6 requires
// restoration to return the claim to the exact status recorded in
// unsupported_prior_status, so every legal prior status is a legal target.
// RestoreFromUnsupported, not a bare TransitionStatus call, is what enforces
// that the target equals the recorded prior status -- these edges only make
// the shape legal.
var assertionTransitions = map[string]map[string]bool{
	StatusRepresented: {StatusCandidate: true, StatusSuperseded: true, StatusUnsupported: true},
	StatusCandidate:   {StatusInReview: true, StatusRejected: true, StatusDeferred: true, StatusUnsupported: true},
	StatusInReview:    {StatusAccepted: true, StatusRejected: true, StatusDeferred: true, StatusUnsupported: true},
	StatusAccepted:    {StatusSuperseded: true, StatusUnsupported: true},
	StatusUnsupported: {
		StatusRepresented: true,
		StatusCandidate:   true,
		StatusInReview:    true,
		StatusDeferred:    true,
		StatusAccepted:    true,
	},
	StatusDeferred:   {StatusUnsupported: true}, // otherwise only via RetryDeferred
	StatusRejected:   {},
	StatusSuperseded: {},
}

// evidenceLossPriorStatuses are the statuses DR6 requires to be recorded in
// unsupported_prior_status when the last supporting evidence link is removed.
// rejected and superseded are absent by design: they are historical decision
// states that do not transition merely because evidence changes.
var evidenceLossPriorStatuses = map[string]bool{
	StatusRepresented: true,
	StatusCandidate:   true,
	StatusInReview:    true,
	StatusDeferred:    true,
	StatusAccepted:    true,
}

func transitionAllowed(from, to string) bool {
	return assertionTransitions[from][to]
}

// ValidAssertionStatus reports whether a status value is a legal assertion
// status.
func ValidAssertionStatus(status string) bool {
	_, ok := assertionTransitions[status]
	return ok
}

// EvidenceLossTransitionAllowed reports whether losing the last supporting
// evidence link may move an assertion in this status to 'unsupported' with its
// prior status recorded (ADR 2026081801 DR6).
func EvidenceLossTransitionAllowed(from string) bool {
	return evidenceLossPriorStatuses[from]
}

// ValidUnsupportedPriorStatus mirrors the database constraint added by
// migration 20260818000006: unsupported_prior_status is restricted to the
// statuses evidence loss can originate from.
func ValidUnsupportedPriorStatus(status string) bool {
	return evidenceLossPriorStatuses[status]
}
