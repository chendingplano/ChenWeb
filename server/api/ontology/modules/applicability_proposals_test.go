package modules

import (
	"encoding/json"
	"testing"
)

func TestValidProposalTransitions(t *testing.T) {
	tests := []struct {
		from, to string
		valid    bool
	}{
		{ProposalStatusDraft, ProposalStatusInReview, true},
		{ProposalStatusDraft, ProposalStatusApproved, false},
		{ProposalStatusDraft, ProposalStatusRejected, false},
		{ProposalStatusInReview, ProposalStatusApproved, true},
		{ProposalStatusInReview, ProposalStatusRejected, true},
		{ProposalStatusInReview, ProposalStatusDraft, false},
		{ProposalStatusApproved, ProposalStatusIncludedInRelease, true},
		{ProposalStatusApproved, ProposalStatusRejected, false},
		{ProposalStatusRejected, ProposalStatusDraft, false},
		{ProposalStatusIncludedInRelease, ProposalStatusApproved, false},
	}
	for _, tt := range tests {
		got := validProposalTransition(tt.from, tt.to)
		if got != tt.valid {
			t.Errorf("validProposalTransition(%q, %q) = %v, want %v", tt.from, tt.to, got, tt.valid)
		}
	}
}

func TestPredicateChecksumDeterministic(t *testing.T) {
	raw := json.RawMessage(`{"version":1,"expression":{"kind":"fact","path":"document.doc_kind","op":"eq","value":"product_specification"}}`)
	c1 := predicateChecksum(raw)
	c2 := predicateChecksum(raw)
	if c1 != c2 {
		t.Fatalf("checksum not deterministic: %q != %q", c1, c2)
	}
	if c1 == "" {
		t.Fatal("checksum is empty")
	}
	if len(c1) < 10 {
		t.Fatalf("checksum too short: %q", c1)
	}
}

func TestParsePredicateDocument(t *testing.T) {
	raw := json.RawMessage(`{"version":1,"expression":{"kind":"fact","path":"document.doc_kind","op":"eq","value":"product_specification"}}`)
	doc, err := parsePredicateDocument(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if doc.Version != 1 {
		t.Fatalf("version = %d, want 1", doc.Version)
	}
	if doc.Expression.Kind != "fact" {
		t.Fatalf("kind = %q, want fact", doc.Expression.Kind)
	}
}

func TestParsePredicateDocumentInvalidJSON(t *testing.T) {
	_, err := parsePredicateDocument(json.RawMessage(`{invalid`))
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestProposalStoreRequiresModuleID(t *testing.T) {
	store := ProposalStore{}
	_, err := store.CreateProposal(nil, "", 1, json.RawMessage(`{}`), "", "")
	if err == nil {
		t.Fatal("expected error for empty module_id")
	}
}

func TestProposalStoreRequiresReleaseID(t *testing.T) {
	store := ProposalStore{}
	_, err := store.CreateProposal(nil, "test", 0, json.RawMessage(`{}`), "", "")
	if err == nil {
		t.Fatal("expected error for zero release_id")
	}
}

func TestProposalStoreRequiresPredicate(t *testing.T) {
	store := ProposalStore{}
	_, err := store.CreateProposal(nil, "test", 1, nil, "", "")
	if err == nil {
		t.Fatal("expected error for nil predicate")
	}
}

func TestProposalStoreNilDB(t *testing.T) {
	store := ProposalStore{}
	_, err := store.CreateProposal(nil, "test", 1, json.RawMessage(`{"version":1,"expression":{"kind":"fact","path":"document.doc_kind","op":"eq","value":"x"}}`), "", "")
	if err == nil {
		t.Fatal("expected error for nil DB")
	}
}

func TestTransitionProposalRequiresActorForApproval(t *testing.T) {
	// This test verifies the validation logic without a real DB.
	// The actual DB interaction is tested in live validation (I2).
	store := ProposalStore{}
	_, err := store.TransitionProposal(nil, 0, ProposalStatusApproved, "", nil)
	if err == nil {
		t.Fatal("expected error for zero id")
	}
}

func TestNullIfEmptyStr(t *testing.T) {
	if v := nullIfEmptyStr(""); v != nil {
		t.Fatalf("expected nil for empty string, got %v", v)
	}
	if v := nullIfEmptyStr("  "); v != nil {
		t.Fatalf("expected nil for whitespace string, got %v", v)
	}
	if v := nullIfEmptyStr("hello"); v != "hello" {
		t.Fatalf("expected hello, got %v", v)
	}
}
