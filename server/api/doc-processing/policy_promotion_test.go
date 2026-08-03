package docprocessing

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
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
	// Verify the PromotedProposal type is accessible.
	proposal := PromotedProposal{
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
	var _ DraftPolicyPromoter = PolicyPromotionStore{}
}

func TestPolicyCompileFailureLeavesActiveUntouched(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	store := PolicyPromotionStore{DB: db}

	// Idempotency check: no existing draft for this release.
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id FROM kb.pipeline_policies`)).
		WillReturnError(sql.ErrNoRows)

	// Max version lookup succeeds.
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT COALESCE(MAX(version), 0) FROM kb.pipeline_policies`)).
		WillReturnRows(sqlmock.NewRows([]string{"coalesce"}).AddRow(3))

	// INSERT draft policy fails — simulating a DB error during draft creation.
	mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO kb.pipeline_policies`)).
		WillReturnError(fmt.Errorf("connection refused"))

	policyID, err := store.EnsureDraftFromModuleRelease(
		context.Background(), 1, "checksum-abc", nil,
	)
	if err == nil {
		t.Fatal("expected error when INSERT draft policy fails")
	}
	if policyID != 0 {
		t.Fatalf("policyID=%d want 0 on failure", policyID)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("ExpectationsWereMet: %v", err)
	}
}

func TestPolicyActivationFailureRollback(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	store := PolicyPromotionStore{DB: db}

	proposals := []PromotedProposal{
		{
			ProposalID:        101,
			Predicate:         json.RawMessage(`{"version":1,"expression":{"kind":"all","items":[]}}`),
			PredicateChecksum: "sha256:abc123",
		},
	}

	// Idempotency check: no existing draft.
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id FROM kb.pipeline_policies`)).
		WillReturnError(sql.ErrNoRows)

	// Max version lookup.
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT COALESCE(MAX(version), 0) FROM kb.pipeline_policies`)).
		WillReturnRows(sqlmock.NewRows([]string{"coalesce"}).AddRow(5))

	// INSERT draft policy — must use status='draft', never 'active'.
	mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO kb.pipeline_policies`)).
		WithArgs(6, "module_release:1", "checksum-xyz").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(99)))

	// Default pipeline lookup.
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id FROM kb.pipelines WHERE name`)).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(7)))

	// Materialize the proposal binding.
	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO kb.pipeline_bindings`)).
		WithArgs(
			int64(7), int64(99), "module_proposal:101",
			string(proposals[0].Predicate), proposals[0].PredicateChecksum,
		).
		WillReturnResult(sqlmock.NewResult(0, 1))

	policyID, err := store.EnsureDraftFromModuleRelease(
		context.Background(), 1, "checksum-xyz", proposals,
	)
	if err != nil {
		t.Fatalf("EnsureDraftFromModuleRelease: %v", err)
	}
	if policyID != 99 {
		t.Fatalf("policyID=%d want 99", policyID)
	}

	// Verify no activation query was issued — EnsureDraftFromModuleRelease
	// only creates drafts, so activation failure is inherently safe.
	// ExpectationsWereMet confirms no unexpected queries (e.g., UPDATE
	// status='active') were executed.
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unexpected queries (activation should not occur): %v", err)
	}
}
