package assertions

import (
	"context"
	"strings"
	"testing"
)

func TestDecisionCandidateOrder(t *testing.T) {
	for _, key := range []string{"identity", "kind", "method", "source", "confidence", "status", "resolution", "modified"} {
		order := decisionCandidateOrder(key, "asc")
		if !strings.Contains(order, " ASC NULLS LAST, id ASC") {
			t.Errorf("decisionCandidateOrder(%q, asc) = %q, want ascending nulls-last order with id tie-breaker", key, order)
		}
	}
	if got := decisionCandidateOrder("confidence", "desc"); !strings.Contains(got, "confidence DESC NULLS LAST, id DESC") {
		t.Fatalf("descending confidence order = %q", got)
	}
	if got := decisionCandidateOrder("not-allowed", "desc"); got != "ORDER BY id DESC" {
		t.Fatalf("unsupported sort order = %q, want default order", got)
	}
	if got := decisionCandidateOrder("status", "sideways"); !strings.Contains(got, "status ASC NULLS LAST, id ASC") {
		t.Fatalf("invalid direction order = %q, want ascending order", got)
	}
}

func TestDecisionCandidateTransitionAllowed(t *testing.T) {
	cases := []struct {
		from, to string
		want     bool
	}{
		{StatusCandidate, StatusInReview, true},
		{StatusInReview, StatusAccepted, true},
		{StatusAccepted, StatusSuperseded, true},
		{StatusAccepted, StatusUnsupported, false}, // assertion-only arc, not valid for candidates
		{StatusDeferred, StatusCandidate, false},   // only via RetryDeferred
	}
	for _, c := range cases {
		if got := decisionCandidateTransitionAllowed(c.from, c.to); got != c.want {
			t.Errorf("decisionCandidateTransitionAllowed(%s, %s) = %v, want %v", c.from, c.to, got, c.want)
		}
	}
}

func TestPayloadFingerprintIsDeterministicAndOrderIndependent(t *testing.T) {
	fp1, err := PayloadFingerprint([]byte(`{"a":1,"b":2}`))
	if err != nil {
		t.Fatalf("PayloadFingerprint: %v", err)
	}
	fp2, err := PayloadFingerprint([]byte(`{"b":2,"a":1}`))
	if err != nil {
		t.Fatalf("PayloadFingerprint: %v", err)
	}
	if fp1 != fp2 {
		t.Fatalf("expected key-order-independent fingerprints to match: %s != %s", fp1, fp2)
	}

	fp3, err := PayloadFingerprint([]byte(`{"a":1,"b":3}`))
	if err != nil {
		t.Fatalf("PayloadFingerprint: %v", err)
	}
	if fp1 == fp3 {
		t.Fatal("expected different payloads to produce different fingerprints")
	}
}

func TestDecisionCandidateStoreProposeRejectsUnsupportedKind(t *testing.T) {
	store := DecisionCandidateStore{DB: nil}
	_, err := store.Propose(context.Background(), DecisionCandidate{
		LogicalIdentityKey: "x",
		CandidateKind:      "widget",
		Method:             "human",
		ProposedPayload:    []byte(`{}`),
	})
	if err == nil {
		t.Fatal("expected error for unsupported candidate_kind")
	}
}

func TestDecisionCandidateStoreProposeRejectsUnsupportedMethod(t *testing.T) {
	store := DecisionCandidateStore{DB: nil}
	_, err := store.Propose(context.Background(), DecisionCandidate{
		LogicalIdentityKey: "x",
		CandidateKind:      "referent",
		Method:             "psychic",
		ProposedPayload:    []byte(`{}`),
	})
	if err == nil {
		t.Fatal("expected error for unsupported method")
	}
}

func TestDecisionCandidateStoreProposeRequiresPayload(t *testing.T) {
	store := DecisionCandidateStore{DB: nil}
	_, err := store.Propose(context.Background(), DecisionCandidate{
		LogicalIdentityKey: "x",
		CandidateKind:      "referent",
		Method:             "human",
	})
	if err == nil {
		t.Fatal("expected error for empty proposed_payload")
	}
}

func TestDecisionCandidateStoreDeferRequiresDependencyFingerprint(t *testing.T) {
	store := DecisionCandidateStore{DB: nil}
	_, err := store.DeferCandidate(context.Background(), 1, "", "reason", "tester")
	if err == nil {
		t.Fatal("expected error when dependency fingerprint is empty")
	}
}
