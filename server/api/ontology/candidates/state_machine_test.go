package candidates

import "testing"

func TestTransitionAllowedEnforcesSpec93Machine(t *testing.T) {
	allowed := [][2]string{
		{StatusDiscovered, StatusDraft},
		{StatusDiscovered, StatusRejected},
		{StatusDiscovered, StatusDeferred},
		{StatusDraft, StatusInReview},
		{StatusDraft, StatusRejected},
		{StatusDraft, StatusDeferred},
		{StatusInReview, StatusApproved},
		{StatusInReview, StatusRejected},
		{StatusInReview, StatusDeferred},
		{StatusApproved, StatusIncludedInRelease},
		{StatusApproved, StatusSuperseded},
		{StatusIncludedInRelease, StatusSuperseded},
	}
	for _, pair := range allowed {
		if !transitionAllowed(pair[0], pair[1]) {
			t.Fatalf("expected allowed transition %s -> %s", pair[0], pair[1])
		}
	}

	illegal := [][2]string{
		{StatusDiscovered, StatusApproved},          // must pass through draft/in_review
		{StatusDraft, StatusApproved},               // skip in_review
		{StatusApproved, StatusDraft},               // no going back
		{StatusRejected, StatusDraft},               // terminal
		{StatusSuperseded, StatusApproved},          // terminal
		{StatusDeferred, StatusDraft},               // only via RetryDeferred with a changed fingerprint
		{StatusDeferred, StatusApproved},            // no direct path
		{StatusInReview, StatusInReview},            // no self-loops
		{StatusIncludedInRelease, StatusApproved},   // no re-activation
	}
	for _, pair := range illegal {
		if transitionAllowed(pair[0], pair[1]) {
			t.Fatalf("expected illegal transition %s -> %s", pair[0], pair[1])
		}
	}
}

func TestValidateCandidateStatus(t *testing.T) {
	for _, s := range []string{
		StatusDiscovered, StatusDraft, StatusInReview, StatusApproved,
		StatusIncludedInRelease, StatusRejected, StatusDeferred, StatusSuperseded,
	} {
		if !ValidateCandidateStatus(s) {
			t.Fatalf("expected valid status %q", s)
		}
	}
	for _, s := range []string{"active", "", "published"} {
		if ValidateCandidateStatus(s) {
			t.Fatalf("expected invalid status %q", s)
		}
	}
}
