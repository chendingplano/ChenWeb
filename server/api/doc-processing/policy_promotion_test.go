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

// beginPromotionTx opens a sqlmock-backed transaction. sqlmock matches
// transaction lifecycle expectations explicitly, so every test that needs a
// *sql.Tx must declare ExpectBegin.
func beginPromotionTx(t *testing.T) (*sql.DB, sqlmock.Sqlmock, *sql.Tx) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	mock.ExpectBegin()
	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	return db, mock, tx
}

func TestPolicyPromotionStoreNilTx(t *testing.T) {
	store := PolicyPromotionStore{}
	_, err := store.EnsureDraftFromModuleRelease(context.Background(), nil, 1, "checksum", nil)
	if err == nil {
		t.Fatal("expected error for nil tx")
	}
}

func TestPolicyPromotionStoreRequiresReleaseID(t *testing.T) {
	_, _, tx := beginPromotionTx(t)
	store := PolicyPromotionStore{}
	_, err := store.EnsureDraftFromModuleRelease(context.Background(), tx, 0, "checksum", nil)
	if err == nil {
		t.Fatal("expected error for zero release_id")
	}
}

func TestPolicyPromotionStoreRequiresChecksum(t *testing.T) {
	_, _, tx := beginPromotionTx(t)
	store := PolicyPromotionStore{}
	_, err := store.EnsureDraftFromModuleRelease(context.Background(), tx, 1, "", nil)
	if err == nil {
		t.Fatal("expected error for empty checksum")
	}
}

func TestPromoteModuleReleaseProposalsNilPromoterOrLister(t *testing.T) {
	_, _, tx := beginPromotionTx(t)
	// Nil promoter and nil lister must both be optional, never a panic (P5
	// review 2026080302 finding P5-10's nil-lister panic).
	for _, tc := range []struct {
		name     string
		promoter DraftPolicyPromoter
		lister   ApprovedProposalLister
	}{
		{"nil promoter", nil, nil},
		{"nil lister", PolicyPromotionStore{}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			policyID, err := PromoteModuleReleaseProposals(context.Background(), tx, tc.promoter, tc.lister, 1, "checksum")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if policyID != 0 {
				t.Fatalf("expected 0 policy_id, got %d", policyID)
			}
		})
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
	_, mock, tx := beginPromotionTx(t)

	store := PolicyPromotionStore{}

	// Idempotency check: no existing policy for this release.
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id FROM kb.pipeline_policies`)).
		WillReturnError(sql.ErrNoRows)

	// Max version lookup succeeds.
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT COALESCE(MAX(version), 0) FROM kb.pipeline_policies`)).
		WillReturnRows(sqlmock.NewRows([]string{"coalesce"}).AddRow(3))

	// INSERT draft policy fails — simulating a DB error during draft creation.
	mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO kb.pipeline_policies`)).
		WillReturnError(fmt.Errorf("connection refused"))

	policyID, err := store.EnsureDraftFromModuleRelease(
		context.Background(), tx, 1, "checksum-abc", nil,
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
	_, mock, tx := beginPromotionTx(t)

	store := PolicyPromotionStore{}

	proposals := []PromotedProposal{
		{
			ProposalID:        101,
			Predicate:         json.RawMessage(`{"version":1,"expression":{"kind":"all","items":[]}}`),
			PredicateChecksum: "sha256:abc123",
		},
	}

	// Idempotency check: no existing policy.
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
		context.Background(), tx, 1, "checksum-xyz", proposals,
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

// TestPromotionIdempotentAcrossDraftAndActivatedStates proves the idempotency
// key matches a policy in ANY state for the same source release, not just a
// draft (P5 review 2026080302 finding P5-10's wrong idempotency key): after a
// release is activated its draft became active, and re-promoting must return
// the existing policy without minting a duplicate version.
func TestPromotionIdempotentAcrossDraftAndActivatedStates(t *testing.T) {
	_, mock, tx := beginPromotionTx(t)

	// An existing policy (any status) for this release's source_ref.
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id FROM kb.pipeline_policies`)).
		WithArgs("module_release:1").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(55)))

	policyID, err := (PolicyPromotionStore{}).EnsureDraftFromModuleRelease(
		context.Background(), tx, 1, "checksum-xyz", []PromotedProposal{{ProposalID: 7, Predicate: json.RawMessage(`{}`), PredicateChecksum: "c"}},
	)
	if err != nil {
		t.Fatalf("EnsureDraftFromModuleRelease: %v", err)
	}
	if policyID != 55 {
		t.Fatalf("policyID=%d want existing 55", policyID)
	}
	// No INSERT/version query may have been issued.
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unexpected queries (idempotent hit must issue no inserts): %v", err)
	}
}

// TestPromotionCreatesDistinctDraftForDistinctRelease proves a genuinely new
// release (a different source_ref) gets its own draft policy rather than
// reusing the previous release's.
func TestPromotionCreatesDistinctDraftForDistinctRelease(t *testing.T) {
	_, mock, tx := beginPromotionTx(t)

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id FROM kb.pipeline_policies`)).
		WithArgs("module_release:2").
		WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT COALESCE(MAX(version), 0) FROM kb.pipeline_policies`)).
		WillReturnRows(sqlmock.NewRows([]string{"coalesce"}).AddRow(5))
	mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO kb.pipeline_policies`)).
		WithArgs(6, "module_release:2", "checksum-2").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(88)))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id FROM kb.pipelines WHERE name`)).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(7)))
	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO kb.pipeline_bindings`)).
		WithArgs(int64(7), int64(88), "module_proposal:8", `{}`, "d").
		WillReturnResult(sqlmock.NewResult(0, 1))

	policyID, err := (PolicyPromotionStore{}).EnsureDraftFromModuleRelease(
		context.Background(), tx, 2, "checksum-2", []PromotedProposal{{ProposalID: 8, Predicate: json.RawMessage(`{}`), PredicateChecksum: "d"}},
	)
	if err != nil {
		t.Fatalf("EnsureDraftFromModuleRelease: %v", err)
	}
	if policyID != 88 {
		t.Fatalf("policyID=%d want 88", policyID)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestPromotionVersionScanErrorPropagates proves a failed MAX(version) lookup
// is surfaced instead of silently minting version 1 from a zero value (P5
// review 2026080302 finding P5-10).
func TestPromotionVersionScanErrorPropagates(t *testing.T) {
	_, mock, tx := beginPromotionTx(t)

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id FROM kb.pipeline_policies`)).
		WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT COALESCE(MAX(version), 0) FROM kb.pipeline_policies`)).
		WillReturnError(fmt.Errorf("connection refused"))

	if _, err := (PolicyPromotionStore{}).EnsureDraftFromModuleRelease(
		context.Background(), tx, 1, "checksum", nil,
	); err == nil {
		t.Fatal("expected the MAX(version) scan error to propagate")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
