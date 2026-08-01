// Package assertions implements the qualified-assertion and evidence model
// (ADR 2026072901 DR9; spec 2026072702 §8.3) and the spec §9.3 OPERATIONAL
// semantic-decision state machine. This is a distinct lifecycle from the P2
// governed-content machine on kb.ontology_terms/kb.ontology_candidates:
// assertions are operational data (a source's claim, decided through an
// approved policy), never an ontology release.
package assertions

import "errors"

// AssertionStatus values follow the spec §9.3 operational semantic-decision
// state machine.
const (
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

// assertionTransitions is the spec §9.3 operational machine:
//
//	candidate -> in_review -> accepted
//	                      \-> rejected
//	                      \-> deferred
//	deferred --dependency changed--> candidate
//	accepted --decision-relevant revision--> superseded
//	accepted --last evidence lost--> unsupported
//	unsupported --qualifying evidence restored--> accepted
//
// deferred -> candidate is not listed here because it requires the
// dependency fingerprint to have changed; it is only reachable via
// RetryDeferred (spec §16.3 item 12). rejected and superseded are terminal
// for a specific payload revision; reconsideration creates a new revision.
var assertionTransitions = map[string]map[string]bool{
	StatusCandidate:   {StatusInReview: true, StatusRejected: true, StatusDeferred: true},
	StatusInReview:    {StatusAccepted: true, StatusRejected: true, StatusDeferred: true},
	StatusAccepted:    {StatusSuperseded: true, StatusUnsupported: true},
	StatusUnsupported: {StatusAccepted: true},
	StatusDeferred:    {}, // only via RetryDeferred
	StatusRejected:    {},
	StatusSuperseded:  {},
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
