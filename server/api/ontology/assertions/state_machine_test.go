package assertions

import "testing"

func TestAssertionTransitionAllowed(t *testing.T) {
	cases := []struct {
		from, to string
		want     bool
	}{
		{StatusCandidate, StatusInReview, true},
		{StatusCandidate, StatusRejected, true},
		{StatusCandidate, StatusDeferred, true},
		{StatusCandidate, StatusAccepted, false}, // must pass through in_review
		{StatusInReview, StatusAccepted, true},
		{StatusInReview, StatusRejected, true},
		{StatusAccepted, StatusSuperseded, true},
		{StatusAccepted, StatusUnsupported, true},
		{StatusAccepted, StatusCandidate, false}, // no path back except via unsupported
		{StatusUnsupported, StatusAccepted, true},
		{StatusUnsupported, StatusSuperseded, false},
		{StatusDeferred, StatusCandidate, false}, // only via RetryDeferred
		{StatusRejected, StatusCandidate, false}, // terminal; reconsideration needs a new revision
		{StatusSuperseded, StatusAccepted, false},
	}
	for _, c := range cases {
		if got := transitionAllowed(c.from, c.to); got != c.want {
			t.Errorf("transitionAllowed(%s, %s) = %v, want %v", c.from, c.to, got, c.want)
		}
	}
}

func TestValidAssertionStatus(t *testing.T) {
	for _, s := range []string{StatusCandidate, StatusInReview, StatusAccepted, StatusRejected, StatusDeferred, StatusSuperseded, StatusUnsupported} {
		if !ValidAssertionStatus(s) {
			t.Errorf("expected %s to be a valid status", s)
		}
	}
	if ValidAssertionStatus("bogus") {
		t.Error("expected bogus to be invalid")
	}
}
