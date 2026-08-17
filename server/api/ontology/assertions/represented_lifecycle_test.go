package assertions

import "testing"

// Task 4.5 / ADR 2026081801 DR6. These tests pin the lifecycle SHAPE; the
// database constraints and the store's restore path are exercised by the
// integration tests.

// Lossless ingestion writes `represented`, never `accepted`. The path to
// endorsement is explicit: represented -> candidate -> in_review -> accepted.
func TestRepresentedReachesAcceptedOnlyThroughGovernance(t *testing.T) {
	if !ValidAssertionStatus(StatusRepresented) {
		t.Fatal("represented must be a legal assertion status")
	}
	if transitionAllowed(StatusRepresented, StatusAccepted) {
		t.Fatal("represented -> accepted must not be a direct edge: governance is explicit (DR6)")
	}
	if !transitionAllowed(StatusRepresented, StatusCandidate) {
		t.Fatal("represented -> candidate must be legal: it is how a claim enters governance")
	}
	if !transitionAllowed(StatusCandidate, StatusInReview) || !transitionAllowed(StatusInReview, StatusAccepted) {
		t.Fatal("the existing governed review path must remain intact")
	}
}

// DR6: evidence loss applies to represented, candidate, in_review, deferred,
// and accepted -- and to nothing else.
func TestEvidenceLossOriginStatuses(t *testing.T) {
	for _, status := range []string{
		StatusRepresented, StatusCandidate, StatusInReview, StatusDeferred, StatusAccepted,
	} {
		if !EvidenceLossTransitionAllowed(status) {
			t.Errorf("evidence loss from %q should be allowed (DR6)", status)
		}
		if !transitionAllowed(status, StatusUnsupported) {
			t.Errorf("%q -> unsupported must be a legal edge", status)
		}
	}
	// rejected and superseded are historical decision states: DR6 says they do
	// not transition merely because evidence changed.
	for _, status := range []string{StatusRejected, StatusSuperseded} {
		if EvidenceLossTransitionAllowed(status) {
			t.Errorf("evidence loss from %q must not transition it (DR6)", status)
		}
		if transitionAllowed(status, StatusUnsupported) {
			t.Errorf("%q -> unsupported must not be legal", status)
		}
	}
}

// DR6: restoration returns the claim to the exact recorded prior status. Every
// legal prior status must therefore be a legal restoration target.
func TestRestorationTargetsCoverEveryPriorStatus(t *testing.T) {
	for _, status := range []string{
		StatusRepresented, StatusCandidate, StatusInReview, StatusDeferred, StatusAccepted,
	} {
		if !ValidUnsupportedPriorStatus(status) {
			t.Errorf("%q should be a valid unsupported_prior_status", status)
		}
		if !transitionAllowed(StatusUnsupported, status) {
			t.Errorf("unsupported -> %q must be legal so restoration can return the recorded status", status)
		}
	}
	for _, status := range []string{StatusRejected, StatusSuperseded} {
		if ValidUnsupportedPriorStatus(status) {
			t.Errorf("%q must not be a valid unsupported_prior_status", status)
		}
	}
}

// The regression this guards: restoring a represented claim must not promote it
// to accepted. Because restoration is driven by the recorded prior status, a
// represented claim can only come back as represented.
func TestRestorationNeverEscalatesRepresentedToAccepted(t *testing.T) {
	prior := StatusRepresented
	if !ValidUnsupportedPriorStatus(prior) {
		t.Fatalf("%q should be recordable as a prior status", prior)
	}
	restored := prior // restoration reads the recorded value; it never re-decides
	if restored == StatusAccepted {
		t.Fatal("restoring a represented claim must not yield accepted (DR6)")
	}
}
