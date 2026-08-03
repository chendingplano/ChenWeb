package docprocessing

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"

	"github.com/chendingplano/deepdoc/server/api/ontology/modules"
)

func TestPolicyPromotionStoreNilDB(t *testing.T) {
	store := PolicyPromotionStore{}
	_, err := store.EnsureDraftFromModuleRelease(context.Background(), 1, "checksum", nil)
	if err == nil {
		t.Fatal("expected error for nil DB")
	}
}

func TestPolicyPromotionStoreRequiresReleaseID(t *testing.T) {
	store := PolicyPromotionStore{DB: &sql.DB{}}
	_, err := store.EnsureDraftFromModuleRelease(context.Background(), 0, "checksum", nil)
	if err == nil {
		t.Fatal("expected error for zero release_id")
	}
}

func TestPolicyPromotionStoreRequiresChecksum(t *testing.T) {
	store := PolicyPromotionStore{DB: &sql.DB{}}
	_, err := store.EnsureDraftFromModuleRelease(context.Background(), 1, "", nil)
	if err == nil {
		t.Fatal("expected error for empty checksum")
	}
}

func TestPromoteModuleReleaseProposalsNilPromoter(t *testing.T) {
	policyID, err := PromoteModuleReleaseProposals(context.Background(), nil, nil, 1, "checksum")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if policyID != 0 {
		t.Fatalf("expected 0 policy_id for nil promoter, got %d", policyID)
	}
}

func TestPromotedProposalShape(t *testing.T) {
	// Verify the PromotedProposal type is accessible from modules package.
	proposal := modules.PromotedProposal{
		ProposalID:        1,
		Predicate:         json.RawMessage(`{"version":1}`),
		PredicateChecksum: "sha256:abc",
	}
	if proposal.ProposalID != 1 {
		t.Fatalf("expected ProposalID 1, got %d", proposal.ProposalID)
	}
	if len(proposal.Predicate) == 0 {
		t.Fatal("expected non-empty Predicate")
	}
	if proposal.PredicateChecksum == "" {
		t.Fatal("expected non-empty PredicateChecksum")
	}
}

func TestPolicyPromotionStoreImplementsInterface(t *testing.T) {
	// Compile-time check that PolicyPromotionStore satisfies DraftPolicyPromoter.
	var _ modules.DraftPolicyPromoter = PolicyPromotionStore{}
}
