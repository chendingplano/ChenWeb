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

// TestEnsureDraftBindingInsertFailurePropagates proves a failure to INSERT
// the conditional binding row is surfaced to the caller (the release
// transaction then rolls back in production). ADR 2026081001 DR3 retired
// the separate kb.pipeline_policies draft row this test originally
// exercised; promotion now materializes inactive conditional bindings
// directly (see policy_promotion.go's EnsureDraftFromModuleRelease).
func TestEnsureDraftBindingInsertFailurePropagates(t *testing.T) {
	_, mock, tx := beginPromotionTx(t)

	store := PolicyPromotionStore{}
	proposals := []PromotedProposal{{ProposalID: 101, Predicate: json.RawMessage(`{}`), PredicateChecksum: "sha256:abc123"}}

	// Idempotency check: no existing binding for this release.
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id FROM kb.pipeline_bindings`)).
		WithArgs("module_release:1").
		WillReturnError(sql.ErrNoRows)

	// Default pipeline lookup succeeds.
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, version FROM kb.pipelines WHERE name = $1 AND status = 'active' LIMIT 1`)).
		WithArgs(DefaultProductionPipelineName).
		WillReturnRows(sqlmock.NewRows([]string{"id", "version"}).AddRow(int64(7), int64(2)))

	// INSERT the conditional binding fails — simulating a DB error.
	mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO kb.pipeline_bindings`)).
		WillReturnError(fmt.Errorf("connection refused"))

	bindingID, err := store.EnsureDraftFromModuleRelease(
		context.Background(), tx, 1, "checksum-abc", proposals,
	)
	if err == nil {
		t.Fatal("expected error when INSERT conditional binding fails")
	}
	if bindingID != 0 {
		t.Fatalf("bindingID=%d want 0 on failure", bindingID)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("ExpectationsWereMet: %v", err)
	}
}

// TestEnsureDraftBindingCreatesInactiveBindingWithoutActivating proves the
// success path: EnsureDraftFromModuleRelease materializes the proposal as an
// inactive (active=false) conditional binding and never issues an activation
// query (spec 2026080102 section 8: "It never activates routing"). ADR
// 2026081001 DR3: active=false is what "draft, not yet live" means now that
// kb.pipeline_policies is retired.
func TestEnsureDraftBindingCreatesInactiveBindingWithoutActivating(t *testing.T) {
	_, mock, tx := beginPromotionTx(t)

	store := PolicyPromotionStore{}

	proposals := []PromotedProposal{
		{
			ProposalID:        101,
			Predicate:         json.RawMessage(`{"version":1,"expression":{"kind":"all","items":[]}}`),
			PredicateChecksum: "sha256:abc123",
		},
	}

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id FROM kb.pipeline_bindings`)).
		WithArgs("module_release:1").
		WillReturnError(sql.ErrNoRows)

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, version FROM kb.pipelines WHERE name = $1 AND status = 'active' LIMIT 1`)).
		WithArgs(DefaultProductionPipelineName).
		WillReturnRows(sqlmock.NewRows([]string{"id", "version"}).AddRow(int64(7), int64(2)))

	// active=false is the load-bearing assertion here.
	mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO kb.pipeline_bindings`)).
		WithArgs(int64(7), "module_release:1", string(proposals[0].Predicate), proposals[0].PredicateChecksum).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(99)))

	bindingID, err := store.EnsureDraftFromModuleRelease(
		context.Background(), tx, 1, "checksum-xyz", proposals,
	)
	if err != nil {
		t.Fatalf("EnsureDraftFromModuleRelease: %v", err)
	}
	if bindingID != 99 {
		t.Fatalf("bindingID=%d want 99", bindingID)
	}

	// ExpectationsWereMet confirms no unexpected queries (e.g., an active=true
	// UPDATE) were executed -- promotion never activates.
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unexpected queries (activation should not occur): %v", err)
	}
}

// TestPromotionIdempotentAcrossReRuns proves re-promoting the same release
// returns the already-materialized binding without creating a duplicate (P5
// review 2026080302 finding P5-10's wrong idempotency key), regardless of
// whether that binding has since been activated (active flipped to true) --
// the idempotency lookup matches on name alone, not on active state.
func TestPromotionIdempotentAcrossReRuns(t *testing.T) {
	_, mock, tx := beginPromotionTx(t)

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id FROM kb.pipeline_bindings`)).
		WithArgs("module_release:1").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(55)))

	bindingID, err := (PolicyPromotionStore{}).EnsureDraftFromModuleRelease(
		context.Background(), tx, 1, "checksum-xyz", []PromotedProposal{{ProposalID: 7, Predicate: json.RawMessage(`{}`), PredicateChecksum: "c"}},
	)
	if err != nil {
		t.Fatalf("EnsureDraftFromModuleRelease: %v", err)
	}
	if bindingID != 55 {
		t.Fatalf("bindingID=%d want existing 55", bindingID)
	}
	// No INSERT/pipeline-lookup query may have been issued.
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unexpected queries (idempotent hit must issue no inserts): %v", err)
	}
}

// TestPromotionCreatesDistinctBindingForDistinctRelease proves a genuinely
// new release (a different source_ref) gets its own binding rather than
// reusing the previous release's.
func TestPromotionCreatesDistinctBindingForDistinctRelease(t *testing.T) {
	_, mock, tx := beginPromotionTx(t)

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id FROM kb.pipeline_bindings`)).
		WithArgs("module_release:2").
		WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, version FROM kb.pipelines WHERE name = $1 AND status = 'active' LIMIT 1`)).
		WithArgs(DefaultProductionPipelineName).
		WillReturnRows(sqlmock.NewRows([]string{"id", "version"}).AddRow(int64(7), int64(2)))
	mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO kb.pipeline_bindings`)).
		WithArgs(int64(7), "module_release:2", `{}`, "d").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(88)))

	bindingID, err := (PolicyPromotionStore{}).EnsureDraftFromModuleRelease(
		context.Background(), tx, 2, "checksum-2", []PromotedProposal{{ProposalID: 8, Predicate: json.RawMessage(`{}`), PredicateChecksum: "d"}},
	)
	if err != nil {
		t.Fatalf("EnsureDraftFromModuleRelease: %v", err)
	}
	if bindingID != 88 {
		t.Fatalf("bindingID=%d want 88", bindingID)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

// TestPromotionDefaultPipelineLookupErrorPropagates proves a failed default-
// pipeline lookup is surfaced instead of silently materializing a binding
// against a zero-value pipeline id (P5 review 2026080302 finding P5-10's
// "surface lookup failures" principle, now applied to the pipeline lookup
// that replaced the retired MAX(version) policy lookup).
func TestPromotionDefaultPipelineLookupErrorPropagates(t *testing.T) {
	_, mock, tx := beginPromotionTx(t)

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id FROM kb.pipeline_bindings`)).
		WithArgs("module_release:1").
		WillReturnError(sql.ErrNoRows)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, version FROM kb.pipelines WHERE name = $1 AND status = 'active' LIMIT 1`)).
		WithArgs(DefaultProductionPipelineName).
		WillReturnError(fmt.Errorf("connection refused"))

	if _, err := (PolicyPromotionStore{}).EnsureDraftFromModuleRelease(
		context.Background(), tx, 1, "checksum", []PromotedProposal{{ProposalID: 1, Predicate: json.RawMessage(`{}`), PredicateChecksum: "c"}},
	); err == nil {
		t.Fatal("expected the default-pipeline lookup error to propagate")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
